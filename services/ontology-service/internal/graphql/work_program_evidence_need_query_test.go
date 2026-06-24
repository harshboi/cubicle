package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestLatestWorkProgramEvidenceNeedModelsLoadsPersistedRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	qualityGate, err := store.Client().WorkProgramQualityGate.Create().
		SetKey("work-program-quality-gate:owner-load").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetGateKey("owner_load").
		SetGateState("passed").
		SetBlocking(false).
		SetDetail("Owner-load gate is passing in this fixture.").
		SetRecommendedAction("Keep validating owner load rows.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_quality_gate").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|owner_load").
		Save(ctx)
	if err != nil {
		t.Fatalf("create quality gate: %v", err)
	}
	action, err := store.Client().WorkAction.Create().
		SetKey("tpm-action:rebalance").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("flink-kubernetes-operator").
		SetOwnerKey("github:owner-load").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:rebalance").
		SetRankScore(80).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetGateKey("owner_load").
		SetEvidenceKind("owner_load_balancing").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey("flink-kubernetes-operator").
		SetOwnerKey("github:owner-load").
		SetActionKey("tpm-action:rebalance").
		SetActionState("open").
		SetMetricKey("overloaded_owner").
		SetExecutionState("owner_load_rows_open").
		SetBackingActionCount(3).
		SetCurrentCount(2).
		SetRequiredCount(0).
		SetMissingCount(2).
		SetRecommendedAction("Rebalance owner load.").
		SetNextExecutionStep("Use persisted owner-load rows.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|owner_load:rebalance").
		SetSourceURL("https://github.com/apache/flink-kubernetes-operator/pull/1").
		SetQualityGate(qualityGate).
		SetWorkAction(action).
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:test-other-owner").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("required_check_coverage").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey("apache/flink-kubernetes-operator#2").
		SetOwnerKey("github:other-owner").
		SetActionKey("tpm-action:other").
		SetActionState("closed").
		SetExecutionState("stale_source_action_review_needed").
		SetRecommendedAction("Review stale source action.").
		SetNextExecutionStep("Review stale source action.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|source_coverage:other").
		Save(ctx)
	if err != nil {
		t.Fatalf("create other owner evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:test-older").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt.Add(-time.Hour)).
		SetGateKey("owner_load").
		SetEvidenceKind("owner_load_balancing").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey("flink-kubernetes-operator").
		SetOwnerKey("github:owner-load").
		SetActionKey("tpm-action:older").
		SetActionState("open").
		SetExecutionState("owner_load_rows_open").
		SetRecommendedAction("Older row.").
		SetNextExecutionStep("Older row.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T06:03:16Z|owner_load:older").
		Save(ctx)
	if err != nil {
		t.Fatalf("create older evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramEvidenceNeedModels(ctx, &source, &workstream, 20)
	if err != nil {
		t.Fatalf("latest evidence needs: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("latest evidence needs count = %d, want 2", len(rows))
	}
	var ownerLoadRow *model.WorkProgramAutomationEvidenceNeed
	for _, row := range rows {
		if row.Key == "owner_load:rebalance" {
			ownerLoadRow = row
		}
	}
	if ownerLoadRow == nil || ownerLoadRow.ExecutionState != "owner_load_rows_open" || ownerLoadRow.BackingActionCount != 3 {
		t.Fatalf("latest evidence need did not use persisted row: %#v", ownerLoadRow)
	}
	if ownerLoadRow.OwnerKey == nil || *ownerLoadRow.OwnerKey != "github:owner-load" {
		t.Fatalf("latest evidence need owner key = %#v, want github:owner-load", ownerLoadRow.OwnerKey)
	}
	if ownerLoadRow.ActionKey == nil || *ownerLoadRow.ActionKey != "tpm-action:rebalance" {
		t.Fatalf("latest evidence need action key = %#v, want tpm-action:rebalance", ownerLoadRow.ActionKey)
	}
	if ownerLoadRow.ActionState == nil || *ownerLoadRow.ActionState != "open" {
		t.Fatalf("latest evidence need action state = %#v, want open", ownerLoadRow.ActionState)
	}
	if ownerLoadRow.QualityGate == nil || ownerLoadRow.QualityGate.Key != "owner_load" || ownerLoadRow.QualityGate.GateState != "passed" {
		t.Fatalf("latest evidence need quality gate = %#v, want owner_load passed", ownerLoadRow.QualityGate)
	}
	if ownerLoadRow.Action == nil || ownerLoadRow.Action.Key != "tpm-action:rebalance" || ownerLoadRow.Action.DecisionState != "validation_lead" {
		t.Fatalf("latest evidence need action = %#v, want linked validation action", ownerLoadRow.Action)
	}
	if ownerLoadRow.SourceURL == nil || *ownerLoadRow.SourceURL != "https://github.com/apache/flink-kubernetes-operator/pull/1" {
		t.Fatalf("latest evidence need source URL = %#v, want source URL", ownerLoadRow.SourceURL)
	}
	cappedRows, totalCount, err := resolver.latestWorkProgramEvidenceNeedModelsAndCountForFilters(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  &source,
		workstreamKey: &workstream,
	}, 1)
	if err != nil {
		t.Fatalf("latest evidence needs with count: %v", err)
	}
	if len(cappedRows) != 1 || totalCount != 2 {
		t.Fatalf("latest capped evidence rows/count = %d/%d, want 1/2", len(cappedRows), totalCount)
	}

	gate := "owner_load"
	owner := "github:owner-load"
	actionState := "open"
	filtered, err := resolver.latestWorkProgramEvidenceNeedModelsForFilters(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  &source,
		workstreamKey: &workstream,
		gateKey:       &gate,
		ownerKey:      &owner,
		actionState:   &actionState,
	}, 20)
	if err != nil {
		t.Fatalf("filtered evidence needs: %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("filtered evidence needs count = %d, want 1: %#v", len(filtered), filtered)
	}
	if filtered[0].ActionKey == nil || *filtered[0].ActionKey != "tpm-action:rebalance" {
		t.Fatalf("filtered evidence need action key = %#v, want latest open action", filtered[0].ActionKey)
	}
}

func TestWorkProgramEvidenceNeedRunPrefixOnlyUsesRunShapedExternalIDs(t *testing.T) {
	if got := workProgramEvidenceNeedRunPrefix("flink-kubernetes-operator|2026-06-21T07:03:16Z|owner_load:rebalance"); got != "flink-kubernetes-operator|2026-06-21T07:03:16Z|" {
		t.Fatalf("pipe run prefix = %q, want recognized run prefix", got)
	}
	if got := workProgramEvidenceNeedRunPrefix("source_coverage:owner-a"); got != "" {
		t.Fatalf("colon-only run prefix = %q, want empty prefix to avoid stale prefix matching", got)
	}
}

func TestLatestWorkProgramEvidenceNeedGeneratedAtUsesRunMembers(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	included, err := store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("owner_load").
		SetEvidenceKind("owner_load_balancing").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey(workstream).
		SetOwnerKey("github:owner-load").
		SetActionKey("tpm-action:included").
		SetActionState("open").
		SetExecutionState("owner_load_rows_open").
		SetRecommendedAction("Use the durable run member.").
		SetNextExecutionStep("Use the durable run member.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|included").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("required_check_coverage").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey("apache/flink-kubernetes-operator#2").
		SetOwnerKey("github:other-owner").
		SetActionKey("tpm-action:noise").
		SetActionState("open").
		SetExecutionState("stale_source_action_review_needed").
		SetRecommendedAction("This row has the same timestamp but is outside the run.").
		SetNextExecutionStep("This row has the same timestamp but is outside the run.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|noise").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp evidence need: %v", err)
	}
	run, err := store.Client().WorkProgramRun.Create().
		SetKey("work-program-run:fixture").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("blocked").
		SetReadinessScore(25).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetEvidenceNeedCount(1).
		SetMemberCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|run").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create work program run: %v", err)
	}
	_, err = store.Client().WorkProgramRunMember.Create().
		SetWorkProgramRunID(run.ID).
		SetRunKey(run.Key).
		SetMemberTable(workProgramRunMemberTableEvidenceNeeds).
		SetMemberID(included.ID).
		SetMemberKey(included.Key).
		SetMemberExternalKind(included.ExternalKind).
		SetMemberExternalID(included.ExternalID).
		SetMemberRankScore(included.RankScore).
		SetCreatedAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create work program run member: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	rows, total, err := resolver.latestWorkProgramEvidenceNeedModelsAndCountForFilters(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  &source,
		workstreamKey: &workstreamArg,
		generatedAt:   &generatedAt,
	}, 20)
	if err != nil {
		t.Fatalf("generated-at evidence needs: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("generated-at evidence rows/count = %d/%d, want 1/1: %#v", len(rows), total, rows)
	}
	if rows[0].Key != "included" {
		t.Fatalf("generated-at evidence key = %q, want included", rows[0].Key)
	}
}
