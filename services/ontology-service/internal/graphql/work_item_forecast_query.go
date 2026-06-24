package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workitemforecast"
	"cubicle/services/ontology-service/internal/graphql/model"
	"fmt"
	"strings"
	"time"
)

func workItemForecastModel(row *genent.WorkItemForecast) *model.WorkItemForecast {
	return workItemForecastModelWithReadiness(row, nil)
}

func workItemForecastModelWithReadiness(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) *model.WorkItemForecast {
	etaClaimAllowed := workItemForecastETAClaimAllowed(row, readiness)
	var action *model.WorkAction
	if row.Edges.WorkAction != nil {
		action = workActionModelWithClaimPolicy(row.Edges.WorkAction, workItemForecastActionClaimPolicy(row, readiness))
	}
	return &model.WorkItemForecast{
		Key:                     row.Key,
		SourceInstance:          row.SourceInstance,
		ForecastKind:            row.ForecastKind.String(),
		SubjectKind:             row.SubjectKind.String(),
		SubjectKey:              row.SubjectKey,
		SubjectTitle:            optionalString(workItemForecastSubjectTitle(row)),
		SubjectURL:              optionalString(workItemForecastSubjectURL(row)),
		SubjectState:            optionalString(row.SubjectState),
		ForecastMethod:          optionalString(row.ForecastMethod),
		ModelName:               optionalString(row.ModelName),
		AgeDays:                 optionalFloat(row.AgeDays),
		PredictedTotalCycleDays: optionalFloat(row.PredictedTotalCycleDays),
		PredictedRemainingDays:  optionalFloat(row.PredictedRemainingDays),
		OverdueDays:             optionalFloat(row.OverdueDays),
		RiskScore:               row.RiskScore,
		RiskBand:                row.RiskBand.String(),
		ReadinessState:          row.ReadinessState.String(),
		EtaForecastReady:        etaClaimAllowed,
		EtaClaimAllowed:         etaClaimAllowed,
		ForecastClaimUse:        workItemForecastClaimUse(row, readiness),
		ForecastClaimGateReason: workItemForecastClaimGateReason(row, readiness),
		ReadinessReason:         optionalString(row.ReadinessReason),
		ActionabilityState:      workItemForecastActionabilityState(row, readiness),
		RecommendedAction:       workItemForecastRecommendedAction(row, readiness),
		Interpretation:          workItemForecastInterpretation(row, readiness),
		EvidenceRef:             optionalString(firstNonempty(evidenceRef(row.Edges.LatestEvidence), row.SourceURL)),
		Evidence:                workEvidenceSummary(row.Edges.LatestEvidence),
		Action:                  action,
		Badges:                  workItemForecastBadges(row, readiness),
	}
}

func workItemForecastETAClaimAllowed(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) bool {
	return row.ReadyForEta &&
		readiness != nil &&
		readiness.EtaForecastReady &&
		workItemForecastRunMatchesReadiness(row, readiness)
}

func workItemForecastActionClaimPolicy(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) workActionClaimPolicy {
	if readiness == nil {
		return workActionClaimPolicy{}
	}
	return workActionClaimPolicy{
		etaForecastReady:         row.ReadyForEta && readiness.EtaForecastReady && workItemForecastRunMatchesReadiness(row, readiness),
		etaReadinessGateReason:   workItemForecastClaimGateReason(row, readiness),
		etaReadinessContextKnown: true,
	}
}

func workItemForecastClaimUse(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) string {
	if workItemForecastETAClaimAllowed(row, readiness) {
		return "eta_candidate"
	}
	switch workItemForecastActionabilityState(row, readiness) {
	case "owner_status_needed", "risk_triage":
		return "risk_triage_only"
	default:
		return "watch_only"
	}
}

func workItemForecastClaimGateReason(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) string {
	if workItemForecastETAClaimAllowed(row, readiness) {
		return "eta_forecast_ready"
	}
	if row.ReadyForEta && readiness != nil && readiness.EtaForecastReady && !workItemForecastRunMatchesReadiness(row, readiness) {
		return "forecast_run_mismatch"
	}
	if row.ReadyForEta && (readiness == nil || !readiness.EtaForecastReady) {
		if reason := forecastReadinessGateReason(readiness); reason != "" {
			return "global_eta_forecast_gated:" + reason
		}
		return "global_eta_forecast_not_verified"
	}
	switch row.ReadinessState {
	case workitemforecast.ReadinessStateGated:
		return "eta_forecast_gated"
	case workitemforecast.ReadinessStateInsufficientSample:
		return "eta_forecast_insufficient_sample"
	case workitemforecast.ReadinessStateReady:
		return "ready_state_without_eta_claim"
	default:
		return "eta_forecast_not_ready"
	}
}

func workItemForecastRunMatchesReadiness(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) bool {
	if row == nil || readiness == nil || readiness.EvaluatedAt == nil || strings.TrimSpace(*readiness.EvaluatedAt) == "" || row.ForecastedAt.IsZero() {
		return false
	}
	evaluatedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(*readiness.EvaluatedAt))
	if err != nil {
		return false
	}
	return row.ForecastedAt.UTC().Equal(evaluatedAt.UTC())
}

func forecastReadinessForForecastRows(readiness *model.WorkForecastReadiness, rows []*genent.WorkItemForecast) *model.WorkForecastReadiness {
	if readiness == nil || !readiness.EtaForecastReady || len(rows) == 0 {
		return readiness
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if !workItemForecastRunMatchesReadiness(row, readiness) {
			return forecastReadinessWithETAGate(readiness, "forecast_run_mismatch")
		}
	}
	return readiness
}

func forecastReadinessWithETAGate(readiness *model.WorkForecastReadiness, reason string) *model.WorkForecastReadiness {
	if readiness == nil {
		return nil
	}
	out := *readiness
	out.EtaForecastReady = false
	out.ReadinessState = "gated"
	out.EtaReadinessBlockingReason = optionalString(reason)
	out.ReadinessReason = optionalString(reason)
	out.Detail = optionalString(reason)
	out.Badges = workForecastReadinessBadges(&out)
	return &out
}

func workItemForecastSubjectTitle(row *genent.WorkItemForecast) string {
	if row.Edges.PullRequest != nil && row.Edges.PullRequest.Title != "" {
		return row.Edges.PullRequest.Title
	}
	if row.Edges.Ticket != nil && row.Edges.Ticket.Title != "" {
		return row.Edges.Ticket.Title
	}
	return ""
}

func workItemForecastSubjectURL(row *genent.WorkItemForecast) string {
	if row.Edges.PullRequest != nil && row.Edges.PullRequest.SourceURL != "" {
		return row.Edges.PullRequest.SourceURL
	}
	if row.Edges.Ticket != nil && row.Edges.Ticket.SourceURL != "" {
		return row.Edges.Ticket.SourceURL
	}
	return row.SourceURL
}

func workItemForecastBadges(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		workItemForecastRiskBadge(row),
	}
	switch workItemForecastActionabilityState(row, readiness) {
	case "eta_commitment_ready":
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:action_eta_ready", Label: "ETA action ready", Tone: "success"})
	case "owner_status_needed":
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:action_owner_status", Label: "Owner status needed", Tone: "danger"})
	case "risk_triage":
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:action_risk_triage", Label: "Risk triage", Tone: "warning"})
	case "watch":
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:action_watch", Label: "Watch", Tone: "neutral"})
	}
	if workItemForecastETAClaimAllowed(row, readiness) {
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:eta_ready", Label: "ETA ready", Tone: "success"})
	} else if row.ReadyForEta && (readiness == nil || !readiness.EtaForecastReady) {
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:eta_global_gated", Label: "ETA globally gated", Tone: "warning", Detail: optionalString(workItemForecastClaimGateReason(row, readiness))})
	} else if row.ReadinessState == workitemforecast.ReadinessStateGated {
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:eta_gated", Label: "ETA gated", Tone: "warning", Detail: optionalString(row.ReadinessReason)})
	} else if row.ReadinessState == workitemforecast.ReadinessStateInsufficientSample {
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:insufficient_sample", Label: "Forecast sample low", Tone: "warning", Detail: optionalString(row.ReadinessReason)})
	}
	if row.OverdueDays != nil && *row.OverdueDays > 0 {
		badges = append(badges, &model.WorkActionBadge{Key: "forecast:overdue", Label: "Over forecast", Tone: "danger", Detail: optionalString(fmt.Sprintf("%.1fd past forecast", *row.OverdueDays))})
	}
	return badges
}

func workItemForecastActionabilityState(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) string {
	if workItemForecastETAClaimAllowed(row, readiness) {
		return "eta_commitment_ready"
	}
	switch row.RiskBand {
	case workitemforecast.RiskBandCritical:
		return "owner_status_needed"
	case workitemforecast.RiskBandHigh:
		return "risk_triage"
	}
	if row.OverdueDays != nil && *row.OverdueDays > 0 {
		return "risk_triage"
	}
	return "watch"
}

func workItemForecastRecommendedAction(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) string {
	switch workItemForecastActionabilityState(row, readiness) {
	case "eta_commitment_ready":
		return "Use this forecast as an ETA candidate after confirming the owner plan and latest source state."
	case "owner_status_needed":
		return "Treat this as a TPM risk lead: ask the owner for merge, close, or parking status, but do not present it as an ETA commitment."
	case "risk_triage":
		return "Review this forecast with the owner or reviewer as risk triage until ETA readiness gates clear."
	default:
		return "Keep this item on forecast watch; no ETA commitment or owner escalation is supported by the current forecast evidence."
	}
}

func workItemForecastInterpretation(row *genent.WorkItemForecast, readiness *model.WorkForecastReadiness) string {
	parts := []string{}
	if row.RiskBand != workitemforecast.RiskBandUnknown {
		parts = append(parts, row.RiskBand.String()+" forecast risk")
	}
	if row.AgeDays != nil {
		parts = append(parts, fmt.Sprintf("%.1fd old", *row.AgeDays))
	}
	if row.PredictedTotalCycleDays != nil {
		parts = append(parts, fmt.Sprintf("%.1fd baseline cycle", *row.PredictedTotalCycleDays))
	}
	if row.OverdueDays != nil && *row.OverdueDays > 0 {
		parts = append(parts, fmt.Sprintf("%.1fd over baseline", *row.OverdueDays))
	}
	if workItemForecastETAClaimAllowed(row, readiness) {
		parts = append(parts, "ETA-ready")
	} else {
		parts = append(parts, "ETA-gated")
	}
	if len(parts) == 0 {
		return "Forecast evidence is available but has no actionable risk signal."
	}
	return strings.Join(parts, "; ") + "."
}

func workItemForecastRiskBadge(row *genent.WorkItemForecast) *model.WorkActionBadge {
	switch row.RiskBand {
	case workitemforecast.RiskBandCritical:
		return &model.WorkActionBadge{Key: "forecast:risk_critical", Label: "Critical risk", Tone: "danger", Detail: optionalString(fmt.Sprintf("score %.0f", row.RiskScore))}
	case workitemforecast.RiskBandHigh:
		return &model.WorkActionBadge{Key: "forecast:risk_high", Label: "High risk", Tone: "warning", Detail: optionalString(fmt.Sprintf("score %.0f", row.RiskScore))}
	case workitemforecast.RiskBandMedium:
		return &model.WorkActionBadge{Key: "forecast:risk_medium", Label: "Medium risk", Tone: "info", Detail: optionalString(fmt.Sprintf("score %.0f", row.RiskScore))}
	case workitemforecast.RiskBandLow:
		return &model.WorkActionBadge{Key: "forecast:risk_low", Label: "Low risk", Tone: "neutral", Detail: optionalString(fmt.Sprintf("score %.0f", row.RiskScore))}
	default:
		return &model.WorkActionBadge{Key: "forecast:risk_unknown", Label: "Risk unknown", Tone: "neutral"}
	}
}
