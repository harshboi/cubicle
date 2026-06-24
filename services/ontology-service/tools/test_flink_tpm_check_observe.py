#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import pathlib
import sys
import unittest

import pandas as pd


MODULE_PATH = pathlib.Path(__file__).with_name("flink_tpm_check_observe.py")
SPEC = importlib.util.spec_from_file_location("flink_tpm_check_observe", MODULE_PATH)
observer = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules["flink_tpm_check_observe"] = observer
SPEC.loader.exec_module(observer)


class CheckObserverRequiredChecksTest(unittest.TestCase):
    def test_rest_404_with_zero_branch_protection_rules_is_observed_no_required_contexts(self) -> None:
        rest = observer.FetchResult(
            "https://api.github.com/repos/apache/flink-kubernetes-operator/branches/main/protection/required_status_checks",
            404,
            None,
            'http_error:404:{"message":"Not Found"}',
            "github_token",
            "authenticated_api_current_observation",
            complete=False,
        )
        graphql = observer.FetchResult(
            "https://api.github.com/graphql",
            200,
            {"data": {"repository": {"branchProtectionRules": {"totalCount": 0, "nodes": []}}}},
            "",
            "github_token",
            "authenticated_api_current_observation",
            complete=True,
        )

        result = observer.required_status_checks_result_from_branch_protection_graphql(
            "apache/flink-kubernetes-operator",
            "main",
            rest,
            graphql,
        )

        self.assertIsNotNone(result)
        assert result is not None
        self.assertEqual(result.status_code, 200)
        self.assertEqual(observer.required_contexts_from_payload(result.payload), [])
        match = observer.required_check_match([], [], [], [], result)
        self.assertEqual(match["coverage_state"], "required_checks_observed")
        self.assertEqual(match["match_state"], "no_required_contexts_configured")

    def test_branch_protection_graphql_extracts_contexts_for_matching_branch(self) -> None:
        payload = {
            "data": {
                "repository": {
                    "branchProtectionRules": {
                        "totalCount": 2,
                        "nodes": [
                            {
                                "pattern": "release/*",
                                "requiresStatusChecks": True,
                                "requiredStatusCheckContexts": ["release-check"],
                                "matchingRefs": {"totalCount": 0, "nodes": []},
                            },
                            {
                                "pattern": "main",
                                "requiresStatusChecks": True,
                                "requiredStatusCheckContexts": ["build", "test"],
                                "matchingRefs": {"totalCount": 1, "nodes": [{"name": "main"}]},
                            },
                        ],
                    }
                }
            }
        }

        self.assertEqual(observer.required_contexts_from_branch_protection_rules(payload, "main"), ["build", "test"])


class CheckSignalReadinessTest(unittest.TestCase):
    def test_failing_non_required_checks_are_validation_leads_not_eta_or_product_actions(self) -> None:
        observations = pd.DataFrame(
            [
                {
                    "observed_at": "2026-06-23T00:00:00+00:00",
                    "effective_state": "open",
                    "combined_signal": "failing_checks",
                    "source_coverage_state": "complete",
                    "pr_fetch_status_code": 200,
                    "pr_fetch_complete": True,
                    "check_fetch_status_code": 200,
                    "check_fetch_complete": True,
                    "check_fetch_page_count": 1,
                    "status_fetch_status_code": 200,
                    "status_fetch_complete": True,
                    "status_fetch_page_count": 1,
                    "required_check_coverage_state": "required_checks_observed",
                    "required_check_match_state": "no_required_contexts_configured",
                    "head_source": "current_api",
                }
            ]
        )
        summary = observer.build_summary(observations, observations)

        readiness = observer.build_check_signal_readiness(
            observer.parse_dt("2026-06-23T00:00:00+00:00"),
            observations,
            summary,
        )

        by_key = {row["readiness_key"]: row for row in readiness.to_dict("records")}
        self.assertTrue(by_key["ci_followup_validation"]["ready"])
        self.assertEqual(by_key["ci_followup_validation"]["readiness_state"], "ready_with_current_open_signal")
        self.assertFalse(by_key["required_check_product_action"]["ready"])
        self.assertEqual(by_key["required_check_product_action"]["support_level"], "product_action_evidence")
        self.assertEqual(by_key["required_check_product_action"]["readiness_state"], "no_required_contexts_configured")
        self.assertFalse(by_key["ci_eta_feature"]["ready"])
        self.assertEqual(by_key["ci_eta_feature"]["readiness_state"], "single_live_observation")

    def test_required_check_signal_is_evidence_not_product_action_readiness(self) -> None:
        observations = pd.DataFrame(
            [
                {
                    "observed_at": "2026-06-23T00:00:00+00:00",
                    "effective_state": "open",
                    "combined_signal": "failing_checks",
                    "source_coverage_state": "complete",
                    "pr_fetch_status_code": 200,
                    "pr_fetch_complete": True,
                    "check_fetch_status_code": 200,
                    "check_fetch_complete": True,
                    "check_fetch_page_count": 1,
                    "status_fetch_status_code": 200,
                    "status_fetch_complete": True,
                    "status_fetch_page_count": 1,
                    "required_check_coverage_state": "required_checks_observed",
                    "required_check_match_state": "required_context_failing_or_pending",
                    "head_source": "current_api",
                }
            ]
        )
        summary = observer.build_summary(observations, observations)

        readiness = observer.build_check_signal_readiness(
            observer.parse_dt("2026-06-23T00:00:00+00:00"),
            observations,
            summary,
        )

        product_row = {row["readiness_key"]: row for row in readiness.to_dict("records")}["required_check_product_action"]
        self.assertFalse(product_row["ready"])
        self.assertEqual(product_row["support_level"], "product_action_evidence")
        self.assertEqual(product_row["readiness_state"], "required_check_evidence_ready_measurement_required")
        self.assertIn("product action still requires measurement gates", product_row["blocking_reason"])

    def test_coverage_limited_checks_do_not_support_validation_actions(self) -> None:
        observations = pd.DataFrame(
            [
                {
                    "observed_at": "2026-06-23T00:00:00+00:00",
                    "effective_state": "open",
                    "combined_signal": "coverage_partial",
                    "source_coverage_state": "partial",
                    "pr_fetch_status_code": 200,
                    "pr_fetch_complete": True,
                    "check_fetch_status_code": 403,
                    "check_fetch_complete": False,
                    "check_fetch_page_count": 0,
                    "status_fetch_status_code": 200,
                    "status_fetch_complete": True,
                    "status_fetch_page_count": 1,
                    "required_check_coverage_state": "required_checks_unavailable",
                    "required_check_match_state": "required_check_coverage_unavailable",
                    "head_source": "current_api",
                }
            ]
        )
        summary = observer.build_summary(observations, observations)

        readiness = observer.build_check_signal_readiness(
            observer.parse_dt("2026-06-23T00:00:00+00:00"),
            observations,
            summary,
        )

        by_key = {row["readiness_key"]: row for row in readiness.to_dict("records")}
        self.assertFalse(by_key["ci_followup_validation"]["ready"])
        self.assertEqual(by_key["ci_followup_validation"]["readiness_state"], "coverage_limited")
        self.assertFalse(by_key["required_check_product_action"]["ready"])
        self.assertEqual(by_key["required_check_product_action"]["readiness_state"], "coverage_limited")


if __name__ == "__main__":
    unittest.main()
