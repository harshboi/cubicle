#!/usr/bin/env python3

from __future__ import annotations

import pathlib
import sys
import unittest


TOOLS_DIR = pathlib.Path(__file__).parent
if str(TOOLS_DIR) not in sys.path:
    sys.path.insert(0, str(TOOLS_DIR))

import ai_first_poc_architecture_review as review  # noqa: E402


class AiFirstPocArchitectureReviewTest(unittest.TestCase):
    def test_review_separates_poc_genericity_from_product_safe_rollout(self) -> None:
        report = review.build_review(
            readiness_report(
                product_safe=False,
                raw_model=False,
                blockers=[
                    {"key": "real_acl_connector", "detail": "real ACL missing"},
                    {"key": "negative_partial_real_connector_cases", "detail": "real stale scope missing"},
                ],
            ),
            real_connector_inventory={
                "database_count": 25,
                "ok_database_count": 25,
                "real_non_flink_candidate_count": 0,
                "flink_shaped_candidate_count": 11,
                "product_acl_current_nonpublic_database_count": 0,
                "source_scope_stale_or_not_attempted_database_count": 0,
            },
        )

        self.assertTrue(report["verdicts"]["working_poc_viable"]["passes"], report)
        self.assertTrue(report["verdicts"]["not_pigeonholed_at_graph_boundary"]["passes"], report)
        self.assertFalse(report["verdicts"]["connector_dataset_genericity_proven"]["passes"], report)
        self.assertTrue(report["verdicts"]["llm_graph_traversal_viable"]["passes"], report)
        self.assertFalse(report["verdicts"]["raw_model_can_stand_alone"]["passes"], report)
        self.assertFalse(report["verdicts"]["product_safe_rollout_ready"]["passes"], report)
        self.assertEqual(len(report["product_safe_blockers"]), 2)
        self.assertIn("current persisted connector data root is still Flink-shaped", report["summary"])
        self.assertIn("real source ACL state", "\n".join(report["next_actions"]))
        self.assertIn("source-not-attempted", "\n".join(report["next_actions"]))

    def test_review_can_be_product_safe_when_all_evidence_is_present(self) -> None:
        report = review.build_review(
            readiness_report(product_safe=True, raw_model=True, blockers=[]),
            real_connector_inventory={
                "database_count": 3,
                "ok_database_count": 3,
                "real_non_flink_candidate_count": 1,
                "flink_shaped_candidate_count": 1,
                "product_acl_current_nonpublic_database_count": 1,
                "source_scope_stale_or_not_attempted_database_count": 1,
            },
        )

        self.assertTrue(report["verdicts"]["product_safe_rollout_ready"]["passes"], report)
        self.assertTrue(report["verdicts"]["connector_dataset_genericity_proven"]["passes"], report)
        self.assertEqual(report["product_safe_blockers"], [])
        self.assertEqual(report["next_actions"], ["Run the product-safe required gate and move from PoC to rollout review."])

    def test_review_without_inventory_keeps_connector_specific_risk_unknown(self) -> None:
        report = review.build_review(readiness_report(product_safe=False, raw_model=False, blockers=[]))

        self.assertFalse(report["verdicts"]["connector_dataset_genericity_proven"]["passes"], report)
        self.assertEqual(report["inventory_findings"]["connector_data_root_pigeonhole_risk"], "unknown")


def readiness_report(*, product_safe: bool, raw_model: bool, blockers: list[dict[str, str]]) -> dict[str, object]:
    return {
            "tiers": {
                "poc_green": {"passes": True},
                "production_genericity_advisory_green": {"passes": True},
                "raw_model_product_ready": {"passes": raw_model},
                "repaired_or_deterministic_product_path_ready": {"passes": True},
                "eval_gated_answer_path_ready": {"passes": True},
                "product_safe_architecture_green": {"passes": product_safe},
            },
        "case_summaries": [
            {"key": "open_graph_revenue_minimum", "passed": True},
            {"key": "real_connector_claimable_flink", "passed": True},
        ],
        "answer_eval_summaries": [
            eval_row("open_graph_revenue_minimum", "deterministic", True),
            eval_row("real_connector_claimable_flink", "repaired_model", True),
            eval_row("real_connector_claimable_flink", "raw_model", raw_model),
            eval_row("open_graph_revenue_minimum", "seed_only_baseline", False),
            eval_row("open_graph_revenue_minimum", "typed_row_baseline", False),
        ],
        "product_safe_requirements": [
            {"key": row["key"], "satisfied": False, "detail": row["detail"], "verification": []}
            for row in blockers
        ],
    }


def eval_row(case: str, kind: str, passes: bool) -> dict[str, object]:
    return {
        "case": case,
        "answer_kind": kind,
        "passes_eval": passes,
        "golden_pass_count": 1 if passes else 0,
        "golden_question_count": 1,
    }


if __name__ == "__main__":
    unittest.main()
