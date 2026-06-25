package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workEvidenceSummary(row *genent.Evidence) *model.WorkEvidenceSummary {
	if row == nil {
		return nil
	}
	return &model.WorkEvidenceSummary{
		Key:              row.Key,
		Ref:              evidenceRef(row),
		ClaimKind:        row.ClaimKind.String(),
		ClaimTargetKind:  optionalString(row.ClaimTargetKind),
		ClaimField:       optionalString(row.ClaimField),
		RelationshipKind: optionalString(row.RelationshipKind),
		LocatorKind:      optionalString(row.LocatorKind),
		Locator:          optionalString(row.Locator),
		SourceSpanKey:    optionalString(row.SourceSpanKey),
		Ordinal:          row.Ordinal,
		SpanStart:        row.SpanStart,
		SpanEnd:          row.SpanEnd,
		SourceSystem:     optionalString(row.SourceSystem),
		SourceInstance:   optionalString(row.SourceInstance),
		ExternalKind:     optionalString(row.ExternalKind),
		ExternalID:       optionalString(row.ExternalID),
		SourceURL:        optionalString(row.SourceURL),
		ProofState:       row.ProofState.String(),
		FreshnessState:   row.FreshnessState.String(),
		Visibility:       row.Visibility.String(),
		Confidence:       row.Confidence,
		ObservedAt:       optionalTime(row.ObservedAt),
		Excerpt:          optionalString(row.Excerpt),
		ExcerptTruncated: row.ExcerptTruncated,
	}
}

func workActionEvidenceSummary(row *genent.WorkAction) *model.WorkEvidenceSummary {
	if row == nil {
		return nil
	}
	if row.Edges.LatestEvidence != nil {
		return workEvidenceSummary(row.Edges.LatestEvidence)
	}
	for _, observation := range row.Edges.Observations {
		if observation.Edges.LatestEvidence != nil {
			return workEvidenceSummary(observation.Edges.LatestEvidence)
		}
	}
	return nil
}
