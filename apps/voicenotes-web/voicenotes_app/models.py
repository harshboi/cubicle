"""VoiceNotes domain models and JSON conversion helpers."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any
from uuid import uuid4


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def new_id(prefix: str) -> str:
    return f"{prefix}_{uuid4().hex}"


@dataclass(frozen=True)
class User:
    """Authenticated VoiceNotes user identity."""

    user_id: str
    email: str
    display_name: str
    role: str = "user"
    auth_subject: str | None = None

    @classmethod
    def from_email(
        cls,
        email: str,
        display_name: str | None = None,
        role: str = "user",
        auth_subject: str | None = None,
    ) -> "User":
        normalized = email.strip().lower()
        safe_id = normalized.replace("@", "_at_").replace(".", "_")
        return cls(
            user_id=safe_id,
            email=normalized,
            display_name=display_name or normalized,
            role=role,
            auth_subject=auth_subject,
        )


@dataclass
class TranscriptSegment:
    """Transcript segment persisted inside a note export."""

    segment_id: str
    text: str
    start_time_ms: int | None = None
    end_time_ms: int | None = None
    speaker_id: str | None = None
    is_final: bool = True
    source_text: str | None = None
    translated_text: str | None = None
    translation_model: str | None = None
    translation_status: str = "not_requested"

    @classmethod
    def from_event(cls, event: dict[str, Any]) -> "TranscriptSegment":
        return cls(
            segment_id=str(event.get("segment_id") or new_id("seg")),
            text=str(event.get("text") or ""),
            start_time_ms=_optional_int(event.get("start_time_ms")),
            end_time_ms=_optional_int(event.get("end_time_ms")),
            speaker_id=str(event["speaker_id"]) if event.get("speaker_id") else None,
            is_final=bool(event.get("is_final", event.get("type") == "final_transcript")),
            source_text=str(event.get("source_text") or event.get("text") or "") or None,
            translated_text=str(event["translated_text"]) if event.get("translated_text") else None,
            translation_model=str(event["translation_model"]) if event.get("translation_model") else None,
            translation_status=str(event.get("translation_status") or "not_requested"),
        )

    def to_dict(self) -> dict[str, Any]:
        return {
            "segment_id": self.segment_id,
            "text": self.text,
            "start_time_ms": self.start_time_ms,
            "end_time_ms": self.end_time_ms,
            "speaker_id": self.speaker_id,
            "is_final": self.is_final,
            "source_text": self.source_text,
            "translated_text": self.translated_text,
            "translation_model": self.translation_model,
            "translation_status": self.translation_status,
        }


@dataclass
class Note:
    """Voice note metadata plus transcript intelligence fields."""

    note_id: str
    user_id: str
    owner_email: str
    title: str
    status: str = "recording"
    created_at: str = field(default_factory=utc_now)
    updated_at: str = field(default_factory=utc_now)
    started_at: str | None = None
    stopped_at: str | None = None
    duration_ms: int = 0
    language_mode: str = "english_to_english"
    diarization_enabled: bool = False
    speaker_count: int = 0
    segment_count: int = 0
    word_count: int = 0
    transcript_s3_key: str | None = None
    transcript_text_s3_key: str | None = None
    transcript_md_s3_key: str | None = None
    deleted_at: str | None = None
    summary: str | None = None
    action_items: list[dict[str, Any]] = field(default_factory=list)
    decisions: list[str] = field(default_factory=list)
    open_questions: list[str] = field(default_factory=list)
    generated_title: str | None = None
    summary_status: str = "not_started"
    summary_model: str | None = None
    summary_generated_at: str | None = None

    @classmethod
    def create(
        cls,
        *,
        user: User,
        language_mode: str,
        diarization_enabled: bool,
        note_id: str | None = None,
    ) -> "Note":
        now = utc_now()
        return cls(
            note_id=note_id or new_id("note"),
            user_id=user.user_id,
            owner_email=user.email,
            title="Untitled voice note",
            status="recording",
            created_at=now,
            updated_at=now,
            started_at=now,
            language_mode=language_mode,
            diarization_enabled=diarization_enabled,
        )

    def to_dict(self) -> dict[str, Any]:
        return {
            "note_id": self.note_id,
            "user_id": self.user_id,
            "owner_email": self.owner_email,
            "title": self.title,
            "status": self.status,
            "created_at": self.created_at,
            "updated_at": self.updated_at,
            "started_at": self.started_at,
            "stopped_at": self.stopped_at,
            "duration_ms": self.duration_ms,
            "language_mode": self.language_mode,
            "diarization_enabled": self.diarization_enabled,
            "speaker_count": self.speaker_count,
            "segment_count": self.segment_count,
            "word_count": self.word_count,
            "transcript_s3_key": self.transcript_s3_key,
            "transcript_text_s3_key": self.transcript_text_s3_key,
            "transcript_md_s3_key": self.transcript_md_s3_key,
            "deleted_at": self.deleted_at,
            "summary": self.summary,
            "action_items": self.action_items,
            "decisions": self.decisions,
            "open_questions": self.open_questions,
            "generated_title": self.generated_title,
            "summary_status": self.summary_status,
            "summary_model": self.summary_model,
            "summary_generated_at": self.summary_generated_at,
        }

    @classmethod
    def from_dict(cls, payload: dict[str, Any]) -> "Note":
        fields = cls.__dataclass_fields__
        return cls(**{key: value for key, value in payload.items() if key in fields})


def _optional_int(value: Any) -> int | None:
    if value is None:
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None
