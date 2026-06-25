package graphql

import (
	"context"
	"fmt"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func (r *queryResolver) workProgramExecutionPacket(ctx context.Context, workstreamKey string, actionState *string, actionLimit *int, evidenceLimit *int, reviewLimit *int, sourceInstance *string) (*model.WorkProgramExecutionPacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramExecutionPacket requires an Ent-backed ontology store")
	}
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil, fmt.Errorf("workProgramExecutionPacket requires workstreamKey")
	}
	state, err := normalizeWorkProgramExecutionActionState(actionState)
	if err != nil {
		return nil, err
	}
	rowLimit := boundedLimit(actionLimit, 20, 100)
	evidenceRowLimit := boundedLimit(evidenceLimit, 50, 200)

	readiness, err := r.workProgramTpmReadinessPacket(ctx, workstreamKey, &rowLimit, &evidenceRowLimit, reviewLimit, sourceInstance)
	if err != nil {
		return nil, err
	}
	sourceFilter := readiness.SourceInstance
	workstreamFilter := &workstreamKey
	runGeneratedAt, err := r.latestWorkProgramAutomationReadinessRunGeneratedAt(ctx, sourceFilter, workstreamFilter)
	if err != nil {
		return nil, err
	}
	items, err := r.workProgramItemRowsForSource(ctx, 1000, workstreamFilter, nil, nil, nil, sourceFilter)
	if err != nil {
		return nil, err
	}
	claimPolicy := workActionClaimPolicyForTpmReadiness(readiness)
	actionRows := workProgramExecutionActionRows(items, state, rowLimit)
	actionModels := workActionModelsWithClaimPolicy(actionRows, claimPolicy)
	actionCounts := workProgramExecutionActionCounts(actionRows, claimPolicy)
	evidenceNeeds, evidenceNeedCount, err := r.latestWorkProgramEvidenceNeedModelsAndCountForFilters(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  sourceFilter,
		workstreamKey: workstreamFilter,
		generatedAt:   runGeneratedAt,
	}, evidenceRowLimit)
	if err != nil {
		return nil, err
	}

	executionState := workProgramExecutionState(readiness, len(actionModels), actionCounts, evidenceNeedCount)
	recommendedFocus := workProgramExecutionFocus(actionModels, evidenceNeeds, readiness)
	return &model.WorkProgramExecutionPacket{
		SourceInstance:             sourceFilter,
		GeneratedAt:                optionalTimePtr(runGeneratedAt),
		WorkstreamKey:              workstreamKey,
		ActionState:                state,
		ExecutionState:             executionState,
		AutonomousActionReady:      readiness.AutonomousActionReady,
		HumanReviewRequired:        workProgramExecutionHumanRequired(readiness, actionCounts, evidenceNeedCount),
		SourceCoverageState:        readiness.SourceCoverageState,
		SourceCoverageLimitedCount: readiness.SourceCoverageLimitedCount,
		SourceCoverageUnknownCount: readiness.SourceCoverageUnknownCount,
		AbsenceClaimsAllowed:       readiness.AbsenceClaimsAllowed,
		ActionCount:                len(actionModels),
		ProductActionCount:         actionCounts.productAction,
		ValidationLeadCount:        actionCounts.validationLead,
		SourceRepairCount:          actionCounts.sourceRepair,
		CloseoutReviewCount:        actionCounts.closeoutReview,
		ModelOrRuleQaCount:         actionCounts.modelOrRuleQa,
		EvidenceNeedCount:          evidenceNeedCount,
		AutomationSummary:          workProgramExecutionSummary(workstreamKey, executionState, state, len(actionModels), actionCounts, evidenceNeedCount, readiness, recommendedFocus),
		RecommendedFocus:           recommendedFocus,
		TpmReadiness:               readiness,
		Actions:                    actionModels,
		EvidenceNeeds:              evidenceNeeds,
	}, nil
}

type workProgramExecutionActionCount struct {
	productAction  int
	validationLead int
	sourceRepair   int
	closeoutReview int
	modelOrRuleQa  int
}

func normalizeWorkProgramExecutionActionState(actionState *string) (string, error) {
	state := "open"
	if actionState != nil {
		state = strings.TrimSpace(*actionState)
	}
	if state == "" {
		state = "open"
	}
	if state == "all" {
		return state, nil
	}
	value := workaction.ActionState(state)
	if err := workaction.ActionStateValidator(value); err != nil {
		return "", err
	}
	return state, nil
}

func workProgramExecutionActionRows(items []*genent.WorkProgramItem, actionState string, limit int) []*genent.WorkAction {
	rows := make([]*genent.WorkAction, 0, min(limit, len(items)))
	seen := map[string]bool{}
	for _, item := range items {
		if item == nil || item.Edges.WorkAction == nil {
			continue
		}
		action := item.Edges.WorkAction
		if actionState != "all" && action.ActionState.String() != actionState {
			continue
		}
		if seen[action.Key] {
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

func workActionModels(rows []*genent.WorkAction) []*model.WorkAction {
	return workActionModelsWithClaimPolicy(rows, workActionClaimPolicy{})
}

func workActionModelsWithClaimPolicy(rows []*genent.WorkAction, claimPolicy workActionClaimPolicy) []*model.WorkAction {
	out := make([]*model.WorkAction, 0, len(rows))
	for _, row := range rows {
		out = append(out, workActionModelWithClaimPolicy(row, claimPolicy))
	}
	return out
}

func workActionClaimPolicyForTpmReadiness(readiness *model.WorkProgramTpmReadinessPacket) workActionClaimPolicy {
	if readiness == nil {
		return workActionClaimPolicy{}
	}
	return workActionClaimPolicy{
		etaForecastReady:         readiness.ForecastEtaReady,
		etaReadinessGateReason:   readiness.ForecastReadinessState,
		etaReadinessContextKnown: true,
	}
}

func workProgramExecutionActionCounts(rows []*genent.WorkAction, claimPolicy workActionClaimPolicy) workProgramExecutionActionCount {
	var counts workProgramExecutionActionCount
	for _, row := range rows {
		if row == nil {
			continue
		}
		if workActionProductActionAllowedWithPolicy(row, claimPolicy) {
			counts.productAction++
			continue
		}
		switch row.DecisionState {
		case workaction.DecisionStateValidationLead:
			counts.validationLead++
		case workaction.DecisionStateSourceRepair:
			counts.sourceRepair++
		case workaction.DecisionStateCloseoutReview:
			counts.closeoutReview++
		case workaction.DecisionStateModelOrRuleQa:
			counts.modelOrRuleQa++
		}
	}
	return counts
}

func workProgramExecutionState(readiness *model.WorkProgramTpmReadinessPacket, actionCount int, actionCounts workProgramExecutionActionCount, evidenceNeedCount int) string {
	if readiness != nil && readiness.ReplacementState == "blocked" && !readiness.AbsenceClaimsAllowed {
		return "blocked_source_coverage"
	}
	if readiness != nil && readiness.ResponsibilityValidationCount > 0 {
		return "human_review_required"
	}
	if actionCount == 0 && evidenceNeedCount == 0 {
		return "no_open_work"
	}
	if readiness != nil && readiness.ReplacementState == "blocked" {
		return "blocked_review_queue"
	}
	if evidenceNeedCount > 0 || actionCounts.validationLead > 0 || actionCounts.sourceRepair > 0 || actionCounts.closeoutReview > 0 || actionCounts.modelOrRuleQa > 0 {
		return "human_review_required"
	}
	if readiness != nil && readiness.AutonomousActionReady && actionCounts.productAction > 0 {
		return "autonomous_actions_ready"
	}
	if actionCounts.productAction > 0 {
		return "product_actions_ready"
	}
	return "review_required"
}

func workProgramExecutionHumanRequired(readiness *model.WorkProgramTpmReadinessPacket, actionCounts workProgramExecutionActionCount, evidenceNeedCount int) bool {
	if readiness == nil || !readiness.AutonomousActionReady {
		return true
	}
	return evidenceNeedCount > 0 || actionCounts.validationLead > 0 || actionCounts.sourceRepair > 0 || actionCounts.closeoutReview > 0 || actionCounts.modelOrRuleQa > 0
}

func workProgramExecutionFocus(actions []*model.WorkAction, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed, readiness *model.WorkProgramTpmReadinessPacket) *string {
	if readiness != nil && readiness.RecommendedFocus != nil {
		if value := optionalTrimmedPointerValue(readiness.RecommendedFocus); value != nil {
			return value
		}
	}
	for _, need := range evidenceNeeds {
		if need != nil {
			if value := optionalTrimmedPointer(need.RecommendedAction); value != nil {
				return value
			}
		}
	}
	for _, action := range actions {
		if action != nil {
			if value := optionalTrimmedPointerValue(action.RecommendedAction); value != nil {
				return value
			}
		}
	}
	return nil
}

func workProgramExecutionSummary(workstreamKey string, executionState string, actionState string, actionCount int, counts workProgramExecutionActionCount, evidenceNeedCount int, readiness *model.WorkProgramTpmReadinessPacket, recommendedFocus *string) string {
	replacementState := "unknown"
	sourceCoverageState := "unknown"
	sourceCoverageLimitedCount := 0
	sourceCoverageUnknownCount := 0
	absenceClaimsAllowed := false
	if readiness != nil {
		replacementState = readiness.ReplacementState
		sourceCoverageState = readiness.SourceCoverageState
		sourceCoverageLimitedCount = readiness.SourceCoverageLimitedCount
		sourceCoverageUnknownCount = readiness.SourceCoverageUnknownCount
		absenceClaimsAllowed = readiness.AbsenceClaimsAllowed
	}
	summary := fmt.Sprintf("%s execution packet is %s for %s actions; AI-TPM replacement readiness is %s. Source coverage is %s with %d limited item(s), %d unknown item(s), and absence claims allowed: %t. %d action(s): %d product, %d validation, %d source repair, %d closeout, %d model/QA. %d evidence need(s).", workstreamKey, executionState, actionState, replacementState, sourceCoverageState, sourceCoverageLimitedCount, sourceCoverageUnknownCount, absenceClaimsAllowed, actionCount, counts.productAction, counts.validationLead, counts.sourceRepair, counts.closeoutReview, counts.modelOrRuleQa, evidenceNeedCount)
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}
