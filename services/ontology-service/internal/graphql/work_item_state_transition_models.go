package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workitemstatetransition"
	"cubicle/services/ontology-service/internal/graphql/model"
	"strconv"
)

func workItemStateTransitionModel(row *genent.WorkItemStateTransition) *model.WorkItemStateTransition {
	return &model.WorkItemStateTransition{
		Key:                  row.Key,
		SourceInstance:       optionalString(row.SourceInstance),
		SubjectKind:          row.SubjectKind.String(),
		SubjectKey:           row.SubjectKey,
		SubjectTitle:         optionalString(workItemStateTransitionSubjectTitle(row)),
		SubjectURL:           optionalString(workItemStateTransitionSubjectURL(row)),
		FromObservedAt:       optionalTime(row.FromObservedAt),
		ToObservedAt:         optionalTime(row.ToObservedAt),
		FromState:            optionalString(row.FromState),
		ToState:              optionalString(row.ToState),
		TransitionKind:       row.TransitionKind.String(),
		TransitionConfidence: row.TransitionConfidence,
		ConfidenceBasis:      row.ConfidenceBasis.String(),
		VerificationState:    row.VerificationState.String(),
		Terminal:             row.Terminal,
		RequiresCloseout:     row.RequiresCloseout,
		Note:                 optionalString(row.Note),
		FromSnapshot:         optionalTransitionSnapshot(row.Edges.FromSnapshot),
		ToSnapshot:           optionalTransitionSnapshot(row.Edges.ToSnapshot),
		EvidenceRef:          optionalString(evidenceRef(row.Edges.LatestEvidence)),
		Evidence:             workEvidenceSummary(row.Edges.LatestEvidence),
		Badges:               workItemStateTransitionBadges(row),
	}
}

func workItemStateTransitionSubjectTitle(row *genent.WorkItemStateTransition) string {
	if row.Edges.ToSnapshot != nil {
		if title := workItemStateSnapshotSubjectTitle(row.Edges.ToSnapshot); title != "" {
			return title
		}
	}
	if row.Edges.PullRequest != nil && row.Edges.PullRequest.Title != "" {
		return row.Edges.PullRequest.Title
	}
	if row.Edges.Ticket != nil && row.Edges.Ticket.Title != "" {
		return row.Edges.Ticket.Title
	}
	return ""
}

func workItemStateTransitionSubjectURL(row *genent.WorkItemStateTransition) string {
	if row.Edges.PullRequest != nil && row.Edges.PullRequest.SourceURL != "" {
		return row.Edges.PullRequest.SourceURL
	}
	if row.Edges.Ticket != nil && row.Edges.Ticket.SourceURL != "" {
		return row.Edges.Ticket.SourceURL
	}
	if row.Edges.ToSnapshot != nil {
		return workItemStateSnapshotSubjectURL(row.Edges.ToSnapshot)
	}
	return row.SourceURL
}

func optionalTransitionSnapshot(row *genent.WorkItemStateSnapshot) *model.WorkItemStateSnapshot {
	if row == nil {
		return nil
	}
	return workItemStateSnapshotModel(row)
}

func workItemStateTransitionBadges(row *genent.WorkItemStateTransition) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if row.Terminal {
		badges = append(badges, &model.WorkActionBadge{Key: "transition:terminal", Label: "Terminal transition", Tone: "success", Detail: optionalString(row.FromState + " -> " + row.ToState)})
	} else {
		badges = append(badges, &model.WorkActionBadge{Key: "transition:" + row.TransitionKind.String(), Label: transitionKindLabel(row.TransitionKind), Tone: transitionKindTone(row.TransitionKind), Detail: optionalString(row.FromState + " -> " + row.ToState)})
	}
	if row.RequiresCloseout {
		badges = append(badges, &model.WorkActionBadge{Key: "transition:closeout_required", Label: "Closeout required", Tone: "warning", Detail: optionalString(row.Note)})
	}
	if row.VerificationState != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "transition:verification_" + row.VerificationState.String(), Label: transitionVerificationLabel(row.VerificationState.String()), Tone: transitionVerificationTone(row.VerificationState.String())})
	}
	if row.TransitionConfidence > 0 {
		badges = append(badges, &model.WorkActionBadge{Key: "transition:confidence", Label: "Detection confidence", Tone: "info", Detail: optionalString(row.ConfidenceBasis.String() + " " + strconv.FormatFloat(row.TransitionConfidence, 'f', 2, 64))})
	}
	return badges
}

func transitionKindLabel(kind workitemstatetransition.TransitionKind) string {
	switch kind {
	case workitemstatetransition.TransitionKindTerminalStateChange:
		return "Terminal transition"
	case workitemstatetransition.TransitionKindCoverageStateChange:
		return "Coverage changed"
	case workitemstatetransition.TransitionKindStateRefresh:
		return "State refreshed"
	default:
		return "State changed"
	}
}

func transitionKindTone(kind workitemstatetransition.TransitionKind) string {
	switch kind {
	case workitemstatetransition.TransitionKindTerminalStateChange:
		return "success"
	case workitemstatetransition.TransitionKindCoverageStateChange:
		return "warning"
	default:
		return "info"
	}
}

func transitionVerificationLabel(value string) string {
	switch value {
	case "closeout_required":
		return "Closeout required"
	case "source_verified":
		return "Source verified"
	case "human_verified":
		return "Human verified"
	default:
		return "Candidate"
	}
}

func transitionVerificationTone(value string) string {
	switch value {
	case "closeout_required":
		return "warning"
	case "source_verified", "human_verified":
		return "success"
	default:
		return "info"
	}
}
