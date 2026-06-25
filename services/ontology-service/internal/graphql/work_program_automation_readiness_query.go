package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogramautomationreadiness"
	"cubicle/services/ontology-service/ent/workprogramrun"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkProgramAutomationReadinessModel(
	ctx context.Context,
	sourceFilter *string,
	workstreamKey *string,
	gates []*model.WorkProgramBriefQualityGate,
	evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed,
) (*model.WorkProgramAutomationReadiness, error) {
	latest, err := r.latestWorkProgramAutomationReadinessRunAnchor(ctx, sourceFilter, workstreamKey)
	if err != nil || latest == nil {
		return nil, err
	}
	return workProgramAutomationReadinessModel(latest, gates, evidenceNeeds), nil
}

func (r *queryResolver) latestWorkProgramAutomationReadinessRunGeneratedAt(ctx context.Context, sourceFilter *string, workstreamKey *string) (*time.Time, error) {
	run, err := r.latestWorkProgramRunAnchor(ctx, sourceFilter, workstreamKey)
	if err != nil {
		return nil, err
	}
	if run != nil && !run.GeneratedAt.IsZero() {
		generatedAt := run.GeneratedAt
		return &generatedAt, nil
	}
	latest, err := r.latestWorkProgramAutomationReadinessRunAnchor(ctx, sourceFilter, workstreamKey)
	if err != nil || latest == nil || latest.GeneratedAt.IsZero() {
		return nil, err
	}
	generatedAt := latest.GeneratedAt
	return &generatedAt, nil
}

func (r *queryResolver) latestWorkProgramRunAnchor(ctx context.Context, sourceFilter *string, workstreamKey *string) (*genent.WorkProgramRun, error) {
	query := r.EntClient.WorkProgramRun.Query().
		Where(workprogramrun.SourceSystemEQ("cubicle_analytics")).
		Order(
			workprogramrun.ByGeneratedAt(entsql.OrderDesc()),
			workprogramrun.ByRankScore(entsql.OrderDesc()),
			workprogramrun.ByMemberCount(entsql.OrderDesc()),
		)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramrun.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramrun.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	latest, err := query.First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func (r *queryResolver) latestWorkProgramAutomationReadinessRunAnchor(ctx context.Context, sourceFilter *string, workstreamKey *string) (*genent.WorkProgramAutomationReadiness, error) {
	latestQuery := r.applyWorkProgramAutomationReadinessFilters(
		r.EntClient.WorkProgramAutomationReadiness.Query().
			WithLatestEvidence().
			Order(
				workprogramautomationreadiness.ByGeneratedAt(entsql.OrderDesc()),
				workprogramautomationreadiness.ByRankScore(entsql.OrderDesc()),
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

func (r *queryResolver) applyWorkProgramAutomationReadinessFilters(query *genent.WorkProgramAutomationReadinessQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramAutomationReadinessQuery {
	query = query.Where(
		workprogramautomationreadiness.SourceSystemEQ("cubicle_analytics"),
		workprogramautomationreadiness.ExternalKindEQ("tpm_work_program_automation_readiness"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramautomationreadiness.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramautomationreadiness.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramAutomationReadinessRunPrefix(externalID string) string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return ""
	}
	if idx := strings.LastIndex(externalID, "|"); idx >= 0 {
		return externalID[:idx+1]
	}
	return ""
}
