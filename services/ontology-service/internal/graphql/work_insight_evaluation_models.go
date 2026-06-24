package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/internal/graphql/model"
	"sort"
	"strconv"
	"strings"
)

const (
	minMeasurementLabelTotal             = 10
	minMeasurementLabelsPerKind          = 10
	minPrecisionRateForProductAction     = 0.70
	minUsefulSignalRateForProductAction  = 0.80
	minActionabilityRateForProductAction = 0.70
)

const (
	workInsightMeasurementScopeProductCandidate = "product_candidate"
	workInsightMeasurementScopeContextOnly      = "context_only"
	workInsightMeasurementScopeModelQuality     = "model_quality"
	workInsightMeasurementScopeValidationLead   = "validation_lead"
)

type workInsightKindEvaluationValues struct {
	insightKind               string
	currentInsightCount       int
	reviewRowCount            int
	measurementLabelCount     int
	openReviewRequestCount    int
	truthLabeledCount         int
	actionabilityLabeledCount int
	truePositiveCount         int
	falsePositiveCount        int
	partialCount              int
	actionableCount           int
	needsOwnerCount           int
}

func workInsightEvaluationModel(sourceInstance *string, rows []*genent.WorkInsight) *model.WorkInsightEvaluation {
	valuesByKind := map[string]*workInsightKindEvaluationValues{}
	total := &workInsightKindEvaluationValues{}
	productTotal := &workInsightKindEvaluationValues{}
	for _, row := range rows {
		kind := row.InsightKind.String()
		values := valuesByKind[kind]
		if values == nil {
			values = &workInsightKindEvaluationValues{insightKind: kind}
			valuesByKind[kind] = values
		}
		addInsightEvaluationCounts(values, row)
		addInsightEvaluationCounts(total, row)
		if workInsightMeasurementScope(kind) == workInsightMeasurementScopeProductCandidate {
			addInsightEvaluationCounts(productTotal, row)
		}
	}

	kinds := make([]*model.WorkInsightKindEvaluation, 0, len(valuesByKind))
	readyKinds := 0
	productActionReadyKinds := 0
	qualityGatedKinds := 0
	for _, values := range valuesByKind {
		kind := workInsightKindEvaluationModel(values)
		if kind.ReadyToMeasure {
			readyKinds++
		}
		if kind.ReadyForProductAction {
			productActionReadyKinds++
		} else if kind.ReadyToMeasure && workInsightKindMeasurementScope(kind) == workInsightMeasurementScopeProductCandidate {
			qualityGatedKinds++
		}
		kinds = append(kinds, kind)
	}
	sort.SliceStable(kinds, func(i, j int) bool {
		if kinds[i].ReadyForProductAction != kinds[j].ReadyForProductAction {
			return kinds[i].ReadyForProductAction
		}
		if kinds[i].ReadyToMeasure != kinds[j].ReadyToMeasure {
			return kinds[i].ReadyToMeasure
		}
		if kinds[i].CurrentInsightCount != kinds[j].CurrentInsightCount {
			return kinds[i].CurrentInsightCount > kinds[j].CurrentInsightCount
		}
		return kinds[i].InsightKind < kinds[j].InsightKind
	})

	gatedKinds := len(kinds) - readyKinds
	readyToMeasurePrecision := productTotal.currentInsightCount > 0 &&
		productTotal.truthLabeledCount >= minMeasurementLabelTotal &&
		workInsightAllProductCandidateKindsReadyToMeasure(kinds)
	readyToMeasureActionability := productTotal.currentInsightCount > 0 &&
		productTotal.actionabilityLabeledCount >= minMeasurementLabelTotal &&
		workInsightAllProductCandidateKindsReadyToMeasure(kinds)
	evaluation := &model.WorkInsightEvaluation{
		SourceInstance:                       sourceInstance,
		CurrentInsightCount:                  productTotal.currentInsightCount,
		ReviewRowCount:                       productTotal.reviewRowCount,
		MeasurementLabelCount:                productTotal.measurementLabelCount,
		OpenReviewRequestCount:               productTotal.openReviewRequestCount,
		MinLabeledTotalRequired:              minMeasurementLabelTotal,
		MinLabeledPerKindRequired:            minMeasurementLabelsPerKind,
		MinPrecisionRateForProductAction:     minPrecisionRateForProductAction,
		MinUsefulSignalRateForProductAction:  minUsefulSignalRateForProductAction,
		MinActionabilityRateForProductAction: minActionabilityRateForProductAction,
		PrecisionRate:                        workInsightRate(productTotal.truePositiveCount, productTotal.truthLabeledCount),
		UsefulSignalRate:                     workInsightRate(productTotal.truePositiveCount+productTotal.partialCount, productTotal.truthLabeledCount),
		ActionabilityRate:                    workInsightRate(productTotal.actionableCount+productTotal.needsOwnerCount, productTotal.actionabilityLabeledCount),
		FalsePositiveRate:                    workInsightRate(productTotal.falsePositiveCount, productTotal.truthLabeledCount),
		MeasurementCoverageRate:              workInsightCoverageRate(productTotal.measurementLabelCount, productTotal.currentInsightCount),
		ReadyToMeasurePrecision:              readyToMeasurePrecision,
		ReadyToMeasureActionability:          readyToMeasureActionability,
		ReadyInsightKindCount:                readyKinds,
		ProductActionReadyKindCount:          productActionReadyKinds,
		QualityGatedInsightKindCount:         qualityGatedKinds,
		GatedInsightKindCount:                gatedKinds,
		RecommendedNextStep:                  workInsightEvaluationNextStep(kinds),
		Badges:                               workInsightEvaluationBadges(productTotal, readyKinds, productActionReadyKinds, qualityGatedKinds, gatedKinds),
		Kinds:                                kinds,
	}
	return evaluation
}

func addInsightEvaluationCounts(values *workInsightKindEvaluationValues, row *genent.WorkInsight) {
	values.currentInsightCount++
	hasResolvedMeasurement := false
	for _, review := range row.Edges.Reviews {
		addReviewEvaluationCounts(values, review)
		if isResolvedMeasurementReview(review) {
			hasResolvedMeasurement = true
		}
	}
	if !hasResolvedMeasurement {
		values.openReviewRequestCount++
	}
}

func addReviewEvaluationCounts(values *workInsightKindEvaluationValues, review *genent.WorkInsightReview) {
	values.reviewRowCount++
	if !isGoldMeasurementReview(review) {
		return
	}
	values.measurementLabelCount++
	if review.TruthLabel != workinsightreview.TruthLabelUnknown {
		values.truthLabeledCount++
	}
	if review.ActionabilityLabel != workinsightreview.ActionabilityLabelUnknown {
		values.actionabilityLabeledCount++
	}
	switch review.TruthLabel {
	case workinsightreview.TruthLabelTruePositive:
		values.truePositiveCount++
	case workinsightreview.TruthLabelFalsePositive:
		values.falsePositiveCount++
	case workinsightreview.TruthLabelPartial:
		values.partialCount++
	}
	switch review.ActionabilityLabel {
	case workinsightreview.ActionabilityLabelActionable:
		values.actionableCount++
	case workinsightreview.ActionabilityLabelNeedsOwner:
		values.needsOwnerCount++
	}
}

func workInsightKindEvaluationModel(values *workInsightKindEvaluationValues) *model.WorkInsightKindEvaluation {
	required := requiredMeasurementLabelCount(values.currentInsightCount)
	readyToMeasure := required == 0 || (values.truthLabeledCount >= required && values.actionabilityLabeledCount >= required)
	precisionRate := workInsightRate(values.truePositiveCount, values.truthLabeledCount)
	usefulSignalRate := workInsightRate(values.truePositiveCount+values.partialCount, values.truthLabeledCount)
	actionabilityRate := workInsightRate(values.actionableCount+values.needsOwnerCount, values.actionabilityLabeledCount)
	falsePositiveRate := workInsightRate(values.falsePositiveCount, values.truthLabeledCount)
	measurementCoverageRate := workInsightCoverageRate(values.measurementLabelCount, values.currentInsightCount)
	measurementScope := workInsightMeasurementScope(values.insightKind)
	readyForProductAction := measurementScope == workInsightMeasurementScopeProductCandidate &&
		readyToMeasure &&
		precisionRate >= minPrecisionRateForProductAction &&
		usefulSignalRate >= minUsefulSignalRateForProductAction &&
		actionabilityRate >= minActionabilityRateForProductAction
	gateState, gateReason := workInsightProductActionGate(measurementScope, readyToMeasure, readyForProductAction, required, values, precisionRate, usefulSignalRate, actionabilityRate)
	out := &model.WorkInsightKindEvaluation{
		InsightKind:               values.insightKind,
		MeasurementScope:          measurementScope,
		CurrentInsightCount:       values.currentInsightCount,
		ReviewRowCount:            values.reviewRowCount,
		MeasurementLabelCount:     values.measurementLabelCount,
		OpenReviewRequestCount:    values.openReviewRequestCount,
		TruthLabeledCount:         values.truthLabeledCount,
		ActionabilityLabeledCount: values.actionabilityLabeledCount,
		TruePositiveCount:         values.truePositiveCount,
		FalsePositiveCount:        values.falsePositiveCount,
		PartialCount:              values.partialCount,
		ActionableCount:           values.actionableCount,
		NeedsOwnerCount:           values.needsOwnerCount,
		PrecisionRate:             precisionRate,
		UsefulSignalRate:          usefulSignalRate,
		ActionabilityRate:         actionabilityRate,
		FalsePositiveRate:         falsePositiveRate,
		MeasurementCoverageRate:   measurementCoverageRate,
		RequiredLabelCount:        required,
		ReadyToMeasure:            readyToMeasure,
		ReadyForProductAction:     readyForProductAction,
		ProductActionGateState:    gateState,
		ProductActionGateReason:   gateReason,
		RecommendedAction:         workInsightKindEvaluationAction(values.insightKind, measurementScope, readyToMeasure, readyForProductAction, required, values),
		Badges:                    workInsightKindEvaluationBadges(values, readyToMeasure, readyForProductAction, required),
	}
	return out
}

func workInsightMeasurementScope(insightKind string) string {
	switch insightKind {
	case "status_summary", "blocker_candidate", "forecast_risk":
		return workInsightMeasurementScopeProductCandidate
	case "developer_correlation", "dependency_cluster":
		return workInsightMeasurementScopeContextOnly
	case "model_quality":
		return workInsightMeasurementScopeModelQuality
	default:
		return workInsightMeasurementScopeValidationLead
	}
}

func workInsightKindMeasurementScope(kind *model.WorkInsightKindEvaluation) string {
	if kind == nil {
		return workInsightMeasurementScopeValidationLead
	}
	canonicalScope := workInsightMeasurementScope(kind.InsightKind)
	if scope := normalizeWorkInsightMeasurementScope(kind.MeasurementScope); scope != "" {
		if canonicalScope != workInsightMeasurementScopeValidationLead && scope != canonicalScope {
			return canonicalScope
		}
		return scope
	}
	return canonicalScope
}

func normalizeWorkInsightMeasurementScope(scope string) string {
	switch strings.TrimSpace(scope) {
	case workInsightMeasurementScopeProductCandidate:
		return workInsightMeasurementScopeProductCandidate
	case workInsightMeasurementScopeContextOnly:
		return workInsightMeasurementScopeContextOnly
	case workInsightMeasurementScopeModelQuality:
		return workInsightMeasurementScopeModelQuality
	case workInsightMeasurementScopeValidationLead:
		return workInsightMeasurementScopeValidationLead
	default:
		return ""
	}
}

func workInsightAllProductCandidateKindsReadyToMeasure(kinds []*model.WorkInsightKindEvaluation) bool {
	productCandidateKinds := 0
	for _, kind := range kinds {
		if kind == nil || workInsightKindMeasurementScope(kind) != workInsightMeasurementScopeProductCandidate {
			continue
		}
		productCandidateKinds++
		if !kind.ReadyToMeasure {
			return false
		}
	}
	return productCandidateKinds > 0
}

func workInsightProductActionGate(measurementScope string, readyToMeasure bool, readyForProductAction bool, required int, values *workInsightKindEvaluationValues, precisionRate float64, usefulSignalRate float64, actionabilityRate float64) (string, string) {
	switch measurementScope {
	case workInsightMeasurementScopeContextOnly:
		return "context_only", "This insight kind is retained for routing, topology, or workload context and cannot independently support product-action automation."
	case workInsightMeasurementScopeModelQuality:
		return "model_quality", "This insight kind measures model or rule quality; it gates automation readiness but is not product action."
	case workInsightMeasurementScopeValidationLead:
		return "validation_only", "This insight kind can create validation leads, but it has no product-action automation contract yet."
	}
	if !readyToMeasure {
		missing := required - values.measurementLabelCount
		if missing < 0 {
			missing = 0
		}
		return "measurement_gated", "Needs " + strconv.Itoa(missing) + " more gold label(s) before product-action quality can be measured."
	}
	if readyForProductAction {
		return "passed", "Measured precision, useful-signal, and actionability rates meet product-action thresholds."
	}
	missing := []string{}
	if precisionRate < minPrecisionRateForProductAction {
		missing = append(missing, "precision")
	}
	if usefulSignalRate < minUsefulSignalRateForProductAction {
		missing = append(missing, "useful signal")
	}
	if actionabilityRate < minActionabilityRateForProductAction {
		missing = append(missing, "actionability")
	}
	return "quality_gated", "Measured " + strings.Join(missing, ", ") + " below product-action threshold."
}

func workInsightRate(numerator int, denominator int) float64 {
	if denominator <= 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

func workInsightCoverageRate(labelCount int, currentInsightCount int) float64 {
	if currentInsightCount <= 0 {
		return 0
	}
	rate := float64(labelCount) / float64(currentInsightCount)
	if rate > 1 {
		return 1
	}
	return rate
}

func requiredMeasurementLabelCount(currentInsightCount int) int {
	if currentInsightCount <= 0 {
		return 0
	}
	if currentInsightCount < minMeasurementLabelsPerKind {
		return currentInsightCount
	}
	return minMeasurementLabelsPerKind
}

func isGoldMeasurementReview(review *genent.WorkInsightReview) bool {
	if review == nil {
		return false
	}
	if !review.MeasurementEligible {
		return false
	}
	if review.ReviewKind == workinsightreview.ReviewKindHumanAssessment {
		return true
	}
	if review.ReviewKind != workinsightreview.ReviewKindEvaluationLabel {
		return false
	}
	return review.LabelQuality == workinsightreview.LabelQualityGold
}

func isResolvedMeasurementReview(review *genent.WorkInsightReview) bool {
	if !isGoldMeasurementReview(review) {
		return false
	}
	switch review.ReviewState {
	case workinsightreview.ReviewStateAccepted, workinsightreview.ReviewStateDismissed, workinsightreview.ReviewStateResolved:
	default:
		return false
	}
	switch review.TruthLabel {
	case workinsightreview.TruthLabelTruePositive, workinsightreview.TruthLabelFalsePositive:
	default:
		return false
	}
	switch review.ActionabilityLabel {
	case workinsightreview.ActionabilityLabelActionable, workinsightreview.ActionabilityLabelNeedsOwner, workinsightreview.ActionabilityLabelNotActionable:
		return true
	default:
		return false
	}
}

func workInsightReviewLabelQuality(review *genent.WorkInsightReview) string {
	if review.LabelQuality != "" && review.LabelQuality != workinsightreview.LabelQualityUnknown {
		return review.LabelQuality.String()
	}
	reviewerKind := strings.ToLower(review.ReviewerKind.String())
	if strings.HasPrefix(reviewerKind, "imported_") {
		return strings.TrimPrefix(reviewerKind, "imported_")
	}
	reviewerKey := strings.ToLower(review.ReviewerKey)
	labelSet := strings.ToLower(review.LabelSet)
	if reviewerKind == workinsightreview.ReviewerKindImported.String() && strings.Contains(reviewerKey, "gold") {
		return "gold"
	}
	if strings.Contains(labelSet, "gold") {
		return "gold"
	}
	return ""
}

func workInsightEvaluationBadges(values *workInsightKindEvaluationValues, readyKinds int, productActionReadyKinds int, qualityGatedKinds int, gatedKinds int) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if readyKinds > 0 {
		badges = append(badges, countBadge("evaluation:ready_kinds", "Measurement-ready kinds", "success", readyKinds))
	}
	if productActionReadyKinds > 0 {
		badges = append(badges, countBadge("evaluation:product_action_ready_kinds", "Product-action-ready kinds", "success", productActionReadyKinds))
	}
	if qualityGatedKinds > 0 {
		badges = append(badges, countBadge("evaluation:quality_gated_kinds", "Quality-gated kinds", "warning", qualityGatedKinds))
	}
	if gatedKinds > 0 {
		badges = append(badges, countBadge("evaluation:gated_kinds", "Gated insight kinds", "warning", gatedKinds))
	}
	if values.openReviewRequestCount > 0 {
		badges = append(badges, countBadge("evaluation:open_reviews", "Open review requests", "warning", values.openReviewRequestCount))
	}
	if values.measurementLabelCount > 0 {
		badges = append(badges, countBadge("evaluation:gold_labels", "Gold labels", "info", values.measurementLabelCount))
	}
	return badges
}

func workInsightKindEvaluationBadges(values *workInsightKindEvaluationValues, readyToMeasure bool, readyForProductAction bool, required int) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if readyForProductAction {
		badges = append(badges, &model.WorkActionBadge{Key: "evaluation:product_action_ready", Label: "Product action ready", Tone: "success"})
	} else if readyToMeasure {
		badges = append(badges, &model.WorkActionBadge{Key: "evaluation:measurement_ready", Label: "Measurement ready", Tone: "success"})
	} else {
		missing := required - values.measurementLabelCount
		if missing < 0 {
			missing = 0
		}
		badges = append(badges, &model.WorkActionBadge{Key: "evaluation:needs_gold_labels", Label: "Needs gold labels", Tone: "warning", Detail: optionalString(strconv.Itoa(missing) + " more")})
	}
	if values.openReviewRequestCount > 0 {
		badges = append(badges, countBadge("evaluation:open_reviews", "Open review requests", "warning", values.openReviewRequestCount))
	}
	if values.falsePositiveCount > 0 {
		badges = append(badges, countBadge("evaluation:false_positives", "False positives", "warning", values.falsePositiveCount))
	}
	return badges
}

func workInsightEvaluationNextStep(kinds []*model.WorkInsightKindEvaluation) string {
	if len(kinds) == 0 {
		return "No current generated insights are available for evaluation."
	}
	needsLabels := []string{}
	for _, kind := range kinds {
		if workInsightKindMeasurementScope(kind) == workInsightMeasurementScopeProductCandidate && !kind.ReadyToMeasure {
			needsLabels = append(needsLabels, kind.InsightKind)
		}
	}
	if len(needsLabels) > 0 {
		return "Gold-label " + strings.Join(needsLabels, ", ") + " before using those insight kinds for autonomous product actions; keep them as validation leads meanwhile."
	}
	qualityGated := []string{}
	for _, kind := range kinds {
		if workInsightKindMeasurementScope(kind) == workInsightMeasurementScopeProductCandidate && kind.ReadyToMeasure && !kind.ReadyForProductAction {
			qualityGated = append(qualityGated, kind.InsightKind)
		}
	}
	if len(qualityGated) > 0 {
		return "Improve or suppress " + strings.Join(qualityGated, ", ") + " before promoting those insight kinds to product-action automation."
	}
	contextOnly := []string{}
	for _, kind := range kinds {
		if workInsightKindMeasurementScope(kind) == workInsightMeasurementScopeContextOnly {
			contextOnly = append(contextOnly, kind.InsightKind)
		}
	}
	if len(contextOnly) == len(kinds) {
		return "Measured context-only insight kinds " + strings.Join(contextOnly, ", ") + " can support routing and review, but not autonomous product actions."
	}
	return "All current insight kinds have enough gold labels for measurement; use kind-level product-action gates before promoting any signal to autonomous product actions."
}

func workInsightKindEvaluationAction(insightKind string, measurementScope string, readyToMeasure bool, readyForProductAction bool, required int, values *workInsightKindEvaluationValues) string {
	switch measurementScope {
	case workInsightMeasurementScopeContextOnly:
		if insightKind == "developer_correlation" {
			return "Keep this as workload/routing context; label usefulness for capacity or escalation, not ownership, causality, performance, ETA, or blockers."
		}
		if insightKind == "dependency_cluster" {
			return "Keep this as topology context until a source-backed blocking dependency or owner-confirmed coordination action exists."
		}
		return "Keep this signal as context until a product-action contract exists."
	case workInsightMeasurementScopeModelQuality:
		return "Use this to gate model readiness, not as a product escalation."
	case workInsightMeasurementScopeValidationLead:
		return "Use this signal for validation packets only until a product-action contract and measurement target are defined."
	}
	if !readyToMeasure {
		missing := required - values.measurementLabelCount
		if missing < 0 {
			missing = 0
		}
		switch insightKind {
		case "forecast_risk":
			return "Gold-label " + strconv.Itoa(missing) + " forecast-risk leads and keep ETA output gated until forecast quality beats simple baselines."
		case "model_quality":
			return "Review the forecast backtest gate before promoting ETA forecasts beyond risk triage."
		default:
			return "Gold-label " + strconv.Itoa(missing) + " current " + insightKind + " insight(s) before promoting this kind beyond validation leads."
		}
	}
	if readyForProductAction {
		return "This insight kind meets product-action precision and actionability thresholds; keep sampling dismissed and partial cases."
	}
	return "Measurement coverage is sufficient, but precision/actionability are too weak for product-action gating."
}
