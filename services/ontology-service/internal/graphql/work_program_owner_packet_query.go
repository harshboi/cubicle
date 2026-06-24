package graphql

import (
	"fmt"
	"strings"

	"cubicle/services/ontology-service/internal/graphql/model"
)

func firstWorkProgramOwnerPacketActionFocus(actions []*model.WorkAction, evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed) *string {
	for _, action := range actions {
		if action == nil || action.RecommendedAction == nil {
			continue
		}
		if value := strings.TrimSpace(*action.RecommendedAction); value != "" {
			return &value
		}
	}
	for _, need := range evidenceNeeds {
		if need == nil {
			continue
		}
		if value := strings.TrimSpace(need.RecommendedAction); value != "" {
			return &value
		}
	}
	return nil
}

func workProgramOwnerPacketHumanRequired(loadStatus string, totalActionCount int, returnedActionCount int, evidenceNeedCount int) bool {
	if evidenceNeedCount > 0 || totalActionCount > 0 || returnedActionCount > 0 {
		return true
	}
	switch strings.TrimSpace(loadStatus) {
	case "watch", "attention_required", "overloaded":
		return true
	default:
		return false
	}
}

func workProgramOwnerPacketAutomationSummary(ownerKey string, actionState string, actionCount int, evidenceNeedCount int, recommendedFocus *string) string {
	if actionCount == 0 && evidenceNeedCount == 0 {
		return fmt.Sprintf("%s has no current %s TPM actions or evidence needs.", ownerKey, actionState)
	}
	summary := fmt.Sprintf("%s has %d %s TPM action(s) and %d evidence need(s).", ownerKey, actionCount, actionState, evidenceNeedCount)
	if recommendedFocus == nil {
		return summary
	}
	if focus := strings.TrimSpace(*recommendedFocus); focus != "" {
		return summary + " " + focus
	}
	return summary
}
