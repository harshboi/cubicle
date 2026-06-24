#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import pathlib
import sqlite3
import sys
import tempfile
import unittest
import warnings
from datetime import datetime, timedelta, timezone

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


analytics = load_tool("flink_tpm_analytics")
brief = load_tool("flink_tpm_action_brief")


class FixtureManifestReadTest(unittest.TestCase):
    def test_reads_correlation_jira_issue_payloads_as_ticket_inputs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            fixture_dir = pathlib.Path(tmp)
            issue_path = fixture_dir / "jira" / "correlation" / "issues" / "FLINK-2.json"
            issue_path.parent.mkdir(parents=True)
            issue_path.write_text(
                '{"key":"FLINK-2","fields":{"summary":"Correlation issue"}}',
                encoding="utf-8",
            )
            manifest = [
                analytics.ManifestEntry(
                    path="jira/correlation/issues/FLINK-2.json",
                    source="jira",
                    object_type="jira_correlation_issue",
                    object_id="FLINK-2",
                    status_code=200,
                )
            ]

            payloads = analytics.read_jira_payloads(fixture_dir, manifest, cleaned_issue_keys=None)

        self.assertEqual(sorted(payloads), ["FLINK-2"])
        self.assertEqual(payloads["FLINK-2"]["fields"]["summary"], "Correlation issue")


class MilestoneSignalTest(unittest.TestCase):
    def test_source_milestones_do_not_convert_release_targets_to_delivery_commitments(self) -> None:
        generated_at = "2026-06-23T00:00:00+00:00"
        jira_payloads = {
            "FLINK-1": {
                "self": "https://issues.apache.org/jira/rest/api/2/issue/FLINK-1",
                "fields": {
                    "updated": "2026-06-21T00:00:00.000+0000",
                    "duedate": "2026-06-30",
                    "resolutiondate": "2026-06-20T00:00:00.000+0000",
                    "fixVersions": [{"name": "2.4.0", "releaseDate": "2026-07-01", "released": False}],
                },
            },
            "FLINK-2": {
                "fields": {
                    "updated": "2026-06-21T00:00:00.000+0000",
                    "fixVersions": [{"name": "2.5.0", "released": False}],
                },
            },
        }
        pr_payloads = {
            "apache/flink#1": {
                "html_url": "https://github.com/apache/flink/pull/1",
                "updated_at": "2026-06-21T00:00:00Z",
                "milestone": {"title": "GitHub release", "due_on": "2026-07-15T00:00:00Z"},
            }
        }

        rows = analytics.build_milestone_signals(jira_payloads, pr_payloads, "fixture-source", generated_at)

        self.assertEqual(int((rows["commitment_strength"] == "explicit_commitment").sum()), 1)
        self.assertEqual(int((rows["delivery_commitment_allowed"] == 1).sum()), 1)
        release_rows = rows[rows["commitment_strength"] == "release_signal"]
        self.assertEqual(len(release_rows), 3)
        self.assertTrue((release_rows["delivery_commitment_allowed"] == 0).all())
        dated_release = release_rows[release_rows["target_date"].notna() & (release_rows["target_date"].astype(str).str.len() > 0)]
        self.assertEqual(len(dated_release), 2)
        self.assertTrue((dated_release["date_claim_allowed"] == 1).all())
        undated = rows[rows["source_payload_key"] == "FLINK-2:fields.fixVersions:2.5.0"].iloc[0]
        self.assertEqual(undated["milestone_state"], "no_target_date")
        self.assertEqual(int(undated["date_claim_allowed"]), 0)
        outcome = rows[rows["commitment_strength"] == "outcome_evidence"].iloc[0]
        self.assertEqual(outcome["milestone_kind"], "resolution_outcome")
        self.assertEqual(int(outcome["delivery_commitment_allowed"]), 0)


class SourceCoverageReadTest(unittest.TestCase):
    def test_current_pr_source_coverage_uses_latest_durable_coverage_run(self) -> None:
        conn = sqlite3.connect(":memory:")
        conn.executescript(
            """
            create table source_scopes (
              id integer primary key,
              scope_key text not null
            );
            create table source_sync_runs (
              id integer primary key,
              source_scope_id integer not null,
              run_key text not null,
              status text not null,
              coverage_mode text not null,
              started_at text,
              completed_at text,
              created_at text
            );
            create table source_sync_issues (
              id integer primary key,
              source_sync_run_id integer not null,
              source_system text,
              external_kind text,
              external_id text,
              issue_code text,
              message text
            );
            insert into source_scopes (id, scope_key) values (1, 'fixture-stream');
            insert into source_sync_runs (
              id, source_scope_id, run_key, status, coverage_mode, started_at, completed_at, created_at
            ) values
              (1, 1, 'base-partial', 'partial', 'partial_scope', '2026-06-21T08:00:00+00:00', '2026-06-21T08:10:00+00:00', '2026-06-21T08:00:00+00:00'),
              (2, 1, 'live-clean', 'complete', 'live_only', '2026-06-21T09:00:00+00:00', '2026-06-21T09:01:00+00:00', '2026-06-21T09:00:00+00:00');
            insert into source_sync_issues (
              id, source_sync_run_id, source_system, external_kind, external_id, issue_code, message
            ) values
              (1, 1, 'github', 'github_pull_request_reviews', 'repo/example#1', 'source_missing_snapshot', 'review snapshot missing');
            """
        )

        rows = analytics.read_current_fixture_pr_source_coverage(conn, "fixture-stream")

        self.assertEqual(len(rows), 1)
        row = rows.iloc[0].to_dict()
        self.assertEqual(row["subject_key"], "repo/example#1")
        self.assertEqual(int(row["source_current_issue_count"]), 1)
        self.assertEqual(int(row["source_current_detail_issue_count"]), 1)
        self.assertEqual(row["source_current_sync_run_key"], "base-partial")
        self.assertEqual(row["source_current_coverage_mode"], "partial_scope")


class TimeSeriesTransitionTest(unittest.TestCase):
    def test_two_observation_history_derives_terminal_and_coverage_transitions(self) -> None:
        conn = sqlite3.connect(":memory:")
        first_observed_at = "2026-06-20T00:00:00+00:00"
        second_observed_at = "2026-06-21T00:00:00+00:00"

        first_prs = pr_features(
            [
                pr_row(1, "open", "fresh"),
                pr_row(2, "open", "fresh"),
                pr_row(3, "open", "partial"),
            ]
        )
        second_prs = pr_features(
            [
                pr_row(1, "open", "fresh"),
                pr_row(2, "merged", "fresh", merged_at=second_observed_at, cycle_time_days=4.0),
                pr_row(3, "open", "fresh"),
            ]
        )

        first_tickets = ticket_features([ticket_row("FLINK-1", "open")])
        second_tickets = ticket_features([ticket_row("FLINK-1", "closed")])

        analytics.persist_time_series_snapshots(conn, "fixture-source", first_observed_at, first_prs, first_tickets)
        first_summary = analytics.persist_time_series_snapshots(conn, "fixture-source", second_observed_at, second_prs, second_tickets)
        second_summary = analytics.persist_time_series_snapshots(conn, "fixture-source", second_observed_at, second_prs, second_tickets)

        rows = conn.execute(
            """
            select subject_kind, subject_key, from_state, to_state, transition_kind, confidence
              from tpm_state_transition_candidates
             order by subject_kind, subject_key
            """
        ).fetchall()
        self.assertEqual(
            rows,
            [
                ("pull_request", "repo/example#2", "open", "merged", "terminal_state_change", 0.95),
                ("pull_request", "repo/example#3", "open", "open", "coverage_state_change", 0.85),
                ("ticket", "FLINK-1", "open", "closed", "terminal_state_change", 0.9),
            ],
        )
        self.assertEqual(metric(first_summary, "observed_snapshot_time_count"), "2")
        self.assertEqual(metric(first_summary, "pr_feature_snapshot_observed_time_count"), "2")
        self.assertEqual(metric(first_summary, "as_of_feature_snapshot_training_example_count"), "1")
        self.assertEqual(metric(first_summary, "as_of_feature_snapshot_terminal_subject_count"), "1")
        self.assertEqual(metric(first_summary, "as_of_feature_snapshot_ready"), "false")
        self.assertEqual(metric(first_summary, "as_of_feature_snapshot_state"), "insufficient_pre_terminal_training_examples")
        self.assertEqual(metric(first_summary, "transition_candidate_count"), "3")
        self.assertEqual(metric(first_summary, "terminal_transition_candidate_count"), "2")
        self.assertEqual(metric(second_summary, "transition_candidate_count"), "3")
        readiness = pd.read_sql_query("select * from tpm_transition_signal_readiness", conn)
        readiness_by_key = {row["readiness_key"]: row for row in readiness.to_dict("records")}
        self.assertTrue(readiness_by_key["terminal_closeout_review"]["ready"])
        self.assertEqual(readiness_by_key["terminal_closeout_review"]["readiness_state"], "ready_with_latest_terminal_transition")
        self.assertFalse(readiness_by_key["source_resolved_closeout"]["ready"])
        self.assertEqual(readiness_by_key["source_resolved_closeout"]["readiness_state"], "needs_authenticated_current_state")

        transition_candidates = pd.read_sql_query(
            "select * from tpm_state_transition_candidates order by subject_key",
            conn,
        )
        actions = brief.append_transition_resolution_actions(
            pd.DataFrame(),
            transition_candidates,
            second_prs,
            second_observed_at,
        )
        self.assertEqual(len(actions), 2)
        closeout_by_subject = {row.subject_key: row for row in actions.itertuples(index=False)}
        self.assertEqual(set(closeout_by_subject), {"repo/example#2", "FLINK-1"})
        pr_action = closeout_by_subject["repo/example#2"]
        ticket_action = closeout_by_subject["FLINK-1"]
        self.assertEqual(pr_action.action_type, "verify_resolution")
        self.assertEqual(pr_action.decision_state, "closeout_review")
        self.assertIn("open to merged", pr_action.why_now)
        self.assertEqual(ticket_action.action_type, "verify_resolution")
        self.assertEqual(ticket_action.decision_state, "closeout_review")
        self.assertIn("open to closed", ticket_action.why_now)

    def test_transition_readiness_blocks_superseded_terminal_closeouts(self) -> None:
        conn = sqlite3.connect(":memory:")
        first_observed_at = "2026-06-20T00:00:00+00:00"
        second_observed_at = "2026-06-21T00:00:00+00:00"
        third_observed_at = "2026-06-22T00:00:00+00:00"

        analytics.persist_time_series_snapshots(
            conn,
            "fixture-source",
            first_observed_at,
            pr_features([pr_row(1, "open", "fresh")]),
            ticket_features([]),
        )
        analytics.persist_time_series_snapshots(
            conn,
            "fixture-source",
            second_observed_at,
            pr_features([pr_row(1, "merged", "fresh", merged_at=second_observed_at, cycle_time_days=2.0)]),
            ticket_features([]),
        )
        analytics.persist_time_series_snapshots(
            conn,
            "fixture-source",
            third_observed_at,
            pr_features([pr_row(1, "open", "fresh")]),
            ticket_features([]),
        )

        readiness = pd.read_sql_query("select * from tpm_transition_signal_readiness", conn)
        by_key = {row["readiness_key"]: row for row in readiness.to_dict("records")}
        self.assertFalse(by_key["terminal_closeout_review"]["ready"])
        self.assertEqual(by_key["terminal_closeout_review"]["readiness_state"], "terminal_transition_superseded")
        self.assertEqual(int(by_key["terminal_closeout_review"]["superseded_terminal_transition_count"]), 1)
        self.assertFalse(by_key["source_resolved_closeout"]["ready"])
        self.assertEqual(by_key["source_resolved_closeout"]["readiness_state"], "blocked_by_later_nonterminal_state")

    def test_as_of_feature_snapshot_readiness_requires_labelled_preterminal_examples(self) -> None:
        conn = sqlite3.connect(":memory:")
        first_observed_at = "2026-06-20T00:00:00+00:00"
        second_observed_at = "2026-06-21T00:00:00+00:00"
        first_prs = pr_features([pr_row(number, "open", "fresh") for number in range(1, 11)])
        second_prs = pr_features(
            [
                pr_row(number, "merged", "fresh", merged_at=second_observed_at, cycle_time_days=4.0 + number)
                for number in range(1, 11)
            ]
        )

        analytics.persist_time_series_snapshots(conn, "fixture-source", first_observed_at, first_prs, empty_ticket_features())
        summary = analytics.persist_time_series_snapshots(conn, "fixture-source", second_observed_at, second_prs, empty_ticket_features())

        self.assertEqual(metric(summary, "pr_feature_snapshot_observed_time_count"), "2")
        self.assertEqual(metric(summary, "as_of_feature_snapshot_training_example_count"), "10")
        self.assertEqual(metric(summary, "as_of_feature_snapshot_terminal_subject_count"), "10")
        self.assertEqual(metric(summary, "as_of_feature_snapshot_ready"), "true")
        self.assertEqual(metric(summary, "as_of_feature_snapshot_state"), "ready")

    def test_event_replay_feature_snapshots_use_safe_dynamic_counts(self) -> None:
        conn = sqlite3.connect(":memory:")
        observed_at = "2026-06-21T00:00:00+00:00"
        current_prs = pr_features(
            [
                pr_row(number, "merged", "fresh", merged_at=observed_at, cycle_time_days=4.0 + number)
                for number in range(1, 11)
            ]
        )
        current_prs["linked_ticket_count"] = 3
        current_prs["issue_key_text_count"] = 2
        detail_payloads = {
            f"repo/example#{number}": {
                "commits": [
                    {"commit": {"author": {"date": "2026-06-18T00:00:00Z"}, "committer": {"date": "2026-06-18T00:00:00Z"}}},
                    {"commit": {"author": {"date": "2026-06-19T00:00:00Z"}, "committer": {"date": "2026-06-19T00:00:00Z"}}},
                ],
                "issue_comments": [{"created_at": "2026-06-18T12:00:00Z"}],
                "review_comments": [{"created_at": "2026-06-19T12:00:00Z"}],
                "reviews": [{"submitted_at": "2026-06-20T00:00:00Z"}],
            }
            for number in range(1, 11)
        }
        event_snapshots = analytics.build_event_pr_feature_snapshots(current_prs, detail_payloads)

        summary = analytics.persist_time_series_snapshots(
            conn,
            "fixture-source",
            observed_at,
            current_prs,
            empty_ticket_features(),
            event_pr_feature_snapshots=event_snapshots,
        )

        self.assertEqual(metric(summary, "as_of_feature_snapshot_ready"), "true")
        self.assertEqual(metric(summary, "as_of_feature_snapshot_state"), "ready_source_event_replay")
        self.assertEqual(metric(summary, "as_of_feature_snapshot_basis"), "source_event_replay")
        self.assertEqual(metric(summary, "event_replay_pr_feature_snapshot_subject_count"), "10")
        self.assertGreater(int(metric(summary, "event_replay_pr_feature_snapshot_count")), 10)
        row = conn.execute(
            """
            select commits, comments, review_comments, linked_ticket_count, issue_key_text_count,
                   additions, additions_missing,
                   changed_files, changed_files_missing, lifecycle_fields_source,
                   churn_fields_source
              from tpm_pr_feature_snapshots
             where subject_key = 'repo/example#1'
               and observed_at = '2026-06-19T00:00:00+00:00'
            """
        ).fetchone()
        self.assertEqual(
            row,
            (
                2,
                1,
                0,
                0,
                0,
                0,
                1,
                0,
                1,
                "source_event_replay",
                "source_event_replay_safe_dynamic_counts_only",
            ),
        )


class DeveloperCorrelationTest(unittest.TestCase):
    def test_direct_identity_bridge_produces_guardrailed_correlation_lead(self) -> None:
        pr_authorships = pd.DataFrame(
            [
                {
                    "person_key": "person:jira:owner",
                    "display_name": "Owner One",
                    "github_login": "owner",
                    "jira_account_id": "owner",
                    "repository": "repo/example",
                    "pr_number": 1,
                    "pr_key": "pull-request:repo/example#1",
                    "title": "Risky PR",
                    "state": "open",
                    "pr_url": "https://github.com/repo/example/pull/1",
                },
                {
                    "person_key": "person:github:unbridged",
                    "display_name": "Unbridged",
                    "github_login": "unbridged",
                    "jira_account_id": "",
                    "repository": "repo/example",
                    "pr_number": 2,
                    "pr_key": "pull-request:repo/example#2",
                    "title": "Other PR",
                    "state": "open",
                    "pr_url": "https://github.com/repo/example/pull/2",
                },
            ]
        )
        ticket_roles = pd.DataFrame(
            [
                {
                    "person_key": "person:jira:owner",
                    "display_name": "Owner One",
                    "github_login": "owner",
                    "jira_account_id": "owner",
                    "assignment_kind": "assignee",
                    "ticket_key": "FLINK-100",
                    "external_kind": "jira_correlation_issue",
                    "title": "Extra same-window bug",
                    "status": "Open",
                    "priority": "Major",
                    "source_url": "https://issues.apache.org/jira/browse/FLINK-100",
                    "source_updated_at": "2026-06-21T00:00:00+00:00",
                },
                {
                    "person_key": "person:github:unbridged",
                    "display_name": "Unbridged",
                    "github_login": "unbridged",
                    "jira_account_id": "",
                    "assignment_kind": "reporter",
                    "ticket_key": "FLINK-200",
                    "external_kind": "jira_correlation_issue",
                    "title": "Ambiguous extra ticket",
                    "status": "Open",
                    "priority": "Major",
                    "source_url": "https://issues.apache.org/jira/browse/FLINK-200",
                    "source_updated_at": "2026-06-21T00:00:00+00:00",
                },
            ]
        )
        forecasts = pd.DataFrame(
            [
                {
                    "repository": "repo/example",
                    "pr_number": 1,
                    "state": "open",
                    "risk_band": "critical",
                    "risk_score": 95,
                    "age_days": 10,
                    "stale_days": 5,
                    "source_current_coverage_state": "coverage_limited",
                },
                {
                    "repository": "repo/example",
                    "pr_number": 2,
                    "state": "open",
                    "risk_band": "critical",
                    "risk_score": 95,
                    "age_days": 10,
                    "stale_days": 5,
                    "source_current_coverage_state": "coverage_limited",
                },
            ]
        )
        tickets = pd.DataFrame(
            [
                {"ticket_key": "FLINK-100", "blocker_keyword_count": 1, "linked_pr_count": 0, "comment_count": 1, "participant_count": 1},
                {"ticket_key": "FLINK-200", "blocker_keyword_count": 1, "linked_pr_count": 0, "comment_count": 1, "participant_count": 1},
            ]
        )

        rows = analytics.build_developer_correlation(pr_authorships, ticket_roles, forecasts, tickets)
        by_person = {row.person_key: row for row in rows.itertuples(index=False)}

        bridged = by_person["person:jira:owner"]
        self.assertEqual(bridged.identity_bridge_state, "direct_github_jira_person")
        self.assertEqual(bridged.identity_match_method, "typed_person_github_and_jira_fields")
        self.assertEqual(bridged.identity_evidence_count, 2)
        self.assertEqual(bridged.identity_conflict_count, 0)
        self.assertEqual(bridged.source_coverage_state, "coverage_limited")
        self.assertEqual(bridged.source_object_type_counts, "github_pull_request:1,jira_issue:0,jira_correlation_issue:1")
        self.assertEqual(bridged.correlation_state, "correlatable_same_identity")
        self.assertEqual(bridged.high_risk_open_pr_count, 1)
        self.assertEqual(bridged.extra_jira_ticket_count, 1)
        self.assertIn("never proves causality", bridged.guardrail)
        self.assertIn("performance", bridged.guardrail)

        unbridged = by_person["person:github:unbridged"]
        self.assertEqual(unbridged.identity_bridge_state, "github_only_person")
        self.assertEqual(unbridged.identity_match_method, "typed_person_github_only")
        self.assertEqual(unbridged.correlation_state, "unbridged_overlap_not_product_claim")

        cards = analytics.build_developer_correlation_cards(rows)
        self.assertEqual(len(cards), 1)
        self.assertEqual(cards[0]["insight_kind"], "developer_correlation")
        self.assertEqual(cards[0]["subject_kind"], "unknown")
        self.assertEqual(cards[0]["subject_key"], "person:jira:owner")
        self.assertIn("not causality", cards[0]["score_explanation"])
        self.assertIn("performance", cards[0]["details"])

    def test_developer_correlation_validation_reports_aggregate_signal_guardrails(self) -> None:
        rows = pd.DataFrame(
            [
                {
                    "identity_bridge_state": "direct_github_jira_person",
                    "pr_authored_count": 4,
                    "open_pr_authored_count": 1,
                    "high_risk_open_pr_count": 0,
                    "extra_jira_ticket_count": 1,
                    "open_extra_jira_ticket_count": 0,
                    "same_window_ticket_pressure": 0.0,
                },
                {
                    "identity_bridge_state": "direct_github_jira_person",
                    "pr_authored_count": 6,
                    "open_pr_authored_count": 1,
                    "high_risk_open_pr_count": 1,
                    "extra_jira_ticket_count": 3,
                    "open_extra_jira_ticket_count": 2,
                    "same_window_ticket_pressure": 0.33,
                },
                {
                    "identity_bridge_state": "direct_github_jira_person",
                    "pr_authored_count": 7,
                    "open_pr_authored_count": 2,
                    "high_risk_open_pr_count": 1,
                    "extra_jira_ticket_count": 4,
                    "open_extra_jira_ticket_count": 3,
                    "same_window_ticket_pressure": 0.43,
                },
                {
                    "identity_bridge_state": "direct_github_jira_person",
                    "pr_authored_count": 8,
                    "open_pr_authored_count": 3,
                    "high_risk_open_pr_count": 2,
                    "extra_jira_ticket_count": 6,
                    "open_extra_jira_ticket_count": 4,
                    "same_window_ticket_pressure": 0.5,
                },
                {
                    "identity_bridge_state": "direct_github_jira_person",
                    "pr_authored_count": 9,
                    "open_pr_authored_count": 4,
                    "high_risk_open_pr_count": 3,
                    "extra_jira_ticket_count": 8,
                    "open_extra_jira_ticket_count": 6,
                    "same_window_ticket_pressure": 0.67,
                },
                {
                    "identity_bridge_state": "github_only_person",
                    "pr_authored_count": 20,
                    "open_pr_authored_count": 10,
                    "high_risk_open_pr_count": 10,
                    "extra_jira_ticket_count": 20,
                    "open_extra_jira_ticket_count": 20,
                    "same_window_ticket_pressure": 1.0,
                },
            ]
        )

        validation = analytics.build_developer_correlation_validation(rows)
        by_metric = {row.metric: row for row in validation.itertuples(index=False)}

        self.assertEqual(by_metric["direct_identity_sample_count"].value, "5")
        self.assertEqual(by_metric["direct_identity_pr_and_extra_jira_count"].value, "5")
        self.assertEqual(by_metric["spearman_open_extra_jira_vs_high_risk_open_pr"].sample_count, 5)
        self.assertGreater(float(by_metric["spearman_open_extra_jira_vs_high_risk_open_pr"].value), 0.8)
        self.assertIn("never proves causality", by_metric["top_quartile_high_risk_open_pr_lift"].guardrail)


class WorkInsightReviewQueueTest(unittest.TestCase):
    def test_stored_measurement_flag_does_not_promote_adversarial_or_smoke_labels(self) -> None:
        conn = sqlite3.connect(":memory:")
        conn.execute(
            """
            create table work_insights (
              id integer primary key,
              key text,
              insight_kind text,
              severity text,
              subject_kind text,
              subject_key text,
              score real,
              confidence real,
              rank_score real,
              producer_state text,
              source_system text,
              source_instance text,
              external_kind text
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
              source_url text
            )
            """
        )
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, subject_kind, subject_key,
              score, confidence, rank_score, producer_state, source_system,
              source_instance, external_kind
            ) values
              (1, 'insight:gold', 'blocker_candidate', 'high', 'pull_request', 'repo/example#1', 90, 0.9, 90, 'current', 'cubicle_analytics', 'fixture-source', 'tpm_insight'),
              (2, 'insight:smoke', 'blocker_candidate', 'medium', 'pull_request', 'repo/example#2', 70, 0.8, 70, 'current', 'cubicle_analytics', 'fixture-source', 'tpm_insight'),
              (3, 'insight:adversarial', 'blocker_candidate', 'medium', 'pull_request', 'repo/example#3', 69, 0.8, 69, 'current', 'cubicle_analytics', 'fixture-source', 'tpm_insight'),
              (4, 'insight:gold-counted', 'blocker_candidate', 'high', 'pull_request', 'repo/example#4', 91, 0.95, 91, 'current', 'cubicle_analytics', 'fixture-source', 'tpm_insight')
            """
        )
        conn.execute(
            """
            insert into work_insight_reviews (
              id, key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible,
              reviewer_kind, reviewer_key, owner_key, next_action, rationale,
              reviewed_at, source_url
            ) values
              (10, 'review:gold', 1, 'evaluation_label', 'accepted', 'true_positive',
               'actionable', 'agent_gold', 'gold', 0, 'imported', 'judge',
               '', '', '', '2026-06-22T00:00:00+00:00', 'labels.tsv'),
              (11, 'review:smoke', 2, 'evaluation_label', 'accepted', 'true_positive',
               'actionable', 'agent_smoke', 'smoke', 1, 'imported', 'smoke',
               '', '', '', '2026-06-22T00:00:00+00:00', 'labels.tsv'),
              (12, 'review:adversarial', 3, 'evaluation_label', 'dismissed', 'false_positive',
               'not_actionable', 'agent_adversarial', 'adversarial', 1, 'imported', 'adversarial',
               '', '', '', '2026-06-22T00:00:00+00:00', 'labels.tsv'),
              (13, 'review:gold-counted', 4, 'evaluation_label', 'accepted', 'true_positive',
               'actionable', 'agent_gold', 'gold', 1, 'imported', 'judge',
               '', '', '', '2026-06-22T00:00:00+00:00', 'labels.tsv')
            """
        )

        rows = analytics.read_work_insight_review_queue(conn, "fixture-source", set())
        by_key = {row.insight_key: row.measurement_eligible for row in rows.itertuples(index=False)}
        self.assertEqual(by_key["insight:gold"], "false")
        self.assertEqual(by_key["insight:gold-counted"], "true")
        self.assertEqual(by_key["insight:smoke"], "false")
        self.assertEqual(by_key["insight:adversarial"], "false")
        self.assertEqual(len(analytics.measurement_label_rows(rows)), 1)

        promoted = analytics.read_work_insight_review_queue(conn, "fixture-source", {"agent_adversarial"})
        promoted_by_key = {row.insight_key: row.measurement_eligible for row in promoted.itertuples(index=False)}
        self.assertEqual(promoted_by_key["insight:adversarial"], "false")

    def test_stored_explicit_false_blocks_allowlisted_label_set(self) -> None:
        row = pd.Series(
            {
                "review_kind": "evaluation_label",
                "label_quality": "candidate",
                "label_set": "source_oracle_seed",
                "stored_measurement_eligible": "0",
            }
        )

        self.assertFalse(analytics.is_measurement_label(row, {"source_oracle_seed"}))


class ForecastRiskBacktestTest(unittest.TestCase):
    def test_forecast_feature_leakage_guard_rejects_labels_decisions_and_future_fields(self) -> None:
        for leaked in [
            "truth_label",
            "actionability_label",
            "decision_state",
            "program_status",
            "merged_at",
            "risk_score",
            "created_month_index",
            "created_quarter",
            "calendar_week",
        ]:
            with self.subTest(leaked=leaked):
                with self.assertRaises(ValueError):
                    analytics.assert_forecast_feature_columns(["additions", leaked])

    def test_build_forecasts_reports_allowlisted_feature_guard(self) -> None:
        rows = [
            forecast_feature_pr_row(1, "merged", 3.0),
            forecast_feature_pr_row(2, "merged", 9.0),
            forecast_feature_pr_row(3, "open", None),
        ]

        summary, forecasts, backtest, risk_backtest = analytics.build_forecasts(pd.DataFrame(rows))
        summary_metrics = {row.metric: row.value for row in summary.itertuples(index=False)}

        self.assertEqual(summary_metrics["forecast_feature_leakage_guard"], "passed")
        self.assertEqual(summary_metrics["forecast_calendar_feature_guard"], "passed")
        self.assertEqual(summary_metrics["eta_readiness_state"], "blocked")
        self.assertEqual(summary_metrics["eta_primary_blocker"], "insufficient_merged_pr_sample")
        self.assertEqual(summary_metrics["eta_next_evidence_needed"], "collect_repeated_as_of_pr_snapshots_and_closed_outcomes")
        self.assertIn("additions", summary_metrics["forecast_feature_set"])
        self.assertNotIn("truth_label", summary_metrics["forecast_feature_set"])
        self.assertEqual(len(forecasts), 3)
        self.assertFalse(backtest.empty)
        self.assertFalse(risk_backtest.empty)

    def test_forecast_feature_set_readiness_matrix_quarantines_calendar_probe(self) -> None:
        rows = [
            forecast_feature_pr_row(number, "merged", float(2 + (number % 8)))
            for number in range(1, 25)
        ]

        matrix = analytics.build_forecast_feature_set_readiness_matrix(
            pd.DataFrame(rows),
            temporal_feature_snapshot_ready=True,
        )

        self.assertEqual(
            set(matrix["feature_set_key"]),
            {
                "production_source_safe",
                "source_safe_derived_no_calendar",
                "created_time_probe_quarantined",
            },
        )
        calendar_rows = matrix[matrix["feature_set_key"] == "created_time_probe_quarantined"]
        self.assertFalse(calendar_rows.empty)
        self.assertEqual(set(calendar_rows["guardrail_state"]), {"quarantined_calendar_cohort"})
        self.assertEqual(set(calendar_rows["eta_promotable"]), {"false"})
        self.assertTrue((calendar_rows["note"].str.contains("calendar-cohort probe only")).all())

    def test_build_forecasts_reports_source_event_as_of_backtest(self) -> None:
        base = datetime(2026, 1, 1, tzinfo=timezone.utc)
        rows = []
        snapshots = []
        for idx in range(12):
            number = idx + 1
            cycle_days = float(2 + (idx % 5))
            created_at = base + timedelta(days=idx)
            terminal_at = created_at + timedelta(days=cycle_days)
            row = forecast_feature_pr_row(number, "merged", cycle_days)
            row.update(
                {
                    "created_at": created_at.isoformat(),
                    "merged_at": terminal_at.isoformat(),
                    "closed_at": terminal_at.isoformat(),
                    "age_days": cycle_days,
                    "cycle_time_days": cycle_days,
                }
            )
            rows.append(row)
            snapshots.append(event_snapshot_pr_row(number, created_at, terminal_at, cycle_days, observed_offset_days=0.25))
            snapshots.append(event_snapshot_pr_row(number, created_at, terminal_at, cycle_days, observed_offset_days=0.5))

        summary, _, backtest, _ = analytics.build_forecasts(
            pd.DataFrame(rows),
            temporal_feature_snapshot_ready=True,
            event_pr_feature_snapshots=pd.DataFrame(snapshots),
        )
        summary_metrics = {row.metric: row.value for row in summary.itertuples(index=False)}

        self.assertEqual(summary_metrics["source_event_as_of_backtest_state"], "baseline_available")
        self.assertEqual(summary_metrics["source_event_as_of_subject_count"], "12")
        self.assertEqual(summary_metrics["source_event_as_of_training_example_count"], "24")
        self.assertIn("source_event_as_of_kfold", set(backtest["evaluation"]))
        self.assertIn("source_event_as_of_chronological_holdout", set(backtest["evaluation"]))
        self.assertTrue((backtest[backtest["evaluation"].str.startswith("source_event_as_of")]["ready_for_eta"] == "false").all())

    def test_tpm_decision_target_backtest_reports_abandonment_risk_without_product_action(self) -> None:
        base = datetime(2026, 1, 1, tzinfo=timezone.utc)
        snapshots = []
        for idx in range(24):
            number = idx + 1
            abandoned = idx % 4 == 0
            cycle_days = float(4 + (idx % 6))
            created_at = base + timedelta(days=idx)
            terminal_at = created_at + timedelta(days=cycle_days)
            snapshots.append(
                event_snapshot_pr_row(
                    number,
                    created_at,
                    terminal_at,
                    cycle_days,
                    observed_offset_days=0.25,
                    is_merged=not abandoned,
                )
            )
            snapshots.append(
                event_snapshot_pr_row(
                    number,
                    created_at,
                    terminal_at,
                    cycle_days,
                    observed_offset_days=1.0,
                    is_merged=not abandoned,
                )
            )

        backtest = analytics.build_tpm_decision_target_backtest(
            pd.DataFrame(snapshots),
            analytics.forecast_feature_columns(),
        )

        self.assertIn("source_event_as_of_grouped_kfold", set(backtest["evaluation"]))
        self.assertIn("source_event_as_of_chronological_holdout", set(backtest["evaluation"]))
        self.assertIn("abandonment_heuristic", set(backtest["model"]))
        self.assertTrue((backtest["target_kind"] == "abandonment_risk").all())
        self.assertTrue((backtest["ready_for_product_action"] == "false").all())
        self.assertGreater(backtest["positive_count"].fillna(0).max(), 0)

    def test_tpm_decision_target_readiness_keeps_unstable_signal_validation_gated(self) -> None:
        backtest = pd.DataFrame(
            [
                decision_target_row("source_event_as_of_grouped_kfold", "random_forest_classifier", 1, 0.3466),
                decision_target_row("source_event_as_of_grouped_kfold", "random_forest_classifier", 2, 0.3169),
                decision_target_row("source_event_as_of_grouped_kfold", "random_forest_classifier", 3, 0.2983),
                decision_target_row("source_event_as_of_grouped_kfold", "random_forest_classifier", 4, -0.0086),
                decision_target_row("source_event_as_of_grouped_kfold", "random_forest_classifier", 5, 0.2569),
                decision_target_row(
                    "source_event_as_of_chronological_holdout",
                    "random_forest_classifier",
                    1,
                    0.3758,
                    precision=0.5789,
                    roc_auc=0.6748,
                ),
                decision_target_row(
                    "source_event_as_of_coverage_stratified_summary",
                    "coverage_guardrail",
                    0,
                    None,
                    coverage_stratum="not_testable_single_stratum",
                ),
                decision_target_row(
                    "source_event_as_of_coverage_stratum",
                    "random_forest_classifier_oof",
                    0,
                    0.2213,
                    coverage_stratum="coverage=observed;detail=observed",
                ),
            ]
        )

        readiness = analytics.build_tpm_decision_target_readiness(backtest)
        row = readiness[readiness["model"] == "random_forest_classifier"].iloc[0]

        self.assertEqual(row["same_model_validation_gate"], "gated")
        self.assertEqual(row["product_action_gate_state"], "validation_gated")
        self.assertEqual(row["product_action_ready"], "false")
        self.assertEqual(row["validation_ready"], "false")
        self.assertEqual(row["coverage_ready"], "false")
        self.assertEqual(row["independent_evidence_ready"], "false")
        self.assertEqual(row["owner_policy_ready"], "false")
        self.assertEqual(row["coverage_gate_state"], "not_testable_single_stratum")
        self.assertEqual(row["recommended_next_evidence"], "stabilize_grouped_kfold_signal_across_developer_or_time_splits")

    def test_tpm_decision_target_readiness_keeps_product_action_gated_after_model_and_coverage_pass(self) -> None:
        backtest = pd.DataFrame(
            [
                decision_target_row("source_event_as_of_grouped_kfold", "random_forest_classifier", 1, 0.24),
                decision_target_row("source_event_as_of_grouped_kfold", "random_forest_classifier", 2, 0.18),
                decision_target_row("source_event_as_of_grouped_kfold", "random_forest_classifier", 3, 0.16),
                decision_target_row(
                    "source_event_as_of_chronological_holdout",
                    "random_forest_classifier",
                    1,
                    0.21,
                    precision=0.52,
                    roc_auc=0.72,
                ),
                decision_target_row(
                    "source_event_as_of_coverage_stratified_summary",
                    "coverage_guardrail",
                    0,
                    None,
                    coverage_stratum="stratified",
                ),
                decision_target_row(
                    "source_event_as_of_coverage_stratum",
                    "random_forest_classifier_oof",
                    0,
                    0.12,
                    coverage_stratum="coverage=observed;detail=observed",
                ),
                decision_target_row(
                    "source_event_as_of_coverage_stratum",
                    "random_forest_classifier_oof",
                    0,
                    0.11,
                    coverage_stratum="coverage=detail_failed;detail=failed",
                ),
            ]
        )

        readiness = analytics.build_tpm_decision_target_readiness(backtest)
        row = readiness[readiness["model"] == "random_forest_classifier"].iloc[0]

        self.assertEqual(row["same_model_validation_gate"], "passed")
        self.assertEqual(row["validation_ready"], "true")
        self.assertEqual(row["coverage_ready"], "true")
        self.assertEqual(row["independent_evidence_ready"], "false")
        self.assertEqual(row["owner_policy_ready"], "false")
        self.assertEqual(row["product_action_gate_state"], "evidence_gated")
        self.assertEqual(row["product_action_ready"], "false")
        self.assertEqual(row["coverage_stratum_count"], 2)
        self.assertEqual(row["recommended_next_evidence"], "attach_independent_non_generated_evidence_before_product_action")

    def test_tpm_decision_target_backtest_reports_coverage_stratified_validation(self) -> None:
        base = datetime(2026, 1, 1, tzinfo=timezone.utc)
        snapshots = []
        for idx in range(48):
            number = idx + 1
            abandoned = idx % 4 == 0
            cycle_days = float(4 + (idx % 6))
            created_at = base + timedelta(days=idx)
            terminal_at = created_at + timedelta(days=cycle_days)
            coverage_state = "observed" if idx < 24 else "detail_failed"
            detail_state = "observed" if idx < 24 else "failed"
            for offset in (0.25, 1.0):
                row = event_snapshot_pr_row(
                    number,
                    created_at,
                    terminal_at,
                    cycle_days,
                    observed_offset_days=offset,
                    is_merged=not abandoned,
                    source_current_coverage_state=coverage_state,
                    source_current_detail_state=detail_state,
                )
                if abandoned:
                    row["comments"] = 8
                    row["review_comments"] = 3
                    row["commits"] = 0
                snapshots.append(row)

        backtest = analytics.build_tpm_decision_target_backtest(
            pd.DataFrame(snapshots),
            analytics.forecast_feature_columns(),
        )

        summary = backtest[backtest["evaluation"] == "source_event_as_of_coverage_stratified_summary"].iloc[0]
        self.assertEqual(summary["coverage_stratum"], "stratified")
        coverage_rows = backtest[backtest["evaluation"] == "source_event_as_of_coverage_stratum"]
        self.assertIn("abandonment_heuristic", set(coverage_rows["model"]))
        self.assertIn("random_forest_classifier_oof", set(coverage_rows["model"]))
        self.assertEqual(coverage_rows["coverage_stratum"].nunique(), 2)
        self.assertTrue((coverage_rows["ready_for_product_action"] == "false").all())
        self.assertTrue(coverage_rows["note"].str.contains("within source coverage stratum").any())

    def test_tpm_decision_target_backtest_keeps_open_prs_censored_not_negative(self) -> None:
        base = datetime(2026, 1, 1, tzinfo=timezone.utc)
        snapshots = []
        for idx in range(10):
            created_at = base + timedelta(days=idx)
            terminal_at = created_at + timedelta(days=3)
            snapshots.append(
                event_snapshot_pr_row(
                    idx + 1,
                    created_at,
                    terminal_at,
                    3.0,
                    observed_offset_days=0.5,
                    is_merged=True,
                )
            )
        for idx in range(3):
            created_at = base + timedelta(days=20 + idx)
            row = event_snapshot_pr_row(
                100 + idx,
                created_at,
                created_at + timedelta(days=5),
                5.0,
                observed_offset_days=0.5,
                is_merged=False,
            )
            row["closed_at"] = ""
            row["merged_at"] = ""
            snapshots.append(row)

        examples = analytics.tpm_decision_target_examples(pd.DataFrame(snapshots), analytics.forecast_feature_columns())

        self.assertEqual(examples["subject_key"].nunique(), 10)
        self.assertEqual(int(examples["target_abandoned"].sum()), 0)

    def test_heuristic_cycle_prediction_accepts_snapshot_rows_without_current_fields(self) -> None:
        snapshots = pd.DataFrame(
            [
                {"comments": 1, "review_comments": 0, "linked_ticket_count": 1},
                {"comments": 2, "review_comments": 1, "linked_ticket_count": 2},
            ]
        )

        prediction = analytics.heuristic_cycle_prediction(snapshots, median_cycle=3.0, p75_cycle=6.0)

        self.assertEqual(len(prediction), 2)
        self.assertTrue((prediction >= 3.0).all())

    def test_fold_level_improvement_does_not_mark_eta_ready_when_aggregate_gate_fails(self) -> None:
        rows = []
        for idx in range(30):
            cycle_days = 2.0 if idx < 20 else 20.0
            row = forecast_feature_pr_row(idx + 1, "merged", cycle_days)
            row["author_prior_median_cycle_days"] = 2.0
            rows.append(row)

        backtest = analytics.build_forecast_backtest(
            pd.DataFrame(rows),
            analytics.forecast_feature_columns(),
            temporal_feature_snapshot_ready=True,
        )
        metrics = analytics.forecast_backtest_metrics(backtest)

        self.assertFalse(analytics.forecast_eta_model_backtest_ready(metrics))
        self.assertTrue((backtest["ready_for_eta"] == "false").all())

    def test_lifecycle_as_of_baseline_is_reported_but_never_eta_ready(self) -> None:
        base = datetime(2026, 1, 1, tzinfo=timezone.utc)
        rows = []
        for idx in range(40):
            cycle_days = float(2 + (idx % 8))
            created_at = base + timedelta(days=idx)
            merged_at = created_at + timedelta(days=cycle_days)
            row = {
                "repository": "repo/example",
                "pr_number": idx + 1,
                "state": "merged",
                "created_at": created_at.isoformat(),
                "merged_at": merged_at.isoformat(),
                "closed_at": merged_at.isoformat(),
                "age_days": cycle_days,
                "stale_days": 0.0,
                "cycle_time_days": cycle_days,
                "total_lines_changed": 30,
                "days_since_review_activity": 0,
            }
            for column in analytics.FORECAST_FEATURE_COLUMNS:
                row[column] = 0
            row.update(
                {
                    "additions": 20,
                    "deletions": 10,
                    "changed_files": 2,
                    "commits": 1,
                    "comments": 1,
                    "review_comments": 0,
                    "linked_ticket_count": 1,
                    "requested_reviewer_count": 0,
                    "draft": False,
                }
            )
            rows.append(row)

        summary, _, backtest, _ = analytics.build_forecasts(pd.DataFrame(rows))
        summary_metrics = {row.metric: row.value for row in summary.itertuples(index=False)}
        lifecycle_rows = backtest[backtest["evaluation"] == "lifecycle_as_of_baseline"]

        self.assertEqual(summary_metrics["lifecycle_as_of_backtest_state"], "baseline_available")
        self.assertEqual(summary_metrics["lifecycle_as_of_terminal_subject_count"], "40")
        self.assertGreater(int(summary_metrics["lifecycle_as_of_training_example_count"]), 40)
        self.assertIn("age_bucket_median_remaining", set(lifecycle_rows["model"]))
        self.assertTrue((lifecycle_rows["ready_for_eta"] == "false").all())
        self.assertEqual(summary_metrics["eta_forecast_ready"], "false")
        self.assertEqual(summary_metrics["eta_temporal_snapshot_state"], "as_of_feature_snapshot_series_missing")

    def test_survival_time_to_merge_baseline_uses_censored_rows_and_stays_eta_gated(self) -> None:
        base = datetime(2026, 1, 1, tzinfo=timezone.utc)
        rows = []
        for idx in range(35):
            cycle_days = float(2 + (idx % 12))
            created_at = base + timedelta(days=idx)
            terminal_at = created_at + timedelta(days=cycle_days)
            rows.append(survival_feature_pr_row(idx + 1, "merged", created_at, cycle_days, terminal_at))
        for idx in range(10):
            cycle_days = float(12 + idx)
            created_at = base + timedelta(days=60 + idx)
            terminal_at = created_at + timedelta(days=cycle_days)
            rows.append(survival_feature_pr_row(100 + idx, "closed", created_at, cycle_days, terminal_at))
        for idx in range(6):
            created_at = base + timedelta(days=90 + idx)
            rows.append(survival_feature_pr_row(200 + idx, "open", created_at, None, None, age_days=20.0 + idx))

        summary, _, backtest, _ = analytics.build_forecasts(pd.DataFrame(rows))
        summary_metrics = {row.metric: row.value for row in summary.itertuples(index=False)}
        survival_rows = backtest[backtest["evaluation"] == "survival_time_to_merge"]

        self.assertEqual(summary_metrics["survival_time_to_merge_state"], "baseline_available")
        self.assertEqual(summary_metrics["survival_time_to_merge_subject_count"], "51")
        self.assertEqual(summary_metrics["survival_time_to_merge_event_subject_count"], "35")
        self.assertEqual(summary_metrics["survival_time_to_merge_censored_subject_count"], "16")
        self.assertEqual(summary_metrics["survival_time_to_merge_open_censored_subject_count"], "6")
        self.assertGreater(int(summary_metrics["survival_time_to_merge_backtest_example_count"]), 0)
        self.assertIn("km_restricted_mean_remaining", set(survival_rows["model"]))
        self.assertTrue((survival_rows["ready_for_eta"] == "false").all())
        self.assertEqual(summary_metrics["eta_forecast_ready"], "false")

    def test_build_forecasts_concat_paths_do_not_emit_future_warnings(self) -> None:
        base = datetime(2026, 1, 1, tzinfo=timezone.utc)
        rows = []
        for idx in range(25):
            cycle_days = float(2 + (idx % 10))
            created_at = base + timedelta(days=idx)
            terminal_at = created_at + timedelta(days=cycle_days)
            rows.append(survival_feature_pr_row(idx + 1, "merged", created_at, cycle_days, terminal_at))
        for idx in range(5):
            created_at = base + timedelta(days=40 + idx)
            rows.append(survival_feature_pr_row(100 + idx, "open", created_at, None, None, age_days=12.0 + idx))

        with warnings.catch_warnings():
            warnings.simplefilter("error", FutureWarning)
            summary, _, backtest, _ = analytics.build_forecasts(pd.DataFrame(rows))

        summary_metrics = {row.metric: row.value for row in summary.itertuples(index=False)}
        self.assertEqual(summary_metrics["lifecycle_as_of_backtest_state"], "baseline_available")
        self.assertEqual(summary_metrics["survival_time_to_merge_state"], "baseline_available")
        self.assertIn("lifecycle_as_of_baseline", set(backtest["evaluation"]))
        self.assertIn("survival_time_to_merge", set(backtest["evaluation"]))

    def test_concat_preserving_columns_keeps_all_null_typed_column_dtype(self) -> None:
        left = pd.DataFrame({"name": ["one"], "typed_nulls": pd.Series([None], dtype="string")})
        right = pd.DataFrame({"name": ["two"], "typed_nulls": pd.Series([None], dtype="string")})

        out = analytics.concat_dataframes_preserving_columns([left, right], ["name", "typed_nulls"])

        self.assertEqual(out["name"].tolist(), ["one", "two"])
        self.assertEqual(str(out["typed_nulls"].dtype), "string")

    def test_forecast_readiness_diagnostics_require_kfold_chrono_and_snapshots(self) -> None:
        rows = analytics.forecast_readiness_diagnostic_rows(
            {
                "median_cycle_baseline_kfold_mae": 10.0,
                "heuristic_percentile_kfold_mae": 9.0,
                "gradient_boosting_absolute_error_kfold_mae": 9.4,
                "random_forest_regressor_kfold_mae": 8.0,
                "median_cycle_baseline_chronological_holdout_mae": 10.0,
                "heuristic_percentile_chronological_holdout_mae": 9.0,
                "gradient_boosting_absolute_error_chronological_holdout_mae": 9.2,
                "random_forest_regressor_chronological_holdout_mae": 9.5,
            },
            50,
        )
        by_metric = {row["metric"]: row["value"] for row in rows}

        self.assertEqual(by_metric["eta_readiness_state"], "blocked")
        self.assertEqual(by_metric["eta_primary_blocker"], "chronological_holdout_model_does_not_beat_baseline")
        self.assertEqual(by_metric["eta_blocker_count"], "2")
        self.assertEqual(by_metric["eta_best_kfold_model"], "random_forest_regressor")
        self.assertEqual(by_metric["eta_best_chronological_model"], "heuristic_percentile")
        self.assertEqual(by_metric["eta_same_model_backtest_gate"], "gated")
        self.assertEqual(by_metric["eta_best_candidate_model"], "random_forest_regressor")
        self.assertEqual(by_metric["eta_kfold_best_candidate_improvement_pct"], "20.00")
        self.assertEqual(by_metric["eta_chronological_best_candidate_improvement_pct"], "5.00")
        self.assertEqual(by_metric["eta_kfold_random_forest_improvement_pct"], "20.00")
        self.assertEqual(by_metric["eta_chronological_random_forest_improvement_pct"], "5.00")
        self.assertEqual(by_metric["eta_temporal_snapshot_state"], "as_of_feature_snapshot_series_missing")

    def test_gradient_boosting_candidate_is_reported_without_weakening_eta_gate(self) -> None:
        rows = analytics.forecast_readiness_diagnostic_rows(
            {
                "median_cycle_baseline_kfold_mae": 10.0,
                "heuristic_percentile_kfold_mae": 12.0,
                "gradient_boosting_absolute_error_kfold_mae": 9.2,
                "random_forest_regressor_kfold_mae": 11.0,
                "median_cycle_baseline_chronological_holdout_mae": 10.0,
                "heuristic_percentile_chronological_holdout_mae": 12.0,
                "gradient_boosting_absolute_error_chronological_holdout_mae": 9.4,
                "random_forest_regressor_chronological_holdout_mae": 11.0,
            },
            50,
            temporal_feature_snapshot_ready=True,
        )
        by_metric = {row["metric"]: row["value"] for row in rows}

        self.assertEqual(by_metric["eta_best_candidate_model"], "gradient_boosting_absolute_error")
        self.assertEqual(by_metric["eta_best_kfold_model"], "gradient_boosting_absolute_error")
        self.assertEqual(by_metric["eta_best_chronological_model"], "gradient_boosting_absolute_error")
        self.assertEqual(by_metric["eta_same_model_backtest_gate"], "gated")
        self.assertEqual(by_metric["eta_kfold_best_candidate_improvement_pct"], "8.00")
        self.assertEqual(by_metric["eta_chronological_best_candidate_improvement_pct"], "6.00")
        self.assertEqual(by_metric["eta_model_backtest_ready"], "false")
        self.assertEqual(by_metric["eta_readiness_state"], "blocked")
        self.assertEqual(by_metric["eta_primary_blocker"], "kfold_model_does_not_beat_baseline")

    def test_forecast_readiness_next_evidence_moves_to_model_quality_after_snapshots_exist(self) -> None:
        rows = analytics.forecast_readiness_diagnostic_rows(
            {
                "median_cycle_baseline_kfold_mae": 10.0,
                "heuristic_percentile_kfold_mae": 9.0,
                "random_forest_regressor_kfold_mae": 11.0,
                "median_cycle_baseline_chronological_holdout_mae": 10.0,
                "heuristic_percentile_chronological_holdout_mae": 9.0,
                "random_forest_regressor_chronological_holdout_mae": 8.0,
            },
            50,
            temporal_feature_snapshot_ready=True,
        )
        by_metric = {row["metric"]: row["value"] for row in rows}

        self.assertEqual(by_metric["eta_temporal_snapshot_state"], "as_of_feature_snapshot_series_ready")
        self.assertEqual(by_metric["eta_primary_blocker"], "kfold_model_does_not_beat_baseline")
        self.assertEqual(by_metric["eta_best_kfold_model"], "heuristic_percentile")
        self.assertEqual(by_metric["eta_best_chronological_model"], "random_forest_regressor")
        self.assertEqual(by_metric["eta_same_model_backtest_gate"], "gated")
        self.assertEqual(by_metric["eta_next_evidence_needed"], "improve_model_features_and_validate_against_as_of_snapshots")

    def test_forecast_reliability_separates_eta_range_and_risk_triage(self) -> None:
        summary = pd.DataFrame(
            [
                {"metric": "eta_forecast_ready", "value": "false", "note": "fixture"},
                {"metric": "eta_best_candidate_model", "value": "gradient_boosting_absolute_error", "note": "fixture"},
                {"metric": "eta_kfold_best_candidate_improvement_pct", "value": "7.04", "note": "fixture"},
                {"metric": "eta_primary_blocker", "value": "kfold_model_does_not_beat_baseline", "note": "fixture"},
                {"metric": "eta_next_evidence_needed", "value": "improve_model_features_and_validate_against_as_of_snapshots", "note": "fixture"},
                {"metric": "lifecycle_as_of_best_model", "value": "age_bucket_median_remaining", "note": "fixture"},
                {"metric": "lifecycle_as_of_best_mae_days", "value": "21.52", "note": "fixture"},
                {"metric": "survival_time_to_merge_best_model", "value": "km_median_remaining", "note": "fixture"},
                {"metric": "survival_time_to_merge_best_mae_days", "value": "21.14", "note": "fixture"},
                {"metric": "risk_triage_lift_at_10pct", "value": "0.3446", "note": "fixture"},
                {"metric": "risk_triage_coverage_stratified_state", "value": "not_testable_single_stratum", "note": "fixture"},
            ]
        )

        reliability = analytics.build_forecast_reliability(summary)
        by_product = {row.forecast_product: row for row in reliability.itertuples(index=False)}

        self.assertEqual(by_product["point_eta"].readiness_state, "gated")
        self.assertEqual(by_product["point_eta"].product_safe, "false")
        self.assertIn("risk triage", by_product["point_eta"].guardrail)
        self.assertEqual(by_product["range_eta"].readiness_state, "diagnostic_only")
        self.assertEqual(by_product["range_eta"].product_safe, "false")
        self.assertEqual(by_product["range_eta"].best_model, "survival_time_to_merge:km_median_remaining")
        self.assertEqual(by_product["range_eta"].metric_value, "21.14")
        self.assertEqual(by_product["risk_triage"].readiness_state, "ready_with_coverage_guardrail")
        self.assertEqual(by_product["risk_triage"].product_safe, "true")
        self.assertEqual(by_product["risk_triage"].safe_use, "attention_ordering")

    def test_gradient_boosting_absolute_error_can_clear_eta_model_gate(self) -> None:
        gradient_boosting_wins = {
            "gradient_boosting_absolute_error_kfold_mae": 7.0,
            "median_cycle_baseline_kfold_mae": 10.0,
            "heuristic_percentile_kfold_mae": 10.0,
            "gradient_boosting_absolute_error_chronological_holdout_mae": 7.5,
            "median_cycle_baseline_chronological_holdout_mae": 10.0,
            "heuristic_percentile_chronological_holdout_mae": 10.0,
        }

        self.assertEqual(analytics.forecast_eta_ready_model(gradient_boosting_wins), "gradient_boosting_absolute_error")
        self.assertTrue(analytics.forecast_eta_model_backtest_ready(gradient_boosting_wins))
        self.assertTrue(analytics.forecast_eta_ready(gradient_boosting_wins, temporal_feature_snapshot_ready=True))

    def test_hist_gradient_boosting_absolute_error_can_clear_eta_model_gate(self) -> None:
        hist_gradient_boosting_wins = {
            "hist_gradient_boosting_absolute_error_kfold_mae": 7.0,
            "median_cycle_baseline_kfold_mae": 10.0,
            "heuristic_percentile_kfold_mae": 10.0,
            "hist_gradient_boosting_absolute_error_chronological_holdout_mae": 7.5,
            "median_cycle_baseline_chronological_holdout_mae": 10.0,
            "heuristic_percentile_chronological_holdout_mae": 10.0,
        }

        self.assertEqual(analytics.forecast_eta_ready_model(hist_gradient_boosting_wins), "hist_gradient_boosting_absolute_error")
        self.assertTrue(analytics.forecast_eta_model_backtest_ready(hist_gradient_boosting_wins))
        self.assertTrue(analytics.forecast_eta_ready(hist_gradient_boosting_wins, temporal_feature_snapshot_ready=True))

    def test_eta_ready_model_selects_best_passing_candidate(self) -> None:
        two_models_pass = {
            "gradient_boosting_absolute_error_kfold_mae": 7.0,
            "gradient_boosting_absolute_error_chronological_holdout_mae": 8.0,
            "hist_gradient_boosting_absolute_error_kfold_mae": 7.2,
            "hist_gradient_boosting_absolute_error_chronological_holdout_mae": 7.1,
            "median_cycle_baseline_kfold_mae": 10.0,
            "median_cycle_baseline_chronological_holdout_mae": 10.0,
            "heuristic_percentile_kfold_mae": 10.0,
            "heuristic_percentile_chronological_holdout_mae": 10.0,
        }

        self.assertEqual(analytics.forecast_eta_ready_model(two_models_pass), "hist_gradient_boosting_absolute_error")

    def test_build_forecast_backtest_includes_gradient_boosting_candidate(self) -> None:
        rows = []
        for idx in range(30):
            row = forecast_feature_pr_row(idx + 1, "merged", float(2 + (idx % 8)))
            row["additions"] = idx * 10
            row["total_lines_changed"] = idx * 10
            rows.append(row)

        backtest = analytics.build_forecast_backtest(
            pd.DataFrame(rows),
            analytics.forecast_feature_columns(),
            temporal_feature_snapshot_ready=True,
        )

        self.assertIn("gradient_boosting_absolute_error", set(backtest["model"]))
        self.assertIn("hist_gradient_boosting_absolute_error", set(backtest["model"]))

    def test_static_risk_backtest_reports_precision_and_lift(self) -> None:
        rows = []
        for idx in range(30):
            slow = idx < 8
            rows.append(
                {
                    "cycle_time_days": 20.0 if slow else 1.0,
                    "total_lines_changed": 2500 if slow else 20,
                    "comments": 18 if slow else 0,
                    "review_comments": 12 if slow else 0,
                    "linked_ticket_count": 3 if slow else 1,
                    "requested_reviewer_count": 2 if slow else 0,
                    "days_since_review_activity": 12 if slow else 0,
                    "draft": False,
                }
            )

        backtest = analytics.build_forecast_risk_backtest(pd.DataFrame(rows))
        by_metric = {row.metric: row for row in backtest.itertuples(index=False)}

        self.assertEqual(by_metric["sample_count"].value, "30")
        self.assertEqual(by_metric["slow_cycle_base_rate"].value, "0.2667")
        self.assertGreater(float(by_metric["precision_at_10pct"].value), 0.9)
        self.assertGreater(float(by_metric["lift_vs_baseline_at_10pct"].value), 0.6)
        self.assertIn("not ETA", by_metric["precision_at_10pct"].guardrail)
        self.assertEqual(by_metric["coverage_stratified_backtest_state"].value, "not_testable_single_stratum")
        self.assertIn("cannot be tested", by_metric["coverage_stratified_backtest_state"].interpretation)
        self.assertEqual(by_metric["coverage_stratum_count"].value, "1")

    def test_static_risk_backtest_reports_coverage_stratified_precision(self) -> None:
        rows = []
        for idx in range(30):
            slow = idx < 8
            rows.append(
                {
                    "pr_number": idx + 1,
                    "cycle_time_days": 20.0 if slow else 1.0,
                    "total_lines_changed": 2500 if slow else 20,
                    "comments": 18 if slow else 0,
                    "review_comments": 12 if slow else 0,
                    "linked_ticket_count": 3 if slow else 1,
                    "requested_reviewer_count": 2 if slow else 0,
                    "days_since_review_activity": 12 if slow else 0,
                    "draft": False,
                    "source_current_coverage_state": "observed",
                    "source_current_detail_state": "observed",
                }
            )
        for idx in range(30):
            slow = idx < 8
            rows.append(
                {
                    "pr_number": 100 + idx + 1,
                    "cycle_time_days": 22.0 if slow else 2.0,
                    "total_lines_changed": 2600 if slow else 25,
                    "comments": 18 if slow else 0,
                    "review_comments": 12 if slow else 0,
                    "linked_ticket_count": 3 if slow else 1,
                    "requested_reviewer_count": 2 if slow else 0,
                    "days_since_review_activity": 12 if slow else 0,
                    "draft": False,
                    "source_current_coverage_state": "detail_failed",
                    "source_current_detail_state": "failed",
                }
            )

        backtest = analytics.build_forecast_risk_backtest(pd.DataFrame(rows))
        by_metric = {row.metric: row for row in backtest.itertuples(index=False)}

        self.assertEqual(by_metric["coverage_stratified_backtest_state"].value, "stratified")
        self.assertEqual(by_metric["coverage_stratum_count"].value, "2")
        observed_precision = next(
            row
            for row in backtest.itertuples(index=False)
            if row.metric.startswith("coverage_stratum_coverage_observed_detail_observed_")
            and row.metric.endswith("_precision_at_10pct")
        )
        failed_precision = next(
            row
            for row in backtest.itertuples(index=False)
            if row.metric.startswith("coverage_stratum_coverage_detail_failed_detail_failed_")
            and row.metric.endswith("_precision_at_10pct")
        )
        self.assertGreater(
            float(observed_precision.value),
            0.9,
        )
        self.assertGreater(
            float(failed_precision.value),
            0.9,
        )

    def test_static_risk_backtest_flags_coverage_confounded_global_lift(self) -> None:
        rows = []
        for idx in range(40):
            slow = idx % 4 != 3
            rows.append(
                {
                    "pr_number": idx + 1,
                    "cycle_time_days": 20.0 if slow else 1.0,
                    "total_lines_changed": 2500,
                    "comments": 18,
                    "review_comments": 12,
                    "linked_ticket_count": 3,
                    "requested_reviewer_count": 2,
                    "days_since_review_activity": 12,
                    "draft": False,
                    "source_current_coverage_state": "observed",
                    "source_current_detail_state": "observed",
                    "source_current_coverage_mode": "authenticated",
                    "lifecycle_fields_source": "authenticated_api_current_observation",
                    "churn_fields_source": "authenticated_api_current_observation",
                    "mergeability_fields_source": "authenticated_api_current_observation",
                }
            )
        for idx in range(40):
            slow = idx % 4 == 0
            rows.append(
                {
                    "pr_number": 100 + idx + 1,
                    "cycle_time_days": 20.0 if slow else 1.0,
                    "total_lines_changed": 20,
                    "comments": 0,
                    "review_comments": 0,
                    "linked_ticket_count": 1,
                    "requested_reviewer_count": 0,
                    "days_since_review_activity": 0,
                    "draft": False,
                    "source_current_coverage_state": "detail_failed",
                    "source_current_detail_state": "failed",
                    "source_current_coverage_mode": "anonymous",
                    "lifecycle_fields_source": "partial_remote_link",
                    "churn_fields_source": "missing",
                    "mergeability_fields_source": "missing",
                }
            )

        backtest = analytics.build_forecast_risk_backtest(pd.DataFrame(rows))
        by_metric = {row.metric: row for row in backtest.itertuples(index=False)}

        self.assertGreater(float(by_metric["lift_vs_baseline_at_10pct"].value), 0.2)
        self.assertEqual(by_metric["coverage_stratified_backtest_state"].value, "confounded")
        self.assertIn("coverage-confounded", by_metric["coverage_stratified_backtest_state"].interpretation)
        self.assertLessEqual(float(by_metric["coverage_stratified_max_lift_at_10pct"].value), 0.01)

    def test_top_fraction_tie_breaker_does_not_use_cycle_time_truth(self) -> None:
        rows = pd.DataFrame(
            [
                {"pr_number": 1, "static_risk_triage_score": 50.0, "cycle_time_days": 1.0},
                {"pr_number": 2, "static_risk_triage_score": 50.0, "cycle_time_days": 20.0},
                {"pr_number": 3, "static_risk_triage_score": 50.0, "cycle_time_days": 30.0},
                {"pr_number": 4, "static_risk_triage_score": 10.0, "cycle_time_days": 40.0},
            ]
        )

        top, label = analytics.top_fraction(rows, 0.25, "static_risk_triage_score")

        self.assertEqual(label, "25pct")
        self.assertEqual(top.iloc[0]["pr_number"], 1)

    def test_eta_readiness_requires_chronological_holdout_to_beat_baselines(self) -> None:
        kfold_only = {
            "random_forest_regressor_kfold_mae": 8.0,
            "median_cycle_baseline_kfold_mae": 10.0,
            "heuristic_percentile_kfold_mae": 10.0,
        }
        chronological_loses = {
            **kfold_only,
            "random_forest_regressor_chronological_holdout_mae": 9.5,
            "median_cycle_baseline_chronological_holdout_mae": 10.0,
            "heuristic_percentile_chronological_holdout_mae": 10.0,
        }
        chronological_wins = {
            **kfold_only,
            "random_forest_regressor_chronological_holdout_mae": 8.0,
            "median_cycle_baseline_chronological_holdout_mae": 10.0,
            "heuristic_percentile_chronological_holdout_mae": 10.0,
        }

        self.assertFalse(analytics.forecast_eta_model_backtest_ready(kfold_only))
        self.assertFalse(analytics.forecast_eta_model_backtest_ready(chronological_loses))
        self.assertTrue(analytics.forecast_eta_model_backtest_ready(chronological_wins))
        self.assertFalse(analytics.forecast_eta_ready(chronological_wins))
        self.assertTrue(analytics.forecast_eta_ready(chronological_wins, temporal_feature_snapshot_ready=True))

    def test_author_history_median_cycle_can_clear_eta_model_gate(self) -> None:
        graph_prior_wins = {
            "author_history_median_cycle_kfold_mae": 7.0,
            "median_cycle_baseline_kfold_mae": 10.0,
            "heuristic_percentile_kfold_mae": 10.0,
            "author_history_median_cycle_chronological_holdout_mae": 7.5,
            "median_cycle_baseline_chronological_holdout_mae": 10.0,
            "heuristic_percentile_chronological_holdout_mae": 10.0,
        }

        self.assertEqual(analytics.forecast_eta_ready_model(graph_prior_wins), "author_history_median_cycle")
        self.assertTrue(analytics.forecast_eta_model_backtest_ready(graph_prior_wins))
        self.assertTrue(analytics.forecast_eta_ready(graph_prior_wins, temporal_feature_snapshot_ready=True))

    def test_author_history_features_are_as_of_safe(self) -> None:
        rows = pd.DataFrame(
            [
                {
                    "repository": "repo/example",
                    "pr_number": 1,
                    "author_login": "owner-a",
                    "created_at": "2026-06-01T00:00:00+00:00",
                    "merged_at": "2026-06-03T00:00:00+00:00",
                    "closed_at": "2026-06-03T00:00:00+00:00",
                    "cycle_time_days": 2.0,
                },
                {
                    "repository": "repo/example",
                    "pr_number": 2,
                    "author_login": "owner-a",
                    "created_at": "2026-06-04T00:00:00+00:00",
                    "merged_at": "",
                    "closed_at": "",
                    "cycle_time_days": None,
                },
                {
                    "repository": "repo/example",
                    "pr_number": 3,
                    "author_login": "owner-a",
                    "created_at": "2026-06-05T00:00:00+00:00",
                    "merged_at": "2026-06-08T00:00:00+00:00",
                    "closed_at": "2026-06-08T00:00:00+00:00",
                    "cycle_time_days": 3.0,
                },
            ]
        )

        out = analytics.add_author_history_features(rows)
        second = out[out["pr_number"] == 2].iloc[0]
        third = out[out["pr_number"] == 3].iloc[0]

        self.assertEqual(second["author_prior_pr_count"], 1)
        self.assertEqual(second["author_prior_merged_pr_count"], 1)
        self.assertEqual(second["author_prior_median_cycle_days"], 2.0)
        self.assertEqual(third["author_prior_pr_count"], 2)
        self.assertEqual(third["author_prior_merged_pr_count"], 1)
        self.assertEqual(third["author_open_pr_count"], 1)


def metric(rows: pd.DataFrame, key: str) -> str:
    match = rows[rows["metric"] == key]
    if match.empty:
        return ""
    return str(match.iloc[0]["value"])


def pr_features(rows: list[dict[str, object]]) -> pd.DataFrame:
    return pd.DataFrame(rows)


def forecast_feature_pr_row(number: int, state: str, cycle_time_days: float | None) -> dict[str, object]:
    row: dict[str, object] = {
        "repository": "repo/example",
        "pr_number": number,
        "state": state,
        "created_at": f"2026-06-{number:02d}T00:00:00+00:00",
        "age_days": 4.0,
        "stale_days": 1.0,
        "cycle_time_days": cycle_time_days,
        "total_lines_changed": 30,
        "days_since_review_activity": 0,
    }
    for column in analytics.FORECAST_FEATURE_COLUMNS:
        row[column] = 0
    row.update(
        {
            "additions": 20,
            "deletions": 10,
            "changed_files": 2,
            "commits": 1,
            "comments": 1,
            "review_comments": 0,
            "linked_ticket_count": 1,
            "requested_reviewer_count": 0,
            "draft": False,
        }
    )
    return row


def decision_target_row(
    evaluation: str,
    model: str,
    fold: int,
    lift: float | None,
    *,
    precision: float | None = None,
    roc_auc: float | None = None,
    coverage_stratum: str = "",
) -> dict[str, object]:
    return {
        "target_kind": "abandonment_risk",
        "evaluation": evaluation,
        "model": model,
        "fold": fold,
        "train_count": 100,
        "test_count": 40,
        "positive_count": 10,
        "baseline_positive_rate": 0.25,
        "precision_at_10pct": precision,
        "lift_at_10pct": lift,
        "roc_auc": roc_auc,
        "average_precision": None,
        "coverage_stratum": coverage_stratum,
        "ready_for_product_action": "false",
        "note": "test row",
    }


def event_snapshot_pr_row(
    number: int,
    created_at: datetime,
    terminal_at: datetime,
    cycle_time_days: float,
    *,
    observed_offset_days: float,
    is_merged: bool = True,
    source_current_coverage_state: str = "observed",
    source_current_detail_state: str = "observed",
) -> dict[str, object]:
    observed_at = created_at + timedelta(days=observed_offset_days)
    row = forecast_feature_pr_row(number, "open", cycle_time_days)
    row.update(
        {
            "repository": "repo/example",
            "pr_number": number,
            "subject_key": f"repo/example#{number}",
            "state": "open",
            "created_at": created_at.isoformat(),
            "updated_at": observed_at.isoformat(),
            "merged_at": terminal_at.isoformat() if is_merged else "",
            "closed_at": terminal_at.isoformat(),
            "event_replay_observed_at": observed_at.isoformat(),
            "age_days": observed_offset_days,
            "cycle_time_days": cycle_time_days,
            "is_merged": 1 if is_merged else 0,
            "total_lines_changed": 0,
            "stale_days": 0,
            "comments": 1,
            "review_comments": 0,
            "commits": 1,
            "linked_ticket_count": 1,
            "source_current_coverage_state": source_current_coverage_state,
            "source_current_detail_state": source_current_detail_state,
            "source_current_coverage_mode": "current",
            "lifecycle_fields_source": "source_event",
            "churn_fields_source": "source_event",
            "mergeability_fields_source": "source_event",
        }
    )
    return row


def survival_feature_pr_row(
    number: int,
    state: str,
    created_at: datetime,
    cycle_time_days: float | None,
    terminal_at: datetime | None,
    *,
    age_days: float | None = None,
) -> dict[str, object]:
    row = forecast_feature_pr_row(number if number < 28 else ((number % 27) + 1), state, cycle_time_days)
    row.update(
        {
            "repository": "repo/example",
            "pr_number": number,
            "state": state,
            "created_at": created_at.isoformat(),
            "merged_at": terminal_at.isoformat() if state == "merged" and terminal_at is not None else "",
            "closed_at": terminal_at.isoformat() if state in {"merged", "closed"} and terminal_at is not None else "",
            "age_days": age_days if age_days is not None else (cycle_time_days or 0.0),
            "cycle_time_days": cycle_time_days,
        }
    )
    return row


def pr_row(
    number: int,
    state: str,
    coverage_state: str,
    *,
    merged_at: str = "",
    cycle_time_days: float | None = None,
) -> dict[str, object]:
    subject_key = f"repo/example#{number}"
    return {
        "repository": "repo/example",
        "pr_number": number,
        "state": state,
        "title": f"Example PR {number}",
        "pr_url": f"https://github.com/repo/example/pull/{number}",
        "created_at": "2026-06-17T00:00:00+00:00",
        "updated_at": "2026-06-21T00:00:00+00:00",
        "closed_at": merged_at,
        "merged_at": merged_at,
        "age_days": 4.0,
        "stale_days": 0.0,
        "cycle_time_days": cycle_time_days,
        "risk_score": 50.0,
        "risk_band": "medium",
        "forecast_method": "fixture",
        "source_current_coverage_state": coverage_state,
        "source_current_detail_state": "observed",
        "source_current_issue_codes": "",
        "source_current_issue_kinds": "",
        "lifecycle_fields_source": "fixture",
        "churn_fields_source": "fixture",
        "mergeability_fields_source": "fixture",
        "author_login": "owner",
        "subject_key": subject_key,
    }


def empty_ticket_features() -> pd.DataFrame:
    return pd.DataFrame(
        columns=[
            "ticket_key",
            "status",
            "priority",
            "title",
            "updated_at",
            "linked_pr_count",
            "fresh_pr_link_count",
            "partial_pr_link_count",
            "comment_count",
            "participant_count",
            "blocker_keyword_count",
        ]
    )


def ticket_features(rows: list[dict[str, object]]) -> pd.DataFrame:
    return pd.DataFrame(rows)


def ticket_row(ticket_key: str, status: str) -> dict[str, object]:
    return {
        "ticket_key": ticket_key,
        "status": status,
        "priority": "Major",
        "title": f"{ticket_key} fixture",
        "updated_at": "2026-06-21T00:00:00+00:00",
        "linked_pr_count": 1,
        "fresh_pr_link_count": 1,
        "partial_pr_link_count": 0,
        "comment_count": 0,
        "participant_count": 0,
        "blocker_keyword_count": 0,
    }


if __name__ == "__main__":
    unittest.main()
