package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workforecastevaluation"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/internal/graphql/model"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type workActionOwnerRollupValues struct {
	ownerKey            string
	ownerSource         string
	actionCount         int
	productActionCount  int
	validationLeadCount int
	nowCount            int
	highPriorityCount   int
	maxRankScore        float64
	rows                []*genent.WorkAction
}

type workActionClaimPolicy struct {
	etaForecastReady                 bool
	etaReadinessGateReason           string
	etaReadinessContextKnown         bool
	forecastEvaluatedAt              string
	responsibilityValidationRequired bool
	responsibilityValidationReason   string
}

var (
	medianBaselineMAEPattern = regexp.MustCompile(`median baseline MAE ([0-9]+(?:\.[0-9]+)?)d`)
	heuristicMAEPattern      = regexp.MustCompile(`heuristic MAE ([0-9]+(?:\.[0-9]+)?)d`)
	randomForestMAEPattern   = regexp.MustCompile(`random forest MAE ([0-9]+(?:\.[0-9]+)?)d`)
	bestBacktestModelPattern = regexp.MustCompile(`best K-fold model ([A-Za-z0-9_-]+)`)
	forecastBlockerPattern   = regexp.MustCompile(`primary blocker ([A-Za-z0-9_:-]+)`)
)

const (
	forecastEvaluationKindSourceEventKfold   = "source_event_as_of_kfold"
	forecastEvaluationKindSourceEventHoldout = "source_event_as_of_chronological_holdout"
)

func workActionModel(row *genent.WorkAction) *model.WorkAction {
	return workActionModelWithClaimPolicy(row, workActionClaimPolicy{})
}

func workActionModelWithForecastReadiness(row *genent.WorkAction, readiness *model.WorkForecastReadiness) *model.WorkAction {
	return workActionModelWithClaimPolicy(row, workActionClaimPolicyForForecastReadiness(readiness))
}

func workActionModelWithClaimPolicy(row *genent.WorkAction, claimPolicy workActionClaimPolicy) *model.WorkAction {
	return &model.WorkAction{
		Key:                    row.Key,
		SourceInstance:         optionalString(row.SourceInstance),
		ActionType:             row.ActionType.String(),
		ActionState:            row.ActionState.String(),
		DecisionState:          row.DecisionState.String(),
		Decision:               optionalString(row.Decision),
		DecisionReason:         optionalString(row.DecisionReason),
		SubjectKind:            row.SubjectKind.String(),
		SubjectKey:             row.SubjectKey,
		SubjectTitle:           optionalString(workActionSubjectTitle(row)),
		SubjectURL:             optionalString(workActionSubjectURL(row)),
		RelatedTicketKeys:      workActionRelatedTicketKeys(row),
		RelatedPullRequestKeys: workActionRelatedPullRequestKeys(row),
		OwnerKey:               optionalString(row.OwnerKey),
		OwnerSource:            optionalString(row.OwnerSource),
		DueBucket:              row.DueBucket.String(),
		RankScore:              row.RankScore,
		SourceURL:              optionalString(row.SourceURL),
		DecisionNeeded:         optionalString(workActionDecisionNeeded(row)),
		RecommendedAction:      optionalString(standupRecommendedAction(row)),
		ClaimUse:               workActionClaimUse(row, claimPolicy),
		ClaimGateReason:        workActionClaimGateReason(row, claimPolicy),
		ProductActionAllowed:   workActionProductActionAllowedWithPolicy(row, claimPolicy),
		AbsenceClaimAllowed:    workActionAbsenceClaimAllowed(row),
		EtaClaimAllowed:        workActionETAClaimAllowedWithPolicy(row, claimPolicy),
		EvidenceRef:            optionalString(workActionEvidenceRef(row)),
		Evidence:               workActionEvidenceSummary(row),
		Badges:                 workActionBadges(row, claimPolicy),
		Observations:           workActionObservationModels(row.Edges.Observations),
		SourceInsights:         workInsightSummaryModels(row.Edges.SourceInsights),
	}
}

func workActionClaimPolicyForForecastReadiness(readiness *model.WorkForecastReadiness) workActionClaimPolicy {
	if readiness == nil {
		return workActionClaimPolicy{}
	}
	contextKnown := readiness.EtaForecastReady ||
		readiness.TypedEvaluationCount > 0 ||
		readiness.ReadinessState != "" && readiness.ReadinessState != "unknown" ||
		readiness.EtaReadinessBlockingReason != nil ||
		readiness.ReadinessReason != nil ||
		readiness.Detail != nil
	return workActionClaimPolicy{
		etaForecastReady:         readiness.EtaForecastReady,
		etaReadinessGateReason:   forecastReadinessGateReason(readiness),
		etaReadinessContextKnown: contextKnown,
		forecastEvaluatedAt:      pointerString(readiness.EvaluatedAt),
	}
}

func forecastReadinessGateReason(readiness *model.WorkForecastReadiness) string {
	if readiness == nil {
		return ""
	}
	return firstNonempty(
		pointerString(readiness.EtaReadinessBlockingReason),
		pointerString(readiness.ReadinessReason),
		pointerString(readiness.Detail),
		readiness.ReadinessState,
	)
}

func workActionObservationModels(rows []*genent.WorkActionObservation) []*model.WorkActionObservation {
	out := make([]*model.WorkActionObservation, 0, len(rows))
	for _, row := range rows {
		out = append(out, &model.WorkActionObservation{
			Key:                           row.Key,
			ObservationKind:               row.ObservationKind.String(),
			SourceCoverageState:           optionalString(row.SourceCoverageState),
			AuthState:                     optionalString(row.AuthState),
			CurrentState:                  optionalString(row.CurrentState),
			CiSignal:                      optionalString(row.CiSignal),
			CiRequiredCheckCoverageState:  optionalString(row.CiRequiredCheckCoverageState),
			CiRequiredCheckMatchState:     optionalString(row.CiRequiredCheckMatchState),
			CiRequiredContextCount:        row.CiRequiredContextCount,
			CiFailingRequiredContextCount: row.CiFailingRequiredContextCount,
			CiPendingRequiredContextCount: row.CiPendingRequiredContextCount,
			CiMissingRequiredContextCount: row.CiMissingRequiredContextCount,
			CiFailingRequiredContexts:     optionalString(row.CiFailingRequiredContexts),
			CiPendingRequiredContexts:     optionalString(row.CiPendingRequiredContexts),
			CiMissingRequiredContexts:     optionalString(row.CiMissingRequiredContexts),
			CiFailingContextCount:         row.CiFailingContextCount,
			CiPendingContextCount:         row.CiPendingContextCount,
			CiFailingContexts:             optionalString(row.CiFailingContexts),
			CiPendingContexts:             optionalString(row.CiPendingContexts),
			SupportsAction:                row.SupportsAction,
			ObservedAt:                    optionalTime(row.ObservedAt),
			SourceURL:                     optionalString(row.SourceURL),
		})
	}
	return out
}

func workInsightSummaryModels(rows []*genent.WorkInsight) []*model.WorkInsightSummary {
	out := make([]*model.WorkInsightSummary, 0, len(rows))
	for _, row := range rows {
		review := bestWorkInsightReview(row.Edges.Reviews)
		out = append(out, &model.WorkInsightSummary{
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
		})
	}
	return out
}

func bestWorkInsightReview(rows []*genent.WorkInsightReview) *genent.WorkInsightReview {
	var best *genent.WorkInsightReview
	for _, row := range rows {
		if best == nil || workInsightReviewRank(row) > workInsightReviewRank(best) {
			best = row
		}
	}
	return best
}

func workInsightReviewRank(row *genent.WorkInsightReview) int {
	if row == nil {
		return -1
	}
	rank := 0
	if isGoldMeasurementReview(row) {
		rank += 10_000
	}
	if row.ReviewerKind == workinsightreview.ReviewerKindHuman {
		rank += 2_000
	}
	switch row.LabelQuality {
	case workinsightreview.LabelQualityGold:
		rank += 1_000
	case workinsightreview.LabelQualitySmoke:
		rank += 300
	case workinsightreview.LabelQualityCandidate:
		rank += 100
	}
	switch row.ReviewKind {
	case workinsightreview.ReviewKindHumanAssessment:
		rank += 500
	case workinsightreview.ReviewKindEvaluationLabel:
		rank += 300
	case workinsightreview.ReviewKindTriageRequest:
		rank += 50
	}
	switch row.ReviewState {
	case workinsightreview.ReviewStateAccepted, workinsightreview.ReviewStateResolved, workinsightreview.ReviewStateDismissed:
		rank += 200
	case workinsightreview.ReviewStateNeedsMoreData:
		rank += 150
	case workinsightreview.ReviewStateRequested:
		rank += 20
	}
	if !row.ReviewedAt.IsZero() {
		rank += int(row.ReviewedAt.Unix() % 100)
	} else if !row.UpdatedAt.IsZero() {
		rank += int(row.UpdatedAt.Unix() % 100)
	}
	return rank
}

func workInsightEvidenceExcerpt(row *genent.WorkInsight) string {
	if row == nil || row.Edges.LatestEvidence == nil {
		return ""
	}
	return row.Edges.LatestEvidence.Excerpt
}

func optionalReviewKind(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.ReviewKind.String())
}

func optionalReviewState(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.ReviewState.String())
}

func optionalTruthLabel(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.TruthLabel.String())
}

func optionalActionabilityLabel(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.ActionabilityLabel.String())
}

func optionalLabelQuality(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.LabelQuality.String())
}

func optionalReviewerKind(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.ReviewerKind.String())
}

func optionalReviewerKey(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.ReviewerKey)
}

func optionalLabelSet(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.LabelSet)
}

func optionalReviewNextAction(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.NextAction)
}

func optionalReviewRationale(row *genent.WorkInsightReview) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.Rationale)
}

func workInsightSummaryBadges(row *genent.WorkInsight, review *genent.WorkInsightReview) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{Key: "insight:" + row.InsightKind.String(), Label: row.InsightKind.String(), Tone: "info"},
	}
	if row.ProducerState != "" && row.ProducerState.String() != "current" {
		badges = append(badges, &model.WorkActionBadge{Key: "insight:stale", Label: "Stale insight", Tone: "warning", Detail: optionalString(row.ProducerState.String())})
	}
	if review == nil {
		return append(badges, &model.WorkActionBadge{Key: "review:none", Label: "No review label", Tone: "warning"})
	}
	if isGoldMeasurementReview(review) {
		detail := review.LabelQuality.String()
		if review.ReviewerKind != "" || review.ReviewerKey != "" {
			detail = strings.TrimSpace(detail + " " + review.ReviewerKind.String() + ":" + review.ReviewerKey)
		}
		badges = append(badges, &model.WorkActionBadge{Key: "review:measurement_eligible", Label: "Measurement label", Tone: "success", Detail: optionalString(detail)})
	} else if review.MeasurementEligible || review.LabelQuality != workinsightreview.LabelQualityUnknown {
		badges = append(badges, &model.WorkActionBadge{Key: "review:not_measurement", Label: "Non-measurement label", Tone: "info", Detail: optionalString(review.LabelQuality.String())})
	}
	if review.ReviewerKind != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "reviewer:" + review.ReviewerKind.String(), Label: review.ReviewerKind.String(), Tone: "info", Detail: optionalString(review.ReviewerKey)})
	}
	badges = append(badges, reviewStateBadge(review))
	if review.TruthLabel != workinsightreview.TruthLabelUnknown {
		badges = append(badges, truthLabelBadge(review))
	}
	if review.ActionabilityLabel != workinsightreview.ActionabilityLabelUnknown {
		badges = append(badges, actionabilityLabelBadge(review))
	}
	return badges
}

func reviewStateBadge(row *genent.WorkInsightReview) *model.WorkActionBadge {
	switch row.ReviewState {
	case workinsightreview.ReviewStateAccepted, workinsightreview.ReviewStateResolved:
		return &model.WorkActionBadge{Key: "review:" + row.ReviewState.String(), Label: row.ReviewState.String(), Tone: "success"}
	case workinsightreview.ReviewStateDismissed:
		return &model.WorkActionBadge{Key: "review:dismissed", Label: "Dismissed", Tone: "neutral"}
	case workinsightreview.ReviewStateNeedsMoreData:
		return &model.WorkActionBadge{Key: "review:needs_more_data", Label: "Needs more data", Tone: "warning"}
	default:
		return &model.WorkActionBadge{Key: "review:requested", Label: "Review requested", Tone: "warning"}
	}
}

func truthLabelBadge(row *genent.WorkInsightReview) *model.WorkActionBadge {
	switch row.TruthLabel {
	case workinsightreview.TruthLabelTruePositive:
		return &model.WorkActionBadge{Key: "truth:true_positive", Label: "True positive", Tone: "success"}
	case workinsightreview.TruthLabelFalsePositive:
		return &model.WorkActionBadge{Key: "truth:false_positive", Label: "False positive", Tone: "neutral"}
	case workinsightreview.TruthLabelPartial:
		return &model.WorkActionBadge{Key: "truth:partial", Label: "Partial", Tone: "warning"}
	default:
		return &model.WorkActionBadge{Key: "truth:unknown", Label: "Truth unknown", Tone: "warning"}
	}
}

func actionabilityLabelBadge(row *genent.WorkInsightReview) *model.WorkActionBadge {
	switch row.ActionabilityLabel {
	case workinsightreview.ActionabilityLabelActionable:
		return &model.WorkActionBadge{Key: "actionability:actionable", Label: "Actionable", Tone: "success"}
	case workinsightreview.ActionabilityLabelNeedsOwner:
		return &model.WorkActionBadge{Key: "actionability:needs_owner", Label: "Needs owner", Tone: "warning"}
	case workinsightreview.ActionabilityLabelNotActionable:
		return &model.WorkActionBadge{Key: "actionability:not_actionable", Label: "Not actionable", Tone: "neutral"}
	default:
		return &model.WorkActionBadge{Key: "actionability:unknown", Label: "Actionability unknown", Tone: "warning"}
	}
}

func workActionSubjectTitle(row *genent.WorkAction) string {
	if row.Edges.PullRequest != nil && row.Edges.PullRequest.Title != "" {
		return row.Edges.PullRequest.Title
	}
	if row.Edges.Ticket != nil && row.Edges.Ticket.Title != "" {
		return row.Edges.Ticket.Title
	}
	return firstInsightTitle(row)
}

func workActionSubjectURL(row *genent.WorkAction) string {
	if row.Edges.PullRequest != nil && row.Edges.PullRequest.SourceURL != "" {
		return row.Edges.PullRequest.SourceURL
	}
	if row.Edges.Ticket != nil && row.Edges.Ticket.SourceURL != "" {
		return row.Edges.Ticket.SourceURL
	}
	if row.SourceURL != "" {
		return row.SourceURL
	}
	for _, insight := range row.Edges.SourceInsights {
		if insight.SourceURL != "" {
			return insight.SourceURL
		}
	}
	for _, observation := range row.Edges.Observations {
		if observation.SourceURL != "" {
			return observation.SourceURL
		}
	}
	return ""
}

func workActionRelatedTicketKeys(row *genent.WorkAction) []string {
	keys := map[string]bool{}
	if row.SubjectKind == workaction.SubjectKindTicket && row.SubjectKey != "" {
		keys[row.SubjectKey] = true
	}
	if row.Edges.Ticket != nil {
		if key := ticketDisplayKey(row.Edges.Ticket); key != "" {
			keys[key] = true
		}
	}
	if row.Edges.PullRequest != nil {
		for _, ticket := range row.Edges.PullRequest.Edges.Tickets {
			if key := ticketDisplayKey(ticket); key != "" {
				keys[key] = true
			}
		}
	}
	return sortedKeys(keys)
}

func workActionRelatedPullRequestKeys(row *genent.WorkAction) []string {
	keys := map[string]bool{}
	if row.SubjectKind == workaction.SubjectKindPullRequest && row.SubjectKey != "" {
		keys[row.SubjectKey] = true
	}
	if row.Edges.PullRequest != nil {
		if key := pullRequestDisplayKey(row.Edges.PullRequest); key != "" {
			keys[key] = true
		}
	}
	if row.Edges.Ticket != nil {
		for _, pr := range row.Edges.Ticket.Edges.PullRequests {
			if key := pullRequestDisplayKey(pr); key != "" {
				keys[key] = true
			}
		}
	}
	return sortedKeys(keys)
}

func ticketDisplayKey(row *genent.Ticket) string {
	if row == nil {
		return ""
	}
	if row.ExternalID != "" {
		return row.ExternalID
	}
	return sourceNeutralSuffix(row.Key)
}

func pullRequestDisplayKey(row *genent.PullRequest) string {
	if row == nil {
		return ""
	}
	if row.Repository != "" && row.Number > 0 {
		return row.Repository + "#" + strconv.Itoa(row.Number)
	}
	return sourceNeutralSuffix(row.Key)
}

func sourceNeutralSuffix(key string) string {
	if key == "" {
		return ""
	}
	parts := strings.Split(key, ":")
	return parts[len(parts)-1]
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func workActionDecisionNeeded(row *genent.WorkAction) string {
	if row.DecisionState == workaction.DecisionStateValidationLead && row.ActionType == workaction.ActionTypeDecisionOrOwnerFollowup {
		return "gold-label risk/actionability before owner decision"
	}
	if row.DecisionState == workaction.DecisionStateValidationLead && row.ActionType == workaction.ActionTypeCiCheckFollowup {
		return "determine required/merge-blocking check semantics"
	}
	switch row.ActionType {
	case workaction.ActionTypeDecisionOrOwnerFollowup:
		return "merge / close / park / assign owner"
	case workaction.ActionTypeValidateSignal:
		return "validate / suppress / escalate"
	case workaction.ActionTypeCiCheckFollowup:
		return "fix / mark non-blocking / assign owner"
	case workaction.ActionTypeReviewWaitFollowup:
		return "confirm reviewer / reassign / close"
	case workaction.ActionTypeRefreshSource:
		return "refresh source before product claim"
	case workaction.ActionTypeVerifyResolution:
		return "confirm closeout"
	case workaction.ActionTypeModelQualityReview:
		return "collect labels and more snapshots"
	case workaction.ActionTypeDismissedSignal:
		return "none"
	default:
		return "review"
	}
}

func workActionClaimUse(row *genent.WorkAction, claimPolicy workActionClaimPolicy) string {
	if workActionProductActionAllowedWithPolicy(row, claimPolicy) {
		if workActionETAClaimAllowedWithPolicy(row, claimPolicy) {
			return "eta_candidate_product_action"
		}
		if workActionHasSourceInsightKind(row, "forecast_risk") {
			return "risk_triage_owner_followup"
		}
		return "product_action"
	}
	if workActionResponsibilityValidationRequired(row, claimPolicy) {
		return "responsibility_validation"
	}
	if workActionRequestsETAClaim(row) {
		if workActionHasSourceInsightKind(row, "forecast_risk") {
			return "risk_triage_only"
		}
		return "eta_validation_required"
	}
	if workActionAbsenceClaimAllowed(row) {
		return "source_resolved_absence_claim"
	}
	if workActionCoverageLimited(row) {
		return "source_coverage_validation"
	}
	switch row.DecisionState {
	case workaction.DecisionStateValidationLead:
		switch {
		case workActionHasSourceInsightKind(row, "developer_correlation"):
			return "workload_context_only"
		case workActionHasSourceInsightKind(row, "forecast_risk"):
			return "risk_triage_only"
		case workActionHasSourceInsightKind(row, "blocker_candidate"):
			return "blocker_validation"
		default:
			return "validation_lead"
		}
	case workaction.DecisionStateSourceRepair:
		return "source_repair"
	case workaction.DecisionStateCloseoutReview:
		return "closeout_review"
	case workaction.DecisionStateSourceResolved:
		return "source_resolved_validation"
	case workaction.DecisionStateModelOrRuleQa:
		return "model_or_rule_qa"
	case workaction.DecisionStateSuppressedSignal:
		return "suppressed_signal"
	default:
		return "pending_review"
	}
}

func workActionClaimGateReason(row *genent.WorkAction, claimPolicy workActionClaimPolicy) string {
	if workActionResponsibilityValidationRequired(row, claimPolicy) {
		return workActionResponsibilityValidationGateReason(claimPolicy)
	}
	if workActionRequestsETAClaim(row) && !workActionETAClaimAllowedWithPolicy(row, claimPolicy) {
		return workActionETAGateReason(claimPolicy)
	}
	if workActionCoverageLimited(row) {
		return "source_coverage_limited"
	}
	if row.DecisionReason != "" {
		return row.DecisionReason
	}
	switch row.DecisionState {
	case workaction.DecisionStateProductAction:
		return "product_action_gate_passed"
	case workaction.DecisionStateSourceResolved:
		return "authenticated_terminal_source_observed"
	case workaction.DecisionStateValidationLead:
		return "validation_required_before_product_claim"
	case workaction.DecisionStateSourceRepair:
		return "source_repair_required"
	case workaction.DecisionStateCloseoutReview:
		return "closeout_review_required"
	case workaction.DecisionStateModelOrRuleQa:
		return "model_or_rule_quality_review_required"
	case workaction.DecisionStateSuppressedSignal:
		return "suppressed_signal"
	default:
		return "pending_review"
	}
}

func workActionProductActionAllowed(row *genent.WorkAction) bool {
	return workActionProductActionAllowedWithPolicy(row, workActionClaimPolicy{})
}

func workActionProductActionAllowedWithPolicy(row *genent.WorkAction, claimPolicy workActionClaimPolicy) bool {
	if row == nil {
		return false
	}
	if workActionResponsibilityValidationRequired(row, claimPolicy) {
		return false
	}
	if workActionRequestsETAClaim(row) && !workActionETAClaimAllowedWithPolicy(row, claimPolicy) {
		return false
	}
	return row.DecisionState == workaction.DecisionStateProductAction && !workActionCoverageLimited(row)
}

func workActionResponsibilityValidationRequired(row *genent.WorkAction, claimPolicy workActionClaimPolicy) bool {
	return row != nil &&
		row.DecisionState == workaction.DecisionStateProductAction &&
		claimPolicy.responsibilityValidationRequired
}

func workActionResponsibilityValidationGateReason(claimPolicy workActionClaimPolicy) string {
	if strings.TrimSpace(claimPolicy.responsibilityValidationReason) != "" {
		return strings.TrimSpace(claimPolicy.responsibilityValidationReason)
	}
	return "responsibility_validation_required"
}

func workActionAbsenceClaimAllowed(row *genent.WorkAction) bool {
	return row.DecisionState == workaction.DecisionStateSourceResolved && !workActionCoverageLimited(row)
}

func workActionETAClaimAllowed(row *genent.WorkAction) bool {
	return workActionETAClaimAllowedWithPolicy(row, workActionClaimPolicy{})
}

func workActionETAClaimAllowedWithPolicy(row *genent.WorkAction, claimPolicy workActionClaimPolicy) bool {
	if row == nil {
		return false
	}
	return row.DecisionState == workaction.DecisionStateProductAction &&
		!workActionCoverageLimited(row) &&
		claimPolicy.etaReadinessContextKnown &&
		claimPolicy.etaForecastReady &&
		workActionRequestsETAClaim(row)
}

func workActionRequestsETAClaim(row *genent.WorkAction) bool {
	if row == nil {
		return false
	}
	return forecastEvidenceSaysETAReady(row, nil)
}

func workActionETAGateReason(claimPolicy workActionClaimPolicy) string {
	if claimPolicy.etaReadinessContextKnown && strings.TrimSpace(claimPolicy.etaReadinessGateReason) != "" {
		return "eta_forecast_readiness_gated:" + strings.TrimSpace(claimPolicy.etaReadinessGateReason)
	}
	return "eta_forecast_readiness_not_verified"
}

func workActionCoverageLimited(row *genent.WorkAction) bool {
	if row.DecisionState == workaction.DecisionStateSourceRepair {
		return true
	}
	for _, observation := range row.Edges.Observations {
		if observation == nil {
			continue
		}
		if workClaimCoverageStateLimited(observation.SourceCoverageState) {
			return true
		}
		if workClaimCoverageStateGenerated(observation.SourceCoverageState) {
			return true
		}
		if strings.Contains(strings.ToLower(observation.AuthState), "anonymous") {
			return true
		}
	}
	return false
}

func workActionHasSourceInsightKind(row *genent.WorkAction, insightKind string) bool {
	for _, insight := range row.Edges.SourceInsights {
		if insight != nil && insight.InsightKind.String() == insightKind {
			return true
		}
	}
	return false
}

func workClaimCoverageStateLimited(state string) bool {
	state = strings.ToLower(state)
	return strings.Contains(state, "failed") ||
		strings.Contains(state, "failure") ||
		strings.Contains(state, "partial") ||
		strings.Contains(state, "repair") ||
		strings.Contains(state, "unavailable") ||
		strings.Contains(state, "unknown") ||
		strings.Contains(state, "missing") ||
		state == "" ||
		state == "not_observed"
}

func workClaimCoverageStateGenerated(state string) bool {
	return strings.Contains(strings.ToLower(state), "generated")
}

func workActionEvidenceRef(row *genent.WorkAction) string {
	if row.Edges.LatestEvidence != nil {
		return evidenceRef(row.Edges.LatestEvidence)
	}
	for _, observation := range row.Edges.Observations {
		if observation.Edges.LatestEvidence != nil {
			return evidenceRef(observation.Edges.LatestEvidence)
		}
		if observation.SourceURL != "" {
			return observation.SourceURL
		}
	}
	return firstNonempty(standupEvidenceRef(row), workActionSubjectURL(row))
}

func evidenceRef(row *genent.Evidence) string {
	if row == nil {
		return ""
	}
	parts := []string{}
	if row.LocatorKind != "" {
		parts = append(parts, row.LocatorKind)
	}
	if row.Locator != "" {
		parts = append(parts, row.Locator)
	}
	if row.SourceURL != "" {
		parts = append(parts, row.SourceURL)
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return firstNonempty(row.SourceSpanKey, row.ExternalID, row.Key)
}

func workActionBadges(row *genent.WorkAction, claimPolicy workActionClaimPolicy) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		decisionStateBadge(row, claimPolicy),
		dueBucketBadge(row),
	}
	if badge := observationBadge(row.Edges.Observations); badge != nil {
		badges = append(badges, badge)
	}
	if badge := ciSignalBadge(row.Edges.Observations); badge != nil {
		badges = append(badges, badge)
	}
	return badges
}

func workActionSummaryModel(actionState string, sourceInstance *string, rows []*genent.WorkAction, forecastRows ...*genent.WorkForecastEvaluation) *model.WorkActionSummary {
	summary := &model.WorkActionSummary{
		SourceInstance: sourceInstance,
		ActionState:    actionState,
		TotalCount:     len(rows),
	}
	summary.ForecastReadiness = workForecastReadiness(sourceInstance, rows, forecastRows...)
	claimPolicy := workActionClaimPolicyForForecastReadiness(summary.ForecastReadiness)
	decisionCounts := map[string]int{}
	actionCounts := map[string]int{}
	dueCounts := map[string]int{}
	ownerCounts := map[string]int{}
	for _, row := range rows {
		decisionState := row.DecisionState.String()
		actionType := row.ActionType.String()
		dueBucket := row.DueBucket.String()
		ownerKey := workActionOwnerKey(row)
		decisionCounts[decisionState]++
		actionCounts[actionType]++
		dueCounts[dueBucket]++
		ownerCounts[ownerKey]++
		if row.RankScore >= 75 || row.DueBucket == workaction.DueBucketNow {
			summary.HighPriorityCount++
		}
		hasSupportingObservation := false
		for _, observation := range row.Edges.Observations {
			if observation.SupportsAction {
				hasSupportingObservation = true
			}
		}
		if hasSupportingObservation {
			summary.SupportsActionObservationCount++
		}
	}
	summary.ProductActionCount = workActionProductActionCount(rows, claimPolicy)
	summary.ValidationLeadCount = decisionCounts[workaction.DecisionStateValidationLead.String()]
	summary.SourceRepairCount = decisionCounts[workaction.DecisionStateSourceRepair.String()]
	summary.CloseoutReviewCount = decisionCounts[workaction.DecisionStateCloseoutReview.String()]
	summary.ModelOrRuleQaCount = decisionCounts[workaction.DecisionStateModelOrRuleQa.String()]
	summary.SuppressedSignalCount = decisionCounts[workaction.DecisionStateSuppressedSignal.String()]
	summary.NowCount = dueCounts[workaction.DueBucketNow.String()]
	summary.ThisWeekCount = dueCounts[workaction.DueBucketThisWeek.String()]
	summary.WatchCount = dueCounts[workaction.DueBucketWatch.String()]
	summary.UnscheduledCount = dueCounts[workaction.DueBucketUnscheduled.String()]
	summary.Breakdowns = append(summary.Breakdowns, workActionBreakdowns("decision_state", decisionCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workActionBreakdowns("action_type", actionCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workActionBreakdowns("due_bucket", dueCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workActionBreakdowns("owner_key", ownerCounts)...)
	summary.OwnerRollups = workActionOwnerRollups(rows, claimPolicy)
	summary.Badges = workActionSummaryBadges(summary, actionCounts)

	topLimit := 10
	if len(rows) < topLimit {
		topLimit = len(rows)
	}
	summary.TopActions = make([]*model.WorkAction, 0, topLimit)
	for _, row := range rows[:topLimit] {
		summary.TopActions = append(summary.TopActions, workActionModelWithClaimPolicy(row, claimPolicy))
	}
	return summary
}

func workActionProductActionCount(rows []*genent.WorkAction, claimPolicy workActionClaimPolicy) int {
	count := 0
	for _, row := range rows {
		if workActionProductActionAllowedWithPolicy(row, claimPolicy) {
			count++
		}
	}
	return count
}

func workstreamStandupModel(actionState string, sourceInstance *string, rows []*genent.WorkAction, forecastRows ...*genent.WorkForecastEvaluation) *model.WorkstreamStandup {
	summary := workActionSummaryModel(actionState, sourceInstance, rows, forecastRows...)
	ownerCount := 0
	topOwnerActionCount := 0
	for _, rollup := range summary.OwnerRollups {
		if rollup.OwnerKey != "unassigned" {
			ownerCount++
		}
		if rollup.ActionCount > topOwnerActionCount {
			topOwnerActionCount = rollup.ActionCount
		}
	}

	criticalOrHighValidationLeads := 0
	anonymousObservations := 0
	coverageLimited := 0
	for _, row := range rows {
		if row.DecisionState == workaction.DecisionStateValidationLead && isHighPriorityAction(row) {
			criticalOrHighValidationLeads++
		}
		if actionHasAnonymousObservation(row) {
			anonymousObservations++
		}
		if actionHasLimitedCoverage(row) {
			coverageLimited++
		}
	}

	standup := &model.WorkstreamStandup{
		SourceInstance:                    sourceInstance,
		ActionState:                       actionState,
		OperatingStatus:                   standupOperatingStatus(summary),
		ActionItemCount:                   summary.TotalCount,
		ProductActionCount:                summary.ProductActionCount,
		ValidationLeadCount:               summary.ValidationLeadCount,
		CriticalOrHighValidationLeadCount: criticalOrHighValidationLeads,
		ModelOrRuleQaCount:                summary.ModelOrRuleQaCount,
		CloseoutReviewCount:               summary.CloseoutReviewCount,
		OwnerCount:                        ownerCount,
		TopOwnerActionCount:               topOwnerActionCount,
		NowCount:                          summary.NowCount,
		AnonymousObservationCount:         anonymousObservations,
		CoverageLimitedCount:              coverageLimited,
		EtaForecastReady:                  summary.ForecastReadiness.EtaForecastReady,
		RecommendedCadenceFocus:           standupCadenceFocus(summary, criticalOrHighValidationLeads, anonymousObservations, coverageLimited),
		ForecastReadiness:                 summary.ForecastReadiness,
		Sections:                          workstreamStandupSections(rows, summary.OwnerRollups),
	}
	return standup
}

func workstreamStandupSections(rows []*genent.WorkAction, ownerRollups []*model.WorkActionOwnerRollup) []*model.WorkstreamStandupSection {
	sections := []*model.WorkstreamStandupSection{}
	seenActionKeys := map[string]bool{}
	topRows := make([]*genent.WorkAction, 0, len(rows))
	for _, row := range rows {
		if row.ActionType == workaction.ActionTypeModelQualityReview ||
			row.DecisionState == workaction.DecisionStateSuppressedSignal ||
			row.DecisionState == workaction.DecisionStateSourceResolved {
			continue
		}
		if row.DecisionState == workaction.DecisionStateCloseoutReview {
			continue
		}
		topRows = append(topRows, row)
	}
	sortStandupActions(topRows)
	topLimit := 8
	if len(topRows) < topLimit {
		topLimit = len(topRows)
	}
	for _, row := range topRows[:topLimit] {
		seenActionKeys[row.Key] = true
		sections = append(sections, workstreamActionSection(len(sections)+1, workstreamActionSectionKind(row), row))
	}
	for _, row := range rows {
		if row.ActionType == workaction.ActionTypeModelQualityReview {
			seenActionKeys[row.Key] = true
			sections = append(sections, workstreamActionSection(len(sections)+1, "model_quality", row))
			break
		}
	}
	ownerLimit := 8
	if len(ownerRollups) < ownerLimit {
		ownerLimit = len(ownerRollups)
	}
	for _, rollup := range ownerRollups[:ownerLimit] {
		if rollup.OwnerKey == "unassigned" && rollup.ActionCount == 0 {
			continue
		}
		sections = append(sections, workstreamOwnerSection(len(sections)+1, rollup))
	}
	for _, row := range rows {
		if seenActionKeys[row.Key] || (row.DecisionState != workaction.DecisionStateCloseoutReview && row.DecisionState != workaction.DecisionStateSourceResolved) {
			continue
		}
		sections = append(sections, workstreamActionSection(len(sections)+1, "resolved_change", row))
	}
	return sections
}

func sortStandupActions(rows []*genent.WorkAction) {
	sort.SliceStable(rows, func(i, j int) bool {
		leftDecision := standupDecisionRank(rows[i])
		rightDecision := standupDecisionRank(rows[j])
		if leftDecision != rightDecision {
			return leftDecision > rightDecision
		}
		leftDue := workActionDueRank(rows[i].DueBucket)
		rightDue := workActionDueRank(rows[j].DueBucket)
		if leftDue != rightDue {
			return leftDue > rightDue
		}
		if rows[i].RankScore != rows[j].RankScore {
			return rows[i].RankScore > rows[j].RankScore
		}
		return rows[i].Key < rows[j].Key
	})
}

func standupDecisionRank(row *genent.WorkAction) int {
	switch row.DecisionState {
	case workaction.DecisionStateProductAction:
		return 5
	case workaction.DecisionStateSourceRepair:
		return 4
	case workaction.DecisionStateValidationLead:
		return 3
	case workaction.DecisionStateCloseoutReview:
		return 2
	case workaction.DecisionStateSourceResolved:
		return 0
	case workaction.DecisionStateModelOrRuleQa:
		return 1
	default:
		return 0
	}
}

func workstreamActionSection(rank int, sectionKind string, row *genent.WorkAction) *model.WorkstreamStandupSection {
	return &model.WorkstreamStandupSection{
		SectionRank:       rank,
		SectionKind:       sectionKind,
		Urgency:           standupUrgency(row),
		FreshnessState:    row.FreshnessState.String(),
		Confidence:        row.Confidence,
		OwnerKey:          optionalString(row.OwnerKey),
		SubjectKey:        optionalString(row.SubjectKey),
		ActionType:        optionalString(row.ActionType.String()),
		StatusSignal:      optionalString(standupStatusSignal(row)),
		Summary:           standupSummary(row),
		RecommendedAction: standupRecommendedAction(row),
		EvidenceRef:       optionalString(standupEvidenceRef(row)),
		Action:            workActionModel(row),
	}
}

func workstreamOwnerSection(rank int, rollup *model.WorkActionOwnerRollup) *model.WorkstreamStandupSection {
	topActionType := ""
	topSubjects := []string{}
	if len(rollup.TopActions) > 0 {
		topActionType = rollup.TopActions[0].ActionType
		for _, action := range rollup.TopActions {
			topSubjects = append(topSubjects, action.SubjectKey)
		}
	}
	summary := strconv.Itoa(rollup.ActionCount) + " action(s), " + strconv.Itoa(rollup.HighPriorityCount) + " high-priority; top subjects: " + strings.Join(topSubjects, ", ")
	return &model.WorkstreamStandupSection{
		SectionRank:       rank,
		SectionKind:       "owner_load",
		Urgency:           ownerLoadUrgency(rollup),
		FreshnessState:    "partial",
		Confidence:        0.85,
		OwnerKey:          optionalString(rollup.OwnerKey),
		ActionType:        optionalString(topActionType),
		StatusSignal:      optionalString("owner_has_open_actions"),
		Summary:           summary,
		RecommendedAction: ownerLoadRecommendedAction(topActionType),
		EvidenceRef:       optionalString("workActionSummary.ownerRollups"),
		Action:            firstStandupAction(rollup.TopActions),
	}
}

func firstStandupAction(rows []*model.WorkAction) *model.WorkAction {
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func standupOperatingStatus(summary *model.WorkActionSummary) string {
	if summary.TotalCount == 0 {
		return "clear"
	}
	if summary.ProductActionCount > 0 || summary.SourceRepairCount > 0 {
		return "attention_required"
	}
	if summary.ValidationLeadCount > 0 {
		return "validation_required"
	}
	if summary.CloseoutReviewCount > 0 || summary.ModelOrRuleQaCount > 0 {
		return "review_required"
	}
	return "watch"
}

func standupCadenceFocus(summary *model.WorkActionSummary, criticalOrHighValidationLeads int, anonymousObservations int, coverageLimited int) string {
	focus := []string{}
	if summary.ProductActionCount > 0 {
		focus = append(focus, "triage "+strconv.Itoa(summary.ProductActionCount)+" product-safe actions")
	}
	if summary.ValidationLeadCount > 0 {
		if criticalOrHighValidationLeads > 0 {
			focus = append(focus, "gold-label or validate "+strconv.Itoa(criticalOrHighValidationLeads)+" urgent leads")
		} else {
			focus = append(focus, "gold-label or validate "+strconv.Itoa(summary.ValidationLeadCount)+" leads")
		}
	}
	if count := countSummaryActionType(summary, workaction.ActionTypeCiCheckFollowup.String()); count > 0 {
		focus = append(focus, "route "+strconv.Itoa(count)+" PRs with failing checks")
	}
	if summary.SourceRepairCount > 0 {
		focus = append(focus, "refresh "+strconv.Itoa(summary.SourceRepairCount)+" source-limited subjects")
	}
	if coverageLimited > 0 {
		focus = append(focus, "treat "+strconv.Itoa(coverageLimited)+" coverage-limited observations as leads only")
	}
	if anonymousObservations > 0 {
		focus = append(focus, "treat "+strconv.Itoa(anonymousObservations)+" anonymous public observations as lower-auth confidence")
	}
	if summary.ForecastReadiness != nil && !summary.ForecastReadiness.EtaForecastReady {
		focus = append(focus, "keep forecast output as risk triage, not ETA commitment")
	}
	if len(focus) == 0 {
		return "review labels and watch for new state changes"
	}
	return strings.Join(focus, "; ")
}

func countSummaryActionType(summary *model.WorkActionSummary, actionType string) int {
	for _, breakdown := range summary.Breakdowns {
		if breakdown.Dimension == "action_type" && breakdown.Key == actionType {
			return breakdown.Count
		}
	}
	return 0
}

func workstreamActionSectionKind(row *genent.WorkAction) string {
	switch row.DecisionState {
	case workaction.DecisionStateProductAction:
		return "product_action"
	case workaction.DecisionStateValidationLead:
		return "validation_lead"
	case workaction.DecisionStateSourceRepair:
		return "source_repair"
	case workaction.DecisionStateCloseoutReview:
		return "closeout_review"
	case workaction.DecisionStateSourceResolved:
		return "resolved_change"
	case workaction.DecisionStateModelOrRuleQa:
		return "model_or_rule_qa"
	default:
		return "top_action"
	}
}

func standupUrgency(row *genent.WorkAction) string {
	if row.ActionType == workaction.ActionTypeModelQualityReview || row.DecisionState == workaction.DecisionStateCloseoutReview {
		return "medium"
	}
	if row.DecisionState == workaction.DecisionStateSourceResolved {
		return "low"
	}
	if row.RankScore >= 90 {
		return "critical"
	}
	if row.RankScore >= 75 || row.DueBucket == workaction.DueBucketNow {
		return "high"
	}
	if row.RankScore >= 50 || row.DueBucket == workaction.DueBucketThisWeek {
		return "medium"
	}
	return "low"
}

func ownerLoadUrgency(rollup *model.WorkActionOwnerRollup) string {
	if rollup.ProductActionCount > 0 || rollup.NowCount > 0 || rollup.HighPriorityCount > 0 {
		return "high"
	}
	return "medium"
}

func standupStatusSignal(row *genent.WorkAction) string {
	for _, observation := range row.Edges.Observations {
		if observation.CiSignal != "" {
			return observation.CiSignal
		}
		if observation.CurrentState != "" {
			return observation.CurrentState
		}
	}
	if row.DecisionState == workaction.DecisionStateModelOrRuleQa {
		return "model_quality_gate"
	}
	return row.DecisionState.String()
}

func standupSummary(row *genent.WorkAction) string {
	switch row.ActionType {
	case workaction.ActionTypeClearBlocker:
		return "Clear blocker candidate: " + row.SubjectKey
	case workaction.ActionTypeDecisionOrOwnerFollowup:
		return "Validate risk lead before owner decision: " + row.SubjectKey
	case workaction.ActionTypeCiCheckFollowup:
		return "Review CI check state: " + row.SubjectKey
	case workaction.ActionTypeReviewWaitFollowup:
		return "Confirm requested reviewer lead: " + row.SubjectKey
	case workaction.ActionTypeModelQualityReview:
		return "Review forecast model quality"
	case workaction.ActionTypeVerifyResolution:
		return "Verify resolved follow-up: " + row.SubjectKey
	case workaction.ActionTypeValidateSignal:
		return "Validate generated signal: " + row.SubjectKey
	default:
		if title := firstInsightTitle(row); title != "" {
			return title
		}
		return row.SubjectKey
	}
}

func standupRecommendedAction(row *genent.WorkAction) string {
	owner := firstNonempty(row.OwnerKey, "the current owner")
	switch row.ActionType {
	case workaction.ActionTypeClearBlocker:
		return "Ask for blocker status with " + owner + ", capture the concrete next step, and label whether the blocker candidate was actionable."
	case workaction.ActionTypeDecisionOrOwnerFollowup:
		if row.DecisionState == workaction.DecisionStateProductAction && workActionHasSourceInsightKind(row, "forecast_risk") {
			return "Ask " + owner + " for merge, close, park, or reviewer status; keep this as risk triage, not an ETA commitment."
		}
		return "Gold-label the risk/actionability for " + row.SubjectKey + "; keep this as a validation lead until forecast and measurement gates pass."
	case workaction.ActionTypeCiCheckFollowup:
		return "Review failing or pending GitHub checks with " + owner + "; label whether they are required or merge-blocking before escalating as product work."
	case workaction.ActionTypeReviewWaitFollowup:
		return "Confirm whether the requested reviewer is still expected with " + owner + "; record reviewer owner or merge/close decision."
	case workaction.ActionTypeModelQualityReview:
		return "Keep ETA use gated by backtest quality; collect time-series snapshots and labeled outcomes before treating forecasts as commitments."
	case workaction.ActionTypeVerifyResolution:
		return "Confirm this item should be closed out, attach the transition evidence, and remove any stale open-work escalation."
	case workaction.ActionTypeValidateSignal:
		return "Review source evidence for " + row.SubjectKey + ", label truth/actionability first, then escalate to an owner only if the signal is confirmed."
	default:
		if action := firstInsightRecommendedAction(row); action != "" {
			return action
		}
		return "Review the generated action and record a truth/actionability label."
	}
}

func ownerLoadRecommendedAction(actionType string) string {
	switch actionType {
	case workaction.ActionTypeDecisionOrOwnerFollowup.String():
		return "Decide merge, close, park, or owner path for aged open work."
	case workaction.ActionTypeClearBlocker.String(), workaction.ActionTypeValidateSignal.String():
		return "Validate blocker evidence before escalating as product work."
	case workaction.ActionTypeCiCheckFollowup.String():
		return "Assign failing checks to an owner or record why they do not block merge."
	case workaction.ActionTypeReviewWaitFollowup.String():
		return "Confirm whether the requested reviewer is still the right lead."
	default:
		return "Review the generated action and record a truth/actionability label."
	}
}

func standupEvidenceRef(row *genent.WorkAction) string {
	if row.SourceURL != "" {
		return row.SourceURL
	}
	for _, insight := range row.Edges.SourceInsights {
		if insight.SourceURL != "" {
			return insight.SourceURL
		}
	}
	return ""
}

func firstInsightTitle(row *genent.WorkAction) string {
	for _, insight := range row.Edges.SourceInsights {
		if insight.Title != "" {
			return insight.Title
		}
	}
	return ""
}

func firstInsightRecommendedAction(row *genent.WorkAction) string {
	for _, insight := range row.Edges.SourceInsights {
		if insight.RecommendedAction != "" {
			return insight.RecommendedAction
		}
	}
	return ""
}

func isHighPriorityAction(row *genent.WorkAction) bool {
	return row.RankScore >= 75 || row.DueBucket == workaction.DueBucketNow
}

func actionHasAnonymousObservation(row *genent.WorkAction) bool {
	for _, observation := range row.Edges.Observations {
		if observation.AuthState == "anonymous" || strings.Contains(observation.SourceCoverageState, "anonymous_success") {
			return true
		}
	}
	return false
}

func actionHasLimitedCoverage(row *genent.WorkAction) bool {
	if len(row.Edges.Observations) == 0 {
		return true
	}
	for _, observation := range row.Edges.Observations {
		if strings.Contains(observation.SourceCoverageState, "failed") || strings.Contains(observation.SourceCoverageState, "partial") || strings.Contains(observation.SourceCoverageState, "unavailable") {
			return true
		}
	}
	return false
}

func workActionOwnerRollups(rows []*genent.WorkAction, claimPolicy workActionClaimPolicy) []*model.WorkActionOwnerRollup {
	rollupsByOwner := map[string]*workActionOwnerRollupValues{}
	for _, row := range rows {
		ownerKey := workActionOwnerKey(row)
		rollup := rollupsByOwner[ownerKey]
		if rollup == nil {
			rollup = &workActionOwnerRollupValues{
				ownerKey:    ownerKey,
				ownerSource: "unassigned",
			}
			rollupsByOwner[ownerKey] = rollup
		}
		if rollup.ownerSource == "unassigned" && row.OwnerSource != "" {
			rollup.ownerSource = row.OwnerSource
		}
		rollup.actionCount++
		rollup.rows = append(rollup.rows, row)
		if workActionProductActionAllowedWithPolicy(row, claimPolicy) {
			rollup.productActionCount++
		}
		if row.DecisionState == workaction.DecisionStateValidationLead {
			rollup.validationLeadCount++
		}
		if row.DueBucket == workaction.DueBucketNow {
			rollup.nowCount++
		}
		if row.RankScore >= 75 || row.DueBucket == workaction.DueBucketNow {
			rollup.highPriorityCount++
		}
		if row.RankScore > rollup.maxRankScore {
			rollup.maxRankScore = row.RankScore
		}
	}

	rollups := make([]*workActionOwnerRollupValues, 0, len(rollupsByOwner))
	for _, rollup := range rollupsByOwner {
		sortWorkActions(rollup.rows)
		rollups = append(rollups, rollup)
	}
	sort.SliceStable(rollups, func(i, j int) bool {
		left := rollups[i]
		right := rollups[j]
		if left.productActionCount != right.productActionCount {
			return left.productActionCount > right.productActionCount
		}
		if left.nowCount != right.nowCount {
			return left.nowCount > right.nowCount
		}
		if left.highPriorityCount != right.highPriorityCount {
			return left.highPriorityCount > right.highPriorityCount
		}
		if left.maxRankScore != right.maxRankScore {
			return left.maxRankScore > right.maxRankScore
		}
		return left.ownerKey < right.ownerKey
	})

	out := make([]*model.WorkActionOwnerRollup, 0, len(rollups))
	for _, rollup := range rollups {
		topLimit := 3
		if len(rollup.rows) < topLimit {
			topLimit = len(rollup.rows)
		}
		topActions := make([]*model.WorkAction, 0, topLimit)
		for _, row := range rollup.rows[:topLimit] {
			topActions = append(topActions, workActionModelWithClaimPolicy(row, claimPolicy))
		}
		out = append(out, &model.WorkActionOwnerRollup{
			OwnerKey:            rollup.ownerKey,
			OwnerSource:         optionalString(rollup.ownerSource),
			ActionCount:         rollup.actionCount,
			ProductActionCount:  rollup.productActionCount,
			ValidationLeadCount: rollup.validationLeadCount,
			NowCount:            rollup.nowCount,
			HighPriorityCount:   rollup.highPriorityCount,
			MaxRankScore:        rollup.maxRankScore,
			Badges:              workActionOwnerBadges(rollup),
			TopActions:          topActions,
		})
	}
	return out
}

func workForecastReadiness(sourceInstance *string, rows []*genent.WorkAction, forecastRows ...*genent.WorkForecastEvaluation) *model.WorkForecastReadiness {
	readiness := &model.WorkForecastReadiness{
		SourceInstance:         sourceInstance,
		EtaForecastReady:       false,
		ReadinessState:         "unknown",
		GatedForecastLeadCount: 0,
		Badges:                 []*model.WorkActionBadge{},
	}
	var qualityAction *genent.WorkAction
	var qualityInsight *genent.WorkInsight
	for _, row := range rows {
		if actionHasInsightKind(row, workinsight.InsightKindForecastRisk.String()) &&
			row.ActionType == workaction.ActionTypeDecisionOrOwnerFollowup &&
			row.DecisionState == workaction.DecisionStateValidationLead {
			readiness.GatedForecastLeadCount++
		}
		if qualityAction == nil && (row.ActionType == workaction.ActionTypeModelQualityReview || actionHasInsightKind(row, workinsight.InsightKindModelQuality.String())) {
			qualityAction = row
			qualityInsight = firstInsightByKind(row, workinsight.InsightKindModelQuality.String())
		}
	}

	if len(forecastRows) > 0 {
		applyTypedForecastReadiness(readiness, forecastRows)
		if qualityAction != nil {
			readiness.QualityAction = workActionModel(qualityAction)
		}
	} else if qualityAction != nil {
		readiness.QualityAction = workActionModel(qualityAction)
		readiness.ReadinessState = "gated"
		readiness.ForecastMethod = optionalString(firstNonempty(qualityInsightModelMethod(qualityInsight), qualityAction.Decision))
		readiness.Detail = optionalString(firstNonempty(qualityInsightDetails(qualityInsight), qualityAction.DecisionReason))
		readiness.ReadinessReason = optionalString(firstNonempty(qualityInsightDetails(qualityInsight), qualityAction.DecisionReason))
		readiness.BestBacktestModel = optionalString(extractStringMetric(qualityInsightDetails(qualityInsight), bestBacktestModelPattern))
		readiness.MedianBaselineMaeDays = extractFloatMetric(qualityInsightDetails(qualityInsight), medianBaselineMAEPattern)
		readiness.HeuristicMaeDays = extractFloatMetric(qualityInsightDetails(qualityInsight), heuristicMAEPattern)
		readiness.RandomForestMaeDays = extractFloatMetric(qualityInsightDetails(qualityInsight), randomForestMAEPattern)
	} else if readiness.GatedForecastLeadCount > 0 {
		readiness.ReadinessState = "gated"
	}
	readiness.Badges = workForecastReadinessBadges(readiness)
	return readiness
}

func applyTypedForecastReadiness(readiness *model.WorkForecastReadiness, rows []*genent.WorkForecastEvaluation) {
	readiness.TypedEvaluationCount = len(rows)
	summary := forecastSummaryRow(rows)
	if summary != nil {
		readiness.SourceInstance = optionalString(summary.SourceInstance)
		readiness.EvaluatedAt = optionalTime(summary.EvaluatedAt)
		readiness.EtaForecastReady = summary.ReadyForEta
		readiness.ReadinessState = summary.ReadinessState.String()
		readiness.ForecastMethod = optionalString(summary.ForecastMethod)
		readiness.BestBacktestModel = optionalString(summary.BestModelName)
		readiness.BaselineSampleCount = summary.BaselineSampleCount
		readiness.OpenCandidateCount = summary.OpenCandidateCount
		readiness.ClosedUnmergedCount = summary.ClosedUnmergedCount
		readiness.ObservedSnapshotTimeCount = summary.ObservedSnapshotTimeCount
		readiness.TransitionCandidateCount = summary.TransitionCandidateCount
		readiness.TerminalTransitionCandidateCount = summary.TerminalTransitionCandidateCount
		readiness.TransitionHistoryReady = summary.TransitionHistoryReady
		readiness.MedianCycleDays = optionalFloat(summary.MedianCycleDays)
		readiness.P75CycleDays = optionalFloat(summary.P75CycleDays)
		readiness.Detail = optionalString(summary.ReadinessReason)
		readiness.ReadinessReason = optionalString(summary.ReadinessReason)
		if !summary.ReadyForEta {
			readiness.EtaReadinessBlockingReason = optionalString(firstNonempty(
				extractStringMetric(summary.ReadinessReason, forecastBlockerPattern),
				summary.ReadinessReason,
			))
		}
		readiness.EvidenceRef = optionalString(evidenceRef(summary.Edges.LatestEvidence))
		readiness.Evidence = workEvidenceSummary(summary.Edges.LatestEvidence)
	}
	readiness.MedianBaselineMaeDays = meanForecastMAE(rows, workforecastevaluation.EvaluationKindKfold, "median_cycle_baseline")
	readiness.HeuristicMaeDays = meanForecastMAE(rows, workforecastevaluation.EvaluationKindKfold, "heuristic_percentile")
	readiness.RandomForestMaeDays = meanForecastMAE(rows, workforecastevaluation.EvaluationKindKfold, "random_forest_regressor")
	if best := bestMeanForecastMAE(rows, workforecastevaluation.EvaluationKindKfold.String()); best != nil {
		readiness.BestKfoldModel = optionalString(best.modelName)
		readiness.BestKfoldMaeDays = floatPtr(best.maeDays)
	}
	if best := bestMeanForecastMAE(rows, workforecastevaluation.EvaluationKindChronologicalHoldout.String()); best != nil {
		readiness.BestChronologicalHoldoutModel = optionalString(best.modelName)
		readiness.BestChronologicalHoldoutMaeDays = floatPtr(best.maeDays)
	}
	if best := bestMeanForecastMAE(rows, forecastEvaluationKindSourceEventKfold); best != nil {
		readiness.SourceEventAsOfKfoldModel = optionalString(best.modelName)
		readiness.SourceEventAsOfKfoldMaeDays = floatPtr(best.maeDays)
	}
	if best := bestMeanForecastMAE(rows, forecastEvaluationKindSourceEventHoldout); best != nil {
		readiness.SourceEventAsOfChronologicalHoldoutModel = optionalString(best.modelName)
		readiness.SourceEventAsOfChronologicalHoldoutMaeDays = floatPtr(best.maeDays)
	}
	if best := bestMeanForecastMAE(rows, workforecastevaluation.EvaluationKindSurvivalTimeToMerge.String()); best != nil {
		readiness.SurvivalModel = optionalString(best.modelName)
		readiness.SurvivalMaeDays = floatPtr(best.maeDays)
	}
	if readiness.ReadinessState == "unknown" && readiness.GatedForecastLeadCount > 0 {
		readiness.ReadinessState = "gated"
	}
}

type forecastMAESummary struct {
	modelName string
	maeDays   float64
}

type forecastMAEAggregate struct {
	total  float64
	weight int
}

func forecastSummaryRow(rows []*genent.WorkForecastEvaluation) *genent.WorkForecastEvaluation {
	for _, row := range rows {
		if row.EvaluationKind == workforecastevaluation.EvaluationKindSummary {
			return row
		}
	}
	return nil
}

func meanForecastMAE(rows []*genent.WorkForecastEvaluation, evaluationKind workforecastevaluation.EvaluationKind, modelName string) *float64 {
	aggregate := &forecastMAEAggregate{}
	for _, row := range rows {
		if row.EvaluationKind != evaluationKind || row.ModelName != modelName || row.MaeDays == nil {
			continue
		}
		aggregate.add(row)
	}
	return aggregate.mean()
}

func (aggregate *forecastMAEAggregate) add(row *genent.WorkForecastEvaluation) {
	if aggregate == nil || row == nil || row.MaeDays == nil {
		return
	}
	weight := row.TestCount
	if weight <= 0 {
		weight = 1
	}
	aggregate.total += *row.MaeDays * float64(weight)
	aggregate.weight += weight
}

func (aggregate *forecastMAEAggregate) mean() *float64 {
	if aggregate == nil {
		return nil
	}
	if aggregate.weight == 0 {
		return nil
	}
	value := math.Round((aggregate.total/float64(aggregate.weight))*100) / 100
	return &value
}

func bestMeanForecastMAE(rows []*genent.WorkForecastEvaluation, evaluationKinds ...string) *forecastMAESummary {
	allowed := make(map[string]bool, len(evaluationKinds))
	for _, kind := range evaluationKinds {
		allowed[kind] = true
	}
	byModel := map[string]*forecastMAEAggregate{}
	for _, row := range rows {
		if row.MaeDays == nil || !allowed[row.EvaluationKind.String()] {
			continue
		}
		modelName := firstNonempty(row.ModelName, row.BestModelName)
		if modelName == "" {
			continue
		}
		aggregate := byModel[modelName]
		if aggregate == nil {
			aggregate = &forecastMAEAggregate{}
			byModel[modelName] = aggregate
		}
		aggregate.add(row)
	}

	var best *forecastMAESummary
	for modelName, aggregate := range byModel {
		mean := aggregate.mean()
		if mean == nil {
			continue
		}
		maeDays := *mean
		if best == nil || maeDays < best.maeDays || (maeDays == best.maeDays && modelName < best.modelName) {
			best = &forecastMAESummary{modelName: modelName, maeDays: maeDays}
		}
	}
	return best
}

func floatPtr(value float64) *float64 {
	return &value
}

func optionalFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	return value
}

func actionHasInsightKind(row *genent.WorkAction, insightKind string) bool {
	for _, insight := range row.Edges.SourceInsights {
		if insight.InsightKind.String() == insightKind {
			return true
		}
	}
	return false
}

func firstInsightByKind(row *genent.WorkAction, insightKind string) *genent.WorkInsight {
	for _, insight := range row.Edges.SourceInsights {
		if insight.InsightKind.String() == insightKind {
			return insight
		}
	}
	return nil
}

func qualityInsightDetails(row *genent.WorkInsight) string {
	if row == nil {
		return ""
	}
	return row.Details
}

func qualityInsightModelMethod(row *genent.WorkInsight) string {
	if row == nil {
		return ""
	}
	return row.ModelMethod
}

func forecastEvidenceSaysETAReady(action *genent.WorkAction, insight *genent.WorkInsight) bool {
	text := strings.ToLower(strings.Join([]string{
		action.Decision,
		action.DecisionReason,
		qualityInsightDetails(insight),
		qualityInsightModelMethod(insight),
	}, " "))
	if textContainsAny(text,
		"not eta-ready",
		"not eta ready",
		"not ready for eta",
		"eta not ready",
		"eta_forecast_ready=false",
		"not an eta",
		"not an eta promise",
		"no eta",
	) {
		return false
	}
	return strings.Contains(text, "eta_forecast_ready=true") || strings.Contains(text, "eta-ready=true") || strings.Contains(text, "eta ready")
}

func textContainsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func workForecastReadinessBadges(readiness *model.WorkForecastReadiness) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if readiness.EtaForecastReady {
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:eta_ready", Label: "ETA ready", Tone: "success"})
	} else if readiness.ReadinessState == "gated" {
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:eta_gated", Label: "ETA gated", Tone: "warning", Detail: optionalString("backtest has not beaten baseline")})
	}
	if readiness.GatedForecastLeadCount > 0 {
		badges = append(badges, countBadge("forecast:gated_leads", "Forecast validation leads", "warning", readiness.GatedForecastLeadCount))
	}
	if readiness.BestBacktestModel != nil && *readiness.BestBacktestModel == "median_cycle_baseline" {
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:baseline_wins", Label: "Baseline wins", Tone: "warning", Detail: optionalString("learned ETA model rejected")})
	}
	if readiness.TypedEvaluationCount > 0 || readiness.ForecastMethod != nil || readiness.ReadinessState != "unknown" {
		if readiness.TransitionHistoryReady {
			badges = append(badges, countBadge("forecast:transition_history_ready", "Transition history", "success", readiness.TransitionCandidateCount))
		} else {
			detail := fmt.Sprintf("%d observed snapshot time(s), %d transition candidate(s)", readiness.ObservedSnapshotTimeCount, readiness.TransitionCandidateCount)
			badges = append(badges, &model.WorkActionBadge{Key: "forecast:transition_history_gated", Label: "Transition history gated", Tone: "warning", Detail: optionalString(detail)})
		}
	}
	return badges
}

func extractStringMetric(text string, pattern *regexp.Regexp) string {
	matches := pattern.FindStringSubmatch(text)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func extractFloatMetric(text string, pattern *regexp.Regexp) *float64 {
	raw := extractStringMetric(text, pattern)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &value
}

func workActionOwnerKey(row *genent.WorkAction) string {
	if row.OwnerKey == "" {
		return "unassigned"
	}
	return row.OwnerKey
}

func workActionOwnerBadges(values *workActionOwnerRollupValues) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if values.productActionCount > 0 {
		badges = append(badges, countBadge("owner:product_actions", "Product actions", "success", values.productActionCount))
	}
	if values.validationLeadCount > 0 {
		badges = append(badges, countBadge("owner:validation_leads", "Validation leads", "warning", values.validationLeadCount))
	}
	if values.nowCount > 0 {
		badges = append(badges, countBadge("owner:due_now", "Due now", "danger", values.nowCount))
	}
	return badges
}

func workActionBreakdowns(dimension string, counts map[string]int) []*model.WorkActionBreakdown {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]*model.WorkActionBreakdown, 0, len(keys))
	for _, key := range keys {
		out = append(out, &model.WorkActionBreakdown{
			Dimension: dimension,
			Key:       key,
			Count:     counts[key],
		})
	}
	return out
}

func workActionSummaryBadges(summary *model.WorkActionSummary, actionCounts map[string]int) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if summary.ProductActionCount > 0 {
		badges = append(badges, countBadge("summary:product_actions", "Product actions", "success", summary.ProductActionCount))
	}
	if summary.ValidationLeadCount > 0 {
		badges = append(badges, countBadge("summary:validation_leads", "Validation leads", "warning", summary.ValidationLeadCount))
	}
	if count := actionCounts[workaction.ActionTypeCiCheckFollowup.String()]; count > 0 {
		badges = append(badges, countBadge("summary:ci_gated", "CI gated", "warning", count))
	}
	if count := actionCounts[workaction.ActionTypeDecisionOrOwnerFollowup.String()]; count > 0 {
		badges = append(badges, countBadge("summary:forecast_gated", "Forecast gated", "warning", count))
	}
	if summary.CloseoutReviewCount > 0 {
		badges = append(badges, countBadge("summary:closeout_review", "Closeout reviews", "info", summary.CloseoutReviewCount))
	}
	return badges
}

func countBadge(key string, label string, tone string, count int) *model.WorkActionBadge {
	detail := optionalString(strconv.Itoa(count) + " item(s)")
	return &model.WorkActionBadge{Key: key, Label: label, Tone: tone, Detail: detail}
}

func decisionStateBadge(row *genent.WorkAction, claimPolicy workActionClaimPolicy) *model.WorkActionBadge {
	detail := optionalString(row.DecisionReason)
	switch row.DecisionState {
	case workaction.DecisionStateProductAction:
		if workActionResponsibilityValidationRequired(row, claimPolicy) {
			return &model.WorkActionBadge{Key: "decision:responsibility_validation", Label: "Responsibility validation", Tone: "warning", Detail: optionalString(workActionResponsibilityValidationGateReason(claimPolicy))}
		}
		if workActionRequestsETAClaim(row) && !workActionProductActionAllowedWithPolicy(row, claimPolicy) {
			return &model.WorkActionBadge{Key: "decision:eta_gated", Label: "ETA gated", Tone: "warning", Detail: optionalString(workActionETAGateReason(claimPolicy))}
		}
		return &model.WorkActionBadge{Key: "decision:product_action", Label: "Product action", Tone: "success", Detail: detail}
	case workaction.DecisionStateValidationLead:
		return &model.WorkActionBadge{Key: "decision:validation_lead", Label: "Validation lead", Tone: "warning", Detail: detail}
	case workaction.DecisionStateCloseoutReview:
		return &model.WorkActionBadge{Key: "decision:closeout_review", Label: "Closeout review", Tone: "info", Detail: detail}
	case workaction.DecisionStateSourceResolved:
		return &model.WorkActionBadge{Key: "decision:source_resolved", Label: "Source resolved", Tone: "success", Detail: detail}
	case workaction.DecisionStateModelOrRuleQa:
		return &model.WorkActionBadge{Key: "decision:model_or_rule_qa", Label: "Model/rule QA", Tone: "info", Detail: detail}
	case workaction.DecisionStateSourceRepair:
		return &model.WorkActionBadge{Key: "decision:source_repair", Label: "Source repair", Tone: "danger", Detail: detail}
	case workaction.DecisionStateSuppressedSignal:
		return &model.WorkActionBadge{Key: "decision:suppressed_signal", Label: "Suppressed signal", Tone: "neutral", Detail: detail}
	default:
		return &model.WorkActionBadge{Key: "decision:pending_review", Label: "Pending review", Tone: "warning", Detail: detail}
	}
}

func dueBucketBadge(row *genent.WorkAction) *model.WorkActionBadge {
	switch row.DueBucket {
	case workaction.DueBucketNow:
		return &model.WorkActionBadge{Key: "due:now", Label: "Due now", Tone: "danger"}
	case workaction.DueBucketThisWeek:
		return &model.WorkActionBadge{Key: "due:this_week", Label: "Due this week", Tone: "warning"}
	case workaction.DueBucketWatch:
		return &model.WorkActionBadge{Key: "due:watch", Label: "Watch", Tone: "info"}
	default:
		return &model.WorkActionBadge{Key: "due:unscheduled", Label: "Unscheduled", Tone: "neutral"}
	}
}

func observationBadge(rows []*genent.WorkActionObservation) *model.WorkActionBadge {
	if len(rows) == 0 {
		return &model.WorkActionBadge{Key: "coverage:none", Label: "No observation", Tone: "warning"}
	}
	for _, row := range rows {
		if row.SupportsAction {
			return &model.WorkActionBadge{Key: "coverage:supports_action", Label: "Source supports action", Tone: "success", Detail: optionalString(row.SourceCoverageState)}
		}
	}
	for _, row := range rows {
		if strings.Contains(row.SourceCoverageState, "failed") || strings.Contains(row.SourceCoverageState, "partial") || strings.Contains(row.SourceCoverageState, "unavailable") {
			return &model.WorkActionBadge{Key: "coverage:limited", Label: "Coverage limited", Tone: "warning", Detail: optionalString(row.SourceCoverageState)}
		}
	}
	return &model.WorkActionBadge{Key: "coverage:evidence_only", Label: "Evidence only", Tone: "info", Detail: optionalString(rows[0].SourceCoverageState)}
}

func ciSignalBadge(rows []*genent.WorkActionObservation) *model.WorkActionBadge {
	for _, row := range rows {
		if row.CiSignal != "" {
			return &model.WorkActionBadge{Key: "signal:ci", Label: "CI signal", Tone: "warning", Detail: optionalString(row.CiSignal)}
		}
	}
	return nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func optionalTime(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	text := value.Format(time.RFC3339Nano)
	return &text
}

func sortWorkActions(rows []*genent.WorkAction) {
	sort.SliceStable(rows, func(i, j int) bool {
		leftDue := workActionDueRank(rows[i].DueBucket)
		rightDue := workActionDueRank(rows[j].DueBucket)
		if leftDue != rightDue {
			return leftDue > rightDue
		}
		if rows[i].RankScore != rows[j].RankScore {
			return rows[i].RankScore > rows[j].RankScore
		}
		return rows[i].Key < rows[j].Key
	})
}

func workActionDueRank(value workaction.DueBucket) int {
	switch value {
	case workaction.DueBucketNow:
		return 3
	case workaction.DueBucketThisWeek:
		return 2
	case workaction.DueBucketWatch:
		return 1
	default:
		return 0
	}
}
