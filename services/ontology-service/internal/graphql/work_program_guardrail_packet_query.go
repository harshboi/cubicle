package graphql

import (
	"context"
	"fmt"
	"strings"

	"cubicle/services/ontology-service/internal/graphql/model"
)

func (r *queryResolver) workProgramGuardrailPacket(ctx context.Context, workstreamKey string, limit *int, evidenceLimit *int, sourceInstance *string) (*model.WorkProgramGuardrailPacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramGuardrailPacket requires an Ent-backed ontology store")
	}
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil, fmt.Errorf("workProgramGuardrailPacket requires workstreamKey")
	}
	sourceFilter, err := r.aggregateSourceInstance(ctx, sourceInstance)
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

	gates, blockingGateCount, err := r.latestWorkProgramQualityGateModelsAndBlockingCountForGeneratedAt(ctx, sourceFilter, workstreamFilter, runGeneratedAt, rowLimit)
	if err != nil {
		return nil, err
	}
	checks, checkCounts, err := r.latestWorkProgramAdversarialCheckModelsAndCountsForGeneratedAt(ctx, sourceFilter, workstreamFilter, runGeneratedAt, rowLimit)
	if err != nil {
		return nil, err
	}
	caveats, caveatCount, err := r.latestWorkProgramBriefCaveatModelsAndCountForGeneratedAt(ctx, sourceFilter, workstreamFilter, runGeneratedAt, rowLimit)
	if err != nil {
		return nil, err
	}
	evidenceNeeds, evidenceNeedCount, err := r.latestWorkProgramEvidenceNeedModelsAndCountForFilters(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  sourceFilter,
		workstreamKey: workstreamFilter,
		generatedAt:   runGeneratedAt,
	}, evidenceRowLimit)
	if err != nil {
		return nil, err
	}
	readiness, err := r.latestWorkProgramAutomationReadinessModel(ctx, sourceFilter, workstreamFilter, gates, evidenceNeeds)
	if err != nil {
		return nil, err
	}
	responsibilities, responsibilityValidationCount, err := r.workProgramAttentionResponsibilityModelsAndCount(ctx, sourceFilter, workstreamKey, rowLimit)
	if err != nil {
		return nil, err
	}

	failedCheckCount := checkCounts.failed
	warningCheckCount := checkCounts.warning
	readinessState, rawAutonomousActionReady, rawHumanReviewRequired := workProgramGuardrailReadiness(readiness)
	guardrailState := workProgramGuardrailState(rawAutonomousActionReady, rawHumanReviewRequired, blockingGateCount, failedCheckCount, warningCheckCount, caveatCount, evidenceNeedCount)
	autonomousActionReady := workProgramGuardrailAutonomousActionReady(guardrailState, rawAutonomousActionReady)
	humanReviewRequired := workProgramGuardrailHumanReviewRequired(guardrailState, rawHumanReviewRequired)
	recommendedFocus := workProgramGuardrailFocus(checks, gates, caveats, evidenceNeeds, readiness)
	if responsibilityValidationCount > 0 {
		responsibilityFocus := workProgramAutomationPlanResponsibilityFocus(responsibilities)
		if guardrailState == "autonomous_ready" {
			guardrailState = "human_review_required"
			if responsibilityFocus != nil {
				recommendedFocus = responsibilityFocus
			}
		}
		autonomousActionReady = false
		humanReviewRequired = true
		if recommendedFocus == nil {
			recommendedFocus = responsibilityFocus
		}
		readiness = workProgramGuardrailReadinessWithResponsibilityValidation(readiness, responsibilityValidationCount)
		readinessState, _, _ = workProgramGuardrailReadiness(readiness)
	}
	automationSummary := workProgramGuardrailSummary(workstreamKey, guardrailState, readinessState, blockingGateCount, failedCheckCount, warningCheckCount, evidenceNeedCount, recommendedFocus)
	if responsibilityValidationCount > 0 {
		automationSummary = fmt.Sprintf("%s %d responsibility validation(s) require human review.", automationSummary, responsibilityValidationCount)
	}

	return &model.WorkProgramGuardrailPacket{
		SourceInstance:                sourceFilter,
		GeneratedAt:                   optionalTimePtr(runGeneratedAt),
		WorkstreamKey:                 workstreamKey,
		GuardrailState:                guardrailState,
		ReadinessState:                readinessState,
		AutonomousActionReady:         autonomousActionReady,
		HumanReviewRequired:           humanReviewRequired,
		BlockingGateCount:             blockingGateCount,
		FailedCheckCount:              failedCheckCount,
		FailedAdversarialCheckCount:   failedCheckCount,
		WarningCheckCount:             warningCheckCount,
		WarningAdversarialCheckCount:  warningCheckCount,
		CaveatCount:                   caveatCount,
		EvidenceNeedCount:             evidenceNeedCount,
		ResponsibilityValidationCount: responsibilityValidationCount,
		AutomationSummary:             automationSummary,
		RecommendedFocus:              recommendedFocus,
		AutomationReadiness:           readiness,
		QualityGates:                  gates,
		AdversarialChecks:             checks,
		Caveats:                       caveats,
		EvidenceNeeds:                 evidenceNeeds,
		Responsibilities:              responsibilities,
	}, nil
}

func workProgramGuardrailBlockingGateCount(gates []*model.WorkProgramBriefQualityGate) int {
	count := 0
	for _, gate := range gates {
		if gate != nil && gate.Blocking {
			count++
		}
	}
	return count
}

func workProgramGuardrailCheckCounts(checks []*model.WorkProgramAdversarialCheck) (int, int) {
	failed := 0
	warning := 0
	for _, check := range checks {
		if check == nil {
			continue
		}
		switch strings.TrimSpace(check.CheckState) {
		case "fail":
			failed++
		case "warning":
			warning++
		}
	}
	return failed, warning
}

func workProgramGuardrailReadiness(readiness *model.WorkProgramAutomationReadiness) (string, bool, bool) {
	if readiness == nil {
		return "unknown", false, true
	}
	readinessState := strings.TrimSpace(readiness.ReadinessState)
	if readinessState == "" {
		readinessState = "unknown"
	}
	return readinessState, readiness.AutonomousActionReady, readiness.HumanReviewRequired
}

func workProgramGuardrailReadinessWithResponsibilityValidation(readiness *model.WorkProgramAutomationReadiness, responsibilityValidationCount int) *model.WorkProgramAutomationReadiness {
	if readiness == nil || responsibilityValidationCount <= 0 {
		return readiness
	}
	out := *readiness
	out.AutonomousActionReady = false
	out.HumanReviewRequired = true
	if strings.TrimSpace(out.ReadinessState) == "ready" || strings.TrimSpace(out.ReadinessState) == "autonomous_ready" {
		out.ReadinessState = "human_review_required"
	}
	out.HumanRequiredAreas = workProgramUniqueStrings(append(out.HumanRequiredAreas, "responsibility_validation"))
	out.RequiredEvidence = workProgramUniqueStrings(append(out.RequiredEvidence, "validated_accountable_owner"))
	responsibilityRationale := fmt.Sprintf("%d responsibility validation(s) require human review before autonomous execution.", responsibilityValidationCount)
	if strings.TrimSpace(out.Rationale) == "" {
		out.Rationale = responsibilityRationale
	} else if !strings.Contains(out.Rationale, "responsibility validation") {
		out.Rationale = strings.TrimSpace(out.Rationale) + " " + responsibilityRationale
	}
	return &out
}

func workProgramGuardrailState(autonomousActionReady bool, humanReviewRequired bool, blockingGateCount int, failedCheckCount int, warningCheckCount int, caveatCount int, evidenceNeedCount int) string {
	if failedCheckCount > 0 || blockingGateCount > 0 {
		return "blocked"
	}
	if humanReviewRequired || warningCheckCount > 0 || caveatCount > 0 || evidenceNeedCount > 0 {
		return "human_review_required"
	}
	if autonomousActionReady {
		return "autonomous_ready"
	}
	return "review_required"
}

func workProgramGuardrailAutonomousActionReady(guardrailState string, rawAutonomousActionReady bool) bool {
	return rawAutonomousActionReady && strings.TrimSpace(guardrailState) == "autonomous_ready"
}

func workProgramGuardrailHumanReviewRequired(guardrailState string, rawHumanReviewRequired bool) bool {
	return rawHumanReviewRequired || strings.TrimSpace(guardrailState) != "autonomous_ready"
}

func workProgramGuardrailFocus(
	checks []*model.WorkProgramAdversarialCheck,
	gates []*model.WorkProgramBriefQualityGate,
	caveats []*model.WorkProgramBriefCaveat,
	evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed,
	readiness *model.WorkProgramAutomationReadiness,
) *string {
	for _, check := range checks {
		if check != nil && strings.TrimSpace(check.CheckState) == "fail" {
			if value := optionalTrimmedPointer(check.RecommendedAction); value != nil {
				return value
			}
		}
	}
	for _, gate := range gates {
		if gate != nil && gate.Blocking {
			if value := optionalTrimmedPointerValue(gate.RecommendedAction); value != nil {
				return value
			}
		}
	}
	for _, check := range checks {
		if check != nil && strings.TrimSpace(check.CheckState) == "warning" {
			if value := optionalTrimmedPointer(check.RecommendedAction); value != nil {
				return value
			}
		}
	}
	for _, caveat := range caveats {
		if caveat != nil {
			if value := optionalTrimmedPointerValue(caveat.RecommendedAction); value != nil {
				return value
			}
		}
	}
	for _, need := range evidenceNeeds {
		if need != nil {
			if value := optionalTrimmedPointer(need.RecommendedAction); value != nil {
				return value
			}
		}
	}
	if readiness != nil {
		if value := optionalTrimmedPointer(readiness.Rationale); value != nil {
			return value
		}
	}
	return nil
}

func optionalTrimmedPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalTrimmedPointerValue(value *string) *string {
	if value == nil {
		return nil
	}
	return optionalTrimmedPointer(*value)
}

func workProgramGuardrailSummary(workstreamKey string, guardrailState string, readinessState string, blockingGateCount int, failedCheckCount int, warningCheckCount int, evidenceNeedCount int, recommendedFocus *string) string {
	summary := fmt.Sprintf("%s guardrails are %s; automation readiness is %s. %d blocking gate(s), %d failed adversarial check(s), %d warning adversarial check(s), %d evidence need(s).", workstreamKey, guardrailState, readinessState, blockingGateCount, failedCheckCount, warningCheckCount, evidenceNeedCount)
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}
