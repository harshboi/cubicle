package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
)

func TestLatestWorkProgramQualityGateModelsLoadsPersistedRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	_, err = store.Client().WorkProgramQualityGate.Create().
		SetKey("work-program-quality-gate:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetGateState("gated").
		SetBlocking(true).
		SetDetail("Persisted source coverage gate.").
		SetRecommendedAction("Use persisted quality gates.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_quality_gate").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|source_coverage").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create quality gate: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramQualityGateModels(ctx, &source, &workstream, 20)
	if err != nil {
		t.Fatalf("latest quality gates: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("latest quality gates count = %d, want 1", len(rows))
	}
	if rows[0].Key != "source_coverage" || rows[0].GateState != "gated" || !rows[0].Blocking {
		t.Fatalf("latest quality gate did not use persisted row: %#v", rows[0])
	}
	if rows[0].RecommendedAction == nil || *rows[0].RecommendedAction != "Use persisted quality gates." {
		t.Fatalf("latest quality gate did not map recommended action: %#v", rows[0])
	}
}

func TestLatestWorkProgramQualityGateGeneratedAtUsesRunMembers(t *testing.T) {
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
	included, err := store.Client().WorkProgramQualityGate.Create().
		SetKey("work-program-quality-gate:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("included_gate").
		SetGateState("gated").
		SetBlocking(true).
		SetDetail("Included gate.").
		SetRecommendedAction("Use the durable run member.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_quality_gate").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|included_gate").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included quality gate: %v", err)
	}
	_, err = store.Client().WorkProgramQualityGate.Create().
		SetKey("work-program-quality-gate:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("noise_gate").
		SetGateState("gated").
		SetBlocking(true).
		SetDetail("Same timestamp, outside the run.").
		SetRecommendedAction("Should be excluded by run membership.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_quality_gate").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|noise_gate").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp quality gate: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, workstream, generatedAt, workProgramRunMemberTableQualityGates, included.ID, included.Key, included.ExternalKind, included.ExternalID, included.RankScore)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	rows, blockingCount, err := resolver.latestWorkProgramQualityGateModelsAndBlockingCountForGeneratedAt(ctx, &source, &workstreamArg, &generatedAt, 20)
	if err != nil {
		t.Fatalf("generated-at quality gates: %v", err)
	}
	if blockingCount != 1 || len(rows) != 1 {
		t.Fatalf("generated-at quality gate rows/blocking = %d/%d, want 1/1: %#v", len(rows), blockingCount, rows)
	}
	if rows[0].Key != "included_gate" {
		t.Fatalf("generated-at quality gate key = %q, want included_gate", rows[0].Key)
	}
}
