from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Protocol

import httpx

from .settings import Settings


class TextIntelligenceError(RuntimeError):
    pass


@dataclass(frozen=True)
class TranslationResult:
    segment_id: str
    source_text: str
    text: str
    translation_model: str
    context_line_count: int
    latency_ms: int | None = None


@dataclass(frozen=True)
class TranscriptSummaryResult:
    summary: str | None = None
    action_items: list[dict[str, Any]] = field(default_factory=list)
    decisions: list[str] = field(default_factory=list)
    open_questions: list[str] = field(default_factory=list)
    generated_title: str | None = None
    model: str | None = None


class TextIntelligenceClient(Protocol):
    async def translate_line(
        self,
        *,
        note_id: str,
        segment_id: str,
        previous_lines: list[str],
        target_line: str,
    ) -> TranslationResult:
        ...

    async def summarize_transcript(
        self,
        *,
        note_id: str,
        lines: list[str],
    ) -> TranscriptSummaryResult:
        ...


@dataclass
class HTTPTextIntelligenceClient:
    base_url: str
    auth_token: str = ""
    model: str = "Qwen/Qwen2.5-7B-Instruct"
    request_timeout_seconds: int = 12
    summary_timeout_seconds: int = 45

    async def translate_line(
        self,
        *,
        note_id: str,
        segment_id: str,
        previous_lines: list[str],
        target_line: str,
    ) -> TranslationResult:
        payload = {
            "note_id": note_id,
            "segment_id": segment_id,
            "previous_lines": previous_lines,
            "target_line": target_line,
            "target_language": "en",
            "model": self.model,
        }
        response = await self._post_json("/v1/translate-line", payload, timeout=self.request_timeout_seconds)
        translated = _required_text(response, "text")
        return TranslationResult(
            segment_id=_required_text(response, "segment_id") or segment_id,
            source_text=_required_text(response, "source_text") or target_line,
            text=translated,
            translation_model=_required_text(response, "translation_model") or self.model,
            context_line_count=_optional_int(response.get("context_line_count"), default=len(previous_lines)),
            latency_ms=_optional_int(response.get("latency_ms"), default=None),
        )

    async def summarize_transcript(
        self,
        *,
        note_id: str,
        lines: list[str],
    ) -> TranscriptSummaryResult:
        response = await self._post_json(
            "/v1/summarize-transcript",
            {"note_id": note_id, "lines": lines, "model": self.model},
            timeout=self.summary_timeout_seconds,
        )
        return TranscriptSummaryResult(
            summary=_optional_text(response.get("summary")),
            action_items=_object_list(response.get("action_items")),
            decisions=_string_list(response.get("decisions")),
            open_questions=_string_list(response.get("open_questions")),
            generated_title=_optional_text(response.get("generated_title")),
            model=_optional_text(response.get("model")),
        )

    async def _post_json(self, path: str, payload: dict[str, Any], *, timeout: int) -> dict[str, Any]:
        headers = {"Content-Type": "application/json"}
        if self.auth_token.strip():
            headers["Authorization"] = f"Bearer {self.auth_token.strip()}"
        url = f"{self.base_url.rstrip('/')}{path}"
        try:
            async with httpx.AsyncClient(timeout=timeout) as client:
                response = await client.post(url, json=payload, headers=headers)
                response.raise_for_status()
                decoded = response.json()
        except Exception as exc:
            raise TextIntelligenceError(f"text intelligence request failed: {type(exc).__name__}") from exc
        if not isinstance(decoded, dict):
            raise TextIntelligenceError("text intelligence response must be a JSON object")
        return decoded


def create_text_intelligence_client(settings: Settings) -> TextIntelligenceClient | None:
    if not settings.text_intelligence_enabled:
        return None
    if not settings.text_intelligence_url.strip():
        return None
    return HTTPTextIntelligenceClient(
        base_url=settings.text_intelligence_url,
        auth_token=settings.text_intelligence_token,
        model=settings.text_intelligence_model,
        request_timeout_seconds=settings.text_intelligence_request_timeout_seconds,
        summary_timeout_seconds=settings.text_intelligence_summary_timeout_seconds,
    )


def _required_text(payload: dict[str, Any], key: str) -> str:
    value = payload.get(key)
    return value.strip() if isinstance(value, str) else ""


def _optional_text(value: Any) -> str | None:
    return value.strip() if isinstance(value, str) and value.strip() else None


def _optional_int(value: Any, *, default: int | None) -> int | None:
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def _string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [item.strip() for item in value if isinstance(item, str) and item.strip()]


def _object_list(value: Any) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, dict)]
