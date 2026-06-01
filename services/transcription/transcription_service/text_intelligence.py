"""OpenAI-compatible translation and transcript-summary provider adapters."""

from __future__ import annotations

from dataclasses import dataclass, field
import json
import os
import re
import time
from typing import Any, Protocol
from urllib import error as urllib_error
from urllib import request as urllib_request


class TextIntelligenceProviderError(RuntimeError):
    """Translation or transcript-summary provider is unavailable or invalid."""

    pass


class TextIntelligenceProvider(Protocol):
    """Provider boundary for post-transcription language intelligence."""

    def warmup(self) -> None:
        """Fail fast when required runtime dependencies are not ready."""

        ...

    def translate_line(
        self,
        *,
        previous_lines: list[str],
        target_line: str,
        target_language: str,
    ) -> dict[str, Any]:
        """Translate one transcript line using nearby context only."""

        ...

    def summarize_transcript(self, *, lines: list[str]) -> dict[str, Any]:
        """Return summary JSON fields consumed by VoiceNotes."""

        ...

    def answer_transcript_question(self, *, lines: list[str], question: str) -> dict[str, Any]:
        """Answer from transcript text without external retrieval."""

        ...

    def runtime_status(self) -> dict[str, Any]:
        """Return provider readiness metadata safe for status endpoints."""

        ...


@dataclass(frozen=True)
class TextIntelligenceSettings:
    """Environment-backed settings for the text-intelligence provider."""

    provider: str = "mock"
    model: str = "Qwen/Qwen2.5-7B-Instruct"
    vllm_base_url: str = "http://127.0.0.1:8000"
    request_timeout_seconds: float = 20.0
    summary_timeout_seconds: float = 60.0
    max_translation_tokens: int = 160
    max_summary_tokens: int = 1200
    temperature: float = 0.0

    @classmethod
    def from_environment(cls) -> "TextIntelligenceSettings":
        """Load text-intelligence settings from process environment variables."""

        return cls(
            provider=os.environ.get("TEXT_INTELLIGENCE_PROVIDER", "mock").strip().lower(),
            model=os.environ.get("TEXT_INTELLIGENCE_MODEL", cls.model).strip() or cls.model,
            vllm_base_url=os.environ.get("TEXT_INTELLIGENCE_VLLM_BASE_URL", cls.vllm_base_url).strip()
            or cls.vllm_base_url,
            request_timeout_seconds=_env_float(
                "TEXT_INTELLIGENCE_REQUEST_TIMEOUT_SECONDS",
                cls.request_timeout_seconds,
            ),
            summary_timeout_seconds=_env_float(
                "TEXT_INTELLIGENCE_SUMMARY_TIMEOUT_SECONDS",
                cls.summary_timeout_seconds,
            ),
            max_translation_tokens=_env_int("TEXT_INTELLIGENCE_MAX_TRANSLATION_TOKENS", cls.max_translation_tokens),
            max_summary_tokens=_env_int("TEXT_INTELLIGENCE_MAX_SUMMARY_TOKENS", cls.max_summary_tokens),
            temperature=_env_float("TEXT_INTELLIGENCE_TEMPERATURE", cls.temperature),
        )


@dataclass
class TextIntelligenceProviderFactory:
    """Creates the configured translation/summary provider."""

    settings: TextIntelligenceSettings = field(default_factory=TextIntelligenceSettings.from_environment)

    def create_provider(self) -> TextIntelligenceProvider:
        """Instantiate the provider selected by settings."""

        if self.settings.provider == "mock":
            return MockTextIntelligenceProvider(settings=self.settings)
        if self.settings.provider in {"vllm", "openai_compatible"}:
            return OpenAICompatibleTextIntelligenceProvider(settings=self.settings)
        raise TextIntelligenceProviderError(f"unsupported text intelligence provider: {self.settings.provider}")

    def runtime_status(self) -> dict[str, Any]:
        """Return provider status without throwing on bad config."""

        try:
            return self.create_provider().runtime_status()
        except TextIntelligenceProviderError as exc:
            return {"provider": self.settings.provider, "ready": False, "error": str(exc)}


@dataclass
class MockTextIntelligenceProvider:
    """Deterministic no-network provider for local development and tests."""

    settings: TextIntelligenceSettings = field(default_factory=TextIntelligenceSettings.from_environment)

    def warmup(self) -> None:
        """Mock provider has no runtime dependency to initialize."""

        return None

    def translate_line(
        self,
        *,
        previous_lines: list[str],
        target_line: str,
        target_language: str,
    ) -> dict[str, Any]:
        """Echo the target line while preserving response shape."""

        return {
            "text": target_line.strip(),
            "model": self.settings.model,
            "context_line_count": len(previous_lines),
        }

    def summarize_transcript(self, *, lines: list[str]) -> dict[str, Any]:
        """Produce stable placeholder summary fields from transcript text."""

        first_line = lines[0].strip() if lines else ""
        return {
            "summary": first_line[:180] if first_line else "",
            "action_items": [],
            "decisions": [],
            "open_questions": [],
            "generated_title": first_line[:72] if first_line else "",
            "model": self.settings.model,
        }

    def answer_transcript_question(self, *, lines: list[str], question: str) -> dict[str, Any]:
        """Return the service's safe unknown-answer sentinel."""

        return {"answer": "Not found in transcript.", "model": self.settings.model}

    def runtime_status(self) -> dict[str, Any]:
        """Report mock readiness without touching external services."""

        return {
            "provider": "mock",
            "model": self.settings.model,
            "ready": True,
            "retention": "disabled",
        }


@dataclass
class OpenAICompatibleTextIntelligenceProvider:
    """OpenAI-compatible chat-completions adapter for vLLM-style runtimes."""

    settings: TextIntelligenceSettings = field(default_factory=TextIntelligenceSettings.from_environment)

    def warmup(self) -> None:
        """Probe runtime health through the status path."""

        self.runtime_status()

    def translate_line(
        self,
        *,
        previous_lines: list[str],
        target_line: str,
        target_language: str,
    ) -> dict[str, Any]:
        """Request one strict line translation from the configured model."""

        content = self._chat_completion(
            system=(
                "You translate transcript lines to English. Use context only to resolve meaning. "
                "Return only the English translation of TARGET. If TARGET is already English, return it unchanged. "
                "Do not add labels, explanations, quotation marks, or phrases like 'Here is the translation'."
            ),
            user=_translation_prompt(previous_lines=previous_lines, target_line=target_line),
            max_tokens=self.settings.max_translation_tokens,
            timeout=self.settings.request_timeout_seconds,
        )
        return {
            "text": _translation_line(content),
            "model": self.settings.model,
            "context_line_count": len(previous_lines),
        }

    def summarize_transcript(self, *, lines: list[str]) -> dict[str, Any]:
        """Request strict JSON summary fields from the configured model."""

        content = self._chat_completion(
            system=(
                "You summarize transcripts. Return strict JSON with keys: summary, action_items, "
                "decisions, open_questions, generated_title. Do not invent facts."
            ),
            user=_summary_prompt(lines),
            max_tokens=self.settings.max_summary_tokens,
            timeout=self.settings.summary_timeout_seconds,
        )
        decoded = _json_object_from_text(content)
        return {
            "summary": _optional_text(decoded.get("summary")),
            "action_items": _object_list(decoded.get("action_items")),
            "decisions": _string_list(decoded.get("decisions")),
            "open_questions": _string_list(decoded.get("open_questions")),
            "generated_title": _optional_text(decoded.get("generated_title")),
            "model": self.settings.model,
        }

    def answer_transcript_question(self, *, lines: list[str], question: str) -> dict[str, Any]:
        """Ask the model to answer only from provided transcript lines."""

        content = self._chat_completion(
            system=(
                "Answer only from the transcript. If the answer is not present, say exactly: "
                "Not found in transcript. Do not infer missing facts."
            ),
            user=_question_prompt(lines=lines, question=question),
            max_tokens=400,
            timeout=self.settings.request_timeout_seconds,
        )
        return {"answer": content.strip() or "Not found in transcript.", "model": self.settings.model}

    def runtime_status(self) -> dict[str, Any]:
        """Probe health and model listing endpoints with short timeouts."""

        endpoint = self.settings.vllm_base_url.rstrip("/")
        ready = False
        error: str | None = None
        served_models: list[str] = []
        try:
            with urllib_request.urlopen(f"{endpoint}/health", timeout=min(5.0, self.settings.request_timeout_seconds)) as response:
                ready = 200 <= int(response.status) < 300
        except (urllib_error.URLError, TimeoutError, OSError) as exc:
            error = type(exc).__name__
        if ready:
            try:
                with urllib_request.urlopen(f"{endpoint}/v1/models", timeout=min(5.0, self.settings.request_timeout_seconds)) as response:
                    decoded = json.loads(response.read().decode("utf-8"))
                served_models = [
                    item["id"]
                    for item in decoded.get("data", [])
                    if isinstance(item, dict) and isinstance(item.get("id"), str)
                ]
            except (urllib_error.URLError, TimeoutError, OSError, json.JSONDecodeError, UnicodeDecodeError):
                served_models = []
        return {
            "provider": "vllm",
            "model": self.settings.model,
            "vllm_base_url_configured": bool(endpoint),
            "ready": ready,
            "model_loaded": self.settings.model in served_models if served_models else None,
            "served_models": served_models,
            "error": error,
            "retention": "disabled",
        }

    def _chat_completion(self, *, system: str, user: str, max_tokens: int, timeout: float) -> str:
        endpoint = f"{self.settings.vllm_base_url.rstrip('/')}/v1/chat/completions"
        payload = {
            "model": self.settings.model,
            "messages": [
                {"role": "system", "content": system},
                {"role": "user", "content": user},
            ],
            "temperature": self.settings.temperature,
            "max_tokens": max_tokens,
        }
        request = urllib_request.Request(
            endpoint,
            data=json.dumps(payload, separators=(",", ":"), sort_keys=True).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib_request.urlopen(request, timeout=timeout) as response:
                decoded = json.loads(response.read().decode("utf-8"))
        except (urllib_error.URLError, TimeoutError, json.JSONDecodeError, UnicodeDecodeError) as exc:
            raise TextIntelligenceProviderError(f"vLLM chat completion failed: {type(exc).__name__}") from exc
        try:
            content = decoded["choices"][0]["message"]["content"]
        except (KeyError, IndexError, TypeError) as exc:
            raise TextIntelligenceProviderError("vLLM response did not include message content") from exc
        if not isinstance(content, str):
            raise TextIntelligenceProviderError("vLLM message content must be text")
        return content


def translate_line(
    provider: TextIntelligenceProvider,
    *,
    segment_id: str,
    previous_lines: list[str],
    target_line: str,
    target_language: str,
) -> dict[str, Any]:
    started = time.monotonic()
    result = provider.translate_line(
        previous_lines=previous_lines,
        target_line=target_line,
        target_language=target_language,
    )
    translated = _optional_text(result.get("text")) or target_line.strip()
    return {
        "segment_id": segment_id,
        "source_text": target_line,
        "text": translated,
        "translation_model": _optional_text(result.get("model")) or "unknown",
        "context_line_count": _optional_int(result.get("context_line_count"), len(previous_lines)),
        "latency_ms": max(0, int((time.monotonic() - started) * 1000)),
    }


def summarize_transcript(provider: TextIntelligenceProvider, *, lines: list[str]) -> dict[str, Any]:
    result = provider.summarize_transcript(lines=lines)
    return {
        "summary": _optional_text(result.get("summary")),
        "action_items": _object_list(result.get("action_items")),
        "decisions": _string_list(result.get("decisions")),
        "open_questions": _string_list(result.get("open_questions")),
        "generated_title": _optional_text(result.get("generated_title")),
        "model": _optional_text(result.get("model")) or "unknown",
    }


def _translation_prompt(*, previous_lines: list[str], target_line: str) -> str:
    context = "\n".join(f"{index + 1}. {line}" for index, line in enumerate(previous_lines))
    return (
        "Translate TARGET to English.\n"
        "Use CONTEXT only to understand pronouns, names, references, and mixed-language fragments.\n"
        "Return only the English translation of TARGET.\n"
        "If TARGET is already English, return it unchanged.\n"
        "Do not include a preface, label, explanation, or quotation marks.\n\n"
        f"CONTEXT:\n{context or '(none)'}\n\n"
        f"TARGET:\n{target_line}"
    )


def _summary_prompt(lines: list[str]) -> str:
    transcript = "\n".join(f"{index + 1}. {line}" for index, line in enumerate(lines))
    return (
        "Summarize this transcript. Return only JSON.\n"
        "action_items must be an array of objects with task, owner, and due_date keys.\n"
        "Use null when owner or due_date is not present.\n\n"
        f"TRANSCRIPT:\n{transcript}"
    )


def _question_prompt(*, lines: list[str], question: str) -> str:
    transcript = "\n".join(f"{index + 1}. {line}" for index, line in enumerate(lines))
    return f"TRANSCRIPT:\n{transcript}\n\nQUESTION:\n{question}"


def _single_line(value: str) -> str:
    return " ".join(value.strip().split())


_TRANSLATION_PREFIX_RE = re.compile(
    r"^(?:"
    r"here(?:'s| is)\s+(?:the\s+)?(?:english\s+)?translation"
    r"(?:\s+of\s+(?:the\s+)?(?:target\s+)?(?:text|line))?"
    r"|the\s+english\s+translation\s+is"
    r"|english\s+translation"
    r"|translation"
    r"|target\s+translation"
    r")\s*[:\-]\s*",
    re.IGNORECASE,
)


def _translation_line(value: str) -> str:
    text = _strip_outer_quotes(_single_line(value))
    for _ in range(3):
        stripped = _TRANSLATION_PREFIX_RE.sub("", text).strip()
        stripped = _strip_outer_quotes(stripped)
        if stripped == text:
            break
        text = stripped
    return text


def _strip_outer_quotes(value: str) -> str:
    text = value.strip()
    if len(text) >= 2 and text[0] == text[-1] and text[0] in {'"', "'"}:
        return text[1:-1].strip()
    return text


def _json_object_from_text(value: str) -> dict[str, Any]:
    text = value.strip()
    if text.startswith("```"):
        text = text.strip("`")
        if text.lower().startswith("json"):
            text = text[4:].strip()
    try:
        decoded = json.loads(text)
    except json.JSONDecodeError:
        return {}
    return decoded if isinstance(decoded, dict) else {}


def _optional_text(value: Any) -> str | None:
    return value.strip() if isinstance(value, str) and value.strip() else None


def _string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [item.strip() for item in value if isinstance(item, str) and item.strip()]


def _object_list(value: Any) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        return []
    return [item for item in value if isinstance(item, dict)]


def _optional_int(value: Any, default: int) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def _env_int(name: str, default: int) -> int:
    value = os.environ.get(name)
    if not value:
        return default
    return int(value)


def _env_float(name: str, default: float) -> float:
    value = os.environ.get(name)
    if not value:
        return default
    return float(value)
