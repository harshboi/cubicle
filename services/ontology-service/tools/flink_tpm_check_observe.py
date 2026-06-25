#!/usr/bin/env python3
"""Observe GitHub check/status state for the bounded Flink PR fixture.

This is a live supplemental observer. It does not turn missing or forbidden
GitHub check data into product absence. Non-200 fetches are persisted as source
coverage issues. Only HTTP 200 check/status payloads can produce CI follow-up
WorkInsight rows.
"""

from __future__ import annotations

import argparse
import fnmatch
import hashlib
import json
import math
import os
import re
import shlex
import sqlite3
import subprocess
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import pandas as pd


CHECK_OBSERVER_VERSION = "2026-06-21.1"
SECRET_TOKEN_RE = re.compile(r"\b(?:ghp_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|xoxb-[A-Za-z0-9-]+)\b")
FAILING_CHECK_CONCLUSIONS = {"failure", "cancelled", "timed_out", "action_required", "startup_failure"}
NEUTRAL_CHECK_CONCLUSIONS = {"neutral", "skipped", "stale"}
PENDING_CHECK_STATUSES = {"queued", "in_progress", "waiting", "requested", "pending"}
FAILING_STATUS_STATES = {"failure", "error"}
PENDING_STATUS_STATES = {"pending", "expected"}
CHECK_CARD_COLUMNS = [
    "insight_kind",
    "severity",
    "subject_kind",
    "subject_key",
    "identity_key",
    "source_url",
    "title",
    "details",
    "recommended_action",
    "model_method",
    "score",
    "score_explanation",
    "confidence",
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
    "producer_state",
    "stale_reason",
]
REQUIRED_CHECK_OBSERVER_KIND = "github_required_status_checks"


@dataclass(frozen=True)
class FetchResult:
    url: str
    status_code: int | None
    payload: Any
    error: str
    auth_state: str = "anonymous"
    coverage_kind: str = "public_api_current_observation"
    complete: bool = True
    page_count: int = 0
    headers: dict[str, str] | None = None


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--fixture-dir", required=True, type=Path)
    parser.add_argument("--ontology-db", required=True, type=Path)
    parser.add_argument("--analytics-db", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    parser.add_argument("--observed-at", default=datetime.now(timezone.utc).isoformat())
    parser.add_argument("--timeout-seconds", type=float, default=20.0)
    parser.add_argument("--state-scope", choices=["all", "open"], default="all")
    parser.add_argument("--limit", type=int, default=0)
    parser.add_argument(
        "--github-token-env",
        default="GITHUB_TOKEN",
        help="environment variable containing a GitHub token; GH_TOKEN and GITHUB_TOKEN are also checked",
    )
    parser.add_argument(
        "--github-token-command",
        default="",
        help="optional local command that prints a GitHub token, such as 'gh auth token'; output is not logged or persisted",
    )
    parser.add_argument(
        "--no-refresh-pr-heads",
        action="store_true",
        help="use fixture PR head SHAs directly instead of first refreshing current PR payloads",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    observed_at = parse_dt(args.observed_at) or datetime.now(timezone.utc)
    github_token = github_token_from_env(args.github_token_env) or github_token_from_command(args.github_token_command)

    fixture_prs = read_fixture_prs(args.fixture_dir, args.ontology_db)
    if args.state_scope == "open":
        fixture_prs = fixture_prs[fixture_prs["fixture_state"] == "open"].copy()
    if args.limit > 0:
        fixture_prs = fixture_prs.head(args.limit).copy()

    raw_dir = args.fixture_dir / "github" / "check-observations"
    rows: list[dict[str, Any]] = []
    for fixture_row in fixture_prs.itertuples(index=False):
        rows.append(
            observe_pr_checks(
                fixture_row._asdict(),
                observed_at,
                args.timeout_seconds,
                github_token,
                raw_dir,
                refresh_pr_heads=not args.no_refresh_pr_heads,
            )
        )

    observations = pd.DataFrame(rows)
    summary = build_summary(observations, fixture_prs)
    check_readiness = build_check_signal_readiness(observed_at, observations, summary)
    check_cards = build_ci_insight_cards(observations)

    args.analytics_db.parent.mkdir(parents=True, exist_ok=True)
    with sqlite3.connect(args.analytics_db) as analytics_conn:
        observations.to_sql("tpm_pr_check_observations", analytics_conn, if_exists="replace", index=False)
        summary.to_sql("tpm_check_summary", analytics_conn, if_exists="replace", index=False)
        check_readiness.to_sql("tpm_check_signal_readiness", analytics_conn, if_exists="replace", index=False)
        check_cards.to_sql("tpm_check_insight_cards", analytics_conn, if_exists="replace", index=False)
        merge_ci_cards_into_analytics(analytics_conn, check_cards)

    with sqlite3.connect(args.ontology_db) as ontology_conn:
        sync_counts = persist_check_sync_run(ontology_conn, observations, observed_at, args.fixture_dir.name)
        insight_counts = persist_ci_work_insights(ontology_conn, check_cards, observed_at, args.fixture_dir.name)
        persist_ci_review_requests(ontology_conn, args.fixture_dir.name, observed_at)
        summary = append_summary_row(summary, "source_sync_run_created_count", str(sync_counts["run_created_count"]), "check observer SourceSyncRun rows created or refreshed")
        summary = append_summary_row(summary, "source_sync_issue_created_count", str(sync_counts["issue_created_count"]), "check observer coverage failures persisted as SourceSyncIssue rows")
        summary = append_summary_row(summary, "ontology_ci_insight_current_count", str(insight_counts["current_count"]), "current CI WorkInsight rows produced from HTTP 200 check/status payloads")
        summary = append_summary_row(summary, "ontology_ci_insight_staled_count", str(insight_counts["staled_count"]), "prior CI WorkInsight rows marked stale before this observation")

    with sqlite3.connect(args.analytics_db) as analytics_conn:
        summary.to_sql("tpm_check_summary", analytics_conn, if_exists="replace", index=False)

    write_report(args.report, observed_at, observations, summary, check_readiness, check_cards)


def read_fixture_prs(fixture_dir: Path, ontology_db: Path) -> pd.DataFrame:
    manifest = fixture_dir / "manifest.ndjson"
    manifest_rows: dict[str, dict[str, Any]] = {}
    for line in manifest.read_text().splitlines():
        if not line.strip():
            continue
        record = json.loads(line)
        if record.get("source") != "github" or record.get("source_object_type") != "github_pull_request":
            continue
        if int(record.get("status_code") or 0) != 200:
            continue
        payload_path = fixture_dir / str(record["path"])
        payload = json.loads(payload_path.read_text())
        repo, number = parse_source_object_id(str(record.get("source_object_id") or ""))
        if not repo:
            repo = repository_from_pr_payload(payload)
        pr_number = int(number or payload.get("number") or 0)
        subject_key = f"{repo}#{pr_number}"
        manifest_rows[subject_key] = {
            "repository": repo,
            "pr_number": pr_number,
            "subject_key": subject_key,
            "fixture_path": str(payload_path),
            "fixture_api_url": str(record.get("url") or payload.get("url") or ""),
            "fixture_html_url": str(payload.get("html_url") or ""),
            "fixture_state": normalize_pr_state(payload),
            "fixture_title": str(payload.get("title") or ""),
            "fixture_updated_at": str(payload.get("updated_at") or ""),
            "fixture_head_sha": str(((payload.get("head") or {}).get("sha")) or ""),
            "fixture_statuses_url": str(payload.get("statuses_url") or ""),
            "subject_source": "manifest_fallback",
        }
    rows = read_typed_pr_rows(ontology_db, manifest_rows)
    if not rows:
        rows = list(manifest_rows.values())
    out = pd.DataFrame(rows)
    if out.empty:
        return pd.DataFrame(
            columns=[
                "repository",
                "pr_number",
                "subject_key",
                "fixture_path",
                "fixture_api_url",
                "fixture_html_url",
                "fixture_state",
                "fixture_title",
                "fixture_updated_at",
                "fixture_head_sha",
                "fixture_statuses_url",
                "subject_source",
            ]
        )
    return out.sort_values(["repository", "pr_number"]).reset_index(drop=True)


def read_typed_pr_rows(ontology_db: Path, manifest_rows: dict[str, dict[str, Any]]) -> list[dict[str, Any]]:
    if not ontology_db.exists():
        return []
    with sqlite3.connect(ontology_db) as conn:
        if not table_exists(conn, "pull_requests"):
            return []
        typed = conn.execute(
            """
            select repository, number, state, title, source_url, source_updated_at
            from pull_requests
            where repository is not null
              and repository != ''
              and number is not null
              and state != 'unknown'
            order by repository, number
            """
        ).fetchall()
    rows: list[dict[str, Any]] = []
    for repository, number, state, title, source_url, source_updated_at in typed:
        pr_number = int(number)
        subject_key = f"{repository}#{pr_number}"
        manifest = manifest_rows.get(subject_key, {})
        rows.append(
            {
                "repository": repository,
                "pr_number": pr_number,
                "subject_key": subject_key,
                "fixture_path": manifest.get("fixture_path", ""),
                "fixture_api_url": manifest.get("fixture_api_url") or github_api_pr_url(repository, pr_number),
                "fixture_html_url": source_url or manifest.get("fixture_html_url", "") or github_pr_url(subject_key),
                "fixture_state": state or manifest.get("fixture_state", ""),
                "fixture_title": title or manifest.get("fixture_title", ""),
                "fixture_updated_at": source_updated_at or manifest.get("fixture_updated_at", ""),
                "fixture_head_sha": manifest.get("fixture_head_sha", ""),
                "fixture_statuses_url": manifest.get("fixture_statuses_url", ""),
                "subject_source": "typed_pull_request",
            }
        )
    return rows


def observe_pr_checks(
    fixture_row: dict[str, Any],
    observed_at: datetime,
    timeout_seconds: float,
    github_token: str,
    raw_dir: Path,
    refresh_pr_heads: bool,
) -> dict[str, Any]:
    repo = str(fixture_row["repository"])
    pr_number = int(fixture_row["pr_number"])
    subject_key = str(fixture_row["subject_key"])
    api_url = str(fixture_row["fixture_api_url"])
    html_url = str(fixture_row["fixture_html_url"]) or github_pr_url(subject_key)
    fixture_head_sha = str(fixture_row["fixture_head_sha"])
    fixture_statuses_url = str(fixture_row["fixture_statuses_url"])

    if refresh_pr_heads:
        pr_result = fetch_json(api_url, timeout_seconds, github_token=github_token)
        write_raw_fetch(raw_dir, repo, pr_number, "pull", pr_result)
    else:
        payload = json.loads(Path(fixture_row["fixture_path"]).read_text())
        pr_result = FetchResult(api_url, 200, payload, "", "fixture_snapshot", "fixture_snapshot", page_count=1)

    effective_payload = pr_result.payload if pr_result.status_code == 200 and isinstance(pr_result.payload, dict) else {}
    head_sha = str(((effective_payload.get("head") or {}).get("sha")) or fixture_head_sha)
    base_branch = str(((effective_payload.get("base") or {}).get("ref")) or "")
    head_source = "current_api" if pr_result.status_code == 200 and refresh_pr_heads else "fixture_snapshot"
    current_state = normalize_pr_state(effective_payload) if effective_payload else ""
    effective_state = current_state or str(fixture_row["fixture_state"])
    statuses_url = str(effective_payload.get("statuses_url") or fixture_statuses_url or "")
    check_runs_url = github_check_runs_url(repo, head_sha)
    required_checks_url = github_required_status_checks_url(repo, base_branch)

    if head_sha:
        check_result = fetch_github_paginated_json(check_runs_url, timeout_seconds, github_token=github_token, collection_key="check_runs")
        status_result = fetch_github_paginated_json(statuses_url, timeout_seconds, github_token=github_token) if statuses_url else FetchResult("", None, None, "missing_statuses_url", "none", "not_observed", complete=False)
    else:
        check_result = FetchResult("", None, None, "missing_head_sha", "none", "not_observed", complete=False)
        status_result = FetchResult("", None, None, "missing_head_sha", "none", "not_observed", complete=False)
    required_rest_result = fetch_json(required_checks_url, timeout_seconds, github_token=github_token) if base_branch else FetchResult("", None, None, "missing_base_branch", "none", "not_observed", complete=False)
    required_result = required_status_checks_result_with_graphql_fallback(repo, base_branch, required_rest_result, timeout_seconds, github_token)
    if required_result.url != required_rest_result.url or required_result.status_code != required_rest_result.status_code:
        write_raw_fetch(raw_dir, repo, pr_number, "required-status-checks-rest", required_rest_result)
        write_raw_fetch(raw_dir, repo, pr_number, "required-status-checks-graphql", required_result)
    write_raw_fetch(raw_dir, repo, pr_number, "check-runs", check_result)
    write_raw_fetch(raw_dir, repo, pr_number, "statuses", status_result)
    write_raw_fetch(raw_dir, repo, pr_number, "required-status-checks", required_result)

    check_counts = summarize_check_runs(check_result.payload if check_result.status_code == 200 else None)
    status_counts = summarize_commit_statuses(status_result.payload if status_result.status_code == 200 else None)
    signal = combined_signal(check_result, status_result, check_counts, status_counts)
    failing_names = sorted(set(check_counts["failing_names"] + status_counts["failing_names"]))
    pending_names = sorted(set(check_counts["pending_names"] + status_counts["pending_names"]))
    success_names = sorted(set(check_counts["success_names"] + status_counts["success_names"]))
    required_contexts = required_contexts_from_payload(required_result.payload if required_result.status_code == 200 else None)
    required_match = required_check_match(required_contexts, failing_names, pending_names, success_names, required_result)
    coverage_state = coverage_state_for(pr_result, check_result, status_result)
    return {
        "observed_at": observed_at.isoformat(),
        "repository": repo,
        "pr_number": pr_number,
        "subject_key": subject_key,
        "pr_url": html_url,
        "fixture_state": fixture_row["fixture_state"],
        "subject_source": fixture_row.get("subject_source", ""),
        "current_pr_state": current_state,
        "effective_state": effective_state,
        "fixture_title": fixture_row["fixture_title"],
        "fixture_updated_at": fixture_row["fixture_updated_at"],
        "base_branch": base_branch,
        "head_sha": head_sha,
        "head_source": head_source,
        "pr_fetch_url": pr_result.url,
        "pr_fetch_status_code": pr_result.status_code,
        "pr_fetch_complete": bool(pr_result.complete),
        "pr_fetch_page_count": int(pr_result.page_count or 0),
        "pr_fetch_error": redact(pr_result.error),
        "check_runs_url": check_result.url,
        "check_fetch_status_code": check_result.status_code,
        "check_fetch_complete": bool(check_result.complete),
        "check_fetch_page_count": int(check_result.page_count or 0),
        "check_fetch_error": redact(check_result.error),
        "statuses_url": status_result.url,
        "status_fetch_status_code": status_result.status_code,
        "status_fetch_complete": bool(status_result.complete),
        "status_fetch_page_count": int(status_result.page_count or 0),
        "status_fetch_error": redact(status_result.error),
        "required_checks_url": required_result.url,
        "required_checks_fetch_status_code": required_result.status_code,
        "required_checks_fetch_complete": bool(required_result.complete),
        "required_checks_fetch_error": redact(required_result.error),
        "fetch_auth_state": combined_auth_state([pr_result, check_result, status_result]),
        "fetch_coverage_kind": combined_coverage_kind([pr_result, check_result, status_result, required_result]),
        "source_coverage_state": coverage_state,
        "required_check_coverage_state": required_match["coverage_state"],
        "required_check_match_state": required_match["match_state"],
        "required_check_context_count": len(required_contexts),
        "required_check_contexts": ", ".join(required_contexts),
        "failing_required_context_count": len(required_match["failing_required"]),
        "pending_required_context_count": len(required_match["pending_required"]),
        "missing_required_context_count": len(required_match["missing_required"]),
        "successful_required_context_count": len(required_match["successful_required"]),
        "failing_required_contexts": ", ".join(required_match["failing_required"]),
        "pending_required_contexts": ", ".join(required_match["pending_required"]),
        "missing_required_contexts": ", ".join(required_match["missing_required"]),
        "successful_required_contexts": ", ".join(required_match["successful_required"][:20]),
        "combined_signal": signal,
        "check_run_count": check_counts["check_run_count"],
        "failing_check_run_count": check_counts["failing_count"],
        "pending_check_run_count": check_counts["pending_count"],
        "successful_check_run_count": check_counts["success_count"],
        "neutral_check_run_count": check_counts["neutral_count"],
        "status_context_count": status_counts["status_context_count"],
        "failing_status_context_count": status_counts["failing_count"],
        "pending_status_context_count": status_counts["pending_count"],
        "successful_status_context_count": status_counts["success_count"],
        "failing_context_count": len(failing_names),
        "pending_context_count": len(pending_names),
        "success_context_count": len(success_names),
        "failing_contexts": ", ".join(failing_names),
        "pending_contexts": ", ".join(pending_names),
        "success_contexts": ", ".join(success_names[:20]),
        "evidence_source_url": evidence_source_url_for(signal, check_result, status_result),
        "evidence_external_kind": evidence_external_kind_for(signal, check_counts, status_counts),
    }


def fetch_json(url: str, timeout_seconds: float, github_token: str = "") -> FetchResult:
    if not url:
        return FetchResult("", None, None, "missing_url", "none", "not_observed")
    headers = {
        "Accept": "application/vnd.github+json" if url.startswith("https://api.github.com/") else "application/json",
        "User-Agent": "cubicle-ai-tpm-check-observer/0.1",
    }
    auth_state = "anonymous"
    coverage_kind = "public_api_current_observation"
    if github_token and url.startswith("https://api.github.com/"):
        headers["Authorization"] = f"Bearer {github_token}"
        auth_state = "github_token"
        coverage_kind = "authenticated_api_current_observation"
    request = urllib.request.Request(url, headers=headers)
    try:
        with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
            raw = response.read()
            payload = json.loads(raw.decode("utf-8")) if raw else None
            return FetchResult(url, int(response.status), payload, "", auth_state, coverage_kind, complete=True, page_count=1, headers=dict(response.headers.items()))
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8", errors="replace")[:500]
        return FetchResult(url, int(err.code), None, redact(f"http_error:{err.code}:{body}"), auth_state, coverage_kind, complete=False, page_count=0)
    except Exception as err:  # noqa: BLE001 - preserve coverage failure details for source diagnostics.
        return FetchResult(url, None, None, redact(f"{type(err).__name__}:{err}"), auth_state, coverage_kind, complete=False, page_count=0)


def fetch_github_graphql(query: str, variables: dict[str, Any], timeout_seconds: float, github_token: str = "") -> FetchResult:
    url = "https://api.github.com/graphql"
    if not github_token:
        return FetchResult(url, None, None, "missing_github_token", "none", "not_observed", complete=False)
    body = json.dumps({"query": query, "variables": variables}).encode("utf-8")
    headers = {
        "Accept": "application/vnd.github+json",
        "Authorization": f"Bearer {github_token}",
        "Content-Type": "application/json",
        "User-Agent": "cubicle-ai-tpm-check-observer/0.1",
    }
    request = urllib.request.Request(url, data=body, headers=headers, method="POST")
    try:
        with urllib.request.urlopen(request, timeout=timeout_seconds) as response:
            raw = response.read()
            payload = json.loads(raw.decode("utf-8")) if raw else None
            errors = payload.get("errors") if isinstance(payload, dict) else None
            if errors:
                return FetchResult(url, int(response.status), payload, redact(f"graphql_errors:{json.dumps(errors)[:500]}"), "github_token", "authenticated_api_current_observation", complete=False, page_count=1, headers=dict(response.headers.items()))
            return FetchResult(url, int(response.status), payload, "", "github_token", "authenticated_api_current_observation", complete=True, page_count=1, headers=dict(response.headers.items()))
    except urllib.error.HTTPError as err:
        body_text = err.read().decode("utf-8", errors="replace")[:500]
        return FetchResult(url, int(err.code), None, redact(f"http_error:{err.code}:{body_text}"), "github_token", "authenticated_api_current_observation", complete=False, page_count=0)
    except Exception as err:  # noqa: BLE001 - preserve coverage failure details for source diagnostics.
        return FetchResult(url, None, None, redact(f"{type(err).__name__}:{err}"), "github_token", "authenticated_api_current_observation", complete=False, page_count=0)


def fetch_github_paginated_json(
    url: str,
    timeout_seconds: float,
    github_token: str = "",
    collection_key: str = "",
    max_pages: int = 20,
) -> FetchResult:
    if not url:
        return FetchResult("", None, None, "missing_url", "none", "not_observed", complete=False)
    first_url = append_query_param(url, "per_page", "100") if url.startswith("https://api.github.com/") else url
    current_url = first_url
    page_count = 0
    first_payload: Any = None
    aggregate_items: list[Any] = []
    last_result: FetchResult | None = None
    while current_url:
        if page_count >= max_pages:
            return FetchResult(
                first_url,
                200,
                aggregate_paginated_payload(first_payload, aggregate_items, collection_key),
                f"pagination_partial:max_pages:{max_pages}",
                last_result.auth_state if last_result else "anonymous",
                last_result.coverage_kind if last_result else "public_api_current_observation",
                complete=False,
                page_count=page_count,
            )
        result = fetch_json(current_url, timeout_seconds, github_token=github_token)
        last_result = result
        if result.status_code != 200:
            if page_count == 0:
                return result
            return FetchResult(
                first_url,
                result.status_code,
                aggregate_paginated_payload(first_payload, aggregate_items, collection_key),
                f"pagination_partial:page:{page_count + 1}:{result.error}",
                result.auth_state,
                result.coverage_kind,
                complete=False,
                page_count=page_count,
            )
        page_count += 1
        if first_payload is None:
            first_payload = result.payload
        aggregate_items.extend(extract_page_items(result.payload, collection_key))
        current_url = next_link(result.headers or {})
    return FetchResult(
        first_url,
        200,
        aggregate_paginated_payload(first_payload, aggregate_items, collection_key),
        "",
        last_result.auth_state if last_result else "anonymous",
        last_result.coverage_kind if last_result else "public_api_current_observation",
        complete=True,
        page_count=page_count,
    )


def summarize_check_runs(payload: Any) -> dict[str, Any]:
    runs = payload.get("check_runs", []) if isinstance(payload, dict) else []
    counts = {
        "check_run_count": len(runs),
        "failing_count": 0,
        "pending_count": 0,
        "success_count": 0,
        "neutral_count": 0,
        "failing_names": [],
        "pending_names": [],
        "success_names": [],
    }
    for run in runs:
        if not isinstance(run, dict):
            continue
        name = str(run.get("name") or run.get("external_id") or "unnamed-check")
        status = str(run.get("status") or "").lower()
        conclusion = str(run.get("conclusion") or "").lower()
        if conclusion in FAILING_CHECK_CONCLUSIONS:
            counts["failing_count"] += 1
            counts["failing_names"].append(name)
        elif status in PENDING_CHECK_STATUSES or (status != "completed" and not conclusion):
            counts["pending_count"] += 1
            counts["pending_names"].append(name)
        elif conclusion == "success":
            counts["success_count"] += 1
            counts["success_names"].append(name)
        elif conclusion in NEUTRAL_CHECK_CONCLUSIONS:
            counts["neutral_count"] += 1
        elif conclusion:
            counts["neutral_count"] += 1
        else:
            counts["pending_count"] += 1
            counts["pending_names"].append(name)
    return counts


def summarize_commit_statuses(payload: Any) -> dict[str, Any]:
    statuses = payload if isinstance(payload, list) else payload.get("statuses", []) if isinstance(payload, dict) else []
    latest_by_context: dict[str, dict[str, Any]] = {}
    for status in statuses:
        if not isinstance(status, dict):
            continue
        context = str(status.get("context") or status.get("name") or "unnamed-status")
        if context not in latest_by_context:
            latest_by_context[context] = status
    counts = {
        "status_context_count": len(latest_by_context),
        "failing_count": 0,
        "pending_count": 0,
        "success_count": 0,
        "failing_names": [],
        "pending_names": [],
        "success_names": [],
    }
    for context, status in latest_by_context.items():
        state = str(status.get("state") or "").lower()
        if state in FAILING_STATUS_STATES:
            counts["failing_count"] += 1
            counts["failing_names"].append(context)
        elif state in PENDING_STATUS_STATES:
            counts["pending_count"] += 1
            counts["pending_names"].append(context)
        elif state == "success":
            counts["success_count"] += 1
            counts["success_names"].append(context)
    return counts


def required_contexts_from_payload(payload: Any) -> list[str]:
    if not isinstance(payload, dict):
        return []
    contexts = {str(value) for value in payload.get("contexts", []) if str(value)}
    for check in payload.get("checks", []) or []:
        if not isinstance(check, dict):
            continue
        context = str(check.get("context") or "").strip()
        if context:
            contexts.add(context)
    return sorted(contexts)


BRANCH_PROTECTION_RULES_QUERY = """
query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    branchProtectionRules(first: 100) {
      totalCount
      nodes {
        pattern
        requiresStatusChecks
        requiredStatusCheckContexts
        matchingRefs(first: 100) {
          totalCount
          nodes {
            name
          }
        }
      }
    }
  }
}
"""


def required_status_checks_result_with_graphql_fallback(
    repo: str,
    branch: str,
    rest_result: FetchResult,
    timeout_seconds: float,
    github_token: str,
) -> FetchResult:
    if rest_result.status_code == 200 or rest_result.status_code != 404 or not repo or not branch:
        return rest_result
    owner, name = split_repo_name(repo)
    if not owner or not name:
        return rest_result
    graphql_result = fetch_github_graphql(
        BRANCH_PROTECTION_RULES_QUERY,
        {"owner": owner, "name": name},
        timeout_seconds,
        github_token=github_token,
    )
    return required_status_checks_result_from_branch_protection_graphql(repo, branch, rest_result, graphql_result) or rest_result


def required_status_checks_result_from_branch_protection_graphql(
    repo: str,
    branch: str,
    rest_result: FetchResult,
    graphql_result: FetchResult,
) -> FetchResult | None:
    if graphql_result.status_code != 200 or not graphql_result.complete:
        return None
    contexts = required_contexts_from_branch_protection_rules(graphql_result.payload, branch)
    if contexts is None:
        return None
    payload = {
        "contexts": contexts,
        "checks": [],
        "source": "github_graphql_branch_protection_rules",
        "rest_required_status_checks_url": rest_result.url,
        "rest_required_status_checks_status_code": rest_result.status_code,
        "branch_protection_rules": (((graphql_result.payload or {}).get("data") or {}).get("repository") or {}).get("branchProtectionRules"),
    }
    return FetchResult(
        "https://api.github.com/graphql#branchProtectionRules",
        200,
        payload,
        "rest_required_status_checks_404_graphql_branch_protection_observed",
        graphql_result.auth_state,
        graphql_result.coverage_kind,
        complete=True,
        page_count=1,
        headers=graphql_result.headers,
    )


def required_contexts_from_branch_protection_rules(payload: Any, branch: str) -> list[str] | None:
    rules = (((payload or {}).get("data") or {}).get("repository") or {}).get("branchProtectionRules")
    if not isinstance(rules, dict):
        return None
    nodes = rules.get("nodes")
    if not isinstance(nodes, list):
        return None
    contexts: set[str] = set()
    for node in nodes:
        if not isinstance(node, dict) or not branch_protection_rule_matches_branch(node, branch):
            continue
        if not bool(node.get("requiresStatusChecks")):
            continue
        for context in node.get("requiredStatusCheckContexts") or []:
            context = str(context or "").strip()
            if context:
                contexts.add(context)
    return sorted(contexts)


def branch_protection_rule_matches_branch(rule: dict[str, Any], branch: str) -> bool:
    branch = str(branch or "").strip()
    if not branch:
        return False
    matching_refs = (rule.get("matchingRefs") or {}).get("nodes") or []
    for ref in matching_refs:
        if isinstance(ref, dict) and str(ref.get("name") or "").strip() == branch:
            return True
    pattern = str(rule.get("pattern") or "").strip()
    if not pattern:
        return False
    return pattern == branch or fnmatch.fnmatchcase(branch, pattern)


def split_repo_name(repo: str) -> tuple[str, str]:
    parts = str(repo or "").split("/", 1)
    if len(parts) != 2:
        return "", ""
    return parts[0], parts[1]


def required_check_match(
    required_contexts: list[str],
    failing_names: list[str],
    pending_names: list[str],
    success_names: list[str],
    required_result: FetchResult,
) -> dict[str, Any]:
    if required_result.status_code != 200 or not required_result.complete:
        return {
            "coverage_state": "required_checks_unavailable",
            "match_state": "required_check_coverage_unavailable",
            "failing_required": [],
            "pending_required": [],
            "missing_required": [],
            "successful_required": [],
        }
    required = set(required_contexts)
    if not required:
        return {
            "coverage_state": "required_checks_observed",
            "match_state": "no_required_contexts_configured",
            "failing_required": [],
            "pending_required": [],
            "missing_required": [],
            "successful_required": [],
        }
    failing = required.intersection(failing_names)
    pending = required.intersection(pending_names)
    successful = required.intersection(success_names)
    missing = required.difference(failing | pending | successful)
    if failing or pending:
        match_state = "required_context_failing_or_pending"
    elif missing:
        match_state = "required_context_missing"
    else:
        match_state = "required_contexts_successful"
    return {
        "coverage_state": "required_checks_observed",
        "match_state": match_state,
        "failing_required": sorted(failing),
        "pending_required": sorted(pending),
        "missing_required": sorted(missing),
        "successful_required": sorted(successful),
    }


def combined_signal(check_result: FetchResult, status_result: FetchResult, check_counts: dict[str, Any], status_counts: dict[str, Any]) -> str:
    observed_endpoint_count = int(check_result.status_code == 200 and check_result.complete) + int(status_result.status_code == 200 and status_result.complete)
    partial_endpoint_count = int(check_result.status_code == 200 and not check_result.complete) + int(status_result.status_code == 200 and not status_result.complete)
    if observed_endpoint_count == 0:
        return "coverage_partial" if partial_endpoint_count else "coverage_failed"
    if int(check_counts["failing_count"]) + int(status_counts["failing_count"]) > 0:
        return "failing_checks"
    if int(check_counts["pending_count"]) + int(status_counts["pending_count"]) > 0:
        return "pending_checks"
    if partial_endpoint_count or check_result.status_code != 200 or status_result.status_code != 200:
        return "coverage_partial"
    total_contexts = int(check_counts["check_run_count"]) + int(status_counts["status_context_count"])
    if total_contexts == 0:
        return "no_checks_reported_by_api"
    return "passing_or_neutral_checks"


def build_ci_insight_cards(observations: pd.DataFrame) -> pd.DataFrame:
    if observations.empty:
        return pd.DataFrame(columns=CHECK_CARD_COLUMNS)
    rows: list[dict[str, Any]] = []
    for row in observations.itertuples(index=False):
        signal = str(row.combined_signal)
        if str(row.effective_state) != "open":
            continue
        if signal not in {"failing_checks", "pending_checks"}:
            continue
        if str(row.source_coverage_state) == "failed":
            continue
        if str(row.head_source) != "current_api":
            continue
        failing_count = int(row.failing_context_count or 0)
        pending_count = int(row.pending_context_count or 0)
        severity = "high" if failing_count else "medium"
        score = min(95.0, 80.0 + failing_count * 5.0) if failing_count else min(75.0, 50.0 + pending_count * 8.0)
        names = split_contexts(row.failing_contexts if failing_count else row.pending_contexts)
        evidence_excerpt = check_evidence_excerpt(signal, names, row.head_sha)
        subject_key = str(row.subject_key)
        rows.append(
            {
                "insight_kind": "status_summary",
                "severity": severity,
                "subject_kind": "pull_request",
                "subject_key": subject_key,
                "identity_key": "ci_check_state",
                "source_url": row.pr_url,
                "title": f"CI check state needs review: {subject_key}",
                "details": check_details(signal, subject_key, names, row.head_sha),
                "recommended_action": "Review the failing or pending GitHub checks, confirm whether they block merge, and record the owner or decision.",
                "model_method": "ci_check_state_live_observation",
                "score": score,
                "score_explanation": "Score is a triage priority from live GitHub check/status payloads; non-200 coverage failures do not emit this insight.",
                "confidence": 0.72 if failing_count else 0.62,
                "evidence_excerpt": evidence_excerpt,
                "evidence_source_system": "github",
                "evidence_source_instance": row.repository,
                "evidence_external_kind": row.evidence_external_kind,
                "evidence_external_id": f"{subject_key}:{row.head_sha}",
                "evidence_source_url": row.evidence_source_url,
                "evidence_locator_kind": row.evidence_external_kind,
                "evidence_locator": row.evidence_source_url,
                "evidence_source_span_key": stable_digest([subject_key, row.head_sha, signal, "|".join(names)]),
                "evidence_span_start": None,
                "evidence_span_end": None,
                "evidence_excerpt_truncated": False,
                "producer_state": "current",
                "stale_reason": "",
            }
        )
    return pd.DataFrame(rows, columns=CHECK_CARD_COLUMNS)


def merge_ci_cards_into_analytics(conn: sqlite3.Connection, check_cards: pd.DataFrame) -> None:
    if table_exists(conn, "tpm_insight_cards"):
        existing = pd.read_sql_query("select * from tpm_insight_cards", conn)
    else:
        existing = pd.DataFrame(columns=CHECK_CARD_COLUMNS)
    if not existing.empty:
        keep = existing.apply(
            lambda row: not (
                str(row.get("identity_key", "")) == "ci_check_state"
                or str(row.get("model_method", "")).startswith("ci_check_state")
            ),
            axis=1,
        )
        existing = existing[keep].copy()
    if existing.empty:
        merged = check_cards.copy()
    elif check_cards.empty:
        merged = existing.copy()
    else:
        merged = pd.concat([existing.astype(object), check_cards.astype(object)], ignore_index=True)
    merged.to_sql("tpm_insight_cards", conn, if_exists="replace", index=False)
    current = merged[merged.get("producer_state", pd.Series(dtype=str)).fillna("current") == "current"].copy() if not merged.empty else merged
    current.to_sql("tpm_current_insight_cards", conn, if_exists="replace", index=False)


def persist_ci_work_insights(
    conn: sqlite3.Connection,
    check_cards: pd.DataFrame,
    observed_at: datetime,
    source_instance: str,
) -> dict[str, int]:
    if not table_exists(conn, "work_insights"):
        return {"current_count": 0, "staled_count": 0}
    observed_iso = observed_at.isoformat()
    cursor = conn.execute(
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
              and model_name = 'flink_tpm_check_observe'
              and latest_evidence_id is not null
        )
        """,
        (observed_iso, source_instance),
    )
    evidence_staled = int(cursor.rowcount or 0)
    cursor = conn.execute(
        """
        update work_insights
        set producer_state = 'stale',
            freshness_state = 'stale',
            updated_at = ?
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_insight'
          and model_name = 'flink_tpm_check_observe'
        """,
        (observed_iso, source_instance),
    )
    insight_staled = int(cursor.rowcount or 0)
    if check_cards.empty:
        conn.commit()
        return {"current_count": 0, "staled_count": max(evidence_staled, insight_staled)}

    pr_ids = {
        f"{row[0]}#{int(row[1])}": int(row[2])
        for row in conn.execute("select repository, number, id from pull_requests where repository is not null and number is not null")
    }
    current_count = 0
    for row in check_cards.itertuples(index=False):
        card = row._asdict()
        subject_key = str(card["subject_key"])
        pull_request_id = pr_ids.get(subject_key)
        if pull_request_id is None:
            continue
        digest = stable_digest([card["insight_kind"], "pull_request", subject_key, card.get("identity_key") or "ci_check_state"])
        insight_key = f"work-insight:cubicle-check-observe:{source_instance}:{digest}"
        evidence_key = f"evidence:work-insight:cubicle-check-observe:{source_instance}:{digest}"
        external_id = f"tpm-insight:ci-check:{digest}"
        score = clean_float(card.get("score"), 0.0)
        confidence = clean_float(card.get("confidence"), 0.5)
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
              ?, ?, ?, 'current', 'pull_request', ?,
              ?, null, ?, ?, ?,
              'flink_tpm_check_observe', ?, ?, ?, ?,
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
              title = excluded.title,
              details = excluded.details,
              recommended_action = excluded.recommended_action,
              model_version = excluded.model_version,
              model_method = excluded.model_method,
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
                subject_key,
                pull_request_id,
                card["title"],
                card.get("details") or "",
                card.get("recommended_action") or "",
                CHECK_OBSERVER_VERSION,
                card.get("model_method") or "",
                score,
                card.get("score_explanation") or "",
                source_instance,
                external_id,
                card.get("source_url") or "",
                confidence,
                observed_iso,
                observed_iso,
                score,
                observed_iso,
                observed_iso,
            ),
        )
        insight_id = int(conn.execute("select id from work_insights where key = ?", (insight_key,)).fetchone()[0])
        excerpt = str(card.get("evidence_excerpt") or "")
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
              ?, 'candidate', 'work_insight', ?, 'source_api_observation',
              ?, ?, ?, null, null,
              ?, false,
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
              excerpt = excluded.excerpt,
              excerpt_truncated = false,
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
                card.get("evidence_locator_kind") or "",
                card.get("evidence_locator") or "",
                card.get("evidence_source_span_key") or digest,
                excerpt,
                stable_digest([excerpt]),
                observed_iso,
                card.get("evidence_source_system") or "github",
                card.get("evidence_source_instance") or "",
                card.get("evidence_external_kind") or "",
                card.get("evidence_external_id") or "",
                card.get("evidence_source_url") or "",
                confidence,
                observed_iso,
                observed_iso,
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
            (evidence_id, observed_iso, insight_id),
        )
        current_count += 1
    conn.commit()
    return {"current_count": current_count, "staled_count": max(evidence_staled, insight_staled)}


def persist_ci_review_requests(conn: sqlite3.Connection, source_instance: str, observed_at: datetime) -> None:
    if not table_exists(conn, "work_insight_reviews"):
        return
    observed_iso = observed_at.isoformat()
    rows = conn.execute(
        """
        select id, key, subject_key, source_url
        from work_insights
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_insight'
          and model_name = 'flink_tpm_check_observe'
          and producer_state = 'current'
        """,
        (source_instance,),
    ).fetchall()
    for insight_id, insight_key, subject_key, source_url in rows:
        digest = stable_digest([insight_key, "triage_request"])
        review_key = f"work-insight-review:cubicle-check-observe:{source_instance}:{digest}"
        external_id = f"tpm-insight-review:ci-check:{digest}"
        next_action = f"Review GitHub check/status evidence for {subject_key}; record whether it blocks merge and who owns the fix or decision."
        rationale = "Generated CI check-state follow-up from HTTP 200 GitHub check/status payloads; source coverage failures are tracked separately."
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, reviewer_kind, reviewer_key, next_action,
              rationale, source_system, source_instance, external_kind,
              external_id, source_url, created_at, updated_at
            ) values (
              ?, ?, 'triage_request', 'requested', 'unknown',
              'unknown', 'system', 'flink_tpm_check_observe', ?,
              ?, 'cubicle_analytics', ?, 'tpm_insight_review',
              ?, ?, ?, ?
            )
            on conflict(key) do update set
              next_action = excluded.next_action,
              rationale = excluded.rationale,
              source_url = excluded.source_url,
              updated_at = excluded.updated_at
            """,
            (
                review_key,
                insight_id,
                next_action,
                rationale,
                source_instance,
                external_id,
                source_url or "",
                observed_iso,
                observed_iso,
            ),
        )
    conn.commit()


def persist_check_sync_run(
    conn: sqlite3.Connection,
    observations: pd.DataFrame,
    observed_at: datetime,
    source_instance: str,
) -> dict[str, int]:
    if observations.empty:
        return {"run_created_count": 0, "issue_created_count": 0}
    observed_iso = observed_at.isoformat()
    scope_id = ensure_check_scope(conn, source_instance, observed_iso)
    failed_rows = build_failed_fetch_rows(observations)
    issue_count = len(failed_rows)
    any_rate_limited = any(row["issue_code"] == "source_rate_limited" for row in failed_rows)
    status = "complete" if issue_count == 0 else "rate_limited" if any_rate_limited else "partial"
    coverage_mode = "live_only" if issue_count == 0 else "partial_scope"
    error_code = None if issue_count == 0 else "source_rate_limited" if any_rate_limited else "source_check_observation_partial"
    error_message = None if issue_count == 0 else f"{issue_count} check/status source reads failed; failures are coverage evidence, not product absence."
    run_key = check_run_key(source_instance, observed_iso)
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
    for failed in failed_rows:
        conn.execute(
            """
            insert into source_sync_issues (
              severity, issue_code, message, source_system, source_instance,
              external_kind, external_id, source_url, created_at, updated_at,
              source_scope_id, source_sync_run_id
            ) values (
              'warning', ?, ?, 'github', ?,
              ?, ?, ?, ?, ?,
              ?, ?
            )
            """,
            (
                failed["issue_code"],
                failed["message"],
                failed["repository"],
                failed["external_kind"],
                failed["external_id"],
                failed["source_url"],
                observed_iso,
                observed_iso,
                scope_id,
                run_id,
            ),
        )
    refresh_current_source_sync_issue_view(conn)
    conn.commit()
    return {"run_created_count": 1, "issue_created_count": issue_count}


def build_failed_fetch_rows(observations: pd.DataFrame) -> list[dict[str, str]]:
    failed: list[dict[str, str]] = []
    for row in observations.itertuples(index=False):
        endpoints = [
            ("github_pull_request", row.pr_fetch_url, row.pr_fetch_status_code, row.pr_fetch_error),
            ("github_check_runs", row.check_runs_url, row.check_fetch_status_code, row.check_fetch_error),
            ("github_commit_statuses", row.statuses_url, row.status_fetch_status_code, row.status_fetch_error),
        ]
        complete_by_kind = {
            "github_pull_request": bool(row.pr_fetch_complete),
            "github_check_runs": bool(row.check_fetch_complete),
            "github_commit_statuses": bool(row.status_fetch_complete),
        }
        for external_kind, url, status_code, error in endpoints:
            if status_code == 200 and complete_by_kind.get(external_kind, True):
                continue
            if not url and not error:
                continue
            if status_code == 200 and not error:
                error = "pagination_partial:incomplete_fetch"
            failed.append(
                {
                    "repository": str(row.repository),
                    "external_kind": external_kind,
                    "external_id": f"{row.subject_key}:{row.head_sha}",
                    "source_url": str(url or row.pr_url or ""),
                    "issue_code": sync_issue_code(status_code, error),
                    "message": sync_issue_message(external_kind, status_code, error),
                }
            )
    return failed


def ensure_check_scope(conn: sqlite3.Connection, source_instance: str, now: str) -> int:
    connection_key = f"source-connection:cubicle-check-observe:{source_instance}"
    conn.execute(
        """
        insert into source_connections (
          key, source_system, source_instance, display_name, connector_kind,
          is_enabled, last_synced_at, created_at, updated_at
        ) values (
          ?, 'cubicle_check_observer', ?, ?, 'github_check_observer',
          true, ?, ?, ?
        )
        on conflict(key) do update set
          last_synced_at = excluded.last_synced_at,
          updated_at = excluded.updated_at
        """,
        (connection_key, source_instance, f"AI TPM check observer for {source_instance}", now, now, now),
    )
    connection_id = int(conn.execute("select id from source_connections where key = ?", (connection_key,)).fetchone()[0])
    scope_key = f"source-scope:cubicle-check-observe:{source_instance}"
    conn.execute(
        """
        insert into source_scopes (
          key, scope_kind, scope_key, display_name, crawl_policy,
          is_enabled, created_at, updated_at, source_connection_id
        ) values (
          ?, 'pull_request_checks', ?, ?, 'github_check_observer',
          true, ?, ?, ?
        )
        on conflict(key) do update set
          updated_at = excluded.updated_at,
          source_connection_id = excluded.source_connection_id
        """,
        (scope_key, source_instance, f"AI TPM check observer for {source_instance}", now, now, connection_id),
    )
    return int(conn.execute("select id from source_scopes where key = ?", (scope_key,)).fetchone()[0])


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


def build_summary(observations: pd.DataFrame, fixture_prs: pd.DataFrame) -> pd.DataFrame:
    rows = [
        {"metric": "selected_pr_count", "value": str(len(fixture_prs)), "note": "typed PR product rows selected for check observation; manifest payloads are supplemental replay metadata"},
        {"metric": "check_observation_count", "value": str(len(observations)), "note": "PRs attempted by the check observer"},
    ]
    if observations.empty:
        return pd.DataFrame(rows)
    open_observations = observations[observations["effective_state"] == "open"]
    rows.extend(
        [
            {"metric": "open_pr_observation_count", "value": str(len(open_observations)), "note": "observed PRs whose latest known state is open"},
            {"metric": "current_pr_fetch_success_count", "value": str(int(((observations["pr_fetch_status_code"] == 200) & observations["pr_fetch_complete"]).sum())), "note": "PR payload refreshes with HTTP 200 and complete pagination"},
            {"metric": "check_runs_fetch_success_count", "value": str(int(((observations["check_fetch_status_code"] == 200) & observations["check_fetch_complete"]).sum())), "note": "check-runs endpoint fetches with HTTP 200 and complete pagination"},
            {"metric": "statuses_fetch_success_count", "value": str(int(((observations["status_fetch_status_code"] == 200) & observations["status_fetch_complete"]).sum())), "note": "commit statuses endpoint fetches with HTTP 200 and complete pagination"},
            {"metric": "check_runs_max_page_count", "value": str(int(pd.to_numeric(observations["check_fetch_page_count"], errors="coerce").fillna(0).max())), "note": "maximum paginated check-runs pages read for one PR"},
            {"metric": "statuses_max_page_count", "value": str(int(pd.to_numeric(observations["status_fetch_page_count"], errors="coerce").fillna(0).max())), "note": "maximum paginated commit-status pages read for one PR"},
            {"metric": "source_coverage_failed_count", "value": str(int((observations["source_coverage_state"] == "failed").sum())), "note": "PRs with no successful check/status observation"},
            {"metric": "source_coverage_partial_count", "value": str(int((observations["source_coverage_state"] == "partial").sum())), "note": "PRs with some but not all check/status endpoints observed"},
            {"metric": "failing_check_pr_count", "value": str(int((observations["combined_signal"] == "failing_checks").sum())), "note": "PRs with at least one failing check/status context from HTTP 200 payloads"},
            {"metric": "open_failing_check_pr_count", "value": str(int((open_observations["combined_signal"] == "failing_checks").sum())), "note": "open PRs with at least one failing check/status context from HTTP 200 payloads"},
            {"metric": "pending_check_pr_count", "value": str(int((observations["combined_signal"] == "pending_checks").sum())), "note": "PRs with pending checks and no failing check/status context"},
            {"metric": "open_pending_check_pr_count", "value": str(int((open_observations["combined_signal"] == "pending_checks").sum())), "note": "open PRs with pending checks and no failing check/status context"},
            {"metric": "passing_or_neutral_pr_count", "value": str(int((observations["combined_signal"] == "passing_or_neutral_checks").sum())), "note": "PRs with observed checks/statuses and no failing or pending contexts"},
            {"metric": "no_checks_reported_pr_count", "value": str(int((observations["combined_signal"] == "no_checks_reported_by_api").sum())), "note": "HTTP 200 check/status APIs reported no contexts"},
        ]
    )
    for signal, count in observations.groupby("combined_signal").size().items():
        rows.append({"metric": f"signal_{signal}", "value": str(int(count)), "note": "combined check/status signal"})
    return pd.DataFrame(rows)


def build_check_signal_readiness(observed_at: datetime, observations: pd.DataFrame, summary: pd.DataFrame) -> pd.DataFrame:
    selected_count = summary_metric_int(summary, "selected_pr_count")
    observation_count = summary_metric_int(summary, "check_observation_count")
    open_count = summary_metric_int(summary, "open_pr_observation_count")
    failed_count = summary_metric_int(summary, "source_coverage_failed_count")
    partial_count = summary_metric_int(summary, "source_coverage_partial_count")
    failing_count = summary_metric_int(summary, "failing_check_pr_count")
    open_failing_count = summary_metric_int(summary, "open_failing_check_pr_count")
    pending_count = summary_metric_int(summary, "pending_check_pr_count")
    open_pending_count = summary_metric_int(summary, "open_pending_check_pr_count")
    no_checks_count = summary_metric_int(summary, "no_checks_reported_pr_count")
    complete_source_count = max(0, observation_count - failed_count - partial_count)

    required_unavailable_count = 0
    no_required_context_count = 0
    required_signal_count = 0
    observed_at_count = 0
    current_head_count = 0
    if not observations.empty:
        required_unavailable_count = int((observations["required_check_coverage_state"] != "required_checks_observed").sum())
        no_required_context_count = int((observations["required_check_match_state"] == "no_required_contexts_configured").sum())
        required_signal_count = int(
            observations["required_check_match_state"].isin(
                ["required_context_failing_or_pending", "required_context_missing"]
            ).sum()
        )
        observed_at_count = int(observations["observed_at"].astype(str).nunique())
        current_head_count = int((observations["head_source"] == "current_api").sum()) if "head_source" in observations.columns else 0

    coverage_state = "not_observed"
    if observation_count > 0 and failed_count == 0 and partial_count == 0:
        coverage_state = "complete"
    elif complete_source_count > 0:
        coverage_state = "partial"
    elif observation_count > 0:
        coverage_state = "failed"

    ci_validation_ready = coverage_state == "complete" and (open_failing_count + open_pending_count) > 0
    if ci_validation_ready:
        validation_state = "ready_with_current_open_signal"
        validation_reason = "HTTP 200 PR/check/status observations contain open failing or pending CI context evidence."
        validation_action = "Use as owner validation leads; keep source coverage and required-check context fields attached."
    elif coverage_state == "complete":
        validation_state = "ready_no_current_open_signal"
        validation_reason = "HTTP 200 PR/check/status observations are complete, but no open failing or pending CI signal is present."
        validation_action = "Keep observing check/status state and preserve no-checks-reported rows as current observations only."
    elif coverage_state == "not_observed":
        validation_state = "not_observed"
        validation_reason = "No check/status observations were captured."
        validation_action = "Run the check observer before generating CI follow-up actions."
    else:
        validation_state = "coverage_limited"
        validation_reason = "Some PR/check/status observations are failed or partial; they cannot support source absence claims."
        validation_action = "Repair source coverage before using CI state as an owner follow-up signal."

    product_ready = False
    if coverage_state == "complete" and required_unavailable_count == 0 and required_signal_count > 0:
        product_state = "required_check_evidence_ready_measurement_required"
        product_reason = "Branch-protection required status-check evidence has a failing, pending, or missing required context; product action still requires measurement gates."
        product_action = "Use as required-check evidence for review; do not mark product action ready until measurement gates pass."
    elif coverage_state != "complete":
        product_state = "coverage_limited"
        product_reason = "Required-check-backed action needs complete PR/check/status coverage first."
        product_action = "Repair source coverage, then re-evaluate required check context state."
    elif required_unavailable_count > 0:
        product_state = "required_check_coverage_limited"
        product_reason = "At least one PR lacks observed branch-protection required status-check configuration."
        product_action = "Fetch branch protection evidence before using required-check state as product-action evidence."
    elif no_required_context_count == observation_count and observation_count > 0:
        product_state = "no_required_contexts_configured"
        product_reason = "Branch-protection evidence reports no required status-check contexts for the observed PR base branches; this does not prove rulesets or required workflows are absent."
        product_action = "Treat failing non-required contexts as validation leads, not product-action evidence."
    else:
        product_state = "no_required_check_signal"
        product_reason = "Branch-protection required status-check configuration is observed, but no required context is failing, pending, or missing."
        product_action = "Keep required-check state as context and do not create product-action claims from passing checks."

    eta_ready = False
    if observed_at_count >= 2 and current_head_count < observation_count:
        eta_state = "candidate_needs_outcome_backtest"
        eta_reason = "Multiple observations exist, but the check features still need point-in-time outcome backtesting before ETA promotion."
        eta_action = "Join repeated check snapshots to later outcomes and require same-model lift over baselines."
    elif observed_at_count >= 2:
        eta_state = "live_snapshot_series_only"
        eta_reason = "Multiple live observations exist, but they are current-head observations and not yet proven as-of-safe."
        eta_action = "Persist pre-terminal check snapshots with observation time, head SHA, and later outcome labels."
    else:
        eta_state = "single_live_observation"
        eta_reason = "Only one live check/status observation time is available; this is action evidence, not an ETA feature series."
        eta_action = "Collect repeated as-of check/status snapshots before using CI state in ETA models."

    common = {
        "generated_at": observed_at.isoformat(),
        "workstream_key": "flink-kubernetes-operator",
        "selected_pr_count": selected_count,
        "check_observation_count": observation_count,
        "open_pr_observation_count": open_count,
        "complete_source_observation_count": complete_source_count,
        "source_coverage_state": coverage_state,
        "source_coverage_failed_count": failed_count,
        "source_coverage_partial_count": partial_count,
        "failing_check_pr_count": failing_count,
        "open_failing_check_pr_count": open_failing_count,
        "pending_check_pr_count": pending_count,
        "open_pending_check_pr_count": open_pending_count,
        "no_checks_reported_pr_count": no_checks_count,
        "required_check_unavailable_count": required_unavailable_count,
        "no_required_contexts_configured_count": no_required_context_count,
        "required_check_signal_count": required_signal_count,
        "distinct_observed_at_count": observed_at_count,
        "current_head_observation_count": current_head_count,
    }
    return pd.DataFrame(
        [
            {
                **common,
                "readiness_key": "ci_followup_validation",
                "support_level": "validation_lead",
                "ready": bool(ci_validation_ready),
                "readiness_state": validation_state,
                "blocking_reason": validation_reason,
                "recommended_action": validation_action,
            },
            {
                **common,
                "readiness_key": "required_check_product_action",
                "support_level": "product_action_evidence",
                "ready": bool(product_ready),
                "readiness_state": product_state,
                "blocking_reason": product_reason,
                "recommended_action": product_action,
            },
            {
                **common,
                "readiness_key": "ci_eta_feature",
                "support_level": "eta_feature",
                "ready": bool(eta_ready),
                "readiness_state": eta_state,
                "blocking_reason": eta_reason,
                "recommended_action": eta_action,
            },
        ]
    )


def summary_metric_int(summary: pd.DataFrame, metric: str) -> int:
    if summary.empty or "metric" not in summary.columns or "value" not in summary.columns:
        return 0
    matches = summary[summary["metric"] == metric]
    if matches.empty:
        return 0
    try:
        return int(float(matches.iloc[0]["value"]))
    except (TypeError, ValueError):
        return 0


def write_report(path: Path, observed_at: datetime, observations: pd.DataFrame, summary: pd.DataFrame, check_readiness: pd.DataFrame, check_cards: pd.DataFrame) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    columns = [
        "subject_key",
        "effective_state",
        "head_source",
        "source_coverage_state",
        "combined_signal",
        "failing_context_count",
        "pending_context_count",
        "failing_contexts",
        "pending_contexts",
        "pr_url",
    ]
    lines = [
        "# Flink AI TPM Check Observations",
        "",
        f"Observed at: {observed_at.isoformat()}",
        "",
        "## Summary",
        "",
        df_to_markdown(summary),
        "",
        "## Check Signal Readiness",
        "",
        df_to_markdown(
            check_readiness[
                [
                    "readiness_key",
                    "support_level",
                    "ready",
                    "readiness_state",
                    "source_coverage_state",
                    "open_failing_check_pr_count",
                    "open_pending_check_pr_count",
                    "required_check_signal_count",
                    "distinct_observed_at_count",
                    "blocking_reason",
                    "recommended_action",
                ]
            ]
        )
        if not check_readiness.empty
        else "",
        "",
        "## Failing or Pending Open PRs",
        "",
        df_to_markdown(observations[(observations["effective_state"] == "open") & (observations["combined_signal"].isin(["failing_checks", "pending_checks"]))].head(50)[columns])
        if not observations.empty
        else "",
        "",
        "## CI WorkInsight Cards",
        "",
        df_to_markdown(check_cards[["severity", "subject_key", "score", "title", "evidence_excerpt", "evidence_source_url"]].head(50))
        if not check_cards.empty
        else "",
        "",
        "## Coverage Failures",
        "",
        df_to_markdown(
            observations[
                (observations["pr_fetch_status_code"] != 200)
                | (observations["check_fetch_status_code"] != 200)
                | (observations["status_fetch_status_code"] != 200)
                | (~observations["pr_fetch_complete"])
                | (~observations["check_fetch_complete"])
                | (~observations["status_fetch_complete"])
            ][
                [
                    "subject_key",
                    "pr_fetch_status_code",
                    "pr_fetch_complete",
                    "check_fetch_status_code",
                    "check_fetch_complete",
                    "check_fetch_page_count",
                    "status_fetch_status_code",
                    "status_fetch_complete",
                    "status_fetch_page_count",
                    "pr_fetch_error",
                    "check_fetch_error",
                    "status_fetch_error",
                ]
            ].head(50)
        )
        if not observations.empty
        else "",
        "",
        "## Interpretation",
        "",
        "- These rows are live supplemental observations for the bounded fixture PRs.",
        "- Non-200 or incomplete paginated GitHub check/status responses are coverage evidence only and do not imply checks are absent.",
        "- CI WorkInsight rows are emitted only for open PRs whose current PR payload was observed and whose check/status evidence came from HTTP 200 payloads.",
        "- `no_checks_reported_by_api` only means the complete observed HTTP 200 GitHub APIs returned zero contexts for that head SHA.",
        "- Required-check coverage currently reflects branch-protection required status checks observed through REST/GraphQL fallback; it does not prove GitHub rulesets or required workflows are absent.",
    ]
    path.write_text("\n".join(lines) + "\n")


def write_raw_fetch(raw_dir: Path, repo: str, pr_number: int, name: str, result: FetchResult) -> None:
    safe_repo = repo.replace("/", "__")
    target_dir = raw_dir / f"{safe_repo}__{pr_number}"
    target_dir.mkdir(parents=True, exist_ok=True)
    envelope = {
        "url": result.url,
        "status_code": result.status_code,
        "auth_state": result.auth_state,
        "coverage_kind": result.coverage_kind,
        "complete": result.complete,
        "page_count": result.page_count,
        "error": redact(result.error),
        "payload": result.payload,
    }
    (target_dir / f"{name}.json").write_text(json.dumps(envelope, sort_keys=True, indent=2) + "\n")


def coverage_state_for(pr_result: FetchResult, check_result: FetchResult, status_result: FetchResult) -> str:
    if pr_result.status_code != 200 or not pr_result.complete:
        return "failed"
    observed = int(check_result.status_code == 200 and check_result.complete) + int(status_result.status_code == 200 and status_result.complete)
    partial = int(check_result.status_code == 200 and not check_result.complete) + int(status_result.status_code == 200 and not status_result.complete)
    if observed == 2:
        return "complete"
    if observed or partial:
        return "partial"
    return "failed"


def combined_auth_state(results: list[FetchResult]) -> str:
    states = sorted({result.auth_state for result in results if result.auth_state and result.auth_state != "none"})
    return ",".join(states) if states else "none"


def combined_coverage_kind(results: list[FetchResult]) -> str:
    states = sorted({result.coverage_kind for result in results if result.coverage_kind and result.coverage_kind != "not_observed"})
    return ",".join(states) if states else "not_observed"


def evidence_source_url_for(signal: str, check_result: FetchResult, status_result: FetchResult) -> str:
    if signal in {"failing_checks", "pending_checks"} and check_result.status_code == 200:
        return check_result.url
    if status_result.status_code == 200:
        return status_result.url
    return check_result.url or status_result.url


def evidence_external_kind_for(signal: str, check_counts: dict[str, Any], status_counts: dict[str, Any]) -> str:
    if signal in {"failing_checks", "pending_checks"}:
        if int(check_counts["failing_count"]) + int(check_counts["pending_count"]) > 0:
            return "github_check_runs"
        if int(status_counts["failing_count"]) + int(status_counts["pending_count"]) > 0:
            return "github_commit_statuses"
    return "github_check_state"


def check_details(signal: str, subject_key: str, names: list[str], head_sha: str) -> str:
    label = "failing" if signal == "failing_checks" else "pending"
    visible = ", ".join(names[:8]) if names else "unnamed contexts"
    suffix = "..." if len(names) > 8 else ""
    short_sha = head_sha[:12] if head_sha else "unknown"
    return f"GitHub check/status APIs reported {label} contexts for {subject_key} head {short_sha}: {visible}{suffix}. This is a live check-state lead, not a mergeability claim."


def check_evidence_excerpt(signal: str, names: list[str], head_sha: str) -> str:
    label = "Failing" if signal == "failing_checks" else "Pending"
    visible = ", ".join(names[:10]) if names else "unnamed contexts"
    suffix = "..." if len(names) > 10 else ""
    short_sha = head_sha[:12] if head_sha else "unknown"
    return f"{label} GitHub check/status contexts on head {short_sha}: {visible}{suffix}"


def split_contexts(value: Any) -> list[str]:
    return [part.strip() for part in str(value or "").split(",") if part.strip()]


def parse_source_object_id(value: str) -> tuple[str, str]:
    if "#" not in value:
        return "", ""
    repo, number = value.rsplit("#", 1)
    return repo, number


def repository_from_pr_payload(payload: dict[str, Any]) -> str:
    base_repo = ((payload.get("base") or {}).get("repo") or {}).get("full_name")
    return str(base_repo or "")


def normalize_pr_state(payload: dict[str, Any]) -> str:
    if not payload:
        return ""
    if payload.get("merged_at"):
        return "merged"
    return str(payload.get("state") or "unknown")


def github_pr_url(subject_key: str) -> str:
    repo, number = parse_source_object_id(subject_key)
    return f"https://github.com/{repo}/pull/{number}" if repo and number else ""


def github_api_pr_url(repo: str, pr_number: int) -> str:
    return f"https://api.github.com/repos/{repo}/pulls/{pr_number}" if repo and pr_number else ""


def github_check_runs_url(repo: str, head_sha: str) -> str:
    return f"https://api.github.com/repos/{repo}/commits/{head_sha}/check-runs?per_page=100" if repo and head_sha else ""


def github_required_status_checks_url(repo: str, branch: str) -> str:
    if not repo or not branch:
        return ""
    return f"https://api.github.com/repos/{repo}/branches/{urllib.parse.quote(branch, safe='')}/protection/required_status_checks"


def append_query_param(url: str, key: str, value: str) -> str:
    if re.search(rf"([?&]){re.escape(key)}=", url):
        return url
    separator = "&" if "?" in url else "?"
    return f"{url}{separator}{key}={value}"


def next_link(headers: dict[str, str]) -> str:
    link = ""
    for key, value in headers.items():
        if key.lower() == "link":
            link = value
            break
    if not link:
        return ""
    for part in link.split(","):
        section = part.strip()
        if 'rel="next"' not in section:
            continue
        match = re.search(r"<([^>]+)>", section)
        if match:
            return match.group(1)
    return ""


def extract_page_items(payload: Any, collection_key: str) -> list[Any]:
    if collection_key and isinstance(payload, dict):
        items = payload.get(collection_key)
        return list(items) if isinstance(items, list) else []
    if isinstance(payload, list):
        return list(payload)
    return []


def aggregate_paginated_payload(first_payload: Any, items: list[Any], collection_key: str) -> Any:
    if collection_key:
        base = dict(first_payload) if isinstance(first_payload, dict) else {}
        base[collection_key] = items
        return base
    return items


def check_run_key(source_instance: str, observed_iso: str) -> str:
    digest = stable_digest([source_instance, observed_iso])
    return f"source-sync-run:{source_instance}:tpm-check-observe:{digest}"


def sync_issue_code(status_code: Any, fetch_error: Any) -> str:
    text = str(fetch_error or "").lower()
    try:
        status = int(status_code)
    except (TypeError, ValueError):
        status = 0
    if "pagination_partial" in text:
        return "source_partial_page"
    if status == 429 or "rate limit" in text:
        return "source_rate_limited"
    if status == 403:
        return "source_forbidden"
    if status:
        return "source_non_200"
    return "source_unavailable"


def sync_issue_message(external_kind: str, status_code: Any, fetch_error: Any) -> str:
    try:
        status = int(status_code)
    except (TypeError, ValueError):
        status = 0
    if "pagination_partial" in str(fetch_error or "").lower():
        return f"{external_kind} source pagination was incomplete; retained as coverage failure, not product absence"
    if status:
        return f"{external_kind} source request returned status {status}; retained as coverage failure, not product absence"
    return f"{external_kind} source request failed before HTTP status; retained as coverage failure: {str(fetch_error or '')[:240]}"


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


def append_summary_row(summary: pd.DataFrame, metric: str, value: str, note: str) -> pd.DataFrame:
    row = pd.DataFrame([{"metric": metric, "value": value, "note": note}])
    if summary.empty:
        return row
    return pd.concat([summary, row], ignore_index=True)


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


def clean_float(value: Any, default: float) -> float:
    try:
        if value is None or (isinstance(value, float) and math.isnan(value)):
            return default
        return float(value)
    except (TypeError, ValueError):
        return default


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


def stable_digest(parts: list[Any]) -> str:
    digest = hashlib.sha256("\n".join(str(part) for part in parts).encode("utf-8")).hexdigest()
    return digest[:24]


def table_exists(conn: sqlite3.Connection, table_name: str) -> bool:
    row = conn.execute("select 1 from sqlite_master where type = 'table' and name = ?", (table_name,)).fetchone()
    return row is not None


def redact(text: Any) -> str:
    return SECRET_TOKEN_RE.sub("[REDACTED_TOKEN]", str(text or ""))


if __name__ == "__main__":
    main()
