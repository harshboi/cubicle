from __future__ import annotations

import asyncio
import json
import os
import struct
import tempfile
import time
from typing import get_type_hints
import unittest
from unittest.mock import patch

from transcription_service.asr import (
    ASRProviderSettings,
    VLLMVoxtralRealtimeStream,
    _merge_realtime_text_fragments,
)
from transcription_service.admin import _render_audio_tuning
from transcription_service.admin_store import (
    AdminAudioTuningConfig,
    AdminStoreError,
    DynamoDBAudioTuningStore,
    InMemoryAdminStore,
    audio_tuning_config_pk,
    normalize_audio_tuning_config,
)
from transcription_service.auth import AuthError, TokenAuthenticator, mint_signed_user_token
from transcription_service.logging_utils import safe_log_payload
from transcription_service.protocol import (
    AudioFrameError,
    SessionConfigError,
    decode_start_session,
    encode_event,
    speaker_update_event,
    validate_audio_frame,
)
from transcription_service.server import FastAPI, _asr_settings_for_new_session, create_app, health_payload
from transcription_service.user_registry import DynamoDBUserRegistry, UserRegistrySettings


def start_payload(**overrides):
    payload = {
        "type": "start_session",
        "protocol_version": "transcription.v1",
        "session_id": "session-1",
        "transcription_enabled": True,
        "diarization_enabled": True,
        "language_mode": "english_to_english",
        "sample_rate": 16000,
        "channel_count": 1,
        "audio_encoding": "pcm_s16le",
        "client_timestamp": "2026-05-17T17:00:00.000Z",
        "app_version": "0.1.0",
        "privacy_safe_device_id": "device-1",
    }
    payload.update(overrides)
    return json.dumps(payload)


class FakeDynamoDBClient:
    def __init__(self, items=None, *, fail=False):
        self.items = items or {}
        self.fail = fail
        self.calls = []

    def get_item(self, *, TableName, Key, ConsistentRead):
        del ConsistentRead
        if self.fail:
            raise RuntimeError("dynamodb unavailable")
        pk = Key["pk"]["S"]
        sk = Key.get("sk", {}).get("S")
        self.calls.append((TableName, pk, sk))
        item = self.items.get((TableName, pk, sk))
        if item is None:
            return {}
        return {"Item": item}

    def put_item(self, *, TableName, Item):
        if self.fail:
            raise RuntimeError("dynamodb unavailable")
        pk = Item["pk"]["S"]
        sk = Item.get("sk", {}).get("S")
        self.calls.append(("put", TableName, pk, sk))
        self.items[(TableName, pk, sk)] = Item


class ProtocolTests(unittest.TestCase):
    def test_session_config_validation_accepts_supported_language_modes(self):
        english = decode_start_session(start_payload(language_mode="english_to_english"))
        japanese = decode_start_session(start_payload(language_mode="japanese_to_english"))
        multilingual = decode_start_session(start_payload(language_mode="multilingual_to_english"))

        self.assertEqual(english.model_route, "voxtral-realtime-transcribe-en")
        self.assertEqual(japanese.model_route, "voxtral-realtime-ja-en")
        self.assertEqual(multilingual.model_route, "voxtral-realtime-auto-source")

    def test_session_config_validation_rejects_disabled_or_bad_audio_contract(self):
        with self.assertRaises(SessionConfigError):
            decode_start_session(start_payload(transcription_enabled=False))
        with self.assertRaises(SessionConfigError):
            decode_start_session(start_payload(sample_rate=48000))
        with self.assertRaises(SessionConfigError):
            decode_start_session(start_payload(language_mode="english_to_japanese"))

    def test_auth_failure_rejection(self):
        authenticator = TokenAuthenticator(expected_token="expected")

        with self.assertRaises(AuthError):
            authenticator.validate_authorization_header(None)
        with self.assertRaises(AuthError):
            authenticator.validate_authorization_header("Bearer wrong")
        context = authenticator.validate_authorization_header("Bearer expected")
        self.assertEqual(context.mode, "shared_token")

    def test_signed_user_token_accepts_allowed_user(self):
        token = mint_signed_user_token(
            signing_secret="secret",
            subject="user-1",
            email="prabhat7@cisco.com",
            token_id="token-1",
        )
        authenticator = TokenAuthenticator(
            auth_mode="signed_user_token",
            signing_secret="secret",
            allowed_users=frozenset({"prabhat7@cisco.com"}),
        )

        context = authenticator.validate_authorization_header(f"Bearer {token}")

        self.assertEqual(context.mode, "signed_user_token")
        self.assertEqual(context.user_id, "user-1")
        self.assertEqual(context.email, "prabhat7@cisco.com")
        self.assertEqual(context.token_id, "token-1")

    def test_signed_user_token_rejects_disallowed_user(self):
        token = mint_signed_user_token(
            signing_secret="secret",
            subject="user-2",
            email="outside@example.com",
        )
        authenticator = TokenAuthenticator(
            auth_mode="signed_user_token",
            signing_secret="secret",
            allowed_users=frozenset({"prabhat7@cisco.com"}),
        )

        with self.assertRaises(AuthError):
            authenticator.validate_authorization_header(f"Bearer {token}")

    def test_signed_user_token_rejects_expired_revoked_or_wrong_scope(self):
        expired = mint_signed_user_token(signing_secret="secret", subject="user-1", ttl_seconds=-1)
        revoked = mint_signed_user_token(
            signing_secret="secret",
            subject="user-1",
            token_id="revoked-token",
        )
        wrong_scope = mint_signed_user_token(
            signing_secret="secret",
            subject="user-1",
            scopes=("profile:read",),
        )
        authenticator = TokenAuthenticator(
            auth_mode="signed_user_token",
            signing_secret="secret",
            revoked_token_ids=frozenset({"revoked-token"}),
        )

        with self.assertRaises(AuthError):
            authenticator.validate_authorization_header(f"Bearer {expired}")
        with self.assertRaises(AuthError):
            authenticator.validate_authorization_header(f"Bearer {revoked}")
        with self.assertRaises(AuthError):
            authenticator.validate_authorization_header(f"Bearer {wrong_scope}")

    def test_signed_user_token_can_be_loaded_from_environment(self):
        with tempfile.NamedTemporaryFile("w", delete=False) as signing_secret:
            signing_secret.write("secret\n")
            signing_secret_path = signing_secret.name
        self.addCleanup(lambda: os.path.exists(signing_secret_path) and os.unlink(signing_secret_path))
        token = mint_signed_user_token(
            signing_secret="secret",
            subject="prabhat7@cisco.com",
            email="prabhat7@cisco.com",
        )

        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_AUTH_MODE": "signed_user_token",
                "TRANSCRIPTION_TOKEN_SIGNING_SECRET_FILE": signing_secret_path,
                "TRANSCRIPTION_ALLOWED_USERS": "prabhat7@cisco.com",
            },
            clear=True,
        ):
            authenticator = TokenAuthenticator.from_environment()

        context = authenticator.validate_authorization_header(f"Bearer {token}")

        self.assertEqual(context.audit_user, "prabhat7@cisco.com")

    def test_signed_user_token_accepts_dynamic_registry_user(self):
        token = mint_signed_user_token(
            signing_secret="secret",
            subject="user-1",
            email="prabhat7@cisco.com",
            token_id="token-1",
        )
        client = FakeDynamoDBClient(
            {
                ("users", "USER#prabhat7@cisco.com", None): {"status": {"S": "active"}},
                ("tokens", "USER#prabhat7@cisco.com", "TOKEN#token-1"): {
                    "status": {"S": "active"}
                },
            }
        )
        authenticator = TokenAuthenticator(
            auth_mode="signed_user_token",
            signing_secret="secret",
            user_registry=DynamoDBUserRegistry(
                client=client,
                user_table_name="users",
                token_ledger_table_name="tokens",
                require_token_ledger=True,
                cache_ttl_seconds=0,
            ),
        )

        context = authenticator.validate_authorization_header(f"Bearer {token}")

        self.assertEqual(context.audit_user, "prabhat7@cisco.com")
        self.assertIn(("users", "USER#prabhat7@cisco.com", None), client.calls)
        self.assertIn(("tokens", "USER#prabhat7@cisco.com", "TOKEN#token-1"), client.calls)

    def test_signed_user_token_registry_overrides_static_allowed_user_list(self):
        token = mint_signed_user_token(
            signing_secret="secret",
            subject="user-2",
            email="neelamsngh09@gmail.com",
            token_id="token-2",
        )
        client = FakeDynamoDBClient(
            {
                ("users", "USER#neelamsngh09@gmail.com", None): {"status": {"S": "active"}},
                ("tokens", "USER#neelamsngh09@gmail.com", "TOKEN#token-2"): {
                    "status": {"S": "active"}
                },
            }
        )
        authenticator = TokenAuthenticator(
            auth_mode="signed_user_token",
            signing_secret="secret",
            allowed_users=frozenset({"prabhatsingh@gmail.com"}),
            user_registry=DynamoDBUserRegistry(
                client=client,
                user_table_name="users",
                token_ledger_table_name="tokens",
                require_token_ledger=True,
                cache_ttl_seconds=0,
            ),
        )

        context = authenticator.validate_authorization_header(f"Bearer {token}")

        self.assertEqual(context.audit_user, "neelamsngh09@gmail.com")
        self.assertIn(("users", "USER#neelamsngh09@gmail.com", None), client.calls)
        self.assertIn(("tokens", "USER#neelamsngh09@gmail.com", "TOKEN#token-2"), client.calls)

    def test_signed_user_token_rejects_dynamic_registry_disabled_or_revoked_user(self):
        disabled = mint_signed_user_token(
            signing_secret="secret",
            subject="disabled@example.com",
            email="disabled@example.com",
            token_id="disabled-token",
        )
        revoked = mint_signed_user_token(
            signing_secret="secret",
            subject="active@example.com",
            email="active@example.com",
            token_id="revoked-token",
        )
        client = FakeDynamoDBClient(
            {
                ("users", "USER#disabled@example.com", None): {"status": {"S": "disabled"}},
                ("users", "USER#active@example.com", None): {"status": {"S": "active"}},
                ("tokens", "USER#active@example.com", "TOKEN#revoked-token"): {
                    "status": {"S": "revoked"}
                },
            }
        )
        authenticator = TokenAuthenticator(
            auth_mode="signed_user_token",
            signing_secret="secret",
            user_registry=DynamoDBUserRegistry(
                client=client,
                user_table_name="users",
                token_ledger_table_name="tokens",
                require_token_ledger=True,
                cache_ttl_seconds=0,
            ),
        )

        with self.assertRaisesRegex(AuthError, "user is not active"):
            authenticator.validate_authorization_header(f"Bearer {disabled}")
        with self.assertRaisesRegex(AuthError, "signed token has been revoked"):
            authenticator.validate_authorization_header(f"Bearer {revoked}")

    def test_signed_user_token_fails_closed_when_dynamic_registry_lookup_fails(self):
        token = mint_signed_user_token(
            signing_secret="secret",
            subject="active@example.com",
            email="active@example.com",
            token_id="token-1",
        )
        authenticator = TokenAuthenticator(
            auth_mode="signed_user_token",
            signing_secret="secret",
            user_registry=DynamoDBUserRegistry(
                client=FakeDynamoDBClient(fail=True),
                user_table_name="users",
                cache_ttl_seconds=0,
            ),
        )

        with self.assertRaisesRegex(AuthError, "user registry lookup failed"):
            authenticator.validate_authorization_header(f"Bearer {token}")

    def test_user_registry_settings_load_dynamodb_environment_without_secret_values(self):
        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_USER_REGISTRY_BACKEND": "dynamodb",
                "TRANSCRIPTION_USER_REGISTRY_TABLE": "users",
                "TRANSCRIPTION_TOKEN_LEDGER_TABLE": "tokens",
                "TRANSCRIPTION_USER_REGISTRY_CACHE_TTL_SECONDS": "15",
                "TRANSCRIPTION_USER_REGISTRY_REQUIRE_TOKEN_LEDGER": "true",
            },
            clear=True,
        ):
            settings = UserRegistrySettings.from_environment()

        self.assertEqual(settings.backend, "dynamodb")
        self.assertEqual(settings.user_table_name, "users")
        self.assertEqual(settings.token_ledger_table_name, "tokens")
        self.assertEqual(settings.cache_ttl_seconds, 15)
        self.assertTrue(settings.require_token_ledger)

    def test_auth_token_can_be_loaded_from_file_without_env_secret(self):
        with tempfile.NamedTemporaryFile("w", delete=False) as token_file:
            token_file.write("expected\n")
            token_file_path = token_file.name
        self.addCleanup(lambda: os.path.exists(token_file_path) and os.unlink(token_file_path))

        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_SERVICE_TOKEN": "wrong",
                "TRANSCRIPTION_SERVICE_TOKEN_FILE": token_file_path,
            },
            clear=True,
        ):
            authenticator = TokenAuthenticator.from_environment()

        authenticator.validate_authorization_header("Bearer expected")
        with self.assertRaises(AuthError):
            authenticator.validate_authorization_header("Bearer wrong")

    def test_audio_frame_validation(self):
        validate_audio_frame(bytes([0, 1, 2, 3]))

        with self.assertRaises(AudioFrameError):
            validate_audio_frame(b"")
        with self.assertRaises(AudioFrameError):
            validate_audio_frame(bytes([1]))
        with self.assertRaises(AudioFrameError):
            validate_audio_frame(bytes(64 * 1024 + 2))

    def test_partial_final_event_contract_is_json_and_excludes_audio(self):
        event = json.loads(
            encode_event(
                "partial_transcript",
                segment_id="segment-1",
                start_time_ms=0,
                text="hello",
                is_partial=True,
                language_mode="english_to_english",
                model_name="mock-asr",
                model_version="slice-3",
            )
        )

        self.assertEqual(event["type"], "partial_transcript")
        self.assertEqual(event["segment_id"], "segment-1")
        self.assertEqual(event["language_mode"], "english_to_english")
        self.assertNotIn("audio", event)
        self.assertNotIn("raw_audio", event)

    def test_speaker_update_event_contract_preserves_segment_identity_without_text(self):
        event = json.loads(
            speaker_update_event(
                segment_id="segment-1",
                speaker_id="2",
                start_time_ms=100,
                end_time_ms=900,
                overlap_ms=700,
                is_final=True,
            )
        )

        self.assertEqual(event["type"], "speaker_update")
        self.assertEqual(event["segment_id"], "segment-1")
        self.assertEqual(event["speaker_id"], "2")
        self.assertEqual(event["overlap_ms"], 700)
        self.assertTrue(event["is_final"])
        self.assertNotIn("text", event)

    def test_health_payload(self):
        payload = health_payload(ASRProviderSettings(provider="mock"))

        self.assertEqual(payload["status"], "ok")
        self.assertEqual(payload["asr_provider"], "mock")
        self.assertEqual(payload["diarization_provider"], "mock")
        self.assertEqual(payload["diarization"]["ready"], True)
        self.assertEqual(payload["user_registry"]["backend"], "env")
        self.assertFalse(payload["user_registry"]["enabled"])
        self.assertEqual(payload["retention"], "disabled")
        self.assertIn("gpu_available", payload)

    def test_self_hosted_voxtral_health_has_no_external_api_dependency(self):
        payload = health_payload(
            ASRProviderSettings(
                provider="voxtral_self_hosted",
                voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
                voxtral_model_version="self-hosted-vllm",
                voxtral_realtime_url="ws://127.0.0.1:8000/v1/realtime",
            )
        )

        self.assertEqual(payload["asr_provider"], "voxtral_self_hosted")
        self.assertEqual(payload["external_api_dependency"], False)
        self.assertEqual(payload["compute_type"], "vllm")
        self.assertTrue(payload["realtime_url_configured"])

    def test_vllm_environment_aliases_configure_self_hosted_voxtral_runtime(self):
        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_ASR_PROVIDER": "voxtral_self_hosted",
                "TRANSCRIPTION_VOXTRAL_RUNTIME": "vllm",
                "VLLM_BASE_URL": "http://localhost:8000",
                "VLLM_REALTIME_URL": "ws://localhost:8000/v1/realtime",
                "VLLM_MODEL": "mistralai/Voxtral-Mini-4B-Realtime-2602",
                "TRANSCRIPTION_REQUIRE_GPU": "false",
            },
            clear=True,
        ):
            settings = ASRProviderSettings.from_environment()

        self.assertEqual(settings.provider, "voxtral_self_hosted")
        self.assertEqual(settings.voxtral_runtime, "vllm")
        self.assertEqual(settings.vllm_base_url, "http://localhost:8000")
        self.assertEqual(settings.voxtral_realtime_url, "ws://localhost:8000/v1/realtime")
        self.assertEqual(settings.voxtral_model, "mistralai/Voxtral-Mini-4B-Realtime-2602")
        self.assertFalse(settings.require_gpu)

    def test_vllm_base_url_derives_realtime_url_when_specific_url_is_absent(self):
        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_ASR_PROVIDER": "voxtral_self_hosted",
                "VLLM_BASE_URL": "https://vllm.example.internal:8443",
            },
            clear=True,
        ):
            settings = ASRProviderSettings.from_environment()

        self.assertEqual(settings.voxtral_realtime_url, "wss://vllm.example.internal:8443/v1/realtime")

    def test_vllm_final_response_timeout_is_configurable(self):
        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_ASR_PROVIDER": "voxtral_self_hosted",
                "TRANSCRIPTION_VOXTRAL_FINAL_RESPONSE_TIMEOUT_SECONDS": "45.5",
            },
            clear=True,
        ):
            settings = ASRProviderSettings.from_environment()

        self.assertEqual(settings.voxtral_final_response_timeout_seconds, 45.5)

    def test_vllm_input_normalization_settings_are_configurable(self):
        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_ASR_PROVIDER": "voxtral_self_hosted",
                "TRANSCRIPTION_VOXTRAL_INPUT_RMS_TARGET": "0.18",
                "TRANSCRIPTION_VOXTRAL_INPUT_RMS_FLOOR": "0.01",
                "TRANSCRIPTION_VOXTRAL_INPUT_MAX_GAIN": "12",
                "TRANSCRIPTION_VOXTRAL_INPUT_PEAK_CEILING": "0.85",
            },
            clear=True,
        ):
            settings = ASRProviderSettings.from_environment()

        self.assertEqual(settings.voxtral_input_rms_target, 0.18)
        self.assertEqual(settings.voxtral_input_rms_floor, 0.01)
        self.assertEqual(settings.voxtral_input_max_gain, 12)
        self.assertEqual(settings.voxtral_input_peak_ceiling, 0.85)

    def test_audio_tuning_config_validates_bounds(self):
        config = normalize_audio_tuning_config(
            target_rms=0.20,
            rms_floor=0.008,
            max_gain=24.0,
            peak_ceiling=0.92,
        )

        self.assertEqual(config.target_rms, 0.20)
        self.assertEqual(config.rms_floor, 0.008)
        with self.assertRaisesRegex(AdminStoreError, "rms_floor"):
            normalize_audio_tuning_config(
                target_rms=0.20,
                rms_floor=0.20,
                max_gain=24.0,
                peak_ceiling=0.92,
            )

    def test_admin_audio_tuning_store_round_trips_runtime_config(self):
        store = InMemoryAdminStore()
        saved = store.save_audio_tuning_config(
            AdminAudioTuningConfig(
                target_rms=0.18,
                rms_floor=0.01,
                max_gain=12.0,
                peak_ceiling=0.85,
                updated_by="admin@example.com",
            )
        )

        loaded = store.get_audio_tuning_config()

        self.assertEqual(loaded, saved)
        self.assertEqual(loaded.updated_by, "admin@example.com")

    def test_dynamodb_audio_tuning_store_reads_config_row(self):
        client = FakeDynamoDBClient(
            {
                ("users", audio_tuning_config_pk(), None): {
                    "target_rms": {"N": "0.19"},
                    "rms_floor": {"N": "0.009"},
                    "max_gain": {"N": "18"},
                    "peak_ceiling": {"N": "0.9"},
                    "updated_at": {"S": "2026-05-23T00:00:00Z"},
                    "updated_by": {"S": "admin@example.com"},
                }
            }
        )
        store = DynamoDBAudioTuningStore(client=client, table_name="users")

        loaded = store.get_audio_tuning_config()

        self.assertEqual(loaded.target_rms, 0.19)
        self.assertEqual(loaded.rms_floor, 0.009)
        self.assertEqual(loaded.max_gain, 18)
        self.assertEqual(loaded.peak_ceiling, 0.9)
        self.assertIn(("users", audio_tuning_config_pk(), None), client.calls)

    def test_runtime_audio_tuning_overrides_asr_settings_for_new_sessions(self):
        client = FakeDynamoDBClient(
            {
                ("users", audio_tuning_config_pk(), None): {
                    "target_rms": {"N": "0.18"},
                    "rms_floor": {"N": "0.01"},
                    "max_gain": {"N": "12"},
                    "peak_ceiling": {"N": "0.85"},
                }
            }
        )
        base_settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_input_rms_target=0.20,
            voxtral_input_rms_floor=0.008,
            voxtral_input_max_gain=24.0,
            voxtral_input_peak_ceiling=0.92,
        )

        settings = _asr_settings_for_new_session(
            base_settings,
            DynamoDBAudioTuningStore(client=client, table_name="users"),
        )

        self.assertEqual(settings.voxtral_input_rms_target, 0.18)
        self.assertEqual(settings.voxtral_input_rms_floor, 0.01)
        self.assertEqual(settings.voxtral_input_max_gain, 12)
        self.assertEqual(settings.voxtral_input_peak_ceiling, 0.85)

    def test_admin_audio_tuning_page_shows_recommended_values(self):
        rendered = _render_audio_tuning(
            config=AdminAudioTuningConfig(),
            using_defaults=True,
            csrf_token="csrf",
        )

        self.assertIn("Recommended: 20%", rendered)
        self.assertIn("Recommended: 0.8%", rendered)
        self.assertIn("Recommended: 24x", rendered)
        self.assertIn("Recommended: 92%", rendered)
        self.assertIn("New sessions", rendered)

    def test_vllm_realtime_contract_uses_top_level_model_and_final_commit(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        payload = stream._session_update_payload()
        websocket = FakeRealtimeWebSocket()

        asyncio.run(stream._commit_audio_buffer(websocket, final=True))

        self.assertEqual(payload["type"], "session.update")
        self.assertEqual(payload["model"], "mistralai/Voxtral-Mini-4B-Realtime-2602")
        self.assertEqual(payload["session"]["model"], "mistralai/Voxtral-Mini-4B-Realtime-2602")
        self.assertEqual(
            websocket.messages,
            [
                '{"type": "input_audio_buffer.commit"}',
                '{"type": "input_audio_buffer.commit", "final": true}',
            ],
        )

    def test_vllm_realtime_sender_does_not_commit_while_generation_is_active(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        websocket = FakeRealtimeWebSocket()
        stream._audio_queue.put(bytes(16_000))
        stream._audio_queue.put(bytes(16_000))
        stream._audio_queue.put(bytes(16_000))
        stream._audio_queue.put(None)

        asyncio.run(stream._send_audio_frames(websocket))

        sent_payloads = [json.loads(message) for message in websocket.messages]
        self.assertEqual(
            [payload["type"] for payload in sent_payloads],
            [
                "input_audio_buffer.append",
                "input_audio_buffer.append",
                "input_audio_buffer.commit",
                "input_audio_buffer.append",
                "input_audio_buffer.commit",
            ],
        )
        self.assertNotIn("final", sent_payloads[2])
        self.assertTrue(sent_payloads[-1]["final"])

    def test_vllm_input_normalization_lifts_quiet_speech_to_target_rms(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
            voxtral_input_rms_target=0.20,
            voxtral_input_rms_floor=0.008,
            voxtral_input_max_gain=24.0,
            voxtral_input_peak_ceiling=0.92,
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        quiet_speech = struct.pack("<h", 327) * 1600

        _normalized, telemetry = stream._normalize_audio_frame(quiet_speech)

        self.assertAlmostEqual(telemetry["input_rms"], 0.01, places=3)
        self.assertGreater(telemetry["applied_gain"], 19)
        self.assertAlmostEqual(telemetry["output_rms"], 0.20, places=2)
        self.assertLessEqual(telemetry["output_peak"], 0.92)

    def test_vllm_input_normalization_leaves_near_silence_below_floor(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
            voxtral_input_rms_target=0.20,
            voxtral_input_rms_floor=0.008,
            voxtral_input_max_gain=24.0,
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        near_silence = struct.pack("<h", 80) * 1600

        normalized, telemetry = stream._normalize_audio_frame(near_silence)

        self.assertEqual(normalized, near_silence)
        self.assertEqual(telemetry["applied_gain"], 1.0)

    def test_vllm_realtime_sender_reopens_stale_generation_window(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        websocket = FakeRealtimeWebSocket()
        stream._generation_started = True
        stream._generation_started_at_monotonic = 0.0
        stream._audio_queue.put(bytes(32_000))
        stream._audio_queue.put(None)

        asyncio.run(stream._send_audio_frames(websocket))

        sent_payloads = [json.loads(message) for message in websocket.messages]
        self.assertEqual(
            [payload["type"] for payload in sent_payloads],
            [
                "input_audio_buffer.append",
                "input_audio_buffer.commit",
                "input_audio_buffer.commit",
            ],
        )
        self.assertNotIn("final", sent_payloads[1])
        self.assertTrue(sent_payloads[-1]["final"])

    def test_vllm_realtime_sender_reopens_stale_generation_window_for_short_next_line(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        websocket = FakeRealtimeWebSocket()
        stream._generation_started = True
        stream._generation_started_at_monotonic = 0.0
        stream._audio_queue.put(bytes(8_000))
        stream._audio_queue.put(None)

        asyncio.run(stream._send_audio_frames(websocket))

        sent_payloads = [json.loads(message) for message in websocket.messages]
        self.assertEqual(
            [payload["type"] for payload in sent_payloads],
            [
                "input_audio_buffer.append",
                "input_audio_buffer.commit",
                "input_audio_buffer.commit",
            ],
        )
        self.assertNotIn("final", sent_payloads[1])
        self.assertTrue(sent_payloads[-1]["final"])

    def test_vllm_realtime_sender_reopens_long_generation_window_even_with_recent_delta(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        websocket = FakeRealtimeWebSocket()
        stream._generation_started = True
        stream._generation_started_at_monotonic = time.monotonic() - stream._generation_max_window_seconds() - 0.1
        stream._last_delta_at_monotonic = time.monotonic()
        stream._audio_queue.put(bytes(32_000))
        stream._audio_queue.put(None)

        asyncio.run(stream._send_audio_frames(websocket))

        sent_payloads = [json.loads(message) for message in websocket.messages]
        self.assertEqual(
            [payload["type"] for payload in sent_payloads],
            [
                "input_audio_buffer.append",
                "input_audio_buffer.commit",
                "input_audio_buffer.commit",
            ],
        )
        self.assertNotIn("final", sent_payloads[1])
        self.assertTrue(sent_payloads[-1]["final"])

    def test_vllm_realtime_stale_window_uses_last_text_delta_as_activity(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        stream._generation_started = True
        stream._generation_started_at_monotonic = 0.0
        stream._last_delta_at_monotonic = time.monotonic()

        self.assertFalse(stream._generation_has_stalled())

    def test_vllm_realtime_receiver_continues_after_response_done(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        websocket = FakeRealtimeWebSocket(
            receive_messages=[
                json.dumps({"type": "response.text.delta", "delta": "first"}),
                json.dumps({"type": "response.done"}),
                json.dumps({"type": "response.text.delta", "delta": "second"}),
                json.dumps({"type": "transcription.done"}),
            ]
        )

        asyncio.run(stream._receive_events(websocket))

        self.assertEqual(stream.drain_text_deltas(), ["first", "second"])

    def test_vllm_realtime_final_wait_allows_delayed_done(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
            voxtral_final_response_timeout_seconds=0.5,
        )
        stream = VLLMVoxtralRealtimeStream(settings)

        async def delayed_done():
            await asyncio.sleep(0.05)

        async def run_wait():
            task = asyncio.create_task(delayed_done())
            await stream._wait_for_final_events(task)
            return task.done(), task.cancelled()

        done, cancelled = asyncio.run(run_wait())

        self.assertTrue(done)
        self.assertFalse(cancelled)

    def test_vllm_realtime_final_wait_cancels_silent_receiver(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
            voxtral_final_response_timeout_seconds=0.01,
        )
        stream = VLLMVoxtralRealtimeStream(settings)

        async def silent_receiver():
            await asyncio.sleep(10)

        async def run_wait():
            task = asyncio.create_task(silent_receiver())
            await stream._wait_for_final_events(task)
            return task.done(), task.cancelled()

        done, cancelled = asyncio.run(run_wait())

        self.assertTrue(done)
        self.assertTrue(cancelled)

    def test_vllm_realtime_stop_waits_for_configured_final_window(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
            voxtral_final_response_timeout_seconds=12.5,
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        thread = FakeThread()
        stream._thread = thread  # type: ignore[assignment]

        stream.stop()

        self.assertEqual(thread.join_timeout, 17.5)

    def test_vllm_realtime_response_done_reopens_live_commit_window(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_realtime_url="ws://localhost:8000/v1/realtime",
        )
        stream = VLLMVoxtralRealtimeStream(settings)
        stream._generation_started = True
        websocket = FakeRealtimeWebSocket(
            receive_messages=[
                json.dumps({"type": "response.done"}),
                json.dumps({"type": "transcription.done"}),
            ]
        )

        asyncio.run(stream._receive_events(websocket))

        self.assertFalse(stream._generation_started)

    def test_realtime_merge_collapses_repeated_full_transcript_artifact(self):
        transcript = (
            "This is a long transcription diagnostic with an alpha marker near the beginning. "
            "The middle marker is blue canyon and the service should keep committing audio. "
            "The final marker is green lantern near the end of the stream."
        )
        partial_transcript = transcript.replace("This is", "This", 1)

        merged = _merge_realtime_text_fragments(partial_transcript, [f"{partial_transcript} {transcript}"])

        self.assertEqual(merged, transcript)

    @unittest.skipIf(FastAPI is None, "FastAPI runtime dependencies are not installed")
    def test_websocket_route_resolves_websocket_type_hint(self):
        app = create_app()
        route = next(route for route in app.routes if getattr(route, "path", None) == "/v1/transcription")
        hints = get_type_hints(route.endpoint)

        self.assertEqual(hints["websocket"].__name__, "WebSocket")

    def test_default_logs_redact_transcript_audio_and_tokens(self):
        payload = safe_log_payload(
            event="session",
            session_id="session-1",
            text="secret transcript",
            audio_bytes=b"not printable",
            token="secret",
            speaker_embedding=[0.1, 0.2],
            word_timestamps=[{"word": "secret", "start": 0.0}],
        )

        self.assertIn('"session_id":"session-1"', payload)
        self.assertNotIn("secret transcript", payload)
        self.assertNotIn("secret", payload)
        self.assertIn('"text":"[redacted]"', payload)
        self.assertIn('"audio_bytes":"[redacted]"', payload)
        self.assertIn('"speaker_embedding":"[redacted]"', payload)
        self.assertIn('"word_timestamps":"[redacted]"', payload)


class FakeRealtimeWebSocket:
    def __init__(self, receive_messages=None):
        self.messages = []
        self.receive_messages = receive_messages or []

    async def send(self, message):
        self.messages.append(message)

    def __aiter__(self):
        self._receive_iterator = iter(self.receive_messages)
        return self

    async def __anext__(self):
        try:
            return next(self._receive_iterator)
        except StopIteration:
            raise StopAsyncIteration


class FakeThread:
    def __init__(self):
        self.join_timeout = None

    def join(self, timeout=None):
        self.join_timeout = timeout

    def is_alive(self):
        return False


if __name__ == "__main__":
    unittest.main()
