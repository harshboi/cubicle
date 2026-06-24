package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workactionobservation"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/ent/workstream"
	"cubicle/services/ontology-service/ent/workstreamstandupsection"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkstreamStandupSectionModels(ctx context.Context, rowLimit int, workstreamKey *string, sectionKind *string, sourceInstance *string) ([]*model.WorkstreamStandupSection, error) {
	latestQuery := r.applyWorkstreamStandupSectionWorkstreamFilter(
		r.EntClient.WorkstreamStandupSection.Query().
			Order(
				workstreamstandupsection.ByGeneratedAt(entsql.OrderDesc()),
				workstreamstandupsection.BySectionRank(),
			),
		workstreamKey,
	)
	latestQuery = r.applyWorkstreamStandupSectionSourceFilter(latestQuery, sourceInstance)
	latest, err := latestQuery.First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	query := r.workstreamStandupSectionQueryWithEdges(sourceInstance)
	query = r.applyWorkstreamStandupSectionWorkstreamFilter(query, workstreamKey)
	query = r.applyWorkstreamStandupSectionSourceFilter(query, sourceInstance)
	if latest.WorkstreamHealthSnapshotID != 0 {
		query = query.Where(workstreamstandupsection.WorkstreamHealthSnapshotIDEQ(latest.WorkstreamHealthSnapshotID))
	} else if latest.GeneratedAt.IsZero() {
		query = query.Where(workstreamstandupsection.GeneratedAtIsNil())
	} else {
		query = query.Where(workstreamstandupsection.GeneratedAtEQ(latest.GeneratedAt))
	}
	if sectionKind != nil && *sectionKind != "" {
		value := workstreamstandupsection.SectionKind(*sectionKind)
		if err := workstreamstandupsection.SectionKindValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workstreamstandupsection.SectionKindEQ(value))
	}
	rows, err := query.
		Order(workstreamstandupsection.BySectionRank()).
		Limit(rowLimit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	actions := make([]*genent.WorkAction, 0, len(rows))
	for _, row := range rows {
		if row.Edges.WorkAction != nil {
			actions = append(actions, row.Edges.WorkAction)
		}
	}
	if err := r.hydrateWorkActionSourceInsightDetails(ctx, actions, sourceInstance); err != nil {
		return nil, err
	}
	out := make([]*model.WorkstreamStandupSection, 0, len(rows))
	for _, row := range rows {
		out = append(out, workstreamStandupSectionModel(row))
	}
	return out, nil
}

func (r *queryResolver) workstreamStandupSectionQueryWithEdges(sourceInstance *string) *genent.WorkstreamStandupSectionQuery {
	return r.EntClient.WorkstreamStandupSection.Query().
		WithLatestEvidence().
		WithWorkAction(func(q *genent.WorkActionQuery) {
			if sourceInstance != nil {
				q.Where(
					workaction.SourceSystemEQ("cubicle_analytics"),
					workaction.SourceInstanceEQ(*sourceInstance),
					workaction.ExternalKindEQ("tpm_work_action"),
				)
			}
			q.WithPullRequest(func(prq *genent.PullRequestQuery) {
				prq.WithTickets()
			})
			q.WithTicket(func(tq *genent.TicketQuery) {
				tq.WithPullRequests()
			})
			q.WithLatestEvidence()
			q.WithObservations(func(oq *genent.WorkActionObservationQuery) {
				if sourceInstance != nil {
					oq.Where(
						workactionobservation.SourceSystemEQ("cubicle_analytics"),
						workactionobservation.SourceInstanceEQ(*sourceInstance),
						workactionobservation.ExternalKindEQ("tpm_work_action_observation"),
					)
				}
				oq.WithLatestEvidence()
				oq.Order(workactionobservation.ByObservedAt(entsql.OrderDesc()))
			})
			q.WithSourceInsights(func(iq *genent.WorkInsightQuery) {
				if sourceInstance != nil {
					iq.Where(
						workinsight.SourceSystemEQ("cubicle_analytics"),
						workinsight.SourceInstanceEQ(*sourceInstance),
						workinsight.ExternalKindEQ("tpm_insight"),
					)
				}
				iq.WithLatestEvidence()
				iq.WithReviews(func(rq *genent.WorkInsightReviewQuery) {
					rq = applyWorkInsightReviewSourceFilter(rq, sourceInstance)
					rq.Order(
						workinsightreview.ByMeasurementEligible(entsql.OrderDesc()),
						workinsightreview.ByReviewedAt(entsql.OrderDesc()),
						workinsightreview.ByUpdatedAt(entsql.OrderDesc()),
					)
				})
				iq.Order(workinsight.ByRankScore(entsql.OrderDesc()))
			})
		})
}

func (r *queryResolver) applyWorkstreamStandupSectionSourceFilter(query *genent.WorkstreamStandupSectionQuery, sourceInstance *string) *genent.WorkstreamStandupSectionQuery {
	if sourceInstance != nil {
		query = query.Where(
			workstreamstandupsection.SourceSystemEQ("cubicle_analytics"),
			workstreamstandupsection.SourceInstanceEQ(*sourceInstance),
			workstreamstandupsection.ExternalKindEQ("tpm_workstream_standup_section"),
		)
	}
	return query
}

func (r *queryResolver) applyWorkstreamStandupSectionWorkstreamFilter(query *genent.WorkstreamStandupSectionQuery, workstreamKey *string) *genent.WorkstreamStandupSectionQuery {
	if workstreamKey != nil && *workstreamKey != "" {
		filterKey := strings.TrimSpace(*workstreamKey)
		filterKeys := []string{filterKey}
		if strings.HasPrefix(filterKey, "workstream:") {
			filterKeys = append(filterKeys, strings.TrimPrefix(filterKey, "workstream:"))
		} else {
			filterKeys = append(filterKeys, "workstream:"+filterKey)
		}
		query = query.Where(workstreamstandupsection.Or(
			workstreamstandupsection.WorkstreamKeyIn(filterKeys...),
			workstreamstandupsection.HasWorkstreamWith(workstream.KeyIn(filterKeys...)),
		))
	}
	return query
}
