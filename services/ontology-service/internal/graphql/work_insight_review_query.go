package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) workInsightReviewRows(ctx context.Context, limit int, sourceInstance *string, reviewState *string, reviewKind *string, insightKind *string, measurementEligible *bool) ([]*genent.WorkInsightReview, error) {
	insightKinds := []string{}
	if insightKind != nil {
		insightKinds = append(insightKinds, *insightKind)
	}
	return r.workInsightReviewRowsForInsightKinds(ctx, limit, sourceInstance, reviewState, reviewKind, insightKinds, measurementEligible)
}

func (r *queryResolver) workInsightReviewRowsForInsightKinds(ctx context.Context, limit int, sourceInstance *string, reviewState *string, reviewKind *string, insightKinds []string, measurementEligible *bool) ([]*genent.WorkInsightReview, error) {
	return r.workInsightReviewRowsForInsightKindsWithOptions(ctx, limit, sourceInstance, reviewState, reviewKind, insightKinds, measurementEligible, false)
}

func (r *queryResolver) workInsightReviewRowsForUnmeasuredInsightKinds(ctx context.Context, limit int, sourceInstance *string, reviewState *string, reviewKind *string, insightKinds []string) ([]*genent.WorkInsightReview, error) {
	return r.workInsightReviewRowsForInsightKindsWithOptions(ctx, limit, sourceInstance, reviewState, reviewKind, insightKinds, nil, true)
}

func (r *queryResolver) workInsightReviewRowsForInsightKindsWithOptions(ctx context.Context, limit int, sourceInstance *string, reviewState *string, reviewKind *string, insightKinds []string, measurementEligible *bool, excludeResolvedMeasurement bool) ([]*genent.WorkInsightReview, error) {
	sourceFilter, err := r.aggregateSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	if sourceFilter == nil {
		return []*genent.WorkInsightReview{}, nil
	}
	query := r.EntClient.WorkInsightReview.Query().
		WithInsight(func(q *genent.WorkInsightQuery) {
			q.WithLatestEvidence()
			q.WithReviews(func(rq *genent.WorkInsightReviewQuery) {
				rq = applyWorkInsightReviewSourceFilter(rq, sourceFilter)
				rq.Order(
					workinsightreview.ByMeasurementEligible(entsql.OrderDesc()),
					workinsightreview.ByReviewedAt(entsql.OrderDesc()),
					workinsightreview.ByUpdatedAt(entsql.OrderDesc()),
				)
			})
		}).
		Order(
			workinsightreview.ByMeasurementEligible(entsql.OrderAsc()),
			workinsightreview.ByUpdatedAt(entsql.OrderDesc()),
			workinsightreview.ByCreatedAt(entsql.OrderDesc()),
		).
		Limit(limit)

	query = applyWorkInsightReviewSourceFilter(query, sourceFilter)
	query = query.Where(workinsightreview.HasInsightWith(
		workinsight.SourceSystemEQ("cubicle_analytics"),
		workinsight.SourceInstanceEQ(*sourceFilter),
		workinsight.ExternalKindEQ("tpm_insight"),
		workinsight.ProducerStateEQ(workinsight.ProducerStateCurrent),
	))
	query, err = applyWorkInsightReviewStateKindFilters(query, reviewState, reviewKind)
	if err != nil {
		return nil, err
	}
	query, err = applyWorkInsightReviewInsightKindFilters(query, insightKinds)
	if err != nil {
		return nil, err
	}
	if measurementEligible != nil {
		if *measurementEligible {
			query = query.Where(workInsightReviewTrustedMeasurementPredicate())
		} else {
			query = query.Where(workinsightreview.Not(workInsightReviewTrustedMeasurementPredicate()))
		}
	}
	if excludeResolvedMeasurement {
		query = query.Where(workInsightReviewUnresolvedMeasurementPredicate())
	}
	return query.All(ctx)
}

func (r *queryResolver) workInsightReviewQueueCountForInsightKinds(ctx context.Context, sourceInstance *string, reviewState *string, reviewKind *string, insightKinds []string) (int, error) {
	return r.workInsightReviewQueueCountForInsightKindsWithOptions(ctx, sourceInstance, reviewState, reviewKind, insightKinds, false)
}

func (r *queryResolver) workInsightReviewQueueCountForUnmeasuredInsightKinds(ctx context.Context, sourceInstance *string, reviewState *string, reviewKind *string, insightKinds []string) (int, error) {
	return r.workInsightReviewQueueCountForInsightKindsWithOptions(ctx, sourceInstance, reviewState, reviewKind, insightKinds, true)
}

func (r *queryResolver) workInsightReviewQueueCountForInsightKindsWithOptions(ctx context.Context, sourceInstance *string, reviewState *string, reviewKind *string, insightKinds []string, excludeResolvedMeasurement bool) (int, error) {
	sourceFilter, err := r.aggregateSourceInstance(ctx, sourceInstance)
	if err != nil {
		return 0, err
	}
	if sourceFilter == nil {
		return 0, nil
	}
	query := r.EntClient.WorkInsightReview.Query()
	query = applyWorkInsightReviewSourceFilter(query, sourceFilter)
	query = query.Where(workinsightreview.HasInsightWith(
		workinsight.SourceSystemEQ("cubicle_analytics"),
		workinsight.SourceInstanceEQ(*sourceFilter),
		workinsight.ExternalKindEQ("tpm_insight"),
		workinsight.ProducerStateEQ(workinsight.ProducerStateCurrent),
	))
	query, err = applyWorkInsightReviewStateKindFilters(query, reviewState, reviewKind)
	if err != nil {
		return 0, err
	}
	query, err = applyWorkInsightReviewInsightKindFilters(query, insightKinds)
	if err != nil {
		return 0, err
	}
	if excludeResolvedMeasurement {
		query = query.Where(workInsightReviewUnresolvedMeasurementPredicate())
	}
	return query.Count(ctx)
}

func applyWorkInsightReviewStateKindFilters(query *genent.WorkInsightReviewQuery, reviewState *string, reviewKind *string) (*genent.WorkInsightReviewQuery, error) {
	if reviewState != nil {
		state := strings.TrimSpace(*reviewState)
		if state != "" && state != "all" {
			value := workinsightreview.ReviewState(state)
			if err := workinsightreview.ReviewStateValidator(value); err != nil {
				return nil, err
			}
			query = query.Where(workinsightreview.ReviewStateEQ(value))
		}
	}
	if reviewKind != nil {
		kind := strings.TrimSpace(*reviewKind)
		if kind == "" || kind == "all" {
			return query, nil
		}
		value := workinsightreview.ReviewKind(kind)
		if err := workinsightreview.ReviewKindValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workinsightreview.ReviewKindEQ(value))
	}
	return query, nil
}

func applyWorkInsightReviewInsightKindFilters(query *genent.WorkInsightReviewQuery, insightKinds []string) (*genent.WorkInsightReviewQuery, error) {
	values := make([]workinsight.InsightKind, 0, len(insightKinds))
	seen := map[string]bool{}
	for _, insightKind := range insightKinds {
		insightKind = strings.TrimSpace(insightKind)
		if insightKind == "" || insightKind == "all" || seen[insightKind] {
			continue
		}
		value := workinsight.InsightKind(insightKind)
		if err := workinsight.InsightKindValidator(value); err != nil {
			return nil, err
		}
		values = append(values, value)
		seen[insightKind] = true
	}
	if len(values) > 0 {
		query = query.Where(workinsightreview.HasInsightWith(workinsight.InsightKindIn(values...)))
	}
	return query, nil
}

func workInsightReviewUnresolvedMeasurementPredicate() predicate.WorkInsightReview {
	return workinsightreview.Not(workinsightreview.HasInsightWith(
		workinsight.HasReviewsWith(
			workInsightReviewTrustedMeasurementPredicate(),
			workinsightreview.ReviewStateIn(workinsightreview.ReviewStateAccepted, workinsightreview.ReviewStateDismissed, workinsightreview.ReviewStateResolved),
			workinsightreview.TruthLabelIn(workinsightreview.TruthLabelTruePositive, workinsightreview.TruthLabelFalsePositive),
			workinsightreview.ActionabilityLabelIn(workinsightreview.ActionabilityLabelActionable, workinsightreview.ActionabilityLabelNeedsOwner, workinsightreview.ActionabilityLabelNotActionable),
		),
	))
}

func workInsightReviewTrustedMeasurementPredicate() predicate.WorkInsightReview {
	return workinsightreview.And(
		workinsightreview.MeasurementEligibleEQ(true),
		workinsightreview.Or(
			workinsightreview.ReviewKindEQ(workinsightreview.ReviewKindHumanAssessment),
			workinsightreview.And(
				workinsightreview.ReviewKindEQ(workinsightreview.ReviewKindEvaluationLabel),
				workinsightreview.LabelQualityEQ(workinsightreview.LabelQualityGold),
			),
		),
	)
}

func applyWorkInsightReviewSourceFilter(query *genent.WorkInsightReviewQuery, sourceInstance *string) *genent.WorkInsightReviewQuery {
	if sourceInstance == nil {
		return query
	}
	return query.Where(
		workinsightreview.SourceInstanceEQ(*sourceInstance),
		workinsightreview.Or(
			workinsightreview.And(
				workinsightreview.SourceSystemEQ("cubicle_analytics"),
				workinsightreview.ExternalKindEQ("tpm_insight_review"),
			),
			workinsightreview.And(
				workinsightreview.SourceSystemEQ("cubicle_evaluation"),
				workinsightreview.ExternalKindEQ("tpm_review_label"),
			),
		),
	)
}
