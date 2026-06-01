from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Iterable


@dataclass(frozen=True)
class TranscriptTiming:
    segment_id: str
    start_time_ms: int
    end_time_ms: int | None


@dataclass(frozen=True)
class SpeakerAssignment:
    segment_id: str
    speaker_id: str
    start_time_ms: int
    end_time_ms: int | None
    overlap_ms: int
    is_final: bool = True


def assign_speakers_to_segments(
    segments: Iterable[TranscriptTiming],
    speaker_turns: Iterable[Any],
) -> list[SpeakerAssignment]:
    assignments: list[SpeakerAssignment] = []
    turns = sorted(speaker_turns, key=lambda turn: (int(turn.start_time_ms), int(turn.end_time_ms)))
    for segment in segments:
        best_turn = _best_turn_for_segment(segment, turns)
        if best_turn is None:
            continue
        overlap_ms = _overlap_ms(segment, best_turn)
        assignments.append(
            SpeakerAssignment(
                segment_id=segment.segment_id,
                speaker_id=str(best_turn.speaker_id),
                start_time_ms=segment.start_time_ms,
                end_time_ms=segment.end_time_ms,
                overlap_ms=overlap_ms,
            )
        )
    return assignments


def _best_turn_for_segment(segment: TranscriptTiming, turns: list[Any]) -> Any | None:
    if not turns:
        return None
    ranked = sorted(
        ((turn, _overlap_ms(segment, turn)) for turn in turns),
        key=lambda item: (item[1], -abs(int(item[0].start_time_ms) - segment.start_time_ms)),
        reverse=True,
    )
    if ranked[0][1] > 0:
        return ranked[0][0]
    segment_midpoint = _midpoint(segment.start_time_ms, _segment_end(segment, turns))
    return min(turns, key=lambda turn: abs(_midpoint(int(turn.start_time_ms), int(turn.end_time_ms)) - segment_midpoint))


def _overlap_ms(segment: TranscriptTiming, turn: Any) -> int:
    segment_end = segment.end_time_ms
    if segment_end is None:
        segment_end = int(turn.end_time_ms)
    start = max(segment.start_time_ms, int(turn.start_time_ms))
    end = min(segment_end, int(turn.end_time_ms))
    return max(0, end - start)


def _segment_end(segment: TranscriptTiming, turns: list[Any]) -> int:
    if segment.end_time_ms is not None:
        return segment.end_time_ms
    return max(int(turn.end_time_ms) for turn in turns)


def _midpoint(start_time_ms: int, end_time_ms: int) -> int:
    return start_time_ms + max(0, end_time_ms - start_time_ms) // 2
