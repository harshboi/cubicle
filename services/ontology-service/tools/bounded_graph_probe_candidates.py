#!/usr/bin/env python3
"""Find useful boundedGraphContext probe candidates in an ontology SQLite DB.

This is a planning/audit helper, not a product read path. It scans typed Ent and
OpenGraph relationship rows, estimates which seeds have claimable-looking
relationship neighborhoods, and emits runnable probe suggestions.
"""

from __future__ import annotations

import argparse
import json
import sqlite3
from dataclasses import dataclass
from pathlib import Path
from typing import Any


CURRENT_STATES = {"fresh", "current"}
PUBLIC_VISIBILITY = "public"


@dataclass(frozen=True)
class RelationshipFamily:
    table: str
    from_table: str
    from_id_column: str
    from_object_type: str
    to_table: str
    to_id_column: str
    to_object_type: str
    association_type: str
    association_column: str | None = None


RELATIONSHIP_FAMILIES = [
    RelationshipFamily(
        table="ticket_pull_requests",
        from_table="tickets",
        from_id_column="ticket_id",
        from_object_type="ticket",
        to_table="pull_requests",
        to_id_column="pull_request_id",
        to_object_type="pull_request",
        association_type="implemented_by",
    ),
    RelationshipFamily(
        table="ticket_documents",
        from_table="tickets",
        from_id_column="ticket_id",
        from_object_type="ticket",
        to_table="documents",
        to_id_column="document_id",
        to_object_type="document",
        association_type="documented_by",
    ),
    RelationshipFamily(
        table="ticket_messages",
        from_table="tickets",
        from_id_column="ticket_id",
        from_object_type="ticket",
        to_table="messages",
        to_id_column="message_id",
        to_object_type="message",
        association_type="discussed_in",
    ),
    RelationshipFamily(
        table="ticket_assignments",
        from_table="tickets",
        from_id_column="ticket_id",
        from_object_type="ticket",
        to_table="persons",
        to_id_column="person_id",
        to_object_type="person",
        association_type="assignment",
        association_column="assignment_kind",
    ),
    RelationshipFamily(
        table="pull_request_authorships",
        from_table="pull_requests",
        from_id_column="pull_request_id",
        from_object_type="pull_request",
        to_table="persons",
        to_id_column="person_id",
        to_object_type="person",
        association_type="authorship",
        association_column="authorship_kind",
    ),
    RelationshipFamily(
        table="pull_request_reviews",
        from_table="pull_requests",
        from_id_column="pull_request_id",
        from_object_type="pull_request",
        to_table="persons",
        to_id_column="person_id",
        to_object_type="person",
        association_type="review",
        association_column="review_kind",
    ),
    RelationshipFamily(
        table="document_links",
        from_table="documents",
        from_id_column="from_document_id",
        from_object_type="document",
        to_table="documents",
        to_id_column="to_document_id",
        to_object_type="document",
        association_type="links_to",
        association_column="document_link_kind",
    ),
    RelationshipFamily(
        table="open_graph_associations",
        from_table="open_graph_objects",
        from_id_column="from_object_id",
        from_object_type="open_graph",
        to_table="open_graph_objects",
        to_id_column="to_object_id",
        to_object_type="open_graph",
        association_type="open_graph",
        association_column="association_type",
    ),
]


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Audit bounded graph probe candidates in an ontology SQLite DB.")
    parser.add_argument("--database", type=Path, required=True)
    parser.add_argument("--limit", type=int, default=20)
    parser.add_argument("--report-json", type=Path)
    parser.add_argument("--report-md", type=Path)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    report = build_report(args.database, limit=max(args.limit, 1))
    if args.report_json:
        args.report_json.parent.mkdir(parents=True, exist_ok=True)
        args.report_json.write_text(json.dumps(report, indent=2, sort_keys=True), encoding="utf-8")
    if args.report_md:
        args.report_md.parent.mkdir(parents=True, exist_ok=True)
        args.report_md.write_text(render_markdown(report), encoding="utf-8")
    if not args.report_json and not args.report_md:
        print(json.dumps(report, indent=2, sort_keys=True))


def build_report(database: Path, *, limit: int) -> dict[str, Any]:
    with sqlite3.connect(database) as conn:
        conn.row_factory = sqlite3.Row
        tables = table_names(conn)
        object_counts = object_table_counts(conn, tables)
        source_summary = source_instance_summary(conn, tables)
        relationship_summaries = [
            relationship_family_summary(conn, tables, family, limit=limit)
            for family in RELATIONSHIP_FAMILIES
            if family.table in tables and family.from_table in tables and family.to_table in tables
        ]
    candidates = sorted(
        [
            candidate
            for summary in relationship_summaries
            for candidate in summary["top_start_candidates"]
        ],
        key=lambda row: (
            -row["promotable_association_count"],
            -row["association_count"],
            row["start_object_type"],
            row["start_key"],
            row["association_type"],
        ),
    )[:limit]
    candidates_by_family = {
        summary["table"]: summary["top_start_candidates"][: min(limit, 10)]
        for summary in relationship_summaries
        if summary["top_start_candidates"]
    }
    return {
        "database": str(database),
        "object_counts": object_counts,
        "source_summary": source_summary,
        "relationship_summaries": relationship_summaries,
        "top_probe_candidates": candidates,
        "top_probe_candidates_by_family": candidates_by_family,
        "pigeonhole_signal": pigeonhole_signal(object_counts, source_summary, relationship_summaries),
    }


def table_names(conn: sqlite3.Connection) -> set[str]:
    rows = conn.execute("select name from sqlite_master where type = 'table'").fetchall()
    return {str(row["name"]) for row in rows}


def table_count(conn: sqlite3.Connection, table: str) -> int:
    return int(conn.execute(f"select count(*) as c from {table}").fetchone()["c"])


def object_table_counts(conn: sqlite3.Connection, tables: set[str]) -> dict[str, int]:
    object_tables = [
        "tickets",
        "pull_requests",
        "documents",
        "messages",
        "persons",
        "open_graph_objects",
        "open_graph_associations",
        "source_sync_issues",
    ]
    return {table: table_count(conn, table) for table in object_tables if table in tables}


def source_instance_summary(conn: sqlite3.Connection, tables: set[str]) -> dict[str, Any]:
    out: dict[str, Any] = {}
    for table in ["tickets", "pull_requests", "documents", "messages", "open_graph_objects"]:
        if table not in tables or not column_exists(conn, table, "source_system"):
            continue
        rows = conn.execute(
            f"""
            select
              coalesce(source_system, '') as source_system,
              coalesce(source_instance, '') as source_instance,
              coalesce(external_kind, '') as external_kind,
              coalesce(freshness_state, '') as freshness_state,
              count(*) as row_count
            from {table}
            group by source_system, source_instance, external_kind, freshness_state
            order by row_count desc, source_system, source_instance, external_kind, freshness_state
            limit 30
            """
        ).fetchall()
        out[table] = [dict(row) for row in rows]
    return out


def relationship_family_summary(
    conn: sqlite3.Connection,
    tables: set[str],
    family: RelationshipFamily,
    *,
    limit: int,
) -> dict[str, Any]:
    assoc_expr = f"r.{family.association_column}" if family.association_column else f"'{family.association_type}'"
    from_type_expr = object_type_expr("f", family.from_object_type)
    to_type_expr = object_type_expr("t", family.to_object_type)
    rows = conn.execute(
        f"""
        select
          {assoc_expr} as association_type,
          count(*) as association_count,
          sum(case when {promotion_condition()} then 1 else 0 end) as promotable_association_count,
          sum(case when coalesce(f.freshness_state, '') in ('partial', 'stale', 'superseded', 'tombstoned')
                    or coalesce(t.freshness_state, '') in ('partial', 'stale', 'superseded', 'tombstoned')
              then 1 else 0 end) as noncurrent_endpoint_count,
          sum(case when coalesce(r.visibility, '') <> 'public'
                    or coalesce(f.visibility, '') <> 'public'
                    or coalesce(t.visibility, '') <> 'public'
              then 1 else 0 end) as restricted_count
        from {family.table} r
        join {family.from_table} f on f.id = r.{family.from_id_column}
        join {family.to_table} t on t.id = r.{family.to_id_column}
        group by association_type
        order by promotable_association_count desc, association_count desc, association_type
        """
    ).fetchall()
    top = conn.execute(
        f"""
        select
          {from_type_expr} as start_object_type,
          f.key as start_key,
          {assoc_expr} as association_type,
          count(*) as association_count,
          sum(case when {promotion_condition()} then 1 else 0 end) as promotable_association_count,
          sum(case when coalesce(f.freshness_state, '') in ('partial', 'stale', 'superseded', 'tombstoned')
                    or coalesce(t.freshness_state, '') in ('partial', 'stale', 'superseded', 'tombstoned')
              then 1 else 0 end) as noncurrent_endpoint_count,
          sum(case when coalesce(r.visibility, '') <> 'public'
                    or coalesce(f.visibility, '') <> 'public'
                    or coalesce(t.visibility, '') <> 'public'
              then 1 else 0 end) as restricted_count,
          {to_type_expr} as reached_object_type
        from {family.table} r
        join {family.from_table} f on f.id = r.{family.from_id_column}
        join {family.to_table} t on t.id = r.{family.to_id_column}
        group by start_object_type, start_key, association_type, reached_object_type
        order by promotable_association_count desc, association_count desc, start_key
        limit ?
        """,
        (limit,),
    ).fetchall()
    return {
        "table": family.table,
        "from_object_type": family.from_object_type,
        "to_object_type": family.to_object_type,
        "association_summaries": [dict(row) for row in rows],
        "top_start_candidates": [candidate_with_command(dict(row)) for row in top],
    }


def promotion_condition() -> str:
    return """
        coalesce(r.visibility, '') = 'public'
        and coalesce(f.visibility, '') = 'public'
        and coalesce(t.visibility, '') = 'public'
        and coalesce(r.freshness_state, '') in ('fresh', 'current')
        and coalesce(f.freshness_state, '') in ('fresh', 'current')
        and coalesce(t.freshness_state, '') in ('fresh', 'current')
        and coalesce(r.confidence, 0) >= 1
        and (coalesce(r.evidence_count, 0) > 0 or r.latest_evidence_id is not null)
    """


def object_type_expr(alias: str, configured_type: str) -> str:
    if configured_type == "open_graph":
        return f"{alias}.object_type"
    return f"'{configured_type}'"


def candidate_with_command(row: dict[str, Any]) -> dict[str, Any]:
    row["probe_env"] = {
        "START_OBJECT_TYPE": row["start_object_type"],
        "START_KEY": row["start_key"],
        "ASSOCIATION_TYPES": row["association_type"],
    }
    row["probe_hint"] = (
        "DATABASE=$DB "
        f"START_OBJECT_TYPE={shellish(row['start_object_type'])} "
        f"START_KEY={shellish(row['start_key'])} "
        f"ASSOCIATION_TYPES={shellish(row['association_type'])} "
        "DEPTH=1 LIMIT_PER_OBJECT=6 "
        "tools/eval_packs/real_connector_bounded_probe/run_llm.sh"
    )
    return row


def shellish(value: Any) -> str:
    text = str(value)
    if not text or any(ch.isspace() for ch in text):
        return json.dumps(text)
    return text


def column_exists(conn: sqlite3.Connection, table: str, column: str) -> bool:
    return any(str(row["name"]) == column for row in conn.execute(f"pragma table_info({table})").fetchall())


def pigeonhole_signal(
    object_counts: dict[str, int],
    source_summary: dict[str, Any],
    relationship_summaries: list[dict[str, Any]],
) -> dict[str, Any]:
    source_instances = sorted(
        {
            row.get("source_instance", "")
            for rows in source_summary.values()
            for row in rows
            if row.get("source_instance")
        }
    )
    open_graph_count = object_counts.get("open_graph_objects", 0)
    non_flink_source_instances = [
        value
        for value in source_instances
        if "flink" not in value.lower() and "apache-jira" not in value.lower()
    ]
    promotable_relationship_count = sum(
        row.get("promotable_association_count", 0)
        for summary in relationship_summaries
        for row in summary.get("association_summaries", [])
    )
    return {
        "source_instances": source_instances,
        "non_flink_source_instances": non_flink_source_instances,
        "open_graph_object_count": open_graph_count,
        "promotable_relationship_count": promotable_relationship_count,
        "assessment": pigeonhole_assessment(open_graph_count, non_flink_source_instances, promotable_relationship_count),
    }


def pigeonhole_assessment(open_graph_count: int, non_flink_source_instances: list[str], promotable_relationship_count: int) -> str:
    if open_graph_count > 0 and non_flink_source_instances:
        return "real DB contains open graph rows and non-Flink source instances"
    if non_flink_source_instances:
        return "real DB has non-Flink source instances but no open graph rows"
    if promotable_relationship_count > 0:
        return "real DB has promotable relationships, but current persisted sources are still Flink/Jira/GitHub shaped"
    return "no promotable real connector graph candidate found"


def render_markdown(report: dict[str, Any]) -> str:
    lines = [
        "# Bounded Graph Probe Candidates",
        "",
        f"Database: `{report['database']}`",
        "",
        "## Object Counts",
        "",
    ]
    for table, count in report["object_counts"].items():
        lines.append(f"- `{table}`: {count}")
    lines.extend(["", "## Pigeonhole Signal", ""])
    signal = report["pigeonhole_signal"]
    lines.append(f"- assessment: {signal['assessment']}")
    lines.append(f"- source instances: {', '.join(signal['source_instances']) or 'none'}")
    lines.append(f"- non-Flink source instances: {', '.join(signal['non_flink_source_instances']) or 'none'}")
    lines.append(f"- open graph objects: {signal['open_graph_object_count']}")
    lines.append(f"- promotable relationships: {signal['promotable_relationship_count']}")
    lines.extend(["", "## Relationship Families", ""])
    for summary in report["relationship_summaries"]:
        lines.append(f"### `{summary['table']}`")
        if not summary["association_summaries"]:
            lines.append("")
            lines.append("No rows.")
            lines.append("")
            continue
        for row in summary["association_summaries"]:
            lines.append(
                "- `{association_type}`: {promotable_association_count}/{association_count} promotable; "
                "{noncurrent_endpoint_count} noncurrent endpoint row(s); {restricted_count} restricted row(s)".format(**row)
            )
        family_candidates = summary.get("top_start_candidates", [])[:5]
        if family_candidates:
            lines.append("")
            lines.append("Top starts:")
            for row in family_candidates:
                lines.append(
                    "- `{start_object_type}` `{start_key}` via `{association_type}`: "
                    "{promotable_association_count}/{association_count} promotable; command: `{probe_hint}`".format(
                        **row
                    )
                )
        lines.append("")
    lines.extend(["## Top Probe Candidates Overall", ""])
    for row in report["top_probe_candidates"]:
        lines.append(
            "- `{start_object_type}` `{start_key}` via `{association_type}`: "
            "{promotable_association_count}/{association_count} promotable; command: `{probe_hint}`".format(**row)
        )
    lines.append("")
    return "\n".join(lines)


if __name__ == "__main__":
    main()
