"""FastAPI service factory for transcription WebSocket sessions and admin routes."""

from __future__ import annotations

import asyncio
from dataclasses import replace
from datetime import datetime, timezone
import logging
import os
from typing import Any

try:
    from fastapi import FastAPI, WebSocket, WebSocketDisconnect
except ImportError:
    FastAPI = None  # type: ignore[assignment]
    WebSocket = None  # type: ignore[assignment]
    WebSocketDisconnect = None  # type: ignore[assignment]

from .admin import AdminSettings, create_admin_router
from .admin_store import AdminAudioTuningConfig, AdminUsageEvent, DynamoDBAdminStore, DynamoDBAudioTuningStore
from .asr import ASRProviderError, ASRProviderFactory, ASRProviderSettings
from .auth import AuthError, TokenAuthenticator
from .diarization import (
    DiarizationProviderError,
    DiarizationProviderFactory,
    DiarizationProviderSettings,
)
from .logging_utils import log_event
from .protocol import AudioFrameError, SessionConfigError, decode_start_session, encode_event
from .session import BackpressureError, TranscriptionSession, decode_client_text_message
from .user_registry import UserRegistrySettings


logger = logging.getLogger("transcription_service")


def health_payload(
    settings: ASRProviderSettings | None = None,
    diarization_settings: DiarizationProviderSettings | None = None,
    user_registry_settings: UserRegistrySettings | None = None,
) -> dict[str, Any]:
    factory = ASRProviderFactory(settings=settings or ASRProviderSettings.from_environment())
    diarization_factory = DiarizationProviderFactory(
        settings=diarization_settings or DiarizationProviderSettings.from_environment()
    )
    runtime_status = factory.runtime_status()
    diarization_status = diarization_factory.runtime_status()
    registry_status = (user_registry_settings or UserRegistrySettings.from_environment()).runtime_status()
    service_status = (
        "ok"
        if runtime_status.get("ready", False) and diarization_status.get("ready", False)
        else "degraded"
    )
    return {
        **runtime_status,
        "status": service_status,
        "service": "cubicle-transcription-service",
        "asr_provider": runtime_status.get("provider"),
        "diarization_provider": diarization_status.get("provider"),
        "diarization": diarization_status,
        "user_registry": registry_status,
        "retention": "disabled",
    }


def readiness_payload(
    settings: ASRProviderSettings | None = None,
    diarization_settings: DiarizationProviderSettings | None = None,
    user_registry_settings: UserRegistrySettings | None = None,
) -> dict[str, Any]:
    asr_settings = settings or ASRProviderSettings.from_environment()
    diarization = diarization_settings or DiarizationProviderSettings.from_environment()
    registry = user_registry_settings or UserRegistrySettings.from_environment()
    return {
        "status": "ok",
        "service": "cubicle-transcription-service",
        "asr_provider": asr_settings.provider,
        "diarization_provider": diarization.provider,
        "user_registry": registry.runtime_status(),
        "retention": "disabled",
    }


def create_app():
    if FastAPI is None or WebSocket is None or WebSocketDisconnect is None:
        raise RuntimeError(
            "FastAPI runtime dependencies are not installed. "
            "Install requirements.txt before running the WebSocket service."
        )

    app = FastAPI(title="Cubicle Transcription Service", version="0.1.0")
    authenticator = TokenAuthenticator.from_environment()
    asr_settings = ASRProviderSettings.from_environment()
    diarization_settings = DiarizationProviderSettings.from_environment()
    diarization_factory = DiarizationProviderFactory(settings=diarization_settings)
    user_registry_settings = UserRegistrySettings.from_environment()
    admin_settings = AdminSettings.from_environment()
    usage_store = _create_usage_store_from_environment()
    audio_tuning_store = _create_audio_tuning_store_from_environment()
    diarization_stop_timeout_seconds = _effective_diarization_stop_timeout_seconds(
        diarization_settings,
        _env_float("TRANSCRIPTION_DIARIZATION_STOP_TIMEOUT_SECONDS", 45.0),
    )
    diarization_warmup_enabled = _env_bool("TRANSCRIPTION_DIARIZATION_WARMUP_ENABLED", True)

    if admin_settings.enabled:
        app.include_router(create_admin_router(settings=admin_settings))

    @app.on_event("startup")
    async def warm_diarization_pipeline() -> None:
        if not diarization_warmup_enabled or diarization_settings.provider != "pyannote":
            return

        async def warm() -> None:
            log_event(
                logger,
                "diarization_warmup_started",
                provider=diarization_settings.provider,
                model_name=diarization_settings.model_name,
                device=diarization_settings.device,
            )
            try:
                await asyncio.to_thread(diarization_factory.create_provider().warmup)
            except Exception as exc:
                log_event(
                    logger,
                    "diarization_warmup_failed",
                    provider=diarization_settings.provider,
                    error_class=type(exc).__name__,
                )
                return
            log_event(
                logger,
                "diarization_warmup_completed",
                provider=diarization_settings.provider,
                model_name=diarization_settings.model_name,
                device=diarization_settings.device,
            )

        app.state.diarization_warmup_task = asyncio.create_task(warm())

    @app.get("/healthz")
    async def healthz():
        return readiness_payload(asr_settings, diarization_settings, user_registry_settings)

    @app.get("/runtimez")
    async def runtimez():
        return health_payload(asr_settings, diarization_settings, user_registry_settings)

    @app.websocket("/v1/transcription")
    async def transcription_socket(websocket: WebSocket):
        authorization = websocket.headers.get("authorization")
        try:
            auth_context = authenticator.validate_authorization_header(authorization)
        except AuthError as exc:
            log_event(
                logger,
                "websocket_auth_failed",
                auth_required=authenticator.auth_required,
                auth_mode=authenticator.auth_mode,
                authorization_present=bool(authorization),
                auth_error=str(exc),
            )
            await websocket.close(code=4401, reason="unauthorized")
            return

        await websocket.accept()
        session: TranscriptionSession | None = None
        session_started_at: datetime | None = None
        usage_recorded = False

        def record_usage_once(reason: str) -> None:
            nonlocal usage_recorded
            if usage_recorded or session is None or usage_store is None:
                return
            usage_recorded = True
            audit_user = session.auth_context.audit_user
            if not audit_user:
                return
            metrics = session.usage_metrics()
            stopped_at = datetime.now(timezone.utc)
            try:
                usage_store.record_usage_event(
                    AdminUsageEvent(
                        email=audit_user,
                        session_id=session.config.session_id,
                        token_id=session.auth_context.token_id,
                        auth_mode=session.auth_context.mode,
                        language_mode=session.config.language_mode,
                        diarization_enabled=session.config.diarization_enabled,
                        started_at=_format_dt(session_started_at or stopped_at),
                        stopped_at=_format_dt(stopped_at),
                        audio_bytes=metrics["audio_bytes"],
                        audio_ms=metrics["audio_ms"],
                        stop_reason=reason,
                    )
                )
            except Exception:
                logger.exception("failed to record metadata-only transcription usage")

        try:
            start_message = await websocket.receive_text()
            config = decode_start_session(start_message)
            session_started_at = datetime.now(timezone.utc)
            session_asr_settings = _asr_settings_for_new_session(asr_settings, audio_tuning_store)
            session = TranscriptionSession(
                config=config,
                auth_context=auth_context,
                asr_provider=ASRProviderFactory(settings=session_asr_settings).create_provider(),
                diarization_provider=diarization_factory.create_provider(),
            )
            for event in session.start_events():
                await websocket.send_text(event)
            log_event(
                logger,
                "session_started",
                session_id=config.session_id,
                authenticated_user_id=auth_context.audit_user,
                auth_mode=auth_context.mode,
                language_mode=config.language_mode,
                diarization_enabled=config.diarization_enabled,
                audio_tuning_target_rms=session_asr_settings.voxtral_input_rms_target,
                audio_tuning_rms_floor=session_asr_settings.voxtral_input_rms_floor,
                audio_tuning_max_gain=session_asr_settings.voxtral_input_max_gain,
                audio_tuning_peak_ceiling=session_asr_settings.voxtral_input_peak_ceiling,
            )

            while True:
                message = await websocket.receive()
                if message.get("type") == "websocket.disconnect":
                    record_usage_once("websocket_disconnect")
                    break
                if message.get("bytes") is not None:
                    assert session is not None
                    for event in session.receive_audio_frame(message["bytes"]):
                        await websocket.send_text(event)
                    continue

                text_payload = message.get("text")
                if text_payload is None:
                    continue
                message_type, _ = decode_client_text_message(text_payload)
                if message_type == "stop_session":
                    assert session is not None
                    final_events = session.stop_transcript_events()
                    for event in final_events:
                        await websocket.send_text(event)
                    for event in session.stop_processing_events(final_events):
                        await websocket.send_text(event)
                    try:
                        speaker_events = await asyncio.wait_for(
                            asyncio.to_thread(session.stop_diarization_events, final_events),
                            timeout=diarization_stop_timeout_seconds,
                        )
                    except TimeoutError:
                        log_event(
                            logger,
                            "diarization_stop_timeout",
                            session_id=session.config.session_id,
                            timeout_seconds=diarization_stop_timeout_seconds,
                            audio_ms=session.usage_metrics()["audio_ms"],
                        )
                        speaker_events = [
                            encode_event(
                                "diarization_status",
                                session_id=session.config.session_id,
                                status="timed_out",
                                timeout_seconds=diarization_stop_timeout_seconds,
                                audio_ms=session.usage_metrics()["audio_ms"],
                            )
                        ]
                    for event in speaker_events:
                        await websocket.send_text(event)
                    await websocket.send_text(session.session_stopped_event())
                    record_usage_once("client_stop")
                    break
                await websocket.send_text(
                    encode_event("error", code="unsupported_message", message="unsupported client message")
                )
        except WebSocketDisconnect:
            if session is not None:
                session.stop_events()
                record_usage_once("websocket_disconnect")
        except RuntimeError as exc:
            if "disconnect message has been received" in str(exc):
                if session is not None:
                    session.stop_events()
                    record_usage_once("websocket_disconnect")
                return
            raise
        except (
            ASRProviderError,
            DiarizationProviderError,
            BackpressureError,
            AudioFrameError,
            SessionConfigError,
            ValueError,
        ) as exc:
            record_usage_once("bad_request")
            await websocket.send_text(encode_event("error", code="bad_request", message=str(exc)))
            await websocket.close(code=4400, reason="bad_request")
        except Exception as exc:
            logger.exception("transcription websocket failed")
            record_usage_once("internal_error")
            await websocket.send_text(encode_event("error", code="internal_error", message=type(exc).__name__))
            await websocket.close(code=1011, reason="internal_error")

    return app


def _asr_settings_for_new_session(
    base_settings: ASRProviderSettings,
    audio_tuning_store: DynamoDBAudioTuningStore | None,
) -> ASRProviderSettings:
    if audio_tuning_store is None:
        return base_settings
    try:
        tuning = audio_tuning_store.get_audio_tuning_config()
    except Exception:
        logger.exception("failed to load runtime audio tuning config; using startup defaults")
        return base_settings
    if tuning is None:
        return base_settings
    return _apply_audio_tuning(base_settings, tuning)


def _apply_audio_tuning(
    settings: ASRProviderSettings,
    tuning: AdminAudioTuningConfig,
) -> ASRProviderSettings:
    return replace(
        settings,
        voxtral_input_rms_target=tuning.target_rms,
        voxtral_input_rms_floor=tuning.rms_floor,
        voxtral_input_max_gain=tuning.max_gain,
        voxtral_input_peak_ceiling=tuning.peak_ceiling,
    )


def _create_usage_store_from_environment() -> DynamoDBAdminStore | None:
    user_table_name = _env_text("TRANSCRIPTION_USER_REGISTRY_TABLE")
    token_ledger_table_name = _env_text("TRANSCRIPTION_TOKEN_LEDGER_TABLE")
    audit_table_name = _env_text("TRANSCRIPTION_ADMIN_AUDIT_TABLE")
    if not user_table_name or not token_ledger_table_name or not audit_table_name:
        return None
    try:
        import boto3  # type: ignore[import-not-found]
    except ImportError:
        logger.warning("boto3 is not installed; transcription usage recording disabled")
        return None
    return DynamoDBAdminStore(
        client=boto3.client("dynamodb", region_name=_env_text("AWS_REGION") or _env_text("AWS_DEFAULT_REGION")),
        user_table_name=user_table_name,
        token_ledger_table_name=token_ledger_table_name,
        audit_table_name=audit_table_name,
    )


def _create_audio_tuning_store_from_environment() -> DynamoDBAudioTuningStore | None:
    table_name = _env_text("TRANSCRIPTION_RUNTIME_CONFIG_TABLE") or _env_text("TRANSCRIPTION_USER_REGISTRY_TABLE")
    if not table_name:
        return None
    try:
        import boto3  # type: ignore[import-not-found]
    except ImportError:
        logger.warning("boto3 is not installed; runtime audio tuning config disabled")
        return None
    return DynamoDBAudioTuningStore(
        client=boto3.client("dynamodb", region_name=_env_text("AWS_REGION") or _env_text("AWS_DEFAULT_REGION")),
        table_name=table_name,
    )


def _env_text(name: str) -> str | None:
    value = os.environ.get(name, "").strip()
    return value or None


def _env_float(name: str, default: float) -> float:
    value = os.environ.get(name, "").strip()
    if not value:
        return default
    try:
        parsed = float(value)
    except ValueError:
        logger.warning("%s must be a number; using default %.1f", name, default)
        return default
    return max(0.1, parsed)


def _env_bool(name: str, default: bool) -> bool:
    value = os.environ.get(name, "").strip().lower()
    if not value:
        return default
    return value in {"1", "true", "yes", "on"}


def _effective_diarization_stop_timeout_seconds(
    settings: DiarizationProviderSettings,
    configured_stop_timeout_seconds: float,
) -> float:
    if settings.provider not in {"remote_http", "worker_http"}:
        return configured_stop_timeout_seconds
    return max(configured_stop_timeout_seconds, settings.worker_timeout_seconds + 5.0)


def _format_dt(value: datetime) -> str:
    return value.astimezone(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


try:
    app = create_app()
except RuntimeError:
    app = None
