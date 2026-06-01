from __future__ import annotations

import base64
from dataclasses import dataclass, field
import importlib.util
import json
import os
import tempfile
import threading
from urllib import error as urllib_error
from urllib import parse as urllib_parse
from urllib import request as urllib_request
from typing import Any, Protocol
import wave

from .protocol import StartSessionConfig


class DiarizationProviderError(RuntimeError):
    pass


class DiarizationProvider(Protocol):
    def warmup(self) -> None:
        ...

    def diarize(self, config: StartSessionConfig, pcm_s16le: bytes) -> list["SpeakerTurn"]:
        ...

    def runtime_status(self) -> dict[str, Any]:
        ...


@dataclass(frozen=True)
class SpeakerTurn:
    speaker_id: str
    start_time_ms: int
    end_time_ms: int
    confidence: float | None = None


@dataclass(frozen=True)
class DiarizationProviderSettings:
    provider: str = "mock"
    model_name: str = "pyannote/speaker-diarization-community-1"
    model_version: str = "pyannote-audio-4.x"
    device: str = "cpu"
    auth_token: str | None = None
    min_speakers: int | None = None
    max_speakers: int | None = None
    worker_url: str | None = None
    worker_timeout_seconds: float = 45.0
    worker_auth_token: str | None = None

    @classmethod
    def from_environment(cls) -> "DiarizationProviderSettings":
        return cls(
            provider=os.environ.get("TRANSCRIPTION_DIARIZATION_PROVIDER", "mock").strip().lower(),
            model_name=os.environ.get(
                "TRANSCRIPTION_PYANNOTE_MODEL",
                "pyannote/speaker-diarization-community-1",
            ).strip(),
            model_version=os.environ.get("TRANSCRIPTION_PYANNOTE_MODEL_VERSION", "pyannote-audio-4.x").strip(),
            device=os.environ.get("TRANSCRIPTION_PYANNOTE_DEVICE", "cpu").strip().lower(),
            auth_token=_optional_env_text("PYANNOTE_AUTH_TOKEN")
            or _optional_env_text("HF_TOKEN")
            or _optional_env_text("HUGGINGFACE_TOKEN"),
            min_speakers=_optional_env_int("TRANSCRIPTION_DIARIZATION_MIN_SPEAKERS"),
            max_speakers=_optional_env_int("TRANSCRIPTION_DIARIZATION_MAX_SPEAKERS"),
            worker_url=_optional_env_text("TRANSCRIPTION_DIARIZATION_WORKER_URL"),
            worker_timeout_seconds=_optional_env_float("TRANSCRIPTION_DIARIZATION_WORKER_TIMEOUT_SECONDS", 45.0),
            worker_auth_token=_optional_env_text("TRANSCRIPTION_DIARIZATION_WORKER_AUTH_TOKEN"),
        )


@dataclass
class DiarizationProviderFactory:
    settings: DiarizationProviderSettings = field(default_factory=DiarizationProviderSettings.from_environment)
    pipeline_cache: "PyannotePipelineCache" = field(default_factory=lambda: default_pipeline_cache)

    def create_provider(self) -> DiarizationProvider:
        if self.settings.provider == "mock":
            return MockDiarizationProvider()
        if self.settings.provider == "pyannote":
            return PyannoteDiarizationProvider(settings=self.settings, pipeline_cache=self.pipeline_cache)
        if self.settings.provider in {"remote_http", "worker_http"}:
            return RemoteHTTPDiarizationProvider(settings=self.settings)
        if self.settings.provider in {"off", "disabled", "none"}:
            return DisabledDiarizationProvider(provider_name=self.settings.provider)
        raise DiarizationProviderError(f"unsupported diarization provider: {self.settings.provider}")

    def runtime_status(self) -> dict[str, Any]:
        try:
            return self.create_provider().runtime_status()
        except DiarizationProviderError as exc:
            return {
                "provider": self.settings.provider,
                "ready": False,
                "error": str(exc),
            }


@dataclass
class DisabledDiarizationProvider:
    provider_name: str = "disabled"

    def warmup(self) -> None:
        return None

    def runtime_status(self) -> dict[str, Any]:
        return {
            "provider": self.provider_name,
            "model_name": None,
            "model_version": None,
            "device": None,
            "dependencies_available": True,
            "auth_token_configured": False,
            "warmed": True,
            "ready": True,
            "speaker_labeling_available": False,
        }

    def diarize(self, _config: StartSessionConfig, _pcm_s16le: bytes) -> list[SpeakerTurn]:
        return []


@dataclass
class MockDiarizationProvider:
    model_name: str = "mock-diarization"
    model_version: str = "slice-5"
    _warmed: bool = False

    def warmup(self) -> None:
        self._warmed = True

    def runtime_status(self) -> dict[str, Any]:
        return {
            "provider": "mock",
            "model_name": self.model_name,
            "model_version": self.model_version,
            "device": "cpu",
            "dependencies_available": True,
            "auth_token_configured": False,
            "warmed": self._warmed,
            "ready": True,
            "speaker_labeling_available": False,
        }

    def diarize(self, config: StartSessionConfig, pcm_s16le: bytes) -> list[SpeakerTurn]:
        return []


class PyannotePipelineCache:
    def __init__(self) -> None:
        self._pipelines: dict[tuple[str, str, str | None], Any] = {}
        self._lock = threading.Lock()

    def load_pipeline(self, settings: DiarizationProviderSettings) -> Any:
        cache_key = (settings.model_name, settings.device, _token_cache_key(settings.auth_token))
        with self._lock:
            cached = self._pipelines.get(cache_key)
            if cached is not None:
                return cached

            os.environ.setdefault("PYANNOTE_METRICS_ENABLED", "0")
            try:
                from pyannote.audio import Pipeline
            except ImportError as exc:
                raise DiarizationProviderError(
                    "pyannote.audio is not installed; install requirements-diarization.txt "
                    "or use TRANSCRIPTION_DIARIZATION_PROVIDER=mock"
                ) from exc

            try:
                try:
                    pipeline = Pipeline.from_pretrained(settings.model_name, token=settings.auth_token)
                except TypeError:
                    pipeline = Pipeline.from_pretrained(settings.model_name, use_auth_token=settings.auth_token)
            except Exception as exc:
                raise DiarizationProviderError(
                    f"could not load pyannote pipeline: {type(exc).__name__}"
                ) from exc

            if settings.device and settings.device != "cpu":
                try:
                    import torch

                    pipeline.to(torch.device(settings.device))
                except Exception as exc:
                    raise DiarizationProviderError(f"could not move pyannote pipeline to {settings.device}") from exc

            self._pipelines[cache_key] = pipeline
            return pipeline

    def is_loaded(self, settings: DiarizationProviderSettings) -> bool:
        cache_key = (settings.model_name, settings.device, _token_cache_key(settings.auth_token))
        if not self._lock.acquire(blocking=False):
            return False
        try:
            return cache_key in self._pipelines
        finally:
            self._lock.release()


default_pipeline_cache = PyannotePipelineCache()


@dataclass
class RemoteHTTPDiarizationProvider:
    settings: DiarizationProviderSettings = field(default_factory=DiarizationProviderSettings.from_environment)

    def warmup(self) -> None:
        self._validate_runtime()

    def runtime_status(self) -> dict[str, Any]:
        endpoint_configured = bool(self.settings.worker_url and self.settings.worker_url.strip())
        return {
            "provider": self.settings.provider,
            "model_name": "remote-diarization-worker",
            "model_version": "http-v1",
            "device": "remote",
            "dependencies_available": True,
            "worker_url_configured": endpoint_configured,
            "auth_token_configured": bool(self.settings.worker_auth_token and self.settings.worker_auth_token.strip()),
            "warmed": endpoint_configured,
            "speaker_labeling_available": endpoint_configured,
            "ready": endpoint_configured,
        }

    def diarize(self, config: StartSessionConfig, pcm_s16le: bytes) -> list[SpeakerTurn]:
        if not config.diarization_enabled or not pcm_s16le:
            return []
        self._validate_runtime()
        endpoint = _worker_endpoint(self.settings.worker_url or "")
        payload = {
            "protocol_version": config.protocol_version,
            "session_id": config.session_id,
            "diarization_enabled": config.diarization_enabled,
            "language_mode": config.language_mode,
            "sample_rate": config.sample_rate,
            "channel_count": config.channel_count,
            "audio_encoding": config.audio_encoding,
            "client_timestamp": config.client_timestamp,
            "app_version": config.app_version,
            "privacy_safe_device_id": config.privacy_safe_device_id,
            "audio_b64": base64.b64encode(pcm_s16le).decode("ascii"),
        }
        headers = {"Content-Type": "application/json"}
        if self.settings.worker_auth_token and self.settings.worker_auth_token.strip():
            headers["Authorization"] = f"Bearer {self.settings.worker_auth_token.strip()}"
        request = urllib_request.Request(
            endpoint,
            data=json.dumps(payload, separators=(",", ":"), sort_keys=True).encode("utf-8"),
            headers=headers,
            method="POST",
        )
        try:
            with urllib_request.urlopen(request, timeout=self.settings.worker_timeout_seconds) as response:
                response_payload = json.loads(response.read().decode("utf-8"))
        except (urllib_error.URLError, TimeoutError, json.JSONDecodeError, UnicodeDecodeError) as exc:
            raise DiarizationProviderError(f"remote diarization worker request failed: {type(exc).__name__}") from exc
        return _speaker_turns_from_worker_response(response_payload)

    def _validate_runtime(self) -> None:
        if not self.settings.worker_url or not self.settings.worker_url.strip():
            raise DiarizationProviderError(
                "TRANSCRIPTION_DIARIZATION_WORKER_URL is required for "
                "TRANSCRIPTION_DIARIZATION_PROVIDER=remote_http"
            )


@dataclass
class PyannoteDiarizationProvider:
    settings: DiarizationProviderSettings = field(default_factory=DiarizationProviderSettings.from_environment)
    pipeline_cache: PyannotePipelineCache = field(default_factory=lambda: default_pipeline_cache)
    _pipeline: Any | None = None
    _warmed: bool = False

    def warmup(self) -> None:
        self._validate_runtime()
        self._pipeline = self.pipeline_cache.load_pipeline(self.settings)
        self._warmed = True

    def runtime_status(self) -> dict[str, Any]:
        dependencies_available = _optional_dependency_available("pyannote.audio")
        token_required = _requires_auth_token(self.settings.model_name)
        token_configured = bool(self.settings.auth_token and self.settings.auth_token.strip())
        return {
            "provider": "pyannote",
            "model_name": self.settings.model_name,
            "model_version": self.settings.model_version,
            "device": self.settings.device,
            "dependencies_available": dependencies_available,
            "auth_token_required": token_required,
            "auth_token_configured": token_configured,
            "min_speakers": self.settings.min_speakers,
            "max_speakers": self.settings.max_speakers,
            "telemetry_enabled": os.environ.get("PYANNOTE_METRICS_ENABLED", "0") not in {"0", "false", "False"},
            "warmed": self._warmed or _pipeline_cache_loaded(self.pipeline_cache, self.settings),
            "speaker_labeling_available": True,
            "ready": dependencies_available and (token_configured or not token_required),
        }

    def diarize(self, config: StartSessionConfig, pcm_s16le: bytes) -> list[SpeakerTurn]:
        if not config.diarization_enabled or not pcm_s16le:
            return []
        if self._pipeline is None:
            self.warmup()
        assert self._pipeline is not None
        with tempfile.NamedTemporaryFile(suffix=".wav") as audio_file:
            _write_pcm_wav(audio_file.name, pcm_s16le, sample_rate=config.sample_rate)
            options: dict[str, int] = {}
            if self.settings.min_speakers is not None:
                options["min_speakers"] = self.settings.min_speakers
            if self.settings.max_speakers is not None:
                options["max_speakers"] = self.settings.max_speakers
            output = self._pipeline(audio_file.name, **options)
        return _speaker_turns_from_pyannote_output(output)

    def _validate_runtime(self) -> None:
        status = self.runtime_status()
        if not status["dependencies_available"]:
            raise DiarizationProviderError(
                "pyannote.audio is not installed; install requirements-diarization.txt "
                "or use TRANSCRIPTION_DIARIZATION_PROVIDER=mock"
            )
        if status["auth_token_required"] and not status["auth_token_configured"]:
            raise DiarizationProviderError(
                "PYANNOTE_AUTH_TOKEN, HF_TOKEN, or HUGGINGFACE_TOKEN is required for pyannote diarization"
            )


def _speaker_turns_from_pyannote_output(output: Any) -> list[SpeakerTurn]:
    raw_turns: list[tuple[float, float, str]] = []
    # pyannote community pipelines return a pipeline output object with
    # Annotation fields. Always use itertracks(yield_label=True), otherwise
    # Annotation iteration can expose track ids instead of speaker labels.
    exclusive_diarization = getattr(output, "exclusive_speaker_diarization", None)
    speaker_diarization = getattr(output, "speaker_diarization", None)
    if exclusive_diarization is not None:
        annotation = exclusive_diarization
    elif speaker_diarization is not None:
        annotation = speaker_diarization
    else:
        annotation = output
    if hasattr(annotation, "itertracks"):
        for turn, _track, speaker in annotation.itertracks(yield_label=True):
            raw_turns.append((float(turn.start), float(turn.end), str(speaker)))
    else:
        raise DiarizationProviderError("pyannote output did not include speaker turns")

    label_map: dict[str, str] = {}
    turns: list[SpeakerTurn] = []
    for start_seconds, end_seconds, source_label in sorted(raw_turns, key=lambda item: (item[0], item[1])):
        if source_label not in label_map:
            label_map[source_label] = str(len(label_map) + 1)
        turns.append(
            SpeakerTurn(
                speaker_id=label_map[source_label],
                start_time_ms=max(0, round(start_seconds * 1000)),
                end_time_ms=max(0, round(end_seconds * 1000)),
            )
        )
    return [turn for turn in turns if turn.end_time_ms > turn.start_time_ms]


def _write_pcm_wav(path: str, pcm_s16le: bytes, *, sample_rate: int) -> None:
    with wave.open(path, "wb") as wav_file:
        wav_file.setnchannels(1)
        wav_file.setsampwidth(2)
        wav_file.setframerate(sample_rate)
        wav_file.writeframes(pcm_s16le)


def _pcm_duration_ms(pcm_s16le: bytes, *, sample_rate: int) -> int:
    sample_count = len(pcm_s16le) // 2
    return int(sample_count * 1000 / sample_rate)


def _requires_auth_token(model_name: str) -> bool:
    if os.path.exists(model_name) or model_name.startswith((".", "/")):
        return False
    return model_name.startswith("pyannote/")


def _token_cache_key(token: str | None) -> str | None:
    return "configured" if token and token.strip() else None


def _pipeline_cache_loaded(cache: Any, settings: DiarizationProviderSettings) -> bool:
    is_loaded = getattr(cache, "is_loaded", None)
    if not callable(is_loaded):
        return False
    try:
        return bool(is_loaded(settings))
    except Exception:
        return False


def _optional_env_int(name: str) -> int | None:
    raw = os.environ.get(name)
    if raw is None or not raw.strip():
        return None
    try:
        return int(raw.strip())
    except ValueError as exc:
        raise DiarizationProviderError(f"{name} must be an integer") from exc


def _optional_env_float(name: str, default: float) -> float:
    raw = os.environ.get(name)
    if raw is None or not raw.strip():
        return default
    try:
        return max(0.1, float(raw.strip()))
    except ValueError as exc:
        raise DiarizationProviderError(f"{name} must be a number") from exc


def _optional_env_text(name: str) -> str | None:
    raw = os.environ.get(name)
    if raw is None:
        return None
    stripped = raw.strip()
    return stripped or None


def _optional_dependency_available(module_name: str) -> bool:
    try:
        return importlib.util.find_spec(module_name) is not None
    except ModuleNotFoundError:
        return False


def _worker_endpoint(worker_url: str) -> str:
    base = worker_url.strip()
    if base.endswith("/v1/diarization"):
        return base
    return urllib_parse.urljoin(base.rstrip("/") + "/", "v1/diarization")


def _speaker_turns_from_worker_response(payload: Any) -> list[SpeakerTurn]:
    if not isinstance(payload, dict):
        raise DiarizationProviderError("remote diarization worker returned invalid JSON")
    raw_turns = payload.get("speaker_turns")
    if not isinstance(raw_turns, list):
        raise DiarizationProviderError("remote diarization worker response is missing speaker_turns")
    turns: list[SpeakerTurn] = []
    for item in raw_turns:
        if not isinstance(item, dict):
            raise DiarizationProviderError("remote diarization worker returned invalid speaker turn")
        speaker_id = item.get("speaker_id")
        start_time_ms = item.get("start_time_ms")
        end_time_ms = item.get("end_time_ms")
        confidence = item.get("confidence")
        if not isinstance(speaker_id, str) or not speaker_id.strip():
            raise DiarizationProviderError("remote diarization worker returned invalid speaker_id")
        if not isinstance(start_time_ms, int) or not isinstance(end_time_ms, int):
            raise DiarizationProviderError("remote diarization worker returned invalid speaker turn times")
        if end_time_ms <= start_time_ms:
            continue
        turns.append(
            SpeakerTurn(
                speaker_id=speaker_id.strip(),
                start_time_ms=max(0, start_time_ms),
                end_time_ms=max(0, end_time_ms),
                confidence=confidence if isinstance(confidence, float | int) else None,
            )
        )
    return turns
