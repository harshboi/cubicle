package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightevaluationsnapshot"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) workInsightMeasurementPacket(ctx context.Context, sourceInstance *string, reviewLimit *int, insightKind *string) (*model.WorkInsightMeasurementPacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workInsightMeasurementPacket requires an Ent-backed ontology store")
	}
	insightKindFilter, err := normalizedWorkInsightKindArgument(insightKind)
	if err != nil {
		return nil, err
	}
	sourceFilter, snapshotRow, err := r.insightMeasurementSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	evaluation, err := r.WorkInsightEvaluation(ctx, sourceFilter)
	if err != nil {
		return nil, err
	}
	if evaluation == nil {
		evaluation = emptyWorkInsightEvaluation(sourceFilter)
	}
	evaluation = workInsightMeasurementEvaluationForKind(evaluation, sourceFilter, insightKindFilter)

	reviewState := "requested"
	reviewKind := "triage_request"
	reviewRows, err := r.workInsightReviewRows(ctx, boundedLimit(reviewLimit, 20, 100), sourceFilter, &reviewState, &reviewKind, insightKindFilter, nil)
	if err != nil {
		return nil, err
	}
	reviewQueueTotalCount, err := r.workInsightReviewQueueCountForOptionalInsightKind(ctx, sourceFilter, &reviewState, &reviewKind, insightKindFilter)
	if err != nil {
		return nil, err
	}
	reviewQueue := make([]*model.WorkInsightReview, 0, len(reviewRows))
	for _, row := range reviewRows {
		reviewQueue = append(reviewQueue, workInsightReviewModel(row))
	}

	readyToMeasure := workInsightMeasurementReady(evaluation)
	productActionReady := workInsightProductActionReady(evaluation, readyToMeasure)
	measurementState := workInsightMeasurementState(evaluation, readyToMeasure, productActionReady)
	measurementGaps, measurementMissingLabelCount := workInsightMeasurementGaps(evaluation)
	measurementGapKinds := workInsightMeasurementGapKinds(measurementGaps)
	measurementGapReviewQueueTotalCount := 0
	measurementGapReviewQueue := []*model.WorkInsightReview{}
	if len(measurementGapKinds) > 0 {
		measurementGapReviewRows, err := r.workInsightReviewRowsForUnmeasuredInsightKinds(ctx, boundedLimit(reviewLimit, 20, 100), sourceFilter, &reviewState, &reviewKind, measurementGapKinds)
		if err != nil {
			return nil, err
		}
		measurementGapReviewQueueTotalCount, err = r.workInsightReviewQueueCountForUnmeasuredInsightKinds(ctx, sourceFilter, &reviewState, &reviewKind, measurementGapKinds)
		if err != nil {
			return nil, err
		}
		measurementGapReviewQueue = make([]*model.WorkInsightReview, 0, len(measurementGapReviewRows))
		for _, row := range measurementGapReviewRows {
			measurementGapReviewQueue = append(measurementGapReviewQueue, workInsightReviewModel(row))
		}
	}
	recommendedFocus := workInsightMeasurementFocus(evaluation, reviewQueue)

	return &model.WorkInsightMeasurementPacket{
		SourceInstance:                      sourceFilter,
		GeneratedAt:                         workInsightMeasurementGeneratedAt(snapshotRow),
		InsightKind:                         insightKindFilter,
		MeasurementState:                    measurementState,
		ReadyToMeasure:                      readyToMeasure,
		ProductActionReady:                  productActionReady,
		CurrentInsightCount:                 evaluation.CurrentInsightCount,
		MeasurementLabelCount:               evaluation.MeasurementLabelCount,
		OpenReviewRequestCount:              evaluation.OpenReviewRequestCount,
		ReviewQueueCount:                    len(reviewQueue),
		ReviewQueueTotalCount:               reviewQueueTotalCount,
		ReadyInsightKindCount:               evaluation.ReadyInsightKindCount,
		ProductActionReadyKindCount:         evaluation.ProductActionReadyKindCount,
		QualityGatedInsightKindCount:        evaluation.QualityGatedInsightKindCount,
		GatedInsightKindCount:               evaluation.GatedInsightKindCount,
		MeasurementGapCount:                 len(measurementGaps),
		MeasurementMissingLabelCount:        measurementMissingLabelCount,
		MeasurementGapReviewQueueCount:      len(measurementGapReviewQueue),
		MeasurementGapReviewQueueTotalCount: measurementGapReviewQueueTotalCount,
		PrecisionRate:                       evaluation.PrecisionRate,
		UsefulSignalRate:                    evaluation.UsefulSignalRate,
		ActionabilityRate:                   evaluation.ActionabilityRate,
		MeasurementCoverageRate:             evaluation.MeasurementCoverageRate,
		AutomationSummary:                   workInsightMeasurementSummary(measurementState, readyToMeasure, productActionReady, evaluation, len(measurementGaps), measurementMissingLabelCount, len(reviewQueue), reviewQueueTotalCount, recommendedFocus),
		RecommendedFocus:                    recommendedFocus,
		Evaluation:                          evaluation,
		MeasurementGaps:                     measurementGaps,
		MeasurementGapReviewQueue:           measurementGapReviewQueue,
		ReviewQueue:                         reviewQueue,
	}, nil
}

func (r *queryResolver) insightMeasurementSourceInstance(ctx context.Context, sourceInstance *string) (*string, *genent.WorkInsightEvaluationSnapshot, error) {
	sourceFilter, err := optionalSourceInstanceArgument(sourceInstance, "sourceInstance")
	if err != nil {
		return nil, nil, err
	}
	snapshotRow, err := r.latestWorkInsightEvaluationSnapshotRow(ctx, sourceFilter)
	if err != nil {
		return nil, nil, err
	}
	if sourceFilter != nil {
		return sourceFilter, snapshotRow, nil
	}
	if snapshotRow != nil && strings.TrimSpace(snapshotRow.SourceInstance) != "" {
		source := strings.TrimSpace(snapshotRow.SourceInstance)
		return &source, snapshotRow, nil
	}
	fallbackSource, err := r.aggregateSourceInstance(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	if fallbackSource != nil && snapshotRow == nil {
		snapshotRow, err = r.latestWorkInsightEvaluationSnapshotRow(ctx, fallbackSource)
		if err != nil {
			return nil, nil, err
		}
	}
	return fallbackSource, snapshotRow, nil
}

func (r *queryResolver) latestWorkInsightEvaluationSnapshotRow(ctx context.Context, sourceFilter *string) (*genent.WorkInsightEvaluationSnapshot, error) {
	row, err := r.applyWorkInsightEvaluationSnapshotFilters(
		r.EntClient.WorkInsightEvaluationSnapshot.Query(),
		sourceFilter,
	).
		Order(
			workinsightevaluationsnapshot.ByGeneratedAt(entsql.OrderDesc()),
			workinsightevaluationsnapshot.ByRankScore(entsql.OrderDesc()),
			workinsightevaluationsnapshot.ByUpdatedAt(entsql.OrderDesc()),
		).
		First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row, nil
}

func workInsightMeasurementGeneratedAt(snapshotRow *genent.WorkInsightEvaluationSnapshot) *string {
	if snapshotRow == nil {
		return nil
	}
	return optionalTime(snapshotRow.GeneratedAt)
}

func (r *queryResolver) workInsightReviewQueueCount(ctx context.Context, sourceFilter *string, reviewState *string, reviewKind *string) (int, error) {
	return r.workInsightReviewQueueCountForOptionalInsightKind(ctx, sourceFilter, reviewState, reviewKind, nil)
}

func (r *queryResolver) workInsightReviewQueueCountForOptionalInsightKind(ctx context.Context, sourceFilter *string, reviewState *string, reviewKind *string, insightKind *string) (int, error) {
	insightKinds := []string{}
	if insightKind != nil {
		insightKinds = append(insightKinds, *insightKind)
	}
	return r.workInsightReviewQueueCountForInsightKinds(ctx, sourceFilter, reviewState, reviewKind, insightKinds)
}

func emptyWorkInsightEvaluation(sourceInstance *string) *model.WorkInsightEvaluation {
	return &model.WorkInsightEvaluation{
		SourceInstance:                       sourceInstance,
		MinLabeledTotalRequired:              minMeasurementLabelTotal,
		MinLabeledPerKindRequired:            minMeasurementLabelsPerKind,
		MinPrecisionRateForProductAction:     minPrecisionRateForProductAction,
		MinUsefulSignalRateForProductAction:  minUsefulSignalRateForProductAction,
		MinActionabilityRateForProductAction: minActionabilityRateForProductAction,
		RecommendedNextStep:                  "No current generated insights are available for evaluation.",
		Badges:                               []*model.WorkActionBadge{},
		Kinds:                                []*model.WorkInsightKindEvaluation{},
	}
}

func normalizedWorkInsightKindArgument(insightKind *string) (*string, error) {
	if insightKind == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*insightKind)
	if value == "" || value == "all" {
		return nil, nil
	}
	typed := workinsight.InsightKind(value)
	if err := workinsight.InsightKindValidator(typed); err != nil {
		return nil, fmt.Errorf("invalid insightKind %q: %w", value, err)
	}
	return &value, nil
}

func workInsightMeasurementEvaluationForKind(evaluation *model.WorkInsightEvaluation, sourceInstance *string, insightKind *string) *model.WorkInsightEvaluation {
	if insightKind == nil {
		return evaluation
	}
	if evaluation == nil {
		evaluation = emptyWorkInsightEvaluation(sourceInstance)
	}
	for _, kind := range evaluation.Kinds {
		if kind == nil || kind.InsightKind != *insightKind {
			continue
		}
		return workInsightSingleKindMeasurementEvaluation(evaluation, kind)
	}
	empty := emptyWorkInsightEvaluation(sourceInstance)
	empty.RecommendedNextStep = fmt.Sprintf("No current %s insights are available for evaluation.", *insightKind)
	return empty
}

func workInsightSingleKindMeasurementEvaluation(evaluation *model.WorkInsightEvaluation, kind *model.WorkInsightKindEvaluation) *model.WorkInsightEvaluation {
	if kind == nil {
		return emptyWorkInsightEvaluation(nil)
	}
	requiredLabelCount := kind.RequiredLabelCount
	if requiredLabelCount <= 0 && evaluation != nil {
		requiredLabelCount = evaluation.MinLabeledPerKindRequired
	}
	readyKindCount := 0
	if kind.ReadyToMeasure {
		readyKindCount = 1
	}
	isProductCandidate := workInsightKindMeasurementScope(kind) == workInsightMeasurementScopeProductCandidate
	productActionReadyKindCount := 0
	if isProductCandidate && kind.ReadyForProductAction {
		productActionReadyKindCount = 1
	}
	qualityGatedKindCount := 0
	gatedKindCount := 0
	if !kind.ReadyToMeasure {
		gatedKindCount = 1
	} else if !kind.ReadyForProductAction && strings.TrimSpace(kind.ProductActionGateState) != "" && strings.TrimSpace(kind.ProductActionGateState) != "context_only" {
		qualityGatedKindCount = 1
	}
	values := &workInsightKindEvaluationValues{
		insightKind:               kind.InsightKind,
		currentInsightCount:       kind.CurrentInsightCount,
		reviewRowCount:            kind.ReviewRowCount,
		measurementLabelCount:     kind.MeasurementLabelCount,
		openReviewRequestCount:    kind.OpenReviewRequestCount,
		truthLabeledCount:         kind.TruthLabeledCount,
		actionabilityLabeledCount: kind.ActionabilityLabeledCount,
		truePositiveCount:         kind.TruePositiveCount,
		falsePositiveCount:        kind.FalsePositiveCount,
		partialCount:              kind.PartialCount,
		actionableCount:           kind.ActionableCount,
		needsOwnerCount:           kind.NeedsOwnerCount,
	}
	return &model.WorkInsightEvaluation{
		SourceInstance:                       evaluation.SourceInstance,
		CurrentInsightCount:                  kind.CurrentInsightCount,
		ReviewRowCount:                       kind.ReviewRowCount,
		MeasurementLabelCount:                kind.MeasurementLabelCount,
		OpenReviewRequestCount:               kind.OpenReviewRequestCount,
		MinLabeledTotalRequired:              requiredLabelCount,
		MinLabeledPerKindRequired:            requiredLabelCount,
		MinPrecisionRateForProductAction:     evaluation.MinPrecisionRateForProductAction,
		MinUsefulSignalRateForProductAction:  evaluation.MinUsefulSignalRateForProductAction,
		MinActionabilityRateForProductAction: evaluation.MinActionabilityRateForProductAction,
		PrecisionRate:                        kind.PrecisionRate,
		UsefulSignalRate:                     kind.UsefulSignalRate,
		ActionabilityRate:                    kind.ActionabilityRate,
		FalsePositiveRate:                    kind.FalsePositiveRate,
		MeasurementCoverageRate:              kind.MeasurementCoverageRate,
		ReadyToMeasurePrecision:              kind.ReadyToMeasure,
		ReadyToMeasureActionability:          kind.ReadyToMeasure,
		ReadyInsightKindCount:                readyKindCount,
		ProductActionReadyKindCount:          productActionReadyKindCount,
		QualityGatedInsightKindCount:         qualityGatedKindCount,
		GatedInsightKindCount:                gatedKindCount,
		RecommendedNextStep:                  kind.RecommendedAction,
		Badges:                               workInsightEvaluationBadges(values, readyKindCount, productActionReadyKindCount, qualityGatedKindCount, gatedKindCount),
		Kinds:                                []*model.WorkInsightKindEvaluation{kind},
	}
}

func workInsightMeasurementReady(evaluation *model.WorkInsightEvaluation) bool {
	if evaluation == nil || evaluation.CurrentInsightCount == 0 {
		return false
	}
	return evaluation.ReadyToMeasurePrecision &&
		evaluation.ReadyToMeasureActionability &&
		workInsightAllProductCandidateKindsReadyToMeasure(evaluation.Kinds)
}

func workInsightProductActionReady(evaluation *model.WorkInsightEvaluation, readyToMeasure bool) bool {
	if evaluation == nil || !readyToMeasure || len(evaluation.Kinds) == 0 {
		return false
	}
	productCandidateKinds := 0
	for _, kind := range evaluation.Kinds {
		if kind == nil || workInsightKindMeasurementScope(kind) != workInsightMeasurementScopeProductCandidate {
			continue
		}
		productCandidateKinds++
		if !kind.ReadyForProductAction {
			return false
		}
	}
	return productCandidateKinds > 0
}

func workInsightMeasurementState(evaluation *model.WorkInsightEvaluation, readyToMeasure bool, productActionReady bool) string {
	if evaluation == nil || evaluation.CurrentInsightCount == 0 {
		return "no_current_insights"
	}
	if productActionReady {
		return "product_action_ready"
	}
	if evaluation.GatedInsightKindCount > 0 || !readyToMeasure {
		return "labeling_needed"
	}
	if evaluation.QualityGatedInsightKindCount > 0 {
		return "quality_gated"
	}
	if evaluation.ProductActionReadyKindCount > 0 {
		return "partially_product_action_ready"
	}
	return "review_required"
}

func workInsightMeasurementFocus(evaluation *model.WorkInsightEvaluation, reviewQueue []*model.WorkInsightReview) *string {
	if evaluation != nil {
		for _, kind := range evaluation.Kinds {
			if kind != nil && !kind.ReadyToMeasure {
				if value := optionalTrimmedPointer(kind.RecommendedAction); value != nil {
					return value
				}
			}
		}
		for _, kind := range evaluation.Kinds {
			if kind != nil && kind.ReadyToMeasure && !kind.ReadyForProductAction {
				if value := optionalTrimmedPointer(kind.RecommendedAction); value != nil {
					return value
				}
			}
		}
	}
	for _, review := range reviewQueue {
		if review == nil {
			continue
		}
		if value := optionalTrimmedPointerValue(review.ReviewNextAction); value != nil {
			return value
		}
		if review.Insight != nil {
			if value := optionalTrimmedPointerValue(review.Insight.RecommendedAction); value != nil {
				return value
			}
		}
	}
	if evaluation != nil {
		if value := optionalTrimmedPointer(evaluation.RecommendedNextStep); value != nil {
			return value
		}
	}
	return nil
}

func workInsightMeasurementGaps(evaluation *model.WorkInsightEvaluation) ([]*model.WorkInsightKindEvaluation, int) {
	if evaluation == nil {
		return []*model.WorkInsightKindEvaluation{}, 0
	}
	gaps := make([]*model.WorkInsightKindEvaluation, 0, len(evaluation.Kinds))
	kindMissingTotal := 0
	for _, kind := range evaluation.Kinds {
		if kind == nil || kind.ReadyToMeasure {
			continue
		}
		gaps = append(gaps, kind)
		kindMissingTotal += workInsightKindMissingMeasurementLabels(kind)
	}
	sort.SliceStable(gaps, func(i, j int) bool {
		leftMissing := workInsightKindMissingMeasurementLabels(gaps[i])
		rightMissing := workInsightKindMissingMeasurementLabels(gaps[j])
		if leftMissing != rightMissing {
			return leftMissing > rightMissing
		}
		if gaps[i].CurrentInsightCount != gaps[j].CurrentInsightCount {
			return gaps[i].CurrentInsightCount > gaps[j].CurrentInsightCount
		}
		return gaps[i].InsightKind < gaps[j].InsightKind
	})
	aggregateMissing := workInsightAggregateMissingMeasurementLabels(evaluation)
	if aggregateMissing > kindMissingTotal {
		return gaps, aggregateMissing
	}
	return gaps, kindMissingTotal
}

func workInsightKindMissingMeasurementLabels(kind *model.WorkInsightKindEvaluation) int {
	if kind == nil || kind.ReadyToMeasure {
		return 0
	}
	truthMissing := kind.RequiredLabelCount - kind.TruthLabeledCount
	actionabilityMissing := kind.RequiredLabelCount - kind.ActionabilityLabeledCount
	if truthMissing < 0 {
		truthMissing = 0
	}
	if actionabilityMissing < 0 {
		actionabilityMissing = 0
	}
	if truthMissing > actionabilityMissing {
		return truthMissing
	}
	return actionabilityMissing
}

func workInsightMeasurementGapKinds(gaps []*model.WorkInsightKindEvaluation) []string {
	out := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		if gap == nil || strings.TrimSpace(gap.InsightKind) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(gap.InsightKind))
	}
	return out
}

func workInsightAggregateMissingMeasurementLabels(evaluation *model.WorkInsightEvaluation) int {
	if evaluation == nil || (evaluation.ReadyToMeasurePrecision && evaluation.ReadyToMeasureActionability) {
		return 0
	}
	required := evaluation.MinLabeledTotalRequired
	if required <= 0 {
		return 0
	}
	missing := required - evaluation.MeasurementLabelCount
	if missing < 0 {
		return 0
	}
	return missing
}

func workInsightMeasurementSummary(measurementState string, readyToMeasure bool, productActionReady bool, evaluation *model.WorkInsightEvaluation, measurementGapCount int, measurementMissingLabelCount int, reviewQueueCount int, reviewQueueTotalCount int, recommendedFocus *string) string {
	currentInsightCount := 0
	measurementLabelCount := 0
	openReviewRequestCount := 0
	productActionReadyKindCount := 0
	gatedInsightKindCount := 0
	qualityGatedInsightKindCount := 0
	if evaluation != nil {
		currentInsightCount = evaluation.CurrentInsightCount
		measurementLabelCount = evaluation.MeasurementLabelCount
		openReviewRequestCount = evaluation.OpenReviewRequestCount
		productActionReadyKindCount = evaluation.ProductActionReadyKindCount
		gatedInsightKindCount = evaluation.GatedInsightKindCount
		qualityGatedInsightKindCount = evaluation.QualityGatedInsightKindCount
	}
	usage := "keep generated insights as review leads; this is not a full workstream automation gate"
	if productActionReady {
		usage = "insight-quality gates pass for eligible kinds, but workstream automation still requires guardrail checks"
	} else if readyToMeasure {
		usage = "measurement is ready, but insight product-action automation remains quality-gated"
	}
	summary := fmt.Sprintf("Insight-quality measurement is %s; %s. %d current insight(s), %d measurement label(s), %d open review request(s), %d queued review row(s) returned out of %d, %d product-action-ready kind(s), %d label-gated kind(s), %d quality-gated kind(s), %d measurement gap kind(s), and %d missing measurement label(s).", measurementState, usage, currentInsightCount, measurementLabelCount, openReviewRequestCount, reviewQueueCount, reviewQueueTotalCount, productActionReadyKindCount, gatedInsightKindCount, qualityGatedInsightKindCount, measurementGapCount, measurementMissingLabelCount)
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}
