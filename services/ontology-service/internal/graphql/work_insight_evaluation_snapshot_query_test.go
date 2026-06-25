package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
)

func TestLatestWorkInsightEvaluationSnapshotModelLoadsPersistedRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	snapshot, err := store.Client().WorkInsightEvaluationSnapshot.Create().
		SetKey("work-insight-evaluation-snapshot:test").
		SetGeneratedAt(generatedAt).
		SetCurrentInsightCount(3).
		SetReviewRowCount(6).
		SetMeasurementLabelCount(3).
		SetOpenReviewRequestCount(1).
		SetMinLabeledTotalRequired(10).
		SetMinLabeledPerKindRequired(10).
		SetMinPrecisionRateForProductAction(0.7).
		SetMinUsefulSignalRateForProductAction(0.8).
		SetMinActionabilityRateForProductAction(0.7).
		SetPrecisionRate(0.3333).
		SetUsefulSignalRate(0.6667).
		SetActionabilityRate(0.6667).
		SetFalsePositiveRate(0.3333).
		SetMeasurementCoverageRate(1.0).
		SetReadyToMeasurePrecision(false).
		SetReadyToMeasureActionability(false).
		SetReadyInsightKindCount(1).
		SetProductActionReadyKindCount(0).
		SetQualityGatedInsightKindCount(1).
		SetGatedInsightKindCount(0).
		SetRecommendedNextStep("Improve blocker_candidate before product action.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_insight_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-21T07:03:16Z|insight_evaluation").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight evaluation snapshot: %v", err)
	}

	_, err = store.Client().WorkInsightKindEvaluationSnapshot.Create().
		SetKey("work-insight-kind-evaluation-snapshot:test").
		SetEvaluationSnapshot(snapshot).
		SetGeneratedAt(generatedAt).
		SetInsightKind("blocker_candidate").
		SetMeasurementScope("product_candidate").
		SetCurrentInsightCount(3).
		SetReviewRowCount(6).
		SetMeasurementLabelCount(3).
		SetOpenReviewRequestCount(1).
		SetTruthLabeledCount(3).
		SetActionabilityLabeledCount(3).
		SetTruePositiveCount(1).
		SetFalsePositiveCount(1).
		SetPartialCount(1).
		SetActionableCount(1).
		SetNeedsOwnerCount(1).
		SetPrecisionRate(0.3333).
		SetUsefulSignalRate(0.6667).
		SetActionabilityRate(0.6667).
		SetFalsePositiveRate(0.3333).
		SetMeasurementCoverageRate(1.0).
		SetRequiredLabelCount(3).
		SetReadyToMeasure(true).
		SetReadyForProductAction(false).
		SetProductActionGateState("quality_gated").
		SetProductActionGateReason("Measured precision below product-action threshold.").
		SetRecommendedAction("Measurement coverage is sufficient, but precision/actionability are too weak for product-action gating.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_insight_kind_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-21T07:03:16Z|insight_kind|blocker_candidate").
		SetRankScore(50).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight kind evaluation snapshot: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	source := "fixture-source"
	evaluation, err := resolver.latestWorkInsightEvaluationSnapshotModel(ctx, &source)
	if err != nil {
		t.Fatalf("latest insight evaluation snapshot: %v", err)
	}
	if evaluation == nil {
		t.Fatalf("latest insight evaluation snapshot = nil, want row")
	}
	if evaluation.SourceInstance == nil || *evaluation.SourceInstance != "fixture-source" {
		t.Fatalf("sourceInstance = %#v", evaluation.SourceInstance)
	}
	if evaluation.CurrentInsightCount != 3 || evaluation.MeasurementLabelCount != 3 || evaluation.ReadyInsightKindCount != 1 || evaluation.QualityGatedInsightKindCount != 1 {
		t.Fatalf("aggregate counts were not mapped: %#v", evaluation)
	}
	if evaluation.PrecisionRate != 0.3333 || evaluation.ActionabilityRate != 0.6667 || evaluation.RecommendedNextStep != "Improve blocker_candidate before product action." {
		t.Fatalf("aggregate rates/narrative were not mapped: %#v", evaluation)
	}
	if len(evaluation.Kinds) != 1 {
		t.Fatalf("kind count = %d, want 1", len(evaluation.Kinds))
	}
	kind := evaluation.Kinds[0]
	if kind.InsightKind != "blocker_candidate" || kind.MeasurementScope != "product_candidate" || !kind.ReadyToMeasure || kind.ReadyForProductAction || kind.ProductActionGateState != "quality_gated" {
		t.Fatalf("kind gate fields were not mapped: %#v", kind)
	}
	if kind.PrecisionRate != 0.3333 || kind.UsefulSignalRate != 0.6667 || kind.ActionabilityRate != 0.6667 || kind.RequiredLabelCount != 3 {
		t.Fatalf("kind rates were not mapped: %#v", kind)
	}
}
