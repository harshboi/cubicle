package graphql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workdependencyedge"
	"cubicle/services/ontology-service/ent/workdependencyendpoint"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/ent/workprogramrun"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) workProgramGraphContext(
	ctx context.Context,
	workstreamKey string,
	itemLimit *int,
	actionLimit *int,
	edgeLimit *int,
	insightLimit *int,
	forecastLimit *int,
	evidenceLimit *int,
	traversalDepth *int,
	runKey *string,
	generatedAt *string,
	sourceInstance *string,
) (*model.WorkProgramGraphContext, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramGraphContext requires an Ent-backed ontology store")
	}
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil, fmt.Errorf("workProgramGraphContext requires workstreamKey")
	}
	sourceFilter, err := optionalSourceInstanceArgument(sourceInstance, "sourceInstance")
	if err != nil {
		return nil, err
	}
	requestedRunKey, err := optionalSourceInstanceArgument(runKey, "runKey")
	if err != nil {
		return nil, err
	}
	requestedGeneratedAt, err := workProgramGraphContextGeneratedAtArgument(generatedAt)
	if err != nil {
		return nil, err
	}

	itemRowLimit := boundedLimit(itemLimit, 20, 100)
	actionRowLimit := boundedLimit(actionLimit, 20, 100)
	edgeRowLimit := boundedLimit(edgeLimit, 50, 200)
	insightRowLimit := boundedLimit(insightLimit, 20, 100)
	forecastRowLimit := boundedLimit(forecastLimit, 20, 100)
	evidenceRowLimit := boundedLimit(evidenceLimit, 50, 200)
	depth := workProgramGraphContextDepth(traversalDepth)
	workstreamFilter := &workstreamKey
	run, err := r.workProgramGraphContextRunAnchor(ctx, sourceFilter, workstreamFilter, requestedRunKey, requestedGeneratedAt)
	if err != nil {
		return nil, err
	}
	if sourceFilter == nil && run != nil && strings.TrimSpace(run.SourceInstance) != "" {
		source := strings.TrimSpace(run.SourceInstance)
		sourceFilter = &source
	}
	if sourceFilter == nil {
		sourceFilter, err = r.aggregateSourceInstance(ctx, nil)
		if err != nil {
			return nil, err
		}
	}
	if sourceFilter == nil {
		return nil, fmt.Errorf("workProgramGraphContext requires sourceInstance, runKey/generatedAt, or persisted analytics source")
	}
	if run == nil {
		run, err = r.workProgramGraphContextRunAnchor(ctx, sourceFilter, workstreamFilter, requestedRunKey, requestedGeneratedAt)
		if err != nil {
			return nil, err
		}
	}
	scopeMode := workProgramGraphContextScopeMode(sourceInstance, requestedRunKey, requestedGeneratedAt, run)
	selectedRunKey := (*string)(nil)
	if run != nil {
		selectedRunKey = optionalString(run.Key)
	}
	runScope, err := r.workProgramGraphContextRunScope(ctx, run)
	if err != nil {
		return nil, err
	}

	items, err := r.workProgramItemRowsForSourceWithRunMembers(ctx, itemRowLimit, workstreamFilter, nil, nil, nil, sourceFilter, runScope.itemIDs, runScope.itemScoped)
	if err != nil {
		return nil, err
	}
	scope := workProgramRiskScopeFor(workstreamFilter, items)
	actions := workProgramGraphContextActions(items, actionRowLimit)
	actionClaimPolicies, err := r.workActionResponsibilityClaimPolicies(ctx, sourceFilter, actions, workActionClaimPolicy{})
	if err != nil {
		return nil, err
	}
	itemClaimPolicies, err := r.workProgramItemResponsibilityClaimPolicies(ctx, sourceFilter, items, workActionClaimPolicy{})
	if err != nil {
		return nil, err
	}

	dependencyRows, reachedKeys, err := r.workProgramGraphContextDependencyRows(ctx, sourceFilter, scope, edgeRowLimit, depth, runScope.dependencyIDs, runScope.dependencyScoped)
	if err != nil {
		return nil, err
	}
	subjectKeys := workProgramGraphContextSubjectKeys(workstreamKey, items, actions, dependencyRows, reachedKeys)
	insightRows, err := r.workProgramGraphContextInsightRows(ctx, sourceFilter, subjectKeys, insightRowLimit, runScope.insightIDs, runScope.insightScoped)
	if err != nil {
		return nil, err
	}

	readiness, err := r.forecastReadinessModel(ctx, sourceFilter, "open")
	if err != nil {
		return nil, err
	}
	forecastRows, _, err := r.topWorkProgramForecastRowsAndCountWithRunMembers(ctx, sourceFilter, scope, forecastRowLimit, runScope.forecastIDs, runScope.forecastScoped)
	if err != nil {
		return nil, err
	}
	readiness = forecastReadinessForForecastRows(readiness, forecastRows)
	forecastModels := workItemForecastModels(forecastRows, readiness)

	guardrailPacket, err := r.workProgramGuardrailPacket(ctx, workstreamKey, nil, &evidenceRowLimit, sourceFilter)
	if err != nil {
		return nil, err
	}
	sourceCoveragePacket, err := r.workProgramSourceCoveragePacket(ctx, workstreamKey, nil, &evidenceRowLimit, sourceFilter)
	if err != nil {
		return nil, err
	}
	forecastPacket, err := r.workProgramForecastPacket(ctx, workstreamKey, &forecastRowLimit, &evidenceRowLimit, sourceFilter)
	if err != nil {
		return nil, err
	}

	itemModels := workProgramGraphContextItemModelsWithClaimPolicies(items, itemClaimPolicies, workActionClaimPolicy{})
	actionModels := make([]*model.WorkAction, 0, len(actions))
	for _, action := range actions {
		actionModels = append(actionModels, workActionModelWithClaimPolicy(action, workActionClaimPolicyForRow(action.Key, actionClaimPolicies, workActionClaimPolicy{})))
	}
	dependencyModels := workDependencyEdgeModels(dependencyRows)
	insightModels := workInsightSummaryModels(insightRows)
	reachedSubjectKeys := workProgramGraphContextReachedSubjectKeys(subjectKeys)
	contextGeneratedAt := workProgramGraphContextGeneratedAt(guardrailPacket, sourceCoveragePacket, forecastPacket)
	if contextGeneratedAt == nil && run != nil {
		contextGeneratedAt = optionalTime(run.GeneratedAt)
	}
	citations := workProgramGraphContextCitations(
		itemModels,
		items,
		actionModels,
		actions,
		dependencyModels,
		dependencyRows,
		insightModels,
		insightRows,
		forecastModels,
		forecastRows,
		guardrailPacket,
		sourceCoveragePacket,
		forecastPacket,
	)
	allowedCitations := workProgramGraphCitationRefs(citations)
	contextHash := workProgramGraphContextHash(
		workstreamKey,
		*sourceFilter,
		scopeMode,
		pointerString(selectedRunKey),
		pointerString(contextGeneratedAt),
		depth,
		allowedCitations,
		reachedSubjectKeys,
		itemModels,
		actionModels,
		dependencyModels,
		insightModels,
		forecastModels,
		guardrailPacket,
		sourceCoveragePacket,
		forecastPacket,
	)
	contextCitation := workProgramGraphContextCitation(contextHash, workstreamKey, *sourceFilter, scopeMode, selectedRunKey)
	citations = append([]*model.WorkGraphCitation{contextCitation}, citations...)
	allowedCitations = workProgramGraphCitationRefs(citations)
	graphSummary := workProgramGraphContextSummary(workstreamKey, itemModels, actionModels, dependencyModels, insightModels, forecastModels, guardrailPacket, sourceCoveragePacket, forecastPacket)
	llmTask := workProgramGraphContextLLMTask(workstreamKey, scopeMode)

	return &model.WorkProgramGraphContext{
		SourceInstance:       sourceFilter,
		GeneratedAt:          contextGeneratedAt,
		ScopeMode:            scopeMode,
		RunKey:               selectedRunKey,
		WorkstreamKey:        workstreamKey,
		ContextHash:          contextHash,
		TraversalDepth:       depth,
		ItemCount:            len(itemModels),
		ActionCount:          len(actionModels),
		DependencyEdgeCount:  len(dependencyModels),
		InsightCount:         len(insightModels),
		ForecastCount:        len(forecastModels),
		GraphSummary:         graphSummary,
		LlmTask:              llmTask,
		ReachedSubjectKeys:   reachedSubjectKeys,
		AllowedCitations:     allowedCitations,
		Citations:            citations,
		Items:                itemModels,
		Actions:              actionModels,
		DependencyEdges:      dependencyModels,
		Insights:             insightModels,
		Forecasts:            forecastModels,
		QualityGates:         guardrailPacket.QualityGates,
		EvidenceNeeds:        workProgramGraphContextEvidenceNeeds(guardrailPacket, sourceCoveragePacket, forecastPacket),
		ForecastPacket:       forecastPacket,
		GuardrailPacket:      guardrailPacket,
		SourceCoveragePacket: sourceCoveragePacket,
		Badges:               workProgramGraphContextBadges(scopeMode, guardrailPacket, sourceCoveragePacket, forecastPacket),
	}, nil
}

func workProgramGraphContextDepth(value *int) int {
	depth := 2
	if value != nil {
		depth = *value
	}
	if depth < 1 {
		return 1
	}
	if depth > 4 {
		return 4
	}
	return depth
}

type workProgramGraphContextRunScope struct {
	itemIDs          []int
	itemScoped       bool
	dependencyIDs    []int
	dependencyScoped bool
	insightIDs       []int
	insightScoped    bool
	forecastIDs      []int
	forecastScoped   bool
}

func (r *queryResolver) workProgramGraphContextRunScope(ctx context.Context, run *genent.WorkProgramRun) (workProgramGraphContextRunScope, error) {
	if run == nil {
		return workProgramGraphContextRunScope{}, nil
	}
	itemIDs, itemScoped, err := r.workProgramRunMemberIDsForRun(ctx, run, workProgramRunMemberTableItems)
	if err != nil {
		return workProgramGraphContextRunScope{}, err
	}
	dependencyIDs, dependencyScoped, err := r.workProgramRunMemberIDsForRun(ctx, run, workProgramRunMemberTableDependencyEdges)
	if err != nil {
		return workProgramGraphContextRunScope{}, err
	}
	insightIDs, insightScoped, err := r.workProgramRunMemberIDsForRun(ctx, run, workProgramRunMemberTableInsights)
	if err != nil {
		return workProgramGraphContextRunScope{}, err
	}
	forecastIDs, forecastScoped, err := r.workProgramRunMemberIDsForRun(ctx, run, workProgramRunMemberTableForecasts)
	if err != nil {
		return workProgramGraphContextRunScope{}, err
	}
	return workProgramGraphContextRunScope{
		itemIDs:          itemIDs,
		itemScoped:       itemScoped,
		dependencyIDs:    dependencyIDs,
		dependencyScoped: dependencyScoped,
		insightIDs:       insightIDs,
		insightScoped:    insightScoped,
		forecastIDs:      forecastIDs,
		forecastScoped:   forecastScoped,
	}, nil
}

func workProgramGraphContextGeneratedAtArgument(value *string) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, fmt.Errorf("generatedAt cannot be blank")
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("generatedAt must be RFC3339 timestamp")
}

func (r *queryResolver) workProgramGraphContextRunAnchor(ctx context.Context, sourceFilter *string, workstreamKey *string, runKey *string, generatedAt *time.Time) (*genent.WorkProgramRun, error) {
	if runKey == nil && generatedAt == nil {
		return r.latestWorkProgramRunAnchor(ctx, sourceFilter, workstreamKey)
	}
	query := r.EntClient.WorkProgramRun.Query().
		Where(workprogramrun.SourceSystemEQ("cubicle_analytics")).
		Order(
			workprogramrun.ByGeneratedAt(entsql.OrderDesc()),
			workprogramrun.ByRankScore(entsql.OrderDesc()),
			workprogramrun.ByMemberCount(entsql.OrderDesc()),
		)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramrun.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramrun.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	if runKey != nil && strings.TrimSpace(*runKey) != "" {
		query = query.Where(workprogramrun.KeyEQ(strings.TrimSpace(*runKey)))
	}
	if generatedAt != nil {
		if generatedAt.IsZero() {
			query = query.Where(workprogramrun.GeneratedAtIsNil())
		} else {
			query = query.Where(workProgramGeneratedAtTextPredicate(workprogramrun.FieldGeneratedAt, *generatedAt))
		}
	}
	run, err := query.First(ctx)
	if genent.IsNotFound(err) {
		return nil, fmt.Errorf("workProgramGraphContext run boundary not found")
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

func workProgramGraphContextScopeMode(sourceInstance *string, runKey *string, generatedAt *time.Time, run *genent.WorkProgramRun) string {
	sourceMode := "latest_source"
	if sourceInstance != nil && strings.TrimSpace(*sourceInstance) != "" {
		sourceMode = "explicit_source"
	}
	runMode := "latest_run"
	if runKey != nil && strings.TrimSpace(*runKey) != "" {
		runMode = "explicit_run_key"
	} else if generatedAt != nil {
		runMode = "explicit_generated_at"
	}
	if run != nil {
		return sourceMode + ":" + runMode + ":work_program_run_boundary"
	}
	return sourceMode + ":" + runMode + ":latest_graph_rows_without_run_boundary"
}

func workProgramGraphContextActions(items []*genent.WorkProgramItem, limit int) []*genent.WorkAction {
	rows := make([]*genent.WorkAction, 0, min(limit, len(items)))
	seen := map[string]bool{}
	for _, item := range items {
		if item == nil || item.Edges.WorkAction == nil {
			continue
		}
		action := item.Edges.WorkAction
		if strings.TrimSpace(action.Key) == "" || seen[action.Key] {
			continue
		}
		seen[action.Key] = true
		rows = append(rows, action)
		if len(rows) >= limit {
			break
		}
	}
	sortWorkActions(rows)
	return rows
}

func workProgramGraphContextItemModelsWithClaimPolicies(rows []*genent.WorkProgramItem, claimPolicies map[string]workActionClaimPolicy, fallback workActionClaimPolicy) []*model.WorkProgramItem {
	out := make([]*model.WorkProgramItem, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, workProgramItemModelWithClaimPolicy(row, workActionClaimPolicyForRow(row.Key, claimPolicies, fallback)))
	}
	return out
}

func (r *queryResolver) workProgramGraphContextDependencyRows(ctx context.Context, sourceFilter *string, scope workProgramRiskScope, limit int, depth int, runMemberIDs []int, runMemberScoped bool) ([]*genent.WorkDependencyEdge, []string, error) {
	if sourceFilter == nil || strings.TrimSpace(*sourceFilter) == "" {
		return []*genent.WorkDependencyEdge{}, nil, nil
	}
	if scope.scoped && len(scope.dependencyKeys) == 0 && len(scope.workstreamIDs) == 0 {
		return []*genent.WorkDependencyEdge{}, nil, nil
	}
	if runMemberScoped && len(runMemberIDs) == 0 {
		return []*genent.WorkDependencyEdge{}, nil, nil
	}
	seenEdges := map[string]bool{}
	reached := map[string]bool{}
	for _, key := range scope.dependencyKeys {
		if strings.TrimSpace(key) != "" {
			reached[strings.TrimSpace(key)] = true
		}
	}
	frontier := append([]string{}, scope.dependencyKeys...)
	rows := []*genent.WorkDependencyEdge{}
	for step := 0; step < depth && len(rows) < limit; step++ {
		query := r.workProgramGraphContextDependencyQuery(sourceFilter, limit)
		if runMemberScoped {
			query = query.Where(workdependencyedge.IDIn(runMemberIDs...))
		}
		if step == 0 {
			query = query.Where(workProgramDependencyScopePredicate(scope))
		} else {
			frontier = workProgramUniqueStrings(frontier)
			if len(frontier) == 0 {
				break
			}
			query = query.Where(workdependencyedge.Or(
				workdependencyedge.FromKeyIn(frontier...),
				workdependencyedge.ToKeyIn(frontier...),
			))
		}
		query = query.Limit(limit - len(rows))
		stepRows, err := query.All(ctx)
		if err != nil {
			return nil, nil, err
		}
		nextFrontier := []string{}
		for _, row := range stepRows {
			if row == nil {
				continue
			}
			if !seenEdges[row.Key] {
				seenEdges[row.Key] = true
				rows = append(rows, row)
			}
			for _, key := range []string{row.FromKey, row.ToKey} {
				key = strings.TrimSpace(key)
				if key == "" || reached[key] {
					continue
				}
				reached[key] = true
				nextFrontier = append(nextFrontier, key)
			}
			if len(rows) >= limit {
				break
			}
		}
		if len(nextFrontier) == 0 {
			break
		}
		frontier = nextFrontier
	}
	return rows, sortedKeys(reached), nil
}

func (r *queryResolver) workProgramGraphContextDependencyQuery(sourceFilter *string, limit int) *genent.WorkDependencyEdgeQuery {
	query := r.EntClient.WorkDependencyEdge.Query().
		WithWorkBlocker(func(q *genent.WorkBlockerQuery) {
			q.Where(
				workblocker.SourceSystemEQ("cubicle_analytics"),
				workblocker.ExternalKindEQ("tpm_work_blocker"),
			)
			if sourceFilter != nil {
				q.Where(workblocker.SourceInstanceEQ(*sourceFilter))
			}
			q.WithLatestEvidence()
		}).
		WithWorkAction(workActionDetails(sourceFilter)).
		WithEndpoints(func(q *genent.WorkDependencyEndpointQuery) {
			q.WithLatestEvidence().
				Order(workdependencyendpoint.ByEndpointRole())
		}).
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
	return query
}

func workProgramGraphContextSubjectKeys(workstreamKey string, items []*genent.WorkProgramItem, actions []*genent.WorkAction, dependencies []*genent.WorkDependencyEdge, reached []string) []string {
	set := map[string]bool{}
	for _, key := range workProgramWorkstreamFilterKeys(workstreamKey) {
		set[strings.TrimSpace(key)] = true
	}
	for _, item := range items {
		if item != nil {
			set[strings.TrimSpace(item.SubjectKey)] = true
			set[strings.TrimSpace(item.Key)] = true
		}
	}
	for _, action := range actions {
		if action != nil {
			set[strings.TrimSpace(action.SubjectKey)] = true
			set[strings.TrimSpace(action.Key)] = true
		}
	}
	for _, edge := range dependencies {
		if edge != nil {
			set[strings.TrimSpace(edge.FromKey)] = true
			set[strings.TrimSpace(edge.ToKey)] = true
			set[strings.TrimSpace(edge.Key)] = true
		}
	}
	for _, key := range reached {
		set[strings.TrimSpace(key)] = true
	}
	return sortedKeys(set)
}

func workProgramGraphContextReachedSubjectKeys(keys []string) []string {
	return workProgramUniqueStrings(keys)
}

func (r *queryResolver) workProgramGraphContextInsightRows(ctx context.Context, sourceFilter *string, subjectKeys []string, limit int, runMemberIDs []int, runMemberScoped bool) ([]*genent.WorkInsight, error) {
	if sourceFilter == nil || strings.TrimSpace(*sourceFilter) == "" || len(subjectKeys) == 0 {
		return []*genent.WorkInsight{}, nil
	}
	if runMemberScoped && len(runMemberIDs) == 0 {
		return []*genent.WorkInsight{}, nil
	}
	query := r.EntClient.WorkInsight.Query().
		WithLatestEvidence().
		WithReviews(func(rq *genent.WorkInsightReviewQuery) {
			rq = applyWorkInsightReviewSourceFilter(rq, sourceFilter)
			rq.Order(
				workinsightreview.ByMeasurementEligible(entsql.OrderDesc()),
				workinsightreview.ByReviewedAt(entsql.OrderDesc()),
				workinsightreview.ByUpdatedAt(entsql.OrderDesc()),
			)
		}).
		Where(
			workinsight.SourceSystemEQ("cubicle_analytics"),
			workinsight.SourceInstanceEQ(*sourceFilter),
			workinsight.ExternalKindEQ("tpm_insight"),
			workinsight.InsightKindNEQ(workinsight.InsightKindAiGraphBrief),
			workinsight.Or(
				workinsight.ModelMethodIsNil(),
				workinsight.Not(workinsight.ModelMethodHasPrefix("bounded_graph_context_to_cited_brief")),
			),
			workinsight.Or(
				workinsight.SourceURLIsNil(),
				workinsight.Not(workinsight.SourceURLHasPrefix("cubicle://graph-brief/")),
			),
			workinsight.ProducerStateEQ(workinsight.ProducerStateCurrent),
			workinsight.SubjectKeyIn(subjectKeys...),
		).
		Order(
			workinsight.ByRankScore(entsql.OrderDesc()),
			workinsight.ByLastActivityAt(entsql.OrderDesc()),
			workinsight.ByUpdatedAt(entsql.OrderDesc()),
		).
		Limit(limit)
	if runMemberScoped {
		query = query.Where(workinsight.IDIn(runMemberIDs...))
	}
	return query.All(ctx)
}

func workProgramGraphContextCitations(
	itemModels []*model.WorkProgramItem,
	itemRows []*genent.WorkProgramItem,
	actionModels []*model.WorkAction,
	actionRows []*genent.WorkAction,
	dependencyModels []*model.WorkDependencyEdge,
	dependencyRows []*genent.WorkDependencyEdge,
	insightModels []*model.WorkInsightSummary,
	insightRows []*genent.WorkInsight,
	forecastModels []*model.WorkItemForecast,
	forecastRows []*genent.WorkItemForecast,
	guardrail *model.WorkProgramGuardrailPacket,
	sourceCoverage *model.WorkProgramSourceCoveragePacket,
	forecast *model.WorkProgramForecastPacket,
) []*model.WorkGraphCitation {
	citations := []*model.WorkGraphCitation{}
	itemRowsByKey := workProgramGraphContextItemRowsByKey(itemRows)
	for _, row := range itemModels {
		if row == nil {
			continue
		}
		freshness, visibility := "unknown", "unknown"
		if entRow := itemRowsByKey[row.Key]; entRow != nil {
			freshness = entRow.FreshnessState.String()
			visibility = entRow.Visibility.String()
		}
		citations = append(citations, workProgramGraphRowCitation(
			"work_program_items",
			"work_program_item",
			row.Key,
			row.SourceInstance,
			row.Evidence,
			freshness,
			visibility,
			row.ClaimUse,
			row.ClaimGateReason,
			row.ProductActionAllowed,
			nil,
		))
	}

	actionRowsByKey := workProgramGraphContextActionRowsByKey(actionRows)
	for _, row := range actionModels {
		if row == nil {
			continue
		}
		freshness, visibility := "unknown", "unknown"
		if entRow := actionRowsByKey[row.Key]; entRow != nil {
			freshness = entRow.FreshnessState.String()
			visibility = entRow.Visibility.String()
		}
		citations = append(citations, workProgramGraphRowCitation(
			"work_actions",
			"work_action",
			row.Key,
			row.SourceInstance,
			row.Evidence,
			freshness,
			visibility,
			row.ClaimUse,
			row.ClaimGateReason,
			row.ProductActionAllowed,
			nil,
		))
	}

	dependencyRowsByKey := workProgramGraphContextDependencyRowsByKey(dependencyRows)
	for _, row := range dependencyModels {
		if row == nil {
			continue
		}
		freshness, visibility := row.FreshnessState, row.Visibility
		if entRow := dependencyRowsByKey[row.Key]; entRow != nil {
			freshness = entRow.FreshnessState.String()
			visibility = entRow.Visibility.String()
		}
		citations = append(citations, workProgramGraphRowCitation(
			"work_dependency_edges",
			"work_dependency_edge",
			row.Key,
			row.SourceInstance,
			row.Evidence,
			freshness,
			visibility,
			row.ClaimUse,
			row.ClaimGateReason,
			row.RelationshipClaimAllowed,
			nil,
		))
	}

	insightRowsByKey := workProgramGraphContextInsightRowsByKey(insightRows)
	for _, row := range insightModels {
		if row == nil {
			continue
		}
		freshness, visibility := "unknown", "unknown"
		sourceInstance := (*string)(nil)
		if entRow := insightRowsByKey[row.Key]; entRow != nil {
			freshness = entRow.FreshnessState.String()
			visibility = entRow.Visibility.String()
			sourceInstance = optionalString(entRow.SourceInstance)
		}
		citations = append(citations, workProgramGraphRowCitation(
			"work_insights",
			"work_insight",
			row.Key,
			sourceInstance,
			row.Evidence,
			freshness,
			visibility,
			"context_signal",
			firstNonempty(pointerString(row.ReviewState), row.InsightKind),
			false,
			nil,
		))
	}

	forecastRowsByKey := workProgramGraphContextForecastRowsByKey(forecastRows)
	for _, row := range forecastModels {
		if row == nil {
			continue
		}
		freshness, visibility := "unknown", "unknown"
		if entRow := forecastRowsByKey[row.Key]; entRow != nil {
			freshness = entRow.FreshnessState.String()
			visibility = entRow.Visibility.String()
		}
		citations = append(citations, workProgramGraphRowCitation(
			"work_item_forecasts",
			"work_item_forecast",
			row.Key,
			&row.SourceInstance,
			row.Evidence,
			freshness,
			visibility,
			row.ForecastClaimUse,
			row.ForecastClaimGateReason,
			row.EtaClaimAllowed,
			nil,
		))
	}

	citations = append(citations, workProgramGraphPacketCitations(guardrail, sourceCoverage, forecast)...)
	return workProgramGraphContextUniqueCitations(citations)
}

func workProgramGraphContextCitation(contextHash string, workstreamKey string, sourceInstance string, scopeMode string, runKey *string) *model.WorkGraphCitation {
	detail := fmt.Sprintf("scope=%s", scopeMode)
	if runKey != nil && strings.TrimSpace(*runKey) != "" {
		detail += "; run_key=" + strings.TrimSpace(*runKey)
	}
	return &model.WorkGraphCitation{
		Ref:              fmt.Sprintf("[context:%s]", contextHash),
		CitationKind:     "graph_context",
		NodeKind:         "work_program_graph_context",
		NodeKey:          workstreamKey,
		SourceInstance:   optionalString(sourceInstance),
		ProofState:       "derived_context",
		FreshnessState:   "current",
		Visibility:       "private",
		ClaimUse:         optionalString("context_boundary"),
		ClaimGateReason:  optionalString(scopeMode),
		ClaimAllowed:     true,
		ExcerptAllowed:   false,
		SourceURLAllowed: false,
		Detail:           optionalString(detail),
	}
}

func workProgramGraphRowCitation(table string, nodeKind string, key string, sourceInstance *string, evidence *model.WorkEvidenceSummary, fallbackFreshness string, fallbackVisibility string, claimUse string, claimGateReason string, claimAllowed bool, detail *string) *model.WorkGraphCitation {
	proofState, freshnessState, visibility, evidenceRef, excerptAllowed, sourceURLAllowed := workProgramGraphCitationEvidencePolicy(evidence, fallbackFreshness, fallbackVisibility)
	return &model.WorkGraphCitation{
		Ref:              workProgramGraphCitationRef(table, key),
		CitationKind:     "typed_row",
		NodeKind:         nodeKind,
		NodeKey:          key,
		SourceInstance:   sourceInstance,
		EvidenceRef:      evidenceRef,
		ProofState:       proofState,
		FreshnessState:   freshnessState,
		Visibility:       visibility,
		ClaimUse:         optionalString(claimUse),
		ClaimGateReason:  optionalString(claimGateReason),
		ClaimAllowed:     claimAllowed,
		ExcerptAllowed:   excerptAllowed,
		SourceURLAllowed: sourceURLAllowed,
		Detail:           detail,
	}
}

func workProgramGraphPacketCitations(guardrail *model.WorkProgramGuardrailPacket, sourceCoverage *model.WorkProgramSourceCoveragePacket, forecast *model.WorkProgramForecastPacket) []*model.WorkGraphCitation {
	citations := []*model.WorkGraphCitation{
		workProgramGraphDerivedCitation("[analytics:tpm_forecast_summary]", "analytics_summary", "tpm_forecast_summary", nil, "risk_triage_context", "forecast_summary_context", false, "Forecast analytics are diagnostic unless forecast packet gates allow ETA claims."),
		workProgramGraphDerivedCitation("[analytics:tpm_evaluation_readiness]", "analytics_summary", "tpm_evaluation_readiness", nil, "evaluation_context", "readiness_context", false, "Evaluation readiness constrains generated claims."),
		workProgramGraphDerivedCitation("[analytics:tpm_blocker_candidates]", "analytics_summary", "tpm_blocker_candidates", nil, "blocker_candidate_context", "measurement_required", false, "Blocker candidates require measurement before product-action claims."),
	}
	if guardrail != nil {
		citations = append(citations, workProgramGraphDerivedCitation(workProgramGraphCitationRef("guardrail", guardrail.WorkstreamKey), "work_program_guardrail_packet", guardrail.WorkstreamKey, guardrail.SourceInstance, "guardrail_packet", guardrail.GuardrailState, guardrail.AutonomousActionReady, guardrail.AutomationSummary))
		for _, gate := range guardrail.QualityGates {
			if gate == nil {
				continue
			}
			citations = append(citations, workProgramGraphDerivedCitation(workProgramGraphCitationRef("work_program_quality_gates", gate.Key), "work_program_quality_gate", gate.Key, guardrail.SourceInstance, "quality_gate", gate.GateState, !gate.Blocking, gate.Detail))
		}
		for _, need := range guardrail.EvidenceNeeds {
			citations = append(citations, workProgramGraphEvidenceNeedCitation(need, guardrail.SourceInstance))
		}
	}
	if sourceCoverage != nil {
		citations = append(citations, workProgramGraphDerivedCitation(workProgramGraphCitationRef("source_coverage", sourceCoverage.WorkstreamKey), "work_program_source_coverage_packet", sourceCoverage.WorkstreamKey, sourceCoverage.SourceInstance, "source_coverage_gate", sourceCoverage.AbsenceClaimGateReason, sourceCoverage.AbsenceClaimsAllowed, sourceCoverage.AutomationSummary))
		for _, need := range sourceCoverage.EvidenceNeeds {
			citations = append(citations, workProgramGraphEvidenceNeedCitation(need, sourceCoverage.SourceInstance))
		}
	}
	if forecast != nil {
		for _, need := range forecast.EvidenceNeeds {
			citations = append(citations, workProgramGraphEvidenceNeedCitation(need, forecast.SourceInstance))
		}
	}
	return citations
}

func workProgramGraphDerivedCitation(ref string, nodeKind string, nodeKey string, sourceInstance *string, claimUse string, claimGateReason string, claimAllowed bool, detail string) *model.WorkGraphCitation {
	return &model.WorkGraphCitation{
		Ref:              ref,
		CitationKind:     "derived_packet",
		NodeKind:         nodeKind,
		NodeKey:          nodeKey,
		SourceInstance:   sourceInstance,
		ProofState:       "derived_packet",
		FreshnessState:   "current",
		Visibility:       "private",
		ClaimUse:         optionalString(claimUse),
		ClaimGateReason:  optionalString(claimGateReason),
		ClaimAllowed:     claimAllowed,
		ExcerptAllowed:   false,
		SourceURLAllowed: false,
		Detail:           optionalString(detail),
	}
}

func workProgramGraphEvidenceNeedCitation(need *model.WorkProgramAutomationEvidenceNeed, sourceInstance *string) *model.WorkGraphCitation {
	if need == nil {
		return nil
	}
	return workProgramGraphDerivedCitation(
		workProgramGraphCitationRef("work_program_evidence_needs", need.Key),
		"work_program_evidence_need",
		need.Key,
		sourceInstance,
		"evidence_need",
		need.GateKey,
		false,
		firstNonempty(need.RecommendedAction, need.NextExecutionStep, need.EvidenceKind),
	)
}

func workProgramGraphCitationEvidencePolicy(evidence *model.WorkEvidenceSummary, fallbackFreshness string, fallbackVisibility string) (string, string, string, *string, bool, bool) {
	proofState := "no_direct_evidence"
	freshnessState := firstNonempty(fallbackFreshness, "unknown")
	visibility := firstNonempty(fallbackVisibility, "unknown")
	evidenceRef := (*string)(nil)
	if evidence != nil {
		proofState = firstNonempty(evidence.ProofState, proofState)
		freshnessState = firstNonempty(evidence.FreshnessState, freshnessState)
		visibility = firstNonempty(evidence.Visibility, visibility)
		evidenceRef = optionalString(evidence.Ref)
	}
	excerptAllowed := proofState == "current" && freshnessState == "fresh" && visibility != "restricted" && visibility != "unknown"
	sourceURLAllowed := excerptAllowed
	return proofState, freshnessState, visibility, evidenceRef, excerptAllowed, sourceURLAllowed
}

func workProgramGraphCitationRefs(citations []*model.WorkGraphCitation) []string {
	refs := make(map[string]bool, len(citations))
	for _, citation := range citations {
		if citation == nil || strings.TrimSpace(citation.Ref) == "" {
			continue
		}
		refs[strings.TrimSpace(citation.Ref)] = true
	}
	return sortedKeys(refs)
}

func workProgramGraphCitationRef(table string, key string) string {
	return fmt.Sprintf("[%s:%s]", table, strings.TrimSpace(key))
}

func workProgramGraphContextUniqueCitations(rows []*model.WorkGraphCitation) []*model.WorkGraphCitation {
	out := make([]*model.WorkGraphCitation, 0, len(rows))
	seen := map[string]bool{}
	for _, row := range rows {
		if row == nil || strings.TrimSpace(row.Ref) == "" || seen[row.Ref] {
			continue
		}
		seen[row.Ref] = true
		out = append(out, row)
	}
	return out
}

func workProgramGraphContextItemRowsByKey(rows []*genent.WorkProgramItem) map[string]*genent.WorkProgramItem {
	out := map[string]*genent.WorkProgramItem{}
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.Key) != "" {
			out[row.Key] = row
		}
	}
	return out
}

func workProgramGraphContextActionRowsByKey(rows []*genent.WorkAction) map[string]*genent.WorkAction {
	out := map[string]*genent.WorkAction{}
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.Key) != "" {
			out[row.Key] = row
		}
	}
	return out
}

func workProgramGraphContextDependencyRowsByKey(rows []*genent.WorkDependencyEdge) map[string]*genent.WorkDependencyEdge {
	out := map[string]*genent.WorkDependencyEdge{}
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.Key) != "" {
			out[row.Key] = row
		}
	}
	return out
}

func workProgramGraphContextInsightRowsByKey(rows []*genent.WorkInsight) map[string]*genent.WorkInsight {
	out := map[string]*genent.WorkInsight{}
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.Key) != "" {
			out[row.Key] = row
		}
	}
	return out
}

func workProgramGraphContextForecastRowsByKey(rows []*genent.WorkItemForecast) map[string]*genent.WorkItemForecast {
	out := map[string]*genent.WorkItemForecast{}
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.Key) != "" {
			out[row.Key] = row
		}
	}
	return out
}

func workProgramGraphContextAllowedCitations(
	items []*genent.WorkProgramItem,
	actions []*genent.WorkAction,
	dependencies []*genent.WorkDependencyEdge,
	insights []*genent.WorkInsight,
	forecasts []*genent.WorkItemForecast,
	guardrail *model.WorkProgramGuardrailPacket,
	sourceCoverage *model.WorkProgramSourceCoveragePacket,
	forecast *model.WorkProgramForecastPacket,
) []string {
	citations := map[string]bool{
		"[analytics:tpm_forecast_summary]":     true,
		"[analytics:tpm_evaluation_readiness]": true,
		"[analytics:tpm_blocker_candidates]":   true,
	}
	for _, item := range items {
		if item != nil {
			workProgramGraphContextAddCitation(citations, "work_program_items", item.Key)
		}
	}
	for _, action := range actions {
		if action != nil {
			workProgramGraphContextAddCitation(citations, "work_actions", action.Key)
		}
	}
	for _, edge := range dependencies {
		if edge != nil {
			workProgramGraphContextAddCitation(citations, "work_dependency_edges", edge.Key)
		}
	}
	for _, insight := range insights {
		if insight != nil {
			workProgramGraphContextAddCitation(citations, "work_insights", insight.Key)
		}
	}
	for _, row := range forecasts {
		if row != nil {
			workProgramGraphContextAddCitation(citations, "work_item_forecasts", row.Key)
		}
	}
	if guardrail != nil {
		workProgramGraphContextAddCitation(citations, "guardrail", guardrail.WorkstreamKey)
		for _, gate := range guardrail.QualityGates {
			if gate != nil {
				workProgramGraphContextAddCitation(citations, "work_program_quality_gates", gate.Key)
			}
		}
		for _, need := range guardrail.EvidenceNeeds {
			if need != nil {
				workProgramGraphContextAddCitation(citations, "work_program_evidence_needs", need.Key)
			}
		}
	}
	if sourceCoverage != nil {
		workProgramGraphContextAddCitation(citations, "source_coverage", sourceCoverage.WorkstreamKey)
		for _, need := range sourceCoverage.EvidenceNeeds {
			if need != nil {
				workProgramGraphContextAddCitation(citations, "work_program_evidence_needs", need.Key)
			}
		}
	}
	if forecast != nil {
		for _, need := range forecast.EvidenceNeeds {
			if need != nil {
				workProgramGraphContextAddCitation(citations, "work_program_evidence_needs", need.Key)
			}
		}
	}
	return sortedKeys(citations)
}

func workProgramGraphContextAddCitation(citations map[string]bool, table string, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	citations[fmt.Sprintf("[%s:%s]", table, key)] = true
}

func workProgramGraphContextHash(
	workstreamKey string,
	sourceInstance string,
	scopeMode string,
	runKey string,
	generatedAt string,
	depth int,
	citations []string,
	reachedSubjectKeys []string,
	items []*model.WorkProgramItem,
	actions []*model.WorkAction,
	dependencies []*model.WorkDependencyEdge,
	insights []*model.WorkInsightSummary,
	forecasts []*model.WorkItemForecast,
	guardrail *model.WorkProgramGuardrailPacket,
	sourceCoverage *model.WorkProgramSourceCoveragePacket,
	forecast *model.WorkProgramForecastPacket,
) string {
	payload := struct {
		WorkstreamKey      string                            `json:"workstream_key"`
		SourceInstance     string                            `json:"source_instance"`
		ScopeMode          string                            `json:"scope_mode"`
		RunKey             string                            `json:"run_key"`
		GeneratedAt        string                            `json:"generated_at"`
		TraversalDepth     int                               `json:"traversal_depth"`
		AllowedCitations   []string                          `json:"allowed_citations"`
		ReachedSubjectKeys []string                          `json:"reached_subject_keys"`
		Items              []workProgramGraphContextHashNode `json:"items"`
		Actions            []workProgramGraphContextHashNode `json:"actions"`
		Dependencies       []workProgramGraphContextHashNode `json:"dependencies"`
		Insights           []workProgramGraphContextHashNode `json:"insights"`
		Forecasts          []workProgramGraphContextHashNode `json:"forecasts"`
		Packets            []workProgramGraphContextHashNode `json:"packets"`
	}{
		WorkstreamKey:      workstreamKey,
		SourceInstance:     sourceInstance,
		ScopeMode:          scopeMode,
		RunKey:             runKey,
		GeneratedAt:        generatedAt,
		TraversalDepth:     depth,
		AllowedCitations:   citations,
		ReachedSubjectKeys: reachedSubjectKeys,
		Items:              workProgramGraphContextItemHashNodes(items),
		Actions:            workProgramGraphContextActionHashNodes(actions),
		Dependencies:       workProgramGraphContextDependencyHashNodes(dependencies),
		Insights:           workProgramGraphContextInsightHashNodes(insights),
		Forecasts:          workProgramGraphContextForecastHashNodes(forecasts),
		Packets:            workProgramGraphContextPacketHashNodes(guardrail, sourceCoverage, forecast),
	}
	bytes, _ := json.Marshal(payload)
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:])[:16]
}

type workProgramGraphContextHashNode struct {
	Kind     string  `json:"kind"`
	Key      string  `json:"key"`
	State    string  `json:"state,omitempty"`
	Gate     string  `json:"gate,omitempty"`
	Use      string  `json:"use,omitempty"`
	Allowed  bool    `json:"allowed,omitempty"`
	Count    int     `json:"count,omitempty"`
	Score    float64 `json:"score,omitempty"`
	Severity string  `json:"severity,omitempty"`
}

func workProgramGraphContextItemHashNodes(rows []*model.WorkProgramItem) []workProgramGraphContextHashNode {
	out := make([]workProgramGraphContextHashNode, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, workProgramGraphContextHashNode{
			Kind:    "work_program_item",
			Key:     row.Key,
			State:   row.DecisionState,
			Gate:    row.ClaimGateReason,
			Use:     row.ClaimUse,
			Allowed: row.ProductActionAllowed,
			Score:   row.RiskScore,
		})
	}
	return out
}

func workProgramGraphContextActionHashNodes(rows []*model.WorkAction) []workProgramGraphContextHashNode {
	out := make([]workProgramGraphContextHashNode, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, workProgramGraphContextHashNode{
			Kind:    "work_action",
			Key:     row.Key,
			State:   row.DecisionState,
			Gate:    row.ClaimGateReason,
			Use:     row.ClaimUse,
			Allowed: row.ProductActionAllowed,
			Score:   row.RankScore,
		})
	}
	return out
}

func workProgramGraphContextDependencyHashNodes(rows []*model.WorkDependencyEdge) []workProgramGraphContextHashNode {
	out := make([]workProgramGraphContextHashNode, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, workProgramGraphContextHashNode{
			Kind:    "work_dependency_edge",
			Key:     row.Key,
			State:   row.EdgeKind,
			Gate:    row.ClaimGateReason,
			Use:     row.ClaimUse,
			Allowed: row.RelationshipClaimAllowed,
			Score:   row.RankScore,
		})
	}
	return out
}

func workProgramGraphContextInsightHashNodes(rows []*model.WorkInsightSummary) []workProgramGraphContextHashNode {
	out := make([]workProgramGraphContextHashNode, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, workProgramGraphContextHashNode{
			Kind:     "work_insight",
			Key:      row.Key,
			State:    row.InsightKind,
			Score:    row.Score,
			Severity: row.Severity,
		})
	}
	return out
}

func workProgramGraphContextForecastHashNodes(rows []*model.WorkItemForecast) []workProgramGraphContextHashNode {
	out := make([]workProgramGraphContextHashNode, 0, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		out = append(out, workProgramGraphContextHashNode{
			Kind:    "work_item_forecast",
			Key:     row.Key,
			State:   row.ReadinessState,
			Gate:    row.ForecastClaimGateReason,
			Use:     row.ForecastClaimUse,
			Allowed: row.EtaClaimAllowed,
			Score:   row.RiskScore,
		})
	}
	return out
}

func workProgramGraphContextPacketHashNodes(guardrail *model.WorkProgramGuardrailPacket, sourceCoverage *model.WorkProgramSourceCoveragePacket, forecast *model.WorkProgramForecastPacket) []workProgramGraphContextHashNode {
	out := []workProgramGraphContextHashNode{}
	if guardrail != nil {
		out = append(out, workProgramGraphContextHashNode{Kind: "guardrail", Key: guardrail.WorkstreamKey, State: guardrail.GuardrailState, Gate: guardrail.ReadinessState, Allowed: guardrail.AutonomousActionReady, Count: guardrail.EvidenceNeedCount + guardrail.BlockingGateCount + guardrail.FailedCheckCount})
	}
	if sourceCoverage != nil {
		out = append(out, workProgramGraphContextHashNode{Kind: "source_coverage", Key: sourceCoverage.WorkstreamKey, State: sourceCoverage.CoverageState, Gate: sourceCoverage.AbsenceClaimGateReason, Allowed: sourceCoverage.AbsenceClaimsAllowed, Count: sourceCoverage.SourceSyncIssueCount + sourceCoverage.EvidenceNeedCount})
	}
	if forecast != nil {
		out = append(out, workProgramGraphContextHashNode{Kind: "forecast", Key: forecast.WorkstreamKey, State: forecast.ReadinessState, Allowed: forecast.EtaForecastReady, Count: forecast.EvidenceNeedCount + forecast.HighRiskForecastCount})
	}
	return out
}

func workProgramGraphContextSummary(
	workstreamKey string,
	items []*model.WorkProgramItem,
	actions []*model.WorkAction,
	dependencies []*model.WorkDependencyEdge,
	insights []*model.WorkInsightSummary,
	forecasts []*model.WorkItemForecast,
	guardrail *model.WorkProgramGuardrailPacket,
	sourceCoverage *model.WorkProgramSourceCoveragePacket,
	forecast *model.WorkProgramForecastPacket,
) string {
	guardrailState := "unknown"
	if guardrail != nil {
		guardrailState = guardrail.GuardrailState
	}
	coverageState := "unknown"
	absenceGate := "unknown"
	if sourceCoverage != nil {
		coverageState = sourceCoverage.CoverageState
		absenceGate = sourceCoverage.AbsenceClaimGateReason
	}
	forecastState := "unknown"
	forecastUse := "risk triage only"
	if forecast != nil {
		forecastState = forecast.ReadinessState
		if forecast.EtaForecastReady {
			forecastUse = "ETA candidates require owner confirmation"
		}
	}
	return fmt.Sprintf(
		"%s graph context includes %d items, %d actions, %d dependency edges, %d insights, and %d high-risk forecasts; guardrail=%s, source_coverage=%s, absence_gate=%s, forecast=%s (%s).",
		workstreamKey,
		len(items),
		len(actions),
		len(dependencies),
		len(insights),
		len(forecasts),
		guardrailState,
		coverageState,
		absenceGate,
		forecastState,
		forecastUse,
	)
}

func workProgramGraphContextLLMTask(workstreamKey string, scopeMode string) string {
	task := fmt.Sprintf("Summarize the bounded typed Cubicle graph for %s. Use only allowed citations, preserve source-coverage and guardrail limits, and frame forecasts as risk triage unless the forecast packet says ETA is ready.", workstreamKey)
	if strings.Contains(scopeMode, "work_program_run_boundary") {
		task += " Treat runKey/generatedAt as a hard run boundary for graph rows that have WorkProgramRunMember membership."
	} else if strings.Contains(scopeMode, "latest_graph_rows") {
		task += " Treat runKey/generatedAt as a packet boundary only; graph rows are latest scoped rows unless a future context snapshot says otherwise."
	}
	return task
}

func workProgramGraphContextGeneratedAt(packets ...interface{}) *string {
	for _, packet := range packets {
		switch value := packet.(type) {
		case *model.WorkProgramGuardrailPacket:
			if value != nil && value.GeneratedAt != nil {
				return value.GeneratedAt
			}
		case *model.WorkProgramSourceCoveragePacket:
			if value != nil && value.GeneratedAt != nil {
				return value.GeneratedAt
			}
		case *model.WorkProgramForecastPacket:
			if value != nil && value.GeneratedAt != nil {
				return value.GeneratedAt
			}
		}
	}
	return nil
}

func workProgramGraphContextEvidenceNeeds(packets ...interface{}) []*model.WorkProgramAutomationEvidenceNeed {
	out := []*model.WorkProgramAutomationEvidenceNeed{}
	seen := map[string]bool{}
	for _, packet := range packets {
		var needs []*model.WorkProgramAutomationEvidenceNeed
		switch value := packet.(type) {
		case *model.WorkProgramGuardrailPacket:
			if value != nil {
				needs = value.EvidenceNeeds
			}
		case *model.WorkProgramSourceCoveragePacket:
			if value != nil {
				needs = value.EvidenceNeeds
			}
		case *model.WorkProgramForecastPacket:
			if value != nil {
				needs = value.EvidenceNeeds
			}
		}
		for _, need := range needs {
			if need == nil || strings.TrimSpace(need.Key) == "" || seen[need.Key] {
				continue
			}
			seen[need.Key] = true
			out = append(out, need)
		}
	}
	return out
}

func workProgramGraphContextBadges(scopeMode string, guardrail *model.WorkProgramGuardrailPacket, sourceCoverage *model.WorkProgramSourceCoveragePacket, forecast *model.WorkProgramForecastPacket) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{Key: "graph_context:typed_rows", Label: "Typed graph context", Tone: "success"},
	}
	if strings.Contains(scopeMode, "work_program_run_boundary") {
		badges = append(badges, &model.WorkActionBadge{
			Key:    "graph_context:run_boundary",
			Label:  "Run boundary",
			Tone:   "success",
			Detail: optionalString("graph rows are filtered through WorkProgramRunMember membership when available"),
		})
	} else if strings.Contains(scopeMode, "latest_graph_rows") {
		badges = append(badges, &model.WorkActionBadge{
			Key:    "graph_context:latest_graph_rows",
			Label:  "Latest graph rows",
			Tone:   "warning",
			Detail: optionalString("runKey/generatedAt bounds packet rows; graph rows are latest scoped rows"),
		})
	}
	if sourceCoverage == nil || !sourceCoverage.AbsenceClaimsAllowed {
		detail := (*string)(nil)
		if sourceCoverage != nil {
			detail = optionalString(sourceCoverage.AbsenceClaimGateReason)
		}
		badges = append(badges, &model.WorkActionBadge{Key: "graph_context:coverage_limited", Label: "Coverage limits absence claims", Tone: "warning", Detail: detail})
	}
	if guardrail == nil || guardrail.HumanReviewRequired {
		detail := (*string)(nil)
		if guardrail != nil {
			detail = optionalString(guardrail.GuardrailState)
		}
		badges = append(badges, &model.WorkActionBadge{Key: "graph_context:human_review", Label: "Human review required", Tone: "warning", Detail: detail})
	}
	if forecast == nil || !forecast.EtaForecastReady {
		detail := (*string)(nil)
		if forecast != nil {
			detail = optionalString(forecast.ReadinessState)
		}
		badges = append(badges, &model.WorkActionBadge{Key: "graph_context:eta_gated", Label: "ETA gated", Tone: "info", Detail: detail})
	}
	return badges
}
