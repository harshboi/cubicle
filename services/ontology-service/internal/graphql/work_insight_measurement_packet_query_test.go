package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkInsightMeasurementPacketCombinesPersistedEvaluationAndReviewQueue(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	snapshot, err := store.Client().WorkInsightEvaluationSnapshot.Create().
		SetKey("work-insight-evaluation-snapshot:packet").
		SetGeneratedAt(generatedAt).
		SetCurrentInsightCount(3).
		SetReviewRowCount(2).
		SetMeasurementLabelCount(1).
		SetOpenReviewRequestCount(2).
		SetMinLabeledTotalRequired(10).
		SetMinLabeledPerKindRequired(10).
		SetMinPrecisionRateForProductAction(0.7).
		SetMinUsefulSignalRateForProductAction(0.8).
		SetMinActionabilityRateForProductAction(0.7).
		SetPrecisionRate(0.5).
		SetUsefulSignalRate(0.5).
		SetActionabilityRate(0.0).
		SetFalsePositiveRate(0.5).
		SetMeasurementCoverageRate(0.3333).
		SetReadyToMeasurePrecision(false).
		SetReadyToMeasureActionability(false).
		SetReadyInsightKindCount(0).
		SetProductActionReadyKindCount(0).
		SetQualityGatedInsightKindCount(0).
		SetGatedInsightKindCount(1).
		SetRecommendedNextStep("Gold-label status_summary before using insight automation.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-21T07:03:16Z|insight_evaluation").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight evaluation snapshot: %v", err)
	}
	_, err = store.Client().WorkInsightKindEvaluationSnapshot.Create().
		SetKey("work-insight-kind-evaluation-snapshot:status-summary").
		SetEvaluationSnapshot(snapshot).
		SetGeneratedAt(generatedAt).
		SetInsightKind("status_summary").
		SetCurrentInsightCount(3).
		SetReviewRowCount(2).
		SetMeasurementLabelCount(1).
		SetOpenReviewRequestCount(2).
		SetTruthLabeledCount(1).
		SetActionabilityLabeledCount(0).
		SetTruePositiveCount(1).
		SetFalsePositiveCount(0).
		SetPartialCount(0).
		SetActionableCount(0).
		SetNeedsOwnerCount(0).
		SetPrecisionRate(1.0).
		SetUsefulSignalRate(1.0).
		SetActionabilityRate(0.0).
		SetFalsePositiveRate(0.0).
		SetMeasurementCoverageRate(0.3333).
		SetRequiredLabelCount(3).
		SetReadyToMeasure(false).
		SetReadyForProductAction(false).
		SetProductActionGateState("measurement_gated").
		SetProductActionGateReason("Needs 2 more gold labels before product-action quality can be measured.").
		SetRecommendedAction("Gold-label 2 current status_summary insight(s) before promoting this kind beyond validation leads.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_kind_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-21T07:03:16Z|insight_kind|status_summary").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight kind evaluation snapshot: %v", err)
	}

	insight, err := store.Client().WorkInsight.Create().
		SetKey("work-insight:status-summary").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityHigh).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#1084").
		SetTitle("Status summary needs validation").
		SetDetails("Generated status summary should be checked by a TPM.").
		SetRecommendedAction("Validate the generated status summary against source evidence.").
		SetModelName("tpm_rules").
		SetModelVersion("test").
		SetModelMethod("fixture").
		SetScore(88).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight").
		SetExternalID("fixture-source|status-summary|1084").
		SetRankScore(88).
		Save(ctx)
	if err != nil {
		t.Fatalf("create work insight: %v", err)
	}
	_, err = store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:status-summary").
		SetInsight(insight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetTruthLabel(workinsightreview.TruthLabelUnknown).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelUnknown).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetNextAction("Add a gold label for this status summary.").
		SetRationale("Measurement gate needs more labels.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight_review").
		SetExternalID("fixture-source|status-summary|1084|review").
		Save(ctx)
	if err != nil {
		t.Fatalf("create work insight review: %v", err)
	}
	otherInsight, err := store.Client().WorkInsight.Create().
		SetKey("work-insight:status-summary-other").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityMedium).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#1085").
		SetTitle("Second status summary needs validation").
		SetRecommendedAction("Validate the second generated status summary.").
		SetScore(80).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight").
		SetExternalID("fixture-source|status-summary|1085").
		SetRankScore(80).
		Save(ctx)
	if err != nil {
		t.Fatalf("create second work insight: %v", err)
	}
	_, err = store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:status-summary-other").
		SetInsight(otherInsight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetTruthLabel(workinsightreview.TruthLabelUnknown).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelUnknown).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetNextAction("Add another gold label for status summary.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight_review").
		SetExternalID("fixture-source|status-summary|1085|review").
		Save(ctx)
	if err != nil {
		t.Fatalf("create second work insight review: %v", err)
	}
	_, err = store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:status-summary-other-label").
		SetInsight(otherInsight).
		SetReviewKind(workinsightreview.ReviewKindEvaluationLabel).
		SetReviewState(workinsightreview.ReviewStateAccepted).
		SetTruthLabel(workinsightreview.TruthLabelTruePositive).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelNeedsOwner).
		SetLabelSet("fixture-gold").
		SetLabelQuality(workinsightreview.LabelQualityGold).
		SetMeasurementEligible(true).
		SetReviewerKind(workinsightreview.ReviewerKindImported).
		SetReviewerKey("fixture-gold").
		SetRationale("Resolved measurement label should remove stale triage from the measurement gap queue.").
		SetReviewedAt(generatedAt.Add(time.Minute)).
		SetSourceSystem("cubicle_evaluation").
		SetSourceInstance(source).
		SetExternalKind("tpm_review_label").
		SetExternalID("fixture-source|status-summary|1085|gold-label").
		Save(ctx)
	if err != nil {
		t.Fatalf("create resolved work insight measurement label: %v", err)
	}
	unrelatedInsight, err := store.Client().WorkInsight.Create().
		SetKey("work-insight:blocker-unrelated").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityMedium).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#1086").
		SetTitle("Unrelated blocker review is not a measurement gap").
		SetRecommendedAction("Validate unrelated blocker candidate.").
		SetScore(70).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight").
		SetExternalID("fixture-source|blocker|1086").
		SetRankScore(70).
		Save(ctx)
	if err != nil {
		t.Fatalf("create unrelated work insight: %v", err)
	}
	_, err = store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:blocker-unrelated").
		SetInsight(unrelatedInsight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetTruthLabel(workinsightreview.TruthLabelUnknown).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelUnknown).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetNextAction("Validate unrelated blocker candidate.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight_review").
		SetExternalID("fixture-source|blocker|1086|review").
		Save(ctx)
	if err != nil {
		t.Fatalf("create unrelated work insight review: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	reviewLimit := 1
	sourceArg := source
	packet, err := resolver.WorkInsightMeasurementPacket(ctx, &sourceArg, &reviewLimit, nil)
	if err != nil {
		t.Fatalf("measurement packet: %v", err)
	}

	if packet.SourceInstance == nil || *packet.SourceInstance != source {
		t.Fatalf("packet source = %#v, want %s", packet.SourceInstance, source)
	}
	if packet.GeneratedAt == nil || !strings.Contains(*packet.GeneratedAt, "2026-06-21T07:03:16Z") {
		t.Fatalf("packet generatedAt = %#v, want persisted snapshot time", packet.GeneratedAt)
	}
	if packet.MeasurementState != "labeling_needed" || packet.ReadyToMeasure || packet.ProductActionReady {
		t.Fatalf("packet readiness = state:%s ready:%v product:%v, want labeling_needed/false/false", packet.MeasurementState, packet.ReadyToMeasure, packet.ProductActionReady)
	}
	if packet.CurrentInsightCount != 3 || packet.MeasurementLabelCount != 1 || packet.OpenReviewRequestCount != 2 || packet.ReviewQueueCount != 1 || packet.ReviewQueueTotalCount != 3 {
		t.Fatalf("packet counts = current:%d labels:%d open:%d queue:%d total:%d, want 3/1/2/1/3", packet.CurrentInsightCount, packet.MeasurementLabelCount, packet.OpenReviewRequestCount, packet.ReviewQueueCount, packet.ReviewQueueTotalCount)
	}
	if packet.GatedInsightKindCount != 1 || packet.QualityGatedInsightKindCount != 0 || packet.ProductActionReadyKindCount != 0 {
		t.Fatalf("packet gate counts = gated:%d quality:%d product:%d, want 1/0/0", packet.GatedInsightKindCount, packet.QualityGatedInsightKindCount, packet.ProductActionReadyKindCount)
	}
	if packet.MeasurementGapCount != 1 || packet.MeasurementMissingLabelCount != 9 {
		t.Fatalf("packet measurement gaps = count:%d missing:%d, want 1/9", packet.MeasurementGapCount, packet.MeasurementMissingLabelCount)
	}
	if packet.Evaluation == nil || len(packet.Evaluation.Kinds) != 1 || packet.Evaluation.Kinds[0].InsightKind != "status_summary" {
		t.Fatalf("packet evaluation = %#v, want persisted status_summary kind", packet.Evaluation)
	}
	if len(packet.MeasurementGaps) != 1 || packet.MeasurementGaps[0].InsightKind != "status_summary" {
		t.Fatalf("packet measurement gaps = %#v, want status_summary gap", packet.MeasurementGaps)
	}
	if packet.MeasurementGapReviewQueueCount != 1 || packet.MeasurementGapReviewQueueTotalCount != 1 {
		t.Fatalf("packet measurement gap queue counts = returned:%d total:%d, want 1/1", packet.MeasurementGapReviewQueueCount, packet.MeasurementGapReviewQueueTotalCount)
	}
	if len(packet.MeasurementGapReviewQueue) != 1 || packet.MeasurementGapReviewQueue[0].Insight.InsightKind != "status_summary" || packet.MeasurementGapReviewQueue[0].Insight.SubjectKey != "apache/flink-kubernetes-operator#1084" {
		t.Fatalf("packet measurement gap queue = %#v, want one unmeasured status_summary review", packet.MeasurementGapReviewQueue)
	}
	if len(packet.ReviewQueue) != 1 || packet.ReviewQueue[0].ReviewState != "requested" || packet.ReviewQueue[0].Insight.Key == "" {
		t.Fatalf("packet review queue = %#v, want one requested row with insight", packet.ReviewQueue)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Gold-label 2 current status_summary") {
		t.Fatalf("packet focus = %#v, want kind labeling action", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "not a full workstream automation gate") || !strings.Contains(packet.AutomationSummary, "1 queued review row(s) returned out of 3") || !strings.Contains(packet.AutomationSummary, "9 missing measurement label(s)") {
		t.Fatalf("packet summary = %q, want conservative scope wording and capped queue count", packet.AutomationSummary)
	}
}

func TestWorkInsightMeasurementPacketFiltersToBlockerCandidateQueue(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	generatedAt := time.Date(2026, 6, 23, 16, 0, 0, 0, time.UTC)
	snapshot, err := store.Client().WorkInsightEvaluationSnapshot.Create().
		SetKey("work-insight-evaluation-snapshot:blocker-filter").
		SetGeneratedAt(generatedAt).
		SetCurrentInsightCount(12).
		SetReviewRowCount(12).
		SetMeasurementLabelCount(2).
		SetOpenReviewRequestCount(12).
		SetMinLabeledTotalRequired(10).
		SetMinLabeledPerKindRequired(10).
		SetMinPrecisionRateForProductAction(0.7).
		SetMinUsefulSignalRateForProductAction(0.8).
		SetMinActionabilityRateForProductAction(0.7).
		SetPrecisionRate(1.0).
		SetUsefulSignalRate(1.0).
		SetActionabilityRate(0.5).
		SetFalsePositiveRate(0.0).
		SetMeasurementCoverageRate(0.1667).
		SetReadyToMeasurePrecision(false).
		SetReadyToMeasureActionability(false).
		SetReadyInsightKindCount(0).
		SetProductActionReadyKindCount(0).
		SetQualityGatedInsightKindCount(0).
		SetGatedInsightKindCount(2).
		SetRecommendedNextStep("Gold-label blocker_candidate before clearing blockers automatically.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-23T16:00:00Z|insight_evaluation").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight evaluation snapshot: %v", err)
	}
	_, err = store.Client().WorkInsightKindEvaluationSnapshot.Create().
		SetKey("work-insight-kind-evaluation-snapshot:blocker-candidate").
		SetEvaluationSnapshot(snapshot).
		SetGeneratedAt(generatedAt).
		SetInsightKind("blocker_candidate").
		SetMeasurementScope(workInsightMeasurementScopeProductCandidate).
		SetCurrentInsightCount(10).
		SetReviewRowCount(20).
		SetMeasurementLabelCount(0).
		SetOpenReviewRequestCount(10).
		SetRequiredLabelCount(10).
		SetReadyToMeasure(false).
		SetReadyForProductAction(false).
		SetProductActionGateState("measurement_gated").
		SetProductActionGateReason("Needs blocker truth labels before materializing blockers.").
		SetRecommendedAction("Gold-label 10 blocker_candidate insight(s) before promoting blocker automation.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_kind_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-23T16:00:00Z|insight_kind|blocker_candidate").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocker kind evaluation: %v", err)
	}
	_, err = store.Client().WorkInsightKindEvaluationSnapshot.Create().
		SetKey("work-insight-kind-evaluation-snapshot:status-summary-filter").
		SetEvaluationSnapshot(snapshot).
		SetGeneratedAt(generatedAt).
		SetInsightKind("status_summary").
		SetCurrentInsightCount(2).
		SetReviewRowCount(2).
		SetMeasurementLabelCount(2).
		SetOpenReviewRequestCount(2).
		SetRequiredLabelCount(10).
		SetReadyToMeasure(false).
		SetReadyForProductAction(false).
		SetProductActionGateState("measurement_gated").
		SetProductActionGateReason("Needs status summary labels.").
		SetRecommendedAction("Gold-label status_summary separately.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_kind_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-23T16:00:00Z|insight_kind|status_summary").
		SetRankScore(80).
		Save(ctx)
	if err != nil {
		t.Fatalf("create status kind evaluation: %v", err)
	}
	blockerInsight, err := store.Client().WorkInsight.Create().
		SetKey("work-insight:blocker-filter").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityHigh).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#1099").
		SetTitle("Blocked PR candidate").
		SetRecommendedAction("Validate whether this PR is blocked.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight").
		SetExternalID("fixture-source|blocker|1099").
		SetRankScore(99).
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocker insight: %v", err)
	}
	_, err = store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:blocker-filter").
		SetInsight(blockerInsight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetTruthLabel(workinsightreview.TruthLabelUnknown).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelUnknown).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetNextAction("Add a blocker gold label.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight_review").
		SetExternalID("fixture-source|blocker|1099|review").
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocker review: %v", err)
	}
	_, err = store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:blocker-filter-source-oracle").
		SetInsight(blockerInsight).
		SetReviewKind(workinsightreview.ReviewKindEvaluationLabel).
		SetReviewState(workinsightreview.ReviewStateAccepted).
		SetTruthLabel(workinsightreview.TruthLabelTruePositive).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelNeedsOwner).
		SetLabelSet("source_oracle_seed").
		SetLabelQuality(workinsightreview.LabelQualityCandidate).
		SetMeasurementEligible(true).
		SetReviewerKind(workinsightreview.ReviewerKindImported).
		SetReviewerKey("source_oracle").
		SetRationale("Raw flag should not hide the gold-label gap.").
		SetSourceSystem("cubicle_evaluation").
		SetSourceInstance(source).
		SetExternalKind("tpm_review_label").
		SetExternalID("fixture-source|blocker|1099|source-oracle-label").
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocker source-oracle label: %v", err)
	}
	statusInsight, err := store.Client().WorkInsight.Create().
		SetKey("work-insight:status-filter").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityMedium).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#1100").
		SetTitle("Status summary candidate").
		SetRecommendedAction("Validate status summary.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight").
		SetExternalID("fixture-source|status|1100").
		SetRankScore(50).
		Save(ctx)
	if err != nil {
		t.Fatalf("create status insight: %v", err)
	}
	_, err = store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:status-filter").
		SetInsight(statusInsight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetTruthLabel(workinsightreview.TruthLabelUnknown).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelUnknown).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetNextAction("Add a status summary gold label.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight_review").
		SetExternalID("fixture-source|status|1100|review").
		Save(ctx)
	if err != nil {
		t.Fatalf("create status review: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	reviewLimit := 20
	sourceArg := source
	insightKind := "blocker_candidate"
	packet, err := resolver.WorkInsightMeasurementPacket(ctx, &sourceArg, &reviewLimit, &insightKind)
	if err != nil {
		t.Fatalf("blocker measurement packet: %v", err)
	}

	if packet.InsightKind == nil || *packet.InsightKind != "blocker_candidate" {
		t.Fatalf("packet insightKind = %#v, want blocker_candidate", packet.InsightKind)
	}
	if packet.CurrentInsightCount != 10 || packet.MeasurementLabelCount != 0 || packet.OpenReviewRequestCount != 10 {
		t.Fatalf("filtered counts = current:%d labels:%d open:%d, want blocker-only 10/0/10", packet.CurrentInsightCount, packet.MeasurementLabelCount, packet.OpenReviewRequestCount)
	}
	if packet.ReviewQueueTotalCount != 1 || len(packet.ReviewQueue) != 1 || packet.ReviewQueue[0].Insight.InsightKind != "blocker_candidate" {
		t.Fatalf("filtered review queue = total:%d rows:%#v, want only blocker review", packet.ReviewQueueTotalCount, packet.ReviewQueue)
	}
	if packet.MeasurementGapCount != 1 || packet.MeasurementMissingLabelCount != 10 || packet.MeasurementGapReviewQueueTotalCount != 1 {
		t.Fatalf("filtered measurement gap = count:%d missing:%d queue:%d, want 1/10/1", packet.MeasurementGapCount, packet.MeasurementMissingLabelCount, packet.MeasurementGapReviewQueueTotalCount)
	}
	if packet.Evaluation == nil || len(packet.Evaluation.Kinds) != 1 || packet.Evaluation.Kinds[0].InsightKind != "blocker_candidate" {
		t.Fatalf("filtered evaluation = %#v, want one blocker kind", packet.Evaluation)
	}
	if !strings.Contains(packet.AutomationSummary, "10 current insight(s)") || !strings.Contains(packet.AutomationSummary, "10 missing measurement label(s)") {
		t.Fatalf("packet summary = %q, want blocker-specific counts", packet.AutomationSummary)
	}
	trustedOnly := true
	trustedRows, err := resolver.WorkInsightReviews(ctx, &reviewLimit, &sourceArg, nil, nil, &insightKind, &trustedOnly)
	if err != nil {
		t.Fatalf("trusted work insight reviews: %v", err)
	}
	if len(trustedRows) != 0 {
		t.Fatalf("trusted review rows = %#v, want no raw source-oracle candidate rows", trustedRows)
	}
	nonMeasurement := false
	nonMeasurementRows, err := resolver.WorkInsightReviews(ctx, &reviewLimit, &sourceArg, nil, nil, &insightKind, &nonMeasurement)
	if err != nil {
		t.Fatalf("non-measurement work insight reviews: %v", err)
	}
	var sourceOracleReview *model.WorkInsightReview
	for _, row := range nonMeasurementRows {
		if row.Key == "work-insight-review:blocker-filter-source-oracle" {
			sourceOracleReview = row
			break
		}
	}
	if sourceOracleReview == nil || sourceOracleReview.MeasurementEligible || !hasWorkActionBadge(sourceOracleReview.Badges, "review:not_measurement") || hasWorkActionBadge(sourceOracleReview.Badges, "review:measurement_eligible") {
		t.Fatalf("source-oracle candidate review serialized as measurement-grade: %#v", sourceOracleReview)
	}
}

func TestWorkInsightProductActionReadyUsesProductCandidateKinds(t *testing.T) {
	evaluation := &model.WorkInsightEvaluation{
		CurrentInsightCount:         2,
		ProductActionReadyKindCount: 1,
		Kinds: []*model.WorkInsightKindEvaluation{
			{InsightKind: "status_summary", ReadyToMeasure: true, ReadyForProductAction: true},
			{InsightKind: "developer_correlation", ReadyToMeasure: true, ReadyForProductAction: false, ProductActionGateState: "context_only"},
		},
	}
	if !workInsightProductActionReady(evaluation, true) {
		t.Fatalf("context-only kinds should not prevent product-candidate readiness: %#v", evaluation)
	}
	if state := workInsightMeasurementState(evaluation, true, true); state != "product_action_ready" {
		t.Fatalf("measurement state = %q, want product_action_ready", state)
	}

	contextOnly := &model.WorkInsightEvaluation{
		CurrentInsightCount:         1,
		ProductActionReadyKindCount: 0,
		Kinds: []*model.WorkInsightKindEvaluation{
			{InsightKind: "developer_correlation", ReadyToMeasure: true, ReadyForProductAction: false, ProductActionGateState: "context_only"},
		},
	}
	if workInsightProductActionReady(contextOnly, true) {
		t.Fatalf("context-only kinds alone must not be product-action ready: %#v", contextOnly)
	}

	staleAggregate := &model.WorkInsightEvaluation{
		CurrentInsightCount:         1,
		ProductActionReadyKindCount: 1,
		Kinds: []*model.WorkInsightKindEvaluation{
			{
				InsightKind:            "status_summary",
				MeasurementScope:       workInsightMeasurementScopeProductCandidate,
				ReadyToMeasure:         true,
				ReadyForProductAction:  false,
				ProductActionGateState: "quality_gated",
			},
		},
	}
	if workInsightProductActionReady(staleAggregate, true) {
		t.Fatalf("stale aggregate ready count must not override child product-candidate gate: %#v", staleAggregate)
	}
}

func TestWorkInsightMeasurementGapsIncludesAggregateMissingLabels(t *testing.T) {
	evaluation := &model.WorkInsightEvaluation{
		CurrentInsightCount:         12,
		MeasurementLabelCount:       5,
		MinLabeledTotalRequired:     10,
		ReadyToMeasurePrecision:     false,
		ReadyToMeasureActionability: false,
		Kinds: []*model.WorkInsightKindEvaluation{
			{
				InsightKind:               "blocker_candidate",
				CurrentInsightCount:       5,
				MeasurementLabelCount:     5,
				TruthLabeledCount:         5,
				ActionabilityLabeledCount: 5,
				RequiredLabelCount:        5,
				ReadyToMeasure:            true,
			},
		},
	}

	gaps, missing := workInsightMeasurementGaps(evaluation)
	if len(gaps) != 0 || missing != 5 {
		t.Fatalf("measurement gaps = %d missing:%d, want aggregate-only gap with 5 missing labels", len(gaps), missing)
	}
}

func hasWorkActionBadge(rows []*model.WorkActionBadge, key string) bool {
	for _, row := range rows {
		if row != nil && row.Key == key {
			return true
		}
	}
	return false
}
