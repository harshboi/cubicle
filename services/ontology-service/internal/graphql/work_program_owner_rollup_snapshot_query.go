package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workactionobservation"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/ent/workprogramownerrollupsnapshot"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkProgramOwnerRollupSnapshotModels(
	ctx context.Context,
	sourceFilter *string,
	workstreamKey *string,
	limit int,
) ([]*model.WorkProgramOwnerRollup, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, nil, workProgramRunMemberTableOwnerRollupSnapshots)
	if err != nil {
		return nil, err
	}
	if hasRunMembers {
		if len(memberIDs) == 0 {
			return []*model.WorkProgramOwnerRollup{}, nil
		}
		rows, err := r.applyWorkProgramOwnerRollupSnapshotFilters(
			r.EntClient.WorkProgramOwnerRollupSnapshot.Query(),
			sourceFilter,
			workstreamKey,
		).
			Where(workprogramownerrollupsnapshot.IDIn(memberIDs...)).
			Order(
				workprogramownerrollupsnapshot.ByMaxRiskScore(entsql.OrderDesc()),
				workprogramownerrollupsnapshot.ByItemCount(entsql.OrderDesc()),
				workprogramownerrollupsnapshot.ByOwnerKey(),
			).
			Limit(limit).
			All(ctx)
		if err != nil {
			return nil, err
		}
		itemKeys := []string{}
		for _, row := range rows {
			itemKeys = append(itemKeys, splitLineList(row.TopItemKeys)...)
		}
		itemsByKey, err := r.workProgramItemModelsByKey(ctx, itemKeys, sourceFilter)
		if err != nil {
			return nil, err
		}
		out := make([]*model.WorkProgramOwnerRollup, 0, len(rows))
		for _, row := range rows {
			out = append(out, workProgramOwnerRollupSnapshotModel(row, workProgramTopItemModels(row.TopItemKeys, itemsByKey)))
		}
		return out, nil
	}
	latestQuery := r.applyWorkProgramOwnerRollupSnapshotFilters(
		r.EntClient.WorkProgramOwnerRollupSnapshot.Query(),
		sourceFilter,
		workstreamKey,
	)
	latest, err := latestQuery.
		Order(
			workprogramownerrollupsnapshot.ByGeneratedAt(entsql.OrderDesc()),
			workprogramownerrollupsnapshot.ByRankScore(entsql.OrderDesc()),
			workprogramownerrollupsnapshot.ByUpdatedAt(entsql.OrderDesc()),
		).
		First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	runSource := sourceFilter
	if runSource == nil && strings.TrimSpace(latest.SourceInstance) != "" {
		runSource = optionalString(latest.SourceInstance)
	}
	runWorkstream := workstreamKey
	if runWorkstream == nil && strings.TrimSpace(latest.WorkstreamKey) != "" {
		runWorkstream = optionalString(latest.WorkstreamKey)
	}
	runQuery := r.applyWorkProgramOwnerRollupSnapshotFilters(
		r.EntClient.WorkProgramOwnerRollupSnapshot.Query(),
		runSource,
		runWorkstream,
	).Where(workprogramownerrollupsnapshot.GeneratedAtEQ(latest.GeneratedAt))

	rows, err := runQuery.
		Order(
			workprogramownerrollupsnapshot.ByMaxRiskScore(entsql.OrderDesc()),
			workprogramownerrollupsnapshot.ByItemCount(entsql.OrderDesc()),
			workprogramownerrollupsnapshot.ByOwnerKey(),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	itemKeys := []string{}
	for _, row := range rows {
		itemKeys = append(itemKeys, splitLineList(row.TopItemKeys)...)
	}
	itemsByKey, err := r.workProgramItemModelsByKey(ctx, itemKeys, runSource)
	if err != nil {
		return nil, err
	}

	out := make([]*model.WorkProgramOwnerRollup, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramOwnerRollupSnapshotModel(row, workProgramTopItemModels(row.TopItemKeys, itemsByKey)))
	}
	return out, nil
}

func (r *queryResolver) applyWorkProgramOwnerRollupSnapshotFilters(query *genent.WorkProgramOwnerRollupSnapshotQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramOwnerRollupSnapshotQuery {
	query = query.Where(
		workprogramownerrollupsnapshot.SourceSystemEQ("cubicle_analytics"),
		workprogramownerrollupsnapshot.ExternalKindEQ("tpm_work_program_owner_rollup_snapshot"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramownerrollupsnapshot.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramownerrollupsnapshot.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramOwnerRollupSnapshotModel(row *genent.WorkProgramOwnerRollupSnapshot, topItems []*model.WorkProgramItem) *model.WorkProgramOwnerRollup {
	aggregate := &workProgramOwnerAggregate{
		ownerKey:              row.OwnerKey,
		ownerSource:           row.OwnerSource,
		itemCount:             row.ItemCount,
		needsDecisionCount:    row.NeedsDecisionCount,
		validateSignalCount:   row.ValidateSignalCount,
		ciFailingCount:        row.CiFailingCount,
		waitingReviewCount:    row.WaitingReviewCount,
		sourceRepairCount:     row.SourceRepairCount,
		closureCandidateCount: row.ClosureCandidateCount,
		nowCount:              row.NowCount,
		highRiskCount:         row.HighRiskCount,
		maxRiskScore:          row.MaxRiskScore,
	}
	return &model.WorkProgramOwnerRollup{
		OwnerKey:              row.OwnerKey,
		OwnerSource:           optionalString(row.OwnerSource),
		ItemCount:             row.ItemCount,
		NeedsDecisionCount:    row.NeedsDecisionCount,
		ValidateSignalCount:   row.ValidateSignalCount,
		CiFailingCount:        row.CiFailingCount,
		WaitingReviewCount:    row.WaitingReviewCount,
		SourceRepairCount:     row.SourceRepairCount,
		ClosureCandidateCount: row.ClosureCandidateCount,
		NowCount:              row.NowCount,
		HighRiskCount:         row.HighRiskCount,
		MaxRiskScore:          row.MaxRiskScore,
		Badges:                workProgramOwnerBadges(aggregate),
		TopItems:              topItems,
	}
}

func workProgramTopItemModels(topItemKeys string, itemsByKey map[string]*model.WorkProgramItem) []*model.WorkProgramItem {
	keys := splitLineList(topItemKeys)
	out := make([]*model.WorkProgramItem, 0, len(keys))
	for _, key := range keys {
		if item := itemsByKey[key]; item != nil {
			out = append(out, item)
		}
	}
	return out
}

func (r *queryResolver) workProgramItemModelsByKey(ctx context.Context, keys []string, sourceFilter *string) (map[string]*model.WorkProgramItem, error) {
	keys = workProgramUniqueStrings(keys)
	if len(keys) == 0 {
		return map[string]*model.WorkProgramItem{}, nil
	}
	query := r.EntClient.WorkProgramItem.Query().
		WithLatestEvidence().
		WithWorkAction(func(q *genent.WorkActionQuery) {
			if sourceFilter != nil {
				q.Where(
					workaction.SourceSystemEQ("cubicle_analytics"),
					workaction.SourceInstanceEQ(*sourceFilter),
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
				if sourceFilter != nil {
					oq.Where(
						workactionobservation.SourceSystemEQ("cubicle_analytics"),
						workactionobservation.SourceInstanceEQ(*sourceFilter),
						workactionobservation.ExternalKindEQ("tpm_work_action_observation"),
					)
				}
				oq.WithLatestEvidence()
				oq.Order(workactionobservation.ByObservedAt(entsql.OrderDesc()))
			})
			q.WithSourceInsights(func(iq *genent.WorkInsightQuery) {
				if sourceFilter != nil {
					iq.Where(
						workinsight.SourceSystemEQ("cubicle_analytics"),
						workinsight.SourceInstanceEQ(*sourceFilter),
						workinsight.ExternalKindEQ("tpm_insight"),
					)
				}
				iq.WithLatestEvidence()
				iq.WithReviews(func(rq *genent.WorkInsightReviewQuery) {
					rq = applyWorkInsightReviewSourceFilter(rq, sourceFilter)
					rq.Order(
						workinsightreview.ByMeasurementEligible(entsql.OrderDesc()),
						workinsightreview.ByReviewedAt(entsql.OrderDesc()),
						workinsightreview.ByUpdatedAt(entsql.OrderDesc()),
					)
				})
				iq.Order(workinsight.ByRankScore(entsql.OrderDesc()))
			})
		}).
		Where(
			workprogramitem.SourceSystemEQ("cubicle_analytics"),
			workprogramitem.ExternalKindEQ("tpm_program_item"),
			workprogramitem.KeyIn(keys...),
		)
	if sourceFilter != nil {
		query = query.Where(workprogramitem.SourceInstanceEQ(*sourceFilter))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	actions := make([]*genent.WorkAction, 0, len(rows))
	out := make(map[string]*model.WorkProgramItem, len(rows))
	for _, row := range rows {
		if row.Edges.WorkAction != nil {
			actions = append(actions, row.Edges.WorkAction)
		}
		out[row.Key] = workProgramItemModel(row)
	}
	if err := r.hydrateWorkActionSourceInsightDetails(ctx, actions, sourceFilter); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Key] = workProgramItemModel(row)
	}
	return out, nil
}
