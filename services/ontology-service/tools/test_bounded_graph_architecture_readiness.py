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

import bounded_graph_architecture_readiness as readiness  # noqa: E402


class BoundedGraphArchitectureReadinessTest(unittest.TestCase):
    def test_tiers_separate_poc_from_raw_and_product_safe(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            case_dir = root / "real_case"
            case_dir.mkdir()
            write_json(case_dir / "raw_eval.json", eval_report(False, True, 2, 6))
            write_json(case_dir / "repaired_eval.json", eval_report(True, True, 6, 6, repair=True))
            write_json(case_dir / "generic_baseline_eval.json", eval_report(True, True, 6, 6))
            matrix_path = root / "report.json"
            matrix = {
                "passes_matrix": True,
                "out_dir": str(root),
                "required_tags": [
                    "auth-limited",
                    "real-connector",
                    "source-authority",
                    "typed-vs-graph-promotion",
                ],
                "covered_tags": [
                    "auth-limited",
                    "real-connector",
                    "real-non-github-domain",
                    "real-non-flink-connector",
                    "source-authority",
                    "typed-vs-graph-promotion",
                ],
                "advisory_tags": ["real-non-flink-connector", "real-non-github-domain"],
                "advisory_gaps": [],
                "cases": [
                    {
                        "key": "real_case",
                        "passed": True,
                        "out_dir": str(case_dir),
                        "tags": ["real-connector", "real-non-flink-connector", "real-non-github-domain"],
                        "metrics": {"golden_pass_total": 14, "golden_question_total": 18},
                    }
                ],
            }
            write_json(matrix_path, matrix)

            report = readiness.build_readiness_report(
                matrix,
                matrix_report_path=matrix_path,
                product_safe_evidence={},
            )

        self.assertTrue(report["tiers"]["poc_green"]["passes"])
        self.assertTrue(report["tiers"]["production_genericity_advisory_green"]["passes"])
        self.assertFalse(report["tiers"]["raw_model_product_ready"]["passes"])
        self.assertTrue(report["tiers"]["repaired_or_deterministic_product_path_ready"]["passes"])
        self.assertTrue(report["tiers"]["eval_gated_answer_path_ready"]["passes"])
        self.assertFalse(report["tiers"]["product_safe_architecture_green"]["passes"])
        self.assertNotIn("raw model output is diagnostic", "\n".join(report["blockers"]))
        self.assertIn("raw model output is diagnostic", "\n".join(report["diagnostics"]))

    def test_product_safe_does_not_require_raw_model_when_repaired_path_is_ready(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            case_dir = root / "case"
            case_dir.mkdir()
            write_json(case_dir / "raw_eval.json", eval_report(False, True, 2, 3))
            write_json(case_dir / "repaired_eval.json", eval_report(True, True, 3, 3, repair=True))
            matrix_path = root / "report.json"
            matrix = {
                "passes_matrix": True,
                "out_dir": str(root),
                "required_tags": ["real-connector"],
                "covered_tags": ["real-connector", "real-non-flink-connector", "real-non-github-domain"],
                "advisory_tags": ["real-non-flink-connector", "real-non-github-domain"],
                "advisory_gaps": [],
                "cases": [{"key": "case", "passed": True, "out_dir": str(case_dir), "metrics": {}}],
            }
            evidence = {key: True for key in readiness.DEFAULT_PRODUCT_SAFE_REQUIREMENTS}

            report = readiness.build_readiness_report(
                matrix,
                matrix_report_path=matrix_path,
                product_safe_evidence=evidence,
            )

        self.assertFalse(report["tiers"]["raw_model_product_ready"]["passes"])
        self.assertTrue(report["tiers"]["repaired_or_deterministic_product_path_ready"]["passes"])
        self.assertTrue(report["tiers"]["eval_gated_answer_path_ready"]["passes"])
        self.assertTrue(report["tiers"]["product_safe_architecture_green"]["passes"])
        self.assertEqual(report["blockers"], [])
        self.assertIn("raw model output is diagnostic", "\n".join(report["diagnostics"]))

    def test_product_safe_can_pass_when_explicit_evidence_is_supplied(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            case_dir = root / "case"
            case_dir.mkdir()
            write_json(case_dir / "raw_eval.json", eval_report(True, True, 3, 3))
            matrix_path = root / "report.json"
            matrix = {
                "passes_matrix": True,
                "out_dir": str(root),
                "required_tags": ["real-connector"],
                "covered_tags": ["real-connector", "real-non-flink-connector", "real-non-github-domain"],
                "advisory_tags": ["real-non-flink-connector", "real-non-github-domain"],
                "advisory_gaps": [],
                "cases": [{"key": "case", "passed": True, "out_dir": str(case_dir), "metrics": {}}],
            }
            evidence = {key: True for key in readiness.DEFAULT_PRODUCT_SAFE_REQUIREMENTS}

            report = readiness.build_readiness_report(
                matrix,
                matrix_report_path=matrix_path,
                product_safe_evidence=evidence,
            )

        self.assertTrue(report["tiers"]["product_safe_architecture_green"]["passes"])
        self.assertEqual(report["blockers"], [])

    def test_structured_requirement_evidence_is_preserved(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            matrix_path = root / "report.json"
            matrix = {
                "passes_matrix": False,
                "out_dir": str(root),
                "required_tags": [],
                "covered_tags": [],
                "cases": [],
            }
            report = readiness.build_readiness_report(
                matrix,
                matrix_report_path=matrix_path,
                product_safe_evidence={
                    "generated_summary_quarantine": {
                        "satisfied": True,
                        "detail": "generated summaries stay quarantined",
                        "evidence": ["test-path"],
                        "verification": ["test-command"],
                    }
                },
            )

        requirement = next(
            row for row in report["product_safe_requirements"] if row["key"] == "generated_summary_quarantine"
        )
        self.assertTrue(requirement["satisfied"])
        self.assertEqual(requirement["detail"], "generated summaries stay quarantined")
        self.assertEqual(requirement["evidence"], ["test-path"])
        self.assertEqual(requirement["verification"], ["test-command"])


def eval_report(passes: bool, smoke: bool, pass_count: int, question_count: int, repair: bool = False) -> dict[str, object]:
    return {
        "passes_eval": passes,
        "passes_smoke_eval": smoke,
        "repair_applied": repair,
        "golden_eval": {
            "pass_count": pass_count,
            "question_count": question_count,
        },
    }


def write_json(path: pathlib.Path, payload: object) -> None:
    path.write_text(json.dumps(payload, indent=2, sort_keys=True), encoding="utf-8")


if __name__ == "__main__":
    unittest.main()
