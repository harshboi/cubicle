"""ASR provider abstractions and local/remote transcription adapters."""

from __future__ import annotations

import asyncio
import base64
from contextlib import suppress
from dataclasses import dataclass, field
import importlib.util
import json
import logging
import os
from queue import Empty, Queue
import tempfile
from threading import Thread
import time
from typing import Any, Protocol
import wave

from .logging_utils import log_event
from .protocol import StartSessionConfig, transcript_event


logger = logging.getLogger(__name__)


class ASRProviderError(RuntimeError):
    """ASR runtime is unsupported, missing dependencies, or failed."""

    pass


class ASRProvider(Protocol):
    """Provider boundary for live audio ingestion and final transcripts."""

    def warmup(self) -> None:
        """Load heavy runtime dependencies before frames arrive."""

        ...

    def ingest_audio(self, config: StartSessionConfig, audio_frame: bytes) -> list[str]:
        """Accept one PCM frame and return zero or more encoded events."""

        ...

    def finalize(self, config: StartSessionConfig) -> list[str]:
        """Flush provider state and return final encoded events."""

        ...

    def runtime_status(self) -> dict[str, Any]:
        """Return operator-safe readiness metadata."""

        ...


class VoxtralRealtimeStream(Protocol):
    """Minimal stream contract used by Voxtral ASR providers."""

    def start(self) -> None:
        """Open the realtime stream before audio is sent."""

        ...

    def send_audio(self, audio_frame: bytes) -> None:
        """Queue one normalized audio frame for streaming."""

        ...

    def drain_text_deltas(self) -> list[str]:
        """Return currently available text deltas without blocking."""

        ...

    def stop(self) -> None:
        """Close the stream and surface any background failure."""

        ...


@dataclass(frozen=True)
class ASRProviderSettings:
    """Environment-backed settings for ASR and realtime stream adapters."""

    provider: str = "mock"
    voxtral_model: str = "voxtral-mini-transcribe-realtime-2602"
    voxtral_model_version: str = "mistral-realtime-2602"
    voxtral_target_streaming_delay_ms: int = 480
    voxtral_runtime: str = "mistral_managed"
    vllm_base_url: str = "http://127.0.0.1:8000"
    voxtral_realtime_url: str = "ws://127.0.0.1:8000/v1/realtime"
    voxtral_realtime_auth_token: str | None = None
    voxtral_final_response_timeout_seconds: float = 30.0
    voxtral_input_rms_target: float = 0.20
    voxtral_input_rms_floor: float = 0.008
    voxtral_input_max_gain: float = 24.0
    voxtral_input_peak_ceiling: float = 0.92
    mistral_api_key: str | None = None
    mistral_base_url: str = "wss://api.mistral.ai"
    model_cache_dir: str = "/models"
    models_offline: bool = False
    model_name: str = "large-v3-turbo"
    model_version: str = "faster-whisper"
    device: str = "cuda"
    compute_type: str = "float16"
    cpu_threads: int = 4
    require_gpu: bool = False
    partial_min_audio_ms: int = 1_000

    @classmethod
    def from_environment(cls) -> "ASRProviderSettings":
        """Load ASR settings from process environment variables."""

        vllm_base_url = _env_first("VLLM_BASE_URL", default="http://127.0.0.1:8000").strip()
        return cls(
            provider=os.environ.get("TRANSCRIPTION_ASR_PROVIDER", "mock").strip().lower(),
            voxtral_model=_env_first(
                "TRANSCRIPTION_VOXTRAL_MODEL",
                "VLLM_MODEL",
                default="voxtral-mini-transcribe-realtime-2602",
            ).strip(),
            voxtral_model_version=os.environ.get(
                "TRANSCRIPTION_VOXTRAL_MODEL_VERSION", "mistral-realtime-2602"
            ).strip(),
            voxtral_target_streaming_delay_ms=_env_int("TRANSCRIPTION_VOXTRAL_TARGET_DELAY_MS", 480),
            voxtral_runtime=_env_first("TRANSCRIPTION_VOXTRAL_RUNTIME", "VLLM_RUNTIME", default="mistral_managed")
            .strip()
            .lower(),
            vllm_base_url=vllm_base_url,
            voxtral_realtime_url=_env_first(
                "TRANSCRIPTION_VOXTRAL_REALTIME_URL",
                "VLLM_REALTIME_URL",
                default=_vllm_realtime_url(vllm_base_url),
            ).strip(),
            voxtral_realtime_auth_token=os.environ.get("TRANSCRIPTION_VOXTRAL_REALTIME_TOKEN"),
            voxtral_final_response_timeout_seconds=_env_float(
                "TRANSCRIPTION_VOXTRAL_FINAL_RESPONSE_TIMEOUT_SECONDS",
                _env_float("VLLM_FINAL_RESPONSE_TIMEOUT_SECONDS", 30.0),
            ),
            voxtral_input_rms_target=_env_float(
                "TRANSCRIPTION_VOXTRAL_INPUT_RMS_TARGET",
                _env_float("VLLM_INPUT_RMS_TARGET", 0.20),
            ),
            voxtral_input_rms_floor=_env_float(
                "TRANSCRIPTION_VOXTRAL_INPUT_RMS_FLOOR",
                _env_float("VLLM_INPUT_RMS_FLOOR", 0.008),
            ),
            voxtral_input_max_gain=_env_float(
                "TRANSCRIPTION_VOXTRAL_INPUT_MAX_GAIN",
                _env_float("VLLM_INPUT_MAX_GAIN", 24.0),
            ),
            voxtral_input_peak_ceiling=_env_float(
                "TRANSCRIPTION_VOXTRAL_INPUT_PEAK_CEILING",
                _env_float("VLLM_INPUT_PEAK_CEILING", 0.92),
            ),
            mistral_api_key=os.environ.get("MISTRAL_API_KEY"),
            mistral_base_url=os.environ.get("MISTRAL_BASE_URL", "wss://api.mistral.ai").strip(),
            model_cache_dir=os.environ.get("TRANSCRIPTION_MODEL_CACHE_DIR", "/models").strip(),
            models_offline=_env_bool("TRANSCRIPTION_MODELS_OFFLINE", False),
            model_name=os.environ.get("TRANSCRIPTION_WHISPER_MODEL", "large-v3-turbo").strip(),
            model_version=os.environ.get("TRANSCRIPTION_WHISPER_MODEL_VERSION", "faster-whisper").strip(),
            device=os.environ.get("TRANSCRIPTION_WHISPER_DEVICE", "cuda").strip().lower(),
            compute_type=os.environ.get("TRANSCRIPTION_WHISPER_COMPUTE_TYPE", "float16").strip(),
            cpu_threads=_env_int("TRANSCRIPTION_WHISPER_CPU_THREADS", 4),
            require_gpu=_env_bool("TRANSCRIPTION_REQUIRE_GPU", False),
            partial_min_audio_ms=_env_int("TRANSCRIPTION_PARTIAL_MIN_AUDIO_MS", 1_000),
        )


@dataclass
class ASRProviderFactory:
    """Creates the configured ASR provider and shares model caches."""

    settings: ASRProviderSettings = field(default_factory=ASRProviderSettings.from_environment)
    model_cache: "FasterWhisperModelCache" = field(default_factory=lambda: default_model_cache)
    voxtral_stream_factory: "VoxtralRealtimeStreamFactory | None" = None

    def create_provider(self) -> ASRProvider:
        """Instantiate the provider selected by settings."""

        if self.settings.provider == "mock":
            return MockASRProvider()
        if self.settings.provider in {"voxtral", "voxtral_realtime"}:
            return VoxtralRealtimeASRProvider(
                settings=self.settings,
                stream_factory=self.voxtral_stream_factory or VoxtralRealtimeStreamFactory(self.settings),
            )
        if self.settings.provider in {"voxtral_self_hosted", "voxtral_vllm", "voxtral_local"}:
            return SelfHostedVoxtralRealtimeASRProvider(
                settings=self.settings,
                stream_factory=self.voxtral_stream_factory or SelfHostedVoxtralRealtimeStreamFactory(self.settings),
            )
        if self.settings.provider == "faster_whisper":
            return FasterWhisperASRProvider(settings=self.settings, model_cache=self.model_cache)
        raise ASRProviderError(f"unsupported ASR provider: {self.settings.provider}")

    def runtime_status(self) -> dict[str, Any]:
        """Return provider readiness without mutating session state."""

        if self.settings.provider == "mock":
            return MockASRProvider().runtime_status()
        if self.settings.provider in {"voxtral", "voxtral_realtime"}:
            return VoxtralRealtimeASRProvider(
                settings=self.settings,
                stream_factory=self.voxtral_stream_factory or VoxtralRealtimeStreamFactory(self.settings),
            ).runtime_status()
        if self.settings.provider in {"voxtral_self_hosted", "voxtral_vllm", "voxtral_local"}:
            return SelfHostedVoxtralRealtimeASRProvider(
                settings=self.settings,
                stream_factory=self.voxtral_stream_factory or SelfHostedVoxtralRealtimeStreamFactory(self.settings),
            ).runtime_status()
        if self.settings.provider == "faster_whisper":
            return FasterWhisperASRProvider(settings=self.settings, model_cache=self.model_cache).runtime_status()
        return {"provider": self.settings.provider, "ready": False, "error": "unsupported_provider"}


@dataclass
class MockASRProvider:
    """Deterministic transcript provider for local development and tests."""

    model_name: str = "mock-asr"
    model_version: str = "slice-4"
    _frame_count: int = 0
    _total_bytes: int = 0
    _warmed: bool = False

    def warmup(self) -> None:
        """Mark the mock provider as initialized."""

        self._warmed = True

    @property
    def is_warmed(self) -> bool:
        """Expose warmup state for tests without parsing status JSON."""

        return self._warmed

    def runtime_status(self) -> dict[str, Any]:
        """Return stable mock readiness metadata."""

        return {
            "provider": "mock",
            "model_name": self.model_name,
            "model_version": self.model_version,
            "device": "cpu",
            "compute_type": "none",
            "gpu_available": detect_gpu_available(),
            "gpu_required": False,
            "warmed": self._warmed,
            "ready": True,
        }

    def ingest_audio(self, config: StartSessionConfig, audio_frame: bytes) -> list[str]:
        """Emit a synthetic partial transcript after each frame."""

        self._frame_count += 1
        self._total_bytes += len(audio_frame)
        text = self._text_for(config, final=False)
        return [
            transcript_event(
                "partial_transcript",
                segment_id=f"{config.session_id}-segment-1",
                start_time_ms=0,
                end_time_ms=None,
                text=text,
                is_final=False,
                speaker_id=None,
                language_mode=config.language_mode,
                model_name=self.model_name,
                model_version=self.model_version,
                confidence=None,
            )
        ]

    def finalize(self, config: StartSessionConfig) -> list[str]:
        """Emit one synthetic final transcript when audio was received."""

        if self._frame_count == 0:
            return []
        text = self._text_for(config, final=True)
        return [
            transcript_event(
                "final_transcript",
                segment_id=f"{config.session_id}-segment-1",
                start_time_ms=0,
                end_time_ms=max(500, min(5_000, self._total_bytes // 16)),
                text=text,
                is_final=True,
                speaker_id=None,
                language_mode=config.language_mode,
                model_name=self.model_name,
                model_version=self.model_version,
                confidence=0.90,
            )
        ]

    def _text_for(self, config: StartSessionConfig, *, final: bool) -> str:
        if config.language_mode == "japanese_to_english":
            return "Mock translated English transcript." if final else "Mock translated English"
        if config.language_mode == "multilingual_to_english":
            return "Mock multilingual transcript." if final else "Mock multilingual"
        return "Mock English transcript." if final else "Mock English"


@dataclass
class VoxtralRealtimeStreamFactory:
    """Factory for the managed Mistral realtime stream adapter."""

    settings: ASRProviderSettings

    def create_stream(self) -> VoxtralRealtimeStream:
        """Create a managed Mistral Voxtral stream."""

        return MistralVoxtralRealtimeStream(self.settings)


@dataclass
class SelfHostedVoxtralRealtimeStreamFactory:
    """Factory for the self-hosted vLLM realtime stream adapter."""

    settings: ASRProviderSettings

    def create_stream(self) -> VoxtralRealtimeStream:
        """Create a VPC-local vLLM Voxtral stream."""

        return VLLMVoxtralRealtimeStream(self.settings)


@dataclass
class VoxtralRealtimeASRProvider:
    """Managed Mistral Voxtral adapter that converts deltas to events."""

    settings: ASRProviderSettings = field(default_factory=ASRProviderSettings.from_environment)
    stream_factory: VoxtralRealtimeStreamFactory = field(
        default_factory=lambda: VoxtralRealtimeStreamFactory(ASRProviderSettings.from_environment())
    )
    _stream: VoxtralRealtimeStream | None = None
    _transcript_text: str = ""
    _audio_bytes: int = 0
    _warmed: bool = False

    def warmup(self) -> None:
        """Validate credentials and open the realtime stream."""

        self._validate_runtime()
        self._stream = self.stream_factory.create_stream()
        self._stream.start()
        self._warmed = True

    def runtime_status(self) -> dict[str, Any]:
        """Expose managed API readiness and key configuration state."""

        dependencies_available = _optional_dependency_available("mistralai")
        api_key_present = bool(self.settings.mistral_api_key and self.settings.mistral_api_key.strip())
        return {
            "provider": "voxtral_realtime",
            "model_name": self.settings.voxtral_model,
            "model_version": self.settings.voxtral_model_version,
            "device": "mistral_realtime_api",
            "compute_type": "managed",
            "dependencies_available": dependencies_available,
            "api_key_configured": api_key_present,
            "base_url": self.settings.mistral_base_url,
            "target_streaming_delay_ms": self.settings.voxtral_target_streaming_delay_ms,
            "final_response_timeout_seconds": self.settings.voxtral_final_response_timeout_seconds,
            "gpu_available": None,
            "gpu_required": False,
            "warmed": self._warmed,
            "ready": dependencies_available and api_key_present,
        }

    def ingest_audio(self, config: StartSessionConfig, audio_frame: bytes) -> list[str]:
        """Forward one frame and return any available partial text."""

        if self._stream is None:
            raise ASRProviderError("Voxtral Realtime stream has not started")
        self._audio_bytes += len(audio_frame)
        self._stream.send_audio(audio_frame)
        return self._events_for_deltas(config, is_final=False)

    def finalize(self, config: StartSessionConfig) -> list[str]:
        """Stop streaming and return the latest final transcript event."""

        if self._stream is None:
            return []
        self._stream.stop()
        events = self._events_for_deltas(config, is_final=True)
        if events:
            return events
        if not self._transcript_text.strip():
            return []
        return [self._transcript_event(config, is_final=True)]

    def _validate_runtime(self) -> None:
        if not self.settings.mistral_api_key or not self.settings.mistral_api_key.strip():
            raise ASRProviderError("MISTRAL_API_KEY is required for TRANSCRIPTION_ASR_PROVIDER=voxtral_realtime")
        if not _optional_dependency_available("mistralai"):
            raise ASRProviderError(
                "mistralai realtime SDK is not installed; install requirements-voxtral.txt "
                "or use TRANSCRIPTION_ASR_PROVIDER=faster_whisper/mock"
            )

    def _events_for_deltas(self, config: StartSessionConfig, *, is_final: bool) -> list[str]:
        assert self._stream is not None
        deltas = self._stream.drain_text_deltas()
        if deltas:
            log_event(
                logger,
                "asr_text_deltas",
                provider=self.runtime_status().get("provider"),
                session_id=config.session_id,
                delta_count=len(deltas),
                delta_chars=sum(len(delta) for delta in deltas),
                final=is_final,
                transcript_chars=len(self._transcript_text),
            )
        if not deltas and not is_final:
            return []
        self._transcript_text = _merge_realtime_text_fragments(self._transcript_text, deltas)
        if not self._transcript_text:
            return []
        return [self._transcript_event(config, is_final=is_final)]

    def _transcript_event(self, config: StartSessionConfig, *, is_final: bool) -> str:
        return transcript_event(
            "final_transcript" if is_final else "partial_transcript",
            segment_id=f"{config.session_id}-segment-1",
            start_time_ms=0,
            end_time_ms=self._duration_ms() if is_final else None,
            text=self._transcript_text,
            is_final=is_final,
            speaker_id=None,
            language_mode=config.language_mode,
            model_name=self.settings.voxtral_model,
            model_version=self.settings.voxtral_model_version,
            confidence=None,
        )

    def _duration_ms(self) -> int:
        sample_count = self._audio_bytes // 2
        return int(sample_count * 1000 / 16_000)


@dataclass
class SelfHostedVoxtralRealtimeASRProvider(VoxtralRealtimeASRProvider):
    """Self-hosted Voxtral adapter for OpenAI Realtime-compatible vLLM."""

    stream_factory: SelfHostedVoxtralRealtimeStreamFactory = field(
        default_factory=lambda: SelfHostedVoxtralRealtimeStreamFactory(ASRProviderSettings.from_environment())
    )

    def runtime_status(self) -> dict[str, Any]:
        """Expose vLLM endpoint, auth, and dependency readiness."""

        dependencies_available = _optional_dependency_available("websockets")
        endpoint_configured = bool(self.settings.voxtral_realtime_url and self.settings.voxtral_realtime_url.strip())
        return {
            "provider": "voxtral_self_hosted",
            "model_name": self.settings.voxtral_model,
            "model_version": self.settings.voxtral_model_version,
            "device": "cuda",
            "compute_type": "vllm",
            "runtime": self.settings.voxtral_runtime,
            "vllm_base_url_configured": bool(
                self.settings.vllm_base_url
                and self.settings.vllm_base_url.strip()
            ),
            "realtime_url_configured": endpoint_configured,
            "realtime_auth_configured": bool(
                self.settings.voxtral_realtime_auth_token
                and self.settings.voxtral_realtime_auth_token.strip()
            ),
            "external_api_dependency": False,
            "dependencies_available": dependencies_available,
            "target_streaming_delay_ms": self.settings.voxtral_target_streaming_delay_ms,
            "final_response_timeout_seconds": self.settings.voxtral_final_response_timeout_seconds,
            "model_cache_dir": self.settings.model_cache_dir,
            "models_offline": self.settings.models_offline,
            "gpu_available": detect_gpu_available(),
            "gpu_required": self.settings.require_gpu,
            "warmed": self._warmed,
            "ready": dependencies_available and endpoint_configured,
        }

    def _validate_runtime(self) -> None:
        if not self.settings.voxtral_realtime_url or not self.settings.voxtral_realtime_url.strip():
            raise ASRProviderError(
                "TRANSCRIPTION_VOXTRAL_REALTIME_URL is required for "
                "TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted"
            )
        if not _optional_dependency_available("websockets"):
            raise ASRProviderError(
                "websockets is not installed; install requirements.txt before using "
                "TRANSCRIPTION_ASR_PROVIDER=voxtral_self_hosted"
            )

    def _events_for_deltas(self, config: StartSessionConfig, *, is_final: bool) -> list[str]:
        assert self._stream is not None
        deltas = self._stream.drain_text_deltas()
        if deltas:
            log_event(
                logger,
                "asr_text_deltas",
                provider="voxtral_self_hosted",
                session_id=config.session_id,
                delta_count=len(deltas),
                delta_chars=sum(len(delta) for delta in deltas),
                final=is_final,
                transcript_chars=len(self._transcript_text),
            )
        if not deltas and not is_final:
            return []
        self._transcript_text = _merge_realtime_text_fragments(self._transcript_text, deltas)
        if not self._transcript_text:
            return []
        return [self._transcript_event(config, is_final=is_final)]

    def _transcript_event(self, config: StartSessionConfig, *, is_final: bool) -> str:
        return transcript_event(
            "final_transcript" if is_final else "partial_transcript",
            segment_id=f"{config.session_id}-segment-1",
            start_time_ms=0,
            end_time_ms=self._duration_ms() if is_final else None,
            text=self._transcript_text,
            is_final=is_final,
            speaker_id=None,
            language_mode=config.language_mode,
            model_name=self.settings.voxtral_model,
            model_version=self.settings.voxtral_model_version,
            confidence=None,
        )

    def _duration_ms(self) -> int:
        sample_count = self._audio_bytes // 2
        return int(sample_count * 1000 / 16_000)


class MistralVoxtralRealtimeStream:
    """Threaded bridge from sync audio calls to Mistral's async SDK."""

    def __init__(self, settings: ASRProviderSettings) -> None:
        self.settings = settings
        self._audio_queue: Queue[bytes | None] = Queue(maxsize=64)
        self._text_queue: Queue[str] = Queue()
        self._error_queue: Queue[BaseException] = Queue()
        self._thread: Thread | None = None

    def start(self) -> None:
        """Start the background async stream runner."""

        self._thread = Thread(target=self._run_thread, name="voxtral-realtime-stream", daemon=True)
        self._thread.start()

    def send_audio(self, audio_frame: bytes) -> None:
        """Queue one PCM frame or raise a prior stream failure."""

        self._raise_if_failed()
        self._audio_queue.put(audio_frame, timeout=2)

    def drain_text_deltas(self) -> list[str]:
        """Drain available SDK text deltas without blocking the caller."""

        self._raise_if_failed()
        deltas: list[str] = []
        while True:
            try:
                deltas.append(self._text_queue.get_nowait())
            except Empty:
                return deltas

    def stop(self) -> None:
        """Signal end-of-audio and wait for final SDK events."""

        self._audio_queue.put(None, timeout=2)
        if self._thread is not None:
            join_timeout = self._stop_join_timeout_seconds()
            self._thread.join(timeout=join_timeout)
            if self._thread.is_alive():
                logger.warning(
                    "managed voxtral realtime stream thread did not finish within %.1fs",
                    join_timeout,
                )
        self._raise_if_failed()

    def _stop_join_timeout_seconds(self) -> float:
        return max(10.0, float(self.settings.voxtral_final_response_timeout_seconds) + 5.0)

    def _run_thread(self) -> None:
        try:
            asyncio.run(self._run())
        except BaseException as exc:
            self._error_queue.put(exc)

    async def _run(self) -> None:
        from mistralai.client import Mistral
        from mistralai.client.models import (
            AudioFormat,
            RealtimeTranscriptionError,
            TranscriptionStreamDone,
            TranscriptionStreamTextDelta,
        )
        from mistralai.extra.realtime import UnknownRealtimeEvent

        client = Mistral(api_key=self.settings.mistral_api_key, server_url=self.settings.mistral_base_url)
        audio_format = AudioFormat(encoding="pcm_s16le", sample_rate=16_000)
        async for event in client.audio.realtime.transcribe_stream(
            audio_stream=self._audio_stream(),
            model=self.settings.voxtral_model,
            audio_format=audio_format,
            target_streaming_delay_ms=self.settings.voxtral_target_streaming_delay_ms,
        ):
            if isinstance(event, TranscriptionStreamTextDelta):
                self._text_queue.put(event.text)
            elif isinstance(event, TranscriptionStreamDone):
                break
            elif isinstance(event, RealtimeTranscriptionError):
                self._error_queue.put(ASRProviderError(str(event.error)))
                break
            elif isinstance(event, UnknownRealtimeEvent):
                continue

    async def _audio_stream(self):
        loop = asyncio.get_running_loop()
        while True:
            frame = await loop.run_in_executor(None, self._audio_queue.get)
            if frame is None:
                break
            yield frame

    def _raise_if_failed(self) -> None:
        try:
            error = self._error_queue.get_nowait()
        except Empty:
            return
        raise ASRProviderError(str(error)) from error


class VLLMVoxtralRealtimeStream:
    """Adapter for a VPC-local vLLM/OpenAI Realtime Voxtral runtime."""

    def __init__(self, settings: ASRProviderSettings) -> None:
        self.settings = settings
        self._audio_queue: Queue[bytes | None] = Queue(maxsize=64)
        self._text_queue: Queue[str] = Queue()
        self._error_queue: Queue[BaseException] = Queue()
        self._thread: Thread | None = None
        self._generation_started = False
        self._generation_idle_event: asyncio.Event | None = None
        self._generation_started_at_monotonic: float | None = None
        self._last_delta_at_monotonic: float | None = None
        self._final_commit_sent = False

    def start(self) -> None:
        """Start the background websocket runner."""

        self._thread = Thread(target=self._run_thread, name="voxtral-vllm-realtime-stream", daemon=True)
        self._thread.start()

    def send_audio(self, audio_frame: bytes) -> None:
        """Queue one PCM frame or raise a prior websocket failure."""

        self._raise_if_failed()
        self._audio_queue.put(audio_frame, timeout=2)

    def drain_text_deltas(self) -> list[str]:
        """Drain available realtime deltas without blocking the caller."""

        self._raise_if_failed()
        deltas: list[str] = []
        while True:
            try:
                deltas.append(self._text_queue.get_nowait())
            except Empty:
                return deltas

    def stop(self) -> None:
        """Signal final commit and wait for completion or timeout."""

        self._audio_queue.put(None, timeout=2)
        if self._thread is not None:
            join_timeout = self._stop_join_timeout_seconds()
            self._thread.join(timeout=join_timeout)
            if self._thread.is_alive():
                logger.warning(
                    "vllm realtime stream thread did not finish within %.1fs",
                    join_timeout,
                )
        self._raise_if_failed()

    def _stop_join_timeout_seconds(self) -> float:
        return max(10.0, float(self.settings.voxtral_final_response_timeout_seconds) + 5.0)

    def _run_thread(self) -> None:
        try:
            asyncio.run(self._run())
        except BaseException as exc:
            self._error_queue.put(exc)

    async def _run(self) -> None:
        import websockets

        headers: dict[str, str] = {}
        if self.settings.voxtral_realtime_auth_token and self.settings.voxtral_realtime_auth_token.strip():
            headers["Authorization"] = f"Bearer {self.settings.voxtral_realtime_auth_token}"

        async with websockets.connect(
            self.settings.voxtral_realtime_url,
            extra_headers=headers or None,
            max_size=None,
        ) as websocket:
            self._generation_idle_event = asyncio.Event()
            self._generation_idle_event.set()
            await websocket.send(json.dumps(self._session_update_payload()))
            receiver = asyncio.create_task(self._receive_events(websocket))
            try:
                await self._send_audio_frames(websocket)
                await self._wait_for_final_events(receiver)
            finally:
                if not receiver.done():
                    receiver.cancel()
                    with suppress(asyncio.CancelledError):
                        await receiver

    async def _wait_for_final_events(self, receiver: "asyncio.Task[None]") -> None:
        timeout = max(0.1, float(self.settings.voxtral_final_response_timeout_seconds))
        try:
            await asyncio.wait_for(receiver, timeout=timeout)
        except asyncio.TimeoutError:
            logger.warning(
                "vllm realtime stream timed out waiting for final transcript events after %.1fs",
                timeout,
            )
            receiver.cancel()
            with suppress(asyncio.CancelledError):
                await receiver

    def _session_update_payload(self) -> dict[str, Any]:
        return {
            "type": "session.update",
            "model": self.settings.voxtral_model,
            "modalities": ["text"],
            "input_audio_format": "pcm16",
            "transcription_delay_ms": self.settings.voxtral_target_streaming_delay_ms,
            "session": {
                "model": self.settings.voxtral_model,
                "modalities": ["text"],
                "input_audio_format": "pcm16",
                "transcription_delay_ms": self.settings.voxtral_target_streaming_delay_ms,
            },
        }

    async def _send_audio_frames(self, websocket: Any) -> None:
        loop = asyncio.get_running_loop()
        pending_audio_bytes = 0
        commit_threshold_bytes = 32_000
        stale_recovery_threshold_bytes = 8_000
        appended_frames = 0
        appended_pcm_bytes = 0
        while True:
            frame = await loop.run_in_executor(None, self._audio_queue.get)
            if frame is None:
                break
            frame, normalization = self._normalize_audio_frame(frame)
            await websocket.send(
                json.dumps(
                    {
                        "type": "input_audio_buffer.append",
                        "audio": base64.b64encode(frame).decode("ascii"),
                    }
                )
            )
            appended_frames += 1
            appended_pcm_bytes += len(frame)
            pending_audio_bytes += len(frame)
            if appended_frames == 1 or appended_frames % 10 == 0:
                log_event(
                    logger,
                    "vllm_audio_frames_queued",
                    frame_count=appended_frames,
                    total_pcm_bytes=appended_pcm_bytes,
                    pending_pcm_bytes=pending_audio_bytes,
                    generation_started=self._generation_started,
                    input_rms=round(normalization["input_rms"], 4),
                    input_peak=round(normalization["input_peak"], 4),
                    output_rms=round(normalization["output_rms"], 4),
                    output_peak=round(normalization["output_peak"], 4),
                    applied_input_gain=round(normalization["applied_gain"], 3),
                    model_name=self.settings.voxtral_model,
                )
            if pending_audio_bytes >= commit_threshold_bytes or (
                self._generation_started
                and pending_audio_bytes >= stale_recovery_threshold_bytes
                and self._generation_has_stalled()
            ):
                reopen_reason = self._generation_reopen_reason(pending_audio_bytes)
                if reopen_reason:
                    log_event(
                        logger,
                        "vllm_generation_window_reopened",
                        reason=reopen_reason,
                        pending_pcm_bytes=pending_audio_bytes,
                        stale_after_seconds=self._generation_stale_threshold_seconds(),
                        max_window_seconds=self._generation_max_window_seconds(),
                        model_name=self.settings.voxtral_model,
                    )
                    self._mark_generation_finished()
                if self._generation_started:
                    continue
                await self._commit_audio_buffer(websocket, pending_audio_bytes=pending_audio_bytes)
                pending_audio_bytes = 0
        await self._wait_for_generation_idle()
        await self._commit_audio_buffer(websocket, final=True, pending_audio_bytes=pending_audio_bytes)

    async def _commit_audio_buffer(
        self,
        websocket: Any,
        *,
        final: bool = False,
        pending_audio_bytes: int = 0,
    ) -> None:
        if final:
            self._final_commit_sent = True
            if not self._generation_started:
                self._mark_generation_started()
                log_event(
                    logger,
                    "vllm_audio_commit",
                    final=False,
                    pending_pcm_bytes=pending_audio_bytes,
                    generation_started=self._generation_started,
                    model_name=self.settings.voxtral_model,
                )
                await websocket.send(json.dumps({"type": "input_audio_buffer.commit"}))
            log_event(
                logger,
                "vllm_audio_commit",
                final=True,
                pending_pcm_bytes=pending_audio_bytes,
                generation_started=self._generation_started,
                model_name=self.settings.voxtral_model,
            )
            await websocket.send(json.dumps({"type": "input_audio_buffer.commit", "final": True}))
            return

        if self._generation_started:
            log_event(
                logger,
                "vllm_audio_commit_skipped",
                final=False,
                pending_pcm_bytes=pending_audio_bytes,
                generation_started=True,
                model_name=self.settings.voxtral_model,
            )
            return
        self._mark_generation_started()
        log_event(
            logger,
            "vllm_audio_commit",
            final=False,
            pending_pcm_bytes=pending_audio_bytes,
            generation_started=self._generation_started,
            model_name=self.settings.voxtral_model,
        )
        await websocket.send(json.dumps({"type": "input_audio_buffer.commit"}))

    async def _wait_for_generation_idle(self) -> None:
        if not self._generation_started or self._generation_idle_event is None:
            return
        timeout = min(5.0, max(0.1, float(self.settings.voxtral_final_response_timeout_seconds)))
        with suppress(asyncio.TimeoutError):
            await asyncio.wait_for(self._generation_idle_event.wait(), timeout=timeout)

    def _mark_generation_started(self) -> None:
        self._generation_started = True
        self._generation_started_at_monotonic = time.monotonic()
        self._last_delta_at_monotonic = None
        if self._generation_idle_event is not None:
            self._generation_idle_event.clear()

    def _mark_generation_finished(self) -> None:
        self._generation_started = False
        self._generation_started_at_monotonic = None
        self._last_delta_at_monotonic = None
        if self._generation_idle_event is not None:
            self._generation_idle_event.set()

    def _generation_stale_threshold_seconds(self) -> float:
        return max(1.0, min(5.0, self.settings.voxtral_target_streaming_delay_ms / 1_000.0 * 6.0))

    def _generation_max_window_seconds(self) -> float:
        return max(6.0, self._generation_stale_threshold_seconds() * 3.0)

    def _generation_reopen_reason(self, pending_audio_bytes: int) -> str | None:
        if not self._generation_started or self._generation_started_at_monotonic is None:
            return None
        if self._generation_has_stalled():
            return "stale_generation_window"
        if pending_audio_bytes < 32_000:
            return None
        active_seconds = time.monotonic() - self._generation_started_at_monotonic
        if active_seconds >= self._generation_max_window_seconds():
            return "max_generation_window"
        return None

    def _generation_has_stalled(self) -> bool:
        if not self._generation_started or self._generation_started_at_monotonic is None:
            return False
        last_activity = self._last_delta_at_monotonic or self._generation_started_at_monotonic
        return (time.monotonic() - last_activity) >= self._generation_stale_threshold_seconds()

    def _normalize_audio_frame(self, frame: bytes) -> tuple[bytes, dict[str, float]]:
        input_rms, input_peak = _pcm16_rms_peak(frame)
        target_rms = _clamp_float(self.settings.voxtral_input_rms_target, 0.01, 0.60)
        rms_floor = _clamp_float(self.settings.voxtral_input_rms_floor, 0.0, target_rms)
        max_gain = max(1.0, self.settings.voxtral_input_max_gain)
        peak_ceiling = _clamp_float(self.settings.voxtral_input_peak_ceiling, 0.10, 0.99)
        gain = 1.0
        if input_rms >= rms_floor and 0.0 < input_rms < target_rms:
            rms_gain = target_rms / input_rms
            peak_gain = peak_ceiling / input_peak if input_peak > 0 else max_gain
            gain = max(1.0, min(rms_gain, peak_gain, max_gain))
        normalized = _scale_pcm16(frame, gain)
        output_rms, output_peak = _pcm16_rms_peak(normalized)
        return normalized, {
            "input_rms": input_rms,
            "input_peak": input_peak,
            "output_rms": output_rms,
            "output_peak": output_peak,
            "applied_gain": gain,
        }

    async def _receive_events(self, websocket: Any) -> None:
        async for message in websocket:
            if isinstance(message, bytes):
                continue
            try:
                payload = json.loads(message)
            except json.JSONDecodeError:
                continue
            error = _extract_realtime_error(payload)
            if error is not None:
                self._error_queue.put(ASRProviderError(error))
                return
            delta = _extract_realtime_text_delta(payload)
            if delta:
                self._last_delta_at_monotonic = time.monotonic()
                self._text_queue.put(delta)
                log_event(
                    logger,
                    "vllm_text_delta",
                    delta_chars=len(delta),
                    model_name=self.settings.voxtral_model,
                )
            if _is_realtime_response_window_done(payload):
                self._mark_generation_finished()
                log_event(
                    logger,
                    "vllm_response_window_done",
                    event_type=payload.get("type"),
                    model_name=self.settings.voxtral_model,
                )
            if self._final_commit_sent and _is_realtime_done(payload):
                log_event(
                    logger,
                    "vllm_realtime_done",
                    event_type=payload.get("type"),
                    model_name=self.settings.voxtral_model,
                )
                return

    def _raise_if_failed(self) -> None:
        try:
            error = self._error_queue.get_nowait()
        except Empty:
            return
        raise ASRProviderError(str(error)) from error


class FasterWhisperModelCache:
    """Process cache for faster-whisper models keyed by runtime settings."""

    def __init__(self) -> None:
        self._models: dict[tuple[str, str, str, int], Any] = {}

    def load_model(self, settings: ASRProviderSettings) -> Any:
        """Load or reuse a faster-whisper model for the active settings."""

        cache_key = (settings.model_name, settings.device, settings.compute_type, settings.cpu_threads)
        model = self._models.get(cache_key)
        if model is not None:
            return model

        try:
            from faster_whisper import WhisperModel
        except ImportError as exc:
            raise ASRProviderError(
                "faster-whisper is not installed; install requirements-asr.txt or use TRANSCRIPTION_ASR_PROVIDER=mock"
            ) from exc

        model = WhisperModel(
            settings.model_name,
            device=settings.device,
            compute_type=settings.compute_type,
            cpu_threads=settings.cpu_threads,
        )
        self._models[cache_key] = model
        return model


default_model_cache = FasterWhisperModelCache()


@dataclass
class FasterWhisperASRProvider:
    """Batch-style faster-whisper adapter behind the live ASR interface."""

    settings: ASRProviderSettings = field(default_factory=ASRProviderSettings.from_environment)
    model_cache: FasterWhisperModelCache = field(default_factory=lambda: default_model_cache)
    _audio_buffer: bytearray = field(default_factory=bytearray)
    _last_partial_text: str = ""
    _warmed: bool = False

    def warmup(self) -> None:
        """Validate GPU policy and load the faster-whisper model."""

        self._validate_runtime()
        self.model_cache.load_model(self.settings)
        self._warmed = True

    @property
    def sample_rate(self) -> int:
        """Server ASR sample rate expected by the wire protocol."""

        return 16_000

    def runtime_status(self) -> dict[str, Any]:
        """Expose dependency, GPU, and model-cache readiness."""

        gpu_available = detect_gpu_available()
        dependencies_available = _optional_dependency_available("faster_whisper")
        gpu_ready = self.settings.device != "cuda" or gpu_available or not self.settings.require_gpu
        ready = dependencies_available and gpu_ready
        return {
            "provider": "faster_whisper",
            "model_name": self.settings.model_name,
            "model_version": self.settings.model_version,
            "device": self.settings.device,
            "compute_type": self.settings.compute_type,
            "dependencies_available": dependencies_available,
            "gpu_available": gpu_available,
            "gpu_required": self.settings.require_gpu,
            "warmed": self._warmed,
            "ready": ready,
        }

    def ingest_audio(self, config: StartSessionConfig, audio_frame: bytes) -> list[str]:
        """Buffer PCM and periodically emit partial transcripts."""

        self._audio_buffer.extend(audio_frame)
        if self._buffer_duration_ms() < self.settings.partial_min_audio_ms:
            return []

        text, _confidence = self._transcribe_buffer(config, final=False)
        if not text or text == self._last_partial_text:
            return []
        self._last_partial_text = text
        return [
            transcript_event(
                "partial_transcript",
                segment_id=f"{config.session_id}-segment-1",
                start_time_ms=0,
                end_time_ms=self._buffer_duration_ms(),
                text=text,
                is_final=False,
                speaker_id=None,
                language_mode=config.language_mode,
                model_name=self.settings.model_name,
                model_version=self.settings.model_version,
                confidence=None,
            )
        ]

    def finalize(self, config: StartSessionConfig) -> list[str]:
        """Transcribe the buffered audio and emit one final event."""

        if not self._audio_buffer:
            return []
        text, confidence = self._transcribe_buffer(config, final=True)
        if not text:
            return []
        return [
            transcript_event(
                "final_transcript",
                segment_id=f"{config.session_id}-segment-1",
                start_time_ms=0,
                end_time_ms=self._buffer_duration_ms(),
                text=text,
                is_final=True,
                speaker_id=None,
                language_mode=config.language_mode,
                model_name=self.settings.model_name,
                model_version=self.settings.model_version,
                confidence=confidence,
            )
        ]

    def _validate_runtime(self) -> None:
        if self.settings.device == "cuda" and self.settings.require_gpu and not detect_gpu_available():
            raise ASRProviderError("TRANSCRIPTION_REQUIRE_GPU is enabled but no CUDA GPU is visible")

    def _buffer_duration_ms(self) -> int:
        bytes_per_sample = 2
        sample_count = len(self._audio_buffer) // bytes_per_sample
        return int(sample_count * 1000 / self.sample_rate)

    def _transcribe_buffer(self, config: StartSessionConfig, *, final: bool) -> tuple[str, float | None]:
        self._validate_runtime()
        model = self.model_cache.load_model(self.settings)
        task, language = transcription_task_for_language_mode(config.language_mode)
        with tempfile.NamedTemporaryFile(suffix=".wav") as audio_file:
            _write_pcm_wav(audio_file.name, bytes(self._audio_buffer), sample_rate=self.sample_rate)
            segments, _info = model.transcribe(
                audio_file.name,
                language=language,
                task=task,
                beam_size=5 if final else 1,
                vad_filter=True,
                condition_on_previous_text=False,
            )
            return _segments_to_text_and_confidence(segments)


def transcription_task_for_language_mode(language_mode: str) -> tuple[str, str | None]:
    if language_mode == "japanese_to_english":
        return "translate", "ja"
    if language_mode == "multilingual_to_english":
        return "transcribe", None
    return "transcribe", "en"


def detect_gpu_available() -> bool:
    try:
        import ctranslate2

        return ctranslate2.get_cuda_device_count() > 0
    except Exception:
        return False


def _segments_to_text_and_confidence(segments: Any) -> tuple[str, float | None]:
    text_parts: list[str] = []
    probabilities: list[float] = []
    for segment in segments:
        text = getattr(segment, "text", "")
        if isinstance(text, str) and text.strip():
            text_parts.append(text.strip())
        probability = getattr(segment, "avg_logprob", None)
        if isinstance(probability, (int, float)):
            probabilities.append(max(0.0, min(1.0, float(2.718281828459045 ** probability))))
    confidence = sum(probabilities) / len(probabilities) if probabilities else None
    return " ".join(text_parts).strip(), confidence


def _merge_realtime_text_fragments(current_text: str, fragments: list[str]) -> str:
    merged = current_text.strip()
    for fragment in fragments:
        if not isinstance(fragment, str) or not fragment.strip():
            continue
        candidate = fragment.strip()
        if not merged:
            merged = candidate
            continue
        if candidate == merged or candidate.startswith(merged):
            merged = candidate
            continue
        if merged.endswith(candidate):
            continue
        merged = (merged + fragment).strip()
    return _collapse_repeated_transcript_text(merged)


def _collapse_repeated_transcript_text(text: str) -> str:
    normalized = " ".join(text.split()).strip()
    words = normalized.split()
    if len(words) < 24 or len(normalized) < 120:
        return normalized
    midpoint = len(words) // 2
    split_start = max(12, midpoint - 4)
    split_end = min(len(words) - 12, midpoint + 4)
    best_ratio = 0.0
    best_second_half: list[str] | None = None
    for split in range(split_start, split_end + 1):
        first_half = words[:split]
        second_half = words[split:]
        length_delta = abs(len(first_half) - len(second_half))
        if length_delta > max(4, len(words) // 20):
            continue
        ratio = _transcript_word_similarity(first_half, second_half)
        if ratio > best_ratio:
            best_ratio = ratio
            best_second_half = second_half
    if best_ratio >= 0.92 and best_second_half is not None:
        return " ".join(best_second_half)
    return normalized


def _transcript_word_key(words: list[str]) -> tuple[str, ...]:
    return tuple(word.strip(".,!?;:\"'()[]{}").lower() for word in words)


def _transcript_word_similarity(first: list[str], second: list[str]) -> float:
    from difflib import SequenceMatcher

    return SequenceMatcher(None, _transcript_word_key(first), _transcript_word_key(second)).ratio()


def _extract_realtime_text_delta(payload: dict[str, Any]) -> str | None:
    event_type = payload.get("type")
    if event_type in {
        "response.audio_transcript.delta",
        "response.text.delta",
        "response.output_text.delta",
        "conversation.item.input_audio_transcription.delta",
        "transcription_session.delta",
        "transcription.delta",
    }:
        return _first_string(payload, ("delta", "text", "transcript"))
    if event_type in {
        "response.audio_transcript.done",
        "response.text.done",
        "response.output_text.done",
        "conversation.item.input_audio_transcription.completed",
        "transcription_session.completed",
        "transcription.completed",
        "transcription.done",
    }:
        return _first_string(payload, ("transcript", "text"))
    return None


def _extract_realtime_error(payload: dict[str, Any]) -> str | None:
    if payload.get("type") != "error":
        return None
    error = payload.get("error")
    if isinstance(error, str) and error.strip():
        return error.strip()
    if isinstance(error, dict):
        message = error.get("message")
        if isinstance(message, str) and message.strip():
            return message.strip()
    message = payload.get("message")
    return message.strip() if isinstance(message, str) and message.strip() else "voxtral realtime runtime error"


def _is_realtime_done(payload: dict[str, Any]) -> bool:
    return payload.get("type") in {
        "response.audio_transcript.done",
        "response.text.done",
        "response.output_text.done",
        "conversation.item.input_audio_transcription.completed",
        "transcription_session.completed",
        "transcription.completed",
        "transcription.done",
    }


def _is_realtime_response_window_done(payload: dict[str, Any]) -> bool:
    return payload.get("type") in {
        "response.done",
        "response.audio_transcript.done",
        "response.text.done",
        "response.output_text.done",
        "conversation.item.input_audio_transcription.completed",
        "transcription_session.completed",
        "transcription.completed",
        "transcription.done",
    }


def _first_string(payload: dict[str, Any], keys: tuple[str, ...]) -> str | None:
    for key in keys:
        value = payload.get(key)
        if isinstance(value, str) and value:
            return value
    return None


def _write_pcm_wav(path: str, pcm_s16le: bytes, *, sample_rate: int) -> None:
    with wave.open(path, "wb") as wav_file:
        wav_file.setnchannels(1)
        wav_file.setsampwidth(2)
        wav_file.setframerate(sample_rate)
        wav_file.writeframes(pcm_s16le)


def _pcm16_rms_peak(pcm_s16le: bytes) -> tuple[float, float]:
    sample_count = len(pcm_s16le) // 2
    if sample_count == 0:
        return 0.0, 0.0
    total_square = 0.0
    peak = 0
    for index in range(0, sample_count * 2, 2):
        sample = int.from_bytes(pcm_s16le[index : index + 2], "little", signed=True)
        absolute = abs(sample)
        peak = max(peak, absolute)
        total_square += float(sample) * float(sample)
    return (total_square / sample_count) ** 0.5 / 32768.0, peak / 32768.0


def _scale_pcm16(pcm_s16le: bytes, gain: float) -> bytes:
    if gain <= 1.0001:
        return pcm_s16le
    output = bytearray(pcm_s16le)
    sample_count = len(output) // 2
    for index in range(0, sample_count * 2, 2):
        sample = int.from_bytes(output[index : index + 2], "little", signed=True)
        scaled = int(round(sample * gain))
        scaled = min(32767, max(-32768, scaled))
        output[index : index + 2] = int(scaled).to_bytes(2, "little", signed=True)
    return bytes(output)


def _clamp_float(value: float, lower_bound: float, upper_bound: float) -> float:
    return min(max(value, lower_bound), upper_bound)


def _env_bool(name: str, default: bool) -> bool:
    raw = os.environ.get(name)
    if raw is None:
        return default
    return raw.strip().lower() in {"1", "true", "yes", "on"}


def _env_int(name: str, default: int) -> int:
    raw = os.environ.get(name)
    if raw is None:
        return default
    try:
        return int(raw.strip())
    except ValueError as exc:
        raise ASRProviderError(f"{name} must be an integer") from exc


def _env_float(name: str, default: float) -> float:
    raw = os.environ.get(name)
    if raw is None:
        return default
    try:
        return float(raw.strip())
    except ValueError as exc:
        raise ASRProviderError(f"{name} must be a number") from exc


def _env_first(*names: str, default: str) -> str:
    for name in names:
        value = os.environ.get(name)
        if value is not None:
            return value
    return default


def _vllm_realtime_url(base_url: str) -> str:
    normalized = (base_url or "http://127.0.0.1:8000").strip().rstrip("/")
    if normalized.startswith("https://"):
        return "wss://" + normalized[len("https://") :] + "/v1/realtime"
    if normalized.startswith("http://"):
        return "ws://" + normalized[len("http://") :] + "/v1/realtime"
    if normalized.startswith("ws://") or normalized.startswith("wss://"):
        return normalized + "/v1/realtime"
    return f"ws://{normalized}/v1/realtime"


def _optional_dependency_available(module_name: str) -> bool:
    return importlib.util.find_spec(module_name) is not None
