package graphql

import (
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramEvidenceNeedModel(row *genent.WorkProgramEvidenceNeed) *model.WorkProgramAutomationEvidenceNeed {
	return &model.WorkProgramAutomationEvidenceNeed{
		Key:                workProgramEvidenceNeedDisplayKey(row),
		GateKey:            row.GateKey,
		EvidenceKind:       row.EvidenceKind,
		Priority:           row.Priority.String(),
		TargetKind:         row.TargetKind,
		TargetKey:          optionalString(row.TargetKey),
		OwnerKey:           optionalString(row.OwnerKey),
		ActionKey:          optionalString(row.ActionKey),
		ActionState:        optionalString(row.ActionState),
		MetricKey:          optionalString(row.MetricKey),
		ExecutionState:     row.ExecutionState,
		BackingActionCount: row.BackingActionCount,
		CurrentCount:       row.CurrentCount,
		RequiredCount:      row.RequiredCount,
		MissingCount:       row.MissingCount,
		CurrentRate:        workProgramEvidenceNeedOptionalRate(row.CurrentRate, row.MetricKey),
		RequiredRate:       workProgramEvidenceNeedOptionalRate(row.RequiredRate, row.MetricKey),
		RecommendedAction:  row.RecommendedAction,
		NextExecutionStep:  row.NextExecutionStep,
		SourceURL:          optionalString(row.SourceURL),
		Evidence:           workEvidenceSummary(row.Edges.LatestEvidence),
		QualityGate:        workProgramEvidenceNeedQualityGateModel(row),
		Action:             workProgramEvidenceNeedActionModel(row),
	}
}

func workProgramEvidenceNeedModels(rows []*genent.WorkProgramEvidenceNeed) []*model.WorkProgramAutomationEvidenceNeed {
	out := make([]*model.WorkProgramAutomationEvidenceNeed, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramEvidenceNeedModel(row))
	}
	return out
}

func workProgramEvidenceNeedDisplayKey(row *genent.WorkProgramEvidenceNeed) string {
	if row.ExternalID != "" {
		if idx := strings.LastIndex(row.ExternalID, "|"); idx >= 0 && idx+1 < len(row.ExternalID) {
			return row.ExternalID[idx+1:]
		}
		if idx := strings.LastIndex(row.ExternalID, ":"); idx >= 0 && idx+1 < len(row.ExternalID) {
			return row.ExternalID[idx+1:]
		}
		return row.ExternalID
	}
	return row.Key
}

func workProgramEvidenceNeedQualityGateModel(row *genent.WorkProgramEvidenceNeed) *model.WorkProgramBriefQualityGate {
	if row == nil || row.Edges.QualityGate == nil {
		return nil
	}
	return workProgramQualityGateModel(row.Edges.QualityGate)
}

func workProgramEvidenceNeedActionModel(row *genent.WorkProgramEvidenceNeed) *model.WorkAction {
	if row == nil || row.Edges.WorkAction == nil {
		return nil
	}
	return workActionModel(row.Edges.WorkAction)
}

func workProgramEvidenceNeedOptionalRate(value float64, metricKey string) *float64 {
	switch metricKey {
	case "precision", "useful_signal", "actionability":
		return &value
	default:
		return nil
	}
}
