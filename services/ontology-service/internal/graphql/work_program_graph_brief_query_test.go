package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkProgramGraphBriefLoadsCurrentAIArtifact(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	generatedAt := time.Date(2026, 6, 23, 19, 15, 0, 0, time.UTC)
	evidenceRow, err := store.Client().Evidence.Create().
		SetKey("evidence:cubicle-ai:graph-brief").
		SetClaimKind(evidence.ClaimKindGeneratedSummary).
		SetClaimTargetKind("work_insight").
		SetClaimField("details").
		SetLocatorKind("ai_graph_brief").
		SetLocator("context_hash:ctx123").
		SetSourceSpanKey("ctx123").
		SetExcerpt("# Operating Brief\n- Cited generated brief. [context:ctx123]").
		SetProofState(evidence.ProofStateGenerated).
		SetObservedAt(generatedAt).
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief_evidence").
		SetExternalID(workstream + "|ctx123|evidence").
		SetSourceURL("cubicle://graph-brief/ctx123").
		SetSourceVersion("ctx123").
		SetContentHash("brief-evidence-hash").
		SetConfidence(0.72).
		Save(ctx)
	if err != nil {
		t.Fatalf("create graph brief evidence: %v", err)
	}
	briefText := strings.Join([]string{
		"# Operating Brief",
		"## Confirmed Facts",
		"- Current graph brief is cited. [context:ctx123]",
		"## Validation Leads",
		"- Validate generated follow-ups. [guardrail:ctx123]",
		"## What Not To Claim",
		"- Do not claim ETA readiness. [analytics:tpm_forecast_summary]",
	}, "\n")
	insight, err := store.Client().WorkInsight.Create().
		SetKey("work-insight:cubicle-ai:graph-brief").
		SetInsightKind(workinsight.InsightKindAiGraphBrief).
		SetSeverity(workinsight.SeverityInfo).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey(workstream).
		SetTitle("AI graph brief: " + workstream).
		SetDetails(briefText + "\n\nEvaluation:\n{\"passes_smoke_eval\":true}").
		SetRecommendedAction("Review the generated brief.").
		SetModelName("mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16").
		SetModelVersion("answer-hash").
		SetModelMethod("bounded_graph_context_to_cited_brief").
		SetScore(100).
		SetScoreExplanation("smoke_eval=true; unknown_citations=0; uncited_claims=0; forbidden_claims=0").
		SetLatestEvidenceID(evidenceRow.ID).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief").
		SetExternalID(workstream + "|ctx123|ai_graph_brief").
		SetSourceURL("cubicle://graph-brief/ctx123").
		SetSourceVersion("ctx123").
		SetContentHash("answer-hash").
		SetFreshnessState(workinsight.FreshnessStateFresh).
		SetVisibility(workinsight.VisibilityPrivate).
		SetConfidence(0.72).
		SetEventCount(12).
		SetFirstSeenAt(generatedAt).
		SetLastActivityAt(generatedAt).
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create graph brief insight: %v", err)
	}
	snapshot, err := store.Client().WorkProgramBriefSnapshot.Create().
		SetKey("work-program-brief-snapshot:cubicle-ai:graph-brief").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt).
		SetOperatingStatus("attention_required").
		SetDecisionPressure("human_review_required").
		SetForecastState("gated").
		SetPrimaryRisk("source coverage and generated-claim guardrails").
		SetExecutiveSummary("AI graph brief header.").
		SetRecommendedFocus("Review generated validation leads.").
		SetNextCadenceFocus("Re-run after source refresh.").
		SetCapabilityGaps("source_coverage\nmeasurement_labels").
		SetLatestEvidenceID(evidenceRow.ID).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief_snapshot").
		SetExternalID(workstream + "|ctx123|ai_graph_brief_snapshot").
		SetSourceURL("cubicle://graph-brief/ctx123").
		SetConfidence(0.72).
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create graph brief snapshot: %v", err)
	}
	seedWorkProgramRunMember(t, ctx, store.Client(), source, "flink-kubernetes-operator", generatedAt, workProgramRunMemberTableBriefSnapshots, snapshot.ID, snapshot.Key, snapshot.ExternalKind, snapshot.ExternalID, snapshot.RankScore)
	_, err = store.Client().WorkInsight.Create().
		SetKey("work-insight:cubicle-ai:superseded").
		SetInsightKind(workinsight.InsightKindAiGraphBrief).
		SetProducerState(workinsight.ProducerStateSuperseded).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey(workstream).
		SetTitle("Superseded graph brief").
		SetDetails("old").
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief").
		SetExternalID(workstream + "|old|ai_graph_brief").
		SetRankScore(1000).
		SetLastActivityAt(generatedAt.Add(time.Hour)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create superseded graph brief: %v", err)
	}
	genericEvidence, err := store.Client().Evidence.Create().
		SetKey("evidence:cubicle-ai:graph-brief-generic").
		SetClaimKind(evidence.ClaimKindGeneratedSummary).
		SetClaimTargetKind("work_insight").
		SetClaimField("details").
		SetLocatorKind("ai_graph_brief").
		SetLocator("context_hash:ctx456").
		SetSourceSpanKey("ctx456").
		SetExcerpt("# Operating Brief\n- Generic graph brief. [context:ctx456]").
		SetProofState(evidence.ProofStateGenerated).
		SetObservedAt(generatedAt.Add(2 * time.Hour)).
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief_evidence").
		SetExternalID(workstream + "|generic|ctx456|evidence").
		SetSourceURL("cubicle://graph-brief/generic/ctx456").
		SetSourceVersion("ctx456").
		SetContentHash("brief-generic-evidence-hash").
		SetConfidence(0.72).
		Save(ctx)
	if err != nil {
		t.Fatalf("create generic graph brief evidence: %v", err)
	}
	genericInsight, err := store.Client().WorkInsight.Create().
		SetKey("work-insight:cubicle-ai:graph-brief-generic").
		SetInsightKind(workinsight.InsightKindAiGraphBrief).
		SetSeverity(workinsight.SeverityInfo).
		SetProducerState(workinsight.ProducerStateCurrent).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey(workstream).
		SetTitle("AI graph brief generic: " + workstream).
		SetDetails(strings.ReplaceAll(briefText, "Current graph brief", "Generic graph brief") + "\n\nEvaluation:\n{\"passes_smoke_eval\":true}").
		SetRecommendedAction("Review the generated generic graph brief.").
		SetModelName("mlx-community/Qwen3-Coder-30B-A3B-Instruct-bf16").
		SetModelVersion("answer-hash-generic").
		SetModelMethod("bounded_graph_context_to_cited_brief:generic").
		SetScore(100).
		SetScoreExplanation("smoke_eval=true; unknown_citations=0; uncited_claims=0; forbidden_claims=0").
		SetLatestEvidenceID(genericEvidence.ID).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief").
		SetExternalID(workstream + "|generic|ctx456|ai_graph_brief").
		SetSourceURL("cubicle://graph-brief/generic/ctx456").
		SetSourceVersion("ctx456").
		SetContentHash("answer-hash-generic").
		SetFreshnessState(workinsight.FreshnessStateFresh).
		SetVisibility(workinsight.VisibilityPrivate).
		SetConfidence(0.72).
		SetEventCount(12).
		SetFirstSeenAt(generatedAt.Add(2 * time.Hour)).
		SetLastActivityAt(generatedAt.Add(2 * time.Hour)).
		SetRankScore(1000).
		Save(ctx)
	if err != nil {
		t.Fatalf("create generic graph brief insight: %v", err)
	}
	genericSnapshot, err := store.Client().WorkProgramBriefSnapshot.Create().
		SetKey("work-program-brief-snapshot:cubicle-ai:graph-brief-generic").
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(generatedAt.Add(2 * time.Hour)).
		SetOperatingStatus("attention_required").
		SetDecisionPressure("human_review_required").
		SetForecastState("gated").
		SetPrimaryRisk("generic graph guardrails").
		SetExecutiveSummary("AI generic graph brief header.").
		SetRecommendedFocus("Review generated validation leads.").
		SetNextCadenceFocus("Re-run after source refresh.").
		SetLatestEvidenceID(genericEvidence.ID).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief_snapshot").
		SetExternalID(workstream + "|generic|ctx456|ai_graph_brief_snapshot").
		SetSourceURL("cubicle://graph-brief/generic/ctx456").
		SetConfidence(0.72).
		SetRankScore(1000).
		Save(ctx)
	if err != nil {
		t.Fatalf("create generic graph brief snapshot: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	got, err := resolver.WorkProgramGraphBrief(ctx, workstream, &source, nil)
	if err != nil {
		t.Fatalf("workProgramGraphBrief: %v", err)
	}
	if got == nil {
		t.Fatalf("workProgramGraphBrief = nil, want artifact")
	}
	if got.ContextHash != "ctx123" || got.WorkstreamKey != workstream {
		t.Fatalf("graph brief identity = %#v, want ctx123/%s", got, workstream)
	}
	if got.BriefMode != "operating" {
		t.Fatalf("graph brief mode = %q, want operating", got.BriefMode)
	}
	if got.Insight == nil || got.Insight.Key != insight.Key || got.Insight.InsightKind != "ai_graph_brief" {
		t.Fatalf("graph brief insight = %#v, want current ai graph brief", got.Insight)
	}
	if got.Evidence == nil || got.Evidence.ProofState != "generated" || got.Evidence.ClaimKind != "generated_summary" {
		t.Fatalf("graph brief evidence = %#v, want generated-summary evidence", got.Evidence)
	}
	if got.SnapshotKey == nil || *got.SnapshotKey != snapshot.Key {
		t.Fatalf("graph brief snapshot key = %#v, want %s", got.SnapshotKey, snapshot.Key)
	}
	if got.RunKey == nil || !strings.Contains(*got.RunKey, "work-program-run:flink-kubernetes-operator") {
		t.Fatalf("graph brief run key = %#v, want run member key", got.RunKey)
	}
	if strings.Contains(got.BriefMarkdown, "Evaluation:") {
		t.Fatalf("brief markdown leaked evaluation payload: %q", got.BriefMarkdown)
	}
	mode := "generic"
	generic, err := resolver.WorkProgramGraphBrief(ctx, workstream, &source, &mode)
	if err != nil {
		t.Fatalf("generic workProgramGraphBrief: %v", err)
	}
	if generic == nil || generic.BriefMode != "generic" || generic.ContextHash != "ctx456" {
		t.Fatalf("generic graph brief = %#v, want mode generic ctx456", generic)
	}
	if generic.Insight == nil || generic.Insight.Key != genericInsight.Key {
		t.Fatalf("generic graph brief insight = %#v, want %s", generic.Insight, genericInsight.Key)
	}
	if generic.SnapshotKey == nil || *generic.SnapshotKey != genericSnapshot.Key {
		t.Fatalf("generic graph brief snapshot key = %#v, want %s", generic.SnapshotKey, genericSnapshot.Key)
	}
}

func TestWorkProgramGraphBriefKeepsAISnapshotOutOfTPMBrief(t *testing.T) {
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
	_, err = store.Client().WorkProgramBriefSnapshot.Create().
		SetKey("work-program-brief-snapshot:cubicle-ai:only").
		SetWorkstreamKey(workstream).
		SetGeneratedAt(time.Date(2026, 6, 23, 19, 15, 0, 0, time.UTC)).
		SetOperatingStatus("attention_required").
		SetDecisionPressure("human_review_required").
		SetForecastState("gated").
		SetExecutiveSummary("AI graph brief header.").
		SetRecommendedFocus("Review generated validation leads.").
		SetNextCadenceFocus("Re-run after source refresh.").
		SetSourceSystem("cubicle_ai").
		SetSourceInstance(source).
		SetExternalKind("ai_graph_brief_snapshot").
		SetExternalID(workstream + "|ctx123|ai_graph_brief_snapshot").
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create AI graph brief snapshot: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	workstreamArg := "workstream:" + workstream
	got, err := resolver.latestWorkProgramBriefSnapshotData(ctx, &source, &workstreamArg)
	if err != nil {
		t.Fatalf("latest TPM brief snapshot: %v", err)
	}
	if got != nil {
		t.Fatalf("latest TPM brief snapshot loaded AI graph brief snapshot: %#v", got)
	}
}
