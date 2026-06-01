from __future__ import annotations

import os
import unittest

from fastapi.testclient import TestClient

from transcription_service.text_intelligence import OpenAICompatibleTextIntelligenceProvider, TextIntelligenceSettings
from transcription_service.text_intelligence_worker import create_app, worker_readiness_payload


class TextIntelligenceWorkerTests(unittest.TestCase):
    def setUp(self) -> None:
        self.previous_warmup = os.environ.get("TEXT_INTELLIGENCE_WARMUP_ENABLED")
        os.environ["TEXT_INTELLIGENCE_WARMUP_ENABLED"] = "false"

    def tearDown(self) -> None:
        if self.previous_warmup is None:
            os.environ.pop("TEXT_INTELLIGENCE_WARMUP_ENABLED", None)
        else:
            os.environ["TEXT_INTELLIGENCE_WARMUP_ENABLED"] = self.previous_warmup

    def test_worker_readiness_is_metadata_only(self):
        payload = worker_readiness_payload(TextIntelligenceSettings(provider="mock", model="test-model"))

        self.assertEqual(payload["status"], "ok")
        self.assertEqual(payload["service"], "cubicle-transcription-text-intelligence-worker")
        self.assertEqual(payload["provider"], "mock")
        self.assertEqual(payload["model"], "test-model")
        self.assertEqual(payload["retention"], "disabled")

    def test_worker_rejects_missing_auth_token(self):
        app = create_app(
            settings=TextIntelligenceSettings(provider="mock", model="test-model"),
            auth_token="worker-token",
        )

        with TestClient(app) as client:
            response = client.post(
                "/v1/translate-line",
                json={"segment_id": "seg1", "target_line": "こんにちは", "previous_lines": []},
            )

        self.assertEqual(response.status_code, 401)

    def test_worker_translates_line_with_context_count(self):
        app = create_app(
            settings=TextIntelligenceSettings(provider="mock", model="test-model"),
            auth_token="worker-token",
        )

        with TestClient(app) as client:
            response = client.post(
                "/v1/translate-line",
                headers={"Authorization": "Bearer worker-token"},
                json={
                    "segment_id": "seg1",
                    "target_line": "Schedule the launch",
                    "previous_lines": ["Context one", "Context two"],
                },
            )

        self.assertEqual(response.status_code, 200)
        payload = response.json()
        self.assertEqual(payload["segment_id"], "seg1")
        self.assertEqual(payload["source_text"], "Schedule the launch")
        self.assertEqual(payload["text"], "Schedule the launch")
        self.assertEqual(payload["translation_model"], "test-model")
        self.assertEqual(payload["context_line_count"], 2)

    def test_worker_summarizes_transcript(self):
        app = create_app(
            settings=TextIntelligenceSettings(provider="mock", model="test-model"),
            auth_token="worker-token",
        )

        with TestClient(app) as client:
            response = client.post(
                "/v1/summarize-transcript",
                headers={"Authorization": "Bearer worker-token"},
                json={"lines": ["Discussed migration plan.", "Owner will follow up."]},
            )

        self.assertEqual(response.status_code, 200)
        payload = response.json()
        self.assertEqual(payload["summary"], "Discussed migration plan.")
        self.assertEqual(payload["model"], "test-model")
        self.assertEqual(payload["action_items"], [])

    def test_openai_provider_strips_translation_boilerplate(self):
        provider = FakeOpenAITextIntelligenceProvider(
            settings=TextIntelligenceSettings(provider="vllm", model="test-model"),
            response="Here is the English translation of the TARGET text: The meeting starts now.",
        )

        payload = provider.translate_line(previous_lines=[], target_line="会議が始まります", target_language="en")

        self.assertEqual(payload["text"], "The meeting starts now.")
        self.assertEqual(payload["model"], "test-model")

    def test_openai_provider_strips_wrapping_quotes(self):
        provider = FakeOpenAITextIntelligenceProvider(
            settings=TextIntelligenceSettings(provider="vllm", model="test-model"),
            response='"Translation: The room is ready."',
        )

        payload = provider.translate_line(previous_lines=[], target_line="部屋の準備ができました", target_language="en")

        self.assertEqual(payload["text"], "The room is ready.")


class FakeOpenAITextIntelligenceProvider(OpenAICompatibleTextIntelligenceProvider):
    def __init__(self, *, settings: TextIntelligenceSettings, response: str) -> None:
        super().__init__(settings=settings)
        self.response = response

    def _chat_completion(self, *, system: str, user: str, max_tokens: int, timeout: float) -> str:
        return self.response


if __name__ == "__main__":
    unittest.main()
