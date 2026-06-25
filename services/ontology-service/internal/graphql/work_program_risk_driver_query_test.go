package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
)

func TestLatestWorkProgramRiskDriverModelsLoadsPersistedRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	_, err = store.Client().WorkProgramRiskDriver.Create().
		SetKey("work-program-risk-driver:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetDriverKey("risk-driver:test").
		SetDriverKind("blocker").
		SetSubjectKind("pull_request").
		SetSubjectKey("repo/example#1").
		SetTitle("Persisted blocker").
		SetStatus("active").
		SetRecommendedAction("Use persisted risk drivers.").
		SetEvidenceRef("tpm_risk_driver fixture").
		SetBadgeKeys("risk_driver:kind\nrisk_driver:status").
		SetBadgeLabels("Blocker\nActive").
		SetBadgeTones("danger\nwarning").
		SetBadgeDetails("driver kind\ncurrent status").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_risk_driver").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|risk-driver:test").
		SetRankScore(123).
		Save(ctx)
	if err != nil {
		t.Fatalf("create risk driver: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramRiskDriverModels(ctx, &source, &workstream, 20)
	if err != nil {
		t.Fatalf("latest risk drivers: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("latest risk driver count = %d, want 1", len(rows))
	}
	if rows[0].Key != "risk-driver:test" || rows[0].DriverKind != "blocker" || rows[0].Status != "active" || rows[0].RankScore != 123 {
		t.Fatalf("latest risk driver did not use persisted row: %#v", rows[0])
	}
	if rows[0].SubjectKey == nil || *rows[0].SubjectKey != "repo/example#1" {
		t.Fatalf("latest risk driver did not map subject: %#v", rows[0])
	}
	if len(rows[0].Badges) != 2 || rows[0].Badges[0].Key != "risk_driver:kind" || rows[0].Badges[0].Tone != "danger" {
		t.Fatalf("latest risk driver did not reconstruct badges: %#v", rows[0].Badges)
	}
}

func TestLatestWorkProgramRiskDriverModelsUseRunMembers(t *testing.T) {
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
	included, err := store.Client().WorkProgramRiskDriver.Create().
		SetKey("work-program-risk-driver:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetDriverKey("risk-driver:included").
		SetDriverKind("blocker").
		SetSubjectKind("pull_request").
		SetSubjectKey("repo/example#1").
		SetTitle("Included risk").
		SetStatus("active").
		SetRecommendedAction("Use the durable run member.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_risk_driver").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|risk-driver:included").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included risk driver: %v", err)
	}
	_, err = store.Client().WorkProgramRiskDriver.Create().
		SetKey("work-program-risk-driver:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetDriverKey("risk-driver:noise").
		SetDriverKind("blocker").
		SetSubjectKind("pull_request").
		SetSubjectKey("repo/example#2").
		SetTitle("Noise risk").
		SetStatus("active").
		SetRecommendedAction("Should be excluded by run membership.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_risk_driver").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|risk-driver:noise").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp risk driver: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, workstream, generatedAt, workProgramRunMemberTableRiskDrivers, included.ID, included.Key, included.ExternalKind, included.ExternalID, included.RankScore)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramRiskDriverModels(ctx, &source, &workstreamArg, 20)
	if err != nil {
		t.Fatalf("latest risk drivers: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("latest risk drivers len = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0].Key != "risk-driver:included" {
		t.Fatalf("latest risk driver key = %q, want risk-driver:included", rows[0].Key)
	}
}
