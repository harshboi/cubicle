package graphql

import (
	"context"
	"strconv"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogramsummarysnapshot"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

type workProgramSummarySnapshotData struct {
	SourceInstance             *string
	WorkstreamKey              *string
	TotalCount                 int
	NeedsDecisionCount         int
	ValidateSignalCount        int
	CiFailingCount             int
	WaitingReviewCount         int
	SourceRepairCount          int
	ClosedPendingReviewCount   int
	ModelQualityCount          int
	ClosureCandidateCount      int
	DismissedCount             int
	NowCount                   int
	HighRiskCount              int
	UnassignedCount            int
	ProductActionCount         int
	ValidationLeadCount        int
	SourceCoverageLimitedCount int
	OwnerLoadStatus            string
	OwnerLoadActionCount       int
	OverloadedOwnerCount       int
	AttentionOwnerCount        int
	UnassignedActionCount      int
	BlockerCount               int
	ActiveBlockerCount         int
	ValidatingBlockerCount     int
	BlockerImpactCount         int
	ActiveBlockerImpactCount   int
	DependencyEdgeCount        int
	BlockingDependencyCount    int
	NeedsActionDependencyCount int
	OperatingStatus            string
	DecisionPressure           string
	ForecastState              string
	PrimaryRisk                *string
	RecommendedFocus           string
	CapabilityGaps             []string
	Breakdowns                 []*model.WorkActionBreakdown
}

func (r *queryResolver) latestWorkProgramSummarySnapshotData(ctx context.Context, sourceFilter *string, workstreamKey *string) (*workProgramSummarySnapshotData, error) {
	memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, nil, workProgramRunMemberTableSummarySnapshots)
	if err != nil {
		return nil, err
	}
	if hasRunMembers {
		if len(memberIDs) == 0 {
			return nil, nil
		}
		row, err := r.applyWorkProgramSummarySnapshotFilters(
			r.EntClient.WorkProgramSummarySnapshot.Query(),
			sourceFilter,
			workstreamKey,
		).
			Where(workprogramsummarysnapshot.IDIn(memberIDs...)).
			Order(
				workprogramsummarysnapshot.ByGeneratedAt(entsql.OrderDesc()),
				workprogramsummarysnapshot.ByRankScore(entsql.OrderDesc()),
				workprogramsummarysnapshot.ByUpdatedAt(entsql.OrderDesc()),
			).
			First(ctx)
		if genent.IsNotFound(err) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return workProgramSummarySnapshotDataModel(row), nil
	}
	query := r.applyWorkProgramSummarySnapshotFilters(
		r.EntClient.WorkProgramSummarySnapshot.Query(),
		sourceFilter,
		workstreamKey,
	)
	row, err := query.
		Order(
			workprogramsummarysnapshot.ByGeneratedAt(entsql.OrderDesc()),
			workprogramsummarysnapshot.ByRankScore(entsql.OrderDesc()),
			workprogramsummarysnapshot.ByUpdatedAt(entsql.OrderDesc()),
		).
		First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return workProgramSummarySnapshotDataModel(row), nil
}

func (r *queryResolver) applyWorkProgramSummarySnapshotFilters(query *genent.WorkProgramSummarySnapshotQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramSummarySnapshotQuery {
	query = query.Where(
		workprogramsummarysnapshot.SourceSystemEQ("cubicle_analytics"),
		workprogramsummarysnapshot.ExternalKindEQ("tpm_work_program_summary_snapshot"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogramsummarysnapshot.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogramsummarysnapshot.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramSummarySnapshotDataModel(row *genent.WorkProgramSummarySnapshot) *workProgramSummarySnapshotData {
	return &workProgramSummarySnapshotData{
		SourceInstance:             optionalString(row.SourceInstance),
		WorkstreamKey:              optionalString(row.WorkstreamKey),
		TotalCount:                 row.TotalCount,
		NeedsDecisionCount:         row.NeedsDecisionCount,
		ValidateSignalCount:        row.ValidateSignalCount,
		CiFailingCount:             row.CiFailingCount,
		WaitingReviewCount:         row.WaitingReviewCount,
		SourceRepairCount:          row.SourceRepairCount,
		ClosedPendingReviewCount:   row.ClosedPendingReviewCount,
		ModelQualityCount:          row.ModelQualityCount,
		ClosureCandidateCount:      row.ClosureCandidateCount,
		DismissedCount:             row.DismissedCount,
		NowCount:                   row.NowCount,
		HighRiskCount:              row.HighRiskCount,
		UnassignedCount:            row.UnassignedCount,
		ProductActionCount:         row.ProductActionCount,
		ValidationLeadCount:        row.ValidationLeadCount,
		SourceCoverageLimitedCount: row.SourceCoverageLimitedCount,
		OwnerLoadStatus:            row.OwnerLoadStatus,
		OwnerLoadActionCount:       row.OwnerLoadActionCount,
		OverloadedOwnerCount:       row.OverloadedOwnerCount,
		AttentionOwnerCount:        row.AttentionOwnerCount,
		UnassignedActionCount:      row.UnassignedActionCount,
		BlockerCount:               row.BlockerCount,
		ActiveBlockerCount:         row.ActiveBlockerCount,
		ValidatingBlockerCount:     row.ValidatingBlockerCount,
		BlockerImpactCount:         row.BlockerImpactCount,
		ActiveBlockerImpactCount:   row.ActiveBlockerImpactCount,
		DependencyEdgeCount:        row.DependencyEdgeCount,
		BlockingDependencyCount:    row.BlockingDependencyCount,
		NeedsActionDependencyCount: row.NeedsActionDependencyCount,
		OperatingStatus:            row.OperatingStatus,
		DecisionPressure:           row.DecisionPressure,
		ForecastState:              row.ForecastState,
		PrimaryRisk:                optionalString(row.PrimaryRisk),
		RecommendedFocus:           row.RecommendedFocus,
		CapabilityGaps:             workProgramUniqueStrings(splitLineList(row.CapabilityGaps)),
		Breakdowns:                 workProgramSummarySnapshotBreakdowns(row),
	}
}

func applyWorkProgramSummarySnapshotData(summary *model.WorkProgramSummary, snapshot *workProgramSummarySnapshotData) *model.WorkProgramSummary {
	if summary == nil || snapshot == nil {
		return summary
	}
	summary.SourceInstance = snapshot.SourceInstance
	summary.WorkstreamKey = snapshot.WorkstreamKey
	summary.TotalCount = snapshot.TotalCount
	summary.NeedsDecisionCount = snapshot.NeedsDecisionCount
	summary.ValidateSignalCount = snapshot.ValidateSignalCount
	summary.CiFailingCount = snapshot.CiFailingCount
	summary.WaitingReviewCount = snapshot.WaitingReviewCount
	summary.SourceRepairCount = snapshot.SourceRepairCount
	summary.ClosedPendingReviewCount = snapshot.ClosedPendingReviewCount
	summary.ModelQualityCount = snapshot.ModelQualityCount
	summary.ClosureCandidateCount = snapshot.ClosureCandidateCount
	summary.DismissedCount = snapshot.DismissedCount
	summary.NowCount = snapshot.NowCount
	summary.HighRiskCount = snapshot.HighRiskCount
	summary.UnassignedCount = snapshot.UnassignedCount
	summary.ProductActionCount = snapshot.ProductActionCount
	summary.ValidationLeadCount = snapshot.ValidationLeadCount
	summary.SourceCoverageLimitedCount = snapshot.SourceCoverageLimitedCount
	summary.OwnerLoadStatus = snapshot.OwnerLoadStatus
	summary.OwnerLoadActionCount = snapshot.OwnerLoadActionCount
	summary.OverloadedOwnerCount = snapshot.OverloadedOwnerCount
	summary.AttentionOwnerCount = snapshot.AttentionOwnerCount
	summary.UnassignedActionCount = snapshot.UnassignedActionCount
	summary.BlockerCount = snapshot.BlockerCount
	summary.ActiveBlockerCount = snapshot.ActiveBlockerCount
	summary.ValidatingBlockerCount = snapshot.ValidatingBlockerCount
	summary.BlockerImpactCount = snapshot.BlockerImpactCount
	summary.ActiveBlockerImpactCount = snapshot.ActiveBlockerImpactCount
	summary.DependencyEdgeCount = snapshot.DependencyEdgeCount
	summary.BlockingDependencyCount = snapshot.BlockingDependencyCount
	summary.NeedsActionDependencyCount = snapshot.NeedsActionDependencyCount
	summary.OperatingStatus = snapshot.OperatingStatus
	summary.DecisionPressure = snapshot.DecisionPressure
	summary.ForecastState = snapshot.ForecastState
	summary.PrimaryRisk = snapshot.PrimaryRisk
	summary.RecommendedFocus = snapshot.RecommendedFocus
	summary.CapabilityGaps = snapshot.CapabilityGaps
	if len(snapshot.Breakdowns) > 0 {
		summary.Breakdowns = snapshot.Breakdowns
	}
	summary.Badges = workProgramSummaryBadges(summary)
	return summary
}

func workProgramSummarySnapshotBreakdowns(row *genent.WorkProgramSummarySnapshot) []*model.WorkActionBreakdown {
	dimensions := splitLineList(row.BreakdownDimensions)
	keys := splitLineList(row.BreakdownKeys)
	counts := splitLineList(row.BreakdownCounts)
	limit := min(len(dimensions), len(keys), len(counts))
	out := make([]*model.WorkActionBreakdown, 0, limit)
	for i := 0; i < limit; i++ {
		count, err := strconv.Atoi(counts[i])
		if err != nil {
			continue
		}
		out = append(out, &model.WorkActionBreakdown{
			Dimension: dimensions[i],
			Key:       keys[i],
			Count:     count,
		})
	}
	return out
}
