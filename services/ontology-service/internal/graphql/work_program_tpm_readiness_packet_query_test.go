package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogramadversarialcheck"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramTpmReadinessPacketComposesGuardrailsMeasurementAndFunctions(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)

	_, err = store.Client().WorkProgramQualityGate.Create().
		SetKey("work-program-quality-gate:tpm-readiness:forecast").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("forecast_readiness").
		SetGateState("gated").
		SetBlocking(true).
		SetDetail("Forecast gate blocks ETA claims.").
		SetRecommendedAction("Keep ETA claims gated.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_quality_gate").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|forecast_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create quality gate: %v", err)
	}
	_, err = store.Client().WorkProgramAdversarialCheck.Create().
		SetKey("work-program-adversarial-check:tpm-readiness:forecast").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCheckKind("forecast_overclaim").
		SetCheckState(workprogramadversarialcheck.CheckStateFail).
		SetSeverity(workprogramadversarialcheck.SeverityCritical).
		SetTitle("ETA overclaim risk").
		SetDetail("Forecast is not ETA ready.").
		SetRecommendedAction("Do not present ETA commitments.").
		SetBlockingGateKeys("forecast_readiness").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_adversarial_check").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:forecast_overclaim").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create adversarial check: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:tpm-readiness:forecast").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("forecast_readiness").
		SetEvidenceKind("forecast_backtest_quality").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey("workstream:flink-kubernetes-operator").
		SetExecutionState("needs_human_review").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Review forecast backtest quality before making ETA commitments.").
		SetNextExecutionStep("Keep ETA claims blocked until forecast backtest quality clears.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|forecast_readiness:forecast_backtest").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:tpm-readiness").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("blocked").
		SetReadinessScore(25).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetSafeAutomationAreas("agenda_summarization").
		SetHumanRequiredAreas("eta_commitments").
		SetRationale("Forecast gate blocks autonomous TPM replacement.").
		SetRequiredEvidence("forecast backtest quality").
		SetBlockingGateKeys("forecast_readiness").
		SetQualityGateCount(1).
		SetBlockingGateCount(1).
		SetEvidenceNeedCount(1).
		SetTpmFunctionCount(3).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create automation readiness: %v", err)
	}
	_, err = store.Client().WorkstreamHealthSnapshot.Create().
		SetKey("workstream-health-snapshot:tpm-readiness").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetOperatingStatus("attention_required").
		SetActionItemCount(22).
		SetFailingCheckPrCount(2).
		SetOpenFailingCheckPrCount(2).
		SetRecommendedCadenceFocus("Route 2 PRs with failing checks.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_workstream_health_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:health").
		SetRankScore(80).
		Save(ctx)
	if err != nil {
		t.Fatalf("create workstream health snapshot: %v", err)
	}

	createFunction := func(functionKey, readinessState, automationState string, humanRequired bool, rank float64, recommendedAction string) {
		t.Helper()
		_, err := store.Client().WorkProgramTPMFunctionReadiness.Create().
			SetKey("work-program-tpm-function-readiness:" + functionKey).
			SetWorkstreamKey(workstream).
			SetGeneratedAt(generatedAt).
			SetFunctionKey(functionKey).
			SetFunctionName(strings.ReplaceAll(functionKey, "_", " ")).
			SetReadinessState(readinessState).
			SetAutomationState(automationState).
			SetHumanRequired(humanRequired).
			SetSupportingSignalCount(1).
			SetBlockingGateKeys("forecast_readiness").
			SetDetail("Fixture readiness row for " + functionKey + ".").
			SetRecommendedAction(recommendedAction).
			SetSourceSystem("cubicle_analytics").
			SetSourceInstance(source).
			SetExternalKind("tpm_work_program_tpm_function_readiness").
			SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|" + functionKey).
			SetRankScore(rank).
			Save(ctx)
		if err != nil {
			t.Fatalf("create function readiness %s: %v", functionKey, err)
		}
	}
	createFunction("operating_brief", "automatable", "can_publish_operating_brief", false, 100, "Keep publishing sourced operating briefs.")
	createFunction("blocker_management", "supervised", "can_rank_and_draft", true, 90, "Draft blocker follow-up but require owner confirmation.")
	createFunction("forecast_triage", "blocked", "risk_triage_only", true, 95, "Keep forecasts as risk triage, not ETA commitments.")

	snapshot, err := store.Client().WorkInsightEvaluationSnapshot.Create().
		SetKey("work-insight-evaluation-snapshot:tpm-readiness").
		SetGeneratedAt(generatedAt).
		SetCurrentInsightCount(10).
		SetReviewRowCount(0).
		SetMeasurementLabelCount(0).
		SetOpenReviewRequestCount(10).
		SetMinLabeledTotalRequired(10).
		SetMinLabeledPerKindRequired(10).
		SetMinPrecisionRateForProductAction(0.7).
		SetMinUsefulSignalRateForProductAction(0.8).
		SetMinActionabilityRateForProductAction(0.7).
		SetReadyToMeasurePrecision(false).
		SetReadyToMeasureActionability(false).
		SetGatedInsightKindCount(1).
		SetRecommendedNextStep("Gold-label forecast risk before product-action automation.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-21T07:03:16Z|insight_evaluation").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight evaluation snapshot: %v", err)
	}
	_, err = store.Client().WorkInsightKindEvaluationSnapshot.Create().
		SetKey("work-insight-kind-evaluation-snapshot:tpm-readiness:forecast-risk").
		SetEvaluationSnapshot(snapshot).
		SetGeneratedAt(generatedAt).
		SetInsightKind("forecast_risk").
		SetCurrentInsightCount(10).
		SetReviewRowCount(0).
		SetMeasurementLabelCount(0).
		SetOpenReviewRequestCount(10).
		SetTruthLabeledCount(0).
		SetActionabilityLabeledCount(0).
		SetRequiredLabelCount(10).
		SetReadyToMeasure(false).
		SetReadyForProductAction(false).
		SetProductActionGateState("measurement_gated").
		SetProductActionGateReason("Needs 10 more gold labels before product-action quality can be measured.").
		SetRecommendedAction("Gold-label 10 current forecast_risk insight(s) before promoting this kind beyond validation leads.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_kind_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-21T07:03:16Z|insight_kind|forecast_risk").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight kind evaluation snapshot: %v", err)
	}
	decisionEvidence := seedDecisionTargetEvidence(t, ctx, store.Client(), source, "tpm-readiness", "cubicle_analytics", "tpm_generated_evidence")
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:tpm-readiness-coverage",
		source:                  source,
		externalID:              "tpm-readiness-coverage",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratified_summary",
		modelName:               "coverage_guardrail",
		coverageStratum:         "not_testable_single_stratum",
		readyForProductAction:   false,
		productActionGateState:  "validation_gated",
		productActionGateReason: "coverage confounding cannot be tested",
		note:                    "coverage confounding cannot be tested from this sample",
		evaluatedAt:             generatedAt,
		rankScore:               100,
		latestEvidence:          decisionEvidence,
	})
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:tpm-readiness-rf",
		source:                  source,
		externalID:              "tpm-readiness-rf",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratum",
		modelName:               "random_forest_classifier_oof",
		coverageStratum:         "coverage=observed;detail=observed",
		readyForProductAction:   true,
		productActionGateState:  "passed",
		productActionGateReason: "producer claims ready, but this is generated validation evidence only",
		precisionAt10pct:        ptrFloat(0.3793),
		evaluatedAt:             generatedAt,
		rankScore:               80,
		latestEvidence:          decisionEvidence,
	})

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	functionLimit := 1
	evidenceLimit := 20
	reviewLimit := 20
	sourceArg := source
	packet, err := resolver.WorkProgramTpmReadinessPacket(ctx, "workstream:flink-kubernetes-operator", &functionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("tpm readiness packet: %v", err)
	}

	if packet.SourceInstance == nil || *packet.SourceInstance != source {
		t.Fatalf("packet source = %#v, want %s", packet.SourceInstance, source)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != "2026-06-21T07:03:16Z" {
		t.Fatalf("packet generatedAt = %#v, want readiness run timestamp", packet.GeneratedAt)
	}
	if packet.ReplacementState != "blocked" || packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("packet readiness = state:%s autonomous:%v human:%v, want blocked/false/true", packet.ReplacementState, packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.TotalFunctionCount != 3 || packet.ReadyFunctionCount != 1 || packet.SupervisedFunctionCount != 1 || packet.BlockedFunctionCount != 1 || packet.HumanRequiredFunctionCount != 2 {
		t.Fatalf("packet function counts = total:%d ready:%d supervised:%d blocked:%d human:%d, want 3/1/1/1/2", packet.TotalFunctionCount, packet.ReadyFunctionCount, packet.SupervisedFunctionCount, packet.BlockedFunctionCount, packet.HumanRequiredFunctionCount)
	}
	if len(packet.TpmFunctionReadiness) != 1 {
		t.Fatalf("returned function rows = %d, want display limit 1", len(packet.TpmFunctionReadiness))
	}
	if packet.BlockingGateCount != 1 || packet.FailedCheckCount != 1 || packet.EvidenceNeedCount != 1 {
		t.Fatalf("packet guardrail counts = gates:%d failed:%d evidence:%d, want 1/1/1", packet.BlockingGateCount, packet.FailedCheckCount, packet.EvidenceNeedCount)
	}
	if packet.FailedAdversarialCheckCount != 1 || packet.FailingCheckPullRequestCount != 2 || packet.OpenFailingCheckPullRequestCount != 2 {
		t.Fatalf("packet check counts = adversarial:%d failing PR:%d open failing PR:%d, want 1/2/2", packet.FailedAdversarialCheckCount, packet.FailingCheckPullRequestCount, packet.OpenFailingCheckPullRequestCount)
	}
	if packet.MeasurementState != "labeling_needed" || packet.MeasurementProductActionReady {
		t.Fatalf("packet measurement = state:%s ready:%v, want labeling_needed/false", packet.MeasurementState, packet.MeasurementProductActionReady)
	}
	if packet.MeasurementGapCount != 1 || packet.MeasurementMissingLabelCount != 10 {
		t.Fatalf("packet measurement gaps = count:%d missing:%d, want 1/10", packet.MeasurementGapCount, packet.MeasurementMissingLabelCount)
	}
	if packet.ForecastReadinessState != "unknown" || packet.ForecastEtaReady {
		t.Fatalf("packet forecast readiness = state:%s eta:%v, want unknown/false", packet.ForecastReadinessState, packet.ForecastEtaReady)
	}
	if packet.DecisionTargetEvaluationState != "validation_gated" || packet.DecisionTargetEvaluationCount != 2 || packet.ProductReadyDecisionTargetEvaluationCount != 0 {
		t.Fatalf("packet decision target summary = state:%s count:%d ready:%d, want validation_gated/2/0", packet.DecisionTargetEvaluationState, packet.DecisionTargetEvaluationCount, packet.ProductReadyDecisionTargetEvaluationCount)
	}
	if packet.DecisionTargetReadiness == nil || packet.DecisionTargetReadiness.CoverageGateState != "validation_gated" || packet.DecisionTargetReadiness.ProductActionReady {
		t.Fatalf("packet decision target readiness = %#v, want validation-gated false", packet.DecisionTargetReadiness)
	}
	if len(packet.DecisionTargetEvaluations) != 1 || packet.DecisionTargetEvaluations[0].ModelName != "coverage_guardrail" || packet.DecisionTargetEvaluations[0].ProductActionAllowed {
		t.Fatalf("packet decision target rows = %#v, want capped coverage guardrail row with product action blocked", packet.DecisionTargetEvaluations)
	}
	if packet.GuardrailPacket == nil || packet.MeasurementPacket == nil || packet.AutomationReadiness == nil {
		t.Fatalf("packet nested guardrail/measurement/readiness missing: %#v", packet)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Keep forecasts as risk triage") {
		t.Fatalf("packet focus = %#v, want blocked function action", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "AI-TPM replacement readiness is blocked") || !strings.Contains(packet.AutomationSummary, "1 blocked function(s)") || !strings.Contains(packet.AutomationSummary, "10 missing measurement label(s)") || !strings.Contains(packet.AutomationSummary, "2 failing PR(s), 2 still open") || !strings.Contains(packet.AutomationSummary, "Decision-target evaluation is validation_gated with 2 row(s), 0 product-ready") {
		t.Fatalf("packet summary = %q, want blocked replacement summary with decision-target gate", packet.AutomationSummary)
	}
}

func TestWorkProgramTpmReadinessPacketBlocksAutonomyWhenSourceCoverageLimitsAbsenceClaims(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 21, 8, 4, 0, 0, time.UTC)

	_, err = store.Client().WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:source-coverage-gate").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("ready").
		SetReadinessScore(95).
		SetAutonomousActionReady(true).
		SetHumanReviewRequired(false).
		SetSafeAutomationAreas("operating_brief").
		SetRationale("Fixture guardrails otherwise allow autonomous TPM action.").
		SetQualityGateCount(0).
		SetBlockingGateCount(0).
		SetEvidenceNeedCount(0).
		SetTpmFunctionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:04:00Z|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create automation readiness: %v", err)
	}
	_, err = store.Client().WorkProgramTPMFunctionReadiness.Create().
		SetKey("work-program-tpm-function-readiness:source-coverage-gate:operating-brief").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetFunctionKey("operating_brief").
		SetFunctionName("operating brief").
		SetReadinessState("automatable").
		SetAutomationState("can_publish_operating_brief").
		SetHumanRequired(false).
		SetSupportingSignalCount(1).
		SetDetail("Fixture function is automatable when source coverage is safe.").
		SetRecommendedAction("Keep publishing sourced operating briefs.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_tpm_function_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:04:00Z|operating_brief").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create function readiness: %v", err)
	}
	snapshot, err := store.Client().WorkInsightEvaluationSnapshot.Create().
		SetKey("work-insight-evaluation-snapshot:source-coverage-gate").
		SetGeneratedAt(generatedAt).
		SetCurrentInsightCount(10).
		SetReviewRowCount(10).
		SetMeasurementLabelCount(10).
		SetOpenReviewRequestCount(0).
		SetMinLabeledTotalRequired(10).
		SetMinLabeledPerKindRequired(10).
		SetMinPrecisionRateForProductAction(0.7).
		SetMinUsefulSignalRateForProductAction(0.8).
		SetMinActionabilityRateForProductAction(0.7).
		SetPrecisionRate(1.0).
		SetUsefulSignalRate(1.0).
		SetActionabilityRate(1.0).
		SetFalsePositiveRate(0.0).
		SetMeasurementCoverageRate(1.0).
		SetReadyToMeasurePrecision(true).
		SetReadyToMeasureActionability(true).
		SetReadyInsightKindCount(1).
		SetProductActionReadyKindCount(1).
		SetQualityGatedInsightKindCount(0).
		SetGatedInsightKindCount(0).
		SetRecommendedNextStep("Measurement gates are ready for product action.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-21T08:04:00Z|insight_evaluation").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight evaluation snapshot: %v", err)
	}
	_, err = store.Client().WorkInsightKindEvaluationSnapshot.Create().
		SetKey("work-insight-kind-evaluation-snapshot:source-coverage-gate:status-summary").
		SetEvaluationSnapshot(snapshot).
		SetGeneratedAt(generatedAt).
		SetInsightKind("status_summary").
		SetCurrentInsightCount(10).
		SetReviewRowCount(10).
		SetMeasurementLabelCount(10).
		SetOpenReviewRequestCount(0).
		SetTruthLabeledCount(10).
		SetActionabilityLabeledCount(10).
		SetTruePositiveCount(10).
		SetFalsePositiveCount(0).
		SetPartialCount(0).
		SetActionableCount(10).
		SetNeedsOwnerCount(0).
		SetPrecisionRate(1.0).
		SetUsefulSignalRate(1.0).
		SetActionabilityRate(1.0).
		SetFalsePositiveRate(0.0).
		SetMeasurementCoverageRate(1.0).
		SetRequiredLabelCount(10).
		SetReadyToMeasure(true).
		SetReadyForProductAction(true).
		SetProductActionGateState("passed").
		SetProductActionGateReason("Measured precision, useful-signal, and actionability rates meet product-action thresholds.").
		SetRecommendedAction("Insight kind is product-action ready.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_insight_kind_evaluation_snapshot").
		SetExternalID("fixture-source|2026-06-21T08:04:00Z|insight_kind|status_summary").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight kind evaluation snapshot: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:source-coverage-gate:partial").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#72").
		SetTitle("Coverage-limited PR").
		SetProgramStatus(workprogramitem.ProgramStatusSourceRepair).
		SetTpmBucket(workprogramitem.TpmBucketSourceRepair).
		SetDecisionState(workprogramitem.DecisionStateSourceRepair).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStatePartial).
		SetSourceCoverageState("partial:public_api_current_observation").
		SetNextAction("Re-observe PR 72 with authenticated source access before making absence claims.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|source-coverage-gate|partial").
		SetRankScore(100).
		SetRiskScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create partial program item: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	functionLimit := 20
	evidenceLimit := 20
	reviewLimit := 20
	sourceArg := source
	packet, err := resolver.WorkProgramTpmReadinessPacket(ctx, "workstream:flink-kubernetes-operator", &functionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("tpm readiness packet: %v", err)
	}

	if packet.ReplacementState != "blocked" || packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("packet readiness = state:%s autonomous:%v human:%v, want blocked/false/true from source coverage", packet.ReplacementState, packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != "2026-06-21T08:04:00Z" {
		t.Fatalf("packet generatedAt = %#v, want readiness run timestamp", packet.GeneratedAt)
	}
	if packet.ReadyFunctionCount != 1 || packet.BlockedFunctionCount != 0 || packet.HumanRequiredFunctionCount != 0 {
		t.Fatalf("function counts = ready:%d blocked:%d human:%d, want 1/0/0", packet.ReadyFunctionCount, packet.BlockedFunctionCount, packet.HumanRequiredFunctionCount)
	}
	if packet.MeasurementState != "product_action_ready" || !packet.MeasurementProductActionReady {
		t.Fatalf("measurement = state:%s ready:%v, want product_action_ready/true", packet.MeasurementState, packet.MeasurementProductActionReady)
	}
	if packet.SourceCoveragePacket == nil || packet.SourceCoverageState != "limited" || packet.SourceCoverageLimitedCount != 1 || packet.SourceCoverageUnknownCount != 0 || packet.AbsenceClaimsAllowed {
		t.Fatalf("source coverage = packet:%#v state:%s limited:%d unknown:%d absence:%v, want limited/1/0/false", packet.SourceCoveragePacket, packet.SourceCoverageState, packet.SourceCoverageLimitedCount, packet.SourceCoverageUnknownCount, packet.AbsenceClaimsAllowed)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Re-observe PR 72") {
		t.Fatalf("packet focus = %#v, want source coverage focus", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "Source coverage is limited") || !strings.Contains(packet.AutomationSummary, "absence claims allowed: false") {
		t.Fatalf("packet summary = %q, want source coverage gate", packet.AutomationSummary)
	}
}

func TestWorkProgramTpmReadinessPacketBlocksAutonomyWhenResponsibilityNeedsValidation(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 21, 9, 15, 0, 0, time.UTC)
	seedWorkProgramAutonomousReadyFixture(t, ctx, store.Client(), source, workstream, generatedAt)
	_, responsibility := seedCandidateWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	functionLimit := 20
	evidenceLimit := 20
	reviewLimit := 20
	sourceArg := source
	packet, err := resolver.WorkProgramTpmReadinessPacket(ctx, "workstream:flink-kubernetes-operator", &functionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("tpm readiness packet: %v", err)
	}

	if packet.ReplacementState != "human_review_required" || packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("packet readiness = state:%s autonomous:%v human:%v, want human_review_required/false/true", packet.ReplacementState, packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.ResponsibilityValidationCount != 1 || len(packet.Responsibilities) != 1 || packet.Responsibilities[0].Key != responsibility.Key {
		t.Fatalf("responsibility validation = count:%d rows:%#v, want candidate responsibility", packet.ResponsibilityValidationCount, packet.Responsibilities)
	}
	if packet.ReadyFunctionCount != 1 || packet.BlockedFunctionCount != 0 || packet.HumanRequiredFunctionCount != 0 {
		t.Fatalf("function counts = ready:%d blocked:%d human:%d, want 1/0/0", packet.ReadyFunctionCount, packet.BlockedFunctionCount, packet.HumanRequiredFunctionCount)
	}
	if packet.MeasurementState != "product_action_ready" || !packet.MeasurementProductActionReady {
		t.Fatalf("measurement = state:%s ready:%v, want product_action_ready/true", packet.MeasurementState, packet.MeasurementProductActionReady)
	}
	if packet.SourceCoverageState != "complete" || !packet.AbsenceClaimsAllowed {
		t.Fatalf("source coverage = state:%s absence:%v, want complete/true", packet.SourceCoverageState, packet.AbsenceClaimsAllowed)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "generated owner routing") {
		t.Fatalf("packet focus = %#v, want responsibility validation focus", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "1 responsibility validation(s) require human review") {
		t.Fatalf("summary = %q, want responsibility validation count", packet.AutomationSummary)
	}
}

func TestWorkProgramTpmReadinessPacketBlocksAutonomyWhenResponsibilityIsUnassigned(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 21, 9, 20, 0, 0, time.UTC)
	seedWorkProgramAutonomousReadyFixture(t, ctx, store.Client(), source, workstream, generatedAt)
	_, responsibility := seedUnassignedWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	functionLimit := 20
	evidenceLimit := 20
	reviewLimit := 20
	sourceArg := source
	packet, err := resolver.WorkProgramTpmReadinessPacket(ctx, "workstream:flink-kubernetes-operator", &functionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("tpm readiness packet: %v", err)
	}

	if packet.ReplacementState != "human_review_required" || packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("packet readiness = state:%s autonomous:%v human:%v, want human_review_required/false/true", packet.ReplacementState, packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.ResponsibilityValidationCount != 1 || len(packet.Responsibilities) != 1 || packet.Responsibilities[0].Key != responsibility.Key {
		t.Fatalf("responsibility validation = count:%d rows:%#v, want unassigned responsibility", packet.ResponsibilityValidationCount, packet.Responsibilities)
	}
	if packet.Responsibilities[0].PartyKind != "unassigned" {
		t.Fatalf("responsibility party kind = %q, want unassigned", packet.Responsibilities[0].PartyKind)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Assign an accountable owner") {
		t.Fatalf("packet focus = %#v, want unassigned ownership focus", packet.RecommendedFocus)
	}
}

func TestWorkProgramTpmReadinessPacketAllowsActiveSourceNativeResponsibility(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 21, 9, 25, 0, 0, time.UTC)
	seedWorkProgramAutonomousReadyFixture(t, ctx, store.Client(), source, workstream, generatedAt)
	seedActiveSourceNativeWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	functionLimit := 20
	evidenceLimit := 20
	reviewLimit := 20
	sourceArg := source
	packet, err := resolver.WorkProgramTpmReadinessPacket(ctx, "workstream:flink-kubernetes-operator", &functionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("tpm readiness packet: %v", err)
	}

	if packet.ReplacementState != "autonomous_ready" || !packet.AutonomousActionReady || packet.HumanReviewRequired {
		t.Fatalf("packet readiness = state:%s autonomous:%v human:%v, want autonomous_ready/true/false", packet.ReplacementState, packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.ResponsibilityValidationCount != 0 || len(packet.Responsibilities) != 0 {
		t.Fatalf("responsibility validation = count:%d rows:%#v, want active source-native responsibility ignored", packet.ResponsibilityValidationCount, packet.Responsibilities)
	}
}

func TestWorkProgramTPMReplacementStateRequiresHumanReviewForEvidenceNeeds(t *testing.T) {
	guardrail := &model.WorkProgramGuardrailPacket{
		GuardrailState:        "human_review_required",
		AutonomousActionReady: false,
		HumanReviewRequired:   true,
		EvidenceNeedCount:     2,
	}
	measurement := &model.WorkInsightMeasurementPacket{
		MeasurementState:   "product_action_ready",
		ProductActionReady: true,
	}
	sourceCoverage := &model.WorkProgramSourceCoveragePacket{
		CoverageState:        "complete",
		AbsenceClaimsAllowed: true,
	}
	forecastReadiness := &model.WorkForecastReadiness{EtaForecastReady: true, ReadinessState: "ready"}
	counts := workProgramTPMReadinessFunctionCount{total: 1, ready: 1}
	state := workProgramTPMReplacementState(guardrail, measurement, sourceCoverage, forecastReadiness, nil, counts)
	if state != "human_review_required" {
		t.Fatalf("replacement state = %q, want human_review_required when evidence needs remain", state)
	}
}

func TestWorkProgramTPMReplacementStateRequiresFunctionEvidenceForAutonomy(t *testing.T) {
	guardrail := &model.WorkProgramGuardrailPacket{
		GuardrailState:        "autonomous_ready",
		AutonomousActionReady: true,
		HumanReviewRequired:   false,
		EvidenceNeedCount:     0,
	}
	measurement := &model.WorkInsightMeasurementPacket{
		MeasurementState:   "product_action_ready",
		ProductActionReady: true,
	}
	sourceCoverage := &model.WorkProgramSourceCoveragePacket{
		CoverageState:        "complete",
		AbsenceClaimsAllowed: true,
	}
	forecastReadiness := &model.WorkForecastReadiness{EtaForecastReady: true, ReadinessState: "ready"}
	state := workProgramTPMReplacementState(guardrail, measurement, sourceCoverage, forecastReadiness, nil, workProgramTPMReadinessFunctionCount{})
	if state != "review_required" {
		t.Fatalf("replacement state = %q, want review_required with no function readiness evidence", state)
	}
}

func TestWorkProgramTPMReplacementStateRequiresForecastReadinessForAutonomy(t *testing.T) {
	guardrail := &model.WorkProgramGuardrailPacket{
		GuardrailState:        "autonomous_ready",
		AutonomousActionReady: true,
		HumanReviewRequired:   false,
		EvidenceNeedCount:     0,
	}
	measurement := &model.WorkInsightMeasurementPacket{
		MeasurementState:   "product_action_ready",
		ProductActionReady: true,
	}
	sourceCoverage := &model.WorkProgramSourceCoveragePacket{
		CoverageState:        "complete",
		AbsenceClaimsAllowed: true,
	}
	counts := workProgramTPMReadinessFunctionCount{total: 1, ready: 1}
	state := workProgramTPMReplacementState(guardrail, measurement, sourceCoverage, &model.WorkForecastReadiness{ReadinessState: "gated"}, nil, counts)
	if state != "human_review_required" {
		t.Fatalf("replacement state = %q, want human_review_required when ETA forecast readiness is gated", state)
	}
}
