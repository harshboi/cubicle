package graphql

import (
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramRiskDriverModel(row *genent.WorkProgramRiskDriver) *model.WorkProgramBriefRiskDriver {
	return &model.WorkProgramBriefRiskDriver{
		Key:               workProgramRiskDriverDisplayKey(row),
		DriverKind:        row.DriverKind,
		SubjectKey:        optionalString(row.SubjectKey),
		Title:             row.Title,
		Status:            row.Status,
		RecommendedAction: optionalString(row.RecommendedAction),
		EvidenceRef:       optionalString(row.EvidenceRef),
		Evidence:          workEvidenceSummary(row.Edges.LatestEvidence),
		RankScore:         row.RankScore,
		Badges:            workProgramRiskDriverBadges(row),
	}
}

func workProgramRiskDriverModels(rows []*genent.WorkProgramRiskDriver) []*model.WorkProgramBriefRiskDriver {
	out := make([]*model.WorkProgramBriefRiskDriver, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramRiskDriverModel(row))
	}
	return out
}

func workProgramRiskDriverDisplayKey(row *genent.WorkProgramRiskDriver) string {
	if row.DriverKey != "" {
		return row.DriverKey
	}
	if row.ExternalID != "" {
		if idx := strings.LastIndex(row.ExternalID, "|"); idx >= 0 && idx+1 < len(row.ExternalID) {
			return row.ExternalID[idx+1:]
		}
		return row.ExternalID
	}
	return row.Key
}

func workProgramRiskDriverBadges(row *genent.WorkProgramRiskDriver) []*model.WorkActionBadge {
	keys := splitLineList(row.BadgeKeys)
	labels := splitLineList(row.BadgeLabels)
	tones := splitLineList(row.BadgeTones)
	details := splitLineList(row.BadgeDetails)
	badges := make([]*model.WorkActionBadge, 0, len(keys))
	for idx, key := range keys {
		if key == "" {
			continue
		}
		label := valueAt(labels, idx)
		if label == "" {
			label = key
		}
		tone := valueAt(tones, idx)
		if tone == "" {
			tone = "info"
		}
		badges = append(badges, &model.WorkActionBadge{
			Key:    key,
			Label:  label,
			Tone:   tone,
			Detail: optionalString(valueAt(details, idx)),
		})
	}
	return badges
}

func valueAt(values []string, idx int) string {
	if idx < 0 || idx >= len(values) {
		return ""
	}
	return values[idx]
}
