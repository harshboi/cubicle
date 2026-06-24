#!/usr/bin/env python3

from __future__ import annotations

import csv
import importlib.util
import pathlib
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("flink_tpm_source_oracle_labels.py")
SPEC = importlib.util.spec_from_file_location("flink_tpm_source_oracle_labels", MODULE_PATH)
oracle = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(oracle)


class SourceOracleLabelsTest(unittest.TestCase):
    def test_forecast_risk_label_requires_open_observed_overdue_pr(self) -> None:
        insight = forecast_insight("apache/flink-kubernetes-operator#1043")
        summary = {"eta_forecast_ready": "false"}
        forecast = {
            "state": "open",
            "source_current_coverage_state": "observed",
            "source_current_detail_state": "observed",
            "age_days": "198.3",
            "predicted_total_cycle_days": "12.74",
            "overdue_days": "185.56",
            "risk_score": "100",
        }

        label = oracle.forecast_risk_label(insight, summary, forecast)

        self.assertIsNotNone(label)
        assert label is not None
        self.assertEqual(label["truth_label"], "true_positive")
        self.assertEqual(label["actionability_label"], "needs_owner")
        self.assertEqual(label["review_state"], "accepted")
        self.assertEqual(label["measurement_eligible"], "false")
        self.assertIn("not an ETA commitment", label["next_action"])
        self.assertIn("not an independent precision label", label["rationale"])
        self.assertIn("age 198.30d exceeds", label["rationale"])

    def test_forecast_risk_skips_failed_or_not_overdue_source(self) -> None:
        insight = forecast_insight("apache/flink-kubernetes-operator#1043")
        summary = {"eta_forecast_ready": "false"}
        failed = {
            "state": "open",
            "source_current_coverage_state": "observed",
            "source_current_detail_state": "failed",
            "age_days": "198.3",
            "predicted_total_cycle_days": "12.74",
            "overdue_days": "185.56",
            "risk_score": "100",
        }
        not_overdue = {
            "state": "open",
            "source_current_coverage_state": "observed",
            "source_current_detail_state": "observed",
            "age_days": "2.0",
            "predicted_total_cycle_days": "12.74",
            "overdue_days": "0",
            "risk_score": "40",
        }

        self.assertIsNone(oracle.forecast_risk_label(insight, summary, failed))
        self.assertIsNone(oracle.forecast_risk_label(insight, summary, not_overdue))

    def test_model_quality_label_uses_backtest_loss_to_baseline(self) -> None:
        insight = {
            "insight_key": "work-insight:model-quality",
            "insight_kind": "model_quality",
            "subject_kind": "unknown",
            "subject_key": "flink-pr-cycle-forecast",
        }
        summary = {
            "eta_forecast_ready": "false",
            "backtest_best_model": "median_cycle_baseline",
            "backtest_median_mae_days": "6.53",
            "backtest_random_forest_mae_days": "8.83",
        }

        label = oracle.model_quality_label(insight, summary)

        self.assertIsNotNone(label)
        assert label is not None
        self.assertEqual(label["truth_label"], "true_positive")
        self.assertEqual(label["actionability_label"], "actionable")
        self.assertEqual(label["measurement_eligible"], "false")
        self.assertIn("not field precision", label["rationale"])
        self.assertIn("does not beat the median baseline", label["rationale"])

    def test_blocker_candidate_label_is_partial_owner_confirmation(self) -> None:
        insight = {
            "insight_key": "work-insight:blocker",
            "insight_kind": "blocker_candidate",
            "subject_kind": "pull_request",
            "subject_key": "apache/flink-kubernetes-operator#1079",
            "title": "Possible blocker signal: stuck",
            "evidence_locator_kind": "github_pull_body",
            "evidence_excerpt": "Finalizer prevents the CR from being removed, causing the resource to be stuck.",
        }
        forecast = {"state": "open", "source_current_detail_state": "observed"}
        action = {
            "current_state": "open",
            "owner_hint": "github:owner",
            "source_coverage_kind": "authenticated_api_current_observation",
        }

        label = oracle.blocker_candidate_source_label(insight, forecast, action)

        self.assertIsNotNone(label)
        assert label is not None
        self.assertEqual(label["truth_label"], "partial")
        self.assertEqual(label["actionability_label"], "needs_owner")
        self.assertEqual(label["review_state"], "needs_more_data")
        self.assertEqual(label["measurement_eligible"], "false")
        self.assertEqual(label["owner_key"], "github:owner")
        self.assertIn("not owner-confirmed blocker", label["rationale"])
        self.assertIn("signal=stuck", label["evidence_summary"])

    def test_blocker_candidate_label_requires_open_source_excerpt(self) -> None:
        insight = {
            "insight_key": "work-insight:blocker",
            "insight_kind": "blocker_candidate",
            "subject_kind": "pull_request",
            "subject_key": "apache/flink-kubernetes-operator#1079",
            "title": "Possible blocker signal: stuck",
        }

        self.assertIsNone(oracle.blocker_candidate_source_label(insight, {"state": "open"}, {}))
        self.assertIsNone(
            oracle.blocker_candidate_source_label(
                {**insight, "evidence_excerpt": "stuck signal"},
                {"state": "closed"},
                {},
            )
        )

    def test_ci_status_label_keeps_required_check_guardrail(self) -> None:
        insight = status_insight("apache/flink-kubernetes-operator#1084")
        observation = {
            "effective_state": "open",
            "combined_signal": "failing_checks",
            "required_check_coverage_state": "required_checks_unavailable",
            "failing_context_count": "78",
            "pending_context_count": "0",
        }
        action = {"owner_hint": "github:vsantwana"}

        label = oracle.ci_status_label(insight, observation, action)

        self.assertIsNotNone(label)
        assert label is not None
        self.assertEqual(label["truth_label"], "true_positive")
        self.assertEqual(label["actionability_label"], "needs_owner")
        self.assertEqual(label["owner_key"], "github:vsantwana")
        self.assertIn("not a merge-blocker claim", label["rationale"])
        self.assertIn("required_checks_unavailable", label["evidence_summary"])

    def test_reviewer_wait_label_is_owner_confirmation_not_inactivity(self) -> None:
        insight = status_insight("apache/flink-kubernetes-operator#1133")
        forecast = {
            "state": "open",
            "source_current_detail_state": "observed",
            "requested_reviewer_count": "1",
            "requested_reviewers": "Dennis-Mircea,spuru9",
        }
        action = {"owner_hint": "github:spuru9"}

        label = oracle.reviewer_wait_status_label(insight, forecast, action)

        self.assertIsNotNone(label)
        assert label is not None
        self.assertEqual(label["truth_label"], "true_positive")
        self.assertEqual(label["actionability_label"], "needs_owner")
        self.assertIn("not reviewer inactivity", label["rationale"])

    def test_dependency_cluster_label_is_partial_coordination_lead(self) -> None:
        insight = {
            "insight_key": "work-insight:dependency",
            "insight_kind": "dependency_cluster",
            "subject_kind": "ticket",
            "subject_key": "FLINK-34961",
            "title": "Ticket spans 14 PRs: GitHub Actions runner statistics",
        }

        label = oracle.dependency_cluster_source_label(
            insight,
            {"ticket_pr_count": 14, "partial_remote_link_count": 13},
        )

        self.assertIsNotNone(label)
        assert label is not None
        self.assertEqual(label["truth_label"], "partial")
        self.assertEqual(label["actionability_label"], "needs_owner")
        self.assertEqual(label["review_state"], "needs_more_data")
        self.assertEqual(label["measurement_eligible"], "false")
        self.assertIn("coordination-review lead only", label["rationale"])
        self.assertIn("not a source-confirmed blocking dependency", label["rationale"])
        self.assertIn("ticket_pr_edges=14", label["evidence_summary"])

    def test_dependency_cluster_label_requires_ticket_topology(self) -> None:
        insight = {
            "insight_key": "work-insight:dependency",
            "insight_kind": "dependency_cluster",
            "subject_kind": "ticket",
            "subject_key": "FLINK-34961",
        }

        self.assertIsNone(oracle.dependency_cluster_source_label(insight, {"ticket_pr_count": 2}))
        self.assertIsNone(oracle.dependency_cluster_source_label({**insight, "subject_kind": "pull_request"}, {"ticket_pr_count": 14}))

    def test_dependency_edges_by_ticket_counts_partial_remote_links(self) -> None:
        rows = [
            {"edge_kind": "ticket_pr", "source_key": "ticket:FLINK-1", "target_key": "pr:repo/example#1", "risk_signal": "partial_remote_link"},
            {"edge_kind": "ticket_pr", "source_key": "ticket:FLINK-1", "target_key": "pr:repo/example#2", "risk_signal": ""},
            {"edge_kind": "workstream_component", "source_key": "component:1", "target_key": "ticket:FLINK-1", "risk_signal": "multi_object_workstream"},
        ]

        summary = oracle.dependency_edges_by_ticket(rows)

        self.assertEqual(summary["FLINK-1"], {"ticket_pr_count": 2, "partial_remote_link_count": 1})

    def test_developer_correlation_label_is_partial_workload_context(self) -> None:
        insight = {
            "insight_key": "work-insight:developer",
            "insight_kind": "developer_correlation",
            "subject_kind": "unknown",
            "subject_key": "person:jira:owner",
        }
        row = {
            "person_key": "person:jira:owner",
            "display_name": "Owner One",
            "identity_bridge_state": "direct_github_jira_person",
            "correlation_state": "correlatable_same_identity",
            "pr_authored_count": "8",
            "open_pr_authored_count": "2",
            "high_risk_open_pr_count": "1",
            "extra_jira_ticket_count": "13",
            "open_extra_jira_ticket_count": "5",
            "extra_jira_blocker_ticket_count": "2",
        }

        label = oracle.developer_correlation_source_label(insight, row)

        self.assertIsNotNone(label)
        assert label is not None
        self.assertEqual(label["truth_label"], "partial")
        self.assertEqual(label["actionability_label"], "needs_owner")
        self.assertEqual(label["review_state"], "needs_more_data")
        self.assertEqual(label["measurement_eligible"], "false")
        self.assertIn("workload/routing review lead only", label["rationale"])
        self.assertIn("not causality", label["rationale"])
        self.assertIn("pr_authored=8", label["evidence_summary"])

    def test_developer_correlation_label_requires_direct_identity_and_overlap(self) -> None:
        insight = {
            "insight_key": "work-insight:developer",
            "insight_kind": "developer_correlation",
            "subject_kind": "unknown",
            "subject_key": "person:jira:owner",
        }
        direct = {
            "identity_bridge_state": "direct_github_jira_person",
            "correlation_state": "correlatable_same_identity",
            "pr_authored_count": "8",
            "extra_jira_ticket_count": "13",
        }

        self.assertIsNone(oracle.developer_correlation_source_label(insight, {**direct, "identity_bridge_state": "github_only_person"}))
        self.assertIsNone(oracle.developer_correlation_source_label(insight, {**direct, "correlation_state": "pr_author_without_extra_jira_signal"}))
        self.assertIsNone(oracle.developer_correlation_source_label(insight, {**direct, "extra_jira_ticket_count": "0"}))

    def test_build_source_oracle_labels_labels_context_leads_with_boundaries(self) -> None:
        labels = oracle.build_source_oracle_labels(
            [
                {
                    "insight_key": "work-insight:blocker",
                    "insight_kind": "blocker_candidate",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1079",
                    "title": "Possible blocker signal: stuck",
                    "evidence_excerpt": "The CR is permanently stuck in terminating state.",
                },
                {
                    "insight_key": "work-insight:dev-correlation",
                    "insight_kind": "developer_correlation",
                    "subject_kind": "unknown",
                    "subject_key": "person:jira:owner",
                },
                {
                    "insight_key": "work-insight:dependency",
                    "insight_kind": "dependency_cluster",
                    "subject_kind": "ticket",
                    "subject_key": "FLINK-34961",
                },
            ],
            {},
            {"apache/flink-kubernetes-operator#1079": {"state": "open", "source_current_detail_state": "observed"}},
            {},
            {},
            {"FLINK-34961": {"ticket_pr_count": 14, "partial_remote_link_count": 13}},
            {
                "person:jira:owner": {
                    "person_key": "person:jira:owner",
                    "identity_bridge_state": "direct_github_jira_person",
                    "correlation_state": "correlatable_same_identity",
                    "pr_authored_count": "8",
                    "extra_jira_ticket_count": "13",
                }
            },
        )

        self.assertEqual(len(labels), 3)
        self.assertEqual(labels[0]["insight_kind"], "blocker_candidate")
        self.assertEqual(labels[1]["insight_kind"], "developer_correlation")
        self.assertEqual(labels[2]["insight_kind"], "dependency_cluster")

    def test_write_tsv_outputs_importable_columns(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            output = pathlib.Path(tmp) / "oracle.tsv"
            row = oracle.base_label(
                forecast_insight("apache/flink-kubernetes-operator#1043"),
                truth_label="true_positive",
                actionability_label="needs_owner",
                review_state="accepted",
                next_action="Ask owner.",
                rationale="Source oracle.",
                oracle_kind="forecast_risk_source_state",
                evidence_summary="state=open",
            )

            oracle.write_tsv([row], output)

            with output.open(newline="", encoding="utf-8") as handle:
                reader = csv.DictReader(handle, delimiter="\t")
                self.assertEqual(reader.fieldnames, oracle.OUTPUT_COLUMNS)
                written = list(reader)
            self.assertEqual(len(written), 1)
            self.assertEqual(written[0]["measurement_eligible"], "false")


def forecast_insight(subject_key: str) -> dict[str, str]:
    return {
        "insight_key": f"work-insight:forecast:{subject_key}",
        "insight_kind": "forecast_risk",
        "subject_kind": "pull_request",
        "subject_key": subject_key,
    }


def status_insight(subject_key: str) -> dict[str, str]:
    return {
        "insight_key": f"work-insight:status:{subject_key}",
        "insight_kind": "status_summary",
        "subject_kind": "pull_request",
        "subject_key": subject_key,
    }


if __name__ == "__main__":
    unittest.main()
