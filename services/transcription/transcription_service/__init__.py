"""AWS-backed transcription service scaffold for Cubicle."""

from .auth import AuthError, TokenAuthenticator
from .protocol import (
    AudioFrameError,
    SessionConfigError,
    StartSessionConfig,
    decode_start_session,
    encode_event,
    validate_audio_frame,
)
from .session import SessionEventSink, TranscriptionSession

__all__ = [
    "AudioFrameError",
    "AuthError",
    "SessionConfigError",
    "SessionEventSink",
    "StartSessionConfig",
    "TokenAuthenticator",
    "TranscriptionSession",
    "decode_start_session",
    "encode_event",
    "validate_audio_frame",
]
