package graphql

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workresponsibility"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) workProgramAttentionPacket(ctx context.Context, workstreamKey string, limit *int, evidenceLimit *int, sourceInstance *string) (*model.WorkProgramAttentionPacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramAttentionPacket requires an Ent-backed ontology store")
	}
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil, fmt.Errorf("workProgramAttentionPacket requires workstreamKey")
	}
	rowLimit := boundedLimit(limit, 20, 100)
	evidenceRowLimit := boundedLimit(evidenceLimit, 50, 200)
	sourceFilter, err := r.workProgramTPMReadinessSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	tpmReadiness, err := r.workProgramTpmReadinessPacket(ctx, workstreamKey, &rowLimit, &evidenceRowLimit, nil, sourceFilter)
	if err != nil {
		return nil, err
	}
	openActionState := "open"
	execution, err := r.workProgramExecutionPacket(ctx, workstreamKey, &openActionState, &rowLimit, &evidenceRowLimit, nil, sourceFilter)
	if err != nil {
		return nil, err
	}
	openBlockerState := "open"
	blockers, err := r.workProgramBlockerPacket(ctx, workstreamKey, &openBlockerState, &rowLimit, &evidenceRowLimit, sourceFilter)
	if err != nil {
		return nil, err
	}
	forecasts, err := r.workProgramForecastPacket(ctx, workstreamKey, &rowLimit, &evidenceRowLimit, sourceFilter)
	if err != nil {
		return nil, err
	}
	responsibilities, err := r.workProgramAttentionResponsibilityModels(ctx, sourceFilter, workstreamKey, rowLimit)
	if err != nil {
		return nil, err
	}

	priorities := workProgramAttentionPriorities(execution, blockers, forecasts, responsibilities)
	totalPriorityCount := len(priorities)
	urgentCount, humanRequiredCount := workProgramAttentionCounts(priorities)
	recommendedFocus := workProgramAttentionFocus(priorities, execution, blockers, forecasts, tpmReadiness)
	attentionState := workProgramAttentionState(priorities, tpmReadiness, execution, blockers, forecasts)
	priorities = limitWorkProgramAttentionPriorities(priorities, rowLimit)

	return &model.WorkProgramAttentionPacket{
		SourceInstance:     sourceFilter,
		GeneratedAt:        tpmReadiness.GeneratedAt,
		WorkstreamKey:      workstreamKey,
		AttentionState:     attentionState,
		PriorityCount:      totalPriorityCount,
		UrgentCount:        urgentCount,
		HumanRequiredCount: humanRequiredCount,
		EvidenceNeedCount:  execution.EvidenceNeedCount,
		AutomationSummary:  workProgramAttentionSummary(workstreamKey, attentionState, totalPriorityCount, urgentCount, humanRequiredCount, execution.EvidenceNeedCount, recommendedFocus),
		RecommendedFocus:   recommendedFocus,
		TpmReadiness:       tpmReadiness,
		Priorities:         priorities,
		EvidenceNeeds:      execution.EvidenceNeeds,
	}, nil
}

func (r *queryResolver) workProgramAttentionResponsibilityModels(ctx context.Context, sourceFilter *string, workstreamKey string, limit int) ([]*model.WorkResponsibility, error) {
	rows, _, err := r.workProgramAttentionResponsibilityModelsAndCount(ctx, sourceFilter, workstreamKey, limit)
	return rows, err
}

func (r *queryResolver) workProgramAttentionResponsibilityModelsAndCount(ctx context.Context, sourceFilter *string, workstreamKey string, limit int) ([]*model.WorkResponsibility, int, error) {
	if r.EntClient == nil || limit <= 0 {
		return []*model.WorkResponsibility{}, 0, nil
	}
	count, err := r.workProgramAttentionResponsibilityCount(ctx, sourceFilter, workstreamKey)
	if err != nil {
		return nil, 0, err
	}
	query, err := r.workProgramAttentionResponsibilityBaseQuery(sourceFilter, workstreamKey)
	if err != nil {
		return []*model.WorkResponsibility{}, count, nil
	}
	if query == nil {
		return []*model.WorkResponsibility{}, count, nil
	}
	query = query.
		WithPerson().
		WithPullRequest().
		WithTicket().
		WithWorkAction(workActionDetails(sourceFilter)).
		WithWorkBlocker(func(q *genent.WorkBlockerQuery) {
			q.WithLatestEvidence()
			q.WithWorkAction(workActionDetails(sourceFilter))
		}).
		WithWorkProgramItem(func(q *genent.WorkProgramItemQuery) {
			q.WithLatestEvidence()
			q.WithWorkAction(workActionDetails(sourceFilter))
		}).
		WithWorkProgramEvidenceNeed(func(q *genent.WorkProgramEvidenceNeedQuery) {
			q.WithLatestEvidence()
			q.WithWorkAction(workActionDetails(sourceFilter))
		}).
		WithLatestEvidence().
		Order(
			workresponsibility.ByRankScore(entsql.OrderDesc()),
			workresponsibility.ByLastActivityAt(entsql.OrderDesc()),
			workresponsibility.ByKey(),
		).
		Limit(limit)
	rows, err := query.All(ctx)
	if err != nil {
		return nil, 0, err
	}
	return workResponsibilityModels(rows), count, nil
}

func (r *queryResolver) workProgramAttentionResponsibilityCount(ctx context.Context, sourceFilter *string, workstreamKey string) (int, error) {
	if r.EntClient == nil {
		return 0, nil
	}
	query, err := r.workProgramAttentionResponsibilityBaseQuery(sourceFilter, workstreamKey)
	if err != nil || query == nil {
		return 0, err
	}
	return query.Count(ctx)
}

func (r *queryResolver) workProgramAttentionResponsibilityBaseQuery(sourceFilter *string, workstreamKey string) (*genent.WorkResponsibilityQuery, error) {
	scope := workProgramOwnerResponsibilityScopePredicate(workstreamKey)
	if scope == nil {
		return nil, nil
	}
	query := r.EntClient.WorkResponsibility.Query().
		Where(
			workresponsibility.SourceSystemEQ("cubicle_analytics"),
			workresponsibility.ExternalKindEQ("tpm_work_responsibility"),
			workresponsibility.Or(
				workresponsibility.ResponsibilityStateEQ(workresponsibility.ResponsibilityStateCandidate),
				workresponsibility.PartyKindEQ(workresponsibility.PartyKindUnassigned),
			),
			scope,
		)
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		query = query.Where(workresponsibility.SourceInstanceEQ(strings.TrimSpace(*sourceFilter)))
	}
	return query, nil
}

func workProgramAttentionPriorities(execution *model.WorkProgramExecutionPacket, blockers *model.WorkProgramBlockerPacket, forecasts *model.WorkProgramForecastPacket, responsibilities []*model.WorkResponsibility) []*model.WorkProgramAttentionPriority {
	priorities := []*model.WorkProgramAttentionPriority{}
	seen := map[string]bool{}
	appendPriority := func(priority *model.WorkProgramAttentionPriority) {
		if priority == nil || priority.Key == "" || seen[priority.Key] {
			return
		}
		seen[priority.Key] = true
		priorities = append(priorities, priority)
	}
	if execution != nil {
		for _, action := range execution.Actions {
			appendPriority(workProgramAttentionPriorityForAction(action))
		}
		for _, need := range execution.EvidenceNeeds {
			appendPriority(workProgramAttentionPriorityForEvidenceNeed(need))
		}
	}
	if blockers != nil {
		for _, blocker := range blockers.Blockers {
			appendPriority(workProgramAttentionPriorityForBlocker(blocker))
		}
		for _, impact := range blockers.Impacts {
			appendPriority(workProgramAttentionPriorityForImpact(impact))
		}
		for _, need := range blockers.EvidenceNeeds {
			appendPriority(workProgramAttentionPriorityForEvidenceNeed(need))
		}
	}
	if forecasts != nil {
		for _, forecast := range forecasts.Forecasts {
			appendPriority(workProgramAttentionPriorityForForecast(forecast))
		}
		for _, need := range forecasts.EvidenceNeeds {
			appendPriority(workProgramAttentionPriorityForEvidenceNeed(need))
		}
	}
	for _, responsibility := range responsibilities {
		appendPriority(workProgramAttentionPriorityForResponsibility(responsibility))
	}
	workProgramSortAttentionPriorities(priorities)
	return priorities
}

func workProgramAttentionPriorityForAction(action *model.WorkAction) *model.WorkProgramAttentionPriority {
	if action == nil {
		return nil
	}
	score := action.RankScore + workProgramAttentionActionBoost(action)
	title := firstNonempty(pointerString(action.DecisionNeeded), pointerString(action.SubjectTitle), action.SubjectKey, action.Key)
	reason := firstNonempty(action.ClaimGateReason, pointerString(action.DecisionReason), "action requires review")
	humanRequired := !action.ProductActionAllowed || action.DecisionState != "product_action"
	return &model.WorkProgramAttentionPriority{
		Key:                  "attention:action:" + action.Key,
		PriorityKind:         "action",
		PriorityState:        action.DecisionState,
		SubjectKind:          optionalString(action.SubjectKind),
		SubjectKey:           optionalString(action.SubjectKey),
		Title:                title,
		Reason:               reason,
		RecommendedAction:    action.RecommendedAction,
		Urgency:              workProgramAttentionUrgency(score),
		RankScore:            score,
		ClaimUse:             optionalString(action.ClaimUse),
		ClaimGateReason:      optionalString(action.ClaimGateReason),
		ProductActionAllowed: action.ProductActionAllowed,
		HumanRequired:        humanRequired,
		SourceURL:            action.SourceURL,
		EvidenceRef:          action.EvidenceRef,
		Evidence:             action.Evidence,
		Action:               action,
	}
}

func workProgramAttentionPriorityForBlocker(blocker *model.WorkBlocker) *model.WorkProgramAttentionPriority {
	if blocker == nil {
		return nil
	}
	score := blocker.RankScore + workProgramAttentionBlockerBoost(blocker)
	reason := firstNonempty(blocker.ClaimGateReason, pointerString(blocker.Summary), "blocker requires review")
	return &model.WorkProgramAttentionPriority{
		Key:                  "attention:blocker:" + blocker.Key,
		PriorityKind:         "blocker",
		PriorityState:        blocker.BlockerState,
		SubjectKind:          optionalString(blocker.SubjectKind),
		SubjectKey:           optionalString(blocker.SubjectKey),
		Title:                blocker.Title,
		Reason:               reason,
		RecommendedAction:    blocker.RecommendedAction,
		Urgency:              workProgramAttentionUrgency(score),
		RankScore:            score,
		ClaimUse:             optionalString(blocker.ClaimUse),
		ClaimGateReason:      optionalString(blocker.ClaimGateReason),
		ProductActionAllowed: blocker.ProductActionAllowed,
		HumanRequired:        !blocker.BlockerClaimAllowed,
		SourceURL:            blocker.SourceURL,
		EvidenceRef:          blocker.EvidenceRef,
		Evidence:             blocker.Evidence,
		Blocker:              blocker,
	}
}

func workProgramAttentionPriorityForImpact(impact *model.WorkBlockerImpact) *model.WorkProgramAttentionPriority {
	if impact == nil {
		return nil
	}
	score := math.Max(impact.ImpactScore, impact.RankScore) + workProgramAttentionImpactBoost(impact)
	reason := firstNonempty(impact.ClaimGateReason, pointerString(impact.Summary), "blocker impact requires review")
	return &model.WorkProgramAttentionPriority{
		Key:                  "attention:blocker_impact:" + impact.Key,
		PriorityKind:         "blocker_impact",
		PriorityState:        impact.ImpactState,
		SubjectKind:          optionalString(impact.SubjectKind),
		SubjectKey:           optionalString(impact.SubjectKey),
		Title:                impact.Title,
		Reason:               reason,
		RecommendedAction:    impact.RecommendedAction,
		Urgency:              workProgramAttentionUrgency(score),
		RankScore:            score,
		ClaimUse:             optionalString(impact.ClaimUse),
		ClaimGateReason:      optionalString(impact.ClaimGateReason),
		ProductActionAllowed: impact.ImpactClaimAllowed,
		HumanRequired:        !impact.ImpactClaimAllowed,
		SourceURL:            impact.SourceURL,
		EvidenceRef:          impact.EvidenceRef,
		Evidence:             impact.Evidence,
		BlockerImpact:        impact,
	}
}

func workProgramAttentionPriorityForForecast(forecast *model.WorkItemForecast) *model.WorkProgramAttentionPriority {
	if forecast == nil {
		return nil
	}
	score := forecast.RiskScore + workProgramAttentionForecastBoost(forecast)
	title := firstNonempty(pointerString(forecast.SubjectTitle), forecast.SubjectKey, forecast.Key)
	reason := firstNonempty(forecast.ForecastClaimGateReason, pointerString(forecast.ReadinessReason), forecast.Interpretation)
	return &model.WorkProgramAttentionPriority{
		Key:                  "attention:forecast:" + forecast.Key,
		PriorityKind:         "forecast",
		PriorityState:        forecast.RiskBand,
		SubjectKind:          optionalString(forecast.SubjectKind),
		SubjectKey:           optionalString(forecast.SubjectKey),
		Title:                title,
		Reason:               reason,
		RecommendedAction:    optionalString(forecast.RecommendedAction),
		Urgency:              workProgramAttentionUrgency(score),
		RankScore:            score,
		ClaimUse:             optionalString(forecast.ForecastClaimUse),
		ClaimGateReason:      optionalString(forecast.ForecastClaimGateReason),
		ProductActionAllowed: forecast.EtaClaimAllowed,
		HumanRequired:        !forecast.EtaClaimAllowed || forecast.ActionabilityState != "watch",
		SourceURL:            forecast.SubjectURL,
		EvidenceRef:          forecast.EvidenceRef,
		Evidence:             forecast.Evidence,
		Forecast:             forecast,
	}
}

func workProgramAttentionPriorityForEvidenceNeed(need *model.WorkProgramAutomationEvidenceNeed) *model.WorkProgramAttentionPriority {
	if need == nil {
		return nil
	}
	score := workProgramAttentionEvidenceNeedScore(need)
	title := firstNonempty(need.EvidenceKind, need.GateKey, need.Key)
	reason := firstNonempty(need.NextExecutionStep, need.RecommendedAction, "evidence is required before automation")
	return &model.WorkProgramAttentionPriority{
		Key:                  "attention:evidence_need:" + need.Key,
		PriorityKind:         "evidence_need",
		PriorityState:        need.ExecutionState,
		SubjectKind:          optionalString(need.TargetKind),
		SubjectKey:           need.TargetKey,
		Title:                title,
		Reason:               reason,
		RecommendedAction:    optionalString(need.RecommendedAction),
		Urgency:              workProgramAttentionUrgency(score),
		RankScore:            score,
		ClaimUse:             optionalString("evidence_collection"),
		ClaimGateReason:      optionalString("evidence_required:" + need.GateKey),
		ProductActionAllowed: false,
		HumanRequired:        true,
		SourceURL:            need.SourceURL,
		Evidence:             need.Evidence,
		EvidenceNeed:         need,
	}
}

func workProgramAttentionPriorityForResponsibility(responsibility *model.WorkResponsibility) *model.WorkProgramAttentionPriority {
	if responsibility == nil {
		return nil
	}
	score := responsibility.RankScore + workProgramAttentionResponsibilityBoost(responsibility)
	title := firstNonempty(pointerString(responsibility.SubjectTitle), responsibility.SubjectKey, responsibility.Key)
	reason := firstNonempty(pointerString(responsibility.ResponsibilityStateReason), "responsibility requires validation")
	recommendedAction := workProgramAttentionResponsibilityRecommendedAction(responsibility)
	return &model.WorkProgramAttentionPriority{
		Key:                  "attention:responsibility:" + responsibility.Key,
		PriorityKind:         "responsibility",
		PriorityState:        responsibility.ResponsibilityState,
		SubjectKind:          optionalString(responsibility.SubjectKind),
		SubjectKey:           optionalString(responsibility.SubjectKey),
		Title:                title,
		Reason:               reason,
		RecommendedAction:    optionalString(recommendedAction),
		Urgency:              workProgramAttentionUrgency(score),
		RankScore:            score,
		ClaimUse:             optionalString("accountability_validation"),
		ClaimGateReason:      optionalString("responsibility:" + responsibility.BasisKind),
		ProductActionAllowed: false,
		HumanRequired:        true,
		SourceURL:            responsibility.SourceURL,
		EvidenceRef:          responsibility.EvidenceRef,
		Evidence:             responsibility.Evidence,
		Responsibility:       responsibility,
	}
}

func workProgramAttentionActionBoost(action *model.WorkAction) float64 {
	boost := 0.0
	switch action.DecisionState {
	case "product_action":
		boost += 20
	case "source_repair", "closeout_review", "model_or_rule_qa":
		boost += 15
	case "validation_lead":
		boost += 10
	}
	switch action.DueBucket {
	case "now":
		boost += 15
	case "this_week":
		boost += 5
	}
	if action.ProductActionAllowed {
		boost += 5
	}
	return boost
}

func workProgramAttentionBlockerBoost(blocker *model.WorkBlocker) float64 {
	boost := 0.0
	switch blocker.BlockerState {
	case "active":
		boost += 25
	case "validating":
		boost += 10
	}
	switch blocker.Severity {
	case "critical":
		boost += 25
	case "high":
		boost += 15
	}
	if blocker.BlockerClaimAllowed {
		boost += 10
	}
	return boost
}

func workProgramAttentionImpactBoost(impact *model.WorkBlockerImpact) float64 {
	boost := 0.0
	switch impact.ImpactState {
	case "active":
		boost += 20
	case "validating":
		boost += 10
	}
	switch impact.Severity {
	case "critical":
		boost += 25
	case "high":
		boost += 15
	}
	return boost
}

func workProgramAttentionForecastBoost(forecast *model.WorkItemForecast) float64 {
	switch forecast.RiskBand {
	case "critical":
		return 20
	case "high":
		return 10
	default:
		return 0
	}
}

func workProgramAttentionEvidenceNeedScore(need *model.WorkProgramAutomationEvidenceNeed) float64 {
	score := 50.0
	switch need.Priority {
	case "critical":
		score = 115
	case "high":
		score = 95
	case "medium":
		score = 75
	case "low":
		score = 55
	}
	score += math.Min(float64(need.MissingCount), 10)
	if need.ExecutionState != "ready" && need.ExecutionState != "complete" {
		score += 5
	}
	return score
}

func workProgramAttentionResponsibilityBoost(responsibility *model.WorkResponsibility) float64 {
	if responsibility == nil {
		return 0
	}
	boost := 0.0
	if responsibility.ResponsibilityState == "candidate" {
		boost += 20
	}
	if responsibility.BasisKind == "generated_candidate" {
		boost += 10
	}
	if responsibility.PartyKind == "unassigned" {
		boost += 30
	}
	return boost
}

func workProgramAttentionResponsibilityRecommendedAction(responsibility *model.WorkResponsibility) string {
	if responsibility == nil {
		return ""
	}
	if responsibility.PartyKind == "unassigned" {
		return "Assign an accountable owner before treating this work as executable."
	}
	if responsibility.BasisKind == "generated_candidate" {
		return "Validate the generated owner routing before treating it as product accountability."
	}
	return "Validate the responsibility state before using it for TPM execution."
}

func workProgramAttentionUrgency(score float64) string {
	switch {
	case score >= 120:
		return "critical"
	case score >= 95:
		return "high"
	case score >= 75:
		return "medium"
	default:
		return "low"
	}
}

func workProgramSortAttentionPriorities(rows []*model.WorkProgramAttentionPriority) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].RankScore != rows[j].RankScore {
			return rows[i].RankScore > rows[j].RankScore
		}
		if workProgramAttentionKindRank(rows[i].PriorityKind) != workProgramAttentionKindRank(rows[j].PriorityKind) {
			return workProgramAttentionKindRank(rows[i].PriorityKind) < workProgramAttentionKindRank(rows[j].PriorityKind)
		}
		return rows[i].Key < rows[j].Key
	})
}

func workProgramAttentionKindRank(kind string) int {
	switch kind {
	case "blocker_impact":
		return 0
	case "blocker":
		return 1
	case "action":
		return 2
	case "evidence_need":
		return 3
	case "responsibility":
		return 4
	case "forecast":
		return 5
	default:
		return 10
	}
}

func limitWorkProgramAttentionPriorities(rows []*model.WorkProgramAttentionPriority, limit int) []*model.WorkProgramAttentionPriority {
	if limit <= 0 || len(rows) <= limit {
		return rows
	}
	return rows[:limit]
}

func workProgramAttentionCounts(rows []*model.WorkProgramAttentionPriority) (int, int) {
	urgentCount := 0
	humanRequiredCount := 0
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.Urgency == "critical" || row.Urgency == "high" {
			urgentCount++
		}
		if row.HumanRequired {
			humanRequiredCount++
		}
	}
	return urgentCount, humanRequiredCount
}

func workProgramAttentionFocus(priorities []*model.WorkProgramAttentionPriority, execution *model.WorkProgramExecutionPacket, blockers *model.WorkProgramBlockerPacket, forecasts *model.WorkProgramForecastPacket, readiness *model.WorkProgramTpmReadinessPacket) *string {
	for _, priority := range priorities {
		if priority != nil && priority.RecommendedAction != nil && strings.TrimSpace(*priority.RecommendedAction) != "" {
			return optionalTrimmedPointerValue(priority.RecommendedAction)
		}
	}
	if execution != nil && execution.RecommendedFocus != nil {
		return optionalTrimmedPointerValue(execution.RecommendedFocus)
	}
	if blockers != nil && blockers.RecommendedFocus != nil {
		return optionalTrimmedPointerValue(blockers.RecommendedFocus)
	}
	if forecasts != nil && forecasts.RecommendedFocus != nil {
		return optionalTrimmedPointerValue(forecasts.RecommendedFocus)
	}
	if readiness != nil && readiness.RecommendedFocus != nil {
		return optionalTrimmedPointerValue(readiness.RecommendedFocus)
	}
	return nil
}

func workProgramAttentionState(priorities []*model.WorkProgramAttentionPriority, readiness *model.WorkProgramTpmReadinessPacket, execution *model.WorkProgramExecutionPacket, blockers *model.WorkProgramBlockerPacket, forecasts *model.WorkProgramForecastPacket) string {
	if readiness != nil && readiness.ReplacementState == "blocked" && !readiness.AbsenceClaimsAllowed {
		return "blocked_source_coverage"
	}
	for _, priority := range priorities {
		if priority == nil {
			continue
		}
		if priority.Urgency == "critical" && priority.HumanRequired {
			return "urgent_human_review"
		}
	}
	for _, priority := range priorities {
		if priority != nil && priority.PriorityKind == "responsibility" && priority.HumanRequired {
			return "responsibility_review_required"
		}
	}
	if execution != nil && execution.ExecutionState != "" && execution.ExecutionState != "no_open_work" {
		return execution.ExecutionState
	}
	if blockers != nil && blockers.HumanRequired {
		return "blocker_review_required"
	}
	if forecasts != nil && forecasts.HumanRequired {
		return "forecast_review_required"
	}
	if len(priorities) == 0 {
		return "clear"
	}
	return "attention_required"
}

func workProgramAttentionSummary(workstreamKey string, attentionState string, priorityCount int, urgentCount int, humanRequiredCount int, evidenceNeedCount int, recommendedFocus *string) string {
	summary := fmt.Sprintf("%s attention queue is %s with %d priority item(s), %d urgent item(s), %d human-required item(s), and %d evidence need(s).", workstreamKey, attentionState, priorityCount, urgentCount, humanRequiredCount, evidenceNeedCount)
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
