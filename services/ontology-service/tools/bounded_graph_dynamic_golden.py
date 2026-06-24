#!/usr/bin/env python3
"""Generate a small golden-question pack from a boundedGraphContext payload.

This is for real connector-backed probes where the exact seed and neighbors are
chosen at runtime. The generated pack tests whether an answer can cite the
bounded traversal shape, seed object, selected associations, and sparse coverage
guardrail without smuggling WorkProgram/analytics/source diagnostic vocabulary.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--bounded-graph-context-json", type=Path, required=True)
    parser.add_argument("--golden-json", type=Path, required=True)
    parser.add_argument("--name", default="real-connector-bounded-dynamic")
    parser.add_argument("--max-associations", type=int, default=4)
    parser.add_argument("--forbid", action="append", default=[])
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    payload = json.loads(args.bounded_graph_context_json.read_text(encoding="utf-8"))
    context = extract_context(payload)
    golden = build_golden(context, args.name, max_associations=max(args.max_associations, 0), extra_forbidden=args.forbid)
    args.golden_json.parent.mkdir(parents=True, exist_ok=True)
    args.golden_json.write_text(json.dumps(golden, indent=2, sort_keys=True), encoding="utf-8")


def extract_context(payload: Any) -> dict[str, Any]:
    if isinstance(payload, dict) and isinstance(payload.get("boundedGraphContext"), dict):
        return payload["boundedGraphContext"]
    if isinstance(payload, dict) and isinstance(payload.get("data"), dict) and isinstance(payload["data"].get("boundedGraphContext"), dict):
        return payload["data"]["boundedGraphContext"]
    if isinstance(payload, dict) and isinstance(payload.get("objects"), list) and isinstance(payload.get("associations"), list):
        return payload
    raise SystemExit("input must be a boundedGraphContext object or envelope")


def build_golden(context: dict[str, Any], name: str, *, max_associations: int, extra_forbidden: list[str]) -> dict[str, Any]:
    objects = [row for row in context.get("objects", []) if isinstance(row, dict)]
    associations = [row for row in context.get("associations", []) if isinstance(row, dict)]
    coverage = context.get("coverage") if isinstance(context.get("coverage"), dict) else {}
    coverage_state = str(coverage.get("coverageState") or coverage.get("coverage_state") or "unknown")
    seed = context.get("seed") if isinstance(context.get("seed"), dict) else {}
    seed_key = str(seed.get("key") or "")
    seed_type = str(seed.get("objectType") or seed.get("object_type") or "")
    categories = ["traversal-shape", "seed-object", "generic-no-workprogram-analytics"]
    questions = [
        traversal_question(objects, associations, coverage_state, extra_forbidden),
    ]
    seed_question = build_seed_question(seed_key, seed_type, objects, coverage_state, extra_forbidden)
    if seed_question:
        questions.append(seed_question)
    selected_associations = select_associations(associations, max_associations)
    for index, association in enumerate(selected_associations, start=1):
        categories.append(f"association-{index}")
        questions.append(association_question(index, association, coverage_state, extra_forbidden))
    hydration = hydration_gate_question(selected_associations, coverage_state, extra_forbidden)
    if hydration:
        categories.append("hydration-gate")
        questions.append(hydration)
    if coverage_state in {"sparse", "unknown", "partial", "auth_limited"}:
        categories.append("sparse-coverage")
        questions.append(sparse_coverage_question(coverage_state, extra_forbidden))
    questions.append(no_workprogram_analytics_question(coverage_state, extra_forbidden))
    return {
        "name": name,
        "required_categories": sorted(set(categories)),
        "required_source_coverage_states": [coverage_state],
        "questions": questions,
    }


def traversal_question(objects: list[dict[str, Any]], associations: list[dict[str, Any]], coverage_state: str, extra_forbidden: list[str]) -> dict[str, Any]:
    return {
        "key": "dynamic:traversal-shape",
        "category": "traversal-shape",
        "source_coverage_state": coverage_state,
        "question": "Does the answer describe the bounded graph traversal shape?",
        "expected_facts": [
            {
                "text": f"{len(objects)} object(s) and {len(associations)} association(s)",
                "citation_prefix": "[context:",
            }
        ],
        "forbidden_phrases": common_forbidden(extra_forbidden),
    }


def build_seed_question(seed_key: str, seed_type: str, objects: list[dict[str, Any]], coverage_state: str, extra_forbidden: list[str]) -> dict[str, Any] | None:
    seed_object = next(
        (
            row
            for row in objects
            if str(row.get("key") or "") == seed_key
            and (not seed_type or str(row.get("objectType") or row.get("object_type") or "") == seed_type)
        ),
        None,
    )
    if not seed_object:
        return None
    return {
        "key": "dynamic:seed-object",
        "category": "seed-object",
        "source_coverage_state": coverage_state,
        "question": "Does the answer cite the seed object?",
        "expected_facts": [
            {
                "text": seed_key,
                "citation_prefix": "[graph_objects:",
            }
        ],
        "forbidden_phrases": common_forbidden(extra_forbidden),
    }


def select_associations(associations: list[dict[str, Any]], limit: int) -> list[dict[str, Any]]:
    return sorted(
        associations,
        key=lambda row: (
            row.get("seedDistance") is None and row.get("seed_distance") is None,
            int(row.get("seedDistance") if row.get("seedDistance") is not None else row.get("seed_distance") or 1_000_000),
            str(row.get("from", {}).get("key") if isinstance(row.get("from"), dict) else ""),
            str(row.get("associationType") or row.get("association_type") or ""),
            str(row.get("to", {}).get("key") if isinstance(row.get("to"), dict) else ""),
        ),
    )[:limit]


def association_question(index: int, association: dict[str, Any], coverage_state: str, extra_forbidden: list[str]) -> dict[str, Any]:
    from_ref = association.get("from") if isinstance(association.get("from"), dict) else {}
    to_ref = association.get("to") if isinstance(association.get("to"), dict) else {}
    from_key = str(from_ref.get("key") or "")
    to_key = str(to_ref.get("key") or "")
    association_type = str(association.get("associationType") or association.get("association_type") or "")
    return {
        "key": f"dynamic:association-{index}",
        "category": f"association-{index}",
        "source_coverage_state": coverage_state,
        "question": "Does the answer cite a selected bounded graph relationship?",
        "expected_facts": [
            {
                "text": f"{from_key}` -> `{to_key}",
                "citation_prefix": "[graph_associations:",
            },
            {
                "text": association_type,
                "citation_prefix": "[graph_associations:",
            },
        ],
        "forbidden_phrases": common_forbidden(extra_forbidden),
    }


def hydration_gate_question(associations: list[dict[str, Any]], coverage_state: str, extra_forbidden: list[str]) -> dict[str, Any] | None:
    gated = [
        row
        for row in associations
        if "hydration" in str(row.get("claimGateReason") or row.get("claim_gate_reason") or "")
        or str(row.get("claimAllowed") or row.get("claim_allowed")).lower() == "false"
    ]
    if not gated:
        return None
    reason = str(gated[0].get("claimGateReason") or gated[0].get("claim_gate_reason") or "")
    return {
        "key": "dynamic:hydration-gate",
        "category": "hydration-gate",
        "source_coverage_state": coverage_state,
        "question": "Does the answer keep incomplete relationships as validation context?",
        "expected_facts": [
            {
                "text": reason,
                "citation_prefix": "[graph_associations:",
            }
        ],
        "forbidden_phrases": [
            *common_forbidden(extra_forbidden),
            "confirmed implementation",
            "fully hydrated",
            "production truth",
        ],
    }


def sparse_coverage_question(coverage_state: str, extra_forbidden: list[str]) -> dict[str, Any]:
    return {
        "key": "dynamic:sparse-coverage",
        "category": "sparse-coverage",
        "source_coverage_state": coverage_state,
        "question": "Does sparse source coverage keep missing neighbors unknown?",
        "expected_facts": [
            {
                "text": "missing neighbors are unknown, not absent",
                "citation_prefix": "[guardrail:",
            }
        ],
        "forbidden_phrases": [
            *common_forbidden(extra_forbidden),
            "no other pull requests",
            "no other tickets",
            "absence claims are allowed",
        ],
    }


def no_workprogram_analytics_question(coverage_state: str, extra_forbidden: list[str]) -> dict[str, Any]:
    return {
        "key": "dynamic:no-workprogram-analytics",
        "category": "generic-no-workprogram-analytics",
        "source_coverage_state": coverage_state,
        "question": "Does the generic bounded answer avoid WorkProgram and analytics citations?",
        "expected_facts": [
            {
                "text": "missing neighbors are unknown, not absent",
                "citation_prefix": "[guardrail:",
            }
        ],
        "forbidden_phrases": common_forbidden(extra_forbidden),
    }


def common_forbidden(extra: list[str]) -> list[str]:
    return sorted(
        {
            "WorkProgram",
            "TPM",
            "TicketLensResult",
            "PullRequestLensResult",
            "[analytics:",
            "SourceSyncIssue",
            "SourceSyncRun",
            "SourceScope",
            "UnresolvedReference",
            "token=",
            "Authorization",
            *[value for item in extra for value in item.split(",") if value.strip()],
        }
    )


if __name__ == "__main__":
    main()
