package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workstreamHealthSnapshotModel(row *genent.WorkstreamHealthSnapshot) *model.WorkstreamHealthSnapshot {
	return &model.WorkstreamHealthSnapshot{
		Key:                               row.Key,
		SourceInstance:                    optionalString(row.SourceInstance),
		WorkstreamKey:                     row.WorkstreamKey,
		WorkstreamTitle:                   optionalString(workstreamHealthTitle(row)),
		GeneratedAt:                       optionalTime(row.GeneratedAt),
		OperatingStatus:                   row.OperatingStatus.String(),
		ActionItemCount:                   row.ActionItemCount,
		ProductActionCount:                row.ProductActionCount,
		ValidationLeadCount:               row.ValidationLeadCount,
		CriticalOrHighValidationLeadCount: row.CriticalOrHighValidationLeadCount,
		ModelOrRuleQaCount:                row.ModelOrRuleQaCount,
		CloseoutReviewCount:               row.CloseoutReviewCount,
		OwnerCount:                        row.OwnerCount,
		TopOwnerActionCount:               row.TopOwnerActionCount,
		FailingCheckPullRequestCount:      row.FailingCheckPrCount,
		OpenFailingCheckPullRequestCount:  row.OpenFailingCheckPrCount,
		SourceRepairCount:                 row.SourceRepairCount,
		CoverageLimitedCount:              row.CoverageLimitedCount,
		AnonymousObservationCount:         row.AnonymousObservationCount,
		TerminalTransitionCount:           row.TerminalTransitionCount,
		TerminalTransitionSubjects:        optionalString(row.TerminalTransitionSubjects),
		EtaForecastReady:                  row.EtaForecastReady,
		TruthLabelCoverage:                optionalString(row.TruthLabelCoverage),
		ActionabilityLabelCoverage:        optionalString(row.ActionabilityLabelCoverage),
		RecommendedCadenceFocus:           optionalString(row.RecommendedCadenceFocus),
		EvidenceRef:                       optionalString(evidenceRef(row.Edges.LatestEvidence)),
		Evidence:                          workEvidenceSummary(row.Edges.LatestEvidence),
		Badges:                            workstreamHealthBadges(row),
	}
}

func workstreamHealthTitle(row *genent.WorkstreamHealthSnapshot) string {
	if row.Edges.Workstream != nil && row.Edges.Workstream.Title != "" {
		return row.Edges.Workstream.Title
	}
	return row.WorkstreamKey
}

func workstreamHealthBadges(row *genent.WorkstreamHealthSnapshot) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{Key: "workstream_health:status", Label: workstreamHealthStatusLabel(row.OperatingStatus.String()), Tone: workstreamHealthStatusTone(row.OperatingStatus.String())},
	}
	if row.ProductActionCount > 0 {
		badges = append(badges, countBadge("workstream_health:product_actions", "Product actions", "success", row.ProductActionCount))
	}
	if row.ValidationLeadCount > 0 {
		badges = append(badges, countBadge("workstream_health:validation_leads", "Validation leads", "warning", row.ValidationLeadCount))
	}
	if row.CriticalOrHighValidationLeadCount > 0 {
		badges = append(badges, countBadge("workstream_health:urgent_validation", "Urgent validation", "danger", row.CriticalOrHighValidationLeadCount))
	}
	if row.OpenFailingCheckPrCount > 0 {
		badges = append(badges, countBadge("workstream_health:failing_checks", "Failing checks", "warning", row.OpenFailingCheckPrCount))
	}
	if row.SourceRepairCount > 0 {
		badges = append(badges, countBadge("workstream_health:source_repair", "Source repair", "danger", row.SourceRepairCount))
	}
	if row.CoverageLimitedCount > 0 {
		badges = append(badges, countBadge("workstream_health:coverage_limited", "Coverage limited", "warning", row.CoverageLimitedCount))
	}
	if row.TerminalTransitionCount > 0 {
		badges = append(badges, countBadge("workstream_health:terminal_transitions", "Terminal transitions", "info", row.TerminalTransitionCount))
	}
	if !row.EtaForecastReady {
		badges = append(badges, &model.WorkActionBadge{Key: "workstream_health:forecast_gated", Label: "Forecast gated", Tone: "warning"})
	}
	if row.AnonymousObservationCount > 0 {
		badges = append(badges, countBadge("workstream_health:anonymous_observations", "Anonymous observations", "warning", row.AnonymousObservationCount))
	}
	return badges
}

func workstreamHealthStatusLabel(status string) string {
	switch status {
	case "attention_required":
		return "Attention required"
	case "validation_required":
		return "Validation required"
	case "watch":
		return "Watch"
	case "clear":
		return "Clear"
	default:
		return "Unknown"
	}
}

func workstreamHealthStatusTone(status string) string {
	switch status {
	case "attention_required":
		return "danger"
	case "validation_required":
		return "warning"
	case "watch":
		return "info"
	case "clear":
		return "success"
	default:
		return "neutral"
	}
}
