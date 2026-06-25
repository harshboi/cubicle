package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workforecastevaluation"
	"cubicle/services/ontology-service/ent/workitemforecast"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramForecastPacketAggregatesReadinessRiskAndEvidence(t *testing.T) {
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
	subjectKey := "apache/flink-kubernetes-operator#100"
	secondSubjectKey := "apache/flink-kubernetes-operator#101"
	offScopeSubjectKey := "apache/flink-kubernetes-operator#999"
	runAt := time.Date(2026, 6, 21, 14, 10, 0, 0, time.UTC)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, runAt)

	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:packet:summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("median_cycle_baseline").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("median_cycle_baseline").
		SetBaselineSampleCount(60).
		SetOpenCandidateCount(20).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("best K-fold model is still baseline; do not present ETA commitments").
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary:2026-06-21T14:10:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:packet:kfold-gradient").
		SetEvaluationKind(workforecastevaluation.EvaluationKindKfold).
		SetModelName("gradient_boosting_absolute_error").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("gradient_boosting_absolute_error").
		SetTestCount(10).
		SetMaeDays(6.07).
		SetImprovementVsMedianPct(7.04).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("gradient boosting remains gated for ETA but useful as a diagnostic.").
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("kfold-gradient:2026-06-21T14:10:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:packet:survival").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSurvivalTimeToMerge).
		SetModelName("km_median_remaining").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("km_median_remaining").
		SetTestCount(10).
		SetMaeDays(21.14).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("remaining-time baseline is diagnostic only.").
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("survival:2026-06-21T14:10:00Z").
		SaveX(ctx)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:forecast-subject").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("High-risk forecast subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(91).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:forecast-subject").
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:forecast-second-subject").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(secondSubjectKey).
		SetTitle("Second high-risk forecast subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(90).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:forecast-second-subject").
		Save(ctx)
	if err != nil {
		t.Fatalf("create second program item: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:forecast-other-workstream").
		SetWorkstreamKey("other-workstream").
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(offScopeSubjectKey).
		SetTitle("Off-scope forecast subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(100).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:forecast-other-workstream").
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope program item: %v", err)
	}

	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:packet:scoped").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetSubjectState("open").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetRiskScore(95).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetReadinessReason("ETA remains gated by backtest quality.").
		SetForecastedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:packet:scoped").
		Save(ctx)
	if err != nil {
		t.Fatalf("create scoped forecast: %v", err)
	}
	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:packet:scoped-second").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(secondSubjectKey).
		SetSubjectState("open").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetRiskBand(workitemforecast.RiskBandHigh).
		SetRiskScore(94).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetReadinessReason("ETA remains gated by backtest quality.").
		SetForecastedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:packet:scoped-second").
		Save(ctx)
	if err != nil {
		t.Fatalf("create second scoped forecast: %v", err)
	}
	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:packet:off-scope").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(offScopeSubjectKey).
		SetSubjectState("open").
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetRiskScore(100).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetForecastedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:packet:off-scope").
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope forecast: %v", err)
	}

	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:forecast-subject").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(runAt).
		SetGateKey("forecast_readiness").
		SetEvidenceKind("forecast_backtest_quality").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey(subjectKey).
		SetExecutionState("risk_action_open").
		SetBackingActionCount(1).
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Review forecast backtest quality before making ETA commitments.").
		SetNextExecutionStep("Keep forecast output as risk triage until ETA gates clear.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T14:10:00Z|forecast_readiness:forecast-subject").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:forecast-target-blocker").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(runAt).
		SetGateKey("blocker_clearance").
		SetEvidenceKind("blocker_owner_status").
		SetPriority(workprogramevidenceneed.PriorityCritical).
		SetTargetKind("pull_request").
		SetTargetKey(subjectKey).
		SetExecutionState("action_open").
		SetRecommendedAction("Confirm blocker clearance for the high-risk forecast target.").
		SetNextExecutionStep("Drive blocker clearance separately from forecast-readiness messaging.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T14:10:00Z|blocker_clearance:forecast-subject").
		SetRankScore(99).
		Save(ctx)
	if err != nil {
		t.Fatalf("create target blocker evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:forecast-off-target").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(runAt).
		SetGateKey("forecast_readiness").
		SetEvidenceKind("forecast_risk_triage").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey(offScopeSubjectKey).
		SetExecutionState("risk_action_open").
		SetRecommendedAction("Off-target forecast evidence should not count in scoped packet.").
		SetNextExecutionStep("Do not include unrelated forecast target evidence.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T14:10:00Z|forecast_readiness:off-target").
		SetRankScore(98).
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-target forecast evidence need: %v", err)
	}
	decisionEvidence := seedDecisionTargetEvidence(t, ctx, store.Client(), source, "forecast-packet", "cubicle_analytics", "tpm_generated_evidence")
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:forecast-packet-coverage",
		source:                  source,
		externalID:              "forecast-packet-coverage",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratified_summary",
		modelName:               "coverage_guardrail",
		coverageStratum:         "not_testable_single_stratum",
		readyForProductAction:   false,
		productActionGateState:  "validation_gated",
		productActionGateReason: "coverage confounding cannot be tested",
		note:                    "coverage confounding cannot be tested from this sample",
		evaluatedAt:             runAt,
		rankScore:               100,
		latestEvidence:          decisionEvidence,
	})
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:forecast-packet-rf",
		source:                  source,
		externalID:              "forecast-packet-rf",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratum",
		modelName:               "random_forest_classifier_oof",
		coverageStratum:         "coverage=observed;detail=observed",
		readyForProductAction:   true,
		productActionGateState:  "passed",
		productActionGateReason: "producer claims ready, but coverage guardrail keeps this validation-only",
		precisionAt10pct:        ptrFloat(0.3793),
		liftAt10pct:             ptrFloat(0.3446),
		evaluatedAt:             runAt,
		rankScore:               80,
		latestEvidence:          decisionEvidence,
	})

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rowLimit := 1
	evidenceLimit := 1
	sourceArg := source
	packet, err := resolver.WorkProgramForecastPacket(ctx, "workstream:flink-kubernetes-operator", &rowLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("forecast packet: %v", err)
	}
	if packet.SourceInstance == nil || *packet.SourceInstance != source {
		t.Fatalf("packet source = %#v, want %s", packet.SourceInstance, source)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != runAt.Format(time.RFC3339) {
		t.Fatalf("packet generatedAt = %#v, want readiness run timestamp", packet.GeneratedAt)
	}
	if packet.WorkstreamKey != "workstream:flink-kubernetes-operator" {
		t.Fatalf("packet workstream = %s, want query workstream", packet.WorkstreamKey)
	}
	if packet.EtaForecastReady {
		t.Fatalf("packet etaForecastReady=true, want gated ETA")
	}
	if packet.ReadinessState != "gated" || packet.ForecastMethod == nil || *packet.ForecastMethod != "typed_forecast_backtest_gate" {
		t.Fatalf("packet readiness = %s method=%#v, want gated typed_forecast_backtest_gate", packet.ReadinessState, packet.ForecastMethod)
	}
	if packet.HighRiskForecastCount != 2 || len(packet.Forecasts) != 1 || packet.Forecasts[0].SubjectKey != subjectKey {
		t.Fatalf("packet forecasts leaked, undercounted, or missed scoped row: count=%d rows=%#v", packet.HighRiskForecastCount, packet.Forecasts)
	}
	if packet.EtaReadyForecastCount != 0 {
		t.Fatalf("packet eta-ready forecast count = %d, want 0", packet.EtaReadyForecastCount)
	}
	if packet.DecisionTargetEvaluationState != "validation_gated" || packet.DecisionTargetEvaluationCount != 2 || packet.ProductReadyDecisionTargetEvaluationCount != 0 {
		t.Fatalf("decision target summary = state:%s count:%d ready:%d, want validation_gated/2/0", packet.DecisionTargetEvaluationState, packet.DecisionTargetEvaluationCount, packet.ProductReadyDecisionTargetEvaluationCount)
	}
	if packet.DecisionTargetReadiness == nil || packet.DecisionTargetReadiness.CoverageGateState != "validation_gated" || packet.DecisionTargetReadiness.ProductActionReady {
		t.Fatalf("decision target readiness = %#v, want validation gate and no product action", packet.DecisionTargetReadiness)
	}
	reliabilityByProduct := workProgramForecastReliabilityByProduct(packet.ForecastReliability)
	if len(reliabilityByProduct) != 3 {
		t.Fatalf("forecast reliability rows = %#v, want point, range, and risk rows", packet.ForecastReliability)
	}
	if row := reliabilityByProduct["point_eta"]; row == nil || row.ProductSafe || row.SafeUse != "diagnostic_only" || row.BestModel != "gradient_boosting_absolute_error" || row.PrimaryMetric != "eta_kfold_best_candidate_improvement_pct" || row.MetricValue != "7.04" {
		t.Fatalf("point ETA reliability = %#v, want gated diagnostic gradient row", row)
	}
	if row := reliabilityByProduct["range_eta"]; row == nil || row.ProductSafe || row.SafeUse != "wide_range_context" || row.BestModel != "survival_time_to_merge:km_median_remaining" || row.MetricValue != "21.14" {
		t.Fatalf("range ETA reliability = %#v, want diagnostic survival baseline", row)
	}
	if row := reliabilityByProduct["risk_triage"]; row == nil || !row.ProductSafe || row.SafeUse != "attention_ordering" || row.ReadinessState != "ready_with_coverage_guardrail" || row.MetricValue != "0.3446" {
		t.Fatalf("risk triage reliability = %#v, want attention-ordering row with coverage guardrail", row)
	}
	if len(packet.DecisionTargetEvaluations) != 1 || packet.DecisionTargetEvaluations[0].ModelName != "coverage_guardrail" || packet.DecisionTargetEvaluations[0].ProductActionAllowed {
		t.Fatalf("packet decision target rows = %#v, want capped coverage guardrail row with product action blocked", packet.DecisionTargetEvaluations)
	}
	if packet.EvidenceNeedCount != 2 || len(packet.EvidenceNeeds) != 1 {
		t.Fatalf("packet evidence needs = count=%d rows=%#v, want 2 total and 1 capped returned row", packet.EvidenceNeedCount, packet.EvidenceNeeds)
	}
	for _, need := range packet.EvidenceNeeds {
		if need.TargetKey == nil || *need.TargetKey != subjectKey {
			t.Fatalf("packet evidence need has wrong target: %#v", need)
		}
	}
	if !packet.HumanRequired {
		t.Fatalf("packet humanRequired=false, want true")
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Review forecast backtest quality") {
		t.Fatalf("packet recommended focus = %#v, want forecast readiness evidence action", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "risk triage only") || !strings.Contains(packet.AutomationSummary, "Decision-target evaluation is validation_gated with 2 row(s)") || !strings.Contains(packet.AutomationSummary, "2 high-risk forecast(s)") {
		t.Fatalf("packet automation summary = %q, want triage guidance, decision-target gate, and high-risk count", packet.AutomationSummary)
	}
}

func workProgramForecastReliabilityByProduct(rows []*model.WorkProgramForecastReliability) map[string]*model.WorkProgramForecastReliability {
	out := make(map[string]*model.WorkProgramForecastReliability, len(rows))
	for _, row := range rows {
		if row != nil {
			out[row.ForecastProduct] = row
		}
	}
	return out
}

func TestWorkProgramForecastPacketClampsETAClaimsWhenDecisionTargetValidationGated(t *testing.T) {
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
	subjectKey := "apache/flink-kubernetes-operator#202"
	runAt := time.Date(2026, 6, 21, 17, 0, 0, 0, time.UTC)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, runAt)

	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:packet:decision-target-gate-summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("source_event_as_of_gradient_boosting").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("source_event_as_of_gradient_boosting").
		SetBaselineSampleCount(80).
		SetOpenCandidateCount(20).
		SetReadyForEta(true).
		SetReadinessState(workforecastevaluation.ReadinessStateReady).
		SetReadinessReason("ETA forecast gate cleared, pending decision-target validation.").
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary:decision-target-gate:2026-06-21T17:00:00Z").
		SaveX(ctx)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:decision-target-gate").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Decision target gated ETA forecast subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:decision-target-gate").
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}

	action, err := store.Client().WorkAction.Create().
		SetKey("tpm-action:decision-target-gate").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecision("owner_eta_followup").
		SetDecisionReason("eta_forecast_ready=true from typed forecast run").
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(99).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:decision-target-gate").
		SetLastActivityAt(runAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}

	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:packet:decision-target-gate").
		SetWorkAction(action).
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetSubjectState("open").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetRiskScore(97).
		SetReadyForEta(true).
		SetReadinessState(workitemforecast.ReadinessStateReady).
		SetReadinessReason("row and summary both say ETA ready; decision-target gate must still clamp product use").
		SetForecastedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:packet:decision-target-gate").
		Save(ctx)
	if err != nil {
		t.Fatalf("create forecast: %v", err)
	}

	decisionEvidence := seedDecisionTargetEvidence(t, ctx, store.Client(), source, "forecast-decision-target-gate", "cubicle_analytics", "tpm_generated_evidence")
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:forecast-decision-target-gate",
		source:                  source,
		externalID:              "forecast-decision-target-gate",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratified_summary",
		modelName:               "coverage_guardrail",
		coverageStratum:         "not_testable_single_stratum",
		readyForProductAction:   false,
		productActionGateState:  "validation_gated",
		productActionGateReason: "coverage confounding cannot be tested",
		note:                    "coverage confounding cannot be tested from this sample",
		evaluatedAt:             runAt,
		rankScore:               100,
		latestEvidence:          decisionEvidence,
	})

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramForecastPacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("forecast packet: %v", err)
	}
	if packet.ReadinessState != "validation_gated" || packet.EtaForecastReady || packet.EtaReadyForecastCount != 0 {
		t.Fatalf("packet ETA readiness = state:%s ready:%v count:%d, want decision-target validation gate", packet.ReadinessState, packet.EtaForecastReady, packet.EtaReadyForecastCount)
	}
	if packet.Readiness == nil || packet.Readiness.EtaReadinessBlockingReason == nil || *packet.Readiness.EtaReadinessBlockingReason != "decision_target_validation_gated" {
		t.Fatalf("packet readiness = %#v, want decision_target_validation_gated blocking reason", packet.Readiness)
	}
	if !packet.HumanRequired || packet.DecisionTargetEvaluationState != "validation_gated" || packet.DecisionTargetReadiness == nil || packet.DecisionTargetReadiness.ProductActionReady {
		t.Fatalf("decision target gate = human:%v state:%s readiness:%#v, want human-required validation gate", packet.HumanRequired, packet.DecisionTargetEvaluationState, packet.DecisionTargetReadiness)
	}
	if len(packet.Forecasts) != 1 {
		t.Fatalf("packet forecast count = %d, want 1", len(packet.Forecasts))
	}
	forecast := packet.Forecasts[0]
	if forecast.EtaForecastReady || forecast.EtaClaimAllowed || forecast.ForecastClaimUse == "eta_candidate" {
		t.Fatalf("forecast claim = etaReady:%v etaAllowed:%v use:%s, want decision-target clamp", forecast.EtaForecastReady, forecast.EtaClaimAllowed, forecast.ForecastClaimUse)
	}
	if forecast.ForecastClaimGateReason != "global_eta_forecast_gated:decision_target_validation_gated" {
		t.Fatalf("forecast gate reason = %q, want decision-target gate", forecast.ForecastClaimGateReason)
	}
	if forecast.Action == nil {
		t.Fatalf("forecast action missing")
	}
	if forecast.Action.EtaClaimAllowed || forecast.Action.ProductActionAllowed || forecast.Action.ClaimUse == "eta_candidate_product_action" {
		t.Fatalf("action claim = eta:%v product:%v use:%s, want decision-target clamp", forecast.Action.EtaClaimAllowed, forecast.Action.ProductActionAllowed, forecast.Action.ClaimUse)
	}
	if forecast.Action.ClaimGateReason != "eta_forecast_readiness_gated:global_eta_forecast_gated:decision_target_validation_gated" {
		t.Fatalf("action gate reason = %q, want nested decision-target gate", forecast.Action.ClaimGateReason)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "coverage confounding cannot be tested") {
		t.Fatalf("packet focus = %#v, want decision-target validation focus", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "risk triage only") || !strings.Contains(packet.AutomationSummary, "Decision-target evaluation is validation_gated") {
		t.Fatalf("packet summary = %q, want risk-triage decision-target gate", packet.AutomationSummary)
	}
}

func TestWorkProgramForecastPacketClampsStaleETAClaimsToGlobalReadiness(t *testing.T) {
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
	subjectKey := "apache/flink-kubernetes-operator#200"
	runAt := time.Date(2026, 6, 21, 16, 0, 0, 0, time.UTC)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, runAt)

	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:packet:eta-clamp-summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("gradient_boosting_absolute_error").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("gradient_boosting_absolute_error").
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("ETA forecast is gated: primary blocker kfold_model_does_not_beat_baseline").
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary:eta-clamp:2026-06-21T16:00:00Z").
		SaveX(ctx)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:eta-clamp").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Stale ETA forecast subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:eta-clamp").
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}

	action, err := store.Client().WorkAction.Create().
		SetKey("tpm-action:eta-clamp").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecisionReason("eta_forecast_ready=true from stale action row").
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(99).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:eta-clamp").
		SetLastActivityAt(runAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}

	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:packet:eta-clamp").
		SetWorkAction(action).
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetSubjectState("open").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetRiskScore(96).
		SetReadyForEta(true).
		SetReadinessState(workitemforecast.ReadinessStateReady).
		SetReadinessReason("stale row says ready, global summary is gated").
		SetForecastedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:packet:eta-clamp").
		Save(ctx)
	if err != nil {
		t.Fatalf("create forecast: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramForecastPacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("forecast packet: %v", err)
	}
	if packet.EtaForecastReady || packet.EtaReadyForecastCount != 0 {
		t.Fatalf("packet ETA readiness = ready:%v count:%d, want globally gated", packet.EtaForecastReady, packet.EtaReadyForecastCount)
	}
	if len(packet.Forecasts) != 1 {
		t.Fatalf("packet forecast count = %d, want 1", len(packet.Forecasts))
	}
	forecast := packet.Forecasts[0]
	if forecast.EtaForecastReady || forecast.EtaClaimAllowed || forecast.ForecastClaimUse == "eta_candidate" {
		t.Fatalf("forecast claim = etaReady:%v etaAllowed:%v use:%s, want global gate clamp", forecast.EtaForecastReady, forecast.EtaClaimAllowed, forecast.ForecastClaimUse)
	}
	if !strings.HasPrefix(forecast.ForecastClaimGateReason, "global_eta_forecast_gated:") {
		t.Fatalf("forecast gate reason = %q, want global ETA gate", forecast.ForecastClaimGateReason)
	}
	if forecast.Action == nil {
		t.Fatalf("forecast action missing")
	}
	if forecast.Action.EtaClaimAllowed || forecast.Action.ProductActionAllowed || forecast.Action.ClaimUse == "eta_candidate_product_action" {
		t.Fatalf("action claim = eta:%v product:%v use:%s, want global gate clamp", forecast.Action.EtaClaimAllowed, forecast.Action.ProductActionAllowed, forecast.Action.ClaimUse)
	}
	if !strings.HasPrefix(forecast.Action.ClaimGateReason, "eta_forecast_readiness_gated:") {
		t.Fatalf("action gate reason = %q, want ETA readiness gate", forecast.Action.ClaimGateReason)
	}
	for _, badge := range forecast.Action.Badges {
		if badge.Key == "decision:product_action" {
			t.Fatalf("clamped ETA action still exposed product-action badge: %#v", forecast.Action.Badges)
		}
	}
}

func TestWorkProgramForecastPacketClampsReadyEvaluationFromDifferentForecastRun(t *testing.T) {
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
	subjectKey := "apache/flink-kubernetes-operator#201"
	evaluationAt := time.Date(2026, 6, 21, 16, 0, 0, 0, time.UTC)
	forecastedAt := evaluationAt.Add(time.Hour)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, forecastedAt)

	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:packet:eta-run-mismatch-summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("gradient_boosting_absolute_error").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("gradient_boosting_absolute_error").
		SetReadyForEta(true).
		SetReadinessState(workforecastevaluation.ReadinessStateReady).
		SetReadinessReason("older forecast evaluation cleared ETA gate").
		SetEvaluatedAt(evaluationAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary:eta-run-mismatch:2026-06-21T16:00:00Z").
		SaveX(ctx)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:eta-run-mismatch").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Newer ready forecast subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:eta-run-mismatch").
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}

	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:packet:eta-run-mismatch").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetSubjectState("open").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetRiskScore(96).
		SetReadyForEta(true).
		SetReadinessState(workitemforecast.ReadinessStateReady).
		SetReadinessReason("row says ready, but row run is newer than evaluation").
		SetForecastedAt(forecastedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:packet:eta-run-mismatch").
		Save(ctx)
	if err != nil {
		t.Fatalf("create forecast: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramForecastPacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("forecast packet: %v", err)
	}
	if packet.EtaForecastReady || packet.Readiness == nil || packet.Readiness.EtaReadinessBlockingReason == nil || *packet.Readiness.EtaReadinessBlockingReason != "forecast_run_mismatch" {
		t.Fatalf("packet readiness = ready:%v readiness:%#v, want forecast_run_mismatch gate", packet.EtaForecastReady, packet.Readiness)
	}
	if packet.EtaReadyForecastCount != 0 || len(packet.Forecasts) != 1 {
		t.Fatalf("packet forecast counts = etaReady:%d rows:%d, want 0/1", packet.EtaReadyForecastCount, len(packet.Forecasts))
	}
	forecast := packet.Forecasts[0]
	if forecast.EtaForecastReady || forecast.EtaClaimAllowed || forecast.ForecastClaimGateReason != "global_eta_forecast_gated:forecast_run_mismatch" {
		t.Fatalf("forecast claim = etaReady:%v etaAllowed:%v gate:%s, want packet-level run mismatch clamp", forecast.EtaForecastReady, forecast.EtaClaimAllowed, forecast.ForecastClaimGateReason)
	}
}

func TestWorkProgramForecastPacketUsesReadinessRunForEvidenceNeeds(t *testing.T) {
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
	subjectKey := "apache/flink-kubernetes-operator#100"
	oldRunAt := time.Date(2026, 6, 21, 14, 10, 0, 0, time.UTC)
	newRunAt := oldRunAt.Add(time.Hour)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, newRunAt)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:forecast-subject:run-boundary").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("High-risk forecast subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:forecast-subject:run-boundary").
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:packet:run-boundary").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetSubjectState("open").
		SetRiskBand(workitemforecast.RiskBandHigh).
		SetRiskScore(94).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetForecastedAt(newRunAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:packet:run-boundary").
		Save(ctx)
	if err != nil {
		t.Fatalf("create forecast: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:forecast-stale-run").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(oldRunAt).
		SetGateKey("forecast_readiness").
		SetEvidenceKind("forecast_backtest_quality").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey(subjectKey).
		SetExecutionState("risk_action_open").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("This stale forecast evidence should not leak into the new packet.").
		SetNextExecutionStep("Do not include stale evidence need rows.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T14:10:00Z|forecast_readiness:stale").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramForecastPacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("forecast packet: %v", err)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != newRunAt.Format(time.RFC3339) {
		t.Fatalf("packet generatedAt = %#v, want newer readiness run timestamp", packet.GeneratedAt)
	}
	if packet.HighRiskForecastCount != 1 {
		t.Fatalf("highRiskForecastCount = %d, want current forecast still present", packet.HighRiskForecastCount)
	}
	if packet.EvidenceNeedCount != 0 || len(packet.EvidenceNeeds) != 0 {
		t.Fatalf("packet leaked stale evidence needs = count:%d rows:%#v, want 0", packet.EvidenceNeedCount, packet.EvidenceNeeds)
	}
}
