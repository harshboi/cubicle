#!/usr/bin/env python3
"""Observe current source state for queued AI-TPM insights.

This script is intentionally observational. It records whether a subject moved
after the fixture snapshot, but it does not write truth/actionability labels.
Human or imported labels still belong in WorkInsightReview rows.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import math
import os
import re
import shlex
import sqlite3
import subprocess
import urllib.error
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import pandas as pd


GITHUB_PR_RE = re.compile(r"^https://github\.com/(?P<owner>[^/]+)/(?P<repo>[^/]+)/pull/(?P<number>\d+)")
ISSUE_KEY_RE = re.compile(r"(?i)\bFLINK-\d+\b")


@dataclass(frozen=True)
class FetchResult:
    url: str
    status_code: int | None
    payload: dict[str, Any] | None
    error: str
    auth_state: str = "anonymous"
    coverage_kind: str = "public_api_current_observation"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ontology-db", required=True, type=Path)
    parser.add_argument("--analytics-db", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    parser.add_argument("--observed-at", default=datetime.now(timezone.utc).isoformat())
    parser.add_argument("--timeout-seconds", type=float, default=20.0)
    parser.add_argument("--github-token-env", default="GITHUB_TOKEN", help="environment variable containing a GitHub token; GH_TOKEN and GITHUB_TOKEN are also checked")
    parser.add_argument("--github-token-command", default="", help="optional local command that prints a GitHub token, such as 'gh auth token'; output is not logged or persisted")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    observed_at = parse_dt(args.observed_at) or datetime.now(timezone.utc)
    github_token = github_token_from_env(args.github_token_env) or github_token_from_command(args.github_token_command)
    with sqlite3.connect(args.ontology_db) as ontology_conn:
        ontology_subjects = read_current_insight_subjects(ontology_conn)

    with sqlite3.connect(args.analytics_db) as analytics_conn:
        baseline = read_baseline_subjects(analytics_conn)
        analytics_subjects = read_current_analytics_insight_subjects(
            analytics_conn,
            infer_source_instance(ontology_subjects, args.analytics_db),
        )
    subjects = merge_current_insight_subjects(ontology_subjects, analytics_subjects)
    source_instance = infer_source_instance(subjects, args.analytics_db)

    rows: list[dict[str, Any]] = []
    for subject in subjects.itertuples(index=False):
        baseline_row = baseline.get((subject.subject_kind, subject.subject_key), {})
        result = fetch_subject(subject.subject_kind, subject.subject_key, subject.source_url, args.timeout_seconds, github_token)
        rows.append(build_observation_row(subject, baseline_row, result, observed_at))

    observations = pd.DataFrame(rows)
    summary = build_summary(observations)
    with sqlite3.connect(args.ontology_db) as ontology_conn:
        sync_counts = persist_followup_sync_run(ontology_conn, observations, observed_at, source_instance)
        terminal_stale_count = stale_terminal_work_insights(ontology_conn, observations, observed_at)
        summary = append_summary_row(summary, "source_sync_run_created_count", str(sync_counts["run_created_count"]), "follow-up source sync run rows created or refreshed")
        summary = append_summary_row(summary, "source_sync_issue_created_count", str(sync_counts["issue_created_count"]), "follow-up coverage failures persisted as SourceSyncIssue rows")
        summary = append_summary_row(summary, "ontology_terminal_staled_count", str(terminal_stale_count), "current insights marked stale because follow-up observed terminal source state")
        summary = append_summary_row(summary, "ontology_coverage_failed_staled_count", "0", "coverage failures remain sync issues and never stale product insights")
    args.analytics_db.parent.mkdir(parents=True, exist_ok=True)
    with sqlite3.connect(args.analytics_db) as analytics_conn:
        annotate_analytics_insight_card_states(analytics_conn, observations)
        time_series_counts = persist_followup_time_series_snapshots(analytics_conn, source_instance, observations)
        summary = append_summary_row(summary, "followup_pr_state_snapshot_upsert_count", str(time_series_counts["pr_snapshot_count"]), "live follow-up PR state snapshots upserted into TPM history")
        summary = append_summary_row(summary, "followup_ticket_state_snapshot_upsert_count", str(time_series_counts["ticket_snapshot_count"]), "live follow-up ticket state snapshots upserted into TPM history")
        summary = append_summary_row(summary, "followup_transition_candidate_count", str(time_series_counts["transition_candidate_count"]), "transition candidates available after live follow-up snapshots")
        observations.to_sql("tpm_followup_observations", analytics_conn, if_exists="replace", index=False)
        summary.to_sql("tpm_followup_summary", analytics_conn, if_exists="replace", index=False)

    write_report(args.report, observed_at, observations, summary)


def read_current_insight_subjects(conn: sqlite3.Connection) -> pd.DataFrame:
    return pd.read_sql_query(
        """
        select
          group_concat(wi.key, '|') as insight_keys,
          group_concat(wi.insight_kind, '|') as insight_kinds,
          count(*) as insight_row_count,
          max(case wi.severity
            when 'critical' then 5
            when 'high' then 4
            when 'medium' then 3
            when 'low' then 2
            else 1
          end) as severity_rank,
          wi.subject_kind,
          wi.subject_key,
          max(wi.source_instance) as source_instance,
          max(wi.source_url) as source_url,
          max(wi.score) as score,
          max(wi.confidence) as confidence
        from work_insights wi
        where wi.producer_state = 'current'
          and wi.subject_kind in ('pull_request', 'ticket')
        group by wi.subject_kind, wi.subject_key
        order by
          severity_rank desc,
          max(wi.rank_score) desc,
          wi.subject_key
        """,
        conn,
    )


def read_current_analytics_insight_subjects(conn: sqlite3.Connection, source_instance: str) -> pd.DataFrame:
    table_name = "tpm_current_insight_cards" if table_exists(conn, "tpm_current_insight_cards") else "tpm_insight_cards"
    if not table_exists(conn, table_name):
        return empty_current_insight_subjects()
    where = "subject_kind in ('pull_request', 'ticket')"
    if column_exists(conn, table_name, "producer_state"):
        where += " and producer_state = 'current'"
    return pd.read_sql_query(
        f"""
        select
          group_concat(coalesce(nullif(identity_key, ''), insight_kind), '|') as insight_keys,
          group_concat(insight_kind, '|') as insight_kinds,
          count(*) as insight_row_count,
          max(case severity
            when 'critical' then 5
            when 'high' then 4
            when 'medium' then 3
            when 'low' then 2
            else 1
          end) as severity_rank,
          subject_kind,
          subject_key,
          ? as source_instance,
          coalesce(max(nullif(source_url, '')), max(nullif(evidence_source_url, '')), '') as source_url,
          max(score) as score,
          max(confidence) as confidence
        from {table_name}
        where {where}
        group by subject_kind, subject_key
        order by
          severity_rank desc,
          max(score) desc,
          subject_key
        """,
        conn,
        params=(source_instance,),
    )


def merge_current_insight_subjects(ontology_subjects: pd.DataFrame, analytics_subjects: pd.DataFrame) -> pd.DataFrame:
    frames: list[pd.DataFrame] = []
    for rank, frame in ((2, ontology_subjects), (1, analytics_subjects)):
        if frame.empty:
            continue
        copy = frame.copy()
        copy["_subject_source_rank"] = rank
        frames.append(copy)
    if not frames:
        return empty_current_insight_subjects()
    combined = pd.concat(frames, ignore_index=True)
    combined["severity_rank"] = pd.to_numeric(combined["severity_rank"], errors="coerce").fillna(0)
    combined["score"] = pd.to_numeric(combined["score"], errors="coerce").fillna(0.0)
    combined = combined.sort_values(
        ["subject_kind", "subject_key", "_subject_source_rank", "severity_rank", "score"],
        ascending=[True, True, False, False, False],
    )
    combined = combined.drop_duplicates(["subject_kind", "subject_key"], keep="first")
    return combined.drop(columns=["_subject_source_rank"])[current_insight_subject_columns()]


def empty_current_insight_subjects() -> pd.DataFrame:
    return pd.DataFrame(columns=current_insight_subject_columns())


def current_insight_subject_columns() -> list[str]:
    return [
        "insight_keys",
        "insight_kinds",
        "insight_row_count",
        "severity_rank",
        "subject_kind",
        "subject_key",
        "source_instance",
        "source_url",
        "score",
        "confidence",
    ]


def read_baseline_subjects(conn: sqlite3.Connection) -> dict[tuple[str, str], dict[str, Any]]:
    baseline: dict[tuple[str, str], dict[str, Any]] = {}
    if table_exists(conn, "tpm_pr_features"):
        pr_rows = pd.read_sql_query(
            """
            select
              repository,
              pr_number,
              state,
              title,
              updated_at,
              closed_at,
              merged_at
            from tpm_pr_features
            """,
            conn,
        )
        for row in pr_rows.itertuples(index=False):
            baseline[("pull_request", f"{row.repository}#{int(row.pr_number)}")] = {
                "state": row.state,
                "title": row.title,
                "updated_at": row.updated_at,
                "closed_at": row.closed_at,
                "merged_at": row.merged_at,
            }
    if table_exists(conn, "tpm_ticket_features"):
        ticket_rows = pd.read_sql_query(
            """
            select ticket_key, status, title, updated_at
            from tpm_ticket_features
            """,
            conn,
        )
        for row in ticket_rows.itertuples(index=False):
            baseline[("ticket", str(row.ticket_key).upper())] = {
                "state": normalize_ticket_state(row.status),
                "title": row.title,
                "updated_at": row.updated_at,
            }
    return baseline


def fetch_subject(subject_kind: str, subject_key: str, source_url: str, timeout_seconds: float, github_token: str) -> FetchResult:
    if subject_kind == "pull_request":
        api_url = github_api_url(subject_key, source_url)
        if not api_url:
            return FetchResult("", None, None, "unable_to_derive_github_api_url", "none", "not_observed")
        return fetch_json(api_url, timeout_seconds, github_token=github_token)
    if subject_kind == "ticket":
        issue_key = extract_issue_key(subject_key, source_url)
        if not issue_key:
            return FetchResult("", None, None, "unable_to_derive_jira_issue_key", "none", "not_observed")
        return fetch_json(f"https://issues.apache.org/jira/rest/api/2/issue/{issue_key}", timeout_seconds)
    return FetchResult("", None, None, f"unsupported_subject_kind:{subject_kind}", "none", "not_observed")


def github_api_url(subject_key: str, source_url: str) -> str:
    if "#" in subject_key:
        repo, number = subject_key.rsplit("#", 1)
        return f"https://api.github.com/repos/{repo}/pulls/{number}"
    match = GITHUB_PR_RE.match(source_url or "")
    if not match:
        return ""
    return f"https://api.github.com/repos/{match.group('owner')}/{match.group('repo')}/pulls/{match.group('number')}"


def extract_issue_key(subject_key: str, source_url: str) -> str:
    match = ISSUE_KEY_RE.search(subject_key or "")
    if match:
        return match.group(0).upper()
    match = ISSUE_KEY_RE.search(source_url or "")
    return match.group(0).upper() if match else ""


def fetch_json(url: str, timeout_seconds: float, github_token: str = "") -> FetchResult:
    headers = {
        "Accept": "application/json",
        "User-Agent": "cubicle-ai-tpm-followup/0.1",
    }
    auth_state = "anonymous"
    coverage_kind = "public_api_current_observation"
    if github_token and url.startswith("https://api.github.com/"):
        headers["Authorization"] = f"Bearer {github_token}"
        auth_state = "github_token"
        coverage_kind = "authenticated_api_current_observation"
    request = urllib.request.Request(
        url,
        headers=headers,
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
            raw = response.read()
            return FetchResult(url, int(response.status), json.loads(raw.decode("utf-8")), "", auth_state, coverage_kind)
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8", errors="replace")[:500]
        return FetchResult(url, int(err.code), None, f"http_error:{err.code}:{body}", auth_state, coverage_kind)
    except Exception as err:  # noqa: BLE001 - record source observation failure.
        return FetchResult(url, None, None, f"{type(err).__name__}:{err}", auth_state, coverage_kind)


def build_observation_row(subject: Any, baseline: dict[str, Any], result: FetchResult, observed_at: datetime) -> dict[str, Any]:
    current = normalize_current_state(subject.subject_kind, result.payload)
    baseline_state = str(baseline.get("state") or "")
    current_state = current.get("state", "")
    state_changed = ""
    if baseline_state and current_state:
        state_changed = "true" if baseline_state != current_state else "false"
    current_updated_at = current.get("updated_at", "")
    baseline_updated_at = str(baseline.get("updated_at") or "")
    days_since_source_update = ""
    current_updated_dt = parse_dt(current_updated_at)
    if current_updated_dt is not None:
        days_since_source_update = f"{max(0.0, (observed_at - current_updated_dt).total_seconds() / 86400.0):.2f}"
    return {
        "observed_at": observed_at.isoformat(),
        "insight_keys": subject.insight_keys,
        "insight_kinds": subject.insight_kinds,
        "insight_row_count": int(subject.insight_row_count),
        "severity": severity_from_rank(int(subject.severity_rank or 0)),
        "subject_kind": subject.subject_kind,
        "subject_key": subject.subject_key,
        "source_instance": subject.source_instance,
        "source_url": subject.source_url,
        "fetch_url": result.url,
        "fetch_status_code": result.status_code,
        "fetch_error": result.error,
        "fetch_auth_state": result.auth_state,
        "fetch_coverage_kind": result.coverage_kind,
        "baseline_state": baseline_state,
        "current_state": current_state,
        "state_changed": state_changed,
        "baseline_title": baseline.get("title", ""),
        "current_title": current.get("title", ""),
        "baseline_updated_at": baseline_updated_at,
        "current_updated_at": current_updated_at,
        "current_closed_at": current.get("closed_at", ""),
        "current_merged_at": current.get("merged_at", ""),
        "days_since_source_update": days_since_source_update,
        "outcome_signal": outcome_signal(subject.insight_kinds, subject.subject_kind, baseline_state, current_state),
        "observation_payload": json.dumps(current, sort_keys=True),
    }


def normalize_current_state(subject_kind: str, payload: dict[str, Any] | None) -> dict[str, str]:
    if not payload:
        return {}
    if subject_kind == "pull_request":
        merged_at = payload.get("merged_at") or ""
        state = "merged" if merged_at else str(payload.get("state") or "unknown")
        return {
            "state": state,
            "title": str(payload.get("title") or ""),
            "updated_at": str(payload.get("updated_at") or ""),
            "closed_at": str(payload.get("closed_at") or ""),
            "merged_at": str(merged_at),
        }
    if subject_kind == "ticket":
        fields = payload.get("fields") or {}
        status = ((fields.get("status") or {}).get("name") or "")
        return {
            "state": normalize_ticket_state(status),
            "title": str(fields.get("summary") or payload.get("key") or ""),
            "updated_at": str(fields.get("updated") or ""),
            "closed_at": str(fields.get("resolutiondate") or ""),
            "merged_at": "",
        }
    return {}


def normalize_ticket_state(status: str) -> str:
    lowered = (status or "").strip().lower()
    if lowered in {"closed", "done", "resolved", "complete", "completed"}:
        return "closed"
    if lowered:
        return "open"
    return "unknown"


def outcome_signal(insight_kinds: str, subject_kind: str, baseline_state: str, current_state: str) -> str:
    if not current_state:
        return "not_observed"
    if baseline_state and baseline_state != current_state:
        if subject_kind == "pull_request" and current_state in {"merged", "closed"}:
            return "subject_became_terminal"
        if subject_kind == "ticket" and current_state == "closed":
            return "subject_became_closed"
        return "subject_state_changed"
    if current_state == "open" and any(kind in {"forecast_risk", "blocker_candidate"} for kind in split_kinds(insight_kinds)):
        return "still_open"
    return "no_state_change"


def build_summary(observations: pd.DataFrame) -> pd.DataFrame:
    if observations.empty:
        return pd.DataFrame([{"metric": "observation_count", "value": "0", "note": "no subjects observed"}])
    success = observations[observations["fetch_status_code"] == 200]
    insight_row_count = int(pd.to_numeric(observations["insight_row_count"], errors="coerce").fillna(0).sum())
    rows = [
        {"metric": "observation_count", "value": str(len(observations)), "note": "unique subjects requested for follow-up"},
        {"metric": "unique_subject_count", "value": str(len(observations)), "note": "unique subjects requested for follow-up"},
        {"metric": "insight_row_count", "value": str(insight_row_count), "note": "current insight rows represented by the subject observations"},
        {"metric": "fetch_success_count", "value": str(len(success)), "note": "unique subject requests with HTTP 200"},
        {"metric": "anonymous_fetch_success_count", "value": str(int((success["fetch_auth_state"] == "anonymous").sum())), "note": "HTTP 200 follow-ups from anonymous public APIs"},
        {"metric": "authenticated_fetch_success_count", "value": str(int((success["fetch_auth_state"] != "anonymous").sum())), "note": "HTTP 200 follow-ups using an auth token"},
        {"metric": "fetch_error_count", "value": str(int((observations["fetch_status_code"] != 200).sum())), "note": "unique subject requests that failed or were unavailable"},
        {"metric": "state_changed_count", "value": str(int((observations["state_changed"] == "true").sum())), "note": "unique subjects whose normalized state changed since fixture baseline"},
        {"metric": "terminal_transition_count", "value": str(int(observations["outcome_signal"].isin(["subject_became_terminal", "subject_became_closed"]).sum())), "note": "unique subjects that became terminal/closed after the fixture baseline"},
    ]
    for signal, count in observations.groupby("outcome_signal").size().items():
        rows.append({"metric": f"outcome_{signal}", "value": str(int(count)), "note": "follow-up observation outcome signal"})
    for insight_kind, count in count_insight_kinds(observations).items():
        rows.append({"metric": f"observations_{insight_kind}", "value": str(int(count)), "note": "insight rows represented by kind"})
    return pd.DataFrame(rows)


def write_report(path: Path, observed_at: datetime, observations: pd.DataFrame, summary: pd.DataFrame) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = [
        "# Flink AI TPM Follow-up Observations",
        "",
        f"Observed at: {observed_at.isoformat()}",
        "",
        "## Summary",
        "",
        df_to_markdown(summary),
        "",
        "## State Changes",
        "",
        df_to_markdown(observations[observations["state_changed"] == "true"].head(30)) if not observations.empty else "No observations.",
        "",
        "## Open Follow-ups",
        "",
        df_to_markdown(observations[observations["outcome_signal"] == "still_open"].head(30)) if not observations.empty else "No observations.",
        "",
        "## Interpretation",
        "",
        "- These rows are follow-up source observations, not truth/actionability labels.",
        "- Successful fetches here are anonymous public API observations unless `authenticated_fetch_success_count` is nonzero.",
        "- Terminal state changes can help prioritize review, but they do not prove blocker precision by themselves.",
        "- Failed source requests remain observation failures and must not be normalized as absence.",
    ]
    path.write_text("\n".join(lines) + "\n")


def stale_terminal_work_insights(conn: sqlite3.Connection, observations: pd.DataFrame, observed_at: datetime) -> int:
    if observations.empty:
        return 0
    terminal = observations[observations["outcome_signal"].isin(["subject_became_terminal", "subject_became_closed"])]
    if terminal.empty:
        return 0
    updated = 0
    observed_iso = observed_at.isoformat()
    for row in terminal.itertuples(index=False):
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
                  and external_kind = 'tpm_insight'
                  and producer_state = 'current'
                  and subject_kind = ?
                  and subject_key = ?
                  and latest_evidence_id is not null
            )
            """,
            (observed_iso, row.subject_kind, row.subject_key),
        )
        cursor = conn.execute(
            """
            update work_insights
            set producer_state = 'stale',
                freshness_state = 'stale',
                updated_at = ?
            where source_system = 'cubicle_analytics'
              and external_kind = 'tpm_insight'
              and producer_state = 'current'
              and subject_kind = ?
              and subject_key = ?
            """,
            (observed_iso, row.subject_kind, row.subject_key),
        )
        updated += int(cursor.rowcount or 0)
    conn.commit()
    return updated


def persist_followup_sync_run(
    conn: sqlite3.Connection,
    observations: pd.DataFrame,
    observed_at: datetime,
    source_instance: str,
) -> dict[str, int]:
    if observations.empty:
        return {"run_created_count": 0, "issue_created_count": 0}
    observed_iso = observed_at.isoformat()
    scope_id = ensure_followup_scope(conn, source_instance, observed_iso)
    failed = observations[observations["fetch_status_code"] != 200].copy()
    issue_count = len(failed)
    any_rate_limited = any(sync_issue_code(row.fetch_status_code, row.fetch_error) == "source_rate_limited" for row in failed.itertuples(index=False))
    if issue_count == 0:
        status = "complete"
        coverage_mode = "live_only"
        error_code = None
        error_message = None
    else:
        status = "rate_limited" if any_rate_limited else "partial"
        coverage_mode = "partial_scope"
        error_code = "source_rate_limited" if any_rate_limited else "source_followup_partial"
        error_message = f"{issue_count} of {len(observations)} follow-up source reads failed; failures are coverage evidence, not product absence."
    run_key = followup_run_key(source_instance, observed_iso)
    conn.execute(
        """
        insert into source_sync_runs (
          run_key, sync_mode, coverage_mode, status, started_at, completed_at,
          objects_seen_count, issues_created_count, error_code, error_message,
          created_at, updated_at, source_scope_id
        ) values (
          ?, 'federated_live', ?, ?, ?, ?,
          ?, ?, ?, ?,
          ?, ?, ?
        )
        on conflict(source_scope_id, run_key) do update set
          coverage_mode = excluded.coverage_mode,
          status = excluded.status,
          completed_at = excluded.completed_at,
          objects_seen_count = excluded.objects_seen_count,
          issues_created_count = excluded.issues_created_count,
          error_code = excluded.error_code,
          error_message = excluded.error_message,
          updated_at = excluded.updated_at
        """,
        (
            run_key,
            coverage_mode,
            status,
            observed_iso,
            observed_iso,
            len(observations),
            issue_count,
            error_code,
            error_message,
            observed_iso,
            observed_iso,
            scope_id,
        ),
    )
    run_id = int(conn.execute("select id from source_sync_runs where source_scope_id = ? and run_key = ?", (scope_id, run_key)).fetchone()[0])
    conn.execute("delete from source_sync_issues where source_sync_run_id = ?", (run_id,))
    for row in failed.itertuples(index=False):
        source_system, source_instance_for_issue, external_kind, external_id = issue_source_identity(row.subject_kind, row.subject_key)
        conn.execute(
            """
            insert into source_sync_issues (
              severity, issue_code, message, source_system, source_instance,
              external_kind, external_id, source_url, created_at, updated_at,
              source_scope_id, source_sync_run_id
            ) values (
              'warning', ?, ?, ?, ?,
              ?, ?, ?, ?, ?,
              ?, ?
            )
            """,
            (
                sync_issue_code(row.fetch_status_code, row.fetch_error),
                sync_issue_message(row.fetch_status_code, row.fetch_error),
                source_system,
                source_instance_for_issue,
                external_kind,
                external_id,
                row.fetch_url or row.source_url or "",
                observed_iso,
                observed_iso,
                scope_id,
                run_id,
            ),
        )
    refresh_current_source_sync_issue_view(conn)
    conn.commit()
    return {"run_created_count": 1, "issue_created_count": issue_count}


def refresh_current_source_sync_issue_view(conn: sqlite3.Connection) -> None:
    conn.execute("drop view if exists current_source_sync_issues")
    conn.execute(
        """
        create view current_source_sync_issues as
        select ssi.*
        from source_sync_issues ssi
        join source_sync_runs ssr on ssr.id = ssi.source_sync_run_id
        join (
            select ranked.source_scope_id, ranked.id as latest_run_id
            from (
                select
                  id,
                  source_scope_id,
                  row_number() over (
                    partition by source_scope_id
                    order by coalesce(completed_at, started_at, created_at) desc, id desc
                  ) as row_rank
                from source_sync_runs
            ) ranked
            where ranked.row_rank = 1
        ) latest on latest.latest_run_id = ssr.id
        """
    )


def annotate_analytics_insight_card_states(conn: sqlite3.Connection, observations: pd.DataFrame) -> None:
    if not table_exists(conn, "tpm_insight_cards"):
        return
    cards = pd.read_sql_query("select * from tpm_insight_cards", conn)
    if cards.empty:
        cards.to_sql("tpm_current_insight_cards", conn, if_exists="replace", index=False)
        return
    cards["producer_state"] = "current"
    cards["stale_reason"] = ""
    if not observations.empty:
        terminal = observations[observations["outcome_signal"].isin(["subject_became_terminal", "subject_became_closed"])]
        terminal_keys = {(str(row.subject_kind), str(row.subject_key)) for row in terminal.itertuples(index=False)}
        if terminal_keys:
            mask = cards.apply(lambda row: (str(row.get("subject_kind", "")), str(row.get("subject_key", ""))) in terminal_keys, axis=1)
            cards.loc[mask, "producer_state"] = "stale"
            cards.loc[mask, "stale_reason"] = "terminal_followup_observed"
    cards.to_sql("tpm_insight_cards", conn, if_exists="replace", index=False)
    cards[cards["producer_state"] == "current"].to_sql("tpm_current_insight_cards", conn, if_exists="replace", index=False)


def persist_followup_time_series_snapshots(
    conn: sqlite3.Connection,
    source_instance: str,
    observations: pd.DataFrame,
) -> dict[str, int]:
    ensure_time_series_tables(conn)
    if observations.empty:
        refresh_state_transition_candidates(conn)
        refresh_time_series_summary(conn, source_instance)
        return {"pr_snapshot_count": 0, "ticket_snapshot_count": 0, "transition_candidate_count": transition_count(conn, source_instance)}
    successful = observations[(observations["fetch_status_code"] == 200) & (observations["current_state"].astype(str) != "")].copy()
    pr_count = upsert_followup_pr_state_snapshots(conn, source_instance, successful[successful["subject_kind"] == "pull_request"])
    ticket_count = upsert_followup_ticket_state_snapshots(conn, source_instance, successful[successful["subject_kind"] == "ticket"])
    refresh_state_transition_candidates(conn)
    refresh_time_series_summary(conn, source_instance)
    conn.commit()
    return {"pr_snapshot_count": pr_count, "ticket_snapshot_count": ticket_count, "transition_candidate_count": transition_count(conn, source_instance)}


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


def upsert_followup_pr_state_snapshots(conn: sqlite3.Connection, source_instance: str, observations: pd.DataFrame) -> int:
    rows = []
    captured_at = datetime.now(timezone.utc).isoformat()
    for row in observations.itertuples(index=False):
        subject_key = str(row.subject_key)
        if "#" not in subject_key:
            continue
        repo, number = subject_key.rsplit("#", 1)
        observed_at = normalize_iso(row.observed_at)
        snapshot_key = f"tpm-pr-state-snapshot:{stable_digest([source_instance, observed_at, subject_key])}"
        rows.append(
            (
                snapshot_key,
                source_instance,
                observed_at,
                subject_key,
                repo,
                int(number),
                str(row.current_state or ""),
                str(row.current_title or row.baseline_title or subject_key),
                str(row.source_url or ""),
                "",
                str(row.current_updated_at or ""),
                str(row.current_closed_at or ""),
                str(row.current_merged_at or ""),
                None,
                safe_float(row.days_since_source_update),
                None,
                None,
                "",
                "live_followup_observation",
                "observed",
                "observed",
                "",
                "",
                "live_followup_observation",
                "",
                "",
                captured_at,
            )
        )
    if not rows:
        return 0
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
          source_updated_at = excluded.source_updated_at,
          closed_at = excluded.closed_at,
          merged_at = excluded.merged_at,
          stale_days = excluded.stale_days,
          forecast_method = excluded.forecast_method,
          source_current_coverage_state = excluded.source_current_coverage_state,
          source_current_detail_state = excluded.source_current_detail_state,
          lifecycle_fields_source = excluded.lifecycle_fields_source,
          captured_at = excluded.captured_at
        """,
        rows,
    )
    return len(rows)


def upsert_followup_ticket_state_snapshots(conn: sqlite3.Connection, source_instance: str, observations: pd.DataFrame) -> int:
    rows = []
    captured_at = datetime.now(timezone.utc).isoformat()
    for row in observations.itertuples(index=False):
        ticket_key = str(row.subject_key or "").upper()
        if not ticket_key:
            continue
        observed_at = normalize_iso(row.observed_at)
        snapshot_key = f"tpm-ticket-state-snapshot:{stable_digest([source_instance, observed_at, ticket_key])}"
        rows.append(
            (
                snapshot_key,
                source_instance,
                observed_at,
                ticket_key,
                str(row.current_state or ""),
                "",
                str(row.current_title or row.baseline_title or ticket_key),
                str(row.current_updated_at or ""),
                0,
                0,
                0,
                0,
                0,
                0,
                captured_at,
            )
        )
    if not rows:
        return 0
    conn.executemany(
        """
        insert into tpm_ticket_state_snapshots (
          snapshot_key, source_instance, observed_at, ticket_key, status, priority, title,
          updated_at, linked_pr_count, fresh_pr_link_count, partial_pr_link_count,
          comment_count, participant_count, blocker_keyword_count, captured_at
        ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        on conflict(snapshot_key) do update set
          status = excluded.status,
          title = excluded.title,
          updated_at = excluded.updated_at,
          captured_at = excluded.captured_at
        """,
        rows,
    )
    return len(rows)


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


def refresh_time_series_summary(conn: sqlite3.Connection, source_instance: str) -> None:
    rows = [
        ("pr_state_snapshot_count", scalar_int(conn, "select count(*) from tpm_pr_state_snapshots where source_instance = ?", (source_instance,)), "Append/upserted PR state observations available for trend and transition analysis."),
        ("ticket_state_snapshot_count", scalar_int(conn, "select count(*) from tpm_ticket_state_snapshots where source_instance = ?", (source_instance,)), "Append/upserted ticket state observations available for trend and transition analysis."),
        (
            "observed_snapshot_time_count",
            scalar_int(
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
            "Distinct source-observed timestamps across PR and ticket snapshots.",
        ),
        ("transition_candidate_count", transition_count(conn, source_instance), "Adjacent snapshot state or coverage changes detected; candidates require source evidence validation."),
        (
            "terminal_transition_candidate_count",
            scalar_int(conn, "select count(*) from tpm_state_transition_candidates where source_instance = ? and transition_kind = 'terminal_state_change'", (source_instance,)),
            "Adjacent snapshot changes into merged/closed-like terminal states.",
        ),
    ]
    conn.execute(
        """
        create table if not exists tpm_time_series_summary (
          metric text,
          value text,
          note text
        )
        """
    )
    conn.execute("delete from tpm_time_series_summary")
    conn.executemany("insert into tpm_time_series_summary(metric, value, note) values (?, ?, ?)", [(metric, str(value), note) for metric, value, note in rows])


def transition_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not table_exists(conn, "tpm_state_transition_candidates"):
        return 0
    return scalar_int(conn, "select count(*) from tpm_state_transition_candidates where source_instance = ?", (source_instance,))


def scalar_int(conn: sqlite3.Connection, query: str, params: tuple[Any, ...] = ()) -> int:
    row = conn.execute(query, params).fetchone()
    if row is None:
        return 0
    return int(row[0] or 0)


def normalize_iso(value: Any) -> str:
    dt = parse_dt(value)
    return dt.isoformat() if dt is not None else str(value or "")


def safe_float(value: Any) -> float | None:
    if value is None:
        return None
    if isinstance(value, float) and math.isnan(value):
        return None
    text = str(value).strip()
    if not text:
        return None
    try:
        return float(text)
    except ValueError:
        return None


def stable_digest(parts: list[Any]) -> str:
    return hashlib.sha256("\n".join(str(part) for part in parts).encode("utf-8")).hexdigest()[:24]


def ensure_followup_scope(conn: sqlite3.Connection, source_instance: str, now: str) -> int:
    connection_key = f"source-connection:cubicle-followup:{source_instance}"
    conn.execute(
        """
        insert into source_connections (
          key, source_system, source_instance, display_name, connector_kind,
          is_enabled, last_synced_at, created_at, updated_at
        ) values (
          ?, 'cubicle_followup', ?, ?, 'public_api_followup',
          true, ?, ?, ?
        )
        on conflict(key) do update set
          last_synced_at = excluded.last_synced_at,
          updated_at = excluded.updated_at
        """,
        (connection_key, source_instance, f"AI TPM follow-up for {source_instance}", now, now, now),
    )
    connection_id = int(conn.execute("select id from source_connections where key = ?", (connection_key,)).fetchone()[0])
    scope_key = f"source-scope:cubicle-followup:{source_instance}"
    conn.execute(
        """
        insert into source_scopes (
          key, scope_kind, scope_key, display_name, crawl_policy,
          is_enabled, created_at, updated_at, source_connection_id
        ) values (
          ?, 'workstream_followup', ?, ?, 'public_api_followup',
          true, ?, ?, ?
        )
        on conflict(key) do update set
          updated_at = excluded.updated_at,
          source_connection_id = excluded.source_connection_id
        """,
        (scope_key, source_instance, f"AI TPM follow-up for {source_instance}", now, now, connection_id),
    )
    return int(conn.execute("select id from source_scopes where key = ?", (scope_key,)).fetchone()[0])


def infer_source_instance(subjects: pd.DataFrame, analytics_db: Path) -> str:
    if not subjects.empty and "source_instance" in subjects.columns:
        values = [str(value) for value in subjects["source_instance"].dropna().tolist() if str(value)]
        if values:
            return values[0]
    return analytics_db.parent.name


def github_token_from_env(primary: str) -> str:
    keys = [primary, "GH_TOKEN", "GITHUB_TOKEN"]
    for key in keys:
        key = str(key or "").strip()
        if not key:
            continue
        token = os.environ.get(key, "").strip()
        if token:
            return token
    return ""


def github_token_from_command(command: str) -> str:
    command = str(command or "").strip()
    if not command:
        return ""
    try:
        completed = subprocess.run(
            shlex.split(command),
            check=False,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            text=True,
            timeout=10,
        )
    except Exception:
        return ""
    if completed.returncode != 0:
        return ""
    return completed.stdout.strip().splitlines()[0].strip() if completed.stdout.strip() else ""


def followup_run_key(source_instance: str, observed_iso: str) -> str:
    digest = hashlib.sha256(f"{source_instance}\n{observed_iso}".encode("utf-8")).hexdigest()[:16]
    return f"source-sync-run:{source_instance}:tpm-followup:{digest}"


def issue_source_identity(subject_kind: str, subject_key: str) -> tuple[str, str, str, str]:
    if subject_kind == "pull_request" and "#" in subject_key:
        repo, _ = subject_key.rsplit("#", 1)
        return "github", repo, "github_pull_request", subject_key
    if subject_kind == "ticket":
        return "jira", "issues.apache.org", "jira_issue", subject_key
    return "unknown", "", subject_kind or "unknown", subject_key


def sync_issue_code(status_code: Any, fetch_error: str) -> str:
    text = str(fetch_error or "").lower()
    try:
        status = int(status_code)
    except (TypeError, ValueError):
        status = 0
    if status == 429 or "rate limit" in text:
        return "source_rate_limited"
    if status == 403:
        return "source_forbidden"
    if status:
        return "source_non_200"
    return "source_unavailable"


def sync_issue_message(status_code: Any, fetch_error: str) -> str:
    try:
        status = int(status_code)
    except (TypeError, ValueError):
        status = 0
    if status:
        return f"follow-up source request returned status {status}; retained as coverage failure, not product absence"
    return f"follow-up source request failed before HTTP status; retained as coverage failure: {str(fetch_error or '')[:240]}"


def append_summary_row(summary: pd.DataFrame, metric: str, value: str, note: str) -> pd.DataFrame:
    row = pd.DataFrame([{"metric": metric, "value": value, "note": note}])
    if summary.empty:
        return row
    return pd.concat([summary, row], ignore_index=True)


def count_insight_kinds(observations: pd.DataFrame) -> dict[str, int]:
    counts: dict[str, int] = {}
    for row in observations.itertuples(index=False):
        for kind in split_kinds(row.insight_kinds):
            counts[kind] = counts.get(kind, 0) + 1
    return counts


def split_kinds(value: str) -> list[str]:
    return [part for part in str(value or "").split("|") if part]


def severity_from_rank(rank: int) -> str:
    if rank >= 5:
        return "critical"
    if rank == 4:
        return "high"
    if rank == 3:
        return "medium"
    if rank == 2:
        return "low"
    return "info"


def df_to_markdown(df: pd.DataFrame) -> str:
    if df.empty:
        return ""
    columns = list(df.columns)
    rows = ["| " + " | ".join(columns) + " |", "| " + " | ".join(["---"] * len(columns)) + " |"]
    for _, row in df.iterrows():
        rows.append("| " + " | ".join(markdown_cell(row[column]) for column in columns) + " |")
    return "\n".join(rows)


def markdown_cell(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, float) and math.isnan(value):
        return ""
    return re.sub(r"\s+", " ", str(value).replace("\n", " ").replace("|", "\\|")).strip()


def parse_dt(value: Any) -> datetime | None:
    if not value:
        return None
    text = str(value).replace("Z", "+00:00")
    if re.search(r"[+-]\d{4}$", text):
        text = f"{text[:-5]}{text[-5:-2]}:{text[-2:]}"
    try:
        dt = datetime.fromisoformat(text)
    except ValueError:
        return None
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc)


def table_exists(conn: sqlite3.Connection, table_name: str) -> bool:
    row = conn.execute("select 1 from sqlite_master where type = 'table' and name = ?", (table_name,)).fetchone()
    return row is not None


def column_exists(conn: sqlite3.Connection, table_name: str, column_name: str) -> bool:
    return any(row[1] == column_name for row in conn.execute(f"pragma table_info({table_name})").fetchall())


if __name__ == "__main__":
    main()
