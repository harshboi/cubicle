package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workitemstatesnapshot"
	"cubicle/services/ontology-service/internal/graphql/model"
	"strconv"
	"strings"
	"time"
)

func workItemStateSnapshotModel(row *genent.WorkItemStateSnapshot) *model.WorkItemStateSnapshot {
	return &model.WorkItemStateSnapshot{
		Key:                         row.Key,
		SourceInstance:              optionalString(row.SourceInstance),
		SubjectKind:                 row.SubjectKind.String(),
		SubjectKey:                  row.SubjectKey,
		SubjectTitle:                optionalString(workItemStateSnapshotSubjectTitle(row)),
		SubjectURL:                  optionalString(workItemStateSnapshotSubjectURL(row)),
		State:                       optionalString(row.State),
		ObservedAt:                  optionalTime(row.ObservedAt),
		CapturedAt:                  optionalTime(row.CapturedAt),
		SourceUpdatedAt:             optionalTimePtr(row.SourceUpdatedAt),
		AgeDays:                     optionalFloat(row.AgeDays),
		StaleDays:                   optionalFloat(row.StaleDays),
		CycleTimeDays:               optionalFloat(row.CycleTimeDays),
		RiskScore:                   row.RiskScore,
		RiskBand:                    row.RiskBand.String(),
		ForecastMethod:              optionalString(row.ForecastMethod),
		SourceCurrentCoverageState:  optionalString(row.SourceCurrentCoverageState),
		SourceCurrentDetailState:    optionalString(row.SourceCurrentDetailState),
		SourceCurrentIssueCodes:     optionalString(row.SourceCurrentIssueCodes),
		SourceCurrentIssueKinds:     optionalString(row.SourceCurrentIssueKinds),
		Priority:                    optionalString(row.Priority),
		LinkedPullRequestCount:      row.LinkedPrCount,
		FreshPullRequestLinkCount:   row.FreshPrLinkCount,
		PartialPullRequestLinkCount: row.PartialPrLinkCount,
		CommentCount:                row.CommentCount,
		ParticipantCount:            row.ParticipantCount,
		BlockerKeywordCount:         row.BlockerKeywordCount,
		EvidenceRef:                 optionalString(evidenceRef(row.Edges.LatestEvidence)),
		Evidence:                    workEvidenceSummary(row.Edges.LatestEvidence),
		Badges:                      workItemStateSnapshotBadges(row),
	}
}

func workItemStateSnapshotSubjectTitle(row *genent.WorkItemStateSnapshot) string {
	if row.Title != "" {
		return row.Title
	}
	if row.Edges.PullRequest != nil && row.Edges.PullRequest.Title != "" {
		return row.Edges.PullRequest.Title
	}
	if row.Edges.Ticket != nil && row.Edges.Ticket.Title != "" {
		return row.Edges.Ticket.Title
	}
	return ""
}

func workItemStateSnapshotSubjectURL(row *genent.WorkItemStateSnapshot) string {
	if row.Edges.PullRequest != nil && row.Edges.PullRequest.SourceURL != "" {
		return row.Edges.PullRequest.SourceURL
	}
	if row.Edges.Ticket != nil && row.Edges.Ticket.SourceURL != "" {
		return row.Edges.Ticket.SourceURL
	}
	return row.SourceURL
}

func workItemStateSnapshotBadges(row *genent.WorkItemStateSnapshot) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if row.State != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "snapshot:state", Label: row.State, Tone: snapshotStateTone(row.State)})
	}
	if row.RiskBand == workitemstatesnapshot.RiskBandCritical || row.RiskBand == workitemstatesnapshot.RiskBandHigh {
		badges = append(badges, &model.WorkActionBadge{Key: "snapshot:risk_" + row.RiskBand.String(), Label: snapshotRiskLabel(row.RiskBand), Tone: snapshotRiskTone(row.RiskBand), Detail: optionalString("score " + strconv.FormatFloat(row.RiskScore, 'f', 0, 64))})
	}
	if row.SourceCurrentCoverageState != "" || row.SourceCurrentDetailState != "" {
		detail := strings.TrimSpace(row.SourceCurrentCoverageState + " " + row.SourceCurrentDetailState)
		tone := "success"
		if strings.Contains(detail, "partial") || strings.Contains(detail, "failed") || strings.Contains(detail, "unavailable") {
			tone = "warning"
		}
		badges = append(badges, &model.WorkActionBadge{Key: "snapshot:coverage", Label: "Coverage", Tone: tone, Detail: optionalString(detail)})
	}
	if row.BlockerKeywordCount > 0 {
		badges = append(badges, countBadge("snapshot:blocker_keywords", "Blocker keywords", "warning", row.BlockerKeywordCount))
	}
	if row.PartialPrLinkCount > 0 {
		badges = append(badges, countBadge("snapshot:partial_pr_links", "Partial PR links", "warning", row.PartialPrLinkCount))
	}
	return badges
}

func optionalTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	return optionalTime(*value)
}

func snapshotRiskLabel(risk workitemstatesnapshot.RiskBand) string {
	switch risk {
	case workitemstatesnapshot.RiskBandCritical:
		return "Critical risk"
	case workitemstatesnapshot.RiskBandHigh:
		return "High risk"
	default:
		return "Risk"
	}
}

func snapshotStateTone(state string) string {
	switch strings.ToLower(state) {
	case "merged", "closed", "resolved", "done":
		return "success"
	case "open", "in progress":
		return "info"
	default:
		return "neutral"
	}
}

func snapshotRiskTone(risk workitemstatesnapshot.RiskBand) string {
	switch risk {
	case workitemstatesnapshot.RiskBandCritical:
		return "danger"
	case workitemstatesnapshot.RiskBandHigh:
		return "warning"
	default:
		return "info"
	}
}
