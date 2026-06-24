package graphql

import (
	"fmt"
	"strconv"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func boundedLimit(value *int, fallback int, max int) int {
	limit := fallback
	if value != nil {
		limit = *value
	}
	if limit <= 0 {
		limit = fallback
	}
	if limit > max {
		limit = max
	}
	return limit
}

func workLensWindowModel(row *genent.WorkLensWindow) *model.WorkLensWindow {
	lens := row.Edges.Lens
	lensKey := ""
	lensKind := ""
	targetKind := ""
	displayName := ""
	if lens != nil {
		lensKey = lens.Key
		lensKind = lens.WorkLensKind.String()
		targetKind = lens.LensTargetKind.String()
		displayName = lens.DisplayName
	}
	return &model.WorkLensWindow{
		Key:                row.Key,
		LensKey:            lensKey,
		LensKind:           lensKind,
		LensTargetKind:     targetKind,
		DisplayName:        displayName,
		WindowKind:         row.LensWindowKind.String(),
		ResultCount:        row.ResultCount,
		IsComplete:         row.IsComplete,
		LastIndexedAt:      optionalTime(row.LastIndexedAt),
		FreshnessState:     row.FreshnessState.String(),
		Visibility:         row.Visibility.String(),
		Confidence:         row.Confidence,
		Badges:             workLensWindowBadges(row),
		PullRequestResults: workLensPullRequestResultModels(row.Edges.PullRequestResults),
		TicketResults:      workLensTicketResultModels(row.Edges.TicketResults),
	}
}

func workLensPullRequestResultModels(rows []*genent.PullRequestLensResult) []*model.WorkLensPullRequestResult {
	out := make([]*model.WorkLensPullRequestResult, 0, len(rows))
	for _, row := range rows {
		pr := row.Edges.PullRequest
		out = append(out, &model.WorkLensPullRequestResult{
			Key:               lensResultKey("pull_request", row.ID, row.ExternalID),
			RelationKind:      row.RelationKind.String(),
			RankScore:         row.RankScore,
			FreshnessState:    row.FreshnessState.String(),
			Visibility:        row.Visibility.String(),
			Confidence:        row.Confidence,
			SubjectKey:        pullRequestDisplayKey(pr),
			Title:             optionalString(pullRequestTitle(pr)),
			State:             optionalString(pullRequestState(pr)),
			SourceURL:         optionalString(pullRequestSourceURL(pr, row.SourceURL)),
			RelatedTicketKeys: lensResultTicketKeys(pr),
			Badges:            workLensResultBadges(row.RelationKind.String(), row.FreshnessState.String(), row.RankScore),
		})
	}
	return out
}

func workLensTicketResultModels(rows []*genent.TicketLensResult) []*model.WorkLensTicketResult {
	out := make([]*model.WorkLensTicketResult, 0, len(rows))
	for _, row := range rows {
		ticket := row.Edges.Ticket
		out = append(out, &model.WorkLensTicketResult{
			Key:                    lensResultKey("ticket", row.ID, row.ExternalID),
			RelationKind:           row.RelationKind.String(),
			RankScore:              row.RankScore,
			FreshnessState:         row.FreshnessState.String(),
			Visibility:             row.Visibility.String(),
			Confidence:             row.Confidence,
			SubjectKey:             ticketDisplayKey(ticket),
			Title:                  optionalString(ticketTitle(ticket)),
			State:                  optionalString(ticketState(ticket)),
			SourceURL:              optionalString(ticketSourceURL(ticket, row.SourceURL)),
			RelatedPullRequestKeys: lensResultPullRequestKeys(ticket),
			Badges:                 workLensResultBadges(row.RelationKind.String(), row.FreshnessState.String(), row.RankScore),
		})
	}
	return out
}

func lensResultKey(targetKind string, id int, externalID string) string {
	if externalID != "" {
		return fmt.Sprintf("work-lens-result:%s:%s", targetKind, externalID)
	}
	return fmt.Sprintf("work-lens-result:%s:%d", targetKind, id)
}

func pullRequestTitle(row *genent.PullRequest) string {
	if row == nil {
		return ""
	}
	return row.Title
}

func pullRequestState(row *genent.PullRequest) string {
	if row == nil {
		return ""
	}
	return row.State.String()
}

func pullRequestSourceURL(row *genent.PullRequest, fallback string) string {
	if row != nil && row.SourceURL != "" {
		return row.SourceURL
	}
	return fallback
}

func ticketTitle(row *genent.Ticket) string {
	if row == nil {
		return ""
	}
	return row.Title
}

func ticketState(row *genent.Ticket) string {
	if row == nil {
		return ""
	}
	return row.Status.String()
}

func ticketSourceURL(row *genent.Ticket, fallback string) string {
	if row != nil && row.SourceURL != "" {
		return row.SourceURL
	}
	return fallback
}

func lensResultTicketKeys(row *genent.PullRequest) []string {
	keys := map[string]bool{}
	if row != nil {
		for _, ticket := range row.Edges.Tickets {
			if key := ticketDisplayKey(ticket); key != "" {
				keys[key] = true
			}
		}
	}
	return sortedKeys(keys)
}

func lensResultPullRequestKeys(row *genent.Ticket) []string {
	keys := map[string]bool{}
	if row != nil {
		for _, pr := range row.Edges.PullRequests {
			if key := pullRequestDisplayKey(pr); key != "" {
				keys[key] = true
			}
		}
	}
	return sortedKeys(keys)
}

func workLensWindowBadges(row *genent.WorkLensWindow) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		countBadge("lens:result_count", "Results", "info", row.ResultCount),
	}
	if row.IsComplete {
		badges = append(badges, &model.WorkActionBadge{Key: "lens:complete", Label: "Complete", Tone: "success"})
	} else {
		badges = append(badges, &model.WorkActionBadge{Key: "lens:partial", Label: "Partial", Tone: "warning"})
	}
	if row.FreshnessState.String() == "partial" {
		badges = append(badges, &model.WorkActionBadge{Key: "coverage:partial", Label: "Partial coverage", Tone: "warning"})
	}
	return badges
}

func workLensResultBadges(relationKind string, freshnessState string, rankScore float64) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{Key: "relation:" + relationKind, Label: relationKind, Tone: "info"},
	}
	if freshnessState != "" && freshnessState != "unknown" {
		badges = append(badges, &model.WorkActionBadge{Key: "freshness:" + freshnessState, Label: freshnessState, Tone: freshnessTone(freshnessState)})
	}
	if rankScore > 0 {
		badges = append(badges, &model.WorkActionBadge{Key: "rank:score", Label: "Rank score", Tone: "info", Detail: optionalString(strconv.FormatFloat(rankScore, 'f', 2, 64))})
	}
	return badges
}

func freshnessTone(value string) string {
	switch value {
	case "fresh":
		return "success"
	case "partial", "stale":
		return "warning"
	default:
		return "neutral"
	}
}
