package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestLatestWorkProgramSummarySnapshotDataOverlaysAggregateFields(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	_, err = store.Client().WorkProgramSummarySnapshot.Create().
		SetKey("work-program-summary-snapshot:test").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetTotalCount(22).
		SetNeedsDecisionCount(1).
		SetValidateSignalCount(12).
		SetWaitingReviewCount(2).
		SetClosedPendingReviewCount(1).
		SetModelQualityCount(1).
		SetClosureCandidateCount(4).
		SetDismissedCount(2).
		SetNowCount(16).
		SetHighRiskCount(8).
		SetUnassignedCount(3).
		SetProductActionCount(4).
		SetValidationLeadCount(14).
		SetSourceCoverageLimitedCount(5).
		SetOwnerLoadStatus("attention_required").
		SetOwnerLoadActionCount(7).
		SetOverloadedOwnerCount(1).
		SetAttentionOwnerCount(1).
		SetUnassignedActionCount(2).
		SetBlockerCount(7).
		SetActiveBlockerCount(4).
		SetValidatingBlockerCount(2).
		SetBlockerImpactCount(14).
		SetActiveBlockerImpactCount(8).
		SetDependencyEdgeCount(220).
		SetBlockingDependencyCount(9).
		SetNeedsActionDependencyCount(9).
		SetOperatingStatus("blocked").
		SetDecisionPressure("blocked").
		SetForecastState("gated").
		SetPrimaryRisk("active_blockers").
		SetRecommendedFocus("Focus on persisted blocker state.").
		SetCapabilityGaps("forecast_gated\nactive_blockers").
		SetBreakdownDimensions("program_status\nprogram_status\nowner_load_status").
		SetBreakdownKeys("validate_signal\nclosure_candidate\nattention_required").
		SetBreakdownCounts("12\n4\n1").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_program_summary_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|summary_snapshot").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create summary snapshot: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	snapshot, err := resolver.latestWorkProgramSummarySnapshotData(ctx, &source, &workstream)
	if err != nil {
		t.Fatalf("latest summary snapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatalf("latest summary snapshot = nil, want row")
	}

	summary := applyWorkProgramSummarySnapshotData(&model.WorkProgramSummary{
		ForecastReadiness: &model.WorkForecastReadiness{EtaForecastReady: false},
		TopItems:          []*model.WorkProgramItem{{Key: "typed-detail-survives"}},
	}, snapshot)
	if summary.TotalCount != 22 || summary.ProductActionCount != 4 || summary.ValidationLeadCount != 14 {
		t.Fatalf("summary counts were not overlaid: %#v", summary)
	}
	if summary.OperatingStatus != "blocked" || summary.DecisionPressure != "blocked" || summary.PrimaryRisk == nil || *summary.PrimaryRisk != "active_blockers" {
		t.Fatalf("summary posture was not overlaid: %#v", summary)
	}
	if len(summary.CapabilityGaps) != 2 || summary.CapabilityGaps[0] != "forecast_gated" || summary.CapabilityGaps[1] != "active_blockers" {
		t.Fatalf("capability gaps were not overlaid: %#v", summary.CapabilityGaps)
	}
	if len(summary.Breakdowns) != 3 || summary.Breakdowns[0].Dimension != "program_status" || summary.Breakdowns[0].Key != "validate_signal" || summary.Breakdowns[0].Count != 12 {
		t.Fatalf("breakdowns were not mapped: %#v", summary.Breakdowns)
	}
	if len(summary.TopItems) != 1 || summary.TopItems[0].Key != "typed-detail-survives" {
		t.Fatalf("typed drill-down rows should survive overlay: %#v", summary.TopItems)
	}
	if len(summary.Badges) == 0 {
		t.Fatalf("summary badges were not recomputed after overlay")
	}
}

func TestLatestWorkProgramSummarySnapshotDataUsesRunMembers(t *testing.T) {
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
	included, err := store.Client().WorkProgramSummarySnapshot.Create().
		SetKey("work-program-summary-snapshot:included").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetTotalCount(1).
		SetProductActionCount(1).
		SetOwnerLoadStatus("nominal").
		SetOperatingStatus("included").
		SetDecisionPressure("included").
		SetForecastState("gated").
		SetRecommendedFocus("Included summary.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_summary_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|summary_snapshot").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create included summary snapshot: %v", err)
	}
	_, err = store.Client().WorkProgramSummarySnapshot.Create().
		SetKey("work-program-summary-snapshot:stale-same-timestamp").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetTotalCount(99).
		SetProductActionCount(99).
		SetOwnerLoadStatus("attention_required").
		SetOperatingStatus("noise").
		SetDecisionPressure("noise").
		SetForecastState("gated").
		SetRecommendedFocus("Should be excluded by run membership.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_summary_snapshot").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|summary_snapshot_noise").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale same-timestamp summary snapshot: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, workstream, generatedAt, workProgramRunMemberTableSummarySnapshots, included.ID, included.Key, included.ExternalKind, included.ExternalID, included.RankScore)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:flink-kubernetes-operator"
	snapshot, err := resolver.latestWorkProgramSummarySnapshotData(ctx, &source, &workstreamArg)
	if err != nil {
		t.Fatalf("latest summary snapshot: %v", err)
	}
	if snapshot == nil {
		t.Fatalf("latest summary snapshot = nil, want included row")
	}
	if snapshot.TotalCount != 1 || snapshot.OperatingStatus != "included" {
		t.Fatalf("latest summary snapshot did not use run member: %#v", snapshot)
	}
}
