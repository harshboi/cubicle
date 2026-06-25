package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkProgramItemsClampProductActionWhenLinkedResponsibilityNeedsValidation(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 23, 10, 20, 0, 0, time.UTC)
	action, _ := seedCandidateWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)
	_, err = store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:responsibility-gated").
		SetWorkActionID(action.ID).
		SetWorkstreamKey(workstream).
		SetSubjectKind(workprogramitem.SubjectKindUnknown).
		SetSubjectKey(action.SubjectKey).
		SetTitle("Responsibility gated item").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDecisionGateReason("local product action row must still honor responsibility gate").
		SetDueBucket(workprogramitem.DueBucketNow).
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_program_item").
		SetExternalID("work-program-item:responsibility-gated").
		SetRankScore(100).
		SetRiskScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create program item: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	sourceArg := source
	rows, err := resolver.WorkProgramItems(ctx, &limit, nil, nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("work program items: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("program item count = %d, want 1: %#v", len(rows), rows)
	}
	row := rows[0]
	if row.ProductActionAllowed || row.ClaimUse != "responsibility_validation" {
		t.Fatalf("program item claim = product:%v use:%s, want responsibility validation gate", row.ProductActionAllowed, row.ClaimUse)
	}
	if !strings.Contains(row.ClaimGateReason, "generated owner hint requires validation") {
		t.Fatalf("program item gate reason = %#v, want responsibility validation reason", row.ClaimGateReason)
	}
	if row.Action == nil {
		t.Fatalf("program item action missing")
	}
	if row.Action.ProductActionAllowed || row.Action.ClaimUse != "responsibility_validation" {
		t.Fatalf("nested action claim = product:%v use:%s, want responsibility validation gate", row.Action.ProductActionAllowed, row.Action.ClaimUse)
	}
	for _, badge := range row.Action.Badges {
		if badge.Key == "decision:product_action" {
			t.Fatalf("nested responsibility-gated action still exposed product-action badge: %#v", row.Action.Badges)
		}
	}
}

func TestWorkProgramItemSummaryCountsOnlyEffectiveProductActions(t *testing.T) {
	staleETAItem := &genent.WorkProgramItem{
		Key:                 "work-program-item:eta-stale",
		WorkstreamKey:       "flink-kubernetes-operator",
		SubjectKind:         workprogramitem.SubjectKindUnknown,
		SubjectKey:          "subject:eta-stale",
		Title:               "ETA stale item",
		ProgramStatus:       workprogramitem.ProgramStatusNeedsDecision,
		TpmBucket:           workprogramitem.TpmBucketRisk,
		DecisionState:       workprogramitem.DecisionStateProductAction,
		DecisionGateReason:  "eta_forecast_ready=true from stale program item",
		DueBucket:           workprogramitem.DueBucketNow,
		FreshnessState:      workprogramitem.FreshnessStateFresh,
		SourceCoverageState: "observed:authenticated_api_current_observation",
		RiskScore:           100,
		RankScore:           100,
	}
	productItem := &genent.WorkProgramItem{
		Key:                 "work-program-item:product",
		WorkstreamKey:       "flink-kubernetes-operator",
		SubjectKind:         workprogramitem.SubjectKindUnknown,
		SubjectKey:          "subject:product",
		Title:               "Product item",
		ProgramStatus:       workprogramitem.ProgramStatusNeedsDecision,
		TpmBucket:           workprogramitem.TpmBucketRisk,
		DecisionState:       workprogramitem.DecisionStateProductAction,
		DecisionGateReason:  "human-reviewed product action",
		DueBucket:           workprogramitem.DueBucketThisWeek,
		FreshnessState:      workprogramitem.FreshnessStateFresh,
		SourceCoverageState: "observed:authenticated_api_current_observation",
		RiskScore:           90,
		RankScore:           90,
	}
	rows := []*genent.WorkProgramItem{staleETAItem, productItem}
	source := "fixture-source"
	workstream := "workstream:flink-kubernetes-operator"
	gatedReadiness := &model.WorkForecastReadiness{
		EtaForecastReady:           false,
		ReadinessState:             "gated",
		TypedEvaluationCount:       3,
		EtaReadinessBlockingReason: optionalString("kfold_model_does_not_beat_baseline"),
	}

	generic := workProgramItemModel(staleETAItem)
	if generic.ProductActionAllowed || generic.EtaClaimAllowed || generic.ClaimUse == "eta_candidate_product_action" {
		t.Fatalf("generic program item claim = product:%v eta:%v use:%s, want ETA/product claim clamped", generic.ProductActionAllowed, generic.EtaClaimAllowed, generic.ClaimUse)
	}
	if generic.ClaimGateReason != "eta_forecast_readiness_not_verified" {
		t.Fatalf("generic program item gate = %q, want readiness-not-verified gate", generic.ClaimGateReason)
	}

	summary := workProgramSummaryModel(&source, &workstream, rows, workProgramExternalSignals{}, gatedReadiness)
	if summary.TotalCount != 2 {
		t.Fatalf("summary total = %d, want 2", summary.TotalCount)
	}
	if summary.ProductActionCount != 1 {
		t.Fatalf("summary productActionCount = %d, want only the effective product action", summary.ProductActionCount)
	}
	if len(summary.TopItems) < 2 {
		t.Fatalf("top items = %d, want both rows", len(summary.TopItems))
	}
	if summary.TopItems[0].ProductActionAllowed || summary.TopItems[0].EtaClaimAllowed {
		t.Fatalf("stale ETA top item claim = product:%v eta:%v, want clamped", summary.TopItems[0].ProductActionAllowed, summary.TopItems[0].EtaClaimAllowed)
	}
	if summary.TopItems[0].ClaimGateReason != "eta_forecast_readiness_gated:kfold_model_does_not_beat_baseline" {
		t.Fatalf("stale ETA top item gate = %q, want typed readiness blocker", summary.TopItems[0].ClaimGateReason)
	}
	if !summary.TopItems[1].ProductActionAllowed {
		t.Fatalf("non-ETA product item was incorrectly clamped: %#v", summary.TopItems[1])
	}

	readySummary := workProgramSummaryModel(&source, &workstream, rows, workProgramExternalSignals{}, &model.WorkForecastReadiness{
		EtaForecastReady:     true,
		ReadinessState:       "ready",
		TypedEvaluationCount: 3,
	})
	if readySummary.ProductActionCount != 2 {
		t.Fatalf("ready summary productActionCount = %d, want both product items", readySummary.ProductActionCount)
	}
	if !readySummary.TopItems[0].ProductActionAllowed || !readySummary.TopItems[0].EtaClaimAllowed {
		t.Fatalf("ready ETA top item claim = product:%v eta:%v, want allowed", readySummary.TopItems[0].ProductActionAllowed, readySummary.TopItems[0].EtaClaimAllowed)
	}
}
