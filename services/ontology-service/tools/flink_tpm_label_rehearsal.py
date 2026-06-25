#!/usr/bin/env python3
"""Dry-run a completed TPM label file against copied ontology/analytics DBs."""

from __future__ import annotations

import argparse
import csv
import hashlib
import sqlite3
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


TOOLS_DIR = Path(__file__).resolve().parent
TRUTH_LABELS = {"unknown", "true_positive", "false_positive", "partial"}
ACTIONABILITY_LABELS = {"unknown", "actionable", "not_actionable", "needs_owner"}
REVIEW_STATES = {"", "requested", "needs_more_data", "accepted", "dismissed", "resolved"}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser()
    parser.add_argument("--ontology-db", required=True, type=Path)
    parser.add_argument("--analytics-db", required=True, type=Path)
    parser.add_argument("--labels", required=True, type=Path)
    parser.add_argument("--source-instance", required=True)
    parser.add_argument("--out-dir", required=True, type=Path)
    parser.add_argument("--label-set", default="rehearsal_eval")
    parser.add_argument("--label-quality", default="gold", choices=["auto", "adversarial", "candidate", "gold", "smoke"])
    parser.add_argument("--measurement-label-set", action="append", default=[])
    parser.add_argument("--reviewer-key", default="rehearsal")
    parser.add_argument("--reviewed-at", default=datetime.now(timezone.utc).isoformat())
    parser.add_argument("--prefix", default="label-rehearsal")
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    paths = prepare_rehearsal_paths(args.out_dir, args.prefix, args.ontology_db, args.analytics_db)
    source_hashes_before = database_hashes(args.ontology_db, args.analytics_db)
    label_summary = summarize_label_file(args.labels)
    preflight_report = paths["out_dir"] / f"{args.prefix}.label-preflight.md"
    write_label_preflight_report(preflight_report, args.labels, label_summary, source_hashes_before)
    validate_label_preflight(label_summary, preflight_report)
    copy_database(args.ontology_db, paths["ontology_db"])
    copy_database(args.analytics_db, paths["analytics_db"])
    for copied_db, source_db, db_role in [
        (paths["ontology_db"], args.ontology_db, "ontology"),
        (paths["analytics_db"], args.analytics_db, "analytics"),
    ]:
        mark_rehearsal_copy(
            copied_db,
            source_db=source_db,
            db_role=db_role,
            labels=args.labels,
            source_instance=args.source_instance,
            label_set=args.label_set,
            prefix=args.prefix,
            reviewed_at=args.reviewed_at,
        )

    review_report = paths["out_dir"] / f"{args.prefix}.review-evaluation.md"
    action_report = paths["out_dir"] / f"{args.prefix}.action-brief.md"
    queue_path = paths["out_dir"] / f"{args.prefix}.remaining-measurement-queue.tsv"
    queue_report = paths["out_dir"] / f"{args.prefix}.remaining-measurement-queue.md"

    review_cmd = [
        sys.executable,
        str(TOOLS_DIR / "flink_tpm_review_labels.py"),
        "--ontology-db",
        str(paths["ontology_db"]),
        "--analytics-db",
        str(paths["analytics_db"]),
        "--source-instance",
        args.source_instance,
        "--import-labels",
        str(args.labels),
        "--label-set",
        args.label_set,
        "--label-quality",
        args.label_quality,
        "--reviewer-key",
        args.reviewer_key,
        "--reviewed-at",
        args.reviewed_at,
        "--report",
        str(review_report),
        "--export-measurement-queue",
        str(queue_path),
        "--measurement-queue-report",
        str(queue_report),
        "--measurement-queue-size",
        "60",
    ]
    for label_set in args.measurement_label_set:
        review_cmd.extend(["--measurement-label-set", label_set])
    run_command(review_cmd)

    action_cmd = [
        sys.executable,
        str(TOOLS_DIR / "flink_tpm_action_brief.py"),
        "--analytics-db",
        str(paths["analytics_db"]),
        "--ontology-db",
        str(paths["ontology_db"]),
        "--source-instance",
        args.source_instance,
        "--report",
        str(action_report),
    ]
    for label_set in args.measurement_label_set:
        action_cmd.extend(["--measurement-label-set", label_set])
    run_command(action_cmd)
    source_hashes_after = database_hashes(args.ontology_db, args.analytics_db)
    source_hashes_match = source_hashes_before == source_hashes_after

    summary = collect_rehearsal_summary(paths["analytics_db"], paths["ontology_db"], args.label_set, args.source_instance)
    summary["source_copy_boundary_counts"] = collect_source_copy_boundary_counts(
        args.ontology_db,
        paths["ontology_db"],
        args.label_set,
        args.source_instance,
    )
    write_rehearsal_report(
        paths["out_dir"] / f"{args.prefix}.summary.md",
        source_instance=args.source_instance,
        source_ontology_db=args.ontology_db,
        source_analytics_db=args.analytics_db,
        copied_ontology_db=paths["ontology_db"],
        copied_analytics_db=paths["analytics_db"],
        labels=args.labels,
        review_report=review_report,
        action_report=action_report,
        queue_path=queue_path,
        queue_report=queue_report,
        preflight_report=preflight_report,
        label_summary=label_summary,
        source_hashes_before=source_hashes_before,
        source_hashes_after=source_hashes_after,
        source_hashes_match=source_hashes_match,
        summary=summary,
    )
    if not source_hashes_match:
        raise SystemExit("source database hash changed during rehearsal; inspect summary before using results")


def prepare_rehearsal_paths(out_dir: Path, prefix: str, ontology_db: Path, analytics_db: Path) -> dict[str, Path]:
    out_dir.mkdir(parents=True, exist_ok=True)
    copied_ontology = out_dir / f"{prefix}.{ontology_db.name}"
    copied_analytics = out_dir / f"{prefix}.{analytics_db.name}"
    for source, destination in [(ontology_db, copied_ontology), (analytics_db, copied_analytics)]:
        if source.resolve() == destination.resolve():
            raise SystemExit(f"refusing to overwrite source database during rehearsal: {source}")
    return {"out_dir": out_dir, "ontology_db": copied_ontology, "analytics_db": copied_analytics}


def copy_database(source: Path, destination: Path) -> None:
    if not source.exists():
        raise SystemExit(f"database not found: {source}")
    destination.parent.mkdir(parents=True, exist_ok=True)
    if destination.exists():
        destination.unlink()
    with sqlite3.connect(f"file:{source}?mode=ro", uri=True) as source_conn:
        with sqlite3.connect(destination) as destination_conn:
            source_conn.backup(destination_conn)


def mark_rehearsal_copy(
    db_path: Path,
    *,
    source_db: Path,
    db_role: str,
    labels: Path,
    source_instance: str,
    label_set: str,
    prefix: str,
    reviewed_at: str,
) -> None:
    created_at = datetime.now(timezone.utc).isoformat()
    rows = {
        "rehearsal_only": "true",
        "warning": "This SQLite database is a copied label rehearsal artifact. Do not use copied rows as product truth.",
        "db_role": db_role,
        "source_db": str(source_db),
        "labels": str(labels),
        "source_instance": source_instance,
        "label_set": label_set,
        "prefix": prefix,
        "reviewed_at": reviewed_at,
        "created_at": created_at,
    }
    with sqlite3.connect(db_path) as conn:
        conn.execute(
            """
            create table if not exists cubicle_rehearsal_manifest (
              key text primary key,
              value text not null
            )
            """
        )
        conn.execute("delete from cubicle_rehearsal_manifest")
        conn.executemany(
            "insert into cubicle_rehearsal_manifest (key, value) values (?, ?)",
            sorted(rows.items()),
        )
        conn.commit()


def database_hashes(ontology_db: Path, analytics_db: Path) -> dict[str, str]:
    return {
        "ontology_db": file_sha256(ontology_db),
        "ontology_db_wal": sidecar_file_sha256_or_state(ontology_db.with_name(ontology_db.name + "-wal")),
        "ontology_db_shm": sidecar_file_sha256_or_state(ontology_db.with_name(ontology_db.name + "-shm")),
        "analytics_db": file_sha256(analytics_db),
        "analytics_db_wal": sidecar_file_sha256_or_state(analytics_db.with_name(analytics_db.name + "-wal")),
        "analytics_db_shm": sidecar_file_sha256_or_state(analytics_db.with_name(analytics_db.name + "-shm")),
    }


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def sidecar_file_sha256_or_state(path: Path) -> str:
    if not path.exists():
        return "absent"
    if path.stat().st_size == 0:
        return "empty"
    return file_sha256(path)


def summarize_label_file(path: Path) -> dict[str, Any]:
    if not path.exists():
        raise SystemExit(f"label file not found: {path}")
    sep = "\t" if path.suffix.lower() in {".tsv", ".tab"} else ","
    with path.open(newline="") as handle:
        rows = list(csv.DictReader(handle, delimiter=sep))
    summary: dict[str, Any] = {
        "row_count": len(rows),
        "importable_label_count": 0,
        "blank_or_unknown_label_count": 0,
        "missing_insight_key_count": 0,
        "invalid_rows": [],
        "insight_kind_counts": {},
        "truth_label_counts": {},
        "actionability_label_counts": {},
    }
    for index, row in enumerate(rows, start=2):
        insight_key = clean_text(row.get("insight_key"))
        if not insight_key:
            summary["missing_insight_key_count"] += 1
            continue
        insight_kind = clean_text(row.get("insight_kind")) or "unknown"
        truth_label = clean_text(row.get("truth_label") or row.get("gold_truth_label")) or "unknown"
        actionability_label = clean_text(row.get("actionability_label") or row.get("gold_actionability_label")) or "unknown"
        review_state = clean_text(row.get("review_state") or row.get("gold_review_state"))
        increment_count(summary["insight_kind_counts"], insight_kind)
        increment_count(summary["truth_label_counts"], truth_label)
        increment_count(summary["actionability_label_counts"], actionability_label)
        invalid: list[str] = []
        if truth_label not in TRUTH_LABELS:
            invalid.append(f"truth_label={truth_label!r}")
        if actionability_label not in ACTIONABILITY_LABELS:
            invalid.append(f"actionability_label={actionability_label!r}")
        if review_state not in REVIEW_STATES:
            invalid.append(f"review_state={review_state!r}")
        if invalid:
            summary["invalid_rows"].append({"line": index, "insight_key": insight_key, "errors": ", ".join(invalid)})
            continue
        if truth_label == "unknown" and actionability_label == "unknown":
            summary["blank_or_unknown_label_count"] += 1
            continue
        summary["importable_label_count"] += 1
    return summary


def validate_label_preflight(summary: dict[str, Any], report_path: Path) -> None:
    invalid_rows = summary.get("invalid_rows") or []
    if invalid_rows:
        raise SystemExit(f"label preflight failed: {len(invalid_rows)} invalid row(s); see {report_path}")
    if int(summary.get("importable_label_count") or 0) == 0:
        raise SystemExit(f"label preflight failed: no importable gold labels; fill gold_* columns before rehearsal. See {report_path}")


def increment_count(counts: dict[str, int], key: str) -> None:
    counts[key] = int(counts.get(key, 0)) + 1


def clean_text(value: Any) -> str:
    if value is None:
        return ""
    return str(value).strip()


def run_command(command: list[str]) -> None:
    subprocess.run(command, check=True)


def collect_rehearsal_summary(analytics_db: Path, ontology_db: Path, label_set: str, source_instance: str = "") -> dict[str, Any]:
    summary: dict[str, Any] = {}
    with sqlite3.connect(analytics_db) as conn:
        summary["action_summary"] = metric_map(conn, "tpm_action_summary")
        summary["measurement_summary"] = metric_map(conn, "tpm_measurement_label_summary")
        summary["review_metrics"] = scoped_metric_map(conn, "tpm_review_metrics", "all")
        summary["decision_state_counts"] = grouped_counts(conn, "tpm_action_items", "decision_state")
        summary["action_type_counts"] = grouped_counts(conn, "tpm_action_items", "action_type")
    with sqlite3.connect(ontology_db) as conn:
        summary["imported_label_count"] = scalar_int(
            conn,
            """
            select count(*)
              from work_insight_reviews
             where review_kind = 'evaluation_label'
               and label_set = ?
            """,
            (label_set,),
        )
        summary["measurement_eligible_imported_label_count"] = scalar_int(
            conn,
            """
            select count(*)
              from work_insight_reviews
             where review_kind = 'evaluation_label'
               and label_set = ?
               and measurement_eligible = 1
            """,
            (label_set,),
        )
        if table_exists(conn, "work_insights"):
            summary["current_imported_label_count"] = scalar_int(
                conn,
                """
                select count(*)
                  from work_insight_reviews wir
                  join work_insights wi on wi.id = wir.work_insight_id
                 where wir.review_kind = 'evaluation_label'
                   and wir.label_set = ?
                   and wi.producer_state = 'current'
                """,
                (label_set,),
            )
            summary["stale_imported_label_count"] = scalar_int(
                conn,
                """
                select count(*)
                  from work_insight_reviews wir
                  join work_insights wi on wi.id = wir.work_insight_id
                 where wir.review_kind = 'evaluation_label'
                   and wir.label_set = ?
                   and wi.producer_state != 'current'
                """,
                (label_set,),
            )
        else:
            summary["current_imported_label_count"] = summary["imported_label_count"]
            summary["stale_imported_label_count"] = 0
        summary["work_blocker_count"] = source_scoped_count(conn, "work_blockers", source_instance)
        summary["active_work_blocker_count"] = source_scoped_count(conn, "work_blockers", source_instance, "blocker_state = 'active'")
        summary["work_dependency_edge_counts"] = source_scoped_grouped_counts(conn, "work_dependency_edges", "edge_kind", source_instance)
        summary["rehearsal_manifest"] = key_value_table(conn, "cubicle_rehearsal_manifest")
    return summary


def metric_map(conn: sqlite3.Connection, table_name: str) -> dict[str, str]:
    if not table_exists(conn, table_name):
        return {}
    rows = conn.execute(f"select metric, value from {table_name}").fetchall()
    return {str(metric): str(value) for metric, value in rows}


def scoped_metric_map(conn: sqlite3.Connection, table_name: str, scope: str) -> dict[str, str]:
    if not table_exists(conn, table_name):
        return {}
    rows = conn.execute(f"select metric, value from {table_name} where scope = ?", (scope,)).fetchall()
    return {str(metric): str(value) for metric, value in rows}


def grouped_counts(conn: sqlite3.Connection, table_name: str, column: str) -> dict[str, int]:
    if not table_exists(conn, table_name):
        return {}
    rows = conn.execute(f"select {column}, count(*) from {table_name} group by {column} order by {column}").fetchall()
    return {str(key or ""): int(count) for key, count in rows}


def source_scoped_count(conn: sqlite3.Connection, table_name: str, source_instance: str, where_clause: str = "") -> int:
    if not table_exists(conn, table_name):
        return 0
    clauses: list[str] = []
    params: list[Any] = []
    if source_instance and column_exists(conn, table_name, "source_instance"):
        clauses.append("source_instance = ?")
        params.append(source_instance)
    if where_clause:
        clauses.append(where_clause)
    query = f"select count(*) from {table_name}"
    if clauses:
        query += " where " + " and ".join(clauses)
    return scalar_int(conn, query, tuple(params))


def source_scoped_grouped_counts(conn: sqlite3.Connection, table_name: str, column: str, source_instance: str) -> dict[str, int]:
    if not table_exists(conn, table_name):
        return {}
    params: list[Any] = []
    query = f"select {column}, count(*) from {table_name}"
    if source_instance and column_exists(conn, table_name, "source_instance"):
        query += " where source_instance = ?"
        params.append(source_instance)
    query += f" group by {column} order by {column}"
    rows = conn.execute(query, tuple(params)).fetchall()
    return {str(key or ""): int(count) for key, count in rows}


def key_value_table(conn: sqlite3.Connection, table_name: str) -> dict[str, str]:
    if not table_exists(conn, table_name):
        return {}
    rows = conn.execute(f"select key, value from {table_name}").fetchall()
    return {str(key): str(value) for key, value in rows}


def collect_source_copy_boundary_counts(source_ontology_db: Path, copied_ontology_db: Path, label_set: str, source_instance: str) -> dict[str, dict[str, int]]:
    with sqlite3.connect(source_ontology_db) as source_conn:
        source_counts = boundary_counts(source_conn, label_set, source_instance)
    with sqlite3.connect(copied_ontology_db) as copy_conn:
        copy_counts = boundary_counts(copy_conn, label_set, source_instance)
    return {"source": source_counts, "copy": copy_counts}


def boundary_counts(conn: sqlite3.Connection, label_set: str, source_instance: str) -> dict[str, int]:
    blocked_by = source_scoped_count(conn, "work_dependency_edges", source_instance, "edge_kind = 'blocked_by'")
    needs_action = source_scoped_count(conn, "work_dependency_edges", source_instance, "edge_kind = 'needs_action'")
    return {
        "label_set_reviews": label_set_review_count(conn, label_set),
        "measurement_eligible_label_set_reviews": label_set_review_count(conn, label_set, measurement_eligible=True),
        "work_blockers": source_scoped_count(conn, "work_blockers", source_instance),
        "active_work_blockers": source_scoped_count(conn, "work_blockers", source_instance, "blocker_state = 'active'"),
        "blocked_by_edges": blocked_by,
        "needs_action_edges": needs_action,
    }


def label_set_review_count(conn: sqlite3.Connection, label_set: str, *, measurement_eligible: bool = False) -> int:
    if not table_exists(conn, "work_insight_reviews"):
        return 0
    clauses = ["review_kind = 'evaluation_label'", "label_set = ?"]
    params: list[Any] = [label_set]
    if measurement_eligible:
        clauses.append("measurement_eligible = 1")
    return scalar_int(conn, "select count(*) from work_insight_reviews where " + " and ".join(clauses), tuple(params))


def scalar_int(conn: sqlite3.Connection, query: str, params: tuple[Any, ...] = ()) -> int:
    row = conn.execute(query, params).fetchone()
    return int(row[0] or 0) if row else 0


def table_exists(conn: sqlite3.Connection, table_name: str) -> bool:
    row = conn.execute("select 1 from sqlite_master where type = 'table' and name = ?", (table_name,)).fetchone()
    return row is not None


def column_exists(conn: sqlite3.Connection, table_name: str, column_name: str) -> bool:
    if not table_exists(conn, table_name):
        return False
    return any(str(row[1]) == column_name for row in conn.execute(f"pragma table_info({table_name})"))


def write_rehearsal_report(
    path: Path,
    *,
    source_instance: str,
    source_ontology_db: Path,
    source_analytics_db: Path,
    copied_ontology_db: Path,
    copied_analytics_db: Path,
    labels: Path,
    review_report: Path,
    action_report: Path,
    queue_path: Path,
    queue_report: Path,
    preflight_report: Path,
    label_summary: dict[str, Any],
    source_hashes_before: dict[str, str],
    source_hashes_after: dict[str, str],
    source_hashes_match: bool,
    summary: dict[str, Any],
) -> None:
    action_summary = summary.get("action_summary", {})
    measurement_summary = summary.get("measurement_summary", {})
    review_metrics = summary.get("review_metrics", {})
    rehearsal_manifest = summary.get("rehearsal_manifest", {})
    source_copy_boundary_counts = summary.get("source_copy_boundary_counts", {})
    lines = [
        "# TPM Label Rehearsal",
        "",
        f"Source instance: {source_instance}",
        "",
        "## Inputs",
        "",
        f"- Source ontology DB: `{source_ontology_db}`",
        f"- Source analytics DB: `{source_analytics_db}`",
        f"- Label file: `{labels}`",
        f"- Label preflight: `{preflight_report}`",
        "",
        "## Rehearsal Copies",
        "",
        f"- Copied ontology DB: `{copied_ontology_db}`",
        f"- Copied analytics DB: `{copied_analytics_db}`",
        "- The source databases are not written by this rehearsal.",
        f"- Source DB hash check: {'unchanged' if source_hashes_match else 'changed'}",
        f"- Source ontology DB SHA-256 before: `{source_hashes_before.get('ontology_db', '')}`",
        f"- Source ontology DB SHA-256 after: `{source_hashes_after.get('ontology_db', '')}`",
        f"- Source ontology WAL state before: `{source_hashes_before.get('ontology_db_wal', '')}`",
        f"- Source ontology WAL state after: `{source_hashes_after.get('ontology_db_wal', '')}`",
        f"- Source ontology SHM state before: `{source_hashes_before.get('ontology_db_shm', '')}`",
        f"- Source ontology SHM state after: `{source_hashes_after.get('ontology_db_shm', '')}`",
        f"- Source analytics DB SHA-256 before: `{source_hashes_before.get('analytics_db', '')}`",
        f"- Source analytics DB SHA-256 after: `{source_hashes_after.get('analytics_db', '')}`",
        f"- Source analytics WAL state before: `{source_hashes_before.get('analytics_db_wal', '')}`",
        f"- Source analytics WAL state after: `{source_hashes_after.get('analytics_db_wal', '')}`",
        f"- Source analytics SHM state before: `{source_hashes_before.get('analytics_db_shm', '')}`",
        f"- Source analytics SHM state after: `{source_hashes_after.get('analytics_db_shm', '')}`",
        "",
        "## Safety Boundary",
        "",
        "- Rehearsal output is copied-DB evidence only.",
        "- Do not treat copied work blockers, dependency edges, or synthetic labels as product truth.",
        "- Production promotion requires a real reviewed label file and a separate approved import into the source ontology DB.",
        f"- Copied DB manifest marker: `rehearsal_only={rehearsal_manifest.get('rehearsal_only', 'missing')}`",
        f"- Copied DB manifest warning: `{rehearsal_manifest.get('warning', 'missing')}`",
        "",
        "## Label Preflight",
        "",
        f"- Label rows: {label_summary.get('row_count', 0)}",
        f"- Importable label rows: {label_summary.get('importable_label_count', 0)}",
        f"- Blank/unknown rows: {label_summary.get('blank_or_unknown_label_count', 0)}",
        f"- Missing insight key rows: {label_summary.get('missing_insight_key_count', 0)}",
        "",
        "## Outcome",
        "",
        f"- Imported label rows for this label set: {summary.get('imported_label_count', 0)}",
        f"- Current imported label rows: {summary.get('current_imported_label_count', 0)}",
        f"- Stale/non-current imported label rows: {summary.get('stale_imported_label_count', 0)}",
        f"- Measurement-eligible imported label rows: {summary.get('measurement_eligible_imported_label_count', 0)}",
        f"- Work blocker rows: {summary.get('work_blocker_count', 0)}",
        f"- Active work blocker rows: {summary.get('active_work_blocker_count', 0)}",
        f"- Measurement labels now in analytics summary: {measurement_summary.get('measurement_label_count', '0')}",
        f"- Precision readiness: {review_metrics.get('ready_to_measure_precision', 'false')}",
        f"- Actionability readiness: {review_metrics.get('ready_to_measure_actionability', 'false')}",
        f"- Product action count: {action_summary.get('open_work_count', '0')}",
        f"- Validation lead count: {action_summary.get('validation_lead_count', '0')}",
        f"- Source repair count: {action_summary.get('source_repair_count', '0')}",
        "",
        "## Decision States",
        "",
        markdown_counts(summary.get("decision_state_counts", {})),
        "",
        "## Action Types",
        "",
        markdown_counts(summary.get("action_type_counts", {})),
        "",
        "## Dependency Edges",
        "",
        markdown_counts(summary.get("work_dependency_edge_counts", {})),
        "",
        "## Source vs Copy Boundary Counts",
        "",
        source_copy_boundary_markdown(source_copy_boundary_counts),
        "",
        "## Generated Reports",
        "",
        f"- Review evaluation: `{review_report}`",
        f"- Action brief: `{action_report}`",
        f"- Remaining measurement queue: `{queue_path}`",
        f"- Remaining measurement queue report: `{queue_report}`",
    ]
    path.write_text("\n".join(lines) + "\n")


def write_label_preflight_report(
    path: Path,
    labels: Path,
    summary: dict[str, Any],
    source_hashes_before: dict[str, str],
) -> None:
    invalid_rows = summary.get("invalid_rows") or []
    lines = [
        "# TPM Label Rehearsal Preflight",
        "",
        f"Label file: `{labels}`",
        "",
        "## Source Hashes Before Rehearsal",
        "",
        f"- Ontology DB SHA-256: `{source_hashes_before.get('ontology_db', '')}`",
        f"- Analytics DB SHA-256: `{source_hashes_before.get('analytics_db', '')}`",
        "",
        "## Label Readiness",
        "",
        f"- Label rows: {summary.get('row_count', 0)}",
        f"- Importable label rows: {summary.get('importable_label_count', 0)}",
        f"- Blank/unknown rows: {summary.get('blank_or_unknown_label_count', 0)}",
        f"- Missing insight key rows: {summary.get('missing_insight_key_count', 0)}",
        f"- Invalid rows: {len(invalid_rows)}",
        "",
        "## Insight Kinds",
        "",
        markdown_counts(summary.get("insight_kind_counts", {})),
        "",
        "## Truth Labels",
        "",
        markdown_counts(summary.get("truth_label_counts", {})),
        "",
        "## Actionability Labels",
        "",
        markdown_counts(summary.get("actionability_label_counts", {})),
    ]
    if invalid_rows:
        lines.extend(["", "## Invalid Rows", "", "| line | insight_key | errors |", "| --- | --- | --- |"])
        for row in invalid_rows:
            lines.append(f"| {row.get('line', '')} | {row.get('insight_key', '')} | {row.get('errors', '')} |")
    path.write_text("\n".join(lines) + "\n")


def markdown_counts(counts: dict[str, int]) -> str:
    if not counts:
        return "No rows."
    lines = ["| key | count |", "| --- | --- |"]
    for key, count in sorted(counts.items()):
        lines.append(f"| {key} | {count} |")
    return "\n".join(lines)


def source_copy_boundary_markdown(counts: dict[str, dict[str, int]]) -> str:
    source = counts.get("source", {}) if isinstance(counts, dict) else {}
    copy = counts.get("copy", {}) if isinstance(counts, dict) else {}
    keys = sorted(set(source.keys()).union(copy.keys()))
    if not keys:
        return "No rows."
    lines = ["| metric | source | copy |", "| --- | ---: | ---: |"]
    for key in keys:
        lines.append(f"| {key} | {int(source.get(key, 0))} | {int(copy.get(key, 0))} |")
    return "\n".join(lines)


if __name__ == "__main__":
    main()
