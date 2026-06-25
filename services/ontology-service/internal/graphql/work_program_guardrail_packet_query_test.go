package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workprogramadversarialcheck"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkProgramGuardrailPacketAggregatesBlockingSafetyRows(t *testing.T) {
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
		SetKey("work-program-quality-gate:forecast").
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
	_, err = store.Client().WorkProgramQualityGate.Create().
		SetKey("work-program-quality-gate:measurement").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("measurement_precision").
		SetGateState("gated").
		SetBlocking(true).
		SetDetail("Measurement gate blocks autonomous product action.").
		SetRecommendedAction("Label current insights before product-action automation.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_quality_gate").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|measurement_precision").
		SetRankScore(80).
		Save(ctx)
	if err != nil {
		t.Fatalf("create second quality gate: %v", err)
	}
	_, err = store.Client().WorkProgramAdversarialCheck.Create().
		SetKey("work-program-adversarial-check:eta-overclaim").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCheckKind("eta_overclaim").
		SetCheckState(workprogramadversarialcheck.CheckStateFail).
		SetSeverity(workprogramadversarialcheck.SeverityHigh).
		SetTitle("ETA claim overclaimed").
		SetDetail("Forecast is not ETA ready.").
		SetRecommendedAction("Do not present ETA commitments.").
		SetBlockingGateKeys("forecast_readiness").
		SetEvidenceRefs("forecast_backtest summary").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_adversarial_check").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:eta_overclaim").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create adversarial check: %v", err)
	}
	_, err = store.Client().WorkProgramAdversarialCheck.Create().
		SetKey("work-program-adversarial-check:source-absence").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCheckKind("source_absence_claims").
		SetCheckState(workprogramadversarialcheck.CheckStateFail).
		SetSeverity(workprogramadversarialcheck.SeverityCritical).
		SetTitle("Absence claims unsafe").
		SetDetail("Source coverage is limited.").
		SetRecommendedAction("Disable absence claims until source coverage is complete.").
		SetBlockingGateKeys("source_coverage").
		SetEvidenceRefs("source_coverage summary").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_adversarial_check").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:source_absence_claims").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create second adversarial check: %v", err)
	}
	_, err = store.Client().WorkProgramAdversarialCheck.Create().
		SetKey("work-program-adversarial-check:decision-owner").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCheckKind("product_decision").
		SetCheckState(workprogramadversarialcheck.CheckStateWarning).
		SetSeverity(workprogramadversarialcheck.SeverityMedium).
		SetTitle("Owner decision needs review").
		SetDetail("Product decision still needs owner confirmation.").
		SetRecommendedAction("Confirm the owner decision before automated action.").
		SetBlockingGateKeys("measurement_precision").
		SetEvidenceRefs("product_decision summary").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_adversarial_check").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:product_decision").
		SetRankScore(70).
		Save(ctx)
	if err != nil {
		t.Fatalf("create warning adversarial check: %v", err)
	}
	_, err = store.Client().WorkProgramBriefCaveat.Create().
		SetKey("work-program-brief-caveat:forecast-gated").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCaveatKey("forecast_gated").
		SetSeverity("warning").
		SetTitle("Forecast output remains gated").
		SetDetail("Forecasts support risk triage but not ETA commitments.").
		SetRecommendedAction("Label forecast fields as review-only.").
		SetEvidenceRef("forecast_backtest summary").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_brief_caveat").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|forecast_gated").
		SetRankScore(75).
		Save(ctx)
	if err != nil {
		t.Fatalf("create caveat: %v", err)
	}
	_, err = store.Client().WorkProgramBriefCaveat.Create().
		SetKey("work-program-brief-caveat:coverage-limited").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetCaveatKey("coverage_limited").
		SetSeverity("warning").
		SetTitle("Source coverage limited").
		SetDetail("Generated-only and partial rows are review leads.").
		SetRecommendedAction("Avoid absence claims until coverage is complete.").
		SetEvidenceRef("source_coverage summary").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_brief_caveat").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|coverage_limited").
		SetRankScore(65).
		Save(ctx)
	if err != nil {
		t.Fatalf("create second caveat: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:forecast-readiness").
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
		SetKey("work-program-evidence-need:measurement-labels").
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
		SetKey("work-program-automation-readiness:blocked").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("blocked").
		SetReadinessScore(35).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetSafeAutomationAreas("agenda_summarization\nsource_citation").
		SetHumanRequiredAreas("eta_commitments\nmeasurement_claims").
		SetRationale("Forecast and measurement gates block autonomous action.").
		SetRequiredEvidence("forecast backtest quality").
		SetBlockingGateKeys("forecast_readiness").
		SetQualityGateCount(2).
		SetBlockingGateCount(2).
		SetEvidenceNeedCount(2).
		SetTpmFunctionCount(4).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create automation readiness: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rowLimit := 1
	evidenceLimit := 1
	sourceArg := source
	packet, err := resolver.WorkProgramGuardrailPacket(ctx, "workstream:flink-kubernetes-operator", &rowLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("guardrail packet: %v", err)
	}

	if packet.SourceInstance == nil || *packet.SourceInstance != source {
		t.Fatalf("packet source = %#v, want %s", packet.SourceInstance, source)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != "2026-06-21T07:03:16Z" {
		t.Fatalf("packet generatedAt = %#v, want readiness run timestamp", packet.GeneratedAt)
	}
	if packet.WorkstreamKey != "workstream:flink-kubernetes-operator" {
		t.Fatalf("packet workstream = %s, want query workstream", packet.WorkstreamKey)
	}
	if packet.GuardrailState != "blocked" || packet.ReadinessState != "blocked" {
		t.Fatalf("packet states = guardrail:%s readiness:%s, want blocked/blocked", packet.GuardrailState, packet.ReadinessState)
	}
	if packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("packet readiness flags = autonomous:%v human:%v, want false/true", packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.BlockingGateCount != 2 || packet.FailedCheckCount != 2 || packet.WarningCheckCount != 1 || packet.CaveatCount != 2 || packet.EvidenceNeedCount != 2 {
		t.Fatalf("packet counts = gates:%d failed:%d warning:%d caveats:%d evidence:%d, want 2/2/1/2/2", packet.BlockingGateCount, packet.FailedCheckCount, packet.WarningCheckCount, packet.CaveatCount, packet.EvidenceNeedCount)
	}
	if packet.FailedAdversarialCheckCount != 2 || packet.WarningAdversarialCheckCount != 1 {
		t.Fatalf("packet adversarial check counts = failed:%d warning:%d, want 2/1", packet.FailedAdversarialCheckCount, packet.WarningAdversarialCheckCount)
	}
	if packet.AutomationReadiness == nil || packet.AutomationReadiness.ReadinessState != "blocked" {
		t.Fatalf("packet readiness row = %#v, want blocked row", packet.AutomationReadiness)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Do not present ETA commitments") {
		t.Fatalf("packet recommended focus = %#v, want failed check action", packet.RecommendedFocus)
	}
	if len(packet.QualityGates) != 1 || len(packet.AdversarialChecks) != 1 || len(packet.Caveats) != 1 || len(packet.EvidenceNeeds) != 1 {
		t.Fatalf("packet row lengths = gates:%d checks:%d caveats:%d evidence:%d, want 1/1/1/1 because rows are capped separately from counts", len(packet.QualityGates), len(packet.AdversarialChecks), len(packet.Caveats), len(packet.EvidenceNeeds))
	}
	if !strings.Contains(packet.AutomationSummary, "2 blocking gate(s)") || !strings.Contains(packet.AutomationSummary, "2 failed adversarial check(s)") || !strings.Contains(packet.AutomationSummary, "1 warning adversarial check(s)") || !strings.Contains(packet.AutomationSummary, "2 evidence need(s)") {
		t.Fatalf("packet automation summary = %q, want blocking and failed counts", packet.AutomationSummary)
	}
}

func TestWorkProgramGuardrailPacketBlocksAutonomyWhenEvidenceNeedsRemain(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:autonomous-contradiction").
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
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:00:00Z|measurement_precision:forecast_risk").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:autonomous-contradiction").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("ready").
		SetReadinessScore(95).
		SetAutonomousActionReady(true).
		SetHumanReviewRequired(false).
		SetSafeAutomationAreas("operating_brief").
		SetHumanRequiredAreas("").
		SetRationale("Persisted row is stale relative to evidence queue.").
		SetRequiredEvidence("").
		SetBlockingGateKeys("").
		SetQualityGateCount(0).
		SetBlockingGateCount(0).
		SetEvidenceNeedCount(0).
		SetTpmFunctionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:00:00Z|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create automation readiness: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rowLimit := 20
	evidenceLimit := 1
	sourceArg := source
	packet, err := resolver.WorkProgramGuardrailPacket(ctx, "workstream:flink-kubernetes-operator", &rowLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("guardrail packet: %v", err)
	}
	if packet.GuardrailState != "human_review_required" {
		t.Fatalf("guardrail state = %q, want human_review_required", packet.GuardrailState)
	}
	if packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("readiness flags = autonomous:%v human:%v, want false/true when evidence needs remain", packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.EvidenceNeedCount != 1 || len(packet.EvidenceNeeds) != 1 {
		t.Fatalf("evidence count/rows = %d/%d, want 1/1", packet.EvidenceNeedCount, len(packet.EvidenceNeeds))
	}
}

func TestWorkProgramGuardrailPacketBlocksAutonomyWhenResponsibilityNeedsValidation(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 21, 8, 30, 0, 0, time.UTC)
	seedWorkProgramAutonomousReadyFixture(t, ctx, store.Client(), source, workstream, generatedAt)
	_, responsibility := seedCandidateWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rowLimit := 20
	evidenceLimit := 20
	sourceArg := source
	packet, err := resolver.WorkProgramGuardrailPacket(ctx, "workstream:flink-kubernetes-operator", &rowLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("guardrail packet: %v", err)
	}

	if packet.GuardrailState != "human_review_required" || packet.AutonomousActionReady || !packet.HumanReviewRequired {
		t.Fatalf("packet guardrail = state:%s autonomous:%v human:%v, want human_review_required/false/true", packet.GuardrailState, packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.ResponsibilityValidationCount != 1 || len(packet.Responsibilities) != 1 || packet.Responsibilities[0].Key != responsibility.Key {
		t.Fatalf("responsibilities = count:%d rows:%#v, want validation row", packet.ResponsibilityValidationCount, packet.Responsibilities)
	}
	if packet.AutomationReadiness == nil || packet.AutomationReadiness.AutonomousActionReady || !packet.AutomationReadiness.HumanReviewRequired || packet.AutomationReadiness.ReadinessState != "human_review_required" {
		t.Fatalf("nested readiness = %#v, want responsibility-clamped human review", packet.AutomationReadiness)
	}
	assertContainsString(t, packet.AutomationReadiness.HumanRequiredAreas, "responsibility_validation")
	assertContainsString(t, packet.AutomationReadiness.RequiredEvidence, "validated_accountable_owner")
	if !strings.Contains(packet.AutomationReadiness.Rationale, "responsibility validation") {
		t.Fatalf("nested readiness rationale = %q, want responsibility validation rationale", packet.AutomationReadiness.Rationale)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "generated owner routing") {
		t.Fatalf("packet focus = %#v, want responsibility validation focus", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "1 responsibility validation(s) require human review") {
		t.Fatalf("summary = %q, want responsibility validation count", packet.AutomationSummary)
	}
}

func TestWorkProgramGuardrailPacketUsesAutomationReadinessRunBoundary(t *testing.T) {
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
	olderRun := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)
	newerRun := olderRun.Add(2 * time.Hour)

	_, err = store.Client().WorkProgramQualityGate.Create().
		SetKey("work-program-quality-gate:old:forecast").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(olderRun).
		SetGateKey("forecast_readiness").
		SetGateState("gated").
		SetBlocking(true).
		SetDetail("Old forecast gate should not leak into newer readiness.").
		SetRecommendedAction("Ignore stale gate when newer run has no gate.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_quality_gate").
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:00:00Z|forecast_readiness").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old quality gate: %v", err)
	}
	_, err = store.Client().WorkProgramAdversarialCheck.Create().
		SetKey("work-program-adversarial-check:old:eta").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(olderRun).
		SetCheckKind("eta_overclaim").
		SetCheckState(workprogramadversarialcheck.CheckStateFail).
		SetSeverity(workprogramadversarialcheck.SeverityCritical).
		SetTitle("Old ETA check should not leak").
		SetDetail("Older materialization failed ETA guardrails.").
		SetRecommendedAction("Ignore stale failed checks for newer run packets.").
		SetBlockingGateKeys("forecast_readiness").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_adversarial_check").
		SetExternalID("flink-kubernetes-operator:2026-06-21T08:00:00Z:eta_overclaim").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old adversarial check: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:old:forecast").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(olderRun).
		SetGateKey("forecast_readiness").
		SetEvidenceKind("forecast_backtest_quality").
		SetPriority(workprogramevidenceneed.PriorityCritical).
		SetTargetKind("workstream").
		SetTargetKey("workstream:flink-kubernetes-operator").
		SetExecutionState("needs_human_review").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Old evidence need should not leak into newer readiness.").
		SetNextExecutionStep("Ignore stale evidence queue.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:00:00Z|forecast_readiness:old").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:old").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(olderRun).
		SetReadinessState("blocked").
		SetReadinessScore(10).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetRationale("Older run was blocked.").
		SetBlockingGateKeys("forecast_readiness").
		SetQualityGateCount(1).
		SetBlockingGateCount(1).
		SetEvidenceNeedCount(1).
		SetTpmFunctionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:00:00Z|automation_readiness").
		SetRankScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old readiness: %v", err)
	}
	_, err = store.Client().WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:new").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(newerRun).
		SetReadinessState("ready").
		SetReadinessScore(95).
		SetAutonomousActionReady(true).
		SetHumanReviewRequired(false).
		SetSafeAutomationAreas("operating_brief").
		SetRationale("Newer run cleared generated guardrails.").
		SetQualityGateCount(0).
		SetBlockingGateCount(0).
		SetEvidenceNeedCount(0).
		SetTpmFunctionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-21T10:00:00Z|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create new readiness: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rowLimit := 20
	evidenceLimit := 20
	sourceArg := source
	packet, err := resolver.WorkProgramGuardrailPacket(ctx, "workstream:flink-kubernetes-operator", &rowLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("guardrail packet: %v", err)
	}
	if packet.GuardrailState != "autonomous_ready" || !packet.AutonomousActionReady || packet.HumanReviewRequired {
		t.Fatalf("packet guardrail = state:%s autonomous:%v human:%v, want autonomous_ready/true/false", packet.GuardrailState, packet.AutonomousActionReady, packet.HumanReviewRequired)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != "2026-06-21T10:00:00Z" {
		t.Fatalf("packet generatedAt = %#v, want newer readiness run timestamp", packet.GeneratedAt)
	}
	if packet.BlockingGateCount != 0 || packet.FailedCheckCount != 0 || packet.EvidenceNeedCount != 0 {
		t.Fatalf("packet leaked stale run counts = gates:%d failed:%d evidence:%d, want 0/0/0", packet.BlockingGateCount, packet.FailedCheckCount, packet.EvidenceNeedCount)
	}
	if len(packet.QualityGates) != 0 || len(packet.AdversarialChecks) != 0 || len(packet.EvidenceNeeds) != 0 {
		t.Fatalf("packet leaked stale run rows = gates:%d checks:%d evidence:%d, want 0/0/0", len(packet.QualityGates), len(packet.AdversarialChecks), len(packet.EvidenceNeeds))
	}
	if packet.AutomationReadiness == nil || packet.AutomationReadiness.ReadinessState != "ready" {
		t.Fatalf("packet readiness = %#v, want newer ready row", packet.AutomationReadiness)
	}
}
