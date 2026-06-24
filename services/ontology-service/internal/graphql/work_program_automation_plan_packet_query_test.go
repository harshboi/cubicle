package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramAutomationPlanBlocksSourceCoverageAndClassifiesOnce(t *testing.T) {
	focus := "Re-observe coverage-limited PRs before product claims."
	execution := &model.WorkProgramExecutionPacket{
		SourceInstance:             stringPointer("fixture-source"),
		WorkstreamKey:              "workstream:flink-kubernetes-operator",
		ActionState:                "open",
		ExecutionState:             "blocked_source_coverage",
		AutonomousActionReady:      false,
		HumanReviewRequired:        true,
		SourceCoverageState:        "limited",
		SourceCoverageLimitedCount: 5,
		AbsenceClaimsAllowed:       false,
		RecommendedFocus:           &focus,
		Actions: []*model.WorkAction{
			{Key: "work-action:product", DecisionState: "product_action"},
			{Key: "work-action:validation", DecisionState: "validation_lead"},
		},
		EvidenceNeeds: []*model.WorkProgramAutomationEvidenceNeed{
			{
				Key:               "work-program-evidence-need:coverage",
				GateKey:           "source_coverage",
				RecommendedAction: "Repair source coverage before making absence claims.",
			},
		},
		TpmReadiness: &model.WorkProgramTpmReadinessPacket{
			ReplacementState:           "blocked",
			AutonomousActionReady:      false,
			HumanReviewRequired:        true,
			ReadyFunctionCount:         1,
			SupervisedFunctionCount:    1,
			BlockedFunctionCount:       1,
			HumanRequiredFunctionCount: 2,
			BlockingGateCount:          1,
			FailedCheckCount:           1,
			EvidenceNeedCount:          1,
			MeasurementState:           "labeling_needed",
			ForecastReadinessState:     "unknown",
			ForecastEtaReady:           false,
			SourceCoverageState:        "limited",
			SourceCoverageLimitedCount: 5,
			AbsenceClaimsAllowed:       false,
			AutomationReadiness:        &model.WorkProgramAutomationReadiness{SafeAutomationAreas: []string{"operating_brief"}, HumanRequiredAreas: []string{"eta_commitments", "eta_commitments"}},
			TpmFunctionReadiness: []*model.WorkProgramTpmFunctionReadiness{
				{FunctionKey: "operating_brief", ReadinessState: "automatable", HumanRequired: false},
				{FunctionKey: "blocker_management", ReadinessState: "supervised", HumanRequired: true},
				{FunctionKey: "forecast_triage", ReadinessState: "blocked", HumanRequired: true, RecommendedAction: "Keep forecasts as risk triage."},
			},
		},
	}

	queues := workProgramAutomationPlanQueuesFor(execution)
	actionPlans := workProgramAutomationActionPlans(execution, workProgramAutomationPlanBlockedAreas(execution, []string{"eta_commitments"}))
	if workProgramAutomationPlanAutonomyAllowed(execution) {
		t.Fatalf("autonomy allowed for coverage-blocked execution")
	}
	if got := workProgramAutomationPlanState(execution, queues); got != "blocked_source_coverage" {
		t.Fatalf("plan state = %q, want blocked_source_coverage", got)
	}
	if got := workProgramAutomationPlanAutonomyLevel("blocked_source_coverage"); got != "blocked" {
		t.Fatalf("autonomy level = %q, want blocked", got)
	}
	if len(queues.autonomousActions) != 0 || len(queues.humanReviewActions) != 0 || len(queues.blockedActions) != 2 {
		t.Fatalf("action queues = autonomous:%d human:%d blocked:%d, want 0/0/2", len(queues.autonomousActions), len(queues.humanReviewActions), len(queues.blockedActions))
	}
	if len(actionPlans) != 2 {
		t.Fatalf("action plans = %d, want 2", len(actionPlans))
	}
	for _, plan := range actionPlans {
		if plan.Disposition != "blocked" || plan.AutonomyLevel != "blocked" {
			t.Fatalf("action plan disposition = %s/%s, want blocked/blocked", plan.Disposition, plan.AutonomyLevel)
		}
		assertContainsString(t, plan.BlockingAreas, "source_coverage")
		if !strings.Contains(plan.Reason, "Source coverage") {
			t.Fatalf("action plan reason = %q, want source coverage reason", plan.Reason)
		}
	}
	if len(queues.safeFunctions) != 1 || len(queues.supervisedFunctions) != 1 || len(queues.blockedFunctions) != 1 {
		t.Fatalf("function queues = safe:%d supervised:%d blocked:%d, want 1/1/1", len(queues.safeFunctions), len(queues.supervisedFunctions), len(queues.blockedFunctions))
	}
	if !workProgramAutomationPlanHumanReviewRequired(execution, queues, false) {
		t.Fatalf("coverage-blocked execution should require human review")
	}

	safeAreas, humanAreas := workProgramAutomationPlanAreas(execution)
	if len(safeAreas) != 1 || safeAreas[0] != "operating_brief" {
		t.Fatalf("safe areas = %#v, want operating_brief", safeAreas)
	}
	if len(humanAreas) != 1 || humanAreas[0] != "eta_commitments" {
		t.Fatalf("human areas = %#v, want deduplicated eta_commitments", humanAreas)
	}
	blockedAreas := workProgramAutomationPlanBlockedAreas(execution, humanAreas)
	assertContainsString(t, blockedAreas, "source_coverage")
	assertContainsString(t, blockedAreas, "guardrail_gates")
	assertContainsString(t, blockedAreas, "adversarial_checks")
	assertContainsString(t, blockedAreas, "insight_measurement")
	assertContainsString(t, blockedAreas, "forecast_readiness")

	recommendedFocus := workProgramAutomationPlanFocus(execution, queues)
	if recommendedFocus == nil || *recommendedFocus != focus {
		t.Fatalf("focus = %#v, want execution focus", recommendedFocus)
	}
	summary := workProgramAutomationPlanSummary(execution, "blocked_source_coverage", "blocked", queues, blockedAreas, recommendedFocus)
	if !strings.Contains(summary, "blocked_source_coverage") || !strings.Contains(summary, "Blocked areas:") || !strings.Contains(summary, "source_coverage") {
		t.Fatalf("summary = %q, want blocked source coverage explanation", summary)
	}
}

func TestWorkProgramAutomationPlanAllowsAutonomyOnlyWhenAllGatesClear(t *testing.T) {
	execution := &model.WorkProgramExecutionPacket{
		SourceInstance:        stringPointer("fixture-source"),
		WorkstreamKey:         "workstream:flink-kubernetes-operator",
		ActionState:           "open",
		ExecutionState:        "autonomous_actions_ready",
		AutonomousActionReady: true,
		HumanReviewRequired:   false,
		AbsenceClaimsAllowed:  true,
		Actions: []*model.WorkAction{
			{Key: "work-action:product", DecisionState: "product_action"},
		},
		TpmReadiness: &model.WorkProgramTpmReadinessPacket{
			ReplacementState:              "autonomous_ready",
			AutonomousActionReady:         true,
			HumanReviewRequired:           false,
			ReadyFunctionCount:            1,
			SupervisedFunctionCount:       0,
			BlockedFunctionCount:          0,
			HumanRequiredFunctionCount:    0,
			BlockingGateCount:             0,
			FailedCheckCount:              0,
			EvidenceNeedCount:             0,
			MeasurementState:              "product_action_ready",
			MeasurementProductActionReady: true,
			ForecastReadinessState:        "ready",
			ForecastEtaReady:              true,
			AbsenceClaimsAllowed:          true,
			AutomationReadiness:           &model.WorkProgramAutomationReadiness{SafeAutomationAreas: []string{"operating_brief"}},
			TpmFunctionReadiness: []*model.WorkProgramTpmFunctionReadiness{
				{FunctionKey: "operating_brief", ReadinessState: "automatable", HumanRequired: false},
			},
		},
	}

	queues := workProgramAutomationPlanQueuesFor(execution)
	actionPlans := workProgramAutomationActionPlans(execution, nil)
	if !workProgramAutomationPlanAutonomyAllowed(execution) {
		t.Fatalf("autonomy not allowed for fully clear execution")
	}
	if got := workProgramAutomationPlanState(execution, queues); got != "autonomous_actions_ready" {
		t.Fatalf("plan state = %q, want autonomous_actions_ready", got)
	}
	if got := workProgramAutomationPlanAutonomyLevel("autonomous_actions_ready"); got != "autonomous" {
		t.Fatalf("autonomy level = %q, want autonomous", got)
	}
	if len(queues.autonomousActions) != 1 || len(queues.humanReviewActions) != 0 || len(queues.blockedActions) != 0 {
		t.Fatalf("action queues = autonomous:%d human:%d blocked:%d, want 1/0/0", len(queues.autonomousActions), len(queues.humanReviewActions), len(queues.blockedActions))
	}
	if len(actionPlans) != 1 || actionPlans[0].Disposition != "autonomous" || actionPlans[0].AutonomyLevel != "autonomous" || len(actionPlans[0].BlockingAreas) != 0 {
		t.Fatalf("action plan = %#v, want autonomous with no blocking areas", actionPlans)
	}
	if !strings.Contains(actionPlans[0].Reason, "All automation gates are clear") {
		t.Fatalf("action plan reason = %q, want clear automation reason", actionPlans[0].Reason)
	}
	if workProgramAutomationPlanHumanReviewRequired(execution, queues, true) {
		t.Fatalf("fully clear execution should not require human review")
	}
	if blockedAreas := workProgramAutomationPlanBlockedAreas(execution, nil); len(blockedAreas) != 0 {
		t.Fatalf("blocked areas = %#v, want none", blockedAreas)
	}

	forecastGated := *execution
	forecastGatedReadiness := *execution.TpmReadiness
	forecastGatedReadiness.ForecastEtaReady = false
	forecastGated.TpmReadiness = &forecastGatedReadiness
	forecastQueues := workProgramAutomationPlanQueuesFor(&forecastGated)
	forecastPlans := workProgramAutomationActionPlans(&forecastGated, workProgramAutomationPlanBlockedAreas(&forecastGated, nil))
	if workProgramAutomationPlanAutonomyAllowed(&forecastGated) {
		t.Fatalf("forecast-gated execution should not allow autonomy")
	}
	if len(forecastQueues.autonomousActions) != 0 || len(forecastQueues.humanReviewActions) != 1 || len(forecastQueues.blockedActions) != 0 {
		t.Fatalf("forecast-gated queues = autonomous:%d human:%d blocked:%d, want 0/1/0", len(forecastQueues.autonomousActions), len(forecastQueues.humanReviewActions), len(forecastQueues.blockedActions))
	}
	if len(forecastPlans) != 1 || forecastPlans[0].Disposition != "human_review" || forecastPlans[0].AutonomyLevel != "supervised" {
		t.Fatalf("forecast-gated action plan = %#v, want human_review/supervised", forecastPlans)
	}
	if !strings.Contains(forecastPlans[0].Reason, "Forecast output is risk triage") {
		t.Fatalf("forecast-gated reason = %q, want forecast risk triage reason", forecastPlans[0].Reason)
	}
	assertContainsString(t, workProgramAutomationPlanBlockedAreas(&forecastGated, nil), "forecast_readiness")
}

func TestWorkProgramAutomationPlanBlocksIdleAutonomyForResponsibilityValidation(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 21, 9, 45, 0, 0, time.UTC)
	seedWorkProgramAutonomousReadyFixture(t, ctx, store.Client(), source, workstream, generatedAt)
	_, responsibility := seedCandidateWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramAutomationPlanPacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("automation plan packet: %v", err)
	}

	if packet.PlanState != "human_review_required" || packet.AutonomyLevel != "supervised" {
		t.Fatalf("plan state = %s/%s, want human_review_required/supervised", packet.PlanState, packet.AutonomyLevel)
	}
	if packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("plan readiness = autonomous:%v human:%v, want false/true", packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.ResponsibilityValidationCount != 1 || len(packet.Responsibilities) != 1 || packet.Responsibilities[0].Key != responsibility.Key {
		t.Fatalf("responsibilities = count:%d rows:%#v, want validation row", packet.ResponsibilityValidationCount, packet.Responsibilities)
	}
	assertContainsString(t, packet.HumanRequiredAreas, "responsibility_validation")
	assertContainsString(t, packet.BlockedAutomationAreas, "responsibility_validation")
	if packet.ExecutionPacket == nil || packet.ExecutionPacket.ExecutionState != "human_review_required" || packet.ExecutionPacket.AutonomousActionReady {
		t.Fatalf("execution packet = %#v, want responsibility-gated human review", packet.ExecutionPacket)
	}
	if !strings.Contains(packet.AutomationSummary, "1 responsibility validation(s)") {
		t.Fatalf("summary = %q, want responsibility validation count", packet.AutomationSummary)
	}
}

func TestWorkProgramAutomationEvidenceNeedsDoNotLeakAcrossActions(t *testing.T) {
	productAction := &model.WorkAction{
		Key:           "work-action:product",
		ActionState:   "open",
		DecisionState: "product_action",
		SubjectKey:    "subject:product",
	}
	otherAction := &model.WorkAction{
		Key:           "work-action:other",
		ActionState:   "open",
		DecisionState: "validation_lead",
		SubjectKey:    "subject:other",
	}
	evidenceNeed := &model.WorkProgramAutomationEvidenceNeed{
		Key:         "need:product",
		GateKey:     "blocker_clearance",
		ActionKey:   stringPointer("work-action:product"),
		ActionState: stringPointer("open"),
		TargetKey:   stringPointer("subject:product"),
	}
	globalNeed := &model.WorkProgramAutomationEvidenceNeed{
		Key:     "need:global",
		GateKey: "source_coverage",
	}
	execution := &model.WorkProgramExecutionPacket{
		EvidenceNeeds: []*model.WorkProgramAutomationEvidenceNeed{evidenceNeed, globalNeed},
	}

	productNeeds := workProgramAutomationActionEvidenceNeeds(execution.EvidenceNeeds, productAction, []string{"blocker_clearance", "source_coverage"})
	if len(productNeeds) != 1 || productNeeds[0].Key != "need:product" {
		t.Fatalf("product needs = %#v, want only action-specific need", productNeeds)
	}
	otherNeeds := workProgramAutomationActionEvidenceNeeds(execution.EvidenceNeeds, otherAction, []string{"blocker_clearance", "source_coverage"})
	if len(otherNeeds) != 1 || otherNeeds[0].Key != "need:global" {
		t.Fatalf("other needs = %#v, want only global need", otherNeeds)
	}
}

func TestWorkProgramAutomationActionPlansUseHydratedEvidenceNeeds(t *testing.T) {
	action := &model.WorkAction{
		Key:               "work-action:product",
		ActionState:       "open",
		DecisionState:     "product_action",
		SubjectKey:        "subject:product",
		RecommendedAction: stringPointer("Ask the owner for the concrete blocker state."),
	}
	execution := &model.WorkProgramExecutionPacket{
		ExecutionState:       "blocked_review_queue",
		AbsenceClaimsAllowed: true,
		Actions:              []*model.WorkAction{action},
		TpmReadiness: &model.WorkProgramTpmReadinessPacket{
			ReplacementState:       "blocked",
			BlockingGateCount:      1,
			ForecastReadinessState: "ready",
			ForecastEtaReady:       true,
			AbsenceClaimsAllowed:   true,
		},
	}
	hydrated := []*model.WorkProgramAutomationEvidenceNeed{
		{
			Key:               "need:product",
			GateKey:           "blocker_clearance",
			ActionKey:         stringPointer("work-action:product"),
			TargetKey:         stringPointer("subject:product"),
			RecommendedAction: "Confirm blocker clearance with the owner.",
		},
	}

	plans := workProgramAutomationActionPlansWithEvidence(execution, []string{"blocker_clearance"}, hydrated)
	if len(plans) != 1 {
		t.Fatalf("plans = %d, want 1", len(plans))
	}
	if len(plans[0].EvidenceNeeds) != 1 || plans[0].EvidenceNeeds[0].Key != "need:product" {
		t.Fatalf("plan evidence = %#v, want hydrated action-specific need", plans[0].EvidenceNeeds)
	}
	if plans[0].RecommendedAction == nil || *plans[0].RecommendedAction != "Ask the owner for the concrete blocker state." {
		t.Fatalf("recommended action = %#v, want action recommendation to win", plans[0].RecommendedAction)
	}
}

func stringPointer(value string) *string {
	return &value
}

func assertContainsString(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("values = %#v, want %q", values, want)
}
