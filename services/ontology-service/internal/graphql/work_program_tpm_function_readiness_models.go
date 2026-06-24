package graphql

import (
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramTPMFunctionReadinessModel(row *genent.WorkProgramTPMFunctionReadiness) *model.WorkProgramTpmFunctionReadiness {
	return &model.WorkProgramTpmFunctionReadiness{
		FunctionKey:           workProgramTPMFunctionReadinessDisplayKey(row),
		FunctionName:          row.FunctionName,
		ReadinessState:        row.ReadinessState,
		AutomationState:       row.AutomationState,
		HumanRequired:         row.HumanRequired,
		SupportingSignalCount: row.SupportingSignalCount,
		BlockingGateKeys:      workProgramUniqueStrings(splitLineList(row.BlockingGateKeys)),
		BlockingGates:         workProgramQualityGateModels(row.Edges.BlockingQualityGates),
		Detail:                row.Detail,
		RecommendedAction:     row.RecommendedAction,
	}
}

func workProgramTPMFunctionReadinessModels(rows []*genent.WorkProgramTPMFunctionReadiness) []*model.WorkProgramTpmFunctionReadiness {
	out := make([]*model.WorkProgramTpmFunctionReadiness, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramTPMFunctionReadinessModel(row))
	}
	return out
}

func workProgramTPMFunctionReadinessDisplayKey(row *genent.WorkProgramTPMFunctionReadiness) string {
	if row.FunctionKey != "" {
		return row.FunctionKey
	}
	if row.ExternalID != "" {
		if idx := strings.LastIndex(row.ExternalID, "|"); idx >= 0 && idx+1 < len(row.ExternalID) {
			return row.ExternalID[idx+1:]
		}
		return row.ExternalID
	}
	return row.Key
}
