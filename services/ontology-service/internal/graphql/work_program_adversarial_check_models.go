package graphql

import (
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramAdversarialCheckModel(row *genent.WorkProgramAdversarialCheck) *model.WorkProgramAdversarialCheck {
	evidenceRefs := splitLineList(row.EvidenceRefs)
	if ref := evidenceRef(row.Edges.LatestEvidence); ref != "" {
		evidenceRefs = append(evidenceRefs, ref)
	}
	return &model.WorkProgramAdversarialCheck{
		Key:               workProgramAdversarialCheckDisplayKey(row),
		CheckKind:         row.CheckKind,
		CheckState:        row.CheckState.String(),
		Severity:          row.Severity.String(),
		Title:             row.Title,
		Detail:            row.Detail,
		RecommendedAction: row.RecommendedAction,
		BlockingGateKeys:  workProgramUniqueStrings(splitLineList(row.BlockingGateKeys)),
		EvidenceRefs:      workProgramUniqueStrings(evidenceRefs),
	}
}

func workProgramAdversarialCheckModels(rows []*genent.WorkProgramAdversarialCheck) []*model.WorkProgramAdversarialCheck {
	out := make([]*model.WorkProgramAdversarialCheck, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramAdversarialCheckModel(row))
	}
	return out
}

func workProgramAdversarialCheckDisplayKey(row *genent.WorkProgramAdversarialCheck) string {
	if row.ExternalID != "" {
		if idx := strings.LastIndex(row.ExternalID, ":"); idx >= 0 && idx+1 < len(row.ExternalID) {
			return row.ExternalID[idx+1:]
		}
		return row.ExternalID
	}
	return row.Key
}

func splitLineList(value string) []string {
	out := []string{}
	for _, part := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '\n' || r == ','
	}) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
