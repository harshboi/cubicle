package graphql

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/sourceconnection"
	"cubicle/services/ontology-service/ent/sourcescope"
	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/workprogramevidenceneed"
	"cubicle/services/ontology-service/internal/graphql/model"

	entsql "entgo.io/ent/dialect/sql"
)

type workProgramSourceCoverageCounts struct {
	complete int
	limited  int
	unknown  int
}

type workProgramAbsenceClaimGate struct {
	allowed bool
	reason  string
}

func (r *queryResolver) workProgramSourceCoveragePacket(ctx context.Context, workstreamKey string, limit *int, evidenceLimit *int, sourceInstance *string) (*model.WorkProgramSourceCoveragePacket, error) {
	if r.EntClient == nil {
		return nil, fmt.Errorf("workProgramSourceCoveragePacket requires an Ent-backed ontology store")
	}
	workstreamKey = strings.TrimSpace(workstreamKey)
	if workstreamKey == "" {
		return nil, fmt.Errorf("workProgramSourceCoveragePacket requires workstreamKey")
	}
	rowLimit := boundedLimit(limit, 50, 200)
	evidenceRowLimit := boundedLimit(evidenceLimit, 50, 200)
	sourceFilter, err := r.aggregateSourceInstance(ctx, sourceInstance)
	if err != nil {
		return nil, err
	}
	workstreamFilter := &workstreamKey
	runGeneratedAt, err := r.latestWorkProgramAutomationReadinessRunGeneratedAt(ctx, sourceFilter, workstreamFilter)
	if err != nil {
		return nil, err
	}

	rows, err := r.workProgramItemRowsForSource(ctx, 1000, workstreamFilter, nil, nil, nil, sourceFilter)
	if err != nil {
		return nil, err
	}
	counts, affectedItems := workProgramSourceCoverageItemCounts(rows, rowLimit)

	syncIssueRows, syncIssueCount, authOrRateLimitIssueCount, err := r.sourceSyncIssueRowsForCoveragePacket(ctx, sourceFilter, &workstreamKey, rowLimit)
	if err != nil {
		return nil, err
	}
	evidenceNeeds, evidenceNeedCount, err := r.latestWorkProgramEvidenceNeedModelsAndCountForPredicates(ctx, workProgramEvidenceNeedFilters{
		sourceFilter:  sourceFilter,
		workstreamKey: workstreamFilter,
		generatedAt:   runGeneratedAt,
	}, evidenceRowLimit, workprogramevidenceneed.GateKeyIn("source_coverage", "source_authentication", "claim_provenance"))
	if err != nil {
		return nil, err
	}
	absenceBlockingEvidenceNeedCount := evidenceNeedCount

	coverageState := workProgramSourceCoveragePacketState(counts, syncIssueCount, authOrRateLimitIssueCount, len(rows))
	absenceClaimGate := workProgramSourceCoverageAbsenceClaimGate(counts, syncIssueCount, authOrRateLimitIssueCount, absenceBlockingEvidenceNeedCount, len(rows))
	absenceClaimsAllowed := absenceClaimGate.allowed
	recommendedFocus := workProgramSourceCoveragePacketFocus(evidenceNeeds, syncIssueRows, affectedItems, counts, authOrRateLimitIssueCount)
	return &model.WorkProgramSourceCoveragePacket{
		SourceInstance:            sourceFilter,
		GeneratedAt:               optionalTimePtr(runGeneratedAt),
		WorkstreamKey:             workstreamKey,
		CoverageState:             coverageState,
		CompleteItemCount:         counts.complete,
		LimitedItemCount:          counts.limited,
		UnknownItemCount:          counts.unknown,
		SourceSyncIssueCount:      syncIssueCount,
		AuthOrRateLimitIssueCount: authOrRateLimitIssueCount,
		EvidenceNeedCount:         evidenceNeedCount,
		AbsenceClaimsAllowed:      absenceClaimsAllowed,
		AbsenceClaimGateReason:    absenceClaimGate.reason,
		HumanReviewRequired:       !absenceClaimsAllowed || evidenceNeedCount > 0,
		AutomationSummary:         workProgramSourceCoveragePacketSummary(workstreamKey, coverageState, counts, syncIssueCount, authOrRateLimitIssueCount, evidenceNeedCount, absenceClaimsAllowed, absenceClaimGate.reason, recommendedFocus),
		RecommendedFocus:          recommendedFocus,
		AffectedItems:             workProgramItemModels(affectedItems),
		SourceSyncIssues:          sourceSyncIssueModels(syncIssueRows),
		EvidenceNeeds:             evidenceNeeds,
	}, nil
}

func workProgramSourceCoverageItemCounts(rows []*genent.WorkProgramItem, limit int) (workProgramSourceCoverageCounts, []*genent.WorkProgramItem) {
	var counts workProgramSourceCoverageCounts
	affected := make([]*genent.WorkProgramItem, 0, min(limit, len(rows)))
	for _, row := range rows {
		if row == nil {
			continue
		}
		switch {
		case workProgramItemCoverageUnknown(row):
			counts.unknown++
			if len(affected) < limit {
				affected = append(affected, row)
			}
		case workProgramItemCoverageLimited(row):
			counts.limited++
			if len(affected) < limit {
				affected = append(affected, row)
			}
		default:
			counts.complete++
		}
	}
	return counts, affected
}

func workProgramItemCoverageUnknown(row *genent.WorkProgramItem) bool {
	state := strings.ToLower(strings.TrimSpace(row.SourceCoverageState))
	if strings.Contains(state, "unknown") {
		return true
	}
	return state == "" && row.FreshnessState.String() == "unknown"
}

func workProgramSourceCoveragePacketState(counts workProgramSourceCoverageCounts, syncIssueCount int, authOrRateLimitIssueCount int, totalItemCount int) string {
	switch {
	case authOrRateLimitIssueCount > 0 || syncIssueCount > 0 || counts.limited > 0:
		return "limited"
	case counts.unknown > 0:
		return "unknown"
	case totalItemCount == 0:
		return "empty"
	default:
		return "complete"
	}
}

func workProgramSourceCoverageAbsenceClaimGate(counts workProgramSourceCoverageCounts, syncIssueCount int, authOrRateLimitIssueCount int, evidenceNeedCount int, totalItemCount int) workProgramAbsenceClaimGate {
	switch {
	case authOrRateLimitIssueCount > 0:
		return workProgramAbsenceClaimGate{reason: "source_auth_or_rate_limited"}
	case syncIssueCount > 0:
		return workProgramAbsenceClaimGate{reason: "source_sync_issues_present"}
	case counts.limited > 0:
		return workProgramAbsenceClaimGate{reason: "limited_source_coverage"}
	case counts.unknown > 0:
		return workProgramAbsenceClaimGate{reason: "unknown_source_coverage"}
	case evidenceNeedCount > 0:
		return workProgramAbsenceClaimGate{reason: "source_coverage_evidence_needed"}
	case totalItemCount == 0:
		return workProgramAbsenceClaimGate{reason: "no_program_items_in_scope"}
	default:
		return workProgramAbsenceClaimGate{allowed: true, reason: "complete_source_coverage"}
	}
}

func (r *queryResolver) sourceSyncIssueRowsForCoveragePacket(ctx context.Context, sourceFilter *string, workstreamKey *string, limit int) ([]*genent.SourceSyncIssue, int, int, error) {
	runID, err := r.latestSourceSyncRunIDForCoveragePacket(ctx, sourceFilter, workstreamKey)
	if err != nil {
		return nil, 0, 0, err
	}
	total, err := r.sourceSyncIssueCoverageQuery(sourceFilter, workstreamKey, runID).Count(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	authOrRateLimit, err := r.sourceSyncIssueCoverageQuery(sourceFilter, workstreamKey, runID).
		Where(sourceSyncIssueAuthOrRateLimitPredicate()).
		Count(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	rows, err := r.sourceSyncIssueCoverageQuery(sourceFilter, workstreamKey, runID).
		Order(
			sourcesyncissue.ByCreatedAt(entsql.OrderDesc()),
			sourcesyncissue.ByID(entsql.OrderDesc()),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	return rows, total, authOrRateLimit, nil
}

func (r *queryResolver) latestSourceSyncRunIDForCoveragePacket(ctx context.Context, sourceFilter *string, workstreamKey *string) (*int, error) {
	query := r.EntClient.SourceSyncRun.Query()
	query = applySourceSyncRunCoverageScopeFilter(query, sourceFilter, workstreamKey)
	query = query.Where(sourcesyncrun.CoverageModeIn(
		sourcesyncrun.CoverageModeExactScope,
		sourcesyncrun.CoverageModePartialScope,
	))
	row, err := query.
		Order(
			sourcesyncrun.ByStartedAt(entsql.OrderDesc()),
			sourcesyncrun.ByCreatedAt(entsql.OrderDesc()),
			sourcesyncrun.ByID(entsql.OrderDesc()),
		).
		First(ctx)
	if genent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row.ID, nil
}

func (r *queryResolver) sourceSyncIssueCoverageQuery(sourceFilter *string, workstreamKey *string, runID *int) *genent.SourceSyncIssueQuery {
	query := r.EntClient.SourceSyncIssue.Query()
	query = applySourceSyncIssueCoverageScopeFilter(query, sourceFilter, workstreamKey)
	if runID != nil {
		query = query.Where(sourcesyncissue.SourceSyncRunIDEQ(*runID))
	}
	return query
}

func applySourceSyncRunCoverageScopeFilter(query *genent.SourceSyncRunQuery, sourceFilter *string, workstreamKey *string) *genent.SourceSyncRunQuery {
	scopePredicates := sourceCoverageScopePredicates(sourceFilter, workstreamKey)
	if len(scopePredicates) == 0 {
		return query
	}
	return query.Where(sourcesyncrun.HasScopeWith(scopePredicates...))
}

func applySourceSyncIssueCoverageScopeFilter(query *genent.SourceSyncIssueQuery, sourceFilter *string, workstreamKey *string) *genent.SourceSyncIssueQuery {
	scopePredicates := sourceCoverageScopePredicates(sourceFilter, workstreamKey)
	if len(scopePredicates) == 0 {
		return query
	}
	return query.Where(sourcesyncissue.HasScopeWith(scopePredicates...))
}

func sourceCoverageScopePredicates(sourceFilter *string, workstreamKey *string) []predicate.SourceScope {
	var sourcePredicate predicate.SourceScope
	if sourceFilter != nil && strings.TrimSpace(*sourceFilter) != "" {
		source := strings.TrimSpace(*sourceFilter)
		sourcePredicate = sourcescope.HasConnectionWith(sourceconnection.SourceInstanceEQ(source))
	}
	var workstreamPredicate predicate.SourceScope
	if workstreamKey != nil && strings.TrimSpace(*workstreamKey) != "" {
		workstreamPredicate = sourceCoverageWorkstreamScopePredicate(*workstreamKey)
	}
	if sourcePredicate != nil && workstreamPredicate != nil {
		source := strings.TrimSpace(*sourceFilter)
		return []predicate.SourceScope{
			sourcePredicate,
			sourcescope.Or(
				workstreamPredicate,
				sourcescope.ScopeKeyEQ(source),
			),
		}
	}
	if sourcePredicate != nil {
		return []predicate.SourceScope{sourcePredicate}
	}
	if workstreamPredicate != nil {
		return []predicate.SourceScope{workstreamPredicate}
	}
	return nil
}

func sourceCoverageWorkstreamScopePredicate(workstreamKey string) predicate.SourceScope {
	filterKeys := workProgramWorkstreamFilterKeys(workstreamKey)
	scopePredicates := []predicate.SourceScope{
		sourcescope.ScopeKeyIn(filterKeys...),
	}
	for _, key := range filterKeys {
		key = strings.TrimSpace(strings.TrimPrefix(key, "workstream:"))
		if key == "" || strings.Contains(key, "/") {
			continue
		}
		scopePredicates = append(scopePredicates, sourcescope.And(
			sourcescope.ScopeKindEQ("repository"),
			sourcescope.ScopeKeyHasSuffix("/"+key),
		))
	}
	return sourcescope.Or(scopePredicates...)
}

func sourceSyncIssueAuthOrRateLimitPredicate() func(*entsql.Selector) {
	return sourcesyncissue.Or(
		sourcesyncissue.IssueCodeContainsFold("forbidden"),
		sourcesyncissue.IssueCodeContainsFold("rate_limited"),
		sourcesyncissue.IssueCodeContainsFold("rate limit"),
		sourcesyncissue.IssueCodeContainsFold("403"),
		sourcesyncissue.IssueCodeContainsFold("429"),
		sourcesyncissue.MessageContainsFold("forbidden"),
		sourcesyncissue.MessageContainsFold("rate limit"),
		sourcesyncissue.MessageContainsFold("403"),
		sourcesyncissue.MessageContainsFold("429"),
	)
}

func sourceSyncIssueModels(rows []*genent.SourceSyncIssue) []*model.SourceSyncIssue {
	out := make([]*model.SourceSyncIssue, 0, len(rows))
	for _, row := range rows {
		out = append(out, sourceSyncIssueModel(row))
	}
	return out
}

func sourceSyncIssueModel(row *genent.SourceSyncIssue) *model.SourceSyncIssue {
	return &model.SourceSyncIssue{
		Key:            "source-sync-issue:" + strconv.Itoa(row.ID),
		SourceInstance: optionalString(row.SourceInstance),
		SourceSystem:   optionalString(row.SourceSystem),
		ExternalKind:   optionalString(row.ExternalKind),
		ExternalID:     optionalString(row.ExternalID),
		IssueCode:      row.IssueCode,
		Severity:       row.Severity.String(),
		Message:        optionalString(sourceSyncIssueSafeMessage(row)),
		SourceURL:      nil,
	}
}

func sourceSyncIssueSafeMessage(row *genent.SourceSyncIssue) string {
	if strings.TrimSpace(row.IssueCode) == "" {
		return "Source coverage issue requires review before absence claims."
	}
	return "Source coverage issue " + strings.TrimSpace(row.IssueCode) + " requires review before absence claims."
}

func workProgramItemModels(rows []*genent.WorkProgramItem) []*model.WorkProgramItem {
	out := make([]*model.WorkProgramItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, workProgramItemModel(row))
	}
	return out
}

func workProgramSourceCoveragePacketFocus(evidenceNeeds []*model.WorkProgramAutomationEvidenceNeed, syncIssues []*genent.SourceSyncIssue, affectedItems []*genent.WorkProgramItem, counts workProgramSourceCoverageCounts, authOrRateLimitIssueCount int) *string {
	for _, need := range evidenceNeeds {
		if need == nil {
			continue
		}
		if value := optionalTrimmedPointer(need.RecommendedAction); value != nil {
			return value
		}
		if value := optionalTrimmedPointer(need.NextExecutionStep); value != nil {
			return value
		}
	}
	if authOrRateLimitIssueCount > 0 {
		return optionalTrimmedPointer("Repair source authentication or wait out rate limiting before making absence or completion claims.")
	}
	for _, issue := range syncIssues {
		if issue == nil {
			continue
		}
		if value := optionalTrimmedPointer(sourceSyncIssueSafeMessage(issue)); value != nil {
			return value
		}
		if value := optionalTrimmedPointer(issue.IssueCode); value != nil {
			return value
		}
	}
	for _, item := range affectedItems {
		if item == nil {
			continue
		}
		if value := optionalTrimmedPointer(item.NextAction); value != nil {
			return value
		}
		if value := optionalTrimmedPointer(item.DecisionNeeded); value != nil {
			return value
		}
	}
	if counts.unknown > 0 {
		return optionalTrimmedPointer("Backfill source coverage state for typed program items before allowing absence claims.")
	}
	if counts.limited > 0 {
		return optionalTrimmedPointer("Treat coverage-limited program items as review leads until source coverage is complete.")
	}
	return nil
}

func workProgramSourceCoveragePacketSummary(workstreamKey string, coverageState string, counts workProgramSourceCoverageCounts, syncIssueCount int, authOrRateLimitIssueCount int, evidenceNeedCount int, absenceClaimsAllowed bool, absenceClaimGateReason string, recommendedFocus *string) string {
	claims := "absence claims allowed"
	if !absenceClaimsAllowed {
		claims = "absence claims disabled"
	}
	summary := fmt.Sprintf("%s source coverage is %s; %d complete item(s), %d limited item(s), %d unknown item(s), %d sync issue(s), %d auth/rate-limit issue(s), and %d source-coverage evidence need(s). %s; absence claim gate reason: %s.", workstreamKey, coverageState, counts.complete, counts.limited, counts.unknown, syncIssueCount, authOrRateLimitIssueCount, evidenceNeedCount, claims, absenceClaimGateReason)
	if recommendedFocus == nil || strings.TrimSpace(*recommendedFocus) == "" {
		return summary
	}
	return summary + " " + strings.TrimSpace(*recommendedFocus)
}
