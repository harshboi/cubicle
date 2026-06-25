package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestLatestWorkProgramAutomationReadinessModelLoadsPersistedRow(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	_, err = store.Client().WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetReadinessState("blocked").
		SetReadinessScore(42).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetSafeAutomationAreas("agenda_summarization\nsource_citation").
		SetHumanRequiredAreas("eta_commitments\nmeasurement_claims").
		SetRationale("Persisted readiness rationale.").
		SetRequiredEvidence("forecast backtest outcomes\ngold labels").
		SetBlockingGateKeys("forecast_readiness\nmeasurement_precision").
		SetQualityGateCount(6).
		SetBlockingGateCount(2).
		SetEvidenceNeedCount(3).
		SetTpmFunctionCount(7).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create automation readiness: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	gates := []*model.WorkProgramBriefQualityGate{{Key: "forecast_readiness", GateState: "gated", Blocking: true}}
	evidenceNeeds := []*model.WorkProgramAutomationEvidenceNeed{{Key: "forecast_readiness:backtest", GateKey: "forecast_readiness"}}
	row, err := resolver.latestWorkProgramAutomationReadinessModel(ctx, &source, &workstream, gates, evidenceNeeds)
	if err != nil {
		t.Fatalf("latest automation readiness: %v", err)
	}
	if row == nil {
		t.Fatalf("latest automation readiness = nil, want row")
	}
	if row.ReadinessState != "blocked" || row.ReadinessScore != 42 || row.AutonomousActionReady || !row.HumanReviewRequired {
		t.Fatalf("latest automation readiness did not use persisted verdict: %#v", row)
	}
	if len(row.SafeAutomationAreas) != 2 || row.SafeAutomationAreas[0] != "agenda_summarization" {
		t.Fatalf("latest automation readiness did not parse safe areas: %#v", row.SafeAutomationAreas)
	}
	if len(row.Gates) != 1 || row.Gates[0].Key != "forecast_readiness" {
		t.Fatalf("latest automation readiness did not attach gates: %#v", row.Gates)
	}
	if len(row.EvidenceWorkQueue) != 1 || row.EvidenceWorkQueue[0].Key != "forecast_readiness:backtest" {
		t.Fatalf("latest automation readiness did not attach evidence queue: %#v", row.EvidenceWorkQueue)
	}
}

func TestLatestWorkProgramRunGeneratedAtPrefersDurableRunBoundary(t *testing.T) {
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
	oldGeneratedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	runGeneratedAt := oldGeneratedAt.Add(time.Hour)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, oldGeneratedAt)
	_, err = store.Client().WorkProgramRun.Create().
		SetKey("work-program-run:test").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(runGeneratedAt).
		SetReadinessState("blocked").
		SetReadinessScore(25).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetMemberCount(3).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:03:16Z|run").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create work program run: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	generatedAt, err := resolver.latestWorkProgramAutomationReadinessRunGeneratedAt(ctx, &source, &workstreamArg)
	if err != nil {
		t.Fatalf("latest run generatedAt: %v", err)
	}
	if generatedAt == nil || !generatedAt.Equal(runGeneratedAt) {
		t.Fatalf("latest run generatedAt = %#v, want durable WorkProgramRun timestamp %s", generatedAt, runGeneratedAt.Format(time.RFC3339))
	}
}

func TestLatestWorkProgramRunGeneratedAtFallsBackToAutomationReadiness(t *testing.T) {
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
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, generatedAt)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	actual, err := resolver.latestWorkProgramAutomationReadinessRunGeneratedAt(ctx, &source, &workstreamArg)
	if err != nil {
		t.Fatalf("latest fallback generatedAt: %v", err)
	}
	if actual == nil || !actual.Equal(generatedAt) {
		t.Fatalf("latest fallback generatedAt = %#v, want automation readiness timestamp %s", actual, generatedAt.Format(time.RFC3339))
	}
}
