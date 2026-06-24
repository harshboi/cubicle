package graphql

import (
	"context"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workforecastevaluation"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/ent/workresponsibility"
)

func seedWorkProgramAutomationReadinessRun(t *testing.T, ctx context.Context, client *genent.Client, source string, workstream string, generatedAt time.Time) {
	t.Helper()

	runKey := generatedAt.UTC().Format("20060102T150405Z")
	_, err := client.WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:" + workstream + ":" + runKey).
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("ready").
		SetReadinessScore(90).
		SetAutonomousActionReady(true).
		SetHumanReviewRequired(false).
		SetSafeAutomationAreas("operating_brief").
		SetHumanRequiredAreas("").
		SetRationale("Seeded materialization run for packet-boundary tests.").
		SetRequiredEvidence("").
		SetBlockingGateKeys("").
		SetQualityGateCount(0).
		SetBlockingGateCount(0).
		SetEvidenceNeedCount(0).
		SetTpmFunctionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID(workstream + "|" + generatedAt.UTC().Format(time.RFC3339) + "|automation_readiness").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create automation readiness run: %v", err)
	}
}

func seedWorkProgramRunMember(t *testing.T, ctx context.Context, client *genent.Client, source string, workstream string, generatedAt time.Time, memberTable string, memberID int, memberKey string, memberExternalKind string, memberExternalID string, memberRankScore float64) {
	t.Helper()

	runKey := "work-program-run:" + workstream + ":" + generatedAt.UTC().Format("20060102T150405Z") + ":" + memberTable
	run, err := client.WorkProgramRun.Create().
		SetKey(runKey).
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("blocked").
		SetReadinessScore(25).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetMemberCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalID(workstream + "|" + generatedAt.UTC().Format(time.RFC3339) + "|run").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create work program run: %v", err)
	}
	_, err = client.WorkProgramRunMember.Create().
		SetWorkProgramRunID(run.ID).
		SetRunKey(run.Key).
		SetMemberTable(memberTable).
		SetMemberID(memberID).
		SetMemberKey(memberKey).
		SetMemberExternalKind(memberExternalKind).
		SetMemberExternalID(memberExternalID).
		SetMemberRankScore(memberRankScore).
		SetCreatedAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create work program run member: %v", err)
	}
}

func seedWorkProgramAutonomousReadyFixture(t *testing.T, ctx context.Context, client *genent.Client, source string, workstream string, generatedAt time.Time) {
	t.Helper()

	runKey := generatedAt.UTC().Format("20060102T150405Z")
	_, err := client.WorkProgramAutomationReadiness.Create().
		SetKey("work-program-automation-readiness:" + workstream + ":" + runKey + ":autonomous-ready").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("ready").
		SetReadinessScore(95).
		SetAutonomousActionReady(true).
		SetHumanReviewRequired(false).
		SetSafeAutomationAreas("operating_brief").
		SetHumanRequiredAreas("").
		SetRationale("Fixture guardrails otherwise allow autonomous TPM action.").
		SetRequiredEvidence("").
		SetBlockingGateKeys("").
		SetQualityGateCount(0).
		SetBlockingGateCount(0).
		SetEvidenceNeedCount(0).
		SetTpmFunctionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_automation_readiness").
		SetExternalID(workstream + "|" + generatedAt.UTC().Format(time.RFC3339) + "|automation_readiness").
		SetRankScore(95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create automation readiness: %v", err)
	}
	_, err = client.WorkProgramTPMFunctionReadiness.Create().
		SetKey("work-program-tpm-function-readiness:" + workstream + ":" + runKey + ":operating-brief").
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
		SetExternalID(workstream + "|" + generatedAt.UTC().Format(time.RFC3339) + "|operating_brief").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create function readiness: %v", err)
	}
	_, err = client.WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:" + workstream + ":" + runKey + ":summary-ready").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("source_event_as_of_gradient_boosting").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("source_event_as_of_gradient_boosting").
		SetBaselineSampleCount(50).
		SetOpenCandidateCount(10).
		SetReadyForEta(true).
		SetReadinessState(workforecastevaluation.ReadinessStateReady).
		SetReadinessReason("Forecast backtest gates cleared for fixture autonomy.").
		SetEvaluatedAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID(workstream + "|" + generatedAt.UTC().Format(time.RFC3339) + "|forecast_summary").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create forecast readiness: %v", err)
	}
	snapshot, err := client.WorkInsightEvaluationSnapshot.Create().
		SetKey("work-insight-evaluation-snapshot:" + workstream + ":" + runKey + ":ready").
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
		SetExternalID(source + "|" + generatedAt.UTC().Format(time.RFC3339) + "|insight_evaluation").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight evaluation snapshot: %v", err)
	}
	_, err = client.WorkInsightKindEvaluationSnapshot.Create().
		SetKey("work-insight-kind-evaluation-snapshot:" + workstream + ":" + runKey + ":status-summary").
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
		SetExternalID(source + "|" + generatedAt.UTC().Format(time.RFC3339) + "|insight_kind|status_summary").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight kind evaluation snapshot: %v", err)
	}
	_, err = client.WorkProgramItem.Create().
		SetKey("work-program-item:" + workstream + ":" + runKey + ":complete").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#ready").
		SetTitle("Coverage-complete work item").
		SetProgramStatus(workprogramitem.ProgramStatusUnknown).
		SetTpmBucket(workprogramitem.TpmBucketUnknown).
		SetDecisionState(workprogramitem.DecisionStatePendingReview).
		SetDueBucket(workprogramitem.DueBucketWatch).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID(workstream + "|" + generatedAt.UTC().Format(time.RFC3339) + "|coverage_complete").
		SetRankScore(10).
		SetRiskScore(10).
		Save(ctx)
	if err != nil {
		t.Fatalf("create complete program item: %v", err)
	}
}

func seedCandidateWorkActionResponsibility(t *testing.T, ctx context.Context, client *genent.Client, source string, workstream string, generatedAt time.Time) (*genent.WorkAction, *genent.WorkResponsibility) {
	t.Helper()

	action, err := client.WorkAction.Create().
		SetKey("work-action:" + workstream + ":candidate-owner").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#candidate-owner").
		SetDecision("owner_followup").
		SetDecisionReason("candidate owner must be validated").
		SetDueBucket(workaction.DueBucketNow).
		SetOwnerKey("github:candidate-owner").
		SetOwnerSource("generated.action_owner").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID(workstream + "|candidate-owner-action").
		SetRankScore(60).
		Save(ctx)
	if err != nil {
		t.Fatalf("create candidate responsibility action: %v", err)
	}
	responsibility, err := client.WorkResponsibility.Create().
		SetKey("work-responsibility:" + workstream + ":candidate-owner").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workresponsibility.SubjectKindWorkAction).
		SetSubjectKey(action.Key).
		SetWorkActionID(action.ID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey("github:candidate-owner").
		SetPartySource("generated.action_owner").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAccountable).
		SetBasisKind(workresponsibility.BasisKindGeneratedCandidate).
		SetResponsibilityState(workresponsibility.ResponsibilityStateCandidate).
		SetResponsibilityStateReason("generated owner hint requires validation before product accountability").
		SetGeneratedAt(generatedAt).
		SetLastActivityAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID(workstream + "|candidate-owner-responsibility").
		SetRankScore(70).
		Save(ctx)
	if err != nil {
		t.Fatalf("create candidate responsibility: %v", err)
	}
	return action, responsibility
}

func seedUnassignedWorkActionResponsibility(t *testing.T, ctx context.Context, client *genent.Client, source string, workstream string, generatedAt time.Time) (*genent.WorkAction, *genent.WorkResponsibility) {
	t.Helper()

	action, err := client.WorkAction.Create().
		SetKey("work-action:" + workstream + ":unassigned-owner").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#unassigned-owner").
		SetDecision("owner_followup").
		SetDecisionReason("no accountable owner was resolved").
		SetDueBucket(workaction.DueBucketNow).
		SetOwnerKey("unassigned").
		SetOwnerSource("unassigned").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID(workstream + "|unassigned-owner-action").
		SetRankScore(60).
		Save(ctx)
	if err != nil {
		t.Fatalf("create unassigned responsibility action: %v", err)
	}
	responsibility, err := client.WorkResponsibility.Create().
		SetKey("work-responsibility:" + workstream + ":unassigned-owner").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workresponsibility.SubjectKindWorkAction).
		SetSubjectKey(action.Key).
		SetWorkActionID(action.ID).
		SetPartyKind(workresponsibility.PartyKindUnassigned).
		SetPartyKey("unassigned").
		SetPartySource("unassigned").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAccountable).
		SetBasisKind(workresponsibility.BasisKindGeneratedCandidate).
		SetResponsibilityState(workresponsibility.ResponsibilityStateCandidate).
		SetResponsibilityStateReason("no accountable party was resolved").
		SetGeneratedAt(generatedAt).
		SetLastActivityAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID(workstream + "|unassigned-owner-responsibility").
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create unassigned responsibility: %v", err)
	}
	return action, responsibility
}

func seedActiveSourceNativeWorkActionResponsibility(t *testing.T, ctx context.Context, client *genent.Client, source string, workstream string, generatedAt time.Time) (*genent.WorkAction, *genent.WorkResponsibility) {
	t.Helper()

	action, err := client.WorkAction.Create().
		SetKey("work-action:" + workstream + ":source-native-owner").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#source-native-owner").
		SetDecision("owner_followup").
		SetDecisionReason("source-native owner is already authoritative").
		SetDueBucket(workaction.DueBucketNow).
		SetOwnerKey("github:source-native-owner").
		SetOwnerSource("github.pr.author").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID(workstream + "|source-native-owner-action").
		SetRankScore(60).
		Save(ctx)
	if err != nil {
		t.Fatalf("create source-native responsibility action: %v", err)
	}
	responsibility, err := client.WorkResponsibility.Create().
		SetKey("work-responsibility:" + workstream + ":source-native-owner").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workresponsibility.SubjectKindWorkAction).
		SetSubjectKey(action.Key).
		SetWorkActionID(action.ID).
		SetPartyKind(workresponsibility.PartyKindUnresolved).
		SetPartyKey("github:source-native-owner").
		SetPartySource("github.pr.author").
		SetResponsibilityKind(workresponsibility.ResponsibilityKindAuthor).
		SetBasisKind(workresponsibility.BasisKindSourceNative).
		SetResponsibilityState(workresponsibility.ResponsibilityStateActive).
		SetResponsibilityStateReason("source-native field supports active responsibility").
		SetGeneratedAt(generatedAt).
		SetLastActivityAt(generatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_responsibility").
		SetExternalID(workstream + "|source-native-owner-responsibility").
		SetRankScore(500).
		Save(ctx)
	if err != nil {
		t.Fatalf("create source-native responsibility: %v", err)
	}
	return action, responsibility
}
