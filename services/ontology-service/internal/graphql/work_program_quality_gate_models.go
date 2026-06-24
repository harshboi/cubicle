package graphql

import (
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramQualityGateModel(row *genent.WorkProgramQualityGate) *model.WorkProgramBriefQualityGate {
	return &model.WorkProgramBriefQualityGate{
		Key:               workProgramQualityGateDisplayKey(row),
		GateState:         row.GateState,
		Blocking:          row.Blocking,
		Detail:            row.Detail,
		RecommendedAction: optionalString(row.RecommendedAction),
	}
}

func workProgramQualityGateModels(rows []*genent.WorkProgramQualityGate) []*model.WorkProgramBriefQualityGate {
	out := make([]*model.WorkProgramBriefQualityGate, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramQualityGateModel(row))
	}
	return out
}

func workProgramQualityGateDisplayKey(row *genent.WorkProgramQualityGate) string {
	if row.GateKey != "" {
		return row.GateKey
	}
	if row.ExternalID != "" {
		if idx := strings.LastIndex(row.ExternalID, "|"); idx >= 0 && idx+1 < len(row.ExternalID) {
			return row.ExternalID[idx+1:]
		}
		return row.ExternalID
	}
	return row.Key
}
