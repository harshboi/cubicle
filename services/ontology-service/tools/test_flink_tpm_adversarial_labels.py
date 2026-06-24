#!/usr/bin/env python3

from __future__ import annotations

import csv
import importlib.util
import pathlib
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("flink_tpm_adversarial_labels.py")
SPEC = importlib.util.spec_from_file_location("flink_tpm_adversarial_labels", MODULE_PATH)
adversarial = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(adversarial)


class AdversarialLabelsTest(unittest.TestCase):
    def test_forecast_risk_stays_risk_triage_not_eta_forecast(self) -> None:
        row = {
            "queue_rank": "1",
            "measurement_bucket": "risk_actionability",
            "insight_key": "work-insight:risk",
            "insight_kind": "forecast_risk",
            "subject_kind": "pull_request",
            "subject_key": "apache/flink-kubernetes-operator#1043",
            "action_type": "decision_or_owner_followup",
            "title": "Very old open PR",
            "owner_key": "github:owner",
            "evidence_excerpt": "Age 198.3d. This is age/staleness triage, not an ETA.",
        }

        label = adversarial.label_row(row)

        self.assertEqual(label["truth_label"], "partial")
        self.assertEqual(label["actionability_label"], "needs_owner")
        self.assertEqual(label["review_state"], "needs_more_data")
        self.assertIn("ETA readiness is false", label["rationale"])
        self.assertIn("risk triage only", label["next_action"])

    def test_negated_blocker_keyword_is_dismissed(self) -> None:
        row = {
            "queue_rank": "2",
            "measurement_bucket": "blocker_truth",
            "insight_key": "work-insight:blocker",
            "insight_kind": "blocker_candidate",
            "subject_kind": "pull_request",
            "subject_key": "apache/flink-kubernetes-operator#1010",
            "action_type": "clear_blocker",
            "title": "Mentions blocker",
            "evidence_excerpt": "There is no blocker for this change and it is not failing.",
        }

        label = adversarial.label_row(row)

        self.assertEqual(label["truth_label"], "false_positive")
        self.assertEqual(label["actionability_label"], "not_actionable")
        self.assertEqual(label["review_state"], "dismissed")
        self.assertIn("negated context", label["rationale"])

    def test_developer_correlation_is_context_not_causality(self) -> None:
        row = {
            "queue_rank": "3",
            "measurement_bucket": "developer_correlation_truth",
            "insight_key": "work-insight:developer-correlation",
            "insight_kind": "developer_correlation",
            "subject_kind": "unknown",
            "subject_key": "person:jira:owner",
            "action_type": "review_workload_context",
            "title": "Same-window Jira load near PR owner",
            "evidence_excerpt": "Direct identity bridge with same-window Jira tickets.",
        }

        label = adversarial.label_row(row)

        self.assertEqual(label["truth_label"], "partial")
        self.assertEqual(label["actionability_label"], "needs_owner")
        self.assertEqual(label["review_state"], "needs_more_data")
        self.assertIn("weak", label["rationale"])
        self.assertIn("contextual only", label["rationale"])
        self.assertIn("do not infer ownership, causality", label["next_action"])

    def test_write_tsv_outputs_importable_review_columns(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            output = root / "labels.tsv"
            rows = adversarial.build_adversarial_labels(
                [
                    {
                        "queue_rank": "1",
                        "measurement_bucket": "ci_actionability",
                        "insight_key": "work-insight:ci",
                        "insight_kind": "ci_check_followup",
                        "subject_kind": "pull_request",
                        "subject_key": "apache/flink-kubernetes-operator#1020",
                        "action_type": "ci_check_followup",
                        "title": "Failing CI check",
                        "owner_key": "github:owner",
                    }
                ],
                limit=5,
            )

            adversarial.write_tsv(rows, output)

            with output.open(newline="", encoding="utf-8") as handle:
                reader = csv.DictReader(handle, delimiter="\t")
                self.assertEqual(reader.fieldnames, adversarial.OUTPUT_COLUMNS)
                written = list(reader)
            self.assertEqual(len(written), 1)
            self.assertEqual(written[0]["truth_label"], "partial")
            self.assertEqual(written[0]["review_state"], "needs_more_data")
            self.assertIn("required-check coverage is unavailable", written[0]["rationale"])


if __name__ == "__main__":
    unittest.main()
