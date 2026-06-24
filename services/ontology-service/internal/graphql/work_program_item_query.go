package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workactionobservation"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workblockerimpact"
	"cubicle/services/ontology-service/ent/workdependencyedge"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/ent/workitemforecast"
	"cubicle/services/ontology-service/ent/workownerloadsnapshot"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/ent/workstream"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

type workProgramExternalSignals struct {
	blockerCount               int
	activeBlockerCount         int
	validatingBlockerCount     int
	blockerImpactCount         int
	activeBlockerImpactCount   int
	dependencyEdgeCount        int
	blockingDependencyCount    int
	needsActionDependencyCount int
	topBlockers                []*genent.WorkBlocker
	topBlockerImpacts          []*genent.WorkBlockerImpact
	topDependencies            []*genent.WorkDependencyEdge
	topForecasts               []*genent.WorkItemForecast
	ownerLoadSnapshots         []*model.WorkOwnerLoadSnapshot
}

type workProgramRiskScope struct {
	scoped         bool
	subjectKeys    []string
	dependencyKeys []string
	workstreamIDs  []int
}

func (r *queryResolver) workProgramItemRowsForSource(
	ctx context.Context,
	limit int,
	workstreamKey *string,
	programStatus *string,
	tpmBucket *string,
	ownerKey *string,
	sourceFilter *string,
) ([]*genent.WorkProgramItem, error) {
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
		).
		Order(
			workprogramitem.ByRiskScore(entsql.OrderDesc()),
			workprogramitem.ByLastActivityAt(entsql.OrderDesc()),
			workprogramitem.ByUpdatedAt(entsql.OrderDesc()),
		).
		Limit(limit)
	if sourceFilter != nil {
		query = query.Where(workprogramitem.SourceInstanceEQ(*sourceFilter))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		filterKeys := workProgramWorkstreamFilterKeys(*workstreamKey)
		query = query.Where(workprogramitem.WorkstreamKeyIn(filterKeys...))
	}
	if programStatus != nil && strings.TrimSpace(*programStatus) != "" && strings.TrimSpace(*programStatus) != "all" {
		value := workprogramitem.ProgramStatus(strings.TrimSpace(*programStatus))
		if err := workprogramitem.ProgramStatusValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workprogramitem.ProgramStatusEQ(value))
	}
	if tpmBucket != nil && strings.TrimSpace(*tpmBucket) != "" && strings.TrimSpace(*tpmBucket) != "all" {
		value := workprogramitem.TpmBucket(strings.TrimSpace(*tpmBucket))
		if err := workprogramitem.TpmBucketValidator(value); err != nil {
			return nil, err
		}
		query = query.Where(workprogramitem.TpmBucketEQ(value))
	}
	if ownerKey != nil && strings.TrimSpace(*ownerKey) != "" {
		query = query.Where(workprogramitem.OwnerKeyEQ(strings.TrimSpace(*ownerKey)))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	actions := make([]*genent.WorkAction, 0, len(rows))
	for _, row := range rows {
		if row.Edges.WorkAction != nil {
			actions = append(actions, row.Edges.WorkAction)
		}
	}
	if err := r.hydrateWorkActionSourceInsightDetails(ctx, actions, sourceFilter); err != nil {
		return nil, err
	}
	return rows, nil
}

func workProgramWorkstreamFilterKeys(value string) []string {
	filterKey := strings.TrimSpace(value)
	filterKeys := []string{filterKey}
	if strings.HasPrefix(filterKey, "workstream:") {
		filterKeys = append(filterKeys, strings.TrimPrefix(filterKey, "workstream:"))
	} else {
		filterKeys = append(filterKeys, "workstream:"+filterKey)
	}
	return filterKeys
}

func (r *queryResolver) workProgramExternalSignals(ctx context.Context, sourceFilter *string, workstreamKey *string, rows []*genent.WorkProgramItem) (workProgramExternalSignals, error) {
	var out workProgramExternalSignals
	scope := workProgramRiskScopeFor(workstreamKey, rows)
	countBlockers := func(extra ...func(*genent.WorkBlockerQuery) *genent.WorkBlockerQuery) (int, error) {
		query := r.EntClient.WorkBlocker.Query().
			Where(
				workblocker.SourceSystemEQ("cubicle_analytics"),
				workblocker.ExternalKindEQ("tpm_work_blocker"),
			)
		if sourceFilter != nil {
			query = query.Where(workblocker.SourceInstanceEQ(*sourceFilter))
		}
		if scope.scoped {
			if len(scope.subjectKeys) == 0 {
				return 0, nil
			}
			query = query.Where(workblocker.SubjectKeyIn(scope.subjectKeys...))
		}
		for _, apply := range extra {
			query = apply(query)
		}
		return query.Count(ctx)
	}
	countImpacts := func(extra ...func(*genent.WorkBlockerImpactQuery) *genent.WorkBlockerImpactQuery) (int, error) {
		query := r.EntClient.WorkBlockerImpact.Query().
			Where(
				workblockerimpact.SourceSystemEQ("cubicle_analytics"),
				workblockerimpact.ExternalKindEQ("tpm_work_blocker_impact"),
			)
		if sourceFilter != nil {
			query = query.Where(workblockerimpact.SourceInstanceEQ(*sourceFilter))
		}
		if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
			query = query.Where(
				workblockerimpact.AffectedKindEQ(workblockerimpact.AffectedKindWorkstream),
				workblockerimpact.AffectedKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...),
			)
		}
		for _, apply := range extra {
			query = apply(query)
		}
		return query.Count(ctx)
	}
	countDependencies := func(extra ...func(*genent.WorkDependencyEdgeQuery) *genent.WorkDependencyEdgeQuery) (int, error) {
		query := r.EntClient.WorkDependencyEdge.Query().
			Where(
				workdependencyedge.SourceSystemEQ("cubicle_analytics"),
				workdependencyedge.ExternalKindEQ("tpm_work_dependency_edge"),
			)
		if sourceFilter != nil {
			query = query.Where(workdependencyedge.SourceInstanceEQ(*sourceFilter))
		}
		if scope.scoped {
			if len(scope.dependencyKeys) == 0 && len(scope.workstreamIDs) == 0 {
				return 0, nil
			}
			query = query.Where(workProgramDependencyScopePredicate(scope))
		}
		for _, apply := range extra {
			query = apply(query)
		}
		return query.Count(ctx)
	}

	var err error
	if out.blockerCount, err = countBlockers(); err != nil {
		return out, err
	}
	if out.activeBlockerCount, err = countBlockers(func(q *genent.WorkBlockerQuery) *genent.WorkBlockerQuery {
		return q.Where(workblocker.BlockerStateEQ(workblocker.BlockerStateActive))
	}); err != nil {
		return out, err
	}
	if out.validatingBlockerCount, err = countBlockers(func(q *genent.WorkBlockerQuery) *genent.WorkBlockerQuery {
		return q.Where(workblocker.BlockerStateEQ(workblocker.BlockerStateValidating))
	}); err != nil {
		return out, err
	}
	if out.blockerImpactCount, err = countImpacts(); err != nil {
		return out, err
	}
	if out.activeBlockerImpactCount, err = countImpacts(func(q *genent.WorkBlockerImpactQuery) *genent.WorkBlockerImpactQuery {
		return q.Where(workblockerimpact.ImpactStateEQ(workblockerimpact.ImpactStateActive))
	}); err != nil {
		return out, err
	}
	if out.dependencyEdgeCount, err = countDependencies(); err != nil {
		return out, err
	}
	if out.blockingDependencyCount, err = countDependencies(func(q *genent.WorkDependencyEdgeQuery) *genent.WorkDependencyEdgeQuery {
		return q.Where(workdependencyedge.EdgeKindEQ(workdependencyedge.EdgeKindBlockedBy))
	}); err != nil {
		return out, err
	}
	if out.needsActionDependencyCount, err = countDependencies(func(q *genent.WorkDependencyEdgeQuery) *genent.WorkDependencyEdgeQuery {
		return q.Where(workdependencyedge.EdgeKindEQ(workdependencyedge.EdgeKindNeedsAction))
	}); err != nil {
		return out, err
	}
	if out.topBlockers, err = r.topWorkProgramBlockerRows(ctx, sourceFilter, scope, 5); err != nil {
		return out, err
	}
	if out.topBlockerImpacts, err = r.topWorkProgramBlockerImpactRows(ctx, sourceFilter, workstreamKey, 5); err != nil {
		return out, err
	}
	if out.topDependencies, err = r.topWorkProgramDependencyRows(ctx, sourceFilter, scope, 5); err != nil {
		return out, err
	}
	if out.topForecasts, err = r.topWorkProgramForecastRows(ctx, sourceFilter, scope, 5); err != nil {
		return out, err
	}
	if out.ownerLoadSnapshots, err = r.latestWorkProgramOwnerLoadModels(ctx, sourceFilter, workstreamKey, 100); err != nil {
		return out, err
	}
	return out, nil
}

func (r *queryResolver) latestWorkProgramOwnerLoadModels(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkOwnerLoadSnapshot, error) {
	rowLimit := limit
	if rowLimit <= 0 {
		rowLimit = 100
	}
	latestQuery := r.applyOwnerLoadFilters(
		r.EntClient.WorkOwnerLoadSnapshot.Query().
			Order(
				workownerloadsnapshot.ByGeneratedAt(entsql.OrderDesc()),
				workownerloadsnapshot.ByActionCount(entsql.OrderDesc()),
				workownerloadsnapshot.ByMaxPriorityScore(entsql.OrderDesc()),
			),
		workstreamKey,
		nil,
		sourceFilter,
	)
	latest, err := latestQuery.First(ctx)
	if genent.IsNotFound(err) {
		return []*model.WorkOwnerLoadSnapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	query := r.applyOwnerLoadFilters(
		r.EntClient.WorkOwnerLoadSnapshot.Query().
			WithPerson().
			WithLatestEvidence(),
		workstreamKey,
		nil,
		sourceFilter,
	)
	if latest.SourceInstance != "" {
		query = query.Where(workownerloadsnapshot.SourceInstanceEQ(latest.SourceInstance))
	}
	if runPrefix := ownerLoadRunPrefix(latest.ExternalID, latest.OwnerKey); runPrefix != "" {
		query = query.Where(workownerloadsnapshot.ExternalIDHasPrefix(runPrefix + ":"))
	} else if latest.GeneratedAt.IsZero() {
		query = query.Where(workownerloadsnapshot.GeneratedAtIsNil())
	} else {
		query = query.Where(workownerloadsnapshot.GeneratedAtEQ(latest.GeneratedAt))
	}
	rows, err := query.
		Order(
			workownerloadsnapshot.ByActionCount(entsql.OrderDesc()),
			workownerloadsnapshot.ByMaxPriorityScore(entsql.OrderDesc()),
			workownerloadsnapshot.ByOwnerKey(),
		).
		Limit(rowLimit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return r.ownerLoadSnapshotModels(ctx, rows)
}

func workProgramDependencyScopePredicate(scope workProgramRiskScope) predicate.WorkDependencyEdge {
	predicates := []predicate.WorkDependencyEdge{}
	if len(scope.dependencyKeys) > 0 {
		predicates = append(predicates,
			workdependencyedge.FromKeyIn(scope.dependencyKeys...),
			workdependencyedge.ToKeyIn(scope.dependencyKeys...),
		)
	}
	if len(scope.workstreamIDs) > 0 {
		predicates = append(predicates, workdependencyedge.WorkstreamIDIn(scope.workstreamIDs...))
	}
	return workdependencyedge.Or(predicates...)
}

func workProgramRiskScopeFor(workstreamKey *string, rows []*genent.WorkProgramItem) workProgramRiskScope {
	if workstreamKey == nil || strings.TrimSpace(*workstreamKey) == "" {
		return workProgramRiskScope{}
	}
	subjectSet := map[string]bool{}
	workstreamIDSet := map[int]bool{}
	for _, row := range rows {
		if strings.TrimSpace(row.SubjectKey) != "" {
			subjectSet[row.SubjectKey] = true
		}
		if row.WorkstreamID > 0 {
			workstreamIDSet[row.WorkstreamID] = true
		}
	}
	subjectKeys := make([]string, 0, len(subjectSet))
	for key := range subjectSet {
		subjectKeys = append(subjectKeys, key)
	}
	workstreamIDs := make([]int, 0, len(workstreamIDSet))
	for id := range workstreamIDSet {
		workstreamIDs = append(workstreamIDs, id)
	}
	dependencySet := map[string]bool{}
	for _, key := range subjectKeys {
		dependencySet[key] = true
	}
	for _, key := range workProgramWorkstreamFilterKeys(*workstreamKey) {
		dependencySet[key] = true
	}
	dependencyKeys := make([]string, 0, len(dependencySet))
	for key := range dependencySet {
		dependencyKeys = append(dependencyKeys, key)
	}
	return workProgramRiskScope{scoped: true, subjectKeys: subjectKeys, dependencyKeys: dependencyKeys, workstreamIDs: workstreamIDs}
}

func (r *queryResolver) topWorkProgramBlockerRows(ctx context.Context, sourceFilter *string, scope workProgramRiskScope, limit int) ([]*genent.WorkBlocker, error) {
	if scope.scoped && len(scope.subjectKeys) == 0 {
		return []*genent.WorkBlocker{}, nil
	}
	query := r.EntClient.WorkBlocker.Query().
		WithPullRequest().
		WithTicket()
	if sourceFilter != nil {
		query = query.
			WithWorkAction(func(q *genent.WorkActionQuery) {
				q.Where(workaction.SourceInstanceEQ(*sourceFilter))
			}).
			WithWorkInsight(func(q *genent.WorkInsightQuery) {
				q.Where(workinsight.SourceInstanceEQ(*sourceFilter))
			})
	} else {
		query = query.
			WithWorkAction().
			WithWorkInsight()
	}
	query = query.
		WithLatestEvidence().
		Where(
			workblocker.SourceSystemEQ("cubicle_analytics"),
			workblocker.ExternalKindEQ("tpm_work_blocker"),
			workblocker.BlockerStateIn(
				workblocker.BlockerStateActive,
				workblocker.BlockerStateValidating,
			),
		).
		Order(
			workblocker.ByRankScore(entsql.OrderDesc()),
			workblocker.ByLastActivityAt(entsql.OrderDesc()),
			workblocker.ByUpdatedAt(entsql.OrderDesc()),
		).
		Limit(limit)
	if sourceFilter != nil {
		query = query.Where(workblocker.SourceInstanceEQ(*sourceFilter))
	}
	if scope.scoped {
		query = query.Where(workblocker.SubjectKeyIn(scope.subjectKeys...))
	}
	return query.All(ctx)
}

func (r *queryResolver) topWorkProgramBlockerImpactRows(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*genent.WorkBlockerImpact, error) {
	query := r.EntClient.WorkBlockerImpact.Query()
	if sourceFilter != nil {
		query = query.
			WithWorkBlocker(func(q *genent.WorkBlockerQuery) {
				q.Where(workblocker.SourceInstanceEQ(*sourceFilter))
			}).
			WithWorkAction(func(q *genent.WorkActionQuery) {
				q.Where(workaction.SourceInstanceEQ(*sourceFilter))
			}).
			WithWorkstream(func(q *genent.WorkstreamQuery) {
				q.Where(workstream.SourceInstanceEQ(*sourceFilter))
			})
	} else {
		query = query.
			WithWorkBlocker().
			WithWorkAction().
			WithWorkstream()
	}
	query = query.
		WithPullRequest().
		WithTicket().
		WithLatestEvidence().
		Where(
			workblockerimpact.SourceSystemEQ("cubicle_analytics"),
			workblockerimpact.ExternalKindEQ("tpm_work_blocker_impact"),
			workblockerimpact.ImpactStateIn(
				workblockerimpact.ImpactStateActive,
				workblockerimpact.ImpactStateValidating,
			),
		).
		Order(
			workblockerimpact.ByImpactScore(entsql.OrderDesc()),
			workblockerimpact.ByRankScore(entsql.OrderDesc()),
			workblockerimpact.ByLastActivityAt(entsql.OrderDesc()),
		).
		Limit(limit)
	if sourceFilter != nil {
		query = query.Where(workblockerimpact.SourceInstanceEQ(*sourceFilter))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(
			workblockerimpact.AffectedKindEQ(workblockerimpact.AffectedKindWorkstream),
			workblockerimpact.AffectedKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...),
		)
	}
	return query.All(ctx)
}

func (r *queryResolver) topWorkProgramDependencyRows(ctx context.Context, sourceFilter *string, scope workProgramRiskScope, limit int) ([]*genent.WorkDependencyEdge, error) {
	if scope.scoped && len(scope.dependencyKeys) == 0 && len(scope.workstreamIDs) == 0 {
		return []*genent.WorkDependencyEdge{}, nil
	}
	query := r.EntClient.WorkDependencyEdge.Query().
		WithLatestEvidence().
		Where(
			workdependencyedge.SourceSystemEQ("cubicle_analytics"),
			workdependencyedge.ExternalKindEQ("tpm_work_dependency_edge"),
			workdependencyedge.EdgeKindIn(
				workdependencyedge.EdgeKindBlockedBy,
				workdependencyedge.EdgeKindNeedsAction,
			),
		).
		Order(
			workdependencyedge.ByRankScore(entsql.OrderDesc()),
			workdependencyedge.ByLastActivityAt(entsql.OrderDesc()),
			workdependencyedge.ByUpdatedAt(entsql.OrderDesc()),
		).
		Limit(limit)
	if sourceFilter != nil {
		query = query.Where(workdependencyedge.SourceInstanceEQ(*sourceFilter))
	}
	if scope.scoped {
		query = query.Where(workProgramDependencyScopePredicate(scope))
	}
	return query.All(ctx)
}

func (r *queryResolver) topWorkProgramForecastRows(ctx context.Context, sourceFilter *string, scope workProgramRiskScope, limit int) ([]*genent.WorkItemForecast, error) {
	rows, _, err := r.topWorkProgramForecastRowsAndCount(ctx, sourceFilter, scope, limit)
	return rows, err
}

func (r *queryResolver) topWorkProgramForecastRowsAndCount(ctx context.Context, sourceFilter *string, scope workProgramRiskScope, limit int) ([]*genent.WorkItemForecast, int, error) {
	if sourceFilter == nil || strings.TrimSpace(*sourceFilter) == "" {
		return []*genent.WorkItemForecast{}, 0, nil
	}
	if scope.scoped && len(scope.subjectKeys) == 0 {
		return []*genent.WorkItemForecast{}, 0, nil
	}
	source := strings.TrimSpace(*sourceFilter)
	buildQuery := func() *genent.WorkItemForecastQuery {
		query := r.EntClient.WorkItemForecast.Query().
			Where(
				workitemforecast.SourceSystemEQ("cubicle_analytics"),
				workitemforecast.SourceInstanceEQ(source),
				workitemforecast.ExternalKindIn("tpm_pr_forecast", "tpm_work_item_forecast"),
				workitemforecast.SubjectStateEQ("open"),
				workitemforecast.RiskBandIn(workitemforecast.RiskBandCritical, workitemforecast.RiskBandHigh),
			)
		if scope.scoped {
			query = query.Where(workitemforecast.SubjectKeyIn(scope.subjectKeys...))
		}
		return query
	}
	latest, err := buildQuery().
		Where(workitemforecast.ForecastedAtNotNil()).
		Order(
			workitemforecast.ByForecastedAt(entsql.OrderDesc()),
			workitemforecast.ByUpdatedAt(entsql.OrderDesc()),
		).
		First(ctx)
	if err != nil && !genent.IsNotFound(err) {
		return nil, 0, err
	}
	countRows, err := buildQuery().
		Order(
			workitemforecast.ByForecastedAt(entsql.OrderDesc()),
			workitemforecast.ByRiskScore(entsql.OrderDesc()),
			workitemforecast.ByOverdueDays(entsql.OrderDesc()),
			workitemforecast.ByUpdatedAt(entsql.OrderDesc()),
		).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	countRows = currentWorkProgramForecastRunRows(countRows, latest)
	highRiskForecastCount := distinctWorkItemForecastLeadCount(countRows)
	fetchLimit := limit * 5
	if fetchLimit < 25 {
		fetchLimit = 25
	}
	rows, err := buildQuery().
		WithPullRequest(func(q *genent.PullRequestQuery) {
			q.WithTickets()
		}).
		WithTicket(func(q *genent.TicketQuery) {
			q.WithPullRequests()
		}).
		WithWorkAction(workActionDetails(sourceFilter)).
		WithLatestEvidence().
		Order(
			workitemforecast.ByForecastedAt(entsql.OrderDesc()),
			workitemforecast.ByRiskScore(entsql.OrderDesc()),
			workitemforecast.ByOverdueDays(entsql.OrderDesc()),
			workitemforecast.ByUpdatedAt(entsql.OrderDesc()),
		).
		Limit(fetchLimit).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows = currentWorkProgramForecastRunRows(rows, latest)
	out := make([]*genent.WorkItemForecast, 0, min(limit, len(rows)))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := workItemForecastLeadKey(row)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, row)
		if len(out) >= limit {
			break
		}
	}
	return out, highRiskForecastCount, nil
}

func currentWorkProgramForecastRunRows(rows []*genent.WorkItemForecast, latest *genent.WorkItemForecast) []*genent.WorkItemForecast {
	if latest == nil || latest.ForecastedAt.IsZero() {
		return rows
	}
	latestForecastedAt := latest.ForecastedAt
	currentRunRows := rows[:0]
	for _, row := range rows {
		if !row.ForecastedAt.IsZero() && row.ForecastedAt.UTC().Equal(latestForecastedAt.UTC()) {
			currentRunRows = append(currentRunRows, row)
		}
	}
	return currentRunRows
}

func distinctWorkItemForecastLeadCount(rows []*genent.WorkItemForecast) int {
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		key := workItemForecastLeadKey(row)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
	}
	return len(seen)
}

func workItemForecastLeadKey(row *genent.WorkItemForecast) string {
	if row == nil {
		return ""
	}
	return row.ForecastKind.String() + "\x00" + row.SubjectKind.String() + "\x00" + row.SubjectKey
}
