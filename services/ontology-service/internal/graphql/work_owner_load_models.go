package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workOwnerLoadSnapshotModel(row *genent.WorkOwnerLoadSnapshot, topActions []*genent.WorkAction) *model.WorkOwnerLoadSnapshot {
	return &model.WorkOwnerLoadSnapshot{
		Key:                       row.Key,
		WorkstreamKey:             row.WorkstreamKey,
		SourceInstance:            row.SourceInstance,
		OwnerKey:                  row.OwnerKey,
		OwnerDisplayName:          optionalString(row.OwnerDisplayName),
		PersonKey:                 optionalPersonKey(row.Edges.Person),
		GeneratedAt:               optionalTime(row.GeneratedAt),
		LoadStatus:                row.LoadStatus.String(),
		ActionCount:               row.ActionCount,
		ProductActionCount:        row.ProductActionCount,
		ValidationLeadCount:       row.ValidationLeadCount,
		ModelOrRuleQaCount:        row.ModelOrRuleQaCount,
		CriticalOrHighCount:       row.CriticalOrHighCount,
		MaxPriorityScore:          row.MaxPriorityScore,
		AvgPriorityScore:          row.AvgPriorityScore,
		DecisionFollowupCount:     row.DecisionFollowupCount,
		ValidateSignalCount:       row.ValidateSignalCount,
		CiCheckFollowupCount:      row.CiCheckFollowupCount,
		ReviewWaitFollowupCount:   row.ReviewWaitFollowupCount,
		CoverageLimitedCount:      row.CoverageLimitedCount,
		AnonymousObservationCount: row.AnonymousObservationCount,
		NeedsHumanReviewCount:     row.NeedsHumanReviewCount,
		TopActionType:             optionalString(row.TopActionType),
		TopSubjects:               optionalString(row.TopSubjects),
		TopActions:                workOwnerLoadActionModels(topActions),
		RecommendedFocus:          optionalString(row.RecommendedFocus),
		FreshnessState:            row.FreshnessState.String(),
		Confidence:                row.Confidence,
		EvidenceRef:               optionalString(evidenceRef(row.Edges.LatestEvidence)),
		Evidence:                  workEvidenceSummary(row.Edges.LatestEvidence),
		Badges:                    workOwnerLoadBadges(row),
	}
}

func workOwnerLoadActionModels(rows []*genent.WorkAction) []*model.WorkAction {
	out := make([]*model.WorkAction, 0, len(rows))
	for _, row := range rows {
		out = append(out, workActionModel(row))
	}
	return out
}

func optionalPersonKey(row *genent.Person) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.Key)
}

func workOwnerLoadBadges(row *genent.WorkOwnerLoadSnapshot) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{Key: "owner_load:status", Label: ownerLoadStatusLabel(row.LoadStatus.String()), Tone: ownerLoadStatusTone(row.LoadStatus.String())},
	}
	if row.ProductActionCount > 0 {
		badges = append(badges, countBadge("owner_load:product_actions", "Product actions", "success", row.ProductActionCount))
	}
	if row.ValidationLeadCount > 0 {
		badges = append(badges, countBadge("owner_load:validation_leads", "Validation leads", "warning", row.ValidationLeadCount))
	}
	if row.CriticalOrHighCount > 0 {
		badges = append(badges, countBadge("owner_load:urgent_actions", "Urgent actions", "danger", row.CriticalOrHighCount))
	}
	if row.CiCheckFollowupCount > 0 {
		badges = append(badges, countBadge("owner_load:ci_checks", "CI checks", "warning", row.CiCheckFollowupCount))
	}
	if row.NeedsHumanReviewCount > 0 {
		badges = append(badges, countBadge("owner_load:needs_review", "Needs review", "warning", row.NeedsHumanReviewCount))
	}
	if row.CoverageLimitedCount > 0 {
		badges = append(badges, countBadge("owner_load:coverage_limited", "Coverage limited", "warning", row.CoverageLimitedCount))
	}
	if row.AnonymousObservationCount > 0 {
		badges = append(badges, countBadge("owner_load:anonymous_observations", "Anonymous observations", "warning", row.AnonymousObservationCount))
	}
	return badges
}

func ownerLoadStatusLabel(status string) string {
	switch status {
	case "overloaded":
		return "Overloaded"
	case "attention_required":
		return "Attention required"
	case "watch":
		return "Watch"
	case "clear":
		return "Clear"
	default:
		return "Unknown"
	}
}

func ownerLoadStatusTone(status string) string {
	switch status {
	case "overloaded", "attention_required":
		return "danger"
	case "watch":
		return "warning"
	case "clear":
		return "success"
	default:
		return "neutral"
	}
}
