package graphql

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func TestWorkActionsFiltersByOwnerKey(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 21, 12, 20, 0, 0, time.UTC)
	_, err = store.Client().WorkAction.Create().
		SetKey("tpm-action:owner-a").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("repo/example#1").
		SetOwnerKey("github:owner-a").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:owner-a").
		SetLastActivityAt(now).
		SetRankScore(90).
		Save(ctx)
	if err != nil {
		t.Fatalf("create owner-a action: %v", err)
	}
	_, err = store.Client().WorkAction.Create().
		SetKey("tpm-action:owner-b").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("repo/example#2").
		SetOwnerKey("github:owner-b").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:owner-b").
		SetLastActivityAt(now.Add(time.Minute)).
		SetRankScore(100).
		Save(ctx)
	if err != nil {
		t.Fatalf("create owner-b action: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	source := "fixture-source"
	owner := "github:owner-a"
	rows, err := resolver.WorkActions(ctx, &limit, nil, nil, &owner, &source)
	if err != nil {
		t.Fatalf("work actions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("work actions count = %d, want 1: %#v", len(rows), rows)
	}
	if rows[0].Key != "tpm-action:owner-a" || rows[0].OwnerKey == nil || *rows[0].OwnerKey != owner {
		t.Fatalf("workActions owner filter leaked wrong row: %#v", rows[0])
	}
}

func TestWorkActionsClampProductActionWhenResponsibilityNeedsValidation(t *testing.T) {
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
	generatedAt := time.Date(2026, 6, 23, 10, 15, 0, 0, time.UTC)
	action, _ := seedCandidateWorkActionResponsibility(t, ctx, store.Client(), source, workstream, generatedAt)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	sourceArg := source
	rows, err := resolver.WorkActions(ctx, &limit, nil, nil, nil, &sourceArg)
	if err != nil {
		t.Fatalf("work actions: %v", err)
	}
	if len(rows) != 1 || rows[0].Key != action.Key {
		t.Fatalf("work actions = %#v, want candidate responsibility action", rows)
	}
	row := rows[0]
	if row.ProductActionAllowed || row.ClaimUse != "responsibility_validation" {
		t.Fatalf("action claim = product:%v use:%s, want responsibility validation gate", row.ProductActionAllowed, row.ClaimUse)
	}
	if !strings.Contains(row.ClaimGateReason, "generated owner hint requires validation") {
		t.Fatalf("claim gate reason = %#v, want responsibility validation reason", row.ClaimGateReason)
	}
	for _, badge := range row.Badges {
		if badge.Key == "decision:product_action" {
			t.Fatalf("responsibility-gated action still exposed product-action badge: %#v", row.Badges)
		}
	}
}

func TestWorkActionETAClaimRequiresForecastReadinessContext(t *testing.T) {
	row := &genent.WorkAction{
		Key:            "tpm-action:eta-stale",
		ActionType:     workaction.ActionTypeDecisionOrOwnerFollowup,
		ActionState:    workaction.ActionStateOpen,
		DecisionState:  workaction.DecisionStateProductAction,
		DecisionReason: "eta_forecast_ready=true from stale action row",
		SubjectKind:    workaction.SubjectKindUnknown,
		SubjectKey:     "repo/example#1",
		DueBucket:      workaction.DueBucketNow,
	}

	generic := workActionModel(row)
	if generic.EtaClaimAllowed || generic.ProductActionAllowed || generic.ClaimUse == "eta_candidate_product_action" {
		t.Fatalf("generic action claim = eta:%v product:%v use:%s, want ETA/product claim clamped without readiness context", generic.EtaClaimAllowed, generic.ProductActionAllowed, generic.ClaimUse)
	}
	if generic.ClaimGateReason != "eta_forecast_readiness_not_verified" {
		t.Fatalf("generic claim gate = %q, want readiness-not-verified gate", generic.ClaimGateReason)
	}

	ready := workActionModelWithForecastReadiness(row, &model.WorkForecastReadiness{
		EtaForecastReady: true,
		ReadinessState:   "ready",
	})
	if !ready.EtaClaimAllowed || !ready.ProductActionAllowed || ready.ClaimUse != "eta_candidate_product_action" {
		t.Fatalf("ready action claim = eta:%v product:%v use:%s, want ETA product action allowed with readiness context", ready.EtaClaimAllowed, ready.ProductActionAllowed, ready.ClaimUse)
	}

	gated := workActionModelWithForecastReadiness(row, &model.WorkForecastReadiness{
		EtaForecastReady:                 false,
		ReadinessState:                   "gated",
		EtaReadinessBlockingReason:       optionalString("kfold_model_does_not_beat_baseline"),
		ReadinessReason:                  optionalString("ETA forecast is gated"),
		TypedEvaluationCount:             5,
		TransitionCandidateCount:         1,
		ObservedSnapshotTimeCount:        2,
		GatedForecastLeadCount:           0,
		TerminalTransitionCandidateCount: 0,
	})
	if gated.EtaClaimAllowed || gated.ProductActionAllowed || gated.ClaimUse == "eta_candidate_product_action" {
		t.Fatalf("gated action claim = eta:%v product:%v use:%s, want ETA/product claim clamped by gated readiness", gated.EtaClaimAllowed, gated.ProductActionAllowed, gated.ClaimUse)
	}
	if gated.ClaimGateReason != "eta_forecast_readiness_gated:kfold_model_does_not_beat_baseline" {
		t.Fatalf("gated claim gate = %q, want typed readiness blocker", gated.ClaimGateReason)
	}
	for _, badge := range gated.Badges {
		if badge.Key == "decision:product_action" {
			t.Fatalf("gated ETA action still exposed product-action badge: %#v", gated.Badges)
		}
	}
}

func TestWorkActionForecastRiskProductActionDoesNotEnableETAClaim(t *testing.T) {
	row := &genent.WorkAction{
		Key:            "tpm-action:risk-triage-owner",
		ActionType:     workaction.ActionTypeDecisionOrOwnerFollowup,
		ActionState:    workaction.ActionStateOpen,
		DecisionState:  workaction.DecisionStateProductAction,
		DecisionReason: "risk-triage backtest supports attention ordering; eta_forecast_ready=false, so allowed use is owner/status follow-up only, not ETA",
		SubjectKind:    workaction.SubjectKindPullRequest,
		SubjectKey:     "repo/example#42",
		OwnerKey:       "github:owner-a",
		DueBucket:      workaction.DueBucketNow,
		Edges: genent.WorkActionEdges{
			SourceInsights: []*genent.WorkInsight{
				{InsightKind: workinsight.InsightKindForecastRisk},
			},
		},
	}

	actionModel := workActionModelWithForecastReadiness(row, &model.WorkForecastReadiness{
		EtaForecastReady: false,
		ReadinessState:   "gated",
	})

	if !actionModel.ProductActionAllowed || actionModel.EtaClaimAllowed {
		t.Fatalf("risk triage action claim = product:%v eta:%v, want product follow-up without ETA", actionModel.ProductActionAllowed, actionModel.EtaClaimAllowed)
	}
	if actionModel.ClaimUse != "risk_triage_owner_followup" {
		t.Fatalf("claim use = %q, want risk_triage_owner_followup", actionModel.ClaimUse)
	}
	if actionModel.RecommendedAction == nil || !strings.Contains(*actionModel.RecommendedAction, "not an ETA commitment") {
		t.Fatalf("recommended action = %v, want non-ETA owner follow-up wording", actionModel.RecommendedAction)
	}
}

func TestWorkActionSummaryCountsOnlyEffectiveProductActions(t *testing.T) {
	staleETAAction := &genent.WorkAction{
		Key:            "tpm-action:eta-stale",
		ActionType:     workaction.ActionTypeDecisionOrOwnerFollowup,
		ActionState:    workaction.ActionStateOpen,
		DecisionState:  workaction.DecisionStateProductAction,
		DecisionReason: "eta_forecast_ready=true from stale action row",
		SubjectKind:    workaction.SubjectKindUnknown,
		SubjectKey:     "repo/example#1",
		OwnerKey:       "github:owner-a",
		OwnerSource:    "fixture",
		DueBucket:      workaction.DueBucketNow,
		RankScore:      100,
	}
	productAction := &genent.WorkAction{
		Key:            "tpm-action:product",
		ActionType:     workaction.ActionTypeValidateSignal,
		ActionState:    workaction.ActionStateOpen,
		DecisionState:  workaction.DecisionStateProductAction,
		DecisionReason: "human-reviewed product action",
		SubjectKind:    workaction.SubjectKindUnknown,
		SubjectKey:     "repo/example#2",
		OwnerKey:       "github:owner-a",
		OwnerSource:    "fixture",
		DueBucket:      workaction.DueBucketThisWeek,
		RankScore:      90,
	}

	source := "fixture-source"
	summary := workActionSummaryModel("open", &source, []*genent.WorkAction{staleETAAction, productAction})
	if summary.TotalCount != 2 {
		t.Fatalf("summary total = %d, want 2", summary.TotalCount)
	}
	if summary.ProductActionCount != 1 {
		t.Fatalf("summary productActionCount = %d, want only the effective product action", summary.ProductActionCount)
	}
	if len(summary.OwnerRollups) != 1 || summary.OwnerRollups[0].ProductActionCount != 1 {
		t.Fatalf("owner rollups = %#v, want one effective product action", summary.OwnerRollups)
	}
	if len(summary.TopActions) < 2 {
		t.Fatalf("top actions = %d, want both rows", len(summary.TopActions))
	}
	if summary.TopActions[0].Key != staleETAAction.Key {
		t.Fatalf("top action ordering changed: got %s, want stale ETA action first by rank", summary.TopActions[0].Key)
	}
	if summary.TopActions[0].ProductActionAllowed || summary.TopActions[0].EtaClaimAllowed {
		t.Fatalf("stale ETA top action claim = product:%v eta:%v, want clamped", summary.TopActions[0].ProductActionAllowed, summary.TopActions[0].EtaClaimAllowed)
	}
	for _, badge := range summary.TopActions[0].Badges {
		if badge.Key == "decision:product_action" {
			t.Fatalf("stale ETA top action still exposed product-action badge: %#v", summary.TopActions[0].Badges)
		}
	}
	if !summary.TopActions[1].ProductActionAllowed {
		t.Fatalf("non-ETA product action was incorrectly clamped: %#v", summary.TopActions[1])
	}
}
