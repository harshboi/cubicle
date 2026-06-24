package graphql

import (
	"context"
	"fmt"
	"strings"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/document"
	"cubicle/services/ontology-service/ent/message"
	"cubicle/services/ontology-service/ent/predicate"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphcontext"
	"cubicle/services/ontology-service/internal/ontology"
)

type boundedGraphSourceIdentity struct {
	objectType         domain.ObjectType
	sourceSystem       string
	sourceInstance     string
	externalKind       string
	externalID         string
	sourceScopeStateID int
	lastConfirmedAt    time.Time
}

func defaultBoundedGraphCoveragePolicy() graphcontext.CoveragePolicy {
	return graphcontext.CoveragePolicy{
		CoverageState:          "sparse",
		AbsenceClaimsAllowed:   false,
		AbsenceClaimGateReason: "source_coverage_gate",
		Summary:                "Coverage is server-owned and remains sparse until a source-scope coverage provider proves completeness.",
	}
}

func (r *queryResolver) boundedGraphCoveragePolicy(ctx context.Context, start domain.ObjectRef) graphcontext.CoveragePolicy {
	policy := defaultBoundedGraphCoveragePolicy()
	if r.EntClient == nil {
		return policy
	}

	identity, ok := r.boundedGraphStartSourceIdentity(ctx, start)
	if !ok || identity.sourceSystem == "" || identity.sourceInstance == "" || identity.externalID == "" {
		return policy
	}
	policy = seedObservationCoveragePolicy(policy, identity)
	sourceScopeID := 0
	if scopePolicy, scopeID, ok := r.boundedGraphSourceScopeCoveragePolicy(ctx, identity); ok {
		policy = scopePolicy
		sourceScopeID = scopeID
	}

	predicates := boundedGraphSourceIssuePredicates(identity, sourceScopeID)
	if len(predicates) == 0 {
		return policy
	}
	issueCount, err := r.EntClient.SourceSyncIssue.Query().
		Where(predicates...).
		Count(ctx)
	if err != nil {
		policy.CoverageState = "limited"
		policy.AbsenceClaimsAllowed = false
		policy.AbsenceClaimGateReason = "source_sync_issue_check_failed"
		policy.Summary = "Coverage is limited because source sync issues could not be checked; absence claims remain disabled."
		return policy
	}
	if issueCount == 0 {
		return policy
	}
	authPredicates := append([]predicate.SourceSyncIssue{}, predicates...)
	authPredicates = append(authPredicates, sourceSyncIssueAuthOrRateLimitPredicate())
	authOrRateLimitIssueCount, err := r.EntClient.SourceSyncIssue.Query().
		Where(authPredicates...).
		Count(ctx)
	if err != nil {
		authOrRateLimitIssueCount = 0
	}

	policy.CoverageState = "limited"
	policy.AbsenceClaimsAllowed = false
	if authOrRateLimitIssueCount > 0 {
		policy.AbsenceClaimGateReason = "source_auth_or_rate_limit"
	} else {
		policy.AbsenceClaimGateReason = "source_sync_issue"
	}
	policy.Summary = fmt.Sprintf(
		"Coverage is limited for %s %s: %d source sync issue(s), %d auth/rate-limit issue(s). Raw sync issue bodies and source URLs are coverage evidence only, not prompt facts.",
		start.ObjectType,
		start.Key,
		issueCount,
		authOrRateLimitIssueCount,
	)
	return policy
}

func seedObservationCoveragePolicy(policy graphcontext.CoveragePolicy, identity boundedGraphSourceIdentity) graphcontext.CoveragePolicy {
	policy.SourceSystem = identity.sourceSystem
	policy.SourceInstance = identity.sourceInstance
	if !identity.lastConfirmedAt.IsZero() {
		observedAt := identity.lastConfirmedAt.UTC().Format(timeRFC3339)
		policy.CoverageWindowStart = observedAt
		policy.CoverageWindowEnd = observedAt
		policy.Summary = fmt.Sprintf(
			"Sparse seed observation for %s/%s at %s; absence claims remain disabled until exact source-scope coverage proves relation, source, time, and principal completeness.",
			identity.sourceSystem,
			identity.sourceInstance,
			observedAt,
		)
	}
	return policy
}

func boundedGraphSourceIssuePredicates(identity boundedGraphSourceIdentity, sourceScopeID int) []predicate.SourceSyncIssue {
	sourceObjectPredicates := boundedGraphSourceObjectIssuePredicates(identity)
	if sourceScopeID > 0 {
		if len(sourceObjectPredicates) > 0 {
			return []predicate.SourceSyncIssue{
				sourcesyncissue.Or(
					sourcesyncissue.SourceScopeIDEQ(sourceScopeID),
					sourcesyncissue.And(sourceObjectPredicates...),
				),
			}
		}
		return []predicate.SourceSyncIssue{sourcesyncissue.SourceScopeIDEQ(sourceScopeID)}
	}
	return sourceObjectPredicates
}

func boundedGraphSourceObjectIssuePredicates(identity boundedGraphSourceIdentity) []predicate.SourceSyncIssue {
	if identity.sourceSystem == "" || identity.sourceInstance == "" || identity.externalID == "" {
		return nil
	}
	predicates := []predicate.SourceSyncIssue{
		sourcesyncissue.SourceSystemEQ(identity.sourceSystem),
		sourcesyncissue.SourceInstanceEQ(identity.sourceInstance),
		sourcesyncissue.ExternalIDEQ(identity.externalID),
	}
	if externalKinds := boundedGraphSourceIssueExternalKinds(identity); len(externalKinds) > 0 {
		predicates = append(predicates, sourcesyncissue.ExternalKindIn(externalKinds...))
	}
	return predicates
}

func boundedGraphSourceIssueExternalKinds(identity boundedGraphSourceIdentity) []string {
	switch identity.objectType {
	case ontology.ObjectTicket:
		if identity.sourceSystem == "jira" {
			return []string{"jira_issue", "jira_remote_links", "jira_correlation_issue", "jira_correlation_remote_links"}
		}
	case ontology.ObjectPullRequest:
		if identity.sourceSystem == "github" {
			return []string{
				"github_pull_request",
				"github_pull_request_files",
				"github_issue_comments",
				"github_pull_request_review_comments",
				"github_pull_request_reviews",
				"github_pull_request_commits",
			}
		}
	}
	if identity.externalKind == "" {
		return nil
	}
	return []string{identity.externalKind}
}

func (r *queryResolver) boundedGraphSourceScopeCoveragePolicy(ctx context.Context, identity boundedGraphSourceIdentity) (graphcontext.CoveragePolicy, int, bool) {
	if identity.sourceScopeStateID <= 0 {
		return graphcontext.CoveragePolicy{}, 0, false
	}
	state, err := r.EntClient.SourceScopeState.Get(ctx, identity.sourceScopeStateID)
	if err != nil {
		return graphcontext.CoveragePolicy{}, 0, false
	}
	scope, err := state.QueryScope().Only(ctx)
	if err != nil {
		return graphcontext.CoveragePolicy{}, state.SourceScopeID, false
	}
	connection, err := scope.QueryConnection().Only(ctx)
	if err != nil {
		return graphcontext.CoveragePolicy{}, state.SourceScopeID, false
	}

	sourceSystem := strings.TrimSpace(connection.SourceSystem)
	if sourceSystem == "" {
		sourceSystem = identity.sourceSystem
	}
	sourceInstance := strings.TrimSpace(connection.SourceInstance)
	if sourceInstance == "" {
		sourceInstance = identity.sourceInstance
	}
	policy := graphcontext.CoveragePolicy{
		CoverageState:          "limited",
		AbsenceClaimsAllowed:   false,
		AbsenceClaimGateReason: "source_scope_not_exact",
		SourceSystem:           sourceSystem,
		SourceInstance:         sourceInstance,
		Summary: fmt.Sprintf(
			"Coverage is limited for source scope %s %s: freshness=%s coverage_mode=%s.",
			strings.TrimSpace(scope.ScopeKind),
			strings.TrimSpace(scope.ScopeKey),
			state.FreshnessState,
			state.CoverageMode,
		),
	}
	if identity.sourceSystem != "" && sourceSystem != identity.sourceSystem || identity.sourceInstance != "" && sourceInstance != identity.sourceInstance {
		policy.AbsenceClaimGateReason = "source_scope_identity_mismatch"
		policy.Summary = fmt.Sprintf(
			"Coverage is limited because product row source %s/%s does not match source scope connection %s/%s.",
			identity.sourceSystem,
			identity.sourceInstance,
			sourceSystem,
			sourceInstance,
		)
		policy = boundedGraphLimitedStateObservationWindow(policy, state)
		return policy, state.SourceScopeID, true
	}
	if state.FreshnessState != sourcescopestate.FreshnessStateFresh {
		policy.AbsenceClaimGateReason = "source_scope_not_fresh"
		policy = boundedGraphLimitedStateObservationWindow(policy, state)
		return policy, state.SourceScopeID, true
	}
	if state.CoverageMode != sourcescopestate.CoverageModeExactScope {
		policy = boundedGraphLimitedStateObservationWindow(policy, state)
		return policy, state.SourceScopeID, true
	}

	policy.CoverageState = "complete"
	declaredAssociationTypes := boundedGraphScopeAbsenceClaimAssociationTypes(scope.CrawlPolicy)
	policy.AbsenceClaimAssociationTypes = boundedGraphSourceSupportedAbsenceClaimAssociationTypes(identity, sourceSystem, declaredAssociationTypes)
	policy.AbsenceClaimGateReason = "source_scope_relation_coverage_required"
	policy.Summary = fmt.Sprintf(
		"Coverage is complete for source scope %s %s with exact source coverage. Absence claims still require explicit relationship-path coverage.",
		strings.TrimSpace(scope.ScopeKind),
		strings.TrimSpace(scope.ScopeKey),
	)
	if len(declaredAssociationTypes) > 0 && len(policy.AbsenceClaimAssociationTypes) == 0 {
		policy.AbsenceClaimGateReason = "source_scope_relation_coverage_unsupported"
		policy.Summary = fmt.Sprintf(
			"Coverage is complete for source scope %s %s, but the declared relationship coverage is not supported for %s rows from %s.",
			strings.TrimSpace(scope.ScopeKind),
			strings.TrimSpace(scope.ScopeKey),
			identity.objectType,
			sourceSystem,
		)
	}
	if len(policy.AbsenceClaimAssociationTypes) > 0 {
		if boundedGraphCoverageCompleteForPrincipal(ctx) {
			policy.AbsenceClaimsAllowed = true
			policy.AbsenceClaimGateReason = "complete_relation_path_coverage"
		} else {
			policy.AbsenceClaimGateReason = "principal_coverage_required"
			policy.Summary = fmt.Sprintf(
				"Coverage is complete for source scope %s %s, but absence claims require principal-aware source coverage.",
				strings.TrimSpace(scope.ScopeKind),
				strings.TrimSpace(scope.ScopeKey),
			)
		}
	}
	if run, ok := boundedGraphLatestSuccessfulRun(ctx, state); ok {
		if start, end, ok := boundedGraphRunCoverageWindow(run); ok {
			policy.CoverageWindowStart = start
			policy.CoverageWindowEnd = end
		}
	}
	return policy, state.SourceScopeID, true
}

func boundedGraphLimitedStateObservationWindow(policy graphcontext.CoveragePolicy, state *genent.SourceScopeState) graphcontext.CoveragePolicy {
	if policy.CoverageWindowStart != "" && policy.CoverageWindowEnd != "" {
		return policy
	}
	observedAt := state.LastAttemptedAt
	if observedAt.IsZero() {
		observedAt = state.LastSuccessfulAt
	}
	if observedAt.IsZero() {
		return policy
	}
	point := observedAt.UTC().Format(timeRFC3339)
	policy.CoverageWindowStart = point
	policy.CoverageWindowEnd = point
	return policy
}

func boundedGraphCoverageCompleteForPrincipal(ctx context.Context) bool {
	access, ok := BoundedGraphPrincipalAccessFromContext(ctx)
	return ok && access.CoverageCompleteForPrincipal
}

func boundedGraphLatestSuccessfulRun(ctx context.Context, state *genent.SourceScopeState) (*genent.SourceSyncRun, bool) {
	if state.LastSuccessfulSyncRunID == 0 {
		return nil, false
	}
	run, err := state.QueryLastSuccessfulSyncRun().Only(ctx)
	if err != nil {
		return nil, false
	}
	return run, true
}

func boundedGraphRunCoverageWindow(run *genent.SourceSyncRun) (string, string, bool) {
	if run.Status != sourcesyncrun.StatusComplete || run.CoverageStartAt.IsZero() || run.CoverageEndAt.IsZero() {
		return "", "", false
	}
	return run.CoverageStartAt.UTC().Format(timeRFC3339), run.CoverageEndAt.UTC().Format(timeRFC3339), true
}

const timeRFC3339 = "2006-01-02T15:04:05Z07:00"

func boundedGraphScopeAbsenceClaimAssociationTypes(crawlPolicy string) []string {
	var values []string
	for _, token := range strings.FieldsFunc(crawlPolicy, func(r rune) bool {
		return r == ';' || r == '\n' || r == '\t' || r == ' '
	}) {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		for _, prefix := range []string{
			"bounded_graph_absence=",
			"absence_claim_association_types=",
			"association_types=",
		} {
			if rest, ok := strings.CutPrefix(token, prefix); ok {
				values = append(values, splitBoundedGraphAssociationCoverageTypes(rest)...)
			}
		}
	}
	return values
}

func boundedGraphSourceSupportedAbsenceClaimAssociationTypes(identity boundedGraphSourceIdentity, sourceSystem string, values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if boundedGraphSourceSupportsAbsenceClaimAssociation(identity, sourceSystem, value) {
			out = append(out, value)
		}
	}
	return out
}

func boundedGraphSourceSupportsAbsenceClaimAssociation(identity boundedGraphSourceIdentity, sourceSystem string, associationType string) bool {
	associationType = strings.TrimSpace(associationType)
	switch identity.objectType {
	case ontology.ObjectTicket:
		return sourceSystem == "jira" && stringInSet(associationType, string(ontology.AssocImplementedBy), string(ontology.AssocDocuments), string(ontology.AssocDiscussedIn))
	case ontology.ObjectPullRequest:
		return sourceSystem == "github" && stringInSet(associationType, "author", "creator", "reviewer", "approver", "commenter", "requested_reviewer")
	case ontology.ObjectDocument:
		return sourceSystem == "docs" && stringInSet(associationType, "links_to")
	case ontology.ObjectMessage:
		return sourceSystem == "chat" && stringInSet(associationType, string(ontology.AssocDiscussedIn))
	default:
		return false
	}
}

func stringInSet(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func splitBoundedGraphAssociationCoverageTypes(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}

func (r *queryResolver) boundedGraphStartSourceIdentity(ctx context.Context, start domain.ObjectRef) (boundedGraphSourceIdentity, bool) {
	switch start.ObjectType {
	case ontology.ObjectTicket:
		row, err := r.EntClient.Ticket.Query().Where(ticket.KeyEQ(start.Key)).Only(ctx)
		if err != nil {
			return boundedGraphSourceIdentity{}, false
		}
		return boundedGraphSourceIdentity{
			objectType:         ontology.ObjectTicket,
			sourceSystem:       strings.TrimSpace(row.SourceSystem),
			sourceInstance:     strings.TrimSpace(row.SourceInstance),
			externalKind:       strings.TrimSpace(row.ExternalKind),
			externalID:         strings.TrimSpace(row.ExternalID),
			sourceScopeStateID: row.SourceScopeStateID,
			lastConfirmedAt:    row.LastConfirmedAt,
		}, true
	case ontology.ObjectPullRequest:
		row, err := r.EntClient.PullRequest.Query().Where(pullrequest.KeyEQ(start.Key)).Only(ctx)
		if err != nil {
			return boundedGraphSourceIdentity{}, false
		}
		return boundedGraphSourceIdentity{
			objectType:         ontology.ObjectPullRequest,
			sourceSystem:       strings.TrimSpace(row.SourceSystem),
			sourceInstance:     strings.TrimSpace(row.SourceInstance),
			externalKind:       strings.TrimSpace(row.ExternalKind),
			externalID:         strings.TrimSpace(row.ExternalID),
			sourceScopeStateID: row.SourceScopeStateID,
			lastConfirmedAt:    row.LastConfirmedAt,
		}, true
	case ontology.ObjectDocument:
		row, err := r.EntClient.Document.Query().Where(document.KeyEQ(start.Key)).Only(ctx)
		if err != nil {
			return boundedGraphSourceIdentity{}, false
		}
		return boundedGraphSourceIdentity{
			objectType:         ontology.ObjectDocument,
			sourceSystem:       strings.TrimSpace(row.SourceSystem),
			sourceInstance:     strings.TrimSpace(row.SourceInstance),
			externalKind:       strings.TrimSpace(row.ExternalKind),
			externalID:         strings.TrimSpace(row.ExternalID),
			sourceScopeStateID: row.SourceScopeStateID,
			lastConfirmedAt:    row.LastConfirmedAt,
		}, true
	case ontology.ObjectMessage:
		row, err := r.EntClient.Message.Query().Where(message.KeyEQ(start.Key)).Only(ctx)
		if err != nil {
			return boundedGraphSourceIdentity{}, false
		}
		return boundedGraphSourceIdentity{
			objectType:         ontology.ObjectMessage,
			sourceSystem:       strings.TrimSpace(row.SourceSystem),
			sourceInstance:     strings.TrimSpace(row.SourceInstance),
			externalKind:       strings.TrimSpace(row.ExternalKind),
			externalID:         strings.TrimSpace(row.ExternalID),
			sourceScopeStateID: row.SourceScopeStateID,
			lastConfirmedAt:    row.LastConfirmedAt,
		}, true
	default:
		return boundedGraphSourceIdentity{}, false
	}
}
