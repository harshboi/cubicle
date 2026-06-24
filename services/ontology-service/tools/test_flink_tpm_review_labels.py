#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import pathlib
import sqlite3
import sys
import unittest

import pandas as pd


TOOLS_DIR = pathlib.Path(__file__).parent


def load_tool(name: str):
    module_path = TOOLS_DIR / f"{name}.py"
    spec = importlib.util.spec_from_file_location(name, module_path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


labels = load_tool("flink_tpm_review_labels")


class MeasurementQueueTest(unittest.TestCase):
    def test_explicit_measurement_flag_does_not_promote_smoke_label(self) -> None:
        self.assertFalse(
            labels.measurement_eligible_value(
                "true",
                "evaluation_label",
                "smoke",
                "agent_smoke",
                set(),
            )
        )
        self.assertFalse(
            labels.measurement_eligible_value(
                "true",
                "evaluation_label",
                "adversarial",
                "agent_adversarial",
                set(),
            )
        )
        self.assertFalse(
            labels.measurement_eligible_value(
                "true",
                "evaluation_label",
                "candidate",
                "manual_candidate",
                set(),
            )
        )
        self.assertTrue(
            labels.measurement_eligible_value(
                "true",
                "evaluation_label",
                "gold",
                "agent_gold",
                set(),
            )
        )
        self.assertFalse(
            labels.measurement_eligible_value(
                "false",
                "evaluation_label",
                "gold",
                "agent_gold",
                set(),
            )
        )
        self.assertFalse(
            labels.measurement_eligible_value(
                "true",
                "evaluation_label",
                "candidate",
                "source_oracle_seed",
                {"source_oracle_seed"},
            )
        )
        self.assertFalse(
            labels.measurement_eligible_value(
                "false",
                "evaluation_label",
                "candidate",
                "source_oracle_seed",
                {"source_oracle_seed"},
            )
        )

    def test_stored_measurement_flag_does_not_override_label_quality(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_review_tables(conn)
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, subject_kind, subject_key, severity,
              producer_state, score, confidence, source_system, source_instance,
              external_kind
            ) values
              (1, 'insight:smoke', 'blocker_candidate', 'pull_request', 'repo/example#1', 'high',
               'current', 90, 0.9, 'cubicle_analytics', 'fixture-source', 'tpm_insight'),
              (2, 'insight:gold', 'blocker_candidate', 'pull_request', 'repo/example#2', 'high',
               'current', 91, 0.95, 'cubicle_analytics', 'fixture-source', 'tpm_insight'),
              (3, 'insight:adversarial', 'blocker_candidate', 'pull_request', 'repo/example#3', 'high',
               'current', 92, 0.96, 'cubicle_analytics', 'fixture-source', 'tpm_insight')
            """
        )
        conn.execute(
            """
            insert into work_insight_reviews (
              id, key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, owner_key, next_action, rationale,
              reviewed_at, source_system, external_id, source_url
            ) values
              (10, 'review:smoke', 1, 'evaluation_label', 'accepted', 'true_positive',
               'actionable', 'agent_smoke', 'smoke', 1, 'imported', 'smoke-agent',
               '', '', '', '2026-06-22T00:00:00+00:00', 'cubicle_evaluation',
               'smoke', 'labels.tsv'),
              (11, 'review:gold', 2, 'evaluation_label', 'accepted', 'true_positive',
               'actionable', 'agent_gold', 'gold', 0, 'imported', 'gold-agent',
               '', '', '', '2026-06-22T00:00:00+00:00', 'cubicle_evaluation',
               'gold', 'labels.tsv'),
              (12, 'review:adversarial', 3, 'evaluation_label', 'accepted', 'false_positive',
               'not_actionable', 'agent_adversarial', 'adversarial', 1, 'imported', 'adversarial-agent',
               '', '', '', '2026-06-22T00:00:00+00:00', 'cubicle_evaluation',
               'adversarial', 'labels.tsv')
            """
        )

        default_rows = labels.read_evaluation_labels(conn, "fixture-source", set())
        default_by_key = {row.insight_key: row.measurement_eligible for row in default_rows.itertuples(index=False)}
        self.assertEqual(default_by_key["insight:smoke"], "false")
        self.assertEqual(default_by_key["insight:gold"], "false")
        self.assertEqual(default_by_key["insight:adversarial"], "false")

        promoted_rows = labels.read_evaluation_labels(conn, "fixture-source", {"agent_smoke", "agent_adversarial"})
        promoted_by_key = {row.insight_key: row.measurement_eligible for row in promoted_rows.itertuples(index=False)}
        self.assertEqual(promoted_by_key["insight:smoke"], "false")
        self.assertEqual(promoted_by_key["insight:adversarial"], "false")

    def test_auto_quality_detects_adversarial_labels(self) -> None:
        self.assertEqual(
            labels.resolve_label_quality("auto", "agent_adversarial_review", pathlib.Path("labels.tsv"), "judge"),
            "adversarial",
        )
        row = pd.Series(
            {
                "review_kind": "evaluation_label",
                "label_quality": "",
                "reviewer_kind": "imported",
                "label_set": "manual_eval",
                "reviewer_key": "adversarial-judge",
                "label_source_url": "",
                "rationale": "Adversarial challenge: likely false positive.",
            }
        )
        self.assertEqual(labels.infer_label_quality_from_review(row), "adversarial")

    def test_import_accepts_filled_measurement_queue_gold_columns(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_review_tables(conn)
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, subject_kind, subject_key, severity,
              producer_state, score, confidence, source_system, source_instance,
              external_kind, source_url
            ) values (
              1, 'insight:risk', 'forecast_risk', 'pull_request', 'repo/example#7',
              'critical', 'current', 95, 0.8, 'cubicle_analytics',
              'fixture-source', 'tpm_insight', 'https://github.com/repo/example/pull/7'
            )
            """
        )
        queue_row = pd.DataFrame(
            [
                {
                    "insight_key": "insight:risk",
                    "gold_truth_label": "partial",
                    "gold_actionability_label": "needs_owner",
                    "gold_review_state": "needs_more_data",
                    "gold_owner_key": "github:owner",
                    "gold_next_action": "Ask owner whether this is still active.",
                    "gold_rationale": "Risk is real but needs source context before escalation.",
                }
            ]
        )

        imported = labels.import_labels(
            conn,
            queue_row,
            "fixture-source",
            "manual_gold_queue",
            "gold",
            set(),
            "harsh",
            labels.parse_dt("2026-06-22T00:00:00+00:00"),
            pathlib.Path("queue.tsv"),
        )

        self.assertEqual(imported, 1)
        rows = labels.read_evaluation_labels(conn, "fixture-source", set())
        self.assertEqual(len(rows), 1)
        row = rows.iloc[0]
        self.assertEqual(row["truth_label"], "partial")
        self.assertEqual(row["actionability_label"], "needs_owner")
        self.assertEqual(row["review_state"], "needs_more_data")
        self.assertEqual(row["owner_key"], "github:owner")
        self.assertEqual(row["measurement_eligible"], "true")

    def test_developer_correlation_uses_specific_actionability_bucket(self) -> None:
        template = pd.DataFrame(
            [
                {
                    "insight_key": "work-insight:developer-correlation",
                    "insight_kind": "developer_correlation",
                    "subject_kind": "unknown",
                    "subject_key": "person:jira:owner",
                    "severity": "high",
                    "producer_state": "current",
                    "title": "Same-window Jira load near PR owner",
                    "details": "Direct identity bridge with same-window Jira tickets.",
                    "recommended_action": "Review workload/routing context.",
                    "score": 100.0,
                    "confidence": 0.7,
                    "evidence_locator_kind": "analytics_output",
                    "evidence_source_url": "report.md",
                    "evidence_excerpt": "never proves causality, ownership, or blocker absence",
                }
            ]
        )
        action_items = pd.DataFrame(
            [
                {
                    "subject_kind": "unknown",
                    "subject_key": "person:jira:owner",
                    "action_type": "review_insight",
                    "priority_score": 100.0,
                    "owner_hint": "",
                }
            ]
        )
        program_register = pd.DataFrame(
            [
                {
                    "subject_kind": "unknown",
                    "subject_key": "person:jira:owner",
                    "program_status": "needs_review",
                    "due_bucket": "now",
                    "risk_score": 100.0,
                    "owner_key": "",
                    "requested_reviewer_keys": "",
                }
            ]
        )

        queue = labels.build_measurement_queue(template, pd.DataFrame(), action_items, program_register, 10)

        self.assertEqual(len(queue), 1)
        row = queue.iloc[0]
        self.assertEqual(row["measurement_bucket"], "developer_correlation_actionability")
        self.assertEqual(row["insight_kind"], "developer_correlation")
        self.assertIn("routing, capacity, or escalation", row["review_prompt"])
        self.assertIn("do not treat it as ownership, performance", row["review_prompt"])
        self.assertEqual(row["truth_label_options"], labels.TRUTH_LABEL_OPTIONS)
        self.assertEqual(row["actionability_label_options"], labels.ACTIONABILITY_LABEL_OPTIONS)
        self.assertEqual(row["review_state_options"], labels.REVIEW_STATE_OPTIONS)
        self.assertIn("routing/capacity context only", row["promotion_guardrail"])
        self.assertIn("never use them as ownership", row["promotion_guardrail"])
        self.assertGreater(float(row["priority_score"]), 0)

    def test_measurement_template_filter_exports_only_requested_insight_kind(self) -> None:
        template = pd.DataFrame(
            [
                {
                    "insight_key": "insight:blocker",
                    "insight_kind": "blocker_candidate",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#1",
                    "severity": "high",
                    "producer_state": "current",
                    "title": "Possible blocker",
                    "score": 90.0,
                    "confidence": 0.8,
                },
                {
                    "insight_key": "insight:forecast",
                    "insight_kind": "forecast_risk",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#2",
                    "severity": "critical",
                    "producer_state": "current",
                    "title": "Forecast risk",
                    "score": 100.0,
                    "confidence": 0.7,
                },
            ]
        )

        filtered = labels.filter_measurement_template(template, {"blocker_candidate"})
        label_rows = pd.DataFrame(
            [
                {"insight_key": "insight:blocker", "measurement_eligible": "false"},
                {"insight_key": "insight:forecast", "measurement_eligible": "false"},
            ]
        )
        queue = labels.build_measurement_queue(filtered, pd.DataFrame(), pd.DataFrame(), pd.DataFrame(), 10)
        summary = labels.build_measurement_queue_summary(filtered, label_rows, queue)

        self.assertEqual(filtered["insight_key"].tolist(), ["insight:blocker"])
        self.assertEqual(queue["insight_kind"].tolist(), ["blocker_candidate"])
        self.assertEqual(queue.iloc[0]["measurement_bucket"], "blocker_precision")
        self.assertIn("Only accepted true_positive", queue.iloc[0]["promotion_guardrail"])
        self.assertIn("partial remains needs_more_data", queue.iloc[0]["promotion_guardrail"])
        filtered_queue = labels.with_queue_filter(queue, {"blocker_candidate"})
        self.assertEqual(filtered_queue.iloc[0]["queue_filter"], "insight_kind:blocker_candidate")
        summary_by_metric = {row.metric: row.value for row in summary.itertuples(index=False)}
        self.assertEqual(summary_by_metric["current_insight_count"], "1")
        self.assertEqual(summary_by_metric["non_measurement_label_count"], "1")
        self.assertEqual(summary_by_metric["queue_blocker_precision"], "1")


def create_minimal_review_tables(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table work_insights (
          id integer primary key,
          key text,
          insight_kind text,
          subject_kind text,
          subject_key text,
          severity text,
          producer_state text,
          score real,
          confidence real,
          source_system text,
          source_instance text,
          external_kind text,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table work_insight_reviews (
          id integer primary key,
          key text,
          work_insight_id integer,
          review_kind text,
          review_state text,
          truth_label text,
          actionability_label text,
          label_set text,
          label_quality text,
          measurement_eligible integer,
          reviewer_kind text,
          reviewer_key text,
          owner_key text,
          next_action text,
          rationale text,
          reviewed_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          created_at text,
          updated_at text,
          unique(source_system, source_instance, external_kind, external_id)
        )
        """
    )


if __name__ == "__main__":
    unittest.main()
