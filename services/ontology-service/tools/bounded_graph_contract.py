#!/usr/bin/env python3
"""Validate the generic boundedGraphContext prompt contract."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

import cubicle_graph_brief as graph_brief


WORKPROGRAM_CONTEXT_KEYS = {
    "actions",
    "badges",
    "dependencyEdges",
    "evidenceNeeds",
    "forecastPacket",
    "forecasts",
    "guardrailPacket",
    "insights",
    "items",
    "qualityGates",
    "sourceCoveragePacket",
    "workActions",
    "workDependencyEdges",
    "workInsights",
    "workItemForecasts",
    "workProgramItems",
}
WORKPROGRAM_OBJECT_TYPES = {
    "work_action",
    "work_dependency_edge",
    "work_insight",
    "work_item_forecast",
    "work_program",
    "work_program_item",
    "workstream_health",
}
CONNECTOR_SOURCE_NEUTRAL_OBJECT_TYPES = {
    "person",
}
BUILTIN_PRODUCT_OBJECT_TYPES = {
    "action_candidate",
    "blocker",
    "code_file",
    "decision",
    "document",
    "document_fragment",
    "message",
    "person",
    "pull_request",
    "risk",
    "team",
    "ticket",
    "workstream",
}
GENERATED_SOURCE_VALUES = {
    "cubicle_ai",
    "generated",
    "llm",
}
RAW_PROMPT_FIELD_KEYS = {
    "body",
    "body_sha256",
    "headers",
    "locator",
    "raw",
    "raw_body",
    "rawbody",
    "source_url",
    "sourceurl",
    "token",
    "url",
}


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Validate a boundedGraphContext JSON contract.")
    parser.add_argument("--bounded-graph-context-json", type=Path, required=True)
    parser.add_argument("--report-json", type=Path)
    parser.add_argument("--profile", choices=["fixture", "connector"], default="fixture")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    report = validate_bounded_graph_context_file(args.bounded_graph_context_json, profile=args.profile)
    if args.report_json:
        args.report_json.parent.mkdir(parents=True, exist_ok=True)
        args.report_json.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    if not report["passes_contract"]:
        raise SystemExit(format_contract_errors(report))


def validate_bounded_graph_context_file(path: Path, *, profile: str = "fixture") -> dict[str, Any]:
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        return contract_report(None, profile, [{"path": "$", "kind": "invalid_json", "detail": str(exc)}], [])
    return validate_bounded_graph_context_payload(payload, profile=profile)


def validate_bounded_graph_context_payload(payload: Any, *, profile: str = "fixture") -> dict[str, Any]:
    errors: list[dict[str, str]] = []
    warnings: list[dict[str, str]] = []
    if not isinstance(payload, dict):
        return contract_report(None, profile, [{"path": "$", "kind": "invalid_payload", "detail": "bounded graph input must be a JSON object"}], warnings)
    if isinstance(payload.get("workProgramGraphContext"), dict):
        errors.append({"path": "$.workProgramGraphContext", "kind": "workprogram_payload", "detail": "bounded graph entrypoint does not accept workProgramGraphContext"})
    data = payload.get("data")
    if isinstance(data, dict) and isinstance(data.get("workProgramGraphContext"), dict):
        errors.append({"path": "$.data.workProgramGraphContext", "kind": "workprogram_payload", "detail": "bounded graph entrypoint does not accept workProgramGraphContext"})

    try:
        context = graph_brief.extract_bounded_graph_context_payload(payload)
    except ValueError as exc:
        errors.append({"path": "$", "kind": "missing_bounded_context", "detail": str(exc)})
        return contract_report(None, profile, errors, warnings)

    validate_context_shape(context, profile, errors, warnings)
    validate_prompt_safety_fields(context, errors)
    return contract_report(context, profile, errors, warnings)


def validate_context_shape(context: dict[str, Any], profile: str, errors: list[dict[str, str]], warnings: list[dict[str, str]]) -> None:
    scope_mode = clean(context.get("scopeMode") or context.get("scope_mode"))
    if scope_mode != "bounded_graph_context":
        errors.append({"path": "$.boundedGraphContext.scopeMode", "kind": "invalid_scope_mode", "detail": "bounded graph input must declare scopeMode=bounded_graph_context"})
    for key in sorted(WORKPROGRAM_CONTEXT_KEYS):
        if key in context:
            errors.append({"path": f"$.boundedGraphContext.{key}", "kind": "workprogram_key", "detail": f"bounded graph input contains WorkProgram context key {key}"})
    if isinstance(context.get("analytics"), dict):
        errors.append({"path": "$.boundedGraphContext.analytics", "kind": "analytics_rows", "detail": "bounded graph input must not carry analytics rows"})

    require_nonempty(context, "contextHash", "$.boundedGraphContext.contextHash", errors)
    validate_seed(context.get("seed"), errors)
    validate_int_at_least(context.get("depth"), 0, "$.boundedGraphContext.depth", errors)
    validate_int_at_least(context.get("limitPerObject") or context.get("limit_per_object"), 1, "$.boundedGraphContext.limitPerObject", errors)
    validate_coverage(context.get("coverage"), context, profile, errors, warnings)
    validate_objects(context.get("objects"), profile, errors, warnings)
    validate_associations(context.get("associations"), context.get("objects"), context.get("evidence"), profile, errors, warnings)
    validate_supplied_citations(context.get("citations"), errors)


def validate_seed(value: Any, errors: list[dict[str, str]]) -> None:
    if not isinstance(value, dict):
        errors.append({"path": "$.boundedGraphContext.seed", "kind": "missing_seed", "detail": "seed object is required"})
        return
    require_nonempty(value, "objectType", "$.boundedGraphContext.seed.objectType", errors)
    require_nonempty(value, "key", "$.boundedGraphContext.seed.key", errors)


def validate_coverage(value: Any, context: dict[str, Any], profile: str, errors: list[dict[str, str]], warnings: list[dict[str, str]]) -> None:
    if not isinstance(value, dict):
        errors.append({"path": "$.boundedGraphContext.coverage", "kind": "missing_coverage", "detail": "coverage object is required"})
        return
    require_nonempty(value, "coverageState", "$.boundedGraphContext.coverage.coverageState", errors)
    if "absenceClaimsAllowed" not in value and "absence_claims_allowed" not in value:
        errors.append({"path": "$.boundedGraphContext.coverage.absenceClaimsAllowed", "kind": "missing_absence_policy", "detail": "absenceClaimsAllowed is required"})
    absence_allowed = bool(value.get("absenceClaimsAllowed", value.get("absence_claims_allowed", False)))
    gate_reason = clean(value.get("absenceClaimGateReason") or value.get("absence_claim_gate_reason"))
    if not gate_reason:
        errors.append({"path": "$.boundedGraphContext.coverage.absenceClaimGateReason", "kind": "missing_absence_gate_reason", "detail": "absenceClaimGateReason is required"})
    association_scope_present = "absenceClaimAssociationTypes" in value or "absence_claim_association_types" in value
    covered_associations = string_list(value.get("absenceClaimAssociationTypes") or value.get("absence_claim_association_types"))
    if absence_allowed and not coverage_covers_requested_associations(covered_associations, requested_association_types(context)):
        errors.append({"path": "$.boundedGraphContext.coverage.absenceClaimAssociationTypes", "kind": "absence_relation_scope_missing", "detail": "absence claims require relation/path-scoped coverage"})
    for field, json_name in [
        ("sourceSystem", "sourceSystem"),
        ("sourceInstance", "sourceInstance"),
        ("coverageWindowStart", "coverageWindowStart"),
        ("coverageWindowEnd", "coverageWindowEnd"),
    ]:
        text = clean(value.get(field) or value.get(graph_brief.camel_to_snake(field)))
        if absence_allowed and not text:
            errors.append({"path": f"$.boundedGraphContext.coverage.{json_name}", "kind": "absence_source_scope_missing", "detail": f"absence claims require {json_name}"})
        elif profile == "connector" and not text:
            warnings.append({"path": f"$.boundedGraphContext.coverage.{json_name}", "kind": "connector_source_scope_missing", "detail": f"connector profile should provide {json_name} even when absence claims are gated"})
    if profile == "connector" and not association_scope_present:
        warnings.append({"path": "$.boundedGraphContext.coverage.absenceClaimAssociationTypes", "kind": "connector_relation_scope_missing", "detail": "connector profile should declare covered association types or an explicit empty proof scope"})


def validate_objects(values: Any, profile: str, errors: list[dict[str, str]], warnings: list[dict[str, str]]) -> None:
    if not isinstance(values, list):
        errors.append({"path": "$.boundedGraphContext.objects", "kind": "invalid_objects", "detail": "objects must be a list"})
        return
    for index, row in enumerate(values):
        path = f"$.boundedGraphContext.objects[{index}]"
        if not isinstance(row, dict):
            errors.append({"path": path, "kind": "invalid_object", "detail": "object row must be an object"})
            continue
        object_type = clean(row.get("objectType") or row.get("object_type"))
        if object_type in WORKPROGRAM_OBJECT_TYPES:
            errors.append({"path": f"{path}.objectType", "kind": "workprogram_object_type", "detail": f"bounded graph input contains WorkProgram object type {object_type}"})
        require_nonempty(row, "objectType", f"{path}.objectType", errors)
        require_nonempty(row, "key", f"{path}.key", errors)
        if "claimAllowed" not in row and "claim_allowed" not in row:
            errors.append({"path": f"{path}.claimAllowed", "kind": "missing_claim_policy", "detail": "object claimAllowed is required"})
        claim_allowed = bool(row.get("claimAllowed", row.get("claim_allowed", False)))
        if claim_allowed:
            require_nonempty(row, "proofState", f"{path}.proofState", errors)
            visibility = clean(row.get("visibility"))
            if not visibility:
                errors.append({"path": f"{path}.visibility", "kind": "claimable_visibility_missing_object", "detail": "claimable objects must declare public visibility"})
            elif visibility != "public":
                errors.append({"path": f"{path}.visibility", "kind": "claimable_restricted_object", "detail": "claimable objects must be public"})
            freshness = clean(row.get("freshnessState") or row.get("freshness_state"))
            if freshness in {"", "unknown"}:
                errors.append({"path": f"{path}.freshnessState", "kind": "claimable_unknown_freshness_object", "detail": "claimable objects must declare fresh/current freshness"})
            elif freshness in {"partial", "stale", "superseded", "tombstoned"}:
                errors.append({"path": f"{path}.freshnessState", "kind": "claimable_noncurrent_object", "detail": "claimable objects must not be partial, stale, superseded, or tombstoned"})
            if source_is_generated(row):
                errors.append({"path": path, "kind": "claimable_generated_object", "detail": "generated objects require source evidence before they can support claims"})
            if object_type not in BUILTIN_PRODUCT_OBJECT_TYPES:
                errors.append({"path": f"{path}.objectType", "kind": "claimable_open_graph_object", "detail": "custom OpenGraph object rows are context-only until promoted into product schema"})
        elif not clean(row.get("claimGateReason") or row.get("claim_gate_reason")):
            errors.append({"path": f"{path}.claimGateReason", "kind": "missing_claim_gate_reason", "detail": "non-claimable objects require claimGateReason"})
        if profile == "connector" and object_type not in CONNECTOR_SOURCE_NEUTRAL_OBJECT_TYPES and not clean(row.get("sourceInstance") or row.get("source_instance")):
            warnings.append({"path": f"{path}.sourceInstance", "kind": "connector_object_source_scope_missing", "detail": "connector object should provide sourceInstance"})


def validate_associations(values: Any, object_values: Any, evidence_values: Any, profile: str, errors: list[dict[str, str]], warnings: list[dict[str, str]]) -> None:
    if not isinstance(values, list):
        errors.append({"path": "$.boundedGraphContext.associations", "kind": "invalid_associations", "detail": "associations must be a list"})
        return
    evidence_keys = {
        clean(row.get("key"))
        for row in evidence_values or []
        if isinstance(row, dict) and clean(row.get("key"))
    }
    evidence_by_key = {
        clean(row.get("key")): row
        for row in evidence_values or []
        if isinstance(row, dict) and clean(row.get("key"))
    }
    objects_by_ref = {
        object_ref_key(row): row
        for row in object_values or []
        if isinstance(row, dict) and object_ref_key(row)
    }
    logical_association_counts = {}
    for row in values:
        if isinstance(row, dict):
            key = logical_association_key(row)
            if key:
                logical_association_counts[key] = logical_association_counts.get(key, 0) + 1
    for index, row in enumerate(values):
        path = f"$.boundedGraphContext.associations[{index}]"
        if not isinstance(row, dict):
            errors.append({"path": path, "kind": "invalid_association", "detail": "association row must be an object"})
            continue
        require_nonempty(row, "key", f"{path}.key", errors)
        require_nonempty(row, "associationType", f"{path}.associationType", errors)
        validate_ref(row.get("from"), f"{path}.from", errors)
        validate_ref(row.get("to"), f"{path}.to", errors)
        if "claimAllowed" not in row and "claim_allowed" not in row:
            errors.append({"path": f"{path}.claimAllowed", "kind": "missing_claim_policy", "detail": "association claimAllowed is required"})
        claim_allowed = bool(row.get("claimAllowed", row.get("claim_allowed", False)))
        evidence_key = clean(row.get("evidenceKey") or row.get("evidence_key"))
        if claim_allowed:
            for field in ["evidenceKey", "proofState", "visibility", "freshnessState"]:
                require_nonempty(row, field, f"{path}.{field}", errors)
            if logical_association_counts.get(logical_association_key(row), 0) > 1:
                errors.append({"path": path, "kind": "claimable_duplicate_logical_association", "detail": "duplicate logical associations require merge review before they can support claims"})
            validate_claimable_association_endpoint(row.get("from"), objects_by_ref, f"{path}.from", errors)
            validate_claimable_association_endpoint(row.get("to"), objects_by_ref, f"{path}.to", errors)
            if evidence_key and evidence_key not in evidence_keys:
                errors.append({"path": f"{path}.evidenceKey", "kind": "missing_evidence_row", "detail": f"claimable association evidenceKey {evidence_key} is not present in evidence rows"})
            elif evidence_key and source_is_generated(evidence_by_key.get(evidence_key, {})):
                errors.append({"path": f"{path}.evidenceKey", "kind": "claimable_generated_relationship_evidence", "detail": "generated relationship evidence requires source-backed proof before it can support claims"})
            if clean(row.get("visibility")) != "public":
                errors.append({"path": f"{path}.visibility", "kind": "claimable_restricted_association", "detail": "claimable associations must be public"})
            if clean(row.get("freshnessState") or row.get("freshness_state")) not in {"fresh", "current"}:
                errors.append({"path": f"{path}.freshnessState", "kind": "claimable_noncurrent_association", "detail": "claimable associations must be fresh or current"})
            if clean(row.get("proofState") or row.get("proof_state")) not in {"source_observed", "current"}:
                errors.append({"path": f"{path}.proofState", "kind": "claimable_noncurrent_proof", "detail": "claimable associations require source_observed/current proof"})
            confidence = graph_brief.parse_float_or_none(row.get("confidence"))
            if confidence is None or confidence < 1:
                errors.append({"path": f"{path}.confidence", "kind": "claimable_low_confidence", "detail": "claimable associations require confidence >= 1"})
        elif not clean(row.get("claimGateReason") or row.get("claim_gate_reason")):
            errors.append({"path": f"{path}.claimGateReason", "kind": "missing_claim_gate_reason", "detail": "non-claimable associations require claimGateReason"})
        if profile == "connector" and not evidence_key:
            warnings.append({"path": f"{path}.evidenceKey", "kind": "connector_relationship_evidence_missing", "detail": "connector associations should provide evidenceKey"})


def validate_ref(value: Any, path: str, errors: list[dict[str, str]]) -> None:
    if not isinstance(value, dict):
        errors.append({"path": path, "kind": "invalid_ref", "detail": "association endpoint must be an object"})
        return
    require_nonempty(value, "objectType", f"{path}.objectType", errors)
    require_nonempty(value, "key", f"{path}.key", errors)


def validate_claimable_association_endpoint(value: Any, objects_by_ref: dict[str, dict[str, Any]], path: str, errors: list[dict[str, str]]) -> None:
    if not isinstance(value, dict):
        return
    endpoint = objects_by_ref.get(object_ref_key(value))
    if endpoint is None:
        errors.append({"path": path, "kind": "claimable_association_endpoint_missing", "detail": "claimable associations require both endpoints in boundedGraphContext.objects"})
        return
    visibility = clean(endpoint.get("visibility"))
    if visibility != "public":
        errors.append({"path": path, "kind": "claimable_association_endpoint_not_public", "detail": "claimable association endpoints must be public"})
    freshness = clean(endpoint.get("freshnessState") or endpoint.get("freshness_state"))
    if freshness in {"", "unknown"}:
        errors.append({"path": path, "kind": "claimable_association_endpoint_unknown_freshness", "detail": "claimable association endpoints must declare fresh/current freshness"})
    elif freshness in {"partial", "stale", "superseded", "tombstoned"}:
        errors.append({"path": path, "kind": "claimable_association_endpoint_not_current", "detail": "claimable association endpoints must be hydrated and current"})


def validate_supplied_citations(values: Any, errors: list[dict[str, str]]) -> None:
    if values is None:
        return
    if not isinstance(values, list):
        errors.append({"path": "$.boundedGraphContext.citations", "kind": "invalid_citations", "detail": "citations must be a list when supplied"})
        return
    for index, citation in enumerate(values):
        if not isinstance(citation, dict):
            continue
        ref = clean(citation.get("ref"))
        if ref.startswith("[analytics:"):
            errors.append({"path": f"$.boundedGraphContext.citations[{index}].ref", "kind": "analytics_citation", "detail": "bounded graph input must not carry analytics citations"})


def object_ref_key(value: dict[str, Any]) -> str:
    object_type = clean(value.get("objectType") or value.get("object_type"))
    key = clean(value.get("key"))
    if not object_type or not key:
        return ""
    return object_type + ":" + key


def logical_association_key(value: dict[str, Any]) -> str:
    association_type = clean(value.get("associationType") or value.get("association_type"))
    from_ref = value.get("from")
    to_ref = value.get("to")
    if not isinstance(from_ref, dict) or not isinstance(to_ref, dict):
        return ""
    from_key = object_ref_key(from_ref)
    to_key = object_ref_key(to_ref)
    if not association_type or not from_key or not to_key:
        return ""
    return from_key + "|" + association_type + "|" + to_key


def source_is_generated(value: dict[str, Any]) -> bool:
    source = clean(value.get("source") or value.get("sourceSystem") or value.get("source_system")).lower()
    return source in GENERATED_SOURCE_VALUES


def validate_prompt_safety_fields(value: Any, errors: list[dict[str, str]], path: str = "$") -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            normalized_key = clean(key).replace("-", "_").lower()
            if normalized_key in RAW_PROMPT_FIELD_KEYS:
                errors.append({"path": f"{path}.{key}", "kind": "raw_prompt_field", "detail": f"bounded graph prompt context must not expose raw/source field {key}"})
            validate_prompt_safety_fields(child, errors, f"{path}.{key}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            validate_prompt_safety_fields(child, errors, f"{path}[{index}]")


def contract_report(context: dict[str, Any] | None, profile: str, errors: list[dict[str, str]], warnings: list[dict[str, str]]) -> dict[str, Any]:
    rows = context if isinstance(context, dict) else {}
    blocking_warning_count = len(warnings) if profile == "connector" else 0
    return {
        "profile": profile,
        "context_hash": clean(rows.get("contextHash") or rows.get("context_hash")) or None,
        "object_count": len(rows.get("objects", [])) if isinstance(rows.get("objects"), list) else 0,
        "association_count": len(rows.get("associations", [])) if isinstance(rows.get("associations"), list) else 0,
        "error_count": len(errors),
        "warning_count": len(warnings),
        "blocking_warning_count": blocking_warning_count,
        "errors": errors,
        "warnings": warnings,
        "passes_contract": not errors and blocking_warning_count == 0,
    }


def format_contract_errors(report: dict[str, Any]) -> str:
    errors = report.get("errors", [])
    warnings = report.get("warnings", [])
    if not errors and not warnings:
        return "bounded graph contract failed"
    rows = errors if errors else warnings
    return "; ".join(f"{row.get('path')}: {row.get('detail')}" for row in rows[:5])


def require_nonempty(row: dict[str, Any], key: str, path: str, errors: list[dict[str, str]]) -> None:
    snake_key = graph_brief.camel_to_snake(key)
    if not clean(row.get(key) or row.get(snake_key)):
        errors.append({"path": path, "kind": "missing_required_field", "detail": f"{key} is required"})


def validate_int_at_least(value: Any, minimum: int, path: str, errors: list[dict[str, str]]) -> None:
    try:
        parsed = int(value)
    except (TypeError, ValueError):
        errors.append({"path": path, "kind": "invalid_integer", "detail": f"integer >= {minimum} is required"})
        return
    if parsed < minimum:
        errors.append({"path": path, "kind": "invalid_integer", "detail": f"integer >= {minimum} is required"})


def requested_association_types(context: dict[str, Any]) -> list[str]:
    return string_list(
        context.get("associationTypes")
        or context.get("association_types")
        or context.get("requestedAssociationTypes")
        or context.get("requested_association_types")
    )


def coverage_covers_requested_associations(covered: list[str], requested: list[str]) -> bool:
    covered_set = set(covered)
    if "*" in covered_set or "all" in covered_set:
        return True
    if not requested:
        return False
    return all(value in covered_set for value in requested)


def string_list(value: Any) -> list[str]:
    if value is None:
        return []
    if isinstance(value, str):
        value = [value]
    if not isinstance(value, list):
        return []
    return sorted({clean(row) for row in value if clean(row)})


def clean(value: Any) -> str:
    return str(value or "").strip()


if __name__ == "__main__":
    main()
