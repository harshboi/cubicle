import base64
import hashlib
import hmac
import json

from voicenotes_app.models import Note, User
from voicenotes_app.proxy import (
    RealtimeTranslationCoordinator,
    RealtimeTranscriptLineNormalizer,
    RecordingAccessRevoked,
    _browser_event,
    _check_recording_access,
    _parse_start,
    _upstream_authorization_token,
    _upstream_recording_loop,
)
from voicenotes_app.settings import Settings
from voicenotes_app.text_intelligence import TranscriptSummaryResult, TranslationResult
from voicenotes_app.transcripts import TranscriptCollector


def _decode_part(value: str) -> dict:
    padding = "=" * (-len(value) % 4)
    return json.loads(base64.urlsafe_b64decode((value + padding).encode("ascii")))


def test_upstream_authorization_uses_static_token_without_signing_secret():
    settings = Settings(upstream_transcription_token="static-service-token")
    user = User.from_email("user@example.com")

    token = _upstream_authorization_token(settings, user)

    assert token == "static-service-token"


def test_recording_start_forces_diarization_off_even_when_client_requests_it():
    start = _parse_start(
        json.dumps(
            {
                "type": "start_recording",
                "language_mode": "multilingual_to_english",
                "diarization_enabled": True,
            }
        )
    )

    assert start["language_mode"] == "multilingual_to_english"
    assert start["diarization_enabled"] is False


def test_recording_start_defaults_to_multilingual_source_mode():
    start = _parse_start(json.dumps({"type": "start_recording", "diarization_enabled": False}))

    assert start["language_mode"] == "multilingual_to_english"
    assert start["diarization_enabled"] is False


def test_browser_event_strips_speaker_data_when_diarization_is_disabled():
    user = User.from_email("owner@example.com")
    note = Note.create(user=user, language_mode="english_to_english", diarization_enabled=False, note_id="note_1")

    transcript_event = _browser_event(
        note,
        {
            "type": "final_transcript",
            "segment_id": "seg_1",
            "text": "hello",
            "speaker_id": "SPEAKER_00",
        },
    )
    speaker_event = _browser_event(note, {"type": "speaker_update", "segment_id": "seg_1", "speaker_id": "SPEAKER_00"})

    assert transcript_event is not None
    assert "speaker_id" not in transcript_event
    assert speaker_event is None


def test_upstream_authorization_mints_signed_user_token_when_configured():
    settings = Settings(
        upstream_transcription_token="static-service-token",
        upstream_transcription_signing_secret="signing-secret",
        upstream_transcription_token_ttl_seconds=600,
    )
    user = User.from_email("User@Example.com", "Example User")

    token = _upstream_authorization_token(settings, user)
    header_b64, payload_b64, signature_b64 = token.split(".")

    header = _decode_part(header_b64)
    claims = _decode_part(payload_b64)
    expected_signature = hmac.new(
        b"signing-secret",
        f"{header_b64}.{payload_b64}".encode("ascii"),
        hashlib.sha256,
    ).digest()
    padding = "=" * (-len(signature_b64) % 4)
    supplied_signature = base64.urlsafe_b64decode((signature_b64 + padding).encode("ascii"))

    assert hmac.compare_digest(supplied_signature, expected_signature)
    assert header == {"alg": "HS256", "typ": "JWT"}
    assert claims["iss"] == "cubicle-transcription"
    assert claims["aud"] == "cubicle-macos"
    assert claims["sub"] == "user_at_example_com"
    assert claims["email"] == "user@example.com"
    assert claims["scope"] == "transcription:stream"
    assert claims["exp"] - claims["iat"] == 600
    assert claims["jti"]


async def test_recording_access_check_fails_closed_when_validator_rejects():
    async def reject_access(user: User) -> None:
        raise RuntimeError("revoked")

    try:
        await _check_recording_access(User.from_email("deleted@example.com"), reject_access, 0.0, 5)
    except RecordingAccessRevoked:
        pass
    else:
        raise AssertionError("revoked recording access accepted")


async def test_realtime_translation_uses_previous_two_source_lines_as_context():
    user = User.from_email("owner@example.com")
    note = Note.create(user=user, language_mode="english_to_english", diarization_enabled=False, note_id="note_1")
    collector = TranscriptCollector()
    websocket = FakeWebSocket()
    client = FakeTextIntelligenceClient()
    coordinator = RealtimeTranslationCoordinator(
        client=client,
        note=note,
        settings=Settings(text_intelligence_context_lines=2),
        collector=collector,
        websocket=websocket,
        send_lock=FakeLock(),
    )

    for index, text in enumerate(["First line", "Second line", "तीसरी line"]):
        event = {
            "type": "final_transcript",
            "note_id": note.note_id,
            "segment_id": f"seg_{index}",
            "text": text,
            "is_final": True,
        }
        collector.add_event(event)
        coordinator.observe_transcript_event(event)

    await coordinator.drain()

    assert client.calls[-1]["previous_lines"] == ["First line", "Second line"]
    assert collector.segments["seg_2"].source_text == "तीसरी line"
    assert collector.segments["seg_2"].text == "EN: तीसरी line"
    assert collector.segments["seg_2"].translation_status == "complete"
    assert len(websocket.sent) == 3
    assert websocket.sent[-1]["type"] == "translated_transcript"


async def test_realtime_translation_failure_keeps_original_line():
    user = User.from_email("owner@example.com")
    note = Note.create(user=user, language_mode="english_to_english", diarization_enabled=False, note_id="note_1")
    collector = TranscriptCollector()
    websocket = FakeWebSocket()
    coordinator = RealtimeTranslationCoordinator(
        client=FailingTextIntelligenceClient(),
        note=note,
        settings=Settings(text_intelligence_context_lines=2),
        collector=collector,
        websocket=websocket,
        send_lock=FakeLock(),
    )
    event = {
        "type": "final_transcript",
        "note_id": note.note_id,
        "segment_id": "seg_1",
        "text": "こんにちは",
        "is_final": True,
    }

    collector.add_event(event)
    coordinator.observe_transcript_event(event)
    await coordinator.drain()

    assert collector.segments["seg_1"].text == "こんにちは"
    assert collector.segments["seg_1"].source_text == "こんにちは"
    assert collector.segments["seg_1"].translated_text is None
    assert collector.segments["seg_1"].translation_status == "failed"
    assert len(websocket.sent) == 1
    failed_event = websocket.sent[0]
    assert failed_event["type"] == "translated_transcript"
    assert failed_event["note_id"] == note.note_id
    assert failed_event["segment_id"] == "seg_1"
    assert failed_event["source_text"] == "こんにちは"
    assert failed_event["text"] == "こんにちは"
    assert failed_event["translation_model"] == coordinator.settings.text_intelligence_model
    assert failed_event["translation_status"] == "failed"
    assert failed_event["context_line_count"] == 0
    assert failed_event["latency_ms"] >= 0


def test_realtime_line_normalizer_splits_long_unpunctuated_partials():
    normalizer = RealtimeTranscriptLineNormalizer(max_words_per_line=4, max_chars_per_line=80)
    event = {
        "type": "partial_transcript",
        "note_id": "note_1",
        "segment_id": "seg_1",
        "text": "एक दो तीन चार पांच छह सात",
        "is_final": False,
        "is_partial": True,
        "language_mode": "multilingual_to_english",
    }

    normalized = normalizer.observe_event(event)

    assert [item["type"] for item in normalized] == ["final_transcript", "partial_transcript"]
    assert normalized[0]["segment_id"] == "seg_1-line-1"
    assert normalized[0]["text"] == "एक दो तीन चार"
    assert normalized[1]["segment_id"] == "seg_1-line-2"
    assert normalized[1]["text"] == "पांच छह सात"


def test_realtime_line_normalizer_splits_cjk_punctuation_without_waiting_for_spaces():
    normalizer = RealtimeTranscriptLineNormalizer()
    event = {
        "type": "partial_transcript",
        "note_id": "note_1",
        "segment_id": "seg_1",
        "text": "今日は東京で会議をしました。次に大阪へ移動して新しい計画を説明します",
        "is_final": False,
        "is_partial": True,
        "language_mode": "multilingual_to_english",
    }

    normalized = normalizer.observe_event(event)

    assert normalized[0]["type"] == "final_transcript"
    assert normalized[0]["segment_id"] == "seg_1-line-1"
    assert normalized[0]["text"] == "今日は東京で会議をしました。"
    assert normalized[1]["type"] == "partial_transcript"
    assert normalized[1]["segment_id"] == "seg_1-line-2"


def test_realtime_line_normalizer_uses_shorter_cjk_char_chunks():
    normalizer = RealtimeTranscriptLineNormalizer(max_chars_per_line=140)
    text = "これは長い日本語の音声認識結果で句読点がしばらく出てこないため画面に大きな塊として残らないように短い単位へ分割します続きの内容も翻訳へ早く渡します"
    event = {
        "type": "partial_transcript",
        "note_id": "note_1",
        "segment_id": "seg_1",
        "text": text,
        "is_final": False,
        "is_partial": True,
        "language_mode": "multilingual_to_english",
    }

    normalized = normalizer.observe_event(event)

    assert normalized[0]["type"] == "final_transcript"
    assert len(normalized[0]["text"]) <= 72
    assert normalized[1]["type"] == "partial_transcript"


def test_realtime_line_normalizer_splits_mixed_cjk_latin_partials_before_waiting_for_done():
    normalizer = RealtimeTranscriptLineNormalizer(max_chars_per_line=140)
    text = (
        "TeですInternationalあとそばも好きです理由4食理由はこれ安いからですもう一回言ってください"
        "Indian cuisineCurryKebabChinese noodles確かに外でよく食べます"
    )
    event = {
        "type": "partial_transcript",
        "note_id": "note_1",
        "segment_id": "seg_1",
        "text": text,
        "is_final": False,
        "is_partial": True,
        "language_mode": "multilingual_to_english",
    }

    normalized = normalizer.observe_event(event)

    assert normalized[0]["type"] == "final_transcript"
    assert len(normalized[0]["text"]) <= 72
    assert normalized[1]["type"] == "partial_transcript"


def test_realtime_line_normalizer_finalizes_only_new_trailing_line_on_stop():
    normalizer = RealtimeTranscriptLineNormalizer(max_words_per_line=4, max_chars_per_line=80)
    partial = {
        "type": "partial_transcript",
        "note_id": "note_1",
        "segment_id": "seg_1",
        "text": "एक दो तीन चार पांच छह सात",
        "is_final": False,
        "is_partial": True,
        "language_mode": "multilingual_to_english",
    }
    final = {**partial, "type": "final_transcript", "is_final": True, "is_partial": False}

    normalizer.observe_event(partial)
    normalized = normalizer.observe_event(final)

    assert len(normalized) == 1
    assert normalized[0]["type"] == "final_transcript"
    assert normalized[0]["segment_id"] == "seg_1-line-2"
    assert normalized[0]["text"] == "पांच छह सात"


async def test_realtime_translation_ignores_duplicate_final_source_line():
    user = User.from_email("owner@example.com")
    note = Note.create(user=user, language_mode="multilingual_to_english", diarization_enabled=False, note_id="note_1")
    collector = TranscriptCollector()
    websocket = FakeWebSocket()
    client = FakeTextIntelligenceClient()
    coordinator = RealtimeTranslationCoordinator(
        client=client,
        note=note,
        settings=Settings(text_intelligence_context_lines=2),
        collector=collector,
        websocket=websocket,
        send_lock=FakeLock(),
    )
    event = {
        "type": "final_transcript",
        "note_id": note.note_id,
        "segment_id": "seg_1",
        "text": "नमस्ते दुनिया",
        "is_final": True,
    }

    collector.add_event(event)
    coordinator.observe_transcript_event(event)
    coordinator.observe_transcript_event(event)
    await coordinator.drain()

    assert len(client.calls) == 1
    assert len(websocket.sent) == 1


async def test_upstream_recording_loop_normalizes_realtime_partial_lines(monkeypatch):
    user = User.from_email("owner@example.com")
    note = Note.create(user=user, language_mode="multilingual_to_english", diarization_enabled=False, note_id="note_1")
    collector = TranscriptCollector()
    websocket = FakeRecordingWebSocket()
    upstream = FakeUpstream(
        [
            {
                "type": "partial_transcript",
                "segment_id": "seg_1",
                "text": (
                    "एक दो तीन चार पांच छह सात आठ नौ दस ग्यारह बारह "
                    "तेरह चौदह पंद्रह सोलह सत्रह अठारह उन्नीस बीस"
                ),
                "is_final": False,
                "language_mode": "multilingual_to_english",
            },
            {"type": "session_stopped", "session_id": note.note_id},
        ]
    )

    async def fake_connect_upstream(url, headers):
        return upstream

    monkeypatch.setattr("voicenotes_app.proxy._connect_upstream", fake_connect_upstream)

    await _upstream_recording_loop(
        websocket,
        note=note,
        user=user,
        settings=Settings(upstream_transcription_token="token", text_intelligence_context_lines=2),
        store=FakeNoteStore(),
        collector=collector,
        text_intelligence_client=FakeTextIntelligenceClient(),
    )

    sent_types = [event["type"] for event in websocket.sent]
    assert "final_transcript" in sent_types
    assert "translated_transcript" in sent_types
    assert any(event.get("segment_id") == "seg_1-line-1" for event in websocket.sent)


class FakeLock:
    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc, traceback):
        return None


class FakeWebSocket:
    def __init__(self) -> None:
        self.sent: list[dict] = []

    async def send_text(self, value: str) -> None:
        self.sent.append(json.loads(value))


class FakeRecordingWebSocket(FakeWebSocket):
    def __init__(self) -> None:
        super().__init__()
        self.closed = False
        self._messages = [{"text": json.dumps({"type": "stop_recording"})}]

    async def receive(self) -> dict:
        if self._messages:
            return self._messages.pop(0)
        return {"type": "websocket.disconnect"}

    async def close(self, code: int = 1000, reason: str | None = None) -> None:
        self.closed = True


class FakeUpstream:
    def __init__(self, events: list[dict]) -> None:
        self.events = [json.dumps(event) for event in events]
        self.sent: list[str] = []

    def __aiter__(self):
        return self

    async def __anext__(self):
        if not self.events:
            raise StopAsyncIteration
        return self.events.pop(0)

    async def send(self, value: str) -> None:
        self.sent.append(value)

    async def close(self) -> None:
        return None


class FakeNoteStore:
    def finalize_note(self, user: User, note: Note, segments, *, status: str) -> Note:
        note.status = status
        return note

    def record_audit(self, user: User, action: str, metadata: dict) -> None:
        return None


class FakeTextIntelligenceClient:
    def __init__(self) -> None:
        self.calls: list[dict] = []

    async def translate_line(
        self,
        *,
        note_id: str,
        segment_id: str,
        previous_lines: list[str],
        target_line: str,
    ) -> TranslationResult:
        self.calls.append(
            {
                "note_id": note_id,
                "segment_id": segment_id,
                "previous_lines": previous_lines,
                "target_line": target_line,
            }
        )
        return TranslationResult(
            segment_id=segment_id,
            text=f"EN: {target_line}",
            source_text=target_line,
            translation_model="test-model",
            context_line_count=len(previous_lines),
            latency_ms=7,
        )

    async def summarize_transcript(self, *, note_id: str, lines: list[str]) -> TranscriptSummaryResult:
        return TranscriptSummaryResult(model="test-model")


class FailingTextIntelligenceClient(FakeTextIntelligenceClient):
    async def translate_line(
        self,
        *,
        note_id: str,
        segment_id: str,
        previous_lines: list[str],
        target_line: str,
    ) -> TranslationResult:
        raise RuntimeError("translation failed")
