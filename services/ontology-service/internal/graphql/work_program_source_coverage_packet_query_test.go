package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkProgramSourceCoveragePacketBlocksAbsenceClaimsForPartialCoverage(t *testing.T) {
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
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, generatedAt)

	connection, err := store.Client().SourceConnection.Create().
		SetKey("source-connection:fixture").
		SetSourceSystem("github").
		SetSourceInstance(source).
		SetDisplayName("GitHub fixture").
		Save(ctx)
	if err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	scope, err := store.Client().SourceScope.Create().
		SetKey("source-scope:fixture").
		SetConnection(connection).
		SetScopeKind("workstream").
		SetScopeKey(source).
		Save(ctx)
	if err != nil {
		t.Fatalf("create source scope: %v", err)
	}
	olderRun, err := store.Client().SourceSyncRun.Create().
		SetScope(scope).
		SetRunKey("fixture-run:older").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModePartialScope).
		SetStatus(sourcesyncrun.StatusComplete).
		SetStartedAt(generatedAt.Add(-time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create older source sync run: %v", err)
	}
	latestRun, err := store.Client().SourceSyncRun.Create().
		SetScope(scope).
		SetRunKey("fixture-run:latest").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModePartialScope).
		SetStatus(sourcesyncrun.StatusComplete).
		SetStartedAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create latest source sync run: %v", err)
	}
	_, err = store.Client().SourceSyncIssue.Create().
		SetScope(scope).
		SetSyncRun(latestRun).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_forbidden").
		SetMessage("source snapshot returned status 403; body retained for replay coverage only").
		SetSourceSystem("github").
		SetSourceInstance("github.com/apache/flink-kubernetes-operator").
		SetExternalKind("github_pr").
		SetExternalID("apache/flink-kubernetes-operator#72").
		SetSourceURL("https://github.com/apache/flink-kubernetes-operator/pull/72").
		Save(ctx)
	if err != nil {
		t.Fatalf("create source sync issue: %v", err)
	}
	_, err = store.Client().SourceSyncIssue.Create().
		SetScope(scope).
		SetSyncRun(olderRun).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_rate_limited").
		SetMessage("older run should not affect latest source coverage packet").
		SetSourceSystem("github").
		SetSourceInstance("github.com/apache/flink-kubernetes-operator").
		SetExternalKind("github_pr").
		SetExternalID("apache/flink-kubernetes-operator#old").
		Save(ctx)
	if err != nil {
		t.Fatalf("create older source sync issue: %v", err)
	}
	otherScope, err := store.Client().SourceScope.Create().
		SetKey("source-scope:other-workstream").
		SetConnection(connection).
		SetScopeKind("workstream").
		SetScopeKey("other-workstream").
		Save(ctx)
	if err != nil {
		t.Fatalf("create other source scope: %v", err)
	}
	otherRun, err := store.Client().SourceSyncRun.Create().
		SetScope(otherScope).
		SetRunKey("fixture-run:other").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModePartialScope).
		SetStatus(sourcesyncrun.StatusComplete).
		SetStartedAt(generatedAt.Add(time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create other source sync run: %v", err)
	}
	_, err = store.Client().SourceSyncIssue.Create().
		SetScope(otherScope).
		SetSyncRun(otherRun).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_forbidden").
		SetMessage("other workstream issue should not affect this source coverage packet").
		SetSourceSystem("github").
		SetSourceInstance("github.com/apache/flink-kubernetes-operator").
		SetExternalKind("github_pr").
		SetExternalID("apache/flink-kubernetes-operator#other").
		Save(ctx)
	if err != nil {
		t.Fatalf("create other source sync issue: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:partial").
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
		SetNextAction("Hydrate authenticated PR details before claiming missing reviews.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|partial").
		SetRankScore(100).
		SetRiskScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create partial program item: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:generated").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#74").
		SetTitle("Generated-only forecast risk lead").
		SetProgramStatus(workprogramitem.ProgramStatusValidateSignal).
		SetTpmBucket(workprogramitem.TpmBucketRiskValidation).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("generated:forecast_risk_backstop").
		SetNextAction("Validate generated forecast risk before making source-backed claims.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|generated").
		SetRankScore(90).
		SetRiskScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create generated program item: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:complete").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#73").
		SetTitle("Complete PR").
		SetProgramStatus(workprogramitem.ProgramStatusWaitingReview).
		SetTpmBucket(workprogramitem.TpmBucketReview).
		SetDecisionState(workprogramitem.DecisionStatePendingReview).
		SetDueBucket(workprogramitem.DueBucketThisWeek).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|complete").
		SetRankScore(40).
		SetRiskScore(40).
		Save(ctx)
	if err != nil {
		t.Fatalf("create complete program item: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:source-coverage").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("source_authentication").
		SetEvidenceKind("source_authentication").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey("workstream:flink-kubernetes-operator").
		SetExecutionState("review_actions_open").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Authenticate GitHub replay before allowing absence claims.").
		SetNextExecutionStep("Refresh PR details with authenticated GitHub access.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|source_coverage:auth").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:source-coverage:coverage-row").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("required_check_coverage").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("pull_request").
		SetTargetKey("github:apache/flink-kubernetes-operator#72").
		SetExecutionState("review_actions_open").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Refresh PR 72 with authenticated source coverage before absence claims.").
		SetNextExecutionStep("Re-fetch PR 72 after source authentication is repaired.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T07:03:16Z|source_coverage:pr72").
		SetRankScore(80).
		Save(ctx)
	if err != nil {
		t.Fatalf("create second evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rowLimit := 10
	evidenceLimit := 1
	sourceArg := source
	packet, err := resolver.WorkProgramSourceCoveragePacket(ctx, "workstream:flink-kubernetes-operator", &rowLimit, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("source coverage packet: %v", err)
	}

	if packet.SourceInstance == nil || *packet.SourceInstance != source {
		t.Fatalf("packet source = %#v, want %s", packet.SourceInstance, source)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != generatedAt.Format(time.RFC3339) {
		t.Fatalf("packet generatedAt = %#v, want readiness run timestamp", packet.GeneratedAt)
	}
	if packet.CoverageState != "limited" || packet.AbsenceClaimsAllowed || !packet.HumanReviewRequired {
		t.Fatalf("packet state = %s absence:%v human:%v, want limited/false/true", packet.CoverageState, packet.AbsenceClaimsAllowed, packet.HumanReviewRequired)
	}
	if packet.AbsenceClaimGateReason != "source_auth_or_rate_limited" {
		t.Fatalf("absenceClaimGateReason = %q, want source_auth_or_rate_limited", packet.AbsenceClaimGateReason)
	}
	if packet.CompleteItemCount != 2 || packet.LimitedItemCount != 1 || packet.UnknownItemCount != 0 {
		t.Fatalf("item counts = complete:%d limited:%d unknown:%d, want 2/1/0", packet.CompleteItemCount, packet.LimitedItemCount, packet.UnknownItemCount)
	}
	if packet.SourceSyncIssueCount != 1 || packet.AuthOrRateLimitIssueCount != 1 {
		t.Fatalf("sync issue counts = total:%d auth:%d, want 1/1", packet.SourceSyncIssueCount, packet.AuthOrRateLimitIssueCount)
	}
	if len(packet.AffectedItems) != 1 {
		t.Fatalf("affected items = %#v, want only true partial source coverage item", packet.AffectedItems)
	}
	affectedKeys := map[string]bool{}
	for _, item := range packet.AffectedItems {
		affectedKeys[item.Key] = true
	}
	if !affectedKeys["work-program-item:partial"] || affectedKeys["work-program-item:generated"] {
		t.Fatalf("affected items = %#v, want partial item and not generated item", packet.AffectedItems)
	}
	if len(packet.SourceSyncIssues) != 1 || packet.SourceSyncIssues[0].IssueCode != "source_forbidden" {
		t.Fatalf("source issues = %#v, want source_forbidden", packet.SourceSyncIssues)
	}
	if packet.SourceSyncIssues[0].Message == nil || strings.Contains(*packet.SourceSyncIssues[0].Message, "403") || !strings.Contains(*packet.SourceSyncIssues[0].Message, "source_forbidden") {
		t.Fatalf("source issue message = %#v, want sanitized issue-code summary", packet.SourceSyncIssues[0].Message)
	}
	if packet.SourceSyncIssues[0].SourceURL != nil {
		t.Fatalf("source issue URL = %#v, want source URL redacted from coverage packet", packet.SourceSyncIssues[0].SourceURL)
	}
	if len(packet.EvidenceNeeds) != 1 || packet.EvidenceNeeds[0].GateKey != "source_authentication" {
		t.Fatalf("evidence needs = %#v, want source_authentication need", packet.EvidenceNeeds)
	}
	if packet.EvidenceNeedCount != 2 {
		t.Fatalf("evidenceNeedCount = %d, want uncapped count 2", packet.EvidenceNeedCount)
	}
	if packet.RecommendedFocus == nil || !strings.Contains(*packet.RecommendedFocus, "Authenticate GitHub replay") {
		t.Fatalf("recommended focus = %#v, want evidence need focus", packet.RecommendedFocus)
	}
	if !strings.Contains(packet.AutomationSummary, "absence claims disabled") || !strings.Contains(packet.AutomationSummary, "1 auth/rate-limit issue") || !strings.Contains(packet.AutomationSummary, "2 source-coverage evidence need(s)") {
		t.Fatalf("automation summary = %q, want coverage guardrail language", packet.AutomationSummary)
	}
	if !strings.Contains(packet.AutomationSummary, "absence claim gate reason: source_auth_or_rate_limited") {
		t.Fatalf("automation summary = %q, want absence claim gate reason", packet.AutomationSummary)
	}
}

func TestWorkProgramSourceCoveragePacketBlocksAbsenceClaimsForEvidenceNeeds(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 21, 8, 14, 0, 0, time.UTC)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:complete").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#73").
		SetTitle("Complete PR").
		SetProgramStatus(workprogramitem.ProgramStatusWaitingReview).
		SetTpmBucket(workprogramitem.TpmBucketReview).
		SetDecisionState(workprogramitem.DecisionStatePendingReview).
		SetDueBucket(workprogramitem.DueBucketThisWeek).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|complete").
		SetRankScore(40).
		SetRiskScore(40).
		Save(ctx)
	if err != nil {
		t.Fatalf("create complete program item: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:source-coverage").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("source_authentication").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey("workstream:flink-kubernetes-operator").
		SetExecutionState("review_actions_open").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("Authenticate GitHub replay before allowing absence claims.").
		SetNextExecutionStep("Refresh PR details with authenticated GitHub access.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T08:14:00Z|source_authentication:auth").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramSourceCoveragePacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("source coverage packet: %v", err)
	}

	if packet.CoverageState != "complete" {
		t.Fatalf("coverageState = %q, want complete", packet.CoverageState)
	}
	if packet.AbsenceClaimsAllowed || !packet.HumanReviewRequired {
		t.Fatalf("absence/human = %v/%v, want false/true", packet.AbsenceClaimsAllowed, packet.HumanReviewRequired)
	}
	if packet.AbsenceClaimGateReason != "source_coverage_evidence_needed" {
		t.Fatalf("absenceClaimGateReason = %q, want source_coverage_evidence_needed", packet.AbsenceClaimGateReason)
	}
	if !strings.Contains(packet.AutomationSummary, "absence claim gate reason: source_coverage_evidence_needed") {
		t.Fatalf("automation summary = %q, want evidence-needed gate reason", packet.AutomationSummary)
	}
}

func TestWorkProgramSourceCoveragePacketBlocksAbsenceClaimsForWatchOnlyAuthAndProvenanceNeeds(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 21, 8, 45, 0, 0, time.UTC)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, generatedAt)

	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:complete-with-watch-needs").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#73").
		SetTitle("Complete PR with validation-only guardrail work").
		SetProgramStatus(workprogramitem.ProgramStatusWaitingReview).
		SetTpmBucket(workprogramitem.TpmBucketReview).
		SetDecisionState(workprogramitem.DecisionStatePendingReview).
		SetDueBucket(workprogramitem.DueBucketThisWeek).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|complete-with-watch-needs").
		SetRankScore(40).
		SetRiskScore(40).
		Save(ctx)
	if err != nil {
		t.Fatalf("create complete program item: %v", err)
	}
	for _, gate := range []struct {
		key    string
		detail string
	}{
		{"source_authentication", "Validation-only rows have anonymous observations."},
		{"claim_provenance", "Validation-only rows depend on generated evidence."},
	} {
		_, err = store.Client().WorkProgramQualityGate.Create().
			SetKey("work-program-quality-gate:" + gate.key + ":watch").
			SetWorkstreamKey(workstream).
			SetGeneratedAt(generatedAt).
			SetGateKey(gate.key).
			SetGateState("watch").
			SetBlocking(false).
			SetDetail(gate.detail).
			SetRecommendedAction("Keep validation-only evidence visible while broad absence claims stay gated.").
			SetSourceSystem("cubicle_analytics").
			SetSourceInstance(source).
			SetExternalKind("tpm_work_program_quality_gate").
			SetExternalID("flink-kubernetes-operator|2026-06-21T08:45:00Z|" + gate.key).
			SetRankScore(40).
			Save(ctx)
		if err != nil {
			t.Fatalf("create %s quality gate: %v", gate.key, err)
		}
	}
	for _, need := range []struct {
		key          string
		gateKey      string
		evidenceKind string
		metricKey    string
	}{
		{"source-authentication-watch", "source_authentication", "source_authentication", "anonymous_observation"},
		{"claim-provenance-watch", "claim_provenance", "generated_evidence", "generated_evidence"},
	} {
		_, err = store.Client().WorkProgramEvidenceNeed.Create().
			SetKey("work-program-evidence-need:" + need.key).
			SetWorkstreamKey(workstream).
			SetGeneratedAt(generatedAt).
			SetGateKey(need.gateKey).
			SetEvidenceKind(need.evidenceKind).
			SetPriority(workprogramevidenceneed.PriorityMedium).
			SetTargetKind("workstream").
			SetTargetKey("workstream:flink-kubernetes-operator").
			SetMetricKey(need.metricKey).
			SetExecutionState("review_actions_open").
			SetCurrentCount(0).
			SetRequiredCount(1).
			SetMissingCount(1).
			SetRecommendedAction("Keep this validation-only evidence need visible.").
			SetNextExecutionStep("Review validation-only guardrail work before enabling broad absence claims.").
			SetSourceSystem("cubicle_analytics").
			SetSourceInstance(source).
			SetExternalKind("tpm_work_program_evidence_need").
			SetExternalID("flink-kubernetes-operator|2026-06-21T08:45:00Z|" + need.key).
			SetRankScore(40).
			Save(ctx)
		if err != nil {
			t.Fatalf("create %s evidence need: %v", need.key, err)
		}
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramSourceCoveragePacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("source coverage packet: %v", err)
	}

	if packet.CoverageState != "complete" {
		t.Fatalf("coverageState = %q, want complete", packet.CoverageState)
	}
	if packet.AbsenceClaimsAllowed || !packet.HumanReviewRequired {
		t.Fatalf("absence/human = %v/%v, want false/true", packet.AbsenceClaimsAllowed, packet.HumanReviewRequired)
	}
	if packet.AbsenceClaimGateReason != "source_coverage_evidence_needed" {
		t.Fatalf("absenceClaimGateReason = %q, want source_coverage_evidence_needed", packet.AbsenceClaimGateReason)
	}
	if packet.EvidenceNeedCount != 2 || len(packet.EvidenceNeeds) != 2 {
		t.Fatalf("evidence needs = count:%d rows:%#v, want 2 visible watch needs", packet.EvidenceNeedCount, packet.EvidenceNeeds)
	}
	if !strings.Contains(packet.AutomationSummary, "absence claims disabled") || !strings.Contains(packet.AutomationSummary, "source_coverage_evidence_needed") {
		t.Fatalf("automation summary = %q, want disabled absence claims with evidence-needed gate", packet.AutomationSummary)
	}
}

func TestWorkProgramSourceCoveragePacketAllowsAbsenceClaimsForCompleteCoverage(t *testing.T) {
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
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:complete").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#73").
		SetTitle("Complete PR").
		SetProgramStatus(workprogramitem.ProgramStatusWaitingReview).
		SetTpmBucket(workprogramitem.TpmBucketReview).
		SetDecisionState(workprogramitem.DecisionStatePendingReview).
		SetDueBucket(workprogramitem.DueBucketThisWeek).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|complete").
		SetRankScore(40).
		SetRiskScore(40).
		Save(ctx)
	if err != nil {
		t.Fatalf("create complete program item: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramSourceCoveragePacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("source coverage packet: %v", err)
	}

	if packet.CoverageState != "complete" || !packet.AbsenceClaimsAllowed || packet.HumanReviewRequired {
		t.Fatalf("state/absence/human = %q/%v/%v, want complete/true/false", packet.CoverageState, packet.AbsenceClaimsAllowed, packet.HumanReviewRequired)
	}
	if packet.AbsenceClaimGateReason != "complete_source_coverage" {
		t.Fatalf("absenceClaimGateReason = %q, want complete_source_coverage", packet.AbsenceClaimGateReason)
	}
}

func TestWorkProgramSourceCoveragePacketDoesNotLetLiveOnlyRunClearSnapshotFailures(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)

	connection, err := store.Client().SourceConnection.Create().
		SetKey("source-connection:fixture").
		SetSourceSystem("github").
		SetSourceInstance(source).
		SetDisplayName("GitHub fixture").
		Save(ctx)
	if err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	baseScope, err := store.Client().SourceScope.Create().
		SetKey("source-scope:base").
		SetConnection(connection).
		SetScopeKind("workstream").
		SetScopeKey(source).
		Save(ctx)
	if err != nil {
		t.Fatalf("create base source scope: %v", err)
	}
	baseRun, err := store.Client().SourceSyncRun.Create().
		SetScope(baseScope).
		SetRunKey("fixture-run:base-partial").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModePartialScope).
		SetStatus(sourcesyncrun.StatusPartial).
		SetStartedAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create base source sync run: %v", err)
	}
	_, err = store.Client().SourceSyncIssue.Create().
		SetScope(baseScope).
		SetSyncRun(baseRun).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_missing_snapshot").
		SetMessage("PR review snapshot missing; retained as coverage evidence only").
		SetSourceSystem("github").
		SetSourceInstance("github.com/apache/flink-kubernetes-operator").
		SetExternalKind("github_pr_reviews").
		SetExternalID("apache/flink-kubernetes-operator#73").
		Save(ctx)
	if err != nil {
		t.Fatalf("create base source sync issue: %v", err)
	}
	liveScope, err := store.Client().SourceScope.Create().
		SetKey("source-scope:live-checks").
		SetConnection(connection).
		SetScopeKind("pull_request_checks").
		SetScopeKey(source).
		Save(ctx)
	if err != nil {
		t.Fatalf("create live source scope: %v", err)
	}
	_, err = store.Client().SourceSyncRun.Create().
		SetScope(liveScope).
		SetRunKey("fixture-run:live-clean").
		SetSyncMode(sourcesyncrun.SyncModeFederatedLive).
		SetCoverageMode(sourcesyncrun.CoverageModeLiveOnly).
		SetStatus(sourcesyncrun.StatusComplete).
		SetStartedAt(generatedAt.Add(time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create live source sync run: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:complete").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#73").
		SetTitle("Complete-looking PR").
		SetProgramStatus(workprogramitem.ProgramStatusWaitingReview).
		SetTpmBucket(workprogramitem.TpmBucketReview).
		SetDecisionState(workprogramitem.DecisionStatePendingReview).
		SetDueBucket(workprogramitem.DueBucketThisWeek).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|complete-looking").
		SetRankScore(40).
		SetRiskScore(40).
		Save(ctx)
	if err != nil {
		t.Fatalf("create complete-looking program item: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramSourceCoveragePacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("source coverage packet: %v", err)
	}

	if packet.CoverageState != "limited" {
		t.Fatalf("coverageState = %q, want limited", packet.CoverageState)
	}
	if packet.SourceSyncIssueCount != 1 {
		t.Fatalf("sourceSyncIssueCount = %d, want base snapshot issue count 1", packet.SourceSyncIssueCount)
	}
	if packet.AbsenceClaimsAllowed {
		t.Fatalf("absenceClaimsAllowed = true, want false because live_only run cannot clear snapshot failures")
	}
	if packet.AbsenceClaimGateReason != "source_sync_issues_present" {
		t.Fatalf("absenceClaimGateReason = %q, want source_sync_issues_present", packet.AbsenceClaimGateReason)
	}
	if packet.RecommendedFocus == nil || strings.Contains(*packet.RecommendedFocus, "PR review snapshot missing") || !strings.Contains(*packet.RecommendedFocus, "source_missing_snapshot") {
		t.Fatalf("recommended focus = %#v, want sanitized source issue focus", packet.RecommendedFocus)
	}
}

func TestWorkProgramSourceCoveragePacketUsesReadinessRunForEvidenceNeedsOnly(t *testing.T) {
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
	oldRunAt := time.Date(2026, 6, 21, 9, 0, 0, 0, time.UTC)
	newRunAt := oldRunAt.Add(time.Hour)
	seedWorkProgramAutomationReadinessRun(t, ctx, store.Client(), source, workstream, newRunAt)

	connection, err := store.Client().SourceConnection.Create().
		SetKey("source-connection:run-boundary").
		SetSourceSystem("github").
		SetSourceInstance(source).
		SetDisplayName("GitHub fixture").
		Save(ctx)
	if err != nil {
		t.Fatalf("create source connection: %v", err)
	}
	scope, err := store.Client().SourceScope.Create().
		SetKey("source-scope:run-boundary").
		SetConnection(connection).
		SetScopeKind("workstream").
		SetScopeKey(source).
		Save(ctx)
	if err != nil {
		t.Fatalf("create source scope: %v", err)
	}
	latestRun, err := store.Client().SourceSyncRun.Create().
		SetScope(scope).
		SetRunKey("fixture-run:source-coverage:latest").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModePartialScope).
		SetStatus(sourcesyncrun.StatusPartial).
		SetStartedAt(newRunAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create latest source sync run: %v", err)
	}
	_, err = store.Client().SourceSyncIssue.Create().
		SetScope(scope).
		SetSyncRun(latestRun).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_forbidden").
		SetMessage("source snapshot returned status 403; body retained for replay coverage only").
		SetSourceSystem("github").
		SetSourceInstance("github.com/apache/flink-kubernetes-operator").
		SetExternalKind("github_pr").
		SetExternalID("apache/flink-kubernetes-operator#72").
		Save(ctx)
	if err != nil {
		t.Fatalf("create source sync issue: %v", err)
	}
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:complete:run-boundary").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#73").
		SetTitle("Complete-looking PR").
		SetProgramStatus(workprogramitem.ProgramStatusWaitingReview).
		SetTpmBucket(workprogramitem.TpmBucketReview).
		SetDecisionState(workprogramitem.DecisionStatePendingReview).
		SetDueBucket(workprogramitem.DueBucketThisWeek).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("fixture|complete-looking:run-boundary").
		SetRankScore(40).
		SetRiskScore(40).
		Save(ctx)
	if err != nil {
		t.Fatalf("create complete-looking program item: %v", err)
	}
	_, err = store.Client().WorkProgramEvidenceNeed.Create().
		SetKey("work-program-evidence-need:source-coverage-stale-run").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(oldRunAt).
		SetGateKey("source_coverage").
		SetEvidenceKind("source_authentication").
		SetPriority(workprogramevidenceneed.PriorityHigh).
		SetTargetKind("workstream").
		SetTargetKey("workstream:flink-kubernetes-operator").
		SetExecutionState("review_actions_open").
		SetCurrentCount(0).
		SetRequiredCount(1).
		SetMissingCount(1).
		SetRecommendedAction("This stale source-coverage need should not leak into the new packet.").
		SetNextExecutionStep("Do not include stale evidence need rows.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_evidence_need").
		SetExternalID("flink-kubernetes-operator|2026-06-21T09:00:00Z|source_coverage:stale").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create stale evidence need: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	packet, err := resolver.WorkProgramSourceCoveragePacket(ctx, "workstream:flink-kubernetes-operator", nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("source coverage packet: %v", err)
	}
	if packet.GeneratedAt == nil || *packet.GeneratedAt != newRunAt.Format(time.RFC3339) {
		t.Fatalf("packet generatedAt = %#v, want newer readiness run timestamp", packet.GeneratedAt)
	}
	if packet.SourceSyncIssueCount != 1 || packet.AuthOrRateLimitIssueCount != 1 {
		t.Fatalf("source sync issue counts = total:%d auth:%d, want live latest issue preserved", packet.SourceSyncIssueCount, packet.AuthOrRateLimitIssueCount)
	}
	if packet.EvidenceNeedCount != 0 || len(packet.EvidenceNeeds) != 0 {
		t.Fatalf("packet leaked stale evidence needs = count:%d rows:%#v, want 0", packet.EvidenceNeedCount, packet.EvidenceNeeds)
	}
	if packet.AbsenceClaimGateReason != "source_auth_or_rate_limited" {
		t.Fatalf("absenceClaimGateReason = %q, want live source issue to keep gate closed", packet.AbsenceClaimGateReason)
	}
}
