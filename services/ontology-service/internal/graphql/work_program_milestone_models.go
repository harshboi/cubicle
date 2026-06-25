package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramMilestoneModel(row *genent.WorkProgramMilestone) *model.WorkProgramMilestone {
	return &model.WorkProgramMilestone{
		Key:                       row.Key,
		SourceInstance:            optionalString(row.SourceInstance),
		WorkstreamKey:             row.WorkstreamKey,
		SubjectKind:               row.SubjectKind.String(),
		SubjectObjectType:         optionalString(row.SubjectObjectType),
		SubjectKey:                row.SubjectKey,
		SubjectTitle:              optionalString(workProgramMilestoneSubjectTitle(row)),
		SubjectURL:                optionalString(workProgramMilestoneSubjectURL(row)),
		MilestoneKind:             row.MilestoneKind.String(),
		MilestoneName:             row.MilestoneName,
		TargetDate:                optionalTime(row.TargetDate),
		OutcomeDate:               optionalTime(row.OutcomeDate),
		MilestoneState:            row.MilestoneState.String(),
		CommitmentStrength:        row.CommitmentStrength.String(),
		DateClaimAllowed:          row.DateClaimAllowed,
		DeliveryCommitmentAllowed: row.DeliveryCommitmentAllowed,
		ClaimGateReason:           row.ClaimGateReason,
		SourceField:               row.SourceField,
		SourcePayloadKey:          optionalString(row.SourcePayloadKey),
		SourceURL:                 optionalString(row.SourceURL),
		CapturedAt:                optionalTime(row.CapturedAt),
		GeneratedAt:               optionalTime(row.GeneratedAt),
		EvidenceRef:               optionalString(firstNonempty(evidenceRef(row.Edges.LatestEvidence), row.SourceURL)),
		Evidence:                  workEvidenceSummary(row.Edges.LatestEvidence),
		Badges:                    workProgramMilestoneBadges(row),
	}
}

func workProgramMilestoneModels(rows []*genent.WorkProgramMilestone) []*model.WorkProgramMilestone {
	out := make([]*model.WorkProgramMilestone, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramMilestoneModel(row))
	}
	return out
}

func workProgramMilestoneSubjectTitle(row *genent.WorkProgramMilestone) string {
	if row.Edges.Ticket != nil {
		return row.Edges.Ticket.Title
	}
	if row.Edges.PullRequest != nil {
		return row.Edges.PullRequest.Title
	}
	return ""
}

func workProgramMilestoneSubjectURL(row *genent.WorkProgramMilestone) string {
	if row.Edges.Ticket != nil {
		return row.Edges.Ticket.SourceURL
	}
	if row.Edges.PullRequest != nil {
		return row.Edges.PullRequest.SourceURL
	}
	return ""
}

func workProgramMilestoneBadges(row *genent.WorkProgramMilestone) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{Key: "milestone:" + row.MilestoneKind.String(), Label: milestoneKindLabel(row.MilestoneKind.String()), Tone: "info"},
	}
	if row.DeliveryCommitmentAllowed {
		badges = append(badges, &model.WorkActionBadge{Key: "milestone:delivery_commitment", Label: "Delivery commitment", Tone: "success", Detail: optionalString(row.ClaimGateReason)})
	} else if row.DateClaimAllowed {
		badges = append(badges, &model.WorkActionBadge{Key: "milestone:date_claim", Label: milestoneDateClaimLabel(row.MilestoneKind.String()), Tone: "info", Detail: optionalString(row.ClaimGateReason)})
	} else {
		badges = append(badges, &model.WorkActionBadge{Key: "milestone:claim_gated", Label: "Date gated", Tone: "warning", Detail: optionalString(row.ClaimGateReason)})
	}
	switch row.MilestoneState.String() {
	case "past_target_unresolved", "resolved_after_target":
		badges = append(badges, &model.WorkActionBadge{Key: "milestone:past_target", Label: milestonePastTargetLabel(row.MilestoneKind.String()), Tone: "warning"})
	case "resolved_before_target":
		badges = append(badges, &model.WorkActionBadge{Key: "milestone:on_target", Label: milestoneOnTargetLabel(row.MilestoneKind.String()), Tone: "success"})
	case "no_target_date":
		badges = append(badges, &model.WorkActionBadge{Key: "milestone:no_target_date", Label: "No target date", Tone: "warning"})
	}
	return badges
}

func milestoneKindLabel(kind string) string {
	switch kind {
	case "explicit_due_date":
		return "Explicit due date"
	case "resolution_outcome":
		return "Outcome fact"
	default:
		return "Release target"
	}
}

func milestoneDateClaimLabel(kind string) string {
	switch kind {
	case "explicit_due_date":
		return "Due date"
	case "resolution_outcome":
		return "Outcome date observed"
	default:
		return "Release target date"
	}
}

func milestonePastTargetLabel(kind string) string {
	switch kind {
	case "explicit_due_date":
		return "Past due date"
	case "resolution_outcome":
		return "Outcome date"
	default:
		return "Past release target"
	}
}

func milestoneOnTargetLabel(kind string) string {
	switch kind {
	case "explicit_due_date":
		return "Resolved before due date"
	case "resolution_outcome":
		return "Outcome date"
	default:
		return "Resolved before release target"
	}
}
