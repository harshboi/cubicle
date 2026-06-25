package graphql

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"

	_ "github.com/mattn/go-sqlite3"
)

func TestWorkProgramPacketsAgainstPersistedDB(t *testing.T) {
	dbPath := strings.TrimSpace(os.Getenv("CUBICLE_TPM_PACKET_SMOKE_DB"))
	if dbPath == "" {
		t.Skip("set CUBICLE_TPM_PACKET_SMOKE_DB to run persisted AI-TPM packet smoke")
	}
	absoluteDBPath, err := filepath.Abs(dbPath)
	if err != nil {
		t.Fatalf("resolve db path: %v", err)
	}
	if _, err := os.Stat(absoluteDBPath); err != nil {
		t.Fatalf("stat smoke db: %v", err)
	}

	ctx := context.Background()
	rawDB, err := sql.Open("sqlite3", absoluteDBPath)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer rawDB.Close()

	anchor := latestPacketSmokeRunAnchor(t, ctx, rawDB)
	if source := strings.TrimSpace(os.Getenv("CUBICLE_TPM_PACKET_SMOKE_SOURCE")); source != "" {
		anchor.sourceInstance = source
	}
	if workstream := strings.TrimSpace(os.Getenv("CUBICLE_TPM_PACKET_SMOKE_WORKSTREAM")); workstream != "" {
		anchor.workstreamKey = strings.TrimPrefix(workstream, "workstream:")
	}
	runPrefix := workProgramAutomationReadinessRunPrefix(anchor.externalID)
	if runPrefix == "" {
		t.Fatalf("latest readiness external_id %q has no run prefix", anchor.externalID)
	}

	store, err := entstore.Open(ctx, entstore.Config{DatabasePath: absoluteDBPath})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	expected := packetSmokeExpectedCounts(t, ctx, rawDB, anchor, runPrefix, packetSmokeAdversarialRunPrefix(runPrefix))

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := anchor.sourceInstance
	workstreamArg := workProgramSmokeWorkstreamArg(anchor.workstreamKey)
	evidenceLimit := 5
	functionLimit := 20
	reviewLimit := 5
	actionLimit := 5

	tpm, err := resolver.WorkProgramTpmReadinessPacket(ctx, workstreamArg, &functionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("tpm readiness packet: %v", err)
	}
	assertPacketSmokeGeneratedAt(t, "tpm readiness", tpm.GeneratedAt, anchor.generatedAt)
	if tpm.EvidenceNeedCount != expected.evidenceNeedCount {
		t.Fatalf("tpm evidenceNeedCount = %d, want persisted run count %d", tpm.EvidenceNeedCount, expected.evidenceNeedCount)
	}
	if tpm.BlockingGateCount != expected.blockingGateCount {
		t.Fatalf("tpm blockingGateCount = %d, want persisted run count %d", tpm.BlockingGateCount, expected.blockingGateCount)
	}
	if tpm.FailedCheckCount != expected.failedCheckCount {
		t.Fatalf("tpm failedCheckCount = %d, want persisted run count %d", tpm.FailedCheckCount, expected.failedCheckCount)
	}
	if tpm.TotalFunctionCount != expected.functionCount || tpm.ReadyFunctionCount != expected.readyFunctionCount || tpm.SupervisedFunctionCount != expected.supervisedFunctionCount || tpm.BlockedFunctionCount != expected.blockedFunctionCount {
		t.Fatalf("tpm function counts = total:%d ready:%d supervised:%d blocked:%d, want %d/%d/%d/%d", tpm.TotalFunctionCount, tpm.ReadyFunctionCount, tpm.SupervisedFunctionCount, tpm.BlockedFunctionCount, expected.functionCount, expected.readyFunctionCount, expected.supervisedFunctionCount, expected.blockedFunctionCount)
	}
	if tpm.DecisionTargetEvaluationCount != expected.decisionTargetEvaluationCount {
		t.Fatalf("tpm decisionTargetEvaluationCount = %d, want persisted latest-run count %d", tpm.DecisionTargetEvaluationCount, expected.decisionTargetEvaluationCount)
	}
	if tpm.DecisionTargetReadiness == nil {
		t.Fatalf("tpm decision target readiness missing")
	}
	if tpm.ProductReadyDecisionTargetEvaluationCount != tpm.DecisionTargetReadiness.ProductReadyEvaluationCount {
		t.Fatalf("tpm product-ready decision target count = %d, readiness count = %d", tpm.ProductReadyDecisionTargetEvaluationCount, tpm.DecisionTargetReadiness.ProductReadyEvaluationCount)
	}
	if tpm.DecisionTargetReadiness.CoverageGateState != "passed" && tpm.ProductReadyDecisionTargetEvaluationCount != 0 {
		t.Fatalf("tpm product-ready decision target count = %d despite coverage gate %s", tpm.ProductReadyDecisionTargetEvaluationCount, tpm.DecisionTargetReadiness.CoverageGateState)
	}
	if linkCount := packetSmokeTPMFunctionBlockingGateLinkCount(t, ctx, rawDB, anchor, runPrefix); linkCount != packetSmokeTPMFunctionBlockingGateModelCount(tpm.TpmFunctionReadiness) {
		t.Fatalf("tpm function blocking gate links = %d, want persisted join count %d", packetSmokeTPMFunctionBlockingGateModelCount(tpm.TpmFunctionReadiness), linkCount)
	}

	guardrail := tpm.GuardrailPacket
	if guardrail == nil {
		t.Fatalf("tpm guardrail packet missing")
	}
	assertPacketSmokeGeneratedAt(t, "guardrail", guardrail.GeneratedAt, anchor.generatedAt)
	if guardrail.EvidenceNeedCount != expected.evidenceNeedCount || guardrail.BlockingGateCount != expected.blockingGateCount || guardrail.FailedCheckCount != expected.failedCheckCount {
		t.Fatalf("guardrail counts = evidence:%d gates:%d failed:%d, want %d/%d/%d", guardrail.EvidenceNeedCount, guardrail.BlockingGateCount, guardrail.FailedCheckCount, expected.evidenceNeedCount, expected.blockingGateCount, expected.failedCheckCount)
	}

	execution, err := resolver.WorkProgramExecutionPacket(ctx, workstreamArg, nil, &actionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("execution packet: %v", err)
	}
	assertPacketSmokeGeneratedAt(t, "execution", execution.GeneratedAt, anchor.generatedAt)
	if execution.EvidenceNeedCount != expected.evidenceNeedCount {
		t.Fatalf("execution evidenceNeedCount = %d, want persisted run count %d", execution.EvidenceNeedCount, expected.evidenceNeedCount)
	}

	plan, err := resolver.WorkProgramAutomationPlanPacket(ctx, workstreamArg, nil, &actionLimit, &evidenceLimit, &reviewLimit, &sourceArg)
	if err != nil {
		t.Fatalf("automation plan packet: %v", err)
	}
	assertPacketSmokeGeneratedAt(t, "automation plan", plan.GeneratedAt, anchor.generatedAt)
	if plan.EvidenceNeedCount != expected.evidenceNeedCount {
		t.Fatalf("plan evidenceNeedCount = %d, want persisted run count %d", plan.EvidenceNeedCount, expected.evidenceNeedCount)
	}

	sourceCoverage, err := resolver.WorkProgramSourceCoveragePacket(ctx, workstreamArg, nil, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("source coverage packet: %v", err)
	}
	assertPacketSmokeGeneratedAt(t, "source coverage", sourceCoverage.GeneratedAt, anchor.generatedAt)
	if sourceCoverage.EvidenceNeedCount != expected.sourceCoverageEvidenceNeedCount {
		t.Fatalf("source coverage evidenceNeedCount = %d, want persisted source guardrail count %d", sourceCoverage.EvidenceNeedCount, expected.sourceCoverageEvidenceNeedCount)
	}

	forecast, err := resolver.WorkProgramForecastPacket(ctx, workstreamArg, nil, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("forecast packet: %v", err)
	}
	assertPacketSmokeGeneratedAt(t, "forecast", forecast.GeneratedAt, anchor.generatedAt)
	if expected.forecastReadinessEvidenceNeedCount > 0 && forecast.EvidenceNeedCount == 0 {
		t.Fatalf("forecast evidenceNeedCount = 0, want nonzero because persisted run has %d forecast-readiness need(s)", expected.forecastReadinessEvidenceNeedCount)
	}
	if forecast.Readiness == nil {
		t.Fatalf("forecast readiness missing")
	}
	if forecast.EtaForecastReady != forecast.Readiness.EtaForecastReady {
		t.Fatalf("forecast etaForecastReady = %v, readiness etaForecastReady = %v", forecast.EtaForecastReady, forecast.Readiness.EtaForecastReady)
	}
	if forecast.EtaForecastReady {
		if !strings.Contains(forecast.AutomationSummary, "ETA candidates") {
			t.Fatalf("ETA-ready forecast summary %q does not mention ETA candidate use", forecast.AutomationSummary)
		}
	} else if !strings.Contains(forecast.AutomationSummary, "risk triage only") {
		t.Fatalf("gated forecast summary %q does not preserve risk-triage-only framing", forecast.AutomationSummary)
	}
	if forecast.DecisionTargetEvaluationCount != expected.decisionTargetEvaluationCount {
		t.Fatalf("forecast decisionTargetEvaluationCount = %d, want persisted latest-run count %d", forecast.DecisionTargetEvaluationCount, expected.decisionTargetEvaluationCount)
	}
	if forecast.DecisionTargetReadiness == nil || forecast.DecisionTargetReadiness.EvaluationCount != expected.decisionTargetEvaluationCount {
		t.Fatalf("forecast decision target readiness = %#v, want latest-run count %d", forecast.DecisionTargetReadiness, expected.decisionTargetEvaluationCount)
	}
	reliabilityByProduct := workProgramForecastReliabilityByProduct(forecast.ForecastReliability)
	if len(reliabilityByProduct) != 3 {
		t.Fatalf("forecast reliability rows = %#v, want point, range, and risk rows", forecast.ForecastReliability)
	}
	pointETA := reliabilityByProduct["point_eta"]
	if pointETA == nil {
		t.Fatalf("point ETA reliability missing: %#v", forecast.ForecastReliability)
	}
	if pointETA.ProductSafe != forecast.EtaForecastReady {
		t.Fatalf("point ETA productSafe = %v, forecast etaReady = %v", pointETA.ProductSafe, forecast.EtaForecastReady)
	}
	if !forecast.EtaForecastReady && (pointETA.SafeUse != "diagnostic_only" || !strings.Contains(pointETA.Guardrail, "risk triage")) {
		t.Fatalf("gated point ETA reliability = %#v, want diagnostic-only risk-triage guardrail", pointETA)
	}
	rangeETA := reliabilityByProduct["range_eta"]
	if rangeETA == nil || rangeETA.ProductSafe || rangeETA.SafeUse != "wide_range_context" || rangeETA.BestModel == "" || rangeETA.MetricValue == "" {
		t.Fatalf("range ETA reliability = %#v, want diagnostic remaining-time baseline", rangeETA)
	}
	riskTriage := reliabilityByProduct["risk_triage"]
	if riskTriage == nil || riskTriage.SafeUse != "attention_ordering" || riskTriage.PrimaryMetric != "risk_triage_lift_at_10pct" || !strings.Contains(riskTriage.Guardrail, "attention ordering") {
		t.Fatalf("risk triage reliability = %#v, want attention-ordering guardrail", riskTriage)
	}

	blocker, err := resolver.WorkProgramBlockerPacket(ctx, workstreamArg, nil, nil, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("blocker packet: %v", err)
	}
	assertPacketSmokeGeneratedAt(t, "blocker", blocker.GeneratedAt, anchor.generatedAt)
	if expected.blockerEvidenceNeedCount > 0 && blocker.EvidenceNeedCount == 0 {
		t.Fatalf("blocker evidenceNeedCount = 0, want nonzero because persisted run has %d blocker-related need(s)", expected.blockerEvidenceNeedCount)
	}
	blockerMeasurementKind := "blocker_candidate"
	blockerMeasurement, err := resolver.WorkInsightMeasurementPacket(ctx, &sourceArg, &reviewLimit, &blockerMeasurementKind)
	if err != nil {
		t.Fatalf("blocker insight measurement packet: %v", err)
	}
	expectedBlockerMeasurement := packetSmokeInsightKindCounts(t, ctx, rawDB, anchor.sourceInstance, blockerMeasurementKind)
	expectedBlockerQueueCount := packetSmokeInsightKindReviewQueueCount(t, ctx, rawDB, anchor.sourceInstance, blockerMeasurementKind)
	if blockerMeasurement.InsightKind == nil || *blockerMeasurement.InsightKind != blockerMeasurementKind {
		t.Fatalf("blocker measurement insightKind = %#v, want %s", blockerMeasurement.InsightKind, blockerMeasurementKind)
	}
	if blockerMeasurement.CurrentInsightCount != expectedBlockerMeasurement.currentInsightCount || blockerMeasurement.MeasurementLabelCount != expectedBlockerMeasurement.measurementLabelCount || blockerMeasurement.OpenReviewRequestCount != expectedBlockerMeasurement.openReviewRequestCount {
		t.Fatalf("blocker measurement counts = current:%d labels:%d open:%d, want latest persisted kind %d/%d/%d", blockerMeasurement.CurrentInsightCount, blockerMeasurement.MeasurementLabelCount, blockerMeasurement.OpenReviewRequestCount, expectedBlockerMeasurement.currentInsightCount, expectedBlockerMeasurement.measurementLabelCount, expectedBlockerMeasurement.openReviewRequestCount)
	}
	if blockerMeasurement.ReviewQueueTotalCount != expectedBlockerQueueCount {
		t.Fatalf("blocker measurement reviewQueueTotalCount = %d, want persisted blocker review queue count %d", blockerMeasurement.ReviewQueueTotalCount, expectedBlockerQueueCount)
	}
	for _, review := range blockerMeasurement.ReviewQueue {
		if review == nil || review.Insight == nil || review.Insight.InsightKind != blockerMeasurementKind {
			t.Fatalf("blocker measurement review queue contains non-blocker row: %#v", review)
		}
	}
	if expectedBlockerMeasurement.measurementLabelCount == 0 && expectedBlockerMeasurement.openReviewRequestCount > 0 && blockerMeasurement.ProductActionReady {
		t.Fatalf("blocker measurement is product-action ready with zero labels and %d open reviews", expectedBlockerMeasurement.openReviewRequestCount)
	}
	assertPacketSmokeDependencyValidationEdgesGated(t, ctx, rawDB, resolver, sourceArg)
	assertPacketSmokeDependencyEndpointsExposed(t, ctx, rawDB, resolver, sourceArg)

	milestoneLimit := 10
	milestone, err := resolver.WorkProgramMilestonePacket(ctx, workstreamArg, &milestoneLimit, &sourceArg)
	if err != nil {
		t.Fatalf("milestone packet: %v", err)
	}
	assertPacketSmokeGeneratedAt(t, "milestone", milestone.GeneratedAt, anchor.generatedAt)
	if milestone.TotalCount != expected.milestoneCount {
		t.Fatalf("milestone totalCount = %d, want persisted run count %d", milestone.TotalCount, expected.milestoneCount)
	}
	if milestone.ReleaseTargetCount != expected.releaseTargetMilestoneCount || milestone.DatedReleaseTargetCount != expected.datedReleaseTargetMilestoneCount {
		t.Fatalf("milestone release counts = %d/%d, want %d/%d", milestone.ReleaseTargetCount, milestone.DatedReleaseTargetCount, expected.releaseTargetMilestoneCount, expected.datedReleaseTargetMilestoneCount)
	}
	if milestone.ExplicitDueDateCount != expected.explicitDueDateMilestoneCount || milestone.DeliveryCommitmentCount != expected.deliveryCommitmentMilestoneCount || milestone.DeliveryCommitmentAllowedCount != expected.deliveryCommitmentAllowedMilestoneCount {
		t.Fatalf("milestone commitment counts = due:%d delivery:%d allowed:%d, want %d/%d/%d", milestone.ExplicitDueDateCount, milestone.DeliveryCommitmentCount, milestone.DeliveryCommitmentAllowedCount, expected.explicitDueDateMilestoneCount, expected.deliveryCommitmentMilestoneCount, expected.deliveryCommitmentAllowedMilestoneCount)
	}
	if milestone.OutcomeFactCount != expected.outcomeMilestoneCount || milestone.DateClaimAllowedCount != expected.dateClaimAllowedMilestoneCount {
		t.Fatalf("milestone outcome/date-claim counts = %d/%d, want %d/%d", milestone.OutcomeFactCount, milestone.DateClaimAllowedCount, expected.outcomeMilestoneCount, expected.dateClaimAllowedMilestoneCount)
	}
	assertPacketSmokeMilestoneBadgesAvoidAbsenceClaims(t, milestone.Badges)

	attention, err := resolver.WorkProgramAttentionPacket(ctx, workstreamArg, nil, &evidenceLimit, &sourceArg)
	if err != nil {
		t.Fatalf("attention packet: %v", err)
	}
	assertPacketSmokeGeneratedAt(t, "attention", attention.GeneratedAt, anchor.generatedAt)
	if attention.EvidenceNeedCount != expected.evidenceNeedCount {
		t.Fatalf("attention evidenceNeedCount = %d, want execution evidence count %d", attention.EvidenceNeedCount, expected.evidenceNeedCount)
	}

	if owner := packetSmokeOwner(t, ctx, rawDB, anchor); owner != "" {
		ownerPacket, err := resolver.WorkProgramOwnerPacket(ctx, workstreamArg, owner, nil, &actionLimit, &evidenceLimit, &sourceArg)
		if err != nil {
			t.Fatalf("owner packet: %v", err)
		}
		assertPacketSmokeGeneratedAt(t, "owner", ownerPacket.GeneratedAt, anchor.generatedAt)
	}

	t.Logf("packet smoke ok: source=%s workstream=%s generatedAt=%s evidence=%d gates=%d failedChecks=%d functions=%d sourceGuardrailEvidence=%d", anchor.sourceInstance, workstreamArg, anchor.generatedAt.Format(time.RFC3339Nano), expected.evidenceNeedCount, expected.blockingGateCount, expected.failedCheckCount, expected.functionCount, expected.sourceCoverageEvidenceNeedCount)
}

type packetSmokeRunAnchor struct {
	sourceInstance string
	workstreamKey  string
	generatedAt    time.Time
	externalID     string
}

type packetSmokeCounts struct {
	evidenceNeedCount                       int
	sourceCoverageEvidenceNeedCount         int
	forecastReadinessEvidenceNeedCount      int
	blockerEvidenceNeedCount                int
	blockingGateCount                       int
	failedCheckCount                        int
	functionCount                           int
	readyFunctionCount                      int
	supervisedFunctionCount                 int
	blockedFunctionCount                    int
	decisionTargetEvaluationCount           int
	milestoneCount                          int
	releaseTargetMilestoneCount             int
	datedReleaseTargetMilestoneCount        int
	explicitDueDateMilestoneCount           int
	deliveryCommitmentMilestoneCount        int
	deliveryCommitmentAllowedMilestoneCount int
	outcomeMilestoneCount                   int
	dateClaimAllowedMilestoneCount          int
}

type packetSmokeInsightKindTotals struct {
	currentInsightCount    int
	measurementLabelCount  int
	openReviewRequestCount int
}

func latestPacketSmokeRunAnchor(t *testing.T, ctx context.Context, db *sql.DB) packetSmokeRunAnchor {
	t.Helper()
	var generatedAtText string
	anchor := packetSmokeRunAnchor{}
	err := db.QueryRowContext(ctx, `
select source_instance, workstream_key, generated_at, external_id
  from work_program_automation_readinesses
 order by generated_at desc, rank_score desc
 limit 1
`).Scan(&anchor.sourceInstance, &anchor.workstreamKey, &generatedAtText, &anchor.externalID)
	if err != nil {
		t.Fatalf("read latest readiness run: %v", err)
	}
	anchor.generatedAt = parsePacketSmokeTime(t, generatedAtText)
	return anchor
}

func packetSmokeExpectedCounts(t *testing.T, ctx context.Context, db *sql.DB, anchor packetSmokeRunAnchor, runPrefix string, adversarialRunPrefix string) packetSmokeCounts {
	t.Helper()
	counts := packetSmokeCounts{}
	counts.evidenceNeedCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_evidence_needs where source_instance = ? and external_id like ?`, anchor.sourceInstance, runPrefix+"%")
	counts.sourceCoverageEvidenceNeedCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_evidence_needs where source_instance = ? and external_id like ? and gate_key in ('source_coverage', 'source_authentication', 'claim_provenance')`, anchor.sourceInstance, runPrefix+"%")
	counts.forecastReadinessEvidenceNeedCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_evidence_needs where source_instance = ? and external_id like ? and gate_key = 'forecast_readiness'`, anchor.sourceInstance, runPrefix+"%")
	counts.blockerEvidenceNeedCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_evidence_needs where source_instance = ? and external_id like ? and gate_key in ('blocker_clearance', 'source_coverage', 'forecast_readiness')`, anchor.sourceInstance, runPrefix+"%")
	counts.blockingGateCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_quality_gates where source_instance = ? and external_id like ? and blocking = 1`, anchor.sourceInstance, runPrefix+"%")
	counts.failedCheckCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_adversarial_checks where source_instance = ? and external_id like ? and check_state = 'fail'`, anchor.sourceInstance, adversarialRunPrefix+"%")
	counts.functionCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_tpm_function_readinesses where source_instance = ? and external_id like ?`, anchor.sourceInstance, runPrefix+"%")
	counts.readyFunctionCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_tpm_function_readinesses where source_instance = ? and external_id like ? and readiness_state in ('ready', 'automatable')`, anchor.sourceInstance, runPrefix+"%")
	counts.supervisedFunctionCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_tpm_function_readinesses where source_instance = ? and external_id like ? and readiness_state = 'supervised'`, anchor.sourceInstance, runPrefix+"%")
	counts.blockedFunctionCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_tpm_function_readinesses where source_instance = ? and external_id like ? and readiness_state = 'blocked'`, anchor.sourceInstance, runPrefix+"%")
	counts.milestoneCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_milestones where source_instance = ? and external_id like ?`, anchor.sourceInstance, runPrefix+"%")
	counts.releaseTargetMilestoneCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_milestones where source_instance = ? and external_id like ? and milestone_kind = 'release_target'`, anchor.sourceInstance, runPrefix+"%")
	counts.datedReleaseTargetMilestoneCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_milestones where source_instance = ? and external_id like ? and milestone_kind = 'release_target' and target_date is not null`, anchor.sourceInstance, runPrefix+"%")
	counts.explicitDueDateMilestoneCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_milestones where source_instance = ? and external_id like ? and milestone_kind = 'explicit_due_date'`, anchor.sourceInstance, runPrefix+"%")
	counts.deliveryCommitmentMilestoneCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_milestones where source_instance = ? and external_id like ? and commitment_strength = 'explicit_commitment' and delivery_commitment_allowed = 1`, anchor.sourceInstance, runPrefix+"%")
	counts.deliveryCommitmentAllowedMilestoneCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_milestones where source_instance = ? and external_id like ? and delivery_commitment_allowed = 1`, anchor.sourceInstance, runPrefix+"%")
	counts.outcomeMilestoneCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_milestones where source_instance = ? and external_id like ? and milestone_kind = 'resolution_outcome'`, anchor.sourceInstance, runPrefix+"%")
	counts.dateClaimAllowedMilestoneCount = packetSmokeCount(t, ctx, db, `select count(*) from work_program_milestones where source_instance = ? and external_id like ? and date_claim_allowed = 1`, anchor.sourceInstance, runPrefix+"%")
	counts.decisionTargetEvaluationCount = packetSmokeCount(t, ctx, db, `
select count(*)
  from work_decision_target_evaluations
 where source_instance = ?
   and source_system = 'cubicle_analytics'
   and external_kind = 'tpm_decision_target_evaluation'
   and evaluated_at = (
     select max(evaluated_at)
       from work_decision_target_evaluations
      where source_instance = ?
        and source_system = 'cubicle_analytics'
        and external_kind = 'tpm_decision_target_evaluation'
   )
`, anchor.sourceInstance, anchor.sourceInstance)
	return counts
}

func packetSmokeAdversarialRunPrefix(runPrefix string) string {
	parts := strings.Split(strings.TrimSuffix(runPrefix, "|"), "|")
	if len(parts) < 2 {
		return runPrefix
	}
	return parts[0] + ":" + parts[1] + ":"
}

func packetSmokeCount(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v\nquery: %s", err, query)
	}
	return count
}

func packetSmokeInsightKindCounts(t *testing.T, ctx context.Context, db *sql.DB, sourceInstance string, insightKind string) packetSmokeInsightKindTotals {
	t.Helper()
	counts := packetSmokeInsightKindTotals{}
	err := db.QueryRowContext(ctx, `
select current_insight_count, measurement_label_count, open_review_request_count
  from work_insight_kind_evaluation_snapshots
 where source_instance = ?
   and source_system = 'cubicle_analytics'
   and external_kind = 'tpm_work_insight_kind_evaluation_snapshot'
   and insight_kind = ?
 order by generated_at desc, rank_score desc, updated_at desc
 limit 1
`, sourceInstance, insightKind).Scan(&counts.currentInsightCount, &counts.measurementLabelCount, &counts.openReviewRequestCount)
	if err != nil {
		t.Fatalf("read latest %s insight-kind counts: %v", insightKind, err)
	}
	return counts
}

func packetSmokeInsightKindReviewQueueCount(t *testing.T, ctx context.Context, db *sql.DB, sourceInstance string, insightKind string) int {
	t.Helper()
	return packetSmokeCount(t, ctx, db, `
select count(*)
  from work_insight_reviews wir
  join work_insights wi on wi.id = wir.work_insight_id
 where wir.source_instance = ?
   and wir.source_system = 'cubicle_analytics'
   and wir.external_kind = 'tpm_insight_review'
   and wir.review_state = 'requested'
   and wir.review_kind = 'triage_request'
   and wi.source_system = 'cubicle_analytics'
   and wi.source_instance = ?
   and wi.external_kind = 'tpm_insight'
   and wi.producer_state = 'current'
   and wi.insight_kind = ?
`, sourceInstance, sourceInstance, insightKind)
}

func assertPacketSmokeMilestoneBadgesAvoidAbsenceClaims(t *testing.T, badges []*model.WorkActionBadge) {
	t.Helper()
	for _, badge := range badges {
		if strings.Contains(badge.Label, "No due-date commitments") || strings.Contains(badge.Label, "No delivery commitments") {
			t.Fatalf("milestone packet badge makes unsupported absence claim: %#v", badge)
		}
	}
}

func packetSmokeTPMFunctionBlockingGateLinkCount(t *testing.T, ctx context.Context, db *sql.DB, anchor packetSmokeRunAnchor, runPrefix string) int {
	t.Helper()
	return packetSmokeCount(
		t,
		ctx,
		db,
		`
select count(*)
  from work_program_tpm_function_readiness_blocking_quality_gates links
  join work_program_tpm_function_readinesses functions
    on functions.id = links.work_program_tpm_function_readiness_id
 where functions.source_instance = ?
   and functions.external_id like ?
`,
		anchor.sourceInstance,
		runPrefix+"%",
	)
}

func packetSmokeTPMFunctionBlockingGateModelCount(functions []*model.WorkProgramTpmFunctionReadiness) int {
	count := 0
	for _, function := range functions {
		if function == nil {
			continue
		}
		count += len(function.BlockingGates)
	}
	return count
}

func assertPacketSmokeDependencyValidationEdgesGated(t *testing.T, ctx context.Context, db *sql.DB, resolver *queryResolver, source string) {
	t.Helper()
	validationKeys := packetSmokeDependencyValidationKeys(t, ctx, db, source)
	if len(validationKeys) == 0 {
		return
	}
	limit := 500
	rows, err := resolver.WorkDependencyEdges(ctx, &limit, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("dependency edges: %v", err)
	}
	validationSet := map[string]bool{}
	for _, key := range validationKeys {
		validationSet[key] = true
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if !validationSet[row.Key] {
			continue
		}
		seen[row.Key] = true
		if row.RelationshipClaimAllowed {
			t.Fatalf("dependency edge %q is still validation-backed but relationshipClaimAllowed=true", row.Key)
		}
		if row.ClaimUse == "relationship_claim" || row.ClaimGateReason == "relationship_claim_gate_passed" {
			t.Fatalf("dependency edge %q is still validation-backed but emitted relationship claim fields: use=%q reason=%q", row.Key, row.ClaimUse, row.ClaimGateReason)
		}
	}
	for _, key := range validationKeys {
		if !seen[key] {
			t.Fatalf("validation-backed dependency edge %q not returned by workDependencyEdges smoke query", key)
		}
	}
	t.Logf("dependency edge claim gate ok: %d validation-backed edge(s) remained validation-only", len(validationKeys))
}

func assertPacketSmokeDependencyEndpointsExposed(t *testing.T, ctx context.Context, db *sql.DB, resolver *queryResolver, source string) {
	t.Helper()
	endpointCount := packetSmokeCount(t, ctx, db, `select count(*) from work_dependency_endpoints where source_instance = ?`, source)
	if endpointCount == 0 {
		t.Fatalf("persisted smoke DB has no dependency endpoints for source %q", source)
	}
	ticketPREdgeCount := packetSmokeCount(t, ctx, db, `select count(*) from work_dependency_edges where source_instance = ? and edge_kind = 'ticket_pr'`, source)
	canonicalMirrorCount := packetSmokeCount(t, ctx, db, `select count(*) from work_dependency_edges where source_instance = ? and edge_kind = 'ticket_pr' and relationship_authority = 'canonical_mirror' and canonical_relationship_kind = 'ticket_pull_request'`, source)
	if canonicalMirrorCount != ticketPREdgeCount {
		t.Fatalf("ticket_pr canonical mirror count = %d, want all %d ticket_pr edge(s)", canonicalMirrorCount, ticketPREdgeCount)
	}
	projectionCanonicalCount := packetSmokeCount(t, ctx, db, `select count(*) from work_dependency_edges where source_instance = ? and relationship_authority != 'canonical_mirror' and canonical_relationship_kind is not null`, source)
	if projectionCanonicalCount != 0 {
		t.Fatalf("operating projection canonical kind count = %d, want 0", projectionCanonicalCount)
	}
	componentEdgeCount := packetSmokeCount(t, ctx, db, `select count(*) from work_dependency_edges where source_instance = ? and from_kind = 'component'`, source)
	if componentEdgeCount == 0 {
		t.Fatalf("persisted smoke DB has no component-origin dependency edges for source %q", source)
	}
	limit := 20
	fromKind := "component"
	rows, err := resolver.WorkDependencyEdges(ctx, &limit, &fromKind, nil, nil, &source)
	if err != nil {
		t.Fatalf("component dependency edges: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("component dependency edge smoke query returned no rows, want up to %d of %d persisted edge(s)", limit, componentEdgeCount)
	}
	for _, row := range rows {
		if len(row.Endpoints) != 2 {
			t.Fatalf("dependency edge %q exposed %d endpoint(s), want 2", row.Key, len(row.Endpoints))
		}
		if row.RelationshipAuthority != "operating_projection" || row.CanonicalRelationshipKind != nil {
			t.Fatalf("dependency edge %q authority = %q canonical=%#v, want operating projection", row.Key, row.RelationshipAuthority, row.CanonicalRelationshipKind)
		}
		if row.FromEndpoint == nil || row.ToEndpoint == nil {
			t.Fatalf("dependency edge %q endpoints = from:%#v to:%#v, want both convenience endpoints", row.Key, row.FromEndpoint, row.ToEndpoint)
		}
		if row.FromEndpoint.EndpointRole != "from" || row.FromEndpoint.NodeKind != "component" || row.FromEndpoint.ResolutionState != "key_only" {
			t.Fatalf("dependency edge %q fromEndpoint = %#v, want key-only component endpoint", row.Key, row.FromEndpoint)
		}
		if row.ToEndpoint.EndpointRole != "to" || row.ToEndpoint.ResolutionState != "resolved" {
			t.Fatalf("dependency edge %q toEndpoint = %#v, want resolved to endpoint", row.Key, row.ToEndpoint)
		}
	}
	t.Logf("dependency endpoint exposure ok: checked %d component-origin edge(s), endpoints=%d canonicalMirrors=%d", len(rows), endpointCount, canonicalMirrorCount)
}

func packetSmokeDependencyValidationKeys(t *testing.T, ctx context.Context, db *sql.DB, source string) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
select wde.key
  from work_dependency_edges wde
  left join work_blockers wb on wb.id = wde.work_blocker_id
  left join work_actions wa on wa.id = wde.work_action_id
 where wde.source_instance = ?
   and wde.edge_kind in ('blocked_by', 'needs_action')
   and (
     wb.decision_state = 'validation_lead'
     or wb.review_state = 'needs_more_data'
     or wa.decision_state = 'validation_lead'
   )
 order by wde.rank_score desc, wde.last_activity_at desc
`, source)
	if err != nil {
		t.Fatalf("query validation dependency edges: %v", err)
	}
	defer rows.Close()

	keys := []string{}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			t.Fatalf("scan validation dependency edge: %v", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate validation dependency edges: %v", err)
	}
	return keys
}

func packetSmokeOwner(t *testing.T, ctx context.Context, db *sql.DB, anchor packetSmokeRunAnchor) string {
	t.Helper()
	if owner := strings.TrimSpace(os.Getenv("CUBICLE_TPM_PACKET_SMOKE_OWNER")); owner != "" {
		return owner
	}
	var owner string
	err := db.QueryRowContext(ctx, `
select owner_key
  from work_owner_load_snapshots
 where source_instance = ?
   and workstream_key in (?, ?)
   and owner_key not in ('', '(clear)', '(unassigned)')
 order by generated_at desc, action_count desc, rank_score desc
 limit 1
`, anchor.sourceInstance, anchor.workstreamKey, "workstream:"+anchor.workstreamKey).Scan(&owner)
	if err == sql.ErrNoRows {
		return ""
	}
	if err != nil {
		t.Fatalf("read smoke owner: %v", err)
	}
	return owner
}

func workProgramSmokeWorkstreamArg(workstreamKey string) string {
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" || strings.HasPrefix(workstreamKey, "workstream:") {
		return workstreamKey
	}
	return "workstream:" + workstreamKey
}

func assertPacketSmokeGeneratedAt(t *testing.T, label string, got *string, want time.Time) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s generatedAt is nil, want %s", label, want.Format(time.RFC3339Nano))
	}
	gotTime := parsePacketSmokeTime(t, *got)
	if !gotTime.Equal(want.UTC()) {
		t.Fatalf("%s generatedAt = %s, want %s", label, gotTime.Format(time.RFC3339Nano), want.UTC().Format(time.RFC3339Nano))
	}
}

func parsePacketSmokeTime(t *testing.T, value string) time.Time {
	t.Helper()
	value = strings.TrimSpace(value)
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02T15:04:05.999999999-07:00",
	} {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed.UTC()
		}
	}
	t.Fatalf("parse time %q", value)
	return time.Time{}
}
