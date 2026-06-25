package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workprogrammilestone"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) latestWorkProgramMilestoneRows(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*genent.WorkProgramMilestone, []*genent.WorkProgramMilestone, *time.Time, error) {
	runGeneratedAt, err := r.latestWorkProgramAutomationReadinessRunGeneratedAt(ctx, sourceFilter, workstreamKey)
	if err != nil {
		return nil, nil, nil, err
	}
	if runGeneratedAt == nil {
		latest, err := r.latestWorkProgramMilestoneRunAnchor(ctx, sourceFilter, workstreamKey)
		if err != nil || latest == nil {
			return []*genent.WorkProgramMilestone{}, []*genent.WorkProgramMilestone{}, nil, err
		}
		generatedAt := latest.GeneratedAt
		runGeneratedAt = &generatedAt
	}
	memberIDs, hasRunMembers, err := r.latestWorkProgramRunMemberIDs(ctx, sourceFilter, workstreamKey, runGeneratedAt, workProgramRunMemberTableMilestones)
	if err != nil {
		return nil, nil, nil, err
	}
	if hasRunMembers && len(memberIDs) == 0 {
		return []*genent.WorkProgramMilestone{}, []*genent.WorkProgramMilestone{}, runGeneratedAt, nil
	}
	allQuery := r.workProgramMilestoneQueryForGeneratedAt(sourceFilter, workstreamKey, runGeneratedAt, true)
	if hasRunMembers {
		allQuery = allQuery.Where(workprogrammilestone.IDIn(memberIDs...))
	}
	allRows, err := allQuery.
		Order(
			workprogrammilestone.ByRankScore(entsql.OrderDesc()),
			workprogrammilestone.BySubjectKey(),
			workprogrammilestone.ByMilestoneName(),
		).
		All(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	rows := allRows
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, allRows, runGeneratedAt, nil
}

func (r *queryResolver) latestWorkProgramMilestoneRunAnchor(ctx context.Context, sourceFilter *string, workstreamKey *string) (*genent.WorkProgramMilestone, error) {
	query := r.applyWorkProgramMilestoneFilters(
		r.EntClient.WorkProgramMilestone.Query().
			WithLatestEvidence().
			WithTicket().
			WithPullRequest().
			Order(
				workprogrammilestone.ByGeneratedAt(entsql.OrderDesc()),
				workprogrammilestone.ByRankScore(entsql.OrderDesc()),
			),
		sourceFilter,
		workstreamKey,
	)
	latest, err := query.First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return latest, nil
}

func (r *queryResolver) workProgramMilestoneQueryForGeneratedAt(sourceFilter *string, workstreamKey *string, generatedAt *time.Time, withEdges bool) *genent.WorkProgramMilestoneQuery {
	query := r.EntClient.WorkProgramMilestone.Query()
	if withEdges {
		query = query.WithLatestEvidence().WithTicket().WithPullRequest()
	}
	query = r.applyWorkProgramMilestoneFilters(query, sourceFilter, workstreamKey)
	if generatedAt == nil || generatedAt.IsZero() {
		return query.Where(workprogrammilestone.GeneratedAtIsNil())
	}
	return query.Where(workProgramGeneratedAtTextPredicate(workprogrammilestone.FieldGeneratedAt, *generatedAt))
}

func (r *queryResolver) applyWorkProgramMilestoneFilters(query *genent.WorkProgramMilestoneQuery, sourceFilter *string, workstreamKey *string) *genent.WorkProgramMilestoneQuery {
	query = query.Where(
		workprogrammilestone.SourceSystemEQ("cubicle_analytics"),
		workprogrammilestone.ExternalKindEQ("tpm_work_program_milestone"),
	)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workprogrammilestone.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		query = query.Where(workprogrammilestone.WorkstreamKeyIn(workProgramWorkstreamFilterKeys(*workstreamKey)...))
	}
	return query
}

func workProgramMilestonePacketModel(sourceFilter *string, workstreamKey string, generatedAt *time.Time, rows []*genent.WorkProgramMilestone, allRows []*genent.WorkProgramMilestone) *model.WorkProgramMilestonePacket {
	packet := emptyWorkProgramMilestonePacket(workstreamKey, sourceFilter)
	packet.GeneratedAt = optionalTimePtr(generatedAt)
	packet.Milestones = workProgramMilestoneModels(rows)
	packet.TotalCount = len(allRows)
	for _, row := range allRows {
		switch row.MilestoneKind.String() {
		case "release_target":
			packet.ReleaseTargetCount++
			if !row.TargetDate.IsZero() {
				packet.DatedReleaseTargetCount++
			}
		case "explicit_due_date":
			packet.ExplicitDueDateCount++
		case "resolution_outcome":
			packet.OutcomeFactCount++
		}
		if row.CommitmentStrength.String() == "explicit_commitment" && row.DeliveryCommitmentAllowed {
			packet.DeliveryCommitmentCount++
		}
		if row.DateClaimAllowed {
			packet.DateClaimAllowedCount++
		}
		if row.DeliveryCommitmentAllowed {
			packet.DeliveryCommitmentAllowedCount++
		}
	}
	packet.Badges = workProgramMilestonePacketBadges(packet)
	return packet
}

func emptyWorkProgramMilestonePacket(workstreamKey string, sourceFilter *string) *model.WorkProgramMilestonePacket {
	return &model.WorkProgramMilestonePacket{
		SourceInstance: optionalString(pointerString(sourceFilter)),
		WorkstreamKey:  workstreamKey,
		Milestones:     []*model.WorkProgramMilestone{},
		Badges:         []*model.WorkActionBadge{},
	}
}

func workProgramMilestonePacketBadges(packet *model.WorkProgramMilestonePacket) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if packet.ExplicitDueDateCount == 0 {
		badges = append(badges, &model.WorkActionBadge{
			Key:    "milestone:no_due_date_rows",
			Label:  "No due-date rows observed",
			Tone:   "info",
			Detail: optionalString("Not an absence claim without complete source coverage."),
		})
	}
	if packet.DatedReleaseTargetCount > 0 {
		badges = append(badges, &model.WorkActionBadge{Key: "milestone:release_targets", Label: "Release targets", Tone: "info", Detail: optionalString(fmt.Sprintf("%d dated", packet.DatedReleaseTargetCount))})
	}
	if packet.DeliveryCommitmentAllowedCount == 0 {
		badges = append(badges, &model.WorkActionBadge{
			Key:    "milestone:no_delivery_commitment_rows",
			Label:  "No delivery-commitment rows observed",
			Tone:   "info",
			Detail: optionalString("Not an absence claim without complete source coverage."),
		})
	}
	return badges
}
