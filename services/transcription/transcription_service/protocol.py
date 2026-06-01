from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
import json
from typing import Any, Mapping


PROTOCOL_VERSION = "transcription.v1"
SUPPORTED_LANGUAGE_MODES = {"english_to_english", "japanese_to_english", "multilingual_to_english"}
SUPPORTED_AUDIO_ENCODINGS = {"pcm_s16le"}
MAX_AUDIO_FRAME_BYTES = 64 * 1024


class SessionConfigError(ValueError):
    pass


class AudioFrameError(ValueError):
    pass


@dataclass(frozen=True)
class StartSessionConfig:
    protocol_version: str
    session_id: str
    transcription_enabled: bool
    diarization_enabled: bool
    language_mode: str
    sample_rate: int
    channel_count: int
    audio_encoding: str
    client_timestamp: str
    app_version: str | None = None
    privacy_safe_device_id: str | None = None

    @classmethod
    def from_payload(cls, payload: Mapping[str, Any]) -> "StartSessionConfig":
        if payload.get("type") != "start_session":
            raise SessionConfigError("expected start_session message")

        required = [
            "protocol_version",
            "session_id",
            "transcription_enabled",
            "diarization_enabled",
            "language_mode",
            "sample_rate",
            "channel_count",
            "audio_encoding",
            "client_timestamp",
        ]
        missing = [key for key in required if key not in payload]
        if missing:
            raise SessionConfigError(f"missing required fields: {', '.join(missing)}")

        config = cls(
            protocol_version=str(payload["protocol_version"]),
            session_id=_required_string(payload, "session_id"),
            transcription_enabled=_required_bool(payload, "transcription_enabled"),
            diarization_enabled=_required_bool(payload, "diarization_enabled"),
            language_mode=_required_string(payload, "language_mode"),
            sample_rate=_required_int(payload, "sample_rate"),
            channel_count=_required_int(payload, "channel_count"),
            audio_encoding=_required_string(payload, "audio_encoding"),
            client_timestamp=_required_string(payload, "client_timestamp"),
            app_version=_optional_string(payload, "app_version"),
            privacy_safe_device_id=_optional_string(payload, "privacy_safe_device_id"),
        )
        config.validate()
        return config

    def validate(self) -> None:
        if self.protocol_version != PROTOCOL_VERSION:
            raise SessionConfigError(f"unsupported protocol_version: {self.protocol_version}")
        if not self.transcription_enabled:
            raise SessionConfigError("transcription_enabled must be true for a session")
        if self.language_mode not in SUPPORTED_LANGUAGE_MODES:
            raise SessionConfigError(f"unsupported language_mode: {self.language_mode}")
        if self.sample_rate != 16_000:
            raise SessionConfigError("sample_rate must be 16000")
        if self.channel_count != 1:
            raise SessionConfigError("channel_count must be 1")
        if self.audio_encoding not in SUPPORTED_AUDIO_ENCODINGS:
            raise SessionConfigError(f"unsupported audio_encoding: {self.audio_encoding}")
        _parse_timestamp(self.client_timestamp)

    @property
    def model_route(self) -> str:
        if self.language_mode == "japanese_to_english":
            return "voxtral-realtime-ja-en"
        if self.language_mode == "multilingual_to_english":
            return "voxtral-realtime-auto-source"
        return "voxtral-realtime-transcribe-en"


def decode_start_session(raw: str | bytes) -> StartSessionConfig:
    try:
        if isinstance(raw, bytes):
            raw = raw.decode("utf-8")
        payload = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise SessionConfigError("invalid start_session JSON") from exc
    if not isinstance(payload, dict):
        raise SessionConfigError("start_session JSON must be an object")
    return StartSessionConfig.from_payload(payload)


def validate_audio_frame(frame: bytes) -> None:
    if not isinstance(frame, bytes):
        raise AudioFrameError("audio frames must be binary WebSocket messages")
    if not frame:
        raise AudioFrameError("audio frame must not be empty")
    if len(frame) > MAX_AUDIO_FRAME_BYTES:
        raise AudioFrameError("audio frame exceeds 64 KiB")
    if len(frame) % 2 != 0:
        raise AudioFrameError("pcm_s16le audio frames must contain whole samples")


def encode_event(event_type: str, **metadata: Any) -> str:
    payload = {
        "type": event_type,
        "created_at": metadata.pop("created_at", utc_now()),
        **metadata,
    }
    return json.dumps(payload, sort_keys=True, separators=(",", ":"))


def transcript_event(
    event_type: str,
    *,
    segment_id: str,
    start_time_ms: int,
    end_time_ms: int | None,
    text: str,
    is_final: bool,
    speaker_id: str | None,
    language_mode: str,
    model_name: str,
    model_version: str,
    confidence: float | None = None,
) -> str:
    payload: dict[str, Any] = {
        "segment_id": segment_id,
        "start_time_ms": start_time_ms,
        "text": text,
        "is_partial": not is_final,
        "is_final": is_final,
        "language_mode": language_mode,
        "model_name": model_name,
        "model_version": model_version,
    }
    if end_time_ms is not None:
        payload["end_time_ms"] = end_time_ms
    if speaker_id:
        payload["speaker_id"] = speaker_id
    if confidence is not None:
        payload["confidence"] = confidence
    return encode_event(event_type, **payload)


def speaker_update_event(
    *,
    segment_id: str,
    speaker_id: str,
    start_time_ms: int,
    end_time_ms: int | None,
    overlap_ms: int | None = None,
    is_final: bool = True,
) -> str:
    payload: dict[str, Any] = {
        "segment_id": segment_id,
        "speaker_id": speaker_id,
        "start_time_ms": start_time_ms,
        "is_final": is_final,
    }
    if end_time_ms is not None:
        payload["end_time_ms"] = end_time_ms
    if overlap_ms is not None:
        payload["overlap_ms"] = overlap_ms
    return encode_event("speaker_update", **payload)


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def _parse_timestamp(value: str) -> None:
    try:
        datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as exc:
        raise SessionConfigError("client_timestamp must be ISO-8601") from exc


def _required_string(payload: Mapping[str, Any], key: str) -> str:
    value = payload.get(key)
    if not isinstance(value, str) or not value.strip():
        raise SessionConfigError(f"{key} must be a non-empty string")
    return value.strip()


def _optional_string(payload: Mapping[str, Any], key: str) -> str | None:
    value = payload.get(key)
    if value is None:
        return None
    if not isinstance(value, str) or not value.strip():
        raise SessionConfigError(f"{key} must be a non-empty string when provided")
    return value.strip()


def _required_bool(payload: Mapping[str, Any], key: str) -> bool:
    value = payload.get(key)
    if not isinstance(value, bool):
        raise SessionConfigError(f"{key} must be a boolean")
    return value


def _required_int(payload: Mapping[str, Any], key: str) -> int:
    value = payload.get(key)
    if not isinstance(value, int):
        raise SessionConfigError(f"{key} must be an integer")
    return value
