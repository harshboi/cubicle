package graphql

import (
	"strings"
	"testing"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkInsightKindEvaluationRequiresMeasuredQualityForProductAction(t *testing.T) {
	kind := workInsightKindEvaluationModel(&workInsightKindEvaluationValues{
		insightKind:               "blocker_candidate",
		currentInsightCount:       10,
		measurementLabelCount:     10,
		truthLabeledCount:         10,
		actionabilityLabeledCount: 10,
		truePositiveCount:         4,
		partialCount:              2,
		falsePositiveCount:        4,
		actionableCount:           4,
		needsOwnerCount:           2,
	})
	if !kind.ReadyToMeasure {
		t.Fatalf("expected measurement-ready kind: %#v", kind)
	}
	if kind.MeasurementScope != workInsightMeasurementScopeProductCandidate {
		t.Fatalf("measurement scope = %q, want product_candidate: %#v", kind.MeasurementScope, kind)
	}
	if kind.ReadyForProductAction {
		t.Fatalf("low precision kind should not be product-action ready: %#v", kind)
	}
	if kind.ProductActionGateState != "quality_gated" || !strings.Contains(kind.ProductActionGateReason, "precision") {
		t.Fatalf("unexpected quality gate reason: %#v", kind)
	}
	if kind.PrecisionRate != 0.4 || kind.UsefulSignalRate != 0.6 || kind.ActionabilityRate != 0.6 || kind.FalsePositiveRate != 0.4 || kind.MeasurementCoverageRate != 1 {
		t.Fatalf("unexpected measured rates: %#v", kind)
	}
}

func TestWorkInsightKindEvaluationKeepsDependencyClusterContextOnly(t *testing.T) {
	kind := workInsightKindEvaluationModel(&workInsightKindEvaluationValues{
		insightKind:               "dependency_cluster",
		currentInsightCount:       10,
		measurementLabelCount:     10,
		truthLabeledCount:         10,
		actionabilityLabeledCount: 10,
		truePositiveCount:         10,
		needsOwnerCount:           10,
	})
	if !kind.ReadyToMeasure {
		t.Fatalf("expected dependency cluster to be measurement-ready: %#v", kind)
	}
	if kind.MeasurementScope != workInsightMeasurementScopeContextOnly {
		t.Fatalf("measurement scope = %q, want context_only: %#v", kind.MeasurementScope, kind)
	}
	if kind.ReadyForProductAction {
		t.Fatalf("context-only dependency cluster should not be product-action ready: %#v", kind)
	}
	if kind.ProductActionGateState != "context_only" || !strings.Contains(kind.ProductActionGateReason, "cannot independently support product-action") {
		t.Fatalf("dependency cluster should expose context-only gate: %#v", kind)
	}
	if !strings.Contains(kind.RecommendedAction, "topology context") {
		t.Fatalf("dependency cluster action should preserve topology-context boundary: %q", kind.RecommendedAction)
	}
}

func TestWorkInsightKindMeasurementScopeClampsKnownContextKind(t *testing.T) {
	kind := &model.WorkInsightKindEvaluation{
		InsightKind:           "developer_correlation",
		MeasurementScope:      workInsightMeasurementScopeProductCandidate,
		ReadyToMeasure:        true,
		ReadyForProductAction: true,
	}
	if scope := workInsightKindMeasurementScope(kind); scope != workInsightMeasurementScopeContextOnly {
		t.Fatalf("known context-only kind scope = %q, want context_only", scope)
	}
}

func TestWorkInsightEvaluationDoesNotQualityGateContextOnlyKinds(t *testing.T) {
	evaluation := workInsightEvaluationModel(nil, []*genent.WorkInsight{
		testMeasuredInsight(workinsight.InsightKindStatusSummary, workinsightreview.TruthLabelTruePositive, workinsightreview.ActionabilityLabelActionable),
		testMeasuredInsight(workinsight.InsightKindDeveloperCorrelation, workinsightreview.TruthLabelPartial, workinsightreview.ActionabilityLabelNeedsOwner),
	})
	if evaluation.CurrentInsightCount != 1 || evaluation.MeasurementLabelCount != 1 {
		t.Fatalf("aggregate counts = current:%d labels:%d, want product-candidate scoped 1/1: %#v", evaluation.CurrentInsightCount, evaluation.MeasurementLabelCount, evaluation)
	}
	if evaluation.PrecisionRate != 1 || evaluation.UsefulSignalRate != 1 || evaluation.ActionabilityRate != 1 {
		t.Fatalf("aggregate rates should be product-candidate scoped, got %#v", evaluation)
	}
	if evaluation.ReadyInsightKindCount != 2 {
		t.Fatalf("ready kind count = %d, want 2: %#v", evaluation.ReadyInsightKindCount, evaluation)
	}
	if evaluation.ProductActionReadyKindCount != 1 {
		t.Fatalf("product-action-ready kind count = %d, want 1: %#v", evaluation.ProductActionReadyKindCount, evaluation)
	}
	if evaluation.QualityGatedInsightKindCount != 0 {
		t.Fatalf("context-only kind should not count as quality-gated product-action kind: %#v", evaluation)
	}
	if evaluation.GatedInsightKindCount != 0 {
		t.Fatalf("both fixture kinds should be measurement-ready: %#v", evaluation)
	}
	if !strings.Contains(evaluation.RecommendedNextStep, "kind-level product-action gates") {
		t.Fatalf("aggregate next step should point callers at kind-level product-action gates: %q", evaluation.RecommendedNextStep)
	}
}

func TestWorkInsightEvaluationRequiresGoldQualityOrHumanAssessment(t *testing.T) {
	evaluation := workInsightEvaluationModel(nil, []*genent.WorkInsight{
		{
			InsightKind: workinsight.InsightKindBlockerCandidate,
			Edges: genent.WorkInsightEdges{
				Reviews: []*genent.WorkInsightReview{
					{
						ReviewKind:          workinsightreview.ReviewKindEvaluationLabel,
						ReviewState:         workinsightreview.ReviewStateAccepted,
						TruthLabel:          workinsightreview.TruthLabelTruePositive,
						ActionabilityLabel:  workinsightreview.ActionabilityLabelNeedsOwner,
						LabelSet:            "source_oracle_seed",
						LabelQuality:        workinsightreview.LabelQualityCandidate,
						MeasurementEligible: true,
					},
					{
						ReviewKind:          workinsightreview.ReviewKindEvaluationLabel,
						ReviewState:         workinsightreview.ReviewStateAccepted,
						TruthLabel:          workinsightreview.TruthLabelTruePositive,
						ActionabilityLabel:  workinsightreview.ActionabilityLabelActionable,
						LabelSet:            "agent_adversarial",
						LabelQuality:        workinsightreview.LabelQualitySmoke,
						MeasurementEligible: true,
					},
					{
						ReviewKind:          workinsightreview.ReviewKindEvaluationLabel,
						ReviewState:         workinsightreview.ReviewStateAccepted,
						TruthLabel:          workinsightreview.TruthLabelTruePositive,
						ActionabilityLabel:  workinsightreview.ActionabilityLabelActionable,
						LabelSet:            "agent_gold",
						LabelQuality:        workinsightreview.LabelQualityGold,
						MeasurementEligible: false,
					},
				},
			},
		},
		testMeasuredInsight(workinsight.InsightKindBlockerCandidate, workinsightreview.TruthLabelTruePositive, workinsightreview.ActionabilityLabelNeedsOwner),
		{
			InsightKind: workinsight.InsightKindBlockerCandidate,
			Edges: genent.WorkInsightEdges{
				Reviews: []*genent.WorkInsightReview{
					{
						ReviewKind:          workinsightreview.ReviewKindHumanAssessment,
						ReviewState:         workinsightreview.ReviewStateAccepted,
						TruthLabel:          workinsightreview.TruthLabelPartial,
						ActionabilityLabel:  workinsightreview.ActionabilityLabelNeedsOwner,
						LabelQuality:        workinsightreview.LabelQualityUnknown,
						MeasurementEligible: true,
					},
				},
			},
		},
	})
	if evaluation.CurrentInsightCount != 3 {
		t.Fatalf("current insight count = %d, want 3: %#v", evaluation.CurrentInsightCount, evaluation)
	}
	if evaluation.MeasurementLabelCount != 2 || evaluation.OpenReviewRequestCount != 2 {
		t.Fatalf("measurement labels/open queue = %d/%d, want 2/2: %#v", evaluation.MeasurementLabelCount, evaluation.OpenReviewRequestCount, evaluation)
	}
	if len(evaluation.Kinds) != 1 {
		t.Fatalf("kind count = %d, want 1: %#v", len(evaluation.Kinds), evaluation)
	}
	kind := evaluation.Kinds[0]
	if kind.TruePositiveCount != 1 || kind.PartialCount != 1 || kind.NeedsOwnerCount != 2 {
		t.Fatalf("unexpected measurement-only counts: %#v", kind)
	}
}

func TestWorkInsightMeasurementReadyIgnoresContextOnlyGating(t *testing.T) {
	evaluation := &model.WorkInsightEvaluation{
		CurrentInsightCount:         1,
		ReadyToMeasurePrecision:     true,
		ReadyToMeasureActionability: true,
		GatedInsightKindCount:       1,
		Kinds: []*model.WorkInsightKindEvaluation{
			{InsightKind: "status_summary", MeasurementScope: workInsightMeasurementScopeProductCandidate, ReadyToMeasure: true, ReadyForProductAction: true},
			{InsightKind: "custom_context_signal", MeasurementScope: workInsightMeasurementScopeContextOnly, ReadyToMeasure: false, ReadyForProductAction: false, ProductActionGateState: "context_only"},
		},
	}
	if !workInsightMeasurementReady(evaluation) {
		t.Fatalf("context-only kind should not block product-candidate measurement readiness: %#v", evaluation)
	}

	contextOnly := &model.WorkInsightEvaluation{
		CurrentInsightCount:         1,
		ReadyToMeasurePrecision:     true,
		ReadyToMeasureActionability: true,
		Kinds: []*model.WorkInsightKindEvaluation{
			{InsightKind: "custom_context_signal", MeasurementScope: workInsightMeasurementScopeContextOnly, ReadyToMeasure: true, ReadyForProductAction: false, ProductActionGateState: "context_only"},
		},
	}
	if workInsightMeasurementReady(contextOnly) {
		t.Fatalf("context-only-only evaluation must not be measurement-ready for product actions: %#v", contextOnly)
	}
}

func TestWorkProgramBriefQualityGatesBlockMeasuredButLowQualityInsights(t *testing.T) {
	gates := workProgramBriefQualityGates(&model.WorkProgramSummary{
		ForecastReadiness: &model.WorkForecastReadiness{EtaForecastReady: true},
	}, &model.WorkInsightEvaluation{
		ReadyToMeasurePrecision:      true,
		ReadyToMeasureActionability:  true,
		PrecisionRate:                0.4,
		UsefulSignalRate:             0.6,
		ActionabilityRate:            0.6,
		QualityGatedInsightKindCount: 1,
		Kinds: []*model.WorkInsightKindEvaluation{
			{
				InsightKind:           "status_summary",
				ReadyToMeasure:        true,
				ReadyForProductAction: false,
			},
		},
	})
	precisionGate := findBriefQualityGate(gates, "measurement_precision")
	if precisionGate == nil || precisionGate.GateState != "gated" || !precisionGate.Blocking || !strings.Contains(precisionGate.Detail, "below product-action threshold") {
		t.Fatalf("measurement precision gate did not block low measured quality: %#v", precisionGate)
	}
	actionabilityGate := findBriefQualityGate(gates, "measurement_actionability")
	if actionabilityGate == nil || actionabilityGate.GateState != "gated" || !actionabilityGate.Blocking || !strings.Contains(actionabilityGate.Detail, "below product-action threshold") {
		t.Fatalf("measurement actionability gate did not block low measured quality: %#v", actionabilityGate)
	}
}

func TestWorkProgramBriefFallbackDoesNotUseGlobalReadinessAsProductActionReady(t *testing.T) {
	evaluation := &model.WorkInsightEvaluation{
		ReadyToMeasurePrecision:     true,
		ReadyToMeasureActionability: true,
		PrecisionRate:               1,
		UsefulSignalRate:            1,
		ActionabilityRate:           1,
		MeasurementLabelCount:       10,
		Kinds: []*model.WorkInsightKindEvaluation{
			{
				InsightKind:           "dependency_cluster",
				ReadyToMeasure:        true,
				ReadyForProductAction: false,
			},
		},
	}
	summary := &model.WorkProgramSummary{ForecastReadiness: &model.WorkForecastReadiness{EtaForecastReady: true}}
	gates := workProgramBriefQualityGates(summary, evaluation)
	functions := workProgramTPMFunctionReadiness(summary, nil, evaluation, gates)
	checks := workProgramAdversarialChecks(summary, nil, evaluation, gates)

	precisionGate := findBriefQualityGate(gates, "measurement_precision")
	if precisionGate == nil || precisionGate.GateState != "gated" || !precisionGate.Blocking || !strings.Contains(precisionGate.Detail, "No product-action insight kind") {
		t.Fatalf("measurement precision gate should not pass from context-only/global labels: %#v", precisionGate)
	}
	insightQuality := findTPMFunctionReadiness(functions, "insight_quality")
	if insightQuality == nil || insightQuality.ReadinessState == "automatable" || !insightQuality.HumanRequired {
		t.Fatalf("insight_quality fallback overclaimed readiness: %#v", insightQuality)
	}
	measurementCheck := findWorkProgramAdversarialCheck(checks, "measurement_overclaim")
	if measurementCheck == nil || measurementCheck.CheckState != "fail" || !strings.Contains(measurementCheck.Title, "Product-action") {
		t.Fatalf("measurement adversarial check should fail product-action overclaim: %#v", measurementCheck)
	}
}

func TestWorkProgramBriefFallbackUsesKindLevelProductActionReadiness(t *testing.T) {
	evaluation := &model.WorkInsightEvaluation{
		ReadyToMeasurePrecision:      false,
		ReadyToMeasureActionability:  false,
		ProductActionReadyKindCount:  1,
		QualityGatedInsightKindCount: 0,
		GatedInsightKindCount:        0,
		MeasurementLabelCount:        3,
		Kinds: []*model.WorkInsightKindEvaluation{
			{
				InsightKind:           "status_summary",
				ReadyToMeasure:        true,
				ReadyForProductAction: true,
			},
		},
	}
	summary := &model.WorkProgramSummary{ForecastReadiness: &model.WorkForecastReadiness{EtaForecastReady: true}}
	gates := workProgramBriefQualityGates(summary, evaluation)
	functions := workProgramTPMFunctionReadiness(summary, nil, evaluation, gates)

	precisionGate := findBriefQualityGate(gates, "measurement_precision")
	if precisionGate == nil || precisionGate.GateState != "passed" || precisionGate.Blocking {
		t.Fatalf("measurement precision gate should pass from kind-level product-action readiness: %#v", precisionGate)
	}
	insightQuality := findTPMFunctionReadiness(functions, "insight_quality")
	if insightQuality == nil || insightQuality.ReadinessState != "automatable" || insightQuality.HumanRequired {
		t.Fatalf("insight_quality fallback should use kind-level product-action readiness: %#v", insightQuality)
	}
	if !strings.Contains(insightQuality.RecommendedAction, "global/context labels as validation coverage") {
		t.Fatalf("insight_quality fallback should preserve global/context boundary: %q", insightQuality.RecommendedAction)
	}
}

func TestWorkProgramBriefFallbackRejectsStaleAggregateReadyCount(t *testing.T) {
	evaluation := &model.WorkInsightEvaluation{
		ReadyToMeasurePrecision:     true,
		ReadyToMeasureActionability: true,
		ProductActionReadyKindCount: 1,
		Kinds: []*model.WorkInsightKindEvaluation{
			{
				InsightKind:            "status_summary",
				MeasurementScope:       workInsightMeasurementScopeProductCandidate,
				ReadyToMeasure:         true,
				ReadyForProductAction:  false,
				ProductActionGateState: "quality_gated",
			},
		},
	}
	summary := &model.WorkProgramSummary{ForecastReadiness: &model.WorkForecastReadiness{EtaForecastReady: true}}
	gates := workProgramBriefQualityGates(summary, evaluation)
	precisionGate := findBriefQualityGate(gates, "measurement_precision")
	if precisionGate == nil || precisionGate.GateState != "gated" || !precisionGate.Blocking {
		t.Fatalf("stale aggregate ready count should not pass measurement gate: %#v", precisionGate)
	}
}

func TestWorkProgramBriefQualityGatesWatchValidationOnlySourceAndClaimLimits(t *testing.T) {
	gates := workProgramBriefQualityGates(&model.WorkProgramSummary{
		ForecastReadiness: &model.WorkForecastReadiness{EtaForecastReady: true},
		Breakdowns: []*model.WorkActionBreakdown{
			{Dimension: workProgramAuthLimitedObservationDimension, Key: "anonymous_observation", Count: 6},
			{Dimension: workProgramGeneratedClaimLimitDimension, Key: "generated_evidence", Count: 11},
		},
	}, &model.WorkInsightEvaluation{
		ReadyToMeasurePrecision:     true,
		ReadyToMeasureActionability: true,
		PrecisionRate:               0.9,
		UsefulSignalRate:            0.9,
		ActionabilityRate:           0.8,
	})

	authGate := findBriefQualityGate(gates, "source_authentication")
	if authGate == nil || authGate.GateState != "watch" || authGate.Blocking || !strings.Contains(authGate.Detail, "validation or QA program items") {
		t.Fatalf("source auth gate did not watch validation-only limits: %#v", authGate)
	}
	claimGate := findBriefQualityGate(gates, "claim_provenance")
	if claimGate == nil || claimGate.GateState != "watch" || claimGate.Blocking || !strings.Contains(claimGate.Detail, "validation or QA program items") {
		t.Fatalf("claim provenance gate did not watch validation-only limits: %#v", claimGate)
	}
}

func TestWorkProgramBriefQualityGatesBlockProductDecisionSourceAndClaimLimits(t *testing.T) {
	gates := workProgramBriefQualityGates(&model.WorkProgramSummary{
		ForecastReadiness: &model.WorkForecastReadiness{EtaForecastReady: true},
		Breakdowns: []*model.WorkActionBreakdown{
			{Dimension: workProgramAuthLimitedObservationDimension, Key: "anonymous_observation", Count: 6},
			{Dimension: workProgramAuthLimitedProductDecisionDimension, Key: "anonymous_observation", Count: 1},
			{Dimension: workProgramGeneratedClaimLimitDimension, Key: "generated_evidence", Count: 11},
			{Dimension: workProgramGeneratedClaimProductDecisionDimension, Key: "generated_evidence", Count: 2},
		},
	}, &model.WorkInsightEvaluation{
		ReadyToMeasurePrecision:     true,
		ReadyToMeasureActionability: true,
		PrecisionRate:               0.9,
		UsefulSignalRate:            0.9,
		ActionabilityRate:           0.8,
	})

	authGate := findBriefQualityGate(gates, "source_authentication")
	if authGate == nil || authGate.GateState != "gated" || !authGate.Blocking || !strings.Contains(authGate.Detail, "1 product-action or decision program item") {
		t.Fatalf("source auth gate did not block product-decision limits: %#v", authGate)
	}
	claimGate := findBriefQualityGate(gates, "claim_provenance")
	if claimGate == nil || claimGate.GateState != "gated" || !claimGate.Blocking || !strings.Contains(claimGate.Detail, "2 product-action or decision program items") {
		t.Fatalf("claim provenance gate did not block product-decision limits: %#v", claimGate)
	}
}

func TestWorkProgramItemProductDecisionOpenIncludesCloseoutReview(t *testing.T) {
	if !workProgramItemProductDecisionOpen(&genent.WorkProgramItem{DecisionState: workprogramitem.DecisionStateCloseoutReview}) {
		t.Fatalf("closeout review decision state should count as a human decision boundary")
	}
	if !workProgramItemProductDecisionOpen(&genent.WorkProgramItem{ProgramStatus: workprogramitem.ProgramStatusClosedPendingReview}) {
		t.Fatalf("closed pending review program status should count as a human decision boundary")
	}
	if workProgramItemProductDecisionOpen(&genent.WorkProgramItem{DecisionState: workprogramitem.DecisionStateValidationLead, ProgramStatus: workprogramitem.ProgramStatusValidateSignal}) {
		t.Fatalf("validation leads should not count as product-decision rows")
	}
}

func TestWorkProgramAutomationEvidenceQueueIncludesQualityAndClearanceWork(t *testing.T) {
	evaluation := &model.WorkInsightEvaluation{
		Kinds: []*model.WorkInsightKindEvaluation{
			{
				InsightKind:               "blocker_candidate",
				MeasurementLabelCount:     10,
				TruthLabeledCount:         10,
				ActionabilityLabeledCount: 10,
				TruePositiveCount:         4,
				PartialCount:              2,
				ActionableCount:           4,
				NeedsOwnerCount:           2,
				PrecisionRate:             0.4,
				UsefulSignalRate:          0.6,
				ActionabilityRate:         0.6,
				ReadyToMeasure:            true,
				ReadyForProductAction:     false,
			},
			{
				InsightKind:           "forecast_risk",
				MeasurementLabelCount: 0,
				RequiredLabelCount:    10,
				ReadyToMeasure:        false,
			},
		},
	}
	summary := &model.WorkProgramSummary{
		WorkstreamKey:              optionalString("flink-kubernetes-operator"),
		TotalCount:                 19,
		SourceCoverageLimitedCount: 2,
		ActiveBlockerCount:         4,
		ActiveBlockerImpactCount:   8,
		ForecastReadiness:          &model.WorkForecastReadiness{EtaForecastReady: false},
		Breakdowns: []*model.WorkActionBreakdown{
			{Dimension: workProgramSourceCoverageLimitDimension, Key: "required_check_coverage_unavailable", Count: 1},
			{Dimension: workProgramSourceCoverageLimitDimension, Key: "source_failure", Count: 1},
			{Dimension: workProgramAuthLimitedObservationDimension, Key: "anonymous_observation", Count: 2},
			{Dimension: workProgramGeneratedClaimLimitDimension, Key: "generated_evidence", Count: 1},
		},
	}
	gates := workProgramBriefQualityGates(summary, evaluation)
	queue := workProgramAutomationEvidenceQueue(summary, evaluation, gates)
	if len(queue) == 0 || queue[0].EvidenceKind != "blocker_clearance" || queue[0].Priority != "critical" || queue[0].MissingCount != 12 {
		t.Fatalf("expected critical blocker-clearance work first: %#v", queue)
	}
	if queue[0].ExecutionState != "missing_action" || queue[0].BackingActionCount != 0 || !strings.Contains(queue[0].NextExecutionStep, "Create blocker-clearance actions") {
		t.Fatalf("expected blocker clearance queue item to expose missing action coverage: %#v", queue[0])
	}
	for _, key := range []string{
		"measurement_quality:blocker_candidate:precision",
		"measurement_quality:blocker_candidate:useful_signal",
		"measurement_quality:blocker_candidate:actionability",
		"measurement_labels:forecast_risk",
		"source_authentication:anonymous_observation",
		"claim_provenance:generated_evidence",
		"source_coverage:required_check_coverage_unavailable",
		"source_coverage:source_failure",
		"forecast_readiness:backtest",
	} {
		if findEvidenceNeed(queue, key) == nil {
			t.Fatalf("automation evidence queue missing %q from %#v", key, queue)
		}
	}
	precisionNeed := findEvidenceNeed(queue, "measurement_quality:blocker_candidate:precision")
	if precisionNeed.CurrentRate == nil || precisionNeed.RequiredRate == nil || *precisionNeed.CurrentRate != 0.4 || *precisionNeed.RequiredRate != 0.7 || precisionNeed.CurrentCount != 4 || precisionNeed.RequiredCount != 7 || precisionNeed.MissingCount != 3 {
		t.Fatalf("unexpected precision evidence need: %#v", precisionNeed)
	}
	authNeed := findEvidenceNeed(queue, "source_authentication:anonymous_observation")
	if authNeed.GateKey != "source_authentication" || authNeed.EvidenceKind != "source_authentication" || authNeed.ExecutionState != "auth_upgrade_needed" || !strings.Contains(authNeed.NextExecutionStep, "authenticated re-observation") {
		t.Fatalf("unexpected anonymous coverage execution state: %#v", authNeed)
	}
	generatedNeed := findEvidenceNeed(queue, "claim_provenance:generated_evidence")
	if generatedNeed.GateKey != "claim_provenance" || generatedNeed.EvidenceKind != "generated_evidence" || generatedNeed.ExecutionState != "claim_provenance_action_needed" || !strings.Contains(generatedNeed.NextExecutionStep, "provenance-review action") {
		t.Fatalf("unexpected generated evidence execution state: %#v", generatedNeed)
	}
	requiredCheckNeed := findEvidenceNeed(queue, "source_coverage:required_check_coverage_unavailable")
	if requiredCheckNeed.EvidenceKind != "required_check_coverage" || requiredCheckNeed.ExecutionState != "configuration_evidence_needed" || !strings.Contains(requiredCheckNeed.NextExecutionStep, "branch protection") {
		t.Fatalf("unexpected required-check coverage execution state: %#v", requiredCheckNeed)
	}
	sourceNeed := findEvidenceNeed(queue, "source_coverage:source_failure")
	if sourceNeed.EvidenceKind != "source_coverage" || sourceNeed.ExecutionState != "missing_source_repair_action" || !strings.Contains(sourceNeed.NextExecutionStep, "Create source-repair actions") {
		t.Fatalf("unexpected source repair execution state: %#v", sourceNeed)
	}
}

func findBriefQualityGate(gates []*model.WorkProgramBriefQualityGate, key string) *model.WorkProgramBriefQualityGate {
	for _, gate := range gates {
		if gate.Key == key {
			return gate
		}
	}
	return nil
}

func findTPMFunctionReadiness(rows []*model.WorkProgramTpmFunctionReadiness, key string) *model.WorkProgramTpmFunctionReadiness {
	for _, row := range rows {
		if row.FunctionKey == key {
			return row
		}
	}
	return nil
}

func findWorkProgramAdversarialCheck(rows []*model.WorkProgramAdversarialCheck, key string) *model.WorkProgramAdversarialCheck {
	for _, row := range rows {
		if row.Key == key {
			return row
		}
	}
	return nil
}

func findEvidenceNeed(rows []*model.WorkProgramAutomationEvidenceNeed, key string) *model.WorkProgramAutomationEvidenceNeed {
	for _, row := range rows {
		if row.Key == key {
			return row
		}
	}
	return nil
}

func testMeasuredInsight(insightKind workinsight.InsightKind, truthLabel workinsightreview.TruthLabel, actionabilityLabel workinsightreview.ActionabilityLabel) *genent.WorkInsight {
	return &genent.WorkInsight{
		InsightKind: insightKind,
		Edges: genent.WorkInsightEdges{
			Reviews: []*genent.WorkInsightReview{
				{
					ReviewKind:          workinsightreview.ReviewKindEvaluationLabel,
					ReviewState:         workinsightreview.ReviewStateAccepted,
					TruthLabel:          truthLabel,
					ActionabilityLabel:  actionabilityLabel,
					LabelQuality:        workinsightreview.LabelQualityGold,
					MeasurementEligible: true,
				},
			},
		},
	}
}
