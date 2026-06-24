package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogramadversarialcheck"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestLatestWorkProgramAdversarialCheckModelsLoadsPersistedRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	_, err = store.Client().WorkProgramAdversarialCheck.Create().
		SetKey("work-program-adversarial-check:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetCheckKind("operating_brief").
		SetCheckState(workprogramadversarialcheck.CheckStatePass).
		SetSeverity(workprogramadversarialcheck.SeverityInfo).
		SetTitle("Persisted check title").
		SetDetail("Persisted detail.").
		SetRecommendedAction("Use persisted rows.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_adversarial_check").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:brief_basis").
		Save(ctx)
	if err != nil {
		t.Fatalf("create adversarial check: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramAdversarialCheckModels(ctx, &source, &workstream, 20)
	if err != nil {
		t.Fatalf("latest checks: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("latest checks count = %d, want 1", len(rows))
	}
	if rows[0].Key != "brief_basis" || rows[0].Title != "Persisted check title" {
		t.Fatalf("latest check did not use persisted display fields: %#v", rows[0])
	}
}

func TestLatestWorkProgramAdversarialCheckGeneratedAtUsesRunMembers(t *testing.T) {
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
	included, err := store.Client().WorkProgramAdversarialCheck.Create().
		SetKey("work-program-adversarial-check:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCheckKind("operating_brief").
		SetCheckState(workprogramadversarialcheck.CheckStateFail).
		SetSeverity(workprogramadversarialcheck.SeverityHigh).
		SetTitle("Included check").
		SetDetail("Included by durable run membership.").
		SetRecommendedAction("Use the durable run member.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_adversarial_check").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:included").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included adversarial check: %v", err)
	}
	_, err = store.Client().WorkProgramAdversarialCheck.Create().
		SetKey("work-program-adversarial-check:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCheckKind("operating_brief").
		SetCheckState(workprogramadversarialcheck.CheckStateFail).
		SetSeverity(workprogramadversarialcheck.SeverityHigh).
		SetTitle("Noise check").
		SetDetail("Same timestamp, outside the run.").
		SetRecommendedAction("Should be excluded by run membership.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_adversarial_check").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:noise").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp adversarial check: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, workstream, generatedAt, workProgramRunMemberTableAdversarialChecks, included.ID, included.Key, included.ExternalKind, included.ExternalID, included.RankScore)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	rows, counts, err := resolver.latestWorkProgramAdversarialCheckModelsAndCountsForGeneratedAt(ctx, &source, &workstreamArg, &generatedAt, 20)
	if err != nil {
		t.Fatalf("generated-at adversarial checks: %v", err)
	}
	if counts.failed != 1 || counts.warning != 0 || len(rows) != 1 {
		t.Fatalf("generated-at adversarial rows/counts = %d/%+v, want 1 failed only: %#v", len(rows), counts, rows)
	}
	if rows[0].Key != "included" {
		t.Fatalf("generated-at adversarial key = %q, want included", rows[0].Key)
	}
}
