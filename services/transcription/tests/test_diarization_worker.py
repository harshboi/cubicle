from __future__ import annotations

import base64
import unittest

from transcription_service.diarization import DiarizationProviderSettings
from transcription_service.diarization_worker import (
    _audio_from_payload,
    _config_from_payload,
    worker_readiness_payload,
)


class DiarizationWorkerTests(unittest.TestCase):
    def test_worker_job_payload_decodes_session_metadata_and_audio(self):
        payload = {
            "protocol_version": "transcription.v1",
            "session_id": "session-abc",
            "diarization_enabled": True,
            "language_mode": "english_to_english",
            "sample_rate": 16000,
            "channel_count": 1,
            "audio_encoding": "pcm_s16le",
            "client_timestamp": "2026-05-17T17:00:00.000Z",
            "audio_b64": base64.b64encode(bytes([0, 0, 1, 0])).decode("ascii"),
        }

        config = _config_from_payload(payload)
        audio = _audio_from_payload(payload)

        self.assertEqual(config.session_id, "session-abc")
        self.assertTrue(config.diarization_enabled)
        self.assertEqual(audio, bytes([0, 0, 1, 0]))

    def test_worker_readiness_is_metadata_only(self):
        payload = worker_readiness_payload(
            DiarizationProviderSettings(
                provider="pyannote",
                model_name="pyannote/speaker-diarization-community-1",
                model_version="pyannote-audio-4.x",
                device="cuda",
            )
        )

        self.assertEqual(payload["status"], "ok")
        self.assertEqual(payload["service"], "cubicle-transcription-diarization-worker")
        self.assertEqual(payload["diarization_provider"], "pyannote")
        self.assertEqual(payload["device"], "cuda")
        self.assertEqual(payload["retention"], "disabled")

    def test_worker_rejects_invalid_audio_payload(self):
        with self.assertRaises(ValueError):
            _audio_from_payload({"audio_b64": "not valid base64"})


if __name__ == "__main__":
    unittest.main()
