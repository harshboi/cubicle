#!/usr/bin/env python3
"""Build an AI-ready Cubicle graph-context brief from persisted ontology rows.

This is intentionally not another product packet. It is a small PoC for the
AI-first path: resolve a workstream, collect a bounded evidence-bearing graph
neighborhood, and emit a cited context bundle plus deterministic brief scaffold
that can be handed to an LLM.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shlex
import sqlite3
import subprocess
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable


DEFAULT_ITEM_LIMIT = 20
DEFAULT_EDGE_LIMIT = 80
DEFAULT_EVIDENCE_LIMIT = 80
DEFAULT_TRAVERSAL_DEPTH = 2
PROMPT_TEXT_LIMIT = 320
REQUIRED_BRIEF_SECTIONS = ["# Operating Brief", "## Confirmed Facts", "## Validation Leads", "## What Not To Claim"]
MAX_BRIEF_BULLETS_PER_SECTION = 3
BOUNDED_GRAPH_MAX_BRIEF_BULLETS_PER_SECTION = 4
GRAPH_BRIEF_MODEL_METHOD = "bounded_graph_context_to_cited_brief"
GRAPH_BRIEF_PROMPT_MODES = {"operating", "generic"}
PROMPT_ANALYTICS_METRICS = {
    "forecast_summary": [
        "eta_forecast_ready",
        "eta_readiness_state",
        "eta_primary_blocker",
        "eta_next_evidence_needed",
        "eta_best_candidate_model",
        "eta_best_kfold_model",
        "eta_best_chronological_model",
        "risk_triage_product_safe",
        "risk_triage_lift_at_10pct",
        "risk_triage_safe_use",
        "risk_triage_ready_for_product_action",
        "open_pr_count",
        "high_risk_open_pr_count",
    ],
    "measurement_readiness": [
        "ready_to_measure_precision",
        "ready_to_measure_actionability",
        "evaluation_label_row_count",
        "measurement_coverage_rate",
        "actionability_rate",
        "actionable_count",
        "open_review_request_count",
        "gold_label_count",
    ],
    "measurement_queue_summary": [
        "measurement_queue_count",
        "queue_risk_actionability",
        "queue_blocker_precision",
    ],
}


PROMPT_ROW_FIELDS = {
    "work_program_items": [
        "key",
        "subject_kind",
        "subject_key",
        "title",
        "program_status",
        "decision_state",
        "due_bucket",
        "owner_key",
        "next_action",
        "risk_score",
        "source_coverage_state",
        "latest_evidence_id",
        "rank_score",
        "_table",
    ],
    "work_actions": [
        "key",
        "action_type",
        "action_state",
        "decision_state",
        "decision_reason",
        "subject_kind",
        "subject_key",
        "owner_key",
        "due_bucket",
        "latest_evidence_id",
        "rank_score",
        "_table",
    ],
    "work_insights": [
        "key",
        "insight_kind",
        "severity",
        "producer_state",
        "subject_kind",
        "subject_key",
        "title",
        "details",
        "recommended_action",
        "score",
        "score_explanation",
        "latest_evidence_id",
        "rank_score",
        "_table",
    ],
    "work_item_forecasts": [
        "key",
        "forecast_kind",
        "subject_kind",
        "subject_key",
        "subject_state",
        "forecast_method",
        "model_name",
        "age_days",
        "predicted_remaining_days",
        "overdue_days",
        "risk_score",
        "risk_band",
        "readiness_state",
        "ready_for_eta",
        "readiness_reason",
        "latest_evidence_id",
        "rank_score",
        "_table",
    ],
    "work_dependency_edges": [
        "key",
        "edge_kind",
        "relationship_authority",
        "canonical_relationship_kind",
        "from_kind",
        "from_key",
        "to_kind",
        "to_key",
        "risk_signal",
        "source_coverage_state",
        "latest_evidence_id",
        "rank_score",
        "_table",
    ],
    "evidence": [
        "key",
        "claim_kind",
        "claim_target_kind",
        "claim_field",
        "relationship_kind",
        "locator_kind",
        "proof_state",
        "source_system",
        "external_kind",
        "external_id",
        "observed_at",
        "_table",
    ],
    "quality_gates": ["gate_key", "gate_state", "blocking", "detail", "recommended_action", "generated_at", "_table"],
    "evidence_needs": [
        "gate_key",
        "evidence_kind",
        "priority",
        "execution_state",
        "target_kind",
        "target_key",
        "recommended_action",
        "missing_count",
        "generated_at",
        "_table",
    ],
    "graph_objects": [
        "key",
        "object_type",
        "title",
        "source",
        "source_instance",
        "visibility",
        "freshness_state",
        "proof_state",
        "claim_allowed",
        "claim_gate_reason",
        "source_coverage_state",
        "seed_reachable",
        "seed_distance",
        "rank_score",
        "_table",
    ],
    "graph_associations": [
        "key",
        "association_type",
        "endpoint_phrase",
        "from_kind",
        "from_key",
        "to_kind",
        "to_key",
        "evidence_key",
        "confidence",
        "visibility",
        "freshness_state",
        "proof_state",
        "claim_allowed",
        "claim_gate_reason",
        "seed_reachable",
        "seed_distance",
        "_table",
    ],
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ontology-db", type=Path)
    parser.add_argument("--analytics-db", type=Path)
    parser.add_argument("--graph-context-json", type=Path)
    parser.add_argument("--bounded-graph-context-json", type=Path)
    parser.add_argument("--workstream-key")
    parser.add_argument("--source-instance")
    parser.add_argument("--item-limit", type=int, default=DEFAULT_ITEM_LIMIT)
    parser.add_argument("--edge-limit", type=int, default=DEFAULT_EDGE_LIMIT)
    parser.add_argument("--evidence-limit", type=int, default=DEFAULT_EVIDENCE_LIMIT)
    parser.add_argument("--traversal-depth", type=int, default=DEFAULT_TRAVERSAL_DEPTH)
    parser.add_argument("--context-json", type=Path)
    parser.add_argument("--brief-md", type=Path)
    parser.add_argument("--typed-row-baseline-md", type=Path)
    parser.add_argument("--generic-baseline-md", type=Path)
    parser.add_argument("--prompt-md", type=Path)
    parser.add_argument("--prompt-mode", choices=["operating", "generic"], default="operating")
    parser.add_argument("--llm-command")
    parser.add_argument("--mlx-model")
    parser.add_argument("--mlx-python", default="/Users/harsh/.venv-vllm-metal/bin/python")
    parser.add_argument("--llm-max-tokens", type=int, default=8192)
    parser.add_argument("--llm-model-name")
    parser.add_argument("--llm-brief-md", type=Path)
    parser.add_argument("--repaired-brief-md", type=Path)
    parser.add_argument("--evaluation-json", type=Path)
    parser.add_argument("--golden-json", type=Path)
    parser.add_argument("--compare-answers-json", type=Path)
    parser.add_argument("--comparison-json", type=Path)
    parser.add_argument("--require-promotion-gates", action="store_true")
    parser.add_argument("--llm-timeout-seconds", type=int, default=120)
    parser.add_argument("--persist-ai-insight", action="store_true")
    parser.add_argument("--generated-at")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    if args.compare_answers_json:
        if not args.golden_json:
            raise SystemExit("--golden-json is required with --compare-answers-json")
        if not args.comparison_json:
            raise SystemExit("--comparison-json is required with --compare-answers-json")
        comparison = evaluate_golden_answer_comparison(
            json.loads(args.golden_json.read_text(encoding="utf-8")),
            json.loads(args.compare_answers_json.read_text(encoding="utf-8")),
            base_dir=args.compare_answers_json.parent,
        )
        args.comparison_json.parent.mkdir(parents=True, exist_ok=True)
        args.comparison_json.write_text(json.dumps(comparison, indent=2, sort_keys=True), encoding="utf-8")
        if args.require_promotion_gates:
            if not comparison["promotion_gates"]:
                raise SystemExit("--require-promotion-gates requires at least one promotion gate")
            if not comparison["passes_promotion_gates"]:
                raise SystemExit("promotion gates failed")
        return
    if args.graph_context_json and args.bounded_graph_context_json:
        raise SystemExit("--graph-context-json and --bounded-graph-context-json are mutually exclusive")
    if not args.workstream_key and not (args.graph_context_json or args.bounded_graph_context_json):
        raise SystemExit(
            "--workstream-key is required unless --compare-answers-json, --graph-context-json, or --bounded-graph-context-json is provided"
        )
    if not args.context_json:
        raise SystemExit("--context-json is required unless --compare-answers-json is provided")
    if not args.brief_md:
        raise SystemExit("--brief-md is required unless --compare-answers-json is provided")
    if (args.llm_command or args.mlx_model) and not (args.graph_context_json or args.bounded_graph_context_json):
        raise SystemExit("--graph-context-json or --bounded-graph-context-json is required for --llm-command or --mlx-model")
    if args.persist_ai_insight and not args.graph_context_json:
        raise SystemExit("--graph-context-json is required for --persist-ai-insight")
    if args.bounded_graph_context_json:
        context = build_context_bundle_from_bounded_graph_context_json(args.bounded_graph_context_json)
    elif args.graph_context_json:
        context = build_context_bundle_from_graph_context_json(args.graph_context_json, analytics_db=args.analytics_db)
    else:
        if not args.ontology_db:
            raise SystemExit("--ontology-db is required unless --graph-context-json is provided")
        context = build_context_bundle(
            args.ontology_db,
            analytics_db=args.analytics_db,
            workstream_key=args.workstream_key,
            source_instance=args.source_instance,
            item_limit=args.item_limit,
            edge_limit=args.edge_limit,
            evidence_limit=args.evidence_limit,
            traversal_depth=args.traversal_depth,
        )
    brief = render_brief(context)
    args.context_json.parent.mkdir(parents=True, exist_ok=True)
    args.brief_md.parent.mkdir(parents=True, exist_ok=True)
    args.context_json.write_text(json.dumps(context, indent=2, sort_keys=True), encoding="utf-8")
    args.brief_md.write_text(brief, encoding="utf-8")
    if args.typed_row_baseline_md:
        args.typed_row_baseline_md.parent.mkdir(parents=True, exist_ok=True)
        args.typed_row_baseline_md.write_text(render_typed_row_baseline(context), encoding="utf-8")
    if args.generic_baseline_md:
        args.generic_baseline_md.parent.mkdir(parents=True, exist_ok=True)
        args.generic_baseline_md.write_text(render_generic_graph_baseline(context), encoding="utf-8")
    prompt = render_prompt(context, mode=args.prompt_mode) if args.prompt_md or args.llm_command else None
    if args.prompt_md and prompt is not None:
        args.prompt_md.parent.mkdir(parents=True, exist_ok=True)
        args.prompt_md.write_text(prompt, encoding="utf-8")
    llm_command = args.llm_command
    if args.mlx_model:
        if llm_command:
            raise SystemExit("--mlx-model and --llm-command are mutually exclusive")
        llm_command = mlx_lm_command(args.mlx_python, args.mlx_model, args.llm_max_tokens)
    if llm_command:
        if not args.llm_brief_md:
            raise SystemExit("--llm-brief-md is required when --llm-command is provided")
        args.llm_brief_md.parent.mkdir(parents=True, exist_ok=True)
        args.llm_brief_md.write_text(
            run_llm_command(llm_command, prompt or render_prompt(context, mode=args.prompt_mode), args.llm_timeout_seconds),
            encoding="utf-8",
        )
    if args.evaluation_json or args.persist_ai_insight or args.golden_json:
        if not args.llm_brief_md:
            raise SystemExit("--llm-brief-md is required when --evaluation-json, --golden-json, or --persist-ai-insight is provided")
        raw_answer_text = args.llm_brief_md.read_text(encoding="utf-8")
        golden_spec = json.loads(args.golden_json.read_text(encoding="utf-8")) if args.golden_json else None
        answer_text = raw_answer_text
        raw_evaluation = evaluate_brief_for_gates(context, raw_answer_text, golden_spec)
        if args.repaired_brief_md:
            answer_text = repair_llm_brief(context, answer_text)
            args.repaired_brief_md.parent.mkdir(parents=True, exist_ok=True)
            args.repaired_brief_md.write_text(answer_text, encoding="utf-8")
        evaluation = evaluate_brief_for_gates(context, answer_text, golden_spec)
        evaluation["prompt_mode"] = args.prompt_mode
        evaluation["repair_applied"] = bool(args.repaired_brief_md)
        evaluation["evaluated_answer_kind"] = "repaired" if args.repaired_brief_md else "raw"
        if args.repaired_brief_md:
            evaluation["repair_changed_answer"] = raw_answer_text != answer_text
            evaluation["raw_answer_eval"] = raw_evaluation
        if args.persist_ai_insight:
            if not args.ontology_db:
                raise SystemExit("--ontology-db is required when --persist-ai-insight is provided")
            if not raw_evaluation["passes_eval"]:
                raise SystemExit("--persist-ai-insight requires the raw brief to pass configured evaluation gates; repair cannot be the only passing path")
            if not evaluation["passes_eval"]:
                raise SystemExit("--persist-ai-insight requires a brief that passes configured evaluation gates")
            with sqlite3.connect(args.ontology_db) as conn:
                conn.row_factory = sqlite3.Row
                evaluation["persisted_ai_insight"] = persist_ai_graph_brief_insight(
                    conn,
                    context,
                    answer_text,
                    evaluation,
                    llm_command=llm_command,
                    llm_model_name=args.llm_model_name,
                    generated_at=args.generated_at,
                    prompt_mode=args.prompt_mode,
                )
        if not args.evaluation_json:
            return
        args.evaluation_json.parent.mkdir(parents=True, exist_ok=True)
        args.evaluation_json.write_text(json.dumps(evaluation, indent=2, sort_keys=True), encoding="utf-8")


def build_context_bundle(
    ontology_db: Path,
    *,
    analytics_db: Path | None,
    workstream_key: str,
    source_instance: str | None,
    item_limit: int = DEFAULT_ITEM_LIMIT,
    edge_limit: int = DEFAULT_EDGE_LIMIT,
    evidence_limit: int = DEFAULT_EVIDENCE_LIMIT,
    traversal_depth: int = DEFAULT_TRAVERSAL_DEPTH,
) -> dict[str, Any]:
    with sqlite3.connect(ontology_db) as conn:
        conn.row_factory = sqlite3.Row
        resolved_source = source_instance or latest_source_instance(conn)
        workstream_keys = workstream_sql_keys(workstream_key)
        items = work_program_items(conn, workstream_keys, resolved_source, bounded(item_limit, 1, 100))
        seed_subject_keys = sorted({str(row["subject_key"]) for row in items if row.get("subject_key")})
        traversal = traverse_dependency_neighborhood(
            conn,
            seed_subject_keys,
            resolved_source,
            depth=bounded(traversal_depth, 0, 4),
            edge_limit=bounded(edge_limit, 1, 500),
        )
        subject_keys = traversal["subject_keys"]
        action_keys = sorted({str(row["work_action_key"]) for row in items if row.get("work_action_key")})
        actions = work_actions(conn, subject_keys, action_keys, resolved_source, bounded(item_limit, 1, 100))
        insights = work_insights(conn, subject_keys, resolved_source, bounded(item_limit, 1, 100))
        forecasts = work_item_forecasts(conn, subject_keys, resolved_source, bounded(item_limit, 1, 100))
        edges = traversal["edges"]
        evidence_ids = collect_evidence_ids(items, actions, insights, forecasts, edges)
        evidence = evidence_rows(conn, evidence_ids, resolved_source, bounded(evidence_limit, 1, 500))
        quality_gates = latest_quality_gates(conn, workstream_keys, resolved_source)
        evidence_needs = latest_evidence_needs(conn, workstream_keys, resolved_source, 20)

    analytics = analytics_context(analytics_db) if analytics_db else {}
    context = {
        "seed": {
            "object_type": "workstream",
            "key": canonical_workstream_key(workstream_key),
            "source_instance": resolved_source,
        },
        "context_hash_inputs": {
            "ontology_db": str(ontology_db),
            "analytics_db": str(analytics_db) if analytics_db else None,
            "workstream_key": canonical_workstream_key(workstream_key),
            "source_instance": resolved_source,
            "item_limit": bounded(item_limit, 1, 100),
            "edge_limit": bounded(edge_limit, 1, 500),
            "evidence_limit": bounded(evidence_limit, 1, 500),
            "traversal_depth": bounded(traversal_depth, 0, 4),
        },
        "traversal": {
            "seed_subject_keys": seed_subject_keys,
            "reached_subject_keys": subject_keys,
            "depth": bounded(traversal_depth, 0, 4),
            "edge_count": len(edges),
        },
        "rows": {
            "work_program_items": items,
            "work_actions": actions,
            "work_insights": insights,
            "work_item_forecasts": forecasts,
            "work_dependency_edges": edges,
            "evidence": evidence,
            "quality_gates": quality_gates,
            "evidence_needs": evidence_needs,
        },
        "analytics": analytics,
        "guardrails": guardrails(quality_gates, analytics),
    }
    context["context_hash"] = stable_hash(context)
    context["llm_task"] = llm_task()
    return context


def build_context_bundle_from_graph_context_json(graph_context_json: Path, *, analytics_db: Path | None = None) -> dict[str, Any]:
    payload = json.loads(graph_context_json.read_text(encoding="utf-8"))
    graph_context = extract_graph_context_payload(payload)
    analytics = analytics_context(analytics_db) if analytics_db else analytics_from_graph_context(graph_context)
    rows = {
        "work_program_items": graph_context_rows(graph_context.get("items", []), "work_program_items"),
        "work_actions": graph_context_rows(graph_context.get("actions", []), "work_actions"),
        "work_insights": graph_context_rows(graph_context.get("insights", []), "work_insights"),
        "work_item_forecasts": graph_context_rows(graph_context.get("forecasts", []), "work_item_forecasts"),
        "work_dependency_edges": graph_context_rows(graph_context.get("dependencyEdges", []), "work_dependency_edges"),
        "quality_gates": graph_context_rows(graph_context.get("qualityGates", []), "work_program_quality_gates"),
        "evidence_needs": graph_context_rows(graph_context.get("evidenceNeeds", []), "work_program_evidence_needs"),
        "evidence": graph_context_evidence_rows(graph_context),
    }
    workstream_key = str(graph_context.get("workstreamKey") or "")
    source_instance = graph_context.get("sourceInstance")
    context = {
        "seed": {
            "object_type": "workstream",
            "key": canonical_workstream_key(workstream_key),
            "source_instance": source_instance,
        },
        "context_hash_inputs": {
            "graph_context_hash": graph_context.get("contextHash"),
            "scope_mode": graph_context.get("scopeMode"),
            "run_key": graph_context.get("runKey"),
            "generated_at": graph_context.get("generatedAt"),
            "source_instance": source_instance,
        },
        "traversal": {
            "seed_subject_keys": sorted({str(row.get("subject_key")) for row in rows["work_program_items"] if row.get("subject_key")}),
            "reached_subject_keys": graph_context.get("reachedSubjectKeys", []),
            "depth": int(graph_context.get("traversalDepth") or 0),
            "edge_count": int(graph_context.get("dependencyEdgeCount") or len(rows["work_dependency_edges"])),
        },
        "rows": rows,
        "analytics": analytics,
        "guardrails": graph_context_guardrails(graph_context, analytics),
        "citations": graph_context.get("citations", []),
        "allowed_citations": graph_context.get("allowedCitations", []),
        "graph_context": graph_context,
        "context_hash": str(graph_context.get("contextHash") or stable_digest([json.dumps(graph_context, sort_keys=True)]))[:16],
        "llm_task": graph_context.get("llmTask") or llm_task(),
    }
    return context


def build_context_bundle_from_bounded_graph_context_json(bounded_graph_context_json: Path) -> dict[str, Any]:
    payload = json.loads(bounded_graph_context_json.read_text(encoding="utf-8"))
    bounded_context = extract_bounded_graph_context_payload(payload)
    seed = bounded_graph_seed(bounded_context)
    objects = bounded_graph_objects(bounded_context.get("objects", []))
    associations = bounded_graph_associations(bounded_context.get("associations", []), objects)
    annotate_bounded_graph_seed_relevance(str(seed.get("key") or "object:unknown"), objects, associations)
    evidence = bounded_graph_evidence_rows(bounded_context.get("evidence", []))
    coverage = bounded_graph_coverage(bounded_context)
    context_hash = str(bounded_context.get("contextHash") or bounded_context.get("context_hash") or "")[:16]
    if not context_hash:
        context_hash = stable_digest([json.dumps(bounded_context, sort_keys=True)])[:16]
    seed_key = str(seed.get("key") or "object:unknown")
    depth = int(bounded_context.get("depth") or bounded_context.get("traversalDepth") or bounded_context.get("traversal_depth") or 0)
    guardrail_text = [str(value).strip() for value in bounded_context.get("guardrails", []) if str(value).strip()]
    if not coverage["absence_claims_allowed"]:
        guardrail_text.append("Source coverage gates absence claims; missing neighbors are unknown, not absent.")
    citations = bounded_graph_citations(context_hash, seed, objects, associations, coverage, bounded_context)
    context = {
        "seed": {
            "object_type": str(seed.get("object_type") or seed.get("objectType") or "object"),
            "key": seed_key,
            "source_instance": str(bounded_context.get("sourceInstance") or bounded_context.get("source_instance") or "").strip() or None,
        },
        "context_hash_inputs": {
            "bounded_graph_context_hash": context_hash,
            "scope_mode": str(bounded_context.get("scopeMode") or bounded_context.get("scope_mode") or "bounded_graph_context"),
            "depth": depth,
            "limit_per_object": bounded_context.get("limitPerObject") or bounded_context.get("limit_per_object"),
        },
        "traversal": {
            "seed_subject_keys": [seed_key],
            "reached_subject_keys": sorted({str(row.get("key")) for row in objects if str(row.get("key") or "").strip()}),
            "depth": depth,
            "edge_count": len(associations),
            "limit_per_object": bounded_context.get("limitPerObject") or bounded_context.get("limit_per_object"),
        },
        "rows": {
            "graph_objects": objects,
            "graph_associations": associations,
            "evidence": evidence,
        },
        "analytics": {
            "source_coverage": {
                "coverage_state": {"value": coverage["coverage_state"], "note": coverage["summary"]},
                "absence_claims_allowed": {"value": str(coverage["absence_claims_allowed"]).lower(), "note": coverage["absence_claim_gate_reason"]},
            },
        },
        "guardrails": sorted({text for text in guardrail_text if text}),
        "citations": citations,
        "allowed_citations": sorted({str(row.get("ref")) for row in citations if str(row.get("ref") or "").strip()}),
        "bounded_graph_context": bounded_context,
        "context_hash": context_hash,
        "llm_task": (
            "Given the bounded generic graph context, summarize only the reached objects and associations. "
            "Keep source coverage and absence-claim limits explicit, and do not introduce domain-specific claims unless they are present in the graph rows."
        ),
    }
    return context


def extract_graph_context_payload(payload: Any) -> dict[str, Any]:
    if not isinstance(payload, dict):
        raise ValueError("graph context JSON must contain an object")
    if isinstance(payload.get("data"), dict) and isinstance(payload["data"].get("workProgramGraphContext"), dict):
        return payload["data"]["workProgramGraphContext"]
    if isinstance(payload.get("workProgramGraphContext"), dict):
        return payload["workProgramGraphContext"]
    required = {"workstreamKey", "contextHash"}
    if required.issubset(payload):
        return payload
    raise ValueError("graph context JSON must be a WorkProgramGraphContext object or GraphQL response")


def extract_bounded_graph_context_payload(payload: Any) -> dict[str, Any]:
    if not isinstance(payload, dict):
        raise ValueError("bounded graph context JSON must contain an object")
    if isinstance(payload.get("data"), dict) and isinstance(payload["data"].get("boundedGraphContext"), dict):
        return payload["data"]["boundedGraphContext"]
    if isinstance(payload.get("boundedGraphContext"), dict):
        return payload["boundedGraphContext"]
    if isinstance(payload.get("bounded_graph_context"), dict):
        return payload["bounded_graph_context"]
    if "objects" in payload and "associations" in payload:
        return payload
    raise ValueError("bounded graph context JSON must be a BoundedGraphContext object or GraphQL-style response")


def bounded_graph_seed(context: dict[str, Any]) -> dict[str, Any]:
    seed = context.get("seed") or context.get("start") or {}
    return seed if isinstance(seed, dict) else {}


def bounded_graph_objects(values: Any) -> list[dict[str, Any]]:
    if not isinstance(values, list):
        return []
    out: list[dict[str, Any]] = []
    for value in values:
        if not isinstance(value, dict):
            continue
        key = str(value.get("key") or "").strip()
        if not key:
            continue
        row = {
            "_table": "graph_objects",
            "key": key,
            "object_type": str(value.get("objectType") or value.get("object_type") or "object").strip() or "object",
            "title": value.get("title") or key,
            "source": value.get("source"),
            "source_instance": value.get("sourceInstance") or value.get("source_instance"),
            "external_id": value.get("externalID") or value.get("external_id"),
            "visibility": value.get("visibility"),
            "freshness_state": value.get("freshnessState") or value.get("freshness_state"),
            "proof_state": value.get("proofState") or value.get("proof_state") or "typed_graph_row",
            "claim_allowed": bool(value.get("claimAllowed", value.get("claim_allowed", True))),
            "claim_gate_reason": value.get("claimGateReason") or value.get("claim_gate_reason") or "bounded_graph_object",
            "source_coverage_state": value.get("sourceCoverageState") or value.get("source_coverage_state"),
            "rank_score": value.get("rankScore") or value.get("rank_score"),
        }
        apply_bounded_graph_object_claim_policy(row)
        out.append(row)
    return out


def apply_bounded_graph_object_claim_policy(row: dict[str, Any]) -> None:
    gate_reason = bounded_graph_object_claim_gate_reason(row)
    if not gate_reason:
        return
    row["claim_allowed"] = False
    row["claim_gate_reason"] = gate_reason


def bounded_graph_object_claim_gate_reason(row: dict[str, Any]) -> str:
    freshness_state = str(row.get("freshness_state") or "").strip()
    if freshness_state == "partial":
        return "object_partial_requires_hydration"
    if freshness_state in {"stale", "superseded", "tombstoned"}:
        return "object_not_current"
    visibility = str(row.get("visibility") or "").strip()
    if visibility and visibility != "public":
        return "object_visibility_restricted"
    if str(row.get("source") or "").strip() in {"cubicle_ai", "generated", "llm"}:
        return "object_generated_requires_source_evidence"
    return ""


def bounded_graph_associations(values: Any, objects: list[dict[str, Any]] | None = None) -> list[dict[str, Any]]:
    if not isinstance(values, list):
        return []
    out: list[dict[str, Any]] = []
    for index, value in enumerate(values):
        if not isinstance(value, dict):
            continue
        from_ref = value.get("from") if isinstance(value.get("from"), dict) else {}
        to_ref = value.get("to") if isinstance(value.get("to"), dict) else {}
        from_key = str(from_ref.get("key") or value.get("from_key") or "").strip()
        to_key = str(to_ref.get("key") or value.get("to_key") or "").strip()
        if not from_key or not to_key:
            continue
        association_type = str(value.get("associationType") or value.get("association_type") or "related_to").strip() or "related_to"
        metadata = value.get("metadata") if isinstance(value.get("metadata"), dict) else {}
        out.append(
            {
                "_table": "graph_associations",
                "key": str(value.get("key") or f"association:{from_key}:{association_type}:{to_key}:{index + 1}"),
                "association_type": association_type,
                "endpoint_phrase": f"`{from_key}` -> `{to_key}`",
                "from_kind": from_ref.get("objectType") or from_ref.get("object_type") or value.get("from_kind") or "object",
                "from_key": from_key,
                "to_kind": to_ref.get("objectType") or to_ref.get("object_type") or value.get("to_kind") or "object",
                "to_key": to_key,
                "evidence_key": metadata.get("evidenceKey") or metadata.get("evidence_key") or value.get("evidenceKey") or value.get("evidence_key"),
                "evidence_count": metadata.get("evidenceCount") or metadata.get("evidence_count") or value.get("evidenceCount") or value.get("evidence_count"),
                "confidence": metadata.get("confidence", value.get("confidence")),
                "visibility": metadata.get("visibility", value.get("visibility")),
                "freshness_state": metadata.get("freshnessState") or metadata.get("freshness_state") or value.get("freshnessState") or value.get("freshness_state"),
                "proof_state": value.get("proofState") or value.get("proof_state") or "typed_association",
                "claim_allowed": bool(value.get("claimAllowed", value.get("claim_allowed", False))),
                "claim_gate_reason": value.get("claimGateReason") or value.get("claim_gate_reason") or "association_requires_evidence_review",
            }
        )
    gate_partial_endpoint_bounded_graph_associations(out, objects or [])
    gate_hidden_multi_evidence_bounded_graph_associations(out)
    gate_conflicting_bounded_graph_associations(out)
    return out


def gate_hidden_multi_evidence_bounded_graph_associations(rows: list[dict[str, Any]]) -> None:
    for row in rows:
        evidence_count = parse_int_or_none(row.get("evidence_count"))
        if evidence_count is None or evidence_count <= 1:
            continue
        row["claim_allowed"] = False
        row["claim_gate_reason"] = "relationship_multi_evidence_requires_review"
        row["proof_state"] = "candidate" if str(row.get("evidence_key") or "").strip() else "evidence_needed"


def gate_partial_endpoint_bounded_graph_associations(rows: list[dict[str, Any]], objects: list[dict[str, Any]]) -> None:
    objects_by_ref = bounded_graph_objects_by_ref(objects)
    for row in rows:
        for kind_key in (
            (str(row.get("from_kind") or "").strip(), str(row.get("from_key") or "").strip()),
            (str(row.get("to_kind") or "").strip(), str(row.get("to_key") or "").strip()),
        ):
            endpoint = objects_by_ref.get(kind_key)
            if endpoint is None:
                row["claim_allowed"] = False
                row["claim_gate_reason"] = "relationship_endpoint_missing_requires_hydration"
                row["proof_state"] = "candidate" if str(row.get("evidence_key") or "").strip() else "evidence_needed"
                break
            if str(endpoint.get("freshness_state") or "").strip() == "partial":
                row["claim_allowed"] = False
                row["claim_gate_reason"] = "relationship_endpoint_partial_requires_hydration"
                row["proof_state"] = "candidate" if str(row.get("evidence_key") or "").strip() else "evidence_needed"
                break


def bounded_graph_objects_by_ref(objects: list[dict[str, Any]]) -> dict[tuple[str, str], dict[str, Any]]:
    out: dict[tuple[str, str], dict[str, Any]] = {}
    for row in objects:
        kind = str(row.get("object_type") or "").strip()
        key = str(row.get("key") or "").strip()
        if kind and key:
            out[(kind, key)] = row
    return out


def gate_conflicting_bounded_graph_associations(rows: list[dict[str, Any]]) -> None:
    grouped: dict[tuple[str, str, str, str, str], list[dict[str, Any]]] = {}
    for row in rows:
        grouped.setdefault(logical_bounded_graph_association_key(row), []).append(row)
    for group in grouped.values():
        if len(group) <= 1:
            continue
        if all(bounded_graph_association_row_is_current_claim(row) for row in group):
            continue
        for row in group:
            row["claim_allowed"] = False
            row["claim_gate_reason"] = "relationship_multi_evidence_requires_review"
            row["proof_state"] = "candidate" if str(row.get("evidence_key") or "").strip() else "evidence_needed"


def logical_bounded_graph_association_key(row: dict[str, Any]) -> tuple[str, str, str, str, str]:
    return (
        str(row.get("from_kind") or "").strip(),
        str(row.get("from_key") or "").strip(),
        str(row.get("association_type") or "").strip(),
        str(row.get("to_kind") or "").strip(),
        str(row.get("to_key") or "").strip(),
    )


def bounded_graph_association_row_is_current_claim(row: dict[str, Any]) -> bool:
    if not bool(row.get("claim_allowed")):
        return False
    if not str(row.get("evidence_key") or "").strip():
        return False
    if str(row.get("visibility") or "").strip() != "public":
        return False
    freshness_state = str(row.get("freshness_state") or "").strip()
    if freshness_state not in {"fresh", "current"}:
        return False
    proof_state = str(row.get("proof_state") or "").strip()
    if proof_state not in {"source_observed", "current"}:
        return False
    confidence = parse_float_or_none(row.get("confidence"))
    if confidence is not None and confidence < 1:
        return False
    return True


def parse_float_or_none(value: Any) -> float | None:
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def parse_int_or_none(value: Any) -> int | None:
    try:
        return int(value)
    except (TypeError, ValueError):
        return None


def bounded_graph_evidence_rows(values: Any) -> list[dict[str, Any]]:
    if not isinstance(values, list):
        return []
    out: list[dict[str, Any]] = []
    for value in values:
        if not isinstance(value, dict):
            continue
        key = str(value.get("key") or "").strip()
        if not key:
            continue
        row = dict(value)
        row["_table"] = "evidence"
        row["key"] = key
        out.append(row)
    return out


def bounded_graph_coverage(context: dict[str, Any]) -> dict[str, Any]:
    coverage = context.get("coverage") if isinstance(context.get("coverage"), dict) else {}
    coverage_state = str(coverage.get("coverageState") or coverage.get("coverage_state") or "unknown")
    absence_allowed = bool(coverage.get("absenceClaimsAllowed", coverage.get("absence_claims_allowed", False)))
    gate_reason = str(coverage.get("absenceClaimGateReason") or coverage.get("absence_claim_gate_reason") or "source_coverage_gate")
    summary = str(coverage.get("summary") or coverage.get("automationSummary") or coverage.get("automation_summary") or "")
    covered_association_types = normalize_string_list(
        coverage.get("absenceClaimAssociationTypes") or coverage.get("absence_claim_association_types")
    )
    source_system = str(coverage.get("sourceSystem") or coverage.get("source_system") or "").strip()
    source_instance = str(coverage.get("sourceInstance") or coverage.get("source_instance") or "").strip()
    coverage_window_start = str(coverage.get("coverageWindowStart") or coverage.get("coverage_window_start") or "").strip()
    coverage_window_end = str(coverage.get("coverageWindowEnd") or coverage.get("coverage_window_end") or "").strip()
    requested_association_types = normalize_string_list(
        context.get("associationTypes")
        or context.get("association_types")
        or context.get("requestedAssociationTypes")
        or context.get("requested_association_types")
    )
    if absence_allowed and coverage_state != "complete":
        absence_allowed = False
        gate_reason = "source_coverage_not_complete"
        summary = append_summary_clause(summary, "Absence claims remain disabled because source coverage is not complete.")
    if absence_allowed and not coverage_covers_requested_associations(covered_association_types, requested_association_types):
        absence_allowed = False
        gate_reason = "relation_path_coverage_required"
        summary = append_summary_clause(summary, "Absence claims remain disabled because source coverage is not scoped to the requested relationship path.")
    if absence_allowed and (not source_system or not source_instance):
        absence_allowed = False
        gate_reason = "source_scope_coverage_required"
        summary = append_summary_clause(summary, "Absence claims remain disabled because source coverage is not scoped to a source system and instance.")
    if absence_allowed and (not coverage_window_start or not coverage_window_end):
        absence_allowed = False
        gate_reason = "source_time_window_required"
        summary = append_summary_clause(summary, "Absence claims remain disabled because source coverage is not scoped to a freshness window.")
    return {
        "coverage_state": coverage_state,
        "absence_claims_allowed": absence_allowed,
        "absence_claim_gate_reason": gate_reason,
        "summary": summary,
        "absence_claim_association_types": covered_association_types,
        "source_system": source_system,
        "source_instance": source_instance,
        "coverage_window_start": coverage_window_start,
        "coverage_window_end": coverage_window_end,
    }


def normalize_string_list(values: Any) -> list[str]:
    if values is None:
        return []
    if isinstance(values, str):
        values = [values]
    if not isinstance(values, list):
        return []
    out: list[str] = []
    seen: set[str] = set()
    for value in values:
        text = str(value).strip()
        if not text or text in seen:
            continue
        seen.add(text)
        out.append(text)
    return sorted(out)


def coverage_covers_requested_associations(covered: list[str], requested: list[str]) -> bool:
    covered_set = set(covered)
    if "*" in covered_set or "all" in covered_set:
        return True
    if not requested:
        return False
    return all(value in covered_set for value in requested)


def append_summary_clause(summary: str, clause: str) -> str:
    summary = summary.strip()
    clause = clause.strip()
    if not clause or clause in summary:
        return summary
    if not summary:
        return clause
    return summary + " " + clause


def annotate_bounded_graph_seed_relevance(seed_key: str, objects: list[dict[str, Any]], associations: list[dict[str, Any]]) -> None:
    distances = bounded_graph_seed_distances(seed_key, associations)
    if seed_key and seed_key not in distances:
        distances[seed_key] = 0
    for row in objects:
        key = str(row.get("key") or "")
        distance = distances.get(key)
        row["seed_reachable"] = distance is not None
        row["seed_distance"] = distance if distance is not None else None
        if distance is None:
            row["claim_allowed"] = False
            row["claim_gate_reason"] = "disconnected_from_seed_component"
    for row in associations:
        from_distance = distances.get(str(row.get("from_key") or ""))
        to_distance = distances.get(str(row.get("to_key") or ""))
        reachable = from_distance is not None and to_distance is not None
        row["seed_reachable"] = reachable
        row["seed_distance"] = min(from_distance, to_distance) if reachable else None
        if not reachable:
            row["claim_allowed"] = False
            row["claim_gate_reason"] = "disconnected_from_seed_component"


def bounded_graph_seed_distances(seed_key: str, associations: list[dict[str, Any]]) -> dict[str, int]:
    if not seed_key:
        return {}
    adjacency: dict[str, set[str]] = {}
    for row in associations:
        from_key = str(row.get("from_key") or "")
        to_key = str(row.get("to_key") or "")
        if not from_key or not to_key:
            continue
        adjacency.setdefault(from_key, set()).add(to_key)
        adjacency.setdefault(to_key, set()).add(from_key)
    distances = {seed_key: 0}
    queue = [seed_key]
    for key in queue:
        for neighbor in sorted(adjacency.get(key, set())):
            if neighbor in distances:
                continue
            distances[neighbor] = distances[key] + 1
            queue.append(neighbor)
    return distances


def bounded_graph_citations(context_hash: str, seed: dict[str, Any], objects: list[dict[str, Any]], associations: list[dict[str, Any]], coverage: dict[str, Any], context: dict[str, Any]) -> list[dict[str, Any]]:
    seed_key = str(seed.get("key") or "object:unknown")
    rows: list[dict[str, Any]] = [
        {
            "ref": citation("context", context_hash),
            "citationKind": "graph_context",
            "nodeKind": "bounded_graph_context",
            "nodeKey": context_hash,
            "claimAllowed": True,
            "claimUse": "context_boundary",
            "claimGateReason": "bounded_graph_context",
        },
        {
            "ref": citation("guardrail", context_hash),
            "citationKind": "guardrail",
            "nodeKind": "bounded_graph_guardrail",
            "nodeKey": context_hash,
            "claimAllowed": False,
            "claimUse": "guardrail",
            "claimGateReason": "guardrail_only",
        },
        {
            "ref": citation("source_coverage", seed_key),
            "citationKind": "source_coverage",
            "nodeKind": "bounded_graph_source_coverage",
            "nodeKey": seed_key,
            "claimAllowed": True,
            "claimUse": "source_coverage_gate",
            "claimGateReason": coverage["absence_claim_gate_reason"],
            "proofState": coverage["coverage_state"],
        },
    ]
    for row in objects:
        rows.append(
            {
                "ref": row_citation(row),
                "citationKind": "typed_graph_object",
                "nodeKind": "graph_object",
                "nodeKey": row["key"],
                "claimAllowed": bool(row.get("claim_allowed")),
                "claimUse": "typed_object",
                "claimGateReason": row.get("claim_gate_reason"),
                "proofState": row.get("proof_state"),
            }
        )
    for row in associations:
        rows.append(
            {
                "ref": row_citation(row),
                "citationKind": "typed_graph_association",
                "nodeKind": "graph_association",
                "nodeKey": row["key"],
                "associationType": row.get("association_type"),
                "claimAllowed": bool(row.get("claim_allowed")),
                "claimUse": "typed_association" if row.get("claim_allowed") else "validation_lead",
                "claimGateReason": row.get("claim_gate_reason"),
                "proofState": row.get("proof_state"),
            }
        )
    supplied = context.get("citations", [])
    if isinstance(supplied, list):
        rows.extend(row for row in supplied if isinstance(row, dict) and str(row.get("ref") or "").strip())
    return unique_citations_by_ref(rows)


def unique_citations_by_ref(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    seen: set[str] = set()
    for row in rows:
        ref = str(row.get("ref") or "").strip()
        if not ref or ref in seen:
            continue
        seen.add(ref)
        out.append(row)
    return out


def analytics_from_graph_context(graph_context: dict[str, Any]) -> dict[str, Any]:
    forecast_packet = graph_context.get("forecastPacket") or {}
    guardrail_packet = graph_context.get("guardrailPacket") or {}
    source_packet = graph_context.get("sourceCoveragePacket") or {}
    eta_ready = bool(forecast_packet.get("etaForecastReady"))
    return {
        "forecast_summary": {
            "eta_forecast_ready": {"value": str(eta_ready).lower(), "note": str(forecast_packet.get("automationSummary") or "")},
            "eta_readiness_state": {"value": str(forecast_packet.get("readinessState") or "unknown"), "note": ""},
            "risk_triage_safe_use": {"value": "attention_ordering", "note": "Graph context preserves forecast output as risk triage unless ETA is ready."},
        },
        "measurement_readiness": {
            "ready_to_measure_precision": {"value": str(not bool(guardrail_packet.get("humanReviewRequired"))).lower(), "note": str(guardrail_packet.get("automationSummary") or "")},
            "ready_to_measure_actionability": {"value": str(not bool(guardrail_packet.get("humanReviewRequired"))).lower(), "note": str(guardrail_packet.get("readinessState") or "")},
        },
        "source_coverage": {
            "coverage_state": {"value": str(source_packet.get("coverageState") or "unknown"), "note": str(source_packet.get("automationSummary") or "")},
            "absence_claims_allowed": {"value": str(bool(source_packet.get("absenceClaimsAllowed"))).lower(), "note": str(source_packet.get("absenceClaimGateReason") or "")},
        },
        "blocker_candidate_count": 0,
    }


def graph_context_guardrails(graph_context: dict[str, Any], analytics: dict[str, Any]) -> list[str]:
    out = guardrails([], analytics)
    forecast_packet = graph_context.get("forecastPacket") or {}
    source_packet = graph_context.get("sourceCoveragePacket") or {}
    guardrail_packet = graph_context.get("guardrailPacket") or {}
    if forecast_packet and not forecast_packet.get("etaForecastReady"):
        summary = str(forecast_packet.get("automationSummary") or "Forecast packet gates ETA claims; use risk triage only.")
        out.append(summary)
    if source_packet and not source_packet.get("absenceClaimsAllowed"):
        out.append(str(source_packet.get("automationSummary") or "Source coverage gates absence claims."))
    if guardrail_packet and guardrail_packet.get("humanReviewRequired"):
        out.append(str(guardrail_packet.get("automationSummary") or "Human review is required before autonomous action."))
    return sorted({text for text in out if str(text).strip()})


def graph_context_rows(rows: Any, table: str) -> list[dict[str, Any]]:
    if not isinstance(rows, list):
        return []
    out: list[dict[str, Any]] = []
    for row in rows:
        if not isinstance(row, dict):
            continue
        converted = camel_to_snake_dict(row)
        converted["_table"] = table
        if table == "work_program_quality_gates" and converted.get("key") and not converted.get("gate_key"):
            converted["gate_key"] = converted["key"]
        if table == "work_program_evidence_needs" and converted.get("key") and not converted.get("target_key"):
            converted["target_key"] = converted["key"]
        out.append(converted)
    return out


def graph_context_evidence_rows(graph_context: dict[str, Any]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    for table_key in ["items", "actions", "dependencyEdges", "insights", "forecasts"]:
        values = graph_context.get(table_key)
        if not isinstance(values, list):
            continue
        for row in values:
            if not isinstance(row, dict) or not isinstance(row.get("evidence"), dict):
                continue
            evidence = camel_to_snake_dict(row["evidence"])
            evidence["_table"] = "evidences"
            rows.append(evidence)
    seen: set[str] = set()
    out: list[dict[str, Any]] = []
    for row in rows:
        key = str(row.get("key") or row.get("ref") or len(out))
        if key in seen:
            continue
        seen.add(key)
        out.append(row)
    return out


def camel_to_snake_dict(value: dict[str, Any]) -> dict[str, Any]:
    return {camel_to_snake(str(key)): camel_to_snake_value(row_value) for key, row_value in value.items()}


def camel_to_snake_value(value: Any) -> Any:
    if isinstance(value, dict):
        return camel_to_snake_dict(value)
    if isinstance(value, list):
        return [camel_to_snake_value(item) for item in value]
    return value


def camel_to_snake(value: str) -> str:
    return re.sub(r"(?<!^)(?=[A-Z])", "_", value).lower()


def work_program_items(conn: sqlite3.Connection, workstream_keys: list[str], source_instance: str | None, limit: int) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_program_items"):
        return []
    source_clause, params = source_predicate("wpi", source_instance)
    placeholders = ",".join("?" for _ in workstream_keys)
    rows = conn.execute(
        f"""
        select
          wpi.id,
          wpi.key,
          wpi.workstream_key,
          wpi.subject_kind,
          wpi.subject_key,
          wpi.title,
          wpi.program_status,
          wpi.tpm_bucket,
          wpi.decision_state,
          wpi.due_bucket,
          wpi.owner_key,
          wpi.next_action,
          wpi.risk_score,
          wpi.source_coverage_state,
          wpi.latest_evidence_id,
          wpi.rank_score,
          wa.key as work_action_key
        from work_program_items wpi
        left join work_actions wa on wa.id = wpi.work_action_id
        where wpi.workstream_key in ({placeholders})
          {source_clause}
        order by wpi.rank_score desc, wpi.risk_score desc, wpi.updated_at desc
        limit ?
        """,
        [*workstream_keys, *params, limit],
    ).fetchall()
    return [row_dict(row, "work_program_items") for row in rows]


def work_actions(
    conn: sqlite3.Connection,
    subject_keys: list[str],
    action_keys: list[str],
    source_instance: str | None,
    limit: int,
) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_actions"):
        return []
    predicates: list[str] = []
    params: list[Any] = []
    if subject_keys:
        predicates.append(f"subject_key in ({placeholders(subject_keys)})")
        params.extend(subject_keys)
    if action_keys:
        predicates.append(f"key in ({placeholders(action_keys)})")
        params.extend(action_keys)
    if not predicates:
        return []
    source_clause, source_params = source_predicate(None, source_instance)
    rows = conn.execute(
        f"""
        select
          id, key, action_type, action_state, decision_state, decision,
          decision_reason, subject_kind, subject_key, owner_key, due_bucket,
          latest_evidence_id, rank_score, source_url
        from work_actions
        where ({' or '.join(predicates)})
          and action_state = 'open'
          {source_clause}
        order by rank_score desc, updated_at desc
        limit ?
        """,
        [*params, *source_params, limit],
    ).fetchall()
    return [row_dict(row, "work_actions") for row in rows]


def work_insights(conn: sqlite3.Connection, subject_keys: list[str], source_instance: str | None, limit: int) -> list[dict[str, Any]]:
    if not subject_keys or not table_exists(conn, "work_insights"):
        return []
    source_clause, source_params = source_predicate(None, source_instance)
    quarantine_clause = work_insight_generated_summary_quarantine_clause(conn)
    rows = conn.execute(
        f"""
        select
          id, key, insight_kind, severity, producer_state, subject_kind,
          subject_key, title, details, recommended_action, score,
          score_explanation, latest_evidence_id, rank_score, source_url
        from work_insights
        where subject_key in ({placeholders(subject_keys)})
          and producer_state = 'current'
          {source_clause}
          {quarantine_clause}
        order by rank_score desc, score desc, updated_at desc
        limit ?
        """,
        [*subject_keys, *source_params, limit],
    ).fetchall()
    return [row_dict(row, "work_insights") for row in rows]


def work_insight_generated_summary_quarantine_clause(conn: sqlite3.Connection) -> str:
    columns = set(table_columns(conn, "work_insights"))
    clauses: list[str] = []
    if "insight_kind" in columns:
        clauses.append("coalesce(insight_kind, '') != 'ai_graph_brief'")
    if "source_system" in columns:
        clauses.append("coalesce(source_system, '') != 'cubicle_ai'")
    if "external_kind" in columns:
        clauses.append("coalesce(external_kind, '') not like 'ai_graph_brief%'")
    if "model_method" in columns:
        clauses.append("coalesce(model_method, '') not like 'bounded_graph_context_to_cited_brief%'")
    if "source_url" in columns:
        clauses.append("coalesce(source_url, '') not like 'cubicle://graph-brief/%'")
    if not clauses:
        return ""
    return "and " + " and ".join(f"({clause})" for clause in clauses)


def work_item_forecasts(conn: sqlite3.Connection, subject_keys: list[str], source_instance: str | None, limit: int) -> list[dict[str, Any]]:
    if not subject_keys or not table_exists(conn, "work_item_forecasts"):
        return []
    source_clause, source_params = source_predicate(None, source_instance)
    rows = conn.execute(
        f"""
        select
          id, key, forecast_kind, subject_kind, subject_key, subject_state,
          forecast_method, model_name, age_days, predicted_total_cycle_days,
          predicted_remaining_days, overdue_days, risk_score, risk_band,
          readiness_state, ready_for_eta, readiness_reason, latest_evidence_id,
          rank_score, source_url
        from work_item_forecasts
        where subject_key in ({placeholders(subject_keys)})
          {source_clause}
        order by risk_score desc, rank_score desc, updated_at desc
        limit ?
        """,
        [*subject_keys, *source_params, limit],
    ).fetchall()
    return [row_dict(row, "work_item_forecasts") for row in rows]


def traverse_dependency_neighborhood(
    conn: sqlite3.Connection,
    seed_subject_keys: list[str],
    source_instance: str | None,
    *,
    depth: int,
    edge_limit: int,
) -> dict[str, Any]:
    if not seed_subject_keys:
        return {"subject_keys": [], "edges": []}
    reached = set(seed_subject_keys)
    frontier = set(seed_subject_keys)
    edges_by_key: dict[str, dict[str, Any]] = {}
    remaining = edge_limit
    for _ in range(depth):
        if not frontier or remaining <= 0:
            break
        rows = work_dependency_edges(conn, sorted(frontier), source_instance, remaining)
        next_frontier: set[str] = set()
        for edge in rows:
            edge_key = str(edge.get("key") or edge.get("id"))
            edges_by_key[edge_key] = edge
            for endpoint in [edge.get("from_key"), edge.get("to_key")]:
                if endpoint is None or str(endpoint).strip() == "":
                    continue
                endpoint_key = str(endpoint)
                if endpoint_key not in reached:
                    reached.add(endpoint_key)
                    next_frontier.add(endpoint_key)
        remaining = max(0, edge_limit - len(edges_by_key))
        frontier = next_frontier
    return {
        "subject_keys": sorted(reached),
        "edges": sorted(edges_by_key.values(), key=lambda row: (-float(row.get("rank_score") or 0), str(row.get("key") or row.get("id")))),
    }


def work_dependency_edges(conn: sqlite3.Connection, subject_keys: list[str], source_instance: str | None, limit: int) -> list[dict[str, Any]]:
    if not subject_keys or not table_exists(conn, "work_dependency_edges"):
        return []
    source_clause, source_params = source_predicate(None, source_instance)
    rows = conn.execute(
        f"""
        select
          id, key, edge_kind, relationship_authority, canonical_relationship_kind,
          from_kind, from_key, to_kind, to_key, risk_signal,
          source_coverage_state, latest_evidence_id, rank_score, source_url
        from work_dependency_edges
        where (from_key in ({placeholders(subject_keys)}) or to_key in ({placeholders(subject_keys)}))
          {source_clause}
        order by rank_score desc, last_activity_at desc, updated_at desc
        limit ?
        """,
        [*subject_keys, *subject_keys, *source_params, limit],
    ).fetchall()
    return [row_dict(row, "work_dependency_edges") for row in rows]


def evidence_rows(conn: sqlite3.Connection, evidence_ids: list[int], source_instance: str | None, limit: int) -> list[dict[str, Any]]:
    if not evidence_ids or not table_exists(conn, "evidences"):
        return []
    source_clause, source_params = source_predicate(None, source_instance)
    rows = conn.execute(
        f"""
        select
          id, key, claim_kind, claim_target_kind, claim_field,
          relationship_kind, locator_kind, locator, excerpt,
          excerpt_truncated, proof_state, source_system, source_instance,
          external_kind, external_id, source_url, observed_at
        from evidences
        where id in ({placeholders(evidence_ids)})
          {source_clause}
        order by id desc
        limit ?
        """,
        [*evidence_ids, *source_params, limit],
    ).fetchall()
    return [row_dict(row, "evidences") for row in rows]


def latest_quality_gates(conn: sqlite3.Connection, workstream_keys: list[str], source_instance: str | None) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_program_quality_gates"):
        return []
    generated_at = latest_generated_at(conn, "work_program_quality_gates", workstream_keys, source_instance)
    if not generated_at:
        return []
    source_clause, source_params = source_predicate(None, source_instance)
    rows = conn.execute(
        f"""
        select gate_key, gate_state, blocking, detail, recommended_action, generated_at
          from work_program_quality_gates
         where workstream_key in ({placeholders(workstream_keys)})
           and generated_at = ?
           {source_clause}
         order by blocking desc, rank_score desc, gate_key
        """,
        [*workstream_keys, generated_at, *source_params],
    ).fetchall()
    return [row_dict(row, "work_program_quality_gates") for row in rows]


def latest_evidence_needs(conn: sqlite3.Connection, workstream_keys: list[str], source_instance: str | None, limit: int) -> list[dict[str, Any]]:
    if not table_exists(conn, "work_program_evidence_needs"):
        return []
    generated_at = latest_generated_at(conn, "work_program_evidence_needs", workstream_keys, source_instance)
    if not generated_at:
        return []
    source_clause, source_params = source_predicate(None, source_instance)
    rows = conn.execute(
        f"""
        select gate_key, evidence_kind, priority, execution_state,
               target_kind, target_key, action_key, recommended_action,
               missing_count, generated_at
          from work_program_evidence_needs
         where workstream_key in ({placeholders(workstream_keys)})
           and generated_at = ?
           {source_clause}
         order by rank_score desc, missing_count desc
         limit ?
        """,
        [*workstream_keys, generated_at, *source_params, limit],
    ).fetchall()
    return [row_dict(row, "work_program_evidence_needs") for row in rows]


def analytics_context(analytics_db: Path | None) -> dict[str, Any]:
    if analytics_db is None or not analytics_db.exists():
        return {}
    with sqlite3.connect(analytics_db) as conn:
        conn.row_factory = sqlite3.Row
        return {
            "forecast_summary": metric_table(conn, "tpm_forecast_summary"),
            "forecast_reliability": table_rows(conn, "tpm_forecast_reliability", 10),
            "measurement_readiness": metric_table(conn, "tpm_evaluation_readiness"),
            "measurement_queue_summary": metric_table(conn, "tpm_measurement_label_summary"),
            "blocker_candidate_count": table_count(conn, "tpm_blocker_candidates"),
        }


def metric_table(conn: sqlite3.Connection, table: str) -> dict[str, dict[str, str]]:
    if not table_exists(conn, table):
        return {}
    columns = set(table_columns(conn, table))
    if not {"metric", "value"}.issubset(columns):
        return {}
    note_expr = "note" if "note" in columns else "'' as note"
    rows = conn.execute(f"select metric, value, {note_expr} from {table}").fetchall()
    return {str(row["metric"]): {"value": str(row["value"]), "note": str(row["note"] or "")} for row in rows}


def table_rows(conn: sqlite3.Connection, table: str, limit: int) -> list[dict[str, Any]]:
    if not table_exists(conn, table):
        return []
    return [dict(row) for row in conn.execute(f"select * from {table} limit ?", [limit]).fetchall()]


def table_count(conn: sqlite3.Connection, table: str) -> int:
    if not table_exists(conn, table):
        return 0
    return int(conn.execute(f"select count(*) from {table}").fetchone()[0] or 0)


def guardrails(quality_gates: list[dict[str, Any]], analytics: dict[str, Any]) -> list[str]:
    out: list[str] = []
    gate_by_key = {str(row.get("gate_key")): row for row in quality_gates}
    forecast_summary = analytics.get("forecast_summary", {})
    eta_ready = forecast_summary.get("eta_forecast_ready", {}).get("value", "").lower()
    if eta_ready != "true":
        out.append("Do not make ETA commitments; forecasts are risk-triage signals only.")
    measurement = analytics.get("measurement_readiness", {})
    if measurement.get("ready_to_measure_precision", {}).get("value", "").lower() != "true":
        out.append("Do not present generated insights as measured-precision product claims.")
    if gate_by_key.get("source_authentication", {}).get("gate_state") in {"watch", "gated"}:
        out.append("Call out anonymous/public-source observations separately from authenticated evidence.")
    if gate_by_key.get("claim_provenance", {}).get("gate_state") in {"watch", "gated"}:
        out.append("Mark generated or derived claims as validation leads unless directly evidenced.")
    blocker_candidates = analytics.get("blocker_candidate_count", 0)
    if blocker_candidates:
        out.append(f"Distinguish confirmed blockers from {blocker_candidates} blocker candidates needing validation.")
    return out


def render_brief(context: dict[str, Any]) -> str:
    rows = context["rows"]
    analytics = context.get("analytics", {})
    forecast_summary = analytics.get("forecast_summary", {})
    measurement = analytics.get("measurement_readiness", {})
    blocker_candidate_count = analytics.get("blocker_candidate_count", 0)
    product_claims_gated = product_claims_are_gated(context)
    items = rows.get("work_program_items", [])
    actions = rows.get("work_actions", [])
    insights = rows.get("work_insights", [])
    forecasts = rows.get("work_item_forecasts", [])
    evidence = rows.get("evidence", [])
    traversal = context.get("traversal", {})
    lines = [
        "# Cubicle Graph Brief PoC",
        "",
        f"- Seed: `{context['seed']['key']}`",
        f"- Source instance: `{context['seed'].get('source_instance') or 'all'}`",
        f"- Context hash: `{context['context_hash']}`",
        "",
        "## Situation",
        "",
        f"- Retrieved {len(items)} work item(s), {len(actions)} action(s), {len(insights)} current insight(s), {len(forecasts)} forecast row(s), {traversal.get('edge_count', 0)} dependency edge(s), and {len(evidence)} evidence row(s) across {len(traversal.get('reached_subject_keys', []))} traversed node(s). {citation('context', context['context_hash'])}",
        f"- ETA readiness is `{forecast_summary.get('eta_forecast_ready', {}).get('value', 'unknown')}`; use forecast rows for risk ordering, not date commitments. {citation('analytics', 'tpm_forecast_summary')}",
        f"- Measurement precision readiness is `{measurement.get('ready_to_measure_precision', {}).get('value', 'unknown')}`; open review queues should be treated as validation work. {citation('analytics', 'tpm_evaluation_readiness')}",
        f"- Confirmed blocker rows are not inferred here; analytics reports {blocker_candidate_count} blocker candidate(s) needing validation. {citation('analytics', 'tpm_blocker_candidates')}",
        "",
        "## Highest-Signal Work",
        "",
    ]
    for item in items[:8]:
        lines.append(
            f"- `{item['subject_key']}`: {item['title']} "
            f"[status={item.get('program_status')}, decision={item_decision_use(item, product_claims_gated)}, risk={format_number(item.get('risk_score'))}]. "
            f"Next: {item.get('next_action') or 'review source context'}. {row_citation(item)}"
        )
    lines.extend(["", "## Current Actions", ""])
    for action in actions[:8]:
        lines.append(
            f"- `{action['subject_key']}`: {action_decision_use(action, product_claims_gated)} / {action.get('action_state')} "
            f"owned by `{action.get('owner_key') or 'unassigned'}`. "
            f"Reason: {action.get('decision_reason') or action.get('decision') or 'not recorded'}. {row_citation(action)}"
        )
    lines.extend(["", "## Guardrails", ""])
    for guardrail in context.get("guardrails", []):
        lines.append(f"- {guardrail} {citation('guardrail', context['context_hash'])}")
    lines.extend(["", "## LLM Task", "", context["llm_task"], ""])
    return "\n".join(lines)


def render_generic_graph_baseline(context: dict[str, Any]) -> str:
    limit = max_brief_bullets_per_section(context)
    confirmed = generic_baseline_confirmed_bullets(context)[:limit]
    validation = generic_baseline_validation_bullets(context)[:limit]
    what_not = generic_baseline_what_not_to_claim_bullets(context)[:limit]
    return "\n".join(
        [
            "# Operating Brief",
            "",
            "## Confirmed Facts",
            "",
            *confirmed,
            "",
            "## Validation Leads",
            "",
            *validation,
            "",
            "## What Not To Claim",
            "",
            *what_not,
            "",
        ]
    )


def render_typed_row_baseline(context: dict[str, Any]) -> str:
    confirmed = typed_row_baseline_confirmed_bullets(context)[:MAX_BRIEF_BULLETS_PER_SECTION]
    validation = typed_row_baseline_validation_bullets(context)[:MAX_BRIEF_BULLETS_PER_SECTION]
    what_not = typed_row_baseline_what_not_to_claim_bullets(context)[:MAX_BRIEF_BULLETS_PER_SECTION]
    return "\n".join(
        [
            "# Operating Brief",
            "",
            "## Confirmed Facts",
            "",
            *confirmed,
            "",
            "## Validation Leads",
            "",
            *validation,
            "",
            "## What Not To Claim",
            "",
            *what_not,
            "",
        ]
    )


def max_brief_bullets_per_section(context: dict[str, Any]) -> int:
    if is_bounded_graph_context(context):
        return BOUNDED_GRAPH_MAX_BRIEF_BULLETS_PER_SECTION
    return MAX_BRIEF_BULLETS_PER_SECTION


def typed_row_baseline_confirmed_bullets(context: dict[str, Any]) -> list[str]:
    rows = context.get("rows", {})
    graph_objects = list_rows(rows, "graph_objects")
    graph_associations = list_rows(rows, "graph_associations")
    if graph_objects or graph_associations:
        return bounded_graph_typed_row_baseline_confirmed_bullets(context, graph_objects, graph_associations)
    items = list_rows(rows, "work_program_items")
    actions = list_rows(rows, "work_actions")
    context_hash = str(context.get("context_hash", ""))
    bullets = [
        f"- The typed-row baseline contains {len(items)} work item(s) and {len(actions)} action row(s). {citation('context', context_hash)}"
    ]
    for row in [*items, *actions]:
        token = row_citation(row)
        if not citation_can_support_confirmed_fact(context, token):
            continue
        subject = row.get("subject_key") or row.get("key") or "unknown"
        title = row.get("title") or row.get("decision_reason") or row.get("action_type") or "typed row"
        bullets.append(f"- Typed row `{subject}` is present: {title}. {token}")
        if len(bullets) >= MAX_BRIEF_BULLETS_PER_SECTION:
            break
    if len(bullets) == 1:
        bullets.append(f"- No claimable typed work item or action row was selected by the typed-row baseline. {citation('context', context_hash)}")
    return bullets


def bounded_graph_typed_row_baseline_confirmed_bullets(context: dict[str, Any], objects: list[dict[str, Any]], associations: list[dict[str, Any]]) -> list[str]:
    context_hash = str(context.get("context_hash", ""))
    bullets = [
        f"- The typed-row baseline contains {len(objects)} typed graph object row(s) and excludes {len(associations)} relationship association row(s). {citation('context', context_hash)}"
    ]
    coverage_bullet = generic_bounded_graph_coverage_bullet(context)
    if coverage_bullet:
        bullets.append(coverage_bullet)
    for row in [row for row in objects if row.get("seed_reachable") is not False]:
        token = row_citation(row)
        if not citation_can_support_confirmed_fact(context, token):
            continue
        bullets.append(f"- Typed object row `{row.get('key')}` is present: {row.get('title') or row.get('object_type')}. {token}")
        if len(bullets) >= MAX_BRIEF_BULLETS_PER_SECTION:
            break
    if len(bullets) == 1:
        bullets.append(f"- No claimable typed graph object row was selected by the typed-row baseline. {citation('context', context_hash)}")
    return bullets


def typed_row_baseline_validation_bullets(context: dict[str, Any]) -> list[str]:
    rows = context.get("rows", {})
    if list_rows(rows, "graph_objects") or list_rows(rows, "graph_associations"):
        context_hash = str(context.get("context_hash", ""))
        return [
            f"- Relationship association rows are intentionally excluded from this typed-row baseline; use bounded graph traversal before making relationship claims. {citation('guardrail', context_hash)}"
        ]
    bullets: list[str] = []
    allowed = allowed_citations(context)
    for row in [*list_rows(rows, "work_program_items"), *list_rows(rows, "work_actions")]:
        token = row_citation(row)
        if token not in allowed:
            continue
        subject = row.get("subject_key") or row.get("key") or "unknown"
        title = row.get("next_action") or row.get("decision_reason") or row.get("title") or "review typed row"
        bullets.append(f"- Review typed row `{subject}` before product action: {title}. {token}")
        if len(bullets) >= MAX_BRIEF_BULLETS_PER_SECTION:
            break
    if bullets:
        return bullets
    context_hash = str(context.get("context_hash", ""))
    return [f"- Add typed work item or action rows before making product claims. {citation('guardrail', context_hash)}"]


def typed_row_baseline_what_not_to_claim_bullets(context: dict[str, Any]) -> list[str]:
    context_hash = str(context.get("context_hash", ""))
    guardrails = [str(value).strip() for value in context.get("guardrails", []) if str(value).strip()]
    bullets = [f"- {guardrail} {citation('guardrail', context_hash)}" for guardrail in guardrails[:MAX_BRIEF_BULLETS_PER_SECTION]]
    if bullets:
        return bullets
    return [f"- Do not infer product facts from dependency topology, generated summaries, or missing source rows. {citation('guardrail', context_hash)}"]


def generic_baseline_confirmed_bullets(context: dict[str, Any]) -> list[str]:
    rows = context.get("rows", {})
    graph_objects = list_rows(rows, "graph_objects")
    graph_associations = list_rows(rows, "graph_associations")
    if graph_objects or graph_associations:
        return generic_bounded_graph_confirmed_bullets(context, graph_objects, graph_associations)
    items = list_rows(rows, "work_program_items")
    actions = list_rows(rows, "work_actions")
    insights = list_rows(rows, "work_insights")
    forecasts = list_rows(rows, "work_item_forecasts")
    edges = list_rows(rows, "work_dependency_edges")
    context_hash = str(context.get("context_hash", ""))
    bullets = [
        f"- The bounded graph context contains {len(items)} work item(s), {len(actions)} action(s), {len(insights)} insight(s), {len(forecasts)} forecast row(s), and {len(edges)} dependency edge(s). {citation('context', context_hash)}"
    ]
    for row in [*items, *actions]:
        token = row_citation(row)
        if citation_can_support_confirmed_fact(context, token):
            subject = row.get("subject_key") or row.get("key") or "unknown"
            title = row.get("title") or row.get("decision_reason") or row.get("action_type") or "claimable graph row"
            bullets.append(f"- Claimable graph row `{subject}` is present: {title}. {token}")
            break
    if len(bullets) == 1:
        bullets.append(f"- No additional claimable row citation was selected by the generic baseline. {citation('context', context_hash)}")
    return bullets


def generic_bounded_graph_confirmed_bullets(context: dict[str, Any], objects: list[dict[str, Any]], associations: list[dict[str, Any]]) -> list[str]:
    context_hash = str(context.get("context_hash", ""))
    limit = max_brief_bullets_per_section(context)
    bullets = [
        f"- The bounded graph context contains {len(objects)} object(s) and {len(associations)} association(s). {citation('context', context_hash)}"
    ]
    coverage_bullet = generic_bounded_graph_coverage_bullet(context)
    if coverage_bullet:
        bullets.append(coverage_bullet)
    reachable_associations = [row for row in associations if row.get("seed_reachable") is not False]
    reachable_objects = [row for row in objects if row.get("seed_reachable") is not False]
    selected_claim = False
    for row in [*generic_bounded_graph_ordered_associations(context, reachable_associations), *reachable_objects]:
        token = row_citation(row)
        if not citation_can_support_confirmed_fact(context, token):
            continue
        if row.get("_table") == "graph_associations":
            bullets.append(f"- Claimable association `{row.get('from_key')}` -> `{row.get('to_key')}` is present as `{row.get('association_type')}`. {token}")
        else:
            bullets.append(f"- Claimable object `{row.get('key')}` is present: {row.get('title') or row.get('object_type')}. {token}")
        selected_claim = True
        break
    object_summary = generic_bounded_graph_object_summary_bullet(context, reachable_objects)
    if object_summary and len(bullets) < limit:
        bullets.append(object_summary)
        selected_claim = True
    if not selected_claim:
        bullets.append(f"- No additional claimable graph object or association was selected by the generic baseline. {citation('context', context_hash)}")
    return bullets


def generic_bounded_graph_object_summary_bullet(context: dict[str, Any], objects: list[dict[str, Any]]) -> str:
    claimable = []
    for row in objects:
        token = row_citation(row)
        if citation_can_support_confirmed_fact(context, token):
            claimable.append(row)
    if not claimable:
        return ""
    def seed_distance(row: dict[str, Any]) -> int:
        distance = row.get("seed_distance")
        if distance is None:
            return 1_000_000
        return int(distance)

    claimable = sorted(
        claimable,
        key=lambda row: (
            row.get("seed_distance") is None,
            seed_distance(row),
            str(row.get("key") or ""),
        ),
    )
    selected = claimable[:3]
    labels = ", ".join(f"`{row.get('key')}`" for row in selected)
    tokens = " ".join(row_citation(row) for row in selected)
    return f"- Claimable objects include {labels}. {tokens}"


def generic_bounded_graph_coverage_bullet(context: dict[str, Any]) -> str:
    source_coverage = context.get("analytics", {}).get("source_coverage", {})
    coverage_state = str(source_coverage.get("coverage_state", {}).get("value") or "").strip()
    if coverage_state in {"", "unknown", "sparse"}:
        return ""
    absence_allowed = str(source_coverage.get("absence_claims_allowed", {}).get("value") or "false").strip()
    gate_reason = str(source_coverage.get("absence_claims_allowed", {}).get("note") or "source_coverage_gate").strip()
    seed_key = str(context.get("seed", {}).get("key") or "object:unknown")
    if absence_allowed == "true":
        return f"- Source coverage is `{coverage_state}`; absence claims are allowed by `{gate_reason}`. {citation('source_coverage', seed_key)}"
    return f"- Source coverage is `{coverage_state}`; do not infer missing neighbors. Gate reason: `{gate_reason}`. Raw sync issue bodies and source URLs are coverage evidence only, not prompt facts. {citation('source_coverage', seed_key)}"


def generic_baseline_validation_bullets(context: dict[str, Any]) -> list[str]:
    rows = context.get("rows", {})
    bullets: list[str] = []
    limit = max_brief_bullets_per_section(context)
    reachable_associations = [row for row in list_rows(rows, "graph_associations") if row.get("seed_reachable") is not False]
    for row in generic_bounded_graph_ordered_associations(context, reachable_associations):
        token = row_citation(row)
        detail = row.get("claim_gate_reason") or row.get("association_type") or "association requires validation"
        bullets.append(
            f"- Treat association `{row.get('from_key')}` -> `{row.get('to_key')}` as `{row.get('association_type')}` validation context before product action: {detail}. {token}"
        )
        if len(bullets) >= limit:
            return bullets
    for row in [*list_rows(rows, "work_dependency_edges"), *list_rows(rows, "work_insights"), *list_rows(rows, "work_item_forecasts")]:
        token = row_citation(row)
        subject = row.get("from_key") or row.get("subject_key") or row.get("key") or "unknown"
        detail = row.get("risk_signal") or row.get("title") or row.get("readiness_reason") or row.get("edge_kind") or "derived graph context"
        bullets.append(f"- Treat `{subject}` as validation context before product action: {detail}. {token}")
        if len(bullets) >= limit:
            break
    if bullets:
        return bullets
    context_hash = str(context.get("context_hash", ""))
    return [f"- Review source coverage and claim policy before promoting generated context. {citation('guardrail', context_hash)}"]


def generic_bounded_graph_ordered_associations(context: dict[str, Any], associations: list[dict[str, Any]]) -> list[dict[str, Any]]:
    seed = context.get("seed", {})
    seed_key = str(seed.get("key") or "")
    seed_kind = str(seed.get("object_type") or seed.get("objectType") or "")
    relation_priority = {
        "implemented_by": 0,
        "documented_by": 1,
        "discussed_in": 2,
        "assignee": 3,
        "owner": 4,
        "reporter": 5,
        "author": 6,
        "creator": 7,
        "approver": 8,
        "reviewer": 9,
        "requested_reviewer": 10,
        "commenter": 11,
        "links_to": 12,
    }

    distances = bounded_graph_seed_distances(seed_key, associations)

    def endpoint_matches_seed(row: dict[str, Any]) -> bool:
        return (
            str(row.get("from_key") or "") == seed_key
            and (not seed_kind or str(row.get("from_kind") or "") == seed_kind)
        ) or (
            str(row.get("to_key") or "") == seed_key
            and (not seed_kind or str(row.get("to_kind") or "") == seed_kind)
        )

    def association_distance(row: dict[str, Any]) -> int:
        from_distance = distances.get(str(row.get("from_key") or ""), 1_000_000)
        to_distance = distances.get(str(row.get("to_key") or ""), 1_000_000)
        return min(from_distance, to_distance)

    return sorted(
        associations,
        key=lambda row: (
            0 if endpoint_matches_seed(row) else 1,
            association_distance(row),
            relation_priority.get(str(row.get("association_type") or ""), 100),
            str(row.get("from_key") or ""),
            str(row.get("association_type") or ""),
            str(row.get("to_key") or ""),
        ),
    )


def generic_baseline_what_not_to_claim_bullets(context: dict[str, Any]) -> list[str]:
    context_hash = str(context.get("context_hash", ""))
    limit = max_brief_bullets_per_section(context)
    bullets: list[str] = []
    if is_bounded_graph_context(context):
        source_coverage = context.get("analytics", {}).get("source_coverage", {})
        absence_allowed = str(source_coverage.get("absence_claims_allowed", {}).get("value") or "false").strip()
        coverage_state = str(source_coverage.get("coverage_state", {}).get("value") or "unknown").strip()
        gate_reason = str(source_coverage.get("absence_claims_allowed", {}).get("note") or "source_coverage_gate").strip()
        seed_key = str(context.get("seed", {}).get("key") or "object:unknown")
        if absence_allowed != "true" and coverage_state not in {"auth_limited"}:
            bullets.append(
                f"- Source coverage is `{coverage_state}`; absence claims remain gated by `{gate_reason}`. {citation('source_coverage', seed_key)}"
            )
    guardrails = [str(value).strip() for value in context.get("guardrails", []) if str(value).strip()]
    bullets.extend(f"- {guardrail} {citation('guardrail', context_hash)}" for guardrail in guardrails)
    if bullets:
        return bullets[:limit]
    return [f"- Do not promote derived graph context without a claimable citation or human review. {citation('guardrail', context_hash)}"]


def list_rows(rows_by_table: Any, table: str) -> list[dict[str, Any]]:
    if not isinstance(rows_by_table, dict):
        return []
    rows = rows_by_table.get(table, [])
    if not isinstance(rows, list):
        return []
    return [row for row in rows if isinstance(row, dict)]


def render_prompt(context: dict[str, Any], *, mode: str = "operating") -> str:
    generic_bounded = mode == "generic" and is_bounded_graph_context(context)
    prompt_context = {
        "seed": context.get("seed", {}),
        "context_hash": context.get("context_hash"),
        "traversal": context.get("traversal", {}),
        "rows": compact_prompt_rows(context.get("rows", {})),
        "guardrails": context.get("guardrails", []),
        "citation_policy": compact_prompt_citations(context.get("citations", [])),
    }
    prompt_analytics = compact_prompt_analytics(context.get("analytics", {}))
    if generic_bounded:
        prompt_context["source_coverage"] = prompt_analytics.get("source_coverage", {})
        prompt_context["graph_summary"] = generic_bounded_prompt_summary(context)
    else:
        prompt_context["analytics"] = prompt_analytics
    role, task, extra_rules = prompt_mode_content(mode)
    analytics_shortcuts = prompt_analytics_shortcut_lines(context)
    citation_boundary_rules = [
        "- Cite source coverage state only with a `source_coverage` citation.",
        "- Use only the citation tokens listed above; unlisted citation families are outside this generic bounded graph prompt scope.",
        "- Include a short `What Not To Claim` section when a guardrail blocks a product claim; cite each bullet with a guardrail or source coverage citation.",
    ] if generic_bounded else [
        "- Analytics citation shortcuts are not automatically claimable in `Confirmed Facts`; use source coverage, context, or claim-allowed row citations for confirmed facts.",
        "- Cite source coverage state with a `source_coverage` citation, not a forecast or blocker analytics citation.",
        "- Include a short `What Not To Claim` section when a guardrail blocks a product claim; cite each bullet with a guardrail or analytics citation.",
        "- Do not make ETA commitments unless the context explicitly says ETA readiness is true.",
        "- Do not turn blocker candidates into confirmed blockers.",
    ]
    return "\n".join(
        [
            "# Cubicle Graph Brief Prompt",
            "",
            "## Role",
            "",
            role,
            "",
            "## Required Output",
            "",
            "- Return exactly these Markdown sections: `# Operating Brief`, `## Confirmed Facts`, `## Validation Leads`, `## What Not To Claim`.",
            "- Write 2-3 bullets in each section; never write a fourth bullet in any section.",
            "- Stop immediately after the final `What Not To Claim` bullet.",
            "- Use bullet lists only; do not use Markdown tables, intro paragraphs, conclusions, or horizontal rules.",
            "- Use only facts present in the JSON context.",
            "- Cite every bullet with one or more bracket citations copied exactly from the allowed citation list.",
            "- Do not invent citation aliases from JSON keys; if a citation token is not listed, do not use it.",
            "- Never create bracket citations from JSON field names such as `seed`, `traversal`, `rows`, summary fields, or `source_coverage`; copy citations only from `Allowed Citations`.",
            "- Summary fields are copy-only hints, not citations; do not put summary field names inside brackets.",
            "- In `Confirmed Facts`, every row citation must have `claim_allowed=true`; use gated citations only for `Validation Leads` or `What Not To Claim`.",
            *citation_boundary_rules,
            "- Do not quote raw excerpts or source URLs unless a citation has `excerpt_allowed` or `source_url_allowed` set to true.",
            "- Do not alter citation dates, source-instance strings, row keys, or table names.",
            "- Separate confirmed facts from validation leads.",
            "- Keep source coverage state separate from absence-claim permission. If coverage is complete but absence claims are gated, say absence claims are gated; do not say coverage is incomplete.",
            "- If you cannot find a cited row for a claim, omit the claim.",
            *extra_rules,
            "",
            "## Allowed Citations",
            "",
            *[f"- {token}" for token in sorted(allowed_citations(context))],
            "",
            *analytics_shortcuts,
            "",
            "## Citation Policy",
            "",
            "The JSON context includes structured citation metadata. Use it to decide whether a citation supports a confirmed fact, a validation lead, or only a guardrail.",
            "",
            "## Task",
            "",
            task,
            "",
            "## Context JSON",
            "",
            "```json",
            json.dumps(prompt_context, indent=2, sort_keys=True),
            "```",
            "",
            "## Final Output Contract",
            "",
            "- Now write only the final brief.",
            "- Use exactly the required sections and no other text.",
            "- Write 2-3 bullets per section and 9 bullets maximum total.",
            "- Do not list every row. Select the highest-value cited examples.",
            "- Stop immediately after the final `What Not To Claim` bullet.",
            "",
        ]
    )


def prompt_analytics_shortcut_lines(context: dict[str, Any]) -> list[str]:
    if is_bounded_graph_context(context):
        return [
            "## Generic Graph Citation Scope",
            "",
            "- Use only context, source coverage, guardrail, graph object, graph association, and evidence citations copied from the allowed list.",
        ]
    return [
        "## Analytics Citation Shortcuts",
        "",
        "- Forecast readiness and risk ranking: [analytics:tpm_forecast_summary]",
        "- Measurement and precision readiness: [analytics:tpm_evaluation_readiness]",
        "- Blocker candidates: [analytics:tpm_blocker_candidates]",
    ]


def prompt_mode_content(mode: str) -> tuple[str, str, list[str]]:
    if mode == "generic":
        return (
            "You are an AI graph-context analyst. Your job is to explain the supplied bounded graph traversal, claim policy, and guardrails.",
            generic_llm_task(),
            [
                "- In `Confirmed Facts`, include the bounded traversal shape and at least one claimable row if citation policy allows it.",
                "- If the summary JSON includes `traversal_count_phrase`, use that exact phrase when describing traversal shape.",
                "- Follow the summary JSON's confirmed-fact instruction when deciding whether associations can appear in `Confirmed Facts`.",
                "- When citing an association row, copy that row's `endpoint_phrase` exactly and cite that same association row.",
                "- When an association row has `claim_gate_reason`, copy that reason exactly in `Validation Leads` or `What Not To Claim`.",
                "- When describing an association, write endpoint keys exactly as ``from_key` -> `to_key`` and name the `association_type`.",
                "- Do not put graph objects or graph associations with `claim_allowed=false` in `Confirmed Facts`.",
                "- In `Validation Leads`, explain derived topology edges or generated insights as context, not product truth.",
                "- In `What Not To Claim`, preserve guardrails for product truth, generated insight, source coverage, and derived-edge claims.",
                "- Do not prioritize generated summary facts unless they support graph safety, claim policy, or traversal interpretation.",
                "- If a graph row has `seed_reachable=false` or `claim_gate_reason=disconnected_from_seed_component`, do not mention it in any section.",
            ],
        )
    return (
        "You are an AI workstream analyst. Your job is to summarize the supplied Cubicle graph context, not to infer beyond it.",
        "Given the JSON context bundle, write a concise operating brief with citations. Use only cited rows. Separate confirmed facts from validation leads. Include 'what not to claim' when source coverage, measurement, or forecast gates are not ready.",
        [],
    )


def compact_prompt_rows(rows_by_table: dict[str, Any]) -> dict[str, Any]:
    compacted: dict[str, Any] = {}
    for table, rows in rows_by_table.items():
        if not isinstance(rows, list):
            compacted[table] = rows
            continue
        fields = PROMPT_ROW_FIELDS.get(table)
        compacted[table] = [compact_prompt_row(row, fields) for row in rows if isinstance(row, dict)]
    return compacted


def compact_prompt_analytics(analytics: dict[str, Any]) -> dict[str, Any]:
    compacted: dict[str, Any] = {}
    for key, value in analytics.items():
        metric_keys = PROMPT_ANALYTICS_METRICS.get(key)
        if isinstance(value, dict) and metric_keys is not None:
            compacted[key] = {metric: value[metric] for metric in metric_keys if metric in value}
        elif key == "forecast_reliability" and isinstance(value, list):
            compacted[key] = value[:3]
        else:
            compacted[key] = value
    return compacted


def compact_prompt_citations(citations: Any) -> list[dict[str, Any]]:
    if not isinstance(citations, list):
        return []
    fields = [
        "ref",
        "citationKind",
        "citation_kind",
        "nodeKind",
        "node_kind",
        "nodeKey",
        "node_key",
        "associationType",
        "association_type",
        "proofState",
        "proof_state",
        "freshnessState",
        "freshness_state",
        "visibility",
        "claimUse",
        "claim_use",
        "claimGateReason",
        "claim_gate_reason",
        "claimAllowed",
        "claim_allowed",
        "excerptAllowed",
        "excerpt_allowed",
        "sourceUrlAllowed",
        "source_url_allowed",
        "detail",
    ]
    compacted: list[dict[str, Any]] = []
    for row in citations[:80]:
        if not isinstance(row, dict):
            continue
        compacted.append({field: truncate_prompt_value(row[field]) for field in fields if field in row})
    return compacted


def compact_prompt_row(row: dict[str, Any], fields: list[str] | None) -> dict[str, Any]:
    selected_fields = fields or ["key", "id", "_table"]
    out: dict[str, Any] = {}
    for field in selected_fields:
        if field not in row:
            continue
        out[field] = truncate_prompt_value(row[field])
    return out


def truncate_prompt_value(value: Any) -> Any:
    if not isinstance(value, str):
        return value
    text = one_line(value)
    if len(text) <= PROMPT_TEXT_LIMIT:
        return text
    return text[: PROMPT_TEXT_LIMIT - 3].rstrip() + "..."


def mlx_lm_command(python_path: str, model: str, max_tokens: int) -> str:
    tokens = bounded(max_tokens, 64, 32768)
    return " ".join(
        shlex.quote(part)
        for part in [
            python_path,
            "-m",
            "mlx_lm",
            "generate",
            "--model",
            model,
            "--prompt",
            "-",
            "--max-tokens",
            str(tokens),
            "--temp",
            "0",
            "--verbose",
            "False",
        ]
    )


def run_llm_command(command: str, prompt: str, timeout_seconds: int) -> str:
    argv = shlex.split(command)
    if not argv:
        raise ValueError("llm command is empty")
    try:
        result = subprocess.run(
            argv,
            input=prompt,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=bounded(timeout_seconds, 1, 900),
            check=False,
        )
    except FileNotFoundError as exc:
        raise RuntimeError(f"llm command not found: {argv[0]}") from exc
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"llm command timed out after {bounded(timeout_seconds, 1, 900)}s") from exc
    if result.returncode != 0:
        stderr = one_line(result.stderr)[:500]
        raise RuntimeError(f"llm command failed with exit code {result.returncode}: {stderr}")
    output = clean_command_output(result.stdout)
    if not output:
        raise RuntimeError("llm command produced empty output")
    return output + "\n"


def clean_command_output(text: str) -> str:
    without_ansi = re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", text or "")
    normalized_citations = normalize_citation_brackets(without_ansi)
    return "\n".join(line.rstrip() for line in normalized_citations.replace("\r", "\n").splitlines()).strip()


def normalize_citation_brackets(text: str) -> str:
    return re.sub(r"【([^【】\n]+:[^【】\n]+)】", r"[\1]", text or "")


def evaluate_llm_brief(context: dict[str, Any], answer_text: str) -> dict[str, Any]:
    allowed = allowed_citations(context)
    citations = extract_citations(answer_text)
    unknown = sorted({token for token in citations if token not in allowed})
    claim_lines = material_claim_lines(answer_text)
    uncited_lines = [line for line in claim_lines if not extract_citations(line)]
    citation_policy = citation_policy_violations(context, answer_text)
    statement_support = statement_support_audit(context, answer_text)
    forbidden = forbidden_claim_violations(context, answer_text)
    semantic = semantic_guardrail_violations(context, answer_text)
    structure = brief_structure_violations(answer_text, max_bullets=max_brief_bullets_per_section(context))
    return {
        "context_hash": context.get("context_hash"),
        "allowed_citation_count": len(allowed),
        "citation_count": len(citations),
        "unknown_citation_count": len(unknown),
        "unknown_citations": unknown,
        "material_claim_line_count": len(claim_lines),
        "uncited_material_claim_line_count": len(uncited_lines),
        "uncited_material_claim_lines": uncited_lines[:20],
        "citation_policy_violation_count": len(citation_policy),
        "citation_policy_violations": citation_policy,
        "statement_support": statement_support,
        "unsupported_statement_count": statement_support["unsupported_statement_count"],
        "forbidden_claim_violation_count": len(forbidden),
        "forbidden_claim_violations": forbidden,
        "semantic_guardrail_violation_count": len(semantic),
        "semantic_guardrail_violations": semantic,
        "structure_violation_count": len(structure),
        "structure_violations": structure,
        "passes_smoke_eval": not unknown and not uncited_lines and not citation_policy and not statement_support["unsupported_statement_count"] and not forbidden and not semantic and not structure,
    }


def evaluate_brief_for_gates(context: dict[str, Any], answer_text: str, golden_spec: Any | None = None) -> dict[str, Any]:
    evaluation = evaluate_llm_brief(context, answer_text)
    evaluation["passes_eval"] = evaluation["passes_smoke_eval"]
    if golden_spec is not None:
        golden_eval = evaluate_golden_questions(answer_text, golden_spec)
        evaluation["golden_eval"] = golden_eval
        evaluation["passes_eval"] = bool(evaluation["passes_smoke_eval"] and golden_eval["passes_golden_eval"])
    return evaluation


def evaluate_golden_questions(answer_text: str, golden_spec: Any) -> dict[str, Any]:
    questions = golden_questions(golden_spec)
    rows: list[dict[str, Any]] = []
    for index, question in enumerate(questions):
        row = evaluate_golden_question(answer_text, question, index)
        rows.append(row)
    failure_count = sum(1 for row in rows if not row["passes"])
    category_summary = summarize_golden_question_categories(rows)
    source_coverage_summary = summarize_golden_source_coverage_states(rows)
    missing_required_categories = [
        category
        for category in golden_required_categories(golden_spec)
        if category_summary.get(category, {}).get("question_count", 0) == 0
    ]
    missing_required_source_coverage_states = [
        state
        for state in golden_required_source_coverage_states(golden_spec)
        if source_coverage_summary.get(state, {}).get("question_count", 0) == 0
    ]
    return {
        "question_count": len(rows),
        "pass_count": len(rows) - failure_count,
        "failure_count": failure_count,
        "category_summary": category_summary,
        "source_coverage_summary": source_coverage_summary,
        "missing_required_categories": missing_required_categories,
        "missing_required_source_coverage_states": missing_required_source_coverage_states,
        "passes_golden_eval": failure_count == 0 and not missing_required_categories and not missing_required_source_coverage_states,
        "questions": rows,
    }


def evaluate_golden_answer_comparison(golden_spec: Any, answers_spec: Any, *, base_dir: Path | None = None) -> dict[str, Any]:
    answers = golden_answer_entries(answers_spec, base_dir=base_dir)
    rows: list[dict[str, Any]] = []
    for index, answer in enumerate(answers):
        golden_eval = evaluate_golden_questions(answer["text"], golden_spec)
        rows.append(
            {
                "key": answer["key"],
                "answer_key": answer["key"],
                "label": answer["label"],
                "path": answer.get("path"),
                "strategy": answer.get("strategy"),
                "answer_kind": answer.get("answer_kind"),
                "rank": 0,
                "passes_golden_eval": golden_eval["passes_golden_eval"],
                "pass_count": golden_eval["pass_count"],
                "failure_count": golden_eval["failure_count"],
                "question_count": golden_eval["question_count"],
                "category_summary": golden_eval["category_summary"],
                "source_coverage_summary": golden_eval["source_coverage_summary"],
                "golden_eval": golden_eval,
            }
        )
    ranked = sorted(rows, key=lambda row: (-int(row["pass_count"]), int(row["failure_count"]), str(row["key"])))
    for rank, row in enumerate(ranked, start=1):
        row["rank"] = rank
    promotion_gates = evaluate_answer_promotion_gates(answers_spec, ranked, golden_required_categories(golden_spec))
    return {
        "answer_count": len(rows),
        "question_count": len(golden_questions(golden_spec)),
        "required_categories": golden_required_categories(golden_spec),
        "required_source_coverage_states": golden_required_source_coverage_states(golden_spec),
        "best_answer_keys_by_category": best_answer_keys_by_category(ranked),
        "best_answer_keys": [row["key"] for row in ranked if ranked and row["pass_count"] == ranked[0]["pass_count"] and row["failure_count"] == ranked[0]["failure_count"]],
        "promotion_gates": promotion_gates,
        "passes_promotion_gates": all(row["passes"] for row in promotion_gates),
        "answers": ranked,
    }


def evaluate_answer_promotion_gates(answers_spec: Any, rows: list[dict[str, Any]], default_categories: list[str]) -> list[dict[str, Any]]:
    gates = answer_promotion_gates(answers_spec)
    if not gates:
        return []
    rows_by_key = {str(row["key"]): row for row in rows}
    out: list[dict[str, Any]] = []
    for index, gate in enumerate(gates):
        if not isinstance(gate, dict):
            continue
        candidate_key = str(gate.get("candidate_key") or gate.get("candidate") or "").strip()
        baseline_key = str(gate.get("baseline_key") or gate.get("baseline") or "").strip()
        categories = sorted(set(string_list(gate.get("required_categories", [])) or default_categories))
        candidate = rows_by_key.get(candidate_key)
        baseline = rows_by_key.get(baseline_key)
        if candidate is None or baseline is None:
            out.append(
                {
                    "key": str(gate.get("key") or f"promotion_gate:{index + 1}"),
                    "candidate_key": candidate_key,
                    "baseline_key": baseline_key,
                    "required_categories": categories,
                    "passes": False,
                    "failure_reasons": ["missing_candidate_or_baseline"],
                    "category_results": [],
                }
            )
            continue
        category_results = [
            answer_promotion_category_result(candidate, baseline, category)
            for category in categories
        ]
        candidate_better_overall = int(candidate["pass_count"]) > int(baseline["pass_count"]) or int(candidate["failure_count"]) < int(baseline["failure_count"])
        no_category_regressions = all(result["candidate_no_worse"] for result in category_results)
        candidate_passes_eval = bool(candidate.get("passes_golden_eval"))
        failure_reasons: list[str] = []
        if not candidate_passes_eval:
            failure_reasons.append("candidate_does_not_pass_golden_eval")
        if not candidate_better_overall:
            failure_reasons.append("candidate_does_not_beat_baseline_overall")
        if not no_category_regressions:
            failure_reasons.append("candidate_regresses_required_category")
        out.append(
            {
                "key": str(gate.get("key") or f"{candidate_key}:over:{baseline_key}"),
                "candidate_key": candidate_key,
                "baseline_key": baseline_key,
                "required_categories": categories,
                "candidate_pass_count": int(candidate["pass_count"]),
                "candidate_failure_count": int(candidate["failure_count"]),
                "baseline_pass_count": int(baseline["pass_count"]),
                "baseline_failure_count": int(baseline["failure_count"]),
                "candidate_better_overall": candidate_better_overall,
                "candidate_passes_eval": candidate_passes_eval,
                "no_category_regressions": no_category_regressions,
                "failure_reasons": failure_reasons,
                "category_results": category_results,
                "passes": not failure_reasons,
            }
        )
    return out


def answer_promotion_gates(answers_spec: Any) -> list[dict[str, Any]]:
    if not isinstance(answers_spec, dict):
        return []
    gates = answers_spec.get("promotion_gates")
    if isinstance(gates, list):
        return [gate for gate in gates if isinstance(gate, dict)]
    gate = answers_spec.get("promotion_gate")
    if isinstance(gate, dict):
        return [gate]
    candidate_keys = string_list(answers_spec.get("candidate_keys", []))
    baseline_key = str(answers_spec.get("baseline_key") or "").strip()
    if baseline_key and candidate_keys:
        return [{"baseline_key": baseline_key, "candidate_key": candidate_key} for candidate_key in candidate_keys]
    return []


def answer_promotion_category_result(candidate: dict[str, Any], baseline: dict[str, Any], category: str) -> dict[str, Any]:
    candidate_category = candidate.get("category_summary", {}).get(category, {})
    baseline_category = baseline.get("category_summary", {}).get(category, {})
    candidate_pass_count = int(candidate_category.get("pass_count", 0))
    candidate_failure_count = int(candidate_category.get("failure_count", 0))
    baseline_pass_count = int(baseline_category.get("pass_count", 0))
    baseline_failure_count = int(baseline_category.get("failure_count", 0))
    return {
        "category": category,
        "candidate_pass_count": candidate_pass_count,
        "candidate_failure_count": candidate_failure_count,
        "baseline_pass_count": baseline_pass_count,
        "baseline_failure_count": baseline_failure_count,
        "candidate_no_worse": candidate_pass_count >= baseline_pass_count and candidate_failure_count <= baseline_failure_count,
    }


def golden_answer_entries(answers_spec: Any, *, base_dir: Path | None = None) -> list[dict[str, Any]]:
    if isinstance(answers_spec, list):
        entries = answers_spec
    elif isinstance(answers_spec, dict):
        entries = answers_spec.get("answers", [])
    else:
        raise ValueError("answer comparison spec must be an object with answers or a list")
    if not isinstance(entries, list):
        raise ValueError("answer comparison entries must be a list")
    out: list[dict[str, Any]] = []
    for index, entry in enumerate(entries):
        if not isinstance(entry, dict):
            continue
        key = str(entry.get("key") or f"answer:{index + 1}")
        label = str(entry.get("label") or key)
        text = str(entry.get("text") or "")
        path_value = str(entry.get("path") or "").strip()
        if path_value:
            answer_path = Path(path_value)
            if not answer_path.is_absolute() and base_dir is not None:
                answer_path = base_dir / answer_path
            text = answer_path.read_text(encoding="utf-8")
        out.append(
            {
                "key": key,
                "label": label,
                "text": text,
                "path": path_value or None,
                "strategy": optional_answer_metadata(entry, "strategy"),
                "answer_kind": optional_answer_metadata(entry, "answer_kind"),
            }
        )
    return out


def optional_answer_metadata(entry: dict[str, Any], key: str) -> str | None:
    value = str(entry.get(key) or "").strip()
    return value or None


def golden_questions(golden_spec: Any) -> list[dict[str, Any]]:
    if isinstance(golden_spec, list):
        questions = golden_spec
    elif isinstance(golden_spec, dict):
        questions = golden_spec.get("questions", [])
    else:
        raise ValueError("golden eval spec must be an object with questions or a list")
    if not isinstance(questions, list):
        raise ValueError("golden eval questions must be a list")
    return [question for question in questions if isinstance(question, dict)]


def golden_required_categories(golden_spec: Any) -> list[str]:
    if not isinstance(golden_spec, dict):
        return []
    return sorted(set(string_list(golden_spec.get("required_categories", []))))


def golden_required_source_coverage_states(golden_spec: Any) -> list[str]:
    if not isinstance(golden_spec, dict):
        return []
    return sorted(set(string_list(golden_spec.get("required_source_coverage_states", []))))


def evaluate_golden_question(answer_text: str, question: dict[str, Any], index: int) -> dict[str, Any]:
    normalized_answer = normalized_for_match(answer_text)
    expected_facts = golden_expected_items(question, "expected_facts")
    expected_no_answer = golden_expected_items(question, "expected_no_answer")
    raw_missing_expected_facts = [
        expected
        for expected in expected_facts
        if not golden_expected_item_matches(answer_text, normalized_answer, expected)
    ]
    missing_expected_no_answer = [
        expected
        for expected in expected_no_answer
        if not golden_expected_item_matches(answer_text, normalized_answer, expected)
    ]
    no_answer_used = bool(expected_no_answer) and not missing_expected_no_answer and (bool(raw_missing_expected_facts) or not expected_facts)
    missing_expected_facts = [] if no_answer_used else raw_missing_expected_facts
    missing_required_no_answer = missing_expected_no_answer if expected_no_answer and not expected_facts else []
    missing_expected_citations = [
        token
        for token in string_list(question.get("expected_citations", []))
        if token not in extract_citations(answer_text)
    ]
    forbidden_phrase_hits = golden_forbidden_phrase_hits(answer_text, string_list(question.get("forbidden_phrases", [])))
    missing_required_sections = [
        section
        for section in string_list(question.get("required_sections", []))
        if section not in answer_text
    ]
    passes = not missing_expected_facts and not missing_required_no_answer and not missing_expected_citations and not forbidden_phrase_hits and not missing_required_sections
    return {
        "key": str(question.get("key") or f"question:{index + 1}"),
        "question": str(question.get("question") or ""),
        "categories": golden_question_categories(question),
        "source_coverage_state": golden_question_source_coverage_state(question),
        "no_answer_allowed": bool(expected_no_answer),
        "no_answer_used": no_answer_used,
        "missing_expected_facts": missing_expected_facts,
        "missing_expected_no_answer": missing_expected_no_answer,
        "missing_expected_citations": missing_expected_citations,
        "forbidden_phrase_hits": forbidden_phrase_hits,
        "missing_required_sections": missing_required_sections,
        "passes": passes,
    }


def golden_question_categories(question: dict[str, Any]) -> list[str]:
    categories = string_list(question.get("categories", []))
    category = str(question.get("category") or "").strip()
    if category:
        categories.append(category)
    if not categories:
        categories.append("uncategorized")
    return sorted(set(categories))


def golden_question_source_coverage_state(question: dict[str, Any]) -> str:
    state = str(question.get("source_coverage_state") or "").strip()
    return state or "unspecified"


def summarize_golden_question_categories(rows: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    summary: dict[str, dict[str, Any]] = {}
    for row in rows:
        for category in string_list(row.get("categories", [])):
            entry = summary.setdefault(category, {"question_count": 0, "pass_count": 0, "failure_count": 0, "passes_category": True})
            entry["question_count"] += 1
            if row.get("passes"):
                entry["pass_count"] += 1
            else:
                entry["failure_count"] += 1
                entry["passes_category"] = False
    return {key: summary[key] for key in sorted(summary)}


def summarize_golden_source_coverage_states(rows: list[dict[str, Any]]) -> dict[str, dict[str, Any]]:
    summary: dict[str, dict[str, Any]] = {}
    for row in rows:
        state = str(row.get("source_coverage_state") or "unspecified")
        entry = summary.setdefault(state, {"question_count": 0, "pass_count": 0, "failure_count": 0, "passes_state": True})
        entry["question_count"] += 1
        if row.get("passes"):
            entry["pass_count"] += 1
        else:
            entry["failure_count"] += 1
            entry["passes_state"] = False
    return {key: summary[key] for key in sorted(summary)}


def best_answer_keys_by_category(rows: list[dict[str, Any]]) -> dict[str, list[str]]:
    categories = sorted({category for row in rows for category in row.get("category_summary", {}).keys()})
    out: dict[str, list[str]] = {}
    for category in categories:
        scored = [
            (
                int(row.get("category_summary", {}).get(category, {}).get("pass_count", 0)),
                int(row.get("category_summary", {}).get(category, {}).get("failure_count", 0)),
                str(row["key"]),
            )
            for row in rows
        ]
        if not scored:
            continue
        best_pass = max(score[0] for score in scored)
        best_failure = min(score[1] for score in scored if score[0] == best_pass)
        out[category] = sorted(key for pass_count, failure_count, key in scored if pass_count == best_pass and failure_count == best_failure)
    return out


def golden_expected_items(question: dict[str, Any], key: str) -> list[Any]:
    values = question.get(key, [])
    if not isinstance(values, list):
        return []
    return values


def golden_expected_item_matches(answer_text: str, normalized_answer: str, expected: Any) -> bool:
    if isinstance(expected, str):
        return normalized_for_match(expected) in normalized_answer
    if not isinstance(expected, dict):
        return False
    if isinstance(expected.get("any_of"), list):
        return any(golden_expected_item_matches(answer_text, normalized_answer, option) for option in expected["any_of"])
    if isinstance(expected.get("all_of"), list):
        return all(golden_expected_item_matches(answer_text, normalized_answer, option) for option in expected["all_of"])
    text = str(expected.get("text") or expected.get("contains") or "").strip()
    citation_token = str(expected.get("citation") or "").strip()
    citation_prefix = str(expected.get("citation_prefix") or "").strip()
    if text and citation_token:
        normalized_text = normalized_for_match(text)
        return any(normalized_text in normalized_for_match(line) and citation_token in line for line in material_claim_lines(answer_text))
    if text and citation_prefix:
        normalized_text = normalized_for_match(text)
        return any(
            normalized_text in normalized_for_match(line) and any(token.startswith(citation_prefix) for token in extract_citations(line))
            for line in material_claim_lines(answer_text)
        )
    if text and normalized_for_match(text) not in normalized_answer:
        return False
    if citation_token and citation_token not in extract_citations(answer_text):
        return False
    if citation_prefix and not any(token.startswith(citation_prefix) for token in extract_citations(answer_text)):
        return False
    return bool(text or citation_token or citation_prefix)


def golden_forbidden_phrase_hits(answer_text: str, phrases: list[str]) -> list[str]:
    hits: list[str] = []
    for line in material_claim_lines(answer_text):
        normalized_line = normalized_for_match(line)
        if explicit_prohibition_line(normalized_line):
            continue
        for phrase in phrases:
            if forbidden_phrase_present_without_negation(normalized_line, normalized_for_match(phrase)):
                hits.append(phrase)
    return sorted(set(hits))


def forbidden_phrase_present_without_negation(normalized_line: str, normalized_phrase: str) -> bool:
    if not normalized_phrase:
        return False
    start = 0
    while True:
        index = normalized_line.find(normalized_phrase, start)
        if index < 0:
            return False
        prefix = normalized_line[max(0, index - 32) : index]
        if not any(marker in prefix for marker in ["not ", "no ", "without ", "avoid ", "never "]):
            return True
        start = index + len(normalized_phrase)


def string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [str(item) for item in value if str(item).strip()]


def normalized_for_match(text: str) -> str:
    return " ".join((text or "").lower().split())


def repair_llm_brief(context: dict[str, Any], answer_text: str) -> str:
    sections = extract_brief_sections(answer_text)
    limit = max_brief_bullets_per_section(context)
    confirmed = valid_section_bullets(context, "## Confirmed Facts", sections.get("## Confirmed Facts", []))[:limit]
    validation = valid_section_bullets(context, "## Validation Leads", sections.get("## Validation Leads", []))[:limit]
    what_not = valid_section_bullets(context, "## What Not To Claim", sections.get("## What Not To Claim", []))[:limit]
    if is_bounded_graph_context(context):
        confirmed = fill_repair_bullets(generic_baseline_confirmed_bullets(context), confirmed, replace_same_citations=False, limit=limit)
        validation = fill_repair_bullets(generic_baseline_validation_bullets(context), validation, replace_same_citations=False, limit=limit)
        what_not = fill_repair_bullets(generic_baseline_what_not_to_claim_bullets(context), what_not, replace_same_citations=False, limit=limit)
    if not confirmed:
        confirmed = fallback_confirmed_bullets(context)
    if not validation:
        validation = fallback_validation_bullets(context)
    if not what_not:
        what_not = fallback_what_not_to_claim_bullets(context)
    return "\n".join(
        [
            "# Operating Brief",
            "",
            "## Confirmed Facts",
            "",
            *confirmed[:limit],
            "",
            "## Validation Leads",
            "",
            *validation[:limit],
            "",
            "## What Not To Claim",
            "",
            *what_not[:limit],
            "",
        ]
    )


def fill_repair_bullets(
    existing: list[str],
    candidates: list[str],
    *,
    replace_same_citations: bool = True,
    limit: int = MAX_BRIEF_BULLETS_PER_SECTION,
) -> list[str]:
    out = list(existing)
    seen_bullets = {normalized_for_match(row) for row in out}
    for candidate in candidates:
        bullet_key = normalized_for_match(candidate)
        if bullet_key in seen_bullets:
            continue
        candidate_citations = tuple(extract_citations(candidate))
        duplicate_citation = False
        if candidate_citations:
            duplicate_citation = any(tuple(extract_citations(existing_row)) == candidate_citations for existing_row in out)
        if duplicate_citation and not replace_same_citations:
            continue
        replaced_existing = False
        if replace_same_citations and candidate_citations:
            for index, existing_row in enumerate(out):
                if tuple(extract_citations(existing_row)) == candidate_citations:
                    seen_bullets.discard(normalized_for_match(existing_row))
                    out[index] = candidate
                    seen_bullets.add(bullet_key)
                    replaced_existing = True
                    break
        if replaced_existing:
            continue
        out.append(candidate)
        seen_bullets.add(bullet_key)
        if len(out) >= limit:
            break
    return out


def extract_brief_sections(answer_text: str) -> dict[str, list[str]]:
    sections: dict[str, list[str]] = {}
    current: str | None = None
    for raw_line in (answer_text or "").splitlines():
        line = raw_line.strip()
        if line in REQUIRED_BRIEF_SECTIONS[1:]:
            current = line
            sections.setdefault(current, [])
            continue
        if current and line.startswith("- "):
            sections[current].append(line)
    return sections


def valid_section_bullets(context: dict[str, Any], section: str, bullets: list[str]) -> list[str]:
    allowed = allowed_citations(context)
    out: list[str] = []
    for bullet in bullets:
        citations = extract_citations(bullet)
        if not citations or any(token not in allowed for token in citations):
            continue
        if semantic_guardrail_violations(context, bullet):
            continue
        if disconnected_seed_component_mentions(context, bullet):
            continue
        if any(citation_is_disconnected_from_seed_component(context, token) for token in citations):
            continue
        if section == "## Confirmed Facts" and any(not citation_can_support_confirmed_fact(context, token) for token in citations):
            continue
        if section == "## Confirmed Facts" and confirmed_relationship_claim_requires_association_citation(context, bullet):
            continue
        out.append(bullet)
    return out


def fallback_confirmed_bullets(context: dict[str, Any]) -> list[str]:
    analytics = context.get("analytics", {})
    forecast = analytics.get("forecast_summary", {})
    blocker_count = analytics.get("blocker_candidate_count", 0)
    eta_ready = metric_value(forecast, "eta_forecast_ready", "unknown")
    return [
        f"- The graph context was built for `{context.get('seed', {}).get('key', 'unknown')}` with context hash `{context.get('context_hash')}`. {citation('context', str(context.get('context_hash')))}",
        f"- ETA readiness is `{eta_ready}`, so forecasts remain risk-triage inputs rather than date commitments. {citation('analytics', 'tpm_forecast_summary')}",
        f"- Analytics reports {blocker_count} blocker candidates that still need validation. {citation('analytics', 'tpm_blocker_candidates')}",
    ]


def fallback_validation_bullets(context: dict[str, Any]) -> list[str]:
    if is_bounded_graph_context(context):
        return generic_baseline_validation_bullets(context)
    bullets: list[str] = []
    for item in context.get("rows", {}).get("work_program_items", [])[:MAX_BRIEF_BULLETS_PER_SECTION]:
        subject = item.get("subject_key") or item.get("key") or "unknown"
        title = item.get("title") or "Review cited work item"
        bullets.append(f"- Validate `{subject}` before promoting it to product action: {title}. {row_citation(item)}")
    if bullets:
        return bullets
    return [
        f"- Add human review labels before promoting generated insight claims. {citation('analytics', 'tpm_evaluation_readiness')}",
    ]


def fallback_what_not_to_claim_bullets(context: dict[str, Any]) -> list[str]:
    context_hash = str(context.get("context_hash"))
    guardrails = context.get("guardrails", [])
    if guardrails:
        return [f"- {guardrail} {citation('guardrail', context_hash)}" for guardrail in guardrails[:MAX_BRIEF_BULLETS_PER_SECTION]]
    return [f"- Do not make uncited product claims from this generated brief. {citation('guardrail', context_hash)}"]


def metric_value(metrics: Any, key: str, default: str) -> str:
    if not isinstance(metrics, dict):
        return default
    value = metrics.get(key, {})
    if isinstance(value, dict):
        return str(value.get("value", default))
    return str(value or default)


def brief_structure_violations(answer_text: str, *, max_bullets: int = MAX_BRIEF_BULLETS_PER_SECTION) -> list[dict[str, Any]]:
    lines = [line.rstrip() for line in (answer_text or "").splitlines()]
    violations: list[dict[str, Any]] = []
    stripped_lines = [line.strip() for line in lines if line.strip()]
    for section in REQUIRED_BRIEF_SECTIONS:
        count = sum(1 for line in stripped_lines if line == section)
        if count != 1:
            violations.append({"kind": "required_section_count", "section": section, "count": count})
    for line in stripped_lines:
        if line.startswith("|"):
            violations.append({"kind": "markdown_table", "line": line})
        if set(line) <= {"-"} and len(line) >= 3:
            violations.append({"kind": "horizontal_rule", "line": line})
        if is_material_nonbullet_line(line):
            violations.append({"kind": "nonbullet_material_line", "line": line})
    bullet_counts = section_bullet_counts(lines)
    for section in REQUIRED_BRIEF_SECTIONS[1:]:
        count = bullet_counts.get(section, 0)
        if count > max_bullets:
            violations.append({"kind": "too_many_bullets", "section": section, "count": count, "maximum": max_bullets})
    return violations


def section_bullet_counts(lines: list[str]) -> dict[str, int]:
    counts: dict[str, int] = {}
    current_section: str | None = None
    for raw_line in lines:
        line = raw_line.strip()
        if line in REQUIRED_BRIEF_SECTIONS:
            current_section = line
            counts.setdefault(current_section, 0)
            continue
        if current_section and line.startswith("- "):
            counts[current_section] = counts.get(current_section, 0) + 1
    return counts


def is_material_nonbullet_line(line: str) -> bool:
    if not line or line in REQUIRED_BRIEF_SECTIONS or line.startswith("- "):
        return False
    if line.startswith("```") or line.lower() in {"confirmed facts", "validation leads", "what not to claim"}:
        return False
    return len(line) >= 18


def persist_ai_graph_brief_insight(
    conn: sqlite3.Connection,
    context: dict[str, Any],
    answer_text: str,
    evaluation: dict[str, Any],
    *,
    llm_command: str | None,
    llm_model_name: str | None,
    generated_at: str | None,
    prompt_mode: str = "operating",
) -> dict[str, Any]:
    if not table_exists(conn, "work_insights") or not table_exists(conn, "evidences"):
        raise RuntimeError("ontology DB must have work_insights and evidences tables to persist AI graph brief")
    if not evaluation.get("passes_smoke_eval"):
        raise RuntimeError("AI graph brief must pass smoke evaluation before persistence")
    now = generated_at or datetime.now(timezone.utc).isoformat()
    prompt_mode = normalized_graph_brief_prompt_mode(prompt_mode)
    evaluation = dict(evaluation)
    evaluation.setdefault("prompt_mode", prompt_mode)
    context_hash = str(context.get("context_hash") or stable_digest([answer_text]))
    seed = context.get("seed", {})
    workstream_key = str(seed.get("key") or "workstream:unknown")
    source_instance = str(seed.get("source_instance") or "")
    external_id = f"{workstream_key}|{prompt_mode}|{context_hash}|ai_graph_brief"
    insight_key = f"work-insight:cubicle-ai:{source_instance or 'all'}:{stable_digest([external_id])}"
    answer_hash = stable_digest([answer_text])
    row_counts = {
        table: len(rows)
        for table, rows in context.get("rows", {}).items()
        if isinstance(rows, list)
    }
    summary = (
        f"AI graph brief for {workstream_key}: {evaluation.get('material_claim_line_count', 0)} cited claim lines; "
        f"{evaluation.get('citation_count', 0)} citations; context {context_hash}."
    )
    details = "\n\n".join(
        [
            answer_text.strip(),
            "",
            "Evaluation:",
            json.dumps(evaluation, indent=2, sort_keys=True),
        ]
    ).strip()
    insight_values = {
        "key": insight_key,
        "insight_kind": "ai_graph_brief",
        "severity": "info",
        "producer_state": "current",
        "subject_kind": "unknown",
        "subject_key": workstream_key,
        "title": f"AI graph brief: {workstream_key}",
        "details": details,
        "recommended_action": "Review the generated brief, validate cited claims, and decide which follow-ups become product actions.",
        "model_name": llm_model_name or model_name_from_command(llm_command),
        "model_version": answer_hash,
        "model_method": graph_brief_model_method(prompt_mode),
        "score": 100.0 if evaluation.get("passes_smoke_eval") else 0.0,
        "score_explanation": (
            f"smoke_eval={str(bool(evaluation.get('passes_smoke_eval'))).lower()}; "
            f"unknown_citations={evaluation.get('unknown_citation_count', 0)}; "
            f"uncited_claims={evaluation.get('uncited_material_claim_line_count', 0)}; "
            f"forbidden_claims={evaluation.get('forbidden_claim_violation_count', 0)}"
        ),
        "evidence_count": 1,
        "source_system": "cubicle_ai",
        "source_instance": source_instance,
        "external_kind": "ai_graph_brief",
        "external_id": external_id,
        "source_url": f"cubicle://graph-brief/{prompt_mode}/{context_hash}",
        "source_version": context_hash,
        "source_updated_at": now,
        "content_hash": answer_hash,
        "deletion_state": "present",
        "freshness_state": "fresh",
        "visibility": "private",
        "confidence": 0.72,
        "event_count": sum(row_counts.values()),
        "first_seen_at": now,
        "last_activity_at": now,
        "rank_score": 100.0,
        "created_at": now,
        "updated_at": now,
    }
    upsert_by_key(conn, "work_insights", insight_values)
    insight_id = row_id_by_key(conn, "work_insights", insight_key)
    supersede_prior_ai_graph_briefs(conn, source_instance, workstream_key, insight_id, now, prompt_mode=prompt_mode)
    evidence_key = f"evidence:cubicle-ai:{source_instance or 'all'}:{stable_digest([external_id, 'evidence'])}"
    evidence_values = {
        "key": evidence_key,
        "claim_kind": "generated_summary",
        "claim_target_kind": "work_insight",
        "claim_target_id": insight_id,
        "claim_field": "details",
        "locator_kind": "ai_graph_brief",
        "locator": f"context_hash:{context_hash}",
        "source_span_key": context_hash,
        "ordinal": 0,
        "excerpt": truncate_evidence_excerpt(answer_text),
        "excerpt_truncated": len(answer_text) > 2000,
        "text_hash": stable_digest([answer_text]),
        "proof_state": "generated",
        "observed_at": now,
        "source_system": "cubicle_ai",
        "source_instance": source_instance,
        "external_kind": "ai_graph_brief_evidence",
        "external_id": f"{external_id}|evidence",
        "source_url": f"cubicle://graph-brief/{prompt_mode}/{context_hash}",
        "source_version": context_hash,
        "source_updated_at": now,
        "content_hash": stable_digest([answer_text, json.dumps(evaluation, sort_keys=True)]),
        "deletion_state": "present",
        "freshness_state": "fresh",
        "visibility": "private",
        "confidence": 0.72,
        "created_at": now,
        "updated_at": now,
    }
    upsert_by_key(conn, "evidences", evidence_values)
    evidence_id = row_id_by_key(conn, "evidences", evidence_key)
    if column_exists(conn, "work_insights", "latest_evidence_id") and column_exists(conn, "work_insights", "evidence_count"):
        conn.execute("update work_insights set latest_evidence_id = ?, evidence_count = ? where id = ?", (evidence_id, 1, insight_id))
    elif column_exists(conn, "work_insights", "latest_evidence_id"):
        conn.execute("update work_insights set latest_evidence_id = ? where id = ?", (evidence_id, insight_id))
    snapshot = persist_ai_graph_brief_snapshot(
        conn,
        context,
        answer_text,
        evaluation,
        evidence_id=evidence_id,
        generated_at=now,
        prompt_mode=prompt_mode,
    )
    conn.commit()
    persisted = {
        "work_insight_id": insight_id,
        "work_insight_key": insight_key,
        "evidence_id": evidence_id,
        "evidence_key": evidence_key,
        "external_id": external_id,
        "context_hash": context_hash,
        "prompt_mode": prompt_mode,
    }
    if snapshot:
        persisted["work_program_brief_snapshot"] = snapshot
    return persisted


def persist_ai_graph_brief_snapshot(
    conn: sqlite3.Connection,
    context: dict[str, Any],
    answer_text: str,
    evaluation: dict[str, Any],
    *,
    evidence_id: int,
    generated_at: str,
    prompt_mode: str = "operating",
) -> dict[str, Any] | None:
    if not table_exists(conn, "work_program_brief_snapshots") or not table_exists(conn, "workstreams"):
        return None
    prompt_mode = normalized_graph_brief_prompt_mode(prompt_mode)
    seed = context.get("seed", {})
    workstream_key = str(seed.get("key") or "workstream:unknown")
    source_instance = str(seed.get("source_instance") or "")
    run = latest_work_program_run(conn, source_instance, workstream_key)
    run_generated_at = row_value(run, "generated_at") if run is not None else None
    snapshot_generated_at = str(run_generated_at) if run_generated_at else generated_at
    workstream_id = workstream_id_for_key(conn, workstream_key)
    context_hash = str(context.get("context_hash") or stable_digest([answer_text]))
    external_id = f"{workstream_key}|{prompt_mode}|{context_hash}|ai_graph_brief_snapshot"
    snapshot_key = f"work-program-brief-snapshot:cubicle-ai:{source_instance or 'all'}:{stable_digest([external_id])}"
    rows = context.get("rows", {})
    analytics = context.get("analytics", {})
    forecast_state = forecast_readiness_state(analytics)
    values = {
        "key": snapshot_key,
        "workstream_id": workstream_id,
        "workstream_key": workstream_key.removeprefix("workstream:"),
        "generated_at": snapshot_generated_at,
        "operating_status": "attention_required",
        "decision_pressure": "human_review_required",
        "forecast_state": forecast_state,
        "primary_risk": primary_guardrail(context),
        "executive_summary": first_brief_bullet(answer_text) or f"AI graph brief for {workstream_key}.",
        "recommended_focus": "Review generated validation leads and promote only cited, source-backed follow-ups.",
        "next_cadence_focus": "Re-run graph brief after source refresh and human review labels.",
        "capability_gaps": "\n".join(context.get("guardrails", [])),
        "total_count": sum(len(value) for value in rows.values() if isinstance(value, list)),
        "product_action_count": count_rows_with_value(rows.get("work_program_items", []), "decision_state", "product_action"),
        "validation_lead_count": count_rows_with_value(rows.get("work_program_items", []), "decision_state", "validation_lead"),
        "source_coverage_limited_count": count_rows_containing(rows.get("work_program_items", []), "source_coverage_state", "limited"),
        "active_blocker_count": 0,
        "active_blocker_impact_count": 0,
        "needs_action_dependency_count": len(rows.get("work_dependency_edges", [])) if isinstance(rows.get("work_dependency_edges"), list) else 0,
        "overloaded_owner_count": metric_int(analytics.get("measurement_readiness", {}), "overloaded_owner_count"),
        "unassigned_action_count": count_rows_with_blank(rows.get("work_actions", []), "owner_key"),
        "quality_gate_count": len(rows.get("quality_gates", [])) if isinstance(rows.get("quality_gates"), list) else 0,
        "blocking_gate_count": count_rows_truthy(rows.get("quality_gates", []), "blocking"),
        "caveat_count": len(context.get("guardrails", [])),
        "risk_driver_count": evaluation.get("material_claim_line_count", 0),
        "source_system": "cubicle_ai",
        "source_instance": source_instance,
        "external_kind": "ai_graph_brief_snapshot",
        "external_id": external_id,
        "source_url": f"cubicle://graph-brief/{prompt_mode}/{context_hash}",
        "latest_evidence_id": evidence_id,
        "evidence_count": 1,
        "freshness_state": "fresh",
        "visibility": "private",
        "confidence": 0.72,
        "event_count": evaluation.get("material_claim_line_count", 0),
        "first_seen_at": generated_at,
        "last_activity_at": generated_at,
        "rank_score": 100.0,
        "created_at": generated_at,
        "updated_at": generated_at,
    }
    upsert_by_key(conn, "work_program_brief_snapshots", values)
    snapshot_id = row_id_by_key(conn, "work_program_brief_snapshots", snapshot_key)
    run_member = persist_run_member_for_snapshot(conn, run, snapshot_id, values, generated_at)
    return {
        "snapshot_id": snapshot_id,
        "snapshot_key": snapshot_key,
        "external_id": external_id,
        "generated_at": snapshot_generated_at,
        "run_member": run_member,
    }


def persist_run_member_for_snapshot(
    conn: sqlite3.Connection,
    run: sqlite3.Row | None,
    snapshot_id: int,
    snapshot_values: dict[str, Any],
    generated_at: str,
) -> dict[str, Any] | None:
    if run is None or not table_exists(conn, "work_program_run_members"):
        return None
    run_key = str(run["run_key"])
    columns = [
        "run_key",
        "member_table",
        "member_id",
        "member_key",
        "member_external_kind",
        "member_external_id",
        "member_rank_score",
        "created_at",
    ]
    values: list[Any] = [
        run_key,
        "work_program_brief_snapshots",
        snapshot_id,
        snapshot_values["key"],
        snapshot_values["external_kind"],
        snapshot_values["external_id"],
        snapshot_values["rank_score"],
        generated_at,
    ]
    if column_exists(conn, "work_program_run_members", "work_program_run_id"):
        columns.insert(0, "work_program_run_id")
        values.insert(0, int(run["id"]))
    assignments = [f"{column} = excluded.{column}" for column in columns if column not in {"run_key", "member_table", "member_id"}]
    conn.execute(
        f"""
        insert into work_program_run_members ({", ".join(columns)})
        values ({", ".join("?" for _ in columns)})
        on conflict(run_key, member_table, member_id) do update set {", ".join(assignments)}
        """,
        values,
    )
    refresh_run_member_counts(conn, run_key)
    return {"run_key": run_key, "member_table": "work_program_brief_snapshots", "member_id": snapshot_id}


def row_value(row: sqlite3.Row | None, key: str) -> Any:
    if row is None:
        return None
    return row[key] if key in row.keys() else None


def upsert_by_key(conn: sqlite3.Connection, table: str, values: dict[str, Any]) -> None:
    columns = [column for column in table_columns(conn, table) if column in values]
    if "key" not in columns:
        raise RuntimeError(f"{table} must have a key column for AI graph brief persistence")
    assignments = [f"{column} = excluded.{column}" for column in columns if column != "key"]
    conn.execute(
        f"""
        insert into {table} ({", ".join(columns)})
        values ({", ".join("?" for _ in columns)})
        on conflict(key) do update set {", ".join(assignments)}
        """,
        [values[column] for column in columns],
    )


def normalized_graph_brief_prompt_mode(value: str | None) -> str:
    mode = str(value or "operating").strip().lower()
    if mode not in GRAPH_BRIEF_PROMPT_MODES:
        raise ValueError(f"unsupported graph brief prompt mode: {value!r}")
    return mode


def graph_brief_model_method(prompt_mode: str) -> str:
    return f"{GRAPH_BRIEF_MODEL_METHOD}:{normalized_graph_brief_prompt_mode(prompt_mode)}"


def supersede_prior_ai_graph_briefs(
    conn: sqlite3.Connection,
    source_instance: str,
    subject_key: str,
    current_id: int,
    updated_at: str,
    *,
    prompt_mode: str = "operating",
) -> None:
    if not table_exists(conn, "work_insights"):
        return
    required = {"producer_state", "source_instance", "subject_key", "insight_kind", "updated_at", "id", "model_method", "external_id", "source_url"}
    if not required.issubset(set(table_columns(conn, "work_insights"))):
        return
    prompt_mode = normalized_graph_brief_prompt_mode(prompt_mode)
    if prompt_mode == "operating":
        mode_filter = f"""
           and (
             model_method = ?
             or external_id like ?
             or source_url like ?
             or (
               (model_method is null or model_method = '' or model_method = ?)
               and (external_id is null or external_id not like '%|generic|%')
               and (source_url is null or source_url not like '%/generic/%')
             )
           )
        """
        params = [
            graph_brief_model_method(prompt_mode),
            "%|operating|%",
            "%/operating/%",
            GRAPH_BRIEF_MODEL_METHOD,
        ]
    else:
        mode_filter = """
           and (
             model_method = ?
             or external_id like ?
             or source_url like ?
           )
        """
        params = [
            graph_brief_model_method(prompt_mode),
            f"%|{prompt_mode}|%",
            f"%/{prompt_mode}/%",
        ]
    conn.execute(
        f"""
        update work_insights
           set producer_state = 'superseded', updated_at = ?
         where insight_kind = 'ai_graph_brief'
           and source_instance = ?
           and subject_key = ?
           and id != ?
           and producer_state = 'current'
        {mode_filter}
        """,
        [updated_at, source_instance, subject_key, current_id, *params],
    )


def row_id_by_key(conn: sqlite3.Connection, table: str, key: str) -> int:
    row = conn.execute(f"select id from {table} where key = ?", (key,)).fetchone()
    if row is None:
        raise RuntimeError(f"{table} row was not persisted for key {key}")
    return int(row["id"] if isinstance(row, sqlite3.Row) else row[0])


def truncate_evidence_excerpt(text: str) -> str:
    cleaned = one_line(text)
    if len(cleaned) <= 2000:
        return cleaned
    return cleaned[:1997].rstrip() + "..."


def model_name_from_command(command: str | None) -> str:
    if not command:
        return "external_llm"
    parts = shlex.split(command)
    for index, part in enumerate(parts):
        if part == "--model" and index + 1 < len(parts):
            return parts[index + 1]
    return parts[0] if parts else "external_llm"


def latest_work_program_run(conn: sqlite3.Connection, source_instance: str, workstream_key: str) -> sqlite3.Row | None:
    if not table_exists(conn, "work_program_runs"):
        return None
    keys = workstream_sql_keys(workstream_key)
    source_clause, source_params = source_predicate(None, source_instance)
    return conn.execute(
        f"""
        select id, run_key, workstream_key, generated_at
          from work_program_runs
         where workstream_key in ({placeholders(keys)})
           {source_clause}
         order by generated_at desc
         limit 1
        """,
        [*keys, *source_params],
    ).fetchone()


def workstream_id_for_key(conn: sqlite3.Connection, workstream_key: str) -> int | None:
    if not table_exists(conn, "workstreams"):
        return None
    keys = workstream_sql_keys(workstream_key)
    row = conn.execute(f"select id from workstreams where key in ({placeholders(keys)}) order by id limit 1", keys).fetchone()
    if row is None:
        return None
    return int(row["id"] if isinstance(row, sqlite3.Row) else row[0])


def refresh_run_member_counts(conn: sqlite3.Connection, run_key: str) -> None:
    if not table_exists(conn, "work_program_runs") or not table_exists(conn, "work_program_run_members"):
        return
    member_count = int(conn.execute("select count(*) from work_program_run_members where run_key = ?", [run_key]).fetchone()[0] or 0)
    brief_count = int(
        conn.execute(
            "select count(*) from work_program_run_members where run_key = ? and member_table = 'work_program_brief_snapshots'",
            [run_key],
        ).fetchone()[0]
        or 0
    )
    updates = ["member_count = ?"]
    values: list[Any] = [member_count]
    if column_exists(conn, "work_program_runs", "brief_snapshot_count"):
        updates.append("brief_snapshot_count = ?")
        values.append(brief_count)
    values.append(run_key)
    conn.execute(f"update work_program_runs set {', '.join(updates)} where run_key = ?", values)


def forecast_readiness_state(analytics: dict[str, Any]) -> str:
    forecast_summary = analytics.get("forecast_summary", {})
    state = forecast_summary.get("eta_readiness_state", {}).get("value") if isinstance(forecast_summary, dict) else None
    if state:
        return str(state)
    ready = forecast_summary.get("eta_forecast_ready", {}).get("value") if isinstance(forecast_summary, dict) else None
    if str(ready).lower() == "true":
        return "ready"
    if ready is not None:
        return "gated"
    return "unknown"


def primary_guardrail(context: dict[str, Any]) -> str | None:
    guardrails = context.get("guardrails", [])
    if not guardrails:
        return None
    return str(guardrails[0])


def first_brief_bullet(answer_text: str) -> str | None:
    for line in (answer_text or "").splitlines():
        stripped = line.strip()
        if stripped.startswith("- "):
            return stripped[2:].strip()
    return None


def count_rows_with_value(rows: Any, field: str, value: str) -> int:
    if not isinstance(rows, list):
        return 0
    return sum(1 for row in rows if isinstance(row, dict) and str(row.get(field) or "") == value)


def count_rows_containing(rows: Any, field: str, text: str) -> int:
    if not isinstance(rows, list):
        return 0
    needle = text.lower()
    return sum(1 for row in rows if isinstance(row, dict) and needle in str(row.get(field) or "").lower())


def count_rows_with_blank(rows: Any, field: str) -> int:
    if not isinstance(rows, list):
        return 0
    return sum(1 for row in rows if isinstance(row, dict) and not str(row.get(field) or "").strip())


def count_rows_truthy(rows: Any, field: str) -> int:
    if not isinstance(rows, list):
        return 0
    return sum(1 for row in rows if isinstance(row, dict) and truthy(row.get(field)))


def truthy(value: Any) -> bool:
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return value != 0
    return str(value or "").strip().lower() in {"1", "true", "yes", "y"}


def metric_int(metrics: Any, key: str) -> int:
    if not isinstance(metrics, dict):
        return 0
    value = metrics.get(key, {})
    if isinstance(value, dict):
        value = value.get("value")
    try:
        return int(float(value))
    except (TypeError, ValueError):
        return 0


def allowed_citations(context: dict[str, Any]) -> set[str]:
    tokens = {
        citation("context", str(context.get("context_hash", ""))),
        citation("guardrail", str(context.get("context_hash", ""))),
    }
    if not is_bounded_graph_context(context):
        tokens.update(
            {
                citation("analytics", "tpm_forecast_summary"),
                citation("analytics", "tpm_evaluation_readiness"),
                citation("analytics", "tpm_blocker_candidates"),
            }
        )
    for token in context.get("allowed_citations", []):
        if isinstance(token, str):
            tokens.add(token)
    for row in context.get("citations", []):
        if isinstance(row, dict) and str(row.get("ref") or "").strip():
            tokens.add(str(row["ref"]))
    if not has_structured_citation_policy(context):
        for rows in context.get("rows", {}).values():
            if not isinstance(rows, list):
                continue
            for row in rows:
                if isinstance(row, dict):
                    tokens.add(row_citation(row))
    return {token for token in tokens if token != "[]"}


def has_structured_citation_policy(context: dict[str, Any]) -> bool:
    return bool(context.get("citations") or context.get("allowed_citations"))


def is_bounded_graph_context(context: dict[str, Any]) -> bool:
    return isinstance(context.get("bounded_graph_context"), dict)


def citation_policy_by_ref(context: dict[str, Any]) -> dict[str, dict[str, Any]]:
    policy: dict[str, dict[str, Any]] = {}
    for row in context.get("citations", []):
        if isinstance(row, dict):
            ref = str(row.get("ref") or "").strip()
            if ref:
                policy[ref] = row
    return policy


def citation_can_support_confirmed_fact(context: dict[str, Any], token: str) -> bool:
    context_hash = str(context.get("context_hash", ""))
    confirmed_shortcuts = {citation("context", context_hash)}
    if not is_bounded_graph_context(context):
        confirmed_shortcuts.update(
            {
                citation("analytics", "tpm_forecast_summary"),
                citation("analytics", "tpm_evaluation_readiness"),
                citation("analytics", "tpm_blocker_candidates"),
            }
        )
    policy = citation_policy_by_ref(context).get(token)
    if policy is not None:
        return truthy(policy.get("claimAllowed", policy.get("claim_allowed")))
    if token in confirmed_shortcuts:
        return True
    if has_structured_citation_policy(context):
        return False
    return token in allowed_citations(context)


def citation_policy_violations(context: dict[str, Any], answer_text: str) -> list[dict[str, Any]]:
    violations: list[dict[str, Any]] = []
    sections = extract_brief_sections(answer_text)
    for line in material_claim_lines(answer_text):
        disconnected = [token for token in extract_citations(line) if citation_is_disconnected_from_seed_component(context, token)]
        if disconnected:
            violations.append({"kind": "disconnected_seed_component_citation_not_allowed", "citations": disconnected, "line": line})
    for bullet in sections.get("## Confirmed Facts", []):
        blocked = [token for token in extract_citations(bullet) if not citation_can_support_confirmed_fact(context, token)]
        if blocked:
            violations.append({"kind": "confirmed_fact_requires_claim_allowed_citation", "citations": blocked, "line": bullet})
        if confirmed_relationship_claim_requires_association_citation(context, bullet):
            violations.append(
                {
                    "kind": "confirmed_relationship_requires_claim_allowed_association_citation",
                    "citations": extract_citations(bullet),
                    "line": bullet,
                }
            )
        if confirmed_absence_claim_requires_allowed_source_coverage(context, bullet):
            violations.append(
                {
                    "kind": "absence_claim_requires_allowed_source_coverage",
                    "citations": extract_citations(bullet),
                    "line": bullet,
                }
            )
        if confirmed_product_claim_requires_product_citation(context, bullet):
            violations.append(
                {
                    "kind": "confirmed_product_claim_requires_product_citation",
                    "citations": extract_citations(bullet),
                    "line": bullet,
                }
            )
    if has_structured_citation_policy(context):
        for line in material_claim_lines(answer_text):
            if not contains_source_url(line):
                continue
            citations = extract_citations(line)
            if not citations or not any(citation_can_expose_source_url(context, token) for token in citations):
                violations.append({"kind": "source_url_requires_allowed_citation", "citations": citations, "line": line})
    return violations


def statement_support_audit(context: dict[str, Any], answer_text: str) -> dict[str, Any]:
    rows: list[dict[str, Any]] = []
    allowed = allowed_citations(context)
    for record in material_claim_line_records(answer_text):
        line = record["line"]
        section = record["section"]
        citations = extract_citations(line)
        unknown = sorted({token for token in citations if token not in allowed})
        reasons = statement_support_failures(context, section, line, citations, unknown)
        if not citations:
            status = "uncited"
        elif unknown:
            status = "unknown_citation"
        elif reasons:
            status = "unsupported"
        elif section == "## Confirmed Facts":
            status = "supported_confirmed_fact"
        elif section == "## What Not To Claim":
            status = "supported_guardrail"
        elif section == "## Validation Leads":
            status = "supported_validation_context"
        else:
            status = "supported"
        rows.append(
            {
                "section": section,
                "line": line,
                "citations": citations,
                "unknown_citations": unknown,
                "support_status": status,
                "support_failures": reasons,
                "citation_claim_uses": statement_citation_claim_uses(context, citations),
            }
        )
    unsupported_statuses = {"uncited", "unknown_citation", "unsupported"}
    unsupported = [row for row in rows if row["support_status"] in unsupported_statuses]
    return {
        "statement_count": len(rows),
        "supported_statement_count": len(rows) - len(unsupported),
        "unsupported_statement_count": len(unsupported),
        "rows": rows,
    }


def statement_support_failures(context: dict[str, Any], section: str, line: str, citations: list[str], unknown: list[str]) -> list[dict[str, Any]]:
    failures: list[dict[str, Any]] = []
    if not citations:
        failures.append({"kind": "uncited_material_statement"})
        return failures
    if unknown:
        failures.append({"kind": "unknown_citation", "citations": unknown})
        return failures
    semantic = semantic_guardrail_violations(context, line)
    for violation in semantic:
        failures.append({"kind": violation.get("kind", "semantic_guardrail_violation")})
    if disconnected_seed_component_mentions(context, line):
        failures.append({"kind": "disconnected_seed_component_mention"})
    disconnected = [token for token in citations if citation_is_disconnected_from_seed_component(context, token)]
    if disconnected:
        failures.append({"kind": "disconnected_seed_component_citation_not_allowed", "citations": disconnected})
    if contains_source_url(line) and not any(citation_can_expose_source_url(context, token) for token in citations):
        failures.append({"kind": "source_url_requires_allowed_citation", "citations": citations})
    if section == "## Confirmed Facts":
        blocked = [token for token in citations if not citation_can_support_confirmed_fact(context, token)]
        if blocked:
            failures.append({"kind": "confirmed_fact_requires_claim_allowed_citation", "citations": blocked})
        if confirmed_relationship_claim_requires_association_citation(context, line):
            failures.append({"kind": "confirmed_relationship_requires_claim_allowed_association_citation", "citations": citations})
        if confirmed_absence_claim_requires_allowed_source_coverage(context, line):
            failures.append({"kind": "absence_claim_requires_allowed_source_coverage", "citations": citations})
        if confirmed_product_claim_requires_product_citation(context, line):
            failures.append({"kind": "confirmed_product_claim_requires_product_citation", "citations": citations})
    return failures


def statement_citation_claim_uses(context: dict[str, Any], citations: list[str]) -> list[dict[str, Any]]:
    policy = citation_policy_by_ref(context)
    out: list[dict[str, Any]] = []
    for token in citations:
        row = policy.get(token, {})
        out.append(
            {
                "ref": token,
                "claim_allowed": bool(row.get("claimAllowed", row.get("claim_allowed", citation_can_support_confirmed_fact(context, token)))),
                "claim_use": row.get("claimUse") or row.get("claim_use") or inferred_claim_use(context, token),
                "node_kind": row.get("nodeKind") or row.get("node_kind") or "",
                "association_type": row.get("associationType") or row.get("association_type") or "",
            }
        )
    return out


def inferred_claim_use(context: dict[str, Any], token: str) -> str:
    if token.startswith("[guardrail:"):
        return "guardrail"
    if token.startswith("[source_coverage:"):
        return "source_coverage_gate"
    if token.startswith("[context:"):
        return "context_boundary"
    if token.startswith("[analytics:"):
        return "analytics_summary"
    if token.startswith("[graph_associations:"):
        return "typed_association" if citation_can_support_confirmed_fact(context, token) else "validation_lead"
    if token.startswith("[graph_objects:"):
        return "typed_object" if citation_can_support_confirmed_fact(context, token) else "validation_lead"
    return "unknown"


def citation_is_disconnected_from_seed_component(context: dict[str, Any], token: str) -> bool:
    policy = citation_policy_by_ref(context).get(token)
    if policy is None:
        return False
    gate_reason = str(policy.get("claimGateReason") or policy.get("claim_gate_reason") or "").lower()
    return gate_reason == "disconnected_from_seed_component"


def confirmed_absence_claim_requires_allowed_source_coverage(context: dict[str, Any], text: str) -> bool:
    if not confirmed_absence_claim_present(text):
        return False
    return not any(citation_can_support_absence_claim(context, token) for token in extract_citations(text))


def confirmed_absence_claim_present(text: str) -> bool:
    normalized = " ".join((text or "").lower().split())
    if explicit_prohibition_line(normalized):
        return False
    absence_patterns = [
        r"\bthere (are|is) no (linked )?(pull requests|prs|reviews|blockers|owners|linked objects|linked tickets)\b",
        r"\bno (linked )?(pull requests|prs|reviews|blockers|owners|linked objects|linked tickets)\b",
        r"\bdoes not have (any )?(linked )?(pull requests|prs|reviews|blockers|owners|linked tickets)\b",
        r"\bhas no (linked )?(pull requests|prs|reviews|blockers|owners|linked tickets)\b",
    ]
    return any(re.search(pattern, normalized) for pattern in absence_patterns)


def citation_can_support_absence_claim(context: dict[str, Any], token: str) -> bool:
    if not absence_claims_allowed(context):
        return False
    return citation_is_source_coverage(context, token)


def absence_claims_allowed(context: dict[str, Any]) -> bool:
    source_coverage = context.get("analytics", {}).get("source_coverage", {})
    if isinstance(source_coverage, dict) and "absence_claims_allowed" in source_coverage:
        value = metric_value(source_coverage, "absence_claims_allowed", "false")
        return truthy(value)
    bounded_context = context.get("bounded_graph_context", {})
    if isinstance(bounded_context, dict):
        coverage = bounded_context.get("coverage") or {}
        if isinstance(coverage, dict) and ("absenceClaimsAllowed" in coverage or "absence_claims_allowed" in coverage):
            return bool(coverage.get("absenceClaimsAllowed", coverage.get("absence_claims_allowed")))
    graph_context = context.get("graph_context", {})
    if isinstance(graph_context, dict):
        source_packet = graph_context.get("sourceCoveragePacket") or {}
        if isinstance(source_packet, dict) and "absenceClaimsAllowed" in source_packet:
            return bool(source_packet.get("absenceClaimsAllowed"))
    value = metric_value(source_coverage, "absence_claims_allowed", "false")
    return truthy(value)


def citation_is_source_coverage(context: dict[str, Any], token: str) -> bool:
    if token.startswith("[source_coverage:"):
        return True
    policy = citation_policy_by_ref(context).get(token)
    if policy is None:
        return False
    citation_kind = str(policy.get("citationKind") or policy.get("citation_kind") or "").lower()
    node_kind = str(policy.get("nodeKind") or policy.get("node_kind") or "").lower()
    claim_use = str(policy.get("claimUse") or policy.get("claim_use") or "").lower()
    return citation_kind == "source_coverage" or node_kind.endswith("source_coverage_packet") or claim_use == "source_coverage_gate"


def confirmed_product_claim_requires_product_citation(context: dict[str, Any], text: str) -> bool:
    if not confirmed_product_claim_present(text):
        return False
    return not any(citation_can_support_product_claim(context, token) for token in extract_citations(text))


def confirmed_product_claim_present(text: str) -> bool:
    normalized = " ".join((text or "").lower().split())
    if explicit_prohibition_line(normalized):
        return False
    product_patterns = [
        r"\breviewer(s)? approved\b",
        r"\breview approved\b",
        r"\bapproved by reviewer(s)?\b",
        r"\bapproved the pr\b",
        r"\bapproved the pull request\b",
        r"\blinked (pull request|pr) exists\b",
        r"\bhas (a )?linked (pull request|pr)\b",
        r"\bready for product action\b",
    ]
    return any(re.search(pattern, normalized) for pattern in product_patterns)


def citation_can_support_product_claim(context: dict[str, Any], token: str) -> bool:
    if not citation_can_support_confirmed_fact(context, token):
        return False
    if citation_is_boundary_or_analytics(context, token):
        return False
    return True


def citation_is_boundary_or_analytics(context: dict[str, Any], token: str) -> bool:
    if token.startswith(("[context:", "[guardrail:", "[source_coverage:", "[analytics:")):
        return True
    policy = citation_policy_by_ref(context).get(token)
    if policy is None:
        return False
    citation_kind = str(policy.get("citationKind") or policy.get("citation_kind") or "").lower()
    node_kind = str(policy.get("nodeKind") or policy.get("node_kind") or "").lower()
    claim_use = str(policy.get("claimUse") or policy.get("claim_use") or "").lower()
    return (
        citation_kind in {"graph_context", "guardrail", "source_coverage", "analytics_summary", "derived_packet"}
        or node_kind in {"work_program_graph_context", "bounded_graph_context", "bounded_graph_guardrail", "bounded_graph_source_coverage"}
        or node_kind.endswith("source_coverage_packet")
        or claim_use in {"context_boundary", "guardrail", "source_coverage_gate"}
    )


def confirmed_relationship_claim_requires_association_citation(context: dict[str, Any], text: str) -> bool:
    if not has_structured_citation_policy(context):
        return False
    normalized = " ".join((text or "").lower().split())
    relationship_patterns = [
        r"\bassociated with\b",
        r"\blinked to\b",
        r"\bvia (an? )?[a-z0-9_ -]*relationship\b",
        r"\bpossible follow[- ]up\b",
        r"\bimplemented by\b",
        r"\bdocumented by\b",
        r"\bdiscussed in\b",
        r"\bblocked by\b",
        r"\bowned by\b",
        r"\bis present as `?[a-z0-9_ -]+`?\b",
    ]
    if not any(re.search(pattern, normalized) for pattern in relationship_patterns):
        return False
    citations = extract_citations(text)
    claimed_kinds = claimed_relationship_kinds(normalized)
    if claimed_kinds:
        return not all(
            any(citation_matches_relationship_kind(context, token, kind) for token in citations)
            for kind in claimed_kinds
        )
    return not any(citation_is_claim_allowed_graph_association(context, token) for token in citations)


def claimed_relationship_kinds(normalized_text: str) -> set[str]:
    kind_patterns = {
        "implemented_by": [
            r"\bimplemented[_ -]by\b",
            r"\bimplemented by\b",
            r"\bimplements\b",
            r"\bimplementation\b",
        ],
        "documented_by": [
            r"\bdocumented[_ -]by\b",
            r"\bdocumented by\b",
            r"\bdocuments\b",
            r"\bdocumentation\b",
        ],
        "discussed_in": [
            r"\bdiscussed[_ -]in\b",
            r"\bdiscussed in\b",
            r"\bdiscussion relationship\b",
        ],
        "possible_followup_for": [
            r"\bpossible[_ -]follow[-_ ]?up[_ -]for\b",
            r"\bpossible follow[- ]up\b",
            r"\bfollow[- ]up\b",
        ],
        "mentions": [
            r"\bmentions\b",
            r"\bmentioned in\b",
        ],
        "assignee": [
            r"\bassignee\b",
            r"\bassigned to\b",
            r"\bowned by\b",
        ],
        "blocked_by": [
            r"\bblocked[_ -]by\b",
            r"\bblocked by\b",
        ],
    }
    out: set[str] = set()
    for kind, patterns in kind_patterns.items():
        if any(re.search(pattern, normalized_text) for pattern in patterns):
            out.add(kind)
    return out


def citation_matches_relationship_kind(context: dict[str, Any], token: str, relationship_kind: str) -> bool:
    if not citation_is_claim_allowed_graph_association(context, token):
        return False
    return citation_association_type(context, token) == relationship_kind


def citation_association_type(context: dict[str, Any], token: str) -> str:
    policy = citation_policy_by_ref(context).get(token)
    if policy is not None:
        value = policy.get("associationType", policy.get("association_type"))
        if value:
            return str(value)
        node_key = str(policy.get("nodeKey") or policy.get("node_key") or "")
        kind = association_type_from_key(node_key)
        if kind:
            return kind
    if token.startswith("[graph_associations:") and token.endswith("]"):
        kind = association_type_from_key(token[len("[graph_associations:") : -1])
        if kind:
            return kind
    return ""


def association_type_from_key(key: str) -> str:
    known_types = [
        "possible_followup_for",
        "implemented_by",
        "documented_by",
        "discussed_in",
        "blocked_by",
        "assignee",
        "mentions",
        "links_to",
        "related_to",
    ]
    normalized = str(key or "").lower()
    for known in known_types:
        if known in normalized:
            return known
    parts = normalized.split(":")
    if len(parts) >= 4 and parts[0] in {"association", "assoc"}:
        return parts[2]
    return ""


def citation_is_claim_allowed_graph_association(context: dict[str, Any], token: str) -> bool:
    policy = citation_policy_by_ref(context).get(token)
    if policy is not None:
        citation_kind = str(policy.get("citationKind") or policy.get("citation_kind") or "")
        node_kind = str(policy.get("nodeKind") or policy.get("node_kind") or "")
        is_association = citation_kind == "typed_graph_association" or node_kind == "graph_association"
        return is_association and truthy(policy.get("claimAllowed", policy.get("claim_allowed")))
    if has_structured_citation_policy(context):
        return False
    return token.startswith("[graph_associations:")


def citation_can_expose_source_url(context: dict[str, Any], token: str) -> bool:
    policy = citation_policy_by_ref(context).get(token)
    if policy is None:
        return False
    return truthy(policy.get("sourceUrlAllowed", policy.get("source_url_allowed")))


def contains_source_url(text: str) -> bool:
    return bool(re.search(r"https?://\S+", text or ""))


def extract_citations(text: str) -> list[str]:
    return re.findall(r"\[[^\[\]\n]+:[^\[\]\n]+\]", text or "")


def material_claim_lines(text: str) -> list[str]:
    return [row["line"] for row in material_claim_line_records(text)]


def material_claim_line_records(text: str) -> list[dict[str, str]]:
    lines: list[dict[str, str]] = []
    current_section = "outside_required_sections"
    for raw_line in (text or "").splitlines():
        line = raw_line.strip()
        if line in REQUIRED_BRIEF_SECTIONS[1:]:
            current_section = line
            continue
        if not line or line.startswith("#") or line.startswith("```"):
            continue
        if line.lower() in {"confirmed facts", "validation leads", "what not to claim", "next actions"}:
            continue
        if len(line) < 18:
            continue
        lines.append({"section": current_section, "line": line})
    return lines


def forbidden_claim_violations(context: dict[str, Any], answer_text: str) -> list[dict[str, str]]:
    violations: list[dict[str, str]] = []
    if not eta_ready(context):
        eta_patterns = [
            r"\bwill (merge|ship|finish|close|complete) by\b",
            r"\beta (date|commitment|deadline) is\b",
            r"\bexpected to (merge|ship|finish|close|complete) (on|by)\b",
            r"\bshould (merge|ship|finish|close|complete) by\b",
        ]
        violations.extend(pattern_violations(answer_text, "eta_not_ready", eta_patterns))
    if blocker_candidates_need_validation(context):
        blocker_patterns = [
            r"\bconfirmed blocker\b",
            r"\bconfirmed blockers\b",
            r"\bis blocked by\b",
            r"\bare blocked by\b",
        ]
        violations.extend(pattern_violations(answer_text, "blockers_not_confirmed", blocker_patterns))
    if product_claims_are_gated(context):
        product_patterns = [
            r"\bmeasured precision is good\b",
            r"\bprecision is proven\b",
            r"\bready for product action\b",
        ]
        violations.extend(pattern_violations(answer_text, "product_claims_gated", product_patterns))
    return violations


def semantic_guardrail_violations(context: dict[str, Any], answer_text: str) -> list[dict[str, str]]:
    violations: list[dict[str, str]] = []
    for line in material_claim_lines(answer_text):
        mentions = disconnected_seed_component_mentions(context, line)
        if mentions:
            violations.append(
                {
                    "guardrail": "disconnected_seed_component_mentioned",
                    "pattern": mentions[0],
                    "line": line,
                }
            )
    if source_coverage_state(context).lower() == "complete":
        coverage_complete_contradictions = [
            r"\bdo not claim [^\n.]*source coverage (is |as )?complete\b",
            r"\bsource coverage (is|remains) (not complete|incomplete|sparse|auth[- ]limited|limited|unknown)\b",
            r"\bcoverage (is|remains) (not complete|incomplete|sparse|auth[- ]limited|limited|unknown)\b",
        ]
        for line in material_claim_lines(answer_text):
            normalized = " ".join(line.lower().split())
            for pattern in coverage_complete_contradictions:
                if re.search(pattern, normalized):
                    violations.append({"guardrail": "source_coverage_complete_contradicted", "pattern": pattern, "line": line})
    return violations


def disconnected_seed_component_mentions(context: dict[str, Any], line: str) -> list[str]:
    if not is_bounded_graph_context(context):
        return []
    normalized = normalized_for_match(line)
    if not normalized:
        return []
    return [term for term in disconnected_seed_component_terms(context) if term in normalized]


def disconnected_seed_component_terms(context: dict[str, Any]) -> list[str]:
    terms: set[str] = set()
    rows = context.get("rows", {})
    for table in ["graph_objects", "graph_associations"]:
        for row in list_rows(rows, table):
            if row.get("seed_reachable") is not False and row.get("claim_gate_reason") != "disconnected_from_seed_component":
                continue
            candidate_values = [
                row.get("key"),
                row.get("from_key"),
                row.get("to_key"),
                row.get("title"),
            ]
            for value in candidate_values:
                add_disconnected_seed_component_terms(terms, value)
    return sorted(terms, key=lambda term: (-len(term), term))


def add_disconnected_seed_component_terms(terms: set[str], value: Any) -> None:
    raw = str(value or "").strip()
    if not raw:
        return
    normalized = normalized_for_match(raw)
    if len(normalized) >= 4:
        terms.add(normalized)
    for separator in [":", "/", "|"]:
        if separator in raw:
            tail = raw.rsplit(separator, 1)[-1].strip()
            normalized_tail = normalized_for_match(tail)
            if len(normalized_tail) >= 4:
                terms.add(normalized_tail)
    for match in re.findall(r"[A-Z]+-\d+|#\d+", raw):
        if len(match) >= 3:
            terms.add(normalized_for_match(match))


def source_coverage_state(context: dict[str, Any]) -> str:
    graph_context = context.get("graph_context", {})
    if isinstance(graph_context, dict):
        source_packet = graph_context.get("sourceCoveragePacket") or {}
        if isinstance(source_packet, dict):
            state = str(source_packet.get("coverageState") or "").strip()
            if state:
                return state
    source_coverage = context.get("analytics", {}).get("source_coverage", {})
    return metric_value(source_coverage, "coverage_state", "unknown")


def pattern_violations(answer_text: str, guardrail: str, patterns: list[str]) -> list[dict[str, str]]:
    violations: list[dict[str, str]] = []
    for line in material_claim_lines(answer_text):
        normalized = " ".join(line.lower().split())
        if explicit_prohibition_line(normalized):
            continue
        for pattern in patterns:
            if regex_pattern_present_without_negation(normalized, pattern):
                violations.append({"guardrail": guardrail, "pattern": pattern, "line": line})
    return violations


def regex_pattern_present_without_negation(normalized_line: str, pattern: str) -> bool:
    for match in re.finditer(pattern, normalized_line):
        prefix = normalized_line[max(0, match.start() - 32) : match.start()]
        if not any(marker in prefix for marker in ["not ", "no ", "without ", "avoid ", "never "]):
            return True
    return False


def explicit_prohibition_line(line: str) -> bool:
    return any(phrase in line for phrase in ["do not", "don't", "not claim", "not an ", "not a "])


def eta_ready(context: dict[str, Any]) -> bool:
    graph_context = context.get("graph_context", {})
    if isinstance(graph_context, dict):
        forecast_packet = graph_context.get("forecastPacket") or {}
        if isinstance(forecast_packet, dict) and "etaForecastReady" in forecast_packet:
            return bool(forecast_packet.get("etaForecastReady"))
    forecast_summary = context.get("analytics", {}).get("forecast_summary", {})
    return forecast_summary.get("eta_forecast_ready", {}).get("value", "").lower() == "true"


def blocker_candidates_need_validation(context: dict[str, Any]) -> bool:
    return int(context.get("analytics", {}).get("blocker_candidate_count", 0) or 0) > 0


def product_claims_are_gated(context: dict[str, Any]) -> bool:
    graph_context = context.get("graph_context", {})
    if isinstance(graph_context, dict):
        guardrail_packet = graph_context.get("guardrailPacket") or {}
        source_packet = graph_context.get("sourceCoveragePacket") or {}
        forecast_packet = graph_context.get("forecastPacket") or {}
        if guardrail_packet.get("humanReviewRequired") or source_packet.get("absenceClaimsAllowed") is False or forecast_packet.get("etaForecastReady") is False:
            return True
    analytics = context.get("analytics", {})
    forecast_summary = analytics.get("forecast_summary", {})
    measurement = analytics.get("measurement_readiness", {})
    eta_ready = forecast_summary.get("eta_forecast_ready", {}).get("value", "").lower() == "true"
    precision_ready = measurement.get("ready_to_measure_precision", {}).get("value", "").lower() == "true"
    actionability_ready = measurement.get("ready_to_measure_actionability", {}).get("value", "").lower() == "true"
    return not (eta_ready and precision_ready and actionability_ready)


def item_decision_use(item: dict[str, Any], product_claims_gated: bool) -> str:
    decision = str(item.get("decision_state") or "unknown")
    if product_claims_gated and decision == "product_action":
        return "product_action(raw); safe_use=owner/status follow-up"
    return decision


def action_decision_use(action: dict[str, Any], product_claims_gated: bool) -> str:
    decision = str(action.get("decision_state") or "unknown")
    if product_claims_gated and decision == "product_action":
        return "product_action(raw); safe_use=gated follow-up"
    return decision


def llm_task() -> str:
    return (
        "Given the JSON context bundle, write a concise operating brief with citations. "
        "Use only cited rows. Separate confirmed facts from validation leads. Include "
        "'what not to claim' when source coverage, measurement, or forecast gates are not ready."
    )


def generic_llm_task() -> str:
    return (
        "Given the JSON context bundle, write a concise graph-safety brief with citations. "
        "Explain the bounded traversal shape, cite claimable graph rows as confirmed facts, "
        "treat derived topology and generated rows as validation context, and preserve the "
        "guardrails that prevent product, absence, generated-summary, or source-truth overclaims."
    )


def generic_bounded_prompt_summary(context: dict[str, Any]) -> dict[str, Any]:
    objects = list_rows(context.get("rows", {}), "graph_objects")
    associations = list_rows(context.get("rows", {}), "graph_associations")
    claimable_objects = [row for row in objects if truthy(row.get("claim_allowed"))]
    claimable_associations = [row for row in associations if truthy(row.get("claim_allowed"))]
    gated_objects = len(objects) - len(claimable_objects)
    gated_associations = len(associations) - len(claimable_associations)
    if claimable_associations:
        confirmed_instruction = "Confirmed Facts may cite claimable association rows; gated association rows belong in Validation Leads."
    else:
        confirmed_instruction = "No claimable association rows are selected; keep association relationships out of Confirmed Facts and put them in Validation Leads."
    return {
        "object_count": len(objects),
        "association_count": len(associations),
        "claimable_object_count": len(claimable_objects),
        "gated_object_count": gated_objects,
        "claimable_association_count": len(claimable_associations),
        "gated_association_count": gated_associations,
        "traversal_count_phrase": f"{len(objects)} object(s) and {len(associations)} association(s)",
        "association_endpoint_format": "`from_key` -> `to_key`",
        "confirmed_fact_instruction": confirmed_instruction,
    }


def collect_evidence_ids(*row_sets: Iterable[dict[str, Any]]) -> list[int]:
    ids: list[int] = []
    for rows in row_sets:
        for row in rows:
            value = row.get("latest_evidence_id")
            if value is None or value == "":
                continue
            try:
                ids.append(int(value))
            except (TypeError, ValueError):
                continue
    return sorted(set(ids))


def latest_source_instance(conn: sqlite3.Connection) -> str | None:
    for table in ["work_program_items", "work_insights", "work_item_forecasts"]:
        if not table_exists(conn, table) or not column_exists(conn, table, "source_instance"):
            continue
        row = conn.execute(
            f"""
            select source_instance, count(*) as count
              from {table}
             where source_instance is not null and source_instance != ''
             group by source_instance
             order by count desc
             limit 1
            """
        ).fetchone()
        if row is not None and row["source_instance"]:
            return str(row["source_instance"])
    return None


def latest_generated_at(conn: sqlite3.Connection, table: str, workstream_keys: list[str], source_instance: str | None) -> str | None:
    if not all(column_exists(conn, table, column) for column in ["workstream_key", "generated_at"]):
        return None
    source_clause, source_params = source_predicate(None, source_instance)
    row = conn.execute(
        f"""
        select generated_at
          from {table}
         where workstream_key in ({placeholders(workstream_keys)})
           {source_clause}
         order by generated_at desc
         limit 1
        """,
        [*workstream_keys, *source_params],
    ).fetchone()
    return str(row["generated_at"]) if row is not None and row["generated_at"] else None


def row_dict(row: sqlite3.Row, table: str) -> dict[str, Any]:
    out = {key: normalize_value(row[key]) for key in row.keys()}
    out["_table"] = table
    return out


def normalize_value(value: Any) -> Any:
    if isinstance(value, bytes):
        return value.decode("utf-8", errors="replace")
    return value


def stable_hash(context: dict[str, Any]) -> str:
    payload = json.dumps(
        {
            "seed": context.get("seed"),
            "rows": context.get("rows"),
            "analytics": context.get("analytics"),
            "guardrails": context.get("guardrails"),
            "context_hash_inputs": context.get("context_hash_inputs"),
        },
        sort_keys=True,
        default=str,
    ).encode("utf-8")
    return hashlib.sha256(payload).hexdigest()[:16]


def stable_digest(parts: Iterable[Any]) -> str:
    payload = "\n".join(str(part) for part in parts).encode("utf-8")
    return hashlib.sha256(payload).hexdigest()[:24]


def citation(kind: str, key: str) -> str:
    return f"[{kind}:{key}]"


def row_citation(row: dict[str, Any]) -> str:
    return citation(str(row.get("_table", "row")), row_citation_key(row))


def row_citation_key(row: dict[str, Any]) -> str:
    for field in ["key", "gate_key", "target_key", "id"]:
        value = row.get(field)
        if value is not None and str(value).strip():
            return str(value)
    return "unknown"


def canonical_workstream_key(workstream_key: str) -> str:
    key = str(workstream_key or "").strip()
    if key.startswith("workstream:"):
        return key
    return f"workstream:{key}"


def workstream_sql_keys(workstream_key: str) -> list[str]:
    canonical = canonical_workstream_key(workstream_key)
    raw = canonical.removeprefix("workstream:")
    return [canonical, raw]


def source_predicate(alias: str | None, source_instance: str | None) -> tuple[str, list[Any]]:
    if not source_instance:
        return "", []
    prefix = f"{alias}." if alias else ""
    return f"and {prefix}source_instance = ?", [source_instance]


def placeholders(values: Iterable[Any]) -> str:
    return ",".join("?" for _ in values)


def bounded(value: int, minimum: int, maximum: int) -> int:
    return max(minimum, min(maximum, int(value)))


def format_number(value: Any) -> str:
    if value is None:
        return "unknown"
    try:
        number = float(value)
    except (TypeError, ValueError):
        return str(value)
    if number.is_integer():
        return str(int(number))
    return f"{number:.2f}"


def one_line(value: Any) -> str:
    return " ".join(str(value or "").split())


def table_exists(conn: sqlite3.Connection, table: str) -> bool:
    row = conn.execute(
        "select 1 from sqlite_master where type in ('table', 'view') and name = ?",
        [table],
    ).fetchone()
    return row is not None


def table_columns(conn: sqlite3.Connection, table: str) -> list[str]:
    return [str(row[1]) for row in conn.execute(f"pragma table_info({table})").fetchall()]


def column_exists(conn: sqlite3.Connection, table: str, column: str) -> bool:
    return column in set(table_columns(conn, table))


if __name__ == "__main__":
    main()
