package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
)

func TestLatestWorkProgramTPMFunctionReadinessModelsLoadsPersistedRows(t *testing.T) {
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
		SetGateState("gated").
		SetBlocking(true).
		SetDetail("Owner-load gate is blocking this function.").
		SetRecommendedAction("Rebalance owner load before autonomous execution.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_quality_gate").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|owner_load").
		Save(ctx)
	if err != nil {
		t.Fatalf("create quality gate: %v", err)
	}
	_, err = store.Client().WorkProgramTPMFunctionReadiness.Create().
		SetKey("work-program-tpm-function-readiness:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetFunctionKey("execution_capacity").
		SetFunctionName("Execution capacity").
		SetReadinessState("blocked").
		SetAutomationState("rebalance_required").
		SetHumanRequired(true).
		SetSupportingSignalCount(5).
		SetBlockingGateKeys("owner_load").
		SetDetail("Persisted readiness detail.").
		SetRecommendedAction("Use persisted readiness rows.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_tpm_function_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|execution_capacity").
		SetRankScore(95).
		AddBlockingQualityGates(qualityGate).
		Save(ctx)
	if err != nil {
		t.Fatalf("create tpm function readiness: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramTPMFunctionReadinessModels(ctx, &source, &workstream, 20)
	if err != nil {
		t.Fatalf("latest tpm function readiness: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("latest tpm function readiness count = %d, want 1", len(rows))
	}
	if rows[0].FunctionKey != "execution_capacity" || rows[0].ReadinessState != "blocked" || rows[0].AutomationState != "rebalance_required" || !rows[0].HumanRequired {
		t.Fatalf("latest tpm function readiness did not use persisted row: %#v", rows[0])
	}
	if len(rows[0].BlockingGateKeys) != 1 || rows[0].BlockingGateKeys[0] != "owner_load" {
		t.Fatalf("latest tpm function readiness did not parse blocking gates: %#v", rows[0])
	}
	if len(rows[0].BlockingGates) != 1 || rows[0].BlockingGates[0].Key != "owner_load" || !rows[0].BlockingGates[0].Blocking {
		t.Fatalf("latest tpm function readiness did not load typed blocking gates: %#v", rows[0].BlockingGates)
	}
}

func TestLatestWorkProgramTPMFunctionReadinessGeneratedAtUsesRunMembers(t *testing.T) {
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
	included, err := store.Client().WorkProgramTPMFunctionReadiness.Create().
		SetKey("work-program-tpm-function-readiness:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetFunctionKey("included_function").
		SetFunctionName("Included function").
		SetReadinessState("blocked").
		SetAutomationState("review_required").
		SetHumanRequired(true).
		SetSupportingSignalCount(1).
		SetBlockingGateKeys("").
		SetDetail("Included by durable run membership.").
		SetRecommendedAction("Use the durable run member.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_tpm_function_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|included_function").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included tpm function readiness: %v", err)
	}
	_, err = store.Client().WorkProgramTPMFunctionReadiness.Create().
		SetKey("work-program-tpm-function-readiness:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetFunctionKey("noise_function").
		SetFunctionName("Noise function").
		SetReadinessState("blocked").
		SetAutomationState("review_required").
		SetHumanRequired(true).
		SetSupportingSignalCount(99).
		SetBlockingGateKeys("").
		SetDetail("Same timestamp, outside the run.").
		SetRecommendedAction("Should be excluded by run membership.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_tpm_function_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|noise_function").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp tpm function readiness: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, workstream, generatedAt, workProgramRunMemberTableTPMFunctionReadiness, included.ID, included.Key, included.ExternalKind, included.ExternalID, included.RankScore)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramTPMFunctionReadinessRowsForGeneratedAt(ctx, &source, &workstreamArg, &generatedAt)
	if err != nil {
		t.Fatalf("generated-at tpm function readiness: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("generated-at tpm function readiness rows = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0].FunctionKey != "included_function" {
		t.Fatalf("generated-at tpm function readiness key = %q, want included_function", rows[0].FunctionKey)
	}
}
