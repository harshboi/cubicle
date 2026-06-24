package graphql

import (
	"strings"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphcontext"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func boundedGraphAssociationTypes(values []string) []domain.AssociationType {
	out := make([]domain.AssociationType, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, domain.AssociationType(value))
	}
	return out
}

func boundedGraphDepth(value *int) int {
	if value == nil {
		return 2
	}
	depth := *value
	if depth < 0 {
		return 2
	}
	if depth > 4 {
		return 4
	}
	return depth
}

func boundedGraphContextModel(value graphcontext.BoundedGraphContext) *model.BoundedGraphContext {
	return &model.BoundedGraphContext{
		ContextHash:    value.ContextHash,
		Seed:           boundedGraphRefModel(value.Seed),
		Depth:          value.Depth,
		LimitPerObject: value.LimitPerObject,
		ScopeMode:      value.ScopeMode,
		Coverage:       boundedGraphCoverageModel(value.Coverage),
		Guardrails:     append([]string(nil), value.Guardrails...),
		Objects:        boundedGraphObjectModels(value.Objects),
		Associations:   boundedGraphAssociationModels(value.Associations),
		Evidence:       boundedGraphEvidenceModels(value.Evidence),
	}
}

func boundedGraphRefModel(value graphcontext.Ref) *model.BoundedGraphRef {
	return &model.BoundedGraphRef{
		ObjectType: value.ObjectType,
		Key:        value.Key,
	}
}

func boundedGraphCoverageModel(value graphcontext.CoveragePolicy) *model.BoundedGraphCoverage {
	return &model.BoundedGraphCoverage{
		CoverageState:                value.CoverageState,
		AbsenceClaimsAllowed:         value.AbsenceClaimsAllowed,
		AbsenceClaimGateReason:       value.AbsenceClaimGateReason,
		AbsenceClaimAssociationTypes: append([]string(nil), value.AbsenceClaimAssociationTypes...),
		SourceSystem:                 optionalString(value.SourceSystem),
		SourceInstance:               optionalString(value.SourceInstance),
		CoverageWindowStart:          optionalString(value.CoverageWindowStart),
		CoverageWindowEnd:            optionalString(value.CoverageWindowEnd),
		Summary:                      optionalString(value.Summary),
	}
}

func boundedGraphObjectModels(values []graphcontext.Object) []*model.BoundedGraphObject {
	out := make([]*model.BoundedGraphObject, 0, len(values))
	for _, value := range values {
		out = append(out, &model.BoundedGraphObject{
			ObjectType:          value.ObjectType,
			Key:                 value.Key,
			Title:               value.Title,
			Source:              optionalString(value.Source),
			SourceInstance:      optionalString(value.SourceInstance),
			ExternalID:          optionalString(value.ExternalID),
			Visibility:          optionalString(value.Visibility),
			FreshnessState:      optionalString(value.FreshnessState),
			ProofState:          value.ProofState,
			ClaimAllowed:        value.ClaimAllowed,
			ClaimGateReason:     value.ClaimGateReason,
			SourceCoverageState: optionalString(value.SourceCoverageState),
			RankScore:           value.RankScore,
		})
	}
	return out
}

func boundedGraphAssociationModels(values []graphcontext.Association) []*model.BoundedGraphAssociation {
	out := make([]*model.BoundedGraphAssociation, 0, len(values))
	for _, value := range values {
		out = append(out, &model.BoundedGraphAssociation{
			Key:             value.Key,
			AssociationType: value.AssociationType,
			From:            boundedGraphRefModel(value.From),
			To:              boundedGraphRefModel(value.To),
			EvidenceKey:     optionalString(value.EvidenceKey),
			Confidence:      value.Confidence,
			Visibility:      optionalString(value.Visibility),
			FreshnessState:  optionalString(value.FreshnessState),
			ProofState:      value.ProofState,
			ClaimAllowed:    value.ClaimAllowed,
			ClaimGateReason: value.ClaimGateReason,
		})
	}
	return out
}

func boundedGraphEvidenceModels(values []graphcontext.Evidence) []*model.BoundedGraphEvidence {
	out := make([]*model.BoundedGraphEvidence, 0, len(values))
	for _, value := range values {
		out = append(out, &model.BoundedGraphEvidence{
			Key:            value.Key,
			Source:         optionalString(value.Source),
			SourceInstance: optionalString(value.SourceInstance),
			LocatorKind:    optionalString(value.LocatorKind),
			Visibility:     optionalString(value.Visibility),
			FreshnessState: optionalString(value.FreshnessState),
			Confidence:     value.Confidence,
		})
	}
	return out
}
