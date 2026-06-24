#!/usr/bin/env python3
"""Draft source-oracle context labels for source-verifiable AI-TPM insights."""

from __future__ import annotations

import argparse
import csv
import sqlite3
from pathlib import Path
from typing import Any


OUTPUT_COLUMNS = [
    "insight_key",
    "insight_kind",
    "subject_kind",
    "subject_key",
    "truth_label",
    "actionability_label",
    "review_state",
    "owner_key",
    "next_action",
    "rationale",
    "oracle_kind",
    "evidence_summary",
    "measurement_eligible",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ontology-db", required=True, type=Path)
    parser.add_argument("--analytics-db", required=True, type=Path)
    parser.add_argument("--source-instance", required=True)
    parser.add_argument("--output", required=True, type=Path)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    with sqlite3.connect(args.ontology_db) as ontology_conn, sqlite3.connect(args.analytics_db) as analytics_conn:
        insights = read_current_insights(ontology_conn, args.source_instance)
        forecast_summary = metric_map(read_table(analytics_conn, "tpm_forecast_summary"))
        pr_forecasts = by_subject_key(read_pr_forecasts(analytics_conn))
        check_observations = by_subject_key(read_table(analytics_conn, "tpm_pr_check_observations"))
        action_items = latest_action_by_subject(read_table(analytics_conn, "tpm_action_items"))
        dependency_edges = dependency_edges_by_ticket(read_table(analytics_conn, "tpm_dependency_edges"))
        developer_correlation = by_person_key(read_table(analytics_conn, "tpm_developer_correlation"))
        labels = build_source_oracle_labels(
            insights,
            forecast_summary,
            pr_forecasts,
            check_observations,
            action_items,
            dependency_edges,
            developer_correlation,
        )
    write_tsv(labels, args.output)


def read_current_insights(conn: sqlite3.Connection, source_instance: str) -> list[dict[str, Any]]:
    conn.row_factory = sqlite3.Row
    return [
        dict(row)
        for row in conn.execute(
            """
            select
              wi.key as insight_key,
              wi.insight_kind,
              wi.subject_kind,
              wi.subject_key,
              wi.severity,
              wi.title,
              wi.details,
              wi.recommended_action,
              wi.source_url,
              coalesce(e.locator_kind, '') as evidence_locator_kind,
              coalesce(e.source_url, '') as evidence_source_url,
              coalesce(e.excerpt, '') as evidence_excerpt
            from work_insights wi
            left join evidences e on e.id = wi.latest_evidence_id
            where wi.source_system = 'cubicle_analytics'
              and wi.source_instance = ?
              and wi.external_kind = 'tpm_insight'
              and wi.producer_state = 'current'
            order by wi.insight_kind, wi.subject_key
            """,
            (source_instance,),
        ).fetchall()
    ]


def read_pr_forecasts(conn: sqlite3.Connection) -> list[dict[str, Any]]:
    if not table_exists(conn, "tpm_pr_forecasts"):
        return []
    conn.row_factory = sqlite3.Row
    return [
        dict(row)
        for row in conn.execute(
            """
            select
              repository || '#' || pr_number as subject_key,
              *
            from tpm_pr_forecasts
            """
        ).fetchall()
    ]


def read_table(conn: sqlite3.Connection, table_name: str) -> list[dict[str, Any]]:
    if not table_exists(conn, table_name):
        return []
    conn.row_factory = sqlite3.Row
    return [dict(row) for row in conn.execute(f"select * from {table_name}").fetchall()]


def table_exists(conn: sqlite3.Connection, table_name: str) -> bool:
    return conn.execute("select 1 from sqlite_master where type = 'table' and name = ?", (table_name,)).fetchone() is not None


def build_source_oracle_labels(
    insights: list[dict[str, Any]],
    forecast_summary: dict[str, str],
    pr_forecasts: dict[str, dict[str, Any]],
    check_observations: dict[str, dict[str, Any]],
    action_items: dict[tuple[str, str], dict[str, Any]],
    dependency_edges: dict[str, dict[str, int]] | None = None,
    developer_correlation: dict[str, dict[str, Any]] | None = None,
) -> list[dict[str, str]]:
    labels: list[dict[str, str]] = []
    dependency_edges = dependency_edges or {}
    developer_correlation = developer_correlation or {}
    for insight in insights:
        label = label_insight(
            insight,
            forecast_summary,
            pr_forecasts,
            check_observations,
            action_items,
            dependency_edges,
            developer_correlation,
        )
        if label:
            labels.append(label)
    return labels


def label_insight(
    insight: dict[str, Any],
    forecast_summary: dict[str, str],
    pr_forecasts: dict[str, dict[str, Any]],
    check_observations: dict[str, dict[str, Any]],
    action_items: dict[tuple[str, str], dict[str, Any]],
    dependency_edges: dict[str, dict[str, int]],
    developer_correlation: dict[str, dict[str, Any]],
) -> dict[str, str] | None:
    insight_kind = clean(insight.get("insight_kind"))
    subject_key = clean(insight.get("subject_key"))
    if insight_kind == "forecast_risk":
        return forecast_risk_label(insight, forecast_summary, pr_forecasts.get(subject_key))
    if insight_kind == "blocker_candidate":
        action = action_items.get((clean(insight.get("subject_kind")), subject_key), {})
        return blocker_candidate_source_label(insight, pr_forecasts.get(subject_key), action)
    if insight_kind == "model_quality":
        return model_quality_label(insight, forecast_summary)
    if insight_kind == "status_summary":
        action = action_items.get((clean(insight.get("subject_kind")), subject_key), {})
        if clean(action.get("action_type")) == "ci_check_followup":
            return ci_status_label(insight, check_observations.get(subject_key), action)
        if clean(action.get("action_type")) == "review_wait_followup":
            return reviewer_wait_status_label(insight, pr_forecasts.get(subject_key), action)
    if insight_kind == "dependency_cluster":
        return dependency_cluster_source_label(insight, dependency_edges.get(subject_key, {}))
    if insight_kind == "developer_correlation":
        return developer_correlation_source_label(insight, developer_correlation.get(subject_key))
    return None


def forecast_risk_label(
    insight: dict[str, Any],
    forecast_summary: dict[str, str],
    forecast: dict[str, Any] | None,
) -> dict[str, str] | None:
    if not forecast:
        return None
    if clean(forecast.get("state")) != "open":
        return None
    if clean(forecast.get("source_current_detail_state")) == "failed":
        return None
    if clean(forecast.get("source_current_coverage_state")) not in {"observed", "complete"}:
        return None
    age_days = safe_float(forecast.get("age_days"))
    threshold_days = safe_float(forecast.get("predicted_total_cycle_days"))
    overdue_days = safe_float(forecast.get("overdue_days"))
    risk_score = safe_float(forecast.get("risk_score"))
    if age_days <= 0 or threshold_days <= 0 or overdue_days <= 0 or risk_score < 60:
        return None
    eta_ready = forecast_summary.get("eta_forecast_ready", "false").lower() == "true"
    subject_key = clean(insight.get("subject_key"))
    return base_label(
        insight,
        truth_label="true_positive",
        actionability_label="needs_owner",
        review_state="accepted",
        next_action=(
            f"Ask the owner for merge, close, park, or reviewer status on {subject_key}; "
            + ("ETA gate passed, but keep owner decision explicit." if eta_ready else "keep this as age/staleness risk triage, not an ETA commitment.")
        ),
        rationale=(
            "Source oracle: current PR state is open with observed detail coverage; "
            f"age {age_days:.2f}d exceeds the generated slow-cycle threshold {threshold_days:.2f}d by {overdue_days:.2f}d "
            f"with risk score {risk_score:.0f}. ETA readiness is {str(eta_ready).lower()}. "
            "This is a consistency check over generated forecast thresholds, not an independent precision label."
        ),
        oracle_kind="forecast_risk_source_state",
        evidence_summary=f"state=open age_days={age_days:.2f} threshold_days={threshold_days:.2f} overdue_days={overdue_days:.2f} risk_score={risk_score:.0f}",
    )


def blocker_candidate_source_label(
    insight: dict[str, Any],
    forecast: dict[str, Any] | None,
    action: dict[str, Any],
) -> dict[str, str] | None:
    subject_key = clean(insight.get("subject_key"))
    current_state = clean(action.get("current_state")) or clean((forecast or {}).get("state"))
    if current_state != "open":
        return None
    if clean((forecast or {}).get("source_current_detail_state")) == "failed":
        return None
    excerpt = clean(insight.get("evidence_excerpt")) or clean(action.get("evidence_summary"))
    if not excerpt:
        return None
    signal = blocker_signal_from_title(clean(insight.get("title"))) or "source blocker-like signal"
    coverage = clean(action.get("source_coverage_kind")) or clean((forecast or {}).get("source_current_coverage_state")) or clean(insight.get("evidence_locator_kind")) or "source_excerpt"
    return base_label(
        insight,
        truth_label="partial",
        actionability_label="needs_owner",
        review_state="needs_more_data",
        owner_key=clean(action.get("owner_hint")),
        next_action=f"Ask the owner whether the current source signal for {subject_key} is an actual blocker, already understood context, or non-blocking.",
        rationale=(
            "Source oracle: current source state is open and the evidence excerpt contains a blocker-like signal "
            f"({signal}). This is measurement evidence for a blocker-validation lead only; it is not owner-confirmed blocker clearance, dependency state, or autonomous escalation."
        ),
        oracle_kind="blocker_candidate_source_signal",
        evidence_summary=f"state=open signal={signal} coverage={coverage} excerpt={excerpt[:160]}",
    )


def model_quality_label(insight: dict[str, Any], forecast_summary: dict[str, str]) -> dict[str, str] | None:
    eta_ready = forecast_summary.get("eta_forecast_ready", "").lower()
    best_model = forecast_summary.get("backtest_best_model", "")
    median_mae = safe_float(forecast_summary.get("backtest_median_mae_days"))
    rf_mae = safe_float(forecast_summary.get("backtest_random_forest_mae_days"))
    if eta_ready != "false" or not best_model or median_mae <= 0 or rf_mae <= 0:
        return None
    if rf_mae <= median_mae:
        return None
    return base_label(
        insight,
        truth_label="true_positive",
        actionability_label="actionable",
        review_state="accepted",
        next_action="Keep ETA commitments gated; continue collecting time-series snapshots and outcome labels before forecast automation.",
        rationale=(
            "Source oracle: forecast backtest marks ETA readiness false because the learned random-forest model "
            f"MAE {rf_mae:.2f}d does not beat the median baseline MAE {median_mae:.2f}d; best model is {best_model}. "
            "This validates forecast gating consistency, not field precision."
        ),
        oracle_kind="forecast_backtest_quality",
        evidence_summary=f"eta_forecast_ready=false best_model={best_model} median_mae={median_mae:.2f} rf_mae={rf_mae:.2f}",
    )


def ci_status_label(
    insight: dict[str, Any],
    observation: dict[str, Any] | None,
    action: dict[str, Any],
) -> dict[str, str] | None:
    if not observation:
        return None
    signal = clean(observation.get("combined_signal"))
    if signal not in {"failing_checks", "pending_checks", "failing_or_pending"}:
        return None
    if clean(observation.get("effective_state")) != "open":
        return None
    failing = int(safe_float(observation.get("failing_context_count")))
    pending = int(safe_float(observation.get("pending_context_count")))
    if failing + pending <= 0:
        return None
    required_state = clean(observation.get("required_check_coverage_state")) or clean(action.get("required_check_coverage_state"))
    subject_key = clean(insight.get("subject_key"))
    return base_label(
        insight,
        truth_label="true_positive",
        actionability_label="needs_owner",
        review_state="accepted",
        owner_key=clean(action.get("owner_hint")),
        next_action=f"Assign an owner to decide whether {subject_key} check failures or pending checks block work; record non-blocking if required checks do not apply.",
        rationale=(
            "Source oracle: GitHub check/status observation shows an open PR with "
            f"{failing} failing and {pending} pending contexts. Required-check coverage is {required_state or 'unknown'}, "
            "so this validates a review lead, not a merge-blocker claim."
        ),
        oracle_kind="ci_status_source_state",
        evidence_summary=f"signal={signal} failing_context_count={failing} pending_context_count={pending} required_check_coverage_state={required_state}",
    )


def reviewer_wait_status_label(
    insight: dict[str, Any],
    forecast: dict[str, Any] | None,
    action: dict[str, Any],
) -> dict[str, str] | None:
    if not forecast:
        return None
    if clean(forecast.get("state")) != "open":
        return None
    if clean(forecast.get("source_current_detail_state")) == "failed":
        return None
    requested_count = int(safe_float(forecast.get("requested_reviewer_count")))
    requested = clean(forecast.get("requested_reviewers"))
    if requested_count <= 0 or not requested:
        return None
    subject_key = clean(insight.get("subject_key"))
    return base_label(
        insight,
        truth_label="true_positive",
        actionability_label="needs_owner",
        review_state="accepted",
        owner_key=clean(action.get("owner_hint")),
        next_action=f"Confirm whether the requested reviewer list for {subject_key} is still the right review path, then reassign, merge, park, or close.",
        rationale=(
            "Source oracle: current PR is open with observed detail coverage and requested reviewer evidence is present "
            f"({requested_count} reviewer entry). This validates owner-confirmation work, not reviewer inactivity."
        ),
        oracle_kind="reviewer_wait_source_state",
        evidence_summary=f"state=open requested_reviewer_count={requested_count} requested_reviewers={requested}",
    )


def dependency_cluster_source_label(insight: dict[str, Any], edge_summary: dict[str, int]) -> dict[str, str] | None:
    subject_kind = clean(insight.get("subject_kind"))
    subject_key = clean(insight.get("subject_key"))
    if subject_kind != "ticket" or not subject_key:
        return None
    ticket_pr_count = int(edge_summary.get("ticket_pr_count", 0))
    partial_count = int(edge_summary.get("partial_remote_link_count", 0))
    if ticket_pr_count < 3:
        return None
    return base_label(
        insight,
        truth_label="partial",
        actionability_label="needs_owner",
        review_state="needs_more_data",
        next_action=(
            f"Split {subject_key} status by linked PR and ask owners which PR, if any, is blocking or needs coordination; "
            "record non-blocking context separately."
        ),
        rationale=(
            "Source oracle: typed dependency topology has "
            f"{ticket_pr_count} ticket-to-PR edge(s) for this ticket, including {partial_count} partial remote-link stub(s). "
            "This validates a coordination-review lead only; it is not a source-confirmed blocking dependency, owner-confirmed coordination action, or product-action claim."
        ),
        oracle_kind="dependency_cluster_topology",
        evidence_summary=f"ticket_pr_edges={ticket_pr_count} partial_remote_link_edges={partial_count} model=ticket_pr_component_rule",
    )


def developer_correlation_source_label(insight: dict[str, Any], row: dict[str, Any] | None) -> dict[str, str] | None:
    if not row:
        return None
    if clean(row.get("identity_bridge_state")) != "direct_github_jira_person":
        return None
    if clean(row.get("correlation_state")) != "correlatable_same_identity":
        return None
    pr_count = int(safe_float(row.get("pr_authored_count")))
    extra_ticket_count = int(safe_float(row.get("extra_jira_ticket_count")))
    if pr_count <= 0 or extra_ticket_count <= 0:
        return None
    open_pr_count = int(safe_float(row.get("open_pr_authored_count")))
    high_risk_open_pr_count = int(safe_float(row.get("high_risk_open_pr_count")))
    open_extra_ticket_count = int(safe_float(row.get("open_extra_jira_ticket_count")))
    blocker_ticket_count = int(safe_float(row.get("extra_jira_blocker_ticket_count")))
    display_name = clean(row.get("display_name")) or clean(insight.get("subject_key"))
    subject_key = clean(insight.get("subject_key"))
    return base_label(
        insight,
        truth_label="partial",
        actionability_label="needs_owner",
        review_state="needs_more_data",
        next_action=(
            f"Review {display_name}'s same-window PR/Jira workload context; ask whether it changes routing, capacity, priority, or escalation, "
            "and record non-actionable context separately."
        ),
        rationale=(
            "Source oracle: typed Person identity has both GitHub and Jira identity fields, and analytics observed "
            f"{pr_count} authored PR(s) plus {extra_ticket_count} extra same-window Jira ticket(s). "
            "This validates a workload/routing review lead only; it is not causality, ownership, performance, ETA, blocker presence, or blocker absence."
        ),
        oracle_kind="developer_correlation_workload_context",
        evidence_summary=(
            f"person={subject_key} pr_authored={pr_count} open_prs={open_pr_count} high_risk_open_prs={high_risk_open_pr_count} "
            f"extra_jira={extra_ticket_count} open_extra_jira={open_extra_ticket_count} extra_jira_blocker_keywords={blocker_ticket_count}"
        ),
    )


def base_label(
    insight: dict[str, Any],
    *,
    truth_label: str,
    actionability_label: str,
    review_state: str,
    next_action: str,
    rationale: str,
    oracle_kind: str,
    evidence_summary: str,
    owner_key: str = "",
    measurement_eligible: str = "false",
) -> dict[str, str]:
    return {
        "insight_key": clean(insight.get("insight_key")),
        "insight_kind": clean(insight.get("insight_kind")),
        "subject_kind": clean(insight.get("subject_kind")),
        "subject_key": clean(insight.get("subject_key")),
        "truth_label": truth_label,
        "actionability_label": actionability_label,
        "review_state": review_state,
        "owner_key": owner_key,
        "next_action": next_action,
        "rationale": rationale,
        "oracle_kind": oracle_kind,
        "evidence_summary": evidence_summary,
        "measurement_eligible": measurement_eligible,
    }


def by_subject_key(rows: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    return {clean(row.get("subject_key")): row for row in rows if clean(row.get("subject_key"))}


def by_person_key(rows: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    return {clean(row.get("person_key")): row for row in rows if clean(row.get("person_key"))}


def latest_action_by_subject(rows: list[dict[str, Any]]) -> dict[tuple[str, str], dict[str, Any]]:
    ordered = sorted(rows, key=lambda row: safe_float(row.get("priority_score")), reverse=True)
    result: dict[tuple[str, str], dict[str, Any]] = {}
    for row in ordered:
        key = (clean(row.get("subject_kind")), clean(row.get("subject_key")))
        if key[1] and key not in result:
            result[key] = row
    return result


def dependency_edges_by_ticket(rows: list[dict[str, Any]]) -> dict[str, dict[str, int]]:
    result: dict[str, dict[str, int]] = {}
    for row in rows:
        if clean(row.get("edge_kind")) != "ticket_pr":
            continue
        ticket_key = ticket_key_from_edge(row)
        if not ticket_key:
            continue
        summary = result.setdefault(ticket_key, {"ticket_pr_count": 0, "partial_remote_link_count": 0})
        summary["ticket_pr_count"] += 1
        if clean(row.get("risk_signal")) == "partial_remote_link":
            summary["partial_remote_link_count"] += 1
    return result


def ticket_key_from_edge(row: dict[str, Any]) -> str:
    for key in [clean(row.get("source_key")), clean(row.get("target_key"))]:
        if key.startswith("ticket:"):
            return key.removeprefix("ticket:")
    return ""


def blocker_signal_from_title(title: str) -> str:
    prefix = "possible blocker signal:"
    lowered = title.lower()
    if lowered.startswith(prefix):
        return title[len(prefix) :].strip()
    return ""


def metric_map(rows: list[dict[str, Any]]) -> dict[str, str]:
    return {clean(row.get("metric")): clean(row.get("value")) for row in rows if clean(row.get("metric"))}


def write_tsv(rows: list[dict[str, Any]], path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=OUTPUT_COLUMNS, delimiter="\t", extrasaction="ignore")
        writer.writeheader()
        writer.writerows(rows)


def safe_float(value: Any) -> float:
    try:
        result = float(value)
    except (TypeError, ValueError):
        return 0.0
    return result


def clean(value: Any) -> str:
    return "" if value is None else str(value).strip()


if __name__ == "__main__":
    main()
