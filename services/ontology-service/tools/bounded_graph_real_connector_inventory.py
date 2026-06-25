#!/usr/bin/env python3
"""Inventory real connector DBs for bounded graph promotion candidates."""

from __future__ import annotations

import argparse
import glob
import json
import sqlite3
from pathlib import Path
from typing import Any

import bounded_graph_probe_candidates


DEFAULT_DATA_ROOT = Path("/Users/harsh/workspace/data")
PRODUCT_ACL_TABLES = [
    "tickets",
    "pull_requests",
    "ticket_pull_requests",
    "documents",
    "messages",
    "persons",
    "open_graph_objects",
    "open_graph_associations",
    "work_dependency_edges",
]
EVIDENCE_ACL_TABLES = ["evidences"]
NON_PUBLIC_VISIBILITIES = ("private", "restricted", "team")
PRODUCTION_CONNECTOR_KINDS = (
    "confluence_cloud",
    "github_app",
    "github_rest_api",
    "google_drive_api",
    "jira_cloud",
    "linear_api",
    "notion_api",
    "pypi_json_api",
    "slack_api",
)


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Find real connector bounded graph candidates across ontology DBs.")
    parser.add_argument("--data-root", type=Path, default=DEFAULT_DATA_ROOT)
    parser.add_argument("--database", action="append", type=Path, default=[])
    parser.add_argument("--database-glob", action="append", default=[])
    parser.add_argument("--limit-per-db", type=int, default=5)
    parser.add_argument("--max-dbs", type=int, default=0, help="Optional cap after sorting discovered DB paths.")
    parser.add_argument("--report-json", type=Path)
    parser.add_argument("--report-md", type=Path)
    parser.add_argument("--require-real-non-flink", action="store_true")
    parser.add_argument("--require-product-acl-row", action="store_true")
    parser.add_argument("--require-real-acl-ingestion", action="store_true")
    parser.add_argument("--require-source-scope-negative-row", action="store_true")
    parser.add_argument("--require-real-source-scope-negative", action="store_true")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    report = build_inventory(
        data_root=args.data_root,
        explicit_databases=args.database,
        database_globs=args.database_glob,
        limit_per_db=max(args.limit_per_db, 1),
        max_dbs=max(args.max_dbs, 0),
    )
    if args.report_json:
        args.report_json.parent.mkdir(parents=True, exist_ok=True)
        args.report_json.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    if args.report_md:
        args.report_md.parent.mkdir(parents=True, exist_ok=True)
        args.report_md.write_text(render_markdown(report), encoding="utf-8")
    if not args.report_json and not args.report_md:
        print(json.dumps(report, indent=2, sort_keys=True))
    if args.require_real_non_flink and not report["passes_real_non_flink_requirement"]:
        raise SystemExit("no real non-Flink connector-backed bounded graph candidate found")
    if args.require_product_acl_row and not report["passes_product_acl_row_requirement"]:
        raise SystemExit("no product-row non-public current ACL ingestion found")
    if args.require_real_acl_ingestion and not report["passes_real_acl_ingestion_requirement"]:
        raise SystemExit("no real connector product-row non-public current ACL ingestion found")
    if args.require_source_scope_negative_row and not report["passes_source_scope_negative_row_requirement"]:
        raise SystemExit("no stale or not-attempted source-scope state row found")
    if args.require_real_source_scope_negative and not report["passes_real_source_scope_negative_requirement"]:
        raise SystemExit("no real connector graph candidate with stale or not-attempted source-scope state found")


def build_inventory(
    *,
    data_root: Path,
    explicit_databases: list[Path],
    database_globs: list[str],
    limit_per_db: int,
    max_dbs: int = 0,
) -> dict[str, Any]:
    databases = discover_databases(data_root, explicit_databases, database_globs)
    if max_dbs:
        databases = databases[:max_dbs]
    db_reports = [scan_database(path, limit_per_db=limit_per_db) for path in databases]
    candidate_reports = [row for row in db_reports if row.get("scan_status") == "ok"]
    real_non_flink = [row for row in candidate_reports if row["candidate_kind"] == "real_non_flink_candidate"]
    flink_shaped = [row for row in candidate_reports if row["candidate_kind"] == "flink_shaped_candidate"]
    open_graph = [row for row in candidate_reports if row.get("open_graph_object_count", 0) > 0]
    product_acl_row_databases = [
        row
        for row in candidate_reports
        if row.get("product_acl_current_nonpublic_count", 0) > 0
    ]
    real_acl_databases = [
        row
        for row in product_acl_row_databases
        if row.get("production_acl_candidate_count", 0) > 0
    ]
    source_backed_acl_candidate_databases = [
        row
        for row in candidate_reports
        if row.get("real_acl_candidate_source_backed_count", 0) > 0
    ]
    source_scope_negative_row_databases = [
        row
        for row in candidate_reports
        if row.get("source_scope_stale_or_not_attempted_count", 0) > 0
    ]
    real_stale_or_not_attempted_source_scope_databases = [
        row
        for row in source_scope_negative_row_databases
        if row.get("production_source_scope_negative_candidate_count", 0) > 0
    ]
    source_backed_source_scope_negative_candidate_databases = [
        row
        for row in candidate_reports
        if row.get("real_source_scope_negative_candidate_count", 0) > 0
    ]
    return {
        "data_root": str(data_root),
        "database_count": len(databases),
        "scanned_database_count": len(db_reports),
        "ok_database_count": len(candidate_reports),
        "error_database_count": len([row for row in db_reports if row.get("scan_status") != "ok"]),
        "real_non_flink_candidate_count": len(real_non_flink),
        "flink_shaped_candidate_count": len(flink_shaped),
        "open_graph_database_count": len(open_graph),
        "product_acl_current_nonpublic_database_count": len(product_acl_row_databases),
        "source_backed_acl_candidate_database_count": len(source_backed_acl_candidate_databases),
        "real_acl_ingestion_database_count": len(real_acl_databases),
        "source_scope_negative_row_database_count": len(source_scope_negative_row_databases),
        "source_backed_source_scope_negative_candidate_database_count": len(
            source_backed_source_scope_negative_candidate_databases
        ),
        "source_scope_stale_or_not_attempted_database_count": len(real_stale_or_not_attempted_source_scope_databases),
        "production_source_connection_count": sum(
            int(row.get("production_source_connection_count") or 0)
            for row in candidate_reports
        ),
        "source_connection_connector_kind_counts": merge_count_maps(
            row.get("source_connection_connector_kind_counts", {})
            for row in candidate_reports
        ),
        "production_connector_kinds": list(PRODUCTION_CONNECTOR_KINDS),
        "passes_real_non_flink_requirement": bool(real_non_flink),
        "passes_product_acl_row_requirement": bool(product_acl_row_databases),
        "passes_real_acl_ingestion_requirement": bool(real_acl_databases),
        "passes_source_scope_negative_row_requirement": bool(source_scope_negative_row_databases),
        "passes_real_source_scope_negative_requirement": bool(real_stale_or_not_attempted_source_scope_databases),
        "databases": db_reports,
    }


def discover_databases(data_root: Path, explicit_databases: list[Path], database_globs: list[str]) -> list[Path]:
    paths: set[Path] = {path.expanduser().resolve() for path in explicit_databases}
    for pattern in database_globs:
        paths.update(Path(path).expanduser().resolve() for path in glob.glob(pattern))
    if not paths and data_root.exists():
        paths.update(path.resolve() for path in data_root.rglob("*.db"))
    return sorted(paths, key=lambda path: str(path))


def merge_count_maps(values: Any) -> dict[str, int]:
    merged: dict[str, int] = {}
    for value in values:
        if not isinstance(value, dict):
            continue
        for key, count in value.items():
            merged[str(key)] = merged.get(str(key), 0) + int(count or 0)
    return dict(sorted(merged.items()))


def scan_database(path: Path, *, limit_per_db: int) -> dict[str, Any]:
    try:
        report = bounded_graph_probe_candidates.build_report(path, limit=limit_per_db)
    except (OSError, sqlite3.DatabaseError, KeyError, ValueError) as exc:
        return {
            "database": str(path),
            "scan_status": "error",
            "error": str(exc),
        }
    signal = report.get("pigeonhole_signal", {})
    candidate_kind = classify_candidate(signal)
    product_safe = scan_product_safe_signals(path)
    return {
        "database": str(path),
        "scan_status": "ok",
        "candidate_kind": candidate_kind,
        "assessment": signal.get("assessment", ""),
        "source_instances": signal.get("source_instances", []),
        "non_flink_source_instances": signal.get("non_flink_source_instances", []),
        "open_graph_object_count": signal.get("open_graph_object_count", 0),
        "promotable_relationship_count": signal.get("promotable_relationship_count", 0),
        **product_safe,
        "object_counts": report.get("object_counts", {}),
        "top_probe_candidates": report.get("top_probe_candidates", [])[:limit_per_db],
    }


def classify_candidate(signal: dict[str, Any]) -> str:
    non_flink = [value for value in signal.get("non_flink_source_instances", []) if value]
    promotable = int(signal.get("promotable_relationship_count") or 0)
    if non_flink and promotable > 0:
        return "real_non_flink_candidate"
    if non_flink:
        return "non_flink_no_promotable_relationship"
    if promotable > 0:
        return "flink_shaped_candidate"
    return "no_promotable_candidate"


def scan_product_safe_signals(path: Path) -> dict[str, Any]:
    with sqlite3.connect(path) as conn:
        tables = table_names(conn)
        columns_by_table = {table: column_names(conn, table) for table in tables}
        product_acl = scan_acl_tables(conn, tables, PRODUCT_ACL_TABLES)
        evidence_acl = scan_acl_tables(conn, tables, EVIDENCE_ACL_TABLES)
        source_scope = scan_source_scope_states(conn, tables)
        source_connections = scan_source_connections(conn, tables, columns_by_table)
        source_backed_acl_candidate_count = scan_real_acl_candidate_rows(
            conn,
            tables,
            columns_by_table,
            require_production_connector=False,
        )
        production_acl_candidate_count = scan_real_acl_candidate_rows(
            conn,
            tables,
            columns_by_table,
            require_production_connector=True,
        )
        source_backed_source_scope_negative_candidate_count = scan_real_source_scope_negative_candidates(
            conn,
            tables,
            columns_by_table,
            require_production_connector=False,
        )
        production_source_scope_negative_candidate_count = scan_real_source_scope_negative_candidates(
            conn,
            tables,
            columns_by_table,
            require_production_connector=True,
        )
    return {
        "source_connection_count": source_connections["count"],
        "source_connection_connector_kind_counts": source_connections["connector_kind_counts"],
        "production_source_connection_count": source_connections["production_count"],
        "product_acl_current_nonpublic_count": product_acl["current_nonpublic_count"],
        "product_acl_current_nonpublic_tables": product_acl["current_nonpublic_tables"],
        "real_acl_candidate_source_backed_count": source_backed_acl_candidate_count,
        "production_acl_candidate_count": production_acl_candidate_count,
        "evidence_acl_current_nonpublic_count": evidence_acl["current_nonpublic_count"],
        "evidence_acl_current_nonpublic_tables": evidence_acl["current_nonpublic_tables"],
        "source_scope_state_count": source_scope["state_count"],
        "source_scope_fresh_partial_count": source_scope["fresh_partial_count"],
        "source_scope_stale_or_not_attempted_count": source_scope["stale_or_not_attempted_count"],
        "real_source_scope_negative_candidate_count": source_backed_source_scope_negative_candidate_count,
        "production_source_scope_negative_candidate_count": production_source_scope_negative_candidate_count,
        "source_scope_state_breakdown": source_scope["breakdown"],
    }


def scan_acl_tables(conn: sqlite3.Connection, tables: set[str], candidate_tables: list[str]) -> dict[str, Any]:
    total = 0
    table_counts: dict[str, int] = {}
    for table in candidate_tables:
        if table not in tables:
            continue
        cols = column_names(conn, table)
        if not {"visibility", "acl_state"}.issubset(cols):
            continue
        placeholders = ",".join("?" for _ in NON_PUBLIC_VISIBILITIES)
        query = f"""
            select count(*)
            from {table}
            where visibility in ({placeholders})
              and acl_state = 'current'
        """
        count = int(conn.execute(query, NON_PUBLIC_VISIBILITIES).fetchone()[0] or 0)
        if count:
            table_counts[table] = count
            total += count
    return {
        "current_nonpublic_count": total,
        "current_nonpublic_tables": table_counts,
    }


def scan_source_scope_states(conn: sqlite3.Connection, tables: set[str]) -> dict[str, Any]:
    if "source_scope_states" not in tables:
        return {
            "state_count": 0,
            "fresh_partial_count": 0,
            "stale_or_not_attempted_count": 0,
            "breakdown": [],
        }
    cols = column_names(conn, "source_scope_states")
    required = {"freshness_state", "coverage_mode", "last_attempted_at"}
    if not required.issubset(cols):
        return {
            "state_count": 0,
            "fresh_partial_count": 0,
            "stale_or_not_attempted_count": 0,
            "breakdown": [],
        }
    rows = conn.execute(
        """
        select
          coalesce(freshness_state, ''),
          coalesce(coverage_mode, ''),
          case when last_attempted_at is null then 'not_attempted' else 'attempted' end,
          count(*)
        from source_scope_states
        group by 1, 2, 3
        order by 1, 2, 3
        """
    ).fetchall()
    state_count = sum(int(row[3] or 0) for row in rows)
    fresh_partial_count = sum(
        int(count or 0)
        for freshness, coverage, _, count in rows
        if freshness == "fresh" and coverage == "partial_scope"
    )
    stale_or_not_attempted_count = sum(
        int(count or 0)
        for freshness, _, attempt_state, count in rows
        if freshness != "fresh" or attempt_state == "not_attempted"
    )
    return {
        "state_count": state_count,
        "fresh_partial_count": fresh_partial_count,
        "stale_or_not_attempted_count": stale_or_not_attempted_count,
        "breakdown": [
            {
                "freshness_state": str(freshness),
                "coverage_mode": str(coverage),
                "attempt_state": str(attempt_state),
                "count": int(count or 0),
            }
            for freshness, coverage, attempt_state, count in rows
        ],
    }


def scan_real_acl_candidate_rows(
    conn: sqlite3.Connection,
    tables: set[str],
    columns_by_table: dict[str, set[str]],
    *,
    require_production_connector: bool,
) -> int:
    if not source_connection_identity_available(columns_by_table, require_connector_kind=require_production_connector):
        return 0
    total = 0
    for family in bounded_graph_probe_candidates.RELATIONSHIP_FAMILIES:
        if not relationship_family_available(tables, columns_by_table, family):
            continue
        candidate_condition = relationship_candidate_condition(
            columns_by_table,
            family,
            require_public_visibility=False,
        )
        if not candidate_condition:
            continue
        acl_conditions = [
            acl_source_backed_condition(
                "r",
                columns_by_table[family.table],
                require_production_connector=require_production_connector,
            ),
            acl_source_backed_condition(
                "f",
                columns_by_table[family.from_table],
                require_production_connector=require_production_connector,
            ),
            acl_source_backed_condition(
                "t",
                columns_by_table[family.to_table],
                require_production_connector=require_production_connector,
            ),
        ]
        acl_condition = " or ".join(f"({condition})" for condition in acl_conditions if condition)
        if not acl_condition:
            continue
        total += int(
            conn.execute(
                f"""
                select count(*)
                from {family.table} r
                join {family.from_table} f on f.id = r.{family.from_id_column}
                join {family.to_table} t on t.id = r.{family.to_id_column}
                where {candidate_condition}
                  and ({acl_condition})
                """
            ).fetchone()[0]
            or 0
        )
    return total


def scan_real_source_scope_negative_candidates(
    conn: sqlite3.Connection,
    tables: set[str],
    columns_by_table: dict[str, set[str]],
    *,
    require_production_connector: bool,
) -> int:
    if not source_scope_identity_available(tables, columns_by_table, require_connector_kind=require_production_connector):
        return 0
    total = 0
    for family in bounded_graph_probe_candidates.RELATIONSHIP_FAMILIES:
        if not relationship_family_available(tables, columns_by_table, family):
            continue
        candidate_condition = relationship_candidate_condition(
            columns_by_table,
            family,
            require_public_visibility=True,
        )
        if not candidate_condition:
            continue
        scope_conditions = [
            source_scope_negative_condition(
                "r",
                columns_by_table[family.table],
                require_production_connector=require_production_connector,
            ),
            source_scope_negative_condition(
                "f",
                columns_by_table[family.from_table],
                require_production_connector=require_production_connector,
            ),
            source_scope_negative_condition(
                "t",
                columns_by_table[family.to_table],
                require_production_connector=require_production_connector,
            ),
        ]
        scope_condition = " or ".join(f"({condition})" for condition in scope_conditions if condition)
        if not scope_condition:
            continue
        total += int(
            conn.execute(
                f"""
                select count(*)
                from {family.table} r
                join {family.from_table} f on f.id = r.{family.from_id_column}
                join {family.to_table} t on t.id = r.{family.to_id_column}
                where {candidate_condition}
                  and ({scope_condition})
                """
            ).fetchone()[0]
            or 0
        )
    return total


def relationship_family_available(
    tables: set[str],
    columns_by_table: dict[str, set[str]],
    family: bounded_graph_probe_candidates.RelationshipFamily,
) -> bool:
    if family.table not in tables or family.from_table not in tables or family.to_table not in tables:
        return False
    relation_cols = columns_by_table[family.table]
    from_cols = columns_by_table[family.from_table]
    to_cols = columns_by_table[family.to_table]
    return (
        family.from_id_column in relation_cols
        and family.to_id_column in relation_cols
        and "id" in from_cols
        and "id" in to_cols
    )


def relationship_candidate_condition(
    columns_by_table: dict[str, set[str]],
    family: bounded_graph_probe_candidates.RelationshipFamily,
    *,
    require_public_visibility: bool,
) -> str | None:
    relation_cols = columns_by_table[family.table]
    from_cols = columns_by_table[family.from_table]
    to_cols = columns_by_table[family.to_table]
    required_relation = {"freshness_state", "confidence"}
    required_endpoints = {"freshness_state"}
    if require_public_visibility:
        required_relation.add("visibility")
        required_endpoints.add("visibility")
    if not required_relation.issubset(relation_cols):
        return None
    if not required_endpoints.issubset(from_cols) or not required_endpoints.issubset(to_cols):
        return None
    evidence_terms = []
    if "evidence_count" in relation_cols:
        evidence_terms.append("coalesce(r.evidence_count, 0) > 0")
    if "latest_evidence_id" in relation_cols:
        evidence_terms.append("r.latest_evidence_id is not null")
    if not evidence_terms:
        return None
    terms = [
        "coalesce(r.freshness_state, '') in ('fresh', 'current')",
        "coalesce(f.freshness_state, '') in ('fresh', 'current')",
        "coalesce(t.freshness_state, '') in ('fresh', 'current')",
        "coalesce(r.confidence, 0) >= 1",
        "(" + " or ".join(evidence_terms) + ")",
    ]
    if require_public_visibility:
        terms.extend(
            [
                "coalesce(r.visibility, '') = 'public'",
                "coalesce(f.visibility, '') = 'public'",
                "coalesce(t.visibility, '') = 'public'",
            ]
        )
    return " and ".join(terms)


def scan_source_connections(
    conn: sqlite3.Connection,
    tables: set[str],
    columns_by_table: dict[str, set[str]],
) -> dict[str, Any]:
    if "source_connections" not in tables:
        return {"count": 0, "connector_kind_counts": {}, "production_count": 0}
    cols = columns_by_table.get("source_connections", set())
    count = count_rows(conn, tables, "source_connections")
    if "connector_kind" not in cols:
        return {"count": count, "connector_kind_counts": {"": count} if count else {}, "production_count": 0}
    rows = conn.execute(
        """
        select coalesce(connector_kind, '') as connector_kind, count(*)
        from source_connections
        group by 1
        order by 2 desc, 1
        """
    ).fetchall()
    connector_kind_counts = {str(kind): int(row_count or 0) for kind, row_count in rows}
    production_count = sum(
        row_count
        for kind, row_count in connector_kind_counts.items()
        if is_production_connector_kind(kind)
    )
    return {
        "count": count,
        "connector_kind_counts": connector_kind_counts,
        "production_count": production_count,
    }


def source_connection_identity_available(
    columns_by_table: dict[str, set[str]],
    *,
    require_connector_kind: bool,
) -> bool:
    required = {"source_system", "source_instance"}
    if require_connector_kind:
        required.add("connector_kind")
    return required.issubset(columns_by_table.get("source_connections", set()))


def source_scope_identity_available(
    tables: set[str],
    columns_by_table: dict[str, set[str]],
    *,
    require_connector_kind: bool,
) -> bool:
    connection_required = {"id", "source_system", "source_instance"}
    if require_connector_kind:
        connection_required.add("connector_kind")
    return (
        "source_scope_states" in tables
        and "source_scopes" in tables
        and "source_connections" in tables
        and {"id", "freshness_state", "last_attempted_at", "source_scope_id"}.issubset(
            columns_by_table.get("source_scope_states", set())
        )
        and {"id", "source_connection_id"}.issubset(columns_by_table.get("source_scopes", set()))
        and connection_required.issubset(columns_by_table.get("source_connections", set()))
    )


def acl_source_backed_condition(
    alias: str,
    columns: set[str],
    *,
    require_production_connector: bool,
) -> str | None:
    if not {"visibility", "acl_state", "source_system", "source_instance"}.issubset(columns):
        return None
    visibilities = ", ".join(repr(value) for value in NON_PUBLIC_VISIBILITIES)
    connector_kind_clause = production_connector_clause("sc") if require_production_connector else ""
    return f"""
        coalesce({alias}.visibility, '') in ({visibilities})
        and coalesce({alias}.acl_state, '') = 'current'
        and exists (
            select 1
            from source_connections sc
            where coalesce(sc.source_system, '') = coalesce({alias}.source_system, '')
              and coalesce(sc.source_instance, '') = coalesce({alias}.source_instance, '')
              {connector_kind_clause}
        )
    """


def source_scope_negative_condition(
    alias: str,
    columns: set[str],
    *,
    require_production_connector: bool,
) -> str | None:
    if "source_scope_state_id" not in columns:
        return None
    source_match = ""
    if {"source_system", "source_instance"}.issubset(columns):
        source_match = f"""
            and coalesce(sc.source_system, '') = coalesce({alias}.source_system, '')
            and coalesce(sc.source_instance, '') = coalesce({alias}.source_instance, '')
        """
    connector_kind_clause = production_connector_clause("sc") if require_production_connector else ""
    return f"""
        exists (
            select 1
            from source_scope_states s
            join source_scopes scope on scope.id = s.source_scope_id
            join source_connections sc on sc.id = scope.source_connection_id
            where s.id = {alias}.source_scope_state_id
              and (coalesce(s.freshness_state, '') <> 'fresh' or s.last_attempted_at is null)
              {source_match}
              {connector_kind_clause}
        )
    """


def production_connector_clause(alias: str) -> str:
    allowed = ", ".join(repr(value) for value in PRODUCTION_CONNECTOR_KINDS)
    return f"""
        and lower(coalesce({alias}.connector_kind, '')) in ({allowed})
    """


def is_production_connector_kind(value: str) -> bool:
    value = str(value or "").strip().lower()
    return value in PRODUCTION_CONNECTOR_KINDS


def table_names(conn: sqlite3.Connection) -> set[str]:
    return {
        str(row[0])
        for row in conn.execute("select name from sqlite_master where type='table'")
    }


def column_names(conn: sqlite3.Connection, table: str) -> set[str]:
    return {
        str(row[1])
        for row in conn.execute(f"pragma table_info({table})")
    }


def count_rows(conn: sqlite3.Connection, tables: set[str], table: str) -> int:
    if table not in tables:
        return 0
    return int(conn.execute(f"select count(*) from {table}").fetchone()[0] or 0)


def render_markdown(report: dict[str, Any]) -> str:
    lines = [
        "# Real Connector Bounded Graph Inventory",
        "",
        f"Data root: `{report['data_root']}`",
        "",
        "## Summary",
        "",
        f"- databases discovered: {report['database_count']}",
        f"- databases scanned successfully: {report['ok_database_count']}",
        f"- scan errors: {report['error_database_count']}",
        f"- real non-Flink connector candidates: {report['real_non_flink_candidate_count']}",
        f"- Flink-shaped candidates: {report['flink_shaped_candidate_count']}",
        f"- DBs with open graph rows: {report['open_graph_database_count']}",
        f"- DBs with product-row non-public current ACL: {report['product_acl_current_nonpublic_database_count']}",
        f"- DBs with source-backed candidate ACL rows: {report['source_backed_acl_candidate_database_count']}",
        f"- DBs with production-like connector-backed product ACL ingestion: {report['real_acl_ingestion_database_count']}",
        f"- DBs with stale or not-attempted source-scope rows: {report['source_scope_negative_row_database_count']}",
        f"- DBs with source-backed source-scope negative candidate rows: {report['source_backed_source_scope_negative_candidate_database_count']}",
        f"- real connector candidate DBs with stale or not-attempted source-scope rows: {report['source_scope_stale_or_not_attempted_database_count']}",
        f"- source connection connector kinds: {json.dumps(report['source_connection_connector_kind_counts'], sort_keys=True)}",
        f"- accepted production connector kinds: {json.dumps(report['production_connector_kinds'], sort_keys=True)}",
        f"- production-like source connections: {report['production_source_connection_count']}",
        f"- passes real non-Flink requirement: {str(report['passes_real_non_flink_requirement']).lower()}",
        f"- passes product ACL row requirement: {str(report['passes_product_acl_row_requirement']).lower()}",
        f"- passes real ACL ingestion requirement: {str(report['passes_real_acl_ingestion_requirement']).lower()}",
        f"- passes source-scope negative row requirement: {str(report['passes_source_scope_negative_row_requirement']).lower()}",
        f"- passes real source-scope negative requirement: {str(report['passes_real_source_scope_negative_requirement']).lower()}",
        "",
        "## Databases",
        "",
    ]
    for row in report["databases"]:
        lines.append(f"### `{row['database']}`")
        if row.get("scan_status") != "ok":
            lines.append("")
            lines.append(f"- scan status: error")
            lines.append(f"- error: {row.get('error', '')}")
            lines.append("")
            continue
        lines.extend([
            "",
            f"- candidate kind: `{row['candidate_kind']}`",
            f"- assessment: {row['assessment']}",
            f"- source instances: {', '.join(row['source_instances']) or 'none'}",
            f"- non-Flink source instances: {', '.join(row['non_flink_source_instances']) or 'none'}",
            f"- open graph objects: {row['open_graph_object_count']}",
            f"- promotable relationships: {row['promotable_relationship_count']}",
            f"- source connections: {row['source_connection_count']}",
            f"- source connection connector kinds: {json.dumps(row['source_connection_connector_kind_counts'], sort_keys=True)}",
            f"- production-like source connections: {row['production_source_connection_count']}",
            f"- product-row non-public current ACL rows: {row['product_acl_current_nonpublic_count']}",
            f"- real source-backed candidate ACL rows: {row['real_acl_candidate_source_backed_count']}",
            f"- production-like candidate ACL rows: {row['production_acl_candidate_count']}",
            f"- evidence non-public current ACL rows: {row['evidence_acl_current_nonpublic_count']}",
            f"- source-scope states: {row['source_scope_state_count']}",
            f"- source-scope stale or not-attempted states: {row['source_scope_stale_or_not_attempted_count']}",
            f"- real source-scope negative candidate rows: {row['real_source_scope_negative_candidate_count']}",
            f"- production-like source-scope negative candidate rows: {row['production_source_scope_negative_candidate_count']}",
        ])
        if row.get("product_acl_current_nonpublic_tables"):
            lines.append(f"- product ACL tables: {json.dumps(row['product_acl_current_nonpublic_tables'], sort_keys=True)}")
        if row.get("source_scope_state_breakdown"):
            lines.append(f"- source-scope breakdown: {json.dumps(row['source_scope_state_breakdown'], sort_keys=True)}")
        candidates = row.get("top_probe_candidates", [])
        if candidates:
            lines.append("- top probes:")
            for candidate in candidates[:3]:
                lines.append(
                    "  - `{start_object_type}` `{start_key}` via `{association_type}`: "
                    "{promotable_association_count}/{association_count} promotable".format(**candidate)
                )
        lines.append("")
    return "\n".join(lines)


if __name__ == "__main__":
    main()
