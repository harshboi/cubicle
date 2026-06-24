#!/usr/bin/env python3

from __future__ import annotations

import pathlib
import sqlite3
import sys
import tempfile
import unittest


TOOLS_DIR = pathlib.Path(__file__).parent
if str(TOOLS_DIR) not in sys.path:
    sys.path.insert(0, str(TOOLS_DIR))

import bounded_graph_real_connector_inventory as inventory  # noqa: E402


class BoundedGraphRealConnectorInventoryTest(unittest.TestCase):
    def test_inventory_detects_real_non_flink_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            db = root / "linear_github.db"
            seed_candidate_db(db, ticket_source_instance="linear/team-roadmap", pr_source_instance="github.com/acme/app")

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_real_non_flink_requirement"], report)
        self.assertEqual(report["real_non_flink_candidate_count"], 1)
        self.assertEqual(report["databases"][0]["candidate_kind"], "real_non_flink_candidate")

    def test_inventory_marks_flink_shaped_candidate_as_not_enough(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            db = root / "flink.db"
            seed_candidate_db(db, ticket_source_instance="apache-jira", pr_source_instance="github.com/apache/flink-kubernetes-operator")

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertFalse(report["passes_real_non_flink_requirement"], report)
        self.assertEqual(report["flink_shaped_candidate_count"], 1)
        self.assertEqual(report["databases"][0]["candidate_kind"], "flink_shaped_candidate")

    def test_inventory_requires_product_row_acl_not_evidence_only_acl(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            evidence_only = root / "evidence_acl_only.db"
            product_acl = root / "product_acl.db"
            source_connection_acl = root / "source_connection_acl.db"
            seed_candidate_db(
                evidence_only,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                evidence_acl_current_nonpublic=True,
            )
            seed_candidate_db(
                product_acl,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                product_acl_current_nonpublic=True,
            )
            seed_candidate_db(
                source_connection_acl,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                product_acl_current_nonpublic=True,
                source_connection=True,
                source_connection_connector_kind="github_app",
            )

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_product_acl_row_requirement"], report)
        self.assertTrue(report["passes_real_acl_ingestion_requirement"], report)
        by_name = {pathlib.Path(row["database"]).name: row for row in report["databases"]}
        self.assertEqual(by_name["evidence_acl_only.db"]["product_acl_current_nonpublic_count"], 0, report)
        self.assertEqual(by_name["evidence_acl_only.db"]["evidence_acl_current_nonpublic_count"], 1, report)
        self.assertEqual(by_name["product_acl.db"]["product_acl_current_nonpublic_count"], 1, report)
        self.assertEqual(by_name["product_acl.db"]["source_connection_count"], 0, report)
        self.assertEqual(by_name["source_connection_acl.db"]["source_connection_count"], 1, report)
        self.assertEqual(by_name["source_connection_acl.db"]["real_acl_candidate_source_backed_count"], 1, report)
        self.assertEqual(by_name["source_connection_acl.db"]["production_acl_candidate_count"], 1, report)

    def test_real_acl_ingestion_requires_acl_row_to_match_candidate_source_connection(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            unrelated = root / "unrelated_source_connection_acl.db"
            seed_candidate_db(
                unrelated,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                product_acl_current_nonpublic=True,
                source_connection=True,
                source_connection_source_instance="github.com/acme/unrelated",
            )

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_product_acl_row_requirement"], report)
        self.assertFalse(report["passes_real_acl_ingestion_requirement"], report)
        self.assertEqual(report["product_acl_current_nonpublic_database_count"], 1, report)
        self.assertEqual(report["real_acl_ingestion_database_count"], 0, report)
        self.assertEqual(report["databases"][0]["real_acl_candidate_source_backed_count"], 0, report)
        self.assertEqual(report["databases"][0]["production_acl_candidate_count"], 0, report)

    def test_real_acl_ingestion_rejects_fixture_replay_connector_kind(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            fixture_replay = root / "fixture_replay_acl.db"
            seed_candidate_db(
                fixture_replay,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                product_acl_current_nonpublic=True,
                source_connection=True,
                source_connection_connector_kind="fixture_replay",
            )

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_product_acl_row_requirement"], report)
        self.assertFalse(report["passes_real_acl_ingestion_requirement"], report)
        row = report["databases"][0]
        self.assertEqual(row["source_connection_connector_kind_counts"], {"fixture_replay": 1}, report)
        self.assertEqual(row["real_acl_candidate_source_backed_count"], 1, report)
        self.assertEqual(row["production_acl_candidate_count"], 0, report)
        self.assertEqual(row["production_source_connection_count"], 0, report)

    def test_real_acl_ingestion_rejects_unknown_connector_kind(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            unknown = root / "unknown_connector_acl.db"
            seed_candidate_db(
                unknown,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                product_acl_current_nonpublic=True,
                source_connection=True,
                source_connection_connector_kind="github",
            )

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_product_acl_row_requirement"], report)
        self.assertFalse(report["passes_real_acl_ingestion_requirement"], report)
        row = report["databases"][0]
        self.assertEqual(row["source_connection_connector_kind_counts"], {"github": 1}, report)
        self.assertEqual(row["real_acl_candidate_source_backed_count"], 1, report)
        self.assertEqual(row["production_acl_candidate_count"], 0, report)
        self.assertEqual(row["production_source_connection_count"], 0, report)

    def test_inventory_detects_stale_or_not_attempted_source_scope_rows(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            fresh_partial = root / "fresh_partial.db"
            stale = root / "stale_scope.db"
            seed_candidate_db(
                fresh_partial,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                source_scope_states=[("fresh", "partial_scope", "2026-06-24T10:00:00Z")],
            )
            seed_candidate_db(
                stale,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                source_scope_states=[("stale", "exact_scope", "2026-06-24T10:00:00Z")],
                attach_source_scope_state_to_candidate=True,
                source_connection_connector_kind="github_app",
            )

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_real_source_scope_negative_requirement"], report)
        by_name = {pathlib.Path(row["database"]).name: row for row in report["databases"]}
        self.assertEqual(by_name["fresh_partial.db"]["source_scope_stale_or_not_attempted_count"], 0, report)
        self.assertEqual(by_name["stale_scope.db"]["source_scope_stale_or_not_attempted_count"], 1, report)
        self.assertEqual(by_name["stale_scope.db"]["real_source_scope_negative_candidate_count"], 1, report)
        self.assertEqual(by_name["stale_scope.db"]["production_source_scope_negative_candidate_count"], 1, report)

    def test_real_source_scope_negative_requires_state_to_be_referenced_by_candidate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            unrelated = root / "unrelated_stale_scope.db"
            seed_candidate_db(
                unrelated,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                source_scope_states=[("stale", "exact_scope", "2026-06-24T10:00:00Z")],
                attach_source_scope_state_to_candidate=False,
            )

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_source_scope_negative_row_requirement"], report)
        self.assertFalse(report["passes_real_source_scope_negative_requirement"], report)
        self.assertEqual(report["source_scope_negative_row_database_count"], 1, report)
        self.assertEqual(report["source_scope_stale_or_not_attempted_database_count"], 0, report)
        self.assertEqual(report["databases"][0]["real_source_scope_negative_candidate_count"], 0, report)
        self.assertEqual(report["databases"][0]["production_source_scope_negative_candidate_count"], 0, report)

    def test_real_source_scope_negative_rejects_registration_connector_kind(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            registered = root / "registered_scope_candidate.db"
            seed_candidate_db(
                registered,
                ticket_source_instance="linear/team-roadmap",
                pr_source_instance="github.com/acme/app",
                source_scope_states=[("unknown", "unknown", None)],
                attach_source_scope_state_to_candidate=True,
                source_connection_connector_kind="source_scope_registration",
            )

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_source_scope_negative_row_requirement"], report)
        self.assertFalse(report["passes_real_source_scope_negative_requirement"], report)
        row = report["databases"][0]
        self.assertEqual(row["source_connection_connector_kind_counts"], {"source_scope_registration": 1}, report)
        self.assertEqual(row["real_source_scope_negative_candidate_count"], 1, report)
        self.assertEqual(row["production_source_scope_negative_candidate_count"], 0, report)
        self.assertEqual(row["production_source_connection_count"], 0, report)

    def test_source_scope_row_alone_does_not_satisfy_real_connector_negative_gate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            registered = root / "registered_scope_only.db"
            seed_source_scope_only_db(
                registered,
                source_scope_states=[("unknown", "unknown", None)],
            )

            report = inventory.build_inventory(
                data_root=root,
                explicit_databases=[],
                database_globs=[],
                limit_per_db=3,
            )

        self.assertTrue(report["passes_source_scope_negative_row_requirement"], report)
        self.assertFalse(report["passes_real_source_scope_negative_requirement"], report)
        self.assertEqual(report["source_scope_negative_row_database_count"], 1, report)
        self.assertEqual(report["source_scope_stale_or_not_attempted_database_count"], 0, report)
        self.assertEqual(report["databases"][0]["candidate_kind"], "no_promotable_candidate", report)


def seed_candidate_db(
    path: pathlib.Path,
    *,
    ticket_source_instance: str,
    pr_source_instance: str,
    product_acl_current_nonpublic: bool = False,
    evidence_acl_current_nonpublic: bool = False,
    source_scope_states: list[tuple[str, str, str | None]] | None = None,
    source_connection: bool = False,
    source_connection_source_instance: str | None = None,
    source_connection_connector_kind: str = "github_app",
    attach_source_scope_state_to_candidate: bool = False,
) -> None:
    with sqlite3.connect(path) as conn:
        conn.executescript(
            """
            create table tickets (
              id integer primary key,
              key text,
              source_system text,
              source_instance text,
              external_kind text,
              freshness_state text,
              visibility text,
              acl_state text,
              source_scope_state_id integer
            );
            create table pull_requests (
              id integer primary key,
              key text,
              source_system text,
              source_instance text,
              external_kind text,
              freshness_state text,
              visibility text,
              acl_state text,
              source_scope_state_id integer
            );
            create table ticket_pull_requests (
              id integer primary key,
              ticket_id integer,
              pull_request_id integer,
              visibility text,
              acl_state text,
              freshness_state text,
              confidence real,
              evidence_count integer,
              latest_evidence_id integer,
              source_system text,
              source_instance text,
              source_scope_state_id integer
            );
            create table evidences (
              id integer primary key,
              visibility text,
              acl_state text
            );
            create table source_connections (
              id integer primary key,
              source_system text,
              source_instance text,
              connector_kind text
            );
            create table source_scopes (
              id integer primary key,
              source_connection_id integer
            );
            create table source_scope_states (
              id integer primary key,
              freshness_state text,
              coverage_mode text,
              last_attempted_at text,
              source_scope_id integer
            );
            """
        )
        source_connection_instance = source_connection_source_instance or pr_source_instance
        if source_connection or source_scope_states:
            conn.execute(
                "insert into source_connections (id, source_system, source_instance, connector_kind) values (1, 'github', ?, ?)",
                (source_connection_instance, source_connection_connector_kind),
            )
        if source_scope_states:
            conn.execute("insert into source_scopes (id, source_connection_id) values (1, 1)")
        attached_scope_id = 1 if attach_source_scope_state_to_candidate and source_scope_states else None
        conn.execute(
            """
            insert into tickets
              (id, key, source_system, source_instance, external_kind, freshness_state, visibility, acl_state, source_scope_state_id)
            values
              (1, 'ticket:roadmap:123', 'tracker', ?, 'ticket', 'fresh', 'public', 'current', null)
            """,
            (ticket_source_instance,),
        )
        conn.execute(
            """
            insert into pull_requests
              (id, key, source_system, source_instance, external_kind, freshness_state, visibility, acl_state, source_scope_state_id)
            values
              (1, 'pull-request:github:acme/app#7', 'github', ?, 'pull_request', 'fresh', ?, 'current', ?)
            """,
            (pr_source_instance, "private" if product_acl_current_nonpublic else "public", attached_scope_id),
        )
        conn.execute(
            """
            insert into ticket_pull_requests
              (id, ticket_id, pull_request_id, visibility, acl_state, freshness_state, confidence, evidence_count, latest_evidence_id, source_system, source_instance, source_scope_state_id)
            values
              (1, 1, 1, 'public', 'current', 'fresh', 1, 1, null, 'github', ?, null)
            """,
            (pr_source_instance,),
        )
        conn.execute(
            "insert into evidences (id, visibility, acl_state) values (1, ?, 'current')",
            ("restricted" if evidence_acl_current_nonpublic else "public",),
        )
        for index, (freshness, coverage, attempted_at) in enumerate(source_scope_states or [], start=1):
            conn.execute(
                """
                insert into source_scope_states
                  (id, freshness_state, coverage_mode, last_attempted_at, source_scope_id)
                values
                  (?, ?, ?, ?, 1)
                """,
                (index, freshness, coverage, attempted_at),
            )


def seed_source_scope_only_db(
    path: pathlib.Path,
    *,
    source_scope_states: list[tuple[str, str, str | None]],
) -> None:
    with sqlite3.connect(path) as conn:
        conn.executescript(
            """
            create table source_scope_states (
              id integer primary key,
              freshness_state text,
              coverage_mode text,
              last_attempted_at text
            );
            """
        )
        for index, (freshness, coverage, attempted_at) in enumerate(source_scope_states, start=1):
            conn.execute(
                """
                insert into source_scope_states
                  (id, freshness_state, coverage_mode, last_attempted_at)
                values
                  (?, ?, ?, ?)
                """,
                (index, freshness, coverage, attempted_at),
            )


if __name__ == "__main__":
    unittest.main()
