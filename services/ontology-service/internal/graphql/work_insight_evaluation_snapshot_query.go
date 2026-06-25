package graphql

import (
	"context"
	"sort"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workinsightevaluationsnapshot"
	"cubicle/services/ontology-service/ent/workinsightkindevaluationsnapshot"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkInsightEvaluationSnapshotModel(ctx context.Context, sourceFilter *string) (*model.WorkInsightEvaluation, error) {
	query := r.applyWorkInsightEvaluationSnapshotFilters(
		r.EntClient.WorkInsightEvaluationSnapshot.Query().
			WithKindEvaluations(func(q *genent.WorkInsightKindEvaluationSnapshotQuery) {
				q.Order(
					workinsightkindevaluationsnapshot.ByRankScore(entsql.OrderDesc()),
					workinsightkindevaluationsnapshot.ByInsightKind(),
				)
			}),
		sourceFilter,
	)
	row, err := query.
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
	return workInsightEvaluationSnapshotModel(row), nil
}

func (r *queryResolver) applyWorkInsightEvaluationSnapshotFilters(query *genent.WorkInsightEvaluationSnapshotQuery, sourceFilter *string) *genent.WorkInsightEvaluationSnapshotQuery {
	query = query.Where(
		workinsightevaluationsnapshot.SourceSystemEQ("cubicle_analytics"),
		workinsightevaluationsnapshot.ExternalKindEQ("tpm_work_insight_evaluation_snapshot"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workinsightevaluationsnapshot.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	return query
}

func workInsightEvaluationSnapshotModel(row *genent.WorkInsightEvaluationSnapshot) *model.WorkInsightEvaluation {
	kinds := make([]*model.WorkInsightKindEvaluation, 0, len(row.Edges.KindEvaluations))
	for _, kindRow := range row.Edges.KindEvaluations {
		kinds = append(kinds, workInsightKindEvaluationSnapshotModel(kindRow))
	}
	sort.SliceStable(kinds, func(i, j int) bool {
		if kinds[i].ReadyForProductAction != kinds[j].ReadyForProductAction {
			return kinds[i].ReadyForProductAction
		}
		if kinds[i].ReadyToMeasure != kinds[j].ReadyToMeasure {
			return kinds[i].ReadyToMeasure
		}
		if kinds[i].CurrentInsightCount != kinds[j].CurrentInsightCount {
			return kinds[i].CurrentInsightCount > kinds[j].CurrentInsightCount
		}
		return kinds[i].InsightKind < kinds[j].InsightKind
	})
	total := &workInsightKindEvaluationValues{
		currentInsightCount:    row.CurrentInsightCount,
		reviewRowCount:         row.ReviewRowCount,
		measurementLabelCount:  row.MeasurementLabelCount,
		openReviewRequestCount: row.OpenReviewRequestCount,
	}
	return &model.WorkInsightEvaluation{
		SourceInstance:                       optionalString(row.SourceInstance),
		CurrentInsightCount:                  row.CurrentInsightCount,
		ReviewRowCount:                       row.ReviewRowCount,
		MeasurementLabelCount:                row.MeasurementLabelCount,
		OpenReviewRequestCount:               row.OpenReviewRequestCount,
		MinLabeledTotalRequired:              row.MinLabeledTotalRequired,
		MinLabeledPerKindRequired:            row.MinLabeledPerKindRequired,
		MinPrecisionRateForProductAction:     row.MinPrecisionRateForProductAction,
		MinUsefulSignalRateForProductAction:  row.MinUsefulSignalRateForProductAction,
		MinActionabilityRateForProductAction: row.MinActionabilityRateForProductAction,
		PrecisionRate:                        row.PrecisionRate,
		UsefulSignalRate:                     row.UsefulSignalRate,
		ActionabilityRate:                    row.ActionabilityRate,
		FalsePositiveRate:                    row.FalsePositiveRate,
		MeasurementCoverageRate:              row.MeasurementCoverageRate,
		ReadyToMeasurePrecision:              row.ReadyToMeasurePrecision,
		ReadyToMeasureActionability:          row.ReadyToMeasureActionability,
		ReadyInsightKindCount:                row.ReadyInsightKindCount,
		ProductActionReadyKindCount:          row.ProductActionReadyKindCount,
		QualityGatedInsightKindCount:         row.QualityGatedInsightKindCount,
		GatedInsightKindCount:                row.GatedInsightKindCount,
		RecommendedNextStep:                  row.RecommendedNextStep,
		Badges:                               workInsightEvaluationBadges(total, row.ReadyInsightKindCount, row.ProductActionReadyKindCount, row.QualityGatedInsightKindCount, row.GatedInsightKindCount),
		Kinds:                                kinds,
	}
}

func workInsightKindEvaluationSnapshotModel(row *genent.WorkInsightKindEvaluationSnapshot) *model.WorkInsightKindEvaluation {
	measurementScope := normalizeWorkInsightMeasurementScope(row.MeasurementScope)
	if measurementScope == "" {
		measurementScope = workInsightMeasurementScope(row.InsightKind)
	}
	values := &workInsightKindEvaluationValues{
		insightKind:               row.InsightKind,
		currentInsightCount:       row.CurrentInsightCount,
		reviewRowCount:            row.ReviewRowCount,
		measurementLabelCount:     row.MeasurementLabelCount,
		openReviewRequestCount:    row.OpenReviewRequestCount,
		truthLabeledCount:         row.TruthLabeledCount,
		actionabilityLabeledCount: row.ActionabilityLabeledCount,
		truePositiveCount:         row.TruePositiveCount,
		falsePositiveCount:        row.FalsePositiveCount,
		partialCount:              row.PartialCount,
		actionableCount:           row.ActionableCount,
		needsOwnerCount:           row.NeedsOwnerCount,
	}
	return &model.WorkInsightKindEvaluation{
		InsightKind:               row.InsightKind,
		MeasurementScope:          measurementScope,
		CurrentInsightCount:       row.CurrentInsightCount,
		ReviewRowCount:            row.ReviewRowCount,
		MeasurementLabelCount:     row.MeasurementLabelCount,
		OpenReviewRequestCount:    row.OpenReviewRequestCount,
		TruthLabeledCount:         row.TruthLabeledCount,
		ActionabilityLabeledCount: row.ActionabilityLabeledCount,
		TruePositiveCount:         row.TruePositiveCount,
		FalsePositiveCount:        row.FalsePositiveCount,
		PartialCount:              row.PartialCount,
		ActionableCount:           row.ActionableCount,
		NeedsOwnerCount:           row.NeedsOwnerCount,
		PrecisionRate:             row.PrecisionRate,
		UsefulSignalRate:          row.UsefulSignalRate,
		ActionabilityRate:         row.ActionabilityRate,
		FalsePositiveRate:         row.FalsePositiveRate,
		MeasurementCoverageRate:   row.MeasurementCoverageRate,
		RequiredLabelCount:        row.RequiredLabelCount,
		ReadyToMeasure:            row.ReadyToMeasure,
		ReadyForProductAction:     row.ReadyForProductAction,
		ProductActionGateState:    row.ProductActionGateState,
		ProductActionGateReason:   row.ProductActionGateReason,
		RecommendedAction:         row.RecommendedAction,
		Badges:                    workInsightKindEvaluationBadges(values, row.ReadyToMeasure, row.ReadyForProductAction, row.RequiredLabelCount),
	}
}
