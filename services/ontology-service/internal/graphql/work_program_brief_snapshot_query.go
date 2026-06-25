package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogrambriefsnapshot"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkProgramBriefSnapshotData(ctx context.Context, sourceFilter *string, workstreamKey *string) (*workProgramBriefSnapshotData, error) {
	memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, nil, workProgramRunMemberTableBriefSnapshots)
	if err != nil {
		return nil, err
	}
	if hasRunMembers {
		if len(memberIDs) == 0 {
			return nil, nil
		}
		row, err := r.applyWorkProgramBriefSnapshotFilters(
			r.EntClient.WorkProgramBriefSnapshot.Query().
				WithLatestEvidence(),
			sourceFilter,
			workstreamKey,
		).
			Where(workprogrambriefsnapshot.IDIn(memberIDs...)).
			Order(
				workprogrambriefsnapshot.ByGeneratedAt(entsql.OrderDesc()),
				workprogrambriefsnapshot.ByRankScore(entsql.OrderDesc()),
			).
			First(ctx)
		if genent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return workProgramBriefSnapshotDataModel(row), nil
	}
	query := r.applyWorkProgramBriefSnapshotFilters(
		r.EntClient.WorkProgramBriefSnapshot.Query().
			WithLatestEvidence(),
		sourceFilter,
		workstreamKey,
	)
	row, err := query.
		Order(
			workprogrambriefsnapshot.ByGeneratedAt(entsql.OrderDesc()),
			workprogrambriefsnapshot.ByRankScore(entsql.OrderDesc()),
		).
		First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return workProgramBriefSnapshotDataModel(row), nil
}

func (r *queryResolver) applyWorkProgramBriefSnapshotFilters(query *genent.WorkProgramBriefSnapshotQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramBriefSnapshotQuery {
	query = query.Where(
		workprogrambriefsnapshot.SourceSystemEQ("cubicle_analytics"),
		workprogrambriefsnapshot.ExternalKindEQ("tpm_work_program_brief_snapshot"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogrambriefsnapshot.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogrambriefsnapshot.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}
