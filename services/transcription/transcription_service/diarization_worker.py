"""FastAPI worker that exposes diarization over an internal HTTP boundary."""

from __future__ import annotations

import asyncio
import base64
import binascii
from dataclasses import replace
import logging
import os
from typing import Any

try:
    from fastapi import FastAPI, HTTPException, Request
except ImportError:
    FastAPI = None  # type: ignore[assignment]
    HTTPException = None  # type: ignore[assignment]
    Request = None  # type: ignore[assignment]

from .diarization import (
    DiarizationProviderError,
    DiarizationProviderFactory,
    DiarizationProviderSettings,
)
from .protocol import SessionConfigError, StartSessionConfig


logger = logging.getLogger("transcription_service.diarization_worker")


def worker_settings_from_environment() -> DiarizationProviderSettings:
    settings = DiarizationProviderSettings.from_environment()
    provider = os.environ.get("TRANSCRIPTION_WORKER_DIARIZATION_PROVIDER", "pyannote").strip().lower()
    return replace(settings, provider=provider)


def worker_readiness_payload(settings: DiarizationProviderSettings | None = None) -> dict[str, Any]:
    worker_settings = settings or worker_settings_from_environment()
    return {
        "status": "ok",
        "service": "cubicle-transcription-diarization-worker",
        "diarization_provider": worker_settings.provider,
        "model_name": worker_settings.model_name,
        "model_version": worker_settings.model_version,
        "device": worker_settings.device,
        "retention": "disabled",
    }


def create_app(
    *,
    settings: DiarizationProviderSettings | None = None,
    provider_factory: DiarizationProviderFactory | None = None,
    auth_token: str | None = None,
):
    if FastAPI is None or HTTPException is None or Request is None:
        raise RuntimeError("FastAPI runtime dependencies are not installed.")

    worker_settings = settings or worker_settings_from_environment()
    factory = provider_factory or DiarizationProviderFactory(settings=worker_settings)
    expected_auth_token = auth_token if auth_token is not None else _env_text("TRANSCRIPTION_DIARIZATION_WORKER_AUTH_TOKEN")
    warmup_enabled = _env_bool("TRANSCRIPTION_DIARIZATION_WARMUP_ENABLED", True)
    app = FastAPI(title="Cubicle Diarization Worker", version="0.1.0")

    @app.on_event("startup")
    async def warm_diarization_provider() -> None:
        if not warmup_enabled or worker_settings.provider != "pyannote":
            return

        async def warm() -> None:
            try:
                await asyncio.to_thread(factory.create_provider().warmup)
            except Exception as exc:
                logger.warning(
                    "diarization worker warmup failed",
                    extra={
                        "provider": worker_settings.provider,
                        "model_name": worker_settings.model_name,
                        "device": worker_settings.device,
                        "error_class": type(exc).__name__,
                    },
                )
                return
            logger.info(
                "diarization worker warmup completed",
                extra={
                    "provider": worker_settings.provider,
                    "model_name": worker_settings.model_name,
                    "device": worker_settings.device,
                },
            )

        app.state.diarization_warmup_task = asyncio.create_task(warm())

    @app.get("/healthz")
    async def healthz():
        return worker_readiness_payload(worker_settings)

    @app.get("/runtimez")
    async def runtimez():
        return factory.runtime_status()

    @app.post("/v1/diarization")
    async def diarize(request: Request):
        _validate_authorization(request, expected_auth_token)
        try:
            payload = await request.json()
            config = _config_from_payload(payload)
            pcm_s16le = _audio_from_payload(payload)
        except (SessionConfigError, ValueError) as exc:
            raise HTTPException(status_code=400, detail=str(exc)) from exc

        try:
            provider = factory.create_provider()
            turns = await asyncio.to_thread(provider.diarize, config, pcm_s16le)
            status = provider.runtime_status()
        except DiarizationProviderError as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc

        return {
            "session_id": config.session_id,
            "provider": status.get("provider"),
            "model_name": status.get("model_name"),
            "model_version": status.get("model_version"),
            "speaker_turns": [
                {
                    "speaker_id": turn.speaker_id,
                    "start_time_ms": turn.start_time_ms,
                    "end_time_ms": turn.end_time_ms,
                    **({"confidence": turn.confidence} if turn.confidence is not None else {}),
                }
                for turn in turns
            ],
            "retention": "disabled",
        }

    return app


def _config_from_payload(payload: Any) -> StartSessionConfig:
    if not isinstance(payload, dict):
        raise ValueError("diarization job must be a JSON object")
    return StartSessionConfig.from_payload(
        {
            "type": "start_session",
            "protocol_version": payload.get("protocol_version"),
            "session_id": payload.get("session_id"),
            "transcription_enabled": True,
            "diarization_enabled": payload.get("diarization_enabled"),
            "language_mode": payload.get("language_mode"),
            "sample_rate": payload.get("sample_rate"),
            "channel_count": payload.get("channel_count"),
            "audio_encoding": payload.get("audio_encoding"),
            "client_timestamp": payload.get("client_timestamp"),
            "app_version": payload.get("app_version"),
            "privacy_safe_device_id": payload.get("privacy_safe_device_id"),
        }
    )


def _audio_from_payload(payload: Any) -> bytes:
    if not isinstance(payload, dict):
        raise ValueError("diarization job must be a JSON object")
    audio_b64 = payload.get("audio_b64")
    if not isinstance(audio_b64, str) or not audio_b64.strip():
        raise ValueError("audio_b64 is required")
    try:
        pcm_s16le = base64.b64decode(audio_b64, validate=True)
    except (binascii.Error, ValueError) as exc:
        raise ValueError("audio_b64 must be valid base64") from exc
    if not pcm_s16le:
        raise ValueError("audio must not be empty")
    if len(pcm_s16le) % 2 != 0:
        raise ValueError("pcm_s16le audio must contain whole samples")
    return pcm_s16le


def _validate_authorization(request: Request, expected_auth_token: str | None) -> None:
    if not expected_auth_token:
        return
    authorization = request.headers.get("authorization", "")
    expected = f"Bearer {expected_auth_token}"
    if authorization != expected:
        raise HTTPException(status_code=401, detail="unauthorized")


def _env_text(name: str) -> str | None:
    value = os.environ.get(name, "").strip()
    return value or None


def _env_bool(name: str, default: bool) -> bool:
    value = os.environ.get(name, "").strip().lower()
    if not value:
        return default
    return value in {"1", "true", "yes", "on"}


try:
    app = create_app()
except RuntimeError:
    app = None
