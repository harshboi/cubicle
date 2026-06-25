package graphql

import (
	"context"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workprogramrun"
	"cubicle/services/ontology-service/ent/workprogramrunmember"

	entsql "entgo.io/ent/dialect/sql"
)

const (
	workProgramRunMemberTableAdversarialChecks    = "work_program_adversarial_checks"
	workProgramRunMemberTableBriefCaveats         = "work_program_brief_caveats"
	workProgramRunMemberTableBriefSnapshots       = "work_program_brief_snapshots"
	workProgramRunMemberTableDependencyEdges      = "work_dependency_edges"
	workProgramRunMemberTableEvidenceNeeds        = "work_program_evidence_needs"
	workProgramRunMemberTableForecasts            = "work_item_forecasts"
	workProgramRunMemberTableInsights             = "work_insights"
	workProgramRunMemberTableItems                = "work_program_items"
	workProgramRunMemberTableMilestones           = "work_program_milestones"
	workProgramRunMemberTableOwnerRollupSnapshots = "work_program_owner_rollup_snapshots"
	workProgramRunMemberTableQualityGates         = "work_program_quality_gates"
	workProgramRunMemberTableRiskDrivers          = "work_program_risk_drivers"
	workProgramRunMemberTableSummarySnapshots     = "work_program_summary_snapshots"
	workProgramRunMemberTableTPMFunctionReadiness = "work_program_tpm_function_readinesses"
)

func (r *queryResolver) latestWorkProgramRunMemberIDs(ctx context.Context, sourceFilter *string, workstreamKey *string, generatedAt *time.Time, memberTable string) ([]int, bool, error) {
	memberTable = strings.TrimSpace(memberTable)
	if memberTable == "" {
		return nil, false, nil
	}
	run, err := r.workProgramRunAnchorForGeneratedAt(ctx, sourceFilter, workstreamKey, generatedAt)
	if err != nil || run == nil {
		return nil, false, err
	}
	runPredicate := workProgramRunMemberRunPredicate(run)
	if runPredicate == nil {
		return nil, false, nil
	}
	members, err := r.EntClient.WorkProgramRunMember.Query().
		Where(runPredicate, workprogramrunmember.MemberTableEQ(memberTable)).
		Order(
			workprogramrunmember.ByMemberRankScore(entsql.OrderDesc()),
			workprogramrunmember.ByMemberID(),
		).
		All(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(members) == 0 {
		hasRunMembers, err := r.EntClient.WorkProgramRunMember.Query().
			Where(runPredicate).
			Exist(ctx)
		if err != nil {
			return nil, false, err
		}
		if !hasRunMembers && run.MemberCount <= 0 {
			return nil, false, nil
		}
		return []int{}, true, nil
	}
	seen := map[int]bool{}
	ids := make([]int, 0, len(members))
	for _, member := range members {
		if member.MemberID <= 0 || seen[member.MemberID] {
			continue
		}
		seen[member.MemberID] = true
		ids = append(ids, member.MemberID)
	}
	return ids, true, nil
}

func (r *queryResolver) workProgramRunMemberIDsForRun(ctx context.Context, run *genent.WorkProgramRun, memberTable string) ([]int, bool, error) {
	memberTable = strings.TrimSpace(memberTable)
	if memberTable == "" {
		return nil, false, nil
	}
	runPredicate := workProgramRunMemberRunPredicate(run)
	if runPredicate == nil {
		return nil, false, nil
	}
	members, err := r.EntClient.WorkProgramRunMember.Query().
		Where(runPredicate, workprogramrunmember.MemberTableEQ(memberTable)).
		Order(
			workprogramrunmember.ByMemberRankScore(entsql.OrderDesc()),
			workprogramrunmember.ByMemberID(),
		).
		All(ctx)
	if err != nil {
		return nil, false, err
	}
	if len(members) == 0 {
		hasRunMembers, err := r.EntClient.WorkProgramRunMember.Query().
			Where(runPredicate).
			Exist(ctx)
		if err != nil {
			return nil, false, err
		}
		if !hasRunMembers && run.MemberCount <= 0 {
			return nil, false, nil
		}
		return []int{}, true, nil
	}
	seen := map[int]bool{}
	ids := make([]int, 0, len(members))
	for _, member := range members {
		if member.MemberID <= 0 || seen[member.MemberID] {
			continue
		}
		seen[member.MemberID] = true
		ids = append(ids, member.MemberID)
	}
	return ids, true, nil
}

func (r *queryResolver) workProgramRunAnchorForGeneratedAt(ctx context.Context, sourceFilter *string, workstreamKey *string, generatedAt *time.Time) (*genent.WorkProgramRun, error) {
	if generatedAt == nil {
		return r.latestWorkProgramRunAnchor(ctx, sourceFilter, workstreamKey)
	}
	query := r.EntClient.WorkProgramRun.Query().
		Where(workprogramrun.SourceSystemEQ("cubicle_analytics")).
		Order(
			workprogramrun.ByRankScore(entsql.OrderDesc()),
			workprogramrun.ByMemberCount(entsql.OrderDesc()),
			workprogramrun.ByKey(),
		)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramrun.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramrun.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	if generatedAt.IsZero() {
		query = query.Where(workprogramrun.GeneratedAtIsNil())
	} else {
		query = query.Where(workProgramGeneratedAtTextPredicate(workprogramrun.FieldGeneratedAt, *generatedAt))
	}
	run, err := query.First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return run, nil
}

func workProgramRunMemberRunPredicate(run *genent.WorkProgramRun) predicate.WorkProgramRunMember {
	if run == nil {
		return nil
	}
	predicates := make([]predicate.WorkProgramRunMember, 0, 2)
	if run.ID > 0 {
		predicates = append(predicates, workprogramrunmember.WorkProgramRunIDEQ(run.ID))
	}
	if strings.TrimSpace(run.Key) != "" {
		predicates = append(predicates, workprogramrunmember.RunKeyEQ(strings.TrimSpace(run.Key)))
	}
	switch len(predicates) {
	case 0:
		return nil
	case 1:
		return predicates[0]
	default:
		return workprogramrunmember.Or(predicates...)
	}
}
