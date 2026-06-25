package graphql

import (
	"fmt"
	"math"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workforecastevaluation"
	"cubicle/services/ontology-service/internal/graphql/model"
)

const (
	forecastEvaluationKindLifecycleAsOf = "lifecycle_as_of_baseline"
)

func workProgramForecastReliabilityModels(forecastRows []*genent.WorkForecastEvaluation, decisionRows []*genent.WorkDecisionTargetEvaluation, readiness *model.WorkForecastReadiness, decisionReadiness *model.WorkDecisionTargetReadiness) []*model.WorkProgramForecastReliability {
	return []*model.WorkProgramForecastReliability{
		workProgramPointEtaReliability(forecastRows, readiness),
		workProgramRangeEtaReliability(forecastRows),
		workProgramRiskTriageReliability(decisionRows, decisionReadiness),
	}
}

func workProgramPointEtaReliability(rows []*genent.WorkForecastEvaluation, readiness *model.WorkForecastReadiness) *model.WorkProgramForecastReliability {
	if readiness == nil || readiness.TypedEvaluationCount == 0 {
		return &model.WorkProgramForecastReliability{
			ForecastProduct: "point_eta",
			ReadinessState:  "missing_forecast_summary",
			ProductSafe:     false,
			SafeUse:         "none",
			BestModel:       "",
			PrimaryMetric:   "",
			MetricValue:     "",
			NextEvidence:    "run_forecast_backtests",
			Guardrail:       "No forecast summary exists, so Cubicle cannot make point ETA claims.",
		}
	}
	best := bestForecastImprovement(rows, workforecastevaluation.EvaluationKindKfold.String(), firstStringPtr(readiness.BestBacktestModel, readiness.BestKfoldModel))
	bestModel := firstNonempty(best.modelName, stringPtrValue(readiness.BestBacktestModel), stringPtrValue(readiness.BestKfoldModel), stringPtrValue(readiness.ForecastMethod))
	metricName := "eta_kfold_best_candidate_improvement_pct"
	metricValue := formatOptionalMetric(best.valuePtr(), 2)
	if metricValue == "" && readiness.BestKfoldMaeDays != nil {
		metricName = "eta_kfold_best_mae_days"
		metricValue = formatMetricValue(*readiness.BestKfoldMaeDays, 2)
	}
	blocker := "forecast_quality_gate"
	if readiness.EtaReadinessBlockingReason != nil && strings.TrimSpace(*readiness.EtaReadinessBlockingReason) != "" {
		blocker = strings.TrimSpace(*readiness.EtaReadinessBlockingReason)
	}
	if readiness.EtaForecastReady {
		return &model.WorkProgramForecastReliability{
			ForecastProduct: "point_eta",
			ReadinessState:  "ready",
			ProductSafe:     true,
			SafeUse:         "date_or_remaining_day_commitment",
			BestModel:       bestModel,
			PrimaryMetric:   metricName,
			MetricValue:     metricValue,
			NextEvidence:    "monitor_eta_drift_against_observed_outcomes",
			Guardrail:       "Point ETA is product-safe only when the same candidate beats median and heuristic baselines on K-fold and chronological holdout, and as-of snapshots are ready.",
		}
	}
	return &model.WorkProgramForecastReliability{
		ForecastProduct: "point_eta",
		ReadinessState:  firstNonempty(readiness.ReadinessState, "gated"),
		ProductSafe:     false,
		SafeUse:         "diagnostic_only",
		BestModel:       bestModel,
		PrimaryMetric:   metricName,
		MetricValue:     metricValue,
		NextEvidence:    "improve_model_features_and_validate_against_as_of_snapshots",
		Guardrail:       "Point ETA is blocked by " + blocker + "; use forecast outputs for risk triage only.",
	}
}

func workProgramRangeEtaReliability(rows []*genent.WorkForecastEvaluation) *model.WorkProgramForecastReliability {
	best := bestRemainingTimeBaseline(rows)
	if best.modelName == "" {
		return &model.WorkProgramForecastReliability{
			ForecastProduct: "range_eta",
			ReadinessState:  "missing_baseline",
			ProductSafe:     false,
			SafeUse:         "none",
			BestModel:       "",
			PrimaryMetric:   "",
			MetricValue:     "",
			NextEvidence:    "calibrate_prediction_intervals_against_live_as_of_snapshots",
			Guardrail:       "No lifecycle or survival remaining-time baseline is available for range-style forecast diagnostics.",
		}
	}
	return &model.WorkProgramForecastReliability{
		ForecastProduct: "range_eta",
		ReadinessState:  "diagnostic_only",
		ProductSafe:     false,
		SafeUse:         "wide_range_context",
		BestModel:       best.modelName,
		PrimaryMetric:   "best_remaining_time_mae_days",
		MetricValue:     formatMetricValue(best.value, 2),
		NextEvidence:    "calibrate_prediction_intervals_against_live_as_of_snapshots",
		Guardrail:       "Remaining-time baselines can explain uncertainty, but they are not product-safe ETA ranges until interval coverage and width are validated on live as-of snapshots.",
	}
}

func workProgramRiskTriageReliability(rows []*genent.WorkDecisionTargetEvaluation, readiness *model.WorkDecisionTargetReadiness) *model.WorkProgramForecastReliability {
	best := bestRiskTriageLift(rows)
	riskReady := best.value >= 0.10
	readinessState := "gated"
	nextEvidence := "collect_more_slow_cycle_outcomes"
	if riskReady {
		readinessState = "ready"
		if readiness != nil && readiness.CoverageGateState != "passed" {
			readinessState = "ready_with_coverage_guardrail"
			nextEvidence = "add_more_coverage_strata_for_confounding_checks"
		}
	}
	return &model.WorkProgramForecastReliability{
		ForecastProduct: "risk_triage",
		ReadinessState:  readinessState,
		ProductSafe:     riskReady,
		SafeUse:         workProgramRiskTriageSafeUse(riskReady),
		BestModel:       best.modelName,
		PrimaryMetric:   "risk_triage_lift_at_10pct",
		MetricValue:     formatOptionalMetric(best.valuePtr(), 4),
		NextEvidence:    nextEvidence,
		Guardrail:       "Risk triage is product-safe only for attention ordering; it is not an ETA, causality, blocker, or autonomous action claim.",
	}
}

func workProgramRiskTriageSafeUse(ready bool) string {
	if ready {
		return "attention_ordering"
	}
	return "diagnostic_only"
}

type namedMetric struct {
	modelName string
	value     float64
	ok        bool
}

func (metric namedMetric) valuePtr() *float64 {
	if !metric.ok {
		return nil
	}
	return &metric.value
}

func bestForecastImprovement(rows []*genent.WorkForecastEvaluation, evaluationKind string, preferredModel string) namedMetric {
	byModel := map[string]*weightedMetric{}
	for _, row := range rows {
		if row == nil || row.ImprovementVsMedianPct == nil || row.EvaluationKind.String() != evaluationKind {
			continue
		}
		modelName := firstNonempty(row.ModelName, row.BestModelName)
		if modelName == "" {
			continue
		}
		metric := byModel[modelName]
		if metric == nil {
			metric = &weightedMetric{}
			byModel[modelName] = metric
		}
		metric.add(*row.ImprovementVsMedianPct, row.TestCount)
	}
	if preferredModel != "" {
		if metric := byModel[preferredModel]; metric != nil {
			if mean, ok := metric.mean(); ok {
				return namedMetric{modelName: preferredModel, value: mean, ok: true}
			}
		}
	}
	var best namedMetric
	for modelName, metric := range byModel {
		mean, ok := metric.mean()
		if !ok {
			continue
		}
		if !best.ok || mean > best.value || (mean == best.value && modelName < best.modelName) {
			best = namedMetric{modelName: modelName, value: mean, ok: true}
		}
	}
	return best
}

func bestRemainingTimeBaseline(rows []*genent.WorkForecastEvaluation) namedMetric {
	var best namedMetric
	for _, row := range rows {
		if row == nil || row.MaeDays == nil {
			continue
		}
		kind := row.EvaluationKind.String()
		if kind != forecastEvaluationKindLifecycleAsOf && kind != workforecastevaluation.EvaluationKindSurvivalTimeToMerge.String() {
			continue
		}
		prefix := "lifecycle_as_of"
		if kind == workforecastevaluation.EvaluationKindSurvivalTimeToMerge.String() {
			prefix = "survival_time_to_merge"
		}
		modelName := firstNonempty(row.ModelName, row.BestModelName)
		if modelName == "" {
			continue
		}
		fullName := prefix + ":" + modelName
		value := *row.MaeDays
		if !best.ok || value < best.value || (value == best.value && fullName < best.modelName) {
			best = namedMetric{modelName: fullName, value: value, ok: true}
		}
	}
	return best
}

func bestRiskTriageLift(rows []*genent.WorkDecisionTargetEvaluation) namedMetric {
	var best namedMetric
	for _, row := range rows {
		if row == nil || row.LiftAt10pct == nil {
			continue
		}
		modelName := firstNonempty(row.ModelName, row.TargetKind)
		value := *row.LiftAt10pct
		if !best.ok || value > best.value || (value == best.value && modelName < best.modelName) {
			best = namedMetric{modelName: modelName, value: value, ok: true}
		}
	}
	return best
}

type weightedMetric struct {
	total  float64
	weight int
}

func (metric *weightedMetric) add(value float64, weight int) {
	if weight <= 0 {
		weight = 1
	}
	metric.total += value * float64(weight)
	metric.weight += weight
}

func (metric *weightedMetric) mean() (float64, bool) {
	if metric == nil || metric.weight == 0 {
		return 0, false
	}
	return metric.total / float64(metric.weight), true
}

func firstStringPtr(values ...*string) string {
	for _, value := range values {
		if value != nil && strings.TrimSpace(*value) != "" {
			return strings.TrimSpace(*value)
		}
	}
	return ""
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func formatOptionalMetric(value *float64, precision int) string {
	if value == nil {
		return ""
	}
	return formatMetricValue(*value, precision)
}

func formatMetricValue(value float64, precision int) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	return fmt.Sprintf("%.*f", precision, value)
}
