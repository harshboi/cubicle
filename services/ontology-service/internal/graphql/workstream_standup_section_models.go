package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workstreamStandupSectionModel(row *genent.WorkstreamStandupSection) *model.WorkstreamStandupSection {
	evidence := row.EvidenceRef
	if evidence == "" {
		evidence = evidenceRef(row.Edges.LatestEvidence)
	}
	var action *model.WorkAction
	if row.Edges.WorkAction != nil {
		action = workActionModel(row.Edges.WorkAction)
	}
	return &model.WorkstreamStandupSection{
		SourceInstance:    optionalString(row.SourceInstance),
		GeneratedAt:       optionalTime(row.GeneratedAt),
		SectionRank:       row.SectionRank,
		SectionKind:       row.SectionKind.String(),
		Urgency:           row.Urgency.String(),
		FreshnessState:    row.FreshnessState.String(),
		Confidence:        row.Confidence,
		OwnerKey:          optionalString(row.OwnerKey),
		SubjectKind:       row.SubjectKind.String(),
		SubjectObjectType: optionalString(row.SubjectObjectType),
		SubjectKey:        optionalString(row.SubjectKey),
		ActionType:        optionalString(row.ActionType),
		StatusSignal:      optionalString(row.StatusSignal),
		Summary:           row.Summary,
		RecommendedAction: row.RecommendedAction,
		EvidenceRef:       optionalString(evidence),
		Evidence:          workEvidenceSummary(row.Edges.LatestEvidence),
		Action:            action,
	}
}
