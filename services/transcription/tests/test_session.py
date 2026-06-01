from __future__ import annotations

import json
import os
from types import SimpleNamespace
import unittest
from unittest.mock import patch

from transcription_service.asr import (
    ASRProviderError,
    ASRProviderFactory,
    ASRProviderSettings,
    FasterWhisperASRProvider,
    SelfHostedVoxtralRealtimeASRProvider,
    VoxtralRealtimeASRProvider,
    transcription_task_for_language_mode,
)
from transcription_service.alignment import TranscriptTiming, assign_speakers_to_segments
from transcription_service.diarization import (
    DiarizationProviderError,
    DiarizationProviderFactory,
    DiarizationProviderSettings,
    PyannoteDiarizationProvider,
    SpeakerTurn,
)
from transcription_service.protocol import decode_start_session
from transcription_service.server import _effective_diarization_stop_timeout_seconds
from transcription_service.session import BackpressureError, TranscriptionSession


def make_config(*, diarization_enabled=True, language_mode="english_to_english"):
    return decode_start_session(
        json.dumps(
            {
                "type": "start_session",
                "protocol_version": "transcription.v1",
                "session_id": "session-abc",
                "transcription_enabled": True,
                "diarization_enabled": diarization_enabled,
                "language_mode": language_mode,
                "sample_rate": 16000,
                "channel_count": 1,
                "audio_encoding": "pcm_s16le",
                "client_timestamp": "2026-05-17T17:00:00.000Z",
            }
        )
    )


class TranscriptionSessionTests(unittest.TestCase):
    def test_mock_asr_warmup_and_english_route(self):
        session = TranscriptionSession(config=make_config(language_mode="english_to_english"))
        start_events = [json.loads(event) for event in session.start_events()]
        partial_events = [json.loads(event) for event in session.receive_audio_frame(bytes([0, 1, 2, 3]))]
        stop_events = [json.loads(event) for event in session.stop_events()]

        self.assertEqual(start_events[0]["type"], "session_started")
        self.assertEqual(start_events[0]["model_route"], "voxtral-realtime-transcribe-en")
        self.assertEqual(start_events[0]["asr_provider"], "mock")
        self.assertEqual(start_events[0]["model_name"], "mock-asr")
        self.assertEqual(partial_events[0]["type"], "partial_transcript")
        self.assertEqual(partial_events[0]["language_mode"], "english_to_english")
        self.assertEqual(stop_events[0]["type"], "final_transcript")
        self.assertEqual(stop_events[-1]["type"], "session_stopped")

    def test_mock_asr_japanese_to_english_route(self):
        session = TranscriptionSession(config=make_config(language_mode="japanese_to_english"))
        session.start_events()
        partial_event = json.loads(session.receive_audio_frame(bytes([0, 1, 2, 3]))[0])
        final_event = json.loads(session.stop_events()[0])

        self.assertEqual(partial_event["language_mode"], "japanese_to_english")
        self.assertIn("translated English", partial_event["text"])
        self.assertEqual(final_event["language_mode"], "japanese_to_english")

    def test_mock_diarization_reports_unavailable_without_fake_speaker_labels(self):
        session = TranscriptionSession(config=make_config(diarization_enabled=True))
        start_events = [json.loads(event) for event in session.start_events()]
        partial_event = json.loads(session.receive_audio_frame(bytes([0, 1]))[0])
        stop_events = [json.loads(event) for event in session.stop_events()]

        self.assertEqual(start_events[1]["type"], "diarization_status")
        self.assertEqual(start_events[1]["status"], "unavailable")
        self.assertEqual(start_events[1]["provider"], "mock")
        self.assertEqual(start_events[1]["reason"], "mock_provider")
        self.assertFalse(start_events[0]["diarization_speaker_labeling_available"])
        self.assertNotIn("speaker_id", partial_event)
        self.assertEqual(stop_events[0]["type"], "final_transcript")
        self.assertNotIn("speaker_id", stop_events[0])
        self.assertEqual(stop_events[1]["type"], "diarization_status")
        self.assertEqual(stop_events[1]["status"], "unavailable")
        self.assertEqual(stop_events[1]["reason"], "mock_provider")
        self.assertEqual(stop_events[-1]["type"], "session_stopped")
        self.assertNotIn("speaker_update", {event["type"] for event in stop_events})

    def test_diarization_splits_long_final_transcript_for_speaker_assignment(self):
        session = TranscriptionSession(
            config=make_config(diarization_enabled=True),
            asr_provider=SentenceFinalASRProvider(),
            diarization_provider=TwoSpeakerDiarizationProvider(),
        )
        start_events = [json.loads(event) for event in session.start_events()]
        session.receive_audio_frame(bytes(32_000))

        stop_events = [json.loads(event) for event in session.stop_events()]
        final_events = [event for event in stop_events if event["type"] == "final_transcript"]
        speaker_events = [event for event in stop_events if event["type"] == "speaker_update"]

        self.assertEqual(len(final_events), 2)
        self.assertTrue(start_events[0]["diarization_speaker_labeling_available"])
        self.assertEqual(final_events[0]["segment_id"], "session-abc-segment-1")
        self.assertEqual(final_events[1]["segment_id"], "session-abc-segment-1-part-2")
        self.assertEqual([event["speaker_id"] for event in final_events], ["1", "2"])
        self.assertEqual([event["speaker_id"] for event in speaker_events], ["1", "2"])

    def test_diarization_splits_devanagari_sentence_boundaries_for_speaker_assignment(self):
        session = TranscriptionSession(
            config=make_config(diarization_enabled=True),
            asr_provider=DevanagariSentenceFinalASRProvider(),
            diarization_provider=TwoSpeakerDiarizationProvider(),
        )
        session.start_events()
        session.receive_audio_frame(bytes(32_000))

        stop_events = [json.loads(event) for event in session.stop_events()]
        final_events = [event for event in stop_events if event["type"] == "final_transcript"]
        speaker_events = [event for event in stop_events if event["type"] == "speaker_update"]

        self.assertEqual(len(final_events), 2)
        self.assertEqual(final_events[0]["text"], "पहला वक्ता योजना समझाता है।")
        self.assertEqual(final_events[1]["text"], "दूसरा वक्ता समयसीमा पर सवाल करता है।")
        self.assertEqual([event["speaker_id"] for event in final_events], ["1", "2"])
        self.assertEqual([event["speaker_id"] for event in speaker_events], ["1", "2"])

    def test_diarization_warmup_failure_does_not_stop_transcription(self):
        session = TranscriptionSession(
            config=make_config(diarization_enabled=True),
            asr_provider=SentenceFinalASRProvider(),
            diarization_provider=FailingWarmupDiarizationProvider(),
        )

        start_events = [json.loads(event) for event in session.start_events()]
        session.receive_audio_frame(bytes(32_000))
        stop_events = [json.loads(event) for event in session.stop_events()]

        final_events = [event for event in stop_events if event["type"] == "final_transcript"]
        status_events = [event for event in [*start_events, *stop_events] if event["type"] == "diarization_status"]

        self.assertEqual(start_events[0]["type"], "session_started")
        self.assertFalse(start_events[0]["diarization_ready"])
        self.assertFalse(start_events[0]["diarization_warmed"])
        self.assertEqual(status_events[0]["status"], "failed")
        self.assertEqual(status_events[0]["error_class"], "FakeGatedRepoError")
        self.assertEqual(status_events[0]["provider"], "fake-diarization")
        self.assertEqual(len(final_events), 1)
        self.assertNotIn("speaker_id", final_events[0])
        self.assertEqual(stop_events[-1]["type"], "session_stopped")

    def test_diarization_disabled_omits_speaker_labels_and_speaker_updates(self):
        session = TranscriptionSession(config=make_config(diarization_enabled=False))
        session.start_events()
        partial_event = json.loads(session.receive_audio_frame(bytes([0, 1]))[0])
        stop_events = [json.loads(event) for event in session.stop_events()]

        self.assertNotIn("speaker_id", partial_event)
        self.assertNotIn("speaker_id", stop_events[0])
        self.assertNotIn("speaker_update", {event["type"] for event in stop_events})

    def test_backpressure_behavior(self):
        session = TranscriptionSession(config=make_config(), max_pending_audio_frames=0)
        session.start_events()

        with self.assertRaises(BackpressureError):
            session.receive_audio_frame(bytes([0, 1]))

    def test_asr_factory_selects_mock_by_default_and_rejects_unknown_provider(self):
        self.assertEqual(ASRProviderFactory(ASRProviderSettings(provider="mock")).create_provider().runtime_status()["provider"], "mock")

        with self.assertRaises(ASRProviderError):
            ASRProviderFactory(ASRProviderSettings(provider="unknown")).create_provider()

    def test_voxtral_realtime_provider_streams_text_deltas(self):
        settings = ASRProviderSettings(
            provider="voxtral_realtime",
            voxtral_model="voxtral-mini-transcribe-realtime-2602",
            voxtral_model_version="test-voxtral",
            mistral_api_key="test-key",
        )
        provider = VoxtralRealtimeASRProvider(settings=settings, stream_factory=FakeVoxtralStreamFactory())
        session = TranscriptionSession(
            config=make_config(language_mode="japanese_to_english"),
            asr_provider=provider,
        )

        with patch("transcription_service.asr._optional_dependency_available", return_value=True):
            start_events = [json.loads(event) for event in session.start_events()]
            partial_event = json.loads(session.receive_audio_frame(bytes([0, 1]))[0])
            final_event = json.loads(session.stop_events()[0])

        self.assertEqual(start_events[0]["asr_provider"], "voxtral_realtime")
        self.assertEqual(start_events[0]["model_name"], "voxtral-mini-transcribe-realtime-2602")
        self.assertEqual(partial_event["text"], "hello")
        self.assertEqual(final_event["text"], "hello world")
        self.assertEqual(final_event["language_mode"], "japanese_to_english")

    def test_self_hosted_voxtral_provider_uses_internal_runtime_without_mistral_key(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_model_version="self-hosted-vllm",
            voxtral_runtime="vllm",
            voxtral_realtime_url="ws://127.0.0.1:8000/v1/realtime",
            mistral_api_key=None,
        )
        provider = SelfHostedVoxtralRealtimeASRProvider(settings=settings, stream_factory=FakeVoxtralStreamFactory())
        session = TranscriptionSession(config=make_config(), asr_provider=provider)

        with patch("transcription_service.asr._optional_dependency_available", return_value=True):
            start_events = [json.loads(event) for event in session.start_events()]
            partial_event = json.loads(session.receive_audio_frame(bytes([0, 1]))[0])

        self.assertEqual(start_events[0]["asr_provider"], "voxtral_self_hosted")
        self.assertEqual(start_events[0]["model_name"], "mistralai/Voxtral-Mini-4B-Realtime-2602")
        self.assertEqual(start_events[0]["device"], "cuda")
        self.assertEqual(start_events[0]["compute_type"], "vllm")
        self.assertEqual(partial_event["text"], "hello")
        self.assertIsNone(settings.mistral_api_key)

    def test_self_hosted_voxtral_provider_treats_final_transcript_as_replacement(self):
        settings = ASRProviderSettings(
            provider="voxtral_self_hosted",
            voxtral_model="mistralai/Voxtral-Mini-4B-Realtime-2602",
            voxtral_model_version="self-hosted-vllm",
            voxtral_runtime="vllm",
            voxtral_realtime_url="ws://127.0.0.1:8000/v1/realtime",
        )
        stream = FakeVoxtralStream(deltas=[["hello"], ["hello world"]])
        provider = SelfHostedVoxtralRealtimeASRProvider(
            settings=settings,
            stream_factory=FakeVoxtralStreamFactory(stream),
        )
        session = TranscriptionSession(config=make_config(diarization_enabled=False), asr_provider=provider)

        with patch("transcription_service.asr._optional_dependency_available", return_value=True):
            session.start_events()
            partial_event = json.loads(session.receive_audio_frame(bytes([0, 1]))[0])
            final_event = json.loads(session.stop_events()[0])

        self.assertEqual(partial_event["text"], "hello")
        self.assertEqual(final_event["text"], "hello world")

    def test_language_modes_map_to_faster_whisper_tasks(self):
        self.assertEqual(transcription_task_for_language_mode("english_to_english"), ("transcribe", "en"))
        self.assertEqual(transcription_task_for_language_mode("japanese_to_english"), ("translate", "ja"))
        self.assertEqual(transcription_task_for_language_mode("multilingual_to_english"), ("transcribe", None))

    def test_faster_whisper_provider_uses_audio_buffer_for_final_transcript(self):
        cache = FakeWhisperModelCache(
            FakeWhisperModel(
                [
                    [SimpleNamespace(text="partial text", avg_logprob=-0.1)],
                    [SimpleNamespace(text="final text", avg_logprob=-0.05)],
                ]
            )
        )
        settings = ASRProviderSettings(
            provider="faster_whisper",
            model_name="large-v3-turbo",
            model_version="test-model",
            device="cpu",
            compute_type="int8",
            require_gpu=False,
            partial_min_audio_ms=1,
        )
        provider = FasterWhisperASRProvider(settings=settings, model_cache=cache)
        session = TranscriptionSession(config=make_config(language_mode="japanese_to_english"), asr_provider=provider)

        start_events = [json.loads(event) for event in session.start_events()]
        partial_event = json.loads(session.receive_audio_frame(bytes(64))[0])
        final_event = json.loads(session.stop_events()[0])

        self.assertEqual(start_events[0]["asr_provider"], "faster_whisper")
        self.assertEqual(partial_event["text"], "partial text")
        self.assertEqual(final_event["text"], "final text")
        self.assertEqual(cache.model.calls[0]["task"], "translate")
        self.assertEqual(cache.model.calls[0]["language"], "ja")
        self.assertEqual(cache.model.calls[-1]["beam_size"], 5)

    def test_alignment_assigns_segment_to_speaker_turn_by_overlap(self):
        assignments = assign_speakers_to_segments(
            [
                TranscriptTiming(segment_id="segment-a", start_time_ms=100, end_time_ms=1_200),
                TranscriptTiming(segment_id="segment-b", start_time_ms=1_500, end_time_ms=2_500),
            ],
            [
                SpeakerTurn(speaker_id="1", start_time_ms=0, end_time_ms=1_000),
                SpeakerTurn(speaker_id="2", start_time_ms=1_000, end_time_ms=3_000),
            ],
        )

        self.assertEqual([assignment.speaker_id for assignment in assignments], ["1", "2"])
        self.assertEqual(assignments[0].segment_id, "segment-a")
        self.assertGreater(assignments[0].overlap_ms, 0)

    def test_pyannote_provider_converts_annotation_to_stable_speaker_turns(self):
        settings = DiarizationProviderSettings(
            provider="pyannote",
            model_name="pyannote/speaker-diarization-community-1",
            model_version="test-pyannote",
            auth_token="hf-test",
        )
        provider = PyannoteDiarizationProvider(settings=settings, pipeline_cache=FakePyannotePipelineCache())

        with patch("transcription_service.diarization._optional_dependency_available", return_value=True):
            provider.warmup()
            turns = provider.diarize(make_config(diarization_enabled=True), bytes(32000))

        self.assertEqual([turn.speaker_id for turn in turns], ["1", "2", "1"])
        self.assertEqual(turns[0].start_time_ms, 0)
        self.assertEqual(turns[0].end_time_ms, 750)
        self.assertTrue(provider.runtime_status()["auth_token_configured"])

    def test_pyannote_provider_uses_speaker_labels_from_pipeline_output_annotation(self):
        settings = DiarizationProviderSettings(
            provider="pyannote",
            model_name="pyannote/speaker-diarization-community-1",
            model_version="test-pyannote",
            auth_token="hf-test",
        )
        provider = PyannoteDiarizationProvider(
            settings=settings,
            pipeline_cache=FakePyannotePipelineCache(FakePyannotePipelineOutput()),
        )

        with patch("transcription_service.diarization._optional_dependency_available", return_value=True):
            provider.warmup()
            turns = provider.diarize(make_config(diarization_enabled=True), bytes(32000))

        self.assertEqual([turn.speaker_id for turn in turns], ["1", "2", "1"])

    def test_diarization_factory_reports_pyannote_missing_token_as_degraded(self):
        status = DiarizationProviderFactory(
            DiarizationProviderSettings(provider="pyannote", auth_token=None)
        ).runtime_status()

        self.assertEqual(status["provider"], "pyannote")
        self.assertFalse(status["ready"])

    def test_pyannote_settings_strip_auth_token_whitespace(self):
        with patch.dict(
            os.environ,
            {
                "TRANSCRIPTION_DIARIZATION_PROVIDER": "pyannote",
                "PYANNOTE_AUTH_TOKEN": " hf-test-token\n",
            },
        ):
            settings = DiarizationProviderSettings.from_environment()

        self.assertEqual(settings.auth_token, "hf-test-token")

    def test_remote_http_diarization_provider_posts_audio_to_worker(self):
        settings = DiarizationProviderSettings(
            provider="remote_http",
            worker_url="http://worker.internal:8080",
            worker_timeout_seconds=2.5,
            worker_auth_token="worker-secret",
        )
        provider = DiarizationProviderFactory(settings=settings).create_provider()

        def fake_urlopen(request, timeout):
            self.assertEqual(request.full_url, "http://worker.internal:8080/v1/diarization")
            self.assertEqual(timeout, 2.5)
            self.assertEqual(request.get_header("Authorization"), "Bearer worker-secret")
            payload = json.loads(request.data.decode("utf-8"))
            self.assertEqual(payload["session_id"], "session-abc")
            self.assertEqual(payload["sample_rate"], 16000)
            self.assertEqual(payload["audio_b64"], "AAABAA==")
            self.assertNotIn("text", payload)
            return FakeHTTPResponse(
                {
                    "session_id": "session-abc",
                    "speaker_turns": [
                        {"speaker_id": "1", "start_time_ms": 0, "end_time_ms": 500},
                        {"speaker_id": "2", "start_time_ms": 500, "end_time_ms": 1000},
                    ],
                }
            )

        with patch("transcription_service.diarization.urllib_request.urlopen", side_effect=fake_urlopen):
            turns = provider.diarize(make_config(diarization_enabled=True), bytes([0, 0, 1, 0]))

        self.assertEqual([turn.speaker_id for turn in turns], ["1", "2"])
        self.assertEqual(turns[1].start_time_ms, 500)

    def test_remote_diarization_stop_timeout_does_not_preempt_worker_timeout(self):
        settings = DiarizationProviderSettings(provider="remote_http", worker_timeout_seconds=90)

        self.assertEqual(_effective_diarization_stop_timeout_seconds(settings, 45), 95)
        self.assertEqual(_effective_diarization_stop_timeout_seconds(settings, 180), 180)
        self.assertEqual(
            _effective_diarization_stop_timeout_seconds(
                DiarizationProviderSettings(provider="mock", worker_timeout_seconds=90),
                45,
            ),
            45,
        )


class FakeWhisperModelCache:
    def __init__(self, model):
        self.model = model

    def load_model(self, _settings):
        return self.model


class FakeWhisperModel:
    def __init__(self, segment_batches):
        self._segment_batches = list(segment_batches)
        self.calls = []

    def transcribe(self, audio_path, **kwargs):
        self.calls.append({"audio_path": audio_path, **kwargs})
        return self._segment_batches.pop(0), SimpleNamespace()


class FakeVoxtralStreamFactory:
    def __init__(self, stream=None):
        self.stream = stream

    def create_stream(self):
        return self.stream or FakeVoxtralStream()


class FakeVoxtralStream:
    def __init__(self, deltas=None):
        self.started = False
        self.stopped = False
        self._deltas = deltas or [["hello"], [" world"]]

    def start(self):
        self.started = True

    def send_audio(self, _audio_frame):
        pass

    def drain_text_deltas(self):
        if not self._deltas:
            return []
        return self._deltas.pop(0)

    def stop(self):
        self.stopped = True


class FakePyannotePipelineCache:
    def __init__(self, output=None):
        self.output = output or FakePyannoteAnnotation()

    def load_pipeline(self, _settings):
        return FakePyannotePipeline(self.output)


class FakePyannotePipeline:
    def __init__(self, output):
        self.output = output

    def __call__(self, audio_path, **kwargs):
        self.audio_path = audio_path
        self.kwargs = kwargs
        return self.output


class FakePyannoteAnnotation:
    def itertracks(self, yield_label=True):
        assert yield_label is True
        yield SimpleNamespace(start=0.0, end=0.75), None, "SPEAKER_00"
        yield SimpleNamespace(start=0.75, end=1.4), None, "SPEAKER_01"
        yield SimpleNamespace(start=1.4, end=1.8), None, "SPEAKER_00"


class FakePyannotePipelineOutput:
    def __init__(self):
        self.speaker_diarization = FakePyannoteAnnotationWithMisleadingIterator()


class FakePyannoteAnnotationWithMisleadingIterator(FakePyannoteAnnotation):
    def __iter__(self):
        yield SimpleNamespace(start=0.0, end=0.75), "_"
        yield SimpleNamespace(start=0.75, end=1.4), "_"
        yield SimpleNamespace(start=1.4, end=1.8), "_"


class SentenceFinalASRProvider:
    def warmup(self):
        return None

    def runtime_status(self):
        return {
            "provider": "fake-sentence-asr",
            "model_name": "fake",
            "model_version": "test",
            "device": "cpu",
            "compute_type": "test",
            "gpu_available": False,
            "gpu_required": False,
            "warmed": True,
            "ready": True,
        }

    def ingest_audio(self, config, audio_frame):
        return []

    def finalize(self, config):
        from transcription_service.protocol import transcript_event

        return [
            transcript_event(
                "final_transcript",
                segment_id=f"{config.session_id}-segment-1",
                start_time_ms=0,
                end_time_ms=4_000,
                text="First speaker explains the plan clearly. Second speaker challenges the timeline directly.",
                is_final=True,
                speaker_id=None,
                language_mode=config.language_mode,
                model_name="fake",
                model_version="test",
            )
        ]


class DevanagariSentenceFinalASRProvider(SentenceFinalASRProvider):
    def finalize(self, config):
        from transcription_service.protocol import transcript_event

        return [
            transcript_event(
                "final_transcript",
                segment_id=f"{config.session_id}-segment-1",
                start_time_ms=0,
                end_time_ms=4_000,
                text="पहला वक्ता योजना समझाता है। दूसरा वक्ता समयसीमा पर सवाल करता है।",
                is_final=True,
                speaker_id=None,
                language_mode=config.language_mode,
                model_name="fake",
                model_version="test",
            )
        ]


class TwoSpeakerDiarizationProvider:
    def warmup(self):
        return None

    def runtime_status(self):
        return {
            "provider": "fake-diarization",
            "model_name": "fake",
            "model_version": "test",
            "device": "cpu",
            "dependencies_available": True,
            "auth_token_configured": True,
            "warmed": True,
            "ready": True,
        }

    def diarize(self, config, pcm_s16le):
        return [
            SpeakerTurn(speaker_id="1", start_time_ms=0, end_time_ms=2_000),
            SpeakerTurn(speaker_id="2", start_time_ms=2_000, end_time_ms=4_000),
        ]


class FailingWarmupDiarizationProvider(TwoSpeakerDiarizationProvider):
    def warmup(self):
        raise DiarizationProviderError("could not load pyannote pipeline: FakeGatedRepoError") from FakeGatedRepoError(
            "gated"
        )


class FakeGatedRepoError(RuntimeError):
    pass


class FakeHTTPResponse:
    def __init__(self, payload):
        self.payload = payload

    def __enter__(self):
        return self

    def __exit__(self, exc_type, exc, traceback):
        return False

    def read(self):
        return json.dumps(self.payload).encode("utf-8")


if __name__ == "__main__":
    unittest.main()
