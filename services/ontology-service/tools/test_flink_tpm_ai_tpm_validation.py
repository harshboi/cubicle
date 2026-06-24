#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import pathlib
import sqlite3
import sys
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("flink_tpm_ai_tpm_validation.py")
SPEC = importlib.util.spec_from_file_location("flink_tpm_ai_tpm_validation", MODULE_PATH)
validation = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules["flink_tpm_ai_tpm_validation"] = validation
SPEC.loader.exec_module(validation)


class AITPMValidationReportTest(unittest.TestCase):
    def test_not_observed_counts_as_limited_source_coverage(self) -> None:
        self.assertTrue(validation.coverage_state_limited("not_observed"))

    def test_decision_target_report_rows_keep_guardrail_visible_under_limit(self) -> None:
        rows = [
            {
                "evaluation": "source_event_as_of_grouped_kfold",
                "model": f"random_forest_classifier_fold_{index}",
                "coverage_stratum": "coverage=observed",
            }
            for index in range(12)
        ]
        rows.append(
            {
                "evaluation": "source_event_as_of_coverage_stratified_summary",
                "model": "coverage_guardrail",
                "coverage_stratum": "not_testable_single_stratum",
            }
        )

        report_rows = validation.decision_target_report_rows(
            rows,
            evaluation_key="evaluation",
            model_key="model",
        )

        self.assertEqual(len(report_rows), 8)
        self.assertIn("coverage_guardrail", [row["model"] for row in report_rows])
        self.assertIn("not_testable_single_stratum", [row["coverage_stratum"] for row in report_rows])

    def test_evidence_need_relationship_rows_report_gate_and_action_links(self) -> None:
        latest = validation.LatestRun(
            source_instance="fixture-source",
            workstream_key="workstream:fixture",
            generated_at="2026-06-23T03:10:00+00:00",
            run_key="run:fixture",
            readiness_state="blocked",
            readiness_score=0.0,
            human_review_required=True,
            autonomous_action_ready=False,
            evidence_need_count=2,
            blocking_gate_count=2,
            tpm_function_count=1,
            external_id="fixture",
        )
        with sqlite3.connect(":memory:") as conn:
            conn.executescript(
                """
                create table work_program_quality_gates (
                  id integer primary key,
                  source_instance text,
                  workstream_key text,
                  generated_at text,
                  blocking integer
                );
                create table work_program_evidence_needs (
                  id integer primary key,
                  source_instance text,
                  workstream_key text,
                  generated_at text,
                  quality_gate_id integer,
                  work_action_id integer
                );
                insert into work_program_quality_gates values
                  (1, 'fixture-source', 'fixture', '2026-06-23T03:10:00+00:00', 1),
                  (2, 'fixture-source', 'fixture', '2026-06-23T03:10:00+00:00', 1);
                insert into work_program_evidence_needs values
                  (10, 'fixture-source', 'fixture', '2026-06-23T03:10:00+00:00', 1, 100),
                  (11, 'fixture-source', 'fixture', '2026-06-23T03:10:00+00:00', null, null);
                """
            )
            rows = {metric: value for metric, value, _ in validation.evidence_need_relationship_rows(conn, latest)}

        self.assertEqual(rows["work_program_evidence_need_count"], "2")
        self.assertEqual(rows["evidence_needs_with_quality_gate_link"], "1")
        self.assertEqual(rows["evidence_needs_without_quality_gate_link"], "1")
        self.assertEqual(rows["evidence_needs_with_work_action_link"], "1")
        self.assertEqual(rows["blocking_quality_gates_without_evidence_need"], "1")

    def test_autonomous_verdict_requires_current_blockers_to_clear(self) -> None:
        latest = validation.LatestRun(
            source_instance="fixture-source",
            workstream_key="workstream:fixture",
            generated_at="2026-06-23T03:10:00+00:00",
            run_key="run:fixture",
            readiness_state="autonomous_ready",
            readiness_score=100.0,
            human_review_required=False,
            autonomous_action_ready=True,
            evidence_need_count=0,
            blocking_gate_count=0,
            tpm_function_count=2,
            external_id="fixture",
        )
        forecast_summary = {"eta_forecast_ready": {"value": "true"}}

        self.assertEqual(
            validation.ai_tpm_verdict(
                latest,
                [],
                [],
                [("source_sync_issue_count", "1"), ("source_coverage_evidence_need_count", "0")],
                forecast_summary,
                [],
            ),
            "operating_brief_ready_but_source_coverage_blocked",
        )
        self.assertEqual(
            validation.ai_tpm_verdict(
                latest,
                [],
                [{"check_state": "fail"}],
                [("source_sync_issue_count", "0"), ("source_coverage_evidence_need_count", "0")],
                forecast_summary,
                [],
            ),
            "supervised_ai_tpm_only",
        )
        self.assertEqual(
            validation.ai_tpm_verdict(
                latest,
                [],
                [],
                [("source_sync_issue_count", "0"), ("source_coverage_evidence_need_count", "0")],
                forecast_summary,
                [],
            ),
            "autonomous_candidate",
        )

    def test_tpm_function_readiness_relationship_rows_report_gate_links(self) -> None:
        latest = validation.LatestRun(
            source_instance="fixture-source",
            workstream_key="workstream:fixture",
            generated_at="2026-06-23T03:10:00+00:00",
            run_key="run:fixture",
            readiness_state="blocked",
            readiness_score=0.0,
            human_review_required=True,
            autonomous_action_ready=False,
            evidence_need_count=2,
            blocking_gate_count=2,
            tpm_function_count=2,
            external_id="fixture",
        )
        with sqlite3.connect(":memory:") as conn:
            conn.row_factory = sqlite3.Row
            conn.executescript(
                """
                create table work_program_tpm_function_readinesses (
                  id integer primary key,
                  source_instance text,
                  workstream_key text,
                  generated_at text,
                  blocking_gate_keys text
                );
                create table work_program_tpm_function_readiness_blocking_quality_gates (
                  work_program_tpm_function_readiness_id integer,
                  work_program_quality_gate_id integer
                );
                insert into work_program_tpm_function_readinesses values
                  (10, 'fixture-source', 'fixture', '2026-06-23T03:10:00+00:00', 'forecast_readiness'),
                  (11, 'fixture-source', 'fixture', '2026-06-23T03:10:00+00:00', 'global_insight_precision');
                insert into work_program_tpm_function_readiness_blocking_quality_gates values
                  (10, 100);
                """
            )
            rows = {metric: value for metric, value, _ in validation.tpm_function_readiness_relationship_rows(conn, latest)}

        self.assertEqual(rows["work_program_tpm_function_readiness_count"], "2")
        self.assertEqual(rows["tpm_function_readiness_with_blocking_gate_keys"], "2")
        self.assertEqual(rows["tpm_function_readiness_blocking_gate_link_count"], "1")
        self.assertEqual(rows["tpm_function_readinesses_with_blocking_gate_links"], "1")
        self.assertEqual(rows["tpm_function_readiness_without_blocking_gate_links"], "1")

    def test_report_summarizes_blocked_source_coverage_and_forecast_gating(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db, readiness_generated_at="2026-06-22T07:44:28.600243+00:00")
            seed_analytics_db(analytics_db)

            report = validation.build_validation_report(
                ontology_db,
                analytics_db,
                source_instance="fixture-source",
                workstream_key="workstream:fixture",
            )

        self.assertIn("Verdict: `operating_brief_ready_but_source_coverage_blocked`", report)
        self.assertIn("| source_sync_issue_count | 2 |", report)
        self.assertIn("| source_sync_issue_count_matching_source_instance | 0 |", report)
        self.assertIn("| source_sync_issue_count_matching_source_scope | 2 |", report)
        self.assertIn("| limited_program_item_count | 0 |", report)
        self.assertIn("| pr_source_coverage:detail_failed / failed | 2 |", report)
        self.assertIn("| truth_label_coverage | 1/3 |", report)
        self.assertIn("| ready_to_measure_precision | false |", report)
        self.assertIn("| counted_measurement_label_count | 0 |", report)
        self.assertIn("| available_label_pack_measurement_count | 4 |", report)
        self.assertIn("| measurement_label_trust_delta | 4 |", report)
        self.assertIn("| measurement_label_trust_boundary | candidate_labels_not_counted |", report)
        self.assertIn("| eta_forecast_ready | false |", report)
        self.assertIn("| eta_primary_blocker | kfold_model_does_not_beat_baseline |", report)
        self.assertIn("| eta_temporal_snapshot_state | as_of_feature_snapshot_series_missing |", report)
        self.assertIn("| risk_triage_coverage_stratified_state | not_testable_single_stratum |", report)
        self.assertIn("Forecast product reliability:", report)
        self.assertIn("| point_eta | gated | false | diagnostic_only | eta_kfold_best_candidate_improvement_pct | 7.04 | improve_model_features_and_validate_against_as_of_snapshots | Point ETA is blocked by kfold_model_does_not_beat_baseline; use forecast outputs for risk triage only. |", report)
        self.assertIn("| risk_triage | ready_with_coverage_guardrail | true | attention_ordering | risk_triage_lift_at_10pct | 0.3446 | add_more_coverage_strata_for_confounding_checks | Risk triage is product-safe only for attention ordering; it is not an ETA, causality, blocker, or autonomous action claim. |", report)
        self.assertIn("| tpm_decision_target_backtest | 2 |", report)
        self.assertIn("| work_decision_target_evaluations | 2 |", report)
        self.assertIn("TPM decision-target analytics rows:", report)
        self.assertIn("| abandonment_risk | source_event_as_of_coverage_stratified_summary | coverage_guardrail | not_testable_single_stratum |  |  | false | coverage confounding cannot be tested |", report)
        self.assertIn("| abandonment_risk | source_event_as_of_grouped_kfold | random_forest_classifier | coverage=observed | 0.5 | 0.3 | false | validation only |", report)
        self.assertIn("Persisted decision-target evaluation rows:", report)
        self.assertIn("| abandonment_risk | source_event_as_of_coverage_stratified_summary | coverage_guardrail | not_testable_single_stratum |  |  | false | validation_gated | coverage confounding cannot be tested |", report)
        self.assertIn("| work_forecast_evaluations | 2 |", report)
        self.assertIn("| work_program_run_table | missing |", report)
        self.assertIn("| missing_latest_evidence:work_actions | 0 |", report)
        self.assertIn("| unresolved_pull_request_subject:work_actions | 0 |", report)
        self.assertIn("| work_actions_with_source_insights | 1 |", report)
        self.assertIn("| work_actions_without_source_insights | 1 |", report)
        self.assertIn("| work_actions_with_forecast_evidence_links | 1 |", report)
        self.assertIn("| work_actions_without_evidence_chain | 0 |", report)
        self.assertIn("| work_action_observation_count | 2 |", report)
        self.assertIn("| work_action_observation_limited_or_auth_count | 1 |", report)
        self.assertIn("| work_action_observation_kind:source_state | 2 |", report)
        self.assertIn("| followup_fetch_success_count | 3 |", report)
        self.assertIn("| check_source_coverage_failed_count | 0 |", report)
        self.assertIn("| tpm_check_signal_readiness | 3 |", report)
        self.assertIn("| check_readiness:ci_followup_validation | ready:ready_with_current_open_signal | validation_lead: HTTP 200 check/status observations include one failing open PR. |", report)
        self.assertIn("| check_readiness:ci_eta_feature | gated:single_live_observation | eta_feature: One live observation is not a point-in-time feature series. |", report)
        self.assertIn("| tpm_transition_signal_readiness | 3 |", report)
        self.assertIn("| transition_readiness:terminal_closeout_review | gated:terminal_transition_superseded | closeout_review: Terminal transition candidates were followed by later open-state evidence. |", report)
        self.assertIn("| transition_readiness:source_resolved_closeout | gated:blocked_by_later_nonterminal_state | source_resolved_evidence: Later source state contradicts terminal transition evidence. |", report)
        self.assertIn("| missing_latest_evidence:work_dependency_edges | 1 |", report)
        self.assertIn("| relationship_evidence:work_dependency_edges.ticket_pr | 1 |", report)
        self.assertIn("| generated_evidence:work_dependency_edges.ticket_pr | 0 |", report)
        self.assertIn("| invalid_relationship_authority:work_dependency_edges | 0 |", report)
        self.assertIn("| canonical_mirror_missing_kind:work_dependency_edges | 0 |", report)
        self.assertIn("| invalid_canonical_relationship_kind:work_dependency_edges | 0 |", report)
        self.assertIn("| projection_with_canonical_kind:work_dependency_edges | 0 |", report)
        self.assertIn("| ticket_pr_not_canonical_mirror:work_dependency_edges | 0 |", report)
        self.assertIn("| canonical_mirror_missing_typed_row:work_dependency_edges.ticket_pr | 0 |", report)
        self.assertIn("| non_ticket_pr_canonical_mirror:work_dependency_edges | 0 |", report)
        self.assertIn("| work_dependency_edge_authority:canonical_mirror | 1 |", report)
        self.assertIn("| work_dependency_edge_authority:operating_projection | 1 |", report)
        self.assertIn("| work_dependency_edges_with_context_id | 2 |", report)
        self.assertIn("| unresolved_ticket_endpoint:work_dependency_edges.ticket_pr | 0 |", report)
        self.assertIn("| unresolved_pull_request_endpoint:work_dependency_edges.ticket_pr | 0 |", report)
        self.assertIn("| unresolved_target_endpoint:work_dependency_edges.workstream_cluster | 0 |", report)
        self.assertIn("| work_dependency_endpoints | 4 |", report)
        self.assertIn("| work_dependency_endpoint_expected_count | 4 |", report)
        self.assertIn("| work_dependency_endpoint_count_delta | 0 |", report)
        self.assertIn("| orphaned_work_dependency_endpoints | 0 |", report)
        self.assertIn("| invalid_endpoint_role:work_dependency_endpoints | 0 |", report)
        self.assertIn("| invalid_node_kind:work_dependency_endpoints | 0 |", report)
        self.assertIn("| invalid_resolution_state:work_dependency_endpoints | 0 |", report)
        self.assertIn("| invalid_key_only_endpoint:work_dependency_endpoints | 0 |", report)
        self.assertIn("| invalid_resolved_pointer_shape:work_dependency_endpoints | 0 |", report)
        self.assertIn("| work_dependency_endpoint_resolution:key_only | 1 |", report)
        self.assertIn("| work_dependency_endpoint_resolution:resolved | 3 |", report)
        self.assertIn("| missing_typed_target:work_dependency_endpoints | 0 |", report)
        self.assertIn("| developer_correlation_rows | 1 |", report)
        self.assertIn("| correlation_state:direct_identity_same_window | 1 |", report)
        self.assertIn("| extra_jira_ticket_count | 4 |", report)
        self.assertIn("Developer correlation validation:", report)
        self.assertIn("| spearman_open_extra_jira_vs_high_risk_open_pr | 0.09 | 56 | rank correlation over direct identity rows | Weak positive workload co-occurrence; not causality. | Same-window developer correlation is a workload/attention lead only. |", report)
        self.assertIn("| operating_brief | automatable | can_publish_operating_brief | false |", report)
        self.assertIn("| source_coverage | blocked | coverage_repair_required | true | source_coverage |", report)

    def test_report_matches_timestamp_variants_for_latest_run_rows(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(
                ontology_db,
                readiness_generated_at="2026-06-22T07:44:28.600243Z",
                row_generated_at="2026-06-22T07:44:28.600243+00:00",
            )
            seed_analytics_db(analytics_db)

            report = validation.build_validation_report(ontology_db, analytics_db)

        self.assertIn("- Latest run: `2026-06-22T07:44:28.600243Z`", report)
        self.assertIn("| source_coverage | gated | true | 2 limited items. |", report)
        self.assertIn("| source_coverage | generated_evidence | medium | qa_action_open | 1 |", report)

    def test_report_surfaces_durable_work_program_run_members(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            generated_at = "2026-06-22T07:44:28.600243+00:00"
            seed_ontology_db(ontology_db, readiness_generated_at=generated_at)
            with sqlite3.connect(ontology_db) as conn:
                conn.executescript(
                    """
                    create table work_program_runs (
                      id integer primary key,
                      run_key text unique,
                      source_instance text,
                      workstream_key text,
                      generated_at text
                    );
                    create table work_program_run_members (
                      run_key text,
                      member_table text,
                      member_id integer
                    );
                    create table work_program_milestones (
                      id integer primary key,
                      source_instance text,
                      workstream_key text,
                      generated_at text
                    );
                    """
                )
                conn.execute(
                    "insert into work_program_runs values (1, 'run:fixture', 'fixture-source', 'workstream:fixture', ?)",
                    [generated_at],
                )
                conn.execute(
                    "insert into work_program_milestones values (3, 'fixture-source', 'workstream:fixture', ?)",
                    [generated_at],
                )
                conn.executemany(
                    "insert into work_program_run_members values (?, ?, ?)",
                    [
                        ("run:fixture", "work_program_quality_gates", 1),
                        ("run:fixture", "work_program_evidence_needs", 2),
                        ("run:fixture", "work_program_milestones", 3),
                    ],
                )
            seed_analytics_db(analytics_db)

            report = validation.build_validation_report(ontology_db, analytics_db)

        self.assertIn("| work_program_run_table | present |", report)
        self.assertIn("| work_program_run_key | run:fixture |", report)
        self.assertIn("| work_program_run_member_count | 3 |", report)
        self.assertIn("| run_member_table:work_program_quality_gates | 1 |", report)
        self.assertIn("| run_member_table:work_program_evidence_needs | 1 |", report)
        self.assertIn("| run_member_table:work_program_milestones | 1 |", report)
        self.assertIn("| run_member_missing_target:work_program_milestones | 0 |", report)
        self.assertIn("| run_member_latest_timestamp_delta:work_program_milestones | 0 |", report)
        self.assertIn("| run_member_unknown_table_count | 0 |", report)

    def test_dependency_edge_endpoint_resolution_flags_missing_typed_endpoints(self) -> None:
        with sqlite3.connect(":memory:") as conn:
            conn.executescript(
                """
                create table work_dependency_edges (
                  source_instance text,
                  edge_kind text,
                  from_kind text,
                  to_kind text,
                  workstream_id integer,
                  work_blocker_id integer,
                  work_action_id integer,
                  ticket_id integer,
                  pull_request_id integer
                );
                create table tickets (id integer);
                create table pull_requests (id integer);
                create table work_blockers (id integer);
                create table work_actions (id integer);
                create table workstreams (id integer);
                insert into pull_requests values (10);
                insert into tickets values (20);
                insert into workstreams values (30);
                insert into work_blockers values (40);
                insert into work_actions values (50);
                """
            )
            conn.executemany(
                "insert into work_dependency_edges values (?, ?, ?, ?, ?, ?, ?, ?, ?)",
                [
                    ("fixture-source", "ticket_pr", "ticket", "pull_request", 30, None, None, None, 10),
                    ("fixture-source", "ticket_pr", "pull_request", "ticket", 30, None, None, 20, 10),
                    ("fixture-source", "blocked_by", "pull_request", "blocker", 30, None, None, None, 10),
                    ("fixture-source", "blocked_by", "blocker", "pull_request", 30, 40, None, None, 10),
                    ("fixture-source", "needs_action", "blocker", "action", 30, 40, None, None, None),
                    ("fixture-source", "needs_action", "action", "blocker", 30, 40, 50, None, None),
                    ("fixture-source", "workstream_cluster", "component", "ticket", None, None, None, 20, None),
                    ("fixture-source", "workstream_cluster", "workstream", "ticket", 30, None, None, 20, None),
                ],
            )

            rows = {
                metric: value
                for metric, value, _ in validation.dependency_edge_endpoint_resolution_rows(conn, "fixture-source")
            }

        self.assertEqual(rows["invalid_endpoint_shape:work_dependency_edges.ticket_pr"], "1")
        self.assertEqual(rows["invalid_endpoint_shape:work_dependency_edges.blocked_by"], "1")
        self.assertEqual(rows["invalid_endpoint_shape:work_dependency_edges.needs_action"], "1")
        self.assertEqual(rows["invalid_endpoint_shape:work_dependency_edges.workstream_cluster"], "1")
        self.assertEqual(rows["unresolved_ticket_endpoint:work_dependency_edges.ticket_pr"], "1")
        self.assertEqual(rows["unresolved_blocker_endpoint:work_dependency_edges.blocked_by"], "1")
        self.assertEqual(rows["unresolved_action_endpoint:work_dependency_edges.needs_action"], "1")
        self.assertEqual(rows["unresolved_workstream_context:work_dependency_edges.workstream_cluster"], "1")

    def test_dependency_edge_endpoint_resolution_tolerates_partial_schema(self) -> None:
        with sqlite3.connect(":memory:") as conn:
            conn.executescript(
                """
                create table work_dependency_edges (
                  edge_kind text,
                  from_kind text,
                  to_kind text,
                  workstream_id integer,
                  work_blocker_id integer,
                  work_action_id integer,
                  ticket_id integer,
                  pull_request_id integer
                );
                insert into work_dependency_edges values (
                  'ticket_pr', 'ticket', 'pull_request', null, null, null, null, 10
                );
                """
            )

            rows = {
                metric: value
                for metric, value, _ in validation.dependency_edge_endpoint_resolution_rows(conn, "fixture-source")
            }

        self.assertEqual(rows["invalid_endpoint_shape:work_dependency_edges.ticket_pr"], "0")
        self.assertEqual(rows["unresolved_ticket_endpoint:work_dependency_edges.ticket_pr"], "1")

    def test_dependency_edge_authority_flags_malformed_rows(self) -> None:
        with sqlite3.connect(":memory:") as conn:
            conn.executescript(
                """
                create table work_dependency_edges (
                  id integer primary key,
                  source_instance text,
                  edge_kind text,
                  relationship_authority text,
                  canonical_relationship_kind text,
                  ticket_id integer,
                  pull_request_id integer
                );
                create table ticket_pull_requests (
                  ticket_id integer,
                  pull_request_id integer
                );
                insert into ticket_pull_requests values (20, 10);
                insert into work_dependency_edges values
                  (1, 'fixture-source', 'ticket_pr', 'operating_projection', null, 20, 10),
                  (2, 'fixture-source', 'ticket_pr', 'canonical_mirror', null, 20, 10),
                  (3, 'fixture-source', 'ticket_pr', 'canonical_mirror', 'ticket_pull_request', 21, 11),
                  (4, 'fixture-source', 'blocked_by', 'operating_projection', 'ticket_pull_request', null, null),
                  (5, 'fixture-source', 'needs_action', 'canonical_mirror', 'work_action', null, null),
                  (6, 'fixture-source', 'related_work', 'nonsense', null, null, null),
                  (7, 'other-source', 'ticket_pr', 'canonical_mirror', 'ticket_pull_request', 20, 10);
                """
            )

            rows = {
                metric: value
                for metric, value, _ in validation.dependency_edge_authority_rows(conn, "fixture-source")
            }

        self.assertEqual(rows["invalid_relationship_authority:work_dependency_edges"], "1")
        self.assertEqual(rows["canonical_mirror_missing_kind:work_dependency_edges"], "1")
        self.assertEqual(rows["invalid_canonical_relationship_kind:work_dependency_edges"], "1")
        self.assertEqual(rows["projection_with_canonical_kind:work_dependency_edges"], "1")
        self.assertEqual(rows["ticket_pr_not_canonical_mirror:work_dependency_edges"], "2")
        self.assertEqual(rows["canonical_mirror_missing_typed_row:work_dependency_edges.ticket_pr"], "1")
        self.assertEqual(rows["non_ticket_pr_canonical_mirror:work_dependency_edges"], "1")
        self.assertEqual(rows["work_dependency_edge_authority:canonical_mirror"], "3")
        self.assertEqual(rows["work_dependency_edge_authority:nonsense"], "1")
        self.assertEqual(rows["work_dependency_edge_authority:operating_projection"], "2")

    def test_dependency_endpoint_integrity_flags_malformed_endpoint_rows(self) -> None:
        with sqlite3.connect(":memory:") as conn:
            conn.executescript(
                """
                create table work_dependency_edges (
                  id integer primary key,
                  source_instance text
                );
                create table work_dependency_endpoints (
                  id integer primary key,
                  source_instance text,
                  work_dependency_edge_id integer,
                  endpoint_role text,
                  node_kind text,
                  node_key text,
                  resolution_state text,
                  workstream_id integer,
                  work_blocker_id integer,
                  work_action_id integer,
                  ticket_id integer,
                  pull_request_id integer
                );
                insert into work_dependency_edges values (1, 'fixture-source');
                insert into work_dependency_endpoints values
                  (1, 'fixture-source', 1, 'middle', 'ticket', 'FLINK-1', 'resolved', null, null, null, 20, null),
                  (2, 'fixture-source', 1, 'from', 'nonsense', 'bad', 'resolved', null, null, null, null, null),
                  (3, 'fixture-source', 1, 'from', 'ticket', 'FLINK-2', 'unknown', null, null, null, null, null),
                  (4, 'fixture-source', 1, 'from', 'ticket', 'FLINK-3', 'key_only', null, null, null, null, null),
                  (5, 'fixture-source', 1, 'from', 'component', 'component:x', 'missing', null, null, null, null, null),
                  (6, 'fixture-source', 1, 'to', 'ticket', 'FLINK-4', 'resolved', null, null, null, 20, 10),
                  (7, 'fixture-source', 99, 'to', 'pull_request', 'repo/example#1', 'resolved', null, null, null, null, 10);
                """
            )

            rows = {
                metric: value
                for metric, value, _ in validation.dependency_endpoint_integrity_rows(conn, "fixture-source")
            }

        self.assertEqual(rows["work_dependency_endpoints"], "7")
        self.assertEqual(rows["work_dependency_endpoint_expected_count"], "2")
        self.assertEqual(rows["work_dependency_endpoint_count_delta"], "5")
        self.assertEqual(rows["orphaned_work_dependency_endpoints"], "1")
        self.assertEqual(rows["invalid_endpoint_role:work_dependency_endpoints"], "1")
        self.assertEqual(rows["invalid_node_kind:work_dependency_endpoints"], "1")
        self.assertEqual(rows["invalid_resolution_state:work_dependency_endpoints"], "1")
        self.assertEqual(rows["invalid_key_only_endpoint:work_dependency_endpoints"], "2")
        self.assertEqual(rows["invalid_resolved_pointer_shape:work_dependency_endpoints"], "2")
        self.assertEqual(rows["missing_typed_target:work_dependency_endpoints"], "0")

    def test_no_run_report_is_explicit(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            sqlite3.connect(ontology_db).close()
            sqlite3.connect(analytics_db).close()

            report = validation.build_validation_report(ontology_db, analytics_db)

        self.assertIn("Verdict: `no_automation_readiness_run`", report)


def seed_ontology_db(
    db_path: pathlib.Path,
    *,
    readiness_generated_at: str,
    row_generated_at: str | None = None,
) -> None:
    generated_at = row_generated_at or readiness_generated_at
    with sqlite3.connect(db_path) as conn:
        conn.executescript(
            """
            create table work_program_automation_readinesses (
              id integer primary key,
              source_instance text,
              workstream_key text,
              generated_at text,
              readiness_state text,
              readiness_score real,
              autonomous_action_ready integer,
              human_review_required integer,
              evidence_need_count integer,
              blocking_gate_count integer,
              tpm_function_count integer,
              external_id text,
              rank_score real
            );
            create table work_program_quality_gates (
              source_instance text,
              workstream_key text,
              generated_at text,
              gate_key text,
              gate_state text,
              blocking integer,
              detail text,
              recommended_action text
            );
            create table work_program_evidence_needs (
              source_instance text,
              workstream_key text,
              generated_at text,
              gate_key text,
              evidence_kind text,
              priority text,
              execution_state text,
              target_kind text,
              target_key text,
              recommended_action text
            );
            create table work_program_tpm_function_readinesses (
              source_instance text,
              workstream_key text,
              generated_at text,
              function_key text,
              readiness_state text,
              automation_state text,
              human_required integer,
              blocking_gate_keys text,
              detail text,
              recommended_action text
            );
            create table work_program_adversarial_checks (
              source_instance text,
              workstream_key text,
              generated_at text,
              check_kind text,
              check_state text,
              severity text,
              title text,
              detail text,
              recommended_action text
            );
            create table work_owner_load_snapshots (
              source_instance text,
              workstream_key text,
              generated_at text,
              load_status text,
              owner_key text,
              action_count integer,
              critical_or_high_count integer,
              coverage_limited_count integer,
              needs_human_review_count integer
            );
            create table work_program_items (
              source_instance text,
              workstream_key text,
              source_coverage_state text,
              subject_kind text,
              subject_key text,
              pull_request_id integer,
              ticket_id integer,
              latest_evidence_id integer
            );
            create table work_item_forecasts (
              id integer primary key,
              source_instance text,
              risk_band text,
              ready_for_eta integer,
              subject_kind text,
              subject_key text,
              pull_request_id integer,
              ticket_id integer,
              work_action_id integer,
              latest_evidence_id integer
            );
            create table work_forecast_evaluations (
              id integer primary key,
              source_instance text,
              external_kind text,
              readiness_state text,
              ready_for_eta integer,
              latest_evidence_id integer
            );
            create table work_decision_target_evaluations (
              id integer primary key,
              source_instance text,
              evaluated_at text,
              target_kind text,
              evaluation_kind text,
              model_name text,
              coverage_stratum text,
              precision_at_10pct real,
              lift_at_10pct real,
              ready_for_product_action integer,
              product_action_gate_state text,
              note text
            );
            create table source_sync_issues (
              source_instance text,
              source_scope_id integer
            );
            create table source_scopes (
              id integer,
              scope_key text
            );
            create table work_blockers (
              id integer primary key,
              source_instance text,
              blocker_state text,
              subject_kind text,
              subject_key text,
              pull_request_id integer,
              ticket_id integer,
              latest_evidence_id integer
            );
            create table work_blocker_impacts (
              source_instance text
            );
            create table work_actions (
              id integer primary key,
              source_instance text,
              decision_state text,
              subject_kind text,
              subject_key text,
              pull_request_id integer,
              ticket_id integer,
              latest_evidence_id integer
            );
            create table work_action_observations (
              source_instance text,
              observed_at text,
              observation_kind text,
              source_coverage_state text,
              auth_state text,
              supports_action integer
            );
            create table work_insights (
              id integer primary key,
              source_instance text,
              subject_kind text,
              subject_key text,
              pull_request_id integer,
              ticket_id integer,
              latest_evidence_id integer
            );
            create table work_action_source_insights (
              work_action_id integer,
              work_insight_id integer
            );
            create table work_dependency_edges (
              id integer primary key,
              source_instance text,
              edge_kind text,
              relationship_authority text,
              canonical_relationship_kind text,
              from_kind text,
              to_kind text,
              latest_evidence_id integer,
              workstream_id integer,
              work_blocker_id integer,
              work_action_id integer,
              ticket_id integer,
              pull_request_id integer
            );
            create table work_dependency_endpoints (
              id integer primary key,
              source_instance text,
              work_dependency_edge_id integer,
              endpoint_role text,
              node_kind text,
              node_key text,
              resolution_state text
            );
            create table tickets (id integer);
            create table pull_requests (id integer);
            create table ticket_pull_requests (id integer);
            create table evidences (
              id integer,
              claim_kind text,
              external_kind text
            );
            """
        )
        conn.executemany(
            "insert into evidences values (?, ?, ?)",
            [
                (1, "relationship", "jira_remote_links"),
                (2, "object_state", "tpm_generated_evidence"),
            ],
        )
        conn.execute("insert into tickets values (20)")
        conn.execute("insert into pull_requests values (10)")
        conn.execute(
            """
            insert into work_program_automation_readinesses values (
              1, 'fixture-source', 'workstream:fixture', ?, 'blocked', 5.0,
              0, 1, 3, 2, 2, 'workstream:fixture|run|automation_readiness', 10.0
            )
            """,
            [readiness_generated_at],
        )
        conn.executemany(
            "insert into work_program_quality_gates values (?, ?, ?, ?, ?, ?, ?, ?)",
            [
                ("fixture-source", "workstream:fixture", generated_at, "source_coverage", "gated", 1, "2 limited items.", "Repair source coverage."),
                ("fixture-source", "workstream:fixture", generated_at, "forecast_readiness", "gated", 1, "ETA forecast remains gated.", "Use for risk triage only."),
                ("fixture-source", "workstream:fixture", generated_at, "blocker_clearance", "passed", 0, "No active blockers.", ""),
            ],
        )
        conn.executemany(
            "insert into work_program_evidence_needs values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                ("fixture-source", "workstream:fixture", generated_at, "source_coverage", "generated_evidence", "medium", "qa_action_open", "workstream", "workstream:fixture", "Audit generated evidence."),
                ("fixture-source", "workstream:fixture", generated_at, "forecast_readiness", "forecast_backtest", "high", "actions_open", "workstream", "workstream:fixture", "Improve ETA validation."),
            ],
        )
        conn.executemany(
            "insert into work_program_tpm_function_readinesses values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                ("fixture-source", "workstream:fixture", generated_at, "operating_brief", "automatable", "can_publish_operating_brief", 0, "", "Brief is ready.", "Publish."),
                ("fixture-source", "workstream:fixture", generated_at, "source_coverage", "blocked", "coverage_repair_required", 1, "source_coverage", "Coverage blocked.", "Repair source."),
            ],
        )
        conn.executemany(
            "insert into work_program_adversarial_checks values (?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                ("fixture-source", "workstream:fixture", generated_at, "source_absence_claim", "fail", "critical", "No absence claims", "403/429 prevents absence claims.", "Repair source."),
                ("fixture-source", "workstream:fixture", generated_at, "brief_shape", "pass", "info", "Brief OK", "Operating brief is present.", ""),
            ],
        )
        conn.executemany(
            "insert into work_owner_load_snapshots values (?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                ("fixture-source", "workstream:fixture", generated_at, "overloaded", "github:owner", 7, 3, 2, 5),
                ("fixture-source", "workstream:fixture", generated_at, "watch", "github:reviewer", 2, 0, 0, 1),
            ],
        )
        conn.executemany(
            """
            insert into work_program_items (
              source_instance, workstream_key, source_coverage_state,
              subject_kind, subject_key, pull_request_id, ticket_id, latest_evidence_id
            ) values (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            [
                ("fixture-source", "workstream:fixture", "observed:authenticated_api_current_observation", "pull_request", "repo/example#1", 10, None, 1),
                ("fixture-source", "workstream:fixture", "generated:forecast_backtest", "pull_request", "repo/example#2", 11, None, 1),
                ("fixture-source", "workstream:fixture", "anonymous_success:public_api_current_observation", "ticket", "FLINK-1", None, 20, 1),
            ],
        )
        conn.executemany(
            """
            insert into work_item_forecasts (
              id, source_instance, risk_band, ready_for_eta, subject_kind, subject_key,
              pull_request_id, ticket_id, work_action_id, latest_evidence_id
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            [
                (201, "fixture-source", "critical", 0, "pull_request", "repo/example#1", 10, None, None, 1),
                (202, "fixture-source", "low", 0, "pull_request", "repo/example#2", 11, None, None, 1),
                (203, "fixture-source", "high", 0, "ticket", "FLINK-1", None, 20, 2, 1),
            ],
        )
        conn.executemany(
            "insert into work_forecast_evaluations values (?, ?, ?, ?, ?, ?)",
            [
                (301, "fixture-source", "tpm_forecast_evaluation", "gated", 0, 1),
                (302, "fixture-source", "tpm_forecast_risk_backtest", "gated", 0, 1),
            ],
        )
        conn.executemany(
            "insert into work_decision_target_evaluations values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                (401, "fixture-source", generated_at, "abandonment_risk", "source_event_as_of_coverage_stratified_summary", "coverage_guardrail", "not_testable_single_stratum", None, None, 0, "validation_gated", "coverage confounding cannot be tested"),
                (402, "fixture-source", generated_at, "abandonment_risk", "source_event_as_of_grouped_kfold", "random_forest_classifier", "coverage=observed", 0.5, 0.3, 0, "gated", "validation only"),
            ],
        )
        conn.execute("insert into source_scopes values (1, 'fixture-source')")
        conn.executemany(
            "insert into source_sync_issues values (?, ?)",
            [("github.com/repo/example", 1), ("github.com/repo/example", 1)],
        )
        conn.execute("insert into work_blockers values (1, 'fixture-source', 'open', 'pull_request', 'repo/example#1', 10, null, 1)")
        conn.execute("insert into work_blocker_impacts values ('fixture-source')")
        conn.executemany(
            """
            insert into work_actions (
              id, source_instance, decision_state, subject_kind, subject_key,
              pull_request_id, ticket_id, latest_evidence_id
            ) values (?, ?, ?, ?, ?, ?, ?, ?)
            """,
            [
                (1, "fixture-source", "validation_lead", "pull_request", "repo/example#1", 10, None, 1),
                (2, "fixture-source", "validation_lead", "ticket", "FLINK-1", None, 20, 1),
            ],
        )
        conn.executemany(
            """
            insert into work_action_observations (
              source_instance, observed_at, observation_kind, source_coverage_state,
              auth_state, supports_action
            ) values (?, ?, ?, ?, ?, ?)
            """,
            [
                ("fixture-source", generated_at, "source_state", "observed:authenticated_api_current_observation", "github_token", 0),
                ("fixture-source", generated_at, "source_state", "anonymous_success:public_api_current_observation", "anonymous", 0),
                ("fixture-source", "2026-06-21T00:00:00Z", "source_state", "generated:old", "", 0),
            ],
        )
        conn.executemany(
            "insert into work_insights values (?, ?, ?, ?, ?, ?, ?)",
            [
                (101, "fixture-source", "pull_request", "repo/example#1", 10, None, 1),
                (102, "fixture-source", "ticket", "FLINK-1", None, 20, 1),
            ],
        )
        conn.execute("insert into work_action_source_insights values (1, 101)")
        conn.executemany(
            """
            insert into work_dependency_edges (
              id, source_instance, edge_kind, relationship_authority, canonical_relationship_kind,
              from_kind, to_kind, latest_evidence_id, workstream_id, work_blocker_id,
              work_action_id, ticket_id, pull_request_id
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            [
                (1, "fixture-source", "ticket_pr", "canonical_mirror", "ticket_pull_request", "ticket", "pull_request", 1, 1, None, 1, 20, 10),
                (2, "fixture-source", "workstream_cluster", "operating_projection", None, "component", "ticket", None, 1, None, 2, 20, None),
            ],
        )
        conn.executemany(
            """
            insert into work_dependency_endpoints (
              id, source_instance, work_dependency_edge_id, endpoint_role,
              node_kind, node_key, resolution_state
            ) values (?, ?, ?, ?, ?, ?, ?)
            """,
            [
                (1, "fixture-source", 1, "from", "ticket", "FLINK-1", "resolved"),
                (2, "fixture-source", 1, "to", "pull_request", "repo/example#1", "resolved"),
                (3, "fixture-source", 2, "from", "component", "component:fixture", "key_only"),
                (4, "fixture-source", 2, "to", "ticket", "FLINK-1", "resolved"),
            ],
        )


def seed_analytics_db(db_path: pathlib.Path) -> None:
    with sqlite3.connect(db_path) as conn:
        conn.executescript(
            """
            create table tpm_forecast_summary (
              metric text,
              value text,
              note text
            );
            create table tpm_forecast_backtest (
              evaluation text,
              model text,
              mae_days real,
              ready_for_eta text,
              note text
            );
            create table tpm_forecast_reliability (
              forecast_product text,
              readiness_state text,
              product_safe text,
              safe_use text,
              best_model text,
              primary_metric text,
              metric_value text,
              next_evidence text,
              guardrail text
            );
            create table tpm_decision_target_backtest (
              target_kind text,
              evaluation text,
              model text,
              coverage_stratum text,
              precision_at_10pct real,
              lift_at_10pct real,
              ready_for_product_action text,
              note text
            );
            create table tpm_blocker_candidates (candidate_kind text);
            create table tpm_action_items (decision_state text);
            create table tpm_work_action_observations (observation_kind text);
            create table tpm_followup_observations (subject_key text);
            create table tpm_pr_check_observations (subject_key text);
            create table tpm_followup_summary (
              metric text,
              value text,
              note text
            );
            create table tpm_check_summary (
              metric text,
              value text,
              note text
            );
            create table tpm_check_signal_readiness (
              readiness_key text,
              ready integer,
              readiness_state text,
              support_level text,
              blocking_reason text
            );
            create table tpm_transition_signal_readiness (
              readiness_key text,
              ready integer,
              readiness_state text,
              support_level text,
              blocking_reason text
            );
            create table tpm_developer_correlation (
              correlation_state text,
              extra_jira_ticket_count integer
            );
            create table tpm_developer_correlation_validation (
              metric text,
              value text,
              sample_count integer,
              method text,
              interpretation text,
              guardrail text
            );
            create table tpm_pr_source_coverage (
              source_current_coverage_state text,
              source_current_detail_state text
            );
            create table tpm_evaluation_readiness (
              metric text,
              value text,
              note text
            );
            create table tpm_measurement_label_summary (
              metric text,
              value text,
              note text
            );
            """
        )
        conn.executemany(
            "insert into tpm_forecast_summary values (?, ?, ?)",
            [
                ("forecast_method", "heuristic_percentile_rf_rejected", "risk triage method"),
                ("eta_forecast_ready", "false", "model did not beat baseline"),
                ("eta_readiness_state", "blocked", "ETA commitments are blocked"),
                ("eta_model_backtest_ready", "false", "model quality alone is not ready"),
                ("eta_primary_blocker", "kfold_model_does_not_beat_baseline", "RF did not beat the median baseline"),
                ("eta_blocker_count", "3", "failed ETA readiness checks"),
                ("eta_kfold_best_candidate_improvement_pct", "7.04", "best candidate K-fold lift"),
                ("eta_kfold_random_forest_improvement_pct", "-35.22", "K-fold RF improvement"),
                ("eta_chronological_random_forest_improvement_pct", "7.70", "chronological RF improvement"),
                ("eta_temporal_snapshot_state", "as_of_feature_snapshot_series_missing", "no as-of feature snapshot series"),
                ("eta_next_evidence_needed", "collect_repeated_as_of_pr_snapshots_and_closed_outcomes", "path to ETA readiness"),
                ("forecast_feature_leakage_guard", "passed", "future fields excluded"),
                ("backtest_median_mae_days", "6.53", "median baseline"),
                ("backtest_random_forest_mae_days", "8.83", "random forest"),
                (
                    "risk_triage_coverage_stratified_state",
                    "not_testable_single_stratum",
                    "coverage confounding cannot be tested from this sample",
                ),
                ("risk_triage_coverage_stratum_count", "1", "one source coverage/provenance stratum"),
            ],
        )
        conn.execute(
            "insert into tpm_forecast_backtest values ('cycle_time', 'median_cycle_baseline', 6.53, 'false', 'baseline wins')"
        )
        conn.executemany(
            "insert into tpm_forecast_reliability values (?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                (
                    "point_eta",
                    "gated",
                    "false",
                    "diagnostic_only",
                    "gradient_boosting_absolute_error",
                    "eta_kfold_best_candidate_improvement_pct",
                    "7.04",
                    "improve_model_features_and_validate_against_as_of_snapshots",
                    "Point ETA is blocked by kfold_model_does_not_beat_baseline; use forecast outputs for risk triage only.",
                ),
                (
                    "risk_triage",
                    "ready_with_coverage_guardrail",
                    "true",
                    "attention_ordering",
                    "static_risk_triage_score",
                    "risk_triage_lift_at_10pct",
                    "0.3446",
                    "add_more_coverage_strata_for_confounding_checks",
                    "Risk triage is product-safe only for attention ordering; it is not an ETA, causality, blocker, or autonomous action claim.",
                ),
            ],
        )
        conn.execute(
            "insert into tpm_decision_target_backtest values ('abandonment_risk', 'source_event_as_of_grouped_kfold', 'random_forest_classifier', 'coverage=observed', 0.5, 0.3, 'false', 'validation only')"
        )
        conn.execute(
            "insert into tpm_decision_target_backtest values ('abandonment_risk', 'source_event_as_of_coverage_stratified_summary', 'coverage_guardrail', 'not_testable_single_stratum', null, null, 'false', 'coverage confounding cannot be tested')"
        )
        conn.execute("insert into tpm_blocker_candidates values ('blocker_keyword')")
        conn.execute("insert into tpm_action_items values ('validation_lead')")
        conn.execute("insert into tpm_work_action_observations values ('source_state')")
        conn.execute("insert into tpm_followup_observations values ('repo/example#1')")
        conn.execute("insert into tpm_pr_check_observations values ('repo/example#1')")
        conn.executemany(
            "insert into tpm_followup_summary values (?, ?, ?)",
            [
                ("fetch_success_count", "3", "fixture follow-up HTTP 200 reads"),
                ("fetch_error_count", "0", "fixture follow-up failures"),
            ],
        )
        conn.executemany(
            "insert into tpm_check_summary values (?, ?, ?)",
            [
                ("check_runs_fetch_success_count", "2", "fixture check-run reads"),
                ("source_coverage_failed_count", "0", "fixture check coverage failures"),
            ],
        )
        conn.executemany(
            "insert into tpm_check_signal_readiness values (?, ?, ?, ?, ?)",
            [
                (
                    "ci_followup_validation",
                    1,
                    "ready_with_current_open_signal",
                    "validation_lead",
                    "HTTP 200 check/status observations include one failing open PR.",
                ),
                (
                    "required_check_product_action",
                    0,
                    "no_required_contexts_configured",
                    "product_action_evidence",
                    "Branch-protection evidence reports no required status-check contexts; rulesets are not proven absent.",
                ),
                (
                    "ci_eta_feature",
                    0,
                    "single_live_observation",
                    "eta_feature",
                    "One live observation is not a point-in-time feature series.",
                ),
            ],
        )
        conn.executemany(
            "insert into tpm_transition_signal_readiness values (?, ?, ?, ?, ?)",
            [
                (
                    "terminal_closeout_review",
                    0,
                    "terminal_transition_superseded",
                    "closeout_review",
                    "Terminal transition candidates were followed by later open-state evidence.",
                ),
                (
                    "source_resolved_closeout",
                    0,
                    "blocked_by_later_nonterminal_state",
                    "source_resolved_evidence",
                    "Later source state contradicts terminal transition evidence.",
                ),
                (
                    "transition_eta_feature",
                    0,
                    "candidate_needs_transition_label_validation",
                    "eta_feature",
                    "Transition candidates need changelog validation before ETA use.",
                ),
            ],
        )
        conn.execute("insert into tpm_developer_correlation values ('direct_identity_same_window', 4)")
        conn.executemany(
            "insert into tpm_developer_correlation_validation values (?, ?, ?, ?, ?, ?)",
            [
                (
                    "direct_identity_sample_count",
                    "56",
                    56,
                    "direct GitHub/Jira identity rows with PR authorship or same-window Jira activity",
                    "Population available for aggregate correlation.",
                    "Same-window developer correlation is a workload/attention lead only.",
                ),
                (
                    "spearman_open_extra_jira_vs_high_risk_open_pr",
                    "0.09",
                    56,
                    "rank correlation over direct identity rows",
                    "Weak positive workload co-occurrence; not causality.",
                    "Same-window developer correlation is a workload/attention lead only.",
                ),
            ],
        )
        conn.executemany(
            "insert into tpm_pr_source_coverage values (?, ?)",
            [
                ("detail_failed", "failed"),
                ("detail_failed", "failed"),
                ("observed", "observed"),
            ],
        )
        conn.executemany(
            "insert into tpm_evaluation_readiness values (?, ?, ?)",
            [
                ("evaluation_label_row_count", "0", "fixture counted labels"),
                ("truth_label_coverage", "1/3", "fixture truth coverage"),
                ("ready_to_measure_precision", "false", "fixture precision gate"),
                ("measurement_labels_blocker_candidate", "0", "fixture blocker labels"),
                ("ready_to_measure_blocker_candidate", "false", "fixture blocker gate"),
            ],
        )
        conn.executemany(
            "insert into tpm_measurement_label_summary values (?, ?, ?)",
            [
                ("measurement_label_count", "4", "fixture available label pack"),
                ("measurement_queue_count", "2", "fixture queue"),
                ("non_measurement_label_count", "1", "fixture QA-only labels"),
            ],
        )


if __name__ == "__main__":
    unittest.main()
