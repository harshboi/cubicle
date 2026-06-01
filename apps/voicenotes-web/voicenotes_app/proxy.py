"""Browser recording WebSocket proxy and transcript event normalization."""

from __future__ import annotations

import asyncio
import base64
from contextlib import suppress
from datetime import datetime, timedelta, timezone
import hashlib
import hmac
import json
import logging
import re
import secrets
from typing import Any, Awaitable, Callable

from fastapi import WebSocket
import websockets

from .models import Note, TranscriptSegment, User, new_id, utc_now
from .settings import Settings
from .store import NoteStore
from .text_intelligence import TextIntelligenceClient
from .transcripts import TranscriptCollector


logger = logging.getLogger("voicenotes.recording")

SUPPORTED_LANGUAGE_MODES = {"english_to_english", "japanese_to_english", "multilingual_to_english"}
AccessValidator = Callable[[User], Awaitable[None]]


class RecordingError(Exception):
    """Recording-session failure for the browser WebSocket proxy."""

    pass


class RecordingAccessRevoked(RecordingError):
    """Raised when access is revoked while a recording is active."""

    pass


class ActiveSessionRegistry:
    """In-memory per-user active-recording counter."""

    def __init__(self) -> None:
        self._active_by_user: dict[str, int] = {}
        self._lock = asyncio.Lock()

    async def acquire(self, user: User, limit: int) -> bool:
        async with self._lock:
            current = self._active_by_user.get(user.user_id, 0)
            if current >= limit:
                return False
            self._active_by_user[user.user_id] = current + 1
            return True

    async def release(self, user: User) -> None:
        async with self._lock:
            current = self._active_by_user.get(user.user_id, 0)
            if current <= 1:
                self._active_by_user.pop(user.user_id, None)
            else:
                self._active_by_user[user.user_id] = current - 1


class RealtimeTranslationCoordinator:
    """Schedules translation for finalized transcript lines without blocking capture."""

    def __init__(
        self,
        *,
        client: TextIntelligenceClient | None,
        note: Note,
        settings: Settings,
        collector: TranscriptCollector,
        websocket: WebSocket,
        send_lock: asyncio.Lock,
    ) -> None:
        self.client = client
        self.note = note
        self.settings = settings
        self.collector = collector
        self.websocket = websocket
        self.send_lock = send_lock
        self.source_lines: list[str] = []
        self._source_by_segment_id: dict[str, str] = {}
        self.tasks: list[asyncio.Task[None]] = []

    def observe_transcript_event(self, event: dict[str, Any]) -> None:
        if event.get("type") != "final_transcript":
            return
        segment_id = str(event.get("segment_id") or "")
        source_text = str(event.get("text") or "").strip()
        if not segment_id or not source_text:
            return
        if self._source_by_segment_id.get(segment_id) == source_text:
            return
        previous_lines = self.source_lines[-self.settings.text_intelligence_context_lines :]
        self._source_by_segment_id[segment_id] = source_text
        self.source_lines.append(source_text)
        if self.client is None:
            return
        self.tasks.append(
            asyncio.create_task(
                self._translate_line(
                    segment_id=segment_id,
                    source_text=source_text,
                    previous_lines=previous_lines,
                )
            )
        )

    async def drain(self) -> None:
        if not self.tasks:
            return
        pending = [task for task in self.tasks if not task.done()]
        if not pending:
            return
        timeout = self.settings.text_intelligence_flush_timeout_seconds
        if timeout <= 0:
            for task in pending:
                task.cancel()
            return
        done, still_pending = await asyncio.wait(pending, timeout=timeout)
        for task in still_pending:
            task.cancel()
        for task in done:
            with suppress(Exception):
                task.result()

    async def _translate_line(self, *, segment_id: str, source_text: str, previous_lines: list[str]) -> None:
        assert self.client is not None
        started = asyncio.get_running_loop().time()
        try:
            result = await self.client.translate_line(
                note_id=self.note.note_id,
                segment_id=segment_id,
                previous_lines=previous_lines,
                target_line=source_text,
            )
        except Exception as exc:
            logger.warning(
                "realtime_translation_failed",
                extra={
                    "note_id": self.note.note_id,
                    "segment_id": segment_id,
                    "error_class": type(exc).__name__,
                },
            )
            translated_event = {
                "type": "translated_transcript",
                "note_id": self.note.note_id,
                "segment_id": segment_id,
                "source_text": source_text,
                "text": source_text,
                "translation_model": self.settings.text_intelligence_model,
                "translation_status": "failed",
                "context_line_count": len(previous_lines),
                "latency_ms": _elapsed_ms(started),
            }
            self.collector.add_event(translated_event)
            async with self.send_lock:
                await self.websocket.send_text(json.dumps(translated_event, sort_keys=True, separators=(",", ":")))
            return

        translated_event = {
            "type": "translated_transcript",
            "note_id": self.note.note_id,
            "segment_id": segment_id,
            "source_text": result.source_text or source_text,
            "text": result.text,
            "translation_model": result.translation_model,
            "translation_status": "complete",
            "context_line_count": result.context_line_count,
            "latency_ms": result.latency_ms if result.latency_ms is not None else _elapsed_ms(started),
        }
        self.collector.add_event(translated_event)
        async with self.send_lock:
            await self.websocket.send_text(json.dumps(translated_event, sort_keys=True, separators=(",", ":")))


class RealtimeTranscriptLineNormalizer:
    """Splits long transcript events into stable browser-visible line events."""

    def __init__(self, *, max_words_per_line: int = 18, max_chars_per_line: int = 140) -> None:
        self.max_words_per_line = max(4, max_words_per_line)
        self.max_chars_per_line = max(40, max_chars_per_line)
        self._finalized_text_by_segment_id: dict[str, str] = {}

    def observe_event(self, event: dict[str, Any]) -> list[dict[str, Any]]:
        event_type = event.get("type")
        if event_type not in {"partial_transcript", "final_transcript"}:
            return [event]
        source_segment_id = str(event.get("segment_id") or "")
        text = str(event.get("text") or "").strip()
        if not source_segment_id or not text:
            return []

        chunks = _transcript_line_chunks(
            text,
            max_words_per_line=self.max_words_per_line,
            max_chars_per_line=self.max_chars_per_line,
        )
        if not chunks:
            return []

        final_chunk_count = len(chunks)
        if event_type == "partial_transcript" and not _ends_with_sentence_boundary(text):
            final_chunk_count = max(0, len(chunks) - 1)

        normalized_events: list[dict[str, Any]] = []
        for index, chunk in enumerate(chunks):
            is_final = event_type == "final_transcript" or index < final_chunk_count
            line_event = self._line_event(event, source_segment_id, index, chunk, is_final=is_final)
            line_segment_id = str(line_event["segment_id"])
            if is_final:
                if self._finalized_text_by_segment_id.get(line_segment_id) == chunk:
                    continue
                self._finalized_text_by_segment_id[line_segment_id] = chunk
            normalized_events.append(line_event)
        return normalized_events

    @staticmethod
    def _line_event(
        event: dict[str, Any],
        source_segment_id: str,
        index: int,
        text: str,
        *,
        is_final: bool,
    ) -> dict[str, Any]:
        line_event = dict(event)
        line_event["type"] = "final_transcript" if is_final else "partial_transcript"
        line_event["segment_id"] = _line_segment_id(source_segment_id, index)
        line_event["text"] = text
        line_event["is_final"] = is_final
        line_event["is_partial"] = not is_final
        return line_event


async def handle_recording_socket(
    websocket: WebSocket,
    *,
    user: User,
    settings: Settings,
    store: NoteStore,
    active_sessions: ActiveSessionRegistry,
    access_validator: AccessValidator | None = None,
    text_intelligence_client: TextIntelligenceClient | None = None,
) -> None:
    acquired = await active_sessions.acquire(user, settings.max_concurrent_sessions_per_user)
    if not acquired:
        await websocket.close(code=4409, reason="too_many_sessions")
        return
    try:
        await _handle_recording_socket(
            websocket,
            user=user,
            settings=settings,
            store=store,
            access_validator=access_validator,
            text_intelligence_client=text_intelligence_client,
        )
    finally:
        await active_sessions.release(user)


async def _handle_recording_socket(
    websocket: WebSocket,
    *,
    user: User,
    settings: Settings,
    store: NoteStore,
    access_validator: AccessValidator | None = None,
    text_intelligence_client: TextIntelligenceClient | None = None,
) -> None:
    await websocket.accept()
    try:
        raw_start = await asyncio.wait_for(websocket.receive_text(), timeout=15)
        start = _parse_start(raw_start)
    except Exception as exc:
        await websocket.send_text(_event("error", code="bad_start", message="invalid recording start"))
        await websocket.close(code=4400, reason="bad_start")
        logger.info("recording_start_rejected", extra={"user_id": user.user_id, "error": type(exc).__name__})
        return

    note = Note.create(
        user=user,
        language_mode=start["language_mode"],
        diarization_enabled=start["diarization_enabled"],
    )
    store.create_note(note)
    store.record_audit(user, "recording_started", {"note_id": note.note_id})
    await websocket.send_text(
        _event(
            "recording_started",
            note_id=note.note_id,
            language_mode=note.language_mode,
            diarization_enabled=note.diarization_enabled,
        )
    )

    collector = TranscriptCollector()
    try:
        if settings.mock_transcription:
            await _mock_recording_loop(
                websocket,
                note=note,
                user=user,
                settings=settings,
                store=store,
                collector=collector,
                access_validator=access_validator,
                text_intelligence_client=text_intelligence_client,
            )
        else:
            await _upstream_recording_loop(
                websocket,
                note=note,
                user=user,
                settings=settings,
                store=store,
                collector=collector,
                access_validator=access_validator,
                text_intelligence_client=text_intelligence_client,
            )
    except RecordingAccessRevoked as exc:
        logger.warning("recording_access_revoked", extra={"user_id": user.user_id, "note_id": note.note_id})
        segments = collector.final_segments()
        store.finalize_note(user, note, segments, status="failed")
        store.record_audit(user, "recording_failed", {"note_id": note.note_id, "error_class": type(exc).__name__})
        with suppress(Exception):
            await websocket.send_text(
                _event("error", code="access_revoked", message="VoiceNotes access was revoked")
            )
            await websocket.close(code=4401, reason="access_revoked")
    except Exception as exc:
        logger.exception("recording_session_failed", extra={"user_id": user.user_id, "note_id": note.note_id})
        segments = collector.final_segments()
        store.finalize_note(user, note, segments, status="failed")
        store.record_audit(user, "recording_failed", {"note_id": note.note_id, "error_class": type(exc).__name__})
        with suppress(Exception):
            await websocket.send_text(_event("error", code="recording_failed", message=type(exc).__name__))
            await websocket.close(code=1011, reason="recording_failed")


async def _mock_recording_loop(
    websocket: WebSocket,
    *,
    note: Note,
    user: User,
    settings: Settings,
    store: NoteStore,
    collector: TranscriptCollector,
    access_validator: AccessValidator | None = None,
    text_intelligence_client: TextIntelligenceClient | None = None,
) -> None:
    audio_frames = 0
    emitted_partial = False
    next_access_check = 0.0
    send_lock = asyncio.Lock()
    transcript_lines = RealtimeTranscriptLineNormalizer()
    translations = RealtimeTranslationCoordinator(
        client=text_intelligence_client,
        note=note,
        settings=settings,
        collector=collector,
        websocket=websocket,
        send_lock=send_lock,
    )
    while True:
        message = await websocket.receive()
        if message.get("type") == "websocket.disconnect":
            store.finalize_note(user, note, collector.final_segments(), status="failed")
            store.record_audit(user, "recording_disconnected", {"note_id": note.note_id})
            return
        next_access_check = await _check_recording_access(
            user,
            access_validator,
            next_access_check,
            settings.oidc_recording_access_check_seconds,
        )
        if message.get("bytes") is not None:
            audio_frames += 1
            if not emitted_partial and audio_frames >= 2:
                event = {
                    "type": "partial_transcript",
                    "note_id": note.note_id,
                    "segment_id": "mock_partial",
                    "text": "Listening...",
                    "is_final": False,
                    "start_time_ms": 0,
                }
                await websocket.send_text(json.dumps(event, sort_keys=True, separators=(",", ":")))
                emitted_partial = True
            continue
        text_payload = message.get("text")
        if not text_payload:
            continue
        try:
            payload = json.loads(text_payload)
        except json.JSONDecodeError:
            continue
        if payload.get("type") == "stop_recording":
            segment = TranscriptSegment(
                segment_id=new_id("seg"),
                text="Voice note captured in local mock mode.",
                start_time_ms=0,
                end_time_ms=max(1000, audio_frames * 128),
                speaker_id="SPEAKER_00" if note.diarization_enabled else None,
                is_final=True,
            )
            collector.segments[segment.segment_id] = segment
            collector.order.append(segment.segment_id)
            final_event = {
                "type": "final_transcript",
                "note_id": note.note_id,
                **segment.to_dict(),
            }
            translations.observe_transcript_event(final_event)
            async with send_lock:
                await websocket.send_text(json.dumps(final_event, sort_keys=True, separators=(",", ":")))
            await translations.drain()
            finalized = store.finalize_note(user, note, collector.final_segments(), status="complete")
            store.record_audit(user, "recording_stopped", {"note_id": note.note_id})
            _schedule_transcript_summary(
                user=user,
                note=finalized,
                settings=settings,
                store=store,
                segments=collector.final_segments(),
                text_intelligence_client=text_intelligence_client,
            )
            await websocket.send_text(
                _event(
                    "recording_stopped",
                    note_id=note.note_id,
                    status=finalized.status,
                    duration_ms=finalized.duration_ms,
                )
            )
            await websocket.close(code=1000)
            return


async def _upstream_recording_loop(
    websocket: WebSocket,
    *,
    note: Note,
    user: User,
    settings: Settings,
    store: NoteStore,
    collector: TranscriptCollector,
    access_validator: AccessValidator | None = None,
    text_intelligence_client: TextIntelligenceClient | None = None,
) -> None:
    token = _upstream_authorization_token(settings, user)
    headers = {"Authorization": f"Bearer {token}"}
    upstream = await _connect_upstream(settings.upstream_transcription_url, headers)
    upstream_closed = asyncio.Event()
    send_lock = asyncio.Lock()
    transcript_lines = RealtimeTranscriptLineNormalizer()
    translations = RealtimeTranslationCoordinator(
        client=text_intelligence_client,
        note=note,
        settings=settings,
        collector=collector,
        websocket=websocket,
        send_lock=send_lock,
    )

    async def relay_upstream() -> None:
        try:
            async for raw in upstream:
                try:
                    event = json.loads(raw)
                except json.JSONDecodeError:
                    continue
                for normalized_event in transcript_lines.observe_event(event):
                    collector.add_event(normalized_event)
                    browser_event = _browser_event(note, normalized_event)
                    if browser_event:
                        translations.observe_transcript_event(browser_event)
                        async with send_lock:
                            await websocket.send_text(
                                json.dumps(browser_event, sort_keys=True, separators=(",", ":"))
                            )
                if event.get("type") == "session_stopped":
                    upstream_closed.set()
                    return
        finally:
            upstream_closed.set()

    relay_task = asyncio.create_task(relay_upstream())
    next_access_check = 0.0
    try:
        await upstream.send(json.dumps(_upstream_start_session(note), sort_keys=True, separators=(",", ":")))
        while True:
            message = await asyncio.wait_for(websocket.receive(), timeout=settings.max_recording_seconds)
            if message.get("type") == "websocket.disconnect":
                with suppress(Exception):
                    await upstream.close()
                store.finalize_note(user, note, collector.final_segments(), status="failed")
                store.record_audit(user, "recording_disconnected", {"note_id": note.note_id})
                return
            next_access_check = await _check_recording_access(
                user,
                access_validator,
                next_access_check,
                settings.oidc_recording_access_check_seconds,
            )
            if message.get("bytes") is not None:
                await upstream.send(message["bytes"])
                continue
            text_payload = message.get("text")
            if not text_payload:
                continue
            try:
                payload = json.loads(text_payload)
            except json.JSONDecodeError:
                continue
            if payload.get("type") == "stop_recording":
                await upstream.send(json.dumps({"type": "stop_session"}, sort_keys=True, separators=(",", ":")))
                with suppress(asyncio.TimeoutError):
                    await asyncio.wait_for(upstream_closed.wait(), timeout=45)
                await translations.drain()
                finalized = store.finalize_note(user, note, collector.final_segments(), status="complete")
                store.record_audit(user, "recording_stopped", {"note_id": note.note_id})
                _schedule_transcript_summary(
                    user=user,
                    note=finalized,
                    settings=settings,
                    store=store,
                    segments=collector.final_segments(),
                    text_intelligence_client=text_intelligence_client,
                )
                await websocket.send_text(
                    _event(
                        "recording_stopped",
                        note_id=note.note_id,
                        status=finalized.status,
                        duration_ms=finalized.duration_ms,
                    )
                )
                await websocket.close(code=1000)
                return
    finally:
        relay_task.cancel()
        with suppress(Exception):
            await upstream.close()
        with suppress(asyncio.CancelledError):
            await relay_task


async def _check_recording_access(
    user: User,
    access_validator: AccessValidator | None,
    next_check_at: float,
    interval_seconds: int,
) -> float:
    if access_validator is None:
        return next_check_at
    now = asyncio.get_running_loop().time()
    if now < next_check_at:
        return next_check_at
    try:
        await access_validator(user)
    except Exception as exc:
        raise RecordingAccessRevoked("recording access revoked") from exc
    return now + max(1.0, float(interval_seconds))


def _schedule_transcript_summary(
    *,
    user: User,
    note: Note,
    settings: Settings,
    store: NoteStore,
    segments: list[TranscriptSegment],
    text_intelligence_client: TextIntelligenceClient | None,
) -> None:
    if text_intelligence_client is None or not settings.text_intelligence_summary_enabled:
        return
    lines = [" ".join(segment.text.split()) for segment in segments if segment.text.strip()]
    if not lines:
        return
    asyncio.create_task(
        _generate_transcript_summary(
            user=user,
            note_id=note.note_id,
            store=store,
            lines=lines,
            text_intelligence_client=text_intelligence_client,
        )
    )


async def _generate_transcript_summary(
    *,
    user: User,
    note_id: str,
    store: NoteStore,
    lines: list[str],
    text_intelligence_client: TextIntelligenceClient,
) -> None:
    try:
        result = await text_intelligence_client.summarize_transcript(note_id=note_id, lines=lines)
        store.update_note_intelligence(
            user,
            note_id,
            summary=result.summary,
            action_items=result.action_items,
            decisions=result.decisions,
            open_questions=result.open_questions,
            generated_title=result.generated_title,
            summary_model=result.model,
            status="complete",
        )
        store.record_audit(user, "summary_generated", {"note_id": note_id, "model": result.model or ""})
    except Exception as exc:
        store.update_note_intelligence(
            user,
            note_id,
            summary=None,
            action_items=[],
            decisions=[],
            open_questions=[],
            generated_title=None,
            summary_model=None,
            status="failed",
        )
        store.record_audit(user, "summary_failed", {"note_id": note_id, "error_class": type(exc).__name__})


async def _connect_upstream(url: str, headers: dict[str, str]):
    try:
        return await websockets.connect(url, additional_headers=headers, max_size=2**22)
    except TypeError:
        return await websockets.connect(url, extra_headers=headers, max_size=2**22)


def _upstream_authorization_token(settings: Settings, user: User) -> str:
    if settings.upstream_transcription_signing_secret:
        return _mint_upstream_transcription_token(settings, user)
    if settings.upstream_transcription_token:
        return settings.upstream_transcription_token
    raise RecordingError("upstream transcription authorization is not configured")


def _mint_upstream_transcription_token(settings: Settings, user: User) -> str:
    issued_at = datetime.now(timezone.utc)
    ttl_seconds = max(60, settings.upstream_transcription_token_ttl_seconds)
    expires_at = issued_at + timedelta(seconds=ttl_seconds)
    claims: dict[str, Any] = {
        "iss": settings.upstream_transcription_token_issuer,
        "aud": settings.upstream_transcription_token_audience,
        "sub": user.user_id,
        "email": user.email,
        "iat": int(issued_at.timestamp()),
        "nbf": int(issued_at.timestamp()),
        "exp": int(expires_at.timestamp()),
        "scope": settings.upstream_transcription_token_scope,
        "jti": secrets.token_urlsafe(18),
    }
    header = {"alg": "HS256", "typ": "JWT"}
    header_b64 = _base64url_encode(json.dumps(header, sort_keys=True, separators=(",", ":")).encode("utf-8"))
    payload_b64 = _base64url_encode(json.dumps(claims, sort_keys=True, separators=(",", ":")).encode("utf-8"))
    signing_input = f"{header_b64}.{payload_b64}".encode("ascii")
    signature = hmac.new(
        settings.upstream_transcription_signing_secret.encode("utf-8"),
        signing_input,
        hashlib.sha256,
    ).digest()
    return f"{header_b64}.{payload_b64}.{_base64url_encode(signature)}"


def _base64url_encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _parse_start(raw_start: str) -> dict[str, Any]:
    payload = json.loads(raw_start)
    if payload.get("type") != "start_recording":
        raise ValueError("expected start_recording")
    language_mode = str(payload.get("language_mode") or "multilingual_to_english")
    if language_mode not in SUPPORTED_LANGUAGE_MODES:
        raise ValueError("unsupported language mode")
    return {
        "language_mode": language_mode,
        "diarization_enabled": False,
    }


def _upstream_start_session(note: Note) -> dict[str, Any]:
    return {
        "type": "start_session",
        "protocol_version": "transcription.v1",
        "session_id": note.note_id,
        "transcription_enabled": True,
        "diarization_enabled": note.diarization_enabled,
        "language_mode": note.language_mode,
        "sample_rate": 16000,
        "channel_count": 1,
        "audio_encoding": "pcm_s16le",
        "client_timestamp": utc_now(),
        "app_version": "voicenotes-web-0.1",
        "privacy_safe_device_id": "voicenotes-web",
    }


def _browser_event(note: Note, event: dict[str, Any]) -> dict[str, Any] | None:
    event_type = event.get("type")
    if event_type in {"session_started", "diarization_status"}:
        return {"type": event_type, "note_id": note.note_id, **_safe_metadata(event)}
    if event_type in {"partial_transcript", "final_transcript"}:
        browser_event = {"note_id": note.note_id, **event}
        if not note.diarization_enabled:
            browser_event.pop("speaker_id", None)
        return browser_event
    if event_type == "speaker_update":
        if not note.diarization_enabled:
            return None
        return {"note_id": note.note_id, **event}
    if event_type == "error":
        return {"type": "error", "note_id": note.note_id, "code": event.get("code"), "message": event.get("message")}
    return None


def _safe_metadata(event: dict[str, Any]) -> dict[str, Any]:
    denied = {"text", "token", "authorization", "raw_audio"}
    return {str(key): value for key, value in event.items() if key not in denied}


def _event(event_type: str, **payload: Any) -> str:
    return json.dumps({"type": event_type, "created_at": utc_now(), **payload}, sort_keys=True, separators=(",", ":"))


def _elapsed_ms(started: float) -> int:
    return max(0, int((asyncio.get_running_loop().time() - started) * 1000))


def _line_segment_id(source_segment_id: str, index: int) -> str:
    return f"{source_segment_id}-line-{index + 1}"


_SENTENCE_BOUNDARY_RE = re.compile(r"[.!?।॥。！？]\s*$")
_CJK_TEXT_RE = re.compile(r"[\u3040-\u30ff\u3400-\u9fff\uff66-\uff9f]")
_COMPACT_SCRIPT_RE = re.compile(r"[\u0900-\u097f\u3040-\u30ff\u3400-\u9fff\uff66-\uff9f]")
_LATIN_OR_DIGIT_RE = re.compile(r"[A-Za-z0-9]")


def _transcript_line_chunks(
    text: str,
    *,
    max_words_per_line: int = 18,
    max_chars_per_line: int = 140,
) -> list[str]:
    normalized = re.sub(r"\s+", " ", text).strip()
    if not normalized:
        return []

    sentence_chunks = [
        match.group(0).strip()
        for match in re.finditer(r".+?(?:[.!?।॥]+(?=\s|$)|[。！？]+|$)", normalized)
        if match.group(0).strip()
    ]
    chunks: list[str] = []
    for sentence in sentence_chunks or [normalized]:
        chunks.extend(
            _split_long_transcript_line(
                sentence,
                max_words_per_line=max_words_per_line,
                max_chars_per_line=max_chars_per_line,
            )
        )
    return [chunk for chunk in chunks if chunk]


def _split_long_transcript_line(
    text: str,
    *,
    max_words_per_line: int,
    max_chars_per_line: int,
) -> list[str]:
    char_limit = _effective_max_chars_per_line(text, max_chars_per_line=max_chars_per_line)
    words = text.split()
    if len(words) <= 1:
        return _split_by_chars(text, max_chars_per_line=char_limit)

    chunks: list[str] = []
    current: list[str] = []
    for word in words:
        if len(word) > char_limit:
            if current:
                chunks.append(" ".join(current))
                current = []
            chunks.extend(_split_by_chars(word, max_chars_per_line=char_limit))
            continue
        candidate = " ".join([*current, word])
        if current and (len(current) >= max_words_per_line or len(candidate) > char_limit):
            chunks.append(" ".join(current))
            current = [word]
        else:
            current.append(word)
    if current:
        chunks.append(" ".join(current))
    return chunks


def _split_by_chars(text: str, *, max_chars_per_line: int) -> list[str]:
    if len(text) <= max_chars_per_line:
        return [text]
    return [text[index : index + max_chars_per_line].strip() for index in range(0, len(text), max_chars_per_line)]


def _effective_max_chars_per_line(text: str, *, max_chars_per_line: int) -> int:
    if _contains_compact_script(text) and (_contains_cjk(text) or _mostly_unspaced_text(text)):
        return min(max_chars_per_line, 72)
    return max_chars_per_line


def _contains_cjk(text: str) -> bool:
    return bool(_CJK_TEXT_RE.search(text))


def _contains_compact_script(text: str) -> bool:
    return bool(_COMPACT_SCRIPT_RE.search(text))


def _mostly_unspaced_text(text: str) -> bool:
    compact = re.sub(r"\s+", "", text)
    if not compact:
        return False
    wordish = len(_LATIN_OR_DIGIT_RE.findall(compact))
    return wordish / max(1, len(compact)) < 0.35


def _ends_with_sentence_boundary(text: str) -> bool:
    return bool(_SENTENCE_BOUNDARY_RE.search(text.strip()))
