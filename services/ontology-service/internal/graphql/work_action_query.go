package graphql

import (
	"context"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workactionobservation"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) workActionRows(ctx context.Context, state string, limit int) ([]*genent.WorkAction, error) {
	return r.workActionRowsForSource(ctx, state, limit, nil)
}

func (r *queryResolver) workActionRowsForSource(ctx context.Context, state string, limit int, sourceInstance *string) ([]*genent.WorkAction, error) {
	return r.workActionRowsForSourceAndDecision(ctx, state, limit, sourceInstance, nil, nil)
}

func (r *queryResolver) workActionRowsForSourceAndDecision(ctx context.Context, state string, limit int, sourceInstance *string, decisionState *string, ownerKey *string) ([]*genent.WorkAction, error) {
	sourceFilter, err := optionalSourceInstanceArgument(sourceInstance, "sourceInstance")
	if err != nil {
		return nil, err
	}
	query := r.EntClient.WorkAction.Query().
		WithPullRequest(func(q *genent.PullRequestQuery) { q.WithTickets() }).
		WithTicket(func(q *genent.TicketQuery) { q.WithPullRequests() }).
		WithLatestEvidence().
		WithObservations(workActionObservationDetails(sourceFilter)).
		WithSourceInsights(workActionSourceInsightDetails(sourceFilter)).
		Order(
			workaction.ByRankScore(entsql.OrderDesc()),
			workaction.ByLastActivityAt(entsql.OrderDesc()),
		).
		Limit(limit)

	if sourceFilter != nil {
		query = query.Where(
			workaction.SourceSystemEQ("cubicle_analytics"),
			workaction.SourceInstanceEQ(*sourceFilter),
			workaction.ExternalKindEQ("tpm_work_action"),
		)
	}
	if decisionState != nil && *decisionState != "" {
		value := workaction.DecisionState(*decisionState)
		if err := workaction.DecisionStateValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workaction.DecisionStateEQ(value))
	}
	if ownerKey != nil && *ownerKey != "" {
		query = query.Where(workaction.OwnerKeyEQ(*ownerKey))
	}

	if state != "all" {
		value := workaction.ActionState(state)
		if err := workaction.ActionStateValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workaction.ActionStateEQ(value))
	}

	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	if err := r.hydrateWorkActionSourceInsightDetails(ctx, rows, sourceFilter); err != nil {
		return nil, err
	}
	sortWorkActions(rows)
	return rows, nil
}

func workActionDetails(sourceFilter *string) func(*genent.WorkActionQuery) {
	return func(q *genent.WorkActionQuery) {
		if sourceFilter != nil {
			q.Where(
				workaction.SourceSystemEQ("cubicle_analytics"),
				workaction.SourceInstanceEQ(*sourceFilter),
				workaction.ExternalKindEQ("tpm_work_action"),
			)
		}
		q.WithPullRequest(func(prq *genent.PullRequestQuery) { prq.WithTickets() })
		q.WithTicket(func(tq *genent.TicketQuery) { tq.WithPullRequests() })
		q.WithLatestEvidence()
		q.WithObservations(workActionObservationDetails(sourceFilter))
		q.WithSourceInsights(workActionSourceInsightDetails(sourceFilter))
	}
}

func workActionObservationDetails(sourceFilter *string) func(*genent.WorkActionObservationQuery) {
	return func(q *genent.WorkActionObservationQuery) {
		if sourceFilter != nil {
			q.Where(
				workactionobservation.SourceSystemEQ("cubicle_analytics"),
				workactionobservation.SourceInstanceEQ(*sourceFilter),
				workactionobservation.ExternalKindEQ("tpm_work_action_observation"),
			)
		}
		q.WithLatestEvidence()
		q.Order(workactionobservation.ByObservedAt(entsql.OrderDesc()))
	}
}

func workActionSourceInsightDetails(sourceFilter *string) func(*genent.WorkInsightQuery) {
	return func(q *genent.WorkInsightQuery) {
		if sourceFilter != nil {
			q.Where(
				workinsight.SourceSystemEQ("cubicle_analytics"),
				workinsight.SourceInstanceEQ(*sourceFilter),
				workinsight.ExternalKindEQ("tpm_insight"),
			)
		}
		q.WithLatestEvidence()
		q.WithReviews(func(rq *genent.WorkInsightReviewQuery) {
			rq = applyWorkInsightReviewSourceFilter(rq, sourceFilter)
			rq.Order(
				workinsightreview.ByMeasurementEligible(entsql.OrderDesc()),
				workinsightreview.ByReviewedAt(entsql.OrderDesc()),
				workinsightreview.ByUpdatedAt(entsql.OrderDesc()),
			)
		})
		q.Order(workinsight.ByRankScore(entsql.OrderDesc()))
	}
}

func (r *queryResolver) hydrateWorkActionSourceInsightDetails(ctx context.Context, rows []*genent.WorkAction, sourceInstance *string) error {
	ids := map[int]bool{}
	keys := map[string]bool{}
	for _, action := range rows {
		for _, insight := range action.Edges.SourceInsights {
			if insight == nil {
				continue
			}
			if insight.ID > 0 {
				ids[insight.ID] = true
			}
			if insight.Key != "" {
				keys[insight.Key] = true
			}
		}
	}
	if len(ids) == 0 && len(keys) == 0 {
		return nil
	}
	orderedIDs := make([]int, 0, len(ids))
	for id := range ids {
		orderedIDs = append(orderedIDs, id)
	}
	orderedKeys := make([]string, 0, len(keys))
	for key := range keys {
		orderedKeys = append(orderedKeys, key)
	}
	predicates := []predicate.WorkInsight{}
	if len(orderedIDs) > 0 {
		predicates = append(predicates, workinsight.IDIn(orderedIDs...))
	}
	if len(orderedKeys) > 0 {
		predicates = append(predicates, workinsight.KeyIn(orderedKeys...))
	}
	detailQuery := r.EntClient.WorkInsight.Query().
		Where(workinsight.Or(predicates...))
	if sourceInstance != nil {
		detailQuery = detailQuery.Where(
			workinsight.SourceSystemEQ("cubicle_analytics"),
			workinsight.SourceInstanceEQ(*sourceInstance),
			workinsight.ExternalKindEQ("tpm_insight"),
		)
	}
	detailedRows, err := detailQuery.
		WithLatestEvidence().
		WithReviews(func(rq *genent.WorkInsightReviewQuery) {
			rq = applyWorkInsightReviewSourceFilter(rq, sourceInstance)
			rq.Order(
				workinsightreview.ByMeasurementEligible(entsql.OrderDesc()),
				workinsightreview.ByReviewedAt(entsql.OrderDesc()),
				workinsightreview.ByUpdatedAt(entsql.OrderDesc()),
			)
		}).
		All(ctx)
	if err != nil {
		return err
	}
	byID := make(map[int]*genent.WorkInsight, len(detailedRows))
	byKey := make(map[string]*genent.WorkInsight, len(detailedRows))
	for _, insight := range detailedRows {
		byID[insight.ID] = insight
		byKey[insight.Key] = insight
	}
	for _, action := range rows {
		for i, insight := range action.Edges.SourceInsights {
			if detailed := byID[insight.ID]; detailed != nil {
				action.Edges.SourceInsights[i] = detailed
				continue
			}
			if detailed := byKey[insight.Key]; detailed != nil {
				action.Edges.SourceInsights[i] = detailed
			}
		}
	}
	return nil
}
