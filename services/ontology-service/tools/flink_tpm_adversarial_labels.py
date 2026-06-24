#!/usr/bin/env python3
"""Draft non-measurement adversarial labels for current AI-TPM review queues."""

from __future__ import annotations

import argparse
import csv
import re
from pathlib import Path
from typing import Any


NEGATED_BLOCKER_RE = re.compile(
    r"(?i)\b(no|not|none|without)\b.{0,60}\b(block(?:ed|er|ing)?|regression|conflict|failure|failing|error|dependency|dependencies)\b"
)


OUTPUT_COLUMNS = [
    "queue_rank",
    "measurement_bucket",
    "insight_key",
    "insight_kind",
    "subject_kind",
    "subject_key",
    "action_type",
    "title",
    "truth_label",
    "actionability_label",
    "review_state",
    "owner_key",
    "next_action",
    "rationale",
]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--measurement-queue", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--limit", type=int, default=60)
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    queue = read_tsv(args.measurement_queue)
    labels = build_adversarial_labels(queue, args.limit)
    write_tsv(labels, args.output)


def read_tsv(path: Path) -> list[dict[str, str]]:
    with path.open(newline="", encoding="utf-8") as handle:
        return [dict(row) for row in csv.DictReader(handle, delimiter="\t")]


def write_tsv(rows: list[dict[str, Any]], path: Path) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=OUTPUT_COLUMNS, delimiter="\t", extrasaction="ignore")
        writer.writeheader()
        writer.writerows(rows)


def build_adversarial_labels(queue: list[dict[str, str]], limit: int) -> list[dict[str, Any]]:
    labels: list[dict[str, Any]] = []
    for row in queue[: max(0, limit)]:
        labels.append(label_row(row))
    return labels


def label_row(row: dict[str, str]) -> dict[str, Any]:
    truth_label, actionability_label, review_state, next_action, rationale = classify(row)
    return {
        "queue_rank": clean(row.get("queue_rank")),
        "measurement_bucket": clean(row.get("measurement_bucket")),
        "insight_key": clean(row.get("insight_key")),
        "insight_kind": clean(row.get("insight_kind")),
        "subject_kind": clean(row.get("subject_kind")),
        "subject_key": clean(row.get("subject_key")),
        "action_type": clean(row.get("action_type")),
        "title": clean(row.get("title")),
        "truth_label": truth_label,
        "actionability_label": actionability_label,
        "review_state": review_state,
        "owner_key": clean(row.get("owner_key")),
        "next_action": next_action,
        "rationale": rationale,
    }


def classify(row: dict[str, str]) -> tuple[str, str, str, str, str]:
    insight_kind = clean(row.get("insight_kind"))
    bucket = clean(row.get("measurement_bucket"))
    action_type = clean(row.get("action_type"))
    subject = clean(row.get("subject_key"))
    evidence = clean(row.get("evidence_excerpt"))
    title = clean(row.get("title"))

    if bucket == "ci_actionability" or action_type == "ci_check_followup" or "ci check" in title.lower():
        return (
            "partial",
            "needs_owner",
            "needs_more_data",
            f"Confirm whether observed failing or pending checks block {subject}; assign an owner or record why they are non-blocking.",
            "Adversarial review: live check/status payloads can justify a CI follow-up, but required-check coverage is unavailable, so this is not yet a product-action claim.",
        )

    if insight_kind == "forecast_risk":
        return (
            "partial",
            "needs_owner",
            "needs_more_data",
            f"Ask the owner for merge, close, or parking status on {subject}; keep this as risk triage only.",
            "Adversarial review: historical risk-triage backtest supports attention ordering, but ETA readiness is false and the evidence must not be presented as a date forecast.",
        )

    if insight_kind == "developer_correlation":
        return (
            "partial",
            "needs_owner",
            "needs_more_data",
            f"Use {subject} only for capacity/routing discussion; do not infer ownership, causality, performance, or blocker absence.",
            "Adversarial review: direct identity makes the workload comparison admissible, but aggregate correlation is weak and the signal is contextual only.",
        )

    if insight_kind == "blocker_candidate":
        if NEGATED_BLOCKER_RE.search(evidence):
            return (
                "false_positive",
                "not_actionable",
                "dismissed",
                f"Suppress blocker escalation for {subject} unless stronger non-negated evidence appears.",
                "Adversarial review: blocker keyword appears in a negated context, so this should not become a product blocker claim.",
            )
        return (
            "partial",
            "needs_owner",
            "needs_more_data",
            f"Validate blocker evidence for {subject} with the owner or source thread before escalation.",
            "Adversarial review: source text contains blocker-like language, but keyword evidence alone is insufficient for autonomous escalation.",
        )

    if insight_kind == "dependency_cluster":
        return (
            "partial",
            "needs_owner",
            "needs_more_data",
            f"Split {subject} into owned dependency edges and identify which edge is actually blocking progress.",
            "Adversarial review: a large linked-work cluster is operationally useful, but it does not prove a blocker without owner-confirmed dependency state.",
        )

    return (
        "partial",
        "needs_owner",
        "needs_more_data",
        f"Review {subject} and record whether the signal should become product work.",
        "Adversarial review: generated TPM signal needs source validation and owner judgment before product escalation.",
    )


def clean(value: Any) -> str:
    return "" if value is None else str(value).strip()


if __name__ == "__main__":
    main()
