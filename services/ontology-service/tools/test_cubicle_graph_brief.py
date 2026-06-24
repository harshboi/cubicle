#!/usr/bin/env python3

from __future__ import annotations

import importlib.util
import pathlib
import sqlite3
import sys
import tempfile
import unittest


MODULE_PATH = pathlib.Path(__file__).with_name("cubicle_graph_brief.py")
MIXED_EVAL_PACK = pathlib.Path(__file__).with_name("eval_packs") / "ai_first_mixed_minimum"
BOUNDED_GRAPH_PACK = pathlib.Path(__file__).with_name("eval_packs") / "bounded_graph_minimum"
BOUNDED_GRAPH_AUTH_LIMITED_PACK = pathlib.Path(__file__).with_name("eval_packs") / "bounded_graph_auth_limited"
COMPANY_AI_FIRST_PACK = pathlib.Path(__file__).with_name("eval_packs") / "company_ai_first_minimum"
SPEC = importlib.util.spec_from_file_location("cubicle_graph_brief", MODULE_PATH)
graph_brief = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
sys.modules["cubicle_graph_brief"] = graph_brief
SPEC.loader.exec_module(graph_brief)


class CubicleGraphBriefTest(unittest.TestCase):
    def test_build_context_bundle_collects_cited_work_neighborhood(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)

            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="workstream:fixture",
                source_instance="fixture-source",
                item_limit=5,
                edge_limit=10,
                evidence_limit=10,
                traversal_depth=2,
            )

        self.assertEqual(context["seed"]["key"], "workstream:fixture")
        self.assertEqual(context["seed"]["source_instance"], "fixture-source")
        self.assertEqual(len(context["rows"]["work_program_items"]), 2)
        self.assertEqual(len(context["rows"]["work_actions"]), 1)
        self.assertEqual(context["traversal"]["depth"], 2)
        self.assertIn("FLINK-2", context["traversal"]["reached_subject_keys"])
        self.assertEqual(len(context["rows"]["work_insights"]), 2)
        self.assertNotIn(
            "work-insight:generated-summary-launder",
            {row["key"] for row in context["rows"]["work_insights"]},
        )
        self.assertNotIn(
            "work-insight:misclassified-generated-summary",
            {row["key"] for row in context["rows"]["work_insights"]},
        )
        self.assertEqual(len(context["rows"]["work_item_forecasts"]), 1)
        self.assertEqual(len(context["rows"]["work_dependency_edges"]), 3)
        self.assertEqual(len(context["rows"]["evidence"]), 1)
        self.assertEqual(context["analytics"]["blocker_candidate_count"], 7)
        self.assertIn("Do not make ETA commitments", " ".join(context["guardrails"]))
        self.assertIn("Distinguish confirmed blockers from 7 blocker candidates", " ".join(context["guardrails"]))
        self.assertRegex(context["context_hash"], r"^[0-9a-f]{16}$")

    def test_render_brief_separates_risk_triage_from_product_claims(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_run_boundary_tables(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        brief = graph_brief.render_brief(context)

        self.assertIn("# Cubicle Graph Brief PoC", brief)
        self.assertIn("ETA readiness is `false`", brief)
        self.assertIn("risk ordering, not date commitments", brief)
        self.assertIn("3 dependency edge(s)", brief)
        self.assertIn("across 4 traversed node(s)", brief)
        self.assertIn("decision=product_action(raw); safe_use=owner/status follow-up", brief)
        self.assertIn("product_action(raw); safe_use=gated follow-up / open", brief)
        self.assertIn("7 blocker candidate(s) needing validation", brief)
        self.assertIn("[work_program_items:work-program-item:pr-1]", brief)
        self.assertIn("[work_actions:work-action:review-pr-1]", brief)
        self.assertIn("[guardrail:", brief)
        self.assertIn("Use only cited rows", brief)

    def test_render_prompt_contains_answer_contract_and_allowed_citations(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_run_boundary_tables(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        prompt = graph_brief.render_prompt(context)

        self.assertIn("# Cubicle Graph Brief Prompt", prompt)
        self.assertIn("Cite every bullet", prompt)
        self.assertIn("never write a fourth bullet", prompt)
        self.assertIn("Stop immediately after the final", prompt)
        self.assertIn("Use bullet lists only; do not use Markdown tables", prompt)
        self.assertIn("Do not invent citation aliases", prompt)
        self.assertIn("Measurement and precision readiness: [analytics:tpm_evaluation_readiness]", prompt)
        self.assertIn("Do not turn blocker candidates into confirmed blockers", prompt)
        self.assertIn("[context:" + context["context_hash"] + "]", prompt)
        self.assertIn("[work_program_items:work-program-item:pr-1]", prompt)
        self.assertIn('"guardrails"', prompt)
        self.assertIn("```json", prompt)
        self.assertNotIn("false_positive_forecast_risk", prompt)

    def test_render_prompt_generic_mode_targets_graph_safety(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        prompt = graph_brief.render_prompt(context, mode="generic")

        self.assertIn("AI graph-context analyst", prompt)
        self.assertIn("bounded traversal shape", prompt)
        self.assertIn("at least one claimable row", prompt)
        self.assertIn("derived topology edges", prompt)
        self.assertIn("graph-safety brief", prompt)
        self.assertIn("[context:" + context["context_hash"] + "]", prompt)
        self.assertIn("[work_dependency_edges:work-dependency-edge:ticket-pr]", prompt)

    def test_main_writes_generic_prompt_mode(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology_db = root / "ontology.db"
            seed_ontology_db(ontology_db)
            prompt_md = root / "generic_prompt.md"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--ontology-db",
                str(ontology_db),
                "--workstream-key",
                "fixture",
                "--source-instance",
                "fixture-source",
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--prompt-md",
                str(prompt_md),
                "--prompt-mode",
                "generic",
            ]
            try:
                graph_brief.main()
            finally:
                sys.argv = old_argv

            prompt = prompt_md.read_text(encoding="utf-8")

        self.assertIn("AI graph-context analyst", prompt)
        self.assertIn("graph-safety brief", prompt)

    def test_render_prompt_minimizes_private_evidence_source_fields(self) -> None:
        context = {
            "seed": {"key": "workstream:fixture", "source_instance": "fixture-source"},
            "context_hash": "abc123def4567890",
            "rows": {
                "work_actions": [
                    {
                        "key": "work-action:private",
                        "action_type": "review",
                        "source_url": "https://private.example/action",
                        "_table": "work_actions",
                    }
                ],
                "evidence": [
                    {
                        "key": "evidence:private",
                        "locator": "private-locator-token",
                        "excerpt": "private excerpt text",
                        "source_url": "https://private.example/evidence",
                        "proof_state": "observed",
                        "_table": "evidence",
                    }
                ],
            },
            "analytics": {},
            "guardrails": [],
            "citations": [
                {
                    "ref": "[evidence:evidence:private]",
                    "claimAllowed": True,
                    "excerptAllowed": False,
                    "sourceUrlAllowed": False,
                }
            ],
        }

        prompt = graph_brief.render_prompt(context)

        self.assertNotIn("https://private.example/action", prompt)
        self.assertNotIn("https://private.example/evidence", prompt)
        self.assertNotIn("private-locator-token", prompt)
        self.assertNotIn("private excerpt text", prompt)

    def test_render_generic_graph_baseline_uses_contract_and_citations(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        baseline = graph_brief.render_generic_graph_baseline(context)
        evaluation = graph_brief.evaluate_llm_brief(context, baseline)

        self.assertIn("## Confirmed Facts", baseline)
        self.assertIn("## Validation Leads", baseline)
        self.assertIn("## What Not To Claim", baseline)
        self.assertIn("[context:" + context["context_hash"] + "]", baseline)
        self.assertIn("[work_program_items:work-program-item:pr-1]", baseline)
        self.assertIn("[guardrail:" + context["context_hash"] + "]", baseline)
        self.assertNotIn("confirmed blockers", baseline.lower())
        self.assertTrue(evaluation["passes_smoke_eval"])

    def test_bounded_graph_context_supports_non_workprogram_graph(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "docmsgctx1234567",
                "seed": {"objectType": "document", "key": "doc:architecture-note"},
                "depth": 2,
                "limitPerObject": 4,
                "coverage": {
                    "coverageState": "sparse",
                    "absenceClaimsAllowed": False,
                    "absenceClaimGateReason": "partial_message_history",
                    "summary": "Only selected document and message rows were loaded.",
                },
                "guardrails": ["Do not claim missing replies, owners, or blockers from sparse message history."],
                "objects": [
                    {
                        "objectType": "document",
                        "key": "doc:architecture-note",
                        "title": "Architecture note",
                        "claimAllowed": True,
                        "proofState": "source_observed",
                    },
                    {
                        "objectType": "message",
                        "key": "message:standup-1",
                        "title": "Standup mention of rollout risk",
                        "claimAllowed": True,
                        "proofState": "source_observed",
                    },
                    {
                        "objectType": "ticket",
                        "key": "ticket:SUP-101",
                        "title": "Support escalation",
                        "claimAllowed": True,
                        "proofState": "source_observed",
                    },
                ],
                "associations": [
                    {
                        "key": "assoc:doc-message",
                        "associationType": "mentions",
                        "from": {"objectType": "document", "key": "doc:architecture-note"},
                        "to": {"objectType": "message", "key": "message:standup-1"},
                        "evidenceKey": "evidence:doc-message",
                        "confidence": 1,
                        "visibility": "public",
                        "freshnessState": "fresh",
                        "claimAllowed": True,
                        "proofState": "source_observed",
                        "claimGateReason": "source_evidence_full_confidence",
                    },
                    {
                        "key": "assoc:message-ticket-candidate",
                        "associationType": "possible_followup_for",
                        "from": {"objectType": "message", "key": "message:standup-1"},
                        "to": {"objectType": "ticket", "key": "ticket:SUP-101"},
                        "claimAllowed": False,
                        "claimGateReason": "candidate_link_requires_human_review",
                    },
                ],
                "evidence": [
                    {
                        "key": "evidence:doc-message",
                        "source": "fixture",
                        "sourceInstance": "generic-bounded-graph-test",
                        "visibility": "public",
                        "freshnessState": "fresh",
                        "confidence": 1,
                    }
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")

            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        baseline = graph_brief.render_generic_graph_baseline(context)
        prompt = graph_brief.render_prompt(context, mode="generic")
        evaluation = graph_brief.evaluate_llm_brief(context, baseline)

        self.assertEqual(context["seed"]["key"], "doc:architecture-note")
        self.assertEqual(len(context["rows"]["graph_objects"]), 3)
        self.assertEqual(len(context["rows"]["graph_associations"]), 2)
        self.assertIn("3 object(s) and 2 association(s)", baseline)
        self.assertIn("[graph_associations:assoc:doc-message]", baseline)
        self.assertIn("[graph_associations:assoc:message-ticket-candidate]", baseline)
        self.assertIn("[guardrail:docmsgctx1234567]", baseline)
        self.assertNotIn("Flink", baseline)
        self.assertNotIn("WorkProgram", baseline)
        self.assertEqual(set(context["analytics"].keys()), {"source_coverage"})
        self.assertNotIn("[analytics:", prompt)
        self.assertNotIn('"analytics"', prompt)
        self.assertNotIn("forecast_summary", prompt)
        self.assertNotIn("measurement_readiness", prompt)
        self.assertNotIn("blocker_candidate_count", prompt)
        self.assertNotIn("TPM", prompt)
        self.assertNotIn("WorkProgram", prompt)
        self.assertNotIn("ETA", prompt)
        self.assertNotIn("blocker_candidate", prompt)
        self.assertIn('"graph_summary"', prompt)
        self.assertIn('"traversal_count_phrase": "3 object(s) and 2 association(s)"', prompt)
        self.assertIn('"claimable_association_count": 1', prompt)
        self.assertIn('"gated_association_count": 1', prompt)
        self.assertIn('"confirmed_fact_instruction"', prompt)
        self.assertIn('"endpoint_phrase": "`doc:architecture-note` -> `message:standup-1`"', prompt)
        self.assertIn('"association_type": "mentions"', prompt)
        self.assertIn("``from_key` -> `to_key``", prompt)
        self.assertNotIn("[analytics:", graph_brief.json.dumps(sorted(graph_brief.allowed_citations(context))))
        self.assertTrue(graph_brief.product_claims_are_gated(context))
        self.assertTrue(evaluation["passes_smoke_eval"], evaluation)

    def test_bounded_graph_context_normalizes_relation_scoped_absence_claims(self) -> None:
        def context_for(coverage: dict[str, object], association_types: list[str] | None = None) -> dict[str, object]:
            bounded_context: dict[str, object] = {
                "contextHash": "absencectx12345",
                "seed": {"objectType": "ticket", "key": "ticket:COMP-101"},
                "coverage": coverage,
                "objects": [
                    {"objectType": "ticket", "key": "ticket:COMP-101", "title": "Launch ticket", "claimAllowed": True}
                ],
                "associations": [],
            }
            if association_types is not None:
                bounded_context["associationTypes"] = association_types
            payload = {"boundedGraphContext": bounded_context}
            with tempfile.TemporaryDirectory() as tmp:
                path = pathlib.Path(tmp) / "bounded_graph.json"
                path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
                return graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        sparse = context_for(
            {
                "coverageState": "sparse",
                "absenceClaimsAllowed": True,
                "absenceClaimAssociationTypes": ["implemented_by"],
            },
            ["implemented_by"],
        )
        self.assertFalse(sparse["analytics"]["source_coverage"]["absence_claims_allowed"]["value"] == "true")
        self.assertEqual(
            sparse["analytics"]["source_coverage"]["absence_claims_allowed"]["note"],
            "source_coverage_not_complete",
        )
        self.assertFalse(graph_brief.absence_claims_allowed(sparse))

        unscoped = context_for(
            {
                "coverageState": "complete",
                "absenceClaimsAllowed": True,
                "absenceClaimAssociationTypes": [],
            },
            ["implemented_by"],
        )
        self.assertFalse(graph_brief.absence_claims_allowed(unscoped))
        self.assertEqual(
            unscoped["analytics"]["source_coverage"]["absence_claims_allowed"]["note"],
            "relation_path_coverage_required",
        )

        relation_only = context_for(
            {
                "coverageState": "complete",
                "absenceClaimsAllowed": True,
                "absenceClaimAssociationTypes": ["implemented_by"],
            },
            ["implemented_by"],
        )
        self.assertFalse(graph_brief.absence_claims_allowed(relation_only))
        self.assertEqual(
            relation_only["analytics"]["source_coverage"]["absence_claims_allowed"]["note"],
            "source_scope_coverage_required",
        )

        source_scoped = context_for(
            {
                "coverageState": "complete",
                "absenceClaimsAllowed": True,
                "absenceClaimAssociationTypes": ["implemented_by"],
                "sourceSystem": "jira",
                "sourceInstance": "company",
            },
            ["implemented_by"],
        )
        self.assertFalse(graph_brief.absence_claims_allowed(source_scoped))
        self.assertEqual(
            source_scoped["analytics"]["source_coverage"]["absence_claims_allowed"]["note"],
            "source_time_window_required",
        )

        fully_scoped = context_for(
            {
                "coverageState": "complete",
                "absenceClaimsAllowed": True,
                "absenceClaimAssociationTypes": ["implemented_by"],
                "sourceSystem": "jira",
                "sourceInstance": "company",
                "coverageWindowStart": "2026-06-24T00:00:00Z",
                "coverageWindowEnd": "2026-06-24T01:00:00Z",
            },
            ["implemented_by"],
        )
        self.assertTrue(graph_brief.absence_claims_allowed(fully_scoped))

        unfiltered = context_for(
            {
                "coverageState": "complete",
                "absenceClaimsAllowed": True,
                "absenceClaimAssociationTypes": ["implemented_by"],
                "sourceSystem": "jira",
                "sourceInstance": "company",
                "coverageWindowStart": "2026-06-24T00:00:00Z",
                "coverageWindowEnd": "2026-06-24T01:00:00Z",
            },
            None,
        )
        self.assertFalse(graph_brief.absence_claims_allowed(unfiltered))

        wildcard = context_for(
            {
                "coverageState": "complete",
                "absenceClaimsAllowed": True,
                "absenceClaimAssociationTypes": ["*"],
                "sourceSystem": "jira",
                "sourceInstance": "company",
                "coverageWindowStart": "2026-06-24T00:00:00Z",
                "coverageWindowEnd": "2026-06-24T01:00:00Z",
            },
            None,
        )
        self.assertTrue(graph_brief.absence_claims_allowed(wildcard))

    def test_bounded_graph_context_gates_conflicting_multi_evidence_association(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "multievidence123",
                "seed": {"objectType": "document", "key": "doc:multi-evidence"},
                "depth": 1,
                "limitPerObject": 4,
                "coverage": {
                    "coverageState": "sparse",
                    "absenceClaimsAllowed": False,
                    "absenceClaimGateReason": "partial_source_coverage",
                },
                "objects": [
                    {"objectType": "document", "key": "doc:multi-evidence", "title": "Multi evidence note", "claimAllowed": True},
                    {"objectType": "ticket", "key": "ticket:MULTI-1", "title": "Multi evidence ticket", "claimAllowed": True},
                ],
                "associations": [
                    {
                        "key": "assoc:doc-ticket-current",
                        "associationType": "documented_by",
                        "from": {"objectType": "document", "key": "doc:multi-evidence"},
                        "to": {"objectType": "ticket", "key": "ticket:MULTI-1"},
                        "evidenceKey": "evidence:current-doc-ticket",
                        "confidence": 1,
                        "visibility": "public",
                        "freshnessState": "fresh",
                        "proofState": "source_observed",
                        "claimAllowed": True,
                    },
                    {
                        "key": "assoc:doc-ticket-stale",
                        "associationType": "documented_by",
                        "from": {"objectType": "document", "key": "doc:multi-evidence"},
                        "to": {"objectType": "ticket", "key": "ticket:MULTI-1"},
                        "evidenceKey": "evidence:stale-doc-ticket",
                        "confidence": 1,
                        "visibility": "public",
                        "freshnessState": "stale",
                        "proofState": "source_observed",
                        "claimAllowed": True,
                    },
                ],
                "evidence": [
                    {"key": "evidence:current-doc-ticket"},
                    {"key": "evidence:stale-doc-ticket"},
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        associations = context["rows"]["graph_associations"]
        self.assertEqual(len(associations), 2)
        for row in associations:
            self.assertFalse(row["claim_allowed"], row)
            self.assertEqual(row["claim_gate_reason"], "relationship_multi_evidence_requires_review")
            self.assertEqual(row["proof_state"], "candidate")
            self.assertFalse(graph_brief.citation_is_claim_allowed_graph_association(context, graph_brief.row_citation(row)))

        baseline = graph_brief.render_generic_graph_baseline(context)
        self.assertNotIn("Claimable association `doc:multi-evidence` -> `ticket:MULTI-1`", baseline)
        self.assertIn("relationship_multi_evidence_requires_review", baseline)

    def test_bounded_graph_context_gates_partial_endpoint_relationship(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "partialendpoint1",
                "seed": {"objectType": "ticket", "key": "ticket:PARTIAL-1"},
                "depth": 1,
                "limitPerObject": 4,
                "coverage": {
                    "coverageState": "sparse",
                    "absenceClaimsAllowed": False,
                    "absenceClaimGateReason": "partial_source_coverage",
                },
                "objects": [
                    {
                        "objectType": "ticket",
                        "key": "ticket:PARTIAL-1",
                        "title": "Partial endpoint ticket",
                        "freshnessState": "fresh",
                        "claimAllowed": True,
                    },
                    {
                        "objectType": "pull_request",
                        "key": "pull-request:repo/example#101",
                        "title": "repo/example#101",
                        "freshnessState": "partial",
                        "claimAllowed": True,
                    },
                ],
                "associations": [
                    {
                        "key": "assoc:partial-ticket-pr",
                        "associationType": "implemented_by",
                        "from": {"objectType": "ticket", "key": "ticket:PARTIAL-1"},
                        "to": {"objectType": "pull_request", "key": "pull-request:repo/example#101"},
                        "evidenceKey": "evidence:partial-ticket-pr",
                        "confidence": 1,
                        "visibility": "public",
                        "freshnessState": "fresh",
                        "proofState": "source_observed",
                        "claimAllowed": True,
                    }
                ],
                "evidence": [{"key": "evidence:partial-ticket-pr"}],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        associations = context["rows"]["graph_associations"]
        self.assertEqual(len(associations), 1)
        row = associations[0]
        self.assertFalse(row["claim_allowed"], row)
        self.assertEqual(row["claim_gate_reason"], "relationship_endpoint_partial_requires_hydration")
        self.assertEqual(row["proof_state"], "candidate")
        self.assertFalse(graph_brief.citation_is_claim_allowed_graph_association(context, graph_brief.row_citation(row)))
        objects = {row["key"]: row for row in context["rows"]["graph_objects"]}
        self.assertFalse(objects["pull-request:repo/example#101"]["claim_allowed"])
        self.assertEqual(objects["pull-request:repo/example#101"]["claim_gate_reason"], "object_partial_requires_hydration")

        baseline = graph_brief.render_generic_graph_baseline(context)
        self.assertNotIn("Claimable association `ticket:PARTIAL-1` -> `pull-request:repo/example#101`", baseline)
        self.assertIn("relationship_endpoint_partial_requires_hydration", baseline)

    def test_bounded_graph_context_gates_missing_endpoint_relationship(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "missingendpoint1",
                "seed": {"objectType": "ticket", "key": "ticket:MISSING-ENDPOINT"},
                "coverage": {"coverageState": "sparse", "absenceClaimsAllowed": False},
                "objects": [
                    {
                        "objectType": "ticket",
                        "key": "ticket:MISSING-ENDPOINT",
                        "title": "Missing endpoint ticket",
                        "freshnessState": "fresh",
                        "claimAllowed": True,
                    }
                ],
                "associations": [
                    {
                        "key": "assoc:missing-endpoint",
                        "associationType": "implemented_by",
                        "from": {"objectType": "ticket", "key": "ticket:MISSING-ENDPOINT"},
                        "to": {"objectType": "pull_request", "key": "pull-request:repo/example#404"},
                        "evidenceKey": "evidence:missing-endpoint",
                        "confidence": 1,
                        "visibility": "public",
                        "freshnessState": "fresh",
                        "proofState": "source_observed",
                        "claimAllowed": True,
                    }
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        row = context["rows"]["graph_associations"][0]
        self.assertFalse(row["claim_allowed"], row)
        self.assertEqual(row["claim_gate_reason"], "relationship_endpoint_missing_requires_hydration")
        self.assertEqual(row["proof_state"], "candidate")

    def test_bounded_graph_context_gates_hidden_multi_evidence_count(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "hiddenmulti123",
                "seed": {"objectType": "ticket", "key": "ticket:HIDDEN-MULTI"},
                "coverage": {"coverageState": "sparse", "absenceClaimsAllowed": False},
                "objects": [
                    {"objectType": "ticket", "key": "ticket:HIDDEN-MULTI", "title": "Hidden multi ticket", "freshnessState": "fresh", "claimAllowed": True},
                    {"objectType": "pull_request", "key": "pull-request:repo/example#505", "title": "Hidden multi PR", "freshnessState": "fresh", "claimAllowed": True},
                ],
                "associations": [
                    {
                        "key": "assoc:hidden-multi",
                        "associationType": "implemented_by",
                        "from": {"objectType": "ticket", "key": "ticket:HIDDEN-MULTI"},
                        "to": {"objectType": "pull_request", "key": "pull-request:repo/example#505"},
                        "evidenceKey": "evidence:hidden-multi-current",
                        "evidenceCount": 2,
                        "confidence": 1,
                        "visibility": "public",
                        "freshnessState": "fresh",
                        "proofState": "source_observed",
                        "claimAllowed": True,
                    }
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        row = context["rows"]["graph_associations"][0]
        self.assertFalse(row["claim_allowed"], row)
        self.assertEqual(row["claim_gate_reason"], "relationship_multi_evidence_requires_review")
        self.assertEqual(row["proof_state"], "candidate")

    def test_bounded_graph_context_gates_partial_generated_and_restricted_objects(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "objectgates123",
                "seed": {"objectType": "document", "key": "doc:object-gates"},
                "coverage": {"coverageState": "sparse", "absenceClaimsAllowed": False},
                "objects": [
                    {"objectType": "document", "key": "doc:object-gates", "title": "Object gates", "freshnessState": "fresh", "claimAllowed": True},
                    {"objectType": "pull_request", "key": "pull-request:repo/example#202", "title": "Partial PR", "freshnessState": "partial", "claimAllowed": True},
                    {"objectType": "document", "key": "doc:generated", "title": "Generated doc", "source": "cubicle_ai", "freshnessState": "fresh", "claimAllowed": True},
                    {"objectType": "message", "key": "message:restricted", "title": "Restricted message", "visibility": "private", "freshnessState": "fresh", "claimAllowed": True},
                ],
                "associations": [
                    {
                        "key": "assoc:seed-partial-pr",
                        "associationType": "mentions",
                        "from": {"objectType": "document", "key": "doc:object-gates"},
                        "to": {"objectType": "pull_request", "key": "pull-request:repo/example#202"},
                        "claimAllowed": False,
                    },
                    {
                        "key": "assoc:seed-generated-doc",
                        "associationType": "mentions",
                        "from": {"objectType": "document", "key": "doc:object-gates"},
                        "to": {"objectType": "document", "key": "doc:generated"},
                        "claimAllowed": False,
                    },
                    {
                        "key": "assoc:seed-restricted-message",
                        "associationType": "mentions",
                        "from": {"objectType": "document", "key": "doc:object-gates"},
                        "to": {"objectType": "message", "key": "message:restricted"},
                        "claimAllowed": False,
                    },
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        objects = {row["key"]: row for row in context["rows"]["graph_objects"]}
        self.assertFalse(objects["pull-request:repo/example#202"]["claim_allowed"])
        self.assertEqual(objects["pull-request:repo/example#202"]["claim_gate_reason"], "object_partial_requires_hydration")
        self.assertFalse(objects["doc:generated"]["claim_allowed"])
        self.assertEqual(objects["doc:generated"]["claim_gate_reason"], "object_generated_requires_source_evidence")
        self.assertFalse(objects["message:restricted"]["claim_allowed"])
        self.assertEqual(objects["message:restricted"]["claim_gate_reason"], "object_visibility_restricted")

    def test_bounded_graph_minimum_pack_baseline_passes_golden_questions(self) -> None:
        context = graph_brief.build_context_bundle_from_bounded_graph_context_json(BOUNDED_GRAPH_PACK / "context.json")
        baseline = graph_brief.render_generic_graph_baseline(context)
        golden = graph_brief.json.loads((BOUNDED_GRAPH_PACK / "golden_questions.json").read_text(encoding="utf-8"))

        evaluation = graph_brief.evaluate_brief_for_gates(context, baseline, golden)

        claimable_associations = [
            row
            for row in context["rows"]["graph_associations"]
            if row.get("claim_allowed")
        ]
        self.assertTrue(claimable_associations)
        self.assertTrue(all(row.get("evidence_key") for row in claimable_associations))
        self.assertTrue(evaluation["passes_eval"], evaluation)
        self.assertEqual(evaluation["golden_eval"]["pass_count"], 5)
        self.assertEqual(evaluation["golden_eval"]["missing_required_categories"], [])
        self.assertEqual(evaluation["golden_eval"]["missing_required_source_coverage_states"], [])

    def test_bounded_graph_auth_limited_pack_baseline_passes_golden_questions(self) -> None:
        context = graph_brief.build_context_bundle_from_bounded_graph_context_json(BOUNDED_GRAPH_AUTH_LIMITED_PACK / "context.json")
        baseline = graph_brief.render_generic_graph_baseline(context)
        golden = graph_brief.json.loads((BOUNDED_GRAPH_AUTH_LIMITED_PACK / "golden_questions.json").read_text(encoding="utf-8"))

        evaluation = graph_brief.evaluate_brief_for_gates(context, baseline, golden)

        self.assertIn("[graph_associations:assoc:ticket-doc-auth-limited]", baseline)
        self.assertIn("403 and 429 source responses are coverage evidence only", baseline)
        self.assertNotIn("WorkProgram", baseline)
        self.assertNotIn("TPM", baseline)
        self.assertNotIn("[analytics:", baseline)
        self.assertTrue(evaluation["passes_eval"], evaluation)
        self.assertEqual(evaluation["golden_eval"]["pass_count"], 6)
        self.assertEqual(evaluation["golden_eval"]["missing_required_categories"], [])
        self.assertEqual(evaluation["golden_eval"]["missing_required_source_coverage_states"], [])

    def test_bounded_graph_confirmed_relationship_requires_association_citation(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "docmsgctx1234567",
                "seed": {"objectType": "message", "key": "message:standup-1"},
                "depth": 2,
                "limitPerObject": 4,
                "coverage": {
                    "coverageState": "sparse",
                    "absenceClaimsAllowed": False,
                    "absenceClaimGateReason": "partial_message_history",
                },
                "objects": [
                    {
                        "objectType": "message",
                        "key": "message:standup-1",
                        "title": "Standup mention of rollout risk",
                        "claimAllowed": True,
                    },
                    {
                        "objectType": "ticket",
                        "key": "ticket:SUP-101",
                        "title": "Support escalation",
                        "claimAllowed": True,
                    },
                ],
                "associations": [
                    {
                        "associationType": "possible_followup_for",
                        "from": {"objectType": "message", "key": "message:standup-1"},
                        "to": {"objectType": "ticket", "key": "ticket:SUP-101"},
                        "claimAllowed": False,
                        "claimGateReason": "candidate_link_requires_human_review",
                    }
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The message `message:standup-1` is confirmed to exist and is associated with the ticket `ticket:SUP-101` as a possible follow-up. [graph_objects:message:standup-1]",
                "## Validation Leads",
                "- The possible follow-up association requires human review. [graph_associations:association:message:standup-1:possible_followup_for:ticket:SUP-101:1]",
                "## What Not To Claim",
                "- Source coverage gates absence claims. [guardrail:docmsgctx1234567]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["citation_policy_violation_count"], 1)
        self.assertEqual(
            evaluation["citation_policy_violations"][0]["kind"],
            "confirmed_relationship_requires_claim_allowed_association_citation",
        )

    def test_bounded_graph_relationship_claim_requires_matching_association_kind(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "relkindctx12345",
                "seed": {"objectType": "ticket", "key": "ticket:COMP-101"},
                "coverage": {"coverageState": "sparse", "absenceClaimsAllowed": False},
                "objects": [
                    {"objectType": "ticket", "key": "ticket:COMP-101", "title": "Launch ticket", "claimAllowed": True},
                    {"objectType": "person", "key": "person:alice", "title": "Alice", "claimAllowed": True},
                    {"objectType": "document", "key": "document:company-plan", "title": "Plan", "claimAllowed": True},
                    {"objectType": "pull_request", "key": "pull-request:company/app#42", "title": "Launch PR", "claimAllowed": True},
                ],
                "associations": [
                    {
                        "key": "assoc:ticket-assignee",
                        "associationType": "assignee",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-101"},
                        "to": {"objectType": "person", "key": "person:alice"},
                        "claimAllowed": True,
                    },
                    {
                        "key": "assoc:ticket-doc",
                        "associationType": "documented_by",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-101"},
                        "to": {"objectType": "document", "key": "document:company-plan"},
                        "claimAllowed": True,
                    },
                    {
                        "key": "assoc:ticket-pr",
                        "associationType": "implemented_by",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-101"},
                        "to": {"objectType": "pull_request", "key": "pull-request:company/app#42"},
                        "claimAllowed": True,
                    },
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- Ticket `ticket:COMP-101` is documented by `document:company-plan` and implemented by `pull-request:company/app#42`. [graph_associations:assoc:ticket-assignee]",
                "## Validation Leads",
                "- Source coverage is sparse. [source_coverage:ticket:COMP-101]",
                "## What Not To Claim",
                "- Missing neighbors are unknown, not absent. [guardrail:relkindctx12345]",
            ]
        )
        repaired = graph_brief.repair_llm_brief(context, answer)
        repaired_evaluation = graph_brief.evaluate_llm_brief(context, repaired)
        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["citation_policy_violation_count"], 1)
        self.assertEqual(evaluation["unsupported_statement_count"], 1)
        unsupported_rows = [
            row
            for row in evaluation["statement_support"]["rows"]
            if row["support_status"] == "unsupported"
        ]
        self.assertEqual(len(unsupported_rows), 1)
        self.assertTrue(
            any(
                failure["kind"] == "confirmed_relationship_requires_claim_allowed_association_citation"
                for failure in unsupported_rows[0]["support_failures"]
            ),
            unsupported_rows,
        )
        self.assertEqual(
            evaluation["citation_policy_violations"][0]["kind"],
            "confirmed_relationship_requires_claim_allowed_association_citation",
        )
        self.assertNotIn("documented by `document:company-plan` and implemented by", repaired)
        self.assertTrue(repaired_evaluation["passes_smoke_eval"], repaired_evaluation)

    def test_statement_support_audit_marks_generic_baseline_supported(self) -> None:
        context = graph_brief.build_context_bundle_from_bounded_graph_context_json(BOUNDED_GRAPH_PACK / "context.json")
        baseline = graph_brief.render_generic_graph_baseline(context)

        evaluation = graph_brief.evaluate_llm_brief(context, baseline)

        self.assertTrue(evaluation["passes_smoke_eval"], evaluation)
        self.assertEqual(evaluation["unsupported_statement_count"], 0)
        self.assertEqual(
            evaluation["statement_support"]["statement_count"],
            evaluation["statement_support"]["supported_statement_count"],
        )
        self.assertTrue(
            any(
                row["section"] == "## Confirmed Facts"
                and row["support_status"] == "supported_confirmed_fact"
                for row in evaluation["statement_support"]["rows"]
            ),
            evaluation["statement_support"],
        )

    def test_bounded_graph_repair_keeps_canonical_guardrail_with_duplicate_citation(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "guardrailctx123",
                "seed": {"objectType": "person", "key": "person:alice"},
                "coverage": {"coverageState": "sparse", "absenceClaimsAllowed": False},
                "objects": [
                    {"objectType": "person", "key": "person:alice", "title": "Alice", "claimAllowed": True},
                    {"objectType": "ticket", "key": "ticket:COMP-101", "title": "Launch ticket", "claimAllowed": True},
                ],
                "associations": [
                    {
                        "key": "assoc:ticket-assignee",
                        "associationType": "assignee",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-101"},
                        "to": {"objectType": "person", "key": "person:alice"},
                        "claimAllowed": True,
                    }
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The graph context starts from `person:alice`. [context:guardrailctx123]",
                "## Validation Leads",
                "- Source coverage is sparse. [source_coverage:person:alice]",
                "## What Not To Claim",
                "- Missing relationships are not complete. [guardrail:guardrailctx123]",
            ]
        )
        repaired = graph_brief.repair_llm_brief(context, answer)

        self.assertNotIn("Missing relationships are not complete. [guardrail:guardrailctx123]", repaired)
        self.assertIn("Source coverage is `sparse`; absence claims remain gated by `source_coverage_gate`. [source_coverage:person:alice]", repaired)
        self.assertIn("Source coverage gates absence claims; missing neighbors are unknown, not absent. [guardrail:guardrailctx123]", repaired)

    def test_render_typed_row_baseline_for_bounded_graph_excludes_associations(self) -> None:
        context = graph_brief.build_context_bundle_from_bounded_graph_context_json(BOUNDED_GRAPH_AUTH_LIMITED_PACK / "context.json")

        baseline = graph_brief.render_typed_row_baseline(context)
        evaluation = graph_brief.evaluate_llm_brief(context, baseline)

        self.assertIn("typed-row baseline contains", baseline)
        self.assertIn("typed graph object row(s)", baseline)
        self.assertIn("[graph_objects:", baseline)
        self.assertIn("[source_coverage:", baseline)
        self.assertNotIn("[graph_associations:", baseline)
        self.assertIn("Relationship association rows are intentionally excluded", baseline)
        self.assertTrue(evaluation["passes_smoke_eval"], evaluation)

    def test_generic_bounded_graph_ordering_demotes_disconnected_distractors(self) -> None:
        context = {
            "seed": {"key": "message:launch-standup", "object_type": "message"},
        }
        associations = [
            {
                "_table": "graph_associations",
                "association_type": "implemented_by",
                "from_key": "ticket:COMP-999",
                "to_key": "pull-request:company/app#99",
            },
            {
                "_table": "graph_associations",
                "association_type": "documented_by",
                "from_key": "ticket:COMP-101",
                "to_key": "document:company-plan",
            },
            {
                "_table": "graph_associations",
                "association_type": "discussed_in",
                "from_key": "ticket:COMP-101",
                "to_key": "message:launch-standup",
            },
            {
                "_table": "graph_associations",
                "association_type": "implemented_by",
                "from_key": "ticket:COMP-101",
                "to_key": "pull-request:company/app#42",
            },
        ]

        ordered = graph_brief.generic_bounded_graph_ordered_associations(context, associations)

        self.assertEqual(
            [(row["from_key"], row["to_key"]) for row in ordered[:3]],
            [
                ("ticket:COMP-101", "message:launch-standup"),
                ("ticket:COMP-101", "pull-request:company/app#42"),
                ("ticket:COMP-101", "document:company-plan"),
            ],
        )
        self.assertEqual(ordered[-1]["from_key"], "ticket:COMP-999")

    def test_bounded_graph_repair_prioritizes_canonical_baseline_for_golden_paths(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "3c29a077d559836c",
                "seed": {"objectType": "message", "key": "message:launch-standup"},
                "coverage": {"coverageState": "sparse", "absenceClaimsAllowed": False},
                "objects": [
                    {"objectType": "message", "key": "message:launch-standup", "title": "Launch standup", "claimAllowed": True},
                    {"objectType": "ticket", "key": "ticket:COMP-101", "title": "Launch ticket", "claimAllowed": True},
                    {"objectType": "document", "key": "document:company-plan", "title": "Company plan", "claimAllowed": True},
                    {"objectType": "pull_request", "key": "pull-request:company/app#42", "title": "Launch PR", "claimAllowed": True},
                ],
                "associations": [
                    {
                        "key": "assoc:message-ticket",
                        "associationType": "discussed_in",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-101"},
                        "to": {"objectType": "message", "key": "message:launch-standup"},
                        "claimAllowed": True,
                    },
                    {
                        "key": "assoc:ticket-pr",
                        "associationType": "implemented_by",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-101"},
                        "to": {"objectType": "pull_request", "key": "pull-request:company/app#42"},
                        "claimAllowed": True,
                    },
                    {
                        "key": "assoc:ticket-document",
                        "associationType": "documented_by",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-101"},
                        "to": {"objectType": "document", "key": "document:company-plan"},
                        "claimAllowed": True,
                    },
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        model_answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The bounded graph traversal reaches a depth of 2 and includes 3 edges. [context:3c29a077d559836c]",
                "- The ticket `ticket:COMP-101` is documented by the company plan and implemented by the pull request. [graph_associations:assoc:ticket-document] [graph_associations:assoc:ticket-pr]",
                "## Validation Leads",
                "- The graph traversal includes related objects such as the company plan and pull request. [source_coverage:message:launch-standup]",
                "- The traversal includes a connected subgraph. [context:3c29a077d559836c]",
                "## What Not To Claim",
                "- Absence claims are gated due to sparse source coverage; missing neighbors cannot be confirmed as absent. [guardrail:3c29a077d559836c]",
            ]
        )
        repaired = graph_brief.repair_llm_brief(context, model_answer)
        golden = graph_brief.json.loads((COMPANY_AI_FIRST_PACK / "golden_message.json").read_text(encoding="utf-8"))
        golden_eval = graph_brief.evaluate_golden_questions(repaired, golden)

        self.assertIn("ticket:COMP-101` -> `message:launch-standup", repaired)
        self.assertIn("ticket:COMP-101` -> `pull-request:company/app#42", repaired)
        self.assertIn("ticket:COMP-101` -> `document:company-plan", repaired)
        self.assertIn("missing neighbors are unknown, not absent", repaired)
        self.assertTrue(golden_eval["passes_golden_eval"], golden_eval)

    def test_bounded_graph_context_marks_disconnected_rows_out_of_scope(self) -> None:
        payload = {
            "boundedGraphContext": {
                "contextHash": "visiblectx123456",
                "seed": {"objectType": "message", "key": "message:launch-standup"},
                "coverage": {"coverageState": "sparse", "absenceClaimsAllowed": False},
                "objects": [
                    {"objectType": "message", "key": "message:launch-standup", "title": "Launch standup", "claimAllowed": True},
                    {"objectType": "ticket", "key": "ticket:COMP-101", "title": "Launch ticket", "claimAllowed": True},
                    {"objectType": "ticket", "key": "ticket:COMP-999", "title": "Unrelated finance export", "claimAllowed": True},
                    {"objectType": "pull_request", "key": "pull-request:company/app#99", "title": "Export finance report", "claimAllowed": True},
                ],
                "associations": [
                    {
                        "key": "assoc:launch-message",
                        "associationType": "discussed_in",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-101"},
                        "to": {"objectType": "message", "key": "message:launch-standup"},
                        "claimAllowed": True,
                    },
                    {
                        "key": "visible-distractor:comp-999-pr-99",
                        "associationType": "implemented_by",
                        "from": {"objectType": "ticket", "key": "ticket:COMP-999"},
                        "to": {"objectType": "pull_request", "key": "pull-request:company/app#99"},
                        "claimAllowed": True,
                    },
                ],
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = pathlib.Path(tmp) / "bounded_graph.json"
            path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_bounded_graph_context_json(path)

        objects = {row["key"]: row for row in context["rows"]["graph_objects"]}
        associations = {row["key"]: row for row in context["rows"]["graph_associations"]}
        baseline = graph_brief.render_generic_graph_baseline(context)
        prompt = graph_brief.render_prompt(context, mode="generic")
        bad_answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The launch message is in context. [graph_objects:message:launch-standup]",
                "## Validation Leads",
                "- The disconnected finance ticket is out of scope. [graph_objects:ticket:COMP-999]",
                "## What Not To Claim",
                "- Missing neighbors are unknown, not absent. [guardrail:visiblectx123456]",
            ]
        )
        evaluation = graph_brief.evaluate_llm_brief(context, bad_answer)
        repaired = graph_brief.repair_llm_brief(context, bad_answer)
        guardrail_cited_distractor_answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The launch message is in context. [graph_objects:message:launch-standup]",
                "## Validation Leads",
                "- Source coverage is sparse. [source_coverage:message:launch-standup]",
                "## What Not To Claim",
                "- Do not claim `ticket:COMP-999` or `pull-request:company/app#99` are part of the seed component. [guardrail:visiblectx123456]",
            ]
        )
        guardrail_evaluation = graph_brief.evaluate_llm_brief(context, guardrail_cited_distractor_answer)
        guardrail_repaired = graph_brief.repair_llm_brief(context, guardrail_cited_distractor_answer)

        self.assertFalse(objects["ticket:COMP-999"]["seed_reachable"])
        self.assertFalse(associations["visible-distractor:comp-999-pr-99"]["seed_reachable"])
        self.assertFalse(objects["ticket:COMP-999"]["claim_allowed"])
        self.assertEqual(objects["ticket:COMP-999"]["claim_gate_reason"], "disconnected_from_seed_component")
        self.assertNotIn("COMP-999", baseline)
        self.assertIn('"seed_reachable": false', prompt)
        self.assertIn("do not mention it in any section", prompt)
        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["citation_policy_violations"][0]["kind"], "disconnected_seed_component_citation_not_allowed")
        self.assertNotIn("COMP-999", repaired)
        self.assertFalse(guardrail_evaluation["passes_smoke_eval"])
        self.assertEqual(guardrail_evaluation["semantic_guardrail_violations"][0]["guardrail"], "disconnected_seed_component_mentioned")
        self.assertNotIn("COMP-999", guardrail_repaired)
        self.assertNotIn("#99", guardrail_repaired)

    def test_render_typed_row_baseline_uses_only_typed_rows(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        baseline = graph_brief.render_typed_row_baseline(context)
        evaluation = graph_brief.evaluate_llm_brief(context, baseline)

        self.assertIn("typed-row baseline contains 2 work item(s) and 1 action row(s)", baseline)
        self.assertIn("[work_program_items:work-program-item:pr-1]", baseline)
        self.assertIn("[work_actions:work-action:review-pr-1]", baseline)
        self.assertNotIn("[work_dependency_edges:", baseline)
        self.assertNotIn("[work_insights:", baseline)
        self.assertNotIn("[work_item_forecasts:", baseline)
        self.assertNotIn("[analytics:", baseline)
        self.assertTrue(evaluation["passes_smoke_eval"])

    def test_generic_graph_baseline_passes_sparse_non_flink_ticket_context(self) -> None:
        payload = {
            "data": {
                "workProgramGraphContext": {
                    "sourceInstance": "ticket-only-fixture",
                    "generatedAt": "2026-06-24T12:00:00Z",
                    "scopeMode": "explicit_source:latest_run:work_program_run_packet_boundary_latest_graph_rows",
                    "runKey": "work-program-run:customer-onboarding",
                    "workstreamKey": "workstream:customer-onboarding",
                    "contextHash": "feedfacecafebeef",
                    "traversalDepth": 1,
                    "dependencyEdgeCount": 0,
                    "reachedSubjectKeys": ["ticket:SUP-101"],
                    "allowedCitations": [
                        "[context:feedfacecafebeef]",
                        "[guardrail:feedfacecafebeef]",
                        "[work_program_items:work-program-item:support-ticket-101]",
                        "[source_coverage:workstream:customer-onboarding]",
                    ],
                    "citations": [
                        {
                            "ref": "[context:feedfacecafebeef]",
                            "citationKind": "graph_context",
                            "nodeKind": "work_program_graph_context",
                            "nodeKey": "workstream:customer-onboarding",
                            "proofState": "derived_context",
                            "freshnessState": "current",
                            "visibility": "private",
                            "claimUse": "context_boundary",
                            "claimGateReason": "explicit_source",
                            "claimAllowed": True,
                            "excerptAllowed": False,
                            "sourceUrlAllowed": False,
                        },
                        {
                            "ref": "[work_program_items:work-program-item:support-ticket-101]",
                            "citationKind": "typed_row",
                            "nodeKind": "work_program_item",
                            "nodeKey": "work-program-item:support-ticket-101",
                            "proofState": "typed_row",
                            "freshnessState": "fresh",
                            "visibility": "private",
                            "claimUse": "product_action",
                            "claimGateReason": "product_action_gate_passed",
                            "claimAllowed": True,
                            "excerptAllowed": False,
                            "sourceUrlAllowed": False,
                        },
                        {
                            "ref": "[source_coverage:workstream:customer-onboarding]",
                            "citationKind": "derived_packet",
                            "nodeKind": "work_program_source_coverage_packet",
                            "nodeKey": "workstream:customer-onboarding",
                            "proofState": "derived_packet",
                            "freshnessState": "current",
                            "visibility": "private",
                            "claimUse": "source_coverage_gate",
                            "claimGateReason": "ticket_only_sparse_coverage",
                            "claimAllowed": False,
                            "excerptAllowed": False,
                            "sourceUrlAllowed": False,
                        },
                    ],
                    "items": [
                        {
                            "key": "work-program-item:support-ticket-101",
                            "subjectKind": "ticket",
                            "subjectKey": "ticket:SUP-101",
                            "title": "Support onboarding ticket",
                            "programStatus": "needs_review",
                            "decisionState": "product_action",
                            "dueBucket": "now",
                            "riskScore": 20,
                            "nextAction": "Ask the support owner to validate the requested onboarding path.",
                            "productActionAllowed": True,
                            "claimUse": "product_action",
                            "claimGateReason": "product_action_gate_passed",
                        }
                    ],
                    "actions": [],
                    "dependencyEdges": [],
                    "insights": [],
                    "forecasts": [],
                    "qualityGates": [],
                    "evidenceNeeds": [],
                    "forecastPacket": {
                        "etaForecastReady": False,
                        "readinessState": "no_forecast_data",
                        "automationSummary": "Forecast packet gates ETA claims for this ticket-only context.",
                    },
                    "guardrailPacket": {
                        "humanReviewRequired": True,
                        "readinessState": "human_review_required",
                        "automationSummary": "Human review is required before taking ticket action.",
                    },
                    "sourceCoveragePacket": {
                        "coverageState": "sparse",
                        "absenceClaimsAllowed": False,
                        "absenceClaimGateReason": "ticket_only_sparse_coverage",
                        "automationSummary": "Coverage is sparse; do not claim missing linked PRs or blockers.",
                    },
                    "llmTask": "Summarize the bounded customer-onboarding ticket context and its claim gates.",
                }
            }
        }
        golden = {
            "questions": [
                {
                    "key": "nonflink:ticket-only:safety",
                    "question": "Can the generic graph baseline summarize sparse ticket-only context safely?",
                    "expected_facts": [
                        {
                            "text": "bounded graph context contains 1 work item(s), 0 action(s), 0 insight(s), 0 forecast row(s), and 0 dependency edge(s)",
                            "citation": "[context:feedfacecafebeef]",
                        },
                        {
                            "text": "Support onboarding ticket",
                            "citation": "[work_program_items:work-program-item:support-ticket-101]",
                        },
                    ],
                    "expected_citations": ["[guardrail:feedfacecafebeef]"],
                    "forbidden_phrases": ["Flink", "TPM", "ETA commitment", "confirmed blocker"],
                    "required_sections": ["## What Not To Claim"],
                }
            ]
        }
        with tempfile.TemporaryDirectory() as tmp:
            graph_context_json = pathlib.Path(tmp) / "graph_context.json"
            graph_context_json.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_graph_context_json(graph_context_json)

        baseline = graph_brief.render_generic_graph_baseline(context)
        evaluation = graph_brief.evaluate_brief_for_gates(context, baseline, golden)

        self.assertIn("Support onboarding ticket", baseline)
        self.assertIn("Coverage is sparse; do not claim missing linked PRs or blockers.", baseline)
        self.assertNotIn("Flink", baseline)
        self.assertNotIn("TPM", baseline)
        self.assertTrue(evaluation["passes_smoke_eval"])
        self.assertTrue(evaluation["passes_eval"])

    def test_main_writes_generic_baseline_md(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology_db = root / "ontology.db"
            seed_ontology_db(ontology_db)
            baseline_md = root / "generic_baseline.md"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--ontology-db",
                str(ontology_db),
                "--workstream-key",
                "fixture",
                "--source-instance",
                "fixture-source",
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--generic-baseline-md",
                str(baseline_md),
            ]
            try:
                graph_brief.main()
            finally:
                sys.argv = old_argv

            baseline = baseline_md.read_text(encoding="utf-8")

        self.assertIn("# Operating Brief", baseline)
        self.assertIn("## What Not To Claim", baseline)

    def test_main_writes_typed_row_baseline_md(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology_db = root / "ontology.db"
            seed_ontology_db(ontology_db)
            baseline_md = root / "typed_row_baseline.md"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--ontology-db",
                str(ontology_db),
                "--workstream-key",
                "fixture",
                "--source-instance",
                "fixture-source",
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--typed-row-baseline-md",
                str(baseline_md),
            ]
            try:
                graph_brief.main()
            finally:
                sys.argv = old_argv

            baseline = baseline_md.read_text(encoding="utf-8")

        self.assertIn("# Operating Brief", baseline)
        self.assertIn("typed-row baseline contains", baseline)

    def test_main_refuses_llm_command_without_graph_context_json(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology_db = root / "ontology.db"
            seed_ontology_db(ontology_db)
            llm_brief_md = root / "llm.md"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--ontology-db",
                str(ontology_db),
                "--workstream-key",
                "fixture",
                "--source-instance",
                "fixture-source",
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--llm-command",
                "cat",
                "--llm-brief-md",
                str(llm_brief_md),
            ]
            try:
                with self.assertRaisesRegex(SystemExit, "--graph-context-json"):
                    graph_brief.main()
            finally:
                sys.argv = old_argv

        self.assertFalse(llm_brief_md.exists())

    def test_main_allows_graph_context_json_without_workstream_key(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            graph_context_json = root / "graph_context.json"
            write_fixture_graph_context_json(graph_context_json)
            llm_brief_md = root / "llm.md"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--graph-context-json",
                str(graph_context_json),
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--llm-command",
                "cat",
                "--llm-brief-md",
                str(llm_brief_md),
            ]
            try:
                graph_brief.main()
            finally:
                sys.argv = old_argv

            self.assertTrue(llm_brief_md.exists())

    def test_evaluate_llm_brief_accepts_cited_guardrailed_answer(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The context contains risk and dependency signals for the fixture workstream. "
                + f"[context:{context['context_hash']}]",
                "## Validation Leads",
                "- The risky PR should be treated as owner/status follow-up, not an ETA commitment. "
                + "[work_program_items:work-program-item:pr-1]",
                "## What Not To Claim",
                "- Do not claim blocker candidates are confirmed blockers. "
                + f"[guardrail:{context['context_hash']}]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertTrue(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["unknown_citation_count"], 0)
        self.assertEqual(evaluation["uncited_material_claim_line_count"], 0)
        self.assertEqual(evaluation["forbidden_claim_violation_count"], 0)

    def test_evaluate_llm_brief_flags_unknown_citations_uncited_claims_and_guardrail_breaks(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        answer = "\n".join(
            [
                "# Bad Brief",
                "- This PR will merge by Friday. [work_program_items:work-program-item:pr-1]",
                "- There is a confirmed blocker in the dependency path. [made_up:row]",
                "- The model is ready for product action.",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["unknown_citations"], ["[made_up:row]"])
        self.assertEqual(evaluation["uncited_material_claim_line_count"], 1)
        guardrails = {row["guardrail"] for row in evaluation["forbidden_claim_violations"]}
        self.assertIn("eta_not_ready", guardrails)
        self.assertIn("blockers_not_confirmed", guardrails)
        self.assertIn("product_claims_gated", guardrails)

    def test_evaluate_llm_brief_flags_complete_coverage_contradiction(self) -> None:
        context = {
            "context_hash": "covctx",
            "analytics": {
                "source_coverage": {
                    "coverage_state": {"value": "complete"},
                    "absence_claims_allowed": {"value": "false"},
                }
            },
            "graph_context": {
                "sourceCoveragePacket": {
                    "coverageState": "complete",
                    "absenceClaimsAllowed": False,
                    "absenceClaimGateReason": "source_coverage_evidence_needed",
                }
            },
            "allowed_citations": ["[context:covctx]", "[guardrail:covctx]"],
            "citations": [
                {"ref": "[context:covctx]", "claimAllowed": True},
                {"ref": "[guardrail:covctx]", "claimAllowed": False},
            ],
        }
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- Source coverage is complete, but absence claims remain gated. [context:covctx]",
                "## Validation Leads",
                "- Review evidence needs before making absence claims. [guardrail:covctx]",
                "## What Not To Claim",
                "- Do not claim source coverage as complete because evidence needs are present. [guardrail:covctx]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["semantic_guardrail_violation_count"], 1)
        self.assertEqual(evaluation["semantic_guardrail_violations"][0]["guardrail"], "source_coverage_complete_contradicted")

    def test_repair_llm_brief_removes_complete_coverage_contradiction(self) -> None:
        context = {
            "context_hash": "covctx",
            "analytics": {
                "source_coverage": {
                    "coverage_state": {"value": "complete"},
                    "absence_claims_allowed": {"value": "false"},
                }
            },
            "graph_context": {
                "sourceCoveragePacket": {
                    "coverageState": "complete",
                    "absenceClaimsAllowed": False,
                    "absenceClaimGateReason": "source_coverage_evidence_needed",
                }
            },
            "guardrails": ["Absence claims are gated by source coverage evidence needs."],
            "allowed_citations": ["[context:covctx]", "[guardrail:covctx]"],
            "citations": [
                {"ref": "[context:covctx]", "claimAllowed": True},
                {"ref": "[guardrail:covctx]", "claimAllowed": False},
            ],
        }
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- Source coverage is complete, but absence claims remain gated. [context:covctx]",
                "## Validation Leads",
                "- Review evidence needs before making absence claims. [guardrail:covctx]",
                "## What Not To Claim",
                "- Do not claim source coverage as complete because evidence needs are present. [guardrail:covctx]",
            ]
        )

        repaired = graph_brief.repair_llm_brief(context, answer)
        evaluation = graph_brief.evaluate_llm_brief(context, repaired)

        self.assertNotIn("Do not claim source coverage as complete", repaired)
        self.assertEqual(evaluation["semantic_guardrail_violation_count"], 0)

    def test_evaluate_llm_brief_flags_prompt_shape_violations(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        context_citation = f"[context:{context['context_hash']}]"
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                f"- One cited fact. {context_citation}",
                f"- Two cited facts. {context_citation}",
                f"- Three cited facts. {context_citation}",
                f"- Four cited facts. {context_citation}",
                "## Validation Leads",
                f"- One cited lead. {context_citation}",
                "## What Not To Claim",
                f"- One cited guardrail. [guardrail:{context['context_hash']}]",
                "This uncited concluding paragraph should not appear.",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        kinds = {row["kind"] for row in evaluation["structure_violations"]}
        self.assertIn("too_many_bullets", kinds)
        self.assertIn("nonbullet_material_line", kinds)

    def test_repair_llm_brief_enforces_contract_with_valid_cited_bullets(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        context_citation = f"[context:{context['context_hash']}]"
        bad_answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                f"- One cited fact. {context_citation}",
                f"- Two cited facts. {context_citation}",
                f"- Three cited facts. {context_citation}",
                f"- Four cited facts. {context_citation}",
                "## Validation Leads",
                "- This invented citation should be dropped. [made_up:row]",
            ]
        )

        repaired = graph_brief.repair_llm_brief(context, bad_answer)
        evaluation = graph_brief.evaluate_llm_brief(context, repaired)

        self.assertTrue(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["structure_violation_count"], 0)
        self.assertNotIn("Four cited facts", repaired)
        self.assertIn("## What Not To Claim", repaired)

    def test_evaluate_llm_brief_allows_eta_model_mentions_without_date_commitment(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )

        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The best model for ETA is still not product-safe. [analytics:tpm_forecast_summary]",
                "## Validation Leads",
                "- Add labels before product use. [analytics:tpm_evaluation_readiness]",
                "## What Not To Claim",
                f"- Do not claim an ETA date. [guardrail:{context['context_hash']}]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertTrue(evaluation["passes_smoke_eval"])

    def test_evaluate_golden_questions_scores_expected_facts_and_forbidden_claims(self) -> None:
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The fixture graph contains cited risk context. [context:abc123]",
                "## Validation Leads",
                "- Add labels before product use. [analytics:tpm_evaluation_readiness]",
                "## What Not To Claim",
                "- Do not claim an ETA date. [guardrail:abc123]",
            ]
        )
        golden = {
            "questions": [
                {
                    "key": "fixture:risk-context",
                    "question": "What should a TPM know?",
                    "expected_facts": [
                        {"text": "fixture graph contains cited risk context", "citation": "[context:abc123]"},
                        "Add labels before product use",
                    ],
                    "expected_citations": ["[analytics:tpm_evaluation_readiness]"],
                    "forbidden_phrases": ["will merge by Friday", "confirmed blocker"],
                    "required_sections": ["## What Not To Claim"],
                }
            ]
        }

        good = graph_brief.evaluate_golden_questions(answer, golden)
        bad = graph_brief.evaluate_golden_questions(
            answer + "\n- This PR will merge by Friday. [context:abc123]",
            {
                "questions": [
                    {
                        "key": "fixture:bad",
                        "expected_facts": ["missing required fact"],
                        "expected_citations": ["[work_program_items:missing]"],
                        "forbidden_phrases": ["will merge by Friday"],
                    }
                ]
            },
        )

        self.assertTrue(good["passes_golden_eval"])
        self.assertEqual(good["pass_count"], 1)
        self.assertFalse(bad["passes_golden_eval"])
        self.assertEqual(bad["failure_count"], 1)
        self.assertEqual(bad["questions"][0]["missing_expected_facts"], ["missing required fact"])
        self.assertEqual(bad["questions"][0]["missing_expected_citations"], ["[work_program_items:missing]"])
        self.assertEqual(bad["questions"][0]["forbidden_phrase_hits"], ["will merge by Friday"])

    def test_evaluate_golden_questions_requires_expected_fact_and_citation_same_line(self) -> None:
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The fixture graph contains cited risk context. [context:wrong]",
                "- A different claim has the expected citation. [context:abc123]",
                "## Validation Leads",
                "- Add labels before product use. [analytics:tpm_evaluation_readiness]",
                "## What Not To Claim",
                "- Do not claim an ETA date. [guardrail:abc123]",
            ]
        )
        golden = {
            "questions": [
                {
                    "key": "fixture:same-line",
                    "expected_facts": [
                        {"text": "fixture graph contains cited risk context", "citation": "[context:abc123]"},
                    ],
                }
            ]
        }

        evaluation = graph_brief.evaluate_golden_questions(answer, golden)

        self.assertFalse(evaluation["passes_golden_eval"])
        self.assertEqual(
            evaluation["questions"][0]["missing_expected_facts"],
            [{"text": "fixture graph contains cited risk context", "citation": "[context:abc123]"}],
        )

    def test_evaluate_golden_questions_supports_alternatives_and_citation_prefix(self) -> None:
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The bounded traversal explores a 2-depth graph and reaches 18 subject keys. [context:abc123]",
                "- A product_action row needs owner follow-up. [work_program_items:item-1]",
                "## Validation Leads",
                "- A partial_remote_link edge is incomplete dependency coverage. [work_dependency_edges:edge-1]",
                "## What Not To Claim",
                "- Do not make ETA commitments. [guardrail:abc123]",
            ]
        )
        golden = {
            "questions": [
                {
                    "key": "fixture:generic",
                    "expected_facts": [
                        {
                            "any_of": [
                                {"text": "6 work item(s)", "citation": "[context:abc123]"},
                                {"text": "2-depth graph", "citation": "[context:abc123]"},
                            ]
                        },
                        {"text": "product_action", "citation_prefix": "[work_program_items:"},
                        {"text": "incomplete dependency coverage", "citation_prefix": "[work_dependency_edges:"},
                    ],
                }
            ]
        }

        evaluation = graph_brief.evaluate_golden_questions(answer, golden)

        self.assertTrue(evaluation["passes_golden_eval"])

    def test_evaluate_golden_questions_ignores_forbidden_phrase_inside_explicit_prohibition(self) -> None:
        good_answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The fixture graph contains cited risk context. [context:abc123]",
                "## Validation Leads",
                "- Add labels before product use. [analytics:tpm_evaluation_readiness]",
                "## What Not To Claim",
                "- Do not claim a confirmed blocker. [guardrail:abc123]",
            ]
        )
        bad_answer = good_answer + "\n- This is a confirmed blocker. [context:abc123]"
        golden = {"questions": [{"key": "fixture:forbidden", "forbidden_phrases": ["confirmed blocker"]}]}

        good = graph_brief.evaluate_golden_questions(good_answer, golden)
        bad = graph_brief.evaluate_golden_questions(bad_answer, golden)

        self.assertTrue(good["passes_golden_eval"])
        self.assertFalse(bad["passes_golden_eval"])
        self.assertEqual(bad["questions"][0]["forbidden_phrase_hits"], ["confirmed blocker"])

    def test_evaluate_golden_questions_ignores_locally_negated_forbidden_phrase(self) -> None:
        good_answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- Forecast risk is not an actual ETA forecast. [analytics:tpm_forecast_summary]",
                "## Validation Leads",
                "- Treat this as risk triage and not ETA commitment. [analytics:tpm_forecast_summary]",
                "## What Not To Claim",
                "- Keep date claims gated. [guardrail:abc123]",
            ]
        )
        bad_answer = good_answer + "\n- This is an actual ETA forecast. [analytics:tpm_forecast_summary]"
        golden = {"questions": [{"key": "fixture:negated", "forbidden_phrases": ["actual ETA forecast", "ETA commitment"]}]}

        good = graph_brief.evaluate_golden_questions(good_answer, golden)
        bad = graph_brief.evaluate_golden_questions(bad_answer, golden)

        self.assertTrue(good["passes_golden_eval"])
        self.assertFalse(bad["passes_golden_eval"])
        self.assertEqual(bad["questions"][0]["forbidden_phrase_hits"], ["actual ETA forecast"])

    def test_evaluate_golden_questions_reports_required_category_coverage(self) -> None:
        answer = "\n".join(
            [
                "- GitHub context is bounded. [context:github123]",
                "- Ticket-only context preserves source coverage limits. [source_coverage:workstream:support]",
            ]
        )
        golden = {
            "required_categories": ["github-only", "ticket-only", "sparse-coverage"],
            "questions": [
                {
                    "key": "github:scope",
                    "category": "github-only",
                    "expected_facts": [{"text": "GitHub context is bounded", "citation": "[context:github123]"}],
                },
                {
                    "key": "ticket:sparse",
                    "categories": ["ticket-only", "sparse-coverage"],
                    "expected_facts": [
                        {
                            "text": "Ticket-only context preserves source coverage limits",
                            "citation": "[source_coverage:workstream:support]",
                        }
                    ],
                },
            ],
        }

        evaluation = graph_brief.evaluate_golden_questions(answer, golden)

        self.assertTrue(evaluation["passes_golden_eval"])
        self.assertEqual(evaluation["missing_required_categories"], [])
        self.assertEqual(evaluation["category_summary"]["github-only"]["pass_count"], 1)
        self.assertEqual(evaluation["category_summary"]["sparse-coverage"]["pass_count"], 1)
        self.assertEqual(evaluation["category_summary"]["ticket-only"]["pass_count"], 1)

        incomplete = graph_brief.evaluate_golden_questions(
            answer,
            {
                "required_categories": ["github-only", "ticket-only"],
                "questions": [
                    {
                        "key": "github:scope",
                        "category": "github-only",
                        "expected_facts": [{"text": "GitHub context is bounded", "citation": "[context:github123]"}],
                    }
                ],
            },
        )
        self.assertFalse(incomplete["passes_golden_eval"])
        self.assertEqual(incomplete["missing_required_categories"], ["ticket-only"])

    def test_evaluate_golden_questions_supports_no_answer_and_source_coverage_state(self) -> None:
        answer = "\n".join(
            [
                "- Reviewer approval is unknown because sparse coverage blocks that product fact. [source_coverage:workstream:support]",
            ]
        )
        golden = {
            "required_source_coverage_states": ["sparse"],
            "questions": [
                {
                    "key": "coverage:no-answer",
                    "category": "sparse-coverage",
                    "source_coverage_state": "sparse",
                    "expected_facts": [
                        {
                            "text": "Reviewer approved the ticket",
                            "citation": "[work_program_items:work-program-item:support-ticket-101]",
                        }
                    ],
                    "expected_no_answer": [
                        {
                            "text": "Reviewer approval is unknown because sparse coverage blocks that product fact",
                            "citation": "[source_coverage:workstream:support]",
                        }
                    ],
                    "forbidden_phrases": ["Reviewer approved the ticket"],
                }
            ],
        }

        evaluation = graph_brief.evaluate_golden_questions(answer, golden)

        self.assertTrue(evaluation["passes_golden_eval"])
        self.assertEqual(evaluation["missing_required_source_coverage_states"], [])
        self.assertEqual(evaluation["source_coverage_summary"]["sparse"]["pass_count"], 1)
        self.assertTrue(evaluation["questions"][0]["no_answer_allowed"])
        self.assertTrue(evaluation["questions"][0]["no_answer_used"])
        self.assertEqual(evaluation["questions"][0]["missing_expected_facts"], [])

        missing_state = graph_brief.evaluate_golden_questions(
            answer,
            {
                "required_source_coverage_states": ["complete"],
                "questions": [
                    {
                        "key": "coverage:no-answer",
                        "source_coverage_state": "sparse",
                        "expected_no_answer": ["Reviewer approval is unknown"],
                    }
                ],
            },
        )
        self.assertFalse(missing_state["passes_golden_eval"])
        self.assertEqual(missing_state["missing_required_source_coverage_states"], ["complete"])

    def test_evaluate_llm_brief_ignores_locally_negated_forbidden_claims(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                f"- The context is available. [context:{context['context_hash']}]",
                "## Validation Leads",
                "- Review candidates before product use. [analytics:tpm_blocker_candidates]",
                "## What Not To Claim",
                "- The candidates are not confirmed blockers. [analytics:tpm_blocker_candidates]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertTrue(evaluation["passes_smoke_eval"])

    def test_main_writes_golden_eval_gate_into_evaluation_json(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology_db = root / "ontology.db"
            seed_ontology_db(ontology_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=None,
                workstream_key="fixture",
                source_instance="fixture-source",
            )
            answer = "\n".join(
                [
                    "# Operating Brief",
                    "## Confirmed Facts",
                    "- The fixture graph contains cited risk context. "
                    + f"[context:{context['context_hash']}]",
                    "## Validation Leads",
                    "- Review the risky PR before product action. [work_program_items:work-program-item:pr-1]",
                    "## What Not To Claim",
                    f"- Do not claim an ETA date. [guardrail:{context['context_hash']}]",
                ]
            )
            answer_md = root / "answer.md"
            answer_md.write_text(answer, encoding="utf-8")
            golden_json = root / "golden.json"
            golden_json.write_text(
                graph_brief.json.dumps(
                    {
                        "questions": [
                            {
                                "key": "fixture:cli",
                                "question": "What did the graph brief answer?",
                                "expected_facts": [
                                    {
                                        "text": "fixture graph contains cited risk context",
                                        "citation": f"[context:{context['context_hash']}]",
                                    }
                                ],
                                "expected_citations": ["[work_program_items:work-program-item:pr-1]"],
                                "forbidden_phrases": ["will merge by Friday"],
                                "required_sections": ["## What Not To Claim"],
                            }
                        ]
                    }
                ),
                encoding="utf-8",
            )
            evaluation_json = root / "eval.json"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--ontology-db",
                str(ontology_db),
                "--workstream-key",
                "fixture",
                "--source-instance",
                "fixture-source",
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--llm-brief-md",
                str(answer_md),
                "--golden-json",
                str(golden_json),
                "--evaluation-json",
                str(evaluation_json),
            ]
            try:
                graph_brief.main()
            finally:
                sys.argv = old_argv

            evaluation = graph_brief.json.loads(evaluation_json.read_text(encoding="utf-8"))

        self.assertTrue(evaluation["passes_smoke_eval"])
        self.assertTrue(evaluation["passes_eval"])
        self.assertFalse(evaluation["repair_applied"])
        self.assertEqual(evaluation["evaluated_answer_kind"], "raw")
        self.assertTrue(evaluation["golden_eval"]["passes_golden_eval"])
        self.assertEqual(evaluation["golden_eval"]["question_count"], 1)

    def test_main_reports_raw_and_repaired_eval_separately(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology_db = root / "ontology.db"
            seed_ontology_db(ontology_db)
            bad_answer = "\n".join(
                [
                    "# Operating Brief",
                    "## Confirmed Facts",
                    "- The fixture graph contains uncited risk context.",
                    "## Validation Leads",
                    "- Review the risky PR before product action.",
                    "## What Not To Claim",
                    "- Do not claim an ETA date.",
                ]
            )
            answer_md = root / "answer.md"
            answer_md.write_text(bad_answer, encoding="utf-8")
            evaluation_json = root / "eval.json"
            repaired_md = root / "repaired.md"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--ontology-db",
                str(ontology_db),
                "--workstream-key",
                "fixture",
                "--source-instance",
                "fixture-source",
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--llm-brief-md",
                str(answer_md),
                "--repaired-brief-md",
                str(repaired_md),
                "--evaluation-json",
                str(evaluation_json),
            ]
            try:
                graph_brief.main()
            finally:
                sys.argv = old_argv

            evaluation = graph_brief.json.loads(evaluation_json.read_text(encoding="utf-8"))
            repaired = repaired_md.read_text(encoding="utf-8")

        self.assertTrue(evaluation["repair_applied"])
        self.assertTrue(evaluation["repair_changed_answer"])
        self.assertEqual(evaluation["evaluated_answer_kind"], "repaired")
        self.assertFalse(evaluation["raw_answer_eval"]["passes_smoke_eval"])
        self.assertGreater(evaluation["raw_answer_eval"]["uncited_material_claim_line_count"], 0)
        self.assertTrue(evaluation["passes_smoke_eval"])
        self.assertIn("## What Not To Claim", repaired)

    def test_main_refuses_to_persist_repaired_only_passing_brief(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology_db = root / "ontology.db"
            seed_ontology_db(ontology_db)
            graph_context_json = root / "graph_context.json"
            write_fixture_graph_context_json(graph_context_json)
            bad_answer = "\n".join(
                [
                    "# Operating Brief",
                    "## Confirmed Facts",
                    "- The fixture graph contains uncited risk context.",
                    "## Validation Leads",
                    "- Review the risky PR before product action.",
                    "## What Not To Claim",
                    "- Do not claim an ETA date.",
                ]
            )
            answer_md = root / "answer.md"
            answer_md.write_text(bad_answer, encoding="utf-8")
            repaired_md = root / "repaired.md"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--ontology-db",
                str(ontology_db),
                "--graph-context-json",
                str(graph_context_json),
                "--workstream-key",
                "fixture",
                "--source-instance",
                "fixture-source",
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--llm-brief-md",
                str(answer_md),
                "--repaired-brief-md",
                str(repaired_md),
                "--persist-ai-insight",
            ]
            try:
                with self.assertRaisesRegex(SystemExit, "raw brief"):
                    graph_brief.main()
            finally:
                sys.argv = old_argv
            with sqlite3.connect(ontology_db) as conn:
                persisted_count = conn.execute(
                    "select count(*) from work_insights where key like 'work-insight:cubicle-ai:%'"
                ).fetchone()[0]
            repaired_exists = repaired_md.exists()

        self.assertEqual(persisted_count, 0)
        self.assertTrue(repaired_exists)

    def test_main_refuses_to_persist_without_graph_context_json(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            ontology_db = root / "ontology.db"
            seed_ontology_db(ontology_db)
            answer_md = root / "answer.md"
            answer_md.write_text(
                "\n".join(
                    [
                        "# Operating Brief",
                        "## Confirmed Facts",
                        "- The graph context is available. [context:abc123def4567890]",
                        "## Validation Leads",
                        "- Review graph rows before product action. [context:abc123def4567890]",
                        "## What Not To Claim",
                        "- Do not make uncited claims. [guardrail:abc123def4567890]",
                    ]
                ),
                encoding="utf-8",
            )
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--ontology-db",
                str(ontology_db),
                "--workstream-key",
                "fixture",
                "--source-instance",
                "fixture-source",
                "--context-json",
                str(root / "context.json"),
                "--brief-md",
                str(root / "brief.md"),
                "--llm-brief-md",
                str(answer_md),
                "--persist-ai-insight",
            ]
            try:
                with self.assertRaisesRegex(SystemExit, "--graph-context-json"):
                    graph_brief.main()
            finally:
                sys.argv = old_argv
            with sqlite3.connect(ontology_db) as conn:
                persisted_count = conn.execute(
                    "select count(*) from work_insights where key like 'work-insight:cubicle-ai:%'"
                ).fetchone()[0]

        self.assertEqual(persisted_count, 0)

    def test_evaluate_golden_answer_comparison_ranks_answer_strategies(self) -> None:
        golden = {
            "required_categories": ["github-only", "ticket-only"],
            "questions": [
                {
                    "key": "fixture:what-next",
                    "category": "github-only",
                    "expected_facts": [
                        {"text": "risk context", "citation": "[context:abc123]"},
                        "Add labels",
                    ],
                    "expected_citations": ["[analytics:tpm_evaluation_readiness]"],
                    "forbidden_phrases": ["will merge by Friday"],
                },
                {
                    "key": "fixture:ticket",
                    "category": "ticket-only",
                    "expected_facts": [{"text": "ticket context", "citation": "[context:ticket123]"}],
                }
            ]
        }
        answers = {
            "answers": [
                {
                    "key": "typed_rows",
                    "label": "Typed rows",
                    "strategy": "typed_row_summary",
                    "answer_kind": "raw",
                    "text": "- Risk context is present. [context:abc123]\n- Add labels. [analytics:tpm_evaluation_readiness]\n- Ticket context. [context:ticket123]",
                },
                {
                    "key": "packet_context",
                    "label": "Packet context",
                    "text": "- Risk context is present. [context:abc123]\n- This PR will merge by Friday. [context:abc123]",
                },
            ]
        }

        comparison = graph_brief.evaluate_golden_answer_comparison(golden, answers)

        self.assertEqual(comparison["answer_count"], 2)
        self.assertEqual(comparison["answers"][0]["key"], "typed_rows")
        self.assertEqual(comparison["answers"][0]["answer_key"], "typed_rows")
        self.assertEqual(comparison["answers"][0]["strategy"], "typed_row_summary")
        self.assertEqual(comparison["answers"][0]["answer_kind"], "raw")
        self.assertEqual(comparison["answers"][0]["rank"], 1)
        self.assertTrue(comparison["answers"][0]["passes_golden_eval"])
        self.assertEqual(comparison["answers"][0]["category_summary"]["github-only"]["pass_count"], 1)
        self.assertEqual(comparison["answers"][0]["category_summary"]["ticket-only"]["pass_count"], 1)
        self.assertFalse(comparison["answers"][1]["passes_golden_eval"])
        self.assertEqual(comparison["answers"][1]["golden_eval"]["questions"][0]["forbidden_phrase_hits"], ["will merge by Friday"])
        self.assertEqual(comparison["required_categories"], ["github-only", "ticket-only"])
        self.assertEqual(comparison["best_answer_keys_by_category"]["github-only"], ["typed_rows"])
        self.assertEqual(comparison["best_answer_keys_by_category"]["ticket-only"], ["typed_rows"])

    def test_evaluate_golden_answer_comparison_enforces_promotion_gates(self) -> None:
        golden = {
            "required_categories": ["github-only", "ticket-only", "sparse-coverage"],
            "questions": [
                {
                    "key": "github:scope",
                    "category": "github-only",
                    "expected_facts": [{"text": "GitHub risk context", "citation": "[context:github]"}],
                },
                {
                    "key": "ticket:scope",
                    "category": "ticket-only",
                    "expected_facts": [{"text": "Ticket escalation", "citation": "[context:ticket]"}],
                },
                {
                    "key": "coverage:sparse",
                    "category": "sparse-coverage",
                    "expected_facts": [
                        {
                            "text": "Sparse coverage blocks absence claims",
                            "citation": "[source_coverage:workstream:support]",
                        }
                    ],
                },
            ],
        }
        answers = {
            "promotion_gates": [
                {"key": "good-over-baseline", "baseline_key": "typed_rows", "candidate_key": "candidate_good"},
                {"key": "tie-over-baseline", "baseline_key": "typed_rows", "candidate_key": "candidate_tie"},
                {"key": "regression-over-baseline", "baseline_key": "typed_rows", "candidate_key": "candidate_regresses"},
            ],
            "answers": [
                {
                    "key": "typed_rows",
                    "text": "- GitHub risk context. [context:github]\n- Ticket escalation. [context:ticket]",
                },
                {
                    "key": "candidate_good",
                    "text": "\n".join(
                        [
                            "- GitHub risk context. [context:github]",
                            "- Ticket escalation. [context:ticket]",
                            "- Sparse coverage blocks absence claims. [source_coverage:workstream:support]",
                        ]
                    ),
                },
                {
                    "key": "candidate_tie",
                    "text": "- GitHub risk context. [context:github]\n- Ticket escalation. [context:ticket]",
                },
                {
                    "key": "candidate_regresses",
                    "text": "- GitHub risk context. [context:github]\n- Sparse coverage blocks absence claims. [source_coverage:workstream:support]",
                },
            ],
        }

        comparison = graph_brief.evaluate_golden_answer_comparison(golden, answers)
        gates = {gate["key"]: gate for gate in comparison["promotion_gates"]}

        self.assertFalse(comparison["passes_promotion_gates"])
        self.assertTrue(gates["good-over-baseline"]["passes"])
        self.assertEqual(gates["good-over-baseline"]["failure_reasons"], [])
        self.assertFalse(gates["tie-over-baseline"]["passes"])
        self.assertIn("candidate_does_not_beat_baseline_overall", gates["tie-over-baseline"]["failure_reasons"])
        self.assertFalse(gates["regression-over-baseline"]["passes"])
        self.assertIn("candidate_regresses_required_category", gates["regression-over-baseline"]["failure_reasons"])
        ticket_result = [
            row for row in gates["regression-over-baseline"]["category_results"] if row["category"] == "ticket-only"
        ][0]
        self.assertFalse(ticket_result["candidate_no_worse"])

    def test_main_compare_answers_json_writes_ranked_comparison(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            golden_json = root / "golden.json"
            golden_json.write_text(
                graph_brief.json.dumps(
                    {
                        "questions": [
                            {
                                "key": "fixture:compare",
                                "expected_facts": ["risk context"],
                                "expected_citations": ["[context:abc123]"],
                                "forbidden_phrases": ["confirmed blocker"],
                            }
                        ]
                    }
                ),
                encoding="utf-8",
            )
            good_answer = root / "good.md"
            good_answer.write_text("- Risk context. [context:abc123]\n", encoding="utf-8")
            bad_answer = root / "bad.md"
            bad_answer.write_text("- Confirmed blocker. [context:abc123]\n", encoding="utf-8")
            answers_json = root / "answers.json"
            answers_json.write_text(
                graph_brief.json.dumps(
                    {
                        "answers": [
                            {"key": "bad", "path": "bad.md"},
                            {"key": "good", "path": "good.md", "strategy": "packet_context", "answer_kind": "repaired"},
                        ]
                    }
                ),
                encoding="utf-8",
            )
            comparison_json = root / "comparison.json"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--golden-json",
                str(golden_json),
                "--compare-answers-json",
                str(answers_json),
                "--comparison-json",
                str(comparison_json),
            ]
            try:
                graph_brief.main()
            finally:
                sys.argv = old_argv

            comparison = graph_brief.json.loads(comparison_json.read_text(encoding="utf-8"))

        self.assertEqual(comparison["answers"][0]["key"], "good")
        self.assertEqual(comparison["answers"][0]["answer_key"], "good")
        self.assertEqual(comparison["answers"][0]["path"], "good.md")
        self.assertEqual(comparison["answers"][0]["strategy"], "packet_context")
        self.assertEqual(comparison["answers"][0]["answer_kind"], "repaired")
        self.assertEqual(comparison["answers"][0]["rank"], 1)
        self.assertEqual(comparison["best_answer_keys"], ["good"])

    def test_main_compare_answers_json_can_fail_required_promotion_gate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = pathlib.Path(tmp)
            golden_json = root / "golden.json"
            golden_json.write_text(
                graph_brief.json.dumps(
                    {
                        "required_categories": ["ticket-only"],
                        "questions": [
                            {
                                "key": "ticket:scope",
                                "category": "ticket-only",
                                "expected_facts": [{"text": "Ticket context", "citation": "[context:ticket]"}],
                            }
                        ],
                    }
                ),
                encoding="utf-8",
            )
            answers_json = root / "answers.json"
            answers_json.write_text(
                graph_brief.json.dumps(
                    {
                        "promotion_gate": {
                            "baseline_key": "typed_rows",
                            "candidate_key": "packet_context",
                        },
                        "answers": [
                            {"key": "typed_rows", "text": "- Ticket context. [context:ticket]"},
                            {"key": "packet_context", "text": "- Missing ticket detail. [context:ticket]"},
                        ],
                    }
                ),
                encoding="utf-8",
            )
            comparison_json = root / "comparison.json"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--golden-json",
                str(golden_json),
                "--compare-answers-json",
                str(answers_json),
                "--comparison-json",
                str(comparison_json),
                "--require-promotion-gates",
            ]
            try:
                with self.assertRaisesRegex(SystemExit, "promotion gates failed"):
                    graph_brief.main()
            finally:
                sys.argv = old_argv

            comparison = graph_brief.json.loads(comparison_json.read_text(encoding="utf-8"))

        self.assertFalse(comparison["passes_promotion_gates"])
        self.assertEqual(comparison["promotion_gates"][0]["failure_reasons"], [
            "candidate_does_not_pass_golden_eval",
            "candidate_does_not_beat_baseline_overall",
            "candidate_regresses_required_category",
        ])

    def test_ai_first_mixed_minimum_eval_pack_passes_required_promotion_gate(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            comparison_json = pathlib.Path(tmp) / "comparison.json"
            old_argv = sys.argv
            sys.argv = [
                "cubicle_graph_brief.py",
                "--golden-json",
                str(MIXED_EVAL_PACK / "golden_questions.json"),
                "--compare-answers-json",
                str(MIXED_EVAL_PACK / "answers.json"),
                "--comparison-json",
                str(comparison_json),
                "--require-promotion-gates",
            ]
            try:
                graph_brief.main()
            finally:
                sys.argv = old_argv

            comparison = graph_brief.json.loads(comparison_json.read_text(encoding="utf-8"))

        self.assertTrue(comparison["passes_promotion_gates"])
        self.assertEqual(comparison["question_count"], 10)
        self.assertEqual(
            comparison["required_categories"],
            [
                "auth-limited",
                "dependency-topology",
                "evidence-missing",
                "forecast-gated",
                "generated-summary",
                "github-only",
                "mixed-source",
                "run-boundary",
                "sparse-coverage",
                "ticket-only",
            ],
        )
        self.assertEqual(comparison["required_source_coverage_states"], ["auth_limited", "complete", "sparse"])
        self.assertEqual(comparison["promotion_gates"][0]["candidate_key"], "bounded-graph-context-reference")
        self.assertEqual(comparison["promotion_gates"][0]["baseline_key"], "typed-row-baseline")
        candidate = next(row for row in comparison["answers"] if row["key"] == "bounded-graph-context-reference")
        self.assertEqual(candidate["source_coverage_summary"]["auth_limited"]["pass_count"], 1)
        self.assertEqual(candidate["source_coverage_summary"]["sparse"]["pass_count"], 2)
        evidence_question = next(row for row in candidate["golden_eval"]["questions"] if row["key"] == "evidence:missing")
        self.assertTrue(evidence_question["no_answer_used"])
        self.assertEqual(evidence_question["source_coverage_state"], "sparse")

    def test_persist_ai_graph_brief_insight_materializes_generated_insight_and_evidence(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_run_boundary_tables(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )
            answer = "\n".join(
                [
                    "# Operating Brief",
                    "## Confirmed Facts",
                    "- The fixture graph contains cited risk context. "
                    + f"[context:{context['context_hash']}]",
                    "## Validation Leads",
                    "- Add labels before product use. [analytics:tpm_evaluation_readiness]",
                    "## What Not To Claim",
                    f"- Do not claim an ETA date. [guardrail:{context['context_hash']}]",
                ]
            )
            evaluation = graph_brief.evaluate_llm_brief(context, answer)
            self.assertTrue(evaluation["passes_smoke_eval"])

            with sqlite3.connect(ontology_db) as conn:
                conn.row_factory = sqlite3.Row
                conn.execute(
                    """
                    insert into work_insights (
                      id, key, insight_kind, producer_state, subject_kind,
                      subject_key, source_instance, updated_at
                    ) values (
                      900, 'work-insight:old-ai-brief', 'ai_graph_brief', 'current',
                      'unknown', 'workstream:fixture', 'fixture-source', '2026-06-23T16:00:00+00:00'
                    )
                    """
                )
                persisted = graph_brief.persist_ai_graph_brief_insight(
                    conn,
                    context,
                    answer,
                    evaluation,
                    llm_command="/Users/harsh/.venv-vllm-metal/bin/python -m mlx_lm generate --model mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16",
                    llm_model_name="mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16",
                    generated_at="2026-06-23T17:30:00+00:00",
                )
                insight = conn.execute("select * from work_insights where id = ?", [persisted["work_insight_id"]]).fetchone()
                evidence = conn.execute("select * from evidences where id = ?", [persisted["evidence_id"]]).fetchone()
                snapshot = conn.execute(
                    "select * from work_program_brief_snapshots where id = ?",
                    [persisted["work_program_brief_snapshot"]["snapshot_id"]],
                ).fetchone()
                run_member = conn.execute(
                    """
                    select * from work_program_run_members
                     where run_key = ? and member_table = 'work_program_brief_snapshots'
                       and member_id = ?
                    """,
                    [
                        persisted["work_program_brief_snapshot"]["run_member"]["run_key"],
                        persisted["work_program_brief_snapshot"]["snapshot_id"],
                    ],
                ).fetchone()
                old = conn.execute("select producer_state from work_insights where id = 900").fetchone()

        self.assertEqual(insight["insight_kind"], "ai_graph_brief")
        self.assertEqual(insight["subject_kind"], "unknown")
        self.assertEqual(insight["subject_key"], "workstream:fixture")
        self.assertEqual(insight["producer_state"], "current")
        self.assertEqual(insight["score"], 100.0)
        if "model_name" in insight.keys():
            self.assertEqual(insight["model_name"], "mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16")
        self.assertEqual(insight["model_method"], graph_brief.graph_brief_model_method("operating"))
        self.assertIn("|operating|", insight["external_id"])
        self.assertIn("/operating/", insight["source_url"])
        self.assertIn("smoke_eval=true", insight["score_explanation"])
        self.assertIn('"prompt_mode": "operating"', insight["details"])
        self.assertEqual(insight["latest_evidence_id"], persisted["evidence_id"])
        self.assertEqual(evidence["claim_kind"], "generated_summary")
        self.assertEqual(evidence["claim_target_kind"], "work_insight")
        self.assertEqual(evidence["external_kind"], "ai_graph_brief_evidence")
        self.assertIn("|operating|", evidence["external_id"])
        self.assertIn("/operating/", evidence["source_url"])
        self.assertIn("fixture graph contains cited risk context", evidence["excerpt"])
        self.assertEqual(snapshot["external_kind"], "ai_graph_brief_snapshot")
        self.assertIn("|operating|", snapshot["external_id"])
        self.assertIn("/operating/", snapshot["source_url"])
        self.assertEqual(snapshot["latest_evidence_id"], persisted["evidence_id"])
        self.assertEqual(snapshot["generated_at"], "2026-06-23T11:13:38+00:00")
        self.assertIsNotNone(run_member)
        self.assertEqual(run_member["member_key"], snapshot["key"])
        self.assertEqual(old["producer_state"], "superseded")

    def test_persist_ai_graph_brief_scopes_current_rows_by_prompt_mode(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_run_boundary_tables(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )
            answer = "\n".join(
                [
                    "# Operating Brief",
                    "## Confirmed Facts",
                    f"- The bounded graph context is available. [context:{context['context_hash']}]",
                    "## Validation Leads",
                    "- Treat analytics as validation context. [analytics:tpm_evaluation_readiness]",
                    "## What Not To Claim",
                    f"- Do not treat generated summaries as source truth. [guardrail:{context['context_hash']}]",
                ]
            )
            evaluation = graph_brief.evaluate_llm_brief(context, answer)
            self.assertTrue(evaluation["passes_smoke_eval"])

            with sqlite3.connect(ontology_db) as conn:
                conn.row_factory = sqlite3.Row
                conn.executemany(
                    """
                    insert into work_insights (
                      id, key, insight_kind, producer_state, subject_kind,
                      subject_key, source_instance, model_method, external_id,
                      source_url, updated_at
                    ) values (?, ?, 'ai_graph_brief', 'current', 'unknown',
                      'workstream:fixture', 'fixture-source', ?, ?, ?, '2026-06-23T16:00:00+00:00')
                    """,
                    [
                        (
                            900,
                            "work-insight:old-operating",
                            graph_brief.graph_brief_model_method("operating"),
                            "workstream:fixture|operating|old|ai_graph_brief",
                            "cubicle://graph-brief/operating/old",
                        ),
                        (
                            901,
                            "work-insight:old-generic",
                            graph_brief.graph_brief_model_method("generic"),
                            "workstream:fixture|generic|old|ai_graph_brief",
                            "cubicle://graph-brief/generic/old",
                        ),
                    ],
                )
                graph_brief.persist_ai_graph_brief_insight(
                    conn,
                    context,
                    answer,
                    evaluation,
                    llm_command=None,
                    llm_model_name="local-test-model",
                    generated_at="2026-06-23T17:30:00+00:00",
                    prompt_mode="generic",
                )
                rows = {
                    row["key"]: row["producer_state"]
                    for row in conn.execute(
                        "select key, producer_state from work_insights where id in (900, 901)"
                    ).fetchall()
                }

        self.assertEqual(rows["work-insight:old-operating"], "current")
        self.assertEqual(rows["work-insight:old-generic"], "superseded")

    def test_persist_ai_graph_brief_insight_rejects_failed_eval(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )
            answer = "- This uncited answer should not persist."
            evaluation = graph_brief.evaluate_llm_brief(context, answer)
            self.assertFalse(evaluation["passes_smoke_eval"])

            with sqlite3.connect(ontology_db) as conn:
                with self.assertRaises(RuntimeError):
                    graph_brief.persist_ai_graph_brief_insight(
                        conn,
                        context,
                        answer,
                        evaluation,
                        llm_command=None,
                        llm_model_name=None,
                        generated_at="2026-06-23T17:30:00+00:00",
                    )

    def test_run_llm_command_feeds_prompt_to_external_command(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            ontology_db = pathlib.Path(tmp) / "ontology.db"
            analytics_db = pathlib.Path(tmp) / "analytics.db"
            seed_ontology_db(ontology_db)
            seed_analytics_db(analytics_db)
            context = graph_brief.build_context_bundle(
                ontology_db,
                analytics_db=analytics_db,
                workstream_key="fixture",
                source_instance="fixture-source",
            )
            prompt = graph_brief.render_prompt(context)
            script = pathlib.Path(tmp) / "fake_llm.py"
            script.write_text(
                "\n".join(
                    [
                        "import re",
                        "import sys",
                        "prompt = sys.stdin.read()",
                        "match = re.search(r'\\[context:[^\\]]+\\]', prompt)",
                        "assert match, 'missing context citation'",
                        "print('# Operating Brief')",
                        "print('## Confirmed Facts')",
                        "print('- The external command received the graph prompt. ' + match.group(0))",
                        "print('## Validation Leads')",
                        "print('- Review cited graph context before product action. ' + match.group(0))",
                        "print('## What Not To Claim')",
                        "print('- Do not make uncited claims. ' + match.group(0))",
                    ]
                ),
                encoding="utf-8",
            )

            output = graph_brief.run_llm_command(f"{sys.executable} {script}", prompt, 5)

        self.assertIn("external command received the graph prompt", output)
        evaluation = graph_brief.evaluate_llm_brief(context, output)
        self.assertTrue(evaluation["passes_smoke_eval"])

    def test_build_context_bundle_from_graph_context_json_preserves_structured_citation_policy(self) -> None:
        payload = {
            "data": {
                "workProgramGraphContext": {
                    "sourceInstance": "fixture-source",
                    "generatedAt": "2026-06-23T12:00:00Z",
                    "scopeMode": "explicit_source:latest_run:work_program_run_packet_boundary_latest_graph_rows",
                    "runKey": "work-program-run:fixture",
                    "workstreamKey": "workstream:fixture",
                    "contextHash": "abc123def4567890",
                    "traversalDepth": 2,
                    "dependencyEdgeCount": 1,
                    "reachedSubjectKeys": ["repo/example#1", "work-action:review-pr-1"],
                    "allowedCitations": [
                        "[context:abc123def4567890]",
                        "[work_program_items:work-program-item:pr-1]",
                        "[source_coverage:workstream:fixture]",
                    ],
                    "citations": [
                        {
                            "ref": "[context:abc123def4567890]",
                            "citationKind": "graph_context",
                            "nodeKind": "work_program_graph_context",
                            "nodeKey": "workstream:fixture",
                            "proofState": "derived_context",
                            "freshnessState": "current",
                            "visibility": "private",
                            "claimUse": "context_boundary",
                            "claimGateReason": "explicit_source",
                            "claimAllowed": True,
                            "excerptAllowed": False,
                            "sourceUrlAllowed": False,
                        },
                        {
                            "ref": "[work_program_items:work-program-item:pr-1]",
                            "citationKind": "typed_row",
                            "nodeKind": "work_program_item",
                            "nodeKey": "work-program-item:pr-1",
                            "proofState": "no_direct_evidence",
                            "freshnessState": "fresh",
                            "visibility": "unknown",
                            "claimUse": "product_action",
                            "claimGateReason": "product_action_gate_passed",
                            "claimAllowed": True,
                            "excerptAllowed": False,
                            "sourceUrlAllowed": False,
                        },
                        {
                            "ref": "[source_coverage:workstream:fixture]",
                            "citationKind": "derived_packet",
                            "nodeKind": "work_program_source_coverage_packet",
                            "nodeKey": "workstream:fixture",
                            "proofState": "derived_packet",
                            "freshnessState": "current",
                            "visibility": "private",
                            "claimUse": "source_coverage_gate",
                            "claimGateReason": "complete_source_coverage",
                            "claimAllowed": True,
                            "excerptAllowed": False,
                            "sourceUrlAllowed": False,
                        },
                    ],
                    "items": [
                        {
                            "key": "work-program-item:pr-1",
                            "subjectKey": "repo/example#1",
                            "title": "Review risky PR",
                            "programStatus": "needs_review",
                            "decisionState": "product_action",
                            "dueBucket": "now",
                            "riskScore": 95,
                            "nextAction": "Ask owner to validate risk context.",
                            "productActionAllowed": True,
                            "claimUse": "product_action",
                            "claimGateReason": "product_action_gate_passed",
                        }
                    ],
                    "actions": [
                        {
                            "key": "work-action:review-pr-1",
                            "subjectKey": "repo/example#1",
                            "actionState": "open",
                            "decisionState": "product_action",
                            "decisionReason": "Forecast risk needs owner validation.",
                            "ownerKey": "github:owner",
                            "rankScore": 100,
                        }
                    ],
                    "dependencyEdges": [
                        {
                            "key": "work-dependency-edge:pr-action",
                            "edgeKind": "needs_action",
                            "fromKey": "repo/example#1",
                            "toKey": "work-action:review-pr-1",
                            "rankScore": 100,
                            "relationshipClaimAllowed": False,
                            "claimUse": "needs_action_validation",
                            "claimGateReason": "derived_dependency_edge_not_product_claim",
                        }
                    ],
                    "insights": [],
                    "forecasts": [],
                    "qualityGates": [],
                    "evidenceNeeds": [],
                    "forecastPacket": {
                        "etaForecastReady": False,
                        "readinessState": "gated",
                        "automationSummary": "Forecast output is risk triage only.",
                    },
                    "guardrailPacket": {
                        "humanReviewRequired": True,
                        "automationSummary": "Human review required.",
                    },
                    "sourceCoveragePacket": {
                        "coverageState": "complete",
                        "absenceClaimsAllowed": True,
                        "absenceClaimGateReason": "complete_source_coverage",
                        "automationSummary": "Coverage complete.",
                    },
                    "llmTask": "Summarize the typed graph context.",
                }
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            graph_context_json = pathlib.Path(tmp) / "graph_context.json"
            graph_context_json.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_graph_context_json(graph_context_json)

        self.assertEqual(context["seed"]["key"], "workstream:fixture")
        self.assertEqual(context["context_hash"], "abc123def4567890")
        self.assertEqual(context["rows"]["work_program_items"][0]["subject_key"], "repo/example#1")
        self.assertIn("[work_program_items:work-program-item:pr-1]", graph_brief.allowed_citations(context))
        self.assertNotIn("[work_dependency_edges:work-dependency-edge:pr-action]", graph_brief.allowed_citations(context))
        self.assertTrue(graph_brief.product_claims_are_gated(context))
        prompt = graph_brief.render_prompt(context)
        self.assertIn('"citation_policy"', prompt)
        self.assertIn('"claimAllowed": true', prompt)
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The graph context includes the risky PR item. [work_program_items:work-program-item:pr-1]",
                "## Validation Leads",
                "- Human review is required before product action. [context:abc123def4567890]",
                "## What Not To Claim",
                "- Do not make ETA commitments from this context. [source_coverage:workstream:fixture]",
            ]
        )
        evaluation = graph_brief.evaluate_llm_brief(context, answer)
        self.assertTrue(evaluation["passes_smoke_eval"])

    def test_evaluate_llm_brief_rejects_gated_graph_citation_in_confirmed_facts(self) -> None:
        payload = {
            "workProgramGraphContext": {
                "sourceInstance": "fixture-source",
                "workstreamKey": "workstream:fixture",
                "contextHash": "abc123def4567890",
                "allowedCitations": [
                    "[context:abc123def4567890]",
                    "[work_program_items:work-program-item:gated]",
                ],
                "citations": [
                    {
                        "ref": "[work_program_items:work-program-item:gated]",
                        "citationKind": "typed_row",
                        "nodeKind": "work_program_item",
                        "nodeKey": "work-program-item:gated",
                        "claimAllowed": False,
                        "claimUse": "validation_lead",
                        "claimGateReason": "human_review_required",
                    }
                ],
                "items": [
                    {
                        "key": "work-program-item:gated",
                        "subjectKey": "repo/example#2",
                        "title": "Needs validation",
                        "decisionState": "validation_lead",
                    }
                ],
                "actions": [],
                "dependencyEdges": [],
                "insights": [],
                "forecasts": [],
                "qualityGates": [],
                "evidenceNeeds": [],
                "forecastPacket": {"etaForecastReady": False},
                "guardrailPacket": {"humanReviewRequired": True},
                "sourceCoveragePacket": {"absenceClaimsAllowed": False},
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            graph_context_json = pathlib.Path(tmp) / "graph_context.json"
            graph_context_json.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_graph_context_json(graph_context_json)

        bad_answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The gated row is a confirmed product fact. [work_program_items:work-program-item:gated]",
                "## Validation Leads",
                "- Review the gated row before product action. [work_program_items:work-program-item:gated]",
                "## What Not To Claim",
                "- Do not claim sparse sources prove absence. [context:abc123def4567890]",
            ]
        )

        bad_evaluation = graph_brief.evaluate_llm_brief(context, bad_answer)

        self.assertFalse(bad_evaluation["passes_smoke_eval"])
        self.assertEqual(bad_evaluation["citation_policy_violation_count"], 1)
        self.assertEqual(
            bad_evaluation["citation_policy_violations"][0]["kind"],
            "confirmed_fact_requires_claim_allowed_citation",
        )

        repaired = graph_brief.repair_llm_brief(context, bad_answer)
        repaired_evaluation = graph_brief.evaluate_llm_brief(context, repaired)
        self.assertTrue(repaired_evaluation["passes_smoke_eval"])
        self.assertNotIn("confirmed product fact", repaired)

    def test_evaluate_llm_brief_rejects_source_url_leak_without_policy(self) -> None:
        payload = {
            "workProgramGraphContext": {
                "sourceInstance": "fixture-source",
                "workstreamKey": "workstream:fixture",
                "contextHash": "abc123def4567890",
                "allowedCitations": [
                    "[context:abc123def4567890]",
                    "[work_program_items:work-program-item:pr-1]",
                ],
                "citations": [
                    {
                        "ref": "[work_program_items:work-program-item:pr-1]",
                        "citationKind": "typed_row",
                        "nodeKind": "work_program_item",
                        "nodeKey": "work-program-item:pr-1",
                        "claimAllowed": True,
                        "sourceUrlAllowed": False,
                    }
                ],
                "items": [
                    {
                        "key": "work-program-item:pr-1",
                        "subjectKey": "repo/example#1",
                        "title": "Review risky PR",
                        "decisionState": "product_action",
                    }
                ],
                "actions": [],
                "dependencyEdges": [],
                "insights": [],
                "forecasts": [],
                "qualityGates": [],
                "evidenceNeeds": [],
                "forecastPacket": {"etaForecastReady": False},
                "guardrailPacket": {"humanReviewRequired": True},
                "sourceCoveragePacket": {"absenceClaimsAllowed": True},
            }
        }
        with tempfile.TemporaryDirectory() as tmp:
            graph_context_json = pathlib.Path(tmp) / "graph_context.json"
            graph_context_json.write_text(graph_brief.json.dumps(payload), encoding="utf-8")
            context = graph_brief.build_context_bundle_from_graph_context_json(graph_context_json)

        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- The risky PR is present at https://github.example/repo/pull/1 [work_program_items:work-program-item:pr-1]",
                "## Validation Leads",
                "- Validate the owner follow-up before product action. [work_program_items:work-program-item:pr-1]",
                "## What Not To Claim",
                "- Do not expose source URLs without citation policy. [context:abc123def4567890]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["citation_policy_violation_count"], 1)
        self.assertEqual(evaluation["citation_policy_violations"][0]["kind"], "source_url_requires_allowed_citation")

        context["citations"][0]["sourceUrlAllowed"] = True
        allowed_evaluation = graph_brief.evaluate_llm_brief(context, answer)
        self.assertTrue(allowed_evaluation["passes_smoke_eval"])

    def test_evaluate_llm_brief_rejects_nonclaimable_analytics_confirmed_fact(self) -> None:
        context = {
            "context_hash": "abc123def4567890",
            "allowed_citations": [
                "[context:abc123def4567890]",
                "[analytics:tpm_forecast_summary]",
            ],
            "citations": [
                {
                    "ref": "[analytics:tpm_forecast_summary]",
                    "citationKind": "analytics_summary",
                    "claimAllowed": False,
                }
            ],
            "rows": {},
            "guardrails": [],
        }
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- Forecast analytics prove the workstream is ready. [analytics:tpm_forecast_summary]",
                "## Validation Leads",
                "- Review forecast readiness before using dates. [analytics:tpm_forecast_summary]",
                "## What Not To Claim",
                "- Do not treat analytics as product truth. [context:abc123def4567890]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["citation_policy_violation_count"], 1)
        self.assertEqual(
            evaluation["citation_policy_violations"][0]["kind"],
            "confirmed_fact_requires_claim_allowed_citation",
        )

    def test_evaluate_llm_brief_rejects_absence_claim_when_coverage_blocks_absence(self) -> None:
        context = {
            "context_hash": "abc123def4567890",
            "analytics": {
                "source_coverage": {
                    "coverage_state": {"value": "sparse"},
                    "absence_claims_allowed": {"value": "false", "note": "partial_review_history"},
                }
            },
            "allowed_citations": [
                "[context:abc123def4567890]",
                "[source_coverage:workstream:fixture]",
            ],
            "citations": [
                {
                    "ref": "[context:abc123def4567890]",
                    "citationKind": "graph_context",
                    "claimAllowed": True,
                    "claimUse": "context_boundary",
                },
                {
                    "ref": "[source_coverage:workstream:fixture]",
                    "citationKind": "source_coverage",
                    "claimAllowed": True,
                    "claimUse": "source_coverage_gate",
                },
            ],
            "rows": {},
            "guardrails": [],
        }
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- There are no reviews or blockers for this work. [source_coverage:workstream:fixture]",
                "## Validation Leads",
                "- Review source coverage before making absence claims. [source_coverage:workstream:fixture]",
                "## What Not To Claim",
                "- Sparse coverage keeps missing neighbors unknown. [context:abc123def4567890]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["citation_policy_violation_count"], 1)
        self.assertEqual(
            evaluation["citation_policy_violations"][0]["kind"],
            "absence_claim_requires_allowed_source_coverage",
        )

    def test_evaluate_llm_brief_rejects_approval_claim_with_only_boundary_citation(self) -> None:
        context = {
            "context_hash": "abc123def4567890",
            "allowed_citations": [
                "[context:abc123def4567890]",
                "[source_coverage:workstream:fixture]",
            ],
            "citations": [
                {
                    "ref": "[context:abc123def4567890]",
                    "citationKind": "graph_context",
                    "claimAllowed": True,
                    "claimUse": "context_boundary",
                },
                {
                    "ref": "[source_coverage:workstream:fixture]",
                    "citationKind": "source_coverage",
                    "claimAllowed": True,
                    "claimUse": "source_coverage_gate",
                },
            ],
            "rows": {},
            "guardrails": [],
        }
        answer = "\n".join(
            [
                "# Operating Brief",
                "## Confirmed Facts",
                "- Reviewer approved the PR. [context:abc123def4567890]",
                "## Validation Leads",
                "- Check review evidence before product action. [source_coverage:workstream:fixture]",
                "## What Not To Claim",
                "- Do not use coverage metadata as review evidence. [context:abc123def4567890]",
            ]
        )

        evaluation = graph_brief.evaluate_llm_brief(context, answer)

        self.assertFalse(evaluation["passes_smoke_eval"])
        self.assertEqual(evaluation["citation_policy_violation_count"], 1)
        self.assertEqual(
            evaluation["citation_policy_violations"][0]["kind"],
            "confirmed_product_claim_requires_product_citation",
        )

    def test_mlx_lm_command_uses_stdin_and_token_budget(self) -> None:
        command = graph_brief.mlx_lm_command(
            "/Users/harsh/.venv-vllm-metal/bin/python",
            "mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16",
            8192,
        )

        self.assertIn("mlx_lm generate", command)
        self.assertIn("--prompt -", command)
        self.assertIn("--max-tokens 8192", command)
        self.assertIn("--verbose False", command)

    def test_parse_args_defaults_to_larger_local_token_budget(self) -> None:
        old_argv = sys.argv
        sys.argv = [
            "cubicle_graph_brief.py",
            "--workstream-key",
            "fixture",
            "--context-json",
            "context.json",
            "--brief-md",
            "brief.md",
        ]
        try:
            args = graph_brief.parse_args()
        finally:
            sys.argv = old_argv

        self.assertEqual(args.llm_max_tokens, 8192)

    def test_clean_command_output_strips_terminal_control_sequences(self) -> None:
        cleaned = graph_brief.clean_command_output("\x1b[?25l\r\x1b[1G# Brief\x1b[0m\n- Claim. \x1b[32m【context:abc】\x1b[0m\n")

        self.assertEqual(cleaned, "# Brief\n- Claim. [context:abc]")

    def test_row_citation_uses_gate_key_when_row_has_no_key(self) -> None:
        row = {"_table": "work_program_quality_gates", "gate_key": "owner_load"}

        self.assertEqual(
            graph_brief.row_citation(row),
            "[work_program_quality_gates:owner_load]",
        )


def seed_ontology_db(path: pathlib.Path) -> None:
    generated_at = "2026-06-23T12:00:00+00:00"
    with sqlite3.connect(path) as conn:
        conn.executescript(
            """
            create table work_program_items (
              id integer primary key,
              key text,
              workstream_key text,
              subject_kind text,
              subject_key text,
              title text,
              program_status text,
              tpm_bucket text,
              decision_state text,
              due_bucket text,
              owner_key text,
              next_action text,
              risk_score real,
              source_coverage_state text,
              latest_evidence_id integer,
              rank_score real,
              updated_at text,
              work_action_id integer,
              source_instance text
            );
            create table work_actions (
              id integer primary key,
              key text,
              action_type text,
              action_state text,
              decision_state text,
              decision text,
              decision_reason text,
              subject_kind text,
              subject_key text,
              owner_key text,
              due_bucket text,
              latest_evidence_id integer,
              rank_score real,
              source_url text,
              updated_at text,
              source_instance text
            );
            create table work_insights (
              id integer primary key,
              key text unique,
              insight_kind text,
              severity text,
              producer_state text,
              subject_kind text,
              subject_key text,
              title text,
              details text,
              recommended_action text,
              model_name text,
              model_version text,
              model_method text,
              score real,
              score_explanation text,
              latest_evidence_id integer,
              rank_score real,
              source_system text,
              external_kind text,
              external_id text,
              source_url text,
              source_version text,
              updated_at text,
              source_instance text
            );
            create table work_item_forecasts (
              id integer primary key,
              key text,
              forecast_kind text,
              subject_kind text,
              subject_key text,
              subject_state text,
              forecast_method text,
              model_name text,
              age_days real,
              predicted_total_cycle_days real,
              predicted_remaining_days real,
              overdue_days real,
              risk_score real,
              risk_band text,
              readiness_state text,
              ready_for_eta integer,
              readiness_reason text,
              latest_evidence_id integer,
              rank_score real,
              source_url text,
              updated_at text,
              source_instance text
            );
            create table work_dependency_edges (
              id integer primary key,
              key text,
              edge_kind text,
              relationship_authority text,
              canonical_relationship_kind text,
              from_kind text,
              from_key text,
              to_kind text,
              to_key text,
              risk_signal text,
              source_coverage_state text,
              latest_evidence_id integer,
              rank_score real,
              source_url text,
              last_activity_at text,
              updated_at text,
              source_instance text
            );
            create table evidences (
              id integer primary key,
              key text unique,
              claim_kind text,
              claim_target_kind text,
              claim_field text,
              relationship_kind text,
              locator_kind text,
              locator text,
              excerpt text,
              excerpt_truncated integer,
              proof_state text,
              source_system text,
              source_instance text,
              external_kind text,
              external_id text,
              source_url text,
              observed_at text
            );
            create table work_program_quality_gates (
              gate_key text,
              gate_state text,
              blocking integer,
              detail text,
              recommended_action text,
              generated_at text,
              workstream_key text,
              source_instance text,
              rank_score real
            );
            create table work_program_evidence_needs (
              gate_key text,
              evidence_kind text,
              priority text,
              execution_state text,
              target_kind text,
              target_key text,
              action_key text,
              recommended_action text,
              missing_count integer,
              generated_at text,
              workstream_key text,
              source_instance text,
              rank_score real
            );
            """
        )
        conn.executemany(
            "insert into work_program_items values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                (
                    1,
                    "work-program-item:pr-1",
                    "workstream:fixture",
                    "pull_request",
                    "repo/example#1",
                    "Review risky PR",
                    "needs_review",
                    "risk",
                    "product_action",
                    "now",
                    "github:owner",
                    "Ask owner to validate risk context.",
                    95.0,
                    "observed:authenticated_api_current_observation",
                    10,
                    100.0,
                    generated_at,
                    20,
                    "fixture-source",
                ),
                (
                    2,
                    "work-program-item:ticket-1",
                    "workstream:fixture",
                    "ticket",
                    "FLINK-1",
                    "Coordinate linked ticket",
                    "watch",
                    "coordination",
                    "validation_lead",
                    "this_week",
                    "",
                    "Review ticket/PR cluster.",
                    80.0,
                    "generated:dependency_cluster",
                    10,
                    90.0,
                    generated_at,
                    None,
                    "fixture-source",
                ),
            ],
        )
        conn.execute(
            "insert into work_actions values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                20,
                "work-action:review-pr-1",
                "review_insight",
                "open",
                "product_action",
                "",
                "Forecast risk needs owner validation.",
                "pull_request",
                "repo/example#1",
                "github:owner",
                "now",
                10,
                100.0,
                "https://example.test/action",
                generated_at,
                "fixture-source",
            ),
        )
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, producer_state, subject_kind,
              subject_key, title, details, recommended_action, score,
              score_explanation, latest_evidence_id, rank_score, source_url,
              updated_at, source_instance
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                30,
                "work-insight:risk-pr-1",
                "forecast_risk",
                "high",
                "current",
                "pull_request",
                "repo/example#1",
                "PR is above slow-cycle risk threshold",
                "Risk is a triage signal, not an ETA.",
                "Ask owner whether this is blocked.",
                91.0,
                "overdue and high churn",
                10,
                95.0,
                "https://example.test/insight",
                generated_at,
                "fixture-source",
            ),
        )
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, producer_state, subject_kind,
              subject_key, title, details, recommended_action, score,
              score_explanation, latest_evidence_id, rank_score, source_url,
              updated_at, source_instance
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                31,
                "work-insight:ticket-2",
                "dependency_cluster",
                "medium",
                "current",
                "ticket",
                "FLINK-2",
                "Second-hop ticket is in dependency path",
                "This ticket is reached by traversing dependency edges.",
                "Review whether this edge matters.",
                70.0,
                "second-hop dependency",
                10,
                70.0,
                "https://example.test/insight/2",
                generated_at,
                "fixture-source",
            ),
        )
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, producer_state, subject_kind,
              subject_key, title, details, recommended_action, model_method,
              score, score_explanation, latest_evidence_id, rank_score,
              source_system, external_kind, external_id, source_url, updated_at,
              source_instance
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                32,
                "work-insight:generated-summary-launder",
                "ai_graph_brief",
                "info",
                "current",
                "pull_request",
                "repo/example#1",
                "Generated graph brief must not re-enter context",
                "This is a verifier-passed generated summary, not source truth.",
                "Do not use generated summaries as source context.",
                graph_brief.graph_brief_model_method("generic"),
                100.0,
                "smoke_eval=true",
                10,
                1000.0,
                "cubicle_ai",
                "ai_graph_brief",
                "workstream:fixture|generic|ctx|ai_graph_brief",
                "cubicle://graph-brief/generic/ctx",
                generated_at,
                "fixture-source",
            ),
        )
        conn.execute(
            """
            insert into work_insights (
              id, key, insight_kind, severity, producer_state, subject_kind,
              subject_key, title, details, recommended_action, model_method,
              score, score_explanation, latest_evidence_id, rank_score,
              source_system, external_kind, external_id, source_url, updated_at,
              source_instance
            ) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """,
            (
                33,
                "work-insight:misclassified-generated-summary",
                "status_summary",
                "info",
                "current",
                "pull_request",
                "repo/example#1",
                "Misclassified graph brief must not re-enter context",
                "This row has normal TPM source shape but graph-brief producer metadata.",
                "Keep generated summaries quarantined from graph-context reads.",
                graph_brief.graph_brief_model_method("generic"),
                100.0,
                "smoke_eval=true",
                10,
                1001.0,
                "cubicle_analytics",
                "tpm_insight",
                "workstream:fixture|generic|ctx|misclassified",
                "cubicle://graph-brief/generic/ctx",
                generated_at,
                "fixture-source",
            ),
        )
        conn.execute(
            "insert into work_item_forecasts values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                40,
                "work-item-forecast:pr-1",
                "cycle_time",
                "pull_request",
                "repo/example#1",
                "open",
                "heuristic_percentile_ml_rejected",
                "risk_triage",
                12.0,
                None,
                None,
                3.0,
                94.0,
                "high",
                "gated",
                0,
                "ETA blocked by model validation.",
                10,
                94.0,
                "https://example.test/forecast",
                generated_at,
                "fixture-source",
            ),
        )
        conn.execute(
            "insert into work_dependency_edges values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                50,
                "work-dependency-edge:ticket-pr",
                "ticket_pr",
                "canonical_mirror",
                "ticket_pull_request",
                "ticket",
                "FLINK-1",
                "pull_request",
                "repo/example#1",
                "",
                "observed",
                10,
                100.0,
                "https://example.test/edge",
                generated_at,
                generated_at,
                "fixture-source",
            ),
        )
        conn.executemany(
            "insert into work_dependency_edges values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                (
                    51,
                    "work-dependency-edge:ticket-pr-2",
                    "ticket_pr",
                    "canonical_mirror",
                    "ticket_pull_request",
                    "ticket",
                    "FLINK-1",
                    "pull_request",
                    "repo/example#2",
                    "",
                    "observed",
                    10,
                    90.0,
                    "https://example.test/edge/2",
                    generated_at,
                    generated_at,
                    "fixture-source",
                ),
                (
                    52,
                    "work-dependency-edge:second-hop",
                    "related_work",
                    "operating_projection",
                    "",
                    "pull_request",
                    "repo/example#2",
                    "ticket",
                    "FLINK-2",
                    "coordination",
                    "generated",
                    10,
                    80.0,
                    "https://example.test/edge/3",
                    generated_at,
                    generated_at,
                    "fixture-source",
                ),
            ],
        )
        conn.execute(
            "insert into evidences values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                10,
                "evidence:pr-1",
                "object_state",
                "pull_request",
                "state",
                "",
                "source_url",
                "https://example.test/pr/1",
                "PR is open and awaiting review.",
                0,
                "current",
                "github",
                "fixture-source",
                "github_pull_request",
                "repo/example#1",
                "https://example.test/pr/1",
                generated_at,
            ),
        )
        conn.executemany(
            "insert into work_program_quality_gates values (?, ?, ?, ?, ?, ?, ?, ?, ?)",
            [
                (
                    "forecast_readiness",
                    "gated",
                    1,
                    "ETA forecast is gated.",
                    "Use risk triage only.",
                    generated_at,
                    "workstream:fixture",
                    "fixture-source",
                    100.0,
                ),
                (
                    "claim_provenance",
                    "watch",
                    0,
                    "Generated claims need validation.",
                    "Cite source rows.",
                    generated_at,
                    "workstream:fixture",
                    "fixture-source",
                    80.0,
                ),
            ],
        )
        conn.execute(
            "insert into work_program_evidence_needs values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
            (
                "global_insight_precision",
                "insight_labels",
                "high",
                "review_request_open",
                "workstream",
                "workstream:fixture",
                "",
                "Add gold labels before product claims.",
                10,
                generated_at,
                "workstream:fixture",
                "fixture-source",
                100.0,
            ),
        )


def seed_analytics_db(path: pathlib.Path) -> None:
    with sqlite3.connect(path) as conn:
        conn.executescript(
            """
            create table tpm_forecast_summary (
              metric text,
              value text,
              note text
            );
            create table tpm_evaluation_readiness (
              metric text,
              value text,
              note text
            );
            create table tpm_measurement_label_summary (
              metric text,
              value text,
              note text
            );
            create table tpm_forecast_reliability (
              forecast_product text,
              readiness_state text,
              product_safe text,
              safe_use text
            );
            create table tpm_blocker_candidates (
              subject_key text
            );
            """
        )
        conn.executemany(
            "insert into tpm_forecast_summary values (?, ?, ?)",
            [
                ("eta_forecast_ready", "false", "ETA model does not pass readiness."),
                ("risk_triage_lift_at_10pct", "0.34", "Risk ranking is useful for attention ordering."),
            ],
        )
        conn.executemany(
            "insert into tpm_evaluation_readiness values (?, ?, ?)",
            [
                ("ready_to_measure_precision", "false", "Needs gold labels."),
                ("ready_to_measure_actionability", "false", "Needs actionability labels."),
            ],
        )
        conn.execute("insert into tpm_measurement_label_summary values ('measurement_queue_count', '10', 'queue')")
        conn.execute("insert into tpm_forecast_reliability values ('risk_triage', 'ready_with_guardrail', 'true', 'attention_ordering')")
        conn.executemany("insert into tpm_blocker_candidates values (?)", [(f"repo/example#{idx}",) for idx in range(7)])


def write_fixture_graph_context_json(path: pathlib.Path) -> None:
    payload = {
        "data": {
            "workProgramGraphContext": {
                "sourceInstance": "fixture-source",
                "generatedAt": "2026-06-23T12:00:00Z",
                "scopeMode": "explicit_source:latest_run:work_program_run_packet_boundary_latest_graph_rows",
                "runKey": "work-program-run:fixture",
                "workstreamKey": "workstream:fixture",
                "contextHash": "abc123def4567890",
                "traversalDepth": 1,
                "dependencyEdgeCount": 0,
                "reachedSubjectKeys": ["repo/example#1"],
                "allowedCitations": [
                    "[context:abc123def4567890]",
                    "[guardrail:abc123def4567890]",
                    "[work_program_items:work-program-item:pr-1]",
                ],
                "citations": [
                    {
                        "ref": "[context:abc123def4567890]",
                        "citationKind": "graph_context",
                        "nodeKind": "work_program_graph_context",
                        "nodeKey": "workstream:fixture",
                        "proofState": "derived_context",
                        "freshnessState": "current",
                        "visibility": "private",
                        "claimUse": "context_boundary",
                        "claimGateReason": "explicit_source",
                        "claimAllowed": True,
                        "excerptAllowed": False,
                        "sourceUrlAllowed": False,
                    },
                    {
                        "ref": "[work_program_items:work-program-item:pr-1]",
                        "citationKind": "typed_row",
                        "nodeKind": "work_program_item",
                        "nodeKey": "work-program-item:pr-1",
                        "proofState": "typed_row",
                        "freshnessState": "fresh",
                        "visibility": "private",
                        "claimUse": "product_action",
                        "claimGateReason": "product_action_gate_passed",
                        "claimAllowed": True,
                        "excerptAllowed": False,
                        "sourceUrlAllowed": False,
                    },
                ],
                "items": [
                    {
                        "key": "work-program-item:pr-1",
                        "subjectKind": "pull_request",
                        "subjectKey": "repo/example#1",
                        "title": "Review risky PR",
                        "programStatus": "needs_review",
                        "decisionState": "product_action",
                        "nextAction": "Ask owner to validate risk context.",
                        "productActionAllowed": True,
                        "claimUse": "product_action",
                        "claimGateReason": "product_action_gate_passed",
                    }
                ],
                "actions": [],
                "dependencyEdges": [],
                "insights": [],
                "forecasts": [],
                "qualityGates": [],
                "evidenceNeeds": [],
                "forecastPacket": {
                    "etaForecastReady": False,
                    "readinessState": "gated",
                    "automationSummary": "Forecast output is risk triage only.",
                },
                "guardrailPacket": {
                    "humanReviewRequired": True,
                    "automationSummary": "Human review required.",
                },
                "sourceCoveragePacket": {
                    "coverageState": "complete",
                    "absenceClaimsAllowed": True,
                    "absenceClaimGateReason": "complete_source_coverage",
                    "automationSummary": "Coverage complete.",
                },
                "llmTask": "Summarize the typed graph context.",
            }
        }
    }
    path.write_text(graph_brief.json.dumps(payload), encoding="utf-8")


def seed_run_boundary_tables(path: pathlib.Path) -> None:
    with sqlite3.connect(path) as conn:
        conn.executescript(
            """
            create table workstreams (
              id integer primary key autoincrement,
              key text unique,
              title text
            );
            create table work_program_runs (
              id integer primary key autoincrement,
              run_key text unique,
              workstream_key text,
              generated_at text,
              source_instance text,
              brief_snapshot_count integer default 0,
              member_count integer default 0
            );
            create table work_program_run_members (
              id integer primary key autoincrement,
              work_program_run_id integer,
              run_key text,
              member_table text,
              member_id integer,
              member_key text,
              member_external_kind text,
              member_external_id text,
              member_rank_score real,
              created_at text,
              unique(run_key, member_table, member_id)
            );
            create table work_program_brief_snapshots (
              id integer primary key autoincrement,
              key text unique,
              workstream_id integer,
              workstream_key text,
              generated_at text,
              operating_status text,
              decision_pressure text,
              forecast_state text,
              primary_risk text,
              executive_summary text,
              recommended_focus text,
              next_cadence_focus text,
              capability_gaps text,
              total_count integer,
              product_action_count integer,
              validation_lead_count integer,
              source_coverage_limited_count integer,
              active_blocker_count integer,
              active_blocker_impact_count integer,
              needs_action_dependency_count integer,
              overloaded_owner_count integer,
              unassigned_action_count integer,
              quality_gate_count integer,
              blocking_gate_count integer,
              caveat_count integer,
              risk_driver_count integer,
              source_system text,
              source_instance text,
              external_kind text,
              external_id text,
              source_url text,
              latest_evidence_id integer,
              evidence_count integer,
              freshness_state text,
              visibility text,
              confidence real,
              event_count integer,
              first_seen_at text,
              last_activity_at text,
              rank_score real,
              created_at text,
              updated_at text
            );
            insert into workstreams (id, key, title)
            values (1, 'workstream:fixture', 'Fixture Workstream');
            insert into work_program_runs (
              id, run_key, workstream_key, generated_at, source_instance,
              brief_snapshot_count, member_count
            ) values (
              1, 'work-program-run:fixture', 'fixture', '2026-06-23T11:13:38+00:00',
              'fixture-source', 0, 0
            );
            """
        )


if __name__ == "__main__":
    unittest.main()
