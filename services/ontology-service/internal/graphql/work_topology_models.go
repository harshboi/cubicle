package graphql

import (
	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workblockerimpact"
	"cubicle/services/ontology-service/ent/workdependencyedge"
	"cubicle/services/ontology-service/internal/graphql/model"
	"fmt"
)

func workBlockerModel(row *genent.WorkBlocker) *model.WorkBlocker {
	return &model.WorkBlocker{
		Key:                  row.Key,
		SourceInstance:       optionalString(row.SourceInstance),
		BlockerKind:          row.BlockerKind.String(),
		BlockerState:         row.BlockerState.String(),
		Severity:             row.Severity.String(),
		SubjectKind:          row.SubjectKind.String(),
		SubjectKey:           row.SubjectKey,
		Title:                row.Title,
		Summary:              optionalString(row.Summary),
		RecommendedAction:    optionalString(row.RecommendedAction),
		OwnerKey:             optionalString(row.OwnerKey),
		OwnerSource:          optionalString(row.OwnerSource),
		DecisionState:        row.DecisionState.String(),
		SourceCoverageState:  optionalString(row.SourceCoverageState),
		ReviewState:          row.ReviewState.String(),
		TruthLabel:           row.TruthLabel.String(),
		ActionabilityLabel:   row.ActionabilityLabel.String(),
		LabelQuality:         row.LabelQuality.String(),
		MeasurementEligible:  row.MeasurementEligible,
		ReviewerKind:         row.ReviewerKind.String(),
		ReviewerKey:          optionalString(row.ReviewerKey),
		LabelSet:             optionalString(row.LabelSet),
		ClaimUse:             workBlockerClaimUse(row),
		ClaimGateReason:      workBlockerClaimGateReason(row),
		ProductActionAllowed: workBlockerProductActionAllowed(row),
		BlockerClaimAllowed:  workBlockerClaimAllowed(row),
		AbsenceClaimAllowed:  workBlockerAbsenceClaimAllowed(row),
		ActionKey:            optionalWorkActionKey(row.Edges.WorkAction),
		SourceInsightKey:     optionalWorkInsightKey(row.Edges.WorkInsight),
		SourceURL:            optionalString(row.SourceURL),
		EvidenceRef:          optionalString(workBlockerEvidenceRef(row)),
		Evidence:             workEvidenceSummary(row.Edges.LatestEvidence),
		RankScore:            row.RankScore,
		FreshnessState:       row.FreshnessState.String(),
		Visibility:           row.Visibility.String(),
		Confidence:           row.Confidence,
		Badges:               workBlockerBadges(row),
	}
}

func workBlockerModels(rows []*genent.WorkBlocker) []*model.WorkBlocker {
	out := make([]*model.WorkBlocker, 0, len(rows))
	for _, row := range rows {
		out = append(out, workBlockerModel(row))
	}
	return out
}

func workDependencyEdgeModel(row *genent.WorkDependencyEdge) *model.WorkDependencyEdge {
	endpoints := workDependencyEndpointModels(row.Edges.Endpoints)
	canonicalRelationshipKind := row.CanonicalRelationshipKind.String()
	var fromEndpoint *model.WorkDependencyEndpoint
	var toEndpoint *model.WorkDependencyEndpoint
	for _, endpoint := range endpoints {
		switch endpoint.EndpointRole {
		case "from":
			fromEndpoint = endpoint
		case "to":
			toEndpoint = endpoint
		}
	}
	return &model.WorkDependencyEdge{
		Key:                       row.Key,
		SourceInstance:            optionalString(row.SourceInstance),
		EdgeKind:                  row.EdgeKind.String(),
		RelationshipAuthority:     row.RelationshipAuthority.String(),
		CanonicalRelationshipKind: optionalString(canonicalRelationshipKind),
		FromKind:                  row.FromKind.String(),
		FromKey:                   row.FromKey,
		ToKind:                    row.ToKind.String(),
		ToKey:                     row.ToKey,
		FromEndpoint:              fromEndpoint,
		ToEndpoint:                toEndpoint,
		Endpoints:                 endpoints,
		RiskSignal:                optionalString(row.RiskSignal),
		SourceCoverageState:       optionalString(row.SourceCoverageState),
		ClaimUse:                  workDependencyEdgeClaimUse(row),
		ClaimGateReason:           workDependencyEdgeClaimGateReason(row),
		RelationshipClaimAllowed:  workDependencyEdgeRelationshipClaimAllowed(row),
		SourceURL:                 optionalString(row.SourceURL),
		EvidenceRef:               optionalString(workDependencyEdgeEvidenceRef(row)),
		Evidence:                  workEvidenceSummary(row.Edges.LatestEvidence),
		RankScore:                 row.RankScore,
		FreshnessState:            row.FreshnessState.String(),
		Visibility:                row.Visibility.String(),
		Confidence:                row.Confidence,
		Badges:                    workDependencyEdgeBadges(row),
	}
}

func workDependencyEdgeModels(rows []*genent.WorkDependencyEdge) []*model.WorkDependencyEdge {
	out := make([]*model.WorkDependencyEdge, 0, len(rows))
	for _, row := range rows {
		out = append(out, workDependencyEdgeModel(row))
	}
	return out
}

func workDependencyEndpointModels(rows []*genent.WorkDependencyEndpoint) []*model.WorkDependencyEndpoint {
	out := make([]*model.WorkDependencyEndpoint, 0, len(rows))
	for _, row := range rows {
		out = append(out, workDependencyEndpointModel(row))
	}
	return out
}

func workDependencyEndpointModel(row *genent.WorkDependencyEndpoint) *model.WorkDependencyEndpoint {
	return &model.WorkDependencyEndpoint{
		Key:              row.Key,
		EndpointRole:     row.EndpointRole.String(),
		NodeKind:         row.NodeKind.String(),
		NodeKey:          row.NodeKey,
		ResolutionState:  row.ResolutionState.String(),
		ResolutionReason: optionalString(row.ResolutionReason),
		SourceURL:        optionalString(row.SourceURL),
		EvidenceRef:      optionalString(workDependencyEndpointEvidenceRef(row)),
		Evidence:         workEvidenceSummary(row.Edges.LatestEvidence),
		RankScore:        row.RankScore,
		FreshnessState:   row.FreshnessState.String(),
		Visibility:       row.Visibility.String(),
		Confidence:       row.Confidence,
	}
}

func workBlockerImpactModel(row *genent.WorkBlockerImpact) *model.WorkBlockerImpact {
	blockerState := row.ImpactState.String()
	if row.Edges.WorkBlocker != nil {
		blockerState = row.Edges.WorkBlocker.BlockerState.String()
	}
	return &model.WorkBlockerImpact{
		Key:                 row.Key,
		SourceInstance:      optionalString(row.SourceInstance),
		ImpactKind:          row.ImpactKind.String(),
		ImpactState:         row.ImpactState.String(),
		ImpactScore:         row.ImpactScore,
		Severity:            row.Severity.String(),
		BlockerKind:         row.BlockerKind.String(),
		BlockerKey:          optionalWorkBlockerKey(row.Edges.WorkBlocker),
		BlockerState:        blockerState,
		ActionKey:           optionalWorkActionKey(row.Edges.WorkAction),
		WorkstreamKey:       optionalWorkstreamKey(row.Edges.Workstream),
		AffectedKind:        row.AffectedKind.String(),
		AffectedKey:         row.AffectedKey,
		SubjectKind:         row.SubjectKind.String(),
		SubjectKey:          row.SubjectKey,
		PathLength:          row.PathLength,
		Title:               row.Title,
		Summary:             optionalString(row.Summary),
		RecommendedAction:   optionalString(row.RecommendedAction),
		SourceCoverageState: optionalString(row.SourceCoverageState),
		ClaimUse:            workBlockerImpactClaimUse(row),
		ClaimGateReason:     workBlockerImpactClaimGateReason(row),
		ImpactClaimAllowed:  workBlockerImpactClaimAllowed(row),
		SourceURL:           optionalString(row.SourceURL),
		EvidenceRef:         optionalString(workBlockerImpactEvidenceRef(row)),
		Evidence:            workEvidenceSummary(row.Edges.LatestEvidence),
		RankScore:           row.RankScore,
		FreshnessState:      row.FreshnessState.String(),
		Visibility:          row.Visibility.String(),
		Confidence:          row.Confidence,
		Badges:              workBlockerImpactBadges(row),
	}
}

func workBlockerImpactModels(rows []*genent.WorkBlockerImpact) []*model.WorkBlockerImpact {
	out := make([]*model.WorkBlockerImpact, 0, len(rows))
	for _, row := range rows {
		out = append(out, workBlockerImpactModel(row))
	}
	return out
}

func optionalWorkActionKey(row *genent.WorkAction) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.Key)
}

func optionalWorkBlockerKey(row *genent.WorkBlocker) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.Key)
}

func optionalWorkstreamKey(row *genent.Workstream) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.Key)
}

func optionalWorkInsightKey(row *genent.WorkInsight) *string {
	if row == nil {
		return nil
	}
	return optionalString(row.Key)
}

func workBlockerEvidenceRef(row *genent.WorkBlocker) string {
	return firstNonempty(evidenceRef(row.Edges.LatestEvidence), row.SourceURL, row.ExternalID, row.Key)
}

func workDependencyEdgeEvidenceRef(row *genent.WorkDependencyEdge) string {
	return firstNonempty(evidenceRef(row.Edges.LatestEvidence), row.SourceURL, row.ExternalID, row.Key)
}

func workDependencyEndpointEvidenceRef(row *genent.WorkDependencyEndpoint) string {
	return firstNonempty(evidenceRef(row.Edges.LatestEvidence), row.SourceURL, row.ExternalID, row.Key)
}

func workBlockerImpactEvidenceRef(row *genent.WorkBlockerImpact) string {
	return firstNonempty(evidenceRef(row.Edges.LatestEvidence), row.SourceURL, row.ExternalID, row.Key)
}

func workBlockerClaimUse(row *genent.WorkBlocker) string {
	if workBlockerClaimAllowed(row) {
		return "blocker_claim"
	}
	if workBlockerAbsenceClaimAllowed(row) {
		return "source_resolved_absence_claim"
	}
	if workBlockerCoverageLimited(row) {
		return "source_coverage_validation"
	}
	switch row.DecisionState {
	case workblocker.DecisionStateValidationLead:
		return "blocker_validation"
	case workblocker.DecisionStateSourceRepair:
		return "source_repair"
	case workblocker.DecisionStateCloseoutReview:
		return "closeout_review"
	case workblocker.DecisionStateSourceResolved:
		return "source_resolved_validation"
	case workblocker.DecisionStateModelOrRuleQa:
		return "model_or_rule_qa"
	case workblocker.DecisionStateSuppressedSignal:
		return "suppressed_signal"
	default:
		return "pending_review"
	}
}

func workBlockerClaimGateReason(row *genent.WorkBlocker) string {
	if workBlockerCoverageLimited(row) {
		return "source_coverage_limited"
	}
	if workBlockerClaimAllowed(row) {
		return "blocker_claim_gate_passed"
	}
	if workBlockerAbsenceClaimAllowed(row) {
		return "authenticated_terminal_source_observed"
	}
	if row.DecisionState != workblocker.DecisionStateProductAction {
		return "decision_state_" + row.DecisionState.String()
	}
	if row.BlockerState != workblocker.BlockerStateActive {
		return "blocker_state_not_active"
	}
	if row.ReviewState != workblocker.ReviewStateAccepted && row.ReviewState != workblocker.ReviewStateResolved {
		return "blocker_claim_requires_accepted_review"
	}
	if row.TruthLabel != workblocker.TruthLabelTruePositive {
		return "blocker_claim_requires_true_positive_label"
	}
	if row.ActionabilityLabel != workblocker.ActionabilityLabelActionable && row.ActionabilityLabel != workblocker.ActionabilityLabelNeedsOwner {
		return "blocker_claim_requires_actionable_label"
	}
	if row.LabelQuality != workblocker.LabelQualityGold || !row.MeasurementEligible {
		return "blocker_claim_requires_gold_measurement_label"
	}
	return "blocker_claim_not_ready"
}

func workBlockerProductActionAllowed(row *genent.WorkBlocker) bool {
	return workBlockerClaimAllowed(row)
}

func workBlockerClaimAllowed(row *genent.WorkBlocker) bool {
	return workBlockerProductActionBaseAllowed(row) &&
		row.BlockerState == workblocker.BlockerStateActive &&
		(row.ReviewState == workblocker.ReviewStateAccepted || row.ReviewState == workblocker.ReviewStateResolved) &&
		row.TruthLabel == workblocker.TruthLabelTruePositive &&
		(row.ActionabilityLabel == workblocker.ActionabilityLabelActionable || row.ActionabilityLabel == workblocker.ActionabilityLabelNeedsOwner) &&
		row.LabelQuality == workblocker.LabelQualityGold &&
		row.MeasurementEligible
}

func workBlockerProductActionBaseAllowed(row *genent.WorkBlocker) bool {
	return row.DecisionState == workblocker.DecisionStateProductAction && !workBlockerCoverageLimited(row)
}

func workBlockerAbsenceClaimAllowed(row *genent.WorkBlocker) bool {
	return row.DecisionState == workblocker.DecisionStateSourceResolved &&
		(row.BlockerState == workblocker.BlockerStateResolved || row.BlockerState == workblocker.BlockerStateDismissed) &&
		(row.ReviewState == workblocker.ReviewStateAccepted || row.ReviewState == workblocker.ReviewStateResolved) &&
		!workBlockerCoverageLimited(row)
}

func workBlockerCoverageLimited(row *genent.WorkBlocker) bool {
	return row.FreshnessState != workblocker.FreshnessStateFresh || workClaimCoverageStateLimited(row.SourceCoverageState)
}

func workDependencyEdgeClaimUse(row *genent.WorkDependencyEdge) string {
	if workDependencyEdgeRelationshipClaimAllowed(row) {
		return "relationship_claim"
	}
	if workDependencyEdgeCoverageLimited(row) {
		return "source_coverage_validation"
	}
	switch row.EdgeKind {
	case workdependencyedge.EdgeKindBlockedBy:
		return "blocked_by_validation"
	case workdependencyedge.EdgeKindNeedsAction:
		return "needs_action_validation"
	default:
		return "topology_context"
	}
}

func workDependencyEdgeClaimGateReason(row *genent.WorkDependencyEdge) string {
	if workDependencyEdgeCoverageLimited(row) {
		return "source_coverage_limited"
	}
	if row.EdgeKind != workdependencyedge.EdgeKindBlockedBy && row.EdgeKind != workdependencyedge.EdgeKindNeedsAction {
		return "topology_context_not_product_claim"
	}
	if row.Confidence < 0.8 {
		return "relationship_claim_requires_high_confidence"
	}
	if row.EdgeKind == workdependencyedge.EdgeKindBlockedBy && row.WorkBlockerID == 0 {
		return "relationship_claim_requires_blocker_context"
	}
	if row.WorkBlockerID != 0 {
		if row.Edges.WorkBlocker == nil {
			return "linked_blocker_claim_not_loaded"
		}
		if !workBlockerClaimAllowed(row.Edges.WorkBlocker) {
			return "linked_blocker_claim_not_allowed"
		}
	}
	if row.EdgeKind == workdependencyedge.EdgeKindNeedsAction && row.WorkActionID == 0 {
		return "relationship_claim_requires_action_context"
	}
	if row.WorkActionID != 0 {
		if row.Edges.WorkAction == nil {
			return "linked_action_claim_not_loaded"
		}
		if !workActionProductActionAllowed(row.Edges.WorkAction) {
			return "linked_action_claim_not_allowed"
		}
	}
	return "derived_dependency_edge_not_product_claim"
}

func workDependencyEdgeRelationshipClaimAllowed(row *genent.WorkDependencyEdge) bool {
	// WorkDependencyEdge is an operating-topology edge. The linked WorkAction or
	// WorkBlocker can be claimable, but the derived edge is not product truth.
	return false
}

func workDependencyEdgeCoverageLimited(row *genent.WorkDependencyEdge) bool {
	return row.FreshnessState != workdependencyedge.FreshnessStateFresh || workClaimCoverageStateLimited(row.SourceCoverageState)
}

func workBlockerImpactClaimUse(row *genent.WorkBlockerImpact) string {
	if workBlockerImpactClaimAllowed(row) {
		return "impact_claim"
	}
	if workBlockerImpactCoverageLimited(row) {
		return "source_coverage_validation"
	}
	switch row.ImpactState {
	case workblockerimpact.ImpactStateValidating:
		return "impact_validation"
	case workblockerimpact.ImpactStateResolved:
		return "impact_closeout"
	case workblockerimpact.ImpactStateDismissed:
		return "suppressed_signal"
	default:
		return "impact_context"
	}
}

func workBlockerImpactClaimGateReason(row *genent.WorkBlockerImpact) string {
	if workBlockerImpactCoverageLimited(row) {
		return "source_coverage_limited"
	}
	if workBlockerImpactClaimAllowed(row) {
		return "impact_claim_gate_passed"
	}
	if row.ImpactState != workblockerimpact.ImpactStateActive {
		return "impact_state_not_active"
	}
	if row.WorkBlockerID != 0 && row.Edges.WorkBlocker == nil {
		return "linked_blocker_claim_not_loaded"
	}
	if row.Edges.WorkBlocker != nil && !workBlockerClaimAllowed(row.Edges.WorkBlocker) {
		return "linked_blocker_claim_not_allowed"
	}
	return "impact_claim_requires_blocker_claim_context"
}

func workBlockerImpactClaimAllowed(row *genent.WorkBlockerImpact) bool {
	if workBlockerImpactCoverageLimited(row) || row.ImpactState != workblockerimpact.ImpactStateActive || row.Confidence < 0.8 {
		return false
	}
	if row.WorkBlockerID != 0 {
		return row.Edges.WorkBlocker != nil && workBlockerClaimAllowed(row.Edges.WorkBlocker)
	}
	return true
}

func workBlockerImpactCoverageLimited(row *genent.WorkBlockerImpact) bool {
	return row.FreshnessState != workblockerimpact.FreshnessStateFresh || workClaimCoverageStateLimited(row.SourceCoverageState)
}

func workBlockerBadges(row *genent.WorkBlocker) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{
			Key:    "blocker:" + row.BlockerState.String(),
			Label:  blockerStateLabel(row.BlockerState),
			Tone:   blockerStateTone(row.BlockerState),
			Detail: optionalString(row.BlockerKind.String()),
		},
		{
			Key:    "decision:" + row.DecisionState.String(),
			Label:  workBlockerDecisionBadgeLabel(row),
			Tone:   workBlockerDecisionBadgeTone(row),
			Detail: optionalString(workBlockerDecisionBadgeDetail(row)),
		},
	}
	if row.Severity == workblocker.SeverityHigh || row.Severity == workblocker.SeverityCritical {
		badges = append(badges, &model.WorkActionBadge{
			Key:   "severity:" + row.Severity.String(),
			Label: severityLabel(row.Severity.String()),
			Tone:  severityTone(row.Severity.String()),
		})
	}
	if row.MeasurementEligible {
		badges = append(badges, &model.WorkActionBadge{Key: "review:measurement_eligible", Label: "Measurement label", Tone: "success", Detail: optionalString(row.LabelSet)})
	}
	if row.ReviewerKind != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "reviewer:" + row.ReviewerKind.String(), Label: reviewerKindLabel(row.ReviewerKind.String()), Tone: "info", Detail: optionalString(row.ReviewerKey)})
	}
	if row.TruthLabel != workblocker.TruthLabelUnknown {
		badges = append(badges, &model.WorkActionBadge{Key: "truth:" + row.TruthLabel.String(), Label: truthLabel(row.TruthLabel.String()), Tone: truthTone(row.TruthLabel.String())})
	}
	if row.ActionabilityLabel != workblocker.ActionabilityLabelUnknown {
		badges = append(badges, &model.WorkActionBadge{Key: "actionability:" + row.ActionabilityLabel.String(), Label: actionabilityLabel(row.ActionabilityLabel.String()), Tone: actionabilityTone(row.ActionabilityLabel.String())})
	}
	if row.SourceCoverageState != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "coverage", Label: "Coverage", Tone: "info", Detail: optionalString(row.SourceCoverageState)})
	}
	return badges
}

func workBlockerDecisionBadgeLabel(row *genent.WorkBlocker) string {
	if row == nil {
		return "unknown"
	}
	if row != nil && row.DecisionState == workblocker.DecisionStateProductAction && !workBlockerClaimAllowed(row) {
		return "Blocker validation"
	}
	return decisionStateLabel(row.DecisionState.String())
}

func workBlockerDecisionBadgeTone(row *genent.WorkBlocker) string {
	if row == nil {
		return "neutral"
	}
	if row != nil && row.DecisionState == workblocker.DecisionStateProductAction && !workBlockerClaimAllowed(row) {
		return "warning"
	}
	return decisionStateTone(row.DecisionState.String())
}

func workBlockerDecisionBadgeDetail(row *genent.WorkBlocker) string {
	if row == nil {
		return "unknown"
	}
	if row != nil && row.DecisionState == workblocker.DecisionStateProductAction && !workBlockerClaimAllowed(row) {
		return workBlockerClaimGateReason(row)
	}
	return row.DecisionState.String()
}

func workBlockerImpactBadges(row *genent.WorkBlockerImpact) []*model.WorkActionBadge {
	badges := []*model.WorkActionBadge{
		{Key: "impact:" + row.ImpactKind.String(), Label: impactKindLabel(row.ImpactKind), Tone: impactKindTone(row.ImpactKind)},
		{Key: "impact_state:" + row.ImpactState.String(), Label: blockerStateLabel(workblocker.BlockerState(row.ImpactState.String())), Tone: blockerStateTone(workblocker.BlockerState(row.ImpactState.String()))},
		{Key: "severity:" + row.Severity.String(), Label: severityLabel(row.Severity.String()), Tone: severityTone(row.Severity.String())},
	}
	if row.WorkstreamID != 0 && row.Edges.Workstream != nil {
		badges = append(badges, &model.WorkActionBadge{Key: "affected:workstream", Label: "Workstream", Tone: "info", Detail: optionalString(row.Edges.Workstream.Key)})
	}
	if row.PathLength > 0 {
		badges = append(badges, &model.WorkActionBadge{Key: "path:hops", Label: "Path hops", Tone: "info", Detail: optionalString(fmtInt(row.PathLength))})
	}
	if row.SourceCoverageState != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "coverage", Label: "Coverage", Tone: "info", Detail: optionalString(row.SourceCoverageState)})
	}
	return badges
}

func workDependencyEdgeBadges(row *genent.WorkDependencyEdge) []*model.WorkActionBadge {
	canonicalRelationshipKind := row.CanonicalRelationshipKind.String()
	badges := []*model.WorkActionBadge{
		{Key: "edge:" + row.EdgeKind.String(), Label: edgeKindLabel(row.EdgeKind), Tone: "info"},
		{Key: "authority:" + row.RelationshipAuthority.String(), Label: relationshipAuthorityLabel(row.RelationshipAuthority), Tone: relationshipAuthorityTone(row.RelationshipAuthority)},
		{Key: "freshness:" + row.FreshnessState.String(), Label: freshnessLabel(row.FreshnessState.String()), Tone: freshnessTone(row.FreshnessState.String())},
	}
	if canonicalRelationshipKind != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "canonical:" + canonicalRelationshipKind, Label: "Canonical", Tone: "success", Detail: optionalString(canonicalRelationshipKind)})
	}
	if row.RiskSignal != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "risk:" + row.RiskSignal, Label: "Risk", Tone: "warning", Detail: optionalString(row.RiskSignal)})
	}
	if row.SourceCoverageState != "" {
		badges = append(badges, &model.WorkActionBadge{Key: "coverage", Label: "Coverage", Tone: "info", Detail: optionalString(row.SourceCoverageState)})
	}
	return badges
}

func impactKindLabel(value workblockerimpact.ImpactKind) string {
	switch value {
	case workblockerimpact.ImpactKindDirectSubject:
		return "Direct subject"
	case workblockerimpact.ImpactKindWorkstream:
		return "Workstream impact"
	case workblockerimpact.ImpactKindDependencyChain:
		return "Dependency chain"
	default:
		return value.String()
	}
}

func impactKindTone(value workblockerimpact.ImpactKind) string {
	switch value {
	case workblockerimpact.ImpactKindDirectSubject:
		return "warning"
	case workblockerimpact.ImpactKindWorkstream, workblockerimpact.ImpactKindDependencyChain:
		return "info"
	default:
		return "neutral"
	}
}

func fmtInt(value int) string {
	return fmt.Sprintf("%d", value)
}

func blockerStateLabel(value workblocker.BlockerState) string {
	switch value {
	case workblocker.BlockerStateActive:
		return "Active blocker"
	case workblocker.BlockerStateValidating:
		return "Validating"
	case workblocker.BlockerStateResolved:
		return "Resolved"
	case workblocker.BlockerStateDismissed:
		return "Dismissed"
	default:
		return "Unknown"
	}
}

func blockerStateTone(value workblocker.BlockerState) string {
	switch value {
	case workblocker.BlockerStateActive:
		return "warning"
	case workblocker.BlockerStateValidating:
		return "info"
	case workblocker.BlockerStateResolved:
		return "success"
	case workblocker.BlockerStateDismissed:
		return "muted"
	default:
		return "neutral"
	}
}

func edgeKindLabel(value workdependencyedge.EdgeKind) string {
	switch value {
	case workdependencyedge.EdgeKindTicketPr:
		return "Ticket PR"
	case workdependencyedge.EdgeKindWorkstreamMember:
		return "Workstream member"
	case workdependencyedge.EdgeKindWorkstreamCluster:
		return "Workstream cluster"
	case workdependencyedge.EdgeKindBlockedBy:
		return "Blocked by"
	case workdependencyedge.EdgeKindNeedsAction:
		return "Needs action"
	case workdependencyedge.EdgeKindRelatedWork:
		return "Related work"
	default:
		return value.String()
	}
}

func relationshipAuthorityLabel(value workdependencyedge.RelationshipAuthority) string {
	switch value {
	case workdependencyedge.RelationshipAuthorityCanonicalMirror:
		return "Canonical mirror"
	case workdependencyedge.RelationshipAuthorityOperatingProjection:
		return "Operating projection"
	default:
		return value.String()
	}
}

func relationshipAuthorityTone(value workdependencyedge.RelationshipAuthority) string {
	switch value {
	case workdependencyedge.RelationshipAuthorityCanonicalMirror:
		return "success"
	case workdependencyedge.RelationshipAuthorityOperatingProjection:
		return "info"
	default:
		return "neutral"
	}
}

func decisionStateLabel(value string) string {
	switch value {
	case "product_action":
		return "Product action"
	case "validation_lead":
		return "Validation lead"
	case "source_repair":
		return "Source repair"
	case "closeout_review":
		return "Closeout review"
	case "source_resolved":
		return "Source resolved"
	case "model_or_rule_qa":
		return "Model QA"
	case "suppressed_signal":
		return "Suppressed"
	default:
		return value
	}
}

func decisionStateTone(value string) string {
	switch value {
	case "product_action":
		return "warning"
	case "validation_lead", "closeout_review", "model_or_rule_qa":
		return "info"
	case "source_resolved":
		return "success"
	case "source_repair":
		return "warning"
	case "suppressed_signal":
		return "muted"
	default:
		return "neutral"
	}
}

func severityLabel(value string) string {
	switch value {
	case "critical":
		return "Critical"
	case "high":
		return "High"
	case "medium":
		return "Medium"
	case "low":
		return "Low"
	default:
		return "Info"
	}
}

func severityTone(value string) string {
	switch value {
	case "critical", "high":
		return "warning"
	case "medium":
		return "info"
	default:
		return "neutral"
	}
}

func reviewerKindLabel(value string) string {
	switch value {
	case "human":
		return "Human review"
	case "imported":
		return "Imported review"
	case "system":
		return "System review"
	default:
		return value
	}
}

func truthLabel(value string) string {
	switch value {
	case "true_positive":
		return "True positive"
	case "false_positive":
		return "False positive"
	case "partial":
		return "Partial"
	default:
		return "Truth unknown"
	}
}

func truthTone(value string) string {
	switch value {
	case "true_positive":
		return "success"
	case "partial":
		return "warning"
	case "false_positive":
		return "neutral"
	default:
		return "warning"
	}
}

func actionabilityLabel(value string) string {
	switch value {
	case "actionable":
		return "Actionable"
	case "needs_owner":
		return "Needs owner"
	case "not_actionable":
		return "Not actionable"
	default:
		return "Actionability unknown"
	}
}

func actionabilityTone(value string) string {
	switch value {
	case "actionable":
		return "success"
	case "needs_owner":
		return "warning"
	case "not_actionable":
		return "neutral"
	default:
		return "warning"
	}
}

func freshnessLabel(value string) string {
	switch value {
	case "fresh":
		return "Fresh"
	case "partial":
		return "Partial"
	case "stale":
		return "Stale"
	default:
		return "Unknown"
	}
}
