package graphql

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workdependencyedge"
	"cubicle/services/ontology-service/ent/workdependencyendpoint"
	"cubicle/services/ontology-service/internal/entstore"
)

func TestWorkDependencyEdgesRequireLinkedClaimReady(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	now := time.Date(2026, 6, 22, 10, 15, 0, 0, time.UTC)

	validatingAction, err := store.Client().WorkAction.Create().
		SetKey("work-action:dependency:validating").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("repo/example#1").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("dependency:validating-action").
		SetRankScore(90).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create validating action: %v", err)
	}
	validatingBlocker, err := store.Client().WorkBlocker.Create().
		SetKey("work-blocker:dependency:validating").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityHigh).
		SetSubjectKind(workblocker.SubjectKindUnknown).
		SetSubjectKey("repo/example#1").
		SetDecisionState(workblocker.DecisionStateValidationLead).
		SetReviewState(workblocker.ReviewStateNeedsMoreData).
		SetTruthLabel(workblocker.TruthLabelPartial).
		SetActionabilityLabel(workblocker.ActionabilityLabelNeedsOwner).
		SetLabelQuality(workblocker.LabelQualityCandidate).
		SetTitle("Generated blocker still needs validation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker").
		SetExternalID("dependency:validating-blocker").
		SetRankScore(95).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create validating blocker: %v", err)
	}
	_, err = store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:validating-blocked-by").
		SetEdgeKind(workdependencyedge.EdgeKindBlockedBy).
		SetFromKind(workdependencyedge.FromKindPullRequest).
		SetFromKey("repo/example#1").
		SetToKind(workdependencyedge.ToKindBlocker).
		SetToKey(validatingBlocker.Key).
		SetWorkBlocker(validatingBlocker).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("dependency:validating-blocked-by").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetConfidence(0.95).
		SetRankScore(95).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create validating blocked-by edge: %v", err)
	}
	_, err = store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:validating-needs-action").
		SetEdgeKind(workdependencyedge.EdgeKindNeedsAction).
		SetFromKind(workdependencyedge.FromKindPullRequest).
		SetFromKey("repo/example#1").
		SetToKind(workdependencyedge.ToKindAction).
		SetToKey(validatingAction.Key).
		SetWorkAction(validatingAction).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("dependency:validating-needs-action").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetConfidence(0.95).
		SetRankScore(90).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create validating needs-action edge: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	rows, err := resolver.WorkDependencyEdges(ctx, &limit, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("work dependency edges: %v", err)
	}
	byKey := map[string]*modelWorkDependencyEdgeForTest{}
	for _, row := range rows {
		byKey[row.Key] = &modelWorkDependencyEdgeForTest{
			relationshipClaimAllowed: row.RelationshipClaimAllowed,
			claimUse:                 row.ClaimUse,
			claimGateReason:          row.ClaimGateReason,
		}
	}
	assertDependencyGateForTest(t, byKey, "work-dependency-edge:validating-blocked-by", false, "blocked_by_validation", "linked_blocker_claim_not_allowed")
	assertDependencyGateForTest(t, byKey, "work-dependency-edge:validating-needs-action", false, "needs_action_validation", "linked_action_claim_not_allowed")
}

func TestWorkDependencyEdgesKeepMeasurementBackedEdgesAsTopologyContext(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	now := time.Date(2026, 6, 22, 10, 45, 0, 0, time.UTC)

	productAction, err := store.Client().WorkAction.Create().
		SetKey("work-action:dependency:product").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecisionReason("measurement-backed owner follow-up").
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("repo/example#2").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("dependency:product-action").
		SetRankScore(99).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create product action: %v", err)
	}
	productBlocker, err := store.Client().WorkBlocker.Create().
		SetKey("work-blocker:dependency:product").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityHigh).
		SetSubjectKind(workblocker.SubjectKindUnknown).
		SetSubjectKey("repo/example#2").
		SetDecisionState(workblocker.DecisionStateProductAction).
		SetReviewState(workblocker.ReviewStateAccepted).
		SetTruthLabel(workblocker.TruthLabelTruePositive).
		SetActionabilityLabel(workblocker.ActionabilityLabelActionable).
		SetLabelQuality(workblocker.LabelQualityGold).
		SetMeasurementEligible(true).
		SetTitle("Accepted blocker claim").
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_blocker").
		SetExternalID("dependency:product-blocker").
		SetFreshnessState(workblocker.FreshnessStateFresh).
		SetRankScore(98).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create product blocker: %v", err)
	}
	_, err = store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:product-blocked-by").
		SetEdgeKind(workdependencyedge.EdgeKindBlockedBy).
		SetFromKind(workdependencyedge.FromKindPullRequest).
		SetFromKey("repo/example#2").
		SetToKind(workdependencyedge.ToKindBlocker).
		SetToKey(productBlocker.Key).
		SetWorkBlocker(productBlocker).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("dependency:product-blocked-by").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetConfidence(0.95).
		SetRankScore(98).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create product blocked-by edge: %v", err)
	}
	_, err = store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:product-needs-action").
		SetEdgeKind(workdependencyedge.EdgeKindNeedsAction).
		SetFromKind(workdependencyedge.FromKindPullRequest).
		SetFromKey("repo/example#2").
		SetToKind(workdependencyedge.ToKindAction).
		SetToKey(productAction.Key).
		SetWorkAction(productAction).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("dependency:product-needs-action").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetConfidence(0.95).
		SetRankScore(99).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create product needs-action edge: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	rows, err := resolver.WorkDependencyEdges(ctx, &limit, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("work dependency edges: %v", err)
	}
	byKey := map[string]*modelWorkDependencyEdgeForTest{}
	for _, row := range rows {
		byKey[row.Key] = &modelWorkDependencyEdgeForTest{
			relationshipClaimAllowed: row.RelationshipClaimAllowed,
			claimUse:                 row.ClaimUse,
			claimGateReason:          row.ClaimGateReason,
		}
	}
	assertDependencyGateForTest(t, byKey, "work-dependency-edge:product-blocked-by", false, "blocked_by_validation", "derived_dependency_edge_not_product_claim")
	assertDependencyGateForTest(t, byKey, "work-dependency-edge:product-needs-action", false, "needs_action_validation", "derived_dependency_edge_not_product_claim")
}

func TestWorkDependencyEdgesExposeTypedEndpoints(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	now := time.Date(2026, 6, 22, 11, 15, 0, 0, time.UTC)

	action, err := store.Client().WorkAction.Create().
		SetKey("work-action:dependency:endpoint-product").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("component:autoscaler").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_action").
		SetExternalID("dependency:endpoint-product-action").
		SetRankScore(88).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	edge, err := store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:endpoint-component-needs-action").
		SetEdgeKind(workdependencyedge.EdgeKindNeedsAction).
		SetFromKind(workdependencyedge.FromKindComponent).
		SetFromKey("component:autoscaler").
		SetToKind(workdependencyedge.ToKindAction).
		SetToKey(action.Key).
		SetWorkAction(action).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("dependency:endpoint-component-needs-action").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetConfidence(0.91).
		SetRankScore(91).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create dependency edge: %v", err)
	}
	_, err = store.Client().WorkDependencyEndpoint.Create().
		SetKey("work-dependency-endpoint:endpoint-component-needs-action:from").
		SetWorkDependencyEdge(edge).
		SetEndpointRole(workdependencyendpoint.EndpointRoleFrom).
		SetNodeKind(workdependencyendpoint.NodeKindComponent).
		SetNodeKey("component:autoscaler").
		SetResolutionState(workdependencyendpoint.ResolutionStateKeyOnly).
		SetResolutionReason("component endpoints have no typed table yet").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_endpoint").
		SetExternalID("dependency:endpoint-component-needs-action:from").
		SetFreshnessState(workdependencyendpoint.FreshnessStateFresh).
		SetVisibility(workdependencyendpoint.VisibilityUnknown).
		SetConfidence(0.91).
		SetRankScore(91).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create from endpoint: %v", err)
	}
	_, err = store.Client().WorkDependencyEndpoint.Create().
		SetKey("work-dependency-endpoint:endpoint-component-needs-action:to").
		SetWorkDependencyEdge(edge).
		SetEndpointRole(workdependencyendpoint.EndpointRoleTo).
		SetNodeKind(workdependencyendpoint.NodeKindAction).
		SetNodeKey(action.Key).
		SetResolutionState(workdependencyendpoint.ResolutionStateResolved).
		SetResolutionReason("resolved to typed action row").
		SetWorkAction(action).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_endpoint").
		SetExternalID("dependency:endpoint-component-needs-action:to").
		SetFreshnessState(workdependencyendpoint.FreshnessStateFresh).
		SetVisibility(workdependencyendpoint.VisibilityUnknown).
		SetConfidence(0.91).
		SetRankScore(91).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create to endpoint: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	rows, err := resolver.WorkDependencyEdges(ctx, &limit, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("work dependency edges: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("work dependency edge rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if len(row.Endpoints) != 2 {
		t.Fatalf("dependency endpoint rows = %d, want 2", len(row.Endpoints))
	}
	if row.FromEndpoint == nil {
		t.Fatalf("from endpoint was not exposed")
	}
	if row.FromEndpoint.EndpointRole != "from" ||
		row.FromEndpoint.NodeKind != "component" ||
		row.FromEndpoint.NodeKey != "component:autoscaler" ||
		row.FromEndpoint.ResolutionState != "key_only" {
		t.Fatalf("from endpoint = %#v, want key-only component endpoint", row.FromEndpoint)
	}
	if row.ToEndpoint == nil {
		t.Fatalf("to endpoint was not exposed")
	}
	if row.ToEndpoint.EndpointRole != "to" ||
		row.ToEndpoint.NodeKind != "action" ||
		row.ToEndpoint.NodeKey != action.Key ||
		row.ToEndpoint.ResolutionState != "resolved" {
		t.Fatalf("to endpoint = %#v, want resolved action endpoint for %q", row.ToEndpoint, action.Key)
	}
}

func TestWorkDependencyEdgesExposeRelationshipAuthority(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "fixture-source"
	now := time.Date(2026, 6, 22, 11, 45, 0, 0, time.UTC)
	ticketRow, err := store.Client().Ticket.Create().
		SetKey("ticket:FLINK-12345").
		SetTitle("Autoscaler work").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance(source).
		SetExternalKind("jira_issue").
		SetExternalID("FLINK-12345").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityUnknown).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	prRow, err := store.Client().PullRequest.Create().
		SetKey("pull-request:apache/flink-kubernetes-operator#42").
		SetRepository("apache/flink-kubernetes-operator").
		SetNumber(42).
		SetTitle("Implement autoscaler work").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance(source).
		SetExternalKind("github_pull_request").
		SetExternalID("apache/flink-kubernetes-operator#42").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityUnknown).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create pull request: %v", err)
	}
	relationRow, err := store.Client().TicketPullRequest.Create().
		SetTicketID(ticketRow.ID).
		SetPullRequestID(prRow.ID).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance(source).
		SetExternalKind("jira_remote_link").
		SetExternalID("FLINK-12345:apache/flink-kubernetes-operator#42").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityUnknown).
		SetConfidence(0.95).
		Save(ctx)
	if err != nil {
		t.Fatalf("create ticket pull request: %v", err)
	}
	_, err = store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:canonical-ticket-pr").
		SetEdgeKind(workdependencyedge.EdgeKindTicketPr).
		SetRelationshipAuthority(workdependencyedge.RelationshipAuthorityCanonicalMirror).
		SetCanonicalRelationshipKind(workdependencyedge.CanonicalRelationshipKindTicketPullRequest).
		SetFromKind(workdependencyedge.FromKindTicket).
		SetFromKey("FLINK-12345").
		SetToKind(workdependencyedge.ToKindPullRequest).
		SetToKey("apache/flink-kubernetes-operator#42").
		SetTicketID(ticketRow.ID).
		SetPullRequestID(prRow.ID).
		SetTicketPullRequestID(relationRow.ID).
		SetSourceCoverageState("observed:jira_remote_link").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("dependency:canonical-ticket-pr").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetConfidence(0.95).
		SetRankScore(80).
		SetLastActivityAt(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create canonical dependency edge: %v", err)
	}

	resolver := (&Resolver{EntClient: store.Client()}).Query().(*queryResolver)
	limit := 10
	rows, err := resolver.WorkDependencyEdges(ctx, &limit, nil, nil, nil, &source)
	if err != nil {
		t.Fatalf("work dependency edges: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("work dependency edge rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.RelationshipAuthority != "canonical_mirror" {
		t.Fatalf("relationshipAuthority = %q, want canonical_mirror", row.RelationshipAuthority)
	}
	if row.CanonicalRelationshipKind == nil || *row.CanonicalRelationshipKind != "ticket_pull_request" {
		t.Fatalf("canonicalRelationshipKind = %#v, want ticket_pull_request", row.CanonicalRelationshipKind)
	}
	if row.ClaimUse != "topology_context" || row.ClaimGateReason != "topology_context_not_product_claim" || row.RelationshipClaimAllowed {
		t.Fatalf("ticket_pr mirror should stay topology context in this resolver, got use=%q reason=%q allowed=%v", row.ClaimUse, row.ClaimGateReason, row.RelationshipClaimAllowed)
	}
}

type modelWorkDependencyEdgeForTest struct {
	relationshipClaimAllowed bool
	claimUse                 string
	claimGateReason          string
}

func assertDependencyGateForTest(t *testing.T, rows map[string]*modelWorkDependencyEdgeForTest, key string, allowed bool, claimUse string, gateReason string) {
	t.Helper()
	row := rows[key]
	if row == nil {
		t.Fatalf("missing dependency edge %q in rows: %#v", key, rows)
	}
	if row.relationshipClaimAllowed != allowed || row.claimUse != claimUse || row.claimGateReason != gateReason {
		t.Fatalf("dependency edge %q gate = allowed:%v use:%q reason:%q, want allowed:%v use:%q reason:%q",
			key, row.relationshipClaimAllowed, row.claimUse, row.claimGateReason, allowed, claimUse, gateReason)
	}
}
