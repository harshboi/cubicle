package graphql

import (
	"context"
	"fmt"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workblockerimpact"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/ent/workstream"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) workProgramBlockerPacket(ctx context.Context, workstreamKey string, blockerState *string, limit *int, evidenceLimit *int, sourceInstance *string) (*model.WorkProgramBlockerPacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramBlockerPacket requires an Ent-backed ontology store")
	}
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil, fmt.Errorf("workProgramBlockerPacket requires workstreamKey")
	}
	sourceFilter, err := r.aggregateSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	state, err := normalizeWorkProgramBlockerPacketState(blockerState)
	if err != nil {
		return nil, err
	}
	rowLimit := boundedLimit(limit, 20, 100)
	evidenceRowLimit := boundedLimit(evidenceLimit, 50, 200)

	workstreamFilter := &workstreamKey
	runGeneratedAt, err := r.latestWorkProgramAutomationReadinessRunGeneratedAt(ctx, sourceFilter, workstreamFilter)
	if err != nil {
		return nil, err
	}
	items, err := r.workProgramItemRowsForSource(ctx, 1000, workstreamFilter, nil, nil, nil, sourceFilter)
	if err != nil {
		return nil, err
	}
	scope := workProgramRiskScopeFor(workstreamFilter, items)
	blockers, err := r.workProgramBlockerPacketBlockerRows(ctx, sourceFilter, scope, state, rowLimit)
	if err != nil {
		return nil, err
	}
	impacts, err := r.workProgramBlockerPacketImpactRows(ctx, sourceFilter, workstreamFilter, scope, state, rowLimit)
	if err != nil {
		return nil, err
	}
	forecasts, highRiskForecastCount, err := r.topWorkProgramForecastRowsAndCount(ctx, sourceFilter, scope, rowLimit)
	if err != nil {
		return nil, err
	}
	forecastReadiness, err := r.forecastReadinessModel(ctx, sourceFilter, "open")
	if err != nil {
		return nil, err
	}
	forecastReadiness = forecastReadinessForForecastRows(forecastReadiness, forecasts)
	evidencePredicate := workProgramBlockerPacketEvidenceNeedPredicate(workstreamFilter, blockers, impacts, forecasts)
	filteredEvidenceNeeds, evidenceNeedCount, err := r.latestWorkProgramEvidenceNeedModelsAndCountForPredicates(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  sourceFilter,
		workstreamKey: workstreamFilter,
		generatedAt:   runGeneratedAt,
	}, evidenceRowLimit, evidencePredicate)
	if err != nil {
		return nil, err
	}
	blockerModels := workBlockerModels(blockers)
	impactModels := workBlockerImpactModels(impacts)
	forecastModels := workItemForecastModels(forecasts, forecastReadiness)
	recommendedFocus := workProgramBlockerPacketFocus(blockerModels, impactModels, forecastModels, filteredEvidenceNeeds)

	return &model.WorkProgramBlockerPacket{
		SourceInstance:        sourceFilter,
		GeneratedAt:           optionalTimePtr(runGeneratedAt),
		WorkstreamKey:         workstreamKey,
		BlockerState:          state,
		BlockerCount:          len(blockerModels),
		ImpactCount:           len(impactModels),
		HighRiskForecastCount: highRiskForecastCount,
		EvidenceNeedCount:     evidenceNeedCount,
		HumanRequired:         workProgramBlockerPacketHumanRequired(len(blockerModels), len(impactModels), highRiskForecastCount, evidenceNeedCount),
		AutomationSummary:     workProgramBlockerPacketSummary(workstreamKey, state, len(blockerModels), len(impactModels), highRiskForecastCount, evidenceNeedCount, recommendedFocus),
		RecommendedFocus:      recommendedFocus,
		Blockers:              blockerModels,
		Impacts:               impactModels,
		Forecasts:             forecastModels,
		EvidenceNeeds:         filteredEvidenceNeeds,
	}, nil
}

func normalizeWorkProgramBlockerPacketState(blockerState *string) (string, error) {
	state := "open"
	if blockerState != nil {
		state = strings.TrimSpace(*blockerState)
	}
	if state == "" {
		state = "open"
	}
	switch state {
	case "all", "open":
		return state, nil
	default:
		value := workblocker.BlockerState(state)
		if err := workblocker.BlockerStateValidator(value); err != nil {
			return "", err
		}
		return state, nil
	}
}

func (r *queryResolver) workProgramBlockerPacketBlockerRows(ctx context.Context, sourceFilter *string, scope workProgramRiskScope, state string, limit int) ([]*genent.WorkBlocker, error) {
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
		).
		Order(
			workblocker.ByRankScore(entsql.OrderDesc()),
			workblocker.ByLastActivityAt(entsql.OrderDesc()),
			workblocker.ByUpdatedAt(entsql.OrderDesc()),
		).
		Limit(limit)
	query = applyWorkProgramBlockerStateFilter(query, state)
	if sourceFilter != nil {
		query = query.Where(workblocker.SourceInstanceEQ(*sourceFilter))
	}
	if scope.scoped {
		query = query.Where(workblocker.SubjectKeyIn(scope.subjectKeys...))
	}
	return query.All(ctx)
}

func applyWorkProgramBlockerStateFilter(query *genent.WorkBlockerQuery, state string) *genent.WorkBlockerQuery {
	switch state {
	case "all":
		return query
	case "open":
		return query.Where(workblocker.BlockerStateIn(
			workblocker.BlockerStateActive,
			workblocker.BlockerStateValidating,
		))
	default:
		return query.Where(workblocker.BlockerStateEQ(workblocker.BlockerState(state)))
	}
}

func (r *queryResolver) workProgramBlockerPacketImpactRows(ctx context.Context, sourceFilter *string, workstreamKey *string, scope workProgramRiskScope, state string, limit int) ([]*genent.WorkBlockerImpact, error) {
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
		).
		Order(
			workblockerimpact.ByImpactScore(entsql.OrderDesc()),
			workblockerimpact.ByRankScore(entsql.OrderDesc()),
			workblockerimpact.ByLastActivityAt(entsql.OrderDesc()),
		).
		Limit(limit)
	query = applyWorkProgramBlockerImpactStateFilter(query, state)
	if sourceFilter != nil {
		query = query.Where(workblockerimpact.SourceInstanceEQ(*sourceFilter))
	}
	query = applyWorkProgramBlockerPacketImpactScope(query, workstreamKey, scope)
	return query.All(ctx)
}

func applyWorkProgramBlockerImpactStateFilter(query *genent.WorkBlockerImpactQuery, state string) *genent.WorkBlockerImpactQuery {
	switch state {
	case "all":
		return query
	case "open":
		return query.Where(workblockerimpact.ImpactStateIn(
			workblockerimpact.ImpactStateActive,
			workblockerimpact.ImpactStateValidating,
		))
	default:
		return query.Where(workblockerimpact.ImpactStateEQ(workblockerimpact.ImpactState(state)))
	}
}

func applyWorkProgramBlockerPacketImpactScope(query *genent.WorkBlockerImpactQuery, workstreamKey *string, scope workProgramRiskScope) *genent.WorkBlockerImpactQuery {
	predicates := []predicate.WorkBlockerImpact{}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		predicates = append(predicates, workblockerimpact.And(
			workblockerimpact.AffectedKindEQ(workblockerimpact.AffectedKindWorkstream),
			workblockerimpact.AffectedKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...),
		))
	}
	if scope.scoped && len(scope.subjectKeys) > 0 {
		predicates = append(predicates, workblockerimpact.AffectedKeyIn(scope.subjectKeys...))
	}
	if len(predicates) == 0 {
		return query
	}
	return query.Where(workblockerimpact.Or(predicates...))
}

func workProgramBlockerPacketEvidenceNeedPredicate(workstreamKey *string, blockers []*genent.WorkBlocker, impacts []*genent.WorkBlockerImpact, forecasts []*genent.WorkItemForecast) predicate.WorkProgramEvidenceNeed {
	targetKeys := map[string]bool{}
	actionKeys := map[string]bool{}
	for _, blocker := range blockers {
		if blocker == nil {
			continue
		}
		if strings.TrimSpace(blocker.SubjectKey) != "" {
			targetKeys[blocker.SubjectKey] = true
		}
		if blocker.Edges.WorkAction != nil && strings.TrimSpace(blocker.Edges.WorkAction.Key) != "" {
			actionKeys[blocker.Edges.WorkAction.Key] = true
		}
	}
	for _, impact := range impacts {
		if impact == nil {
			continue
		}
		if strings.TrimSpace(impact.AffectedKey) != "" {
			targetKeys[impact.AffectedKey] = true
		}
		if strings.TrimSpace(impact.SubjectKey) != "" {
			targetKeys[impact.SubjectKey] = true
		}
		if impact.Edges.WorkAction != nil && strings.TrimSpace(impact.Edges.WorkAction.Key) != "" {
			actionKeys[impact.Edges.WorkAction.Key] = true
		}
	}
	for _, forecast := range forecasts {
		if forecast == nil {
			continue
		}
		if strings.TrimSpace(forecast.SubjectKey) != "" {
			targetKeys[forecast.SubjectKey] = true
		}
		if forecast.Edges.WorkAction != nil && strings.TrimSpace(forecast.Edges.WorkAction.Key) != "" {
			actionKeys[forecast.Edges.WorkAction.Key] = true
		}
	}
	predicates := []predicate.WorkProgramEvidenceNeed{
		workprogramevidenceneed.And(
			workprogramevidenceneed.GateKeyIn("blocker_clearance", "dependency_pressure"),
			workProgramPacketGlobalEvidenceTargetPredicate(workstreamKey),
		),
	}
	if keys := sortedKeys(targetKeys); len(keys) > 0 {
		predicates = append(predicates, workprogramevidenceneed.TargetKeyIn(keys...))
	}
	if keys := sortedKeys(actionKeys); len(keys) > 0 {
		predicates = append(predicates, workprogramevidenceneed.ActionKeyIn(keys...))
	}
	return workprogramevidenceneed.Or(predicates...)
}

func workProgramBlockerPacketEvidenceNeeds(blockers []*genent.WorkBlocker, impacts []*genent.WorkBlockerImpact, forecasts []*genent.WorkItemForecast, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed, limit int) []*model.WorkProgramAutomationEvidenceNeed {
	targetKeys := map[string]bool{}
	actionKeys := map[string]bool{}
	for _, blocker := range blockers {
		if blocker == nil {
			continue
		}
		targetKeys[blocker.SubjectKey] = true
		if blocker.Edges.WorkAction != nil {
			actionKeys[blocker.Edges.WorkAction.Key] = true
		}
	}
	for _, impact := range impacts {
		if impact == nil {
			continue
		}
		targetKeys[impact.AffectedKey] = true
		targetKeys[impact.SubjectKey] = true
		if impact.Edges.WorkAction != nil {
			actionKeys[impact.Edges.WorkAction.Key] = true
		}
	}
	for _, forecast := range forecasts {
		if forecast == nil {
			continue
		}
		targetKeys[forecast.SubjectKey] = true
		if forecast.Edges.WorkAction != nil {
			actionKeys[forecast.Edges.WorkAction.Key] = true
		}
	}
	out := make([]*model.WorkProgramAutomationEvidenceNeed, 0, min(limit, len(evidenceNeeds)))
	for _, need := range evidenceNeeds {
		if need == nil {
			continue
		}
		if workProgramBlockerPacketEvidenceNeedMatches(need, targetKeys, actionKeys) {
			out = append(out, need)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func workProgramBlockerPacketEvidenceNeedMatches(need *model.WorkProgramAutomationEvidenceNeed, targetKeys map[string]bool, actionKeys map[string]bool) bool {
	switch need.GateKey {
	case "blocker_clearance", "dependency_pressure":
		return true
	}
	if need.TargetKey != nil && targetKeys[*need.TargetKey] {
		return true
	}
	if need.ActionKey != nil && actionKeys[*need.ActionKey] {
		return true
	}
	return false
}

func workProgramBlockerPacketFocus(blockers []*model.WorkBlocker, impacts []*model.WorkBlockerImpact, forecasts []*model.WorkItemForecast, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed) *string {
	for _, blocker := range blockers {
		if blocker != nil && blocker.RecommendedAction != nil && strings.TrimSpace(*blocker.RecommendedAction) != "" {
			value := strings.TrimSpace(*blocker.RecommendedAction)
			return &value
		}
	}
	for _, impact := range impacts {
		if impact != nil && impact.RecommendedAction != nil && strings.TrimSpace(*impact.RecommendedAction) != "" {
			value := strings.TrimSpace(*impact.RecommendedAction)
			return &value
		}
	}
	for _, forecast := range forecasts {
		if forecast != nil && strings.TrimSpace(forecast.RecommendedAction) != "" {
			value := strings.TrimSpace(forecast.RecommendedAction)
			return &value
		}
	}
	for _, need := range evidenceNeeds {
		if need != nil && strings.TrimSpace(need.RecommendedAction) != "" {
			value := strings.TrimSpace(need.RecommendedAction)
			return &value
		}
	}
	return nil
}

func workProgramBlockerPacketHumanRequired(blockerCount int, impactCount int, forecastCount int, evidenceNeedCount int) bool {
	return blockerCount > 0 || impactCount > 0 || forecastCount > 0 || evidenceNeedCount > 0
}

func workProgramBlockerPacketSummary(workstreamKey string, state string, blockerCount int, impactCount int, forecastCount int, evidenceNeedCount int, recommendedFocus *string) string {
	summary := fmt.Sprintf("%s has %d %s blocker(s), %d blocker impact(s), %d high-risk forecast(s), and %d evidence need(s).", workstreamKey, blockerCount, state, impactCount, forecastCount, evidenceNeedCount)
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}
