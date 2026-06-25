package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkDecisionTargetReadinessUsesLatestRunCoverageAndClampsProductAction(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	oldRunAt := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)
	newRunAt := oldRunAt.Add(time.Hour)

	generatedEvidence := seedDecisionTargetEvidence(t, ctx, store.Client(), source, "generated", "cubicle_analytics", "tpm_generated_evidence")
	sourceEvidence := seedDecisionTargetEvidence(t, ctx, store.Client(), source, "source", "github", "pull_request")

	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:old-ready",
		source:                  source,
		externalID:              "old-ready",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratified_summary",
		modelName:               "coverage_guardrail",
		coverageStratum:         "coverage=observed",
		readyForProductAction:   true,
		productActionGateState:  "passed",
		productActionGateReason: "old run should not decide current readiness",
		evaluatedAt:             oldRunAt,
		rankScore:               100,
		latestEvidence:          sourceEvidence,
	})
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:new-coverage",
		source:                  source,
		externalID:              "new-coverage",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratified_summary",
		modelName:               "coverage_guardrail",
		coverageStratum:         "not_testable_single_stratum",
		readyForProductAction:   false,
		productActionGateState:  "validation_gated",
		productActionGateReason: "coverage confounding cannot be tested",
		note:                    "coverage confounding cannot be tested from this sample",
		evaluatedAt:             newRunAt,
		rankScore:               100,
		latestEvidence:          generatedEvidence,
	})
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:new-rf",
		source:                  source,
		externalID:              "new-rf",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratum",
		modelName:               "random_forest_classifier_oof",
		coverageStratum:         "coverage=observed;detail=observed",
		readyForProductAction:   true,
		productActionGateState:  "passed",
		productActionGateReason: "producer claims ready, but generated evidence and coverage gate must clamp it",
		precisionAt10pct:        ptrFloat(0.3793),
		liftAt10pct:             ptrFloat(0.2213),
		rocAuc:                  ptrFloat(0.696),
		averagePrecision:        ptrFloat(0.3654),
		evaluatedAt:             newRunAt,
		rankScore:               80,
		latestEvidence:          generatedEvidence,
	})
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:other-source",
		source:                  "other-source",
		externalID:              "other-source",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratified_summary",
		modelName:               "coverage_guardrail",
		coverageStratum:         "coverage=observed",
		readyForProductAction:   true,
		productActionGateState:  "passed",
		productActionGateReason: "wrong source should not leak",
		evaluatedAt:             newRunAt,
		rankScore:               100,
		latestEvidence:          sourceEvidence,
	})

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	readiness, err := resolver.WorkDecisionTargetReadiness(ctx, nil, &sourceArg)
	if err != nil {
		t.Fatalf("decision-target readiness: %v", err)
	}
	if readiness.SourceInstance == nil || *readiness.SourceInstance != source {
		t.Fatalf("source = %#v, want %s", readiness.SourceInstance, source)
	}
	if readiness.EvaluatedAt == nil || *readiness.EvaluatedAt != newRunAt.Format(time.RFC3339Nano) {
		t.Fatalf("evaluatedAt = %#v, want latest run", readiness.EvaluatedAt)
	}
	if readiness.EvaluationState != "validation_gated" || readiness.CoverageGateState != "validation_gated" || readiness.ProductActionReady {
		t.Fatalf("readiness = state:%s coverage:%s product:%v, want validation gated false", readiness.EvaluationState, readiness.CoverageGateState, readiness.ProductActionReady)
	}
	if readiness.EvaluationCount != 2 || readiness.ProductReadyEvaluationCount != 0 || readiness.ValidationGatedEvaluationCount != 2 {
		t.Fatalf("counts = eval:%d product:%d gated:%d, want 2/0/2", readiness.EvaluationCount, readiness.ProductReadyEvaluationCount, readiness.ValidationGatedEvaluationCount)
	}
	if !readiness.GeneratedEvidenceOnly {
		t.Fatalf("generatedEvidenceOnly=false, want true for current run")
	}
	if readiness.CoverageStratum == nil || *readiness.CoverageStratum != "not_testable_single_stratum" {
		t.Fatalf("coverage stratum = %#v", readiness.CoverageStratum)
	}
	if readiness.RecommendedFocus == nil || !strings.Contains(*readiness.RecommendedFocus, "coverage confounding") {
		t.Fatalf("recommended focus = %#v, want coverage gate reason", readiness.RecommendedFocus)
	}
	if len(readiness.Evaluations) != 2 {
		t.Fatalf("nested evaluations = %d, want 2 current-run rows", len(readiness.Evaluations))
	}
	if readiness.Evaluations[0].ModelName != "coverage_guardrail" {
		t.Fatalf("first nested row = %s, want coverage guardrail first", readiness.Evaluations[0].ModelName)
	}
	for _, evaluation := range readiness.Evaluations {
		if evaluation.ProductActionAllowed {
			t.Fatalf("evaluation %s productActionAllowed=true, want resolver clamp", evaluation.Key)
		}
		if evaluation.EvaluationKind == "source_event_as_of_coverage_stratum" && evaluation.DecisionClaimUse != "ranking_validation_only" {
			t.Fatalf("decision claim use = %s, want ranking_validation_only", evaluation.DecisionClaimUse)
		}
	}

	rows, err := resolver.WorkDecisionTargetEvaluations(ctx, nil, nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("decision-target rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("query returned %d rows, want latest-run source-scoped rows only", len(rows))
	}
}

func TestWorkDecisionTargetReadinessDoesNotTrustReadyRowsWithoutCoverageGuardrail(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	runAt := time.Date(2026, 6, 22, 9, 0, 0, 0, time.UTC)
	sourceEvidence := seedDecisionTargetEvidence(t, ctx, store.Client(), source, "source", "github", "pull_request")
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:no-coverage-ready",
		source:                  source,
		externalID:              "no-coverage-ready",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_chronological_holdout",
		modelName:               "random_forest_classifier",
		readyForProductAction:   true,
		productActionGateState:  "passed",
		productActionGateReason: "producer claims ready without coverage guardrail",
		precisionAt10pct:        ptrFloat(0.9),
		evaluatedAt:             runAt,
		rankScore:               100,
		latestEvidence:          sourceEvidence,
	})

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	readiness, err := resolver.WorkDecisionTargetReadiness(ctx, nil, &sourceArg)
	if err != nil {
		t.Fatalf("decision-target readiness: %v", err)
	}
	if readiness.EvaluationState != "missing_coverage_guardrail" || readiness.CoverageGateState != "missing_coverage_guardrail" || readiness.ProductActionReady {
		t.Fatalf("readiness = state:%s coverage:%s product:%v, want missing coverage gate false", readiness.EvaluationState, readiness.CoverageGateState, readiness.ProductActionReady)
	}
	if readiness.ProductReadyEvaluationCount != 0 || len(readiness.Evaluations) != 1 || readiness.Evaluations[0].ProductActionAllowed {
		t.Fatalf("ready row was trusted despite missing coverage guardrail: readiness=%#v row=%#v", readiness, readiness.Evaluations)
	}
}

func TestWorkDecisionTargetReadinessRequiresEvidenceSourceMatch(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	runAt := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	mismatchedEvidence := seedDecisionTargetEvidence(t, ctx, store.Client(), "other-source", "mismatched-source", "github", "pull_request")
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:mismatched-coverage",
		source:                  source,
		externalID:              "mismatched-coverage",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratified_summary",
		modelName:               "coverage_guardrail",
		coverageStratum:         "coverage=observed",
		readyForProductAction:   true,
		productActionGateState:  "passed",
		productActionGateReason: "coverage gate passed",
		evaluatedAt:             runAt,
		rankScore:               100,
		latestEvidence:          mismatchedEvidence,
	})
	seedDecisionTargetEvaluation(t, ctx, store.Client(), decisionTargetEvaluationSeed{
		key:                     "work-decision-target-evaluation:mismatched-rf",
		source:                  source,
		externalID:              "mismatched-rf",
		targetKind:              "abandonment_risk",
		evaluationKind:          "source_event_as_of_coverage_stratum",
		modelName:               "random_forest_classifier_oof",
		coverageStratum:         "coverage=observed",
		readyForProductAction:   true,
		productActionGateState:  "passed",
		productActionGateReason: "producer claims ready with wrong-source evidence",
		precisionAt10pct:        ptrFloat(0.9),
		evaluatedAt:             runAt,
		rankScore:               90,
		latestEvidence:          mismatchedEvidence,
	})

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	sourceArg := source
	readiness, err := resolver.WorkDecisionTargetReadiness(ctx, nil, &sourceArg)
	if err != nil {
		t.Fatalf("decision-target readiness: %v", err)
	}
	if readiness.CoverageGateState != "passed" {
		t.Fatalf("coverage gate = %s, want passed to isolate evidence-source clamp", readiness.CoverageGateState)
	}
	if !readiness.GeneratedEvidenceOnly || readiness.ProductActionReady || readiness.ProductReadyEvaluationCount != 0 {
		t.Fatalf("readiness = generatedOnly:%v product:%v readyCount:%d, want wrong-source evidence treated as generated/evidence-only", readiness.GeneratedEvidenceOnly, readiness.ProductActionReady, readiness.ProductReadyEvaluationCount)
	}
	for _, evaluation := range readiness.Evaluations {
		if evaluation.ProductActionAllowed || evaluation.DecisionClaimGateReason != "generated_evidence_only" {
			t.Fatalf("evaluation %s allowed=%v gate=%s, want generated_evidence_only clamp", evaluation.Key, evaluation.ProductActionAllowed, evaluation.DecisionClaimGateReason)
		}
	}
}

type decisionTargetEvaluationSeed struct {
	key                     string
	source                  string
	externalID              string
	targetKind              string
	evaluationKind          string
	modelName               string
	coverageStratum         string
	readyForProductAction   bool
	productActionGateState  string
	productActionGateReason string
	note                    string
	precisionAt10pct        *float64
	liftAt10pct             *float64
	rocAuc                  *float64
	averagePrecision        *float64
	evaluatedAt             time.Time
	rankScore               float64
	latestEvidence          *genent.Evidence
}

func seedDecisionTargetEvaluation(t *testing.T, ctx context.Context, client *genent.Client, seed decisionTargetEvaluationSeed) *genent.WorkDecisionTargetEvaluation {
	t.Helper()
	create := client.WorkDecisionTargetEvaluation.Create().
		SetKey(seed.key).
		SetTargetKind(seed.targetKind).
		SetEvaluationKind(seed.evaluationKind).
		SetModelName(seed.modelName).
		SetCoverageStratum(seed.coverageStratum).
		SetReadyForProductAction(seed.readyForProductAction).
		SetProductActionGateState(seed.productActionGateState).
		SetProductActionGateReason(seed.productActionGateReason).
		SetNote(seed.note).
		SetEvaluatedAt(seed.evaluatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(seed.source).
		SetExternalKind("tpm_decision_target_evaluation").
		SetExternalID(seed.externalID).
		SetEvidenceCount(1).
		SetRankScore(seed.rankScore)
	if seed.precisionAt10pct != nil {
		create.SetPrecisionAt10pct(*seed.precisionAt10pct)
	}
	if seed.liftAt10pct != nil {
		create.SetLiftAt10pct(*seed.liftAt10pct)
	}
	if seed.rocAuc != nil {
		create.SetRocAuc(*seed.rocAuc)
	}
	if seed.averagePrecision != nil {
		create.SetAveragePrecision(*seed.averagePrecision)
	}
	if seed.latestEvidence != nil {
		create.SetLatestEvidence(seed.latestEvidence)
	}
	row, err := create.Save(ctx)
	if err != nil {
		t.Fatalf("create decision-target evaluation %s: %v", seed.key, err)
	}
	return row
}

func seedDecisionTargetEvidence(t *testing.T, ctx context.Context, client *genent.Client, source string, suffix string, sourceSystem string, externalKind string) *genent.Evidence {
	t.Helper()
	row, err := client.Evidence.Create().
		SetKey("evidence:decision-target:" + suffix).
		SetSourceSystem(sourceSystem).
		SetSourceInstance(source).
		SetExternalKind(externalKind).
		SetExternalID("decision-target:" + suffix).
		SetClaimTargetKind("work_decision_target_evaluation").
		SetClaimField("decision_target_validation").
		SetLocatorKind("row").
		SetLocator("decision-target:" + suffix).
		SetExcerpt("Decision-target validation evidence " + suffix).
		Save(ctx)
	if err != nil {
		t.Fatalf("create decision-target evidence %s: %v", suffix, err)
	}
	return row
}

func ptrFloat(value float64) *float64 {
	return &value
}
