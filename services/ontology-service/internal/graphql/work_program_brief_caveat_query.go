package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogrambriefcaveat"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkProgramBriefCaveatModels(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramBriefCaveat, error) {
	rows, _, err := r.latestWorkProgramBriefCaveatModelsAndCount(ctx, sourceFilter, workstreamKey, limit)
	return rows, err
}

func (r *queryResolver) latestWorkProgramBriefCaveatModelsAndCount(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramBriefCaveat, int, error) {
	return r.latestWorkProgramBriefCaveatModelsAndCountForGeneratedAt(ctx, sourceFilter, workstreamKey, nil, limit)
}

func (r *queryResolver) latestWorkProgramBriefCaveatModelsAndCountForGeneratedAt(ctx context.Context, sourceFilter *string, workstreamKey *string, generatedAt *time.Time, limit int) ([]*model.WorkProgramBriefCaveat, int, error) {
	rowLimit := limit
	if rowLimit <= 0 {
		rowLimit = 20
	}
	if generatedAt != nil {
		memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, generatedAt, workProgramRunMemberTableBriefCaveats)
		if err != nil {
			return nil, 0, err
		}
		if hasRunMembers && len(memberIDs) == 0 {
			return []*model.WorkProgramBriefCaveat{}, 0, nil
		}
		countQuery := r.workProgramBriefCaveatQueryForGeneratedAt(sourceFilter, workstreamKey, generatedAt, false)
		if hasRunMembers {
			countQuery = countQuery.Where(workprogrambriefcaveat.IDIn(memberIDs...))
		}
		total, err := countQuery.Count(ctx)
		if err != nil {
			return nil, 0, err
		}
		query := r.workProgramBriefCaveatQueryForGeneratedAt(sourceFilter, workstreamKey, generatedAt, true)
		if hasRunMembers {
			query = query.Where(workprogrambriefcaveat.IDIn(memberIDs...))
		}
		rows, err := query.
			Order(
				workprogrambriefcaveat.ByRankScore(entsql.OrderDesc()),
				workprogrambriefcaveat.ByCaveatKey(),
			).
			Limit(rowLimit).
			All(ctx)
		if err != nil {
			return nil, 0, err
		}
		return workProgramBriefCaveatModels(rows), total, nil
	}
	latest, err := r.latestWorkProgramBriefCaveatRunAnchor(ctx, sourceFilter, workstreamKey)
	if err != nil || latest == nil {
		return []*model.WorkProgramBriefCaveat{}, 0, err
	}
	total, err := r.workProgramBriefCaveatQueryForRunAnchor(sourceFilter, workstreamKey, latest, false).Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.workProgramBriefCaveatQueryForRunAnchor(sourceFilter, workstreamKey, latest, true).
		Order(
			workprogrambriefcaveat.ByRankScore(entsql.OrderDesc()),
			workprogrambriefcaveat.ByCaveatKey(),
		).
		Limit(rowLimit).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return workProgramBriefCaveatModels(rows), total, nil
}

func (r *queryResolver) workProgramBriefCaveatQueryForGeneratedAt(sourceFilter *string, workstreamKey *string, generatedAt *time.Time, withLatestEvidence bool) *genent.WorkProgramBriefCaveatQuery {
	query := r.EntClient.WorkProgramBriefCaveat.Query()
	if withLatestEvidence {
		query = query.WithLatestEvidence()
	}
	query = r.applyWorkProgramBriefCaveatFilters(query, sourceFilter, workstreamKey)
	if generatedAt == nil || generatedAt.IsZero() {
		return query.Where(workprogrambriefcaveat.GeneratedAtIsNil())
	}
	return query.Where(workProgramGeneratedAtTextPredicate(workprogrambriefcaveat.FieldGeneratedAt, *generatedAt))
}

func (r *queryResolver) latestWorkProgramBriefCaveatRunAnchor(ctx context.Context, sourceFilter *string, workstreamKey *string) (*genent.WorkProgramBriefCaveat, error) {
	latestQuery := r.applyWorkProgramBriefCaveatFilters(
		r.EntClient.WorkProgramBriefCaveat.Query().
			Order(
				workprogrambriefcaveat.ByGeneratedAt(entsql.OrderDesc()),
				workprogrambriefcaveat.ByRankScore(entsql.OrderDesc()),
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

func (r *queryResolver) workProgramBriefCaveatQueryForRunAnchor(sourceFilter *string, workstreamKey *string, latest *genent.WorkProgramBriefCaveat, withLatestEvidence bool) *genent.WorkProgramBriefCaveatQuery {
	query := r.EntClient.WorkProgramBriefCaveat.Query()
	if withLatestEvidence {
		query = query.WithLatestEvidence()
	}
	query = r.applyWorkProgramBriefCaveatFilters(query, sourceFilter, workstreamKey)
	if latest.SourceInstance != "" {
		query = query.Where(workprogrambriefcaveat.SourceInstanceEQ(latest.SourceInstance))
	}
	if runPrefix := workProgramBriefCaveatRunPrefix(latest.ExternalID); runPrefix != "" {
		query = query.Where(workprogrambriefcaveat.ExternalIDHasPrefix(runPrefix))
	} else if latest.GeneratedAt.IsZero() {
		query = query.Where(workprogrambriefcaveat.GeneratedAtIsNil())
	} else {
		query = query.Where(workprogrambriefcaveat.GeneratedAtEQ(latest.GeneratedAt))
	}
	return query
}

func (r *queryResolver) applyWorkProgramBriefCaveatFilters(query *genent.WorkProgramBriefCaveatQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramBriefCaveatQuery {
	query = query.Where(
		workprogrambriefcaveat.SourceSystemEQ("cubicle_analytics"),
		workprogrambriefcaveat.ExternalKindEQ("tpm_work_program_brief_caveat"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogrambriefcaveat.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogrambriefcaveat.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramBriefCaveatRunPrefix(externalID string) string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return ""
	}
	if idx := strings.LastIndex(externalID, "|"); idx >= 0 {
		return externalID[:idx+1]
	}
	return ""
}
