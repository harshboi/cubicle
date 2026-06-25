package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogramriskdriver"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkProgramRiskDriverModels(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramBriefRiskDriver, error) {
	rowLimit := limit
	if rowLimit <= 0 {
		rowLimit = 50
	}
	memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, nil, workProgramRunMemberTableRiskDrivers)
	if err != nil {
		return nil, err
	}
	if hasRunMembers {
		if len(memberIDs) == 0 {
			return []*model.WorkProgramBriefRiskDriver{}, nil
		}
		rows, err := r.applyWorkProgramRiskDriverFilters(
			r.EntClient.WorkProgramRiskDriver.Query().
				WithLatestEvidence(),
			sourceFilter,
			workstreamKey,
		).
			Where(workprogramriskdriver.IDIn(memberIDs...)).
			Order(
				workprogramriskdriver.ByRankScore(entsql.OrderDesc()),
				workprogramriskdriver.ByDriverKind(),
				workprogramriskdriver.ByDriverKey(),
			).
			Limit(rowLimit).
			All(ctx)
		if err != nil {
			return nil, err
		}
		return workProgramRiskDriverModels(rows), nil
	}
	latestQuery := r.applyWorkProgramRiskDriverFilters(
		r.EntClient.WorkProgramRiskDriver.Query().
			Order(
				workprogramriskdriver.ByGeneratedAt(entsql.OrderDesc()),
				workprogramriskdriver.ByRankScore(entsql.OrderDesc()),
			),
		sourceFilter,
		workstreamKey,
	)
	latest, err := latestQuery.First(ctx)
	if genent.IsNotFound(err) {
		return []*model.WorkProgramBriefRiskDriver{}, nil
	}
	if err != nil {
		return nil, err
	}

	query := r.applyWorkProgramRiskDriverFilters(
		r.EntClient.WorkProgramRiskDriver.Query().
			WithLatestEvidence(),
		sourceFilter,
		workstreamKey,
	)
	if latest.SourceInstance != "" {
		query = query.Where(workprogramriskdriver.SourceInstanceEQ(latest.SourceInstance))
	}
	if runPrefix := workProgramRiskDriverRunPrefix(latest.ExternalID); runPrefix != "" {
		query = query.Where(workprogramriskdriver.ExternalIDHasPrefix(runPrefix))
	} else if latest.GeneratedAt.IsZero() {
		query = query.Where(workprogramriskdriver.GeneratedAtIsNil())
	} else {
		query = query.Where(workprogramriskdriver.GeneratedAtEQ(latest.GeneratedAt))
	}
	rows, err := query.
		Order(
			workprogramriskdriver.ByRankScore(entsql.OrderDesc()),
			workprogramriskdriver.ByDriverKind(),
			workprogramriskdriver.ByDriverKey(),
		).
		Limit(rowLimit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return workProgramRiskDriverModels(rows), nil
}

func (r *queryResolver) applyWorkProgramRiskDriverFilters(query *genent.WorkProgramRiskDriverQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramRiskDriverQuery {
	query = query.Where(
		workprogramriskdriver.SourceSystemEQ("cubicle_analytics"),
		workprogramriskdriver.ExternalKindEQ("tpm_work_program_risk_driver"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramriskdriver.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramriskdriver.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramRiskDriverRunPrefix(externalID string) string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return ""
	}
	if idx := strings.LastIndex(externalID, "|"); idx >= 0 {
		return externalID[:idx+1]
	}
	return ""
}
