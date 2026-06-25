package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workblockerimpact"
	"cubicle/services/ontology-service/ent/workitemforecast"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkProgramBlockerPacketAggregatesScopedRows(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 21, 13, 40, 0, 0, time.UTC)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, generatedAt)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:blocker-subject").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Scoped blocker subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:blocker-subject").
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:blocker-second-subject").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(secondSubjectKey).
		SetTitle("Second scoped forecast subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(90).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:blocker-second-subject").
		Save(ctx)
	if err != nil {
		t.Fatalf("create second program item: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:other-workstream").
		SetWorkstreamKey("other-workstream").
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(offScopeSubjectKey).
		SetTitle("Off-scope blocker subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(99).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:other-workstream").
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope program item: %v", err)
	}

	_, err = store.Client().WorkBlocker.Create().
		SetKey("work-blocker:scoped").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityHigh).
		SetSubjectKind(workblocker.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetDecisionState(workblocker.DecisionStateValidationLead).
		SetReviewState(workblocker.ReviewStateNeedsMoreData).
		SetTruthLabel(workblocker.TruthLabelPartial).
		SetActionabilityLabel(workblocker.ActionabilityLabelNeedsOwner).
		SetLabelQuality(workblocker.LabelQualityCandidate).
		SetReviewerKind(workblocker.ReviewerKindSystem).
		SetTitle("Owner decision is blocking merge").
		SetRecommendedAction("Confirm the owner decision before presenting this as clear to merge.").
		SetRankScore(98).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker").
		SetExternalID("work-blocker:scoped").
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create scoped blocker: %v", err)
	}
	_, err = store.Client().WorkBlocker.Create().
		SetKey("work-blocker:off-scope").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityCritical).
		SetSubjectKind(workblocker.SubjectKindUnknown).
		SetSubjectKey(offScopeSubjectKey).
		SetTitle("Off-scope blocker").
		SetRankScore(100).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker").
		SetExternalID("work-blocker:off-scope").
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-scope blocker: %v", err)
	}

	_, err = store.Client().WorkBlockerImpact.Create().
		SetKey("work-blocker-impact:scoped").
		SetImpactKind(workblockerimpact.ImpactKindWorkstream).
		SetImpactState(workblockerimpact.ImpactStateActive).
		SetImpactScore(97).
		SetSeverity(workblockerimpact.SeverityHigh).
		SetBlockerKind(workblockerimpact.BlockerKindDecision).
		SetAffectedKind(workblockerimpact.AffectedKindWorkstream).
		SetAffectedKey(workstream).
		SetSubjectKind(workblockerimpact.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetPathLength(1).
		SetTitle("Decision blocker affects the workstream").
		SetRecommendedAction("Review the blocking decision in the workstream risk queue.").
		SetRankScore(97).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker_impact").
		SetExternalID("work-blocker-impact:scoped").
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create scoped impact: %v", err)
	}

	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:scoped").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetSubjectState("open").
		SetRiskBand(workitemforecast.RiskBandHigh).
		SetRiskScore(91).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetForecastedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:scoped").
		Save(ctx)
	if err != nil {
		t.Fatalf("create scoped forecast: %v", err)
	}
	_, err = store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:scoped-second").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindUnknown).
		SetSubjectKey(secondSubjectKey).
		SetSubjectState("open").
		SetRiskBand(workitemforecast.RiskBandHigh).
		SetRiskScore(90).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetForecastedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("work-item-forecast:scoped-second").
		Save(ctx)
	if err != nil {
		t.Fatalf("create second scoped forecast: %v", err)
	}

	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:blocker-subject").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("blocker_source_validation").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey(subjectKey).
		SetExecutionState("blocker_source_review_needed").
		SetBackingActionCount(1).
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Refresh blocker evidence before making absence claims.").
		SetNextExecutionStep("Re-read source evidence and record the coverage caveat.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T13:40:00Z|source_coverage:blocker-subject").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:blocker-global").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("blocker_clearance").
		SetEvidenceKind("blocker_clearance").
		SetPriority(workprogramevidenceneed.PriorityCritical).
		SetTargetKind("workstream").
		SetTargetKey(workstream).
		SetExecutionState("actions_open").
		SetBackingActionCount(1).
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Confirm global blocker clearance before publishing unblock claims.").
		SetNextExecutionStep("Review blocker-clearance queue.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T13:40:00Z|blocker_clearance:global").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create global blocker evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:blocker-off-target").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("blocker_clearance").
		SetEvidenceKind("blocker_clearance").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey(offScopeSubjectKey).
		SetExecutionState("action_open").
		SetRecommendedAction("Off-target blocker evidence should not count in scoped packet.").
		SetNextExecutionStep("Do not include unrelated blocker target evidence.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T13:40:00Z|blocker_clearance:off-target").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create off-target blocker evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rowLimit := 1
	evidenceLimit := 1
	sourceArg := source
	packet, err := resolver.WorkProgramBlockerPacket(ctx, "workstream:flink-kubernetes-operator", nil, &rowLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("blocker packet: %v", err)
	}
	if packet.SourceInstance == nil || *packet.SourceInstance != source {
		t.Fatalf("packet source = %#v, want %s", packet.SourceInstance, source)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != generatedAt.Format(time.RFC3339) {
		t.Fatalf("packet generatedAt = %#v, want readiness run timestamp", packet.GeneratedAt)
	}
	if packet.WorkstreamKey != "workstream:flink-kubernetes-operator" || packet.BlockerState != "open" {
		t.Fatalf("packet scope/state = %s/%s, want workstream/open", packet.WorkstreamKey, packet.BlockerState)
	}
	if packet.BlockerCount != 1 || len(packet.Blockers) != 1 || packet.Blockers[0].Key != "work-blocker:scoped" {
		t.Fatalf("packet blockers leaked or missed scoped row: count=%d rows=%#v", packet.BlockerCount, packet.Blockers)
	}
	if packet.ImpactCount != 1 || len(packet.Impacts) != 1 || packet.Impacts[0].AffectedKey != workstream {
		t.Fatalf("packet impacts = count=%d rows=%#v, want scoped workstream impact", packet.ImpactCount, packet.Impacts)
	}
	if packet.HighRiskForecastCount != 2 || len(packet.Forecasts) != 1 || packet.Forecasts[0].SubjectKey != subjectKey {
		t.Fatalf("packet forecasts = count=%d rows=%#v, want scoped high-risk forecast", packet.HighRiskForecastCount, packet.Forecasts)
	}
	if packet.EvidenceNeedCount != 2 || len(packet.EvidenceNeeds) != 1 || packet.EvidenceNeeds[0].TargetKey == nil || *packet.EvidenceNeeds[0].TargetKey != subjectKey {
		t.Fatalf("packet evidence needs = count=%d rows=%#v, want 2 total and 1 capped target-scoped evidence row", packet.EvidenceNeedCount, packet.EvidenceNeeds)
	}
	if !packet.HumanRequired {
		t.Fatalf("packet humanRequired=false, want true")
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Confirm the owner decision") {
		t.Fatalf("packet recommended focus = %#v, want blocker recommendation", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "1 open blocker(s)") || !strings.Contains(packet.AutomationSummary, "2 high-risk forecast(s)") || !strings.Contains(packet.AutomationSummary, "2 evidence need(s)") {
		t.Fatalf("packet automation summary = %q, want blocker and forecast counts", packet.AutomationSummary)
	}
}

func TestWorkProgramBlockerPacketUsesReadinessRunForEvidenceNeeds(t *testing.T) {
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
	oldRunAt := time.Date(2026, 6, 21, 13, 40, 0, 0, time.UTC)
	newRunAt := oldRunAt.Add(time.Hour)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, newRunAt)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:blocker-subject:run-boundary").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Scoped blocker subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item:blocker-subject:run-boundary").
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	_, err = store.Client().WorkBlocker.Create().
		SetKey("work-blocker:run-boundary").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityHigh).
		SetSubjectKind(workblocker.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Owner decision is blocking merge").
		SetRecommendedAction("Confirm the owner decision before publishing unblock claims.").
		SetRankScore(98).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker").
		SetExternalID("work-blocker:run-boundary").
		SetLastActivityAt(newRunAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:blocker-stale-run").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(oldRunAt).
		SetGateKey("blocker_clearance").
		SetEvidenceKind("blocker_clearance").
		SetPriority(workprogramevidenceneed.PriorityCritical).
		SetTargetKind("pull_request").
		SetTargetKey(subjectKey).
		SetExecutionState("action_open").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("This stale blocker evidence should not leak into the new packet.").
		SetNextExecutionStep("Do not include stale evidence need rows.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T13:40:00Z|blocker_clearance:stale").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramBlockerPacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("blocker packet: %v", err)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != newRunAt.Format(time.RFC3339) {
		t.Fatalf("packet generatedAt = %#v, want newer readiness run timestamp", packet.GeneratedAt)
	}
	if packet.BlockerCount != 1 {
		t.Fatalf("blockerCount = %d, want current blocker still present", packet.BlockerCount)
	}
	if packet.EvidenceNeedCount != 0 || len(packet.EvidenceNeeds) != 0 {
		t.Fatalf("packet leaked stale evidence needs = count:%d rows:%#v, want 0", packet.EvidenceNeedCount, packet.EvidenceNeeds)
	}
}
