package graphql

import (
	"context"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/workresponsibility"
)

func workActionClaimPolicyWithResponsibilityValidation(policy workActionClaimPolicy, reason string) workActionClaimPolicy {
	policy.responsibilityValidationRequired = true
	policy.responsibilityValidationReason = firstNonempty(strings.TrimSpace(reason), "responsibility_validation_required")
	return policy
}

func workResponsibilityClaimPolicy(row *genent.WorkResponsibility) workActionClaimPolicy {
	if !workResponsibilityRequiresValidation(row) {
		return workActionClaimPolicy{}
	}
	return workActionClaimPolicyWithResponsibilityValidation(workActionClaimPolicy{}, workResponsibilityValidationGateReason(row))
}

func workResponsibilityRequiresValidation(row *genent.WorkResponsibility) bool {
	return row != nil &&
		(row.ResponsibilityState == workresponsibility.ResponsibilityStateCandidate ||
			row.PartyKind == workresponsibility.PartyKindUnassigned)
}

func workActionClaimPolicyForRow(key string, policies map[string]workActionClaimPolicy, fallback workActionClaimPolicy) workActionClaimPolicy {
	if policy, ok := policies[key]; ok {
		return policy
	}
	return fallback
}

func (r *queryResolver) workActionResponsibilityClaimPolicies(ctx context.Context, sourceFilter *string, rows []*genent.WorkAction, fallback workActionClaimPolicy) (map[string]workActionClaimPolicy, error) {
	out := map[string]workActionClaimPolicy{}
	if r.EntClient == nil || len(rows) == 0 {
		return out, nil
	}
	actionIDs := make([]int, 0, len(rows))
	actionKeys := make([]string, 0, len(rows))
	keyByID := map[int]string{}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.ID > 0 {
			actionIDs = append(actionIDs, row.ID)
			keyByID[row.ID] = row.Key
		}
		if strings.TrimSpace(row.Key) != "" {
			actionKeys = append(actionKeys, row.Key)
		}
	}
	predicates := workResponsibilityValidationActionPredicates(actionIDs, actionKeys)
	if len(predicates) == 0 {
		return out, nil
	}
	responsibilities, err := r.workResponsibilityValidationRowsForPredicates(ctx, sourceFilter, predicates...)
	if err != nil {
		return nil, err
	}
	for _, responsibility := range responsibilities {
		if responsibility == nil {
			continue
		}
		reason := workResponsibilityValidationGateReason(responsibility)
		if responsibility.WorkActionID > 0 {
			if key := keyByID[responsibility.WorkActionID]; key != "" {
				out[key] = workActionClaimPolicyWithResponsibilityValidation(fallback, reason)
			}
		}
		if responsibility.SubjectKind == workresponsibility.SubjectKindWorkAction && strings.TrimSpace(responsibility.SubjectKey) != "" {
			out[responsibility.SubjectKey] = workActionClaimPolicyWithResponsibilityValidation(fallback, reason)
		}
	}
	return out, nil
}

func (r *queryResolver) workProgramItemResponsibilityClaimPolicies(ctx context.Context, sourceFilter *string, rows []*genent.WorkProgramItem, fallback workActionClaimPolicy) (map[string]workActionClaimPolicy, error) {
	out := map[string]workActionClaimPolicy{}
	if r.EntClient == nil || len(rows) == 0 {
		return out, nil
	}
	itemIDs := make([]int, 0, len(rows))
	actionIDs := []int{}
	actionKeys := []string{}
	itemKeyByID := map[int]string{}
	itemKeysByActionID := map[int][]string{}
	itemKeysByActionKey := map[string][]string{}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.ID > 0 {
			itemIDs = append(itemIDs, row.ID)
			itemKeyByID[row.ID] = row.Key
		}
		if action := row.Edges.WorkAction; action != nil {
			if action.ID > 0 {
				actionIDs = append(actionIDs, action.ID)
				itemKeysByActionID[action.ID] = append(itemKeysByActionID[action.ID], row.Key)
			}
			if strings.TrimSpace(action.Key) != "" {
				actionKeys = append(actionKeys, action.Key)
				itemKeysByActionKey[action.Key] = append(itemKeysByActionKey[action.Key], row.Key)
			}
		}
	}
	predicates := []predicate.WorkResponsibility{}
	if len(itemIDs) > 0 {
		predicates = append(predicates, workresponsibility.WorkProgramItemIDIn(itemIDs...))
	}
	predicates = append(predicates, workResponsibilityValidationActionPredicates(actionIDs, actionKeys)...)
	if len(predicates) == 0 {
		return out, nil
	}
	responsibilities, err := r.workResponsibilityValidationRowsForPredicates(ctx, sourceFilter, predicates...)
	if err != nil {
		return nil, err
	}
	for _, responsibility := range responsibilities {
		if responsibility == nil {
			continue
		}
		reason := workResponsibilityValidationGateReason(responsibility)
		if responsibility.WorkProgramItemID > 0 {
			if key := itemKeyByID[responsibility.WorkProgramItemID]; key != "" {
				out[key] = workActionClaimPolicyWithResponsibilityValidation(fallback, reason)
			}
		}
		if responsibility.WorkActionID > 0 {
			for _, key := range itemKeysByActionID[responsibility.WorkActionID] {
				out[key] = workActionClaimPolicyWithResponsibilityValidation(fallback, reason)
			}
		}
		if responsibility.SubjectKind == workresponsibility.SubjectKindWorkAction && strings.TrimSpace(responsibility.SubjectKey) != "" {
			for _, key := range itemKeysByActionKey[responsibility.SubjectKey] {
				out[key] = workActionClaimPolicyWithResponsibilityValidation(fallback, reason)
			}
		}
	}
	return out, nil
}

func workResponsibilityValidationActionPredicates(actionIDs []int, actionKeys []string) []predicate.WorkResponsibility {
	predicates := []predicate.WorkResponsibility{}
	if len(actionIDs) > 0 {
		predicates = append(predicates, workresponsibility.WorkActionIDIn(actionIDs...))
	}
	if len(actionKeys) > 0 {
		predicates = append(predicates, workresponsibility.And(
			workresponsibility.SubjectKindEQ(workresponsibility.SubjectKindWorkAction),
			workresponsibility.SubjectKeyIn(actionKeys...),
		))
	}
	return predicates
}

func (r *queryResolver) workResponsibilityValidationRowsForPredicates(ctx context.Context, sourceFilter *string, predicates ...predicate.WorkResponsibility) ([]*genent.WorkResponsibility, error) {
	if len(predicates) == 0 {
		return []*genent.WorkResponsibility{}, nil
	}
	query := r.EntClient.WorkResponsibility.Query().
		Where(
			workresponsibility.SourceSystemEQ("cubicle_analytics"),
			workresponsibility.ExternalKindEQ("tpm_work_responsibility"),
			workresponsibility.Or(
				workresponsibility.ResponsibilityStateEQ(workresponsibility.ResponsibilityStateCandidate),
				workresponsibility.PartyKindEQ(workresponsibility.PartyKindUnassigned),
			),
			workresponsibility.Or(predicates...),
		)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workresponsibility.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	return query.All(ctx)
}

func workResponsibilityValidationGateReason(row *genent.WorkResponsibility) string {
	if row == nil {
		return "responsibility_validation_required"
	}
	if row.PartyKind == workresponsibility.PartyKindUnassigned {
		return firstNonempty(row.ResponsibilityStateReason, "responsibility_unassigned")
	}
	if row.ResponsibilityState == workresponsibility.ResponsibilityStateCandidate {
		return firstNonempty(row.ResponsibilityStateReason, "responsibility_candidate_requires_validation")
	}
	return firstNonempty(row.ResponsibilityStateReason, "responsibility_validation_required")
}
