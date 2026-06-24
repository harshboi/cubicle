package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workownerloadsnapshot"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/ent/workresponsibility"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkProgramOwnerPacketAggregatesOwnerRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 12, 45, 0, 0, time.UTC)
	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	owner := "github:owner-a"
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, generatedAt)
	_, err = store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("owner-load:owner-a").
		SetWorkstreamKey(workstream).
		SetOwnerKey(owner).
		SetGeneratedAt(generatedAt).
		SetLoadStatus(workownerloadsnapshot.LoadStatusOverloaded).
		SetActionCount(2).
		SetCriticalOrHighCount(2).
		SetMaxPriorityScore(96).
		SetRecommendedFocus("Focus owner-a on stale source coverage and validation lead follow-up.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T12:45:00Z:github:owner-a").
		Save(ctx)
	if err != nil {
		t.Fatalf("create owner load: %v", err)
	}
	_, err = store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("owner-load:owner-b").
		SetWorkstreamKey(workstream).
		SetOwnerKey("github:owner-b").
		SetGeneratedAt(generatedAt).
		SetLoadStatus(workownerloadsnapshot.LoadStatusWatch).
		SetActionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T12:45:00Z:github:owner-b").
		Save(ctx)
	if err != nil {
		t.Fatalf("create other owner load: %v", err)
	}

	now := generatedAt.Add(5 * time.Minute)
	ownerActionID := 0
	for _, row := range []struct {
		key       string
		ownerKey  string
		subject   string
		stream    string
		rankScore float64
	}{
		{key: "tpm-action:owner-a-1", ownerKey: owner, subject: "apache/flink-kubernetes-operator#1", stream: workstream, rankScore: 96},
		{key: "tpm-action:owner-a-2", ownerKey: owner, subject: "apache/flink-kubernetes-operator#2", stream: workstream, rankScore: 91},
		{key: "tpm-action:owner-a-other-workstream", ownerKey: owner, subject: "apache/flink-kubernetes-operator#99", stream: "other-workstream", rankScore: 100},
		{key: "tpm-action:owner-b-1", ownerKey: "github:owner-b", subject: "apache/flink-kubernetes-operator#3", stream: workstream, rankScore: 99},
	} {
		action, err := store.Client().WorkAction.Create().
			SetKey(row.key).
			SetActionType(workaction.ActionTypeValidateSignal).
			SetActionState(workaction.ActionStateOpen).
			SetDecisionState(workaction.DecisionStateValidationLead).
			SetSubjectKind(workaction.SubjectKindUnknown).
			SetSubjectKey(row.subject).
			SetOwnerKey(row.ownerKey).
			SetSourceSystem("cubicle_analytics").
			SetSourceInstance(source).
			SetExternalKind("tpm_work_action").
			SetExternalID(row.key).
			SetLastActivityAt(now).
			SetRankScore(row.rankScore).
			Save(ctx)
		if err != nil {
			t.Fatalf("create action %s: %v", row.key, err)
		}
		if row.key == "tpm-action:owner-a-1" {
			ownerActionID = action.ID
		}
		_, err = store.Client().WorkProgramItem.Create().
			SetKey("work-program-item:" + row.key).
			SetWorkAction(action).
			SetWorkstreamKey(row.stream).
			SetSubjectKind(workprogramitem.SubjectKindUnknown).
			SetSubjectKey(row.subject).
			SetTitle("Program item for " + row.key).
			SetProgramStatus(workprogramitem.ProgramStatusValidateSignal).
			SetTpmBucket(workprogramitem.TpmBucketRiskValidation).
			SetDecisionState(workprogramitem.DecisionStateValidationLead).
			SetDueBucket(workprogramitem.DueBucketNow).
			SetOwnerKey(row.ownerKey).
			SetFreshnessState(workprogramitem.FreshnessStateFresh).
			SetSourceCoverageState("observed:authenticated_api_current_observation").
			SetRiskScore(row.rankScore).
			SetSourceSystem("cubicle_analytics").
			SetSourceInstance(source).
			SetExternalKind("tpm_program_item").
			SetExternalID("program-item|" + row.key).
			SetRankScore(row.rankScore).
			Save(ctx)
		if err != nil {
			t.Fatalf("create program item %s: %v", row.key, err)
		}
	}
	if ownerActionID == 0 {
		t.Fatal("owner action ID was not captured")
	}
	pr, err := store.Client().PullRequest.Create().
		SetKey("apache/flink-kubernetes-operator#1").
		SetRepository("apache/flink-kubernetes-operator").
		SetNumber(1).
		SetTitle("Owner-a source-native PR").
		SetSourceURL("https://github.com/apache/flink-kubernetes-operator/pull/1").
		Save(ctx)
	if err != nil {
		t.Fatalf("create pull request: %v", err)
	}
	_, err = store.Client().WorkResponsibility.Create().
		SetKey("responsibility:owner-a:pr-author").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workresponsibility.SubjectKindPullRequest).
		SetSubjectKey(pr.Key).
		SetPullRequestID(pr.ID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey(owner).
		SetPartySource("github.pr.author").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAuthor).
		SetBasisKind(workresponsibility.BasisKindSourceNative).
		SetResponsibilityState(workresponsibility.ResponsibilityStateActive).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID("owner-a-pr-author").
		SetLastActivityAt(now).
		SetRankScore(85).
		Save(ctx)
	if err != nil {
		t.Fatalf("create source-native responsibility: %v", err)
	}
	_, err = store.Client().WorkResponsibility.Create().
		SetKey("responsibility:owner-a:action-accountable").
		SetSubjectKind(workresponsibility.SubjectKindWorkAction).
		SetSubjectKey("tpm-action:owner-a-1").
		SetWorkActionID(ownerActionID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey(owner).
		SetPartySource("generated.action_owner").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAccountable).
		SetBasisKind(workresponsibility.BasisKindGeneratedCandidate).
		SetResponsibilityState(workresponsibility.ResponsibilityStateCandidate).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID("owner-a-action-accountable").
		SetLastActivityAt(now).
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create generated responsibility: %v", err)
	}
	_, err = store.Client().WorkResponsibility.Create().
		SetKey("responsibility:owner-b:pr-author").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workresponsibility.SubjectKindPullRequest).
		SetSubjectKey(pr.Key).
		SetPullRequestID(pr.ID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey("github:owner-b").
		SetPartySource("github.pr.author").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAuthor).
		SetBasisKind(workresponsibility.BasisKindSourceNative).
		SetResponsibilityState(workresponsibility.ResponsibilityStateActive).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID("owner-b-pr-author").
		SetLastActivityAt(now).
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create other-owner responsibility: %v", err)
	}

	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:owner-a").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("required_check_coverage").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey("apache/flink-kubernetes-operator#1").
		SetOwnerKey(owner).
		SetActionKey("tpm-action:owner-a-1").
		SetActionState("open").
		SetExecutionState("stale_source_action_review_needed").
		SetBackingActionCount(2).
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Review stale source coverage before making absence claims.").
		SetNextExecutionStep("Refresh the source bundle or mark the evidence caveat.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T12:45:00Z|source_coverage:owner-a").
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:owner-a-closed").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("stale_source_action").
		SetPriority(workprogramevidenceneed.PriorityMedium).
		SetTargetKind("pull_request").
		SetTargetKey("apache/flink-kubernetes-operator#2").
		SetOwnerKey(owner).
		SetActionKey("tpm-action:owner-a-closed").
		SetActionState("closed").
		SetExecutionState("stale_source_action_review_needed").
		SetBackingActionCount(1).
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Review closed stale source action before treating coverage as complete.").
		SetNextExecutionStep("Keep stale source action evidence visible in owner packets.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T12:45:00Z|source_coverage:owner-a-closed").
		Save(ctx)
	if err != nil {
		t.Fatalf("create closed evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:owner-b").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("owner_load").
		SetEvidenceKind("owner_load_balancing").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey(workstream).
		SetOwnerKey("github:owner-b").
		SetActionKey("tpm-action:owner-b-1").
		SetActionState("open").
		SetExecutionState("owner_load_rows_open").
		SetRecommendedAction("Owner b should not leak into owner a packet.").
		SetNextExecutionStep("Owner b should not leak into owner a packet.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T12:45:00Z|owner_load:owner-b").
		Save(ctx)
	if err != nil {
		t.Fatalf("create other evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	actionLimit := 1
	evidenceLimit := 1
	sourceArg := source
	packet, err := resolver.WorkProgramOwnerPacket(ctx, "workstream:flink-kubernetes-operator", owner, nil, &actionLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("owner packet: %v", err)
	}
	if packet.OwnerKey != owner || packet.WorkstreamKey != "workstream:flink-kubernetes-operator" {
		t.Fatalf("owner packet scoped to wrong owner/workstream: %#v", packet)
	}
	if packet.SourceInstance == nil || *packet.SourceInstance != source {
		t.Fatalf("owner packet source = %#v, want %s", packet.SourceInstance, source)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != generatedAt.Format(time.RFC3339) {
		t.Fatalf("owner packet generatedAt = %#v, want readiness run timestamp", packet.GeneratedAt)
	}
	if packet.LoadStatus != "overloaded" || packet.OwnerLoad == nil {
		t.Fatalf("owner packet load = %s ownerLoad=%#v, want overloaded owner load", packet.LoadStatus, packet.OwnerLoad)
	}
	if packet.ActionCount != 2 {
		t.Fatalf("owner packet action count = %d, want persisted owner-load total 2", packet.ActionCount)
	}
	if len(packet.Actions) != 1 || packet.Actions[0].OwnerKey == nil || *packet.Actions[0].OwnerKey != owner {
		t.Fatalf("owner packet actions not capped/scoped: %#v", packet.Actions)
	}
	if strings.Contains(packet.Actions[0].Key, "other-workstream") {
		t.Fatalf("owner packet action leaked other workstream: %#v", packet.Actions[0])
	}
	if packet.EvidenceNeedCount != 2 || len(packet.EvidenceNeeds) != 1 {
		t.Fatalf("owner packet evidence count=%d rows=%d, want 2 total and 1 capped returned row", packet.EvidenceNeedCount, len(packet.EvidenceNeeds))
	}
	if packet.ResponsibilityCount != 2 || packet.ActiveResponsibilityCount != 1 || packet.CandidateResponsibilityCount != 1 || packet.UnassignedResponsibilityCount != 0 {
		t.Fatalf(
			"owner packet responsibility counts = total:%d active:%d candidate:%d unassigned:%d, want 2/1/1/0",
			packet.ResponsibilityCount,
			packet.ActiveResponsibilityCount,
			packet.CandidateResponsibilityCount,
			packet.UnassignedResponsibilityCount,
		)
	}
	if len(packet.Responsibilities) != 2 {
		t.Fatalf("owner packet responsibilities = %#v, want 2 rows", packet.Responsibilities)
	}
	for _, responsibility := range packet.Responsibilities {
		if responsibility.PartyKey != owner {
			t.Fatalf("owner packet responsibility leaked wrong owner: %#v", responsibility)
		}
	}
	var candidate, active = packet.Responsibilities[0], packet.Responsibilities[0]
	for _, responsibility := range packet.Responsibilities {
		if responsibility.ResponsibilityState == "candidate" {
			candidate = responsibility
		}
		if responsibility.ResponsibilityState == "active" {
			active = responsibility
		}
	}
	if candidate.SubjectKind != "work_action" || candidate.BasisKind != "generated_candidate" || candidate.ResponsibilityState != "candidate" || candidate.Action == nil {
		t.Fatalf("owner packet candidate responsibility = %#v, want generated action candidate", candidate)
	}
	if active.SubjectKind != "pull_request" || active.BasisKind != "source_native" || active.ResponsibilityState != "active" {
		t.Fatalf("owner packet active responsibility = %#v, want active source-native PR responsibility", active)
	}
	for _, need := range packet.EvidenceNeeds {
		if need.OwnerKey == nil || *need.OwnerKey != owner {
			t.Fatalf("owner packet evidence leaked wrong owner: %#v", need)
		}
	}
	if !packet.HumanRequired {
		t.Fatalf("owner packet humanRequired=false, want true")
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Focus owner-a") {
		t.Fatalf("owner packet recommended focus = %#v, want owner-load focus", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "2 open TPM action(s)") || !strings.Contains(packet.AutomationSummary, "2 evidence need(s)") {
		t.Fatalf("owner packet automation summary = %q, want action/evidence counts", packet.AutomationSummary)
	}
}

func TestWorkProgramOwnerPacketTreatsCandidateResponsibilityAsHumanRequired(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	generatedAt := time.Date(2026, 6, 21, 12, 45, 0, 0, time.UTC)
	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	owner := "github:owner-candidate"
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, generatedAt)
	action, err := store.Client().WorkAction.Create().
		SetKey("tpm-action:owner-candidate").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#77").
		SetOwnerKey(owner).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:owner-candidate").
		SetLastActivityAt(generatedAt).
		SetRankScore(75).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	_, err = store.Client().WorkResponsibility.Create().
		SetKey("responsibility:owner-candidate:action-accountable").
		SetSubjectKind(workresponsibility.SubjectKindWorkAction).
		SetSubjectKey(action.Key).
		SetWorkActionID(action.ID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey(owner).
		SetPartySource("generated.action_owner").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAccountable).
		SetBasisKind(workresponsibility.BasisKindGeneratedCandidate).
		SetResponsibilityState(workresponsibility.ResponsibilityStateCandidate).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID("owner-candidate-action-accountable").
		SetLastActivityAt(generatedAt).
		SetRankScore(75).
		Save(ctx)
	if err != nil {
		t.Fatalf("create candidate responsibility: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramOwnerPacket(ctx, "workstream:flink-kubernetes-operator", owner, nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("owner packet: %v", err)
	}
	if packet.ActionCount != 0 || packet.EvidenceNeedCount != 0 {
		t.Fatalf("packet action/evidence counts = %d/%d, want 0/0 without program item or evidence rows", packet.ActionCount, packet.EvidenceNeedCount)
	}
	if packet.ResponsibilityCount != 1 || packet.CandidateResponsibilityCount != 1 || len(packet.Responsibilities) != 1 {
		t.Fatalf("packet responsibilities = count:%d candidate:%d rows:%#v, want one candidate", packet.ResponsibilityCount, packet.CandidateResponsibilityCount, packet.Responsibilities)
	}
	if !packet.HumanRequired {
		t.Fatalf("owner packet humanRequired=false, want candidate responsibility to require validation")
	}
}

func TestWorkProgramOwnerPacketUsesReadinessRunForEvidenceNeeds(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	oldRunAt := time.Date(2026, 6, 21, 12, 45, 0, 0, time.UTC)
	newRunAt := oldRunAt.Add(time.Hour)
	source := "fixture-source"
	workstream := "flink-kubernetes-operator"
	owner := "github:owner-a"
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, newRunAt)

	_, err = store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("owner-load:owner-a:run-boundary").
		SetWorkstreamKey(workstream).
		SetOwnerKey(owner).
		SetGeneratedAt(oldRunAt).
		SetLoadStatus(workownerloadsnapshot.LoadStatusWatch).
		SetActionCount(1).
		SetRecommendedFocus("Owner load remains visible even when stale generated evidence is filtered.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T12:45:00Z:github:owner-a").
		Save(ctx)
	if err != nil {
		t.Fatalf("create owner load: %v", err)
	}
	action, err := store.Client().WorkAction.Create().
		SetKey("tpm-action:owner-a-run-boundary").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#1").
		SetOwnerKey(owner).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:owner-a-run-boundary").
		SetLastActivityAt(newRunAt).
		SetRankScore(96).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:owner-a-run-boundary").
		SetWorkAction(action).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#1").
		SetTitle("Owner scoped work item").
		SetProgramStatus(workprogramitem.ProgramStatusValidateSignal).
		SetTpmBucket(workprogramitem.TpmBucketRiskValidation).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetOwnerKey(owner).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(96).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("program-item|owner-a-run-boundary").
		SetRankScore(96).
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:owner-a-stale-run").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(oldRunAt).
		SetGateKey("owner_load").
		SetEvidenceKind("owner_load_balancing").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey(workstream).
		SetOwnerKey(owner).
		SetActionKey("tpm-action:owner-a-run-boundary").
		SetActionState("open").
		SetExecutionState("owner_load_rows_open").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("This stale owner evidence should not leak into the new packet.").
		SetNextExecutionStep("Do not include stale evidence need rows.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T12:45:00Z|owner_load:owner-a-stale").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramOwnerPacket(ctx, "workstream:flink-kubernetes-operator", owner, nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("owner packet: %v", err)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != newRunAt.Format(time.RFC3339) {
		t.Fatalf("packet generatedAt = %#v, want newer readiness run timestamp", packet.GeneratedAt)
	}
	if packet.OwnerLoad == nil || packet.OwnerLoad.GeneratedAt == nil || *packet.OwnerLoad.GeneratedAt != oldRunAt.Format(time.RFC3339) {
		t.Fatalf("owner load generatedAt = %#v, want owner-load snapshot timestamp", packet.OwnerLoad)
	}
	if packet.ActionCount != 1 || len(packet.Actions) != 1 {
		t.Fatalf("owner packet actions = count:%d rows:%#v, want owner action still present", packet.ActionCount, packet.Actions)
	}
	if packet.EvidenceNeedCount != 0 || len(packet.EvidenceNeeds) != 0 {
		t.Fatalf("packet leaked stale evidence needs = count:%d rows:%#v, want 0", packet.EvidenceNeedCount, packet.EvidenceNeeds)
	}
}
