package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workdependencyedge"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramGraphContextBuildsTypedLLMContext(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	decoySource := "other-source"
	workstream := "flink-kubernetes-operator"
	queryWorkstream := "workstream:" + workstream
	subjectKey := "github:apache/flink-kubernetes-operator#100"
	generatedAt := time.Date(2026, 6, 23, 12, 0, 0, 0, time.UTC)

	action, err := store.Client().WorkAction.Create().
		SetKey("work-action:graph-context:product").
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
		SetExternalID("graph-context:product-action").
		SetFreshnessState(workaction.FreshnessStateFresh).
		SetRankScore(90).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	item, err := store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:graph-context:subject").
		SetWorkAction(action).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Graph context subject").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(91).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("graph-context:subject").
		SetRankScore(91).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}
	dependency, err := store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:graph-context:needs-action").
		SetEdgeKind(workdependencyedge.EdgeKindNeedsAction).
		SetFromKind(workdependencyedge.FromKindPullRequest).
		SetFromKey(subjectKey).
		SetToKind(workdependencyedge.ToKindAction).
		SetToKey(action.Key).
		SetWorkAction(action).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("graph-context:needs-action").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetConfidence(0.94).
		SetRankScore(92).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create dependency edge: %v", err)
	}
	insight, err := store.Client().WorkInsight.Create().
		SetKey("work-insight:graph-context:status").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityInfo).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Status summary").
		SetDetails("Maintainer follow-up is ready for review.").
		SetRecommendedAction("Confirm owner status.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight").
		SetExternalID("graph-context:status").
		SetFreshnessState(workinsight.FreshnessStateFresh).
		SetRankScore(80).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create insight: %v", err)
	}
	_, err = store.Client().WorkInsight.Create().
		SetKey("work-insight:graph-context:generated-summary-launder").
		SetInsightKind(workinsight.InsightKindAiGraphBrief).
		SetSeverity(workinsight.SeverityInfo).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Generated summary must not re-enter graph context").
		SetDetails("Verifier-passed generated summary is generated evidence, not source truth.").
		SetRecommendedAction("Read through workProgramGraphBrief, not graph-context insights.").
		SetModelMethod("bounded_graph_context_to_cited_brief:generic").
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief").
		SetExternalID("graph-context|generic|ctx|ai_graph_brief").
		SetSourceURL("cubicle://graph-brief/generic/ctx").
		SetFreshnessState(workinsight.FreshnessStateFresh).
		SetRankScore(1000).
		SetLastActivityAt(generatedAt.Add(2 * time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create generated summary insight: %v", err)
	}
	_, err = store.Client().WorkInsight.Create().
		SetKey("work-insight:graph-context:misclassified-generated-summary").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityInfo).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Misclassified generated graph brief must not re-enter context").
		SetDetails("This row has the normal TPM source shape but graph-brief producer metadata.").
		SetRecommendedAction("Keep it quarantined from graph-context insights.").
		SetModelMethod("bounded_graph_context_to_cited_brief:generic").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_insight").
		SetExternalID("graph-context:misclassified-generated-summary").
		SetSourceURL("cubicle://graph-brief/generic/ctx").
		SetFreshnessState(workinsight.FreshnessStateFresh).
		SetRankScore(1001).
		SetLastActivityAt(generatedAt.Add(3 * time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create misclassified generated summary insight: %v", err)
	}
	run, err := store.Client().WorkProgramRun.Create().
		SetKey("work-program-run:" + workstream + ":graph-context").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("human_review_required").
		SetReadinessScore(50).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetMemberCount(3).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_run").
		SetExternalID(workstream + "|graph-context-run").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create work program run: %v", err)
	}
	seedWorkProgramGraphContextRunMember(t, ctx, store.Client(), run.ID, run.Key, generatedAt, workProgramRunMemberTableItems, item.ID, item.Key, item.ExternalKind, item.ExternalID, item.RankScore)
	seedWorkProgramGraphContextRunMember(t, ctx, store.Client(), run.ID, run.Key, generatedAt, workProgramRunMemberTableDependencyEdges, dependency.ID, dependency.Key, dependency.ExternalKind, dependency.ExternalID, dependency.RankScore)
	seedWorkProgramGraphContextRunMember(t, ctx, store.Client(), run.ID, run.Key, generatedAt, workProgramRunMemberTableInsights, insight.ID, insight.Key, insight.ExternalKind, insight.ExternalID, insight.RankScore)
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:graph-context:decoy").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("github:apache/flink-kubernetes-operator#999").
		SetTitle("Decoy item").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(1000).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(decoySource).
		SetExternalKind("tpm_program_item").
		SetExternalID("graph-context:decoy").
		SetRankScore(1000).
		SetLastActivityAt(generatedAt.Add(time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create decoy program item: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	got, err := resolver.WorkProgramGraphContext(ctx, queryWorkstream, nil, nil, nil, nil, nil, nil, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("workProgramGraphContext: %v", err)
	}
	if got.WorkstreamKey != queryWorkstream || got.SourceInstance == nil || *got.SourceInstance != source {
		t.Fatalf("context scope = workstream:%q source:%#v, want %s/%s", got.WorkstreamKey, got.SourceInstance, queryWorkstream, source)
	}
	if got.ScopeMode != "explicit_source:latest_run:work_program_run_boundary" {
		t.Fatalf("scopeMode = %q, want explicit run-bound scope", got.ScopeMode)
	}
	if !strings.Contains(got.LlmTask, "hard run boundary") {
		t.Fatalf("llmTask = %q, want hard run boundary instruction", got.LlmTask)
	}
	if !hasWorkGraphContextBadge(got.Badges, "graph_context:run_boundary") {
		t.Fatalf("badges = %#v, want run boundary badge", got.Badges)
	}
	if got.RunKey == nil || *got.RunKey != "work-program-run:"+workstream+":graph-context" {
		t.Fatalf("runKey = %#v, want seeded run", got.RunKey)
	}
	if got.ContextHash == "" || len(got.ContextHash) != 16 {
		t.Fatalf("contextHash = %q, want 16-char hash", got.ContextHash)
	}
	if got.ItemCount != 1 || got.ActionCount != 1 || got.DependencyEdgeCount != 1 || got.InsightCount != 1 {
		t.Fatalf("counts = item:%d action:%d edge:%d insight:%d, want 1/1/1/1", got.ItemCount, got.ActionCount, got.DependencyEdgeCount, got.InsightCount)
	}
	if got.Items[0].Key != item.Key || !got.Items[0].ProductActionAllowed || got.Items[0].ClaimGateReason != "product_action_gate_passed" {
		t.Fatalf("item model = %#v, want scoped product-action item", got.Items[0])
	}
	if got.Actions[0].Key != action.Key || !got.Actions[0].ProductActionAllowed || got.Actions[0].ClaimUse != "product_action" {
		t.Fatalf("action model = %#v, want scoped product-action action", got.Actions[0])
	}
	if got.DependencyEdges[0].ClaimGateReason != "derived_dependency_edge_not_product_claim" || got.DependencyEdges[0].RelationshipClaimAllowed {
		t.Fatalf("dependency gate = %#v, want derived topology context only", got.DependencyEdges[0])
	}
	if got.Insights[0].Key != insight.Key {
		t.Fatalf("insight key = %q, want %s", got.Insights[0].Key, insight.Key)
	}
	for _, contextInsight := range got.Insights {
		if strings.Contains(contextInsight.Key, "generated-summary") {
			t.Fatalf("graph context leaked generated AI brief as insight: %#v", contextInsight)
		}
	}
	for _, citation := range []string{
		"[context:" + got.ContextHash + "]",
		"[work_program_items:" + item.Key + "]",
		"[work_actions:" + action.Key + "]",
		"[work_dependency_edges:work-dependency-edge:graph-context:needs-action]",
		"[work_insights:" + insight.Key + "]",
		"[source_coverage:" + queryWorkstream + "]",
	} {
		assertContainsString(t, got.AllowedCitations, citation)
	}
	citationsByRef := workGraphCitationsByRefForTest(got.Citations)
	itemCitation := citationsByRef["[work_program_items:"+item.Key+"]"]
	if itemCitation == nil || itemCitation.NodeKind != "work_program_item" || !itemCitation.ClaimAllowed || itemCitation.ClaimGateReason == nil || *itemCitation.ClaimGateReason != "product_action_gate_passed" {
		t.Fatalf("item structured citation = %#v, want product-action citation metadata", itemCitation)
	}
	dependencyCitation := citationsByRef["[work_dependency_edges:work-dependency-edge:graph-context:needs-action]"]
	if dependencyCitation == nil || dependencyCitation.NodeKind != "work_dependency_edge" || dependencyCitation.ClaimAllowed || dependencyCitation.SourceURLAllowed || dependencyCitation.ExcerptAllowed {
		t.Fatalf("dependency structured citation = %#v, want non-claimable topology citation without raw-source allowances", dependencyCitation)
	}
	sourceCoverageCitation := citationsByRef["[source_coverage:"+queryWorkstream+"]"]
	if sourceCoverageCitation == nil || sourceCoverageCitation.NodeKind != "work_program_source_coverage_packet" || !sourceCoverageCitation.ClaimAllowed {
		t.Fatalf("source coverage citation = %#v, want packet-level absence gate metadata", sourceCoverageCitation)
	}
	for _, citation := range got.AllowedCitations {
		if strings.Contains(citation, "decoy") || strings.Contains(citation, decoySource) {
			t.Fatalf("allowed citations leaked decoy source: %#v", got.AllowedCitations)
		}
	}
	if got.SourceCoveragePacket == nil || !got.SourceCoveragePacket.AbsenceClaimsAllowed {
		t.Fatalf("source coverage packet = %#v, want complete source coverage", got.SourceCoveragePacket)
	}
	if got.ForecastPacket == nil || !strings.Contains(got.ForecastPacket.AutomationSummary, "risk triage only") {
		t.Fatalf("forecast packet summary = %#v, want risk-triage framing", got.ForecastPacket)
	}

	runKey := "work-program-run:" + workstream + ":graph-context"
	byRun, err := resolver.WorkProgramGraphContext(ctx, queryWorkstream, nil, nil, nil, nil, nil, nil, nil, &runKey, nil, nil)
	if err != nil {
		t.Fatalf("workProgramGraphContext by runKey: %v", err)
	}
	if byRun.SourceInstance == nil || *byRun.SourceInstance != source || byRun.RunKey == nil || *byRun.RunKey != runKey {
		t.Fatalf("run-key scoped context = source:%#v run:%#v, want %s/%s", byRun.SourceInstance, byRun.RunKey, source, runKey)
	}
	if byRun.ScopeMode != "latest_source:explicit_run_key:work_program_run_boundary" {
		t.Fatalf("run-key scopeMode = %q, want explicit run-key scope", byRun.ScopeMode)
	}
}

func seedWorkProgramGraphContextRunMember(t *testing.T, ctx context.Context, client *genent.Client, runID int, runKey string, createdAt time.Time, memberTable string, memberID int, memberKey string, memberExternalKind string, memberExternalID string, memberRankScore float64) {
	t.Helper()
	_, err := client.WorkProgramRunMember.Create().
		SetWorkProgramRunID(runID).
		SetRunKey(runKey).
		SetMemberTable(memberTable).
		SetMemberID(memberID).
		SetMemberKey(memberKey).
		SetMemberExternalKind(memberExternalKind).
		SetMemberExternalID(memberExternalID).
		SetMemberRankScore(memberRankScore).
		SetCreatedAt(createdAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create graph context run member: %v", err)
	}
}

func hasWorkGraphContextBadge(badges []*model.WorkActionBadge, key string) bool {
	for _, badge := range badges {
		if badge != nil && badge.Key == key {
			return true
		}
	}
	return false
}

func TestWorkProgramGraphCitationEvidencePolicyDoesNotQuoteGeneratedEvidence(t *testing.T) {
	proofState, freshnessState, visibility, evidenceRef, excerptAllowed, sourceURLAllowed := workProgramGraphCitationEvidencePolicy(
		&model.WorkEvidenceSummary{
			Ref:            "evidence:generated-brief",
			ProofState:     "generated",
			FreshnessState: "fresh",
			Visibility:     "public",
		},
		"fresh",
		"public",
	)

	if proofState != "generated" || freshnessState != "fresh" || visibility != "public" || evidenceRef == nil || *evidenceRef != "evidence:generated-brief" {
		t.Fatalf("policy state = proof:%s freshness:%s visibility:%s ref:%#v", proofState, freshnessState, visibility, evidenceRef)
	}
	if excerptAllowed || sourceURLAllowed {
		t.Fatalf("generated evidence allowances = excerpt:%v sourceURL:%v, want both false", excerptAllowed, sourceURLAllowed)
	}
}

func TestWorkProgramGraphContextRunKeyUsesRunMembersNotLatestRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "customer-onboarding"
	queryWorkstream := "workstream:" + workstream
	oldRunAt := time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC)
	newRunAt := oldRunAt.Add(2 * time.Hour)
	oldRunKey := "work-program-run:" + workstream + ":old"

	oldRun, err := store.Client().WorkProgramRun.Create().
		SetKey(oldRunKey).
		SetWorkstreamKey(workstream).
		SetGeneratedAt(oldRunAt).
		SetReadinessState("human_review_required").
		SetReadinessScore(40).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetMemberCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_run").
		SetExternalID(workstream + "|old-run").
		SetRankScore(40).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old run: %v", err)
	}
	newRun, err := store.Client().WorkProgramRun.Create().
		SetKey("work-program-run:" + workstream + ":new").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(newRunAt).
		SetReadinessState("human_review_required").
		SetReadinessScore(70).
		SetAutonomousActionReady(false).
		SetHumanReviewRequired(true).
		SetMemberCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_run").
		SetExternalID(workstream + "|new-run").
		SetRankScore(70).
		Save(ctx)
	if err != nil {
		t.Fatalf("create new run: %v", err)
	}
	oldItem, err := store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:old-run-row").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("ticket:OLD-1").
		SetTitle("Old run ticket row").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketWatch).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(10).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("old-run-row").
		SetRankScore(10).
		SetLastActivityAt(oldRunAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create old item: %v", err)
	}
	newItem, err := store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:newer-graph-row").
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey("ticket:NEW-1").
		SetTitle("Newer graph row after old run").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(99).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("newer-graph-row").
		SetRankScore(99).
		SetLastActivityAt(newRunAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create newer item: %v", err)
	}
	seedWorkProgramGraphContextRunMember(t, ctx, store.Client(), oldRun.ID, oldRun.Key, oldRunAt, workProgramRunMemberTableItems, oldItem.ID, oldItem.Key, oldItem.ExternalKind, oldItem.ExternalID, oldItem.RankScore)
	seedWorkProgramGraphContextRunMember(t, ctx, store.Client(), newRun.ID, newRun.Key, newRunAt, workProgramRunMemberTableItems, newItem.ID, newItem.Key, newItem.ExternalKind, newItem.ExternalID, newItem.RankScore)

	itemLimit := 1
	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	got, err := resolver.WorkProgramGraphContext(ctx, queryWorkstream, &itemLimit, nil, nil, nil, nil, nil, nil, &oldRunKey, nil, nil)
	if err != nil {
		t.Fatalf("workProgramGraphContext by old runKey: %v", err)
	}

	if got.RunKey == nil || *got.RunKey != oldRunKey {
		t.Fatalf("runKey = %#v, want old run key %s", got.RunKey, oldRunKey)
	}
	if got.ScopeMode != "latest_source:explicit_run_key:work_program_run_boundary" {
		t.Fatalf("scopeMode = %q, want explicit run boundary", got.ScopeMode)
	}
	if !strings.Contains(got.LlmTask, "hard run boundary") {
		t.Fatalf("llmTask = %q, want hard run boundary instruction", got.LlmTask)
	}
	if !hasWorkGraphContextBadge(got.Badges, "graph_context:run_boundary") {
		t.Fatalf("badges = %#v, want run boundary badge", got.Badges)
	}
	if got.ItemCount != 1 || len(got.Items) != 1 || got.Items[0].Key != oldItem.Key {
		t.Fatalf("items = %#v, want old run member row %s", got.Items, oldItem.Key)
	}
}

func TestWorkProgramGraphContextHashChangesWithClaimGates(t *testing.T) {
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
	subjectKey := "github:apache/flink-kubernetes-operator#101"
	generatedAt := time.Date(2026, 6, 23, 12, 30, 0, 0, time.UTC)

	action, err := store.Client().WorkAction.Create().
		SetKey("work-action:graph-context:hash").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("graph-context:hash-action").
		SetFreshnessState(workaction.FreshnessStateFresh).
		SetRankScore(70).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	item, err := store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:graph-context:hash").
		SetWorkAction(action).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(subjectKey).
		SetTitle("Hash-sensitive item").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetRiskScore(70).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("graph-context:hash-item").
		SetRankScore(70).
		SetLastActivityAt(generatedAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	_, err = store.Client().WorkProgramRun.Create().
		SetKey("work-program-run:" + workstream + ":graph-context-hash").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(generatedAt).
		SetReadinessState("human_review_required").
		SetReadinessScore(50).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_program_run").
		SetExternalID(workstream + "|graph-context-hash-run").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	first, err := resolver.WorkProgramGraphContext(ctx, queryWorkstream, nil, nil, nil, nil, nil, nil, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("first graph context: %v", err)
	}

	_, err = item.Update().
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDecisionGateReason("validation_required_before_product_claim").
		Save(ctx)
	if err != nil {
		t.Fatalf("update item gate: %v", err)
	}
	second, err := resolver.WorkProgramGraphContext(ctx, queryWorkstream, nil, nil, nil, nil, nil, nil, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("second graph context: %v", err)
	}
	if first.ContextHash == second.ContextHash {
		t.Fatalf("contextHash did not change after claim gate changed: %s", first.ContextHash)
	}
	if second.Items[0].ProductActionAllowed {
		t.Fatalf("updated item still allows product action: %#v", second.Items[0])
	}
}

func workGraphCitationsByRefForTest(rows []*model.WorkGraphCitation) map[string]*model.WorkGraphCitation {
	out := map[string]*model.WorkGraphCitation{}
	for _, row := range rows {
		if row != nil {
			out[row.Ref] = row
		}
	}
	return out
}
