package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogramqualitygate"
	"cubicle/services/ontology-service/ent/workprogramtpmfunctionreadiness"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkProgramTPMFunctionReadinessModels(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramTpmFunctionReadiness, error) {
	rows, err := r.latestWorkProgramTPMFunctionReadinessRows(ctx, sourceFilter, workstreamKey)
	if err != nil {
		return nil, err
	}
	return workProgramTPMFunctionReadinessModels(limitWorkProgramTPMFunctionReadinessRows(rows, limit)), nil
}

func (r *queryResolver) latestWorkProgramTPMFunctionReadinessRows(ctx context.Context, sourceFilter *string, workstreamKey *string) ([]*genent.WorkProgramTPMFunctionReadiness, error) {
	return r.latestWorkProgramTPMFunctionReadinessRowsForGeneratedAt(ctx, sourceFilter, workstreamKey, nil)
}

func (r *queryResolver) latestWorkProgramTPMFunctionReadinessRowsForGeneratedAt(ctx context.Context, sourceFilter *string, workstreamKey *string, generatedAt *time.Time) ([]*genent.WorkProgramTPMFunctionReadiness, error) {
	if generatedAt != nil {
		memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, generatedAt, workProgramRunMemberTableTPMFunctionReadiness)
		if err != nil {
			return nil, err
		}
		if hasRunMembers && len(memberIDs) == 0 {
			return []*genent.WorkProgramTPMFunctionReadiness{}, nil
		}
		query := r.applyWorkProgramTPMFunctionReadinessFilters(
			r.EntClient.WorkProgramTPMFunctionReadiness.Query().
				WithLatestEvidence().
				WithBlockingQualityGates(func(gateQuery *genent.WorkProgramQualityGateQuery) {
					gateQuery.Order(workprogramqualitygate.ByGateKey())
				}),
			sourceFilter,
			workstreamKey,
		)
		if generatedAt.IsZero() {
			query = query.Where(workprogramtpmfunctionreadiness.GeneratedAtIsNil())
		} else {
			query = query.Where(workProgramGeneratedAtTextPredicate(workprogramtpmfunctionreadiness.FieldGeneratedAt, *generatedAt))
		}
		if hasRunMembers {
			query = query.Where(workprogramtpmfunctionreadiness.IDIn(memberIDs...))
		}
		rows, err := query.
			Order(
				workprogramtpmfunctionreadiness.ByRankScore(entsql.OrderDesc()),
				workprogramtpmfunctionreadiness.ByFunctionKey(),
			).
			All(ctx)
		if err != nil {
			return nil, err
		}
		return rows, nil
	}
	latestQuery := r.applyWorkProgramTPMFunctionReadinessFilters(
		r.EntClient.WorkProgramTPMFunctionReadiness.Query().
			Order(
				workprogramtpmfunctionreadiness.ByGeneratedAt(entsql.OrderDesc()),
				workprogramtpmfunctionreadiness.ByRankScore(entsql.OrderDesc()),
			),
		sourceFilter,
		workstreamKey,
	)
	latest, err := latestQuery.First(ctx)
	if genent.IsNotFound(err) {
		return []*genent.WorkProgramTPMFunctionReadiness{}, nil
	}
	if err != nil {
		return nil, err
	}

	query := r.applyWorkProgramTPMFunctionReadinessFilters(
		r.EntClient.WorkProgramTPMFunctionReadiness.Query().
			WithLatestEvidence().
			WithBlockingQualityGates(func(gateQuery *genent.WorkProgramQualityGateQuery) {
				gateQuery.Order(workprogramqualitygate.ByGateKey())
			}),
		sourceFilter,
		workstreamKey,
	)
	if latest.SourceInstance != "" {
		query = query.Where(workprogramtpmfunctionreadiness.SourceInstanceEQ(latest.SourceInstance))
	}
	if runPrefix := workProgramTPMFunctionReadinessRunPrefix(latest.ExternalID); runPrefix != "" {
		query = query.Where(workprogramtpmfunctionreadiness.ExternalIDHasPrefix(runPrefix))
	} else if latest.GeneratedAt.IsZero() {
		query = query.Where(workprogramtpmfunctionreadiness.GeneratedAtIsNil())
	} else {
		query = query.Where(workprogramtpmfunctionreadiness.GeneratedAtEQ(latest.GeneratedAt))
	}
	rows, err := query.
		Order(
			workprogramtpmfunctionreadiness.ByRankScore(entsql.OrderDesc()),
			workprogramtpmfunctionreadiness.ByFunctionKey(),
		).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func limitWorkProgramTPMFunctionReadinessRows(rows []*genent.WorkProgramTPMFunctionReadiness, limit int) []*genent.WorkProgramTPMFunctionReadiness {
	rowLimit := limit
	if rowLimit <= 0 {
		rowLimit = 20
	}
	if len(rows) <= rowLimit {
		return rows
	}
	return rows[:rowLimit]
}

func (r *queryResolver) applyWorkProgramTPMFunctionReadinessFilters(query *genent.WorkProgramTPMFunctionReadinessQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramTPMFunctionReadinessQuery {
	query = query.Where(
		workprogramtpmfunctionreadiness.SourceSystemEQ("cubicle_analytics"),
		workprogramtpmfunctionreadiness.ExternalKindEQ("tpm_work_program_tpm_function_readiness"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramtpmfunctionreadiness.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramtpmfunctionreadiness.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramTPMFunctionReadinessRunPrefix(externalID string) string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return ""
	}
	if idx := strings.LastIndex(externalID, "|"); idx >= 0 {
		return externalID[:idx+1]
	}
	return ""
}
