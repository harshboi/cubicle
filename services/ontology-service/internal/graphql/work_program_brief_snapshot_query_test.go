package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
)

func TestLatestWorkProgramBriefSnapshotDataLoadsPersistedRow(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	_, err = store.Client().WorkProgramBriefSnapshot.Create().
		SetKey("work-program-brief-snapshot:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetOperatingStatus("blocked").
		SetDecisionPressure("product_decisions").
		SetForecastState("gated").
		SetPrimaryRisk("blockers").
		SetExecutiveSummary("Persisted executive summary.").
		SetRecommendedFocus("Persisted recommended focus.").
		SetNextCadenceFocus("Persisted cadence focus.").
		SetCapabilityGaps("forecast_readiness\nsource_coverage").
		SetTotalCount(12).
		SetProductActionCount(3).
		SetValidationLeadCount(4).
		SetSourceCoverageLimitedCount(2).
		SetActiveBlockerCount(1).
		SetActiveBlockerImpactCount(1).
		SetNeedsActionDependencyCount(5).
		SetOverloadedOwnerCount(1).
		SetUnassignedActionCount(2).
		SetQualityGateCount(6).
		SetBlockingGateCount(5).
		SetCaveatCount(4).
		SetRiskDriverCount(10).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_brief_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|brief_snapshot").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create brief snapshot: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	row, err := resolver.latestWorkProgramBriefSnapshotData(ctx, &source, &workstream)
	if err != nil {
		t.Fatalf("latest brief snapshot: %v", err)
	}
	if row == nil {
		t.Fatalf("latest brief snapshot = nil, want row")
	}
	if row.OperatingStatus != "blocked" || row.DecisionPressure != "product_decisions" || row.ForecastState != "gated" {
		t.Fatalf("latest brief snapshot did not map status fields: %#v", row)
	}
	if row.ExecutiveSummary != "Persisted executive summary." || row.RecommendedFocus != "Persisted recommended focus." || row.NextCadenceFocus != "Persisted cadence focus." {
		t.Fatalf("latest brief snapshot did not map narrative: %#v", row)
	}
	if len(row.CapabilityGaps) != 2 || row.CapabilityGaps[0] != "forecast_readiness" || row.CapabilityGaps[1] != "source_coverage" {
		t.Fatalf("latest brief snapshot did not map capability gaps: %#v", row.CapabilityGaps)
	}
	if row.PrimaryRisk == nil || *row.PrimaryRisk != "blockers" {
		t.Fatalf("latest brief snapshot did not map primary risk: %#v", row.PrimaryRisk)
	}
	if row.GeneratedAt == nil || *row.GeneratedAt != "2026-06-21T07:03:16Z" {
		t.Fatalf("latest brief snapshot generatedAt = %#v", row.GeneratedAt)
	}
}

func TestLatestWorkProgramBriefSnapshotDataUsesRunMembers(t *testing.T) {
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
	included, err := store.Client().WorkProgramBriefSnapshot.Create().
		SetKey("work-program-brief-snapshot:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetOperatingStatus("included").
		SetDecisionPressure("included").
		SetForecastState("gated").
		SetExecutiveSummary("Included brief.").
		SetRecommendedFocus("Included focus.").
		SetNextCadenceFocus("Included cadence.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_brief_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|brief_snapshot").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included brief snapshot: %v", err)
	}
	_, err = store.Client().WorkProgramBriefSnapshot.Create().
		SetKey("work-program-brief-snapshot:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetOperatingStatus("noise").
		SetDecisionPressure("noise").
		SetForecastState("gated").
		SetExecutiveSummary("Noise brief.").
		SetRecommendedFocus("Should be excluded by run membership.").
		SetNextCadenceFocus("Should be excluded by run membership.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_brief_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|brief_snapshot_noise").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp brief snapshot: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, workstream, generatedAt, workProgramRunMemberTableBriefSnapshots, included.ID, included.Key, included.ExternalKind, included.ExternalID, included.RankScore)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	row, err := resolver.latestWorkProgramBriefSnapshotData(ctx, &source, &workstreamArg)
	if err != nil {
		t.Fatalf("latest brief snapshot: %v", err)
	}
	if row == nil {
		t.Fatalf("latest brief snapshot = nil, want included row")
	}
	if row.OperatingStatus != "included" || row.ExecutiveSummary != "Included brief." {
		t.Fatalf("latest brief snapshot did not use run member: %#v", row)
	}
}
