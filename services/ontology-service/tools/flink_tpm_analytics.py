#!/usr/bin/env python3
"""Persist AI-TPM analytics over a Flink Jira/GitHub fixture.

The script prefers typed Ent product rows for identity, lifecycle, review, and
PR metric features. Raw replay payloads remain available for fallback and for
source-span extraction that has not yet been fully promoted to Evidence rows.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import re
import sqlite3
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from pathlib import Path
from typing import Any

import networkx as nx
import pandas as pd
from sklearn.ensemble import (
    GradientBoostingRegressor,
    HistGradientBoostingRegressor,
    RandomForestClassifier,
    RandomForestRegressor,
)
from sklearn.metrics import average_precision_score, mean_absolute_error, roc_auc_score
from sklearn.model_selection import GroupKFold, KFold


ISSUE_RE = re.compile(r"(?i)(?<![A-Za-z0-9])FLINK-\d+(?![A-Za-z0-9]|\.\d)")
BLOCKER_RE = re.compile(
    r"(?i)\b("
    r"block(?:ed|er|ing)?|stuck|waiting|unable|cannot|can't|fail(?:ed|ing|s)?|"
    r"flaky|regression|conflict|missing|required|depends? on|dependency|"
    r"not ready|incomplete|timeout|error"
    r")\b"
)
SECRET_TOKEN_RE = re.compile(r"\b(?:ghp_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|xoxb-[A-Za-z0-9-]+)\b")
NEGATED_BLOCKER_RE = re.compile(
    r"(?is)\b("
    r"none\b.{0,80}\bblock(?:ed|er|ing)?|"
    r"not\b.{0,40}\bblock(?:ed|er|ing)?|"
    r"no\b.{0,40}\bblock(?:ed|er|ing)?|"
    r"not\b.{0,40}\b(regression|conflict|error|fail(?:ed|ing|s)?)|"
    r"no\b.{0,40}\b(dependency|dependencies|conflict|error)"
    r")\b"
)
BOILERPLATE_BLOCKER_RE = re.compile(
    r"(?is)"
    r"dependencies\s*\(does it add or upgrade a dependency\)\s*:[^\n\r]*|"
    r"public api.*?:\s*no|"
    r"customresourcedescriptors.*?:\s*no"
)
SOURCE_PR_BUNDLE_KINDS = [
	"github_pull_request",
	"github_pull_request_files",
	"github_issue_comments",
	"github_pull_request_review_comments",
	"github_pull_request_reviews",
	"github_pull_request_commits",
]
JIRA_ISSUE_OBJECT_TYPES = {"jira_issue", "jira_correlation_issue"}
ANALYTICS_VERSION = "2026-06-22.3"
DEVELOPER_CORRELATION_GUARDRAIL = (
    "Same-window developer correlation is a workload/attention lead only: it requires a direct "
    "Person row with both GitHub and Jira identity before comparing authored PRs to extra Jira tickets, "
    "and it never proves causality, ownership, performance, or blocker absence."
)
MEASUREMENT_LABEL_QUALITIES = {"gold"}
POSITIVE_ACTIONABILITY = {"actionable", "needs_owner"}
RESOLVED_REVIEW_STATES = {"accepted", "dismissed", "resolved"}
MIN_MEASUREMENT_LABEL_TOTAL = 10
MIN_MEASUREMENT_LABEL_PER_KIND = 10
MIN_RISK_BACKTEST_COVERAGE_STRATUM_SAMPLE = 20
MIN_AS_OF_FEATURE_SNAPSHOT_OBSERVED_TIMES = 2
MIN_AS_OF_FEATURE_SNAPSHOT_TRAINING_EXAMPLES = 10
MIN_LIFECYCLE_AS_OF_SUBJECTS = 20
LIFECYCLE_AS_OF_CHECKPOINT_DAYS = [0.25, 0.5, 1.0, 2.0, 4.0, 7.0, 14.0, 30.0, 60.0]
MIN_SURVIVAL_TIME_TO_MERGE_SUBJECTS = 20
SURVIVAL_TIME_TO_MERGE_RESTRICTED_MAX_DAYS = 180.0
MIN_TPM_DECISION_TARGET_SUBJECTS = 20
MIN_TPM_DECISION_TARGET_MEAN_LIFT = 0.10
MIN_TPM_DECISION_TARGET_CHRONO_LIFT = 0.10
FORECAST_FEATURE_COLUMNS = [
    "additions",
    "deletions",
    "changed_files",
    "commits",
    "comments",
    "review_comments",
    "linked_ticket_count",
    "requested_reviewer_count",
    "issue_key_text_count",
    "author_prior_pr_count",
    "author_prior_merged_pr_count",
    "author_prior_median_cycle_days",
    "author_open_pr_count",
    "draft",
    "additions_missing",
    "deletions_missing",
    "changed_files_missing",
    "commits_missing",
    "comments_missing",
    "review_comments_missing",
    "draft_missing",
]
SOURCE_SAFE_DERIVED_FORECAST_FEATURE_COLUMNS = [
    "total_lines_changed",
    "missing_feature_count",
    "author_merge_rate",
    "churn_per_file",
    "churn_per_commit",
    "comments_per_commit",
    "has_requested_reviewer",
    "has_linked_ticket",
    "has_issue_key_text",
    "log1p_additions",
    "log1p_deletions",
    "log1p_total_lines_changed",
    "log1p_changed_files",
    "log1p_commits",
    "log1p_comments",
    "log1p_review_comments",
    "log1p_linked_ticket_count",
    "log1p_requested_reviewer_count",
    "log1p_issue_key_text_count",
    "log1p_author_prior_pr_count",
    "log1p_author_prior_merged_pr_count",
    "log1p_author_open_pr_count",
    "log1p_author_prior_median_cycle_days",
]
CALENDAR_PROBE_FORECAST_FEATURE_COLUMNS = [
    "created_month_index",
    "created_quarter",
]
FORECAST_FORBIDDEN_FEATURE_COLUMNS = {
    "action_state",
    "actionability_label",
    "closed_at",
    "cycle_time_days",
    "decision",
    "decision_state",
    "label_quality",
    "merged_at",
    "overdue_days",
    "predicted_remaining_days",
    "predicted_total_cycle_days",
    "producer_state",
    "program_status",
    "ready_for_eta",
    "risk_band",
    "risk_score",
    "review_state",
    "truth_label",
}
FORECAST_FORBIDDEN_FEATURE_SUBSTRINGS = (
    "actionability_label",
    "calendar",
    "created_",
    "decision_state",
    "future_",
    "measurement_label",
    "_month",
    "predicted_",
    "_quarter",
    "review_label",
    "truth_label",
)


@dataclass(frozen=True)
class ManifestEntry:
    path: str
    source: str
    object_type: str
    object_id: str
    status_code: int


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixture-dir", required=True, type=Path)
    parser.add_argument("--ontology-db", required=True, type=Path)
    parser.add_argument("--analytics-db", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    parser.add_argument("--generated-at", default=None, help="Optional fixed analysis timestamp. Defaults to the latest source timestamp in the fixture.")
    parser.add_argument("--measurement-label-set", action="append", default=[], help="label set names to treat as measurement-eligible in readiness metrics")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    fixture_dir: Path = args.fixture_dir
    ontology_db: Path = args.ontology_db
    analytics_db: Path = args.analytics_db

    manifest = read_manifest(fixture_dir)
    cleaned_issue_keys = read_cleaned_issue_keys(fixture_dir)
    pr_payloads = read_pr_payloads(fixture_dir, manifest)
    pr_detail_payloads = read_pr_detail_payloads(fixture_dir, manifest)
    jira_payloads = read_jira_payloads(fixture_dir, manifest, cleaned_issue_keys)
    analysis_at = args.generated_at or infer_analysis_at(pr_payloads, jira_payloads)

    with sqlite3.connect(ontology_db) as ent_conn:
        ticket_pr_edges = pd.read_sql_query(
            """
            select
              t.external_id as ticket_key,
              pr.repository as repository,
              pr.number as pr_number,
              pr.key as pr_key,
              pr.freshness_state as pr_freshness,
              tpr.freshness_state as edge_freshness
            from ticket_pull_requests tpr
            join tickets t on t.id = tpr.ticket_id
            join pull_requests pr on pr.id = tpr.pull_request_id
            """,
            ent_conn,
        )
        people = pd.read_sql_query(
            """
            select
              p.key,
              p.display_name,
              coalesce(p.github_login, '') as github_login,
              coalesce(p.jira_account_id, '') as jira_account_id,
              coalesce(p.primary_email, '') as primary_email
            from persons p
            """,
            ent_conn,
        )
        person_activity = pd.read_sql_query(
            """
            select
              p.key,
              p.display_name,
              count(distinct ta.ticket_id) as jira_ticket_roles,
              count(distinct pra.pull_request_id) as pr_authored,
              count(distinct prr.pull_request_id) as pr_reviewed,
              count(distinct tm.ticket_id) as ticket_messages
            from persons p
            left join ticket_assignments ta on ta.person_id = p.id
            left join pull_request_authorships pra on pra.person_id = p.id
            left join pull_request_reviews prr on prr.person_id = p.id
            left join message_authorships ma on ma.person_id = p.id
            left join messages m on m.id = ma.message_id
            left join ticket_messages tm on tm.message_id = m.id
            group by p.id
            """,
            ent_conn,
        )
        developer_pr_authorships = pd.read_sql_query(
            """
            select
              p.key as person_key,
              p.display_name,
              coalesce(p.github_login, '') as github_login,
              coalesce(p.jira_account_id, '') as jira_account_id,
              pr.repository,
              pr.number as pr_number,
              pr.key as pr_key,
              pr.title,
              pr.state,
              pr.source_url as pr_url,
              pr.source_created_at,
              pr.source_updated_at,
              pr.closed_at,
              pr.merged_at
            from pull_request_authorships pra
            join persons p on p.id = pra.person_id
            join pull_requests pr on pr.id = pra.pull_request_id
            where pra.authorship_kind = 'author'
            """,
            ent_conn,
        )
        developer_ticket_roles = pd.read_sql_query(
            """
            select
              p.key as person_key,
              p.display_name,
              coalesce(p.github_login, '') as github_login,
              coalesce(p.jira_account_id, '') as jira_account_id,
              ta.assignment_kind,
              t.external_id as ticket_key,
              t.external_kind,
              t.title,
              t.status,
              t.priority,
              t.source_url,
              t.source_updated_at
            from ticket_assignments ta
            join persons p on p.id = ta.person_id
            join tickets t on t.id = ta.ticket_id
            where t.external_kind in ('jira_issue', 'jira_correlation_issue')
            """,
            ent_conn,
        )
        pr_review_summary = pd.read_sql_query(
            """
            select
              pr.repository as repository,
              pr.number as pr_number,
              count(*) as review_relationship_count,
              sum(case when prr.review_kind = 'requested_reviewer' then 1 else 0 end) as requested_reviewer_count,
              group_concat(distinct nullif(coalesce(p.github_login, p.display_name, ''), '')) as requested_reviewers,
              '' as latest_review_activity_at,
              max(coalesce(e.source_system, 'github')) as evidence_source_system,
              max(coalesce(e.source_instance, pr.source_instance)) as evidence_source_instance,
              max(coalesce(e.external_kind, 'github_pull_request')) as evidence_external_kind,
              max(coalesce(e.external_id, pr.external_id)) as evidence_external_id,
              max(coalesce(e.source_url, pr.source_url)) as evidence_source_url,
              max(coalesce(e.locator_kind, 'github_pull_request_review')) as evidence_locator_kind,
              max(coalesce(e.locator, pr.source_url)) as evidence_locator,
              max(coalesce(e.source_span_key, 'requested_reviewers')) as evidence_source_span_key,
              max(e.span_start) as evidence_span_start,
              max(e.span_end) as evidence_span_end,
              group_concat(distinct nullif(coalesce(e.excerpt, p.github_login, p.display_name, ''), '')) as evidence_excerpt
            from pull_request_reviews prr
            join pull_requests pr on pr.id = prr.pull_request_id
            join persons p on p.id = prr.person_id
            left join evidences e on e.id = prr.latest_evidence_id
            group by pr.repository, pr.number
            """,
            ent_conn,
        )
        typed_pr_rows = pd.read_sql_query(
            """
            select
              pr.id,
              pr.key,
              pr.repository,
              pr.number as pr_number,
              pr.title,
              pr.state,
              pr.source_url as pr_url,
              pr.source_created_at,
              pr.source_updated_at,
              pr.closed_at,
              pr.merged_at,
              pr.additions,
              pr.deletions,
              pr.changed_files_count,
              pr.commit_count,
              pr.issue_comment_count,
              pr.review_comment_count,
              pr.is_draft,
              pr.is_mergeable,
              pr.search_text,
              pr.freshness_state,
              pr.visibility,
              group_concat(distinct nullif(coalesce(p.github_login, p.display_name, ''), '')) as author_login
            from pull_requests pr
            left join pull_request_authorships pra on pra.pull_request_id = pr.id
              and pra.authorship_kind = 'author'
            left join persons p on p.id = pra.person_id
            where pr.state != 'unknown'
               or pr.source_created_at is not null
            group by pr.id
            """,
            ent_conn,
        )
        typed_ticket_rows = pd.read_sql_query(
            """
            select
              t.id,
              t.key,
              t.external_id as ticket_key,
              t.title,
              t.body,
              t.search_text,
              t.status,
              t.priority,
              t.source_url,
              t.source_updated_at,
              t.freshness_state,
              t.visibility,
              count(distinct tm.message_id) as comment_count,
              group_concat(distinct case when ta.assignment_kind = 'assignee' then nullif(coalesce(ap.display_name, ap.key, ''), '') end) as assignee,
              group_concat(distinct case when ta.assignment_kind = 'reporter' then nullif(coalesce(rp.display_name, rp.key, ''), '') end) as reporter,
              count(distinct coalesce(ta.person_id, ma.person_id)) as participant_count
            from tickets t
            left join ticket_assignments ta on ta.ticket_id = t.id
            left join persons ap on ap.id = ta.person_id and ta.assignment_kind = 'assignee'
            left join persons rp on rp.id = ta.person_id and ta.assignment_kind = 'reporter'
            left join ticket_messages tm on tm.ticket_id = t.id
            left join message_authorships ma on ma.message_id = tm.message_id
            group by t.id
            """,
            ent_conn,
        )
        pull_request_subjects = pd.read_sql_query(
            """
            select id, key, repository, number
            from pull_requests
            """,
            ent_conn,
        )
        ticket_subjects = pd.read_sql_query(
            """
            select id, key, external_id
            from tickets
            """,
            ent_conn,
        )
        current_pr_source_coverage = read_current_fixture_pr_source_coverage(ent_conn, fixture_dir.name)

    pr_features = build_pr_features(typed_pr_rows, pr_payloads, ticket_pr_edges, pr_review_summary, current_pr_source_coverage, analysis_at)
    event_pr_feature_snapshots = build_event_pr_feature_snapshots(pr_features, pr_detail_payloads)
    pr_source_coverage = build_pr_source_coverage(pr_features)
    ticket_features = build_ticket_features(typed_ticket_rows, ticket_pr_edges)
    milestone_signals = build_milestone_signals(jira_payloads, pr_payloads, fixture_dir.name, analysis_at)
    feature_provenance = build_feature_provenance()
    blocker_candidates = build_blocker_candidates(jira_payloads, pr_payloads, ticket_pr_edges)
    analytics_db.parent.mkdir(parents=True, exist_ok=True)
    with sqlite3.connect(analytics_db) as out_conn:
        time_series_summary = persist_time_series_snapshots(
            out_conn,
            fixture_dir.name,
            analysis_at,
            pr_features,
            ticket_features,
            event_pr_feature_snapshots=event_pr_feature_snapshots,
        )
        transition_signal_readiness = pd.read_sql_query("select * from tpm_transition_signal_readiness", out_conn)
    temporal_feature_snapshot_ready = metric_map(time_series_summary).get("as_of_feature_snapshot_ready") == "true"
    forecast_summary, pr_forecasts, forecast_backtest, forecast_risk_backtest = build_forecasts(
        pr_features,
        temporal_feature_snapshot_ready=temporal_feature_snapshot_ready,
        event_pr_feature_snapshots=event_pr_feature_snapshots,
    )
    forecast_reliability = build_forecast_reliability(forecast_summary)
    forecast_feature_set_readiness = build_forecast_feature_set_readiness_matrix(
        pr_features,
        temporal_feature_snapshot_ready=temporal_feature_snapshot_ready,
    )
    decision_target_backtest = build_tpm_decision_target_backtest(event_pr_feature_snapshots, forecast_feature_columns())
    decision_target_readiness = build_tpm_decision_target_readiness(decision_target_backtest)
    developer_correlation = build_developer_correlation(
        developer_pr_authorships,
        developer_ticket_roles,
        pr_forecasts,
        ticket_features,
    )
    developer_correlation_validation = build_developer_correlation_validation(developer_correlation)
    dependency_edges = build_dependency_edges(ticket_pr_edges)
    review_bottlenecks = build_review_bottlenecks(pr_forecasts)
    insight_cards = build_insight_cards(
        pr_forecasts,
        ticket_features,
        blocker_candidates,
        dependency_edges,
        review_bottlenecks,
        forecast_summary,
        forecast_backtest,
        developer_correlation,
    )

    with sqlite3.connect(analytics_db) as out_conn:
        write_table(out_conn, "tpm_pr_features", pr_features)
        write_table(out_conn, "tpm_pr_source_coverage", pr_source_coverage)
        write_table(out_conn, "tpm_milestone_signals", milestone_signals)
        write_table(out_conn, "tpm_feature_provenance", feature_provenance)
        write_table(out_conn, "tpm_pr_forecasts", pr_forecasts)
        write_table(out_conn, "tpm_ticket_features", ticket_features)
        write_table(out_conn, "tpm_person_activity", person_activity)
        write_table(out_conn, "tpm_people", people)
        write_table(out_conn, "tpm_developer_correlation", developer_correlation)
        write_table(out_conn, "tpm_developer_correlation_validation", developer_correlation_validation)
        write_table(out_conn, "tpm_blocker_candidates", blocker_candidates)
        write_table(out_conn, "tpm_dependency_edges", dependency_edges)
        write_table(out_conn, "tpm_review_bottlenecks", review_bottlenecks)
        write_table(out_conn, "tpm_forecast_summary", forecast_summary)
        write_table(out_conn, "tpm_forecast_reliability", forecast_reliability)
        write_table(out_conn, "tpm_forecast_backtest", forecast_backtest)
        write_table(out_conn, "tpm_forecast_feature_set_readiness", forecast_feature_set_readiness)
        write_table(out_conn, "tpm_forecast_risk_backtest", forecast_risk_backtest)
        write_table(out_conn, "tpm_decision_target_backtest", decision_target_backtest)
        write_table(out_conn, "tpm_decision_target_readiness", decision_target_readiness)
        write_table(out_conn, "tpm_insight_cards", insight_cards)
        out_conn.execute(
            """
            create table if not exists tpm_run_metadata (
              key text primary key,
              value text not null
            )
            """
        )
        out_conn.executemany(
            "insert or replace into tpm_run_metadata(key, value) values (?, ?)",
            [
                ("generated_at", analysis_at),
                ("fixture_dir", str(fixture_dir)),
                ("ontology_db", str(ontology_db)),
                ("pr_payload_count", str(len(pr_payloads))),
                ("jira_payload_count", str(len(jira_payloads))),
                ("analytics_version", ANALYTICS_VERSION),
            ],
        )
        write_table(out_conn, "tpm_time_series_summary", time_series_summary)

    with sqlite3.connect(ontology_db) as ontology_conn:
        persist_work_insights(
            ontology_conn,
            insight_cards,
            pull_request_subjects,
            ticket_subjects,
            fixture_dir.name,
            analysis_at,
            args.report,
        )
        persist_work_insight_review_requests(ontology_conn, fixture_dir.name, analysis_at)
        review_queue = read_work_insight_review_queue(ontology_conn, fixture_dir.name, set(args.measurement_label_set or []))
        evaluation_readiness = build_evaluation_readiness(insight_cards, review_queue)

    with sqlite3.connect(analytics_db) as out_conn:
        write_table(out_conn, "tpm_insight_review_queue", review_queue)
        write_table(out_conn, "tpm_evaluation_readiness", evaluation_readiness)

    write_report(
        args.report,
        analysis_at,
        pr_features,
        pr_forecasts,
        ticket_features,
        blocker_candidates,
        dependency_edges,
        review_bottlenecks,
        forecast_summary,
        forecast_reliability,
        forecast_backtest,
        forecast_feature_set_readiness,
        forecast_risk_backtest,
        decision_target_backtest,
        decision_target_readiness,
        milestone_signals,
        feature_provenance,
        pr_source_coverage,
        developer_correlation,
        developer_correlation_validation,
        time_series_summary,
        transition_signal_readiness,
        insight_cards,
        review_queue,
        evaluation_readiness,
    )


def read_manifest(fixture_dir: Path) -> list[ManifestEntry]:
    ndjson = fixture_dir / "manifest.ndjson"
    if ndjson.exists():
        entries: list[ManifestEntry] = []
        for line in ndjson.read_text().splitlines():
            if not line.strip():
                continue
            raw = json.loads(line)
            entries.append(
                ManifestEntry(
                    path=raw["path"],
                    source=raw["source"],
                    object_type=raw["source_object_type"],
                    object_id=raw["source_object_id"],
                    status_code=int(raw["status_code"]),
                )
            )
        return entries

    manifest_json = fixture_dir / "manifest.json"
    raw_manifest = json.loads(manifest_json.read_text())
    entries = []
    for raw in raw_manifest["snapshots"]:
        entries.append(
            ManifestEntry(
                path=raw["path"],
                source=raw["source_key"],
                object_type=raw["source_object_type"],
                object_id=raw["source_object_id"],
                status_code=int(raw["response"]["status_code"]),
            )
        )
    return entries


def read_cleaned_issue_keys(fixture_dir: Path) -> set[str] | None:
    cleaned = fixture_dir / "analysis" / "jira_issue_keys.cleaned.txt"
    if not cleaned.exists():
        return None
    return {line.strip().upper() for line in cleaned.read_text().splitlines() if line.strip()}


def read_pr_payloads(fixture_dir: Path, manifest: list[ManifestEntry]) -> dict[str, dict[str, Any]]:
    payloads: dict[str, dict[str, Any]] = {}
    for entry in manifest:
        if entry.object_type != "github_pull_request" or entry.status_code != 200:
            continue
        payload = json.loads((fixture_dir / entry.path).read_text())
        payloads[entry.object_id] = payload
    return payloads


def read_pr_detail_payloads(fixture_dir: Path, manifest: list[ManifestEntry]) -> dict[str, dict[str, Any]]:
    detail_types = {
        "github_issue_comments": "issue_comments",
        "github_pull_request_review_comments": "review_comments",
        "github_pull_request_reviews": "reviews",
        "github_pull_request_commits": "commits",
        "github_pull_request_files": "files",
    }
    payloads: dict[str, dict[str, Any]] = {}
    for entry in manifest:
        detail_key = detail_types.get(entry.object_type)
        if detail_key is None or entry.status_code != 200:
            continue
        subject = payloads.setdefault(entry.object_id, {})
        subject[detail_key] = json.loads((fixture_dir / entry.path).read_text())
    return payloads


def read_jira_payloads(
    fixture_dir: Path,
    manifest: list[ManifestEntry],
    cleaned_issue_keys: set[str] | None,
) -> dict[str, dict[str, Any]]:
    payloads: dict[str, dict[str, Any]] = {}
    for entry in manifest:
        if entry.object_type not in JIRA_ISSUE_OBJECT_TYPES or entry.status_code != 200:
            continue
        key = entry.object_id.upper()
        if cleaned_issue_keys is not None and key not in cleaned_issue_keys:
            continue
        payloads[key] = json.loads((fixture_dir / entry.path).read_text())
    return payloads


def read_current_fixture_pr_source_coverage(conn: sqlite3.Connection, stream_key: str) -> pd.DataFrame:
    columns = [
        "subject_key",
        "source_current_issue_count",
        "source_current_detail_issue_count",
        "source_current_issue_codes",
        "source_current_issue_kinds",
        "source_current_failure_message",
        "source_current_sync_run_key",
        "source_current_run_status",
        "source_current_coverage_mode",
        "source_current_observed_at",
    ]
    required_tables = {"source_scopes", "source_sync_runs", "source_sync_issues"}
    if any(not table_exists(conn, table) for table in required_tables):
        return pd.DataFrame(columns=columns)
    placeholders = ",".join("?" for _ in SOURCE_PR_BUNDLE_KINDS)
    query = f"""
        with latest_run as (
            select
              ssr.id,
              ssr.run_key,
              ssr.status,
              ssr.coverage_mode,
              coalesce(ssr.completed_at, ssr.started_at, ssr.created_at) as observed_at
            from source_sync_runs ssr
            join source_scopes ss on ss.id = ssr.source_scope_id
            where ss.scope_key = ?
              and ssr.coverage_mode in ('exact_scope', 'partial_scope')
            order by coalesce(ssr.completed_at, ssr.started_at, ssr.created_at) desc, ssr.id desc
            limit 1
        )
        select
          ssi.external_id as subject_key,
          count(*) as source_current_issue_count,
          sum(case when ssi.external_kind in ({placeholders}) then 1 else 0 end) as source_current_detail_issue_count,
          group_concat(distinct ssi.issue_code) as source_current_issue_codes,
          group_concat(distinct ssi.external_kind) as source_current_issue_kinds,
          max(ssi.message) as source_current_failure_message,
          max(latest_run.run_key) as source_current_sync_run_key,
          max(latest_run.status) as source_current_run_status,
          max(latest_run.coverage_mode) as source_current_coverage_mode,
          max(latest_run.observed_at) as source_current_observed_at
        from source_sync_issues ssi
        join latest_run on latest_run.id = ssi.source_sync_run_id
        where ssi.source_system = 'github'
          and ssi.external_kind in ({placeholders})
          and coalesce(ssi.external_id, '') != ''
        group by ssi.external_id
    """
    return pd.read_sql_query(query, conn, params=[stream_key, *SOURCE_PR_BUNDLE_KINDS, *SOURCE_PR_BUNDLE_KINDS])


def infer_analysis_at(
    pr_payloads: dict[str, dict[str, Any]],
    jira_payloads: dict[str, dict[str, Any]],
) -> str:
    observed_times: list[datetime] = []
    for payload in pr_payloads.values():
        for key in ["updated_at", "created_at", "closed_at", "merged_at"]:
            dt = parse_dt(payload.get(key))
            if dt is not None:
                observed_times.append(dt)
    for payload in jira_payloads.values():
        fields = payload.get("fields") or {}
        for key in ["updated", "created", "resolutiondate"]:
            dt = parse_dt(fields.get(key))
            if dt is not None:
                observed_times.append(dt)
        for comment in (((fields.get("comment") or {}).get("comments")) or []):
            for key in ["updated", "created"]:
                dt = parse_dt(comment.get(key))
                if dt is not None:
                    observed_times.append(dt)
    if not observed_times:
        return datetime.now(timezone.utc).isoformat()
    return max(observed_times).isoformat()


def build_pr_features(
    typed_pr_rows: pd.DataFrame,
    pr_payloads: dict[str, dict[str, Any]],
    ticket_pr_edges: pd.DataFrame,
    pr_review_summary: pd.DataFrame,
    current_pr_source_coverage: pd.DataFrame,
    generated_at: str,
) -> pd.DataFrame:
    edge_counts = ticket_pr_edges.groupby(["repository", "pr_number"]).agg(
        linked_ticket_count=("ticket_key", "nunique"),
        partial_ticket_link_count=("pr_freshness", lambda values: int((values == "partial").sum())),
    )
    review_counts = pd.DataFrame()
    if not pr_review_summary.empty:
        review_counts = pr_review_summary.set_index(["repository", "pr_number"])
    source_coverage = pd.DataFrame()
    if not current_pr_source_coverage.empty:
        source_coverage = current_pr_source_coverage.set_index("subject_key")
    generated_dt = parse_dt(generated_at) or datetime.now(timezone.utc)
    rows = []
    for pr_row in typed_pr_rows.itertuples(index=False):
        repo = str(pr_row.repository)
        number = int(pr_row.pr_number)
        subject_key = f"{repo}#{number}"
        payload = pr_payloads.get(f"{repo}#{number}", {})
        coverage_row = source_coverage.loc[subject_key].to_dict() if not source_coverage.empty and subject_key in source_coverage.index else {}
        coverage_state = pr_source_coverage_state(coverage_row)
        created_at = parse_dt(getattr(pr_row, "source_created_at", "")) or parse_dt(payload.get("created_at"))
        updated_at = parse_dt(getattr(pr_row, "source_updated_at", "")) or parse_dt(payload.get("updated_at"))
        closed_at = parse_dt(getattr(pr_row, "closed_at", "")) or parse_dt(payload.get("closed_at"))
        merged_at = parse_dt(getattr(pr_row, "merged_at", "")) or parse_dt(payload.get("merged_at"))
        terminal_at = merged_at or closed_at
        age_days = days_between(created_at, generated_dt)
        cycle_time_days = days_between(created_at, terminal_at) if terminal_at else None
        linked_counts = edge_counts.loc[(repo, number)].to_dict() if (repo, number) in edge_counts.index else {}
        review_row = review_counts.loc[(repo, number)].to_dict() if not review_counts.empty and (repo, number) in review_counts.index else {}
        latest_review_activity_at = parse_dt(review_row.get("latest_review_activity_at"))
        search_text = clean_text(getattr(pr_row, "search_text", "")) or "\n".join([clean_text(payload.get("title")), clean_text(payload.get("body"))])
        issue_keys_in_text = sorted(set(ISSUE_RE.findall(search_text)))
        typed_state = clean_text(getattr(pr_row, "state", ""))
        state = typed_state if typed_state and typed_state != "unknown" else normalize_pr_state(payload)
        lifecycle_source = "typed_pull_request" if created_at and (updated_at or terminal_at or state == "open") else "replay_payload_fallback"
        additions, additions_source = typed_int_or_payload(pr_row, "additions", payload, "additions")
        deletions, deletions_source = typed_int_or_payload(pr_row, "deletions", payload, "deletions")
        changed_files, changed_files_source = typed_int_or_payload(pr_row, "changed_files_count", payload, "changed_files")
        commits, commits_source = typed_int_or_payload(pr_row, "commit_count", payload, "commits")
        comments, comments_source = typed_int_or_payload(pr_row, "issue_comment_count", payload, "comments")
        review_comments, review_comments_source = typed_int_or_payload(pr_row, "review_comment_count", payload, "review_comments")
        draft, draft_source = typed_bool_int_or_payload(pr_row, "is_draft", payload, "draft")
        mergeable, mergeable_source = typed_bool_int_or_payload(pr_row, "is_mergeable", payload, "mergeable")
        churn_source = aggregate_feature_source(
            [
                additions_source,
                deletions_source,
                changed_files_source,
                commits_source,
                comments_source,
                review_comments_source,
                draft_source,
            ]
        )
        rows.append(
            {
                "repository": repo,
                "pr_number": number,
                "pr_url": clean_text(getattr(pr_row, "pr_url", "")) or clean_text(payload.get("html_url")),
                "title": clean_text(getattr(pr_row, "title", "")) or clean_text(payload.get("title")),
                "state": state,
                "is_terminal": int(bool(terminal_at)),
                "is_merged": int(bool(merged_at)),
                "created_at": iso_or_none(created_at),
                "updated_at": iso_or_none(updated_at),
                "closed_at": iso_or_none(closed_at),
                "merged_at": iso_or_none(merged_at),
                "age_days": round(age_days, 2) if age_days is not None else None,
                "cycle_time_days": round(cycle_time_days, 2) if cycle_time_days is not None else None,
                "stale_days": round(days_between(updated_at, generated_dt) or 0, 2),
                "additions": additions,
                "deletions": deletions,
                "changed_files": changed_files,
                "commits": commits,
                "comments": comments,
                "review_comments": review_comments,
                "draft": draft,
                "mergeable": mergeable,
                "additions_missing": int(additions_source == "missing"),
                "deletions_missing": int(deletions_source == "missing"),
                "changed_files_missing": int(changed_files_source == "missing"),
                "commits_missing": int(commits_source == "missing"),
                "comments_missing": int(comments_source == "missing"),
                "review_comments_missing": int(review_comments_source == "missing"),
                "draft_missing": int(draft_source == "missing"),
                "mergeable_known": int(mergeable_source != "missing"),
                "linked_ticket_count": int(linked_counts.get("linked_ticket_count", 0)),
                "partial_ticket_link_count": int(linked_counts.get("partial_ticket_link_count", 0)),
                "issue_key_text_count": len(issue_keys_in_text),
                "review_relationship_count": int(review_row.get("review_relationship_count", 0) or 0),
                "requested_reviewer_count": int(review_row.get("requested_reviewer_count", 0) or 0),
                "requested_reviewers": clean_text(review_row.get("requested_reviewers")),
                "latest_review_activity_at": iso_or_none(latest_review_activity_at),
                "days_since_review_activity": round(days_between(latest_review_activity_at, generated_dt) or 0, 2) if latest_review_activity_at else None,
                "review_evidence_excerpt": clean_text(review_row.get("evidence_excerpt")),
                "review_evidence_source_system": clean_text(review_row.get("evidence_source_system")),
                "review_evidence_source_instance": clean_text(review_row.get("evidence_source_instance")),
                "review_evidence_external_kind": clean_text(review_row.get("evidence_external_kind")),
                "review_evidence_external_id": clean_text(review_row.get("evidence_external_id")),
                "review_evidence_source_url": clean_text(review_row.get("evidence_source_url")),
                "review_evidence_locator_kind": clean_text(review_row.get("evidence_locator_kind")),
                "review_evidence_locator": clean_text(review_row.get("evidence_locator")),
                "review_evidence_source_span_key": clean_text(review_row.get("evidence_source_span_key")),
                "review_evidence_span_start": clean_int(review_row.get("evidence_span_start")),
                "review_evidence_span_end": clean_int(review_row.get("evidence_span_end")),
                "author_login": clean_text(getattr(pr_row, "author_login", "")) or ((payload.get("user") or {}).get("login") or ""),
                "issue_keys_in_text": ",".join(issue_keys_in_text),
                "identity_fields_source": "typed_pull_request",
                "text_fields_source": "typed_pull_request" if clean_text(getattr(pr_row, "search_text", "")) else "replay_payload_fallback",
                "lifecycle_fields_source": lifecycle_source,
                "review_fields_source": "typed_pull_request_reviews",
                "churn_fields_source": churn_source,
                "mergeability_fields_source": aggregate_feature_source([mergeable_source]),
                "source_current_coverage_state": coverage_state,
                "source_current_detail_state": "failed" if int(coverage_row.get("source_current_detail_issue_count") or 0) > 0 else "observed",
                "source_current_issue_count": int(coverage_row.get("source_current_issue_count") or 0),
                "source_current_detail_issue_count": int(coverage_row.get("source_current_detail_issue_count") or 0),
                "source_current_issue_codes": clean_text(coverage_row.get("source_current_issue_codes")),
                "source_current_issue_kinds": clean_text(coverage_row.get("source_current_issue_kinds")),
                "source_current_failure_message": clean_text(coverage_row.get("source_current_failure_message")),
                "source_current_sync_run_key": clean_text(coverage_row.get("source_current_sync_run_key")),
                "source_current_run_status": clean_text(coverage_row.get("source_current_run_status")),
                "source_current_coverage_mode": clean_text(coverage_row.get("source_current_coverage_mode")),
                "source_current_observed_at": clean_text(coverage_row.get("source_current_observed_at")),
                "source_freshness_state": clean_text(getattr(pr_row, "freshness_state", "")),
                "source_visibility": clean_text(getattr(pr_row, "visibility", "")),
            }
        )
    df = pd.DataFrame(rows)
    if not df.empty:
        df["total_lines_changed"] = df["additions"] + df["deletions"]
        df = add_author_history_features(df)
    return df


def add_author_history_features(
    rows: pd.DataFrame,
    *,
    history: pd.DataFrame | None = None,
    as_of_column: str = "created_at",
) -> pd.DataFrame:
    out = rows.copy()
    defaults = {
        "author_prior_pr_count": 0,
        "author_prior_merged_pr_count": 0,
        "author_prior_median_cycle_days": 0.0,
        "author_open_pr_count": 0,
    }
    if out.empty:
        for column, value in defaults.items():
            out[column] = value
        return out
    hist = (history.copy() if history is not None else out.copy())
    if "author_login" not in out.columns or "author_login" not in hist.columns:
        for column, value in defaults.items():
            out[column] = value
        return out

    hist["_created_dt"] = hist.get("created_at", pd.Series("", index=hist.index)).map(parse_dt)
    terminal_source = hist.get("merged_at", pd.Series("", index=hist.index)).fillna("")
    if "closed_at" in hist.columns:
        terminal_source = terminal_source.mask(terminal_source == "", hist["closed_at"].fillna(""))
    hist["_terminal_dt"] = terminal_source.map(parse_dt)
    hist["_cycle_time_days"] = pd.to_numeric(hist.get("cycle_time_days", pd.Series([None] * len(hist))), errors="coerce")

    prior_pr_counts: list[int] = []
    prior_merged_counts: list[int] = []
    prior_medians: list[float] = []
    open_counts: list[int] = []
    for row in out.itertuples(index=False):
        author = clean_text(getattr(row, "author_login", ""))
        as_of = parse_dt(clean_text(getattr(row, as_of_column, "")))
        if not author or as_of is None:
            prior_pr_counts.append(0)
            prior_merged_counts.append(0)
            prior_medians.append(0.0)
            open_counts.append(0)
            continue
        prior = hist[(hist["author_login"].map(clean_text) == author) & (hist["_created_dt"].notna()) & (hist["_created_dt"] < as_of)]
        prior_merged = prior[
            prior["_terminal_dt"].notna()
            & (prior["_terminal_dt"] < as_of)
            & prior["_cycle_time_days"].notna()
        ]
        open_at_as_of = prior[
            prior["_created_dt"].notna()
            & (prior["_created_dt"] < as_of)
            & (prior["_terminal_dt"].isna() | (prior["_terminal_dt"] > as_of))
        ]
        prior_pr_counts.append(int(len(prior)))
        prior_merged_counts.append(int(len(prior_merged)))
        prior_medians.append(float(prior_merged["_cycle_time_days"].median()) if not prior_merged.empty else 0.0)
        open_counts.append(int(len(open_at_as_of)))

    out["author_prior_pr_count"] = prior_pr_counts
    out["author_prior_merged_pr_count"] = prior_merged_counts
    out["author_prior_median_cycle_days"] = [round(value, 4) for value in prior_medians]
    out["author_open_pr_count"] = open_counts
    return out


def build_event_pr_feature_snapshots(pr_features: pd.DataFrame, pr_detail_payloads: dict[str, dict[str, Any]]) -> pd.DataFrame:
    if pr_features.empty:
        return pd.DataFrame()
    rows: list[dict[str, Any]] = []
    for current in pr_features.itertuples(index=False):
        repository = clean_text(getattr(current, "repository", ""))
        pr_number = clean_int(getattr(current, "pr_number", None))
        if not repository or pr_number is None:
            continue
        subject_key = f"{repository}#{pr_number}"
        details = pr_detail_payloads.get(subject_key, {})
        created_at = parse_dt(clean_text(getattr(current, "created_at", "")))
        if created_at is None:
            continue
        current_state = clean_text(getattr(current, "state", "")).lower()
        terminal_at = parse_dt(clean_text(getattr(current, "merged_at", "")) or clean_text(getattr(current, "closed_at", "")))
        target_cycle_time_days = safe_float(getattr(current, "cycle_time_days", None))
        target_merged = 1 if clean_bool(getattr(current, "is_merged", False)) or current_state == "merged" else 0

        commit_times = sorted(dt for dt in pr_commit_event_times(details.get("commits", [])) if dt is not None)
        issue_comment_times = sorted(dt for dt in item_event_times(details.get("issue_comments", []), ["created_at", "updated_at"]) if dt is not None)
        review_comment_times = sorted(dt for dt in item_event_times(details.get("review_comments", []), ["created_at", "updated_at"]) if dt is not None)
        review_times = sorted(dt for dt in item_event_times(details.get("reviews", []), ["submitted_at"]) if dt is not None)
        observed_times = sorted({created_at, *commit_times, *issue_comment_times, *review_comment_times, *review_times})
        if terminal_at is not None:
            observed_times = [dt for dt in observed_times if dt < terminal_at]
        if not observed_times:
            continue

        for observed_at in observed_times:
            age_days = days_between(created_at, observed_at)
            rows.append(
                {
                    "repository": repository,
                    "pr_number": pr_number,
                    "state": "open",
                    "created_at": iso_or_none(created_at),
                    "updated_at": iso_or_none(observed_at),
                    "closed_at": iso_or_none(terminal_at) if terminal_at is not None and current_state == "closed" else "",
                    "merged_at": iso_or_none(terminal_at) if target_merged else "",
                    "age_days": round(age_days, 2) if age_days is not None else None,
                    "cycle_time_days": target_cycle_time_days,
                    "is_merged": target_merged,
                    "additions": 0,
                    "deletions": 0,
                    "changed_files": 0,
                    "commits": count_datetimes_at_or_before(commit_times, observed_at),
                    "comments": count_datetimes_at_or_before(issue_comment_times, observed_at),
                    "review_comments": count_datetimes_at_or_before(review_comment_times, observed_at),
                    "linked_ticket_count": 0,
                    "requested_reviewer_count": 0,
                    "issue_key_text_count": 0,
                    "draft": False,
                    "additions_missing": 1,
                    "deletions_missing": 1,
                    "changed_files_missing": 1,
                    "commits_missing": 0,
                    "comments_missing": 0,
                    "review_comments_missing": 0,
                    "draft_missing": 1,
                    "total_lines_changed": 0,
                    "source_current_coverage_state": "observed",
                    "source_current_detail_state": "observed",
                    "lifecycle_fields_source": "source_event_replay",
                    "churn_fields_source": "source_event_replay_safe_dynamic_counts_only",
                    "review_fields_source": "source_event_replay",
                    "event_replay_observed_at": iso_or_none(observed_at),
                    "author_login": clean_text(getattr(current, "author_login", "")),
                }
            )
    if not rows:
        return pd.DataFrame()
    snapshots = pd.DataFrame(rows).drop_duplicates(["repository", "pr_number", "event_replay_observed_at"])
    return add_author_history_features(snapshots, history=pr_features, as_of_column="event_replay_observed_at")


def pr_commit_event_times(commits: Any) -> list[datetime]:
    if not isinstance(commits, list):
        return []
    times: list[datetime] = []
    for item in commits:
        if not isinstance(item, dict):
            continue
        commit = item.get("commit") if isinstance(item.get("commit"), dict) else {}
        author = commit.get("author") if isinstance(commit.get("author"), dict) else {}
        committer = commit.get("committer") if isinstance(commit.get("committer"), dict) else {}
        dt = parse_dt(author.get("date")) or parse_dt(committer.get("date"))
        if dt is not None:
            times.append(dt)
    return times


def item_event_times(items: Any, keys: list[str]) -> list[datetime]:
    if not isinstance(items, list):
        return []
    times: list[datetime] = []
    for item in items:
        if not isinstance(item, dict):
            continue
        for key in keys:
            dt = parse_dt(item.get(key))
            if dt is not None:
                times.append(dt)
                break
    return times


def count_datetimes_at_or_before(values: list[datetime], observed_at: datetime) -> int:
    return sum(1 for value in values if value <= observed_at)


def pr_source_coverage_state(coverage_row: dict[str, Any]) -> str:
    issue_count = int(coverage_row.get("source_current_issue_count") or 0)
    detail_issue_count = int(coverage_row.get("source_current_detail_issue_count") or 0)
    if detail_issue_count > 0:
        return "detail_failed"
    if issue_count > 0:
        return "coverage_limited"
    return "observed"


def build_pr_source_coverage(pr_features: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "repository",
        "pr_number",
        "subject_key",
        "source_current_coverage_state",
        "source_current_detail_state",
        "source_current_issue_count",
        "source_current_detail_issue_count",
        "source_current_issue_codes",
        "source_current_issue_kinds",
        "source_current_failure_message",
        "source_current_sync_run_key",
        "source_current_run_status",
        "source_current_coverage_mode",
        "source_current_observed_at",
    ]
    if pr_features.empty:
        return pd.DataFrame(columns=columns)
    rows = pr_features.copy()
    rows["subject_key"] = rows["repository"].astype(str) + "#" + rows["pr_number"].astype(str)
    return rows[columns]


def typed_int_or_payload(row: Any, row_field: str, payload: dict[str, Any], payload_key: str) -> tuple[int | None, str]:
    typed_value = clean_int(getattr(row, row_field, None))
    if typed_value is not None:
        return typed_value, "typed_pull_request"
    payload_value = clean_int(payload.get(payload_key)) if payload_key in payload else None
    if payload_value is not None:
        return payload_value, "replay_payload_fallback"
    return None, "missing"


def typed_bool_int_or_payload(row: Any, row_field: str, payload: dict[str, Any], payload_key: str) -> tuple[int | None, str]:
    typed_value = getattr(row, row_field, None)
    if has_value(typed_value):
        return int(clean_bool(typed_value)), "typed_pull_request"
    if payload_key in payload and payload.get(payload_key) is not None:
        return int(clean_bool(payload.get(payload_key))), "replay_payload_fallback"
    return None, "missing"


def aggregate_feature_source(sources: list[str]) -> str:
    observed = {source for source in sources if source}
    if observed == {"typed_pull_request"}:
        return "typed_pull_request"
    if observed == {"replay_payload_fallback"}:
        return "replay_payload_fallback"
    if observed == {"missing"}:
        return "missing"
    if "typed_pull_request" in observed and "replay_payload_fallback" in observed:
        return "typed_pull_request_with_replay_fallback"
    if "typed_pull_request" in observed and "missing" in observed:
        return "typed_pull_request_with_missing_values"
    if "replay_payload_fallback" in observed and "missing" in observed:
        return "replay_payload_fallback_with_missing_values"
    return ",".join(sorted(observed))


def normalize_pr_state(payload: dict[str, Any]) -> str:
    if payload.get("merged_at"):
        return "merged"
    return payload.get("state") or "unknown"


def normalize_ticket_state(status: str) -> str:
    lowered = (status or "").strip().lower()
    if lowered in {"closed", "done", "resolved", "complete", "completed"}:
        return "closed"
    if lowered:
        return "open"
    return "unknown"


def is_current_ticket_status(status: str) -> bool:
    return normalize_ticket_state(status) != "closed"


def build_ticket_features(
    typed_ticket_rows: pd.DataFrame,
    ticket_pr_edges: pd.DataFrame,
) -> pd.DataFrame:
    edge_counts = ticket_pr_edges.groupby("ticket_key").agg(
        linked_pr_count=("pr_number", "nunique"),
        partial_pr_link_count=("pr_freshness", lambda values: int((values == "partial").sum())),
        fresh_pr_link_count=("pr_freshness", lambda values: int((values == "fresh").sum())),
    )
    rows = []
    for ticket_row in typed_ticket_rows.itertuples(index=False):
        key = str(ticket_row.ticket_key)
        text = "\n".join(
            [
                clean_text(getattr(ticket_row, "title", "")),
                clean_text(getattr(ticket_row, "body", "")),
                clean_text(getattr(ticket_row, "search_text", "")),
            ]
        )
        edge_row = edge_counts.loc[key].to_dict() if key in edge_counts.index else {}
        rows.append(
            {
                "ticket_key": key,
                "title": clean_text(getattr(ticket_row, "title", "")) or key,
                "status": clean_text(getattr(ticket_row, "status", "")) or "unknown",
                "priority": clean_text(getattr(ticket_row, "priority", "")),
                "updated_at": clean_text(getattr(ticket_row, "source_updated_at", "")),
                "assignee": clean_text(getattr(ticket_row, "assignee", "")),
                "reporter": clean_text(getattr(ticket_row, "reporter", "")),
                "comment_count": int(getattr(ticket_row, "comment_count", 0) or 0),
                "participant_count": int(getattr(ticket_row, "participant_count", 0) or 0),
                "linked_pr_count": int(edge_row.get("linked_pr_count", 0)),
                "fresh_pr_link_count": int(edge_row.get("fresh_pr_link_count", 0)),
                "partial_pr_link_count": int(edge_row.get("partial_pr_link_count", 0)),
                "blocker_keyword_count": len(blocker_matches(text)),
                "text_issue_key_count": len(set(ISSUE_RE.findall(text))),
                "identity_fields_source": "typed_ticket",
                "text_fields_source": "typed_ticket",
                "relationship_fields_source": "typed_ticket_pull_requests",
                "people_fields_source": "typed_ticket_assignments_messages",
            }
        )
    return pd.DataFrame(rows)


def milestone_signal_columns() -> list[str]:
    return [
        "workstream_key",
        "subject_kind",
        "subject_key",
        "milestone_kind",
        "milestone_name",
        "target_date",
        "outcome_date",
        "milestone_state",
        "commitment_strength",
        "date_claim_allowed",
        "delivery_commitment_allowed",
        "claim_gate_reason",
        "source_system",
        "source_instance",
        "external_kind",
        "external_id",
        "source_field",
        "source_payload_key",
        "source_url",
        "captured_at",
        "generated_at",
        "rank_score",
    ]


def build_milestone_signals(
    jira_payloads: dict[str, dict[str, Any]],
    pr_payloads: dict[str, dict[str, Any]],
    source_instance: str,
    generated_at: str,
) -> pd.DataFrame:
    generated_dt = parse_dt(generated_at) or datetime.now(timezone.utc)
    rows: list[dict[str, Any]] = []
    workstream_key = "flink-kubernetes-operator"
    for key, payload in sorted(jira_payloads.items()):
        fields = payload.get("fields") or {}
        subject_key = str(key).upper()
        source_url = jira_issue_url(subject_key, payload)
        resolution_dt = parse_dt(fields.get("resolutiondate"))
        captured_at = iso_or_none(parse_dt(fields.get("updated"))) or generated_at
        due_dt = parse_dt(fields.get("duedate"))
        if due_dt is not None:
            rows.append(
                milestone_signal_row(
                    source_instance=source_instance,
                    workstream_key=workstream_key,
                    subject_kind="ticket",
                    subject_key=subject_key,
                    milestone_kind="explicit_due_date",
                    milestone_name="Jira due date",
                    target_dt=due_dt,
                    outcome_dt=resolution_dt,
                    commitment_strength="explicit_commitment",
                    date_claim_allowed=True,
                    delivery_commitment_allowed=True,
                    claim_gate_reason="source_native_due_date",
                    source_field="jira.fields.duedate",
                    source_payload_key=f"{subject_key}:fields.duedate",
                    source_url=source_url,
                    captured_at=captured_at,
                    generated_at=generated_at,
                    rank_score=100.0,
                )
            )
        for version in fields.get("fixVersions") or []:
            if not isinstance(version, dict):
                continue
            name = clean_text(version.get("name")).strip()
            if not name:
                continue
            release_dt = parse_dt(version.get("releaseDate"))
            rows.append(
                milestone_signal_row(
                    source_instance=source_instance,
                    workstream_key=workstream_key,
                    subject_kind="ticket",
                    subject_key=subject_key,
                    milestone_kind="release_target",
                    milestone_name=name,
                    target_dt=release_dt,
                    outcome_dt=resolution_dt,
                    commitment_strength="release_signal",
                    date_claim_allowed=release_dt is not None,
                    delivery_commitment_allowed=False,
                    claim_gate_reason=(
                        "source_release_target_not_owner_commitment"
                        if release_dt is not None
                        else "source_release_target_without_date"
                    ),
                    source_field="jira.fields.fixVersions.releaseDate",
                    source_payload_key=f"{subject_key}:fields.fixVersions:{name}",
                    source_url=source_url,
                    captured_at=captured_at,
                    generated_at=generated_at,
                    rank_score=75.0 if release_dt is not None else 25.0,
                )
            )
        if resolution_dt is not None:
            rows.append(
                milestone_signal_row(
                    source_instance=source_instance,
                    workstream_key=workstream_key,
                    subject_kind="ticket",
                    subject_key=subject_key,
                    milestone_kind="resolution_outcome",
                    milestone_name="Jira resolution",
                    target_dt=None,
                    outcome_dt=resolution_dt,
                    commitment_strength="outcome_evidence",
                    date_claim_allowed=True,
                    delivery_commitment_allowed=False,
                    claim_gate_reason="source_resolution_outcome_not_future_commitment",
                    source_field="jira.fields.resolutiondate",
                    source_payload_key=f"{subject_key}:fields.resolutiondate",
                    source_url=source_url,
                    captured_at=captured_at,
                    generated_at=generated_at,
                    rank_score=50.0,
                )
            )
    for object_id, payload in sorted(pr_payloads.items()):
        milestone = payload.get("milestone") if isinstance(payload.get("milestone"), dict) else None
        if not milestone:
            continue
        name = clean_text(milestone.get("title") or milestone.get("name")).strip()
        if not name:
            continue
        due_dt = parse_dt(milestone.get("due_on"))
        closed_dt = parse_dt(payload.get("merged_at")) or parse_dt(payload.get("closed_at"))
        rows.append(
            milestone_signal_row(
                source_instance=source_instance,
                workstream_key=workstream_key,
                subject_kind="pull_request",
                subject_key=str(object_id),
                milestone_kind="release_target",
                milestone_name=name,
                target_dt=due_dt,
                outcome_dt=closed_dt,
                commitment_strength="release_signal",
                date_claim_allowed=due_dt is not None,
                delivery_commitment_allowed=False,
                claim_gate_reason=(
                    "source_milestone_due_date_not_owner_commitment"
                    if due_dt is not None
                    else "source_milestone_without_due_date"
                ),
                source_field="github.milestone.due_on",
                source_payload_key=f"{object_id}:milestone:{name}",
                source_url=clean_text(payload.get("html_url")),
                captured_at=iso_or_none(parse_dt(payload.get("updated_at"))) or generated_at,
                generated_at=generated_at,
                rank_score=70.0 if due_dt is not None else 20.0,
            )
        )
    out = pd.DataFrame(rows, columns=milestone_signal_columns())
    if out.empty:
        return out
    out = out.sort_values(
        ["date_claim_allowed", "delivery_commitment_allowed", "rank_score", "subject_key", "milestone_name"],
        ascending=[False, False, False, True, True],
    )
    return out.reset_index(drop=True)


def milestone_signal_row(
    *,
    source_instance: str,
    workstream_key: str,
    subject_kind: str,
    subject_key: str,
    milestone_kind: str,
    milestone_name: str,
    target_dt: datetime | None,
    outcome_dt: datetime | None,
    commitment_strength: str,
    date_claim_allowed: bool,
    delivery_commitment_allowed: bool,
    claim_gate_reason: str,
    source_field: str,
    source_payload_key: str,
    source_url: str,
    captured_at: str,
    generated_at: str,
    rank_score: float,
) -> dict[str, Any]:
    state = milestone_signal_state(target_dt, outcome_dt, parse_dt(generated_at))
    if commitment_strength == "release_signal" and target_dt is None:
        state = "no_target_date"
    return {
        "workstream_key": workstream_key,
        "subject_kind": subject_kind,
        "subject_key": subject_key,
        "milestone_kind": milestone_kind,
        "milestone_name": milestone_name,
        "target_date": iso_or_none(target_dt),
        "outcome_date": iso_or_none(outcome_dt),
        "milestone_state": state,
        "commitment_strength": commitment_strength,
        "date_claim_allowed": int(date_claim_allowed),
        "delivery_commitment_allowed": int(delivery_commitment_allowed),
        "claim_gate_reason": claim_gate_reason,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_program_milestone",
        "external_id": "|".join([workstream_key, subject_kind, subject_key, milestone_kind, milestone_name, source_payload_key]),
        "source_field": source_field,
        "source_payload_key": source_payload_key,
        "source_url": source_url,
        "captured_at": captured_at,
        "generated_at": generated_at,
        "rank_score": rank_score,
    }


def milestone_signal_state(target_dt: datetime | None, outcome_dt: datetime | None, generated_dt: datetime | None) -> str:
    if target_dt is None and outcome_dt is None:
        return "unknown"
    if target_dt is None:
        return "outcome_only"
    if outcome_dt is not None:
        if outcome_dt.date() <= target_dt.date():
            return "resolved_before_target"
        return "resolved_after_target"
    if generated_dt is not None and generated_dt.date() > target_dt.date():
        return "past_target_unresolved"
    return "planned"


def jira_issue_url(key: str, payload: dict[str, Any]) -> str:
    if payload.get("self"):
        return clean_text(payload.get("self"))
    return f"https://issues.apache.org/jira/browse/{key}"


def build_developer_correlation(
    developer_pr_authorships: pd.DataFrame,
    developer_ticket_roles: pd.DataFrame,
    pr_forecasts: pd.DataFrame,
    ticket_features: pd.DataFrame,
) -> pd.DataFrame:
    columns = [
        "person_key",
        "display_name",
        "github_login",
        "jira_account_id",
        "jira_username",
        "jira_key",
        "identity_bridge_state",
        "identity_match_method",
        "identity_evidence_count",
        "identity_conflict_count",
        "correlation_state",
        "window_basis",
        "source_object_type_counts",
        "distinct_ticket_count_method",
        "source_coverage_state",
        "pr_authored_count",
        "open_pr_authored_count",
        "high_risk_open_pr_count",
        "associated_jira_ticket_count",
        "extra_jira_ticket_count",
        "open_extra_jira_ticket_count",
        "extra_jira_assignee_count",
        "extra_jira_reporter_count",
        "extra_jira_blocker_ticket_count",
        "same_window_ticket_pressure",
        "correlation_score",
        "confidence",
        "top_pr_subjects",
        "top_extra_ticket_keys",
        "recommended_tpm_action",
        "guardrail",
    ]
    if developer_pr_authorships.empty and developer_ticket_roles.empty:
        return pd.DataFrame(columns=columns)

    pr_rows = developer_pr_authorships.copy()
    if not pr_rows.empty:
        pr_rows["pr_number"] = pr_rows["pr_number"].map(clean_int)
        pr_rows["subject_key"] = pr_rows["repository"].astype(str) + "#" + pr_rows["pr_number"].astype(str)
        if not pr_forecasts.empty:
            forecast_columns = [
                "repository",
                "pr_number",
                "state",
                "risk_band",
                "risk_score",
                "age_days",
                "stale_days",
                "source_current_coverage_state",
            ]
            forecast_columns = [column for column in forecast_columns if column in pr_forecasts.columns]
            pr_rows = pr_rows.merge(
                pr_forecasts[forecast_columns],
                on=["repository", "pr_number"],
                how="left",
                suffixes=("", "_forecast"),
            )
        if "state_forecast" in pr_rows.columns:
            pr_rows["effective_state"] = pr_rows["state_forecast"].map(clean_text)
        else:
            pr_rows["effective_state"] = pr_rows["state"].map(clean_text)
        pr_rows["effective_state"] = pr_rows["effective_state"].where(pr_rows["effective_state"] != "", pr_rows["state"].map(clean_text))
        pr_rows["risk_band"] = pr_rows.get("risk_band", pd.Series("", index=pr_rows.index)).map(clean_text)
        pr_rows["risk_score"] = pr_rows.get("risk_score", pd.Series(0, index=pr_rows.index)).map(lambda value: float(value or 0))
    ticket_rows = developer_ticket_roles.copy()
    if not ticket_rows.empty:
        ticket_rows["ticket_key"] = ticket_rows["ticket_key"].astype(str).str.upper()
        if not ticket_features.empty:
            feature_columns = ["ticket_key", "blocker_keyword_count", "linked_pr_count", "comment_count", "participant_count"]
            feature_columns = [column for column in feature_columns if column in ticket_features.columns]
            ticket_rows = ticket_rows.merge(ticket_features[feature_columns], on="ticket_key", how="left")
        ticket_rows["normalized_status"] = ticket_rows["status"].map(normalize_ticket_state)
        ticket_rows["blocker_keyword_count"] = ticket_rows.get("blocker_keyword_count", pd.Series(0, index=ticket_rows.index)).fillna(0).map(int)

    people: dict[str, dict[str, Any]] = {}
    for frame in [pr_rows, ticket_rows]:
        if frame.empty:
            continue
        for row in frame[["person_key", "display_name", "github_login", "jira_account_id"]].drop_duplicates().itertuples(index=False):
            key = clean_text(row.person_key)
            if not key:
                continue
            existing = people.setdefault(
                key,
                {
                    "person_key": key,
                    "display_name": "",
                    "github_login": "",
                    "jira_account_id": "",
                    "jira_username": "",
                    "jira_key": "",
                },
            )
            existing["display_name"] = first_nonempty([existing["display_name"], row.display_name])
            existing["github_login"] = first_nonempty([existing["github_login"], row.github_login])
            existing["jira_account_id"] = first_nonempty([existing["jira_account_id"], row.jira_account_id])
            existing["jira_username"] = first_nonempty([existing["jira_username"], row.jira_account_id])
            existing["jira_key"] = first_nonempty([existing["jira_key"], row.jira_account_id])

    rows: list[dict[str, Any]] = []
    for person_key, person in sorted(people.items()):
        person_prs = pr_rows[pr_rows["person_key"] == person_key] if not pr_rows.empty else pd.DataFrame()
        person_tickets = ticket_rows[ticket_rows["person_key"] == person_key] if not ticket_rows.empty else pd.DataFrame()
        associated_tickets = person_tickets[person_tickets["external_kind"] == "jira_issue"] if not person_tickets.empty else pd.DataFrame()
        extra_tickets = person_tickets[person_tickets["external_kind"] == "jira_correlation_issue"] if not person_tickets.empty else pd.DataFrame()
        pr_authored_count = unique_count(person_prs, "subject_key")
        open_pr_authored_count = unique_count(person_prs[person_prs["effective_state"].map(normalize_pr_feature_state) == "open"], "subject_key") if not person_prs.empty else 0
        high_risk_open_pr_count = 0
        if not person_prs.empty:
            open_prs = person_prs[person_prs["effective_state"].map(normalize_pr_feature_state) == "open"]
            high_risk_open_pr_count = unique_count(open_prs[open_prs["risk_band"].isin(["high", "critical"])], "subject_key")
        associated_ticket_count = unique_count(associated_tickets, "ticket_key")
        extra_ticket_count = unique_count(extra_tickets, "ticket_key")
        open_extra_ticket_count = unique_count(extra_tickets[extra_tickets["normalized_status"] == "open"], "ticket_key") if not extra_tickets.empty else 0
        extra_assignee_count = unique_count(extra_tickets[extra_tickets["assignment_kind"] == "assignee"], "ticket_key") if not extra_tickets.empty else 0
        extra_reporter_count = unique_count(extra_tickets[extra_tickets["assignment_kind"] == "reporter"], "ticket_key") if not extra_tickets.empty else 0
        extra_blocker_ticket_count = unique_count(extra_tickets[extra_tickets["blocker_keyword_count"] > 0], "ticket_key") if not extra_tickets.empty else 0
        has_github = bool(clean_text(person["github_login"]))
        has_jira = bool(clean_text(person["jira_account_id"]))
        if has_github and has_jira:
            identity_bridge_state = "direct_github_jira_person"
            identity_match_method = "typed_person_github_and_jira_fields"
            identity_evidence_count = 2
        elif has_github:
            identity_bridge_state = "github_only_person"
            identity_match_method = "typed_person_github_only"
            identity_evidence_count = 1
        elif has_jira:
            identity_bridge_state = "jira_only_person"
            identity_match_method = "typed_person_jira_only"
            identity_evidence_count = 1
        else:
            identity_bridge_state = "unbridged_person"
            identity_match_method = "typed_person_without_cross_source_identity"
            identity_evidence_count = 0
        identity_conflict_count = 0
        if identity_bridge_state == "direct_github_jira_person" and pr_authored_count > 0 and extra_ticket_count > 0:
            correlation_state = "correlatable_same_identity"
            confidence = 0.7
        elif pr_authored_count > 0 and extra_ticket_count > 0:
            correlation_state = "unbridged_overlap_not_product_claim"
            confidence = 0.35
        elif pr_authored_count > 0:
            correlation_state = "pr_author_without_extra_jira_signal"
            confidence = 0.45 if has_jira else 0.25
        elif extra_ticket_count > 0:
            correlation_state = "extra_jira_actor_without_pr_signal"
            confidence = 0.45 if has_github else 0.25
        else:
            correlation_state = "associated_jira_actor_only"
            confidence = 0.25
        same_window_ticket_pressure = round((open_extra_ticket_count / max(1, pr_authored_count)), 2) if pr_authored_count else 0.0
        source_coverage_state = developer_source_coverage_state(person_prs)
        source_object_type_counts = ",".join(
            [
                f"github_pull_request:{pr_authored_count}",
                f"jira_issue:{associated_ticket_count}",
                f"jira_correlation_issue:{extra_ticket_count}",
            ]
        )
        score = developer_correlation_score(
            identity_bridge_state,
            pr_authored_count,
            open_pr_authored_count,
            high_risk_open_pr_count,
            extra_ticket_count,
            open_extra_ticket_count,
            extra_blocker_ticket_count,
        )
        rows.append(
            {
                "person_key": person_key,
                "display_name": clean_text(person["display_name"]) or person_key,
                "github_login": clean_text(person["github_login"]),
                "jira_account_id": clean_text(person["jira_account_id"]),
                "jira_username": clean_text(person["jira_username"]),
                "jira_key": clean_text(person["jira_key"]),
                "identity_bridge_state": identity_bridge_state,
                "identity_match_method": identity_match_method,
                "identity_evidence_count": identity_evidence_count,
                "identity_conflict_count": identity_conflict_count,
                "correlation_state": correlation_state,
                "window_basis": "extra_jira_same_created_window_as_pr_search",
                "source_object_type_counts": source_object_type_counts,
                "distinct_ticket_count_method": "distinct_ticket_key_by_external_kind_and_assignment_role",
                "source_coverage_state": source_coverage_state,
                "pr_authored_count": pr_authored_count,
                "open_pr_authored_count": open_pr_authored_count,
                "high_risk_open_pr_count": high_risk_open_pr_count,
                "associated_jira_ticket_count": associated_ticket_count,
                "extra_jira_ticket_count": extra_ticket_count,
                "open_extra_jira_ticket_count": open_extra_ticket_count,
                "extra_jira_assignee_count": extra_assignee_count,
                "extra_jira_reporter_count": extra_reporter_count,
                "extra_jira_blocker_ticket_count": extra_blocker_ticket_count,
                "same_window_ticket_pressure": same_window_ticket_pressure,
                "correlation_score": score,
                "confidence": confidence,
                "top_pr_subjects": top_values(person_prs, "subject_key", "risk_score"),
                "top_extra_ticket_keys": top_values(extra_tickets, "ticket_key", "blocker_keyword_count"),
                "recommended_tpm_action": developer_correlation_action(correlation_state),
                "guardrail": DEVELOPER_CORRELATION_GUARDRAIL,
            }
        )
    out = pd.DataFrame(rows, columns=columns)
    if out.empty:
        return out
    return out.sort_values(
        ["correlation_score", "high_risk_open_pr_count", "open_extra_jira_ticket_count", "pr_authored_count"],
        ascending=[False, False, False, False],
    )


def normalize_pr_feature_state(state: Any) -> str:
    value = clean_text(state).lower()
    if value in {"open", "merged", "closed"}:
        return value
    return "unknown"


def unique_count(df: pd.DataFrame, column: str) -> int:
    if df.empty or column not in df.columns:
        return 0
    return int(df[column].dropna().astype(str).replace("", pd.NA).dropna().nunique())


def top_values(df: pd.DataFrame, value_column: str, score_column: str) -> str:
    if df.empty or value_column not in df.columns:
        return ""
    rows = df.copy()
    if score_column in rows.columns:
        rows = rows.sort_values(score_column, ascending=False)
    return ",".join(rows[value_column].dropna().astype(str).drop_duplicates().head(5).tolist())


def developer_source_coverage_state(person_prs: pd.DataFrame) -> str:
    if person_prs.empty or "source_current_coverage_state" not in person_prs.columns:
        return "no_pr_authorship"
    states = {clean_text(value) for value in person_prs["source_current_coverage_state"].dropna().tolist()}
    if "detail_failed" in states:
        return "detail_failed"
    if "coverage_limited" in states:
        return "coverage_limited"
    if "observed" in states:
        return "observed"
    return "unknown"


def developer_correlation_score(
    identity_bridge_state: str,
    pr_authored_count: int,
    open_pr_authored_count: int,
    high_risk_open_pr_count: int,
    extra_ticket_count: int,
    open_extra_ticket_count: int,
    extra_blocker_ticket_count: int,
) -> float:
    score = (
        min(pr_authored_count, 25) * 1.5
        + min(open_pr_authored_count, 10) * 3.0
        + min(high_risk_open_pr_count, 5) * 9.0
        + min(extra_ticket_count, 30) * 1.0
        + min(open_extra_ticket_count, 15) * 4.0
        + min(extra_blocker_ticket_count, 10) * 3.0
    )
    if identity_bridge_state != "direct_github_jira_person":
        score *= 0.45
    return round(min(100.0, score), 2)


def developer_correlation_action(correlation_state: str) -> str:
    if correlation_state == "correlatable_same_identity":
        return "Review same-window Jira load beside PR ownership; ask whether the extra Jira work changes owner capacity, priority, or escalation path."
    if correlation_state == "unbridged_overlap_not_product_claim":
        return "Resolve identity bridge before using this as a TPM signal."
    if correlation_state == "pr_author_without_extra_jira_signal":
        return "No extra same-window Jira load found for this bridged PR author; do not claim absence unless Jira identity coverage is complete."
    if correlation_state == "extra_jira_actor_without_pr_signal":
        return "Keep as Jira workload context; do not associate to PR ownership without a direct GitHub identity bridge."
    return "Keep as background Jira participation context only."


def build_developer_correlation_validation(developer_correlation: pd.DataFrame) -> pd.DataFrame:
    columns = ["metric", "value", "sample_count", "method", "interpretation", "guardrail"]
    if developer_correlation.empty:
        return pd.DataFrame(
            [
                {
                    "metric": "direct_identity_sample_count",
                    "value": "0",
                    "sample_count": 0,
                    "method": "direct GitHub/Jira identity rows",
                    "interpretation": "No direct identity rows are available for aggregate correlation.",
                    "guardrail": DEVELOPER_CORRELATION_GUARDRAIL,
                }
            ],
            columns=columns,
        )

    rows = developer_correlation.copy()
    direct = rows[rows["identity_bridge_state"] == "direct_github_jira_person"].copy()
    direct = direct[(direct["pr_authored_count"] > 0) | (direct["extra_jira_ticket_count"] > 0)]
    correlatable = direct[(direct["pr_authored_count"] > 0) & (direct["extra_jira_ticket_count"] > 0)]
    metrics: list[dict[str, Any]] = [
        developer_validation_metric(
            "direct_identity_sample_count",
            len(direct),
            len(direct),
            "direct GitHub/Jira identity rows with PR authorship or same-window Jira activity",
            "Population available for aggregate correlation.",
        ),
        developer_validation_metric(
            "direct_identity_pr_and_extra_jira_count",
            len(correlatable),
            len(direct),
            "direct identity rows with both authored PRs and extra same-window Jira tickets",
            "Rows eligible for product-facing workload correlation leads.",
        ),
    ]
    if direct.empty:
        return pd.DataFrame(metrics, columns=columns)

    metric_pairs = [
        (
            "spearman_open_extra_jira_vs_high_risk_open_pr",
            "open_extra_jira_ticket_count",
            "high_risk_open_pr_count",
            "rank correlation over direct identity rows",
            "Positive values mean developers with more open same-window Jira tickets also tend to have more high-risk open PR leads.",
        ),
        (
            "spearman_open_extra_jira_vs_open_pr",
            "open_extra_jira_ticket_count",
            "open_pr_authored_count",
            "rank correlation over direct identity rows",
            "Positive values mean extra Jira load co-occurs with more open authored PRs.",
        ),
        (
            "spearman_ticket_pressure_vs_high_risk_open_pr",
            "same_window_ticket_pressure",
            "high_risk_open_pr_count",
            "rank correlation over direct identity rows",
            "Positive values mean higher extra-Jira-per-PR pressure co-occurs with high-risk open PR leads.",
        ),
    ]
    for metric, x_column, y_column, method, interpretation in metric_pairs:
        value, sample_count, state = rank_correlation(direct, x_column, y_column)
        metrics.append(
            developer_validation_metric(
                metric,
                value,
                sample_count,
                method,
                f"{interpretation} {state}",
            )
        )

    extra_load = direct[direct["extra_jira_ticket_count"] > 0].copy()
    if len(extra_load) >= 4 and extra_load["open_extra_jira_ticket_count"].nunique() > 1:
        threshold = float(extra_load["open_extra_jira_ticket_count"].quantile(0.75))
        high = extra_load[extra_load["open_extra_jira_ticket_count"] >= threshold]
        low = extra_load[extra_load["open_extra_jira_ticket_count"] < threshold]
        metrics.extend(
            [
                developer_validation_metric(
                    "top_quartile_open_extra_jira_threshold",
                    threshold,
                    len(extra_load),
                    "75th percentile over direct identities with extra same-window Jira tickets",
                    "Threshold used for high-load vs lower-load comparison.",
                ),
                developer_validation_metric(
                    "top_quartile_high_risk_open_pr_lift",
                    mean_difference(high, low, "high_risk_open_pr_count"),
                    len(extra_load),
                    "mean(high-risk open PR count in top extra-Jira-load quartile) minus lower-load mean",
                    "Positive values suggest extra Jira load co-occurs with more high-risk open PR leads; this is not causality.",
                ),
                developer_validation_metric(
                    "top_quartile_open_pr_lift",
                    mean_difference(high, low, "open_pr_authored_count"),
                    len(extra_load),
                    "mean(open authored PR count in top extra-Jira-load quartile) minus lower-load mean",
                    "Positive values suggest extra Jira load co-occurs with more open authored PRs; this is not causality.",
                ),
            ]
        )
    else:
        metrics.append(
            developer_validation_metric(
                "top_quartile_lift_state",
                "insufficient_variation",
                len(extra_load),
                "75th percentile comparison",
                "Not enough direct identities with varied extra Jira load to compute a stable lift.",
            )
        )

    return pd.DataFrame(metrics, columns=columns)


def developer_validation_metric(metric: str, value: Any, sample_count: int, method: str, interpretation: str) -> dict[str, Any]:
    if isinstance(value, float):
        clean_value: Any = round(value, 4)
    else:
        clean_value = value
    return {
        "metric": metric,
        "value": str(clean_value),
        "sample_count": int(sample_count),
        "method": method,
        "interpretation": interpretation,
        "guardrail": DEVELOPER_CORRELATION_GUARDRAIL,
    }


def rank_correlation(df: pd.DataFrame, x_column: str, y_column: str) -> tuple[Any, int, str]:
    if df.empty or x_column not in df.columns or y_column not in df.columns:
        return "not_available", 0, "Missing input columns."
    pairs = df[[x_column, y_column]].copy()
    pairs[x_column] = pd.to_numeric(pairs[x_column], errors="coerce")
    pairs[y_column] = pd.to_numeric(pairs[y_column], errors="coerce")
    pairs = pairs.dropna()
    sample_count = int(len(pairs))
    if sample_count < 5:
        return "insufficient_sample", sample_count, "Sample is too small for aggregate validation."
    if pairs[x_column].nunique() < 2 or pairs[y_column].nunique() < 2:
        return "insufficient_variation", sample_count, "One side has insufficient variation."
    value = float(pairs[x_column].rank(method="average").corr(pairs[y_column].rank(method="average"), method="pearson"))
    if math.isnan(value):
        return "not_available", sample_count, "Correlation could not be computed."
    return round(value, 4), sample_count, "Treat magnitude as a workload co-occurrence signal only."


def mean_difference(high: pd.DataFrame, low: pd.DataFrame, column: str) -> Any:
    if high.empty or low.empty or column not in high.columns or column not in low.columns:
        return "not_available"
    high_values = pd.to_numeric(high[column], errors="coerce").dropna()
    low_values = pd.to_numeric(low[column], errors="coerce").dropna()
    if high_values.empty or low_values.empty:
        return "not_available"
    return round(float(high_values.mean() - low_values.mean()), 4)


def build_feature_provenance() -> pd.DataFrame:
    return pd.DataFrame(
        [
            {
                "feature_table": "tpm_pr_features",
                "field_group": "identity_text_state",
                "source_layer": "typed_pull_requests",
                "notes": "Repository, number, title, state, source URL, and search text are read from typed PullRequest rows.",
            },
            {
                "feature_table": "tpm_pr_features",
                "field_group": "lifecycle",
                "source_layer": "typed_pull_requests",
                "notes": "PR source_created_at, source_updated_at, closed_at, and merged_at drive age and cycle features when present.",
            },
            {
                "feature_table": "tpm_pr_features",
                "field_group": "reviews",
                "source_layer": "typed_pull_request_reviews",
                "notes": "Requested reviewer counts and evidence come from typed PullRequestReview rows.",
            },
            {
                "feature_table": "tpm_pr_features",
                "field_group": "source_text_issue_keys",
                "source_layer": "typed_pull_request_search_text",
                "notes": "Issue-key text counts are extracted from PR title/body search text and treated as source text features, not Jira outcome labels.",
            },
            {
                "feature_table": "tpm_pr_features",
                "field_group": "author_history_graph",
                "source_layer": "typed_pull_request_author_history",
                "notes": "Author prior PR, prior merged PR, median prior cycle, and concurrent open PR counts are computed only from PRs visible before the feature as-of timestamp.",
            },
            {
                "feature_table": "tpm_pr_feature_snapshots",
                "field_group": "source_event_replay_graph_features",
                "source_layer": "github_event_replay_plus_typed_author_history",
                "notes": "Pre-terminal source-event snapshots keep dynamic comment/review/commit counts as of the event, omit retrospectively discovered ticket/text-link counts, and recompute author-history graph features at that event time.",
            },
            {
                "feature_table": "tpm_pr_features",
                "field_group": "churn_counts",
                "source_layer": "typed_pull_requests",
                "notes": "Additions, deletions, changed file counts, commits, comment counters, and draft state are read from typed PullRequest fields; missing values stay null and are tracked with missingness indicators.",
            },
            {
                "feature_table": "tpm_pr_features",
                "field_group": "mergeability",
                "source_layer": "typed_pull_requests_nullable",
                "notes": "Mergeable state is read from nullable typed PullRequest.is_mergeable; GitHub null or missing values remain unknown, not false.",
            },
            {
                "feature_table": "tpm_pr_features",
                "field_group": "current_source_coverage",
                "source_layer": "source_sync_issues",
                "notes": "Latest fixture-run PR bundle failures are retained as coverage limits; detail failures suppress current-state forecast claims and produce refresh-source follow-up.",
            },
            {
                "feature_table": "tpm_ticket_features",
                "field_group": "identity_text_state",
                "source_layer": "typed_tickets",
                "notes": "Ticket key, title, body, status, priority, source update time, and search text are read from typed Ticket rows.",
            },
            {
                "feature_table": "tpm_ticket_features",
                "field_group": "people_and_comments",
                "source_layer": "typed_ticket_assignments_messages",
                "notes": "Assignee/reporter and comment/participant counts come from typed assignment and message relationships.",
            },
            {
                "feature_table": "tpm_developer_correlation",
                "field_group": "direct_identity_workload_overlap",
                "source_layer": "typed_people_pr_authorships_ticket_assignments",
                "notes": "Same-window PR/Jira workload correlation uses only Person rows with direct GitHub and Jira identity bridges for product-facing correlation leads; unbridged rows remain guardrailed context.",
            },
            {
                "feature_table": "tpm_blocker_candidates",
                "field_group": "source_text_spans",
                "source_layer": "replay_payload_text",
                "notes": "Blocker span extraction still reads replay text so excerpts can retain source locators; candidates persist Evidence-backed WorkInsight rows.",
            },
        ]
    )


def build_blocker_candidates(
    jira_payloads: dict[str, dict[str, Any]],
    pr_payloads: dict[str, dict[str, Any]],
    ticket_pr_edges: pd.DataFrame,
) -> pd.DataFrame:
    rows: list[dict[str, Any]] = []
    for key, payload in jira_payloads.items():
        fields = payload.get("fields") or {}
        status = ((fields.get("status") or {}).get("name") or "unknown")
        comments = (((fields.get("comment") or {}).get("comments")) or [])
        for idx, comment in enumerate(comments):
            body = comment.get("body") or ""
            match = first_blocker_match(body)
            if not match:
                continue
            comment_id = str(comment.get("id") or idx)
            issue_locator = payload.get("self") or ""
            comment_locator = comment.get("self") or issue_locator
            rows.append(
                {
                    "candidate_kind": "jira_comment",
                    "product_key": key,
                    "source_url": comment_locator,
                    "actor": display_name(comment.get("author") or {}),
                    "signal": match.group(0).lower(),
                    "severity": blocker_severity(body),
                    "subject_state": normalize_ticket_state(status),
                    "candidate_scope": "current" if is_current_ticket_status(status) else "historical",
                    "evidence_excerpt": snippet(body, match.start()),
                    "evidence_source_system": "jira",
                    "evidence_source_instance": "apache-jira",
                    "evidence_external_kind": "jira_issue",
                    "evidence_external_id": key,
                    "evidence_source_url": comment_locator,
                    "evidence_locator_kind": "jira_comment",
                    "evidence_locator": comment_locator,
                    "evidence_source_span_key": f"jira_comment:{key}:{comment_id}:{match.start()}:{match.end()}",
                    "evidence_span_start": int(match.start()),
                    "evidence_span_end": int(match.end()),
                    "evidence_excerpt_truncated": True,
                    "created_at": comment.get("created") or "",
                }
            )
    for object_id, payload in pr_payloads.items():
        repo, _ = object_id.rsplit("#", 1)
        title = payload.get("title") or ""
        body = payload.get("body") or ""
        title_match = first_blocker_match(title)
        body_match = first_blocker_match(body)
        if title_match:
            text = title
            match = title_match
            locator_kind = "github_pull_title"
        elif body_match:
            text = body
            match = body_match
            locator_kind = "github_pull_body"
        else:
            continue
        api_url = payload.get("url") or payload.get("html_url") or ""
        rows.append(
            {
                "candidate_kind": "pull_request_text",
                "product_key": object_id,
                "source_url": payload.get("html_url") or "",
                "actor": ((payload.get("user") or {}).get("login") or ""),
                "signal": match.group(0).lower(),
                "severity": blocker_severity(text),
                "subject_state": normalize_pr_state(payload),
                "candidate_scope": "current" if normalize_pr_state(payload) == "open" else "historical",
                "evidence_excerpt": snippet(text, match.start()),
                "evidence_source_system": "github",
                "evidence_source_instance": f"github.com/{repo}",
                "evidence_external_kind": "github_pull_request",
                "evidence_external_id": object_id,
                "evidence_source_url": api_url,
                "evidence_locator_kind": locator_kind,
                "evidence_locator": api_url,
                "evidence_source_span_key": stable_digest([locator_kind, object_id, match.start(), match.end(), match.group(0)]),
                "evidence_span_start": int(match.start()),
                "evidence_span_end": int(match.end()),
                "evidence_excerpt_truncated": True,
                "created_at": payload.get("created_at") or "",
            }
        )
    df = pd.DataFrame(rows)
    if df.empty:
        return pd.DataFrame(
            columns=[
                "candidate_kind",
                "product_key",
                "source_url",
                "actor",
                "signal",
                "severity",
                "subject_state",
                "candidate_scope",
                "evidence_excerpt",
                "evidence_source_system",
                "evidence_source_instance",
                "evidence_external_kind",
                "evidence_external_id",
                "evidence_source_url",
                "evidence_locator_kind",
                "evidence_locator",
                "evidence_source_span_key",
                "evidence_span_start",
                "evidence_span_end",
                "evidence_excerpt_truncated",
                "created_at",
            ]
        )
    return df.sort_values(["candidate_scope", "severity", "created_at"], ascending=[True, False, False])


def build_forecasts(
    pr_features: pd.DataFrame,
    *,
    temporal_feature_snapshot_ready: bool = False,
    event_pr_feature_snapshots: pd.DataFrame | None = None,
) -> tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    if pr_features.empty:
        return pd.DataFrame(), pd.DataFrame(), pd.DataFrame(), pd.DataFrame()
    feature_cols = forecast_feature_columns()
    merged = pr_features[(pr_features["state"] == "merged") & pr_features["cycle_time_days"].notna()].copy()
    closed_unmerged = pr_features[(pr_features["state"] == "closed") & pr_features["cycle_time_days"].notna()].copy()
    open_prs = pr_features[pr_features["state"] == "open"].copy()
    median_cycle = float(merged["cycle_time_days"].median()) if not merged.empty else 14.0
    p75_cycle = float(merged["cycle_time_days"].quantile(0.75)) if len(merged) >= 2 else median_cycle * 1.5
    forecast_backtest = build_forecast_backtest(merged, feature_cols, temporal_feature_snapshot_ready=temporal_feature_snapshot_ready)
    source_event_as_of_backtest = build_source_event_as_of_forecast_backtest(event_pr_feature_snapshots, feature_cols)
    lifecycle_as_of_backtest = build_lifecycle_as_of_forecast_backtest(merged)
    survival_backtest = build_survival_time_to_merge_backtest(pr_features)
    forecast_backtest = concat_dataframes_preserving_columns(
        [forecast_backtest, source_event_as_of_backtest, lifecycle_as_of_backtest, survival_backtest]
    )
    forecast_risk_backtest = build_forecast_risk_backtest(merged)
    backtest_metrics = forecast_backtest_metrics(forecast_backtest)
    source_event_metrics = source_event_as_of_backtest_metrics(source_event_as_of_backtest, event_pr_feature_snapshots)
    lifecycle_metrics = lifecycle_as_of_backtest_metrics(lifecycle_as_of_backtest, merged)
    survival_metrics = survival_time_to_merge_metrics(survival_backtest, pr_features)
    risk_backtest_metrics = metric_map(forecast_risk_backtest)
    model_method = "heuristic_percentile_backtest_limited"
    cv_mae = backtest_metrics.get("random_forest_regressor_kfold_mae")
    gb_mae = backtest_metrics.get("gradient_boosting_absolute_error_kfold_mae")
    hist_gb_mae = backtest_metrics.get("hist_gradient_boosting_absolute_error_kfold_mae")
    heuristic_cv_mae = backtest_metrics.get("heuristic_percentile_kfold_mae")
    median_cv_mae = backtest_metrics.get("median_cycle_baseline_kfold_mae")
    author_cv_mae = backtest_metrics.get("author_history_median_cycle_kfold_mae")
    baseline_mae = None
    heuristic_predicted = heuristic_cycle_prediction(pr_features, median_cycle, p75_cycle)
    predicted_total = heuristic_predicted
    ready_model = forecast_eta_ready_model(backtest_metrics)
    if len(merged) >= 1:
        baseline_mae = float(mean_absolute_error(merged["cycle_time_days"].astype(float), [median_cycle] * len(merged)))
    if ready_model == "author_history_median_cycle":
        predicted_total = author_history_cycle_prediction(pr_features, heuristic_predicted)
        model_method = "author_history_median_cycle"
    elif len(merged) >= 20 and median_cv_mae is not None:
        if ready_model == "gradient_boosting_absolute_error":
            model = GradientBoostingRegressor(
                loss="absolute_error",
                random_state=42,
                n_estimators=120,
                max_depth=2,
                learning_rate=0.05,
                min_samples_leaf=10,
            )
            model.fit(merged[feature_cols].fillna(0), merged["cycle_time_days"].astype(float))
            predicted_total = model.predict(pr_features[feature_cols].fillna(0))
            model_method = "gradient_boosting_absolute_error"
        elif ready_model == "hist_gradient_boosting_absolute_error":
            model = HistGradientBoostingRegressor(
                loss="absolute_error",
                random_state=42,
                max_leaf_nodes=15,
                learning_rate=0.04,
                min_samples_leaf=20,
                max_iter=180,
            )
            model.fit(merged[feature_cols].fillna(0), merged["cycle_time_days"].astype(float))
            predicted_total = model.predict(pr_features[feature_cols].fillna(0))
            model_method = "hist_gradient_boosting_absolute_error"
        elif ready_model == "random_forest_regressor":
            model = RandomForestRegressor(n_estimators=200, random_state=42, min_samples_leaf=2)
            x = merged[feature_cols].fillna(0)
            y = merged["cycle_time_days"].astype(float)
            model.fit(x, y)
            predicted_total = model.predict(pr_features[feature_cols].fillna(0))
            model_method = "random_forest_regressor"
        else:
            model_method = "heuristic_percentile_ml_rejected"
    forecasts = pr_features.copy()
    forecasts["predicted_total_cycle_days"] = [round(float(v), 2) for v in predicted_total]
    forecasts["predicted_remaining_days"] = forecasts.apply(
        lambda row: max(0.0, row["predicted_total_cycle_days"] - (row["age_days"] or 0.0)) if pd.isna(row["cycle_time_days"]) else 0.0,
        axis=1,
    )
    forecasts["overdue_days"] = forecasts.apply(
        lambda row: max(0.0, (row["age_days"] or 0.0) - row["predicted_total_cycle_days"]) if pd.isna(row["cycle_time_days"]) else 0.0,
        axis=1,
    )
    forecasts["risk_score"] = forecasts.apply(lambda row: risk_score(row, median_cycle, p75_cycle), axis=1)
    forecasts["risk_band"] = forecasts["risk_score"].apply(risk_band)
    forecasts["forecast_method"] = model_method
    summary = pd.DataFrame(
        [
            {"metric": "merged_pr_count", "value": str(len(merged)), "note": "delivery baseline sample for time-to-merge risk"},
            {"metric": "closed_unmerged_pr_count", "value": str(len(closed_unmerged)), "note": "excluded from delivery baseline; treated as abandonment/closure signal"},
            {"metric": "open_pr_count", "value": str(len(open_prs)), "note": "current risk triage candidates"},
            {"metric": "median_merged_cycle_days", "value": f"{median_cycle:.2f}", "note": "median time to merge for merged PRs"},
            {"metric": "p75_merged_cycle_days", "value": f"{p75_cycle:.2f}", "note": "slow-merge threshold for merged PRs"},
            {"metric": "avg_closed_unmerged_cycle_days", "value": "" if closed_unmerged.empty else f"{float(closed_unmerged['cycle_time_days'].mean()):.2f}", "note": "historical closure/abandonment age, not delivery cycle"},
            {"metric": "forecast_method", "value": model_method, "note": "risk method chosen from available sample size"},
            {"metric": "forecast_feature_set", "value": ",".join(feature_cols), "note": "allowlisted pre-label source features used by the ETA/risk backtest"},
            {"metric": "forecast_feature_leakage_guard", "value": "passed", "note": "feature set excludes labels, action decisions, model outputs, and future lifecycle fields"},
            {"metric": "forecast_calendar_feature_guard", "value": "passed", "note": "production ETA feature set excludes created-time, calendar, month, and quarter cohort proxies"},
            {"metric": "median_baseline_mae_days", "value": "" if baseline_mae is None else f"{baseline_mae:.2f}", "note": "naive merged-PR baseline error"},
            {"metric": "cross_validated_mae_days", "value": "" if cv_mae is None else f"{cv_mae:.2f}", "note": "only populated for evaluated ML model"},
            {"metric": "backtest_best_model", "value": backtest_metrics.get("best_kfold_model", ""), "note": "lowest K-fold MAE row among evaluated models and baselines"},
            {"metric": "backtest_best_chronological_model", "value": backtest_metrics.get("best_chronological_holdout_model", ""), "note": "lowest chronological-holdout MAE row among evaluated models and baselines"},
            {"metric": "backtest_median_mae_days", "value": format_optional_float(median_cv_mae), "note": "K-fold MAE for median-cycle baseline"},
            {"metric": "backtest_heuristic_mae_days", "value": format_optional_float(heuristic_cv_mae), "note": "K-fold MAE for current heuristic cycle estimate"},
            {"metric": "backtest_author_history_mae_days", "value": format_optional_float(author_cv_mae), "note": "K-fold MAE for source-graph author-history median cycle baseline"},
            {"metric": "backtest_gradient_boosting_mae_days", "value": format_optional_float(gb_mae), "note": "K-fold MAE for absolute-error gradient boosting ETA model"},
            {"metric": "backtest_hist_gradient_boosting_mae_days", "value": format_optional_float(hist_gb_mae), "note": "K-fold MAE for absolute-error histogram gradient boosting ETA candidate"},
            {"metric": "backtest_random_forest_mae_days", "value": format_optional_float(cv_mae), "note": "K-fold MAE for random forest ETA model"},
            {"metric": "source_event_as_of_backtest_state", "value": source_event_metrics.get("state", "missing"), "note": "whether source-event replay snapshots can evaluate ETA features without current-row feature leakage"},
            {"metric": "source_event_as_of_subject_count", "value": source_event_metrics.get("subject_count", "0"), "note": "merged PR subjects represented by source-event replay snapshots"},
            {"metric": "source_event_as_of_training_example_count", "value": source_event_metrics.get("training_example_count", "0"), "note": "pre-terminal source-event replay snapshots used for as-of ETA backtesting"},
            {"metric": "source_event_as_of_ready_model", "value": source_event_metrics.get("ready_model", ""), "note": "candidate model that cleared grouped and chronological source-event as-of gates, if any"},
            {"metric": "source_event_as_of_median_mae_days", "value": source_event_metrics.get("median_cycle_baseline_kfold_mae", ""), "note": "Grouped source-event snapshot K-fold MAE for median-cycle baseline"},
            {"metric": "source_event_as_of_author_history_mae_days", "value": source_event_metrics.get("author_history_median_cycle_kfold_mae", ""), "note": "Grouped source-event snapshot K-fold MAE for author-history median cycle baseline"},
            {"metric": "source_event_as_of_gradient_boosting_mae_days", "value": source_event_metrics.get("gradient_boosting_absolute_error_kfold_mae", ""), "note": "Grouped source-event snapshot K-fold MAE for absolute-error gradient boosting ETA model"},
            {"metric": "source_event_as_of_hist_gradient_boosting_mae_days", "value": source_event_metrics.get("hist_gradient_boosting_absolute_error_kfold_mae", ""), "note": "Grouped source-event snapshot K-fold MAE for absolute-error histogram gradient boosting ETA candidate"},
            {"metric": "source_event_as_of_random_forest_mae_days", "value": source_event_metrics.get("random_forest_regressor_kfold_mae", ""), "note": "Grouped source-event snapshot K-fold MAE for random forest ETA model"},
            {"metric": "lifecycle_as_of_backtest_state", "value": lifecycle_metrics.get("state", "missing"), "note": "whether lifecycle-derived synthetic as-of checkpoints can benchmark simple remaining-time baselines"},
            {"metric": "lifecycle_as_of_terminal_subject_count", "value": lifecycle_metrics.get("terminal_subject_count", "0"), "note": "merged PRs with lifecycle timestamps available for lifecycle-as-of baseline evaluation"},
            {"metric": "lifecycle_as_of_training_example_count", "value": lifecycle_metrics.get("training_example_count", "0"), "note": "synthetic pre-terminal lifecycle checkpoints used to benchmark remaining-time baselines; these are not live source snapshots"},
            {"metric": "lifecycle_as_of_best_model", "value": lifecycle_metrics.get("best_model", ""), "note": "lowest-MAE lifecycle-as-of baseline; informational only and not an ETA readiness gate"},
            {"metric": "lifecycle_as_of_best_mae_days", "value": lifecycle_metrics.get("best_mae_days", ""), "note": "MAE for the best lifecycle-as-of baseline over held-out PR subjects"},
            {"metric": "lifecycle_as_of_age_bucket_mae_days", "value": lifecycle_metrics.get("age_bucket_median_remaining_mae_days", ""), "note": "held-out MAE for age-bucket median remaining-time baseline"},
            {"metric": "survival_time_to_merge_state", "value": survival_metrics.get("state", "missing"), "note": "whether censored time-to-merge survival baselines are available for risk modeling"},
            {"metric": "survival_time_to_merge_subject_count", "value": survival_metrics.get("subject_count", "0"), "note": "PR subjects with observed duration or censoring time for survival analysis"},
            {"metric": "survival_time_to_merge_event_subject_count", "value": survival_metrics.get("event_subject_count", "0"), "note": "merged PR subjects treated as observed delivery events"},
            {"metric": "survival_time_to_merge_censored_subject_count", "value": survival_metrics.get("censored_subject_count", "0"), "note": "open or closed-unmerged PR subjects treated as right-censored for delivery survival analysis"},
            {"metric": "survival_time_to_merge_open_censored_subject_count", "value": survival_metrics.get("open_censored_subject_count", "0"), "note": "currently open PRs contributing censored duration context, not resolved outcomes"},
            {"metric": "survival_time_to_merge_censoring_rate", "value": survival_metrics.get("censoring_rate", ""), "note": "share of survival subjects without a merge event"},
            {"metric": "survival_time_to_merge_backtest_example_count", "value": survival_metrics.get("backtest_example_count", "0"), "note": "held-out pre-terminal examples used to evaluate survival remaining-time baselines"},
            {"metric": "survival_time_to_merge_best_model", "value": survival_metrics.get("best_model", ""), "note": "lowest-MAE survival remaining-time baseline; informational only and not an ETA readiness gate"},
            {"metric": "survival_time_to_merge_best_mae_days", "value": survival_metrics.get("best_mae_days", ""), "note": "MAE for the best censored survival remaining-time baseline over held-out merged PR subjects"},
            {"metric": "risk_triage_precision_at_10pct", "value": risk_backtest_metrics.get("precision_at_10pct", ""), "note": "slow-cycle precision in highest static risk decile; triage validation only"},
            {"metric": "risk_triage_lift_at_10pct", "value": risk_backtest_metrics.get("lift_vs_baseline_at_10pct", ""), "note": "precision lift above historical slow-cycle base rate; triage validation only"},
            {"metric": "risk_triage_coverage_stratified_state", "value": risk_backtest_metrics.get("coverage_stratified_backtest_state", ""), "note": "whether slow-cycle risk lift could be tested within current source coverage/provenance strata"},
            {"metric": "risk_triage_coverage_stratum_count", "value": risk_backtest_metrics.get("coverage_stratum_count", ""), "note": "distinct source coverage/provenance strata in the historical merged risk backtest"},
            {"metric": "risk_triage_coverage_stratified_max_lift_at_10pct", "value": risk_backtest_metrics.get("coverage_stratified_max_lift_at_10pct", ""), "note": "maximum within-stratum slow-cycle lift after holding source coverage/provenance constant"},
            {"metric": "risk_triage_coverage_stratified_weighted_lift_at_10pct", "value": risk_backtest_metrics.get("coverage_stratified_weighted_lift_at_10pct", ""), "note": "sample-weighted within-stratum slow-cycle lift after holding source coverage/provenance constant"},
            {"metric": "eta_forecast_ready", "value": "true" if forecast_eta_ready(backtest_metrics, temporal_feature_snapshot_ready=temporal_feature_snapshot_ready) else "false", "note": "true only when the learned model beats baselines and repeated as-of feature snapshots are available"},
            *forecast_readiness_diagnostic_rows(backtest_metrics, len(merged), temporal_feature_snapshot_ready=temporal_feature_snapshot_ready),
        ]
    )
    return summary, forecasts, forecast_backtest, forecast_risk_backtest


def concat_dataframes_preserving_columns(frames: list[pd.DataFrame], columns: list[str] | None = None) -> pd.DataFrame:
    nonempty = [frame for frame in frames if frame is not None and not frame.empty]
    if columns is None:
        target_columns: list[str] = []
        for frame in nonempty:
            for column in frame.columns:
                if column not in target_columns:
                    target_columns.append(column)
    else:
        target_columns = list(columns)
    if not nonempty:
        return pd.DataFrame(columns=target_columns)
    target_dtypes = dataframe_column_dtypes(nonempty, target_columns)
    trimmed_frames: list[pd.DataFrame] = []
    for frame in nonempty:
        reindexed = frame.reindex(columns=target_columns)
        populated_columns = [column for column in target_columns if reindexed[column].notna().any()]
        if populated_columns:
            trimmed_frames.append(reindexed.loc[:, populated_columns])
        else:
            trimmed_frames.append(pd.DataFrame({"_row_present": [True] * len(reindexed)}, index=reindexed.index))
    merged = pd.concat(trimmed_frames, ignore_index=True).drop(columns=["_row_present"], errors="ignore").reindex(columns=target_columns)
    return restore_dataframe_dtypes(merged, target_dtypes)


def dataframe_column_dtypes(frames: list[pd.DataFrame], columns: list[str]) -> dict[str, Any]:
    dtypes: dict[str, Any] = {}
    for column in columns:
        for frame in frames:
            if column not in frame.columns:
                continue
            dtypes[column] = frame[column].dtype
            if frame[column].notna().any():
                break
    return dtypes


def restore_dataframe_dtypes(df: pd.DataFrame, dtypes: dict[str, Any]) -> pd.DataFrame:
    for column, dtype in dtypes.items():
        if column not in df.columns:
            continue
        try:
            df[column] = df[column].astype(dtype)
        except (TypeError, ValueError):
            continue
    return df


def forecast_feature_columns() -> list[str]:
    assert_forecast_feature_columns(FORECAST_FEATURE_COLUMNS)
    return list(FORECAST_FEATURE_COLUMNS)


def assert_forecast_feature_columns(feature_cols: list[str]) -> None:
    leaked = [column for column in feature_cols if forecast_feature_is_forbidden(column)]
    if leaked:
        raise ValueError(f"forecast feature leakage guard rejected columns: {', '.join(sorted(leaked))}")


def forecast_feature_is_forbidden(column: str) -> bool:
    name = clean_text(column).lower()
    if name in FORECAST_FORBIDDEN_FEATURE_COLUMNS:
        return True
    return any(pattern in name for pattern in FORECAST_FORBIDDEN_FEATURE_SUBSTRINGS)


def build_forecast_feature_set_readiness_matrix(
    pr_features: pd.DataFrame,
    *,
    temporal_feature_snapshot_ready: bool = False,
) -> pd.DataFrame:
    columns = [
        "feature_set_key",
        "feature_policy",
        "model",
        "kfold_mae_days",
        "kfold_improvement_pct",
        "chronological_mae_days",
        "chronological_improvement_pct",
        "same_model_backtest_gate",
        "as_of_snapshot_gate",
        "eta_promotable",
        "guardrail_state",
        "feature_count",
        "note",
    ]
    if pr_features.empty:
        return pd.DataFrame(columns=columns)
    merged = pr_features[(pr_features["state"] == "merged") & pr_features["cycle_time_days"].notna()].copy()
    if len(merged) < 10:
        return pd.DataFrame(
            [
                {
                    "feature_set_key": "production_source_safe",
                    "feature_policy": "production_eta",
                    "model": "none",
                    "kfold_mae_days": "",
                    "kfold_improvement_pct": "",
                    "chronological_mae_days": "",
                    "chronological_improvement_pct": "",
                    "same_model_backtest_gate": "gated",
                    "as_of_snapshot_gate": "passed" if temporal_feature_snapshot_ready else "gated",
                    "eta_promotable": "false",
                    "guardrail_state": "insufficient_sample",
                    "feature_count": str(len(FORECAST_FEATURE_COLUMNS)),
                    "note": "Need at least 10 merged PRs before comparing ETA feature sets.",
                }
            ],
            columns=columns,
        )

    rows: list[dict[str, Any]] = []
    for variant in forecast_feature_set_variants():
        feature_set_key = str(variant["feature_set_key"])
        feature_policy = str(variant["feature_policy"])
        include_derived = bool(variant.get("include_derived", False))
        include_calendar = bool(variant.get("include_calendar", False))
        guardrail_state = str(variant["guardrail_state"])
        variant_features = forecast_feature_variant_columns(include_derived=include_derived, include_calendar=include_calendar)
        variant_data = add_forecast_feature_set_probe_features(merged, include_calendar=include_calendar)
        if not include_calendar:
            assert_forecast_feature_columns(variant_features)
        backtest = build_forecast_backtest(variant_data, variant_features, temporal_feature_snapshot_ready=False)
        metrics = forecast_backtest_metrics(backtest)
        model_names = forecast_backtest_model_names(metrics)
        if not model_names:
            rows.append(
                {
                    "feature_set_key": feature_set_key,
                    "feature_policy": feature_policy,
                    "model": "none",
                    "kfold_mae_days": "",
                    "kfold_improvement_pct": "",
                    "chronological_mae_days": "",
                    "chronological_improvement_pct": "",
                    "same_model_backtest_gate": "gated",
                    "as_of_snapshot_gate": "passed" if temporal_feature_snapshot_ready else "gated",
                    "eta_promotable": "false",
                    "guardrail_state": guardrail_state,
                    "feature_count": str(len(variant_features)),
                    "note": "No comparable K-fold and chronological feature-set metrics were produced.",
                }
            )
            continue
        for model_name in model_names:
            same_model_gate = (
                model_name in ETA_READY_MODEL_CANDIDATES
                and forecast_eta_model_candidate_ready(metrics, model_name)
            )
            calendar_quarantined = guardrail_state == "quarantined_calendar_cohort"
            eta_promotable = same_model_gate and temporal_feature_snapshot_ready and not calendar_quarantined
            rows.append(
                {
                    "feature_set_key": feature_set_key,
                    "feature_policy": feature_policy,
                    "model": model_name,
                    "kfold_mae_days": format_optional_float(metrics.get(f"{model_name}_kfold_mae")),
                    "kfold_improvement_pct": format_optional_float(forecast_eta_candidate_improvement(metrics, model_name, "kfold")),
                    "chronological_mae_days": format_optional_float(metrics.get(f"{model_name}_chronological_holdout_mae")),
                    "chronological_improvement_pct": format_optional_float(
                        forecast_eta_candidate_improvement(metrics, model_name, "chronological_holdout")
                    ),
                    "same_model_backtest_gate": "passed" if same_model_gate else "gated",
                    "as_of_snapshot_gate": "passed" if temporal_feature_snapshot_ready else "gated",
                    "eta_promotable": "true" if eta_promotable else "false",
                    "guardrail_state": guardrail_state,
                    "feature_count": str(len(variant_features)),
                    "note": forecast_feature_set_readiness_note(feature_set_key, model_name, same_model_gate, calendar_quarantined),
                }
            )
    return pd.DataFrame(rows, columns=columns)


def forecast_feature_set_variants() -> list[dict[str, Any]]:
    return [
        {
            "feature_set_key": "production_source_safe",
            "feature_policy": "production_eta",
            "include_derived": False,
            "include_calendar": False,
            "guardrail_state": "production_allowlist",
        },
        {
            "feature_set_key": "source_safe_derived_no_calendar",
            "feature_policy": "diagnostic_candidate",
            "include_derived": True,
            "include_calendar": False,
            "guardrail_state": "source_safe_no_calendar",
        },
        {
            "feature_set_key": "created_time_probe_quarantined",
            "feature_policy": "diagnostic_only",
            "include_derived": True,
            "include_calendar": True,
            "guardrail_state": "quarantined_calendar_cohort",
        },
    ]


def forecast_feature_variant_columns(*, include_derived: bool, include_calendar: bool) -> list[str]:
    columns = list(FORECAST_FEATURE_COLUMNS)
    if include_derived:
        columns.extend(SOURCE_SAFE_DERIVED_FORECAST_FEATURE_COLUMNS)
    if include_calendar:
        columns.extend(CALENDAR_PROBE_FORECAST_FEATURE_COLUMNS)
    return columns


def add_forecast_feature_set_probe_features(df: pd.DataFrame, *, include_calendar: bool = False) -> pd.DataFrame:
    out = df.copy()
    for column in [
        "additions",
        "deletions",
        "total_lines_changed",
        "changed_files",
        "commits",
        "comments",
        "review_comments",
        "linked_ticket_count",
        "requested_reviewer_count",
        "issue_key_text_count",
        "author_prior_pr_count",
        "author_prior_merged_pr_count",
        "author_open_pr_count",
        "author_prior_median_cycle_days",
    ]:
        values = numeric_series(out, column).clip(lower=0)
        out[f"log1p_{column}"] = values.map(math.log1p)
    missing_columns = [column for column in FORECAST_FEATURE_COLUMNS if column.endswith("_missing")]
    out["missing_feature_count"] = sum((numeric_series(out, column) for column in missing_columns), start=pd.Series([0.0] * len(out), index=out.index))
    out["author_merge_rate"] = numeric_series(out, "author_prior_merged_pr_count") / (numeric_series(out, "author_prior_pr_count") + 1.0)
    out["churn_per_file"] = numeric_series(out, "total_lines_changed") / (numeric_series(out, "changed_files") + 1.0)
    out["churn_per_commit"] = numeric_series(out, "total_lines_changed") / (numeric_series(out, "commits") + 1.0)
    out["comments_per_commit"] = (numeric_series(out, "comments") + numeric_series(out, "review_comments")) / (numeric_series(out, "commits") + 1.0)
    out["has_requested_reviewer"] = (numeric_series(out, "requested_reviewer_count") > 0).astype(int)
    out["has_linked_ticket"] = (numeric_series(out, "linked_ticket_count") > 0).astype(int)
    out["has_issue_key_text"] = (numeric_series(out, "issue_key_text_count") > 0).astype(int)
    if include_calendar:
        created = out.get("created_at", pd.Series([""] * len(out), index=out.index)).map(parse_dt)
        out["created_month_index"] = created.map(lambda value: (value.year * 12 + value.month) if value is not None else 0)
        out["created_quarter"] = created.map(lambda value: ((value.month - 1) // 3 + 1) if value is not None else 0)
    return out


def numeric_series(df: pd.DataFrame, column: str) -> pd.Series:
    if column not in df.columns:
        return pd.Series([0.0] * len(df), index=df.index)
    return pd.to_numeric(df[column], errors="coerce").fillna(0.0)


def forecast_backtest_model_names(metrics: dict[str, Any]) -> list[str]:
    names = {
        key[: -len("_kfold_mae")]
        for key in metrics
        if str(key).endswith("_kfold_mae")
    }
    names.update(
        key[: -len("_chronological_holdout_mae")]
        for key in metrics
        if str(key).endswith("_chronological_holdout_mae")
    )
    order = [
        "median_cycle_baseline",
        "heuristic_percentile",
        "author_history_median_cycle",
        "gradient_boosting_absolute_error",
        "hist_gradient_boosting_absolute_error",
        "random_forest_regressor",
    ]
    return [name for name in order if name in names] + sorted(names.difference(order))


def forecast_feature_set_readiness_note(feature_set_key: str, model_name: str, same_model_gate: bool, calendar_quarantined: bool) -> str:
    if calendar_quarantined:
        return (
            f"{feature_set_key}/{model_name} is a diagnostic calendar-cohort probe only; "
            "created-time, month, and quarter features cannot promote ETA readiness."
        )
    if same_model_gate:
        return (
            f"{feature_set_key}/{model_name} cleared same-model backtest thresholds; "
            "ETA still also requires as-of snapshot and promotion review gates."
        )
    return (
        f"{feature_set_key}/{model_name} did not clear the same-model ETA backtest threshold; "
        "keep forecasts as risk triage."
    )


def build_forecast_backtest(merged: pd.DataFrame, feature_cols: list[str], *, temporal_feature_snapshot_ready: bool = False) -> pd.DataFrame:
    columns = [
        "evaluation",
        "model",
        "fold",
        "train_count",
        "test_count",
        "mae_days",
        "median_error_days",
        "p75_error_days",
        "max_error_days",
        "improvement_vs_median_pct",
        "ready_for_eta",
        "note",
    ]
    if len(merged) < 10:
        return pd.DataFrame(
            [
                {
                    "evaluation": "insufficient_sample",
                    "model": "none",
                    "fold": 0,
                    "train_count": len(merged),
                    "test_count": 0,
                    "mae_days": None,
                    "median_error_days": None,
                    "p75_error_days": None,
                    "max_error_days": None,
                    "improvement_vs_median_pct": None,
                    "ready_for_eta": "false",
                    "note": "Need at least 10 merged PRs to backtest cycle forecasting.",
                }
            ],
            columns=columns,
        )
    rows: list[dict[str, Any]] = []
    folds = min(5, len(merged))
    for fold_number, (train_idx, test_idx) in enumerate(KFold(n_splits=folds, shuffle=True, random_state=42).split(merged), start=1):
        train = merged.iloc[train_idx].copy()
        test = merged.iloc[test_idx].copy()
        rows.extend(backtest_fold_rows("kfold", fold_number, train, test, feature_cols))
    chronological = merged.copy()
    chronological["_created_at_dt"] = chronological["created_at"].map(parse_dt)
    chronological = chronological.sort_values(["_created_at_dt", "pr_number"], na_position="last").drop(columns=["_created_at_dt"])
    split = max(1, int(len(chronological) * 0.7))
    if len(chronological) - split >= 5:
        rows.extend(backtest_fold_rows("chronological_holdout", 1, chronological.iloc[:split].copy(), chronological.iloc[split:].copy(), feature_cols))
    out = pd.DataFrame(rows, columns=columns)
    if out.empty:
        return pd.DataFrame(columns=columns)
    median_by_eval = {
        row.evaluation: float(row.mae_days)
        for row in out[(out["model"] == "median_cycle_baseline") & out["mae_days"].notna()].itertuples(index=False)
    }
    out["improvement_vs_median_pct"] = out.apply(
        lambda row: improvement_pct(median_by_eval.get(row["evaluation"]), row["mae_days"]),
        axis=1,
    )
    ready_model = forecast_eta_ready_model(forecast_backtest_metrics(out))
    out["ready_for_eta"] = out.apply(
        lambda row: "true" if temporal_feature_snapshot_ready and ready_model and row["model"] == ready_model else "false",
        axis=1,
    )
    return out


def build_source_event_as_of_forecast_backtest(event_snapshots: pd.DataFrame | None, feature_cols: list[str]) -> pd.DataFrame:
    columns = [
        "evaluation",
        "model",
        "fold",
        "train_count",
        "test_count",
        "mae_days",
        "median_error_days",
        "p75_error_days",
        "max_error_days",
        "improvement_vs_median_pct",
        "ready_for_eta",
        "note",
    ]
    if event_snapshots is None or event_snapshots.empty:
        return pd.DataFrame(columns=columns)

    examples = event_snapshots.copy()
    if "subject_key" not in examples.columns:
        examples["subject_key"] = examples.apply(
            lambda row: f"{clean_text(row.get('repository', ''))}#{clean_int(row.get('pr_number', None)) or ''}",
            axis=1,
        )
    for column in feature_cols:
        if column not in examples.columns:
            examples[column] = 0
    examples["cycle_time_days"] = pd.to_numeric(examples.get("cycle_time_days"), errors="coerce")
    examples["_observed_at_dt"] = examples.get("event_replay_observed_at", pd.Series("", index=examples.index)).map(parse_dt)
    examples["_created_at_dt"] = examples.get("created_at", pd.Series("", index=examples.index)).map(parse_dt)
    merged_at_values = examples.get("merged_at", pd.Series("", index=examples.index)).fillna("")
    closed_at_values = examples.get("closed_at", pd.Series("", index=examples.index)).fillna("")
    examples["_terminal_at_dt"] = merged_at_values.mask(merged_at_values == "", closed_at_values).map(parse_dt)
    if "is_merged" in examples.columns:
        examples = examples[pd.to_numeric(examples["is_merged"], errors="coerce").fillna(0).astype(int) == 1]
    examples = examples[
        examples["subject_key"].map(clean_text).ne("")
        & examples["cycle_time_days"].notna()
        & examples["_observed_at_dt"].notna()
        & examples["_created_at_dt"].notna()
        & (examples["_terminal_at_dt"].isna() | (examples["_observed_at_dt"] < examples["_terminal_at_dt"]))
    ].copy()
    subject_count = int(examples["subject_key"].nunique()) if not examples.empty else 0
    if len(examples) < MIN_AS_OF_FEATURE_SNAPSHOT_TRAINING_EXAMPLES or subject_count < MIN_AS_OF_FEATURE_SNAPSHOT_TRAINING_EXAMPLES:
        return pd.DataFrame(
            [
                {
                    "evaluation": "source_event_as_of",
                    "model": "none",
                    "fold": 0,
                    "train_count": subject_count,
                    "test_count": 0,
                    "mae_days": None,
                    "median_error_days": None,
                    "p75_error_days": None,
                    "max_error_days": None,
                    "improvement_vs_median_pct": None,
                    "ready_for_eta": "false",
                    "note": f"Need at least {MIN_AS_OF_FEATURE_SNAPSHOT_TRAINING_EXAMPLES} merged PR subjects with source-event snapshots.",
                }
            ],
            columns=columns,
        )

    rows: list[dict[str, Any]] = []
    grouped = examples.sort_values(["subject_key", "_observed_at_dt"]).reset_index(drop=True)
    folds = min(5, subject_count)
    if folds >= 2:
        groups = grouped["subject_key"].astype(str)
        for fold_number, (train_idx, test_idx) in enumerate(GroupKFold(n_splits=folds).split(grouped, groups=groups), start=1):
            rows.extend(
                backtest_fold_rows(
                    "source_event_as_of_kfold",
                    fold_number,
                    grouped.iloc[train_idx].copy(),
                    grouped.iloc[test_idx].copy(),
                    feature_cols,
                )
            )

    subjects = (
        examples[["subject_key", "_created_at_dt"]]
        .drop_duplicates("subject_key")
        .sort_values(["_created_at_dt", "subject_key"])
        .reset_index(drop=True)
    )
    split = max(1, int(len(subjects) * 0.7))
    if len(subjects) - split < 5:
        split = max(1, len(subjects) - 5)
    train_subjects = set(subjects.iloc[:split]["subject_key"].tolist())
    test_subjects = set(subjects.iloc[split:]["subject_key"].tolist())
    train = examples[examples["subject_key"].isin(train_subjects)].copy()
    test = examples[examples["subject_key"].isin(test_subjects)].copy()
    if not train.empty and not test.empty:
        rows.extend(backtest_fold_rows("source_event_as_of_chronological_holdout", 1, train, test, feature_cols))

    out = pd.DataFrame(rows, columns=columns)
    if out.empty:
        return pd.DataFrame(columns=columns)
    median_by_eval = {
        row.evaluation: float(row.mae_days)
        for row in out[(out["model"] == "median_cycle_baseline") & out["mae_days"].notna()].itertuples(index=False)
    }
    out["improvement_vs_median_pct"] = out.apply(
        lambda row: improvement_pct(median_by_eval.get(row["evaluation"]), row["mae_days"]),
        axis=1,
    )
    out["ready_for_eta"] = "false"
    out["note"] = "Source-event replay as-of feature backtest; grouped by PR subject so snapshots from one PR cannot cross train/test."
    return out


def build_tpm_decision_target_backtest(event_snapshots: pd.DataFrame | None, feature_cols: list[str]) -> pd.DataFrame:
    columns = [
        "target_kind",
        "evaluation",
        "model",
        "fold",
        "train_count",
        "test_count",
        "positive_count",
        "baseline_positive_rate",
        "precision_at_10pct",
        "lift_at_10pct",
        "roc_auc",
        "average_precision",
        "coverage_stratum",
        "ready_for_product_action",
        "note",
    ]
    examples = tpm_decision_target_examples(event_snapshots, feature_cols)
    subject_count = examples["subject_key"].nunique() if not examples.empty else 0
    positive_subject_count = examples[examples["target_abandoned"] == 1]["subject_key"].nunique() if not examples.empty else 0
    if examples.empty or subject_count < MIN_TPM_DECISION_TARGET_SUBJECTS or positive_subject_count < 2:
        return pd.DataFrame(
            [
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "insufficient_sample",
                    "model": "none",
                    "fold": 0,
                    "train_count": int(subject_count),
                    "test_count": 0,
                    "positive_count": int(positive_subject_count),
                    "baseline_positive_rate": None,
                    "precision_at_10pct": None,
                    "lift_at_10pct": None,
                    "roc_auc": None,
                    "average_precision": None,
                    "coverage_stratum": "",
                    "ready_for_product_action": "false",
                    "note": f"Need at least {MIN_TPM_DECISION_TARGET_SUBJECTS} terminal PR subjects and at least two closed-unmerged subjects for abandonment-risk validation.",
                }
            ],
            columns=columns,
        )

    rows: list[dict[str, Any]] = []
    grouped = examples.sort_values(["subject_key", "_observed_at_dt"]).reset_index(drop=True)
    folds = min(5, subject_count)
    if folds >= 2:
        groups = grouped["subject_key"].astype(str)
        for fold_number, (train_idx, test_idx) in enumerate(GroupKFold(n_splits=folds).split(grouped, groups=groups), start=1):
            rows.extend(
                tpm_decision_target_fold_rows(
                    "source_event_as_of_grouped_kfold",
                    fold_number,
                    grouped.iloc[train_idx].copy(),
                    grouped.iloc[test_idx].copy(),
                    feature_cols,
                )
            )

    subjects = (
        examples[["subject_key", "_created_at_dt"]]
        .drop_duplicates("subject_key")
        .sort_values(["_created_at_dt", "subject_key"])
        .reset_index(drop=True)
    )
    split = max(1, int(len(subjects) * 0.7))
    if len(subjects) - split < 5:
        split = max(1, len(subjects) - 5)
    train_subjects = set(subjects.iloc[:split]["subject_key"].tolist())
    test_subjects = set(subjects.iloc[split:]["subject_key"].tolist())
    train = examples[examples["subject_key"].isin(train_subjects)].copy()
    test = examples[examples["subject_key"].isin(test_subjects)].copy()
    if not train.empty and not test.empty:
        rows.extend(
            tpm_decision_target_fold_rows(
                "source_event_as_of_chronological_holdout",
                1,
                train,
                test,
                feature_cols,
            )
        )
    scored_examples = tpm_decision_target_oof_scores(grouped, feature_cols)
    rows.extend(tpm_decision_target_coverage_rows(scored_examples))

    out = pd.DataFrame(rows, columns=columns)
    if out.empty:
        return pd.DataFrame(columns=columns)
    out["ready_for_product_action"] = "false"
    return out


def build_tpm_decision_target_readiness(decision_target_backtest: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "target_kind",
        "model",
        "grouped_kfold_fold_count",
        "grouped_kfold_mean_lift_at_10pct",
        "grouped_kfold_min_lift_at_10pct",
        "chronological_lift_at_10pct",
        "chronological_precision_at_10pct",
        "chronological_roc_auc",
        "coverage_gate_state",
        "coverage_stratum_count",
        "validation_ready",
        "coverage_ready",
        "independent_evidence_ready",
        "owner_policy_ready",
        "same_model_validation_gate",
        "product_action_gate_state",
        "product_action_ready",
        "recommended_next_evidence",
        "note",
    ]
    if decision_target_backtest.empty:
        return pd.DataFrame(columns=columns)

    rows: list[dict[str, Any]] = []
    for target_kind, target_rows in decision_target_backtest.groupby("target_kind", dropna=False):
        target_kind_text = clean_text(target_kind) or "unknown"
        coverage_gate_state = decision_target_coverage_gate_state(target_rows)
        coverage_stratum_count = decision_target_coverage_stratum_count(target_rows)
        candidate_models = sorted(
            set(
                target_rows[
                    target_rows["model"].astype(str).str.contains("classifier|heuristic", regex=True, na=False)
                    & target_rows["evaluation"].astype(str).isin(
                        [
                            "source_event_as_of_grouped_kfold",
                            "source_event_as_of_chronological_holdout",
                        ]
                    )
                ]["model"].astype(str)
            )
        )
        if not candidate_models:
            candidate_models = sorted(set(target_rows["model"].astype(str))) or ["none"]
        for model_name in candidate_models:
            grouped = target_rows[
                (target_rows["evaluation"] == "source_event_as_of_grouped_kfold")
                & (target_rows["model"] == model_name)
            ].copy()
            chronological = target_rows[
                (target_rows["evaluation"] == "source_event_as_of_chronological_holdout")
                & (target_rows["model"] == model_name)
            ].copy()
            grouped_lifts = pd.to_numeric(grouped.get("lift_at_10pct", pd.Series(dtype=float)), errors="coerce").dropna()
            grouped_fold_count = int(grouped_lifts.count())
            grouped_mean_lift = round(float(grouped_lifts.mean()), 4) if grouped_fold_count else None
            grouped_min_lift = round(float(grouped_lifts.min()), 4) if grouped_fold_count else None
            chronological_row = chronological.sort_values("fold").head(1)
            chronological_lift = decision_target_first_float(chronological_row, "lift_at_10pct")
            chronological_precision = decision_target_first_float(chronological_row, "precision_at_10pct")
            chronological_roc_auc = decision_target_first_float(chronological_row, "roc_auc")
            validation_passed = (
                grouped_fold_count >= 2
                and grouped_mean_lift is not None
                and grouped_mean_lift >= MIN_TPM_DECISION_TARGET_MEAN_LIFT
                and grouped_min_lift is not None
                and grouped_min_lift > 0
                and chronological_lift is not None
                and chronological_lift >= MIN_TPM_DECISION_TARGET_CHRONO_LIFT
            )
            coverage_passed = coverage_gate_state == "stratified"
            independent_evidence_ready = False
            owner_policy_ready = False
            if not validation_passed:
                product_action_gate_state = "validation_gated"
            elif not coverage_passed:
                product_action_gate_state = "coverage_gated"
            elif not independent_evidence_ready:
                product_action_gate_state = "evidence_gated"
            elif not owner_policy_ready:
                product_action_gate_state = "owner_policy_gated"
            else:
                product_action_gate_state = "passed"
            rows.append(
                {
                    "target_kind": target_kind_text,
                    "model": model_name,
                    "grouped_kfold_fold_count": grouped_fold_count,
                    "grouped_kfold_mean_lift_at_10pct": grouped_mean_lift,
                    "grouped_kfold_min_lift_at_10pct": grouped_min_lift,
                    "chronological_lift_at_10pct": chronological_lift,
                    "chronological_precision_at_10pct": chronological_precision,
                    "chronological_roc_auc": chronological_roc_auc,
                    "coverage_gate_state": coverage_gate_state,
                    "coverage_stratum_count": coverage_stratum_count,
                    "validation_ready": "true" if validation_passed else "false",
                    "coverage_ready": "true" if coverage_passed else "false",
                    "independent_evidence_ready": "true" if independent_evidence_ready else "false",
                    "owner_policy_ready": "true" if owner_policy_ready else "false",
                    "same_model_validation_gate": "passed" if validation_passed else "gated",
                    "product_action_gate_state": product_action_gate_state,
                    "product_action_ready": "true" if product_action_gate_state == "passed" else "false",
                    "recommended_next_evidence": decision_target_next_evidence(
                        validation_passed,
                        coverage_passed,
                        coverage_gate_state,
                        grouped_fold_count,
                        grouped_min_lift,
                        chronological_lift,
                        independent_evidence_ready,
                        owner_policy_ready,
                    ),
                    "note": decision_target_readiness_note(model_name, validation_passed, coverage_passed),
                }
            )
    out = pd.DataFrame(rows, columns=columns)
    if out.empty:
        return pd.DataFrame(columns=columns)
    return out.sort_values(["target_kind", "product_action_gate_state", "model"]).reset_index(drop=True)


def decision_target_first_float(rows: pd.DataFrame, column: str) -> float | None:
    if rows.empty or column not in rows.columns:
        return None
    value = safe_float(rows.iloc[0][column])
    return round(value, 4) if value is not None else None


def decision_target_coverage_gate_state(target_rows: pd.DataFrame) -> str:
    summary = target_rows[target_rows["evaluation"] == "source_event_as_of_coverage_stratified_summary"]
    if not summary.empty:
        state = clean_text(summary.iloc[0].get("coverage_stratum", "")).strip()
        if state:
            return state
    if (target_rows["evaluation"] == "insufficient_sample").any():
        return "insufficient_sample"
    return "missing_coverage_summary"


def decision_target_coverage_stratum_count(target_rows: pd.DataFrame) -> int:
    strata = target_rows[target_rows["evaluation"] == "source_event_as_of_coverage_stratum"].get(
        "coverage_stratum",
        pd.Series(dtype=object),
    )
    return int(strata.map(clean_text).str.strip().replace("", pd.NA).dropna().nunique())


def decision_target_next_evidence(
    validation_passed: bool,
    coverage_passed: bool,
    coverage_gate_state: str,
    grouped_fold_count: int,
    grouped_min_lift: float | None,
    chronological_lift: float | None,
    independent_evidence_ready: bool = False,
    owner_policy_ready: bool = False,
) -> str:
    if grouped_fold_count < 2 or chronological_lift is None:
        return "collect_more_terminal_pr_outcomes_with_preterminal_snapshots"
    if not validation_passed:
        if grouped_min_lift is not None and grouped_min_lift <= 0:
            return "stabilize_grouped_kfold_signal_across_developer_or_time_splits"
        return "improve_model_features_and_repeat_kfold_plus_chronological_validation"
    if not coverage_passed:
        if coverage_gate_state == "not_testable_single_stratum":
            return "collect_multi_coverage_or_provenance_strata_before_product_action"
        return "increase_within_stratum_samples_for_coverage_confounding_checks"
    if not independent_evidence_ready:
        return "attach_independent_non_generated_evidence_before_product_action"
    if not owner_policy_ready:
        return "define_owner_review_policy_before_product_action"
    return "promote_decision_target_to_owner_review_with_evidence_audit"


def decision_target_readiness_note(model_name: str, validation_passed: bool, coverage_passed: bool) -> str:
    if validation_passed and coverage_passed:
        return f"{model_name} passes model and coverage validation; still requires independent evidence and owner-reviewed action policy before product action."
    if not validation_passed and not coverage_passed:
        return f"{model_name} has not passed stable same-model validation and coverage/provenance validation."
    if not validation_passed:
        return f"{model_name} has not passed stable same-model validation across grouped K-fold and chronological holdout."
    return f"{model_name} has signal, but coverage/provenance confounding is not cleared."


def tpm_decision_target_examples(event_snapshots: pd.DataFrame | None, feature_cols: list[str]) -> pd.DataFrame:
    if event_snapshots is None or event_snapshots.empty:
        return pd.DataFrame()
    examples = event_snapshots.copy()
    if "subject_key" not in examples.columns:
        examples["subject_key"] = examples.apply(
            lambda row: f"{clean_text(row.get('repository', ''))}#{clean_int(row.get('pr_number', None)) or ''}",
            axis=1,
        )
    for column in feature_cols:
        if column not in examples.columns:
            examples[column] = 0
    examples["cycle_time_days"] = pd.to_numeric(examples.get("cycle_time_days"), errors="coerce")
    examples["_observed_at_dt"] = examples.get("event_replay_observed_at", pd.Series("", index=examples.index)).map(parse_dt)
    examples["_created_at_dt"] = examples.get("created_at", pd.Series("", index=examples.index)).map(parse_dt)
    merged_at_values = examples.get("merged_at", pd.Series("", index=examples.index)).fillna("")
    closed_at_values = examples.get("closed_at", pd.Series("", index=examples.index)).fillna("")
    examples["_terminal_at_dt"] = merged_at_values.mask(merged_at_values == "", closed_at_values).map(parse_dt)
    examples["target_abandoned"] = (pd.to_numeric(examples.get("is_merged", pd.Series(0, index=examples.index)), errors="coerce").fillna(0).astype(int) == 0).astype(int)
    return examples[
        examples["subject_key"].map(clean_text).ne("")
        & examples["cycle_time_days"].notna()
        & examples["_observed_at_dt"].notna()
        & examples["_created_at_dt"].notna()
        & examples["_terminal_at_dt"].notna()
        & (examples["_observed_at_dt"] < examples["_terminal_at_dt"])
    ].copy().assign(coverage_stratum=lambda data: data.apply(risk_backtest_coverage_stratum, axis=1))


def tpm_decision_target_fold_rows(
    evaluation: str,
    fold_number: int,
    train: pd.DataFrame,
    test: pd.DataFrame,
    feature_cols: list[str],
) -> list[dict[str, Any]]:
    target = "target_abandoned"
    train_y = train[target].astype(int)
    rows = [
        tpm_decision_target_metric_row(
            evaluation,
            "base_rate_baseline",
            fold_number,
            len(train),
            len(test),
            test[target].astype(int),
            pd.Series([float(train_y.mean())] * len(test), index=test.index),
        ),
        tpm_decision_target_metric_row(
            evaluation,
            "abandonment_heuristic",
            fold_number,
            len(train),
            len(test),
            test[target].astype(int),
            abandonment_risk_score(test),
        ),
    ]
    if len(train) >= 10 and train_y.nunique() >= 2 and len(test) > 0:
        model = RandomForestClassifier(n_estimators=200, random_state=42, min_samples_leaf=2, class_weight="balanced")
        model.fit(train[feature_cols].fillna(0), train_y)
        probability = pd.Series(model.predict_proba(test[feature_cols].fillna(0))[:, 1], index=test.index)
        rows.append(
            tpm_decision_target_metric_row(
                evaluation,
                "random_forest_classifier",
                fold_number,
                len(train),
                len(test),
                test[target].astype(int),
                probability,
            )
        )
    return rows


def tpm_decision_target_metric_row(
    evaluation: str,
    model_name: str,
    fold_number: int,
    train_count: int,
    test_count: int,
    actual: pd.Series,
    score: pd.Series,
    coverage_stratum: str = "",
) -> dict[str, Any]:
    actual = actual.astype(int)
    score = pd.to_numeric(score, errors="coerce").fillna(0.0)
    base_rate = float(actual.mean()) if len(actual) else 0.0
    rows = pd.DataFrame({"actual": actual, "score": score})
    top, label = top_fraction(rows, 0.10, "score")
    precision = float(top["actual"].mean()) if not top.empty else None
    roc_auc = classification_metric(actual, score, "roc_auc")
    average_precision = classification_metric(actual, score, "average_precision")
    return {
        "target_kind": "abandonment_risk",
        "evaluation": evaluation,
        "model": model_name,
        "fold": fold_number,
        "train_count": int(train_count),
        "test_count": int(test_count),
        "positive_count": int(actual.sum()),
        "baseline_positive_rate": round(base_rate, 4),
        "precision_at_10pct": round(precision, 4) if precision is not None else None,
        "lift_at_10pct": round(precision - base_rate, 4) if precision is not None else None,
        "roc_auc": round(roc_auc, 4) if roc_auc is not None else None,
        "average_precision": round(average_precision, 4) if average_precision is not None else None,
        "coverage_stratum": coverage_stratum,
        "ready_for_product_action": "false",
        "note": decision_target_metric_note(model_name, label, coverage_stratum),
    }


def decision_target_metric_note(model_name: str, label: str, coverage_stratum: str = "") -> str:
    scope = f" within source coverage stratum {coverage_stratum}" if coverage_stratum else ""
    return (
        f"{model_name} ranks pre-terminal source-event snapshots for closed-unmerged risk at {label}{scope}; "
        "validation evidence only, not an autonomous close/park decision."
    )


def tpm_decision_target_oof_scores(examples: pd.DataFrame, feature_cols: list[str]) -> pd.DataFrame:
    if examples.empty:
        return examples.copy()
    scored = examples.copy()
    scored["abandonment_heuristic_score"] = abandonment_risk_score(scored)
    scored["random_forest_classifier_oof_score"] = pd.NA
    subject_count = scored["subject_key"].nunique()
    folds = min(5, subject_count)
    if folds < 2:
        return scored
    groups = scored["subject_key"].astype(str)
    target = "target_abandoned"
    for train_idx, test_idx in GroupKFold(n_splits=folds).split(scored, groups=groups):
        train = scored.iloc[train_idx].copy()
        test = scored.iloc[test_idx].copy()
        train_y = train[target].astype(int)
        if len(train) < 10 or train_y.nunique() < 2 or test.empty:
            continue
        model = RandomForestClassifier(n_estimators=200, random_state=42, min_samples_leaf=2, class_weight="balanced")
        model.fit(train[feature_cols].fillna(0), train_y)
        scored.loc[test.index, "random_forest_classifier_oof_score"] = model.predict_proba(test[feature_cols].fillna(0))[:, 1]
    return scored


def tpm_decision_target_coverage_rows(scored_examples: pd.DataFrame) -> list[dict[str, Any]]:
    if scored_examples.empty:
        return []
    data = scored_examples.copy()
    if "coverage_stratum" not in data.columns:
        data["coverage_stratum"] = data.apply(risk_backtest_coverage_stratum, axis=1)
    stratum_count = int(data["coverage_stratum"].nunique())
    eligible_strata = {
        stratum: group for stratum, group in data.groupby("coverage_stratum") if len(group) >= MIN_RISK_BACKTEST_COVERAGE_STRATUM_SAMPLE
    }
    if stratum_count <= 1:
        state = "not_testable_single_stratum"
        note = "Decision-target validation has one source coverage/provenance stratum; coverage confounding cannot be tested from this sample."
    elif eligible_strata:
        state = "stratified"
        note = "Decision-target validation includes source coverage/provenance strata with enough sample for within-stratum precision checks."
    else:
        state = "insufficient_stratum_sample"
        note = "Decision-target validation has multiple source coverage/provenance strata, but each stratum is too sparse for reliable within-stratum precision."
    rows: list[dict[str, Any]] = [
        {
            "target_kind": "abandonment_risk",
            "evaluation": "source_event_as_of_coverage_stratified_summary",
            "model": "coverage_guardrail",
            "fold": 0,
            "train_count": 0,
            "test_count": int(len(data)),
            "positive_count": int(data["target_abandoned"].astype(int).sum()),
            "baseline_positive_rate": round(float(data["target_abandoned"].astype(int).mean()), 4) if len(data) else None,
            "precision_at_10pct": None,
            "lift_at_10pct": None,
            "roc_auc": None,
            "average_precision": None,
            "coverage_stratum": state,
            "ready_for_product_action": "false",
            "note": f"{note} Distinct source coverage/provenance strata: {stratum_count}. Validation evidence only.",
        }
    ]
    score_columns = [
        ("abandonment_heuristic", "abandonment_heuristic_score"),
        ("random_forest_classifier_oof", "random_forest_classifier_oof_score"),
    ]
    for stratum, group in sorted(data.groupby("coverage_stratum"), key=lambda item: (-len(item[1]), item[0])):
        if len(group) < MIN_RISK_BACKTEST_COVERAGE_STRATUM_SAMPLE:
            rows.append(
                {
                    "target_kind": "abandonment_risk",
                    "evaluation": "source_event_as_of_coverage_stratum",
                    "model": "coverage_guardrail",
                    "fold": 0,
                    "train_count": 0,
                    "test_count": int(len(group)),
                    "positive_count": int(group["target_abandoned"].astype(int).sum()),
                    "baseline_positive_rate": round(float(group["target_abandoned"].astype(int).mean()), 4) if len(group) else None,
                    "precision_at_10pct": None,
                    "lift_at_10pct": None,
                    "roc_auc": None,
                    "average_precision": None,
                    "coverage_stratum": stratum,
                    "ready_for_product_action": "false",
                    "note": f"Need at least {MIN_RISK_BACKTEST_COVERAGE_STRATUM_SAMPLE} rows for within-stratum decision-target precision; validation evidence only.",
                }
            )
            continue
        for model_name, score_column in score_columns:
            if score_column not in group.columns:
                continue
            valid = group[group[score_column].notna()].copy()
            if valid.empty:
                continue
            rows.append(
                tpm_decision_target_metric_row(
                    "source_event_as_of_coverage_stratum",
                    model_name,
                    0,
                    0,
                    len(valid),
                    valid["target_abandoned"].astype(int),
                    pd.to_numeric(valid[score_column], errors="coerce"),
                    stratum,
                )
            )
    return rows


def classification_metric(actual: pd.Series, score: pd.Series, metric: str) -> float | None:
    if len(actual) == 0 or actual.nunique() < 2:
        return None
    try:
        if metric == "roc_auc":
            return float(roc_auc_score(actual.astype(int), score.astype(float)))
        if metric == "average_precision":
            return float(average_precision_score(actual.astype(int), score.astype(float)))
    except ValueError:
        return None
    return None


def abandonment_risk_score(rows: pd.DataFrame) -> pd.Series:
    comments = pd.to_numeric(rows.get("comments", pd.Series(0, index=rows.index)), errors="coerce").fillna(0)
    review_comments = pd.to_numeric(rows.get("review_comments", pd.Series(0, index=rows.index)), errors="coerce").fillna(0)
    linked_tickets = pd.to_numeric(rows.get("linked_ticket_count", pd.Series(0, index=rows.index)), errors="coerce").fillna(0)
    commits = pd.to_numeric(rows.get("commits", pd.Series(0, index=rows.index)), errors="coerce").fillna(0)
    author_open = pd.to_numeric(rows.get("author_open_pr_count", pd.Series(0, index=rows.index)), errors="coerce").fillna(0)
    author_prior = pd.to_numeric(rows.get("author_prior_pr_count", pd.Series(0, index=rows.index)), errors="coerce").fillna(0)
    age = pd.to_numeric(rows.get("age_days", pd.Series(0, index=rows.index)), errors="coerce").fillna(0)
    discussion = (comments + review_comments).clip(upper=20) / 20.0
    graph_load = author_open.clip(upper=10) / 10.0
    sparse_work = 1.0 / (1.0 + commits.clip(lower=0))
    linked_complexity = linked_tickets.clip(upper=5) / 5.0
    prior_context = (author_prior > 0).astype(float) * 0.1
    return (age.clip(upper=60) / 60.0) * 0.30 + discussion * 0.20 + graph_load * 0.20 + sparse_work * 0.20 + linked_complexity * 0.10 - prior_context


def build_lifecycle_as_of_forecast_backtest(merged: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "evaluation",
        "model",
        "fold",
        "train_count",
        "test_count",
        "mae_days",
        "median_error_days",
        "p75_error_days",
        "max_error_days",
        "improvement_vs_median_pct",
        "ready_for_eta",
        "note",
    ]
    examples = lifecycle_as_of_examples(merged)
    subject_count = examples["subject_key"].nunique() if not examples.empty else 0
    if examples.empty or subject_count < MIN_LIFECYCLE_AS_OF_SUBJECTS:
        return pd.DataFrame(
            [
                {
                    "evaluation": "lifecycle_as_of_baseline",
                    "model": "none",
                    "fold": 0,
                    "train_count": subject_count,
                    "test_count": 0,
                    "mae_days": None,
                    "median_error_days": None,
                    "p75_error_days": None,
                    "max_error_days": None,
                    "improvement_vs_median_pct": None,
                    "ready_for_eta": "false",
                    "note": f"Need at least {MIN_LIFECYCLE_AS_OF_SUBJECTS} merged PRs with lifecycle timestamps to benchmark lifecycle-as-of baselines.",
                }
            ],
            columns=columns,
        )

    subjects = (
        examples[["subject_key", "created_at_dt"]]
        .drop_duplicates("subject_key")
        .sort_values(["created_at_dt", "subject_key"])
        .reset_index(drop=True)
    )
    split = max(1, int(len(subjects) * 0.7))
    if len(subjects) - split < 5:
        split = max(1, len(subjects) - 5)
    train_subjects = set(subjects.iloc[:split]["subject_key"].tolist())
    test_subjects = set(subjects.iloc[split:]["subject_key"].tolist())
    train = examples[examples["subject_key"].isin(train_subjects)].copy()
    test = examples[examples["subject_key"].isin(test_subjects)].copy()
    if train.empty or test.empty:
        return pd.DataFrame(
            [
                {
                    "evaluation": "lifecycle_as_of_baseline",
                    "model": "none",
                    "fold": 0,
                    "train_count": len(train),
                    "test_count": len(test),
                    "mae_days": None,
                    "median_error_days": None,
                    "p75_error_days": None,
                    "max_error_days": None,
                    "improvement_vs_median_pct": None,
                    "ready_for_eta": "false",
                    "note": "Lifecycle-as-of subject split produced no train or test examples.",
                }
            ],
            columns=columns,
        )

    actual = test["remaining_days"].astype(float)
    train_median_remaining = float(train["remaining_days"].median())
    train_median_cycle = float(train["cycle_time_days"].median())
    rows = [
        forecast_error_row(
            "lifecycle_as_of_baseline",
            "median_remaining_baseline",
            1,
            len(train),
            len(test),
            actual,
            pd.Series([train_median_remaining] * len(test), index=test.index),
        ),
        forecast_error_row(
            "lifecycle_as_of_baseline",
            "median_cycle_remaining_baseline",
            1,
            len(train),
            len(test),
            actual,
            (train_median_cycle - test["as_of_age_days"].astype(float)).clip(lower=0.0),
        ),
        forecast_error_row(
            "lifecycle_as_of_baseline",
            "age_bucket_median_remaining",
            1,
            len(train),
            len(test),
            actual,
            lifecycle_age_bucket_predictions(train, test, train_median_remaining),
        ),
    ]
    baseline_mae = rows[0]["mae_days"]
    for row in rows:
        row["improvement_vs_median_pct"] = improvement_pct(baseline_mae, row["mae_days"])
        row["ready_for_eta"] = "false"
        row["note"] = (
            "Lifecycle-derived synthetic as-of checkpoint baseline using only created_at, terminal_at, and elapsed age. "
            "This benchmarks simple remaining-time baselines but does not clear ETA readiness without live pre-terminal feature snapshots."
        )
    return pd.DataFrame(rows, columns=columns)


def lifecycle_as_of_examples(merged: pd.DataFrame) -> pd.DataFrame:
    columns = ["subject_key", "created_at_dt", "as_of_age_days", "remaining_days", "cycle_time_days", "age_bucket"]
    if merged.empty:
        return pd.DataFrame(columns=columns)
    rows: list[dict[str, Any]] = []
    for row in merged.itertuples(index=False):
        repository = clean_text(getattr(row, "repository", ""))
        pr_number = clean_int(getattr(row, "pr_number", None))
        created_at = parse_dt(clean_text(getattr(row, "created_at", "")))
        terminal_at = parse_dt(clean_text(getattr(row, "merged_at", "")) or clean_text(getattr(row, "closed_at", "")))
        cycle_time_days = safe_float(getattr(row, "cycle_time_days", None))
        if not repository or pr_number is None or created_at is None or terminal_at is None or cycle_time_days is None or cycle_time_days <= 0:
            continue
        subject_key = f"{repository}#{pr_number}"
        for checkpoint_days in LIFECYCLE_AS_OF_CHECKPOINT_DAYS:
            if checkpoint_days >= cycle_time_days:
                continue
            checkpoint_at = created_at + timedelta(days=float(checkpoint_days))
            if checkpoint_at >= terminal_at:
                continue
            remaining_days = max(0.0, cycle_time_days - float(checkpoint_days))
            rows.append(
                {
                    "subject_key": subject_key,
                    "created_at_dt": created_at,
                    "as_of_age_days": float(checkpoint_days),
                    "remaining_days": remaining_days,
                    "cycle_time_days": float(cycle_time_days),
                    "age_bucket": lifecycle_age_bucket(float(checkpoint_days)),
                }
            )
    return pd.DataFrame(rows, columns=columns)


def lifecycle_age_bucket_predictions(train: pd.DataFrame, test: pd.DataFrame, fallback: float) -> pd.Series:
    bucket_medians = train.groupby("age_bucket")["remaining_days"].median().to_dict() if "age_bucket" in train.columns else {}
    values = [float(bucket_medians.get(bucket, fallback)) for bucket in test["age_bucket"].tolist()]
    return pd.Series(values, index=test.index)


def lifecycle_age_bucket(age_days: float) -> str:
    if age_days < 1.0:
        return "under_1d"
    if age_days < 2.0:
        return "1_2d"
    if age_days < 4.0:
        return "2_4d"
    if age_days < 7.0:
        return "4_7d"
    if age_days < 14.0:
        return "7_14d"
    if age_days < 30.0:
        return "14_30d"
    return "30d_plus"


def lifecycle_as_of_backtest_metrics(backtest: pd.DataFrame, merged: pd.DataFrame | None = None) -> dict[str, str]:
    examples = lifecycle_as_of_examples(merged) if merged is not None and not merged.empty else pd.DataFrame()
    terminal_subject_count = int(examples["subject_key"].nunique()) if not examples.empty else 0
    training_example_count = int(len(examples)) if not examples.empty else 0
    if backtest.empty or "evaluation" not in backtest.columns:
        return {
            "state": "missing",
            "terminal_subject_count": str(terminal_subject_count),
            "training_example_count": str(training_example_count),
        }
    rows = backtest[backtest["evaluation"] == "lifecycle_as_of_baseline"].copy()
    if rows.empty:
        return {
            "state": "missing",
            "terminal_subject_count": str(terminal_subject_count),
            "training_example_count": str(training_example_count),
        }
    scored = rows[rows["mae_days"].notna()].copy()
    if scored.empty:
        return {
            "state": "insufficient_sample",
            "terminal_subject_count": str(terminal_subject_count),
            "training_example_count": str(training_example_count),
        }
    grouped = scored.groupby("model")["mae_days"].mean()
    best_model = str(grouped.sort_values().index[0]) if not grouped.empty else ""
    out = {
        "state": "baseline_available",
        "terminal_subject_count": str(terminal_subject_count),
        "training_example_count": str(training_example_count),
        "best_model": best_model,
        "best_mae_days": format_optional_float(float(grouped[best_model])) if best_model else "",
    }
    for model_name, mae in grouped.items():
        out[f"{model_name}_mae_days"] = format_optional_float(float(mae))
    return out


def build_survival_time_to_merge_backtest(pr_features: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "evaluation",
        "model",
        "fold",
        "train_count",
        "test_count",
        "mae_days",
        "median_error_days",
        "p75_error_days",
        "max_error_days",
        "improvement_vs_median_pct",
        "ready_for_eta",
        "note",
    ]
    subjects = survival_time_to_merge_subjects(pr_features)
    terminal = subjects[subjects["state"].isin(["merged", "closed"])].copy() if not subjects.empty else pd.DataFrame()
    event_count = int(terminal["event_observed"].sum()) if not terminal.empty else 0
    if terminal.empty or len(terminal) < MIN_SURVIVAL_TIME_TO_MERGE_SUBJECTS or event_count < MIN_SURVIVAL_TIME_TO_MERGE_SUBJECTS:
        return pd.DataFrame(
            [
                {
                    "evaluation": "survival_time_to_merge",
                    "model": "none",
                    "fold": 0,
                    "train_count": len(terminal),
                    "test_count": 0,
                    "mae_days": None,
                    "median_error_days": None,
                    "p75_error_days": None,
                    "max_error_days": None,
                    "improvement_vs_median_pct": None,
                    "ready_for_eta": "false",
                    "note": (
                        f"Need at least {MIN_SURVIVAL_TIME_TO_MERGE_SUBJECTS} terminal PR subjects and "
                        f"{MIN_SURVIVAL_TIME_TO_MERGE_SUBJECTS} merge events to evaluate censored time-to-merge baselines."
                    ),
                }
            ],
            columns=columns,
        )

    terminal = terminal.sort_values(["created_at_dt", "subject_key"]).reset_index(drop=True)
    split = max(1, int(len(terminal) * 0.7))
    if len(terminal) - split < 5:
        split = max(1, len(terminal) - 5)
    train = terminal.iloc[:split].copy()
    test_events = terminal.iloc[split:].copy()
    test_events = test_events[test_events["event_observed"] == 1].copy()
    examples = survival_time_to_merge_examples(test_events)
    if train.empty or examples.empty:
        return pd.DataFrame(
            [
                {
                    "evaluation": "survival_time_to_merge",
                    "model": "none",
                    "fold": 0,
                    "train_count": len(train),
                    "test_count": len(examples),
                    "mae_days": None,
                    "median_error_days": None,
                    "p75_error_days": None,
                    "max_error_days": None,
                    "improvement_vs_median_pct": None,
                    "ready_for_eta": "false",
                    "note": "Chronological split produced no held-out merged PR examples for survival evaluation.",
                }
            ],
            columns=columns,
        )

    event_durations = train[train["event_observed"] == 1]["duration_days"].astype(float)
    fallback_duration = float(event_durations.median()) if not event_durations.empty else float(train["duration_days"].median())
    actual = examples["remaining_days"].astype(float)
    rows = [
        forecast_error_row(
            "survival_time_to_merge",
            "median_event_remaining_baseline",
            1,
            len(train),
            len(examples),
            actual,
            (fallback_duration - examples["as_of_age_days"].astype(float)).clip(lower=0.0),
        ),
        forecast_error_row(
            "survival_time_to_merge",
            "km_restricted_mean_remaining",
            1,
            len(train),
            len(examples),
            actual,
            survival_time_to_merge_predictions(train, examples, "restricted_mean", fallback_duration),
        ),
        forecast_error_row(
            "survival_time_to_merge",
            "km_median_remaining",
            1,
            len(train),
            len(examples),
            actual,
            survival_time_to_merge_predictions(train, examples, "median", fallback_duration),
        ),
    ]
    baseline_mae = rows[0]["mae_days"]
    for row in rows:
        row["improvement_vs_median_pct"] = improvement_pct(baseline_mae, row["mae_days"])
        row["ready_for_eta"] = "false"
        row["note"] = (
            "Kaplan-Meier-style censored time-to-merge baseline. Closed-unmerged and open PRs are censoring evidence, "
            "so this supports risk calibration and survival curves, not point ETA commitments."
        )
    return pd.DataFrame(rows, columns=columns)


def survival_time_to_merge_subjects(pr_features: pd.DataFrame) -> pd.DataFrame:
    columns = ["subject_key", "repository", "pr_number", "state", "created_at_dt", "duration_days", "event_observed"]
    if pr_features.empty:
        return pd.DataFrame(columns=columns)
    rows: list[dict[str, Any]] = []
    for row in pr_features.itertuples(index=False):
        repository = clean_text(getattr(row, "repository", ""))
        pr_number = clean_int(getattr(row, "pr_number", None))
        state = clean_text(getattr(row, "state", "")).lower()
        created_at = parse_dt(clean_text(getattr(row, "created_at", "")))
        if not repository or pr_number is None or created_at is None:
            continue
        duration = survival_subject_duration_days(row, state, created_at)
        if duration is None or duration <= 0:
            continue
        rows.append(
            {
                "subject_key": f"{repository}#{pr_number}",
                "repository": repository,
                "pr_number": pr_number,
                "state": state,
                "created_at_dt": created_at,
                "duration_days": float(duration),
                "event_observed": 1 if state == "merged" else 0,
            }
        )
    return pd.DataFrame(rows, columns=columns)


def survival_subject_duration_days(row: Any, state: str, created_at: datetime) -> float | None:
    if state == "merged":
        terminal_at = parse_dt(clean_text(getattr(row, "merged_at", ""))) or parse_dt(clean_text(getattr(row, "closed_at", "")))
        duration = safe_float(getattr(row, "cycle_time_days", None))
        if duration is not None:
            return duration
        return days_between(created_at, terminal_at) if terminal_at is not None else None
    if state == "closed":
        terminal_at = parse_dt(clean_text(getattr(row, "closed_at", "")))
        duration = safe_float(getattr(row, "cycle_time_days", None))
        if duration is not None:
            return duration
        return days_between(created_at, terminal_at) if terminal_at is not None else None
    if state == "open":
        return safe_float(getattr(row, "age_days", None))
    return None


def days_between(start: datetime, end: datetime | None) -> float | None:
    if end is None:
        return None
    return max(0.0, (end - start).total_seconds() / 86400.0)


def survival_time_to_merge_examples(event_subjects: pd.DataFrame) -> pd.DataFrame:
    columns = ["subject_key", "created_at_dt", "as_of_age_days", "remaining_days", "duration_days"]
    if event_subjects.empty:
        return pd.DataFrame(columns=columns)
    rows: list[dict[str, Any]] = []
    for row in event_subjects.itertuples(index=False):
        duration = safe_float(getattr(row, "duration_days", None))
        if duration is None or duration <= 0:
            continue
        for checkpoint_days in LIFECYCLE_AS_OF_CHECKPOINT_DAYS:
            if checkpoint_days >= duration:
                continue
            rows.append(
                {
                    "subject_key": clean_text(getattr(row, "subject_key", "")),
                    "created_at_dt": getattr(row, "created_at_dt", None),
                    "as_of_age_days": float(checkpoint_days),
                    "remaining_days": float(duration) - float(checkpoint_days),
                    "duration_days": float(duration),
                }
            )
    return pd.DataFrame(rows, columns=columns)


def survival_time_to_merge_predictions(train: pd.DataFrame, examples: pd.DataFrame, model: str, fallback_duration: float) -> pd.Series:
    curve = kaplan_meier_curve(train["duration_days"].astype(float), train["event_observed"].astype(int))
    horizon = min(
        SURVIVAL_TIME_TO_MERGE_RESTRICTED_MAX_DAYS,
        max(float(train["duration_days"].max()), fallback_duration),
    )
    values: list[float] = []
    for age in examples["as_of_age_days"].astype(float).tolist():
        if model == "median":
            value = conditional_km_median_remaining(curve, age, horizon)
        else:
            value = conditional_km_restricted_mean_remaining(curve, age, horizon)
        if value is None or not math.isfinite(value):
            value = max(0.0, fallback_duration - age)
        values.append(round(max(0.0, float(value)), 2))
    return pd.Series(values, index=examples.index)


def kaplan_meier_curve(durations: pd.Series, events: pd.Series) -> list[tuple[float, float]]:
    observed = pd.DataFrame({"duration": durations.astype(float), "event": events.astype(int)})
    observed = observed[observed["duration"].notna() & (observed["duration"] >= 0)].sort_values("duration")
    if observed.empty:
        return [(0.0, 1.0)]
    at_risk = len(observed)
    survival = 1.0
    curve = [(0.0, 1.0)]
    for time_value in sorted(observed["duration"].unique()):
        at_time = observed[observed["duration"] == time_value]
        event_count = int(at_time["event"].sum())
        censored_count = int(len(at_time) - event_count)
        if at_risk > 0 and event_count > 0:
            survival *= max(0.0, 1.0 - (event_count / at_risk))
        curve.append((float(time_value), float(survival)))
        at_risk -= event_count + censored_count
    return curve


def km_survival_at(curve: list[tuple[float, float]], time_value: float) -> float:
    survival = 1.0
    for point_time, point_survival in curve:
        if point_time <= time_value:
            survival = point_survival
        else:
            break
    return float(survival)


def conditional_km_restricted_mean_remaining(curve: list[tuple[float, float]], age_days: float, horizon_days: float) -> float | None:
    if horizon_days <= age_days:
        return 0.0
    base_survival = km_survival_at(curve, age_days)
    if base_survival <= 0:
        return None
    breakpoints = [age_days]
    breakpoints.extend(point_time for point_time, _ in curve if age_days < point_time < horizon_days)
    breakpoints.append(horizon_days)
    remaining = 0.0
    for left, right in zip(breakpoints, breakpoints[1:]):
        if right <= left:
            continue
        conditional_survival = km_survival_at(curve, left) / base_survival
        remaining += (right - left) * max(0.0, min(1.0, conditional_survival))
    return remaining


def conditional_km_median_remaining(curve: list[tuple[float, float]], age_days: float, horizon_days: float) -> float | None:
    base_survival = km_survival_at(curve, age_days)
    if base_survival <= 0:
        return None
    target = base_survival * 0.5
    for point_time, point_survival in curve:
        if point_time >= age_days and point_survival <= target:
            return max(0.0, point_time - age_days)
    return conditional_km_restricted_mean_remaining(curve, age_days, horizon_days)


def survival_time_to_merge_metrics(backtest: pd.DataFrame, pr_features: pd.DataFrame) -> dict[str, str]:
    subjects = survival_time_to_merge_subjects(pr_features)
    subject_count = int(len(subjects))
    event_subject_count = int(subjects["event_observed"].sum()) if not subjects.empty else 0
    censored_subject_count = subject_count - event_subject_count
    open_censored_subject_count = int((subjects["state"] == "open").sum()) if not subjects.empty else 0
    censoring_rate = (censored_subject_count / subject_count) if subject_count else None
    if backtest.empty or "evaluation" not in backtest.columns:
        return {
            "state": "missing",
            "subject_count": str(subject_count),
            "event_subject_count": str(event_subject_count),
            "censored_subject_count": str(censored_subject_count),
            "open_censored_subject_count": str(open_censored_subject_count),
            "censoring_rate": format_optional_float(censoring_rate),
            "backtest_example_count": "0",
        }
    rows = backtest[backtest["evaluation"] == "survival_time_to_merge"].copy()
    if rows.empty:
        return {
            "state": "missing",
            "subject_count": str(subject_count),
            "event_subject_count": str(event_subject_count),
            "censored_subject_count": str(censored_subject_count),
            "open_censored_subject_count": str(open_censored_subject_count),
            "censoring_rate": format_optional_float(censoring_rate),
            "backtest_example_count": "0",
        }
    scored = rows[rows["mae_days"].notna()].copy()
    if scored.empty:
        return {
            "state": "insufficient_sample",
            "subject_count": str(subject_count),
            "event_subject_count": str(event_subject_count),
            "censored_subject_count": str(censored_subject_count),
            "open_censored_subject_count": str(open_censored_subject_count),
            "censoring_rate": format_optional_float(censoring_rate),
            "backtest_example_count": "0",
        }
    grouped = scored.groupby("model")["mae_days"].mean()
    best_model = str(grouped.sort_values().index[0]) if not grouped.empty else ""
    return {
        "state": "baseline_available",
        "subject_count": str(subject_count),
        "event_subject_count": str(event_subject_count),
        "censored_subject_count": str(censored_subject_count),
        "open_censored_subject_count": str(open_censored_subject_count),
        "censoring_rate": format_optional_float(censoring_rate),
        "backtest_example_count": str(int(scored["test_count"].max())) if "test_count" in scored.columns else "0",
        "best_model": best_model,
        "best_mae_days": format_optional_float(float(grouped[best_model])) if best_model else "",
    }


def backtest_fold_rows(
    evaluation: str,
    fold_number: int,
    train: pd.DataFrame,
    test: pd.DataFrame,
    feature_cols: list[str],
) -> list[dict[str, Any]]:
    train_median = float(train["cycle_time_days"].median())
    train_p75 = float(train["cycle_time_days"].quantile(0.75)) if len(train) >= 2 else train_median * 1.5
    actual = test["cycle_time_days"].astype(float)
    rows = [
        forecast_error_row(evaluation, "median_cycle_baseline", fold_number, len(train), len(test), actual, pd.Series([train_median] * len(test), index=test.index)),
        forecast_error_row(evaluation, "heuristic_percentile", fold_number, len(train), len(test), actual, heuristic_cycle_prediction(test, train_median, train_p75)),
    ]
    if "author_prior_median_cycle_days" in test.columns:
        author_prediction = pd.to_numeric(test["author_prior_median_cycle_days"], errors="coerce")
        author_prediction = author_prediction.where(author_prediction > 0, train_median).fillna(train_median)
        rows.append(forecast_error_row(evaluation, "author_history_median_cycle", fold_number, len(train), len(test), actual, author_prediction))
    if len(train) >= 10 and len(test) > 0:
        boosting = GradientBoostingRegressor(
            loss="absolute_error",
            random_state=42,
            n_estimators=120,
            max_depth=2,
            learning_rate=0.05,
            min_samples_leaf=10,
        )
        boosting.fit(train[feature_cols].fillna(0), train["cycle_time_days"].astype(float))
        boosting_prediction = pd.Series(boosting.predict(test[feature_cols].fillna(0)), index=test.index)
        rows.append(forecast_error_row(evaluation, "gradient_boosting_absolute_error", fold_number, len(train), len(test), actual, boosting_prediction))
        hist_boosting = HistGradientBoostingRegressor(
            loss="absolute_error",
            random_state=42,
            max_leaf_nodes=15,
            learning_rate=0.04,
            min_samples_leaf=20,
            max_iter=180,
        )
        hist_boosting.fit(train[feature_cols].fillna(0), train["cycle_time_days"].astype(float))
        hist_boosting_prediction = pd.Series(hist_boosting.predict(test[feature_cols].fillna(0)), index=test.index)
        rows.append(
            forecast_error_row(
                evaluation,
                "hist_gradient_boosting_absolute_error",
                fold_number,
                len(train),
                len(test),
                actual,
                hist_boosting_prediction,
            )
        )
        model = RandomForestRegressor(n_estimators=200, random_state=42, min_samples_leaf=2)
        model.fit(train[feature_cols].fillna(0), train["cycle_time_days"].astype(float))
        prediction = pd.Series(model.predict(test[feature_cols].fillna(0)), index=test.index)
        rows.append(forecast_error_row(evaluation, "random_forest_regressor", fold_number, len(train), len(test), actual, prediction))
    return rows


def forecast_error_row(
    evaluation: str,
    model_name: str,
    fold_number: int,
    train_count: int,
    test_count: int,
    actual: pd.Series,
    prediction: pd.Series,
) -> dict[str, Any]:
    errors = (actual.astype(float) - prediction.astype(float)).abs()
    return {
        "evaluation": evaluation,
        "model": model_name,
        "fold": fold_number,
        "train_count": train_count,
        "test_count": test_count,
        "mae_days": round(float(errors.mean()), 2) if len(errors) else None,
        "median_error_days": round(float(errors.median()), 2) if len(errors) else None,
        "p75_error_days": round(float(errors.quantile(0.75)), 2) if len(errors) else None,
        "max_error_days": round(float(errors.max()), 2) if len(errors) else None,
        "improvement_vs_median_pct": None,
        "ready_for_eta": "false",
        "note": "Backtest error on merged PR cycle time; lower MAE is better.",
    }


def forecast_backtest_metrics(backtest: pd.DataFrame) -> dict[str, Any]:
    if backtest.empty or "mae_days" not in backtest.columns:
        return {}
    metrics: dict[str, Any] = {}
    for evaluation in ["kfold", "chronological_holdout"]:
        scored = backtest[(backtest["evaluation"] == evaluation) & backtest["mae_days"].notna()].copy()
        if scored.empty:
            continue
        grouped = scored.groupby("model")["mae_days"].mean()
        for model_name, mae in grouped.items():
            metrics[f"{model_name}_{evaluation}_mae"] = float(mae)
        best_model = grouped.sort_values().index[0] if not grouped.empty else ""
        metrics[f"best_{evaluation}_model"] = best_model
    return metrics


def source_event_as_of_backtest_metrics(backtest: pd.DataFrame, event_snapshots: pd.DataFrame | None) -> dict[str, str]:
    subject_count, training_example_count = source_event_as_of_example_counts(event_snapshots)
    if event_snapshots is None or event_snapshots.empty:
        return {
            "state": "missing",
            "subject_count": "0",
            "training_example_count": "0",
        }
    metrics: dict[str, Any] = {}
    if not backtest.empty and "mae_days" in backtest.columns:
        for source_eval, metric_eval in [
            ("source_event_as_of_kfold", "kfold"),
            ("source_event_as_of_chronological_holdout", "chronological_holdout"),
        ]:
            scored = backtest[(backtest["evaluation"] == source_eval) & backtest["mae_days"].notna()].copy()
            if scored.empty:
                continue
            grouped = scored.groupby("model")["mae_days"].mean()
            for model_name, mae in grouped.items():
                metrics[f"{model_name}_{metric_eval}_mae"] = float(mae)
    ready_model = forecast_eta_ready_model(metrics)
    state = "baseline_available" if metrics else "insufficient_sample"
    return {
        "state": state,
        "subject_count": str(subject_count),
        "training_example_count": str(training_example_count),
        "ready_model": ready_model,
        "median_cycle_baseline_kfold_mae": format_optional_float(metrics.get("median_cycle_baseline_kfold_mae")),
        "author_history_median_cycle_kfold_mae": format_optional_float(metrics.get("author_history_median_cycle_kfold_mae")),
        "gradient_boosting_absolute_error_kfold_mae": format_optional_float(metrics.get("gradient_boosting_absolute_error_kfold_mae")),
        "hist_gradient_boosting_absolute_error_kfold_mae": format_optional_float(metrics.get("hist_gradient_boosting_absolute_error_kfold_mae")),
        "random_forest_regressor_kfold_mae": format_optional_float(metrics.get("random_forest_regressor_kfold_mae")),
    }


def source_event_as_of_example_counts(event_snapshots: pd.DataFrame | None) -> tuple[int, int]:
    if event_snapshots is None or event_snapshots.empty:
        return 0, 0
    examples = event_snapshots.copy()
    if "subject_key" not in examples.columns:
        examples["subject_key"] = examples.apply(
            lambda row: f"{clean_text(row.get('repository', ''))}#{clean_int(row.get('pr_number', None)) or ''}",
            axis=1,
        )
    examples["cycle_time_days"] = pd.to_numeric(examples.get("cycle_time_days"), errors="coerce")
    examples["_observed_at_dt"] = examples.get("event_replay_observed_at", pd.Series("", index=examples.index)).map(parse_dt)
    merged_at_values = examples.get("merged_at", pd.Series("", index=examples.index)).fillna("")
    closed_at_values = examples.get("closed_at", pd.Series("", index=examples.index)).fillna("")
    examples["_terminal_at_dt"] = merged_at_values.mask(merged_at_values == "", closed_at_values).map(parse_dt)
    if "is_merged" in examples.columns:
        examples = examples[pd.to_numeric(examples["is_merged"], errors="coerce").fillna(0).astype(int) == 1]
    examples = examples[
        examples["subject_key"].map(clean_text).ne("")
        & examples["cycle_time_days"].notna()
        & examples["_observed_at_dt"].notna()
        & (examples["_terminal_at_dt"].isna() | (examples["_observed_at_dt"] < examples["_terminal_at_dt"]))
    ].copy()
    return int(examples["subject_key"].nunique()) if not examples.empty else 0, int(len(examples))


def forecast_readiness_diagnostic_rows(
    backtest_metrics: dict[str, Any],
    merged_count: int,
    *,
    temporal_feature_snapshot_ready: bool = False,
) -> list[dict[str, str]]:
    ready_model = forecast_eta_ready_model(backtest_metrics)
    best_candidate_model = ready_model or forecast_eta_best_candidate_model(backtest_metrics)
    best_kfold_model = str(backtest_metrics.get("best_kfold_model", "") or forecast_best_model_for_evaluation(backtest_metrics, "kfold"))
    best_chronological_model = str(
        backtest_metrics.get("best_chronological_holdout_model", "")
        or forecast_best_model_for_evaluation(backtest_metrics, "chronological_holdout")
    )
    model_ready = bool(ready_model)
    eta_ready = forecast_eta_ready(backtest_metrics, temporal_feature_snapshot_ready=temporal_feature_snapshot_ready)
    best_candidate_kfold_improvement = forecast_eta_candidate_improvement(backtest_metrics, best_candidate_model, "kfold")
    best_candidate_chrono_improvement = forecast_eta_candidate_improvement(backtest_metrics, best_candidate_model, "chronological_holdout")
    kfold_improvement = improvement_pct(
        backtest_metrics.get("median_cycle_baseline_kfold_mae"),
        backtest_metrics.get("random_forest_regressor_kfold_mae"),
    )
    chrono_improvement = improvement_pct(
        backtest_metrics.get("median_cycle_baseline_chronological_holdout_mae"),
        backtest_metrics.get("random_forest_regressor_chronological_holdout_mae"),
    )
    author_kfold_improvement = improvement_pct(
        backtest_metrics.get("median_cycle_baseline_kfold_mae"),
        backtest_metrics.get("author_history_median_cycle_kfold_mae"),
    )
    author_chrono_improvement = improvement_pct(
        backtest_metrics.get("median_cycle_baseline_chronological_holdout_mae"),
        backtest_metrics.get("author_history_median_cycle_chronological_holdout_mae"),
    )
    blockers: list[str] = []
    if merged_count < 10:
        blockers.append("insufficient_merged_pr_sample")
    elif not model_ready:
        if best_candidate_kfold_improvement is None or best_candidate_kfold_improvement < 10.0:
            blockers.append("kfold_model_does_not_beat_baseline")
        if best_candidate_chrono_improvement is None or best_candidate_chrono_improvement < 10.0:
            blockers.append("chronological_holdout_model_does_not_beat_baseline")
    if not temporal_feature_snapshot_ready:
        blockers.append("as_of_feature_snapshot_series_missing")
    primary = blockers[0] if blockers else "none"
    snapshot_state = "as_of_feature_snapshot_series_ready" if temporal_feature_snapshot_ready else "as_of_feature_snapshot_series_missing"
    if eta_ready:
        next_evidence = "monitor_eta_drift_against_observed_outcomes"
    elif not temporal_feature_snapshot_ready:
        next_evidence = "collect_repeated_as_of_pr_snapshots_and_closed_outcomes"
    elif not model_ready:
        next_evidence = "improve_model_features_and_validate_against_as_of_snapshots"
    else:
        next_evidence = "clear_remaining_forecast_readiness_blockers"
    return [
        {
            "metric": "eta_readiness_state",
            "value": "ready" if eta_ready else "blocked",
            "note": "readiness state for ETA commitments after baseline, chronological holdout, and snapshot-series checks",
        },
        {
            "metric": "eta_model_backtest_ready",
            "value": "true" if model_ready else "false",
            "note": "true when at least one candidate model beats median and heuristic baselines on K-fold and chronological holdout; temporal as-of feature snapshots are checked separately",
        },
        {
            "metric": "eta_ready_model",
            "value": ready_model,
            "note": "candidate model that cleared both K-fold and chronological ETA backtest gates, if any",
        },
        {
            "metric": "eta_best_kfold_model",
            "value": best_kfold_model,
            "note": "lowest-MAE K-fold row across models and baselines; diagnostic only and not enough for ETA readiness by itself",
        },
        {
            "metric": "eta_best_chronological_model",
            "value": best_chronological_model,
            "note": "lowest-MAE chronological-holdout row across models and baselines; diagnostic only and not enough for ETA readiness by itself",
        },
        {
            "metric": "eta_same_model_backtest_gate",
            "value": "passed" if model_ready else "gated",
            "note": "ETA requires one candidate model to beat median and heuristic baselines on both K-fold and chronological holdout",
        },
        {
            "metric": "eta_best_candidate_model",
            "value": best_candidate_model,
            "note": "lowest K-fold MAE candidate among ETA models; diagnostic only unless ETA gates pass",
        },
        {
            "metric": "eta_primary_blocker",
            "value": primary,
            "note": "first failed ETA readiness condition; risk ranking can still be useful when this is not none",
        },
        {
            "metric": "eta_blocker_count",
            "value": str(len(blockers)),
            "note": "count of failed ETA readiness conditions represented in forecast summary diagnostics",
        },
        {
            "metric": "eta_kfold_random_forest_improvement_pct",
            "value": format_optional_float(kfold_improvement),
            "note": "random-forest K-fold MAE improvement over the median-cycle baseline; ETA requires at least 10 percent",
        },
        {
            "metric": "eta_chronological_random_forest_improvement_pct",
            "value": format_optional_float(chrono_improvement),
            "note": "random-forest chronological-holdout MAE improvement over the median-cycle baseline; ETA requires at least 10 percent",
        },
        {
            "metric": "eta_kfold_best_candidate_improvement_pct",
            "value": format_optional_float(best_candidate_kfold_improvement),
            "note": "best ETA candidate K-fold MAE improvement over the median-cycle baseline; ETA requires at least 10 percent",
        },
        {
            "metric": "eta_chronological_best_candidate_improvement_pct",
            "value": format_optional_float(best_candidate_chrono_improvement),
            "note": "best ETA candidate chronological-holdout MAE improvement over the median-cycle baseline; ETA requires at least 10 percent",
        },
        {
            "metric": "eta_kfold_author_history_improvement_pct",
            "value": format_optional_float(author_kfold_improvement),
            "note": "author-history median-cycle K-fold MAE improvement over the median-cycle baseline; ETA requires at least 10 percent",
        },
        {
            "metric": "eta_chronological_author_history_improvement_pct",
            "value": format_optional_float(author_chrono_improvement),
            "note": "author-history median-cycle chronological-holdout MAE improvement over the median-cycle baseline; ETA requires at least 10 percent",
        },
        {
            "metric": "eta_temporal_snapshot_state",
            "value": snapshot_state,
            "note": "reliable remaining-time ETA needs repeated as-of feature snapshots; latest or terminal PR feature rows are risk-triage inputs only",
        },
        {
            "metric": "eta_next_evidence_needed",
            "value": next_evidence,
            "note": "next data product needed before forecast output can graduate from risk triage to ETA commitment",
        },
    ]


def build_forecast_reliability(forecast_summary: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "forecast_product",
        "readiness_state",
        "product_safe",
        "safe_use",
        "best_model",
        "primary_metric",
        "metric_value",
        "next_evidence",
        "guardrail",
    ]
    metrics = metric_map(forecast_summary)
    if not metrics:
        return pd.DataFrame(
            [
                forecast_reliability_row(
                    "point_eta",
                    "missing_forecast_summary",
                    False,
                    "none",
                    "",
                    "",
                    "",
                    "run_forecast_backtests",
                    "No forecast summary exists, so Cubicle cannot make point ETA, range ETA, or risk-triage claims.",
                )
            ],
            columns=columns,
        )

    eta_ready = metrics.get("eta_forecast_ready", "").lower() == "true"
    eta_model = metrics.get("eta_ready_model") or metrics.get("eta_best_candidate_model", "")
    eta_metric = metrics.get("eta_kfold_best_candidate_improvement_pct", "")
    eta_blocker = metrics.get("eta_primary_blocker", "") or "none"
    rows = [
        forecast_reliability_row(
            "point_eta",
            "ready" if eta_ready else "gated",
            eta_ready,
            "date_or_remaining_day_commitment" if eta_ready else "diagnostic_only",
            eta_model,
            "eta_kfold_best_candidate_improvement_pct",
            eta_metric,
            "monitor_eta_drift_against_observed_outcomes" if eta_ready else metrics.get("eta_next_evidence_needed", "improve_model_features"),
            "Point ETA is product-safe only when the same candidate beats median and heuristic baselines on K-fold and chronological holdout, and as-of snapshots are ready."
            if eta_ready
            else f"Point ETA is blocked by {eta_blocker}; use forecast outputs for risk triage only.",
        )
    ]

    range_model, range_mae = best_range_baseline(metrics)
    range_available = bool(range_model)
    rows.append(
        forecast_reliability_row(
            "range_eta",
            "diagnostic_only" if range_available else "missing_baseline",
            False,
            "wide_range_context" if range_available else "none",
            range_model,
            "best_remaining_time_mae_days" if range_available else "",
            format_optional_float(range_mae) if range_available else "",
            "calibrate_prediction_intervals_against_live_as_of_snapshots",
            "Remaining-time baselines can explain uncertainty, but they are not product-safe ETA ranges until interval coverage and width are validated on live as-of snapshots."
            if range_available
            else "No lifecycle or survival remaining-time baseline is available for range-style forecast diagnostics.",
        )
    )

    risk_lift = safe_float(metrics.get("risk_triage_lift_at_10pct"))
    risk_state = metrics.get("risk_triage_coverage_stratified_state", "")
    risk_ready = risk_lift is not None and risk_lift >= 0.10
    if risk_ready and risk_state == "stratified":
        triage_state = "ready"
    elif risk_ready:
        triage_state = "ready_with_coverage_guardrail"
    else:
        triage_state = "gated"
    rows.append(
        forecast_reliability_row(
            "risk_triage",
            triage_state,
            risk_ready,
            "attention_ordering" if risk_ready else "diagnostic_only",
            "static_risk_triage_score",
            "risk_triage_lift_at_10pct",
            "" if risk_lift is None else f"{risk_lift:.4f}",
            "add_more_coverage_strata_for_confounding_checks" if risk_ready and risk_state != "stratified" else "collect_more_slow_cycle_outcomes",
            "Risk triage is product-safe only for attention ordering; it is not an ETA, causality, blocker, or autonomous action claim.",
        )
    )
    return pd.DataFrame(rows, columns=columns)


def best_range_baseline(metrics: dict[str, str]) -> tuple[str, float | None]:
    candidates = [
        ("lifecycle_as_of:" + metrics.get("lifecycle_as_of_best_model", ""), metrics.get("lifecycle_as_of_best_mae_days")),
        ("survival_time_to_merge:" + metrics.get("survival_time_to_merge_best_model", ""), metrics.get("survival_time_to_merge_best_mae_days")),
    ]
    scored: list[tuple[float, str]] = []
    for model_name, value in candidates:
        mae = safe_float(value)
        if not model_name.endswith(":") and mae is not None:
            scored.append((mae, model_name))
    if not scored:
        return "", None
    mae, model_name = sorted(scored)[0]
    return model_name, mae


def forecast_reliability_row(
    forecast_product: str,
    readiness_state: str,
    product_safe: bool,
    safe_use: str,
    best_model: str,
    primary_metric: str,
    metric_value: str,
    next_evidence: str,
    guardrail: str,
) -> dict[str, Any]:
    return {
        "forecast_product": forecast_product,
        "readiness_state": readiness_state,
        "product_safe": "true" if product_safe else "false",
        "safe_use": safe_use,
        "best_model": best_model,
        "primary_metric": primary_metric,
        "metric_value": metric_value,
        "next_evidence": next_evidence,
        "guardrail": guardrail,
    }


def build_forecast_risk_backtest(merged: pd.DataFrame) -> pd.DataFrame:
    columns = ["metric", "value", "sample_count", "method", "interpretation", "guardrail"]
    guardrail = (
        "Risk-triage validation ranks historical merged PRs by static PR metadata. "
        "It is useful for TPM attention ordering, not ETA commitments, causality, or current blocker claims."
    )
    if len(merged) < 20:
        return pd.DataFrame(
            [
                {
                    "metric": "risk_triage_backtest_state",
                    "value": "insufficient_sample",
                    "sample_count": len(merged),
                    "method": "historical merged PR slow-cycle ranking",
                    "interpretation": "Need at least 20 merged PRs to validate slow-cycle risk triage.",
                    "guardrail": guardrail,
                }
            ],
            columns=columns,
        )
    rows = merged.copy()
    rows["cycle_time_days"] = pd.to_numeric(rows["cycle_time_days"], errors="coerce")
    rows = rows.dropna(subset=["cycle_time_days"])
    if rows.empty:
        return pd.DataFrame(columns=columns)
    slow_threshold = float(rows["cycle_time_days"].quantile(0.75))
    rows["slow_cycle_label"] = rows["cycle_time_days"] >= slow_threshold
    rows["static_risk_triage_score"] = rows.apply(static_risk_triage_score, axis=1)
    baseline_rate = float(rows["slow_cycle_label"].mean())

    metrics: list[dict[str, Any]] = [
        risk_backtest_metric("sample_count", len(rows), len(rows), "historical merged PR rows", "Rows available for slow-cycle risk validation.", guardrail),
        risk_backtest_metric("slow_cycle_threshold_days", slow_threshold, len(rows), "75th percentile merged cycle time", "Threshold used to label historical slow-cycle PRs.", guardrail),
        risk_backtest_metric("slow_cycle_base_rate", baseline_rate, len(rows), "slow-cycle labels / merged PR sample", "Base rate for precision/lift comparison.", guardrail),
    ]
    for fraction in [0.1, 0.2, 0.25]:
        top, label = top_fraction(rows, fraction, "static_risk_triage_score")
        precision = float(top["slow_cycle_label"].mean()) if not top.empty else 0.0
        metrics.append(
            risk_backtest_metric(
                f"precision_at_{label}",
                precision,
                len(top),
                f"top {label} by static risk-triage score",
                "Share of selected historical PRs that were slow-cycle.",
                guardrail,
            )
        )
        metrics.append(
            risk_backtest_metric(
                f"lift_vs_baseline_at_{label}",
                precision - baseline_rate,
                len(top),
                f"top {label} precision minus slow-cycle base rate",
                "Positive lift means risk ranking enriches for slow-cycle PRs.",
                guardrail,
            )
        )
    metrics.extend(build_coverage_stratified_risk_metrics(rows, guardrail))
    corr_value, sample_count, state = rank_correlation(rows, "static_risk_triage_score", "cycle_time_days")
    metrics.append(
        risk_backtest_metric(
            "spearman_static_risk_score_vs_cycle_time",
            corr_value,
            sample_count,
            "rank correlation over historical merged PRs",
            f"Positive values mean higher static triage scores co-occur with longer merged cycle times. {state}",
            guardrail,
        )
    )
    high, _ = top_fraction(rows, 0.25, "static_risk_triage_score")
    rest = rows.drop(index=high.index)
    metrics.append(
        risk_backtest_metric(
            "top_quartile_cycle_time_lift_days",
            mean_difference(high, rest, "cycle_time_days"),
            len(rows),
            "mean cycle time in top risk quartile minus rest",
            "Positive values mean the top risk quartile had longer historical cycles.",
            guardrail,
        )
    )
    return pd.DataFrame(metrics, columns=columns)


def build_coverage_stratified_risk_metrics(rows: pd.DataFrame, guardrail: str) -> list[dict[str, Any]]:
    if rows.empty:
        return []
    data = rows.copy()
    data["coverage_stratum"] = data.apply(risk_backtest_coverage_stratum, axis=1)
    stratum_count = int(data["coverage_stratum"].nunique())
    global_top, _ = top_fraction(data, 0.1, "static_risk_triage_score")
    global_baseline = float(data["slow_cycle_label"].mean())
    global_precision = float(global_top["slow_cycle_label"].mean()) if not global_top.empty else 0.0
    global_lift = global_precision - global_baseline
    eligible: dict[str, pd.DataFrame] = {}
    stratum_lifts: dict[str, float] = {}
    for stratum, group in data.groupby("coverage_stratum"):
        if len(group) < MIN_RISK_BACKTEST_COVERAGE_STRATUM_SAMPLE:
            continue
        eligible[stratum] = group
        baseline_rate = float(group["slow_cycle_label"].mean())
        top, _ = top_fraction(group, 0.1, "static_risk_triage_score")
        precision = float(top["slow_cycle_label"].mean()) if not top.empty else 0.0
        stratum_lifts[stratum] = precision - baseline_rate
    if stratum_count <= 1:
        state = "not_testable_single_stratum"
        interpretation = "Historical merged PR risk validation has one source coverage/provenance stratum; coverage confounding cannot be tested from this sample."
    elif eligible and global_lift > 0.02 and max(stratum_lifts.values()) <= 0.01:
        state = "confounded"
        interpretation = "Global slow-cycle lift is positive but eligible source coverage/provenance strata do not show positive within-stratum lift; treat ranking as coverage-confounded."
    elif eligible:
        state = "stratified"
        interpretation = "Historical merged PR risk validation includes multiple source coverage/provenance strata with enough sample for within-stratum precision checks."
    else:
        state = "insufficient_stratum_sample"
        interpretation = "Historical merged PR risk validation has multiple source coverage/provenance strata, but each stratum is too sparse for reliable within-stratum precision."
    metrics = [
        risk_backtest_metric(
            "coverage_stratum_count",
            stratum_count,
            len(data),
            "distinct current source coverage/provenance strata",
            "Counts coverage/detail/provenance states in the historical merged risk backtest sample.",
            guardrail,
        ),
        risk_backtest_metric(
            "coverage_stratified_backtest_state",
            state,
            len(data),
            "coverage-stratified risk backtest readiness",
            interpretation,
            guardrail,
        ),
    ]
    if eligible:
        weighted_lift = sum(stratum_lifts[stratum] * len(group) for stratum, group in eligible.items()) / sum(
            len(group) for group in eligible.values()
        )
        metrics.extend(
            [
                risk_backtest_metric(
                    "coverage_stratified_max_lift_at_10pct",
                    max(stratum_lifts.values()),
                    sum(len(group) for group in eligible.values()),
                    "maximum within-stratum top 10pct lift",
                    "Maximum slow-cycle lift after holding source coverage/provenance stratum constant.",
                    guardrail,
                ),
                risk_backtest_metric(
                    "coverage_stratified_weighted_lift_at_10pct",
                    weighted_lift,
                    sum(len(group) for group in eligible.values()),
                    "sample-weighted within-stratum top 10pct lift",
                    "Sample-weighted slow-cycle lift after holding source coverage/provenance stratum constant.",
                    guardrail,
                ),
            ]
        )
    for stratum, group in sorted(data.groupby("coverage_stratum"), key=lambda item: (-len(item[1]), item[0])):
        slug = risk_backtest_coverage_slug(stratum)
        metrics.append(
            risk_backtest_metric(
                f"coverage_stratum_{slug}_sample_count",
                len(group),
                len(group),
                f"historical merged PR rows in {stratum}",
                "Rows available for this source-coverage stratum.",
                guardrail,
            )
        )
        if len(group) < MIN_RISK_BACKTEST_COVERAGE_STRATUM_SAMPLE:
            metrics.append(
                risk_backtest_metric(
                    f"coverage_stratum_{slug}_state",
                    "insufficient_sample",
                    len(group),
                    f"coverage-stratified precision in {stratum}",
                    f"Need at least {MIN_RISK_BACKTEST_COVERAGE_STRATUM_SAMPLE} rows for within-stratum slow-cycle precision.",
                    guardrail,
                )
            )
            continue
        baseline_rate = float(group["slow_cycle_label"].mean())
        top, label = top_fraction(group, 0.1, "static_risk_triage_score")
        precision = float(top["slow_cycle_label"].mean()) if not top.empty else 0.0
        metrics.extend(
            [
                risk_backtest_metric(
                    f"coverage_stratum_{slug}_slow_cycle_base_rate",
                    baseline_rate,
                    len(group),
                    f"slow-cycle labels in {stratum}",
                    "Base rate for within-stratum precision/lift comparison.",
                    guardrail,
                ),
                risk_backtest_metric(
                    f"coverage_stratum_{slug}_precision_at_{label}",
                    precision,
                    len(top),
                    f"top {label} by static risk score within {stratum}",
                    "Within-stratum share of selected historical PRs that were slow-cycle.",
                    guardrail,
                ),
                risk_backtest_metric(
                    f"coverage_stratum_{slug}_lift_vs_baseline_at_{label}",
                    precision - baseline_rate,
                    len(top),
                    f"top {label} precision minus stratum slow-cycle base rate",
                    "Positive lift means risk ranking enriches for slow-cycle PRs after holding source coverage constant.",
                    guardrail,
                ),
            ]
        )
    return metrics


def risk_backtest_coverage_stratum(row: pd.Series) -> str:
    coverage = clean_text(row.get("source_current_coverage_state")) or "unknown"
    detail = clean_text(row.get("source_current_detail_state")) or "unknown"
    lifecycle = clean_text(row.get("lifecycle_fields_source")) or "unknown"
    churn = clean_text(row.get("churn_fields_source")) or "unknown"
    mergeability = clean_text(row.get("mergeability_fields_source")) or "unknown"
    mode = clean_text(row.get("source_current_coverage_mode")) or "unknown"
    return f"coverage={coverage};detail={detail};mode={mode};lifecycle={lifecycle};churn={churn};mergeability={mergeability}"


def risk_backtest_coverage_slug(stratum: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "_", stratum.lower()).strip("_")
    return slug[:96] or "unknown"


def static_risk_triage_score(row: pd.Series) -> float:
    lines = float(row.get("total_lines_changed") or 0)
    comments = float(row.get("comments") or 0) + float(row.get("review_comments") or 0)
    linked = float(row.get("linked_ticket_count") or 0)
    requested_reviewers = float(row.get("requested_reviewer_count") or 0)
    review_wait = safe_float(row.get("days_since_review_activity")) or 0.0
    draft = 1.0 if clean_bool(row.get("draft")) else 0.0
    score = 0.0
    score += min(lines, 5000.0) / 5000.0 * 25.0
    score += min(comments, 30.0) / 30.0 * 25.0
    score += min(linked, 5.0) / 5.0 * 15.0
    score += min(requested_reviewers, 5.0) / 5.0 * 15.0
    score += min(review_wait, 30.0) / 30.0 * 10.0
    score += draft * 10.0
    return round(min(score, 100.0), 4)


def top_fraction(df: pd.DataFrame, fraction: float, score_column: str) -> tuple[pd.DataFrame, str]:
    label = f"{int(fraction * 100)}pct"
    if df.empty or score_column not in df.columns:
        return pd.DataFrame(), label
    count = max(1, int(math.ceil(len(df) * fraction)))
    rows = df.copy()
    if "pr_number" in rows.columns:
        rows["_risk_tiebreaker"] = pd.to_numeric(rows["pr_number"], errors="coerce")
    else:
        rows["_risk_tiebreaker"] = range(len(rows))
    top = rows.sort_values([score_column, "_risk_tiebreaker"], ascending=[False, True], na_position="last").head(count).copy()
    return top.drop(columns=["_risk_tiebreaker"]), label


def risk_backtest_metric(metric: str, value: Any, sample_count: int, method: str, interpretation: str, guardrail: str) -> dict[str, Any]:
    if isinstance(value, float):
        clean_value: Any = round(value, 4)
    else:
        clean_value = value
    return {
        "metric": metric,
        "value": str(clean_value),
        "sample_count": int(sample_count),
        "method": method,
        "interpretation": interpretation,
        "guardrail": guardrail,
    }


ETA_READY_MODEL_CANDIDATES = [
    "gradient_boosting_absolute_error",
    "hist_gradient_boosting_absolute_error",
    "random_forest_regressor",
    "author_history_median_cycle",
]


def forecast_eta_ready_model(metrics: dict[str, Any]) -> str:
    scored: list[tuple[float, float, float, int, str]] = []
    for model_name in ETA_READY_MODEL_CANDIDATES:
        if not forecast_eta_model_candidate_ready(metrics, model_name):
            continue
        kfold_mae = safe_float(metrics.get(f"{model_name}_kfold_mae"))
        chrono_mae = safe_float(metrics.get(f"{model_name}_chronological_holdout_mae"))
        if kfold_mae is None or chrono_mae is None:
            continue
        scored.append((kfold_mae + chrono_mae, kfold_mae, chrono_mae, ETA_READY_MODEL_CANDIDATES.index(model_name), model_name))
    if not scored:
        return ""
    return sorted(scored)[0][4]


def forecast_eta_best_candidate_model(metrics: dict[str, Any]) -> str:
    scored: list[tuple[float, str]] = []
    for model_name in ETA_READY_MODEL_CANDIDATES:
        model_mae = safe_float(metrics.get(f"{model_name}_kfold_mae"))
        chrono_mae = safe_float(metrics.get(f"{model_name}_chronological_holdout_mae"))
        if model_mae is None or chrono_mae is None:
            continue
        scored.append((model_mae, model_name))
    if not scored:
        return ""
    return sorted(scored, key=lambda item: (item[0], ETA_READY_MODEL_CANDIDATES.index(item[1])))[0][1]


def forecast_eta_candidate_improvement(metrics: dict[str, Any], model_name: str, evaluation: str) -> float | None:
    if not model_name:
        return None
    return improvement_pct(
        metrics.get(f"median_cycle_baseline_{evaluation}_mae"),
        metrics.get(f"{model_name}_{evaluation}_mae"),
    )


def forecast_eta_model_candidate_ready(metrics: dict[str, Any], model_name: str) -> bool:
    model_mae = metrics.get(f"{model_name}_kfold_mae")
    median_mae = metrics.get("median_cycle_baseline_kfold_mae")
    heuristic_mae = metrics.get("heuristic_percentile_kfold_mae")
    model_chrono_mae = metrics.get(f"{model_name}_chronological_holdout_mae")
    median_chrono_mae = metrics.get("median_cycle_baseline_chronological_holdout_mae")
    heuristic_chrono_mae = metrics.get("heuristic_percentile_chronological_holdout_mae")
    return (
        model_mae is not None
        and median_mae is not None
        and heuristic_mae is not None
        and model_chrono_mae is not None
        and median_chrono_mae is not None
        and heuristic_chrono_mae is not None
        and model_mae <= median_mae * 0.9
        and model_mae <= heuristic_mae * 0.9
        and model_chrono_mae <= median_chrono_mae * 0.9
        and model_chrono_mae <= heuristic_chrono_mae * 0.9
    )


def forecast_eta_model_backtest_ready(metrics: dict[str, Any]) -> bool:
    return bool(forecast_eta_ready_model(metrics))


def forecast_eta_ready(metrics: dict[str, Any], *, temporal_feature_snapshot_ready: bool = False) -> bool:
    return forecast_eta_model_backtest_ready(metrics) and temporal_feature_snapshot_ready


def forecast_best_model_for_evaluation(metrics: dict[str, Any], evaluation: str) -> str:
    suffix = f"_{evaluation}_mae"
    scored: list[tuple[float, str]] = []
    for key, value in metrics.items():
        if not str(key).endswith(suffix):
            continue
        mae = safe_float(value)
        if mae is None:
            continue
        scored.append((mae, str(key)[: -len(suffix)]))
    if not scored:
        return ""
    return sorted(scored, key=lambda item: (item[0], item[1]))[0][1]


def author_history_cycle_prediction(pr_features: pd.DataFrame, fallback: pd.Series | float) -> pd.Series:
    fallback_series = fallback if isinstance(fallback, pd.Series) else pd.Series([float(fallback)] * len(pr_features), index=pr_features.index)
    if "author_prior_median_cycle_days" not in pr_features.columns:
        return fallback_series
    values = pd.to_numeric(pr_features["author_prior_median_cycle_days"], errors="coerce")
    return values.where(values > 0, fallback_series).fillna(fallback_series)


def heuristic_cycle_prediction(pr_features: pd.DataFrame, median_cycle: float, p75_cycle: float) -> pd.Series:
    size = pd.to_numeric(pr_features.get("total_lines_changed", pd.Series(0, index=pr_features.index)), errors="coerce").fillna(0)
    comments = pd.to_numeric(pr_features.get("comments", pd.Series(0, index=pr_features.index)), errors="coerce").fillna(0)
    review_comments = pd.to_numeric(pr_features.get("review_comments", pd.Series(0, index=pr_features.index)), errors="coerce").fillna(0)
    linked_tickets = pd.to_numeric(pr_features.get("linked_ticket_count", pd.Series(0, index=pr_features.index)), errors="coerce").fillna(0)
    requested_reviewers = pd.to_numeric(pr_features.get("requested_reviewer_count", pd.Series(0, index=pr_features.index)), errors="coerce").fillna(0)
    stale_days = pd.to_numeric(pr_features.get("stale_days", pd.Series(0, index=pr_features.index)), errors="coerce").fillna(0)
    size_factor = (size.clip(upper=5000) / 5000.0) * max(p75_cycle - median_cycle, 0)
    discussion_factor = (comments + review_comments).clip(upper=20) * 0.4
    multi_ticket_factor = linked_tickets.clip(lower=1) * 1.5
    review_wait_factor = requested_reviewers.clip(upper=5) * 1.2
    staleness_factor = stale_days.clip(upper=60) * 0.1
    return median_cycle + size_factor + discussion_factor + multi_ticket_factor + review_wait_factor + staleness_factor


def build_dependency_edges(ticket_pr_edges: pd.DataFrame) -> pd.DataFrame:
    graph = nx.Graph()
    rows: list[dict[str, Any]] = []
    for edge in ticket_pr_edges.itertuples(index=False):
        ticket_node = f"ticket:{edge.ticket_key}"
        pr_node = f"pr:{edge.repository}#{edge.pr_number}"
        graph.add_edge(ticket_node, pr_node)
        rows.append(
            {
                "edge_kind": "ticket_pr",
                "source_key": ticket_node,
                "target_key": pr_node,
                "freshness": edge.edge_freshness,
                "risk_signal": "partial_remote_link" if edge.pr_freshness == "partial" else "",
            }
        )
    for component_id, component in enumerate(nx.connected_components(graph), start=1):
        if len(component) <= 2:
            continue
        for node in component:
            rows.append(
                {
                    "edge_kind": "workstream_component",
                    "source_key": f"component:{component_id}",
                    "target_key": node,
                    "freshness": "",
                    "risk_signal": "multi_object_workstream",
                }
            )
    return pd.DataFrame(rows)


def build_review_bottlenecks(pr_forecasts: pd.DataFrame) -> pd.DataFrame:
    if pr_forecasts.empty:
        return pd.DataFrame(
            columns=[
                "repository",
                "pr_number",
                "pr_url",
                "title",
                "age_days",
                "stale_days",
                "requested_reviewer_count",
                "requested_reviewers",
                "latest_review_activity_at",
                "risk_score",
                "severity",
                "bottleneck_score",
                "bottleneck_reason",
                "evidence_excerpt",
                "evidence_source_system",
                "evidence_source_instance",
                "evidence_external_kind",
                "evidence_external_id",
                "evidence_source_url",
                "evidence_locator_kind",
                "evidence_locator",
                "evidence_source_span_key",
                "evidence_span_start",
                "evidence_span_end",
            ]
        )
    rows = []
    open_prs = pr_forecasts[(pr_forecasts["state"] == "open") & (pr_forecasts["requested_reviewer_count"].fillna(0) > 0)].copy()
    for row in open_prs.itertuples(index=False):
        age_days = float(row.age_days or 0)
        stale_days = float(row.stale_days or 0)
        requested_count = int(row.requested_reviewer_count or 0)
        score = min(
            100.0,
            30.0
            + requested_count * 12.0
            + min(age_days, 90.0) * 0.4
            + min(stale_days, 30.0) * 1.0,
        )
        severity = "medium" if score >= 60 else "low"
        rows.append(
            {
                "repository": row.repository,
                "pr_number": int(row.pr_number),
                "pr_url": row.pr_url,
                "title": row.title,
                "age_days": age_days,
                "stale_days": stale_days,
                "requested_reviewer_count": requested_count,
                "requested_reviewers": row.requested_reviewers,
                "latest_review_activity_at": row.latest_review_activity_at,
                "risk_score": int(row.risk_score),
                "severity": severity,
                "bottleneck_score": round(score, 2),
                "bottleneck_reason": (
                    f"{requested_count} requested reviewer(s); PR age {format_days(age_days)}, "
                    f"source stale {format_days(stale_days)}. Treat as a requested-reviewer-still-listed lead; review-request event time is not modeled yet."
                ),
                "evidence_excerpt": row.review_evidence_excerpt,
                "evidence_source_system": row.review_evidence_source_system,
                "evidence_source_instance": row.review_evidence_source_instance,
                "evidence_external_kind": row.review_evidence_external_kind,
                "evidence_external_id": row.review_evidence_external_id,
                "evidence_source_url": row.review_evidence_source_url,
                "evidence_locator_kind": row.review_evidence_locator_kind,
                "evidence_locator": row.review_evidence_locator,
                "evidence_source_span_key": row.review_evidence_source_span_key,
                "evidence_span_start": row.review_evidence_span_start,
                "evidence_span_end": row.review_evidence_span_end,
            }
        )
    out = pd.DataFrame(rows)
    if out.empty:
        return pd.DataFrame(
            columns=[
                "repository",
                "pr_number",
                "pr_url",
                "title",
                "age_days",
                "stale_days",
                "requested_reviewer_count",
                "requested_reviewers",
                "latest_review_activity_at",
                "risk_score",
                "severity",
                "bottleneck_score",
                "bottleneck_reason",
                "evidence_excerpt",
                "evidence_source_system",
                "evidence_source_instance",
                "evidence_external_kind",
                "evidence_external_id",
                "evidence_source_url",
                "evidence_locator_kind",
                "evidence_locator",
                "evidence_source_span_key",
                "evidence_span_start",
                "evidence_span_end",
            ]
        )
    return out.sort_values(["bottleneck_score", "risk_score", "age_days"], ascending=[False, False, False])


def build_insight_cards(
    pr_forecasts: pd.DataFrame,
    ticket_features: pd.DataFrame,
    blocker_candidates: pd.DataFrame,
    dependency_edges: pd.DataFrame,
    review_bottlenecks: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    forecast_backtest: pd.DataFrame,
    developer_correlation: pd.DataFrame | None = None,
) -> pd.DataFrame:
    cards: list[dict[str, Any]] = []
    cards.extend(build_model_quality_cards(forecast_summary, forecast_backtest))
    cards.extend(build_source_coverage_cards(pr_forecasts))
    if developer_correlation is not None:
        cards.extend(build_developer_correlation_cards(developer_correlation))
    if not pr_forecasts.empty:
        eta_ready = metric_map(forecast_summary).get("eta_forecast_ready", "false") == "true"
        detail_state = pr_forecasts.get("source_current_detail_state", pd.Series("observed", index=pr_forecasts.index))
        risky = pr_forecasts[
            (pr_forecasts["state"] == "open")
            & (pr_forecasts["risk_band"].isin(["high", "critical"]))
            & (detail_state != "failed")
        ].sort_values(
            ["risk_score", "age_days"], ascending=[False, False]
        )
        for row in risky.head(10).itertuples(index=False):
            if eta_ready:
                details = (
                    f"Age {format_days(row.age_days)}, ETA model cycle estimate {format_days(row.predicted_total_cycle_days)}, "
                    f"remaining {format_days(row.predicted_remaining_days)}, risk score {int(row.risk_score)}."
                )
            else:
                details = (
                    f"Age {format_days(row.age_days)}, slow-cycle risk threshold {format_days(row.predicted_total_cycle_days)}, "
                    f"age past threshold {format_days(row.overdue_days)}, risk score {int(row.risk_score)}. "
                    "This is age/staleness triage, not an ETA."
                )
            cards.append(
                {
                    "insight_kind": "forecast_risk",
                    "severity": row.risk_band,
                    "subject_kind": "pull_request",
                    "subject_key": f"{row.repository}#{row.pr_number}",
                    "identity_key": "cycle_risk",
                    "source_url": row.pr_url,
                    "title": f"Open PR age/staleness risk: {row.title}",
                    "details": details,
                    "recommended_action": "TPM follow-up: confirm owner, decision needed, and review/merge path.",
                    "model_method": row.forecast_method,
                    "score": float(row.risk_score),
                    "score_explanation": "Risk score is heuristic triage unless model quality proves an ETA model beats baseline.",
                    "confidence": 0.45 if "rejected" in str(row.forecast_method) else 0.7,
                }
            )
    if not review_bottlenecks.empty:
        for row in review_bottlenecks.head(10).itertuples(index=False):
            subject_key = f"{row.repository}#{row.pr_number}"
            cards.append(
                {
                    "insight_kind": "status_summary",
                    "severity": row.severity,
                    "subject_kind": "pull_request",
                    "subject_key": subject_key,
                    "identity_key": "review_wait",
                    "source_url": row.pr_url,
                    "title": f"Requested reviewer still listed: {row.title}",
                    "details": row.bottleneck_reason,
                    "recommended_action": "TPM follow-up: confirm whether the listed reviewer is still expected, then record the review owner or merge/close decision.",
                    "model_method": "typed_review_relationship_rule",
                    "score": float(row.bottleneck_score),
                    "score_explanation": "Score combines open PR age, source staleness, and requested reviewer count; review-request event time is not modeled yet.",
                    "confidence": 0.5,
                    "evidence_excerpt": row.evidence_excerpt,
                    "evidence_source_system": row.evidence_source_system,
                    "evidence_source_instance": row.evidence_source_instance,
                    "evidence_external_kind": row.evidence_external_kind,
                    "evidence_external_id": row.evidence_external_id,
                    "evidence_source_url": row.evidence_source_url,
                    "evidence_locator_kind": row.evidence_locator_kind,
                    "evidence_locator": row.evidence_locator,
                    "evidence_source_span_key": row.evidence_source_span_key,
                    "evidence_span_start": row.evidence_span_start,
                    "evidence_span_end": row.evidence_span_end,
                    "evidence_excerpt_truncated": False,
                }
            )
    if not blocker_candidates.empty:
        current_blockers = blocker_candidates[blocker_candidates["candidate_scope"] == "current"].copy()
        if not current_blockers.empty:
            current_blockers["candidate_count"] = current_blockers.groupby("product_key")["product_key"].transform("count")
            current_blockers = current_blockers.sort_values(["severity", "created_at"], ascending=[False, False]).drop_duplicates("product_key")
        for row in current_blockers.head(10).itertuples(index=False):
            cards.append(
                {
                    "insight_kind": "blocker_candidate",
                    "severity": severity_label(row.severity),
                    "subject_kind": subject_kind_for(row.product_key),
                    "subject_key": row.product_key,
                    "identity_key": stable_digest(["blocker", row.signal, row.source_url]),
                    "source_url": row.source_url,
                    "title": f"Possible blocker signal: {row.signal}",
                    "details": f"{row.candidate_count} current keyword evidence span(s). Strongest excerpt: {row.evidence_excerpt}",
                    "recommended_action": "TPM follow-up: validate whether this is still blocking and record owner/next step.",
                    "model_method": "keyword_evidence_candidate",
                    "score": float(row.severity * 20),
                    "score_explanation": f"Keyword candidate severity {row.severity}/5; requires human validation.",
                    "confidence": 0.55,
                    "evidence_excerpt": row.evidence_excerpt,
                    "evidence_source_system": row.evidence_source_system,
                    "evidence_source_instance": row.evidence_source_instance,
                    "evidence_external_kind": row.evidence_external_kind,
                    "evidence_external_id": row.evidence_external_id,
                    "evidence_source_url": row.evidence_source_url,
                    "evidence_locator_kind": row.evidence_locator_kind,
                    "evidence_locator": row.evidence_locator,
                    "evidence_source_span_key": row.evidence_source_span_key,
                    "evidence_span_start": row.evidence_span_start,
                    "evidence_span_end": row.evidence_span_end,
                    "evidence_excerpt_truncated": row.evidence_excerpt_truncated,
                }
            )
    if not ticket_features.empty:
        current_tickets = ticket_features[ticket_features["status"].map(normalize_ticket_state) == "open"]
        multi_pr = current_tickets[current_tickets["linked_pr_count"] >= 3].sort_values("linked_pr_count", ascending=False)
        for row in multi_pr.head(10).itertuples(index=False):
            cards.append(
                {
                    "insight_kind": "dependency_cluster",
                    "severity": "medium",
                    "subject_kind": "ticket",
                    "subject_key": row.ticket_key,
                    "identity_key": "ticket_pr_component",
                    "source_url": "",
                    "title": f"Ticket spans {row.linked_pr_count} PRs: {row.title}",
                    "details": f"{row.partial_pr_link_count} linked PRs are partial remote-link stubs.",
                    "recommended_action": "TPM follow-up: split status by PR and identify unresolved dependency owners.",
                    "model_method": "ticket_pr_component_rule",
                    "score": min(100.0, float(row.linked_pr_count * 20)),
                    "score_explanation": "Score increases with the number of linked PRs requiring coordination.",
                    "confidence": 0.65,
                }
            )
    out = pd.DataFrame(cards)
    if not out.empty:
        out["producer_state"] = "current"
        out["stale_reason"] = ""
    return out


def build_developer_correlation_cards(developer_correlation: pd.DataFrame) -> list[dict[str, Any]]:
    if developer_correlation.empty:
        return []
    rows = developer_correlation[
        (developer_correlation["correlation_state"] == "correlatable_same_identity")
        & (developer_correlation["extra_jira_ticket_count"] > 0)
        & (developer_correlation["pr_authored_count"] > 0)
    ].copy()
    if rows.empty:
        return []
    rows = rows.sort_values(["correlation_score", "high_risk_open_pr_count", "open_extra_jira_ticket_count"], ascending=[False, False, False])
    cards: list[dict[str, Any]] = []
    for row in rows.head(10).itertuples(index=False):
        severity = "high" if float(row.correlation_score or 0) >= 80 else "medium"
        display = clean_text(row.display_name) or clean_text(row.github_login) or clean_text(row.person_key)
        details = (
            f"{display} has a direct GitHub/Jira identity bridge, authored {int(row.pr_authored_count)} captured PR(s), "
            f"has {int(row.open_pr_authored_count)} open authored PR(s), {int(row.high_risk_open_pr_count)} high-risk open PR lead(s), "
            f"and appears on {int(row.open_extra_jira_ticket_count)}/{int(row.extra_jira_ticket_count)} extra same-window Jira ticket(s). "
            f"Top PRs: {clean_text(row.top_pr_subjects) or 'none'}; top extra Jira tickets: {clean_text(row.top_extra_ticket_keys) or 'none'}. "
            f"{DEVELOPER_CORRELATION_GUARDRAIL}"
        )
        cards.append(
            {
                "insight_kind": "developer_correlation",
                "severity": severity,
                "subject_kind": "unknown",
                "subject_key": row.person_key,
                "identity_key": "direct_identity_same_window_jira_load",
                "source_url": "",
                "title": f"Same-window Jira load near PR owner: {display}",
                "details": details,
                "recommended_action": row.recommended_tpm_action,
                "model_method": "direct_identity_same_window_overlap",
                "score": float(row.correlation_score),
                "score_explanation": "Score combines captured PR ownership, open/high-risk PRs, and extra same-window Jira ticket pressure; it is a workload lead, not causality.",
                "confidence": float(row.confidence),
            }
        )
    return cards


def build_source_coverage_cards(pr_forecasts: pd.DataFrame) -> list[dict[str, Any]]:
    if pr_forecasts.empty or "source_current_coverage_state" not in pr_forecasts.columns:
        return []
    rows = pr_forecasts[pr_forecasts["source_current_coverage_state"].isin(["detail_failed", "coverage_limited"])].copy()
    if rows.empty:
        return []
    cards: list[dict[str, Any]] = []
    rows = rows.sort_values(["source_current_detail_issue_count", "source_current_issue_count", "risk_score"], ascending=[False, False, False])
    for row in rows.head(25).itertuples(index=False):
        subject_key = f"{row.repository}#{int(row.pr_number)}"
        detail_failed = str(row.source_current_coverage_state) == "detail_failed"
        severity = "high" if detail_failed else "medium"
        issue_count = clean_int(getattr(row, "source_current_issue_count", 0)) or 0
        issue_kinds = clean_text(getattr(row, "source_current_issue_kinds", ""))
        issue_codes = clean_text(getattr(row, "source_current_issue_codes", ""))
        failure_message = clean_text(getattr(row, "source_current_failure_message", ""))
        details = (
            f"Latest fixture source sync reported {issue_count} PR-bundle coverage issue(s)"
            f" for {subject_key}; kinds={issue_kinds or 'unknown'}; codes={issue_codes or 'unknown'}."
        )
        if detail_failed:
            details += " GitHub PR detail was not verified in the latest run, so typed PR state may be stale."
        if failure_message:
            details += f" Source diagnostic: {failure_message}"
        cards.append(
            {
                "insight_kind": "status_summary",
                "severity": severity,
                "subject_kind": "pull_request",
                "subject_key": subject_key,
                "identity_key": "source_coverage_limited",
                "source_url": row.pr_url,
                "title": f"PR source coverage limited: {row.title}",
                "details": details,
                "recommended_action": "Refresh GitHub PR detail/bundle coverage before making current-state, absence, or completion claims.",
                "model_method": "source_sync_issue_coverage_gate",
                "score": 88.0 if detail_failed else 62.0,
                "score_explanation": "Score reflects source coverage risk, not product delivery risk.",
                "confidence": 0.9,
            }
        )
    return cards


def persist_time_series_snapshots(
    conn: sqlite3.Connection,
    source_instance: str,
    observed_at: str,
    pr_features: pd.DataFrame,
    ticket_features: pd.DataFrame,
    event_pr_feature_snapshots: pd.DataFrame | None = None,
) -> pd.DataFrame:
    observed_dt = parse_dt(observed_at) or datetime.now(timezone.utc)
    observed_iso = observed_dt.isoformat()
    captured_at = datetime.now(timezone.utc).isoformat()
    ensure_time_series_tables(conn)
    upsert_pr_state_snapshots(conn, source_instance, observed_iso, captured_at, pr_features)
    upsert_pr_feature_snapshots(conn, source_instance, observed_iso, captured_at, pr_features)
    if event_pr_feature_snapshots is not None and not event_pr_feature_snapshots.empty:
        for event_observed_at, group in event_pr_feature_snapshots.groupby("event_replay_observed_at"):
            if clean_text(event_observed_at):
                upsert_pr_feature_snapshots(conn, source_instance, clean_text(event_observed_at), captured_at, group.copy())
    upsert_ticket_state_snapshots(conn, source_instance, observed_iso, captured_at, ticket_features)
    refresh_state_transition_candidates(conn)
    summary = build_time_series_summary(conn, source_instance)
    transition_signal_readiness = build_transition_signal_readiness(conn, source_instance, summary)
    transition_signal_readiness.to_sql("tpm_transition_signal_readiness", conn, if_exists="replace", index=False)
    conn.commit()
    return summary


def ensure_time_series_tables(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table if not exists tpm_pr_state_snapshots (
          snapshot_key text primary key,
          source_instance text not null,
          observed_at text not null,
          subject_key text not null,
          repository text not null,
          pr_number integer not null,
          state text,
          title text,
          pr_url text,
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
          captured_at text not null
        )
        """
    )
    conn.execute("create index if not exists idx_tpm_pr_state_snapshots_subject_observed on tpm_pr_state_snapshots(subject_key, observed_at)")
    conn.execute(
        """
        create table if not exists tpm_pr_feature_snapshots (
          snapshot_key text primary key,
          source_instance text not null,
          observed_at text not null,
          subject_key text not null,
          repository text not null,
          pr_number integer not null,
          state text,
          additions integer,
          deletions integer,
          changed_files integer,
          commits integer,
          comments integer,
          review_comments integer,
          linked_ticket_count integer,
          requested_reviewer_count integer,
          issue_key_text_count integer,
          author_prior_pr_count integer,
          author_prior_merged_pr_count integer,
          author_prior_median_cycle_days real,
          author_open_pr_count integer,
          draft integer,
          additions_missing integer,
          deletions_missing integer,
          changed_files_missing integer,
          commits_missing integer,
          comments_missing integer,
          review_comments_missing integer,
          draft_missing integer,
          total_lines_changed integer,
          source_current_coverage_state text,
          source_current_detail_state text,
          lifecycle_fields_source text,
          churn_fields_source text,
          review_fields_source text,
          terminal_at text,
          target_cycle_time_days real,
          target_merged integer,
          captured_at text not null
        )
        """
    )
    ensure_table_columns(
        conn,
        "tpm_pr_feature_snapshots",
        {
            "issue_key_text_count": "integer",
            "author_prior_pr_count": "integer",
            "author_prior_merged_pr_count": "integer",
            "author_prior_median_cycle_days": "real",
            "author_open_pr_count": "integer",
        },
    )
    conn.execute("create index if not exists idx_tpm_pr_feature_snapshots_subject_observed on tpm_pr_feature_snapshots(subject_key, observed_at)")
    conn.execute("create index if not exists idx_tpm_pr_feature_snapshots_training on tpm_pr_feature_snapshots(source_instance, observed_at, terminal_at)")
    conn.execute(
        """
        create table if not exists tpm_ticket_state_snapshots (
          snapshot_key text primary key,
          source_instance text not null,
          observed_at text not null,
          ticket_key text not null,
          status text,
          priority text,
          title text,
          updated_at text,
          linked_pr_count integer,
          fresh_pr_link_count integer,
          partial_pr_link_count integer,
          comment_count integer,
          participant_count integer,
          blocker_keyword_count integer,
          captured_at text not null
        )
        """
    )
    conn.execute("create index if not exists idx_tpm_ticket_state_snapshots_subject_observed on tpm_ticket_state_snapshots(ticket_key, observed_at)")
    conn.execute(
        """
        create table if not exists tpm_state_transition_candidates (
          transition_key text primary key,
          source_instance text not null,
          subject_kind text not null,
          subject_key text not null,
          from_observed_at text not null,
          to_observed_at text not null,
          from_state text,
          to_state text,
          transition_kind text not null,
          confidence real not null,
          note text,
          created_at text not null,
          updated_at text not null
        )
        """
    )
    conn.execute("create index if not exists idx_tpm_state_transition_subject on tpm_state_transition_candidates(subject_kind, subject_key, to_observed_at)")


def upsert_pr_state_snapshots(
    conn: sqlite3.Connection,
    source_instance: str,
    observed_at: str,
    captured_at: str,
    pr_features: pd.DataFrame,
) -> None:
    if pr_features.empty:
        return
    rows = []
    for row in pr_features.itertuples(index=False):
        subject_key = f"{row.repository}#{int(row.pr_number)}"
        snapshot_key = f"tpm-pr-state-snapshot:{stable_digest([source_instance, observed_at, subject_key])}"
        rows.append(
            (
                snapshot_key,
                source_instance,
                observed_at,
                subject_key,
                clean_text(row.repository),
                int(row.pr_number),
                clean_text(getattr(row, "state", "")),
                clean_text(getattr(row, "title", "")),
                clean_text(getattr(row, "pr_url", "")),
                clean_text(getattr(row, "created_at", "")),
                clean_text(getattr(row, "updated_at", "")),
                clean_text(getattr(row, "closed_at", "")),
                clean_text(getattr(row, "merged_at", "")),
                safe_float(getattr(row, "age_days", None)),
                safe_float(getattr(row, "stale_days", None)),
                safe_float(getattr(row, "cycle_time_days", None)),
                safe_float(getattr(row, "risk_score", None)),
                clean_text(getattr(row, "risk_band", "")),
                clean_text(getattr(row, "forecast_method", "")),
                clean_text(getattr(row, "source_current_coverage_state", "")),
                clean_text(getattr(row, "source_current_detail_state", "")),
                clean_text(getattr(row, "source_current_issue_codes", "")),
                clean_text(getattr(row, "source_current_issue_kinds", "")),
                clean_text(getattr(row, "lifecycle_fields_source", "")),
                clean_text(getattr(row, "churn_fields_source", "")),
                clean_text(getattr(row, "mergeability_fields_source", "")),
                captured_at,
            )
        )
    conn.executemany(
        """
        insert into tpm_pr_state_snapshots (
          snapshot_key, source_instance, observed_at, subject_key, repository, pr_number,
          state, title, pr_url, source_created_at, source_updated_at, closed_at, merged_at,
          age_days, stale_days, cycle_time_days, risk_score, risk_band, forecast_method,
          source_current_coverage_state, source_current_detail_state, source_current_issue_codes,
          source_current_issue_kinds, lifecycle_fields_source, churn_fields_source,
          mergeability_fields_source, captured_at
        ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        on conflict(snapshot_key) do update set
          state = excluded.state,
          title = excluded.title,
          pr_url = excluded.pr_url,
          source_created_at = excluded.source_created_at,
          source_updated_at = excluded.source_updated_at,
          closed_at = excluded.closed_at,
          merged_at = excluded.merged_at,
          age_days = excluded.age_days,
          stale_days = excluded.stale_days,
          cycle_time_days = excluded.cycle_time_days,
          risk_score = excluded.risk_score,
          risk_band = excluded.risk_band,
          forecast_method = excluded.forecast_method,
          source_current_coverage_state = excluded.source_current_coverage_state,
          source_current_detail_state = excluded.source_current_detail_state,
          source_current_issue_codes = excluded.source_current_issue_codes,
          source_current_issue_kinds = excluded.source_current_issue_kinds,
          lifecycle_fields_source = excluded.lifecycle_fields_source,
          churn_fields_source = excluded.churn_fields_source,
          mergeability_fields_source = excluded.mergeability_fields_source,
          captured_at = excluded.captured_at
        """,
        rows,
    )


def upsert_pr_feature_snapshots(
    conn: sqlite3.Connection,
    source_instance: str,
    observed_at: str,
    captured_at: str,
    pr_features: pd.DataFrame,
) -> None:
    if pr_features.empty:
        return
    rows = []
    target_updates = []
    for row in pr_features.itertuples(index=False):
        repository = clean_text(getattr(row, "repository", ""))
        pr_number = clean_int(getattr(row, "pr_number", None)) or 0
        if not repository or pr_number <= 0:
            continue
        subject_key = f"{repository}#{pr_number}"
        terminal_at = clean_text(getattr(row, "merged_at", "")) or clean_text(getattr(row, "closed_at", ""))
        target_cycle_time_days = safe_float(getattr(row, "cycle_time_days", None))
        target_merged = 1 if clean_bool(getattr(row, "is_merged", False)) else 0
        additions = clean_int(getattr(row, "additions", None)) or 0
        deletions = clean_int(getattr(row, "deletions", None)) or 0
        if terminal_at and target_cycle_time_days is not None:
            target_updates.append((terminal_at, target_cycle_time_days, target_merged, source_instance, subject_key))
        rows.append(
            (
                f"tpm-pr-feature-snapshot:{stable_digest([source_instance, observed_at, subject_key])}",
                source_instance,
                observed_at,
                subject_key,
                repository,
                pr_number,
                clean_text(getattr(row, "state", "")),
                additions,
                deletions,
                clean_int(getattr(row, "changed_files", None)) or 0,
                clean_int(getattr(row, "commits", None)) or 0,
                clean_int(getattr(row, "comments", None)) or 0,
                clean_int(getattr(row, "review_comments", None)) or 0,
                clean_int(getattr(row, "linked_ticket_count", None)) or 0,
                clean_int(getattr(row, "requested_reviewer_count", None)) or 0,
                clean_int(getattr(row, "issue_key_text_count", None)) or 0,
                clean_int(getattr(row, "author_prior_pr_count", None)) or 0,
                clean_int(getattr(row, "author_prior_merged_pr_count", None)) or 0,
                safe_float(getattr(row, "author_prior_median_cycle_days", None)) or 0.0,
                clean_int(getattr(row, "author_open_pr_count", None)) or 0,
                1 if clean_bool(getattr(row, "draft", False)) else 0,
                clean_int(getattr(row, "additions_missing", None)) or 0,
                clean_int(getattr(row, "deletions_missing", None)) or 0,
                clean_int(getattr(row, "changed_files_missing", None)) or 0,
                clean_int(getattr(row, "commits_missing", None)) or 0,
                clean_int(getattr(row, "comments_missing", None)) or 0,
                clean_int(getattr(row, "review_comments_missing", None)) or 0,
                clean_int(getattr(row, "draft_missing", None)) or 0,
                clean_int(getattr(row, "total_lines_changed", None)) or additions + deletions,
                clean_text(getattr(row, "source_current_coverage_state", "")),
                clean_text(getattr(row, "source_current_detail_state", "")),
                clean_text(getattr(row, "lifecycle_fields_source", "")),
                clean_text(getattr(row, "churn_fields_source", "")),
                clean_text(getattr(row, "review_fields_source", "")),
                terminal_at,
                target_cycle_time_days,
                target_merged,
                captured_at,
            )
        )
    conn.executemany(
        """
        insert into tpm_pr_feature_snapshots (
          snapshot_key, source_instance, observed_at, subject_key, repository, pr_number, state,
          additions, deletions, changed_files, commits, comments, review_comments,
          linked_ticket_count, requested_reviewer_count, issue_key_text_count,
          author_prior_pr_count, author_prior_merged_pr_count, author_prior_median_cycle_days,
          author_open_pr_count, draft,
          additions_missing, deletions_missing, changed_files_missing, commits_missing,
          comments_missing, review_comments_missing, draft_missing, total_lines_changed,
          source_current_coverage_state, source_current_detail_state, lifecycle_fields_source,
          churn_fields_source, review_fields_source, terminal_at, target_cycle_time_days,
          target_merged, captured_at
        ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        on conflict(snapshot_key) do update set
          state = excluded.state,
          additions = excluded.additions,
          deletions = excluded.deletions,
          changed_files = excluded.changed_files,
          commits = excluded.commits,
          comments = excluded.comments,
          review_comments = excluded.review_comments,
          linked_ticket_count = excluded.linked_ticket_count,
          requested_reviewer_count = excluded.requested_reviewer_count,
          issue_key_text_count = excluded.issue_key_text_count,
          author_prior_pr_count = excluded.author_prior_pr_count,
          author_prior_merged_pr_count = excluded.author_prior_merged_pr_count,
          author_prior_median_cycle_days = excluded.author_prior_median_cycle_days,
          author_open_pr_count = excluded.author_open_pr_count,
          draft = excluded.draft,
          additions_missing = excluded.additions_missing,
          deletions_missing = excluded.deletions_missing,
          changed_files_missing = excluded.changed_files_missing,
          commits_missing = excluded.commits_missing,
          comments_missing = excluded.comments_missing,
          review_comments_missing = excluded.review_comments_missing,
          draft_missing = excluded.draft_missing,
          total_lines_changed = excluded.total_lines_changed,
          source_current_coverage_state = excluded.source_current_coverage_state,
          source_current_detail_state = excluded.source_current_detail_state,
          lifecycle_fields_source = excluded.lifecycle_fields_source,
          churn_fields_source = excluded.churn_fields_source,
          review_fields_source = excluded.review_fields_source,
          terminal_at = excluded.terminal_at,
          target_cycle_time_days = excluded.target_cycle_time_days,
          target_merged = excluded.target_merged,
          captured_at = excluded.captured_at
        """,
        rows,
    )
    if target_updates:
        conn.executemany(
            """
            update tpm_pr_feature_snapshots
               set terminal_at = ?,
                   target_cycle_time_days = ?,
                   target_merged = ?
             where source_instance = ?
               and subject_key = ?
               and (target_cycle_time_days is null or coalesce(terminal_at, '') = '')
            """,
            target_updates,
        )


def upsert_ticket_state_snapshots(
    conn: sqlite3.Connection,
    source_instance: str,
    observed_at: str,
    captured_at: str,
    ticket_features: pd.DataFrame,
) -> None:
    if ticket_features.empty:
        return
    rows = []
    for row in ticket_features.itertuples(index=False):
        ticket_key = clean_text(getattr(row, "ticket_key", "")).upper()
        if not ticket_key:
            continue
        snapshot_key = f"tpm-ticket-state-snapshot:{stable_digest([source_instance, observed_at, ticket_key])}"
        rows.append(
            (
                snapshot_key,
                source_instance,
                observed_at,
                ticket_key,
                clean_text(getattr(row, "status", "")),
                clean_text(getattr(row, "priority", "")),
                clean_text(getattr(row, "title", "")),
                clean_text(getattr(row, "updated_at", "")),
                clean_int(getattr(row, "linked_pr_count", None)) or 0,
                clean_int(getattr(row, "fresh_pr_link_count", None)) or 0,
                clean_int(getattr(row, "partial_pr_link_count", None)) or 0,
                clean_int(getattr(row, "comment_count", None)) or 0,
                clean_int(getattr(row, "participant_count", None)) or 0,
                clean_int(getattr(row, "blocker_keyword_count", None)) or 0,
                captured_at,
            )
        )
    conn.executemany(
        """
        insert into tpm_ticket_state_snapshots (
          snapshot_key, source_instance, observed_at, ticket_key, status, priority, title,
          updated_at, linked_pr_count, fresh_pr_link_count, partial_pr_link_count,
          comment_count, participant_count, blocker_keyword_count, captured_at
        ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        on conflict(snapshot_key) do update set
          status = excluded.status,
          priority = excluded.priority,
          title = excluded.title,
          updated_at = excluded.updated_at,
          linked_pr_count = excluded.linked_pr_count,
          fresh_pr_link_count = excluded.fresh_pr_link_count,
          partial_pr_link_count = excluded.partial_pr_link_count,
          comment_count = excluded.comment_count,
          participant_count = excluded.participant_count,
          blocker_keyword_count = excluded.blocker_keyword_count,
          captured_at = excluded.captured_at
        """,
        rows,
    )


def refresh_state_transition_candidates(conn: sqlite3.Connection) -> None:
    now = datetime.now(timezone.utc).isoformat()
    conn.execute(
        """
        insert into tpm_state_transition_candidates (
          transition_key, source_instance, subject_kind, subject_key,
          from_observed_at, to_observed_at, from_state, to_state,
          transition_kind, confidence, note, created_at, updated_at
        )
        select
          'tpm-transition:' || source_instance || ':pull_request:' || subject_key || ':' || previous_observed_at || ':' || observed_at,
          source_instance,
          'pull_request',
          subject_key,
          previous_observed_at,
          observed_at,
          previous_state,
          state,
          case
            when state in ('merged', 'closed') and previous_state != state then 'terminal_state_change'
            when previous_state != state then 'state_change'
            when previous_coverage_state != source_current_coverage_state then 'coverage_state_change'
            else 'state_refresh'
          end,
          case
            when previous_state != state then 0.95
            when previous_coverage_state != source_current_coverage_state then 0.85
            else 0.5
          end,
          'Derived from adjacent tpm_pr_state_snapshots; validate against source evidence before treating as ground truth.',
          ?,
          ?
        from (
          select
            *,
            lag(observed_at) over (partition by source_instance, subject_key order by observed_at) as previous_observed_at,
            lag(state) over (partition by source_instance, subject_key order by observed_at) as previous_state,
            lag(source_current_coverage_state) over (partition by source_instance, subject_key order by observed_at) as previous_coverage_state
          from tpm_pr_state_snapshots
        ) sequenced
        where previous_observed_at is not null
          and (
            coalesce(previous_state, '') != coalesce(state, '')
            or coalesce(previous_coverage_state, '') != coalesce(source_current_coverage_state, '')
          )
        on conflict(transition_key) do update set
          from_state = excluded.from_state,
          to_state = excluded.to_state,
          transition_kind = excluded.transition_kind,
          confidence = excluded.confidence,
          note = excluded.note,
          updated_at = excluded.updated_at
        """,
        (now, now),
    )
    conn.execute(
        """
        insert into tpm_state_transition_candidates (
          transition_key, source_instance, subject_kind, subject_key,
          from_observed_at, to_observed_at, from_state, to_state,
          transition_kind, confidence, note, created_at, updated_at
        )
        select
          'tpm-transition:' || source_instance || ':ticket:' || ticket_key || ':' || previous_observed_at || ':' || observed_at,
          source_instance,
          'ticket',
          ticket_key,
          previous_observed_at,
          observed_at,
          previous_status,
          status,
          case
            when status in ('closed', 'done', 'resolved') and previous_status != status then 'terminal_state_change'
            else 'state_change'
          end,
          0.9,
          'Derived from adjacent tpm_ticket_state_snapshots; validate against Jira changelog before treating as ground truth.',
          ?,
          ?
        from (
          select
            *,
            lag(observed_at) over (partition by source_instance, ticket_key order by observed_at) as previous_observed_at,
            lag(status) over (partition by source_instance, ticket_key order by observed_at) as previous_status
          from tpm_ticket_state_snapshots
        ) sequenced
        where previous_observed_at is not null
          and coalesce(previous_status, '') != coalesce(status, '')
        on conflict(transition_key) do update set
          from_state = excluded.from_state,
          to_state = excluded.to_state,
          transition_kind = excluded.transition_kind,
          confidence = excluded.confidence,
          note = excluded.note,
          updated_at = excluded.updated_at
        """,
        (now, now),
    )


def build_time_series_summary(conn: sqlite3.Connection, source_instance: str) -> pd.DataFrame:
    pr_feature_snapshot_observed_time_count = scalar_int(
        conn,
        "select count(distinct observed_at) from tpm_pr_feature_snapshots where source_instance = ?",
        (source_instance,),
    )
    event_replay_pr_feature_snapshot_count = scalar_int(
        conn,
        """
        select count(*)
          from tpm_pr_feature_snapshots
         where source_instance = ?
           and lifecycle_fields_source = 'source_event_replay'
        """,
        (source_instance,),
    )
    event_replay_pr_feature_snapshot_subject_count = scalar_int(
        conn,
        """
        select count(distinct subject_key)
          from tpm_pr_feature_snapshots
         where source_instance = ?
           and lifecycle_fields_source = 'source_event_replay'
        """,
        (source_instance,),
    )
    as_of_feature_snapshot_training_example_count = scalar_int(
        conn,
        """
        select count(*)
          from tpm_pr_feature_snapshots
         where source_instance = ?
           and target_cycle_time_days is not null
           and coalesce(terminal_at, '') != ''
           and julianday(observed_at) < julianday(terminal_at)
        """,
        (source_instance,),
    )
    as_of_feature_snapshot_terminal_subject_count = scalar_int(
        conn,
        """
        select count(distinct subject_key)
          from tpm_pr_feature_snapshots
         where source_instance = ?
           and target_cycle_time_days is not null
           and coalesce(terminal_at, '') != ''
           and julianday(observed_at) < julianday(terminal_at)
        """,
        (source_instance,),
    )
    as_of_feature_snapshot_ready = (
        pr_feature_snapshot_observed_time_count >= MIN_AS_OF_FEATURE_SNAPSHOT_OBSERVED_TIMES
        and as_of_feature_snapshot_training_example_count >= MIN_AS_OF_FEATURE_SNAPSHOT_TRAINING_EXAMPLES
    )
    if pr_feature_snapshot_observed_time_count < MIN_AS_OF_FEATURE_SNAPSHOT_OBSERVED_TIMES:
        as_of_feature_snapshot_state = "insufficient_observed_times"
    elif as_of_feature_snapshot_training_example_count < MIN_AS_OF_FEATURE_SNAPSHOT_TRAINING_EXAMPLES:
        as_of_feature_snapshot_state = "insufficient_pre_terminal_training_examples"
    elif event_replay_pr_feature_snapshot_count > 0:
        as_of_feature_snapshot_state = "ready_source_event_replay"
    else:
        as_of_feature_snapshot_state = "ready"
    as_of_feature_snapshot_basis = (
        "source_event_replay"
        if event_replay_pr_feature_snapshot_count > 0
        else "observed_snapshot_series"
        if pr_feature_snapshot_observed_time_count >= MIN_AS_OF_FEATURE_SNAPSHOT_OBSERVED_TIMES
        else "insufficient"
    )
    metrics = {
        "pr_state_snapshot_count": scalar_int(conn, "select count(*) from tpm_pr_state_snapshots where source_instance = ?", (source_instance,)),
        "pr_feature_snapshot_count": scalar_int(conn, "select count(*) from tpm_pr_feature_snapshots where source_instance = ?", (source_instance,)),
        "pr_feature_snapshot_observed_time_count": pr_feature_snapshot_observed_time_count,
        "event_replay_pr_feature_snapshot_count": event_replay_pr_feature_snapshot_count,
        "event_replay_pr_feature_snapshot_subject_count": event_replay_pr_feature_snapshot_subject_count,
        "as_of_feature_snapshot_training_example_count": as_of_feature_snapshot_training_example_count,
        "as_of_feature_snapshot_terminal_subject_count": as_of_feature_snapshot_terminal_subject_count,
        "as_of_feature_snapshot_ready": "true" if as_of_feature_snapshot_ready else "false",
        "as_of_feature_snapshot_state": as_of_feature_snapshot_state,
        "as_of_feature_snapshot_basis": as_of_feature_snapshot_basis,
        "ticket_state_snapshot_count": scalar_int(conn, "select count(*) from tpm_ticket_state_snapshots where source_instance = ?", (source_instance,)),
        "observed_snapshot_time_count": scalar_int(
            conn,
            """
            select count(*) from (
              select observed_at from tpm_pr_state_snapshots where source_instance = ?
              union
              select observed_at from tpm_ticket_state_snapshots where source_instance = ?
            )
            """,
            (source_instance, source_instance),
        ),
        "transition_candidate_count": scalar_int(conn, "select count(*) from tpm_state_transition_candidates where source_instance = ?", (source_instance,)),
        "terminal_transition_candidate_count": scalar_int(
            conn,
            "select count(*) from tpm_state_transition_candidates where source_instance = ? and transition_kind = 'terminal_state_change'",
            (source_instance,),
        ),
    }
    rows = [
        {
            "metric": metric,
            "value": str(value),
            "note": time_series_metric_note(metric),
        }
        for metric, value in metrics.items()
    ]
    return pd.DataFrame(rows)


def build_transition_signal_readiness(
    conn: sqlite3.Connection,
    source_instance: str,
    time_series_summary: pd.DataFrame,
) -> pd.DataFrame:
    transition_count = scalar_int(
        conn,
        "select count(*) from tpm_state_transition_candidates where source_instance = ?",
        (source_instance,),
    )
    terminal_count = scalar_int(
        conn,
        """
        select count(*)
          from tpm_state_transition_candidates
         where source_instance = ?
           and transition_kind = 'terminal_state_change'
        """,
        (source_instance,),
    )
    terminal_subject_count = scalar_int(
        conn,
        """
        select count(distinct subject_kind || ':' || subject_key)
          from tpm_state_transition_candidates
         where source_instance = ?
           and transition_kind = 'terminal_state_change'
        """,
        (source_instance,),
    )
    latest_terminal_subject_count = scalar_int(
        conn,
        """
        select count(*)
          from (
            select subject_kind, subject_key, transition_kind,
                   row_number() over (
                     partition by subject_kind, subject_key
                     order by to_observed_at desc, confidence desc, transition_key desc
                   ) as row_rank
              from tpm_state_transition_candidates
             where source_instance = ?
          ) ranked
         where row_rank = 1
           and transition_kind = 'terminal_state_change'
        """,
        (source_instance,),
    )
    superseded_terminal_count = scalar_int(
        conn,
        """
        select count(*)
          from tpm_state_transition_candidates terminal
         where terminal.source_instance = ?
           and terminal.transition_kind = 'terminal_state_change'
           and exists (
             select 1
               from tpm_state_transition_candidates later
              where later.source_instance = terminal.source_instance
                and later.subject_kind = terminal.subject_kind
                and later.subject_key = terminal.subject_key
                and julianday(later.to_observed_at) > julianday(terminal.to_observed_at)
           )
        """,
        (source_instance,),
    )
    as_of_ready = metric_map(time_series_summary).get("as_of_feature_snapshot_ready") == "true"
    observed_time_count = clean_int(metric_map(time_series_summary).get("observed_snapshot_time_count")) or 0
    terminal_closeout_ready = latest_terminal_subject_count > 0
    if terminal_closeout_ready:
        closeout_state = "ready_with_latest_terminal_transition"
        closeout_reason = "At least one subject's latest transition candidate is terminal; create closeout review actions, not automatic closure."
        closeout_action = "Open closeout review actions and require current-state/source evidence before resolution."
    elif superseded_terminal_count > 0:
        closeout_state = "terminal_transition_superseded"
        closeout_reason = "Terminal transition candidates exist, but each is followed by a later non-terminal transition for the same subject."
        closeout_action = "Keep terminal transitions as audit evidence only; do not close or source-resolve current work from superseded terminal candidates."
    elif transition_count > 0:
        closeout_state = "no_latest_terminal_transition"
        closeout_reason = "Transition candidates exist, but none currently indicate latest terminal state."
        closeout_action = "Use transition history for trend/audit context only."
    else:
        closeout_state = "no_transition_history"
        closeout_reason = "No state transition candidates were derived from adjacent snapshots."
        closeout_action = "Collect repeated PR/Jira snapshots before generating transition closeout work."

    source_resolved_ready = False
    if latest_terminal_subject_count > 0:
        source_resolved_state = "needs_authenticated_current_state"
        source_resolved_reason = "Terminal transition evidence exists, but source-resolved closeout additionally requires authenticated current source state to still be terminal."
        source_resolved_action = "Let action brief compare the transition against latest typed state and authenticated follow-up before auto-resolving."
    elif superseded_terminal_count > 0:
        source_resolved_state = "blocked_by_later_nonterminal_state"
        source_resolved_reason = "Latest transition evidence contradicts earlier terminal candidates."
        source_resolved_action = "Do not source-resolve; retain as closeout/audit evidence only."
    else:
        source_resolved_state = "no_terminal_source_resolution_evidence"
        source_resolved_reason = "No latest terminal transition is available for source-resolved closeout."
        source_resolved_action = "Wait for authenticated current terminal state before source-resolved closure."

    eta_ready = False
    if as_of_ready and transition_count > 0:
        eta_state = "candidate_needs_transition_label_validation"
        eta_reason = "As-of feature snapshots exist, but transition candidates are adjacent-snapshot derivations and need source changelog validation before ETA feature promotion."
        eta_action = "Backtest transition-derived features against outcomes and require same-model lift before using them in ETA."
    elif observed_time_count >= 2:
        eta_state = "snapshot_history_insufficient_for_eta"
        eta_reason = "Repeated state snapshots exist, but transition labels are not validated enough for ETA feature promotion."
        eta_action = "Collect source changelog transitions and run transition-feature ablation."
    else:
        eta_state = "single_or_missing_snapshot_history"
        eta_reason = "Transition history is too sparse for ETA feature use."
        eta_action = "Collect repeated state snapshots before transition-feature modeling."

    common = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "workstream_key": "flink-kubernetes-operator",
        "source_instance": source_instance,
        "transition_candidate_count": transition_count,
        "terminal_transition_candidate_count": terminal_count,
        "terminal_transition_subject_count": terminal_subject_count,
        "latest_terminal_transition_subject_count": latest_terminal_subject_count,
        "superseded_terminal_transition_count": superseded_terminal_count,
        "observed_snapshot_time_count": observed_time_count,
        "as_of_feature_snapshot_ready": bool(as_of_ready),
    }
    return pd.DataFrame(
        [
            {
                **common,
                "readiness_key": "terminal_closeout_review",
                "support_level": "closeout_review",
                "ready": bool(terminal_closeout_ready),
                "readiness_state": closeout_state,
                "blocking_reason": closeout_reason,
                "recommended_action": closeout_action,
            },
            {
                **common,
                "readiness_key": "source_resolved_closeout",
                "support_level": "source_resolved_evidence",
                "ready": bool(source_resolved_ready),
                "readiness_state": source_resolved_state,
                "blocking_reason": source_resolved_reason,
                "recommended_action": source_resolved_action,
            },
            {
                **common,
                "readiness_key": "transition_eta_feature",
                "support_level": "eta_feature",
                "ready": bool(eta_ready),
                "readiness_state": eta_state,
                "blocking_reason": eta_reason,
                "recommended_action": eta_action,
            },
        ]
    )


def time_series_metric_note(metric: str) -> str:
    return {
        "pr_state_snapshot_count": "Append/upserted PR state observations available for trend and transition analysis.",
        "pr_feature_snapshot_count": "Append/upserted PR feature observations preserving the mutable inputs used by ETA/risk models.",
        "pr_feature_snapshot_observed_time_count": "Distinct observed timestamps with PR feature snapshots.",
        "event_replay_pr_feature_snapshot_count": "Pre-terminal PR feature snapshots reconstructed from source event timestamps with future-only churn fields marked missing.",
        "event_replay_pr_feature_snapshot_subject_count": "Distinct PR subjects with event-replayed feature snapshots.",
        "as_of_feature_snapshot_training_example_count": "Pre-terminal PR feature snapshots with known closed/merged cycle outcomes; these are the rows eligible for as-of ETA training.",
        "as_of_feature_snapshot_terminal_subject_count": "Distinct terminal PRs with at least one pre-terminal feature snapshot and known cycle outcome.",
        "as_of_feature_snapshot_ready": "True only when enough pre-terminal as-of PR feature snapshots exist for ETA training.",
        "as_of_feature_snapshot_state": "Reason the as-of feature snapshot series is or is not ready for ETA training.",
        "as_of_feature_snapshot_basis": "Whether the as-of feature series comes from repeated observations or reconstructed source event history.",
        "ticket_state_snapshot_count": "Append/upserted ticket state observations available for trend and transition analysis.",
        "observed_snapshot_time_count": "Distinct source-observed timestamps across PR and ticket snapshots.",
        "transition_candidate_count": "Adjacent snapshot state or coverage changes detected; candidates require source evidence validation.",
        "terminal_transition_candidate_count": "Adjacent snapshot changes into merged/closed-like terminal states.",
    }.get(metric, "")


def scalar_int(conn: sqlite3.Connection, query: str, params: tuple[Any, ...] = ()) -> int:
    row = conn.execute(query, params).fetchone()
    if row is None:
        return 0
    return int(row[0] or 0)


def build_model_quality_cards(forecast_summary: pd.DataFrame, forecast_backtest: pd.DataFrame) -> list[dict[str, Any]]:
    if forecast_summary.empty:
        return []
    metrics = metric_map(forecast_summary)
    eta_ready = metrics.get("eta_forecast_ready", "false") == "true"
    median_mae = metrics.get("backtest_median_mae_days", "")
    heuristic_mae = metrics.get("backtest_heuristic_mae_days", "")
    rf_mae = metrics.get("backtest_random_forest_mae_days", "")
    best_model = metrics.get("backtest_best_model", "")
    primary_blocker = metrics.get("eta_primary_blocker", "")
    snapshot_state = metrics.get("eta_temporal_snapshot_state", "")
    next_evidence = metrics.get("eta_next_evidence_needed", "")
    merged_count = metrics.get("merged_pr_count", "")
    severity = "info" if eta_ready else "medium"
    score = 35.0 if eta_ready else 72.0
    title = "Forecast model is ETA-ready" if eta_ready else "Forecast model is not ETA-ready"
    details = (
        f"Backtest over {merged_count} merged PRs: median baseline MAE {median_mae}d, "
        f"heuristic MAE {heuristic_mae}d, random forest MAE {rf_mae}d; best K-fold model {best_model or 'unknown'}."
    )
    if not eta_ready:
        if primary_blocker:
            details += f" Primary blocker: {primary_blocker}."
        if snapshot_state:
            details += f" Snapshot state: {snapshot_state}."
        details += " Use cycle output as TPM risk triage, not an ETA promise."
    return [
        {
            "insight_kind": "model_quality",
            "severity": severity,
            "subject_kind": "unknown",
            "subject_key": "flink-pr-cycle-forecast",
            "identity_key": "forecast_backtest",
            "source_url": "",
            "title": title,
            "details": details,
            "recommended_action": (
                f"Use PR cycle forecasts as prioritization signals only; {next_evidence or 'collect time-series snapshots, Jira transitions, CI outcomes, and human labels'} before using ETA promises."
                if not eta_ready
                else "Forecast model passed the current backtest gate; keep monitoring drift before using it for commitments."
            ),
            "model_method": "forecast_backtest_quality_gate",
            "score": score,
            "score_explanation": "Score reflects forecast-readiness risk; higher means the model needs more validation before ETA use.",
            "confidence": 0.85 if not forecast_backtest.empty else 0.55,
        }
    ]


def subject_kind_for(product_key: str) -> str:
    if "#" in product_key:
        return "pull_request"
    if ISSUE_RE.fullmatch(product_key or ""):
        return "ticket"
    return "unknown"


def persist_work_insights(
    conn: sqlite3.Connection,
    insight_cards: pd.DataFrame,
    pull_request_subjects: pd.DataFrame,
    ticket_subjects: pd.DataFrame,
    source_instance: str,
    generated_at: str,
    report_path: Path,
) -> None:
    if insight_cards.empty:
        return
    if not table_exists(conn, "work_insights"):
        raise RuntimeError("ontology DB is missing work_insights; rerun the Ent migration/load before analytics")

    generated_dt = parse_dt(generated_at) or datetime.now(timezone.utc)
    generated_iso = generated_dt.isoformat()
    pr_ids = {
        f"{row.repository}#{int(row.number)}": int(row.id)
        for row in pull_request_subjects.itertuples(index=False)
        if row.repository and not pd.isna(row.number)
    }
    ticket_ids = {
        str(row.external_id).upper(): int(row.id)
        for row in ticket_subjects.itertuples(index=False)
        if row.external_id
    }
    conn.execute(
        """
        update evidences
        set proof_state = 'stale',
            freshness_state = 'stale',
            updated_at = ?
        where id in (
            select latest_evidence_id
            from work_insights
            where source_system = 'cubicle_analytics'
              and source_instance = ?
              and external_kind = 'tpm_insight'
              and model_name = 'flink_tpm_analytics'
              and latest_evidence_id is not null
        )
        """,
        (generated_iso, source_instance),
    )
    conn.execute(
        """
        update work_insights
        set producer_state = 'stale',
            freshness_state = 'stale',
            updated_at = ?
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_insight'
          and model_name = 'flink_tpm_analytics'
        """,
        (generated_iso, source_instance),
    )
    for row in insight_cards.itertuples(index=False):
        card = row._asdict()
        subject_key = str(card["subject_key"])
        subject_kind = str(card.get("subject_kind") or subject_kind_for(subject_key))
        pull_request_id = pr_ids.get(subject_key) if subject_kind == "pull_request" else None
        ticket_id = ticket_ids.get(subject_key.upper()) if subject_kind == "ticket" else None
        if subject_kind == "pull_request" and pull_request_id is None:
            subject_kind = "unknown"
        if subject_kind == "ticket" and ticket_id is None:
            subject_kind = "unknown"
        if subject_kind == "unknown":
            pull_request_id = None
            ticket_id = None
        digest = stable_digest([card["insight_kind"], subject_kind, subject_key, card.get("identity_key") or "default"])
        insight_key = f"work-insight:cubicle-analytics:{source_instance}:{digest}"
        evidence_key = f"evidence:work-insight:cubicle-analytics:{source_instance}:{digest}"
        confidence = float(card.get("confidence") or 0.5)
        score = float(card.get("score") or 0)
        external_id = f"tpm-insight:{digest}"
        source_url = clean_text(card.get("source_url")) or str(report_path)
        has_source_span = bool(clean_text(card.get("evidence_source_system")) and clean_text(card.get("evidence_locator_kind")))
        if has_source_span:
            evidence_claim_field = "source_text_match"
            evidence_locator_kind = clean_text(card.get("evidence_locator_kind"))
            evidence_locator = clean_text(card.get("evidence_locator"))
            evidence_span_key = clean_text(card.get("evidence_source_span_key")) or digest
            evidence_excerpt = clean_text(card.get("evidence_excerpt")) or clean_text(card.get("details")) or clean_text(card.get("title"))
            evidence_source_system = clean_text(card.get("evidence_source_system"))
            evidence_source_instance = clean_text(card.get("evidence_source_instance"))
            evidence_external_kind = clean_text(card.get("evidence_external_kind"))
            evidence_external_id = clean_text(card.get("evidence_external_id"))
            evidence_source_url = clean_text(card.get("evidence_source_url")) or source_url
            evidence_span_start = clean_int(card.get("evidence_span_start"))
            evidence_span_end = clean_int(card.get("evidence_span_end"))
            evidence_excerpt_truncated = clean_bool(card.get("evidence_excerpt_truncated"))
        else:
            evidence_claim_field = "generated_output"
            evidence_locator_kind = "analytics_output"
            evidence_locator = f"{card['insight_kind']}:{subject_key}"
            evidence_span_key = digest
            evidence_excerpt = clean_text(card.get("details")) or clean_text(card.get("title"))
            evidence_source_system = "cubicle_analytics"
            evidence_source_instance = source_instance
            evidence_external_kind = "tpm_insight_evidence"
            evidence_external_id = external_id
            evidence_source_url = str(report_path)
            evidence_span_start = None
            evidence_span_end = None
            evidence_excerpt_truncated = False
        evidence_text_hash = stable_digest([evidence_excerpt])
        conn.execute(
            """
            insert into work_insights (
              key, insight_kind, severity, producer_state, subject_kind, subject_key,
              pull_request_id, ticket_id, title, details, recommended_action,
              model_name, model_version, model_method, score, score_explanation,
              latest_evidence_id, evidence_count,
              source_system, source_instance, external_kind, external_id, source_url,
              deletion_state, acl_state, freshness_state, visibility, confidence,
              event_count, first_seen_at, last_activity_at, rank_score,
              created_at, updated_at
            ) values (
              ?, ?, ?, 'current', ?, ?,
              ?, ?, ?, ?, ?,
              'flink_tpm_analytics', ?, ?, ?, ?,
              null, 0,
              'cubicle_analytics', ?, 'tpm_insight', ?, ?,
              'present', 'unavailable', 'partial', 'restricted', ?,
              1, ?, ?, ?,
              ?, ?
            )
            on conflict(key) do update set
              severity = excluded.severity,
              producer_state = 'current',
              subject_kind = excluded.subject_kind,
              subject_key = excluded.subject_key,
              pull_request_id = excluded.pull_request_id,
              ticket_id = excluded.ticket_id,
              title = excluded.title,
              details = excluded.details,
              recommended_action = excluded.recommended_action,
              model_method = excluded.model_method,
              model_name = excluded.model_name,
              model_version = excluded.model_version,
              score = excluded.score,
              score_explanation = excluded.score_explanation,
              source_url = excluded.source_url,
              acl_state = 'unavailable',
              freshness_state = 'partial',
              visibility = 'restricted',
              confidence = excluded.confidence,
              last_activity_at = excluded.last_activity_at,
              rank_score = excluded.rank_score,
              updated_at = excluded.updated_at
            """,
            (
                insight_key,
                card["insight_kind"],
                card["severity"],
                subject_kind,
                subject_key,
                pull_request_id,
                ticket_id,
                card["title"],
                card.get("details") or "",
                card.get("recommended_action") or "",
                ANALYTICS_VERSION,
                card.get("model_method") or "",
                score,
                card.get("score_explanation") or "",
                source_instance,
                external_id,
                source_url,
                confidence,
                generated_iso,
                generated_iso,
                score,
                generated_iso,
                generated_iso,
            ),
        )
        insight_id = int(conn.execute("select id from work_insights where key = ?", (insight_key,)).fetchone()[0])
        conn.execute(
            """
            insert into evidences (
              key, claim_kind, claim_target_kind, claim_target_id, claim_field,
              locator_kind, locator, source_span_key, span_start, span_end,
              excerpt, excerpt_truncated,
              text_hash, proof_state, observed_at,
              source_system, source_instance, external_kind, external_id, source_url,
              deletion_state, acl_state, freshness_state, visibility, confidence,
              created_at, updated_at
            ) values (
              ?, 'candidate', 'work_insight', ?, ?,
              ?, ?, ?, ?, ?,
              ?, ?,
              ?, 'current', ?,
              ?, ?, ?, ?, ?,
              'present', 'unavailable', 'partial', 'restricted', ?,
              ?, ?
            )
            on conflict(key) do update set
              claim_target_id = excluded.claim_target_id,
              claim_field = excluded.claim_field,
              locator_kind = excluded.locator_kind,
              locator = excluded.locator,
              source_span_key = excluded.source_span_key,
              span_start = excluded.span_start,
              span_end = excluded.span_end,
              excerpt = excluded.excerpt,
              excerpt_truncated = excluded.excerpt_truncated,
              text_hash = excluded.text_hash,
              proof_state = 'current',
              observed_at = excluded.observed_at,
              source_system = excluded.source_system,
              source_instance = excluded.source_instance,
              external_kind = excluded.external_kind,
              external_id = excluded.external_id,
              source_url = excluded.source_url,
              acl_state = 'unavailable',
              freshness_state = 'partial',
              visibility = 'restricted',
              confidence = excluded.confidence,
              updated_at = excluded.updated_at
            """,
            (
                evidence_key,
                insight_id,
                evidence_claim_field,
                evidence_locator_kind,
                evidence_locator,
                evidence_span_key,
                evidence_span_start,
                evidence_span_end,
                evidence_excerpt,
                evidence_excerpt_truncated,
                evidence_text_hash,
                generated_iso,
                evidence_source_system,
                evidence_source_instance,
                evidence_external_kind,
                evidence_external_id,
                evidence_source_url,
                confidence,
                generated_iso,
                generated_iso,
            ),
        )
        evidence_id = int(conn.execute("select id from evidences where key = ?", (evidence_key,)).fetchone()[0])
        conn.execute(
            """
            update work_insights
            set latest_evidence_id = ?,
                evidence_count = 1,
                updated_at = ?
            where id = ?
            """,
            (evidence_id, generated_iso, insight_id),
        )
    conn.commit()


def persist_work_insight_review_requests(
    conn: sqlite3.Connection,
    source_instance: str,
    generated_at: str,
) -> None:
    if not table_exists(conn, "work_insight_reviews"):
        raise RuntimeError("ontology DB is missing work_insight_reviews; rerun the Ent migration/load before analytics")

    generated_dt = parse_dt(generated_at) or datetime.now(timezone.utc)
    generated_iso = generated_dt.isoformat()
    insight_rows = conn.execute(
        """
        select id, key, insight_kind, severity, subject_kind, subject_key, source_url
        from work_insights
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_insight'
          and producer_state = 'current'
        """,
        (source_instance,),
    ).fetchall()
    for insight_id, insight_key, insight_kind, severity, subject_kind, subject_key, source_url in insight_rows:
        digest = stable_digest([insight_key, "triage_request"])
        review_key = f"work-insight-review:cubicle-analytics:{source_instance}:{digest}"
        external_id = f"tpm-insight-review:{digest}"
        next_action = review_next_action(insight_kind, severity, subject_kind, subject_key)
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_quality, reviewer_kind, reviewer_key, next_action,
              rationale, source_system, source_instance, external_kind,
              external_id, source_url, created_at, updated_at
            ) values (
              ?, ?, 'triage_request', 'requested', 'unknown',
              'unknown', 'unknown', 'system', 'flink_tpm_analytics', ?,
              'Seeded review request; truth and actionability labels require human or imported evaluation.',
              'cubicle_analytics', ?, 'tpm_insight_review',
              ?, ?, ?, ?
            )
            on conflict(key) do update set
              next_action = excluded.next_action,
              rationale = excluded.rationale,
              updated_at = excluded.updated_at
            """,
            (
                review_key,
                int(insight_id),
                next_action,
                source_instance,
                external_id,
                source_url or "",
                generated_iso,
                generated_iso,
            ),
        )
    conn.commit()


def review_next_action(insight_kind: str, severity: str, subject_kind: str, subject_key: str) -> str:
    if insight_kind == "blocker_candidate":
        return f"Validate whether {subject_key} is currently blocked; label truth/actionability and record owner or next step."
    if insight_kind == "forecast_risk":
        return f"Validate whether {subject_key} needs TPM follow-up; label actionability and owner."
    if insight_kind == "dependency_cluster":
        return f"Review {subject_key} for real coordination dependencies; label whether this cluster is actionable."
    if insight_kind == "developer_correlation":
        return f"Review same-window Jira workload context for {subject_key}; label whether it changes routing, capacity, or escalation."
    if insight_kind == "status_summary":
        return f"Confirm whether requested reviewers for {subject_key} are still expected; label actionability without treating sparse requested-reviewer data as proven reviewer inactivity."
    if insight_kind == "model_quality":
        return "Review forecast backtest quality; keep ETA use gated until the model beats simple baselines on enough data."
    return f"Review {subject_kind}:{subject_key} and label truth/actionability."


def read_work_insight_review_queue(
    conn: sqlite3.Connection,
    source_instance: str,
    measurement_label_sets: set[str],
) -> pd.DataFrame:
    if not table_exists(conn, "work_insight_reviews"):
        return pd.DataFrame()
    rows = pd.read_sql_query(
        """
        select
          wir.id as review_id,
          wi.key as insight_key,
          wir.key as review_key,
          wi.insight_kind,
          wi.severity,
          wi.subject_kind,
          wi.subject_key,
          wi.score,
          wi.confidence,
          wir.review_kind,
          wir.review_state,
          wir.truth_label,
          wir.actionability_label,
          coalesce(wir.label_set, '') as label_set,
          coalesce(wir.label_quality, '') as label_quality,
          coalesce(wir.measurement_eligible, 0) as stored_measurement_eligible,
          wir.reviewer_kind,
          coalesce(wir.reviewer_key, '') as reviewer_key,
          coalesce(wir.owner_key, '') as owner_key,
          coalesce(wir.next_action, '') as next_action,
          coalesce(wir.rationale, '') as rationale,
          coalesce(wir.reviewed_at, '') as reviewed_at,
          coalesce(wir.source_url, '') as label_source_url
        from work_insight_reviews wir
        join work_insights wi on wi.id = wir.work_insight_id
        where wi.source_system = 'cubicle_analytics'
          and wi.source_instance = ?
          and wi.external_kind = 'tpm_insight'
          and wi.producer_state = 'current'
        order by
          case wi.severity
            when 'critical' then 5
            when 'high' then 4
            when 'medium' then 3
            when 'low' then 2
            else 1
          end desc,
          wi.rank_score desc,
          wi.subject_key,
          wir.review_kind
        """,
        conn,
        params=(source_instance,),
    )
    if rows.empty:
        return rows
    rows["label_set"] = rows["label_set"].where(rows["label_set"].astype(str) != "", rows["review_key"].map(extract_label_set))
    rows["label_quality"] = rows.apply(infer_label_quality_from_review, axis=1)
    rows["measurement_eligible"] = rows.apply(
        lambda row: "true" if is_measurement_label(row, measurement_label_sets) else "false",
        axis=1,
    )
    return rows


def build_evaluation_readiness(
    insight_cards: pd.DataFrame,
    review_queue: pd.DataFrame,
) -> pd.DataFrame:
    if review_queue.empty:
        return pd.DataFrame(
            [
                {"metric": "current_insight_count", "value": str(len(insight_cards)), "note": "current generated insight cards"},
                {"metric": "review_request_count", "value": "0", "note": "review rows available for truth/actionability labels"},
                {"metric": "can_measure_precision", "value": "false", "note": "requires non-unknown truth labels"},
            ]
        )

    total = int(review_queue["insight_key"].nunique())
    all_label_rows = review_queue[
        review_queue["review_kind"].isin(["human_assessment", "evaluation_label"])
        & ((review_queue["truth_label"] != "unknown") | (review_queue["actionability_label"] != "unknown"))
    ].copy()
    label_rows = measurement_label_rows(all_label_rows)
    truth_labeled = int(label_rows[label_rows["truth_label"] != "unknown"]["insight_key"].nunique()) if not label_rows.empty else 0
    actionability_labeled = int(label_rows[label_rows["actionability_label"] != "unknown"]["insight_key"].nunique()) if not label_rows.empty else 0
    resolved = label_rows[
        label_rows["review_state"].isin(RESOLVED_REVIEW_STATES)
        & label_rows["truth_label"].isin(["true_positive", "false_positive"])
        & label_rows["actionability_label"].isin(POSITIVE_ACTIONABILITY | {"not_actionable"})
    ] if not label_rows.empty else label_rows
    reviewed = int(resolved["insight_key"].nunique()) if not resolved.empty else 0
    min_labeled_total = MIN_MEASUREMENT_LABEL_TOTAL
    min_labeled_per_kind = MIN_MEASUREMENT_LABEL_PER_KIND
    current_by_kind = review_queue.groupby("insight_kind")["insight_key"].nunique()
    truth_by_kind = label_rows[label_rows["truth_label"] != "unknown"].groupby("insight_kind")["insight_key"].nunique() if not label_rows.empty else pd.Series(dtype=int)
    actionability_by_kind = label_rows[label_rows["actionability_label"] != "unknown"].groupby("insight_kind")["insight_key"].nunique() if not label_rows.empty else pd.Series(dtype=int)
    all_kinds = set(review_queue["insight_kind"].unique())
    required_by_kind = {kind: min(min_labeled_per_kind, int(current_by_kind.get(kind, 0))) for kind in all_kinds}
    truth_ready_by_kind = all(truth_by_kind.get(kind, 0) >= required_by_kind.get(kind, min_labeled_per_kind) for kind in all_kinds)
    actionability_ready_by_kind = all(actionability_by_kind.get(kind, 0) >= required_by_kind.get(kind, min_labeled_per_kind) for kind in all_kinds)
    ready_for_precision = truth_labeled >= min_labeled_total and truth_ready_by_kind
    ready_for_actionability = actionability_labeled >= min_labeled_total and actionability_ready_by_kind
    rows = [
        {"metric": "current_insight_count", "value": str(len(insight_cards)), "note": "current generated insight cards"},
        {"metric": "review_row_count", "value": str(len(review_queue)), "note": "review rows available for truth/actionability labels"},
        {"metric": "label_row_count", "value": str(len(all_label_rows)), "note": "human/imported label rows with at least one non-unknown label"},
        {"metric": "evaluation_label_row_count", "value": str(len(label_rows)), "note": "deduped measurement-eligible label rows"},
        {"metric": "non_measurement_label_row_count", "value": str(max(0, len(all_label_rows) - len(label_rows))), "note": "smoke, candidate, or adversarial label rows excluded from measurement readiness"},
        {"metric": "open_review_request_count", "value": str(max(0, total - reviewed)), "note": "current insights still missing resolved measurement labels"},
        {"metric": "reviewed_count", "value": str(reviewed), "note": "current insights with resolved measurement labels"},
        {"metric": "truth_labeled_count", "value": str(truth_labeled), "note": "measurement labels with true/false/partial labels"},
        {"metric": "actionability_labeled_count", "value": str(actionability_labeled), "note": "measurement labels with actionability labels"},
        {"metric": "truth_label_coverage", "value": f"{truth_labeled}/{total}", "note": "current truth-label numerator and denominator"},
        {"metric": "actionability_label_coverage", "value": f"{actionability_labeled}/{total}", "note": "current actionability-label numerator and denominator"},
        {"metric": "min_labeled_total_required", "value": str(min_labeled_total), "note": "minimum labels before aggregate metrics are considered stable enough to report"},
        {"metric": "min_labeled_per_kind_required", "value": str(min_labeled_per_kind), "note": "minimum labels required for each insight kind"},
        {"metric": "has_any_truth_labels", "value": "true" if truth_labeled > 0 else "false", "note": "there is at least one truth label"},
        {"metric": "has_any_actionability_labels", "value": "true" if actionability_labeled > 0 else "false", "note": "there is at least one actionability label"},
        {"metric": "ready_to_measure_precision", "value": "true" if ready_for_precision else "false", "note": "requires enough truth labels overall and per insight kind"},
        {"metric": "ready_to_measure_actionability", "value": "true" if ready_for_actionability else "false", "note": "requires enough actionability labels overall and per insight kind"},
    ]
    for insight_kind, group in review_queue.groupby("insight_kind"):
        required = required_by_kind.get(insight_kind, min_labeled_per_kind)
        kind_truth = int(truth_by_kind.get(insight_kind, 0))
        kind_actionability = int(actionability_by_kind.get(insight_kind, 0))
        rows.append(
            {
                "metric": f"review_requests_{insight_kind}",
                "value": str(int(group["insight_key"].nunique())),
                "note": "distinct current insights by kind that need measurement labels",
            }
        )
        rows.append(
            {
                "metric": f"measurement_required_{insight_kind}",
                "value": str(int(required)),
                "note": "bounded labels required before this sparse insight kind can support product actions",
            }
        )
        rows.append(
            {
                "metric": f"truth_labeled_{insight_kind}",
                "value": str(kind_truth),
                "note": "measurement truth labels by insight kind",
            }
        )
        rows.append(
            {
                "metric": f"actionability_labeled_{insight_kind}",
                "value": str(kind_actionability),
                "note": "measurement actionability labels by insight kind",
            }
        )
        rows.append(
            {
                "metric": f"ready_to_measure_{insight_kind}",
                "value": "true" if kind_truth >= required and kind_actionability >= required and required > 0 else "false",
                "note": "kind-level gate used for action-specific promotion",
            }
        )
    return pd.DataFrame(rows)


def measurement_label_rows(review_queue: pd.DataFrame) -> pd.DataFrame:
    if review_queue.empty:
        return review_queue
    if "measurement_eligible" not in review_queue.columns:
        labeled = review_queue.copy()
    else:
        labeled = review_queue[review_queue["measurement_eligible"] == "true"].copy()
    if labeled.empty:
        return labeled
    if "label_quality" not in labeled.columns:
        labeled["label_quality"] = labeled.apply(infer_label_quality_from_review, axis=1)
    labeled["_quality_rank"] = labeled["label_quality"].map(lambda value: {"gold": 4, "adversarial": 3, "candidate": 2, "smoke": 1}.get(str(value), 0))
    labeled["_review_kind_rank"] = labeled["review_kind"].map(lambda value: 3 if value == "human_assessment" else 1)
    if "review_id" in labeled.columns:
        labeled["_review_id"] = pd.to_numeric(labeled["review_id"], errors="coerce").fillna(0)
    else:
        labeled["_review_id"] = range(len(labeled))
    if "reviewed_at" not in labeled.columns:
        labeled["reviewed_at"] = ""
    return (
        labeled.sort_values(["insight_key", "_quality_rank", "_review_kind_rank", "reviewed_at", "_review_id"])
        .drop_duplicates("insight_key", keep="last")
        .drop(columns=["_quality_rank", "_review_kind_rank", "_review_id"])
    )


def extract_label_set(review_key: Any) -> str:
    parts = clean_text(review_key).split(":")
    if len(parts) >= 5 and parts[0] == "work-insight-review" and parts[1] == "cubicle-evaluation":
        return parts[-2]
    return ""


def infer_label_quality_from_review(row: pd.Series) -> str:
    explicit = clean_text(row.get("label_quality"))
    if explicit in {"adversarial", "candidate", "gold", "smoke"}:
        return explicit
    if clean_text(row.get("review_kind")) == "human_assessment":
        return "gold"
    reviewer_kind = clean_text(row.get("reviewer_kind"))
    if reviewer_kind.startswith("imported_"):
        quality = reviewer_kind.removeprefix("imported_")
        if quality in {"adversarial", "candidate", "gold", "smoke"}:
            return quality
    text = " ".join(
        [
            clean_text(row.get("label_set")),
            clean_text(row.get("reviewer_key")),
            clean_text(row.get("label_source_url")),
            clean_text(row.get("rationale")),
        ]
    ).lower()
    if "adversarial" in text:
        return "adversarial"
    if "smoke" in text:
        return "smoke"
    if "gold" in text:
        return "gold"
    return "candidate"


def is_measurement_label(row: pd.Series, measurement_label_sets: set[str]) -> bool:
    stored = clean_text(row.get("stored_measurement_eligible")).lower()
    if stored in {"false", "0", "no"}:
        return False
    if clean_text(row.get("review_kind")) == "human_assessment":
        return True
    if clean_text(row.get("label_quality")) in MEASUREMENT_LABEL_QUALITIES:
        return True
    return False


def stored_measurement_eligible(row: pd.Series) -> bool:
    value = clean_text(row.get("stored_measurement_eligible")).lower()
    return value in {"true", "1", "yes"}


def table_exists(conn: sqlite3.Connection, table_name: str) -> bool:
    row = conn.execute(
        "select 1 from sqlite_master where type = 'table' and name = ?",
        (table_name,),
    ).fetchone()
    return row is not None


def table_columns(conn: sqlite3.Connection, table_name: str) -> set[str]:
    if not table_exists(conn, table_name):
        return set()
    return {str(row[1]) for row in conn.execute(f"pragma table_info({table_name})")}


def ensure_table_columns(conn: sqlite3.Connection, table_name: str, columns: dict[str, str]) -> None:
    existing = table_columns(conn, table_name)
    for column, sql_type in columns.items():
        if column not in existing:
            conn.execute(f"alter table {table_name} add column {column} {sql_type}")


def stable_digest(parts: list[Any]) -> str:
    payload = "\x1f".join(str(part or "") for part in parts)
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:24]


def risk_score(row: pd.Series, median_cycle: float, p75_cycle: float) -> int:
    age = float(row.get("age_days") or 0)
    stale = float(row.get("stale_days") or 0)
    predicted = float(row.get("predicted_total_cycle_days") or median_cycle)
    lines = float(row.get("total_lines_changed") or 0)
    linked = float(row.get("linked_ticket_count") or 0)
    comments = float(row.get("comments") or 0) + float(row.get("review_comments") or 0)
    requested_reviewers = float(row.get("requested_reviewer_count") or 0)
    overdue = float(row.get("overdue_days") or 0)
    if row.get("state") != "open":
        return 0
    score = 20
    if age > median_cycle:
        score += 20
    if age > p75_cycle:
        score += 20
    if stale >= 7:
        score += 15
    if predicted > p75_cycle:
        score += 10
    if lines > 1000:
        score += 10
    if linked > 1:
        score += 8
    if comments > 8:
        score += 8
    if requested_reviewers > 0 and stale >= 3:
        score += 8
    if overdue > 0:
        score += 10
    return min(100, int(score))


def risk_band(score: int) -> str:
    if score >= 80:
        return "critical"
    if score >= 60:
        return "high"
    if score >= 35:
        return "medium"
    return "low"


def blocker_severity(text: str) -> int:
    lowered = text.lower()
    severity = 1
    for term in ["blocked", "blocker", "stuck", "unable", "cannot", "regression", "failing", "timeout"]:
        if term in lowered:
            severity += 2
    if "?" in text:
        severity += 1
    return min(severity, 5)


def blocker_matches(text: str) -> list[re.Match[str]]:
    return [match for match in BLOCKER_RE.finditer(text or "") if not is_ignored_blocker_match(text or "", match)]


def first_blocker_match(text: str) -> re.Match[str] | None:
    matches = blocker_matches(text)
    return matches[0] if matches else None


def is_ignored_blocker_match(text: str, match: re.Match[str]) -> bool:
    window = text[max(0, match.start() - 100) : min(len(text), match.end() + 100)]
    if NEGATED_BLOCKER_RE.search(window):
        return True
    if BOILERPLATE_BLOCKER_RE.search(window):
        return True
    signal = match.group(0).lower()
    if signal in {"dependency", "dependencies"} and re.search(r"(?is)dependencies\s*\(does it add or upgrade a dependency\)\s*:[^\n\r]*", window):
        return True
    if signal == "error" and re.search(r"(?is)(simplelogger|silence.{0,80}error|log\.[^\s=]+\s*=\s*error)", window):
        return True
    return False


def severity_label(value: int) -> str:
    if value >= 5:
        return "high"
    if value >= 3:
        return "medium"
    return "low"


def format_days(value: Any) -> str:
    if value is None:
        return "unknown"
    if isinstance(value, float) and math.isnan(value):
        return "unknown"
    try:
        numeric = float(value)
    except (TypeError, ValueError):
        return str(value)
    rounded = round(numeric, 1)
    if rounded.is_integer():
        return f"{int(rounded)}d"
    return f"{rounded:.1f}d"


def write_report(
    path: Path,
    analysis_at: str,
    pr_features: pd.DataFrame,
    pr_forecasts: pd.DataFrame,
    ticket_features: pd.DataFrame,
    blocker_candidates: pd.DataFrame,
    dependency_edges: pd.DataFrame,
    review_bottlenecks: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    forecast_reliability: pd.DataFrame,
    forecast_backtest: pd.DataFrame,
    forecast_feature_set_readiness: pd.DataFrame,
    forecast_risk_backtest: pd.DataFrame,
    decision_target_backtest: pd.DataFrame,
    decision_target_readiness: pd.DataFrame,
    milestone_signals: pd.DataFrame,
    feature_provenance: pd.DataFrame,
    pr_source_coverage: pd.DataFrame,
    developer_correlation: pd.DataFrame,
    developer_correlation_validation: pd.DataFrame,
    time_series_summary: pd.DataFrame,
    transition_signal_readiness: pd.DataFrame,
    insight_cards: pd.DataFrame,
    review_queue: pd.DataFrame,
    evaluation_readiness: pd.DataFrame,
) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = [
        "# Flink AI TPM Analytics (Pre-Follow-Up)",
        "",
        f"Observed through: {analysis_at}",
        "",
        "## Scope",
        "",
        f"- PR feature rows: {len(pr_features)}",
        f"- Ticket feature rows: {len(ticket_features)}",
        f"- Forecast rows: {len(pr_forecasts)}",
        f"- Source coverage-limited PRs: {source_coverage_limited_count(pr_source_coverage)}",
        f"- Direct developer correlation leads: {developer_correlation_lead_count(developer_correlation)}",
        f"- Transition candidates: {time_series_metric_value(time_series_summary, 'transition_candidate_count')}",
        f"- Blocker candidates: {len(blocker_candidates)}",
        f"- Dependency edges/components: {len(dependency_edges)}",
        f"- Requested-reviewer leads: {len(review_bottlenecks)}",
        f"- Forecast backtest rows: {len(forecast_backtest)}",
        f"- Forecast feature-set readiness rows: {len(forecast_feature_set_readiness)}",
        f"- TPM decision-target backtest rows: {len(decision_target_backtest)}",
        f"- TPM decision-target readiness rows: {len(decision_target_readiness)}",
        f"- Milestone/date signal rows: {len(milestone_signals)}",
        f"- Explicit due-date commitments: {milestone_signal_count(milestone_signals, 'explicit_commitment')}",
        f"- Feature provenance rows: {len(feature_provenance)}",
        f"- Insight cards: {len(insight_cards)}",
        f"- Insight review rows: {len(review_queue)}",
        "",
        "## Cycle Risk Summary",
        "",
        df_to_markdown(forecast_summary) if not forecast_summary.empty else "No forecast summary generated.",
        "",
        "## Forecast Product Reliability",
        "",
        df_to_markdown(forecast_reliability) if not forecast_reliability.empty else "No forecast product reliability rows generated.",
        "",
        "## Forecast Backtest",
        "",
        df_to_markdown(forecast_backtest_for_report(forecast_backtest)) if not forecast_backtest.empty else "No forecast backtest generated.",
        "",
        "## Forecast Feature-Set Readiness Matrix",
        "",
        forecast_feature_set_readiness_report_markdown(forecast_feature_set_readiness),
        "",
        "## Forecast Risk-Triage Backtest",
        "",
        forecast_risk_backtest_report_markdown(forecast_risk_backtest),
        "",
        "## TPM Decision-Target Backtest",
        "",
        decision_target_backtest_report_markdown(decision_target_backtest),
        "",
        "## TPM Decision-Target Readiness",
        "",
        decision_target_readiness_report_markdown(decision_target_readiness),
        "",
        "## Milestone And Date Signals",
        "",
        milestone_signal_report_markdown(milestone_signals),
        "",
        "## Feature Provenance",
        "",
        df_to_markdown(feature_provenance) if not feature_provenance.empty else "No feature provenance generated.",
        "",
        "## Source Coverage",
        "",
        source_coverage_report_markdown(pr_source_coverage),
        "",
        "## Developer Correlation",
        "",
        "### Aggregate Validation",
        "",
        developer_correlation_validation_report_markdown(developer_correlation_validation),
        "",
        "### Guardrailed Leads",
        "",
        developer_correlation_report_markdown(developer_correlation),
        "",
        "## Time-Series History",
        "",
        df_to_markdown(time_series_summary) if not time_series_summary.empty else "No time-series snapshot summary generated.",
        "",
        "## Transition Signal Readiness",
        "",
        transition_signal_readiness_report_markdown(transition_signal_readiness),
        "",
        "## Highest-Risk Open PRs",
        "",
        df_to_markdown(top_prs(pr_forecasts)) if not pr_forecasts.empty else "No PR forecast rows.",
        "",
        "## Requested Reviewer Leads",
        "",
        df_to_markdown(top_review_bottlenecks(review_bottlenecks)) if not review_bottlenecks.empty else "No review wait candidates found.",
        "",
        "## Top Keyword Blocker Candidates",
        "",
        df_to_markdown(top_blockers(blocker_candidates)) if not blocker_candidates.empty else "No blocker candidates found.",
        "",
        "## Insight Cards",
        "",
        df_to_markdown(insight_cards_for_report(insight_cards).head(20)) if not insight_cards.empty else "No insight cards generated.",
        "",
        "## Evaluation Readiness",
        "",
        df_to_markdown(evaluation_readiness) if not evaluation_readiness.empty else "No evaluation readiness metrics.",
        "",
        "## Review Queue",
        "",
        df_to_markdown(review_queue_for_report(review_queue).head(20)) if not review_queue.empty else "No review queue rows.",
        "",
        "## Interpretation",
        "",
        "- This data supports TPM-style risk triage, blocker candidate detection, requested-reviewer surfacing, dependency clustering, and status synthesis.",
        "- TPM decision-target backtests are validation evidence for prioritization only; they do not authorize autonomous merge, close, park, or reassignment decisions.",
        "- Cycle estimates are prioritization/risk scores, not reliable ETA promises unless `eta_forecast_ready` is true. The dataset has one repo, one search page, and limited complete review/comment history.",
        "- Source coverage-limited PRs preserve older typed facts but do not support current-state, absence, or completion claims until source coverage is refreshed.",
        "- Developer correlation compares directly bridged GitHub/Jira Person rows against same-window extra Jira tickets. It is workload context for TPM follow-up, not proof of causality, ownership, or blocker absence.",
        "- Raw blocker candidates are keyword hits. Only current/open candidates become persisted `blocker_candidate` insight cards. Requested-reviewer leads require typed requested-reviewer rows and open PR state, but do not prove reviewer inactivity because request-event time is not modeled yet.",
        "- This run persists first-class `work_insights` rows with typed PR/ticket subjects and conservative partial/restricted quality flags. Blocker insights now cite source text spans when available; forecast insights still use generated-output evidence because they are derived feature-vector risk scores.",
        "- This run also persists `work_insight_reviews` triage requests. Truth/actionability labels live in separate human or imported assessment rows, not on producer-owned insight rows.",
        "- Follow-up observation can mark some generated insights stale after this report is written; use `tpm_action_brief.md` or `tpm_current_insight_cards` for the post-follow-up operating view.",
        "- The next iteration should add time-series snapshots, Jira transitions, and labeled review outcomes so forecast error and blocker precision can be measured over time.",
    ]
    path.write_text("\n".join(lines) + "\n")


def forecast_backtest_for_report(forecast_backtest: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "evaluation",
        "model",
        "fold",
        "train_count",
        "test_count",
        "mae_days",
        "median_error_days",
        "p75_error_days",
        "improvement_vs_median_pct",
        "ready_for_eta",
        "note",
    ]
    if forecast_backtest.empty:
        return pd.DataFrame(columns=columns)
    existing = [column for column in columns if column in forecast_backtest.columns]
    return forecast_backtest[existing].sort_values(
        ["evaluation", "fold", "mae_days"],
        ascending=[True, True, True],
    ).head(30)


def forecast_feature_set_readiness_report_markdown(forecast_feature_set_readiness: pd.DataFrame) -> str:
    columns = [
        "feature_set_key",
        "feature_policy",
        "model",
        "kfold_mae_days",
        "kfold_improvement_pct",
        "chronological_mae_days",
        "chronological_improvement_pct",
        "same_model_backtest_gate",
        "as_of_snapshot_gate",
        "eta_promotable",
        "guardrail_state",
        "note",
    ]
    if forecast_feature_set_readiness.empty:
        return "No forecast feature-set readiness matrix generated."
    existing = [column for column in columns if column in forecast_feature_set_readiness.columns]
    return df_to_markdown(forecast_feature_set_readiness[existing])


def transition_signal_readiness_report_markdown(transition_signal_readiness: pd.DataFrame) -> str:
    if transition_signal_readiness.empty:
        return "No transition signal readiness rows."
    columns = [
        "readiness_key",
        "support_level",
        "ready",
        "readiness_state",
        "transition_candidate_count",
        "terminal_transition_candidate_count",
        "latest_terminal_transition_subject_count",
        "superseded_terminal_transition_count",
        "as_of_feature_snapshot_ready",
        "blocking_reason",
        "recommended_action",
    ]
    existing = [column for column in columns if column in transition_signal_readiness.columns]
    return df_to_markdown(transition_signal_readiness[existing])


def forecast_risk_backtest_report_markdown(forecast_risk_backtest: pd.DataFrame) -> str:
    columns = ["metric", "value", "sample_count", "method", "interpretation"]
    if forecast_risk_backtest.empty:
        return "No forecast risk-triage backtest generated."
    existing = [column for column in columns if column in forecast_risk_backtest.columns]
    return df_to_markdown(forecast_risk_backtest[existing])


def decision_target_backtest_report_markdown(decision_target_backtest: pd.DataFrame) -> str:
    columns = [
        "target_kind",
        "evaluation",
        "model",
        "fold",
        "train_count",
        "test_count",
        "positive_count",
        "baseline_positive_rate",
        "precision_at_10pct",
        "lift_at_10pct",
        "roc_auc",
        "average_precision",
        "coverage_stratum",
        "ready_for_product_action",
        "note",
    ]
    if decision_target_backtest.empty:
        return "No TPM decision-target backtest generated."
    existing = [column for column in columns if column in decision_target_backtest.columns]
    return df_to_markdown(decision_target_backtest[existing].head(40))


def decision_target_readiness_report_markdown(decision_target_readiness: pd.DataFrame) -> str:
    columns = [
        "target_kind",
        "model",
        "grouped_kfold_fold_count",
        "grouped_kfold_mean_lift_at_10pct",
        "grouped_kfold_min_lift_at_10pct",
        "chronological_lift_at_10pct",
        "chronological_precision_at_10pct",
        "chronological_roc_auc",
        "coverage_gate_state",
        "coverage_stratum_count",
        "validation_ready",
        "coverage_ready",
        "independent_evidence_ready",
        "owner_policy_ready",
        "same_model_validation_gate",
        "product_action_gate_state",
        "product_action_ready",
        "recommended_next_evidence",
        "note",
    ]
    if decision_target_readiness.empty:
        return "No TPM decision-target readiness rows generated."
    existing = [column for column in columns if column in decision_target_readiness.columns]
    return df_to_markdown(decision_target_readiness[existing])


def milestone_signal_count(milestone_signals: pd.DataFrame, commitment_strength: str) -> int:
    if milestone_signals.empty or "commitment_strength" not in milestone_signals.columns:
        return 0
    return int((milestone_signals["commitment_strength"].astype(str) == commitment_strength).sum())


def milestone_signal_report_markdown(milestone_signals: pd.DataFrame) -> str:
    if milestone_signals.empty:
        return "No milestone or source date signals generated."
    rows = []
    for (milestone_kind, commitment_strength, date_claim_allowed, delivery_commitment_allowed), group in milestone_signals.groupby(
        ["milestone_kind", "commitment_strength", "date_claim_allowed", "delivery_commitment_allowed"],
        dropna=False,
    ):
        target_dates = group["target_date"].fillna("").astype(str).str.strip() if "target_date" in group.columns else pd.Series([], dtype=str)
        outcome_dates = group["outcome_date"].fillna("").astype(str).str.strip() if "outcome_date" in group.columns else pd.Series([], dtype=str)
        rows.append(
            {
                "milestone_kind": milestone_kind,
                "commitment_strength": commitment_strength,
                "date_claim_allowed": int(bool(date_claim_allowed)),
                "delivery_commitment_allowed": int(bool(delivery_commitment_allowed)),
                "count": int(len(group)),
                "dated_count": int(target_dates.ne("").sum()),
                "outcome_count": int(outcome_dates.ne("").sum()),
            }
        )
    summary = pd.DataFrame(rows).sort_values(["delivery_commitment_allowed", "date_claim_allowed", "count"], ascending=[False, False, False])
    top_columns = [
        "subject_key",
        "milestone_kind",
        "milestone_name",
        "target_date",
        "outcome_date",
        "milestone_state",
        "commitment_strength",
        "claim_gate_reason",
    ]
    existing = [column for column in top_columns if column in milestone_signals.columns]
    top = milestone_signals[existing].head(20)
    return "\n\n".join(
        [
            df_to_markdown(summary),
            "Top signals:",
            df_to_markdown(top),
            "Forecasts and generated due buckets are intentionally excluded from this table.",
        ]
    )


def source_coverage_limited_count(pr_source_coverage: pd.DataFrame) -> int:
    if pr_source_coverage.empty or "source_current_coverage_state" not in pr_source_coverage.columns:
        return 0
    return int(pr_source_coverage["source_current_coverage_state"].isin(["detail_failed", "coverage_limited"]).sum())


def time_series_metric_value(time_series_summary: pd.DataFrame, metric: str) -> str:
    if time_series_summary.empty:
        return "0"
    rows = time_series_summary[time_series_summary["metric"] == metric]
    if rows.empty:
        return "0"
    return str(rows.iloc[0]["value"])


def source_coverage_for_report(pr_source_coverage: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "subject_key",
        "source_current_coverage_state",
        "source_current_issue_count",
        "source_current_detail_issue_count",
        "source_current_issue_codes",
        "source_current_issue_kinds",
        "source_current_sync_run_key",
    ]
    if pr_source_coverage.empty:
        return pd.DataFrame(columns=columns)
    rows = pr_source_coverage[pr_source_coverage["source_current_coverage_state"].isin(["detail_failed", "coverage_limited"])].copy()
    if rows.empty:
        return pd.DataFrame(columns=columns)
    existing = [column for column in columns if column in rows.columns]
    return rows[existing].head(25)


def source_coverage_report_markdown(pr_source_coverage: pd.DataFrame) -> str:
    rows = source_coverage_for_report(pr_source_coverage)
    if rows.empty:
        return "No coverage-limited PRs in the latest fixture source sync."
    return df_to_markdown(rows)


def developer_correlation_lead_count(developer_correlation: pd.DataFrame) -> int:
    if developer_correlation.empty:
        return 0
    rows = developer_correlation[
        (developer_correlation["correlation_state"] == "correlatable_same_identity")
        & (developer_correlation["extra_jira_ticket_count"] > 0)
        & (developer_correlation["pr_authored_count"] > 0)
    ]
    return int(len(rows))


def developer_correlation_for_report(developer_correlation: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "display_name",
        "github_login",
        "jira_key",
        "correlation_state",
        "identity_match_method",
        "source_coverage_state",
        "pr_authored_count",
        "open_pr_authored_count",
        "high_risk_open_pr_count",
        "extra_jira_ticket_count",
        "open_extra_jira_ticket_count",
        "extra_jira_blocker_ticket_count",
        "same_window_ticket_pressure",
        "correlation_score",
        "confidence",
        "top_pr_subjects",
        "top_extra_ticket_keys",
        "recommended_tpm_action",
    ]
    if developer_correlation.empty:
        return pd.DataFrame(columns=columns)
    rows = developer_correlation[developer_correlation["correlation_state"] == "correlatable_same_identity"].copy()
    if rows.empty:
        return pd.DataFrame(columns=columns)
    existing = [column for column in columns if column in rows.columns]
    return rows[existing].sort_values(
        ["correlation_score", "high_risk_open_pr_count", "open_extra_jira_ticket_count"],
        ascending=[False, False, False],
    ).head(20)


def developer_correlation_report_markdown(developer_correlation: pd.DataFrame) -> str:
    rows = developer_correlation_for_report(developer_correlation)
    if rows.empty:
        return "No direct GitHub/Jira identity correlation leads found. Keep co-occurrence rows as identity-resolution candidates only."
    return df_to_markdown(rows)


def developer_correlation_validation_report_markdown(developer_correlation_validation: pd.DataFrame) -> str:
    columns = ["metric", "value", "sample_count", "method", "interpretation"]
    if developer_correlation_validation.empty:
        return "No aggregate developer-correlation validation rows generated."
    existing = [column for column in columns if column in developer_correlation_validation.columns]
    return df_to_markdown(developer_correlation_validation[existing])


def top_prs(pr_forecasts: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "repository",
        "pr_number",
        "state",
        "age_days",
        "stale_days",
        "slow_cycle_risk_threshold_days",
        "age_past_threshold_days",
        "risk_score",
        "risk_band",
        "requested_reviewer_count",
        "requested_reviewers",
        "title",
    ]
    detail_state = pr_forecasts.get("source_current_detail_state", pd.Series("observed", index=pr_forecasts.index))
    rows = pr_forecasts[(pr_forecasts["state"] == "open") & (detail_state != "failed")].sort_values(["risk_score", "age_days"], ascending=[False, False]).copy()
    rows["slow_cycle_risk_threshold_days"] = rows["predicted_total_cycle_days"]
    rows["age_past_threshold_days"] = rows["overdue_days"]
    return rows[columns].head(15)


def top_blockers(blocker_candidates: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "candidate_kind",
        "candidate_scope",
        "subject_state",
        "product_key",
        "actor",
        "signal",
        "severity",
        "evidence_locator_kind",
        "evidence_span_start",
        "evidence_span_end",
        "evidence_excerpt",
    ]
    existing = [column for column in columns if column in blocker_candidates.columns]
    return blocker_candidates[existing].head(15)


def top_review_bottlenecks(review_bottlenecks: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "repository",
        "pr_number",
        "severity",
        "bottleneck_score",
        "requested_reviewer_count",
        "requested_reviewers",
        "age_days",
        "stale_days",
        "title",
        "bottleneck_reason",
    ]
    existing = [column for column in columns if column in review_bottlenecks.columns]
    return review_bottlenecks[existing].head(15)


def insight_cards_for_report(insight_cards: pd.DataFrame) -> pd.DataFrame:
    return insight_cards.drop(columns=["identity_key"], errors="ignore")


def review_queue_for_report(review_queue: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "insight_kind",
        "severity",
        "subject_kind",
        "subject_key",
        "review_state",
        "truth_label",
        "actionability_label",
        "next_action",
    ]
    existing = [column for column in columns if column in review_queue.columns]
    return review_queue[existing]


def df_to_markdown(df: pd.DataFrame) -> str:
    if df.empty:
        return ""
    columns = list(df.columns)
    rows = []
    rows.append("| " + " | ".join(columns) + " |")
    rows.append("| " + " | ".join(["---"] * len(columns)) + " |")
    for _, row in df.iterrows():
        rows.append("| " + " | ".join(markdown_cell(row[column]) for column in columns) + " |")
    return "\n".join(rows)


def markdown_cell(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, float) and math.isnan(value):
        return ""
    text = str(value).replace("\n", " ").replace("|", "\\|")
    return re.sub(r"\s+", " ", text).strip()


def clean_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, float) and math.isnan(value):
        return ""
    return str(value)


def first_nonempty(values: list[Any]) -> str:
    for value in values:
        text = clean_text(value).strip()
        if text:
            return text
    return ""


def has_value(value: Any) -> bool:
    if value is None:
        return False
    if isinstance(value, float) and math.isnan(value):
        return False
    return True


def clean_int(value: Any) -> int | None:
    if value is None:
        return None
    if isinstance(value, float) and math.isnan(value):
        return None
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def clean_bool(value: Any) -> bool:
    if value is None:
        return False
    if isinstance(value, float) and math.isnan(value):
        return False
    if isinstance(value, bool):
        return value
    return str(value).strip().lower() in {"1", "true", "yes", "y"}


def metric_map(df: pd.DataFrame) -> dict[str, str]:
    if df.empty or "metric" not in df.columns or "value" not in df.columns:
        return {}
    return {str(row.metric): str(row.value) for row in df.itertuples(index=False)}


def format_optional_float(value: Any) -> str:
    numeric = safe_float(value)
    return "" if numeric is None else f"{numeric:.2f}"


def safe_float(value: Any) -> float | None:
    if value is None:
        return None
    if isinstance(value, float) and math.isnan(value):
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def improvement_pct(baseline: Any, candidate: Any) -> float | None:
    baseline_value = safe_float(baseline)
    candidate_value = safe_float(candidate)
    if baseline_value is None or candidate_value is None or baseline_value <= 0:
        return None
    return round(((baseline_value - candidate_value) / baseline_value) * 100.0, 2)


def write_table(conn: sqlite3.Connection, name: str, df: pd.DataFrame) -> None:
    df.to_sql(name, conn, if_exists="replace", index=False)


def parse_dt(value: Any) -> datetime | None:
    if not value:
        return None
    if isinstance(value, datetime):
        dt = value
    else:
        text = str(value).replace("Z", "+00:00")
        try:
            dt = datetime.fromisoformat(text)
        except ValueError:
            try:
                dt = datetime.strptime(str(value), "%Y-%m-%dT%H:%M:%S.%f%z")
            except ValueError:
                return None
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc)


def days_between(start: datetime | None, end: datetime | None) -> float | None:
    if start is None or end is None:
        return None
    return max(0.0, (end - start).total_seconds() / 86400.0)


def iso_or_none(value: datetime | None) -> str | None:
    return value.isoformat() if value else None


def stringify_description(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, str):
        return value
    return json.dumps(value)


def display_name(user: dict[str, Any]) -> str:
    return (user.get("displayName") or user.get("name") or user.get("login") or user.get("key") or "").strip()


def user_key(user: dict[str, Any]) -> str:
    return (user.get("key") or user.get("name") or user.get("login") or "").strip()


def snippet(text: str, index: int, window: int = 180) -> str:
    start = max(0, index - window // 2)
    end = min(len(text), index + window // 2)
    if start > 0:
        prior_space = max(text.rfind(" ", 0, start), text.rfind("\n", 0, start), text.rfind("\r", 0, start), text.rfind("\t", 0, start))
        if prior_space >= 0 and index - prior_space <= window:
            start = prior_space + 1
    if end < len(text):
        suffix = text[end:]
        next_space = re.search(r"\s", suffix)
        if next_space is not None and end + next_space.start() - index <= window:
            end += next_space.start()
    return redact_secret_tokens(re.sub(r"\s+", " ", text[start:end]).strip())


def redact_secret_tokens(text: str) -> str:
    return SECRET_TOKEN_RE.sub("[REDACTED_TOKEN]", text)


if __name__ == "__main__":
    main()
