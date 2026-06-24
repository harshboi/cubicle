#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import pathlib
import sqlite3
import unittest
import warnings

import pandas as pd


MODULE_PATH = pathlib.Path(__file__).with_name("flink_tpm_action_brief.py")
SPEC = importlib.util.spec_from_file_location("flink_tpm_action_brief", MODULE_PATH)
brief = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(brief)


class CheckSignalWordingTest(unittest.TestCase):
    def test_ci_owner_focus_stays_validation_oriented(self) -> None:
        self.assertEqual(
            brief.owner_focus_for("ci_check_followup"),
            "Validate failing-check evidence with the owner before treating it as product work.",
        )

    def test_cadence_focus_labels_failing_checks_as_ci_leads(self) -> None:
        focus = brief.cadence_focus(
            critical_or_high_count=0,
            validation_lead_count=0,
            critical_or_high_validation_lead_count=0,
            failing_check_pr_count=1,
            source_repair_count=0,
            coverage_limited_count=0,
            anonymous_observation_count=0,
            eta_ready="true",
        )

        self.assertEqual(focus, "validate 1 PR with failing checks as CI lead")


class WorkProgramRunMaterializationTest(unittest.TestCase):
    def test_work_program_run_materializes_run_boundary_and_members(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_program_run_tables(conn)
        generated_at = "2026-06-22T07:44:28+00:00"

        seed_run_member(
            conn,
            "work_program_automation_readinesses",
            "readiness",
            generated_at,
            readiness_state="blocked",
            readiness_score=5.0,
            blocking_gate_count=1,
            evidence_need_count=1,
            tpm_function_count=2,
            rank_score=5.0,
        )
        seed_run_member(conn, "work_program_quality_gates", "gate:source", generated_at, rank_score=100.0)
        seed_run_member(conn, "work_program_quality_gates", "gate:forecast", generated_at, rank_score=90.0)
        seed_run_member(conn, "work_program_evidence_needs", "need:source", generated_at, rank_score=80.0)
        seed_run_member(conn, "work_owner_load_snapshots", "owner:one", generated_at, rank_score=70.0)

        brief.persist_work_program_run_to_ontology(conn, "fixture-source", generated_at)
        brief.persist_work_program_run_to_ontology(conn, "fixture-source", generated_at)

        run = conn.execute(
            """
            select readiness_state, readiness_score, blocking_gate_count,
                   evidence_need_count, tpm_function_count, quality_gate_count,
                   owner_load_snapshot_count, member_count
              from work_program_runs
            """
        ).fetchone()
        self.assertEqual(run, ("blocked", 5.0, 1, 1, 2, 2, 1, 5))
        self.assertEqual(conn.execute("select count(*) from work_program_run_members").fetchone()[0], 5)
        self.assertEqual(
            conn.execute(
                """
                select count(*)
                  from work_program_run_members
                 where work_program_run_id is not null
                """
            ).fetchone()[0],
            5,
        )
        members = conn.execute(
            """
            select member_table, count(*)
              from work_program_run_members
             group by member_table
             order by member_table
            """
        ).fetchall()
        self.assertEqual(
            members,
            [
                ("work_owner_load_snapshots", 1),
                ("work_program_automation_readinesses", 1),
                ("work_program_evidence_needs", 1),
                ("work_program_quality_gates", 2),
            ],
        )


class WorkResponsibilityMaterializationTest(unittest.TestCase):
    def test_work_responsibilities_materialize_typed_subjects_and_parties(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_responsibility_tables(conn)
        generated_at = "2026-06-23T09:30:00+00:00"

        conn.execute(
            """
            insert into persons (id, key, display_name, github_login)
            values (1, 'person:github:owner-one', 'Owner One', 'owner-one')
            """
        )
        conn.execute(
            """
            insert into person_identities (
              person_id, handle, external_id, source_system, external_kind, identity_status
            ) values (1, 'owner-one', 'owner-one', 'github', 'github_user', 'active')
            """
        )
        conn.execute(
            """
            insert into pull_requests (id, key, repository, number, title, source_url)
            values (
              10, 'apache/flink-kubernetes-operator#42', 'apache/flink-kubernetes-operator', 42,
              'Fix autoscaler reconciliation', 'https://github.com/apache/flink-kubernetes-operator/pull/42'
            )
            """
        )
        conn.execute(
            """
            insert into work_actions (
              id, key, owner_key, owner_source, action_state, decision_state,
              created_from_run_key, source_system, source_instance, external_kind,
              external_id, source_url, evidence_count, freshness_state, visibility,
              confidence, event_count, first_seen_at, last_activity_at, rank_score, opened_at
            ) values (
              20, 'work-action:owner-followup', 'github:owner-one', 'generated.action_owner',
              'open', 'candidate', 'run:fixture', 'cubicle_analytics', 'fixture-source',
              'tpm_work_action', 'action-owner-followup', 'https://example.test/action',
              0, 'fresh', 'public', 0.9, 2, ?, ?, 80.0, ?
            )
            """,
            (generated_at, generated_at, generated_at),
        )
        conn.execute(
            """
            insert into work_program_items (
              id, key, workstream_key, subject_kind, subject_key, pull_request_id,
              owner_key, owner_source, author_dri, requested_reviewer_keys, reviewer_or_approver,
              source_system, source_instance, external_kind, external_id, source_url,
              evidence_count, freshness_state, visibility, confidence, event_count,
              first_seen_at, last_activity_at, rank_score, register_updated_at
            ) values (
              30, 'work-program-item:pr-42', 'workstream:flink', 'pull_request',
              'apache/flink-kubernetes-operator#42', 10, 'unassigned', 'unassigned',
              'owner-one', 'reviewer-one', '', 'cubicle_analytics', 'fixture-source',
              'tpm_program_item', 'program-item-pr-42',
              'https://github.com/apache/flink-kubernetes-operator/pull/42',
              0, 'fresh', 'public', 0.95, 3, ?, ?, 100.0, ?
            )
            """,
            (generated_at, generated_at, generated_at),
        )
        conn.execute(
            """
            insert into work_program_evidence_needs (
              id, key, workstream_key, owner_key, action_state, execution_state,
              source_system, source_instance, external_kind, external_id, source_url,
              evidence_count, freshness_state, visibility, confidence, event_count,
              first_seen_at, last_activity_at, rank_score, generated_at
            ) values (
              40, 'work-program-evidence-need:ci-proof', 'workstream:flink', '',
              'open', 'pending', 'cubicle_analytics', 'fixture-source',
              'tpm_work_program_evidence_need', 'evidence-need-ci-proof',
              'https://example.test/evidence-need', 0, 'fresh', 'public',
              0.7, 1, ?, ?, 70.0, ?
            )
            """,
            (generated_at, generated_at, generated_at),
        )

        brief.persist_work_responsibilities_to_ontology(conn, "fixture-source", generated_at)
        brief.persist_work_responsibilities_to_ontology(conn, "fixture-source", generated_at)

        pr_author = conn.execute(
            """
            select subject_kind, subject_key, pull_request_id, work_program_item_id,
                   party_kind, party_key, person_id, responsibility_kind,
                   basis_kind, responsibility_state, evidence_count
              from work_responsibilities
             where subject_kind = 'pull_request'
               and responsibility_kind = 'author'
            """
        ).fetchone()
        self.assertEqual(
            pr_author,
            (
                "pull_request",
                "apache/flink-kubernetes-operator#42",
                10,
                30,
                "person",
                "github:owner-one",
                1,
                "author",
                "source_native",
                "active",
                1,
            ),
        )
        self.assertIsNotNone(
            conn.execute(
                """
                select latest_evidence_id
                  from work_responsibilities
                 where subject_kind = 'pull_request'
                   and responsibility_kind = 'author'
                """
            ).fetchone()[0]
        )

        action_owner = conn.execute(
            """
            select subject_kind, work_action_id, party_kind, party_key, person_id,
                   responsibility_kind, basis_kind, responsibility_state
              from work_responsibilities
             where subject_kind = 'work_action'
            """
        ).fetchone()
        self.assertEqual(
            action_owner,
            (
                "work_action",
                20,
                "person",
                "github:owner-one",
                1,
                "accountable",
                "generated_candidate",
                "candidate",
            ),
        )

        unassigned_rows = conn.execute(
            """
            select subject_kind, party_kind, party_key, person_id, responsibility_kind
              from work_responsibilities
             where party_kind = 'unassigned'
             order by subject_kind, responsibility_kind
            """
        ).fetchall()
        self.assertEqual(
            unassigned_rows,
            [
                ("pull_request", "unassigned", "unassigned", None, "accountable"),
                ("work_program_evidence_need", "unassigned", "unassigned", None, "validation_owner"),
            ],
        )

        unresolved_reviewer = conn.execute(
            """
            select party_kind, party_key, person_id, responsibility_kind, basis_kind
              from work_responsibilities
             where responsibility_kind = 'reviewer'
            """
        ).fetchone()
        self.assertEqual(unresolved_reviewer, ("unresolved", "github:reviewer-one", None, "reviewer", "source_native"))

        current_evidence = conn.execute(
            """
            select count(*)
              from work_responsibilities r
              join evidences e on e.id = r.latest_evidence_id
             where claim_kind = 'relationship'
               and claim_target_kind = 'work_responsibility'
               and locator_kind = 'tpm_work_responsibility'
            """
        ).fetchone()[0]
        self.assertEqual(current_evidence, conn.execute("select count(*) from work_responsibilities").fetchone()[0])


class WorkActionGateTest(unittest.TestCase):
    def test_current_insight_cards_are_preferred_for_operating_brief(self) -> None:
        base = pd.DataFrame(
            [
                {
                    "insight_kind": "forecast_risk",
                    "subject_key": "repo/example#1",
                    "producer_state": "current",
                }
            ]
        )
        current = pd.DataFrame(
            [
                {
                    "insight_kind": "status_summary",
                    "identity_key": "ci_check_state",
                    "subject_key": "repo/example#2",
                    "producer_state": "current",
                },
                {
                    "insight_kind": "status_summary",
                    "identity_key": "ci_check_state",
                    "subject_key": "repo/example#3",
                    "producer_state": "stale",
                },
            ]
        )

        selected = brief.choose_operating_insight_cards(base, current)

        self.assertEqual(selected["subject_key"].tolist(), ["repo/example#2"])
        self.assertEqual(selected.iloc[0]["identity_key"], "ci_check_state")

    def test_current_insight_cards_refresh_matching_base_fields(self) -> None:
        base = pd.DataFrame(
            [
                {
                    "insight_kind": "developer_correlation",
                    "identity_key": "direct_identity_same_window_jira_load",
                    "subject_kind": "unknown",
                    "subject_key": "person:jira:one",
                    "title": "Fresh title",
                    "details": "Fresh guardrail: never proves causality, ownership, performance, or blocker absence.",
                    "recommended_action": "Fresh action",
                    "producer_state": "current",
                    "stale_reason": "",
                },
                {
                    "insight_kind": "forecast_risk",
                    "identity_key": "risk",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#4",
                    "details": "New base-only card should not bypass current-card selection.",
                    "producer_state": "current",
                    "stale_reason": "",
                },
            ]
        )
        current = pd.DataFrame(
            [
                {
                    "insight_kind": "developer_correlation",
                    "identity_key": "direct_identity_same_window_jira_load",
                    "subject_kind": "unknown",
                    "subject_key": "person:jira:one",
                    "title": "Stale title",
                    "details": "Old guardrail: never proves causality, ownership, or blocker absence.",
                    "recommended_action": "Old action",
                    "producer_state": "current",
                    "stale_reason": "preserved_state",
                }
            ]
        )

        selected = brief.choose_operating_insight_cards(base, current)

        self.assertEqual(selected["subject_key"].tolist(), ["person:jira:one"])
        self.assertEqual(selected.iloc[0]["title"], "Fresh title")
        self.assertEqual(selected.iloc[0]["recommended_action"], "Fresh action")
        self.assertIn("performance", selected.iloc[0]["details"])
        self.assertEqual(selected.iloc[0]["producer_state"], "current")
        self.assertEqual(selected.iloc[0]["stale_reason"], "preserved_state")

    def test_generated_evidence_is_claim_provenance_limited_not_source_limited(self) -> None:
        action_items = pd.DataFrame(
            [
                {
                    "action_type": "decision_or_owner_followup",
                    "decision_state": "validation_lead",
                    "urgency": "high",
                    "source_observation_status": "generated_evidence",
                    "source_coverage_kind": "forecast_risk_backstop",
                    "candidate_dismissed_kinds": "",
                    "needs_human_review": "true",
                }
            ]
        )

        metrics = {row.metric: row.value for row in brief.build_summary(action_items, pd.DataFrame()).itertuples(index=False)}

        self.assertEqual(metrics["coverage_limited_count"], "0")
        self.assertEqual(metrics["source_partial_count"], "0")
        self.assertTrue(brief.is_coverage_limited_action(pd.Series(action_items.iloc[0])))
        self.assertEqual(brief.work_program_item_freshness("generated:forecast_risk_backstop", "pull_request"), "fresh")
        self.assertFalse(
            brief.ontology_program_item_coverage_limited(
                {
                    "source_coverage_state": "generated:forecast_risk_backstop",
                    "freshness_state": "fresh",
                }
            )
        )
        self.assertTrue(
            brief.ontology_program_item_generated_claim_limited(
                {
                    "source_coverage_state": "generated:forecast_risk_backstop",
                    "freshness_state": "fresh",
                }
            )
        )

    def test_not_observed_program_item_is_coverage_limited(self) -> None:
        row = {
            "source_coverage_state": "not_observed",
            "freshness_state": "fresh",
        }

        self.assertTrue(brief.ontology_program_item_coverage_limited(row))
        self.assertEqual(brief.ontology_program_item_coverage_limit_kind(row), "not_observed")

    def test_sparse_kind_readiness_can_promote_only_labeled_action_kind(self) -> None:
        review_queue = pd.DataFrame(
            [
                triage_row("blocker-1", "blocker_candidate"),
                triage_row("blocker-2", "blocker_candidate"),
                triage_row("status-1", "status_summary"),
                label_row(10, "blocker-1", "blocker_candidate"),
                label_row(11, "blocker-2", "blocker_candidate"),
            ]
        )

        readiness = brief.build_live_evaluation_readiness(review_queue)
        gates = brief.build_decision_gates(readiness, pd.DataFrame())

        kind_readiness = gates["kind_readiness"]
        self.assertEqual(kind_readiness["blocker_candidate"]["required"], 2)
        self.assertEqual(kind_readiness["status_summary"]["required"], 1)
        self.assertTrue(kind_readiness["blocker_candidate"]["ready"])
        self.assertFalse(kind_readiness["status_summary"]["ready"])

        state, reason = brief.decision_state_for_action(
            "validate_signal",
            pd.DataFrame([{"insight_kind": "blocker_candidate"}]),
            {"open_review_request_count_by_kind": {"blocker_candidate": 0}},
            gates,
        )
        self.assertEqual(state, "product_action", reason)

        state, reason = brief.decision_state_for_action(
            "review_wait_followup",
            pd.DataFrame([{"insight_kind": "status_summary"}]),
            {"open_review_request_count_by_kind": {"status_summary": 1}},
            gates,
        )
        self.assertEqual(state, "validation_lead")
        self.assertIn("status_summary", reason)

    def test_non_forecast_product_action_reason_does_not_claim_eta_gate_passed(self) -> None:
        review_queue = pd.DataFrame(
            [
                triage_row("status-1", "status_summary"),
                label_row(10, "status-1", "status_summary"),
            ]
        )
        readiness = brief.build_live_evaluation_readiness(review_queue)
        gates = brief.build_decision_gates(readiness, pd.DataFrame([{"metric": "eta_forecast_ready", "value": "false"}]))

        state, reason = brief.decision_state_for_action(
            "review_wait_followup",
            pd.DataFrame([{"insight_kind": "status_summary"}, {"insight_kind": "forecast_risk"}]),
            {"open_review_request_count_by_kind": {"status_summary": 0}},
            gates,
        )

        self.assertEqual(state, "product_action", reason)
        self.assertIn("ETA forecast gate is not required", reason)
        self.assertNotIn("forecast risk cannot become", reason)
        self.assertNotIn("model gates passed", reason)

    def test_forecast_risk_owner_followup_promotes_without_eta_claim(self) -> None:
        forecast_summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false"},
                {"metric": "risk_triage_lift_at_10pct", "value": "0.3446"},
            ]
        )
        gates = brief.build_decision_gates(pd.DataFrame(), forecast_summary)

        state, reason = brief.decision_state_for_action(
            "decision_or_owner_followup",
            pd.DataFrame([{"insight_kind": "forecast_risk"}]),
            {"open_review_request_count_by_kind": {"forecast_risk": 99}},
            gates,
            {
                "fetch_success_count": 1,
                "auth_states": "github_token",
                "coverage_kinds": "authenticated_api_current_observation",
                "current_state": "open",
            },
        )

        self.assertEqual(state, "product_action", reason)
        self.assertIn("attention ordering", reason)
        self.assertIn("not ETA", reason)
        self.assertNotIn("ETA forecast gate passed", reason)

    def test_forecast_risk_owner_followup_needs_risk_triage_backtest(self) -> None:
        gates = brief.build_decision_gates(
            pd.DataFrame(),
            pd.DataFrame([{"metric": "eta_forecast_ready", "value": "false"}]),
        )

        state, reason = brief.decision_state_for_action(
            "decision_or_owner_followup",
            pd.DataFrame([{"insight_kind": "forecast_risk"}]),
            {},
            gates,
            {"fetch_success_count": 1, "auth_states": "github_token"},
        )

        self.assertEqual(state, "validation_lead")
        self.assertIn("risk-triage backtest is not ready", reason)

    def test_ci_requires_required_check_evidence_before_product_action(self) -> None:
        review_queue = pd.DataFrame(
            [
                {
                    **triage_row("status-1", "status_summary"),
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#1084",
                },
                {
                    **label_row(20, "status-1", "status_summary"),
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#1084",
                },
            ]
        )
        readiness = brief.build_live_evaluation_readiness(review_queue)
        gates = brief.build_decision_gates(readiness, pd.DataFrame())
        group = pd.DataFrame([{"insight_kind": "status_summary"}])
        reviews = {"open_review_request_count_by_kind": {"status_summary": 0}}

        state, reason = brief.decision_state_for_action(
            "ci_check_followup",
            group,
            reviews,
            gates,
            {"required_check_match_state": "required_check_coverage_unavailable"},
        )
        self.assertEqual(state, "validation_lead")
        self.assertIn("required-check coverage", reason)

        state, reason = brief.decision_state_for_action(
            "ci_check_followup",
            group,
            reviews,
            gates,
            {
                "required_check_match_state": "required_context_failing_or_pending",
                "failing_required_context_count": 2,
                "pending_required_context_count": 0,
            },
        )
        self.assertEqual(state, "product_action", reason)
        self.assertIn("required check", reason)

    def test_developer_correlation_never_promotes_to_product_action(self) -> None:
        gates = {
            "precision_ready": True,
            "actionability_ready": True,
            "truth_label_coverage": "ready",
            "actionability_label_coverage": "ready",
            "kind_readiness": {
                "developer_correlation": {
                    "ready": True,
                    "truth_labeled": 10,
                    "actionability_labeled": 10,
                    "required": 10,
                }
            },
            "eta_forecast_ready": True,
        }
        group = pd.DataFrame([{"insight_kind": "developer_correlation"}])

        state, reason = brief.decision_state_for_action(
            "review_insight",
            group,
            {"open_review_request_count_by_kind": {"developer_correlation": 0}},
            gates,
        )

        self.assertEqual(state, "validation_lead")
        self.assertIn("workload context only", reason)
        self.assertIn("performance", reason)

    def test_ci_required_check_fields_flow_into_action_observations(self) -> None:
        review_queue = pd.DataFrame(
            [
                {
                    **triage_row("status-1", "status_summary"),
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#1084",
                },
                {
                    **label_row(20, "status-1", "status_summary"),
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#1084",
                },
            ]
        )
        readiness = brief.build_live_evaluation_readiness(review_queue)
        insight_cards = pd.DataFrame(
            [
                {
                    "insight_key": "status-1",
                    "insight_kind": "status_summary",
                    "producer_state": "current",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#1084",
                    "severity": "high",
                    "score": 88,
                    "confidence": 0.9,
                    "title": "CI check state",
                    "details": "CI has failing contexts",
                    "recommended_action": "Review failing CI.",
                    "source_url": "https://github.com/repo/example/pull/1084",
                    "identity_key": "ci_check_state",
                    "model_method": "ci_check_state",
                }
            ]
        )
        check_observations = pd.DataFrame(
            [
                {
                    "observed_at": "2026-06-21T02:00:00+00:00",
                    "subject_key": "repo/example#1084",
                    "effective_state": "open",
                    "fixture_state": "open",
                    "pr_fetch_status_code": 200,
                    "check_fetch_status_code": 200,
                    "status_fetch_status_code": 200,
                    "pr_fetch_complete": True,
                    "check_fetch_complete": True,
                    "status_fetch_complete": True,
                    "fetch_auth_state": "github_token",
                    "fetch_coverage_kind": "authenticated_api_current_observation",
                    "source_coverage_state": "complete",
                    "required_check_coverage_state": "required_checks_observed",
                    "required_check_match_state": "required_context_failing_or_pending",
                    "required_check_context_count": 3,
                    "failing_required_context_count": 2,
                    "pending_required_context_count": 1,
                    "missing_required_context_count": 0,
                    "failing_required_contexts": "maven build (17), maven build (21)",
                    "pending_required_contexts": "e2e",
                    "missing_required_contexts": "",
                    "failing_context_count": 5,
                    "pending_context_count": 1,
                    "failing_contexts": "maven build (17), maven build (21), lint",
                    "pending_contexts": "e2e",
                }
            ]
        )

        action_items = brief.build_action_items(
            insight_cards,
            pd.DataFrame(),
            check_observations,
            review_queue,
            pd.DataFrame(),
            pd.DataFrame(),
            readiness,
            pd.DataFrame(),
            "fixture-source",
            "2026-06-21T02:00:00+00:00",
        )
        self.assertEqual(len(action_items), 1)
        action = action_items.iloc[0]
        self.assertEqual(action["action_type"], "ci_check_followup")
        self.assertEqual(action["required_check_match_state"], "required_context_failing_or_pending")
        self.assertEqual(action["failing_required_context_count"], 2)
        self.assertEqual(action["pending_context_count"], 1)

        observations = brief.build_work_action_observations(action_items, "2026-06-21T02:00:00+00:00")
        observation = observations.iloc[0]
        self.assertEqual(observation["observation_kind"], "ci_signal")
        self.assertEqual(observation["ci_required_check_coverage_state"], "required_checks_observed")
        self.assertEqual(observation["ci_required_check_match_state"], "required_context_failing_or_pending")
        self.assertEqual(observation["ci_failing_required_context_count"], 2)
        self.assertEqual(observation["ci_pending_required_context_count"], 1)
        self.assertEqual(observation["ci_failing_context_count"], 5)
        self.assertIn("maven build (17)", observation["ci_failing_required_contexts"])

    def test_current_pr_source_coverage_overrides_stale_terminal_followup(self) -> None:
        self.assertTrue(
            brief.should_use_source_coverage_followup(
                {
                    "outcome_signal": "subject_became_terminal",
                    "current_state": "merged",
                    "fetch_success_count": 1,
                },
                {
                    "outcome_signal": "still_open",
                    "current_state": "open",
                    "fetch_success_count": 1,
                },
            )
        )

        insight_cards = pd.DataFrame(
            [
                {
                    "insight_key": "status-1",
                    "insight_kind": "status_summary",
                    "producer_state": "current",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#1085",
                    "severity": "high",
                    "score": 88,
                    "confidence": 0.9,
                    "title": "Stale terminal follow-up",
                    "details": "Older follow-up said terminal, but current PR source says open.",
                    "recommended_action": "Review current PR state.",
                    "source_url": "https://github.com/repo/example/pull/1085",
                    "identity_key": "status_summary",
                    "model_method": "status_summary",
                }
            ]
        )
        followups = pd.DataFrame(
            [
                {
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#1085",
                    "outcome_signal": "subject_became_terminal",
                    "current_state": "merged",
                    "baseline_state": "open",
                    "fetch_status_code": 200,
                    "fetch_error": "",
                    "fetch_auth_state": "github_token",
                    "fetch_coverage_kind": "authenticated_api_current_observation",
                    "days_since_source_update": 0,
                    "observed_at": "2026-06-20T00:00:00+00:00",
                }
            ]
        )
        pr_features = pd.DataFrame(
            [
                {
                    "repository": "repo/example",
                    "pr_number": 1085,
                    "state": "open",
                    "source_current_coverage_state": "observed",
                    "source_current_detail_state": "observed",
                    "source_current_observed_at": "2026-06-21T00:00:00+00:00",
                    "source_visibility": "github_token",
                }
            ]
        )

        action_items = brief.build_action_items(
            insight_cards,
            followups,
            pd.DataFrame(),
            pd.DataFrame(),
            pr_features,
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            "fixture-source",
            "2026-06-21T02:00:00+00:00",
        )

        self.assertEqual(len(action_items), 1)
        action = action_items.iloc[0]
        self.assertEqual(action["action_type"], "review_wait_followup")
        self.assertEqual(action["decision_state"], "validation_lead")
        self.assertEqual(action["current_state"], "open")
        self.assertEqual(action["status_signal"], "still_open")
        self.assertEqual(action["source_observation_status"], "observed")
        self.assertNotIn("closeout", action["recommended_action"].lower())

    def test_gold_blocker_labels_stay_validation_only_when_precision_is_quality_gated(self) -> None:
        insight_cards = pd.DataFrame(
            [
                {
                    "insight_key": f"insight:blocker:{idx}",
                    "insight_kind": "blocker_candidate",
                    "producer_state": "current",
                    "subject_kind": "pull_request",
                    "subject_key": f"repo/example#{idx}",
                    "severity": "high",
                    "score": 90 - idx,
                    "confidence": 0.8,
                    "title": f"Blocker candidate {idx}",
                    "details": f"Evidence for blocker candidate {idx}",
                    "recommended_action": "Review and clear blocker.",
                    "source_url": f"https://github.com/repo/example/pull/{idx}",
                    "identity_key": "blocker_candidate",
                }
                for idx in range(10)
            ]
        )
        review_queue = pd.DataFrame(
            [
                gold_blocker_label_row(
                    idx,
                    truth_label="true_positive" if idx < 6 else "false_positive",
                    actionability_label="actionable" if idx < 6 else "not_actionable",
                    review_state="accepted" if idx < 6 else "dismissed",
                )
                for idx in range(10)
            ]
        )
        readiness = brief.build_live_evaluation_readiness(review_queue)

        action_items = brief.build_action_items(
            insight_cards,
            pd.DataFrame(),
            pd.DataFrame(),
            review_queue,
            pd.DataFrame(),
            pd.DataFrame(),
            readiness,
            pd.DataFrame(),
            "fixture-source",
            "2026-06-21T02:00:00+00:00",
        )

        by_subject = {row.subject_key: row for row in action_items.itertuples(index=False)}
        for idx in range(6):
            row = by_subject[f"repo/example#{idx}"]
            self.assertEqual(row.action_type, "validate_signal")
            self.assertEqual(row.decision_state, "validation_lead")
            self.assertIn("product_action_gate quality_gated", row.decision_gate_reason)
        for idx in range(6, 10):
            row = by_subject[f"repo/example#{idx}"]
            self.assertEqual(row.action_type, "dismissed_signal")
            self.assertEqual(row.decision_state, "suppressed_signal")

    def test_gold_blocker_labels_promote_only_after_product_action_quality_gate_passes(self) -> None:
        insight_cards = pd.DataFrame(
            [
                {
                    "insight_key": f"insight:blocker:{idx}",
                    "insight_kind": "blocker_candidate",
                    "producer_state": "current",
                    "subject_kind": "pull_request",
                    "subject_key": f"repo/example#{idx}",
                    "severity": "high",
                    "score": 90 - idx,
                    "confidence": 0.8,
                    "title": f"Blocker candidate {idx}",
                    "details": f"Evidence for blocker candidate {idx}",
                    "recommended_action": "Review and clear blocker.",
                    "source_url": f"https://github.com/repo/example/pull/{idx}",
                    "identity_key": "blocker_candidate",
                }
                for idx in range(10)
            ]
        )
        review_queue = pd.DataFrame(
            [
                gold_blocker_label_row(
                    idx,
                    truth_label="true_positive",
                    actionability_label="actionable",
                    review_state="accepted",
                )
                for idx in range(10)
            ]
        )
        readiness = brief.build_live_evaluation_readiness(review_queue)

        action_items = brief.build_action_items(
            insight_cards,
            pd.DataFrame(),
            pd.DataFrame(),
            review_queue,
            pd.DataFrame(),
            pd.DataFrame(),
            readiness,
            pd.DataFrame(),
            "fixture-source",
            "2026-06-21T02:00:00+00:00",
        )

        by_subject = {row.subject_key: row for row in action_items.itertuples(index=False)}
        self.assertEqual(len(by_subject), 10)
        for idx in range(10):
            row = by_subject[f"repo/example#{idx}"]
            self.assertEqual(row.action_type, "clear_blocker")
            self.assertEqual(row.decision_state, "product_action")

    def test_forecast_backstop_adds_actions_for_uncovered_high_risk_forecasts(self) -> None:
        action_items = pd.DataFrame(
            [
                {
                    **empty_action_item_row("repo/example#1"),
                    "action_key": "tpm-action:existing",
                    "action_type": "review_wait_followup",
                    "decision_state": "validation_lead",
                },
                {
                    **empty_action_item_row("repo/example#2"),
                    "action_key": "tpm-action:dismissed",
                    "action_type": "dismissed_signal",
                    "decision_state": "suppressed_signal",
                },
            ],
            columns=brief.empty_action_items().columns,
        )
        pr_features = pd.DataFrame(
            [
                forecast_pr_row(1, risk_score=100, overdue_days=30, author_login="owner-one"),
                forecast_pr_row(2, risk_score=85, overdue_days=40, author_login="owner-two"),
                forecast_pr_row(3, risk_score=80, overdue_days=50, author_login="owner-three"),
                forecast_pr_row(4, risk_score=20, risk_band="low", overdue_days=0, author_login="owner-four"),
                {**forecast_pr_row(5, risk_score=95, overdue_days=10, author_login="owner-five"), "source_current_detail_state": "failed"},
                {
                    key: value
                    for key, value in forecast_pr_row(6, risk_score=90, overdue_days=12, author_login="owner-six").items()
                    if key not in {"source_current_coverage_state", "source_current_detail_state", "source_current_observed_at"}
                },
            ]
        )
        forecast_summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false", "note": "gate"},
                {"metric": "risk_triage_lift_at_10pct", "value": "0.3446", "note": "risk triage ready"},
            ]
        )

        out = brief.append_forecast_risk_backstop_actions(
            action_items,
            pr_features,
            forecast_summary,
            "2026-06-21T08:00:00+00:00",
        )

        active = out[out["action_type"] != "dismissed_signal"]
        active_by_subject = {row.subject_key: row for row in active.itertuples(index=False)}
        dismissed_by_subject = {row.subject_key: row for row in out[out["action_type"] == "dismissed_signal"].itertuples(index=False)}
        self.assertEqual(active_by_subject["repo/example#1"].action_key, "tpm-action:existing")
        self.assertEqual(active_by_subject["repo/example#2"].action_type, "decision_or_owner_followup")
        self.assertEqual(active_by_subject["repo/example#2"].decision_state, "product_action")
        self.assertIn("forecast risk has no existing open TPM action", active_by_subject["repo/example#2"].decision_gate_reason)
        self.assertIn("not an ETA commitment", active_by_subject["repo/example#2"].decision_gate_reason)
        self.assertEqual(active_by_subject["repo/example#2"].owner_hint, "github:owner-two")
        self.assertEqual(active_by_subject["repo/example#2"].source_observation_status, "observed")
        self.assertEqual(active_by_subject["repo/example#2"].source_coverage_kind, "fixture_source_sync:observed,pr_detail:observed")
        self.assertEqual(active_by_subject["repo/example#2"].current_state, "open")
        self.assertEqual(active_by_subject["repo/example#3"].action_type, "decision_or_owner_followup")
        self.assertEqual(active_by_subject["repo/example#3"].decision_state, "product_action")
        self.assertEqual(active_by_subject["repo/example#6"].source_observation_status, "generated_evidence")
        self.assertEqual(active_by_subject["repo/example#6"].source_coverage_kind, "forecast_risk_backstop")
        self.assertEqual(active_by_subject["repo/example#6"].decision_state, "validation_lead")
        self.assertIn("repo/example#2", dismissed_by_subject)
        self.assertNotIn("repo/example#4", active_by_subject)
        self.assertNotIn("repo/example#5", active_by_subject)

    def test_work_program_evidence_need_builder_prioritizes_executable_gaps(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "review_requests_blocker_candidate", "value": "4"},
                {"metric": "measurement_required_blocker_candidate", "value": "4"},
                {"metric": "truth_labeled_blocker_candidate", "value": "1"},
                {"metric": "actionability_labeled_blocker_candidate", "value": "1"},
                {"metric": "ready_to_measure_blocker_candidate", "value": "false"},
            ]
        )
        forecast_summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false"},
                {"metric": "merged_pr_count", "value": "50"},
                {"metric": "backtest_best_model", "value": "median_cycle_baseline"},
                {"metric": "backtest_median_mae_days", "value": "6.53"},
                {"metric": "backtest_heuristic_mae_days", "value": "11.44"},
                {"metric": "backtest_hist_gradient_boosting_mae_days", "value": "6.15"},
                {"metric": "backtest_random_forest_mae_days", "value": "8.83"},
                {"metric": "eta_primary_blocker", "value": "kfold_model_does_not_beat_baseline"},
                {"metric": "eta_kfold_random_forest_improvement_pct", "value": "-35.22"},
                {"metric": "eta_chronological_random_forest_improvement_pct", "value": "7.7"},
                {"metric": "eta_temporal_snapshot_state", "value": "as_of_feature_snapshot_series_missing"},
                {"metric": "eta_next_evidence_needed", "value": "collect_repeated_as_of_pr_snapshots_and_closed_outcomes"},
            ]
        )
        facts = {
            "program_status_counts": {"model_quality": 1, "source_repair": 2},
            "decision_state_counts": {"validation_lead": 3, "product_action": 2},
            "forecast_risk_target_count": 2,
            "forecast_risk_targets": [
                {
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#forecast",
                    "risk_band": "critical",
                    "risk_score": 98,
                    "predicted_remaining_days": 0,
                    "overdue_days": 42.5,
                    "readiness_state": "gated",
                    "ready_for_eta": 0,
                    "readiness_reason": "ETA forecast is gated by backtest.",
                    "source_url": "https://github.com/repo/example/pull/forecast",
                    "work_action_id": 48,
                    "action_key": "tpm-action:forecast",
                    "action_state": "open",
                    "action_owner_key": "github:forecast-owner",
                    "action_source_url": "https://github.com/repo/example/pull/forecast",
                }
            ],
            "product_action_count": 2,
            "needs_decision_count": 0,
            "needs_action_dependency_count": 2,
            "auth_limited_observation_counts": {"anonymous_observation": 2},
            "auth_limited_observation_targets": [
                {
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#coverage",
                    "title": "Anonymous observation coverage gap",
                    "program_status": "validate_signal",
                    "decision_state": "validation_lead",
                    "source_coverage_state": "anonymous_success:public_api_current_observation",
                    "freshness_state": "partial",
                    "source_url": "https://github.com/repo/example/pull/coverage",
                    "work_action_id": 44,
                    "action_key": "tpm-action:coverage",
                    "action_state": "open",
                    "action_owner_key": "github:coverage-owner",
                    "action_source_url": "https://github.com/repo/example/pull/coverage",
                },
                {
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#stale-coverage",
                    "title": "Stale anonymous observation coverage gap",
                    "program_status": "dismissed",
                    "decision_state": "suppressed_signal",
                    "source_coverage_state": "anonymous_success:public_api_current_observation",
                    "freshness_state": "partial",
                    "source_url": "https://github.com/repo/example/pull/stale-coverage",
                    "work_action_id": 49,
                    "action_key": "tpm-action:stale-coverage",
                    "action_state": "closed",
                    "action_owner_key": "github:coverage-owner",
                    "action_source_url": "https://github.com/repo/example/pull/stale-coverage",
                }
            ],
            "product_decision_targets": [
                {
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#decision",
                    "title": "Clear blocker candidate: repo/example#decision",
                    "program_status": "closure_candidate",
                    "decision_state": "product_action",
                    "owner_key": "github:decision-owner",
                    "source_url": "https://github.com/repo/example/pull/decision",
                    "work_action_id": 45,
                    "action_key": "tpm-action:decision",
                    "action_state": "open",
                    "action_owner_key": "github:decision-owner",
                    "action_source_url": "https://github.com/repo/example/pull/decision",
                }
            ],
            "dependency_action_targets": [
                {
                    "key": "dependency-edge:open",
                    "edge_kind": "needs_action",
                    "from_kind": "blocker",
                    "from_key": "work-blocker:open",
                    "to_kind": "action",
                    "to_key": "tpm-action:open",
                    "risk_signal": "validation_lead",
                    "source_url": "https://github.com/repo/example/pull/dependency",
                    "work_action_id": 46,
                    "action_key": "tpm-action:open",
                    "action_type": "validate_signal",
                    "action_state": "open",
                    "action_subject_kind": "pull_request",
                    "action_subject_key": "repo/example#dependency",
                    "action_owner_key": "github:dependency-owner",
                    "action_decision_state": "validation_lead",
                    "action_source_url": "https://github.com/repo/example/pull/dependency",
                },
                {
                    "key": "dependency-edge:closed",
                    "edge_kind": "needs_action",
                    "from_kind": "blocker",
                    "from_key": "work-blocker:closed",
                    "to_kind": "action",
                    "to_key": "tpm-action:closed",
                    "risk_signal": "suppressed_signal",
                    "source_url": "https://github.com/repo/example/pull/stale",
                    "work_action_id": 47,
                    "action_key": "tpm-action:closed",
                    "action_type": "dismissed_signal",
                    "action_state": "closed",
                    "action_subject_kind": "pull_request",
                    "action_subject_key": "repo/example#stale",
                    "action_owner_key": "github:stale-owner",
                    "action_decision_state": "suppressed_signal",
                    "action_source_url": "https://github.com/repo/example/pull/stale",
                },
            ],
            "generated_claim_limit_counts": {"generated_evidence": 1},
            "generated_claim_limited_targets": [
                {
                    "subject_kind": "unknown",
                    "subject_key": "person:jira:generated",
                    "title": "Same-window Jira load near PR owner",
                    "program_status": "validate_signal",
                    "decision_state": "validation_lead",
                    "source_coverage_state": "generated:direct_identity_same_window_overlap",
                    "freshness_state": "partial",
                    "source_url": "",
                    "work_action_id": 50,
                    "action_key": "tpm-action:generated-claim",
                    "action_state": "open",
                    "action_owner_key": "",
                    "action_source_url": "",
                }
            ],
            "measurement_label_targets": [
                {
                    "insight_kind": "blocker_candidate",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#9",
                    "title": "Possible blocker signal",
                    "source_url": "https://github.com/repo/example/pull/9",
                    "review_request_count": 1,
                    "score": 88,
                }
            ],
            "overloaded_owner_count": 1,
            "unassigned_action_count": 1,
            "owner_load_row_count": 2,
            "owner_load_action_count": 5,
            "owner_load_targets": [
                {
                    "owner_key": "github:busy-owner",
                    "load_status": "overloaded",
                    "action_count": 3,
                    "top_action_type": "decision_or_owner_followup",
                    "top_subjects": "repo/example#1, repo/example#2",
                    "recommended_focus": "Decide owner path for aged work.",
                    "source_url": "https://github.com/repo/example",
                },
                {
                    "owner_key": "(unassigned)",
                    "load_status": "watch",
                    "action_count": 1,
                    "top_action_type": "model_quality_review",
                    "top_subjects": "forecast-risk-model",
                    "recommended_focus": "Assign model quality review.",
                    "source_url": "https://github.com/repo/example",
                },
            ],
            "active_blocker_count": 1,
            "active_blocker_impact_count": 1,
            "active_blocker_targets": [
                {
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#7",
                    "owner_key": "github:owner-seven",
                    "severity": "critical",
                    "blocker_kind": "source_signal",
                    "title": "Clear blocker candidate: repo/example#7",
                    "source_url": "https://github.com/repo/example/pull/7",
                    "active_impact_count": 2,
                    "work_action_id": 42,
                    "action_key": "tpm-action:blocker",
                    "action_state": "open",
                    "action_owner_key": "github:owner-seven",
                    "action_source_url": "https://github.com/repo/example/pull/7",
                }
            ],
        }

        needs = brief.build_work_program_evidence_needs(readiness, forecast_summary, facts)
        by_key = {row["key"]: row for row in needs}

        self.assertEqual(needs[0]["key"], "blocker_clearance:active")
        self.assertEqual(by_key["forecast_readiness:backtest"]["execution_state"], "actions_open")
        self.assertEqual(by_key["forecast_readiness:risk_triage"]["execution_state"], "risk_triage_actions_open")
        self.assertEqual(by_key["forecast_readiness:risk_triage"]["backing_action_count"], 2)
        forecast_need = by_key["forecast_readiness:risk_triage:repo/example#forecast"]
        self.assertEqual(forecast_need["target_kind"], "pull_request")
        self.assertEqual(forecast_need["target_key"], "repo/example#forecast")
        self.assertEqual(forecast_need["metric_key"], "critical")
        self.assertEqual(forecast_need["execution_state"], "risk_action_open")
        self.assertEqual(forecast_need["backing_action_count"], 1)
        self.assertEqual(forecast_need["owner_key"], "github:forecast-owner")
        self.assertEqual(forecast_need["action_key"], "tpm-action:forecast")
        self.assertEqual(forecast_need["action_state"], "open")
        self.assertEqual(forecast_need["source_url"], "https://github.com/repo/example/pull/forecast")
        self.assertIn("github:forecast-owner", forecast_need["next_execution_step"])
        self.assertIn("until the forecast backtest gate passes", forecast_need["next_execution_step"])
        self.assertEqual(by_key["measurement_labels:blocker_candidate"]["execution_state"], "validation_actions_open")
        actionability_need = by_key["measurement_actionability:blocker_candidate"]
        self.assertEqual(actionability_need["gate_key"], "global_insight_actionability")
        self.assertEqual(actionability_need["evidence_kind"], "actionability_labels")
        self.assertEqual(actionability_need["current_count"], 1)
        self.assertEqual(actionability_need["required_count"], 4)
        product_precision_need = by_key["measurement_precision:product_action"]
        self.assertEqual(product_precision_need["gate_key"], "measurement_precision")
        self.assertEqual(product_precision_need["evidence_kind"], "product_action_precision_labels")
        self.assertEqual(product_precision_need["metric_key"], "no product-action insight kinds")
        self.assertEqual(product_precision_need["execution_state"], "product_action_measurement_scope_missing")
        product_actionability_need = by_key["measurement_actionability:product_action"]
        self.assertEqual(product_actionability_need["gate_key"], "measurement_actionability")
        self.assertEqual(product_actionability_need["evidence_kind"], "product_action_actionability_labels")
        self.assertEqual(product_actionability_need["metric_key"], "no product-action insight kinds")
        self.assertEqual(product_actionability_need["execution_state"], "product_action_measurement_scope_missing")
        label_need = by_key["measurement_labels:blocker_candidate:repo/example#9"]
        self.assertEqual(label_need["target_kind"], "pull_request")
        self.assertEqual(label_need["target_key"], "repo/example#9")
        self.assertEqual(label_need["metric_key"], "blocker_candidate")
        self.assertEqual(label_need["execution_state"], "review_request_open")
        self.assertEqual(label_need["backing_action_count"], 1)
        self.assertIn("truth and actionability", label_need["next_execution_step"])
        self.assertEqual(by_key["source_authentication:anonymous_observation"]["evidence_kind"], "source_authentication")
        self.assertEqual(by_key["source_authentication:anonymous_observation"]["execution_state"], "review_actions_open")
        source_need = by_key["source_authentication:anonymous_observation:repo/example#coverage"]
        self.assertEqual(source_need["target_kind"], "pull_request")
        self.assertEqual(source_need["target_key"], "repo/example#coverage")
        self.assertEqual(source_need["metric_key"], "anonymous_observation")
        self.assertEqual(source_need["execution_state"], "action_open")
        self.assertEqual(source_need["backing_action_count"], 1)
        self.assertEqual(source_need["owner_key"], "github:coverage-owner")
        self.assertEqual(source_need["action_key"], "tpm-action:coverage")
        self.assertEqual(source_need["action_state"], "open")
        self.assertIn("authenticated source access", source_need["next_execution_step"])
        stale_source_need = by_key["source_authentication:anonymous_observation:repo/example#stale-coverage"]
        self.assertEqual(stale_source_need["execution_state"], "stale_source_action_review_needed")
        self.assertEqual(stale_source_need["backing_action_count"], 0)
        self.assertEqual(stale_source_need["action_key"], "tpm-action:stale-coverage")
        self.assertEqual(stale_source_need["action_state"], "closed")
        self.assertEqual(by_key["claim_provenance:generated_evidence"]["evidence_kind"], "generated_evidence")
        self.assertEqual(by_key["claim_provenance:generated_evidence"]["execution_state"], "qa_actions_open")
        generated_need = by_key["claim_provenance:generated_evidence:person:jira:generated"]
        self.assertEqual(generated_need["target_kind"], "unknown")
        self.assertEqual(generated_need["target_key"], "person:jira:generated")
        self.assertEqual(generated_need["execution_state"], "qa_action_open")
        self.assertEqual(generated_need["backing_action_count"], 1)
        self.assertIn("generated or derived claim evidence", generated_need["next_execution_step"])
        self.assertEqual(by_key["owner_load:rebalance"]["backing_action_count"], 5)
        self.assertEqual(by_key["owner_load:rebalance"]["execution_state"], "owner_load_rows_open")
        owner_need = by_key["owner_load:owner:github:busy-owner"]
        self.assertEqual(owner_need["target_kind"], "owner")
        self.assertEqual(owner_need["target_key"], "github:busy-owner")
        self.assertEqual(owner_need["owner_key"], "github:busy-owner")
        self.assertEqual(owner_need["execution_state"], "owner_queue_overloaded")
        self.assertEqual(owner_need["backing_action_count"], 3)
        self.assertIn("repo/example#1", owner_need["next_execution_step"])
        unassigned_need = by_key["owner_load:owner:(unassigned)"]
        self.assertEqual(unassigned_need["execution_state"], "assignment_needed")
        self.assertEqual(unassigned_need["backing_action_count"], 1)
        self.assertIn("forecast-risk-model", unassigned_need["next_execution_step"])
        blocker_need = by_key["blocker_clearance:pull_request:repo/example#7"]
        self.assertEqual(blocker_need["target_kind"], "pull_request")
        self.assertEqual(blocker_need["target_key"], "repo/example#7")
        self.assertEqual(blocker_need["execution_state"], "action_open")
        self.assertEqual(blocker_need["backing_action_count"], 1)
        self.assertEqual(blocker_need["owner_key"], "github:owner-seven")
        self.assertEqual(blocker_need["action_key"], "tpm-action:blocker")
        self.assertEqual(blocker_need["action_state"], "open")
        self.assertIn("github:owner-seven", blocker_need["next_execution_step"])
        self.assertIn("https://github.com/repo/example/pull/7", blocker_need["next_execution_step"])
        self.assertEqual(by_key["product_decision:human_review"]["execution_state"], "decision_actions_open")
        self.assertEqual(by_key["product_decision:human_review"]["backing_action_count"], 2)
        decision_need = by_key["product_decision:pull_request:repo/example#decision"]
        self.assertEqual(decision_need["target_kind"], "pull_request")
        self.assertEqual(decision_need["target_key"], "repo/example#decision")
        self.assertEqual(decision_need["metric_key"], "product_action")
        self.assertEqual(decision_need["execution_state"], "decision_action_open")
        self.assertEqual(decision_need["backing_action_count"], 1)
        self.assertEqual(decision_need["owner_key"], "github:decision-owner")
        self.assertEqual(decision_need["action_key"], "tpm-action:decision")
        self.assertEqual(decision_need["action_state"], "open")
        self.assertEqual(decision_need["source_url"], "https://github.com/repo/example/pull/decision")
        self.assertIn("github:decision-owner", decision_need["next_execution_step"])
        self.assertIn("human-approved", decision_need["next_execution_step"])
        self.assertEqual(by_key["dependency_pressure:needs_action"]["execution_state"], "dependency_actions_open")
        self.assertEqual(by_key["dependency_pressure:needs_action"]["backing_action_count"], 2)
        dependency_need = by_key["dependency_pressure:dependency-edge:open"]
        self.assertEqual(dependency_need["target_kind"], "pull_request")
        self.assertEqual(dependency_need["target_key"], "repo/example#dependency")
        self.assertEqual(dependency_need["metric_key"], "validation_lead")
        self.assertEqual(dependency_need["execution_state"], "dependency_action_open")
        self.assertEqual(dependency_need["backing_action_count"], 1)
        self.assertEqual(dependency_need["owner_key"], "github:dependency-owner")
        self.assertEqual(dependency_need["action_key"], "tpm-action:open")
        self.assertEqual(dependency_need["action_state"], "open")
        self.assertEqual(dependency_need["source_url"], "https://github.com/repo/example/pull/dependency")
        self.assertIn("github:dependency-owner", dependency_need["next_execution_step"])
        stale_dependency = by_key["dependency_pressure:dependency-edge:closed"]
        self.assertEqual(stale_dependency["execution_state"], "stale_dependency_review_needed")
        self.assertEqual(stale_dependency["backing_action_count"], 0)
        self.assertEqual(stale_dependency["owner_key"], "github:stale-owner")
        self.assertEqual(stale_dependency["action_key"], "tpm-action:closed")
        self.assertEqual(stale_dependency["action_state"], "closed")
        self.assertIn("linked action tpm-action:closed is closed", stale_dependency["next_execution_step"])

    def test_program_evidence_needs_materialize_gate_and_action_edges(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_action_tables(conn)
        conn.executescript(
            """
            create table workstreams (
              id integer primary key autoincrement,
              key text unique,
              source_system text,
              source_instance text,
              external_kind text
            );
            create table work_program_quality_gates (
              id integer primary key autoincrement,
              key text unique,
              workstream_id integer,
              workstream_key text,
              generated_at text,
              gate_key text,
              gate_state text,
              blocking integer,
              detail text,
              recommended_action text,
              source_system text,
              source_instance text,
              external_kind text,
              external_id text,
              source_url text,
              latest_evidence_id integer,
              evidence_count integer,
              freshness_state text,
              visibility text,
              confidence real,
              event_count integer,
              first_seen_at text,
              last_activity_at text,
              rank_score real,
              created_at text,
              updated_at text
            );
            create table work_program_evidence_needs (
              id integer primary key autoincrement,
              key text unique,
              workstream_id integer,
              workstream_key text,
              generated_at text,
              gate_key text,
              evidence_kind text,
              priority text,
              target_kind text,
              target_key text,
              owner_key text,
              action_key text,
              work_action_id integer,
              quality_gate_id integer,
              action_state text,
              metric_key text,
              execution_state text,
              backing_action_count integer,
              current_count integer,
              required_count integer,
              missing_count integer,
              current_rate real,
              required_rate real,
              recommended_action text,
              next_execution_step text,
              source_system text,
              source_instance text,
              external_kind text,
              external_id text,
              source_url text,
              latest_evidence_id integer,
              evidence_count integer,
              freshness_state text,
              visibility text,
              confidence real,
              event_count integer,
              first_seen_at text,
              last_activity_at text,
              rank_score real,
              created_at text,
              updated_at text
            );
            insert into workstreams (id, key, source_system, source_instance, external_kind)
            values (7, 'workstream:flink-kubernetes-operator', 'cubicle_analytics', 'fixture-source', 'tpm_workstream');
            insert into work_actions (id, key, source_system, source_instance, external_kind, action_state)
            values (11, 'tpm-action:forecast', 'cubicle_analytics', 'fixture-source', 'tpm_work_action', 'open');
            insert into work_program_quality_gates (
              id, key, workstream_id, workstream_key, generated_at, gate_key, gate_state,
              blocking, detail, recommended_action, source_system, source_instance,
              external_kind, external_id, rank_score
            )
            values (
              13, 'gate:forecast', 7, 'flink-kubernetes-operator',
              '2026-06-23T03:10:00+00:00', 'forecast_readiness', 'gated',
              1, 'Model quality gate is blocked.', 'Improve model validation.',
              'cubicle_analytics', 'fixture-source', 'tpm_work_program_quality_gate',
              'flink-kubernetes-operator|2026-06-23T03:10:00+00:00|forecast_readiness',
              100.0
            ), (
              14, 'gate:measurement-precision', 7, 'flink-kubernetes-operator',
              '2026-06-23T03:10:00+00:00', 'measurement_precision', 'gated',
              1, 'Product-action precision is blocked.', 'Label product-action insight precision.',
              'cubicle_analytics', 'fixture-source', 'tpm_work_program_quality_gate',
              'flink-kubernetes-operator|2026-06-23T03:10:00+00:00|measurement_precision',
              90.0
            ), (
              15, 'gate:measurement-actionability', 7, 'flink-kubernetes-operator',
              '2026-06-23T03:10:00+00:00', 'measurement_actionability', 'gated',
              1, 'Product-action actionability is blocked.', 'Label product-action insight actionability.',
              'cubicle_analytics', 'fixture-source', 'tpm_work_program_quality_gate',
              'flink-kubernetes-operator|2026-06-23T03:10:00+00:00|measurement_actionability',
              89.0
            );
            """
        )

        old_facts = brief.ontology_work_program_adversarial_facts
        old_build = brief.build_work_program_evidence_needs
        try:
            brief.ontology_work_program_adversarial_facts = lambda *_args, **_kwargs: {}
            brief.build_work_program_evidence_needs = lambda *_args, **_kwargs: [
                {
                    "key": "forecast_readiness:risk_triage:repo/example#1",
                    "gate_key": "forecast_readiness",
                    "evidence_kind": "forecast_backtest",
                    "priority": "high",
                    "target_kind": "pull_request",
                    "target_key": "repo/example#1",
                    "owner_key": "github:owner",
                    "action_key": "tpm-action:forecast",
                    "action_state": "open",
                    "metric_key": "critical",
                    "execution_state": "risk_action_open",
                    "backing_action_count": 1,
                    "current_count": 0,
                    "required_count": 1,
                    "missing_count": 1,
                    "recommended_action": "Improve model validation.",
                    "next_execution_step": "Review forecast action.",
                    "source_url": "https://github.com/repo/example/pull/1",
                },
                {
                    "key": "measurement_precision:product_action",
                    "gate_key": "measurement_precision",
                    "evidence_kind": "product_action_precision_labels",
                    "priority": "high",
                    "target_kind": "workstream",
                    "target_key": "flink-kubernetes-operator",
                    "metric_key": "status_summary",
                    "execution_state": "product_action_labels_missing",
                    "backing_action_count": 0,
                    "current_count": 0,
                    "required_count": 10,
                    "missing_count": 10,
                    "recommended_action": "Label product-action insight precision.",
                    "next_execution_step": "Collect product-action precision labels.",
                },
                {
                    "key": "measurement_actionability:product_action",
                    "gate_key": "measurement_actionability",
                    "evidence_kind": "product_action_actionability_labels",
                    "priority": "high",
                    "target_kind": "workstream",
                    "target_key": "flink-kubernetes-operator",
                    "metric_key": "status_summary",
                    "execution_state": "product_action_labels_missing",
                    "backing_action_count": 0,
                    "current_count": 0,
                    "required_count": 10,
                    "missing_count": 10,
                    "recommended_action": "Label product-action insight actionability.",
                    "next_execution_step": "Collect product-action actionability labels.",
                }
            ]
            brief.persist_work_program_evidence_needs_to_ontology(
                conn,
                "fixture-source",
                pd.DataFrame(),
                pd.DataFrame(),
                "2026-06-23T03:10:00+00:00",
            )
        finally:
            brief.ontology_work_program_adversarial_facts = old_facts
            brief.build_work_program_evidence_needs = old_build

        rows = conn.execute(
            """
            select action_key, work_action_id, gate_key, quality_gate_id, latest_evidence_id
              from work_program_evidence_needs
             where external_kind = 'tpm_work_program_evidence_need'
             order by gate_key
            """
        ).fetchall()
        by_gate = {row[2]: row for row in rows}
        self.assertEqual(by_gate["forecast_readiness"][0], "tpm-action:forecast")
        self.assertEqual(by_gate["forecast_readiness"][1], 11)
        self.assertEqual(by_gate["forecast_readiness"][3], 13)
        self.assertEqual(by_gate["measurement_precision"][3], 14)
        self.assertEqual(by_gate["measurement_actionability"][3], 15)
        self.assertTrue(all(row[4] is not None for row in rows))

    def test_tpm_function_readiness_materializes_blocking_gate_edges(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_generated_evidence_table(conn)
        conn.executescript(
            """
            create table workstreams (
              id integer primary key autoincrement,
              key text unique,
              source_system text,
              source_instance text,
              external_kind text
            );
            create table work_program_quality_gates (
              id integer primary key autoincrement,
              key text unique,
              workstream_id integer,
              workstream_key text,
              generated_at text,
              gate_key text,
              gate_state text,
              blocking integer,
              detail text,
              recommended_action text,
              source_system text,
              source_instance text,
              external_kind text,
              external_id text,
              source_url text,
              latest_evidence_id integer,
              evidence_count integer,
              freshness_state text,
              visibility text,
              confidence real,
              event_count integer,
              first_seen_at text,
              last_activity_at text,
              rank_score real,
              created_at text,
              updated_at text
            );
            create table work_program_tpm_function_readinesses (
              id integer primary key autoincrement,
              key text unique,
              workstream_id integer,
              workstream_key text,
              generated_at text,
              function_key text,
              function_name text,
              readiness_state text,
              automation_state text,
              human_required integer,
              supporting_signal_count integer,
              blocking_gate_keys text,
              detail text,
              recommended_action text,
              source_system text,
              source_instance text,
              external_kind text,
              external_id text,
              source_url text,
              latest_evidence_id integer,
              evidence_count integer,
              freshness_state text,
              visibility text,
              confidence real,
              event_count integer,
              first_seen_at text,
              last_activity_at text,
              rank_score real,
              created_at text,
              updated_at text
            );
            create table work_program_tpm_function_readiness_blocking_quality_gates (
              work_program_tpm_function_readiness_id integer,
              work_program_quality_gate_id integer,
              primary key (work_program_tpm_function_readiness_id, work_program_quality_gate_id)
            );
            insert into workstreams (id, key, source_system, source_instance, external_kind)
            values (7, 'workstream:flink-kubernetes-operator', 'cubicle_analytics', 'fixture-source', 'tpm_workstream');
            insert into work_program_quality_gates (
              id, key, workstream_id, workstream_key, generated_at, gate_key, gate_state,
              blocking, detail, recommended_action, source_system, source_instance,
              external_kind, external_id, rank_score
            )
            values (
              13, 'gate:forecast', 7, 'flink-kubernetes-operator',
              '2026-06-23T03:10:00+00:00', 'forecast_readiness', 'gated',
              1, 'Forecast gate is blocked.', 'Improve model validation.',
              'cubicle_analytics', 'fixture-source', 'tpm_work_program_quality_gate',
              'flink-kubernetes-operator|2026-06-23T03:10:00+00:00|forecast_readiness',
              100.0
            );
            """
        )

        old_facts = brief.ontology_work_program_adversarial_facts
        old_build = brief.build_work_program_tpm_function_readiness
        try:
            brief.ontology_work_program_adversarial_facts = lambda *_args, **_kwargs: {}
            brief.build_work_program_tpm_function_readiness = lambda *_args, **_kwargs: [
                {
                    "function_key": "forecast_triage",
                    "function_name": "Forecast triage",
                    "readiness_state": "blocked",
                    "automation_state": "forecast_gated",
                    "human_required": True,
                    "supporting_signal_count": 3,
                    "blocking_gate_keys": ["forecast_readiness"],
                    "detail": "Forecast model does not beat baseline.",
                    "recommended_action": "Improve model validation.",
                }
            ]
            brief.persist_work_program_tpm_function_readiness_to_ontology(
                conn,
                "fixture-source",
                pd.DataFrame(),
                pd.DataFrame(),
                "2026-06-23T03:10:00+00:00",
            )
        finally:
            brief.ontology_work_program_adversarial_facts = old_facts
            brief.build_work_program_tpm_function_readiness = old_build

        row = conn.execute(
            """
            select id, blocking_gate_keys, latest_evidence_id
              from work_program_tpm_function_readinesses
             where external_kind = 'tpm_work_program_tpm_function_readiness'
            """
        ).fetchone()
        self.assertIsNotNone(row)
        self.assertEqual(row[1], "forecast_readiness")
        self.assertIsNotNone(row[2])
        self.assertEqual(
            conn.execute(
                """
                select work_program_tpm_function_readiness_id, work_program_quality_gate_id
                  from work_program_tpm_function_readiness_blocking_quality_gates
                """
            ).fetchall(),
            [(row[0], 13)],
        )

    def test_work_program_brief_caveat_builder_surfaces_claim_limits(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "false"},
                {"metric": "gated_insight_kind_count", "value": "2"},
                {"metric": "open_review_request_count", "value": "5"},
            ]
        )
        forecast_summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false"},
                {"metric": "merged_pr_count", "value": "7"},
            ]
        )
        facts = {
            "source_coverage_limited_count": 3,
            "auth_limited_observation_count": 2,
            "generated_claim_limited_count": 1,
            "program_item_evidence_refs": ["program item evidence"],
            "overloaded_owner_count": 1,
            "unassigned_action_count": 2,
            "owner_load_evidence_refs": ["owner load evidence"],
            "active_blocker_count": 1,
            "active_blocker_impact_count": 1,
            "blocker_evidence_refs": ["blocker evidence"],
            "needs_action_dependency_count": 4,
            "forecast_evidence_refs": ["forecast evidence"],
        }

        caveats = brief.build_work_program_brief_caveats(readiness, forecast_summary, facts)
        by_key = {row["key"]: row for row in caveats}

        self.assertEqual(
            list(by_key),
            [
                "forecast_gated",
                "measurement_gated",
                "coverage_limited",
                "source_authentication_limited",
                "generated_claim_provenance",
                "owner_load",
                "active_blockers",
                "dependency_pressure",
            ],
        )
        self.assertIn("anonymous source observation", by_key["source_authentication_limited"]["detail"])
        self.assertIn("generated or derived claim evidence", by_key["generated_claim_provenance"]["detail"])
        self.assertEqual(by_key["owner_load"]["severity"], "danger")
        self.assertIn("Do not present forecast dates", by_key["forecast_gated"]["recommended_action"])
        self.assertEqual(by_key["active_blockers"]["evidence_ref"], "blocker evidence")

    def test_work_program_brief_snapshot_builder_persists_operating_header(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "true"},
            ]
        )
        forecast_summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false"},
                {"metric": "merged_pr_count", "value": "50"},
                {"metric": "backtest_best_model", "value": "median_cycle_baseline"},
                {"metric": "backtest_median_mae_days", "value": "6.53"},
                {"metric": "backtest_heuristic_mae_days", "value": "11.44"},
                {"metric": "backtest_hist_gradient_boosting_mae_days", "value": "6.15"},
                {"metric": "backtest_random_forest_mae_days", "value": "8.83"},
                {"metric": "eta_primary_blocker", "value": "kfold_model_does_not_beat_baseline"},
                {"metric": "eta_kfold_random_forest_improvement_pct", "value": "-35.22"},
                {"metric": "eta_chronological_random_forest_improvement_pct", "value": "7.70"},
                {"metric": "eta_temporal_snapshot_state", "value": "as_of_feature_snapshot_series_missing"},
                {"metric": "eta_next_evidence_needed", "value": "collect_repeated_as_of_pr_snapshots_and_closed_outcomes"},
            ]
        )
        facts = {
            "total_count": 12,
            "program_status_counts": {"needs_decision": 1},
            "decision_state_counts": {"product_action": 2, "validation_lead": 3},
            "source_coverage_limited_count": 4,
            "active_blocker_count": 1,
            "active_blocker_impact_count": 2,
            "needs_action_dependency_count": 5,
            "overloaded_owner_count": 1,
            "unassigned_action_count": 1,
        }
        gates = [{"key": "source_coverage", "blocking": True}]
        caveats = [{"key": "active_blockers", "severity": "danger"}]
        risk_drivers = [{"key": "risk-1"}]

        snapshot = brief.build_work_program_brief_snapshot(readiness, forecast_summary, facts, gates, caveats, risk_drivers)

        self.assertEqual(snapshot["operating_status"], "blocked")
        self.assertEqual(snapshot["decision_pressure"], "blocked")
        self.assertEqual(snapshot["forecast_state"], "gated")
        self.assertEqual(snapshot["primary_risk"], "active_blockers")
        self.assertIn("blocked: 12 typed program items", snapshot["executive_summary"])
        self.assertIn("active blocker", snapshot["recommended_focus"])
        self.assertIn("Run blocker review", snapshot["next_cadence_focus"])
        self.assertIn("forecast_gated", snapshot["capability_gaps"])
        self.assertIn("active_blockers", snapshot["capability_gaps"])
        self.assertEqual(snapshot["risk_driver_count"], 1)

    def test_live_evaluation_readiness_emits_quality_rates(self) -> None:
        review_queue = []
        for idx, truth, actionability in [
            (1, "true_positive", "actionable"),
            (2, "true_positive", "actionable"),
            (3, "partial", "needs_owner"),
            (4, "false_positive", "not_actionable"),
        ]:
            review_queue.append(triage_row(f"insight:{idx}", "blocker_candidate"))
            row = label_row(100 + idx, f"insight:{idx}", "blocker_candidate")
            row["truth_label"] = truth
            row["actionability_label"] = actionability
            review_queue.append(row)

        readiness = brief.build_live_evaluation_readiness(pd.DataFrame(review_queue))
        metrics = {row.metric: row.value for row in readiness.itertuples(index=False)}

        self.assertEqual(metrics["precision_rate"], "0.5")
        self.assertEqual(metrics["useful_signal_rate"], "0.75")
        self.assertEqual(metrics["actionability_rate"], "0.75")
        self.assertEqual(metrics["false_positive_rate"], "0.25")
        self.assertEqual(metrics["measurement_coverage_rate"], "1")

    def test_live_evaluation_readiness_includes_per_kind_rates_for_persistence(self) -> None:
        review_queue = []
        for idx, truth, actionability in [
            (1, "true_positive", "actionable"),
            (2, "partial", "needs_owner"),
            (3, "false_positive", "not_actionable"),
        ]:
            review_queue.append(triage_row(f"insight:blocker:{idx}", "blocker_candidate"))
            row = label_row(200 + idx, f"insight:blocker:{idx}", "blocker_candidate")
            row["truth_label"] = truth
            row["actionability_label"] = actionability
            review_queue.append(row)

        readiness = brief.build_live_evaluation_readiness(pd.DataFrame(review_queue))
        metrics = {row.metric: row.value for row in readiness.itertuples(index=False)}

        self.assertEqual(metrics["measurement_labels_blocker_candidate"], "3")
        self.assertEqual(metrics["open_review_requests_blocker_candidate"], "1")
        self.assertEqual(metrics["true_positive_blocker_candidate"], "1")
        self.assertEqual(metrics["partial_blocker_candidate"], "1")
        self.assertEqual(metrics["false_positive_blocker_candidate"], "1")
        self.assertEqual(metrics["precision_rate_blocker_candidate"], "0.3333")
        self.assertEqual(metrics["useful_signal_rate_blocker_candidate"], "0.6667")
        self.assertEqual(metrics["actionability_rate_blocker_candidate"], "0.6667")

    def test_work_program_quality_gate_builder_uses_rates_and_operating_facts(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "evaluation_label_row_count", "value": "12"},
                {"metric": "open_review_request_count", "value": "3"},
                {"metric": "ready_to_measure_precision", "value": "true"},
                {"metric": "ready_to_measure_actionability", "value": "true"},
                {"metric": "precision_rate", "value": "0.8"},
                {"metric": "useful_signal_rate", "value": "0.9"},
                {"metric": "actionability_rate", "value": "0.6"},
                {"metric": "review_requests_status_summary", "value": "12"},
                {"metric": "measurement_required_status_summary", "value": "10"},
                {"metric": "measurement_labels_status_summary", "value": "12"},
                {"metric": "open_review_requests_status_summary", "value": "3"},
                {"metric": "truth_labeled_status_summary", "value": "10"},
                {"metric": "actionability_labeled_status_summary", "value": "10"},
                {"metric": "true_positive_status_summary", "value": "8"},
                {"metric": "partial_status_summary", "value": "1"},
                {"metric": "false_positive_status_summary", "value": "2"},
                {"metric": "actionable_status_summary", "value": "6"},
                {"metric": "needs_owner_status_summary", "value": "0"},
            ]
        )
        forecast_summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false"},
                {"metric": "merged_pr_count", "value": "50"},
                {"metric": "backtest_best_model", "value": "median_cycle_baseline"},
                {"metric": "backtest_median_mae_days", "value": "6.53"},
                {"metric": "backtest_heuristic_mae_days", "value": "11.44"},
                {"metric": "backtest_hist_gradient_boosting_mae_days", "value": "6.15"},
                {"metric": "backtest_random_forest_mae_days", "value": "8.83"},
                {"metric": "eta_primary_blocker", "value": "kfold_model_does_not_beat_baseline"},
                {"metric": "eta_kfold_random_forest_improvement_pct", "value": "-35.22"},
                {"metric": "eta_chronological_random_forest_improvement_pct", "value": "7.70"},
                {"metric": "eta_temporal_snapshot_state", "value": "as_of_feature_snapshot_series_missing"},
                {"metric": "eta_next_evidence_needed", "value": "collect_repeated_as_of_pr_snapshots_and_closed_outcomes"},
            ]
        )
        facts = {
            "product_action_insight_kinds": ["status_summary"],
            "source_coverage_limited_count": 2,
            "auth_limited_observation_count": 1,
            "auth_limited_product_decision_count": 1,
            "generated_claim_limited_count": 1,
            "generated_claim_product_decision_count": 1,
            "overloaded_owner_count": 1,
            "unassigned_action_count": 0,
            "active_blocker_count": 0,
            "active_blocker_impact_count": 0,
        }

        gates = brief.build_work_program_quality_gates(readiness, forecast_summary, facts)
        by_key = {row["key"]: row for row in gates}

        self.assertEqual(len(gates), 13)
        self.assertTrue(by_key["forecast_readiness"]["blocking"])
        self.assertIn("primary blocker kfold_model_does_not_beat_baseline", by_key["forecast_readiness"]["detail"])
        self.assertIn("histogram gradient boosting MAE 6.15d", by_key["forecast_readiness"]["detail"])
        self.assertIn("as_of_feature_snapshot_series_missing", by_key["forecast_readiness"]["detail"])
        self.assertEqual(by_key["measurement_precision"]["gate_state"], "passed")
        self.assertFalse(by_key["measurement_precision"]["blocking"])
        self.assertEqual(by_key["measurement_actionability"]["gate_state"], "gated")
        self.assertIn("below product-action threshold", by_key["measurement_actionability"]["detail"])
        self.assertEqual(by_key["global_insight_precision"]["gate_state"], "passed")
        self.assertFalse(by_key["global_insight_precision"]["blocking"])
        self.assertEqual(by_key["global_insight_actionability"]["gate_state"], "passed")
        self.assertFalse(by_key["global_insight_actionability"]["blocking"])
        self.assertTrue(by_key["source_coverage"]["blocking"])
        self.assertTrue(by_key["source_authentication"]["blocking"])
        self.assertTrue(by_key["claim_provenance"]["blocking"])
        self.assertIn("anonymous/public source observation", by_key["source_authentication"]["detail"])
        self.assertIn("generated or derived claim evidence", by_key["claim_provenance"]["detail"])
        self.assertTrue(by_key["owner_load"]["blocking"])
        self.assertFalse(by_key["blocker_clearance"]["blocking"])
        self.assertEqual(by_key["validation_backlog"]["gate_state"], "passed")
        self.assertFalse(by_key["validation_backlog"]["blocking"])
        self.assertEqual(by_key["dependency_pressure"]["gate_state"], "passed")
        self.assertFalse(by_key["dependency_pressure"]["blocking"])
        self.assertEqual(by_key["product_decision"]["gate_state"], "passed")
        self.assertFalse(by_key["product_decision"]["blocking"])

    def test_product_action_quality_does_not_fallback_to_context_only_labels(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "evaluation_label_row_count", "value": "10"},
                {"metric": "open_review_request_count", "value": "0"},
                {"metric": "ready_to_measure_precision", "value": "true"},
                {"metric": "ready_to_measure_actionability", "value": "true"},
                {"metric": "precision_rate", "value": "1.0"},
                {"metric": "useful_signal_rate", "value": "1.0"},
                {"metric": "actionability_rate", "value": "1.0"},
                {"metric": "review_requests_dependency_cluster", "value": "10"},
                {"metric": "measurement_required_dependency_cluster", "value": "10"},
                {"metric": "measurement_labels_dependency_cluster", "value": "10"},
                {"metric": "truth_labeled_dependency_cluster", "value": "10"},
                {"metric": "actionability_labeled_dependency_cluster", "value": "10"},
                {"metric": "true_positive_dependency_cluster", "value": "10"},
                {"metric": "partial_dependency_cluster", "value": "0"},
                {"metric": "actionable_dependency_cluster", "value": "0"},
                {"metric": "needs_owner_dependency_cluster", "value": "10"},
            ]
        )
        forecast_summary = pd.DataFrame([{"metric": "eta_forecast_ready", "value": "true"}])
        facts = {
            "product_action_insight_kinds": ["dependency_cluster"],
            "source_coverage_limited_count": 0,
            "auth_limited_observation_count": 0,
            "generated_claim_limited_count": 0,
            "overloaded_owner_count": 0,
            "unassigned_action_count": 0,
            "active_blocker_count": 0,
            "active_blocker_impact_count": 0,
        }

        gates = brief.build_work_program_quality_gates(readiness, forecast_summary, facts)
        by_key = {row["key"]: row for row in gates}

        self.assertEqual(by_key["measurement_precision"]["gate_state"], "gated")
        self.assertIn("no product-action insight kinds", by_key["measurement_precision"]["detail"])
        self.assertEqual(by_key["measurement_actionability"]["gate_state"], "gated")
        self.assertIn("no product-action insight kinds", by_key["measurement_actionability"]["detail"])
        self.assertEqual(by_key["global_insight_precision"]["gate_state"], "passed")
        self.assertEqual(by_key["global_insight_actionability"]["gate_state"], "passed")
        self.assertIn("validation coverage measurement", by_key["global_insight_precision"]["detail"])
        self.assertIn("product-action readiness", by_key["global_insight_actionability"]["detail"])

        functions = brief.build_work_program_tpm_function_readiness(readiness, forecast_summary, facts)
        function_by_key = {row["function_key"]: row for row in functions}
        insight_quality = function_by_key["insight_quality"]
        self.assertEqual(insight_quality["readiness_state"], "assisted")
        self.assertEqual(insight_quality["automation_state"], "validation_only")
        self.assertTrue(insight_quality["human_required"])
        self.assertEqual(insight_quality["blocking_gate_keys"], ["measurement_precision", "measurement_actionability"])
        self.assertIn("validation and routing only", insight_quality["recommended_action"])

        checks = brief.build_work_program_adversarial_checks(readiness, forecast_summary, facts)
        check_by_key = {row["key"]: row for row in checks}
        measurement_check = check_by_key["measurement_overclaim"]
        self.assertEqual(measurement_check["check_state"], "fail")
        self.assertEqual(measurement_check["title"], "Product-action insight quality overclaim risk")
        self.assertEqual(measurement_check["blocking_gate_keys"], ["measurement_precision", "measurement_actionability"])
        self.assertIn("no product-action insight kinds", measurement_check["detail"])

    def test_source_auth_and_claim_gates_watch_validation_only_limits(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "evaluation_label_row_count", "value": "12"},
                {"metric": "ready_to_measure_precision", "value": "true"},
                {"metric": "ready_to_measure_actionability", "value": "true"},
                {"metric": "precision_rate", "value": "0.9"},
                {"metric": "useful_signal_rate", "value": "0.9"},
                {"metric": "actionability_rate", "value": "0.8"},
            ]
        )
        forecast_summary = pd.DataFrame([{"metric": "eta_forecast_ready", "value": "true"}])
        facts = {
            "source_coverage_limited_count": 0,
            "auth_limited_observation_count": 6,
            "auth_limited_product_decision_count": 0,
            "generated_claim_limited_count": 11,
            "generated_claim_product_decision_count": 0,
            "overloaded_owner_count": 0,
            "unassigned_action_count": 0,
            "active_blocker_count": 0,
            "active_blocker_impact_count": 0,
        }

        gates = brief.build_work_program_quality_gates(readiness, forecast_summary, facts)
        by_key = {row["key"]: row for row in gates}

        self.assertEqual(by_key["source_authentication"]["gate_state"], "watch")
        self.assertFalse(by_key["source_authentication"]["blocking"])
        self.assertIn("6 validation or QA program items", by_key["source_authentication"]["detail"])
        self.assertIn("do not promote", by_key["source_authentication"]["recommended_action"])
        self.assertEqual(by_key["claim_provenance"]["gate_state"], "watch")
        self.assertFalse(by_key["claim_provenance"]["blocking"])
        self.assertIn("11 validation or QA program items", by_key["claim_provenance"]["detail"])
        self.assertIn("before promotion to product actions", by_key["claim_provenance"]["recommended_action"])

    def test_owner_load_gate_ignores_validation_only_unassigned_actions(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "evaluation_label_row_count", "value": "12"},
                {"metric": "ready_to_measure_precision", "value": "true"},
                {"metric": "ready_to_measure_actionability", "value": "true"},
                {"metric": "precision_rate", "value": "0.9"},
                {"metric": "useful_signal_rate", "value": "0.9"},
                {"metric": "actionability_rate", "value": "0.8"},
            ]
        )
        forecast_summary = pd.DataFrame([{"metric": "eta_forecast_ready", "value": "true"}])
        facts = {
            "source_coverage_limited_count": 0,
            "auth_limited_observation_count": 0,
            "generated_claim_limited_count": 0,
            "overloaded_owner_count": 0,
            "unassigned_action_count": 0,
            "unassigned_total_action_count": 15,
            "active_blocker_count": 0,
            "active_blocker_impact_count": 0,
        }

        gates = brief.build_work_program_quality_gates(readiness, forecast_summary, facts)
        by_key = {row["key"]: row for row in gates}

        self.assertEqual(by_key["owner_load"]["gate_state"], "passed")
        self.assertFalse(by_key["owner_load"]["blocking"])
        self.assertIn("unassigned product actions", by_key["owner_load"]["detail"])
        self.assertEqual(by_key["validation_backlog"]["gate_state"], "watch")
        self.assertFalse(by_key["validation_backlog"]["blocking"])
        self.assertIn("15 unassigned validation or QA actions", by_key["validation_backlog"]["detail"])

    def test_owner_load_status_does_not_overload_validation_only_rows(self) -> None:
        validation_only = pd.Series(
            {
                "action_count": 4,
                "product_action_count": 0,
                "validation_lead_count": 4,
                "critical_or_high_count": 0,
                "max_priority_score": 100,
                "coverage_limited_count": 0,
                "anonymous_observation_count": 0,
                "needs_human_review_count": 4,
            }
        )
        product_pressure = pd.Series(
            {
                "action_count": 2,
                "product_action_count": 1,
                "validation_lead_count": 1,
                "critical_or_high_count": 1,
                "max_priority_score": 93,
                "coverage_limited_count": 0,
                "anonymous_observation_count": 0,
                "needs_human_review_count": 1,
            }
        )

        self.assertEqual(brief.owner_load_status(validation_only), "watch")
        self.assertEqual(brief.owner_load_status(product_pressure), "overloaded")

    def test_quality_gates_scope_measurement_to_product_action_kinds(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "evaluation_label_row_count", "value": "3"},
                {"metric": "open_review_request_count", "value": "10"},
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "false"},
                {"metric": "precision_rate", "value": "0.2"},
                {"metric": "useful_signal_rate", "value": "0.2"},
                {"metric": "actionability_rate", "value": "0.2"},
                {"metric": "review_requests_status_summary", "value": "3"},
                {"metric": "measurement_required_status_summary", "value": "3"},
                {"metric": "measurement_labels_status_summary", "value": "3"},
                {"metric": "open_review_requests_status_summary", "value": "0"},
                {"metric": "truth_labeled_status_summary", "value": "3"},
                {"metric": "actionability_labeled_status_summary", "value": "3"},
                {"metric": "true_positive_status_summary", "value": "3"},
                {"metric": "partial_status_summary", "value": "0"},
                {"metric": "false_positive_status_summary", "value": "0"},
                {"metric": "actionable_status_summary", "value": "0"},
                {"metric": "needs_owner_status_summary", "value": "3"},
                {"metric": "review_requests_blocker_candidate", "value": "10"},
                {"metric": "measurement_required_blocker_candidate", "value": "10"},
                {"metric": "measurement_labels_blocker_candidate", "value": "0"},
                {"metric": "truth_labeled_blocker_candidate", "value": "0"},
                {"metric": "actionability_labeled_blocker_candidate", "value": "0"},
            ]
        )
        forecast_summary = pd.DataFrame([{"metric": "eta_forecast_ready", "value": "false"}])
        facts = {
            "product_action_insight_kinds": ["status_summary"],
            "source_coverage_limited_count": 0,
            "auth_limited_observation_count": 0,
            "generated_claim_limited_count": 0,
            "overloaded_owner_count": 0,
            "unassigned_action_count": 0,
            "active_blocker_count": 0,
            "active_blocker_impact_count": 0,
        }

        gates = brief.build_work_program_quality_gates(readiness, forecast_summary, facts)
        by_key = {row["key"]: row for row in gates}

        self.assertEqual(by_key["measurement_precision"]["gate_state"], "passed")
        self.assertIn("status_summary", by_key["measurement_precision"]["detail"])
        self.assertEqual(by_key["measurement_actionability"]["gate_state"], "passed")

    def test_lifecycle_as_of_forecast_evaluation_kind_stays_gated(self) -> None:
        row = pd.Series(
            {
                "evaluation": "lifecycle_as_of_baseline",
                "model": "age_bucket_median_remaining",
                "ready_for_eta": "false",
                "note": "Lifecycle baseline only.",
            }
        )

        self.assertEqual(brief.forecast_evaluation_kind("lifecycle_as_of_baseline"), "lifecycle_as_of_baseline")
        self.assertEqual(brief.forecast_row_readiness_state("lifecycle_as_of_baseline", row), "gated")
        self.assertIn("Lifecycle baseline only", brief.forecast_row_readiness_reason("lifecycle_as_of_baseline", row))

    def test_survival_forecast_evaluation_kind_stays_gated(self) -> None:
        row = pd.Series(
            {
                "evaluation": "survival_time_to_merge",
                "model": "km_restricted_mean_remaining",
                "ready_for_eta": "false",
                "note": "Censored survival baseline only.",
            }
        )

        self.assertEqual(brief.forecast_evaluation_kind("survival_time_to_merge"), "survival_time_to_merge")
        self.assertEqual(brief.forecast_row_readiness_state("survival_time_to_merge", row), "gated")
        self.assertIn("Censored survival baseline only", brief.forecast_row_readiness_reason("survival_time_to_merge", row))

    def test_source_event_as_of_forecast_evaluation_kind_stays_gated(self) -> None:
        row = pd.Series(
            {
                "evaluation": "source_event_as_of_kfold",
                "model": "random_forest_regressor",
                "ready_for_eta": "false",
                "note": "Source-event replay as-of feature backtest.",
            }
        )

        self.assertEqual(brief.forecast_evaluation_kind("source_event_as_of_kfold"), "source_event_as_of_kfold")
        self.assertEqual(
            brief.forecast_evaluation_kind("source_event_as_of_chronological_holdout"),
            "source_event_as_of_chronological_holdout",
        )
        self.assertEqual(brief.forecast_row_readiness_state("source_event_as_of_kfold", row), "gated")
        self.assertIn("Source-event replay", brief.forecast_row_readiness_reason("source_event_as_of_kfold", row))

    def test_product_action_insight_kinds_use_work_action_edges(self) -> None:
        conn = sqlite3.connect(":memory:")
        conn.executescript(
            """
            create table work_actions (
              id integer primary key,
              source_system text,
              source_instance text,
              external_kind text,
              decision_state text,
              action_state text
            );
            create table work_insights (
              id integer primary key,
              source_system text,
              source_instance text,
              insight_kind text
            );
            create table work_action_source_insights (
              work_action_id integer,
              work_insight_id integer
            );
            insert into work_actions values
              (1, 'cubicle_analytics', 'fixture-source', 'tpm_work_action', 'product_action', 'open'),
              (2, 'cubicle_analytics', 'fixture-source', 'tpm_work_action', 'validation_lead', 'open');
            insert into work_insights values
              (10, 'cubicle_analytics', 'fixture-source', 'status_summary'),
              (20, 'cubicle_analytics', 'fixture-source', 'blocker_candidate');
            insert into work_action_source_insights values
              (1, 10),
              (2, 20);
            """
        )

        kinds = brief.ontology_work_action_insight_kinds(
            conn,
            "fixture-source",
            "wa.decision_state = 'product_action' and wa.action_state = 'open'",
        )

        self.assertEqual(kinds, ["status_summary"])

    def test_source_coverage_classifier_splits_auth_and_generated_limits(self) -> None:
        anonymous = {"source_coverage_state": "anonymous_success:public_api_current_observation", "freshness_state": "fresh"}
        generated = {"source_coverage_state": "generated:direct_identity_same_window_overlap", "freshness_state": "partial"}
        missing = {"source_coverage_state": "not_observed", "freshness_state": "partial"}

        self.assertFalse(brief.ontology_program_item_coverage_limited(anonymous))
        self.assertTrue(brief.ontology_program_item_auth_limited(anonymous))
        self.assertFalse(brief.ontology_program_item_generated_claim_limited(anonymous))

        self.assertFalse(brief.ontology_program_item_coverage_limited(generated))
        self.assertFalse(brief.ontology_program_item_auth_limited(generated))
        self.assertTrue(brief.ontology_program_item_generated_claim_limited(generated))

        self.assertTrue(brief.ontology_program_item_coverage_limited(missing))
        self.assertFalse(brief.ontology_program_item_auth_limited(missing))
        self.assertFalse(brief.ontology_program_item_generated_claim_limited(missing))
        self.assertTrue(brief.ontology_program_item_product_decision_open({"decision_state": "closeout_review"}))
        self.assertTrue(brief.ontology_program_item_product_decision_open({"program_status": "closed_pending_review"}))
        self.assertFalse(brief.ontology_program_item_product_decision_open({"decision_state": "validation_lead", "program_status": "validate_signal"}))

    def test_work_program_automation_readiness_builder_summarizes_replacement_boundary(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "true"},
                {"metric": "precision_rate", "value": "0.4"},
                {"metric": "useful_signal_rate", "value": "0.6"},
                {"metric": "actionability_rate", "value": "0.8"},
                {"metric": "evaluation_label_row_count", "value": "10"},
                {"metric": "open_review_request_count", "value": "4"},
            ]
        )
        forecast_summary = pd.DataFrame([{"metric": "eta_forecast_ready", "value": "false"}])
        facts = {
            "standup_section_count": 3,
            "active_blocker_count": 2,
            "active_blocker_impact_count": 1,
            "needs_action_dependency_count": 4,
            "source_coverage_limited_count": 2,
            "auth_limited_observation_count": 1,
            "auth_limited_product_decision_count": 1,
            "generated_claim_limited_count": 1,
            "generated_claim_product_decision_count": 1,
            "overloaded_owner_count": 1,
            "unassigned_action_count": 1,
            "product_action_count": 3,
            "needs_decision_count": 1,
            "program_item_evidence_refs": ["pr https://example.test/pull/1"],
        }
        gates = brief.build_work_program_quality_gates(readiness, forecast_summary, facts)
        evidence_needs = brief.build_work_program_evidence_needs(readiness, forecast_summary, facts)
        functions = brief.build_work_program_tpm_function_readiness(readiness, forecast_summary, facts)

        snapshot = brief.build_work_program_automation_readiness(readiness, forecast_summary, facts, gates, evidence_needs, functions)

        self.assertEqual(snapshot["readiness_state"], "blocked")
        self.assertEqual(snapshot["readiness_score"], 0.0)
        self.assertFalse(snapshot["autonomous_action_ready"])
        self.assertTrue(snapshot["human_review_required"])
        self.assertIn("agenda_summarization", snapshot["safe_automation_areas"])
        self.assertIn("source_citation", snapshot["safe_automation_areas"])
        self.assertIn("eta_commitments", snapshot["human_required_areas"])
        self.assertIn("measurement_claims", snapshot["human_required_areas"])
        self.assertIn("source_authentication", snapshot["human_required_areas"])
        self.assertIn("claim_provenance", snapshot["human_required_areas"])
        self.assertIn("product_decisions", snapshot["human_required_areas"])
        self.assertIn("measurement_precision", snapshot["blocking_gate_keys"])
        self.assertIn("source_authentication", snapshot["blocking_gate_keys"])
        self.assertIn("claim_provenance", snapshot["blocking_gate_keys"])
        self.assertIn("authenticated re-observation", " ".join(snapshot["required_evidence"]))
        self.assertIn("generated claims", " ".join(snapshot["required_evidence"]))
        self.assertEqual(snapshot["quality_gate_count"], len(gates))
        self.assertEqual(snapshot["evidence_need_count"], len(evidence_needs))
        self.assertEqual(snapshot["tpm_function_count"], len(functions))

    def test_work_program_tpm_function_readiness_builder_surfaces_human_gates(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "evaluation_label_row_count", "value": "4"},
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "true"},
            ]
        )
        forecast_summary = pd.DataFrame([{"metric": "eta_forecast_ready", "value": "false"}])
        facts = {
            "total_count": 22,
            "standup_section_count": 12,
            "source_coverage_limited_count": 5,
            "overloaded_owner_count": 1,
            "unassigned_action_count": 2,
            "owner_load_action_count": 7,
            "active_blocker_count": 1,
            "active_blocker_impact_count": 1,
            "needs_action_dependency_count": 2,
            "product_action_count": 3,
            "needs_decision_count": 1,
        }

        rows = brief.build_work_program_tpm_function_readiness(readiness, forecast_summary, facts)
        by_key = {row["function_key"]: row for row in rows}

        self.assertEqual(len(rows), 7)
        self.assertEqual(by_key["operating_brief"]["readiness_state"], "automatable")
        self.assertFalse(by_key["operating_brief"]["human_required"])
        self.assertEqual(by_key["blocker_management"]["readiness_state"], "supervised")
        self.assertEqual(by_key["blocker_management"]["supporting_signal_count"], 4)
        self.assertEqual(by_key["blocker_management"]["blocking_gate_keys"], ["blocker_clearance"])
        self.assertEqual(by_key["forecast_triage"]["automation_state"], "risk_triage_only")
        self.assertEqual(by_key["execution_capacity"]["readiness_state"], "blocked")
        self.assertEqual(by_key["execution_capacity"]["blocking_gate_keys"], ["owner_load"])
        self.assertEqual(by_key["source_coverage"]["readiness_state"], "blocked")
        self.assertEqual(by_key["insight_quality"]["blocking_gate_keys"], ["global_insight_precision"])
        self.assertEqual(by_key["product_decisions"]["readiness_state"], "supervised")

    def test_insight_quality_global_readiness_uses_distinct_blocking_keys(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "evaluation_label_row_count", "value": "12"},
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "false"},
                {"metric": "precision_rate", "value": "1"},
                {"metric": "actionability_rate", "value": "1"},
                {"metric": "review_requests_status_summary", "value": "3"},
                {"metric": "measurement_required_status_summary", "value": "3"},
                {"metric": "measurement_labels_status_summary", "value": "3"},
                {"metric": "truth_labeled_status_summary", "value": "3"},
                {"metric": "actionability_labeled_status_summary", "value": "3"},
                {"metric": "true_positive_status_summary", "value": "3"},
                {"metric": "needs_owner_status_summary", "value": "3"},
            ]
        )
        forecast_summary = pd.DataFrame([{"metric": "eta_forecast_ready", "value": "true"}])
        facts = {
            "total_count": 3,
            "standup_section_count": 1,
            "product_action_insight_kinds": ["status_summary"],
        }

        gates = brief.build_work_program_quality_gates(readiness, forecast_summary, facts)
        functions = brief.build_work_program_tpm_function_readiness(readiness, forecast_summary, facts)
        gate_by_key = {row["key"]: row for row in gates}
        function_by_key = {row["function_key"]: row for row in functions}

        self.assertEqual(gate_by_key["measurement_precision"]["gate_state"], "passed")
        self.assertEqual(gate_by_key["measurement_actionability"]["gate_state"], "passed")
        self.assertEqual(gate_by_key["global_insight_precision"]["gate_state"], "gated")
        self.assertTrue(gate_by_key["global_insight_precision"]["blocking"])
        self.assertEqual(gate_by_key["global_insight_actionability"]["gate_state"], "gated")
        self.assertTrue(gate_by_key["global_insight_actionability"]["blocking"])
        self.assertEqual(function_by_key["insight_quality"]["readiness_state"], "automatable")
        self.assertEqual(function_by_key["insight_quality"]["automation_state"], "measurement_ready")
        self.assertFalse(function_by_key["insight_quality"]["human_required"])
        self.assertEqual(function_by_key["insight_quality"]["blocking_gate_keys"], [])
        self.assertIn(
            "Product-action insight precision and actionability",
            function_by_key["insight_quality"]["detail"],
        )

    def test_work_program_tpm_function_readiness_does_not_attach_passed_blocker_gate_to_dependencies(self) -> None:
        readiness = pd.DataFrame(
            [
                {"metric": "evaluation_label_row_count", "value": "4"},
                {"metric": "ready_to_measure_precision", "value": "true"},
                {"metric": "ready_to_measure_actionability", "value": "true"},
            ]
        )
        forecast_summary = pd.DataFrame([{"metric": "eta_forecast_ready", "value": "true"}])
        facts = {
            "total_count": 5,
            "standup_section_count": 1,
            "active_blocker_count": 0,
            "active_blocker_impact_count": 0,
            "needs_action_dependency_count": 3,
        }

        rows = brief.build_work_program_tpm_function_readiness(readiness, forecast_summary, facts)
        by_key = {row["function_key"]: row for row in rows}

        self.assertEqual(by_key["blocker_management"]["readiness_state"], "supervised")
        self.assertEqual(by_key["blocker_management"]["supporting_signal_count"], 3)
        self.assertEqual(by_key["blocker_management"]["blocking_gate_keys"], [])

    def test_review_measurement_eligibility_is_persisted_and_read_from_ontology(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_review_tables(conn)
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, subject_kind, subject_key,
              score, confidence, producer_state, source_system, source_instance,
              external_kind
            ) values
              (1, 'insight:blocker:gold', 'blocker_candidate', 'high', 'pull_request', 'repo/example#1', 90, 0.9, 'current', 'cubicle_analytics', 'fixture-source', 'tpm_insight'),
              (2, 'insight:blocker:smoke', 'blocker_candidate', 'medium', 'pull_request', 'repo/example#2', 70, 0.8, 'current', 'cubicle_analytics', 'fixture-source', 'tpm_insight'),
              (3, 'insight:blocker:adversarial', 'blocker_candidate', 'medium', 'pull_request', 'repo/example#3', 69, 0.8, 'current', 'cubicle_analytics', 'fixture-source', 'tpm_insight'),
              (4, 'insight:forecast:source-oracle', 'forecast_risk', 'high', 'pull_request', 'repo/example#4', 95, 0.8, 'current', 'cubicle_analytics', 'fixture-source', 'tpm_insight')
            """
        )
        conn.execute(
            """
            insert into work_insight_reviews (
              id, key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, source_system, source_instance,
              external_kind
            ) values
              (10, 'review:gold', 1, 'evaluation_label', 'accepted', 'true_positive', 'actionable', 'agent_gold', 'gold', 0, 'imported', 'judge', 'cubicle_evaluation', 'fixture-source', 'tpm_review_label'),
              (11, 'review:smoke', 2, 'evaluation_label', 'accepted', 'true_positive', 'actionable', 'agent_smoke', 'smoke', 1, 'imported', 'smoke', 'cubicle_evaluation', 'fixture-source', 'tpm_review_label'),
              (12, 'review:adversarial', 3, 'evaluation_label', 'dismissed', 'false_positive', 'not_actionable', 'agent_adversarial', 'adversarial', 1, 'imported', 'adversarial', 'cubicle_evaluation', 'fixture-source', 'tpm_review_label'),
              (13, 'review:source-oracle', 4, 'evaluation_label', 'accepted', 'true_positive', 'needs_owner', 'source_oracle_seed', 'candidate', 0, 'imported', 'source_oracle', 'cubicle_evaluation', 'fixture-source', 'tpm_review_label'),
              (14, 'review:stale-source', 4, 'evaluation_label', 'accepted', 'true_positive', 'actionable', 'agent_gold', 'gold', 1, 'imported', 'judge', 'cubicle_evaluation', 'stale-source', 'tpm_review_label'),
              (15, 'review:wrong-kind', 4, 'evaluation_label', 'accepted', 'true_positive', 'actionable', 'agent_gold', 'gold', 1, 'imported', 'judge', 'cubicle_evaluation', 'fixture-source', 'wrong_kind')
            """
        )

        brief.backfill_review_measurement_eligibility(conn, "fixture-source", set())
        stored = dict(conn.execute("select key, measurement_eligible from work_insight_reviews").fetchall())
        self.assertEqual(stored["review:gold"], 0)
        self.assertEqual(stored["review:smoke"], 0)
        self.assertEqual(stored["review:adversarial"], 0)
        self.assertEqual(
            conn.execute("select measurement_eligible from work_insight_reviews where key = 'review:source-oracle'").fetchone()[0],
            0,
        )
        self.assertEqual(stored["review:stale-source"], 1)
        self.assertEqual(stored["review:wrong-kind"], 1)
        rows = brief.read_review_queue_from_ontology(conn, "fixture-source", set())
        self.assertEqual(set(rows["review_key"].tolist()), {"review:gold", "review:smoke", "review:adversarial", "review:source-oracle"})
        by_key = {row.insight_key: row.measurement_eligible for row in rows.itertuples(index=False)}
        self.assertEqual(by_key["insight:blocker:gold"], "false")
        self.assertEqual(by_key["insight:blocker:smoke"], "false")
        self.assertEqual(by_key["insight:blocker:adversarial"], "false")
        self.assertEqual(by_key["insight:forecast:source-oracle"], "false")

        brief.backfill_review_measurement_eligibility(conn, "fixture-source", {"source_oracle_seed"})
        self.assertEqual(
            conn.execute("select measurement_eligible from work_insight_reviews where key = 'review:source-oracle'").fetchone()[0],
            0,
        )
        rows = brief.read_review_queue_from_ontology(conn, "fixture-source", {"source_oracle_seed"})
        by_key = {row.insight_key: row.measurement_eligible for row in rows.itertuples(index=False)}
        self.assertEqual(by_key["insight:forecast:source-oracle"], "false")

        brief.backfill_review_measurement_eligibility(conn, "fixture-source", {"agent_smoke", "agent_adversarial"})
        rows = brief.read_review_queue_from_ontology(conn, "fixture-source", {"agent_smoke", "agent_adversarial"})
        by_key = {row.insight_key: row.measurement_eligible for row in rows.itertuples(index=False)}
        self.assertEqual(by_key["insight:blocker:gold"], "false")
        self.assertEqual(by_key["insight:blocker:smoke"], "false")
        self.assertEqual(by_key["insight:blocker:adversarial"], "false")
        self.assertEqual(by_key["insight:forecast:source-oracle"], "false")

    def test_measurement_label_targets_ignore_raw_flagged_non_gold_reviews(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_review_tables(conn)
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, subject_kind, subject_key, title, score,
              source_url, producer_state, source_system, source_instance,
              external_kind, rank_score, updated_at
            ) values
              (1, 'insight:candidate', 'blocker_candidate', 'pull_request',
               'repo/example#1', 'Candidate-only blocker', 90,
               'https://github.com/repo/example/pull/1', 'current',
               'cubicle_analytics', 'fixture-source', 'tpm_insight', 90,
               '2026-06-21T01:00:00+00:00'),
              (2, 'insight:gold', 'blocker_candidate', 'pull_request',
               'repo/example#2', 'Gold blocker', 91,
               'https://github.com/repo/example/pull/2', 'current',
               'cubicle_analytics', 'fixture-source', 'tpm_insight', 91,
               '2026-06-21T01:01:00+00:00')
            """
        )
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, source_system, source_instance,
              external_kind
            ) values
              ('review:candidate', 1, 'evaluation_label', 'accepted',
               'true_positive', 'needs_owner', 'source_oracle_seed',
               'candidate', 1, 'imported', 'source_oracle',
               'cubicle_evaluation', 'fixture-source', 'tpm_review_label'),
              ('review:gold', 2, 'evaluation_label', 'accepted',
               'true_positive', 'needs_owner', 'agent_gold',
               'gold', 1, 'imported', 'judge',
               'cubicle_evaluation', 'fixture-source', 'tpm_review_label')
            """
        )

        targets = brief.ontology_measurement_label_targets(conn, "fixture-source", limit=10)

        self.assertEqual([row["insight_key"] for row in targets], ["insight:candidate"])

    def test_work_action_observation_materialization_replaces_current_observation(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_action_tables(conn)
        action_items = pd.DataFrame(
            [
                {
                    "action_key": "tpm-action:test:forecast-quality",
                    "action_type": "model_quality_review",
                    "decision_state": "model_or_rule_qa",
                    "decision_gate_reason": "model quality gate is about readiness, not product escalation",
                    "subject_kind": "unknown",
                    "subject_key": "flink-pr-cycle-forecast",
                    "urgency": "medium",
                    "priority_score": 72,
                    "confidence": 0.8,
                    "source_observation_status": "generated_evidence",
                    "source_coverage_kind": "forecast_backtest",
                    "source_url": "https://example.test/forecast",
                    "evidence_ref": "forecast_backtest baseline-vs-model https://example.test/forecast",
                }
            ]
        )
        first_observations = brief.build_work_action_observations(action_items, "2026-06-21T01:00:00+00:00")
        brief.persist_work_actions_to_ontology(
            conn,
            "fixture-source",
            action_items,
            first_observations,
            "2026-06-21T01:00:00+00:00",
        )
        second_observations = brief.build_work_action_observations(action_items, "2026-06-21T02:00:00+00:00")
        brief.persist_work_actions_to_ontology(
            conn,
            "fixture-source",
            action_items,
            second_observations,
            "2026-06-21T02:00:00+00:00",
        )

        rows = conn.execute(
            "select key, observation_kind, observed_at, source_coverage_state from work_action_observations order by id"
        ).fetchall()
        self.assertEqual(
            rows,
            [
                (
                    f"work-action-observation:cubicle-analytics:fixture-source:{brief.stable_digest(['tpm-action:test:forecast-quality', 'model_or_rule_qa'])}",
                    "model_or_rule_qa",
                    "2026-06-21T02:00:00+00:00",
                    "generated:forecast_backtest",
                )
            ],
        )
        action_evidence = conn.execute(
            """
            select e.locator_kind, e.locator, e.source_url
              from work_actions wa
              join evidences e on e.id = wa.latest_evidence_id
             where wa.key = 'tpm-action:test:forecast-quality'
            """
        ).fetchone()
        self.assertEqual(
            action_evidence,
            ("forecast_backtest", "baseline-vs-model", "https://example.test/forecast"),
        )
        observation_evidence = conn.execute(
            """
            select e.claim_target_kind, e.locator_kind, e.locator
              from work_action_observations wao
              join evidences e on e.id = wao.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(
            observation_evidence,
            ("work_action_observation", "forecast_backtest", "baseline-vs-model"),
        )

    def test_manually_closed_generated_work_action_is_not_reopened_by_refresh(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_action_tables(conn)
        action_items = pd.DataFrame(
            [
                {
                    "action_key": "tpm-action:test:closeout",
                    "action_type": "verify_resolution",
                    "decision_state": "closeout_review",
                    "decision_gate_reason": "terminal transition requires closeout confirmation",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#72",
                    "source_link_insight_kinds": "state_transition",
                    "owner_hint": "github:owner",
                    "source_url": "https://github.com/repo/example/pull/72",
                    "evidence_ref": "state_transition tpm-transition:test",
                    "confidence": 0.95,
                    "priority_score": 65,
                }
            ]
        )
        brief.persist_work_actions_to_ontology(
            conn,
            "fixture-source",
            action_items,
            pd.DataFrame(),
            "2026-06-21T01:00:00+00:00",
        )
        conn.execute(
            """
            update work_actions
               set action_state = 'closed',
                   closed_at = '2026-06-21T02:00:00+00:00',
                   updated_at = '2026-06-21T02:00:00+00:00'
             where key = 'tpm-action:test:closeout'
            """
        )

        brief.persist_work_actions_to_ontology(
            conn,
            "fixture-source",
            action_items,
            pd.DataFrame(),
            "2026-06-21T03:00:00+00:00",
        )

        row = conn.execute(
            "select action_state, closed_at, updated_at from work_actions where key = 'tpm-action:test:closeout'"
        ).fetchone()
        self.assertEqual(row, ("closed", "2026-06-21T02:00:00+00:00", "2026-06-21T02:00:00+00:00"))

    def test_generator_closed_source_resolved_action_reopens_when_current_run_downgrades_it(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_action_tables(conn)
        source_resolved = pd.DataFrame(
            [
                {
                    "action_key": "tpm-action:test:closeout",
                    "action_type": "verify_resolution",
                    "decision_state": "source_resolved",
                    "decision_gate_reason": "authenticated source observed terminal state",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#72",
                    "source_link_insight_kinds": "state_transition",
                    "owner_hint": "github:owner",
                    "source_url": "https://github.com/repo/example/pull/72",
                    "evidence_ref": "state_transition tpm-transition:test",
                    "confidence": 0.95,
                    "priority_score": 20,
                }
            ]
        )
        closeout_review = source_resolved.copy()
        closeout_review.at[0, "decision_state"] = "closeout_review"
        closeout_review.at[0, "decision_gate_reason"] = "terminal transition requires closeout confirmation"
        closeout_review.at[0, "priority_score"] = 65

        brief.persist_work_actions_to_ontology(
            conn,
            "fixture-source",
            source_resolved,
            pd.DataFrame(),
            "2026-06-21T01:00:00+00:00",
        )
        brief.persist_work_actions_to_ontology(
            conn,
            "fixture-source",
            closeout_review,
            pd.DataFrame(),
            "2026-06-21T03:00:00+00:00",
        )

        row = conn.execute(
            "select action_state, decision_state, closed_at, updated_at from work_actions where key = 'tpm-action:test:closeout'"
        ).fetchone()
        self.assertEqual(row, ("open", "closeout_review", None, "2026-06-21T03:00:00+00:00"))

    def test_stale_closed_generated_work_action_is_superseded_when_current_run_drops_it(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_action_tables(conn)
        action_items = pd.DataFrame(
            [
                {
                    "action_key": "tpm-action:test:closeout",
                    "action_type": "verify_resolution",
                    "decision_state": "source_resolved",
                    "decision_gate_reason": "authenticated source observed terminal state",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#72",
                    "source_link_insight_kinds": "state_transition",
                    "owner_hint": "github:owner",
                    "source_url": "https://github.com/repo/example/pull/72",
                    "evidence_ref": "state_transition tpm-transition:test",
                    "confidence": 0.95,
                    "priority_score": 20,
                }
            ]
        )
        brief.persist_work_actions_to_ontology(
            conn,
            "fixture-source",
            action_items,
            pd.DataFrame(),
            "2026-06-21T01:00:00+00:00",
        )

        brief.persist_work_actions_to_ontology(
            conn,
            "fixture-source",
            pd.DataFrame(columns=action_items.columns),
            pd.DataFrame(),
            "2026-06-21T03:00:00+00:00",
        )

        row = conn.execute(
            "select action_state, decision_state, updated_at from work_actions where key = 'tpm-action:test:closeout'"
        ).fetchone()
        self.assertEqual(row, ("superseded", "source_resolved", "2026-06-21T03:00:00+00:00"))

    def test_product_backed_work_blocker_identity_is_stable_across_action_refresh(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_tables(conn)
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, subject_kind, subject_key, title,
              details, recommended_action, source_url, latest_evidence_id, score,
              confidence, rank_score, producer_state, source_system, source_instance,
              external_kind, updated_at
            ) values (
              1, 'work-insight:test:blocker', 'blocker_candidate', 'high',
              'pull_request', 'repo/example#9', 'Merge-blocking CI signal',
              'CI appears to be blocking merge.', 'Ask CI owner to confirm.',
              'https://github.com/repo/example/pull/9/checks', null, 91,
              0.86, 91, 'current', 'cubicle_analytics', 'fixture-source',
              'tpm_insight', '2026-06-21T01:00:00+00:00'
            )
            """
        )
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, reviewed_at, updated_at
            ) values
              ('review:imported', 1, 'evaluation_label', 'accepted', 'true_positive',
               'needs_owner', 'agent_gold_blockers', 'gold', 1, 'imported',
               'codex_agent_adjudication', '2026-06-21T01:05:00+00:00',
               '2026-06-21T01:10:00+00:00')
            """
        )

        first_action_id = insert_blocker_action(conn, "tpm-action:test:first")
        action_items = pd.DataFrame([blocker_action_item("tpm-action:test:first", "first action")])
        brief.persist_work_blockers_to_ontology(
            conn,
            "fixture-source",
            action_items,
            "2026-06-21T01:15:00+00:00",
        )
        first_blocker = conn.execute(
            "select key, external_id, work_action_id from work_blockers"
        ).fetchone()
        self.assertIsNotNone(first_blocker)

        second_action_id = insert_blocker_action(conn, "tpm-action:test:second")
        action_items = pd.DataFrame([blocker_action_item("tpm-action:test:second", "second action")])
        brief.persist_work_blockers_to_ontology(
            conn,
            "fixture-source",
            action_items,
            "2026-06-21T01:30:00+00:00",
        )

        rows = conn.execute(
            """
            select key, external_id, work_action_id, reviewer_kind, reviewer_key,
                   review_state, truth_label, actionability_label, measurement_eligible
              from work_blockers
            """
        ).fetchall()
        self.assertEqual(len(rows), 1)
        row = rows[0]
        self.assertEqual(row[0], first_blocker[0])
        self.assertEqual(row[1], first_blocker[1])
        self.assertEqual(row[2], second_action_id)
        self.assertNotEqual(first_action_id, second_action_id)
        self.assertEqual(row[3], "imported")
        self.assertEqual(row[4], "codex_agent_adjudication")
        self.assertEqual(row[5], "accepted")
        self.assertEqual(row[6], "true_positive")
        self.assertEqual(row[7], "needs_owner")
        self.assertEqual(row[8], 1)

    def test_validation_lead_blocker_candidate_does_not_materialize_work_blocker(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_tables(conn)
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, subject_kind, subject_key, title,
              details, recommended_action, source_url, latest_evidence_id, score,
              confidence, rank_score, producer_state, source_system, source_instance,
              external_kind, updated_at
            ) values (
              1, 'work-insight:test:blocker', 'blocker_candidate', 'high',
              'pull_request', 'repo/example#9', 'Potential blocker',
              'Keyword evidence says blocker.', 'Validate first.',
              'https://github.com/repo/example/pull/9', null, 91,
              0.86, 91, 'current', 'cubicle_analytics', 'fixture-source',
              'tpm_insight', '2026-06-21T01:00:00+00:00'
            )
            """
        )
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, reviewed_at, updated_at
            ) values (
              'review:adversarial', 1, 'evaluation_label', 'needs_more_data',
              'partial', 'needs_owner', 'adversarial', 'adversarial', 0,
              'imported', 'adversarial-review', '2026-06-21T01:05:00+00:00',
              '2026-06-21T01:05:00+00:00'
            )
            """
        )
        action_id = insert_blocker_action(
            conn,
            "tpm-action:test:validation",
            action_type="validate_signal",
            decision_state="validation_lead",
        )
        action_items = pd.DataFrame(
            [
                blocker_action_item(
                    "tpm-action:test:validation",
                    "validation action",
                    action_type="validate_signal",
                    decision_state="validation_lead",
                )
            ]
        )

        brief.persist_work_blockers_to_ontology(
            conn,
            "fixture-source",
            action_items,
            "2026-06-21T01:15:00+00:00",
        )

        self.assertGreater(action_id, 0)
        self.assertEqual(conn.execute("select count(*) from work_actions").fetchone()[0], 1)
        self.assertEqual(conn.execute("select count(*) from work_blockers").fetchone()[0], 0)

    def test_measurement_override_non_gold_true_positive_does_not_materialize_blocker_edges(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_impact_tables(conn)
        generated_at = "2026-06-21T01:15:00+00:00"
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, subject_kind, subject_key, title,
              details, recommended_action, source_url, latest_evidence_id, score,
              confidence, rank_score, producer_state, source_system, source_instance,
              external_kind, updated_at
            ) values (
              1, 'work-insight:test:blocker', 'blocker_candidate', 'high',
              'pull_request', 'repo/example#9', 'Potential blocker',
              'Keyword evidence says blocker.', 'Validate first.',
              'https://github.com/repo/example/pull/9', null, 91,
              0.86, 91, 'current', 'cubicle_analytics', 'fixture-source',
              'tpm_insight', '2026-06-21T01:00:00+00:00'
            )
            """
        )
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, source_system, source_instance,
              external_kind, reviewed_at, updated_at
            ) values (
              'review:adversarial-partial', 1, 'evaluation_label',
              'accepted', 'true_positive', 'needs_owner', 'agent_adversarial',
              'adversarial', 0, 'imported', 'adversarial-review',
              'cubicle_evaluation', 'fixture-source', 'tpm_review_label',
              '2026-06-21T01:05:00+00:00', '2026-06-21T01:05:00+00:00'
            )
            """
        )
        brief.backfill_review_measurement_eligibility(conn, "fixture-source", {"agent_adversarial"})
        self.assertEqual(
            conn.execute(
                "select measurement_eligible from work_insight_reviews where key = 'review:adversarial-partial'"
            ).fetchone()[0],
            0,
        )
        conn.execute(
            "update work_insight_reviews set measurement_eligible = 1 where key = 'review:adversarial-partial'"
        )

        action_id = insert_blocker_action(conn, "tpm-action:test:partial")
        action_items = pd.DataFrame([blocker_action_item("tpm-action:test:partial", "partial action")])

        brief.persist_work_blockers_to_ontology(conn, "fixture-source", action_items, generated_at)
        brief.persist_work_dependency_edges_to_ontology(conn, "fixture-source", pd.DataFrame(), generated_at)

        self.assertGreater(action_id, 0)
        self.assertEqual(conn.execute("select count(*) from work_actions").fetchone()[0], 1)
        self.assertEqual(conn.execute("select count(*) from work_blockers").fetchone()[0], 0)
        self.assertEqual(
            conn.execute(
                """
                select count(*)
                  from work_dependency_edges
                 where edge_kind in ('blocked_by', 'needs_action')
                """
            ).fetchone()[0],
            0,
        )

    def test_human_dismissal_removes_stale_product_backed_work_blocker(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_tables(conn)
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, subject_kind, subject_key, title,
              details, recommended_action, source_url, latest_evidence_id, score,
              confidence, rank_score, producer_state, source_system, source_instance,
              external_kind, updated_at
            ) values (
              1, 'work-insight:test:blocker', 'blocker_candidate', 'high',
              'pull_request', 'repo/example#9', 'Merge-blocking CI signal',
              'CI appears to be blocking merge.', 'Ask CI owner to confirm.',
              'https://github.com/repo/example/pull/9/checks', null, 91,
              0.86, 91, 'current', 'cubicle_analytics', 'fixture-source',
              'tpm_insight', '2026-06-21T01:00:00+00:00'
            )
            """
        )
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, reviewed_at, updated_at
            ) values (
              'review:imported', 1, 'evaluation_label', 'accepted', 'true_positive',
              'needs_owner', 'agent_gold_blockers', 'gold', 1, 'imported',
              'codex_agent_adjudication', '2026-06-21T01:05:00+00:00',
              '2026-06-21T01:05:00+00:00'
            )
            """
        )
        insert_blocker_action(conn, "tpm-action:test:first")
        action_items = pd.DataFrame([blocker_action_item("tpm-action:test:first", "first action")])
        brief.persist_work_blockers_to_ontology(
            conn,
            "fixture-source",
            action_items,
            "2026-06-21T01:15:00+00:00",
        )
        self.assertEqual(conn.execute("select count(*) from work_blockers").fetchone()[0], 1)
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, reviewed_at, updated_at
            ) values (
              'review:human', 1, 'human_assessment', 'dismissed', 'false_positive',
              'not_actionable', 'human_review', 'gold', 0, 'human',
              'harsh', '2026-06-21T01:20:00+00:00',
              '2026-06-21T01:20:00+00:00'
            )
            """
        )

        brief.persist_work_blockers_to_ontology(
            conn,
            "fixture-source",
            action_items,
            "2026-06-21T01:30:00+00:00",
        )

        self.assertEqual(conn.execute("select count(*) from work_blockers").fetchone()[0], 0)

    def test_work_blocker_impacts_materialize_direct_and_workstream_projection(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_impact_tables(conn)
        conn.execute(
            """
            insert into work_actions (
              id, key, action_type, action_state, decision_state, subject_kind, subject_key,
              source_system, source_instance, external_kind, external_id
            ) values (10, 'tpm-action:test:blocker', 'clear_blocker', 'open', 'product_action',
                      'pull_request', 'repo/example#9', 'cubicle_analytics', 'fixture-source',
                      'tpm_work_action', 'tpm-action:test:blocker')
            """
        )
        conn.execute(
            """
            insert into work_blockers (
              id, key, blocker_kind, blocker_state, severity, subject_kind, subject_key,
              work_action_id, decision_state, source_coverage_state, title, summary,
              recommended_action, source_system, source_instance, external_kind, external_id,
              source_url, latest_evidence_id, evidence_count, freshness_state, visibility,
              confidence, rank_score, last_activity_at
            ) values (
              20, 'work-blocker:test:ci', 'ci', 'active', 'high', 'pull_request',
              'repo/example#9', 10, 'product_action', 'observed:github_checks:complete',
              'CI blocks merge', 'Failing check blocks merge.', 'Ask owner to clear CI.',
              'cubicle_analytics', 'fixture-source', 'tpm_work_blocker', 'work-blocker:test:ci',
              'https://github.com/repo/example/pull/9/checks', 7, 1, 'fresh', 'unknown',
              0.86, 91, '2026-06-21T00:00:00+00:00'
            )
            """
        )
        conn.execute(
            """
            insert into workstreams (id, key, title, source_system, source_instance, external_kind, external_id)
            values (30, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator',
                    'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            """
            insert into work_dependency_edges (
              id, key, edge_kind, from_kind, from_key, to_kind, to_key, risk_signal,
              source_coverage_state, workstream_id, work_blocker_id, work_action_id,
              source_system, source_instance, external_kind, external_id
            ) values (
              40, 'work-dependency-edge:test:blocker', 'blocked_by', 'pull_request',
              'repo/example#9', 'blocker', 'work-blocker:test:ci', 'product_action',
              'fresh', 30, 20, 10, 'cubicle_analytics', 'fixture-source',
              'tpm_work_dependency_edge', 'edge:test:blocker'
            )
            """
        )

        brief.persist_work_blocker_impacts_to_ontology(
            conn,
            "fixture-source",
            "2026-06-21T00:00:00+00:00",
        )

        rows = conn.execute(
            """
            select impact_kind, impact_state, affected_kind, affected_key, workstream_id,
                   work_blocker_id, work_action_id, impact_score, path_length, title,
                   recommended_action, source_coverage_state, latest_evidence_id
              from work_blocker_impacts
             order by path_length, affected_kind
            """
        ).fetchall()
        self.assertEqual(len(rows), 2)
        self.assertEqual(
            rows[0],
            (
                "direct_subject",
                "active",
                "pull_request",
                "repo/example#9",
                None,
                20,
                10,
                166.0,
                0,
                "CI blocks merge",
                "Ask owner to clear CI.",
                "observed:github_checks:complete",
                7,
            ),
        )
        self.assertEqual(rows[1][0:9], ("workstream", "active", "workstream", "workstream:flink-kubernetes-operator", 30, 20, 10, 161.0, 1))
        self.assertIn("impacts workstream:flink-kubernetes-operator", rows[1][9])

    def test_work_blocker_refresh_deletes_stale_generated_topology_for_source(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_impact_tables(conn)
        conn.execute(
            """
            insert into work_blockers (
              id, key, blocker_kind, blocker_state, severity, subject_kind, subject_key,
              decision_state, source_system, source_instance, external_kind, external_id,
              freshness_state, visibility, confidence, rank_score
            ) values
              (20, 'work-blocker:stale', 'ci', 'active', 'high', 'pull_request',
               'repo/example#9', 'product_action', 'cubicle_analytics', 'fixture-source',
               'tpm_work_blocker', 'work-blocker:stale', 'fresh', 'unknown', 0.9, 91),
              (21, 'work-blocker:other-source', 'ci', 'active', 'high', 'pull_request',
               'repo/example#10', 'product_action', 'cubicle_analytics', 'other-source',
               'tpm_work_blocker', 'work-blocker:other-source', 'fresh', 'unknown', 0.9, 91)
            """
        )
        conn.execute(
            """
            insert into work_dependency_edges (
              id, key, edge_kind, from_kind, from_key, to_kind, to_key, work_blocker_id,
              source_system, source_instance, external_kind, external_id
            ) values
              (40, 'work-dependency-edge:stale', 'blocked_by', 'pull_request',
               'repo/example#9', 'blocker', 'work-blocker:stale', 20, 'cubicle_analytics',
               'fixture-source', 'tpm_work_dependency_edge', 'work-dependency-edge:stale'),
              (41, 'work-dependency-edge:other-source', 'blocked_by', 'pull_request',
               'repo/example#10', 'blocker', 'work-blocker:other-source', 21,
               'cubicle_analytics', 'other-source', 'tpm_work_dependency_edge',
               'work-dependency-edge:other-source')
            """
        )
        brief.ensure_work_dependency_endpoints_table(conn)
        conn.executemany(
            """
            insert into work_dependency_endpoints (
              key, endpoint_role, node_kind, node_key, resolution_state,
              source_system, source_instance, external_kind, external_id,
              created_at, updated_at, work_dependency_edge_id, work_blocker_id
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            [
                (
                    "work-dependency-endpoint:stale",
                    "to",
                    "blocker",
                    "work-blocker:stale",
                    "resolved",
                    "cubicle_analytics",
                    "fixture-source",
                    "tpm_work_dependency_endpoint",
                    "work-dependency-endpoint:stale",
                    "2026-06-21T00:00:00+00:00",
                    "2026-06-21T00:00:00+00:00",
                    40,
                    20,
                ),
                (
                    "work-dependency-endpoint:other-source",
                    "to",
                    "blocker",
                    "work-blocker:other-source",
                    "resolved",
                    "cubicle_analytics",
                    "other-source",
                    "tpm_work_dependency_endpoint",
                    "work-dependency-endpoint:other-source",
                    "2026-06-21T00:00:00+00:00",
                    "2026-06-21T00:00:00+00:00",
                    41,
                    21,
                ),
            ],
        )
        conn.execute(
            """
            insert into work_blocker_impacts (
              id, key, impact_kind, impact_state, impact_score, severity, blocker_kind,
              work_blocker_id, affected_kind, affected_key, subject_kind, subject_key,
              path_length, title, source_system, source_instance, external_kind, external_id
            ) values
              (50, 'work-blocker-impact:stale', 'direct_subject', 'active', 100, 'high',
               'ci', 20, 'pull_request', 'repo/example#9', 'pull_request', 'repo/example#9',
               0, 'Stale impact', 'cubicle_analytics', 'fixture-source',
               'tpm_work_blocker_impact', 'work-blocker-impact:stale'),
              (51, 'work-blocker-impact:other-source', 'direct_subject', 'active', 100,
               'high', 'ci', 21, 'pull_request', 'repo/example#10', 'pull_request',
               'repo/example#10', 0, 'Other impact', 'cubicle_analytics', 'other-source',
               'tpm_work_blocker_impact', 'work-blocker-impact:other-source')
            """
        )

        brief.persist_work_blockers_to_ontology(
            conn,
            "fixture-source",
            pd.DataFrame(columns=brief.empty_action_items().columns),
            "2026-06-21T02:00:00+00:00",
        )

        self.assertEqual(conn.execute("select count(*) from work_blockers where source_instance = 'fixture-source'").fetchone()[0], 0)
        self.assertEqual(conn.execute("select count(*) from work_dependency_edges where source_instance = 'fixture-source'").fetchone()[0], 0)
        self.assertEqual(conn.execute("select count(*) from work_dependency_endpoints where source_instance = 'fixture-source'").fetchone()[0], 0)
        self.assertEqual(conn.execute("select count(*) from work_blocker_impacts where source_instance = 'fixture-source'").fetchone()[0], 0)
        self.assertEqual(conn.execute("select count(*) from work_blockers where source_instance = 'other-source'").fetchone()[0], 1)
        self.assertEqual(conn.execute("select count(*) from work_dependency_edges where source_instance = 'other-source'").fetchone()[0], 1)
        self.assertEqual(conn.execute("select count(*) from work_dependency_endpoints where source_instance = 'other-source'").fetchone()[0], 1)
        self.assertEqual(conn.execute("select count(*) from work_blocker_impacts where source_instance = 'other-source'").fetchone()[0], 1)

    def test_dependency_and_impact_empty_refresh_deletes_stale_generated_rows(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_impact_tables(conn)
        conn.execute(
            """
            insert into work_dependency_edges (
              key, edge_kind, from_kind, from_key, to_kind, to_key, source_system,
              source_instance, external_kind, external_id
            ) values
              ('work-dependency-edge:stale', 'related_work', 'pull_request',
               'repo/example#9', 'component', 'component:autoscaler',
               'cubicle_analytics', 'fixture-source', 'tpm_work_dependency_edge',
               'work-dependency-edge:stale'),
              ('work-dependency-edge:other-source', 'related_work', 'pull_request',
               'repo/example#10', 'component', 'component:autoscaler',
               'cubicle_analytics', 'other-source', 'tpm_work_dependency_edge',
               'work-dependency-edge:other-source')
            """
        )
        conn.execute(
            """
            insert into work_blocker_impacts (
              key, impact_kind, impact_state, impact_score, severity, blocker_kind,
              affected_kind, affected_key, subject_kind, subject_key, path_length, title,
              source_system, source_instance, external_kind, external_id
            ) values
              ('work-blocker-impact:stale', 'direct_subject', 'active', 100, 'high',
               'ci', 'pull_request', 'repo/example#9', 'pull_request', 'repo/example#9',
               0, 'Stale impact', 'cubicle_analytics', 'fixture-source',
               'tpm_work_blocker_impact', 'work-blocker-impact:stale'),
              ('work-blocker-impact:other-source', 'direct_subject', 'active', 100,
               'high', 'ci', 'pull_request', 'repo/example#10', 'pull_request',
               'repo/example#10', 0, 'Other impact', 'cubicle_analytics', 'other-source',
               'tpm_work_blocker_impact', 'work-blocker-impact:other-source')
            """
        )

        brief.persist_work_dependency_edges_to_ontology(
            conn,
            "fixture-source",
            pd.DataFrame(columns=["source_key", "target_key", "edge_kind", "risk_signal", "freshness"]),
            "2026-06-21T02:00:00+00:00",
        )
        brief.persist_work_blocker_impacts_to_ontology(
            conn,
            "fixture-source",
            "2026-06-21T02:00:00+00:00",
        )

        self.assertEqual(conn.execute("select count(*) from work_dependency_edges where source_instance = 'fixture-source'").fetchone()[0], 0)
        self.assertEqual(conn.execute("select count(*) from work_blocker_impacts where source_instance = 'fixture-source'").fetchone()[0], 0)
        self.assertEqual(conn.execute("select count(*) from work_dependency_edges where source_instance = 'other-source'").fetchone()[0], 1)
        self.assertEqual(conn.execute("select count(*) from work_blocker_impacts where source_instance = 'other-source'").fetchone()[0], 1)

    def test_dependency_topology_edges_materialize_generated_evidence(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_impact_tables(conn)
        conn.execute(
            """
            insert into workstreams (id, key, title, source_system, source_instance, external_kind, external_id)
            values (30, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator',
                    'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            """
            insert into tickets (id, external_id)
            values (1, 'FLINK-12345')
            """
        )
        conn.execute(
            """
            insert into pull_requests (id, repository, number)
            values (2, 'apache/flink-kubernetes-operator', 42)
            """
        )
        conn.execute(
            """
            insert into evidences (
              id, key, claim_kind, relationship_kind, locator_kind, locator,
              source_system, source_instance, external_kind
            ) values (
              7, 'evidence:relationship', 'relationship', 'implemented_by',
              'jira_remote_link', 'FLINK-12345 -> PR 42', 'jira',
              'fixture-source', 'jira_remote_links'
            )
            """
        )
        conn.execute(
            """
            insert into ticket_pull_requests (
              ticket_id, pull_request_id, ticket_pull_request_kind, latest_evidence_id,
              evidence_count, source_url
            ) values (
              1, 2, 'implemented_by', 7, 1, 'https://issues.apache.org/jira/browse/FLINK-12345'
            )
            """
        )
        dependency_edges = pd.DataFrame(
            [
                {
                    "source_key": "ticket:FLINK-12345",
                    "target_key": "pr:apache/flink-kubernetes-operator#42",
                    "edge_kind": "ticket_pr",
                    "freshness": "fresh",
                    "risk_signal": "remote_link",
                }
            ]
        )

        brief.persist_work_dependency_edges_to_ontology(
            conn,
            "fixture-source",
            dependency_edges,
            "2026-06-21T02:00:00+00:00",
        )

        edge = conn.execute(
            """
            select id, edge_kind, relationship_authority, canonical_relationship_kind,
                   from_kind, from_key, to_kind, to_key, latest_evidence_id,
                   evidence_count, source_coverage_state
              from work_dependency_edges
             where source_instance = 'fixture-source'
            """
        ).fetchone()
        self.assertIsNotNone(edge)
        self.assertEqual(
            edge[1:8],
            (
                "ticket_pr",
                "canonical_mirror",
                "ticket_pull_request",
                "ticket",
                "FLINK-12345",
                "pull_request",
                "apache/flink-kubernetes-operator#42",
            ),
        )
        self.assertIsNotNone(edge[8])
        self.assertEqual(edge[9], 1)
        self.assertEqual(edge[10], "fresh")

        evidence = conn.execute(
            """
            select claim_target_kind, claim_target_id, claim_field, locator_kind, locator, excerpt
              from evidences
             where id = ?
            """,
            (edge[8],),
        ).fetchone()
        edge_key = conn.execute("select key from work_dependency_edges where id = ?", (edge[0],)).fetchone()[0]
        self.assertEqual(edge_key.startswith("work-dependency-edge:"), True)
        self.assertEqual(evidence[0:5], (None, None, None, "jira_remote_link", "FLINK-12345 -> PR 42"))
        self.assertEqual(evidence[5], None)
        endpoints = conn.execute(
            """
            select endpoint_role, node_kind, node_key, resolution_state,
                   ticket_id, pull_request_id, latest_evidence_id
              from work_dependency_endpoints
             where work_dependency_edge_id = ?
             order by endpoint_role
            """,
            (edge[0],),
        ).fetchall()
        self.assertEqual(
            endpoints,
            [
                ("from", "ticket", "FLINK-12345", "resolved", 1, None, 7),
                ("to", "pull_request", "apache/flink-kubernetes-operator#42", "resolved", None, 2, 7),
            ],
        )

    def test_dependency_action_edges_export_ontology_backed_analytics_rows(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_blocker_impact_tables(conn)
        now = "2026-06-21T02:00:00+00:00"
        conn.execute(
            """
            insert into work_actions (
              id, key, action_type, action_state, decision_state, subject_kind, subject_key,
              owner_key, source_system, source_instance, external_kind, external_id, source_url
            ) values (
              10, 'tpm-action:test:blocker-action', 'validate_signal', 'open',
              'validation_lead', 'pull_request', 'repo/example#9', 'github:owner',
              'cubicle_analytics', 'fixture-source', 'tpm_work_action',
              'tpm-action:test:blocker-action', 'https://github.com/repo/example/pull/9'
            )
            """
        )
        conn.execute(
            """
            insert into work_blockers (
              id, key, blocker_kind, blocker_state, severity, subject_kind, subject_key,
              work_action_id, decision_state, source_coverage_state, review_state,
              truth_label, actionability_label, label_quality, measurement_eligible,
              title, source_system, source_instance, external_kind, external_id,
              source_url, freshness_state, rank_score
            ) values (
              20, 'work-blocker:test:ci', 'ci', 'validating', 'high',
              'pull_request', 'repo/example#9', 10, 'validation_lead',
              'observed:github_checks:complete', 'needs_more_data', 'partial',
              'needs_owner', 'candidate', 0, 'CI may block merge',
              'cubicle_analytics', 'fixture-source', 'tpm_work_blocker',
              'work-blocker:test:ci', 'https://github.com/repo/example/pull/9',
              'fresh', 91.0
            )
            """
        )
        conn.executemany(
            """
            insert into work_dependency_edges (
              key, edge_kind, from_kind, from_key, to_kind, to_key, risk_signal,
              source_coverage_state, work_blocker_id, work_action_id, source_system,
              source_instance, external_kind, external_id, source_url, freshness_state,
              rank_score, updated_at
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            [
                (
                    "work-dependency:test:blocked-by",
                    "blocked_by",
                    "pull_request",
                    "repo/example#9",
                    "blocker",
                    "work-blocker:test:ci",
                    "validation_lead",
                    "observed:github_checks:complete",
                    20,
                    10,
                    "cubicle_analytics",
                    "fixture-source",
                    "tpm_work_dependency_edge",
                    "work-dependency:test:blocked-by",
                    "https://github.com/repo/example/pull/9",
                    "fresh",
                    100.0,
                    now,
                ),
                (
                    "work-dependency:test:needs-action",
                    "needs_action",
                    "blocker",
                    "work-blocker:test:ci",
                    "action",
                    "tpm-action:test:blocker-action",
                    "validation_lead",
                    "observed:github_checks:complete",
                    20,
                    10,
                    "cubicle_analytics",
                    "fixture-source",
                    "tpm_work_dependency_edge",
                    "work-dependency:test:needs-action",
                    "https://github.com/repo/example/pull/9",
                    "fresh",
                    99.0,
                    now,
                ),
            ],
        )

        action_edges = brief.ontology_dependency_action_edges_for_analytics(conn, "fixture-source")
        self.assertEqual(action_edges["edge_kind"].tolist(), ["blocked_by", "needs_action"])
        blocked_by = action_edges[action_edges["edge_kind"] == "blocked_by"].iloc[0]
        self.assertEqual(blocked_by["source_key"], "pr:repo/example#9")
        self.assertEqual(blocked_by["target_key"], "work-blocker:test:ci")
        self.assertEqual(blocked_by["action_key"], "tpm-action:test:blocker-action")
        self.assertEqual(blocked_by["blocker_label_quality"], "candidate")
        self.assertEqual(blocked_by["blocker_measurement_eligible"], False)

        stale_analytics = pd.DataFrame(
            [
                {
                    "edge_kind": "needs_action",
                    "source_key": "work-blocker:test:ci",
                    "target_key": "tpm-action:test:blocker-action",
                    "freshness": "stale",
                    "risk_signal": "old_signal",
                },
                {
                    "edge_kind": "ticket_pr",
                    "source_key": "ticket:FLINK-1",
                    "target_key": "pr:repo/example#1",
                    "freshness": "fresh",
                    "risk_signal": "",
                },
            ]
        )
        with warnings.catch_warnings():
            warnings.simplefilter("error", FutureWarning)
            merged = brief.merge_dependency_edges_for_analytics(stale_analytics, action_edges)
        self.assertEqual(len(merged), 3)
        needs_action = merged[
            (merged["edge_kind"] == "needs_action")
            & (merged["source_key"] == "work-blocker:test:ci")
            & (merged["target_key"] == "tpm-action:test:blocker-action")
        ].iloc[0]
        self.assertEqual(needs_action["freshness"], "fresh")
        self.assertEqual(needs_action["risk_signal"], "validation_lead")
        self.assertEqual(needs_action["action_state"], "open")
        with warnings.catch_warnings():
            warnings.simplefilter("error", FutureWarning)
            source_only = brief.merge_dependency_edges_for_analytics(stale_analytics, pd.DataFrame())
        self.assertEqual(source_only["edge_kind"].tolist(), ["ticket_pr"])

    def test_concat_preserving_columns_keeps_all_null_typed_column_dtype(self) -> None:
        left = pd.DataFrame({"edge_kind": ["ticket_pr"], "optional_text": pd.Series([None], dtype="string")})
        right = pd.DataFrame({"edge_kind": ["blocked_by"], "optional_text": pd.Series([None], dtype="string")})

        out = brief.concat_dataframes_preserving_columns([left, right], ["edge_kind", "optional_text"])

        self.assertEqual(out["edge_kind"].tolist(), ["ticket_pr", "blocked_by"])
        self.assertEqual(str(out["optional_text"].dtype), "string")

    def test_forecast_evaluation_materializes_typed_summary_and_backtest_rows(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_forecast_tables(conn)
        forecast_summary = pd.DataFrame(
            [
                {"metric": "merged_pr_count", "value": "60", "note": "delivery baseline sample"},
                {"metric": "closed_unmerged_pr_count", "value": "20", "note": "closure context"},
                {"metric": "open_pr_count", "value": "20", "note": "current candidates"},
                {"metric": "median_merged_cycle_days", "value": "5.25", "note": "median cycle"},
                {"metric": "p75_merged_cycle_days", "value": "11.28", "note": "p75 cycle"},
                {"metric": "avg_closed_unmerged_cycle_days", "value": "102.55", "note": "closed context"},
                {"metric": "forecast_method", "value": "heuristic_percentile_rf_rejected", "note": "selected method"},
                {"metric": "backtest_best_model", "value": "median_cycle_baseline", "note": "best k-fold model"},
                {"metric": "backtest_median_mae_days", "value": "8.71", "note": "median baseline"},
                {"metric": "backtest_heuristic_mae_days", "value": "11.10", "note": "heuristic"},
                {"metric": "backtest_random_forest_mae_days", "value": "10.41", "note": "random forest"},
                {"metric": "eta_forecast_ready", "value": "false", "note": "gate"},
            ]
        )
        forecast_backtest = pd.DataFrame(
            [
                {
                    "evaluation": "kfold",
                    "model": "median_cycle_baseline",
                    "fold": 1,
                    "train_count": 48,
                    "test_count": 12,
                    "mae_days": 8.71,
                    "median_error_days": 4.6,
                    "p75_error_days": 11.69,
                    "max_error_days": 71.66,
                    "improvement_vs_median_pct": 0.0,
                    "ready_for_eta": "false",
                    "note": "Backtest error.",
                },
                {
                    "evaluation": "kfold",
                    "model": "random_forest_regressor",
                    "fold": 1,
                    "train_count": 48,
                    "test_count": 12,
                    "mae_days": 10.41,
                    "median_error_days": 8.46,
                    "p75_error_days": 15.99,
                    "max_error_days": 71.92,
                    "improvement_vs_median_pct": -8.35,
                    "ready_for_eta": "false",
                    "note": "Backtest error.",
                },
                {
                    "evaluation": "survival_time_to_merge",
                    "model": "km_median_remaining",
                    "fold": 1,
                    "train_count": 48,
                    "test_count": 18,
                    "mae_days": 9.25,
                    "median_error_days": 3.75,
                    "p75_error_days": 12.5,
                    "max_error_days": 44.0,
                    "improvement_vs_median_pct": 7.0,
                    "ready_for_eta": "false",
                    "note": "Censored survival baseline.",
                },
                {
                    "evaluation": "source_event_as_of_kfold",
                    "model": "random_forest_regressor",
                    "fold": 1,
                    "train_count": 120,
                    "test_count": 30,
                    "mae_days": 12.5,
                    "median_error_days": 4.5,
                    "p75_error_days": 16.0,
                    "max_error_days": 51.0,
                    "improvement_vs_median_pct": -3.0,
                    "ready_for_eta": "false",
                    "note": "Source-event replay as-of feature backtest.",
                },
            ]
        )
        forecast_risk_backtest = pd.DataFrame(
            [
                {
                    "metric": "precision_at_10pct",
                    "value": "0.59",
                    "sample_count": 84,
                    "method": "top 10pct by static risk score",
                    "interpretation": "Risk triage enriches slow-cycle PRs.",
                    "guardrail": "Attention ranking only; not ETA.",
                },
                {
                    "metric": "coverage_stratified_backtest_state",
                    "value": "single_stratum",
                    "sample_count": 838,
                    "method": "coverage-stratified risk backtest readiness",
                    "interpretation": "No source-coverage stratification confounder in this sample.",
                    "guardrail": "Attention ranking only; not ETA.",
                },
            ]
        )
        decision_target_backtest = pd.DataFrame(
            [
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_grouped_kfold",
                    "model": "random_forest_classifier",
                    "fold": 1,
                    "train_count": 120,
                    "test_count": 30,
                    "positive_count": 6,
                    "baseline_positive_rate": 0.2,
                    "precision_at_10pct": 0.5,
                    "lift_at_10pct": 0.3,
                    "roc_auc": 0.72,
                    "average_precision": 0.42,
                    "coverage_stratum": "coverage=observed;detail=observed;mode=current;lifecycle=source_event;churn=source_event;mergeability=source_event",
                    "ready_for_product_action": "false",
                    "note": "Ranks closed-unmerged risk for validation only.",
                }
            ]
        )
        time_series_summary = pd.DataFrame(
            [
                {"metric": "observed_snapshot_time_count", "value": "1", "note": "one snapshot"},
                {"metric": "transition_candidate_count", "value": "0", "note": "no transitions yet"},
                {"metric": "terminal_transition_candidate_count", "value": "0", "note": "no terminal transitions"},
            ]
        )
        conn.execute(
            """
            insert into work_forecast_evaluations (
              key, evaluation_kind, model_name, ready_for_eta, readiness_state,
              evaluated_at, source_system, source_instance, external_kind, external_id
            ) values (
              'legacy-summary', 'summary', 'legacy_model', 1, 'ready',
              '2026-06-20T05:00:00+00:00', 'cubicle_analytics', 'fixture-source',
              'tpm_forecast_evaluation', 'summary'
            )
            """
        )
        conn.execute(
            """
            insert into work_forecast_evaluations (
              key, evaluation_kind, model_name, ready_for_eta, readiness_state,
              evaluated_at, source_system, source_instance, external_kind, external_id
            ) values (
              'stale-decision-target', 'source_event_as_of_grouped_kfold', 'old_decision_model', 0, 'gated',
              '2026-06-21T05:00:00+00:00', 'cubicle_analytics', 'fixture-source',
              'tpm_decision_target_backtest', 'abandonment_risk:source_event_as_of_grouped_kfold:old_decision_model:1:2026-06-21T05:00:00+00:00'
            )
            """
        )

        brief.persist_work_forecast_evaluations_to_ontology(
            conn,
            "fixture-source",
            forecast_summary,
            forecast_backtest,
            forecast_risk_backtest,
            decision_target_backtest,
            time_series_summary,
            "2026-06-21T05:00:00+00:00",
        )
        brief.persist_work_decision_target_evaluations_to_ontology(
            conn,
            "fixture-source",
            decision_target_backtest,
            "2026-06-21T05:00:00+00:00",
        )

        summary = conn.execute(
            """
            select evaluation_kind, model_name, forecast_method, best_model_name,
                   baseline_sample_count, open_candidate_count, ready_for_eta,
                   readiness_state, observed_snapshot_time_count, transition_candidate_count,
                   terminal_transition_candidate_count, transition_history_ready, readiness_reason
              from work_forecast_evaluations
             where evaluation_kind = 'summary'
            """
        ).fetchone()
        self.assertEqual(
            summary[:12],
            (
                "summary",
                "median_cycle_baseline",
                "heuristic_percentile_rf_rejected",
                "median_cycle_baseline",
                60,
                20,
                0,
                "gated",
                1,
                0,
                0,
                0,
            ),
        )
        self.assertIn("best K-fold model is median_cycle_baseline", summary[12])
        self.assertIn("only 1 distinct observed snapshot", summary[12])
        legacy_count = conn.execute(
            """
            select count(*)
              from work_forecast_evaluations
             where external_id in ('summary', 'kfold:median_cycle_baseline:1', 'kfold:random_forest_regressor:1')
            """
        ).fetchone()[0]
        self.assertEqual(legacy_count, 0)
        summary_evidence = conn.execute(
            """
            select e.claim_target_kind, e.claim_field, e.locator_kind, e.locator, e.excerpt
              from work_forecast_evaluations wfe
              join evidences e on e.id = wfe.latest_evidence_id
             where wfe.evaluation_kind = 'summary'
            """
        ).fetchone()
        self.assertEqual(summary_evidence[:4], ("work_forecast_evaluation", "readiness_state", "forecast_backtest", "summary"))
        self.assertIn("only 1 distinct observed snapshot", summary_evidence[4])
        fold_rows = conn.execute(
            """
            select evaluation_kind, model_name, fold, train_count, test_count,
                   mae_days, readiness_state
              from work_forecast_evaluations
             where evaluation_kind = 'kfold'
             order by model_name
            """
        ).fetchall()
        self.assertEqual(
            fold_rows,
            [
                ("kfold", "median_cycle_baseline", 1, 48, 12, 8.71, "gated"),
                ("kfold", "random_forest_regressor", 1, 48, 12, 10.41, "gated"),
            ],
        )
        survival_rows = conn.execute(
            """
            select evaluation_kind, model_name, fold, train_count, test_count,
                   mae_days, ready_for_eta, readiness_state, readiness_reason, evidence_count
              from work_forecast_evaluations
             where evaluation_kind = 'survival_time_to_merge'
            """
        ).fetchall()
        self.assertEqual(
            survival_rows,
            [
                (
                    "survival_time_to_merge",
                    "km_median_remaining",
                    1,
                    48,
                    18,
                    9.25,
                    0,
                    "gated",
                    "Censored survival baseline.",
                    1,
                )
            ],
        )
        source_event_rows = conn.execute(
            """
            select evaluation_kind, model_name, fold, train_count, test_count,
                   mae_days, ready_for_eta, readiness_state, readiness_reason, evidence_count
              from work_forecast_evaluations
             where evaluation_kind = 'source_event_as_of_kfold'
            """
        ).fetchall()
        self.assertEqual(
            source_event_rows,
            [
                (
                    "source_event_as_of_kfold",
                    "random_forest_regressor",
                    1,
                    120,
                    30,
                    12.5,
                    0,
                    "gated",
                    "Source-event replay as-of feature backtest.",
                    1,
                )
            ],
        )
        risk_rows = conn.execute(
            """
            select model_name, forecast_method, external_kind, external_id,
                   baseline_sample_count, test_count, ready_for_eta,
                   readiness_state, readiness_reason, note, evidence_count
              from work_forecast_evaluations
             where external_kind = 'tpm_forecast_risk_backtest'
             order by external_id
            """
        ).fetchall()
        self.assertEqual(len(risk_rows), 2)
        for row in risk_rows:
            self.assertEqual(row[0:3], ("static_risk_triage", "coverage-stratified risk backtest readiness" if "coverage_stratified" in row[3] else "top 10pct by static risk score", "tpm_forecast_risk_backtest"))
            self.assertEqual(row[6:9], (0, "gated", "Risk-triage backtest supports attention ranking only; ETA commitments remain gated."))
            self.assertEqual(row[10], 1)
        self.assertIn("No source-coverage stratification confounder", " ".join(row[9] for row in risk_rows))
        risk_evidence = conn.execute(
            """
            select e.claim_target_kind, e.claim_field, e.locator_kind, e.excerpt
              from work_forecast_evaluations wfe
              join evidences e on e.id = wfe.latest_evidence_id
             where wfe.external_kind = 'tpm_forecast_risk_backtest'
             order by wfe.external_id
            """
        ).fetchall()
        self.assertEqual(len(risk_evidence), 2)
        self.assertTrue(all(row[0:3] == ("work_forecast_evaluation", "readiness_state", "forecast_risk_backtest") for row in risk_evidence))
        self.assertIn("Attention ranking only; not ETA.", " ".join(row[3] for row in risk_evidence))
        legacy_decision_target_rows = conn.execute(
            """
            select count(*)
              from work_forecast_evaluations
             where external_kind = 'tpm_decision_target_backtest'
            """
        ).fetchone()[0]
        self.assertEqual(legacy_decision_target_rows, 0)
        decision_target_rows = conn.execute(
            """
            select model_name, external_kind, external_id, positive_count,
                   test_count, average_precision, baseline_positive_rate, precision_at_10pct,
                   roc_auc, lift_at_10pct, ready_for_product_action,
                   product_action_gate_state, product_action_gate_reason, note, evidence_count
              from work_decision_target_evaluations
             where external_kind = 'tpm_decision_target_evaluation'
            """
        ).fetchall()
        self.assertEqual(
            decision_target_rows,
            [
                (
                    "random_forest_classifier",
                    "tpm_decision_target_evaluation",
                    "abandonment_risk:source_event_as_of_grouped_kfold:random_forest_classifier:1:coverage=observed;detail=observed;mode=current;lifecycle=source_event;churn=source_event;mergeability=source_event:2026-06-21T05:00:00+00:00",
                    6,
                    30,
                    0.42,
                    0.2,
                    0.5,
                    0.72,
                    0.3,
                    0,
                    "missing_coverage_guardrail",
                    "No coverage-stratified guardrail row is available for decision-target product-action readiness.",
                    "Ranks closed-unmerged risk for validation only.",
                    1,
                )
            ],
        )
        brief.persist_work_forecast_evaluations_to_ontology(
            conn,
            "fixture-source",
            forecast_summary,
            forecast_backtest,
            forecast_risk_backtest,
            decision_target_backtest,
            time_series_summary,
            "2026-06-22T05:00:00+00:00",
        )
        brief.persist_work_decision_target_evaluations_to_ontology(
            conn,
            "fixture-source",
            decision_target_backtest,
            "2026-06-22T05:00:00+00:00",
        )
        run_counts = conn.execute(
            """
            select evaluation_kind, count(*), count(distinct evaluated_at)
              from work_forecast_evaluations
             group by evaluation_kind
             order by evaluation_kind
            """
        ).fetchall()
        self.assertEqual(
            run_counts,
            [
                ("kfold", 4, 2),
                ("source_event_as_of_kfold", 2, 2),
                ("summary", 6, 2),
                ("survival_time_to_merge", 2, 2),
            ],
        )

    def test_decision_target_persistence_clamps_producer_ready_without_coverage_or_independent_evidence(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_forecast_tables(conn)
        decision_target_backtest = pd.DataFrame(
            [
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratified_summary",
                    "model": "coverage_guardrail",
                    "coverage_stratum": "not_testable_single_stratum",
                    "ready_for_product_action": "false",
                    "note": "Decision-target validation has one source coverage/provenance stratum; coverage confounding cannot be tested.",
                },
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratum",
                    "model": "random_forest_classifier_oof",
                    "test_count": 300,
                    "positive_count": 80,
                    "baseline_positive_rate": 0.2667,
                    "precision_at_10pct": 0.8,
                    "lift_at_10pct": 0.5333,
                    "roc_auc": 0.91,
                    "average_precision": 0.77,
                    "coverage_stratum": "coverage=observed;detail=observed",
                    "ready_for_product_action": "true",
                    "product_action_gate_state": "passed",
                    "note": "Producer claims ready, but this row still lacks coverage and independent-evidence gates.",
                },
            ]
        )

        brief.persist_work_decision_target_evaluations_to_ontology(
            conn,
            "fixture-source",
            decision_target_backtest,
            "2026-06-23T03:10:00+00:00",
        )

        rows = conn.execute(
            """
            select model_name, ready_for_product_action, product_action_gate_state,
                   product_action_gate_reason, confidence, evidence_count
              from work_decision_target_evaluations
             order by case when model_name = 'coverage_guardrail' then 0 else 1 end, model_name
            """
        ).fetchall()
        self.assertEqual(len(rows), 2)
        self.assertEqual(
            rows[0],
            (
                "coverage_guardrail",
                0,
                "validation_gated",
                "Decision-target validation has one source coverage/provenance stratum; coverage confounding cannot be tested.",
                0.25,
                1,
            ),
        )
        self.assertEqual(rows[1][0:3], ("random_forest_classifier_oof", 0, "validation_gated"))
        self.assertIn("coverage confounding cannot be tested", rows[1][3])
        self.assertEqual(rows[1][4], 0.7)
        self.assertEqual(rows[1][5], 1)

    def test_decision_target_persistence_requires_independent_evidence_on_coverage_gate(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_forecast_tables(conn)
        decision_target_backtest = pd.DataFrame(
            [
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratified_summary",
                    "model": "coverage_guardrail",
                    "coverage_stratum": "coverage=observed",
                    "ready_for_product_action": "true",
                    "product_action_gate_state": "passed",
                    "note": "Raw analytics claims the global coverage guardrail passed.",
                },
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratum",
                    "model": "random_forest_classifier_oof",
                    "test_count": 300,
                    "positive_count": 80,
                    "baseline_positive_rate": 0.2667,
                    "precision_at_10pct": 0.8,
                    "lift_at_10pct": 0.5333,
                    "roc_auc": 0.91,
                    "average_precision": 0.77,
                    "coverage_stratum": "coverage=observed",
                    "ready_for_product_action": "true",
                    "product_action_gate_state": "passed",
                    "product_action_evidence_kind": "human_review",
                    "note": "Human-reviewed decision-target evaluation passed.",
                },
            ]
        )

        brief.persist_work_decision_target_evaluations_to_ontology(
            conn,
            "fixture-source",
            decision_target_backtest,
            "2026-06-23T03:10:00+00:00",
        )

        rows = conn.execute(
            """
            select model_name, ready_for_product_action, product_action_gate_state,
                   product_action_gate_reason
              from work_decision_target_evaluations
             order by case when model_name = 'coverage_guardrail' then 0 else 1 end, model_name
            """
        ).fetchall()
        self.assertEqual(len(rows), 2)
        for row in rows:
            self.assertEqual(row[1], 0)
            self.assertEqual(row[2], "validation_gated")
            self.assertIn("generated validation evidence only", row[3])

    def test_decision_target_coverage_gate_prefers_global_summary_row(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_forecast_tables(conn)
        decision_target_backtest = pd.DataFrame(
            [
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratum",
                    "model": "coverage_guardrail",
                    "coverage_stratum": "coverage=observed",
                    "ready_for_product_action": "true",
                    "product_action_gate_state": "passed",
                    "product_action_evidence_kind": "human_review",
                    "note": "Per-stratum guardrail passed.",
                },
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratified_summary",
                    "model": "coverage_guardrail",
                    "coverage_stratum": "not_testable_single_stratum",
                    "ready_for_product_action": "false",
                    "note": "Global summary says coverage confounding cannot be tested.",
                },
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratum",
                    "model": "random_forest_classifier_oof",
                    "test_count": 300,
                    "positive_count": 80,
                    "baseline_positive_rate": 0.2667,
                    "precision_at_10pct": 0.8,
                    "lift_at_10pct": 0.5333,
                    "roc_auc": 0.91,
                    "average_precision": 0.77,
                    "coverage_stratum": "coverage=observed",
                    "ready_for_product_action": "true",
                    "product_action_gate_state": "passed",
                    "product_action_evidence_kind": "human_review",
                    "note": "Human-reviewed decision-target evaluation passed.",
                },
            ]
        )

        brief.persist_work_decision_target_evaluations_to_ontology(
            conn,
            "fixture-source",
            decision_target_backtest,
            "2026-06-23T03:10:00+00:00",
        )

        rows = conn.execute(
            """
            select evaluation_kind, model_name, ready_for_product_action, product_action_gate_state,
                   product_action_gate_reason
              from work_decision_target_evaluations
             order by id
            """
        ).fetchall()
        self.assertEqual(len(rows), 3)
        for row in rows:
            self.assertEqual(row[2], 0)
            self.assertEqual(row[3], "validation_gated")
            self.assertIn("coverage confounding cannot be tested", row[4])

    def test_decision_target_persistence_allows_product_ready_only_with_passed_coverage_and_independent_evidence(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_forecast_tables(conn)
        decision_target_backtest = pd.DataFrame(
            [
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratified_summary",
                    "model": "coverage_guardrail",
                    "coverage_stratum": "coverage=observed",
                    "ready_for_product_action": "true",
                    "product_action_gate_state": "passed",
                    "product_action_evidence_kind": "human_review",
                    "note": "Coverage-stratified decision-target validation passed.",
                },
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratum",
                    "model": "random_forest_classifier_oof",
                    "test_count": 300,
                    "positive_count": 80,
                    "baseline_positive_rate": 0.2667,
                    "precision_at_10pct": 0.8,
                    "lift_at_10pct": 0.5333,
                    "roc_auc": 0.91,
                    "average_precision": 0.77,
                    "coverage_stratum": "coverage=observed",
                    "ready_for_product_action": "true",
                    "product_action_gate_state": "passed",
                    "product_action_evidence_kind": "human_review",
                    "note": "Human-reviewed decision-target evaluation passed.",
                },
            ]
        )

        brief.persist_work_decision_target_evaluations_to_ontology(
            conn,
            "fixture-source",
            decision_target_backtest,
            "2026-06-23T03:10:00+00:00",
        )

        rows = conn.execute(
            """
            select model_name, ready_for_product_action, product_action_gate_state,
                   product_action_gate_reason, confidence
              from work_decision_target_evaluations
             order by case when model_name = 'coverage_guardrail' then 0 else 1 end, model_name
            """
        ).fetchall()
        self.assertEqual(
            rows,
            [
                ("coverage_guardrail", 1, "passed", "Decision-target evaluation has passed product-action gates.", 0.95),
                ("random_forest_classifier_oof", 1, "passed", "Decision-target evaluation has passed product-action gates.", 0.95),
            ],
        )

    def test_work_item_forecast_materializes_only_resolved_pr_subjects(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_item_forecast_tables(conn)
        conn.execute(
            """
            insert into pull_requests (id, key, repository, number, title, source_url)
            values
              (1, 'pr:repo/example:72', 'repo/example', 72, 'Typed forecast target', 'https://github.com/repo/example/pull/72'),
              (2, 'pr:repo/example:73', 'repo/example', 73, 'Fallback action target', 'https://github.com/repo/example/pull/73')
            """
        )
        conn.execute(
            """
            insert into work_actions (
              id, key, subject_kind, subject_key, action_type, action_state,
              decision_state, rank_score, source_system, source_instance,
              external_kind, external_id, updated_at
            ) values (
              10, 'tpm-action:test:forecast-risk', 'pull_request', 'repo/example#72',
              'decision_or_owner_followup', 'open', 'validation_lead', 95,
              'cubicle_analytics', 'fixture-source', 'tpm_work_action',
              'tpm-action:test:forecast-risk', '2026-06-21T05:00:00+00:00'
            )
            """
        )
        conn.execute(
            """
            insert into work_actions (
              id, key, subject_kind, subject_key, action_type, action_state,
              decision_state, rank_score, source_system, source_instance,
              external_kind, external_id, updated_at
            ) values (
              11, 'tpm-action:test:blocker-risk', 'pull_request', 'repo/example#73',
              'clear_blocker', 'open', 'product_action', 100,
              'cubicle_analytics', 'fixture-source', 'tpm_work_action',
              'tpm-action:test:blocker-risk', '2026-06-21T05:30:00+00:00'
            )
            """
        )
        pr_features = pd.DataFrame(
            [
                {
                    "repository": "repo/example",
                    "pr_number": 72,
                    "pr_url": "https://github.com/repo/example/pull/72",
                    "state": "open",
                    "age_days": 42.5,
                    "predicted_total_cycle_days": 11.25,
                    "predicted_remaining_days": 0.0,
                    "overdue_days": 31.25,
                    "risk_score": 100.0,
                    "risk_band": "critical",
                    "forecast_method": "heuristic_percentile_rf_rejected",
                },
                {
                    "repository": "repo/example",
                    "pr_number": 73,
                    "pr_url": "https://github.com/repo/example/pull/73",
                    "state": "open",
                    "risk_score": 98.0,
                    "risk_band": "critical",
                },
                {
                    "repository": "repo/example",
                    "pr_number": 999,
                    "pr_url": "https://github.com/repo/example/pull/999",
                    "state": "open",
                    "risk_score": 99.0,
                    "risk_band": "critical",
                },
            ]
        )
        forecast_summary = pd.DataFrame(
            [
                {"metric": "merged_pr_count", "value": "60", "note": "delivery baseline sample"},
                {"metric": "forecast_method", "value": "heuristic_percentile_rf_rejected", "note": "selected method"},
                {"metric": "backtest_best_model", "value": "median_cycle_baseline", "note": "best k-fold model"},
                {"metric": "backtest_median_mae_days", "value": "8.71", "note": "median baseline"},
                {"metric": "backtest_heuristic_mae_days", "value": "11.10", "note": "heuristic"},
                {"metric": "backtest_random_forest_mae_days", "value": "10.41", "note": "random forest"},
                {"metric": "eta_forecast_ready", "value": "false", "note": "gate"},
            ]
        )
        time_series_summary = pd.DataFrame(
            [
                {"metric": "observed_snapshot_time_count", "value": "3", "note": "enough snapshots"},
                {"metric": "transition_candidate_count", "value": "1", "note": "observed transition"},
                {"metric": "terminal_transition_candidate_count", "value": "1", "note": "observed terminal transition"},
            ]
        )

        brief.persist_work_item_forecasts_to_ontology(
            conn,
            "fixture-source",
            pr_features,
            forecast_summary,
            time_series_summary,
            "2026-06-21T06:00:00+00:00",
        )

        rows = conn.execute(
            """
            select subject_kind, subject_key, pull_request_id, ticket_id, subject_state,
                   forecast_method, model_name, age_days, predicted_total_cycle_days,
                   predicted_remaining_days, overdue_days, risk_score, risk_band,
                   readiness_state, ready_for_eta, readiness_reason, source_url,
                   work_action_id, latest_evidence_id, evidence_count
              from work_item_forecasts
             order by subject_key
            """
        ).fetchall()
        self.assertEqual(len(rows), 2)
        self.assertEqual(
            rows[0][:15],
            (
                "pull_request",
                "repo/example#72",
                1,
                None,
                "open",
                "heuristic_percentile_rf_rejected",
                "median_cycle_baseline",
                42.5,
                11.25,
                0.0,
                31.25,
                100.0,
                "critical",
                "gated",
                0,
            ),
        )
        self.assertIn("best K-fold model is median_cycle_baseline", rows[0][15])
        self.assertNotIn("Transition forecasting is gated", rows[0][15])
        self.assertEqual(rows[0][16], "https://github.com/repo/example/pull/72")
        self.assertEqual(rows[0][17], 10)
        self.assertIsNotNone(rows[0][18])
        self.assertEqual(rows[0][19], 1)
        evidence = conn.execute(
            """
            select e.claim_target_kind, e.claim_field, e.locator_kind, e.locator, e.excerpt
              from work_item_forecasts wif
              join evidences e on e.id = wif.latest_evidence_id
             where wif.subject_key = 'repo/example#72'
            """
        ).fetchone()
        self.assertEqual(evidence[:4], ("work_item_forecast", "risk_band", "tpm_pr_forecast", "repo/example#72"))
        self.assertIn("forecast risk critical", evidence[4])
        self.assertIn("ETA-gated", evidence[4])
        self.assertIn("Over baseline by 31.2d", evidence[4])
        self.assertEqual(rows[1][1], "repo/example#73")
        self.assertEqual(rows[1][17], 11)

    def test_work_item_state_snapshots_materialize_resolved_pr_and_ticket_subjects(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_item_state_snapshot_tables(conn)
        conn.execute(
            """
            insert into pull_requests (id, key, repository, number, title, source_url)
            values (1, 'pr:repo/example:72', 'repo/example', 72, 'Snapshot PR', 'https://github.com/repo/example/pull/72')
            """
        )
        conn.execute(
            "insert into tickets (id, key, external_id, title, source_url) values (2, 'ticket:FLINK-1', 'FLINK-1', 'Snapshot ticket', 'https://issues.example.test/FLINK-1')"
        )
        pr_state_snapshots = pd.DataFrame(
            [
                {
                    "snapshot_key": "tpm-pr-state-snapshot:resolved",
                    "observed_at": "2026-06-21T05:29:08+00:00",
                    "subject_key": "repo/example#72",
                    "state": "open",
                    "title": "Snapshot PR",
                    "pr_url": "https://github.com/repo/example/pull/72",
                    "source_created_at": "2026-06-01T00:00:00+00:00",
                    "source_updated_at": "2026-06-20T00:00:00+00:00",
                    "age_days": 20.0,
                    "stale_days": 1.0,
                    "risk_score": 88.0,
                    "risk_band": "critical",
                    "forecast_method": "heuristic_percentile_rf_rejected",
                    "source_current_coverage_state": "observed",
                    "source_current_detail_state": "observed",
                    "lifecycle_fields_source": "live_followup_observation",
                    "captured_at": "2026-06-21T05:29:23+00:00",
                },
                {
                    "snapshot_key": "tpm-pr-state-snapshot:unresolved",
                    "observed_at": "2026-06-21T05:29:08+00:00",
                    "subject_key": "repo/example#999",
                    "state": "open",
                    "captured_at": "2026-06-21T05:29:23+00:00",
                },
            ]
        )
        ticket_state_snapshots = pd.DataFrame(
            [
                {
                    "snapshot_key": "tpm-ticket-state-snapshot:resolved",
                    "observed_at": "2026-06-21T05:29:08+00:00",
                    "ticket_key": "FLINK-1",
                    "status": "open",
                    "priority": "Major",
                    "title": "Snapshot ticket",
                    "updated_at": "2026-06-20T01:00:00+00:00",
                    "linked_pr_count": 1,
                    "fresh_pr_link_count": 1,
                    "partial_pr_link_count": 0,
                    "comment_count": 3,
                    "participant_count": 2,
                    "blocker_keyword_count": 1,
                    "captured_at": "2026-06-21T05:29:23+00:00",
                },
                {
                    "snapshot_key": "tpm-ticket-state-snapshot:unresolved",
                    "observed_at": "2026-06-21T05:29:08+00:00",
                    "ticket_key": "FLINK-404",
                    "status": "open",
                    "captured_at": "2026-06-21T05:29:23+00:00",
                },
            ]
        )

        brief.persist_work_item_state_snapshots_to_ontology(
            conn,
            "fixture-source",
            pr_state_snapshots,
            ticket_state_snapshots,
            "2026-06-21T06:00:00+00:00",
        )

        rows = conn.execute(
            """
            select subject_kind, subject_key, pull_request_id, ticket_id, state,
                   observed_at, captured_at, source_updated_at, age_days, stale_days,
                   risk_score, risk_band, forecast_method, source_current_coverage_state,
                   source_current_detail_state, priority, linked_pr_count, comment_count,
                   participant_count, blocker_keyword_count, source_url
              from work_item_state_snapshots
             order by subject_kind
            """
        ).fetchall()
        self.assertEqual(len(rows), 2)
        self.assertEqual(
            rows[0],
            (
                "pull_request",
                "repo/example#72",
                1,
                None,
                "open",
                "2026-06-21T05:29:08+00:00",
                "2026-06-21T05:29:23+00:00",
                "2026-06-20T00:00:00+00:00",
                20.0,
                1.0,
                88.0,
                "critical",
                "heuristic_percentile_rf_rejected",
                "observed",
                "observed",
                None,
                0,
                0,
                0,
                0,
                "https://github.com/repo/example/pull/72",
            ),
        )
        self.assertEqual(
            rows[1],
            (
                "ticket",
                "FLINK-1",
                None,
                2,
                "open",
                "2026-06-21T05:29:08+00:00",
                "2026-06-21T05:29:23+00:00",
                "2026-06-20T01:00:00+00:00",
                None,
                None,
                0.0,
                "unknown",
                None,
                None,
                None,
                "Major",
                1,
                3,
                2,
                1,
                None,
            ),
        )
        evidence_rows = conn.execute(
            """
            select s.subject_kind, s.subject_key, s.evidence_count,
                   e.claim_target_kind, e.claim_field, e.locator_kind, e.locator, e.excerpt
              from work_item_state_snapshots s
              join evidences e on e.id = s.latest_evidence_id
             order by s.subject_kind
            """
        ).fetchall()
        self.assertEqual(len(evidence_rows), 2)
        self.assertEqual(evidence_rows[0][:6], ("pull_request", "repo/example#72", 1, "work_item_state_snapshot", "state", "state_snapshot"))
        self.assertEqual(evidence_rows[1][:6], ("ticket", "FLINK-1", 1, "work_item_state_snapshot", "state", "state_snapshot"))
        self.assertIn("coverage=observed detail=observed", evidence_rows[0][7])
        self.assertIn("ticket FLINK-1 observed state open", evidence_rows[1][7])

    def test_work_item_state_transitions_materialize_resolved_subject_and_snapshots(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_item_state_transition_tables(conn)
        conn.execute(
            """
            insert into pull_requests (id, key, repository, number, title, source_url)
            values (1, 'pr:repo/example:72', 'repo/example', 72, 'Transition PR', 'https://github.com/repo/example/pull/72')
            """
        )
        conn.executemany(
            """
            insert into work_item_state_snapshots (
              id, key, subject_kind, subject_key, pull_request_id, ticket_id, state,
              observed_at, captured_at, risk_score, risk_band, source_system,
              source_instance, external_kind, external_id
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            [
                (
                    10,
                    "work-item-state-snapshot:test:from",
                    "pull_request",
                    "repo/example#72",
                    1,
                    None,
                    "open",
                    "2026-06-20T05:00:00+00:00",
                    "2026-06-20T05:01:00+00:00",
                    0.0,
                    "unknown",
                    "cubicle_analytics",
                    "fixture-source",
                    "tpm_pr_state_snapshot",
                    "from",
                ),
                (
                    11,
                    "work-item-state-snapshot:test:to",
                    "pull_request",
                    "repo/example#72",
                    1,
                    None,
                    "merged",
                    "2026-06-21T05:00:00+00:00",
                    "2026-06-21T05:01:00+00:00",
                    0.0,
                    "unknown",
                    "cubicle_analytics",
                    "fixture-source",
                    "tpm_pr_state_snapshot",
                    "to",
                ),
            ],
        )
        transition_candidates = pd.DataFrame(
            [
                {
                    "transition_key": "tpm-transition:resolved",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#72",
                    "from_observed_at": "2026-06-20T05:00:00+00:00",
                    "to_observed_at": "2026-06-21T05:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                    "note": "Derived from adjacent snapshots.",
                    "created_at": "2026-06-21T05:02:00+00:00",
                    "updated_at": "2026-06-21T05:03:00+00:00",
                },
                {
                    "transition_key": "tpm-transition:unresolved",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#999",
                    "from_observed_at": "2026-06-20T05:00:00+00:00",
                    "to_observed_at": "2026-06-21T05:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                },
            ]
        )

        brief.persist_work_item_state_transitions_to_ontology(
            conn,
            "fixture-source",
            transition_candidates,
            "2026-06-21T06:00:00+00:00",
        )

        rows = conn.execute(
            """
            select subject_kind, subject_key, pull_request_id, ticket_id,
                   from_snapshot_id, to_snapshot_id, from_observed_at,
                   to_observed_at, from_state, to_state, transition_kind,
                   transition_confidence, confidence_basis, verification_state,
                   terminal, requires_closeout, note,
                   external_id, rank_score
              from work_item_state_transitions
            """
        ).fetchall()
        self.assertEqual(
            rows,
            [
                (
                    "pull_request",
                    "repo/example#72",
                    1,
                    None,
                    10,
                    11,
                    "2026-06-20T05:00:00+00:00",
                    "2026-06-21T05:00:00+00:00",
                    "open",
                    "merged",
                    "terminal_state_change",
                    0.95,
                    "adjacent_snapshot_detection",
                    "closeout_required",
                    1,
                    1,
                    "Derived from adjacent snapshots.",
                    "tpm-transition:resolved",
                    95.0,
                )
            ],
        )
        evidence_row = conn.execute(
            """
            select t.evidence_count, e.claim_target_kind, e.claim_field,
                   e.locator_kind, e.locator, e.excerpt
              from work_item_state_transitions t
              join evidences e on e.id = t.latest_evidence_id
            """
        ).fetchone()
        self.assertIsNotNone(evidence_row)
        self.assertEqual(evidence_row[:5], (1, "work_item_state_transition", "transition_kind", "state_transition", "tpm-transition:resolved"))
        self.assertIn("repo/example#72: open -> merged", evidence_row[5])
        self.assertIn("Derived from adjacent snapshots.", evidence_row[5])

    def test_work_item_state_transitions_refresh_deletes_stale_generated_transitions(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_item_state_transition_tables(conn)
        conn.execute(
            """
            insert into pull_requests (id, key, repository, number, title, source_url)
            values (1, 'pr:repo/example:72', 'repo/example', 72, 'Transition PR', 'https://github.com/repo/example/pull/72')
            """
        )
        conn.executemany(
            """
            insert into work_item_state_snapshots (
              id, key, subject_kind, subject_key, pull_request_id, ticket_id, state,
              observed_at, captured_at, risk_score, risk_band, source_system,
              source_instance, external_kind, external_id
            ) values (?, ?, 'pull_request', 'repo/example#72', 1, null, ?, ?, ?, 0.0,
              'unknown', 'cubicle_analytics', 'fixture-source', 'tpm_pr_state_snapshot', ?)
            """,
            [
                (10, "work-item-state-snapshot:test:from", "open", "2026-06-20T05:00:00+00:00", "2026-06-20T05:01:00+00:00", "from"),
                (11, "work-item-state-snapshot:test:to", "merged", "2026-06-22T05:00:00+00:00", "2026-06-22T05:01:00+00:00", "to"),
            ],
        )
        conn.execute(
            """
            insert into work_item_state_transitions (
              key, subject_kind, subject_key, source_system, source_instance,
              external_kind, external_id, transition_kind, from_state, to_state
            ) values (
              'work-item-state-transition:stale', 'pull_request', 'repo/example#72',
              'cubicle_analytics', 'fixture-source', 'tpm_state_transition_candidate',
              'tpm-transition:stale', 'terminal_state_change', 'open', 'merged'
            )
            """
        )
        transition_candidates = pd.DataFrame(
            [
                {
                    "transition_key": "tpm-transition:current",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#72",
                    "from_observed_at": "2026-06-20T05:00:00+00:00",
                    "to_observed_at": "2026-06-22T05:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                }
            ]
        )

        brief.persist_work_item_state_transitions_to_ontology(
            conn,
            "fixture-source",
            transition_candidates,
            "2026-06-22T06:00:00+00:00",
        )

        rows = conn.execute("select external_id from work_item_state_transitions").fetchall()
        self.assertEqual(rows, [("tpm-transition:current",)])

    def test_work_item_state_transitions_empty_refresh_deletes_stale_generated_transitions(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_item_state_transition_tables(conn)
        conn.execute(
            """
            insert into work_item_state_transitions (
              key, subject_kind, subject_key, source_system, source_instance,
              external_kind, external_id, transition_kind, from_state, to_state
            ) values (
              'work-item-state-transition:stale', 'pull_request', 'repo/example#72',
              'cubicle_analytics', 'fixture-source', 'tpm_state_transition_candidate',
              'tpm-transition:stale', 'terminal_state_change', 'open', 'merged'
            )
            """
        )

        brief.persist_work_item_state_transitions_to_ontology(
            conn,
            "fixture-source",
            pd.DataFrame(),
            "2026-06-22T06:00:00+00:00",
        )

        self.assertEqual(conn.execute("select count(*) from work_item_state_transitions").fetchone()[0], 0)

    def test_work_item_state_transitions_source_scope_and_verified_rows_win(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_item_state_transition_tables(conn)
        conn.executemany(
            """
            insert into pull_requests (id, key, repository, number, title, source_url)
            values (?, ?, 'repo/example', ?, ?, ?)
            """,
            [
                (1, "pr:repo/example:72", 72, "Current source PR", "https://github.com/repo/example/pull/72"),
                (2, "pr:repo/example:73", 73, "Other source PR", "https://github.com/repo/example/pull/73"),
            ],
        )
        conn.executemany(
            """
            insert into work_item_state_snapshots (
              id, key, subject_kind, subject_key, pull_request_id, ticket_id, state,
              observed_at, captured_at, risk_score, risk_band, source_system,
              source_instance, external_kind, external_id
            ) values (?, ?, 'pull_request', ?, ?, null, ?, ?, ?, 0.0,
              'unknown', 'cubicle_analytics', ?, 'tpm_pr_state_snapshot', ?)
            """,
            [
                (10, "snapshot:72:from", "repo/example#72", 1, "open", "2026-06-20T05:00:00+00:00", "2026-06-20T05:01:00+00:00", "fixture-source", "72-from"),
                (11, "snapshot:72:to", "repo/example#72", 1, "merged", "2026-06-22T05:00:00+00:00", "2026-06-22T05:01:00+00:00", "fixture-source", "72-to"),
                (12, "snapshot:73:from", "repo/example#73", 2, "open", "2026-06-20T05:00:00+00:00", "2026-06-20T05:01:00+00:00", "other-source", "73-from"),
                (13, "snapshot:73:to", "repo/example#73", 2, "merged", "2026-06-22T05:00:00+00:00", "2026-06-22T05:01:00+00:00", "other-source", "73-to"),
            ],
        )
        current_key = f"work-item-state-transition:cubicle-analytics:fixture-source:{brief.stable_digest(['tpm-transition:current'])}"
        conn.execute(
            """
            insert into work_item_state_transitions (
              key, subject_kind, subject_key, source_system, source_instance,
              external_kind, external_id, transition_kind, from_state, to_state,
              verification_state, confidence_basis, requires_closeout, latest_evidence_id,
              updated_at
            ) values (
              ?, 'pull_request', 'repo/example#72', 'cubicle_analytics',
              'fixture-source', 'tpm_state_transition_candidate',
              'tpm-transition:current', 'terminal_state_change', 'open', 'merged',
              'human_verified', 'human_verified', 0, 123,
              '2026-06-21T02:00:00+00:00'
            )
            """,
            (current_key,),
        )
        conn.execute(
            """
            insert into work_item_state_transitions (
              key, subject_kind, subject_key, source_system, source_instance,
              external_kind, external_id, transition_kind, from_state, to_state,
              verification_state
            ) values (
              'work-item-state-transition:verified-stale', 'pull_request',
              'repo/example#74', 'cubicle_analytics', 'fixture-source',
              'tpm_state_transition_candidate', 'tpm-transition:verified-stale',
              'terminal_state_change', 'open', 'merged', 'human_verified'
            )
            """
        )
        transition_candidates = pd.DataFrame(
            [
                {
                    "transition_key": "tpm-transition:current",
                    "source_instance": "fixture-source",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#72",
                    "from_observed_at": "2026-06-20T05:00:00+00:00",
                    "to_observed_at": "2026-06-22T05:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                },
                {
                    "transition_key": "tpm-transition:other-source",
                    "source_instance": "other-source",
                    "subject_kind": "pull_request",
                    "subject_key": "repo/example#73",
                    "from_observed_at": "2026-06-20T05:00:00+00:00",
                    "to_observed_at": "2026-06-22T05:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                },
            ]
        )

        brief.persist_work_item_state_transitions_to_ontology(
            conn,
            "fixture-source",
            transition_candidates,
            "2026-06-22T06:00:00+00:00",
        )

        rows = conn.execute(
            """
            select external_id, verification_state, confidence_basis, requires_closeout,
                   latest_evidence_id, updated_at
              from work_item_state_transitions
             order by external_id
            """
        ).fetchall()
        self.assertEqual(
            rows,
            [
                ("tpm-transition:current", "human_verified", "human_verified", 0, 123, "2026-06-22T06:00:00+00:00"),
                ("tpm-transition:verified-stale", "human_verified", None, None, None, None),
            ],
        )

    def test_terminal_transition_does_not_duplicate_existing_closeout_action(self) -> None:
        action_items = pd.DataFrame(
            [
                {
                    "action_key": "tpm-action:existing-closeout",
                    "priority_rank": 1,
                    "urgency": "medium",
                    "priority_score": 65.0,
                    "raw_priority_score": 65.0,
                    "action_type": "verify_resolution",
                    "decision_state": "closeout_review",
                    "decision_gate_reason": "observed terminal state change still needs closeout confirmation",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1085",
                    "insight_kinds": "status_summary",
                    "source_insight_keys": "",
                    "source_link_insight_kinds": "status_summary",
                    "severity": "medium",
                    "severity_rank": 3,
                    "status_signal": "terminal_transition_observed",
                    "baseline_state": "open",
                    "current_state": "merged",
                    "source_observation_status": "observed",
                    "source_auth_state": "",
                    "source_coverage_kind": "authenticated_api_current_observation",
                    "title": "Verify resolved follow-up: apache/flink-kubernetes-operator#1085",
                    "why_now": "Observed open -> merged transition.",
                    "recommended_action": "Confirm closeout.",
                    "owner_hint": "github:Dennis-Mircea",
                    "source_url": "https://github.com/apache/flink-kubernetes-operator/pull/1085",
                    "evidence_ref": "existing",
                    "score": 65.0,
                    "confidence": 0.8,
                    "needs_human_review": "true",
                    "open_review_request_count": 0,
                    "reviewed_count": 0,
                    "evidence_summary": "existing closeout evidence",
                    "generated_at": "2026-06-21T00:00:00+00:00",
                }
            ]
        )
        transition_candidates = pd.DataFrame(
            [
                {
                    "transition_key": "tpm-transition:test",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1085",
                    "from_observed_at": "2026-06-18T00:00:00+00:00",
                    "to_observed_at": "2026-06-21T00:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                    "note": "Observed merged.",
                }
            ]
        )
        pr_features = pd.DataFrame(
            [
                {
                    "repository": "apache/flink-kubernetes-operator",
                    "pr_number": 1085,
                    "pr_url": "https://github.com/apache/flink-kubernetes-operator/pull/1085",
                    "title": "Example",
                    "author_login": "Dennis-Mircea",
                }
            ]
        )

        combined = brief.append_transition_resolution_actions(
            action_items,
            transition_candidates,
            pr_features,
            "2026-06-21T00:00:00+00:00",
        )
        combined = brief.apply_transition_evidence_to_resolution_actions(combined, transition_candidates)

        closeouts = combined[
            (combined["subject_key"] == "apache/flink-kubernetes-operator#1085")
            & (combined["action_type"] == "verify_resolution")
        ]
        self.assertEqual(len(closeouts), 1)
        self.assertEqual(closeouts.iloc[0]["action_key"], "tpm-action:existing-closeout")
        self.assertEqual(closeouts.iloc[0]["evidence_ref"], "state_transition tpm-transition:test")
        self.assertEqual(closeouts.iloc[0]["source_insight_keys"], "tpm-transition:test")
        self.assertEqual(closeouts.iloc[0]["source_link_insight_kinds"], "state_transition")
        self.assertEqual(closeouts.iloc[0]["source_coverage_kind"], "time_series_transition")
        self.assertIn("state_transition: open -> merged", closeouts.iloc[0]["evidence_summary"])

    def test_terminal_transition_candidates_create_one_closeout_per_subject(self) -> None:
        transition_candidates = pd.DataFrame(
            [
                {
                    "transition_key": "tpm-transition:older",
                    "source_instance": "fixture-source",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1085",
                    "from_observed_at": "2026-06-18T00:00:00+00:00",
                    "to_observed_at": "2026-06-20T00:00:00+00:00",
                    "from_state": "open",
                    "to_state": "closed",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.8,
                    "note": "Older terminal observation.",
                },
                {
                    "transition_key": "tpm-transition:newer",
                    "source_instance": "fixture-source",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1085",
                    "from_observed_at": "2026-06-18T00:00:00+00:00",
                    "to_observed_at": "2026-06-21T00:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                    "note": "Newest terminal observation.",
                },
                {
                    "transition_key": "tpm-transition:other-source",
                    "source_instance": "other-source",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1086",
                    "from_observed_at": "2026-06-18T00:00:00+00:00",
                    "to_observed_at": "2026-06-21T00:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                    "note": "Different source.",
                },
            ]
        )
        pr_features = pd.DataFrame(
            [
                {
                    "repository": "apache/flink-kubernetes-operator",
                    "pr_number": 1085,
                    "pr_url": "https://github.com/apache/flink-kubernetes-operator/pull/1085",
                    "title": "Example",
                    "author_login": "Dennis-Mircea",
                }
            ]
        )

        combined = brief.append_transition_resolution_actions(
            pd.DataFrame(),
            transition_candidates,
            pr_features,
            "2026-06-21T00:00:00+00:00",
            "fixture-source",
        )
        combined = brief.apply_transition_evidence_to_resolution_actions(combined, transition_candidates, "fixture-source")

        closeouts = combined[combined["action_type"] == "verify_resolution"]
        self.assertEqual(len(closeouts), 1)
        self.assertEqual(closeouts.iloc[0]["subject_key"], "apache/flink-kubernetes-operator#1085")
        self.assertEqual(closeouts.iloc[0]["source_insight_keys"], "tpm-transition:newer")
        self.assertEqual(closeouts.iloc[0]["current_state"], "merged")

    def test_superseded_terminal_transition_does_not_create_closeout(self) -> None:
        transition_candidates = pd.DataFrame(
            [
                {
                    "transition_key": "tpm-transition:terminal",
                    "source_instance": "fixture-source",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1085",
                    "from_observed_at": "2026-06-18T00:00:00+00:00",
                    "to_observed_at": "2026-06-20T00:00:00+00:00",
                    "from_state": "open",
                    "to_state": "merged",
                    "transition_kind": "terminal_state_change",
                    "confidence": 0.95,
                    "note": "Terminal observation.",
                },
                {
                    "transition_key": "tpm-transition:reopened",
                    "source_instance": "fixture-source",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1085",
                    "from_observed_at": "2026-06-20T00:00:00+00:00",
                    "to_observed_at": "2026-06-21T00:00:00+00:00",
                    "from_state": "merged",
                    "to_state": "open",
                    "transition_kind": "state_change",
                    "confidence": 0.95,
                    "note": "Later non-terminal observation.",
                },
            ]
        )

        combined = brief.append_transition_resolution_actions(
            pd.DataFrame(),
            transition_candidates,
            pd.DataFrame(),
            "2026-06-21T00:00:00+00:00",
            "fixture-source",
        )

        self.assertTrue(combined.empty)

    def test_authenticated_terminal_closeout_is_source_resolved(self) -> None:
        action_items = pd.DataFrame(
            [
                {
                    **empty_action_item_row("apache/flink-kubernetes-operator#1085"),
                    "action_key": "tpm-action:source-resolved",
                    "urgency": "medium",
                    "priority_score": 65.0,
                    "raw_priority_score": 65.0,
                    "action_type": "verify_resolution",
                    "decision_state": "closeout_review",
                    "decision_gate_reason": "observed terminal state change still needs closeout confirmation",
                    "current_state": "merged",
                    "source_observation_status": "observed",
                    "source_auth_state": "github_token",
                    "source_coverage_kind": "time_series_transition",
                    "needs_human_review": "true",
                    "recommended_action": "Confirm closeout.",
                }
            ],
            columns=brief.empty_action_items().columns,
        )

        resolved = brief.apply_source_resolved_closeouts(
            action_items,
            {("pull_request", "apache/flink-kubernetes-operator#1085"): "merged"},
        )
        action = resolved.iloc[0]
        self.assertEqual(action["decision_state"], "source_resolved")
        self.assertEqual(action["needs_human_review"], "false")
        self.assertEqual(action["urgency"], "low")
        self.assertEqual(action["priority_score"], 20.0)
        self.assertIn("source currently reports merged", action["recommended_action"])

        work_actions = brief.build_work_actions(resolved, "2026-06-22T00:00:00+00:00")
        work_action = work_actions.iloc[0]
        self.assertEqual(work_action["action_state"], "closed")
        self.assertEqual(work_action["decision"], "source_confirmed_resolved")
        self.assertEqual(work_action["due_bucket"], "watch")
        self.assertEqual(work_action["closed_at"], "2026-06-22T00:00:00+00:00")

        observations = brief.build_work_action_observations(resolved, "2026-06-22T00:00:00+00:00")
        self.assertEqual(observations.iloc[0]["observation_kind"], "closeout_review")
        self.assertEqual(observations.iloc[0]["supports_action"], "false")

    def test_source_resolved_closeout_requires_latest_state_to_still_be_terminal(self) -> None:
        action_items = pd.DataFrame(
            [
                {
                    **empty_action_item_row("apache/flink-kubernetes-operator#1085"),
                    "action_key": "tpm-action:source-resolved",
                    "urgency": "medium",
                    "priority_score": 65.0,
                    "raw_priority_score": 65.0,
                    "action_type": "verify_resolution",
                    "decision_state": "closeout_review",
                    "decision_gate_reason": "observed terminal state change still needs closeout confirmation",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1085",
                    "current_state": "merged",
                    "source_observation_status": "observed",
                    "source_auth_state": "github_token",
                    "source_coverage_kind": "time_series_transition",
                    "needs_human_review": "true",
                    "recommended_action": "Confirm closeout.",
                }
            ],
            columns=brief.empty_action_items().columns,
        )

        resolved = brief.apply_source_resolved_closeouts(
            action_items,
            {("pull_request", "apache/flink-kubernetes-operator#1085"): "open"},
        )

        action = resolved.iloc[0]
        self.assertEqual(action["decision_state"], "closeout_review")
        self.assertEqual(action["needs_human_review"], "true")
        self.assertEqual(action["recommended_action"], "Confirm closeout.")

        missing_latest = brief.apply_source_resolved_closeouts(action_items, {})
        missing_action = missing_latest.iloc[0]
        self.assertEqual(missing_action["decision_state"], "closeout_review")
        self.assertEqual(missing_action["needs_human_review"], "true")

    def test_source_resolved_closeout_requires_authenticated_terminal_source(self) -> None:
        rows = [
            {
                **empty_action_item_row("repo/example#1"),
                "action_key": "anonymous",
                "action_type": "verify_resolution",
                "decision_state": "closeout_review",
                "current_state": "merged",
                "source_observation_status": "observed",
                "source_auth_state": "anonymous",
                "source_coverage_kind": "time_series_transition",
                "needs_human_review": "true",
            },
            {
                **empty_action_item_row("repo/example#2"),
                "action_key": "non-terminal",
                "action_type": "verify_resolution",
                "decision_state": "closeout_review",
                "current_state": "open",
                "source_observation_status": "observed",
                "source_auth_state": "github_token",
                "source_coverage_kind": "authenticated_api_current_observation",
                "needs_human_review": "true",
            },
        ]

        resolved = brief.apply_source_resolved_closeouts(
            pd.DataFrame(rows, columns=brief.empty_action_items().columns),
            {("pull_request", "repo/example#1"): "merged", ("pull_request", "repo/example#2"): "open"},
        )

        self.assertEqual(resolved["decision_state"].tolist(), ["closeout_review", "closeout_review"])
        self.assertEqual(resolved["needs_human_review"].tolist(), ["true", "true"])
        work_actions = brief.build_work_actions(resolved, "2026-06-22T00:00:00+00:00")
        self.assertEqual(work_actions["action_state"].tolist(), ["open", "open"])
        observations = brief.build_work_action_observations(resolved, "2026-06-22T00:00:00+00:00")
        self.assertEqual(observations["observation_kind"].tolist(), ["closeout_review", "closeout_review"])
        self.assertEqual(observations["supports_action"].tolist(), ["false", "false"])

    def test_workstream_register_materializes_typed_workstream_ticket_membership(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.executemany(
            "insert into tickets (id, key, external_id, title) values (?, ?, ?, ?)",
            [
                (1, "ticket:FLINK-1", "FLINK-1", "One"),
                (2, "ticket:FLINK-2", "FLINK-2", "Two"),
            ],
        )
        program_register = pd.DataFrame(
            [
                {
                    "workstream_key": "flink-kubernetes-operator",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#1",
                    "linked_ticket_keys": "FLINK-2",
                    "decision_state": "product_action",
                    "risk_score": 90,
                    "title": "Clear blocker",
                }
            ]
        )
        ticket_features = pd.DataFrame([{"ticket_key": "FLINK-1"}])

        brief.persist_workstream_register_to_ontology(
            conn,
            "fixture-source",
            program_register,
            ticket_features,
            "2026-06-21T03:00:00+00:00",
        )
        brief.persist_workstream_register_to_ontology(
            conn,
            "fixture-source",
            program_register,
            ticket_features,
            "2026-06-21T04:00:00+00:00",
        )

        workstreams = conn.execute("select key, title, status, event_count, rank_score from workstreams").fetchall()
        self.assertEqual(workstreams, [("workstream:flink-kubernetes-operator", "Flink Kubernetes Operator", "active", 1, 90.0)])
        links = conn.execute(
            """
            select wt.workstream_ticket_kind, t.external_id, wt.source_instance
            from workstream_tickets wt
            join tickets t on t.id = wt.ticket_id
            order by t.external_id
            """
        ).fetchall()
        self.assertEqual(
            links,
            [
                ("contains", "FLINK-1", "fixture-source"),
                ("contains", "FLINK-2", "fixture-source"),
            ],
        )

    def test_program_register_materializes_typed_program_item_with_evidence(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.execute(
            """
            insert into workstreams (
              id, key, title, status, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'active', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            """
            insert into pull_requests (id, key, repository, number, title)
            values (2, 'pr:apache/flink-kubernetes-operator#72', 'apache/flink-kubernetes-operator', 72, 'Improve autoscaler')
            """
        )
        conn.execute(
            """
            insert into work_actions (
              id, key, source_system, source_instance, external_kind, external_id
            ) values (3, 'tpm-action:test:program', 'cubicle_analytics', 'fixture-source', 'tpm_work_action', 'tpm-action:test:program')
            """
        )
        program_register = pd.DataFrame(
            [
                {
                    "program_key": "tpm-program:test",
                    "action_key": "tpm-action:test:program",
                    "workstream_key": "flink-kubernetes-operator",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#72",
                    "linked_ticket_keys": "FLINK-1",
                    "linked_pr_keys": "apache/flink-kubernetes-operator#73",
                    "title": "Improve autoscaler",
                    "program_status": "needs_decision",
                    "tpm_bucket": "risk",
                    "owner_key": "github:owner",
                    "owner_source": "pr_author",
                    "author_dri": "github:owner",
                    "requested_reviewer_keys": "github:reviewer",
                    "reviewer_or_approver": "github:reviewer",
                    "next_action": "Ask owner whether to merge or park.",
                    "decision_needed": "merge / close / park / assign owner",
                    "decision_state": "product_action",
                    "decision_gate_reason": "gold labels support escalation",
                    "due_bucket": "now",
                    "last_source_update_at": "2026-06-20T04:00:00+00:00",
                    "age_days": "4.5",
                    "stale_days": "1.25",
                    "risk_score": "91.5",
                    "blocker_label_state": "not_required",
                    "ci_signal": "required_failing_or_pending",
                    "transition_state": "open",
                    "dependency_summary": "1 linked ticket(s)",
                    "source_coverage_state": "observed:github_pr",
                    "evidence_ref": "github_pr apache/flink-kubernetes-operator#72 https://github.com/apache/flink-kubernetes-operator/pull/72",
                    "label_quality": "gold",
                    "updated_at": "2026-06-21T04:00:00+00:00",
                }
            ]
        )

        brief.persist_work_program_items_to_ontology(
            conn,
            "fixture-source",
            program_register,
            "2026-06-21T04:00:00+00:00",
        )
        program_register.loc[0, "program_key"] = "tpm-program:test:rerouted"
        program_register.loc[0, "next_action"] = "Updated TPM follow-up."
        brief.persist_work_program_items_to_ontology(
            conn,
            "fixture-source",
            program_register,
            "2026-06-21T05:00:00+00:00",
        )

        row = conn.execute(
            """
            select p.workstream_id, p.work_action_id, p.pull_request_id, p.ticket_id,
                   p.workstream_key, p.subject_kind, p.subject_key,
                   p.linked_ticket_keys, p.linked_pull_request_keys,
                   p.program_status, p.tpm_bucket, p.owner_key, p.next_action,
                   p.decision_state, p.due_bucket, p.risk_score, p.source_instance,
                   p.source_url, p.evidence_count,
                   e.claim_target_kind, e.locator_kind, e.excerpt
              from work_program_items p
              join evidences e on e.id = p.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(
            row,
            (
                1,
                3,
                2,
                None,
                "flink-kubernetes-operator",
                "pull_request",
                "apache/flink-kubernetes-operator#72",
                "FLINK-1",
                "apache/flink-kubernetes-operator#73",
                "needs_decision",
                "risk",
                "github:owner",
                "Updated TPM follow-up.",
                "product_action",
                "now",
                91.5,
                "fixture-source",
                "https://github.com/apache/flink-kubernetes-operator/pull/72",
                1,
                "work_program_item",
                "tpm_program_register",
                "pull_request apache/flink-kubernetes-operator#72 in needs_decision/risk: Updated TPM follow-up.",
            ),
        )
        self.assertEqual(conn.execute("select count(*) from work_program_items").fetchone()[0], 1)

    def test_program_register_empty_refresh_deletes_stale_generated_items_for_source(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.executemany(
            """
            insert into work_program_items (
              key, source_system, source_instance, external_kind, subject_kind, subject_key,
              workstream_key, title, program_status, tpm_bucket, decision_state, due_bucket
            ) values (?, 'cubicle_analytics', ?, 'tpm_program_item', 'pull_request', ?,
              'flink-kubernetes-operator', ?, 'needs_decision', 'risk', 'validation_lead', 'now')
            """,
            [
                ("stale:same-source", "fixture-source", "apache/flink-kubernetes-operator#72", "Stale same-source item"),
                ("keep:other-source", "other-source", "apache/flink-kubernetes-operator#73", "Other source item"),
            ],
        )

        brief.persist_work_program_items_to_ontology(
            conn,
            "fixture-source",
            pd.DataFrame(),
            "2026-06-21T06:00:00+00:00",
        )

        rows = conn.execute("select key, source_instance from work_program_items order by key").fetchall()
        self.assertEqual(rows, [("keep:other-source", "other-source")])

    def test_program_milestones_materialize_source_signals_without_due_bucket_commitments(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_work_program_milestone_tables(conn)
        conn.execute(
            """
            insert into workstreams (
              id, key, title, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            "insert into tickets (id, key, external_id, title, source_url) values (2, 'ticket:FLINK-1', 'FLINK-1', 'Fix release target', 'https://issues.apache.org/jira/browse/FLINK-1')"
        )
        signals = pd.DataFrame(
            [
                {
                    "subject_kind": "ticket",
                    "subject_key": "FLINK-1",
                    "milestone_kind": "release_target",
                    "milestone_name": "2.4.0",
                    "target_date": "2026-07-01T00:00:00+00:00",
                    "outcome_date": "",
                    "milestone_state": "planned",
                    "commitment_strength": "release_signal",
                    "date_claim_allowed": 1,
                    "delivery_commitment_allowed": 0,
                    "claim_gate_reason": "source_release_target_not_owner_commitment",
                    "source_field": "jira.fields.fixVersions.releaseDate",
                    "source_payload_key": "FLINK-1:fields.fixVersions:2.4.0",
                    "source_url": "https://issues.apache.org/jira/browse/FLINK-1",
                    "captured_at": "2026-06-22T00:00:00+00:00",
                    "external_id": "milestone|FLINK-1|2.4.0",
                    "rank_score": 75,
                    "due_bucket": "now",
                },
                {
                    "subject_kind": "ticket",
                    "subject_key": "FLINK-1",
                    "milestone_kind": "explicit_due_date",
                    "milestone_name": "Jira due date",
                    "target_date": "2026-06-30T00:00:00+00:00",
                    "outcome_date": "",
                    "milestone_state": "planned",
                    "commitment_strength": "explicit_commitment",
                    "date_claim_allowed": 1,
                    "delivery_commitment_allowed": 1,
                    "claim_gate_reason": "source_native_due_date",
                    "source_field": "jira.fields.duedate",
                    "source_payload_key": "FLINK-1:fields.duedate",
                    "source_url": "https://issues.apache.org/jira/browse/FLINK-1",
                    "captured_at": "2026-06-22T00:00:00+00:00",
                    "external_id": "milestone|FLINK-1|duedate",
                    "rank_score": 100,
                    "due_bucket": "now",
                },
            ]
        )

        brief.persist_work_program_milestones_to_ontology(conn, "fixture-source", signals, "2026-06-23T00:00:00+00:00")
        brief.persist_work_program_milestones_to_ontology(conn, "fixture-source", signals, "2026-06-23T00:00:00+00:00")

        rows = conn.execute(
            """
            select milestone_kind, milestone_name, commitment_strength,
                   date_claim_allowed, delivery_commitment_allowed, evidence_count
              from work_program_milestones
             order by milestone_kind
            """
        ).fetchall()
        self.assertEqual(
            rows,
            [
                ("explicit_due_date", "Jira due date", "explicit_commitment", 1, 1, 1),
                ("release_target", "2.4.0", "release_signal", 1, 0, 1),
            ],
        )
        self.assertEqual(conn.execute("select count(*) from evidences").fetchone()[0], 2)

    def test_program_register_rejects_duplicate_current_items_for_one_action(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.execute(
            """
            insert into work_program_items (
              key, source_system, source_instance, external_kind, subject_kind, subject_key,
              workstream_key, title, program_status, tpm_bucket, decision_state, due_bucket
            ) values ('existing:item', 'cubicle_analytics', 'fixture-source', 'tpm_program_item',
              'pull_request', 'apache/flink-kubernetes-operator#72', 'flink-kubernetes-operator',
              'Existing item', 'needs_decision', 'risk', 'validation_lead', 'now')
            """
        )
        conn.execute(
            """
            insert into pull_requests (id, key, repository, number, title)
            values (2, 'pr:apache/flink-kubernetes-operator#72', 'apache/flink-kubernetes-operator', 72, 'Improve autoscaler')
            """
        )
        conn.execute(
            """
            insert into work_actions (
              id, key, source_system, source_instance, external_kind, external_id
            ) values (3, 'tpm-action:test:program', 'cubicle_analytics', 'fixture-source', 'tpm_work_action', 'tpm-action:test:program')
            """
        )
        program_register = pd.DataFrame(
            [
                {
                    "program_key": "tpm-program:test:a",
                    "action_key": "tpm-action:test:program",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#72",
                },
                {
                    "program_key": "tpm-program:test:b",
                    "action_key": "tpm-action:test:program",
                    "subject_kind": "pull_request",
                    "subject_key": "apache/flink-kubernetes-operator#72",
                },
            ]
        )

        with self.assertRaisesRegex(RuntimeError, "maps one WorkAction to multiple current WorkProgramItems"):
            brief.persist_work_program_items_to_ontology(
                conn,
                "fixture-source",
                program_register,
                "2026-06-21T06:00:00+00:00",
            )

        self.assertEqual(conn.execute("select key from work_program_items").fetchall(), [("existing:item",)])

    def test_workstream_health_snapshot_materializes_operating_rollup_with_evidence(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.execute(
            """
            insert into workstreams (
              id, key, title, status, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'active', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        standup_summary = pd.DataFrame(
            [
                {
                    "generated_at": "2026-06-21T07:03:16+00:00",
                    "workstream_key": "flink-kubernetes-operator",
                    "operating_status": "attention_required",
                    "action_item_count": 19,
                    "open_work_count": 4,
                    "validation_lead_count": 11,
                    "critical_or_high_validation_lead_count": 8,
                    "model_or_rule_qa_count": 1,
                    "closeout_review_count": 1,
                    "owner_count": 13,
                    "top_owner_action_count": 2,
                    "failing_check_pr_count": 2,
                    "open_failing_check_pr_count": 2,
                    "source_repair_count": 0,
                    "coverage_limited_count": 0,
                    "anonymous_observation_count": 2,
                    "terminal_transition_count": 1,
                    "terminal_transition_subjects": "apache/flink-kubernetes-operator#1085",
                    "eta_forecast_ready": "false",
                    "truth_label_coverage": "10/23",
                    "actionability_label_coverage": "10/23",
                    "recommended_cadence_focus": "triage 4 product-safe actions; validate 8 urgent leads",
                }
            ]
        )

        brief.persist_workstream_health_snapshots_to_ontology(
            conn,
            "fixture-source",
            standup_summary,
            "2026-06-21T07:03:16+00:00",
        )

        row = conn.execute(
            """
            select h.workstream_id, h.workstream_key, h.operating_status,
                   h.action_item_count, h.product_action_count,
                   h.validation_lead_count, h.closeout_review_count,
                   h.terminal_transition_count, h.eta_forecast_ready,
                   h.truth_label_coverage, h.freshness_state, h.confidence, h.evidence_count,
                   e.claim_target_kind, e.locator_kind, e.excerpt
              from workstream_health_snapshots h
              join evidences e on e.id = h.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(
            row,
            (
                1,
                "flink-kubernetes-operator",
                "attention_required",
                19,
                4,
                11,
                1,
                1,
                0,
                "10/23",
                "partial",
                0.85,
                1,
                "workstream_health_snapshot",
                "workstream_standup",
                "triage 4 product-safe actions; validate 8 urgent leads",
            ),
        )

    def test_workstream_standup_does_not_mark_missing_inputs_clear(self) -> None:
        standup_summary = brief.build_workstream_standup(
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            "2026-06-21T07:03:16+00:00",
        )

        self.assertEqual(standup_summary.iloc[0]["operating_status"], "unknown")

    def test_workstream_standup_requires_clean_observed_inputs_for_clear(self) -> None:
        summary = pd.DataFrame(
            [
                {"metric": "action_item_count", "value": "0"},
                {"metric": "critical_or_high_count", "value": "0"},
                {"metric": "validation_lead_count", "value": "0"},
                {"metric": "critical_or_high_validation_lead_count", "value": "0"},
                {"metric": "model_or_rule_qa_count", "value": "0"},
                {"metric": "source_repair_count", "value": "0"},
                {"metric": "coverage_limited_count", "value": "0"},
                {"metric": "anonymous_observation_count", "value": "0"},
            ]
        )
        readiness = pd.DataFrame(
            [
                {"metric": "truth_label_coverage", "value": "10/10"},
                {"metric": "actionability_label_coverage", "value": "10/10"},
            ]
        )

        standup_summary = brief.build_workstream_standup(
            pd.DataFrame(),
            pd.DataFrame(),
            summary,
            readiness,
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            "2026-06-21T07:03:16+00:00",
        )

        self.assertEqual(standup_summary.iloc[0]["operating_status"], "clear")

    def test_workstream_standup_keeps_source_gaps_out_of_clear(self) -> None:
        summary = pd.DataFrame(
            [
                {"metric": "action_item_count", "value": "0"},
                {"metric": "critical_or_high_count", "value": "0"},
                {"metric": "validation_lead_count", "value": "0"},
                {"metric": "source_repair_count", "value": "1"},
                {"metric": "coverage_limited_count", "value": "0"},
            ]
        )
        readiness = pd.DataFrame([{"metric": "truth_label_coverage", "value": "10/10"}])

        standup_summary = brief.build_workstream_standup(
            pd.DataFrame(),
            pd.DataFrame(),
            summary,
            readiness,
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            pd.DataFrame(),
            "2026-06-21T07:03:16+00:00",
        )

        self.assertEqual(standup_summary.iloc[0]["operating_status"], "attention_required")

    def test_workstream_standup_sections_materialize_agenda_rows_with_links(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.execute(
            """
            insert into workstreams (
              id, key, title, status, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'active', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            """
            insert into workstream_health_snapshots (
              id, key, workstream_id, workstream_key, generated_at, operating_status,
              source_system, source_instance, external_kind, external_id
            ) values (1, 'health:test', 1, 'flink-kubernetes-operator', '2026-06-21T07:03:16+00:00', 'attention_required',
                      'cubicle_analytics', 'fixture-source', 'tpm_workstream_health_snapshot', 'flink-kubernetes-operator:2026-06-21T07:03:16+00:00')
            """
        )
        conn.execute(
            """
            insert into work_actions (
              id, key, source_url, latest_evidence_id, freshness_state, confidence,
              source_system, source_instance, external_kind, external_id
            ) values (7, 'tpm-action:test:one', 'https://example.test/pr/1', null, 'fresh', 0.91,
                      'cubicle_analytics', 'fixture-source', 'tpm_work_action', 'tpm-action:test:one')
            """
        )
        standup_sections = pd.DataFrame(
            [
                {
                    "generated_at": "2026-06-21T07:03:16+00:00",
                    "workstream_key": "flink-kubernetes-operator",
                    "section_rank": 1,
                    "section_kind": "product_action",
                    "urgency": "critical",
                    "owner_hint": "github:owner",
                    "subject_key": "apache/flink-kubernetes-operator#1079",
                    "action_type": "clear_blocker",
                    "status_signal": "still_open",
                    "summary": "Clear blocker candidate",
                    "recommended_action": "Ask owner for next step.",
                    "evidence_ref": "github_pull_body span https://example.test/pr/1",
                    "action_key": "tpm-action:test:one",
                }
            ]
        )

        brief.persist_workstream_standup_sections_to_ontology(
            conn,
            "fixture-source",
            standup_sections,
            "2026-06-21T07:03:16+00:00",
        )

        row = conn.execute(
            """
            select s.workstream_health_snapshot_id, s.workstream_id, s.work_action_id,
                   s.workstream_key, s.section_rank, s.section_kind, s.urgency,
                   s.subject_kind, s.subject_key, s.summary, s.recommended_action,
                   s.freshness_state, s.confidence, s.evidence_count,
                   e.claim_target_kind, e.locator_kind, e.locator
              from workstream_standup_sections s
              join evidences e on e.id = s.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(
            row,
            (
                1,
                1,
                7,
                "flink-kubernetes-operator",
                1,
                "product_action",
                "critical",
                "pull_request",
                "apache/flink-kubernetes-operator#1079",
                "Clear blocker candidate",
                "Ask owner for next step.",
                "fresh",
                0.91,
                1,
                "workstream_standup_section",
                "github_pull_body",
                "span",
            ),
        )

    def test_workstream_standup_product_sections_require_action_link(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.execute(
            """
            insert into workstreams (
              id, key, title, status, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'active', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            """
            insert into workstream_health_snapshots (
              id, key, workstream_id, workstream_key, generated_at, operating_status,
              source_system, source_instance, external_kind, external_id
            ) values (1, 'health:test', 1, 'flink-kubernetes-operator', '2026-06-21T07:03:16+00:00', 'attention_required',
                      'cubicle_analytics', 'fixture-source', 'tpm_workstream_health_snapshot', 'flink-kubernetes-operator:2026-06-21T07:03:16+00:00')
            """
        )
        standup_sections = pd.DataFrame(
            [
                {
                    "generated_at": "2026-06-21T07:03:16+00:00",
                    "workstream_key": "flink-kubernetes-operator",
                    "section_rank": 1,
                    "section_kind": "product_action",
                    "urgency": "critical",
                    "subject_key": "apache/flink-kubernetes-operator#1079",
                    "action_type": "clear_blocker",
                    "status_signal": "still_open",
                    "summary": "Clear blocker candidate",
                    "recommended_action": "Ask owner for next step.",
                    "evidence_ref": "github_pull_body span https://example.test/pr/1",
                    "action_key": "",
                }
            ]
        )

        with self.assertRaisesRegex(RuntimeError, "cannot link action"):
            brief.persist_workstream_standup_sections_to_ontology(
                conn,
                "fixture-source",
                standup_sections,
                "2026-06-21T07:03:16+00:00",
            )

    def test_work_owner_load_snapshot_materializes_owner_routing_with_evidence(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.execute(
            """
            insert into workstreams (
              id, key, title, status, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'active', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            """
            insert into persons (
              id, key, display_name, github_login
            ) values (3, 'person:github:lrsb', 'lrsb', 'lrsb')
            """
        )
        owner_rollup = pd.DataFrame(
            [
                {
                    "owner_hint": "github:lrsb",
                    "action_count": 2,
                    "product_action_count": 1,
                    "validation_lead_count": 1,
                    "model_or_rule_qa_count": 0,
                    "critical_or_high_count": 1,
                    "max_priority_score": 93.0,
                    "avg_priority_score": 71.5,
                    "decision_followup_count": 0,
                    "validate_signal_count": 1,
                    "ci_check_followup_count": 0,
                    "review_wait_followup_count": 0,
                    "coverage_limited_count": 0,
                    "anonymous_observation_count": 0,
                    "needs_human_review_count": 1,
                    "top_action_type": "clear_blocker",
                    "top_subjects": "apache/flink-kubernetes-operator#1134, apache/flink-kubernetes-operator#1135",
                    "recommended_focus": "Review the generated action and record a truth/actionability label.",
                }
            ]
        )

        brief.persist_work_owner_load_snapshots_to_ontology(
            conn,
            "fixture-source",
            owner_rollup,
            "2026-06-21T07:03:16+00:00",
        )

        row = conn.execute(
            """
            select o.workstream_id, o.person_id, o.owner_key, o.load_status,
                   o.action_count, o.product_action_count, o.validation_lead_count,
                   o.critical_or_high_count, o.needs_human_review_count,
                   o.freshness_state, o.confidence, o.evidence_count,
                   e.claim_target_kind, e.locator_kind, e.confidence
              from work_owner_load_snapshots o
              join evidences e on e.id = o.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(
            row,
            (
                1,
                3,
                "github:lrsb",
                "overloaded",
                2,
                1,
                1,
                1,
                1,
                "fresh",
                1.0,
                1,
                "work_owner_load_snapshot",
                "tpm_owner_action_rollup",
                1.0,
            ),
        )

    def test_work_owner_load_snapshot_does_not_link_bare_display_name(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.execute(
            """
            insert into workstreams (
              id, key, title, status, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'active', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            """
            insert into persons (
              id, key, display_name, github_login
            ) values (4, 'person:jira:swati', 'Swati Gupta', '')
            """
        )
        owner_rollup = pd.DataFrame(
            [
                {
                    "owner_hint": "Swati Gupta",
                    "action_count": 1,
                    "product_action_count": 0,
                    "validation_lead_count": 1,
                    "model_or_rule_qa_count": 0,
                    "critical_or_high_count": 0,
                    "max_priority_score": 65.0,
                    "avg_priority_score": 65.0,
                    "decision_followup_count": 0,
                    "validate_signal_count": 1,
                    "ci_check_followup_count": 0,
                    "review_wait_followup_count": 0,
                    "coverage_limited_count": 0,
                    "anonymous_observation_count": 0,
                    "needs_human_review_count": 1,
                    "top_action_type": "validate_signal",
                    "top_subjects": "FLINK-1",
                    "recommended_focus": "Validate the generated signal.",
                }
            ]
        )

        brief.persist_work_owner_load_snapshots_to_ontology(
            conn,
            "fixture-source",
            owner_rollup,
            "2026-06-21T07:03:16+00:00",
        )

        row = conn.execute(
            """
            select owner_key, person_id, load_status, action_count
              from work_owner_load_snapshots
            """
        ).fetchone()
        self.assertEqual(row, ("Swati Gupta", None, "watch", 1))

    def test_work_owner_load_snapshot_materializes_clear_run_marker(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_workstream_tables(conn)
        conn.execute(
            """
            insert into workstreams (
              id, key, title, status, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'active', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )

        brief.persist_work_owner_load_snapshots_to_ontology(
            conn,
            "fixture-source",
            pd.DataFrame(),
            "2026-06-22T07:03:16+00:00",
        )

        row = conn.execute(
            """
            select o.owner_key, o.load_status, o.action_count, o.freshness_state,
                   o.confidence, o.evidence_count, e.claim_target_kind, e.locator_kind
              from work_owner_load_snapshots o
              join evidences e on e.id = o.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(
            row,
            (
                "(clear)",
                "clear",
                0,
                "fresh",
                1.0,
                1,
                "work_owner_load_snapshot",
                "tpm_owner_action_rollup",
            ),
        )

    def test_work_program_brief_caveats_materialize_with_generated_evidence(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_brief_caveat_tables(conn)
        now = "2026-06-21T07:03:16+00:00"
        conn.execute(
            """
            insert into workstreams (
              id, key, title, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        readiness = pd.DataFrame(
            [
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "false"},
                {"metric": "gated_insight_kind_count", "value": "1"},
                {"metric": "open_review_request_count", "value": "2"},
            ]
        )
        forecast_summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false"},
                {"metric": "merged_pr_count", "value": "7"},
            ]
        )

        brief.persist_work_program_brief_caveats_to_ontology(
            conn,
            "fixture-source",
            readiness,
            forecast_summary,
            now,
        )

        rows = conn.execute(
            """
            select c.caveat_key, c.severity, c.evidence_count, e.claim_target_kind, e.locator_kind
              from work_program_brief_caveats c
              join evidences e on e.id = c.latest_evidence_id
             order by c.rank_score desc, c.caveat_key
            """
        ).fetchall()
        self.assertEqual(
            rows,
            [
                ("forecast_gated", "warning", 1, "work_program_brief_caveat", "tpm_brief_caveat"),
                ("measurement_gated", "warning", 1, "work_program_brief_caveat", "tpm_brief_caveat"),
            ],
        )

    def test_work_program_brief_snapshot_materializes_with_generated_evidence(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_brief_snapshot_tables(conn)
        now = "2026-06-21T07:03:16+00:00"
        conn.execute(
            """
            insert into workstreams (
              id, key, title, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        readiness = pd.DataFrame(
            [
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "false"},
            ]
        )
        forecast_summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false"},
                {"metric": "merged_pr_count", "value": "7"},
            ]
        )

        brief.persist_work_program_brief_snapshot_to_ontology(
            conn,
            "fixture-source",
            readiness,
            forecast_summary,
            now,
        )

        row = conn.execute(
            """
            select s.operating_status, s.decision_pressure, s.forecast_state,
                   s.executive_summary, s.capability_gaps, s.evidence_count,
                   e.claim_target_kind, e.locator_kind
              from work_program_brief_snapshots s
              join evidences e on e.id = s.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(row[0], "clear")
        self.assertEqual(row[1], "forecast_quality")
        self.assertEqual(row[2], "gated")
        self.assertIn("ETA forecast gated", row[3])
        self.assertEqual(row[4], "forecast_gated")
        self.assertEqual(row[5:], (1, "work_program_brief_snapshot", "tpm_brief_snapshot"))

    def test_work_program_summary_snapshot_materializes_operating_counts(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_summary_snapshot_tables(conn)
        now = "2026-06-21T07:03:16+00:00"
        conn.execute(
            """
            insert into workstreams (
              id, key, title, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.executemany(
            """
            insert into work_program_items (
              key, workstream_key, program_status, decision_state, source_coverage_state,
              freshness_state, due_bucket, risk_score, owner_key, source_system,
              source_instance, external_kind, latest_evidence_id, rank_score, updated_at
            ) values (?, 'flink-kubernetes-operator', ?, ?, ?, ?, ?, ?, ?, 'cubicle_analytics',
              'fixture-source', 'tpm_program_item', null, ?, ?)
            """,
            [
                ("item:1", "validate_signal", "validation_lead", "anonymous_observation", "partial", "now", 95.0, "", 95.0, now),
                ("item:2", "closure_candidate", "product_action", "fresh", "fresh", "now", 80.0, "github:alice", 80.0, now),
                ("item:3", "waiting_review", "validation_lead", "fresh", "fresh", "watch", 70.0, "github:bob", 70.0, now),
            ],
        )
        conn.execute(
            """
            insert into work_owner_load_snapshots (
              key, workstream_key, generated_at, owner_key, load_status, action_count,
              source_system, source_instance, external_kind, latest_evidence_id, rank_score, updated_at
            ) values (
              'owner:unassigned', 'flink-kubernetes-operator', ?, '(unassigned)', 'attention_required', 2,
              'cubicle_analytics', 'fixture-source', 'tpm_owner_load_snapshot', null, 90.0, ?
            )
            """,
            (now, now),
        )
        conn.execute(
            """
            insert into work_blockers (
              key, blocker_state, source_system, source_instance, latest_evidence_id, rank_score, updated_at
            ) values ('blocker:1', 'active', 'cubicle_analytics', 'fixture-source', null, 100.0, ?)
            """,
            (now,),
        )
        conn.execute(
            """
            insert into work_blocker_impacts (
              key, impact_state, source_system, source_instance, latest_evidence_id, rank_score, updated_at
            ) values ('impact:1', 'active', 'cubicle_analytics', 'fixture-source', null, 100.0, ?)
            """,
            (now,),
        )
        conn.execute(
            """
            insert into work_dependency_edges (
              key, edge_kind, source_system, source_instance, latest_evidence_id, rank_score, updated_at
            ) values ('dependency:1', 'needs_action', 'cubicle_analytics', 'fixture-source', null, 100.0, ?)
            """,
            (now,),
        )

        brief.persist_work_program_summary_snapshot_to_ontology(
            conn,
            "fixture-source",
            pd.DataFrame(),
            pd.DataFrame([{"metric": "eta_forecast_ready", "value": "false"}]),
            now,
        )

        row = conn.execute(
            """
            select total_count, validate_signal_count, waiting_review_count,
                   closure_candidate_count, product_action_count, validation_lead_count,
                   source_coverage_limited_count, now_count, high_risk_count,
                   unassigned_count, owner_load_status, attention_owner_count,
                   unassigned_action_count, active_blocker_count,
                   active_blocker_impact_count, needs_action_dependency_count,
                   operating_status, decision_pressure, primary_risk,
                   evidence_count
              from work_program_summary_snapshots
            """
        ).fetchone()
        self.assertEqual(
            row,
            (
                3,
                1,
                1,
                1,
                1,
                2,
                0,
                2,
                1,
                1,
                "attention_required",
                1,
                0,
                1,
                1,
                1,
                "blocked",
                "blocked",
                "active_blockers",
                1,
            ),
        )
        breakdown = conn.execute(
            """
            select breakdown_dimensions, breakdown_keys, breakdown_counts
              from work_program_summary_snapshots
            """
        ).fetchone()
        self.assertIn("program_status", breakdown[0])
        self.assertIn("validate_signal", breakdown[1])
        evidence = conn.execute(
            """
            select e.claim_target_kind, e.locator_kind
              from work_program_summary_snapshots s
              join evidences e on e.id = s.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(evidence, ("work_program_summary_snapshot", "tpm_program_summary"))

    def test_work_program_owner_rollup_snapshots_materialize_from_typed_items(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_summary_snapshot_tables(conn)
        now = "2026-06-21T07:03:16+00:00"
        conn.execute(
            """
            insert into workstreams (
              id, key, title, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.executemany(
            """
            insert into work_program_items (
              key, workstream_key, program_status, decision_state, source_coverage_state,
              freshness_state, due_bucket, risk_score, owner_key, owner_source, source_system,
              source_instance, external_kind, external_id, latest_evidence_id, rank_score,
              register_updated_at, last_activity_at, updated_at
            ) values (?, 'flink-kubernetes-operator', ?, ?, ?, ?, ?, ?, ?, ?, 'cubicle_analytics',
              'fixture-source', 'tpm_program_item', ?, null, ?, ?, ?, ?)
            """,
            [
                (
                    "item:alice-high",
                    "needs_decision",
                    "product_action",
                    "fresh",
                    "fresh",
                    "now",
                    95.0,
                    "github:alice",
                    "pr_author",
                    "program-item-alice-high",
                    95.0,
                    now,
                    now,
                    now,
                ),
                (
                    "item:alice-validate",
                    "validate_signal",
                    "validation_lead",
                    "fresh",
                    "fresh",
                    "watch",
                    80.0,
                    "github:alice",
                    "pr_author",
                    "program-item-alice-validate",
                    80.0,
                    now,
                    now,
                    now,
                ),
                (
                    "item:bob-review",
                    "waiting_review",
                    "validation_lead",
                    "anonymous_observation",
                    "partial",
                    "now",
                    91.0,
                    "github:bob",
                    "requested_reviewer",
                    "program-item-bob-review",
                    91.0,
                    now,
                    now,
                    now,
                ),
            ],
        )

        brief.persist_work_program_owner_rollup_snapshots_to_ontology(conn, "fixture-source", now)

        rows = conn.execute(
            """
            select owner_key, owner_source, item_count, needs_decision_count,
                   validate_signal_count, waiting_review_count, now_count,
                   high_risk_count, max_risk_score, top_item_keys,
                   freshness_state, evidence_count
              from work_program_owner_rollup_snapshots
             order by max_risk_score desc, item_count desc, owner_key
            """
        ).fetchall()
        self.assertEqual(
            rows,
            [
                (
                    "github:alice",
                    "pr_author",
                    2,
                    1,
                    1,
                    0,
                    1,
                    1,
                    95.0,
                    "item:alice-high\nitem:alice-validate",
                    "fresh",
                    1,
                ),
                (
                    "github:bob",
                    "requested_reviewer",
                    1,
                    0,
                    0,
                    1,
                    1,
                    1,
                    91.0,
                    "item:bob-review",
                    "partial",
                    1,
                ),
            ],
        )
        evidence = conn.execute(
            """
            select e.claim_target_kind, e.locator_kind
              from work_program_owner_rollup_snapshots s
              join evidences e on e.id = s.latest_evidence_id
             where s.owner_key = 'github:alice'
            """
        ).fetchone()
        self.assertEqual(evidence, ("work_program_owner_rollup_snapshot", "tpm_program_owner_rollup"))

    def test_work_insight_evaluation_snapshot_materializes_aggregate_and_kind_rows(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_insight_evaluation_snapshot_tables(conn)
        now = "2026-06-21T07:03:16+00:00"
        readiness = pd.DataFrame(
            [
                {"metric": "current_insight_count", "value": "5"},
                {"metric": "review_row_count", "value": "8"},
                {"metric": "evaluation_label_row_count", "value": "5"},
                {"metric": "open_review_request_count", "value": "1"},
                {"metric": "precision_rate", "value": "0.3333"},
                {"metric": "useful_signal_rate", "value": "0.6667"},
                {"metric": "actionability_rate", "value": "0.6667"},
                {"metric": "false_positive_rate", "value": "0.3333"},
                {"metric": "measurement_coverage_rate", "value": "1"},
                {"metric": "ready_to_measure_precision", "value": "false"},
                {"metric": "ready_to_measure_actionability", "value": "false"},
                {"metric": "product_candidate_insight_count", "value": "3"},
                {"metric": "product_candidate_review_row_count", "value": "6"},
                {"metric": "product_candidate_measurement_label_count", "value": "3"},
                {"metric": "product_candidate_open_review_request_count", "value": "1"},
                {"metric": "product_candidate_precision_rate", "value": "0.3333"},
                {"metric": "product_candidate_useful_signal_rate", "value": "0.6667"},
                {"metric": "product_candidate_actionability_rate", "value": "0.6667"},
                {"metric": "product_candidate_false_positive_rate", "value": "0.3333"},
                {"metric": "product_candidate_measurement_coverage_rate", "value": "1"},
                {"metric": "product_candidate_ready_to_measure_precision", "value": "false"},
                {"metric": "product_candidate_ready_to_measure_actionability", "value": "false"},
                {"metric": "review_requests_blocker_candidate", "value": "3"},
                {"metric": "review_rows_blocker_candidate", "value": "6"},
                {"metric": "measurement_labels_blocker_candidate", "value": "3"},
                {"metric": "open_review_requests_blocker_candidate", "value": "1"},
                {"metric": "measurement_required_blocker_candidate", "value": "3"},
                {"metric": "truth_labeled_blocker_candidate", "value": "3"},
                {"metric": "actionability_labeled_blocker_candidate", "value": "3"},
                {"metric": "true_positive_blocker_candidate", "value": "1"},
                {"metric": "false_positive_blocker_candidate", "value": "1"},
                {"metric": "partial_blocker_candidate", "value": "1"},
                {"metric": "actionable_blocker_candidate", "value": "1"},
                {"metric": "needs_owner_blocker_candidate", "value": "1"},
                {"metric": "precision_rate_blocker_candidate", "value": "0.3333"},
                {"metric": "useful_signal_rate_blocker_candidate", "value": "0.6667"},
                {"metric": "actionability_rate_blocker_candidate", "value": "0.6667"},
                {"metric": "false_positive_rate_blocker_candidate", "value": "0.3333"},
                {"metric": "measurement_coverage_rate_blocker_candidate", "value": "1"},
                {"metric": "ready_to_measure_blocker_candidate", "value": "true"},
                {"metric": "review_requests_developer_correlation", "value": "2"},
                {"metric": "measurement_scope_developer_correlation", "value": "product_candidate"},
                {"metric": "review_rows_developer_correlation", "value": "2"},
                {"metric": "measurement_labels_developer_correlation", "value": "2"},
                {"metric": "open_review_requests_developer_correlation", "value": "0"},
                {"metric": "measurement_required_developer_correlation", "value": "2"},
                {"metric": "truth_labeled_developer_correlation", "value": "2"},
                {"metric": "actionability_labeled_developer_correlation", "value": "2"},
                {"metric": "partial_developer_correlation", "value": "2"},
                {"metric": "needs_owner_developer_correlation", "value": "2"},
                {"metric": "precision_rate_developer_correlation", "value": "0"},
                {"metric": "useful_signal_rate_developer_correlation", "value": "1"},
                {"metric": "actionability_rate_developer_correlation", "value": "1"},
                {"metric": "false_positive_rate_developer_correlation", "value": "0"},
                {"metric": "measurement_coverage_rate_developer_correlation", "value": "1"},
                {"metric": "ready_to_measure_developer_correlation", "value": "true"},
            ]
        )

        brief.persist_work_insight_evaluation_snapshots_to_ontology(
            conn,
            "fixture-source",
            readiness,
            now,
        )

        aggregate = conn.execute(
            """
            select current_insight_count, measurement_label_count, ready_insight_kind_count,
                   product_action_ready_kind_count,
                   quality_gated_insight_kind_count, gated_insight_kind_count,
                   evidence_count, latest_evidence_id is not null
              from work_insight_evaluation_snapshots
            """
        ).fetchone()
        self.assertEqual(aggregate, (3, 3, 2, 0, 1, 0, 1, 1))
        kind_rows = conn.execute(
            """
            select insight_kind, measurement_scope, ready_to_measure, ready_for_product_action,
                   product_action_gate_state, precision_rate, actionability_rate,
                   evidence_count, work_insight_evaluation_snapshot_id
              from work_insight_kind_evaluation_snapshots
            """
        ).fetchall()
        by_kind = {row[0]: row for row in kind_rows}
        blocker = by_kind["blocker_candidate"]
        self.assertEqual(blocker[:5], ("blocker_candidate", "product_candidate", 1, 0, "quality_gated"))
        self.assertAlmostEqual(blocker[5], 0.3333)
        self.assertAlmostEqual(blocker[6], 0.6667)
        self.assertEqual(blocker[7:], (1, 1))
        developer = by_kind["developer_correlation"]
        self.assertEqual(developer[:5], ("developer_correlation", "context_only", 1, 0, "context_only"))
        self.assertAlmostEqual(developer[5], 0.0)
        self.assertAlmostEqual(developer[6], 1.0)
        self.assertEqual(developer[7:], (1, 1))
        evidence = conn.execute(
            """
            select e.claim_target_kind, e.locator_kind
              from work_insight_evaluation_snapshots s
              join evidences e on e.id = s.latest_evidence_id
            """
        ).fetchone()
        self.assertEqual(evidence, ("work_insight_evaluation_snapshot", "tpm_insight_evaluation"))

    def test_work_program_risk_drivers_materialize_ranked_source_queue(self) -> None:
        conn = sqlite3.connect(":memory:")
        create_minimal_risk_driver_tables(conn)
        now = "2026-06-21T07:03:16+00:00"
        conn.execute(
            """
            insert into workstreams (
              id, key, title, source_system, source_instance, external_kind, external_id
            ) values (1, 'workstream:flink-kubernetes-operator', 'Flink Kubernetes Operator', 'cubicle_analytics', 'fixture-source', 'tpm_workstream', 'flink-kubernetes-operator')
            """
        )
        conn.execute(
            """
            insert into evidences (
              id, key, claim_kind, claim_target_kind, claim_target_id, claim_field,
              locator_kind, locator, source_span_key, proof_state, observed_at,
              source_system, source_instance, external_kind, external_id, source_url
            ) values
              (1, 'evidence:source:blocker', 'object_state', 'work_blocker', 1, 'blocker_state', 'tpm_work_blocker', 'blocker-1', 'span-blocker', 'current', ?, 'cubicle_analytics', 'fixture-source', 'tpm_source_evidence', 'source-blocker', 'https://example.test/blocker'),
              (2, 'evidence:source:impact', 'object_state', 'work_blocker_impact', 1, 'impact_state', 'tpm_work_blocker_impact', 'impact-1', 'span-impact', 'current', ?, 'cubicle_analytics', 'fixture-source', 'tpm_source_evidence', 'source-impact', 'https://example.test/impact')
            """,
            (now, now),
        )
        conn.execute(
            """
            insert into work_blockers (
              key, blocker_kind, blocker_state, severity, subject_kind, subject_key,
              title, recommended_action, source_system, source_instance, external_kind,
              external_id, source_url, latest_evidence_id, evidence_count,
              freshness_state, visibility, confidence, event_count, first_seen_at,
              last_activity_at, rank_score, created_at, updated_at
            ) values (
              'work-blocker:1', 'source_signal', 'active', 'high', 'pull_request', 'repo/example#1',
              'Merge is blocked by source signal', 'Ask owner for blocker clearance.', 'cubicle_analytics', 'fixture-source', 'tpm_work_blocker',
              'blocker-1', 'https://example.test/blocker', 1, 1,
              'fresh', 'public', 0.91, 1, ?, ?, 95.0, ?, ?
            )
            """,
            (now, now, now, now),
        )
        conn.execute(
            """
            insert into work_blocker_impacts (
              key, impact_kind, impact_state, impact_score, severity, blocker_kind,
              affected_kind, affected_key, subject_kind, subject_key, title,
              recommended_action, source_system, source_instance, external_kind,
              external_id, source_url, latest_evidence_id, evidence_count,
              freshness_state, visibility, confidence, event_count, first_seen_at,
              last_activity_at, rank_score, created_at, updated_at
            ) values (
              'work-blocker-impact:1', 'workstream', 'active', 130.0, 'high', 'source_signal',
              'workstream', 'workstream:flink-kubernetes-operator', 'pull_request', 'repo/example#1', 'Blocker impacts workstream',
              'Clear this before publishing the workstream as unblocked.', 'cubicle_analytics', 'fixture-source', 'tpm_work_blocker_impact',
              'impact-1', 'https://example.test/impact', 2, 1,
              'fresh', 'public', 0.92, 1, ?, ?, 130.0, ?, ?
            )
            """,
            (now, now, now, now),
        )
        conn.execute(
            """
            insert into work_dependency_edges (
              key, edge_kind, from_kind, from_key, to_kind, to_key, risk_signal,
              source_coverage_state, workstream_id, source_system, source_instance,
              external_kind, external_id, source_url, latest_evidence_id, evidence_count,
              freshness_state, visibility, confidence, event_count, first_seen_at,
              last_activity_at, rank_score, created_at, updated_at
            ) values (
              'work-dependency:1', 'needs_action', 'blocker', 'work-blocker:1', 'action', 'work-action:1', 'product_action',
              'fresh', 1, 'cubicle_analytics', 'fixture-source',
              'tpm_work_dependency_edge', 'dependency-1', 'https://example.test/dependency', 1, 1,
              'fresh', 'public', 0.88, 1, ?, ?, 100.0, ?, ?
            )
            """,
            (now, now, now, now),
        )
        conn.execute(
            """
            insert into work_item_forecasts (
              key, forecast_kind, subject_kind, subject_key, subject_state, overdue_days,
              risk_score, risk_band, readiness_state, ready_for_eta, readiness_reason,
              forecasted_at, source_system, source_instance, external_kind, external_id,
              source_url, latest_evidence_id, evidence_count, freshness_state, visibility,
              confidence, event_count, first_seen_at, last_activity_at, rank_score, created_at, updated_at
            ) values (
              'work-item-forecast:1', 'cycle_time', 'pull_request', 'repo/example#2', 'open', 10.0,
              80.0, 'high', 'gated', 0, 'ETA is gated.', ?,
              'cubicle_analytics', 'fixture-source', 'tpm_pr_forecast', 'repo/example#2',
              'https://example.test/forecast', 1, 1, 'fresh', 'public',
              0.9, 1, ?, ?, 80.0, ?, ?
            )
            """,
            (now, now, now, now, now),
        )
        conn.execute(
            """
            insert into work_owner_load_snapshots (
              key, workstream_id, workstream_key, owner_key, owner_display_name,
              generated_at, load_status, action_count, max_priority_score,
              recommended_focus, source_system, source_instance, external_kind,
              external_id, source_url, latest_evidence_id, evidence_count,
              freshness_state, visibility, confidence, event_count, first_seen_at,
              last_activity_at, rank_score, created_at, updated_at
            ) values (
              'work-owner-load:1', 1, 'flink-kubernetes-operator', 'alice', 'Alice',
              ?, 'overloaded', 3, 90.0,
              'Rebalance Alice before adding more work.', 'cubicle_analytics', 'fixture-source', 'tpm_owner_load_snapshot',
              'owner-load-1', 'https://example.test/owner-load', 1, 1,
              'fresh', 'public', 0.93, 3, ?, ?, 90.0, ?, ?
            )
            """,
            (now, now, now, now, now),
        )

        drivers = brief.ontology_work_program_risk_drivers(
            conn,
            "fixture-source",
            "flink-kubernetes-operator",
            now,
        )

        self.assertEqual(
            {driver["driver_kind"] for driver in drivers},
            {"blocker", "blocker_impact", "dependency", "forecast_risk", "owner_load"},
        )
        self.assertEqual(drivers[0]["driver_kind"], "blocker_impact")
        owner_driver = next(driver for driver in drivers if driver["driver_kind"] == "owner_load")
        self.assertEqual(owner_driver["rank_score"], 120.0)
        forecast_driver = next(driver for driver in drivers if driver["driver_kind"] == "forecast_risk")
        self.assertEqual(forecast_driver["status"], "risk_triage")

        brief.persist_work_program_risk_drivers_to_ontology(
            conn,
            "fixture-source",
            now,
        )

        row_count = conn.execute(
            """
            select count(*)
              from work_program_risk_drivers
             where source_instance = 'fixture-source'
            """
        ).fetchone()[0]
        self.assertEqual(row_count, 5)
        persisted = conn.execute(
            """
            select driver_kind, status, evidence_count, badge_keys
              from work_program_risk_drivers
             where source_instance = 'fixture-source'
             order by rank_score desc, driver_kind
            """
        ).fetchall()
        self.assertEqual(persisted[0][0], "blocker_impact")
        self.assertTrue(all(row[2] == 1 for row in persisted))
        self.assertTrue(all("risk_driver:kind:" in row[3] for row in persisted))
        evidence = conn.execute(
            """
            select e.claim_target_kind, e.claim_field, e.locator_kind
              from work_program_risk_drivers d
              join evidences e on e.id = d.latest_evidence_id
             where d.driver_kind = 'owner_load'
            """
        ).fetchone()
        self.assertEqual(evidence, ("work_program_risk_driver", "status", "tpm_risk_driver"))


def create_minimal_work_responsibility_tables(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table work_responsibilities (
          id integer primary key autoincrement,
          key text unique,
          person_id integer,
          workstream_id integer,
          pull_request_id integer,
          ticket_id integer,
          work_action_id integer,
          work_blocker_id integer,
          work_program_evidence_need_id integer,
          work_program_item_id integer,
          workstream_key text,
          subject_kind text,
          subject_key text,
          party_kind text,
          party_key text,
          party_source text,
          responsibility_kind text,
          basis_kind text,
          basis_detail text,
          responsibility_state text,
          responsibility_state_reason text,
          generated_at text,
          valid_from text,
          valid_until text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table persons (
          id integer primary key autoincrement,
          key text unique,
          display_name text,
          github_login text
        )
        """
    )
    conn.execute(
        """
        create table person_identities (
          id integer primary key autoincrement,
          person_id integer,
          handle text,
          external_id text,
          source_system text,
          external_kind text,
          identity_status text
        )
        """
    )
    conn.execute(
        """
        create table pull_requests (
          id integer primary key autoincrement,
          key text unique,
          repository text,
          number integer,
          title text,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table tickets (
          id integer primary key autoincrement,
          key text unique,
          external_id text,
          title text,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table work_actions (
          id integer primary key autoincrement,
          key text unique,
          owner_key text,
          owner_source text,
          action_state text,
          decision_state text,
          created_from_run_key text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          opened_at text
        )
        """
    )
    conn.execute(
        """
        create table work_blockers (
          id integer primary key autoincrement,
          key text unique,
          owner_key text,
          owner_source text,
          blocker_state text,
          decision_state text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real
        )
        """
    )
    conn.execute(
        """
        create table work_program_items (
          id integer primary key autoincrement,
          key text unique,
          workstream_key text,
          subject_kind text,
          subject_key text,
          pull_request_id integer,
          ticket_id integer,
          owner_key text,
          owner_source text,
          author_dri text,
          requested_reviewer_keys text,
          reviewer_or_approver text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          register_updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_program_evidence_needs (
          id integer primary key autoincrement,
          key text unique,
          workstream_key text,
          owner_key text,
          action_state text,
          execution_state text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          generated_at text
        )
        """
    )
    conn.execute(
        """
        create table evidences (
          id integer primary key autoincrement,
          key text unique,
          claim_kind text,
          claim_target_kind text,
          claim_target_id integer,
          claim_field text,
          relationship_kind text,
          relationship_id integer,
          locator_kind text,
          locator text,
          source_span_key text,
          excerpt text,
          proof_state text,
          observed_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_updated_at text,
          content_hash text,
          deletion_state text,
          acl_state text,
          last_confirmed_at text,
          last_changed_at text,
          freshness_state text,
          visibility text,
          confidence real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_review_tables(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table work_insights (
          id integer primary key,
          key text,
          insight_kind text,
          severity text,
          subject_kind text,
          subject_key text,
          title text,
          details text,
          recommended_action text,
          source_url text,
          latest_evidence_id integer,
          score real,
          confidence real,
          rank_score real,
          producer_state text,
          source_system text,
          source_instance text,
          external_kind text,
          updated_at text
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
              source_system text,
              source_instance text,
              external_kind text,
              owner_key text,
              next_action text,
              rationale text,
          reviewed_at text,
          updated_at text,
          source_url text
        )
        """
    )


def create_minimal_action_tables(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table work_actions (
          id integer primary key autoincrement,
          key text unique,
          action_type text,
          action_state text,
          decision_state text,
          decision text,
          decision_reason text,
          subject_kind text,
          subject_key text,
          pull_request_id integer,
          ticket_id integer,
          owner_key text,
          owner_source text,
          due_bucket text,
          created_from_run_key text,
          opened_at text,
          decided_at text,
          closed_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_action_observations (
          id integer primary key autoincrement,
          key text unique,
          work_action_id integer,
          observation_kind text,
          source_coverage_state text,
          auth_state text,
          current_state text,
          ci_signal text,
          ci_required_check_coverage_state text,
          ci_required_check_match_state text,
          ci_required_context_count integer,
          ci_failing_required_context_count integer,
          ci_pending_required_context_count integer,
          ci_missing_required_context_count integer,
          ci_failing_required_contexts text,
          ci_pending_required_contexts text,
          ci_missing_required_contexts text,
          ci_failing_context_count integer,
          ci_pending_context_count integer,
          ci_failing_contexts text,
          ci_pending_contexts text,
          supports_action integer,
          observed_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_action_source_insights (
          work_action_id integer,
          work_insight_id integer,
          unique(work_action_id, work_insight_id)
        )
        """
    )
    conn.execute(
        """
        create table evidences (
          id integer primary key autoincrement,
          key text unique,
          claim_kind text,
          claim_target_kind text,
          claim_target_id integer,
          claim_field text,
          relationship_kind text,
          locator_kind text,
          locator text,
          source_span_key text,
          excerpt text,
          proof_state text,
          observed_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_updated_at text,
          content_hash text,
          deletion_state text,
          acl_state text,
          last_confirmed_at text,
          last_changed_at text,
          freshness_state text,
          visibility text,
          confidence real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_blocker_tables(conn: sqlite3.Connection) -> None:
    create_minimal_action_tables(conn)
    create_minimal_review_tables(conn)
    conn.execute(
        """
        create table work_blockers (
          id integer primary key autoincrement,
          key text unique,
          blocker_kind text,
          blocker_state text,
          severity text,
          subject_kind text,
          subject_key text,
          pull_request_id integer,
          ticket_id integer,
          work_action_id integer,
          work_insight_id integer,
          owner_key text,
          owner_source text,
          decision_state text,
          source_coverage_state text,
          review_state text,
          truth_label text,
          actionability_label text,
          label_quality text,
          measurement_eligible integer,
          reviewer_kind text,
          reviewer_key text,
          label_set text,
          title text,
          recommended_action text,
          summary text,
          search_text text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_updated_at text,
          content_hash text,
          deletion_state text,
          acl_state text,
          last_confirmed_at text,
          last_changed_at text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_blocker_impact_tables(conn: sqlite3.Connection) -> None:
    create_minimal_blocker_tables(conn)
    conn.execute(
        """
        create table tickets (
          id integer primary key autoincrement,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table pull_requests (
          id integer primary key autoincrement,
          repository text,
          number integer
        )
        """
    )
    conn.execute(
        """
        create table ticket_pull_requests (
          ticket_id integer,
          pull_request_id integer,
          ticket_pull_request_kind text,
          latest_evidence_id integer,
          evidence_count integer,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table workstreams (
          id integer primary key autoincrement,
          key text unique,
          title text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table work_dependency_edges (
          id integer primary key autoincrement,
          key text unique,
          edge_kind text,
          relationship_authority text,
          canonical_relationship_kind text,
          from_kind text,
          from_key text,
          to_kind text,
          to_key text,
          risk_signal text,
          source_coverage_state text,
          workstream_id integer,
          work_blocker_id integer,
          work_action_id integer,
          ticket_id integer,
          pull_request_id integer,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_updated_at text,
          content_hash text,
          deletion_state text,
          acl_state text,
          last_confirmed_at text,
          last_changed_at text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_blocker_impacts (
          id integer primary key autoincrement,
          key text unique,
          impact_kind text,
          impact_state text,
          impact_score real,
          severity text,
          blocker_kind text,
          work_blocker_id integer,
          work_action_id integer,
          workstream_id integer,
          pull_request_id integer,
          ticket_id integer,
          affected_kind text,
          affected_key text,
          subject_kind text,
          subject_key text,
          path_length integer,
          source_coverage_state text,
          title text,
          recommended_action text,
          summary text,
          search_text text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_forecast_tables(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table evidences (
          id integer primary key autoincrement,
          key text unique,
          claim_kind text,
          claim_target_kind text,
          claim_target_id integer,
          claim_field text,
          locator_kind text,
          locator text,
          source_span_key text,
          excerpt text,
          proof_state text,
          observed_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_updated_at text,
          content_hash text,
          deletion_state text,
          acl_state text,
          last_confirmed_at text,
          last_changed_at text,
          freshness_state text,
          visibility text,
          confidence real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_forecast_evaluations (
          id integer primary key autoincrement,
          key text unique,
          evaluation_kind text,
          model_name text,
          forecast_method text,
          best_model_name text,
          fold integer,
          train_count integer,
          test_count integer,
          baseline_sample_count integer,
          open_candidate_count integer,
          closed_unmerged_count integer,
          observed_snapshot_time_count integer,
          transition_candidate_count integer,
          terminal_transition_candidate_count integer,
          transition_history_ready integer,
          median_cycle_days real,
          p75_cycle_days real,
          avg_closed_unmerged_cycle_days real,
          mae_days real,
          median_error_days real,
          p75_error_days real,
          max_error_days real,
          improvement_vs_median_pct real,
          ready_for_eta integer,
          readiness_state text,
          readiness_reason text,
          note text,
          evaluated_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_work_item_forecast_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table pull_requests (
          id integer primary key autoincrement,
          key text unique,
          repository text,
          number integer,
          title text,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table work_item_forecasts (
          id integer primary key autoincrement,
          key text unique,
          forecast_kind text,
          subject_kind text,
          subject_key text,
          pull_request_id integer,
          ticket_id integer,
          work_action_id integer,
          subject_state text,
          forecast_method text,
          model_name text,
          age_days real,
          predicted_total_cycle_days real,
          predicted_remaining_days real,
          overdue_days real,
          risk_score real,
          risk_band text,
          readiness_state text,
          ready_for_eta integer,
          readiness_reason text,
          forecasted_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_actions (
          id integer primary key autoincrement,
          key text unique,
          subject_kind text,
          subject_key text,
          action_type text,
          action_state text,
          decision_state text,
          rank_score real,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          updated_at text
        )
        """
    )


def create_minimal_work_program_milestone_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table workstreams (
          id integer primary key autoincrement,
          key text unique,
          title text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table tickets (
          id integer primary key autoincrement,
          key text unique,
          external_id text,
          title text,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table pull_requests (
          id integer primary key autoincrement,
          key text unique,
          repository text,
          number integer,
          title text,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table work_program_milestones (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          pull_request_id integer,
          ticket_id integer,
          workstream_key text,
          subject_kind text,
          subject_key text,
          milestone_kind text,
          milestone_name text,
          target_date text,
          outcome_date text,
          milestone_state text,
          commitment_strength text,
          date_claim_allowed integer,
          delivery_commitment_allowed integer,
          claim_gate_reason text,
          source_field text,
          source_payload_key text,
          captured_at text,
          generated_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_generated_evidence_table(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table if not exists evidences (
          id integer primary key autoincrement,
          key text unique,
          claim_kind text,
          claim_target_kind text,
          claim_target_id integer,
          claim_field text,
          locator_kind text,
          locator text,
          source_span_key text,
          excerpt text,
          proof_state text,
          observed_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_updated_at text,
          content_hash text,
          deletion_state text,
          acl_state text,
          last_confirmed_at text,
          last_changed_at text,
          freshness_state text,
          visibility text,
          confidence real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_work_program_run_tables(conn: sqlite3.Connection) -> None:
    common_columns = """
      id integer primary key autoincrement,
      key text unique,
      source_system text,
      source_instance text,
      workstream_key text,
      generated_at text,
      external_kind text,
      external_id text,
      rank_score real
    """
    for table_name in [
        "work_program_quality_gates",
        "work_program_evidence_needs",
        "work_owner_load_snapshots",
    ]:
        conn.execute(f"create table {table_name} ({common_columns})")
    conn.execute(
        f"""
        create table work_program_automation_readinesses (
          {common_columns},
          readiness_state text,
          readiness_score real,
          autonomous_action_ready integer,
          human_review_required integer,
          blocking_gate_count integer,
          evidence_need_count integer,
          tpm_function_count integer
        )
        """
    )


def seed_run_member(
    conn: sqlite3.Connection,
    table_name: str,
    key_suffix: str,
    generated_at: str,
    **overrides: object,
) -> None:
    base = {
        "key": f"{table_name}:{key_suffix}",
        "source_system": "cubicle_analytics",
        "source_instance": "fixture-source",
        "workstream_key": "flink-kubernetes-operator",
        "generated_at": generated_at,
        "external_kind": table_name.removesuffix("s"),
        "external_id": f"flink-kubernetes-operator|{generated_at}|{key_suffix}",
        "rank_score": overrides.pop("rank_score", 1.0),
    }
    base.update(overrides)
    brief.upsert_row(conn, table_name, base, "key")


def create_minimal_insight_evaluation_snapshot_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table work_insight_evaluation_snapshots (
          id integer primary key autoincrement,
          key text unique,
          generated_at text,
          current_insight_count integer,
          review_row_count integer,
          measurement_label_count integer,
          open_review_request_count integer,
          min_labeled_total_required integer,
          min_labeled_per_kind_required integer,
          min_precision_rate_for_product_action real,
          min_useful_signal_rate_for_product_action real,
          min_actionability_rate_for_product_action real,
          precision_rate real,
          useful_signal_rate real,
          actionability_rate real,
          false_positive_rate real,
          measurement_coverage_rate real,
          ready_to_measure_precision integer,
          ready_to_measure_actionability integer,
          ready_insight_kind_count integer,
          product_action_ready_kind_count integer,
          quality_gated_insight_kind_count integer,
          gated_insight_kind_count integer,
          recommended_next_step text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_insight_kind_evaluation_snapshots (
          id integer primary key autoincrement,
          key text unique,
          work_insight_evaluation_snapshot_id integer,
          generated_at text,
          insight_kind text,
          current_insight_count integer,
          review_row_count integer,
          measurement_label_count integer,
          open_review_request_count integer,
          truth_labeled_count integer,
          actionability_labeled_count integer,
          true_positive_count integer,
          false_positive_count integer,
          partial_count integer,
          actionable_count integer,
          needs_owner_count integer,
          precision_rate real,
          useful_signal_rate real,
          actionability_rate real,
          false_positive_rate real,
          measurement_coverage_rate real,
          required_label_count integer,
          ready_to_measure integer,
          ready_for_product_action integer,
          product_action_gate_state text,
          product_action_gate_reason text,
          recommended_action text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_summary_snapshot_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table workstreams (
          id integer primary key autoincrement,
          key text unique,
          title text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table work_program_items (
          key text unique,
          workstream_key text,
          program_status text,
          decision_state text,
          source_coverage_state text,
          freshness_state text,
          due_bucket text,
          risk_score real,
          owner_key text,
          owner_source text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          latest_evidence_id integer,
          rank_score real,
          register_updated_at text,
          last_activity_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_owner_load_snapshots (
          key text unique,
          workstream_key text,
          generated_at text,
          owner_key text,
          load_status text,
          action_count integer,
          source_system text,
          source_instance text,
          external_kind text,
          latest_evidence_id integer,
          rank_score real,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_program_owner_rollup_snapshots (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          workstream_key text,
          generated_at text,
          owner_key text,
          owner_source text,
          item_count integer,
          needs_decision_count integer,
          validate_signal_count integer,
          ci_failing_count integer,
          waiting_review_count integer,
          source_repair_count integer,
          closure_candidate_count integer,
          now_count integer,
          high_risk_count integer,
          max_risk_score real,
          top_item_keys text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_blockers (
          key text unique,
          blocker_state text,
          source_system text,
          source_instance text,
          latest_evidence_id integer,
          rank_score real,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_blocker_impacts (
          key text unique,
          impact_state text,
          source_system text,
          source_instance text,
          latest_evidence_id integer,
          rank_score real,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_dependency_edges (
          key text unique,
          edge_kind text,
          source_system text,
          source_instance text,
          latest_evidence_id integer,
          rank_score real,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_program_summary_snapshots (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          workstream_key text,
          generated_at text,
          total_count integer,
          needs_decision_count integer,
          validate_signal_count integer,
          ci_failing_count integer,
          waiting_review_count integer,
          source_repair_count integer,
          closed_pending_review_count integer,
          model_quality_count integer,
          closure_candidate_count integer,
          dismissed_count integer,
          now_count integer,
          high_risk_count integer,
          unassigned_count integer,
          product_action_count integer,
          validation_lead_count integer,
          source_coverage_limited_count integer,
          owner_load_status text,
          owner_load_action_count integer,
          overloaded_owner_count integer,
          attention_owner_count integer,
          unassigned_action_count integer,
          blocker_count integer,
          active_blocker_count integer,
          validating_blocker_count integer,
          blocker_impact_count integer,
          active_blocker_impact_count integer,
          dependency_edge_count integer,
          blocking_dependency_count integer,
          needs_action_dependency_count integer,
          operating_status text,
          decision_pressure text,
          forecast_state text,
          primary_risk text,
          recommended_focus text,
          capability_gaps text,
          breakdown_dimensions text,
          breakdown_keys text,
          breakdown_counts text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_brief_caveat_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table workstreams (
          id integer primary key autoincrement,
          key text unique,
          title text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table work_program_brief_caveats (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          workstream_key text,
          generated_at text,
          caveat_key text,
          severity text,
          title text,
          detail text,
          recommended_action text,
          evidence_ref text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_brief_snapshot_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table workstreams (
          id integer primary key autoincrement,
          key text unique,
          title text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table work_program_brief_snapshots (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          workstream_key text,
          generated_at text,
          operating_status text,
          decision_pressure text,
          forecast_state text,
          primary_risk text,
          executive_summary text,
          recommended_focus text,
          next_cadence_focus text,
          capability_gaps text,
          total_count integer,
          product_action_count integer,
          validation_lead_count integer,
          source_coverage_limited_count integer,
          active_blocker_count integer,
          active_blocker_impact_count integer,
          needs_action_dependency_count integer,
          overloaded_owner_count integer,
          unassigned_action_count integer,
          quality_gate_count integer,
          blocking_gate_count integer,
          caveat_count integer,
          risk_driver_count integer,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_risk_driver_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table workstreams (
          id integer primary key autoincrement,
          key text unique,
          title text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table work_program_risk_drivers (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          workstream_key text,
          generated_at text,
          driver_key text,
          driver_kind text,
          subject_kind text,
          subject_key text,
          title text,
          status text,
          recommended_action text,
          evidence_ref text,
          badge_keys text,
          badge_labels text,
          badge_tones text,
          badge_details text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_blockers (
          id integer primary key autoincrement,
          key text unique,
          blocker_kind text,
          blocker_state text,
          severity text,
          subject_kind text,
          subject_key text,
          title text,
          recommended_action text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_blocker_impacts (
          id integer primary key autoincrement,
          key text unique,
          impact_kind text,
          impact_state text,
          impact_score real,
          severity text,
          blocker_kind text,
          affected_kind text,
          affected_key text,
          subject_kind text,
          subject_key text,
          title text,
          recommended_action text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_dependency_edges (
          id integer primary key autoincrement,
          key text unique,
          edge_kind text,
          from_kind text,
          from_key text,
          to_kind text,
          to_key text,
          risk_signal text,
          source_coverage_state text,
          workstream_id integer,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_item_forecasts (
          id integer primary key autoincrement,
          key text unique,
          forecast_kind text,
          subject_kind text,
          subject_key text,
          subject_state text,
          overdue_days real,
          risk_score real,
          risk_band text,
          readiness_state text,
          ready_for_eta integer,
          readiness_reason text,
          forecasted_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_owner_load_snapshots (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          workstream_key text,
          owner_key text,
          owner_display_name text,
          generated_at text,
          load_status text,
          action_count integer,
          max_priority_score real,
          recommended_focus text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_work_item_state_snapshot_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table pull_requests (
          id integer primary key autoincrement,
          key text unique,
          repository text,
          number integer,
          title text,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table tickets (
          id integer primary key autoincrement,
          key text unique,
          external_id text,
          title text,
          source_url text
        )
        """
    )
    conn.execute(
        """
        create table work_item_state_snapshots (
          id integer primary key autoincrement,
          key text unique,
          subject_kind text,
          subject_key text,
          pull_request_id integer,
          ticket_id integer,
          state text,
          title text,
          observed_at text,
          captured_at text,
          source_created_at text,
          source_updated_at text,
          closed_at text,
          merged_at text,
          age_days real,
          stale_days real,
          cycle_time_days real,
          risk_score real,
          risk_band text,
          forecast_method text,
          source_current_coverage_state text,
          source_current_detail_state text,
          source_current_issue_codes text,
          source_current_issue_kinds text,
          lifecycle_fields_source text,
          churn_fields_source text,
          mergeability_fields_source text,
          priority text,
          linked_pr_count integer,
          fresh_pr_link_count integer,
          partial_pr_link_count integer,
          comment_count integer,
          participant_count integer,
          blocker_keyword_count integer,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_work_item_state_transition_tables(conn: sqlite3.Connection) -> None:
    create_minimal_work_item_state_snapshot_tables(conn)
    conn.execute(
        """
        create table work_item_state_transitions (
          id integer primary key autoincrement,
          key text unique,
          subject_kind text,
          subject_key text,
          pull_request_id integer,
          ticket_id integer,
          from_snapshot_id integer,
          to_snapshot_id integer,
          from_observed_at text,
          to_observed_at text,
          from_state text,
          to_state text,
          transition_kind text,
          transition_confidence real,
          confidence_basis text,
          verification_state text,
          terminal integer,
          requires_closeout integer,
          note text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )


def create_minimal_workstream_tables(conn: sqlite3.Connection) -> None:
    create_minimal_generated_evidence_table(conn)
    conn.execute(
        """
        create table tickets (
          id integer primary key autoincrement,
          key text unique,
          external_id text,
          title text
        )
        """
    )
    conn.execute(
        """
        create table persons (
          id integer primary key autoincrement,
          key text unique,
          display_name text,
          github_login text
        )
        """
    )
    conn.execute(
        """
        create table person_identities (
          id integer primary key autoincrement,
          person_id integer,
          handle text,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table workstream_health_snapshots (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          workstream_key text,
          generated_at text,
          operating_status text,
          action_item_count integer,
          product_action_count integer,
          validation_lead_count integer,
          critical_or_high_validation_lead_count integer,
          model_or_rule_qa_count integer,
          closeout_review_count integer,
          owner_count integer,
          top_owner_action_count integer,
          failing_check_pr_count integer,
          open_failing_check_pr_count integer,
          source_repair_count integer,
          coverage_limited_count integer,
          anonymous_observation_count integer,
          terminal_transition_count integer,
          terminal_transition_subjects text,
          eta_forecast_ready integer,
          truth_label_coverage text,
          actionability_label_coverage text,
          recommended_cadence_focus text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table pull_requests (
          id integer primary key autoincrement,
          key text unique,
          repository text,
          number integer,
          title text
        )
        """
    )
    conn.execute(
        """
        create table workstreams (
          id integer primary key autoincrement,
          key text unique,
          title text,
          status text,
          summary text,
          search_text text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_updated_at text,
          content_hash text,
          deletion_state text,
          acl_state text,
          last_confirmed_at text,
          last_changed_at text,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_program_items (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          work_action_id integer unique,
          pull_request_id integer,
          ticket_id integer,
          workstream_key text,
          subject_kind text,
          subject_key text,
          linked_ticket_keys text,
          linked_pull_request_keys text,
          title text,
          program_status text,
          tpm_bucket text,
          owner_key text,
          owner_source text,
          author_dri text,
          requested_reviewer_keys text,
          reviewer_or_approver text,
          next_action text,
          decision_needed text,
          decision_state text,
          decision_gate_reason text,
          due_bucket text,
          last_source_update_at text,
          age_days real,
          stale_days real,
          risk_score real,
          blocker_label_state text,
          ci_signal text,
          transition_state text,
          dependency_summary text,
          source_coverage_state text,
          label_quality text,
          register_updated_at text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_owner_load_snapshots (
          id integer primary key autoincrement,
          key text unique,
          workstream_id integer,
          person_id integer,
          workstream_key text,
          owner_key text,
          owner_display_name text,
          generated_at text,
          load_status text,
          action_count integer,
          product_action_count integer,
          validation_lead_count integer,
          model_or_rule_qa_count integer,
          critical_or_high_count integer,
          max_priority_score real,
          avg_priority_score real,
          decision_followup_count integer,
          validate_signal_count integer,
          ci_check_followup_count integer,
          review_wait_followup_count integer,
          coverage_limited_count integer,
          anonymous_observation_count integer,
          needs_human_review_count integer,
          top_action_type text,
          top_subjects text,
          recommended_focus text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table work_actions (
          id integer primary key autoincrement,
          key text unique,
          source_url text,
          latest_evidence_id integer,
          freshness_state text,
          confidence real,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text
        )
        """
    )
    conn.execute(
        """
        create table workstream_standup_sections (
          id integer primary key autoincrement,
          key text unique,
          workstream_health_snapshot_id integer,
          workstream_id integer,
          work_action_id integer,
          workstream_key text,
          generated_at text,
          section_rank integer,
          section_kind text,
          urgency text,
          owner_key text,
          subject_kind text,
          subject_key text,
          action_type text,
          status_signal text,
          summary text,
          recommended_action text,
          evidence_ref text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          latest_evidence_id integer,
          evidence_count integer,
          freshness_state text,
          visibility text,
          confidence real,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          created_at text,
          updated_at text
        )
        """
    )
    conn.execute(
        """
        create table workstream_tickets (
          id integer primary key autoincrement,
          workstream_ticket_kind text,
          evidence_count integer,
          event_count integer,
          first_seen_at text,
          last_activity_at text,
          rank_score real,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_updated_at text,
          content_hash text,
          deletion_state text,
          acl_state text,
          last_confirmed_at text,
          last_changed_at text,
          freshness_state text,
          visibility text,
          confidence real,
          created_at text,
          updated_at text,
          workstream_id integer,
          ticket_id integer,
          unique(workstream_id, ticket_id, workstream_ticket_kind)
        )
        """
    )


def empty_action_item_row(subject_key: str) -> dict[str, object]:
    row = {column: "" for column in brief.empty_action_items().columns}
    row.update(
        {
            "subject_kind": "pull_request",
            "subject_key": subject_key,
            "priority_score": 0,
            "raw_priority_score": 0,
            "severity_rank": 0,
            "score": 0,
            "confidence": 0,
            "open_review_request_count": 0,
            "reviewed_count": 0,
        }
    )
    return row


def forecast_pr_row(
    number: int,
    risk_score: float,
    overdue_days: float,
    author_login: str,
    risk_band: str = "critical",
) -> dict[str, object]:
    return {
        "repository": "repo/example",
        "pr_number": number,
        "pr_url": f"https://github.com/repo/example/pull/{number}",
        "title": f"Forecast target {number}",
        "state": "open",
        "risk_band": risk_band,
        "risk_score": risk_score,
        "overdue_days": overdue_days,
        "author_login": author_login,
        "source_current_coverage_state": "observed",
        "source_current_detail_state": "observed",
        "source_current_observed_at": "2026-06-21T05:00:00+00:00",
        "source_visibility": "public",
    }


def triage_row(insight_key: str, insight_kind: str) -> dict[str, object]:
    return {
        "review_id": 0,
        "insight_key": insight_key,
        "insight_kind": insight_kind,
        "producer_state": "current",
        "review_kind": "triage_request",
        "review_state": "requested",
        "truth_label": "unknown",
        "actionability_label": "unknown",
        "label_quality": "candidate",
        "measurement_eligible": "false",
        "reviewed_at": "",
    }


def label_row(review_id: int, insight_key: str, insight_kind: str) -> dict[str, object]:
    return {
        "review_id": review_id,
        "insight_key": insight_key,
        "insight_kind": insight_kind,
        "producer_state": "current",
        "review_kind": "evaluation_label",
        "review_state": "accepted",
        "truth_label": "true_positive",
        "actionability_label": "actionable",
        "label_quality": "gold",
        "measurement_eligible": "true",
        "reviewed_at": "2026-06-21T00:00:00+00:00",
    }


def gold_blocker_label_row(
    idx: int,
    truth_label: str,
    actionability_label: str,
    review_state: str,
) -> dict[str, object]:
    return {
        "review_id": 100 + idx,
        "insight_key": f"insight:blocker:{idx}",
        "insight_kind": "blocker_candidate",
        "subject_kind": "pull_request",
        "subject_key": f"repo/example#{idx}",
        "producer_state": "current",
        "review_kind": "evaluation_label",
        "review_state": review_state,
        "truth_label": truth_label,
        "actionability_label": actionability_label,
        "label_quality": "gold",
        "measurement_eligible": "true",
        "reviewer_kind": "imported",
        "reviewer_key": "codex_agent_adjudication",
        "reviewed_at": "2026-06-21T00:00:00+00:00",
    }


def insert_blocker_action(
    conn: sqlite3.Connection,
    action_key: str,
    *,
    action_type: str = "clear_blocker",
    decision_state: str = "product_action",
) -> int:
    cursor = conn.execute(
        """
        insert into work_actions (
          key, action_type, action_state, decision_state, subject_kind, subject_key,
          source_system, source_instance, external_kind, external_id
        ) values (?, ?, 'open', ?, 'pull_request',
                  'repo/example#9', 'cubicle_analytics', 'fixture-source',
                  'tpm_work_action', ?)
        """,
        (action_key, action_type, decision_state, action_key),
    )
    action_id = int(cursor.lastrowid)
    conn.execute(
        "insert into work_action_source_insights (work_action_id, work_insight_id) values (?, 1)",
        (action_id,),
    )
    return action_id


def blocker_action_item(
    action_key: str,
    title: str,
    *,
    action_type: str = "clear_blocker",
    decision_state: str = "product_action",
) -> dict[str, object]:
    return {
        "action_key": action_key,
        "action_type": action_type,
        "decision_state": decision_state,
        "subject_kind": "pull_request",
        "subject_key": "repo/example#9",
        "source_link_insight_kinds": "blocker_candidate",
        "owner_hint": "team:autoscaler",
        "title": title,
        "recommended_action": "Ask CI owner to confirm.",
        "why_now": "CI appears to be blocking merge.",
        "evidence_summary": "status_summary: failing check",
        "source_observation_status": "observed",
        "source_coverage_kind": "github_checks_complete",
        "source_url": "https://github.com/repo/example/pull/9/checks",
        "confidence": 0.86,
        "priority_score": 91,
        "score": 91,
        "urgency": "high",
    }


if __name__ == "__main__":
    unittest.main()
