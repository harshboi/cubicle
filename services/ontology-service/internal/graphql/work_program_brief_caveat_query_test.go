package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
)

func TestLatestWorkProgramBriefCaveatModelsLoadsPersistedRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	_, err = store.Client().WorkProgramBriefCaveat.Create().
		SetKey("work-program-brief-caveat:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetCaveatKey("forecast_gated").
		SetSeverity("warning").
		SetTitle("Forecast gated").
		SetDetail("Persisted forecast caveat.").
		SetRecommendedAction("Do not present ETA commitments.").
		SetEvidenceRef("forecast_backtest summary").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_brief_caveat").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|forecast_gated").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create caveat: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	rows, err := resolver.latestWorkProgramBriefCaveatModels(ctx, &source, &workstream, 20)
	if err != nil {
		t.Fatalf("latest caveats: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("latest caveat count = %d, want 1", len(rows))
	}
	if rows[0].Key != "forecast_gated" || rows[0].Severity != "warning" || rows[0].Title != "Forecast gated" {
		t.Fatalf("latest caveat did not use persisted row: %#v", rows[0])
	}
	if rows[0].RecommendedAction == nil || *rows[0].RecommendedAction != "Do not present ETA commitments." {
		t.Fatalf("latest caveat did not map recommended action: %#v", rows[0])
	}
	if rows[0].EvidenceRef == nil || *rows[0].EvidenceRef != "forecast_backtest summary" {
		t.Fatalf("latest caveat did not map evidence ref: %#v", rows[0])
	}
}

func TestLatestWorkProgramBriefCaveatGeneratedAtUsesRunMembers(t *testing.T) {
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
	included, err := store.Client().WorkProgramBriefCaveat.Create().
		SetKey("work-program-brief-caveat:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCaveatKey("included_caveat").
		SetSeverity("warning").
		SetTitle("Included caveat").
		SetDetail("Included by durable run membership.").
		SetRecommendedAction("Use the durable run member.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_brief_caveat").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|included_caveat").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included caveat: %v", err)
	}
	_, err = store.Client().WorkProgramBriefCaveat.Create().
		SetKey("work-program-brief-caveat:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCaveatKey("noise_caveat").
		SetSeverity("error").
		SetTitle("Noise caveat").
		SetDetail("Same timestamp, outside the run.").
		SetRecommendedAction("Should be excluded by run membership.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_brief_caveat").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|noise_caveat").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp caveat: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, workstream, generatedAt, workProgramRunMemberTableBriefCaveats, included.ID, included.Key, included.ExternalKind, included.ExternalID, included.RankScore)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	rows, total, err := resolver.latestWorkProgramBriefCaveatModelsAndCountForGeneratedAt(ctx, &source, &workstreamArg, &generatedAt, 20)
	if err != nil {
		t.Fatalf("generated-at caveats: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("generated-at caveats rows/count = %d/%d, want 1/1: %#v", len(rows), total, rows)
	}
	if rows[0].Key != "included_caveat" {
		t.Fatalf("generated-at caveat key = %q, want included_caveat", rows[0].Key)
	}
}
