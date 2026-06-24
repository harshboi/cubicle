package graphql

import (
	genent "cubicle/services/ontology-service/ent"
)

type workProgramBriefSnapshotData struct {
	GeneratedAt       *string
	OperatingStatus   string
	DecisionPressure  string
	ForecastState     string
	PrimaryRisk       *string
	ExecutiveSummary  string
	RecommendedFocus  string
	NextCadenceFocus  string
	CapabilityGaps    []string
	LatestEvidenceRef string
}

func workProgramBriefSnapshotDataModel(row *genent.WorkProgramBriefSnapshot) *workProgramBriefSnapshotData {
	refValue := evidenceRef(row.Edges.LatestEvidence)
	return &workProgramBriefSnapshotData{
		GeneratedAt:       optionalTime(row.GeneratedAt),
		OperatingStatus:   row.OperatingStatus,
		DecisionPressure:  row.DecisionPressure,
		ForecastState:     row.ForecastState,
		PrimaryRisk:       optionalString(row.PrimaryRisk),
		ExecutiveSummary:  row.ExecutiveSummary,
		RecommendedFocus:  row.RecommendedFocus,
		NextCadenceFocus:  row.NextCadenceFocus,
		CapabilityGaps:    workProgramUniqueStrings(splitLineList(row.CapabilityGaps)),
		LatestEvidenceRef: refValue,
	}
}
