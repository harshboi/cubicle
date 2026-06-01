from __future__ import annotations

import asyncio
import logging
import os
from typing import Any

try:
    from fastapi import FastAPI, HTTPException, Request
except ImportError:
    FastAPI = None  # type: ignore[assignment]
    HTTPException = None  # type: ignore[assignment]
    Request = None  # type: ignore[assignment]

from .text_intelligence import (
    TextIntelligenceProviderError,
    TextIntelligenceProviderFactory,
    TextIntelligenceSettings,
    summarize_transcript,
    translate_line,
)


logger = logging.getLogger("transcription_service.text_intelligence_worker")


def worker_readiness_payload(settings: TextIntelligenceSettings | None = None) -> dict[str, Any]:
    worker_settings = settings or TextIntelligenceSettings.from_environment()
    return {
        "status": "ok",
        "service": "cubicle-transcription-text-intelligence-worker",
        "provider": worker_settings.provider,
        "model": worker_settings.model,
        "retention": "disabled",
    }


def create_app(
    *,
    settings: TextIntelligenceSettings | None = None,
    provider_factory: TextIntelligenceProviderFactory | None = None,
    auth_token: str | None = None,
):
    if FastAPI is None or HTTPException is None or Request is None:
        raise RuntimeError("FastAPI runtime dependencies are not installed.")

    worker_settings = settings or TextIntelligenceSettings.from_environment()
    factory = provider_factory or TextIntelligenceProviderFactory(settings=worker_settings)
    expected_auth_token = auth_token if auth_token is not None else _env_text("TEXT_INTELLIGENCE_WORKER_AUTH_TOKEN")
    warmup_enabled = _env_bool("TEXT_INTELLIGENCE_WARMUP_ENABLED", True)
    app = FastAPI(title="Cubicle Text Intelligence Worker", version="0.1.0")

    @app.on_event("startup")
    async def warm_text_intelligence_provider() -> None:
        if not warmup_enabled:
            return

        async def warm() -> None:
            try:
                await asyncio.to_thread(factory.create_provider().warmup)
            except Exception as exc:
                logger.warning(
                    "text intelligence worker warmup failed",
                    extra={
                        "provider": worker_settings.provider,
                        "model": worker_settings.model,
                        "error_class": type(exc).__name__,
                    },
                )
                return
            logger.info(
                "text intelligence worker warmup completed",
                extra={"provider": worker_settings.provider, "model": worker_settings.model},
            )

        app.state.text_intelligence_warmup_task = asyncio.create_task(warm())

    @app.get("/healthz")
    async def healthz():
        return worker_readiness_payload(worker_settings)

    @app.get("/runtimez")
    async def runtimez():
        return factory.runtime_status()

    @app.post("/v1/translate-line")
    async def translate_line_endpoint(request: Request):
        _validate_authorization(request, expected_auth_token)
        payload = await _request_json(request)
        segment_id = _required_text(payload, "segment_id")
        target_line = _required_text(payload, "target_line")
        target_language = _optional_text(payload.get("target_language")) or "en"
        previous_lines = _text_list(payload.get("previous_lines"))
        try:
            return await asyncio.to_thread(
                translate_line,
                factory.create_provider(),
                segment_id=segment_id,
                previous_lines=previous_lines[-8:],
                target_line=target_line,
                target_language=target_language,
            )
        except TextIntelligenceProviderError as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc

    @app.post("/v1/summarize-transcript")
    async def summarize_transcript_endpoint(request: Request):
        _validate_authorization(request, expected_auth_token)
        payload = await _request_json(request)
        lines = _text_list(payload.get("lines"))
        if not lines:
            raise HTTPException(status_code=400, detail="lines are required")
        try:
            return await asyncio.to_thread(summarize_transcript, factory.create_provider(), lines=lines)
        except TextIntelligenceProviderError as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc

    @app.post("/v1/extract-action-items")
    async def extract_action_items_endpoint(request: Request):
        summary = await summarize_transcript_endpoint(request)
        return {"action_items": summary.get("action_items", []), "model": summary.get("model")}

    @app.post("/v1/generate-title")
    async def generate_title_endpoint(request: Request):
        summary = await summarize_transcript_endpoint(request)
        return {"generated_title": summary.get("generated_title"), "model": summary.get("model")}

    @app.post("/v1/ask-transcript")
    async def ask_transcript_endpoint(request: Request):
        _validate_authorization(request, expected_auth_token)
        payload = await _request_json(request)
        lines = _text_list(payload.get("lines"))
        question = _required_text(payload, "question")
        try:
            return await asyncio.to_thread(
                factory.create_provider().answer_transcript_question,
                lines=lines,
                question=question,
            )
        except TextIntelligenceProviderError as exc:
            raise HTTPException(status_code=503, detail=str(exc)) from exc

    return app


async def _request_json(request: Request) -> dict[str, Any]:
    try:
        payload = await request.json()
    except Exception as exc:
        raise HTTPException(status_code=400, detail="invalid JSON") from exc
    if not isinstance(payload, dict):
        raise HTTPException(status_code=400, detail="request body must be a JSON object")
    return payload


def _validate_authorization(request: Request, expected_auth_token: str | None) -> None:
    if not expected_auth_token:
        return
    authorization = request.headers.get("authorization", "")
    expected = f"Bearer {expected_auth_token}"
    if authorization != expected:
        raise HTTPException(status_code=401, detail="unauthorized")


def _required_text(payload: dict[str, Any], key: str) -> str:
    value = payload.get(key)
    if not isinstance(value, str) or not value.strip():
        raise HTTPException(status_code=400, detail=f"{key} is required")
    return value.strip()


def _optional_text(value: Any) -> str | None:
    return value.strip() if isinstance(value, str) and value.strip() else None


def _text_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [item.strip() for item in value if isinstance(item, str) and item.strip()]


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
