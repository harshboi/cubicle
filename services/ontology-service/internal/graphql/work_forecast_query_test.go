package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workforecastevaluation"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkForecastEvaluationRowsKeepLatestRunBoundary(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	runAt := time.Date(2026, 6, 21, 7, 0, 0, 0, time.UTC)
	strayFoldAt := runAt.Add(time.Hour)
	otherSourceAt := runAt.Add(2 * time.Hour)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("median_cycle_baseline").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("median_cycle_baseline").
		SetBaselineSampleCount(60).
		SetOpenCandidateCount(20).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("summary gate owns readiness").
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary:2026-06-21T07:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:fold-same-run").
		SetEvaluationKind(workforecastevaluation.EvaluationKindKfold).
		SetModelName("median_cycle_baseline").
		SetFold(1).
		SetMaeDays(8.7).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("kfold:median_cycle_baseline:1:2026-06-21T07:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:stray-newer-fold").
		SetEvaluationKind(workforecastevaluation.EvaluationKindKfold).
		SetModelName("random_forest_regressor").
		SetFold(1).
		SetMaeDays(3.1).
		SetReadyForEta(true).
		SetReadinessState(workforecastevaluation.ReadinessStateReady).
		SetEvaluatedAt(strayFoldAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("kfold:random_forest_regressor:1:2026-06-21T08:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:other-source-summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("random_forest_regressor").
		SetReadyForEta(true).
		SetReadinessState(workforecastevaluation.ReadinessStateReady).
		SetEvaluatedAt(otherSourceAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary:2026-06-21T09:00:00Z").
		SaveX(ctx)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	rows, err := resolver.workForecastEvaluationRows(ctx, &source)
	if err != nil {
		t.Fatalf("forecast rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("forecast rows len = %d, want summary plus same-run fold", len(rows))
	}
	for _, row := range rows {
		if row.SourceInstance != source {
			t.Fatalf("row from wrong source included: %#v", row)
		}
		if !row.EvaluatedAt.Equal(runAt) {
			t.Fatalf("row from wrong evaluation run included: kind=%s model=%s evaluatedAt=%s", row.EvaluationKind, row.ModelName, row.EvaluatedAt)
		}
	}
}

func TestForecastReadinessSummaryGatePreventsETAOverclaim(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	runAt := time.Date(2026, 6, 21, 7, 0, 0, 0, time.UTC)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:gated-summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("median_cycle_baseline").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("median_cycle_baseline").
		SetBaselineSampleCount(60).
		SetOpenCandidateCount(20).
		SetObservedSnapshotTimeCount(1).
		SetTransitionCandidateCount(0).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("ETA forecast is gated: primary blocker kfold_model_does_not_beat_baseline; do not present ETA commitments").
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary:2026-06-21T07:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:ready-fold").
		SetEvaluationKind(workforecastevaluation.EvaluationKindKfold).
		SetModelName("random_forest_regressor").
		SetFold(1).
		SetMaeDays(3.1).
		SetReadyForEta(true).
		SetReadinessState(workforecastevaluation.ReadinessStateReady).
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("kfold:random_forest_regressor:1:2026-06-21T07:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:baseline-fold").
		SetEvaluationKind(workforecastevaluation.EvaluationKindKfold).
		SetModelName("median_cycle_baseline").
		SetFold(1).
		SetMaeDays(7.2).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("kfold:median_cycle_baseline:1:2026-06-21T07:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:chronological").
		SetEvaluationKind(workforecastevaluation.EvaluationKindChronologicalHoldout).
		SetModelName("gradient_boosting_absolute_error").
		SetFold(1).
		SetMaeDays(4.4).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("chronological_holdout:gradient_boosting_absolute_error:1:2026-06-21T07:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:source-event-kfold").
		SetEvaluationKind(workforecastevaluation.EvaluationKind("source_event_as_of_kfold")).
		SetModelName("author_history_median_cycle").
		SetFold(1).
		SetMaeDays(4.2).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("source_event_as_of_kfold:author_history_median_cycle:1:2026-06-21T07:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:source-event-chronological").
		SetEvaluationKind(workforecastevaluation.EvaluationKind("source_event_as_of_chronological_holdout")).
		SetModelName("author_history_median_cycle").
		SetFold(1).
		SetMaeDays(5.6).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("source_event_as_of_chronological_holdout:author_history_median_cycle:1:2026-06-21T07:00:00Z").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:survival").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSurvivalTimeToMerge).
		SetModelName("kaplan_meier_baseline").
		SetFold(1).
		SetMaeDays(9.4).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetEvaluatedAt(runAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("survival_time_to_merge:kaplan_meier_baseline:1:2026-06-21T07:00:00Z").
		SaveX(ctx)

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	readiness, err := resolver.forecastReadinessModel(ctx, &source, "open")
	if err != nil {
		t.Fatalf("forecast readiness: %v", err)
	}
	if readiness.EtaForecastReady {
		t.Fatalf("etaForecastReady = true, want summary gate to keep it false")
	}
	if readiness.ReadinessState != "gated" {
		t.Fatalf("readinessState = %q, want gated", readiness.ReadinessState)
	}
	if readiness.BestBacktestModel == nil || *readiness.BestBacktestModel != "median_cycle_baseline" {
		t.Fatalf("bestBacktestModel = %#v, want summary best model", readiness.BestBacktestModel)
	}
	if readiness.EtaReadinessBlockingReason == nil || *readiness.EtaReadinessBlockingReason != "kfold_model_does_not_beat_baseline" {
		t.Fatalf("etaReadinessBlockingReason = %#v, want primary blocker", readiness.EtaReadinessBlockingReason)
	}
	if readiness.BestKfoldModel == nil || *readiness.BestKfoldModel != "random_forest_regressor" || readiness.BestKfoldMaeDays == nil || *readiness.BestKfoldMaeDays != 3.1 {
		t.Fatalf("best kfold = (%#v, %#v), want random_forest_regressor 3.1", readiness.BestKfoldModel, readiness.BestKfoldMaeDays)
	}
	if readiness.BestChronologicalHoldoutModel == nil || *readiness.BestChronologicalHoldoutModel != "gradient_boosting_absolute_error" || readiness.BestChronologicalHoldoutMaeDays == nil || *readiness.BestChronologicalHoldoutMaeDays != 4.4 {
		t.Fatalf("best chronological = (%#v, %#v), want gradient_boosting_absolute_error 4.4", readiness.BestChronologicalHoldoutModel, readiness.BestChronologicalHoldoutMaeDays)
	}
	if readiness.SourceEventAsOfKfoldModel == nil || *readiness.SourceEventAsOfKfoldModel != "author_history_median_cycle" || readiness.SourceEventAsOfKfoldMaeDays == nil || *readiness.SourceEventAsOfKfoldMaeDays != 4.2 {
		t.Fatalf("source-event kfold = (%#v, %#v), want author_history_median_cycle 4.2", readiness.SourceEventAsOfKfoldModel, readiness.SourceEventAsOfKfoldMaeDays)
	}
	if readiness.SourceEventAsOfChronologicalHoldoutModel == nil || *readiness.SourceEventAsOfChronologicalHoldoutModel != "author_history_median_cycle" || readiness.SourceEventAsOfChronologicalHoldoutMaeDays == nil || *readiness.SourceEventAsOfChronologicalHoldoutMaeDays != 5.6 {
		t.Fatalf("source-event chronological = (%#v, %#v), want author_history_median_cycle 5.6", readiness.SourceEventAsOfChronologicalHoldoutModel, readiness.SourceEventAsOfChronologicalHoldoutMaeDays)
	}
	if readiness.SurvivalModel == nil || *readiness.SurvivalModel != "kaplan_meier_baseline" || readiness.SurvivalMaeDays == nil || *readiness.SurvivalMaeDays != 9.4 {
		t.Fatalf("survival = (%#v, %#v), want kaplan_meier_baseline 9.4", readiness.SurvivalModel, readiness.SurvivalMaeDays)
	}
	if readiness.TypedEvaluationCount != 7 {
		t.Fatalf("typedEvaluationCount = %d, want 7", readiness.TypedEvaluationCount)
	}
}

func TestForecastReadinessDiagnosticsUseTestCountWeightedMAE(t *testing.T) {
	source := "fixture-source"
	readiness := workForecastReadiness(&source, nil,
		&genent.WorkForecastEvaluation{
			EvaluationKind: workforecastevaluation.EvaluationKindSummary,
			SourceInstance: source,
			ReadyForEta:    false,
			ReadinessState: workforecastevaluation.ReadinessStateGated,
		},
		&genent.WorkForecastEvaluation{
			EvaluationKind: workforecastevaluation.EvaluationKindKfold,
			ModelName:      "low_single_fold_model",
			TestCount:      1,
			MaeDays:        floatPtr(1.0),
		},
		&genent.WorkForecastEvaluation{
			EvaluationKind: workforecastevaluation.EvaluationKindKfold,
			ModelName:      "low_single_fold_model",
			TestCount:      9,
			MaeDays:        floatPtr(10.0),
		},
		&genent.WorkForecastEvaluation{
			EvaluationKind: workforecastevaluation.EvaluationKindKfold,
			ModelName:      "stable_model",
			TestCount:      10,
			MaeDays:        floatPtr(6.0),
		},
	)
	if readiness.BestKfoldModel == nil || *readiness.BestKfoldModel != "stable_model" {
		t.Fatalf("bestKfoldModel = %#v, want weighted winner stable_model", readiness.BestKfoldModel)
	}
	if readiness.BestKfoldMaeDays == nil || *readiness.BestKfoldMaeDays != 6.0 {
		t.Fatalf("bestKfoldMaeDays = %#v, want 6.0", readiness.BestKfoldMaeDays)
	}
}

func TestForecastEvidenceSaysETAReadyRejectsNegativeLegacyText(t *testing.T) {
	action := &genent.WorkAction{
		DecisionState:  workaction.DecisionStateModelOrRuleQa,
		DecisionReason: "not eta ready; collect source-event snapshots before ETA use",
	}
	if forecastEvidenceSaysETAReady(action, nil) {
		t.Fatalf("negative legacy ETA text was treated as ETA-ready")
	}

	action.DecisionReason = "eta_forecast_ready=true"
	if !forecastEvidenceSaysETAReady(action, nil) {
		t.Fatalf("explicit legacy ETA-ready marker was not accepted")
	}
}

func TestForecastReadinessLegacyTextCannotPromoteETAReady(t *testing.T) {
	source := "fixture-source"
	readiness := workForecastReadiness(&source, []*genent.WorkAction{
		{
			Key:            "tpm-action:model-quality",
			ActionType:     workaction.ActionTypeModelQualityReview,
			ActionState:    workaction.ActionStateOpen,
			DecisionState:  workaction.DecisionStateModelOrRuleQa,
			DecisionReason: "eta_forecast_ready=true from legacy model-quality text",
			SubjectKind:    workaction.SubjectKindUnknown,
			SubjectKey:     "forecast-readiness",
			DueBucket:      workaction.DueBucketWatch,
		},
	})
	if readiness.EtaForecastReady {
		t.Fatalf("legacy text fallback promoted ETA readiness without typed forecast evaluation rows")
	}
	if readiness.ReadinessState != "gated" {
		t.Fatalf("legacy fallback readinessState = %q, want gated", readiness.ReadinessState)
	}
}
