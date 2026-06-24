#!/usr/bin/env python3
"""Audit which bounded graph facts are promotable to product claims."""

from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

import bounded_graph_contract
import cubicle_graph_brief as graph_brief


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Audit boundedGraphContext promotion readiness.")
    parser.add_argument("--bounded-graph-context-json", type=Path, required=True)
    parser.add_argument("--report-json", type=Path)
    parser.add_argument("--profile", choices=["fixture", "connector"], default="fixture")
    parser.add_argument(
        "--source-authority-json",
        type=Path,
        help="Optional relationship source-authority matrix for claimable relationship facts.",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    payload = json.loads(args.bounded_graph_context_json.read_text(encoding="utf-8"))
    source_authority_policy = None
    if args.source_authority_json:
        source_authority_policy = load_source_authority_policy(args.source_authority_json)
    report = audit_bounded_graph_context_payload(
        payload,
        profile=args.profile,
        source_authority_policy=source_authority_policy,
    )
    if args.report_json:
        args.report_json.parent.mkdir(parents=True, exist_ok=True)
        args.report_json.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    if not report["passes_promotion_audit"]:
        raise SystemExit(format_promotion_failures(report))


def load_source_authority_policy(path: Path) -> dict[str, dict[str, Any]]:
    return normalize_source_authority_policy(json.loads(path.read_text(encoding="utf-8")))


def normalize_source_authority_policy(payload: Any) -> dict[str, dict[str, Any]]:
    if not isinstance(payload, dict):
        return {}
    table = payload.get("relationship_authority") or payload.get("relationshipAuthority") or payload
    if not isinstance(table, dict):
        return {}
    out: dict[str, dict[str, Any]] = {}
    for relationship_type, row in table.items():
        relationship_key = clean(relationship_type)
        if not relationship_key or not isinstance(row, dict):
            continue
        out[relationship_key] = {
            "presence_sources": normalize_source_list(
                row.get("presence_sources") or row.get("presenceSources") or []
            ),
            "presence_source_instances": normalize_source_map(
                row.get("presence_source_instances") or row.get("presenceSourceInstances") or {}
            ),
            "presence_mapper_versions": normalize_source_map(
                row.get("presence_mapper_versions") or row.get("presenceMapperVersions") or {}
            ),
            "presence_locator_kinds": normalize_source_map(
                row.get("presence_locator_kinds") or row.get("presenceLocatorKinds") or {}
            ),
            "absence_sources": normalize_source_list(
                row.get("absence_sources") or row.get("absenceSources") or []
            ),
        }
    return out


def normalize_source_list(values: Any) -> list[str]:
    if isinstance(values, str):
        values = [values]
    if not isinstance(values, list):
        return []
    return sorted({source_key(value) for value in values if source_key(value)})


def audit_bounded_graph_context_payload(
    payload: Any,
    *,
    profile: str = "fixture",
    source_authority_policy: dict[str, dict[str, Any]] | None = None,
) -> dict[str, Any]:
    contract = bounded_graph_contract.validate_bounded_graph_context_payload(payload, profile=profile)
    context = graph_brief.extract_bounded_graph_context_payload(payload)
    objects = [row for row in context.get("objects", []) if isinstance(row, dict)]
    associations = [row for row in context.get("associations", []) if isinstance(row, dict)]
    evidence = [row for row in context.get("evidence", []) if isinstance(row, dict)]
    objects_by_ref = {
        bounded_graph_contract.object_ref_key(row): row
        for row in objects
        if bounded_graph_contract.object_ref_key(row)
    }
    evidence_by_key = {
        clean(row.get("key")): row
        for row in evidence
        if clean(row.get("key"))
    }
    logical_counts: dict[str, int] = {}
    for row in associations:
        key = bounded_graph_contract.logical_association_key(row)
        if key:
            logical_counts[key] = logical_counts.get(key, 0) + 1

    object_rows = [audit_object(row) for row in objects]
    association_rows = [
        audit_association(row, objects_by_ref, evidence_by_key, logical_counts, source_authority_policy)
        for row in associations
    ]
    blockers = [
        {"kind": "contract_error", "path": row.get("path", "$"), "detail": row.get("detail", "")}
        for row in contract.get("errors", [])
    ]
    if contract.get("blocking_warning_count", 0):
        blockers.extend(
            {"kind": "contract_warning", "path": row.get("path", "$"), "detail": row.get("detail", "")}
            for row in contract.get("warnings", [])
        )
    blockers.extend(promotion_blockers(object_rows, association_rows))
    passes_promotion_audit = contract["passes_contract"] and not blockers

    return {
        "profile": profile,
        "context_hash": contract.get("context_hash"),
        "source_authority_policy_applied": source_authority_policy is not None,
        "passes_contract": contract["passes_contract"],
        "passes_promotion_audit": passes_promotion_audit,
        "object_count": len(object_rows),
        "association_count": len(association_rows),
        "promotable_object_count": sum(1 for row in object_rows if row["promotionReady"]),
        "promotable_association_count": sum(1 for row in association_rows if row["promotionReady"]),
        "blocked_object_count": sum(1 for row in object_rows if row["blockers"]),
        "blocked_association_count": sum(1 for row in association_rows if row["blockers"]),
        "objects": object_rows,
        "associations": association_rows,
        "blockers": blockers,
    }


def audit_object(row: dict[str, Any]) -> dict[str, Any]:
    blockers: list[str] = []
    claim_requested = bool(row.get("claimAllowed", row.get("claim_allowed", False)))
    if not claim_requested:
        blockers.append(clean(row.get("claimGateReason") or row.get("claim_gate_reason")) or "object_not_claimable")
    if clean(row.get("visibility")) != "public":
        blockers.append("object_visibility_not_public")
    freshness = clean(row.get("freshnessState") or row.get("freshness_state"))
    if freshness in {"partial", "stale", "superseded", "tombstoned"}:
        blockers.append("object_not_current")
    if bounded_graph_contract.source_is_generated(row):
        blockers.append("object_generated_requires_source_evidence")
    return {
        "objectType": clean(row.get("objectType") or row.get("object_type")),
        "key": clean(row.get("key")),
        "claimRequested": claim_requested,
        "promotionReady": not blockers,
        "blockers": sorted(set(blockers)),
    }


def audit_association(
    row: dict[str, Any],
    objects_by_ref: dict[str, dict[str, Any]],
    evidence_by_key: dict[str, dict[str, Any]],
    logical_counts: dict[str, int],
    source_authority_policy: dict[str, dict[str, Any]] | None = None,
) -> dict[str, Any]:
    blockers: list[str] = []
    claim_requested = bool(row.get("claimAllowed", row.get("claim_allowed", False)))
    association_type = clean(row.get("associationType") or row.get("association_type"))
    if not claim_requested:
        blockers.append(clean(row.get("claimGateReason") or row.get("claim_gate_reason")) or "association_not_claimable")
    logical_key = bounded_graph_contract.logical_association_key(row)
    if logical_counts.get(logical_key, 0) > 1:
        blockers.append("relationship_multi_evidence_requires_review")
    for ref_key, side in [
        (bounded_graph_contract.object_ref_key(row.get("from") or {}), "from"),
        (bounded_graph_contract.object_ref_key(row.get("to") or {}), "to"),
    ]:
        endpoint = objects_by_ref.get(ref_key)
        if endpoint is None:
            blockers.append(f"relationship_{side}_endpoint_missing")
            continue
        if clean(endpoint.get("visibility")) != "public":
            blockers.append(f"relationship_{side}_endpoint_restricted")
        if clean(endpoint.get("freshnessState") or endpoint.get("freshness_state")) in {"partial", "stale", "superseded", "tombstoned"}:
            blockers.append(f"relationship_{side}_endpoint_not_current")
    evidence_key = clean(row.get("evidenceKey") or row.get("evidence_key"))
    evidence = evidence_by_key.get(evidence_key)
    evidence_source = association_evidence_source(evidence)
    evidence_source_instance = association_evidence_source_instance(evidence)
    mapper_version = association_mapper_version(row, evidence)
    evidence_locator_kind = association_evidence_locator_kind(evidence)
    if not evidence_key:
        blockers.append("missing_relationship_evidence")
    elif evidence is None:
        blockers.append("relationship_evidence_missing_row")
    elif bounded_graph_contract.source_is_generated(evidence):
        blockers.append("relationship_generated_requires_source_evidence")
    if claim_requested and source_authority_policy is not None:
        blockers.extend(
            source_authority_blockers(
                association_type,
                evidence_source,
                evidence_source_instance,
                mapper_version,
                evidence_locator_kind,
                source_authority_policy,
            )
        )
    if clean(row.get("visibility")) != "public":
        blockers.append("relationship_visibility_not_public")
    if clean(row.get("freshnessState") or row.get("freshness_state")) not in {"fresh", "current"}:
        blockers.append("relationship_not_current")
    if clean(row.get("proofState") or row.get("proof_state")) not in {"source_observed", "current"}:
        blockers.append("relationship_proof_not_source_observed")
    confidence = graph_brief.parse_float_or_none(row.get("confidence"))
    if confidence is None or confidence < 1:
        blockers.append("relationship_confidence_below_one")
    return {
        "key": clean(row.get("key")),
        "associationType": association_type,
        "logicalKey": logical_key,
        "evidenceKey": evidence_key,
        "evidenceSource": evidence_source,
        "evidenceSourceInstance": evidence_source_instance,
        "mapperVersion": mapper_version,
        "evidenceLocatorKind": evidence_locator_kind,
        "claimRequested": claim_requested,
        "promotionReady": not blockers,
        "blockers": sorted(set(blockers)),
    }


def association_evidence_source(evidence: dict[str, Any] | None) -> str:
    if evidence:
        for key in ["source", "sourceSystem", "source_system"]:
            value = source_key(evidence.get(key))
            if value:
                return value
    return ""


def association_evidence_locator_kind(evidence: dict[str, Any] | None) -> str:
    if evidence:
        for key in ["locatorKind", "locator_kind"]:
            value = source_key(evidence.get(key))
            if value:
                return value
    return ""


def association_evidence_source_instance(evidence: dict[str, Any] | None) -> str:
    if evidence:
        for key in ["sourceInstance", "source_instance"]:
            value = source_key(evidence.get(key))
            if value:
                return value
    return ""


def association_mapper_version(row: dict[str, Any], evidence: dict[str, Any] | None) -> str:
    for key in ["mapperVersion", "mapper_version", "sourceVersion", "source_version"]:
        value = source_key(row.get(key))
        if value:
            return value
    if evidence:
        for key in ["mapperVersion", "mapper_version", "sourceVersion", "source_version"]:
            value = source_key(evidence.get(key))
            if value:
                return value
    return ""


def source_authority_blockers(
    association_type: str,
    evidence_source: str,
    evidence_source_instance: str,
    mapper_version: str,
    evidence_locator_kind: str,
    source_authority_policy: dict[str, dict[str, Any]],
) -> list[str]:
    row = source_authority_policy.get(association_type)
    if row is None:
        return ["relationship_authority_policy_missing"]
    presence_sources = row.get("presence_sources") or []
    if not evidence_source:
        return ["relationship_source_authority_missing_evidence_source"]
    if evidence_source not in presence_sources and "*" not in presence_sources and "all" not in presence_sources:
        return ["relationship_source_not_authoritative_for_presence"]
    presence_source_instances = row.get("presence_source_instances") or {}
    if presence_source_instances:
        allowed_instances = presence_source_instances.get(evidence_source) or presence_source_instances.get("*")
        if not allowed_instances:
            return ["relationship_source_authority_missing_instance_policy"]
        if not evidence_source_instance:
            return ["relationship_source_authority_missing_evidence_source_instance"]
        if evidence_source_instance not in allowed_instances and "*" not in allowed_instances and "all" not in allowed_instances:
            return ["relationship_source_instance_not_authoritative_for_presence"]
    presence_mapper_versions = row.get("presence_mapper_versions") or {}
    if presence_mapper_versions:
        allowed_versions = presence_mapper_versions.get(evidence_source) or presence_mapper_versions.get("*")
        if not allowed_versions:
            return ["relationship_source_authority_missing_mapper_version_policy"]
        if not mapper_version:
            return ["relationship_source_authority_missing_evidence_mapper_version"]
        if mapper_version not in allowed_versions and "*" not in allowed_versions and "all" not in allowed_versions:
            return ["relationship_mapper_version_not_authoritative_for_presence"]
    presence_locator_kinds = row.get("presence_locator_kinds") or {}
    if presence_locator_kinds:
        allowed_locators = presence_locator_kinds.get(evidence_source) or presence_locator_kinds.get("*")
        if not allowed_locators:
            return ["relationship_source_authority_missing_locator_policy"]
        if not evidence_locator_kind:
            return ["relationship_source_authority_missing_evidence_locator_kind"]
        if evidence_locator_kind not in allowed_locators and "*" not in allowed_locators and "all" not in allowed_locators:
            return ["relationship_locator_not_authoritative_for_presence"]
    return []


def promotion_blockers(object_rows: list[dict[str, Any]], association_rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    out: list[dict[str, Any]] = []
    out.extend(
        {
            "kind": "object_promotion_blocker",
            "key": row.get("key", ""),
            "detail": ",".join(row.get("blockers", [])),
        }
        for row in object_rows
        if row.get("claimRequested") and row.get("blockers")
    )
    out.extend(
        {
            "kind": "association_promotion_blocker",
            "key": row.get("key", ""),
            "detail": ",".join(row.get("blockers", [])),
        }
        for row in association_rows
        if row.get("claimRequested") and row.get("blockers")
    )
    return out


def format_promotion_failures(report: dict[str, Any]) -> str:
    blockers = report.get("blockers", [])
    if blockers:
        return "; ".join(
            f"{row.get('path') or row.get('key') or '$'}: {row.get('detail')}"
            for row in blockers[:5]
        )
    return "bounded graph promotion audit failed"


def clean(value: Any) -> str:
    return str(value or "").strip()


def source_key(value: Any) -> str:
    return clean(value).lower()


def normalize_source_map(values: Any) -> dict[str, list[str]]:
    if not isinstance(values, dict):
        return {}
    out: dict[str, list[str]] = {}
    for key, raw_list in values.items():
        source = source_key(key)
        if not source:
            continue
        out[source] = normalize_source_list(raw_list)
    return out


if __name__ == "__main__":
    main()
