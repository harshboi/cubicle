package graphql

import (
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramBriefCaveatModel(row *genent.WorkProgramBriefCaveat) *model.WorkProgramBriefCaveat {
	refValue := row.EvidenceRef
	if refValue == "" {
		refValue = evidenceRef(row.Edges.LatestEvidence)
	}
	return &model.WorkProgramBriefCaveat{
		Key:               workProgramBriefCaveatDisplayKey(row),
		Severity:          row.Severity,
		Title:             row.Title,
		Detail:            row.Detail,
		RecommendedAction: optionalString(row.RecommendedAction),
		EvidenceRef:       optionalString(refValue),
		Evidence:          workEvidenceSummary(row.Edges.LatestEvidence),
	}
}

func workProgramBriefCaveatModels(rows []*genent.WorkProgramBriefCaveat) []*model.WorkProgramBriefCaveat {
	out := make([]*model.WorkProgramBriefCaveat, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramBriefCaveatModel(row))
	}
	return out
}

func workProgramBriefCaveatDisplayKey(row *genent.WorkProgramBriefCaveat) string {
	if row.CaveatKey != "" {
		return row.CaveatKey
	}
	if row.ExternalID != "" {
		if idx := strings.LastIndex(row.ExternalID, "|"); idx >= 0 && idx+1 < len(row.ExternalID) {
			return row.ExternalID[idx+1:]
		}
		return row.ExternalID
	}
	return row.Key
}
