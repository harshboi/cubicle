#!/usr/bin/env python3

from __future__ import annotations

import json
import pathlib
import sys
import tempfile
import unittest


TOOLS_DIR = pathlib.Path(__file__).parent
if str(TOOLS_DIR) not in sys.path:
    sys.path.insert(0, str(TOOLS_DIR))

import bounded_graph_promotion_matrix as matrix  # noqa: E402


class BoundedGraphPromotionMatrixTest(unittest.TestCase):
    def test_evaluate_matrix_accepts_passing_case_reports(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            case_dir = root / "case"
            case_dir.mkdir()
            write_json(case_dir / "contract.json", {"passes_contract": True})
            write_json(
                case_dir / "promotion_audit.json",
                {
                    "passes_promotion_audit": True,
                    "promotable_association_count": 2,
                    "blocked_association_count": 0,
                },
            )
            write_json(
                case_dir / "eval.json",
                {
                    "passes_eval": True,
                    "passes_smoke_eval": True,
                    "golden_eval": {"pass_count": 3, "question_count": 3},
                },
            )
            write_json(case_dir / "comparison.json", {"passes_promotion_gates": True, "promotion_gates": [{"passes": True}]})
            (case_dir / "prompt.md").write_text("bounded graph prompt", encoding="utf-8")

            report = matrix.evaluate_matrix(
                {
                    "matrix_key": "test",
                    "required_tags": ["non-flink-open-graph"],
                    "advisory_tags": ["real-non-flink-connector"],
                    "advisory_tag_details": {
                        "real-non-flink-connector": "second real connector missing",
                    },
                    "cases": [
                        {
                            "key": "case",
                            "tags": ["non-flink-open-graph"],
                            "out_dir": str(case_dir),
                            "contract_reports": ["contract.json"],
                            "promotion_audit_reports": [
                                {
                                    "path": "promotion_audit.json",
                                    "min_promotable_association_count": 1,
                                    "max_blocked_association_count": 0,
                                }
                            ],
                            "eval_reports": [{"path": "eval.json", "min_pass_count": 3, "question_count": 3}],
                            "comparison_reports": ["comparison.json"],
                            "scan_paths": ["prompt.md"],
                            "forbidden_terms": ["WorkProgram"],
                        }
                    ],
                },
                out_dir=root,
                run_commands=False,
            )

        self.assertTrue(report["passes_matrix"], report)
        self.assertEqual(report["covered_tags"], ["non-flink-open-graph"])
        self.assertEqual(report["advisory_gaps"][0]["tag"], "real-non-flink-connector")
        self.assertIn("second real connector missing", report["advisory_gaps"][0]["detail"])

    def test_evaluate_matrix_rejects_missing_required_tag_and_forbidden_term(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            case_dir = root / "case"
            case_dir.mkdir()
            write_json(case_dir / "eval.json", {"passes_eval": True, "golden_eval": {"pass_count": 1, "question_count": 1}})
            (case_dir / "prompt.md").write_text("WorkProgram leaked", encoding="utf-8")

            report = matrix.evaluate_matrix(
                {
                    "matrix_key": "test",
                    "required_tags": ["real-connector"],
                    "cases": [
                        {
                            "key": "case",
                            "tags": ["non-flink-open-graph"],
                            "out_dir": str(case_dir),
                            "eval_reports": [{"path": "eval.json", "min_pass_count": 1}],
                            "scan_paths": ["prompt.md"],
                            "forbidden_terms": ["WorkProgram"],
                        }
                    ],
                },
                out_dir=root,
                run_commands=False,
            )

        self.assertFalse(report["passes_matrix"], report)
        details = "\n".join(row["detail"] for row in report["failures"])
        self.assertIn("forbidden terms found", details)
        self.assertIn("missing_required_tag", json.dumps(report["failures"]))

    def test_evaluate_matrix_can_require_advisory_tags(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            case_dir = root / "case"
            case_dir.mkdir()
            write_json(case_dir / "eval.json", {"passes_eval": True, "golden_eval": {"pass_count": 1, "question_count": 1}})

            report = matrix.evaluate_matrix(
                {
                    "matrix_key": "test",
                    "required_tags": ["non-flink-open-graph"],
                    "advisory_tags": ["real-non-flink-connector"],
                    "cases": [
                        {
                            "key": "case",
                            "tags": ["non-flink-open-graph"],
                            "out_dir": str(case_dir),
                            "eval_reports": [{"path": "eval.json", "min_pass_count": 1}],
                        }
                    ],
                },
                out_dir=root,
                run_commands=False,
                require_advisory_tags=True,
            )

        self.assertFalse(report["passes_matrix"], report)
        self.assertIn("missing_advisory_tag", json.dumps(report["failures"]))


def write_json(path: pathlib.Path, payload: object) -> None:
    path.write_text(json.dumps(payload, indent=2, sort_keys=True), encoding="utf-8")


if __name__ == "__main__":
    unittest.main()
