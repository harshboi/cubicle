package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramAutomationReadinessModel(
	row *genent.WorkProgramAutomationReadiness,
	gates []*model.WorkProgramBriefQualityGate,
	evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed,
) *model.WorkProgramAutomationReadiness {
	return &model.WorkProgramAutomationReadiness{
		ReadinessState:        row.ReadinessState,
		ReadinessScore:        row.ReadinessScore,
		AutonomousActionReady: row.AutonomousActionReady,
		HumanReviewRequired:   row.HumanReviewRequired,
		SafeAutomationAreas:   workProgramUniqueStrings(splitLineList(row.SafeAutomationAreas)),
		HumanRequiredAreas:    workProgramUniqueStrings(splitLineList(row.HumanRequiredAreas)),
		Rationale:             row.Rationale,
		RequiredEvidence:      workProgramUniqueStrings(splitLineList(row.RequiredEvidence)),
		EvidenceWorkQueue:     evidenceNeeds,
		Gates:                 gates,
	}
}
