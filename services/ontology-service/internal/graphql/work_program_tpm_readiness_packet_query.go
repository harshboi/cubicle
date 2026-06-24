package graphql

import (
	"context"
	"fmt"
	"strings"

	"cubicle/services/ontology-service/internal/graphql/model"
)

func (r *queryResolver) workProgramTpmReadinessPacket(ctx context.Context, workstreamKey string, functionLimit *int, evidenceLimit *int, reviewLimit *int, sourceInstance *string) (*model.WorkProgramTpmReadinessPacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramTpmReadinessPacket requires an Ent-backed ontology store")
	}
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil, fmt.Errorf("workProgramTpmReadinessPacket requires workstreamKey")
	}
	functionRowLimit := boundedLimit(functionLimit, 20, 100)
	evidenceRowLimit := boundedLimit(evidenceLimit, 50, 200)
	sourceFilter, err := r.workProgramTPMReadinessSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	runGeneratedAt, err := r.latestWorkProgramAutomationReadinessRunGeneratedAt(ctx, sourceFilter, &workstreamKey)
	if err != nil {
		return nil, err
	}

	guardrail, err := r.workProgramGuardrailPacket(ctx, workstreamKey, nil, &evidenceRowLimit, sourceFilter)
	if err != nil {
		return nil, err
	}
	sourceFilter = guardrail.SourceInstance
	sourceCoverage, err := r.workProgramSourceCoveragePacket(ctx, workstreamKey, nil, &evidenceRowLimit, sourceFilter)
	if err != nil {
		return nil, err
	}
	measurement, err := r.workInsightMeasurementPacket(ctx, sourceFilter, reviewLimit, nil)
	if err != nil {
		return nil, err
	}
	workstreamFilter := &workstreamKey
	functionRows, err := r.latestWorkProgramTPMFunctionReadinessRowsForGeneratedAt(ctx, sourceFilter, workstreamFilter, runGeneratedAt)
	if err != nil {
		return nil, err
	}
	allFunctions := workProgramTPMFunctionReadinessModels(functionRows)
	functions := workProgramTPMFunctionReadinessModels(limitWorkProgramTPMFunctionReadinessRows(functionRows, functionRowLimit))
	forecastReadiness, err := r.forecastReadinessModel(ctx, sourceFilter, "open")
	if err != nil {
		return nil, err
	}
	decisionTargetReadiness, err := r.workDecisionTargetReadinessModel(ctx, sourceFilter, functionRowLimit)
	if err != nil {
		return nil, err
	}
	ciCheckCounts, err := r.latestWorkstreamHealthCheckCounts(ctx, sourceFilter, &workstreamKey)
	if err != nil {
		return nil, err
	}
	responsibilities, responsibilityValidationCount, err := r.workProgramAttentionResponsibilityModelsAndCount(ctx, sourceFilter, workstreamKey, evidenceRowLimit)
	if err != nil {
		return nil, err
	}

	functionCounts := workProgramTPMReadinessFunctionCounts(allFunctions)
	replacementState := workProgramTPMReplacementState(guardrail, measurement, sourceCoverage, forecastReadiness, decisionTargetReadiness, functionCounts)
	autonomousActionReady := replacementState == "autonomous_ready"
	humanReviewRequired := !autonomousActionReady || guardrail.HumanReviewRequired || sourceCoverage.HumanReviewRequired || workProgramTPMDecisionTargetHumanRequired(decisionTargetReadiness) || functionCounts.humanRequired > 0
	recommendedFocus := workProgramTPMReadinessFocus(allFunctions, guardrail, sourceCoverage, measurement, forecastReadiness, decisionTargetReadiness)
	if responsibilityValidationCount > 0 {
		responsibilityFocus := workProgramAutomationPlanResponsibilityFocus(responsibilities)
		if replacementState == "autonomous_ready" {
			replacementState = "human_review_required"
			if responsibilityFocus != nil {
				recommendedFocus = responsibilityFocus
			}
		}
		autonomousActionReady = false
		humanReviewRequired = true
		if recommendedFocus == nil {
			recommendedFocus = responsibilityFocus
		}
	}
	automationSummary := workProgramTPMReadinessSummary(workstreamKey, replacementState, functionCounts, guardrail, sourceCoverage, measurement, forecastReadiness, decisionTargetReadiness, ciCheckCounts, recommendedFocus)
	if responsibilityValidationCount > 0 {
		automationSummary = fmt.Sprintf("%s %d responsibility validation(s) require human review.", automationSummary, responsibilityValidationCount)
	}

	return &model.WorkProgramTpmReadinessPacket{
		SourceInstance:                            sourceFilter,
		WorkstreamKey:                             workstreamKey,
		GeneratedAt:                               firstNonNilString(optionalTimePtr(runGeneratedAt), measurement.GeneratedAt),
		ReplacementState:                          replacementState,
		AutonomousActionReady:                     autonomousActionReady,
		HumanReviewRequired:                       humanReviewRequired,
		TotalFunctionCount:                        functionCounts.total,
		ReadyFunctionCount:                        functionCounts.ready,
		SupervisedFunctionCount:                   functionCounts.supervised,
		BlockedFunctionCount:                      functionCounts.blocked,
		HumanRequiredFunctionCount:                functionCounts.humanRequired,
		BlockingGateCount:                         guardrail.BlockingGateCount,
		FailedCheckCount:                          guardrail.FailedCheckCount,
		FailedAdversarialCheckCount:               guardrail.FailedAdversarialCheckCount,
		FailingCheckPullRequestCount:              ciCheckCounts.failingCheckPullRequestCount,
		OpenFailingCheckPullRequestCount:          ciCheckCounts.openFailingCheckPullRequestCount,
		EvidenceNeedCount:                         guardrail.EvidenceNeedCount,
		MeasurementState:                          measurement.MeasurementState,
		MeasurementProductActionReady:             measurement.ProductActionReady,
		MeasurementGapCount:                       measurement.MeasurementGapCount,
		MeasurementMissingLabelCount:              measurement.MeasurementMissingLabelCount,
		ForecastReadinessState:                    forecastReadiness.ReadinessState,
		ForecastEtaReady:                          forecastReadiness.EtaForecastReady,
		DecisionTargetEvaluationState:             decisionTargetReadiness.EvaluationState,
		DecisionTargetEvaluationCount:             decisionTargetReadiness.EvaluationCount,
		ProductReadyDecisionTargetEvaluationCount: decisionTargetReadiness.ProductReadyEvaluationCount,
		SourceCoverageState:                       sourceCoverage.CoverageState,
		SourceCoverageLimitedCount:                sourceCoverage.LimitedItemCount,
		SourceCoverageUnknownCount:                sourceCoverage.UnknownItemCount,
		AbsenceClaimsAllowed:                      sourceCoverage.AbsenceClaimsAllowed,
		ResponsibilityValidationCount:             responsibilityValidationCount,
		AutomationSummary:                         automationSummary,
		RecommendedFocus:                          recommendedFocus,
		GuardrailPacket:                           guardrail,
		SourceCoveragePacket:                      sourceCoverage,
		MeasurementPacket:                         measurement,
		AutomationReadiness:                       guardrail.AutomationReadiness,
		TpmFunctionReadiness:                      functions,
		DecisionTargetReadiness:                   decisionTargetReadiness,
		DecisionTargetEvaluations:                 decisionTargetReadiness.Evaluations,
		QualityGates:                              guardrail.QualityGates,
		EvidenceNeeds:                             guardrail.EvidenceNeeds,
		Responsibilities:                          responsibilities,
	}, nil
}

type workProgramTPMReadinessFunctionCount struct {
	total         int
	ready         int
	supervised    int
	blocked       int
	humanRequired int
}

func workProgramTPMReadinessFunctionCounts(functions []*model.WorkProgramTpmFunctionReadiness) workProgramTPMReadinessFunctionCount {
	var out workProgramTPMReadinessFunctionCount
	for _, function := range functions {
		if function == nil {
			continue
		}
		out.total++
		if function.HumanRequired {
			out.humanRequired++
		}
		switch strings.TrimSpace(function.ReadinessState) {
		case "automatable":
			if !function.HumanRequired {
				out.ready++
			}
		case "supervised":
			out.supervised++
		case "blocked":
			out.blocked++
		}
	}
	return out
}

func (r *queryResolver) workProgramTPMReadinessSourceInstance(ctx context.Context, sourceInstance *string) (*string, error) {
	sourceFilter, err := r.aggregateSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	if sourceFilter != nil {
		return sourceFilter, nil
	}
	sourceFilter, _, err = r.insightMeasurementSourceInstance(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sourceFilter, nil
}

func workProgramTPMReplacementState(guardrail *model.WorkProgramGuardrailPacket, measurement *model.WorkInsightMeasurementPacket, sourceCoverage *model.WorkProgramSourceCoveragePacket, forecastReadiness *model.WorkForecastReadiness, decisionTargetReadiness *model.WorkDecisionTargetReadiness, counts workProgramTPMReadinessFunctionCount) string {
	if guardrail == nil || measurement == nil || sourceCoverage == nil {
		return "review_required"
	}
	if !sourceCoverage.AbsenceClaimsAllowed {
		return "blocked"
	}
	if guardrail.GuardrailState == "blocked" || counts.blocked > 0 || guardrail.FailedCheckCount > 0 || guardrail.BlockingGateCount > 0 {
		return "blocked"
	}
	if measurement.MeasurementState == "labeling_needed" || measurement.MeasurementState == "quality_gated" {
		return "measurement_gated"
	}
	if workProgramTPMDecisionTargetHumanRequired(decisionTargetReadiness) {
		return "measurement_gated"
	}
	if guardrail.GuardrailState == "human_review_required" || guardrail.EvidenceNeedCount > 0 {
		return "human_review_required"
	}
	if counts.total == 0 {
		return "review_required"
	}
	if forecastReadiness == nil || !forecastReadiness.EtaForecastReady {
		return "human_review_required"
	}
	if guardrail.AutonomousActionReady && measurement.ProductActionReady && counts.ready > 0 && counts.blocked == 0 && counts.humanRequired == 0 {
		return "autonomous_ready"
	}
	if counts.supervised > 0 || counts.humanRequired > 0 || guardrail.HumanReviewRequired {
		return "human_review_required"
	}
	if counts.ready > 0 {
		return "supervised_ready"
	}
	return "review_required"
}

func workProgramTPMDecisionTargetHumanRequired(decisionTargetReadiness *model.WorkDecisionTargetReadiness) bool {
	return decisionTargetReadiness != nil && decisionTargetReadiness.EvaluationCount > 0 && !decisionTargetReadiness.ProductActionReady
}

func workProgramTPMReadinessFocus(functions []*model.WorkProgramTpmFunctionReadiness, guardrail *model.WorkProgramGuardrailPacket, sourceCoverage *model.WorkProgramSourceCoveragePacket, measurement *model.WorkInsightMeasurementPacket, forecastReadiness *model.WorkForecastReadiness, decisionTargetReadiness *model.WorkDecisionTargetReadiness) *string {
	for _, function := range functions {
		if function != nil && strings.TrimSpace(function.ReadinessState) == "blocked" {
			if value := optionalTrimmedPointer(function.RecommendedAction); value != nil {
				return value
			}
		}
	}
	for _, function := range functions {
		if function != nil && function.HumanRequired {
			if value := optionalTrimmedPointer(function.RecommendedAction); value != nil {
				return value
			}
		}
	}
	if sourceCoverage != nil && !sourceCoverage.AbsenceClaimsAllowed && sourceCoverage.RecommendedFocus != nil {
		if value := optionalTrimmedPointerValue(sourceCoverage.RecommendedFocus); value != nil {
			return value
		}
	}
	if guardrail != nil && guardrail.RecommendedFocus != nil {
		if value := optionalTrimmedPointerValue(guardrail.RecommendedFocus); value != nil {
			return value
		}
	}
	if decisionTargetReadiness != nil && workProgramTPMDecisionTargetHumanRequired(decisionTargetReadiness) && decisionTargetReadiness.RecommendedFocus != nil {
		if value := optionalTrimmedPointerValue(decisionTargetReadiness.RecommendedFocus); value != nil {
			return value
		}
	}
	if forecastReadiness != nil && !forecastReadiness.EtaForecastReady && forecastReadiness.ReadinessReason != nil {
		if value := optionalTrimmedPointerValue(forecastReadiness.ReadinessReason); value != nil {
			return value
		}
	}
	if measurement != nil && measurement.RecommendedFocus != nil {
		if value := optionalTrimmedPointerValue(measurement.RecommendedFocus); value != nil {
			return value
		}
	}
	if decisionTargetReadiness != nil && decisionTargetReadiness.RecommendedFocus != nil {
		if value := optionalTrimmedPointerValue(decisionTargetReadiness.RecommendedFocus); value != nil {
			return value
		}
	}
	if forecastReadiness != nil && forecastReadiness.ReadinessReason != nil {
		if value := optionalTrimmedPointerValue(forecastReadiness.ReadinessReason); value != nil {
			return value
		}
	}
	return nil
}

func workProgramTPMReadinessSummary(workstreamKey string, replacementState string, counts workProgramTPMReadinessFunctionCount, guardrail *model.WorkProgramGuardrailPacket, sourceCoverage *model.WorkProgramSourceCoveragePacket, measurement *model.WorkInsightMeasurementPacket, forecastReadiness *model.WorkForecastReadiness, decisionTargetReadiness *model.WorkDecisionTargetReadiness, ciCheckCounts workstreamHealthCheckCounts, recommendedFocus *string) string {
	guardrailState := "unknown"
	blockingGateCount := 0
	failedCheckCount := 0
	evidenceNeedCount := 0
	if guardrail != nil {
		guardrailState = guardrail.GuardrailState
		blockingGateCount = guardrail.BlockingGateCount
		failedCheckCount = guardrail.FailedCheckCount
		evidenceNeedCount = guardrail.EvidenceNeedCount
	}
	measurementState := "unknown"
	measurementGapCount := 0
	measurementMissingLabelCount := 0
	if measurement != nil {
		measurementState = measurement.MeasurementState
		measurementGapCount = measurement.MeasurementGapCount
		measurementMissingLabelCount = measurement.MeasurementMissingLabelCount
	}
	sourceCoverageState := "unknown"
	sourceCoverageLimitedCount := 0
	sourceCoverageUnknownCount := 0
	absenceClaimsAllowed := false
	if sourceCoverage != nil {
		sourceCoverageState = sourceCoverage.CoverageState
		sourceCoverageLimitedCount = sourceCoverage.LimitedItemCount
		sourceCoverageUnknownCount = sourceCoverage.UnknownItemCount
		absenceClaimsAllowed = sourceCoverage.AbsenceClaimsAllowed
	}
	forecastState := "unknown"
	etaReady := false
	if forecastReadiness != nil {
		forecastState = forecastReadiness.ReadinessState
		etaReady = forecastReadiness.EtaForecastReady
	}
	decisionTargetState := "missing"
	decisionTargetCount := 0
	productReadyDecisionTargetCount := 0
	if decisionTargetReadiness != nil {
		decisionTargetState = decisionTargetReadiness.EvaluationState
		decisionTargetCount = decisionTargetReadiness.EvaluationCount
		productReadyDecisionTargetCount = decisionTargetReadiness.ProductReadyEvaluationCount
	}
	summary := fmt.Sprintf("%s AI-TPM replacement readiness is %s. %d automatable function(s), %d supervised function(s), %d blocked function(s), %d human-required function(s). Guardrails are %s with %d blocking gate(s), %d failed adversarial check(s), and %d evidence need(s). CI checks report %d failing PR(s), %d still open. Source coverage is %s with %d limited item(s), %d unknown item(s), and absence claims allowed: %t. Insight measurement is %s with %d gap kind(s) and %d missing measurement label(s). Forecast readiness is %s; ETA ready: %t. Decision-target evaluation is %s with %d row(s), %d product-ready.", workstreamKey, replacementState, counts.ready, counts.supervised, counts.blocked, counts.humanRequired, guardrailState, blockingGateCount, failedCheckCount, evidenceNeedCount, ciCheckCounts.failingCheckPullRequestCount, ciCheckCounts.openFailingCheckPullRequestCount, sourceCoverageState, sourceCoverageLimitedCount, sourceCoverageUnknownCount, absenceClaimsAllowed, measurementState, measurementGapCount, measurementMissingLabelCount, forecastState, etaReady, decisionTargetState, decisionTargetCount, productReadyDecisionTargetCount)
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}

func firstNonNilString(values ...*string) *string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return value
		}
	}
	return nil
}
