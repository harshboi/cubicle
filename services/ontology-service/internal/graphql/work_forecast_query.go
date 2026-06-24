package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workforecastevaluation"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) workForecastEvaluationRows(ctx context.Context, sourceInstance *string) ([]*genent.WorkForecastEvaluation, error) {
	query := r.EntClient.WorkForecastEvaluation.Query().
		WithLatestEvidence().
		Order(
			workforecastevaluation.ByEvaluatedAt(entsql.OrderDesc()),
			workforecastevaluation.ByUpdatedAt(entsql.OrderDesc()),
			workforecastevaluation.ByEvaluationKind(),
			workforecastevaluation.ByModelName(),
			workforecastevaluation.ByFold(),
		).
		Limit(1000).
		Where(
			workforecastevaluation.SourceSystemEQ("cubicle_analytics"),
			workforecastevaluation.ExternalKindEQ("tpm_forecast_evaluation"),
		)
	if sourceInstance != nil && strings.TrimSpace(*sourceInstance) != "" {
		query = query.Where(workforecastevaluation.SourceInstanceEQ(strings.TrimSpace(*sourceInstance)))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	return currentForecastEvaluationRows(rows), nil
}

func (r *queryResolver) forecastReadinessModel(ctx context.Context, sourceInstance *string, actionState string) (*model.WorkForecastReadiness, error) {
	forecastRows, err := r.workForecastEvaluationRows(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	actions, err := r.workActionRowsForSource(ctx, actionState, 1000, sourceInstance)
	if err != nil {
		return nil, err
	}
	return workForecastReadiness(sourceInstance, actions, forecastRows...), nil
}

func currentForecastEvaluationRows(rows []*genent.WorkForecastEvaluation) []*genent.WorkForecastEvaluation {
	summary := latestForecastSummaryRow(rows)
	if summary == nil {
		return rows
	}
	if summary.EvaluatedAt.IsZero() {
		out := make([]*genent.WorkForecastEvaluation, 0, len(rows))
		for _, row := range rows {
			if row.SourceInstance == summary.SourceInstance {
				out = append(out, row)
			}
		}
		return out
	}
	out := make([]*genent.WorkForecastEvaluation, 0, len(rows))
	for _, row := range rows {
		if row.SourceInstance == summary.SourceInstance && sameForecastEvaluationRun(row.EvaluatedAt, summary.EvaluatedAt) {
			out = append(out, row)
		}
	}
	return out
}

func latestForecastSummaryRow(rows []*genent.WorkForecastEvaluation) *genent.WorkForecastEvaluation {
	var best *genent.WorkForecastEvaluation
	for _, row := range rows {
		if row.EvaluationKind != workforecastevaluation.EvaluationKindSummary {
			continue
		}
		if best == nil || forecastEvaluationTime(row).After(forecastEvaluationTime(best)) {
			best = row
		}
	}
	return best
}

func forecastEvaluationTime(row *genent.WorkForecastEvaluation) time.Time {
	if row == nil {
		return time.Time{}
	}
	if !row.EvaluatedAt.IsZero() {
		return row.EvaluatedAt
	}
	return row.UpdatedAt
}

func sameForecastEvaluationRun(left time.Time, right time.Time) bool {
	if left.IsZero() || right.IsZero() {
		return false
	}
	return left.UTC().Equal(right.UTC())
}
