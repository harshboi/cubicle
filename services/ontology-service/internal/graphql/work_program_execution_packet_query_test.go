package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workprogramadversarialcheck"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramExecutionPacketScopesActionsAndKeepsReadinessGate(t *testing.T) {
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
		SetKey("work-program-quality-gate:execution:forecast").
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
		SetKey("work-program-adversarial-check:execution:forecast").
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
		SetKey("work-program-evidence-need:execution:forecast").
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
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:execution:measurement").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("measurement_precision").
		SetEvidenceKind("insight_labels").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("insight_kind").
		SetTargetKey("forecast_risk").
		SetExecutionState("validation_actions_open").
		SetCurrentCount(0).
		SetRequiredCount(10).
		SetMissingCount(10).
		SetRecommendedAction("Gold-label forecast risk insights before product-action automation.").
		SetNextExecutionStep("Queue forecast-risk insight labels for review.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|measurement_precision:forecast_risk").
		SetRankScore(80).
		Save(ctx)
	if err != nil {
		t.Fatalf("create second evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:execution").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("blocked").
		SetReadinessScore(20).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetSafeAutomationAreas("agenda_summarization").
		SetHumanRequiredAreas("eta_commitments").
		SetRationale("Forecast gate blocks autonomous execution.").
		SetRequiredEvidence("forecast backtest quality").
		SetBlockingGateKeys("forecast_readiness").
		SetQualityGateCount(1).
		SetBlockingGateCount(1).
		SetEvidenceNeedCount(2).
		SetTpmFunctionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create automation readiness: %v", err)
	}

	createActionAndItem := func(key string, actionType workaction.ActionType, decisionState workaction.DecisionState, programStatus workprogramitem.ProgramStatus, bucket workprogramitem.TpmBucket, stream string, actionState workaction.ActionState, rank float64) {
		t.Helper()
		actionCreate := store.Client().WorkAction.Create().
			SetKey("work-action:" + key).
			SetActionType(actionType).
			SetActionState(actionState).
			SetDecisionState(decisionState).
			SetSubjectKind(workaction.SubjectKindUnknown).
			SetSubjectKey("subject:" + key).
			SetDecision("pending_review").
			SetDecisionReason("fixture gate").
			SetDueBucket(workaction.DueBucketNow).
			SetOwnerKey("github:owner").
			SetOwnerSource("fixture").
			SetSourceSystem("cubicle_analytics").
			SetSourceInstance(source).
			SetExternalKind("tpm_work_action").
			SetExternalID("fixture|" + key).
			SetRankScore(rank)
		if actionState == workaction.ActionStateClosed {
			actionCreate = actionCreate.SetClosedAt(generatedAt)
		}
		action, err := actionCreate.Save(ctx)
		if err != nil {
			t.Fatalf("create action %s: %v", key, err)
		}
		_, err = store.Client().WorkProgramItem.Create().
			SetKey("work-program-item:" + key).
			SetWorkAction(action).
			SetWorkstreamKey(stream).
			SetSubjectKind(workprogramitem.SubjectKindUnknown).
			SetSubjectKey("subject:" + key).
			SetTitle("Program item " + key).
			SetProgramStatus(programStatus).
			SetTpmBucket(bucket).
			SetDecisionState(workprogramitem.DecisionState(decisionState.String())).
			SetDueBucket(workprogramitem.DueBucketNow).
			SetFreshnessState(workprogramitem.FreshnessStateFresh).
			SetSourceCoverageState("observed:authenticated_api_current_observation").
			SetRiskScore(rank).
			SetSourceSystem("cubicle_analytics").
			SetSourceInstance(source).
			SetExternalKind("tpm_program_item").
			SetExternalID("fixture|" + key).
			SetRankScore(rank).
			Save(ctx)
		if err != nil {
			t.Fatalf("create program item %s: %v", key, err)
		}
	}
	createActionAndItem("product", workaction.ActionTypeDecisionOrOwnerFollowup, workaction.DecisionStateProductAction, workprogramitem.ProgramStatusNeedsDecision, workprogramitem.TpmBucketRisk, workstream, workaction.ActionStateOpen, 99)
	createActionAndItem("validation", workaction.ActionTypeValidateSignal, workaction.DecisionStateValidationLead, workprogramitem.ProgramStatusValidateSignal, workprogramitem.TpmBucketRiskValidation, workstream, workaction.ActionStateOpen, 90)
	createActionAndItem("source-repair", workaction.ActionTypeRefreshSource, workaction.DecisionStateSourceRepair, workprogramitem.ProgramStatusSourceRepair, workprogramitem.TpmBucketSourceRepair, workstream, workaction.ActionStateOpen, 80)
	createActionAndItem("other-workstream", workaction.ActionTypeDecisionOrOwnerFollowup, workaction.DecisionStateProductAction, workprogramitem.ProgramStatusNeedsDecision, workprogramitem.TpmBucketRisk, "other-workstream", workaction.ActionStateOpen, 100)
	createActionAndItem("closeout-open", workaction.ActionTypeVerifyResolution, workaction.DecisionStateCloseoutReview, workprogramitem.ProgramStatusClosedPendingReview, workprogramitem.TpmBucketClosure, workstream, workaction.ActionStateOpen, 75)
	createActionAndItem("closed", workaction.ActionTypeVerifyResolution, workaction.DecisionStateCloseoutReview, workprogramitem.ProgramStatusClosedPendingReview, workprogramitem.TpmBucketClosure, workstream, workaction.ActionStateClosed, 70)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	actionLimit := 20
	evidenceLimit := 1
	reviewLimit := 20
	sourceArg := source
	packet, err := resolver.WorkProgramExecutionPacket(ctx, "workstream:flink-kubernetes-operator", nil, &actionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("execution packet: %v", err)
	}

	if packet.SourceInstance == nil || *packet.SourceInstance != source {
		t.Fatalf("packet source = %#v, want %s", packet.SourceInstance, source)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != "2026-06-21T07:03:16Z" {
		t.Fatalf("packet generatedAt = %#v, want readiness run timestamp", packet.GeneratedAt)
	}
	if packet.ExecutionState != "blocked_review_queue" || packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("packet execution = state:%s autonomous:%v human:%v, want blocked_review_queue/false/true", packet.ExecutionState, packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.ActionCount != 4 || packet.ProductActionCount != 1 || packet.ValidationLeadCount != 1 || packet.SourceRepairCount != 1 || packet.CloseoutReviewCount != 1 {
		t.Fatalf("packet counts = actions:%d product:%d validation:%d source:%d closeout:%d, want 4/1/1/1/1", packet.ActionCount, packet.ProductActionCount, packet.ValidationLeadCount, packet.SourceRepairCount, packet.CloseoutReviewCount)
	}
	if packet.EvidenceNeedCount != 2 {
		t.Fatalf("packet evidenceNeedCount = %d, want 2", packet.EvidenceNeedCount)
	}
	if len(packet.EvidenceNeeds) != 1 {
		t.Fatalf("packet evidence rows = %d, want 1 capped row", len(packet.EvidenceNeeds))
	}
	if len(packet.Actions) != 4 {
		t.Fatalf("packet actions length = %d, want 4", len(packet.Actions))
	}
	var closeout *model.WorkAction
	for _, action := range packet.Actions {
		if action.ActionState != "open" {
			t.Fatalf("packet included non-open action: %#v", action)
		}
		if strings.Contains(action.Key, "other-workstream") {
			t.Fatalf("packet leaked other workstream action: %#v", action)
		}
		if strings.Contains(action.Key, "closeout-open") {
			closeout = action
		}
	}
	if closeout == nil {
		t.Fatalf("packet actions = %#v, want open closeout action", packet.Actions)
	}
	if closeout.ClaimUse != "closeout_review" || closeout.ProductActionAllowed || closeout.AbsenceClaimAllowed {
		t.Fatalf("closeout claim boundary = use:%s product:%v absence:%v, want closeout_review/false/false", closeout.ClaimUse, closeout.ProductActionAllowed, closeout.AbsenceClaimAllowed)
	}
	if packet.TpmReadiness == nil || packet.TpmReadiness.ReplacementState != "blocked" {
		t.Fatalf("packet readiness = %#v, want blocked nested readiness", packet.TpmReadiness)
	}
	if packet.TpmReadiness.GeneratedAt == nil || *packet.TpmReadiness.GeneratedAt != *packet.GeneratedAt {
		t.Fatalf("nested readiness generatedAt = %#v, want %s", packet.TpmReadiness.GeneratedAt, *packet.GeneratedAt)
	}
	if packet.TpmReadiness.EvidenceNeedCount != 2 {
		t.Fatalf("nested readiness evidenceNeedCount = %d, want 2 uncapped count", packet.TpmReadiness.EvidenceNeedCount)
	}
	focus := ""
	if packet.RecommendedFocus != nil {
		focus = *packet.RecommendedFocus
	}
	if !strings.Contains(focus, "Do not present ETA commitments") {
		t.Fatalf("packet focus = %q, want readiness focus", focus)
	}
	if !strings.Contains(packet.AutomationSummary, "execution packet is blocked_review_queue") || !strings.Contains(packet.AutomationSummary, "4 action(s)") || !strings.Contains(packet.AutomationSummary, "2 evidence need(s)") {
		t.Fatalf("packet summary = %q, want blocked execution summary", packet.AutomationSummary)
	}
}

func TestWorkProgramExecutionCountsUseEffectiveProductActionPermission(t *testing.T) {
	staleETAAction := &genent.WorkAction{
		Key:            "work-action:eta-stale",
		ActionType:     workaction.ActionTypeDecisionOrOwnerFollowup,
		ActionState:    workaction.ActionStateOpen,
		DecisionState:  workaction.DecisionStateProductAction,
		DecisionReason: "eta_forecast_ready=true from stale action row",
		SubjectKind:    workaction.SubjectKindUnknown,
		SubjectKey:     "subject:eta-stale",
		DueBucket:      workaction.DueBucketNow,
		RankScore:      100,
	}
	productAction := &genent.WorkAction{
		Key:            "work-action:product",
		ActionType:     workaction.ActionTypeValidateSignal,
		ActionState:    workaction.ActionStateOpen,
		DecisionState:  workaction.DecisionStateProductAction,
		DecisionReason: "reviewed product action",
		SubjectKind:    workaction.SubjectKindUnknown,
		SubjectKey:     "subject:product",
		DueBucket:      workaction.DueBucketThisWeek,
		RankScore:      90,
	}
	rows := []*genent.WorkAction{staleETAAction, productAction}
	gatedPolicy := workActionClaimPolicyForTpmReadiness(&model.WorkProgramTpmReadinessPacket{
		ForecastReadinessState: "gated",
		ForecastEtaReady:       false,
	})

	counts := workProgramExecutionActionCounts(rows, gatedPolicy)
	if counts.productAction != 1 {
		t.Fatalf("execution productAction count = %d, want only the effective product action", counts.productAction)
	}
	models := workActionModelsWithClaimPolicy(rows, gatedPolicy)
	if len(models) != 2 {
		t.Fatalf("models length = %d, want 2", len(models))
	}
	if models[0].ProductActionAllowed || models[0].EtaClaimAllowed {
		t.Fatalf("stale ETA execution action claim = product:%v eta:%v, want clamped", models[0].ProductActionAllowed, models[0].EtaClaimAllowed)
	}
	if !models[1].ProductActionAllowed {
		t.Fatalf("non-ETA execution product action was incorrectly clamped: %#v", models[1])
	}

	readyPolicy := workActionClaimPolicyForTpmReadiness(&model.WorkProgramTpmReadinessPacket{
		ForecastReadinessState: "ready",
		ForecastEtaReady:       true,
	})
	counts = workProgramExecutionActionCounts(rows, readyPolicy)
	if counts.productAction != 2 {
		t.Fatalf("execution productAction count with ETA ready = %d, want both product actions", counts.productAction)
	}
}

func TestWorkProgramExecutionStateReportsSourceCoverageBlock(t *testing.T) {
	readiness := &model.WorkProgramTpmReadinessPacket{
		ReplacementState:              "blocked",
		AutonomousActionReady:         false,
		SourceCoverageState:           "limited",
		SourceCoverageLimitedCount:    5,
		SourceCoverageUnknownCount:    1,
		AbsenceClaimsAllowed:          false,
		MeasurementState:              "product_action_ready",
		MeasurementProductActionReady: true,
	}
	counts := workProgramExecutionActionCount{productAction: 1}
	state := workProgramExecutionState(readiness, 1, counts, 0)
	if state != "blocked_source_coverage" {
		t.Fatalf("execution state = %q, want blocked_source_coverage", state)
	}
	summary := workProgramExecutionSummary("flink-kubernetes-operator", state, "open", 1, counts, 0, readiness, nil)
	if !strings.Contains(summary, "Source coverage is limited") || !strings.Contains(summary, "absence claims allowed: false") {
		t.Fatalf("summary = %q, want explicit source coverage gate", summary)
	}
}

func TestWorkProgramExecutionStateTreatsResponsibilityValidationAsOpenHumanWork(t *testing.T) {
	readiness := &model.WorkProgramTpmReadinessPacket{
		ReplacementState:              "human_review_required",
		AutonomousActionReady:         false,
		HumanReviewRequired:           true,
		AbsenceClaimsAllowed:          true,
		ResponsibilityValidationCount: 1,
	}
	if got := workProgramExecutionState(readiness, 0, workProgramExecutionActionCount{}, 0); got != "human_review_required" {
		t.Fatalf("execution state = %q, want human_review_required for responsibility validation", got)
	}
}

func TestWorkProgramExecutionStateDoesNotAutonomizeWithEvidenceNeeds(t *testing.T) {
	readiness := &model.WorkProgramTpmReadinessPacket{
		ReplacementState:      "autonomous_ready",
		AutonomousActionReady: true,
		AbsenceClaimsAllowed:  true,
	}
	counts := workProgramExecutionActionCount{productAction: 1}
	state := workProgramExecutionState(readiness, 1, counts, 1)
	if state != "human_review_required" {
		t.Fatalf("execution state = %q, want human_review_required when evidence needs remain", state)
	}
}
