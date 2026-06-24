package graphql

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workdecisiontargetevaluation"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

func (r *queryResolver) workDecisionTargetEvaluationRows(ctx context.Context, sourceInstance *string, targetKind *string, evaluationKind *string, productActionGateState *string, limit int) ([]*genent.WorkDecisionTargetEvaluation, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workDecisionTargetEvaluations requires an Ent-backed ontology store")
	}
	sourceFilter, err := r.aggregateSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	query := r.EntClient.WorkDecisionTargetEvaluation.Query().
		WithLatestEvidence().
		Where(
			workdecisiontargetevaluation.SourceSystemEQ("cubicle_analytics"),
			workdecisiontargetevaluation.ExternalKindEQ("tpm_decision_target_evaluation"),
		).
		Order(
			workdecisiontargetevaluation.ByEvaluatedAt(entsql.OrderDesc()),
			workdecisiontargetevaluation.ByRankScore(entsql.OrderDesc()),
			workdecisiontargetevaluation.ByUpdatedAt(entsql.OrderDesc()),
			workdecisiontargetevaluation.ByEvaluationKind(),
			workdecisiontargetevaluation.ByModelName(),
			workdecisiontargetevaluation.ByFold(),
		).
		Limit(1000)
	if sourceFilter != nil {
		query = query.Where(workdecisiontargetevaluation.SourceInstanceEQ(*sourceFilter))
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, err
	}
	rows = currentDecisionTargetEvaluationRows(rows)
	rows = filterDecisionTargetEvaluationRows(rows, targetKind, evaluationKind, productActionGateState)
	sortDecisionTargetEvaluationRows(rows)
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func currentDecisionTargetEvaluationRows(rows []*genent.WorkDecisionTargetEvaluation) []*genent.WorkDecisionTargetEvaluation {
	current := latestDecisionTargetEvaluationRunRow(rows)
	if current == nil {
		return rows
	}
	runAt := decisionTargetEvaluationTime(current)
	out := make([]*genent.WorkDecisionTargetEvaluation, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.SourceInstance != current.SourceInstance {
			continue
		}
		if runAt.IsZero() || sameForecastEvaluationRun(decisionTargetEvaluationTime(row), runAt) {
			out = append(out, row)
		}
	}
	return out
}

func latestDecisionTargetEvaluationRunRow(rows []*genent.WorkDecisionTargetEvaluation) *genent.WorkDecisionTargetEvaluation {
	var best *genent.WorkDecisionTargetEvaluation
	for _, row := range rows {
		if row == nil {
			continue
		}
		if best == nil || decisionTargetEvaluationRunRank(row, best) {
			best = row
		}
	}
	return best
}

func decisionTargetEvaluationRunRank(left *genent.WorkDecisionTargetEvaluation, right *genent.WorkDecisionTargetEvaluation) bool {
	leftTime := decisionTargetEvaluationTime(left)
	rightTime := decisionTargetEvaluationTime(right)
	if !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	if decisionTargetEvaluationIsCoverageSummary(left) != decisionTargetEvaluationIsCoverageSummary(right) {
		return decisionTargetEvaluationIsCoverageSummary(left)
	}
	return left.UpdatedAt.After(right.UpdatedAt)
}

func decisionTargetEvaluationTime(row *genent.WorkDecisionTargetEvaluation) time.Time {
	if row == nil {
		return time.Time{}
	}
	if !row.EvaluatedAt.IsZero() {
		return row.EvaluatedAt
	}
	return row.UpdatedAt
}

func filterDecisionTargetEvaluationRows(rows []*genent.WorkDecisionTargetEvaluation, targetKind *string, evaluationKind *string, productActionGateState *string) []*genent.WorkDecisionTargetEvaluation {
	out := rows[:0]
	for _, row := range rows {
		if row == nil {
			continue
		}
		if !matchesStringFilter(row.TargetKind, targetKind) {
			continue
		}
		if !matchesStringFilter(row.EvaluationKind, evaluationKind) {
			continue
		}
		if !matchesStringFilter(row.ProductActionGateState, productActionGateState) {
			continue
		}
		out = append(out, row)
	}
	return out
}

func matchesStringFilter(value string, filter *string) bool {
	if filter == nil {
		return true
	}
	trimmed := strings.TrimSpace(*filter)
	if trimmed == "" || trimmed == "all" {
		return true
	}
	return value == trimmed
}

func sortDecisionTargetEvaluationRows(rows []*genent.WorkDecisionTargetEvaluation) {
	sort.SliceStable(rows, func(i int, j int) bool {
		left := rows[i]
		right := rows[j]
		if decisionTargetEvaluationSortBucket(left) != decisionTargetEvaluationSortBucket(right) {
			return decisionTargetEvaluationSortBucket(left) < decisionTargetEvaluationSortBucket(right)
		}
		if left.RankScore != right.RankScore {
			return left.RankScore > right.RankScore
		}
		if left.TargetKind != right.TargetKind {
			return left.TargetKind < right.TargetKind
		}
		if left.EvaluationKind != right.EvaluationKind {
			return left.EvaluationKind < right.EvaluationKind
		}
		if left.ModelName != right.ModelName {
			return left.ModelName < right.ModelName
		}
		return left.Fold < right.Fold
	})
}

func decisionTargetEvaluationSortBucket(row *genent.WorkDecisionTargetEvaluation) int {
	if row == nil {
		return 100
	}
	if decisionTargetEvaluationIsCoverageSummary(row) {
		return 0
	}
	if !row.ReadyForProductAction {
		return 1
	}
	return 2
}

func decisionTargetEvaluationIsCoverageSummary(row *genent.WorkDecisionTargetEvaluation) bool {
	if row == nil {
		return false
	}
	return strings.Contains(row.EvaluationKind, "coverage_stratified_summary") || row.ModelName == "coverage_guardrail"
}

func workDecisionTargetEvaluationModels(rows []*genent.WorkDecisionTargetEvaluation) []*model.WorkDecisionTargetEvaluation {
	gate := workDecisionTargetCoverageGate(rows)
	out := make([]*model.WorkDecisionTargetEvaluation, 0, len(rows))
	for _, row := range rows {
		out = append(out, workDecisionTargetEvaluationModel(row, gate))
	}
	return out
}

func workDecisionTargetEvaluationModel(row *genent.WorkDecisionTargetEvaluation, gate workDecisionTargetCoverageGateResult) *model.WorkDecisionTargetEvaluation {
	if row == nil {
		return nil
	}
	productActionAllowed := workDecisionTargetProductActionAllowed(row, gate)
	return &model.WorkDecisionTargetEvaluation{
		Key:                     row.Key,
		SourceInstance:          optionalString(row.SourceInstance),
		TargetKind:              row.TargetKind,
		EvaluationKind:          row.EvaluationKind,
		ModelName:               row.ModelName,
		Fold:                    row.Fold,
		TrainCount:              row.TrainCount,
		TestCount:               row.TestCount,
		PositiveCount:           row.PositiveCount,
		BaselinePositiveRate:    optionalFloat(row.BaselinePositiveRate),
		PrecisionAt10pct:        optionalFloat(row.PrecisionAt10pct),
		LiftAt10pct:             optionalFloat(row.LiftAt10pct),
		RocAuc:                  optionalFloat(row.RocAuc),
		AveragePrecision:        optionalFloat(row.AveragePrecision),
		CoverageStratum:         optionalString(row.CoverageStratum),
		ReadyForProductAction:   row.ReadyForProductAction,
		ProductActionAllowed:    productActionAllowed,
		ProductActionGateState:  row.ProductActionGateState,
		ProductActionGateReason: row.ProductActionGateReason,
		DecisionClaimUse:        workDecisionTargetClaimUse(row, productActionAllowed),
		DecisionClaimGateReason: workDecisionTargetClaimGateReason(row, gate, productActionAllowed),
		Note:                    optionalString(row.Note),
		EvaluatedAt:             optionalTime(row.EvaluatedAt),
		EvidenceCount:           row.EvidenceCount,
		FreshnessState:          row.FreshnessState.String(),
		Visibility:              row.Visibility.String(),
		Confidence:              row.Confidence,
		RankScore:               row.RankScore,
		EvidenceRef:             optionalString(firstNonempty(evidenceRef(row.Edges.LatestEvidence), row.SourceURL)),
		Evidence:                workEvidenceSummary(row.Edges.LatestEvidence),
		Badges:                  workDecisionTargetEvaluationBadges(row, productActionAllowed),
	}
}

func workDecisionTargetClaimUse(row *genent.WorkDecisionTargetEvaluation, productActionAllowed bool) string {
	if productActionAllowed {
		return "product_decision_support"
	}
	if decisionTargetEvaluationIsCoverageSummary(row) {
		return "coverage_validation_guardrail"
	}
	return "ranking_validation_only"
}

func workDecisionTargetClaimGateReason(row *genent.WorkDecisionTargetEvaluation, gate workDecisionTargetCoverageGateResult, productActionAllowed bool) string {
	if productActionAllowed {
		return "product_action_ready"
	}
	if gate.state != "passed" {
		return gate.state
	}
	if workDecisionTargetGeneratedOnlyEvidence(row) {
		return "generated_evidence_only"
	}
	if strings.TrimSpace(row.ProductActionGateState) != "" {
		return row.ProductActionGateState
	}
	return "decision_target_validation_only"
}

func workDecisionTargetEvaluationBadges(row *genent.WorkDecisionTargetEvaluation, productActionAllowed bool) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{}
	if productActionAllowed {
		badges = append(badges, &model.WorkActionBadge{Key: "decision_target:product_ready", Label: "Decision target ready", Tone: "success", Detail: optionalString(row.ProductActionGateReason)})
	} else if row.ProductActionGateState == "validation_gated" {
		badges = append(badges, &model.WorkActionBadge{Key: "decision_target:validation_gated", Label: "Validation gated", Tone: "warning", Detail: optionalString(row.ProductActionGateReason)})
	} else {
		badges = append(badges, &model.WorkActionBadge{Key: "decision_target:gated", Label: "Decision gated", Tone: "warning", Detail: optionalString(row.ProductActionGateReason)})
	}
	if decisionTargetEvaluationIsCoverageSummary(row) {
		badges = append(badges, &model.WorkActionBadge{Key: "decision_target:coverage_guardrail", Label: "Coverage guardrail", Tone: "warning", Detail: optionalString(row.CoverageStratum)})
	}
	if row.PrecisionAt10pct != nil {
		badges = append(badges, &model.WorkActionBadge{Key: "decision_target:precision_at_10pct", Label: "Precision@10%", Tone: "info", Detail: optionalString(fmt.Sprintf("%.4f", *row.PrecisionAt10pct))})
	}
	return badges
}

func workDecisionTargetEvaluationState(rows []*genent.WorkDecisionTargetEvaluation) string {
	if len(rows) == 0 {
		return "missing"
	}
	gate := workDecisionTargetCoverageGate(rows)
	if gate.state != "passed" {
		return gate.state
	}
	hasGated := false
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.ProductActionGateState == "validation_gated" {
			return "validation_gated"
		}
		if !row.ReadyForProductAction {
			hasGated = true
		}
	}
	if hasGated {
		return "gated"
	}
	return "product_action_ready"
}

func workDecisionTargetEvaluationProductReadyCount(rows []*genent.WorkDecisionTargetEvaluation) int {
	gate := workDecisionTargetCoverageGate(rows)
	count := 0
	for _, row := range rows {
		if workDecisionTargetProductActionAllowed(row, gate) {
			count++
		}
	}
	return count
}

func workDecisionTargetEvaluationHumanRequired(rows []*genent.WorkDecisionTargetEvaluation) bool {
	if len(rows) == 0 {
		return false
	}
	return workDecisionTargetEvaluationProductReadyCount(rows) < len(rows)
}

type workDecisionTargetCoverageGateResult struct {
	state                 string
	reason                string
	stratum               string
	generatedEvidenceOnly bool
}

func workDecisionTargetCoverageGate(rows []*genent.WorkDecisionTargetEvaluation) workDecisionTargetCoverageGateResult {
	out := workDecisionTargetCoverageGateResult{
		state:  "missing_coverage_guardrail",
		reason: "No current-run coverage-stratified guardrail row is available for decision-target evaluation.",
	}
	if len(rows) == 0 {
		out.state = "missing"
		out.reason = "No current-run decision-target evaluation rows are available."
		return out
	}
	out.generatedEvidenceOnly = true
	var coverage *genent.WorkDecisionTargetEvaluation
	for _, row := range rows {
		if row == nil {
			continue
		}
		if !workDecisionTargetGeneratedOnlyEvidence(row) {
			out.generatedEvidenceOnly = false
		}
		if decisionTargetEvaluationIsCoverageSummary(row) && coverage == nil {
			coverage = row
		}
	}
	if coverage == nil {
		return out
	}
	out.stratum = strings.TrimSpace(coverage.CoverageStratum)
	out.reason = firstNonempty(strings.TrimSpace(coverage.ProductActionGateReason), strings.TrimSpace(coverage.Note), "Coverage-stratified decision-target validation is not product-action ready.")
	text := strings.ToLower(strings.Join([]string{coverage.CoverageStratum, coverage.ProductActionGateState, coverage.ProductActionGateReason, coverage.Note}, " "))
	if strings.Contains(text, "not_testable") || strings.Contains(text, "insufficient") || strings.Contains(text, "cannot be tested") {
		out.state = "validation_gated"
		return out
	}
	if !coverage.ReadyForProductAction || !workDecisionTargetGatePassed(coverage.ProductActionGateState) {
		out.state = firstNonempty(strings.TrimSpace(coverage.ProductActionGateState), "validation_gated")
		return out
	}
	out.state = "passed"
	out.reason = "Coverage-stratified decision-target validation passed for the current run."
	return out
}

func workDecisionTargetProductActionAllowed(row *genent.WorkDecisionTargetEvaluation, gate workDecisionTargetCoverageGateResult) bool {
	if row == nil || !row.ReadyForProductAction {
		return false
	}
	if gate.state != "passed" {
		return false
	}
	if workDecisionTargetGeneratedOnlyEvidence(row) {
		return false
	}
	return workDecisionTargetGatePassed(row.ProductActionGateState)
}

func workDecisionTargetGatePassed(state string) bool {
	switch strings.TrimSpace(state) {
	case "passed", "ready", "product_action_ready":
		return true
	default:
		return false
	}
}

func workDecisionTargetGeneratedOnlyEvidence(row *genent.WorkDecisionTargetEvaluation) bool {
	if row == nil {
		return true
	}
	evidence := row.Edges.LatestEvidence
	if evidence == nil {
		return true
	}
	if strings.TrimSpace(evidence.SourceInstance) != "" && strings.TrimSpace(row.SourceInstance) != "" && evidence.SourceInstance != row.SourceInstance {
		return true
	}
	return evidence.SourceSystem == "cubicle_analytics" || evidence.ExternalKind == "tpm_generated_evidence"
}

func (r *queryResolver) workDecisionTargetReadinessModel(ctx context.Context, sourceInstance *string, limit int) (*model.WorkDecisionTargetReadiness, error) {
	rows, err := r.workDecisionTargetEvaluationRows(ctx, sourceInstance, nil, nil, nil, 0)
	if err != nil {
		return nil, err
	}
	displayRows := rows
	if limit > 0 && len(displayRows) > limit {
		displayRows = displayRows[:limit]
	}
	gate := workDecisionTargetCoverageGate(rows)
	productReadyCount := workDecisionTargetEvaluationProductReadyCount(rows)
	validationGatedCount := 0
	for _, row := range rows {
		if row != nil && !workDecisionTargetProductActionAllowed(row, gate) {
			validationGatedCount++
		}
	}
	state := workDecisionTargetEvaluationState(rows)
	productActionReady := len(rows) > 0 && productReadyCount == len(rows)
	recommendedFocus := workDecisionTargetReadinessFocus(gate, rows)
	return &model.WorkDecisionTargetReadiness{
		SourceInstance:                 decisionTargetReadinessSourceInstance(sourceInstance, rows),
		EvaluatedAt:                    optionalTime(decisionTargetReadinessEvaluatedAt(rows)),
		EvaluationState:                state,
		ProductActionReady:             productActionReady,
		EvaluationCount:                len(rows),
		ProductReadyEvaluationCount:    productReadyCount,
		ValidationGatedEvaluationCount: validationGatedCount,
		CoverageGateState:              gate.state,
		CoverageGateReason:             gate.reason,
		CoverageStratum:                optionalString(gate.stratum),
		GeneratedEvidenceOnly:          gate.generatedEvidenceOnly,
		AutomationSummary:              workDecisionTargetReadinessSummary(state, productActionReady, len(rows), productReadyCount, validationGatedCount, gate, recommendedFocus),
		RecommendedFocus:               recommendedFocus,
		Evaluations:                    workDecisionTargetEvaluationModels(displayRows),
		Badges:                         workDecisionTargetReadinessBadges(state, gate, len(rows), productReadyCount, validationGatedCount),
	}, nil
}

func decisionTargetReadinessSourceInstance(sourceInstance *string, rows []*genent.WorkDecisionTargetEvaluation) *string {
	if sourceInstance != nil && strings.TrimSpace(*sourceInstance) != "" {
		return sourceInstance
	}
	for _, row := range rows {
		if row != nil && strings.TrimSpace(row.SourceInstance) != "" {
			return optionalString(row.SourceInstance)
		}
	}
	return nil
}

func decisionTargetReadinessEvaluatedAt(rows []*genent.WorkDecisionTargetEvaluation) time.Time {
	var best time.Time
	for _, row := range rows {
		t := decisionTargetEvaluationTime(row)
		if t.After(best) {
			best = t
		}
	}
	return best
}

func workDecisionTargetReadinessFocus(gate workDecisionTargetCoverageGateResult, rows []*genent.WorkDecisionTargetEvaluation) *string {
	if gate.state != "passed" && strings.TrimSpace(gate.reason) != "" {
		return optionalString(gate.reason)
	}
	for _, row := range rows {
		if row == nil {
			continue
		}
		if !workDecisionTargetProductActionAllowed(row, gate) && strings.TrimSpace(row.ProductActionGateReason) != "" {
			return optionalString(row.ProductActionGateReason)
		}
	}
	return nil
}

func workDecisionTargetReadinessSummary(state string, productActionReady bool, evaluationCount int, productReadyCount int, validationGatedCount int, gate workDecisionTargetCoverageGateResult, recommendedFocus *string) string {
	usage := "decision-target rows are validation evidence for risk ranking only"
	if productActionReady {
		usage = "decision-target rows may support product decision recommendations after owner confirmation"
	}
	summary := fmt.Sprintf("Decision-target evaluation is %s; %s. %d evaluation row(s), %d product-ready row(s), %d gated row(s). Coverage gate is %s.", state, usage, evaluationCount, productReadyCount, validationGatedCount, gate.state)
	if gate.generatedEvidenceOnly {
		summary += " Current evidence is generated evidence only."
	}
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}

func workDecisionTargetReadinessBadges(state string, gate workDecisionTargetCoverageGateResult, evaluationCount int, productReadyCount int, validationGatedCount int) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		countBadge("decision_target:evaluations", "Decision-target evals", "info", evaluationCount),
	}
	if productReadyCount > 0 {
		badges = append(badges, countBadge("decision_target:product_ready", "Decision target ready", "success", productReadyCount))
	}
	if validationGatedCount > 0 {
		badges = append(badges, countBadge("decision_target:gated", "Decision target gated", "warning", validationGatedCount))
	}
	if gate.state != "passed" {
		badges = append(badges, &model.WorkActionBadge{Key: "decision_target:coverage_gate", Label: "Coverage gated", Tone: "warning", Detail: optionalString(gate.reason)})
	}
	if state == "missing" {
		badges = append(badges, &model.WorkActionBadge{Key: "decision_target:missing", Label: "Decision eval missing", Tone: "warning"})
	}
	return badges
}
