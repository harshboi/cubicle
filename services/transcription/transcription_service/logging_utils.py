"""Structured logging helpers that redact sensitive audio/text fields."""

from __future__ import annotations

import json
import logging
from typing import Any


SENSITIVE_KEYS = {
    "audio",
    "audio_bytes",
    "chunk",
    "raw_audio",
    "text",
    "transcript",
    "word_timestamps",
    "words",
    "speaker_embedding",
    "speaker_embeddings",
    "voiceprint",
    "voiceprints",
    "authorization",
    "auth_token",
    "token",
}


def safe_log_payload(**fields: Any) -> str:
    redacted: dict[str, Any] = {}
    for key, value in fields.items():
        if key.lower() in SENSITIVE_KEYS:
            redacted[key] = "[redacted]"
        else:
            redacted[key] = value
    return json.dumps(redacted, sort_keys=True, separators=(",", ":"))


def log_event(logger: logging.Logger, event: str, **fields: Any) -> None:
    logger.info(safe_log_payload(event=event, **fields))
