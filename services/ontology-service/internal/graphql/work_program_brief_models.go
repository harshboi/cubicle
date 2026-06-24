package graphql

import (
	"math"
	"sort"
	"strings"

	"cubicle/services/ontology-service/internal/graphql/model"
)

func workProgramBriefModel(
	summary *model.WorkProgramSummary,
	persistedSnapshot *workProgramBriefSnapshotData,
	sections []*model.WorkstreamStandupSection,
	evaluation *model.WorkInsightEvaluation,
	persistedQualityGates []*model.WorkProgramBriefQualityGate,
	persistedAutomationReadiness *model.WorkProgramAutomationReadiness,
	persistedRiskDrivers []*model.WorkProgramBriefRiskDriver,
	persistedChecks []*model.WorkProgramAdversarialCheck,
	persistedCaveats []*model.WorkProgramBriefCaveat,
	persistedEvidenceNeeds []*model.WorkProgramAutomationEvidenceNeed,
	persistedFunctionReadiness []*model.WorkProgramTpmFunctionReadiness,
) *model.WorkProgramBrief {
	qualityGates := persistedQualityGates
	if len(qualityGates) == 0 {
		qualityGates = workProgramBriefQualityGates(summary, evaluation)
	}
	riskDriverBuckets := workProgramBriefRiskDriverBuckets(summary)
	if len(persistedRiskDrivers) > 0 {
		riskDriverBuckets = workProgramBriefRiskDriverBucketsForDrivers(persistedRiskDrivers)
	}
	adversarialChecks := persistedChecks
	if len(adversarialChecks) == 0 {
		adversarialChecks = workProgramAdversarialChecks(
			summary,
			sections,
			evaluation,
			qualityGates,
		)
	}
	tpmFunctionReadiness := persistedFunctionReadiness
	if len(tpmFunctionReadiness) == 0 {
		tpmFunctionReadiness = workProgramTPMFunctionReadiness(
			summary,
			sections,
			evaluation,
			qualityGates,
		)
	}
	automationReadiness := persistedAutomationReadiness
	if automationReadiness == nil {
		automationReadiness = workProgramAutomationReadiness(summary, sections, evaluation, qualityGates, persistedEvidenceNeeds)
	}
	caveats := persistedCaveats
	if len(caveats) == 0 {
		caveats = workProgramBriefCaveats(summary, evaluation)
	}
	generatedAt := workProgramBriefGeneratedAt(sections)
	operatingStatus := summary.OperatingStatus
	decisionPressure := summary.DecisionPressure
	forecastState := summary.ForecastState
	primaryRisk := summary.PrimaryRisk
	executiveSummary := workProgramExecutiveSummary(summary)
	recommendedFocus := summary.RecommendedFocus
	nextCadenceFocus := workProgramNextCadenceFocus(summary, sections)
	capabilityGaps := summary.CapabilityGaps
	if persistedSnapshot != nil {
		if persistedSnapshot.GeneratedAt != nil && *persistedSnapshot.GeneratedAt != "" {
			generatedAt = persistedSnapshot.GeneratedAt
		}
		if persistedSnapshot.OperatingStatus != "" {
			operatingStatus = persistedSnapshot.OperatingStatus
		}
		if persistedSnapshot.DecisionPressure != "" {
			decisionPressure = persistedSnapshot.DecisionPressure
		}
		if persistedSnapshot.ForecastState != "" {
			forecastState = persistedSnapshot.ForecastState
		}
		if persistedSnapshot.PrimaryRisk != nil && *persistedSnapshot.PrimaryRisk != "" {
			primaryRisk = persistedSnapshot.PrimaryRisk
		}
		if persistedSnapshot.ExecutiveSummary != "" {
			executiveSummary = persistedSnapshot.ExecutiveSummary
		}
		if persistedSnapshot.RecommendedFocus != "" {
			recommendedFocus = persistedSnapshot.RecommendedFocus
		}
		if persistedSnapshot.NextCadenceFocus != "" {
			nextCadenceFocus = persistedSnapshot.NextCadenceFocus
		}
		if len(persistedSnapshot.CapabilityGaps) > 0 {
			capabilityGaps = persistedSnapshot.CapabilityGaps
		}
	}
	return &model.WorkProgramBrief{
		SourceInstance:       summary.SourceInstance,
		WorkstreamKey:        summary.WorkstreamKey,
		GeneratedAt:          generatedAt,
		OperatingStatus:      operatingStatus,
		DecisionPressure:     decisionPressure,
		ForecastState:        forecastState,
		PrimaryRisk:          primaryRisk,
		ExecutiveSummary:     executiveSummary,
		RecommendedFocus:     recommendedFocus,
		NextCadenceFocus:     nextCadenceFocus,
		CapabilityGaps:       capabilityGaps,
		Summary:              summary,
		StandupSections:      sections,
		ImmediateActions:     workProgramImmediateSections(sections),
		ValidationQueue:      workProgramValidationSections(sections),
		RiskDrivers:          workProgramBriefRiskDrivers(riskDriverBuckets),
		RiskDriverBuckets:    riskDriverBuckets,
		ForecastReadiness:    summary.ForecastReadiness,
		InsightEvaluation:    evaluation,
		AutomationReadiness:  automationReadiness,
		TpmFunctionReadiness: tpmFunctionReadiness,
		AdversarialChecks:    adversarialChecks,
		QualityGates:         qualityGates,
		Caveats:              caveats,
		Badges:               summary.Badges,
	}
}

func workProgramBriefGeneratedAt(sections []*model.WorkstreamStandupSection) *string {
	for _, section := range sections {
		if section.GeneratedAt != nil && *section.GeneratedAt != "" {
			return section.GeneratedAt
		}
	}
	return nil
}

func workProgramExecutiveSummary(summary *model.WorkProgramSummary) string {
	parts := []string{summary.OperatingStatus + ": " + workProgramCountPhrase(summary.TotalCount, "typed program item")}
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
	if summary.ForecastState == "gated" {
		parts = append(parts, "ETA forecast gated")
	}
	return strings.Join(parts, "; ") + "."
}

func workProgramNextCadenceFocus(summary *model.WorkProgramSummary, sections []*model.WorkstreamStandupSection) string {
	switch {
	case summary.ActiveBlockerCount > 0 || summary.ActiveBlockerImpactCount > 0:
		return "Run blocker review, assign owners, and close the highest-impact dependency actions before treating ETA output as a plan."
	case len(workProgramImmediateSections(sections)) > 0:
		return "Drive immediate product actions, then re-check validation and source coverage before the next standup."
	case summary.ValidationLeadCount > 0 || len(workProgramValidationSections(sections)) > 0:
		return "Spend the next review cycle validating generated leads and suppressing low-confidence signals."
	case summary.ForecastState == "gated":
		return "Keep forecast output as risk triage until readiness gates clear."
	default:
		return "Maintain watch and refresh the typed operating brief on the next source sync."
	}
}

func workProgramImmediateSections(sections []*model.WorkstreamStandupSection) []*model.WorkstreamStandupSection {
	out := []*model.WorkstreamStandupSection{}
	for _, section := range sections {
		if section.SectionKind == "product_action" || section.SectionKind == "source_repair" || section.Urgency == "critical" || section.Urgency == "high" {
			out = append(out, section)
		}
	}
	return out
}

func workProgramValidationSections(sections []*model.WorkstreamStandupSection) []*model.WorkstreamStandupSection {
	out := []*model.WorkstreamStandupSection{}
	for _, section := range sections {
		switch section.SectionKind {
		case "validation_lead", "model_quality", "model_or_rule_qa", "closeout_review":
			out = append(out, section)
		}
	}
	return out
}

func workProgramBriefRiskDriverBuckets(summary *model.WorkProgramSummary) []*model.WorkProgramBriefRiskDriverBucket {
	return workProgramBriefRiskDriverBucketsForDrivers(workProgramAllBriefRiskDrivers(summary))
}

func workProgramBriefRiskDriverBucketsForDrivers(drivers []*model.WorkProgramBriefRiskDriver) []*model.WorkProgramBriefRiskDriverBucket {
	byKind := map[string][]*model.WorkProgramBriefRiskDriver{}
	for _, driver := range drivers {
		if driver == nil {
			continue
		}
		byKind[driver.DriverKind] = append(byKind[driver.DriverKind], driver)
	}
	buckets := make([]*model.WorkProgramBriefRiskDriverBucket, 0, len(byKind))
	for kind, drivers := range byKind {
		workProgramSortBriefRiskDrivers(drivers)
		buckets = append(buckets, &model.WorkProgramBriefRiskDriverBucket{
			DriverKind:      kind,
			Title:           workProgramRiskDriverBucketTitle(kind),
			RiskDriverCount: len(drivers),
			TopRiskDrivers:  drivers,
		})
	}
	sort.SliceStable(buckets, func(i, j int) bool {
		leftRank := workProgramRiskDriverKindRank(buckets[i].DriverKind)
		rightRank := workProgramRiskDriverKindRank(buckets[j].DriverKind)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftTop := workProgramRiskDriverBucketTopScore(buckets[i])
		rightTop := workProgramRiskDriverBucketTopScore(buckets[j])
		if leftTop != rightTop {
			return leftTop > rightTop
		}
		return buckets[i].DriverKind < buckets[j].DriverKind
	})
	return buckets
}

func workProgramBriefRiskDrivers(buckets []*model.WorkProgramBriefRiskDriverBucket) []*model.WorkProgramBriefRiskDriver {
	const limit = 10
	const perKindFloor = 2
	out := []*model.WorkProgramBriefRiskDriver{}
	seen := map[string]bool{}
	appendDriver := func(driver *model.WorkProgramBriefRiskDriver) {
		if driver == nil || len(out) >= limit || seen[driver.Key] {
			return
		}
		seen[driver.Key] = true
		out = append(out, driver)
	}
	for pass := 0; pass < perKindFloor && len(out) < limit; pass++ {
		for _, bucket := range buckets {
			if bucket == nil || pass >= len(bucket.TopRiskDrivers) {
				continue
			}
			appendDriver(bucket.TopRiskDrivers[pass])
		}
	}
	remainder := []*model.WorkProgramBriefRiskDriver{}
	for _, bucket := range buckets {
		if bucket == nil {
			continue
		}
		for _, driver := range bucket.TopRiskDrivers {
			if driver != nil && !seen[driver.Key] {
				remainder = append(remainder, driver)
			}
		}
	}
	workProgramSortBriefRiskDrivers(remainder)
	for _, driver := range remainder {
		appendDriver(driver)
	}
	return out
}

func workProgramAllBriefRiskDrivers(summary *model.WorkProgramSummary) []*model.WorkProgramBriefRiskDriver {
	drivers := []*model.WorkProgramBriefRiskDriver{}
	for _, blocker := range summary.TopBlockers {
		drivers = append(drivers, &model.WorkProgramBriefRiskDriver{
			Key:               blocker.Key,
			DriverKind:        "blocker",
			SubjectKey:        optionalString(blocker.SubjectKey),
			Title:             blocker.Title,
			Status:            blocker.BlockerState,
			RecommendedAction: blocker.RecommendedAction,
			EvidenceRef:       blocker.EvidenceRef,
			RankScore:         blocker.RankScore,
			Badges:            blocker.Badges,
		})
	}
	for _, impact := range summary.TopBlockerImpacts {
		drivers = append(drivers, &model.WorkProgramBriefRiskDriver{
			Key:               impact.Key,
			DriverKind:        "blocker_impact",
			SubjectKey:        optionalString(impact.AffectedKey),
			Title:             impact.Title,
			Status:            impact.ImpactState,
			RecommendedAction: impact.RecommendedAction,
			EvidenceRef:       impact.EvidenceRef,
			RankScore:         impact.ImpactScore,
			Badges:            impact.Badges,
		})
	}
	for _, dependency := range summary.TopDependencies {
		drivers = append(drivers, &model.WorkProgramBriefRiskDriver{
			Key:               dependency.Key,
			DriverKind:        "dependency",
			SubjectKey:        optionalString(dependency.FromKey),
			Title:             edgeKindLabelString(dependency.EdgeKind),
			Status:            dependency.EdgeKind,
			RecommendedAction: workProgramDependencyDriverAction(dependency),
			EvidenceRef:       dependency.EvidenceRef,
			RankScore:         dependency.RankScore,
			Badges:            dependency.Badges,
		})
	}
	for _, forecast := range summary.TopForecasts {
		drivers = append(drivers, &model.WorkProgramBriefRiskDriver{
			Key:               forecast.Key,
			DriverKind:        "forecast_risk",
			SubjectKey:        optionalString(forecast.SubjectKey),
			Title:             workProgramForecastDriverTitle(forecast),
			Status:            forecast.ActionabilityState,
			RecommendedAction: optionalString(forecast.RecommendedAction),
			EvidenceRef:       forecast.EvidenceRef,
			RankScore:         workProgramForecastDriverRankScore(forecast),
			Badges:            forecast.Badges,
		})
	}
	for _, ownerLoad := range summary.TopOwnerLoads {
		if !workProgramOwnerLoadDriverApplies(ownerLoad) {
			continue
		}
		drivers = append(drivers, &model.WorkProgramBriefRiskDriver{
			Key:               ownerLoad.Key,
			DriverKind:        "owner_load",
			SubjectKey:        optionalString(ownerLoad.OwnerKey),
			Title:             workProgramOwnerLoadDriverTitle(ownerLoad),
			Status:            ownerLoad.LoadStatus,
			RecommendedAction: workProgramOwnerLoadDriverAction(ownerLoad),
			EvidenceRef:       ownerLoad.EvidenceRef,
			RankScore:         workProgramOwnerLoadDriverRankScore(ownerLoad),
			Badges:            ownerLoad.Badges,
		})
	}
	workProgramSortBriefRiskDrivers(drivers)
	return drivers
}

func workProgramOwnerLoadDriverApplies(row *model.WorkOwnerLoadSnapshot) bool {
	if row == nil {
		return false
	}
	switch row.LoadStatus {
	case "overloaded", "attention_required":
		return true
	default:
		return row.OwnerKey == "(unassigned)" && row.ActionCount > 0
	}
}

func workProgramSortBriefRiskDrivers(drivers []*model.WorkProgramBriefRiskDriver) {
	sort.SliceStable(drivers, func(i, j int) bool {
		if drivers[i].RankScore != drivers[j].RankScore {
			return drivers[i].RankScore > drivers[j].RankScore
		}
		if drivers[i].DriverKind != drivers[j].DriverKind {
			return drivers[i].DriverKind < drivers[j].DriverKind
		}
		return drivers[i].Key < drivers[j].Key
	})
}

func workProgramRiskDriverBucketTopScore(bucket *model.WorkProgramBriefRiskDriverBucket) float64 {
	if bucket == nil || len(bucket.TopRiskDrivers) == 0 || bucket.TopRiskDrivers[0] == nil {
		return 0
	}
	return bucket.TopRiskDrivers[0].RankScore
}

func workProgramRiskDriverKindRank(kind string) int {
	switch kind {
	case "blocker_impact":
		return 0
	case "blocker":
		return 1
	case "dependency":
		return 2
	case "owner_load":
		return 3
	case "forecast_risk":
		return 4
	default:
		return 100
	}
}

func workProgramRiskDriverBucketTitle(kind string) string {
	switch kind {
	case "blocker_impact":
		return "Blocker impacts"
	case "blocker":
		return "Blockers"
	case "dependency":
		return "Dependencies"
	case "owner_load":
		return "Owner load"
	case "forecast_risk":
		return "Forecast risks"
	default:
		return "Other risks"
	}
}

func workProgramForecastDriverTitle(forecast *model.WorkItemForecast) string {
	if forecast == nil {
		return "Forecast risk"
	}
	if forecast.SubjectTitle != nil && *forecast.SubjectTitle != "" {
		return *forecast.SubjectTitle
	}
	if forecast.SubjectKey != "" {
		return "Forecast risk: " + forecast.SubjectKey
	}
	return "Forecast risk"
}

func workProgramForecastDriverRankScore(forecast *model.WorkItemForecast) float64 {
	if forecast == nil {
		return 0
	}
	score := forecast.RiskScore
	if forecast.OverdueDays != nil && *forecast.OverdueDays > 0 {
		score += math.Min(*forecast.OverdueDays, 100)
	}
	return score
}

func workProgramOwnerLoadDriverTitle(row *model.WorkOwnerLoadSnapshot) string {
	if row == nil {
		return "Owner load"
	}
	if row.OwnerKey == "(unassigned)" {
		return "Owner load: unassigned actions"
	}
	if row.OwnerDisplayName != nil && *row.OwnerDisplayName != "" {
		return "Owner load: " + *row.OwnerDisplayName
	}
	if row.OwnerKey != "" {
		return "Owner load: " + row.OwnerKey
	}
	return "Owner load"
}

func workProgramOwnerLoadDriverAction(row *model.WorkOwnerLoadSnapshot) *string {
	if row == nil {
		return nil
	}
	if row.RecommendedFocus != nil && *row.RecommendedFocus != "" {
		return row.RecommendedFocus
	}
	if row.OwnerKey == "(unassigned)" {
		return optionalString("Assign unowned TPM actions before treating the workstream plan as executable.")
	}
	return optionalString("Rebalance overloaded owner queues or explicitly accept the owner concentration.")
}

func workProgramOwnerLoadDriverRankScore(row *model.WorkOwnerLoadSnapshot) float64 {
	if row == nil {
		return 0
	}
	score := row.MaxPriorityScore + float64(row.ActionCount*5)
	switch row.LoadStatus {
	case "overloaded":
		score += 15
	case "attention_required":
		score += 8
	}
	if row.OwnerKey == "(unassigned)" {
		score += 5
	}
	return score
}

func workProgramBriefQualityGates(summary *model.WorkProgramSummary, evaluation *model.WorkInsightEvaluation) []*model.WorkProgramBriefQualityGate {
	gates := []*model.WorkProgramBriefQualityGate{}
	if summary.ForecastReadiness != nil && summary.ForecastReadiness.EtaForecastReady {
		gates = append(gates, workProgramBriefGate("forecast_readiness", "passed", false, "ETA forecast readiness gates passed.", "Continue backtesting forecasts against observed outcomes."))
	} else {
		detail := "ETA forecast remains gated."
		if summary.ForecastReadiness != nil && summary.ForecastReadiness.ReadinessReason != nil && *summary.ForecastReadiness.ReadinessReason != "" {
			detail = *summary.ForecastReadiness.ReadinessReason
		}
		gates = append(gates, workProgramBriefGate("forecast_readiness", "gated", true, detail, "Use forecast output as risk triage, not an ETA commitment."))
	}
	productActionMeasurementReady := workProgramAllProductCandidateKindsReady(evaluation)
	if productActionMeasurementReady {
		gates = append(gates, workProgramBriefGate("measurement_precision", "passed", false, "Product-action precision and useful-signal rates meet kind-level thresholds.", "Keep labeling fresh product-action-backed insights."))
	} else {
		detail := workProgramProductActionMeasurementGateDetail(evaluation, "precision")
		gates = append(gates, workProgramBriefGate("measurement_precision", "gated", true, detail, "Gold-label product-action candidate insight kinds and keep context-only signals as validation leads."))
	}
	if productActionMeasurementReady {
		gates = append(gates, workProgramBriefGate("measurement_actionability", "passed", false, "Product-action actionability meets kind-level thresholds.", "Keep actionability labels current for product-action-backed insights."))
	} else {
		detail := workProgramProductActionMeasurementGateDetail(evaluation, "actionability")
		gates = append(gates, workProgramBriefGate("measurement_actionability", "gated", true, detail, "Add actionability labels for product-action candidate insight kinds before claiming replacement-level automation."))
	}
	if summary.SourceCoverageLimitedCount == 0 {
		gates = append(gates, workProgramBriefGate("source_coverage", "passed", false, "No typed program item in scope reports limited source coverage.", "Continue preserving source coverage state on every sync."))
	} else {
		gates = append(gates, workProgramBriefGate("source_coverage", "gated", true, workProgramCountPhrase(summary.SourceCoverageLimitedCount, "program item")+" "+workProgramHasHave(summary.SourceCoverageLimitedCount)+" limited source coverage.", "Treat affected items as review leads until coverage is complete."))
	}
	authLimitedCount := workProgramMapCount(workProgramAuthLimitedObservationCounts(summary))
	authLimitedProductDecisionCount := workProgramMapCount(workProgramAuthLimitedProductDecisionCounts(summary))
	if authLimitedCount == 0 {
		gates = append(gates, workProgramBriefGate("source_authentication", "passed", false, "No typed program item depends only on anonymous source observation.", "Keep authenticated observation available for product-sensitive claims."))
	} else if authLimitedProductDecisionCount > 0 {
		gates = append(gates, workProgramBriefGate("source_authentication", "gated", true, workProgramCountPhrase(authLimitedProductDecisionCount, "product-action or decision program item")+" "+workProgramHasHave(authLimitedProductDecisionCount)+" only anonymous/public source observation.", "Re-observe these product-decision rows with authenticated access before absence, completion, or autonomous decision claims."))
	} else {
		gates = append(gates, workProgramBriefGate("source_authentication", "watch", false, workProgramCountPhrase(authLimitedCount, "validation or QA program item")+" "+workProgramHasHave(authLimitedCount)+" only anonymous/public source observation.", "Keep these rows as lower-confidence validation leads until authenticated re-observation is attached; do not promote them to product actions first."))
	}
	generatedClaimCount := workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary))
	generatedClaimProductDecisionCount := workProgramMapCount(workProgramGeneratedClaimProductDecisionCounts(summary))
	if generatedClaimCount == 0 {
		gates = append(gates, workProgramBriefGate("claim_provenance", "passed", false, "No typed program item depends only on generated or derived claim evidence.", "Continue linking generated claims to independent source or measurement evidence."))
	} else if generatedClaimProductDecisionCount > 0 {
		gates = append(gates, workProgramBriefGate("claim_provenance", "gated", true, workProgramCountPhrase(generatedClaimProductDecisionCount, "product-action or decision program item")+" depend on generated or derived claim evidence.", "Keep generated product-decision claims in QA or validation until independent provenance or measurement evidence is attached."))
	} else {
		gates = append(gates, workProgramBriefGate("claim_provenance", "watch", false, workProgramCountPhrase(generatedClaimCount, "validation or QA program item")+" depend on generated or derived claim evidence.", "Keep generated validation claims in QA/provenance review and require independent evidence before promotion to product actions."))
	}
	if summary.OverloadedOwnerCount == 0 && summary.UnassignedActionCount == 0 {
		gates = append(gates, workProgramBriefGate("owner_load", "passed", false, "Latest owner-load rows have no overloaded owners or unassigned actions.", "Keep owner-load snapshots fresh as action volume changes."))
	} else {
		parts := []string{}
		if summary.OverloadedOwnerCount > 0 {
			parts = append(parts, workProgramCountPhrase(summary.OverloadedOwnerCount, "overloaded owner"))
		}
		if summary.UnassignedActionCount > 0 {
			parts = append(parts, workProgramCountPhrase(summary.UnassignedActionCount, "unassigned action"))
		}
		gates = append(gates, workProgramBriefGate("owner_load", "gated", true, workProgramJoinFocusParts(parts)+" remain in the latest owner-load snapshot.", "Rebalance overloaded owner queues or assign unassigned actions before treating the plan as autonomously executable."))
	}
	if summary.ActiveBlockerCount == 0 && summary.ActiveBlockerImpactCount == 0 {
		gates = append(gates, workProgramBriefGate("blocker_clearance", "passed", false, "No active blocker is in scope.", "Maintain watch on new blocker signals."))
	} else {
		gates = append(gates, workProgramBriefGate("blocker_clearance", "gated", true, workProgramCountPhrase(summary.ActiveBlockerCount, "active blocker")+" and "+workProgramCountPhrase(summary.ActiveBlockerImpactCount, "active blocker impact")+" remain in scope.", "Assign owners and clear the highest-ranked blocker impacts."))
	}
	return gates
}

func workProgramProductCandidateKindCount(evaluation *model.WorkInsightEvaluation) int {
	if evaluation == nil {
		return 0
	}
	count := 0
	for _, kind := range evaluation.Kinds {
		if kind != nil && workInsightKindMeasurementScope(kind) == workInsightMeasurementScopeProductCandidate {
			count++
		}
	}
	return count
}

func workProgramAllProductCandidateKindsReady(evaluation *model.WorkInsightEvaluation) bool {
	if evaluation == nil {
		return false
	}
	productCandidateKinds := 0
	for _, kind := range evaluation.Kinds {
		if kind == nil || workInsightKindMeasurementScope(kind) != workInsightMeasurementScopeProductCandidate {
			continue
		}
		productCandidateKinds++
		if !kind.ReadyForProductAction {
			return false
		}
	}
	return productCandidateKinds > 0
}

func workProgramProductActionMeasurementGateDetail(evaluation *model.WorkInsightEvaluation, dimension string) string {
	if evaluation == nil {
		return "Product-action insight measurement is gated by missing labels."
	}
	if workProgramProductCandidateKindCount(evaluation) == 0 {
		return "No product-action insight kind is ready for product-action measurement; context-only labels remain validation coverage."
	}
	if evaluation.QualityGatedInsightKindCount > 0 {
		return "Product-action " + dimension + " is measured but below product-action threshold for at least one candidate kind."
	}
	return "Product-action " + dimension + " measurement is gated by kind-level product-action gates."
}

func workProgramBriefGate(key string, state string, blocking bool, detail string, recommendedAction string) *model.WorkProgramBriefQualityGate {
	return &model.WorkProgramBriefQualityGate{
		Key:               key,
		GateState:         state,
		Blocking:          blocking,
		Detail:            detail,
		RecommendedAction: optionalString(recommendedAction),
	}
}

func workProgramAutomationReadiness(
	summary *model.WorkProgramSummary,
	sections []*model.WorkstreamStandupSection,
	evaluation *model.WorkInsightEvaluation,
	gates []*model.WorkProgramBriefQualityGate,
	persistedEvidenceNeeds []*model.WorkProgramAutomationEvidenceNeed,
) *model.WorkProgramAutomationReadiness {
	score := 100.0
	safeAreas := []string{}
	humanAreas := []string{}

	if len(sections) > 0 {
		safeAreas = appendUniqueString(safeAreas, "agenda_summarization")
	}
	if len(summary.TopBlockers) > 0 || len(summary.TopBlockerImpacts) > 0 || len(summary.TopDependencies) > 0 {
		safeAreas = appendUniqueString(safeAreas, "risk_driver_ranking")
	}
	if workProgramBriefHasEvidence(summary) {
		safeAreas = appendUniqueString(safeAreas, "source_citation")
	}
	if summary.ForecastState != "" {
		safeAreas = appendUniqueString(safeAreas, "forecast_triage")
	}

	if summary.ForecastReadiness == nil || !summary.ForecastReadiness.EtaForecastReady {
		score -= 25
		humanAreas = appendUniqueString(humanAreas, "eta_commitments")
	}
	if evaluation == nil || !evaluation.ReadyToMeasurePrecision {
		score -= 25
		humanAreas = appendUniqueString(humanAreas, "measurement_claims")
	}
	if evaluation == nil || !evaluation.ReadyToMeasureActionability {
		score -= 20
		humanAreas = appendUniqueString(humanAreas, "measurement_claims")
	}
	if summary.SourceCoverageLimitedCount > 0 {
		score -= 15
		humanAreas = appendUniqueString(humanAreas, "coverage_repair")
	}
	if workProgramMapCount(workProgramAuthLimitedObservationCounts(summary)) > 0 {
		score -= 10
		humanAreas = appendUniqueString(humanAreas, "source_authentication")
	}
	if workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary)) > 0 {
		score -= 10
		humanAreas = appendUniqueString(humanAreas, "claim_provenance")
	}
	if summary.ActiveBlockerCount > 0 || summary.ActiveBlockerImpactCount > 0 {
		score -= 15
		humanAreas = appendUniqueString(humanAreas, "blocker_clearance")
	}
	if summary.OverloadedOwnerCount > 0 || summary.UnassignedActionCount > 0 {
		score -= 10
		humanAreas = appendUniqueString(humanAreas, "owner_load_balancing")
	}
	if summary.ProductActionCount > 0 || summary.NeedsDecisionCount > 0 {
		humanAreas = appendUniqueString(humanAreas, "product_decisions")
	}
	if score < 0 {
		score = 0
	}

	state := "automatable"
	switch {
	case score < 35 || workProgramHasBlockingGate(gates, "measurement_precision", "measurement_actionability", "source_coverage", "source_authentication", "claim_provenance", "blocker_clearance", "owner_load"):
		state = "blocked"
	case score < 70:
		state = "supervised"
	case score < 90:
		state = "assisted"
	}
	autonomousReady := state == "automatable" && !workProgramHasAnyBlockingGate(gates)
	evidenceWorkQueue := persistedEvidenceNeeds
	if len(evidenceWorkQueue) == 0 {
		evidenceWorkQueue = workProgramAutomationEvidenceQueue(summary, evaluation, gates)
	}

	return &model.WorkProgramAutomationReadiness{
		ReadinessState:        state,
		ReadinessScore:        score,
		AutonomousActionReady: autonomousReady,
		HumanReviewRequired:   !autonomousReady,
		SafeAutomationAreas:   safeAreas,
		HumanRequiredAreas:    humanAreas,
		Rationale:             workProgramAutomationRationale(state, gates),
		RequiredEvidence:      workProgramAutomationRequiredEvidence(gates),
		EvidenceWorkQueue:     evidenceWorkQueue,
		Gates:                 gates,
	}
}

func workProgramAutomationRationale(state string, gates []*model.WorkProgramBriefQualityGate) string {
	blocking := workProgramBlockingGateKeys(gates)
	if len(blocking) == 0 {
		return "Automation can proceed from the current typed work-program evidence, with routine monitoring."
	}
	switch state {
	case "blocked":
		return "Autonomous TPM action is blocked by " + strings.Join(blocking, ", ") + "."
	case "supervised":
		return "Automation can assist, but human supervision remains required until " + strings.Join(blocking, ", ") + " clear."
	default:
		return "Automation can draft and rank work, but human review remains required while " + strings.Join(blocking, ", ") + " remain open."
	}
}

func workProgramAutomationRequiredEvidence(gates []*model.WorkProgramBriefQualityGate) []string {
	required := []string{}
	for _, gate := range gates {
		if gate == nil || !gate.Blocking {
			continue
		}
		switch gate.Key {
		case "forecast_readiness":
			required = appendUniqueString(required, "forecast backtest outcomes and readiness history")
		case "measurement_precision":
			required = appendUniqueString(required, "gold labels for generated insight precision")
		case "measurement_actionability":
			required = appendUniqueString(required, "actionability labels across current insight kinds")
		case "source_coverage":
			required = appendUniqueString(required, "source repair or required-check configuration for limited program items")
		case "source_authentication":
			required = appendUniqueString(required, "authenticated re-observation for anonymous/public source observations")
		case "claim_provenance":
			required = appendUniqueString(required, "independent source, provenance, or measurement evidence for generated claims")
		case "owner_load":
			required = appendUniqueString(required, "owner-load rebalancing or explicit assignment evidence")
		case "blocker_clearance":
			required = appendUniqueString(required, "owner-confirmed blocker clearance or acceptance")
		}
	}
	return required
}

func workProgramAutomationEvidenceQueue(summary *model.WorkProgramSummary, evaluation *model.WorkInsightEvaluation, gates []*model.WorkProgramBriefQualityGate) []*model.WorkProgramAutomationEvidenceNeed {
	queue := []*model.WorkProgramAutomationEvidenceNeed{}
	if workProgramHasBlockingGate(gates, "forecast_readiness") {
		queue = append(queue, workProgramEvidenceNeed("forecast_readiness:backtest", "forecast_readiness", "forecast_backtest", "high", "workstream", summary.WorkstreamKey, "", 0, 1, 1, nil, nil, "Produce a passing forecast backtest before using ETA commitments."))
	}
	if evaluation == nil {
		queue = append(queue, workProgramEvidenceNeed("measurement_precision:labels", "measurement_precision", "insight_labels", "high", "insight_kind", nil, "", 0, minMeasurementLabelTotal, minMeasurementLabelTotal, nil, nil, "Gold-label current generated insights before using them for product-action automation."))
	} else {
		for _, kind := range evaluation.Kinds {
			if kind == nil {
				continue
			}
			if !kind.ReadyToMeasure {
				missing := kind.RequiredLabelCount - kind.MeasurementLabelCount
				if missing < 0 {
					missing = 0
				}
				queue = append(queue, workProgramEvidenceNeed("measurement_labels:"+kind.InsightKind, "measurement_precision", "insight_labels", "high", "insight_kind", optionalString(kind.InsightKind), "", kind.MeasurementLabelCount, kind.RequiredLabelCount, missing, nil, nil, kind.RecommendedAction))
				continue
			}
			if !kind.ReadyForProductAction {
				queue = append(queue, workProgramInsightQualityNeeds(kind)...)
			}
		}
	}
	if summary.SourceCoverageLimitedCount > 0 {
		for kind, count := range workProgramSourceCoverageLimitCounts(summary) {
			queue = append(queue, workProgramSourceCoverageNeed(summary, kind, count))
		}
	}
	for kind, count := range workProgramAuthLimitedObservationCounts(summary) {
		queue = append(queue, workProgramSourceCoverageNeed(summary, kind, count))
	}
	for kind, count := range workProgramGeneratedClaimLimitCounts(summary) {
		queue = append(queue, workProgramSourceCoverageNeed(summary, kind, count))
	}
	ownerLoadWork := summary.OverloadedOwnerCount + summary.UnassignedActionCount
	if ownerLoadWork > 0 {
		queue = append(queue, workProgramEvidenceNeed("owner_load:rebalance", "owner_load", "owner_load_balancing", "high", "workstream", summary.WorkstreamKey, summary.OwnerLoadStatus, ownerLoadWork, 0, ownerLoadWork, nil, nil, "Rebalance overloaded owner queues or assign unassigned actions before treating the plan as autonomously executable."))
	}
	activeBlockerWork := summary.ActiveBlockerCount + summary.ActiveBlockerImpactCount
	if activeBlockerWork > 0 {
		queue = append(queue, workProgramEvidenceNeed("blocker_clearance:active", "blocker_clearance", "blocker_clearance", "critical", "workstream", summary.WorkstreamKey, "", 0, activeBlockerWork, activeBlockerWork, nil, nil, "Assign owners and capture blocker-clearance evidence for active blockers and impacts."))
	}
	workProgramAttachEvidenceExecution(summary, queue)
	sort.SliceStable(queue, func(i, j int) bool {
		if workProgramEvidencePriorityRank(queue[i].Priority) != workProgramEvidencePriorityRank(queue[j].Priority) {
			return workProgramEvidencePriorityRank(queue[i].Priority) < workProgramEvidencePriorityRank(queue[j].Priority)
		}
		return queue[i].Key < queue[j].Key
	})
	return queue
}

func workProgramTPMFunctionReadiness(summary *model.WorkProgramSummary, sections []*model.WorkstreamStandupSection, evaluation *model.WorkInsightEvaluation, gates []*model.WorkProgramBriefQualityGate) []*model.WorkProgramTpmFunctionReadiness {
	rows := []*model.WorkProgramTpmFunctionReadiness{}
	briefSignals := summary.TotalCount + len(sections)
	briefState := "automatable"
	briefAutomation := "can_publish_operating_brief"
	briefHumanRequired := false
	briefDetail := "Typed program rows and standup sections are available for an operating brief."
	briefAction := "Publish the operating brief and keep the source sync fresh."
	if summary.TotalCount == 0 {
		briefState = "blocked"
		briefAutomation = "missing_typed_work"
		briefHumanRequired = true
		briefDetail = "No typed program rows are in scope."
		briefAction = "Load typed work rows before expecting an AI TPM operating brief."
	} else if len(sections) == 0 {
		briefState = "assisted"
		briefAutomation = "summary_only"
		briefDetail = "Typed program rows are available, but no persisted standup sections are in scope."
		briefAction = "Persist standup sections to turn the summary into an agenda."
	}
	rows = append(rows, workProgramTPMFunction(
		"operating_brief",
		"Operating brief",
		briefState,
		briefAutomation,
		briefHumanRequired,
		briefSignals,
		nil,
		briefDetail,
		briefAction,
	))

	blockerSignals := summary.ActiveBlockerCount + summary.ActiveBlockerImpactCount + summary.NeedsActionDependencyCount
	if blockerSignals > 0 {
		rows = append(rows, workProgramTPMFunction(
			"blocker_management",
			"Blocker management",
			"supervised",
			"can_rank_and_draft",
			true,
			blockerSignals,
			workProgramTPMBlockingGateKeys(gates, "blocker_clearance"),
			workProgramCountPhrase(summary.ActiveBlockerCount, "active blocker")+" and "+workProgramCountPhrase(summary.ActiveBlockerImpactCount, "active blocker impact")+" need owner-confirmed clearance.",
			"Use ranked blockers and dependencies to drive owner decisions, but require human confirmation before declaring clearance.",
		))
	} else {
		rows = append(rows, workProgramTPMFunction(
			"blocker_management",
			"Blocker management",
			"automatable",
			"watch_ready",
			false,
			0,
			nil,
			"No active blocker or blocker impact is in scope.",
			"Maintain blocker watch on the next source sync.",
		))
	}

	forecastReady := summary.ForecastReadiness != nil && summary.ForecastReadiness.EtaForecastReady
	forecastSignals := len(summary.TopForecasts)
	if forecastReady {
		rows = append(rows, workProgramTPMFunction(
			"forecast_triage",
			"Forecast triage",
			"automatable",
			"eta_ready",
			false,
			forecastSignals,
			nil,
			"Forecast readiness gates passed for ETA-style output.",
			"Continue backtesting forecast outcomes against observed transitions.",
		))
	} else {
		rows = append(rows, workProgramTPMFunction(
			"forecast_triage",
			"Forecast triage",
			"blocked",
			"risk_triage_only",
			true,
			forecastSignals,
			workProgramTPMBlockingGateKeys(gates, "forecast_readiness"),
			"Forecast output is useful for risk ranking, not ETA commitments.",
			"Use forecast rows as TPM risk leads until readiness gates pass.",
		))
	}

	executionSignals := summary.OwnerLoadActionCount
	if summary.OverloadedOwnerCount > 0 || summary.UnassignedActionCount > 0 {
		rows = append(rows, workProgramTPMFunction(
			"execution_capacity",
			"Execution capacity",
			"blocked",
			"rebalance_required",
			true,
			executionSignals,
			workProgramTPMBlockingGateKeys(gates, "owner_load"),
			workProgramCountPhrase(summary.OverloadedOwnerCount, "overloaded owner")+" and "+workProgramCountPhrase(summary.UnassignedActionCount, "unassigned action")+" constrain execution.",
			"Rebalance overloaded owner queues or assign unowned actions before treating the plan as autonomously executable.",
		))
	} else if executionSignals > 0 {
		rows = append(rows, workProgramTPMFunction(
			"execution_capacity",
			"Execution capacity",
			"assisted",
			"owner_queue_ready",
			false,
			executionSignals,
			nil,
			"Owner-load rows are present with no overloaded owners or unassigned actions.",
			"Use owner queues to follow through on open TPM actions.",
		))
	} else {
		rows = append(rows, workProgramTPMFunction(
			"execution_capacity",
			"Execution capacity",
			"watch",
			"no_open_owner_load",
			false,
			0,
			nil,
			"No owner-load work is currently open.",
			"Refresh owner-load snapshots on the next action generation run.",
		))
	}

	if summary.SourceCoverageLimitedCount > 0 {
		rows = append(rows, workProgramTPMFunction(
			"source_coverage",
			"Source coverage",
			"blocked",
			"coverage_repair_required",
			true,
			summary.SourceCoverageLimitedCount,
			workProgramTPMBlockingGateKeys(gates, "source_coverage"),
			workProgramCountPhrase(summary.SourceCoverageLimitedCount, "program item")+" "+workProgramHasHave(summary.SourceCoverageLimitedCount)+" limited source coverage.",
			"Repair source coverage before using affected rows for absence claims or autonomous decisions.",
		))
	} else {
		rows = append(rows, workProgramTPMFunction(
			"source_coverage",
			"Source coverage",
			"automatable",
			"coverage_ready",
			false,
			summary.TotalCount,
			nil,
			"No typed program item in scope reports limited source coverage.",
			"Keep preserving source coverage on every sync.",
		))
	}

	qualitySignals := 0
	if evaluation != nil {
		qualitySignals = evaluation.MeasurementLabelCount
	}
	if workProgramAllProductCandidateKindsReady(evaluation) {
		rows = append(rows, workProgramTPMFunction(
			"insight_quality",
			"Insight QA",
			"automatable",
			"measurement_ready",
			false,
			qualitySignals,
			nil,
			"Product-action insight precision and actionability have enough labels and meet thresholds.",
			"Use measured kind-level product-action gates to decide which signals can become product actions; keep global/context labels as validation coverage.",
		))
	} else {
		rows = append(rows, workProgramTPMFunction(
			"insight_quality",
			"Insight QA",
			"blocked",
			"labels_required",
			true,
			qualitySignals,
			workProgramTPMBlockingGateKeys(gates, "measurement_precision", "measurement_actionability"),
			"Product-action insight quality is not fully measurement-ready.",
			"Add product-action scoped truth/actionability labels before claiming autonomous TPM replacement.",
		))
	}

	decisionSignals := summary.ProductActionCount + summary.NeedsDecisionCount
	if decisionSignals > 0 {
		rows = append(rows, workProgramTPMFunction(
			"product_decisions",
			"Product decisions",
			"supervised",
			"human_decision_required",
			true,
			decisionSignals,
			nil,
			workProgramCountPhrase(decisionSignals, "product decision signal")+" require owner judgment.",
			"Draft the decision request, but require a human owner to merge, close, park, or reassign.",
		))
	} else {
		rows = append(rows, workProgramTPMFunction(
			"product_decisions",
			"Product decisions",
			"assisted",
			"no_open_product_decisions",
			false,
			0,
			nil,
			"No product-decision action is currently open.",
			"Continue monitoring for new decision or owner follow-up signals.",
		))
	}
	return rows
}

func workProgramTPMFunction(functionKey string, functionName string, readinessState string, automationState string, humanRequired bool, supportingSignalCount int, blockingGateKeys []string, detail string, recommendedAction string) *model.WorkProgramTpmFunctionReadiness {
	return &model.WorkProgramTpmFunctionReadiness{
		FunctionKey:           functionKey,
		FunctionName:          functionName,
		ReadinessState:        readinessState,
		AutomationState:       automationState,
		HumanRequired:         humanRequired,
		SupportingSignalCount: supportingSignalCount,
		BlockingGateKeys:      blockingGateKeys,
		Detail:                detail,
		RecommendedAction:     recommendedAction,
	}
}

func workProgramTPMBlockingGateKeys(gates []*model.WorkProgramBriefQualityGate, keys ...string) []string {
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[key] = true
	}
	out := []string{}
	for _, gate := range gates {
		if gate != nil && gate.Blocking && wanted[gate.Key] {
			out = append(out, gate.Key)
		}
	}
	return out
}

func workProgramAdversarialChecks(summary *model.WorkProgramSummary, sections []*model.WorkstreamStandupSection, evaluation *model.WorkInsightEvaluation, gates []*model.WorkProgramBriefQualityGate) []*model.WorkProgramAdversarialCheck {
	checks := []*model.WorkProgramAdversarialCheck{}
	if summary.TotalCount == 0 {
		checks = append(checks, workProgramAdversarialCheck("brief_basis", "operating_brief", "fail", "critical", "No typed work basis", "The brief has no typed program rows, so any TPM summary would be speculative.", "Load typed program rows before publishing an AI TPM brief.", nil, nil))
	} else if len(sections) == 0 {
		checks = append(checks, workProgramAdversarialCheck("brief_basis", "operating_brief", "warning", "medium", "Agenda not persisted", "Typed program rows exist, but there are no persisted standup sections to anchor a TPM agenda.", "Persist standup sections before treating the brief as meeting-ready.", nil, workProgramTopItemEvidenceRefs(summary)))
	} else {
		checks = append(checks, workProgramAdversarialCheck("brief_basis", "operating_brief", "pass", "info", "Typed brief basis present", "Typed program rows and standup sections are present.", "Keep refreshing the typed source sync and standup generation.", nil, workProgramTopItemEvidenceRefs(summary)))
	}

	if summary.ForecastReadiness == nil || !summary.ForecastReadiness.EtaForecastReady {
		checks = append(checks, workProgramAdversarialCheck("forecast_overclaim", "forecast", "fail", "critical", "ETA overclaim risk", "Forecast rows can rank risk but cannot be presented as ETA commitments.", "Keep forecasts framed as risk triage until forecast readiness gates pass.", workProgramTPMBlockingGateKeys(gates, "forecast_readiness"), workProgramForecastEvidenceRefs(summary)))
	} else {
		checks = append(checks, workProgramAdversarialCheck("forecast_overclaim", "forecast", "pass", "info", "Forecast readiness passed", "Forecast readiness gates are passing for this source scope.", "Continue backtesting forecast output against observed transitions.", nil, workProgramForecastEvidenceRefs(summary)))
	}

	if summary.SourceCoverageLimitedCount > 0 {
		checks = append(checks, workProgramAdversarialCheck("source_absence_claims", "source_coverage", "fail", "critical", "Absence claims unsafe", workProgramCountPhrase(summary.SourceCoverageLimitedCount, "program item")+" "+workProgramHasHave(summary.SourceCoverageLimitedCount)+" limited source coverage.", "Do not claim absence or completion for affected rows until source coverage is repaired.", workProgramTPMBlockingGateKeys(gates, "source_coverage"), nil))
	} else {
		checks = append(checks, workProgramAdversarialCheck("source_absence_claims", "source_coverage", "pass", "info", "Source coverage clear", "No typed program item in scope reports limited source coverage.", "Keep coverage state attached to source-backed rows.", nil, nil))
	}
	authLimitedCount := workProgramMapCount(workProgramAuthLimitedObservationCounts(summary))
	if authLimitedCount > 0 {
		checks = append(checks, workProgramAdversarialCheck("source_authentication_claims", "source_authentication", "warning", "high", "Anonymous observation boundary", workProgramCountPhrase(authLimitedCount, "program item")+" "+workProgramHasHave(authLimitedCount)+" successful but anonymous source observation.", "Do not use anonymous-only observations for absence, completion, or autonomous decision claims until authenticated re-observation is attached.", workProgramTPMBlockingGateKeys(gates, "source_authentication"), nil))
	} else {
		checks = append(checks, workProgramAdversarialCheck("source_authentication_claims", "source_authentication", "pass", "info", "Authenticated observation boundary clear", "No typed program item depends only on anonymous source observation.", "Keep auth state attached to source-backed rows.", nil, nil))
	}
	generatedClaimCount := workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary))
	if generatedClaimCount > 0 {
		checks = append(checks, workProgramAdversarialCheck("generated_claim_provenance", "claim_provenance", "warning", "high", "Generated claim provenance boundary", workProgramCountPhrase(generatedClaimCount, "program item")+" depend on generated or derived claim evidence.", "Keep generated claims in validation or QA until independent source/provenance evidence is attached.", workProgramTPMBlockingGateKeys(gates, "claim_provenance"), nil))
	} else {
		checks = append(checks, workProgramAdversarialCheck("generated_claim_provenance", "claim_provenance", "pass", "info", "Generated claim provenance clear", "No typed program item depends only on generated or derived claim evidence.", "Continue preserving independent provenance for generated claims.", nil, nil))
	}

	if !workProgramAllProductCandidateKindsReady(evaluation) {
		checks = append(checks, workProgramAdversarialCheck("measurement_overclaim", "measurement", "fail", "critical", "Product-action insight quality overclaim risk", "Product-action precision/actionability measurement is not fully ready.", "Add product-action scoped truth/actionability labels before claiming autonomous TPM replacement.", workProgramTPMBlockingGateKeys(gates, "measurement_precision", "measurement_actionability"), nil))
	} else {
		checks = append(checks, workProgramAdversarialCheck("measurement_overclaim", "measurement", "pass", "info", "Product-action insight quality measured", "Product-action precision and actionability are measurement-ready.", "Use measured kind-level product-action gates to decide which signal kinds can become product actions.", nil, nil))
	}

	if summary.OverloadedOwnerCount > 0 || summary.UnassignedActionCount > 0 {
		checks = append(checks, workProgramAdversarialCheck("execution_assumption", "execution_capacity", "fail", "high", "Execution capacity overclaim risk", workProgramCountPhrase(summary.OverloadedOwnerCount, "overloaded owner")+" and "+workProgramCountPhrase(summary.UnassignedActionCount, "unassigned action")+" make autonomous execution unsafe.", "Rebalance overloaded queues or assign unowned actions before treating the plan as executable.", workProgramTPMBlockingGateKeys(gates, "owner_load"), workProgramOwnerLoadEvidenceRefs(summary)))
	} else {
		checks = append(checks, workProgramAdversarialCheck("execution_assumption", "execution_capacity", "pass", "info", "Execution capacity has no owner-load blocker", "Owner-load rows do not show overloaded owners or unassigned actions.", "Keep owner-load snapshots fresh as actions change.", nil, workProgramOwnerLoadEvidenceRefs(summary)))
	}

	if summary.ActiveBlockerCount > 0 || summary.ActiveBlockerImpactCount > 0 {
		checks = append(checks, workProgramAdversarialCheck("blocker_clearance_claim", "blocker_clearance", "fail", "critical", "Blocker clearance overclaim risk", workProgramCountPhrase(summary.ActiveBlockerCount, "active blocker")+" and "+workProgramCountPhrase(summary.ActiveBlockerImpactCount, "active blocker impact")+" remain open.", "Require owner-confirmed clearance before declaring the workstream unblocked.", workProgramTPMBlockingGateKeys(gates, "blocker_clearance"), workProgramBlockerEvidenceRefs(summary)))
	} else {
		checks = append(checks, workProgramAdversarialCheck("blocker_clearance_claim", "blocker_clearance", "pass", "info", "No active blocker clearance claim at risk", "No active blocker or blocker impact is in scope.", "Maintain blocker watch on future syncs.", nil, workProgramBlockerEvidenceRefs(summary)))
	}

	decisionSignals := summary.ProductActionCount + summary.NeedsDecisionCount
	if decisionSignals > 0 {
		checks = append(checks, workProgramAdversarialCheck("human_decision_boundary", "product_decision", "warning", "high", "Human decision boundary", workProgramCountPhrase(decisionSignals, "product decision signal")+" require owner judgment.", "Draft decision requests, but do not automate merge, close, park, or owner reassignment decisions.", nil, nil))
	} else {
		checks = append(checks, workProgramAdversarialCheck("human_decision_boundary", "product_decision", "pass", "info", "No product decision boundary currently open", "No product-decision signal is currently open.", "Continue monitoring decision signals.", nil, nil))
	}
	return checks
}

func workProgramAdversarialCheck(key string, checkKind string, checkState string, severity string, title string, detail string, recommendedAction string, blockingGateKeys []string, evidenceRefs []string) *model.WorkProgramAdversarialCheck {
	return &model.WorkProgramAdversarialCheck{
		Key:               key,
		CheckKind:         checkKind,
		CheckState:        checkState,
		Severity:          severity,
		Title:             title,
		Detail:            detail,
		RecommendedAction: recommendedAction,
		BlockingGateKeys:  workProgramUniqueStrings(blockingGateKeys),
		EvidenceRefs:      workProgramUniqueStrings(evidenceRefs),
	}
}

func workProgramTopItemEvidenceRefs(summary *model.WorkProgramSummary) []string {
	refs := []string{}
	for _, item := range summary.TopItems {
		if item != nil && item.EvidenceRef != nil && *item.EvidenceRef != "" {
			refs = append(refs, *item.EvidenceRef)
		}
	}
	return refs
}

func workProgramForecastEvidenceRefs(summary *model.WorkProgramSummary) []string {
	refs := []string{}
	if summary.ForecastReadiness != nil && summary.ForecastReadiness.EvidenceRef != nil && *summary.ForecastReadiness.EvidenceRef != "" {
		refs = append(refs, *summary.ForecastReadiness.EvidenceRef)
	}
	for _, forecast := range summary.TopForecasts {
		if forecast != nil && forecast.EvidenceRef != nil && *forecast.EvidenceRef != "" {
			refs = append(refs, *forecast.EvidenceRef)
		}
	}
	return refs
}

func workProgramOwnerLoadEvidenceRefs(summary *model.WorkProgramSummary) []string {
	refs := []string{}
	for _, ownerLoad := range summary.TopOwnerLoads {
		if ownerLoad != nil && ownerLoad.EvidenceRef != nil && *ownerLoad.EvidenceRef != "" {
			refs = append(refs, *ownerLoad.EvidenceRef)
		}
	}
	return refs
}

func workProgramBlockerEvidenceRefs(summary *model.WorkProgramSummary) []string {
	refs := []string{}
	for _, blocker := range summary.TopBlockers {
		if blocker != nil && blocker.EvidenceRef != nil && *blocker.EvidenceRef != "" {
			refs = append(refs, *blocker.EvidenceRef)
		}
	}
	for _, impact := range summary.TopBlockerImpacts {
		if impact != nil && impact.EvidenceRef != nil && *impact.EvidenceRef != "" {
			refs = append(refs, *impact.EvidenceRef)
		}
	}
	return refs
}

func workProgramUniqueStrings(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func workProgramSourceCoverageLimitCounts(summary *model.WorkProgramSummary) map[string]int {
	counts := map[string]int{}
	for _, breakdown := range summary.Breakdowns {
		if breakdown == nil || breakdown.Dimension != workProgramSourceCoverageLimitDimension || breakdown.Count <= 0 {
			continue
		}
		counts[breakdown.Key] += breakdown.Count
	}
	if len(counts) == 0 && summary.SourceCoverageLimitedCount > 0 {
		counts["coverage_limited"] = summary.SourceCoverageLimitedCount
	}
	return counts
}

func workProgramAuthLimitedObservationCounts(summary *model.WorkProgramSummary) map[string]int {
	return workProgramBreakdownCounts(summary, workProgramAuthLimitedObservationDimension)
}

func workProgramAuthLimitedProductDecisionCounts(summary *model.WorkProgramSummary) map[string]int {
	return workProgramBreakdownCounts(summary, workProgramAuthLimitedProductDecisionDimension)
}

func workProgramGeneratedClaimLimitCounts(summary *model.WorkProgramSummary) map[string]int {
	return workProgramBreakdownCounts(summary, workProgramGeneratedClaimLimitDimension)
}

func workProgramGeneratedClaimProductDecisionCounts(summary *model.WorkProgramSummary) map[string]int {
	return workProgramBreakdownCounts(summary, workProgramGeneratedClaimProductDecisionDimension)
}

func workProgramBreakdownCounts(summary *model.WorkProgramSummary, dimension string) map[string]int {
	counts := map[string]int{}
	for _, breakdown := range summary.Breakdowns {
		if breakdown == nil || breakdown.Dimension != dimension || breakdown.Count <= 0 {
			continue
		}
		counts[breakdown.Key] += breakdown.Count
	}
	return counts
}

func workProgramMapCount(counts map[string]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}

func workProgramSourceCoverageNeed(summary *model.WorkProgramSummary, limitKind string, count int) *model.WorkProgramAutomationEvidenceNeed {
	gateKey := workProgramSourceCoverageLimitGateKey(limitKind)
	evidenceKind := "source_coverage"
	priority := "medium"
	recommendedAction := "Refresh limited source rows before using affected items for absence claims or autonomous decisions."
	switch limitKind {
	case "anonymous_observation":
		evidenceKind = "source_authentication"
		recommendedAction = "Re-observe anonymous rows with authenticated source access, or keep them as lower-confidence review leads."
	case "required_check_coverage_unavailable":
		evidenceKind = "required_check_coverage"
		recommendedAction = "Capture branch protection or required-check configuration before promoting CI follow-ups."
	case "generated_evidence":
		evidenceKind = "generated_evidence"
		recommendedAction = "Keep generated or derived claim evidence in QA/provenance review, not source repair."
	case "source_failure":
		recommendedAction = "Refresh failed source rows before using affected items for absence claims or autonomous decisions."
	case "source_repair_needed":
		recommendedAction = "Run source-repair actions for rows already classified as repair needed."
	case "unknown_source_coverage", "source_unavailable":
		recommendedAction = "Resolve unknown or unavailable source coverage before using affected items for absence claims or autonomous decisions."
	case "partial_source_coverage":
		recommendedAction = "Refresh partial source coverage before using affected items for absence claims or autonomous decisions."
	}
	return workProgramEvidenceNeed(gateKey+":"+limitKind, gateKey, evidenceKind, priority, "workstream", summary.WorkstreamKey, limitKind, 0, count, count, nil, nil, recommendedAction)
}

func workProgramSourceCoverageLimitGateKey(limitKind string) string {
	switch limitKind {
	case "anonymous_observation":
		return "source_authentication"
	case "generated_evidence":
		return "claim_provenance"
	default:
		return "source_coverage"
	}
}

func workProgramInsightQualityNeeds(kind *model.WorkInsightKindEvaluation) []*model.WorkProgramAutomationEvidenceNeed {
	needs := []*model.WorkProgramAutomationEvidenceNeed{}
	if kind.PrecisionRate < minPrecisionRateForProductAction {
		currentRate := kind.PrecisionRate
		requiredRate := minPrecisionRateForProductAction
		requiredCount := workProgramRequiredCountForRate(requiredRate, kind.TruthLabeledCount)
		currentCount := kind.TruePositiveCount
		needs = append(needs, workProgramEvidenceNeed("measurement_quality:"+kind.InsightKind+":precision", "measurement_precision", "insight_quality", "high", "insight_kind", optionalString(kind.InsightKind), "precision", currentCount, requiredCount, maxInt(requiredCount-currentCount, 0), &currentRate, &requiredRate, "Improve or suppress "+kind.InsightKind+" precision before product-action automation."))
	}
	if kind.UsefulSignalRate < minUsefulSignalRateForProductAction {
		currentRate := kind.UsefulSignalRate
		requiredRate := minUsefulSignalRateForProductAction
		requiredCount := workProgramRequiredCountForRate(requiredRate, kind.TruthLabeledCount)
		currentCount := kind.TruePositiveCount + kind.PartialCount
		needs = append(needs, workProgramEvidenceNeed("measurement_quality:"+kind.InsightKind+":useful_signal", "measurement_precision", "insight_quality", "high", "insight_kind", optionalString(kind.InsightKind), "useful_signal", currentCount, requiredCount, maxInt(requiredCount-currentCount, 0), &currentRate, &requiredRate, "Improve or suppress "+kind.InsightKind+" useful-signal rate before product-action automation."))
	}
	if kind.ActionabilityRate < minActionabilityRateForProductAction {
		currentRate := kind.ActionabilityRate
		requiredRate := minActionabilityRateForProductAction
		requiredCount := workProgramRequiredCountForRate(requiredRate, kind.ActionabilityLabeledCount)
		currentCount := kind.ActionableCount + kind.NeedsOwnerCount
		needs = append(needs, workProgramEvidenceNeed("measurement_quality:"+kind.InsightKind+":actionability", "measurement_actionability", "insight_quality", "high", "insight_kind", optionalString(kind.InsightKind), "actionability", currentCount, requiredCount, maxInt(requiredCount-currentCount, 0), &currentRate, &requiredRate, "Improve or suppress "+kind.InsightKind+" actionability before product-action automation."))
	}
	return needs
}

func workProgramEvidenceNeed(key string, gateKey string, evidenceKind string, priority string, targetKind string, targetKey *string, metricKey string, currentCount int, requiredCount int, missingCount int, currentRate *float64, requiredRate *float64, recommendedAction string) *model.WorkProgramAutomationEvidenceNeed {
	return &model.WorkProgramAutomationEvidenceNeed{
		Key:               key,
		GateKey:           gateKey,
		EvidenceKind:      evidenceKind,
		Priority:          priority,
		TargetKind:        targetKind,
		TargetKey:         targetKey,
		MetricKey:         optionalString(metricKey),
		ExecutionState:    "unknown",
		CurrentCount:      currentCount,
		RequiredCount:     requiredCount,
		MissingCount:      missingCount,
		CurrentRate:       currentRate,
		RequiredRate:      requiredRate,
		RecommendedAction: recommendedAction,
		NextExecutionStep: recommendedAction,
	}
}

func workProgramAttachEvidenceExecution(summary *model.WorkProgramSummary, queue []*model.WorkProgramAutomationEvidenceNeed) {
	for _, need := range queue {
		count, state, next := workProgramEvidenceExecution(summary, need)
		need.BackingActionCount = count
		need.ExecutionState = state
		need.NextExecutionStep = next
	}
}

func workProgramEvidenceExecution(summary *model.WorkProgramSummary, need *model.WorkProgramAutomationEvidenceNeed) (int, string, string) {
	switch need.EvidenceKind {
	case "blocker_clearance":
		if summary.ClosureCandidateCount > 0 || summary.ProductActionCount > 0 {
			return maxInt(summary.ClosureCandidateCount, summary.ProductActionCount), "actions_open", "Use open blocker-clearance product actions to capture owner decisions and clearance evidence."
		}
		return 0, "missing_action", "Create blocker-clearance actions before expecting humans to close this evidence gap."
	case "forecast_backtest":
		if summary.ModelQualityCount > 0 {
			return summary.ModelQualityCount, "actions_open", "Use open model-quality actions to improve and re-check forecast backtest readiness."
		}
		return 0, "missing_action", "Create a model-quality action for forecast readiness before treating this as executable work."
	case "insight_labels", "insight_quality":
		if summary.ValidationLeadCount > 0 {
			return summary.ValidationLeadCount, "validation_actions_open", "Use validation-lead actions to collect labels or suppress low-quality insight kinds."
		}
		return 0, "missing_validation_action", "Create validation actions for this insight-kind evidence gap."
	case "source_authentication":
		backing := summary.ValidationLeadCount + summary.ProductActionCount
		if backing > 0 {
			return backing, "review_actions_open", "Use open review or product actions to re-observe anonymous rows with authenticated access, or keep them as lower-confidence leads."
		}
		return 0, "auth_upgrade_needed", "Create authenticated re-observation or validation actions before treating anonymous observations as product-action evidence."
	case "required_check_coverage":
		backing := maxInt(summary.CiFailingCount, summary.ValidationLeadCount)
		if backing > 0 {
			return backing, "configuration_actions_open", "Use CI-check validation actions to capture branch protection or required-check configuration evidence."
		}
		return 0, "configuration_evidence_needed", "Capture branch protection or required-check configuration before treating CI follow-ups as complete."
	case "generated_evidence":
		if summary.ModelQualityCount > 0 {
			return summary.ModelQualityCount, "qa_actions_open", "Use model-quality actions to review generated or derived claim evidence."
		}
		return 0, "claim_provenance_action_needed", "Create a model-quality or provenance-review action before treating generated claims as cleared evidence."
	case "source_coverage":
		if summary.SourceRepairCount > 0 {
			return summary.SourceRepairCount, "actions_open", "Use source-repair actions to refresh limited rows and re-run coverage-sensitive claims."
		}
		return 0, "missing_source_repair_action", "Create source-repair actions for limited coverage rows before treating the evidence gap as executable."
	case "owner_load_balancing":
		if len(summary.TopOwnerLoads) > 0 {
			return summary.OwnerLoadActionCount, "owner_load_rows_open", "Use latest owner-load rows to rebalance overloaded queues, assign unowned actions, or record why the owner concentration is accepted."
		}
		return 0, "missing_owner_load_snapshot", "Refresh owner-load snapshots before deciding whether the action plan is executable."
	default:
		return 0, "unknown", need.RecommendedAction
	}
}

func workProgramEvidencePriorityRank(priority string) int {
	switch priority {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func workProgramRequiredCountForRate(requiredRate float64, denominator int) int {
	if denominator <= 0 {
		return 0
	}
	return int(math.Ceil(requiredRate * float64(denominator)))
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func workProgramBriefHasEvidence(summary *model.WorkProgramSummary) bool {
	for _, blocker := range summary.TopBlockers {
		if blocker.EvidenceRef != nil && *blocker.EvidenceRef != "" {
			return true
		}
	}
	for _, impact := range summary.TopBlockerImpacts {
		if impact.EvidenceRef != nil && *impact.EvidenceRef != "" {
			return true
		}
	}
	for _, dependency := range summary.TopDependencies {
		if dependency.EvidenceRef != nil && *dependency.EvidenceRef != "" {
			return true
		}
	}
	for _, forecast := range summary.TopForecasts {
		if forecast.EvidenceRef != nil && *forecast.EvidenceRef != "" {
			return true
		}
	}
	for _, item := range summary.TopItems {
		if item.EvidenceRef != nil && *item.EvidenceRef != "" {
			return true
		}
	}
	return false
}

func workProgramHasAnyBlockingGate(gates []*model.WorkProgramBriefQualityGate) bool {
	for _, gate := range gates {
		if gate != nil && gate.Blocking {
			return true
		}
	}
	return false
}

func workProgramHasBlockingGate(gates []*model.WorkProgramBriefQualityGate, keys ...string) bool {
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[key] = true
	}
	for _, gate := range gates {
		if gate != nil && gate.Blocking && wanted[gate.Key] {
			return true
		}
	}
	return false
}

func workProgramBlockingGateKeys(gates []*model.WorkProgramBriefQualityGate) []string {
	keys := []string{}
	for _, gate := range gates {
		if gate != nil && gate.Blocking {
			keys = append(keys, gate.Key)
		}
	}
	return keys
}

func appendUniqueString(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func workProgramBriefCaveats(summary *model.WorkProgramSummary, evaluation *model.WorkInsightEvaluation) []*model.WorkProgramBriefCaveat {
	caveats := []*model.WorkProgramBriefCaveat{}
	if summary.ForecastReadiness == nil || !summary.ForecastReadiness.EtaForecastReady {
		detail := "Forecast output is useful for prioritization, but not ready as an ETA promise."
		evidence := ""
		if summary.ForecastReadiness != nil {
			if summary.ForecastReadiness.ReadinessReason != nil && *summary.ForecastReadiness.ReadinessReason != "" {
				detail = *summary.ForecastReadiness.ReadinessReason
			}
			if summary.ForecastReadiness.EvidenceRef != nil {
				evidence = *summary.ForecastReadiness.EvidenceRef
			}
		}
		caveats = append(caveats, workProgramBriefCaveat("forecast_gated", "warning", "Forecast gated", detail, "Do not present forecast dates as commitments.", evidence))
	}
	if evaluation == nil || !evaluation.ReadyToMeasurePrecision || !evaluation.ReadyToMeasureActionability {
		detail := "Generated insight quality is not fully measurement-ready."
		if evaluation != nil {
			detail = "Generated insight quality is gated by " + workProgramCountPhrase(evaluation.GatedInsightKindCount, "gated insight kind") + " and " + workProgramCountPhrase(evaluation.OpenReviewRequestCount, "open review request") + "."
		}
		caveats = append(caveats, workProgramBriefCaveat("measurement_gated", "warning", "Measurement gated", detail, "Add gold truth/actionability labels before claiming autonomous TPM replacement.", ""))
	}
	if summary.SourceCoverageLimitedCount > 0 {
		caveats = append(caveats, workProgramBriefCaveat("coverage_limited", "warning", "Source coverage limited", workProgramCountPhrase(summary.SourceCoverageLimitedCount, "program item")+" "+workProgramHasHave(summary.SourceCoverageLimitedCount)+" partial or limited source coverage.", "Use these rows as review leads, not absence claims.", ""))
	}
	if authLimitedCount := workProgramMapCount(workProgramAuthLimitedObservationCounts(summary)); authLimitedCount > 0 {
		caveats = append(caveats, workProgramBriefCaveat("source_authentication_limited", "warning", "Source authentication limited", workProgramCountPhrase(authLimitedCount, "program item")+" "+workProgramHasHave(authLimitedCount)+" only anonymous/public source observation.", "Re-observe these rows with authenticated access before absence, completion, or autonomous decision claims.", ""))
	}
	if generatedClaimCount := workProgramMapCount(workProgramGeneratedClaimLimitCounts(summary)); generatedClaimCount > 0 {
		caveats = append(caveats, workProgramBriefCaveat("generated_claim_provenance", "warning", "Generated claim provenance", workProgramCountPhrase(generatedClaimCount, "program item")+" depend on generated or derived claim evidence.", "Attach independent source, provenance, or measurement evidence before treating generated claims as cleared.", ""))
	}
	if summary.OverloadedOwnerCount > 0 || summary.UnassignedActionCount > 0 {
		parts := []string{}
		severity := "warning"
		if summary.OverloadedOwnerCount > 0 {
			parts = append(parts, workProgramCountPhrase(summary.OverloadedOwnerCount, "overloaded owner"))
			severity = "danger"
		}
		if summary.UnassignedActionCount > 0 {
			parts = append(parts, workProgramCountPhrase(summary.UnassignedActionCount, "unassigned action"))
		}
		caveats = append(caveats, workProgramBriefCaveat("owner_load", severity, "Owner load constrained", workProgramJoinFocusParts(parts)+" remain in the latest owner-load snapshot.", "Rebalance or explicitly accept owner concentration before using the brief as an autonomous execution plan.", ""))
	}
	if summary.ActiveBlockerCount > 0 || summary.ActiveBlockerImpactCount > 0 {
		caveats = append(caveats, workProgramBriefCaveat("active_blockers", "danger", "Active blockers", workProgramCountPhrase(summary.ActiveBlockerCount, "active blocker")+" and "+workProgramCountPhrase(summary.ActiveBlockerImpactCount, "active blocker impact")+" remain open.", "Clear blockers before treating the workstream as on track.", ""))
	}
	if summary.NeedsActionDependencyCount > 0 {
		caveats = append(caveats, workProgramBriefCaveat("dependency_pressure", "warning", "Dependency pressure", workProgramCountPhrase(summary.NeedsActionDependencyCount, "dependency needing action")+" "+workProgramIsAre(summary.NeedsActionDependencyCount)+" in scope.", "Use top dependency drivers to assign concrete owners.", ""))
	}
	return caveats
}

func workProgramHasHave(count int) string {
	if count == 1 {
		return "has"
	}
	return "have"
}

func workProgramIsAre(count int) string {
	if count == 1 {
		return "is"
	}
	return "are"
}

func workProgramBriefCaveat(key string, severity string, title string, detail string, recommendedAction string, evidenceRef string) *model.WorkProgramBriefCaveat {
	return &model.WorkProgramBriefCaveat{
		Key:               key,
		Severity:          severity,
		Title:             title,
		Detail:            detail,
		RecommendedAction: optionalString(recommendedAction),
		EvidenceRef:       optionalString(evidenceRef),
	}
}

func edgeKindLabelString(value string) string {
	switch value {
	case "blocked_by":
		return "Blocked by dependency"
	case "needs_action":
		return "Dependency needs action"
	default:
		return value
	}
}

func workProgramDependencyDriverAction(edge *model.WorkDependencyEdge) *string {
	switch edge.EdgeKind {
	case "blocked_by":
		return optionalString("Clear or validate the linked blocker.")
	case "needs_action":
		return optionalString("Drive the linked action to completion.")
	default:
		return nil
	}
}
