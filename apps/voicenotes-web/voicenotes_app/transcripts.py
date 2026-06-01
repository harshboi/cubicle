from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

from .models import Note, TranscriptSegment, utc_now


@dataclass
class TranscriptCollector:
    segments: dict[str, TranscriptSegment] = field(default_factory=dict)
    order: list[str] = field(default_factory=list)

    def add_event(self, event: dict[str, Any]) -> None:
        event_type = event.get("type")
        if event_type in {"partial_transcript", "final_transcript"}:
            segment = TranscriptSegment.from_event(event)
            if not segment.text.strip():
                return
            if segment.segment_id not in self.segments:
                self.order.append(segment.segment_id)
            if event_type == "final_transcript" or segment.segment_id not in self.segments:
                self.segments[segment.segment_id] = segment
            return
        if event_type == "speaker_update":
            segment_id = str(event.get("segment_id") or "")
            if segment_id and segment_id in self.segments and event.get("speaker_id"):
                self.segments[segment_id].speaker_id = str(event["speaker_id"])
            return
        if event_type == "translated_transcript":
            segment_id = str(event.get("segment_id") or "")
            translated = str(event.get("text") or "").strip()
            if not segment_id or not translated or segment_id not in self.segments:
                return
            segment = self.segments[segment_id]
            segment.source_text = str(event.get("source_text") or segment.source_text or segment.text)
            status = str(event.get("translation_status") or "complete")
            segment.translation_model = str(event["translation_model"]) if event.get("translation_model") else None
            segment.translation_status = status
            if status == "complete":
                segment.translated_text = translated
                segment.text = translated

    def final_segments(self) -> list[TranscriptSegment]:
        return [self.segments[segment_id] for segment_id in self.order if self.segments[segment_id].is_final]

    def all_segments(self) -> list[TranscriptSegment]:
        return [self.segments[segment_id] for segment_id in self.order]


def transcript_payload(note: Note, segments: list[TranscriptSegment]) -> dict[str, Any]:
    return {
        "note_id": note.note_id,
        "user_id": note.user_id,
        "owner_email": note.owner_email,
        "created_at": note.created_at,
        "updated_at": utc_now(),
        "language_mode": note.language_mode,
        "diarization_enabled": note.diarization_enabled,
        "segments": [segment.to_dict() for segment in segments],
        "text_intelligence": {
            "summary": note.summary,
            "action_items": note.action_items,
            "decisions": note.decisions,
            "open_questions": note.open_questions,
            "generated_title": note.generated_title,
            "summary_status": note.summary_status,
            "summary_model": note.summary_model,
            "summary_generated_at": note.summary_generated_at,
        },
    }


def render_text(segments: list[TranscriptSegment]) -> str:
    lines: list[str] = []
    last_speaker: str | None = None
    for segment in segments:
        text = " ".join(segment.text.split())
        if not text:
            continue
        speaker = segment.speaker_id
        if speaker and speaker != last_speaker:
            if lines:
                lines.append("")
            lines.append(f"{speaker}: {text}")
            last_speaker = speaker
        elif speaker:
            lines.append(text)
        else:
            lines.append(text)
    return "\n".join(lines).strip() + ("\n" if lines else "")


def render_markdown(note: Note, segments: list[TranscriptSegment]) -> str:
    title = note.title.strip() or "Voice note"
    body = render_text(segments).strip()
    return f"# {title}\n\n{body}\n" if body else f"# {title}\n"


def title_from_segments(segments: list[TranscriptSegment]) -> str:
    for segment in segments:
        text = " ".join(segment.text.split())
        if text:
            title = text[:72].strip()
            return title.rstrip(".,;:") or "Voice note"
    return "Voice note"


def transcript_stats(segments: list[TranscriptSegment]) -> tuple[int, int, int]:
    speaker_ids = {segment.speaker_id for segment in segments if segment.speaker_id}
    word_count = sum(len(segment.text.split()) for segment in segments)
    return len(segments), word_count, len(speaker_ids)
