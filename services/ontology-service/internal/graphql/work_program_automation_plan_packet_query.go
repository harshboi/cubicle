package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cubicle/services/ontology-service/internal/graphql/model"
)

type workProgramAutomationPlanQueues struct {
	autonomousActions   []*model.WorkAction
	humanReviewActions  []*model.WorkAction
	blockedActions      []*model.WorkAction
	safeFunctions       []*model.WorkProgramTpmFunctionReadiness
	supervisedFunctions []*model.WorkProgramTpmFunctionReadiness
	blockedFunctions    []*model.WorkProgramTpmFunctionReadiness
}

func (r *queryResolver) workProgramAutomationPlanPacket(ctx context.Context, workstreamKey string, actionState *string, actionLimit *int, evidenceLimit *int, reviewLimit *int, sourceInstance *string) (*model.WorkProgramAutomationPlanPacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramAutomationPlanPacket requires an Ent-backed ontology store")
	}
	execution, err := r.workProgramExecutionPacket(ctx, workstreamKey, actionState, actionLimit, evidenceLimit, reviewLimit, sourceInstance)
	if err != nil {
		return nil, err
	}
	actionEvidenceNeeds, err := r.workProgramAutomationPlanActionEvidenceNeeds(ctx, execution)
	if err != nil {
		return nil, err
	}
	responsibilityLimit := boundedLimit(actionLimit, 20, 100)
	responsibilities, err := r.workProgramAttentionResponsibilityModels(ctx, execution.SourceInstance, execution.WorkstreamKey, responsibilityLimit)
	if err != nil {
		return nil, err
	}
	planEvidenceNeeds := workProgramMergeAutomationEvidenceNeeds(execution.EvidenceNeeds, actionEvidenceNeeds)
	planExecution := *execution
	planExecution.EvidenceNeeds = planEvidenceNeeds
	if len(responsibilities) > 0 {
		planExecution = workProgramAutomationPlanExecutionWithResponsibilityBlockers(planExecution)
	}
	queues := workProgramAutomationPlanQueuesFor(&planExecution)
	planState := workProgramAutomationPlanStateWithResponsibilityCount(&planExecution, queues, len(responsibilities))
	autonomyLevel := workProgramAutomationPlanAutonomyLevel(planState)
	autonomousActionReady := workProgramAutomationPlanAutonomyAllowed(&planExecution)
	humanReviewRequired := workProgramAutomationPlanHumanReviewRequiredWithResponsibilityCount(&planExecution, queues, autonomousActionReady, len(responsibilities))
	safeAutomationAreas, humanRequiredAreas := workProgramAutomationPlanAreas(&planExecution)
	if len(responsibilities) > 0 {
		humanRequiredAreas = append(humanRequiredAreas, "responsibility_validation")
	}
	blockedAreas := workProgramAutomationPlanBlockedAreas(&planExecution, humanRequiredAreas)
	recommendedFocus := workProgramAutomationPlanFocus(&planExecution, queues)
	if recommendedFocus == nil && len(responsibilities) > 0 {
		recommendedFocus = workProgramAutomationPlanResponsibilityFocus(responsibilities)
	}
	evidenceNeedCount := workProgramAutomationPlanEvidenceNeedCount(&planExecution, planEvidenceNeeds)
	actionPlans := workProgramAutomationActionPlansWithEvidence(&planExecution, blockedAreas, planEvidenceNeeds)
	return &model.WorkProgramAutomationPlanPacket{
		SourceInstance:                execution.SourceInstance,
		GeneratedAt:                   workProgramAutomationPlanGeneratedAt(&planExecution),
		WorkstreamKey:                 execution.WorkstreamKey,
		ActionState:                   execution.ActionState,
		PlanState:                     planState,
		AutonomyLevel:                 autonomyLevel,
		AutonomousActionReady:         autonomousActionReady,
		HumanReviewRequired:           humanReviewRequired,
		AbsenceClaimsAllowed:          execution.AbsenceClaimsAllowed,
		SafeAutomationAreas:           safeAutomationAreas,
		HumanRequiredAreas:            humanRequiredAreas,
		BlockedAutomationAreas:        blockedAreas,
		AutonomousActionCount:         len(queues.autonomousActions),
		HumanReviewActionCount:        len(queues.humanReviewActions),
		BlockedActionCount:            len(queues.blockedActions),
		EvidenceNeedCount:             evidenceNeedCount,
		ResponsibilityValidationCount: len(responsibilities),
		ReadyFunctionCount:            len(queues.safeFunctions),
		SupervisedFunctionCount:       len(queues.supervisedFunctions),
		BlockedFunctionCount:          len(queues.blockedFunctions),
		AutomationSummary:             workProgramAutomationPlanSummaryWithEvidenceAndResponsibilityCounts(&planExecution, planState, autonomyLevel, queues, blockedAreas, recommendedFocus, evidenceNeedCount, len(responsibilities)),
		RecommendedFocus:              recommendedFocus,
		ExecutionPacket:               &planExecution,
		AutonomousActions:             queues.autonomousActions,
		HumanReviewActions:            queues.humanReviewActions,
		BlockedActions:                queues.blockedActions,
		SafeFunctions:                 queues.safeFunctions,
		SupervisedFunctions:           queues.supervisedFunctions,
		BlockedFunctions:              queues.blockedFunctions,
		EvidenceNeeds:                 execution.EvidenceNeeds,
		Responsibilities:              responsibilities,
		ActionPlans:                   actionPlans,
	}, nil
}

func workProgramAutomationPlanExecutionWithResponsibilityBlockers(execution model.WorkProgramExecutionPacket) model.WorkProgramExecutionPacket {
	execution.AutonomousActionReady = false
	execution.HumanReviewRequired = true
	if strings.TrimSpace(execution.ExecutionState) == "autonomous_actions_ready" || strings.TrimSpace(execution.ExecutionState) == "" {
		execution.ExecutionState = "human_review_required"
	}
	if execution.TpmReadiness != nil {
		readiness := *execution.TpmReadiness
		readiness.AutonomousActionReady = false
		readiness.HumanReviewRequired = true
		if readiness.ReplacementState == "autonomous_ready" {
			readiness.ReplacementState = "human_review_required"
		}
		execution.TpmReadiness = &readiness
	}
	return execution
}

func workProgramAutomationPlanQueuesFor(execution *model.WorkProgramExecutionPacket) workProgramAutomationPlanQueues {
	var queues workProgramAutomationPlanQueues
	if execution == nil {
		return queues
	}
	autonomyAllowed := workProgramAutomationPlanAutonomyAllowed(execution)
	for _, function := range workProgramAutomationPlanFunctionRows(execution) {
		if function == nil {
			continue
		}
		switch {
		case strings.TrimSpace(function.ReadinessState) == "automatable" && !function.HumanRequired:
			queues.safeFunctions = append(queues.safeFunctions, function)
		case strings.TrimSpace(function.ReadinessState) == "blocked":
			queues.blockedFunctions = append(queues.blockedFunctions, function)
		default:
			queues.supervisedFunctions = append(queues.supervisedFunctions, function)
		}
	}
	for _, action := range execution.Actions {
		if action == nil {
			continue
		}
		switch {
		case autonomyAllowed && strings.TrimSpace(action.DecisionState) == "product_action":
			queues.autonomousActions = append(queues.autonomousActions, action)
		case strings.HasPrefix(execution.ExecutionState, "blocked"):
			queues.blockedActions = append(queues.blockedActions, action)
		default:
			queues.humanReviewActions = append(queues.humanReviewActions, action)
		}
	}
	return queues
}

func workProgramAutomationPlanAutonomyAllowed(execution *model.WorkProgramExecutionPacket) bool {
	if execution == nil || !execution.AutonomousActionReady || execution.HumanReviewRequired || !execution.AbsenceClaimsAllowed || len(execution.EvidenceNeeds) > 0 {
		return false
	}
	readiness := execution.TpmReadiness
	if readiness == nil {
		return false
	}
	if readiness.ReplacementState != "autonomous_ready" || !readiness.AutonomousActionReady || readiness.HumanReviewRequired || !readiness.AbsenceClaimsAllowed {
		return false
	}
	if readiness.BlockingGateCount > 0 || readiness.FailedCheckCount > 0 || readiness.EvidenceNeedCount > 0 {
		return false
	}
	if readiness.BlockedFunctionCount > 0 || readiness.SupervisedFunctionCount > 0 || readiness.HumanRequiredFunctionCount > 0 {
		return false
	}
	if readiness.MeasurementState != "product_action_ready" || !readiness.MeasurementProductActionReady {
		return false
	}
	if readiness.ForecastReadinessState != "ready" || !readiness.ForecastEtaReady {
		return false
	}
	return true
}

func workProgramAutomationPlanHumanReviewRequired(execution *model.WorkProgramExecutionPacket, queues workProgramAutomationPlanQueues, autonomyAllowed bool) bool {
	return workProgramAutomationPlanHumanReviewRequiredWithResponsibilityCount(execution, queues, autonomyAllowed, 0)
}

func workProgramAutomationPlanHumanReviewRequiredWithResponsibilityCount(execution *model.WorkProgramExecutionPacket, queues workProgramAutomationPlanQueues, autonomyAllowed bool, responsibilityValidationCount int) bool {
	if execution == nil {
		return true
	}
	if responsibilityValidationCount > 0 {
		return true
	}
	if len(execution.Actions) == 0 && len(execution.EvidenceNeeds) == 0 {
		return false
	}
	if autonomyAllowed && len(queues.humanReviewActions) == 0 && len(queues.blockedActions) == 0 {
		return false
	}
	return true
}

func workProgramAutomationPlanGeneratedAt(execution *model.WorkProgramExecutionPacket) *string {
	if execution == nil || execution.TpmReadiness == nil {
		return nil
	}
	return execution.TpmReadiness.GeneratedAt
}

func workProgramAutomationPlanFunctionRows(execution *model.WorkProgramExecutionPacket) []*model.WorkProgramTpmFunctionReadiness {
	if execution == nil || execution.TpmReadiness == nil {
		return []*model.WorkProgramTpmFunctionReadiness{}
	}
	return execution.TpmReadiness.TpmFunctionReadiness
}

func workProgramAutomationPlanState(execution *model.WorkProgramExecutionPacket, queues workProgramAutomationPlanQueues) string {
	return workProgramAutomationPlanStateWithResponsibilityCount(execution, queues, 0)
}

func workProgramAutomationPlanStateWithResponsibilityCount(execution *model.WorkProgramExecutionPacket, queues workProgramAutomationPlanQueues, responsibilityValidationCount int) string {
	if execution == nil {
		return "review_required"
	}
	if execution.ExecutionState == "blocked_source_coverage" {
		return "blocked_source_coverage"
	}
	if strings.HasPrefix(execution.ExecutionState, "blocked") {
		return "blocked"
	}
	if responsibilityValidationCount > 0 {
		return "human_review_required"
	}
	if len(execution.Actions) == 0 && len(execution.EvidenceNeeds) == 0 {
		return "no_open_work"
	}
	if len(queues.autonomousActions) > 0 {
		return "autonomous_actions_ready"
	}
	if execution.HumanReviewRequired || len(queues.humanReviewActions) > 0 || len(execution.EvidenceNeeds) > 0 {
		return "human_review_required"
	}
	return "review_required"
}

func workProgramAutomationPlanAutonomyLevel(planState string) string {
	switch planState {
	case "autonomous_actions_ready":
		return "autonomous"
	case "blocked_source_coverage", "blocked":
		return "blocked"
	case "human_review_required":
		return "supervised"
	case "no_open_work":
		return "idle"
	default:
		return "review_required"
	}
}

func workProgramAutomationPlanAreas(execution *model.WorkProgramExecutionPacket) ([]string, []string) {
	if execution == nil || execution.TpmReadiness == nil || execution.TpmReadiness.AutomationReadiness == nil {
		return []string{}, []string{}
	}
	readiness := execution.TpmReadiness.AutomationReadiness
	return workProgramUniqueStrings(readiness.SafeAutomationAreas), workProgramUniqueStrings(readiness.HumanRequiredAreas)
}

func workProgramAutomationPlanBlockedAreas(execution *model.WorkProgramExecutionPacket, humanRequiredAreas []string) []string {
	areas := append([]string{}, humanRequiredAreas...)
	if execution == nil {
		return workProgramUniqueStrings(areas)
	}
	if !execution.AbsenceClaimsAllowed {
		areas = append(areas, "source_coverage")
	}
	if execution.TpmReadiness != nil {
		if execution.TpmReadiness.BlockingGateCount > 0 {
			areas = append(areas, "guardrail_gates")
		}
		if execution.TpmReadiness.FailedCheckCount > 0 {
			areas = append(areas, "adversarial_checks")
		}
		if execution.TpmReadiness.MeasurementState != "product_action_ready" {
			areas = append(areas, "insight_measurement")
		}
		if execution.TpmReadiness.ForecastReadinessState != "ready" || !execution.TpmReadiness.ForecastEtaReady {
			areas = append(areas, "forecast_readiness")
		}
	}
	return workProgramUniqueStrings(areas)
}

func workProgramAutomationPlanFocus(execution *model.WorkProgramExecutionPacket, queues workProgramAutomationPlanQueues) *string {
	if execution != nil && execution.RecommendedFocus != nil {
		if value := optionalTrimmedPointerValue(execution.RecommendedFocus); value != nil {
			return value
		}
	}
	for _, function := range queues.blockedFunctions {
		if function != nil {
			if value := optionalTrimmedPointer(function.RecommendedAction); value != nil {
				return value
			}
		}
	}
	if execution != nil {
		for _, need := range execution.EvidenceNeeds {
			if need != nil {
				if value := optionalTrimmedPointer(need.RecommendedAction); value != nil {
					return value
				}
			}
		}
	}
	return nil
}

func workProgramAutomationPlanSummary(execution *model.WorkProgramExecutionPacket, planState string, autonomyLevel string, queues workProgramAutomationPlanQueues, blockedAreas []string, recommendedFocus *string) string {
	return workProgramAutomationPlanSummaryWithEvidenceNeedCount(execution, planState, autonomyLevel, queues, blockedAreas, recommendedFocus, workProgramAutomationPlanEvidenceNeedCount(execution, nil))
}

func workProgramAutomationPlanSummaryWithEvidenceNeedCount(execution *model.WorkProgramExecutionPacket, planState string, autonomyLevel string, queues workProgramAutomationPlanQueues, blockedAreas []string, recommendedFocus *string, evidenceNeedCount int) string {
	return workProgramAutomationPlanSummaryWithEvidenceAndResponsibilityCounts(execution, planState, autonomyLevel, queues, blockedAreas, recommendedFocus, evidenceNeedCount, 0)
}

func workProgramAutomationPlanSummaryWithEvidenceAndResponsibilityCounts(execution *model.WorkProgramExecutionPacket, planState string, autonomyLevel string, queues workProgramAutomationPlanQueues, blockedAreas []string, recommendedFocus *string, evidenceNeedCount int, responsibilityValidationCount int) string {
	workstreamKey := ""
	actionState := ""
	if execution != nil {
		workstreamKey = execution.WorkstreamKey
		actionState = execution.ActionState
	}
	summary := fmt.Sprintf("%s automation plan is %s with %s autonomy for %s actions. %d autonomous action(s), %d human-review action(s), %d blocked action(s). %d safe function(s), %d supervised function(s), %d blocked function(s). %d evidence need(s). %d responsibility validation(s). Blocked areas: %s.", workstreamKey, planState, autonomyLevel, actionState, len(queues.autonomousActions), len(queues.humanReviewActions), len(queues.blockedActions), len(queues.safeFunctions), len(queues.supervisedFunctions), len(queues.blockedFunctions), evidenceNeedCount, responsibilityValidationCount, workProgramAutomationPlanAreaSummary(blockedAreas))
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}

func workProgramAutomationPlanResponsibilityFocus(responsibilities []*model.WorkResponsibility) *string {
	for _, responsibility := range responsibilities {
		if responsibility == nil {
			continue
		}
		value := workProgramAttentionResponsibilityRecommendedAction(responsibility)
		if strings.TrimSpace(value) != "" {
			return optionalString(value)
		}
	}
	return nil
}

func workProgramAutomationPlanEvidenceNeedCount(execution *model.WorkProgramExecutionPacket, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed) int {
	count := len(evidenceNeeds)
	if execution != nil {
		if len(execution.EvidenceNeeds) > count {
			count = len(execution.EvidenceNeeds)
		}
		if execution.TpmReadiness != nil && execution.TpmReadiness.EvidenceNeedCount > count {
			count = execution.TpmReadiness.EvidenceNeedCount
		}
	}
	return count
}

func workProgramAutomationPlanAreaSummary(areas []string) string {
	if len(areas) == 0 {
		return "none"
	}
	return strings.Join(areas, ", ")
}

func workProgramAutomationActionPlans(execution *model.WorkProgramExecutionPacket, blockedAreas []string) []*model.WorkProgramAutomationActionPlan {
	evidenceNeeds := []*model.WorkProgramAutomationEvidenceNeed{}
	if execution != nil {
		evidenceNeeds = execution.EvidenceNeeds
	}
	return workProgramAutomationActionPlansWithEvidence(execution, blockedAreas, evidenceNeeds)
}

func workProgramAutomationActionPlansWithEvidence(execution *model.WorkProgramExecutionPacket, blockedAreas []string, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed) []*model.WorkProgramAutomationActionPlan {
	if execution == nil {
		return []*model.WorkProgramAutomationActionPlan{}
	}
	out := make([]*model.WorkProgramAutomationActionPlan, 0, len(execution.Actions))
	for _, action := range execution.Actions {
		if action == nil {
			continue
		}
		disposition := workProgramAutomationActionDisposition(execution, action)
		actionBlockedAreas := workProgramAutomationActionBlockingAreas(execution, action, disposition, blockedAreas)
		actionEvidenceNeeds := workProgramAutomationActionEvidenceNeeds(evidenceNeeds, action, actionBlockedAreas)
		out = append(out, &model.WorkProgramAutomationActionPlan{
			Action:            action,
			Disposition:       disposition,
			AutonomyLevel:     workProgramAutomationActionAutonomyLevel(disposition),
			Reason:            workProgramAutomationActionReason(execution, action, disposition, actionBlockedAreas),
			BlockingAreas:     actionBlockedAreas,
			EvidenceNeeds:     actionEvidenceNeeds,
			RecommendedAction: workProgramAutomationActionRecommendedAction(action, actionEvidenceNeeds),
		})
	}
	return out
}

func (r *queryResolver) workProgramAutomationPlanActionEvidenceNeeds(ctx context.Context, execution *model.WorkProgramExecutionPacket) ([]*model.WorkProgramAutomationEvidenceNeed, error) {
	if execution == nil {
		return []*model.WorkProgramAutomationEvidenceNeed{}, nil
	}
	sourceFilter := execution.SourceInstance
	workstreamFilter := &execution.WorkstreamKey
	generatedAt := workProgramAutomationPlanExecutionGeneratedAtFilter(execution)
	out := []*model.WorkProgramAutomationEvidenceNeed{}
	seen := map[string]bool{}
	for _, action := range execution.Actions {
		if action == nil {
			continue
		}
		if strings.TrimSpace(action.Key) != "" {
			actionKey := strings.TrimSpace(action.Key)
			rows, err := r.latestWorkProgramEvidenceNeedModelsForFilters(ctx, workProgramEvidenceNeedFilters{
				sourceFilter:  sourceFilter,
				workstreamKey: workstreamFilter,
				actionKey:     &actionKey,
				generatedAt:   generatedAt,
			}, 5)
			if err != nil {
				return nil, err
			}
			for _, need := range rows {
				out = appendWorkProgramAutomationEvidenceNeed(out, seen, need)
			}
		}
		if strings.TrimSpace(action.SubjectKey) != "" {
			targetKey := strings.TrimSpace(action.SubjectKey)
			rows, err := r.latestWorkProgramEvidenceNeedModelsForFilters(ctx, workProgramEvidenceNeedFilters{
				sourceFilter:  sourceFilter,
				workstreamKey: workstreamFilter,
				targetKey:     &targetKey,
				generatedAt:   generatedAt,
			}, 5)
			if err != nil {
				return nil, err
			}
			for _, need := range rows {
				out = appendWorkProgramAutomationEvidenceNeed(out, seen, need)
			}
		}
	}
	return out, nil
}

func workProgramAutomationPlanExecutionGeneratedAtFilter(execution *model.WorkProgramExecutionPacket) *time.Time {
	if execution == nil || execution.GeneratedAt == nil {
		return nil
	}
	value := strings.TrimSpace(*execution.GeneratedAt)
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func workProgramMergeAutomationEvidenceNeeds(primary []*model.WorkProgramAutomationEvidenceNeed, extra []*model.WorkProgramAutomationEvidenceNeed) []*model.WorkProgramAutomationEvidenceNeed {
	out := make([]*model.WorkProgramAutomationEvidenceNeed, 0, len(primary)+len(extra))
	seen := map[string]bool{}
	for _, need := range primary {
		out = appendWorkProgramAutomationEvidenceNeed(out, seen, need)
	}
	for _, need := range extra {
		out = appendWorkProgramAutomationEvidenceNeed(out, seen, need)
	}
	return out
}

func workProgramAutomationActionDisposition(execution *model.WorkProgramExecutionPacket, action *model.WorkAction) string {
	if execution == nil || action == nil {
		return "human_review"
	}
	if workProgramAutomationPlanAutonomyAllowed(execution) && strings.TrimSpace(action.DecisionState) == "product_action" {
		return "autonomous"
	}
	if strings.HasPrefix(execution.ExecutionState, "blocked") {
		return "blocked"
	}
	return "human_review"
}

func workProgramAutomationActionAutonomyLevel(disposition string) string {
	switch disposition {
	case "autonomous":
		return "autonomous"
	case "blocked":
		return "blocked"
	default:
		return "supervised"
	}
}

func workProgramAutomationActionBlockingAreas(execution *model.WorkProgramExecutionPacket, action *model.WorkAction, disposition string, blockedAreas []string) []string {
	if disposition == "autonomous" {
		return []string{}
	}
	if disposition == "blocked" {
		return workProgramUniqueStrings(blockedAreas)
	}
	areas := []string{}
	if action != nil && strings.TrimSpace(action.DecisionState) != "product_action" {
		areas = append(areas, "human_review")
	}
	if execution == nil {
		return workProgramUniqueStrings(areas)
	}
	if !execution.AbsenceClaimsAllowed {
		areas = append(areas, "source_coverage")
	}
	readiness := execution.TpmReadiness
	if readiness != nil {
		if readiness.BlockingGateCount > 0 || readiness.FailedCheckCount > 0 || readiness.EvidenceNeedCount > 0 {
			areas = append(areas, "guardrail_gates")
		}
		if readiness.BlockedFunctionCount > 0 || readiness.SupervisedFunctionCount > 0 || readiness.HumanRequiredFunctionCount > 0 {
			areas = append(areas, "function_readiness")
		}
		if readiness.MeasurementState != "product_action_ready" {
			areas = append(areas, "insight_measurement")
		}
		if readiness.ForecastReadinessState != "ready" || !readiness.ForecastEtaReady {
			areas = append(areas, "forecast_readiness")
		}
	}
	if len(execution.EvidenceNeeds) > 0 {
		areas = append(areas, "evidence_needs")
	}
	if len(areas) == 0 {
		areas = append(areas, "human_review")
	}
	return workProgramUniqueStrings(areas)
}

func workProgramAutomationActionReason(execution *model.WorkProgramExecutionPacket, action *model.WorkAction, disposition string, blockingAreas []string) string {
	switch disposition {
	case "autonomous":
		return "All automation gates are clear for this product action."
	case "blocked":
		return workProgramAutomationGateReason(execution, blockingAreas, "blocked")
	default:
		if action != nil && strings.TrimSpace(action.DecisionState) != "product_action" {
			return fmt.Sprintf("Decision state %q requires human review before automation.", strings.TrimSpace(action.DecisionState))
		}
		return workProgramAutomationGateReason(execution, blockingAreas, "requires human review")
	}
}

func workProgramAutomationGateReason(execution *model.WorkProgramExecutionPacket, blockingAreas []string, fallback string) string {
	if execution == nil {
		return "TPM execution readiness is unavailable."
	}
	if !execution.AbsenceClaimsAllowed {
		return "Source coverage is limited or unknown, so absence, closure, and product-action claims are blocked."
	}
	readiness := execution.TpmReadiness
	if readiness == nil {
		return "TPM readiness is unavailable."
	}
	if readiness.BlockingGateCount > 0 || readiness.FailedCheckCount > 0 {
		return "Guardrail checks or blocking quality gates must clear before automation."
	}
	if readiness.BlockedFunctionCount > 0 || readiness.HumanRequiredFunctionCount > 0 {
		return "TPM function readiness still requires human repair or supervision."
	}
	if readiness.MeasurementState != "product_action_ready" || !readiness.MeasurementProductActionReady {
		return "Insight measurement is not product-action ready."
	}
	if readiness.ForecastReadinessState != "ready" || !readiness.ForecastEtaReady {
		return "Forecast output is risk triage only until ETA readiness passes."
	}
	if len(execution.EvidenceNeeds) > 0 || readiness.EvidenceNeedCount > 0 {
		return "Open evidence needs must be resolved before automation."
	}
	if len(blockingAreas) > 0 {
		return fmt.Sprintf("Automation %s while %s remains gated.", fallback, workProgramAutomationPlanAreaSummary(blockingAreas))
	}
	return "Human review is required before automation."
}

func workProgramAutomationActionEvidenceNeeds(evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed, action *model.WorkAction, blockingAreas []string) []*model.WorkProgramAutomationEvidenceNeed {
	if action == nil {
		return []*model.WorkProgramAutomationEvidenceNeed{}
	}
	out := []*model.WorkProgramAutomationEvidenceNeed{}
	seen := map[string]bool{}
	for _, need := range evidenceNeeds {
		if workProgramAutomationEvidenceNeedMatchesAction(need, action, blockingAreas, false) {
			out = appendWorkProgramAutomationEvidenceNeed(out, seen, need)
		}
	}
	if len(out) == 0 {
		for _, need := range evidenceNeeds {
			if workProgramAutomationEvidenceNeedMatchesAction(need, action, blockingAreas, true) {
				out = appendWorkProgramAutomationEvidenceNeed(out, seen, need)
			}
			if len(out) >= 3 {
				break
			}
		}
	}
	return out
}

func workProgramAutomationEvidenceNeedMatchesAction(need *model.WorkProgramAutomationEvidenceNeed, action *model.WorkAction, blockingAreas []string, allowGlobal bool) bool {
	if need == nil || action == nil {
		return false
	}
	if need.ActionKey != nil && strings.TrimSpace(*need.ActionKey) == action.Key {
		return true
	}
	if need.TargetKey != nil {
		targetKey := strings.TrimSpace(*need.TargetKey)
		if targetKey == action.SubjectKey || targetKey == action.Key {
			return true
		}
	}
	if need.ActionKey == nil && need.TargetKey == nil && need.ActionState != nil && strings.TrimSpace(*need.ActionState) != "" && strings.TrimSpace(*need.ActionState) == action.ActionState {
		return true
	}
	if !allowGlobal {
		return false
	}
	if need.ActionKey != nil || need.TargetKey != nil {
		return false
	}
	return workProgramStringInSet(strings.TrimSpace(need.GateKey), blockingAreas)
}

func appendWorkProgramAutomationEvidenceNeed(out []*model.WorkProgramAutomationEvidenceNeed, seen map[string]bool, need *model.WorkProgramAutomationEvidenceNeed) []*model.WorkProgramAutomationEvidenceNeed {
	if need == nil || seen[need.Key] {
		return out
	}
	seen[need.Key] = true
	return append(out, need)
}

func workProgramAutomationActionRecommendedAction(action *model.WorkAction, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed) *string {
	if action != nil {
		if value := optionalTrimmedPointerValue(action.RecommendedAction); value != nil {
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
	return nil
}

func workProgramStringInSet(value string, set []string) bool {
	for _, item := range set {
		if value != "" && value == item {
			return true
		}
	}
	return false
}
