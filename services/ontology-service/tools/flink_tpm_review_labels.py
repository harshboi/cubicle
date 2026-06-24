#!/usr/bin/env python3
"""Export/import AI-TPM review labels and compute evaluation metrics.

Generated WorkInsight rows and seeded triage requests remain producer/system
state. This tool writes separate WorkInsightReview rows with
review_kind='evaluation_label' so model quality can be measured without
overwriting generated facts.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import math
import sqlite3
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import pandas as pd


TRUTH_LABELS = {"unknown", "true_positive", "false_positive", "partial"}
ACTIONABILITY_LABELS = {"unknown", "actionable", "not_actionable", "needs_owner"}
REVIEW_STATES = {"requested", "needs_more_data", "accepted", "dismissed", "resolved"}
LABEL_QUALITIES = {"auto", "adversarial", "candidate", "gold", "smoke"}
MEASUREMENT_LABEL_QUALITIES = {"gold"}
POSITIVE_ACTIONABILITY = {"actionable", "needs_owner"}
TRUTH_LABEL_OPTIONS = "unknown|true_positive|partial|false_positive"
ACTIONABILITY_LABEL_OPTIONS = "unknown|actionable|needs_owner|not_actionable"
REVIEW_STATE_OPTIONS = "requested|needs_more_data|accepted|dismissed|resolved"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ontology-db", required=True, type=Path)
    parser.add_argument("--analytics-db", type=Path, default=None)
    parser.add_argument("--source-instance", default="")
    parser.add_argument("--export-template", type=Path, default=None)
    parser.add_argument("--import-labels", type=Path, default=None)
    parser.add_argument("--label-set", default="manual_eval")
    parser.add_argument("--label-quality", choices=sorted(LABEL_QUALITIES), default="auto")
    parser.add_argument("--measurement-label-set", action="append", default=[])
    parser.add_argument("--export-measurement-queue", type=Path, default=None)
    parser.add_argument("--measurement-queue-report", type=Path, default=None)
    parser.add_argument("--measurement-queue-size", type=int, default=30)
    parser.add_argument("--measurement-insight-kind", action="append", default=[])
    parser.add_argument("--reviewer-key", default="")
    parser.add_argument("--reviewed-at", default=datetime.now(timezone.utc).isoformat())
    parser.add_argument("--report", type=Path, default=None)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    reviewed_dt = parse_dt(args.reviewed_at) or datetime.now(timezone.utc)
    with sqlite3.connect(args.ontology_db) as conn:
        source_instance = args.source_instance or infer_source_instance(conn)
        template = build_label_template(conn, source_instance)
        if args.export_template is not None:
            write_table_file(template, args.export_template)
        imported_count = 0
        if args.import_labels is not None:
            labels = read_table_file(args.import_labels)
            label_quality = resolve_label_quality(args.label_quality, args.label_set, args.import_labels, args.reviewer_key)
            imported_count = import_labels(
                conn,
                labels,
                source_instance,
                args.label_set,
                label_quality,
                set(args.measurement_label_set or []),
                args.reviewer_key or args.label_set,
                reviewed_dt,
                args.import_labels,
            )
        backfill_measurement_eligibility(conn, source_instance, set(args.measurement_label_set or []))
        label_rows = read_evaluation_labels(conn, source_instance, set(args.measurement_label_set or []))
        metrics = build_metrics(measurement_label_rows(label_rows))
        report_rows = build_report_rows(label_rows)

    measurement_insight_kinds = set(args.measurement_insight_kind or [])
    measurement_template = filter_measurement_template(template, measurement_insight_kinds)
    measurement_queue = build_measurement_queue(measurement_template, label_rows, pd.DataFrame(), pd.DataFrame(), args.measurement_queue_size)
    measurement_queue_summary = build_measurement_queue_summary(measurement_template, label_rows, measurement_queue)
    if args.analytics_db is not None:
        with sqlite3.connect(args.analytics_db) as analytics_conn:
            action_items = read_analytics_table(analytics_conn, "tpm_action_items")
            program_register = read_analytics_table(analytics_conn, "tpm_program_register")
            full_queue = build_measurement_queue(template, label_rows, action_items, program_register, args.measurement_queue_size)
            full_summary = build_measurement_queue_summary(template, label_rows, full_queue)
            measurement_queue = build_measurement_queue(measurement_template, label_rows, action_items, program_register, args.measurement_queue_size)
            measurement_queue_summary = build_measurement_queue_summary(measurement_template, label_rows, measurement_queue)
            label_rows.to_sql("tpm_review_labels", analytics_conn, if_exists="replace", index=False)
            metrics.to_sql("tpm_review_metrics", analytics_conn, if_exists="replace", index=False)
            report_rows.to_sql("tpm_review_label_report", analytics_conn, if_exists="replace", index=False)
            full_queue.to_sql("tpm_measurement_label_queue", analytics_conn, if_exists="replace", index=False)
            full_summary.to_sql("tpm_measurement_label_summary", analytics_conn, if_exists="replace", index=False)
            if measurement_insight_kinds:
                filtered_queue = with_queue_filter(measurement_queue, measurement_insight_kinds)
                filtered_summary = with_queue_filter(measurement_queue_summary, measurement_insight_kinds)
                filtered_queue.to_sql("tpm_measurement_label_queue_filtered", analytics_conn, if_exists="replace", index=False)
                filtered_summary.to_sql("tpm_measurement_label_summary_filtered", analytics_conn, if_exists="replace", index=False)

    if args.export_measurement_queue is not None:
        write_table_file(measurement_queue, args.export_measurement_queue)

    if args.report is not None:
        write_report(args.report, source_instance, imported_count, metrics, report_rows, measurement_queue_summary, measurement_queue)

    if args.measurement_queue_report is not None:
        write_measurement_queue_report(args.measurement_queue_report, source_instance, measurement_queue_summary, measurement_queue)


def build_label_template(conn: sqlite3.Connection, source_instance: str) -> pd.DataFrame:
    return pd.read_sql_query(
        """
        select
          wi.key as insight_key,
          wi.insight_kind,
          wi.subject_kind,
          wi.subject_key,
          wi.severity,
          wi.producer_state,
          wi.title,
          wi.details,
          wi.recommended_action,
          wi.score,
          wi.confidence,
          coalesce(e.claim_field, '') as evidence_claim_field,
          coalesce(e.locator_kind, '') as evidence_locator_kind,
          coalesce(e.source_url, '') as evidence_source_url,
          coalesce(e.source_span_key, '') as evidence_source_span_key,
          coalesce(e.span_start, '') as evidence_span_start,
          coalesce(e.span_end, '') as evidence_span_end,
          coalesce(e.excerpt, '') as evidence_excerpt,
          'unknown' as truth_label,
          'unknown' as actionability_label,
          '' as review_state,
          '' as owner_key,
          '' as next_action,
          '' as rationale
        from work_insights wi
        left join evidences e on e.id = wi.latest_evidence_id
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
          wi.insight_kind
        """,
        conn,
        params=(source_instance,),
    )


def filter_measurement_template(template: pd.DataFrame, insight_kinds: set[str]) -> pd.DataFrame:
    if template.empty or not insight_kinds:
        return template
    if "insight_kind" not in template.columns:
        return template.iloc[0:0].copy()
    allowed = {clean_text(kind) for kind in insight_kinds if clean_text(kind)}
    if not allowed:
        return template
    return template[template["insight_kind"].astype(str).map(clean_text).isin(allowed)].copy()


def with_queue_filter(frame: pd.DataFrame, insight_kinds: set[str]) -> pd.DataFrame:
    out = frame.copy()
    filter_label = ",".join(sorted(clean_text(kind) for kind in insight_kinds if clean_text(kind)))
    out.insert(0, "queue_filter", f"insight_kind:{filter_label}" if filter_label else "")
    return out


def import_labels(
    conn: sqlite3.Connection,
    labels: pd.DataFrame,
    source_instance: str,
    label_set: str,
    label_quality: str,
    measurement_label_sets: set[str],
    reviewer_key: str,
    reviewed_at: datetime,
    label_path: Path,
) -> int:
    if labels.empty:
        return 0
    labels = normalize_import_label_columns(labels)
    required = {"insight_key", "truth_label", "actionability_label"}
    missing = required.difference(labels.columns)
    if missing:
        raise SystemExit(f"label file is missing required columns: {', '.join(sorted(missing))}")

    reviewed_iso = reviewed_at.isoformat()
    now_iso = datetime.now(timezone.utc).isoformat()
    reviewer_kind = "imported"
    imported = 0
    for row in labels.to_dict("records"):
        insight_key = clean_text(row.get("insight_key"))
        if not insight_key:
            continue
        truth_label = clean_text(row.get("truth_label")) or "unknown"
        actionability_label = clean_text(row.get("actionability_label")) or "unknown"
        if truth_label == "unknown" and actionability_label == "unknown":
            continue
        validate_enum("truth_label", truth_label, TRUTH_LABELS)
        validate_enum("actionability_label", actionability_label, ACTIONABILITY_LABELS)
        review_state = clean_text(row.get("review_state")) or review_state_for(truth_label, actionability_label)
        validate_enum("review_state", review_state, REVIEW_STATES)
        measurement_eligible = measurement_eligible_value(
            clean_text(row.get("measurement_eligible")),
            "evaluation_label",
            label_quality,
            label_set,
            measurement_label_sets,
        )
        insight = conn.execute(
            """
            select id, source_url
            from work_insights
            where key = ?
              and source_system = 'cubicle_analytics'
              and source_instance = ?
              and external_kind = 'tpm_insight'
            """,
            (insight_key, source_instance),
        ).fetchone()
        if insight is None:
            raise SystemExit(f"label references unknown insight_key for source_instance {source_instance}: {insight_key}")
        insight_id, insight_source_url = int(insight[0]), clean_text(insight[1])
        external_id = stable_digest([label_set, insight_key])
        review_key = f"work-insight-review:cubicle-evaluation:{source_instance}:{label_set}:{external_id}"
        next_action = clean_text(row.get("next_action"))
        rationale = clean_text(row.get("rationale"))
        owner_key = clean_text(row.get("owner_key"))
        conn.execute(
            """
            insert into work_insight_reviews (
              key, work_insight_id, review_kind, review_state, truth_label,
              actionability_label, label_set, label_quality, measurement_eligible, reviewer_kind, reviewer_key, owner_key,
              next_action, rationale, reviewed_at,
              source_system, source_instance, external_kind, external_id,
              source_url, created_at, updated_at
            ) values (
              ?, ?, 'evaluation_label', ?, ?,
              ?, ?, ?, ?, ?, ?, ?,
              ?, ?, ?,
              'cubicle_evaluation', ?, 'tpm_review_label', ?,
              ?, ?, ?
            )
            on conflict(source_system, source_instance, external_kind, external_id) do update set
              review_state = excluded.review_state,
              truth_label = excluded.truth_label,
              actionability_label = excluded.actionability_label,
              label_set = excluded.label_set,
              label_quality = excluded.label_quality,
              measurement_eligible = excluded.measurement_eligible,
              reviewer_kind = excluded.reviewer_kind,
              reviewer_key = excluded.reviewer_key,
              owner_key = excluded.owner_key,
              next_action = excluded.next_action,
              rationale = excluded.rationale,
              reviewed_at = excluded.reviewed_at,
              source_url = excluded.source_url,
              updated_at = excluded.updated_at
            """,
            (
                review_key,
                insight_id,
                review_state,
                truth_label,
                actionability_label,
                label_set,
                label_quality,
                measurement_eligible,
                reviewer_kind,
                reviewer_key,
                owner_key,
                next_action,
                rationale,
                reviewed_iso,
                source_instance,
                external_id,
                str(label_path),
                now_iso,
                now_iso,
            ),
        )
        imported += 1
    conn.commit()
    return imported


def normalize_import_label_columns(labels: pd.DataFrame) -> pd.DataFrame:
    normalized = labels.copy()
    aliases = {
        "truth_label": "gold_truth_label",
        "actionability_label": "gold_actionability_label",
        "review_state": "gold_review_state",
        "owner_key": "gold_owner_key",
        "next_action": "gold_next_action",
        "rationale": "gold_rationale",
    }
    for target, source in aliases.items():
        if target not in normalized.columns and source in normalized.columns:
            normalized[target] = normalized[source]
    return normalized


def read_evaluation_labels(
    conn: sqlite3.Connection,
    source_instance: str,
    measurement_label_sets: set[str],
) -> pd.DataFrame:
    labels = pd.read_sql_query(
        """
        select
          wir.id as review_id,
          wi.key as insight_key,
          wi.insight_kind,
          wi.subject_kind,
          wi.subject_key,
          wi.severity,
          wi.producer_state,
          wi.score,
          wi.confidence,
          wir.key as review_key,
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
          coalesce(wir.source_system, '') as label_source_system,
          coalesce(wir.external_id, '') as label_external_id,
          coalesce(wir.source_url, '') as label_source_url
        from work_insight_reviews wir
        join work_insights wi on wi.id = wir.work_insight_id
        where wi.source_system = 'cubicle_analytics'
          and wi.source_instance = ?
          and wi.external_kind = 'tpm_insight'
          and wi.producer_state = 'current'
          and wir.review_kind in ('human_assessment', 'evaluation_label')
          and (
            wir.truth_label != 'unknown'
            or wir.actionability_label != 'unknown'
          )
        order by wi.insight_kind, wi.subject_key, wir.reviewed_at, wir.id
        """,
        conn,
        params=(source_instance,),
    )
    if labels.empty:
        return labels
    labels["label_set"] = labels["label_set"].where(labels["label_set"].astype(str) != "", labels["review_key"].map(extract_label_set))
    labels["label_quality"] = labels.apply(infer_label_quality_from_review, axis=1)
    labels["measurement_eligible"] = labels.apply(
        lambda row: "true" if is_measurement_label(row, measurement_label_sets) else "false",
        axis=1,
    )
    return labels


def build_metrics(labels: pd.DataFrame) -> pd.DataFrame:
    rows: list[dict[str, str]] = []
    rows.extend(metric_rows_for_scope("all", labels))
    if not labels.empty:
        for insight_kind, group in labels.groupby("insight_kind", sort=True):
            rows.extend(metric_rows_for_scope(f"insight_kind:{insight_kind}", group))
    return pd.DataFrame(rows)


def measurement_label_rows(labels: pd.DataFrame) -> pd.DataFrame:
    if labels.empty or "measurement_eligible" not in labels.columns:
        return labels
    eligible = labels[labels["measurement_eligible"] == "true"].copy()
    return dedupe_label_rows(eligible)


def dedupe_label_rows(labels: pd.DataFrame) -> pd.DataFrame:
    if labels.empty:
        return labels
    ranked = labels.copy()
    ranked["_quality_rank"] = ranked["label_quality"].map(lambda value: {"gold": 4, "adversarial": 3, "candidate": 2, "smoke": 1}.get(str(value), 0))
    ranked["_review_kind_rank"] = ranked["review_kind"].map(lambda value: 3 if value == "human_assessment" else 1)
    ranked["_review_id"] = pd.to_numeric(ranked.get("review_id", 0), errors="coerce").fillna(0)
    ranked = ranked.sort_values(["insight_key", "_quality_rank", "_review_kind_rank", "reviewed_at", "_review_id"])
    return ranked.drop_duplicates("insight_key", keep="last").drop(columns=["_quality_rank", "_review_kind_rank", "_review_id"])


def metric_rows_for_scope(scope: str, labels: pd.DataFrame) -> list[dict[str, str]]:
    total = len(labels)
    if total == 0:
        return [
            {"scope": scope, "metric": "labeled_count", "value": "0", "note": "no measurement-eligible labels"},
            {"scope": scope, "metric": "ready_to_measure_precision", "value": "false", "note": "requires measurement-eligible labels"},
            {"scope": scope, "metric": "ready_to_measure_actionability", "value": "false", "note": "requires measurement-eligible labels"},
        ]

    truth_known = labels[labels["truth_label"] != "unknown"]
    action_known = labels[labels["actionability_label"] != "unknown"]
    true_positive = int((truth_known["truth_label"] == "true_positive").sum())
    false_positive = int((truth_known["truth_label"] == "false_positive").sum())
    partial = int((truth_known["truth_label"] == "partial").sum())
    precision_denominator = true_positive + false_positive + partial
    precision = (true_positive + 0.5 * partial) / precision_denominator if precision_denominator else None
    actionable = int(action_known["actionability_label"].isin(POSITIVE_ACTIONABILITY).sum())
    not_actionable = int((action_known["actionability_label"] == "not_actionable").sum())
    actionability_rate = actionable / len(action_known) if len(action_known) else None
    min_labeled_total = 10
    return [
        {"scope": scope, "metric": "labeled_count", "value": str(total), "note": "deduped measurement-eligible labels for current insights"},
        {"scope": scope, "metric": "truth_labeled_count", "value": str(len(truth_known)), "note": "labels with non-unknown truth"},
        {"scope": scope, "metric": "actionability_labeled_count", "value": str(len(action_known)), "note": "labels with non-unknown actionability"},
        {"scope": scope, "metric": "true_positive_count", "value": str(true_positive), "note": "truth_label=true_positive"},
        {"scope": scope, "metric": "partial_count", "value": str(partial), "note": "truth_label=partial"},
        {"scope": scope, "metric": "false_positive_count", "value": str(false_positive), "note": "truth_label=false_positive"},
        {"scope": scope, "metric": "precision_estimate", "value": format_metric(precision), "note": "tp plus half partial divided by tp+partial+fp"},
        {"scope": scope, "metric": "actionable_count", "value": str(actionable), "note": "actionable or needs_owner labels"},
        {"scope": scope, "metric": "not_actionable_count", "value": str(not_actionable), "note": "actionability_label=not_actionable"},
        {"scope": scope, "metric": "actionability_rate", "value": format_metric(actionability_rate), "note": "actionable or needs_owner over actionability-labeled rows"},
        {"scope": scope, "metric": "ready_to_measure_precision", "value": "true" if len(truth_known) >= min_labeled_total else "false", "note": "requires at least 10 truth labels"},
        {"scope": scope, "metric": "ready_to_measure_actionability", "value": "true" if len(action_known) >= min_labeled_total else "false", "note": "requires at least 10 actionability labels"},
    ]


def build_report_rows(labels: pd.DataFrame) -> pd.DataFrame:
    if labels.empty:
        return labels
    columns = [
        "insight_kind",
        "subject_kind",
        "subject_key",
        "severity",
        "truth_label",
        "actionability_label",
        "review_state",
        "reviewer_key",
        "owner_key",
        "label_set",
        "label_quality",
        "measurement_eligible",
        "next_action",
        "rationale",
    ]
    return labels[columns]


def build_measurement_queue(
    template: pd.DataFrame,
    labels: pd.DataFrame,
    action_items: pd.DataFrame,
    program_register: pd.DataFrame,
    queue_size: int,
) -> pd.DataFrame:
    columns = measurement_queue_columns()
    if template.empty:
        return pd.DataFrame(columns=columns)
    current = template[template["producer_state"] == "current"].copy() if "producer_state" in template.columns else template.copy()
    if current.empty:
        return pd.DataFrame(columns=columns)
    label_context = latest_label_context(labels)
    action_context = action_context_by_subject(action_items)
    register_context = register_context_by_subject(program_register)
    rows: list[dict[str, Any]] = []
    for _, insight in current.iterrows():
        insight_key = clean_text(insight.get("insight_key"))
        subject_kind = clean_text(insight.get("subject_kind"))
        subject_key = clean_text(insight.get("subject_key"))
        label = label_context.get(insight_key, {})
        action = action_context.get((subject_kind, subject_key), {})
        register = register_context.get((subject_kind, subject_key), {})
        measurement_eligible = clean_text(label.get("measurement_eligible"))
        if measurement_eligible == "true":
            continue
        bucket = measurement_bucket(insight, label, action, register)
        priority = measurement_priority(insight, label, action, register, bucket)
        rows.append(
            {
                "queue_rank": 0,
                "measurement_bucket": bucket,
                "priority_score": priority,
                "insight_key": insight_key,
                "insight_kind": clean_text(insight.get("insight_kind")),
                "subject_kind": subject_kind,
                "subject_key": subject_key,
                "severity": clean_text(insight.get("severity")),
                "program_status": clean_text(register.get("program_status")),
                "action_type": clean_text(action.get("action_type")),
                "due_bucket": clean_text(register.get("due_bucket")),
                "owner_key": first_nonempty([register.get("owner_key"), action.get("owner_hint"), label.get("owner_key")]),
                "requested_reviewer_keys": clean_text(register.get("requested_reviewer_keys")),
                "title": clean_text(insight.get("title")),
                "evidence_locator_kind": clean_text(insight.get("evidence_locator_kind")),
                "evidence_source_url": clean_text(insight.get("evidence_source_url")),
                "evidence_excerpt": clean_text(insight.get("evidence_excerpt")),
                "existing_label_quality": clean_text(label.get("label_quality")),
                "existing_measurement_eligible": measurement_eligible or "false",
                "existing_truth_label": clean_text(label.get("truth_label")),
                "existing_actionability_label": clean_text(label.get("actionability_label")),
                "existing_review_state": clean_text(label.get("review_state")),
                "existing_rationale": clean_text(label.get("rationale")),
                "truth_label_options": TRUTH_LABEL_OPTIONS,
                "actionability_label_options": ACTIONABILITY_LABEL_OPTIONS,
                "review_state_options": REVIEW_STATE_OPTIONS,
                "promotion_guardrail": measurement_promotion_guardrail(insight, bucket),
                "gold_truth_label": "unknown",
                "gold_actionability_label": "unknown",
                "gold_review_state": "",
                "gold_owner_key": "",
                "gold_next_action": "",
                "gold_rationale": "",
                "review_prompt": measurement_review_prompt(insight, label, action, register),
            }
        )
    if not rows:
        return pd.DataFrame(columns=columns)
    queue = pd.DataFrame(rows, columns=columns)
    queue = queue.sort_values(["priority_score", "severity", "subject_key", "insight_kind"], ascending=[False, True, True, True]).head(max(1, queue_size)).copy()
    queue["queue_rank"] = range(1, len(queue) + 1)
    return queue


def measurement_queue_columns() -> list[str]:
    return [
        "queue_rank",
        "measurement_bucket",
        "priority_score",
        "insight_key",
        "insight_kind",
        "subject_kind",
        "subject_key",
        "severity",
        "program_status",
        "action_type",
        "due_bucket",
        "owner_key",
        "requested_reviewer_keys",
        "title",
        "evidence_locator_kind",
        "evidence_source_url",
        "evidence_excerpt",
        "existing_label_quality",
        "existing_measurement_eligible",
        "existing_truth_label",
        "existing_actionability_label",
        "existing_review_state",
        "existing_rationale",
        "truth_label_options",
        "actionability_label_options",
        "review_state_options",
        "promotion_guardrail",
        "gold_truth_label",
        "gold_actionability_label",
        "gold_review_state",
        "gold_owner_key",
        "gold_next_action",
        "gold_rationale",
        "review_prompt",
    ]


def latest_label_context(labels: pd.DataFrame) -> dict[str, dict[str, Any]]:
    if labels.empty:
        return {}
    deduped = dedupe_label_rows(labels)
    return {clean_text(row.get("insight_key")): row.to_dict() for _, row in deduped.iterrows()}


def action_context_by_subject(action_items: pd.DataFrame) -> dict[tuple[str, str], dict[str, Any]]:
    if action_items.empty:
        return {}
    rows = action_items.copy()
    rows["priority_score"] = pd.to_numeric(rows.get("priority_score", 0), errors="coerce").fillna(0)
    rows = rows.sort_values(["priority_score", "subject_key"], ascending=[False, True])
    return {
        (clean_text(row.get("subject_kind")), clean_text(row.get("subject_key"))): row.to_dict()
        for _, row in rows.drop_duplicates(["subject_kind", "subject_key"], keep="first").iterrows()
    }


def register_context_by_subject(program_register: pd.DataFrame) -> dict[tuple[str, str], dict[str, Any]]:
    if program_register.empty:
        return {}
    rows = program_register.copy()
    rows["_due_rank"] = rows.get("due_bucket", pd.Series(dtype=str)).map(lambda value: {"now": 3, "this_week": 2, "watch": 1}.get(str(value), 0))
    rows = rows.sort_values(["_due_rank", "risk_score"], ascending=[False, False])
    return {
        (clean_text(row.get("subject_kind")), clean_text(row.get("subject_key"))): row.drop(labels=["_due_rank"], errors="ignore").to_dict()
        for _, row in rows.drop_duplicates(["subject_kind", "subject_key"], keep="first").iterrows()
    }


def measurement_bucket(insight: pd.Series, label: dict[str, Any], action: dict[str, Any], register: dict[str, Any]) -> str:
    if clean_text(label.get("measurement_eligible")) == "true":
        return "already_measured"
    if clean_text(label.get("truth_label")) == "false_positive" or clean_text(label.get("actionability_label")) == "not_actionable":
        return "candidate_dismissal_check"
    if clean_text(register.get("program_status")) == "closed_pending_review":
        return "resolution_check"
    if clean_text(action.get("action_type")) == "ci_check_followup":
        return "ci_actionability"
    if clean_text(insight.get("insight_kind")) == "blocker_candidate":
        return "blocker_precision"
    if clean_text(insight.get("insight_kind")) == "forecast_risk":
        return "risk_actionability"
    if clean_text(insight.get("insight_kind")) == "developer_correlation":
        return "developer_correlation_actionability"
    return "general_quality"


def measurement_priority(
    insight: pd.Series,
    label: dict[str, Any],
    action: dict[str, Any],
    register: dict[str, Any],
    bucket: str,
) -> float:
    score = severity_rank(clean_text(insight.get("severity"))) * 10.0
    score += safe_float(insight.get("score")) / 5.0
    if clean_text(register.get("due_bucket")) == "now":
        score += 15
    if clean_text(action.get("action_type")) in {"decision_or_owner_followup", "validate_signal", "ci_check_followup"}:
        score += 10
    if bucket == "candidate_dismissal_check":
        score += 20
    if bucket == "resolution_check":
        score += 12
    if bucket == "developer_correlation_actionability":
        score += 14
    if clean_text(label.get("label_quality")) in {"smoke", "candidate", "adversarial"}:
        score += 4
    return round(score, 2)


def measurement_review_prompt(
    insight: pd.Series,
    label: dict[str, Any],
    action: dict[str, Any],
    register: dict[str, Any],
) -> str:
    parts = [
        "Gold-label this current TPM insight.",
        f"Question: is `{clean_text(insight.get('insight_kind'))}` for `{clean_text(insight.get('subject_key'))}` true/partial/false, and actionable/not_actionable/needs_owner?",
    ]
    action_type = clean_text(action.get("action_type"))
    if action_type:
        parts.append(f"Operating action currently says `{action_type}`.")
    if clean_text(insight.get("insight_kind")) == "developer_correlation":
        parts.append(
            "For developer correlation, label only whether the direct-identity same-window Jira workload context is useful for routing, capacity, or escalation; do not treat it as ownership, performance, causality, ETA, or blocker evidence."
        )
    existing_truth = clean_text(label.get("truth_label"))
    existing_actionability = clean_text(label.get("actionability_label"))
    existing_quality = clean_text(label.get("label_quality"))
    if existing_truth or existing_actionability:
        parts.append(f"Existing non-measurement label is {existing_quality or 'unknown_quality'}: truth={existing_truth or 'unknown'}, actionability={existing_actionability or 'unknown'}. Re-check independently.")
    if clean_text(register.get("program_status")):
        parts.append(f"Program register status is `{clean_text(register.get('program_status'))}` with due bucket `{clean_text(register.get('due_bucket'))}`.")
    return " ".join(parts)


def measurement_promotion_guardrail(insight: pd.Series, bucket: str) -> str:
    insight_kind = clean_text(insight.get("insight_kind"))
    if insight_kind == "blocker_candidate":
        return "Only accepted true_positive plus actionable or needs_owner gold labels may support blocker/product-action promotion; partial remains needs_more_data validation."
    if insight_kind == "forecast_risk":
        return "Forecast labels validate risk-triage actionability only; they are not ETA commitments unless the separate ETA readiness gate passes."
    if insight_kind == "developer_correlation":
        return "Developer-correlation labels are routing/capacity context only; never use them as ownership, causality, performance, ETA, or blocker evidence."
    if bucket == "candidate_dismissal_check":
        return "Re-check dismissal independently; false_positive or not_actionable labels suppress escalation but do not prove absence outside the cited evidence."
    if bucket == "resolution_check":
        return "Resolution labels validate closeout handling only; confirm terminal source state before closing product work."
    return "Gold labels must be independently reviewed against cited evidence before they affect measurement or product-action gates."


def build_measurement_queue_summary(template: pd.DataFrame, labels: pd.DataFrame, queue: pd.DataFrame) -> pd.DataFrame:
    current_count = int((template["producer_state"] == "current").sum()) if not template.empty and "producer_state" in template.columns else len(template)
    scoped_labels = labels
    if not template.empty and not labels.empty and "insight_key" in template.columns and "insight_key" in labels.columns:
        insight_keys = set(template["insight_key"].astype(str).map(clean_text))
        scoped_labels = labels[labels["insight_key"].astype(str).map(clean_text).isin(insight_keys)].copy()
    measurement_rows = measurement_label_rows(scoped_labels)
    non_measurement_count = int((scoped_labels.get("measurement_eligible", pd.Series(dtype=str)) != "true").sum()) if not scoped_labels.empty else 0
    rows = [
        {"metric": "current_insight_count", "value": str(current_count), "note": "current generated insights eligible for measurement review"},
        {"metric": "measurement_label_count", "value": str(len(measurement_rows)), "note": "deduped measurement-grade labels already available"},
        {"metric": "non_measurement_label_count", "value": str(non_measurement_count), "note": "smoke/candidate/adversarial labels available only as QA context"},
        {"metric": "measurement_queue_count", "value": str(len(queue)), "note": "current insights exported for gold-label review"},
    ]
    if not queue.empty:
        for bucket, count in queue.groupby("measurement_bucket").size().items():
            rows.append({"metric": f"queue_{bucket}", "value": str(int(count)), "note": "measurement queue rows by bucket"})
    return pd.DataFrame(rows)


def write_report(
    path: Path,
    source_instance: str,
    imported_count: int,
    metrics: pd.DataFrame,
    labels: pd.DataFrame,
    measurement_queue_summary: pd.DataFrame,
    measurement_queue: pd.DataFrame,
) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = [
        "# Flink AI TPM Review Evaluation",
        "",
        f"Source instance: {source_instance}",
        f"Imported/updated labels this run: {imported_count}",
        "",
        "## Metrics",
        "",
        df_to_markdown(metrics),
        "",
        "## Measurement Queue Summary",
        "",
        df_to_markdown(measurement_queue_summary),
        "",
        "## Measurement Queue",
        "",
        df_to_markdown(measurement_queue.head(50)) if not measurement_queue.empty else "No current insights require measurement labels.",
        "",
        "## Labels",
        "",
        df_to_markdown(labels.head(100)) if not labels.empty else "No labels imported for current insights.",
        "",
        "## Interpretation",
        "",
        "- Labels are stored as separate `evaluation_label` review rows and do not mutate generated insights or triage requests.",
        "- Metrics use only measurement-eligible labels: human assessments and imported `gold` labels, unless a row is explicitly stored with `measurement_eligible=false`.",
        "- The measurement queue is a review packet: blank `gold_*` columns are for future gold labels and do not change metrics until imported as gold or human labels.",
        "- Smoke, candidate, and adversarial labels are retained for QA but do not make the model measurement-ready by default.",
        "- Precision estimates are only measured over deduped labeled current insights; they are not recall or project-wide quality.",
        "- `needs_owner` counts as actionable because it maps to a concrete TPM handoff.",
    ]
    path.write_text("\n".join(lines) + "\n")


def write_measurement_queue_report(path: Path, source_instance: str, summary: pd.DataFrame, queue: pd.DataFrame) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    lines = [
        "# Flink AI TPM Gold-Label Queue",
        "",
        f"Source instance: {source_instance}",
        "",
        "## Summary",
        "",
        df_to_markdown(summary),
        "",
        "## Queue",
        "",
        df_to_markdown(queue) if not queue.empty else "No current insights require measurement labels.",
        "",
        "## How To Use",
        "",
        "- Fill `gold_truth_label`, `gold_actionability_label`, `gold_review_state`, `gold_owner_key`, `gold_next_action`, and `gold_rationale`.",
        f"- Allowed truth labels: `{TRUTH_LABEL_OPTIONS}`.",
        f"- Allowed actionability labels: `{ACTIONABILITY_LABEL_OPTIONS}`.",
        f"- Allowed review states: `{REVIEW_STATE_OPTIONS}`.",
        "- Treat each row's `promotion_guardrail` as the boundary for what that label is allowed to prove.",
        "- Import this completed queue directly with `--import-labels ... --label-quality gold`; non-gold label sets remain QA context.",
        "- Existing smoke/candidate/adversarial labels are context only and should be re-checked independently.",
    ]
    path.write_text("\n".join(lines) + "\n")


def read_table_file(path: Path) -> pd.DataFrame:
    sep = "\t" if path.suffix.lower() in {".tsv", ".tab"} else ","
    return pd.read_csv(path, sep=sep, dtype=str, keep_default_na=False)


def write_table_file(df: pd.DataFrame, path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    sep = "\t" if path.suffix.lower() in {".tsv", ".tab"} else ","
    df.to_csv(path, sep=sep, index=False, quoting=csv.QUOTE_MINIMAL)


def read_analytics_table(conn: sqlite3.Connection, table_name: str) -> pd.DataFrame:
    if not table_exists(conn, table_name):
        return pd.DataFrame()
    return pd.read_sql_query(f"select * from {table_name}", conn)


def table_exists(conn: sqlite3.Connection, table_name: str) -> bool:
    row = conn.execute("select 1 from sqlite_master where type = 'table' and name = ?", (table_name,)).fetchone()
    return row is not None


def infer_source_instance(conn: sqlite3.Connection) -> str:
    row = conn.execute(
        """
        select source_instance
        from work_insights
        where source_system = 'cubicle_analytics'
          and external_kind = 'tpm_insight'
        group by source_instance
        order by count(*) desc
        limit 1
        """
    ).fetchone()
    if row is None:
        raise SystemExit("could not infer source_instance; pass --source-instance")
    return str(row[0])


def resolve_label_quality(requested: str, label_set: str, label_path: Path | None, reviewer_key: str) -> str:
    if requested != "auto":
        return requested
    text = " ".join([label_set, str(label_path or ""), reviewer_key]).lower()
    if "adversarial" in text:
        return "adversarial"
    if "smoke" in text:
        return "smoke"
    if "gold" in text:
        return "gold"
    return "candidate"


def extract_label_set(review_key: Any) -> str:
    parts = clean_text(review_key).split(":")
    if len(parts) >= 5 and parts[0] == "work-insight-review" and parts[1] == "cubicle-evaluation":
        return parts[-2]
    return ""


def infer_label_quality_from_review(row: pd.Series) -> str:
    explicit = clean_text(row.get("label_quality"))
    if explicit in LABEL_QUALITIES - {"auto"}:
        return explicit
    if clean_text(row.get("review_kind")) == "human_assessment":
        return "gold"
    reviewer_kind = clean_text(row.get("reviewer_kind"))
    if reviewer_kind.startswith("imported_"):
        quality = reviewer_kind.removeprefix("imported_")
        if quality in LABEL_QUALITIES - {"auto"}:
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


def measurement_eligible_value(
    explicit: str,
    review_kind: str,
    label_quality: str,
    label_set: str,
    measurement_label_sets: set[str],
) -> bool:
    explicit = clean_text(explicit).lower()
    measurement_label = is_measurement_label(
        pd.Series(
            {
                "review_kind": review_kind,
                "label_quality": label_quality,
                "label_set": label_set,
            }
        ),
        measurement_label_sets,
    )
    if explicit in {"false", "0", "no"}:
        return False
    if explicit in {"true", "1", "yes"}:
        return measurement_label
    return measurement_label


def stored_measurement_eligible(row: pd.Series) -> bool:
    value = clean_text(row.get("stored_measurement_eligible")).lower()
    return value in {"true", "1", "yes"}


def backfill_measurement_eligibility(
    conn: sqlite3.Connection,
    source_instance: str,
    measurement_label_sets: set[str],
) -> None:
    if not column_exists(conn, "work_insight_reviews", "measurement_eligible"):
        return
    source_filter = "and coalesce(wir.source_system, '') != 'cubicle_evaluation'" if column_exists(conn, "work_insight_reviews", "source_system") else ""
    rows = conn.execute(
        f"""
        select wir.id, wir.review_kind, coalesce(wir.label_quality, ''),
               coalesce(wir.label_set, ''), wir.key
          from work_insight_reviews wir
          join work_insights wi on wi.id = wir.work_insight_id
         where wi.source_system = 'cubicle_analytics'
           and wi.source_instance = ?
           and wi.external_kind = 'tpm_insight'
           {source_filter}
        """,
        (source_instance,),
    ).fetchall()
    for review_id, review_kind, label_quality, label_set, review_key in rows:
        if not label_set:
            label_set = extract_label_set(review_key)
        eligible = measurement_eligible_value("", review_kind, label_quality, label_set, measurement_label_sets)
        conn.execute(
            "update work_insight_reviews set measurement_eligible = ? where id = ?",
            (1 if eligible else 0, review_id),
        )
    conn.commit()


def column_exists(conn: sqlite3.Connection, table_name: str, column_name: str) -> bool:
    if not table_exists(conn, table_name):
        return False
    return any(row[1] == column_name for row in conn.execute(f"pragma table_info({table_name})").fetchall())


def review_state_for(truth_label: str, actionability_label: str) -> str:
    if truth_label == "false_positive" or actionability_label == "not_actionable":
        return "dismissed"
    if truth_label == "unknown" and actionability_label == "unknown":
        return "requested"
    if truth_label == "partial":
        return "needs_more_data"
    return "accepted"


def validate_enum(name: str, value: str, allowed: set[str]) -> None:
    if value not in allowed:
        raise SystemExit(f"invalid {name}={value!r}; expected one of {', '.join(sorted(allowed))}")


def format_metric(value: float | None) -> str:
    if value is None:
        return ""
    return f"{value:.3f}"


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
    return str(value).replace("\n", " ").replace("|", "\\|").strip()


def parse_dt(value: Any) -> datetime | None:
    if not value:
        return None
    text = str(value).replace("Z", "+00:00")
    try:
        dt = datetime.fromisoformat(text)
    except ValueError:
        return None
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc)


def first_nonempty(values: list[Any]) -> str:
    for value in values:
        text = clean_text(value)
        if text:
            return text
    return ""


def safe_float(value: Any) -> float:
    try:
        result = float(value)
    except (TypeError, ValueError):
        return 0.0
    if math.isnan(result):
        return 0.0
    return result


def severity_rank(value: str) -> int:
    return {
        "critical": 5,
        "high": 4,
        "medium": 3,
        "low": 2,
        "info": 1,
    }.get(clean_text(value), 0)


def clean_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, float) and math.isnan(value):
        return ""
    return str(value).strip()


def stable_digest(parts: list[Any]) -> str:
    payload = "\x1f".join(str(part or "") for part in parts)
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:24]


if __name__ == "__main__":
    main()
