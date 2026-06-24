package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/internal/graphql/model"
)

func workInsightReviewModel(row *genent.WorkInsightReview) *model.WorkInsightReview {
	insight := row.Edges.Insight
	if insight != nil {
		insightCopy := *insight
		insightCopy.Edges = insight.Edges
		insightCopy.Edges.Reviews = []*genent.WorkInsightReview{row}
		insight = &insightCopy
	}
	return &model.WorkInsightReview{
		Key:                 row.Key,
		SourceInstance:      optionalString(row.SourceInstance),
		SourceSystem:        row.SourceSystem,
		ExternalKind:        row.ExternalKind,
		ReviewKind:          row.ReviewKind.String(),
		ReviewState:         row.ReviewState.String(),
		TruthLabel:          row.TruthLabel.String(),
		ActionabilityLabel:  row.ActionabilityLabel.String(),
		LabelQuality:        row.LabelQuality.String(),
		MeasurementEligible: isGoldMeasurementReview(row),
		ReviewerKind:        row.ReviewerKind.String(),
		ReviewerKey:         optionalString(row.ReviewerKey),
		OwnerKey:            optionalString(row.OwnerKey),
		LabelSet:            optionalString(row.LabelSet),
		ReviewNextAction:    optionalString(row.NextAction),
		ReviewRationale:     optionalString(row.Rationale),
		ReviewedAt:          optionalTime(row.ReviewedAt),
		Insight:             workInsightSummaryModel(insight),
		Badges:              workInsightReviewBadges(row),
	}
}

func workInsightReviewBadges(row *genent.WorkInsightReview) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if isGoldMeasurementReview(row) {
		badges = append(badges, &model.WorkActionBadge{
			Key:   "review:measurement_eligible",
			Label: "Measurement label",
			Tone:  "success",
		})
	} else if row.MeasurementEligible || row.LabelQuality != workinsightreview.LabelQualityUnknown {
		badges = append(badges, &model.WorkActionBadge{
			Key:    "review:not_measurement",
			Label:  "Non-measurement label",
			Tone:   "info",
			Detail: optionalString(row.LabelQuality.String()),
		})
	}
	if row.ReviewKind == workinsightreview.ReviewKindTriageRequest && row.ReviewState == workinsightreview.ReviewStateRequested {
		badges = append(badges, &model.WorkActionBadge{
			Key:   "review:queue",
			Label: "Review queue",
			Tone:  "warning",
		})
	}
	if badge := reviewStateBadge(row); badge != nil {
		badges = append(badges, badge)
	}
	if badge := truthLabelBadge(row); badge != nil {
		badges = append(badges, badge)
	}
	if badge := actionabilityLabelBadge(row); badge != nil {
		badges = append(badges, badge)
	}
	if row.LabelQuality != "" && row.LabelQuality != workinsightreview.LabelQualityUnknown {
		badges = append(badges, &model.WorkActionBadge{
			Key:    "label_quality:" + row.LabelQuality.String(),
			Label:  "Label quality",
			Tone:   "info",
			Detail: optionalString(row.LabelQuality.String()),
		})
	}
	return badges
}

func workInsightSummaryModel(row *genent.WorkInsight) *model.WorkInsightSummary {
	if row == nil {
		return &model.WorkInsightSummary{
			Key:         "",
			InsightKind: "",
			Severity:    "",
			SubjectKind: "",
			SubjectKey:  "",
			Title:       "",
			Badges:      []*model.WorkActionBadge{},
		}
	}
	review := bestWorkInsightReview(row.Edges.Reviews)
	return &model.WorkInsightSummary{
		Key:                 row.Key,
		InsightKind:         row.InsightKind.String(),
		Severity:            row.Severity.String(),
		SubjectKind:         row.SubjectKind.String(),
		SubjectKey:          row.SubjectKey,
		Title:               row.Title,
		Details:             optionalString(row.Details),
		RecommendedAction:   optionalString(row.RecommendedAction),
		ModelMethod:         optionalString(row.ModelMethod),
		Score:               row.Score,
		ScoreExplanation:    optionalString(row.ScoreExplanation),
		Confidence:          row.Confidence,
		SourceURL:           optionalString(row.SourceURL),
		EvidenceRef:         optionalString(evidenceRef(row.Edges.LatestEvidence)),
		Evidence:            workEvidenceSummary(row.Edges.LatestEvidence),
		EvidenceExcerpt:     optionalString(workInsightEvidenceExcerpt(row)),
		ReviewKind:          optionalReviewKind(review),
		ReviewState:         optionalReviewState(review),
		TruthLabel:          optionalTruthLabel(review),
		ActionabilityLabel:  optionalActionabilityLabel(review),
		LabelQuality:        optionalLabelQuality(review),
		MeasurementEligible: isGoldMeasurementReview(review),
		ReviewerKind:        optionalReviewerKind(review),
		ReviewerKey:         optionalReviewerKey(review),
		LabelSet:            optionalLabelSet(review),
		ReviewNextAction:    optionalReviewNextAction(review),
		ReviewRationale:     optionalReviewRationale(review),
		Badges:              workInsightSummaryBadges(row, review),
	}
}
