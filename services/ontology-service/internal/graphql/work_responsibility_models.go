package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workResponsibilityModel(row *genent.WorkResponsibility) *model.WorkResponsibility {
	if row == nil {
		return nil
	}
	return &model.WorkResponsibility{
		Key:                       row.Key,
		SourceInstance:            optionalString(row.SourceInstance),
		WorkstreamKey:             optionalString(row.WorkstreamKey),
		SubjectKind:               row.SubjectKind.String(),
		SubjectKey:                row.SubjectKey,
		SubjectTitle:              optionalString(workResponsibilitySubjectTitle(row)),
		SubjectURL:                optionalString(workResponsibilitySubjectURL(row)),
		PartyKind:                 row.PartyKind.String(),
		PartyKey:                  row.PartyKey,
		PartySource:               optionalString(row.PartySource),
		PersonKey:                 optionalWorkResponsibilityPersonKey(row),
		PersonDisplayName:         optionalWorkResponsibilityPersonDisplayName(row),
		ResponsibilityKind:        row.ResponsibilityKind.String(),
		BasisKind:                 row.BasisKind.String(),
		BasisDetail:               optionalString(row.BasisDetail),
		ResponsibilityState:       row.ResponsibilityState.String(),
		ResponsibilityStateReason: optionalString(row.ResponsibilityStateReason),
		SourceURL:                 optionalString(row.SourceURL),
		EvidenceRef:               optionalString(evidenceRef(row.Edges.LatestEvidence)),
		Evidence:                  workEvidenceSummary(row.Edges.LatestEvidence),
		FreshnessState:            row.FreshnessState.String(),
		Visibility:                row.Visibility.String(),
		Confidence:                row.Confidence,
		RankScore:                 row.RankScore,
		Badges:                    workResponsibilityBadges(row),
		Action:                    workResponsibilityActionModel(row),
		Blocker:                   workResponsibilityBlockerModel(row),
		ProgramItem:               workResponsibilityProgramItemModel(row),
		EvidenceNeed:              workResponsibilityEvidenceNeedModel(row),
	}
}

func workResponsibilityModels(rows []*genent.WorkResponsibility) []*model.WorkResponsibility {
	out := make([]*model.WorkResponsibility, 0, len(rows))
	for _, row := range rows {
		out = append(out, workResponsibilityModel(row))
	}
	return out
}

func optionalWorkResponsibilityPersonKey(row *genent.WorkResponsibility) *string {
	if row == nil || row.Edges.Person == nil {
		return nil
	}
	return optionalString(row.Edges.Person.Key)
}

func optionalWorkResponsibilityPersonDisplayName(row *genent.WorkResponsibility) *string {
	if row == nil || row.Edges.Person == nil {
		return nil
	}
	return optionalString(row.Edges.Person.DisplayName)
}

func workResponsibilitySubjectTitle(row *genent.WorkResponsibility) string {
	if row == nil {
		return ""
	}
	switch row.SubjectKind.String() {
	case "pull_request":
		if row.Edges.PullRequest != nil {
			return row.Edges.PullRequest.Title
		}
	case "ticket":
		if row.Edges.Ticket != nil {
			return row.Edges.Ticket.Title
		}
	case "work_action":
		if row.Edges.WorkAction != nil {
			return workActionSubjectTitle(row.Edges.WorkAction)
		}
	case "work_blocker":
		if row.Edges.WorkBlocker != nil {
			return row.Edges.WorkBlocker.Title
		}
	case "work_program_evidence_need":
		if row.Edges.WorkProgramEvidenceNeed != nil {
			return row.Edges.WorkProgramEvidenceNeed.GateKey
		}
	}
	return ""
}

func workResponsibilitySubjectURL(row *genent.WorkResponsibility) string {
	if row == nil {
		return ""
	}
	switch row.SubjectKind.String() {
	case "pull_request":
		if row.Edges.PullRequest != nil {
			return row.Edges.PullRequest.SourceURL
		}
	case "ticket":
		if row.Edges.Ticket != nil {
			return row.Edges.Ticket.SourceURL
		}
	case "work_action":
		if row.Edges.WorkAction != nil {
			return workActionSubjectURL(row.Edges.WorkAction)
		}
	case "work_blocker":
		if row.Edges.WorkBlocker != nil {
			return row.Edges.WorkBlocker.SourceURL
		}
	case "work_program_evidence_need":
		if row.Edges.WorkProgramEvidenceNeed != nil {
			return row.Edges.WorkProgramEvidenceNeed.SourceURL
		}
	}
	return row.SourceURL
}

func workResponsibilityActionModel(row *genent.WorkResponsibility) *model.WorkAction {
	if row == nil || row.Edges.WorkAction == nil {
		return nil
	}
	return workActionModelWithClaimPolicy(row.Edges.WorkAction, workResponsibilityClaimPolicy(row))
}

func workResponsibilityBlockerModel(row *genent.WorkResponsibility) *model.WorkBlocker {
	if row == nil || row.Edges.WorkBlocker == nil {
		return nil
	}
	return workBlockerModel(row.Edges.WorkBlocker)
}

func workResponsibilityProgramItemModel(row *genent.WorkResponsibility) *model.WorkProgramItem {
	if row == nil || row.Edges.WorkProgramItem == nil {
		return nil
	}
	return workProgramItemModelWithClaimPolicy(row.Edges.WorkProgramItem, workResponsibilityClaimPolicy(row))
}

func workResponsibilityEvidenceNeedModel(row *genent.WorkResponsibility) *model.WorkProgramAutomationEvidenceNeed {
	if row == nil || row.Edges.WorkProgramEvidenceNeed == nil {
		return nil
	}
	return workProgramEvidenceNeedModel(row.Edges.WorkProgramEvidenceNeed)
}

func workResponsibilityBadges(row *genent.WorkResponsibility) []*model.WorkActionBadge {
	if row == nil {
		return []*model.WorkActionBadge{}
	}
	badges := []*model.WorkActionBadge{}
	switch row.ResponsibilityState.String() {
	case "active":
		badges = append(badges, &model.WorkActionBadge{Key: "responsibility_state", Label: "active", Tone: "success"})
	case "candidate":
		badges = append(badges, &model.WorkActionBadge{Key: "responsibility_state", Label: "candidate", Tone: "warning"})
	default:
		badges = append(badges, &model.WorkActionBadge{Key: "responsibility_state", Label: row.ResponsibilityState.String(), Tone: "neutral"})
	}
	if row.BasisKind.String() == "generated_candidate" {
		badges = append(badges, &model.WorkActionBadge{Key: "responsibility_basis", Label: "generated", Tone: "warning"})
	} else {
		badges = append(badges, &model.WorkActionBadge{Key: "responsibility_basis", Label: row.BasisKind.String(), Tone: "info"})
	}
	if row.PartyKind.String() == "unassigned" || row.PartyKind.String() == "unresolved" {
		badges = append(badges, &model.WorkActionBadge{Key: "party_resolution", Label: row.PartyKind.String(), Tone: "warning"})
	}
	return badges
}
