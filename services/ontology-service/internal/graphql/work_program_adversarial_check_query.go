package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogramadversarialcheck"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

type workProgramAdversarialCheckCounts struct {
	failed  int
	warning int
}

func (r *queryResolver) latestWorkProgramAdversarialCheckModels(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramAdversarialCheck, error) {
	rows, _, err := r.latestWorkProgramAdversarialCheckModelsAndCounts(ctx, sourceFilter, workstreamKey, limit)
	return rows, err
}

func (r *queryResolver) latestWorkProgramAdversarialCheckModelsAndCounts(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramAdversarialCheck, workProgramAdversarialCheckCounts, error) {
	return r.latestWorkProgramAdversarialCheckModelsAndCountsForGeneratedAt(ctx, sourceFilter, workstreamKey, nil, limit)
}

func (r *queryResolver) latestWorkProgramAdversarialCheckModelsAndCountsForGeneratedAt(ctx context.Context, sourceFilter *string, workstreamKey *string, generatedAt *time.Time, limit int) ([]*model.WorkProgramAdversarialCheck, workProgramAdversarialCheckCounts, error) {
	rowLimit := limit
	if rowLimit <= 0 {
		rowLimit = 20
	}
	if generatedAt != nil {
		memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, generatedAt, workProgramRunMemberTableAdversarialChecks)
		if err != nil {
			return nil, workProgramAdversarialCheckCounts{}, err
		}
		if hasRunMembers && len(memberIDs) == 0 {
			return []*model.WorkProgramAdversarialCheck{}, workProgramAdversarialCheckCounts{}, nil
		}
		counts, err := r.workProgramAdversarialCheckCountsForGeneratedAt(ctx, sourceFilter, workstreamKey, generatedAt, memberIDs, hasRunMembers)
		if err != nil {
			return nil, workProgramAdversarialCheckCounts{}, err
		}
		query := r.workProgramAdversarialCheckQueryForGeneratedAt(sourceFilter, workstreamKey, generatedAt, true)
		if hasRunMembers {
			query = query.Where(workprogramadversarialcheck.IDIn(memberIDs...))
		}
		rows, err := query.
			Order(
				workprogramadversarialcheck.ByRankScore(entsql.OrderDesc()),
				workprogramadversarialcheck.ByKey(),
			).
			Limit(rowLimit).
			All(ctx)
		if err != nil {
			return nil, workProgramAdversarialCheckCounts{}, err
		}
		return workProgramAdversarialCheckModels(rows), counts, nil
	}
	latest, err := r.latestWorkProgramAdversarialCheckRunAnchor(ctx, sourceFilter, workstreamKey)
	if err != nil || latest == nil {
		return []*model.WorkProgramAdversarialCheck{}, workProgramAdversarialCheckCounts{}, err
	}
	counts, err := r.workProgramAdversarialCheckCountsForRunAnchor(ctx, sourceFilter, workstreamKey, latest)
	if err != nil {
		return nil, workProgramAdversarialCheckCounts{}, err
	}
	rows, err := r.workProgramAdversarialCheckQueryForRunAnchor(sourceFilter, workstreamKey, latest, true).
		Order(
			workprogramadversarialcheck.ByRankScore(entsql.OrderDesc()),
			workprogramadversarialcheck.ByKey(),
		).
		Limit(rowLimit).
		All(ctx)
	if err != nil {
		return nil, workProgramAdversarialCheckCounts{}, err
	}
	return workProgramAdversarialCheckModels(rows), counts, nil
}

func (r *queryResolver) workProgramAdversarialCheckCountsForGeneratedAt(ctx context.Context, sourceFilter *string, workstreamKey *string, generatedAt *time.Time, memberIDs []int, hasRunMembers bool) (workProgramAdversarialCheckCounts, error) {
	failedQuery := r.workProgramAdversarialCheckQueryForGeneratedAt(sourceFilter, workstreamKey, generatedAt, false).
		Where(workprogramadversarialcheck.CheckStateEQ(workprogramadversarialcheck.CheckStateFail))
	if hasRunMembers {
		failedQuery = failedQuery.Where(workprogramadversarialcheck.IDIn(memberIDs...))
	}
	failed, err := failedQuery.Count(ctx)
	if err != nil {
		return workProgramAdversarialCheckCounts{}, err
	}
	warningQuery := r.workProgramAdversarialCheckQueryForGeneratedAt(sourceFilter, workstreamKey, generatedAt, false).
		Where(workprogramadversarialcheck.CheckStateEQ(workprogramadversarialcheck.CheckStateWarning))
	if hasRunMembers {
		warningQuery = warningQuery.Where(workprogramadversarialcheck.IDIn(memberIDs...))
	}
	warning, err := warningQuery.Count(ctx)
	if err != nil {
		return workProgramAdversarialCheckCounts{}, err
	}
	return workProgramAdversarialCheckCounts{failed: failed, warning: warning}, nil
}

func (r *queryResolver) workProgramAdversarialCheckQueryForGeneratedAt(sourceFilter *string, workstreamKey *string, generatedAt *time.Time, withLatestEvidence bool) *genent.WorkProgramAdversarialCheckQuery {
	query := r.EntClient.WorkProgramAdversarialCheck.Query()
	if withLatestEvidence {
		query = query.WithLatestEvidence()
	}
	query = r.applyWorkProgramAdversarialCheckFilters(query, sourceFilter, workstreamKey)
	if generatedAt == nil || generatedAt.IsZero() {
		return query.Where(workprogramadversarialcheck.GeneratedAtIsNil())
	}
	return query.Where(workProgramGeneratedAtTextPredicate(workprogramadversarialcheck.FieldGeneratedAt, *generatedAt))
}

func (r *queryResolver) latestWorkProgramAdversarialCheckRunAnchor(ctx context.Context, sourceFilter *string, workstreamKey *string) (*genent.WorkProgramAdversarialCheck, error) {
	latestQuery := r.applyWorkProgramAdversarialCheckFilters(
		r.EntClient.WorkProgramAdversarialCheck.Query().
			Order(
				workprogramadversarialcheck.ByGeneratedAt(entsql.OrderDesc()),
				workprogramadversarialcheck.ByRankScore(entsql.OrderDesc()),
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

func (r *queryResolver) workProgramAdversarialCheckCountsForRunAnchor(ctx context.Context, sourceFilter *string, workstreamKey *string, latest *genent.WorkProgramAdversarialCheck) (workProgramAdversarialCheckCounts, error) {
	failed, err := r.workProgramAdversarialCheckQueryForRunAnchor(sourceFilter, workstreamKey, latest, false).
		Where(workprogramadversarialcheck.CheckStateEQ(workprogramadversarialcheck.CheckStateFail)).
		Count(ctx)
	if err != nil {
		return workProgramAdversarialCheckCounts{}, err
	}
	warning, err := r.workProgramAdversarialCheckQueryForRunAnchor(sourceFilter, workstreamKey, latest, false).
		Where(workprogramadversarialcheck.CheckStateEQ(workprogramadversarialcheck.CheckStateWarning)).
		Count(ctx)
	if err != nil {
		return workProgramAdversarialCheckCounts{}, err
	}
	return workProgramAdversarialCheckCounts{failed: failed, warning: warning}, nil
}

func (r *queryResolver) workProgramAdversarialCheckQueryForRunAnchor(sourceFilter *string, workstreamKey *string, latest *genent.WorkProgramAdversarialCheck, withLatestEvidence bool) *genent.WorkProgramAdversarialCheckQuery {
	query := r.EntClient.WorkProgramAdversarialCheck.Query()
	if withLatestEvidence {
		query = query.WithLatestEvidence()
	}
	query = r.applyWorkProgramAdversarialCheckFilters(query, sourceFilter, workstreamKey)
	if latest.SourceInstance != "" {
		query = query.Where(workprogramadversarialcheck.SourceInstanceEQ(latest.SourceInstance))
	}
	if runPrefix := workProgramAdversarialCheckRunPrefix(latest.ExternalID); runPrefix != "" {
		query = query.Where(workprogramadversarialcheck.ExternalIDHasPrefix(runPrefix))
	} else if latest.GeneratedAt.IsZero() {
		query = query.Where(workprogramadversarialcheck.GeneratedAtIsNil())
	} else {
		query = query.Where(workprogramadversarialcheck.GeneratedAtEQ(latest.GeneratedAt))
	}
	return query
}

func (r *queryResolver) applyWorkProgramAdversarialCheckFilters(query *genent.WorkProgramAdversarialCheckQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramAdversarialCheckQuery {
	query = query.Where(
		workprogramadversarialcheck.SourceSystemEQ("cubicle_analytics"),
		workprogramadversarialcheck.ExternalKindEQ("tpm_work_program_adversarial_check"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramadversarialcheck.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramadversarialcheck.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramAdversarialCheckRunPrefix(externalID string) string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return ""
	}
	idx := strings.LastIndex(externalID, ":")
	if idx < 0 || idx+1 >= len(externalID) {
		return ""
	}
	return externalID[:idx+1]
}
