#!/usr/bin/env python3
"""Generate a durable AI-TPM validation report from persisted DBs.

This report intentionally reads persisted ontology and analytics rows instead
of scraping prior Markdown. It is a checkpoint for the replacement question:
which TPM functions can Cubicle do now, which are only assisted, and which gates
still block autonomous product claims.
"""

from __future__ import annotations

import argparse
import sqlite3
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable


ONTOLOGY_TABLES = [
    "tickets",
    "pull_requests",
    "ticket_pull_requests",
    "evidences",
    "work_program_items",
    "work_actions",
    "work_action_observations",
    "work_blockers",
    "work_blocker_impacts",
    "work_dependency_endpoints",
    "work_decision_target_evaluations",
    "work_forecast_evaluations",
    "work_item_forecasts",
    "work_program_evidence_needs",
    "work_program_milestones",
    "work_program_quality_gates",
    "work_program_adversarial_checks",
    "work_program_tpm_function_readinesses",
    "work_owner_load_snapshots",
    "source_sync_issues",
]

ANALYTICS_TABLES = [
    "tpm_pr_forecasts",
    "tpm_pr_source_coverage",
    "tpm_forecast_backtest",
    "tpm_forecast_summary",
    "tpm_forecast_reliability",
    "tpm_forecast_risk_backtest",
    "tpm_decision_target_backtest",
    "tpm_blocker_candidates",
    "tpm_action_items",
    "tpm_action_summary",
    "tpm_work_action_observations",
    "tpm_followup_observations",
    "tpm_pr_check_observations",
    "tpm_followup_summary",
    "tpm_check_summary",
    "tpm_check_signal_readiness",
    "tpm_transition_signal_readiness",
    "tpm_current_insight_cards",
    "tpm_insight_review_queue",
    "tpm_review_labels",
    "tpm_review_metrics",
    "tpm_evaluation_readiness",
    "tpm_measurement_label_summary",
    "tpm_developer_correlation",
    "tpm_developer_correlation_validation",
    "tpm_dependency_edges",
    "tpm_program_register",
    "tpm_owner_action_rollup",
    "tpm_workstream_standup",
]

WORK_PROGRAM_RUN_MEMBER_TABLES = [
    "work_program_quality_gates",
    "work_program_evidence_needs",
    "work_program_adversarial_checks",
    "work_program_automation_readinesses",
    "work_program_tpm_function_readinesses",
    "work_program_summary_snapshots",
    "work_program_brief_snapshots",
    "work_program_brief_caveats",
    "work_program_milestones",
    "work_program_risk_drivers",
    "work_program_owner_rollup_snapshots",
    "work_owner_load_snapshots",
]

PASS_STATES = {"pass", "passed", "ok", "ready"}
LIMITED_COVERAGE_PREFIXES = ("partial", "unknown", "missing")


@dataclass(frozen=True)
class LatestRun:
    source_instance: str
    workstream_key: str
    generated_at: str
    run_key: str
    readiness_state: str
    readiness_score: float
    human_review_required: bool
    autonomous_action_ready: bool
    evidence_need_count: int
    blocking_gate_count: int
    tpm_function_count: int
    external_id: str


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ontology-db", required=True, type=Path)
    parser.add_argument("--analytics-db", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    parser.add_argument("--source-instance", default=None)
    parser.add_argument("--workstream-key", default=None)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    report = build_validation_report(
        args.ontology_db,
        args.analytics_db,
        source_instance=args.source_instance,
        workstream_key=args.workstream_key,
    )
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(report, encoding="utf-8")


def build_validation_report(
    ontology_db: Path,
    analytics_db: Path,
    *,
    source_instance: str | None = None,
    workstream_key: str | None = None,
) -> str:
    with sqlite3.connect(ontology_db) as ontology_conn, sqlite3.connect(analytics_db) as analytics_conn:
        ontology_conn.row_factory = sqlite3.Row
        analytics_conn.row_factory = sqlite3.Row

        latest = latest_readiness_run(ontology_conn, source_instance, workstream_key)
        if latest is None:
            return no_run_report(ontology_db, analytics_db, source_instance, workstream_key)

        ontology_counts = table_counts(ontology_conn, ONTOLOGY_TABLES)
        analytics_counts = table_counts(analytics_conn, ANALYTICS_TABLES)
        graph_integrity = graph_integrity_summary(ontology_conn, latest)
        gates = latest_quality_gates(ontology_conn, latest)
        evidence_needs = latest_evidence_needs(ontology_conn, latest)
        function_readiness = latest_function_readiness(ontology_conn, latest)
        adversarial = latest_adversarial_checks(ontology_conn, latest)
        owner_load = latest_owner_load(ontology_conn, latest)
        program_item_coverage = work_program_item_coverage(ontology_conn, latest)
        forecast_summary = metric_table(analytics_conn, "tpm_forecast_summary")
        forecast_reliability = forecast_reliability_rows(analytics_conn)
        forecast_backtest = forecast_backtest_rows(analytics_conn)
        decision_target_backtest = decision_target_backtest_rows(analytics_conn)
        decision_target_evaluations = decision_target_evaluation_rows(ontology_conn, latest)
        forecast_rollup = work_item_forecast_rollup(ontology_conn, latest)
        blocker_rollup = blocker_summary(ontology_conn, analytics_conn, latest)
        action_rollup = action_summary(ontology_conn, analytics_conn, latest)
        execution_observations = execution_observation_summary(ontology_conn, analytics_conn, latest)
        source_rollup = source_coverage_summary(ontology_conn, latest, program_item_coverage, gates, evidence_needs)
        pr_source_coverage = pr_source_coverage_summary(analytics_conn)
        measurement_readiness = measurement_readiness_summary(analytics_conn)
        correlation_rollup = developer_correlation_summary(analytics_conn)
        correlation_validation = developer_correlation_validation_rows(analytics_conn)

        verdict = ai_tpm_verdict(latest, gates, adversarial, source_rollup, forecast_summary, evidence_needs)
        lines: list[str] = []
        lines.append("# AI-TPM Validation Report")
        lines.append("")
        lines.append(f"- Ontology DB: `{ontology_db}`")
        lines.append(f"- Analytics DB: `{analytics_db}`")
        lines.append(f"- Source instance: `{latest.source_instance}`")
        lines.append(f"- Workstream: `{latest.workstream_key}`")
        lines.append(f"- Latest run: `{latest.generated_at}`")
        lines.append(f"- Verdict: `{verdict}`")
        lines.append("")
        lines.extend(verdict_explanation(verdict, latest, gates, source_rollup, pr_source_coverage, forecast_summary, adversarial))
        lines.append("")
        lines.append("## Persisted Coverage")
        lines.append("")
        lines.extend(markdown_table(["table", "rows"], sorted(ontology_counts.items())))
        lines.append("")
        lines.append("## Analytics Coverage")
        lines.append("")
        lines.extend(markdown_table(["table", "rows"], sorted(analytics_counts.items())))
        lines.append("")
        lines.append("## Graph Integrity")
        lines.append("")
        lines.extend(markdown_table(["check", "value", "note"], graph_integrity))
        lines.append("")
        lines.append("## Latest Automation Run")
        lines.append("")
        lines.extend(
            markdown_table(
                ["metric", "value"],
                [
                    ("readiness_state", latest.readiness_state),
                    ("readiness_score", format_float(latest.readiness_score)),
                    ("autonomous_action_ready", bool_text(latest.autonomous_action_ready)),
                    ("human_review_required", bool_text(latest.human_review_required)),
                    ("blocking_gate_count", latest.blocking_gate_count),
                    ("evidence_need_count", latest.evidence_need_count),
                    ("tpm_function_count", latest.tpm_function_count),
                ],
            )
        )
        lines.append("")
        lines.append("## Quality Gates")
        lines.append("")
        if gates:
            lines.extend(
                markdown_table(
                    ["gate", "state", "blocking", "detail"],
                    [
                        (
                            row["gate_key"],
                            row["gate_state"],
                            bool_text(as_bool(row["blocking"])),
                            row["detail"],
                        )
                        for row in gates
                    ],
                )
            )
        else:
            lines.append("No quality gates were persisted for the latest run.")
        lines.append("")
        lines.append("## TPM Function Readiness")
        lines.append("")
        if function_readiness:
            lines.extend(
                markdown_table(
                    ["function", "readiness", "automation", "human_required", "blocking_gates"],
                    [
                        (
                            row["function_key"],
                            row["readiness_state"],
                            row["automation_state"],
                            bool_text(as_bool(row["human_required"])),
                            one_line(row["blocking_gate_keys"]),
                        )
                        for row in function_readiness
                    ],
                )
            )
        else:
            lines.append("No TPM function readiness rows were persisted for the latest run.")
        lines.append("")
        lines.append("## Evidence Needs")
        lines.append("")
        lines.extend(group_rows_table(evidence_needs, ["gate_key", "evidence_kind", "priority", "execution_state"]))
        lines.append("")
        lines.append("## Forecasting")
        lines.append("")
        lines.extend(
            markdown_table(
                ["metric", "value", "note"],
                forecast_metric_rows(forecast_summary, forecast_rollup),
            )
        )
        if forecast_reliability:
            lines.append("")
            lines.append("Forecast product reliability:")
            lines.append("")
            lines.extend(
                markdown_table(
                    ["product", "state", "product_safe", "safe_use", "metric", "value", "next_evidence", "guardrail"],
                    [
                        (
                            row.get("forecast_product", ""),
                            row.get("readiness_state", ""),
                            row.get("product_safe", ""),
                            row.get("safe_use", ""),
                            row.get("primary_metric", ""),
                            row.get("metric_value", ""),
                            row.get("next_evidence", ""),
                            row.get("guardrail", ""),
                        )
                        for row in forecast_reliability
                    ],
                )
            )
        if forecast_backtest:
            lines.append("")
            lines.append("Backtest rows:")
            lines.append("")
            lines.extend(
                markdown_table(
                    ["evaluation", "model", "mae_days", "ready_for_eta", "note"],
                    [
                        (
                            row.get("evaluation", ""),
                            row.get("model", ""),
                            format_float(row.get("mae_days")),
                            row.get("ready_for_eta", ""),
                            row.get("note", ""),
                        )
                        for row in forecast_backtest[:8]
                    ],
                )
            )
        if decision_target_backtest:
            lines.append("")
            lines.append("TPM decision-target analytics rows:")
            lines.append("")
            lines.extend(
                markdown_table(
                    ["target", "evaluation", "model", "coverage_stratum", "precision_at_10pct", "lift_at_10pct", "ready_for_product_action", "note"],
                    [
                        (
                            row.get("target_kind", ""),
                            row.get("evaluation", ""),
                            row.get("model", ""),
                            row.get("coverage_stratum", ""),
                            format_float(row.get("precision_at_10pct")),
                            format_float(row.get("lift_at_10pct")),
                            row.get("ready_for_product_action", ""),
                            row.get("note", ""),
                        )
                        for row in decision_target_report_rows(
                            decision_target_backtest,
                            evaluation_key="evaluation",
                            model_key="model",
                        )
                    ],
                )
            )
        if decision_target_evaluations:
            lines.append("")
            lines.append("Persisted decision-target evaluation rows:")
            lines.append("")
            lines.extend(
                markdown_table(
                    ["target", "evaluation", "model", "coverage_stratum", "precision_at_10pct", "lift_at_10pct", "ready_for_product_action", "gate_state", "note"],
                    [
                        (
                            row.get("target_kind", ""),
                            row.get("evaluation_kind", ""),
                            row.get("model_name", ""),
                            row.get("coverage_stratum", ""),
                            format_float(row.get("precision_at_10pct")),
                            format_float(row.get("lift_at_10pct")),
                            bool_text(as_bool(row.get("ready_for_product_action"))),
                            row.get("product_action_gate_state", ""),
                            row.get("note", ""),
                        )
                        for row in decision_target_report_rows(
                            decision_target_evaluations,
                            evaluation_key="evaluation_kind",
                            model_key="model_name",
                        )
                    ],
                )
            )
        lines.append("")
        lines.append("## Measurement Readiness")
        lines.append("")
        lines.extend(markdown_table(["metric", "value", "note"], measurement_readiness))
        lines.append("")
        lines.append("## Blockers And Actions")
        lines.append("")
        lines.extend(markdown_table(["metric", "value"], blocker_rollup + action_rollup))
        lines.append("")
        lines.append("## Execution Observations")
        lines.append("")
        lines.extend(markdown_table(["metric", "value", "note"], execution_observations))
        lines.append("")
        lines.append("## Source Coverage")
        lines.append("")
        lines.extend(markdown_table(["metric", "value"], source_rollup))
        if pr_source_coverage:
            lines.append("")
            lines.append("PR source coverage:")
            lines.append("")
            lines.extend(markdown_table(["metric", "value"], pr_source_coverage))
        if program_item_coverage:
            lines.append("")
            lines.append("Program item coverage states:")
            lines.append("")
            lines.extend(markdown_table(["source_coverage_state", "items"], program_item_coverage))
        lines.append("")
        lines.append("## Adversarial Checks")
        lines.append("")
        lines.extend(group_rows_table(adversarial, ["check_state", "severity", "check_kind"]))
        lines.append("")
        lines.append("## Owner Load")
        lines.append("")
        lines.extend(group_rows_table(owner_load, ["load_status"]))
        lines.append("")
        lines.append("## Developer Correlation")
        lines.append("")
        lines.extend(markdown_table(["metric", "value"], correlation_rollup))
        if correlation_validation:
            lines.append("")
            lines.append("Developer correlation validation:")
            lines.append("")
            lines.extend(
                markdown_table(
                    ["metric", "value", "sample_count", "method", "interpretation", "guardrail"],
                    correlation_validation,
                )
            )
        lines.append("")
        lines.append("## Replacement Boundary")
        lines.append("")
        lines.append("- Safe now: operating brief generation, ranked risk/blocker queues, source repair queues, owner-load triage, and evidence review routing.")
        lines.append("- Not safe yet: autonomous absence claims, autonomous owner/product decisions, and ETA commitments.")
        lines.append("- Next validation: clear blocking source/auth/provenance issues when present, add measurement labels by insight kind, keep forecast leakage and coverage-stratification guards passing, then rerun this report and the persisted GraphQL packet smoke.")
        lines.append("")
        return "\n".join(lines)


def no_run_report(ontology_db: Path, analytics_db: Path, source_instance: str | None, workstream_key: str | None) -> str:
    return "\n".join(
        [
            "# AI-TPM Validation Report",
            "",
            f"- Ontology DB: `{ontology_db}`",
            f"- Analytics DB: `{analytics_db}`",
            f"- Source instance filter: `{source_instance or ''}`",
            f"- Workstream filter: `{workstream_key or ''}`",
            "- Verdict: `no_automation_readiness_run`",
            "",
            "No persisted `work_program_automation_readinesses` row matched the requested filters.",
            "",
        ]
    )


def latest_readiness_run(conn: sqlite3.Connection, source_instance: str | None, workstream_key: str | None) -> LatestRun | None:
    if not table_exists(conn, "work_program_automation_readinesses"):
        return None
    predicates: list[str] = []
    params: list[Any] = []
    if source_instance:
        predicates.append("source_instance = ?")
        params.append(source_instance)
    if workstream_key:
        predicates.append("workstream_key = ?")
        params.append(workstream_key)
    where = f"where {' and '.join(predicates)}" if predicates else ""
    row = conn.execute(
        f"""
        select *
          from work_program_automation_readinesses
          {where}
         order by generated_at desc, rank_score desc, id desc
         limit 1
        """,
        params,
    ).fetchone()
    if row is None:
        return None
    return LatestRun(
        source_instance=str(row["source_instance"] or ""),
        workstream_key=str(row["workstream_key"] or ""),
        generated_at=str(row["generated_at"] or ""),
        run_key=latest_work_program_run_key(conn, str(row["source_instance"] or ""), str(row["workstream_key"] or ""), str(row["generated_at"] or "")),
        readiness_state=str(row["readiness_state"] or ""),
        readiness_score=float(row["readiness_score"] or 0),
        human_review_required=as_bool(row["human_review_required"]),
        autonomous_action_ready=as_bool(row["autonomous_action_ready"]),
        evidence_need_count=int(row["evidence_need_count"] or 0),
        blocking_gate_count=int(row["blocking_gate_count"] or 0),
        tpm_function_count=int(row["tpm_function_count"] or 0),
        external_id=str(row["external_id"] or ""),
    )


def latest_quality_gates(conn: sqlite3.Connection, latest: LatestRun) -> list[sqlite3.Row]:
    return rows_for_run(
        conn,
        "work_program_quality_gates",
        latest,
        "gate_key, gate_state, blocking, detail, recommended_action",
        "blocking desc, gate_key",
    )


def latest_evidence_needs(conn: sqlite3.Connection, latest: LatestRun) -> list[sqlite3.Row]:
    return rows_for_run(
        conn,
        "work_program_evidence_needs",
        latest,
        "gate_key, evidence_kind, priority, execution_state, target_kind, target_key, recommended_action",
        "gate_key, priority desc, evidence_kind",
    )


def latest_function_readiness(conn: sqlite3.Connection, latest: LatestRun) -> list[sqlite3.Row]:
    return rows_for_run(
        conn,
        "work_program_tpm_function_readinesses",
        latest,
        "function_key, readiness_state, automation_state, human_required, blocking_gate_keys, detail, recommended_action",
        "function_key",
    )


def latest_adversarial_checks(conn: sqlite3.Connection, latest: LatestRun) -> list[sqlite3.Row]:
    return rows_for_run(
        conn,
        "work_program_adversarial_checks",
        latest,
        "check_kind, check_state, severity, title, detail, recommended_action",
        "case when check_state in ('fail','warning') then 0 else 1 end, severity, check_kind",
    )


def latest_owner_load(conn: sqlite3.Connection, latest: LatestRun) -> list[sqlite3.Row]:
    return rows_for_run(
        conn,
        "work_owner_load_snapshots",
        latest,
        "load_status, owner_key, action_count, critical_or_high_count, coverage_limited_count, needs_human_review_count",
        "load_status, action_count desc",
    )


def rows_for_run(
    conn: sqlite3.Connection,
    table: str,
    latest: LatestRun,
    columns: str,
    order_by: str,
) -> list[sqlite3.Row]:
    if not table_exists(conn, table):
        return []
    values = generated_at_values(latest.generated_at)
    placeholders = ",".join("?" for _ in values)
    workstream_keys = workstream_sql_keys(latest.workstream_key)
    workstream_placeholders = ",".join("?" for _ in workstream_keys)
    return list(
        conn.execute(
            f"""
            select {columns}
              from {table}
             where source_instance = ?
               and workstream_key in ({workstream_placeholders})
               and generated_at in ({placeholders})
             order by {order_by}
            """,
            [latest.source_instance, *workstream_keys, *values],
        )
    )


def work_program_item_coverage(conn: sqlite3.Connection, latest: LatestRun) -> list[tuple[str, int]]:
    if not table_exists(conn, "work_program_items"):
        return []
    rows = conn.execute(
        """
        select coalesce(source_coverage_state, '') as source_coverage_state, count(*) as count
          from work_program_items
         where source_instance = ?
           and workstream_key = ?
         group by source_coverage_state
         order by count desc, source_coverage_state
        """,
        [latest.source_instance, latest.workstream_key],
    ).fetchall()
    return [(str(row["source_coverage_state"] or ""), int(row["count"] or 0)) for row in rows]


def work_item_forecast_rollup(conn: sqlite3.Connection, latest: LatestRun) -> list[tuple[str, str]]:
    if not table_exists(conn, "work_item_forecasts"):
        return []
    rows = conn.execute(
        """
        select risk_band, ready_for_eta, count(*) as count
          from work_item_forecasts
         where source_instance = ?
         group by risk_band, ready_for_eta
         order by risk_band, ready_for_eta
        """,
        [latest.source_instance],
    ).fetchall()
    total = 0
    eta_ready = 0
    high_or_critical = 0
    for row in rows:
        count = int(row["count"] or 0)
        total += count
        if as_bool(row["ready_for_eta"]):
            eta_ready += count
        if str(row["risk_band"] or "") in {"high", "critical"}:
            high_or_critical += count
    return [
        ("persisted_forecast_count", str(total)),
        ("eta_ready_forecast_count", str(eta_ready)),
        ("high_or_critical_forecast_count", str(high_or_critical)),
    ]


def blocker_summary(
    ontology_conn: sqlite3.Connection,
    analytics_conn: sqlite3.Connection,
    latest: LatestRun,
) -> list[tuple[str, str]]:
    out: list[tuple[str, str]] = []
    if table_exists(ontology_conn, "work_blockers"):
        out.append(("work_blocker_count", str(scoped_count(ontology_conn, "work_blockers", latest.source_instance))))
        for state, count in grouped_counts(ontology_conn, "work_blockers", ["blocker_state"], source_instance=latest.source_instance):
            out.append((f"work_blocker_state:{state}", str(count)))
    if table_exists(ontology_conn, "work_blocker_impacts"):
        out.append(("work_blocker_impact_count", str(scoped_count(ontology_conn, "work_blocker_impacts", latest.source_instance))))
    if table_exists(analytics_conn, "tpm_blocker_candidates"):
        out.append(("analytics_blocker_candidate_count", str(table_count(analytics_conn, "tpm_blocker_candidates"))))
    return out or [("blocker_data", "missing")]


def action_summary(
    ontology_conn: sqlite3.Connection,
    analytics_conn: sqlite3.Connection,
    latest: LatestRun,
) -> list[tuple[str, str]]:
    out: list[tuple[str, str]] = []
    if table_exists(ontology_conn, "work_actions"):
        out.append(("work_action_count", str(scoped_count(ontology_conn, "work_actions", latest.source_instance))))
        for key, count in grouped_counts(ontology_conn, "work_actions", ["decision_state"], source_instance=latest.source_instance):
            out.append((f"work_action_decision:{key}", str(count)))
    if table_exists(analytics_conn, "tpm_action_items"):
        out.append(("analytics_action_item_count", str(table_count(analytics_conn, "tpm_action_items"))))
        if column_exists(analytics_conn, "tpm_action_items", "decision_state"):
            for key, count in grouped_counts(analytics_conn, "tpm_action_items", ["decision_state"]):
                out.append((f"analytics_action_decision:{key}", str(count)))
    return out or [("action_data", "missing")]


def execution_observation_summary(
    ontology_conn: sqlite3.Connection,
    analytics_conn: sqlite3.Connection,
    latest: LatestRun,
) -> list[tuple[str, str, str]]:
    rows: list[tuple[str, str, str]] = []
    rows.extend(work_action_observation_summary(ontology_conn, latest))
    rows.extend(metric_table_rows(analytics_conn, "tpm_followup_summary", "followup"))
    rows.extend(metric_table_rows(analytics_conn, "tpm_check_summary", "check"))
    rows.extend(check_signal_readiness_rows(analytics_conn))
    rows.extend(transition_signal_readiness_rows(analytics_conn))
    if not rows:
        return [("execution_observations", "missing", "no live follow-up, check, or action-observation rows found")]
    return rows


def work_action_observation_summary(conn: sqlite3.Connection, latest: LatestRun) -> list[tuple[str, str, str]]:
    if not table_exists(conn, "work_action_observations"):
        return []
    predicates = []
    params: list[Any] = []
    if column_exists(conn, "work_action_observations", "source_instance"):
        predicates.append("source_instance = ?")
        params.append(latest.source_instance)
    if column_exists(conn, "work_action_observations", "observed_at"):
        values = generated_at_values(latest.generated_at)
        placeholders = ",".join("?" for _ in values)
        predicates.append(f"observed_at in ({placeholders})")
        params.extend(values)
    where = f"where {' and '.join(predicates)}" if predicates else ""
    total = int(conn.execute(f"select count(*) from work_action_observations {where}", params).fetchone()[0] or 0)
    out = [
        (
            "work_action_observation_count",
            str(total),
            "latest-run action observations with source, CI, closeout, or QA evidence",
        )
    ]
    if total == 0:
        return out
    if column_exists(conn, "work_action_observations", "supports_action"):
        out.append(
            (
                "work_action_observation_supports_action_count",
                str(
                    int(
                        conn.execute(
                            f"select count(*) from work_action_observations {where} and supports_action = 1"
                            if where
                            else "select count(*) from work_action_observations where supports_action = 1",
                            params,
                        ).fetchone()[0]
                        or 0
                    )
                ),
                "observations currently allowed to support measurement-backed product actions",
            )
        )
    if all(column_exists(conn, "work_action_observations", column) for column in ["source_coverage_state", "auth_state"]):
        limited_or_auth = latest_observation_limited_or_anonymous_count(conn, where, params)
        out.append(
            (
                "work_action_observation_limited_or_auth_count",
                str(limited_or_auth),
                "observations with limited source coverage, generated claim provenance, or anonymous auth",
            )
        )
    out.extend(grouped_observation_rows(conn, where, params, "observation_kind", "work_action_observation_kind"))
    out.extend(grouped_observation_rows(conn, where, params, "source_coverage_state", "work_action_observation_coverage"))
    out.extend(grouped_observation_rows(conn, where, params, "ci_required_check_coverage_state", "work_action_observation_required_check_coverage"))
    return out


def latest_observation_limited_or_anonymous_count(conn: sqlite3.Connection, where: str, params: list[Any]) -> int:
    rows = conn.execute(
        f"""
        select coalesce(source_coverage_state, '') as source_coverage_state,
               coalesce(auth_state, '') as auth_state
          from work_action_observations
          {where}
        """,
        params,
    ).fetchall()
    count = 0
    for row in rows:
        source_state = str(row["source_coverage_state"])
        if (
            coverage_state_limited(source_state)
            or "generated" in source_state.lower()
            or "anonymous" in str(row["auth_state"] or "").lower()
        ):
            count += 1
    return count


def grouped_observation_rows(
    conn: sqlite3.Connection,
    where: str,
    params: list[Any],
    column: str,
    prefix: str,
) -> list[tuple[str, str, str]]:
    if not column_exists(conn, "work_action_observations", column):
        return []
    rows = conn.execute(
        f"""
        select coalesce({column}, '') as value, count(*) as count
          from work_action_observations
          {where}
         group by {column}
         order by count desc, value
        """,
        params,
    ).fetchall()
    out: list[tuple[str, str, str]] = []
    for row in rows:
        value = str(row["value"] or "empty")
        out.append((f"{prefix}:{value}", str(int(row["count"] or 0)), "latest-run action observation grouping"))
    return out


def metric_table_rows(conn: sqlite3.Connection, table_name: str, prefix: str) -> list[tuple[str, str, str]]:
    if not table_exists(conn, table_name):
        return []
    required = {"metric", "value", "note"}
    if not required.issubset(set(table_columns(conn, table_name))):
        return []
    rows = conn.execute(
        f"""
        select metric, value, note
          from {table_name}
         order by rowid
        """
    ).fetchall()
    return [(f"{prefix}_{row['metric']}", str(row["value"] or ""), str(row["note"] or "")) for row in rows]


def check_signal_readiness_rows(conn: sqlite3.Connection) -> list[tuple[str, str, str]]:
    if not table_exists(conn, "tpm_check_signal_readiness"):
        return []
    required = {"readiness_key", "ready", "readiness_state", "support_level", "blocking_reason"}
    if not required.issubset(set(table_columns(conn, "tpm_check_signal_readiness"))):
        return []
    rows = conn.execute(
        """
        select readiness_key, ready, readiness_state, support_level, blocking_reason
          from tpm_check_signal_readiness
         order by readiness_key
        """
    ).fetchall()
    out: list[tuple[str, str, str]] = []
    for row in rows:
        ready = "ready" if as_bool(row["ready"]) else "gated"
        out.append(
            (
                f"check_readiness:{row['readiness_key']}",
                f"{ready}:{row['readiness_state']}",
                f"{row['support_level']}: {row['blocking_reason']}",
            )
        )
    return out


def transition_signal_readiness_rows(conn: sqlite3.Connection) -> list[tuple[str, str, str]]:
    if not table_exists(conn, "tpm_transition_signal_readiness"):
        return []
    required = {"readiness_key", "ready", "readiness_state", "support_level", "blocking_reason"}
    if not required.issubset(set(table_columns(conn, "tpm_transition_signal_readiness"))):
        return []
    rows = conn.execute(
        """
        select readiness_key, ready, readiness_state, support_level, blocking_reason
          from tpm_transition_signal_readiness
         order by readiness_key
        """
    ).fetchall()
    out: list[tuple[str, str, str]] = []
    for row in rows:
        ready = "ready" if as_bool(row["ready"]) else "gated"
        out.append(
            (
                f"transition_readiness:{row['readiness_key']}",
                f"{ready}:{row['readiness_state']}",
                f"{row['support_level']}: {row['blocking_reason']}",
            )
        )
    return out


def source_coverage_summary(
    conn: sqlite3.Connection,
    latest: LatestRun,
    program_item_coverage: list[tuple[str, int]],
    gates: list[sqlite3.Row],
    evidence_needs: list[sqlite3.Row],
) -> list[tuple[str, str]]:
    issue_count, source_instance_issue_count, source_scope_issue_count = source_sync_issue_counts(conn, latest.source_instance)
    limited_items = sum(count for state, count in program_item_coverage if coverage_state_limited(state))
    source_gate = next((row for row in gates if row["gate_key"] == "source_coverage"), None)
    source_evidence = sum(1 for row in evidence_needs if row["gate_key"] == "source_coverage")
    return [
        ("source_sync_issue_count", str(issue_count)),
        ("source_sync_issue_count_matching_source_instance", str(source_instance_issue_count)),
        ("source_sync_issue_count_matching_source_scope", str(source_scope_issue_count)),
        ("limited_program_item_count", str(limited_items)),
        ("source_coverage_gate_state", str(source_gate["gate_state"] if source_gate else "missing")),
        ("source_coverage_gate_blocking", bool_text(as_bool(source_gate["blocking"]) if source_gate else False)),
        ("source_coverage_evidence_need_count", str(source_evidence)),
    ]


def pr_source_coverage_summary(conn: sqlite3.Connection) -> list[tuple[str, str]]:
    if not table_exists(conn, "tpm_pr_source_coverage"):
        return []
    out = [("pr_source_coverage_row_count", str(table_count(conn, "tpm_pr_source_coverage")))]
    for key, count in grouped_counts(conn, "tpm_pr_source_coverage", ["source_current_coverage_state", "source_current_detail_state"]):
        out.append((f"pr_source_coverage:{key}", str(count)))
    if column_exists(conn, "tpm_pr_source_coverage", "source_current_issue_code"):
        for key, count in grouped_counts(conn, "tpm_pr_source_coverage", ["source_current_issue_code"]):
            out.append((f"pr_source_issue_code:{key}", str(count)))
    return out


def measurement_readiness_summary(conn: sqlite3.Connection) -> list[tuple[str, str, str]]:
    preferred = [
        "truth_label_coverage",
        "actionability_label_coverage",
        "ready_to_measure_precision",
        "ready_to_measure_actionability",
        "evaluation_label_row_count",
        "non_measurement_label_row_count",
        "open_review_request_count",
        "measurement_labels_blocker_candidate",
        "measurement_labels_dependency_cluster",
        "measurement_labels_developer_correlation",
        "measurement_labels_forecast_risk",
        "measurement_labels_status_summary",
        "ready_to_measure_blocker_candidate",
        "ready_to_measure_dependency_cluster",
        "ready_to_measure_developer_correlation",
        "ready_to_measure_forecast_risk",
        "ready_to_measure_model_quality",
        "ready_to_measure_status_summary",
        "product_candidate_measurement_label_count",
        "product_candidate_open_review_request_count",
        "product_candidate_ready_to_measure_precision",
        "product_candidate_ready_to_measure_actionability",
        "product_action_gate_state_blocker_candidate",
        "product_action_gate_reason_blocker_candidate",
        "product_action_gate_state_forecast_risk",
        "product_action_gate_reason_forecast_risk",
        "product_action_gate_state_status_summary",
        "product_action_gate_reason_status_summary",
    ]
    summary = metric_table(conn, "tpm_evaluation_readiness")
    rows: list[tuple[str, str, str]] = []
    for metric in preferred:
        if metric in summary:
            rows.append((metric, summary[metric]["value"], summary[metric]["note"]))
    queue_summary = metric_table(conn, "tpm_measurement_label_summary")
    for metric in ["current_insight_count", "measurement_label_count", "measurement_queue_count", "non_measurement_label_count"]:
        if metric in queue_summary:
            rows.append((metric, queue_summary[metric]["value"], queue_summary[metric]["note"]))
    rows.extend(measurement_trust_boundary_rows(summary, queue_summary))
    if not rows:
        return [("measurement_readiness", "missing", "no evaluation readiness tables found")]
    return rows


def measurement_trust_boundary_rows(
    readiness_summary: dict[str, dict[str, str]],
    queue_summary: dict[str, dict[str, str]],
) -> list[tuple[str, str, str]]:
    counted = int_or_none(readiness_summary.get("evaluation_label_row_count", {}).get("value"))
    available = int_or_none(queue_summary.get("measurement_label_count", {}).get("value"))
    if counted is None and available is None:
        return []
    counted = counted or 0
    available = available or 0
    rows = [
        (
            "counted_measurement_label_count",
            str(counted),
            "measurement-eligible labels actually used by readiness gates in this analytics run",
        ),
        (
            "available_label_pack_measurement_count",
            str(available),
            "measurement labels reported by the latest label-pack or queue export; may require explicit trust promotion",
        ),
        (
            "measurement_label_trust_delta",
            str(max(0, available - counted)),
            "available label-pack labels not counted by readiness gates",
        ),
    ]
    if available > counted:
        rows.append(
            (
                "measurement_label_trust_boundary",
                "candidate_labels_not_counted",
                "rerun analytics with an approved --measurement-label-set only after the label source is accepted as measurement-grade",
            )
        )
    elif counted > 0:
        rows.append(
            (
                "measurement_label_trust_boundary",
                "trusted_labels_counted",
                "readiness gates are using the available measurement labels",
            )
        )
    return rows


def int_or_none(value: str | None) -> int | None:
    if value is None:
        return None
    try:
        return int(str(value).strip())
    except ValueError:
        return None


def developer_correlation_summary(conn: sqlite3.Connection) -> list[tuple[str, str]]:
    if not table_exists(conn, "tpm_developer_correlation"):
        return [("developer_correlation", "missing")]
    out = [("developer_correlation_rows", str(table_count(conn, "tpm_developer_correlation")))]
    if column_exists(conn, "tpm_developer_correlation", "correlation_state"):
        for key, count in grouped_counts(conn, "tpm_developer_correlation", ["correlation_state"]):
            out.append((f"correlation_state:{key}", str(count)))
    if column_exists(conn, "tpm_developer_correlation", "extra_jira_ticket_count"):
        value = conn.execute("select coalesce(sum(extra_jira_ticket_count), 0) from tpm_developer_correlation").fetchone()[0]
        out.append(("extra_jira_ticket_count", str(int(value or 0))))
    return out


def developer_correlation_validation_rows(conn: sqlite3.Connection) -> list[tuple[str, str, str, str, str, str]]:
    if not table_exists(conn, "tpm_developer_correlation_validation"):
        return []
    columns = table_columns(conn, "tpm_developer_correlation_validation")
    required = ["metric", "value", "sample_count", "method", "interpretation", "guardrail"]
    if any(column not in columns for column in required):
        return []
    rows = conn.execute(
        """
        select metric, value, sample_count, method, interpretation, guardrail
          from tpm_developer_correlation_validation
         order by case metric
                    when 'direct_identity_sample_count' then 0
                    when 'direct_identity_pr_and_extra_jira_count' then 1
                    when 'spearman_open_extra_jira_vs_high_risk_open_pr' then 2
                    when 'spearman_open_extra_jira_vs_open_pr' then 3
                    when 'spearman_ticket_pressure_vs_high_risk_open_pr' then 4
                    when 'top_quartile_open_extra_jira_threshold' then 5
                    when 'top_quartile_high_risk_open_pr_lift' then 6
                    when 'top_quartile_open_pr_lift' then 7
                    else 99
                  end,
                  metric
        """
    ).fetchall()
    return [
        (
            str(row["metric"] or ""),
            str(row["value"] or ""),
            str(row["sample_count"] or ""),
            str(row["method"] or ""),
            str(row["interpretation"] or ""),
            str(row["guardrail"] or ""),
        )
        for row in rows
    ]


def graph_integrity_summary(conn: sqlite3.Connection, latest: LatestRun) -> list[tuple[str, str, str]]:
    rows: list[tuple[str, str, str]] = []
    run_table_present = table_exists(conn, "work_program_runs")
    rows.append(
        (
            "work_program_run_table",
            "present" if run_table_present else "missing",
            "durable run boundary for generated WorkProgram rows",
        )
    )
    if run_table_present:
        rows.append(("work_program_run_key", latest.run_key or "missing", "latest readiness run mapped to durable run row"))
        rows.append(("work_program_run_member_count", str(work_program_run_member_count(conn, latest.run_key)), "rows explicitly assigned to the durable run boundary"))
        rows.extend(work_program_run_member_integrity_rows(conn, latest))
    for table in WORK_PROGRAM_RUN_MEMBER_TABLES:
        if not table_exists(conn, table):
            continue
        rows.append((f"run_generated_at_count:{table}", str(run_generated_at_count(conn, table, latest)), "distinct generated_at values for this source/workstream"))
        rows.append((f"latest_run_rows:{table}", str(latest_run_row_count(conn, table, latest)), "rows matching the selected latest generated_at"))

    for table in [
        "work_program_items",
        "work_actions",
        "work_insights",
        "work_blockers",
        "work_item_forecasts",
    ]:
        if table_exists(conn, table) and column_exists(conn, table, "latest_evidence_id"):
            rows.append((f"missing_latest_evidence:{table}", str(null_count(conn, table, "latest_evidence_id", latest.source_instance)), "product-facing TPM rows should be evidence-backed"))

    rows.extend(typed_subject_resolution_rows(conn, latest.source_instance))
    rows.extend(evidence_need_relationship_rows(conn, latest))
    rows.extend(tpm_function_readiness_relationship_rows(conn, latest))
    rows.extend(action_source_insight_rows(conn, latest.source_instance))
    rows.extend(dependency_edge_integrity_rows(conn, latest.source_instance))
    return rows


def run_generated_at_count(conn: sqlite3.Connection, table: str, latest: LatestRun) -> int:
    if not all(column_exists(conn, table, column) for column in ["source_instance", "workstream_key", "generated_at"]):
        return 0
    return int(
        conn.execute(
            f"""
            select count(distinct generated_at)
              from {table}
             where source_instance = ?
               and workstream_key = ?
            """,
            [latest.source_instance, latest.workstream_key],
        ).fetchone()[0]
        or 0
    )


def latest_work_program_run_key(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
    generated_at: str,
) -> str:
    if not table_exists(conn, "work_program_runs"):
        return ""
    values = generated_at_values(generated_at)
    placeholders = ",".join("?" for _ in values)
    row = conn.execute(
        f"""
        select run_key
          from work_program_runs
         where source_instance = ?
           and workstream_key in ({", ".join(["?"] * len(workstream_sql_keys(workstream_key)))})
           and generated_at in ({placeholders})
         order by id desc
         limit 1
        """,
        [source_instance, *workstream_sql_keys(workstream_key), *values],
    ).fetchone()
    return str(row["run_key"]) if row is not None and row["run_key"] else ""


def work_program_run_member_count(conn: sqlite3.Connection, run_key: str) -> int:
    if not run_key or not table_exists(conn, "work_program_run_members"):
        return 0
    return int(conn.execute("select count(*) from work_program_run_members where run_key = ?", [run_key]).fetchone()[0] or 0)


def work_program_run_member_integrity_rows(conn: sqlite3.Connection, latest: LatestRun) -> list[tuple[str, str, str]]:
    if not latest.run_key or not table_exists(conn, "work_program_run_members"):
        return [("work_program_run_members_table", "missing", "durable run members should be persisted for packet replay")]
    if not all(column_exists(conn, "work_program_run_members", column) for column in ["run_key", "member_table", "member_id"]):
        return [("work_program_run_members_schema", "partial", "run-member table is missing run_key/member_table/member_id")]

    member_counts = {
        str(row["member_table"] or ""): int(row["count"] or 0)
        for row in conn.execute(
            """
            select member_table, count(*) as count
              from work_program_run_members
             where run_key = ?
             group by member_table
            """,
            [latest.run_key],
        ).fetchall()
    }
    expected = set(WORK_PROGRAM_RUN_MEMBER_TABLES)
    rows: list[tuple[str, str, str]] = []
    for table in WORK_PROGRAM_RUN_MEMBER_TABLES:
        member_count = member_counts.get(table, 0)
        rows.append((f"run_member_table:{table}", str(member_count), "latest durable run members for typed packet table"))
        if member_count > 0 and table_exists(conn, table) and column_exists(conn, table, "id"):
            rows.append(
                (
                    f"run_member_missing_target:{table}",
                    str(run_member_missing_target_count(conn, latest.run_key, table)),
                    "run members whose member_id no longer resolves to the typed table",
                )
            )
        if member_count > 0 and table_exists(conn, table):
            rows.append(
                (
                    f"run_member_latest_timestamp_delta:{table}",
                    str(latest_run_row_count(conn, table, latest) - member_count),
                    "timestamp-selected row count minus explicit durable run members",
                )
            )

    unknown_count = sum(count for table, count in member_counts.items() if table not in expected)
    rows.append(("run_member_unknown_table_count", str(unknown_count), "run members that point at tables not recognized by the validation report"))
    if column_exists(conn, "work_program_run_members", "work_program_run_id"):
        missing_run_id = int(
            conn.execute(
                """
                select count(*)
                  from work_program_run_members
                 where run_key = ?
                   and work_program_run_id is null
                """,
                [latest.run_key],
            ).fetchone()[0]
            or 0
        )
        rows.append(("run_members_missing_work_program_run_id", str(missing_run_id), "latest run members should point back to their WorkProgramRun row"))
    return rows


def run_member_missing_target_count(conn: sqlite3.Connection, run_key: str, table: str) -> int:
    if table not in WORK_PROGRAM_RUN_MEMBER_TABLES:
        return 0
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_program_run_members m
              left join {table} target on target.id = m.member_id
             where m.run_key = ?
               and m.member_table = ?
               and target.id is null
            """,
            [run_key, table],
        ).fetchone()[0]
        or 0
    )


def workstream_sql_keys(workstream_key: str) -> list[str]:
    key = str(workstream_key or "")
    if key.startswith("workstream:"):
        return [key, key.removeprefix("workstream:")]
    return [key, f"workstream:{key}"]


def latest_run_row_count(conn: sqlite3.Connection, table: str, latest: LatestRun) -> int:
    if not all(column_exists(conn, table, column) for column in ["source_instance", "workstream_key", "generated_at"]):
        return 0
    values = generated_at_values(latest.generated_at)
    placeholders = ",".join("?" for _ in values)
    workstream_keys = workstream_sql_keys(latest.workstream_key)
    workstream_placeholders = ",".join("?" for _ in workstream_keys)
    return int(
        conn.execute(
            f"""
            select count(*)
              from {table}
             where source_instance = ?
               and workstream_key in ({workstream_placeholders})
               and generated_at in ({placeholders})
            """,
            [latest.source_instance, *workstream_keys, *values],
        ).fetchone()[0]
        or 0
    )


def null_count(conn: sqlite3.Connection, table: str, column: str, source_instance: str | None = None) -> int:
    predicates = [f"{column} is null"]
    params: list[Any] = []
    if source_instance and column_exists(conn, table, "source_instance"):
        predicates.append("source_instance = ?")
        params.append(source_instance)
    return int(conn.execute(f"select count(*) from {table} where {' and '.join(predicates)}", params).fetchone()[0] or 0)


def typed_subject_resolution_rows(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, str, str]]:
    rows: list[tuple[str, str, str]] = []
    for table in ["work_program_items", "work_actions", "work_insights", "work_blockers", "work_item_forecasts"]:
        if not table_exists(conn, table):
            continue
        if all(column_exists(conn, table, column) for column in ["subject_kind", "pull_request_id"]):
            rows.append(
                (
                    f"unresolved_pull_request_subject:{table}",
                    str(subject_pointer_null_count(conn, table, "pull_request", "pull_request_id", source_instance)),
                    "pull_request subjects should resolve to typed PullRequest rows",
                )
            )
        if all(column_exists(conn, table, column) for column in ["subject_kind", "ticket_id"]):
            rows.append(
                (
                    f"unresolved_ticket_subject:{table}",
                    str(subject_pointer_null_count(conn, table, "ticket", "ticket_id", source_instance)),
                    "ticket subjects should resolve to typed Ticket rows",
                )
            )
    return rows


def subject_pointer_null_count(
    conn: sqlite3.Connection,
    table: str,
    subject_kind: str,
    pointer_column: str,
    source_instance: str,
) -> int:
    predicates = ["subject_kind = ?", f"{pointer_column} is null"]
    params: list[Any] = [subject_kind]
    if column_exists(conn, table, "source_instance"):
        predicates.append("source_instance = ?")
        params.append(source_instance)
    return int(conn.execute(f"select count(*) from {table} where {' and '.join(predicates)}", params).fetchone()[0] or 0)


def evidence_need_relationship_rows(conn: sqlite3.Connection, latest: LatestRun) -> list[tuple[str, str, str]]:
    if not table_exists(conn, "work_program_evidence_needs"):
        return []
    required = {"source_instance", "workstream_key", "generated_at"}
    if not required.issubset(table_columns(conn, "work_program_evidence_needs")):
        return []
    generated_values = generated_at_values(latest.generated_at)
    workstream_keys = workstream_sql_keys(latest.workstream_key)
    workstream_placeholders = ",".join("?" for _ in workstream_keys)
    generated_placeholders = ",".join("?" for _ in generated_values)
    base_where = f"""
        source_instance = ?
        and workstream_key in ({workstream_placeholders})
        and generated_at in ({generated_placeholders})
    """
    params: list[Any] = [latest.source_instance, *workstream_keys, *generated_values]
    total = int(
        conn.execute(
            f"select count(*) from work_program_evidence_needs where {base_where}",
            params,
        ).fetchone()[0]
        or 0
    )
    rows = [("work_program_evidence_need_count", str(total), "latest run evidence needs in the operating graph")]
    if column_exists(conn, "work_program_evidence_needs", "quality_gate_id"):
        linked = int(
            conn.execute(
                f"select count(*) from work_program_evidence_needs where {base_where} and quality_gate_id is not null",
                params,
            ).fetchone()[0]
            or 0
        )
        rows.append(("evidence_needs_with_quality_gate_link", str(linked), "evidence needs with a resolved WorkProgramQualityGate edge"))
        rows.append(("evidence_needs_without_quality_gate_link", str(max(0, total - linked)), "evidence needs still relying only on gate_key text"))
        if table_exists(conn, "work_program_quality_gates") and column_exists(conn, "work_program_quality_gates", "id"):
            blocking_without_needs = int(
                conn.execute(
                    f"""
                    select count(distinct wpg.id)
                      from work_program_quality_gates wpg
                      left join work_program_evidence_needs wpen on wpen.quality_gate_id = wpg.id
                     where wpg.source_instance = ?
                       and wpg.workstream_key in ({workstream_placeholders})
                       and wpg.generated_at in ({generated_placeholders})
                       and coalesce(wpg.blocking, 0) = 1
                       and wpen.id is null
                    """,
                    params,
                ).fetchone()[0]
                or 0
            )
            rows.append(("blocking_quality_gates_without_evidence_need", str(blocking_without_needs), "blocking gates should have at least one linked evidence need"))
    if column_exists(conn, "work_program_evidence_needs", "work_action_id"):
        action_linked = int(
            conn.execute(
                f"select count(*) from work_program_evidence_needs where {base_where} and work_action_id is not null",
                params,
            ).fetchone()[0]
            or 0
        )
        rows.append(("evidence_needs_with_work_action_link", str(action_linked), "evidence needs with a resolved WorkAction edge"))
    return rows


def tpm_function_readiness_relationship_rows(conn: sqlite3.Connection, latest: LatestRun) -> list[tuple[str, str, str]]:
    table_name = "work_program_tpm_function_readinesses"
    join_table = "work_program_tpm_function_readiness_blocking_quality_gates"
    if not table_exists(conn, table_name):
        return []
    required = {"id", "source_instance", "workstream_key", "generated_at", "blocking_gate_keys"}
    if not required.issubset(table_columns(conn, table_name)):
        return []
    generated_values = generated_at_values(latest.generated_at)
    workstream_keys = workstream_sql_keys(latest.workstream_key)
    workstream_placeholders = ",".join("?" for _ in workstream_keys)
    generated_placeholders = ",".join("?" for _ in generated_values)
    base_where = f"""
        source_instance = ?
        and workstream_key in ({workstream_placeholders})
        and generated_at in ({generated_placeholders})
    """
    params: list[Any] = [latest.source_instance, *workstream_keys, *generated_values]
    function_rows = conn.execute(
        f"select id, blocking_gate_keys from {table_name} where {base_where}",
        params,
    ).fetchall()
    rows = [
        (
            "work_program_tpm_function_readiness_count",
            str(len(function_rows)),
            "latest run TPM function readiness rows in the operating graph",
        )
    ]
    text_blocked_ids = {
        int(row["id"])
        for row in function_rows
        if split_line_list(row["blocking_gate_keys"])
    }
    rows.append(
        (
            "tpm_function_readiness_with_blocking_gate_keys",
            str(len(text_blocked_ids)),
            "function readiness rows whose compatibility field names at least one blocking gate",
        )
    )
    if not table_exists(conn, join_table):
        if text_blocked_ids:
            rows.append(
                (
                    "tpm_function_readiness_without_blocking_gate_links",
                    str(len(text_blocked_ids)),
                    "blocking function readiness rows still relying only on blocking_gate_keys text",
                )
            )
        return rows
    join_columns = {"work_program_tpm_function_readiness_id", "work_program_quality_gate_id"}
    if not join_columns.issubset(table_columns(conn, join_table)):
        return rows
    if not function_rows:
        rows.append(("tpm_function_readiness_blocking_gate_link_count", "0", "typed readiness-to-quality-gate edges"))
        rows.append(("tpm_function_readiness_without_blocking_gate_links", "0", "blocking function readiness rows without typed gate edges"))
        return rows
    ids = [int(row["id"]) for row in function_rows]
    id_placeholders = ",".join("?" for _ in ids)
    link_rows = conn.execute(
        f"""
        select work_program_tpm_function_readiness_id, count(*) as link_count
          from {join_table}
         where work_program_tpm_function_readiness_id in ({id_placeholders})
         group by work_program_tpm_function_readiness_id
        """,
        ids,
    ).fetchall()
    linked_ids = {int(row["work_program_tpm_function_readiness_id"]) for row in link_rows if int(row["link_count"] or 0) > 0}
    link_count = sum(int(row["link_count"] or 0) for row in link_rows)
    rows.append(("tpm_function_readiness_blocking_gate_link_count", str(link_count), "typed readiness-to-quality-gate edges"))
    rows.append(("tpm_function_readinesses_with_blocking_gate_links", str(len(text_blocked_ids & linked_ids)), "blocking function readiness rows with typed gate edges"))
    rows.append(("tpm_function_readiness_without_blocking_gate_links", str(len(text_blocked_ids - linked_ids)), "blocking function readiness rows still relying only on blocking_gate_keys text"))
    return rows


def action_source_insight_rows(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, str, str]]:
    if not table_exists(conn, "work_actions") or not table_exists(conn, "work_action_source_insights"):
        return []
    source_condition = "1 = 1"
    params: list[str] = []
    if column_exists(conn, "work_actions", "source_instance"):
        source_condition = "wa.source_instance = ?"
        params.append(source_instance)
    without_count = int(
        conn.execute(
            f"""
            select count(distinct wa.id)
              from work_actions wa
              left join work_action_source_insights wasi on wasi.work_action_id = wa.id
             where {source_condition}
               and wasi.work_insight_id is null
            """,
            params,
        ).fetchone()[0]
        or 0
    )
    with_count = int(
        conn.execute(
            f"""
            select count(distinct wa.id)
              from work_actions wa
              join work_action_source_insights wasi on wasi.work_action_id = wa.id
             where {source_condition}
            """,
            params,
        ).fetchone()[0]
        or 0
    )
    forecast_link_count = 0
    unsupported_count = 0
    if table_exists(conn, "work_item_forecasts") and column_exists(conn, "work_item_forecasts", "work_action_id"):
        forecast_link_count = int(
            conn.execute(
                f"""
                select count(distinct wa.id)
                  from work_actions wa
                  join work_item_forecasts wif on wif.work_action_id = wa.id
                 where {source_condition}
                   and wif.latest_evidence_id is not null
                """,
                params,
            ).fetchone()[0]
            or 0
        )
        unsupported_count = int(
            conn.execute(
                f"""
                select count(distinct wa.id)
                  from work_actions wa
                  left join work_action_source_insights wasi on wasi.work_action_id = wa.id
                  left join work_item_forecasts wif
                    on wif.work_action_id = wa.id
                   and wif.latest_evidence_id is not null
                 where {source_condition}
                   and wasi.work_insight_id is null
                   and wif.work_action_id is null
                   and wa.latest_evidence_id is null
                """,
                params,
            ).fetchone()[0]
            or 0
        )
    else:
        unsupported_count = int(
            conn.execute(
                f"""
                select count(distinct wa.id)
                  from work_actions wa
                  left join work_action_source_insights wasi on wasi.work_action_id = wa.id
                 where {source_condition}
                   and wasi.work_insight_id is null
                   and wa.latest_evidence_id is null
                """,
                params,
            ).fetchone()[0]
            or 0
        )
    return [
        ("work_actions_with_source_insights", str(with_count), "actions with a durable generated-insight source link"),
        ("work_actions_without_source_insights", str(without_count), "actions without generated-insight links should still carry forecast or direct evidence paths"),
        ("work_actions_with_forecast_evidence_links", str(forecast_link_count), "actions backed through typed WorkItemForecast evidence"),
        ("work_actions_without_evidence_chain", str(unsupported_count), "actions with no generated insight, forecast evidence link, or direct evidence"),
    ]


def dependency_edge_integrity_rows(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, str, str]]:
    if not table_exists(conn, "work_dependency_edges"):
        return []
    rows = [("work_dependency_edges", str(scoped_count(conn, "work_dependency_edges", source_instance)), "durable operating-topology edges")]
    if column_exists(conn, "work_dependency_edges", "latest_evidence_id"):
        rows.append(("missing_latest_evidence:work_dependency_edges", str(null_count(conn, "work_dependency_edges", "latest_evidence_id", source_instance)), "dependency edges should eventually expose direct evidence or a required indirect evidence path"))
    if (
        table_exists(conn, "evidences")
        and column_exists(conn, "work_dependency_edges", "edge_kind")
        and column_exists(conn, "work_dependency_edges", "latest_evidence_id")
    ):
        rows.extend(dependency_edge_evidence_provenance_rows(conn, source_instance))
    if column_exists(conn, "work_dependency_edges", "relationship_authority"):
        rows.extend(dependency_edge_authority_rows(conn, source_instance))
    context_columns = [
        column
        for column in ["workstream_id", "work_blocker_id", "work_action_id", "ticket_id", "pull_request_id"]
        if column_exists(conn, "work_dependency_edges", column)
    ]
    if context_columns:
        expression = " or ".join(f"{column} is not null" for column in context_columns)
        source_clause = " and source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
        params: list[Any] = [source_instance] if source_clause else []
        context_count = int(
            conn.execute(
                f"select count(*) from work_dependency_edges where ({expression}){source_clause}",
                params,
            ).fetchone()[0]
            or 0
        )
        total_count = scoped_count(conn, "work_dependency_edges", source_instance)
        rows.append(("work_dependency_edges_with_context_id", str(context_count), "edges with at least one typed context pointer"))
        rows.append(("work_dependency_edges_contextless", str(total_count - context_count), "edges that require key-only endpoint hydration"))
        rows.extend(dependency_edge_endpoint_resolution_rows(conn, source_instance))
    rows.extend(dependency_endpoint_integrity_rows(conn, source_instance))
    return rows


def dependency_edge_authority_rows(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, str, str]]:
    rows: list[tuple[str, str, str]] = [
        (
            "invalid_relationship_authority:work_dependency_edges",
            str(invalid_dependency_edge_authority_count(conn, source_instance)),
            "dependency edges should declare canonical_mirror or operating_projection authority",
        ),
        (
            "canonical_mirror_missing_kind:work_dependency_edges",
            str(canonical_mirror_missing_kind_count(conn, source_instance)),
            "canonical mirrors should name the typed relationship row they mirror",
        ),
        (
            "invalid_canonical_relationship_kind:work_dependency_edges",
            str(invalid_canonical_relationship_kind_count(conn, source_instance)),
            "canonical relationship kinds should match the typed relationship enum",
        ),
        (
            "projection_with_canonical_kind:work_dependency_edges",
            str(projection_with_canonical_kind_count(conn, source_instance)),
            "operating projections should not claim a canonical relationship kind",
        ),
    ]
    if column_exists(conn, "work_dependency_edges", "edge_kind"):
        rows.extend(
            [
                (
                    "ticket_pr_not_canonical_mirror:work_dependency_edges",
                    str(ticket_pr_not_canonical_mirror_count(conn, source_instance)),
                    "ticket_pr topology should explicitly mirror TicketPullRequest rather than become a parallel relationship",
                ),
                (
                    "canonical_mirror_missing_typed_row:work_dependency_edges.ticket_pr",
                    str(canonical_ticket_pr_mirror_missing_typed_row_count(conn, source_instance)),
                    "ticket_pr canonical mirrors should join back to a typed ticket_pull_requests row",
                ),
                (
                    "non_ticket_pr_canonical_mirror:work_dependency_edges",
                    str(non_ticket_pr_canonical_mirror_count(conn, source_instance)),
                    "only audited typed relationship mirrors should use canonical_mirror authority",
                ),
            ]
        )
    for authority, count in dependency_edge_authority_counts(conn, source_instance):
        rows.append((f"work_dependency_edge_authority:{authority}", str(count), "dependency edge relationship-authority count"))
    return rows


def invalid_dependency_edge_authority_count(conn: sqlite3.Connection, source_instance: str) -> int:
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges
             where (relationship_authority is null or relationship_authority not in ('canonical_mirror', 'operating_projection'))
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def canonical_mirror_missing_kind_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not column_exists(conn, "work_dependency_edges", "canonical_relationship_kind"):
        return 0
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges
             where relationship_authority = 'canonical_mirror'
               and nullif(canonical_relationship_kind, '') is null
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def invalid_canonical_relationship_kind_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not column_exists(conn, "work_dependency_edges", "canonical_relationship_kind"):
        return 0
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges
             where nullif(canonical_relationship_kind, '') is not null
               and canonical_relationship_kind not in ('ticket_pull_request')
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def projection_with_canonical_kind_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not column_exists(conn, "work_dependency_edges", "canonical_relationship_kind"):
        return 0
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges
             where relationship_authority != 'canonical_mirror'
               and nullif(canonical_relationship_kind, '') is not null
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def ticket_pr_not_canonical_mirror_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not column_exists(conn, "work_dependency_edges", "canonical_relationship_kind"):
        return 0
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges
             where edge_kind = 'ticket_pr'
               and (
                 relationship_authority != 'canonical_mirror'
                 or canonical_relationship_kind is null
                 or canonical_relationship_kind != 'ticket_pull_request'
               )
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def non_ticket_pr_canonical_mirror_count(conn: sqlite3.Connection, source_instance: str) -> int:
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges
             where edge_kind != 'ticket_pr'
               and relationship_authority = 'canonical_mirror'
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def canonical_ticket_pr_mirror_missing_typed_row_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not table_exists(conn, "ticket_pull_requests"):
        return 0
    required_columns = ["relationship_authority", "edge_kind", "ticket_id", "pull_request_id"]
    if not all(column_exists(conn, "work_dependency_edges", column) for column in required_columns):
        return 0
    if not all(column_exists(conn, "ticket_pull_requests", column) for column in ["ticket_id", "pull_request_id"]):
        return 0
    source_clause = "and wde.source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges wde
              left join ticket_pull_requests tpr
                on tpr.ticket_id = wde.ticket_id
               and tpr.pull_request_id = wde.pull_request_id
             where wde.edge_kind = 'ticket_pr'
               and wde.relationship_authority = 'canonical_mirror'
               and tpr.ticket_id is null
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def dependency_edge_authority_counts(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, int]]:
    source_clause = "where source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    rows = conn.execute(
        f"""
        select coalesce(relationship_authority, 'missing') as relationship_authority, count(*)
          from work_dependency_edges
          {source_clause}
         group by coalesce(relationship_authority, 'missing')
         order by relationship_authority
        """,
        params,
    ).fetchall()
    return [(str(authority), int(count or 0)) for authority, count in rows]


def dependency_endpoint_integrity_rows(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, str, str]]:
    if not table_exists(conn, "work_dependency_endpoints"):
        return [("work_dependency_endpoints", "missing", "first-class dependency endpoint table")]
    rows: list[tuple[str, str, str]] = []
    endpoint_count = scoped_count(conn, "work_dependency_endpoints", source_instance)
    edge_count = scoped_count(conn, "work_dependency_edges", source_instance) if table_exists(conn, "work_dependency_edges") else 0
    expected_count = edge_count * 2
    rows.append(("work_dependency_endpoints", str(endpoint_count), "first-class from/to endpoint rows for dependency topology"))
    rows.append(("work_dependency_endpoint_expected_count", str(expected_count), "two endpoint rows should exist per dependency edge"))
    rows.append(("work_dependency_endpoint_count_delta", str(endpoint_count - expected_count), "actual endpoint row count minus expected endpoint row count"))

    if all(column_exists(conn, "work_dependency_endpoints", column) for column in ["work_dependency_edge_id", "source_instance"]):
        rows.append(("orphaned_work_dependency_endpoints", str(orphaned_dependency_endpoint_count(conn, source_instance)), "endpoint rows whose dependency edge no longer exists"))
    if column_exists(conn, "work_dependency_endpoints", "endpoint_role"):
        rows.append(("invalid_endpoint_role:work_dependency_endpoints", str(invalid_dependency_endpoint_role_count(conn, source_instance)), "dependency endpoint role should be from or to"))
    if column_exists(conn, "work_dependency_endpoints", "resolution_state"):
        rows.append(("invalid_node_kind:work_dependency_endpoints", str(invalid_dependency_endpoint_node_kind_count(conn, source_instance)), "dependency endpoint node kind should be a known topology node kind"))
        rows.append(("invalid_resolution_state:work_dependency_endpoints", str(invalid_dependency_endpoint_resolution_state_count(conn, source_instance)), "dependency endpoint resolution state should be resolved, key_only, or missing"))
        rows.append(("invalid_key_only_endpoint:work_dependency_endpoints", str(invalid_dependency_endpoint_key_only_count(conn, source_instance)), "only component endpoints should be key_only, and component endpoints should be key_only"))
        rows.append(("invalid_resolved_pointer_shape:work_dependency_endpoints", str(invalid_dependency_endpoint_resolved_pointer_shape_count(conn, source_instance)), "resolved endpoints should have exactly one typed pointer matching node_kind"))
        for state, count in dependency_endpoint_resolution_counts(conn, source_instance):
            rows.append((f"work_dependency_endpoint_resolution:{state}", str(count), "endpoint resolution-state count"))
        rows.append(("missing_typed_target:work_dependency_endpoints", str(dependency_endpoint_missing_typed_target_count(conn, source_instance)), "endpoint rows that should resolve to a typed target but do not"))
    return rows


def orphaned_dependency_endpoint_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not table_exists(conn, "work_dependency_edges"):
        return 0
    return int(
        conn.execute(
            """
            select count(*)
              from work_dependency_endpoints endpoint
              left join work_dependency_edges edge on edge.id = endpoint.work_dependency_edge_id
             where endpoint.source_instance = ?
               and edge.id is null
            """,
            [source_instance],
        ).fetchone()[0]
        or 0
    )


def invalid_dependency_endpoint_role_count(conn: sqlite3.Connection, source_instance: str) -> int:
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_endpoints", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_endpoints
             where endpoint_role not in ('from', 'to')
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def invalid_dependency_endpoint_node_kind_count(conn: sqlite3.Connection, source_instance: str) -> int:
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_endpoints", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_endpoints
             where node_kind not in ('workstream', 'ticket', 'pull_request', 'blocker', 'action', 'component')
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def invalid_dependency_endpoint_resolution_state_count(conn: sqlite3.Connection, source_instance: str) -> int:
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_endpoints", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_endpoints
             where resolution_state not in ('resolved', 'key_only', 'missing')
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def invalid_dependency_endpoint_key_only_count(conn: sqlite3.Connection, source_instance: str) -> int:
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_endpoints", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_endpoints
             where (
                 (node_kind = 'component' and resolution_state != 'key_only')
                 or (node_kind != 'component' and resolution_state = 'key_only')
               )
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def invalid_dependency_endpoint_resolved_pointer_shape_count(conn: sqlite3.Connection, source_instance: str) -> int:
    required_columns = ["node_kind", "resolution_state", "workstream_id", "work_blocker_id", "work_action_id", "ticket_id", "pull_request_id"]
    if not all(column_exists(conn, "work_dependency_endpoints", column) for column in required_columns):
        return 0
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_endpoints", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_endpoints
             where resolution_state = 'resolved'
               and not (
                 (node_kind = 'ticket' and ticket_id is not null and pull_request_id is null and workstream_id is null and work_blocker_id is null and work_action_id is null)
                 or (node_kind = 'pull_request' and pull_request_id is not null and ticket_id is null and workstream_id is null and work_blocker_id is null and work_action_id is null)
                 or (node_kind = 'workstream' and workstream_id is not null and ticket_id is null and pull_request_id is null and work_blocker_id is null and work_action_id is null)
                 or (node_kind = 'blocker' and work_blocker_id is not null and ticket_id is null and pull_request_id is null and workstream_id is null and work_action_id is null)
                 or (node_kind = 'action' and work_action_id is not null and ticket_id is null and pull_request_id is null and workstream_id is null and work_blocker_id is null)
               )
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def dependency_endpoint_resolution_counts(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, int]]:
    source_clause = "where source_instance = ?" if column_exists(conn, "work_dependency_endpoints", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    rows = conn.execute(
        f"""
        select coalesce(resolution_state, 'missing') as resolution_state, count(*)
          from work_dependency_endpoints
          {source_clause}
         group by coalesce(resolution_state, 'missing')
         order by resolution_state
        """,
        params,
    ).fetchall()
    return [(str(state), int(count or 0)) for state, count in rows]


def dependency_endpoint_missing_typed_target_count(conn: sqlite3.Connection, source_instance: str) -> int:
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_endpoints", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_endpoints
             where resolution_state = 'missing'
               and node_kind != 'component'
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def dependency_edge_endpoint_resolution_rows(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, str, str]]:
    required_columns = ["edge_kind", "from_kind", "to_kind", "ticket_id", "pull_request_id", "workstream_id", "work_blocker_id", "work_action_id"]
    if not all(column_exists(conn, "work_dependency_edges", column) for column in required_columns):
        return []
    rows = [
        (
            "invalid_endpoint_shape:work_dependency_edges.ticket_pr",
            str(dependency_edge_shape_mismatch_count(conn, source_instance, "ticket_pr", "ticket", "pull_request")),
            "ticket-to-PR topology should be shaped as ticket -> pull_request",
        ),
        (
            "invalid_endpoint_shape:work_dependency_edges.blocked_by",
            str(dependency_edge_shape_mismatch_count(conn, source_instance, "blocked_by", {"ticket", "pull_request"}, "blocker")),
            "blocked-by topology should be shaped as ticket/pull_request -> blocker",
        ),
        (
            "invalid_endpoint_shape:work_dependency_edges.needs_action",
            str(dependency_edge_shape_mismatch_count(conn, source_instance, "needs_action", "blocker", "action")),
            "needs-action topology should be shaped as blocker -> action",
        ),
        (
            "invalid_endpoint_shape:work_dependency_edges.workstream_cluster",
            str(dependency_edge_shape_mismatch_count(conn, source_instance, "workstream_cluster", "component", {"ticket", "pull_request"})),
            "workstream cluster topology should be shaped as component -> ticket/pull_request",
        ),
        (
            "unresolved_ticket_endpoint:work_dependency_edges.ticket_pr",
            str(dependency_edge_null_or_orphan_count(conn, source_instance, "ticket_pr", "ticket_id", "tickets")),
            "ticket-to-PR topology should resolve the Ticket endpoint to a typed Ticket row",
        ),
        (
            "unresolved_pull_request_endpoint:work_dependency_edges.ticket_pr",
            str(dependency_edge_null_or_orphan_count(conn, source_instance, "ticket_pr", "pull_request_id", "pull_requests")),
            "ticket-to-PR topology should resolve the PullRequest endpoint to a typed PullRequest row",
        ),
        (
            "unresolved_subject_endpoint:work_dependency_edges.blocked_by",
            str(blocked_by_subject_endpoint_unresolved_count(conn, source_instance)),
            "blocked-by topology should resolve pull_request or ticket subjects to typed work item rows",
        ),
        (
            "unresolved_blocker_endpoint:work_dependency_edges.blocked_by",
            str(dependency_edge_null_or_orphan_count(conn, source_instance, "blocked_by", "work_blocker_id", "work_blockers")),
            "blocked-by topology should resolve the blocker endpoint to a typed WorkBlocker row",
        ),
        (
            "unresolved_blocker_endpoint:work_dependency_edges.needs_action",
            str(dependency_edge_null_or_orphan_count(conn, source_instance, "needs_action", "work_blocker_id", "work_blockers")),
            "needs-action topology should resolve the blocker endpoint to a typed WorkBlocker row",
        ),
        (
            "unresolved_action_endpoint:work_dependency_edges.needs_action",
            str(dependency_edge_null_or_orphan_count(conn, source_instance, "needs_action", "work_action_id", "work_actions")),
            "needs-action topology should resolve the action endpoint to a typed WorkAction row",
        ),
        (
            "unresolved_workstream_context:work_dependency_edges.workstream_cluster",
            str(dependency_edge_null_or_orphan_count(conn, source_instance, "workstream_cluster", "workstream_id", "workstreams")),
            "workstream cluster topology should resolve the Workstream context",
        ),
        (
            "unresolved_target_endpoint:work_dependency_edges.workstream_cluster",
            str(workstream_cluster_target_endpoint_unresolved_count(conn, source_instance)),
            "workstream cluster topology should resolve ticket or pull_request targets to typed work item rows",
        ),
    ]
    return rows


def dependency_edge_shape_mismatch_count(
    conn: sqlite3.Connection,
    source_instance: str,
    edge_kind: str,
    expected_from: str | set[str],
    expected_to: str | set[str],
) -> int:
    source_clause = "and source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [edge_kind]
    if source_clause:
        params.append(source_instance)
    from_values = {expected_from} if isinstance(expected_from, str) else set(expected_from)
    to_values = {expected_to} if isinstance(expected_to, str) else set(expected_to)
    from_placeholders = ",".join("?" for _ in from_values)
    to_placeholders = ",".join("?" for _ in to_values)
    params.extend(sorted(from_values))
    params.extend(sorted(to_values))
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges
             where edge_kind = ?
               {source_clause}
               and (from_kind not in ({from_placeholders}) or to_kind not in ({to_placeholders}))
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def dependency_edge_null_or_orphan_count(
    conn: sqlite3.Connection,
    source_instance: str,
    edge_kind: str,
    pointer_column: str,
    target_table: str,
) -> int:
    source_clause = "and wde.source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [edge_kind]
    if source_clause:
        params.append(source_instance)
    orphan_clause = ""
    join_clause = ""
    if table_exists(conn, target_table) and column_exists(conn, target_table, "id"):
        join_clause = f"left join {target_table} target on target.id = wde.{pointer_column}"
        orphan_clause = " or target.id is null"
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges wde
              {join_clause}
             where wde.edge_kind = ?
               and (wde.{pointer_column} is null{orphan_clause})
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def blocked_by_subject_endpoint_unresolved_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not all(column_exists(conn, "work_dependency_edges", column) for column in ["from_kind", "ticket_id", "pull_request_id"]):
        return 0
    if not all(table_exists(conn, table) and column_exists(conn, table, "id") for table in ["tickets", "pull_requests"]):
        return 0
    source_clause = "and wde.source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges wde
              left join tickets t on t.id = wde.ticket_id
              left join pull_requests pr on pr.id = wde.pull_request_id
             where wde.edge_kind = 'blocked_by'
               {source_clause}
               and (
                 (wde.from_kind = 'ticket' and (wde.ticket_id is null or t.id is null))
                 or (wde.from_kind = 'pull_request' and (wde.pull_request_id is null or pr.id is null))
               )
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def workstream_cluster_target_endpoint_unresolved_count(conn: sqlite3.Connection, source_instance: str) -> int:
    if not all(column_exists(conn, "work_dependency_edges", column) for column in ["to_kind", "ticket_id", "pull_request_id"]):
        return 0
    if not all(table_exists(conn, table) and column_exists(conn, table, "id") for table in ["tickets", "pull_requests"]):
        return 0
    source_clause = "and wde.source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    return int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges wde
              left join tickets t on t.id = wde.ticket_id
              left join pull_requests pr on pr.id = wde.pull_request_id
             where wde.edge_kind = 'workstream_cluster'
               {source_clause}
               and (
                 (wde.to_kind = 'ticket' and (wde.ticket_id is null or t.id is null))
                 or (wde.to_kind = 'pull_request' and (wde.pull_request_id is null or pr.id is null))
               )
            """,
            params,
        ).fetchone()[0]
        or 0
    )


def dependency_edge_evidence_provenance_rows(conn: sqlite3.Connection, source_instance: str) -> list[tuple[str, str, str]]:
    source_clause = "and wde.source_instance = ?" if column_exists(conn, "work_dependency_edges", "source_instance") else ""
    params: list[Any] = [source_instance] if source_clause else []
    relationship_count = int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges wde
              join evidences e on e.id = wde.latest_evidence_id
             where wde.edge_kind = 'ticket_pr'
               and e.claim_kind = 'relationship'
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )
    generated_ticket_pr_count = int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges wde
              join evidences e on e.id = wde.latest_evidence_id
             where wde.edge_kind = 'ticket_pr'
               and e.external_kind = 'tpm_generated_evidence'
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )
    generated_cluster_count = int(
        conn.execute(
            f"""
            select count(*)
              from work_dependency_edges wde
              join evidences e on e.id = wde.latest_evidence_id
             where wde.edge_kind = 'workstream_cluster'
               and e.external_kind = 'tpm_generated_evidence'
               {source_clause}
            """,
            params,
        ).fetchone()[0]
        or 0
    )
    return [
        ("relationship_evidence:work_dependency_edges.ticket_pr", str(relationship_count), "ticket-to-PR topology should inherit first-order relationship evidence"),
        ("generated_evidence:work_dependency_edges.ticket_pr", str(generated_ticket_pr_count), "ticket-to-PR topology should not fall back to generated evidence when relationship proof exists"),
        ("generated_evidence:work_dependency_edges.workstream_cluster", str(generated_cluster_count), "derived cluster topology is expected to use generated evidence"),
    ]


def source_sync_issue_counts(conn: sqlite3.Connection, source_instance: str) -> tuple[int, int, int]:
    source_table = "current_source_sync_issues" if table_exists(conn, "current_source_sync_issues") else "source_sync_issues"
    if not table_exists(conn, source_table):
        return 0, 0, 0
    total = table_count(conn, source_table)
    if column_exists(conn, source_table, "source_instance"):
        scoped = int(conn.execute(f"select count(*) from {source_table} where source_instance = ?", [source_instance]).fetchone()[0] or 0)
    else:
        scoped = 0
    source_scope_scoped = 0
    if column_exists(conn, source_table, "source_scope_id") and table_exists(conn, "source_scopes"):
        source_scope_scoped = int(
            conn.execute(
                f"""
                select count(*)
                  from {source_table} ssi
                  join source_scopes ss on ss.id = ssi.source_scope_id
                 where ss.scope_key = ?
                """,
                [source_instance],
            ).fetchone()[0]
            or 0
        )
    return total, scoped, source_scope_scoped


def metric_table(conn: sqlite3.Connection, table: str) -> dict[str, dict[str, str]]:
    if not table_exists(conn, table):
        return {}
    rows = conn.execute(f"select metric, value, note from {table}").fetchall()
    return {str(row["metric"]): {"value": str(row["value"]), "note": str(row["note"] or "")} for row in rows}


def forecast_backtest_rows(conn: sqlite3.Connection) -> list[dict[str, str]]:
    if not table_exists(conn, "tpm_forecast_backtest"):
        return []
    rows = conn.execute(
        """
        select evaluation, model, mae_days, ready_for_eta, note
          from tpm_forecast_backtest
         order by evaluation, mae_days
        """
    ).fetchall()
    return [{key: row[key] for key in row.keys()} for row in rows]


def forecast_reliability_rows(conn: sqlite3.Connection) -> list[dict[str, str]]:
    if not table_exists(conn, "tpm_forecast_reliability"):
        return []
    required = [
        "forecast_product",
        "readiness_state",
        "product_safe",
        "safe_use",
        "primary_metric",
        "metric_value",
        "next_evidence",
        "guardrail",
    ]
    columns = table_columns(conn, "tpm_forecast_reliability")
    if any(column not in columns for column in required):
        return []
    rows = conn.execute(
        """
        select forecast_product, readiness_state, product_safe, safe_use,
               primary_metric, metric_value, next_evidence, guardrail
          from tpm_forecast_reliability
         order by case forecast_product
                    when 'point_eta' then 0
                    when 'range_eta' then 1
                    when 'risk_triage' then 2
                    else 3
                  end,
                  forecast_product
        """
    ).fetchall()
    return [{key: row[key] for key in row.keys()} for row in rows]


def decision_target_backtest_rows(conn: sqlite3.Connection) -> list[dict[str, str]]:
    if not table_exists(conn, "tpm_decision_target_backtest"):
        return []
    columns = table_columns(conn, "tpm_decision_target_backtest")
    coverage_expr = "coverage_stratum" if "coverage_stratum" in columns else "'' as coverage_stratum"
    order_suffix = ", fold" if "fold" in table_columns(conn, "tpm_decision_target_backtest") else ""
    rows = conn.execute(
        f"""
        select target_kind, evaluation, model, {coverage_expr}, precision_at_10pct, lift_at_10pct,
               ready_for_product_action, note
          from tpm_decision_target_backtest
         order by target_kind,
                  case
                    when evaluation = 'source_event_as_of_coverage_stratified_summary' then 0
                    when evaluation = 'source_event_as_of_coverage_stratum' then 1
                    else 2
                  end,
                  evaluation, coverage_stratum, model{order_suffix}
        """
    ).fetchall()
    return [{key: row[key] for key in row.keys()} for row in rows]


def decision_target_evaluation_rows(conn: sqlite3.Connection, latest: LatestRun) -> list[dict[str, str]]:
    if not table_exists(conn, "work_decision_target_evaluations"):
        return []
    columns = table_columns(conn, "work_decision_target_evaluations")
    fold_order = ", fold" if "fold" in columns else ""
    rows = conn.execute(
        """
        select target_kind, evaluation_kind, model_name, coalesce(coverage_stratum, '') as coverage_stratum,
               precision_at_10pct, lift_at_10pct, ready_for_product_action,
               product_action_gate_state, note
          from work_decision_target_evaluations
         where source_instance = ?
           and evaluated_at = ?
         order by target_kind,
                  case
                    when evaluation_kind = 'source_event_as_of_coverage_stratified_summary' then 0
                    when evaluation_kind = 'source_event_as_of_coverage_stratum' then 1
                    else 2
                  end,
                  evaluation_kind, coverage_stratum, model_name""" + fold_order + """
        """,
        (latest.source_instance, latest.generated_at),
    ).fetchall()
    return [{key: row[key] for key in row.keys()} for row in rows]


def forecast_metric_rows(summary: dict[str, dict[str, str]], rollup: list[tuple[str, str]]) -> list[tuple[str, str, str]]:
    preferred = [
        "forecast_method",
        "eta_forecast_ready",
        "eta_readiness_state",
        "eta_model_backtest_ready",
        "eta_best_candidate_model",
        "eta_primary_blocker",
        "eta_blocker_count",
        "eta_kfold_best_candidate_improvement_pct",
        "eta_chronological_best_candidate_improvement_pct",
        "eta_kfold_random_forest_improvement_pct",
        "eta_chronological_random_forest_improvement_pct",
        "eta_temporal_snapshot_state",
        "eta_next_evidence_needed",
        "forecast_feature_leakage_guard",
        "merged_pr_count",
        "open_pr_count",
        "median_merged_cycle_days",
        "p75_merged_cycle_days",
        "backtest_best_model",
        "backtest_median_mae_days",
        "backtest_gradient_boosting_mae_days",
        "backtest_random_forest_mae_days",
        "backtest_heuristic_mae_days",
        "lifecycle_as_of_backtest_state",
        "lifecycle_as_of_terminal_subject_count",
        "lifecycle_as_of_training_example_count",
        "lifecycle_as_of_best_model",
        "lifecycle_as_of_best_mae_days",
        "lifecycle_as_of_age_bucket_mae_days",
        "survival_time_to_merge_state",
        "survival_time_to_merge_subject_count",
        "survival_time_to_merge_event_subject_count",
        "survival_time_to_merge_censored_subject_count",
        "survival_time_to_merge_open_censored_subject_count",
        "survival_time_to_merge_censoring_rate",
        "survival_time_to_merge_backtest_example_count",
        "survival_time_to_merge_best_model",
        "survival_time_to_merge_best_mae_days",
        "risk_triage_precision_at_10pct",
        "risk_triage_lift_at_10pct",
        "risk_triage_coverage_stratified_state",
        "risk_triage_coverage_stratum_count",
        "risk_triage_coverage_stratified_max_lift_at_10pct",
        "risk_triage_coverage_stratified_weighted_lift_at_10pct",
    ]
    rows: list[tuple[str, str, str]] = []
    for metric in preferred:
        if metric in summary:
            rows.append((metric, summary[metric]["value"], summary[metric]["note"]))
    for metric, value in rollup:
        rows.append((metric, value, "persisted ontology forecast row count"))
    if not rows:
        rows.append(("forecast_data", "missing", "no forecast summary or forecast rows found"))
    return rows


def ai_tpm_verdict(
    latest: LatestRun,
    gates: list[sqlite3.Row],
    adversarial: list[sqlite3.Row],
    source_rollup: list[tuple[str, str]],
    forecast_summary: dict[str, dict[str, str]],
    evidence_needs: list[sqlite3.Row],
) -> str:
    gate_by_key = {str(row["gate_key"]): row for row in gates}
    source_metrics = dict(source_rollup)
    source_blocked = as_bool(row_value(gate_by_key.get("source_coverage"), "blocking"))
    source_blocked = source_blocked or int(source_metrics.get("source_sync_issue_count", "0") or 0) > 0
    source_blocked = source_blocked or int(source_metrics.get("source_coverage_evidence_need_count", "0") or 0) > 0
    failed_checks = [row for row in adversarial if str(row["check_state"]).lower() not in PASS_STATES]
    eta_ready = (forecast_summary.get("eta_forecast_ready", {}).get("value", "").lower() == "true")
    measurement_blocked = any(as_bool(row["blocking"]) for row in gates if str(row["gate_key"]).startswith("measurement_"))
    owner_blocked = as_bool(row_value(gate_by_key.get("owner_load"), "blocking"))
    forecast_blocked = as_bool(row_value(gate_by_key.get("forecast_readiness"), "blocking")) or not eta_ready

    if source_blocked:
        return "operating_brief_ready_but_source_coverage_blocked"
    if measurement_blocked:
        return "operating_brief_ready_but_measurement_blocked"
    if forecast_blocked:
        return "risk_triage_ready_but_eta_gated"
    if owner_blocked:
        return "operating_brief_ready_but_owner_capacity_blocked"
    if failed_checks or evidence_needs or latest.blocking_gate_count:
        return "supervised_ai_tpm_only"
    if latest.autonomous_action_ready and not latest.human_review_required:
        return "autonomous_candidate"
    return "operating_brief_ready"


def verdict_explanation(
    verdict: str,
    latest: LatestRun,
    gates: list[sqlite3.Row],
    source_rollup: list[tuple[str, str]],
    pr_source_coverage: list[tuple[str, str]],
    forecast_summary: dict[str, dict[str, str]],
    adversarial: list[sqlite3.Row],
) -> list[str]:
    source_metrics = dict(source_rollup)
    pr_source_metrics = dict(pr_source_coverage)
    eta_ready = forecast_summary.get("eta_forecast_ready", {}).get("value", "unknown")
    failed_or_warning = sum(1 for row in adversarial if str(row["check_state"]).lower() not in PASS_STATES)
    pr_detail_failed = pr_source_metrics.get("pr_source_coverage:detail_failed / failed", "0")
    pr_observed = pr_source_metrics.get("pr_source_coverage:observed / observed", "0")
    return [
        "## Executive Verdict",
        "",
        f"`{verdict}` means Cubicle has a persisted operating brief and triage path, but the current run is `{latest.readiness_state}` with `{latest.blocking_gate_count}` blocking gate(s).",
        "",
        f"- Source coverage: {source_metrics.get('source_coverage_gate_state', 'missing')}; {source_metrics.get('source_sync_issue_count', '0')} source sync issue(s), {source_metrics.get('limited_program_item_count', '0')} limited program item(s), {pr_detail_failed} PR detail-failed row(s), {pr_observed} observed PR row(s).",
        f"- Forecasting: ETA readiness is `{eta_ready}`; forecasts should be used for risk ranking unless this flips to true and gates clear.",
        f"- Adversarial checks: {failed_or_warning} non-passing check(s).",
        f"- Quality gates persisted: {len(gates)}.",
    ]


def table_counts(conn: sqlite3.Connection, tables: Iterable[str]) -> dict[str, int]:
    return {table: table_count(conn, table) for table in tables if table_exists(conn, table)}


def table_count(conn: sqlite3.Connection, table: str) -> int:
    return int(conn.execute(f"select count(*) from {table}").fetchone()[0] or 0)


def scoped_count(conn: sqlite3.Connection, table: str, source_instance: str) -> int:
    if not table_exists(conn, table):
        return 0
    if column_exists(conn, table, "source_instance"):
        return int(conn.execute(f"select count(*) from {table} where source_instance = ?", [source_instance]).fetchone()[0] or 0)
    return table_count(conn, table)


def grouped_counts(
    conn: sqlite3.Connection,
    table: str,
    columns: list[str],
    *,
    source_instance: str | None = None,
) -> list[tuple[str, int]]:
    if not table_exists(conn, table):
        return []
    available = [column for column in columns if column_exists(conn, table, column)]
    if not available:
        return []
    select_cols = ", ".join(f"coalesce({column}, '') as {column}" for column in available)
    group_cols = ", ".join(available)
    predicates: list[str] = []
    params: list[Any] = []
    if source_instance and column_exists(conn, table, "source_instance"):
        predicates.append("source_instance = ?")
        params.append(source_instance)
    where = f"where {' and '.join(predicates)}" if predicates else ""
    rows = conn.execute(
        f"""
        select {select_cols}, count(*) as count
          from {table}
          {where}
         group by {group_cols}
         order by count desc, {group_cols}
        """,
        params,
    ).fetchall()
    out: list[tuple[str, int]] = []
    for row in rows:
        key = " / ".join(str(row[column]) for column in available)
        out.append((key, int(row["count"] or 0)))
    return out


def table_exists(conn: sqlite3.Connection, table: str) -> bool:
    return conn.execute(
        "select 1 from sqlite_master where type in ('table', 'view') and name = ?",
        [table],
    ).fetchone() is not None


def column_exists(conn: sqlite3.Connection, table: str, column: str) -> bool:
    if not table_exists(conn, table):
        return False
    return any(row[1] == column for row in conn.execute(f"pragma table_info({table})"))


def table_columns(conn: sqlite3.Connection, table: str) -> set[str]:
    if not table_exists(conn, table):
        return set()
    return {str(row[1]) for row in conn.execute(f"pragma table_info({table})")}


def generated_at_values(value: str) -> list[str]:
    raw = str(value or "").strip()
    if not raw:
        return [raw]
    values = {raw}
    parsed = parse_timestamp(raw)
    if parsed is not None:
        utc = parsed.astimezone(timezone.utc)
        iso_micro = utc.isoformat(timespec="microseconds")
        iso_seconds = utc.isoformat(timespec="seconds")
        values.add(iso_micro)
        values.add(iso_micro.replace("+00:00", "Z"))
        values.add(iso_seconds)
        values.add(iso_seconds.replace("+00:00", "Z"))
        values.add(utc.strftime("%Y-%m-%d %H:%M:%S.%f+00:00"))
        values.add(utc.strftime("%Y-%m-%d %H:%M:%S+00:00"))
    if raw.endswith("Z"):
        values.add(raw[:-1] + "+00:00")
    if raw.endswith("+00:00"):
        values.add(raw[:-6] + "Z")
    return sorted(values)


def parse_timestamp(value: str) -> datetime | None:
    text = value.strip()
    if not text:
        return None
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        parsed = datetime.fromisoformat(text)
    except ValueError:
        return None
    if parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=timezone.utc)
    return parsed


def row_value(row: sqlite3.Row | None, key: str) -> Any:
    if row is None:
        return None
    if key not in row.keys():
        return None
    return row[key]


def coverage_state_limited(state: str) -> bool:
    text = str(state or "").strip().lower()
    if not text:
        return True
    if text == "not_observed":
        return True
    if text.startswith(LIMITED_COVERAGE_PREFIXES):
        return True
    return any(token in text for token in ["unavailable", "missing", "partial", "failed", "failure", "repair"])


def group_rows_table(rows: list[sqlite3.Row], columns: list[str]) -> list[str]:
    if not rows:
        return ["No rows."]
    counts: dict[tuple[str, ...], int] = {}
    for row in rows:
        key = tuple(str(row[column] or "") for column in columns if column in row.keys())
        counts[key] = counts.get(key, 0) + 1
    table_rows = [(*key, count) for key, count in sorted(counts.items(), key=lambda item: (-item[1], item[0]))]
    return markdown_table([*columns, "count"], table_rows)


def markdown_table(headers: list[str], rows: Iterable[Iterable[Any]]) -> list[str]:
    rendered_rows = [[cell_text(cell) for cell in row] for row in rows]
    out = [
        "| " + " | ".join(headers) + " |",
        "| " + " | ".join("---" for _ in headers) + " |",
    ]
    for row in rendered_rows:
        padded = row[: len(headers)] + [""] * max(0, len(headers) - len(row))
        out.append("| " + " | ".join(padded) + " |")
    return out


def decision_target_report_rows(
    rows: list[dict[str, Any]],
    *,
    limit: int = 8,
    evaluation_key: str,
    model_key: str,
) -> list[dict[str, Any]]:
    if len(rows) <= limit:
        return rows
    selected: list[tuple[int, dict[str, Any]]] = []
    selected_indexes: set[int] = set()
    for index, row in enumerate(rows):
        evaluation = one_line(row.get(evaluation_key, ""))
        model = one_line(row.get(model_key, ""))
        coverage_stratum = one_line(row.get("coverage_stratum", ""))
        if (
            evaluation == "source_event_as_of_coverage_stratified_summary"
            or model == "coverage_guardrail"
            or coverage_stratum.startswith("not_testable")
            or coverage_stratum.startswith("insufficient")
        ):
            selected.append((index, row))
            selected_indexes.add(index)
            if len(selected) >= limit:
                return [item for _, item in selected]
    for index, row in enumerate(rows):
        if index in selected_indexes:
            continue
        selected.append((index, row))
        if len(selected) >= limit:
            break
    return [item for _, item in sorted(selected, key=lambda item: item[0])]


def cell_text(value: Any) -> str:
    return one_line(str(value if value is not None else "")).replace("|", "\\|")


def one_line(value: Any) -> str:
    return " ".join(str(value if value is not None else "").split())


def split_line_list(value: Any) -> list[str]:
    if value is None:
        return []
    out: list[str] = []
    seen: set[str] = set()
    for part in str(value).replace(",", "\n").splitlines():
        text = one_line(part)
        if not text or text in seen:
            continue
        seen.add(text)
        out.append(text)
    return out


def format_float(value: Any) -> str:
    if value is None or value == "":
        return ""
    try:
        number = float(value)
    except (TypeError, ValueError):
        return str(value)
    return f"{number:.2f}".rstrip("0").rstrip(".")


def bool_text(value: bool) -> str:
    return "true" if value else "false"


def as_bool(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    text = str(value or "").strip().lower()
    return text in {"1", "true", "yes", "y", "on"}


if __name__ == "__main__":
    main()
