package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

type workProgramEvidenceNeedFilters struct {
	sourceFilter   *string
	workstreamKey  *string
	gateKey        *string
	evidenceKind   *string
	executionState *string
	ownerKey       *string
	actionKey      *string
	actionState    *string
	targetKey      *string
	generatedAt    *time.Time
}

func (r *queryResolver) latestWorkProgramEvidenceNeedModels(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*model.WorkProgramAutomationEvidenceNeed, error) {
	return r.latestWorkProgramEvidenceNeedModelsForFilters(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  sourceFilter,
		workstreamKey: workstreamKey,
	}, limit)
}

func (r *queryResolver) latestWorkProgramEvidenceNeedModelsForFilters(ctx context.Context, filters workProgramEvidenceNeedFilters, limit int) ([]*model.WorkProgramAutomationEvidenceNeed, error) {
	rows, _, err := r.latestWorkProgramEvidenceNeedModelsAndCountForFilters(ctx, filters, limit)
	return rows, err
}

func (r *queryResolver) latestWorkProgramEvidenceNeedModelsAndCountForFilters(ctx context.Context, filters workProgramEvidenceNeedFilters, limit int) ([]*model.WorkProgramAutomationEvidenceNeed, int, error) {
	return r.latestWorkProgramEvidenceNeedModelsAndCountForPredicates(ctx, filters, limit)
}

func (r *queryResolver) latestWorkProgramEvidenceNeedModelsAndCountForPredicates(ctx context.Context, filters workProgramEvidenceNeedFilters, limit int, predicates ...predicate.WorkProgramEvidenceNeed) ([]*model.WorkProgramAutomationEvidenceNeed, int, error) {
	rowLimit := limit
	if rowLimit <= 0 {
		rowLimit = 50
	}
	if filters.generatedAt != nil {
		memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, filters.sourceFilter, filters.workstreamKey, filters.generatedAt, workProgramRunMemberTableEvidenceNeeds)
		if err != nil {
			return nil, 0, err
		}
		if hasRunMembers && len(memberIDs) == 0 {
			return []*model.WorkProgramAutomationEvidenceNeed{}, 0, nil
		}
		countQuery := r.workProgramEvidenceNeedQueryForGeneratedAt(filters, false)
		if hasRunMembers {
			countQuery = countQuery.Where(workprogramevidenceneed.IDIn(memberIDs...))
		}
		if len(predicates) > 0 {
			countQuery = countQuery.Where(predicates...)
		}
		total, err := countQuery.Count(ctx)
		if err != nil {
			return nil, 0, err
		}
		query := r.workProgramEvidenceNeedQueryForGeneratedAt(filters, true)
		if hasRunMembers {
			query = query.Where(workprogramevidenceneed.IDIn(memberIDs...))
		}
		if len(predicates) > 0 {
			query = query.Where(predicates...)
		}
		rows, err := query.
			Order(
				workprogramevidenceneed.ByRankScore(entsql.OrderDesc()),
				workprogramevidenceneed.ByKey(),
			).
			Limit(rowLimit).
			All(ctx)
		if err != nil {
			return nil, 0, err
		}
		return workProgramEvidenceNeedModels(rows), total, nil
	}
	latest, err := r.latestWorkProgramEvidenceNeedRunAnchor(ctx, filters)
	if err != nil {
		return nil, 0, err
	}
	if latest == nil {
		return []*model.WorkProgramAutomationEvidenceNeed{}, 0, nil
	}
	countQuery := r.workProgramEvidenceNeedQueryForRunAnchor(filters, latest, false)
	if len(predicates) > 0 {
		countQuery = countQuery.Where(predicates...)
	}
	total, err := countQuery.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	query := r.workProgramEvidenceNeedQueryForRunAnchor(filters, latest, true)
	if len(predicates) > 0 {
		query = query.Where(predicates...)
	}
	rows, err := query.
		Order(
			workprogramevidenceneed.ByRankScore(entsql.OrderDesc()),
			workprogramevidenceneed.ByKey(),
		).
		Limit(rowLimit).
		All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return workProgramEvidenceNeedModels(rows), total, nil
}

func (r *queryResolver) workProgramEvidenceNeedQueryForGeneratedAt(filters workProgramEvidenceNeedFilters, withLatestEvidence bool) *genent.WorkProgramEvidenceNeedQuery {
	query := r.EntClient.WorkProgramEvidenceNeed.Query()
	if withLatestEvidence {
		query = query.
			WithLatestEvidence().
			WithQualityGate().
			WithWorkAction(func(actionQuery *genent.WorkActionQuery) {
				actionQuery.
					WithLatestEvidence().
					WithObservations().
					WithSourceInsights(func(insightQuery *genent.WorkInsightQuery) {
						insightQuery.WithLatestEvidence().WithReviews()
					})
			})
	}
	query = r.applyWorkProgramEvidenceNeedFilters(query, filters.sourceFilter, filters.workstreamKey)
	if filters.generatedAt == nil || filters.generatedAt.IsZero() {
		query = query.Where(workprogramevidenceneed.GeneratedAtIsNil())
	} else {
		query = query.Where(workProgramGeneratedAtTextPredicate(workprogramevidenceneed.FieldGeneratedAt, *filters.generatedAt))
	}
	return applyWorkProgramEvidenceNeedQueueFilters(query, filters)
}

func (r *queryResolver) latestWorkProgramEvidenceNeedCountForFilters(ctx context.Context, filters workProgramEvidenceNeedFilters) (int, error) {
	latest, err := r.latestWorkProgramEvidenceNeedRunAnchor(ctx, filters)
	if err != nil {
		return 0, err
	}
	if latest == nil {
		return 0, nil
	}
	query := r.workProgramEvidenceNeedQueryForRunAnchor(filters, latest, false)
	return query.Count(ctx)
}

func (r *queryResolver) latestWorkProgramEvidenceNeedRunAnchor(ctx context.Context, filters workProgramEvidenceNeedFilters) (*genent.WorkProgramEvidenceNeed, error) {
	// The latest run is anchored at source/workstream scope, then subqueue
	// filters are applied inside that same snapshot. This avoids falling back to
	// stale action/target-specific evidence from an older analytics run.
	latestQuery := r.applyWorkProgramEvidenceNeedFilters(
		r.EntClient.WorkProgramEvidenceNeed.Query().
			Order(
				workprogramevidenceneed.ByGeneratedAt(entsql.OrderDesc()),
				workprogramevidenceneed.ByRankScore(entsql.OrderDesc()),
			),
		filters.sourceFilter,
		filters.workstreamKey,
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

func (r *queryResolver) workProgramEvidenceNeedQueryForRunAnchor(filters workProgramEvidenceNeedFilters, latest *genent.WorkProgramEvidenceNeed, withLatestEvidence bool) *genent.WorkProgramEvidenceNeedQuery {
	query := r.EntClient.WorkProgramEvidenceNeed.Query()
	if withLatestEvidence {
		query = query.
			WithLatestEvidence().
			WithQualityGate().
			WithWorkAction(func(actionQuery *genent.WorkActionQuery) {
				actionQuery.
					WithLatestEvidence().
					WithObservations().
					WithSourceInsights(func(insightQuery *genent.WorkInsightQuery) {
						insightQuery.WithLatestEvidence().WithReviews()
					})
			})
	}
	query = r.applyWorkProgramEvidenceNeedFilters(query, filters.sourceFilter, filters.workstreamKey)
	if latest.SourceInstance != "" {
		query = query.Where(workprogramevidenceneed.SourceInstanceEQ(latest.SourceInstance))
	}
	if runPrefix := workProgramEvidenceNeedRunPrefix(latest.ExternalID); runPrefix != "" {
		query = query.Where(workprogramevidenceneed.ExternalIDHasPrefix(runPrefix))
	} else if latest.GeneratedAt.IsZero() {
		query = query.Where(workprogramevidenceneed.GeneratedAtIsNil())
	} else {
		query = query.Where(workprogramevidenceneed.GeneratedAtEQ(latest.GeneratedAt))
	}
	return applyWorkProgramEvidenceNeedQueueFilters(query, filters)
}

func applyWorkProgramEvidenceNeedQueueFilters(query *genent.WorkProgramEvidenceNeedQuery, filters workProgramEvidenceNeedFilters) *genent.WorkProgramEvidenceNeedQuery {
	if filters.gateKey != nil && strings.TrimSpace(*filters.gateKey) != "" {
		query = query.Where(workprogramevidenceneed.GateKeyEQ(strings.TrimSpace(*filters.gateKey)))
	}
	if filters.evidenceKind != nil && strings.TrimSpace(*filters.evidenceKind) != "" {
		query = query.Where(workprogramevidenceneed.EvidenceKindEQ(strings.TrimSpace(*filters.evidenceKind)))
	}
	if filters.executionState != nil && strings.TrimSpace(*filters.executionState) != "" {
		query = query.Where(workprogramevidenceneed.ExecutionStateEQ(strings.TrimSpace(*filters.executionState)))
	}
	if filters.ownerKey != nil && strings.TrimSpace(*filters.ownerKey) != "" {
		query = query.Where(workprogramevidenceneed.OwnerKeyEQ(strings.TrimSpace(*filters.ownerKey)))
	}
	if filters.actionKey != nil && strings.TrimSpace(*filters.actionKey) != "" {
		query = query.Where(workprogramevidenceneed.ActionKeyEQ(strings.TrimSpace(*filters.actionKey)))
	}
	if filters.actionState != nil && strings.TrimSpace(*filters.actionState) != "" {
		query = query.Where(workprogramevidenceneed.ActionStateEQ(strings.TrimSpace(*filters.actionState)))
	}
	if filters.targetKey != nil && strings.TrimSpace(*filters.targetKey) != "" {
		query = query.Where(workprogramevidenceneed.TargetKeyEQ(strings.TrimSpace(*filters.targetKey)))
	}
	return query
}

func (r *queryResolver) applyWorkProgramEvidenceNeedFilters(query *genent.WorkProgramEvidenceNeedQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramEvidenceNeedQuery {
	query = query.Where(
		workprogramevidenceneed.SourceSystemEQ("cubicle_analytics"),
		workprogramevidenceneed.ExternalKindEQ("tpm_work_program_evidence_need"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramevidenceneed.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramevidenceneed.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramEvidenceNeedRunPrefix(externalID string) string {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return ""
	}
	if strings.Count(externalID, "|") < 2 {
		return ""
	}
	if idx := strings.LastIndex(externalID, "|"); idx >= 0 {
		return externalID[:idx+1]
	}
	return ""
}

func workProgramPacketGlobalEvidenceTargetPredicate(workstreamKey *string) predicate.WorkProgramEvidenceNeed {
	predicates := []predicate.WorkProgramEvidenceNeed{
		workprogramevidenceneed.TargetKeyIsNil(),
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		predicates = append(predicates, workprogramevidenceneed.TargetKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return workprogramevidenceneed.Or(predicates...)
}
