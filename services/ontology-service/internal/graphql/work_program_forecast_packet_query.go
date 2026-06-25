package graphql

import (
	"context"
	"fmt"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func (r *queryResolver) workProgramForecastPacket(ctx context.Context, workstreamKey string, limit *int, evidenceLimit *int, sourceInstance *string) (*model.WorkProgramForecastPacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramForecastPacket requires an Ent-backed ontology store")
	}
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil, fmt.Errorf("workProgramForecastPacket requires workstreamKey")
	}
	sourceFilter, err := r.aggregateSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	readiness, err := r.forecastReadinessModel(ctx, sourceFilter, "open")
	if err != nil {
		return nil, err
	}
	rowLimit := boundedLimit(limit, 20, 100)
	evidenceRowLimit := boundedLimit(evidenceLimit, 50, 200)
	decisionTargetReadiness, err := r.workDecisionTargetReadinessModel(ctx, sourceFilter, rowLimit)
	if err != nil {
		return nil, err
	}
	forecastEvaluationRows, err := r.workForecastEvaluationRows(ctx, sourceFilter)
	if err != nil {
		return nil, err
	}
	decisionTargetRows, err := r.workDecisionTargetEvaluationRows(ctx, sourceFilter, nil, nil, nil, 0)
	if err != nil {
		return nil, err
	}
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
	forecastRows, highRiskForecastCount, err := r.topWorkProgramForecastRowsAndCount(ctx, sourceFilter, scope, rowLimit)
	if err != nil {
		return nil, err
	}
	readiness = forecastReadinessForForecastRows(readiness, forecastRows)
	readiness = workProgramForecastReadinessWithDecisionTargetGate(readiness, decisionTargetReadiness)
	forecasts := workItemForecastModels(forecastRows, readiness)
	evidencePredicate := workProgramForecastPacketEvidenceNeedPredicate(workstreamFilter, forecastRows)
	filteredEvidenceNeeds, evidenceNeedCount, err := r.latestWorkProgramEvidenceNeedModelsAndCountForPredicates(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  sourceFilter,
		workstreamKey: workstreamFilter,
		generatedAt:   runGeneratedAt,
	}, evidenceRowLimit, evidencePredicate)
	if err != nil {
		return nil, err
	}
	recommendedFocus := workProgramForecastPacketFocus(readiness, forecasts, filteredEvidenceNeeds)
	return &model.WorkProgramForecastPacket{
		SourceInstance:                            sourceFilter,
		GeneratedAt:                               optionalTimePtr(runGeneratedAt),
		WorkstreamKey:                             workstreamKey,
		ReadinessState:                            readiness.ReadinessState,
		EtaForecastReady:                          readiness.EtaForecastReady,
		ForecastMethod:                            readiness.ForecastMethod,
		HighRiskForecastCount:                     highRiskForecastCount,
		EtaReadyForecastCount:                     workProgramForecastPacketEtaReadyCount(forecasts),
		DecisionTargetEvaluationState:             decisionTargetReadiness.EvaluationState,
		DecisionTargetEvaluationCount:             decisionTargetReadiness.EvaluationCount,
		ProductReadyDecisionTargetEvaluationCount: decisionTargetReadiness.ProductReadyEvaluationCount,
		EvidenceNeedCount:                         evidenceNeedCount,
		HumanRequired:                             workProgramForecastPacketHumanRequired(readiness, highRiskForecastCount, evidenceNeedCount, decisionTargetReadiness),
		AutomationSummary:                         workProgramForecastPacketSummary(workstreamKey, readiness, highRiskForecastCount, evidenceNeedCount, decisionTargetReadiness, recommendedFocus),
		RecommendedFocus:                          recommendedFocus,
		Readiness:                                 readiness,
		DecisionTargetReadiness:                   decisionTargetReadiness,
		ForecastReliability:                       workProgramForecastReliabilityModels(forecastEvaluationRows, decisionTargetRows, readiness, decisionTargetReadiness),
		Forecasts:                                 forecasts,
		DecisionTargetEvaluations:                 decisionTargetReadiness.Evaluations,
		EvidenceNeeds:                             filteredEvidenceNeeds,
	}, nil
}

func workProgramForecastReadinessWithDecisionTargetGate(readiness *model.WorkForecastReadiness, decisionTargetReadiness *model.WorkDecisionTargetReadiness) *model.WorkForecastReadiness {
	if readiness == nil || !readiness.EtaForecastReady || !workProgramTPMDecisionTargetHumanRequired(decisionTargetReadiness) {
		return readiness
	}
	out := *readiness
	out.EtaForecastReady = false
	out.ReadinessState = firstNonempty(decisionTargetReadiness.EvaluationState, "validation_gated")
	out.EtaReadinessBlockingReason = optionalString("decision_target_validation_gated")
	focus := workProgramForecastDecisionTargetGateFocus(decisionTargetReadiness)
	out.ReadinessReason = optionalString(focus)
	out.Detail = optionalString(focus)
	out.Badges = workForecastReadinessBadges(&out)
	return &out
}

func workProgramForecastDecisionTargetGateFocus(decisionTargetReadiness *model.WorkDecisionTargetReadiness) string {
	if decisionTargetReadiness == nil {
		return "Decision-target validation must pass before ETA or product-action forecast claims."
	}
	return firstNonempty(
		pointerString(decisionTargetReadiness.RecommendedFocus),
		decisionTargetReadiness.CoverageGateReason,
		decisionTargetReadiness.AutomationSummary,
		"Decision-target validation must pass before ETA or product-action forecast claims.",
	)
}

func workProgramForecastPacketEvidenceNeedPredicate(workstreamKey *string, forecasts []*genent.WorkItemForecast) predicate.WorkProgramEvidenceNeed {
	targetKeys := map[string]bool{}
	actionKeys := map[string]bool{}
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
			workprogramevidenceneed.GateKeyEQ("forecast_readiness"),
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

func workProgramForecastPacketEvidenceNeeds(forecasts []*genent.WorkItemForecast, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed, limit int) []*model.WorkProgramAutomationEvidenceNeed {
	targetKeys := map[string]bool{}
	actionKeys := map[string]bool{}
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
		if workProgramForecastPacketEvidenceNeedMatches(need, targetKeys, actionKeys) {
			out = append(out, need)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func workProgramForecastPacketEvidenceNeedMatches(need *model.WorkProgramAutomationEvidenceNeed, targetKeys map[string]bool, actionKeys map[string]bool) bool {
	if need.GateKey == "forecast_readiness" {
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

func workProgramForecastPacketEtaReadyCount(forecasts []*model.WorkItemForecast) int {
	count := 0
	for _, forecast := range forecasts {
		if forecast != nil && forecast.EtaForecastReady {
			count++
		}
	}
	return count
}

func workProgramForecastPacketHumanRequired(readiness *model.WorkForecastReadiness, forecastCount int, evidenceNeedCount int, decisionTargetReadiness *model.WorkDecisionTargetReadiness) bool {
	if decisionTargetReadiness != nil && decisionTargetReadiness.EvaluationCount > 0 && !decisionTargetReadiness.ProductActionReady {
		return true
	}
	if evidenceNeedCount > 0 || forecastCount > 0 {
		return true
	}
	return readiness == nil || !readiness.EtaForecastReady
}

func workProgramForecastPacketFocus(readiness *model.WorkForecastReadiness, forecasts []*model.WorkItemForecast, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed) *string {
	for _, need := range evidenceNeeds {
		if need == nil || need.GateKey != "forecast_readiness" {
			continue
		}
		if strings.TrimSpace(need.RecommendedAction) != "" {
			value := strings.TrimSpace(need.RecommendedAction)
			return &value
		}
	}
	for _, need := range evidenceNeeds {
		if need != nil && strings.TrimSpace(need.RecommendedAction) != "" {
			value := strings.TrimSpace(need.RecommendedAction)
			return &value
		}
	}
	if readiness != nil && readiness.ReadinessReason != nil && strings.TrimSpace(*readiness.ReadinessReason) != "" {
		value := strings.TrimSpace(*readiness.ReadinessReason)
		return &value
	}
	if readiness != nil && readiness.Detail != nil && strings.TrimSpace(*readiness.Detail) != "" {
		value := strings.TrimSpace(*readiness.Detail)
		return &value
	}
	for _, forecast := range forecasts {
		if forecast != nil && strings.TrimSpace(forecast.RecommendedAction) != "" {
			value := strings.TrimSpace(forecast.RecommendedAction)
			return &value
		}
	}
	return nil
}

func workProgramForecastPacketSummary(workstreamKey string, readiness *model.WorkForecastReadiness, forecastCount int, evidenceNeedCount int, decisionTargetReadiness *model.WorkDecisionTargetReadiness, recommendedFocus *string) string {
	readinessState := "unknown"
	etaReady := false
	if readiness != nil {
		readinessState = readiness.ReadinessState
		etaReady = readiness.EtaForecastReady
	}
	usage := "use forecasts for risk triage only"
	if etaReady {
		usage = "forecasts may support ETA candidates after owner confirmation"
	}
	decisionTargetState := "missing"
	decisionTargetCount := 0
	if decisionTargetReadiness != nil {
		decisionTargetState = decisionTargetReadiness.EvaluationState
		decisionTargetCount = decisionTargetReadiness.EvaluationCount
	}
	summary := fmt.Sprintf("%s forecast readiness is %s; %s. Decision-target evaluation is %s with %d row(s). %d high-risk forecast(s), %d evidence need(s).", workstreamKey, readinessState, usage, decisionTargetState, decisionTargetCount, forecastCount, evidenceNeedCount)
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}
