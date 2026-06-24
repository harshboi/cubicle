package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogramqualitygate"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkProgramQualityGateModels(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramBriefQualityGate, error) {
	rows, _, err := r.latestWorkProgramQualityGateModelsAndBlockingCount(ctx, sourceFilter, workstreamKey, limit)
	return rows, err
}

func (r *queryResolver) latestWorkProgramQualityGateModelsAndBlockingCount(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramBriefQualityGate, int, error) {
	return r.latestWorkProgramQualityGateModelsAndBlockingCountForGeneratedAt(ctx, sourceFilter, workstreamKey, nil, limit)
}

func (r *queryResolver) latestWorkProgramQualityGateModelsAndBlockingCountForGeneratedAt(ctx context.Context, sourceFilter *string, workstreamKey *string, generatedAt *time.Time, limit int) ([]*model.WorkProgramBriefQualityGate, int, error) {
	rowLimit := limit
	if rowLimit <= 0 {
		rowLimit = 20
	}
	if generatedAt != nil {
		memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, generatedAt, workProgramRunMemberTableQualityGates)
		if err != nil {
			return nil, 0, err
		}
		if hasRunMembers && len(memberIDs) == 0 {
			return []*model.WorkProgramBriefQualityGate{}, 0, nil
		}
		blockingQuery := r.workProgramQualityGateQueryForGeneratedAt(sourceFilter, workstreamKey, generatedAt, false).
			Where(workprogramqualitygate.BlockingEQ(true))
		if hasRunMembers {
			blockingQuery = blockingQuery.Where(workprogramqualitygate.IDIn(memberIDs...))
		}
		blockingCount, err := blockingQuery.Count(ctx)
		if err != nil {
			return nil, 0, err
		}
		query := r.workProgramQualityGateQueryForGeneratedAt(sourceFilter, workstreamKey, generatedAt, true)
		if hasRunMembers {
			query = query.Where(workprogramqualitygate.IDIn(memberIDs...))
		}
		rows, err := query.
			Order(
				workprogramqualitygate.ByRankScore(entsql.OrderDesc()),
				workprogramqualitygate.ByGateKey(),
			).
			Limit(rowLimit).
			All(ctx)
		if err != nil {
			return nil, 0, err
		}
		return workProgramQualityGateModels(rows), blockingCount, nil
	}
	latest, err := r.latestWorkProgramQualityGateRunAnchor(ctx, sourceFilter, workstreamKey)
	if err != nil || latest == nil {
		return []*model.WorkProgramBriefQualityGate{}, 0, err
	}
	blockingCount, err := r.workProgramQualityGateQueryForRunAnchor(sourceFilter, workstreamKey, latest, false).
		Where(workprogramqualitygate.BlockingEQ(true)).
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.workProgramQualityGateQueryForRunAnchor(sourceFilter, workstreamKey, latest, true).
		Order(
			workprogramqualitygate.ByRankScore(entsql.OrderDesc()),
			workprogramqualitygate.ByGateKey(),
		).
		Limit(rowLimit).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return workProgramQualityGateModels(rows), blockingCount, nil
}

func (r *queryResolver) workProgramQualityGateQueryForGeneratedAt(sourceFilter *string, workstreamKey *string, generatedAt *time.Time, withLatestEvidence bool) *genent.WorkProgramQualityGateQuery {
	query := r.EntClient.WorkProgramQualityGate.Query()
	if withLatestEvidence {
		query = query.WithLatestEvidence()
	}
	query = r.applyWorkProgramQualityGateFilters(query, sourceFilter, workstreamKey)
	if generatedAt == nil || generatedAt.IsZero() {
		return query.Where(workprogramqualitygate.GeneratedAtIsNil())
	}
	return query.Where(workProgramGeneratedAtTextPredicate(workprogramqualitygate.FieldGeneratedAt, *generatedAt))
}

func (r *queryResolver) latestWorkProgramQualityGateRunAnchor(ctx context.Context, sourceFilter *string, workstreamKey *string) (*genent.WorkProgramQualityGate, error) {
	latestQuery := r.applyWorkProgramQualityGateFilters(
		r.EntClient.WorkProgramQualityGate.Query().
			Order(
				workprogramqualitygate.ByGeneratedAt(entsql.OrderDesc()),
				workprogramqualitygate.ByRankScore(entsql.OrderDesc()),
			),
		sourceFilter,
		workstreamKey,
	)
	latest, err := latestQuery.First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func (r *queryResolver) workProgramQualityGateQueryForRunAnchor(sourceFilter *string, workstreamKey *string, latest *genent.WorkProgramQualityGate, withLatestEvidence bool) *genent.WorkProgramQualityGateQuery {
	query := r.EntClient.WorkProgramQualityGate.Query()
	if withLatestEvidence {
		query = query.WithLatestEvidence()
	}
	query = r.applyWorkProgramQualityGateFilters(query, sourceFilter, workstreamKey)
	if latest.SourceInstance != "" {
		query = query.Where(workprogramqualitygate.SourceInstanceEQ(latest.SourceInstance))
	}
	if runPrefix := workProgramQualityGateRunPrefix(latest.ExternalID); runPrefix != "" {
		query = query.Where(workprogramqualitygate.ExternalIDHasPrefix(runPrefix))
	} else if latest.GeneratedAt.IsZero() {
		query = query.Where(workprogramqualitygate.GeneratedAtIsNil())
	} else {
		query = query.Where(workprogramqualitygate.GeneratedAtEQ(latest.GeneratedAt))
	}
	return query
}

func (r *queryResolver) applyWorkProgramQualityGateFilters(query *genent.WorkProgramQualityGateQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramQualityGateQuery {
	query = query.Where(
		workprogramqualitygate.SourceSystemEQ("cubicle_analytics"),
		workprogramqualitygate.ExternalKindEQ("tpm_work_program_quality_gate"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramqualitygate.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramqualitygate.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramQualityGateRunPrefix(externalID string) string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return ""
	}
	if idx := strings.LastIndex(externalID, "|"); idx >= 0 {
		return externalID[:idx+1]
	}
	return ""
}
