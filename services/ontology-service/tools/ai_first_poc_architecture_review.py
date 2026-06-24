#!/usr/bin/env python3
"""Render the strategic AI-first PoC architecture review from readiness artifacts."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Answer whether the bounded graph PoC is generic, LLM-viable, and product-safe."
    )
    parser.add_argument("--architecture-readiness-json", type=Path, required=True)
    parser.add_argument("--real-connector-inventory-json", type=Path)
    parser.add_argument("--out-json", type=Path)
    parser.add_argument("--out-md", type=Path)
    parser.add_argument("--require-poc-viable", action="store_true")
    parser.add_argument("--require-product-safe", action="store_true")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    report = build_review(
        load_json(args.architecture_readiness_json),
        real_connector_inventory=load_json(args.real_connector_inventory_json) if args.real_connector_inventory_json else {},
    )
    if args.out_json:
        args.out_json.parent.mkdir(parents=True, exist_ok=True)
        args.out_json.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    if args.out_md:
        args.out_md.parent.mkdir(parents=True, exist_ok=True)
        args.out_md.write_text(render_markdown(report), encoding="utf-8")
    print_summary(report)
    failures = []
    if args.require_poc_viable and not report["verdicts"]["working_poc_viable"]["passes"]:
        failures.append("working PoC is not viable")
    if args.require_product_safe and not report["verdicts"]["product_safe_rollout_ready"]["passes"]:
        failures.append("product-safe rollout is not ready")
    if failures:
        raise SystemExit("; ".join(failures))


def build_review(
    readiness: dict[str, Any],
    *,
    real_connector_inventory: dict[str, Any] | None = None,
) -> dict[str, Any]:
    inventory = real_connector_inventory or {}
    tiers = readiness.get("tiers") if isinstance(readiness.get("tiers"), dict) else {}
    case_rows = list_rows(readiness.get("case_summaries"))
    answer_rows = list_rows(readiness.get("answer_eval_summaries"))
    product_requirements = list_rows(readiness.get("product_safe_requirements"))

    poc_green = tier_passes(tiers, "poc_green")
    production_genericity_green = tier_passes(tiers, "production_genericity_advisory_green")
    answer_path_ready = tier_passes(tiers, "eval_gated_answer_path_ready") or tier_passes(
        tiers,
        "any_product_answer_path_ready",
    )
    raw_model_ready = tier_passes(tiers, "raw_model_product_ready")
    product_safe_ready = tier_passes(tiers, "product_safe_architecture_green")
    traversal_signal = graph_traversal_signal(answer_rows)
    product_safe_blockers = [row for row in product_requirements if not bool(row.get("satisfied"))]
    inventory_findings = inventory_findings_from_report(inventory)

    verdicts = {
        "working_poc_viable": {
            "passes": poc_green and answer_path_ready,
            "basis": "matrix PoC gate passes and at least one repaired or deterministic eval-gated answer path passes",
        },
        "not_pigeonholed_at_graph_boundary": {
            "passes": production_genericity_green,
            "basis": "production-genericity advisory gate covers real connector, real non-Flink/non-GitHub domain, OpenGraph, source authority, and typed-vs-graph promotion tags",
        },
        "connector_dataset_genericity_proven": {
            "passes": bool(inventory) and int(inventory.get("real_non_flink_candidate_count") or 0) > 0,
            "basis": "requires at least one real non-Flink connector candidate in the scanned persisted connector data root",
        },
        "llm_graph_traversal_viable": {
            "passes": answer_path_ready and traversal_signal["deterministic_or_repaired_pass_count"] > 0,
            "basis": "bounded graph context can feed deterministic or repaired answer paths; raw model success is diagnostic, not required",
        },
        "raw_model_can_stand_alone": {
            "passes": raw_model_ready,
            "basis": "requires all raw local-model eval reports to pass without repair",
        },
        "product_safe_rollout_ready": {
            "passes": product_safe_ready,
            "basis": "requires production-genericity plus every explicit product-safe requirement to be satisfied",
        },
    }

    return {
        "summary": summary_text(verdicts, product_safe_blockers, inventory_findings),
        "verdicts": verdicts,
        "graph_traversal_signal": traversal_signal,
        "inventory_findings": inventory_findings,
        "product_safe_blockers": [
            {
                "key": clean(row.get("key")),
                "detail": clean(row.get("detail")),
                "verification": clean_list(row.get("verification")),
            }
            for row in product_safe_blockers
        ],
        "recommended_architecture": recommended_architecture(),
        "next_actions": next_actions(verdicts, product_safe_blockers, inventory_findings),
        "case_counts": {
            "case_count": len(case_rows),
            "passing_case_count": sum(1 for row in case_rows if bool(row.get("passed"))),
            "answer_eval_count": len(answer_rows),
        },
    }


def graph_traversal_signal(answer_rows: list[dict[str, Any]]) -> dict[str, Any]:
    deterministic_or_repaired = [
        row
        for row in answer_rows
        if clean(row.get("answer_kind")) in {"deterministic", "repaired_model"}
    ]
    raw_rows = [row for row in answer_rows if clean(row.get("answer_kind")) == "raw_model"]
    seed_baselines = [row for row in answer_rows if clean(row.get("answer_kind")) == "seed_only_baseline"]
    typed_baselines = [row for row in answer_rows if clean(row.get("answer_kind")) == "typed_row_baseline"]
    return {
        "deterministic_or_repaired_pass_count": sum(1 for row in deterministic_or_repaired if bool(row.get("passes_eval"))),
        "deterministic_or_repaired_eval_count": len(deterministic_or_repaired),
        "raw_model_pass_count": sum(1 for row in raw_rows if bool(row.get("passes_eval"))),
        "raw_model_eval_count": len(raw_rows),
        "seed_only_failure_count": sum(1 for row in seed_baselines if not bool(row.get("passes_eval"))),
        "seed_only_eval_count": len(seed_baselines),
        "typed_row_failure_count": sum(1 for row in typed_baselines if not bool(row.get("passes_eval"))),
        "typed_row_eval_count": len(typed_baselines),
        "baselines_show_traversal_value": bool(seed_baselines or typed_baselines)
        and all(not bool(row.get("passes_eval")) for row in [*seed_baselines, *typed_baselines]),
    }


def inventory_findings_from_report(inventory: dict[str, Any]) -> dict[str, Any]:
    if not inventory:
        return {
            "inventory_supplied": False,
            "real_non_flink_candidate_count": None,
            "product_acl_current_nonpublic_database_count": None,
            "source_backed_acl_candidate_database_count": None,
            "real_acl_ingestion_database_count": None,
            "product_acl_current_nonpublic_rows_present": "unknown",
            "source_scope_negative_row_database_count": None,
            "source_backed_source_scope_negative_candidate_database_count": None,
            "source_scope_stale_or_not_attempted_database_count": None,
            "production_source_connection_count": None,
            "source_connection_connector_kind_counts": {},
            "production_connector_kinds": [],
            "connector_data_root_pigeonhole_risk": "unknown",
            "real_acl_ingestion_present": "unknown",
            "source_scope_negative_row_present": "unknown",
            "real_source_scope_negative_present": "unknown",
        }
    real_non_flink = int(inventory.get("real_non_flink_candidate_count") or 0)
    product_acl_rows = int(inventory.get("product_acl_current_nonpublic_database_count") or 0)
    source_backed_acl = int(inventory.get("source_backed_acl_candidate_database_count") or 0)
    real_acl = int(inventory.get("real_acl_ingestion_database_count") or 0)
    source_scope_negative_rows = int(inventory.get("source_scope_negative_row_database_count") or 0)
    source_backed_source_scope_negative = int(
        inventory.get("source_backed_source_scope_negative_candidate_database_count") or 0
    )
    source_scope_negative = int(inventory.get("source_scope_stale_or_not_attempted_database_count") or 0)
    return {
        "inventory_supplied": True,
        "database_count": int(inventory.get("database_count") or 0),
        "ok_database_count": int(inventory.get("ok_database_count") or 0),
        "real_non_flink_candidate_count": real_non_flink,
        "flink_shaped_candidate_count": int(inventory.get("flink_shaped_candidate_count") or 0),
        "product_acl_current_nonpublic_database_count": product_acl_rows,
        "source_backed_acl_candidate_database_count": source_backed_acl,
        "real_acl_ingestion_database_count": real_acl,
        "product_acl_current_nonpublic_rows_present": "yes" if product_acl_rows > 0 else "no",
        "source_scope_negative_row_database_count": source_scope_negative_rows,
        "source_backed_source_scope_negative_candidate_database_count": source_backed_source_scope_negative,
        "source_scope_stale_or_not_attempted_database_count": source_scope_negative,
        "production_source_connection_count": int(inventory.get("production_source_connection_count") or 0),
        "source_connection_connector_kind_counts": inventory.get("source_connection_connector_kind_counts", {}),
        "production_connector_kinds": inventory.get("production_connector_kinds", []),
        "connector_data_root_pigeonhole_risk": "high" if real_non_flink == 0 else "reduced",
        "real_acl_ingestion_present": "not_proven_by_inventory" if real_acl > 0 else "no",
        "source_scope_negative_row_present": "yes" if source_scope_negative_rows > 0 else "no",
        "real_source_scope_negative_present": "yes" if source_scope_negative > 0 else "no",
    }


def recommended_architecture() -> list[dict[str, str]]:
    return [
        {
            "layer": "durable_graph",
            "role": "Source-backed typed rows and governed OpenGraph rows are the truth layer.",
            "guardrail": "No product reads from raw manifests or SourceSync adjacency.",
        },
        {
            "layer": "evidence_authority",
            "role": "Evidence, source authority, coverage, ACL, freshness, confidence, and mapper/source version decide claimability.",
            "guardrail": "Partial coverage, non-200 source issues, and hidden ACL rows can support diagnostics only, not absence or public claims.",
        },
        {
            "layer": "llm_synthesis",
            "role": "LLMs summarize bounded graph context after traversal has selected claimable context.",
            "guardrail": "Raw model output is diagnostic unless it passes the same eval gate; repaired or deterministic paths are display candidates until product-safe gates pass.",
        },
        {
            "layer": "operating_overlays",
            "role": "TPM/work-program and forecast views sit over the graph as evaluated operating artifacts.",
            "guardrail": "Forecasts stay risk triage until they beat simple baselines on real time-series evidence.",
        },
    ]


def next_actions(
    verdicts: dict[str, dict[str, Any]],
    product_safe_blockers: list[dict[str, Any]],
    inventory_findings: dict[str, Any],
) -> list[str]:
    actions: list[str] = []
    if not verdicts["connector_dataset_genericity_proven"]["passes"] and inventory_findings.get("inventory_supplied"):
        actions.append("Add or capture a second real non-Flink connector-backed dataset, not just another Flink/Jira/GitHub slice.")
    blocker_keys = {clean(row.get("key")) for row in product_safe_blockers}
    if "real_acl_connector" in blocker_keys:
        actions.append("Ingest real source ACL state into product graph rows and rerun the real ACL hard gate.")
    elif inventory_findings.get("product_acl_current_nonpublic_rows_present") == "no":
        actions.append("Add product-row ACL ingestion evidence with non-public current rows and rerun the ACL row gate.")
    if inventory_findings.get("real_source_scope_negative_present") == "no":
        actions.append("Capture a real stale-window or source-not-attempted source-scope state and rerun the source-scope hard gate.")
    if not verdicts["raw_model_can_stand_alone"]["passes"]:
        actions.append("Keep raw local-model output behind repair/deterministic verification; do not use raw answers as product truth.")
    if product_safe_blockers:
        actions.append("Keep product-safe architecture red until every explicit product-safe requirement is backed by real evidence.")
    if not actions:
        actions.append("Run the product-safe required gate and move from PoC to rollout review.")
    return actions


def summary_text(
    verdicts: dict[str, dict[str, Any]],
    product_safe_blockers: list[dict[str, Any]],
    inventory_findings: dict[str, Any],
) -> str:
    if verdicts["working_poc_viable"]["passes"] and verdicts["not_pigeonholed_at_graph_boundary"]["passes"]:
        status = "The bounded graph PoC is viable and not pigeonholed at the graph boundary."
    else:
        status = "The bounded graph PoC is not yet proven viable or generic."
    if inventory_findings.get("connector_data_root_pigeonhole_risk") == "high":
        status += " The current persisted connector data root is still Flink-shaped."
    if product_safe_blockers:
        status += " Product-safe rollout remains blocked by real connector evidence gaps."
    return status


def render_markdown(report: dict[str, Any]) -> str:
    lines = [
        "# AI-First PoC Architecture Review",
        "",
        report["summary"],
        "",
        "## Verdicts",
        "",
    ]
    for key, row in report["verdicts"].items():
        lines.append(f"- `{key}`: {'pass' if row['passes'] else 'fail'} - {row['basis']}")
    lines.extend(["", "## Graph Traversal Signal", ""])
    signal = report["graph_traversal_signal"]
    lines.extend([
        f"- deterministic/repaired passing evals: `{signal['deterministic_or_repaired_pass_count']}/{signal['deterministic_or_repaired_eval_count']}`",
        f"- raw-model passing evals: `{signal['raw_model_pass_count']}/{signal['raw_model_eval_count']}`",
        f"- seed-only baseline failures: `{signal['seed_only_failure_count']}/{signal['seed_only_eval_count']}`",
        f"- typed-row baseline failures: `{signal['typed_row_failure_count']}/{signal['typed_row_eval_count']}`",
        f"- baselines show traversal value: `{str(signal['baselines_show_traversal_value']).lower()}`",
    ])
    lines.extend(["", "## Inventory Findings", ""])
    for key, value in report["inventory_findings"].items():
        lines.append(f"- `{key}`: `{value}`")
    lines.extend(["", "## Recommended Architecture", ""])
    for row in report["recommended_architecture"]:
        lines.append(f"- `{row['layer']}`: {row['role']} Guardrail: {row['guardrail']}")
    lines.extend(["", "## Product-Safe Blockers", ""])
    if report["product_safe_blockers"]:
        for blocker in report["product_safe_blockers"]:
            lines.append(f"- `{blocker['key']}`: {blocker['detail']}")
    else:
        lines.append("- none")
    lines.extend(["", "## Next Actions", ""])
    for action in report["next_actions"]:
        lines.append(f"- {action}")
    lines.append("")
    return "\n".join(lines)


def print_summary(report: dict[str, Any]) -> None:
    print(
        "ai_first_poc_architecture_review "
        + " ".join(f"{key}={row['passes']}" for key, row in report["verdicts"].items())
    )
    print(
        "ai_first_poc_architecture_review blockers="
        + str(len(report["product_safe_blockers"]))
        + " next_actions="
        + str(len(report["next_actions"]))
    )


def tier_passes(tiers: dict[str, Any], key: str) -> bool:
    row = tiers.get(key)
    return isinstance(row, dict) and bool(row.get("passes"))


def load_json(path: Path | None) -> dict[str, Any]:
    if path is None:
        return {}
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        raise SystemExit(f"missing JSON file: {path}")
    except json.JSONDecodeError as exc:
        raise SystemExit(f"invalid JSON file {path}: {exc}")
    if not isinstance(data, dict):
        raise SystemExit(f"JSON file is not an object: {path}")
    return data


def list_rows(value: Any) -> list[dict[str, Any]]:
    if not isinstance(value, list):
        return []
    return [row for row in value if isinstance(row, dict)]


def clean_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [text for text in (clean(item) for item in value) if text]


def clean(value: Any) -> str:
    return str(value or "").strip()


if __name__ == "__main__":
    main()
