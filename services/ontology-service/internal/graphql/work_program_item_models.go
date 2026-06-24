package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/graphql/model"
	"sort"
	"strconv"
	"strings"
)

const workProgramSourceCoverageLimitDimension = "source_coverage_limit_kind"
const workProgramAuthLimitedObservationDimension = "auth_limited_observation_kind"
const workProgramAuthLimitedProductDecisionDimension = "auth_limited_product_decision_kind"
const workProgramGeneratedClaimLimitDimension = "generated_claim_limit_kind"
const workProgramGeneratedClaimProductDecisionDimension = "generated_claim_product_decision_kind"

type workProgramOwnerAggregate struct {
	ownerKey              string
	ownerSource           string
	itemCount             int
	needsDecisionCount    int
	validateSignalCount   int
	ciFailingCount        int
	waitingReviewCount    int
	sourceRepairCount     int
	closureCandidateCount int
	nowCount              int
	highRiskCount         int
	maxRiskScore          float64
	rows                  []*genent.WorkProgramItem
}

type workProgramOwnerLoadSummary struct {
	status                string
	actionCount           int
	overloadedOwnerCount  int
	attentionOwnerCount   int
	unassignedActionCount int
	statusCounts          map[string]int
	topOwnerLoads         []*model.WorkOwnerLoadSnapshot
}

func workProgramItemModel(row *genent.WorkProgramItem) *model.WorkProgramItem {
	return workProgramItemModelWithClaimPolicy(row, workActionClaimPolicy{})
}

func workProgramItemModelWithForecastReadiness(row *genent.WorkProgramItem, readiness *model.WorkForecastReadiness) *model.WorkProgramItem {
	return workProgramItemModelWithClaimPolicy(row, workActionClaimPolicyForForecastReadiness(readiness))
}

func workProgramItemModelWithClaimPolicy(row *genent.WorkProgramItem, claimPolicy workActionClaimPolicy) *model.WorkProgramItem {
	var action *model.WorkAction
	if row.Edges.WorkAction != nil {
		action = workActionModelWithClaimPolicy(row.Edges.WorkAction, claimPolicy)
	}
	return &model.WorkProgramItem{
		Key:                   row.Key,
		SourceInstance:        optionalString(row.SourceInstance),
		WorkstreamKey:         row.WorkstreamKey,
		SubjectKind:           row.SubjectKind.String(),
		SubjectKey:            row.SubjectKey,
		LinkedTicketKeys:      splitDisplayList(row.LinkedTicketKeys),
		LinkedPullRequestKeys: splitDisplayList(row.LinkedPullRequestKeys),
		Title:                 row.Title,
		ProgramStatus:         row.ProgramStatus.String(),
		TpmBucket:             row.TpmBucket.String(),
		OwnerKey:              optionalString(row.OwnerKey),
		OwnerSource:           optionalString(row.OwnerSource),
		AuthorDri:             optionalString(row.AuthorDri),
		RequestedReviewerKeys: splitDisplayList(row.RequestedReviewerKeys),
		ReviewerOrApprover:    optionalString(row.ReviewerOrApprover),
		NextAction:            optionalString(row.NextAction),
		DecisionNeeded:        optionalString(row.DecisionNeeded),
		DecisionState:         row.DecisionState.String(),
		DecisionGateReason:    optionalString(row.DecisionGateReason),
		ClaimUse:              workProgramItemClaimUse(row, claimPolicy),
		ClaimGateReason:       workProgramItemClaimGateReason(row, claimPolicy),
		ProductActionAllowed:  workProgramItemProductActionAllowedWithPolicy(row, claimPolicy),
		AbsenceClaimAllowed:   workProgramItemAbsenceClaimAllowed(row),
		EtaClaimAllowed:       workProgramItemETAClaimAllowedWithPolicy(row, claimPolicy),
		DueBucket:             row.DueBucket.String(),
		LastSourceUpdateAt:    optionalTime(row.LastSourceUpdateAt),
		AgeDays:               optionalNonzeroFloat(row.AgeDays),
		StaleDays:             optionalNonzeroFloat(row.StaleDays),
		RiskScore:             row.RiskScore,
		BlockerLabelState:     optionalString(row.BlockerLabelState),
		CiSignal:              optionalString(row.CiSignal),
		TransitionState:       optionalString(row.TransitionState),
		DependencySummary:     optionalString(row.DependencySummary),
		SourceCoverageState:   optionalString(row.SourceCoverageState),
		LabelQuality:          optionalString(row.LabelQuality),
		EvidenceRef:           optionalString(evidenceRef(row.Edges.LatestEvidence)),
		Evidence:              workEvidenceSummary(row.Edges.LatestEvidence),
		Badges:                workProgramItemBadges(row, claimPolicy),
		Action:                action,
	}
}

func splitDisplayList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func optionalNonzeroFloat(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

func workProgramItemClaimUse(row *genent.WorkProgramItem, claimPolicy workActionClaimPolicy) string {
	if workProgramItemProductActionAllowedWithPolicy(row, claimPolicy) {
		if workProgramItemETAClaimAllowedWithPolicy(row, claimPolicy) {
			return "eta_candidate_product_action"
		}
		return "product_action"
	}
	if workProgramItemResponsibilityValidationRequired(row, claimPolicy) {
		return "responsibility_validation"
	}
	if workProgramItemRequestsETAClaim(row) {
		return "eta_validation_required"
	}
	if workProgramItemAbsenceClaimAllowed(row) {
		return "source_resolved_absence_claim"
	}
	if workProgramItemCoverageLimited(row) {
		return "source_coverage_validation"
	}
	switch row.DecisionState.String() {
	case "validation_lead":
		switch row.TpmBucket.String() {
		case "risk", "risk_validation":
			return "risk_triage_only"
		case "blocker":
			return "blocker_validation"
		case "ci", "ci_validation":
			return "ci_validation"
		case "reviewer_wait":
			return "review_wait_validation"
		case "review":
			return "review_validation"
		default:
			return "validation_lead"
		}
	case "source_repair":
		return "source_repair"
	case "closeout_review":
		return "closeout_review"
	case "source_resolved":
		return "source_resolved_validation"
	case "model_or_rule_qa":
		return "model_or_rule_qa"
	case "suppressed_signal":
		return "suppressed_signal"
	default:
		return "pending_review"
	}
}

func workProgramItemClaimGateReason(row *genent.WorkProgramItem, claimPolicy workActionClaimPolicy) string {
	if workProgramItemResponsibilityValidationRequired(row, claimPolicy) {
		return workActionResponsibilityValidationGateReason(claimPolicy)
	}
	if workProgramItemRequestsETAClaim(row) && !workProgramItemETAClaimAllowedWithPolicy(row, claimPolicy) {
		return workActionETAGateReason(claimPolicy)
	}
	if workProgramItemCoverageLimited(row) {
		return "source_coverage_limited:" + workProgramCoverageLimitKind(row)
	}
	if row.DecisionGateReason != "" {
		return row.DecisionGateReason
	}
	switch row.DecisionState.String() {
	case "product_action":
		return "product_action_gate_passed"
	case "source_resolved":
		return "authenticated_terminal_source_observed"
	case "validation_lead":
		return "validation_required_before_product_claim"
	case "source_repair":
		return "source_repair_required"
	case "closeout_review":
		return "closeout_review_required"
	case "model_or_rule_qa":
		return "model_or_rule_quality_review_required"
	case "suppressed_signal":
		return "suppressed_signal"
	default:
		return "pending_review"
	}
}

func workProgramItemProductActionAllowed(row *genent.WorkProgramItem) bool {
	return workProgramItemProductActionAllowedWithPolicy(row, workActionClaimPolicy{})
}

func workProgramItemProductActionAllowedWithPolicy(row *genent.WorkProgramItem, claimPolicy workActionClaimPolicy) bool {
	if row == nil {
		return false
	}
	if workProgramItemResponsibilityValidationRequired(row, claimPolicy) {
		return false
	}
	if workProgramItemRequestsETAClaim(row) && !workProgramItemETAClaimAllowedWithPolicy(row, claimPolicy) {
		return false
	}
	return row.DecisionState.String() == "product_action" && !workProgramItemCoverageLimited(row)
}

func workProgramItemResponsibilityValidationRequired(row *genent.WorkProgramItem, claimPolicy workActionClaimPolicy) bool {
	return row != nil &&
		row.DecisionState.String() == "product_action" &&
		claimPolicy.responsibilityValidationRequired
}

func workProgramItemProductDecisionOpen(row *genent.WorkProgramItem) bool {
	return row.DecisionState.String() == "product_action" ||
		row.DecisionState.String() == "closeout_review" ||
		row.ProgramStatus.String() == "needs_decision" ||
		row.ProgramStatus.String() == "closed_pending_review"
}

func workProgramItemAbsenceClaimAllowed(row *genent.WorkProgramItem) bool {
	return row.DecisionState.String() == "source_resolved" && !workProgramItemCoverageLimited(row)
}

func workProgramItemETAClaimAllowed(row *genent.WorkProgramItem) bool {
	return workProgramItemETAClaimAllowedWithPolicy(row, workActionClaimPolicy{})
}

func workProgramItemETAClaimAllowedWithPolicy(row *genent.WorkProgramItem, claimPolicy workActionClaimPolicy) bool {
	if row == nil {
		return false
	}
	return row.DecisionState.String() == "product_action" &&
		!workProgramItemCoverageLimited(row) &&
		claimPolicy.etaReadinessContextKnown &&
		claimPolicy.etaForecastReady &&
		workProgramItemRequestsETAClaim(row)
}

func workProgramItemRequestsETAClaim(row *genent.WorkProgramItem) bool {
	if row == nil {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		row.DecisionGateReason,
		row.DecisionNeeded,
		row.NextAction,
	}, " "))
	if textContainsAny(text,
		"not eta-ready",
		"not eta ready",
		"not ready for eta",
		"not an eta",
		"not an eta promise",
		"no eta",
	) {
		return false
	}
	return strings.Contains(text, "eta_forecast_ready=true") || strings.Contains(text, "eta-ready=true") || strings.Contains(text, "eta ready")
}

func workProgramItemBadges(row *genent.WorkProgramItem, claimPolicy workActionClaimPolicy) []*model.WorkActionBadge {
	status := row.ProgramStatus.String()
	badges := []*model.WorkActionBadge{
		{Key: "program:status", Label: workProgramStatusLabel(status), Tone: workProgramStatusTone(status)},
	}
	if workProgramItemResponsibilityValidationRequired(row, claimPolicy) {
		badges = append(badges, &model.WorkActionBadge{Key: "program:responsibility_validation", Label: "Responsibility validation", Tone: "warning", Detail: optionalString(workActionResponsibilityValidationGateReason(claimPolicy))})
	}
	if row.DueBucket.String() == "now" {
		badges = append(badges, &model.WorkActionBadge{Key: "program:due_now", Label: "Due now", Tone: "danger"})
	}
	if row.OwnerKey == "" {
		badges = append(badges, &model.WorkActionBadge{Key: "program:unassigned", Label: "Unassigned", Tone: "warning"})
	}
	if row.CiSignal != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "program:ci", Label: "CI signal", Tone: "warning", Detail: optionalString(row.CiSignal)})
	}
	if row.SourceCoverageState != "" && row.FreshnessState.String() == "partial" {
		badges = append(badges, &model.WorkActionBadge{Key: "program:coverage", Label: "Coverage limited", Tone: "warning", Detail: optionalString(row.SourceCoverageState)})
	}
	if row.RiskScore >= 90 {
		badges = append(badges, &model.WorkActionBadge{Key: "program:risk", Label: "High risk", Tone: "danger", Detail: optionalString(strconv.FormatFloat(row.RiskScore, 'f', 1, 64))})
	}
	return badges
}

func workProgramSummaryModel(sourceFilter *string, workstreamKey *string, rows []*genent.WorkProgramItem, signals workProgramExternalSignals, forecastReadiness *model.WorkForecastReadiness) *model.WorkProgramSummary {
	forecastReadiness = forecastReadinessForForecastRows(forecastReadiness, signals.topForecasts)
	claimPolicy := workActionClaimPolicyForForecastReadiness(forecastReadiness)
	summary := &model.WorkProgramSummary{
		SourceInstance:             optionalSourceFilterValue(sourceFilter),
		WorkstreamKey:              optionalWorkstreamFilterValue(workstreamKey),
		BlockerCount:               signals.blockerCount,
		ActiveBlockerCount:         signals.activeBlockerCount,
		ValidatingBlockerCount:     signals.validatingBlockerCount,
		BlockerImpactCount:         signals.blockerImpactCount,
		ActiveBlockerImpactCount:   signals.activeBlockerImpactCount,
		DependencyEdgeCount:        signals.dependencyEdgeCount,
		BlockingDependencyCount:    signals.blockingDependencyCount,
		NeedsActionDependencyCount: signals.needsActionDependencyCount,
		ForecastReadiness:          forecastReadiness,
		Breakdowns:                 []*model.WorkActionBreakdown{},
		OwnerRollups:               []*model.WorkProgramOwnerRollup{},
		TopOwnerLoads:              []*model.WorkOwnerLoadSnapshot{},
		TopItems:                   topWorkProgramItemModelsWithClaimPolicy(rows, 10, claimPolicy),
		TopBlockers:                workBlockerModels(signals.topBlockers),
		TopBlockerImpacts:          workBlockerImpactModels(signals.topBlockerImpacts),
		TopDependencies:            workDependencyEdgeModels(signals.topDependencies),
		TopForecasts:               workItemForecastModels(signals.topForecasts, forecastReadiness),
	}
	statusCounts := map[string]int{}
	bucketCounts := map[string]int{}
	ownerCounts := map[string]int{}
	sourceCoverageLimitCounts := map[string]int{}
	authLimitedObservationCounts := map[string]int{}
	authLimitedProductDecisionCounts := map[string]int{}
	generatedClaimLimitCounts := map[string]int{}
	generatedClaimProductDecisionCounts := map[string]int{}
	ownerRollups := map[string]*workProgramOwnerAggregate{}
	for _, row := range rows {
		summary.TotalCount++
		status := row.ProgramStatus.String()
		bucket := row.TpmBucket.String()
		statusCounts[status]++
		bucketCounts[bucket]++
		switch status {
		case "needs_decision":
			summary.NeedsDecisionCount++
		case "validate_signal":
			summary.ValidateSignalCount++
		case "ci_failing":
			summary.CiFailingCount++
		case "waiting_review":
			summary.WaitingReviewCount++
		case "source_repair":
			summary.SourceRepairCount++
		case "closed_pending_review":
			summary.ClosedPendingReviewCount++
		case "model_quality":
			summary.ModelQualityCount++
		case "closure_candidate":
			summary.ClosureCandidateCount++
		case "dismissed":
			summary.DismissedCount++
		}
		if row.DueBucket.String() == "now" {
			summary.NowCount++
		}
		if row.RiskScore >= 90 {
			summary.HighRiskCount++
		}
		if workProgramItemProductActionAllowedWithPolicy(row, claimPolicy) {
			summary.ProductActionCount++
		}
		if row.DecisionState.String() == "validation_lead" {
			summary.ValidationLeadCount++
		}
		if workProgramItemCoverageLimited(row) {
			summary.SourceCoverageLimitedCount++
			sourceCoverageLimitCounts[workProgramCoverageLimitKind(row)]++
		}
		if workProgramItemSourceStateAuthLimited(row.SourceCoverageState) {
			authLimitedObservationCounts[workProgramCoverageLimitKind(row)]++
			if workProgramItemProductDecisionOpen(row) {
				authLimitedProductDecisionCounts[workProgramCoverageLimitKind(row)]++
			}
		}
		if workClaimCoverageStateGenerated(row.SourceCoverageState) {
			generatedClaimLimitCounts[workProgramCoverageLimitKind(row)]++
			if workProgramItemProductDecisionOpen(row) {
				generatedClaimProductDecisionCounts[workProgramCoverageLimitKind(row)]++
			}
		}
		ownerKey := row.OwnerKey
		if ownerKey == "" {
			ownerKey = "unassigned"
			summary.UnassignedCount++
		}
		ownerCounts[ownerKey]++
		rollup := ownerRollups[ownerKey]
		if rollup == nil {
			rollup = &workProgramOwnerAggregate{
				ownerKey:    ownerKey,
				ownerSource: row.OwnerSource,
			}
			ownerRollups[ownerKey] = rollup
		}
		rollup.itemCount++
		rollup.rows = append(rollup.rows, row)
		if row.RiskScore > rollup.maxRiskScore {
			rollup.maxRiskScore = row.RiskScore
		}
		if rollup.ownerSource == "" && row.OwnerSource != "" {
			rollup.ownerSource = row.OwnerSource
		}
		switch status {
		case "needs_decision":
			rollup.needsDecisionCount++
		case "validate_signal":
			rollup.validateSignalCount++
		case "ci_failing":
			rollup.ciFailingCount++
		case "waiting_review":
			rollup.waitingReviewCount++
		case "source_repair":
			rollup.sourceRepairCount++
		case "closure_candidate":
			rollup.closureCandidateCount++
		}
		if row.DueBucket.String() == "now" {
			rollup.nowCount++
		}
		if row.RiskScore >= 90 {
			rollup.highRiskCount++
		}
	}
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns("program_status", statusCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns("tpm_bucket", bucketCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns("owner_key", ownerCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns(workProgramSourceCoverageLimitDimension, sourceCoverageLimitCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns(workProgramAuthLimitedObservationDimension, authLimitedObservationCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns(workProgramAuthLimitedProductDecisionDimension, authLimitedProductDecisionCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns(workProgramGeneratedClaimLimitDimension, generatedClaimLimitCounts)...)
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns(workProgramGeneratedClaimProductDecisionDimension, generatedClaimProductDecisionCounts)...)
	ownerLoad := workProgramOwnerLoadSummaryFor(signals.ownerLoadSnapshots)
	summary.OwnerLoadStatus = ownerLoad.status
	summary.OwnerLoadActionCount = ownerLoad.actionCount
	summary.OverloadedOwnerCount = ownerLoad.overloadedOwnerCount
	summary.AttentionOwnerCount = ownerLoad.attentionOwnerCount
	summary.UnassignedActionCount = ownerLoad.unassignedActionCount
	summary.TopOwnerLoads = ownerLoad.topOwnerLoads
	summary.Breakdowns = append(summary.Breakdowns, workProgramBreakdowns("owner_load_status", ownerLoad.statusCounts)...)
	summary.OwnerRollups = workProgramOwnerRollupModels(ownerRollups)
	applyWorkProgramOperatingPosture(summary)
	summary.Badges = workProgramSummaryBadges(summary)
	return summary
}

func applyWorkProgramOperatingPosture(summary *model.WorkProgramSummary) {
	if summary.OwnerLoadStatus == "" {
		summary.OwnerLoadStatus = "clear"
	}
	summary.ForecastState = workProgramForecastState(summary)
	summary.OperatingStatus = workProgramOperatingStatus(summary)
	summary.DecisionPressure = workProgramDecisionPressure(summary)
	summary.PrimaryRisk = optionalString(workProgramPrimaryRisk(summary))
	summary.RecommendedFocus = workProgramRecommendedFocus(summary)
	summary.CapabilityGaps = workProgramCapabilityGaps(summary)
}

func workProgramForecastState(summary *model.WorkProgramSummary) string {
	if summary.ForecastReadiness == nil {
		return "missing"
	}
	if summary.ForecastReadiness.EtaForecastReady {
		return "ready"
	}
	return "gated"
}

func workProgramOperatingStatus(summary *model.WorkProgramSummary) string {
	switch {
	case summary.ActiveBlockerCount > 0 || summary.ActiveBlockerImpactCount > 0:
		return "blocked"
	case summary.ProductActionCount > 0 || summary.NeedsDecisionCount > 0 || summary.NowCount > 0 || summary.NeedsActionDependencyCount > 0:
		return "attention_required"
	case summary.OverloadedOwnerCount > 0 || summary.UnassignedActionCount > 0:
		return "attention_required"
	case summary.ValidationLeadCount > 0 || summary.ValidateSignalCount > 0 || summary.SourceCoverageLimitedCount > 0 || workProgramMapCount(workProgramAuthLimitedObservationCounts(summary)) > 0 || workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary)) > 0:
		return "validation_required"
	case summary.TotalCount > 0:
		return "watch"
	default:
		return "clear"
	}
}

func workProgramDecisionPressure(summary *model.WorkProgramSummary) string {
	switch {
	case summary.ActiveBlockerCount > 0 || summary.ActiveBlockerImpactCount > 0:
		return "blocked"
	case summary.ProductActionCount > 0 || summary.NeedsDecisionCount > 0:
		return "product_action"
	case summary.NeedsActionDependencyCount > 0:
		return "dependency_action"
	case summary.OverloadedOwnerCount > 0 || summary.UnassignedActionCount > 0:
		return "owner_load"
	case summary.ValidationLeadCount > 0 || summary.ValidateSignalCount > 0 || workProgramMapCount(workProgramAuthLimitedObservationCounts(summary)) > 0 || workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary)) > 0:
		return "validation"
	case summary.ForecastState == "gated":
		return "forecast_quality"
	default:
		return "watch"
	}
}

func workProgramPrimaryRisk(summary *model.WorkProgramSummary) string {
	switch {
	case summary.ActiveBlockerCount > 0 || summary.ActiveBlockerImpactCount > 0:
		return "active_blockers"
	case summary.BlockingDependencyCount > 0 || summary.NeedsActionDependencyCount > 0:
		return "dependency_pressure"
	case summary.SourceCoverageLimitedCount > 0:
		return "coverage_limited"
	case workProgramMapCount(workProgramAuthLimitedObservationCounts(summary)) > 0:
		return "source_authentication"
	case workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary)) > 0:
		return "claim_provenance"
	case summary.OverloadedOwnerCount > 0 || summary.UnassignedActionCount > 0:
		return "owner_load"
	case summary.ValidationLeadCount > 0 || summary.ValidateSignalCount > 0:
		return "unvalidated_signals"
	case summary.ForecastState == "gated":
		return "forecast_gated"
	case summary.TotalCount > 0:
		return "workstream_watch"
	default:
		return ""
	}
}

func workProgramRecommendedFocus(summary *model.WorkProgramSummary) string {
	parts := []string{}
	if summary.ActiveBlockerCount > 0 {
		parts = append(parts, workProgramCountPhrase(summary.ActiveBlockerCount, "active blocker"))
	}
	if summary.ActiveBlockerImpactCount > 0 {
		parts = append(parts, workProgramCountPhrase(summary.ActiveBlockerImpactCount, "active blocker impact"))
	}
	if summary.ProductActionCount > 0 {
		parts = append(parts, workProgramCountPhrase(summary.ProductActionCount, "product action"))
	}
	if summary.ValidationLeadCount > 0 {
		parts = append(parts, workProgramCountPhrase(summary.ValidationLeadCount, "validation lead"))
	}
	if summary.NeedsActionDependencyCount > 0 {
		parts = append(parts, workProgramCountPhrase(summary.NeedsActionDependencyCount, "dependency needing action"))
	}
	if summary.OverloadedOwnerCount > 0 {
		parts = append(parts, workProgramCountPhrase(summary.OverloadedOwnerCount, "overloaded owner"))
	}
	if summary.UnassignedActionCount > 0 {
		parts = append(parts, workProgramCountPhrase(summary.UnassignedActionCount, "unassigned action"))
	}
	if authLimitedCount := workProgramMapCount(workProgramAuthLimitedObservationCounts(summary)); authLimitedCount > 0 {
		parts = append(parts, "re-observe "+workProgramCountPhrase(authLimitedCount, "anonymous source item"))
	}
	if generatedClaimCount := workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary)); generatedClaimCount > 0 {
		parts = append(parts, "review provenance for "+workProgramCountPhrase(generatedClaimCount, "generated claim item"))
	}
	if summary.ForecastState == "gated" {
		parts = append(parts, "treat ETA forecast as gated")
	}
	if len(parts) == 0 {
		if summary.TotalCount == 0 {
			return "No typed program items are in scope."
		}
		return "Maintain watch on typed program items."
	}
	return "Focus on " + workProgramJoinFocusParts(parts) + "."
}

func workProgramCapabilityGaps(summary *model.WorkProgramSummary) []string {
	gaps := []string{}
	if summary.ForecastState == "gated" || summary.ForecastState == "missing" {
		gaps = append(gaps, "forecast_gated")
	}
	if summary.SourceCoverageLimitedCount > 0 {
		gaps = append(gaps, "coverage_limited")
	}
	if workProgramMapCount(workProgramAuthLimitedObservationCounts(summary)) > 0 {
		gaps = append(gaps, "source_authentication_limited")
	}
	if workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary)) > 0 {
		gaps = append(gaps, "claim_provenance_limited")
	}
	if summary.UnassignedCount > 0 {
		gaps = append(gaps, "unassigned_items")
	}
	if summary.OverloadedOwnerCount > 0 {
		gaps = append(gaps, "owner_overloaded")
	}
	if summary.UnassignedActionCount > 0 {
		gaps = append(gaps, "owner_load_unassigned")
	}
	if summary.ActiveBlockerCount > 0 || summary.ActiveBlockerImpactCount > 0 {
		gaps = append(gaps, "active_blockers")
	}
	if summary.BlockingDependencyCount > 0 || summary.NeedsActionDependencyCount > 0 {
		gaps = append(gaps, "dependency_pressure")
	}
	if summary.ValidationLeadCount > 0 || summary.ValidateSignalCount > 0 {
		gaps = append(gaps, "validation_backlog")
	}
	return gaps
}

func workProgramCountPhrase(count int, label string) string {
	if count == 1 {
		return "1 " + label
	}
	return strconv.Itoa(count) + " " + label + "s"
}

func workProgramJoinFocusParts(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	if len(parts) == 2 {
		return parts[0] + " and " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
}

func optionalSourceFilterValue(sourceFilter *string) *string {
	if sourceFilter == nil {
		return nil
	}
	return optionalString(*sourceFilter)
}

func optionalWorkstreamFilterValue(workstreamKey *string) *string {
	if workstreamKey == nil || strings.TrimSpace(*workstreamKey) == "" {
		return nil
	}
	value := strings.TrimSpace(*workstreamKey)
	value = strings.TrimPrefix(value, "workstream:")
	return optionalString(value)
}

func topWorkProgramItemModels(rows []*genent.WorkProgramItem, limit int) []*model.WorkProgramItem {
	return topWorkProgramItemModelsWithClaimPolicy(rows, limit, workActionClaimPolicy{})
}

func topWorkProgramItemModelsWithClaimPolicy(rows []*genent.WorkProgramItem, limit int, claimPolicy workActionClaimPolicy) []*model.WorkProgramItem {
	if limit > len(rows) {
		limit = len(rows)
	}
	out := make([]*model.WorkProgramItem, 0, limit)
	for _, row := range rows[:limit] {
		out = append(out, workProgramItemModelWithClaimPolicy(row, claimPolicy))
	}
	return out
}

func workItemForecastModels(rows []*genent.WorkItemForecast, readiness *model.WorkForecastReadiness) []*model.WorkItemForecast {
	out := make([]*model.WorkItemForecast, 0, len(rows))
	for _, row := range rows {
		out = append(out, workItemForecastModelWithReadiness(row, readiness))
	}
	return out
}

func workProgramBreakdowns(dimension string, counts map[string]int) []*model.WorkActionBreakdown {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	out := make([]*model.WorkActionBreakdown, 0, len(keys))
	for _, key := range keys {
		out = append(out, &model.WorkActionBreakdown{Dimension: dimension, Key: key, Count: counts[key]})
	}
	return out
}

func workProgramOwnerRollupModels(rollups map[string]*workProgramOwnerAggregate) []*model.WorkProgramOwnerRollup {
	rows := make([]*workProgramOwnerAggregate, 0, len(rollups))
	for _, row := range rollups {
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].maxRiskScore != rows[j].maxRiskScore {
			return rows[i].maxRiskScore > rows[j].maxRiskScore
		}
		if rows[i].itemCount != rows[j].itemCount {
			return rows[i].itemCount > rows[j].itemCount
		}
		return rows[i].ownerKey < rows[j].ownerKey
	})
	out := make([]*model.WorkProgramOwnerRollup, 0, len(rows))
	for _, row := range rows {
		out = append(out, &model.WorkProgramOwnerRollup{
			OwnerKey:              row.ownerKey,
			OwnerSource:           optionalString(row.ownerSource),
			ItemCount:             row.itemCount,
			NeedsDecisionCount:    row.needsDecisionCount,
			ValidateSignalCount:   row.validateSignalCount,
			CiFailingCount:        row.ciFailingCount,
			WaitingReviewCount:    row.waitingReviewCount,
			SourceRepairCount:     row.sourceRepairCount,
			ClosureCandidateCount: row.closureCandidateCount,
			NowCount:              row.nowCount,
			HighRiskCount:         row.highRiskCount,
			MaxRiskScore:          row.maxRiskScore,
			Badges:                workProgramOwnerBadges(row),
			TopItems:              topWorkProgramItemModels(row.rows, 3),
		})
	}
	return out
}

func workProgramOwnerLoadSummaryFor(rows []*model.WorkOwnerLoadSnapshot) workProgramOwnerLoadSummary {
	out := workProgramOwnerLoadSummary{
		status:       "clear",
		statusCounts: map[string]int{},
	}
	if len(rows) == 0 {
		out.statusCounts["clear"] = 1
		return out
	}
	sortedRows := append([]*model.WorkOwnerLoadSnapshot{}, rows...)
	sort.SliceStable(sortedRows, func(i, j int) bool {
		if workProgramOwnerLoadStatusRank(sortedRows[i].LoadStatus) != workProgramOwnerLoadStatusRank(sortedRows[j].LoadStatus) {
			return workProgramOwnerLoadStatusRank(sortedRows[i].LoadStatus) < workProgramOwnerLoadStatusRank(sortedRows[j].LoadStatus)
		}
		if sortedRows[i].ActionCount != sortedRows[j].ActionCount {
			return sortedRows[i].ActionCount > sortedRows[j].ActionCount
		}
		if sortedRows[i].MaxPriorityScore != sortedRows[j].MaxPriorityScore {
			return sortedRows[i].MaxPriorityScore > sortedRows[j].MaxPriorityScore
		}
		return sortedRows[i].OwnerKey < sortedRows[j].OwnerKey
	})
	for _, row := range sortedRows {
		if row == nil {
			continue
		}
		status := row.LoadStatus
		if status == "" {
			status = "unknown"
		}
		out.statusCounts[status]++
		out.actionCount += row.ActionCount
		switch status {
		case "overloaded":
			out.overloadedOwnerCount++
		case "attention_required":
			out.attentionOwnerCount++
		}
		if row.OwnerKey == "(unassigned)" {
			out.unassignedActionCount += row.ActionCount
		}
		if len(out.topOwnerLoads) < 5 && row.OwnerKey != "(clear)" {
			out.topOwnerLoads = append(out.topOwnerLoads, row)
		}
	}
	switch {
	case out.overloadedOwnerCount > 0:
		out.status = "overloaded"
	case out.attentionOwnerCount > 0 || out.unassignedActionCount > 0:
		out.status = "attention_required"
	case out.actionCount > 0:
		out.status = "watch"
	default:
		out.status = "clear"
	}
	return out
}

func workProgramOwnerLoadStatusRank(status string) int {
	switch status {
	case "overloaded":
		return 0
	case "attention_required":
		return 1
	case "watch":
		return 2
	case "clear":
		return 3
	default:
		return 4
	}
}

func workProgramSummaryBadges(summary *model.WorkProgramSummary) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if summary.ProductActionCount > 0 {
		badges = append(badges, countBadge("program_summary:product_actions", "Product actions", "success", summary.ProductActionCount))
	}
	if summary.NeedsDecisionCount > 0 {
		badges = append(badges, countBadge("program_summary:needs_decision", "Needs decision", "danger", summary.NeedsDecisionCount))
	}
	if summary.ValidateSignalCount > 0 {
		badges = append(badges, countBadge("program_summary:validate_signal", "Validate signal", "warning", summary.ValidateSignalCount))
	}
	if summary.CiFailingCount > 0 {
		badges = append(badges, countBadge("program_summary:ci_failing", "CI failing", "warning", summary.CiFailingCount))
	}
	if summary.SourceRepairCount > 0 {
		badges = append(badges, countBadge("program_summary:source_repair", "Source repair", "danger", summary.SourceRepairCount))
	}
	if summary.SourceCoverageLimitedCount > 0 {
		badges = append(badges, countBadge("program_summary:coverage_limited", "Coverage limited", "warning", summary.SourceCoverageLimitedCount))
	}
	if authLimitedCount := workProgramMapCount(workProgramAuthLimitedObservationCounts(summary)); authLimitedCount > 0 {
		badges = append(badges, countBadge("program_summary:source_authentication", "Auth-limited source", "warning", authLimitedCount))
	}
	if generatedClaimCount := workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary)); generatedClaimCount > 0 {
		badges = append(badges, countBadge("program_summary:claim_provenance", "Claim provenance", "warning", generatedClaimCount))
	}
	if summary.UnassignedCount > 0 {
		badges = append(badges, countBadge("program_summary:unassigned", "Unassigned", "warning", summary.UnassignedCount))
	}
	if summary.OverloadedOwnerCount > 0 {
		badges = append(badges, countBadge("program_summary:owner_overloaded", "Overloaded owners", "danger", summary.OverloadedOwnerCount))
	}
	if summary.AttentionOwnerCount > 0 {
		badges = append(badges, countBadge("program_summary:owner_attention", "Owner load attention", "warning", summary.AttentionOwnerCount))
	}
	if summary.UnassignedActionCount > 0 {
		badges = append(badges, countBadge("program_summary:unassigned_actions", "Unassigned actions", "warning", summary.UnassignedActionCount))
	}
	if summary.ActiveBlockerCount > 0 {
		badges = append(badges, countBadge("program_summary:blockers", "Active blockers", "danger", summary.ActiveBlockerCount))
	}
	if summary.ActiveBlockerImpactCount > 0 {
		badges = append(badges, countBadge("program_summary:blocker_impacts", "Blocker impacts", "danger", summary.ActiveBlockerImpactCount))
	}
	if summary.BlockingDependencyCount > 0 {
		badges = append(badges, countBadge("program_summary:blocking_dependencies", "Blocking dependencies", "warning", summary.BlockingDependencyCount))
	}
	if summary.NeedsActionDependencyCount > 0 {
		badges = append(badges, countBadge("program_summary:needs_action_dependencies", "Needs action edges", "warning", summary.NeedsActionDependencyCount))
	}
	if summary.ForecastReadiness != nil && !summary.ForecastReadiness.EtaForecastReady {
		badges = append(badges, &model.WorkActionBadge{Key: "program_summary:forecast_gated", Label: "Forecast gated", Tone: "warning", Detail: summary.ForecastReadiness.ReadinessReason})
	}
	return badges
}

func workProgramOwnerBadges(row *workProgramOwnerAggregate) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if row.needsDecisionCount > 0 {
		badges = append(badges, countBadge("program_owner:needs_decision", "Needs decision", "danger", row.needsDecisionCount))
	}
	if row.validateSignalCount > 0 {
		badges = append(badges, countBadge("program_owner:validate_signal", "Validate signal", "warning", row.validateSignalCount))
	}
	if row.nowCount > 0 {
		badges = append(badges, countBadge("program_owner:due_now", "Due now", "danger", row.nowCount))
	}
	if row.highRiskCount > 0 {
		badges = append(badges, countBadge("program_owner:high_risk", "High risk", "danger", row.highRiskCount))
	}
	return badges
}

func workProgramItemCoverageLimited(row *genent.WorkProgramItem) bool {
	state := row.SourceCoverageState
	if workProgramItemSourceStateAuthLimited(state) || workClaimCoverageStateGenerated(state) {
		return false
	}
	if workClaimCoverageStateLimited(state) {
		return true
	}
	return row.FreshnessState.String() == "partial"
}

func workProgramCoverageLimitKind(row *genent.WorkProgramItem) string {
	state := strings.ToLower(row.SourceCoverageState)
	switch {
	case strings.Contains(state, "required_check_coverage") && strings.Contains(state, "unavailable"):
		return "required_check_coverage_unavailable"
	case strings.Contains(state, "anonymous"):
		return "anonymous_observation"
	case strings.Contains(state, "generated"):
		return "generated_evidence"
	case strings.Contains(state, "failed") || strings.Contains(state, "failure"):
		return "source_failure"
	case strings.Contains(state, "repair"):
		return "source_repair_needed"
	case strings.Contains(state, "unknown"):
		return "unknown_source_coverage"
	case strings.Contains(state, "unavailable"):
		return "source_unavailable"
	case strings.Contains(state, "partial") || row.FreshnessState.String() == "partial":
		return "partial_source_coverage"
	default:
		return "coverage_limited"
	}
}

func workProgramItemSourceStateAuthLimited(state string) bool {
	return strings.Contains(strings.ToLower(state), "anonymous")
}

func workProgramStatusLabel(status string) string {
	switch status {
	case "needs_decision":
		return "Needs decision"
	case "validate_signal":
		return "Validate signal"
	case "ci_failing":
		return "CI failing"
	case "waiting_review":
		return "Waiting review"
	case "source_repair":
		return "Source repair"
	case "closed_pending_review":
		return "Closed pending review"
	case "model_quality":
		return "Model quality"
	case "dismissed":
		return "Dismissed"
	case "closure_candidate":
		return "Closure candidate"
	case "needs_review":
		return "Needs review"
	default:
		return "Unknown"
	}
}

func workProgramStatusTone(status string) string {
	switch status {
	case "needs_decision", "ci_failing", "source_repair":
		return "danger"
	case "validate_signal", "waiting_review", "closed_pending_review", "model_quality", "needs_review":
		return "warning"
	case "closure_candidate":
		return "success"
	case "dismissed":
		return "neutral"
	default:
		return "info"
	}
}
