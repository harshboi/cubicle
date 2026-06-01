from pathlib import Path

from voicenotes_app.models import Note, TranscriptSegment, User
from voicenotes_app.store import LocalNoteStore, NoteNotFound, sort_notes_newest_first


def test_local_store_enforces_user_boundary(tmp_path: Path):
    store = LocalNoteStore(tmp_path)
    owner = User.from_email("owner@example.com")
    other = User.from_email("other@example.com")
    note = Note.create(user=owner, language_mode="english_to_english", diarization_enabled=True)
    store.create_note(note)

    try:
        store.get_note(other, note.note_id)
    except NoteNotFound:
        pass
    else:
        raise AssertionError("other user could read note")


def test_local_store_finalizes_transcript(tmp_path: Path):
    store = LocalNoteStore(tmp_path)
    user = User.from_email("owner@example.com")
    note = Note.create(user=user, language_mode="english_to_english", diarization_enabled=True)
    store.create_note(note)

    finalized = store.finalize_note(
        user,
        note,
        [
            TranscriptSegment(
                segment_id="seg1",
                text="Hello from VoiceNotes",
                start_time_ms=0,
                end_time_ms=1000,
                speaker_id="SPEAKER_00",
            )
        ],
    )
    loaded, transcript = store.get_note(user, note.note_id)

    assert finalized.status == "complete"
    assert loaded.word_count == 3
    assert transcript is not None
    assert transcript["segments"][0]["text"] == "Hello from VoiceNotes"


def test_local_store_updates_note_intelligence(tmp_path: Path):
    store = LocalNoteStore(tmp_path)
    user = User.from_email("owner@example.com")
    note = Note.create(user=user, language_mode="english_to_english", diarization_enabled=False)
    store.create_note(note)
    store.finalize_note(
        user,
        note,
        [
            TranscriptSegment(
                segment_id="seg1",
                text="Schedule the migration tomorrow.",
                start_time_ms=0,
                end_time_ms=1000,
            )
        ],
    )

    updated = store.update_note_intelligence(
        user,
        note.note_id,
        summary="The team discussed a migration.",
        action_items=[{"task": "Schedule migration", "owner": None, "due_date": "tomorrow"}],
        decisions=["Use the new worker."],
        open_questions=["Who owns the rollout?"],
        generated_title="Migration discussion",
        summary_model="Qwen/Qwen2.5-7B-Instruct",
        status="complete",
    )
    loaded, transcript = store.get_note(user, note.note_id)

    assert updated.summary_status == "complete"
    assert loaded.summary == "The team discussed a migration."
    assert loaded.generated_title == "Migration discussion"
    assert transcript is not None
    assert transcript["text_intelligence"]["summary"] == "The team discussed a migration."
    assert transcript["text_intelligence"]["action_items"][0]["task"] == "Schedule migration"


def test_sort_notes_newest_first_uses_created_at():
    user = User.from_email("owner@example.com")
    older = Note.create(user=user, language_mode="english_to_english", diarization_enabled=True, note_id="old")
    newer = Note.create(user=user, language_mode="english_to_english", diarization_enabled=True, note_id="new")
    older.created_at = "2026-05-23T10:00:00.000Z"
    newer.created_at = "2026-05-23T11:00:00.000Z"

    sorted_notes = sort_notes_newest_first([older, newer])

    assert [note.note_id for note in sorted_notes] == ["new", "old"]
