#!/usr/bin/env python3
"""Build a persisted TPM operating brief from generated insights.

The brief is a projection over analytics output. It does not replace
WorkInsight or WorkInsightReview as product truth and it does not label model
precision. Its job is to answer the TPM operating question: what should be
looked at next, given generated risks and the latest source observations?
"""

from __future__ import annotations

import argparse
import hashlib
import math
import re
import sqlite3
from collections import Counter
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import pandas as pd


SEVERITY_RANK = {
    "critical": 5,
    "high": 4,
    "medium": 3,
    "low": 2,
    "info": 1,
}
MEASUREMENT_LABEL_QUALITIES = {"gold"}
POSITIVE_ACTIONABILITY = {"actionable", "needs_owner"}
RESOLVED_REVIEW_STATES = {"accepted", "dismissed", "resolved"}
MIN_MEASUREMENT_LABEL_TOTAL = 10
MIN_MEASUREMENT_LABEL_PER_KIND = 10
MIN_PRECISION_RATE_FOR_PRODUCT_ACTION = 0.70
MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION = 0.80
MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION = 0.70
SOURCE_RESOLVED_DECISION = "source_resolved"
SOURCE_RESOLVED_AUTH_STATES = {"github_token", "authenticated", "token"}
SOURCE_RESOLVED_TERMINAL_STATES = {"closed", "merged"}
INSIGHT_CARD_IDENTITY_COLUMNS = ["insight_kind", "identity_key", "subject_kind", "subject_key"]
INSIGHT_CARD_STATE_COLUMNS = {"producer_state", "stale_reason"}
PRODUCT_ACTION_MEASUREMENT_KINDS = {"status_summary", "blocker_candidate", "forecast_risk"}
CONTEXT_ONLY_MEASUREMENT_KINDS = {"developer_correlation", "dependency_cluster"}
MODEL_QUALITY_MEASUREMENT_KINDS = {"model_quality"}
GLOBAL_INSIGHT_PRECISION_KEY = "global_insight_precision"
GLOBAL_INSIGHT_ACTIONABILITY_KEY = "global_insight_actionability"
ETA_TEMPORAL_READY_STATES = {"as_of_feature_snapshot_series_ready"}
MIN_RISK_TRIAGE_LIFT_FOR_OWNER_FOLLOWUP = 0.0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--analytics-db", required=True, type=Path)
    parser.add_argument("--ontology-db", type=Path, default=None)
    parser.add_argument("--source-instance", default="")
    parser.add_argument("--report", required=True, type=Path)
    parser.add_argument("--generated-at", default="")
    parser.add_argument("--measurement-label-set", action="append", default=[])
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    with sqlite3.connect(args.analytics_db) as conn:
        base_insight_cards = read_table(conn, "tpm_insight_cards")
        current_insight_cards = read_table(conn, "tpm_current_insight_cards")
        insight_cards = choose_operating_insight_cards(base_insight_cards, current_insight_cards)
        followups = read_table(conn, "tpm_followup_observations")
        check_observations = read_table(conn, "tpm_pr_check_observations")
        review_queue = read_table(conn, "tpm_insight_review_queue")
        pr_features = read_table(conn, "tpm_pr_forecasts")
        ticket_features = read_table(conn, "tpm_ticket_features")
        readiness = read_table(conn, "tpm_evaluation_readiness")
        run_metadata = read_table(conn, "tpm_run_metadata")
        forecast_summary = read_table(conn, "tpm_forecast_summary")
        forecast_backtest = read_table(conn, "tpm_forecast_backtest")
        forecast_risk_backtest = read_table(conn, "tpm_forecast_risk_backtest")
        decision_target_backtest = read_table(conn, "tpm_decision_target_backtest")
        check_summary = read_table(conn, "tpm_check_summary")
        check_signal_readiness = read_table(conn, "tpm_check_signal_readiness")
        transition_signal_readiness = read_table(conn, "tpm_transition_signal_readiness")
        transition_candidates_available = table_exists(conn, "tpm_state_transition_candidates")
        transition_candidates = read_table(conn, "tpm_state_transition_candidates")
        time_series_summary = read_table(conn, "tpm_time_series_summary")
        dependency_edges = read_table(conn, "tpm_dependency_edges")
        pr_state_snapshots = read_table(conn, "tpm_pr_state_snapshots")
        ticket_state_snapshots = read_table(conn, "tpm_ticket_state_snapshots")
        milestone_signals = read_table(conn, "tpm_milestone_signals")

    source_instance = args.source_instance or infer_source_instance(run_metadata)
    dependency_edges = source_topology_dependency_edges(dependency_edges)
    transition_candidates = source_scoped_transition_candidates(transition_candidates, source_instance)
    measurement_label_sets = set(args.measurement_label_set or [])
    if args.ontology_db is not None:
        with sqlite3.connect(args.ontology_db) as ontology_conn:
            backfill_review_measurement_eligibility(ontology_conn, source_instance, measurement_label_sets)
            review_queue = read_review_queue_from_ontology(ontology_conn, source_instance, measurement_label_sets)
        readiness = build_live_evaluation_readiness(review_queue)

    latest_observation_at = latest_observed_at(followups, check_observations) or infer_generated_at(followups)
    if args.generated_at:
        ensure_not_older_than_followup(args.generated_at, latest_observation_at)
    generated_at = args.generated_at or latest_observation_at
    action_items = build_action_items(
        insight_cards,
        followups,
        check_observations,
        review_queue,
        pr_features,
        ticket_features,
        readiness,
        forecast_summary,
        source_instance,
        generated_at,
    )
    action_items = append_transition_resolution_actions(action_items, transition_candidates, pr_features, generated_at, source_instance)
    action_items = apply_transition_evidence_to_resolution_actions(action_items, transition_candidates, source_instance)
    action_items = apply_source_resolved_closeouts(
        action_items,
        subject_current_state_by_subject(pr_features, ticket_features),
    )
    action_items = append_forecast_risk_backstop_actions(action_items, pr_features, forecast_summary, generated_at)
    summary = build_summary(action_items, readiness)
    owner_rollup = build_owner_action_rollup(action_items)
    program_register = build_program_register(action_items, pr_features, ticket_features, dependency_edges, transition_candidates, generated_at)
    standup_summary = build_workstream_standup(
        action_items,
        owner_rollup,
        summary,
        readiness,
        forecast_summary,
        check_summary,
        transition_candidates,
        time_series_summary,
        generated_at,
    )
    standup_sections = build_standup_sections(action_items, owner_rollup, transition_candidates)
    work_actions = build_work_actions(action_items, generated_at)
    work_action_observations = build_work_action_observations(action_items, generated_at)
    dependency_action_edges = pd.DataFrame(columns=dependency_action_edge_columns())

    with sqlite3.connect(args.analytics_db) as conn:
        insight_cards.to_sql("tpm_operating_insight_cards", conn, if_exists="replace", index=False)
        operating_current = insight_cards
        if "producer_state" in operating_current.columns:
            operating_current = operating_current[operating_current["producer_state"].astype(str) == "current"].copy()
        operating_current.to_sql("tpm_current_insight_cards", conn, if_exists="replace", index=False)
        action_items.to_sql("tpm_action_items", conn, if_exists="replace", index=False)
        summary.to_sql("tpm_action_summary", conn, if_exists="replace", index=False)
        program_register.to_sql("tpm_program_register", conn, if_exists="replace", index=False)
        owner_rollup.to_sql("tpm_owner_action_rollup", conn, if_exists="replace", index=False)
        standup_summary.to_sql("tpm_workstream_standup", conn, if_exists="replace", index=False)
        standup_sections.to_sql("tpm_standup_sections", conn, if_exists="replace", index=False)
        review_queue.to_sql("tpm_insight_review_queue", conn, if_exists="replace", index=False)
        readiness.to_sql("tpm_evaluation_readiness", conn, if_exists="replace", index=False)
        readiness.to_sql("tpm_action_evaluation_readiness", conn, if_exists="replace", index=False)
        work_actions.to_sql("tpm_work_actions", conn, if_exists="replace", index=False)
        work_action_observations.to_sql("tpm_work_action_observations", conn, if_exists="replace", index=False)

    if args.ontology_db is not None and source_instance:
        with sqlite3.connect(args.ontology_db) as ontology_conn:
            persist_work_actions_to_ontology(
                ontology_conn,
                source_instance,
                action_items,
                work_action_observations,
                generated_at,
            )
            persist_workstream_register_to_ontology(
                ontology_conn,
                source_instance,
                program_register,
                ticket_features,
                generated_at,
            )
            persist_work_program_items_to_ontology(
                ontology_conn,
                source_instance,
                program_register,
                generated_at,
            )
            persist_work_program_milestones_to_ontology(
                ontology_conn,
                source_instance,
                milestone_signals,
                generated_at,
            )
            persist_workstream_health_snapshots_to_ontology(
                ontology_conn,
                source_instance,
                standup_summary,
                generated_at,
            )
            persist_work_owner_load_snapshots_to_ontology(
                ontology_conn,
                source_instance,
                owner_rollup,
                generated_at,
            )
            persist_workstream_standup_sections_to_ontology(
                ontology_conn,
                source_instance,
                standup_sections,
                generated_at,
            )
            persist_work_blockers_to_ontology(
                ontology_conn,
                source_instance,
                action_items,
                generated_at,
            )
            persist_work_dependency_edges_to_ontology(
                ontology_conn,
                source_instance,
                dependency_edges,
                generated_at,
            )
            persist_work_blocker_impacts_to_ontology(
                ontology_conn,
                source_instance,
                generated_at,
            )
            persist_work_forecast_evaluations_to_ontology(
                ontology_conn,
                source_instance,
                forecast_summary,
                forecast_backtest,
                forecast_risk_backtest,
                decision_target_backtest,
                time_series_summary,
                generated_at,
            )
            persist_work_decision_target_evaluations_to_ontology(
                ontology_conn,
                source_instance,
                decision_target_backtest,
                generated_at,
            )
            persist_work_item_forecasts_to_ontology(
                ontology_conn,
                source_instance,
                pr_features,
                forecast_summary,
                time_series_summary,
                generated_at,
            )
            persist_work_item_state_snapshots_to_ontology(
                ontology_conn,
                source_instance,
                pr_state_snapshots,
                ticket_state_snapshots,
                generated_at,
            )
            if transition_candidates_available:
                persist_work_item_state_transitions_to_ontology(
                    ontology_conn,
                    source_instance,
                    transition_candidates,
                    generated_at,
                )
            persist_work_program_quality_gates_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                forecast_summary,
                time_series_summary,
                generated_at,
            )
            persist_work_program_adversarial_checks_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                forecast_summary,
                generated_at,
            )
            persist_work_program_evidence_needs_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                forecast_summary,
                generated_at,
            )
            persist_work_responsibilities_to_ontology(
                ontology_conn,
                source_instance,
                generated_at,
            )
            persist_work_program_tpm_function_readiness_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                forecast_summary,
                generated_at,
            )
            persist_work_program_automation_readiness_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                forecast_summary,
                generated_at,
            )
            persist_work_program_summary_snapshot_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                forecast_summary,
                generated_at,
            )
            persist_work_program_owner_rollup_snapshots_to_ontology(
                ontology_conn,
                source_instance,
                generated_at,
            )
            persist_work_insight_evaluation_snapshots_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                generated_at,
            )
            persist_work_program_brief_caveats_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                forecast_summary,
                generated_at,
            )
            persist_work_program_risk_drivers_to_ontology(
                ontology_conn,
                source_instance,
                generated_at,
            )
            persist_work_program_brief_snapshot_to_ontology(
                ontology_conn,
                source_instance,
                readiness,
                forecast_summary,
                generated_at,
            )
            persist_work_program_run_to_ontology(
                ontology_conn,
                source_instance,
                generated_at,
            )
            dependency_action_edges = ontology_dependency_action_edges_for_analytics(ontology_conn, source_instance)

    if args.ontology_db is not None and source_instance:
        merged_dependency_edges = merge_dependency_edges_for_analytics(dependency_edges, dependency_action_edges)
        with sqlite3.connect(args.analytics_db) as conn:
            merged_dependency_edges.to_sql("tpm_dependency_edges", conn, if_exists="replace", index=False)
            dependency_action_edges.to_sql("tpm_dependency_action_edges", conn, if_exists="replace", index=False)

    write_report(
        args.report,
        generated_at,
        action_items,
        summary,
        readiness,
        owner_rollup,
        standup_summary,
        standup_sections,
        program_register,
        check_signal_readiness,
        transition_signal_readiness,
        work_actions,
        work_action_observations,
    )


def build_action_items(
    insight_cards: pd.DataFrame,
    followups: pd.DataFrame,
    check_observations: pd.DataFrame,
    review_queue: pd.DataFrame,
    pr_features: pd.DataFrame,
    ticket_features: pd.DataFrame,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    source_instance: str,
    generated_at: str,
) -> pd.DataFrame:
    if insight_cards.empty:
        return empty_action_items()

    cards = insight_cards.copy()
    if "producer_state" in cards.columns:
        cards = cards[cards["producer_state"] == "current"].copy()
    if cards.empty:
        return empty_action_items()
    cards["severity_rank"] = cards["severity"].map(lambda value: SEVERITY_RANK.get(str(value), 0))
    cards["score"] = pd.to_numeric(cards["score"], errors="coerce").fillna(0.0)
    cards["confidence"] = pd.to_numeric(cards["confidence"], errors="coerce").fillna(0.0)

    followup_by_subject = aggregate_followups(followups)
    check_followup_by_subject = aggregate_check_observations(check_observations)
    review_by_subject = aggregate_reviews(review_queue)
    owner_hints = build_owner_hints(pr_features, ticket_features)
    pr_source_coverage_by_subject = aggregate_pr_source_coverage(pr_features)
    decision_gates = build_decision_gates(readiness, forecast_summary)

    rows: list[dict[str, Any]] = []
    grouped = cards.sort_values(["severity_rank", "score"], ascending=[False, False]).groupby(
        ["subject_kind", "subject_key"],
        sort=False,
    )
    for (subject_kind, subject_key), group in grouped:
        group = group.copy()
        is_ci_group = is_ci_check_group(group)
        followup = check_followup_by_subject.get((subject_kind, subject_key), {}) if is_ci_group else followup_by_subject.get((subject_kind, subject_key), {})
        if not followup:
            followup = followup_by_subject.get((subject_kind, subject_key), {})
        source_coverage_followup = pr_source_coverage_by_subject.get((subject_kind, subject_key), {})
        if should_use_source_coverage_followup(followup, source_coverage_followup):
            followup = merge_source_coverage_followup(followup, source_coverage_followup)
        reviews = review_by_subject.get((subject_kind, subject_key), {})
        owner_hint = owner_hints.get((subject_kind, subject_key), "")
        action_type = choose_action_type(group, followup, reviews, decision_gates)
        decision_state, decision_gate_reason = decision_state_for_action(action_type, group, reviews, decision_gates, followup)
        raw_priority_score = compute_priority_score(group, followup, action_type, clamp=False)
        priority_score = compute_priority_score(group, followup, action_type)
        urgency = priority_to_urgency(priority_score, action_type)
        top = group.iloc[0]
        action_key = f"tpm-action:{stable_digest([subject_kind, subject_key, ','.join(sorted(group['insight_kind'].astype(str).unique()))])}"
        insight_kinds = ",".join(sorted(group["insight_kind"].astype(str).unique()))
        evidence_group = group_for_action_evidence(action_type, group, reviews)
        source_link_group = group_for_action_source_links(action_type, group, evidence_group)
        source_insight_keys = ",".join(sorted(source_insight_keys_for_group(source_link_group, source_instance)))
        source_link_insight_kinds = ",".join(sorted(source_link_group["insight_kind"].astype(str).unique())) if not source_link_group.empty else ""
        current_state = str(followup.get("current_state") or "")
        baseline_state = str(followup.get("baseline_state") or "")
        outcome_signal = str(followup.get("outcome_signal") or "not_observed")
        source_observation_status = source_status(followup)
        source_coverage_kind = str(followup.get("coverage_kinds") or "")
        if action_type == "model_quality_review":
            outcome_signal = "model_quality_gate"
            source_observation_status = "generated_evidence"
            source_coverage_kind = "forecast_backtest"
        if action_type == "review_insight" and "developer_correlation" in set(group["insight_kind"].astype(str)):
            outcome_signal = "developer_correlation_lead"
            source_observation_status = "generated_evidence"
            source_coverage_kind = "direct_identity_same_window_overlap"
        rows.append(
            {
                "action_key": action_key,
                "priority_rank": 0,
                "urgency": urgency,
                "priority_score": round(priority_score, 2),
                "raw_priority_score": round(raw_priority_score, 2),
                "action_type": action_type,
                "decision_state": decision_state,
                "decision_gate_reason": decision_gate_reason,
                "subject_kind": subject_kind,
                "subject_key": subject_key,
                "insight_kinds": insight_kinds,
                "source_insight_keys": source_insight_keys,
                "source_link_insight_kinds": source_link_insight_kinds,
                "severity": severity_from_rank(int(group["severity_rank"].max())),
                "severity_rank": int(group["severity_rank"].max()),
                "status_signal": outcome_signal,
                "baseline_state": baseline_state,
                "current_state": current_state,
                "source_observation_status": source_observation_status,
                "source_auth_state": str(followup.get("auth_states") or ""),
                "source_coverage_kind": source_coverage_kind,
                "required_check_coverage_state": first_nonempty([followup.get("required_check_coverage_state")]),
                "required_check_match_state": first_nonempty([followup.get("required_check_match_state")]),
                "required_check_context_count": int_metric(followup.get("required_check_context_count")),
                "failing_required_context_count": int_metric(followup.get("failing_required_context_count")),
                "pending_required_context_count": int_metric(followup.get("pending_required_context_count")),
                "missing_required_context_count": int_metric(followup.get("missing_required_context_count")),
                "failing_required_contexts": first_nonempty([followup.get("failing_required_contexts")]),
                "pending_required_contexts": first_nonempty([followup.get("pending_required_contexts")]),
                "missing_required_contexts": first_nonempty([followup.get("missing_required_contexts")]),
                "failing_context_count": int_metric(followup.get("failing_context_count")),
                "pending_context_count": int_metric(followup.get("pending_context_count")),
                "failing_contexts": first_nonempty([followup.get("failing_contexts")]),
                "pending_contexts": first_nonempty([followup.get("pending_contexts")]),
                "title": action_title(action_type, top, subject_key, decision_state),
                "why_now": why_now(group, followup, reviews),
                "recommended_action": recommended_action(action_type, subject_key, owner_hint, source_observation_status, decision_state, insight_kinds),
                "owner_hint": owner_hint,
                "source_url": first_nonempty(group["source_url"]),
                "evidence_ref": evidence_ref(evidence_group),
                "score": round(float(group["score"].max()), 2),
                "confidence": round(float(group["confidence"].max()), 2),
                "needs_human_review": needs_human_review(action_type, reviews, group),
                "open_review_request_count": int(reviews.get("open_review_request_count") or 0),
                "reviewed_count": int(reviews.get("reviewed_count") or 0),
                "candidate_dismissed_kinds": ",".join(sorted(reviews.get("candidate_dismissed_kinds", set()))),
                "operational_dismissed_kinds": ",".join(sorted(reviews.get("operational_dismissed_kinds", set()))),
                "evidence_summary": evidence_summary(evidence_group),
                "generated_at": generated_at,
            }
        )

    out = pd.DataFrame(rows)
    if out.empty:
        return empty_action_items()
    out["source_observation_rank"] = out["source_observation_status"].map(source_observation_rank)
    out = out.sort_values(
        ["raw_priority_score", "severity_rank", "score", "source_observation_rank", "subject_key"],
        ascending=[False, False, False, False, True],
    ).reset_index(drop=True)
    out["priority_rank"] = out.index + 1
    out = out.drop(columns=["source_observation_rank"])
    return out


def choose_operating_insight_cards(base_insight_cards: pd.DataFrame, current_insight_cards: pd.DataFrame) -> pd.DataFrame:
    if not current_insight_cards.empty:
        current = current_insight_cards.copy()
        if "producer_state" in current.columns:
            current_rows = current[current["producer_state"].astype(str) == "current"].copy()
            if not current_rows.empty:
                return refresh_current_insight_cards_from_base(base_insight_cards, current_rows)
        return refresh_current_insight_cards_from_base(base_insight_cards, current)
    return base_insight_cards


def refresh_current_insight_cards_from_base(base_insight_cards: pd.DataFrame, current_insight_cards: pd.DataFrame) -> pd.DataFrame:
    if base_insight_cards.empty or current_insight_cards.empty:
        return current_insight_cards
    if not all(column in base_insight_cards.columns and column in current_insight_cards.columns for column in INSIGHT_CARD_IDENTITY_COLUMNS):
        return current_insight_cards

    base_by_key: dict[tuple[str, ...], dict[str, Any]] = {}
    for _, row in base_insight_cards.iterrows():
        base_by_key[insight_card_identity(row)] = row.to_dict()

    rows: list[dict[str, Any]] = []
    for _, current_row in current_insight_cards.iterrows():
        values = current_row.to_dict()
        base_row = base_by_key.get(insight_card_identity(current_row))
        if base_row is not None:
            for column, value in base_row.items():
                if column not in INSIGHT_CARD_STATE_COLUMNS:
                    values[column] = value
            for column in INSIGHT_CARD_STATE_COLUMNS:
                if column in current_row:
                    values[column] = current_row.get(column)
        rows.append(values)
    return pd.DataFrame(rows)


def insight_card_identity(row: pd.Series | dict[str, Any]) -> tuple[str, ...]:
    return tuple(normalize_card_identity_value(row.get(column, "")) for column in INSIGHT_CARD_IDENTITY_COLUMNS)


def normalize_card_identity_value(value: Any) -> str:
    if value is None:
        return ""
    try:
        if pd.isna(value):
            return ""
    except (TypeError, ValueError):
        pass
    return str(value)


def append_transition_resolution_actions(
    action_items: pd.DataFrame,
    transition_candidates: pd.DataFrame,
    pr_features: pd.DataFrame,
    generated_at: str,
    source_instance: str = "",
) -> pd.DataFrame:
    transition_candidates = source_scoped_transition_candidates(transition_candidates, source_instance)
    if transition_candidates.empty or "transition_kind" not in transition_candidates.columns:
        return action_items
    latest = latest_transition_candidates_by_subject(transition_candidates)
    terminal = latest[latest["transition_kind"] == "terminal_state_change"].copy()
    if terminal.empty:
        return action_items
    terminal["_confidence"] = pd.to_numeric(terminal.get("confidence", 0), errors="coerce").fillna(0.0)
    terminal = terminal.sort_values(["to_observed_at", "_confidence", "transition_key"], ascending=[False, False, True])
    pr_by_subject = pr_feature_by_subject(pr_features)
    existing_keys = set(action_items["action_key"].astype(str).tolist()) if not action_items.empty else set()
    existing_closeout_subjects = set()
    if not action_items.empty:
        closeout_rows = action_items[
            (action_items["action_type"].astype(str) == "verify_resolution")
            | (action_items["decision_state"].astype(str) == "closeout_review")
        ]
        existing_closeout_subjects = {
            (first_nonempty([row.get("subject_kind")]), first_nonempty([row.get("subject_key")]))
            for _, row in closeout_rows.iterrows()
        }
    rows: list[dict[str, Any]] = []
    for row in terminal.itertuples(index=False):
        action_key = f"tpm-action:{stable_digest(['verify_resolution', str(row.transition_key)])}"
        if action_key in existing_keys:
            continue
        subject_kind = str(row.subject_kind)
        subject_key = str(row.subject_key)
        if (subject_kind, subject_key) in existing_closeout_subjects:
            continue
        existing_keys.add(action_key)
        existing_closeout_subjects.add((subject_kind, subject_key))
        pr = pr_by_subject.get(subject_key, {})
        owner_hint = github_owner_hint(pr.get("author_login", ""))
        source_url = str(pr.get("pr_url") or "")
        title = str(pr.get("title") or f"Verify terminal transition: {subject_key}")
        confidence = safe_float(getattr(row, "confidence", 0.0))
        rows.append(
            {
                "action_key": action_key,
                "priority_rank": 0,
                "urgency": "medium",
                "priority_score": 65.0,
                "raw_priority_score": 65.0,
                "action_type": "verify_resolution",
                "decision_state": "closeout_review",
                "decision_gate_reason": "terminal transition requires human closeout confirmation",
                "subject_kind": subject_kind,
                "subject_key": subject_key,
                "insight_kinds": "state_transition",
                "source_insight_keys": str(row.transition_key),
                "source_link_insight_kinds": "state_transition",
                "severity": "medium",
                "severity_rank": 3,
                "status_signal": "terminal_transition_observed",
                "baseline_state": str(row.from_state or ""),
                "current_state": str(row.to_state or ""),
                "source_observation_status": "observed",
                "source_auth_state": "",
                "source_coverage_kind": "time_series_transition",
                "title": f"Verify terminal transition: {title}",
                "why_now": f"Live follow-up observed {subject_key} move from {row.from_state} to {row.to_state} between {row.from_observed_at} and {row.to_observed_at}.",
                "recommended_action": "Confirm this item should be closed out, attach the transition evidence, and remove any stale open-work escalation.",
                "owner_hint": owner_hint,
                "source_url": source_url,
                "evidence_ref": str(row.transition_key),
                "score": round(confidence * 100, 2),
                "confidence": confidence,
                "needs_human_review": "true",
                "open_review_request_count": 0,
                "reviewed_count": 0,
                "evidence_summary": f"state_transition: {row.from_state} -> {row.to_state}; {row.note}",
                "generated_at": generated_at,
            }
        )
    if not rows:
        return action_items
    additions = pd.DataFrame(rows)
    combined = pd.concat([action_items, additions], ignore_index=True) if not action_items.empty else additions
    return rerank_action_items(combined)


def apply_transition_evidence_to_resolution_actions(
    action_items: pd.DataFrame,
    transition_candidates: pd.DataFrame,
    source_instance: str = "",
) -> pd.DataFrame:
    transition_candidates = source_scoped_transition_candidates(transition_candidates, source_instance)
    if action_items.empty or transition_candidates.empty or "transition_kind" not in transition_candidates.columns:
        return action_items
    transitions = terminal_transition_evidence_by_subject(transition_candidates)
    if not transitions:
        return action_items
    out = action_items.copy()
    for idx, action in out.iterrows():
        action_type = first_nonempty([action.get("action_type")])
        decision_state = first_nonempty([action.get("decision_state")])
        if action_type != "verify_resolution" and decision_state != "closeout_review":
            continue
        subject = (first_nonempty([action.get("subject_kind")]), first_nonempty([action.get("subject_key")]))
        transition = transitions.get(subject)
        if transition is None:
            continue
        out.at[idx, "evidence_ref"] = f"state_transition {transition['transition_key']}"
        out.at[idx, "evidence_summary"] = transition["evidence_summary"]
        out.at[idx, "source_insight_keys"] = transition["transition_key"]
        out.at[idx, "source_link_insight_kinds"] = "state_transition"
        out.at[idx, "source_coverage_kind"] = "time_series_transition"
        out.at[idx, "status_signal"] = "terminal_transition_observed"
        out.at[idx, "baseline_state"] = transition["from_state"]
        out.at[idx, "current_state"] = transition["to_state"]
        out.at[idx, "why_now"] = transition["why_now"]
        out.at[idx, "recommended_action"] = "Confirm this item should be closed out, attach the transition evidence, and remove any stale open-work escalation."
    return out


def source_resolved_closeout_allowed(action: pd.Series, latest_subject_states: dict[tuple[str, str], str]) -> bool:
    action_type = first_nonempty([action.get("action_type")])
    if action_type != "verify_resolution":
        return False
    current_state = first_nonempty([action.get("current_state")]).lower()
    if current_state not in SOURCE_RESOLVED_TERMINAL_STATES:
        return False
    subject = (first_nonempty([action.get("subject_kind")]), first_nonempty([action.get("subject_key")]))
    if subject not in latest_subject_states:
        return False
    latest_state = first_nonempty([latest_subject_states.get(subject)]).lower()
    if latest_state != current_state:
        return False
    if first_nonempty([action.get("source_observation_status")]) != "observed":
        return False
    auth_state = first_nonempty([action.get("source_auth_state")]).lower()
    if auth_state not in SOURCE_RESOLVED_AUTH_STATES:
        return False
    coverage_kind = first_nonempty([action.get("source_coverage_kind")])
    if not coverage_kind:
        return False
    return True


def apply_source_resolved_closeouts(
    action_items: pd.DataFrame,
    latest_subject_states: dict[tuple[str, str], str],
) -> pd.DataFrame:
    if action_items.empty:
        return action_items
    out = action_items.copy()
    changed = False
    for idx, action in out.iterrows():
        if not source_resolved_closeout_allowed(action, latest_subject_states):
            continue
        subject_key = first_nonempty([action.get("subject_key")])
        current_state = first_nonempty([action.get("current_state")])
        priority_score = min(safe_float(action.get("priority_score")), 20.0)
        raw_priority_score = min(safe_float(action.get("raw_priority_score")), 20.0)
        out.at[idx, "decision_state"] = SOURCE_RESOLVED_DECISION
        out.at[idx, "decision_gate_reason"] = "authenticated source observed terminal state; closeout is source-resolved"
        out.at[idx, "needs_human_review"] = "false"
        out.at[idx, "recommended_action"] = f"No owner follow-up required for {subject_key}; source currently reports {current_state}."
        out.at[idx, "status_signal"] = "terminal_state_source_resolved"
        out.at[idx, "urgency"] = "low"
        out.at[idx, "priority_score"] = priority_score
        out.at[idx, "raw_priority_score"] = raw_priority_score
        changed = True
    if not changed:
        return action_items
    return rerank_action_items(out)


def append_forecast_risk_backstop_actions(
    action_items: pd.DataFrame,
    pr_features: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    generated_at: str,
) -> pd.DataFrame:
    if pr_features.empty:
        return action_items
    existing_subjects: set[tuple[str, str]] = set()
    if not action_items.empty:
        active = action_items[
            (action_items["action_type"].astype(str) != "dismissed_signal")
            & (action_items["decision_state"].astype(str) != "suppressed_signal")
        ]
        existing_subjects = {
            (first_nonempty([row.get("subject_kind")]), first_nonempty([row.get("subject_key")]))
            for _, row in active.iterrows()
        }
    eta_ready = forecast_effective_eta_ready(forecast_summary)
    risk_triage_ready = risk_triage_owner_followup_ready_from_forecast(forecast_summary)
    rows: list[dict[str, Any]] = []
    for _, pr in pr_features.iterrows():
        repository = first_nonempty([pr.get("repository")])
        pr_number = metric_row_int(pr, "pr_number")
        if not repository or pr_number <= 0:
            continue
        subject_key = f"{repository}#{pr_number}"
        subject = ("pull_request", subject_key)
        if subject in existing_subjects:
            continue
        if first_nonempty([pr.get("state")]) != "open":
            continue
        risk_band = first_nonempty([pr.get("risk_band")])
        if risk_band not in {"critical", "high"}:
            continue
        detail_state = first_nonempty([pr.get("source_current_detail_state")])
        if detail_state == "failed":
            continue
        risk_score = safe_float(pr.get("risk_score"))
        overdue_days = safe_float(pr.get("overdue_days"))
        priority_score = min(100.0, risk_score + min(overdue_days, 15.0))
        source_fields = pr_forecast_source_followup(pr, default_outcome="forecast_risk_uncovered")
        source_observation_status = first_nonempty([source_fields.get("source_observation_status")]) or source_status(source_fields)
        product_followup_allowed = risk_triage_ready and source_observation_status == "observed"
        decision_reason = "forecast risk has no existing open TPM action"
        if not eta_ready and product_followup_allowed:
            decision_reason += "; risk-triage backtest supports owner/status attention ordering; ETA forecast remains gated, so this is not an ETA commitment"
        elif not eta_ready:
            decision_reason += "; ETA forecast remains gated, so this is owner-status triage rather than an ETA commitment"
        source_url = first_nonempty([pr.get("pr_url")])
        title = first_nonempty([pr.get("title")]) or subject_key
        owner_hint = github_owner_hint(pr.get("author_login", ""))
        rows.append(
            {
                "action_key": f"tpm-action:{stable_digest(['forecast_backstop', subject_key])}",
                "priority_rank": 0,
                "urgency": priority_to_urgency(priority_score, "decision_or_owner_followup"),
                "priority_score": round(priority_score, 2),
                "raw_priority_score": round(priority_score, 2),
                "action_type": "decision_or_owner_followup",
                "decision_state": "product_action" if product_followup_allowed else "validation_lead",
                "decision_gate_reason": decision_reason,
                "subject_kind": "pull_request",
                "subject_key": subject_key,
                "insight_kinds": "forecast_risk",
                "source_insight_keys": "",
                "source_link_insight_kinds": "forecast_risk",
                "severity": risk_band,
                "severity_rank": SEVERITY_RANK.get(risk_band, 0),
                "status_signal": source_fields["outcome_signal"],
                "baseline_state": source_fields["baseline_state"],
                "current_state": source_fields["current_state"] or "open",
                "source_observation_status": source_observation_status,
                "source_auth_state": source_fields["auth_states"],
                "source_coverage_kind": source_fields["coverage_kinds"],
                "title": f"Route forecast risk owner follow-up: {title}" if product_followup_allowed else f"Validate forecast risk lead: {title}",
                "why_now": f"Open PR forecast risk is {risk_band} with score {risk_score:.1f} and {overdue_days:.1f}d over baseline, but no open TPM action was attached.",
                "recommended_action": f"Ask {owner_hint or 'the PR owner'} for merge, close, park, or reviewer status; keep this as risk triage, not an ETA commitment.",
                "owner_hint": owner_hint,
                "source_url": source_url,
                "evidence_ref": " ".join(piece for piece in ["tpm_pr_forecast", subject_key, source_url] if piece),
                "score": round(risk_score, 2),
                "confidence": 0.65,
                "needs_human_review": "true",
                "open_review_request_count": 0,
                "reviewed_count": 0,
                "candidate_dismissed_kinds": "",
                "operational_dismissed_kinds": "",
                "evidence_summary": f"forecast_risk: {risk_band}; score {risk_score:.1f}; overdue {overdue_days:.1f}d",
                "generated_at": generated_at,
            }
        )
    if not rows:
        return action_items
    additions = pd.DataFrame(rows, columns=empty_action_items().columns)
    combined = pd.concat([action_items, additions], ignore_index=True) if not action_items.empty else additions
    return rerank_action_items(combined)


def terminal_transition_evidence_by_subject(transition_candidates: pd.DataFrame) -> dict[tuple[str, str], dict[str, str]]:
    latest = latest_transition_candidates_by_subject(transition_candidates)
    terminal = latest[latest["transition_kind"].astype(str) == "terminal_state_change"].copy()
    if terminal.empty:
        return {}
    terminal["_confidence"] = pd.to_numeric(terminal.get("confidence", 0), errors="coerce").fillna(0.0)
    terminal = terminal.sort_values(["to_observed_at", "_confidence"], ascending=[False, False])
    out: dict[tuple[str, str], dict[str, str]] = {}
    for _, row in terminal.iterrows():
        subject = (first_nonempty([row.get("subject_kind")]), first_nonempty([row.get("subject_key")]))
        if not subject[0] or not subject[1] or subject in out:
            continue
        transition_key = first_nonempty([row.get("transition_key")])
        from_state = first_nonempty([row.get("from_state")])
        to_state = first_nonempty([row.get("to_state")])
        from_observed_at = first_nonempty([row.get("from_observed_at")])
        to_observed_at = first_nonempty([row.get("to_observed_at")])
        note = first_nonempty([row.get("note")])
        out[subject] = {
            "transition_key": transition_key,
            "from_state": from_state,
            "to_state": to_state,
            "evidence_summary": f"state_transition: {from_state} -> {to_state}; {note}",
            "why_now": f"Live follow-up observed {subject[1]} move from {from_state} to {to_state} between {from_observed_at} and {to_observed_at}.",
        }
    return out


def latest_transition_candidates_by_subject(transition_candidates: pd.DataFrame) -> pd.DataFrame:
    if transition_candidates.empty:
        return transition_candidates
    required = {"subject_kind", "subject_key", "to_observed_at", "transition_key"}
    if not required.issubset(set(transition_candidates.columns)):
        return transition_candidates
    ranked = transition_candidates.copy()
    ranked["_confidence"] = pd.to_numeric(ranked.get("confidence", 0), errors="coerce").fillna(0.0)
    ranked = ranked.sort_values(
        ["subject_kind", "subject_key", "to_observed_at", "_confidence", "transition_key"],
        ascending=[True, True, False, False, True],
    )
    latest = ranked.drop_duplicates(["subject_kind", "subject_key"], keep="first").copy()
    return latest.drop(columns=["_confidence"], errors="ignore")


def rerank_action_items(action_items: pd.DataFrame) -> pd.DataFrame:
    if action_items.empty:
        return empty_action_items()
    ranked = action_items.copy()
    ranked["source_observation_rank"] = ranked["source_observation_status"].map(source_observation_rank)
    ranked = ranked.sort_values(
        ["raw_priority_score", "severity_rank", "score", "source_observation_rank", "subject_key"],
        ascending=[False, False, False, False, True],
    ).reset_index(drop=True)
    ranked["priority_rank"] = ranked.index + 1
    return ranked.drop(columns=["source_observation_rank"])


def aggregate_followups(followups: pd.DataFrame) -> dict[tuple[str, str], dict[str, Any]]:
    if followups.empty:
        return {}
    rows: dict[tuple[str, str], dict[str, Any]] = {}
    data = followups.copy()
    data["fetch_status_code"] = pd.to_numeric(data["fetch_status_code"], errors="coerce")
    data["days_since_source_update_number"] = pd.to_numeric(data["days_since_source_update"], errors="coerce")
    for (subject_kind, subject_key), group in data.groupby(["subject_kind", "subject_key"], sort=False):
        outcome_values = [str(value) for value in group["outcome_signal"].dropna().tolist()]
        current_values = [str(value) for value in group["current_state"].dropna().tolist() if str(value)]
        baseline_values = [str(value) for value in group["baseline_state"].dropna().tolist() if str(value)]
        fetch_errors = [str(value) for value in group["fetch_error"].dropna().tolist() if str(value)]
        auth_states = {str(value) for value in group.get("fetch_auth_state", pd.Series(dtype=str)).dropna().tolist() if str(value)}
        coverage_kinds = {str(value) for value in group.get("fetch_coverage_kind", pd.Series(dtype=str)).dropna().tolist() if str(value)}
        rows[(subject_kind, subject_key)] = {
            "outcome_signal": choose_outcome_signal(outcome_values),
            "current_state": current_values[0] if current_values else "",
            "baseline_state": baseline_values[0] if baseline_values else "",
            "fetch_success_count": int((group["fetch_status_code"] == 200).sum()),
            "fetch_error_count": int((group["fetch_status_code"] != 200).sum()),
            "fetch_error": "; ".join(fetch_errors[:2]),
            "auth_states": ",".join(sorted(auth_states)),
            "coverage_kinds": ",".join(sorted(coverage_kinds)),
            "days_since_source_update": float(group["days_since_source_update_number"].max())
            if group["days_since_source_update_number"].notna().any()
            else None,
            "observed_at": first_nonempty(group["observed_at"]),
        }
    return rows


def aggregate_check_observations(check_observations: pd.DataFrame) -> dict[tuple[str, str], dict[str, Any]]:
    if check_observations.empty:
        return {}
    rows: dict[tuple[str, str], dict[str, Any]] = {}
    data = check_observations.copy()
    for column in ["pr_fetch_status_code", "check_fetch_status_code", "status_fetch_status_code"]:
        data[column] = pd.to_numeric(data.get(column, pd.Series(dtype=float)), errors="coerce")
    for column in ["pr_fetch_complete", "check_fetch_complete", "status_fetch_complete"]:
        if column not in data.columns:
            data[column] = False
        data[column] = data[column].map(lambda value: value is True or str(value).lower() in {"true", "1", "yes"})
    for _, row in data.iterrows():
        subject_key = first_nonempty([row.get("subject_key")])
        if not subject_key:
            continue
        state = first_nonempty([row.get("effective_state"), row.get("current_pr_state")])
        coverage_state = first_nonempty([row.get("source_coverage_state")])
        status_codes = [row.get("pr_fetch_status_code"), row.get("check_fetch_status_code"), row.get("status_fetch_status_code")]
        complete_flags = [bool(row.get("pr_fetch_complete")), bool(row.get("check_fetch_complete")), bool(row.get("status_fetch_complete"))]
        fetch_success_count = 1 if all(code == 200 for code in status_codes) and all(complete_flags) else 0
        fetch_error_count = 1 if coverage_state == "failed" or any(code != 200 for code in status_codes if not pd.isna(code)) else 0
        outcome_signal = "still_open" if state == "open" else "subject_became_terminal" if state in {"merged", "closed"} else "no_state_change"
        coverage_kinds = first_nonempty([row.get("fetch_coverage_kind")])
        if coverage_state:
            coverage_kinds = ",".join(part for part in [coverage_kinds, f"check_coverage:{coverage_state}"] if part)
        required_check_coverage_state = first_nonempty([row.get("required_check_coverage_state")])
        if required_check_coverage_state:
            coverage_kinds = ",".join(part for part in [coverage_kinds, f"required_check_coverage:{required_check_coverage_state}"] if part)
        rows[("pull_request", subject_key)] = {
            "outcome_signal": outcome_signal,
            "current_state": state,
            "baseline_state": first_nonempty([row.get("fixture_state")]),
            "fetch_success_count": fetch_success_count,
            "fetch_error_count": fetch_error_count,
            "fetch_error": "; ".join(
                value
                for value in [
                    first_nonempty([row.get("pr_fetch_error")]),
                    first_nonempty([row.get("check_fetch_error")]),
                    first_nonempty([row.get("status_fetch_error")]),
                ]
                if value
            ),
            "auth_states": first_nonempty([row.get("fetch_auth_state")]),
            "coverage_kinds": coverage_kinds,
            "coverage_states": coverage_state,
            "required_check_coverage_state": required_check_coverage_state,
            "required_check_match_state": first_nonempty([row.get("required_check_match_state")]),
            "required_check_context_count": int_metric(row.get("required_check_context_count")),
            "failing_required_context_count": int_metric(row.get("failing_required_context_count")),
            "pending_required_context_count": int_metric(row.get("pending_required_context_count")),
            "missing_required_context_count": int_metric(row.get("missing_required_context_count")),
            "failing_required_contexts": first_nonempty([row.get("failing_required_contexts")]),
            "pending_required_contexts": first_nonempty([row.get("pending_required_contexts")]),
            "missing_required_contexts": first_nonempty([row.get("missing_required_contexts")]),
            "failing_context_count": int_metric(row.get("failing_context_count")),
            "pending_context_count": int_metric(row.get("pending_context_count")),
            "failing_contexts": first_nonempty([row.get("failing_contexts")]),
            "pending_contexts": first_nonempty([row.get("pending_contexts")]),
            "days_since_source_update": None,
            "observed_at": first_nonempty([row.get("observed_at")]),
        }
    return rows


def aggregate_pr_source_coverage(pr_features: pd.DataFrame) -> dict[tuple[str, str], dict[str, Any]]:
    if pr_features.empty or "source_current_coverage_state" not in pr_features.columns:
        return {}
    rows: dict[tuple[str, str], dict[str, Any]] = {}
    for _, row in pr_features.iterrows():
        repository = first_nonempty([row.get("repository")])
        pr_number = metric_row_int(row, "pr_number")
        if not repository or pr_number <= 0:
            continue
        coverage_state = first_nonempty([row.get("source_current_coverage_state")])
        if coverage_state not in {"observed", "detail_failed", "coverage_limited"}:
            continue
        subject_key = f"{repository}#{pr_number}"
        rows[("pull_request", subject_key)] = pr_forecast_source_followup(row)
    return rows


def pr_forecast_source_followup(row: pd.Series, default_outcome: str = "") -> dict[str, Any]:
    state = first_nonempty([row.get("state")])
    coverage_state = first_nonempty([row.get("source_current_coverage_state")])
    detail_state = first_nonempty([row.get("source_current_detail_state")])
    issue_codes = first_nonempty([row.get("source_current_issue_codes")])
    issue_kinds = first_nonempty([row.get("source_current_issue_kinds")])
    failure_message = first_nonempty([row.get("source_current_failure_message")])
    if not coverage_state:
        return {
            "outcome_signal": default_outcome or "not_observed",
            "current_state": state if state == "open" else "",
            "baseline_state": state,
            "fetch_success_count": 0,
            "fetch_error_count": 0,
            "fetch_error": "",
            "auth_states": first_nonempty([row.get("source_visibility")]),
            "coverage_kinds": "forecast_risk_backstop" if default_outcome else "",
            "coverage_states": "generated" if default_outcome else "unknown",
            "source_observation_status": "generated_evidence" if default_outcome else "not_observed",
            "days_since_source_update": None,
            "observed_at": first_nonempty([row.get("source_current_observed_at")]),
        }
    coverage_kind_parts = [f"fixture_source_sync:{coverage_state}"]
    if detail_state:
        coverage_kind_parts.append(f"pr_detail:{detail_state}")
    coverage_states = ""
    fetch_success_count = 0
    fetch_error_count = 0
    outcome_signal = default_outcome
    if coverage_state == "observed":
        coverage_states = "complete"
        fetch_success_count = 1
        outcome_signal = outcome_signal or ("still_open" if state == "open" else "subject_became_terminal" if state in {"merged", "closed"} else "source_observed")
    elif coverage_state in {"detail_failed", "coverage_limited"}:
        coverage_states = "failed" if coverage_state == "detail_failed" else "partial"
        fetch_error_count = 1
        outcome_signal = "source_coverage_limited"
    else:
        coverage_states = "unknown"
        outcome_signal = outcome_signal or "not_observed"
    return {
        "outcome_signal": outcome_signal,
        "current_state": state if coverage_state == "observed" else "",
        "baseline_state": state,
        "fetch_success_count": fetch_success_count,
        "fetch_error_count": fetch_error_count,
        "fetch_error": "; ".join(part for part in [issue_codes, issue_kinds, failure_message] if part),
        "auth_states": first_nonempty([row.get("source_visibility")]),
        "coverage_kinds": ",".join(part for part in coverage_kind_parts if part),
        "coverage_states": coverage_states,
        "days_since_source_update": None,
        "observed_at": first_nonempty([row.get("source_current_observed_at")]),
    }


def has_successful_source_observation(followup: dict[str, Any]) -> bool:
    if not followup:
        return False
    return int(followup.get("fetch_success_count") or 0) > 0


def should_use_source_coverage_followup(followup: dict[str, Any], source_coverage: dict[str, Any]) -> bool:
    if not source_coverage:
        return False
    if not has_successful_source_observation(followup):
        return True
    followup_state = first_nonempty([followup.get("current_state")]).lower()
    source_state = first_nonempty([source_coverage.get("current_state")]).lower()
    if not followup_state or not source_state or followup_state == source_state:
        return False
    source_success = int(source_coverage.get("fetch_success_count") or 0) > 0
    if source_success and followup_state in SOURCE_RESOLVED_TERMINAL_STATES and source_state not in SOURCE_RESOLVED_TERMINAL_STATES:
        return True
    return False


def merge_source_coverage_followup(followup: dict[str, Any], source_coverage: dict[str, Any]) -> dict[str, Any]:
    if not followup:
        return source_coverage
    merged = dict(followup)
    merged.update(source_coverage)
    if followup.get("baseline_state") and not merged.get("baseline_state"):
        merged["baseline_state"] = followup.get("baseline_state")
    return merged


def aggregate_reviews(review_queue: pd.DataFrame) -> dict[tuple[str, str], dict[str, Any]]:
    if review_queue.empty:
        return {}
    rows: dict[tuple[str, str], dict[str, Any]] = {}
    for (subject_kind, subject_key), group in review_queue.groupby(["subject_kind", "subject_key"], sort=False):
        current_insight_count = int(group["insight_key"].nunique())
        labeled = effective_label_rows(group)
        operational_labeled = effective_label_rows(group, measurement_only=False)
        truth_labeled_keys = set(labeled[labeled["truth_label"].isin(["true_positive", "false_positive", "partial"])]["insight_key"])
        actionability_labeled_keys = set(labeled[labeled["actionability_label"].isin(POSITIVE_ACTIONABILITY | {"not_actionable"})]["insight_key"])
        needs_more_data_keys = set(labeled[(labeled["truth_label"] == "partial") | (labeled["review_state"] == "needs_more_data")]["insight_key"])
        resolved = labeled[
            labeled["review_state"].isin(RESOLVED_REVIEW_STATES)
            & labeled["truth_label"].isin(["true_positive", "false_positive"])
            & labeled["actionability_label"].isin(POSITIVE_ACTIONABILITY | {"not_actionable"})
        ]
        resolved_keys = set(resolved["insight_key"])
        open_request_count = max(0, current_insight_count - len(resolved_keys))
        current_by_kind = group.groupby("insight_kind")["insight_key"].nunique().to_dict()
        resolved_by_kind = resolved.groupby("insight_kind")["insight_key"].nunique().to_dict() if not resolved.empty else {}
        open_by_kind = {
            str(kind): max(0, int(count) - int(resolved_by_kind.get(kind, 0)))
            for kind, count in current_by_kind.items()
        }
        accepted_kinds = {
            str(row.insight_kind)
            for row in labeled.itertuples(index=False)
            if row.truth_label == "true_positive" and row.actionability_label in {"actionable", "needs_owner"}
        }
        partial_kinds = {
            str(row.insight_kind)
            for row in labeled.itertuples(index=False)
            if row.truth_label == "partial" or row.review_state == "needs_more_data"
        }
        measurement_dismissed_kinds = {
            str(row.insight_kind)
            for row in labeled.itertuples(index=False)
            if row.truth_label == "false_positive" or row.actionability_label == "not_actionable"
        }
        operational_partial_kinds = {
            str(row.insight_kind)
            for row in operational_labeled.itertuples(index=False)
            if row.truth_label == "partial" or row.review_state == "needs_more_data"
        }
        operational_dismissed_kinds = {
            str(row.insight_kind)
            for row in operational_labeled.itertuples(index=False)
            if row.truth_label == "false_positive" or row.actionability_label == "not_actionable"
        }
        candidate_dismissed_kinds = {
            str(row.insight_kind)
            for row in operational_labeled.itertuples(index=False)
            if (row.truth_label == "false_positive" or row.actionability_label == "not_actionable")
            and first_nonempty([getattr(row, "measurement_eligible", "")]) != "true"
        }
        dismissed_kinds = measurement_dismissed_kinds | operational_dismissed_kinds
        rows[(subject_kind, subject_key)] = {
            "open_review_request_count": open_request_count,
            "open_review_request_count_by_kind": open_by_kind,
            "reviewed_count": len(resolved_keys),
            "truth_labeled_count": len(truth_labeled_keys),
            "actionability_labeled_count": len(actionability_labeled_keys),
            "needs_more_data_count": len(needs_more_data_keys),
            "positive_actionability_count": int(labeled["actionability_label"].isin(POSITIVE_ACTIONABILITY).sum()),
            "positive_truth_count": int(labeled["truth_label"].isin(["true_positive", "partial"]).sum()),
            "accepted_kinds": accepted_kinds,
            "partial_kinds": partial_kinds | operational_partial_kinds,
            "dismissed_kinds": dismissed_kinds,
            "measurement_dismissed_kinds": measurement_dismissed_kinds,
            "operational_dismissed_kinds": operational_dismissed_kinds,
            "candidate_dismissed_kinds": candidate_dismissed_kinds,
        }
    return rows


def build_decision_gates(readiness: pd.DataFrame, forecast_summary: pd.DataFrame) -> dict[str, Any]:
    readiness_map = metric_map(readiness)
    forecast_map = metric_map(forecast_summary)
    return {
        "precision_ready": readiness_map.get("ready_to_measure_precision") == "true",
        "actionability_ready": readiness_map.get("ready_to_measure_actionability") == "true",
        "truth_label_coverage": readiness_map.get("truth_label_coverage", ""),
        "actionability_label_coverage": readiness_map.get("actionability_label_coverage", ""),
        "kind_readiness": kind_readiness_map(readiness),
        "eta_forecast_ready": forecast_effective_eta_ready(forecast_summary),
        "risk_triage_owner_followup_ready": risk_triage_owner_followup_ready_from_forecast(forecast_summary),
        "risk_triage_lift_at_10pct": forecast_map.get("risk_triage_lift_at_10pct", ""),
    }


def risk_triage_owner_followup_ready_from_forecast(forecast_summary: pd.DataFrame) -> bool:
    forecast_map = metric_map(forecast_summary)
    lift = safe_float(forecast_map.get("risk_triage_lift_at_10pct"))
    return lift > MIN_RISK_TRIAGE_LIFT_FOR_OWNER_FOLLOWUP


def decision_state_for_action(
    action_type: str,
    group: pd.DataFrame,
    reviews: dict[str, Any],
    gates: dict[str, Any],
    followup: dict[str, Any] | None = None,
) -> tuple[str, str]:
    kinds = set(group["insight_kind"].astype(str)) if not group.empty else set()
    if action_type == "dismissed_signal":
        return "suppressed_signal", "dismissed by non-measurement or measurement review context"
    if action_type == "refresh_source":
        return "source_repair", "source coverage failed or is too sparse for product claims"
    if action_type == "verify_resolution":
        return "closeout_review", "observed terminal state change still needs closeout confirmation"
    if action_type == "model_quality_review" or "model_quality" in kinds:
        return "model_or_rule_qa", "model quality gate is about readiness, not product escalation"
    if "developer_correlation" in kinds:
        return (
            "validation_lead",
            "developer correlation is workload context only; it cannot support causality, ownership, performance, or product-action claims",
        )
    if action_type == "coordinate_workstream" and "dependency_cluster" in kinds:
        return (
            "validation_lead",
            "dependency clusters are topology context until a concrete blocking dependency or owner-confirmed coordination action is sourced",
        )
    if action_type == "ci_check_followup":
        required_ready, required_reason = required_ci_gate(followup or {})
        if not required_ready:
            return "validation_lead", required_reason
        ready, gate_reason = measurement_ready_for_action(action_type, group, gates)
        if not ready:
            return "validation_lead", gate_reason
        open_review_kinds = unresolved_review_kinds_for_action(action_type, group, reviews)
        if open_review_kinds:
            return "validation_lead", f"current action kind still has open truth/actionability review requests: {', '.join(open_review_kinds)}"
        return "product_action", required_reason
    if action_type == "decision_or_owner_followup" and "forecast_risk" in kinds and not bool(gates.get("eta_forecast_ready")):
        if bool(gates.get("risk_triage_owner_followup_ready")):
            if source_status(followup or {}) != "observed":
                return (
                    "validation_lead",
                    "risk-triage owner follow-up needs observed current source state before product routing; ETA forecast remains gated",
                )
            return "product_action", risk_triage_owner_followup_gate_reason(gates)
        return (
            "validation_lead",
            "eta_forecast_ready=false and risk-triage backtest is not ready for owner/status attention ordering",
        )
    ready, gate_reason = measurement_ready_for_action(action_type, group, gates)
    if not ready:
        return "validation_lead", gate_reason
    open_review_kinds = unresolved_review_kinds_for_action(action_type, group, reviews)
    if open_review_kinds:
        return "validation_lead", f"current action kind still has open truth/actionability review requests: {', '.join(open_review_kinds)}"
    return "product_action", product_action_gate_reason(action_type, action_measurement_kinds(action_type, group), gate_reason, gates)


def product_action_gate_reason(action_type: str, kinds: set[str], measurement_reason: str, gates: dict[str, Any]) -> str:
    if "forecast_risk" in kinds:
        if bool(gates.get("eta_forecast_ready")):
            return "measurement gate passed for forecast action kind(s); ETA forecast gate passed"
        if bool(gates.get("risk_triage_owner_followup_ready")):
            return risk_triage_owner_followup_gate_reason(gates)
        return "forecast risk cannot become a product action until ETA forecast readiness passes"
    if not bool(gates.get("eta_forecast_ready")):
        return f"{measurement_reason}; ETA forecast gate is not required for non-forecast action type {action_type}"
    return f"{measurement_reason}; applicable action gates passed"


def risk_triage_owner_followup_gate_reason(gates: dict[str, Any]) -> str:
    lift = first_nonempty([gates.get("risk_triage_lift_at_10pct")])
    lift_phrase = f" lift_at_10pct={lift};" if lift else ""
    return (
        "risk-triage backtest supports attention ordering;"
        f"{lift_phrase} eta_forecast_ready=false, so allowed use is owner/status follow-up only, not ETA"
    )


def required_ci_gate(followup: dict[str, Any]) -> tuple[bool, str]:
    match_state = first_nonempty([followup.get("required_check_match_state")])
    coverage_state = first_nonempty([followup.get("required_check_coverage_state")])
    failing_count = int_metric(followup.get("failing_required_context_count"))
    pending_count = int_metric(followup.get("pending_required_context_count"))
    missing_count = int_metric(followup.get("missing_required_context_count"))
    if match_state == "required_context_failing_or_pending":
        count = failing_count + pending_count
        return True, f"{count} required check context(s) are failing or pending"
    if match_state == "required_context_missing":
        return True, f"{missing_count} required check context(s) are missing from the head SHA"
    if match_state == "required_contexts_successful":
        return False, "required check contexts are observed and successful"
    if match_state == "no_required_contexts_configured":
        return False, "required-check metadata was observed and no required contexts are configured"
    if coverage_state:
        return False, f"required-check coverage is not product-actionable: {coverage_state}"
    return False, "required-check coverage is unavailable; failing checks remain leads, not merge blockers"


def action_measurement_kinds(action_type: str, group: pd.DataFrame) -> set[str]:
    kinds = set(group["insight_kind"].astype(str)) if not group.empty and "insight_kind" in group.columns else set()
    if action_type in {"clear_blocker", "validate_signal"} and "blocker_candidate" in kinds:
        return {"blocker_candidate"}
    if action_type in {"ci_check_followup", "review_wait_followup"} and "status_summary" in kinds:
        return {"status_summary"}
    if action_type == "decision_or_owner_followup" and "forecast_risk" in kinds:
        return {"forecast_risk"}
    if action_type == "coordinate_workstream" and "dependency_cluster" in kinds:
        return {"dependency_cluster"}
    return kinds


def measurement_ready_for_action(action_type: str, group: pd.DataFrame, gates: dict[str, Any]) -> tuple[bool, str]:
    kinds = action_measurement_kinds(action_type, group)
    kind_readiness = gates.get("kind_readiness") if isinstance(gates.get("kind_readiness"), dict) else {}
    if kind_readiness and kinds:
        blocked: list[str] = []
        for kind in sorted(kinds):
            row = kind_readiness.get(kind, {})
            required = int(row.get("required") or 0)
            truth_labeled = int(row.get("truth_labeled") or 0)
            actionability_labeled = int(row.get("actionability_labeled") or 0)
            ready_for_product_action = bool(row.get("ready_for_product_action"))
            gate_state = first_nonempty([row.get("product_action_gate_state")])
            if required <= 0:
                blocked.append(f"{kind}=missing")
            elif truth_labeled < required or actionability_labeled < required:
                blocked.append(f"{kind}=truth {truth_labeled}/{required}, actionability {actionability_labeled}/{required}")
            elif not ready_for_product_action:
                reason = first_nonempty([row.get("product_action_gate_reason")])
                blocked.append(f"{kind}=product_action_gate {gate_state or 'gated'}" + (f" ({reason})" if reason else ""))
        if blocked:
            return False, "measurement gate is not ready for action kind(s): " + "; ".join(blocked)
        return True, "measurement gate passed for action kind(s)"

    if not bool(gates.get("precision_ready")) or not bool(gates.get("actionability_ready")):
        truth = first_nonempty([gates.get("truth_label_coverage")]) or "unknown"
        actionability = first_nonempty([gates.get("actionability_label_coverage")]) or "unknown"
        return (
            False,
            f"measurement gate is not ready: truth_label_coverage={truth}, actionability_label_coverage={actionability}",
        )
    return True, "measurement gate passed"


def unresolved_review_kinds_for_action(action_type: str, group: pd.DataFrame, reviews: dict[str, Any]) -> list[str]:
    kinds = action_measurement_kinds(action_type, group)
    open_by_kind = reviews.get("open_review_request_count_by_kind", {})
    if isinstance(open_by_kind, dict) and kinds:
        return sorted(kind for kind in kinds if int(open_by_kind.get(kind) or 0) > 0)
    if int(reviews.get("open_review_request_count") or 0) > 0:
        return sorted(kinds) or ["unknown"]
    return []


def effective_label_rows(rows: pd.DataFrame, measurement_only: bool = True) -> pd.DataFrame:
    if rows.empty:
        return rows
    label_rows = rows[rows["review_kind"].isin(["human_assessment", "evaluation_label"])].copy()
    if label_rows.empty:
        return label_rows
    if measurement_only and "measurement_eligible" in label_rows.columns:
        label_rows = label_rows[label_rows["measurement_eligible"] == "true"].copy()
    return dedupe_review_labels(label_rows)


def dedupe_review_labels(rows: pd.DataFrame) -> pd.DataFrame:
    if rows.empty:
        return rows
    ranked = rows.copy()
    if "label_quality" not in ranked.columns:
        ranked["label_quality"] = ranked.apply(infer_label_quality_from_review, axis=1)
    ranked["_quality_rank"] = ranked["label_quality"].map(lambda value: {"gold": 4, "adversarial": 3, "candidate": 2, "smoke": 1}.get(str(value), 0))
    ranked["_review_kind_rank"] = ranked["review_kind"].map(lambda value: 3 if value == "human_assessment" else 1)
    ranked["_review_id"] = pd.to_numeric(ranked.get("review_id", 0), errors="coerce").fillna(0)
    if "reviewed_at" not in ranked.columns:
        ranked["reviewed_at"] = ""
    ranked = ranked.sort_values(["insight_key", "_quality_rank", "_review_kind_rank", "reviewed_at", "_review_id"])
    return ranked.drop_duplicates("insight_key", keep="last").drop(columns=["_quality_rank", "_review_kind_rank", "_review_id"])


def build_owner_hints(pr_features: pd.DataFrame, ticket_features: pd.DataFrame) -> dict[tuple[str, str], str]:
    hints: dict[tuple[str, str], str] = {}
    if not pr_features.empty:
        for row in pr_features.itertuples(index=False):
            author = getattr(row, "author_login", "")
            if author:
                hints[("pull_request", f"{row.repository}#{int(row.pr_number)}")] = f"github:{author}"
    if not ticket_features.empty:
        for row in ticket_features.itertuples(index=False):
            assignee = getattr(row, "assignee", "")
            if assignee:
                hints[("ticket", str(row.ticket_key).upper())] = str(assignee)
    return hints


def choose_action_type(group: pd.DataFrame, followup: dict[str, Any], reviews: dict[str, Any], gates: dict[str, Any] | None = None) -> str:
    gates = gates or {}
    outcome = str(followup.get("outcome_signal") or "")
    current_state = str(followup.get("current_state") or "")
    kinds = set(group["insight_kind"].astype(str))
    dismissed_kinds = set(reviews.get("dismissed_kinds", set()))
    active_kinds = kinds - dismissed_kinds
    if dismissed_kinds and not active_kinds:
        return "dismissed_signal"
    if "model_quality" in active_kinds:
        return "model_quality_review"
    if source_status(followup) == "source_failure":
        return "refresh_source"
    if outcome in {"subject_became_terminal", "subject_became_closed"} or current_state in {"merged", "closed"}:
        return "verify_resolution"
    if "blocker_candidate" in active_kinds and "blocker_candidate" in reviews.get("accepted_kinds", set()):
        ready, _ = measurement_ready_for_action("clear_blocker", group, gates)
        if ready:
            return "clear_blocker"
        return "validate_signal"
    if "status_summary" in active_kinds and is_ci_check_group(group):
        return "ci_check_followup"
    if "status_summary" in active_kinds:
        return "review_wait_followup"
    if "blocker_candidate" in active_kinds:
        if "blocker_candidate" in reviews.get("partial_kinds", set()) or "blocker_candidate" not in dismissed_kinds:
            return "validate_signal"
    if "forecast_risk" in active_kinds:
        if "forecast_risk" not in dismissed_kinds:
            return "decision_or_owner_followup"
    if "dependency_cluster" in active_kinds:
        return "coordinate_workstream"
    if dismissed_kinds:
        return "dismissed_signal"
    return "review_insight"


def is_ci_check_group(group: pd.DataFrame) -> bool:
    if group.empty:
        return False
    identity_values = {str(value) for value in group.get("identity_key", pd.Series(dtype=str)).dropna().tolist()}
    method_values = {str(value) for value in group.get("model_method", pd.Series(dtype=str)).dropna().tolist()}
    title_values = {str(value) for value in group.get("title", pd.Series(dtype=str)).dropna().tolist()}
    return (
        "ci_check_state" in identity_values
        or any(value.startswith("ci_check_state") for value in method_values)
        or any("CI check state" in value for value in title_values)
    )


def compute_priority_score(group: pd.DataFrame, followup: dict[str, Any], action_type: str, clamp: bool = True) -> float:
    score = float(group["score"].max())
    if action_type == "review_wait_followup":
        status_rows = group[group["insight_kind"] == "status_summary"] if "insight_kind" in group.columns else group
        raw = float(status_rows["score"].max()) if not status_rows.empty else score
        return min(75.0, max(0.0, raw)) if clamp else raw
    if action_type == "ci_check_followup":
        status_rows = group[group["insight_kind"] == "status_summary"] if "insight_kind" in group.columns else group
        raw = float(status_rows["score"].max()) if not status_rows.empty else score
        return min(95.0, max(0.0, raw)) if clamp else raw
    if action_type == "model_quality_review":
        quality_rows = group[group["insight_kind"] == "model_quality"] if "insight_kind" in group.columns else group
        raw = float(quality_rows["score"].max()) if not quality_rows.empty else score
        return min(80.0, max(0.0, raw)) if clamp else raw
    if action_type == "verify_resolution":
        raw = max(35.0, score * 0.45)
        return min(65.0, raw) if clamp else raw
    if action_type == "refresh_source":
        raw = max(70.0, score + 10.0)
        return min(100.0, raw) if clamp else raw
    kinds = set(group["insight_kind"].astype(str))
    if "blocker_candidate" in kinds:
        score += 10.0
    if "forecast_risk" in kinds:
        score += 5.0
    if "dependency_cluster" in kinds:
        score += 3.0
    days_since_update = followup.get("days_since_source_update")
    if isinstance(days_since_update, float) and not math.isnan(days_since_update):
        if days_since_update >= 30:
            score += 10.0
        elif days_since_update >= 14:
            score += 6.0
        elif days_since_update >= 7:
            score += 3.0
    if clamp:
        return min(100.0, max(0.0, score))
    return max(0.0, score)


def source_observation_rank(status: Any) -> int:
    return {
        "observed": 4,
        "generated_evidence": 4,
        "observed_anonymous": 3,
        "observed_partial": 2,
        "not_observed": 2,
        "source_failure": 1,
    }.get(str(status), 0)


def priority_to_urgency(priority_score: float, action_type: str) -> str:
    if action_type == "dismissed_signal":
        return "low"
    if action_type == "verify_resolution":
        return "medium"
    if action_type == "validate_signal":
        return "high" if priority_score >= 85 else "medium"
    if action_type == "review_wait_followup":
        return "high" if priority_score >= 70 else "medium" if priority_score >= 50 else "low"
    if action_type == "ci_check_followup":
        return "high" if priority_score >= 80 else "medium" if priority_score >= 55 else "low"
    if action_type == "model_quality_review":
        return "medium" if priority_score >= 60 else "low"
    if priority_score >= 90:
        return "critical"
    if priority_score >= 75:
        return "high"
    if priority_score >= 50:
        return "medium"
    return "low"


def action_title(action_type: str, top: pd.Series, subject_key: str, decision_state: str = "") -> str:
    raw_title = str(top.get("title") or subject_key)
    if action_type == "verify_resolution":
        return f"Verify resolved follow-up: {subject_key}"
    if action_type == "clear_blocker":
        return f"Clear blocker candidate: {subject_key}"
    if action_type == "validate_signal":
        return f"Validate generated signal: {subject_key}"
    if action_type == "decision_or_owner_followup":
        if decision_state == "validation_lead":
            return f"Validate risk lead before owner decision: {subject_key}"
        return f"Decide path for at-risk PR: {subject_key}"
    if action_type == "review_wait_followup":
        return f"Confirm requested reviewer lead: {subject_key}"
    if action_type == "ci_check_followup":
        return f"Review CI check state: {subject_key}"
    if action_type == "model_quality_review":
        return "Review forecast model quality"
    if action_type == "coordinate_workstream":
        return f"Coordinate dependency cluster: {subject_key}"
    if action_type == "refresh_source":
        return f"Refresh source before acting: {subject_key}"
    if action_type == "dismissed_signal":
        return f"Dismiss labeled signal: {subject_key}"
    return raw_title


def why_now(group: pd.DataFrame, followup: dict[str, Any], reviews: dict[str, Any]) -> str:
    fragments: list[str] = []
    outcome = str(followup.get("outcome_signal") or "not_observed")
    current_state = str(followup.get("current_state") or "")
    if outcome == "not_observed":
        fragments.append("No live follow-up observation is available yet.")
    elif current_state:
        fragments.append(f"Latest observed state is {current_state} with outcome {outcome}.")
    else:
        fragments.append(f"Latest observation outcome is {outcome}.")
    days_since_update = followup.get("days_since_source_update")
    if isinstance(days_since_update, float) and not math.isnan(days_since_update):
        fragments.append(f"Source last updated {days_since_update:.2f} days before observation.")
    kinds = ", ".join(sorted(group["insight_kind"].astype(str).unique()))
    severity = severity_from_rank(int(group["severity_rank"].max()))
    fragments.append(f"Generated insight mix: {kinds}; max severity {severity}; max score {float(group['score'].max()):.2f}.")
    open_reviews = int(reviews.get("open_review_request_count") or 0)
    if open_reviews:
        fragments.append(f"{open_reviews} review request(s) still need truth/actionability assessment.")
    return " ".join(fragments)


def recommended_action(action_type: str, subject_key: str, owner_hint: str, source_observation_status: str, decision_state: str = "", insight_kinds: str = "") -> str:
    owner_phrase = f" with {owner_hint}" if owner_hint else ""
    if action_type == "verify_resolution":
        return f"Confirm {subject_key} really resolved, then record a human assessment and close or supersede stale follow-up."
    if action_type == "refresh_source":
        return f"Refresh source data for {subject_key}; do not make absence or completion claims until coverage is restored."
    if action_type == "clear_blocker":
        return f"Ask for blocker status{owner_phrase}, capture the concrete next step, and label whether the blocker candidate was actionable."
    if action_type == "validate_signal":
        if source_observation_status == "observed_anonymous":
            return f"Review public source evidence for {subject_key}, label truth/actionability first, then escalate to an owner only if the signal is confirmed."
        return f"Review source evidence for {subject_key}, label truth/actionability first, then escalate to an owner only if the signal is confirmed."
    if action_type == "decision_or_owner_followup":
        if decision_state == "validation_lead":
            return f"Gold-label the risk/actionability for {subject_key}; keep this as a validation lead until forecast and measurement gates pass."
        if "forecast_risk" in split_csv(insight_kinds):
            return f"Ask {owner_hint or 'the PR owner'} for merge, close, park, or reviewer status; keep this as risk triage, not an ETA commitment."
        return f"Confirm owner{owner_phrase}, decide merge/close/park path, and record the decision as review evidence."
    if action_type == "review_wait_followup":
        return f"Confirm whether the requested reviewer is still expected{owner_phrase}; record reviewer owner or merge/close decision."
    if action_type == "ci_check_followup":
        if decision_state == "validation_lead":
            return f"Review failing or pending GitHub checks{owner_phrase}; label whether they are required/merge-blocking before escalating as product work."
        return f"Review failing or pending GitHub checks{owner_phrase}; record whether they block merge and who owns the fix or decision."
    if action_type == "model_quality_review":
        return "Keep ETA use gated by backtest quality; collect time-series snapshots and labeled outcomes before treating forecasts as commitments."
    if action_type == "coordinate_workstream":
        return f"Split linked work into owners and dependency edges; record which dependency is blocking progress."
    if action_type == "dismissed_signal":
        return f"No product escalation for {subject_key}; retain the label as QA/audit evidence and suppress this signal in operating follow-up."
    if action_type == "review_insight" and "developer_correlation" in split_csv(insight_kinds):
        return f"Review same-window Jira workload context for {subject_key}; label whether it changes routing, capacity, or escalation, not ownership or performance."
    if source_observation_status != "observed":
        return f"Review {subject_key} only after source coverage is understood."
    return f"Review {subject_key} and record outcome labels."


def evidence_ref(group: pd.DataFrame) -> str:
    rows = group.copy()
    if "evidence_excerpt" in rows.columns:
        rows["has_source_evidence"] = rows["evidence_excerpt"].map(lambda value: bool(first_nonempty([value])))
    else:
        rows["has_source_evidence"] = False
    for _, row in rows.sort_values(["has_source_evidence", "severity_rank", "score"], ascending=[False, False, False]).iterrows():
        locator_kind = first_nonempty([row.get("evidence_locator_kind", "")])
        evidence_source_url = first_nonempty([row.get("evidence_source_url", "")])
        subject_url = first_nonempty([row.get("source_url", "")])
        span_start = format_span_value(row.get("evidence_span_start", ""))
        span_end = format_span_value(row.get("evidence_span_end", ""))
        span_key = first_nonempty([row.get("evidence_source_span_key", "")])
        if locator_kind or evidence_source_url or span_key:
            span = f"{span_start}-{span_end}" if span_start and span_end else ""
            source_url = evidence_source_url or subject_url
            pieces = [piece for piece in [locator_kind, span, span_key, source_url] if piece]
            return " ".join(pieces)
    subject_url = first_nonempty(group["source_url"]) if "source_url" in group.columns else ""
    return f"analytics_output subject {subject_url}" if subject_url else "analytics_output"


def group_for_action_evidence(action_type: str, group: pd.DataFrame, reviews: dict[str, Any]) -> pd.DataFrame:
    if action_type == "decision_or_owner_followup" and "forecast_risk" in set(group["insight_kind"].astype(str)):
        return group[group["insight_kind"] == "forecast_risk"]
    if action_type == "review_wait_followup" and "status_summary" in set(group["insight_kind"].astype(str)):
        return group[group["insight_kind"] == "status_summary"]
    if action_type == "ci_check_followup" and "status_summary" in set(group["insight_kind"].astype(str)):
        return group[group["insight_kind"] == "status_summary"]
    if action_type == "model_quality_review" and "model_quality" in set(group["insight_kind"].astype(str)):
        return group[group["insight_kind"] == "model_quality"]
    if action_type in {"clear_blocker", "validate_signal"} and "blocker_candidate" in set(group["insight_kind"].astype(str)):
        return group[group["insight_kind"] == "blocker_candidate"]
    if action_type == "dismissed_signal" and reviews.get("dismissed_kinds"):
        dismissed = group[group["insight_kind"].isin(reviews.get("dismissed_kinds", set()))]
        if not dismissed.empty:
            return dismissed
    return group


def group_for_action_source_links(action_type: str, group: pd.DataFrame, evidence_group: pd.DataFrame) -> pd.DataFrame:
    if group.empty:
        return group
    if action_type == "ci_check_followup":
        kinds = {"status_summary", "forecast_risk"}
        linked = group[group["insight_kind"].isin(kinds)]
        return linked if not linked.empty else evidence_group
    return evidence_group if not evidence_group.empty else group


def format_span_value(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, float) and math.isnan(value):
        return ""
    try:
        numeric = float(value)
    except (TypeError, ValueError):
        return str(value)
    if numeric.is_integer():
        return str(int(numeric))
    return str(value)


def evidence_summary(group: pd.DataFrame) -> str:
    summaries = []
    rows = group.copy()
    if "evidence_excerpt" in rows.columns:
        rows["has_source_evidence"] = rows["evidence_excerpt"].map(lambda value: bool(first_nonempty([value])))
    else:
        rows["has_source_evidence"] = False
    for _, row in rows.sort_values(["has_source_evidence", "severity_rank", "score"], ascending=[False, False, False]).head(3).iterrows():
        excerpt = first_nonempty([row.get("evidence_excerpt", "")])
        locator_kind = first_nonempty([row.get("evidence_locator_kind", "")])
        if excerpt:
            locator_phrase = f" [{locator_kind}]" if locator_kind else ""
            summaries.append(f"{row['insight_kind']}{locator_phrase}: {excerpt}")
            continue
        summaries.append(f"{row['insight_kind']}: {row['details']}")
    return " | ".join(summaries)


def needs_human_review(action_type: str, reviews: dict[str, Any], group: pd.DataFrame | None = None) -> str:
    if action_type in {"refresh_source", "dismissed_signal"}:
        return "false"
    if group is not None and unresolved_review_kinds_for_action(action_type, group, reviews):
        return "true"
    if group is None and int(reviews.get("open_review_request_count") or 0) > 0:
        return "true"
    if action_type in {"verify_resolution", "model_quality_review"}:
        return "true"
    return "false"


def build_summary(action_items: pd.DataFrame, readiness: pd.DataFrame) -> pd.DataFrame:
    rows = []
    if action_items.empty:
        rows.append({"metric": "action_item_count", "value": "0", "note": "no generated action items"})
        return pd.DataFrame(rows)
    decision_state = action_items.get("decision_state", pd.Series(dtype=str)).astype(str)
    product_actions = action_items[decision_state == "product_action"]
    validation_leads = action_items[decision_state == "validation_lead"]
    model_or_rule_qa = action_items[decision_state == "model_or_rule_qa"]
    source_resolved = action_items[decision_state == SOURCE_RESOLVED_DECISION]
    source_repairs = action_items[action_items["action_type"] == "refresh_source"]
    dismissed = action_items[action_items["action_type"] == "dismissed_signal"]
    candidate_dismissed_mask = action_items["candidate_dismissed_kinds"].fillna("").astype(str).str.strip() != ""
    candidate_dismissed = action_items[candidate_dismissed_mask]
    source_failed = action_items["source_observation_status"].isin(["source_failure", "source_failed"])
    source_partial = action_items["source_coverage_kind"].astype(str).str.contains("partial|failed", case=False, na=False)
    anonymous_observed = action_items["source_observation_status"] == "observed_anonymous"
    rows.extend(
        [
            {"metric": "action_item_count", "value": str(len(action_items)), "note": "subject-level TPM action items"},
            {
                "metric": "critical_or_high_count",
                "value": str(int(product_actions["urgency"].isin(["critical", "high"]).sum())),
                "note": "product-safe actions that should be inspected first",
            },
            {
                "metric": "open_work_count",
                "value": str(len(product_actions)),
                "note": "product-safe follow-up work",
            },
            {
                "metric": "validation_lead_count",
                "value": str(len(validation_leads)),
                "note": "unmeasured leads that need label or source validation before product escalation",
            },
            {
                "metric": "critical_or_high_validation_lead_count",
                "value": str(int(validation_leads["urgency"].isin(["critical", "high"]).sum())),
                "note": "urgent validation leads, not product-safe actions",
            },
            {
                "metric": "model_or_rule_qa_count",
                "value": str(len(model_or_rule_qa)),
                "note": "model or rule-quality work that should not be product escalation",
            },
            {
                "metric": "source_repair_count",
                "value": str(len(source_repairs)),
                "note": "items that require source refresh before product action",
            },
            {
                "metric": "verify_resolution_count",
                "value": str(int((action_items["action_type"] == "verify_resolution").sum())),
                "note": "items whose source state became terminal and need closure handling",
            },
            {
                "metric": "source_resolved_count",
                "value": str(len(source_resolved)),
                "note": "terminal closeouts resolved directly from authenticated current source state",
            },
            {
                "metric": "dismissed_signal_count",
                "value": str(len(dismissed)),
                "note": "false/not-actionable signals retained for QA/audit but excluded from product follow-up",
            },
            {
                "metric": "candidate_dismissed_component_count",
                "value": str(len(candidate_dismissed)),
                "note": "subjects with smoke/candidate/adversarial-dismissed insight components excluded from operating escalation without counting as measurement labels",
            },
            {
                "metric": "source_failure_count",
                "value": str(int(source_failed.sum())),
                "note": "items blocked by failed follow-up source reads",
            },
            {
                "metric": "coverage_limited_count",
                "value": str(int((source_failed | source_partial).sum())),
                "note": "items whose latest source observation failed, was partial, or is generated-only; anonymous successes are counted separately",
            },
            {
                "metric": "anonymous_observation_count",
                "value": str(int(anonymous_observed.sum())),
                "note": "items observed through anonymous/public APIs",
            },
            {
                "metric": "source_partial_count",
                "value": str(int(source_partial.sum())),
                "note": "items with partial, failed, or generated-only source coverage markers",
            },
            {
                "metric": "human_review_required_count",
                "value": str(int((action_items["needs_human_review"] == "true").sum())),
                "note": "items needing truth/actionability assessment before measuring quality",
            },
            {
                "metric": "ci_check_followup_count",
                "value": str(int((action_items["action_type"] == "ci_check_followup").sum())),
                "note": "items generated from observed failing or pending GitHub check/status payloads",
            },
            {
                "metric": "model_quality_review_count",
                "value": str(int((action_items["action_type"] == "model_quality_review").sum())),
                "note": "items about model readiness rather than product escalation",
            },
        ]
    )
    readiness_map = metric_map(readiness)
    for metric in ["ready_to_measure_precision", "ready_to_measure_actionability", "truth_label_coverage", "actionability_label_coverage"]:
        if metric in readiness_map:
            rows.append({"metric": metric, "value": readiness_map[metric], "note": "effective readiness used by this action brief"})
    return pd.DataFrame(rows)


def insight_measurement_scope(insight_kind: str) -> str:
    if insight_kind in PRODUCT_ACTION_MEASUREMENT_KINDS:
        return "product_candidate"
    if insight_kind in CONTEXT_ONLY_MEASUREMENT_KINDS:
        return "context_only"
    if insight_kind in MODEL_QUALITY_MEASUREMENT_KINDS:
        return "model_quality"
    return "validation_lead"


def normalize_insight_measurement_scope(scope: Any, insight_kind: str) -> str:
    canonical = insight_measurement_scope(insight_kind)
    value = first_nonempty([scope])
    if value not in {"product_candidate", "context_only", "model_quality", "validation_lead"}:
        return canonical
    if canonical != "validation_lead" and value != canonical:
        return canonical
    return value


def is_product_action_measurement_kind(insight_kind: str) -> bool:
    return insight_measurement_scope(insight_kind) == "product_candidate"


def product_action_insight_kinds_from_facts(facts: dict[str, Any]) -> list[str]:
    raw = facts.get("product_action_insight_kinds") or []
    if isinstance(raw, str):
        values = split_csv(raw)
    else:
        values = [str(value) for value in raw if str(value)]
    return sorted({value for value in values if is_product_action_measurement_kind(value)})


def product_action_measurement_quality(readiness: pd.DataFrame, facts: dict[str, Any]) -> dict[str, Any]:
    metrics = metric_map(readiness)
    scoped_kinds = product_action_insight_kinds_from_facts(facts)
    if not scoped_kinds:
        return {
            "scope_kinds": [],
            "precision_ready": False,
            "actionability_ready": False,
            "precision_rate": 0.0,
            "useful_signal_rate": 0.0,
            "actionability_rate": 0.0,
            "measurement_label_count": 0,
            "open_review_request_count": int_metric(metrics.get("open_review_request_count")),
            "measurement_scope": "no_product_action_insight_kinds",
        }

    truth_labeled = 0
    actionability_labeled = 0
    true_positive = 0
    false_positive = 0
    partial = 0
    actionable = 0
    needs_owner = 0
    measurement_labels = 0
    open_reviews = 0
    precision_ready = True
    actionability_ready = True
    for kind in scoped_kinds:
        required = int_metric(metrics.get(f"measurement_required_{kind}"))
        if required == 0:
            required = min(MIN_MEASUREMENT_LABEL_PER_KIND, int_metric(metrics.get(f"review_requests_{kind}")))
        kind_truth_labeled = int_metric(metrics.get(f"truth_labeled_{kind}"))
        kind_actionability_labeled = int_metric(metrics.get(f"actionability_labeled_{kind}"))
        if required <= 0 or kind_truth_labeled < required:
            precision_ready = False
        if required <= 0 or kind_actionability_labeled < required:
            actionability_ready = False
        truth_labeled += kind_truth_labeled
        actionability_labeled += kind_actionability_labeled
        true_positive += int_metric(metrics.get(f"true_positive_{kind}"))
        false_positive += int_metric(metrics.get(f"false_positive_{kind}"))
        partial += int_metric(metrics.get(f"partial_{kind}"))
        actionable += int_metric(metrics.get(f"actionable_{kind}"))
        needs_owner += int_metric(metrics.get(f"needs_owner_{kind}"))
        measurement_labels += int_metric(metrics.get(f"measurement_labels_{kind}"))
        open_reviews += int_metric(metrics.get(f"open_review_requests_{kind}"))
    return {
        "scope_kinds": scoped_kinds,
        "precision_ready": precision_ready,
        "actionability_ready": actionability_ready,
        "precision_rate": true_positive / truth_labeled if truth_labeled else 0.0,
        "useful_signal_rate": (true_positive + partial) / truth_labeled if truth_labeled else 0.0,
        "actionability_rate": (actionable + needs_owner) / actionability_labeled if actionability_labeled else 0.0,
        "false_positive_rate": false_positive / truth_labeled if truth_labeled else 0.0,
        "measurement_label_count": measurement_labels,
        "open_review_request_count": open_reviews,
        "measurement_scope": "product_action_kinds",
    }


def build_live_evaluation_readiness(review_queue: pd.DataFrame) -> pd.DataFrame:
    if review_queue.empty:
        return pd.DataFrame(
            [
                {"metric": "current_insight_count", "value": "0", "note": "live ontology current generated insight rows"},
                {"metric": "review_row_count", "value": "0", "note": "live ontology review rows for current generated insights"},
                {"metric": "precision_rate", "value": "0", "note": "true-positive rate across current measurement labels"},
                {"metric": "useful_signal_rate", "value": "0", "note": "true-positive plus partial rate across current measurement labels"},
                {"metric": "actionability_rate", "value": "0", "note": "actionable or needs-owner rate across current measurement labels"},
                {"metric": "false_positive_rate", "value": "0", "note": "false-positive rate across current measurement labels"},
                {"metric": "measurement_coverage_rate", "value": "0", "note": "current insight share with measurement labels"},
                {"metric": "ready_to_measure_precision", "value": "false", "note": "requires enough truth labels overall and per insight kind"},
                {"metric": "ready_to_measure_actionability", "value": "false", "note": "requires enough actionability labels overall and per insight kind"},
            ]
        )
    current = review_queue
    if "producer_state" in review_queue.columns:
        current = review_queue[review_queue["producer_state"] == "current"]
    current_insight_count = int(current["insight_key"].nunique()) if len(current) else 0
    all_label_rows = current[
        current["review_kind"].isin(["human_assessment", "evaluation_label"])
        & ((current["truth_label"] != "unknown") | (current["actionability_label"] != "unknown"))
    ] if len(current) else current
    label_rows = effective_label_rows(all_label_rows) if len(all_label_rows) else all_label_rows
    truth_labeled = int(label_rows[label_rows["truth_label"] != "unknown"]["insight_key"].nunique()) if len(label_rows) else 0
    actionability_labeled = int(label_rows[label_rows["actionability_label"] != "unknown"]["insight_key"].nunique()) if len(label_rows) else 0
    truth_labeled_keys = set(label_rows[label_rows["truth_label"] != "unknown"]["insight_key"].tolist()) if len(label_rows) else set()
    actionability_labeled_keys = set(label_rows[label_rows["actionability_label"] != "unknown"]["insight_key"].tolist()) if len(label_rows) else set()
    true_positive_count = int((label_rows["truth_label"] == "true_positive").sum()) if len(label_rows) else 0
    false_positive_count = int((label_rows["truth_label"] == "false_positive").sum()) if len(label_rows) else 0
    partial_count = int((label_rows["truth_label"] == "partial").sum()) if len(label_rows) else 0
    actionable_count = int((label_rows["actionability_label"] == "actionable").sum()) if len(label_rows) else 0
    needs_owner_count = int((label_rows["actionability_label"] == "needs_owner").sum()) if len(label_rows) else 0
    resolved = label_rows[
        label_rows["review_state"].isin(RESOLVED_REVIEW_STATES)
        & label_rows["truth_label"].isin(["true_positive", "false_positive"])
        & label_rows["actionability_label"].isin(POSITIVE_ACTIONABILITY | {"not_actionable"})
    ] if len(label_rows) else label_rows
    reviewed_count = int(resolved["insight_key"].nunique()) if len(resolved) else 0
    open_request_count = max(0, current_insight_count - reviewed_count)
    min_labeled_total = MIN_MEASUREMENT_LABEL_TOTAL
    min_labeled_per_kind = MIN_MEASUREMENT_LABEL_PER_KIND
    all_kinds = set(current["insight_kind"].unique()) if current_insight_count else set()
    current_by_kind = current.groupby("insight_kind")["insight_key"].nunique() if current_insight_count else pd.Series(dtype=int)
    truth_by_kind = label_rows[label_rows["truth_label"] != "unknown"].groupby("insight_kind")["insight_key"].nunique() if len(label_rows) else pd.Series(dtype=int)
    actionability_by_kind = label_rows[label_rows["actionability_label"] != "unknown"].groupby("insight_kind")["insight_key"].nunique() if len(label_rows) else pd.Series(dtype=int)
    required_by_kind = {kind: min(min_labeled_per_kind, int(current_by_kind.get(kind, 0))) for kind in all_kinds}
    truth_ready_by_kind = bool(all_kinds) and all(truth_by_kind.get(kind, 0) >= required_by_kind.get(kind, min_labeled_per_kind) for kind in all_kinds)
    actionability_ready_by_kind = bool(all_kinds) and all(actionability_by_kind.get(kind, 0) >= required_by_kind.get(kind, min_labeled_per_kind) for kind in all_kinds)
    product_possible_kinds = {str(kind) for kind in all_kinds if is_product_action_measurement_kind(str(kind))}
    product_current = current[current["insight_kind"].isin(product_possible_kinds)] if product_possible_kinds and len(current) else pd.DataFrame(columns=current.columns)
    product_label_rows = label_rows[label_rows["insight_kind"].isin(product_possible_kinds)] if product_possible_kinds and len(label_rows) else pd.DataFrame(columns=label_rows.columns)
    product_current_count = int(product_current["insight_key"].nunique()) if len(product_current) else 0
    product_truth_labeled = int(product_label_rows[product_label_rows["truth_label"] != "unknown"]["insight_key"].nunique()) if len(product_label_rows) else 0
    product_actionability_labeled = int(product_label_rows[product_label_rows["actionability_label"] != "unknown"]["insight_key"].nunique()) if len(product_label_rows) else 0
    product_true_positive = int((product_label_rows["truth_label"] == "true_positive").sum()) if len(product_label_rows) else 0
    product_false_positive = int((product_label_rows["truth_label"] == "false_positive").sum()) if len(product_label_rows) else 0
    product_partial = int((product_label_rows["truth_label"] == "partial").sum()) if len(product_label_rows) else 0
    product_actionable = int((product_label_rows["actionability_label"] == "actionable").sum()) if len(product_label_rows) else 0
    product_needs_owner = int((product_label_rows["actionability_label"] == "needs_owner").sum()) if len(product_label_rows) else 0
    product_resolved = product_label_rows[
        product_label_rows["review_state"].isin(RESOLVED_REVIEW_STATES)
        & product_label_rows["truth_label"].isin(["true_positive", "false_positive"])
        & product_label_rows["actionability_label"].isin(POSITIVE_ACTIONABILITY | {"not_actionable"})
    ] if len(product_label_rows) else product_label_rows
    product_reviewed_count = int(product_resolved["insight_key"].nunique()) if len(product_resolved) else 0
    product_open_request_count = max(0, product_current_count - product_reviewed_count)
    product_truth_ready_by_kind = bool(product_possible_kinds) and all(
        truth_by_kind.get(kind, 0) >= required_by_kind.get(kind, min_labeled_per_kind)
        for kind in product_possible_kinds
    )
    product_actionability_ready_by_kind = bool(product_possible_kinds) and all(
        actionability_by_kind.get(kind, 0) >= required_by_kind.get(kind, min_labeled_per_kind)
        for kind in product_possible_kinds
    )
    context_only_count = int(current[current["insight_kind"].map(lambda value: insight_measurement_scope(str(value)) == "context_only")]["insight_key"].nunique()) if len(current) else 0
    rows = [
        {"metric": "current_insight_count", "value": str(current_insight_count), "note": "live ontology current generated insight rows"},
        {"metric": "review_row_count", "value": str(len(current)), "note": "live ontology review rows for current generated insights"},
        {"metric": "label_row_count", "value": str(len(all_label_rows)), "note": "human/imported label rows with at least one non-unknown label"},
        {"metric": "evaluation_label_row_count", "value": str(len(label_rows)), "note": "deduped measurement-eligible label rows"},
        {"metric": "non_measurement_label_row_count", "value": str(max(0, len(all_label_rows) - len(label_rows))), "note": "smoke, candidate, or adversarial label rows excluded from readiness"},
        {"metric": "open_review_request_count", "value": str(open_request_count), "note": "current insights still missing accepted/dismissed/resolved measurement labels"},
        {"metric": "reviewed_count", "value": str(reviewed_count), "note": "current insights with resolved measurement labels"},
        {"metric": "truth_labeled_count", "value": str(truth_labeled), "note": "current live measurement labels with true/false/partial labels"},
        {"metric": "actionability_labeled_count", "value": str(actionability_labeled), "note": "current live measurement labels with actionability labels"},
        {"metric": "true_positive_count", "value": str(true_positive_count), "note": "current live measurement labels marked true positive"},
        {"metric": "false_positive_count", "value": str(false_positive_count), "note": "current live measurement labels marked false positive"},
        {"metric": "partial_count", "value": str(partial_count), "note": "current live measurement labels marked partial"},
        {"metric": "actionable_count", "value": str(actionable_count), "note": "current live measurement labels marked actionable"},
        {"metric": "needs_owner_count", "value": str(needs_owner_count), "note": "current live measurement labels marked needs-owner"},
        {"metric": "precision_rate", "value": rate_text(true_positive_count, truth_labeled), "note": "true-positive rate across current measurement labels"},
        {"metric": "useful_signal_rate", "value": rate_text(true_positive_count + partial_count, truth_labeled), "note": "true-positive plus partial rate across current measurement labels"},
        {"metric": "actionability_rate", "value": rate_text(actionable_count + needs_owner_count, actionability_labeled), "note": "actionable or needs-owner rate across current measurement labels"},
        {"metric": "false_positive_rate", "value": rate_text(false_positive_count, truth_labeled), "note": "false-positive rate across current measurement labels"},
        {"metric": "measurement_coverage_rate", "value": rate_text(len(label_rows), current_insight_count), "note": "current insight share with measurement labels"},
        {"metric": "truth_label_coverage", "value": f"{truth_labeled}/{current_insight_count}", "note": "current live truth-label numerator and denominator"},
        {"metric": "actionability_label_coverage", "value": f"{actionability_labeled}/{current_insight_count}", "note": "current live actionability-label numerator and denominator"},
        {"metric": "min_labeled_total_required", "value": str(min_labeled_total), "note": "minimum labels before aggregate metrics are considered stable enough to report"},
        {"metric": "min_labeled_per_kind_required", "value": str(min_labeled_per_kind), "note": "minimum labels required for each insight kind"},
        {"metric": "ready_to_measure_precision", "value": "true" if truth_labeled >= min_labeled_total and truth_ready_by_kind else "false", "note": "requires enough truth labels overall and per insight kind"},
        {"metric": "ready_to_measure_actionability", "value": "true" if actionability_labeled >= min_labeled_total and actionability_ready_by_kind else "false", "note": "requires enough actionability labels overall and per insight kind"},
        {"metric": "product_candidate_insight_count", "value": str(product_current_count), "note": "current insights whose kind can back product-action automation when selected by an action"},
        {"metric": "product_candidate_review_row_count", "value": str(int(len(product_current))), "note": "live review rows for product-action candidate generated insights"},
        {"metric": "context_only_insight_count", "value": str(context_only_count), "note": "current insights retained for context or validation only, not product-action precision"},
        {"metric": "product_candidate_measurement_label_count", "value": str(len(product_label_rows)), "note": "measurement labels on product-action candidate kinds"},
        {"metric": "product_candidate_open_review_request_count", "value": str(product_open_request_count), "note": "product-action candidate insights missing resolved measurement labels"},
        {"metric": "product_candidate_truth_labeled_count", "value": str(product_truth_labeled), "note": "truth labels on product-action candidate kinds"},
        {"metric": "product_candidate_actionability_labeled_count", "value": str(product_actionability_labeled), "note": "actionability labels on product-action candidate kinds"},
        {"metric": "product_candidate_precision_rate", "value": rate_text(product_true_positive, product_truth_labeled), "note": "true-positive rate for product-action candidate kinds"},
        {"metric": "product_candidate_useful_signal_rate", "value": rate_text(product_true_positive + product_partial, product_truth_labeled), "note": "true-positive plus partial rate for product-action candidate kinds"},
        {"metric": "product_candidate_actionability_rate", "value": rate_text(product_actionable + product_needs_owner, product_actionability_labeled), "note": "actionable or needs-owner rate for product-action candidate kinds"},
        {"metric": "product_candidate_false_positive_rate", "value": rate_text(product_false_positive, product_truth_labeled), "note": "false-positive rate for product-action candidate kinds"},
        {"metric": "product_candidate_measurement_coverage_rate", "value": rate_text(len(product_label_rows), product_current_count), "note": "product-action candidate insight share with measurement labels"},
        {"metric": "product_candidate_ready_to_measure_precision", "value": "true" if product_truth_labeled >= min_labeled_total and product_truth_ready_by_kind else "false", "note": "aggregate product-candidate precision readiness before action scoping"},
        {"metric": "product_candidate_ready_to_measure_actionability", "value": "true" if product_actionability_labeled >= min_labeled_total and product_actionability_ready_by_kind else "false", "note": "aggregate product-candidate actionability readiness before action scoping"},
    ]
    if current_insight_count:
        for insight_kind, group in current.groupby("insight_kind"):
            measurement_scope = insight_measurement_scope(str(insight_kind))
            current_kind_count = int(group["insight_key"].nunique())
            kind_label_rows = label_rows[label_rows["insight_kind"] == insight_kind] if len(label_rows) else label_rows
            kind_truth_labeled = int((kind_label_rows["truth_label"] != "unknown").sum()) if len(kind_label_rows) else 0
            kind_actionability_labeled = int((kind_label_rows["actionability_label"] != "unknown").sum()) if len(kind_label_rows) else 0
            kind_true_positive = int((kind_label_rows["truth_label"] == "true_positive").sum()) if len(kind_label_rows) else 0
            kind_false_positive = int((kind_label_rows["truth_label"] == "false_positive").sum()) if len(kind_label_rows) else 0
            kind_partial = int((kind_label_rows["truth_label"] == "partial").sum()) if len(kind_label_rows) else 0
            kind_actionable = int((kind_label_rows["actionability_label"] == "actionable").sum()) if len(kind_label_rows) else 0
            kind_needs_owner = int((kind_label_rows["actionability_label"] == "needs_owner").sum()) if len(kind_label_rows) else 0
            kind_resolved = kind_label_rows[
                kind_label_rows["review_state"].isin(RESOLVED_REVIEW_STATES)
                & kind_label_rows["truth_label"].isin(["true_positive", "false_positive"])
                & kind_label_rows["actionability_label"].isin(POSITIVE_ACTIONABILITY | {"not_actionable"})
            ] if len(kind_label_rows) else kind_label_rows
            kind_reviewed_count = int(kind_resolved["insight_key"].nunique()) if len(kind_resolved) else 0
            kind_open_request_count = max(0, current_kind_count - kind_reviewed_count)
            rows.append({"metric": f"review_requests_{insight_kind}", "value": str(current_kind_count), "note": "distinct current insights by kind that need measurement labels"})
            rows.append({"metric": f"review_rows_{insight_kind}", "value": str(int(len(group))), "note": "raw live review rows by current insight kind"})
            rows.append({"metric": f"measurement_labels_{insight_kind}", "value": str(int(len(kind_label_rows))), "note": "deduped measurement-eligible label rows by insight kind"})
            rows.append({"metric": f"open_review_requests_{insight_kind}", "value": str(kind_open_request_count), "note": "current insights by kind still missing resolved measurement labels"})
            required = required_by_kind.get(insight_kind, min_labeled_per_kind)
            kind_truth = int(truth_by_kind.get(insight_kind, 0))
            kind_actionability = int(actionability_by_kind.get(insight_kind, 0))
            rows.append({"metric": f"measurement_scope_{insight_kind}", "value": measurement_scope, "note": "measurement scope for this insight kind"})
            rows.append({"metric": f"measurement_required_{insight_kind}", "value": str(int(required)), "note": "bounded labels required before this sparse insight kind can support product actions"})
            rows.append({"metric": f"truth_labeled_{insight_kind}", "value": str(kind_truth), "note": "measurement truth labels by insight kind"})
            rows.append({"metric": f"actionability_labeled_{insight_kind}", "value": str(kind_actionability), "note": "measurement actionability labels by insight kind"})
            rows.append({"metric": f"true_positive_{insight_kind}", "value": str(kind_true_positive), "note": "true-positive labels by insight kind"})
            rows.append({"metric": f"false_positive_{insight_kind}", "value": str(kind_false_positive), "note": "false-positive labels by insight kind"})
            rows.append({"metric": f"partial_{insight_kind}", "value": str(kind_partial), "note": "partial labels by insight kind"})
            rows.append({"metric": f"actionable_{insight_kind}", "value": str(kind_actionable), "note": "actionable labels by insight kind"})
            rows.append({"metric": f"needs_owner_{insight_kind}", "value": str(kind_needs_owner), "note": "needs-owner labels by insight kind"})
            rows.append({"metric": f"precision_rate_{insight_kind}", "value": rate_text(kind_true_positive, kind_truth_labeled), "note": "true-positive rate by insight kind"})
            rows.append({"metric": f"useful_signal_rate_{insight_kind}", "value": rate_text(kind_true_positive + kind_partial, kind_truth_labeled), "note": "true-positive plus partial rate by insight kind"})
            rows.append({"metric": f"actionability_rate_{insight_kind}", "value": rate_text(kind_actionable + kind_needs_owner, kind_actionability_labeled), "note": "actionable or needs-owner rate by insight kind"})
            rows.append({"metric": f"false_positive_rate_{insight_kind}", "value": rate_text(kind_false_positive, kind_truth_labeled), "note": "false-positive rate by insight kind"})
            rows.append({"metric": f"measurement_coverage_rate_{insight_kind}", "value": rate_text(len(kind_label_rows), current_kind_count), "note": "current insight share with measurement labels by kind"})
            ready = kind_truth >= required and kind_actionability >= required and required > 0
            rows.append({"metric": f"ready_to_measure_{insight_kind}", "value": "true" if ready else "false", "note": "kind-level gate used for action-specific promotion"})
            kind_values = {
                "insight_kind": insight_kind,
                "measurement_scope": measurement_scope,
                "measurement_label_count": int(len(kind_label_rows)),
                "required_label_count": int(required),
                "ready_to_measure": ready,
                "ready_for_product_action": (
                    measurement_scope == "product_candidate"
                    and ready
                    and safe_float(rate_text(kind_true_positive, kind_truth_labeled)) >= MIN_PRECISION_RATE_FOR_PRODUCT_ACTION
                    and safe_float(rate_text(kind_true_positive + kind_partial, kind_truth_labeled)) >= MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION
                    and safe_float(rate_text(kind_actionable + kind_needs_owner, kind_actionability_labeled)) >= MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION
                ),
                "precision_rate": safe_float(rate_text(kind_true_positive, kind_truth_labeled)),
                "useful_signal_rate": safe_float(rate_text(kind_true_positive + kind_partial, kind_truth_labeled)),
                "actionability_rate": safe_float(rate_text(kind_actionable + kind_needs_owner, kind_actionability_labeled)),
            }
            gate_state, gate_reason = work_insight_kind_product_action_gate(kind_values)
            rows.append({"metric": f"ready_for_product_action_{insight_kind}", "value": "true" if kind_values["ready_for_product_action"] else "false", "note": "kind-level product-action gate after measurement quality thresholds"})
            rows.append({"metric": f"product_action_gate_state_{insight_kind}", "value": gate_state, "note": "kind-level product-action gate state"})
            rows.append({"metric": f"product_action_gate_reason_{insight_kind}", "value": gate_reason, "note": "kind-level product-action gate reason"})
    return pd.DataFrame(rows)


def kind_readiness_map(readiness: pd.DataFrame) -> dict[str, dict[str, int | bool]]:
    if readiness.empty:
        return {}
    metrics = metric_map(readiness)
    kinds = set()
    prefixes = ["review_requests_", "measurement_required_"]
    for metric in metrics:
        for prefix in prefixes:
            if metric.startswith(prefix):
                kinds.add(metric.removeprefix(prefix))
    out: dict[str, dict[str, int | bool]] = {}
    for kind in sorted(kinds):
        current_count = int_metric(metrics.get(f"review_requests_{kind}"))
        required = int_metric(metrics.get(f"measurement_required_{kind}"))
        if required == 0 and current_count > 0:
            required = min(MIN_MEASUREMENT_LABEL_PER_KIND, current_count)
        out[kind] = {
            "current_count": current_count,
            "required": required,
            "truth_labeled": int_metric(metrics.get(f"truth_labeled_{kind}")),
            "actionability_labeled": int_metric(metrics.get(f"actionability_labeled_{kind}")),
            "ready": metrics.get(f"ready_to_measure_{kind}") == "true",
            "ready_for_product_action": metrics.get(f"ready_for_product_action_{kind}") == "true",
            "product_action_gate_state": metrics.get(f"product_action_gate_state_{kind}") or "",
            "product_action_gate_reason": metrics.get(f"product_action_gate_reason_{kind}") or "",
            "precision_rate": safe_float(metrics.get(f"precision_rate_{kind}")),
            "useful_signal_rate": safe_float(metrics.get(f"useful_signal_rate_{kind}")),
            "actionability_rate": safe_float(metrics.get(f"actionability_rate_{kind}")),
        }
    return out


def int_metric(value: Any) -> int:
    try:
        return int(float(str(value)))
    except (TypeError, ValueError):
        return 0


def build_owner_action_rollup(action_items: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "owner_hint",
        "action_count",
        "product_action_count",
        "validation_lead_count",
        "model_or_rule_qa_count",
        "critical_or_high_count",
        "max_priority_score",
        "avg_priority_score",
        "decision_followup_count",
        "validate_signal_count",
        "ci_check_followup_count",
        "review_wait_followup_count",
        "coverage_limited_count",
        "anonymous_observation_count",
        "needs_human_review_count",
        "top_action_type",
        "top_subjects",
        "recommended_focus",
    ]
    if action_items.empty:
        return pd.DataFrame(columns=columns)
    items = action_items[
        (action_items["action_type"] != "dismissed_signal")
        & (action_items["decision_state"] != SOURCE_RESOLVED_DECISION)
    ].copy()
    if items.empty:
        return pd.DataFrame(columns=columns)
    items["owner_hint"] = items["owner_hint"].map(lambda value: first_nonempty([value]) or "(unassigned)")
    items["priority_score"] = pd.to_numeric(items["priority_score"], errors="coerce").fillna(0.0)
    rows: list[dict[str, Any]] = []
    for owner_hint, group in items.sort_values(["priority_score", "raw_priority_score"], ascending=[False, False]).groupby("owner_hint", sort=False):
        top_action_type = first_nonempty([group["action_type"].mode().iloc[0] if not group["action_type"].mode().empty else ""])
        top_subjects = ", ".join(group["subject_key"].astype(str).head(5).tolist())
        rows.append(
            {
                "owner_hint": owner_hint,
                "action_count": int(len(group)),
                "product_action_count": int((group["decision_state"] == "product_action").sum()) if "decision_state" in group.columns else 0,
                "validation_lead_count": int((group["decision_state"] == "validation_lead").sum()) if "decision_state" in group.columns else 0,
                "model_or_rule_qa_count": int((group["decision_state"] == "model_or_rule_qa").sum()) if "decision_state" in group.columns else 0,
                "critical_or_high_count": int(group[(group.get("decision_state", pd.Series(dtype=str)) == "product_action") & group["urgency"].isin(["critical", "high"])].shape[0])
                if "decision_state" in group.columns
                else int(group["urgency"].isin(["critical", "high"]).sum()),
                "max_priority_score": round(float(group["priority_score"].max()), 2),
                "avg_priority_score": round(float(group["priority_score"].mean()), 2),
                "decision_followup_count": int((group["action_type"] == "decision_or_owner_followup").sum()),
                "validate_signal_count": int((group["action_type"] == "validate_signal").sum()),
                "ci_check_followup_count": int((group["action_type"] == "ci_check_followup").sum()),
                "review_wait_followup_count": int((group["action_type"] == "review_wait_followup").sum()),
                "coverage_limited_count": int(group.apply(is_coverage_limited_action, axis=1).sum()),
                "anonymous_observation_count": int(group.apply(is_anonymous_observation_action, axis=1).sum()),
                "needs_human_review_count": int((group["needs_human_review"].astype(str) == "true").sum()),
                "top_action_type": top_action_type,
                "top_subjects": top_subjects,
                "recommended_focus": owner_focus_for(top_action_type),
            }
        )
    return pd.DataFrame(rows, columns=columns).sort_values(
        ["critical_or_high_count", "max_priority_score", "action_count"],
        ascending=[False, False, False],
    )


def build_program_register(
    action_items: pd.DataFrame,
    pr_features: pd.DataFrame,
    ticket_features: pd.DataFrame,
    dependency_edges: pd.DataFrame,
    transition_candidates: pd.DataFrame,
    generated_at: str,
) -> pd.DataFrame:
    columns = program_register_columns()
    if action_items.empty:
        return pd.DataFrame(columns=columns)
    pr_by_subject = pr_feature_by_subject(pr_features)
    ticket_by_subject = ticket_feature_by_subject(ticket_features)
    dependency_lookup = build_dependency_lookup(dependency_edges)
    transition_lookup = build_transition_lookup(transition_candidates)
    rows: list[dict[str, Any]] = []
    for _, action in action_items.iterrows():
        subject_kind = first_nonempty([action.get("subject_kind")])
        subject_key = first_nonempty([action.get("subject_key")])
        action_type = first_nonempty([action.get("action_type")])
        decision_state = first_nonempty([action.get("decision_state")])
        pr = pr_by_subject.get(subject_key, {})
        ticket = ticket_by_subject.get(subject_key.upper(), {})
        linked_tickets, linked_prs = linked_work_for_subject(subject_kind, subject_key, pr, dependency_lookup)
        owner_key, owner_source = owner_for_register(action, pr, ticket)
        author_dri = github_owner_hint(pr.get("author_login", ""))
        requested_reviewers = first_nonempty([pr.get("requested_reviewers")])
        transition_state = transition_lookup.get((subject_kind, subject_key), "")
        risk_score = first_nonempty([action.get("score"), pr.get("risk_score")])
        rows.append(
            {
                "program_key": f"tpm-program:{stable_digest([subject_kind, subject_key, action_type])}",
                "action_key": first_nonempty([action.get("action_key")]),
                "workstream_key": "flink-kubernetes-operator",
                "subject_kind": subject_kind,
                "subject_key": subject_key,
                "linked_ticket_keys": ", ".join(linked_tickets),
                "linked_pr_keys": ", ".join(linked_prs),
                "title": first_nonempty([action.get("title"), pr.get("title"), ticket.get("title")]),
                "program_status": program_status_for_action(action_type, decision_state),
                "tpm_bucket": tpm_bucket_for_action(action_type, decision_state),
                "owner_key": owner_key,
                "owner_source": owner_source,
                "author_dri": author_dri,
                "requested_reviewer_keys": requested_reviewers,
                "reviewer_or_approver": requested_reviewers,
                "next_action": first_nonempty([action.get("recommended_action")]),
                "decision_needed": decision_needed_for_action(action_type, decision_state),
                "decision_state": decision_state,
                "decision_gate_reason": first_nonempty([action.get("decision_gate_reason")]),
                "due_bucket": due_bucket_for_action(action),
                "last_source_update_at": first_nonempty([pr.get("updated_at"), ticket.get("updated_at")]),
                "age_days": first_nonempty([pr.get("age_days")]),
                "stale_days": first_nonempty([pr.get("stale_days")]),
                "risk_score": risk_score,
                "blocker_label_state": blocker_label_state_for_action(action),
                "ci_signal": ci_signal_for_action(action),
                "transition_state": transition_state,
                "dependency_summary": dependency_summary(linked_tickets, linked_prs),
                "source_coverage_state": source_coverage_state_for_register(action),
                "evidence_ref": first_nonempty([action.get("evidence_ref")]),
                "label_quality": label_quality_for_action(action),
                "updated_at": generated_at,
            }
        )
    out = pd.DataFrame(rows, columns=columns)
    out["_due_rank"] = out["due_bucket"].map(lambda value: {"now": 3, "this_week": 2, "watch": 1}.get(str(value), 0))
    out["risk_score"] = pd.to_numeric(out["risk_score"], errors="coerce").fillna(0.0)
    out = out.sort_values(["_due_rank", "risk_score", "subject_key"], ascending=[False, False, True]).drop(columns=["_due_rank"])
    return out


def program_register_columns() -> list[str]:
    return [
        "program_key",
        "action_key",
        "workstream_key",
        "subject_kind",
        "subject_key",
        "linked_ticket_keys",
        "linked_pr_keys",
        "title",
        "program_status",
        "tpm_bucket",
        "owner_key",
        "owner_source",
        "author_dri",
        "requested_reviewer_keys",
        "reviewer_or_approver",
        "next_action",
        "decision_needed",
        "decision_state",
        "decision_gate_reason",
        "due_bucket",
        "last_source_update_at",
        "age_days",
        "stale_days",
        "risk_score",
        "blocker_label_state",
        "ci_signal",
        "transition_state",
        "dependency_summary",
        "source_coverage_state",
        "evidence_ref",
        "label_quality",
        "updated_at",
    ]


def build_workstream_standup(
    action_items: pd.DataFrame,
    owner_rollup: pd.DataFrame,
    summary: pd.DataFrame,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    check_summary: pd.DataFrame,
    transition_candidates: pd.DataFrame,
    time_series_summary: pd.DataFrame,
    generated_at: str,
) -> pd.DataFrame:
    action_item_count = metric_int(summary, "action_item_count")
    summary_observed = metric_text(summary, "action_item_count") != "" or not action_items.empty
    readiness_observed = metric_text(readiness, "truth_label_coverage") != "" or metric_text(readiness, "actionability_label_coverage") != ""
    critical_or_high_count = metric_int(summary, "critical_or_high_count")
    validation_lead_count = metric_int(summary, "validation_lead_count")
    critical_or_high_validation_lead_count = metric_int(summary, "critical_or_high_validation_lead_count")
    model_or_rule_qa_count = metric_int(summary, "model_or_rule_qa_count")
    source_repair_count = metric_int(summary, "source_repair_count")
    coverage_limited_count = metric_int(summary, "coverage_limited_count")
    anonymous_observation_count = metric_int(summary, "anonymous_observation_count")
    failing_check_pr_count = metric_int(check_summary, "failing_check_pr_count")
    open_failing_check_pr_count = metric_int(check_summary, "open_failing_check_pr_count")
    if open_failing_check_pr_count == 0 and not action_items.empty:
        open_failing_check_pr_count = int((action_items["action_type"] == "ci_check_followup").sum())
    unresolved_closeout_subjects: list[str] = []
    unresolved_closeout_count = 0
    if not action_items.empty and "decision_state" in action_items.columns:
        unresolved_closeouts = action_items[action_items["decision_state"] == "closeout_review"].copy()
        unresolved_closeout_count = len(unresolved_closeouts)
        unresolved_closeout_subjects = unresolved_closeouts["subject_key"].astype(str).head(5).tolist() if "subject_key" in unresolved_closeouts.columns else []
    terminal_transition_count = unresolved_closeout_count
    owner_count = int(owner_rollup[owner_rollup["owner_hint"] != "(unassigned)"]["owner_hint"].nunique()) if not owner_rollup.empty else 0
    top_owner_action_count = int(owner_rollup["action_count"].max()) if not owner_rollup.empty else 0
    eta_ready = "true" if forecast_effective_eta_ready(forecast_summary, time_series_summary) else "false"
    attention_pressure = (
        critical_or_high_count > 0
        or open_failing_check_pr_count > 0
        or source_repair_count > 0
        or coverage_limited_count > 0
        or unresolved_closeout_count > 0
    )
    status = "unknown"
    if attention_pressure:
        status = "attention_required"
    elif validation_lead_count > 0:
        status = "validation_required"
    elif summary_observed and action_item_count == 0 and action_items.empty and readiness_observed and anonymous_observation_count == 0:
        status = "clear"
    elif summary_observed:
        status = "watch"
    transition_subjects = ", ".join(unresolved_closeout_subjects)
    return pd.DataFrame(
        [
            {
                "generated_at": generated_at,
                "workstream_key": "flink-kubernetes-operator",
                "operating_status": status,
                "action_item_count": action_item_count,
                "critical_or_high_count": critical_or_high_count,
                "open_work_count": metric_int(summary, "open_work_count"),
                "validation_lead_count": validation_lead_count,
                "critical_or_high_validation_lead_count": critical_or_high_validation_lead_count,
                "model_or_rule_qa_count": model_or_rule_qa_count,
                "closeout_review_count": unresolved_closeout_count,
                "owner_count": owner_count,
                "top_owner_action_count": top_owner_action_count,
                "failing_check_pr_count": failing_check_pr_count,
                "open_failing_check_pr_count": open_failing_check_pr_count,
                "source_repair_count": source_repair_count,
                "coverage_limited_count": coverage_limited_count,
                "anonymous_observation_count": anonymous_observation_count,
                "terminal_transition_count": terminal_transition_count,
                "terminal_transition_subjects": transition_subjects,
                "eta_forecast_ready": eta_ready,
                "truth_label_coverage": metric_text(readiness, "truth_label_coverage"),
                "actionability_label_coverage": metric_text(readiness, "actionability_label_coverage"),
                "recommended_cadence_focus": cadence_focus(
                    critical_or_high_count,
                    validation_lead_count,
                    critical_or_high_validation_lead_count,
                    open_failing_check_pr_count,
                    source_repair_count,
                    coverage_limited_count,
                    anonymous_observation_count,
                    eta_ready,
                ),
            }
        ]
    )


def build_standup_sections(action_items: pd.DataFrame, owner_rollup: pd.DataFrame, transition_candidates: pd.DataFrame) -> pd.DataFrame:
    columns = [
        "section_rank",
        "section_kind",
        "urgency",
        "owner_hint",
        "subject_key",
        "action_type",
        "status_signal",
        "summary",
        "recommended_action",
        "evidence_ref",
        "action_key",
    ]
    rows: list[dict[str, Any]] = []
    rank = 1
    if not action_items.empty:
        now_items = action_items[
            ~action_items["action_type"].isin(["dismissed_signal", "model_quality_review"])
            & (action_items["decision_state"] != SOURCE_RESOLVED_DECISION)
        ].sort_values(["priority_score", "raw_priority_score"], ascending=[False, False])
        for row in now_items.head(8).itertuples(index=False):
            state = first_nonempty([getattr(row, "decision_state", "")])
            rows.append(
                {
                    "section_rank": rank,
                    "section_kind": standup_section_kind_for_state(state),
                    "urgency": row.urgency,
                    "owner_hint": row.owner_hint,
                    "subject_key": row.subject_key,
                    "action_type": row.action_type,
                    "status_signal": row.status_signal,
                    "summary": row.title,
                    "recommended_action": row.recommended_action,
                    "evidence_ref": row.evidence_ref,
                    "action_key": row.action_key,
                }
            )
            rank += 1
        for row in action_items[action_items["action_type"] == "model_quality_review"].head(1).itertuples(index=False):
            rows.append(
                {
                    "section_rank": rank,
                    "section_kind": "model_quality",
                    "urgency": row.urgency,
                    "owner_hint": row.owner_hint,
                    "subject_key": row.subject_key,
                    "action_type": row.action_type,
                    "status_signal": row.status_signal,
                    "summary": row.title,
                    "recommended_action": row.recommended_action,
                    "evidence_ref": row.evidence_ref,
                    "action_key": row.action_key,
                }
            )
            rank += 1
    if not owner_rollup.empty:
        for row in owner_rollup.head(8).itertuples(index=False):
            rows.append(
                {
                    "section_rank": rank,
                    "section_kind": "owner_load",
                    "urgency": "high" if int(row.critical_or_high_count) else "medium",
                    "owner_hint": row.owner_hint,
                    "subject_key": "",
                    "action_type": row.top_action_type,
                    "status_signal": "owner_has_open_actions",
                    "summary": f"{row.action_count} action(s), {row.critical_or_high_count} critical/high; top subjects: {row.top_subjects}",
                    "recommended_action": row.recommended_focus,
                    "evidence_ref": "tpm_owner_action_rollup",
                    "action_key": "",
                }
            )
            rank += 1
    unresolved_closeout_subjects = set()
    if not action_items.empty and "decision_state" in action_items.columns and "subject_key" in action_items.columns:
        unresolved_closeout_subjects = set(
            action_items[action_items["decision_state"] == "closeout_review"]["subject_key"].astype(str).tolist()
        )
    if unresolved_closeout_subjects and not transition_candidates.empty and "transition_kind" in transition_candidates.columns:
        terminal = transition_candidates[transition_candidates["transition_kind"] == "terminal_state_change"].copy()
        terminal = terminal[terminal["subject_key"].astype(str).isin(unresolved_closeout_subjects)]
        for row in terminal.head(8).itertuples(index=False):
            rows.append(
                {
                    "section_rank": rank,
                    "section_kind": "resolved_change",
                    "urgency": "medium",
                    "owner_hint": "",
                    "subject_key": row.subject_key,
                    "action_type": "verify_terminal_transition",
                    "status_signal": row.transition_kind,
                    "summary": f"Observed {row.from_state} -> {row.to_state} transition",
                    "recommended_action": "Confirm the stale action was closed correctly and keep it out of open TPM follow-up.",
                    "evidence_ref": "tpm_state_transition_candidates",
                    "action_key": "",
                }
            )
            rank += 1
    if not rows:
        return pd.DataFrame(columns=columns)
    return pd.DataFrame(rows, columns=columns)


def is_coverage_limited_action(row: pd.Series) -> bool:
    status = str(row.get("source_observation_status") or "")
    coverage_kind = str(row.get("source_coverage_kind") or "")
    return status in {"source_failure", "source_failed", "coverage_limited", "generated_evidence"} or "partial" in coverage_kind or "failed" in coverage_kind or "generated" in coverage_kind


def is_anonymous_observation_action(row: pd.Series) -> bool:
    status = str(row.get("source_observation_status") or "")
    auth_state = str(row.get("source_auth_state") or "")
    return status == "observed_anonymous" or auth_state == "anonymous"


def standup_section_kind_for_state(decision_state: str) -> str:
    return {
        "product_action": "product_action",
        "validation_lead": "validation_lead",
        "source_repair": "source_repair",
        "closeout_review": "closeout_review",
        SOURCE_RESOLVED_DECISION: "resolved_change",
        "model_or_rule_qa": "model_or_rule_qa",
        "suppressed_signal": "suppressed_signal",
    }.get(decision_state, "top_action")


def owner_focus_for(action_type: str) -> str:
    if action_type == "decision_or_owner_followup":
        return "Decide merge, close, park, or owner path for aged open work."
    if action_type == "validate_signal":
        return "Validate blocker evidence before escalating as product work."
    if action_type == "ci_check_followup":
        return "Validate failing-check evidence with the owner before treating it as product work."
    if action_type == "review_wait_followup":
        return "Confirm whether the requested reviewer is still the right lead."
    if action_type == "refresh_source":
        return "Refresh source coverage before making product claims."
    return "Review the generated action and record a truth/actionability label."


def cadence_focus(
    critical_or_high_count: int,
    validation_lead_count: int,
    critical_or_high_validation_lead_count: int,
    failing_check_pr_count: int,
    source_repair_count: int,
    coverage_limited_count: int,
    anonymous_observation_count: int,
    eta_ready: str,
) -> str:
    focus: list[str] = []
    if critical_or_high_count:
        focus.append(f"triage {critical_or_high_count} product-safe actions")
    if validation_lead_count:
        if critical_or_high_validation_lead_count:
            focus.append(f"gold-label or validate {critical_or_high_validation_lead_count} urgent leads")
        else:
            focus.append(f"gold-label or validate {validation_lead_count} leads")
    if failing_check_pr_count:
        noun = "PR" if failing_check_pr_count == 1 else "PRs"
        lead_noun = "CI lead" if failing_check_pr_count == 1 else "CI leads"
        focus.append(f"validate {failing_check_pr_count} {noun} with failing checks as {lead_noun}")
    if source_repair_count:
        focus.append(f"refresh {source_repair_count} source-limited subjects")
    if coverage_limited_count:
        focus.append(f"treat {coverage_limited_count} coverage-limited observations as leads only")
    if anonymous_observation_count:
        focus.append(f"treat {anonymous_observation_count} anonymous public observations as lower-auth confidence")
    if eta_ready != "true":
        focus.append("keep forecast output as risk triage, not ETA commitment")
    return "; ".join(focus) if focus else "review labels and watch for new state changes"


def pr_feature_by_subject(pr_features: pd.DataFrame) -> dict[str, dict[str, Any]]:
    by_subject: dict[str, dict[str, Any]] = {}
    if pr_features.empty:
        return by_subject
    for _, row in pr_features.iterrows():
        try:
            subject_key = f"{row['repository']}#{int(row['pr_number'])}"
        except (KeyError, TypeError, ValueError):
            continue
        by_subject[subject_key] = row.to_dict()
    return by_subject


def subject_current_state_by_subject(
    pr_features: pd.DataFrame,
    ticket_features: pd.DataFrame,
) -> dict[tuple[str, str], str]:
    states: dict[tuple[str, str], str] = {}
    if not pr_features.empty:
        for _, row in pr_features.iterrows():
            repository = first_nonempty([row.get("repository")])
            pr_number = metric_row_int(row, "pr_number")
            state = first_nonempty([row.get("state")]).lower()
            if repository and pr_number > 0 and state:
                states[("pull_request", f"{repository}#{pr_number}")] = state
    if not ticket_features.empty:
        for _, row in ticket_features.iterrows():
            ticket_key = first_nonempty([row.get("ticket_key")]).upper()
            status = first_nonempty([row.get("status")]).lower()
            if ticket_key and status:
                states[("ticket", ticket_key)] = status
    return states


def ticket_feature_by_subject(ticket_features: pd.DataFrame) -> dict[str, dict[str, Any]]:
    by_subject: dict[str, dict[str, Any]] = {}
    if ticket_features.empty or "ticket_key" not in ticket_features.columns:
        return by_subject
    for _, row in ticket_features.iterrows():
        key = first_nonempty([row.get("ticket_key")]).upper()
        if key:
            by_subject[key] = row.to_dict()
    return by_subject


def build_dependency_lookup(dependency_edges: pd.DataFrame) -> dict[tuple[str, str], dict[str, set[str]]]:
    lookup: dict[tuple[str, str], dict[str, set[str]]] = {}
    if dependency_edges.empty:
        return lookup
    for _, row in dependency_edges.iterrows():
        source = parse_dependency_node(row.get("source_key"))
        target = parse_dependency_node(row.get("target_key"))
        if source is None or target is None:
            continue
        add_dependency(lookup, source, target)
        add_dependency(lookup, target, source)
    return lookup


def parse_dependency_node(value: Any) -> tuple[str, str] | None:
    text = first_nonempty([value])
    if text.startswith("ticket:"):
        return ("ticket", text.removeprefix("ticket:").upper())
    if text.startswith("pr:"):
        return ("pull_request", text.removeprefix("pr:"))
    return None


def add_dependency(
    lookup: dict[tuple[str, str], dict[str, set[str]]],
    subject: tuple[str, str],
    linked: tuple[str, str],
) -> None:
    links = lookup.setdefault(subject, {"tickets": set(), "prs": set()})
    if linked[0] == "ticket":
        links["tickets"].add(linked[1])
    if linked[0] == "pull_request":
        links["prs"].add(linked[1])


def dependency_action_edge_columns() -> list[str]:
    return [
        "edge_kind",
        "source_key",
        "target_key",
        "freshness",
        "risk_signal",
        "source_url",
        "source_coverage_state",
        "rank_score",
        "work_action_id",
        "action_key",
        "action_type",
        "action_state",
        "action_decision_state",
        "action_owner_key",
        "action_subject_kind",
        "action_subject_key",
        "work_blocker_id",
        "blocker_key",
        "blocker_state",
        "blocker_review_state",
        "blocker_truth_label",
        "blocker_actionability_label",
        "blocker_label_quality",
        "blocker_measurement_eligible",
    ]


def merge_dependency_edges_for_analytics(base_edges: pd.DataFrame, action_edges: pd.DataFrame) -> pd.DataFrame:
    base_edges = source_topology_dependency_edges(base_edges)
    if base_edges.empty and action_edges.empty:
        return pd.DataFrame(columns=["edge_kind", "source_key", "target_key", "freshness", "risk_signal"])
    columns = list(base_edges.columns) if not base_edges.empty else ["edge_kind", "source_key", "target_key", "freshness", "risk_signal"]
    for column in dependency_action_edge_columns():
        if column not in columns:
            columns.append(column)
    frames = []
    if not base_edges.empty:
        frames.append(base_edges.reindex(columns=columns))
    if not action_edges.empty:
        frames.append(action_edges.reindex(columns=columns))
    merged = concat_dataframes_preserving_columns(frames, columns)
    if {"edge_kind", "source_key", "target_key"}.issubset(merged.columns):
        merged["_dedupe_key"] = (
            merged["edge_kind"].astype(str)
            + "\n"
            + merged["source_key"].astype(str)
            + "\n"
            + merged["target_key"].astype(str)
        )
        merged = merged.drop_duplicates("_dedupe_key", keep="last").drop(columns=["_dedupe_key"])
    if "edge_kind" in merged.columns:
        edge_order = {"blocked_by": 0, "needs_action": 1, "workstream_component": 2, "ticket_pr": 3}
        merged["_edge_order"] = merged["edge_kind"].map(lambda value: edge_order.get(str(value), 9))
        sort_columns = [column for column in ["_edge_order", "edge_kind", "source_key", "target_key"] if column in merged.columns]
        merged = merged.sort_values(sort_columns).drop(columns=["_edge_order"]).reset_index(drop=True)
    return merged


def concat_dataframes_preserving_columns(frames: list[pd.DataFrame], columns: list[str]) -> pd.DataFrame:
    nonempty = [frame for frame in frames if frame is not None and not frame.empty]
    if not nonempty:
        return pd.DataFrame(columns=columns)
    target_dtypes = dataframe_column_dtypes(nonempty, columns)
    trimmed_frames: list[pd.DataFrame] = []
    for frame in nonempty:
        reindexed = frame.reindex(columns=columns)
        populated_columns = [column for column in columns if reindexed[column].notna().any()]
        if populated_columns:
            trimmed_frames.append(reindexed.loc[:, populated_columns])
        else:
            trimmed_frames.append(pd.DataFrame({"_row_present": [True] * len(reindexed)}, index=reindexed.index))
    merged = pd.concat(trimmed_frames, ignore_index=True).drop(columns=["_row_present"], errors="ignore").reindex(columns=columns)
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


def source_topology_dependency_edges(dependency_edges: pd.DataFrame) -> pd.DataFrame:
    if dependency_edges.empty or "edge_kind" not in dependency_edges.columns:
        return dependency_edges
    return dependency_edges[~dependency_edges["edge_kind"].astype(str).isin({"blocked_by", "needs_action"})].copy()


def build_transition_lookup(transition_candidates: pd.DataFrame) -> dict[tuple[str, str], str]:
    lookup: dict[tuple[str, str], str] = {}
    if transition_candidates.empty:
        return lookup
    for _, row in transition_candidates.iterrows():
        subject = (first_nonempty([row.get("subject_kind")]), first_nonempty([row.get("subject_key")]))
        if not subject[0] or not subject[1]:
            continue
        lookup[subject] = f"{first_nonempty([row.get('from_state')])}->{first_nonempty([row.get('to_state')])}"
    return lookup


def linked_work_for_subject(
    subject_kind: str,
    subject_key: str,
    pr: dict[str, Any],
    dependency_lookup: dict[tuple[str, str], dict[str, set[str]]],
) -> tuple[list[str], list[str]]:
    links = dependency_lookup.get((subject_kind, subject_key), {"tickets": set(), "prs": set()})
    tickets = set(links.get("tickets", set()))
    prs = set(links.get("prs", set()))
    if subject_kind == "pull_request":
        prs.discard(subject_key)
        tickets.update(split_csv(first_nonempty([pr.get("issue_keys_in_text")])))
    if subject_kind == "ticket":
        tickets.discard(subject_key.upper())
    return sorted(tickets), sorted(prs)


def split_csv(value: str) -> list[str]:
    return [part.strip() for part in value.split(",") if part.strip()]


def owner_for_register(action: pd.Series, pr: dict[str, Any], ticket: dict[str, Any]) -> tuple[str, str]:
    owner_hint = first_nonempty([action.get("owner_hint")])
    pr_author = github_owner_hint(pr.get("author_login", ""))
    action_type = first_nonempty([action.get("action_type")])
    if owner_hint and pr_author and owner_hint == pr_author:
        if action_type == "review_wait_followup":
            return owner_hint, "pr_author_waiting_on_requested_reviewer"
        return owner_hint, "pr_author"
    if owner_hint:
        return owner_hint, "action_owner_hint"
    if pr_author:
        return pr_author, "pr_author"
    ticket_assignee = first_nonempty([ticket.get("assignee")])
    if ticket_assignee:
        return ticket_assignee, "jira_assignee"
    return "", "unassigned"


def github_owner_hint(login: Any) -> str:
    value = first_nonempty([login])
    if not value:
        return ""
    return value if value.startswith("github:") else f"github:{value}"


def program_status_for_action(action_type: str, decision_state: str = "") -> str:
    if decision_state == SOURCE_RESOLVED_DECISION:
        return "closure_candidate"
    if decision_state == "validation_lead" and action_type == "decision_or_owner_followup":
        return "validate_signal"
    if decision_state == "validation_lead" and action_type == "ci_check_followup":
        return "validate_signal"
    return {
        "decision_or_owner_followup": "needs_decision",
        "validate_signal": "validate_signal",
        "ci_check_followup": "ci_failing",
        "review_wait_followup": "waiting_review",
        "refresh_source": "source_repair",
        "verify_resolution": "closed_pending_review",
        "model_quality_review": "model_quality",
        "dismissed_signal": "dismissed",
        "clear_blocker": "closure_candidate",
    }.get(action_type, "needs_review")


def tpm_bucket_for_action(action_type: str, decision_state: str = "") -> str:
    if decision_state == "validation_lead" and action_type == "decision_or_owner_followup":
        return "risk_validation"
    if decision_state == "validation_lead" and action_type == "ci_check_followup":
        return "ci_validation"
    return {
        "decision_or_owner_followup": "risk",
        "validate_signal": "blocker",
        "ci_check_followup": "ci",
        "review_wait_followup": "reviewer_wait",
        "refresh_source": "source_repair",
        "verify_resolution": "closure",
        "model_quality_review": "model_quality",
        "dismissed_signal": "dismissal",
        "clear_blocker": "closure",
    }.get(action_type, "review")


def decision_needed_for_action(action_type: str, decision_state: str = "") -> str:
    if decision_state == SOURCE_RESOLVED_DECISION:
        return "none"
    if decision_state == "validation_lead" and action_type == "decision_or_owner_followup":
        return "gold-label risk/actionability before owner decision"
    if decision_state == "validation_lead" and action_type == "ci_check_followup":
        return "determine required/merge-blocking check semantics"
    return {
        "decision_or_owner_followup": "merge / close / park / assign owner",
        "validate_signal": "validate / suppress / escalate",
        "ci_check_followup": "fix / mark non-blocking / assign owner",
        "review_wait_followup": "confirm reviewer / reassign / close",
        "refresh_source": "refresh source before product claim",
        "verify_resolution": "confirm closeout",
        "model_quality_review": "collect labels and more snapshots",
        "dismissed_signal": "none",
    }.get(action_type, "review")


def due_bucket_for_action(action: pd.Series) -> str:
    action_type = first_nonempty([action.get("action_type")])
    decision_state = first_nonempty([action.get("decision_state")])
    if decision_state == SOURCE_RESOLVED_DECISION:
        return "watch"
    if action_type == "dismissed_signal":
        return "watch"
    if action_type == "verify_resolution":
        return "this_week"
    priority = safe_float(action.get("priority_score"))
    urgency = first_nonempty([action.get("urgency")])
    if urgency in {"critical", "high"} or priority >= 90:
        return "now"
    if priority >= 65:
        return "this_week"
    return "watch"


def blocker_label_state_for_action(action: pd.Series) -> str:
    action_type = first_nonempty([action.get("action_type")])
    candidate_dismissed = first_nonempty([action.get("candidate_dismissed_kinds")])
    if candidate_dismissed:
        return f"candidate_dismissed:{candidate_dismissed}"
    if action_type == "dismissed_signal":
        return "dismissed"
    if first_nonempty([action.get("needs_human_review")]) == "true":
        return "needs_measurement_label"
    return "not_required"


def ci_signal_for_action(action: pd.Series) -> str:
    if first_nonempty([action.get("action_type")]) == "ci_check_followup":
        if first_nonempty([action.get("decision_state")]) == "product_action":
            return "required_failing_or_pending"
        return "failing_or_pending"
    if "check_coverage" in first_nonempty([action.get("source_coverage_kind")]):
        return "check_observed"
    return ""


def dependency_summary(linked_tickets: list[str], linked_prs: list[str]) -> str:
    pieces = []
    if linked_tickets:
        pieces.append(f"{len(linked_tickets)} linked ticket(s)")
    if linked_prs:
        pieces.append(f"{len(linked_prs)} linked PR(s)")
    return "; ".join(pieces)


def source_coverage_state_for_register(action: pd.Series) -> str:
    status = first_nonempty([action.get("source_observation_status")])
    coverage = first_nonempty([action.get("source_coverage_kind")])
    if status == "observed_anonymous":
        return f"anonymous_success:{coverage}"
    if status == "observed":
        return f"observed:{coverage}"
    if status == "generated_evidence":
        return f"generated:{coverage}"
    return status or coverage


def label_quality_for_action(action: pd.Series) -> str:
    if first_nonempty([action.get("needs_human_review")]) == "true":
        return "unmeasured"
    if first_nonempty([action.get("action_type")]) == "dismissed_signal":
        return "candidate_dismissal"
    return "not_required"


def work_program_subject_kind(value: str) -> str:
    if value in {"pull_request", "ticket"}:
        return value
    return "unknown"


def work_program_status_value(value: str) -> str:
    if value in {
        "unknown",
        "needs_decision",
        "validate_signal",
        "ci_failing",
        "waiting_review",
        "source_repair",
        "closed_pending_review",
        "model_quality",
        "dismissed",
        "closure_candidate",
        "needs_review",
    }:
        return value
    return "unknown"


def work_program_bucket_value(value: str) -> str:
    if value in {
        "unknown",
        "risk",
        "risk_validation",
        "blocker",
        "ci",
        "ci_validation",
        "reviewer_wait",
        "closure",
        "dismissal",
        "model_quality",
        "source_repair",
        "review",
    }:
        return value
    return "unknown"


def work_program_milestone_kind_value(value: str) -> str:
    if value in {"release_target", "explicit_due_date", "resolution_outcome"}:
        return value
    return "release_target"


def work_program_milestone_state_value(value: str) -> str:
    if value in {
        "unknown",
        "planned",
        "past_target_unresolved",
        "resolved_before_target",
        "resolved_after_target",
        "no_target_date",
        "outcome_only",
    }:
        return value
    return "unknown"


def work_program_milestone_commitment_strength_value(value: str) -> str:
    if value in {"unknown", "release_signal", "explicit_commitment", "outcome_evidence"}:
        return value
    return "unknown"


def work_action_decision_state_value(value: str) -> str:
    if value in {
        "product_action",
        "validation_lead",
        "source_repair",
        "closeout_review",
        SOURCE_RESOLVED_DECISION,
        "model_or_rule_qa",
        "suppressed_signal",
        "pending_review",
    }:
        return value
    return "pending_review"


def work_action_due_bucket_value(value: str) -> str:
    if value in {"now", "this_week", "watch", "unscheduled"}:
        return value
    return "unscheduled"


def work_program_item_freshness(source_coverage_state: str, subject_kind: str) -> str:
    state = first_nonempty([source_coverage_state]).lower()
    if subject_kind == "unknown":
        return "partial"
    if not state:
        return "partial"
    partial_markers = ("failed", "failure", "partial", "repair", "unavailable", "unknown", "missing", "not_observed")
    if any(marker in state for marker in partial_markers):
        return "partial"
    return "fresh"


def work_program_item_evidence_excerpt(row: pd.Series, subject_kind: str, subject_key: str) -> str:
    program_status = work_program_status_value(first_nonempty([row.get("program_status")]))
    tpm_bucket = work_program_bucket_value(first_nonempty([row.get("tpm_bucket")]))
    next_action = first_nonempty([row.get("next_action"), row.get("decision_needed"), row.get("title")])
    return f"{subject_kind} {subject_key} in {program_status}/{tpm_bucket}: {next_action}"


def build_work_actions(action_items: pd.DataFrame, generated_at: str) -> pd.DataFrame:
    columns = [
        "action_key",
        "subject_kind",
        "subject_key",
        "source_insight_keys",
        "source_link_insight_kinds",
        "action_type",
        "action_state",
        "decision_state",
        "decision",
        "decision_reason",
        "owner_key",
        "owner_source",
        "due_bucket",
        "created_from_run_key",
        "opened_at",
        "decided_at",
        "closed_at",
    ]
    if action_items.empty:
        return pd.DataFrame(columns=columns)
    rows: list[dict[str, Any]] = []
    for _, action in action_items.iterrows():
        decision_state = first_nonempty([action.get("decision_state")])
        action_type = first_nonempty([action.get("action_type")])
        owner_key = first_nonempty([action.get("owner_hint")])
        action_state = "closed" if decision_state in {"suppressed_signal", SOURCE_RESOLVED_DECISION} else "open"
        closed_at = generated_at if action_state == "closed" else ""
        rows.append(
            {
                "action_key": first_nonempty([action.get("action_key")]),
                "subject_kind": first_nonempty([action.get("subject_kind")]),
                "subject_key": first_nonempty([action.get("subject_key")]),
                "source_insight_keys": first_nonempty([action.get("source_insight_keys")]),
                "source_link_insight_kinds": first_nonempty([action.get("source_link_insight_kinds")]),
                "action_type": action_type,
                "action_state": action_state,
                "decision_state": decision_state,
                "decision": action_decision_for_state(decision_state),
                "decision_reason": first_nonempty([action.get("decision_gate_reason")]),
                "owner_key": owner_key,
                "owner_source": "action_owner_hint" if owner_key else "unassigned",
                "due_bucket": due_bucket_for_action(action),
                "created_from_run_key": f"tpm-action-brief:{generated_at}",
                "opened_at": generated_at,
                "decided_at": generated_at if decision_state in {"product_action", "suppressed_signal", SOURCE_RESOLVED_DECISION} else "",
                "closed_at": closed_at,
            }
        )
    return pd.DataFrame(rows, columns=columns)


def build_work_action_observations(action_items: pd.DataFrame, generated_at: str) -> pd.DataFrame:
    columns = [
        "action_key",
        "observation_kind",
        "observed_at",
        "source_system",
        "source_url",
        "source_coverage_state",
        "auth_state",
        "current_state",
        "ci_signal",
        "ci_required_check_coverage_state",
        "ci_required_check_match_state",
        "ci_required_context_count",
        "ci_failing_required_context_count",
        "ci_pending_required_context_count",
        "ci_missing_required_context_count",
        "ci_failing_required_contexts",
        "ci_pending_required_contexts",
        "ci_missing_required_contexts",
        "ci_failing_context_count",
        "ci_pending_context_count",
        "ci_failing_contexts",
        "ci_pending_contexts",
        "evidence_ref",
        "supports_action",
    ]
    if action_items.empty:
        return pd.DataFrame(columns=columns)
    rows: list[dict[str, Any]] = []
    for _, action in action_items.iterrows():
        action_type = first_nonempty([action.get("action_type")])
        decision_state = first_nonempty([action.get("decision_state")])
        observation_kind = "ci_signal" if action_type == "ci_check_followup" else "source_state"
        if decision_state in {"closeout_review", SOURCE_RESOLVED_DECISION} or action_type == "verify_resolution":
            observation_kind = "closeout_review"
        if decision_state in {"model_or_rule_qa", "suppressed_signal"}:
            observation_kind = decision_state
        supports_action = "true" if decision_state == "product_action" else "false"
        rows.append(
            {
                "action_key": first_nonempty([action.get("action_key")]),
                "observation_kind": observation_kind,
                "observed_at": generated_at,
                "source_system": "cubicle_analytics",
                "source_url": first_nonempty([action.get("source_url")]),
                "source_coverage_state": source_coverage_state_for_register(action),
                "auth_state": first_nonempty([action.get("source_auth_state")]),
                "current_state": first_nonempty([action.get("current_state")]),
                "ci_signal": ci_signal_for_action(action),
                "ci_required_check_coverage_state": first_nonempty([action.get("required_check_coverage_state")]),
                "ci_required_check_match_state": first_nonempty([action.get("required_check_match_state")]),
                "ci_required_context_count": metric_row_int(action, "required_check_context_count"),
                "ci_failing_required_context_count": metric_row_int(action, "failing_required_context_count"),
                "ci_pending_required_context_count": metric_row_int(action, "pending_required_context_count"),
                "ci_missing_required_context_count": metric_row_int(action, "missing_required_context_count"),
                "ci_failing_required_contexts": first_nonempty([action.get("failing_required_contexts")]),
                "ci_pending_required_contexts": first_nonempty([action.get("pending_required_contexts")]),
                "ci_missing_required_contexts": first_nonempty([action.get("missing_required_contexts")]),
                "ci_failing_context_count": metric_row_int(action, "failing_context_count"),
                "ci_pending_context_count": metric_row_int(action, "pending_context_count"),
                "ci_failing_contexts": first_nonempty([action.get("failing_contexts")]),
                "ci_pending_contexts": first_nonempty([action.get("pending_contexts")]),
                "evidence_ref": first_nonempty([action.get("evidence_ref")]),
                "supports_action": supports_action,
            }
        )
    return pd.DataFrame(rows, columns=columns)


def action_decision_for_state(decision_state: str) -> str:
    return {
        "product_action": "ready_for_product_action",
        "validation_lead": "pending_validation",
        "source_repair": "repair_source_before_claim",
        "closeout_review": "pending_closeout_review",
        SOURCE_RESOLVED_DECISION: "source_confirmed_resolved",
        "model_or_rule_qa": "qa_review",
        "suppressed_signal": "dismissed_or_suppressed",
    }.get(decision_state, "pending_review")


def persist_work_actions_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    action_items: pd.DataFrame,
    work_action_observations: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_actions", "work_action_observations", "work_action_source_insights", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before action materialization")
    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    pr_ids = ontology_pr_ids_by_subject(conn)
    ticket_ids = ontology_ticket_ids_by_subject(conn)
    insight_ids = ontology_insight_ids_by_key(conn, source_instance)
    insight_ids_by_signature = ontology_insight_ids_by_signature(conn, source_instance)
    current_action_keys = {
        first_nonempty([action.get("action_key")])
        for _, action in action_items.iterrows()
        if first_nonempty([action.get("action_key")])
    }

    if current_action_keys:
        placeholders = ",".join("?" for _ in current_action_keys)
        conn.execute(
            f"""
            update work_actions
               set action_state = 'superseded',
                   updated_at = ?
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_work_action'
               and not (
                 action_state = 'closed'
                 and decision_state not in ('source_resolved', 'suppressed_signal')
               )
               and key not in ({placeholders})
            """,
            (now, source_instance, *sorted(current_action_keys)),
        )
    else:
        conn.execute(
            """
            update work_actions
               set action_state = 'superseded',
                   updated_at = ?
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_work_action'
               and not (
                 action_state = 'closed'
                 and decision_state not in ('source_resolved', 'suppressed_signal')
               )
            """,
            (now, source_instance),
        )

    action_id_by_key: dict[str, int] = {}
    for _, action in action_items.iterrows():
        action_key = first_nonempty([action.get("action_key")])
        if not action_key:
            continue
        existing_action = conn.execute(
            """
            select id, action_state, decision_state
              from work_actions
             where key = ?
               and source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_work_action'
            """,
            (action_key, source_instance),
        ).fetchone()
        if (
            existing_action is not None
            and str(existing_action[1]) == "closed"
            and str(existing_action[2]) not in {SOURCE_RESOLVED_DECISION, "suppressed_signal"}
        ):
            action_id_by_key[action_key] = int(existing_action[0])
            continue
        subject_kind = first_nonempty([action.get("subject_kind")]) or "unknown"
        subject_key = first_nonempty([action.get("subject_key")])
        pull_request_id = pr_ids.get(subject_key) if subject_kind == "pull_request" else None
        ticket_id = ticket_ids.get(subject_key.upper()) if subject_kind == "ticket" else None
        source_insight_keys = split_csv(first_nonempty([action.get("source_insight_keys")]))
        linked_insight_ids = [insight_ids[key] for key in source_insight_keys if key in insight_ids]
        source_link_insight_kinds = split_csv(first_nonempty([action.get("source_link_insight_kinds")]))
        fallback_insight_kinds = source_link_insight_kinds or split_csv(first_nonempty([action.get("insight_kinds")]))
        for insight_kind in fallback_insight_kinds:
            insight_id = insight_ids_by_signature.get((subject_kind, subject_key, insight_kind))
            if insight_id is not None and insight_id not in linked_insight_ids:
                linked_insight_ids.append(insight_id)
        action_state = first_nonempty([action.get("decision_state")])
        row_action_state = "closed" if action_state in {"suppressed_signal", SOURCE_RESOLVED_DECISION} else "open"
        closed_at = now if row_action_state == "closed" else None
        evidence_ref = first_nonempty([action.get("evidence_ref")])
        action_values = {
            "key": action_key,
            "action_type": first_nonempty([action.get("action_type")]) or "review_insight",
            "action_state": row_action_state,
            "decision_state": action_state or "pending_review",
            "decision": action_decision_for_state(action_state),
            "decision_reason": first_nonempty([action.get("decision_gate_reason")]),
            "subject_kind": subject_kind,
            "subject_key": subject_key,
            "pull_request_id": pull_request_id,
            "ticket_id": ticket_id,
            "owner_key": first_nonempty([action.get("owner_hint")]),
            "owner_source": "action_owner_hint" if first_nonempty([action.get("owner_hint")]) else "unassigned",
            "due_bucket": due_bucket_for_action(action),
            "created_from_run_key": f"tpm-action-brief:{now}",
            "opened_at": now,
            "decided_at": now if action_state in {"product_action", "suppressed_signal", SOURCE_RESOLVED_DECISION} else None,
            "closed_at": closed_at,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_action",
            "external_id": action_key,
            "source_url": first_nonempty([action.get("source_url")]),
            "latest_evidence_id": None,
            "evidence_count": 1 if evidence_ref else 0,
            "freshness_state": "fresh",
            "visibility": "unknown",
            "confidence": safe_float(action.get("confidence")),
            "event_count": max(1, len(linked_insight_ids)),
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": safe_float(action.get("priority_score")),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "work_actions", action_values, "key")
        action_id = int(conn.execute("select id from work_actions where key = ?", (action_key,)).fetchone()[0])
        action_id_by_key[action_key] = action_id
        evidence_id = upsert_action_evidence(
            conn,
            source_instance,
            "work_action",
            action_id,
            "decision_state",
            action_key,
            evidence_ref,
            first_nonempty([action.get("source_url")]),
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update work_actions set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, action_id),
            )
        conn.execute("delete from work_action_source_insights where work_action_id = ?", (action_id,))
        for insight_id in linked_insight_ids:
            conn.execute(
                "insert or ignore into work_action_source_insights (work_action_id, work_insight_id) values (?, ?)",
                (action_id, insight_id),
            )

    if work_action_observations.empty:
        return
    observation_action_ids: set[int] = set()
    for _, observation in work_action_observations.iterrows():
        action_key = first_nonempty([observation.get("action_key")])
        action_id = action_id_by_key.get(action_key)
        if action_id is None:
            row = conn.execute("select id from work_actions where key = ?", (action_key,)).fetchone()
            if row is None:
                continue
            action_id = int(row[0])
        observation_action_ids.add(action_id)
    for action_id in sorted(observation_action_ids):
        conn.execute(
            """
            delete from work_action_observations
             where work_action_id = ?
               and source_system = 'cubicle_analytics'
               and source_instance = ?
            """,
            (action_id, source_instance),
        )
    for _, observation in work_action_observations.iterrows():
        action_key = first_nonempty([observation.get("action_key")])
        action_id = action_id_by_key.get(action_key)
        if action_id is None:
            row = conn.execute("select id from work_actions where key = ?", (action_key,)).fetchone()
            if row is None:
                continue
            action_id = int(row[0])
        observation_kind = first_nonempty([observation.get("observation_kind")]) or "source_state"
        observation_key = f"work-action-observation:cubicle-analytics:{source_instance}:{stable_digest([action_key, observation_kind])}"
        evidence_ref = first_nonempty([observation.get("evidence_ref")])
        observation_values = {
            "key": observation_key,
            "work_action_id": action_id,
            "observation_kind": observation_kind,
            "source_coverage_state": first_nonempty([observation.get("source_coverage_state")]),
            "auth_state": first_nonempty([observation.get("auth_state")]),
            "current_state": first_nonempty([observation.get("current_state")]),
            "ci_signal": first_nonempty([observation.get("ci_signal")]),
            "ci_required_check_coverage_state": first_nonempty([observation.get("ci_required_check_coverage_state")]),
            "ci_required_check_match_state": first_nonempty([observation.get("ci_required_check_match_state")]),
            "ci_required_context_count": metric_row_int(observation, "ci_required_context_count"),
            "ci_failing_required_context_count": metric_row_int(observation, "ci_failing_required_context_count"),
            "ci_pending_required_context_count": metric_row_int(observation, "ci_pending_required_context_count"),
            "ci_missing_required_context_count": metric_row_int(observation, "ci_missing_required_context_count"),
            "ci_failing_required_contexts": first_nonempty([observation.get("ci_failing_required_contexts")]),
            "ci_pending_required_contexts": first_nonempty([observation.get("ci_pending_required_contexts")]),
            "ci_missing_required_contexts": first_nonempty([observation.get("ci_missing_required_contexts")]),
            "ci_failing_context_count": metric_row_int(observation, "ci_failing_context_count"),
            "ci_pending_context_count": metric_row_int(observation, "ci_pending_context_count"),
            "ci_failing_contexts": first_nonempty([observation.get("ci_failing_contexts")]),
            "ci_pending_contexts": first_nonempty([observation.get("ci_pending_contexts")]),
            "supports_action": first_nonempty([observation.get("supports_action")]) == "true",
            "observed_at": now,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_action_observation",
            "external_id": observation_key,
            "source_url": first_nonempty([observation.get("source_url")]),
            "latest_evidence_id": None,
            "evidence_count": 1 if evidence_ref else 0,
            "freshness_state": "fresh",
            "visibility": "unknown",
            "confidence": 1.0,
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "work_action_observations", observation_values, "key")
        observation_id = int(conn.execute("select id from work_action_observations where key = ?", (observation_key,)).fetchone()[0])
        evidence_id = upsert_action_evidence(
            conn,
            source_instance,
            "work_action_observation",
            observation_id,
            "source_state",
            observation_key,
            evidence_ref,
            first_nonempty([observation.get("source_url")]),
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update work_action_observations set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, observation_id),
            )


def upsert_action_evidence(
    conn: sqlite3.Connection,
    source_instance: str,
    claim_target_kind: str,
    claim_target_id: int,
    claim_field: str,
    owner_key: str,
    evidence_ref: str,
    source_url: str,
    now: str,
) -> int | None:
    evidence_ref = first_nonempty([evidence_ref])
    source_url = first_nonempty([source_url])
    if not evidence_ref:
        return None
    locator_kind, locator, parsed_source_url = parse_evidence_ref(evidence_ref, source_url)
    source_url = parsed_source_url or source_url
    evidence_key = f"evidence:cubicle-analytics:{source_instance}:{stable_digest([claim_target_kind, claim_target_id, owner_key, evidence_ref])}"
    evidence_values = {
        "key": evidence_key,
        "claim_kind": "object_state",
        "claim_target_kind": claim_target_kind,
        "claim_target_id": claim_target_id,
        "claim_field": claim_field,
        "locator_kind": locator_kind,
        "locator": locator,
        "source_span_key": stable_digest([evidence_ref]),
        "proof_state": "current",
        "observed_at": now,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_action_evidence",
        "external_id": evidence_key,
        "source_url": source_url,
        "source_updated_at": now,
        "content_hash": stable_digest([evidence_ref, source_url]),
        "deletion_state": "present",
        "acl_state": "unavailable",
        "last_confirmed_at": now,
        "last_changed_at": now,
        "freshness_state": "fresh",
        "visibility": "unknown",
        "confidence": 1.0,
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, "evidences", evidence_values, "key")
    return int(conn.execute("select id from evidences where key = ?", (evidence_key,)).fetchone()[0])


def upsert_generated_evidence(
    conn: sqlite3.Connection,
    source_instance: str,
    claim_target_kind: str,
    claim_target_id: int,
    claim_field: str,
    locator_kind: str,
    locator: str,
    excerpt: str,
    now: str,
) -> int | None:
    locator_kind = first_nonempty([locator_kind])
    locator = first_nonempty([locator])
    if not locator_kind or not locator:
        return None
    excerpt = first_nonempty([excerpt])
    evidence_key = f"evidence:cubicle-analytics:{source_instance}:{stable_digest([claim_target_kind, claim_target_id, claim_field, locator_kind, locator])}"
    evidence_values = {
        "key": evidence_key,
        "claim_kind": "object_state",
        "claim_target_kind": claim_target_kind,
        "claim_target_id": claim_target_id,
        "claim_field": claim_field,
        "locator_kind": locator_kind,
        "locator": locator,
        "source_span_key": stable_digest([locator_kind, locator, excerpt]),
        "excerpt": excerpt,
        "proof_state": "current",
        "observed_at": now,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_generated_evidence",
        "external_id": evidence_key,
        "source_url": "",
        "source_updated_at": now,
        "content_hash": stable_digest([claim_target_kind, claim_target_id, locator_kind, locator, excerpt]),
        "deletion_state": "present",
        "acl_state": "unavailable",
        "last_confirmed_at": now,
        "last_changed_at": now,
        "freshness_state": "fresh",
        "visibility": "unknown",
        "confidence": 1.0,
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, "evidences", evidence_values, "key")
    return int(conn.execute("select id from evidences where key = ?", (evidence_key,)).fetchone()[0])


def parse_evidence_ref(evidence_ref: str, fallback_source_url: str = "") -> tuple[str, str, str]:
    parts = evidence_ref.split()
    if not parts:
        return "", "", fallback_source_url
    locator_kind = parts[0]
    source_url = fallback_source_url
    locator_parts = parts[1:]
    if locator_parts and locator_parts[-1].startswith(("http://", "https://")):
        source_url = locator_parts[-1]
        locator_parts = locator_parts[:-1]
    locator = " ".join(locator_parts) or evidence_ref
    return locator_kind, locator, source_url


def persist_work_blockers_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    action_items: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_blockers", "work_actions", "work_action_source_insights", "work_insights", "work_insight_reviews"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before blocker materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    pr_ids = ontology_pr_ids_by_subject(conn)
    ticket_ids = ontology_ticket_ids_by_subject(conn)
    action_ids = ontology_work_action_ids_by_key(conn, source_instance)
    candidate_actions = action_items[action_items.apply(lambda row: action_is_blocker_candidate(row), axis=1)].copy()
    current_blocker_keys: set[str] = set()
    if action_items.empty or candidate_actions.empty:
        delete_stale_work_blockers(conn, source_instance, current_blocker_keys)
        conn.commit()
        return

    for _, action in candidate_actions.iterrows():
        action_key = first_nonempty([action.get("action_key")])
        action_id = action_ids.get(action_key)
        if not action_key or action_id is None:
            continue
        subject_kind = first_nonempty([action.get("subject_kind")]) or "unknown"
        subject_key = first_nonempty([action.get("subject_key")])
        if not subject_key:
            continue
        source_insight = blocker_source_insight_for_action(conn, action_id)
        if source_insight is None:
            continue
        best_review = best_review_for_insight(conn, int(source_insight["id"]))
        if not blocker_materialization_ready(action, best_review):
            continue
        pull_request_id = pr_ids.get(subject_key) if subject_kind == "pull_request" else None
        ticket_id = ticket_ids.get(subject_key.upper()) if subject_kind == "ticket" else None
        source_coverage_state = source_coverage_state_for_register(action)
        blocker_kind = blocker_kind_for_action(action)
        blocker_identity = blocker_identity_key(subject_kind, subject_key, blocker_kind, source_insight)
        blocker_key = f"work-blocker:cubicle-analytics:{source_instance}:{stable_digest(blocker_identity)}"
        evidence_id = source_insight.get("latest_evidence_id")
        values = {
            "key": blocker_key,
            "blocker_kind": blocker_kind,
            "blocker_state": blocker_state_for_action(action),
            "severity": severity_for_blocker(action, source_insight),
            "subject_kind": subject_kind,
            "subject_key": subject_key,
            "pull_request_id": pull_request_id,
            "ticket_id": ticket_id,
            "work_action_id": action_id,
            "work_insight_id": int(source_insight["id"]),
            "owner_key": first_nonempty([action.get("owner_hint")]),
            "owner_source": "action_owner_hint" if first_nonempty([action.get("owner_hint")]) else "unassigned",
            "decision_state": first_nonempty([action.get("decision_state")]) or "pending_review",
            "source_coverage_state": source_coverage_state,
            "review_state": best_review.get("review_state", "requested"),
            "truth_label": best_review.get("truth_label", "unknown"),
            "actionability_label": best_review.get("actionability_label", "unknown"),
            "label_quality": best_review.get("label_quality", "unknown"),
            "measurement_eligible": bool(best_review.get("measurement_eligible", False)),
            "reviewer_kind": best_review.get("reviewer_kind", "system"),
            "reviewer_key": best_review.get("reviewer_key", ""),
            "label_set": best_review.get("label_set", ""),
            "title": first_nonempty([action.get("title"), source_insight.get("title"), f"Blocker: {subject_key}"]),
            "recommended_action": first_nonempty([action.get("recommended_action"), source_insight.get("recommended_action")]),
            "summary": first_nonempty([action.get("why_now"), source_insight.get("details"), action.get("evidence_summary")]),
            "search_text": " ".join(
                value
                for value in [
                    subject_key,
                    first_nonempty([action.get("title"), source_insight.get("title")]),
                    first_nonempty([action.get("evidence_summary")]),
                ]
                if value
            ),
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_blocker",
            "external_id": stable_external_id("work_blocker", blocker_identity),
            "source_url": first_nonempty([action.get("source_url"), source_insight.get("source_url")]),
            "source_updated_at": now,
            "content_hash": stable_digest(blocker_identity + [best_review.get("review_state", ""), best_review.get("truth_label", ""), action_key]),
            "deletion_state": "present",
            "acl_state": "unavailable",
            "last_confirmed_at": now,
            "last_changed_at": now,
            "latest_evidence_id": int(evidence_id) if evidence_id else None,
            "evidence_count": 1 if evidence_id else 0,
            "freshness_state": "fresh" if "observed" in source_coverage_state else "partial",
            "visibility": "unknown",
            "confidence": safe_float(action.get("confidence")) or safe_float(source_insight.get("confidence")),
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": safe_float(action.get("priority_score")) or safe_float(action.get("score")),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "work_blockers", values, "key")
        current_blocker_keys.add(blocker_key)
    delete_stale_work_blockers(conn, source_instance, current_blocker_keys)
    conn.commit()


def persist_work_dependency_edges_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    dependency_edges: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_dependency_edges"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before dependency materialization")

    conn.execute("pragma foreign_keys = on")
    ensure_work_dependency_endpoints_table(conn)
    now = generated_at or datetime.now(timezone.utc).isoformat()
    pr_ids = ontology_pr_ids_by_subject(conn)
    ticket_ids = ontology_ticket_ids_by_subject(conn)
    ticket_pr_evidence = ontology_ticket_pr_evidence_by_subject(conn)
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    default_workstream_id = workstream_ids.get("workstream:flink-kubernetes-operator")
    current_edge_keys: set[str] = set()
    current_endpoint_keys: set[str] = set()

    if not dependency_edges.empty:
        for _, row in dependency_edges.iterrows():
            source_node = parse_dependency_topology_node(row.get("source_key"))
            target_node = parse_dependency_topology_node(row.get("target_key"))
            if source_node is None or target_node is None:
                continue
            edge_kind = dependency_edge_kind(first_nonempty([row.get("edge_kind")]))
            if edge_kind in {"blocked_by", "needs_action"}:
                continue
            evidence_ref = ticket_pr_evidence_for_edge(edge_kind, source_node, target_node, ticket_pr_evidence)
            edge_key = upsert_work_dependency_edge(
                conn,
                source_instance,
                now,
                edge_kind,
                source_node,
                target_node,
                first_nonempty([row.get("risk_signal")]),
                first_nonempty([row.get("freshness")]) or "unknown",
                default_workstream_id,
                None,
                None,
                ticket_ids,
                pr_ids,
                source_url=first_nonempty([evidence_ref.get("source_url")]) if evidence_ref else "",
                latest_evidence_id=int(evidence_ref["latest_evidence_id"]) if evidence_ref and evidence_ref.get("latest_evidence_id") else None,
                evidence_count=int(evidence_ref.get("evidence_count") or 1) if evidence_ref else 0,
            )
            if edge_key:
                current_edge_keys.add(edge_key)
                current_endpoint_keys.update(
                    upsert_work_dependency_endpoints_for_edge(
                        conn,
                        source_instance,
                        now,
                        edge_key,
                        source_node,
                        target_node,
                        ticket_ids,
                        pr_ids,
                        workstream_ids,
                    )
                )

    for row in blocker_dependency_rows(conn, source_instance):
        subject_node = (row["subject_kind"], row["subject_key"])
        blocker_node = ("blocker", row["blocker_key"])
        action_node = ("action", row["action_key"])
        edge_key = upsert_work_dependency_edge(
            conn,
            source_instance,
            now,
            "blocked_by",
            subject_node,
            blocker_node,
            row["decision_state"],
            row["freshness_state"],
            default_workstream_id,
            row["blocker_id"],
            row["action_id"],
            ticket_ids,
            pr_ids,
            first_nonempty([row.get("source_url")]),
            row.get("latest_evidence_id"),
            int(row.get("evidence_count") or 0),
        )
        if edge_key:
            current_edge_keys.add(edge_key)
            current_endpoint_keys.update(
                upsert_work_dependency_endpoints_for_edge(
                    conn,
                    source_instance,
                    now,
                    edge_key,
                    subject_node,
                    blocker_node,
                    ticket_ids,
                    pr_ids,
                    workstream_ids,
                )
            )
        edge_key = upsert_work_dependency_edge(
            conn,
            source_instance,
            now,
            "needs_action",
            blocker_node,
            action_node,
            row["decision_state"],
            row["freshness_state"],
            default_workstream_id,
            row["blocker_id"],
            row["action_id"],
            ticket_ids,
            pr_ids,
            first_nonempty([row.get("source_url")]),
            row.get("latest_evidence_id"),
            int(row.get("evidence_count") or 0),
        )
        if edge_key:
            current_edge_keys.add(edge_key)
            current_endpoint_keys.update(
                upsert_work_dependency_endpoints_for_edge(
                    conn,
                    source_instance,
                    now,
                    edge_key,
                    blocker_node,
                    action_node,
                    ticket_ids,
                    pr_ids,
                    workstream_ids,
                )
            )
    delete_stale_work_dependency_endpoints(conn, source_instance, current_endpoint_keys)
    delete_stale_work_dependency_edges(conn, source_instance, current_edge_keys)
    conn.commit()


def action_is_blocker_candidate(row: pd.Series) -> bool:
    insight_kinds = set(split_csv(first_nonempty([row.get("source_link_insight_kinds"), row.get("insight_kinds")])))
    action_type = first_nonempty([row.get("action_type")])
    return action_type == "clear_blocker" and "blocker_candidate" in insight_kinds


def blocker_materialization_ready(action: pd.Series, best_review: dict[str, Any]) -> bool:
    if first_nonempty([action.get("action_type")]) != "clear_blocker":
        return False
    if first_nonempty([action.get("decision_state")]) != "product_action":
        return False
    if not trusted_measurement_review(best_review):
        return False
    if first_nonempty([best_review.get("truth_label")]) != "true_positive":
        return False
    if first_nonempty([best_review.get("actionability_label")]) not in POSITIVE_ACTIONABILITY:
        return False
    if first_nonempty([best_review.get("review_state")]) in {"dismissed", "requested"}:
        return False
    return True


def blocker_source_insight_for_action(conn: sqlite3.Connection, action_id: int) -> dict[str, Any] | None:
    row = conn.execute(
        """
        select
          wi.id,
          wi.key,
          wi.insight_kind,
          wi.severity,
          wi.title,
          wi.details,
          wi.recommended_action,
          wi.source_url,
          wi.latest_evidence_id,
          wi.confidence,
          wi.rank_score
        from work_action_source_insights wasi
        join work_insights wi on wi.id = wasi.work_insight_id
        where wasi.work_action_id = ?
          and wi.insight_kind = 'blocker_candidate'
        order by wi.rank_score desc, wi.updated_at desc
        limit 1
        """,
        (action_id,),
    ).fetchone()
    if row is None:
        return None
    columns = ["id", "key", "insight_kind", "severity", "title", "details", "recommended_action", "source_url", "latest_evidence_id", "confidence", "rank_score"]
    return dict(zip(columns, row))


def best_review_for_insight(conn: sqlite3.Connection, insight_id: int) -> dict[str, Any]:
    rows = conn.execute(
        """
        select
          review_kind,
          review_state,
          truth_label,
          actionability_label,
          label_quality,
          measurement_eligible,
          reviewer_kind,
          reviewer_key,
          label_set,
          next_action,
          rationale,
          reviewed_at,
          updated_at
        from work_insight_reviews
        where work_insight_id = ?
        """,
        (insight_id,),
    ).fetchall()
    if not rows:
        return {}
    columns = ["review_kind", "review_state", "truth_label", "actionability_label", "label_quality", "measurement_eligible", "reviewer_kind", "reviewer_key", "label_set", "next_action", "rationale", "reviewed_at", "updated_at"]
    reviews = [dict(zip(columns, row)) for row in rows]
    return max(reviews, key=work_insight_review_score)


def work_insight_review_score(row: dict[str, Any]) -> int:
    score = 0
    if first_nonempty([row.get("reviewer_kind")]) == "human":
        score += 20_000
    if trusted_measurement_review(row):
        score += 10_000
    score += {"gold": 1_000, "adversarial": 500, "smoke": 300, "candidate": 100}.get(first_nonempty([row.get("label_quality")]), 0)
    score += {"human_assessment": 500, "evaluation_label": 300, "triage_request": 50}.get(first_nonempty([row.get("review_kind")]), 0)
    score += {"accepted": 200, "resolved": 200, "dismissed": 200, "needs_more_data": 150, "requested": 20}.get(first_nonempty([row.get("review_state")]), 0)
    return score


def trusted_measurement_review(row: dict[str, Any]) -> bool:
    return is_measurement_label(
        pd.Series(
            {
                "stored_measurement_eligible": row.get("measurement_eligible"),
                "review_kind": row.get("review_kind"),
                "label_quality": row.get("label_quality"),
                "label_set": row.get("label_set"),
            }
        ),
        set(),
    )


def blocker_identity_key(subject_kind: str, subject_key: str, blocker_kind: str, source_insight: dict[str, Any]) -> list[str]:
    cause_key = first_nonempty(
        [
            source_insight.get("key"),
            source_insight.get("latest_evidence_id"),
            source_insight.get("source_url"),
            source_insight.get("title"),
        ]
    )
    return [subject_kind, subject_key, blocker_kind, cause_key]


def stable_external_id(external_kind: str, identity_parts: list[str]) -> str:
    return f"{external_kind}:{stable_digest(identity_parts)}"


def blocker_kind_for_action(action: pd.Series) -> str:
    action_type = first_nonempty([action.get("action_type")])
    if action_type == "ci_check_followup":
        return "ci"
    if action_type == "review_wait_followup":
        return "review"
    if action_type == "decision_or_owner_followup":
        return "decision"
    if action_type == "coordinate_workstream":
        return "dependency"
    return "source_signal"


def blocker_state_for_action(action: pd.Series) -> str:
    decision_state = first_nonempty([action.get("decision_state")])
    if decision_state == "product_action":
        return "active"
    if decision_state in {"closeout_review", SOURCE_RESOLVED_DECISION}:
        return "resolved"
    if decision_state == "suppressed_signal":
        return "dismissed"
    if decision_state == "validation_lead":
        return "validating"
    return "unknown"


def severity_for_blocker(action: pd.Series, source_insight: dict[str, Any]) -> str:
    severity = first_nonempty([action.get("severity"), source_insight.get("severity")])
    if severity in {"critical", "high", "medium", "low", "info"}:
        return severity
    urgency = first_nonempty([action.get("urgency")])
    if urgency in {"critical", "high", "medium", "low"}:
        return urgency
    return "medium"


def parse_dependency_topology_node(value: Any) -> tuple[str, str] | None:
    text = first_nonempty([value])
    if text.startswith("ticket:"):
        return ("ticket", text.removeprefix("ticket:").upper())
    if text.startswith("pr:"):
        return ("pull_request", text.removeprefix("pr:"))
    if text.startswith("component:"):
        return ("component", text)
    if text.startswith("workstream:"):
        return ("workstream", text)
    if text.startswith("work-blocker:"):
        return ("blocker", text)
    if text.startswith("tpm-action:"):
        return ("action", text)
    return None


def dependency_edge_kind(value: str) -> str:
    if value == "ticket_pr":
        return "ticket_pr"
    if value == "workstream_component":
        return "workstream_cluster"
    if value in {"blocked_by", "needs_action", "related_work", "workstream_member"}:
        return value
    return "related_work"


def dependency_edge_relationship_authority(edge_kind: str) -> str:
    if edge_kind == "ticket_pr":
        return "canonical_mirror"
    return "operating_projection"


def dependency_edge_canonical_relationship_kind(edge_kind: str) -> str:
    if edge_kind == "ticket_pr":
        return "ticket_pull_request"
    return ""


def upsert_work_dependency_edge(
    conn: sqlite3.Connection,
    source_instance: str,
    now: str,
    edge_kind: str,
    source_node: tuple[str, str],
    target_node: tuple[str, str],
    risk_signal: str,
    freshness: str,
    workstream_id: int | None,
    blocker_id: int | None,
    action_id: int | None,
    ticket_ids: dict[str, int],
    pr_ids: dict[str, int],
    source_url: str = "",
    latest_evidence_id: int | None = None,
    evidence_count: int = 0,
) -> str | None:
    from_kind, from_key = source_node
    to_kind, to_key = target_node
    if not from_key or not to_key:
        return None
    key = f"work-dependency-edge:cubicle-analytics:{source_instance}:{stable_digest([edge_kind, from_kind, from_key, to_kind, to_key])}"
    freshness_state = freshness if freshness in {"fresh", "partial", "stale", "unknown"} else "unknown"
    relationship_authority = dependency_edge_relationship_authority(edge_kind)
    canonical_relationship_kind = dependency_edge_canonical_relationship_kind(edge_kind)
    values = {
        "key": key,
        "edge_kind": edge_kind,
        "relationship_authority": relationship_authority,
        "canonical_relationship_kind": canonical_relationship_kind,
        "from_kind": from_kind,
        "from_key": from_key,
        "to_kind": to_kind,
        "to_key": to_key,
        "risk_signal": risk_signal,
        "source_coverage_state": freshness_state,
        "workstream_id": workstream_id,
        "work_blocker_id": blocker_id,
        "work_action_id": action_id,
        "ticket_id": endpoint_ticket_id(source_node, target_node, ticket_ids),
        "pull_request_id": endpoint_pr_id(source_node, target_node, pr_ids),
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_dependency_edge",
        "external_id": key,
        "source_url": source_url,
        "source_updated_at": now,
        "content_hash": stable_digest([edge_kind, relationship_authority, canonical_relationship_kind, from_kind, from_key, to_kind, to_key, risk_signal]),
        "deletion_state": "present",
        "acl_state": "unavailable",
        "last_confirmed_at": now,
        "last_changed_at": now,
        "latest_evidence_id": latest_evidence_id,
        "evidence_count": evidence_count,
        "freshness_state": freshness_state,
        "visibility": "unknown",
        "confidence": 0.8 if risk_signal else 0.9,
        "event_count": 1,
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": 100 if edge_kind in {"blocked_by", "needs_action"} else 50,
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, "work_dependency_edges", values, "key")
    row = conn.execute("select id from work_dependency_edges where key = ?", (key,)).fetchone()
    if row is not None and latest_evidence_id is None and evidence_count <= 0:
        row_id = int(row[0])
        generated_evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_dependency_edge",
            row_id,
            "edge",
            "tpm_dependency_edge",
            key,
            dependency_edge_evidence_excerpt(edge_kind, source_node, target_node, risk_signal, freshness_state),
            now,
        )
        if generated_evidence_id is not None:
            conn.execute(
                "update work_dependency_edges set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (generated_evidence_id, row_id),
            )
    return key


def ensure_work_dependency_endpoints_table(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table if not exists work_dependency_endpoints (
          id integer not null primary key autoincrement,
          key text not null unique,
          endpoint_role text not null,
          node_kind text not null,
          node_key text not null,
          resolution_state text not null default 'missing',
          resolution_reason text,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          source_version text,
          source_updated_at text,
          content_hash text,
          deletion_state text not null default 'present',
          deleted_at text,
          acl_policy_key text,
          visibility_hash text,
          acl_state text not null default 'unknown',
          acl_checked_at text,
          freshness_checked_at text,
          source_scope_state_id integer,
          last_confirmed_at text,
          last_changed_at text,
          evidence_count integer not null default 0,
          freshness_state text not null default 'unknown',
          visibility text not null default 'unknown',
          confidence real not null default 1,
          event_count integer not null default 0,
          first_seen_at text,
          last_activity_at text,
          rank_score real not null default 0,
          created_at text not null,
          updated_at text not null,
          work_dependency_edge_id integer not null,
          workstream_id integer,
          work_blocker_id integer,
          work_action_id integer,
          ticket_id integer,
          pull_request_id integer,
          latest_evidence_id integer,
          check(endpoint_role in ('from', 'to')),
          check(node_kind in ('workstream', 'ticket', 'pull_request', 'blocker', 'action', 'component')),
          check(resolution_state in ('resolved', 'key_only', 'missing')),
          check((node_kind = 'component' and resolution_state = 'key_only') or (node_kind != 'component' and resolution_state != 'key_only')),
          check(resolution_state != 'resolved' or ((node_kind = 'ticket' and ticket_id is not null and pull_request_id is null and workstream_id is null and work_blocker_id is null and work_action_id is null) or (node_kind = 'pull_request' and pull_request_id is not null and ticket_id is null and workstream_id is null and work_blocker_id is null and work_action_id is null) or (node_kind = 'workstream' and workstream_id is not null and ticket_id is null and pull_request_id is null and work_blocker_id is null and work_action_id is null) or (node_kind = 'blocker' and work_blocker_id is not null and ticket_id is null and pull_request_id is null and workstream_id is null and work_action_id is null) or (node_kind = 'action' and work_action_id is not null and ticket_id is null and pull_request_id is null and workstream_id is null and work_blocker_id is null))),
          foreign key(work_dependency_edge_id) references work_dependency_edges(id) on delete cascade,
          foreign key(workstream_id) references workstreams(id) on delete set null,
          foreign key(work_blocker_id) references work_blockers(id) on delete set null,
          foreign key(work_action_id) references work_actions(id) on delete set null,
          foreign key(ticket_id) references tickets(id) on delete set null,
          foreign key(pull_request_id) references pull_requests(id) on delete set null,
          foreign key(latest_evidence_id) references evidences(id) on delete set null
        )
        """
    )
    conn.execute("create unique index if not exists workdependencyendpoint_work_dependency_edge_id_endpoint_role on work_dependency_endpoints(work_dependency_edge_id, endpoint_role)")
    conn.execute("create index if not exists workdependencyendpoint_endpoint_role_node_kind_node_key on work_dependency_endpoints(endpoint_role, node_kind, node_key)")
    conn.execute("create index if not exists workdependencyendpoint_node_kind_node_key_resolution_state on work_dependency_endpoints(node_kind, node_key, resolution_state)")
    conn.execute("create index if not exists workdependencyendpoint_workstream_id_endpoint_role on work_dependency_endpoints(workstream_id, endpoint_role)")
    conn.execute("create index if not exists workdependencyendpoint_work_blocker_id_endpoint_role on work_dependency_endpoints(work_blocker_id, endpoint_role)")
    conn.execute("create index if not exists workdependencyendpoint_work_action_id_endpoint_role on work_dependency_endpoints(work_action_id, endpoint_role)")
    conn.execute("create index if not exists workdependencyendpoint_ticket_id_endpoint_role on work_dependency_endpoints(ticket_id, endpoint_role)")
    conn.execute("create index if not exists workdependencyendpoint_pull_request_id_endpoint_role on work_dependency_endpoints(pull_request_id, endpoint_role)")
    conn.execute("create unique index if not exists workdependencyendpoint_source_system_source_instance_external_kind_external_id on work_dependency_endpoints(source_system, source_instance, external_kind, external_id)")


def upsert_work_dependency_endpoints_for_edge(
    conn: sqlite3.Connection,
    source_instance: str,
    now: str,
    edge_key: str,
    source_node: tuple[str, str],
    target_node: tuple[str, str],
    ticket_ids: dict[str, int],
    pr_ids: dict[str, int],
    workstream_ids: dict[str, int],
) -> set[str]:
    if not table_exists(conn, "work_dependency_endpoints"):
        return set()
    row = conn.execute(
        """
        select
          id,
          workstream_id,
          work_blocker_id,
          work_action_id,
          latest_evidence_id,
          evidence_count,
          source_url,
          freshness_state,
          rank_score
        from work_dependency_edges
        where key = ?
        """,
        (edge_key,),
    ).fetchone()
    if row is None:
        return set()
    columns = [
        "id",
        "workstream_id",
        "work_blocker_id",
        "work_action_id",
        "latest_evidence_id",
        "evidence_count",
        "source_url",
        "freshness_state",
        "rank_score",
    ]
    edge = dict(zip(columns, row))
    endpoint_keys: set[str] = set()
    for endpoint_role, node in [("from", source_node), ("to", target_node)]:
        endpoint_key = upsert_work_dependency_endpoint(
            conn,
            source_instance,
            now,
            edge_key,
            int(edge["id"]),
            endpoint_role,
            node,
            edge,
            ticket_ids,
            pr_ids,
            workstream_ids,
        )
        if endpoint_key:
            endpoint_keys.add(endpoint_key)
    return endpoint_keys


def upsert_work_dependency_endpoint(
    conn: sqlite3.Connection,
    source_instance: str,
    now: str,
    edge_key: str,
    edge_id: int,
    endpoint_role: str,
    node: tuple[str, str],
    edge: dict[str, Any],
    ticket_ids: dict[str, int],
    pr_ids: dict[str, int],
    workstream_ids: dict[str, int],
) -> str | None:
    node_kind, node_key = node
    if not node_kind or not node_key:
        return None
    target = dependency_endpoint_target(node_kind, node_key, edge, ticket_ids, pr_ids, workstream_ids)
    key = f"work-dependency-endpoint:cubicle-analytics:{source_instance}:{stable_digest([edge_key, endpoint_role, node_kind, node_key])}"
    values = {
        "key": key,
        "endpoint_role": endpoint_role,
        "node_kind": node_kind,
        "node_key": node_key,
        "resolution_state": target["resolution_state"],
        "resolution_reason": target["resolution_reason"],
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_dependency_endpoint",
        "external_id": key,
        "source_url": first_nonempty([edge.get("source_url")]),
        "source_updated_at": now,
        "content_hash": stable_digest([edge_key, endpoint_role, node_kind, node_key, target["resolution_state"]]),
        "deletion_state": "present",
        "acl_state": "unavailable",
        "last_confirmed_at": now,
        "last_changed_at": now,
        "latest_evidence_id": edge.get("latest_evidence_id"),
        "evidence_count": int(edge.get("evidence_count") or 0),
        "freshness_state": first_nonempty([edge.get("freshness_state")]) or "unknown",
        "visibility": "unknown",
        "confidence": 0.9 if target["resolution_state"] == "resolved" else 0.7,
        "event_count": 1,
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": float(edge.get("rank_score") or 0),
        "created_at": now,
        "updated_at": now,
        "work_dependency_edge_id": edge_id,
        "workstream_id": target.get("workstream_id"),
        "work_blocker_id": target.get("work_blocker_id"),
        "work_action_id": target.get("work_action_id"),
        "ticket_id": target.get("ticket_id"),
        "pull_request_id": target.get("pull_request_id"),
    }
    upsert_row(conn, "work_dependency_endpoints", values, "key")
    return key


def dependency_endpoint_target(
    node_kind: str,
    node_key: str,
    edge: dict[str, Any],
    ticket_ids: dict[str, int],
    pr_ids: dict[str, int],
    workstream_ids: dict[str, int],
) -> dict[str, Any]:
    target = {
        "workstream_id": None,
        "work_blocker_id": None,
        "work_action_id": None,
        "ticket_id": None,
        "pull_request_id": None,
        "resolution_state": "missing",
        "resolution_reason": "no typed target row resolved",
    }
    if node_kind == "component":
        target["resolution_state"] = "key_only"
        target["resolution_reason"] = "component endpoint has no typed table yet"
        return target
    if node_kind == "ticket":
        target["ticket_id"] = ticket_ids.get(node_key.upper())
    elif node_kind == "pull_request":
        target["pull_request_id"] = pr_ids.get(node_key)
    elif node_kind == "workstream":
        target["workstream_id"] = workstream_ids.get(node_key)
    elif node_kind == "blocker":
        target["work_blocker_id"] = edge.get("work_blocker_id")
    elif node_kind == "action":
        target["work_action_id"] = edge.get("work_action_id")
    if any(target.get(column) is not None for column in ["workstream_id", "work_blocker_id", "work_action_id", "ticket_id", "pull_request_id"]):
        target["resolution_state"] = "resolved"
        target["resolution_reason"] = f"resolved to typed {node_kind} row"
    return target


def dependency_edge_evidence_excerpt(
    edge_kind: str,
    source_node: tuple[str, str],
    target_node: tuple[str, str],
    risk_signal: str,
    freshness_state: str,
) -> str:
    from_kind, from_key = source_node
    to_kind, to_key = target_node
    parts = [
        f"{edge_kind} dependency edge",
        f"{from_kind}:{from_key}",
        "->",
        f"{to_kind}:{to_key}",
    ]
    if risk_signal:
        parts.append(f"risk_signal={risk_signal}")
    if freshness_state:
        parts.append(f"source_coverage_state={freshness_state}")
    return " ".join(parts)


def delete_stale_work_blockers(conn: sqlite3.Connection, source_instance: str, current_blocker_keys: set[str]) -> None:
    if not table_exists(conn, "work_blockers"):
        return
    params: list[Any] = [source_instance]
    keep_clause = ""
    if current_blocker_keys:
        placeholders = ", ".join(["?"] * len(current_blocker_keys))
        keep_clause = f" and key not in ({placeholders})"
        params.extend(sorted(current_blocker_keys))
    stale_rows = conn.execute(
        f"""
        select id, key
          from work_blockers
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_blocker'
           {keep_clause}
        """,
        params,
    ).fetchall()
    if not stale_rows:
        return
    stale_ids = [int(row[0]) for row in stale_rows]
    stale_keys = [str(row[1]) for row in stale_rows if row[1]]
    delete_stale_blocker_topology_refs(conn, source_instance, stale_ids, stale_keys)
    placeholders = ", ".join(["?"] * len(stale_ids))
    conn.execute(f"delete from work_blockers where id in ({placeholders})", stale_ids)


def delete_stale_blocker_topology_refs(conn: sqlite3.Connection, source_instance: str, blocker_ids: list[int], blocker_keys: list[str]) -> None:
    if blocker_ids and table_exists(conn, "work_blocker_impacts"):
        placeholders = ", ".join(["?"] * len(blocker_ids))
        conn.execute(
            f"""
            delete from work_blocker_impacts
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_work_blocker_impact'
               and work_blocker_id in ({placeholders})
            """,
            [source_instance, *blocker_ids],
        )
    if not table_exists(conn, "work_dependency_edges"):
        return
    clauses: list[str] = []
    params: list[Any] = [source_instance]
    if blocker_ids:
        placeholders = ", ".join(["?"] * len(blocker_ids))
        clauses.append(f"work_blocker_id in ({placeholders})")
        params.extend(blocker_ids)
    if blocker_keys:
        placeholders = ", ".join(["?"] * len(blocker_keys))
        clauses.append(f"((from_kind = 'blocker' and from_key in ({placeholders})) or (to_kind = 'blocker' and to_key in ({placeholders})))")
        params.extend(blocker_keys)
        params.extend(blocker_keys)
    if not clauses:
        return
    stale_edge_ids = [
        int(row[0])
        for row in conn.execute(
            f"""
            select id
              from work_dependency_edges
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_work_dependency_edge'
               and ({' or '.join(clauses)})
            """,
            params,
        ).fetchall()
    ]
    if stale_edge_ids and table_exists(conn, "work_dependency_endpoints"):
        placeholders = ", ".join(["?"] * len(stale_edge_ids))
        conn.execute(f"delete from work_dependency_endpoints where work_dependency_edge_id in ({placeholders})", stale_edge_ids)
    conn.execute(
        f"""
        delete from work_dependency_edges
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_dependency_edge'
           and ({' or '.join(clauses)})
        """,
        params,
    )


def delete_stale_work_dependency_endpoints(conn: sqlite3.Connection, source_instance: str, current_endpoint_keys: set[str]) -> None:
    if not table_exists(conn, "work_dependency_endpoints"):
        return
    if current_endpoint_keys:
        placeholders = ", ".join(["?"] * len(current_endpoint_keys))
        conn.execute(
            f"""
            delete from work_dependency_endpoints
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_work_dependency_endpoint'
               and key not in ({placeholders})
            """,
            [source_instance, *sorted(current_endpoint_keys)],
        )
        return
    conn.execute(
        """
        delete from work_dependency_endpoints
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_dependency_endpoint'
        """,
        (source_instance,),
    )


def delete_stale_work_dependency_edges(conn: sqlite3.Connection, source_instance: str, current_edge_keys: set[str]) -> None:
    if not table_exists(conn, "work_dependency_edges"):
        return
    if current_edge_keys:
        placeholders = ", ".join(["?"] * len(current_edge_keys))
        conn.execute(
            f"""
            delete from work_dependency_edges
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_work_dependency_edge'
               and key not in ({placeholders})
            """,
            [source_instance, *sorted(current_edge_keys)],
        )
        return
    conn.execute(
        """
        delete from work_dependency_edges
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_dependency_edge'
        """,
        (source_instance,),
    )


def endpoint_ticket_id(source_node: tuple[str, str], target_node: tuple[str, str], ticket_ids: dict[str, int]) -> int | None:
    for kind, key in [source_node, target_node]:
        if kind == "ticket":
            return ticket_ids.get(key.upper())
    return None


def endpoint_pr_id(source_node: tuple[str, str], target_node: tuple[str, str], pr_ids: dict[str, int]) -> int | None:
    for kind, key in [source_node, target_node]:
        if kind == "pull_request":
            return pr_ids.get(key)
    return None


def ticket_pr_evidence_for_edge(
    edge_kind: str,
    source_node: tuple[str, str],
    target_node: tuple[str, str],
    ticket_pr_evidence: dict[tuple[str, str], dict[str, Any]],
) -> dict[str, Any] | None:
    if edge_kind != "ticket_pr":
        return None
    ticket_key = ""
    pr_key = ""
    for kind, key in [source_node, target_node]:
        if kind == "ticket":
            ticket_key = key.upper()
        elif kind == "pull_request":
            pr_key = key
    if not ticket_key or not pr_key:
        return None
    return ticket_pr_evidence.get((ticket_key, pr_key))


def blocker_dependency_rows(conn: sqlite3.Connection, source_instance: str) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_blockers"):
        return []
    rows = conn.execute(
        """
        select
          wb.id as blocker_id,
          wb.key as blocker_key,
          wb.subject_kind,
          wb.subject_key,
          wb.decision_state,
          wb.freshness_state,
          wb.source_url,
          wb.latest_evidence_id,
          wb.evidence_count,
          wa.id as action_id,
          wa.key as action_key
        from work_blockers wb
        join work_actions wa on wa.id = wb.work_action_id
        where wb.source_system = 'cubicle_analytics'
          and wb.source_instance = ?
        """,
        (source_instance,),
    ).fetchall()
    columns = [
        "blocker_id",
        "blocker_key",
        "subject_kind",
        "subject_key",
        "decision_state",
        "freshness_state",
        "source_url",
        "latest_evidence_id",
        "evidence_count",
        "action_id",
        "action_key",
    ]
    return [dict(zip(columns, row)) for row in rows]


def persist_work_blocker_impacts_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    generated_at: str,
) -> None:
    required = ["work_blocker_impacts", "work_blockers"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before blocker impact materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    pr_ids = ontology_pr_ids_by_subject(conn)
    ticket_ids = ontology_ticket_ids_by_subject(conn)
    current_impact_keys: set[str] = set()
    for blocker in blocker_impact_source_rows(conn, source_instance):
        impact_key = upsert_work_blocker_impact(
            conn,
            source_instance,
            now,
            blocker,
            "direct_subject",
            blocker["subject_kind"],
            blocker["subject_key"],
            0,
            None,
            pr_ids.get(blocker["subject_key"]) if blocker["subject_kind"] == "pull_request" else None,
            ticket_ids.get(str(blocker["subject_key"]).upper()) if blocker["subject_kind"] == "ticket" else None,
            blocker.get("source_coverage_state", ""),
        )
        if impact_key:
            current_impact_keys.add(impact_key)
        for workstream_row in blocker_workstream_impact_rows(conn, int(blocker["blocker_id"])):
            impact_key = upsert_work_blocker_impact(
                conn,
                source_instance,
                now,
                blocker,
                "workstream",
                "workstream",
                workstream_row["workstream_key"],
                1,
                int(workstream_row["workstream_id"]),
                None,
                None,
                first_nonempty([workstream_row.get("source_coverage_state"), blocker.get("source_coverage_state")]),
            )
            if impact_key:
                current_impact_keys.add(impact_key)
    delete_stale_work_blocker_impacts(conn, source_instance, current_impact_keys)
    conn.commit()


def blocker_impact_source_rows(conn: sqlite3.Connection, source_instance: str) -> list[dict[str, Any]]:
    rows = conn.execute(
        """
        select
          wb.id as blocker_id,
          wb.key as blocker_key,
          wb.blocker_kind,
          wb.blocker_state,
          wb.severity,
          wb.subject_kind,
          wb.subject_key,
          wb.pull_request_id,
          wb.ticket_id,
          wb.work_action_id,
          wb.owner_key,
          wb.owner_source,
          wb.decision_state,
          wb.source_coverage_state,
          wb.title,
          wb.summary,
          wb.recommended_action,
          wb.source_url,
          wb.latest_evidence_id,
          wb.evidence_count,
          wb.freshness_state,
          wb.visibility,
          wb.confidence,
          wb.rank_score,
          wb.last_activity_at,
          wa.key as action_key
        from work_blockers wb
        left join work_actions wa on wa.id = wb.work_action_id
        where wb.source_system = 'cubicle_analytics'
          and wb.source_instance = ?
        """,
        (source_instance,),
    ).fetchall()
    columns = [
        "blocker_id",
        "blocker_key",
        "blocker_kind",
        "blocker_state",
        "severity",
        "subject_kind",
        "subject_key",
        "pull_request_id",
        "ticket_id",
        "work_action_id",
        "owner_key",
        "owner_source",
        "decision_state",
        "source_coverage_state",
        "title",
        "summary",
        "recommended_action",
        "source_url",
        "latest_evidence_id",
        "evidence_count",
        "freshness_state",
        "visibility",
        "confidence",
        "rank_score",
        "last_activity_at",
        "action_key",
    ]
    return [dict(zip(columns, row)) for row in rows]


def blocker_workstream_impact_rows(conn: sqlite3.Connection, blocker_id: int) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_dependency_edges") or not table_exists(conn, "workstreams"):
        return []
    rows = conn.execute(
        """
        select distinct
          ws.id as workstream_id,
          ws.key as workstream_key,
          wde.source_coverage_state
        from work_dependency_edges wde
        join workstreams ws on ws.id = wde.workstream_id
        where wde.work_blocker_id = ?
          and wde.edge_kind in ('blocked_by', 'needs_action')
          and wde.workstream_id is not null
        """,
        (blocker_id,),
    ).fetchall()
    columns = ["workstream_id", "workstream_key", "source_coverage_state"]
    return [dict(zip(columns, row)) for row in rows]


def upsert_work_blocker_impact(
    conn: sqlite3.Connection,
    source_instance: str,
    now: str,
    blocker: dict[str, Any],
    impact_kind: str,
    affected_kind: str,
    affected_key: str,
    path_length: int,
    workstream_id: int | None,
    pull_request_id: int | None,
    ticket_id: int | None,
    source_coverage_state: str,
) -> str | None:
    blocker_key = first_nonempty([blocker.get("blocker_key")])
    if not blocker_key or not affected_key:
        return None
    identity = [blocker_key, impact_kind, affected_kind, affected_key]
    key = f"work-blocker-impact:cubicle-analytics:{source_instance}:{stable_digest(identity)}"
    impact_score = blocker_impact_score(blocker, path_length)
    values = {
        "key": key,
        "impact_kind": impact_kind,
        "impact_state": first_nonempty([blocker.get("blocker_state")]) or "unknown",
        "impact_score": impact_score,
        "severity": first_nonempty([blocker.get("severity")]) or "info",
        "blocker_kind": first_nonempty([blocker.get("blocker_kind")]) or "source_signal",
        "work_blocker_id": int(blocker["blocker_id"]),
        "work_action_id": blocker.get("work_action_id"),
        "workstream_id": workstream_id,
        "pull_request_id": pull_request_id,
        "ticket_id": ticket_id,
        "affected_kind": affected_kind,
        "affected_key": affected_key,
        "subject_kind": first_nonempty([blocker.get("subject_kind")]) or "unknown",
        "subject_key": first_nonempty([blocker.get("subject_key")]),
        "path_length": path_length,
        "source_coverage_state": source_coverage_state,
        "title": blocker_impact_title(blocker, impact_kind, affected_key),
        "recommended_action": first_nonempty([blocker.get("recommended_action")]),
        "summary": blocker_impact_summary(blocker, impact_kind, affected_key),
        "search_text": " ".join(
            value
            for value in [
                affected_key,
                first_nonempty([blocker.get("subject_key")]),
                first_nonempty([blocker.get("title")]),
                first_nonempty([blocker.get("summary")]),
            ]
            if value
        ),
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_blocker_impact",
        "external_id": stable_external_id("work_blocker_impact", identity),
        "source_url": first_nonempty([blocker.get("source_url")]),
        "latest_evidence_id": blocker.get("latest_evidence_id"),
        "evidence_count": int(blocker.get("evidence_count") or 0),
        "freshness_state": first_nonempty([blocker.get("freshness_state")]) or "unknown",
        "visibility": first_nonempty([blocker.get("visibility")]) or "unknown",
        "confidence": safe_float(blocker.get("confidence")),
        "event_count": 1,
        "first_seen_at": now,
        "last_activity_at": first_nonempty([blocker.get("last_activity_at")]) or now,
        "rank_score": impact_score,
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, "work_blocker_impacts", values, "key")
    return key


def delete_stale_work_blocker_impacts(conn: sqlite3.Connection, source_instance: str, current_impact_keys: set[str]) -> None:
    if not table_exists(conn, "work_blocker_impacts"):
        return
    if current_impact_keys:
        placeholders = ", ".join(["?"] * len(current_impact_keys))
        conn.execute(
            f"""
            delete from work_blocker_impacts
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_work_blocker_impact'
               and key not in ({placeholders})
            """,
            [source_instance, *sorted(current_impact_keys)],
        )
        return
    conn.execute(
        """
        delete from work_blocker_impacts
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_blocker_impact'
        """,
        (source_instance,),
    )


def blocker_impact_score(blocker: dict[str, Any], path_length: int) -> float:
    severity_bonus = {
        "critical": 40.0,
        "high": 30.0,
        "medium": 20.0,
        "low": 10.0,
        "info": 5.0,
    }.get(first_nonempty([blocker.get("severity")]), 0.0)
    state_bonus = {
        "active": 25.0,
        "validating": 10.0,
        "resolved": -20.0,
        "dismissed": -40.0,
    }.get(first_nonempty([blocker.get("blocker_state")]), 0.0)
    decision_bonus = {
        "product_action": 20.0,
        "validation_lead": 8.0,
        "closeout_review": -10.0,
        SOURCE_RESOLVED_DECISION: -20.0,
        "suppressed_signal": -30.0,
    }.get(first_nonempty([blocker.get("decision_state")]), 0.0)
    base = safe_float(blocker.get("rank_score"))
    return max(0.0, round(base + severity_bonus + state_bonus + decision_bonus - (path_length * 5.0), 2))


def blocker_impact_title(blocker: dict[str, Any], impact_kind: str, affected_key: str) -> str:
    blocker_title = first_nonempty([blocker.get("title")]) or f"Blocker: {first_nonempty([blocker.get('subject_key')])}"
    if impact_kind == "workstream":
        return f"{blocker_title} impacts {affected_key}"
    return blocker_title


def blocker_impact_summary(blocker: dict[str, Any], impact_kind: str, affected_key: str) -> str:
    summary = first_nonempty([blocker.get("summary")])
    if impact_kind == "workstream":
        suffix = f"Affects workstream {affected_key} through typed blocker topology."
        return f"{summary} {suffix}".strip() if summary else suffix
    return summary


def persist_work_forecast_evaluations_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    forecast_summary: pd.DataFrame,
    forecast_backtest: pd.DataFrame,
    forecast_risk_backtest: pd.DataFrame,
    decision_target_backtest: pd.DataFrame,
    time_series_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_forecast_evaluations", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before forecast materialization")
    if forecast_summary.empty and forecast_backtest.empty and forecast_risk_backtest.empty:
        return

    now = generated_at or datetime.now(timezone.utc).isoformat()
    delete_legacy_forecast_evaluation_ids(conn, source_instance, forecast_backtest)
    delete_current_forecast_evaluation_kind_rows(
        conn,
        source_instance,
        now,
        ["tpm_forecast_risk_backtest", "tpm_decision_target_backtest"],
    )
    summary_values = forecast_summary_values(source_instance, forecast_summary, forecast_backtest, time_series_summary, now)
    upsert_row(conn, "work_forecast_evaluations", summary_values, "key")
    summary_id = int(conn.execute("select id from work_forecast_evaluations where key = ?", (summary_values["key"],)).fetchone()[0])
    summary_evidence_id = upsert_generated_evidence(
        conn,
        source_instance,
        "work_forecast_evaluation",
        summary_id,
        "readiness_state",
        "forecast_backtest",
        "summary",
        first_nonempty([summary_values.get("readiness_reason"), summary_values.get("note")]),
        now,
    )
    if summary_evidence_id is not None:
        conn.execute(
            "update work_forecast_evaluations set latest_evidence_id = ?, evidence_count = 1 where id = ?",
            (summary_evidence_id, summary_id),
        )

    if not forecast_backtest.empty:
        for _, row in forecast_backtest.iterrows():
            evaluation = forecast_evaluation_kind(first_nonempty([row.get("evaluation")]))
            model_name = first_nonempty([row.get("model")]) or "unknown"
            fold = metric_row_int(row, "fold")
            external_id = f"{evaluation}:{model_name}:{fold}:{now}"
            values = base_forecast_evaluation_values(
                source_instance,
                f"work-forecast-evaluation:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
                evaluation,
                model_name,
                now,
            )
            values.update(
                {
                    "forecast_method": metric_text(forecast_summary, "forecast_method"),
                    "best_model_name": metric_text(forecast_summary, "backtest_best_model"),
                    "fold": fold,
                    "train_count": metric_row_int(row, "train_count"),
                    "test_count": metric_row_int(row, "test_count"),
                    "mae_days": metric_row_float(row, "mae_days"),
                    "median_error_days": metric_row_float(row, "median_error_days"),
                    "p75_error_days": metric_row_float(row, "p75_error_days"),
                    "max_error_days": metric_row_float(row, "max_error_days"),
                    "improvement_vs_median_pct": metric_row_float(row, "improvement_vs_median_pct"),
                    "ready_for_eta": forecast_row_effective_eta_ready(row, forecast_summary, time_series_summary),
                    "readiness_state": forecast_row_readiness_state(evaluation, row, forecast_summary, time_series_summary),
                    "readiness_reason": forecast_row_readiness_reason(evaluation, row, forecast_summary, time_series_summary),
                    "note": first_nonempty([row.get("note")]),
                    "external_id": external_id,
                    "event_count": 1,
                    "rank_score": forecast_row_rank_score(evaluation, row),
                }
            )
            upsert_row(conn, "work_forecast_evaluations", values, "key")
            row_id = int(conn.execute("select id from work_forecast_evaluations where key = ?", (values["key"],)).fetchone()[0])
            evidence_id = upsert_generated_evidence(
                conn,
                source_instance,
                "work_forecast_evaluation",
                row_id,
                "mae_days",
                "forecast_evaluation",
                external_id,
                first_nonempty([values.get("readiness_reason"), values.get("note")]),
                now,
            )
            if evidence_id is not None:
                conn.execute(
                    "update work_forecast_evaluations set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                    (evidence_id, row_id),
            )
    if not forecast_risk_backtest.empty:
        for _, row in forecast_risk_backtest.iterrows():
            metric = first_nonempty([row.get("metric")])
            if not metric:
                continue
            external_id = f"risk_triage:{metric}:{now}"
            values = base_forecast_evaluation_values(
                source_instance,
                f"work-forecast-evaluation:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
                "summary",
                "static_risk_triage",
                now,
            )
            values.update(
                {
                    "forecast_method": first_nonempty([row.get("method")]) or "static_risk_triage_backtest",
                    "best_model_name": "static_risk_triage",
                    "baseline_sample_count": metric_row_int(row, "sample_count"),
                    "test_count": metric_row_int(row, "sample_count"),
                    "ready_for_eta": False,
                    "readiness_state": "gated",
                    "readiness_reason": "Risk-triage backtest supports attention ranking only; ETA commitments remain gated.",
                    "note": forecast_risk_backtest_note(row),
                    "external_kind": "tpm_forecast_risk_backtest",
                    "external_id": external_id,
                    "event_count": 1,
                    "rank_score": forecast_risk_backtest_rank_score(metric),
                }
            )
            upsert_row(conn, "work_forecast_evaluations", values, "key")
            row_id = int(conn.execute("select id from work_forecast_evaluations where key = ?", (values["key"],)).fetchone()[0])
            evidence_id = upsert_generated_evidence(
                conn,
                source_instance,
                "work_forecast_evaluation",
                row_id,
                "readiness_state",
                "forecast_risk_backtest",
                external_id,
                values["note"],
                now,
            )
            if evidence_id is not None:
                conn.execute(
                    "update work_forecast_evaluations set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                    (evidence_id, row_id),
                )
    conn.commit()


def persist_work_decision_target_evaluations_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    decision_target_backtest: pd.DataFrame,
    generated_at: str,
) -> None:
    if decision_target_backtest.empty:
        return
    ensure_work_decision_target_evaluations_table(conn)
    if not table_exists(conn, "evidences"):
        raise RuntimeError("ontology DB is missing evidences; rerun the Ent migration/load before decision-target materialization")
    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    delete_current_forecast_evaluation_kind_rows(
        conn,
        source_instance,
        now,
        ["tpm_decision_target_backtest"],
    )
    conn.execute(
        """
        delete from work_decision_target_evaluations
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and evaluated_at = ?
           and external_kind = 'tpm_decision_target_evaluation'
        """,
        (source_instance, now),
    )
    coverage_gate = decision_target_coverage_gate(decision_target_backtest)
    for _, row in decision_target_backtest.iterrows():
        target_kind = first_nonempty([row.get("target_kind")]) or "decision_target"
        evaluation = first_nonempty([row.get("evaluation")]) or "decision_target_backtest"
        model_name = first_nonempty([row.get("model")]) or "unknown"
        coverage_stratum = first_nonempty([row.get("coverage_stratum")]) or ""
        fold = metric_row_int(row, "fold")
        external_id = f"{target_kind}:{evaluation}:{model_name}:{fold}:{coverage_stratum or 'all'}:{now}"
        ready_for_product_action = decision_target_product_action_allowed(row, coverage_gate)
        gate_state = decision_target_product_gate_state(row, coverage_gate)
        gate_reason = decision_target_product_gate_reason(row, coverage_gate)
        values = {
            "key": f"work-decision-target-evaluation:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "target_kind": target_kind,
            "evaluation_kind": evaluation,
            "model_name": model_name,
            "fold": fold,
            "train_count": metric_row_int(row, "train_count"),
            "test_count": metric_row_int(row, "test_count"),
            "positive_count": metric_row_int(row, "positive_count"),
            "baseline_positive_rate": metric_row_float(row, "baseline_positive_rate"),
            "precision_at_10pct": metric_row_float(row, "precision_at_10pct"),
            "lift_at_10pct": metric_row_float(row, "lift_at_10pct"),
            "roc_auc": metric_row_float(row, "roc_auc"),
            "average_precision": metric_row_float(row, "average_precision"),
            "coverage_stratum": coverage_stratum,
            "ready_for_product_action": ready_for_product_action,
            "product_action_gate_state": gate_state,
            "product_action_gate_reason": gate_reason,
            "note": first_nonempty([row.get("note")]),
            "evaluated_at": now,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_decision_target_evaluation",
            "external_id": external_id,
            "source_url": workstream_source_url("flink-kubernetes-operator"),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": decision_target_evaluation_confidence(row, coverage_gate),
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": decision_target_backtest_rank_score(row),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "work_decision_target_evaluations", values, "key")
        row_id = int(conn.execute("select id from work_decision_target_evaluations where key = ?", (values["key"],)).fetchone()[0])
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_decision_target_evaluation",
            row_id,
            "product_action_gate_state",
            "decision_target_evaluation",
            external_id,
            first_nonempty([values.get("note"), gate_reason]),
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (values["confidence"], evidence_id),
            )
            conn.execute(
                "update work_decision_target_evaluations set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, row_id),
            )
    conn.commit()


def delete_legacy_forecast_evaluation_ids(conn: sqlite3.Connection, source_instance: str, forecast_backtest: pd.DataFrame) -> None:
    legacy_ids = {"summary"}
    if not forecast_backtest.empty:
        for _, row in forecast_backtest.iterrows():
            evaluation = forecast_evaluation_kind(first_nonempty([row.get("evaluation")]))
            model_name = first_nonempty([row.get("model")]) or "unknown"
            fold = metric_row_int(row, "fold")
            legacy_ids.add(f"{evaluation}:{model_name}:{fold}")
    placeholders = ", ".join(["?"] * len(legacy_ids))
    conn.execute(
        f"""
        delete from work_forecast_evaluations
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_forecast_evaluation'
           and external_id in ({placeholders})
        """,
        [source_instance, *sorted(legacy_ids)],
    )


def ensure_work_decision_target_evaluations_table(conn: sqlite3.Connection) -> None:
    conn.execute(
        """
        create table if not exists work_decision_target_evaluations (
          id integer primary key autoincrement,
          key text not null unique,
          target_kind text not null,
          evaluation_kind text not null,
          model_name text not null,
          fold integer not null default 0,
          train_count integer not null default 0,
          test_count integer not null default 0,
          positive_count integer not null default 0,
          baseline_positive_rate real,
          precision_at_10pct real,
          lift_at_10pct real,
          roc_auc real,
          average_precision real,
          coverage_stratum text,
          ready_for_product_action bool not null default false,
          product_action_gate_state text not null,
          product_action_gate_reason text not null,
          note text,
          evaluated_at datetime,
          source_system text,
          source_instance text,
          external_kind text,
          external_id text,
          source_url text,
          evidence_count integer not null default 0,
          freshness_state text not null default 'unknown',
          visibility text not null default 'unknown',
          confidence real not null default 1.0,
          event_count integer not null default 0,
          first_seen_at datetime,
          last_activity_at datetime,
          rank_score real not null default 0,
          created_at datetime not null,
          updated_at datetime not null,
          latest_evidence_id integer,
          foreign key(latest_evidence_id) references evidences(id) on delete set null
        )
        """
    )
    conn.execute(
        """
        create unique index if not exists work_decision_target_evaluations_source_identity
        on work_decision_target_evaluations(source_system, source_instance, external_kind, external_id)
        """
    )
    conn.execute(
        """
        create index if not exists work_decision_target_evaluations_target_model
        on work_decision_target_evaluations(target_kind, evaluation_kind, model_name, fold)
        """
    )
    conn.execute(
        """
        create index if not exists work_decision_target_evaluations_product_gate
        on work_decision_target_evaluations(ready_for_product_action, product_action_gate_state, evaluated_at)
        """
    )


def delete_current_forecast_evaluation_kind_rows(
    conn: sqlite3.Connection,
    source_instance: str,
    evaluated_at: str,
    external_kinds: list[str],
) -> None:
    if not external_kinds:
        return
    placeholders = ", ".join(["?"] * len(external_kinds))
    conn.execute(
        f"""
        delete from work_forecast_evaluations
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and evaluated_at = ?
           and external_kind in ({placeholders})
        """,
        [source_instance, evaluated_at, *external_kinds],
    )


def decision_target_coverage_gate(decision_target_backtest: pd.DataFrame) -> dict[str, str]:
    gate = {
        "state": "missing_coverage_guardrail",
        "reason": "No coverage-stratified guardrail row is available for decision-target product-action readiness.",
        "coverage_stratum": "",
    }
    if decision_target_backtest.empty:
        return gate
    summary_rows = []
    guardrail_rows = []
    for _, row in decision_target_backtest.iterrows():
        evaluation = first_nonempty([row.get("evaluation")])
        model = first_nonempty([row.get("model")])
        if evaluation == "source_event_as_of_coverage_stratified_summary":
            summary_rows.append(row)
        elif model == "coverage_guardrail":
            guardrail_rows.append(row)
    coverage_rows = summary_rows or guardrail_rows
    if not coverage_rows:
        return gate
    coverage = coverage_rows[0]
    coverage_stratum = first_nonempty([coverage.get("coverage_stratum")])
    note = first_nonempty([coverage.get("note")])
    gate["coverage_stratum"] = coverage_stratum
    gate["reason"] = note or "Coverage-stratified decision-target validation has not cleared product-action gates."
    text = " ".join(
        [
            coverage_stratum,
            first_nonempty([coverage.get("product_action_gate_state")]),
            note,
        ]
    ).lower()
    if "not_testable" in text or "insufficient" in text or "cannot be tested" in text:
        gate["state"] = "validation_gated"
        return gate
    if (
        decision_target_row_declares_ready(coverage)
        and decision_target_gate_passed(first_nonempty([coverage.get("product_action_gate_state")]))
        and decision_target_has_independent_product_evidence(coverage)
    ):
        gate["state"] = "passed"
        gate["reason"] = "Coverage-stratified decision-target validation passed."
        return gate
    if decision_target_row_declares_ready(coverage) and decision_target_gate_passed(first_nonempty([coverage.get("product_action_gate_state")])):
        gate["state"] = "validation_gated"
        gate["reason"] = (
            "Coverage-stratified decision-target validation is generated validation evidence only; "
            "require independent source evidence or human measurement before product action."
        )
        return gate
    gate["state"] = first_nonempty([coverage.get("product_action_gate_state")]) or "validation_gated"
    return gate


def decision_target_row_declares_ready(row: pd.Series) -> bool:
    return first_nonempty([row.get("ready_for_product_action")]).lower() == "true"


def decision_target_gate_passed(value: str) -> bool:
    return value.strip().lower() in {"passed", "ready", "product_action_ready"}


def decision_target_has_independent_product_evidence(row: pd.Series) -> bool:
    evidence_kind = first_nonempty(
        [
            row.get("product_action_evidence_kind"),
            row.get("independent_evidence_kind"),
            row.get("evidence_kind"),
        ]
    ).strip().lower()
    if evidence_kind in {"human_review", "source_evidence", "measurement", "gold_label"}:
        return True
    return False


def decision_target_product_action_allowed(row: pd.Series, coverage_gate: dict[str, str]) -> bool:
    return (
        decision_target_row_declares_ready(row)
        and coverage_gate.get("state") == "passed"
        and decision_target_has_independent_product_evidence(row)
        and decision_target_gate_passed(first_nonempty([row.get("product_action_gate_state")]))
    )


def decision_target_product_gate_state(row: pd.Series, coverage_gate: dict[str, str] | None = None) -> str:
    coverage_gate = coverage_gate or {"state": "missing_coverage_guardrail", "reason": ""}
    if decision_target_product_action_allowed(row, coverage_gate):
        return "passed"
    if coverage_gate.get("state") != "passed":
        return coverage_gate.get("state") or "validation_gated"
    if not decision_target_has_independent_product_evidence(row):
        return "validation_gated"
    evaluation = first_nonempty([row.get("evaluation")])
    coverage_stratum = first_nonempty([row.get("coverage_stratum")])
    if evaluation == "insufficient_sample":
        return "insufficient_sample"
    if coverage_stratum.startswith("not_testable") or coverage_stratum.startswith("insufficient"):
        return "validation_gated"
    return "gated"


def decision_target_product_gate_reason(row: pd.Series, coverage_gate: dict[str, str] | None = None) -> str:
    note = first_nonempty([row.get("note")])
    coverage_gate = coverage_gate or {"state": "missing_coverage_guardrail", "reason": ""}
    if decision_target_product_action_allowed(row, coverage_gate):
        return "Decision-target evaluation has passed product-action gates."
    if coverage_gate.get("state") != "passed":
        return coverage_gate.get("reason") or note or "Coverage-stratified decision-target validation has not cleared product-action gates."
    if not decision_target_has_independent_product_evidence(row):
        return "Decision-target evaluation is generated validation evidence only; require independent source evidence or human measurement before product action."
    if note:
        return note
    return "TPM decision-target evaluation is validation evidence only; owner decisions remain human-approved and product-action gated."


def decision_target_evaluation_confidence(row: pd.Series, coverage_gate: dict[str, str] | None = None) -> float:
    coverage_gate = coverage_gate or {"state": "missing_coverage_guardrail", "reason": ""}
    if decision_target_product_action_allowed(row, coverage_gate):
        return 0.95
    if decision_target_row_declares_ready(row) and coverage_gate.get("state") == "passed":
        return 0.75
    test_count = metric_row_int(row, "test_count")
    if first_nonempty([row.get("evaluation")]) == "insufficient_sample" or test_count <= 0:
        return 0.25
    if first_nonempty([row.get("coverage_stratum")]).startswith("not_testable"):
        return 0.45
    return 0.7


def persist_work_item_forecasts_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    pr_features: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    time_series_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_item_forecasts", "pull_requests", "work_actions", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before work item forecast materialization")
    if pr_features.empty:
        return

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    pr_ids = ontology_pr_ids_by_subject(conn)
    forecast_action_ids = ontology_forecast_action_ids_by_subject(conn, source_instance)
    eta_ready = forecast_effective_eta_ready(forecast_summary, time_series_summary)
    baseline_count = metric_int(forecast_summary, "merged_pr_count")
    readiness_state = "ready" if eta_ready else "gated"
    if baseline_count < 10:
        readiness_state = "insufficient_sample"
    best_model = metric_text(forecast_summary, "backtest_best_model")
    forecast_method = metric_text(forecast_summary, "forecast_method") or "unknown"
    model_name = best_model or forecast_method
    observed_snapshot_time_count = metric_int(time_series_summary, "observed_snapshot_time_count")
    transition_candidate_count = metric_int(time_series_summary, "transition_candidate_count")
    readiness_reason = forecast_readiness_reason(
        forecast_summary,
        baseline_count,
        eta_ready,
        best_model,
        observed_snapshot_time_count,
        transition_candidate_count,
    )

    for _, row in pr_features.iterrows():
        repository = first_nonempty([row.get("repository")])
        pr_number = metric_row_int(row, "pr_number")
        if not repository or pr_number <= 0:
            continue
        subject_key = f"{repository}#{pr_number}"
        pull_request_id = pr_ids.get(subject_key)
        if pull_request_id is None:
            continue
        risk_band = forecast_risk_band(first_nonempty([row.get("risk_band")]))
        risk_score = safe_float(row.get("risk_score"))
        values = {
            "key": f"work-item-forecast:cubicle-analytics:{source_instance}:{stable_digest([subject_key, 'cycle_time'])}",
            "forecast_kind": "cycle_time",
            "subject_kind": "pull_request",
            "subject_key": subject_key,
            "pull_request_id": pull_request_id,
            "ticket_id": None,
            "work_action_id": forecast_action_ids.get(subject_key),
            "subject_state": first_nonempty([row.get("state")]),
            "forecast_method": first_nonempty([row.get("forecast_method")]) or forecast_method,
            "model_name": model_name,
            "age_days": metric_row_float(row, "age_days"),
            "predicted_total_cycle_days": metric_row_float(row, "predicted_total_cycle_days"),
            "predicted_remaining_days": metric_row_float(row, "predicted_remaining_days"),
            "overdue_days": metric_row_float(row, "overdue_days"),
            "risk_score": risk_score,
            "risk_band": risk_band,
            "readiness_state": readiness_state,
            "ready_for_eta": eta_ready,
            "readiness_reason": readiness_reason,
            "forecasted_at": now,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_pr_forecast",
            "external_id": subject_key,
            "source_url": first_nonempty([row.get("pr_url")]),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "unknown",
            "confidence": 1.0,
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": risk_score,
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "work_item_forecasts", values, "key")
        attach_work_item_forecast_evidence(conn, source_instance, values, now)
    conn.commit()


def ontology_forecast_action_ids_by_subject(conn: sqlite3.Connection, source_instance: str) -> dict[str, int]:
    if not table_exists(conn, "work_actions"):
        return {}
    rows = conn.execute(
        """
        select id, subject_key
          from work_actions
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_action'
           and subject_kind = 'pull_request'
           and action_state = 'open'
         order by
           case action_type
             when 'decision_or_owner_followup' then 0
             when 'clear_blocker' then 1
             when 'review_wait_followup' then 2
             when 'ci_check_followup' then 3
             when 'validate_signal' then 4
             else 9
           end,
           rank_score desc,
           updated_at desc,
           id desc
        """,
        (source_instance,),
    ).fetchall()
    out: dict[str, int] = {}
    for action_id, subject_key in rows:
        key = first_nonempty([subject_key])
        if key and key not in out:
            out[key] = int(action_id)
    return out


def attach_work_item_forecast_evidence(
    conn: sqlite3.Connection,
    source_instance: str,
    values: dict[str, Any],
    now: str,
) -> None:
    row = conn.execute("select id from work_item_forecasts where key = ?", (values["key"],)).fetchone()
    if row is None:
        return
    forecast_id = int(row[0])
    subject_key = first_nonempty([values.get("subject_key")])
    locator = first_nonempty([values.get("external_id"), subject_key, values.get("key")])
    excerpt = work_item_forecast_evidence_excerpt(values)
    evidence_id = upsert_generated_evidence(
        conn,
        source_instance,
        "work_item_forecast",
        forecast_id,
        "risk_band",
        "tpm_pr_forecast",
        locator,
        excerpt,
        now,
    )
    if evidence_id is not None:
        conn.execute(
            "update work_item_forecasts set latest_evidence_id = ?, evidence_count = 1 where id = ?",
            (evidence_id, forecast_id),
        )


def work_item_forecast_evidence_excerpt(values: dict[str, Any]) -> str:
    subject_kind = first_nonempty([values.get("subject_kind")]) or "work_item"
    subject_key = first_nonempty([values.get("subject_key")]) or "unknown"
    risk_band = first_nonempty([values.get("risk_band")]) or "unknown"
    risk_score = safe_float(values.get("risk_score"))
    readiness_state = first_nonempty([values.get("readiness_state")]) or "unknown"
    eta_phrase = "ETA-ready" if bool(values.get("ready_for_eta")) else "ETA-gated"
    parts = [f"{subject_kind} {subject_key} forecast risk {risk_band} (score {risk_score:.1f}); {readiness_state}; {eta_phrase}."]
    age_days = optional_float(values.get("age_days"))
    predicted_total = optional_float(values.get("predicted_total_cycle_days"))
    overdue_days = optional_float(values.get("overdue_days"))
    if age_days is not None:
        parts.append(f"Age {age_days:.1f}d.")
    if predicted_total is not None:
        parts.append(f"Predicted total cycle {predicted_total:.1f}d.")
    if overdue_days is not None and overdue_days > 0:
        parts.append(f"Over baseline by {overdue_days:.1f}d.")
    readiness_reason = first_nonempty([values.get("readiness_reason")])
    if readiness_reason:
        parts.append(readiness_reason)
    return " ".join(parts)


def persist_work_item_state_snapshots_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    pr_state_snapshots: pd.DataFrame,
    ticket_state_snapshots: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_item_state_snapshots", "pull_requests", "tickets", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before work item state snapshot materialization")
    if pr_state_snapshots.empty and ticket_state_snapshots.empty:
        return

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    pr_ids = ontology_pr_ids_by_subject(conn)
    ticket_ids = ontology_ticket_ids_by_subject(conn)

    if not pr_state_snapshots.empty:
        for _, row in pr_state_snapshots.iterrows():
            subject_key = first_nonempty([row.get("subject_key")])
            pull_request_id = pr_ids.get(subject_key)
            if not subject_key or pull_request_id is None:
                continue
            values = base_work_item_state_snapshot_values(
                source_instance,
                first_nonempty([row.get("snapshot_key")]),
                "tpm_pr_state_snapshot",
                "pull_request",
                subject_key,
                now,
            )
            values.update(
                {
                    "pull_request_id": pull_request_id,
                    "ticket_id": None,
                    "state": first_nonempty([row.get("state")]),
                    "title": first_nonempty([row.get("title")]),
                    "observed_at": first_nonempty([row.get("observed_at")]),
                    "captured_at": first_nonempty([row.get("captured_at")]) or now,
                    "source_created_at": first_nonempty([row.get("source_created_at")]),
                    "source_updated_at": first_nonempty([row.get("source_updated_at")]),
                    "closed_at": first_nonempty([row.get("closed_at")]),
                    "merged_at": first_nonempty([row.get("merged_at")]),
                    "age_days": metric_row_float(row, "age_days"),
                    "stale_days": metric_row_float(row, "stale_days"),
                    "cycle_time_days": metric_row_float(row, "cycle_time_days"),
                    "risk_score": safe_float(row.get("risk_score")),
                    "risk_band": forecast_risk_band(first_nonempty([row.get("risk_band")])),
                    "forecast_method": first_nonempty([row.get("forecast_method")]),
                    "source_current_coverage_state": first_nonempty([row.get("source_current_coverage_state")]),
                    "source_current_detail_state": first_nonempty([row.get("source_current_detail_state")]),
                    "source_current_issue_codes": first_nonempty([row.get("source_current_issue_codes")]),
                    "source_current_issue_kinds": first_nonempty([row.get("source_current_issue_kinds")]),
                    "lifecycle_fields_source": first_nonempty([row.get("lifecycle_fields_source")]),
                    "churn_fields_source": first_nonempty([row.get("churn_fields_source")]),
                    "mergeability_fields_source": first_nonempty([row.get("mergeability_fields_source")]),
                    "source_url": first_nonempty([row.get("pr_url")]),
                    "last_activity_at": first_nonempty([row.get("observed_at")]) or now,
                    "rank_score": safe_float(row.get("risk_score")),
                }
            )
            upsert_row(conn, "work_item_state_snapshots", values, "key")
            attach_work_item_state_snapshot_evidence(conn, source_instance, values, now)

    if not ticket_state_snapshots.empty:
        for _, row in ticket_state_snapshots.iterrows():
            ticket_key = first_nonempty([row.get("ticket_key")]).upper()
            ticket_id = ticket_ids.get(ticket_key)
            if not ticket_key or ticket_id is None:
                continue
            values = base_work_item_state_snapshot_values(
                source_instance,
                first_nonempty([row.get("snapshot_key")]),
                "tpm_ticket_state_snapshot",
                "ticket",
                ticket_key,
                now,
            )
            values.update(
                {
                    "pull_request_id": None,
                    "ticket_id": ticket_id,
                    "state": first_nonempty([row.get("status")]),
                    "title": first_nonempty([row.get("title")]),
                    "observed_at": first_nonempty([row.get("observed_at")]),
                    "captured_at": first_nonempty([row.get("captured_at")]) or now,
                    "source_updated_at": first_nonempty([row.get("updated_at")]),
                    "priority": first_nonempty([row.get("priority")]),
                    "linked_pr_count": metric_row_int(row, "linked_pr_count"),
                    "fresh_pr_link_count": metric_row_int(row, "fresh_pr_link_count"),
                    "partial_pr_link_count": metric_row_int(row, "partial_pr_link_count"),
                    "comment_count": metric_row_int(row, "comment_count"),
                    "participant_count": metric_row_int(row, "participant_count"),
                    "blocker_keyword_count": metric_row_int(row, "blocker_keyword_count"),
                    "last_activity_at": first_nonempty([row.get("observed_at")]) or now,
                    "rank_score": float(metric_row_int(row, "blocker_keyword_count")),
                }
            )
            upsert_row(conn, "work_item_state_snapshots", values, "key")
            attach_work_item_state_snapshot_evidence(conn, source_instance, values, now)
    conn.commit()


def persist_work_item_state_transitions_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    transition_candidates: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_item_state_transitions", "work_item_state_snapshots", "pull_requests", "tickets", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before work item transition materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    transition_candidates = source_scoped_transition_candidates(transition_candidates, source_instance)
    pr_ids = ontology_pr_ids_by_subject(conn)
    ticket_ids = ontology_ticket_ids_by_subject(conn)
    snapshot_ids = ontology_snapshot_ids_by_subject_time(conn)
    current_transition_keys = current_work_item_state_transition_keys(transition_candidates, source_instance, pr_ids, ticket_ids)
    delete_stale_work_item_state_transitions(conn, source_instance, current_transition_keys)
    if transition_candidates.empty:
        conn.commit()
        return

    for _, row in transition_candidates.iterrows():
        subject_kind = normalized_subject_kind(first_nonempty([row.get("subject_kind")]))
        subject_key = first_nonempty([row.get("subject_key")])
        if not subject_key or subject_kind not in {"pull_request", "ticket"}:
            continue
        pull_request_id = pr_ids.get(subject_key) if subject_kind == "pull_request" else None
        ticket_id = ticket_ids.get(subject_key.upper()) if subject_kind == "ticket" else None
        if subject_kind == "pull_request" and pull_request_id is None:
            continue
        if subject_kind == "ticket" and ticket_id is None:
            continue

        from_observed_at = first_nonempty([row.get("from_observed_at")])
        to_observed_at = first_nonempty([row.get("to_observed_at")])
        from_snapshot_id = snapshot_ids.get((subject_kind, subject_key, from_observed_at))
        to_snapshot_id = snapshot_ids.get((subject_kind, subject_key, to_observed_at))
        transition_kind = work_item_transition_kind(first_nonempty([row.get("transition_kind")]))
        terminal = transition_kind == "terminal_state_change" or first_nonempty([row.get("to_state")]).lower() in {"merged", "closed", "resolved", "done"}
        transition_key = first_nonempty([row.get("transition_key")])
        values = {
            "key": f"work-item-state-transition:cubicle-analytics:{source_instance}:{stable_digest([transition_key])}",
            "subject_kind": subject_kind,
            "subject_key": subject_key,
            "pull_request_id": pull_request_id,
            "ticket_id": ticket_id,
            "from_snapshot_id": from_snapshot_id,
            "to_snapshot_id": to_snapshot_id,
            "from_observed_at": from_observed_at,
            "to_observed_at": to_observed_at,
            "from_state": first_nonempty([row.get("from_state")]),
            "to_state": first_nonempty([row.get("to_state")]),
            "transition_kind": transition_kind,
            "transition_confidence": safe_float(row.get("confidence")),
            "confidence_basis": transition_confidence_basis(row),
            "verification_state": transition_verification_state(transition_kind, terminal),
            "terminal": terminal,
            "requires_closeout": terminal,
            "note": first_nonempty([row.get("note")]),
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_state_transition_candidate",
            "external_id": transition_key,
            "source_url": "",
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "unknown",
            "confidence": safe_float(row.get("confidence")),
            "event_count": 1,
            "first_seen_at": first_nonempty([row.get("created_at")]) or now,
            "last_activity_at": to_observed_at or now,
            "rank_score": 95.0 if terminal else 65.0,
            "created_at": first_nonempty([row.get("created_at")]) or now,
            "updated_at": first_nonempty([row.get("updated_at")]) or now,
        }
        if existing_verified_work_item_state_transition(conn, values["key"]):
            mark_verified_transition_seen(conn, values["key"], values, now)
            continue
        upsert_row(conn, "work_item_state_transitions", values, "key")
        attach_work_item_state_transition_evidence(conn, source_instance, values, now)
    conn.commit()


def current_work_item_state_transition_keys(
    transition_candidates: pd.DataFrame,
    source_instance: str,
    pr_ids: dict[str, int],
    ticket_ids: dict[str, int],
) -> set[str]:
    keys: set[str] = set()
    transition_candidates = source_scoped_transition_candidates(transition_candidates, source_instance)
    if transition_candidates.empty:
        return keys
    for _, row in transition_candidates.iterrows():
        subject_kind = normalized_subject_kind(first_nonempty([row.get("subject_kind")]))
        subject_key = first_nonempty([row.get("subject_key")])
        if not subject_key or subject_kind not in {"pull_request", "ticket"}:
            continue
        if subject_kind == "pull_request" and pr_ids.get(subject_key) is None:
            continue
        if subject_kind == "ticket" and ticket_ids.get(subject_key.upper()) is None:
            continue
        transition_key = first_nonempty([row.get("transition_key")])
        if not transition_key:
            continue
        keys.add(f"work-item-state-transition:cubicle-analytics:{source_instance}:{stable_digest([transition_key])}")
    return keys


def delete_stale_work_item_state_transitions(conn: sqlite3.Connection, source_instance: str, current_transition_keys: set[str]) -> None:
    if current_transition_keys:
        placeholders = ", ".join(["?"] * len(current_transition_keys))
        conn.execute(
            f"""
            delete from work_item_state_transitions
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_state_transition_candidate'
               and coalesce(verification_state, 'candidate') not in ('human_verified', 'source_verified')
               and key not in ({placeholders})
            """,
            [source_instance, *sorted(current_transition_keys)],
        )
        return
    conn.execute(
        """
        delete from work_item_state_transitions
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_state_transition_candidate'
           and coalesce(verification_state, 'candidate') not in ('human_verified', 'source_verified')
        """,
        (source_instance,),
    )


def source_scoped_transition_candidates(transition_candidates: pd.DataFrame, source_instance: str) -> pd.DataFrame:
    if transition_candidates.empty or not source_instance or "source_instance" not in transition_candidates.columns:
        return transition_candidates
    scoped = transition_candidates[transition_candidates["source_instance"].astype(str) == str(source_instance)].copy()
    return scoped.reset_index(drop=True)


def existing_verified_work_item_state_transition(conn: sqlite3.Connection, transition_key: str) -> bool:
    row = conn.execute(
        "select verification_state from work_item_state_transitions where key = ?",
        (transition_key,),
    ).fetchone()
    return row is not None and str(row[0]) in {"human_verified", "source_verified"}


def mark_verified_transition_seen(conn: sqlite3.Connection, transition_key: str, values: dict[str, Any], now: str) -> None:
    conn.execute(
        """
        update work_item_state_transitions
           set freshness_state = 'fresh',
               last_activity_at = coalesce(?, last_activity_at),
               updated_at = ?
         where key = ?
           and coalesce(verification_state, 'candidate') in ('human_verified', 'source_verified')
        """,
        (values.get("last_activity_at"), now, transition_key),
    )


def normalized_subject_kind(value: str) -> str:
    if value in {"pull_request", "ticket"}:
        return value
    return "unknown"


def work_item_transition_kind(value: str) -> str:
    if value in {"state_change", "terminal_state_change", "coverage_state_change", "state_refresh"}:
        return value
    return "state_change"


def transition_confidence_basis(row: pd.Series) -> str:
    value = first_nonempty([row.get("confidence_basis")])
    if value in {"unknown", "adjacent_snapshot_detection", "source_verified", "human_verified"}:
        return value
    return "adjacent_snapshot_detection"


def transition_verification_state(transition_kind: str, terminal: bool) -> str:
    if terminal:
        return "closeout_required"
    if transition_kind in {"state_change", "coverage_state_change", "state_refresh"}:
        return "candidate"
    return "candidate"


def ontology_snapshot_ids_by_subject_time(conn: sqlite3.Connection) -> dict[tuple[str, str, str], int]:
    if not table_exists(conn, "work_item_state_snapshots"):
        return {}
    rows = conn.execute(
        "select id, subject_kind, subject_key, observed_at from work_item_state_snapshots"
    ).fetchall()
    return {(str(subject_kind), str(subject_key), str(observed_at)): int(row_id) for row_id, subject_kind, subject_key, observed_at in rows if subject_kind and subject_key and observed_at}


def attach_work_item_state_snapshot_evidence(
    conn: sqlite3.Connection,
    source_instance: str,
    values: dict[str, Any],
    now: str,
) -> None:
    row = conn.execute("select id from work_item_state_snapshots where key = ?", (values["key"],)).fetchone()
    if row is None:
        return
    snapshot_id = int(row[0])
    observed_at = first_nonempty([values.get("observed_at")])
    subject_kind = first_nonempty([values.get("subject_kind")])
    subject_key = first_nonempty([values.get("subject_key")])
    locator = first_nonempty([values.get("external_id"), f"{subject_kind}:{subject_key}:{observed_at or now}"])
    state = first_nonempty([values.get("state")]) or "unknown"
    excerpt = f"{subject_kind} {subject_key} observed state {state} at {observed_at or now}."
    coverage = first_nonempty([values.get("source_current_coverage_state")])
    detail = first_nonempty([values.get("source_current_detail_state")])
    if coverage or detail:
        excerpt = f"{excerpt} coverage={coverage or 'unknown'} detail={detail or 'unknown'}."
    issue_codes = first_nonempty([values.get("source_current_issue_codes")])
    if issue_codes:
        excerpt = f"{excerpt} issues={issue_codes}."
    evidence_id = upsert_generated_evidence(
        conn,
        source_instance,
        "work_item_state_snapshot",
        snapshot_id,
        "state",
        "state_snapshot",
        locator,
        excerpt,
        now,
    )
    if evidence_id is not None:
        conn.execute(
            "update work_item_state_snapshots set latest_evidence_id = ?, evidence_count = 1 where id = ?",
            (evidence_id, snapshot_id),
        )


def attach_work_item_state_transition_evidence(
    conn: sqlite3.Connection,
    source_instance: str,
    values: dict[str, Any],
    now: str,
) -> None:
    row = conn.execute("select id from work_item_state_transitions where key = ?", (values["key"],)).fetchone()
    if row is None:
        return
    transition_id = int(row[0])
    subject_kind = first_nonempty([values.get("subject_kind")])
    subject_key = first_nonempty([values.get("subject_key")])
    from_state = first_nonempty([values.get("from_state")]) or "unknown"
    to_state = first_nonempty([values.get("to_state")]) or "unknown"
    from_observed_at = first_nonempty([values.get("from_observed_at")])
    to_observed_at = first_nonempty([values.get("to_observed_at")])
    locator = first_nonempty([values.get("external_id"), values.get("key")])
    excerpt = f"{subject_kind} {subject_key}: {from_state} -> {to_state} between {from_observed_at or 'unknown'} and {to_observed_at or 'unknown'}."
    note = first_nonempty([values.get("note")])
    if note:
        excerpt = f"{excerpt} {note}"
    evidence_id = upsert_generated_evidence(
        conn,
        source_instance,
        "work_item_state_transition",
        transition_id,
        "transition_kind",
        "state_transition",
        locator,
        excerpt,
        now,
    )
    if evidence_id is not None:
        conn.execute(
            "update work_item_state_transitions set latest_evidence_id = ?, evidence_count = 1 where id = ?",
            (evidence_id, transition_id),
        )


def base_work_item_state_snapshot_values(
    source_instance: str,
    snapshot_key: str,
    external_kind: str,
    subject_kind: str,
    subject_key: str,
    now: str,
) -> dict[str, Any]:
    snapshot_key = snapshot_key or stable_digest([external_kind, subject_kind, subject_key, now])
    return {
        "key": f"work-item-state-snapshot:cubicle-analytics:{source_instance}:{stable_digest([snapshot_key])}",
        "subject_kind": subject_kind,
        "subject_key": subject_key,
        "pull_request_id": None,
        "ticket_id": None,
        "state": "",
        "title": "",
        "observed_at": now,
        "captured_at": now,
        "source_created_at": None,
        "source_updated_at": None,
        "closed_at": None,
        "merged_at": None,
        "age_days": None,
        "stale_days": None,
        "cycle_time_days": None,
        "risk_score": 0.0,
        "risk_band": "unknown",
        "forecast_method": "",
        "source_current_coverage_state": "",
        "source_current_detail_state": "",
        "source_current_issue_codes": "",
        "source_current_issue_kinds": "",
        "lifecycle_fields_source": "",
        "churn_fields_source": "",
        "mergeability_fields_source": "",
        "priority": "",
        "linked_pr_count": 0,
        "fresh_pr_link_count": 0,
        "partial_pr_link_count": 0,
        "comment_count": 0,
        "participant_count": 0,
        "blocker_keyword_count": 0,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": external_kind,
        "external_id": snapshot_key,
        "source_url": "",
        "latest_evidence_id": None,
        "evidence_count": 0,
        "freshness_state": "fresh",
        "visibility": "unknown",
        "confidence": 1.0,
        "event_count": 1,
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": 0.0,
        "created_at": now,
        "updated_at": now,
    }


def forecast_risk_band(value: str) -> str:
    if value in {"low", "medium", "high", "critical"}:
        return value
    return "unknown"


def forecast_summary_values(
    source_instance: str,
    forecast_summary: pd.DataFrame,
    forecast_backtest: pd.DataFrame,
    time_series_summary: pd.DataFrame,
    now: str,
) -> dict[str, Any]:
    eta_ready = forecast_effective_eta_ready(forecast_summary, time_series_summary)
    baseline_count = metric_int(forecast_summary, "merged_pr_count")
    observed_snapshot_time_count = metric_int(time_series_summary, "observed_snapshot_time_count")
    transition_candidate_count = metric_int(time_series_summary, "transition_candidate_count")
    terminal_transition_candidate_count = metric_int(time_series_summary, "terminal_transition_candidate_count")
    transition_history_ready = observed_snapshot_time_count >= 2 and transition_candidate_count > 0
    readiness_state = "ready" if eta_ready else "gated"
    if baseline_count < 10:
        readiness_state = "insufficient_sample"
    best_model = metric_text(forecast_summary, "backtest_best_model")
    forecast_method = metric_text(forecast_summary, "forecast_method")
    reason = forecast_readiness_reason(
        forecast_summary,
        baseline_count,
        eta_ready,
        best_model,
        observed_snapshot_time_count,
        transition_candidate_count,
    )
    external_id = f"summary:{now}"
    values = base_forecast_evaluation_values(
        source_instance,
        f"work-forecast-evaluation:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
        "summary",
        best_model or forecast_method or "forecast_summary",
        now,
    )
    values.update(
        {
            "forecast_method": forecast_method,
            "best_model_name": best_model,
            "baseline_sample_count": baseline_count,
            "open_candidate_count": metric_int(forecast_summary, "open_pr_count"),
            "closed_unmerged_count": metric_int(forecast_summary, "closed_unmerged_pr_count"),
            "observed_snapshot_time_count": observed_snapshot_time_count,
            "transition_candidate_count": transition_candidate_count,
            "terminal_transition_candidate_count": terminal_transition_candidate_count,
            "transition_history_ready": transition_history_ready,
            "median_cycle_days": metric_float(forecast_summary, "median_merged_cycle_days"),
            "p75_cycle_days": metric_float(forecast_summary, "p75_merged_cycle_days"),
            "avg_closed_unmerged_cycle_days": metric_float(forecast_summary, "avg_closed_unmerged_cycle_days"),
            "mae_days": metric_float(forecast_summary, "cross_validated_mae_days"),
            "ready_for_eta": eta_ready,
            "readiness_state": readiness_state,
            "readiness_reason": reason,
            "note": "Typed forecast readiness summary from tpm_forecast_summary and tpm_forecast_backtest.",
            "external_id": external_id,
            "event_count": len(forecast_backtest) if not forecast_backtest.empty else 1,
            "rank_score": 100.0 if eta_ready else 72.0,
        }
    )
    return values


def base_forecast_evaluation_values(
    source_instance: str,
    key: str,
    evaluation_kind: str,
    model_name: str,
    now: str,
) -> dict[str, Any]:
    return {
        "key": key,
        "evaluation_kind": evaluation_kind,
        "model_name": model_name,
        "forecast_method": "",
        "best_model_name": "",
        "fold": 0,
        "train_count": 0,
        "test_count": 0,
        "baseline_sample_count": 0,
        "open_candidate_count": 0,
        "closed_unmerged_count": 0,
        "median_cycle_days": None,
        "p75_cycle_days": None,
        "avg_closed_unmerged_cycle_days": None,
        "mae_days": None,
        "median_error_days": None,
        "p75_error_days": None,
        "max_error_days": None,
        "improvement_vs_median_pct": None,
        "ready_for_eta": False,
        "readiness_state": "unknown",
        "readiness_reason": "",
        "note": "",
        "evaluated_at": now,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_forecast_evaluation",
        "external_id": key,
        "source_url": "",
        "latest_evidence_id": None,
        "evidence_count": 0,
        "freshness_state": "fresh",
        "visibility": "unknown",
        "confidence": 1.0,
        "event_count": 0,
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": 0.0,
        "created_at": now,
        "updated_at": now,
    }


def forecast_evaluation_kind(value: str) -> str:
    if value in {
        "summary",
        "kfold",
        "chronological_holdout",
        "source_event_as_of_kfold",
        "source_event_as_of_chronological_holdout",
        "lifecycle_as_of_baseline",
        "survival_time_to_merge",
        "insufficient_sample",
    }:
        return value
    return "kfold" if value else "summary"


def forecast_row_effective_eta_ready(
    row: pd.Series,
    forecast_summary: pd.DataFrame,
    time_series_summary: pd.DataFrame | None = None,
) -> bool:
    row_ready = first_nonempty([row.get("ready_for_eta")]).lower() == "true"
    return row_ready and forecast_effective_eta_ready(forecast_summary, time_series_summary)


def forecast_effective_eta_ready(forecast_summary: pd.DataFrame, time_series_summary: pd.DataFrame | None = None) -> bool:
    if metric_text(forecast_summary, "eta_forecast_ready").lower() != "true":
        return False
    if metric_text(forecast_summary, "eta_readiness_state").lower() != "ready":
        return False
    if metric_text(forecast_summary, "eta_temporal_snapshot_state") not in ETA_TEMPORAL_READY_STATES:
        return False
    if time_series_summary is not None and not time_series_summary.empty:
        return metric_int(time_series_summary, "observed_snapshot_time_count") >= 2 and metric_int(time_series_summary, "transition_candidate_count") > 0
    return True


def forecast_row_readiness_state(
    evaluation: str,
    row: pd.Series,
    forecast_summary: pd.DataFrame | None = None,
    time_series_summary: pd.DataFrame | None = None,
) -> str:
    if evaluation == "insufficient_sample":
        return "insufficient_sample"
    if forecast_summary is not None:
        return "ready" if forecast_row_effective_eta_ready(row, forecast_summary, time_series_summary) else "gated"
    return "ready" if first_nonempty([row.get("ready_for_eta")]).lower() == "true" else "gated"


def forecast_row_readiness_reason(
    evaluation: str,
    row: pd.Series,
    forecast_summary: pd.DataFrame | None = None,
    time_series_summary: pd.DataFrame | None = None,
) -> str:
    if evaluation == "insufficient_sample":
        return first_nonempty([row.get("note")]) or "Need more merged PR history before ETA evaluation."
    if evaluation == "lifecycle_as_of_baseline":
        return first_nonempty([row.get("note")]) or "Lifecycle-derived as-of baselines are benchmark evidence only; ETA remains gated until live pre-terminal feature snapshots exist."
    if evaluation.startswith("source_event_as_of"):
        return first_nonempty([row.get("note")]) or "Source-event replay as-of backtest is validation evidence only; ETA remains gated until a candidate clears grouped and chronological source-event gates."
    if evaluation == "survival_time_to_merge":
        return first_nonempty([row.get("note")]) or "Censored time-to-merge survival baselines support risk calibration only; ETA remains gated until calibrated live as-of feature snapshots exist."
    model_name = first_nonempty([row.get("model")]) or "unknown"
    mae = metric_row_float(row, "mae_days")
    improvement = metric_row_float(row, "improvement_vs_median_pct")
    row_ready = first_nonempty([row.get("ready_for_eta")]).lower() == "true"
    if row_ready and forecast_summary is not None and not forecast_row_effective_eta_ready(row, forecast_summary, time_series_summary):
        return f"{model_name} met the row-level backtest threshold, but ETA remains gated until as-of feature snapshots and the summary readiness gate pass."
    if row_ready:
        return f"{model_name} beat the median baseline by {improvement:.2f}% on this evaluation row."
    if mae is not None:
        return f"{model_name} remains gated on {evaluation}: MAE {mae:.2f}d did not clear the ETA quality gate."
    return first_nonempty([row.get("note")])


def forecast_row_rank_score(evaluation: str, row: pd.Series) -> float:
    if first_nonempty([row.get("ready_for_eta")]).lower() == "true":
        return 90.0
    if evaluation == "insufficient_sample":
        return 40.0
    return 60.0


def forecast_risk_backtest_note(row: pd.Series) -> str:
    metric = first_nonempty([row.get("metric")]) or "risk_triage_metric"
    value = first_nonempty([row.get("value")])
    interpretation = first_nonempty([row.get("interpretation")])
    guardrail = first_nonempty([row.get("guardrail")])
    parts = [f"{metric}={value}" if value else metric]
    if interpretation:
        parts.append(interpretation)
    if guardrail:
        parts.append(guardrail)
    return " ".join(parts)


def forecast_risk_backtest_rank_score(metric: str) -> float:
    if metric in {"precision_at_10pct", "lift_vs_baseline_at_10pct"}:
        return 82.0
    if metric.startswith("coverage_stratified") or metric.startswith("coverage_stratum"):
        return 76.0
    return 70.0


def decision_target_backtest_rank_score(row: pd.Series) -> float:
    lift = metric_row_float(row, "lift_at_10pct") or 0.0
    precision = metric_row_float(row, "precision_at_10pct") or 0.0
    return min(84.0, 68.0 + max(lift, 0.0) * 20.0 + max(precision, 0.0) * 8.0)


def forecast_readiness_reason(
    forecast_summary: pd.DataFrame,
    baseline_count: int,
    eta_ready: bool,
    best_model: str,
    observed_snapshot_time_count: int | None = None,
    transition_candidate_count: int | None = None,
) -> str:
    if baseline_count < 10:
        return f"Only {baseline_count} merged PRs are available; at least 10 are needed to backtest cycle forecasting."
    median = metric_text(forecast_summary, "backtest_median_mae_days")
    heuristic = metric_text(forecast_summary, "backtest_heuristic_mae_days")
    gradient_boosting = metric_text(forecast_summary, "backtest_gradient_boosting_mae_days")
    hist_gradient_boosting = metric_text(forecast_summary, "backtest_hist_gradient_boosting_mae_days")
    forest = metric_text(forecast_summary, "backtest_random_forest_mae_days")
    best_candidate = metric_text(forecast_summary, "eta_best_candidate_model")
    primary_blocker = metric_text(forecast_summary, "eta_primary_blocker")
    kfold_lift = metric_text(forecast_summary, "eta_kfold_best_candidate_improvement_pct") or metric_text(forecast_summary, "eta_kfold_random_forest_improvement_pct")
    chrono_lift = metric_text(forecast_summary, "eta_chronological_best_candidate_improvement_pct") or metric_text(forecast_summary, "eta_chronological_random_forest_improvement_pct")
    snapshot_state = metric_text(forecast_summary, "eta_temporal_snapshot_state")
    next_evidence = metric_text(forecast_summary, "eta_next_evidence_needed")
    transition_gate = ""
    if observed_snapshot_time_count is None or transition_candidate_count is None:
        transition_gate = ""
    elif observed_snapshot_time_count < 2:
        transition_gate = f" Transition forecasting is gated because only {observed_snapshot_time_count} distinct observed snapshot time(s) exist."
    elif transition_candidate_count == 0:
        transition_gate = " Transition forecasting is gated because no adjacent state/coverage transitions have been observed yet."
    if eta_ready:
        return f"ETA forecast is ready: {best_model or 'selected model'} cleared the typed backtest gate.{transition_gate}"
    diagnostics = []
    if primary_blocker:
        diagnostics.append(f"primary blocker {primary_blocker}")
    if kfold_lift or chrono_lift:
        diagnostics.append(f"best candidate {best_candidate or 'unknown'} lift vs median baseline: K-fold {kfold_lift or 'unknown'}%, chronological {chrono_lift or 'unknown'}%")
    if snapshot_state:
        diagnostics.append(f"snapshot state {snapshot_state}")
    if next_evidence:
        diagnostics.append(f"next evidence {next_evidence}")
    diagnostic_text = " " + "; ".join(diagnostics) + "." if diagnostics else ""
    return f"ETA forecast is gated: best K-fold model is {best_model or 'unknown'}; median baseline MAE {median}d, heuristic MAE {heuristic}d, gradient boosting MAE {gradient_boosting}d, histogram gradient boosting MAE {hist_gradient_boosting}d, random forest MAE {forest}d.{diagnostic_text}{transition_gate}"


def persist_workstream_register_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    program_register: pd.DataFrame,
    ticket_features: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["workstreams", "workstream_tickets", "tickets"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before workstream materialization")
    if program_register.empty and ticket_features.empty:
        return

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    ticket_ids = ontology_ticket_ids_by_subject(conn)
    workstream_keys = workstream_keys_for_register(program_register)
    if not workstream_keys:
        workstream_keys = ["flink-kubernetes-operator"]

    for workstream_key in workstream_keys:
        stream_rows = program_register[program_register["workstream_key"] == workstream_key] if not program_register.empty and "workstream_key" in program_register.columns else pd.DataFrame()
        ticket_keys = ticket_keys_for_workstream(workstream_key, stream_rows, ticket_features)
        linked_ticket_ids = [ticket_ids[key] for key in sorted(ticket_keys) if key in ticket_ids]
        max_risk = max_risk_score(stream_rows)
        workstream_values = {
            "key": f"workstream:{workstream_key}",
            "title": workstream_title(workstream_key),
            "status": "active" if len(linked_ticket_ids) > 0 or len(stream_rows) > 0 else "unknown",
            "summary": workstream_summary(workstream_key, len(linked_ticket_ids), stream_rows),
            "search_text": workstream_search_text(workstream_key, stream_rows),
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_workstream",
            "external_id": workstream_key,
            "source_url": workstream_source_url(workstream_key),
            "source_updated_at": now,
            "content_hash": stable_digest([workstream_key, len(linked_ticket_ids), len(stream_rows), max_risk]),
            "deletion_state": "present",
            "acl_state": "unavailable",
            "last_confirmed_at": now,
            "last_changed_at": now,
            "evidence_count": max(1, len(stream_rows)),
            "freshness_state": "partial",
            "visibility": "public",
            "confidence": 0.9,
            "event_count": len(stream_rows),
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": max_risk,
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "workstreams", workstream_values, "key")
        workstream_id = int(conn.execute("select id from workstreams where key = ?", (workstream_values["key"],)).fetchone()[0])
        for ticket_key in sorted(ticket_keys):
            ticket_id = ticket_ids.get(ticket_key)
            if ticket_id is None:
                continue
            upsert_workstream_ticket(conn, workstream_id, ticket_id, source_instance, workstream_key, ticket_key, now, max_risk)
    conn.commit()


def persist_work_program_items_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    program_register: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_program_items", "workstreams", "work_actions", "pull_requests", "tickets", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before program-register materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    pr_ids = ontology_pr_ids_by_subject(conn)
    ticket_ids = ontology_ticket_ids_by_subject(conn)
    action_ids = ontology_work_action_ids_by_key(conn, source_instance)
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    current_item_keys = current_work_program_item_keys(program_register, source_instance, action_ids, pr_ids, ticket_ids)
    delete_stale_work_program_items(conn, source_instance, current_item_keys)
    if program_register.empty:
        conn.commit()
        return

    for _, row in program_register.iterrows():
        program_key = first_nonempty([row.get("program_key")])
        action_key = first_nonempty([row.get("action_key")])
        if not program_key or not action_key:
            continue
        action_id = action_ids.get(action_key)
        if action_id is None:
            continue

        subject_kind = work_program_subject_kind(first_nonempty([row.get("subject_kind")]))
        subject_key = first_nonempty([row.get("subject_key")])
        if not subject_key:
            continue
        pull_request_id = None
        ticket_id = None
        if subject_kind == "pull_request":
            pull_request_id = pr_ids.get(subject_key)
            if pull_request_id is None:
                continue
        elif subject_kind == "ticket":
            ticket_id = ticket_ids.get(subject_key.upper())
            if ticket_id is None:
                continue

        workstream_key = first_nonempty([row.get("workstream_key")]) or "flink-kubernetes-operator"
        source_coverage_state = first_nonempty([row.get("source_coverage_state")])
        freshness_state = work_program_item_freshness(source_coverage_state, subject_kind)
        confidence = 0.75 if freshness_state == "partial" else 0.9
        register_updated_at = first_nonempty([row.get("updated_at")]) or now
        evidence_ref = first_nonempty([row.get("evidence_ref")])
        _, _, source_url = parse_evidence_ref(evidence_ref, workstream_source_url(workstream_key))
        values = {
            "key": f"work-program-item:cubicle-analytics:{source_instance}:{stable_digest([program_key])}",
            "workstream_id": workstream_ids.get(f"workstream:{workstream_key}"),
            "work_action_id": action_id,
            "pull_request_id": pull_request_id,
            "ticket_id": ticket_id,
            "workstream_key": workstream_key,
            "subject_kind": subject_kind,
            "subject_key": subject_key,
            "linked_ticket_keys": first_nonempty([row.get("linked_ticket_keys")]),
            "linked_pull_request_keys": first_nonempty([row.get("linked_pull_request_keys"), row.get("linked_pr_keys")]),
            "title": first_nonempty([row.get("title")]) or subject_key,
            "program_status": work_program_status_value(first_nonempty([row.get("program_status")])),
            "tpm_bucket": work_program_bucket_value(first_nonempty([row.get("tpm_bucket")])),
            "owner_key": first_nonempty([row.get("owner_key")]),
            "owner_source": first_nonempty([row.get("owner_source")]),
            "author_dri": first_nonempty([row.get("author_dri")]),
            "requested_reviewer_keys": first_nonempty([row.get("requested_reviewer_keys")]),
            "reviewer_or_approver": first_nonempty([row.get("reviewer_or_approver")]),
            "next_action": first_nonempty([row.get("next_action")]),
            "decision_needed": first_nonempty([row.get("decision_needed")]),
            "decision_state": work_action_decision_state_value(first_nonempty([row.get("decision_state")])),
            "decision_gate_reason": first_nonempty([row.get("decision_gate_reason")]),
            "due_bucket": work_action_due_bucket_value(first_nonempty([row.get("due_bucket")])),
            "last_source_update_at": first_nonempty([row.get("last_source_update_at")]),
            "age_days": optional_float(first_nonempty([row.get("age_days")])),
            "stale_days": optional_float(first_nonempty([row.get("stale_days")])),
            "risk_score": safe_float(row.get("risk_score")),
            "blocker_label_state": first_nonempty([row.get("blocker_label_state")]),
            "ci_signal": first_nonempty([row.get("ci_signal")]),
            "transition_state": first_nonempty([row.get("transition_state")]),
            "dependency_summary": first_nonempty([row.get("dependency_summary")]),
            "source_coverage_state": source_coverage_state,
            "label_quality": first_nonempty([row.get("label_quality")]),
            "register_updated_at": register_updated_at,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_program_item",
            "external_id": program_key,
            "source_url": source_url,
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": freshness_state,
            "visibility": "public",
            "confidence": confidence,
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": register_updated_at,
            "rank_score": safe_float(row.get("risk_score")),
            "created_at": now,
            "updated_at": now,
        }
        conn.execute(
            """
            update work_program_items
               set work_action_id = null
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_program_item'
               and work_action_id = ?
               and key != ?
            """,
            (source_instance, action_id, values["key"]),
        )
        upsert_row(conn, "work_program_items", values, "key")
        item_id = int(conn.execute("select id from work_program_items where key = ?", (values["key"],)).fetchone()[0])
        excerpt = work_program_item_evidence_excerpt(row, subject_kind, subject_key)
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_item",
            item_id,
            "program_status",
            "tpm_program_register",
            program_key,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update work_program_items set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, item_id),
            )
    conn.commit()


def persist_work_program_milestones_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    milestone_signals: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_program_milestones", "workstreams", "pull_requests", "tickets", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before milestone materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    run_prefix = f"{workstream_key}|{now}|"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    pr_ids = ontology_pr_ids_by_subject(conn)
    ticket_ids = ontology_ticket_ids_by_subject(conn)
    if milestone_signals.empty:
        conn.execute(
            """
            delete from work_program_milestones
            where source_system = 'cubicle_analytics'
              and source_instance = ?
              and external_kind = 'tpm_work_program_milestone'
              and external_id like ?
            """,
            (source_instance, run_prefix + "%"),
        )
        conn.commit()
        return

    current_keys: list[str] = []
    for _, row in milestone_signals.iterrows():
        subject_kind = work_program_subject_kind(first_nonempty([row.get("subject_kind")]))
        subject_key = first_nonempty([row.get("subject_key")])
        if not subject_key:
            continue
        pull_request_id = None
        ticket_id = None
        if subject_kind == "pull_request":
            pull_request_id = pr_ids.get(subject_key)
            if pull_request_id is None:
                continue
        elif subject_kind == "ticket":
            subject_key = subject_key.upper()
            ticket_id = ticket_ids.get(subject_key)
            if ticket_id is None:
                continue
        else:
            continue

        signal_external_id = first_nonempty([row.get("external_id"), row.get("source_payload_key")])
        if not signal_external_id:
            signal_external_id = "|".join(
                [
                    subject_kind,
                    subject_key,
                    first_nonempty([row.get("milestone_kind")]),
                    first_nonempty([row.get("milestone_name")]),
                ]
            )
        external_id = f"{run_prefix}{signal_external_id}"
        milestone_kind = work_program_milestone_kind_value(first_nonempty([row.get("milestone_kind")]))
        commitment_strength = work_program_milestone_commitment_strength_value(first_nonempty([row.get("commitment_strength")]))
        date_claim_allowed = bool_int(row.get("date_claim_allowed"))
        delivery_commitment_allowed = bool_int(row.get("delivery_commitment_allowed"))
        source_url = first_nonempty([row.get("source_url"), workstream_source_url(workstream_key)])
        target_date = first_nonempty([row.get("target_date")])
        outcome_date = first_nonempty([row.get("outcome_date")])
        captured_at = first_nonempty([row.get("captured_at")]) or now
        last_activity_at = first_nonempty([outcome_date, target_date, captured_at])
        confidence = work_program_milestone_confidence(commitment_strength, date_claim_allowed, delivery_commitment_allowed)
        values = {
            "key": f"work-program-milestone:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_ids.get(f"workstream:{workstream_key}"),
            "pull_request_id": pull_request_id,
            "ticket_id": ticket_id,
            "workstream_key": workstream_key,
            "subject_kind": subject_kind,
            "subject_key": subject_key,
            "milestone_kind": milestone_kind,
            "milestone_name": first_nonempty([row.get("milestone_name")]) or subject_key,
            "target_date": target_date,
            "outcome_date": outcome_date,
            "milestone_state": work_program_milestone_state_value(first_nonempty([row.get("milestone_state")])),
            "commitment_strength": commitment_strength,
            "date_claim_allowed": date_claim_allowed,
            "delivery_commitment_allowed": delivery_commitment_allowed,
            "claim_gate_reason": first_nonempty([row.get("claim_gate_reason")]) or "source_date_signal_requires_review",
            "source_field": first_nonempty([row.get("source_field")]) or "unknown",
            "source_payload_key": first_nonempty([row.get("source_payload_key")]),
            "captured_at": captured_at,
            "generated_at": now,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_program_milestone",
            "external_id": external_id,
            "source_url": source_url,
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": confidence,
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": last_activity_at,
            "rank_score": safe_float(row.get("rank_score")),
            "created_at": now,
            "updated_at": now,
        }
        current_keys.append(values["key"])
        upsert_row(conn, "work_program_milestones", values, "key")
        milestone_id = int(conn.execute("select id from work_program_milestones where key = ?", (values["key"],)).fetchone()[0])
        excerpt = work_program_milestone_evidence_excerpt(values)
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_milestone",
            milestone_id,
            "milestone_state",
            "tpm_work_program_milestone",
            external_id,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set source_url = ?, freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (source_url, confidence, evidence_id),
            )
            conn.execute(
                "update work_program_milestones set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, milestone_id),
            )
    if current_keys:
        placeholders = ", ".join(["?"] * len(current_keys))
        conn.execute(
            f"""
            delete from work_program_milestones
            where source_system = 'cubicle_analytics'
              and source_instance = ?
              and external_kind = 'tpm_work_program_milestone'
              and external_id like ?
              and key not in ({placeholders})
            """,
            (source_instance, run_prefix + "%", *current_keys),
        )
    else:
        conn.execute(
            """
            delete from work_program_milestones
            where source_system = 'cubicle_analytics'
              and source_instance = ?
              and external_kind = 'tpm_work_program_milestone'
              and external_id like ?
            """,
            (source_instance, run_prefix + "%"),
        )
    conn.commit()


def work_program_milestone_confidence(commitment_strength: str, date_claim_allowed: int, delivery_commitment_allowed: int) -> float:
    if delivery_commitment_allowed and commitment_strength == "explicit_commitment":
        return 0.98
    if date_claim_allowed and commitment_strength == "release_signal":
        return 0.9
    if commitment_strength == "outcome_evidence":
        return 0.95
    return 0.75


def work_program_milestone_evidence_excerpt(values: dict[str, Any]) -> str:
    target = first_nonempty([values.get("target_date")])
    outcome = first_nonempty([values.get("outcome_date")])
    pieces = [
        f"{values['subject_kind']} {values['subject_key']}",
        f"{values['milestone_kind']} {values['milestone_name']}",
    ]
    if target:
        pieces.append(f"target {target}")
    if outcome:
        pieces.append(f"outcome {outcome}")
    pieces.append(f"state {values['milestone_state']}")
    pieces.append(f"commitment {values['commitment_strength']}")
    pieces.append(str(values["claim_gate_reason"]))
    return "; ".join(piece for piece in pieces if first_nonempty([piece]))


def current_work_program_item_keys(
    program_register: pd.DataFrame,
    source_instance: str,
    action_ids: dict[str, int],
    pr_ids: dict[str, int],
    ticket_ids: dict[str, int],
) -> set[str]:
    keys: set[str] = set()
    keys_by_action_id: dict[int, set[str]] = {}
    if program_register.empty:
        return keys
    for _, row in program_register.iterrows():
        program_key = first_nonempty([row.get("program_key")])
        action_key = first_nonempty([row.get("action_key")])
        action_id = action_ids.get(action_key)
        if not program_key or action_id is None:
            continue
        subject_kind = work_program_subject_kind(first_nonempty([row.get("subject_kind")]))
        subject_key = first_nonempty([row.get("subject_key")])
        if not subject_key:
            continue
        if subject_kind == "pull_request" and pr_ids.get(subject_key) is None:
            continue
        if subject_kind == "ticket" and ticket_ids.get(subject_key.upper()) is None:
            continue
        key = f"work-program-item:cubicle-analytics:{source_instance}:{stable_digest([program_key])}"
        keys.add(key)
        keys_by_action_id.setdefault(action_id, set()).add(key)
    duplicate_links = {action_id: sorted(action_keys) for action_id, action_keys in keys_by_action_id.items() if len(action_keys) > 1}
    if duplicate_links:
        details = "; ".join(f"work_action_id {action_id}: {', '.join(action_keys)}" for action_id, action_keys in sorted(duplicate_links.items()))
        raise RuntimeError(f"program register maps one WorkAction to multiple current WorkProgramItems: {details}")
    return keys


def delete_stale_work_program_items(conn: sqlite3.Connection, source_instance: str, current_item_keys: set[str]) -> None:
    if current_item_keys:
        placeholders = ", ".join(["?"] * len(current_item_keys))
        conn.execute(
            f"""
            delete from work_program_items
             where source_system = 'cubicle_analytics'
               and source_instance = ?
               and external_kind = 'tpm_program_item'
               and key not in ({placeholders})
            """,
            [source_instance, *sorted(current_item_keys)],
        )
        return
    conn.execute(
        """
        delete from work_program_items
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_program_item'
        """,
        (source_instance,),
    )


def persist_workstream_health_snapshots_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    standup_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["workstream_health_snapshots", "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before workstream health materialization")
    if standup_summary.empty:
        return

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    for _, row in standup_summary.iterrows():
        workstream_key = first_nonempty([row.get("workstream_key")]) or "flink-kubernetes-operator"
        workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
        external_id = f"{workstream_key}:{now}"
        recommended_focus = first_nonempty([row.get("recommended_cadence_focus")])
        operating_status = workstream_operating_status(first_nonempty([row.get("operating_status")]))
        source_repair_count = metric_row_int(row, "source_repair_count")
        coverage_limited_count = metric_row_int(row, "coverage_limited_count")
        anonymous_observation_count = metric_row_int(row, "anonymous_observation_count")
        freshness_state = "fresh"
        confidence = 1.0
        if operating_status == "unknown":
            freshness_state = "unknown"
            confidence = 0.0
        elif source_repair_count > 0 or coverage_limited_count > 0 or anonymous_observation_count > 0:
            freshness_state = "partial"
            confidence = 0.85
        values = {
            "key": f"workstream-health-snapshot:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "workstream_key": workstream_key,
            "generated_at": first_nonempty([row.get("generated_at")]) or now,
            "operating_status": operating_status,
            "action_item_count": metric_row_int(row, "action_item_count"),
            "product_action_count": metric_row_int(row, "open_work_count"),
            "validation_lead_count": metric_row_int(row, "validation_lead_count"),
            "critical_or_high_validation_lead_count": metric_row_int(row, "critical_or_high_validation_lead_count"),
            "model_or_rule_qa_count": metric_row_int(row, "model_or_rule_qa_count"),
            "closeout_review_count": metric_row_int(row, "closeout_review_count"),
            "owner_count": metric_row_int(row, "owner_count"),
            "top_owner_action_count": metric_row_int(row, "top_owner_action_count"),
            "failing_check_pr_count": metric_row_int(row, "failing_check_pr_count"),
            "open_failing_check_pr_count": metric_row_int(row, "open_failing_check_pr_count"),
            "source_repair_count": source_repair_count,
            "coverage_limited_count": coverage_limited_count,
            "anonymous_observation_count": anonymous_observation_count,
            "terminal_transition_count": metric_row_int(row, "terminal_transition_count"),
            "terminal_transition_subjects": first_nonempty([row.get("terminal_transition_subjects")]),
            "eta_forecast_ready": first_nonempty([row.get("eta_forecast_ready")]).lower() == "true",
            "truth_label_coverage": first_nonempty([row.get("truth_label_coverage")]),
            "actionability_label_coverage": first_nonempty([row.get("actionability_label_coverage")]),
            "recommended_cadence_focus": recommended_focus,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_workstream_health_snapshot",
            "external_id": external_id,
            "source_url": workstream_source_url(workstream_key),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": freshness_state,
            "visibility": "public",
            "confidence": confidence,
            "event_count": metric_row_int(row, "action_item_count"),
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": float(metric_row_int(row, "critical_or_high_count") * 10 + metric_row_int(row, "validation_lead_count")),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "workstream_health_snapshots", values, "key")
        snapshot_id = int(conn.execute("select id from workstream_health_snapshots where key = ?", (values["key"],)).fetchone()[0])
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "workstream_health_snapshot",
            snapshot_id,
            "operating_status",
            "workstream_standup",
            external_id,
            recommended_focus or f"{workstream_key} operating status {values['operating_status']}",
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update workstream_health_snapshots set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, snapshot_id),
            )
    conn.commit()


def persist_workstream_standup_sections_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    standup_sections: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["workstream_standup_sections", "workstream_health_snapshots", "workstreams", "work_actions", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before standup section materialization")
    if standup_sections.empty:
        return

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    health_ids = ontology_workstream_health_snapshot_ids_by_external_id(conn, source_instance)
    action_details = ontology_work_action_details_by_key(conn, source_instance)

    for _, row in standup_sections.iterrows():
        section_rank = metric_row_int(row, "section_rank")
        if section_rank <= 0:
            continue
        workstream_key = first_nonempty([row.get("workstream_key")]) or "flink-kubernetes-operator"
        row_generated_at = first_nonempty([row.get("generated_at")]) or now
        health_external_id = f"{workstream_key}:{row_generated_at}"
        health_snapshot_id = health_ids.get(health_external_id)
        workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
        action_key = first_nonempty([row.get("action_key")])
        action = action_details.get(action_key, {})
        evidence_ref = first_nonempty([row.get("evidence_ref")])
        section_kind = workstream_standup_section_kind(first_nonempty([row.get("section_kind")]))
        urgency = workstream_standup_urgency(first_nonempty([row.get("urgency")]))
        if health_snapshot_id is None:
            raise RuntimeError(f"standup section {section_rank} cannot link health snapshot {health_external_id}")
        if workstream_id is None:
            raise RuntimeError(f"standup section {section_rank} cannot link workstream {workstream_key}")
        action_required = section_kind not in {"owner_load", "resolved_change"}
        if action_required and action.get("id") is None:
            raise RuntimeError(f"standup section {section_rank} ({section_kind}) cannot link action {action_key or '<missing>'}")
        subject_key = first_nonempty([row.get("subject_key")])
        subject_kind = standup_subject_kind(subject_key)
        external_id = f"{workstream_key}:{row_generated_at}:{section_rank}"
        summary = first_nonempty([row.get("summary")]) or f"Standup item {section_rank}"
        recommended_action = first_nonempty([row.get("recommended_action")])
        locator_kind, locator, source_url = parse_evidence_ref(evidence_ref, first_nonempty([action.get("source_url")]) or workstream_source_url(workstream_key))
        if not locator_kind:
            locator_kind = "workstream_standup_section"
            locator = external_id
        freshness_state = first_nonempty([action.get("freshness_state")]) or "partial"
        confidence = safe_float(action.get("confidence")) if action.get("confidence") is not None else 0.85

        values = {
            "key": f"workstream-standup-section:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_health_snapshot_id": health_snapshot_id,
            "workstream_id": workstream_id,
            "work_action_id": action.get("id"),
            "workstream_key": workstream_key,
            "generated_at": row_generated_at,
            "section_rank": section_rank,
            "section_kind": section_kind,
            "urgency": urgency,
            "owner_key": first_nonempty([row.get("owner_hint")]),
            "subject_kind": subject_kind,
            "subject_key": subject_key,
            "action_type": first_nonempty([row.get("action_type")]),
            "status_signal": first_nonempty([row.get("status_signal")]),
            "summary": summary,
            "recommended_action": recommended_action,
            "evidence_ref": evidence_ref,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_workstream_standup_section",
            "external_id": external_id,
            "source_url": source_url,
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": freshness_state,
            "visibility": "public",
            "confidence": confidence,
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": max(0.0, 1000.0 - float(section_rank)),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "workstream_standup_sections", values, "key")
        section_id = int(conn.execute("select id from workstream_standup_sections where key = ?", (values["key"],)).fetchone()[0])
        excerpt = f"{summary}. {recommended_action}".strip()
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "workstream_standup_section",
            section_id,
            "recommended_action",
            locator_kind,
            locator,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = ?, confidence = ?, visibility = 'public' where id = ?",
                (freshness_state, confidence, evidence_id),
            )
            conn.execute(
                "update workstream_standup_sections set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, section_id),
            )
    conn.commit()


def persist_work_owner_load_snapshots_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    owner_rollup: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_owner_load_snapshots", "workstreams", "persons", "person_identities", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before owner load materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    person_ids = ontology_person_ids_by_owner_key(conn)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"owner load snapshot cannot link workstream {workstream_key}")

    wrote_snapshot = False
    for _, row in owner_rollup.iterrows():
        owner_key = first_nonempty([row.get("owner_hint")]) or "(unassigned)"
        action_count = metric_row_int(row, "action_count")
        if action_count <= 0:
            continue
        wrote_snapshot = True
        load_status = owner_load_status(row)
        source_limited = metric_row_int(row, "coverage_limited_count") > 0 or metric_row_int(row, "anonymous_observation_count") > 0
        freshness_state = "partial" if source_limited or owner_key == "(unassigned)" else "fresh"
        confidence = 0.85 if source_limited or owner_key == "(unassigned)" else 1.0
        external_id = f"{workstream_key}:{now}:{owner_key}"
        recommended_focus = first_nonempty([row.get("recommended_focus")])
        values = {
            "key": f"work-owner-load:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "person_id": person_ids.get(owner_key),
            "workstream_key": workstream_key,
            "owner_key": owner_key,
            "owner_display_name": owner_display_name_for_key(owner_key),
            "generated_at": now,
            "load_status": load_status,
            "action_count": action_count,
            "product_action_count": metric_row_int(row, "product_action_count"),
            "validation_lead_count": metric_row_int(row, "validation_lead_count"),
            "model_or_rule_qa_count": metric_row_int(row, "model_or_rule_qa_count"),
            "critical_or_high_count": metric_row_int(row, "critical_or_high_count"),
            "max_priority_score": safe_float(row.get("max_priority_score")),
            "avg_priority_score": safe_float(row.get("avg_priority_score")),
            "decision_followup_count": metric_row_int(row, "decision_followup_count"),
            "validate_signal_count": metric_row_int(row, "validate_signal_count"),
            "ci_check_followup_count": metric_row_int(row, "ci_check_followup_count"),
            "review_wait_followup_count": metric_row_int(row, "review_wait_followup_count"),
            "coverage_limited_count": metric_row_int(row, "coverage_limited_count"),
            "anonymous_observation_count": metric_row_int(row, "anonymous_observation_count"),
            "needs_human_review_count": metric_row_int(row, "needs_human_review_count"),
            "top_action_type": first_nonempty([row.get("top_action_type")]),
            "top_subjects": first_nonempty([row.get("top_subjects")]),
            "recommended_focus": recommended_focus,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_owner_load_snapshot",
            "external_id": external_id,
            "source_url": workstream_source_url(workstream_key),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": freshness_state,
            "visibility": "public",
            "confidence": confidence,
            "event_count": action_count,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": safe_float(row.get("max_priority_score")),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "work_owner_load_snapshots", values, "key")
        snapshot_id = int(conn.execute("select id from work_owner_load_snapshots where key = ?", (values["key"],)).fetchone()[0])
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_owner_load_snapshot",
            snapshot_id,
            "recommended_focus",
            "tpm_owner_action_rollup",
            external_id,
            recommended_focus or f"{owner_key} has {action_count} generated action(s)",
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = ?, confidence = ?, visibility = 'public' where id = ?",
                (freshness_state, confidence, evidence_id),
            )
            conn.execute(
                "update work_owner_load_snapshots set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, snapshot_id),
            )
    if not wrote_snapshot:
        owner_key = "(clear)"
        external_id = f"{workstream_key}:{now}:{owner_key}"
        recommended_focus = "No generated owner load actions for this workstream run."
        values = {
            "key": f"work-owner-load:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "person_id": None,
            "workstream_key": workstream_key,
            "owner_key": owner_key,
            "owner_display_name": "",
            "generated_at": now,
            "load_status": "clear",
            "action_count": 0,
            "product_action_count": 0,
            "validation_lead_count": 0,
            "model_or_rule_qa_count": 0,
            "critical_or_high_count": 0,
            "max_priority_score": 0.0,
            "avg_priority_score": 0.0,
            "decision_followup_count": 0,
            "validate_signal_count": 0,
            "ci_check_followup_count": 0,
            "review_wait_followup_count": 0,
            "coverage_limited_count": 0,
            "anonymous_observation_count": 0,
            "needs_human_review_count": 0,
            "top_action_type": "",
            "top_subjects": "",
            "recommended_focus": recommended_focus,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_owner_load_snapshot",
            "external_id": external_id,
            "source_url": workstream_source_url(workstream_key),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": 1.0,
            "event_count": 0,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": 0.0,
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "work_owner_load_snapshots", values, "key")
        snapshot_id = int(conn.execute("select id from work_owner_load_snapshots where key = ?", (values["key"],)).fetchone()[0])
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_owner_load_snapshot",
            snapshot_id,
            "recommended_focus",
            "tpm_owner_action_rollup",
            external_id,
            recommended_focus,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = 1.0, visibility = 'public' where id = ?",
                (evidence_id,),
            )
            conn.execute(
                "update work_owner_load_snapshots set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, snapshot_id),
            )
    conn.commit()


def persist_work_program_quality_gates_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    time_series_summary: pd.DataFrame | None,
    generated_at: str,
) -> None:
    table_name = "work_program_quality_gates"
    required = [table_name, "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before quality-gate materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"quality gates cannot link workstream {workstream_key}")

    facts = ontology_work_program_adversarial_facts(conn, source_instance, workstream_key, now)
    gates = build_work_program_quality_gates(readiness, forecast_summary, facts, time_series_summary)
    run_prefix = f"{workstream_key}|{now}|"
    conn.execute(
        f"""
        delete from {table_name}
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_work_program_quality_gate'
          and external_id like ?
        """,
        (source_instance, run_prefix + "%"),
    )
    for rank, gate in enumerate(gates):
        external_id = f"{run_prefix}{gate['key']}"
        gate_state = first_nonempty([gate.get("gate_state")]) or "gated"
        blocking = bool(gate.get("blocking"))
        values = {
            "key": f"work-program-quality-gate:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "workstream_key": workstream_key,
            "generated_at": now,
            "gate_key": first_nonempty([gate.get("key")]) or "unknown",
            "gate_state": gate_state,
            "blocking": blocking,
            "detail": first_nonempty([gate.get("detail")]) or "No quality-gate detail generated.",
            "recommended_action": first_nonempty([gate.get("recommended_action")]),
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_program_quality_gate",
            "external_id": external_id,
            "source_url": workstream_source_url(workstream_key),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": quality_gate_confidence(gate_state, blocking),
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": (100.0 if blocking else 10.0) - float(rank) / 100.0,
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, table_name, values, "key")
        gate_id = int(conn.execute(f"select id from {table_name} where key = ?", (values["key"],)).fetchone()[0])
        excerpt = f"{values['gate_key']} is {values['gate_state']}: {values['detail']} {values['recommended_action'] or ''}".strip()
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_quality_gate",
            gate_id,
            "gate_state",
            "tpm_quality_gate",
            external_id,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (values["confidence"], evidence_id),
            )
            conn.execute(
                f"update {table_name} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, gate_id),
            )
    conn.commit()


def persist_work_program_adversarial_checks_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_program_adversarial_checks", "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before adversarial check materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"adversarial checks cannot link workstream {workstream_key}")

    facts = ontology_work_program_adversarial_facts(conn, source_instance, workstream_key, now)
    checks = build_work_program_adversarial_checks(readiness, forecast_summary, facts)
    for rank, check in enumerate(checks):
        external_id = f"{workstream_key}:{now}:{check['key']}"
        blocking_gate_keys = unique_strings(check.get("blocking_gate_keys", []))
        evidence_refs = unique_strings(check.get("evidence_refs", []))
        check_state = first_nonempty([check.get("check_state")]) or "warning"
        severity = first_nonempty([check.get("severity")]) or "medium"
        values = {
            "key": f"work-program-adversarial-check:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "workstream_key": workstream_key,
            "generated_at": now,
            "check_kind": first_nonempty([check.get("check_kind")]) or "unknown",
            "check_state": check_state,
            "severity": severity,
            "title": first_nonempty([check.get("title")]) or first_nonempty([check.get("key")]) or "Adversarial check",
            "detail": first_nonempty([check.get("detail")]) or "No detail generated.",
            "recommended_action": first_nonempty([check.get("recommended_action")]) or "Review the generated operating brief before product use.",
            "blocking_gate_keys": "\n".join(blocking_gate_keys),
            "evidence_refs": "\n".join(evidence_refs),
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_program_adversarial_check",
            "external_id": external_id,
            "source_url": workstream_source_url(workstream_key),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": adversarial_check_confidence(check_state, evidence_refs),
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": adversarial_check_rank_score(check_state, severity, rank),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, "work_program_adversarial_checks", values, "key")
        check_id = int(conn.execute("select id from work_program_adversarial_checks where key = ?", (values["key"],)).fetchone()[0])
        excerpt = f"{values['title']}: {values['detail']} {values['recommended_action']}".strip()
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_adversarial_check",
            check_id,
            "check_state",
            "tpm_adversarial_check",
            external_id,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (values["confidence"], evidence_id),
            )
            conn.execute(
                "update work_program_adversarial_checks set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, check_id),
            )
    conn.commit()


def persist_work_program_evidence_needs_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    required = ["work_program_evidence_needs", "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before evidence-need materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"evidence needs cannot link workstream {workstream_key}")

    facts = ontology_work_program_adversarial_facts(conn, source_instance, workstream_key, now)
    needs = build_work_program_evidence_needs(readiness, forecast_summary, facts)
    evidence_need_columns = table_columns(conn, "work_program_evidence_needs")
    action_ids = ontology_work_action_ids_by_key(conn, source_instance) if "work_action_id" in evidence_need_columns else {}
    quality_gate_ids = (
        ontology_work_program_quality_gate_ids_by_key(conn, source_instance, workstream_key, now)
        if "quality_gate_id" in evidence_need_columns
        else {}
    )
    run_prefix = f"{workstream_key}|{now}|"
    conn.execute(
        """
        delete from work_program_evidence_needs
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_work_program_evidence_need'
          and external_id like ?
        """,
        (source_instance, run_prefix + "%"),
    )
    for rank, need in enumerate(needs):
        external_id = f"{run_prefix}{need['key']}"
        priority = first_nonempty([need.get("priority")]) or "medium"
        gate_key = first_nonempty([need.get("gate_key")]) or "unknown"
        action_key = first_nonempty([need.get("action_key")])
        values = {
            "key": f"work-program-evidence-need:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "workstream_key": workstream_key,
            "generated_at": now,
            "gate_key": gate_key,
            "evidence_kind": first_nonempty([need.get("evidence_kind")]) or "unknown",
            "priority": priority,
            "target_kind": first_nonempty([need.get("target_kind")]) or "workstream",
            "target_key": first_nonempty([need.get("target_key")]),
            "owner_key": first_nonempty([need.get("owner_key")]),
            "action_key": action_key,
            "action_state": first_nonempty([need.get("action_state")]),
            "metric_key": first_nonempty([need.get("metric_key")]),
            "execution_state": first_nonempty([need.get("execution_state")]) or "unknown",
            "backing_action_count": int_value(need.get("backing_action_count")),
            "current_count": int_value(need.get("current_count")),
            "required_count": int_value(need.get("required_count")),
            "missing_count": int_value(need.get("missing_count")),
            "current_rate": optional_rate(need.get("current_rate")),
            "required_rate": optional_rate(need.get("required_rate")),
            "recommended_action": first_nonempty([need.get("recommended_action")]) or "Review this evidence need before autonomous TPM action.",
            "next_execution_step": first_nonempty([need.get("next_execution_step")]) or first_nonempty([need.get("recommended_action")]) or "Review this evidence need before autonomous TPM action.",
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_program_evidence_need",
            "external_id": external_id,
            "source_url": first_nonempty([need.get("source_url"), workstream_source_url(workstream_key)]),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": 1.0 if first_nonempty([need.get("execution_state")]) not in {"unknown"} else 0.85,
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": evidence_need_rank_score(priority, int_value(need.get("missing_count")), rank),
            "created_at": now,
            "updated_at": now,
        }
        if "work_action_id" in evidence_need_columns:
            values["work_action_id"] = action_ids.get(action_key)
        if "quality_gate_id" in evidence_need_columns:
            values["quality_gate_id"] = quality_gate_ids.get(gate_key)
        upsert_row(conn, "work_program_evidence_needs", values, "key")
        need_id = int(conn.execute("select id from work_program_evidence_needs where key = ?", (values["key"],)).fetchone()[0])
        excerpt = f"{values['gate_key']} needs {values['evidence_kind']}: {values['next_execution_step']}".strip()
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_evidence_need",
            need_id,
            "execution_state",
            "tpm_evidence_need",
            external_id,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (values["confidence"], evidence_id),
            )
            conn.execute(
                "update work_program_evidence_needs set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, need_id),
            )
    conn.commit()


def persist_work_responsibilities_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    generated_at: str,
) -> None:
    required = [
        "work_responsibilities",
        "persons",
        "person_identities",
        "work_actions",
        "work_blockers",
        "work_program_items",
        "work_program_evidence_needs",
        "pull_requests",
        "tickets",
        "evidences",
    ]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before responsibility materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    person_ids = ontology_person_ids_by_owner_key(conn)
    conn.execute(
        """
        delete from work_responsibilities
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_responsibility'
        """,
        (source_instance,),
    )
    rows: list[dict[str, Any]] = []
    rows.extend(work_action_responsibility_rows(conn, source_instance, person_ids, now))
    rows.extend(work_blocker_responsibility_rows(conn, source_instance, person_ids, now))
    rows.extend(work_program_item_responsibility_rows(conn, source_instance, person_ids, now))
    rows.extend(work_evidence_need_responsibility_rows(conn, source_instance, person_ids, now))

    for row in dedupe_work_responsibility_rows(rows):
        upsert_row(conn, "work_responsibilities", row, "key")
        responsibility_id = int(conn.execute("select id from work_responsibilities where key = ?", (row["key"],)).fetchone()[0])
        evidence_id = upsert_responsibility_evidence(conn, source_instance, responsibility_id, row, now)
        if evidence_id is not None:
            conn.execute(
                "update work_responsibilities set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, responsibility_id),
            )
    conn.commit()


def work_action_responsibility_rows(
    conn: sqlite3.Connection,
    source_instance: str,
    person_ids: dict[str, int],
    now: str,
) -> list[dict[str, Any]]:
    rows = query_dicts(
        conn,
        """
        select id, key, owner_key, owner_source, action_state, decision_state, created_from_run_key,
               source_url, latest_evidence_id, evidence_count, freshness_state, visibility, confidence,
               event_count, first_seen_at, last_activity_at, rank_score, opened_at
          from work_actions
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_action'
           and action_state = 'open'
        """,
        (source_instance,),
    )
    out: list[dict[str, Any]] = []
    for row in rows:
        out.append(
            make_work_responsibility_row(
                source_instance=source_instance,
                source_row=row,
                person_ids=person_ids,
                now=now,
                subject_kind="work_action",
                subject_key=first_nonempty([row.get("key")]),
                responsibility_kind="accountable",
                party_key=first_nonempty([row.get("owner_key")]),
                party_source=first_nonempty([row.get("owner_source")]) or "generated.action_owner",
                basis_kind="generated_candidate",
                basis_detail=first_nonempty([row.get("owner_source"), row.get("created_from_run_key")]),
                subject_ids={"work_action_id": row.get("id")},
                responsibility_state="candidate",
                state_reason="generated action owner hint requires validation before product accountability",
            )
        )
    return out


def work_blocker_responsibility_rows(
    conn: sqlite3.Connection,
    source_instance: str,
    person_ids: dict[str, int],
    now: str,
) -> list[dict[str, Any]]:
    rows = query_dicts(
        conn,
        """
        select id, key, owner_key, owner_source, blocker_state, decision_state,
               source_url, latest_evidence_id, evidence_count, freshness_state, visibility, confidence,
               event_count, first_seen_at, last_activity_at, rank_score
          from work_blockers
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_blocker'
           and blocker_state in ('active', 'validating')
        """,
        (source_instance,),
    )
    out: list[dict[str, Any]] = []
    for row in rows:
        out.append(
            make_work_responsibility_row(
                source_instance=source_instance,
                source_row=row,
                person_ids=person_ids,
                now=now,
                subject_kind="work_blocker",
                subject_key=first_nonempty([row.get("key")]),
                responsibility_kind="accountable",
                party_key=first_nonempty([row.get("owner_key")]),
                party_source=first_nonempty([row.get("owner_source")]) or "generated.blocker_owner",
                basis_kind="generated_candidate",
                basis_detail=first_nonempty([row.get("owner_source"), row.get("blocker_state")]),
                subject_ids={"work_blocker_id": row.get("id")},
                responsibility_state="candidate",
                state_reason="generated blocker ownership requires human or source validation",
            )
        )
    return out


def work_program_item_responsibility_rows(
    conn: sqlite3.Connection,
    source_instance: str,
    person_ids: dict[str, int],
    now: str,
) -> list[dict[str, Any]]:
    rows = query_dicts(
        conn,
        """
        select id, key, workstream_key, subject_kind, subject_key, pull_request_id, ticket_id,
               owner_key, owner_source, author_dri, requested_reviewer_keys, reviewer_or_approver,
               source_url, latest_evidence_id, evidence_count, freshness_state, visibility, confidence,
               event_count, first_seen_at, last_activity_at, rank_score, register_updated_at
          from work_program_items
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_program_item'
           and subject_kind in ('pull_request', 'ticket')
        """,
        (source_instance,),
    )
    out: list[dict[str, Any]] = []
    for row in rows:
        subject_kind = first_nonempty([row.get("subject_kind")])
        subject_key = first_nonempty([row.get("subject_key")])
        subject_ids: dict[str, Any] = {"work_program_item_id": row.get("id")}
        if subject_kind == "pull_request":
            if row.get("pull_request_id") is None:
                continue
            subject_ids["pull_request_id"] = row.get("pull_request_id")
        elif subject_kind == "ticket":
            if row.get("ticket_id") is None:
                continue
            subject_ids["ticket_id"] = row.get("ticket_id")
        else:
            continue

        owner_key = first_nonempty([row.get("owner_key")])
        owner_source = first_nonempty([row.get("owner_source")])
        if owner_key or owner_source == "unassigned":
            responsibility_kind, basis_kind, basis_detail = responsibility_basis_from_program_owner(owner_source)
            out.append(
                make_work_responsibility_row(
                    source_instance=source_instance,
                    source_row=row,
                    person_ids=person_ids,
                    now=now,
                    subject_kind=subject_kind,
                    subject_key=subject_key,
                    responsibility_kind=responsibility_kind,
                    party_key=owner_key,
                    party_source=owner_source or "generated.program_owner",
                    basis_kind=basis_kind,
                    basis_detail=basis_detail,
                    subject_ids=subject_ids,
                    responsibility_state=responsibility_state_for_basis(basis_kind, row, owner_key),
                    state_reason=responsibility_state_reason_for_basis(basis_kind, row, owner_key),
                )
            )

        author_dri = github_owner_hint(row.get("author_dri"))
        if author_dri:
            out.append(
                make_work_responsibility_row(
                    source_instance=source_instance,
                    source_row=row,
                    person_ids=person_ids,
                    now=now,
                    subject_kind=subject_kind,
                    subject_key=subject_key,
                    responsibility_kind="author",
                    party_key=author_dri,
                    party_source="github.pr.author",
                    basis_kind="source_native",
                    basis_detail="github.pr.author",
                    subject_ids=subject_ids,
                    responsibility_state=responsibility_state_for_basis("source_native", row, author_dri),
                    state_reason=responsibility_state_reason_for_basis("source_native", row, author_dri),
                )
            )

        for reviewer in split_owner_keys(row.get("requested_reviewer_keys")):
            out.append(
                make_work_responsibility_row(
                    source_instance=source_instance,
                    source_row=row,
                    person_ids=person_ids,
                    now=now,
                    subject_kind=subject_kind,
                    subject_key=subject_key,
                    responsibility_kind="reviewer",
                    party_key=reviewer,
                    party_source="github.pr.requested_reviewer",
                    basis_kind="source_native",
                    basis_detail="github.pr.requested_reviewers",
                    subject_ids=subject_ids,
                    responsibility_state=responsibility_state_for_basis("source_native", row, reviewer),
                    state_reason=responsibility_state_reason_for_basis("source_native", row, reviewer),
                )
            )
    return out


def work_evidence_need_responsibility_rows(
    conn: sqlite3.Connection,
    source_instance: str,
    person_ids: dict[str, int],
    now: str,
) -> list[dict[str, Any]]:
    rows = query_dicts(
        conn,
        """
        select id, key, workstream_key, owner_key, action_state, execution_state,
               source_url, latest_evidence_id, evidence_count, freshness_state, visibility, confidence,
               event_count, first_seen_at, last_activity_at, rank_score, generated_at
          from work_program_evidence_needs
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_program_evidence_need'
        """,
        (source_instance,),
    )
    out: list[dict[str, Any]] = []
    for row in rows:
        out.append(
            make_work_responsibility_row(
                source_instance=source_instance,
                source_row=row,
                person_ids=person_ids,
                now=now,
                subject_kind="work_program_evidence_need",
                subject_key=first_nonempty([row.get("key")]),
                responsibility_kind="validation_owner",
                party_key=first_nonempty([row.get("owner_key")]),
                party_source="generated.evidence_need_owner",
                basis_kind="generated_candidate",
                basis_detail=first_nonempty([row.get("execution_state"), row.get("action_state")]),
                subject_ids={"work_program_evidence_need_id": row.get("id")},
                responsibility_state="candidate",
                state_reason="generated evidence need owner requires validation before product accountability",
            )
        )
    return out


def make_work_responsibility_row(
    *,
    source_instance: str,
    source_row: dict[str, Any],
    person_ids: dict[str, int],
    now: str,
    subject_kind: str,
    subject_key: str,
    responsibility_kind: str,
    party_key: str,
    party_source: str,
    basis_kind: str,
    basis_detail: str,
    subject_ids: dict[str, Any],
    responsibility_state: str,
    state_reason: str,
) -> dict[str, Any]:
    subject_kind = first_nonempty([subject_kind])
    subject_key = first_nonempty([subject_key])
    party_kind, normalized_party_key, person_id = responsibility_party_identity(party_key, person_ids)
    basis_kind = first_nonempty([basis_kind]) or "generated_candidate"
    responsibility_state = first_nonempty([responsibility_state]) or "candidate"
    semantic_identity = [subject_kind, subject_key, normalized_party_key, responsibility_kind, basis_kind]
    external_id = stable_external_id("work_responsibility", semantic_identity)
    values = {
        "key": f"work-responsibility:cubicle-analytics:{source_instance}:{stable_digest(semantic_identity)}",
        "person_id": person_id,
        "workstream_id": subject_ids.get("workstream_id"),
        "pull_request_id": subject_ids.get("pull_request_id"),
        "ticket_id": subject_ids.get("ticket_id"),
        "work_action_id": subject_ids.get("work_action_id"),
        "work_blocker_id": subject_ids.get("work_blocker_id"),
        "work_program_evidence_need_id": subject_ids.get("work_program_evidence_need_id"),
        "work_program_item_id": subject_ids.get("work_program_item_id"),
        "workstream_key": first_nonempty([source_row.get("workstream_key")]),
        "subject_kind": subject_kind,
        "subject_key": subject_key,
        "party_kind": party_kind,
        "party_key": normalized_party_key,
        "party_source": first_nonempty([party_source]),
        "responsibility_kind": responsibility_kind,
        "basis_kind": basis_kind,
        "basis_detail": first_nonempty([basis_detail]),
        "responsibility_state": responsibility_state,
        "responsibility_state_reason": first_nonempty([state_reason]),
        "generated_at": now,
        "valid_from": first_nonempty([source_row.get("first_seen_at"), source_row.get("opened_at"), source_row.get("generated_at"), now]),
        "valid_until": None if responsibility_state in {"active", "candidate"} else now,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_responsibility",
        "external_id": external_id,
        "source_url": first_nonempty([source_row.get("source_url")]),
        "latest_evidence_id": source_row.get("latest_evidence_id"),
        "evidence_count": int_value(source_row.get("evidence_count")),
        "freshness_state": first_nonempty([source_row.get("freshness_state")]) or "unknown",
        "visibility": first_nonempty([source_row.get("visibility")]) or "unknown",
        "confidence": responsibility_confidence(safe_float(source_row.get("confidence")), basis_kind, party_kind),
        "event_count": max(1, int_value(source_row.get("event_count"))),
        "first_seen_at": first_nonempty([source_row.get("first_seen_at"), now]),
        "last_activity_at": first_nonempty([source_row.get("last_activity_at"), source_row.get("register_updated_at"), now]),
        "rank_score": safe_float(source_row.get("rank_score")),
        "created_at": now,
        "updated_at": now,
    }
    return values


def responsibility_party_identity(party_key: Any, person_ids: dict[str, int]) -> tuple[str, str, int | None]:
    key = first_nonempty([party_key])
    if not key or key in {"(unassigned)", "unassigned", "none", "null"}:
        return "unassigned", "unassigned", None
    if key in person_ids:
        return "person", key, person_ids[key]
    if key.startswith("team:") or key.startswith("group:"):
        return "team", key, None
    return "unresolved", key, None


def split_owner_keys(value: Any) -> list[str]:
    owners: list[str] = []
    for item in split_csv(first_nonempty([value])):
        owner = github_owner_hint(item) if not item.startswith("github:") and "@" not in item else item
        if owner and owner not in owners:
            owners.append(owner)
    return owners


def responsibility_basis_from_program_owner(owner_source: str) -> tuple[str, str, str]:
    source = first_nonempty([owner_source])
    if source == "pr_author":
        return "accountable", "source_native", "github.pr.author"
    if source == "pr_author_waiting_on_requested_reviewer":
        return "coordinator", "source_native", "github.pr.author_waiting_on_requested_reviewer"
    if source == "jira_assignee":
        return "assignee", "source_native", "jira.issue.assignee"
    return "accountable", "generated_candidate", source or "generated.program_owner"


def responsibility_state_for_basis(basis_kind: str, row: dict[str, Any], party_key: Any) -> str:
    party_kind, _, _ = responsibility_party_identity(party_key, {})
    if party_kind == "unassigned":
        return "candidate"
    freshness_state = first_nonempty([row.get("freshness_state")])
    if basis_kind in {"source_native", "derived_from_relationship", "human_override", "imported_label"} and freshness_state != "partial":
        return "active"
    return "candidate"


def responsibility_state_reason_for_basis(basis_kind: str, row: dict[str, Any], party_key: Any) -> str:
    party_kind, _, _ = responsibility_party_identity(party_key, {})
    if party_kind == "unassigned":
        return "no accountable party was resolved"
    freshness_state = first_nonempty([row.get("freshness_state")])
    if basis_kind == "source_native" and freshness_state == "partial":
        return "source-native owner signal is retained but source coverage is partial"
    if basis_kind == "source_native":
        return "source-native field supports active responsibility"
    if basis_kind == "generated_candidate":
        return "generated routing hint requires validation before product accountability"
    return "responsibility imported or derived from a trusted relationship"


def responsibility_confidence(base_confidence: float, basis_kind: str, party_kind: str) -> float:
    confidence = base_confidence or 0.75
    if basis_kind == "generated_candidate":
        confidence = min(confidence, 0.75)
    if party_kind in {"unassigned", "unresolved"}:
        confidence = min(confidence, 0.8)
    return confidence


def dedupe_work_responsibility_rows(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    best: dict[str, dict[str, Any]] = {}
    for row in rows:
        if not first_nonempty([row.get("subject_kind")]) or not first_nonempty([row.get("subject_key")]):
            continue
        key = first_nonempty([row.get("key")])
        if not key:
            continue
        existing = best.get(key)
        if existing is None or responsibility_row_rank(row) > responsibility_row_rank(existing):
            best[key] = row
    return list(best.values())


def responsibility_row_rank(row: dict[str, Any]) -> tuple[int, float, str]:
    state_rank = {"active": 3, "candidate": 2, "resolved": 1, "superseded": 0, "rejected": 0}.get(first_nonempty([row.get("responsibility_state")]), 0)
    basis_rank = {
        "human_override": 5,
        "source_native": 4,
        "derived_from_relationship": 3,
        "imported_label": 2,
        "generated_candidate": 1,
    }.get(first_nonempty([row.get("basis_kind")]), 0)
    return (state_rank + basis_rank, safe_float(row.get("confidence")), first_nonempty([row.get("last_activity_at")]))


def upsert_responsibility_evidence(
    conn: sqlite3.Connection,
    source_instance: str,
    responsibility_id: int,
    values: dict[str, Any],
    now: str,
) -> int | None:
    locator = first_nonempty([values.get("external_id"), values.get("key")])
    if not locator:
        return None
    excerpt = (
        f"{values['responsibility_kind']} responsibility for {values['subject_kind']} {values['subject_key']} "
        f"routes to {values['party_kind']} {values['party_key']} via {values['basis_kind']}."
    )
    evidence_key = f"evidence:cubicle-analytics:{source_instance}:{stable_digest(['work_responsibility', responsibility_id, locator])}"
    evidence_values = {
        "key": evidence_key,
        "claim_kind": "relationship",
        "claim_target_kind": "work_responsibility",
        "claim_target_id": responsibility_id,
        "claim_field": "responsibility_state",
        "relationship_kind": values["responsibility_kind"],
        "relationship_id": responsibility_id,
        "locator_kind": "tpm_work_responsibility",
        "locator": locator,
        "source_span_key": stable_digest([locator, excerpt]),
        "excerpt": excerpt,
        "proof_state": "current",
        "observed_at": now,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_responsibility_evidence",
        "external_id": evidence_key,
        "source_url": first_nonempty([values.get("source_url")]),
        "source_updated_at": now,
        "content_hash": stable_digest([locator, excerpt, values.get("responsibility_state")]),
        "deletion_state": "present",
        "acl_state": "unavailable",
        "last_confirmed_at": now,
        "last_changed_at": now,
        "freshness_state": values.get("freshness_state") or "unknown",
        "visibility": values.get("visibility") or "unknown",
        "confidence": values.get("confidence") or 0.75,
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, "evidences", evidence_values, "key")
    return int(conn.execute("select id from evidences where key = ?", (evidence_key,)).fetchone()[0])


def query_dicts(conn: sqlite3.Connection, sql: str, params: tuple[Any, ...] = ()) -> list[dict[str, Any]]:
    cursor = conn.execute(sql, params)
    columns = [str(column[0]) for column in cursor.description or []]
    return [dict(zip(columns, row)) for row in cursor.fetchall()]


def persist_work_program_tpm_function_readiness_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    table_name = "work_program_tpm_function_readinesses"
    required = [table_name, "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before TPM function readiness materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"TPM function readiness cannot link workstream {workstream_key}")

    facts = ontology_work_program_adversarial_facts(conn, source_instance, workstream_key, now)
    functions = build_work_program_tpm_function_readiness(readiness, forecast_summary, facts)
    quality_gate_ids = ontology_work_program_quality_gate_ids_by_key(conn, source_instance, workstream_key, now)
    run_prefix = f"{workstream_key}|{now}|"
    conn.execute(
        f"""
        delete from {table_name}
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_work_program_tpm_function_readiness'
          and external_id like ?
        """,
        (source_instance, run_prefix + "%"),
    )
    for rank, function in enumerate(functions):
        external_id = f"{run_prefix}{function['function_key']}"
        readiness_state = first_nonempty([function.get("readiness_state")]) or "blocked"
        automation_state = first_nonempty([function.get("automation_state")]) or "unknown"
        human_required = bool(function.get("human_required"))
        blocking_gate_keys = unique_strings(function.get("blocking_gate_keys", []))
        values = {
            "key": f"work-program-tpm-function-readiness:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "workstream_key": workstream_key,
            "generated_at": now,
            "function_key": first_nonempty([function.get("function_key")]) or "unknown",
            "function_name": first_nonempty([function.get("function_name")]) or "TPM function",
            "readiness_state": readiness_state,
            "automation_state": automation_state,
            "human_required": human_required,
            "supporting_signal_count": int_value(function.get("supporting_signal_count")),
            "blocking_gate_keys": "\n".join(blocking_gate_keys),
            "detail": first_nonempty([function.get("detail")]) or "No readiness detail generated.",
            "recommended_action": first_nonempty([function.get("recommended_action")]) or "Review the generated readiness state before autonomous TPM use.",
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_program_tpm_function_readiness",
            "external_id": external_id,
            "source_url": workstream_source_url(workstream_key),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": tpm_function_readiness_confidence(readiness_state, blocking_gate_keys),
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": 100.0 - float(rank),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, table_name, values, "key")
        function_id = int(conn.execute(f"select id from {table_name} where key = ?", (values["key"],)).fetchone()[0])
        link_work_program_tpm_function_readiness_quality_gates(
            conn,
            function_id,
            [quality_gate_ids[key] for key in blocking_gate_keys if key in quality_gate_ids],
        )
        excerpt = f"{values['function_name']} is {values['readiness_state']} via {values['automation_state']}: {values['detail']} {values['recommended_action']}".strip()
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_tpm_function_readiness",
            function_id,
            "readiness_state",
            "tpm_function_readiness",
            external_id,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (values["confidence"], evidence_id),
            )
            conn.execute(
                f"update {table_name} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, function_id),
            )
    conn.commit()


def link_work_program_tpm_function_readiness_quality_gates(
    conn: sqlite3.Connection,
    function_readiness_id: int,
    quality_gate_ids: list[int],
) -> None:
    table_name = "work_program_tpm_function_readiness_blocking_quality_gates"
    if not table_exists(conn, table_name):
        return
    required_columns = {"work_program_tpm_function_readiness_id", "work_program_quality_gate_id"}
    if not required_columns.issubset(table_columns(conn, table_name)):
        return
    conn.execute(
        f"delete from {table_name} where work_program_tpm_function_readiness_id = ?",
        (function_readiness_id,),
    )
    for gate_id in unique_ints(quality_gate_ids):
        conn.execute(
            f"""
            insert or ignore into {table_name} (
                work_program_tpm_function_readiness_id,
                work_program_quality_gate_id
            ) values (?, ?)
            """,
            (function_readiness_id, gate_id),
        )


def persist_work_program_automation_readiness_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    table_name = "work_program_automation_readinesses"
    required = [table_name, "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before automation-readiness materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"automation readiness cannot link workstream {workstream_key}")

    facts = ontology_work_program_adversarial_facts(conn, source_instance, workstream_key, now)
    gates = build_work_program_quality_gates(readiness, forecast_summary, facts)
    evidence_needs = build_work_program_evidence_needs(readiness, forecast_summary, facts)
    function_readiness = build_work_program_tpm_function_readiness(readiness, forecast_summary, facts)
    snapshot = build_work_program_automation_readiness(readiness, forecast_summary, facts, gates, evidence_needs, function_readiness)
    run_prefix = f"{workstream_key}|{now}|"
    conn.execute(
        f"""
        delete from {table_name}
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_work_program_automation_readiness'
          and external_id like ?
        """,
        (source_instance, run_prefix + "%"),
    )
    external_id = f"{run_prefix}automation_readiness"
    blocking_gate_keys = unique_strings(snapshot.get("blocking_gate_keys", []))
    values = {
        "key": f"work-program-automation-readiness:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
        "workstream_id": workstream_id,
        "workstream_key": workstream_key,
        "generated_at": now,
        "readiness_state": first_nonempty([snapshot.get("readiness_state")]) or "blocked",
        "readiness_score": optional_rate(snapshot.get("readiness_score")) or 0.0,
        "autonomous_action_ready": bool(snapshot.get("autonomous_action_ready")),
        "human_review_required": bool(snapshot.get("human_review_required")),
        "safe_automation_areas": "\n".join(unique_strings(snapshot.get("safe_automation_areas", []))),
        "human_required_areas": "\n".join(unique_strings(snapshot.get("human_required_areas", []))),
        "rationale": first_nonempty([snapshot.get("rationale")]) or "Automation readiness has not been evaluated.",
        "required_evidence": "\n".join(unique_strings(snapshot.get("required_evidence", []))),
        "blocking_gate_keys": "\n".join(blocking_gate_keys),
        "quality_gate_count": int_value(snapshot.get("quality_gate_count")),
        "blocking_gate_count": int_value(snapshot.get("blocking_gate_count")),
        "evidence_need_count": int_value(snapshot.get("evidence_need_count")),
        "tpm_function_count": int_value(snapshot.get("tpm_function_count")),
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_program_automation_readiness",
        "external_id": external_id,
        "source_url": workstream_source_url(workstream_key),
        "latest_evidence_id": None,
        "evidence_count": 0,
        "freshness_state": "fresh",
        "visibility": "public",
        "confidence": automation_readiness_confidence(first_nonempty([snapshot.get("readiness_state")]), blocking_gate_keys),
        "event_count": 1,
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": optional_rate(snapshot.get("readiness_score")) or 0.0,
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, table_name, values, "key")
    readiness_id = int(conn.execute(f"select id from {table_name} where key = ?", (values["key"],)).fetchone()[0])
    excerpt = f"Automation readiness is {values['readiness_state']} with score {values['readiness_score']}: {values['rationale']}".strip()
    evidence_id = upsert_generated_evidence(
        conn,
        source_instance,
        "work_program_automation_readiness",
        readiness_id,
        "readiness_state",
        "tpm_automation_readiness",
        external_id,
        excerpt,
        now,
    )
    if evidence_id is not None:
        conn.execute(
            "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
            (values["confidence"], evidence_id),
        )
        conn.execute(
            f"update {table_name} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
            (evidence_id, readiness_id),
        )
    conn.commit()


def persist_work_program_summary_snapshot_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    table_name = "work_program_summary_snapshots"
    required = [table_name, "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before summary-snapshot materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"summary snapshot cannot link workstream {workstream_key}")

    facts = ontology_work_program_adversarial_facts(conn, source_instance, workstream_key, now)
    snapshot = build_work_program_summary_snapshot(readiness, forecast_summary, facts)
    breakdown_dimensions, breakdown_keys, breakdown_counts = work_program_summary_breakdown_fields(snapshot)
    external_id = f"{workstream_key}|{now}|summary_snapshot"
    values = {
        "key": f"work-program-summary-snapshot:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
        "workstream_id": workstream_id,
        "workstream_key": workstream_key,
        "generated_at": now,
        "total_count": int_value(snapshot.get("total_count")),
        "needs_decision_count": int_value(snapshot.get("needs_decision_count")),
        "validate_signal_count": int_value(snapshot.get("validate_signal_count")),
        "ci_failing_count": int_value(snapshot.get("ci_failing_count")),
        "waiting_review_count": int_value(snapshot.get("waiting_review_count")),
        "source_repair_count": int_value(snapshot.get("source_repair_count")),
        "closed_pending_review_count": int_value(snapshot.get("closed_pending_review_count")),
        "model_quality_count": int_value(snapshot.get("model_quality_count")),
        "closure_candidate_count": int_value(snapshot.get("closure_candidate_count")),
        "dismissed_count": int_value(snapshot.get("dismissed_count")),
        "now_count": int_value(snapshot.get("now_count")),
        "high_risk_count": int_value(snapshot.get("high_risk_count")),
        "unassigned_count": int_value(snapshot.get("unassigned_count")),
        "product_action_count": int_value(snapshot.get("product_action_count")),
        "validation_lead_count": int_value(snapshot.get("validation_lead_count")),
        "source_coverage_limited_count": int_value(snapshot.get("source_coverage_limited_count")),
        "owner_load_status": first_nonempty([snapshot.get("owner_load_status")]) or "clear",
        "owner_load_action_count": int_value(snapshot.get("owner_load_action_count")),
        "overloaded_owner_count": int_value(snapshot.get("overloaded_owner_count")),
        "attention_owner_count": int_value(snapshot.get("attention_owner_count")),
        "unassigned_action_count": int_value(snapshot.get("unassigned_action_count")),
        "blocker_count": int_value(snapshot.get("blocker_count")),
        "active_blocker_count": int_value(snapshot.get("active_blocker_count")),
        "validating_blocker_count": int_value(snapshot.get("validating_blocker_count")),
        "blocker_impact_count": int_value(snapshot.get("blocker_impact_count")),
        "active_blocker_impact_count": int_value(snapshot.get("active_blocker_impact_count")),
        "dependency_edge_count": int_value(snapshot.get("dependency_edge_count")),
        "blocking_dependency_count": int_value(snapshot.get("blocking_dependency_count")),
        "needs_action_dependency_count": int_value(snapshot.get("needs_action_dependency_count")),
        "operating_status": first_nonempty([snapshot.get("operating_status")]) or "clear",
        "decision_pressure": first_nonempty([snapshot.get("decision_pressure")]) or "watch",
        "forecast_state": first_nonempty([snapshot.get("forecast_state")]) or "missing",
        "primary_risk": first_nonempty([snapshot.get("primary_risk")]),
        "recommended_focus": first_nonempty([snapshot.get("recommended_focus")]) or "Maintain watch on typed program items.",
        "capability_gaps": "\n".join(unique_strings(snapshot.get("capability_gaps", []))),
        "breakdown_dimensions": breakdown_dimensions,
        "breakdown_keys": breakdown_keys,
        "breakdown_counts": breakdown_counts,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_program_summary_snapshot",
        "external_id": external_id,
        "source_url": workstream_source_url(workstream_key),
        "latest_evidence_id": None,
        "evidence_count": 0,
        "freshness_state": "fresh",
        "visibility": "public",
        "confidence": work_program_summary_snapshot_confidence(snapshot),
        "event_count": max(1, int_value(snapshot.get("total_count"))),
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": work_program_summary_snapshot_rank_score(snapshot),
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, table_name, values, "key")
    snapshot_id = int(conn.execute(f"select id from {table_name} where key = ?", (values["key"],)).fetchone()[0])
    excerpt = (
        f"{values['operating_status']}: {count_phrase(values['total_count'], 'typed program item')}; "
        f"{count_phrase(values['active_blocker_count'], 'active blocker')}; "
        f"{count_phrase(values['product_action_count'], 'product action')}; "
        f"{values['recommended_focus']}"
    )
    evidence_id = upsert_generated_evidence(
        conn,
        source_instance,
        "work_program_summary_snapshot",
        snapshot_id,
        "operating_status",
        "tpm_program_summary",
        external_id,
        excerpt,
        now,
    )
    if evidence_id is not None:
        conn.execute(
            "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
            (values["confidence"], evidence_id),
        )
        conn.execute(
            f"update {table_name} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
            (evidence_id, snapshot_id),
        )
    conn.commit()


def persist_work_program_owner_rollup_snapshots_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    generated_at: str,
) -> None:
    table_name = "work_program_owner_rollup_snapshots"
    required = [table_name, "work_program_items", "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before owner-rollup materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"owner rollup snapshot cannot link workstream {workstream_key}")

    program_items = ontology_program_items_for_owner_rollups(conn, source_instance, workstream_key)
    rows = build_work_program_owner_rollup_snapshots(program_items, workstream_key, now)
    external_prefix = f"{workstream_key}|{now}|owner_rollup|"
    conn.execute(
        f"""
        delete from {table_name}
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_program_owner_rollup_snapshot'
           and external_id like ?
        """,
        (source_instance, f"{external_prefix}%"),
    )
    for row in rows:
        owner_key = first_nonempty([row.get("owner_key")])
        if not owner_key:
            continue
        external_id = f"{external_prefix}{owner_key}"
        top_item_keys = [first_nonempty([value]) for value in row.get("top_item_keys", []) if first_nonempty([value])]
        freshness_state = first_nonempty([row.get("freshness_state")]) or "fresh"
        confidence = 0.75 if freshness_state == "partial" else 0.9
        values = {
            "key": f"work-program-owner-rollup-snapshot:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "workstream_key": workstream_key,
            "generated_at": now,
            "owner_key": owner_key,
            "owner_source": first_nonempty([row.get("owner_source")]),
            "item_count": int_value(row.get("item_count")),
            "needs_decision_count": int_value(row.get("needs_decision_count")),
            "validate_signal_count": int_value(row.get("validate_signal_count")),
            "ci_failing_count": int_value(row.get("ci_failing_count")),
            "waiting_review_count": int_value(row.get("waiting_review_count")),
            "source_repair_count": int_value(row.get("source_repair_count")),
            "closure_candidate_count": int_value(row.get("closure_candidate_count")),
            "now_count": int_value(row.get("now_count")),
            "high_risk_count": int_value(row.get("high_risk_count")),
            "max_risk_score": safe_float(row.get("max_risk_score")),
            "top_item_keys": "\n".join(top_item_keys),
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_program_owner_rollup_snapshot",
            "external_id": external_id,
            "source_url": workstream_source_url(workstream_key),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": freshness_state,
            "visibility": "public",
            "confidence": confidence,
            "event_count": int_value(row.get("item_count")),
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": safe_float(row.get("max_risk_score")),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, table_name, values, "key")
        snapshot_id = int(conn.execute(f"select id from {table_name} where key = ?", (values["key"],)).fetchone()[0])
        excerpt = (
            f"{owner_key} owns {count_phrase(values['item_count'], 'typed program item')}; "
            f"{count_phrase(values['needs_decision_count'], 'needs-decision item')}; "
            f"{count_phrase(values['validate_signal_count'], 'validation item')}; "
            f"max risk {values['max_risk_score']:.1f}."
        )
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_owner_rollup_snapshot",
            snapshot_id,
            "item_count",
            "tpm_program_owner_rollup",
            external_id,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = ?, confidence = ?, visibility = 'public' where id = ?",
                (freshness_state, confidence, evidence_id),
            )
            conn.execute(
                f"update {table_name} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, snapshot_id),
            )
    conn.commit()


def ontology_program_items_for_owner_rollups(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
) -> pd.DataFrame:
    columns = table_columns(conn, "work_program_items")
    select_fields = {
        "key": "''",
        "workstream_key": "''",
        "program_status": "'unknown'",
        "owner_key": "''",
        "owner_source": "''",
        "due_bucket": "'unscheduled'",
        "risk_score": "0",
        "rank_score": "0",
        "freshness_state": "'unknown'",
        "register_updated_at": "''",
        "last_activity_at": "''",
        "updated_at": "''",
    }
    select_sql = ", ".join(
        f"{name} as {name}" if name in columns else f"{fallback} as {name}"
        for name, fallback in select_fields.items()
    )
    predicates = [
        "source_system = 'cubicle_analytics'",
        "source_instance = ?",
        f"workstream_key in ({', '.join(['?'] * len(work_program_workstream_sql_keys(workstream_key)))})",
    ]
    params: list[Any] = [source_instance, *work_program_workstream_sql_keys(workstream_key)]
    if "external_kind" in columns:
        predicates.append("external_kind = 'tpm_program_item'")
    query = f"""
        select {select_sql}
          from work_program_items
         where {" and ".join(predicates)}
    """
    return pd.read_sql_query(query, conn, params=params)


def work_program_workstream_sql_keys(workstream_key: str) -> list[str]:
    key = first_nonempty([workstream_key])
    if not key:
        return [""]
    if key.startswith("workstream:"):
        return [key, key.removeprefix("workstream:")]
    return [key, f"workstream:{key}"]


def build_work_program_owner_rollup_snapshots(
    program_items: pd.DataFrame,
    workstream_key: str,
    generated_at: str,
) -> list[dict[str, Any]]:
    if program_items.empty:
        return []
    grouped: dict[str, list[dict[str, Any]]] = {}
    for row in program_items.to_dict("records"):
        owner_key = first_nonempty([row.get("owner_key")]) or "unassigned"
        grouped.setdefault(owner_key, []).append(row)

    out: list[dict[str, Any]] = []
    for owner_key, rows in grouped.items():
        status_counts = Counter(first_nonempty([row.get("program_status")]) or "unknown" for row in rows)
        top_item_keys = top_work_program_item_keys(rows, 3)
        freshness_state = "partial" if any(first_nonempty([row.get("freshness_state")]) == "partial" for row in rows) else "fresh"
        out.append(
            {
                "workstream_key": workstream_key,
                "generated_at": generated_at,
                "owner_key": owner_key,
                "owner_source": first_nonempty(row.get("owner_source") for row in rows),
                "item_count": len(rows),
                "needs_decision_count": status_counts.get("needs_decision", 0),
                "validate_signal_count": status_counts.get("validate_signal", 0),
                "ci_failing_count": status_counts.get("ci_failing", 0),
                "waiting_review_count": status_counts.get("waiting_review", 0),
                "source_repair_count": status_counts.get("source_repair", 0),
                "closure_candidate_count": status_counts.get("closure_candidate", 0),
                "now_count": sum(1 for row in rows if first_nonempty([row.get("due_bucket")]) == "now"),
                "high_risk_count": sum(1 for row in rows if safe_float(row.get("risk_score")) >= 90),
                "max_risk_score": max((safe_float(row.get("risk_score")) for row in rows), default=0.0),
                "top_item_keys": top_item_keys,
                "freshness_state": freshness_state,
            }
        )
    return sorted(
        out,
        key=lambda row: (-safe_float(row.get("max_risk_score")), -int_value(row.get("item_count")), first_nonempty([row.get("owner_key")])),
    )


def top_work_program_item_keys(rows: list[dict[str, Any]], limit: int) -> list[str]:
    def updated_timestamp(row: dict[str, Any]) -> float:
        value = first_nonempty([row.get("last_activity_at"), row.get("register_updated_at"), row.get("updated_at")])
        parsed = parse_dt(value)
        return parsed.timestamp() if parsed is not None else 0.0

    sorted_rows = sorted(
        rows,
        key=lambda row: (
            -safe_float(row.get("risk_score")),
            -safe_float(row.get("rank_score")),
            -updated_timestamp(row),
            first_nonempty([row.get("key")]),
        ),
    )
    return [first_nonempty([row.get("key")]) for row in sorted_rows[:limit] if first_nonempty([row.get("key")])]


def persist_work_insight_evaluation_snapshots_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    generated_at: str,
) -> None:
    snapshot_table = "work_insight_evaluation_snapshots"
    kind_table = "work_insight_kind_evaluation_snapshots"
    required = [snapshot_table, kind_table, "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before insight-evaluation materialization")
    ensure_work_insight_kind_evaluation_snapshot_compat_columns(conn)

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    metrics = metric_map(readiness)
    kind_rows = build_work_insight_kind_evaluation_snapshots(readiness)
    ready_kind_count = sum(1 for row in kind_rows if bool(row.get("ready_to_measure")))
    product_action_ready_kind_count = sum(1 for row in kind_rows if bool(row.get("ready_for_product_action")))
    quality_gated_kind_count = sum(
        1
        for row in kind_rows
        if first_nonempty([row.get("measurement_scope")]) == "product_candidate"
        and bool(row.get("ready_to_measure"))
        and not bool(row.get("ready_for_product_action"))
    )
    gated_kind_count = sum(1 for row in kind_rows if not bool(row.get("ready_to_measure")))
    external_id = f"{source_instance}|{now}|insight_evaluation"
    has_product_candidate_metrics = "product_candidate_insight_count" in metrics
    values = {
        "key": f"work-insight-evaluation-snapshot:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
        "generated_at": now,
        "current_insight_count": int_metric(metrics.get("product_candidate_insight_count") if has_product_candidate_metrics else metrics.get("current_insight_count")),
        "review_row_count": int_metric(metrics.get("product_candidate_review_row_count") if has_product_candidate_metrics else metrics.get("review_row_count")),
        "measurement_label_count": int_metric(
            metrics.get("product_candidate_measurement_label_count")
            if has_product_candidate_metrics
            else metrics.get("evaluation_label_row_count") or metrics.get("label_row_count")
        ),
        "open_review_request_count": int_metric(metrics.get("product_candidate_open_review_request_count") if has_product_candidate_metrics else metrics.get("open_review_request_count")),
        "min_labeled_total_required": MIN_MEASUREMENT_LABEL_TOTAL,
        "min_labeled_per_kind_required": MIN_MEASUREMENT_LABEL_PER_KIND,
        "min_precision_rate_for_product_action": MIN_PRECISION_RATE_FOR_PRODUCT_ACTION,
        "min_useful_signal_rate_for_product_action": MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION,
        "min_actionability_rate_for_product_action": MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION,
        "precision_rate": safe_float(metrics.get("product_candidate_precision_rate") if has_product_candidate_metrics else metrics.get("precision_rate")),
        "useful_signal_rate": safe_float(metrics.get("product_candidate_useful_signal_rate") if has_product_candidate_metrics else metrics.get("useful_signal_rate")),
        "actionability_rate": safe_float(metrics.get("product_candidate_actionability_rate") if has_product_candidate_metrics else metrics.get("actionability_rate")),
        "false_positive_rate": safe_float(metrics.get("product_candidate_false_positive_rate") if has_product_candidate_metrics else metrics.get("false_positive_rate")),
        "measurement_coverage_rate": safe_float(metrics.get("product_candidate_measurement_coverage_rate") if has_product_candidate_metrics else metrics.get("measurement_coverage_rate")),
        "ready_to_measure_precision": str(
            metrics.get("product_candidate_ready_to_measure_precision")
            if has_product_candidate_metrics
            else metrics.get("ready_to_measure_precision", "")
        ).lower()
        == "true",
        "ready_to_measure_actionability": str(
            metrics.get("product_candidate_ready_to_measure_actionability")
            if has_product_candidate_metrics
            else metrics.get("ready_to_measure_actionability", "")
        ).lower()
        == "true",
        "ready_insight_kind_count": ready_kind_count,
        "product_action_ready_kind_count": product_action_ready_kind_count,
        "quality_gated_insight_kind_count": quality_gated_kind_count,
        "gated_insight_kind_count": gated_kind_count,
        "recommended_next_step": work_insight_evaluation_next_step(kind_rows),
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_insight_evaluation_snapshot",
        "external_id": external_id,
        "source_url": workstream_source_url("flink-kubernetes-operator"),
        "latest_evidence_id": None,
        "evidence_count": 0,
        "freshness_state": "fresh",
        "visibility": "public",
        "confidence": work_insight_evaluation_confidence(kind_rows, int_metric(metrics.get("evaluation_label_row_count") or metrics.get("label_row_count"))),
        "event_count": max(1, int_metric(metrics.get("current_insight_count"))),
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": work_insight_evaluation_rank_score(values_for_rank=metrics, ready_kind_count=ready_kind_count, gated_kind_count=gated_kind_count),
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, snapshot_table, values, "key")
    snapshot_id = int(conn.execute(f"select id from {snapshot_table} where key = ?", (values["key"],)).fetchone()[0])
    excerpt = (
        f"Insight evaluation: {values['measurement_label_count']} measurement label(s), "
        f"precision {values['precision_rate']:.2f}, actionability {values['actionability_rate']:.2f}. "
        f"{values['recommended_next_step']}"
    )
    evidence_id = upsert_generated_evidence(
        conn,
        source_instance,
        "work_insight_evaluation_snapshot",
        snapshot_id,
        "precision_rate",
        "tpm_insight_evaluation",
        external_id,
        excerpt,
        now,
    )
    if evidence_id is not None:
        conn.execute(
            "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
            (values["confidence"], evidence_id),
        )
        conn.execute(
            f"update {snapshot_table} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
            (evidence_id, snapshot_id),
        )

    run_prefix = f"{source_instance}|{now}|insight_kind|"
    conn.execute(
        f"""
        delete from {kind_table}
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_work_insight_kind_evaluation_snapshot'
          and external_id like ?
        """,
        (source_instance, run_prefix + "%"),
    )
    for rank, row in enumerate(kind_rows):
        insight_kind = first_nonempty([row.get("insight_kind")]) or "unknown"
        measurement_scope = normalize_insight_measurement_scope(row.get("measurement_scope"), insight_kind)
        kind_external_id = f"{run_prefix}{insight_kind}"
        kind_values = {
            "key": f"work-insight-kind-evaluation-snapshot:cubicle-analytics:{source_instance}:{stable_digest([kind_external_id])}",
            "work_insight_evaluation_snapshot_id": snapshot_id,
            "generated_at": now,
            "insight_kind": insight_kind,
            "measurement_scope": measurement_scope,
            "current_insight_count": int_value(row.get("current_insight_count")),
            "review_row_count": int_value(row.get("review_row_count")),
            "measurement_label_count": int_value(row.get("measurement_label_count")),
            "open_review_request_count": int_value(row.get("open_review_request_count")),
            "truth_labeled_count": int_value(row.get("truth_labeled_count")),
            "actionability_labeled_count": int_value(row.get("actionability_labeled_count")),
            "true_positive_count": int_value(row.get("true_positive_count")),
            "false_positive_count": int_value(row.get("false_positive_count")),
            "partial_count": int_value(row.get("partial_count")),
            "actionable_count": int_value(row.get("actionable_count")),
            "needs_owner_count": int_value(row.get("needs_owner_count")),
            "precision_rate": safe_float(row.get("precision_rate")),
            "useful_signal_rate": safe_float(row.get("useful_signal_rate")),
            "actionability_rate": safe_float(row.get("actionability_rate")),
            "false_positive_rate": safe_float(row.get("false_positive_rate")),
            "measurement_coverage_rate": safe_float(row.get("measurement_coverage_rate")),
            "required_label_count": int_value(row.get("required_label_count")),
            "ready_to_measure": bool(row.get("ready_to_measure")),
            "ready_for_product_action": bool(row.get("ready_for_product_action")),
            "product_action_gate_state": first_nonempty([row.get("product_action_gate_state")]) or "measurement_gated",
            "product_action_gate_reason": first_nonempty([row.get("product_action_gate_reason")]) or "Insight kind needs measurement labels before product-action quality can be measured.",
            "recommended_action": first_nonempty([row.get("recommended_action")]) or "Gold-label current insights before promoting this kind beyond validation leads.",
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_insight_kind_evaluation_snapshot",
            "external_id": kind_external_id,
            "source_url": workstream_source_url("flink-kubernetes-operator"),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": work_insight_kind_evaluation_confidence(row),
            "event_count": max(1, int_value(row.get("current_insight_count"))),
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": work_insight_kind_evaluation_rank_score(row, rank),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, kind_table, kind_values, "key")
        kind_id = int(conn.execute(f"select id from {kind_table} where key = ?", (kind_values["key"],)).fetchone()[0])
        kind_excerpt = (
            f"{insight_kind}: {kind_values['product_action_gate_state']}. "
            f"precision {kind_values['precision_rate']:.2f}, actionability {kind_values['actionability_rate']:.2f}. "
            f"{kind_values['recommended_action']}"
        )
        kind_evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_insight_kind_evaluation_snapshot",
            kind_id,
            "product_action_gate_state",
            "tpm_insight_kind_evaluation",
            kind_external_id,
            kind_excerpt,
            now,
        )
        if kind_evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (kind_values["confidence"], kind_evidence_id),
            )
            conn.execute(
                f"update {kind_table} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (kind_evidence_id, kind_id),
            )
    conn.commit()


def ensure_work_insight_kind_evaluation_snapshot_compat_columns(conn: sqlite3.Connection) -> None:
    table_name = "work_insight_kind_evaluation_snapshots"
    if not table_exists(conn, table_name):
        return
    if not column_exists(conn, table_name, "measurement_scope"):
        conn.execute(
            f"alter table {table_name} add column measurement_scope text not null default ''"
        )


def build_work_insight_kind_evaluation_snapshots(readiness: pd.DataFrame) -> list[dict[str, Any]]:
    if readiness.empty:
        return []
    metrics = metric_map(readiness)
    kinds = set()
    for metric in metrics:
        if metric.startswith("review_requests_"):
            kinds.add(metric.removeprefix("review_requests_"))
        elif metric.startswith("measurement_required_"):
            kinds.add(metric.removeprefix("measurement_required_"))

    rows: list[dict[str, Any]] = []
    for insight_kind in sorted(kinds):
        measurement_scope = normalize_insight_measurement_scope(metrics.get(f"measurement_scope_{insight_kind}"), insight_kind)
        current_count = int_metric(metrics.get(f"review_requests_{insight_kind}"))
        required = int_metric(metrics.get(f"measurement_required_{insight_kind}"))
        if required == 0 and current_count > 0:
            required = min(MIN_MEASUREMENT_LABEL_PER_KIND, current_count)
        ready_to_measure = str(metrics.get(f"ready_to_measure_{insight_kind}", "")).lower() == "true"
        precision_rate = safe_float(metrics.get(f"precision_rate_{insight_kind}"))
        useful_signal_rate = safe_float(metrics.get(f"useful_signal_rate_{insight_kind}"))
        actionability_rate = safe_float(metrics.get(f"actionability_rate_{insight_kind}"))
        ready_for_product_action = (
            measurement_scope == "product_candidate"
            and ready_to_measure
            and precision_rate >= MIN_PRECISION_RATE_FOR_PRODUCT_ACTION
            and useful_signal_rate >= MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION
            and actionability_rate >= MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION
        )
        values = {
            "insight_kind": insight_kind,
            "measurement_scope": measurement_scope,
            "current_insight_count": current_count,
            "review_row_count": int_metric(metrics.get(f"review_rows_{insight_kind}")),
            "measurement_label_count": int_metric(metrics.get(f"measurement_labels_{insight_kind}")),
            "open_review_request_count": int_metric(metrics.get(f"open_review_requests_{insight_kind}")),
            "truth_labeled_count": int_metric(metrics.get(f"truth_labeled_{insight_kind}")),
            "actionability_labeled_count": int_metric(metrics.get(f"actionability_labeled_{insight_kind}")),
            "true_positive_count": int_metric(metrics.get(f"true_positive_{insight_kind}")),
            "false_positive_count": int_metric(metrics.get(f"false_positive_{insight_kind}")),
            "partial_count": int_metric(metrics.get(f"partial_{insight_kind}")),
            "actionable_count": int_metric(metrics.get(f"actionable_{insight_kind}")),
            "needs_owner_count": int_metric(metrics.get(f"needs_owner_{insight_kind}")),
            "precision_rate": precision_rate,
            "useful_signal_rate": useful_signal_rate,
            "actionability_rate": actionability_rate,
            "false_positive_rate": safe_float(metrics.get(f"false_positive_rate_{insight_kind}")),
            "measurement_coverage_rate": safe_float(metrics.get(f"measurement_coverage_rate_{insight_kind}")),
            "required_label_count": required,
            "ready_to_measure": ready_to_measure,
            "ready_for_product_action": ready_for_product_action,
        }
        gate_state, gate_reason = work_insight_kind_product_action_gate(values)
        values["product_action_gate_state"] = gate_state
        values["product_action_gate_reason"] = gate_reason
        values["recommended_action"] = work_insight_kind_evaluation_action(values)
        rows.append(values)
    return sorted(
        rows,
        key=lambda row: (
            not bool(row.get("ready_for_product_action")),
            not bool(row.get("ready_to_measure")),
            -int_value(row.get("current_insight_count")),
            first_nonempty([row.get("insight_kind")]),
        ),
    )


def work_insight_kind_product_action_gate(values: dict[str, Any]) -> tuple[str, str]:
    measurement_scope = first_nonempty([values.get("measurement_scope")]) or insight_measurement_scope(first_nonempty([values.get("insight_kind")]))
    if measurement_scope == "context_only":
        return "context_only", "This insight kind is retained for routing, topology, or workload context and cannot independently support product-action automation."
    if measurement_scope == "model_quality":
        return "model_quality", "This insight kind measures model or rule quality; it gates automation readiness but is not product action."
    if measurement_scope == "validation_lead":
        return "validation_only", "This insight kind can create validation leads, but it has no product-action automation contract yet."
    ready_to_measure = bool(values.get("ready_to_measure"))
    ready_for_product_action = bool(values.get("ready_for_product_action"))
    required = int_value(values.get("required_label_count"))
    if not ready_to_measure:
        missing = max(0, required - int_value(values.get("measurement_label_count")))
        return "measurement_gated", f"Needs {missing} more gold label(s) before product-action quality can be measured."
    if ready_for_product_action:
        return "passed", "Measured precision, useful-signal, and actionability rates meet product-action thresholds."
    missing = []
    if safe_float(values.get("precision_rate")) < MIN_PRECISION_RATE_FOR_PRODUCT_ACTION:
        missing.append("precision")
    if safe_float(values.get("useful_signal_rate")) < MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION:
        missing.append("useful signal")
    if safe_float(values.get("actionability_rate")) < MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION:
        missing.append("actionability")
    return "quality_gated", f"Measured {', '.join(missing)} below product-action threshold."


def work_insight_kind_evaluation_action(values: dict[str, Any]) -> str:
    insight_kind = first_nonempty([values.get("insight_kind")])
    measurement_scope = first_nonempty([values.get("measurement_scope")]) or insight_measurement_scope(insight_kind)
    ready_to_measure = bool(values.get("ready_to_measure"))
    ready_for_product_action = bool(values.get("ready_for_product_action"))
    required = int_value(values.get("required_label_count"))
    missing = max(0, required - int_value(values.get("measurement_label_count")))
    if measurement_scope == "context_only":
        if insight_kind == "developer_correlation":
            return "Keep this as workload/routing context; label usefulness for capacity or escalation, not ownership, causality, performance, ETA, or blockers."
        if insight_kind == "dependency_cluster":
            return "Keep this as topology context until a source-backed blocking dependency or owner-confirmed coordination action exists."
        return "Keep this signal as context until a product-action contract exists."
    if measurement_scope == "model_quality":
        return "Use this to gate model readiness, not as a product escalation."
    if measurement_scope == "validation_lead":
        return "Use this signal for validation packets only until a product-action contract and measurement target are defined."
    if not ready_to_measure:
        if insight_kind == "forecast_risk":
            return f"Gold-label {missing} forecast-risk leads and keep ETA output gated until forecast quality beats simple baselines."
        if insight_kind == "model_quality":
            return "Review the forecast backtest gate before promoting ETA forecasts beyond risk triage."
        return f"Gold-label {missing} current {insight_kind} insight(s) before promoting this kind beyond validation leads."
    if ready_for_product_action:
        return "This insight kind meets product-action precision and actionability thresholds; keep sampling dismissed and partial cases."
    return "Measurement coverage is sufficient, but precision/actionability are too weak for product-action gating."


def work_insight_evaluation_next_step(kind_rows: list[dict[str, Any]]) -> str:
    if not kind_rows:
        return "No current generated insights are available for evaluation."
    product_rows = [
        row
        for row in kind_rows
        if first_nonempty([row.get("measurement_scope")]) == "product_candidate"
    ]
    needs_labels = [first_nonempty([row.get("insight_kind")]) for row in product_rows if not bool(row.get("ready_to_measure"))]
    needs_labels = [kind for kind in needs_labels if kind]
    if needs_labels:
        return "Gold-label " + ", ".join(needs_labels) + " before using those insight kinds for autonomous product actions; keep them as validation leads meanwhile."
    quality_gated = [first_nonempty([row.get("insight_kind")]) for row in product_rows if bool(row.get("ready_to_measure")) and not bool(row.get("ready_for_product_action"))]
    quality_gated = [kind for kind in quality_gated if kind]
    if quality_gated:
        return "Improve or suppress " + ", ".join(quality_gated) + " before promoting those insight kinds to product-action automation."
    context_rows = [
        first_nonempty([row.get("insight_kind")])
        for row in kind_rows
        if first_nonempty([row.get("measurement_scope")]) in {"context_only", "validation_lead"}
    ]
    context_rows = [kind for kind in context_rows if kind]
    if context_rows:
        return "Product-action kinds are measured; keep " + ", ".join(context_rows) + " as validation/context packets until source-backed product-action contracts exist."
    return "All current insight kinds have enough gold labels for measurement; use kind-level product-action gates before promoting any signal to autonomous product actions."


def work_insight_evaluation_confidence(kind_rows: list[dict[str, Any]], label_count: int) -> float:
    if not kind_rows:
        return 0.6
    ready_count = sum(1 for row in kind_rows if bool(row.get("ready_to_measure")))
    base = 0.65 + min(0.25, label_count / max(1, MIN_MEASUREMENT_LABEL_TOTAL) * 0.25)
    if ready_count == len(kind_rows):
        base += 0.1
    return min(1.0, round(base, 3))


def work_insight_kind_evaluation_confidence(row: dict[str, Any]) -> float:
    if bool(row.get("ready_to_measure")):
        return 0.95 if bool(row.get("ready_for_product_action")) else 0.9
    coverage = safe_float(row.get("measurement_coverage_rate"))
    return round(min(0.85, 0.6 + coverage * 0.25), 3)


def work_insight_evaluation_rank_score(values_for_rank: dict[str, str], ready_kind_count: int, gated_kind_count: int) -> float:
    coverage = safe_float(values_for_rank.get("measurement_coverage_rate"))
    return round(coverage * 70.0 + ready_kind_count * 5.0 - gated_kind_count * 2.0, 2)


def work_insight_kind_evaluation_rank_score(row: dict[str, Any], rank: int) -> float:
    score = safe_float(row.get("measurement_coverage_rate")) * 40.0 + int_value(row.get("current_insight_count")) * 2.0
    if bool(row.get("ready_for_product_action")):
        score += 40.0
    elif bool(row.get("ready_to_measure")):
        score += 25.0
    return round(max(0.0, score - rank * 0.01), 2)


def persist_work_program_brief_caveats_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    table_name = "work_program_brief_caveats"
    required = [table_name, "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before brief-caveat materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"brief caveats cannot link workstream {workstream_key}")

    facts = ontology_work_program_adversarial_facts(conn, source_instance, workstream_key, now)
    caveats = build_work_program_brief_caveats(readiness, forecast_summary, facts)
    run_prefix = f"{workstream_key}|{now}|"
    conn.execute(
        f"""
        delete from {table_name}
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_work_program_brief_caveat'
          and external_id like ?
        """,
        (source_instance, run_prefix + "%"),
    )
    for rank, caveat in enumerate(caveats):
        caveat_key = first_nonempty([caveat.get("key")]) or stable_digest([rank, caveat.get("title")])
        external_id = f"{run_prefix}{caveat_key}"
        severity = first_nonempty([caveat.get("severity")]) or "warning"
        values = {
            "key": f"work-program-brief-caveat:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "workstream_key": workstream_key,
            "generated_at": now,
            "caveat_key": caveat_key,
            "severity": severity,
            "title": first_nonempty([caveat.get("title")]) or "Brief caveat",
            "detail": first_nonempty([caveat.get("detail")]) or "This brief has a caveat that should remain visible to users.",
            "recommended_action": first_nonempty([caveat.get("recommended_action")]),
            "evidence_ref": first_nonempty([caveat.get("evidence_ref")]),
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_program_brief_caveat",
            "external_id": external_id,
            "source_url": workstream_source_url(workstream_key),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": "fresh",
            "visibility": "public",
            "confidence": brief_caveat_confidence(severity, first_nonempty([caveat.get("evidence_ref")])),
            "event_count": 1,
            "first_seen_at": now,
            "last_activity_at": now,
            "rank_score": brief_caveat_rank_score(severity, rank),
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, table_name, values, "key")
        caveat_id = int(conn.execute(f"select id from {table_name} where key = ?", (values["key"],)).fetchone()[0])
        excerpt = f"{values['title']}: {values['detail']} {values['recommended_action'] or ''}".strip()
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_brief_caveat",
            caveat_id,
            "severity",
            "tpm_brief_caveat",
            external_id,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (values["confidence"], evidence_id),
            )
            conn.execute(
                f"update {table_name} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, caveat_id),
            )
    conn.commit()


def persist_work_program_risk_drivers_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    generated_at: str,
) -> None:
    table_name = "work_program_risk_drivers"
    required = [table_name, "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before risk-driver materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"risk drivers cannot link workstream {workstream_key}")

    drivers = ontology_work_program_risk_drivers(conn, source_instance, workstream_key, now)
    run_prefix = f"{workstream_key}|{now}|"
    conn.execute(
        f"""
        delete from {table_name}
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_work_program_risk_driver'
          and external_id like ?
        """,
        (source_instance, run_prefix + "%"),
    )
    for rank, driver in enumerate(drivers):
        driver_key = first_nonempty([driver.get("key")]) or stable_digest([driver.get("driver_kind"), driver.get("subject_key"), rank])
        external_id = f"{run_prefix}{driver_key}"
        badge_keys, badge_labels, badge_tones, badge_details = risk_driver_badge_fields(driver)
        rank_score = safe_float(driver.get("rank_score"))
        values = {
            "key": f"work-program-risk-driver:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
            "workstream_id": workstream_id,
            "workstream_key": workstream_key,
            "generated_at": now,
            "driver_key": driver_key,
            "driver_kind": first_nonempty([driver.get("driver_kind")]) or "unknown",
            "subject_kind": first_nonempty([driver.get("subject_kind")]),
            "subject_key": first_nonempty([driver.get("subject_key")]),
            "title": first_nonempty([driver.get("title")]) or "Risk driver",
            "status": first_nonempty([driver.get("status")]) or "unknown",
            "recommended_action": first_nonempty([driver.get("recommended_action")]),
            "evidence_ref": first_nonempty([driver.get("evidence_ref")]),
            "badge_keys": badge_keys,
            "badge_labels": badge_labels,
            "badge_tones": badge_tones,
            "badge_details": badge_details,
            "source_system": "cubicle_analytics",
            "source_instance": source_instance,
            "external_kind": "tpm_work_program_risk_driver",
            "external_id": external_id,
            "source_url": first_nonempty([driver.get("source_url"), workstream_source_url(workstream_key)]),
            "latest_evidence_id": None,
            "evidence_count": 0,
            "freshness_state": first_nonempty([driver.get("freshness_state")]) or "fresh",
            "visibility": first_nonempty([driver.get("visibility")]) or "public",
            "confidence": risk_driver_confidence(driver),
            "event_count": int_value(driver.get("event_count")) or 1,
            "first_seen_at": first_nonempty([driver.get("first_seen_at")]) or now,
            "last_activity_at": first_nonempty([driver.get("last_activity_at")]) or now,
            "rank_score": rank_score,
            "created_at": now,
            "updated_at": now,
        }
        upsert_row(conn, table_name, values, "key")
        risk_driver_id = int(conn.execute(f"select id from {table_name} where key = ?", (values["key"],)).fetchone()[0])
        excerpt = f"{values['driver_kind']} {values['title']}: {values['status']}. {values['recommended_action'] or ''}".strip()
        evidence_id = upsert_generated_evidence(
            conn,
            source_instance,
            "work_program_risk_driver",
            risk_driver_id,
            "status",
            "tpm_risk_driver",
            external_id,
            excerpt,
            now,
        )
        if evidence_id is not None:
            conn.execute(
                "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
                (values["confidence"], evidence_id),
            )
            conn.execute(
                f"update {table_name} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
                (evidence_id, risk_driver_id),
            )
    conn.commit()


def persist_work_program_brief_snapshot_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    generated_at: str,
) -> None:
    table_name = "work_program_brief_snapshots"
    required = [table_name, "workstreams", "evidences"]
    missing = [table for table in required if not table_exists(conn, table)]
    if missing:
        raise RuntimeError(f"ontology DB is missing {', '.join(missing)}; rerun the Ent migration/load before brief-snapshot materialization")

    conn.execute("pragma foreign_keys = on")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    if workstream_id is None:
        raise RuntimeError(f"brief snapshot cannot link workstream {workstream_key}")

    facts = ontology_work_program_adversarial_facts(conn, source_instance, workstream_key, now)
    gates = build_work_program_quality_gates(readiness, forecast_summary, facts)
    caveats = build_work_program_brief_caveats(readiness, forecast_summary, facts)
    risk_drivers = ontology_work_program_risk_drivers(conn, source_instance, workstream_key, now)
    snapshot = build_work_program_brief_snapshot(readiness, forecast_summary, facts, gates, caveats, risk_drivers)
    external_id = f"{workstream_key}|{now}|brief_snapshot"
    values = {
        "key": f"work-program-brief-snapshot:cubicle-analytics:{source_instance}:{stable_digest([external_id])}",
        "workstream_id": workstream_id,
        "workstream_key": workstream_key,
        "generated_at": now,
        "operating_status": first_nonempty([snapshot.get("operating_status")]) or "unknown",
        "decision_pressure": first_nonempty([snapshot.get("decision_pressure")]) or "watch",
        "forecast_state": first_nonempty([snapshot.get("forecast_state")]) or "missing",
        "primary_risk": first_nonempty([snapshot.get("primary_risk")]),
        "executive_summary": first_nonempty([snapshot.get("executive_summary")]) or "No typed program items are in scope.",
        "recommended_focus": first_nonempty([snapshot.get("recommended_focus")]) or "Maintain watch on typed program items.",
        "next_cadence_focus": first_nonempty([snapshot.get("next_cadence_focus")]) or "Maintain watch and refresh the typed operating brief on the next source sync.",
        "capability_gaps": "\n".join(unique_strings(snapshot.get("capability_gaps", []))),
        "total_count": int_value(snapshot.get("total_count")),
        "product_action_count": int_value(snapshot.get("product_action_count")),
        "validation_lead_count": int_value(snapshot.get("validation_lead_count")),
        "source_coverage_limited_count": int_value(snapshot.get("source_coverage_limited_count")),
        "active_blocker_count": int_value(snapshot.get("active_blocker_count")),
        "active_blocker_impact_count": int_value(snapshot.get("active_blocker_impact_count")),
        "needs_action_dependency_count": int_value(snapshot.get("needs_action_dependency_count")),
        "overloaded_owner_count": int_value(snapshot.get("overloaded_owner_count")),
        "unassigned_action_count": int_value(snapshot.get("unassigned_action_count")),
        "quality_gate_count": len(gates),
        "blocking_gate_count": sum(1 for gate in gates if bool(gate.get("blocking"))),
        "caveat_count": len(caveats),
        "risk_driver_count": len(risk_drivers),
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_work_program_brief_snapshot",
        "external_id": external_id,
        "source_url": workstream_source_url(workstream_key),
        "latest_evidence_id": None,
        "evidence_count": 0,
        "freshness_state": "fresh",
        "visibility": "public",
        "confidence": brief_snapshot_confidence(snapshot, gates, caveats),
        "event_count": max(1, int_value(snapshot.get("total_count"))),
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": brief_snapshot_rank_score(snapshot, gates, caveats),
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, table_name, values, "key")
    snapshot_id = int(conn.execute(f"select id from {table_name} where key = ?", (values["key"],)).fetchone()[0])
    excerpt = f"{values['executive_summary']} {values['recommended_focus']} {values['next_cadence_focus']}".strip()
    evidence_id = upsert_generated_evidence(
        conn,
        source_instance,
        "work_program_brief_snapshot",
        snapshot_id,
        "operating_status",
        "tpm_brief_snapshot",
        external_id,
        excerpt,
        now,
    )
    if evidence_id is not None:
        conn.execute(
            "update evidences set freshness_state = 'fresh', confidence = ?, visibility = 'public' where id = ?",
            (values["confidence"], evidence_id),
        )
        conn.execute(
            f"update {table_name} set latest_evidence_id = ?, evidence_count = 1 where id = ?",
            (evidence_id, snapshot_id),
            )
    conn.commit()


WORK_PROGRAM_RUN_MEMBER_TABLES = [
    "work_program_automation_readinesses",
    "work_program_quality_gates",
    "work_program_evidence_needs",
    "work_program_adversarial_checks",
    "work_program_tpm_function_readinesses",
    "work_program_milestones",
    "work_program_summary_snapshots",
    "work_program_brief_snapshots",
    "work_program_brief_caveats",
    "work_program_risk_drivers",
    "work_program_owner_rollup_snapshots",
    "work_owner_load_snapshots",
]


def persist_work_program_run_to_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    generated_at: str,
) -> None:
    """Persist a durable run boundary over generated WorkProgram rows.

    The current POC materializes WorkProgram rows directly into SQLite tables
    without Ent edges between every snapshot table. This run table gives product
    validation and replay a stable run key plus membership list while Ent schema
    hardening catches up.
    """

    if not source_instance:
        return
    ensure_work_program_run_tables(conn)
    now = generated_at or datetime.now(timezone.utc).isoformat()
    workstream_key = "flink-kubernetes-operator"
    external_id = f"{workstream_key}|{now}|run"
    run_key = f"work-program-run:cubicle-analytics:{source_instance}:{stable_digest([external_id])}"
    readiness = work_program_run_readiness_row(conn, source_instance, workstream_key, now)
    member_counts = {
        table_name: len(work_program_run_member_rows(conn, table_name, source_instance, workstream_key, now))
        for table_name in WORK_PROGRAM_RUN_MEMBER_TABLES
        if table_exists(conn, table_name)
    }
    values = {
        "run_key": run_key,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "workstream_key": workstream_key,
        "generated_at": now,
        "external_id": external_id,
        "readiness_state": first_nonempty([readiness.get("readiness_state")]) or "unknown",
        "readiness_score": safe_float(readiness.get("readiness_score")),
        "autonomous_action_ready": bool(readiness.get("autonomous_action_ready")),
        "human_review_required": bool(readiness.get("human_review_required")) if readiness else True,
        "blocking_gate_count": int_value(readiness.get("blocking_gate_count")),
        "evidence_need_count": int_value(readiness.get("evidence_need_count")),
        "tpm_function_count": int_value(readiness.get("tpm_function_count")),
        "quality_gate_count": member_counts.get("work_program_quality_gates", 0),
        "adversarial_check_count": member_counts.get("work_program_adversarial_checks", 0),
        "owner_load_snapshot_count": member_counts.get("work_owner_load_snapshots", 0),
        "summary_snapshot_count": member_counts.get("work_program_summary_snapshots", 0),
        "brief_snapshot_count": member_counts.get("work_program_brief_snapshots", 0),
        "member_count": sum(member_counts.values()),
        "created_at": now,
        "updated_at": now,
    }
    upsert_row(conn, "work_program_runs", values, "run_key")
    run_id_row = conn.execute("select id from work_program_runs where run_key = ?", (run_key,)).fetchone()
    run_id = int(run_id_row[0]) if run_id_row is not None else None
    conn.execute("delete from work_program_run_members where run_key = ?", (run_key,))
    member_has_run_id = column_exists(conn, "work_program_run_members", "work_program_run_id")
    for table_name in WORK_PROGRAM_RUN_MEMBER_TABLES:
        for member in work_program_run_member_rows(conn, table_name, source_instance, workstream_key, now):
            insert_columns = [
                "run_key",
                "member_table",
                "member_id",
                "member_key",
                "member_external_kind",
                "member_external_id",
                "member_rank_score",
                "created_at",
            ]
            insert_values: list[Any] = [
                run_key,
                table_name,
                member["id"],
                member["key"],
                member["external_kind"],
                member["external_id"],
                member["rank_score"],
                now,
            ]
            update_assignments = [
                "member_key = excluded.member_key",
                "member_external_kind = excluded.member_external_kind",
                "member_external_id = excluded.member_external_id",
                "member_rank_score = excluded.member_rank_score",
                "created_at = excluded.created_at",
            ]
            if member_has_run_id:
                insert_columns.insert(0, "work_program_run_id")
                insert_values.insert(0, run_id)
                update_assignments.insert(0, "work_program_run_id = excluded.work_program_run_id")
            conn.execute(
                f"""
                insert into work_program_run_members (
                  {", ".join(insert_columns)}
                ) values ({", ".join("?" for _ in insert_columns)})
                on conflict(run_key, member_table, member_id) do update set
                  {", ".join(update_assignments)}
                """,
                insert_values,
            )
    conn.commit()


def ensure_work_program_run_tables(conn: sqlite3.Connection) -> None:
    if table_exists(conn, "work_program_runs") and table_exists(conn, "work_program_run_members"):
        ensure_work_program_run_compat_columns(conn)
        return
    conn.execute(
        """
        create table if not exists work_program_runs (
          id integer primary key autoincrement,
          run_key text not null unique,
          source_system text not null,
          source_instance text not null,
          workstream_key text not null,
          generated_at text not null,
          external_id text not null,
          readiness_state text not null,
          readiness_score real not null default 0,
          autonomous_action_ready integer not null default 0,
          human_review_required integer not null default 1,
          blocking_gate_count integer not null default 0,
          evidence_need_count integer not null default 0,
          tpm_function_count integer not null default 0,
          quality_gate_count integer not null default 0,
          adversarial_check_count integer not null default 0,
          owner_load_snapshot_count integer not null default 0,
          summary_snapshot_count integer not null default 0,
          brief_snapshot_count integer not null default 0,
          member_count integer not null default 0,
          created_at text not null,
          updated_at text not null
        )
        """
    )
    conn.execute(
        """
        create table if not exists work_program_run_members (
          id integer primary key autoincrement,
          work_program_run_id integer,
          run_key text not null,
          member_table text not null,
          member_id integer not null,
          member_key text,
          member_external_kind text,
          member_external_id text,
          member_rank_score real,
          created_at text not null,
          unique(run_key, member_table, member_id)
        )
        """
    )
    conn.execute("create index if not exists work_program_runs_source_generated_idx on work_program_runs(source_instance, workstream_key, generated_at)")
    conn.execute("create index if not exists work_program_run_members_run_idx on work_program_run_members(run_key, member_table)")
    conn.execute("create index if not exists work_program_run_members_run_id_idx on work_program_run_members(work_program_run_id, member_table)")


def ensure_work_program_run_compat_columns(conn: sqlite3.Connection) -> None:
    if not column_exists(conn, "work_program_run_members", "work_program_run_id"):
        conn.execute("alter table work_program_run_members add column work_program_run_id integer")
    conn.execute("create index if not exists work_program_run_members_run_id_idx on work_program_run_members(work_program_run_id, member_table)")


def work_program_run_readiness_row(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
    generated_at: str,
) -> dict[str, Any]:
    table_name = "work_program_automation_readinesses"
    if not table_exists(conn, table_name):
        return {}
    row = conn.execute(
        f"""
        select readiness_state, readiness_score, autonomous_action_ready,
               human_review_required, blocking_gate_count, evidence_need_count,
               tpm_function_count
          from {table_name}
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and workstream_key in ({", ".join(["?"] * len(work_program_workstream_sql_keys(workstream_key)))})
           and generated_at = ?
         order by rank_score desc, id desc
         limit 1
        """,
        (source_instance, *work_program_workstream_sql_keys(workstream_key), generated_at),
    ).fetchone()
    if row is None:
        return {}
    columns = [
        "readiness_state",
        "readiness_score",
        "autonomous_action_ready",
        "human_review_required",
        "blocking_gate_count",
        "evidence_need_count",
        "tpm_function_count",
    ]
    return dict(zip(columns, row))


def work_program_run_member_rows(
    conn: sqlite3.Connection,
    table_name: str,
    source_instance: str,
    workstream_key: str,
    generated_at: str,
) -> list[dict[str, Any]]:
    if not table_exists(conn, table_name):
        return []
    columns = table_columns(conn, table_name)
    required = {"id", "source_instance", "generated_at"}
    if not required.issubset(columns):
        return []
    select_fields = [
        "id",
        "key" if "key" in columns else "'' as key",
        "external_kind" if "external_kind" in columns else "'' as external_kind",
        "external_id" if "external_id" in columns else "'' as external_id",
        "rank_score" if "rank_score" in columns else "0 as rank_score",
    ]
    predicates = ["source_instance = ?", "generated_at = ?"]
    params: list[Any] = [source_instance, generated_at]
    if "source_system" in columns:
        predicates.append("source_system = 'cubicle_analytics'")
    if "workstream_key" in columns:
        keys = work_program_workstream_sql_keys(workstream_key)
        predicates.append(f"workstream_key in ({', '.join(['?'] * len(keys))})")
        params.extend(keys)
    rows = conn.execute(
        f"""
        select {", ".join(select_fields)}
          from {table_name}
         where {" and ".join(predicates)}
         order by rank_score desc, id
        """,
        params,
    ).fetchall()
    out: list[dict[str, Any]] = []
    for row in rows:
        out.append(
            {
                "id": int(row[0]),
                "key": first_nonempty([row[1]]),
                "external_kind": first_nonempty([row[2]]),
                "external_id": first_nonempty([row[3]]),
                "rank_score": safe_float(row[4]),
            }
        )
    return out


def ontology_work_program_risk_drivers(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
    generated_at: str,
) -> list[dict[str, Any]]:
    drivers: list[dict[str, Any]] = []
    drivers.extend(ontology_blocker_risk_drivers(conn, source_instance))
    drivers.extend(ontology_blocker_impact_risk_drivers(conn, source_instance, workstream_key))
    drivers.extend(ontology_dependency_risk_drivers(conn, source_instance, workstream_key))
    drivers.extend(ontology_forecast_risk_drivers(conn, source_instance, generated_at))
    drivers.extend(ontology_owner_load_risk_drivers(conn, source_instance, workstream_key, generated_at))
    drivers.sort(
        key=lambda driver: (
            -safe_float(driver.get("rank_score")),
            first_nonempty([driver.get("driver_kind")]),
            first_nonempty([driver.get("key")]),
        )
    )
    return drivers


def ontology_blocker_risk_drivers(conn: sqlite3.Connection, source_instance: str) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_blockers"):
        return []
    rows = conn.execute(
        """
        select
          wb.key,
          wb.blocker_state,
          wb.subject_kind,
          wb.subject_key,
          wb.title,
          wb.recommended_action,
          wb.source_url,
          wb.evidence_count,
          wb.freshness_state,
          wb.visibility,
          wb.confidence,
          wb.event_count,
          wb.first_seen_at,
          wb.last_activity_at,
          wb.rank_score,
          e.locator_kind,
          e.locator,
          e.source_url,
          e.source_span_key,
          e.external_id,
          e.key
        from work_blockers wb
        left join evidences e on e.id = wb.latest_evidence_id
        where wb.source_system = 'cubicle_analytics'
          and wb.source_instance = ?
          and wb.external_kind = 'tpm_work_blocker'
          and wb.blocker_state in ('active', 'validating')
        order by wb.rank_score desc, wb.last_activity_at desc, wb.updated_at desc
        limit 10
        """,
        (source_instance,),
    ).fetchall()
    drivers = []
    for row in rows:
        (
            key,
            blocker_state,
            subject_kind,
            subject_key,
            title,
            recommended_action,
            source_url,
            evidence_count,
            freshness_state,
            visibility,
            confidence,
            event_count,
            first_seen_at,
            last_activity_at,
            rank_score,
            locator_kind,
            locator,
            evidence_source_url,
            source_span_key,
            evidence_external_id,
            evidence_key,
        ) = row
        drivers.append(
            risk_driver(
                key,
                "blocker",
                subject_kind,
                subject_key,
                title,
                blocker_state,
                recommended_action,
                rank_score,
                evidence_ref_from_parts(locator_kind, locator, evidence_source_url, source_span_key, evidence_external_id, evidence_key),
                source_url,
                freshness_state,
                visibility,
                confidence,
                evidence_count,
                event_count,
                first_seen_at,
                last_activity_at,
            )
        )
    return drivers


def ontology_blocker_impact_risk_drivers(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_blocker_impacts"):
        return []
    keys = workstream_filter_keys(workstream_key)
    placeholders = ",".join("?" for _ in keys)
    rows = conn.execute(
        f"""
        select
          wbi.key,
          wbi.impact_state,
          wbi.affected_kind,
          wbi.affected_key,
          wbi.title,
          wbi.recommended_action,
          wbi.source_url,
          wbi.evidence_count,
          wbi.freshness_state,
          wbi.visibility,
          wbi.confidence,
          wbi.event_count,
          wbi.first_seen_at,
          wbi.last_activity_at,
          wbi.impact_score,
          e.locator_kind,
          e.locator,
          e.source_url,
          e.source_span_key,
          e.external_id,
          e.key
        from work_blocker_impacts wbi
        left join evidences e on e.id = wbi.latest_evidence_id
        where wbi.source_system = 'cubicle_analytics'
          and wbi.source_instance = ?
          and wbi.external_kind = 'tpm_work_blocker_impact'
          and wbi.impact_state in ('active', 'validating')
          and wbi.affected_kind = 'workstream'
          and wbi.affected_key in ({placeholders})
        order by wbi.impact_score desc, wbi.rank_score desc, wbi.last_activity_at desc
        limit 10
        """,
        [source_instance, *keys],
    ).fetchall()
    drivers = []
    for row in rows:
        (
            key,
            impact_state,
            affected_kind,
            affected_key,
            title,
            recommended_action,
            source_url,
            evidence_count,
            freshness_state,
            visibility,
            confidence,
            event_count,
            first_seen_at,
            last_activity_at,
            impact_score,
            locator_kind,
            locator,
            evidence_source_url,
            source_span_key,
            evidence_external_id,
            evidence_key,
        ) = row
        drivers.append(
            risk_driver(
                key,
                "blocker_impact",
                affected_kind,
                affected_key,
                title,
                impact_state,
                recommended_action,
                impact_score,
                evidence_ref_from_parts(locator_kind, locator, evidence_source_url, source_span_key, evidence_external_id, evidence_key),
                source_url,
                freshness_state,
                visibility,
                confidence,
                evidence_count,
                event_count,
                first_seen_at,
                last_activity_at,
            )
        )
    return drivers


def ontology_dependency_risk_drivers(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_dependency_edges"):
        return []
    keys = workstream_filter_keys(workstream_key)
    placeholders = ",".join("?" for _ in keys)
    workstream_ids = ontology_workstream_ids_by_key(conn, source_instance)
    workstream_id = workstream_ids.get(f"workstream:{workstream_key}")
    rows = conn.execute(
        f"""
        select
          wde.key,
          wde.edge_kind,
          wde.from_kind,
          wde.from_key,
          wde.to_kind,
          wde.to_key,
          wde.source_url,
          wde.evidence_count,
          wde.freshness_state,
          wde.visibility,
          wde.confidence,
          wde.event_count,
          wde.first_seen_at,
          wde.last_activity_at,
          wde.rank_score,
          e.locator_kind,
          e.locator,
          e.source_url,
          e.source_span_key,
          e.external_id,
          e.key
        from work_dependency_edges wde
        left join evidences e on e.id = wde.latest_evidence_id
        where wde.source_system = 'cubicle_analytics'
          and wde.source_instance = ?
          and wde.external_kind = 'tpm_work_dependency_edge'
          and wde.edge_kind in ('blocked_by', 'needs_action')
          and (
            wde.workstream_id = ?
            or wde.from_key in ({placeholders})
            or wde.to_key in ({placeholders})
          )
        order by wde.rank_score desc, wde.last_activity_at desc, wde.updated_at desc
        limit 10
        """,
        [source_instance, workstream_id, *keys, *keys],
    ).fetchall()
    drivers = []
    for row in rows:
        (
            key,
            edge_kind,
            from_kind,
            from_key,
            to_kind,
            to_key,
            source_url,
            evidence_count,
            freshness_state,
            visibility,
            confidence,
            event_count,
            first_seen_at,
            last_activity_at,
            rank_score,
            locator_kind,
            locator,
            evidence_source_url,
            source_span_key,
            evidence_external_id,
            evidence_key,
        ) = row
        title = dependency_driver_title(edge_kind)
        recommended_action = dependency_driver_action(edge_kind, to_kind, to_key)
        drivers.append(
            risk_driver(
                key,
                "dependency",
                from_kind,
                from_key,
                title,
                edge_kind,
                recommended_action,
                rank_score,
                evidence_ref_from_parts(locator_kind, locator, evidence_source_url, source_span_key, evidence_external_id, evidence_key),
                source_url,
                freshness_state,
                visibility,
                confidence,
                evidence_count,
                event_count,
                first_seen_at,
                last_activity_at,
            )
        )
    return drivers


def ontology_forecast_risk_drivers(
    conn: sqlite3.Connection,
    source_instance: str,
    generated_at: str,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_item_forecasts"):
        return []
    rows = conn.execute(
        """
        select
          wif.key,
          wif.subject_kind,
          wif.subject_key,
          wif.subject_state,
          wif.risk_score,
          wif.risk_band,
          wif.readiness_state,
          wif.ready_for_eta,
          wif.overdue_days,
          wif.readiness_reason,
          wif.source_url,
          wif.evidence_count,
          wif.freshness_state,
          wif.visibility,
          wif.confidence,
          wif.event_count,
          wif.first_seen_at,
          wif.last_activity_at,
          e.locator_kind,
          e.locator,
          e.source_url,
          e.source_span_key,
          e.external_id,
          e.key
        from work_item_forecasts wif
        left join evidences e on e.id = wif.latest_evidence_id
        where wif.source_system = 'cubicle_analytics'
          and wif.source_instance = ?
          and wif.external_kind in ('tpm_pr_forecast', 'tpm_work_item_forecast')
          and wif.subject_state = 'open'
          and wif.risk_band in ('critical', 'high')
        order by (coalesce(wif.risk_score, 0) + min(coalesce(wif.overdue_days, 0), 100)) desc, wif.last_activity_at desc
        limit 10
        """,
        (source_instance,),
    ).fetchall()
    drivers = []
    for row in rows:
        (
            key,
            subject_kind,
            subject_key,
            _subject_state,
            risk_score,
            risk_band,
            readiness_state,
            ready_for_eta,
            overdue_days,
            readiness_reason,
            source_url,
            evidence_count,
            freshness_state,
            visibility,
            confidence,
            event_count,
            first_seen_at,
            last_activity_at,
            locator_kind,
            locator,
            evidence_source_url,
            source_span_key,
            evidence_external_id,
            evidence_key,
        ) = row
        actionability_state = forecast_actionability_state(bool(ready_for_eta), first_nonempty([risk_band]), optional_float(overdue_days))
        drivers.append(
            risk_driver(
                key,
                "forecast_risk",
                subject_kind,
                subject_key,
                forecast_driver_title(subject_key),
                actionability_state,
                forecast_driver_action(actionability_state),
                safe_float(risk_score) + min(max(0.0, safe_float(overdue_days)), 100.0),
                evidence_ref_from_parts(locator_kind, locator, evidence_source_url, source_span_key, evidence_external_id, evidence_key),
                source_url,
                freshness_state,
                visibility,
                confidence,
                evidence_count,
                event_count,
                first_seen_at or generated_at,
                last_activity_at or generated_at,
                {"risk_band": risk_band, "readiness_state": readiness_state, "readiness_reason": readiness_reason},
            )
        )
    return drivers


def ontology_owner_load_risk_drivers(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
    generated_at: str,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_owner_load_snapshots"):
        return []
    keys = workstream_filter_keys(workstream_key)
    latest_generated_at = generated_at
    rows = owner_load_risk_driver_rows(conn, source_instance, keys, latest_generated_at)
    if not rows:
        placeholders = ",".join("?" for _ in keys)
        latest = conn.execute(
            f"""
            select max(generated_at)
            from work_owner_load_snapshots
            where source_system = 'cubicle_analytics'
              and source_instance = ?
              and external_kind = 'tpm_owner_load_snapshot'
              and workstream_key in ({placeholders})
            """,
            [source_instance, *keys],
        ).fetchone()
        latest_generated_at = latest[0] if latest else ""
        if latest_generated_at:
            rows = owner_load_risk_driver_rows(conn, source_instance, keys, latest_generated_at)
    drivers = []
    for row in rows:
        (
            key,
            owner_key,
            owner_display_name,
            load_status,
            action_count,
            max_priority_score,
            recommended_focus,
            source_url,
            evidence_count,
            freshness_state,
            visibility,
            confidence,
            event_count,
            first_seen_at,
            last_activity_at,
            locator_kind,
            locator,
            evidence_source_url,
            source_span_key,
            evidence_external_id,
            evidence_key,
        ) = row
        if not owner_load_driver_applies(owner_key, load_status, int_value(action_count)):
            continue
        drivers.append(
            risk_driver(
                key,
                "owner_load",
                "owner",
                owner_key,
                owner_load_driver_title(owner_key, owner_display_name),
                load_status,
                owner_load_driver_action(owner_key, recommended_focus),
                owner_load_driver_rank_score(load_status, owner_key, action_count, max_priority_score),
                evidence_ref_from_parts(locator_kind, locator, evidence_source_url, source_span_key, evidence_external_id, evidence_key),
                source_url,
                freshness_state,
                visibility,
                confidence,
                evidence_count,
                event_count,
                first_seen_at or latest_generated_at,
                last_activity_at or latest_generated_at,
            )
        )
    drivers.sort(key=lambda driver: -safe_float(driver.get("rank_score")))
    return drivers[:10]


def owner_load_risk_driver_rows(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_keys: list[str],
    generated_at: str,
) -> list[sqlite3.Row]:
    if not generated_at:
        return []
    placeholders = ",".join("?" for _ in workstream_keys)
    return conn.execute(
        f"""
        select
          ol.key,
          ol.owner_key,
          ol.owner_display_name,
          ol.load_status,
          ol.action_count,
          ol.max_priority_score,
          ol.recommended_focus,
          ol.source_url,
          ol.evidence_count,
          ol.freshness_state,
          ol.visibility,
          ol.confidence,
          ol.event_count,
          ol.first_seen_at,
          ol.last_activity_at,
          e.locator_kind,
          e.locator,
          e.source_url,
          e.source_span_key,
          e.external_id,
          e.key
        from work_owner_load_snapshots ol
        left join evidences e on e.id = ol.latest_evidence_id
        where ol.source_system = 'cubicle_analytics'
          and ol.source_instance = ?
          and ol.external_kind = 'tpm_owner_load_snapshot'
          and ol.workstream_key in ({placeholders})
          and ol.generated_at = ?
        order by ol.rank_score desc, ol.action_count desc, ol.owner_key
        limit 25
        """,
        [source_instance, *workstream_keys, generated_at],
    ).fetchall()


def risk_driver(
    key: Any,
    driver_kind: str,
    subject_kind: Any,
    subject_key: Any,
    title: Any,
    status: Any,
    recommended_action: Any,
    rank_score: Any,
    evidence_ref: str,
    source_url: Any,
    freshness_state: Any,
    visibility: Any,
    confidence: Any,
    evidence_count: Any,
    event_count: Any,
    first_seen_at: Any,
    last_activity_at: Any,
    extra: dict[str, Any] | None = None,
) -> dict[str, Any]:
    row = {
        "key": first_nonempty([key]),
        "driver_kind": driver_kind,
        "subject_kind": first_nonempty([subject_kind]),
        "subject_key": first_nonempty([subject_key]),
        "title": first_nonempty([title]) or risk_driver_kind_label(driver_kind),
        "status": first_nonempty([status]) or "unknown",
        "recommended_action": first_nonempty([recommended_action]),
        "rank_score": safe_float(rank_score),
        "evidence_ref": evidence_ref,
        "source_url": first_nonempty([source_url]),
        "freshness_state": first_nonempty([freshness_state]) or "fresh",
        "visibility": first_nonempty([visibility]) or "public",
        "confidence": safe_float(confidence),
        "evidence_count": int_value(evidence_count),
        "event_count": int_value(event_count) or 1,
        "first_seen_at": first_nonempty([first_seen_at]),
        "last_activity_at": first_nonempty([last_activity_at]),
    }
    if extra:
        row.update(extra)
    return row


def workstream_filter_keys(workstream_key: str) -> list[str]:
    key = first_nonempty([workstream_key]) or "flink-kubernetes-operator"
    keys = [key]
    if key.startswith("workstream:"):
        keys.append(key.removeprefix("workstream:"))
    else:
        keys.append(f"workstream:{key}")
    return unique_strings(keys)


def dependency_driver_title(edge_kind: Any) -> str:
    value = first_nonempty([edge_kind])
    if value == "blocked_by":
        return "Blocked by dependency"
    if value == "needs_action":
        return "Dependency needs action"
    return value.replace("_", " ").title() if value else "Dependency"


def dependency_driver_action(edge_kind: Any, to_kind: Any, to_key: Any) -> str:
    value = first_nonempty([edge_kind])
    target = first_nonempty([to_key])
    if value == "blocked_by":
        return "Clear or validate the linked blocker before treating the dependent work as on track."
    if value == "needs_action":
        if target:
            return f"Drive the linked action to completion: {target}."
        return "Drive the linked action to completion."
    if first_nonempty([to_kind]) or target:
        return f"Review dependency target {first_nonempty([to_kind])}:{target}."
    return "Review this dependency before treating the workstream plan as clear."


def forecast_actionability_state(ready_for_eta: bool, risk_band: str, overdue_days: float | None) -> str:
    if ready_for_eta:
        return "eta_commitment_ready"
    if risk_band == "critical":
        return "owner_status_needed"
    if risk_band == "high":
        return "risk_triage"
    if overdue_days is not None and overdue_days > 0:
        return "risk_triage"
    return "watch"


def forecast_driver_title(subject_key: Any) -> str:
    key = first_nonempty([subject_key])
    if key:
        return f"Forecast risk: {key}"
    return "Forecast risk"


def forecast_driver_action(actionability_state: str) -> str:
    if actionability_state == "eta_commitment_ready":
        return "Use this forecast as an ETA candidate after confirming the owner plan and latest source state."
    if actionability_state == "owner_status_needed":
        return "Treat this as a TPM risk lead: ask the owner for merge, close, or parking status, but do not present it as an ETA commitment."
    if actionability_state == "risk_triage":
        return "Review this forecast with the owner or reviewer as risk triage until ETA readiness gates clear."
    return "Keep this item on forecast watch; no ETA commitment or owner escalation is supported by the current forecast evidence."


def owner_load_driver_applies(owner_key: Any, load_status: Any, action_count: int) -> bool:
    status = first_nonempty([load_status])
    owner = first_nonempty([owner_key])
    if status in {"overloaded", "attention_required"}:
        return True
    return owner == "(unassigned)" and action_count > 0


def owner_load_driver_title(owner_key: Any, owner_display_name: Any) -> str:
    owner = first_nonempty([owner_key])
    display = first_nonempty([owner_display_name])
    if owner == "(unassigned)":
        return "Owner load: unassigned product actions"
    if display:
        return f"Owner load: {display}"
    if owner:
        return f"Owner load: {owner}"
    return "Owner load"


def owner_load_driver_action(owner_key: Any, recommended_focus: Any) -> str:
    focus = first_nonempty([recommended_focus])
    if focus:
        return focus
    if first_nonempty([owner_key]) == "(unassigned)":
        return "Assign unowned product actions before treating the workstream plan as executable."
    return "Rebalance overloaded owner queues or explicitly accept the owner concentration."


def owner_load_driver_rank_score(load_status: Any, owner_key: Any, action_count: Any, max_priority_score: Any) -> float:
    score = safe_float(max_priority_score) + float(int_value(action_count) * 5)
    status = first_nonempty([load_status])
    if status == "overloaded":
        score += 15.0
    elif status == "attention_required":
        score += 8.0
    if first_nonempty([owner_key]) == "(unassigned)":
        score += 5.0
    return score


def risk_driver_badge_fields(driver: dict[str, Any]) -> tuple[str, str, str, str]:
    driver_kind = first_nonempty([driver.get("driver_kind")]) or "unknown"
    status = first_nonempty([driver.get("status")]) or "unknown"
    keys = [f"risk_driver:kind:{driver_kind}", f"risk_driver:status:{status}"]
    labels = [risk_driver_kind_label(driver_kind), risk_driver_status_label(status)]
    tones = [risk_driver_kind_tone(driver_kind), risk_driver_status_tone(status)]
    details = [f"driver kind {driver_kind}", f"current status {status}"]
    risk_band = first_nonempty([driver.get("risk_band")])
    if risk_band:
        keys.append(f"forecast:risk:{risk_band}")
        labels.append(risk_driver_status_label(risk_band))
        tones.append(risk_driver_status_tone(risk_band))
        details.append(f"forecast risk band {risk_band}")
    readiness_state = first_nonempty([driver.get("readiness_state")])
    if readiness_state:
        keys.append(f"forecast:readiness:{readiness_state}")
        labels.append("ETA " + risk_driver_status_label(readiness_state).lower())
        tones.append("success" if readiness_state == "ready" else "warning")
        details.append(f"forecast readiness {readiness_state}")
    return "\n".join(keys), "\n".join(labels), "\n".join(tones), "\n".join(details)


def risk_driver_kind_label(driver_kind: str) -> str:
    return {
        "blocker": "Blocker",
        "blocker_impact": "Blocker impact",
        "dependency": "Dependency",
        "forecast_risk": "Forecast risk",
        "owner_load": "Owner load",
    }.get(driver_kind, driver_kind.replace("_", " ").title() if driver_kind else "Risk")


def risk_driver_kind_tone(driver_kind: str) -> str:
    if driver_kind in {"blocker", "blocker_impact"}:
        return "danger"
    if driver_kind in {"dependency", "owner_load", "forecast_risk"}:
        return "warning"
    return "info"


def risk_driver_status_label(status: str) -> str:
    if not status:
        return "Unknown"
    return status.replace("_", " ").title()


def risk_driver_status_tone(status: str) -> str:
    if status in {"active", "overloaded", "critical", "blocked_by", "owner_status_needed"}:
        return "danger"
    if status in {"validating", "attention_required", "high", "needs_action", "risk_triage", "gated", "insufficient_sample"}:
        return "warning"
    if status in {"ready", "eta_commitment_ready", "resolved", "clear"}:
        return "success"
    if status in {"watch", "unknown"}:
        return "neutral"
    return "info"


def risk_driver_confidence(driver: dict[str, Any]) -> float:
    confidence = safe_float(driver.get("confidence"))
    if confidence > 0:
        return confidence
    if first_nonempty([driver.get("evidence_ref")]):
        return 0.9
    return 0.8


def ontology_work_program_adversarial_facts(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
    generated_at: str,
) -> dict[str, Any]:
    keys = [workstream_key, f"workstream:{workstream_key}"]
    program_rows = ontology_program_item_rows(conn, source_instance, keys)
    owner_rows = ontology_owner_load_rows(conn, source_instance, keys, generated_at)
    program_status_counts: dict[str, int] = {}
    decision_state_counts: dict[str, int] = {}
    source_coverage_limit_counts: dict[str, int] = {}
    auth_limited_observation_counts: dict[str, int] = {}
    auth_limited_product_decision_counts: dict[str, int] = {}
    generated_claim_limit_counts: dict[str, int] = {}
    generated_claim_product_decision_counts: dict[str, int] = {}
    owner_load_status_counts: dict[str, int] = {}
    product_action_insight_kinds: set[str] = set()
    validation_context_insight_kinds: set[str] = set()
    for row in program_rows:
        decision_state = first_nonempty([row.get("decision_state")]) or "unknown"
        program_status_counts[first_nonempty([row.get("program_status")]) or "unknown"] = program_status_counts.get(first_nonempty([row.get("program_status")]) or "unknown", 0) + 1
        decision_state_counts[decision_state] = decision_state_counts.get(decision_state, 0) + 1
        action_kinds = split_csv(first_nonempty([row.get("action_source_link_insight_kinds")]))
        if decision_state == "product_action":
            for kind in action_kinds:
                if is_product_action_measurement_kind(kind):
                    product_action_insight_kinds.add(kind)
        else:
            for kind in action_kinds:
                if insight_measurement_scope(kind) in {"context_only", "validation_lead"}:
                    validation_context_insight_kinds.add(kind)
        if ontology_program_item_coverage_limited(row):
            limit_kind = ontology_program_item_coverage_limit_kind(row)
            source_coverage_limit_counts[limit_kind] = source_coverage_limit_counts.get(limit_kind, 0) + 1
        if ontology_program_item_auth_limited(row):
            limit_kind = ontology_program_item_coverage_limit_kind(row)
            auth_limited_observation_counts[limit_kind] = auth_limited_observation_counts.get(limit_kind, 0) + 1
            if ontology_program_item_product_decision_open(row):
                auth_limited_product_decision_counts[limit_kind] = auth_limited_product_decision_counts.get(limit_kind, 0) + 1
        if ontology_program_item_generated_claim_limited(row):
            limit_kind = ontology_program_item_coverage_limit_kind(row)
            generated_claim_limit_counts[limit_kind] = generated_claim_limit_counts.get(limit_kind, 0) + 1
            if ontology_program_item_product_decision_open(row):
                generated_claim_product_decision_counts[limit_kind] = generated_claim_product_decision_counts.get(limit_kind, 0) + 1
    for row in owner_rows:
        status = first_nonempty([row.get("load_status")]) or "clear"
        owner_load_status_counts[status] = owner_load_status_counts.get(status, 0) + 1
    product_action_insight_kinds.update(
        ontology_work_action_insight_kinds(
            conn,
            source_instance,
            "wa.decision_state = 'product_action' and wa.action_state = 'open'",
        )
    )
    validation_context_insight_kinds.update(
        kind
        for kind in ontology_work_action_insight_kinds(
            conn,
            source_instance,
            "wa.decision_state != 'product_action'",
        )
        if insight_measurement_scope(kind) in {"context_only", "validation_lead"}
    )
    active_blocker_count = ontology_count(
        conn,
        "work_blockers",
        source_instance,
        "and blocker_state = 'active'",
    )
    active_blocker_impact_count = ontology_count(
        conn,
        "work_blocker_impacts",
        source_instance,
        "and impact_state = 'active'",
    )
    return {
        "total_count": len(program_rows),
        "program_status_counts": program_status_counts,
        "decision_state_counts": decision_state_counts,
        "product_action_insight_kinds": sorted(product_action_insight_kinds),
        "validation_context_insight_kinds": sorted(validation_context_insight_kinds),
        "source_coverage_limit_counts": source_coverage_limit_counts,
        "auth_limited_observation_counts": auth_limited_observation_counts,
        "auth_limited_product_decision_counts": auth_limited_product_decision_counts,
        "generated_claim_limit_counts": generated_claim_limit_counts,
        "generated_claim_product_decision_counts": generated_claim_product_decision_counts,
        "owner_load_status_counts": owner_load_status_counts,
        "standup_section_count": ontology_count(
            conn,
            "workstream_standup_sections",
            source_instance,
            f"and workstream_key in ({','.join('?' for _ in keys)}) and generated_at = ?",
            [*keys, generated_at],
        ),
        "source_coverage_limited_count": sum(1 for row in program_rows if ontology_program_item_coverage_limited(row)),
        "auth_limited_observation_count": sum(1 for row in program_rows if ontology_program_item_auth_limited(row)),
        "auth_limited_product_decision_count": sum(
            1
            for row in program_rows
            if ontology_program_item_auth_limited(row) and ontology_program_item_product_decision_open(row)
        ),
        "generated_claim_limited_count": sum(1 for row in program_rows if ontology_program_item_generated_claim_limited(row)),
        "generated_claim_product_decision_count": sum(
            1
            for row in program_rows
            if ontology_program_item_generated_claim_limited(row) and ontology_program_item_product_decision_open(row)
        ),
        "product_action_count": sum(1 for row in program_rows if first_nonempty([row.get("decision_state")]) == "product_action"),
        "needs_decision_count": sum(1 for row in program_rows if first_nonempty([row.get("program_status")]) == "needs_decision"),
        "now_count": sum(1 for row in program_rows if first_nonempty([row.get("due_bucket")]) == "now"),
        "high_risk_count": sum(1 for row in program_rows if safe_float(row.get("risk_score")) >= 90),
        "unassigned_count": sum(1 for row in program_rows if first_nonempty([row.get("owner_key")]) in {"", "(unassigned)", "unassigned"}),
        "overloaded_owner_count": sum(1 for row in owner_rows if first_nonempty([row.get("load_status")]) == "overloaded"),
        "attention_owner_count": sum(1 for row in owner_rows if first_nonempty([row.get("load_status")]) == "attention_required"),
        "unassigned_action_count": sum(metric_row_int(pd.Series(row), "product_action_count") for row in owner_rows if first_nonempty([row.get("owner_key")]) == "(unassigned)"),
        "unassigned_total_action_count": sum(metric_row_int(pd.Series(row), "action_count") for row in owner_rows if first_nonempty([row.get("owner_key")]) == "(unassigned)"),
        "owner_load_row_count": len(owner_rows),
        "owner_load_action_count": sum(metric_row_int(pd.Series(row), "action_count") for row in owner_rows),
        "owner_load_status": owner_load_status_from_rows(owner_rows),
        "owner_load_targets": ontology_owner_load_evidence_targets(conn, source_instance, keys, generated_at),
        "forecast_risk_target_count": ontology_count(
            conn,
            "work_item_forecasts",
            source_instance,
            "and subject_state = 'open' and risk_band in ('critical', 'high')",
        ),
        "forecast_risk_targets": ontology_forecast_risk_targets(conn, source_instance),
        "measurement_label_targets": ontology_measurement_label_targets(conn, source_instance),
        "source_coverage_targets": [
            row
            for row in program_rows
            if ontology_program_item_coverage_limited(row) and first_nonempty([row.get("subject_key")])
        ],
        "auth_limited_observation_targets": [
            row
            for row in program_rows
            if ontology_program_item_auth_limited(row) and first_nonempty([row.get("subject_key")])
        ],
        "generated_claim_limited_targets": [
            row
            for row in program_rows
            if ontology_program_item_generated_claim_limited(row) and first_nonempty([row.get("subject_key")])
        ],
        "product_decision_targets": [
            row
            for row in program_rows
            if ontology_program_item_product_decision_open(row) and first_nonempty([row.get("subject_key")])
        ],
        "blocker_count": ontology_count(conn, "work_blockers", source_instance),
        "active_blocker_count": active_blocker_count,
        "validating_blocker_count": ontology_count(
            conn,
            "work_blockers",
            source_instance,
            "and blocker_state = 'validating'",
        ),
        "blocker_impact_count": ontology_count(conn, "work_blocker_impacts", source_instance),
        "active_blocker_impact_count": active_blocker_impact_count,
        "active_blocker_targets": ontology_active_blocker_clearance_targets(conn, source_instance),
        "dependency_edge_count": ontology_count(conn, "work_dependency_edges", source_instance),
        "blocking_dependency_count": ontology_count(
            conn,
            "work_dependency_edges",
            source_instance,
            "and edge_kind = 'blocked_by'",
        ),
        "needs_action_dependency_count": ontology_count(
            conn,
            "work_dependency_edges",
            source_instance,
            "and edge_kind = 'needs_action'",
        ),
        "dependency_action_targets": ontology_dependency_action_targets(conn, source_instance),
        "program_item_evidence_refs": ontology_evidence_refs(
            conn,
            "work_program_items",
            source_instance,
            f"and workstream_key in ({','.join('?' for _ in keys)})",
            keys,
        ),
        "forecast_evidence_refs": unique_strings(
            ontology_evidence_refs(conn, "work_forecast_evaluations", source_instance)
            + ontology_evidence_refs(conn, "work_item_forecasts", source_instance)
        ),
        "owner_load_evidence_refs": ontology_evidence_refs(
            conn,
            "work_owner_load_snapshots",
            source_instance,
            f"and workstream_key in ({','.join('?' for _ in keys)})",
            keys,
        ),
        "blocker_evidence_refs": unique_strings(
            ontology_evidence_refs(conn, "work_blockers", source_instance, "and blocker_state = 'active'")
            + ontology_evidence_refs(conn, "work_blocker_impacts", source_instance, "and impact_state = 'active'")
        ),
    }


def ontology_work_action_insight_kinds(conn: sqlite3.Connection, source_instance: str, where_clause: str) -> list[str]:
    required = ["work_actions", "work_action_source_insights", "work_insights"]
    if any(not table_exists(conn, table) for table in required):
        return []
    rows = conn.execute(
        f"""
        select distinct wi.insight_kind
        from work_actions wa
        join work_action_source_insights wasi on wasi.work_action_id = wa.id
        join work_insights wi on wi.id = wasi.work_insight_id
        where wa.source_system = 'cubicle_analytics'
          and wa.source_instance = ?
          and wa.external_kind = 'tpm_work_action'
          and wi.source_system = 'cubicle_analytics'
          and wi.source_instance = wa.source_instance
          and ({where_clause})
        order by wi.insight_kind
        """,
        (source_instance,),
    ).fetchall()
    return [first_nonempty([row[0]]) for row in rows if first_nonempty([row[0]])]


def build_work_program_quality_gates(
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    facts: dict[str, Any],
    time_series_summary: pd.DataFrame | None = None,
) -> list[dict[str, Any]]:
    gates: list[dict[str, Any]] = []
    forecast_ready = forecast_effective_eta_ready(forecast_summary, time_series_summary)
    if forecast_ready:
        gates.append(quality_gate("forecast_readiness", "passed", False, "ETA forecast readiness gates passed.", "Continue backtesting forecasts against observed outcomes."))
    else:
        gates.append(
            quality_gate(
                "forecast_readiness",
                "gated",
                True,
                forecast_readiness_reason(
                    forecast_summary,
                    metric_int(forecast_summary, "merged_pr_count"),
                    False,
                    metric_text(forecast_summary, "backtest_best_model"),
                    metric_int(time_series_summary, "observed_snapshot_time_count") if time_series_summary is not None else None,
                    metric_int(time_series_summary, "transition_candidate_count") if time_series_summary is not None else None,
                ),
                "Use forecast output as risk triage, not an ETA commitment.",
            )
        )

    product_quality = product_action_measurement_quality(readiness, facts)
    precision_ready = bool(product_quality.get("precision_ready"))
    precision_rate = safe_float(product_quality.get("precision_rate"))
    useful_signal_rate = safe_float(product_quality.get("useful_signal_rate"))
    measurement_label_count = int_value(product_quality.get("measurement_label_count"))
    open_review_request_count = int_value(product_quality.get("open_review_request_count"))
    measured_kinds = product_quality.get("scope_kinds") or []
    measured_kind_phrase = ", ".join(str(kind) for kind in measured_kinds) if measured_kinds else "no product-action insight kinds"
    if precision_ready and precision_rate >= MIN_PRECISION_RATE_FOR_PRODUCT_ACTION and useful_signal_rate >= MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION:
        gates.append(quality_gate("measurement_precision", "passed", False, f"Product-action precision and useful-signal rates meet thresholds for {measured_kind_phrase}.", "Keep labeling fresh product-action-backed insights."))
    else:
        if precision_ready:
            detail = f"Product-action precision is measured for {measured_kind_phrase} but below threshold."
        else:
            detail = f"Product-action precision measurement is gated for {measured_kind_phrase}: {count_phrase(measurement_label_count, 'measurement label')} available, {count_phrase(open_review_request_count, 'open review request')}."
        gates.append(quality_gate("measurement_precision", "gated", True, detail, "Gold-label action-backed insight kinds and keep context-only signals as validation leads."))

    actionability_ready = bool(product_quality.get("actionability_ready"))
    actionability_rate = safe_float(product_quality.get("actionability_rate"))
    if actionability_ready and actionability_rate >= MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION:
        gates.append(quality_gate("measurement_actionability", "passed", False, f"Product-action actionability meets threshold for {measured_kind_phrase}.", "Keep actionability labels current for action-backed insights."))
    else:
        detail = f"Actionability measurement is not ready for {measured_kind_phrase}."
        if actionability_ready:
            detail = f"Product-action actionability is measured for {measured_kind_phrase} but below product-action threshold."
        gates.append(quality_gate("measurement_actionability", "gated", True, detail, "Add actionability labels for action-backed kinds and keep low-actionability context in validation packets."))

    global_precision_ready = metric_text(readiness, "ready_to_measure_precision").lower() == "true"
    global_actionability_ready = metric_text(readiness, "ready_to_measure_actionability").lower() == "true"
    if global_precision_ready:
        gates.append(quality_gate(GLOBAL_INSIGHT_PRECISION_KEY, "passed", False, "Global insight precision has enough labels for validation coverage measurement; product-action readiness is decided by product-action measurement gates.", "Keep global truth labels fresh as generated insight kinds change, and use measurement_precision for product-action promotion."))
    else:
        gates.append(quality_gate(GLOBAL_INSIGHT_PRECISION_KEY, "gated", True, "Global insight precision is not fully measurement-ready for validation coverage; product-action readiness remains separately gated.", "Add gold truth labels for validation and context leads before claiming autonomous TPM replacement."))
    if global_actionability_ready:
        gates.append(quality_gate(GLOBAL_INSIGHT_ACTIONABILITY_KEY, "passed", False, "Global insight actionability has enough labels for validation coverage measurement; product-action readiness is decided by product-action measurement gates.", "Keep global actionability labels fresh as generated insight kinds change, and use measurement_actionability for product-action promotion."))
    else:
        gates.append(quality_gate(GLOBAL_INSIGHT_ACTIONABILITY_KEY, "gated", True, "Global insight actionability is not fully measurement-ready for validation coverage; product-action readiness remains separately gated.", "Add gold actionability labels before allowing generated leads to drive autonomous TPM action."))

    source_limited_count = int_value(facts.get("source_coverage_limited_count"))
    if source_limited_count == 0:
        gates.append(quality_gate("source_coverage", "passed", False, "No typed program item in scope reports limited source coverage.", "Continue preserving source coverage state on every sync."))
    else:
        gates.append(quality_gate("source_coverage", "gated", True, f"{count_phrase(source_limited_count, 'program item')} {has_have(source_limited_count)} limited source coverage.", "Treat affected items as review leads until coverage is complete."))

    auth_limited_count = int_value(facts.get("auth_limited_observation_count"))
    auth_limited_product_decision_count = int_value(facts.get("auth_limited_product_decision_count"))
    if auth_limited_count == 0:
        gates.append(quality_gate("source_authentication", "passed", False, "No typed program item depends only on anonymous source observation.", "Keep authenticated observation available for product-sensitive claims."))
    elif auth_limited_product_decision_count > 0:
        gates.append(quality_gate("source_authentication", "gated", True, f"{count_phrase(auth_limited_product_decision_count, 'product-action or decision program item')} {has_have(auth_limited_product_decision_count)} only anonymous/public source observation.", "Re-observe these product-decision rows with authenticated access before absence, completion, or autonomous decision claims."))
    else:
        gates.append(quality_gate("source_authentication", "watch", False, f"{count_phrase(auth_limited_count, 'validation or QA program item')} {has_have(auth_limited_count)} only anonymous/public source observation.", "Keep these rows as lower-confidence validation leads until authenticated re-observation is attached; do not promote them to product actions first."))

    generated_claim_count = int_value(facts.get("generated_claim_limited_count"))
    generated_claim_product_decision_count = int_value(facts.get("generated_claim_product_decision_count"))
    if generated_claim_count == 0:
        gates.append(quality_gate("claim_provenance", "passed", False, "No typed program item depends only on generated or derived claim evidence.", "Continue linking generated claims to independent source or measurement evidence."))
    elif generated_claim_product_decision_count > 0:
        gates.append(quality_gate("claim_provenance", "gated", True, f"{count_phrase(generated_claim_product_decision_count, 'product-action or decision program item')} depend on generated or derived claim evidence.", "Keep generated product-decision claims in QA or validation until independent provenance or measurement evidence is attached."))
    else:
        gates.append(quality_gate("claim_provenance", "watch", False, f"{count_phrase(generated_claim_count, 'validation or QA program item')} depend on generated or derived claim evidence.", "Keep generated validation claims in QA/provenance review and require independent evidence before promotion to product actions."))

    overloaded_owner_count = int_value(facts.get("overloaded_owner_count"))
    unassigned_action_count = int_value(facts.get("unassigned_action_count"))
    if overloaded_owner_count == 0 and unassigned_action_count == 0:
        gates.append(quality_gate("owner_load", "passed", False, "Latest owner-load rows have no overloaded owners or unassigned product actions.", "Keep owner-load snapshots fresh as action volume changes."))
    else:
        parts = []
        if overloaded_owner_count > 0:
            parts.append(count_phrase(overloaded_owner_count, "overloaded owner"))
        if unassigned_action_count > 0:
            parts.append(count_phrase(unassigned_action_count, "unassigned product action"))
        gates.append(quality_gate("owner_load", "gated", True, f"{'; '.join(parts)} remain in the latest owner-load snapshot.", "Rebalance overloaded owner queues or assign unassigned product actions before treating the plan as autonomously executable."))

    unassigned_total_action_count = int_value(facts.get("unassigned_total_action_count"))
    if unassigned_total_action_count > unassigned_action_count:
        validation_backlog_count = unassigned_total_action_count - unassigned_action_count
        gates.append(
            quality_gate(
                "validation_backlog",
                "watch",
                False,
                f"{count_phrase(validation_backlog_count, 'unassigned validation or QA action')} remain visible but do not block product-action execution capacity.",
                "Route validation leads and QA items through measurement/provenance workflows before promoting them to product actions.",
            )
        )
    else:
        gates.append(
            quality_gate(
                "validation_backlog",
                "passed",
                False,
                "No unassigned validation-only actions remain in the latest owner-load snapshot.",
                "Keep validation backlog counts visible separately from product-action execution capacity.",
            )
        )

    active_blocker_count = int_value(facts.get("active_blocker_count"))
    active_blocker_impact_count = int_value(facts.get("active_blocker_impact_count"))
    if active_blocker_count == 0 and active_blocker_impact_count == 0:
        gates.append(quality_gate("blocker_clearance", "passed", False, "No active blocker is in scope.", "Maintain watch on new blocker signals."))
    else:
        gates.append(quality_gate("blocker_clearance", "gated", True, f"{count_phrase(active_blocker_count, 'active blocker')} and {count_phrase(active_blocker_impact_count, 'active blocker impact')} remain in scope.", "Assign owners and clear the highest-ranked blocker impacts."))

    dependency_action_count = int_value(facts.get("needs_action_dependency_count"))
    if dependency_action_count == 0:
        gates.append(quality_gate("dependency_pressure", "passed", False, "No needs-action dependency edge is in scope.", "Keep dependency topology fresh as actions open and close."))
    else:
        gates.append(quality_gate("dependency_pressure", "watch", False, f"{count_phrase(dependency_action_count, 'needs-action dependency edge')} remain in scope.", "Use dependency edges to drive owner follow-through; require owner evidence before declaring dependencies clear."))

    decision_signal_count = product_decision_signal_count(facts)
    if decision_signal_count == 0:
        gates.append(quality_gate("product_decision", "passed", False, "No product-decision action is currently open.", "Continue monitoring for merge, close, park, and reassignment decisions."))
    else:
        gates.append(quality_gate("product_decision", "watch", False, f"{count_phrase(decision_signal_count, 'product decision signal')} require owner judgment.", "Draft decision requests, but keep merge, close, park, and reassignment decisions human-approved."))
    return gates


def build_work_program_automation_readiness(
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    facts: dict[str, Any],
    gates: list[dict[str, Any]],
    evidence_needs: list[dict[str, Any]],
    function_readiness: list[dict[str, Any]],
) -> dict[str, Any]:
    score = 100.0
    safe_areas: list[str] = []
    human_areas: list[str] = []

    if int_value(facts.get("standup_section_count")) > 0:
        safe_areas = append_unique(safe_areas, "agenda_summarization")
    if int_value(facts.get("active_blocker_count")) > 0 or int_value(facts.get("active_blocker_impact_count")) > 0 or int_value(facts.get("needs_action_dependency_count")) > 0:
        safe_areas = append_unique(safe_areas, "risk_driver_ranking")
    if facts.get("program_item_evidence_refs"):
        safe_areas = append_unique(safe_areas, "source_citation")
    if not forecast_summary.empty:
        safe_areas = append_unique(safe_areas, "forecast_triage")

    forecast_ready = forecast_effective_eta_ready(forecast_summary)
    precision_ready = metric_text(readiness, "ready_to_measure_precision").lower() == "true"
    actionability_ready = metric_text(readiness, "ready_to_measure_actionability").lower() == "true"
    if not forecast_ready:
        score -= 25
        human_areas = append_unique(human_areas, "eta_commitments")
    if not precision_ready:
        score -= 25
        human_areas = append_unique(human_areas, "measurement_claims")
    if not actionability_ready:
        score -= 20
        human_areas = append_unique(human_areas, "measurement_claims")
    if int_value(facts.get("source_coverage_limited_count")) > 0:
        score -= 15
        human_areas = append_unique(human_areas, "coverage_repair")
    if int_value(facts.get("auth_limited_observation_count")) > 0:
        score -= 5
        human_areas = append_unique(human_areas, "source_authentication")
    if int_value(facts.get("generated_claim_limited_count")) > 0:
        score -= 10
        human_areas = append_unique(human_areas, "claim_provenance")
    if int_value(facts.get("active_blocker_count")) > 0 or int_value(facts.get("active_blocker_impact_count")) > 0:
        score -= 15
        human_areas = append_unique(human_areas, "blocker_clearance")
    if int_value(facts.get("overloaded_owner_count")) > 0 or int_value(facts.get("unassigned_action_count")) > 0:
        score -= 10
        human_areas = append_unique(human_areas, "owner_load_balancing")
    if int_value(facts.get("product_action_count")) > 0 or int_value(facts.get("needs_decision_count")) > 0:
        human_areas = append_unique(human_areas, "product_decisions")
    if score < 0:
        score = 0.0

    blocking_gate_keys = [first_nonempty([gate.get("key")]) for gate in gates if bool(gate.get("blocking"))]
    blocking_gate_keys = unique_strings(blocking_gate_keys)
    state = "automatable"
    blocking_for_autonomous_action = {
        "measurement_precision",
        "measurement_actionability",
        "source_coverage",
        "source_authentication",
        "claim_provenance",
        "blocker_clearance",
        "owner_load",
    }
    if score < 35 or any(key in blocking_for_autonomous_action for key in blocking_gate_keys):
        state = "blocked"
    elif score < 70:
        state = "supervised"
    elif score < 90:
        state = "assisted"
    autonomous_ready = state == "automatable" and not blocking_gate_keys

    return {
        "readiness_state": state,
        "readiness_score": score,
        "autonomous_action_ready": autonomous_ready,
        "human_review_required": not autonomous_ready,
        "safe_automation_areas": safe_areas,
        "human_required_areas": human_areas,
        "rationale": automation_readiness_rationale(state, blocking_gate_keys),
        "required_evidence": automation_required_evidence(blocking_gate_keys),
        "blocking_gate_keys": blocking_gate_keys,
        "quality_gate_count": len(gates),
        "blocking_gate_count": len(blocking_gate_keys),
        "evidence_need_count": len(evidence_needs),
        "tpm_function_count": len(function_readiness),
    }


def build_work_program_brief_caveats(
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    facts: dict[str, Any],
) -> list[dict[str, Any]]:
    caveats: list[dict[str, Any]] = []
    forecast_ready = forecast_effective_eta_ready(forecast_summary)
    if not forecast_ready:
        detail = first_nonempty([metric_text(forecast_summary, "readiness_reason")])
        if not detail:
            detail = forecast_readiness_reason(
                forecast_summary,
                metric_int(forecast_summary, "merged_pr_count"),
                False,
                metric_text(forecast_summary, "backtest_best_model"),
                int_value(metric_text(forecast_summary, "observed_snapshot_time_count")),
                int_value(metric_text(forecast_summary, "transition_candidate_count")),
            )
        if not detail:
            detail = "Forecast output is useful for prioritization, but not ready as an ETA promise."
        caveats.append(
            brief_caveat(
                "forecast_gated",
                "warning",
                "Forecast gated",
                detail,
                "Do not present forecast dates as commitments.",
                first_nonempty(facts.get("forecast_evidence_refs", [])),
            )
        )

    precision_ready = metric_text(readiness, "ready_to_measure_precision").lower() == "true"
    actionability_ready = metric_text(readiness, "ready_to_measure_actionability").lower() == "true"
    if not precision_ready or not actionability_ready:
        gated_kind_count = int_value(metric_text(readiness, "gated_insight_kind_count"))
        open_review_request_count = int_value(metric_text(readiness, "open_review_request_count"))
        detail = "Generated insight quality is not fully measurement-ready."
        if gated_kind_count > 0 or open_review_request_count > 0:
            detail = f"Generated insight quality is gated by {count_phrase(gated_kind_count, 'gated insight kind')} and {count_phrase(open_review_request_count, 'open review request')}."
        caveats.append(
            brief_caveat(
                "measurement_gated",
                "warning",
                "Measurement gated",
                detail,
                "Add gold truth/actionability labels before claiming autonomous TPM replacement.",
                "",
            )
        )

    source_limited_count = int_value(facts.get("source_coverage_limited_count"))
    if source_limited_count > 0:
        caveats.append(
            brief_caveat(
                "coverage_limited",
                "warning",
                "Source coverage limited",
                f"{count_phrase(source_limited_count, 'program item')} {has_have(source_limited_count)} partial or limited source coverage.",
                "Use these rows as review leads, not absence claims.",
                first_nonempty(facts.get("program_item_evidence_refs", [])),
            )
        )

    auth_limited_count = int_value(facts.get("auth_limited_observation_count"))
    if auth_limited_count > 0:
        caveats.append(
            brief_caveat(
                "source_authentication_limited",
                "warning",
                "Source authentication limited",
                f"{count_phrase(auth_limited_count, 'program item')} {has_have(auth_limited_count)} successful but anonymous source observation.",
                "Use these rows as lower-confidence leads until authenticated re-observation is attached.",
                first_nonempty(facts.get("program_item_evidence_refs", [])),
            )
        )

    generated_claim_count = int_value(facts.get("generated_claim_limited_count"))
    if generated_claim_count > 0:
        caveats.append(
            brief_caveat(
                "generated_claim_provenance",
                "warning",
                "Generated claim provenance limited",
                f"{count_phrase(generated_claim_count, 'program item')} depend on generated or derived claim evidence.",
                "Keep these rows in validation or QA until independent source/provenance evidence is attached.",
                first_nonempty(facts.get("program_item_evidence_refs", [])),
            )
        )

    overloaded_owner_count = int_value(facts.get("overloaded_owner_count"))
    unassigned_action_count = int_value(facts.get("unassigned_action_count"))
    if overloaded_owner_count > 0 or unassigned_action_count > 0:
        parts = []
        severity = "warning"
        if overloaded_owner_count > 0:
            parts.append(count_phrase(overloaded_owner_count, "overloaded owner"))
            severity = "danger"
        if unassigned_action_count > 0:
            parts.append(count_phrase(unassigned_action_count, "unassigned product action"))
        caveats.append(
            brief_caveat(
                "owner_load",
                severity,
                "Owner load constrained",
                f"{'; '.join(parts)} remain in the latest owner-load snapshot.",
                "Rebalance or explicitly accept product-action owner concentration before using the brief as an autonomous execution plan.",
                first_nonempty(facts.get("owner_load_evidence_refs", [])),
            )
        )

    active_blocker_count = int_value(facts.get("active_blocker_count"))
    active_blocker_impact_count = int_value(facts.get("active_blocker_impact_count"))
    if active_blocker_count > 0 or active_blocker_impact_count > 0:
        caveats.append(
            brief_caveat(
                "active_blockers",
                "danger",
                "Active blockers",
                f"{count_phrase(active_blocker_count, 'active blocker')} and {count_phrase(active_blocker_impact_count, 'active blocker impact')} remain open.",
                "Clear blockers before treating the workstream as on track.",
                first_nonempty(facts.get("blocker_evidence_refs", [])),
            )
        )

    needs_action_dependency_count = int_value(facts.get("needs_action_dependency_count"))
    if needs_action_dependency_count > 0:
        caveats.append(
            brief_caveat(
                "dependency_pressure",
                "warning",
                "Dependency pressure",
                f"{count_phrase(needs_action_dependency_count, 'dependency needing action')} {is_are(needs_action_dependency_count)} in scope.",
                "Use top dependency drivers to assign concrete owners.",
                "",
            )
        )
    return caveats


def brief_caveat(
    key: str,
    severity: str,
    title: str,
    detail: str,
    recommended_action: str,
    evidence_ref: str,
) -> dict[str, Any]:
    return {
        "key": key,
        "severity": severity,
        "title": title,
        "detail": detail,
        "recommended_action": recommended_action,
        "evidence_ref": evidence_ref,
    }


def build_work_program_summary_snapshot(
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    facts: dict[str, Any],
) -> dict[str, Any]:
    forecast_state = work_program_forecast_state(forecast_summary)
    status_counts = dict(facts.get("program_status_counts") or {})
    decision_counts = dict(facts.get("decision_state_counts") or {})
    source_coverage_limit_counts = dict(facts.get("source_coverage_limit_counts") or {})
    auth_limited_observation_counts = dict(facts.get("auth_limited_observation_counts") or {})
    auth_limited_product_decision_counts = dict(facts.get("auth_limited_product_decision_counts") or {})
    generated_claim_limit_counts = dict(facts.get("generated_claim_limit_counts") or {})
    generated_claim_product_decision_counts = dict(facts.get("generated_claim_product_decision_counts") or {})
    owner_load_status_counts = dict(facts.get("owner_load_status_counts") or {})
    total_count = int_value(facts.get("total_count"))
    needs_decision_count = int_value(facts.get("needs_decision_count") or status_counts.get("needs_decision"))
    validate_signal_count = int_value(status_counts.get("validate_signal"))
    ci_failing_count = int_value(status_counts.get("ci_failing"))
    waiting_review_count = int_value(status_counts.get("waiting_review"))
    source_repair_count = int_value(status_counts.get("source_repair"))
    closed_pending_review_count = int_value(status_counts.get("closed_pending_review"))
    model_quality_count = int_value(status_counts.get("model_quality"))
    closure_candidate_count = int_value(status_counts.get("closure_candidate"))
    dismissed_count = int_value(status_counts.get("dismissed"))
    product_action_count = int_value(facts.get("product_action_count") or decision_counts.get("product_action"))
    validation_lead_count = int_value(decision_counts.get("validation_lead"))
    source_coverage_limited_count = int_value(facts.get("source_coverage_limited_count"))
    active_blocker_count = int_value(facts.get("active_blocker_count"))
    active_blocker_impact_count = int_value(facts.get("active_blocker_impact_count"))
    needs_action_dependency_count = int_value(facts.get("needs_action_dependency_count"))
    overloaded_owner_count = int_value(facts.get("overloaded_owner_count"))
    unassigned_action_count = int_value(facts.get("unassigned_action_count"))
    operating_status = work_program_operating_status(
        total_count,
        product_action_count,
        needs_decision_count,
        validation_lead_count,
        source_coverage_limited_count,
        active_blocker_count,
        active_blocker_impact_count,
        needs_action_dependency_count,
        overloaded_owner_count,
        unassigned_action_count,
    )
    decision_pressure = work_program_decision_pressure(
        forecast_state,
        product_action_count,
        needs_decision_count,
        validation_lead_count,
        active_blocker_count,
        active_blocker_impact_count,
        needs_action_dependency_count,
        overloaded_owner_count,
        unassigned_action_count,
    )
    primary_risk = work_program_primary_risk(
        forecast_state,
        total_count,
        validation_lead_count,
        source_coverage_limited_count,
        active_blocker_count,
        active_blocker_impact_count,
        needs_action_dependency_count,
        overloaded_owner_count,
        unassigned_action_count,
    )
    capability_gaps = work_program_capability_gaps(
        forecast_state,
        validation_lead_count,
        source_coverage_limited_count,
        active_blocker_count,
        active_blocker_impact_count,
        needs_action_dependency_count,
        overloaded_owner_count,
        unassigned_action_count,
    )
    return {
        "total_count": total_count,
        "needs_decision_count": needs_decision_count,
        "validate_signal_count": validate_signal_count,
        "ci_failing_count": ci_failing_count,
        "waiting_review_count": waiting_review_count,
        "source_repair_count": source_repair_count,
        "closed_pending_review_count": closed_pending_review_count,
        "model_quality_count": model_quality_count,
        "closure_candidate_count": closure_candidate_count,
        "dismissed_count": dismissed_count,
        "now_count": int_value(facts.get("now_count")),
        "high_risk_count": int_value(facts.get("high_risk_count")),
        "unassigned_count": int_value(facts.get("unassigned_count")),
        "product_action_count": product_action_count,
        "validation_lead_count": validation_lead_count,
        "source_coverage_limited_count": source_coverage_limited_count,
        "owner_load_status": first_nonempty([facts.get("owner_load_status")]) or "clear",
        "owner_load_action_count": int_value(facts.get("owner_load_action_count")),
        "overloaded_owner_count": overloaded_owner_count,
        "attention_owner_count": int_value(facts.get("attention_owner_count")),
        "unassigned_action_count": unassigned_action_count,
        "blocker_count": int_value(facts.get("blocker_count")),
        "active_blocker_count": active_blocker_count,
        "validating_blocker_count": int_value(facts.get("validating_blocker_count")),
        "blocker_impact_count": int_value(facts.get("blocker_impact_count")),
        "active_blocker_impact_count": active_blocker_impact_count,
        "dependency_edge_count": int_value(facts.get("dependency_edge_count")),
        "blocking_dependency_count": int_value(facts.get("blocking_dependency_count")),
        "needs_action_dependency_count": needs_action_dependency_count,
        "operating_status": operating_status,
        "decision_pressure": decision_pressure,
        "forecast_state": forecast_state,
        "primary_risk": primary_risk,
        "recommended_focus": work_program_recommended_focus(
            forecast_state,
            total_count,
            product_action_count,
            validation_lead_count,
            active_blocker_count,
            active_blocker_impact_count,
            needs_action_dependency_count,
            overloaded_owner_count,
            unassigned_action_count,
        ),
        "capability_gaps": capability_gaps,
        "program_status_counts": status_counts,
        "decision_state_counts": decision_counts,
        "source_coverage_limit_counts": source_coverage_limit_counts,
        "auth_limited_observation_counts": auth_limited_observation_counts,
        "auth_limited_product_decision_counts": auth_limited_product_decision_counts,
        "generated_claim_limit_counts": generated_claim_limit_counts,
        "generated_claim_product_decision_counts": generated_claim_product_decision_counts,
        "owner_load_status_counts": owner_load_status_counts,
    }


def work_program_summary_breakdown_fields(snapshot: dict[str, Any]) -> tuple[str, str, str]:
    rows: list[tuple[str, str, int]] = []
    for dimension, counts_key in [
        ("program_status", "program_status_counts"),
        ("decision_state", "decision_state_counts"),
        ("source_coverage_limit_kind", "source_coverage_limit_counts"),
        ("auth_limited_observation_kind", "auth_limited_observation_counts"),
        ("auth_limited_product_decision_kind", "auth_limited_product_decision_counts"),
        ("generated_claim_limit_kind", "generated_claim_limit_counts"),
        ("generated_claim_product_decision_kind", "generated_claim_product_decision_counts"),
        ("owner_load_status", "owner_load_status_counts"),
    ]:
        counts = dict(snapshot.get(counts_key) or {})
        for key, count in sorted(counts.items(), key=lambda item: (-int_value(item[1]), str(item[0]))):
            key_text = first_nonempty([key])
            if not key_text:
                continue
            rows.append((dimension, key_text, int_value(count)))
    return (
        "\n".join(row[0] for row in rows),
        "\n".join(row[1] for row in rows),
        "\n".join(str(row[2]) for row in rows),
    )


def work_program_summary_snapshot_confidence(snapshot: dict[str, Any]) -> float:
    if int_value(snapshot.get("total_count")) == 0:
        return 0.85
    if int_value(snapshot.get("active_blocker_count")) > 0 or int_value(snapshot.get("active_blocker_impact_count")) > 0:
        return 1.0
    if int_value(snapshot.get("source_coverage_limited_count")) > 0:
        return 0.95
    return 0.9


def work_program_summary_snapshot_rank_score(snapshot: dict[str, Any]) -> float:
    score = float(int_value(snapshot.get("total_count")))
    score += float(int_value(snapshot.get("active_blocker_count")) * 15)
    score += float(int_value(snapshot.get("active_blocker_impact_count")) * 10)
    score += float(int_value(snapshot.get("needs_action_dependency_count")) * 5)
    score += float(int_value(snapshot.get("overloaded_owner_count")) * 5)
    return score


def build_work_program_brief_snapshot(
    readiness: pd.DataFrame,
    forecast_summary: pd.DataFrame,
    facts: dict[str, Any],
    gates: list[dict[str, Any]],
    caveats: list[dict[str, Any]],
    risk_drivers: list[dict[str, Any]],
) -> dict[str, Any]:
    forecast_state = work_program_forecast_state(forecast_summary)
    total_count = int_value(facts.get("total_count"))
    decision_counts = dict(facts.get("decision_state_counts") or {})
    status_counts = dict(facts.get("program_status_counts") or {})
    product_action_count = int_value(facts.get("product_action_count") or decision_counts.get("product_action"))
    needs_decision_count = int_value(facts.get("needs_decision_count") or status_counts.get("needs_decision"))
    validation_lead_count = int_value(decision_counts.get("validation_lead"))
    source_coverage_limited_count = int_value(facts.get("source_coverage_limited_count"))
    active_blocker_count = int_value(facts.get("active_blocker_count"))
    active_blocker_impact_count = int_value(facts.get("active_blocker_impact_count"))
    needs_action_dependency_count = int_value(facts.get("needs_action_dependency_count"))
    overloaded_owner_count = int_value(facts.get("overloaded_owner_count"))
    unassigned_action_count = int_value(facts.get("unassigned_action_count"))
    operating_status = work_program_operating_status(
        total_count,
        product_action_count,
        needs_decision_count,
        validation_lead_count,
        source_coverage_limited_count,
        active_blocker_count,
        active_blocker_impact_count,
        needs_action_dependency_count,
        overloaded_owner_count,
        unassigned_action_count,
    )
    decision_pressure = work_program_decision_pressure(
        forecast_state,
        product_action_count,
        needs_decision_count,
        validation_lead_count,
        active_blocker_count,
        active_blocker_impact_count,
        needs_action_dependency_count,
        overloaded_owner_count,
        unassigned_action_count,
    )
    primary_risk = work_program_primary_risk(
        forecast_state,
        total_count,
        validation_lead_count,
        source_coverage_limited_count,
        active_blocker_count,
        active_blocker_impact_count,
        needs_action_dependency_count,
        overloaded_owner_count,
        unassigned_action_count,
    )
    capability_gaps = work_program_capability_gaps(
        forecast_state,
        validation_lead_count,
        source_coverage_limited_count,
        active_blocker_count,
        active_blocker_impact_count,
        needs_action_dependency_count,
        overloaded_owner_count,
        unassigned_action_count,
    )
    return {
        "operating_status": operating_status,
        "decision_pressure": decision_pressure,
        "forecast_state": forecast_state,
        "primary_risk": primary_risk,
        "executive_summary": work_program_executive_summary(
            operating_status,
            total_count,
            product_action_count,
            validation_lead_count,
            active_blocker_count,
            active_blocker_impact_count,
            forecast_state,
        ),
        "recommended_focus": work_program_recommended_focus(
            forecast_state,
            total_count,
            product_action_count,
            validation_lead_count,
            active_blocker_count,
            active_blocker_impact_count,
            needs_action_dependency_count,
            overloaded_owner_count,
            unassigned_action_count,
        ),
        "next_cadence_focus": work_program_next_cadence_focus(
            forecast_state,
            product_action_count,
            validation_lead_count,
            active_blocker_count,
            active_blocker_impact_count,
            needs_action_dependency_count,
            len(risk_drivers),
        ),
        "capability_gaps": capability_gaps,
        "total_count": total_count,
        "product_action_count": product_action_count,
        "validation_lead_count": validation_lead_count,
        "source_coverage_limited_count": source_coverage_limited_count,
        "active_blocker_count": active_blocker_count,
        "active_blocker_impact_count": active_blocker_impact_count,
        "needs_action_dependency_count": needs_action_dependency_count,
        "overloaded_owner_count": overloaded_owner_count,
        "unassigned_action_count": unassigned_action_count,
        "quality_gate_count": len(gates),
        "blocking_gate_count": sum(1 for gate in gates if bool(gate.get("blocking"))),
        "caveat_count": len(caveats),
        "risk_driver_count": len(risk_drivers),
    }


def work_program_forecast_state(forecast_summary: pd.DataFrame) -> str:
    if forecast_summary.empty:
        return "missing"
    if forecast_effective_eta_ready(forecast_summary):
        return "ready"
    return "gated"


def work_program_operating_status(
    total_count: int,
    product_action_count: int,
    needs_decision_count: int,
    validation_lead_count: int,
    source_coverage_limited_count: int,
    active_blocker_count: int,
    active_blocker_impact_count: int,
    needs_action_dependency_count: int,
    overloaded_owner_count: int,
    unassigned_action_count: int,
) -> str:
    if active_blocker_count > 0 or active_blocker_impact_count > 0:
        return "blocked"
    if product_action_count > 0 or needs_decision_count > 0 or needs_action_dependency_count > 0:
        return "attention_required"
    if overloaded_owner_count > 0 or unassigned_action_count > 0:
        return "attention_required"
    if validation_lead_count > 0 or source_coverage_limited_count > 0:
        return "validation_required"
    if total_count > 0:
        return "watch"
    return "clear"


def work_program_decision_pressure(
    forecast_state: str,
    product_action_count: int,
    needs_decision_count: int,
    validation_lead_count: int,
    active_blocker_count: int,
    active_blocker_impact_count: int,
    needs_action_dependency_count: int,
    overloaded_owner_count: int,
    unassigned_action_count: int,
) -> str:
    if active_blocker_count > 0 or active_blocker_impact_count > 0:
        return "blocked"
    if product_action_count > 0 or needs_decision_count > 0:
        return "product_action"
    if needs_action_dependency_count > 0:
        return "dependency_action"
    if overloaded_owner_count > 0 or unassigned_action_count > 0:
        return "owner_load"
    if validation_lead_count > 0:
        return "validation"
    if forecast_state == "gated":
        return "forecast_quality"
    return "watch"


def work_program_primary_risk(
    forecast_state: str,
    total_count: int,
    validation_lead_count: int,
    source_coverage_limited_count: int,
    active_blocker_count: int,
    active_blocker_impact_count: int,
    needs_action_dependency_count: int,
    overloaded_owner_count: int,
    unassigned_action_count: int,
) -> str:
    if active_blocker_count > 0 or active_blocker_impact_count > 0:
        return "active_blockers"
    if needs_action_dependency_count > 0:
        return "dependency_pressure"
    if source_coverage_limited_count > 0:
        return "coverage_limited"
    if overloaded_owner_count > 0 or unassigned_action_count > 0:
        return "owner_load"
    if validation_lead_count > 0:
        return "unvalidated_signals"
    if forecast_state == "gated":
        return "forecast_gated"
    if total_count > 0:
        return "workstream_watch"
    return ""


def work_program_capability_gaps(
    forecast_state: str,
    validation_lead_count: int,
    source_coverage_limited_count: int,
    active_blocker_count: int,
    active_blocker_impact_count: int,
    needs_action_dependency_count: int,
    overloaded_owner_count: int,
    unassigned_action_count: int,
) -> list[str]:
    gaps: list[str] = []
    if forecast_state in {"gated", "missing"}:
        gaps = append_unique(gaps, "forecast_gated")
    if source_coverage_limited_count > 0:
        gaps = append_unique(gaps, "coverage_limited")
    if overloaded_owner_count > 0:
        gaps = append_unique(gaps, "owner_overloaded")
    if unassigned_action_count > 0:
        gaps = append_unique(gaps, "owner_load_unassigned")
    if active_blocker_count > 0 or active_blocker_impact_count > 0:
        gaps = append_unique(gaps, "active_blockers")
    if needs_action_dependency_count > 0:
        gaps = append_unique(gaps, "dependency_pressure")
    if validation_lead_count > 0:
        gaps = append_unique(gaps, "validation_backlog")
    return gaps


def work_program_executive_summary(
    operating_status: str,
    total_count: int,
    product_action_count: int,
    validation_lead_count: int,
    active_blocker_count: int,
    active_blocker_impact_count: int,
    forecast_state: str,
) -> str:
    parts = [f"{operating_status}: {count_phrase(total_count, 'typed program item')}"]
    if active_blocker_count > 0:
        parts.append(count_phrase(active_blocker_count, "active blocker"))
    if active_blocker_impact_count > 0:
        parts.append(count_phrase(active_blocker_impact_count, "active blocker impact"))
    if product_action_count > 0:
        parts.append(count_phrase(product_action_count, "product action"))
    if validation_lead_count > 0:
        parts.append(count_phrase(validation_lead_count, "validation lead"))
    if forecast_state == "gated":
        parts.append("ETA forecast gated")
    return "; ".join(parts) + "."


def work_program_recommended_focus(
    forecast_state: str,
    total_count: int,
    product_action_count: int,
    validation_lead_count: int,
    active_blocker_count: int,
    active_blocker_impact_count: int,
    needs_action_dependency_count: int,
    overloaded_owner_count: int,
    unassigned_action_count: int,
) -> str:
    parts: list[str] = []
    if active_blocker_count > 0:
        parts.append(count_phrase(active_blocker_count, "active blocker"))
    if active_blocker_impact_count > 0:
        parts.append(count_phrase(active_blocker_impact_count, "active blocker impact"))
    if product_action_count > 0:
        parts.append(count_phrase(product_action_count, "product action"))
    if validation_lead_count > 0:
        parts.append(count_phrase(validation_lead_count, "validation lead"))
    if needs_action_dependency_count > 0:
        parts.append(count_phrase(needs_action_dependency_count, "dependency needing action"))
    if overloaded_owner_count > 0:
        parts.append(count_phrase(overloaded_owner_count, "overloaded owner"))
    if unassigned_action_count > 0:
        parts.append(count_phrase(unassigned_action_count, "unassigned product action"))
    if forecast_state == "gated":
        parts.append("treat ETA forecast as gated")
    if not parts:
        if total_count == 0:
            return "No typed program items are in scope."
        return "Maintain watch on typed program items."
    return "Focus on " + join_focus_parts(parts) + "."


def work_program_next_cadence_focus(
    forecast_state: str,
    product_action_count: int,
    validation_lead_count: int,
    active_blocker_count: int,
    active_blocker_impact_count: int,
    needs_action_dependency_count: int,
    risk_driver_count: int,
) -> str:
    if active_blocker_count > 0 or active_blocker_impact_count > 0:
        return "Run blocker review, assign owners, and close the highest-impact dependency actions before treating ETA output as a plan."
    if product_action_count > 0 or needs_action_dependency_count > 0 or risk_driver_count > 0:
        return "Drive immediate product actions, then re-check validation and source coverage before the next standup."
    if validation_lead_count > 0:
        return "Spend the next review cycle validating generated leads and suppressing low-confidence signals."
    if forecast_state == "gated":
        return "Keep forecast output as risk triage until readiness gates clear."
    return "Maintain watch and refresh the typed operating brief on the next source sync."


def brief_snapshot_confidence(snapshot: dict[str, Any], gates: list[dict[str, Any]], caveats: list[dict[str, Any]]) -> float:
    if int_value(snapshot.get("total_count")) == 0:
        return 0.85
    if any(bool(gate.get("blocking")) for gate in gates) or caveats:
        return 1.0
    return 0.95


def brief_snapshot_rank_score(snapshot: dict[str, Any], gates: list[dict[str, Any]], caveats: list[dict[str, Any]]) -> float:
    score = float(int_value(snapshot.get("total_count")))
    score += float(sum(10 for gate in gates if bool(gate.get("blocking"))))
    score += float(len(caveats) * 5)
    return score


def build_work_program_adversarial_checks(readiness: pd.DataFrame, forecast_summary: pd.DataFrame, facts: dict[str, Any]) -> list[dict[str, Any]]:
    checks: list[dict[str, Any]] = []
    total_count = int(facts.get("total_count") or 0)
    standup_section_count = int(facts.get("standup_section_count") or 0)
    forecast_ready = forecast_effective_eta_ready(forecast_summary)
    precision_ready = metric_text(readiness, "ready_to_measure_precision").lower() == "true"
    actionability_ready = metric_text(readiness, "ready_to_measure_actionability").lower() == "true"
    product_quality = product_action_measurement_quality(readiness, facts)
    product_precision_ready = (
        bool(product_quality.get("precision_ready"))
        and safe_float(product_quality.get("precision_rate")) >= MIN_PRECISION_RATE_FOR_PRODUCT_ACTION
        and safe_float(product_quality.get("useful_signal_rate")) >= MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION
    )
    product_actionability_ready = (
        bool(product_quality.get("actionability_ready"))
        and safe_float(product_quality.get("actionability_rate")) >= MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION
    )
    source_limited_count = int(facts.get("source_coverage_limited_count") or 0)
    auth_limited_count = int(facts.get("auth_limited_observation_count") or 0)
    generated_claim_count = int(facts.get("generated_claim_limited_count") or 0)
    overloaded_owner_count = int(facts.get("overloaded_owner_count") or 0)
    unassigned_action_count = int(facts.get("unassigned_action_count") or 0)
    active_blocker_count = int(facts.get("active_blocker_count") or 0)
    active_blocker_impact_count = int(facts.get("active_blocker_impact_count") or 0)
    decision_signal_count = int(facts.get("product_action_count") or 0) + int(facts.get("needs_decision_count") or 0)

    if total_count == 0:
        checks.append(adversarial_check("brief_basis", "operating_brief", "fail", "critical", "No typed work basis", "The brief has no typed program rows, so any TPM summary would be speculative.", "Load typed program rows before publishing an AI TPM brief."))
    elif standup_section_count == 0:
        checks.append(adversarial_check("brief_basis", "operating_brief", "warning", "medium", "Agenda not persisted", "Typed program rows exist, but there are no persisted standup sections to anchor a TPM agenda.", "Persist standup sections before treating the brief as meeting-ready.", evidence_refs=facts.get("program_item_evidence_refs", [])))
    else:
        checks.append(adversarial_check("brief_basis", "operating_brief", "pass", "info", "Typed brief basis present", "Typed program rows and standup sections are present.", "Keep refreshing the typed source sync and standup generation.", evidence_refs=facts.get("program_item_evidence_refs", [])))

    if forecast_ready:
        checks.append(adversarial_check("forecast_overclaim", "forecast", "pass", "info", "Forecast readiness passed", "Forecast readiness gates are passing for this source scope.", "Continue backtesting forecast output against observed transitions.", evidence_refs=facts.get("forecast_evidence_refs", [])))
    else:
        checks.append(adversarial_check("forecast_overclaim", "forecast", "fail", "critical", "ETA overclaim risk", "Forecast rows can rank risk but cannot be presented as ETA commitments.", "Keep forecasts framed as risk triage until forecast readiness gates pass.", ["forecast_readiness"], facts.get("forecast_evidence_refs", [])))

    if source_limited_count > 0:
        checks.append(adversarial_check("source_absence_claims", "source_coverage", "fail", "critical", "Absence claims unsafe", f"{count_phrase(source_limited_count, 'program item')} {has_have(source_limited_count)} limited source coverage.", "Do not claim absence or completion for affected rows until source coverage is repaired.", ["source_coverage"]))
    else:
        checks.append(adversarial_check("source_absence_claims", "source_coverage", "pass", "info", "Source coverage clear", "No typed program item in scope reports limited source coverage.", "Keep coverage state attached to source-backed rows."))

    if auth_limited_count > 0:
        checks.append(adversarial_check("source_authentication_claims", "source_authentication", "warning", "high", "Anonymous observation boundary", f"{count_phrase(auth_limited_count, 'program item')} {has_have(auth_limited_count)} successful but anonymous source observation.", "Do not use anonymous-only observations for absence, completion, or autonomous decision claims until authenticated re-observation is attached.", ["source_authentication"]))
    else:
        checks.append(adversarial_check("source_authentication_claims", "source_authentication", "pass", "info", "Authenticated observation boundary clear", "No typed program item depends only on anonymous source observation.", "Keep auth state attached to source-backed rows."))

    if generated_claim_count > 0:
        checks.append(adversarial_check("generated_claim_provenance", "claim_provenance", "warning", "high", "Generated claim provenance boundary", f"{count_phrase(generated_claim_count, 'program item')} depend on generated or derived claim evidence.", "Keep generated claims in validation or QA until independent source/provenance evidence is attached.", ["claim_provenance"]))
    else:
        checks.append(adversarial_check("generated_claim_provenance", "claim_provenance", "pass", "info", "Generated claim provenance clear", "No typed program item depends only on generated or derived claim evidence.", "Continue preserving independent provenance for generated claims."))

    measurement_gates = []
    if not precision_ready:
        measurement_gates.append(GLOBAL_INSIGHT_PRECISION_KEY)
    if not actionability_ready:
        measurement_gates.append(GLOBAL_INSIGHT_ACTIONABILITY_KEY)
    if measurement_gates:
        checks.append(adversarial_check("measurement_overclaim", "measurement", "fail", "critical", "Global insight quality overclaim risk", "Global insight precision/actionability measurement is not fully ready for TPM replacement.", "Add gold truth/actionability labels before claiming autonomous TPM replacement.", measurement_gates))
    elif not product_precision_ready or not product_actionability_ready:
        product_gates = []
        if not product_precision_ready:
            product_gates.append("measurement_precision")
        if not product_actionability_ready:
            product_gates.append("measurement_actionability")
        measured_kinds = product_quality.get("scope_kinds") or []
        measured_kind_phrase = ", ".join(str(kind) for kind in measured_kinds) if measured_kinds else "no product-action insight kinds"
        checks.append(adversarial_check("measurement_overclaim", "measurement", "fail", "critical", "Product-action insight quality overclaim risk", f"Global/context labels are measured, but product-action measurement is not ready for {measured_kind_phrase}.", "Use measured context labels for validation only; add product-action scoped truth/actionability labels before claiming autonomous TPM replacement.", product_gates))
    else:
        checks.append(adversarial_check("measurement_overclaim", "measurement", "pass", "info", "Product-action insight quality measured", "Product-action precision and actionability are measurement-ready.", "Use measured product-action quality gates to decide which signal kinds can become product actions."))

    if overloaded_owner_count > 0 or unassigned_action_count > 0:
        checks.append(adversarial_check("execution_assumption", "execution_capacity", "fail", "high", "Execution capacity overclaim risk", f"{count_phrase(overloaded_owner_count, 'overloaded owner')} and {count_phrase(unassigned_action_count, 'unassigned product action')} make autonomous execution unsafe.", "Rebalance overloaded product-action queues or assign unowned product actions before treating the plan as executable.", ["owner_load"], facts.get("owner_load_evidence_refs", [])))
    else:
        checks.append(adversarial_check("execution_assumption", "execution_capacity", "pass", "info", "Execution capacity has no owner-load blocker", "Owner-load rows do not show overloaded owners or unassigned product actions.", "Keep owner-load snapshots fresh as actions change.", evidence_refs=facts.get("owner_load_evidence_refs", [])))

    if active_blocker_count > 0 or active_blocker_impact_count > 0:
        checks.append(adversarial_check("blocker_clearance_claim", "blocker_clearance", "fail", "critical", "Blocker clearance overclaim risk", f"{count_phrase(active_blocker_count, 'active blocker')} and {count_phrase(active_blocker_impact_count, 'active blocker impact')} remain open.", "Require owner-confirmed clearance before declaring the workstream unblocked.", ["blocker_clearance"], facts.get("blocker_evidence_refs", [])))
    else:
        checks.append(adversarial_check("blocker_clearance_claim", "blocker_clearance", "pass", "info", "No active blocker clearance claim at risk", "No active blocker or blocker impact is in scope.", "Maintain blocker watch on future syncs.", evidence_refs=facts.get("blocker_evidence_refs", [])))

    if decision_signal_count > 0:
        checks.append(adversarial_check("human_decision_boundary", "product_decision", "warning", "high", "Human decision boundary", f"{count_phrase(decision_signal_count, 'product decision signal')} require owner judgment.", "Draft decision requests, but do not automate merge, close, park, or owner reassignment decisions."))
    else:
        checks.append(adversarial_check("human_decision_boundary", "product_decision", "pass", "info", "No product decision boundary currently open", "No product-decision signal is currently open.", "Continue monitoring decision signals."))
    return checks


def build_work_program_tpm_function_readiness(readiness: pd.DataFrame, forecast_summary: pd.DataFrame, facts: dict[str, Any]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    total_count = int_value(facts.get("total_count"))
    standup_section_count = int_value(facts.get("standup_section_count"))
    forecast_ready = forecast_effective_eta_ready(forecast_summary)
    precision_ready = metric_text(readiness, "ready_to_measure_precision").lower() == "true"
    actionability_ready = metric_text(readiness, "ready_to_measure_actionability").lower() == "true"
    quality_signals = int_value(metric_text(readiness, "evaluation_label_row_count"))
    product_quality = product_action_measurement_quality(readiness, facts)
    product_quality_signals = int_value(product_quality.get("measurement_label_count"))
    product_precision_ready = (
        bool(product_quality.get("precision_ready"))
        and safe_float(product_quality.get("precision_rate")) >= MIN_PRECISION_RATE_FOR_PRODUCT_ACTION
        and safe_float(product_quality.get("useful_signal_rate")) >= MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION
    )
    product_actionability_ready = (
        bool(product_quality.get("actionability_ready"))
        and safe_float(product_quality.get("actionability_rate")) >= MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION
    )
    product_scope_kinds = product_quality.get("scope_kinds") or []
    source_limited_count = int_value(facts.get("source_coverage_limited_count"))
    overloaded_owner_count = int_value(facts.get("overloaded_owner_count"))
    unassigned_action_count = int_value(facts.get("unassigned_action_count"))
    owner_load_action_count = int_value(facts.get("owner_load_action_count"))
    active_blocker_count = int_value(facts.get("active_blocker_count"))
    active_blocker_impact_count = int_value(facts.get("active_blocker_impact_count"))
    needs_action_dependency_count = int_value(facts.get("needs_action_dependency_count"))

    brief_state = "automatable"
    brief_automation = "can_publish_operating_brief"
    brief_human_required = False
    brief_detail = "Typed program rows and standup sections are available for an operating brief."
    brief_action = "Publish the operating brief and keep the source sync fresh."
    if total_count == 0:
        brief_state = "blocked"
        brief_automation = "missing_typed_work"
        brief_human_required = True
        brief_detail = "No typed program rows are in scope."
        brief_action = "Load typed work rows before expecting an AI TPM operating brief."
    elif standup_section_count == 0:
        brief_state = "assisted"
        brief_automation = "summary_only"
        brief_detail = "Typed program rows are available, but no persisted standup sections are in scope."
        brief_action = "Persist standup sections to turn the summary into an agenda."
    rows.append(
        tpm_function_readiness(
            "operating_brief",
            "Operating brief",
            brief_state,
            brief_automation,
            brief_human_required,
            total_count + standup_section_count,
            [],
            brief_detail,
            brief_action,
        )
    )

    blocker_signals = active_blocker_count + active_blocker_impact_count + needs_action_dependency_count
    if blocker_signals > 0:
        blocker_gate_keys = ["blocker_clearance"] if active_blocker_count > 0 or active_blocker_impact_count > 0 else []
        if active_blocker_count > 0 or active_blocker_impact_count > 0:
            blocker_detail = f"{count_phrase(active_blocker_count, 'active blocker')} and {count_phrase(active_blocker_impact_count, 'active blocker impact')} need owner-confirmed clearance."
        else:
            blocker_detail = f"{count_phrase(needs_action_dependency_count, 'dependency action')} need owner follow-through."
        rows.append(
            tpm_function_readiness(
                "blocker_management",
                "Blocker management",
                "supervised",
                "can_rank_and_draft",
                True,
                blocker_signals,
                blocker_gate_keys,
                blocker_detail,
                "Use ranked blockers and dependencies to drive owner decisions, but require human confirmation before declaring clearance.",
            )
        )
    else:
        rows.append(
            tpm_function_readiness(
                "blocker_management",
                "Blocker management",
                "automatable",
                "watch_ready",
                False,
                0,
                [],
                "No active blocker or blocker impact is in scope.",
                "Maintain blocker watch on the next source sync.",
            )
        )

    if forecast_ready:
        rows.append(
            tpm_function_readiness(
                "forecast_triage",
                "Forecast triage",
                "automatable",
                "eta_ready",
                False,
                int_value(facts.get("forecast_signal_count")),
                [],
                "Forecast readiness gates passed for ETA-style output.",
                "Continue backtesting forecast outcomes against observed transitions.",
            )
        )
    else:
        rows.append(
            tpm_function_readiness(
                "forecast_triage",
                "Forecast triage",
                "blocked",
                "risk_triage_only",
                True,
                int_value(facts.get("forecast_signal_count")),
                ["forecast_readiness"],
                "Forecast output is useful for risk ranking, not ETA commitments.",
                "Use forecast rows as TPM risk leads until readiness gates pass.",
            )
        )

    if overloaded_owner_count > 0 or unassigned_action_count > 0:
        rows.append(
            tpm_function_readiness(
                "execution_capacity",
                "Execution capacity",
                "blocked",
                "rebalance_required",
                True,
                owner_load_action_count,
                ["owner_load"],
                f"{count_phrase(overloaded_owner_count, 'overloaded owner')} and {count_phrase(unassigned_action_count, 'unassigned product action')} constrain execution.",
                "Rebalance overloaded product-action queues or assign unowned product actions before treating the plan as autonomously executable.",
            )
        )
    elif owner_load_action_count > 0:
        rows.append(
            tpm_function_readiness(
                "execution_capacity",
                "Execution capacity",
                "assisted",
                "owner_queue_ready",
                False,
                owner_load_action_count,
                [],
                "Owner-load rows are present with no overloaded owners or unassigned product actions.",
                "Use owner queues to follow through on open TPM actions.",
            )
        )
    else:
        rows.append(
            tpm_function_readiness(
                "execution_capacity",
                "Execution capacity",
                "watch",
                "no_open_owner_load",
                False,
                0,
                [],
                "No owner-load work is currently open.",
                "Refresh owner-load snapshots on the next action generation run.",
            )
        )

    if source_limited_count > 0:
        rows.append(
            tpm_function_readiness(
                "source_coverage",
                "Source coverage",
                "blocked",
                "coverage_repair_required",
                True,
                source_limited_count,
                ["source_coverage"],
                f"{count_phrase(source_limited_count, 'program item')} {has_have(source_limited_count)} limited source coverage.",
                "Repair source coverage before using affected rows for absence claims or autonomous decisions.",
            )
        )
    else:
        rows.append(
            tpm_function_readiness(
                "source_coverage",
                "Source coverage",
                "automatable",
                "coverage_ready",
                False,
                total_count,
                [],
                "No typed program item in scope reports limited source coverage.",
                "Keep preserving source coverage on every sync.",
            )
        )

    if product_precision_ready and product_actionability_ready:
        rows.append(
            tpm_function_readiness(
                "insight_quality",
                "Insight QA",
                "automatable",
                "measurement_ready",
                False,
                product_quality_signals,
                [],
                "Product-action insight precision and actionability have enough labels and meet thresholds.",
                "Use measured product-action quality gates to decide which signals can become product actions; keep global/context labels as validation coverage.",
            )
        )
    else:
        blocking_gates = []
        if product_scope_kinds:
            if not product_precision_ready:
                blocking_gates.append("measurement_precision")
            if not product_actionability_ready:
                blocking_gates.append("measurement_actionability")
        elif precision_ready and actionability_ready:
            blocking_gates.extend(["measurement_precision", "measurement_actionability"])
        else:
            if not precision_ready:
                blocking_gates.append(GLOBAL_INSIGHT_PRECISION_KEY)
            if not actionability_ready:
                blocking_gates.append(GLOBAL_INSIGHT_ACTIONABILITY_KEY)
        if precision_ready and actionability_ready and not product_scope_kinds:
            readiness_state = "assisted"
            automation_state = "validation_only"
            detail = "Global/context insight quality is measured, but no product-action insight kind is measurement-ready."
            recommended_action = "Use context labels for validation and routing only; add product-action scoped labels before promoting generated insight quality to autonomous actions."
        elif precision_ready and actionability_ready:
            readiness_state = "blocked"
            automation_state = "product_action_labels_required"
            detail = "Global insight quality is measured, but product-action insight quality is not ready for automation."
            recommended_action = "Add or improve product-action truth/actionability labels before allowing insight quality to promote autonomous product actions."
        else:
            readiness_state = "blocked"
            automation_state = "labels_required"
            detail = "Global insight quality is not fully measurement-ready for TPM replacement."
            recommended_action = "Add gold truth/actionability labels for validation/context leads before claiming autonomous TPM replacement."
        rows.append(
            tpm_function_readiness(
                "insight_quality",
                "Insight QA",
                readiness_state,
                automation_state,
                True,
                quality_signals,
                blocking_gates,
                detail,
                recommended_action,
            )
        )

    decision_signals = int_value(facts.get("product_action_count")) + int_value(facts.get("needs_decision_count"))
    if decision_signals > 0:
        rows.append(
            tpm_function_readiness(
                "product_decisions",
                "Product decisions",
                "supervised",
                "human_decision_required",
                True,
                decision_signals,
                [],
                f"{count_phrase(decision_signals, 'product decision signal')} require owner judgment.",
                "Draft the decision request, but require a human owner to merge, close, park, or reassign.",
            )
        )
    else:
        rows.append(
            tpm_function_readiness(
                "product_decisions",
                "Product decisions",
                "assisted",
                "no_open_product_decisions",
                False,
                0,
                [],
                "No product-decision action is currently open.",
                "Continue monitoring for new decision or owner follow-up signals.",
            )
        )
    return rows


def build_work_program_evidence_needs(readiness: pd.DataFrame, forecast_summary: pd.DataFrame, facts: dict[str, Any]) -> list[dict[str, Any]]:
    needs: list[dict[str, Any]] = []
    forecast_ready = forecast_effective_eta_ready(forecast_summary)
    if not forecast_ready:
        needs.append(
            evidence_need(
                "forecast_readiness:backtest",
                "forecast_readiness",
                "forecast_backtest",
                "high",
                "workstream",
                "flink-kubernetes-operator",
                "",
                0,
                1,
                1,
                None,
                None,
                "Produce a passing forecast backtest before using ETA commitments.",
            )
        )
        forecast_risk_work = int_value(facts.get("forecast_risk_target_count"))
        if forecast_risk_work > 0:
            needs.append(
                evidence_need(
                    "forecast_readiness:risk_triage",
                    "forecast_readiness",
                    "forecast_risk_triage",
                    "high",
                    "workstream",
                    "flink-kubernetes-operator",
                    "risk_triage",
                    0,
                    forecast_risk_work,
                    forecast_risk_work,
                    None,
                    None,
                    "Use high-risk forecast rows as owner-status triage only; do not present them as ETA commitments until backtest readiness passes.",
                )
            )
            for target in facts.get("forecast_risk_targets") or []:
                needs.append(forecast_risk_evidence_need(target))

    kind_readiness = kind_readiness_map(readiness)
    product_quality = product_action_measurement_quality(readiness, facts)
    product_scope_kinds = product_quality.get("scope_kinds") or []
    product_scope = ", ".join(str(kind) for kind in product_scope_kinds) if product_scope_kinds else "no product-action insight kinds"
    product_precision_ready = (
        bool(product_quality.get("precision_ready"))
        and safe_float(product_quality.get("precision_rate")) >= MIN_PRECISION_RATE_FOR_PRODUCT_ACTION
        and safe_float(product_quality.get("useful_signal_rate")) >= MIN_USEFUL_SIGNAL_RATE_FOR_PRODUCT_ACTION
    )
    if not product_precision_ready:
        required = max(MIN_MEASUREMENT_LABEL_TOTAL, int_value(product_quality.get("open_review_request_count")))
        current = int_value(product_quality.get("measurement_label_count"))
        needs.append(
            evidence_need(
                "measurement_precision:product_action",
                "measurement_precision",
                "product_action_precision_labels",
                "high",
                "workstream",
                "flink-kubernetes-operator",
                product_scope,
                current,
                required,
                max(0, required - current),
                safe_float(product_quality.get("precision_rate")),
                MIN_PRECISION_RATE_FOR_PRODUCT_ACTION,
                "Gold-label product-action-backed insight kinds before claiming autonomous TPM replacement.",
            )
        )
    product_actionability_ready = bool(product_quality.get("actionability_ready")) and safe_float(product_quality.get("actionability_rate")) >= MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION
    if not product_actionability_ready:
        required = max(MIN_MEASUREMENT_LABEL_TOTAL, int_value(product_quality.get("open_review_request_count")))
        current = int_value(product_quality.get("measurement_label_count"))
        needs.append(
            evidence_need(
                "measurement_actionability:product_action",
                "measurement_actionability",
                "product_action_actionability_labels",
                "high",
                "workstream",
                "flink-kubernetes-operator",
                product_scope,
                current,
                required,
                max(0, required - current),
                safe_float(product_quality.get("actionability_rate")),
                MIN_ACTIONABILITY_RATE_FOR_PRODUCT_ACTION,
                "Add actionability labels for product-action-backed insight kinds before allowing generated leads to drive autonomous TPM action.",
            )
        )

    if not kind_readiness:
        precision_ready = metric_text(readiness, "ready_to_measure_precision").lower() == "true"
        actionability_ready = metric_text(readiness, "ready_to_measure_actionability").lower() == "true"
        if not precision_ready:
            needs.append(
                evidence_need(
                    f"{GLOBAL_INSIGHT_PRECISION_KEY}:labels",
                    GLOBAL_INSIGHT_PRECISION_KEY,
                    "insight_labels",
                    "high",
                    "insight_kind",
                    "",
                    "",
                    0,
                    MIN_MEASUREMENT_LABEL_TOTAL,
                    MIN_MEASUREMENT_LABEL_TOTAL,
                    None,
                    None,
                    "Gold-label current generated insights before claiming autonomous TPM replacement.",
                )
            )
        if not actionability_ready:
            needs.append(
                evidence_need(
                    f"{GLOBAL_INSIGHT_ACTIONABILITY_KEY}:labels",
                    GLOBAL_INSIGHT_ACTIONABILITY_KEY,
                    "actionability_labels",
                    "high",
                    "insight_kind",
                    "",
                    "",
                    0,
                    MIN_MEASUREMENT_LABEL_TOTAL,
                    MIN_MEASUREMENT_LABEL_TOTAL,
                    None,
                    None,
                    "Add actionability labels for current generated insights before allowing generated leads to drive autonomous TPM action.",
                )
            )
    else:
        for insight_kind, row in sorted(kind_readiness.items()):
            required = int_value(row.get("required"))
            truth_current = int_value(row.get("truth_labeled"))
            actionability_current = int_value(row.get("actionability_labeled"))
            if truth_current < required:
                needs.append(
                    evidence_need(
                        f"measurement_labels:{insight_kind}",
                        GLOBAL_INSIGHT_PRECISION_KEY,
                        "insight_labels",
                        "high",
                        "insight_kind",
                        insight_kind,
                        "",
                        truth_current,
                        required,
                        max(0, required - truth_current),
                        None,
                        None,
                        f"Add truth labels for {insight_kind} before using this signal for autonomous TPM replacement.",
                    )
                )
            if actionability_current < required:
                needs.append(
                    evidence_need(
                        f"measurement_actionability:{insight_kind}",
                        GLOBAL_INSIGHT_ACTIONABILITY_KEY,
                        "actionability_labels",
                        "high",
                        "insight_kind",
                        insight_kind,
                        "",
                        actionability_current,
                        required,
                        max(0, required - actionability_current),
                        None,
                        None,
                        f"Add actionability labels for {insight_kind} before allowing this signal to drive autonomous TPM action.",
                    )
                )
            if not bool(row.get("ready")):
                for target in facts.get("measurement_label_targets") or []:
                    if first_nonempty([target.get("insight_kind")]) == insight_kind:
                        needs.append(measurement_label_evidence_need(target, required))

    for limit_kind, count in sorted(dict(facts.get("source_coverage_limit_counts") or {}).items()):
        needs.append(source_coverage_evidence_need(limit_kind, int_value(count)))
        for target in facts.get("source_coverage_targets") or []:
            if ontology_program_item_coverage_limit_kind(target) == limit_kind:
                needs.append(source_coverage_item_evidence_need(target))

    for limit_kind, count in sorted(dict(facts.get("auth_limited_observation_counts") or {}).items()):
        needs.append(source_coverage_evidence_need(limit_kind, int_value(count)))
        for target in facts.get("auth_limited_observation_targets") or []:
            if ontology_program_item_coverage_limit_kind(target) == limit_kind:
                needs.append(source_coverage_item_evidence_need(target))

    for limit_kind, count in sorted(dict(facts.get("generated_claim_limit_counts") or {}).items()):
        needs.append(source_coverage_evidence_need(limit_kind, int_value(count)))
        for target in facts.get("generated_claim_limited_targets") or []:
            if ontology_program_item_coverage_limit_kind(target) == limit_kind:
                needs.append(source_coverage_item_evidence_need(target))

    owner_load_work = int_value(facts.get("overloaded_owner_count")) + int_value(facts.get("unassigned_action_count"))
    if owner_load_work > 0:
        needs.append(
            evidence_need(
                "owner_load:rebalance",
                "owner_load",
                "owner_load_balancing",
                "high",
                "workstream",
                "flink-kubernetes-operator",
                first_nonempty([facts.get("owner_load_status")]),
                owner_load_work,
                0,
                owner_load_work,
                None,
                None,
                "Rebalance overloaded product-action queues or assign unassigned product actions before treating the plan as autonomously executable.",
            )
        )
        for target in facts.get("owner_load_targets") or []:
            needs.append(owner_load_evidence_need(target))

    active_blocker_work = int_value(facts.get("active_blocker_count")) + int_value(facts.get("active_blocker_impact_count"))
    if active_blocker_work > 0:
        needs.append(
            evidence_need(
                "blocker_clearance:active",
                "blocker_clearance",
                "blocker_clearance",
                "critical",
                "workstream",
                "flink-kubernetes-operator",
                "",
                0,
                active_blocker_work,
                active_blocker_work,
                None,
                None,
                "Assign owners and capture blocker-clearance evidence for active blockers and impacts.",
            )
        )
        for target in facts.get("active_blocker_targets") or []:
            needs.append(blocker_clearance_evidence_need(target))

    product_decision_work = product_decision_signal_count(facts)
    if product_decision_work > 0:
        needs.append(
            evidence_need(
                "product_decision:human_review",
                "product_decision",
                "human_decision",
                "high",
                "workstream",
                "flink-kubernetes-operator",
                "product_decision",
                0,
                product_decision_work,
                product_decision_work,
                None,
                None,
                "Draft decision requests for owner judgment, but do not automate merge, close, park, or reassignment decisions.",
            )
        )
        for target in facts.get("product_decision_targets") or []:
            needs.append(product_decision_evidence_need(target))

    dependency_action_work = int_value(facts.get("needs_action_dependency_count"))
    if dependency_action_work > 0:
        needs.append(
            evidence_need(
                "dependency_pressure:needs_action",
                "dependency_pressure",
                "dependency_action",
                "high",
                "workstream",
                "flink-kubernetes-operator",
                "needs_action",
                0,
                dependency_action_work,
                dependency_action_work,
                None,
                None,
                "Drive needs-action dependency edges to open action completion, or mark stale edges when the linked action is already closed.",
            )
        )
        for target in facts.get("dependency_action_targets") or []:
            needs.append(dependency_action_evidence_need(target))

    for need in needs:
        backing, state, next_step = evidence_need_execution(facts, need)
        need["backing_action_count"] = backing
        need["execution_state"] = state
        need["next_execution_step"] = next_step
    return sorted(needs, key=lambda need: (evidence_priority_rank(first_nonempty([need.get("priority")])), first_nonempty([need.get("key")])))


def blocker_clearance_evidence_need(target: dict[str, Any]) -> dict[str, Any]:
    subject_kind = first_nonempty([target.get("subject_kind")]) or "unknown"
    subject_key = first_nonempty([target.get("subject_key")])
    owner_key = first_nonempty([target.get("owner_key"), target.get("action_owner_key")]) or "unassigned owner"
    severity = first_nonempty([target.get("severity")]) or "critical"
    priority = severity if severity in SEVERITY_RANK else "critical"
    impact_count = int_value(target.get("active_impact_count"))
    title = first_nonempty([target.get("title")]) or f"Active blocker: {subject_key}"
    action_key = first_nonempty([target.get("action_key")])
    action_state = first_nonempty([target.get("action_state")])
    source_url = first_nonempty([target.get("action_source_url"), target.get("source_url")])
    recommended_action = f"Confirm clearance state for {subject_key} with {owner_key}; record owner-confirmed cleared, accepted, or false-positive evidence before declaring this blocker clear."
    need = evidence_need(
        f"blocker_clearance:{subject_kind}:{subject_key}",
        "blocker_clearance",
        "blocker_clearance",
        priority,
        subject_kind,
        subject_key,
        first_nonempty([target.get("blocker_kind")]) or "source_signal",
        0,
        1,
        1,
        None,
        None,
        recommended_action,
    )
    need["blocker_title"] = title
    need["owner_key"] = owner_key
    need["action_key"] = action_key
    need["action_state"] = action_state
    need["source_url"] = source_url
    need["active_impact_count"] = impact_count
    need["backing_action_count_hint"] = 1 if int_value(target.get("work_action_id")) > 0 else 0
    return need


def owner_load_evidence_need(target: dict[str, Any]) -> dict[str, Any]:
    owner_key = first_nonempty([target.get("owner_key")]) or "(unassigned)"
    load_status = first_nonempty([target.get("load_status")]) or "attention_required"
    action_count = int_value(target.get("action_count"))
    top_subjects = first_nonempty([target.get("top_subjects")])
    top_action_type = first_nonempty([target.get("top_action_type")])
    source_url = first_nonempty([target.get("source_url")])
    if owner_key == "(unassigned)":
        recommended_action = f"Assign an accountable owner for {count_phrase(action_count, 'unassigned product action')}"
        if top_subjects:
            recommended_action += f": {top_subjects}"
        recommended_action += "."
    else:
        recommended_action = f"Rebalance {count_phrase(action_count, 'open action')} for {owner_key}, or record why this owner concentration is accepted"
        if top_subjects:
            recommended_action += f": {top_subjects}"
        recommended_action += "."
    need = evidence_need(
        f"owner_load:owner:{owner_key}",
        "owner_load",
        "owner_load_balancing",
        "high",
        "owner",
        owner_key,
        load_status,
        action_count,
        0,
        action_count,
        None,
        None,
        recommended_action,
    )
    need["load_status"] = load_status
    need["owner_key"] = owner_key
    need["top_subjects"] = top_subjects
    need["top_action_type"] = top_action_type
    need["recommended_focus"] = first_nonempty([target.get("recommended_focus")])
    need["source_url"] = source_url
    need["backing_action_count_hint"] = action_count
    return need


def measurement_label_evidence_need(target: dict[str, Any], required_for_kind: int) -> dict[str, Any]:
    insight_kind = first_nonempty([target.get("insight_kind")]) or "unknown"
    subject_kind = first_nonempty([target.get("subject_kind")]) or "unknown"
    subject_key = first_nonempty([target.get("subject_key")])
    title = first_nonempty([target.get("title")]) or f"{insight_kind} label target"
    source_url = first_nonempty([target.get("source_url")])
    recommended_action = f"Gold-label truth and actionability for {insight_kind} on {subject_key}; keep this as validation evidence until {required_for_kind} labels exist for the kind."
    need = evidence_need(
        f"measurement_labels:{insight_kind}:{subject_key}",
        GLOBAL_INSIGHT_PRECISION_KEY,
        "insight_labels",
        "high",
        subject_kind,
        subject_key,
        insight_kind,
        0,
        1,
        1,
        None,
        None,
        recommended_action,
    )
    need["insight_kind"] = insight_kind
    need["insight_title"] = title
    need["source_url"] = source_url
    need["review_request_count_hint"] = int_value(target.get("review_request_count"))
    need["score_hint"] = safe_float(target.get("score"))
    return need


def forecast_risk_evidence_need(target: dict[str, Any]) -> dict[str, Any]:
    subject_kind = first_nonempty([target.get("subject_kind")]) or "unknown"
    subject_key = first_nonempty([target.get("subject_key")])
    risk_band = first_nonempty([target.get("risk_band")]) or "risk_triage"
    risk_score = safe_float(target.get("risk_score"))
    overdue_days = safe_float(target.get("overdue_days"))
    predicted_remaining_days = safe_float(target.get("predicted_remaining_days"))
    owner_key = first_nonempty([target.get("action_owner_key")]) or "the accountable owner"
    source_url = first_nonempty([target.get("action_source_url"), target.get("source_url")])
    recommended_action = f"Ask {owner_key} for merge, close, or parking status on {subject_key}; treat the forecast as risk triage, not an ETA commitment."
    need = evidence_need(
        f"forecast_readiness:risk_triage:{subject_key}",
        "forecast_readiness",
        "forecast_risk_triage",
        "high",
        subject_kind,
        subject_key,
        risk_band,
        0,
        1,
        1,
        None,
        None,
        recommended_action,
    )
    need["risk_band"] = risk_band
    need["risk_score_hint"] = risk_score
    need["overdue_days_hint"] = overdue_days
    need["predicted_remaining_days_hint"] = predicted_remaining_days
    need["readiness_state"] = first_nonempty([target.get("readiness_state")])
    need["ready_for_eta"] = bool(target.get("ready_for_eta"))
    need["readiness_reason"] = first_nonempty([target.get("readiness_reason")])
    need["action_key"] = first_nonempty([target.get("action_key")])
    need["action_state"] = first_nonempty([target.get("action_state")])
    need["owner_key"] = owner_key
    need["source_url"] = source_url
    need["backing_action_count_hint"] = 1 if int_value(target.get("work_action_id")) > 0 and first_nonempty([target.get("action_state")]) != "closed" else 0
    return need


def product_decision_evidence_need(target: dict[str, Any]) -> dict[str, Any]:
    subject_kind = first_nonempty([target.get("subject_kind")]) or "unknown"
    subject_key = first_nonempty([target.get("subject_key")])
    decision_state = first_nonempty([target.get("decision_state")]) or "product_action"
    program_status = first_nonempty([target.get("program_status")])
    owner_key = first_nonempty([target.get("owner_key"), target.get("action_owner_key")]) or "the accountable owner"
    action_key = first_nonempty([target.get("action_key")])
    action_state = first_nonempty([target.get("action_state")])
    source_url = first_nonempty([target.get("action_source_url"), target.get("source_url")])
    title = first_nonempty([target.get("title")]) or f"Product decision: {subject_key}"
    recommended_action = f"Draft an owner decision request for {subject_key}; require {owner_key} to choose merge, close, park, or reassign before changing product state."
    need = evidence_need(
        f"product_decision:{subject_kind}:{subject_key}",
        "product_decision",
        "human_decision",
        "high",
        subject_kind,
        subject_key,
        first_nonempty([decision_state, program_status]),
        0,
        1,
        1,
        None,
        None,
        recommended_action,
    )
    need["owner_key"] = owner_key
    need["action_key"] = action_key
    need["action_state"] = action_state
    need["program_status"] = program_status
    need["decision_state"] = decision_state
    need["source_url"] = source_url
    need["decision_title"] = title
    need["backing_action_count_hint"] = 1 if int_value(target.get("work_action_id")) > 0 else 0
    return need


def dependency_action_evidence_need(target: dict[str, Any]) -> dict[str, Any]:
    edge_key = first_nonempty([target.get("key")])
    action_key = first_nonempty([target.get("action_key"), target.get("to_key")])
    action_state = first_nonempty([target.get("action_state")]) or "unknown"
    action_type = first_nonempty([target.get("action_type"), target.get("risk_signal")]) or "dependency_action"
    subject_kind = first_nonempty([target.get("action_subject_kind"), target.get("to_kind")]) or "dependency_edge"
    subject_key = first_nonempty([target.get("action_subject_key"), target.get("to_key"), edge_key])
    owner_key = first_nonempty([target.get("action_owner_key")]) or "the accountable owner"
    source_url = first_nonempty([target.get("action_source_url"), target.get("source_url")])
    risk_signal = first_nonempty([target.get("risk_signal")]) or "needs_action"
    if action_state == "closed":
        recommended_action = f"Review stale needs-action dependency for {subject_key}; linked action {action_key} is closed, so either remove the edge or reopen work with owner evidence."
    else:
        recommended_action = f"Drive linked dependency action {action_key} for {subject_key} with {owner_key}; record completion or owner acceptance before treating the dependency as clear."
    need = evidence_need(
        f"dependency_pressure:{edge_key or action_key or subject_key}",
        "dependency_pressure",
        "dependency_action",
        "high",
        subject_kind,
        subject_key,
        risk_signal,
        0,
        1,
        1,
        None,
        None,
        recommended_action,
    )
    need["dependency_edge_key"] = edge_key
    need["from_kind"] = first_nonempty([target.get("from_kind")])
    need["from_key"] = first_nonempty([target.get("from_key")])
    need["to_kind"] = first_nonempty([target.get("to_kind")])
    need["to_key"] = first_nonempty([target.get("to_key")])
    need["action_key"] = action_key
    need["action_state"] = action_state
    need["action_type"] = action_type
    need["owner_key"] = owner_key
    need["source_url"] = source_url
    need["backing_action_count_hint"] = 1 if int_value(target.get("work_action_id")) > 0 and action_state != "closed" else 0
    return need


def source_coverage_item_evidence_need(target: dict[str, Any]) -> dict[str, Any]:
    limit_kind = ontology_program_item_coverage_limit_kind(target)
    evidence_kind, _ = source_coverage_kind_details(limit_kind)
    gate_key = source_coverage_limit_gate_key(limit_kind)
    subject_kind = first_nonempty([target.get("subject_kind")]) or "unknown"
    subject_key = first_nonempty([target.get("subject_key")])
    owner_key = first_nonempty([target.get("owner_key"), target.get("action_owner_key")])
    action_key = first_nonempty([target.get("action_key")])
    action_state = first_nonempty([target.get("action_state")])
    source_url = first_nonempty([target.get("action_source_url"), target.get("source_url")])
    title = first_nonempty([target.get("title")]) or subject_key or "source coverage target"
    if limit_kind == "anonymous_observation":
        recommended_action = f"Re-observe {subject_key} with authenticated source access before using it for absence, completion, or autonomous decision claims."
    elif limit_kind == "required_check_coverage_unavailable":
        recommended_action = f"Capture branch protection or required-check configuration for {subject_key} before treating CI follow-ups as complete."
    elif limit_kind == "generated_evidence":
        recommended_action = f"QA generated or derived claim evidence for {subject_key}; keep it out of source repair and absence claims."
    elif limit_kind in {"source_failure", "source_repair_needed"}:
        recommended_action = f"Refresh failed or repair-needed source coverage for {subject_key} before promoting this item."
    elif limit_kind == "not_observed":
        recommended_action = f"Observe current source state for {subject_key} before using this item for absence claims or autonomous decisions."
    else:
        recommended_action = f"Refresh source coverage for {subject_key} before using this item for absence claims or autonomous decisions."
    need = evidence_need(
        f"{gate_key}:{limit_kind}:{subject_key}",
        gate_key,
        evidence_kind,
        "medium",
        subject_kind,
        subject_key,
        limit_kind,
        0,
        1,
        1,
        None,
        None,
        recommended_action,
    )
    need["source_coverage_state"] = first_nonempty([target.get("source_coverage_state")])
    need["freshness_state"] = first_nonempty([target.get("freshness_state")])
    need["program_status"] = first_nonempty([target.get("program_status")])
    need["decision_state"] = first_nonempty([target.get("decision_state")])
    need["owner_key"] = owner_key
    need["action_key"] = action_key
    need["action_state"] = action_state
    need["source_url"] = source_url
    need["source_target_title"] = title
    need["backing_action_count_hint"] = 1 if int_value(target.get("work_action_id")) > 0 else 0
    return need


def evidence_need(
    key: str,
    gate_key: str,
    evidence_kind: str,
    priority: str,
    target_kind: str,
    target_key: str,
    metric_key: str,
    current_count: int,
    required_count: int,
    missing_count: int,
    current_rate: float | None,
    required_rate: float | None,
    recommended_action: str,
) -> dict[str, Any]:
    return {
        "key": key,
        "gate_key": gate_key,
        "evidence_kind": evidence_kind,
        "priority": priority,
        "target_kind": target_kind,
        "target_key": target_key,
        "metric_key": metric_key,
        "execution_state": "unknown",
        "backing_action_count": 0,
        "current_count": current_count,
        "required_count": required_count,
        "missing_count": missing_count,
        "current_rate": current_rate,
        "required_rate": required_rate,
        "recommended_action": recommended_action,
        "next_execution_step": recommended_action,
    }


def source_coverage_kind_details(limit_kind: str) -> tuple[str, str]:
    evidence_kind = "source_coverage"
    recommended_action = "Refresh limited source rows before using affected items for absence claims or autonomous decisions."
    if limit_kind == "anonymous_observation":
        evidence_kind = "source_authentication"
        recommended_action = "Re-observe anonymous rows with authenticated source access, or keep them as lower-confidence review leads."
    elif limit_kind == "required_check_coverage_unavailable":
        evidence_kind = "required_check_coverage"
        recommended_action = "Capture branch protection or required-check configuration before promoting CI follow-ups."
    elif limit_kind == "generated_evidence":
        evidence_kind = "generated_evidence"
        recommended_action = "Keep generated or derived claim evidence in QA/provenance review, not source repair."
    elif limit_kind == "source_failure":
        recommended_action = "Refresh failed source rows before using affected items for absence claims or autonomous decisions."
    elif limit_kind == "source_repair_needed":
        recommended_action = "Run source-repair actions for rows already classified as repair needed."
    elif limit_kind == "not_observed":
        recommended_action = "Observe current source state before using affected items for absence claims or autonomous decisions."
    elif limit_kind in {"unknown_source_coverage", "source_unavailable"}:
        recommended_action = "Resolve unknown or unavailable source coverage before using affected items for absence claims or autonomous decisions."
    elif limit_kind == "partial_source_coverage":
        recommended_action = "Refresh partial source coverage before using affected items for absence claims or autonomous decisions."
    return evidence_kind, recommended_action


def source_coverage_limit_gate_key(limit_kind: str) -> str:
    if limit_kind == "anonymous_observation":
        return "source_authentication"
    if limit_kind == "generated_evidence":
        return "claim_provenance"
    return "source_coverage"


def source_coverage_evidence_need(limit_kind: str, count: int) -> dict[str, Any]:
    evidence_kind, recommended_action = source_coverage_kind_details(limit_kind)
    gate_key = source_coverage_limit_gate_key(limit_kind)
    return evidence_need(
        f"{gate_key}:{limit_kind}",
        gate_key,
        evidence_kind,
        "medium",
        "workstream",
        "flink-kubernetes-operator",
        limit_kind,
        0,
        count,
        count,
        None,
        None,
        recommended_action,
    )


def evidence_need_execution(facts: dict[str, Any], need: dict[str, Any]) -> tuple[int, str, str]:
    status_counts = dict(facts.get("program_status_counts") or {})
    decision_counts = dict(facts.get("decision_state_counts") or {})
    evidence_kind = first_nonempty([need.get("evidence_kind")])
    product_action_count = int_value(decision_counts.get("product_action"))
    validation_lead_count = int_value(decision_counts.get("validation_lead"))
    closure_candidate_count = int_value(status_counts.get("closure_candidate"))
    model_quality_count = int_value(status_counts.get("model_quality"))
    ci_failing_count = int_value(status_counts.get("ci_failing"))
    source_repair_count = int_value(status_counts.get("source_repair"))
    if evidence_kind == "blocker_clearance":
        if first_nonempty([need.get("target_kind")]) != "workstream" and first_nonempty([need.get("target_key")]):
            target_key = first_nonempty([need.get("target_key")])
            owner_key = first_nonempty([need.get("owner_key")]) or "the accountable owner"
            source_url = first_nonempty([need.get("source_url")])
            impact_count = int_value(need.get("active_impact_count"))
            impact_phrase = count_phrase(impact_count, "active impact") if impact_count > 0 else "its active impact"
            source_phrase = f" using {source_url}" if source_url else ""
            action_key = first_nonempty([need.get("action_key")]) or "the linked action"
            action_state = first_nonempty([need.get("action_state")])
            if action_state == "closed":
                return 0, "stale_blocker_action_review_needed", f"Review stale blocker-clearance action for {target_key}; linked action {action_key} is closed, so reopen it or record fresh owner clearance evidence{source_phrase}."
            next_step = f"Ask {owner_key} to confirm whether {target_key} is still blocked; record cleared, accepted-risk, or false-positive evidence{source_phrase} before clearing {impact_phrase}."
            backing = int_value(need.get("backing_action_count_hint"))
            if backing > 0:
                return backing, "action_open", next_step
            return 0, "missing_action", f"Create a blocker-clearance action for {target_key}, then {next_step[0].lower() + next_step[1:]}"
        backing = max(closure_candidate_count, product_action_count)
        if backing > 0:
            return backing, "actions_open", "Use open blocker-clearance product actions to capture owner decisions and clearance evidence."
        return 0, "missing_action", "Create blocker-clearance actions before expecting humans to close this evidence gap."
    if evidence_kind == "human_decision":
        if first_nonempty([need.get("target_kind")]) != "workstream" and first_nonempty([need.get("target_key")]):
            target_key = first_nonempty([need.get("target_key")])
            owner_key = first_nonempty([need.get("owner_key")]) or "the accountable owner"
            source_url = first_nonempty([need.get("source_url")])
            source_phrase = f" using {source_url}" if source_url else ""
            action_key = first_nonempty([need.get("action_key")]) or "the linked action"
            action_state = first_nonempty([need.get("action_state")])
            if action_state == "closed":
                return 0, "stale_decision_action_review_needed", f"Review stale product-decision action for {target_key}; linked action {action_key} is closed, so reopen it or record a fresh owner decision{source_phrase}."
            next_step = f"Draft a decision request for {target_key}; ask {owner_key} to choose merge, close, park, or reassign{source_phrase}, and keep execution human-approved."
            backing = int_value(need.get("backing_action_count_hint"))
            if backing > 0:
                return backing, "decision_action_open", next_step
            return 0, "missing_decision_action", f"Create a product-decision action for {target_key}, then {next_step[0].lower() + next_step[1:]}"
        backing = product_decision_signal_count(facts)
        if backing > 0:
            return backing, "decision_actions_open", "Use open product-decision actions to draft owner requests; do not automate merge, close, park, or reassignment decisions."
        return 0, "missing_decision_action", "Create product-decision actions before treating this human gate as executable."
    if evidence_kind == "dependency_action":
        if first_nonempty([need.get("target_kind")]) != "workstream" and first_nonempty([need.get("target_key")]):
            target_key = first_nonempty([need.get("target_key")])
            owner_key = first_nonempty([need.get("owner_key")]) or "the accountable owner"
            action_key = first_nonempty([need.get("action_key")]) or "the linked action"
            action_state = first_nonempty([need.get("action_state")])
            source_url = first_nonempty([need.get("source_url")])
            source_phrase = f" using {source_url}" if source_url else ""
            if action_state == "closed":
                return 0, "stale_dependency_review_needed", f"Review stale dependency edge for {target_key}; linked action {action_key} is closed, so remove the edge or reopen work with owner evidence{source_phrase}."
            next_step = f"Drive dependency action {action_key} for {target_key} with {owner_key}{source_phrase}; record completion, owner acceptance, or a new blocker before clearing the dependency."
            backing = int_value(need.get("backing_action_count_hint"))
            if backing > 0:
                return backing, "dependency_action_open", next_step
            return 0, "missing_dependency_action", f"Create or reconnect an open dependency action for {target_key}, then {next_step[0].lower() + next_step[1:]}"
        backing = int_value(facts.get("needs_action_dependency_count"))
        if backing > 0:
            return backing, "dependency_actions_open", "Use needs-action dependency edges to drive linked actions to completion, or mark stale edges whose actions are already closed."
        return 0, "missing_dependency_action", "Create dependency actions before treating dependency pressure as executable."
    if evidence_kind == "forecast_risk_triage":
        if first_nonempty([need.get("target_kind")]) != "workstream" and first_nonempty([need.get("target_key")]):
            target_key = first_nonempty([need.get("target_key")])
            owner_key = first_nonempty([need.get("owner_key")]) or "the accountable owner"
            risk_band = first_nonempty([need.get("risk_band"), need.get("metric_key")]) or "risk"
            overdue_days = safe_float(need.get("overdue_days_hint"))
            overdue_phrase = f" after {overdue_days:.1f} overdue days" if overdue_days > 0 else ""
            source_url = first_nonempty([need.get("source_url")])
            source_phrase = f" using {source_url}" if source_url else ""
            action_key = first_nonempty([need.get("action_key")]) or "the linked action"
            action_state = first_nonempty([need.get("action_state")])
            if action_state == "closed":
                return 0, "stale_forecast_action_review_needed", f"Review stale forecast triage action for {target_key}; linked action {action_key} is closed, so reopen it or record current owner status{source_phrase}."
            next_step = f"Ask {owner_key} for merge, close, or parking status on {target_key}{overdue_phrase}{source_phrase}; keep this as {risk_band} risk triage until the forecast backtest gate passes."
            backing = int_value(need.get("backing_action_count_hint"))
            if backing > 0:
                return backing, "risk_action_open", next_step
            return 0, "owner_status_needed", f"Create an owner-status action for {target_key}, then {next_step[0].lower() + next_step[1:]}"
        backing = int_value(facts.get("forecast_risk_target_count"))
        if backing > 0:
            return backing, "risk_triage_actions_open", "Use high-risk forecast rows as owner-status follow-ups; keep them out of ETA commitments until forecast readiness passes."
        return 0, "missing_risk_triage_action", "Create owner-status actions for high-risk forecast rows before treating forecast triage as executable."
    if evidence_kind in {"source_authentication", "required_check_coverage", "generated_evidence", "source_coverage"}:
        source_item_execution = source_coverage_item_execution(need)
        if source_item_execution is not None:
            return source_item_execution
    if evidence_kind == "forecast_backtest":
        if model_quality_count > 0:
            return model_quality_count, "actions_open", "Use open model-quality actions to improve and re-check forecast backtest readiness."
        return 0, "missing_action", "Create a model-quality action for forecast readiness before treating this as executable work."
    if evidence_kind in {"product_action_precision_labels", "product_action_actionability_labels"}:
        scope = first_nonempty([need.get("metric_key")])
        current = int_value(need.get("current_count"))
        required = int_value(need.get("required_count"))
        missing = int_value(need.get("missing_count"))
        if scope == "no product-action insight kinds":
            return 0, "product_action_measurement_scope_missing", "Define which insight kinds can back product actions, then collect truth and actionability labels for that scoped set."
        if current > 0:
            label_kind = "truth" if evidence_kind == "product_action_precision_labels" else "actionability"
            return current, "product_action_labels_incomplete", f"Add {label_kind} labels for product-action-backed insight kinds; {count_phrase(missing, 'label')} still needed before the {first_nonempty([need.get('gate_key')])} gate clears."
        return 0, "product_action_labels_missing", f"Create product-action measurement review requests and collect {count_phrase(required, 'label')} before clearing this gate."
    if evidence_kind in {"insight_labels", "insight_quality"}:
        if first_nonempty([need.get("target_kind")]) not in {"", "insight_kind"} and first_nonempty([need.get("target_key")]):
            insight_kind = first_nonempty([need.get("insight_kind"), need.get("metric_key")]) or "insight"
            target_key = first_nonempty([need.get("target_key")])
            source_url = first_nonempty([need.get("source_url")])
            source_phrase = f" using {source_url}" if source_url else ""
            next_step = f"Gold-label truth and actionability for {insight_kind} on {target_key}{source_phrase}; keep it out of product-action automation until the kind-level measurement gate passes."
            review_requests = int_value(need.get("review_request_count_hint"))
            if review_requests > 0:
                return review_requests, "review_request_open", next_step
            return 0, "missing_review_request", f"Create a review request, then {next_step[0].lower() + next_step[1:]}"
        if validation_lead_count > 0:
            return validation_lead_count, "validation_actions_open", "Use validation-lead actions to collect labels or suppress low-quality insight kinds."
        return 0, "missing_validation_action", "Create validation actions for this insight-kind evidence gap."
    if evidence_kind == "source_authentication":
        backing = validation_lead_count + product_action_count
        if backing > 0:
            return backing, "review_actions_open", "Use open review or product actions to re-observe anonymous rows with authenticated access, or keep them as lower-confidence leads."
        return 0, "auth_upgrade_needed", "Create authenticated re-observation or validation actions before treating anonymous observations as product-action evidence."
    if evidence_kind == "required_check_coverage":
        backing = max(ci_failing_count, validation_lead_count)
        if backing > 0:
            return backing, "configuration_actions_open", "Use CI-check validation actions to capture branch protection or required-check configuration evidence."
        return 0, "configuration_evidence_needed", "Capture branch protection or required-check configuration before treating CI follow-ups as complete."
    if evidence_kind == "generated_evidence":
        if model_quality_count > 0:
            return model_quality_count, "qa_actions_open", "Use model-quality actions to review generated forecast or model-quality evidence."
        return 0, "model_quality_action_needed", "Create a model-quality action before treating generated evidence as cleared source coverage."
    if evidence_kind == "source_coverage":
        if source_repair_count > 0:
            return source_repair_count, "actions_open", "Use source-repair actions to refresh limited rows and re-run coverage-sensitive claims."
        return 0, "missing_source_repair_action", "Create source-repair actions for limited coverage rows before treating the evidence gap as executable."
    if evidence_kind == "owner_load_balancing":
        if first_nonempty([need.get("target_kind")]) == "owner" and first_nonempty([need.get("target_key")]):
            owner_key = first_nonempty([need.get("target_key")])
            action_count = int_value(need.get("backing_action_count_hint")) or int_value(need.get("missing_count"))
            top_subjects = first_nonempty([need.get("top_subjects")])
            source_url = first_nonempty([need.get("source_url")])
            source_phrase = f" from {source_url}" if source_url else ""
            subject_phrase = f" Top subjects: {top_subjects}." if top_subjects else ""
            if owner_key == "(unassigned)":
                return action_count, "assignment_needed", f"Assign owners for {count_phrase(action_count, 'unassigned product action')}{source_phrase}.{subject_phrase}"
            return action_count, "owner_queue_overloaded", f"Rebalance {count_phrase(action_count, 'open action')} for {owner_key}, split work, or record accepted capacity{source_phrase}.{subject_phrase}"
        if int_value(facts.get("owner_load_row_count")) > 0:
            return int_value(facts.get("owner_load_action_count")), "owner_load_rows_open", "Use latest owner-load rows to rebalance overloaded product-action queues, assign unowned product actions, or record why the owner concentration is accepted."
        return 0, "missing_owner_load_snapshot", "Refresh owner-load snapshots before deciding whether the action plan is executable."
    return 0, "unknown", first_nonempty([need.get("recommended_action")])


def source_coverage_item_execution(need: dict[str, Any]) -> tuple[int, str, str] | None:
    target_kind = first_nonempty([need.get("target_kind")])
    target_key = first_nonempty([need.get("target_key")])
    if not target_key or target_kind == "workstream":
        return None
    evidence_kind = first_nonempty([need.get("evidence_kind")])
    source_url = first_nonempty([need.get("source_url")])
    source_phrase = f" using {source_url}" if source_url else ""
    action_key = first_nonempty([need.get("action_key")]) or "the linked action"
    action_state = first_nonempty([need.get("action_state")])
    if action_state == "closed":
        return 0, "stale_source_action_review_needed", f"Review stale source-coverage action for {target_key}; linked action {action_key} is closed, so reopen it or attach fresh coverage evidence{source_phrase}."
    backing = int_value(need.get("backing_action_count_hint"))
    if evidence_kind == "source_authentication":
        next_step = f"Re-observe {target_key} with authenticated source access{source_phrase}; keep lower-confidence observations out of product-action automation until refreshed."
        if backing > 0:
            return backing, "action_open", next_step
        return 0, "auth_reobserve_needed", f"Create an authenticated re-observation action, then {next_step[0].lower() + next_step[1:]}"
    if evidence_kind == "required_check_coverage":
        next_step = f"Capture branch protection or required-check configuration for {target_key}{source_phrase}; do not claim CI follow-up completion until this is attached."
        if backing > 0:
            return backing, "configuration_action_open", next_step
        return 0, "configuration_evidence_needed", f"Create a required-check coverage action, then {next_step[0].lower() + next_step[1:]}"
    if evidence_kind == "generated_evidence":
        next_step = f"QA generated or derived claim evidence for {target_key}{source_phrase}; keep it as validation/provenance evidence, not source-repair proof."
        if backing > 0:
            return backing, "qa_action_open", next_step
        return 0, "qa_review_needed", f"Create a claim-provenance QA action, then {next_step[0].lower() + next_step[1:]}"
    if evidence_kind == "source_coverage":
        next_step = f"Refresh source coverage for {target_key}{source_phrase}; rerun coverage-sensitive claims after the item has fresh source evidence."
        if backing > 0:
            return backing, "source_repair_action_open", next_step
        return 0, "source_repair_needed", f"Create a source-repair action, then {next_step[0].lower() + next_step[1:]}"
    return None


def product_decision_signal_count(facts: dict[str, Any]) -> int:
    explicit = int_value(facts.get("product_action_count")) + int_value(facts.get("needs_decision_count"))
    if explicit > 0:
        return explicit
    status_counts = dict(facts.get("program_status_counts") or {})
    decision_counts = dict(facts.get("decision_state_counts") or {})
    return int_value(decision_counts.get("product_action")) + int_value(status_counts.get("needs_decision"))


def ontology_forecast_risk_targets(
    conn: sqlite3.Connection,
    source_instance: str,
    limit: int = 10,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_item_forecasts"):
        return []
    forecast_columns = table_columns(conn, "work_item_forecasts")
    required_forecast_columns = {
        "key",
        "subject_kind",
        "subject_key",
        "subject_state",
        "risk_band",
        "risk_score",
        "predicted_remaining_days",
        "overdue_days",
        "readiness_state",
        "ready_for_eta",
        "readiness_reason",
        "source_url",
        "work_action_id",
        "source_system",
        "source_instance",
        "external_kind",
        "last_activity_at",
        "updated_at",
    }
    if not required_forecast_columns.issubset(forecast_columns):
        return []
    action_join = ""
    action_select = """
          '' as action_key,
          '' as action_state,
          '' as action_owner_key,
          '' as action_source_url
    """
    if table_exists(conn, "work_actions"):
        action_columns = table_columns(conn, "work_actions")
        required_action_columns = {
            "id",
            "key",
            "action_state",
            "owner_key",
            "source_url",
            "source_system",
            "source_instance",
        }
        if required_action_columns.issubset(action_columns):
            action_join = """
        left join work_actions wa
          on wa.id = wif.work_action_id
         and wa.source_system = wif.source_system
         and wa.source_instance = wif.source_instance
            """
            action_select = """
          wa.key as action_key,
          wa.action_state,
          wa.owner_key as action_owner_key,
          wa.source_url as action_source_url
            """
    rows = conn.execute(
        f"""
        select
          wif.key,
          wif.subject_kind,
          wif.subject_key,
          wif.risk_band,
          wif.risk_score,
          wif.predicted_remaining_days,
          wif.overdue_days,
          wif.readiness_state,
          wif.ready_for_eta,
          wif.readiness_reason,
          wif.source_url,
          wif.work_action_id,
          {action_select}
        from work_item_forecasts wif
        {action_join}
        where wif.source_system = 'cubicle_analytics'
          and wif.source_instance = ?
          and wif.external_kind in ('tpm_pr_forecast', 'tpm_work_item_forecast')
          and wif.subject_state = 'open'
          and wif.risk_band in ('critical', 'high')
        order by
          case wif.risk_band when 'critical' then 0 when 'high' then 1 else 2 end,
          (coalesce(wif.risk_score, 0) + min(coalesce(wif.overdue_days, 0), 100)) desc,
          wif.last_activity_at desc,
          wif.updated_at desc
        limit ?
        """,
        (source_instance, limit),
    ).fetchall()
    columns = [
        "key",
        "subject_kind",
        "subject_key",
        "risk_band",
        "risk_score",
        "predicted_remaining_days",
        "overdue_days",
        "readiness_state",
        "ready_for_eta",
        "readiness_reason",
        "source_url",
        "work_action_id",
        "action_key",
        "action_state",
        "action_owner_key",
        "action_source_url",
    ]
    return [dict(zip(columns, row)) for row in rows]


def ontology_dependency_action_targets(
    conn: sqlite3.Connection,
    source_instance: str,
    limit: int = 25,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_dependency_edges"):
        return []
    dependency_columns = table_columns(conn, "work_dependency_edges")
    required_dependency_columns = {
        "key",
        "edge_kind",
        "from_kind",
        "from_key",
        "to_kind",
        "to_key",
        "risk_signal",
        "source_coverage_state",
        "work_action_id",
        "source_url",
        "evidence_count",
        "freshness_state",
        "rank_score",
        "source_system",
        "source_instance",
        "external_kind",
        "last_activity_at",
        "updated_at",
    }
    if not required_dependency_columns.issubset(dependency_columns):
        return []
    action_join = ""
    action_select = """
          '' as action_key,
          '' as action_type,
          '' as action_state,
          '' as action_subject_kind,
          '' as action_subject_key,
          '' as action_owner_key,
          '' as action_decision_state,
          '' as action_source_url
    """
    order_action_state = "1"
    if table_exists(conn, "work_actions"):
        action_columns = table_columns(conn, "work_actions")
        required_action_columns = {
            "id",
            "key",
            "action_type",
            "action_state",
            "subject_kind",
            "subject_key",
            "owner_key",
            "decision_state",
            "source_url",
            "source_system",
            "source_instance",
        }
        if required_action_columns.issubset(action_columns):
            action_join = """
        left join work_actions wa
          on wa.id = wde.work_action_id
         and wa.source_system = wde.source_system
         and wa.source_instance = wde.source_instance
            """
            action_select = """
          wa.key as action_key,
          wa.action_type,
          wa.action_state,
          wa.subject_kind as action_subject_kind,
          wa.subject_key as action_subject_key,
          wa.owner_key as action_owner_key,
          wa.decision_state as action_decision_state,
          wa.source_url as action_source_url
            """
            order_action_state = "case when wa.action_state = 'open' then 0 when wa.action_state is null then 1 else 2 end"
    rows = conn.execute(
        f"""
        select
          wde.key,
          wde.edge_kind,
          wde.from_kind,
          wde.from_key,
          wde.to_kind,
          wde.to_key,
          wde.risk_signal,
          wde.source_coverage_state,
          wde.work_action_id,
          wde.source_url,
          wde.evidence_count,
          wde.freshness_state,
          wde.rank_score,
          {action_select}
        from work_dependency_edges wde
        {action_join}
        where wde.source_system = 'cubicle_analytics'
          and wde.source_instance = ?
          and wde.external_kind = 'tpm_work_dependency_edge'
          and wde.edge_kind = 'needs_action'
        order by {order_action_state}, wde.rank_score desc, wde.last_activity_at desc, wde.updated_at desc
        limit ?
        """,
        (source_instance, limit),
    ).fetchall()
    columns = [
        "key",
        "edge_kind",
        "from_kind",
        "from_key",
        "to_kind",
        "to_key",
        "risk_signal",
        "source_coverage_state",
        "work_action_id",
        "source_url",
        "evidence_count",
        "freshness_state",
        "rank_score",
        "action_key",
        "action_type",
        "action_state",
        "action_subject_kind",
        "action_subject_key",
        "action_owner_key",
        "action_decision_state",
        "action_source_url",
    ]
    return [dict(zip(columns, row)) for row in rows]


def ontology_dependency_action_edges_for_analytics(
    conn: sqlite3.Connection,
    source_instance: str,
) -> pd.DataFrame:
    if not table_exists(conn, "work_dependency_edges"):
        return pd.DataFrame(columns=dependency_action_edge_columns())
    dependency_columns = table_columns(conn, "work_dependency_edges")
    required_dependency_columns = {
        "edge_kind",
        "from_kind",
        "from_key",
        "to_kind",
        "to_key",
        "risk_signal",
        "source_coverage_state",
        "work_action_id",
        "work_blocker_id",
        "source_url",
        "freshness_state",
        "rank_score",
        "source_system",
        "source_instance",
        "external_kind",
    }
    if not required_dependency_columns.issubset(dependency_columns):
        return pd.DataFrame(columns=dependency_action_edge_columns())

    action_join = ""
    action_select = """
          '' as action_key,
          '' as action_type,
          '' as action_state,
          '' as action_decision_state,
          '' as action_owner_key,
          '' as action_subject_kind,
          '' as action_subject_key
    """
    if table_exists(conn, "work_actions"):
        action_columns = table_columns(conn, "work_actions")
        required_action_columns = {
            "id",
            "key",
            "action_type",
            "action_state",
            "decision_state",
            "owner_key",
            "subject_kind",
            "subject_key",
        }
        if required_action_columns.issubset(action_columns):
            action_join = "left join work_actions wa on wa.id = wde.work_action_id"
            action_select = """
          coalesce(wa.key, '') as action_key,
          coalesce(wa.action_type, '') as action_type,
          coalesce(wa.action_state, '') as action_state,
          coalesce(wa.decision_state, '') as action_decision_state,
          coalesce(wa.owner_key, '') as action_owner_key,
          coalesce(wa.subject_kind, '') as action_subject_kind,
          coalesce(wa.subject_key, '') as action_subject_key
            """

    blocker_join = ""
    blocker_select = """
          '' as blocker_key,
          '' as blocker_state,
          '' as blocker_review_state,
          '' as blocker_truth_label,
          '' as blocker_actionability_label,
          '' as blocker_label_quality,
          0 as blocker_measurement_eligible
    """
    if table_exists(conn, "work_blockers"):
        blocker_columns = table_columns(conn, "work_blockers")
        required_blocker_columns = {
            "id",
            "key",
            "blocker_state",
            "review_state",
            "truth_label",
            "actionability_label",
            "label_quality",
            "measurement_eligible",
        }
        if required_blocker_columns.issubset(blocker_columns):
            blocker_join = "left join work_blockers wb on wb.id = wde.work_blocker_id"
            blocker_select = """
          coalesce(wb.key, '') as blocker_key,
          coalesce(wb.blocker_state, '') as blocker_state,
          coalesce(wb.review_state, '') as blocker_review_state,
          coalesce(wb.truth_label, '') as blocker_truth_label,
          coalesce(wb.actionability_label, '') as blocker_actionability_label,
          coalesce(wb.label_quality, '') as blocker_label_quality,
          coalesce(wb.measurement_eligible, 0) as blocker_measurement_eligible
            """

    rows = conn.execute(
        f"""
        select
          wde.edge_kind,
          wde.from_kind,
          wde.from_key,
          wde.to_kind,
          wde.to_key,
          coalesce(wde.freshness_state, '') as freshness,
          coalesce(wde.risk_signal, '') as risk_signal,
          coalesce(wde.source_url, '') as source_url,
          coalesce(wde.source_coverage_state, '') as source_coverage_state,
          coalesce(wde.rank_score, 0) as rank_score,
          coalesce(wde.work_action_id, 0) as work_action_id,
          coalesce(wde.work_blocker_id, 0) as work_blocker_id,
          {action_select},
          {blocker_select}
        from work_dependency_edges wde
        {action_join}
        {blocker_join}
        where wde.source_system = 'cubicle_analytics'
          and wde.source_instance = ?
          and wde.external_kind = 'tpm_work_dependency_edge'
          and wde.edge_kind in ('blocked_by', 'needs_action')
        order by wde.edge_kind, wde.rank_score desc, wde.from_key, wde.to_key
        """,
        (source_instance,),
    ).fetchall()
    rows_out: list[dict[str, Any]] = []
    for row in rows:
        (
            edge_kind,
            from_kind,
            from_key,
            to_kind,
            to_key,
            freshness,
            risk_signal,
            source_url,
            source_coverage_state,
            rank_score,
            work_action_id,
            work_blocker_id,
            action_key,
            action_type,
            action_state,
            action_decision_state,
            action_owner_key,
            action_subject_kind,
            action_subject_key,
            blocker_key,
            blocker_state,
            blocker_review_state,
            blocker_truth_label,
            blocker_actionability_label,
            blocker_label_quality,
            blocker_measurement_eligible,
        ) = row
        rows_out.append(
            {
                "edge_kind": first_nonempty([edge_kind]),
                "source_key": analytics_dependency_node_key(from_kind, from_key),
                "target_key": analytics_dependency_node_key(to_kind, to_key),
                "freshness": first_nonempty([freshness]),
                "risk_signal": first_nonempty([risk_signal]),
                "source_url": first_nonempty([source_url]),
                "source_coverage_state": first_nonempty([source_coverage_state]),
                "rank_score": safe_float(rank_score),
                "work_action_id": int(work_action_id or 0),
                "action_key": first_nonempty([action_key]),
                "action_type": first_nonempty([action_type]),
                "action_state": first_nonempty([action_state]),
                "action_decision_state": first_nonempty([action_decision_state]),
                "action_owner_key": first_nonempty([action_owner_key]),
                "action_subject_kind": first_nonempty([action_subject_kind]),
                "action_subject_key": first_nonempty([action_subject_key]),
                "work_blocker_id": int(work_blocker_id or 0),
                "blocker_key": first_nonempty([blocker_key]),
                "blocker_state": first_nonempty([blocker_state]),
                "blocker_review_state": first_nonempty([blocker_review_state]),
                "blocker_truth_label": first_nonempty([blocker_truth_label]),
                "blocker_actionability_label": first_nonempty([blocker_actionability_label]),
                "blocker_label_quality": first_nonempty([blocker_label_quality]),
                "blocker_measurement_eligible": bool(blocker_measurement_eligible),
            }
        )
    return pd.DataFrame(rows_out, columns=dependency_action_edge_columns())


def analytics_dependency_node_key(kind: Any, key: Any) -> str:
    kind_text = first_nonempty([kind])
    key_text = first_nonempty([key])
    if not key_text:
        return ""
    if kind_text == "ticket":
        return f"ticket:{key_text.upper()}"
    if kind_text == "pull_request":
        return f"pr:{key_text}"
    if kind_text in {"component", "workstream"}:
        return key_text if key_text.startswith(f"{kind_text}:") else f"{kind_text}:{key_text}"
    if kind_text == "blocker":
        return key_text if key_text.startswith("work-blocker:") else f"work-blocker:{key_text}"
    if kind_text == "action":
        return key_text if key_text.startswith("tpm-action:") else f"tpm-action:{key_text}"
    return f"{kind_text}:{key_text}" if kind_text else key_text


def adversarial_check(
    key: str,
    check_kind: str,
    check_state: str,
    severity: str,
    title: str,
    detail: str,
    recommended_action: str,
    blocking_gate_keys: list[str] | None = None,
    evidence_refs: list[str] | None = None,
) -> dict[str, Any]:
    return {
        "key": key,
        "check_kind": check_kind,
        "check_state": check_state,
        "severity": severity,
        "title": title,
        "detail": detail,
        "recommended_action": recommended_action,
        "blocking_gate_keys": unique_strings(blocking_gate_keys or []),
        "evidence_refs": unique_strings(evidence_refs or []),
    }


def quality_gate(
    key: str,
    gate_state: str,
    blocking: bool,
    detail: str,
    recommended_action: str,
) -> dict[str, Any]:
    return {
        "key": key,
        "gate_state": gate_state,
        "blocking": blocking,
        "detail": detail,
        "recommended_action": recommended_action,
    }


def quality_gate_confidence(gate_state: str, blocking: bool) -> float:
    if blocking or gate_state == "gated":
        return 1.0
    return 0.95


def automation_readiness_rationale(state: str, blocking_gate_keys: list[str]) -> str:
    blocking = unique_strings(blocking_gate_keys)
    if not blocking:
        return "Automation can proceed from the current typed work-program evidence, with routine monitoring."
    if state == "blocked":
        return "Autonomous TPM action is blocked by " + ", ".join(blocking) + "."
    if state == "supervised":
        return "Automation can assist, but human supervision remains required until " + ", ".join(blocking) + " clear."
    return "Automation can draft and rank work, but human review remains required while " + ", ".join(blocking) + " remain open."


def automation_required_evidence(blocking_gate_keys: list[str]) -> list[str]:
    required: list[str] = []
    for key in blocking_gate_keys:
        if key == "forecast_readiness":
            required = append_unique(required, "forecast backtest outcomes and readiness history")
        elif key == "measurement_precision":
            required = append_unique(required, "gold labels for product-action-backed insight precision")
        elif key == "measurement_actionability":
            required = append_unique(required, "actionability labels for product-action-backed insight kinds")
        elif key == GLOBAL_INSIGHT_PRECISION_KEY:
            required = append_unique(required, "gold labels for global generated insight precision")
        elif key == GLOBAL_INSIGHT_ACTIONABILITY_KEY:
            required = append_unique(required, "actionability labels across validation and context insight kinds")
        elif key == "source_coverage":
            required = append_unique(required, "source repair or required-check configuration for limited program items")
        elif key == "source_authentication":
            required = append_unique(required, "authenticated re-observation for anonymous source observations")
        elif key == "claim_provenance":
            required = append_unique(required, "independent source or measurement evidence for generated claims")
        elif key == "owner_load":
            required = append_unique(required, "owner-load rebalancing or explicit assignment evidence")
        elif key == "blocker_clearance":
            required = append_unique(required, "owner-confirmed blocker clearance or acceptance")
    return required


def automation_readiness_confidence(readiness_state: str, blocking_gate_keys: list[str]) -> float:
    if readiness_state == "automatable" and not blocking_gate_keys:
        return 0.95
    return 1.0


def tpm_function_readiness(
    function_key: str,
    function_name: str,
    readiness_state: str,
    automation_state: str,
    human_required: bool,
    supporting_signal_count: int,
    blocking_gate_keys: list[str],
    detail: str,
    recommended_action: str,
) -> dict[str, Any]:
    return {
        "function_key": function_key,
        "function_name": function_name,
        "readiness_state": readiness_state,
        "automation_state": automation_state,
        "human_required": human_required,
        "supporting_signal_count": supporting_signal_count,
        "blocking_gate_keys": unique_strings(blocking_gate_keys),
        "detail": detail,
        "recommended_action": recommended_action,
    }


def tpm_function_readiness_confidence(readiness_state: str, blocking_gate_keys: list[str]) -> float:
    if readiness_state == "automatable" and not blocking_gate_keys:
        return 1.0
    if readiness_state in {"blocked", "supervised"}:
        return 1.0
    return 0.9


def ontology_program_item_rows(conn: sqlite3.Connection, source_instance: str, workstream_keys: list[str]) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_program_items"):
        return []
    available_columns = table_columns(conn, "work_program_items")
    base_columns = ["program_status", "decision_state", "source_coverage_state", "freshness_state", "due_bucket", "risk_score", "owner_key"]
    optional_columns = [
        "subject_kind",
        "subject_key",
        "title",
        "source_url",
        "work_action_id",
        "evidence_count",
        "latest_evidence_id",
        "rank_score",
    ]
    select_columns = [column for column in [*base_columns, *optional_columns] if column in available_columns]
    action_columns: list[str] = []
    action_join = ""
    action_select = ""
    if "work_action_id" in available_columns and table_exists(conn, "work_actions"):
        work_action_columns = table_columns(conn, "work_actions")
        required_action_columns = {"id", "key", "action_state", "owner_key", "source_url", "source_system", "source_instance"}
        if required_action_columns.issubset(work_action_columns):
            action_columns = ["action_key", "action_state", "action_owner_key", "action_source_url"]
            optional_action_selects: list[str] = []
            if "source_link_insight_kinds" in work_action_columns:
                action_columns.append("action_source_link_insight_kinds")
                optional_action_selects.append("wa.source_link_insight_kinds as action_source_link_insight_kinds")
            if "source_insight_keys" in work_action_columns:
                action_columns.append("action_source_insight_keys")
                optional_action_selects.append("wa.source_insight_keys as action_source_insight_keys")
            action_join = """
        left join work_actions wa
          on wa.id = wpi.work_action_id
         and wa.source_system = wpi.source_system
         and wa.source_instance = wpi.source_instance
            """
            action_select = """,
          wa.key as action_key,
          wa.action_state,
          wa.owner_key as action_owner_key,
          wa.source_url as action_source_url"""
            if optional_action_selects:
                action_select += ",\n          " + ",\n          ".join(optional_action_selects)
    placeholders = ",".join("?" for _ in workstream_keys)
    select_expr = ", ".join(f"wpi.{column}" for column in select_columns)
    rows = conn.execute(
        f"""
        select {select_expr}{action_select}
        from work_program_items wpi
        {action_join}
        where wpi.source_system = 'cubicle_analytics'
          and wpi.source_instance = ?
          and wpi.external_kind = 'tpm_program_item'
          and wpi.workstream_key in ({placeholders})
        """,
        [source_instance, *workstream_keys],
    ).fetchall()
    result_columns = [*select_columns, *action_columns]
    results = [dict(zip(result_columns, row)) for row in rows]
    for result in results:
        for column in [*optional_columns, *action_columns]:
            result.setdefault(column, "")
    return results


def ontology_owner_load_rows(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_keys: list[str],
    generated_at: str,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_owner_load_snapshots"):
        return []
    placeholders = ",".join("?" for _ in workstream_keys)
    base_sql = f"""
        select owner_key, load_status, action_count
        from work_owner_load_snapshots
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_owner_load_snapshot'
          and workstream_key in ({placeholders})
    """
    params: list[Any] = [source_instance, *workstream_keys]
    rows = conn.execute(base_sql + " and generated_at = ?", [*params, generated_at]).fetchall()
    if not rows:
        latest = conn.execute(
            f"""
            select max(generated_at)
            from work_owner_load_snapshots
            where source_system = 'cubicle_analytics'
              and source_instance = ?
              and external_kind = 'tpm_owner_load_snapshot'
              and workstream_key in ({placeholders})
            """,
            params,
        ).fetchone()
        latest_generated_at = latest[0] if latest else None
        if latest_generated_at:
            rows = conn.execute(base_sql + " and generated_at = ?", [*params, latest_generated_at]).fetchall()
    columns = ["owner_key", "load_status", "action_count"]
    return [dict(zip(columns, row)) for row in rows]


def ontology_owner_load_evidence_targets(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_keys: list[str],
    generated_at: str,
    limit: int = 10,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_owner_load_snapshots"):
        return []
    required_columns = {
        "owner_key",
        "owner_display_name",
        "load_status",
        "action_count",
        "product_action_count",
        "validation_lead_count",
        "critical_or_high_count",
        "max_priority_score",
        "top_action_type",
        "top_subjects",
        "recommended_focus",
        "source_url",
        "source_system",
        "source_instance",
        "external_kind",
        "workstream_key",
        "generated_at",
    }
    if not required_columns.issubset(table_columns(conn, "work_owner_load_snapshots")):
        return []
    placeholders = ",".join("?" for _ in workstream_keys)
    base_sql = f"""
        select
          owner_key,
          owner_display_name,
          load_status,
          action_count,
          product_action_count,
          validation_lead_count,
          critical_or_high_count,
          max_priority_score,
          top_action_type,
          top_subjects,
          recommended_focus,
          source_url
        from work_owner_load_snapshots
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          and external_kind = 'tpm_owner_load_snapshot'
          and workstream_key in ({placeholders})
          and (load_status = 'overloaded' or (owner_key = '(unassigned)' and product_action_count > 0))
    """
    params: list[Any] = [source_instance, *workstream_keys]
    rows = conn.execute(
        base_sql + """
          and generated_at = ?
        order by
          case when owner_key = '(unassigned)' then 0 when load_status = 'overloaded' then 1 else 2 end,
          max_priority_score desc,
          action_count desc,
          owner_key
        limit ?
        """,
        [*params, generated_at, limit],
    ).fetchall()
    if not rows:
        latest = conn.execute(
            f"""
            select max(generated_at)
            from work_owner_load_snapshots
            where source_system = 'cubicle_analytics'
              and source_instance = ?
              and external_kind = 'tpm_owner_load_snapshot'
              and workstream_key in ({placeholders})
            """,
            params,
        ).fetchone()
        latest_generated_at = latest[0] if latest else None
        if latest_generated_at:
            rows = conn.execute(
                base_sql + """
                  and generated_at = ?
                order by
                  case when owner_key = '(unassigned)' then 0 when load_status = 'overloaded' then 1 else 2 end,
                  max_priority_score desc,
                  action_count desc,
                  owner_key
                limit ?
                """,
                [*params, latest_generated_at, limit],
            ).fetchall()
    columns = [
        "owner_key",
        "owner_display_name",
        "load_status",
        "action_count",
        "product_action_count",
        "validation_lead_count",
        "critical_or_high_count",
        "max_priority_score",
        "top_action_type",
        "top_subjects",
        "recommended_focus",
        "source_url",
    ]
    return [dict(zip(columns, row)) for row in rows]


def ontology_measurement_label_targets(
    conn: sqlite3.Connection,
    source_instance: str,
    limit: int = 25,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_insights") or not table_exists(conn, "work_insight_reviews"):
        return []
    insight_required = {
        "id",
        "key",
        "insight_kind",
        "subject_kind",
        "subject_key",
        "title",
        "score",
        "source_url",
        "producer_state",
        "source_system",
        "source_instance",
        "external_kind",
        "rank_score",
        "updated_at",
    }
    review_required = {
        "work_insight_id",
        "review_kind",
        "review_state",
        "truth_label",
        "actionability_label",
        "label_quality",
        "measurement_eligible",
    }
    if not insight_required.issubset(table_columns(conn, "work_insights")):
        return []
    if not review_required.issubset(table_columns(conn, "work_insight_reviews")):
        return []
    rows = conn.execute(
        """
        select
          wi.key,
          wi.insight_kind,
          wi.subject_kind,
          wi.subject_key,
          wi.title,
          wi.score,
          wi.source_url,
          sum(case when wir.review_state = 'requested' then 1 else 0 end) as review_request_count,
          sum(case when wir.measurement_eligible = 1
                    and (
                      wir.review_kind = 'human_assessment'
                      or (wir.review_kind = 'evaluation_label' and wir.label_quality = 'gold')
                    )
                    and (wir.truth_label != 'unknown' or wir.actionability_label != 'unknown')
                   then 1 else 0 end) as measurement_label_count
        from work_insights wi
        left join work_insight_reviews wir on wir.work_insight_id = wi.id
        where wi.source_system = 'cubicle_analytics'
          and wi.source_instance = ?
          and wi.external_kind = 'tpm_insight'
          and wi.producer_state = 'current'
        group by wi.id
        having measurement_label_count = 0
        order by
          case wi.insight_kind
            when 'forecast_risk' then 0
            when 'status_summary' then 1
            when 'model_quality' then 2
            else 3
          end,
          wi.rank_score desc,
          wi.score desc,
          wi.updated_at desc
        limit ?
        """,
        (source_instance, limit),
    ).fetchall()
    columns = [
        "insight_key",
        "insight_kind",
        "subject_kind",
        "subject_key",
        "title",
        "score",
        "source_url",
        "review_request_count",
        "measurement_label_count",
    ]
    return [dict(zip(columns, row)) for row in rows]


def ontology_active_blocker_clearance_targets(
    conn: sqlite3.Connection,
    source_instance: str,
    limit: int = 12,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_blockers"):
        return []
    blocker_columns = table_columns(conn, "work_blockers")
    required_blocker_columns = {
        "id",
        "external_id",
        "title",
        "blocker_kind",
        "blocker_state",
        "severity",
        "subject_kind",
        "subject_key",
        "owner_key",
        "decision_state",
        "source_coverage_state",
        "review_state",
        "truth_label",
        "actionability_label",
        "recommended_action",
        "source_url",
        "latest_evidence_id",
        "evidence_count",
        "confidence",
        "rank_score",
        "work_action_id",
        "source_system",
        "source_instance",
        "external_kind",
        "updated_at",
    }
    if not required_blocker_columns.issubset(blocker_columns):
        return []
    impact_join = ""
    impact_select = "0 as active_impact_count, coalesce(wb.rank_score, 0) as max_impact_score"
    impact_columns = table_columns(conn, "work_blocker_impacts")
    required_impact_columns = {"id", "work_blocker_id", "source_system", "source_instance", "impact_state", "impact_score"}
    if required_impact_columns.issubset(impact_columns):
        impact_join = """
        left join work_blocker_impacts wbi
          on wbi.work_blocker_id = wb.id
         and wbi.source_system = wb.source_system
         and wbi.source_instance = wb.source_instance
         and wbi.impact_state = 'active'
        """
        impact_select = "count(wbi.id) as active_impact_count, coalesce(max(wbi.impact_score), wb.rank_score, 0) as max_impact_score"
    action_join = ""
    action_select = """
          '' as action_key,
          '' as action_state,
          '' as action_owner_key,
          '' as action_source_url
    """
    action_columns = table_columns(conn, "work_actions")
    required_action_columns = {"id", "key", "action_state", "owner_key", "source_url", "source_system", "source_instance"}
    if required_action_columns.issubset(action_columns):
        action_join = """
        left join work_actions wa
          on wa.id = wb.work_action_id
         and wa.source_system = wb.source_system
         and wa.source_instance = wb.source_instance
        """
        action_select = """
          wa.key as action_key,
          wa.action_state,
          wa.owner_key as action_owner_key,
          wa.source_url as action_source_url
        """
    rows = conn.execute(
        f"""
        select
          wb.id as work_blocker_id,
          wb.external_id,
          wb.title,
          wb.blocker_kind,
          wb.blocker_state,
          wb.severity,
          wb.subject_kind,
          wb.subject_key,
          wb.owner_key,
          wb.decision_state,
          wb.source_coverage_state,
          wb.review_state,
          wb.truth_label,
          wb.actionability_label,
          wb.recommended_action,
          wb.source_url,
          wb.latest_evidence_id,
          wb.evidence_count,
          wb.confidence,
          wb.rank_score,
          wb.work_action_id,
          {impact_select},
          {action_select}
        from work_blockers wb
        {impact_join}
        {action_join}
        where wb.source_system = 'cubicle_analytics'
          and wb.source_instance = ?
          and wb.external_kind = 'tpm_work_blocker'
          and wb.blocker_state = 'active'
        group by wb.id
        order by
          case wb.severity
            when 'critical' then 0
            when 'high' then 1
            when 'medium' then 2
            when 'low' then 3
            else 4
          end,
          max_impact_score desc,
          wb.rank_score desc,
          wb.updated_at desc
        limit ?
        """,
        (source_instance, limit),
    ).fetchall()
    columns = [
        "work_blocker_id",
        "external_id",
        "title",
        "blocker_kind",
        "blocker_state",
        "severity",
        "subject_kind",
        "subject_key",
        "owner_key",
        "decision_state",
        "source_coverage_state",
        "review_state",
        "truth_label",
        "actionability_label",
        "recommended_action",
        "source_url",
        "latest_evidence_id",
        "evidence_count",
        "confidence",
        "rank_score",
        "work_action_id",
        "active_impact_count",
        "max_impact_score",
        "action_key",
        "action_state",
        "action_owner_key",
        "action_source_url",
    ]
    return [dict(zip(columns, row)) for row in rows]


def ontology_count(
    conn: sqlite3.Connection,
    table_name: str,
    source_instance: str,
    extra_where: str = "",
    extra_params: list[Any] | None = None,
) -> int:
    if not table_exists(conn, table_name):
        return 0
    row = conn.execute(
        f"""
        select count(*)
        from {table_name}
        where source_system = 'cubicle_analytics'
          and source_instance = ?
          {extra_where}
        """,
        [source_instance, *(extra_params or [])],
    ).fetchone()
    return int(row[0] or 0) if row else 0


def ontology_evidence_refs(
    conn: sqlite3.Connection,
    table_name: str,
    source_instance: str,
    extra_where: str = "",
    extra_params: list[Any] | None = None,
    limit: int = 5,
) -> list[str]:
    if not table_exists(conn, table_name):
        return []
    rows = conn.execute(
        f"""
        select e.locator_kind, e.locator, e.source_url, e.source_span_key, e.external_id, e.key
        from {table_name} source_row
        left join evidences e on e.id = source_row.latest_evidence_id
        where source_row.source_system = 'cubicle_analytics'
          and source_row.source_instance = ?
          {extra_where}
        order by source_row.rank_score desc, source_row.updated_at desc
        limit ?
        """,
        [source_instance, *(extra_params or []), limit],
    ).fetchall()
    refs = []
    for row in rows:
        refs.append(evidence_ref_from_parts(*row))
    return unique_strings(refs)


def evidence_ref_from_parts(locator_kind: Any, locator: Any, source_url: Any, source_span_key: Any, external_id: Any, key: Any) -> str:
    parts = [first_nonempty([locator_kind]), first_nonempty([locator]), first_nonempty([source_url])]
    ref = " ".join(part for part in parts if part)
    if ref:
        return ref
    return first_nonempty([source_span_key, external_id, key])


def ontology_program_item_coverage_limited(row: dict[str, Any]) -> bool:
    state = first_nonempty([row.get("source_coverage_state")]).lower()
    freshness = first_nonempty([row.get("freshness_state")]).lower()
    if state_uses_auth_limited_observation(state) or state_uses_generated_claim_evidence(state):
        return False
    if state == "not_observed":
        return True
    return freshness == "partial" or state_has_source_coverage_gap(state)


def ontology_program_item_auth_limited(row: dict[str, Any]) -> bool:
    return state_uses_auth_limited_observation(first_nonempty([row.get("source_coverage_state")]).lower())


def ontology_program_item_generated_claim_limited(row: dict[str, Any]) -> bool:
    return state_uses_generated_claim_evidence(first_nonempty([row.get("source_coverage_state")]).lower())


def state_has_source_coverage_gap(state: str) -> bool:
    text = first_nonempty([state]).lower()
    return (
        not text
        or text == "not_observed"
        or any(token in text for token in ["failed", "failure", "partial", "repair", "unavailable", "unknown", "missing"])
    )


def state_uses_auth_limited_observation(state: str) -> bool:
    return "anonymous" in first_nonempty([state]).lower()


def state_uses_generated_claim_evidence(state: str) -> bool:
    return "generated" in first_nonempty([state]).lower()


def ontology_program_item_coverage_limit_kind(row: dict[str, Any]) -> str:
    state = first_nonempty([row.get("source_coverage_state")]).lower()
    freshness = first_nonempty([row.get("freshness_state")]).lower()
    if "required_check_coverage" in state and "unavailable" in state:
        return "required_check_coverage_unavailable"
    if state == "not_observed":
        return "not_observed"
    if "anonymous" in state:
        return "anonymous_observation"
    if "generated" in state:
        return "generated_evidence"
    if "failed" in state or "failure" in state:
        return "source_failure"
    if "repair" in state:
        return "source_repair_needed"
    if "unknown" in state:
        return "unknown_source_coverage"
    if "unavailable" in state:
        return "source_unavailable"
    if "partial" in state or freshness == "partial":
        return "partial_source_coverage"
    return "coverage_limited"


def ontology_program_item_product_decision_open(row: dict[str, Any]) -> bool:
    return (
        first_nonempty([row.get("decision_state")]) in {"product_action", "closeout_review"}
        or first_nonempty([row.get("program_status")]) in {"needs_decision", "closed_pending_review"}
    )


def adversarial_check_confidence(check_state: str, evidence_refs: list[str]) -> float:
    if check_state == "pass" and not evidence_refs:
        return 0.85
    if check_state == "warning":
        return 0.9
    return 1.0


def adversarial_check_rank_score(check_state: str, severity: str, rank: int) -> float:
    state_bonus = {"fail": 100.0, "warning": 50.0, "pass": 10.0}.get(check_state, 0.0)
    severity_bonus = float(SEVERITY_RANK.get(severity, 0) * 10)
    return state_bonus + severity_bonus - float(rank)


def brief_caveat_rank_score(severity: str, rank: int) -> float:
    severity_bonus = {
        "danger": 100.0,
        "warning": 75.0,
        "info": 25.0,
    }.get(severity, 50.0)
    return severity_bonus - float(rank) / 100.0


def brief_caveat_confidence(severity: str, evidence_ref: str) -> float:
    if evidence_ref:
        return 1.0
    if severity == "danger":
        return 0.95
    return 0.9


def evidence_need_rank_score(priority: str, missing_count: int, rank: int) -> float:
    priority_bonus = {
        "critical": 100.0,
        "high": 75.0,
        "medium": 50.0,
        "low": 25.0,
        "info": 10.0,
    }.get(priority, 0.0)
    return priority_bonus + float(max(0, missing_count)) - float(rank) / 100.0


def evidence_priority_rank(priority: str) -> int:
    return {
        "critical": 0,
        "high": 1,
        "medium": 2,
        "low": 3,
        "info": 4,
    }.get(priority, 5)


def owner_load_status_from_rows(rows: list[dict[str, Any]]) -> str:
    statuses = [first_nonempty([row.get("load_status")]) for row in rows]
    if "overloaded" in statuses:
        return "overloaded"
    if "attention_required" in statuses or any(first_nonempty([row.get("owner_key")]) == "(unassigned)" and int_value(row.get("product_action_count")) > 0 for row in rows):
        return "attention_required"
    if any(int_value(row.get("action_count")) > 0 for row in rows):
        return "watch"
    return "clear"


def int_value(value: Any) -> int:
    try:
        return int(float(str(value)))
    except (TypeError, ValueError):
        return 0


def bool_int(value: Any) -> int:
    if isinstance(value, bool):
        return 1 if value else 0
    text = first_nonempty([value]).strip().lower()
    if text in {"1", "true", "yes", "y"}:
        return 1
    if text in {"0", "false", "no", "n"}:
        return 0
    return 1 if int_value(value) != 0 else 0


def optional_rate(value: Any) -> float | None:
    if value is None:
        return None
    try:
        result = float(value)
    except (TypeError, ValueError):
        return None
    if math.isnan(result):
        return None
    return result


def rate_text(numerator: int, denominator: int) -> str:
    if denominator <= 0:
        return "0"
    value = float(numerator) / float(denominator)
    if value > 1:
        value = 1.0
    return f"{value:.4f}".rstrip("0").rstrip(".")


def count_phrase(count: int, label: str) -> str:
    if count == 1:
        return f"1 {label}"
    return f"{count} {label}s"


def join_focus_parts(parts: list[str]) -> str:
    if len(parts) == 1:
        return parts[0]
    if len(parts) == 2:
        return f"{parts[0]} and {parts[1]}"
    return ", ".join(parts[:-1]) + f", and {parts[-1]}"


def has_have(count: int) -> str:
    return "has" if count == 1 else "have"


def is_are(count: int) -> str:
    return "is" if count == 1 else "are"


def unique_strings(values: list[str]) -> list[str]:
    out = []
    seen = set()
    for value in values:
        text = first_nonempty([value])
        if not text or text in seen:
            continue
        seen.add(text)
        out.append(text)
    return out


def unique_ints(values: list[int]) -> list[int]:
    out = []
    seen = set()
    for value in values:
        try:
            int_value = int(value)
        except (TypeError, ValueError):
            continue
        if int_value in seen:
            continue
        seen.add(int_value)
        out.append(int_value)
    return out


def append_unique(values: list[str], value: str) -> list[str]:
    text = first_nonempty([value])
    if text and text not in values:
        values.append(text)
    return values


def owner_load_status(row: pd.Series) -> str:
    action_count = metric_row_int(row, "action_count")
    product_action_count = metric_row_int(row, "product_action_count")
    critical_or_high_count = metric_row_int(row, "critical_or_high_count")
    if action_count <= 0:
        return "clear"
    if product_action_count > 0 and action_count >= 2 and (critical_or_high_count > 0 or safe_float(row.get("max_priority_score")) >= 90):
        return "overloaded"
    if product_action_count > 0 or critical_or_high_count > 0:
        return "attention_required"
    if metric_row_int(row, "coverage_limited_count") > 0 or metric_row_int(row, "anonymous_observation_count") > 0:
        return "attention_required"
    if metric_row_int(row, "validation_lead_count") > 0 or metric_row_int(row, "needs_human_review_count") > 0:
        return "watch"
    return "watch"


def owner_display_name_for_key(owner_key: str) -> str:
    key = first_nonempty([owner_key])
    if key.startswith("github:"):
        return key.removeprefix("github:")
    if key == "(unassigned)":
        return ""
    return key


def workstream_operating_status(value: str) -> str:
    if value in {"unknown", "clear", "watch", "validation_required", "attention_required"}:
        return value
    return "unknown"


def workstream_standup_section_kind(value: str) -> str:
    if value in {
        "top_action",
        "product_action",
        "validation_lead",
        "source_repair",
        "closeout_review",
        "model_or_rule_qa",
        "suppressed_signal",
        "model_quality",
        "owner_load",
        "resolved_change",
    }:
        return value
    return "top_action"


def workstream_standup_urgency(value: str) -> str:
    if value in {"unknown", "critical", "high", "medium", "low"}:
        return value
    return "unknown"


def standup_subject_kind(subject_key: str) -> str:
    key = first_nonempty([subject_key])
    if not key:
        return "unknown"
    if "#" in key:
        return "pull_request"
    if re.match(r"^[A-Z][A-Z0-9]+-\d+$", key):
        return "ticket"
    return "unknown"


def workstream_keys_for_register(program_register: pd.DataFrame) -> list[str]:
    if program_register.empty or "workstream_key" not in program_register.columns:
        return []
    return sorted({first_nonempty([value]) for value in program_register["workstream_key"].tolist() if first_nonempty([value])})


def ticket_keys_for_workstream(workstream_key: str, program_register: pd.DataFrame, ticket_features: pd.DataFrame) -> set[str]:
    keys: set[str] = set()
    if not ticket_features.empty and "ticket_key" in ticket_features.columns:
        keys.update(first_nonempty([value]).upper() for value in ticket_features["ticket_key"].tolist() if first_nonempty([value]))
    if not program_register.empty:
        for _, row in program_register.iterrows():
            if first_nonempty([row.get("subject_kind")]) == "ticket":
                keys.add(first_nonempty([row.get("subject_key")]).upper())
            keys.update(key.upper() for key in split_csv(first_nonempty([row.get("linked_ticket_keys")])))
    return {key for key in keys if key}


def upsert_workstream_ticket(
    conn: sqlite3.Connection,
    workstream_id: int,
    ticket_id: int,
    source_instance: str,
    workstream_key: str,
    ticket_key: str,
    now: str,
    rank_score: float,
) -> None:
    external_id = f"{workstream_key}:{ticket_key}"
    values = {
        "workstream_ticket_kind": "contains",
        "evidence_count": 1,
        "event_count": 1,
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": rank_score,
        "source_system": "cubicle_analytics",
        "source_instance": source_instance,
        "external_kind": "tpm_workstream_ticket",
        "external_id": external_id,
        "source_url": workstream_source_url(workstream_key),
        "source_updated_at": now,
        "content_hash": stable_digest([external_id]),
        "deletion_state": "present",
        "acl_state": "unavailable",
        "last_confirmed_at": now,
        "last_changed_at": now,
        "freshness_state": "partial",
        "visibility": "public",
        "confidence": 0.9,
        "created_at": now,
        "updated_at": now,
        "workstream_id": workstream_id,
        "ticket_id": ticket_id,
    }
    columns = list(values.keys())
    placeholders = ", ".join(["?"] * len(columns))
    assignments = ", ".join(
        f"{column} = excluded.{column}"
        for column in columns
        if column not in {"workstream_id", "ticket_id", "workstream_ticket_kind"}
    )
    conn.execute(
        f"""
        insert into workstream_tickets ({", ".join(columns)})
        values ({placeholders})
        on conflict(workstream_id, ticket_id, workstream_ticket_kind) do update set {assignments}
        """,
        [sqlite_value(values[column]) for column in columns],
    )


def workstream_title(workstream_key: str) -> str:
    if workstream_key == "flink-kubernetes-operator":
        return "Flink Kubernetes Operator"
    return workstream_key.replace("-", " ").title()


def workstream_source_url(workstream_key: str) -> str:
    if workstream_key == "flink-kubernetes-operator":
        return "https://github.com/apache/flink-kubernetes-operator"
    return ""


def workstream_summary(workstream_key: str, ticket_count: int, rows: pd.DataFrame) -> str:
    if rows.empty:
        return f"{workstream_title(workstream_key)} workstream with {ticket_count} captured ticket(s)."
    product_actions = int((rows["decision_state"] == "product_action").sum()) if "decision_state" in rows.columns else 0
    validation_leads = int((rows["decision_state"] == "validation_lead").sum()) if "decision_state" in rows.columns else 0
    return (
        f"{workstream_title(workstream_key)} workstream with {ticket_count} captured ticket(s), "
        f"{len(rows)} operating register row(s), {product_actions} product action(s), "
        f"and {validation_leads} validation lead(s)."
    )


def workstream_search_text(workstream_key: str, rows: pd.DataFrame) -> str:
    titles = []
    if not rows.empty and "title" in rows.columns:
        titles = [first_nonempty([value]) for value in rows["title"].head(10).tolist() if first_nonempty([value])]
    return " ".join([workstream_title(workstream_key), workstream_key, *titles])


def max_risk_score(rows: pd.DataFrame) -> float:
    if rows.empty or "risk_score" not in rows.columns:
        return 0.0
    values = pd.to_numeric(rows["risk_score"], errors="coerce").fillna(0.0)
    return float(values.max()) if len(values) else 0.0


def ontology_pr_ids_by_subject(conn: sqlite3.Connection) -> dict[str, int]:
    if not table_exists(conn, "pull_requests"):
        return {}
    rows = conn.execute("select id, repository, number from pull_requests").fetchall()
    return {f"{repository}#{int(number)}": int(row_id) for row_id, repository, number in rows if repository and number is not None}


def ontology_ticket_ids_by_subject(conn: sqlite3.Connection) -> dict[str, int]:
    if not table_exists(conn, "tickets"):
        return {}
    rows = conn.execute("select id, external_id from tickets").fetchall()
    return {str(external_id).upper(): int(row_id) for row_id, external_id in rows if external_id}


def ontology_ticket_pr_evidence_by_subject(conn: sqlite3.Connection) -> dict[tuple[str, str], dict[str, Any]]:
    if not all(table_exists(conn, table) for table in ["ticket_pull_requests", "tickets", "pull_requests"]):
        return {}
    rows = conn.execute(
        """
        select
          t.external_id as ticket_key,
          pr.repository as repository,
          pr.number as pr_number,
          tpr.latest_evidence_id,
          tpr.evidence_count,
          tpr.source_url
        from ticket_pull_requests tpr
        join tickets t on t.id = tpr.ticket_id
        join pull_requests pr on pr.id = tpr.pull_request_id
        where tpr.latest_evidence_id is not null
        """
    ).fetchall()
    out: dict[tuple[str, str], dict[str, Any]] = {}
    for ticket_key, repository, pr_number, latest_evidence_id, evidence_count, source_url in rows:
        if not ticket_key or not repository or pr_number is None:
            continue
        out[(str(ticket_key).upper(), f"{repository}#{int(pr_number)}")] = {
            "latest_evidence_id": int(latest_evidence_id),
            "evidence_count": int(evidence_count or 1),
            "source_url": source_url,
        }
    return out


def ontology_work_action_ids_by_key(conn: sqlite3.Connection, source_instance: str) -> dict[str, int]:
    if not table_exists(conn, "work_actions"):
        return {}
    rows = conn.execute(
        """
        select id, key
          from work_actions
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_action'
        """,
        (source_instance,),
    ).fetchall()
    return {str(key): int(row_id) for row_id, key in rows if key}


def ontology_work_program_quality_gate_ids_by_key(
    conn: sqlite3.Connection,
    source_instance: str,
    workstream_key: str,
    generated_at: str,
) -> dict[str, int]:
    if not table_exists(conn, "work_program_quality_gates"):
        return {}
    required_columns = {"id", "gate_key", "source_instance", "workstream_key", "generated_at"}
    if not required_columns.issubset(table_columns(conn, "work_program_quality_gates")):
        return {}
    workstream_keys = work_program_workstream_sql_keys(workstream_key)
    rows = conn.execute(
        f"""
        select id, gate_key
          from work_program_quality_gates
         where source_instance = ?
           and workstream_key in ({", ".join(["?"] * len(workstream_keys))})
           and generated_at = ?
        """,
        (source_instance, *workstream_keys, generated_at),
    ).fetchall()
    return {str(gate_key): int(row_id) for row_id, gate_key in rows if gate_key}


def ontology_workstream_ids_by_key(conn: sqlite3.Connection, source_instance: str) -> dict[str, int]:
    if not table_exists(conn, "workstreams"):
        return {}
    rows = conn.execute(
        """
        select id, key
          from workstreams
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_workstream'
        """,
        (source_instance,),
    ).fetchall()
    return {str(key): int(row_id) for row_id, key in rows if key}


def ontology_workstream_health_snapshot_ids_by_external_id(conn: sqlite3.Connection, source_instance: str) -> dict[str, int]:
    if not table_exists(conn, "workstream_health_snapshots"):
        return {}
    rows = conn.execute(
        """
        select id, external_id
          from workstream_health_snapshots
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_workstream_health_snapshot'
        """,
        (source_instance,),
    ).fetchall()
    return {str(external_id): int(row_id) for row_id, external_id in rows if external_id}


def ontology_work_action_details_by_key(conn: sqlite3.Connection, source_instance: str) -> dict[str, dict[str, Any]]:
    if not table_exists(conn, "work_actions"):
        return {}
    rows = conn.execute(
        """
        select id, key, latest_evidence_id, source_url, freshness_state, confidence
          from work_actions
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_work_action'
        """,
        (source_instance,),
    ).fetchall()
    return {
        str(key): {
            "id": int(row_id),
            "latest_evidence_id": latest_evidence_id,
            "source_url": source_url,
            "freshness_state": freshness_state,
            "confidence": confidence,
        }
        for row_id, key, latest_evidence_id, source_url, freshness_state, confidence in rows
        if key
    }


def ontology_person_ids_by_owner_key(conn: sqlite3.Connection) -> dict[str, int]:
    if not table_exists(conn, "persons"):
        return {}
    out: dict[str, int] = {}
    rows = conn.execute("select id, github_login from persons").fetchall()
    for row_id, github_login in rows:
        if github_login:
            out[github_owner_hint(github_login)] = int(row_id)
    identity_columns = table_columns(conn, "person_identities")
    if {"person_id", "handle", "external_id", "source_system", "external_kind"}.issubset(identity_columns):
        status_filter = "and coalesce(identity_status, 'active') = 'active'" if "identity_status" in identity_columns else ""
        identity_rows = conn.execute(
            f"""
            select person_id, handle, external_id
              from person_identities
             where source_system = 'github'
               and external_kind = 'github_user'
               {status_filter}
            """
        ).fetchall()
        for person_id, handle, external_id in identity_rows:
            if handle:
                out.setdefault(github_owner_hint(handle), int(person_id))
            if external_id:
                out.setdefault(github_owner_hint(external_id), int(person_id))
    return out


def ontology_insight_ids_by_key(conn: sqlite3.Connection, source_instance: str) -> dict[str, int]:
    if not table_exists(conn, "work_insights"):
        return {}
    rows = conn.execute(
        """
        select id, key
          from work_insights
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_insight'
        """,
        (source_instance,),
    ).fetchall()
    return {str(key): int(row_id) for row_id, key in rows if key}


def ontology_insight_ids_by_signature(conn: sqlite3.Connection, source_instance: str) -> dict[tuple[str, str, str], int]:
    if not table_exists(conn, "work_insights"):
        return {}
    rows = conn.execute(
        """
        select id, subject_kind, subject_key, insight_kind
          from work_insights
         where source_system = 'cubicle_analytics'
           and source_instance = ?
           and external_kind = 'tpm_insight'
           and producer_state = 'current'
        """,
        (source_instance,),
    ).fetchall()
    out: dict[tuple[str, str, str], int] = {}
    for row_id, subject_kind, subject_key, insight_kind in rows:
        key = (str(subject_kind), str(subject_key), str(insight_kind))
        out.setdefault(key, int(row_id))
    return out


def upsert_row(conn: sqlite3.Connection, table_name: str, values: dict[str, Any], conflict_column: str) -> None:
    columns = list(values.keys())
    placeholders = ", ".join(["?"] * len(columns))
    immutable_on_update = {"created_at"} if table_name == "work_dependency_endpoints" else set()
    assignments = ", ".join(f"{column} = excluded.{column}" for column in columns if column != conflict_column and column not in immutable_on_update)
    sql = f"""
        insert into {table_name} ({", ".join(columns)})
        values ({placeholders})
        on conflict({conflict_column}) do update set {assignments}
    """
    conn.execute(sql, [sqlite_value(values[column]) for column in columns])


def sqlite_value(value: Any) -> Any:
    if value == "":
        return None
    if isinstance(value, bool):
        return 1 if value else 0
    if isinstance(value, float) and math.isnan(value):
        return None
    return value


def source_insight_keys_for_group(group: pd.DataFrame, source_instance: str) -> set[str]:
    if group.empty or not source_instance:
        return set()
    if "insight_key" in group.columns:
        return {first_nonempty([value]) for value in group["insight_key"].tolist() if first_nonempty([value])}
    keys: set[str] = set()
    for _, row in group.iterrows():
        insight_kind = first_nonempty([row.get("insight_kind")])
        subject_kind = first_nonempty([row.get("subject_kind")])
        subject_key = first_nonempty([row.get("subject_key")])
        if not insight_kind or not subject_kind or not subject_key:
            continue
        identity_key = first_nonempty([row.get("identity_key")]) or "default"
        digest = analytics_insight_digest([insight_kind, subject_kind, subject_key, identity_key])
        keys.add(f"work-insight:cubicle-analytics:{source_instance}:{digest}")
    return keys


def analytics_insight_digest(parts: list[Any]) -> str:
    payload = "\x1f".join(str(part or "") for part in parts)
    return hashlib.sha256(payload.encode("utf-8")).hexdigest()[:24]


def safe_float(value: Any) -> float:
    try:
        result = float(value)
    except (TypeError, ValueError):
        return 0.0
    if math.isnan(result):
        return 0.0
    return result


def metric_int(frame: pd.DataFrame, metric: str) -> int:
    value = metric_text(frame, metric)
    try:
        return int(float(value))
    except (TypeError, ValueError):
        return 0


def metric_float(frame: pd.DataFrame, metric: str) -> float | None:
    return optional_float(metric_text(frame, metric))


def metric_text(frame: pd.DataFrame, metric: str) -> str:
    if frame.empty or "metric" not in frame.columns or "value" not in frame.columns:
        return ""
    rows = frame[frame["metric"] == metric]
    if rows.empty:
        return ""
    return first_nonempty([rows.iloc[0].get("value")])


def metric_row_int(row: pd.Series, field_name: str) -> int:
    value = first_nonempty([row.get(field_name)])
    try:
        return int(float(value))
    except (TypeError, ValueError):
        return 0


def metric_row_float(row: pd.Series, field_name: str) -> float | None:
    return optional_float(first_nonempty([row.get(field_name)]))


def optional_float(value: Any) -> float | None:
    try:
        result = float(value)
    except (TypeError, ValueError):
        return None
    if math.isnan(result):
        return None
    return result


def write_report(
    path: Path,
    generated_at: str,
    action_items: pd.DataFrame,
    summary: pd.DataFrame,
    readiness: pd.DataFrame,
    owner_rollup: pd.DataFrame,
    standup_summary: pd.DataFrame,
    standup_sections: pd.DataFrame,
    program_register: pd.DataFrame,
    check_signal_readiness: pd.DataFrame,
    transition_signal_readiness: pd.DataFrame,
    work_actions: pd.DataFrame,
    work_action_observations: pd.DataFrame,
) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    top_columns = [
        "priority_rank",
        "urgency",
        "priority_score",
        "raw_priority_score",
        "action_type",
        "decision_state",
        "decision_gate_reason",
        "subject_key",
        "status_signal",
        "current_state",
        "source_observation_status",
        "source_auth_state",
        "source_coverage_kind",
        "required_check_coverage_state",
        "required_check_match_state",
        "required_check_context_count",
        "failing_required_context_count",
        "pending_required_context_count",
        "missing_required_context_count",
        "owner_hint",
        "title",
        "recommended_action",
        "evidence_summary",
        "evidence_ref",
    ]
    terminal_columns = [
        "priority_rank",
        "subject_key",
        "status_signal",
        "current_state",
        "source_observation_status",
        "title",
        "recommended_action",
        "evidence_summary",
        "evidence_ref",
    ]
    review_wait_columns = [
        "priority_rank",
        "urgency",
        "priority_score",
        "subject_key",
        "source_observation_status",
        "owner_hint",
        "title",
        "recommended_action",
        "evidence_summary",
        "evidence_ref",
    ]
    ci_columns = [
        "priority_rank",
        "urgency",
        "priority_score",
        "subject_key",
        "source_observation_status",
        "source_auth_state",
        "source_coverage_kind",
        "required_check_coverage_state",
        "required_check_match_state",
        "required_check_context_count",
        "failing_required_context_count",
        "pending_required_context_count",
        "missing_required_context_count",
        "owner_hint",
        "title",
        "recommended_action",
        "evidence_summary",
        "evidence_ref",
    ]
    model_quality_columns = [
        "priority_rank",
        "urgency",
        "priority_score",
        "title",
        "recommended_action",
        "evidence_summary",
        "evidence_ref",
    ]
    dismissed_columns = [
        "priority_rank",
        "subject_key",
        "title",
        "recommended_action",
        "evidence_summary",
    ]
    owner_columns = [
        "owner_hint",
        "action_count",
        "product_action_count",
        "validation_lead_count",
        "model_or_rule_qa_count",
        "critical_or_high_count",
        "max_priority_score",
        "decision_followup_count",
        "validate_signal_count",
        "ci_check_followup_count",
        "review_wait_followup_count",
        "coverage_limited_count",
        "anonymous_observation_count",
        "top_subjects",
        "recommended_focus",
    ]
    standup_columns = [
        "section_rank",
        "section_kind",
        "urgency",
        "owner_hint",
        "subject_key",
        "action_type",
        "status_signal",
        "summary",
        "recommended_action",
        "evidence_ref",
    ]
    register_columns = [
        "due_bucket",
        "program_status",
        "tpm_bucket",
        "owner_key",
        "owner_source",
        "author_dri",
        "requested_reviewer_keys",
        "subject_key",
        "linked_ticket_keys",
        "linked_pr_keys",
        "decision_needed",
        "decision_state",
        "decision_gate_reason",
        "next_action",
        "blocker_label_state",
        "label_quality",
        "source_coverage_state",
        "transition_state",
        "evidence_ref",
    ]
    check_readiness_columns = [
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
    transition_readiness_columns = [
        "readiness_key",
        "support_level",
        "ready",
        "readiness_state",
        "transition_candidate_count",
        "terminal_transition_candidate_count",
        "latest_terminal_transition_subject_count",
        "superseded_terminal_transition_count",
        "blocking_reason",
        "recommended_action",
    ]
    work_action_columns = [
        "action_key",
        "subject_key",
        "action_type",
        "action_state",
        "decision_state",
        "decision",
        "decision_reason",
        "owner_key",
        "due_bucket",
    ]
    observation_columns = [
        "action_key",
        "observation_kind",
        "source_coverage_state",
        "auth_state",
        "current_state",
        "ci_signal",
        "ci_required_check_coverage_state",
        "ci_required_check_match_state",
        "ci_required_context_count",
        "ci_failing_required_context_count",
        "ci_pending_required_context_count",
        "ci_missing_required_context_count",
        "ci_failing_required_contexts",
        "ci_pending_required_contexts",
        "ci_missing_required_contexts",
        "ci_failing_context_count",
        "ci_pending_context_count",
        "ci_failing_contexts",
        "ci_pending_contexts",
        "supports_action",
        "evidence_ref",
    ]
    lines = [
        "# Flink AI TPM Action Brief",
        "",
        f"Generated at: {generated_at}",
        "",
        "## Standup Snapshot",
        "",
        df_to_markdown(standup_summary),
        "",
        "## Summary",
        "",
        df_to_markdown(summary),
        "",
        "## Standup Sections",
        "",
        df_to_markdown(standup_sections.head(30)[standup_columns]) if not standup_sections.empty else "",
        "",
        "## Owner Load",
        "",
        df_to_markdown(owner_rollup.head(20)[owner_columns]) if not owner_rollup.empty else "",
        "",
        "## Program Register",
        "",
        df_to_markdown(program_register.head(30)[register_columns]) if not program_register.empty else "",
        "",
        "## Check Signal Readiness",
        "",
        df_to_markdown(check_signal_readiness[[column for column in check_readiness_columns if column in check_signal_readiness.columns]])
        if not check_signal_readiness.empty
        else "",
        "",
        "## Transition Signal Readiness",
        "",
        df_to_markdown(transition_signal_readiness[[column for column in transition_readiness_columns if column in transition_signal_readiness.columns]])
        if not transition_signal_readiness.empty
        else "",
        "",
        "## Action Ledger",
        "",
        df_to_markdown(work_actions.head(30)[work_action_columns]) if not work_actions.empty else "",
        "",
        "## Action Observations",
        "",
        df_to_markdown(work_action_observations.head(30)[observation_columns]) if not work_action_observations.empty else "",
        "",
        "## Measurement-backed Product Actions",
        "",
        df_to_markdown(action_items[action_items["decision_state"] == "product_action"].head(15)[top_columns])
        if not action_items.empty
        else "",
        "",
        "## Unmeasured Validation Leads",
        "",
        df_to_markdown(action_items[action_items["decision_state"] == "validation_lead"].head(15)[top_columns])
        if not action_items.empty
        else "",
        "",
        "## Needs Decision",
        "",
        df_to_markdown(program_register[program_register["program_status"] == "needs_decision"].head(15)[register_columns])
        if not program_register.empty
        else "",
        "",
        "## Blocked / Validate",
        "",
        df_to_markdown(program_register[program_register["program_status"] == "validate_signal"].head(15)[register_columns])
        if not program_register.empty
        else "",
        "",
        "## Recently Resolved / Closeout",
        "",
        df_to_markdown(program_register[program_register["program_status"] == "closed_pending_review"].head(15)[register_columns])
        if not program_register.empty
        else "",
        "",
        "## Top Operating Actions",
        "",
        df_to_markdown(action_items[~action_items["action_type"].isin(["verify_resolution", "dismissed_signal"])].head(15)[top_columns])
        if not action_items.empty
        else "",
        "",
        "## Resolution Checks",
        "",
        df_to_markdown(action_items[action_items["action_type"] == "verify_resolution"].head(15)[terminal_columns])
        if not action_items.empty
        else "",
        "",
        "## Requested Reviewer Follow-ups",
        "",
        df_to_markdown(action_items[action_items["action_type"] == "review_wait_followup"].head(15)[review_wait_columns])
        if not action_items.empty
        else "",
        "",
        "## CI Check Follow-ups",
        "",
        df_to_markdown(action_items[action_items["action_type"] == "ci_check_followup"].head(15)[ci_columns])
        if not action_items.empty
        else "",
        "",
        "## Model Quality",
        "",
        df_to_markdown(action_items[action_items["action_type"] == "model_quality_review"].head(15)[model_quality_columns])
        if not action_items.empty
        else "",
        "",
        "## Labeled Dismissals",
        "",
        df_to_markdown(action_items[action_items["action_type"] == "dismissed_signal"].head(15)[dismissed_columns])
        if not action_items.empty
        else "",
        "",
        "## Measurement Readiness",
        "",
        df_to_markdown(readiness),
        "",
        "## Interpretation",
        "",
        "- This is an operating projection, not product truth.",
        "- Failed or partial source rows are coverage-limited and must not support absence claims; anonymous successful observations are counted separately as lower-auth confidence.",
        "- Smoke/candidate/adversarial dismissals may suppress weak operating leads, but they do not count as measurement-grade precision or actionability labels.",
        "- Measurement-backed product actions, unmeasured validation leads, and model/rule QA are separated by `decision_state`; product reads should not collapse them.",
        "- Items marked as resolved still require review labels before they count as model wins.",
        "- Open blocker and forecast items are triage leads; they are not precision claims until labeled. Measurement readiness is computed from live current ontology review rows and excludes stale insights.",
    ]
    path.write_text("\n".join(lines) + "\n")


def read_table(conn: sqlite3.Connection, table_name: str) -> pd.DataFrame:
    if not table_exists(conn, table_name):
        return pd.DataFrame()
    return pd.read_sql_query(f"select * from {table_name}", conn)


def table_exists(conn: sqlite3.Connection, table_name: str) -> bool:
    row = conn.execute("select 1 from sqlite_master where type = 'table' and name = ?", (table_name,)).fetchone()
    return row is not None


def source_status(followup: dict[str, Any]) -> str:
    if not followup:
        return "not_observed"
    if "failed" in str(followup.get("coverage_states") or ""):
        return "source_failure"
    if int(followup.get("fetch_error_count") or 0) > 0:
        return "source_failure"
    if "partial" in str(followup.get("coverage_states") or ""):
        return "observed_partial"
    if int(followup.get("fetch_success_count") or 0) > 0:
        if "anonymous" in str(followup.get("auth_states") or ""):
            return "observed_anonymous"
        return "observed"
    return "not_observed"


def read_review_queue_from_ontology(
    conn: sqlite3.Connection,
    source_instance: str,
    measurement_label_sets: set[str],
) -> pd.DataFrame:
    if not source_instance or not table_exists(conn, "work_insight_reviews"):
        return pd.DataFrame()
    review_source_filter, review_source_params = work_insight_review_source_filter(conn, "wir", source_instance)
    rows = pd.read_sql_query(
        f"""
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
          wi.producer_state,
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
          {review_source_filter}
        """,
        conn,
        params=(source_instance, *review_source_params),
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


def infer_source_instance(run_metadata: pd.DataFrame) -> str:
    metadata = metric_map(run_metadata.rename(columns={"key": "metric"})) if not run_metadata.empty else {}
    fixture_dir = metadata.get("fixture_dir", "")
    if fixture_dir:
        return Path(fixture_dir).name
    return ""


def ensure_not_older_than_followup(generated_at: str, followup_generated_at: str) -> None:
    generated_dt = parse_dt(generated_at)
    followup_dt = parse_dt(followup_generated_at)
    if generated_dt is not None and followup_dt is not None and generated_dt < followup_dt:
        raise SystemExit(f"--generated-at {generated_at} is older than latest follow-up observation {followup_generated_at}")


def choose_outcome_signal(values: list[str]) -> str:
    precedence = [
        "subject_became_terminal",
        "subject_became_closed",
        "subject_state_changed",
        "still_open",
        "no_state_change",
        "not_observed",
    ]
    present = set(values)
    for item in precedence:
        if item in present:
            return item
    return values[0] if values else "not_observed"


def severity_from_rank(rank: int) -> str:
    for label, value in SEVERITY_RANK.items():
        if value == rank:
            return label
    return "info"


def first_nonempty(values: Any) -> str:
    for value in values:
        if value is None:
            continue
        if isinstance(value, float) and math.isnan(value):
            continue
        text = str(value)
        if text:
            return text
    return ""


def infer_generated_at(followups: pd.DataFrame) -> str:
    if not followups.empty and "observed_at" in followups.columns:
        observed = [value for value in followups["observed_at"].dropna().tolist() if str(value)]
        if observed:
            return max(str(value) for value in observed)
    return datetime.now(timezone.utc).isoformat()


def latest_observed_at(*tables: pd.DataFrame) -> str:
    latest: datetime | None = None
    latest_text = ""
    for table in tables:
        if table.empty or "observed_at" not in table.columns:
            continue
        for value in table["observed_at"].dropna().tolist():
            text = str(value)
            dt = parse_dt(text)
            if dt is None:
                continue
            if latest is None or dt > latest:
                latest = dt
                latest_text = text
    return latest_text


def metric_map(df: pd.DataFrame) -> dict[str, str]:
    if df.empty or "metric" not in df.columns or "value" not in df.columns:
        return {}
    return {str(row.metric): str(row.value) for row in df.itertuples(index=False)}


def extract_label_set(review_key: Any) -> str:
    parts = first_nonempty([review_key]).split(":")
    if len(parts) >= 5 and parts[0] == "work-insight-review" and parts[1] == "cubicle-evaluation":
        return parts[-2]
    return ""


def infer_label_quality_from_review(row: pd.Series) -> str:
    explicit = first_nonempty([row.get("label_quality")])
    if explicit in {"adversarial", "candidate", "gold", "smoke"}:
        return explicit
    if first_nonempty([row.get("review_kind")]) == "human_assessment":
        return "gold"
    reviewer_kind = first_nonempty([row.get("reviewer_kind")])
    if reviewer_kind.startswith("imported_"):
        quality = reviewer_kind.removeprefix("imported_")
        if quality in {"adversarial", "candidate", "gold", "smoke"}:
            return quality
    text = " ".join(
        [
            first_nonempty([row.get("label_set")]),
            first_nonempty([row.get("reviewer_key")]),
            first_nonempty([row.get("label_source_url")]),
            first_nonempty([row.get("rationale")]),
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
    stored = first_nonempty([row.get("stored_measurement_eligible")]).lower()
    if stored in {"false", "0", "no"}:
        return False
    if first_nonempty([row.get("review_kind")]) == "human_assessment":
        return True
    if first_nonempty([row.get("label_quality")]) in MEASUREMENT_LABEL_QUALITIES:
        return True
    return False


def stored_measurement_eligible(row: pd.Series) -> bool:
    value = first_nonempty([row.get("stored_measurement_eligible")]).lower()
    return value in {"true", "1", "yes"}


def measurement_eligible_value(
    review_kind: str,
    label_quality: str,
    label_set: str,
    measurement_label_sets: set[str],
) -> bool:
    return is_measurement_label(
        pd.Series(
            {
                "review_kind": review_kind,
                "label_quality": label_quality,
                "label_set": label_set,
            }
        ),
        measurement_label_sets,
    )


def backfill_review_measurement_eligibility(
    conn: sqlite3.Connection,
    source_instance: str,
    measurement_label_sets: set[str],
) -> None:
    if not source_instance or not column_exists(conn, "work_insight_reviews", "measurement_eligible"):
        return
    review_source_filter, review_source_params = work_insight_review_source_filter(conn, "wir", source_instance)
    rows = conn.execute(
        f"""
        select wir.id, wir.review_kind, coalesce(wir.label_quality, ''),
               coalesce(wir.label_set, ''), wir.key,
               coalesce(wir.measurement_eligible, 0)
          from work_insight_reviews wir
          join work_insights wi on wi.id = wir.work_insight_id
         where wi.source_system = 'cubicle_analytics'
           and wi.source_instance = ?
           and wi.external_kind = 'tpm_insight'
           {review_source_filter}
        """,
        (source_instance, *review_source_params),
    ).fetchall()
    for review_id, review_kind, label_quality, label_set, review_key, stored_eligible in rows:
        if not label_set:
            label_set = extract_label_set(review_key)
        eligible = is_measurement_label(
            pd.Series(
                {
                    "review_kind": review_kind,
                    "label_quality": label_quality,
                    "label_set": label_set,
                    "stored_measurement_eligible": stored_eligible,
                }
            ),
            measurement_label_sets,
        )
        conn.execute(
            "update work_insight_reviews set measurement_eligible = ? where id = ?",
            (1 if eligible else 0, review_id),
        )
    conn.commit()


def work_insight_review_source_filter(
    conn: sqlite3.Connection,
    table_alias: str,
    source_instance: str,
) -> tuple[str, list[str]]:
    required = {"source_system", "source_instance", "external_kind"}
    if not required.issubset(table_columns(conn, "work_insight_reviews")):
        return "", []
    alias = table_alias.strip()
    return (
        f"""
          and {alias}.source_instance = ?
          and (
            ({alias}.source_system = 'cubicle_analytics' and {alias}.external_kind = 'tpm_insight_review')
            or ({alias}.source_system = 'cubicle_evaluation' and {alias}.external_kind = 'tpm_review_label')
          )
        """,
        [source_instance],
    )


def column_exists(conn: sqlite3.Connection, table_name: str, column_name: str) -> bool:
    return column_name in table_columns(conn, table_name)


def table_columns(conn: sqlite3.Connection, table_name: str) -> set[str]:
    if not table_exists(conn, table_name):
        return set()
    return {str(row[1]) for row in conn.execute(f"pragma table_info({table_name})").fetchall()}


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


def stable_digest(parts: list[str]) -> str:
    digest = hashlib.sha256("\n".join(str(part) for part in parts).encode("utf-8")).hexdigest()
    return digest[:24]


def empty_action_items() -> pd.DataFrame:
    return pd.DataFrame(
        columns=[
            "action_key",
            "priority_rank",
            "urgency",
            "priority_score",
            "raw_priority_score",
            "action_type",
            "decision_state",
            "decision_gate_reason",
            "subject_kind",
            "subject_key",
            "insight_kinds",
            "source_insight_keys",
            "source_link_insight_kinds",
            "severity",
            "severity_rank",
            "status_signal",
            "baseline_state",
            "current_state",
            "source_observation_status",
            "source_auth_state",
            "source_coverage_kind",
            "required_check_coverage_state",
            "required_check_match_state",
            "required_check_context_count",
            "failing_required_context_count",
            "pending_required_context_count",
            "missing_required_context_count",
            "failing_required_contexts",
            "pending_required_contexts",
            "missing_required_contexts",
            "failing_context_count",
            "pending_context_count",
            "failing_contexts",
            "pending_contexts",
            "title",
            "why_now",
            "recommended_action",
            "owner_hint",
            "source_url",
            "evidence_ref",
            "score",
            "confidence",
            "needs_human_review",
            "open_review_request_count",
            "reviewed_count",
            "candidate_dismissed_kinds",
            "operational_dismissed_kinds",
            "evidence_summary",
            "generated_at",
        ]
    )


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


if __name__ == "__main__":
    main()
