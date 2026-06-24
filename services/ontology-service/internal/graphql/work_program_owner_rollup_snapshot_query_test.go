package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestLatestWorkProgramOwnerRollupSnapshotModelsHydratesTopItems(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	itemHigh, err := store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:test-high").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#100").
		SetTitle("High-risk product decision").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(99).
		SetOwnerKey("github:owner-a").
		SetOwnerSource("pr_author").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item-high").
		Save(ctx)
	if err != nil {
		t.Fatalf("create high item: %v", err)
	}
	itemLow, err := store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:test-low").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#101").
		SetTitle("Validation follow-up").
		SetProgramStatus(workprogramitem.ProgramStatusValidateSignal).
		SetTpmBucket(workprogramitem.TpmBucketRiskValidation).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketThisWeek).
		SetRiskScore(72).
		SetOwnerKey("github:owner-a").
		SetOwnerSource("pr_author").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item-low").
		Save(ctx)
	if err != nil {
		t.Fatalf("create low item: %v", err)
	}

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	_, err = store.Client().WorkProgramOwnerRollupSnapshot.Create().
		SetKey("work-program-owner-rollup-snapshot:test-owner-a").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetOwnerKey("github:owner-a").
		SetOwnerSource("pr_author").
		SetItemCount(2).
		SetNeedsDecisionCount(1).
		SetValidateSignalCount(1).
		SetNowCount(1).
		SetHighRiskCount(1).
		SetMaxRiskScore(99).
		SetTopItemKeys(itemHigh.Key + "\n" + itemLow.Key).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_owner_rollup_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|owner_rollup|github:owner-a").
		SetRankScore(99).
		Save(ctx)
	if err != nil {
		t.Fatalf("create owner rollup snapshot: %v", err)
	}
	_, err = store.Client().WorkProgramOwnerRollupSnapshot.Create().
		SetKey("work-program-owner-rollup-snapshot:test-owner-b").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetOwnerKey("github:owner-b").
		SetItemCount(1).
		SetWaitingReviewCount(1).
		SetMaxRiskScore(80).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_owner_rollup_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|owner_rollup|github:owner-b").
		SetRankScore(80).
		Save(ctx)
	if err != nil {
		t.Fatalf("create second owner rollup snapshot: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramOwnerRollupSnapshotModels(ctx, &source, &workstream, 100)
	if err != nil {
		t.Fatalf("latest owner rollups: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("owner rollups len = %d, want 2", len(rows))
	}
	if rows[0].OwnerKey != "github:owner-a" || rows[0].ItemCount != 2 || rows[0].NeedsDecisionCount != 1 || rows[0].ValidateSignalCount != 1 {
		t.Fatalf("first owner rollup did not map counts: %#v", rows[0])
	}
	if len(rows[0].TopItems) != 2 || rows[0].TopItems[0].Key != itemHigh.Key || rows[0].TopItems[1].Key != itemLow.Key {
		t.Fatalf("top items were not hydrated in snapshot order: %#v", rows[0].TopItems)
	}
	if len(rows[0].Badges) != 4 {
		t.Fatalf("expected needs-decision, validate-signal, due-now, high-risk badges, got %#v", rows[0].Badges)
	}
}

func TestLatestWorkProgramOwnerRollupSnapshotModelsUseRunMembers(t *testing.T) {
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
	included, err := store.Client().WorkProgramOwnerRollupSnapshot.Create().
		SetKey("work-program-owner-rollup-snapshot:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetOwnerKey("github:included").
		SetItemCount(1).
		SetNeedsDecisionCount(1).
		SetMaxRiskScore(10).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_owner_rollup_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|owner_rollup|github:included").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included owner rollup: %v", err)
	}
	_, err = store.Client().WorkProgramOwnerRollupSnapshot.Create().
		SetKey("work-program-owner-rollup-snapshot:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetOwnerKey("github:noise").
		SetItemCount(99).
		SetNeedsDecisionCount(99).
		SetMaxRiskScore(100).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_owner_rollup_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|owner_rollup|github:noise").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp owner rollup: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, workstream, generatedAt, workProgramRunMemberTableOwnerRollupSnapshots, included.ID, included.Key, included.ExternalKind, included.ExternalID, included.RankScore)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramOwnerRollupSnapshotModels(ctx, &source, &workstreamArg, 100)
	if err != nil {
		t.Fatalf("latest owner rollups: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("owner rollups len = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0].OwnerKey != "github:included" {
		t.Fatalf("owner rollup key = %q, want github:included", rows[0].OwnerKey)
	}
}
