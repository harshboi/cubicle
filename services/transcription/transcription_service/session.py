"""Per-connection transcription session runtime and event orchestration."""

from __future__ import annotations

from collections import deque
from dataclasses import dataclass, field
import json
import logging
import math
import re
from typing import Deque, Protocol

from .alignment import TranscriptTiming, assign_speakers_to_segments
from .asr import ASRProvider, MockASRProvider
from .auth import AuthContext
from .diarization import (
    DiarizationProvider,
    DiarizationProviderError,
    MockDiarizationProvider,
)
from .protocol import (
    AudioFrameError,
    StartSessionConfig,
    encode_event,
    speaker_update_event,
    validate_audio_frame,
)
from .logging_utils import log_event


class BackpressureError(RuntimeError):
    """Raised when a session receives audio faster than it can process."""

    pass


logger = logging.getLogger(__name__)


class SessionEventSink(Protocol):
    """Async text sink for encoded transcription events."""

    async def send_text(self, payload: str) -> None:
        ...


@dataclass
class TranscriptionSession:
    """State machine for one live transcription WebSocket session."""

    config: StartSessionConfig
    auth_context: AuthContext = field(default_factory=lambda: AuthContext(mode="none"))
    asr_provider: ASRProvider = field(default_factory=MockASRProvider)
    diarization_provider: DiarizationProvider = field(default_factory=MockDiarizationProvider)
    max_pending_audio_frames: int = 8
    _pending_audio: Deque[bytes] = field(default_factory=deque)
    _audio_buffer: bytearray = field(default_factory=bytearray)
    _started: bool = False
    _stopped: bool = False
    _diarization_available: bool = True
    _speaker_labeling_available: bool = True
    _diarization_failure_error_class: str | None = None
    _received_audio_frames: int = 0
    _received_audio_bytes: int = 0

    def start_events(self) -> list[str]:
        self.asr_provider.warmup()
        asr_status = self.asr_provider.runtime_status()
        diarization_status = self.diarization_provider.runtime_status()
        if self.config.diarization_enabled:
            try:
                self.diarization_provider.warmup()
                diarization_status = self.diarization_provider.runtime_status()
                self._diarization_available = True
                self._speaker_labeling_available = bool(
                    diarization_status.get("speaker_labeling_available", True)
                )
                self._diarization_failure_error_class = None
            except DiarizationProviderError as exc:
                self._diarization_available = False
                self._speaker_labeling_available = False
                self._diarization_failure_error_class = _diarization_provider_error_class(exc)
                diarization_status = {
                    **self.diarization_provider.runtime_status(),
                    "ready": False,
                    "warmed": False,
                }
        else:
            self._speaker_labeling_available = False
        self._started = True
        events = [
            encode_event(
                "session_started",
                session_id=self.config.session_id,
                authenticated_user_id=self.auth_context.audit_user,
                auth_mode=self.auth_context.mode,
                protocol_version=self.config.protocol_version,
                language_mode=self.config.language_mode,
                diarization_enabled=self.config.diarization_enabled,
                model_route=self.config.model_route,
                asr_provider=asr_status.get("provider"),
                model_name=asr_status.get("model_name"),
                model_version=asr_status.get("model_version"),
                device=asr_status.get("device"),
                compute_type=asr_status.get("compute_type"),
                gpu_available=asr_status.get("gpu_available"),
                gpu_required=asr_status.get("gpu_required"),
                model_warmed=asr_status.get("warmed"),
                diarization_provider=diarization_status.get("provider"),
                diarization_model_name=diarization_status.get("model_name"),
                diarization_model_version=diarization_status.get("model_version"),
                diarization_ready=diarization_status.get("ready"),
                diarization_warmed=diarization_status.get("warmed"),
                diarization_speaker_labeling_available=self._speaker_labeling_available,
                alignment_strategy="segment_time_overlap",
            )
        ]
        if self.config.diarization_enabled:
            status = "enabled" if self._diarization_available and self._speaker_labeling_available else "unavailable"
            if not self._diarization_available:
                status = "failed"
            status_payload = dict(
                session_id=self.config.session_id,
                status=status,
                provider=diarization_status.get("provider"),
                model_name=diarization_status.get("model_name"),
                model_version=diarization_status.get("model_version"),
                alignment_strategy="segment_time_overlap",
            )
            if not self._diarization_available:
                status_payload["error_class"] = self._diarization_failure_error_class or "DiarizationProviderError"
            elif not self._speaker_labeling_available:
                status_payload["reason"] = _diarization_unavailable_reason(diarization_status.get("provider"))
            events.append(encode_event("diarization_status", **status_payload))
        else:
            events.append(
                encode_event(
                    "diarization_status",
                    session_id=self.config.session_id,
                    status="disabled",
                )
            )
        return events

    def receive_audio_frame(self, frame: bytes) -> list[str]:
        if not self._started:
            raise AudioFrameError("session has not started")
        if self._stopped:
            raise AudioFrameError("session is stopped")
        validate_audio_frame(frame)
        if len(self._pending_audio) >= self.max_pending_audio_frames:
            raise BackpressureError("audio backpressure limit exceeded")
        self._pending_audio.append(frame)
        current_frame = self._pending_audio.popleft()
        self._audio_buffer.extend(current_frame)
        self._received_audio_frames += 1
        self._received_audio_bytes += len(current_frame)
        if self._received_audio_frames == 1 or self._received_audio_frames % 10 == 0:
            log_event(
                logger,
                "session_audio_frame",
                session_id=self.config.session_id,
                frame_count=self._received_audio_frames,
                frame_bytes=len(current_frame),
                total_pcm_bytes=self._received_audio_bytes,
                audio_ms=self._audio_duration_ms(),
            )
        return self.asr_provider.ingest_audio(self.config, current_frame)

    def stop_events(self) -> list[str]:
        final_events = self.stop_transcript_events()
        if not final_events:
            return []
        speaker_events = self._speaker_update_events(final_events)
        final_events = _final_events_with_speaker_updates(final_events, speaker_events)
        return [
            *final_events,
            *speaker_events,
            self.session_stopped_event(),
        ]

    def stop_transcript_events(self) -> list[str]:
        if self._stopped:
            return []
        self._stopped = True
        final_events = self.asr_provider.finalize(self.config)
        return _split_final_transcript_events_for_diarization(
            final_events,
            enabled=self.config.diarization_enabled and self._diarization_available and self._speaker_labeling_available,
        )

    def stop_diarization_events(self, final_events: list[str]) -> list[str]:
        return self._speaker_update_events(final_events)

    def stop_processing_events(self, final_events: list[str]) -> list[str]:
        if (
            not self.config.diarization_enabled
            or not self._diarization_available
            or not self._speaker_labeling_available
        ):
            return []
        if not final_events:
            return []
        return [
            encode_event(
                "diarization_status",
                session_id=self.config.session_id,
                status="processing",
                alignment_strategy="segment_time_overlap",
            )
        ]

    def session_stopped_event(self) -> str:
        return encode_event("session_stopped", session_id=self.config.session_id)

    def usage_metrics(self) -> dict[str, int]:
        return {
            "audio_bytes": len(self._audio_buffer),
            "audio_ms": self._audio_duration_ms(),
        }

    def _speaker_update_events(self, final_events: list[str]) -> list[str]:
        if not self.config.diarization_enabled:
            return []
        if not self._diarization_available:
            return [
                encode_event(
                    "diarization_status",
                    session_id=self.config.session_id,
                    status="failed",
                    error_class=self._diarization_failure_error_class or "DiarizationProviderError",
                )
            ]
        if not self._speaker_labeling_available:
            status = self.diarization_provider.runtime_status()
            return [
                encode_event(
                    "diarization_status",
                    session_id=self.config.session_id,
                    status="unavailable",
                    provider=status.get("provider"),
                    reason=_diarization_unavailable_reason(status.get("provider")),
                )
            ]
        try:
            turns = self.diarization_provider.diarize(self.config, bytes(self._audio_buffer))
            segments = _final_segment_timings(final_events, default_end_time_ms=self._audio_duration_ms())
            assignments = assign_speakers_to_segments(segments, turns)
            return [
                *[
                    speaker_update_event(
                        segment_id=assignment.segment_id,
                        speaker_id=assignment.speaker_id,
                        start_time_ms=assignment.start_time_ms,
                        end_time_ms=assignment.end_time_ms,
                        overlap_ms=assignment.overlap_ms,
                        is_final=assignment.is_final,
                    )
                    for assignment in assignments
                ],
                encode_event(
                    "diarization_status",
                    session_id=self.config.session_id,
                    status="completed",
                    speaker_count=len({assignment.speaker_id for assignment in assignments}),
                    speaker_turn_count=len(turns),
                    segment_count=len(assignments),
                    audio_ms=self._audio_duration_ms(),
                ),
            ]
        except DiarizationProviderError as exc:
            return [
                encode_event(
                    "diarization_status",
                    session_id=self.config.session_id,
                    status="failed",
                    error_class=_diarization_provider_error_class(exc),
                )
            ]

    def _audio_duration_ms(self) -> int:
        sample_count = len(self._audio_buffer) // 2
        return int(sample_count * 1000 / self.config.sample_rate)


def decode_client_text_message(payload: str) -> tuple[str, dict]:
    decoded = json.loads(payload)
    if not isinstance(decoded, dict):
        raise ValueError("client message must be a JSON object")
    message_type = decoded.get("type")
    if not isinstance(message_type, str):
        raise ValueError("client message type is required")
    return message_type, decoded


def _diarization_provider_error_class(exc: DiarizationProviderError) -> str:
    cause = exc.__cause__
    if cause is not None:
        return type(cause).__name__
    return type(exc).__name__


def _diarization_unavailable_reason(provider: object) -> str:
    if provider == "mock":
        return "mock_provider"
    if provider in {"off", "disabled", "none"}:
        return "provider_disabled"
    return "speaker_labeling_unavailable"


def _final_segment_timings(final_events: list[str], *, default_end_time_ms: int) -> list[TranscriptTiming]:
    timings: list[TranscriptTiming] = []
    for event in final_events:
        try:
            payload = json.loads(event)
        except json.JSONDecodeError:
            continue
        if payload.get("type") != "final_transcript":
            continue
        segment_id = payload.get("segment_id")
        start_time_ms = payload.get("start_time_ms")
        if not isinstance(segment_id, str) or not isinstance(start_time_ms, int):
            continue
        end_time_ms = payload.get("end_time_ms")
        if not isinstance(end_time_ms, int):
            end_time_ms = max(default_end_time_ms, start_time_ms + 1)
        timings.append(
            TranscriptTiming(
                segment_id=segment_id,
                start_time_ms=start_time_ms,
                end_time_ms=end_time_ms,
            )
        )
    return timings


def _final_events_with_speaker_updates(final_events: list[str], speaker_events: list[str]) -> list[str]:
    speaker_ids_by_segment: dict[str, str] = {}
    for event in speaker_events:
        try:
            payload = json.loads(event)
        except json.JSONDecodeError:
            continue
        if payload.get("type") != "speaker_update":
            continue
        segment_id = payload.get("segment_id")
        speaker_id = payload.get("speaker_id")
        if isinstance(segment_id, str) and isinstance(speaker_id, str) and speaker_id.strip():
            speaker_ids_by_segment[segment_id] = speaker_id

    if not speaker_ids_by_segment:
        return final_events

    updated_events: list[str] = []
    for event in final_events:
        try:
            payload = json.loads(event)
        except json.JSONDecodeError:
            updated_events.append(event)
            continue
        segment_id = payload.get("segment_id")
        if payload.get("type") == "final_transcript" and isinstance(segment_id, str):
            speaker_id = speaker_ids_by_segment.get(segment_id)
            if speaker_id:
                payload["speaker_id"] = speaker_id
                updated_events.append(json.dumps(payload, sort_keys=True, separators=(",", ":")))
                continue
        updated_events.append(event)
    return updated_events


def _split_final_transcript_events_for_diarization(final_events: list[str], *, enabled: bool) -> list[str]:
    if not enabled:
        return final_events

    split_events: list[str] = []
    changed = False
    for event in final_events:
        payload = _decode_event_object(event)
        if payload is None or payload.get("type") != "final_transcript":
            split_events.append(event)
            continue

        text = payload.get("text")
        segment_id = payload.get("segment_id")
        start_time_ms = payload.get("start_time_ms")
        end_time_ms = payload.get("end_time_ms")
        if (
            not isinstance(text, str)
            or not isinstance(segment_id, str)
            or not isinstance(start_time_ms, int)
            or not isinstance(end_time_ms, int)
        ):
            split_events.append(event)
            continue

        chunks = _sentence_chunks(text)
        if len(chunks) <= 1 or end_time_ms <= start_time_ms:
            split_events.append(event)
            continue

        changed = True
        for index, chunk in enumerate(_timed_sentence_chunks(chunks, start_time_ms=start_time_ms, end_time_ms=end_time_ms)):
            next_payload = dict(payload)
            next_payload["segment_id"] = segment_id if index == 0 else f"{segment_id}-part-{index + 1}"
            next_payload["text"] = chunk["text"]
            next_payload["start_time_ms"] = chunk["start_time_ms"]
            next_payload["end_time_ms"] = chunk["end_time_ms"]
            split_events.append(json.dumps(next_payload, sort_keys=True, separators=(",", ":")))

    return split_events if changed else final_events


def _decode_event_object(event: str) -> dict | None:
    try:
        payload = json.loads(event)
    except json.JSONDecodeError:
        return None
    return payload if isinstance(payload, dict) else None


def _sentence_chunks(text: str) -> list[str]:
    normalized = re.sub(r"\s+", " ", text).strip()
    if not normalized:
        return []

    chunks = [
        match.group(0).strip()
        for match in re.finditer(r".+?(?:[.!?।॥]+(?=\s|$)|$)", normalized)
        if match.group(0).strip()
    ]
    if len(chunks) <= 1:
        chunks = [part.strip() for part in re.split(r"\s+(?=(?:and|but|so|then|yeah|yes|no)\b)", normalized, flags=re.IGNORECASE) if part.strip()]
    return _merge_short_chunks(chunks)


def _merge_short_chunks(chunks: list[str], *, min_chars: int = 24) -> list[str]:
    merged: list[str] = []
    pending = ""
    for chunk in chunks:
        candidate = f"{pending} {chunk}".strip() if pending else chunk
        if len(candidate) < min_chars:
            pending = candidate
            continue
        merged.append(candidate)
        pending = ""
    if pending:
        if merged:
            merged[-1] = f"{merged[-1]} {pending}".strip()
        else:
            merged.append(pending)
    return merged


def _timed_sentence_chunks(chunks: list[str], *, start_time_ms: int, end_time_ms: int) -> list[dict]:
    duration_ms = max(1, end_time_ms - start_time_ms)
    total_weight = max(1, sum(len(chunk) for chunk in chunks))
    timed: list[dict] = []
    cursor = start_time_ms
    for index, chunk in enumerate(chunks):
        if index == len(chunks) - 1:
            chunk_end = end_time_ms
        else:
            proportional_duration = duration_ms * len(chunk) / total_weight
            chunk_end = min(end_time_ms, cursor + max(1, math.floor(proportional_duration)))
        timed.append({"text": chunk, "start_time_ms": cursor, "end_time_ms": chunk_end})
        cursor = chunk_end
    return timed
