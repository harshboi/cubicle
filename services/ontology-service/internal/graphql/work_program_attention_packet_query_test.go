package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workblockerimpact"
	"cubicle/services/ontology-service/ent/workitemforecast"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/ent/workresponsibility"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramAttentionPacketRanksUnifiedTPMQueue(t *testing.T) {
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
	queryWorkstream := "workstream:" + workstream
	subjectKey := "apache/flink-kubernetes-operator#100"
	otherSubjectKey := "other/repo#999"
	generatedAt := time.Date(2026, 6, 22, 9, 30, 0, 0, time.UTC)

	evidenceRow, err := store.Client().Evidence.Create().
		SetKey("evidence:attention:evidence-need").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_program_evidence_need").
		SetClaimField("forecast_readiness").
		SetLocatorKind("forecast_backtest").
		SetLocator("fixture://forecast-backtest").
		SetExcerpt("Forecast backtest is still gated; keep ETA claims out of product actions.").
		SetObservedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("evidence-need:forecast").
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence: %v", err)
	}

	action, err := store.Client().WorkAction.Create().
		SetKey("work-action:attention:product").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecision("owner_followup").
		SetDecisionReason("owner status needs confirmation").
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetDueBucket(workaction.DueBucketNow).
		SetOwnerKey("github:maintainer").
		SetOwnerSource("fixture").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("attention:product").
		SetRankScore(88).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:attention:subject").
		SetWorkAction(action).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Attention queue subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(88).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("attention:subject").
		SetRankScore(88).
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	_, err = store.Client().WorkResponsibility.Create().
		SetKey("work-responsibility:attention:candidate").
		SetSubjectKind(workresponsibility.SubjectKindWorkAction).
		SetSubjectKey(action.Key).
		SetWorkActionID(action.ID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey("github:maintainer").
		SetPartySource("generated.action_owner").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAccountable).
		SetBasisKind(workresponsibility.BasisKindGeneratedCandidate).
		SetResponsibilityState(workresponsibility.ResponsibilityStateCandidate).
		SetResponsibilityStateReason("generated action owner hint requires validation before product accountability").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID("attention:candidate").
		SetRankScore(55).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create candidate responsibility: %v", err)
	}
	_, err = store.Client().WorkResponsibility.Create().
		SetKey("work-responsibility:attention:active-source-native").
		SetSubjectKind(workresponsibility.SubjectKindWorkAction).
		SetSubjectKey(action.Key).
		SetWorkActionID(action.ID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey("github:maintainer").
		SetPartySource("github.pr.author").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAuthor).
		SetBasisKind(workresponsibility.BasisKindSourceNative).
		SetResponsibilityState(workresponsibility.ResponsibilityStateActive).
		SetResponsibilityStateReason("source-native field supports active responsibility").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID("attention:active-source-native").
		SetRankScore(500).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create active source-native responsibility: %v", err)
	}

	otherAction, err := store.Client().WorkAction.Create().
		SetKey("work-action:attention:off-scope").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey(otherSubjectKey).
		SetDueBucket(workaction.DueBucketNow).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("attention:off-scope").
		SetRankScore(200).
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope action: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:attention:off-scope").
		SetWorkAction(otherAction).
		SetWorkstreamKey("other-workstream").
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(otherSubjectKey).
		SetTitle("Off-scope attention subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(200).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("attention:off-scope").
		SetRankScore(200).
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope program item: %v", err)
	}
	_, err = store.Client().WorkResponsibility.Create().
		SetKey("work-responsibility:attention:off-scope").
		SetSubjectKind(workresponsibility.SubjectKindWorkAction).
		SetSubjectKey(otherAction.Key).
		SetWorkActionID(otherAction.ID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey("github:maintainer").
		SetPartySource("generated.action_owner").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAccountable).
		SetBasisKind(workresponsibility.BasisKindGeneratedCandidate).
		SetResponsibilityState(workresponsibility.ResponsibilityStateCandidate).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID("attention:off-scope").
		SetRankScore(500).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope responsibility: %v", err)
	}

	blocker, err := store.Client().WorkBlocker.Create().
		SetKey("work-blocker:attention:subject").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityCritical).
		SetSubjectKind(workblocker.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetDecisionState(workblocker.DecisionStateValidationLead).
		SetReviewState(workblocker.ReviewStateNeedsMoreData).
		SetTruthLabel(workblocker.TruthLabelPartial).
		SetActionabilityLabel(workblocker.ActionabilityLabelNeedsOwner).
		SetLabelQuality(workblocker.LabelQualityCandidate).
		SetReviewerKind(workblocker.ReviewerKindSystem).
		SetTitle("Owner decision blocks the stream").
		SetRecommendedAction("Confirm whether the owner intends to merge or park the PR.").
		SetRankScore(85).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker").
		SetExternalID("attention:blocker").
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	_, err = store.Client().WorkBlockerImpact.Create().
		SetKey("work-blocker-impact:attention:subject").
		SetWorkBlocker(blocker).
		SetWorkAction(action).
		SetImpactKind(workblockerimpact.ImpactKindWorkstream).
		SetImpactState(workblockerimpact.ImpactStateActive).
		SetImpactScore(120).
		SetSeverity(workblockerimpact.SeverityCritical).
		SetBlockerKind(workblockerimpact.BlockerKindDecision).
		SetAffectedKind(workblockerimpact.AffectedKindWorkstream).
		SetAffectedKey(workstream).
		SetSubjectKind(workblockerimpact.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetPathLength(1).
		SetTitle("Blocker affects the stream").
		SetRecommendedAction("Escalate the owner decision before relying on the forecast queue.").
		SetRankScore(90).
		SetConfidence(0.7).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker_impact").
		SetExternalID("attention:impact").
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocker impact: %v", err)
	}

	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:attention:subject").
		SetWorkAction(action).
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetSubjectState("open").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetRiskScore(95).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetReadinessReason("ETA remains gated by backtest quality.").
		SetForecastedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("attention:forecast").
		Save(ctx)
	if err != nil {
		t.Fatalf("create forecast: %v", err)
	}
	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:attention:off-scope").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(otherSubjectKey).
		SetSubjectState("open").
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetRiskScore(200).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetForecastedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("attention:forecast:off-scope").
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope forecast: %v", err)
	}

	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:attention:forecast").
		SetLatestEvidence(evidenceRow).
		SetEvidenceCount(1).
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("forecast_readiness").
		SetEvidenceKind("forecast_backtest_quality").
		SetPriority(workprogramevidenceneed.PriorityCritical).
		SetTargetKind("pull_request").
		SetTargetKey(subjectKey).
		SetActionKey(action.Key).
		SetActionState("open").
		SetExecutionState("needs_human_review").
		SetBackingActionCount(1).
		SetCurrentCount(0).
		SetRequiredCount(5).
		SetMissingCount(5).
		SetRecommendedAction("Review forecast backtest quality before making ETA commitments.").
		SetNextExecutionStep("Keep ETA claims blocked until forecast backtest quality clears.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-22T09:30:00Z|forecast_readiness:attention").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create forecast evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:attention:blocker").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("blocker_clearance").
		SetEvidenceKind("blocker_owner_status").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey(subjectKey).
		SetActionKey(action.Key).
		SetActionState("open").
		SetExecutionState("action_open").
		SetBackingActionCount(1).
		SetCurrentCount(0).
		SetRequiredCount(3).
		SetMissingCount(3).
		SetRecommendedAction("Confirm blocker clearance with the owner.").
		SetNextExecutionStep("Queue blocker-clearance follow-up.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-22T09:30:00Z|blocker_clearance:attention").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocker evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:attention:off-scope").
		SetWorkstreamKey("other-workstream").
		SetGeneratedAt(generatedAt).
		SetGateKey("forecast_readiness").
		SetEvidenceKind("forecast_backtest_quality").
		SetPriority(workprogramevidenceneed.PriorityCritical).
		SetTargetKind("pull_request").
		SetTargetKey(otherSubjectKey).
		SetExecutionState("needs_human_review").
		SetRecommendedAction("Off-scope evidence should not leak.").
		SetNextExecutionStep("Do not include this row.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("other-workstream|2026-06-22T09:30:00Z|forecast_readiness:off-scope").
		SetRankScore(500).
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:attention").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("human_review_required").
		SetReadinessScore(55).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetSafeAutomationAreas("operating_brief").
		SetHumanRequiredAreas("forecast_readiness\nblocker_clearance").
		SetRationale("Fixture attention queue needs human review.").
		SetRequiredEvidence("forecast backtest quality\nblocker owner status").
		SetEvidenceNeedCount(2).
		SetTpmFunctionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID("flink-kubernetes-operator|2026-06-22T09:30:00Z|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create attention readiness: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	displayLimit := 3
	evidenceLimit := 10
	sourceArg := source
	capped, err := resolver.WorkProgramAttentionPacket(ctx, queryWorkstream, &displayLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("attention packet capped: %v", err)
	}
	if capped.PriorityCount != 7 || len(capped.Priorities) != 3 {
		t.Fatalf("capped priorities = total:%d rows:%d, want 7 total and 3 returned", capped.PriorityCount, len(capped.Priorities))
	}
	if capped.GeneratedAt == nil || *capped.GeneratedAt != "2026-06-22T09:30:00Z" {
		t.Fatalf("capped generatedAt = %#v, want readiness run timestamp", capped.GeneratedAt)
	}
	if capped.UrgentCount != 6 || capped.HumanRequiredCount != 6 {
		t.Fatalf("capped counts = urgent:%d human:%d, want total-scope 6/6", capped.UrgentCount, capped.HumanRequiredCount)
	}
	if capped.AttentionState != "urgent_human_review" {
		t.Fatalf("attention state = %s, want urgent_human_review", capped.AttentionState)
	}
	if capped.Priorities[0].PriorityKind != "blocker_impact" || capped.Priorities[1].PriorityKind != "blocker" || capped.Priorities[2].PriorityKind != "action" {
		t.Fatalf("top priority order = %s/%s/%s, want blocker_impact/blocker/action", capped.Priorities[0].PriorityKind, capped.Priorities[1].PriorityKind, capped.Priorities[2].PriorityKind)
	}
	if capped.RecommendedFocus == nil || !strings.Contains(*capped.RecommendedFocus, "Escalate the owner decision") {
		t.Fatalf("recommended focus = %#v, want top blocker-impact action", capped.RecommendedFocus)
	}
	if !strings.Contains(capped.AutomationSummary, "7 priority item(s)") || !strings.Contains(capped.AutomationSummary, "6 urgent item(s)") || !strings.Contains(capped.AutomationSummary, "6 human-required item(s)") {
		t.Fatalf("automation summary = %q, want total-scope counts", capped.AutomationSummary)
	}

	fullLimit := 10
	full, err := resolver.WorkProgramAttentionPacket(ctx, queryWorkstream, &fullLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("attention packet full: %v", err)
	}
	if full.SourceInstance == nil || *full.SourceInstance != source {
		t.Fatalf("packet source = %#v, want %s", full.SourceInstance, source)
	}
	if full.WorkstreamKey != queryWorkstream {
		t.Fatalf("packet workstream = %s, want %s", full.WorkstreamKey, queryWorkstream)
	}
	if full.GeneratedAt == nil || *full.GeneratedAt != "2026-06-22T09:30:00Z" || full.TpmReadiness.GeneratedAt == nil || *full.TpmReadiness.GeneratedAt != *full.GeneratedAt {
		t.Fatalf("packet generatedAt = attention:%#v readiness:%#v, want matching materialization timestamp", full.GeneratedAt, full.TpmReadiness.GeneratedAt)
	}
	assertAttentionPriorityKinds(t, full.Priorities, map[string]bool{
		"action":         true,
		"blocker":        true,
		"blocker_impact": true,
		"forecast":       true,
		"evidence_need":  true,
		"responsibility": true,
	})
	for _, priority := range full.Priorities {
		if strings.Contains(priority.Key, "off-scope") || (priority.SubjectKey != nil && *priority.SubjectKey == otherSubjectKey) {
			t.Fatalf("attention packet leaked off-scope priority: %#v", priority)
		}
	}
	evidenceNeedPriority := findAttentionEvidenceNeedPriority(full.Priorities, "forecast_readiness")
	if evidenceNeedPriority == nil {
		t.Fatalf("missing forecast evidence-need priority: %#v", full.Priorities)
	}
	if evidenceNeedPriority.ProductActionAllowed || !evidenceNeedPriority.HumanRequired {
		t.Fatalf("evidence-need gate = product:%v human:%v, want false/true", evidenceNeedPriority.ProductActionAllowed, evidenceNeedPriority.HumanRequired)
	}
	if evidenceNeedPriority.Evidence == nil || evidenceNeedPriority.Evidence.LocatorKind == nil || *evidenceNeedPriority.Evidence.LocatorKind != "forecast_backtest" {
		t.Fatalf("evidence-need evidence = %#v, want forecast_backtest summary", evidenceNeedPriority.Evidence)
	}
	responsibilityPriority := findAttentionResponsibilityPriority(full.Priorities)
	if responsibilityPriority == nil {
		t.Fatalf("missing responsibility priority: %#v", full.Priorities)
	}
	if responsibilityPriority.Responsibility == nil || responsibilityPriority.Responsibility.BasisKind != "generated_candidate" || responsibilityPriority.Responsibility.ResponsibilityState != "candidate" {
		t.Fatalf("responsibility priority = %#v, want generated candidate responsibility", responsibilityPriority)
	}
	if responsibilityPriority.ProductActionAllowed || !responsibilityPriority.HumanRequired {
		t.Fatalf("responsibility gate = product:%v human:%v, want false/true", responsibilityPriority.ProductActionAllowed, responsibilityPriority.HumanRequired)
	}
}

func TestWorkProgramAttentionStatePrioritizesResponsibilityReviewOverAutonomousExecution(t *testing.T) {
	priorities := []*model.WorkProgramAttentionPriority{
		{
			PriorityKind:  "responsibility",
			Urgency:       "medium",
			HumanRequired: true,
		},
	}
	execution := &model.WorkProgramExecutionPacket{ExecutionState: "autonomous_actions_ready"}
	if got := workProgramAttentionState(priorities, nil, execution, nil, nil); got != "responsibility_review_required" {
		t.Fatalf("attention state = %q, want responsibility_review_required", got)
	}
}

func assertAttentionPriorityKinds(t *testing.T, priorities []*model.WorkProgramAttentionPriority, want map[string]bool) {
	t.Helper()
	seen := map[string]bool{}
	for _, priority := range priorities {
		if priority != nil {
			seen[priority.PriorityKind] = true
		}
	}
	for kind := range want {
		if !seen[kind] {
			t.Fatalf("missing priority kind %s in %#v", kind, priorities)
		}
	}
}

func findAttentionEvidenceNeedPriority(priorities []*model.WorkProgramAttentionPriority, gateKey string) *model.WorkProgramAttentionPriority {
	for _, priority := range priorities {
		if priority == nil || priority.PriorityKind != "evidence_need" || priority.EvidenceNeed == nil {
			continue
		}
		if priority.EvidenceNeed.GateKey == gateKey {
			return priority
		}
	}
	return nil
}

func findAttentionResponsibilityPriority(priorities []*model.WorkProgramAttentionPriority) *model.WorkProgramAttentionPriority {
	for _, priority := range priorities {
		if priority == nil || priority.PriorityKind != "responsibility" {
			continue
		}
		return priority
	}
	return nil
}
