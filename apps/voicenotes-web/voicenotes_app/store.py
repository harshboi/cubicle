"""Local and AWS note stores for VoiceNotes transcripts."""

from __future__ import annotations

from abc import ABC, abstractmethod
from datetime import datetime, timezone
import json
from pathlib import Path
import shutil
from typing import Any

from .models import Note, TranscriptSegment, User, utc_now
from .settings import Settings
from .transcripts import render_markdown, render_text, title_from_segments, transcript_payload, transcript_stats


class NoteNotFound(Exception):
    """Raised when a note is missing or inaccessible to the caller."""

    pass


class NoteStore(ABC):
    """Storage contract shared by local filesystem and AWS-backed stores."""

    @abstractmethod
    def create_note(self, note: Note) -> Note:
        raise NotImplementedError

    @abstractmethod
    def list_notes(self, user: User) -> list[Note]:
        raise NotImplementedError

    @abstractmethod
    def get_note(self, user: User, note_id: str) -> tuple[Note, dict[str, Any] | None]:
        raise NotImplementedError

    @abstractmethod
    def update_note_title(self, user: User, note_id: str, title: str) -> Note:
        raise NotImplementedError

    @abstractmethod
    def finalize_note(self, user: User, note: Note, segments: list[TranscriptSegment], status: str = "complete") -> Note:
        raise NotImplementedError

    @abstractmethod
    def update_note_intelligence(
        self,
        user: User,
        note_id: str,
        *,
        summary: str | None,
        action_items: list[dict[str, Any]],
        decisions: list[str],
        open_questions: list[str],
        generated_title: str | None,
        summary_model: str | None,
        status: str,
    ) -> Note:
        raise NotImplementedError

    @abstractmethod
    def delete_note(self, user: User, note_id: str) -> None:
        raise NotImplementedError

    @abstractmethod
    def record_audit(self, user: User, event_type: str, metadata: dict[str, Any] | None = None) -> None:
        raise NotImplementedError


class LocalNoteStore(NoteStore):
    """Filesystem-backed note store for local development."""

    def __init__(self, root: Path) -> None:
        self.root = root
        self.root.mkdir(parents=True, exist_ok=True)

    def create_note(self, note: Note) -> Note:
        note_dir = self._note_dir(note.user_id, note.note_id)
        note_dir.mkdir(parents=True, exist_ok=True)
        self._write_json(note_dir / "metadata.json", note.to_dict())
        return note

    def update_note_intelligence(
        self,
        user: User,
        note_id: str,
        *,
        summary: str | None,
        action_items: list[dict[str, Any]],
        decisions: list[str],
        open_questions: list[str],
        generated_title: str | None,
        summary_model: str | None,
        status: str,
    ) -> Note:
        note, transcript = self.get_note(user, note_id)
        _apply_note_intelligence(
            note,
            summary=summary,
            action_items=action_items,
            decisions=decisions,
            open_questions=open_questions,
            generated_title=generated_title,
            summary_model=summary_model,
            status=status,
        )
        note_dir = self._note_dir(user.user_id, note_id)
        if transcript is not None:
            transcript["text_intelligence"] = _note_intelligence_payload(note)
            self._write_json(note_dir / "transcript.json", transcript)
        self._write_json(note_dir / "metadata.json", note.to_dict())
        return note

    def list_notes(self, user: User) -> list[Note]:
        user_dir = self.root / "users" / user.user_id / "notes"
        if not user_dir.exists():
            return []
        notes: list[Note] = []
        for metadata_path in user_dir.glob("*/metadata.json"):
            note = Note.from_dict(self._read_json(metadata_path))
            if note.deleted_at is None:
                notes.append(note)
        return sort_notes_newest_first(notes)

    def get_note(self, user: User, note_id: str) -> tuple[Note, dict[str, Any] | None]:
        note_dir = self._note_dir(user.user_id, note_id)
        metadata_path = note_dir / "metadata.json"
        if not metadata_path.exists():
            raise NoteNotFound(note_id)
        note = Note.from_dict(self._read_json(metadata_path))
        if note.user_id != user.user_id or note.deleted_at is not None:
            raise NoteNotFound(note_id)
        transcript_path = note_dir / "transcript.json"
        transcript = self._read_json(transcript_path) if transcript_path.exists() else None
        return note, transcript

    def update_note_title(self, user: User, note_id: str, title: str) -> Note:
        note, _ = self.get_note(user, note_id)
        note.title = title.strip()[:140] or note.title
        note.updated_at = utc_now()
        self._write_json(self._note_dir(user.user_id, note_id) / "metadata.json", note.to_dict())
        return note

    def finalize_note(self, user: User, note: Note, segments: list[TranscriptSegment], status: str = "complete") -> Note:
        note_dir = self._note_dir(user.user_id, note.note_id)
        note_dir.mkdir(parents=True, exist_ok=True)
        final_segments = [segment for segment in segments if segment.is_final]
        segment_count, word_count, speaker_count = transcript_stats(final_segments)
        if note.title == "Untitled voice note" and final_segments:
            note.title = title_from_segments(final_segments)
        note.status = status
        note.updated_at = utc_now()
        note.stopped_at = note.updated_at
        note.segment_count = segment_count
        note.word_count = word_count
        note.speaker_count = speaker_count
        note.duration_ms = _duration_ms(note.started_at, note.stopped_at)
        payload = transcript_payload(note, final_segments)
        self._write_json(note_dir / "transcript.json", payload)
        (note_dir / "transcript.txt").write_text(render_text(final_segments), encoding="utf-8")
        (note_dir / "transcript.md").write_text(render_markdown(note, final_segments), encoding="utf-8")
        self._write_json(note_dir / "metadata.json", note.to_dict())
        return note

    def delete_note(self, user: User, note_id: str) -> None:
        note_dir = self._note_dir(user.user_id, note_id)
        if not note_dir.exists():
            raise NoteNotFound(note_id)
        shutil.rmtree(note_dir)
        self.record_audit(user, "transcript_deleted", {"note_id": note_id})

    def record_audit(self, user: User, event_type: str, metadata: dict[str, Any] | None = None) -> None:
        audit_dir = self.root / "users" / user.user_id / "audit"
        audit_dir.mkdir(parents=True, exist_ok=True)
        event = {
            "event_type": event_type,
            "user_id": user.user_id,
            "email": user.email,
            "created_at": utc_now(),
            "metadata": metadata or {},
        }
        with (audit_dir / "events.jsonl").open("a", encoding="utf-8") as handle:
            handle.write(json.dumps(event, sort_keys=True, separators=(",", ":")) + "\n")

    def transcript_text(self, user: User, note_id: str, extension: str) -> tuple[Note, str]:
        note, _ = self.get_note(user, note_id)
        filename = f"transcript.{extension}"
        path = self._note_dir(user.user_id, note_id) / filename
        if not path.exists():
            raise NoteNotFound(note_id)
        return note, path.read_text(encoding="utf-8")

    def _note_dir(self, user_id: str, note_id: str) -> Path:
        return self.root / "users" / user_id / "notes" / note_id

    @staticmethod
    def _write_json(path: Path, payload: dict[str, Any]) -> None:
        path.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")

    @staticmethod
    def _read_json(path: Path) -> dict[str, Any]:
        return json.loads(path.read_text(encoding="utf-8"))


class AwsNoteStore(NoteStore):
    """DynamoDB/S3-backed note store for deployed VoiceNotes."""

    def __init__(self, settings: Settings) -> None:
        import boto3

        if not settings.notes_table_name or not settings.transcript_bucket_name:
            raise ValueError("AWS storage requires notes table and transcript bucket")
        self.settings = settings
        self.ddb = boto3.client("dynamodb", region_name=settings.aws_region)
        self.s3 = boto3.client("s3", region_name=settings.aws_region)

    def create_note(self, note: Note) -> Note:
        self._put_note(note)
        return note

    def update_note_intelligence(
        self,
        user: User,
        note_id: str,
        *,
        summary: str | None,
        action_items: list[dict[str, Any]],
        decisions: list[str],
        open_questions: list[str],
        generated_title: str | None,
        summary_model: str | None,
        status: str,
    ) -> Note:
        note, transcript = self.get_note(user, note_id)
        _apply_note_intelligence(
            note,
            summary=summary,
            action_items=action_items,
            decisions=decisions,
            open_questions=open_questions,
            generated_title=generated_title,
            summary_model=summary_model,
            status=status,
        )
        if transcript is not None and note.transcript_s3_key:
            transcript["text_intelligence"] = _note_intelligence_payload(note)
            self._put_s3_json(note.transcript_s3_key, transcript, user, note)
        self._put_note(note)
        return note

    def list_notes(self, user: User) -> list[Note]:
        response = self.ddb.query(
            TableName=self.settings.notes_table_name,
            KeyConditionExpression="pk = :pk",
            ExpressionAttributeValues={":pk": {"S": self._pk(user.user_id)}},
            ScanIndexForward=False,
        )
        notes = [Note.from_dict(_ddb_json(item["document"])) for item in response.get("Items", [])]
        return sort_notes_newest_first([note for note in notes if note.deleted_at is None])

    def get_note(self, user: User, note_id: str) -> tuple[Note, dict[str, Any] | None]:
        response = self.ddb.get_item(
            TableName=self.settings.notes_table_name,
            Key={"pk": {"S": self._pk(user.user_id)}, "sk": {"S": self._sk(note_id)}},
        )
        item = response.get("Item")
        if not item:
            raise NoteNotFound(note_id)
        note = Note.from_dict(_ddb_json(item["document"]))
        if note.deleted_at is not None:
            raise NoteNotFound(note_id)
        transcript = None
        if note.transcript_s3_key:
            try:
                transcript = json.loads(
                    self.s3.get_object(Bucket=self.settings.transcript_bucket_name, Key=note.transcript_s3_key)[
                        "Body"
                    ].read()
                )
            except Exception as exc:
                if _is_missing_s3_object(exc):
                    raise NoteNotFound(note_id) from exc
                raise
        return note, transcript

    def update_note_title(self, user: User, note_id: str, title: str) -> Note:
        note, _ = self.get_note(user, note_id)
        note.title = title.strip()[:140] or note.title
        note.updated_at = utc_now()
        self._put_note(note)
        return note

    def finalize_note(self, user: User, note: Note, segments: list[TranscriptSegment], status: str = "complete") -> Note:
        final_segments = [segment for segment in segments if segment.is_final]
        segment_count, word_count, speaker_count = transcript_stats(final_segments)
        if note.title == "Untitled voice note" and final_segments:
            note.title = title_from_segments(final_segments)
        note.status = status
        note.updated_at = utc_now()
        note.stopped_at = note.updated_at
        note.segment_count = segment_count
        note.word_count = word_count
        note.speaker_count = speaker_count
        note.duration_ms = _duration_ms(note.started_at, note.stopped_at)
        base_key = f"users/{user.user_id}/notes/{note.note_id}"
        note.transcript_s3_key = f"{base_key}/transcript.json"
        note.transcript_text_s3_key = f"{base_key}/transcript.txt"
        note.transcript_md_s3_key = f"{base_key}/transcript.md"
        self._put_s3_json(note.transcript_s3_key, transcript_payload(note, final_segments), user, note)
        self._put_s3_text(note.transcript_text_s3_key, render_text(final_segments), user, note)
        self._put_s3_text(note.transcript_md_s3_key, render_markdown(note, final_segments), user, note)
        self._put_note(note)
        return note

    def delete_note(self, user: User, note_id: str) -> None:
        note, _ = self.get_note(user, note_id)
        for key in [note.transcript_s3_key, note.transcript_text_s3_key, note.transcript_md_s3_key]:
            if key:
                self.s3.delete_object(Bucket=self.settings.transcript_bucket_name, Key=key)
        note.deleted_at = utc_now()
        note.updated_at = note.deleted_at
        self._put_note(note)
        self.record_audit(user, "transcript_deleted", {"note_id": note_id})

    def record_audit(self, user: User, event_type: str, metadata: dict[str, Any] | None = None) -> None:
        if not self.settings.audit_table_name:
            return
        now = utc_now()
        self.ddb.put_item(
            TableName=self.settings.audit_table_name,
            Item={
                "pk": {"S": self._pk(user.user_id)},
                "sk": {"S": f"EVENT#{now}"},
                "event_type": {"S": event_type},
                "email": {"S": user.email},
                "created_at": {"S": now},
                "metadata": {"S": json.dumps(metadata or {}, sort_keys=True)},
            },
        )

    def transcript_text(self, user: User, note_id: str, extension: str) -> tuple[Note, str]:
        note, _ = self.get_note(user, note_id)
        key = note.transcript_text_s3_key if extension == "txt" else note.transcript_md_s3_key
        if not key:
            raise NoteNotFound(note_id)
        try:
            body = self.s3.get_object(Bucket=self.settings.transcript_bucket_name, Key=key)["Body"].read()
        except Exception as exc:
            if _is_missing_s3_object(exc):
                raise NoteNotFound(note_id) from exc
            raise
        return note, body.decode("utf-8")

    def _put_note(self, note: Note) -> None:
        self.ddb.put_item(
            TableName=self.settings.notes_table_name,
            Item={
                "pk": {"S": self._pk(note.user_id)},
                "sk": {"S": self._sk(note.note_id)},
                "created_at": {"S": note.created_at},
                "status": {"S": note.status},
                "document": {"S": json.dumps(note.to_dict(), sort_keys=True, separators=(",", ":"))},
            },
        )

    def _put_s3_json(self, key: str, payload: dict[str, Any], user: User, note: Note) -> None:
        self._put_s3_text(key, json.dumps(payload, indent=2, sort_keys=True) + "\n", user, note, "application/json")

    def _put_s3_text(
        self,
        key: str,
        body: str,
        user: User,
        note: Note,
        content_type: str = "text/plain; charset=utf-8",
    ) -> None:
        args: dict[str, Any] = {
            "Bucket": self.settings.transcript_bucket_name,
            "Key": key,
            "Body": body.encode("utf-8"),
            "ContentType": content_type,
            "ServerSideEncryption": "aws:kms",
            "Metadata": {"user_id": user.user_id, "note_id": note.note_id},
        }
        if self.settings.transcript_kms_key_id:
            args["SSEKMSKeyId"] = self.settings.transcript_kms_key_id
        self.s3.put_object(**args)

    @staticmethod
    def _pk(user_id: str) -> str:
        return f"USER#{user_id}"

    @staticmethod
    def _sk(note_id: str) -> str:
        return f"NOTE#{note_id}"


def create_store(settings: Settings) -> NoteStore:
    if settings.storage_backend == "aws":
        return AwsNoteStore(settings)
    return LocalNoteStore(settings.local_data_dir)


def sort_notes_newest_first(notes: list[Note]) -> list[Note]:
    notes.sort(key=lambda item: item.created_at, reverse=True)
    return notes


def _duration_ms(started_at: str | None, stopped_at: str | None) -> int:
    if not started_at or not stopped_at:
        return 0
    try:
        start = datetime.fromisoformat(started_at.replace("Z", "+00:00"))
        stop = datetime.fromisoformat(stopped_at.replace("Z", "+00:00"))
    except ValueError:
        return 0
    return max(0, int((stop - start).total_seconds() * 1000))


def _ddb_json(attribute: dict[str, Any]) -> dict[str, Any]:
    return json.loads(attribute["S"])


def _apply_note_intelligence(
    note: Note,
    *,
    summary: str | None,
    action_items: list[dict[str, Any]],
    decisions: list[str],
    open_questions: list[str],
    generated_title: str | None,
    summary_model: str | None,
    status: str,
) -> None:
    note.summary = summary.strip() if isinstance(summary, str) and summary.strip() else None
    note.action_items = action_items
    note.decisions = decisions
    note.open_questions = open_questions
    note.generated_title = generated_title.strip() if isinstance(generated_title, str) and generated_title.strip() else None
    note.summary_model = summary_model.strip() if isinstance(summary_model, str) and summary_model.strip() else None
    note.summary_status = status
    note.summary_generated_at = utc_now() if status == "complete" else None
    note.updated_at = utc_now()


def _note_intelligence_payload(note: Note) -> dict[str, Any]:
    return {
        "summary": note.summary,
        "action_items": note.action_items,
        "decisions": note.decisions,
        "open_questions": note.open_questions,
        "generated_title": note.generated_title,
        "summary_status": note.summary_status,
        "summary_model": note.summary_model,
        "summary_generated_at": note.summary_generated_at,
    }


def _is_missing_s3_object(exc: Exception) -> bool:
    response = getattr(exc, "response", None)
    if not isinstance(response, dict):
        return False
    error = response.get("Error")
    if not isinstance(error, dict):
        return False
    return str(error.get("Code") or "") in {"404", "NoSuchKey", "NotFound"}
