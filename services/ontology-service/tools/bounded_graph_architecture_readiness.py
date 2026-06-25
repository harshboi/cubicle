#!/usr/bin/env python3
"""Summarize AI-first bounded graph architecture readiness from matrix output."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any


DEFAULT_PRODUCT_SAFE_REQUIREMENTS = {
    "real_acl_connector": "real connector-backed principal/group ACL translation blocks hidden hubs",
    "source_acl_product_row_ingestion": "connector-shaped loaders write current ACL metadata onto product graph rows",
    "second_real_non_github_domain": "second real non-Flink and non-GitHub connector/domain proof",
    "negative_partial_real_connector_cases": "real partial/negative connector cases for missing auth, non-200 source issues, missing snapshots, and partial source scope",
    "source_scope_not_attempted_lifecycle_state": "source-scope registration can represent planned but not-attempted coverage without a sync run",
    "real_external_connector_source_scope_negative_capture": "real external connector graph candidate includes stale or not-attempted source-scope state",
    "relationship_identity_conflict_cases": "duplicate/conflicting relationship identity and evidence upsert cases",
    "absence_claim_principal_coverage": "absence claims proven with relation/path/source/time/principal coverage",
    "generated_summary_quarantine": "generated summaries are prevented from re-entering source/product truth",
}


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Build a tiered readiness report from bounded graph promotion-matrix artifacts."
    )
    parser.add_argument("--matrix-report-json", type=Path, required=True)
    parser.add_argument("--product-safe-evidence-json", type=Path)
    parser.add_argument("--out-json", type=Path)
    parser.add_argument("--out-md", type=Path)
    parser.add_argument("--require-production-genericity", action="store_true")
    parser.add_argument("--require-product-safe", action="store_true")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    report = build_readiness_report(
        load_json(args.matrix_report_json),
        matrix_report_path=args.matrix_report_json,
        product_safe_evidence=load_json(args.product_safe_evidence_json) if args.product_safe_evidence_json else {},
    )
    if args.out_json:
        args.out_json.parent.mkdir(parents=True, exist_ok=True)
        args.out_json.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    if args.out_md:
        args.out_md.parent.mkdir(parents=True, exist_ok=True)
        args.out_md.write_text(render_markdown(report), encoding="utf-8")
    print_summary(report)
    failures = []
    if not report["tiers"]["poc_green"]["passes"]:
        failures.append("PoC readiness is not green")
    if args.require_production_genericity and not report["tiers"]["production_genericity_advisory_green"]["passes"]:
        failures.append("production-genericity advisory readiness is not green")
    if args.require_product_safe and not report["tiers"]["product_safe_architecture_green"]["passes"]:
        failures.append("product-safe architecture readiness is not green")
    if failures:
        raise SystemExit("; ".join(failures))


def build_readiness_report(
    matrix_report: dict[str, Any],
    *,
    matrix_report_path: Path,
    product_safe_evidence: dict[str, Any],
) -> dict[str, Any]:
    covered_tags = sorted(clean_list(matrix_report.get("covered_tags")))
    required_tags = sorted(clean_list(matrix_report.get("required_tags")))
    advisory_tags = sorted(clean_list(matrix_report.get("advisory_tags")))
    missing_required_tags = [tag for tag in required_tags if tag not in covered_tags]
    advisory_gaps = list_rows(matrix_report.get("advisory_gaps"))
    case_rows = case_summaries(matrix_report)
    answer_rows = answer_eval_summaries(matrix_report, matrix_report_path)
    product_requirements = product_safe_requirement_rows(product_safe_evidence)

    poc_green = bool(matrix_report.get("passes_matrix")) and not missing_required_tags
    production_genericity_green = (
        poc_green
        and not advisory_gaps
        and "real-non-github-domain" in covered_tags
        and "real-non-flink-connector" in covered_tags
        and "real-connector" in covered_tags
    )
    raw_rows = [row for row in answer_rows if row["answer_kind"] == "raw_model"]
    repaired_rows = [row for row in answer_rows if row["answer_kind"] == "repaired_model"]
    deterministic_rows = [row for row in answer_rows if row["answer_kind"] == "deterministic"]
    raw_model_product_ready = bool(raw_rows) and all(row["passes_eval"] for row in raw_rows)
    repaired_or_deterministic_ready = any(row["passes_eval"] for row in [*repaired_rows, *deterministic_rows])
    eval_gated_answer_path_ready = raw_model_product_ready or repaired_or_deterministic_ready
    product_safe_green = (
        production_genericity_green
        and eval_gated_answer_path_ready
        and all(row["satisfied"] for row in product_requirements)
    )

    blockers = []
    diagnostics = []
    if missing_required_tags:
        blockers.append("missing required matrix tags: " + ", ".join(missing_required_tags))
    if advisory_gaps:
        blockers.append("missing advisory tags: " + ", ".join(str(row.get("tag")) for row in advisory_gaps))
    if not eval_gated_answer_path_ready:
        blockers.append("no raw, repaired, or deterministic eval-gated answer path passes")
    elif not raw_model_product_ready:
        diagnostics.append("raw model output is diagnostic, not product-ready; repaired or deterministic answer path remains the display gate")
    for row in product_requirements:
        if not row["satisfied"]:
            blockers.append(row["detail"])

    return {
        "matrix_report": str(matrix_report_path),
        "matrix_out_dir": clean(matrix_report.get("out_dir")),
        "tiers": {
            "poc_green": {
                "passes": poc_green,
                "basis": "promotion matrix passes and all required tags are covered",
            },
            "production_genericity_advisory_green": {
                "passes": production_genericity_green,
                "basis": "PoC gate passes, advisory gaps are closed, and real non-Flink plus real non-GitHub connector/domain tags are covered",
            },
            "raw_model_product_ready": {
                "passes": raw_model_product_ready,
                "basis": "all raw local-model eval reports pass without repair",
            },
            "repaired_or_deterministic_product_path_ready": {
                "passes": repaired_or_deterministic_ready,
                "basis": "at least one repaired or deterministic answer path passes eval",
            },
            "eval_gated_answer_path_ready": {
                "passes": eval_gated_answer_path_ready,
                "basis": "raw, repaired, or deterministic answer path passes its eval gate",
            },
            "product_safe_architecture_green": {
                "passes": product_safe_green,
                "basis": "production-genericity gate plus explicit product-safe evidence requirements",
            },
        },
        "covered_tags": covered_tags,
        "required_tags": required_tags,
        "advisory_tags": advisory_tags,
        "advisory_gaps": advisory_gaps,
        "missing_required_tags": missing_required_tags,
        "case_summaries": case_rows,
        "answer_eval_summaries": answer_rows,
        "product_safe_requirements": product_requirements,
        "blockers": blockers,
        "diagnostics": diagnostics,
    }


def case_summaries(matrix_report: dict[str, Any]) -> list[dict[str, Any]]:
    out = []
    for case in list_rows(matrix_report.get("cases")):
        metrics = case.get("metrics") if isinstance(case.get("metrics"), dict) else {}
        out.append(
            {
                "key": clean(case.get("key")),
                "passed": bool(case.get("passed")),
                "tags": sorted(clean_list(case.get("tags"))),
                "golden_pass_total": int(metrics.get("golden_pass_total") or 0),
                "golden_question_total": int(metrics.get("golden_question_total") or 0),
                "contract_pass_count": metrics.get("contract_pass_count"),
                "contract_report_count": metrics.get("contract_report_count"),
                "promotion_audit_pass_count": metrics.get("promotion_audit_pass_count"),
                "promotion_audit_report_count": metrics.get("promotion_audit_report_count"),
                "promotable_association_count": metrics.get("promotable_association_count"),
                "blocked_association_count": metrics.get("blocked_association_count"),
                "forbidden_term_hit_count": metrics.get("forbidden_term_hit_count"),
            }
        )
    return out


def answer_eval_summaries(matrix_report: dict[str, Any], matrix_report_path: Path) -> list[dict[str, Any]]:
    matrix_out_dir = Path(clean(matrix_report.get("out_dir")) or matrix_report_path.parent)
    out = []
    for case in list_rows(matrix_report.get("cases")):
        case_key = clean(case.get("key"))
        case_dir = Path(clean(case.get("out_dir")) or matrix_out_dir / case_key)
        for path in sorted(case_dir.glob("*eval.json")):
            answer_kind = answer_kind_for_eval_path(path)
            if answer_kind == "ignored":
                continue
            data = load_json(path)
            golden = data.get("golden_eval") if isinstance(data.get("golden_eval"), dict) else {}
            out.append(
                {
                    "case": case_key,
                    "path": str(path),
                    "file": path.name,
                    "answer_kind": answer_kind,
                    "passes_eval": bool(data.get("passes_eval")),
                    "passes_smoke_eval": bool(data.get("passes_smoke_eval")),
                    "repair_applied": bool(data.get("repair_applied")),
                    "golden_pass_count": int(golden.get("pass_count") or 0),
                    "golden_question_count": int(golden.get("question_count") or 0),
                }
            )
    return out


def answer_kind_for_eval_path(path: Path) -> str:
    name = path.name
    if name == "raw_eval.json":
        return "raw_model"
    if name == "repaired_eval.json":
        return "repaired_model"
    if name in {"eval.json", "generic_baseline_eval.json"}:
        return "deterministic"
    if "seed_only" in name:
        return "seed_only_baseline"
    if "typed_row" in name:
        return "typed_row_baseline"
    return "ignored"


def product_safe_requirement_rows(evidence: dict[str, Any]) -> list[dict[str, Any]]:
    rows = []
    for key, detail in DEFAULT_PRODUCT_SAFE_REQUIREMENTS.items():
        evidence_value = evidence.get(key)
        if isinstance(evidence_value, dict):
            rows.append(
                {
                    "key": key,
                    "satisfied": bool(evidence_value.get("satisfied")),
                    "detail": clean(evidence_value.get("detail")) or detail,
                    "evidence": list(clean_list(evidence_value.get("evidence"))),
                    "verification": list(clean_list(evidence_value.get("verification"))),
                }
            )
            continue
        rows.append(
            {
                "key": key,
                "satisfied": bool(evidence_value),
                "detail": detail,
                "evidence": [],
                "verification": [],
            }
        )
    return rows


def render_markdown(report: dict[str, Any]) -> str:
    lines = [
        "# AI-First Bounded Graph Architecture Readiness",
        "",
        "## Tier Status",
        "",
    ]
    for key, row in report["tiers"].items():
        lines.append(f"- `{key}`: {'pass' if row['passes'] else 'fail'} - {row['basis']}")
    lines.extend(["", "## Case Evidence", ""])
    for row in report["case_summaries"]:
        lines.append(
            f"- `{row['key']}`: {'pass' if row['passed'] else 'fail'}, "
            f"golden `{row['golden_pass_total']}/{row['golden_question_total']}`, "
            f"promotable associations `{row.get('promotable_association_count')}`, "
            f"blocked associations `{row.get('blocked_association_count')}`"
        )
    lines.extend(["", "## Answer Path Evidence", ""])
    for row in report["answer_eval_summaries"]:
        lines.append(
            f"- `{row['case']}` `{row['answer_kind']}` `{row['file']}`: "
            f"{'pass' if row['passes_eval'] else 'fail'}, "
            f"smoke `{'pass' if row['passes_smoke_eval'] else 'fail'}`, "
            f"golden `{row['golden_pass_count']}/{row['golden_question_count']}`, "
            f"repair `{row['repair_applied']}`"
        )
    lines.extend(["", "## Product-Safe Requirements", ""])
    for row in report["product_safe_requirements"]:
        lines.append(f"- `{row['key']}`: {'satisfied' if row['satisfied'] else 'missing'} - {row['detail']}")
        for item in row.get("evidence", []):
            lines.append(f"  - evidence: `{item}`")
        for item in row.get("verification", []):
            lines.append(f"  - verification: `{item}`")
    lines.extend(["", "## Blockers", ""])
    if report["blockers"]:
        for blocker in report["blockers"]:
            lines.append(f"- {blocker}")
    else:
        lines.append("- none")
    lines.extend(["", "## Diagnostics", ""])
    if report.get("diagnostics"):
        for diagnostic in report["diagnostics"]:
            lines.append(f"- {diagnostic}")
    else:
        lines.append("- none")
    lines.append("")
    return "\n".join(lines)


def print_summary(report: dict[str, Any]) -> None:
    tier_text = " ".join(
        f"{key}={row['passes']}"
        for key, row in report["tiers"].items()
    )
    print("bounded_graph_architecture_readiness " + tier_text)
    print(
        "bounded_graph_architecture_readiness cases="
        + str(len(report["case_summaries"]))
        + " answer_evals="
        + str(len(report["answer_eval_summaries"]))
        + " blockers="
        + str(len(report["blockers"]))
        + " diagnostics="
        + str(len(report.get("diagnostics", [])))
    )


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
