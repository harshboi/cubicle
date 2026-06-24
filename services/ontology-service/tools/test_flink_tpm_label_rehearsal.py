#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import pathlib
import sqlite3
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("flink_tpm_label_rehearsal.py")
SPEC = importlib.util.spec_from_file_location("flink_tpm_label_rehearsal", MODULE_PATH)
rehearsal = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(rehearsal)


class LabelRehearsalTest(unittest.TestCase):
    def test_prepare_rehearsal_paths_uses_copied_database_names(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology = root / "ontology.db"
            analytics = root / "analytics.db"
            ontology.write_text("source ontology")
            analytics.write_text("source analytics")

            paths = rehearsal.prepare_rehearsal_paths(root / "out", "dry", ontology, analytics)

            self.assertEqual(paths["ontology_db"], root / "out" / "dry.ontology.db")
            self.assertEqual(paths["analytics_db"], root / "out" / "dry.analytics.db")
            self.assertNotEqual(paths["ontology_db"], ontology)
            self.assertNotEqual(paths["analytics_db"], analytics)

    def test_collect_rehearsal_summary_reads_readiness_and_action_counts(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            analytics = root / "analytics.db"
            ontology = root / "ontology.db"
            with sqlite3.connect(analytics) as conn:
                conn.execute("create table tpm_action_summary (metric text, value text)")
                conn.execute("insert into tpm_action_summary values ('open_work_count', '2')")
                conn.execute("insert into tpm_action_summary values ('validation_lead_count', '3')")
                conn.execute("create table tpm_measurement_label_summary (metric text, value text)")
                conn.execute("insert into tpm_measurement_label_summary values ('measurement_label_count', '4')")
                conn.execute("create table tpm_review_metrics (scope text, metric text, value text)")
                conn.execute("insert into tpm_review_metrics values ('all', 'ready_to_measure_precision', 'true')")
                conn.execute("insert into tpm_review_metrics values ('all', 'ready_to_measure_actionability', 'false')")
                conn.execute("create table tpm_action_items (decision_state text, action_type text)")
                conn.execute("insert into tpm_action_items values ('product_action', 'clear_blocker')")
                conn.execute("insert into tpm_action_items values ('validation_lead', 'validate_signal')")
            with sqlite3.connect(ontology) as conn:
                conn.execute(
                    """
                    create table work_insight_reviews (
                      review_kind text,
                      label_set text,
                      measurement_eligible integer
                    )
                    """
                )
                conn.execute("insert into work_insight_reviews values ('evaluation_label', 'seed', 1)")
                conn.execute("insert into work_insight_reviews values ('evaluation_label', 'seed', 0)")
                conn.execute("create table work_blockers (source_instance text, blocker_state text)")
                conn.execute("insert into work_blockers values ('fixture-source', 'active')")
                conn.execute("insert into work_blockers values ('other-source', 'active')")
                conn.execute("create table work_dependency_edges (source_instance text, edge_kind text)")
                conn.execute("insert into work_dependency_edges values ('fixture-source', 'blocked_by')")
                conn.execute("insert into work_dependency_edges values ('fixture-source', 'needs_action')")
                conn.execute("insert into work_dependency_edges values ('other-source', 'blocked_by')")

            summary = rehearsal.collect_rehearsal_summary(analytics, ontology, "seed", "fixture-source")

            self.assertEqual(summary["action_summary"]["open_work_count"], "2")
            self.assertEqual(summary["measurement_summary"]["measurement_label_count"], "4")
            self.assertEqual(summary["review_metrics"]["ready_to_measure_precision"], "true")
            self.assertEqual(summary["decision_state_counts"], {"product_action": 1, "validation_lead": 1})
            self.assertEqual(summary["action_type_counts"], {"clear_blocker": 1, "validate_signal": 1})
            self.assertEqual(summary["imported_label_count"], 2)
            self.assertEqual(summary["measurement_eligible_imported_label_count"], 1)
            self.assertEqual(summary["work_blocker_count"], 1)
            self.assertEqual(summary["active_work_blocker_count"], 1)
            self.assertEqual(summary["work_dependency_edge_counts"], {"blocked_by": 1, "needs_action": 1})

    def test_label_preflight_rejects_blank_gold_queue(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            labels = root / "labels.tsv"
            labels.write_text(
                "insight_key\tinsight_kind\tgold_truth_label\tgold_actionability_label\tgold_review_state\n"
                "insight:one\tblocker_candidate\tunknown\tunknown\t\n"
            )

            summary = rehearsal.summarize_label_file(labels)

            self.assertEqual(summary["row_count"], 1)
            self.assertEqual(summary["importable_label_count"], 0)
            self.assertEqual(summary["blank_or_unknown_label_count"], 1)
            with self.assertRaises(SystemExit) as raised:
                rehearsal.validate_label_preflight(summary, root / "preflight.md")
            self.assertIn("no importable gold labels", str(raised.exception))

    def test_label_preflight_accepts_completed_gold_queue(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            labels = root / "labels.tsv"
            labels.write_text(
                "insight_key\tinsight_kind\tgold_truth_label\tgold_actionability_label\tgold_review_state\n"
                "insight:one\tblocker_candidate\tpartial\tneeds_owner\tneeds_more_data\n"
            )

            summary = rehearsal.summarize_label_file(labels)

            self.assertEqual(summary["importable_label_count"], 1)
            self.assertEqual(summary["truth_label_counts"], {"partial": 1})
            self.assertEqual(summary["actionability_label_counts"], {"needs_owner": 1})
            rehearsal.validate_label_preflight(summary, root / "preflight.md")

    def test_label_preflight_reports_invalid_enum_rows(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            labels = root / "labels.tsv"
            labels.write_text(
                "insight_key\tinsight_kind\tgold_truth_label\tgold_actionability_label\tgold_review_state\n"
                "insight:one\tblocker_candidate\tmaybe\tneeds_owner\taccepted\n"
            )

            summary = rehearsal.summarize_label_file(labels)

            self.assertEqual(len(summary["invalid_rows"]), 1)
            with self.assertRaises(SystemExit) as raised:
                rehearsal.validate_label_preflight(summary, root / "preflight.md")
            self.assertIn("invalid row", str(raised.exception))

    def test_file_sha256_is_stable_for_source_db_audit(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "source.db"
            path.write_text("db contents")

            first = rehearsal.file_sha256(path)
            second = rehearsal.file_sha256(path)

            self.assertEqual(first, second)
            self.assertEqual(len(first), 64)

    def test_copy_database_uses_sqlite_backup_copy(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            source = root / "source.db"
            destination = root / "copy.db"
            with sqlite3.connect(source) as conn:
                conn.execute("create table labels (key text)")
                conn.execute("insert into labels values ('one')")

            rehearsal.copy_database(source, destination)

            with sqlite3.connect(destination) as conn:
                self.assertEqual(conn.execute("select key from labels").fetchone()[0], "one")

    def test_mark_rehearsal_copy_writes_manifest_inside_copied_db(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            copied = root / "copy.db"
            with sqlite3.connect(copied) as conn:
                conn.execute("create table labels (key text)")

            rehearsal.mark_rehearsal_copy(
                copied,
                source_db=root / "source.db",
                db_role="ontology",
                labels=root / "labels.tsv",
                source_instance="fixture-source",
                label_set="fixture-gold",
                prefix="dry-run",
                reviewed_at="2026-06-23T00:00:00+00:00",
            )

            with sqlite3.connect(copied) as conn:
                manifest = dict(conn.execute("select key, value from cubicle_rehearsal_manifest").fetchall())
            self.assertEqual(manifest["rehearsal_only"], "true")
            self.assertEqual(manifest["db_role"], "ontology")
            self.assertEqual(manifest["label_set"], "fixture-gold")
            self.assertIn("Do not use copied rows as product truth", manifest["warning"])

    def test_source_copy_boundary_counts_compare_truth_tables(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            source = root / "source.db"
            copied = root / "copy.db"
            for path in [source, copied]:
                with sqlite3.connect(path) as conn:
                    conn.execute(
                        """
                        create table work_insight_reviews (
                          review_kind text,
                          label_set text,
                          measurement_eligible integer
                        )
                        """
                    )
                    conn.execute("create table work_blockers (source_instance text, blocker_state text)")
                    conn.execute("create table work_dependency_edges (source_instance text, edge_kind text)")
            with sqlite3.connect(copied) as conn:
                conn.execute("insert into work_insight_reviews values ('evaluation_label', 'fixture-gold', 1)")
                conn.execute("insert into work_blockers values ('fixture-source', 'active')")
                conn.execute("insert into work_dependency_edges values ('fixture-source', 'blocked_by')")
                conn.execute("insert into work_dependency_edges values ('fixture-source', 'needs_action')")

            counts = rehearsal.collect_source_copy_boundary_counts(source, copied, "fixture-gold", "fixture-source")

            self.assertEqual(counts["source"]["label_set_reviews"], 0)
            self.assertEqual(counts["source"]["work_blockers"], 0)
            self.assertEqual(counts["copy"]["label_set_reviews"], 1)
            self.assertEqual(counts["copy"]["measurement_eligible_label_set_reviews"], 1)
            self.assertEqual(counts["copy"]["active_work_blockers"], 1)
            self.assertEqual(counts["copy"]["blocked_by_edges"], 1)
            self.assertEqual(counts["copy"]["needs_action_edges"], 1)

    def test_write_rehearsal_report_declares_source_dbs_not_written(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            report = root / "summary.md"
            rehearsal.write_rehearsal_report(
                report,
                source_instance="fixture",
                source_ontology_db=root / "source-ontology.db",
                source_analytics_db=root / "source-analytics.db",
                copied_ontology_db=root / "copy-ontology.db",
                copied_analytics_db=root / "copy-analytics.db",
                labels=root / "labels.tsv",
                review_report=root / "review.md",
                action_report=root / "action.md",
                queue_path=root / "queue.tsv",
                queue_report=root / "queue.md",
                preflight_report=root / "preflight.md",
                label_summary={
                    "row_count": 1,
                    "importable_label_count": 1,
                    "blank_or_unknown_label_count": 0,
                    "missing_insight_key_count": 0,
                },
                source_hashes_before={"ontology_db": "a" * 64, "analytics_db": "b" * 64},
                source_hashes_after={"ontology_db": "a" * 64, "analytics_db": "b" * 64},
                source_hashes_match=True,
                summary={
                    "imported_label_count": 1,
                    "current_imported_label_count": 1,
                    "stale_imported_label_count": 0,
                    "measurement_eligible_imported_label_count": 1,
                    "work_blocker_count": 1,
                    "active_work_blocker_count": 1,
                    "action_summary": {"open_work_count": "0"},
                    "measurement_summary": {"measurement_label_count": "1"},
                    "review_metrics": {"ready_to_measure_precision": "false", "ready_to_measure_actionability": "false"},
                    "decision_state_counts": {"validation_lead": 1},
                    "action_type_counts": {"validate_signal": 1},
                    "work_dependency_edge_counts": {"blocked_by": 1, "needs_action": 1},
                    "rehearsal_manifest": {
                        "rehearsal_only": "true",
                        "warning": "This SQLite database is a copied label rehearsal artifact.",
                    },
                    "source_copy_boundary_counts": {
                        "source": {"label_set_reviews": 0, "work_blockers": 0},
                        "copy": {"label_set_reviews": 1, "work_blockers": 1},
                    },
                },
            )

            text = report.read_text()
            self.assertIn("The source databases are not written by this rehearsal.", text)
            self.assertIn("Source DB hash check: unchanged", text)
            self.assertIn("Rehearsal output is copied-DB evidence only.", text)
            self.assertIn("Do not treat copied work blockers, dependency edges, or synthetic labels as product truth.", text)
            self.assertIn("Copied DB manifest marker: `rehearsal_only=true`", text)
            self.assertIn("Importable label rows: 1", text)
            self.assertIn("Imported label rows for this label set: 1", text)
            self.assertIn("Current imported label rows: 1", text)
            self.assertIn("Stale/non-current imported label rows: 0", text)
            self.assertIn("Work blocker rows: 1", text)
            self.assertIn("Active work blocker rows: 1", text)
            self.assertIn("| validation_lead | 1 |", text)
            self.assertIn("| blocked_by | 1 |", text)
            self.assertIn("| needs_action | 1 |", text)
            self.assertIn("| label_set_reviews | 0 | 1 |", text)
            self.assertIn("| work_blockers | 0 | 1 |", text)


if __name__ == "__main__":
    unittest.main()
