from pathlib import Path

from fastapi.testclient import TestClient

from voicenotes_app.auth import AuthError, SessionCodec
from voicenotes_app.main import create_app
from voicenotes_app.models import Note, TranscriptSegment, User
from voicenotes_app.settings import Settings
from voicenotes_app.store import LocalNoteStore


def make_client(tmp_path: Path) -> TestClient:
    settings = Settings(
        session_secret="test-secret",
        local_users="prabhat7@cisco.com:voicenotes-dev:Prabhat Singh",
        local_data_dir=tmp_path,
        mock_transcription=True,
    )
    app = create_app(settings=settings, store=LocalNoteStore(tmp_path))
    return TestClient(app)


def login(client: TestClient) -> None:
    response = client.post(
        "/api/login",
        json={"email": "prabhat7@cisco.com", "password": "voicenotes-dev"},
    )
    assert response.status_code == 200


def test_login_and_notes_list(tmp_path: Path):
    client = make_client(tmp_path)

    unauthenticated = client.get("/api/notes")
    assert unauthenticated.status_code == 401

    login(client)
    notes = client.get("/api/notes")
    assert notes.status_code == 200
    assert notes.json()["notes"] == []


def test_mock_recording_saves_note(tmp_path: Path):
    client = make_client(tmp_path)
    login(client)

    with client.websocket_connect("/ws/record") as websocket:
        websocket.send_json(
            {
                "type": "start_recording",
                "language_mode": "english_to_english",
                "diarization_enabled": True,
            }
        )
        started = websocket.receive_json()
        assert started["type"] == "recording_started"
        assert started["diarization_enabled"] is False
        websocket.send_bytes(b"\x00\x01" * 1024)
        websocket.send_bytes(b"\x00\x01" * 1024)
        partial = websocket.receive_json()
        assert partial["type"] == "partial_transcript"
        websocket.send_json({"type": "stop_recording"})
        final = websocket.receive_json()
        stopped = websocket.receive_json()
        assert final["type"] == "final_transcript"
        assert stopped["type"] == "recording_stopped"

    notes = client.get("/api/notes").json()["notes"]
    assert len(notes) == 1
    note_id = notes[0]["note_id"]
    detail = client.get(f"/api/notes/{note_id}").json()
    assert detail["note"]["diarization_enabled"] is False
    assert detail["transcript"]["diarization_enabled"] is False
    assert detail["transcript"]["segments"][0]["text"] == "Voice note captured in local mock mode."
    assert detail["transcript"]["segments"][0]["speaker_id"] is None


def test_download_uses_local_timestamp_filename(tmp_path: Path):
    store = LocalNoteStore(tmp_path)
    settings = Settings(
        session_secret="test-secret",
        local_users="prabhat7@cisco.com:voicenotes-dev:Prabhat Singh",
        local_data_dir=tmp_path,
        mock_transcription=True,
    )
    app = create_app(settings=settings, store=store)
    client = TestClient(app)
    login(client)
    user = User.from_email("prabhat7@cisco.com", "Prabhat Singh")
    note = Note.create(user=user, language_mode="english_to_english", diarization_enabled=True)
    note.created_at = "2026-05-24T15:36:42+00:00"
    store.create_note(note)
    store.finalize_note(
        user,
        note,
        [
            TranscriptSegment(
                segment_id="seg1",
                text="어? 뭐야? This is a test",
                speaker_id="Speaker 1",
            )
        ],
    )

    response = client.get(f"/api/notes/{note.note_id}/download.txt?timezone=America/Los_Angeles")

    assert response.status_code == 200
    disposition = response.headers["content-disposition"]
    assert 'filename="VoiceNotes-2026-05-24-08-36-42PDT.txt"' in disposition
    assert "filename*=UTF-8''VoiceNotes-2026-05-24-08-36-42PDT.txt" in disposition
    assert response.text == "Speaker 1: 어? 뭐야? This is a test\n"


def test_bulk_delete_notes_removes_selected_notes(tmp_path: Path):
    store = LocalNoteStore(tmp_path)
    settings = Settings(
        session_secret="test-secret",
        local_users="prabhat7@cisco.com:voicenotes-dev:Prabhat Singh",
        local_data_dir=tmp_path,
        mock_transcription=True,
    )
    app = create_app(settings=settings, store=store)
    client = TestClient(app)
    login(client)
    user = User.from_email("prabhat7@cisco.com", "Prabhat Singh")
    first = Note.create(user=user, language_mode="english_to_english", diarization_enabled=True, note_id="first")
    second = Note.create(user=user, language_mode="english_to_english", diarization_enabled=True, note_id="second")
    store.create_note(first)
    store.create_note(second)

    response = client.post(
        "/api/notes/bulk-delete",
        json={"note_ids": [first.note_id, first.note_id, "missing"]},
    )

    assert response.status_code == 200
    assert response.json()["deleted"] == [first.note_id]
    assert response.json()["not_found"] == ["missing"]
    remaining = client.get("/api/notes").json()["notes"]
    assert [note["note_id"] for note in remaining] == [second.note_id]


def test_oidc_deleted_user_session_is_rejected(tmp_path: Path):
    settings = Settings(
        auth_mode="oidc",
        session_secret="test-secret",
        local_data_dir=tmp_path,
        mock_transcription=True,
        oidc_issuer="https://cognito-idp.us-west-2.amazonaws.com/us-west-2_example",
    )
    app = create_app(
        settings=settings,
        store=LocalNoteStore(tmp_path),
        user_access_validator=RejectingAccessValidator(),
    )
    client = TestClient(app)
    token = SessionCodec(settings.session_secret, settings.session_ttl_seconds).encode(
        User.from_email("deleted@example.com")
    )

    response = client.get("/api/me", cookies={settings.session_cookie_name: token})
    shell = client.get("/", cookies={settings.session_cookie_name: token}, follow_redirects=False)

    assert response.status_code == 401
    assert shell.status_code == 302
    assert shell.headers["location"] == "/login"


def test_oidc_logout_redirects_through_cognito(tmp_path: Path):
    settings = Settings(
        auth_mode="oidc",
        session_secret="test-secret",
        local_data_dir=tmp_path,
        mock_transcription=True,
        oidc_client_id="client-id",
        oidc_redirect_uri="https://voicenotes.example.com/auth/callback",
        oidc_logout_url="https://auth.example.com/logout",
        oidc_session_validation_enabled=False,
    )
    app = create_app(settings=settings, store=LocalNoteStore(tmp_path))
    client = TestClient(app)

    response = client.post("/api/logout")
    redirect = client.get("/logout", follow_redirects=False)

    assert response.status_code == 200
    assert response.json()["logout_url"] == (
        "https://auth.example.com/logout?"
        "client_id=client-id&logout_uri=https%3A%2F%2Fvoicenotes.example.com%2Flogin"
    )
    assert redirect.status_code == 302
    assert redirect.headers["location"] == response.json()["logout_url"]


def test_oidc_invalid_callback_restarts_login(tmp_path: Path):
    settings = Settings(
        auth_mode="oidc",
        session_secret="test-secret",
        local_data_dir=tmp_path,
        mock_transcription=True,
        oidc_client_id="client-id",
        oidc_redirect_uri="https://voicenotes.example.com/auth/callback",
        oidc_authorization_endpoint="https://auth.example.com/oauth2/authorize",
        oidc_session_validation_enabled=False,
    )
    app = create_app(settings=settings, store=LocalNoteStore(tmp_path))
    client = TestClient(app)

    response = client.get(
        "/auth/callback?code=replayed-code&state=replayed-state",
        cookies={"voicenotes_oidc_state": "newer-state", "voicenotes_oidc_nonce": "nonce"},
        follow_redirects=False,
    )

    assert response.status_code == 302
    assert response.headers["location"] == "/login"
    assert "voicenotes_oidc_state=" in response.headers["set-cookie"]
    assert "Max-Age=0" in response.headers["set-cookie"]


class RejectingAccessValidator:
    async def validate(self, user: User, *, use_cache: bool = True) -> None:
        raise AuthError("revoked")
