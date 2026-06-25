#!/usr/bin/env python3

from __future__ import annotations

import contextlib
import io
import json
import pathlib
import sys
import tempfile
import unittest


TOOLS_DIR = pathlib.Path(__file__).parent
if str(TOOLS_DIR) not in sys.path:
    sys.path.insert(0, str(TOOLS_DIR))

import bounded_graph_brief  # noqa: E402
import bounded_graph_contract  # noqa: E402
import bounded_graph_promotion_audit  # noqa: E402


BOUNDED_GRAPH_PACK = TOOLS_DIR / "eval_packs" / "bounded_graph_minimum"


class BoundedGraphBriefCLITest(unittest.TestCase):
    def test_main_renders_and_evaluates_bounded_graph_contract(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            baseline_md = root / "generic_baseline.md"
            eval_json = root / "eval.json"

            bounded_graph_brief.main(
                [
                    "--bounded-graph-context-json",
                    str(BOUNDED_GRAPH_PACK / "context.json"),
                    "--context-json",
                    str(root / "normalized.json"),
                    "--brief-md",
                    str(root / "scaffold.md"),
                    "--generic-baseline-md",
                    str(baseline_md),
                    "--typed-row-baseline-md",
                    str(root / "typed_row_baseline.md"),
                    "--prompt-md",
                    str(root / "prompt.md"),
                    "--llm-brief-md",
                    str(baseline_md),
                    "--evaluation-json",
                    str(eval_json),
                    "--golden-json",
                    str(BOUNDED_GRAPH_PACK / "golden_questions.json"),
                ]
            )

            prompt = (root / "prompt.md").read_text(encoding="utf-8")
            scaffold = (root / "scaffold.md").read_text(encoding="utf-8")
            evaluation = json.loads(eval_json.read_text(encoding="utf-8"))

        self.assertIn("AI graph-context analyst", prompt)
        self.assertIn("Generic Graph Citation Scope", prompt)
        self.assertNotIn("[analytics:", prompt)
        self.assertNotIn('"analytics"', prompt)
        self.assertNotIn("forecast_summary", prompt)
        self.assertNotIn("measurement_readiness", prompt)
        self.assertNotIn("blocker_candidate_count", prompt)
        self.assertNotIn("[analytics:", scaffold)
        self.assertNotIn("TPM", scaffold)
        self.assertTrue(evaluation["passes_eval"], evaluation)
        self.assertEqual(evaluation["prompt_mode"], "generic")

    def test_parser_rejects_workprogram_and_persistence_inputs(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            base_args = [
                "--bounded-graph-context-json",
                str(BOUNDED_GRAPH_PACK / "context.json"),
                "--context-json",
                str(root / "normalized.json"),
                "--brief-md",
                str(root / "scaffold.md"),
            ]
            rejected_args = [
                ("--ontology-db", "ontology.db"),
                ("--analytics-db", "analytics.db"),
                ("--graph-context-json", "work_program_context.json"),
                ("--workstream-key", "workstream:fixture"),
                ("--source-instance", "fixture-source"),
                ("--persist-ai-insight", None),
            ]

            for flag, value in rejected_args:
                argv = [*base_args, flag]
                if value is not None:
                    argv.append(value)
                with self.subTest(flag=flag):
                    with contextlib.redirect_stderr(io.StringIO()):
                        with self.assertRaises(SystemExit) as raised:
                            bounded_graph_brief.parse_args(argv)
                    self.assertEqual(raised.exception.code, 2)

    def test_parser_rejects_operating_prompt_mode(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            with contextlib.redirect_stderr(io.StringIO()):
                with self.assertRaises(SystemExit) as raised:
                    bounded_graph_brief.parse_args(
                        [
                            "--bounded-graph-context-json",
                            str(BOUNDED_GRAPH_PACK / "context.json"),
                            "--context-json",
                            str(root / "normalized.json"),
                            "--brief-md",
                            str(root / "scaffold.md"),
                            "--prompt-mode",
                            "operating",
                        ]
                    )
            self.assertEqual(raised.exception.code, 2)

    def test_input_validation_rejects_workprogram_payloads_and_smuggled_rows(self) -> None:
        cases = [
            (
                {"data": {"workProgramGraphContext": {"contextHash": "workprogramctx"}}},
                "workProgramGraphContext",
            ),
            (
                {
                    "boundedGraphContext": {
                        "contextHash": "missingmode123",
                        "seed": {"objectType": "document", "key": "doc:x"},
                        "objects": [],
                        "associations": [],
                    }
                },
                "scopeMode=bounded_graph_context",
            ),
            (
                {
                    "boundedGraphContext": {
                        "contextHash": "analyticsrows123",
                        "scopeMode": "bounded_graph_context",
                        "seed": {"objectType": "document", "key": "doc:x"},
                        "objects": [],
                        "associations": [],
                        "analytics": {"forecast_summary": {}},
                    }
                },
                "analytics rows",
            ),
            (
                {
                    "boundedGraphContext": {
                        "contextHash": "analyticscite123",
                        "scopeMode": "bounded_graph_context",
                        "seed": {"objectType": "document", "key": "doc:x"},
                        "objects": [],
                        "associations": [],
                        "citations": [{"ref": "[analytics:tpm_forecast_summary]"}],
                    }
                },
                "analytics citations",
            ),
            (
                {
                    "boundedGraphContext": {
                        "contextHash": "workobject123",
                        "scopeMode": "bounded_graph_context",
                        "seed": {"objectType": "work_program_item", "key": "work-program-item:1"},
                        "objects": [{"objectType": "work_program_item", "key": "work-program-item:1"}],
                        "associations": [],
                    }
                },
                "WorkProgram object type",
            ),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            for index, (payload, expected_message) in enumerate(cases):
                path = root / f"case_{index}.json"
                path.write_text(json.dumps(payload), encoding="utf-8")
                with self.subTest(expected_message=expected_message):
                    with self.assertRaises(SystemExit) as raised:
                        bounded_graph_brief.validate_bounded_graph_input(path)
                    self.assertIn(expected_message, str(raised.exception))

    def test_contract_rejects_claimable_relationship_without_proof_metadata(self) -> None:
        payload = minimal_bounded_context()
        payload["boundedGraphContext"]["associations"] = [
            {
                "key": "assoc:unsafe",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(
            any(row["kind"] == "missing_required_field" and row["path"].endswith(".evidenceKey") for row in report["errors"]),
            report,
        )

    def test_contract_rejects_raw_prompt_fields(self) -> None:
        payload = minimal_bounded_context()
        payload["boundedGraphContext"]["objects"][0]["sourceURL"] = "https://private.example/raw"

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "raw_prompt_field" for row in report["errors"]), report)

    def test_connector_profile_warns_on_missing_source_scope(self) -> None:
        payload = minimal_bounded_context()

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload, profile="connector")

        self.assertFalse(report["passes_contract"], report)
        self.assertGreater(report["blocking_warning_count"], 0)
        self.assertTrue(any(row["kind"] == "connector_source_scope_missing" for row in report["warnings"]), report)

    def test_contract_rejects_claimable_object_without_public_visibility(self) -> None:
        payload = minimal_bounded_context()
        payload["boundedGraphContext"]["objects"][0]["objectType"] = "document"
        payload["boundedGraphContext"]["objects"][0]["claimAllowed"] = True
        payload["boundedGraphContext"]["objects"][0]["proofState"] = "source_observed"
        payload["boundedGraphContext"]["objects"][0].pop("claimGateReason", None)
        del payload["boundedGraphContext"]["objects"][0]["visibility"]

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "claimable_visibility_missing_object" for row in report["errors"]), report)

    def test_contract_rejects_claimable_generated_object(self) -> None:
        payload = minimal_bounded_context()
        payload["boundedGraphContext"]["objects"][0]["objectType"] = "document"
        payload["boundedGraphContext"]["objects"][0]["claimAllowed"] = True
        payload["boundedGraphContext"]["objects"][0]["proofState"] = "source_observed"
        payload["boundedGraphContext"]["objects"][0].pop("claimGateReason", None)
        payload["boundedGraphContext"]["objects"][0]["source"] = "cubicle_ai"

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "claimable_generated_object" for row in report["errors"]), report)

    def test_contract_rejects_claimable_open_graph_object(self) -> None:
        payload = minimal_bounded_context()
        payload["boundedGraphContext"]["objects"][0]["claimAllowed"] = True
        payload["boundedGraphContext"]["objects"][0]["proofState"] = "source_observed"
        payload["boundedGraphContext"]["objects"][0].pop("claimGateReason", None)

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "claimable_open_graph_object" for row in report["errors"]), report)

    def test_contract_rejects_claimable_association_without_evidence_row(self) -> None:
        payload = minimal_bounded_context()
        payload["boundedGraphContext"]["associations"] = [
            {
                "key": "assoc:unsafe",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:missing",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "missing_evidence_row" for row in report["errors"]), report)

    def test_contract_rejects_claimable_association_with_missing_endpoint_object(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:assoc"}]
        context["associations"] = [
            {
                "key": "assoc:missing-endpoint",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:missing"},
                "evidenceKey": "evidence:assoc",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "claimable_association_endpoint_missing" for row in report["errors"]), report)

    def test_contract_rejects_claimable_association_to_partial_endpoint_object(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:assoc"}]
        context["objects"][1]["claimAllowed"] = False
        context["objects"][1]["claimGateReason"] = "object_partial_requires_hydration"
        context["objects"][1]["freshnessState"] = "partial"
        context["associations"] = [
            {
                "key": "assoc:partial-endpoint",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:assoc",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "claimable_association_endpoint_not_current" for row in report["errors"]), report)

    def test_contract_rejects_claimable_duplicate_logical_associations(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:assoc-a"}, {"key": "evidence:assoc-b"}]
        context["associations"] = [
            {
                "key": "assoc:duplicate-a",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:assoc-a",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            },
            {
                "key": "assoc:duplicate-b",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:assoc-b",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            },
        ]

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "claimable_duplicate_logical_association" for row in report["errors"]), report)

    def test_contract_rejects_claimable_association_with_generated_evidence(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:generated-assoc", "source": "cubicle_ai"}]
        context["associations"] = [
            {
                "key": "assoc:generated-evidence",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:generated-assoc",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        self.assertTrue(any(row["kind"] == "claimable_generated_relationship_evidence" for row in report["errors"]), report)

    def test_contract_rejects_absence_claims_without_relation_source_time_scope(self) -> None:
        payload = minimal_bounded_context()
        payload["boundedGraphContext"]["coverage"] = {
            "coverageState": "complete",
            "absenceClaimsAllowed": True,
            "absenceClaimGateReason": "exact_scope",
        }

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_contract"])
        kinds = {row["kind"] for row in report["errors"]}
        self.assertIn("absence_relation_scope_missing", kinds)
        self.assertIn("absence_source_scope_missing", kinds)

    def test_connector_profile_allows_source_neutral_person_without_source_instance(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["coverage"] = {
            "coverageState": "complete",
            "absenceClaimsAllowed": False,
            "absenceClaimGateReason": "principal_coverage_required",
            "absenceClaimAssociationTypes": ["assignee"],
            "sourceSystem": "jira",
            "sourceInstance": "company-ai-first-minimum",
            "coverageWindowStart": "2026-06-17T10:00:00Z",
            "coverageWindowEnd": "2026-06-24T10:00:00Z",
        }
        context["objects"][0].update(
            {
                "objectType": "person",
                "key": "person:alice",
                "title": "Alice",
            }
        )
        context["objects"][0].pop("sourceInstance", None)
        context["objects"][1]["sourceInstance"] = "company-ai-first-minimum"

        report = bounded_graph_contract.validate_bounded_graph_context_payload(payload, profile="connector")

        self.assertTrue(report["passes_contract"], report)
        self.assertFalse(
            any(row["path"] == "$.boundedGraphContext.objects[0].sourceInstance" for row in report["warnings"]),
            report,
        )

    def test_promotion_audit_reports_ready_and_blocked_associations(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:ready", "source": "pagerduty"}]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            },
            {
                "key": "assoc:candidate",
                "associationType": "possible_followup_for",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "claimAllowed": False,
                "claimGateReason": "candidate_link_requires_human_review",
            },
        ]

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(payload)

        self.assertTrue(report["passes_promotion_audit"], report)
        self.assertEqual(report["promotable_association_count"], 1, report)
        blocked = {row["key"]: row for row in report["associations"]}
        self.assertTrue(blocked["assoc:ready"]["promotionReady"], report)
        self.assertIn("candidate_link_requires_human_review", blocked["assoc:candidate"]["blockers"], report)

    def test_promotion_audit_surfaces_contract_blockers(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:generated", "source": "cubicle_ai"}]
        context["associations"] = [
            {
                "key": "assoc:generated",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:generated",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(payload)

        self.assertFalse(report["passes_promotion_audit"], report)
        self.assertTrue(any(row["kind"] == "contract_error" for row in report["blockers"]), report)
        self.assertIn("relationship_generated_requires_source_evidence", report["associations"][0]["blockers"], report)

    def test_promotion_audit_accepts_authoritative_relationship_source(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:ready", "source": "pagerduty"}]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]
        policy = {"has_runbook": {"presence_sources": ["pagerduty"]}}

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy=bounded_graph_promotion_audit.normalize_source_authority_policy(policy),
        )

        self.assertTrue(report["source_authority_policy_applied"], report)
        self.assertTrue(report["passes_promotion_audit"], report)
        self.assertEqual(report["associations"][0]["evidenceSource"], "pagerduty", report)
        self.assertTrue(report["associations"][0]["promotionReady"], report)

    def test_promotion_audit_accepts_authoritative_relationship_source_instance(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [
            {
                "key": "evidence:ready",
                "source": "github",
                "sourceInstance": "github.com/apache/flink-kubernetes-operator",
                "locatorKind": "github_pull_request",
            }
        ]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]
        policy = {
            "has_runbook": {
                "presence_sources": ["github"],
                "presence_source_instances": {"github": ["github.com/apache/flink-kubernetes-operator"]},
                "presence_locator_kinds": {"github": ["github_pull_request"]},
            }
        }

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy=bounded_graph_promotion_audit.normalize_source_authority_policy(policy),
        )

        self.assertTrue(report["passes_promotion_audit"], report)
        self.assertEqual(
            report["associations"][0]["evidenceSourceInstance"],
            "github.com/apache/flink-kubernetes-operator",
            report,
        )

    def test_promotion_audit_rejects_unauthoritative_relationship_source_instance(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [
            {
                "key": "evidence:ready",
                "source": "github",
                "sourceInstance": "github.com/other/repo",
                "locatorKind": "github_pull_request",
            }
        ]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]
        policy = {
            "has_runbook": {
                "presence_sources": ["github"],
                "presence_source_instances": {"github": ["github.com/apache/flink-kubernetes-operator"]},
                "presence_locator_kinds": {"github": ["github_pull_request"]},
            }
        }

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy=bounded_graph_promotion_audit.normalize_source_authority_policy(policy),
        )

        self.assertFalse(report["passes_promotion_audit"], report)
        self.assertIn(
            "relationship_source_instance_not_authoritative_for_presence",
            report["associations"][0]["blockers"],
            report,
        )

    def test_promotion_audit_accepts_authoritative_relationship_mapper_version(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [
            {
                "key": "evidence:ready",
                "source": "github",
                "sourceInstance": "github.com/apache/flink-kubernetes-operator",
                "locatorKind": "github_pull_request",
            }
        ]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "mapperVersion": "github-pr-linker:v1",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]
        policy = {
            "has_runbook": {
                "presence_sources": ["github"],
                "presence_source_instances": {"github": ["github.com/apache/flink-kubernetes-operator"]},
                "presence_mapper_versions": {"github": ["github-pr-linker:v1"]},
                "presence_locator_kinds": {"github": ["github_pull_request"]},
            }
        }

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy=bounded_graph_promotion_audit.normalize_source_authority_policy(policy),
        )

        self.assertTrue(report["passes_promotion_audit"], report)
        self.assertEqual(report["associations"][0]["mapperVersion"], "github-pr-linker:v1", report)

    def test_promotion_audit_rejects_unauthoritative_relationship_mapper_version(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [
            {
                "key": "evidence:ready",
                "source": "github",
                "sourceInstance": "github.com/apache/flink-kubernetes-operator",
                "locatorKind": "github_pull_request",
            }
        ]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "mapperVersion": "github-pr-linker:v0",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]
        policy = {
            "has_runbook": {
                "presence_sources": ["github"],
                "presence_source_instances": {"github": ["github.com/apache/flink-kubernetes-operator"]},
                "presence_mapper_versions": {"github": ["github-pr-linker:v1"]},
                "presence_locator_kinds": {"github": ["github_pull_request"]},
            }
        }

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy=bounded_graph_promotion_audit.normalize_source_authority_policy(policy),
        )

        self.assertFalse(report["passes_promotion_audit"], report)
        self.assertEqual(report["associations"][0]["mapperVersion"], "github-pr-linker:v0", report)
        self.assertIn(
            "relationship_mapper_version_not_authoritative_for_presence",
            report["associations"][0]["blockers"],
            report,
        )

    def test_promotion_audit_rejects_unauthoritative_relationship_source(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:ready", "source": "pagerduty"}]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]
        policy = {"has_runbook": {"presence_sources": ["docs"]}}

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy=bounded_graph_promotion_audit.normalize_source_authority_policy(policy),
        )

        self.assertFalse(report["passes_promotion_audit"], report)
        self.assertIn(
            "relationship_source_not_authoritative_for_presence",
            report["associations"][0]["blockers"],
            report,
        )
        self.assertTrue(any(row["kind"] == "association_promotion_blocker" for row in report["blockers"]), report)

    def test_promotion_audit_does_not_let_association_self_attest_authority(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:ready"}]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "source": "pagerduty",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]
        policy = {"has_runbook": {"presence_sources": ["pagerduty"]}}

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy=bounded_graph_promotion_audit.normalize_source_authority_policy(policy),
        )

        self.assertFalse(report["passes_promotion_audit"], report)
        self.assertEqual(report["associations"][0]["evidenceSource"], "", report)
        self.assertIn(
            "relationship_source_authority_missing_evidence_source",
            report["associations"][0]["blockers"],
            report,
        )

    def test_promotion_audit_rejects_unauthoritative_relationship_locator_kind(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:ready", "source": "pagerduty", "locatorKind": "incident_note"}]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]
        policy = {
            "has_runbook": {
                "presence_sources": ["pagerduty"],
                "presence_locator_kinds": {"pagerduty": ["runbook_link"]},
            }
        }

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy=bounded_graph_promotion_audit.normalize_source_authority_policy(policy),
        )

        self.assertFalse(report["passes_promotion_audit"], report)
        self.assertEqual(report["associations"][0]["evidenceSource"], "pagerduty", report)
        self.assertEqual(report["associations"][0]["evidenceLocatorKind"], "incident_note", report)
        self.assertIn(
            "relationship_locator_not_authoritative_for_presence",
            report["associations"][0]["blockers"],
            report,
        )

    def test_promotion_audit_requires_policy_for_claimable_relationship_family(self) -> None:
        payload = minimal_bounded_context()
        context = payload["boundedGraphContext"]
        context["evidence"] = [{"key": "evidence:ready", "source": "pagerduty"}]
        context["associations"] = [
            {
                "key": "assoc:ready",
                "associationType": "has_runbook",
                "from": {"objectType": "incident", "key": "incident:payments-latency"},
                "to": {"objectType": "runbook_document", "key": "runbook:payments-latency"},
                "evidenceKey": "evidence:ready",
                "claimAllowed": True,
                "visibility": "public",
                "freshnessState": "fresh",
                "proofState": "source_observed",
                "confidence": 1,
            }
        ]

        report = bounded_graph_promotion_audit.audit_bounded_graph_context_payload(
            payload,
            source_authority_policy={"documents": {"presence_sources": ["docs"], "absence_sources": ["docs"]}},
        )

        self.assertFalse(report["passes_promotion_audit"], report)
        self.assertIn("relationship_authority_policy_missing", report["associations"][0]["blockers"], report)


def minimal_bounded_context() -> dict[str, object]:
    return {
        "boundedGraphContext": {
            "contextHash": "contractctx12345",
            "scopeMode": "bounded_graph_context",
            "seed": {"objectType": "incident", "key": "incident:payments-latency"},
            "depth": 1,
            "limitPerObject": 4,
            "coverage": {
                "coverageState": "sparse",
                "absenceClaimsAllowed": False,
                "absenceClaimGateReason": "partial_incident_sources",
            },
            "objects": [
                {
                    "objectType": "incident",
                    "key": "incident:payments-latency",
                    "title": "Payments latency incident",
                    "claimAllowed": False,
                    "claimGateReason": "open_graph_object_context_only",
                    "proofState": "candidate",
                    "visibility": "public",
                    "freshnessState": "fresh",
                },
                {
                    "objectType": "runbook_document",
                    "key": "runbook:payments-latency",
                    "title": "Payments latency runbook",
                    "claimAllowed": False,
                    "claimGateReason": "open_graph_object_context_only",
                    "proofState": "candidate",
                    "visibility": "public",
                    "freshnessState": "fresh",
                },
            ],
            "associations": [],
        }
    }


if __name__ == "__main__":
    unittest.main()
