package ent_test

import (
	"context"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/enttest"
	"cubicle/services/ontology-service/ent/migrate"
	ontologyschema "cubicle/services/ontology-service/ent/schema"
	"cubicle/services/ontology-service/ent/workarea"
	"cubicle/services/ontology-service/ent/workdependencyedge"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/worklens"
	"cubicle/services/ontology-service/ent/workprogrammilestone"
	"cubicle/services/ontology-service/ent/workresponsibility"

	coreent "entgo.io/ent"
	entsqlschema "entgo.io/ent/dialect/sql/schema"
	_ "github.com/mattn/go-sqlite3"
)

// TestWorkLensDeclaresCardinalityBoundary documents the bounded lens table.
func TestWorkLensDeclaresCardinalityBoundary(t *testing.T) {
	table := findTable(t, "work_lenses")
	assertColumn(t, table, "work_area_id")
	assertColumn(t, table, "work_lens_kind")
	assertColumn(t, table, "lens_target_kind")
	assertColumn(t, table, "result_count")
	assertColumn(t, table, "last_indexed_at")
}

// TestWorkLensWindowDeclaresTraversalBoundary documents the partition between
// a broad lens and high-volume result rows.
func TestWorkLensWindowDeclaresTraversalBoundary(t *testing.T) {
	table := findTable(t, "work_lens_windows")
	assertColumn(t, table, "work_lens_id")
	assertColumn(t, table, "lens_window_kind")
	assertColumn(t, table, "window_start_at")
	assertColumn(t, table, "window_end_at")
	assertColumn(t, table, "rank_start")
	assertColumn(t, table, "rank_end")
	assertColumn(t, table, "checkpoint")
	assertIndexColumns(t, table, []string{"work_lens_id", "lens_window_kind", "last_activity_at"})
}

// TestDocumentLensResultCarriesPagingIndex proves the result layer can be
// ranked and paged before loading high-cardinality document targets.
func TestDocumentLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "document_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestPullRequestLensResultCarriesPagingIndex proves the result layer can be
// ranked and paged before loading high-cardinality pull-request targets.
func TestPullRequestLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "pull_request_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestPullRequestCarriesSourceLifecycleTimestamps proves analytics can compute
// age and cycle features from typed PR rows instead of raw GitHub payloads.
func TestPullRequestCarriesSourceLifecycleTimestamps(t *testing.T) {
	table := findTable(t, "pull_requests")
	assertColumn(t, table, "source_created_at")
	assertColumn(t, table, "closed_at")
	assertColumn(t, table, "merged_at")
}

// TestPullRequestCarriesSourceMetrics proves PR size/discussion features are
// modeled as typed current source state, not analytics-only replay payloads.
func TestPullRequestCarriesSourceMetrics(t *testing.T) {
	table := findTable(t, "pull_requests")
	assertColumn(t, table, "additions")
	assertColumn(t, table, "deletions")
	assertColumn(t, table, "changed_files_count")
	assertColumn(t, table, "commit_count")
	assertColumn(t, table, "issue_comment_count")
	assertColumn(t, table, "review_comment_count")
	assertColumn(t, table, "is_draft")
	assertColumn(t, table, "is_mergeable")
}

// TestTicketLensResultCarriesPagingIndex proves the result layer can be ranked
// and paged before loading high-cardinality ticket targets.
func TestTicketLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "ticket_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestMessageLensResultCarriesPagingIndex proves the result layer can be
// ranked and paged before loading high-cardinality message targets.
func TestMessageLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "message_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestOpenGraphRowsDeclareGenericConnectorBoundary proves the generic
// connector graph path is Ent-backed without turning source diagnostics or TPM
// packets into traversal adjacency.
func TestOpenGraphRowsDeclareGenericConnectorBoundary(t *testing.T) {
	objects := findTable(t, "open_graph_objects")
	assertColumn(t, objects, "object_type")
	assertColumn(t, objects, "key")
	assertColumn(t, objects, "title")
	assertColumn(t, objects, "latest_evidence_id")
	assertColumn(t, objects, "evidence_count")
	assertColumn(t, objects, "source_system")
	assertColumn(t, objects, "source_instance")
	assertColumn(t, objects, "external_kind")
	assertColumn(t, objects, "external_id")
	assertColumn(t, objects, "source_scope_state_id")
	assertColumn(t, objects, "visibility")
	assertColumn(t, objects, "freshness_state")
	assertUniqueIndexColumns(t, objects, []string{"object_type", "key"})

	associations := findTable(t, "open_graph_associations")
	assertColumn(t, associations, "from_object_id")
	assertColumn(t, associations, "to_object_id")
	assertColumn(t, associations, "association_type")
	assertColumn(t, associations, "latest_evidence_id")
	assertColumn(t, associations, "evidence_count")
	assertColumn(t, associations, "source_system")
	assertColumn(t, associations, "source_instance")
	assertColumn(t, associations, "source_scope_state_id")
	assertColumn(t, associations, "visibility")
	assertColumn(t, associations, "freshness_state")
	assertUniqueIndexColumns(t, associations, []string{"from_object_id", "to_object_id", "association_type"})
	assertIndexColumns(t, associations, []string{"from_object_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, associations, []string{"to_object_id", "freshness_state", "rank_score", "last_activity_at"})
}

// TestWorkInsightCarriesTPMOutput proves generated TPM signals are queryable
// product rows with typed subjects, evidence, model metadata, and ranking.
func TestWorkInsightCarriesTPMOutput(t *testing.T) {
	table := findTable(t, "work_insights")
	assertColumn(t, table, "insight_kind")
	assertColumn(t, table, "severity")
	assertColumn(t, table, "producer_state")
	assertColumn(t, table, "subject_kind")
	assertColumn(t, table, "subject_key")
	assertColumn(t, table, "pull_request_id")
	assertColumn(t, table, "ticket_id")
	assertColumn(t, table, "model_method")
	assertColumn(t, table, "score")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
	assertColumn(t, table, "source_system")
	assertIndexColumns(t, table, []string{"subject_kind", "subject_key", "producer_state"})
	assertIndexColumns(t, table, []string{"producer_state", "severity", "rank_score", "last_activity_at"})
}

// TestWorkInsightReviewCarriesEvaluationLabels proves generated TPM output has
// a durable review/evaluation loop separate from producer-owned insight state.
func TestWorkInsightReviewCarriesEvaluationLabels(t *testing.T) {
	table := findTable(t, "work_insight_reviews")
	assertColumn(t, table, "work_insight_id")
	assertColumn(t, table, "review_kind")
	assertColumn(t, table, "review_state")
	assertColumn(t, table, "truth_label")
	assertColumn(t, table, "actionability_label")
	assertColumn(t, table, "label_set")
	assertColumn(t, table, "label_quality")
	assertColumn(t, table, "reviewer_kind")
	assertColumn(t, table, "reviewer_key")
	assertColumn(t, table, "owner_key")
	assertColumn(t, table, "next_action")
	assertColumn(t, table, "rationale")
	assertColumn(t, table, "reviewed_at")
	assertIndexColumns(t, table, []string{"work_insight_id", "review_kind", "reviewer_kind"})
	assertIndexColumns(t, table, []string{"review_kind", "label_quality", "review_state"})
	assertIndexColumns(t, table, []string{"review_state", "review_kind", "created_at"})
	assertIndexColumns(t, table, []string{"truth_label", "actionability_label", "review_kind"})
}

// TestWorkActionCarriesGatedTPMDecisions proves product reads can start from a
// durable action ledger instead of treating generated insights as decisions.
func TestWorkActionCarriesGatedTPMDecisions(t *testing.T) {
	table := findTable(t, "work_actions")
	assertColumn(t, table, "action_type")
	assertColumn(t, table, "action_state")
	assertColumn(t, table, "decision_state")
	assertColumn(t, table, "decision")
	assertColumn(t, table, "decision_reason")
	assertColumn(t, table, "subject_kind")
	assertColumn(t, table, "subject_key")
	assertColumn(t, table, "pull_request_id")
	assertColumn(t, table, "ticket_id")
	assertColumn(t, table, "owner_key")
	assertColumn(t, table, "owner_source")
	assertColumn(t, table, "due_bucket")
	assertColumn(t, table, "created_from_run_key")
	assertColumn(t, table, "opened_at")
	assertColumn(t, table, "decided_at")
	assertColumn(t, table, "closed_at")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
	assertIndexColumns(t, table, []string{"decision_state", "action_state", "due_bucket", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"subject_kind", "subject_key", "decision_state", "action_state"})
	assertIndexColumns(t, table, []string{"owner_key", "action_state", "due_bucket", "rank_score"})
}

// TestWorkResponsibilityCarriesTypedAccountability proves ownership is graph
// relationship data with evidence and resolution state, not just owner_key text.
func TestWorkResponsibilityCarriesTypedAccountability(t *testing.T) {
	table := findTable(t, "work_responsibilities")
	assertColumn(t, table, "subject_kind")
	assertColumn(t, table, "subject_key")
	assertColumn(t, table, "pull_request_id")
	assertColumn(t, table, "ticket_id")
	assertColumn(t, table, "workstream_id")
	assertColumn(t, table, "work_action_id")
	assertColumn(t, table, "work_blocker_id")
	assertColumn(t, table, "work_program_evidence_need_id")
	assertColumn(t, table, "work_program_item_id")
	assertColumn(t, table, "party_kind")
	assertColumn(t, table, "party_key")
	assertColumn(t, table, "person_id")
	assertColumn(t, table, "responsibility_kind")
	assertColumn(t, table, "basis_kind")
	assertColumn(t, table, "responsibility_state")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
	assertColumnEnumContains(t, table, "responsibility_kind", "validation_owner")
	assertColumnEnumContains(t, table, "basis_kind", "generated_candidate")
	assertColumnEnumContains(t, table, "party_kind", "unassigned")
	assertUniqueIndexColumns(t, table, []string{"subject_kind", "subject_key", "party_key", "responsibility_kind", "basis_kind"})
	assertIndexColumns(t, table, []string{"party_key", "responsibility_kind", "responsibility_state", "rank_score", "last_activity_at"})
	assertSchemaEdges(t, ontologyschema.WorkResponsibility{}.Edges(), []string{
		"person",
		"workstream",
		"pull_request",
		"ticket",
		"work_action",
		"work_blocker",
		"work_program_item",
		"work_program_evidence_need",
		"latest_evidence",
	})
}

// TestAutomationDispositionRemainsDerived documents that per-action automation
// disposition is a read-time projection over durable actions, evidence needs,
// and readiness gates. Persisting it requires explicit append-only snapshot
// semantics, not a silent column on the action ledger.
func TestAutomationDispositionRemainsDerived(t *testing.T) {
	assertNoTable(t, "work_program_automation_action_plans")
	assertNoTable(t, "work_program_automation_plan_snapshots")
	workActions := findTable(t, "work_actions")
	assertNoColumn(t, workActions, "disposition")
	assertNoColumn(t, workActions, "autonomy_level")
	assertNoColumn(t, workActions, "blocking_areas")
	evidenceNeeds := findTable(t, "work_program_evidence_needs")
	assertNoColumn(t, evidenceNeeds, "disposition")
	assertNoColumn(t, evidenceNeeds, "autonomy_level")
}

// TestWorkProgramEvidenceNeedsSupportActionPlanHydration proves the evidence
// queue supports action-plan lookups without turning disposition into storage.
func TestWorkProgramEvidenceNeedsSupportActionPlanHydration(t *testing.T) {
	table := findTable(t, "work_program_evidence_needs")
	assertColumn(t, table, "action_key")
	assertColumn(t, table, "work_action_id")
	assertColumn(t, table, "quality_gate_id")
	assertColumn(t, table, "target_key")
	assertColumn(t, table, "action_state")
	assertIndexColumns(t, table, []string{"workstream_key", "action_key", "generated_at"})
	assertIndexColumns(t, table, []string{"work_action_id", "generated_at"})
	assertIndexColumns(t, table, []string{"quality_gate_id", "generated_at"})
	assertIndexColumns(t, table, []string{"workstream_key", "target_key", "generated_at"})
	assertIndexColumns(t, table, []string{"source_instance", "workstream_key", "action_key", "generated_at"})
	assertIndexColumns(t, table, []string{"source_instance", "workstream_key", "target_key", "generated_at"})
	assertSchemaEdges(t, ontologyschema.WorkProgramEvidenceNeed{}.Edges(), []string{"workstream", "work_action", "quality_gate", "latest_evidence"})
}

// TestWorkProgramTPMFunctionReadinessLinksBlockingGates proves function-level
// automation readiness points at first-class gate rows instead of only storing
// line-delimited gate keys.
func TestWorkProgramTPMFunctionReadinessLinksBlockingGates(t *testing.T) {
	table := findTable(t, "work_program_tpm_function_readinesses")
	assertColumn(t, table, "blocking_gate_keys")
	assertSchemaEdges(t, ontologyschema.WorkProgramTPMFunctionReadiness{}.Edges(), []string{"workstream", "latest_evidence", "blocking_quality_gates"})
	assertSchemaEdges(t, ontologyschema.WorkProgramQualityGate{}.Edges(), []string{"workstream", "latest_evidence", "blocking_tpm_function_readinesses"})

	joinTable := findTable(t, "work_program_tpm_function_readiness_blocking_quality_gates")
	assertColumn(t, joinTable, "work_program_tpm_function_readiness_id")
	assertColumn(t, joinTable, "work_program_quality_gate_id")
	assertPrimaryKeyColumns(t, joinTable, []string{"work_program_tpm_function_readiness_id", "work_program_quality_gate_id"})
}

// TestWorkProgramRunDeclaresDurableRunBoundary proves AI-TPM packets can be
// scoped to one generated run instead of mixing rows across generations.
func TestWorkProgramRunDeclaresDurableRunBoundary(t *testing.T) {
	table := findTable(t, "work_program_runs")
	assertColumn(t, table, "run_key")
	assertColumn(t, table, "workstream_id")
	assertColumn(t, table, "workstream_key")
	assertColumn(t, table, "generated_at")
	assertColumn(t, table, "readiness_state")
	assertColumn(t, table, "readiness_score")
	assertColumn(t, table, "autonomous_action_ready")
	assertColumn(t, table, "human_review_required")
	assertColumn(t, table, "blocking_gate_count")
	assertColumn(t, table, "evidence_need_count")
	assertColumn(t, table, "tpm_function_count")
	assertColumn(t, table, "quality_gate_count")
	assertColumn(t, table, "adversarial_check_count")
	assertColumn(t, table, "owner_load_snapshot_count")
	assertColumn(t, table, "summary_snapshot_count")
	assertColumn(t, table, "brief_snapshot_count")
	assertColumn(t, table, "member_count")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
	assertUniqueIndexColumns(t, table, []string{"source_system", "source_instance", "external_kind", "external_id"})
	assertIndexColumns(t, table, []string{"source_instance", "workstream_key", "generated_at"})
	assertIndexColumns(t, table, []string{"workstream_key", "generated_at", "readiness_state"})
}

// TestWorkProgramRunMemberDeclaresRunBoundaryMembership proves generated
// packet rows have an inspectable membership list tied to a run key.
func TestWorkProgramRunMemberDeclaresRunBoundaryMembership(t *testing.T) {
	table := findTable(t, "work_program_run_members")
	assertColumn(t, table, "work_program_run_id")
	assertColumn(t, table, "run_key")
	assertColumn(t, table, "member_table")
	assertColumn(t, table, "member_id")
	assertColumn(t, table, "member_key")
	assertColumn(t, table, "member_external_kind")
	assertColumn(t, table, "member_external_id")
	assertColumn(t, table, "member_rank_score")
	assertColumn(t, table, "created_at")
	assertUniqueIndexColumns(t, table, []string{"run_key", "member_table", "member_id"})
	assertIndexColumns(t, table, []string{"run_key", "member_table"})
	assertIndexColumns(t, table, []string{"work_program_run_id", "member_table"})
}

// TestWorkProgramMilestoneCarriesSourceDateSignals proves milestone and date
// signals are typed product rows with claim gates, not forecast or due-bucket
// projections pretending to be commitments.
func TestWorkProgramMilestoneCarriesSourceDateSignals(t *testing.T) {
	table := findTable(t, "work_program_milestones")
	assertColumn(t, table, "workstream_id")
	assertColumn(t, table, "pull_request_id")
	assertColumn(t, table, "ticket_id")
	assertColumn(t, table, "workstream_key")
	assertColumn(t, table, "subject_kind")
	assertColumn(t, table, "subject_key")
	assertColumn(t, table, "milestone_kind")
	assertColumn(t, table, "milestone_name")
	assertColumn(t, table, "target_date")
	assertColumn(t, table, "outcome_date")
	assertColumn(t, table, "milestone_state")
	assertColumn(t, table, "commitment_strength")
	assertColumn(t, table, "date_claim_allowed")
	assertColumn(t, table, "delivery_commitment_allowed")
	assertColumn(t, table, "claim_gate_reason")
	assertColumn(t, table, "source_field")
	assertColumn(t, table, "source_payload_key")
	assertColumn(t, table, "captured_at")
	assertColumn(t, table, "generated_at")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
	assertIndexColumns(t, table, []string{"workstream_key", "generated_at", "milestone_kind", "target_date"})
	assertIndexColumns(t, table, []string{"commitment_strength", "date_claim_allowed", "target_date"})
	assertUniqueIndexColumns(t, table, []string{"source_system", "source_instance", "external_kind", "external_id"})
	assertSchemaEdges(t, ontologyschema.WorkProgramMilestone{}.Edges(), []string{"workstream", "pull_request", "ticket", "latest_evidence"})
}

// TestWorkActionObservationCarriesSourceSupport proves action decisions keep
// current source/check evidence separate from the action state itself.
func TestWorkActionObservationCarriesSourceSupport(t *testing.T) {
	table := findTable(t, "work_action_observations")
	assertColumn(t, table, "work_action_id")
	assertColumn(t, table, "observation_kind")
	assertColumn(t, table, "source_coverage_state")
	assertColumn(t, table, "auth_state")
	assertColumn(t, table, "current_state")
	assertColumn(t, table, "ci_signal")
	assertColumn(t, table, "supports_action")
	assertColumn(t, table, "observed_at")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
	assertIndexColumns(t, table, []string{"work_action_id", "observation_kind", "observed_at"})
	assertIndexColumns(t, table, []string{"observation_kind", "supports_action", "observed_at"})
	assertIndexColumns(t, table, []string{"source_coverage_state", "auth_state"})
}

// TestWorkProgramItemCarriesTPMRegisterProjection proves the AI-TPM program
// register is a typed ontology projection, not an analytics-only table.
func TestWorkProgramItemCarriesTPMRegisterProjection(t *testing.T) {
	table := findTable(t, "work_program_items")
	assertColumn(t, table, "workstream_id")
	assertColumn(t, table, "work_action_id")
	assertColumn(t, table, "pull_request_id")
	assertColumn(t, table, "ticket_id")
	assertColumn(t, table, "workstream_key")
	assertColumn(t, table, "subject_kind")
	assertColumn(t, table, "subject_key")
	assertColumn(t, table, "program_status")
	assertColumn(t, table, "tpm_bucket")
	assertColumn(t, table, "owner_key")
	assertColumn(t, table, "owner_source")
	assertColumn(t, table, "next_action")
	assertColumn(t, table, "decision_needed")
	assertColumn(t, table, "decision_state")
	assertColumn(t, table, "due_bucket")
	assertColumn(t, table, "risk_score")
	assertColumn(t, table, "source_coverage_state")
	assertColumn(t, table, "label_quality")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
	assertIndexColumns(t, table, []string{"workstream_key", "program_status", "due_bucket", "risk_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"subject_kind", "subject_key", "program_status"})
	assertIndexColumns(t, table, []string{"owner_key", "program_status", "due_bucket", "risk_score"})
	assertUniqueIndexColumns(t, table, []string{"source_system", "source_instance", "external_kind", "external_id"})
}

// TestTPMOperatingGraphKeepsTypedContextAndRunBoundaries documents the AI-TPM
// graph boundary: durable operating rows keep typed product pointers and proof;
// generated packet rows stay scoped to one run; topology edges remain bounded
// projections over typed relationships instead of replacing them.
func TestTPMOperatingGraphKeepsTypedContextAndRunBoundaries(t *testing.T) {
	for _, tableName := range []string{
		"work_insights",
		"work_actions",
		"work_blockers",
		"work_program_items",
	} {
		table := findTable(t, tableName)
		assertColumn(t, table, "subject_kind")
		assertColumn(t, table, "subject_key")
		assertColumn(t, table, "pull_request_id")
		assertColumn(t, table, "ticket_id")
		assertColumn(t, table, "latest_evidence_id")
		assertColumn(t, table, "evidence_count")
		assertUniqueIndexColumns(t, table, []string{"source_system", "source_instance", "external_kind", "external_id"})
	}

	for _, tableName := range []string{
		"work_program_automation_readinesses",
		"work_program_quality_gates",
		"work_program_evidence_needs",
		"work_program_adversarial_checks",
		"work_program_tpm_function_readinesses",
		"work_program_summary_snapshots",
		"work_program_brief_snapshots",
		"work_program_owner_rollup_snapshots",
		"work_owner_load_snapshots",
	} {
		table := findTable(t, tableName)
		assertColumn(t, table, "workstream_key")
		assertColumn(t, table, "generated_at")
		assertColumn(t, table, "latest_evidence_id")
		assertColumn(t, table, "evidence_count")
		assertUniqueIndexColumns(t, table, []string{"source_system", "source_instance", "external_kind", "external_id"})
	}

	dependencyEdges := findTable(t, "work_dependency_edges")
	assertColumn(t, dependencyEdges, "edge_kind")
	assertColumn(t, dependencyEdges, "relationship_authority")
	assertColumn(t, dependencyEdges, "canonical_relationship_kind")
	assertColumn(t, dependencyEdges, "from_kind")
	assertColumn(t, dependencyEdges, "from_key")
	assertColumn(t, dependencyEdges, "to_kind")
	assertColumn(t, dependencyEdges, "to_key")
	assertColumn(t, dependencyEdges, "workstream_id")
	assertColumn(t, dependencyEdges, "work_blocker_id")
	assertColumn(t, dependencyEdges, "work_action_id")
	assertColumn(t, dependencyEdges, "pull_request_id")
	assertColumn(t, dependencyEdges, "ticket_id")
	assertColumn(t, dependencyEdges, "latest_evidence_id")
	assertColumn(t, dependencyEdges, "evidence_count")
	assertUniqueIndexColumns(t, dependencyEdges, []string{"edge_kind", "from_kind", "from_key", "to_kind", "to_key"})
	assertColumnEnumContains(t, dependencyEdges, "relationship_authority", workdependencyedge.RelationshipAuthorityCanonicalMirror.String())
	assertColumnEnumContains(t, dependencyEdges, "relationship_authority", workdependencyedge.RelationshipAuthorityOperatingProjection.String())
	assertColumnEnumContains(t, dependencyEdges, "canonical_relationship_kind", workdependencyedge.CanonicalRelationshipKindTicketPullRequest.String())
}

// TestWorkForecastEvaluationIncludesLifecycleAsOfBaseline documents the
// baseline-only bridge between lifecycle-derived as-of analytics and typed
// forecast evaluation rows.
func TestWorkForecastEvaluationIncludesLifecycleAsOfBaseline(t *testing.T) {
	assertColumnEnumContains(t, findTable(t, "work_forecast_evaluations"), "evaluation_kind", "lifecycle_as_of_baseline")
	assertColumnEnumContains(t, findTable(t, "work_forecast_evaluations"), "evaluation_kind", "survival_time_to_merge")
}

// TestWorkInsightKindIncludesDeveloperCorrelation keeps generated analytics
// leads aligned with the Ent vocabulary. Developer correlation is workload
// context only; it must remain gated before any product action.
func TestWorkInsightKindIncludesDeveloperCorrelation(t *testing.T) {
	if err := workinsight.InsightKindValidator(workinsight.InsightKind("developer_correlation")); err != nil {
		t.Fatalf("developer_correlation insight kind rejected: %v", err)
	}
	assertColumnEnumContains(t, findTable(t, "work_insights"), "insight_kind", "developer_correlation")
}

// TestPersonServingGraphAvoidsDirectHighCardinalityEdges proves person pages
// must cross WorkArea, WorkLens, and WorkLensWindow before loading work items.
func TestPersonServingGraphAvoidsDirectHighCardinalityEdges(t *testing.T) {
	assertSchemaEdges(t, ontologyschema.Person{}.Edges(), []string{"work_areas", "identities"})
}

// TestWorkLensServingGraphRequiresWindows proves target loading cannot skip
// WorkLensWindow, the serving cardinality boundary.
func TestWorkLensServingGraphRequiresWindows(t *testing.T) {
	assertSchemaEdges(t, ontologyschema.WorkLens{}.Edges(), []string{"area", "windows"})
}

// TestNorthstarGraphSchemaDeclaresTypedGraphAndIsolatedSyncSupport documents the
// selected product graph: source-backed product rows, typed relationship rows,
// locator-grade Evidence, and source-scope sync metadata that is not traversed
// as product graph adjacency.
func TestNorthstarGraphSchemaDeclaresTypedGraphAndIsolatedSyncSupport(t *testing.T) {
	for _, rejected := range []string{
		"source_runs",
		"source_items",
		"source_delta",
		"evidence_anchors",
		"source_references",
		"source_actor_facts",
		"external_identities",
		"document_fragments",
		"ticket_document_fragments",
	} {
		assertNoTable(t, rejected)
	}

	ticket := findTable(t, "tickets")
	assertColumn(t, ticket, "source_system")
	assertColumn(t, ticket, "source_instance")
	assertColumn(t, ticket, "external_kind")
	assertColumn(t, ticket, "external_id")
	assertColumn(t, ticket, "source_version")
	assertColumn(t, ticket, "content_hash")
	assertColumn(t, ticket, "deletion_state")
	assertColumn(t, ticket, "acl_policy_key")
	assertColumn(t, ticket, "visibility_hash")
	assertColumn(t, ticket, "acl_state")
	assertColumn(t, ticket, "source_scope_state_id")
	assertUniqueIndexColumns(t, ticket, []string{"source_system", "source_instance", "external_kind", "external_id"})

	for _, tableName := range []string{"tickets", "pull_requests", "documents", "messages", "workstreams", "work_insights", "work_actions", "work_action_observations", "work_program_items"} {
		table := findTable(t, tableName)
		assertColumn(t, table, "latest_evidence_id")
		assertColumn(t, table, "evidence_count")
	}

	evidences := findTable(t, "evidences")
	assertColumn(t, evidences, "claim_kind")
	assertColumn(t, evidences, "claim_target_kind")
	assertColumn(t, evidences, "claim_target_id")
	assertColumn(t, evidences, "relationship_kind")
	assertColumn(t, evidences, "locator_kind")
	assertColumn(t, evidences, "locator")
	assertColumn(t, evidences, "source_span_key")
	assertColumn(t, evidences, "span_start")
	assertColumn(t, evidences, "span_end")
	assertColumn(t, evidences, "proof_state")
	assertColumn(t, evidences, "acl_policy_key_snapshot")
	assertColumn(t, evidences, "visibility_hash_snapshot")
	assertIndexColumns(t, evidences, []string{"claim_kind", "claim_target_kind", "claim_target_id"})
	assertIndexColumns(t, evidences, []string{"source_system", "source_instance", "external_kind", "external_id", "source_span_key"})

	personIdentities := findTable(t, "person_identities")
	assertColumn(t, personIdentities, "person_id")
	assertColumn(t, personIdentities, "source_system")
	assertColumn(t, personIdentities, "external_kind")
	assertColumn(t, personIdentities, "identity_status")
	assertUniqueIndexColumns(t, personIdentities, []string{"source_system", "source_instance", "external_kind", "external_id"})

	sourceConnections := findTable(t, "source_connections")
	assertUniqueIndexColumns(t, sourceConnections, []string{"source_system", "source_instance"})
	sourceScopes := findTable(t, "source_scopes")
	assertUniqueIndexColumns(t, sourceScopes, []string{"source_connection_id", "scope_kind", "scope_key"})
	sourceScopeStates := findTable(t, "source_scope_states")
	assertColumn(t, sourceScopeStates, "coverage_mode")
	assertColumn(t, sourceScopeStates, "last_successful_sync_run_id")
	assertUniqueIndexColumns(t, sourceScopeStates, []string{"source_scope_id"})
	sourceSyncRuns := findTable(t, "source_sync_runs")
	assertColumn(t, sourceSyncRuns, "source_scope_id")
	assertColumn(t, sourceSyncRuns, "sync_mode")
	assertColumn(t, sourceSyncRuns, "coverage_mode")
	assertColumn(t, sourceSyncRuns, "status")
	assertColumn(t, sourceSyncRuns, "objects_seen_count")
	assertColumn(t, sourceSyncRuns, "objects_created_count")
	assertColumn(t, sourceSyncRuns, "objects_updated_count")
	assertColumn(t, sourceSyncRuns, "objects_deleted_count")
	assertColumn(t, sourceSyncRuns, "relationships_created_count")
	assertColumn(t, sourceSyncRuns, "relationships_updated_count")
	assertColumn(t, sourceSyncRuns, "relationships_deleted_count")
	assertColumn(t, sourceSyncRuns, "evidence_created_count")
	assertColumn(t, sourceSyncRuns, "issues_created_count")
	assertUniqueIndexColumns(t, sourceSyncRuns, []string{"source_scope_id", "run_key"})

	typedRelationships := []struct {
		tableName       string
		firstEndpoint   string
		secondEndpoint  string
		kindColumn      string
		reverseEndpoint string
	}{
		{"ticket_assignments", "person_id", "ticket_id", "assignment_kind", "ticket_id"},
		{"document_authorships", "person_id", "document_id", "authorship_kind", "document_id"},
		{"message_authorships", "person_id", "message_id", "authorship_kind", "message_id"},
		{"pull_request_authorships", "person_id", "pull_request_id", "authorship_kind", "pull_request_id"},
		{"pull_request_reviews", "person_id", "pull_request_id", "review_kind", "pull_request_id"},
		{"message_mentions", "person_id", "message_id", "mention_kind", "message_id"},
		{"ticket_mentions", "person_id", "ticket_id", "mention_kind", "ticket_id"},
		{"ticket_pull_requests", "ticket_id", "pull_request_id", "ticket_pull_request_kind", "pull_request_id"},
		{"ticket_messages", "ticket_id", "message_id", "ticket_message_kind", "message_id"},
		{"ticket_documents", "ticket_id", "document_id", "ticket_document_kind", "document_id"},
		{"document_links", "from_document_id", "to_document_id", "document_link_kind", "to_document_id"},
		{"workstream_tickets", "workstream_id", "ticket_id", "workstream_ticket_kind", "ticket_id"},
	}
	for _, relationship := range typedRelationships {
		table := findTable(t, relationship.tableName)
		assertColumn(t, table, relationship.kindColumn)
		assertColumn(t, table, "latest_evidence_id")
		assertColumn(t, table, "evidence_count")
		assertColumn(t, table, "source_system")
		assertColumn(t, table, "source_scope_state_id")
		assertUniqueIndexColumns(t, table, []string{relationship.firstEndpoint, relationship.secondEndpoint, relationship.kindColumn})
		assertIndexColumns(t, table, []string{relationship.reverseEndpoint, "freshness_state", "rank_score", "last_activity_at"})
	}

	lensResults := []struct {
		tableName      string
		targetEndpoint string
	}{
		{"document_lens_results", "document_id"},
		{"pull_request_lens_results", "pull_request_id"},
		{"ticket_lens_results", "ticket_id"},
		{"message_lens_results", "message_id"},
	}
	for _, result := range lensResults {
		table := findTable(t, result.tableName)
		assertUniqueIndexColumns(t, table, []string{"work_lens_id", result.targetEndpoint, "relation_kind"})
		assertIndexColumns(t, table, []string{result.targetEndpoint, "freshness_state", "rank_score", "last_activity_at"})
	}
}

// TestWorkLensRejectsMismatchedTargetKind proves lens kind is semantic truth.
func TestWorkLensRejectsMismatchedTargetKind(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ontology-lens-validation?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	area := createTestArea(t, ctx, client)
	if _, err := client.WorkLens.Create().
		SetKey("lens:person:docs:bad-target").
		SetWorkAreaID(area.ID).
		SetWorkLensKind(worklens.WorkLensKindDocumentsCommentedOn).
		SetLensTargetKind(worklens.LensTargetKindTicket).
		SetDisplayName("Bad target").
		Save(ctx); err == nil {
		t.Fatal("expected mismatched lens target kind to fail")
	}

	if _, err := client.WorkLens.Create().
		SetKey("lens:person:docs:commented-on").
		SetWorkAreaID(area.ID).
		SetWorkLensKind(worklens.WorkLensKindDocumentsCommentedOn).
		SetLensTargetKind(worklens.LensTargetKindDocument).
		SetDisplayName("Documents Commented On").
		Save(ctx); err != nil {
		t.Fatalf("expected valid lens target kind to save: %v", err)
	}
}

// TestWorkResponsibilityRejectsMismatchedPointers proves subject_kind and
// party_kind carry real graph invariants.
func TestWorkResponsibilityRejectsMismatchedPointers(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ontology-responsibility-validation?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	person := client.Person.Create().
		SetKey("person:owner").
		SetDisplayName("Owner").
		SetGithubLogin("owner").
		SaveX(ctx)
	pr := client.PullRequest.Create().
		SetKey("pr:repo/example:1").
		SetRepository("repo/example").
		SetNumber(1).
		SetTitle("Responsibility PR").
		SaveX(ctx)

	if _, err := client.WorkResponsibility.Create().
		SetKey("responsibility:bad-pointer").
		SetSubjectKind(workresponsibility.SubjectKindPullRequest).
		SetSubjectKey(pr.Key).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey("github:owner").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAuthor).
		SetBasisKind(workresponsibility.BasisKindSourceNative).
		SetResponsibilityState(workresponsibility.ResponsibilityStateActive).
		Save(ctx); err == nil {
		t.Fatal("expected pull_request responsibility without pull_request_id to fail")
	}

	if _, err := client.WorkResponsibility.Create().
		SetKey("responsibility:bad-party").
		SetSubjectKind(workresponsibility.SubjectKindPullRequest).
		SetSubjectKey(pr.Key).
		SetPullRequestID(pr.ID).
		SetPartyKind(workresponsibility.PartyKindPerson).
		SetPartyKey("github:owner").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAuthor).
		SetBasisKind(workresponsibility.BasisKindSourceNative).
		SetResponsibilityState(workresponsibility.ResponsibilityStateActive).
		Save(ctx); err == nil {
		t.Fatal("expected person party without person_id to fail")
	}

	if _, err := client.WorkResponsibility.Create().
		SetKey("responsibility:good").
		SetSubjectKind(workresponsibility.SubjectKindPullRequest).
		SetSubjectKey(pr.Key).
		SetPullRequestID(pr.ID).
		SetPartyKind(workresponsibility.PartyKindPerson).
		SetPartyKey("github:owner").
		SetPersonID(person.ID).
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAuthor).
		SetBasisKind(workresponsibility.BasisKindSourceNative).
		SetResponsibilityState(workresponsibility.ResponsibilityStateActive).
		Save(ctx); err != nil {
		t.Fatalf("expected valid responsibility to save: %v", err)
	}
}

// TestWorkProgramMilestoneRejectsCommitmentSemanticLeaks proves the schema
// rejects weak date signals that try to cross the delivery-commitment gate.
func TestWorkProgramMilestoneRejectsCommitmentSemanticLeaks(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ontology-milestone-validation?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	ticket := client.Ticket.Create().
		SetKey("ticket:FLINK-1").
		SetTitle("Milestone validation ticket").
		SaveX(ctx)
	targetDate := mustParseTime(t, "2026-07-01T00:00:00Z")
	outcomeDate := mustParseTime(t, "2026-06-30T00:00:00Z")

	if _, err := milestoneBase(client, ticket.ID, "release-as-commitment").
		SetMilestoneKind(workprogrammilestone.MilestoneKindReleaseTarget).
		SetMilestoneName("2.4.0").
		SetTargetDate(targetDate).
		SetMilestoneState(workprogrammilestone.MilestoneStatePlanned).
		SetCommitmentStrength(workprogrammilestone.CommitmentStrengthReleaseSignal).
		SetDateClaimAllowed(true).
		SetDeliveryCommitmentAllowed(true).
		Save(ctx); err == nil {
		t.Fatal("expected release target marked as delivery commitment to fail")
	}

	if _, err := milestoneBase(client, ticket.ID, "due-without-target").
		SetMilestoneKind(workprogrammilestone.MilestoneKindExplicitDueDate).
		SetMilestoneName("Jira due date").
		SetMilestoneState(workprogrammilestone.MilestoneStatePlanned).
		SetCommitmentStrength(workprogrammilestone.CommitmentStrengthExplicitCommitment).
		SetDateClaimAllowed(true).
		SetDeliveryCommitmentAllowed(true).
		Save(ctx); err == nil {
		t.Fatal("expected explicit delivery commitment without target date to fail")
	}

	if _, err := milestoneBase(client, ticket.ID, "outcome-as-commitment").
		SetMilestoneKind(workprogrammilestone.MilestoneKindResolutionOutcome).
		SetMilestoneName("Jira resolution").
		SetOutcomeDate(outcomeDate).
		SetMilestoneState(workprogrammilestone.MilestoneStateOutcomeOnly).
		SetCommitmentStrength(workprogrammilestone.CommitmentStrengthExplicitCommitment).
		SetDateClaimAllowed(true).
		SetDeliveryCommitmentAllowed(true).
		Save(ctx); err == nil {
		t.Fatal("expected resolution outcome marked as commitment to fail")
	}

	if _, err := milestoneBase(client, ticket.ID, "valid-release-target").
		SetMilestoneKind(workprogrammilestone.MilestoneKindReleaseTarget).
		SetMilestoneName("2.4.0").
		SetTargetDate(targetDate).
		SetMilestoneState(workprogrammilestone.MilestoneStatePlanned).
		SetCommitmentStrength(workprogrammilestone.CommitmentStrengthReleaseSignal).
		SetDateClaimAllowed(true).
		SetDeliveryCommitmentAllowed(false).
		Save(ctx); err != nil {
		t.Fatalf("expected valid release target to save: %v", err)
	}
}

// createTestArea creates a document WorkArea for schema tests.
func createTestArea(t *testing.T, ctx context.Context, client *ent.Client) *ent.WorkArea {
	t.Helper()
	person := client.Person.Create().
		SetKey("person:test").
		SetDisplayName("Test Person").
		SaveX(ctx)
	return client.WorkArea.Create().
		SetKey("area:person:test:documents").
		SetPersonID(person.ID).
		SetWorkAreaKind(workarea.WorkAreaKindDocuments).
		SetDisplayName("Documents").
		SaveX(ctx)
}

func milestoneBase(client *ent.Client, ticketID int, suffix string) *ent.WorkProgramMilestoneCreate {
	return client.WorkProgramMilestone.Create().
		SetKey("work-program-milestone:" + suffix).
		SetTicketID(ticketID).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetSubjectKind(workprogrammilestone.SubjectKindTicket).
		SetSubjectKey("FLINK-1").
		SetClaimGateReason("schema test").
		SetSourceField("schema.test").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_milestone").
		SetExternalID("flink-kubernetes-operator|2026-07-01T00:00:00Z|" + suffix)
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

// findTable returns the generated migration table with the requested name.
func findTable(t *testing.T, name string) *entsqlschema.Table {
	t.Helper()
	for _, table := range migrate.Tables {
		if table.Name == name {
			return table
		}
	}
	t.Fatalf("missing table %q", name)
	return nil
}

// assertNoTable fails if the generated migration schema still includes name.
func assertNoTable(t *testing.T, name string) {
	t.Helper()
	for _, table := range migrate.Tables {
		if table.Name == name {
			t.Fatalf("table %q should not be part of the northstar schema", name)
		}
	}
}

// assertColumn fails the test if table does not contain name.
func assertColumn(t *testing.T, table *entsqlschema.Table, name string) {
	t.Helper()
	for _, column := range table.Columns {
		if column.Name == name {
			return
		}
	}
	t.Fatalf("table %q missing column %q", table.Name, name)
}

// assertNoColumn fails if table contains name.
func assertNoColumn(t *testing.T, table *entsqlschema.Table, name string) {
	t.Helper()
	for _, column := range table.Columns {
		if column.Name == name {
			t.Fatalf("table %q should not include column %q", table.Name, name)
		}
	}
}

// assertColumnEnumContains fails if an enum column does not list value.
func assertColumnEnumContains(t *testing.T, table *entsqlschema.Table, columnName string, value string) {
	t.Helper()
	for _, column := range table.Columns {
		if column.Name != columnName {
			continue
		}
		for _, enumValue := range column.Enums {
			if enumValue == value {
				return
			}
		}
		t.Fatalf("table %q column %q enum missing %q; values=%#v", table.Name, columnName, value, column.Enums)
	}
	t.Fatalf("table %q missing enum column %q", table.Name, columnName)
}

// assertIndexColumns fails the test if table does not contain a matching index.
func assertIndexColumns(t *testing.T, table *entsqlschema.Table, names []string) {
	t.Helper()
	for _, idx := range table.Indexes {
		if len(idx.Columns) != len(names) {
			continue
		}
		matches := true
		for i, column := range idx.Columns {
			if column.Name != names[i] {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("table %q missing index over columns %#v", table.Name, names)
}

// assertUniqueIndexColumns fails if table does not contain a matching unique index.
func assertUniqueIndexColumns(t *testing.T, table *entsqlschema.Table, names []string) {
	t.Helper()
	for _, idx := range table.Indexes {
		if !idx.Unique || len(idx.Columns) != len(names) {
			continue
		}
		matches := true
		for i, column := range idx.Columns {
			if column.Name != names[i] {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("table %q missing unique index over columns %#v", table.Name, names)
}

// assertPrimaryKeyColumns fails if table does not have the requested primary key.
func assertPrimaryKeyColumns(t *testing.T, table *entsqlschema.Table, names []string) {
	t.Helper()
	if len(table.PrimaryKey) != len(names) {
		t.Fatalf("table %q primary key column count = %d, want %#v", table.Name, len(table.PrimaryKey), names)
	}
	for i, column := range table.PrimaryKey {
		if column.Name != names[i] {
			t.Fatalf("table %q primary key column %d = %q, want %q", table.Name, i, column.Name, names[i])
		}
	}
}

// assertSchemaEdges fails if a handwritten schema exposes unexpected edge names.
func assertSchemaEdges(t *testing.T, edges []coreent.Edge, names []string) {
	t.Helper()
	if len(edges) != len(names) {
		t.Fatalf("schema edge count = %d, want %#v", len(edges), names)
	}
	for i, edge := range edges {
		descriptor := edge.Descriptor()
		if descriptor.Name != names[i] {
			t.Fatalf("schema edge %d = %q, want %q", i, descriptor.Name, names[i])
		}
		if descriptor.Through != nil {
			t.Fatalf("schema edge %q must not be a direct Through edge", descriptor.Name)
		}
	}
}
