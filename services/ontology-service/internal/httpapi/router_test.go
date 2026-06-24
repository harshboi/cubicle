package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent/document"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/message"
	"cubicle/services/ontology-service/ent/person"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/pullrequestlensresult"
	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketdocument"
	"cubicle/services/ontology-service/ent/ticketlensresult"
	"cubicle/services/ontology-service/ent/ticketmessage"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/ent/workaction"
	"cubicle/services/ontology-service/ent/workactionobservation"
	"cubicle/services/ontology-service/ent/workarea"
	"cubicle/services/ontology-service/ent/workblocker"
	"cubicle/services/ontology-service/ent/workblockerimpact"
	"cubicle/services/ontology-service/ent/workdependencyedge"
	"cubicle/services/ontology-service/ent/workdependencyendpoint"
	"cubicle/services/ontology-service/ent/workforecastevaluation"
	"cubicle/services/ontology-service/ent/workinsight"
	"cubicle/services/ontology-service/ent/workinsightreview"
	"cubicle/services/ontology-service/ent/workitemforecast"
	"cubicle/services/ontology-service/ent/workitemstatesnapshot"
	"cubicle/services/ontology-service/ent/workitemstatetransition"
	"cubicle/services/ontology-service/ent/worklens"
	"cubicle/services/ontology-service/ent/worklenswindow"
	"cubicle/services/ontology-service/ent/workownerloadsnapshot"
	"cubicle/services/ontology-service/ent/workprogramitem"
	"cubicle/services/ontology-service/ent/workstream"
	"cubicle/services/ontology-service/ent/workstreamhealthsnapshot"
	"cubicle/services/ontology-service/ent/workstreamstandupsection"
	"cubicle/services/ontology-service/ent/workstreamticket"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/entstore"
	ontologygraphql "cubicle/services/ontology-service/internal/graphql"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/sampledata"
)

type workEvidenceSummaryResponse struct {
	Key              string  `json:"key"`
	Ref              string  `json:"ref"`
	ClaimKind        string  `json:"claimKind"`
	ClaimTargetKind  string  `json:"claimTargetKind"`
	ClaimField       string  `json:"claimField"`
	RelationshipKind string  `json:"relationshipKind"`
	LocatorKind      string  `json:"locatorKind"`
	Locator          string  `json:"locator"`
	SourceSpanKey    string  `json:"sourceSpanKey"`
	SourceSystem     string  `json:"sourceSystem"`
	SourceInstance   string  `json:"sourceInstance"`
	ExternalKind     string  `json:"externalKind"`
	ExternalID       string  `json:"externalId"`
	SourceURL        string  `json:"sourceUrl"`
	ProofState       string  `json:"proofState"`
	FreshnessState   string  `json:"freshnessState"`
	Visibility       string  `json:"visibility"`
	Confidence       float64 `json:"confidence"`
	ObservedAt       string  `json:"observedAt"`
	Excerpt          string  `json:"excerpt"`
	ExcerptTruncated bool    `json:"excerptTruncated"`
}

func TestHealthzReturnsOK(t *testing.T) {
	router := NewRouter(slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "$schema") {
		t.Fatalf("health response leaked schema metadata into DTO body: %s", rec.Body.String())
	}

	var response HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("health response is not JSON: %v", err)
	}
	if !response.OK || response.Service != "ontology-service" {
		t.Fatalf("expected ok health response, got %#v", response)
	}
}

func TestGraphQLHealthQuery(t *testing.T) {
	router := NewRouter(slog.Default())

	body := `{"query":"query { health { ok service } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Data struct {
			Health HealthResponse `json:"health"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if !response.Data.Health.OK || response.Data.Health.Service != "ontology-service" {
		t.Fatalf("unexpected graphql health response: %#v", response)
	}
}

func TestGraphQLBoundedGraphContextUsesConfiguredExpander(t *testing.T) {
	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		GraphExpander:            sampledata.NewGenericDocumentMessageTicketMemoryStore(),
	})

	body := `{"query":"query { boundedGraphContext(startObjectType: \"document\", startKey: \"doc:architecture-note\") { scopeMode coverage { coverageState absenceClaimsAllowed } objects { objectType key } associations { associationType claimAllowed claimGateReason } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Data struct {
			BoundedGraphContext struct {
				ScopeMode string `json:"scopeMode"`
				Coverage  struct {
					CoverageState        string `json:"coverageState"`
					AbsenceClaimsAllowed bool   `json:"absenceClaimsAllowed"`
				} `json:"coverage"`
				Objects []struct {
					ObjectType string `json:"objectType"`
					Key        string `json:"key"`
				} `json:"objects"`
				Associations []struct {
					AssociationType string `json:"associationType"`
					ClaimAllowed    bool   `json:"claimAllowed"`
					ClaimGateReason string `json:"claimGateReason"`
				} `json:"associations"`
			} `json:"boundedGraphContext"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	got := response.Data.BoundedGraphContext
	if got.ScopeMode != "bounded_graph_context" || got.Coverage.CoverageState != "sparse" || got.Coverage.AbsenceClaimsAllowed {
		t.Fatalf("bounded graph policy = scope:%q coverage:%#v", got.ScopeMode, got.Coverage)
	}
	if len(got.Objects) != 3 || len(got.Associations) != 2 {
		t.Fatalf("bounded graph counts = objects:%d associations:%d, want 3/2", len(got.Objects), len(got.Associations))
	}
	candidateFound := false
	for _, association := range got.Associations {
		if association.AssociationType == "possible_followup_for" {
			candidateFound = true
			if association.ClaimAllowed || association.ClaimGateReason != "candidate_link_requires_human_review" {
				t.Fatalf("candidate association = %#v, want gated non-claimable link", association)
			}
		}
	}
	if !candidateFound {
		t.Fatalf("bounded graph associations = %#v, want possible_followup_for candidate", got.Associations)
	}
}

func TestGraphQLBoundedGraphContextUsesHTTPPrincipalAccessBeforeTraversal(t *testing.T) {
	ctx := context.Background()
	store := graphstore.NewMemoryStore()
	for _, object := range []domain.Object{
		{
			ObjectType:     "document",
			Key:            "doc:http-public-seed",
			Title:          "HTTP public seed",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "message",
			Key:            "message:http-public-direct",
			Title:          "HTTP public direct message",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "person",
			Key:            "person:http-secret-alice",
			Title:          "HTTP Secret Alice",
			Visibility:     "private",
			FreshnessState: domain.FreshnessFresh,
		},
		{
			ObjectType:     "ticket",
			Key:            "ticket:http-public-through-private-hub",
			Title:          "HTTP public descendant through private hub",
			Visibility:     domain.VisibilityPublic,
			FreshnessState: domain.FreshnessFresh,
		},
	} {
		if err := store.UpsertObject(ctx, object); err != nil {
			t.Fatalf("upsert object %s: %v", object.Key, err)
		}
	}
	for _, association := range []domain.Association{
		{
			Key:             "a-http-private-hub",
			From:            domain.ObjectRef{ObjectType: "document", Key: "doc:http-public-seed"},
			To:              domain.ObjectRef{ObjectType: "person", Key: "person:http-secret-alice"},
			AssociationType: "mentions",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:http-private-hub",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "mentions",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               "private",
				FreshnessState:           domain.FreshnessFresh,
			},
		},
		{
			Key:             "z-http-public-direct",
			From:            domain.ObjectRef{ObjectType: "document", Key: "doc:http-public-seed"},
			To:              domain.ObjectRef{ObjectType: "message", Key: "message:http-public-direct"},
			AssociationType: "mentions",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:http-public-direct",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "mentions",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               domain.VisibilityPublic,
				FreshnessState:           domain.FreshnessFresh,
			},
		},
		{
			Key:             "http-private-hub-to-public-descendant",
			From:            domain.ObjectRef{ObjectType: "person", Key: "person:http-secret-alice"},
			To:              domain.ObjectRef{ObjectType: "ticket", Key: "ticket:http-public-through-private-hub"},
			AssociationType: "assignee",
			Metadata: domain.AssociationMetadata{
				EvidenceKey:              "evidence:http-private-descendant",
				EvidenceClaimKind:        "relationship",
				EvidenceRelationshipKind: "assignee",
				EvidenceProofState:       "current",
				Confidence:               1,
				Visibility:               "private",
				FreshnessState:           domain.FreshnessFresh,
			},
		},
	} {
		if err := store.UpsertAssociation(ctx, association); err != nil {
			t.Fatalf("upsert association %s: %v", association.Key, err)
		}
	}

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		GraphExpander:            store,
		BoundedGraphPrincipalAccessProvider: func(r *http.Request) (ontologygraphql.BoundedGraphPrincipalAccess, bool) {
			principal := strings.TrimSpace(r.Header.Get("X-Test-Principal"))
			if principal == "" {
				return ontologygraphql.BoundedGraphPrincipalAccess{}, false
			}
			access := ontologygraphql.BoundedGraphPrincipalAccess{PrincipalKey: principal}
			if strings.EqualFold(r.Header.Get("X-Test-Allow-Private"), "true") {
				access.AllowedVisibilityClasses = []string{"private"}
			}
			return access, true
		},
	})
	body := `{"query":"query { boundedGraphContext(startObjectType: \"document\", startKey: \"doc:http-public-seed\", depth: 2, limitPerObject: 1) { objects { objectType key title } associations { key associationType } } }"}`

	publicOnlyBody := postGraphQLBody(t, router, body, map[string]string{
		"X-Test-Principal": "user:bob",
	})
	if !strings.Contains(publicOnlyBody, "message:http-public-direct") || !strings.Contains(publicOnlyBody, "z-http-public-direct") {
		t.Fatalf("public-only graphql response = %s, want public direct edge after private edge is skipped", publicOnlyBody)
	}
	for _, forbidden := range []string{"person:http-secret-alice", "ticket:http-public-through-private-hub", "HTTP Secret Alice", "private-hub"} {
		if strings.Contains(publicOnlyBody, forbidden) {
			t.Fatalf("public-only graphql response leaked %q: %s", forbidden, publicOnlyBody)
		}
	}

	privateAllowedBody := postGraphQLBody(t, router, body, map[string]string{
		"X-Test-Principal":     "user:alice",
		"X-Test-Allow-Private": "true",
	})
	for _, expected := range []string{"person:http-secret-alice", "ticket:http-public-through-private-hub", "a-http-private-hub", "http-private-hub-to-public-descendant"} {
		if !strings.Contains(privateAllowedBody, expected) {
			t.Fatalf("private-allowed graphql response missing %q: %s", expected, privateAllowedBody)
		}
	}
}

func postGraphQLBody(t *testing.T, router http.Handler, body string, headers map[string]string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"errors"`) {
		t.Fatalf("graphql response had errors: %s", rec.Body.String())
	}
	return rec.Body.String()
}

func TestGraphQLBoundedGraphContextRejectsCallerControlledCoverage(t *testing.T) {
	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		GraphExpander:            sampledata.NewGenericDocumentMessageTicketMemoryStore(),
	})

	body := `{"query":"query { boundedGraphContext(startObjectType: \"document\", startKey: \"doc:architecture-note\", absenceClaimsAllowed: true) { contextHash } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "absenceClaimsAllowed") {
		t.Fatalf("graphql response = %s, want rejected caller-controlled coverage argument", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Unknown argument") && !strings.Contains(rec.Body.String(), "Cannot query field") {
		t.Fatalf("graphql response = %s, want GraphQL validation error", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "contextHash") {
		t.Fatalf("graphql response = %s, should not return bounded graph data for rejected coverage argument", rec.Body.String())
	}
}

func TestGraphQLBoundedGraphContextRequiresConfiguredExpander(t *testing.T) {
	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
	})

	body := `{"query":"query { boundedGraphContext(startObjectType: \"document\", startKey: \"doc:architecture-note\") { contextHash } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "configured graph expander") {
		t.Fatalf("graphql response = %s, want configured graph expander error", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"contextHash"`) {
		t.Fatalf("graphql response = %s, should not return graph data without expander", rec.Body.String())
	}
}

func TestGraphQLBoundedGraphContextUsesEntProductRowsWhenAvailable(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	observedAt := time.Date(2026, 6, 24, 11, 0, 0, 0, time.UTC)
	ticketRow := store.Client().Ticket.Create().
		SetKey("ticket:HTTP-1").
		SetTitle("HTTP ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue").
		SetExternalID("HTTP-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	prRow := store.Client().PullRequest.Create().
		SetKey("pull-request:repo/example#42").
		SetRepository("repo/example").
		SetNumber(42).
		SetTitle("HTTP PR").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#42").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	evidenceRow := store.Client().Evidence.Create().
		SetKey("evidence:http-ticket-pr").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_pull_request").
		SetRelationshipKind("implemented_by").
		SetLocatorKind("jira_remote_link").
		SetLocator("HTTP-1 remote links").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("HTTP-1->repo/example#42").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	store.Client().TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(prRow).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("HTTP-1->repo/example#42").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(evidenceRow).
		SaveX(ctx)
	documentRow := store.Client().Document.Create().
		SetKey("document:http-design").
		SetTitle("HTTP design note").
		SetDocumentKind(document.DocumentKindMarkdown).
		SetSourceSystem("docs").
		SetSourceInstance("example").
		SetExternalKind("markdown").
		SetExternalID("http-design.md").
		SetFreshnessState(document.FreshnessStateFresh).
		SetVisibility(document.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	messageRow := store.Client().Message.Create().
		SetKey("message:http-standup").
		SetBody("HTTP-1 was discussed in standup.").
		SetSummary("HTTP standup note").
		SetSourceSystem("slack").
		SetSourceInstance("example").
		SetExternalKind("slack_message").
		SetExternalID("http-standup").
		SetFreshnessState(message.FreshnessStateFresh).
		SetVisibility(message.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	documentEvidence := store.Client().Evidence.Create().
		SetKey("evidence:http-ticket-document").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_document").
		SetRelationshipKind("documented_by").
		SetLocatorKind("jira_issue_link").
		SetLocator("HTTP-1 linked docs").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue_link").
		SetExternalID("HTTP-1->http-design.md").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	messageEvidence := store.Client().Evidence.Create().
		SetKey("evidence:http-ticket-message").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_message").
		SetRelationshipKind("discussed_in").
		SetLocatorKind("chat_message").
		SetLocator("example/http-standup").
		SetSourceSystem("chat").
		SetSourceInstance("example").
		SetExternalKind("slack_message").
		SetExternalID("http-standup").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	store.Client().TicketDocument.Create().
		SetTicket(ticketRow).
		SetDocument(documentRow).
		SetTicketDocumentKind(ticketdocument.TicketDocumentKindDocumentedBy).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue_link").
		SetExternalID("HTTP-1->http-design.md").
		SetFreshnessState(ticketdocument.FreshnessStateFresh).
		SetVisibility(ticketdocument.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(8).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(documentEvidence).
		SaveX(ctx)
	store.Client().TicketMessage.Create().
		SetTicket(ticketRow).
		SetMessage(messageRow).
		SetTicketMessageKind(ticketmessage.TicketMessageKindDiscussedIn).
		SetSourceSystem("slack").
		SetSourceInstance("example").
		SetExternalKind("slack_message").
		SetExternalID("HTTP-1->http-standup").
		SetFreshnessState(ticketmessage.FreshnessStateFresh).
		SetVisibility(ticketmessage.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(6).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(messageEvidence).
		SaveX(ctx)
	sourceConnection := store.Client().SourceConnection.Create().
		SetKey("source-connection:http-docs").
		SetSourceSystem("docs").
		SetSourceInstance("example").
		SetDisplayName("HTTP docs").
		SetConnectorKind("docs").
		SaveX(ctx)
	sourceScope := store.Client().SourceScope.Create().
		SetKey("source-scope:http-docs").
		SetConnection(sourceConnection).
		SetScopeKind("folder").
		SetScopeKey("http-docs").
		SaveX(ctx)
	sourceRun := store.Client().SourceSyncRun.Create().
		SetScope(sourceScope).
		SetRunKey("source-run:http-docs").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModePartialScope).
		SetStatus(sourcesyncrun.StatusComplete).
		SetStartedAt(observedAt).
		SaveX(ctx)
	store.Client().SourceSyncIssue.Create().
		SetScope(sourceScope).
		SetSyncRun(sourceRun).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("source_forbidden").
		SetMessage("raw status 403 body secret-token=abc123 must stay replay-only").
		SetSourceSystem("docs").
		SetSourceInstance("example").
		SetExternalKind("markdown").
		SetExternalID("http-design.md").
		SetSourceURL("https://docs.example.test/private/http-design").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { boundedGraphContext(startObjectType: \"ticket\", startKey: \"ticket:HTTP-1\", associationTypes: [\"implemented_by\"]) { objects { objectType key } associations { associationType from { objectType key } to { objectType key } claimAllowed claimGateReason evidenceKey } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			BoundedGraphContext struct {
				Coverage struct {
					CoverageState          string `json:"coverageState"`
					AbsenceClaimsAllowed   bool   `json:"absenceClaimsAllowed"`
					AbsenceClaimGateReason string `json:"absenceClaimGateReason"`
					Summary                string `json:"summary"`
				} `json:"coverage"`
				Objects []struct {
					ObjectType          string `json:"objectType"`
					Key                 string `json:"key"`
					SourceCoverageState string `json:"sourceCoverageState"`
				} `json:"objects"`
				Associations []struct {
					AssociationType string `json:"associationType"`
					From            struct {
						ObjectType string `json:"objectType"`
						Key        string `json:"key"`
					} `json:"from"`
					To struct {
						ObjectType string `json:"objectType"`
						Key        string `json:"key"`
					} `json:"to"`
					ClaimAllowed    bool   `json:"claimAllowed"`
					ClaimGateReason string `json:"claimGateReason"`
					EvidenceKey     string `json:"evidenceKey"`
				} `json:"associations"`
			} `json:"boundedGraphContext"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	got := response.Data.BoundedGraphContext
	if len(got.Objects) != 2 || len(got.Associations) != 1 {
		t.Fatalf("bounded graph = %#v, want ticket, PR, and one relationship", got)
	}
	association := got.Associations[0]
	if association.AssociationType != "implemented_by" || association.From.Key != "ticket:HTTP-1" || association.To.Key != "pull-request:repo/example#42" {
		t.Fatalf("association = %#v, want canonical ticket implemented_by PR", association)
	}
	if !association.ClaimAllowed || association.ClaimGateReason != "source_evidence_full_confidence" || association.EvidenceKey != "evidence:http-ticket-pr" {
		t.Fatalf("association claim policy = %#v, want claimable evidence-backed relationship", association)
	}

	body = `{"query":"query { boundedGraphContext(startObjectType: \"document\", startKey: \"document:http-design\", associationTypes: [\"documented_by\", \"discussed_in\"]) { coverage { coverageState absenceClaimsAllowed absenceClaimGateReason summary } objects { objectType key sourceCoverageState } associations { associationType from { objectType key } to { objectType key } claimAllowed claimGateReason evidenceKey } } }"}`
	req = httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	response = struct {
		Data struct {
			BoundedGraphContext struct {
				Coverage struct {
					CoverageState          string `json:"coverageState"`
					AbsenceClaimsAllowed   bool   `json:"absenceClaimsAllowed"`
					AbsenceClaimGateReason string `json:"absenceClaimGateReason"`
					Summary                string `json:"summary"`
				} `json:"coverage"`
				Objects []struct {
					ObjectType          string `json:"objectType"`
					Key                 string `json:"key"`
					SourceCoverageState string `json:"sourceCoverageState"`
				} `json:"objects"`
				Associations []struct {
					AssociationType string `json:"associationType"`
					From            struct {
						ObjectType string `json:"objectType"`
						Key        string `json:"key"`
					} `json:"from"`
					To struct {
						ObjectType string `json:"objectType"`
						Key        string `json:"key"`
					} `json:"to"`
					ClaimAllowed    bool   `json:"claimAllowed"`
					ClaimGateReason string `json:"claimGateReason"`
					EvidenceKey     string `json:"evidenceKey"`
				} `json:"associations"`
			} `json:"boundedGraphContext"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}{}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	got = response.Data.BoundedGraphContext
	if len(got.Objects) != 3 || len(got.Associations) != 2 {
		t.Fatalf("bounded document graph = %#v, want document, ticket, message, and two relationships", got)
	}
	if got.Coverage.CoverageState != "limited" || got.Coverage.AbsenceClaimsAllowed || got.Coverage.AbsenceClaimGateReason != "source_auth_or_rate_limit" {
		t.Fatalf("bounded document coverage = %#v, want limited auth-gated absence policy", got.Coverage)
	}
	for _, expected := range []string{"1 source sync issue", "1 auth/rate-limit issue"} {
		if !strings.Contains(got.Coverage.Summary, expected) {
			t.Fatalf("coverage summary = %q, want %q", got.Coverage.Summary, expected)
		}
	}
	for _, forbidden := range []string{"403", "secret-token", "docs.example.test/private"} {
		if strings.Contains(got.Coverage.Summary, forbidden) {
			t.Fatalf("coverage summary leaked raw sync issue detail %q: %q", forbidden, got.Coverage.Summary)
		}
	}
	if !graphqlBoundedGraphHasObject(got.Objects, "ticket", "ticket:HTTP-1") || !graphqlBoundedGraphHasObject(got.Objects, "message", "message:http-standup") {
		t.Fatalf("bounded document graph objects = %#v, want ticket and message reached from document", got.Objects)
	}
	if !graphqlBoundedGraphHasObjectCoverage(got.Objects, "document", "document:http-design", "limited") {
		t.Fatalf("bounded document graph objects = %#v, want seed object coverage state limited", got.Objects)
	}
	if !graphqlBoundedGraphHasAssociation(got.Associations, "ticket:HTTP-1", "documented_by", "document:http-design", "evidence:http-ticket-document") {
		t.Fatalf("bounded document graph associations = %#v, want canonical ticket documented_by document", got.Associations)
	}
	if !graphqlBoundedGraphHasAssociation(got.Associations, "ticket:HTTP-1", "discussed_in", "message:http-standup", "evidence:http-ticket-message") {
		t.Fatalf("bounded document graph associations = %#v, want canonical ticket discussed_in message", got.Associations)
	}
}

func TestGraphQLBoundedGraphContextUsesEntOpenGraphRowsWhenAvailable(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()
	if err := sampledata.SeedOpenCustomerIncidentGraph(ctx, store.Client()); err != nil {
		t.Fatalf("seed open customer incident graph: %v", err)
	}

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled:    false,
		EntClient:                   store.Client(),
		BoundedGraphSourceAuthority: sampledata.OpenCustomerIncidentSourceAuthorityPolicy(),
	})
	body := `{"query":"query { boundedGraphContext(startObjectType: \"customer_account\", startKey: \"customer:acme\", associationTypes: [\"affected_by\", \"mitigated_by\", \"updated_in\"], depth: 2, limitPerObject: 3) { coverage { coverageState absenceClaimsAllowed } objects { objectType key } associations { associationType from { objectType key } to { objectType key } claimAllowed claimGateReason evidenceKey } evidence { key source locatorKind } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			BoundedGraphContext struct {
				Coverage struct {
					CoverageState        string `json:"coverageState"`
					AbsenceClaimsAllowed bool   `json:"absenceClaimsAllowed"`
				} `json:"coverage"`
				Objects []struct {
					ObjectType string `json:"objectType"`
					Key        string `json:"key"`
				} `json:"objects"`
				Associations []graphqlBoundedGraphAssociation `json:"associations"`
				Evidence     []graphqlBoundedGraphEvidence    `json:"evidence"`
			} `json:"boundedGraphContext"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	got := response.Data.BoundedGraphContext
	if got.Coverage.CoverageState != "sparse" || got.Coverage.AbsenceClaimsAllowed {
		t.Fatalf("coverage = %#v, want sparse open graph coverage with absence claims gated", got.Coverage)
	}
	serialized, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal bounded graph response: %v", err)
	}
	for _, want := range []string{
		`"objectType":"customer_account","key":"customer:acme"`,
		`"objectType":"incident","key":"incident:sev-42"`,
		`"objectType":"runbook_document","key":"runbook:failover"`,
		`"objectType":"slack_message","key":"slack:incident-update-1"`,
	} {
		if !strings.Contains(string(serialized), want) {
			t.Fatalf("open graph context = %s, want %s", serialized, want)
		}
	}
	if strings.Contains(string(serialized), "incident:hidden-private") {
		t.Fatalf("open graph context = %s, private high-rank incident should stay filtered", serialized)
	}
	for _, expected := range []struct {
		from    string
		typ     string
		to      string
		source  string
		locator string
	}{
		{from: "customer:acme", typ: "affected_by", to: "incident:sev-42", source: "pagerduty", locator: "incident_link"},
		{from: "incident:sev-42", typ: "mitigated_by", to: "runbook:failover", source: "docs", locator: "runbook_link"},
		{from: "incident:sev-42", typ: "updated_in", to: "slack:incident-update-1", source: "slack", locator: "slack_message"},
	} {
		if !graphqlBoundedGraphHasClaimableAssociation(got.Associations, got.Evidence, expected.from, expected.typ, expected.to, expected.source, expected.locator) {
			t.Fatalf("open graph associations = %#v, want claimable %+v", got.Associations, expected)
		}
	}
}

type graphqlBoundedGraphRef struct {
	ObjectType string `json:"objectType"`
	Key        string `json:"key"`
}

type graphqlBoundedGraphAssociation struct {
	AssociationType string                 `json:"associationType"`
	From            graphqlBoundedGraphRef `json:"from"`
	To              graphqlBoundedGraphRef `json:"to"`
	ClaimAllowed    bool                   `json:"claimAllowed"`
	ClaimGateReason string                 `json:"claimGateReason"`
	EvidenceKey     string                 `json:"evidenceKey"`
}

type graphqlBoundedGraphEvidence struct {
	Key         string `json:"key"`
	Source      string `json:"source"`
	LocatorKind string `json:"locatorKind"`
}

func graphqlBoundedGraphHasClaimableAssociation(associations []graphqlBoundedGraphAssociation, evidence []graphqlBoundedGraphEvidence, from string, associationType string, to string, source string, locatorKind string) bool {
	evidenceByKey := make(map[string]graphqlBoundedGraphEvidence, len(evidence))
	for _, row := range evidence {
		evidenceByKey[row.Key] = row
	}
	for _, association := range associations {
		evidenceRow := evidenceByKey[association.EvidenceKey]
		if association.From.Key == from &&
			association.AssociationType == associationType &&
			association.To.Key == to &&
			association.ClaimAllowed &&
			association.ClaimGateReason == "source_evidence_full_confidence" &&
			evidenceRow.Source == source &&
			evidenceRow.LocatorKind == locatorKind {
			return true
		}
	}
	return false
}

func graphqlBoundedGraphHasObject(objects []struct {
	ObjectType          string `json:"objectType"`
	Key                 string `json:"key"`
	SourceCoverageState string `json:"sourceCoverageState"`
}, objectType string, key string) bool {
	for _, object := range objects {
		if object.ObjectType == objectType && object.Key == key {
			return true
		}
	}
	return false
}

func graphqlBoundedGraphHasObjectCoverage(objects []struct {
	ObjectType          string `json:"objectType"`
	Key                 string `json:"key"`
	SourceCoverageState string `json:"sourceCoverageState"`
}, objectType string, key string, coverageState string) bool {
	for _, object := range objects {
		if object.ObjectType == objectType && object.Key == key && object.SourceCoverageState == coverageState {
			return true
		}
	}
	return false
}

func graphqlBoundedGraphHasAssociation(associations []struct {
	AssociationType string `json:"associationType"`
	From            struct {
		ObjectType string `json:"objectType"`
		Key        string `json:"key"`
	} `json:"from"`
	To struct {
		ObjectType string `json:"objectType"`
		Key        string `json:"key"`
	} `json:"to"`
	ClaimAllowed    bool   `json:"claimAllowed"`
	ClaimGateReason string `json:"claimGateReason"`
	EvidenceKey     string `json:"evidenceKey"`
}, fromKey string, associationType string, toKey string, evidenceKey string) bool {
	for _, association := range associations {
		if association.From.Key == fromKey &&
			association.AssociationType == associationType &&
			association.To.Key == toKey &&
			association.EvidenceKey == evidenceKey &&
			association.ClaimAllowed &&
			association.ClaimGateReason == "source_evidence_full_confidence" {
			return true
		}
	}
	return false
}

func TestGraphQLWorkActionsStartFromGatedActionRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	ticketRow := store.Client().Ticket.Create().
		SetKey("ticket:test:FLINK-1").
		SetTitle("Example ticket").
		SetStatus(ticket.StatusOpen).
		SetExternalID("FLINK-1").
		SetSourceURL("https://issues.example.test/FLINK-1").
		SaveX(ctx)
	pr := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#1").
		SetRepository("repo/example").
		SetNumber(1).
		SetTitle("Example PR").
		SetSourceURL("https://github.com/repo/example/pull/1").
		SaveX(ctx)
	store.Client().TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(pr).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SaveX(ctx)
	insightEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:ci").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_insight").
		SetLocatorKind("github_check_runs").
		SetLocator("https://github.com/repo/example/pull/1/checks").
		SetSourceSpanKey("checks:repo/example#1").
		SetExcerpt("maven build is failing").
		SetSourceSystem("github").
		SetSourceInstance("github.com/repo/example").
		SetExternalKind("github_check_runs").
		SetExternalID("repo/example#1").
		SetSourceURL("https://github.com/repo/example/pull/1/checks").
		SaveX(ctx)
	insight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:ci").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityHigh).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#1").
		SetPullRequest(pr).
		SetTitle("CI check state needs review").
		SetDetails("CI is failing but required-check semantics need validation.").
		SetRecommendedAction("Review required checks before product escalation.").
		SetModelMethod("typed_check_rule").
		SetScore(84).
		SetScoreExplanation("Score combines failing checks and open PR state.").
		SetConfidence(0.72).
		SetLatestEvidence(insightEvidence).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:ci").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:ci-triage").
		SetInsight(insight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetNextAction("Validate whether the check is required.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_insight_review").
		SetExternalID("work-insight-review:test:ci-triage").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:ci-gold").
		SetInsight(insight).
		SetReviewKind(workinsightreview.ReviewKindEvaluationLabel).
		SetReviewState(workinsightreview.ReviewStateAccepted).
		SetTruthLabel(workinsightreview.TruthLabelTruePositive).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelNeedsOwner).
		SetLabelSet("agent_gold_blockers").
		SetLabelQuality(workinsightreview.LabelQualityGold).
		SetMeasurementEligible(true).
		SetReviewerKind(workinsightreview.ReviewerKindImported).
		SetReviewerKey("agent_gold").
		SetNextAction("Assign CI owner.").
		SetRationale("The failing check needs an owner before merge.").
		SetReviewedAt(time.Date(2026, 6, 21, 5, 30, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_insight_review").
		SetExternalID("work-insight-review:test:ci-gold").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:ci-human").
		SetInsight(insight).
		SetReviewKind(workinsightreview.ReviewKindHumanAssessment).
		SetReviewState(workinsightreview.ReviewStateNeedsMoreData).
		SetTruthLabel(workinsightreview.TruthLabelPartial).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelNeedsOwner).
		SetLabelSet("human_review").
		SetLabelQuality(workinsightreview.LabelQualityGold).
		SetMeasurementEligible(true).
		SetReviewerKind(workinsightreview.ReviewerKindHuman).
		SetReviewerKey("harsh").
		SetNextAction("Ask CI owner whether this is required.").
		SetRationale("Human reviewer wants required-check semantics before escalation.").
		SetReviewedAt(time.Date(2026, 6, 21, 5, 20, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_insight_review").
		SetExternalID("work-insight-review:test:ci-human").
		SaveX(ctx)
	action := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:ci").
		SetActionType(workaction.ActionTypeCiCheckFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetDecision("pending_validation").
		SetDecisionReason("required-check semantics are not modeled yet").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#1").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(84).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:ci").
		AddSourceInsights(insight).
		SaveX(ctx)
	store.Client().WorkActionObservation.Create().
		SetKey("work-action-observation:test:ci").
		SetAction(action).
		SetObservationKind(workactionobservation.ObservationKindCiSignal).
		SetSourceCoverageState("observed:check_coverage:complete").
		SetCurrentState("open").
		SetCiSignal("failing_or_pending").
		SetObservedAt(time.Date(2026, 6, 21, 5, 0, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_action_observation").
		SetExternalID("work-action-observation:test:ci").
		SaveX(ctx)
	store.Client().WorkAction.Create().
		SetKey("tpm-action:test:watch").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetDecision("pending_validation").
		SetDecisionReason("watch item should not outrank now bucket").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#1").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketWatch).
		SetRankScore(100).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:watch").
		AddSourceInsights(insight).
		SaveX(ctx)
	store.Client().WorkAction.Create().
		SetKey("tpm-action:test:superseded").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateSuperseded).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetDecision("pending_validation").
		SetDecisionReason("superseded rows should not appear in default workActions reads").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#1").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(999).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:superseded").
		AddSourceInsights(insight).
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workActions(limit: 1, decisionState: \"validation_lead\") { key actionType decisionState decisionReason subjectKey subjectTitle subjectUrl relatedTicketKeys relatedPullRequestKeys decisionNeeded recommendedAction claimUse claimGateReason productActionAllowed absenceClaimAllowed etaClaimAllowed evidenceRef evidence { key ref claimKind claimTargetKind locatorKind locator sourceSpanKey sourceSystem sourceInstance externalKind externalId sourceUrl proofState freshnessState visibility confidence excerpt excerptTruncated } observations { observationKind supportsAction currentState ciSignal } sourceInsights { insightKind subjectKey title details recommendedAction modelMethod score scoreExplanation confidence sourceUrl evidenceRef evidence { key ref claimKind claimTargetKind locatorKind locator sourceSpanKey sourceSystem sourceInstance externalKind externalId sourceUrl proofState freshnessState visibility confidence excerpt excerptTruncated } evidenceExcerpt reviewKind reviewState truthLabel actionabilityLabel labelQuality measurementEligible reviewerKind reviewerKey labelSet reviewNextAction reviewRationale badges { key tone detail } } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkActions []struct {
				Key                    string                       `json:"key"`
				ActionType             string                       `json:"actionType"`
				DecisionState          string                       `json:"decisionState"`
				DecisionReason         string                       `json:"decisionReason"`
				SubjectKey             string                       `json:"subjectKey"`
				SubjectTitle           string                       `json:"subjectTitle"`
				SubjectURL             string                       `json:"subjectUrl"`
				RelatedTicketKeys      []string                     `json:"relatedTicketKeys"`
				RelatedPullRequestKeys []string                     `json:"relatedPullRequestKeys"`
				DecisionNeeded         string                       `json:"decisionNeeded"`
				RecommendedAction      string                       `json:"recommendedAction"`
				ClaimUse               string                       `json:"claimUse"`
				ClaimGateReason        string                       `json:"claimGateReason"`
				ProductActionAllowed   bool                         `json:"productActionAllowed"`
				AbsenceClaimAllowed    bool                         `json:"absenceClaimAllowed"`
				EtaClaimAllowed        bool                         `json:"etaClaimAllowed"`
				EvidenceRef            string                       `json:"evidenceRef"`
				Evidence               *workEvidenceSummaryResponse `json:"evidence"`
				Observations           []struct {
					ObservationKind string `json:"observationKind"`
					SupportsAction  bool   `json:"supportsAction"`
					CurrentState    string `json:"currentState"`
					CISignal        string `json:"ciSignal"`
				} `json:"observations"`
				SourceInsights []struct {
					InsightKind         string                       `json:"insightKind"`
					SubjectKey          string                       `json:"subjectKey"`
					Title               string                       `json:"title"`
					Details             string                       `json:"details"`
					RecommendedAction   string                       `json:"recommendedAction"`
					ModelMethod         string                       `json:"modelMethod"`
					Score               float64                      `json:"score"`
					ScoreExplanation    string                       `json:"scoreExplanation"`
					Confidence          float64                      `json:"confidence"`
					SourceURL           string                       `json:"sourceUrl"`
					EvidenceRef         string                       `json:"evidenceRef"`
					Evidence            *workEvidenceSummaryResponse `json:"evidence"`
					EvidenceExcerpt     string                       `json:"evidenceExcerpt"`
					ReviewKind          string                       `json:"reviewKind"`
					ReviewState         string                       `json:"reviewState"`
					TruthLabel          string                       `json:"truthLabel"`
					ActionabilityLabel  string                       `json:"actionabilityLabel"`
					LabelQuality        string                       `json:"labelQuality"`
					MeasurementEligible bool                         `json:"measurementEligible"`
					ReviewerKind        string                       `json:"reviewerKind"`
					ReviewerKey         string                       `json:"reviewerKey"`
					LabelSet            string                       `json:"labelSet"`
					ReviewNextAction    string                       `json:"reviewNextAction"`
					ReviewRationale     string                       `json:"reviewRationale"`
					Badges              []struct {
						Key    string `json:"key"`
						Tone   string `json:"tone"`
						Detail string `json:"detail"`
					} `json:"badges"`
				} `json:"sourceInsights"`
			} `json:"workActions"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkActions) != 1 {
		t.Fatalf("expected one action, got %#v", response.Data.WorkActions)
	}
	got := response.Data.WorkActions[0]
	if got.Key != "tpm-action:test:ci" || got.ActionType != "ci_check_followup" || got.DecisionState != "validation_lead" {
		t.Fatalf("unexpected action row: %#v", got)
	}
	if got.SubjectKey != "repo/example#1" || !strings.Contains(got.DecisionReason, "required-check") {
		t.Fatalf("unexpected action details: %#v", got)
	}
	if got.SubjectTitle != "Example PR" || got.SubjectURL != "https://github.com/repo/example/pull/1" {
		t.Fatalf("unexpected typed subject details: %#v", got)
	}
	if strings.Join(got.RelatedTicketKeys, ",") != "FLINK-1" || strings.Join(got.RelatedPullRequestKeys, ",") != "repo/example#1" {
		t.Fatalf("unexpected typed related keys: %#v", got)
	}
	if got.DecisionNeeded != "determine required/merge-blocking check semantics" || !strings.Contains(got.RecommendedAction, "Review failing or pending GitHub checks") {
		t.Fatalf("unexpected action prompt: %#v", got)
	}
	if got.ClaimUse != "validation_lead" || !strings.Contains(got.ClaimGateReason, "required-check") || got.ProductActionAllowed || got.AbsenceClaimAllowed || got.EtaClaimAllowed {
		t.Fatalf("validation lead exposed unsafe claim gates: %#v", got)
	}
	if got.EvidenceRef != "https://github.com/repo/example/pull/1" {
		t.Fatalf("unexpected evidence ref: %#v", got)
	}
	if got.Evidence != nil {
		t.Fatalf("action without direct or observation evidence should not synthesize structured evidence: %#v", got.Evidence)
	}
	if len(got.Observations) != 1 || got.Observations[0].ObservationKind != "ci_signal" || got.Observations[0].SupportsAction {
		t.Fatalf("unexpected observations: %#v", got.Observations)
	}
	if len(got.SourceInsights) != 1 || got.SourceInsights[0].InsightKind != "status_summary" {
		t.Fatalf("unexpected source insights: %#v", got.SourceInsights)
	}
	sourceInsight := got.SourceInsights[0]
	if sourceInsight.Details == "" || sourceInsight.RecommendedAction != "Review required checks before product escalation." || sourceInsight.ModelMethod != "typed_check_rule" {
		t.Fatalf("source insight did not expose generated context: %#v", sourceInsight)
	}
	if sourceInsight.Score != 84 || sourceInsight.ScoreExplanation == "" || sourceInsight.Confidence != 0.72 {
		t.Fatalf("source insight did not expose score context: %#v", sourceInsight)
	}
	if sourceInsight.EvidenceRef == "" || sourceInsight.EvidenceExcerpt != "maven build is failing" {
		t.Fatalf("source insight did not expose evidence context: %#v", sourceInsight)
	}
	if sourceInsight.Evidence == nil || sourceInsight.Evidence.Key != insightEvidence.Key || sourceInsight.Evidence.ClaimKind != "object_state" || sourceInsight.Evidence.ClaimTargetKind != "work_insight" {
		t.Fatalf("source insight did not expose structured evidence identity: %#v", sourceInsight.Evidence)
	}
	if sourceInsight.Evidence.LocatorKind != "github_check_runs" || sourceInsight.Evidence.SourceSystem != "github" || sourceInsight.Evidence.SourceInstance != "github.com/repo/example" {
		t.Fatalf("source insight did not expose structured evidence source: %#v", sourceInsight.Evidence)
	}
	if sourceInsight.Evidence.ExternalKind != "github_check_runs" || sourceInsight.Evidence.ExternalID != "repo/example#1" || sourceInsight.Evidence.Excerpt != "maven build is failing" {
		t.Fatalf("source insight did not expose structured evidence locator/excerpt: %#v", sourceInsight.Evidence)
	}
	if sourceInsight.ReviewKind != "human_assessment" || sourceInsight.ReviewerKind != "human" || sourceInsight.ReviewerKey != "harsh" || sourceInsight.LabelSet != "human_review" {
		t.Fatalf("source insight did not expose human review provenance: %#v", sourceInsight)
	}
	if sourceInsight.ReviewState != "needs_more_data" || sourceInsight.TruthLabel != "partial" || sourceInsight.ActionabilityLabel != "needs_owner" || sourceInsight.LabelQuality != "gold" || !sourceInsight.MeasurementEligible {
		t.Fatalf("source insight did not prefer best human measurement label: %#v", sourceInsight)
	}
	if sourceInsight.ReviewNextAction != "Ask CI owner whether this is required." || sourceInsight.ReviewRationale == "" {
		t.Fatalf("source insight did not expose review guidance: %#v", sourceInsight)
	}
	sourceInsightBadgeKeys := map[string]bool{}
	for _, badge := range sourceInsight.Badges {
		sourceInsightBadgeKeys[badge.Key] = true
	}
	for _, key := range []string{"review:measurement_eligible", "reviewer:human", "truth:partial", "actionability:needs_owner"} {
		if !sourceInsightBadgeKeys[key] {
			t.Fatalf("missing source insight badge %q from %#v", key, sourceInsight.Badges)
		}
	}
}

func TestGraphQLWorkInsightReviewsRequiresExplicitOrCurrentSource(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	insight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:review-anchorless").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityHigh).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("repo/example#anchorless").
		SetTitle("Anchorless review queue row").
		SetScore(88).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:review-anchorless").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:review-anchorless").
		SetInsight(insight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetReviewerKey("flink_tpm_analytics").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight_review").
		SetExternalID("work-insight-review:test:review-anchorless").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { explicit: workInsightReviews(sourceInstance: \"fixture-source\") { key sourceInstance insight { subjectKey reviewState reviewerKey } } unscoped: workInsightReviews { key } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Explicit []struct {
				Key            string `json:"key"`
				SourceInstance string `json:"sourceInstance"`
				Insight        struct {
					SubjectKey  string `json:"subjectKey"`
					ReviewState string `json:"reviewState"`
					ReviewerKey string `json:"reviewerKey"`
				} `json:"insight"`
			} `json:"explicit"`
			Unscoped []struct {
				Key string `json:"key"`
			} `json:"unscoped"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.Explicit) != 1 || response.Data.Explicit[0].SourceInstance != "fixture-source" || response.Data.Explicit[0].Insight.ReviewState != "requested" || response.Data.Explicit[0].Insight.ReviewerKey != "flink_tpm_analytics" {
		t.Fatalf("explicit source review query did not return the typed review row: %#v", response.Data.Explicit)
	}
	if len(response.Data.Unscoped) != 0 {
		t.Fatalf("unscoped review query without current source anchor should not return all reviews: %#v", response.Data.Unscoped)
	}
}

func TestGraphQLWorkDependencyEdgesExposeAuthorityAndEndpoints(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	source := "test"
	now := time.Date(2026, 6, 22, 12, 30, 0, 0, time.UTC)
	ticketRow := store.Client().Ticket.Create().
		SetKey("ticket:test:FLINK-12345").
		SetTitle("Autoscaler work").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance(source).
		SetExternalKind("jira_issue").
		SetExternalID("FLINK-12345").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityUnknown).
		SetLastActivityAt(now).
		SaveX(ctx)
	prRow := store.Client().PullRequest.Create().
		SetKey("pull-request:test:apache/flink-kubernetes-operator#42").
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
		SaveX(ctx)
	store.Client().TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(prRow).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SaveX(ctx)
	edge := store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:test:canonical-ticket-pr").
		SetEdgeKind(workdependencyedge.EdgeKindTicketPr).
		SetRelationshipAuthority(workdependencyedge.RelationshipAuthorityCanonicalMirror).
		SetCanonicalRelationshipKind(workdependencyedge.CanonicalRelationshipKindTicketPullRequest).
		SetFromKind(workdependencyedge.FromKindTicket).
		SetFromKey("FLINK-12345").
		SetToKind(workdependencyedge.ToKindPullRequest).
		SetToKey("apache/flink-kubernetes-operator#42").
		SetTicketID(ticketRow.ID).
		SetPullRequestID(prRow.ID).
		SetSourceCoverageState("observed:jira_remote_link").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("dependency:canonical-ticket-pr").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetConfidence(0.95).
		SetRankScore(80).
		SetLastActivityAt(now).
		SaveX(ctx)
	store.Client().WorkDependencyEndpoint.Create().
		SetKey("work-dependency-endpoint:test:canonical-ticket-pr:from").
		SetWorkDependencyEdge(edge).
		SetEndpointRole(workdependencyendpoint.EndpointRoleFrom).
		SetNodeKind(workdependencyendpoint.NodeKindTicket).
		SetNodeKey("FLINK-12345").
		SetResolutionState(workdependencyendpoint.ResolutionStateResolved).
		SetResolutionReason("resolved to typed ticket row").
		SetTicket(ticketRow).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_endpoint").
		SetExternalID("dependency:canonical-ticket-pr:from").
		SetFreshnessState(workdependencyendpoint.FreshnessStateFresh).
		SetVisibility(workdependencyendpoint.VisibilityUnknown).
		SetConfidence(0.95).
		SetRankScore(80).
		SetLastActivityAt(now).
		SaveX(ctx)
	store.Client().WorkDependencyEndpoint.Create().
		SetKey("work-dependency-endpoint:test:canonical-ticket-pr:to").
		SetWorkDependencyEdge(edge).
		SetEndpointRole(workdependencyendpoint.EndpointRoleTo).
		SetNodeKind(workdependencyendpoint.NodeKindPullRequest).
		SetNodeKey("apache/flink-kubernetes-operator#42").
		SetResolutionState(workdependencyendpoint.ResolutionStateResolved).
		SetResolutionReason("resolved to typed pull request row").
		SetPullRequest(prRow).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance(source).
		SetExternalKind("tpm_work_dependency_endpoint").
		SetExternalID("dependency:canonical-ticket-pr:to").
		SetFreshnessState(workdependencyendpoint.FreshnessStateFresh).
		SetVisibility(workdependencyendpoint.VisibilityUnknown).
		SetConfidence(0.95).
		SetRankScore(80).
		SetLastActivityAt(now).
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		EntClient:                store.Client(),
		GraphQLPlaygroundEnabled: false,
	})
	body := `{"query":"query { workDependencyEdges(limit: 5, edgeKind: \"ticket_pr\", sourceInstance: \"test\") { key edgeKind relationshipAuthority canonicalRelationshipKind fromKind fromKey toKind toKey claimUse claimGateReason relationshipClaimAllowed fromEndpoint { endpointRole nodeKind nodeKey resolutionState } toEndpoint { endpointRole nodeKind nodeKey resolutionState } endpoints { endpointRole nodeKind nodeKey resolutionState } badges { key tone detail } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkDependencyEdges []struct {
				Key                       string `json:"key"`
				EdgeKind                  string `json:"edgeKind"`
				RelationshipAuthority     string `json:"relationshipAuthority"`
				CanonicalRelationshipKind string `json:"canonicalRelationshipKind"`
				FromKind                  string `json:"fromKind"`
				FromKey                   string `json:"fromKey"`
				ToKind                    string `json:"toKind"`
				ToKey                     string `json:"toKey"`
				ClaimUse                  string `json:"claimUse"`
				ClaimGateReason           string `json:"claimGateReason"`
				RelationshipClaimAllowed  bool   `json:"relationshipClaimAllowed"`
				FromEndpoint              struct {
					EndpointRole    string `json:"endpointRole"`
					NodeKind        string `json:"nodeKind"`
					NodeKey         string `json:"nodeKey"`
					ResolutionState string `json:"resolutionState"`
				} `json:"fromEndpoint"`
				ToEndpoint struct {
					EndpointRole    string `json:"endpointRole"`
					NodeKind        string `json:"nodeKind"`
					NodeKey         string `json:"nodeKey"`
					ResolutionState string `json:"resolutionState"`
				} `json:"toEndpoint"`
				Endpoints []struct {
					EndpointRole    string `json:"endpointRole"`
					NodeKind        string `json:"nodeKind"`
					NodeKey         string `json:"nodeKey"`
					ResolutionState string `json:"resolutionState"`
				} `json:"endpoints"`
				Badges []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"workDependencyEdges"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkDependencyEdges) != 1 {
		t.Fatalf("workDependencyEdges returned %d rows, want 1: %#v", len(response.Data.WorkDependencyEdges), response.Data.WorkDependencyEdges)
	}
	row := response.Data.WorkDependencyEdges[0]
	if row.EdgeKind != "ticket_pr" || row.RelationshipAuthority != "canonical_mirror" || row.CanonicalRelationshipKind != "ticket_pull_request" {
		t.Fatalf("dependency authority = edge:%q authority:%q canonical:%q, want canonical ticket_pull_request mirror", row.EdgeKind, row.RelationshipAuthority, row.CanonicalRelationshipKind)
	}
	if row.FromKind != "ticket" || row.FromKey != "FLINK-12345" || row.ToKind != "pull_request" || row.ToKey != "apache/flink-kubernetes-operator#42" {
		t.Fatalf("dependency endpoints = %s:%s -> %s:%s", row.FromKind, row.FromKey, row.ToKind, row.ToKey)
	}
	if row.ClaimUse != "topology_context" || row.ClaimGateReason != "topology_context_not_product_claim" || row.RelationshipClaimAllowed {
		t.Fatalf("canonical topology mirror should not become a relationship product claim: use=%q reason=%q allowed=%v", row.ClaimUse, row.ClaimGateReason, row.RelationshipClaimAllowed)
	}
	if len(row.Endpoints) != 2 {
		t.Fatalf("endpoint list length = %d, want 2", len(row.Endpoints))
	}
	if row.FromEndpoint.EndpointRole != "from" || row.FromEndpoint.NodeKind != "ticket" || row.FromEndpoint.ResolutionState != "resolved" {
		t.Fatalf("from endpoint = %#v, want resolved ticket endpoint", row.FromEndpoint)
	}
	if row.ToEndpoint.EndpointRole != "to" || row.ToEndpoint.NodeKind != "pull_request" || row.ToEndpoint.ResolutionState != "resolved" {
		t.Fatalf("to endpoint = %#v, want resolved pull_request endpoint", row.ToEndpoint)
	}
	if !workDependencyBadgeForTest(row.Badges, "authority:canonical_mirror") || !workDependencyBadgeForTest(row.Badges, "canonical:ticket_pull_request") {
		t.Fatalf("authority/canonical badges missing from %#v", row.Badges)
	}
}

func workDependencyBadgeForTest(rows []struct {
	Key    string `json:"key"`
	Tone   string `json:"tone"`
	Detail string `json:"detail"`
}, key string) bool {
	for _, row := range rows {
		if row.Key == key {
			return true
		}
	}
	return false
}

func TestGraphQLWorkTopologyStartsFromTypedBlockerRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 21, 7, 0, 0, 0, time.UTC)
	ticketRow := store.Client().Ticket.Create().
		SetKey("ticket:test:FLINK-9").
		SetTitle("Autoscaler release ticket").
		SetStatus(ticket.StatusOpen).
		SetExternalID("FLINK-9").
		SetSourceURL("https://issues.example.test/FLINK-9").
		SaveX(ctx)
	pr := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#9").
		SetRepository("repo/example").
		SetNumber(9).
		SetTitle("Fix autoscaler readiness").
		SetState(pullrequest.StateOpen).
		SetSourceURL("https://github.com/repo/example/pull/9").
		SaveX(ctx)
	store.Client().TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(pr).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SaveX(ctx)
	workstreamRow := store.Client().Workstream.Create().
		SetKey("workstream:flink-kubernetes-operator").
		SetTitle("Flink Kubernetes Operator").
		SetStatus(workstream.StatusActive).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_workstream").
		SetExternalID("flink-kubernetes-operator").
		SetSourceURL("https://github.com/apache/flink-kubernetes-operator").
		SetFreshnessState(workstream.FreshnessStateFresh).
		SetVisibility(workstream.VisibilityPublic).
		SetConfidence(0.9).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		SetRankScore(91).
		SaveX(ctx)
	insightEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:blocker").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_blocker").
		SetLocatorKind("github_check_runs").
		SetLocator("https://github.com/repo/example/pull/9/checks").
		SetSourceSpanKey("checks:repo/example#9").
		SetExcerpt("required autoscaler tests are failing").
		SetSourceSystem("github").
		SetSourceInstance("github.com/repo/example").
		SetExternalKind("github_check_runs").
		SetExternalID("repo/example#9").
		SetSourceURL("https://github.com/repo/example/pull/9/checks").
		SaveX(ctx)
	insight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:blocker").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityHigh).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetTitle("Required autoscaler checks are blocking merge").
		SetDetails("The PR has a high-confidence CI blocker candidate.").
		SetRecommendedAction("Assign the CI owner and confirm whether the check is required.").
		SetModelMethod("typed_check_rule").
		SetScore(91).
		SetConfidence(0.86).
		SetLatestEvidence(insightEvidence).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:blocker").
		SaveX(ctx)
	action := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:blocker").
		SetActionType(workaction.ActionTypeClearBlocker).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecision("clear_blocker").
		SetDecisionReason("high-confidence blocker candidate has complete source coverage").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetOwnerKey("team:autoscaler").
		SetOwnerSource("component_owner").
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(91).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:blocker").
		AddSourceInsights(insight).
		SaveX(ctx)
	blocker := store.Client().WorkBlocker.Create().
		SetKey("work-blocker:test:repo/example#9:ci").
		SetBlockerKind(workblocker.BlockerKindCi).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityHigh).
		SetSubjectKind(workblocker.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetWorkAction(action).
		SetWorkInsight(insight).
		SetOwnerKey("team:autoscaler").
		SetOwnerSource("component_owner").
		SetDecisionState(workblocker.DecisionStateProductAction).
		SetSourceCoverageState("observed:github_checks:complete").
		SetReviewState(workblocker.ReviewStateAccepted).
		SetTruthLabel(workblocker.TruthLabelTruePositive).
		SetActionabilityLabel(workblocker.ActionabilityLabelNeedsOwner).
		SetLabelQuality(workblocker.LabelQualityGold).
		SetMeasurementEligible(true).
		SetReviewerKind(workblocker.ReviewerKindImported).
		SetReviewerKey("codex_agent_adjudication").
		SetLabelSet("agent_gold_blockers").
		SetTitle("Required autoscaler checks are blocking merge").
		SetSummary("A high-confidence blocker candidate points at failing required checks.").
		SetRecommendedAction("Assign the autoscaler CI owner before merge.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_blocker").
		SetExternalID("tpm-action:test:blocker").
		SetSourceURL("https://github.com/repo/example/pull/9/checks").
		SetLatestEvidence(insightEvidence).
		SetEvidenceCount(1).
		SetFreshnessState(workblocker.FreshnessStateFresh).
		SetVisibility(workblocker.VisibilityPublic).
		SetConfidence(0.86).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		SetRankScore(91).
		SaveX(ctx)
	validationInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:blocker-validation").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityMedium).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetTitle("Autoscaler check blocker needs validation").
		SetDetails("The same PR also has an unmeasured blocker lead.").
		SetRecommendedAction("Validate source semantics before escalating.").
		SetModelMethod("typed_check_rule").
		SetScore(75).
		SetConfidence(0.66).
		SetLatestEvidence(insightEvidence).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:blocker-validation").
		SaveX(ctx)
	validationAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:blocker-validation").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetDecision("validate_signal").
		SetDecisionReason("source semantics still need validation").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketWatch).
		SetRankScore(75).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:blocker-validation").
		AddSourceInsights(validationInsight).
		SaveX(ctx)
	validationBlocker := store.Client().WorkBlocker.Create().
		SetKey("work-blocker:test:repo/example#9:validation").
		SetBlockerKind(workblocker.BlockerKindSourceSignal).
		SetBlockerState(workblocker.BlockerStateValidating).
		SetSeverity(workblocker.SeverityMedium).
		SetSubjectKind(workblocker.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetWorkAction(validationAction).
		SetWorkInsight(validationInsight).
		SetDecisionState(workblocker.DecisionStateValidationLead).
		SetSourceCoverageState("observed:github_checks:partial").
		SetReviewState(workblocker.ReviewStateRequested).
		SetTitle("Autoscaler check blocker needs validation").
		SetSummary("An unmeasured blocker lead should remain visible in operating reads.").
		SetRecommendedAction("Validate source semantics before escalating.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_blocker").
		SetExternalID("tpm-action:test:blocker-validation").
		SetSourceURL("https://github.com/repo/example/pull/9/checks").
		SetLatestEvidence(insightEvidence).
		SetEvidenceCount(1).
		SetFreshnessState(workblocker.FreshnessStatePartial).
		SetVisibility(workblocker.VisibilityPublic).
		SetConfidence(0.66).
		SetFirstSeenAt(now).
		SetLastActivityAt(now.Add(-1 * time.Minute)).
		SetRankScore(75).
		SaveX(ctx)
	store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:test:repo/example#9:blocker").
		SetEdgeKind(workdependencyedge.EdgeKindBlockedBy).
		SetFromKind(workdependencyedge.FromKindPullRequest).
		SetFromKey("repo/example#9").
		SetToKind(workdependencyedge.ToKindBlocker).
		SetToKey(blocker.Key).
		SetRiskSignal("ci_blocker").
		SetSourceCoverageState("observed:github_checks:complete").
		SetWorkBlockerID(blocker.ID).
		SetWorkActionID(action.ID).
		SetPullRequestID(pr.ID).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("repo/example#9->work-blocker:test:repo/example#9:ci").
		SetSourceURL("https://github.com/repo/example/pull/9/checks").
		SetLatestEvidenceID(insightEvidence.ID).
		SetEvidenceCount(1).
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetVisibility(workdependencyedge.VisibilityPublic).
		SetConfidence(0.86).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		SetRankScore(91).
		SaveX(ctx)
	store.Client().WorkBlockerImpact.Create().
		SetKey("work-blocker-impact:test:repo/example#9:workstream").
		SetImpactKind(workblockerimpact.ImpactKindWorkstream).
		SetImpactState(workblockerimpact.ImpactStateActive).
		SetImpactScore(161).
		SetSeverity(workblockerimpact.SeverityHigh).
		SetBlockerKind(workblockerimpact.BlockerKindCi).
		SetWorkBlocker(blocker).
		SetWorkAction(action).
		SetWorkstream(workstreamRow).
		SetPullRequest(pr).
		SetAffectedKind(workblockerimpact.AffectedKindWorkstream).
		SetAffectedKey("workstream:flink-kubernetes-operator").
		SetSubjectKind(workblockerimpact.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPathLength(1).
		SetSourceCoverageState("observed:github_checks:complete").
		SetTitle("Required autoscaler checks impact Flink Kubernetes Operator").
		SetSummary("The PR blocker affects the release workstream through typed topology.").
		SetRecommendedAction("Clear the CI blocker before treating the workstream as unblocked.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_blocker_impact").
		SetExternalID("work-blocker-impact:test:repo/example#9:workstream").
		SetSourceURL("https://github.com/repo/example/pull/9/checks").
		SetLatestEvidence(insightEvidence).
		SetEvidenceCount(1).
		SetFreshnessState(workblockerimpact.FreshnessStateFresh).
		SetVisibility(workblockerimpact.VisibilityPublic).
		SetConfidence(0.82).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		SetRankScore(161).
		SaveX(ctx)
	store.Client().WorkBlockerImpact.Create().
		SetKey("work-blocker-impact:test:repo/example#9:mapper-state").
		SetImpactKind(workblockerimpact.ImpactKindWorkstream).
		SetImpactState(workblockerimpact.ImpactStateValidating).
		SetImpactScore(1).
		SetSeverity(workblockerimpact.SeverityMedium).
		SetBlockerKind(workblockerimpact.BlockerKindCi).
		SetWorkBlocker(blocker).
		SetWorkAction(action).
		SetWorkstream(workstreamRow).
		SetPullRequest(pr).
		SetAffectedKind(workblockerimpact.AffectedKindWorkstream).
		SetAffectedKey("workstream:flink-kubernetes-operator:mapper").
		SetSubjectKind(workblockerimpact.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPathLength(1).
		SetSourceCoverageState("observed:github_checks:complete").
		SetTitle("Mapper regression fixture").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_blocker_impact").
		SetExternalID("work-blocker-impact:test:repo/example#9:mapper-state").
		SetFreshnessState(workblockerimpact.FreshnessStateFresh).
		SetVisibility(workblockerimpact.VisibilityPublic).
		SetConfidence(0.82).
		SetFirstSeenAt(now).
		SetLastActivityAt(now.Add(-1 * time.Minute)).
		SetRankScore(1).
		SaveX(ctx)
	otherSourceBlocker := store.Client().WorkBlocker.Create().
		SetKey("work-blocker:test:repo/example#9:other-source").
		SetBlockerKind(workblocker.BlockerKindCi).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityCritical).
		SetSubjectKind(workblocker.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetDecisionState(workblocker.DecisionStateProductAction).
		SetSourceCoverageState("observed:other-source").
		SetReviewState(workblocker.ReviewStateAccepted).
		SetTitle("Other source blocker should not leak").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_blocker").
		SetExternalID("tpm-action:test:other-source-blocker").
		SetFreshnessState(workblocker.FreshnessStateFresh).
		SetVisibility(workblocker.VisibilityPublic).
		SetConfidence(0.99).
		SetFirstSeenAt(now).
		SetLastActivityAt(now.Add(time.Minute)).
		SetRankScore(999).
		SaveX(ctx)
	store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:test:repo/example#9:other-source-blocker").
		SetEdgeKind(workdependencyedge.EdgeKindBlockedBy).
		SetFromKind(workdependencyedge.FromKindPullRequest).
		SetFromKey("repo/example#9").
		SetToKind(workdependencyedge.ToKindBlocker).
		SetToKey(otherSourceBlocker.Key).
		SetRiskSignal("other_source_blocker").
		SetSourceCoverageState("observed:other-source").
		SetWorkBlockerID(otherSourceBlocker.ID).
		SetPullRequestID(pr.ID).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("repo/example#9->work-blocker:test:repo/example#9:other-source").
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetVisibility(workdependencyedge.VisibilityPublic).
		SetConfidence(0.99).
		SetFirstSeenAt(now).
		SetLastActivityAt(now.Add(time.Minute)).
		SetRankScore(999).
		SaveX(ctx)
	store.Client().WorkBlockerImpact.Create().
		SetKey("work-blocker-impact:test:repo/example#9:other-source-workstream").
		SetImpactKind(workblockerimpact.ImpactKindWorkstream).
		SetImpactState(workblockerimpact.ImpactStateActive).
		SetImpactScore(999).
		SetSeverity(workblockerimpact.SeverityCritical).
		SetBlockerKind(workblockerimpact.BlockerKindCi).
		SetWorkBlocker(otherSourceBlocker).
		SetWorkstream(workstreamRow).
		SetPullRequest(pr).
		SetAffectedKind(workblockerimpact.AffectedKindWorkstream).
		SetAffectedKey("workstream:flink-kubernetes-operator").
		SetSubjectKind(workblockerimpact.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPathLength(1).
		SetSourceCoverageState("observed:other-source").
		SetTitle("Other source impact should not leak").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_blocker_impact").
		SetExternalID("work-blocker-impact:test:repo/example#9:other-source-workstream").
		SetFreshnessState(workblockerimpact.FreshnessStateFresh).
		SetVisibility(workblockerimpact.VisibilityPublic).
		SetConfidence(0.99).
		SetFirstSeenAt(now).
		SetLastActivityAt(now.Add(time.Minute)).
		SetRankScore(999).
		SaveX(ctx)
	crossSourceInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:cross-source-ref").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityMedium).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetTitle("Cross-source insight should not be exposed").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:cross-source-ref").
		SaveX(ctx)
	crossSourceAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:cross-source-ref").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateClosed).
		SetClosedAt(now).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketWatch).
		SetRankScore(10).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:cross-source-ref").
		AddSourceInsights(crossSourceInsight).
		SaveX(ctx)
	store.Client().WorkBlocker.Create().
		SetKey("work-blocker:test:repo/example#9:cross-source-refs").
		SetBlockerKind(workblocker.BlockerKindSourceSignal).
		SetBlockerState(workblocker.BlockerStateResolved).
		SetSeverity(workblocker.SeverityMedium).
		SetSubjectKind(workblocker.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetWorkAction(crossSourceAction).
		SetWorkInsight(crossSourceInsight).
		SetDecisionState(workblocker.DecisionStateValidationLead).
		SetReviewState(workblocker.ReviewStateRequested).
		SetTitle("Resolved blocker with cross-source refs").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_blocker").
		SetExternalID("tpm-action:test:cross-source-refs").
		SetFreshnessState(workblocker.FreshnessStateFresh).
		SetVisibility(workblocker.VisibilityPublic).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		SetRankScore(1).
		SaveX(ctx)
	crossSourceBlockerRef := store.Client().WorkBlocker.Create().
		SetKey("work-blocker:test:repo/example#9:cross-source-impact-ref").
		SetBlockerKind(workblocker.BlockerKindSourceSignal).
		SetBlockerState(workblocker.BlockerStateResolved).
		SetSeverity(workblocker.SeverityMedium).
		SetSubjectKind(workblocker.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPullRequest(pr).
		SetDecisionState(workblocker.DecisionStateValidationLead).
		SetReviewState(workblocker.ReviewStateRequested).
		SetTitle("Other-source blocker linked from test impact").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_blocker").
		SetExternalID("tpm-action:test:cross-source-impact-ref").
		SetFreshnessState(workblocker.FreshnessStateFresh).
		SetVisibility(workblocker.VisibilityPublic).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		SetRankScore(1).
		SaveX(ctx)
	store.Client().WorkBlockerImpact.Create().
		SetKey("work-blocker-impact:test:repo/example#9:cross-source-refs").
		SetImpactKind(workblockerimpact.ImpactKindWorkstream).
		SetImpactState(workblockerimpact.ImpactStateResolved).
		SetImpactScore(1).
		SetSeverity(workblockerimpact.SeverityMedium).
		SetBlockerKind(workblockerimpact.BlockerKindSourceSignal).
		SetWorkBlocker(crossSourceBlockerRef).
		SetWorkAction(crossSourceAction).
		SetWorkstream(workstreamRow).
		SetPullRequest(pr).
		SetAffectedKind(workblockerimpact.AffectedKindWorkstream).
		SetAffectedKey("workstream:flink-kubernetes-operator").
		SetSubjectKind(workblockerimpact.SubjectKindPullRequest).
		SetSubjectKey("repo/example#9").
		SetPathLength(1).
		SetTitle("Resolved impact with cross-source refs").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_work_blocker_impact").
		SetExternalID("work-blocker-impact:test:repo/example#9:cross-source-refs").
		SetFreshnessState(workblockerimpact.FreshnessStateFresh).
		SetVisibility(workblockerimpact.VisibilityPublic).
		SetFirstSeenAt(now).
		SetLastActivityAt(now).
		SetRankScore(1).
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("fixture selects the current TPM source instance for direct reads").
		SetEvaluatedAt(now.Add(time.Hour)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("test").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("work-forecast-evaluation:test:summary").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { latestSourceBlockers: workBlockers(limit: 5, blockerState: \"active\", subjectKind: \"pull_request\", subjectKey: \"repo/example#9\") { key sourceInstance } latestSourceEdges: workDependencyEdges(limit: 5, fromKind: \"pull_request\", fromKey: \"repo/example#9\", edgeKind: \"blocked_by\") { key sourceInstance } latestSourceImpacts: workBlockerImpacts(limit: 5, affectedKind: \"workstream\", workstreamKey: \"workstream:flink-kubernetes-operator\") { key sourceInstance } defaultWorkBlockers: workBlockers(limit: 5, subjectKind: \"pull_request\", subjectKey: \"repo/example#9\", sourceInstance: \"test\") { key sourceInstance blockerState } activeWorkBlockers: workBlockers(limit: 5, blockerState: \"active\", subjectKind: \"pull_request\", subjectKey: \"repo/example#9\", sourceInstance: \"test\") { key sourceInstance blockerKind blockerState severity subjectKind subjectKey title summary recommendedAction ownerKey ownerSource decisionState sourceCoverageState reviewState truthLabel actionabilityLabel labelQuality measurementEligible reviewerKind reviewerKey labelSet actionKey sourceInsightKey sourceUrl evidenceRef rankScore freshnessState visibility confidence badges { key tone detail } } crossSourceRefBlockers: workBlockers(limit: 5, blockerState: \"resolved\", subjectKind: \"pull_request\", subjectKey: \"repo/example#9\", sourceInstance: \"test\") { key sourceInstance actionKey sourceInsightKey } workDependencyEdges(limit: 5, fromKind: \"pull_request\", fromKey: \"repo/example#9\", edgeKind: \"blocked_by\", sourceInstance: \"test\") { key sourceInstance edgeKind fromKind fromKey toKind toKey riskSignal sourceCoverageState sourceUrl evidenceRef rankScore freshnessState visibility confidence badges { key tone detail } } workBlockerImpacts(limit: 5, impactState: \"active\", affectedKind: \"workstream\", workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"test\") { key sourceInstance impactKind impactState impactScore severity blockerKind blockerKey blockerState actionKey workstreamKey affectedKind affectedKey subjectKind subjectKey pathLength title summary recommendedAction sourceCoverageState sourceUrl evidenceRef rankScore freshnessState visibility confidence badges { key tone detail } } validatingMapperImpacts: workBlockerImpacts(limit: 5, impactState: \"validating\", affectedKind: \"workstream\", workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"test\") { key impactState blockerState } crossSourceRefImpacts: workBlockerImpacts(limit: 5, impactState: \"resolved\", affectedKind: \"workstream\", workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"test\") { key sourceInstance blockerKey actionKey workstreamKey } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			LatestSourceBlockers []struct {
				Key            string `json:"key"`
				SourceInstance string `json:"sourceInstance"`
			} `json:"latestSourceBlockers"`
			LatestSourceEdges []struct {
				Key            string `json:"key"`
				SourceInstance string `json:"sourceInstance"`
			} `json:"latestSourceEdges"`
			LatestSourceImpacts []struct {
				Key            string `json:"key"`
				SourceInstance string `json:"sourceInstance"`
			} `json:"latestSourceImpacts"`
			DefaultWorkBlockers []struct {
				Key            string `json:"key"`
				SourceInstance string `json:"sourceInstance"`
				BlockerState   string `json:"blockerState"`
			} `json:"defaultWorkBlockers"`
			ActiveWorkBlockers []struct {
				Key                 string  `json:"key"`
				SourceInstance      string  `json:"sourceInstance"`
				BlockerKind         string  `json:"blockerKind"`
				BlockerState        string  `json:"blockerState"`
				Severity            string  `json:"severity"`
				SubjectKind         string  `json:"subjectKind"`
				SubjectKey          string  `json:"subjectKey"`
				Title               string  `json:"title"`
				Summary             string  `json:"summary"`
				RecommendedAction   string  `json:"recommendedAction"`
				OwnerKey            string  `json:"ownerKey"`
				OwnerSource         string  `json:"ownerSource"`
				DecisionState       string  `json:"decisionState"`
				SourceCoverageState string  `json:"sourceCoverageState"`
				ReviewState         string  `json:"reviewState"`
				TruthLabel          string  `json:"truthLabel"`
				ActionabilityLabel  string  `json:"actionabilityLabel"`
				LabelQuality        string  `json:"labelQuality"`
				MeasurementEligible bool    `json:"measurementEligible"`
				ReviewerKind        string  `json:"reviewerKind"`
				ReviewerKey         string  `json:"reviewerKey"`
				LabelSet            string  `json:"labelSet"`
				ActionKey           string  `json:"actionKey"`
				SourceInsightKey    string  `json:"sourceInsightKey"`
				SourceURL           string  `json:"sourceUrl"`
				EvidenceRef         string  `json:"evidenceRef"`
				RankScore           float64 `json:"rankScore"`
				FreshnessState      string  `json:"freshnessState"`
				Visibility          string  `json:"visibility"`
				Confidence          float64 `json:"confidence"`
				Badges              []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"activeWorkBlockers"`
			CrossSourceRefBlockers []struct {
				Key              string `json:"key"`
				SourceInstance   string `json:"sourceInstance"`
				ActionKey        string `json:"actionKey"`
				SourceInsightKey string `json:"sourceInsightKey"`
			} `json:"crossSourceRefBlockers"`
			WorkDependencyEdges []struct {
				Key                 string  `json:"key"`
				SourceInstance      string  `json:"sourceInstance"`
				EdgeKind            string  `json:"edgeKind"`
				FromKind            string  `json:"fromKind"`
				FromKey             string  `json:"fromKey"`
				ToKind              string  `json:"toKind"`
				ToKey               string  `json:"toKey"`
				RiskSignal          string  `json:"riskSignal"`
				SourceCoverageState string  `json:"sourceCoverageState"`
				SourceURL           string  `json:"sourceUrl"`
				EvidenceRef         string  `json:"evidenceRef"`
				RankScore           float64 `json:"rankScore"`
				FreshnessState      string  `json:"freshnessState"`
				Visibility          string  `json:"visibility"`
				Confidence          float64 `json:"confidence"`
				Badges              []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"workDependencyEdges"`
			WorkBlockerImpacts []struct {
				Key                 string  `json:"key"`
				SourceInstance      string  `json:"sourceInstance"`
				ImpactKind          string  `json:"impactKind"`
				ImpactState         string  `json:"impactState"`
				ImpactScore         float64 `json:"impactScore"`
				Severity            string  `json:"severity"`
				BlockerKind         string  `json:"blockerKind"`
				BlockerKey          string  `json:"blockerKey"`
				BlockerState        string  `json:"blockerState"`
				ActionKey           string  `json:"actionKey"`
				WorkstreamKey       string  `json:"workstreamKey"`
				AffectedKind        string  `json:"affectedKind"`
				AffectedKey         string  `json:"affectedKey"`
				SubjectKind         string  `json:"subjectKind"`
				SubjectKey          string  `json:"subjectKey"`
				PathLength          int     `json:"pathLength"`
				Title               string  `json:"title"`
				Summary             string  `json:"summary"`
				RecommendedAction   string  `json:"recommendedAction"`
				SourceCoverageState string  `json:"sourceCoverageState"`
				SourceURL           string  `json:"sourceUrl"`
				EvidenceRef         string  `json:"evidenceRef"`
				RankScore           float64 `json:"rankScore"`
				FreshnessState      string  `json:"freshnessState"`
				Visibility          string  `json:"visibility"`
				Confidence          float64 `json:"confidence"`
				Badges              []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"workBlockerImpacts"`
			ValidatingMapperImpacts []struct {
				Key          string `json:"key"`
				ImpactState  string `json:"impactState"`
				BlockerState string `json:"blockerState"`
			} `json:"validatingMapperImpacts"`
			CrossSourceRefImpacts []struct {
				Key            string `json:"key"`
				SourceInstance string `json:"sourceInstance"`
				BlockerKey     string `json:"blockerKey"`
				ActionKey      string `json:"actionKey"`
				WorkstreamKey  string `json:"workstreamKey"`
			} `json:"crossSourceRefImpacts"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.LatestSourceBlockers) == 0 {
		t.Fatalf("latestSourceBlockers did not return current-source rows")
	}
	for _, row := range response.Data.LatestSourceBlockers {
		if row.SourceInstance != "test" {
			t.Fatalf("latestSourceBlockers leaked a non-current source row: %#v", response.Data.LatestSourceBlockers)
		}
	}
	if len(response.Data.LatestSourceEdges) == 0 {
		t.Fatalf("latestSourceEdges did not return current-source rows")
	}
	for _, row := range response.Data.LatestSourceEdges {
		if row.SourceInstance != "test" {
			t.Fatalf("latestSourceEdges leaked a non-current source row: %#v", response.Data.LatestSourceEdges)
		}
	}
	if len(response.Data.LatestSourceImpacts) == 0 {
		t.Fatalf("latestSourceImpacts did not return current-source rows")
	}
	for _, row := range response.Data.LatestSourceImpacts {
		if row.SourceInstance != "test" {
			t.Fatalf("latestSourceImpacts leaked a non-current source row: %#v", response.Data.LatestSourceImpacts)
		}
	}
	defaultStates := map[string]string{}
	for _, row := range response.Data.DefaultWorkBlockers {
		if row.SourceInstance != "test" {
			t.Fatalf("default blocker source filter leaked another source: %#v", response.Data.DefaultWorkBlockers)
		}
		defaultStates[row.Key] = row.BlockerState
	}
	if len(defaultStates) != 2 || defaultStates[blocker.Key] != "active" || defaultStates[validationBlocker.Key] != "validating" {
		t.Fatalf("default blocker read did not include active plus validating blockers: %#v", response.Data.DefaultWorkBlockers)
	}
	if len(response.Data.ActiveWorkBlockers) != 1 {
		t.Fatalf("expected one active blocker, got %#v", response.Data.ActiveWorkBlockers)
	}
	gotBlocker := response.Data.ActiveWorkBlockers[0]
	if gotBlocker.Key != blocker.Key || gotBlocker.BlockerKind != "ci" || gotBlocker.BlockerState != "active" || gotBlocker.DecisionState != "product_action" {
		t.Fatalf("unexpected blocker identity/state: %#v", gotBlocker)
	}
	if gotBlocker.SourceInstance != "test" {
		t.Fatalf("active blocker source filter leaked another source: %#v", gotBlocker)
	}
	if gotBlocker.SubjectKind != "pull_request" || gotBlocker.SubjectKey != "repo/example#9" || gotBlocker.Title == "" {
		t.Fatalf("unexpected blocker subject: %#v", gotBlocker)
	}
	if gotBlocker.ActionKey != action.Key || gotBlocker.SourceInsightKey != insight.Key {
		t.Fatalf("blocker did not expose typed action/insight refs: %#v", gotBlocker)
	}
	if gotBlocker.EvidenceRef == "" || gotBlocker.SourceURL != "https://github.com/repo/example/pull/9/checks" {
		t.Fatalf("blocker did not expose evidence/source refs: %#v", gotBlocker)
	}
	if gotBlocker.ReviewerKind != "imported" || gotBlocker.ReviewerKey != "codex_agent_adjudication" || gotBlocker.LabelSet != "agent_gold_blockers" || !gotBlocker.MeasurementEligible {
		t.Fatalf("blocker did not expose review provenance: %#v", gotBlocker)
	}
	if gotBlocker.TruthLabel != "true_positive" || gotBlocker.ActionabilityLabel != "needs_owner" || gotBlocker.LabelQuality != "gold" {
		t.Fatalf("blocker did not expose label quality: %#v", gotBlocker)
	}
	if len(response.Data.CrossSourceRefBlockers) != 1 {
		t.Fatalf("expected one resolved cross-source-ref blocker, got %#v", response.Data.CrossSourceRefBlockers)
	}
	crossSourceRefBlocker := response.Data.CrossSourceRefBlockers[0]
	if crossSourceRefBlocker.SourceInstance != "test" || crossSourceRefBlocker.ActionKey != "" || crossSourceRefBlocker.SourceInsightKey != "" {
		t.Fatalf("source-scoped blocker exposed cross-source action/insight refs: %#v", crossSourceRefBlocker)
	}
	blockerBadgeKeys := map[string]bool{}
	for _, badge := range gotBlocker.Badges {
		blockerBadgeKeys[badge.Key] = true
	}
	for _, key := range []string{"blocker:active", "decision:product_action", "review:measurement_eligible", "reviewer:imported", "truth:true_positive", "actionability:needs_owner", "coverage"} {
		if !blockerBadgeKeys[key] {
			t.Fatalf("missing blocker badge %q from %#v", key, gotBlocker.Badges)
		}
	}
	if len(response.Data.WorkDependencyEdges) != 1 {
		t.Fatalf("expected one dependency edge, got %#v", response.Data.WorkDependencyEdges)
	}
	gotEdge := response.Data.WorkDependencyEdges[0]
	if gotEdge.EdgeKind != "blocked_by" || gotEdge.FromKind != "pull_request" || gotEdge.FromKey != "repo/example#9" || gotEdge.ToKind != "blocker" || gotEdge.ToKey != blocker.Key {
		t.Fatalf("unexpected dependency edge: %#v", gotEdge)
	}
	if gotEdge.SourceInstance != "test" {
		t.Fatalf("dependency edge source filter leaked another source: %#v", gotEdge)
	}
	if gotEdge.RiskSignal != "ci_blocker" || gotEdge.SourceCoverageState != "observed:github_checks:complete" || !strings.Contains(gotEdge.EvidenceRef, "github_check_runs") {
		t.Fatalf("dependency edge did not expose risk/evidence context: %#v", gotEdge)
	}
	edgeBadgeKeys := map[string]bool{}
	for _, badge := range gotEdge.Badges {
		edgeBadgeKeys[badge.Key] = true
	}
	for _, key := range []string{"edge:blocked_by", "freshness:fresh", "risk:ci_blocker", "coverage"} {
		if !edgeBadgeKeys[key] {
			t.Fatalf("missing dependency edge badge %q from %#v", key, gotEdge.Badges)
		}
	}
	if len(response.Data.WorkBlockerImpacts) != 1 {
		t.Fatalf("expected one blocker impact, got %#v", response.Data.WorkBlockerImpacts)
	}
	gotImpact := response.Data.WorkBlockerImpacts[0]
	if gotImpact.ImpactKind != "workstream" || gotImpact.ImpactState != "active" || gotImpact.ImpactScore != 161 || gotImpact.PathLength != 1 {
		t.Fatalf("unexpected blocker impact scoring: %#v", gotImpact)
	}
	if gotImpact.SourceInstance != "test" {
		t.Fatalf("blocker impact source filter leaked another source: %#v", gotImpact)
	}
	if gotImpact.BlockerKey != blocker.Key || gotImpact.ActionKey != action.Key || gotImpact.WorkstreamKey != workstreamRow.Key {
		t.Fatalf("blocker impact did not expose topology refs: %#v", gotImpact)
	}
	if gotImpact.AffectedKind != "workstream" || gotImpact.AffectedKey != workstreamRow.Key || gotImpact.SubjectKind != "pull_request" || gotImpact.SubjectKey != "repo/example#9" {
		t.Fatalf("unexpected blocker impact endpoints: %#v", gotImpact)
	}
	if gotImpact.SourceCoverageState != "observed:github_checks:complete" || !strings.Contains(gotImpact.EvidenceRef, "github_check_runs") || gotImpact.SourceURL != "https://github.com/repo/example/pull/9/checks" {
		t.Fatalf("blocker impact did not expose evidence/source context: %#v", gotImpact)
	}
	if len(response.Data.CrossSourceRefImpacts) != 1 {
		t.Fatalf("expected one resolved cross-source-ref impact, got %#v", response.Data.CrossSourceRefImpacts)
	}
	crossSourceRefImpact := response.Data.CrossSourceRefImpacts[0]
	if crossSourceRefImpact.SourceInstance != "test" || crossSourceRefImpact.BlockerKey != "" || crossSourceRefImpact.ActionKey != "" || crossSourceRefImpact.WorkstreamKey != workstreamRow.Key {
		t.Fatalf("source-scoped impact exposed cross-source refs or hid same-source workstream: %#v", crossSourceRefImpact)
	}
	impactBadgeKeys := map[string]bool{}
	for _, badge := range gotImpact.Badges {
		impactBadgeKeys[badge.Key] = true
	}
	for _, key := range []string{"impact:workstream", "impact_state:active", "severity:high", "affected:workstream", "path:hops", "coverage"} {
		if !impactBadgeKeys[key] {
			t.Fatalf("missing blocker impact badge %q from %#v", key, gotImpact.Badges)
		}
	}
	if len(response.Data.ValidatingMapperImpacts) != 1 || response.Data.ValidatingMapperImpacts[0].ImpactState != "validating" || response.Data.ValidatingMapperImpacts[0].BlockerState != "active" {
		t.Fatalf("blocker impact did not expose linked blocker state independently from impact state: %#v", response.Data.ValidatingMapperImpacts)
	}

	topologyEvidenceBody := `{"query":"query { blockers: workBlockers(limit: 1, blockerState: \"active\", subjectKind: \"pull_request\", subjectKey: \"repo/example#9\", sourceInstance: \"test\") { key evidence { key ref claimKind claimTargetKind locatorKind locator sourceSpanKey sourceSystem sourceInstance externalKind externalId sourceUrl proofState freshnessState visibility confidence excerpt excerptTruncated } } edges: workDependencyEdges(limit: 1, fromKind: \"pull_request\", fromKey: \"repo/example#9\", edgeKind: \"blocked_by\", sourceInstance: \"test\") { key evidence { key ref claimKind claimTargetKind locatorKind locator sourceSpanKey sourceSystem sourceInstance externalKind externalId sourceUrl proofState freshnessState visibility confidence excerpt excerptTruncated } } impacts: workBlockerImpacts(limit: 1, impactState: \"active\", affectedKind: \"workstream\", workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"test\") { key evidence { key ref claimKind claimTargetKind locatorKind locator sourceSpanKey sourceSystem sourceInstance externalKind externalId sourceUrl proofState freshnessState visibility confidence excerpt excerptTruncated } } }"}`
	topologyEvidenceReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(topologyEvidenceBody))
	topologyEvidenceReq.Header.Set("Content-Type", "application/json")
	topologyEvidenceRec := httptest.NewRecorder()
	router.ServeHTTP(topologyEvidenceRec, topologyEvidenceReq)
	if topologyEvidenceRec.Code != http.StatusOK {
		t.Fatalf("expected topology evidence query status 200, got %d: %s", topologyEvidenceRec.Code, topologyEvidenceRec.Body.String())
	}
	var topologyEvidenceResponse struct {
		Data struct {
			Blockers []struct {
				Key      string                       `json:"key"`
				Evidence *workEvidenceSummaryResponse `json:"evidence"`
			} `json:"blockers"`
			Edges []struct {
				Key      string                       `json:"key"`
				Evidence *workEvidenceSummaryResponse `json:"evidence"`
			} `json:"edges"`
			Impacts []struct {
				Key      string                       `json:"key"`
				Evidence *workEvidenceSummaryResponse `json:"evidence"`
			} `json:"impacts"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(topologyEvidenceRec.Body.Bytes(), &topologyEvidenceResponse); err != nil {
		t.Fatalf("topology evidence graphql response is not JSON: %v", err)
	}
	if len(topologyEvidenceResponse.Errors) > 0 {
		t.Fatalf("topology evidence graphql response had errors: %#v", topologyEvidenceResponse.Errors)
	}
	if len(topologyEvidenceResponse.Data.Blockers) != 1 || len(topologyEvidenceResponse.Data.Edges) != 1 || len(topologyEvidenceResponse.Data.Impacts) != 1 {
		t.Fatalf("topology evidence query returned unexpected rows: %#v", topologyEvidenceResponse.Data)
	}
	for _, row := range []struct {
		kind     string
		key      string
		evidence *workEvidenceSummaryResponse
	}{
		{kind: "blocker", key: topologyEvidenceResponse.Data.Blockers[0].Key, evidence: topologyEvidenceResponse.Data.Blockers[0].Evidence},
		{kind: "edge", key: topologyEvidenceResponse.Data.Edges[0].Key, evidence: topologyEvidenceResponse.Data.Edges[0].Evidence},
		{kind: "impact", key: topologyEvidenceResponse.Data.Impacts[0].Key, evidence: topologyEvidenceResponse.Data.Impacts[0].Evidence},
	} {
		if row.evidence == nil || row.evidence.Key != insightEvidence.Key {
			t.Fatalf("%s %q did not expose the linked structured evidence: %#v", row.kind, row.key, row.evidence)
		}
		if row.evidence.ClaimKind != "object_state" || row.evidence.ClaimTargetKind != "work_blocker" || row.evidence.LocatorKind != "github_check_runs" {
			t.Fatalf("%s %q exposed unexpected evidence claim/locator: %#v", row.kind, row.key, row.evidence)
		}
		if row.evidence.SourceSystem != "github" || row.evidence.SourceInstance != "github.com/repo/example" || row.evidence.ExternalKind != "github_check_runs" {
			t.Fatalf("%s %q exposed unexpected evidence source identity: %#v", row.kind, row.key, row.evidence)
		}
		if row.evidence.ExternalID != "repo/example#9" || row.evidence.Excerpt != "required autoscaler tests are failing" || row.evidence.Ref == "" {
			t.Fatalf("%s %q exposed incomplete evidence details: %#v", row.kind, row.key, row.evidence)
		}
	}

	claimGateBody := `{"query":"query { blockers: workBlockers(limit: 5, blockerState: \"all\", subjectKind: \"pull_request\", subjectKey: \"repo/example#9\", sourceInstance: \"test\") { key claimUse claimGateReason productActionAllowed blockerClaimAllowed absenceClaimAllowed } edges: workDependencyEdges(limit: 5, fromKind: \"pull_request\", fromKey: \"repo/example#9\", edgeKind: \"blocked_by\", sourceInstance: \"test\") { key claimUse claimGateReason relationshipClaimAllowed } impacts: workBlockerImpacts(limit: 5, impactState: \"active\", affectedKind: \"workstream\", workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"test\") { key claimUse claimGateReason impactClaimAllowed } }"}`
	claimGateReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(claimGateBody))
	claimGateReq.Header.Set("Content-Type", "application/json")
	claimGateRec := httptest.NewRecorder()
	router.ServeHTTP(claimGateRec, claimGateReq)
	if claimGateRec.Code != http.StatusOK {
		t.Fatalf("expected claim gate query status 200, got %d: %s", claimGateRec.Code, claimGateRec.Body.String())
	}
	var claimGateResponse struct {
		Data struct {
			Blockers []struct {
				Key                  string `json:"key"`
				ClaimUse             string `json:"claimUse"`
				ClaimGateReason      string `json:"claimGateReason"`
				ProductActionAllowed bool   `json:"productActionAllowed"`
				BlockerClaimAllowed  bool   `json:"blockerClaimAllowed"`
				AbsenceClaimAllowed  bool   `json:"absenceClaimAllowed"`
			} `json:"blockers"`
			Edges []struct {
				Key                      string `json:"key"`
				ClaimUse                 string `json:"claimUse"`
				ClaimGateReason          string `json:"claimGateReason"`
				RelationshipClaimAllowed bool   `json:"relationshipClaimAllowed"`
			} `json:"edges"`
			Impacts []struct {
				Key                string `json:"key"`
				ClaimUse           string `json:"claimUse"`
				ClaimGateReason    string `json:"claimGateReason"`
				ImpactClaimAllowed bool   `json:"impactClaimAllowed"`
			} `json:"impacts"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(claimGateRec.Body.Bytes(), &claimGateResponse); err != nil {
		t.Fatalf("claim gate graphql response is not JSON: %v", err)
	}
	if len(claimGateResponse.Errors) > 0 {
		t.Fatalf("claim gate graphql response had errors: %#v", claimGateResponse.Errors)
	}
	blockerClaims := map[string]struct {
		claimUse             string
		claimGateReason      string
		productActionAllowed bool
		blockerClaimAllowed  bool
		absenceClaimAllowed  bool
	}{}
	for _, row := range claimGateResponse.Data.Blockers {
		blockerClaims[row.Key] = struct {
			claimUse             string
			claimGateReason      string
			productActionAllowed bool
			blockerClaimAllowed  bool
			absenceClaimAllowed  bool
		}{
			claimUse:             row.ClaimUse,
			claimGateReason:      row.ClaimGateReason,
			productActionAllowed: row.ProductActionAllowed,
			blockerClaimAllowed:  row.BlockerClaimAllowed,
			absenceClaimAllowed:  row.AbsenceClaimAllowed,
		}
	}
	activeClaim := blockerClaims[blocker.Key]
	if activeClaim.claimUse != "blocker_claim" || activeClaim.claimGateReason != "blocker_claim_gate_passed" || !activeClaim.productActionAllowed || !activeClaim.blockerClaimAllowed || activeClaim.absenceClaimAllowed {
		t.Fatalf("active blocker exposed unsafe claim gates: %#v", activeClaim)
	}
	validationClaim := blockerClaims[validationBlocker.Key]
	if validationClaim.claimUse != "source_coverage_validation" || validationClaim.claimGateReason != "source_coverage_limited" || validationClaim.productActionAllowed || validationClaim.blockerClaimAllowed || validationClaim.absenceClaimAllowed {
		t.Fatalf("validation blocker exposed unsafe claim gates: %#v", validationClaim)
	}
	if len(claimGateResponse.Data.Edges) != 1 || claimGateResponse.Data.Edges[0].ClaimUse != "blocked_by_validation" || claimGateResponse.Data.Edges[0].ClaimGateReason != "derived_dependency_edge_not_product_claim" || claimGateResponse.Data.Edges[0].RelationshipClaimAllowed {
		t.Fatalf("dependency edge did not preserve derived-topology claim gates: %#v", claimGateResponse.Data.Edges)
	}
	if len(claimGateResponse.Data.Impacts) != 1 || claimGateResponse.Data.Impacts[0].ClaimUse != "impact_claim" || claimGateResponse.Data.Impacts[0].ClaimGateReason != "impact_claim_gate_passed" || !claimGateResponse.Data.Impacts[0].ImpactClaimAllowed {
		t.Fatalf("blocker impact did not expose safe impact claim gates: %#v", claimGateResponse.Data.Impacts)
	}

	blankSourceBody := `{"query":"query { workBlockers(sourceInstance: \"   \") { key } }"}`
	blankSourceReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(blankSourceBody))
	blankSourceReq.Header.Set("Content-Type", "application/json")
	blankSourceRec := httptest.NewRecorder()
	router.ServeHTTP(blankSourceRec, blankSourceReq)
	if blankSourceRec.Code != http.StatusOK {
		t.Fatalf("expected blank source validation status 200, got %d: %s", blankSourceRec.Code, blankSourceRec.Body.String())
	}
	var blankSourceResponse struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(blankSourceRec.Body.Bytes(), &blankSourceResponse); err != nil {
		t.Fatalf("blank source graphql response is not JSON: %v", err)
	}
	if len(blankSourceResponse.Errors) == 0 || !strings.Contains(blankSourceResponse.Errors[0].Message, "sourceInstance cannot be blank") {
		t.Fatalf("blank source query did not return validation error: %s", blankSourceRec.Body.String())
	}
}

func TestGraphQLWorkLensWindowsReturnTypedWindowResults(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 6, 21, 7, 0, 0, 0, time.UTC)
	owner := store.Client().Person.Create().
		SetKey("person:test:owner").
		SetDisplayName("Fixture Owner").
		SetGithubLogin("fixture-owner").
		SetJiraAccountID("fixture-owner-jira").
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic).
		SaveX(ctx)
	codeArea := store.Client().WorkArea.Create().
		SetKey("work-area:test:code").
		SetPerson(owner).
		SetWorkAreaKind(workarea.WorkAreaKindCode).
		SetDisplayName("Code").
		SetLensCount(1).
		SetResultCount(1).
		SaveX(ctx)
	ticketArea := store.Client().WorkArea.Create().
		SetKey("work-area:test:tickets").
		SetPerson(owner).
		SetWorkAreaKind(workarea.WorkAreaKindTickets).
		SetDisplayName("Tickets").
		SetLensCount(1).
		SetResultCount(1).
		SaveX(ctx)
	prLens := store.Client().WorkLens.Create().
		SetKey("work-lens:test:pull-requests").
		SetArea(codeArea).
		SetWorkLensKind(worklens.WorkLensKindPullRequestsAuthored).
		SetLensTargetKind(worklens.LensTargetKindPullRequest).
		SetDisplayName("Fixture pull requests").
		SetResultCount(1).
		SetSourceCount(1).
		SetIsComplete(true).
		SetLastIndexedAt(now).
		SetFreshnessState(worklens.FreshnessStateFresh).
		SetVisibility(worklens.VisibilityPublic).
		SaveX(ctx)
	ticketLens := store.Client().WorkLens.Create().
		SetKey("work-lens:test:tickets").
		SetArea(ticketArea).
		SetWorkLensKind(worklens.WorkLensKindTicketsOwned).
		SetLensTargetKind(worklens.LensTargetKindTicket).
		SetDisplayName("Fixture tickets").
		SetResultCount(1).
		SetSourceCount(1).
		SetIsComplete(true).
		SetLastIndexedAt(now).
		SetFreshnessState(worklens.FreshnessStateFresh).
		SetVisibility(worklens.VisibilityPublic).
		SaveX(ctx)
	prWindow := store.Client().WorkLensWindow.Create().
		SetKey("work-lens-window:test:pull-requests").
		SetLens(prLens).
		SetLensWindowKind(worklenswindow.LensWindowKindSource).
		SetResultCount(1).
		SetIsComplete(true).
		SetSourceSystem("fixture").
		SetLastIndexedAt(now).
		SetFreshnessState(worklenswindow.FreshnessStateFresh).
		SetVisibility(worklenswindow.VisibilityPublic).
		SaveX(ctx)
	ticketWindow := store.Client().WorkLensWindow.Create().
		SetKey("work-lens-window:test:tickets").
		SetLens(ticketLens).
		SetLensWindowKind(worklenswindow.LensWindowKindSource).
		SetResultCount(1).
		SetIsComplete(true).
		SetSourceSystem("fixture").
		SetLastIndexedAt(now).
		SetFreshnessState(worklenswindow.FreshnessStateFresh).
		SetVisibility(worklenswindow.VisibilityPublic).
		SaveX(ctx)

	ticketRow := store.Client().Ticket.Create().
		SetKey("ticket:test:FLINK-1").
		SetTitle("Fixture ticket").
		SetStatus(ticket.StatusOpen).
		SetExternalID("FLINK-1").
		SetSourceURL("https://issues.example.test/FLINK-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SaveX(ctx)
	pr := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#1").
		SetRepository("repo/example").
		SetNumber(1).
		SetTitle("Fixture PR").
		SetState(pullrequest.StateOpen).
		SetSourceURL("https://github.com/repo/example/pull/1").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SaveX(ctx)
	store.Client().TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(pr).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SaveX(ctx)
	store.Client().PullRequestLensResult.Create().
		SetLens(prLens).
		SetWindow(prWindow).
		SetPullRequest(pr).
		SetRelationKind(pullrequestlensresult.RelationKindAuthored).
		SetRankScore(92).
		SetLastActivityAt(now).
		SetSourceSystem("fixture").
		SetFreshnessState(pullrequestlensresult.FreshnessStateFresh).
		SetVisibility(pullrequestlensresult.VisibilityPublic).
		SaveX(ctx)
	store.Client().TicketLensResult.Create().
		SetLens(ticketLens).
		SetWindow(ticketWindow).
		SetTicket(ticketRow).
		SetRelationKind(ticketlensresult.RelationKindOwned).
		SetRankScore(91).
		SetLastActivityAt(now).
		SetSourceSystem("fixture").
		SetFreshnessState(ticketlensresult.FreshnessStateFresh).
		SetVisibility(ticketlensresult.VisibilityPublic).
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { prWindows: workLensWindows(limit: 10, targetKind: \"pull_request\", resultLimit: 1) { key lensKind lensTargetKind displayName resultCount isComplete freshnessState pullRequestResults { subjectKey title state sourceUrl relatedTicketKeys relationKind rankScore badges { key tone } } ticketResults { subjectKey } } ticketWindows: workLensWindows(limit: 10, targetKind: \"ticket\", resultLimit: 1) { key lensKind lensTargetKind displayName resultCount isComplete freshnessState pullRequestResults { subjectKey } ticketResults { subjectKey title state sourceUrl relatedPullRequestKeys relationKind rankScore badges { key tone } } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			PRWindows     []workLensWindowResponse `json:"prWindows"`
			TicketWindows []workLensWindowResponse `json:"ticketWindows"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.PRWindows) != 1 {
		t.Fatalf("expected one PR lens window, got %#v", response.Data.PRWindows)
	}
	prGot := response.Data.PRWindows[0]
	if prGot.LensKind != "pull_requests_authored" || prGot.LensTargetKind != "pull_request" || !prGot.IsComplete {
		t.Fatalf("unexpected PR lens window: %#v", prGot)
	}
	if len(prGot.PullRequestResults) != 1 || len(prGot.TicketResults) != 0 {
		t.Fatalf("expected PR window to expose only PR results, got %#v", prGot)
	}
	prResult := prGot.PullRequestResults[0]
	if prResult.SubjectKey != "repo/example#1" || prResult.Title != "Fixture PR" || prResult.State != "open" {
		t.Fatalf("unexpected PR result subject: %#v", prResult)
	}
	if strings.Join(prResult.RelatedTicketKeys, ",") != "FLINK-1" || prResult.RelationKind != "authored" {
		t.Fatalf("unexpected PR result relations: %#v", prResult)
	}

	if len(response.Data.TicketWindows) != 1 {
		t.Fatalf("expected one ticket lens window, got %#v", response.Data.TicketWindows)
	}
	ticketGot := response.Data.TicketWindows[0]
	if ticketGot.LensKind != "tickets_owned" || ticketGot.LensTargetKind != "ticket" || !ticketGot.IsComplete {
		t.Fatalf("unexpected ticket lens window: %#v", ticketGot)
	}
	if len(ticketGot.TicketResults) != 1 || len(ticketGot.PullRequestResults) != 0 {
		t.Fatalf("expected ticket window to expose only ticket results, got %#v", ticketGot)
	}
	ticketResult := ticketGot.TicketResults[0]
	if ticketResult.SubjectKey != "FLINK-1" || ticketResult.Title != "Fixture ticket" || ticketResult.State != "open" {
		t.Fatalf("unexpected ticket result subject: %#v", ticketResult)
	}
	if strings.Join(ticketResult.RelatedPullRequestKeys, ",") != "repo/example#1" || ticketResult.RelationKind != "owned" {
		t.Fatalf("unexpected ticket result relations: %#v", ticketResult)
	}
}

type workLensWindowResponse struct {
	Key                string                         `json:"key"`
	LensKind           string                         `json:"lensKind"`
	LensTargetKind     string                         `json:"lensTargetKind"`
	DisplayName        string                         `json:"displayName"`
	ResultCount        int                            `json:"resultCount"`
	IsComplete         bool                           `json:"isComplete"`
	FreshnessState     string                         `json:"freshnessState"`
	PullRequestResults []workLensPullRequestResultRow `json:"pullRequestResults"`
	TicketResults      []workLensTicketResultRow      `json:"ticketResults"`
}

type workLensPullRequestResultRow struct {
	SubjectKey        string   `json:"subjectKey"`
	Title             string   `json:"title"`
	State             string   `json:"state"`
	SourceURL         string   `json:"sourceUrl"`
	RelatedTicketKeys []string `json:"relatedTicketKeys"`
	RelationKind      string   `json:"relationKind"`
	RankScore         float64  `json:"rankScore"`
	Badges            []struct {
		Key  string `json:"key"`
		Tone string `json:"tone"`
	} `json:"badges"`
}

type workLensTicketResultRow struct {
	SubjectKey             string   `json:"subjectKey"`
	Title                  string   `json:"title"`
	State                  string   `json:"state"`
	SourceURL              string   `json:"sourceUrl"`
	RelatedPullRequestKeys []string `json:"relatedPullRequestKeys"`
	RelationKind           string   `json:"relationKind"`
	RankScore              float64  `json:"rankScore"`
	Badges                 []struct {
		Key  string `json:"key"`
		Tone string `json:"tone"`
	} `json:"badges"`
}

func TestGraphQLWorkItemForecastsStartFromTypedForecastRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	openPR := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#72").
		SetRepository("repo/example").
		SetNumber(72).
		SetTitle("Typed forecast target").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SaveX(ctx)
	closedPR := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#73").
		SetRepository("repo/example").
		SetNumber(73).
		SetTitle("Closed forecast target").
		SetSourceURL("https://github.com/repo/example/pull/73").
		SaveX(ctx)
	forecastAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:forecast-risk").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetDecision("pending_validation").
		SetDecisionReason("forecast risk remains triage until gates pass").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(openPR).
		SetOwnerKey("github:owner").
		SetOwnerSource("pr_author").
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:forecast-risk").
		SaveX(ctx)
	store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:test:open-critical").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(openPR).
		SetSubjectState("open").
		SetForecastMethod("heuristic_percentile_rf_rejected").
		SetModelName("median_cycle_baseline").
		SetAgeDays(42.5).
		SetPredictedTotalCycleDays(11.25).
		SetPredictedRemainingDays(0).
		SetOverdueDays(31.25).
		SetRiskScore(100).
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetReadyForEta(false).
		SetReadinessReason("typed forecast gate: median-cycle baseline wins; not an ETA promise").
		SetForecastedAt(time.Date(2026, 6, 21, 6, 0, 0, 0, time.UTC)).
		SetWorkAction(forecastAction).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("repo/example#72").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SaveX(ctx)
	store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:test:stale-duplicate-open-critical").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(openPR).
		SetSubjectState("open").
		SetForecastMethod("stale_legacy_forecast").
		SetModelName("stale_model").
		SetRiskScore(1000).
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetReadinessState(workitemforecast.ReadinessStateReady).
		SetReadyForEta(true).
		SetForecastedAt(time.Date(2026, 6, 21, 5, 0, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_item_forecast").
		SetExternalID("repo/example#72").
		SaveX(ctx)
	store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:test:other-source-open-critical").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(openPR).
		SetSubjectState("open").
		SetRiskScore(1000).
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetReadinessState(workitemforecast.ReadinessStateReady).
		SetReadyForEta(true).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("repo/example#72").
		SaveX(ctx)
	store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:test:closed-critical").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindPullRequest).
		SetSubjectKey("repo/example#73").
		SetPullRequest(closedPR).
		SetSubjectState("closed").
		SetRiskScore(999).
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetReadyForEta(false).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("repo/example#73").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workItemForecasts(limit: 1, riskBand: \"critical\", sourceInstance: \"fixture-source\") { key sourceInstance forecastKind subjectKind subjectKey subjectTitle subjectUrl subjectState forecastMethod modelName ageDays predictedTotalCycleDays predictedRemainingDays overdueDays riskScore riskBand readinessState etaForecastReady etaClaimAllowed forecastClaimUse forecastClaimGateReason readinessReason actionabilityState recommendedAction interpretation evidenceRef action { key actionType decisionState ownerKey dueBucket evidenceRef } badges { key tone detail } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkItemForecasts []struct {
				Key                     string  `json:"key"`
				SourceInstance          string  `json:"sourceInstance"`
				ForecastKind            string  `json:"forecastKind"`
				SubjectKind             string  `json:"subjectKind"`
				SubjectKey              string  `json:"subjectKey"`
				SubjectTitle            string  `json:"subjectTitle"`
				SubjectURL              string  `json:"subjectUrl"`
				SubjectState            string  `json:"subjectState"`
				ForecastMethod          string  `json:"forecastMethod"`
				ModelName               string  `json:"modelName"`
				AgeDays                 float64 `json:"ageDays"`
				PredictedTotalCycleDays float64 `json:"predictedTotalCycleDays"`
				PredictedRemainingDays  float64 `json:"predictedRemainingDays"`
				OverdueDays             float64 `json:"overdueDays"`
				RiskScore               float64 `json:"riskScore"`
				RiskBand                string  `json:"riskBand"`
				ReadinessState          string  `json:"readinessState"`
				EtaForecastReady        bool    `json:"etaForecastReady"`
				EtaClaimAllowed         bool    `json:"etaClaimAllowed"`
				ForecastClaimUse        string  `json:"forecastClaimUse"`
				ForecastClaimGateReason string  `json:"forecastClaimGateReason"`
				ReadinessReason         string  `json:"readinessReason"`
				ActionabilityState      string  `json:"actionabilityState"`
				RecommendedAction       string  `json:"recommendedAction"`
				Interpretation          string  `json:"interpretation"`
				EvidenceRef             string  `json:"evidenceRef"`
				Action                  *struct {
					Key           string `json:"key"`
					ActionType    string `json:"actionType"`
					DecisionState string `json:"decisionState"`
					OwnerKey      string `json:"ownerKey"`
					DueBucket     string `json:"dueBucket"`
					EvidenceRef   string `json:"evidenceRef"`
				} `json:"action"`
				Badges []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"workItemForecasts"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkItemForecasts) != 1 {
		t.Fatalf("expected one forecast row, got %#v", response.Data.WorkItemForecasts)
	}
	forecast := response.Data.WorkItemForecasts[0]
	if forecast.Key != "work-item-forecast:test:open-critical" {
		t.Fatalf("expected default open filter to hide closed critical row, got %#v", forecast)
	}
	if forecast.SourceInstance != "fixture-source" {
		t.Fatalf("forecast source filter leaked another source: %#v", forecast)
	}
	if forecast.SubjectTitle != "Typed forecast target" || forecast.SubjectURL != "https://github.com/repo/example/pull/72" {
		t.Fatalf("forecast did not resolve typed PR display fields: %#v", forecast)
	}
	if forecast.ForecastKind != "cycle_time" || forecast.SubjectKind != "pull_request" || forecast.SubjectKey != "repo/example#72" {
		t.Fatalf("unexpected forecast subject identity: %#v", forecast)
	}
	if forecast.RiskBand != "critical" || forecast.RiskScore != 100 || forecast.ReadinessState != "gated" || forecast.EtaForecastReady {
		t.Fatalf("unexpected forecast risk/readiness fields: %#v", forecast)
	}
	if forecast.EtaClaimAllowed || forecast.ForecastClaimUse != "risk_triage_only" || forecast.ForecastClaimGateReason != "eta_forecast_gated" {
		t.Fatalf("unexpected forecast claim-safety fields: %#v", forecast)
	}
	if forecast.AgeDays != 42.5 || forecast.PredictedTotalCycleDays != 11.25 || forecast.PredictedRemainingDays != 0 || forecast.OverdueDays != 31.25 {
		t.Fatalf("unexpected forecast numeric fields: %#v", forecast)
	}
	if forecast.ActionabilityState != "owner_status_needed" || !strings.Contains(forecast.RecommendedAction, "TPM risk lead") || !strings.Contains(forecast.RecommendedAction, "do not present it as an ETA commitment") {
		t.Fatalf("unexpected forecast action guidance: %#v", forecast)
	}
	if !strings.Contains(forecast.Interpretation, "critical forecast risk") || !strings.Contains(forecast.Interpretation, "ETA-gated") || forecast.EvidenceRef != "https://github.com/repo/example/pull/72" {
		t.Fatalf("unexpected forecast interpretation/evidence: %#v", forecast)
	}
	if forecast.Action == nil || forecast.Action.Key != "tpm-action:test:forecast-risk" || forecast.Action.ActionType != "decision_or_owner_followup" || forecast.Action.DecisionState != "validation_lead" || forecast.Action.OwnerKey != "github:owner" || forecast.Action.DueBucket != "now" {
		t.Fatalf("forecast did not expose linked executable action: %#v", forecast.Action)
	}
	badgeKeys := map[string]bool{}
	for _, badge := range forecast.Badges {
		badgeKeys[badge.Key] = true
	}
	for _, key := range []string{"forecast:risk_critical", "forecast:action_owner_status", "forecast:eta_gated", "forecast:overdue"} {
		if !badgeKeys[key] {
			t.Fatalf("missing forecast badge %q from %#v", key, forecast.Badges)
		}
	}
}

func TestGraphQLForecastReadinessIsSourceScoped(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	forecastInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:forecast-risk").
		SetInsightKind(workinsight.InsightKindForecastRisk).
		SetSeverity(workinsight.SeverityHigh).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("repo/example#72").
		SetTitle("Forecast risk").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight").
		SetExternalID("forecast-risk").
		SaveX(ctx)
	store.Client().WorkAction.Create().
		SetKey("work-action:test:forecast-lead").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("repo/example#72").
		SetDueBucket(workaction.DueBucketWatch).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("forecast-lead").
		AddSourceInsights(forecastInsight).
		SaveX(ctx)
	store.Client().WorkAction.Create().
		SetKey("work-action:test:other-source-forecast-lead").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("repo/example#99").
		SetDueBucket(workaction.DueBucketWatch).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("other-source-forecast-lead").
		SaveX(ctx)
	forecastEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:forecast-readiness-source").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_forecast_evaluation").
		SetClaimField("readiness_state").
		SetLocatorKind("forecast_backtest").
		SetLocator("summary").
		SetSourceSpanKey("forecast:summary").
		SetExcerpt("fixture source remains gated").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_generated_evidence").
		SetExternalID("forecast-source-summary").
		SaveX(ctx)
	fixtureEvaluatedAt := time.Date(2026, 6, 21, 6, 15, 0, 0, time.UTC)
	otherEvaluatedAt := time.Date(2026, 6, 21, 7, 15, 0, 0, time.UTC)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:source-summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("median_cycle_baseline").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("median_cycle_baseline").
		SetBaselineSampleCount(60).
		SetOpenCandidateCount(20).
		SetObservedSnapshotTimeCount(1).
		SetTransitionCandidateCount(0).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("fixture source is not ETA-ready").
		SetEvaluatedAt(fixtureEvaluatedAt).
		SetLatestEvidence(forecastEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:other-source-ready").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("random_forest_regressor").
		SetForecastMethod("ready_model").
		SetBestModelName("random_forest_regressor").
		SetBaselineSampleCount(600).
		SetOpenCandidateCount(200).
		SetReadyForEta(true).
		SetReadinessState(workforecastevaluation.ReadinessStateReady).
		SetReadinessReason("other source is ETA-ready").
		SetEvaluatedAt(otherEvaluatedAt).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { forecastReadiness(sourceInstance: \"fixture-source\") { sourceInstance evaluatedAt etaForecastReady readinessState forecastMethod bestBacktestModel baselineSampleCount openCandidateCount typedEvaluationCount gatedForecastLeadCount readinessReason evidenceRef badges { key tone detail } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			ForecastReadiness struct {
				SourceInstance         string `json:"sourceInstance"`
				EvaluatedAt            string `json:"evaluatedAt"`
				EtaForecastReady       bool   `json:"etaForecastReady"`
				ReadinessState         string `json:"readinessState"`
				ForecastMethod         string `json:"forecastMethod"`
				BestBacktestModel      string `json:"bestBacktestModel"`
				BaselineSampleCount    int    `json:"baselineSampleCount"`
				OpenCandidateCount     int    `json:"openCandidateCount"`
				TypedEvaluationCount   int    `json:"typedEvaluationCount"`
				GatedForecastLeadCount int    `json:"gatedForecastLeadCount"`
				ReadinessReason        string `json:"readinessReason"`
				EvidenceRef            string `json:"evidenceRef"`
				Badges                 []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"forecastReadiness"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	readiness := response.Data.ForecastReadiness
	if readiness.EtaForecastReady || readiness.ReadinessState != "gated" || readiness.ForecastMethod != "typed_forecast_backtest_gate" || readiness.BestBacktestModel != "median_cycle_baseline" {
		t.Fatalf("forecast readiness leaked another source or wrong summary: %#v", readiness)
	}
	if readiness.SourceInstance != "fixture-source" || readiness.EvaluatedAt != fixtureEvaluatedAt.Format(time.RFC3339Nano) {
		t.Fatalf("forecast readiness did not expose source/evaluation identity: %#v", readiness)
	}
	if readiness.BaselineSampleCount != 60 || readiness.OpenCandidateCount != 20 || readiness.TypedEvaluationCount != 1 || readiness.GatedForecastLeadCount != 1 {
		t.Fatalf("forecast readiness counts were not source-scoped: %#v", readiness)
	}
	if !strings.Contains(readiness.EvidenceRef, "forecast_backtest") || !hasBadge(readiness.Badges, "forecast:eta_gated") || !hasBadge(readiness.Badges, "forecast:gated_leads") {
		t.Fatalf("forecast readiness missing evidence/badges: %#v", readiness)
	}
}

func TestGraphQLWorkItemStateSnapshotsReturnTypedSubjectHistory(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	pr := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#72").
		SetRepository("repo/example").
		SetNumber(72).
		SetTitle("Snapshot PR").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SaveX(ctx)
	store.Client().WorkItemStateSnapshot.Create().
		SetKey("work-item-state-snapshot:test:old").
		SetSubjectKind(workitemstatesnapshot.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetState("open").
		SetTitle("Snapshot PR").
		SetObservedAt(time.Date(2026, 6, 20, 5, 0, 0, 0, time.UTC)).
		SetCapturedAt(time.Date(2026, 6, 20, 5, 1, 0, 0, time.UTC)).
		SetSourceUpdatedAt(time.Date(2026, 6, 19, 5, 0, 0, 0, time.UTC)).
		SetAgeDays(19).
		SetStaleDays(1).
		SetRiskScore(70).
		SetRiskBand(workitemstatesnapshot.RiskBandHigh).
		SetSourceCurrentCoverageState("observed").
		SetSourceCurrentDetailState("observed").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_pr_state_snapshot").
		SetExternalID("old").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SaveX(ctx)
	latestSnapshot := store.Client().WorkItemStateSnapshot.Create().
		SetKey("work-item-state-snapshot:test:new").
		SetSubjectKind(workitemstatesnapshot.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetState("merged").
		SetTitle("Snapshot PR").
		SetObservedAt(time.Date(2026, 6, 21, 5, 0, 0, 0, time.UTC)).
		SetCapturedAt(time.Date(2026, 6, 21, 5, 1, 0, 0, time.UTC)).
		SetSourceUpdatedAt(time.Date(2026, 6, 21, 4, 0, 0, 0, time.UTC)).
		SetAgeDays(20).
		SetStaleDays(0.04).
		SetCycleTimeDays(20).
		SetRiskScore(95).
		SetRiskBand(workitemstatesnapshot.RiskBandCritical).
		SetForecastMethod("heuristic_percentile_rf_rejected").
		SetSourceCurrentCoverageState("observed").
		SetSourceCurrentDetailState("partial").
		SetSourceCurrentIssueCodes("github_403").
		SetSourceCurrentIssueKinds("detail_missing").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_pr_state_snapshot").
		SetExternalID("new").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SaveX(ctx)
	snapshotEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:state-snapshot").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_item_state_snapshot").
		SetClaimTargetID(latestSnapshot.ID).
		SetClaimField("state").
		SetLocatorKind("state_snapshot").
		SetLocator("new").
		SetSourceSpanKey("snapshot:new").
		SetExcerpt("pull_request repo/example#72 observed state merged at 2026-06-21T05:00:00Z").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_generated_evidence").
		SetExternalID("state-snapshot").
		SaveX(ctx)
	latestSnapshot.Update().SetLatestEvidence(snapshotEvidence).SetEvidenceCount(1).SaveX(ctx)
	store.Client().WorkItemStateSnapshot.Create().
		SetKey("work-item-state-snapshot:test:other-source").
		SetSubjectKind(workitemstatesnapshot.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetState("open").
		SetTitle("Other source snapshot should not leak").
		SetObservedAt(time.Date(2026, 6, 22, 5, 0, 0, 0, time.UTC)).
		SetCapturedAt(time.Date(2026, 6, 22, 5, 1, 0, 0, time.UTC)).
		SetRiskScore(999).
		SetRiskBand(workitemstatesnapshot.RiskBandCritical).
		SetSourceCurrentCoverageState("observed:other-source").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_pr_state_snapshot").
		SetExternalID("other-source").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:snapshot-source").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("fixture selects the current TPM source instance for state snapshots").
		SetEvaluatedAt(time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("snapshot-source").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workItemStateSnapshots(subjectKind: \"pull_request\", subjectKey: \"repo/example#72\", limit: 2) { key sourceInstance subjectKind subjectKey subjectTitle subjectUrl state observedAt capturedAt sourceUpdatedAt ageDays staleDays cycleTimeDays riskScore riskBand forecastMethod sourceCurrentCoverageState sourceCurrentDetailState sourceCurrentIssueCodes sourceCurrentIssueKinds linkedPullRequestCount freshPullRequestLinkCount partialPullRequestLinkCount commentCount participantCount blockerKeywordCount evidenceRef evidence { key ref claimKind claimTargetKind claimField locatorKind locator sourceSpanKey sourceSystem sourceInstance externalKind externalId proofState freshnessState visibility confidence excerpt excerptTruncated } badges { key tone detail } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkItemStateSnapshots []struct {
				Key                         string                       `json:"key"`
				SourceInstance              string                       `json:"sourceInstance"`
				SubjectKind                 string                       `json:"subjectKind"`
				SubjectKey                  string                       `json:"subjectKey"`
				SubjectTitle                string                       `json:"subjectTitle"`
				SubjectURL                  string                       `json:"subjectUrl"`
				State                       string                       `json:"state"`
				ObservedAt                  string                       `json:"observedAt"`
				CapturedAt                  string                       `json:"capturedAt"`
				SourceUpdatedAt             string                       `json:"sourceUpdatedAt"`
				AgeDays                     float64                      `json:"ageDays"`
				StaleDays                   float64                      `json:"staleDays"`
				CycleTimeDays               float64                      `json:"cycleTimeDays"`
				RiskScore                   float64                      `json:"riskScore"`
				RiskBand                    string                       `json:"riskBand"`
				ForecastMethod              string                       `json:"forecastMethod"`
				SourceCurrentCoverageState  string                       `json:"sourceCurrentCoverageState"`
				SourceCurrentDetailState    string                       `json:"sourceCurrentDetailState"`
				SourceCurrentIssueCodes     string                       `json:"sourceCurrentIssueCodes"`
				SourceCurrentIssueKinds     string                       `json:"sourceCurrentIssueKinds"`
				LinkedPullRequestCount      int                          `json:"linkedPullRequestCount"`
				FreshPullRequestLinkCount   int                          `json:"freshPullRequestLinkCount"`
				PartialPullRequestLinkCount int                          `json:"partialPullRequestLinkCount"`
				CommentCount                int                          `json:"commentCount"`
				ParticipantCount            int                          `json:"participantCount"`
				BlockerKeywordCount         int                          `json:"blockerKeywordCount"`
				EvidenceRef                 string                       `json:"evidenceRef"`
				Evidence                    *workEvidenceSummaryResponse `json:"evidence"`
				Badges                      []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"workItemStateSnapshots"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkItemStateSnapshots) != 2 {
		t.Fatalf("expected two snapshots, got %#v", response.Data.WorkItemStateSnapshots)
	}
	latest := response.Data.WorkItemStateSnapshots[0]
	if latest.Key != "work-item-state-snapshot:test:new" || latest.State != "merged" {
		t.Fatalf("expected newest snapshot first, got %#v", latest)
	}
	for _, row := range response.Data.WorkItemStateSnapshots {
		if row.SourceInstance != "fixture-source" {
			t.Fatalf("state snapshot read leaked a non-current source row: %#v", response.Data.WorkItemStateSnapshots)
		}
	}
	if latest.SubjectTitle != "Snapshot PR" || latest.SubjectURL != "https://github.com/repo/example/pull/72" {
		t.Fatalf("snapshot did not resolve typed PR display fields: %#v", latest)
	}
	if latest.RiskBand != "critical" || latest.RiskScore != 95 || latest.ForecastMethod != "heuristic_percentile_rf_rejected" {
		t.Fatalf("unexpected risk fields: %#v", latest)
	}
	if latest.SourceCurrentCoverageState != "observed" || latest.SourceCurrentDetailState != "partial" || latest.SourceCurrentIssueCodes != "github_403" {
		t.Fatalf("unexpected coverage fields: %#v", latest)
	}
	if !strings.Contains(latest.EvidenceRef, "state_snapshot") {
		t.Fatalf("snapshot did not expose generated evidence ref: %#v", latest)
	}
	if latest.Evidence == nil || latest.Evidence.Key != snapshotEvidence.Key || latest.Evidence.ClaimTargetKind != "work_item_state_snapshot" || latest.Evidence.ClaimField != "state" {
		t.Fatalf("snapshot did not expose structured evidence identity: %#v", latest.Evidence)
	}
	if latest.Evidence.LocatorKind != "state_snapshot" || latest.Evidence.SourceSystem != "cubicle_analytics" || latest.Evidence.SourceInstance != "fixture-source" || latest.Evidence.ExternalKind != "tpm_generated_evidence" {
		t.Fatalf("snapshot did not expose structured evidence source: %#v", latest.Evidence)
	}
	if latest.Evidence.ExternalID != "state-snapshot" || !strings.Contains(latest.Evidence.Excerpt, "observed state merged") || latest.Evidence.Ref == "" {
		t.Fatalf("snapshot did not expose structured evidence details: %#v", latest.Evidence)
	}
	badgeKeys := map[string]bool{}
	for _, badge := range latest.Badges {
		badgeKeys[badge.Key] = true
	}
	for _, key := range []string{"snapshot:state", "snapshot:risk_critical", "snapshot:coverage"} {
		if !badgeKeys[key] {
			t.Fatalf("missing snapshot badge %q from %#v", key, latest.Badges)
		}
	}
}

func TestGraphQLWorkItemStateTransitionsReturnCloseoutQueue(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	pr := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#72").
		SetRepository("repo/example").
		SetNumber(72).
		SetTitle("Transition PR").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SaveX(ctx)
	fromSnapshot := store.Client().WorkItemStateSnapshot.Create().
		SetKey("work-item-state-snapshot:test:transition-from").
		SetSubjectKind(workitemstatesnapshot.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetState("open").
		SetTitle("Transition PR").
		SetObservedAt(time.Date(2026, 6, 20, 5, 0, 0, 0, time.UTC)).
		SetCapturedAt(time.Date(2026, 6, 20, 5, 1, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_pr_state_snapshot").
		SetExternalID("from").
		SaveX(ctx)
	toSnapshot := store.Client().WorkItemStateSnapshot.Create().
		SetKey("work-item-state-snapshot:test:transition-to").
		SetSubjectKind(workitemstatesnapshot.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetState("merged").
		SetTitle("Transition PR").
		SetObservedAt(time.Date(2026, 6, 21, 5, 0, 0, 0, time.UTC)).
		SetCapturedAt(time.Date(2026, 6, 21, 5, 1, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_pr_state_snapshot").
		SetExternalID("to").
		SaveX(ctx)
	terminalTransition := store.Client().WorkItemStateTransition.Create().
		SetKey("work-item-state-transition:test:terminal").
		SetSubjectKind(workitemstatetransition.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetFromSnapshot(fromSnapshot).
		SetToSnapshot(toSnapshot).
		SetFromObservedAt(time.Date(2026, 6, 20, 5, 0, 0, 0, time.UTC)).
		SetToObservedAt(time.Date(2026, 6, 21, 5, 0, 0, 0, time.UTC)).
		SetFromState("open").
		SetToState("merged").
		SetTransitionKind(workitemstatetransition.TransitionKindTerminalStateChange).
		SetTransitionConfidence(0.95).
		SetConfidenceBasis(workitemstatetransition.ConfidenceBasisAdjacentSnapshotDetection).
		SetVerificationState(workitemstatetransition.VerificationStateCloseoutRequired).
		SetTerminal(true).
		SetRequiresCloseout(true).
		SetNote("Derived from adjacent snapshots; validate before treating as ground truth.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_state_transition_candidate").
		SetExternalID("transition").
		SaveX(ctx)
	transitionEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:state-transition").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_item_state_transition").
		SetClaimTargetID(terminalTransition.ID).
		SetClaimField("transition_kind").
		SetLocatorKind("state_transition").
		SetLocator("transition").
		SetSourceSpanKey("transition:terminal").
		SetExcerpt("pull_request repo/example#72: open -> merged between adjacent snapshots").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_generated_evidence").
		SetExternalID("state-transition").
		SaveX(ctx)
	terminalTransition.Update().SetLatestEvidence(transitionEvidence).SetEvidenceCount(1).SaveX(ctx)
	store.Client().WorkItemStateTransition.Create().
		SetKey("work-item-state-transition:test:non-closeout").
		SetSubjectKind(workitemstatetransition.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetFromObservedAt(time.Date(2026, 6, 19, 5, 0, 0, 0, time.UTC)).
		SetToObservedAt(time.Date(2026, 6, 20, 5, 0, 0, 0, time.UTC)).
		SetFromState("open").
		SetToState("open").
		SetTransitionKind(workitemstatetransition.TransitionKindCoverageStateChange).
		SetTransitionConfidence(0.85).
		SetTerminal(false).
		SetRequiresCloseout(false).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_state_transition_candidate").
		SetExternalID("coverage").
		SaveX(ctx)
	otherFromSnapshot := store.Client().WorkItemStateSnapshot.Create().
		SetKey("work-item-state-snapshot:test:transition-other-from").
		SetSubjectKind(workitemstatesnapshot.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetState("open").
		SetObservedAt(time.Date(2026, 6, 22, 5, 0, 0, 0, time.UTC)).
		SetCapturedAt(time.Date(2026, 6, 22, 5, 1, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_pr_state_snapshot").
		SetExternalID("other-from").
		SaveX(ctx)
	otherToSnapshot := store.Client().WorkItemStateSnapshot.Create().
		SetKey("work-item-state-snapshot:test:transition-other-to").
		SetSubjectKind(workitemstatesnapshot.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetState("closed").
		SetObservedAt(time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)).
		SetCapturedAt(time.Date(2026, 6, 22, 6, 1, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_pr_state_snapshot").
		SetExternalID("other-to").
		SaveX(ctx)
	store.Client().WorkItemStateTransition.Create().
		SetKey("work-item-state-transition:test:other-source-terminal").
		SetSubjectKind(workitemstatetransition.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetFromSnapshot(otherFromSnapshot).
		SetToSnapshot(otherToSnapshot).
		SetFromObservedAt(time.Date(2026, 6, 22, 5, 0, 0, 0, time.UTC)).
		SetToObservedAt(time.Date(2026, 6, 22, 6, 0, 0, 0, time.UTC)).
		SetFromState("open").
		SetToState("closed").
		SetTransitionKind(workitemstatetransition.TransitionKindTerminalStateChange).
		SetTransitionConfidence(0.99).
		SetConfidenceBasis(workitemstatetransition.ConfidenceBasisAdjacentSnapshotDetection).
		SetVerificationState(workitemstatetransition.VerificationStateCloseoutRequired).
		SetTerminal(true).
		SetRequiresCloseout(true).
		SetNote("Other source transition should not leak into current-source reads.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_state_transition_candidate").
		SetExternalID("other-transition").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:transition-source").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("fixture selects the current TPM source instance for state transitions").
		SetEvaluatedAt(time.Date(2026, 6, 22, 7, 0, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("transition-source").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workItemStateTransitions(limit: 5, requiresCloseout: true, transitionKind: \"terminal_state_change\") { key sourceInstance subjectKind subjectKey subjectTitle subjectUrl fromObservedAt toObservedAt fromState toState transitionKind transitionConfidence confidenceBasis verificationState terminal requiresCloseout note fromSnapshot { sourceInstance state observedAt } toSnapshot { sourceInstance state observedAt } evidenceRef evidence { key ref claimKind claimTargetKind claimField locatorKind locator sourceSpanKey sourceSystem sourceInstance externalKind externalId proofState freshnessState visibility confidence excerpt excerptTruncated } badges { key tone detail } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkItemStateTransitions []struct {
				Key                  string  `json:"key"`
				SourceInstance       string  `json:"sourceInstance"`
				SubjectKind          string  `json:"subjectKind"`
				SubjectKey           string  `json:"subjectKey"`
				SubjectTitle         string  `json:"subjectTitle"`
				SubjectURL           string  `json:"subjectUrl"`
				FromObservedAt       string  `json:"fromObservedAt"`
				ToObservedAt         string  `json:"toObservedAt"`
				FromState            string  `json:"fromState"`
				ToState              string  `json:"toState"`
				TransitionKind       string  `json:"transitionKind"`
				TransitionConfidence float64 `json:"transitionConfidence"`
				ConfidenceBasis      string  `json:"confidenceBasis"`
				VerificationState    string  `json:"verificationState"`
				Terminal             bool    `json:"terminal"`
				RequiresCloseout     bool    `json:"requiresCloseout"`
				Note                 string  `json:"note"`
				FromSnapshot         struct {
					SourceInstance string `json:"sourceInstance"`
					State          string `json:"state"`
					ObservedAt     string `json:"observedAt"`
				} `json:"fromSnapshot"`
				ToSnapshot struct {
					SourceInstance string `json:"sourceInstance"`
					State          string `json:"state"`
					ObservedAt     string `json:"observedAt"`
				} `json:"toSnapshot"`
				EvidenceRef string                       `json:"evidenceRef"`
				Evidence    *workEvidenceSummaryResponse `json:"evidence"`
				Badges      []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"workItemStateTransitions"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkItemStateTransitions) != 1 {
		t.Fatalf("expected one closeout transition, got %#v", response.Data.WorkItemStateTransitions)
	}
	transition := response.Data.WorkItemStateTransitions[0]
	if transition.Key != "work-item-state-transition:test:terminal" {
		t.Fatalf("expected closeout filter to hide non-closeout transition, got %#v", transition)
	}
	if transition.SourceInstance != "fixture-source" || transition.FromSnapshot.SourceInstance != "fixture-source" || transition.ToSnapshot.SourceInstance != "fixture-source" {
		t.Fatalf("state transition read leaked a non-current source row: %#v", transition)
	}
	if transition.SubjectTitle != "Transition PR" || transition.SubjectURL != "https://github.com/repo/example/pull/72" {
		t.Fatalf("transition did not resolve typed PR display fields: %#v", transition)
	}
	if transition.FromState != "open" || transition.ToState != "merged" || transition.TransitionKind != "terminal_state_change" {
		t.Fatalf("unexpected transition state fields: %#v", transition)
	}
	if !transition.Terminal || !transition.RequiresCloseout || transition.TransitionConfidence != 0.95 {
		t.Fatalf("unexpected transition closeout fields: %#v", transition)
	}
	if transition.ConfidenceBasis != "adjacent_snapshot_detection" || transition.VerificationState != "closeout_required" {
		t.Fatalf("unexpected transition verification semantics: %#v", transition)
	}
	if transition.FromSnapshot.State != "open" || transition.ToSnapshot.State != "merged" {
		t.Fatalf("transition did not include typed before/after snapshots: %#v", transition)
	}
	if !strings.Contains(transition.EvidenceRef, "state_transition") {
		t.Fatalf("transition did not expose generated evidence ref: %#v", transition)
	}
	if transition.Evidence == nil || transition.Evidence.Key != transitionEvidence.Key || transition.Evidence.ClaimTargetKind != "work_item_state_transition" || transition.Evidence.ClaimField != "transition_kind" {
		t.Fatalf("transition did not expose structured evidence identity: %#v", transition.Evidence)
	}
	if transition.Evidence.LocatorKind != "state_transition" || transition.Evidence.SourceSystem != "cubicle_analytics" || transition.Evidence.SourceInstance != "fixture-source" || transition.Evidence.ExternalKind != "tpm_generated_evidence" {
		t.Fatalf("transition did not expose structured evidence source: %#v", transition.Evidence)
	}
	if transition.Evidence.ExternalID != "state-transition" || !strings.Contains(transition.Evidence.Excerpt, "open -> merged") || transition.Evidence.Ref == "" {
		t.Fatalf("transition did not expose structured evidence details: %#v", transition.Evidence)
	}
	badgeKeys := map[string]bool{}
	for _, badge := range transition.Badges {
		badgeKeys[badge.Key] = true
	}
	for _, key := range []string{"transition:terminal", "transition:closeout_required", "transition:verification_closeout_required", "transition:confidence"} {
		if !badgeKeys[key] {
			t.Fatalf("missing transition badge %q from %#v", key, transition.Badges)
		}
	}
}

func TestGraphQLWorkstreamsStartFromTypedWorkstreamRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	stream := store.Client().Workstream.Create().
		SetKey("workstream:test:flink").
		SetTitle("Flink Test Stream").
		SetStatus(workstream.StatusActive).
		SetSummary("Typed workstream fixture").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream").
		SetExternalID("fixture-stream").
		SetSourceURL("https://example.test/workstream").
		SetRankScore(91).
		SaveX(ctx)
	store.Client().Workstream.Create().
		SetKey("workstream:test:other-source").
		SetTitle("Other Source Stream").
		SetStatus(workstream.StatusActive).
		SetSummary("This stream should not leak into fixture-source reads").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_workstream").
		SetExternalID("other-stream").
		SetRankScore(999).
		SaveX(ctx)
	ticket := store.Client().Ticket.Create().
		SetKey("ticket:test:FLINK-1").
		SetTitle("Example ticket").
		SetStatus(ticket.StatusOpen).
		SetExternalID("FLINK-1").
		SaveX(ctx)
	pr := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#10").
		SetRepository("repo/example").
		SetNumber(10).
		SetTitle("Example PR").
		SaveX(ctx)
	store.Client().WorkstreamTicket.Create().
		SetWorkstream(stream).
		SetTicket(ticket).
		SetWorkstreamTicketKind(workstreamticket.WorkstreamTicketKindContains).
		SetRankScore(91).
		SaveX(ctx)
	store.Client().TicketPullRequest.Create().
		SetTicket(ticket).
		SetPullRequest(pr).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetRankScore(91).
		SaveX(ctx)
	insight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:blocker-stream").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityHigh).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#10").
		SetPullRequest(pr).
		SetTitle("Possible blocker signal").
		SetScore(91).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:blocker-stream").
		SaveX(ctx)
	action := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:stream-product").
		SetActionType(workaction.ActionTypeClearBlocker).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecision("ready_for_product_action").
		SetDecisionReason("measurement gate passed").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#10").
		SetPullRequest(pr).
		SetOwnerKey("github:owner").
		SetOwnerSource("pr_author").
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(91).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:stream-product").
		AddSourceInsights(insight).
		SaveX(ctx)
	store.Client().WorkActionObservation.Create().
		SetKey("work-action-observation:test:stream-product").
		SetAction(action).
		SetObservationKind(workactionobservation.ObservationKindSourceState).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSupportsAction(true).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action_observation").
		SetExternalID("work-action-observation:test:stream-product").
		SaveX(ctx)
	store.Client().WorkAction.Create().
		SetKey("tpm-action:test:stream-other-source").
		SetActionType(workaction.ActionTypeClearBlocker).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#10").
		SetPullRequest(pr).
		SetOwnerKey("github:other-source").
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(999).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:stream-other-source").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workstreams(sourceInstance: \"fixture-source\") { key sourceInstance title status summary sourceUrl ticketCount actionItemCount productActionCount validationLeadCount nowCount topRiskScore badges { key tone detail } forecastReadiness { sourceInstance readinessState etaForecastReady } topActions { key sourceInstance actionType decisionState subjectKey } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Workstreams []struct {
				Key                 string  `json:"key"`
				SourceInstance      string  `json:"sourceInstance"`
				Title               string  `json:"title"`
				Status              string  `json:"status"`
				Summary             string  `json:"summary"`
				SourceURL           string  `json:"sourceUrl"`
				TicketCount         int     `json:"ticketCount"`
				ActionItemCount     int     `json:"actionItemCount"`
				ProductActionCount  int     `json:"productActionCount"`
				ValidationLeadCount int     `json:"validationLeadCount"`
				NowCount            int     `json:"nowCount"`
				TopRiskScore        float64 `json:"topRiskScore"`
				Badges              []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
				ForecastReadiness struct {
					SourceInstance   string `json:"sourceInstance"`
					ReadinessState   string `json:"readinessState"`
					EtaForecastReady bool   `json:"etaForecastReady"`
				} `json:"forecastReadiness"`
				TopActions []struct {
					Key            string `json:"key"`
					SourceInstance string `json:"sourceInstance"`
					ActionType     string `json:"actionType"`
					DecisionState  string `json:"decisionState"`
					SubjectKey     string `json:"subjectKey"`
				} `json:"topActions"`
			} `json:"workstreams"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.Workstreams) != 1 {
		t.Fatalf("expected one workstream, got %#v", response.Data.Workstreams)
	}
	got := response.Data.Workstreams[0]
	if got.Key != "workstream:test:flink" || got.SourceInstance != "fixture-source" || got.Title != "Flink Test Stream" || got.Status != "active" || got.TicketCount != 1 {
		t.Fatalf("unexpected workstream identity/counts: %#v", got)
	}
	if got.ForecastReadiness.SourceInstance != "fixture-source" {
		t.Fatalf("workstream forecast readiness did not stay source-scoped: %#v", got.ForecastReadiness)
	}
	if got.ActionItemCount != 1 || got.ProductActionCount != 1 || got.ValidationLeadCount != 0 || got.NowCount != 1 || got.TopRiskScore != 91 {
		t.Fatalf("unexpected workstream action fields: %#v", got)
	}
	if !hasBadge(got.Badges, "workstream:tickets") || !hasBadge(got.Badges, "workstream:product_actions") {
		t.Fatalf("expected workstream badges, got %#v", got.Badges)
	}
	if len(got.TopActions) != 1 || got.TopActions[0].Key != "tpm-action:test:stream-product" || got.TopActions[0].SourceInstance != "fixture-source" || got.TopActions[0].SubjectKey != "repo/example#10" {
		t.Fatalf("unexpected workstream top actions: %#v", got.TopActions)
	}
	blankSourceBody := `{"query":"query { workstreams(sourceInstance: \"   \") { key } }"}`
	blankSourceReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(blankSourceBody))
	blankSourceReq.Header.Set("Content-Type", "application/json")
	blankSourceRec := httptest.NewRecorder()
	router.ServeHTTP(blankSourceRec, blankSourceReq)
	var blankSourceResponse struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(blankSourceRec.Body.Bytes(), &blankSourceResponse); err != nil {
		t.Fatalf("blank workstream source graphql response is not JSON: %v", err)
	}
	if len(blankSourceResponse.Errors) == 0 || !strings.Contains(blankSourceResponse.Errors[0].Message, "sourceInstance cannot be blank") {
		t.Fatalf("blank workstream source query did not return validation error: %s", blankSourceRec.Body.String())
	}
}

func TestGraphQLWorkProgramItemsStartFromTypedProgramRows(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	stream := store.Client().Workstream.Create().
		SetKey("workstream:flink-kubernetes-operator").
		SetTitle("Flink Kubernetes Operator").
		SetStatus(workstream.StatusActive).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream").
		SetExternalID("flink-kubernetes-operator").
		SaveX(ctx)
	pr := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#72").
		SetRepository("repo/example").
		SetNumber(72).
		SetTitle("Improve autoscaler").
		SetState(pullrequest.StateOpen).
		SetSourceURL("https://github.com/repo/example/pull/72").
		SaveX(ctx)
	unrelatedPR := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#999").
		SetRepository("repo/example").
		SetNumber(999).
		SetTitle("Unrelated PR").
		SetState(pullrequest.StateOpen).
		SetSourceURL("https://github.com/repo/example/pull/999").
		SaveX(ctx)
	action := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:program").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecision("ready_for_product_action").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetOwnerKey("github:owner").
		SetOwnerSource("pr_author").
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(91.5).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:program").
		SaveX(ctx)
	validationAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:program-validation").
		SetActionType(workaction.ActionTypeValidateSignal).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketThisWeek).
		SetRankScore(80).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:program-validation").
		SaveX(ctx)
	otherAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:program:other-source").
		SetActionType(workaction.ActionTypeRefreshSource).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateSourceRepair).
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(999).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:program:other-source").
		SaveX(ctx)
	item := store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:test:fixture").
		SetWorkstream(stream).
		SetWorkAction(action).
		SetPullRequest(pr).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetSubjectKind(workprogramitem.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetLinkedTicketKeys("FLINK-1, FLINK-2").
		SetLinkedPullRequestKeys("repo/example#73").
		SetTitle("Improve autoscaler").
		SetProgramStatus(workprogramitem.ProgramStatusNeedsDecision).
		SetTpmBucket(workprogramitem.TpmBucketRisk).
		SetOwnerKey("github:owner").
		SetOwnerSource("pr_author").
		SetAuthorDri("github:owner").
		SetRequestedReviewerKeys("github:reviewer").
		SetReviewerOrApprover("github:reviewer").
		SetNextAction("Ask owner whether to merge or park.").
		SetDecisionNeeded("merge / close / park / assign owner").
		SetDecisionState(workprogramitem.DecisionStateProductAction).
		SetDecisionGateReason("gold labels support escalation").
		SetDueBucket(workprogramitem.DueBucketNow).
		SetLastSourceUpdateAt(time.Date(2026, 6, 20, 4, 0, 0, 0, time.UTC)).
		SetAgeDays(4.5).
		SetStaleDays(1.25).
		SetRiskScore(91.5).
		SetBlockerLabelState("not_required").
		SetCiSignal("required_failing_or_pending").
		SetTransitionState("open").
		SetDependencySummary("2 linked ticket(s); 1 linked PR(s)").
		SetSourceCoverageState("observed:github_pr").
		SetLabelQuality("gold").
		SetRegisterUpdatedAt(time.Date(2026, 6, 21, 4, 0, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_program_item").
		SetExternalID("tpm-program:test").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SetFreshnessState(workprogramitem.FreshnessStateFresh).
		SetVisibility(workprogramitem.VisibilityPublic).
		SetConfidence(0.9).
		SetEventCount(1).
		SetFirstSeenAt(time.Date(2026, 6, 21, 4, 0, 0, 0, time.UTC)).
		SetLastActivityAt(time.Date(2026, 6, 21, 4, 0, 0, 0, time.UTC)).
		SetRankScore(91.5).
		SaveX(ctx)
	store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:test:validation").
		SetWorkstream(stream).
		SetWorkAction(validationAction).
		SetPullRequest(pr).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetSubjectKind(workprogramitem.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetTitle("Validate autoscaler blocker signal").
		SetProgramStatus(workprogramitem.ProgramStatusValidateSignal).
		SetTpmBucket(workprogramitem.TpmBucketRiskValidation).
		SetNextAction("Validate generated blocker signal.").
		SetDecisionNeeded("validate / suppress / escalate").
		SetDecisionState(workprogramitem.DecisionStateValidationLead).
		SetDueBucket(workprogramitem.DueBucketThisWeek).
		SetRiskScore(80).
		SetSourceCoverageState("anonymous_success:public_api_current_observation").
		SetRegisterUpdatedAt(time.Date(2026, 6, 21, 4, 5, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_program_item").
		SetExternalID("tpm-program:validation").
		SetFreshnessState(workprogramitem.FreshnessStatePartial).
		SetVisibility(workprogramitem.VisibilityPublic).
		SetConfidence(0.75).
		SetEventCount(1).
		SetRankScore(80).
		SaveX(ctx)
	store.Client().WorkProgramItem.Create().
		SetKey("work-program-item:test:other-source").
		SetWorkstream(stream).
		SetWorkAction(otherAction).
		SetPullRequest(pr).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetSubjectKind(workprogramitem.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetTitle("Other source row").
		SetProgramStatus(workprogramitem.ProgramStatusSourceRepair).
		SetTpmBucket(workprogramitem.TpmBucketSourceRepair).
		SetDecisionState(workprogramitem.DecisionStateSourceRepair).
		SetDueBucket(workprogramitem.DueBucketNow).
		SetRiskScore(999).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_program_item").
		SetExternalID("tpm-program:other").
		SaveX(ctx)
	itemEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:program-item").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_program_item").
		SetClaimTargetID(item.ID).
		SetClaimField("program_status").
		SetLocatorKind("tpm_program_register").
		SetLocator("tpm-program:test").
		SetSourceSpanKey("program-register:test").
		SetExcerpt("pull_request repo/example#72 in needs_decision/risk: Ask owner whether to merge or park.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_generated_evidence").
		SetExternalID("program-item").
		SaveX(ctx)
	item.Update().SetLatestEvidence(itemEvidence).SetEvidenceCount(1).SaveX(ctx)
	ownerLoadAt := time.Date(2026, 6, 21, 7, 5, 0, 0, time.UTC)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:program:owner").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("github:owner").
		SetOwnerDisplayName("owner").
		SetGeneratedAt(ownerLoadAt).
		SetLoadStatus(workownerloadsnapshot.LoadStatusOverloaded).
		SetActionCount(2).
		SetProductActionCount(1).
		SetValidationLeadCount(1).
		SetCriticalOrHighCount(1).
		SetMaxPriorityScore(91.5).
		SetAvgPriorityScore(85.75).
		SetDecisionFollowupCount(1).
		SetValidateSignalCount(1).
		SetNeedsHumanReviewCount(1).
		SetTopActionType("decision_or_owner_followup").
		SetTopSubjects("repo/example#72").
		SetRecommendedFocus("Rebalance owner load before treating the program plan as executable.").
		SetLatestEvidence(itemEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:05:00Z:github:owner").
		SetFreshnessState(workownerloadsnapshot.FreshnessStateFresh).
		SetVisibility(workownerloadsnapshot.VisibilityPublic).
		SetConfidence(1.0).
		SetRankScore(91.5).
		SaveX(ctx)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:program:unassigned").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("(unassigned)").
		SetGeneratedAt(ownerLoadAt).
		SetLoadStatus(workownerloadsnapshot.LoadStatusWatch).
		SetActionCount(1).
		SetModelOrRuleQaCount(1).
		SetMaxPriorityScore(72).
		SetAvgPriorityScore(72).
		SetTopActionType("model_quality_review").
		SetRecommendedFocus("Assign generated QA work before autonomous execution.").
		SetLatestEvidence(itemEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:05:00Z:(unassigned)").
		SetFreshnessState(workownerloadsnapshot.FreshnessStatePartial).
		SetVisibility(workownerloadsnapshot.VisibilityPublic).
		SetConfidence(0.85).
		SetRankScore(72).
		SaveX(ctx)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:program:old").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("github:old").
		SetGeneratedAt(time.Date(2026, 6, 20, 7, 5, 0, 0, time.UTC)).
		SetLoadStatus(workownerloadsnapshot.LoadStatusOverloaded).
		SetActionCount(9).
		SetMaxPriorityScore(100).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-20T07:05:00Z:github:old").
		SetFreshnessState(workownerloadsnapshot.FreshnessStateFresh).
		SetVisibility(workownerloadsnapshot.VisibilityPublic).
		SetConfidence(1.0).
		SetRankScore(100).
		SaveX(ctx)
	store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:test:program-critical").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetSubjectState("open").
		SetForecastMethod("heuristic_percentile_rf_rejected").
		SetModelName("median_cycle_baseline").
		SetAgeDays(42.5).
		SetPredictedTotalCycleDays(11.25).
		SetPredictedRemainingDays(0).
		SetOverdueDays(31.25).
		SetRiskScore(97).
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetReadyForEta(false).
		SetReadinessReason("ETA forecast is gated: median-cycle baseline wins; not an ETA promise.").
		SetForecastedAt(time.Date(2026, 6, 21, 6, 0, 0, 0, time.UTC)).
		SetWorkAction(action).
		SetLatestEvidence(itemEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("repo/example#72").
		SetSourceURL("https://github.com/repo/example/pull/72").
		SetFreshnessState(workitemforecast.FreshnessStateFresh).
		SetVisibility(workitemforecast.VisibilityPublic).
		SetConfidence(0.9).
		SetRankScore(97).
		SaveX(ctx)
	store.Client().WorkItemForecast.Create().
		SetKey("work-item-forecast:test:unrelated-later").
		SetForecastKind(workitemforecast.ForecastKindCycleTime).
		SetSubjectKind(workitemforecast.SubjectKindPullRequest).
		SetSubjectKey("repo/example#999").
		SetPullRequest(unrelatedPR).
		SetSubjectState("open").
		SetForecastMethod("heuristic_percentile_rf_rejected").
		SetModelName("median_cycle_baseline").
		SetAgeDays(60).
		SetPredictedTotalCycleDays(10).
		SetPredictedRemainingDays(0).
		SetOverdueDays(50).
		SetRiskScore(100).
		SetRiskBand(workitemforecast.RiskBandCritical).
		SetReadinessState(workitemforecast.ReadinessStateGated).
		SetReadyForEta(false).
		SetReadinessReason("Unrelated workstream forecast must not displace scoped rows.").
		SetForecastedAt(time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)).
		SetLatestEvidence(itemEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_pr_forecast").
		SetExternalID("repo/example#999").
		SetSourceURL("https://github.com/repo/example/pull/999").
		SetFreshnessState(workitemforecast.FreshnessStateFresh).
		SetVisibility(workitemforecast.VisibilityPublic).
		SetConfidence(0.9).
		SetRankScore(100).
		SaveX(ctx)
	blocker := store.Client().WorkBlocker.Create().
		SetKey("work-blocker:test:program").
		SetBlockerKind(workblocker.BlockerKindDecision).
		SetBlockerState(workblocker.BlockerStateActive).
		SetSeverity(workblocker.SeverityHigh).
		SetSubjectKind(workblocker.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPullRequest(pr).
		SetWorkAction(action).
		SetOwnerKey("github:owner").
		SetOwnerSource("pr_author").
		SetDecisionState(workblocker.DecisionStateProductAction).
		SetSourceCoverageState("observed:github_pr").
		SetReviewState(workblocker.ReviewStateAccepted).
		SetTitle("Owner decision is blocking progress").
		SetSummary("A product decision blocks the program item.").
		SetRecommendedAction("Ask owner whether to merge or park.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_blocker").
		SetExternalID("work-blocker:test:program").
		SetLatestEvidence(itemEvidence).
		SetEvidenceCount(1).
		SetFreshnessState(workblocker.FreshnessStateFresh).
		SetVisibility(workblocker.VisibilityPublic).
		SetConfidence(0.9).
		SetRankScore(91.5).
		SaveX(ctx)
	store.Client().WorkBlockerImpact.Create().
		SetKey("work-blocker-impact:test:program").
		SetImpactKind(workblockerimpact.ImpactKindWorkstream).
		SetImpactState(workblockerimpact.ImpactStateActive).
		SetImpactScore(91.5).
		SetSeverity(workblockerimpact.SeverityHigh).
		SetBlockerKind(workblockerimpact.BlockerKindDecision).
		SetWorkBlocker(blocker).
		SetWorkAction(action).
		SetWorkstream(stream).
		SetPullRequest(pr).
		SetAffectedKind(workblockerimpact.AffectedKindWorkstream).
		SetAffectedKey("workstream:flink-kubernetes-operator").
		SetSubjectKind(workblockerimpact.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetPathLength(1).
		SetSourceCoverageState("observed:github_pr").
		SetTitle("Owner decision impacts Flink Kubernetes Operator").
		SetSummary("The program blocker affects the workstream.").
		SetRecommendedAction("Resolve owner decision.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_blocker_impact").
		SetExternalID("work-blocker-impact:test:program").
		SetLatestEvidence(itemEvidence).
		SetEvidenceCount(1).
		SetFreshnessState(workblockerimpact.FreshnessStateFresh).
		SetVisibility(workblockerimpact.VisibilityPublic).
		SetConfidence(0.9).
		SetRankScore(91.5).
		SaveX(ctx)
	store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:test:blocked-by").
		SetEdgeKind(workdependencyedge.EdgeKindBlockedBy).
		SetFromKind(workdependencyedge.FromKindPullRequest).
		SetFromKey("repo/example#72").
		SetToKind(workdependencyedge.ToKindBlocker).
		SetToKey(blocker.Key).
		SetRiskSignal("owner_decision").
		SetSourceCoverageState("observed:github_pr").
		SetWorkstreamID(stream.ID).
		SetWorkBlockerID(blocker.ID).
		SetWorkActionID(action.ID).
		SetPullRequestID(pr.ID).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("work-dependency-edge:test:blocked-by").
		SetLatestEvidenceID(itemEvidence.ID).
		SetEvidenceCount(1).
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetVisibility(workdependencyedge.VisibilityPublic).
		SetConfidence(0.9).
		SetRankScore(91.5).
		SaveX(ctx)
	store.Client().WorkDependencyEdge.Create().
		SetKey("work-dependency-edge:test:needs-action").
		SetEdgeKind(workdependencyedge.EdgeKindNeedsAction).
		SetFromKind(workdependencyedge.FromKindBlocker).
		SetFromKey(blocker.Key).
		SetToKind(workdependencyedge.ToKindAction).
		SetToKey(action.Key).
		SetRiskSignal("owner_decision").
		SetSourceCoverageState("observed:github_pr").
		SetWorkstreamID(stream.ID).
		SetWorkBlockerID(blocker.ID).
		SetWorkActionID(action.ID).
		SetPullRequestID(pr.ID).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_dependency_edge").
		SetExternalID("work-dependency-edge:test:needs-action").
		SetLatestEvidenceID(itemEvidence.ID).
		SetEvidenceCount(1).
		SetFreshnessState(workdependencyedge.FreshnessStateFresh).
		SetVisibility(workdependencyedge.VisibilityPublic).
		SetConfidence(0.9).
		SetRankScore(91.5).
		SaveX(ctx)
	standupAt := time.Date(2026, 6, 21, 7, 0, 0, 0, time.UTC)
	store.Client().WorkstreamStandupSection.Create().
		SetKey("workstream-standup-section:test:program").
		SetWorkstream(stream).
		SetWorkAction(action).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(standupAt).
		SetSectionRank(1).
		SetSectionKind(workstreamstandupsection.SectionKindProductAction).
		SetUrgency(workstreamstandupsection.UrgencyHigh).
		SetOwnerKey("github:owner").
		SetSubjectKind(workstreamstandupsection.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetActionType("decision_or_owner_followup").
		SetStatusSignal("owner_decision").
		SetSummary("Owner decision is blocking progress.").
		SetRecommendedAction("Ask owner whether to merge or park.").
		SetEvidenceRef("tpm_program_register:fixture").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream_standup_section").
		SetExternalID("standup:test:program").
		SetLatestEvidence(itemEvidence).
		SetEvidenceCount(1).
		SetFreshnessState(workstreamstandupsection.FreshnessStateFresh).
		SetVisibility(workstreamstandupsection.VisibilityPublic).
		SetConfidence(0.9).
		SaveX(ctx)
	store.Client().WorkstreamStandupSection.Create().
		SetKey("workstream-standup-section:test:validation").
		SetWorkstream(stream).
		SetWorkAction(validationAction).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(standupAt).
		SetSectionRank(2).
		SetSectionKind(workstreamstandupsection.SectionKindValidationLead).
		SetUrgency(workstreamstandupsection.UrgencyMedium).
		SetSubjectKind(workstreamstandupsection.SubjectKindPullRequest).
		SetSubjectKey("repo/example#72").
		SetActionType("validate_signal").
		SetStatusSignal("anonymous_success:public_api_current_observation").
		SetSummary("Validate autoscaler blocker signal.").
		SetRecommendedAction("Validate generated blocker signal.").
		SetEvidenceRef("tpm_program_register:validation").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream_standup_section").
		SetExternalID("standup:test:validation").
		SetLatestEvidence(itemEvidence).
		SetEvidenceCount(1).
		SetFreshnessState(workstreamstandupsection.FreshnessStatePartial).
		SetVisibility(workstreamstandupsection.VisibilityPublic).
		SetConfidence(0.75).
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workProgramItems(limit: 10, workstreamKey: \"workstream:flink-kubernetes-operator\", programStatus: \"needs_decision\", sourceInstance: \"fixture-source\") { key sourceInstance workstreamKey subjectKind subjectKey linkedTicketKeys linkedPullRequestKeys title programStatus tpmBucket ownerKey ownerSource authorDri requestedReviewerKeys reviewerOrApprover nextAction decisionNeeded decisionState decisionGateReason claimUse claimGateReason productActionAllowed absenceClaimAllowed etaClaimAllowed dueBucket lastSourceUpdateAt ageDays staleDays riskScore blockerLabelState ciSignal transitionState dependencySummary sourceCoverageState labelQuality evidenceRef badges { key tone detail } action { key sourceInstance decisionState claimUse claimGateReason productActionAllowed absenceClaimAllowed etaClaimAllowed } } workProgramSummary(workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"fixture-source\") { sourceInstance totalCount needsDecisionCount validateSignalCount ciFailingCount waitingReviewCount sourceRepairCount closedPendingReviewCount modelQualityCount closureCandidateCount dismissedCount nowCount highRiskCount unassignedCount productActionCount validationLeadCount sourceCoverageLimitedCount ownerLoadStatus ownerLoadActionCount overloadedOwnerCount attentionOwnerCount unassignedActionCount blockerCount activeBlockerCount validatingBlockerCount blockerImpactCount activeBlockerImpactCount dependencyEdgeCount blockingDependencyCount needsActionDependencyCount operatingStatus decisionPressure forecastState primaryRisk recommendedFocus capabilityGaps forecastReadiness { sourceInstance readinessState etaForecastReady } badges { key tone detail } breakdowns { dimension key count } ownerRollups { ownerKey ownerSource itemCount needsDecisionCount validateSignalCount nowCount highRiskCount maxRiskScore badges { key tone detail } topItems { key programStatus riskScore } } topOwnerLoads { ownerKey loadStatus actionCount maxPriorityScore evidenceRef badges { key tone detail } } topItems { key programStatus riskScore } topBlockers { key blockerState subjectKey actionKey evidenceRef } topBlockerImpacts { key impactState blockerKey actionKey affectedKey evidenceRef } topDependencies { key edgeKind fromKey toKey evidenceRef } topForecasts { key subjectKey riskBand actionabilityState recommendedAction evidenceRef riskScore action { key decisionState } badges { key tone detail } } } workProgramBrief(workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"fixture-source\") { sourceInstance workstreamKey generatedAt operatingStatus decisionPressure forecastState primaryRisk executiveSummary recommendedFocus nextCadenceFocus capabilityGaps summary { totalCount operatingStatus activeBlockerCount ownerLoadStatus overloadedOwnerCount unassignedActionCount } standupSections { sectionKind urgency subjectKey action { key sourceInstance } } immediateActions { sectionKind urgency subjectKey } validationQueue { sectionKind urgency subjectKey } riskDrivers { driverKind key status subjectKey title recommendedAction evidenceRef rankScore } riskDriverBuckets { driverKind title riskDriverCount topRiskDrivers { driverKind key status subjectKey rankScore } } forecastReadiness { readinessState etaForecastReady } insightEvaluation { currentInsightCount measurementLabelCount openReviewRequestCount readyToMeasurePrecision readyToMeasureActionability gatedInsightKindCount } tpmFunctionReadiness { functionKey functionName readinessState automationState humanRequired supportingSignalCount blockingGateKeys detail recommendedAction } adversarialChecks { key checkKind checkState severity title detail recommendedAction blockingGateKeys evidenceRefs } qualityGates { key gateState blocking detail recommendedAction } caveats { key severity title detail recommendedAction evidenceRef } badges { key tone detail } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkProgramItems []struct {
				Key                   string   `json:"key"`
				SourceInstance        string   `json:"sourceInstance"`
				WorkstreamKey         string   `json:"workstreamKey"`
				SubjectKind           string   `json:"subjectKind"`
				SubjectKey            string   `json:"subjectKey"`
				LinkedTicketKeys      []string `json:"linkedTicketKeys"`
				LinkedPullRequestKeys []string `json:"linkedPullRequestKeys"`
				ProgramStatus         string   `json:"programStatus"`
				TpmBucket             string   `json:"tpmBucket"`
				OwnerKey              string   `json:"ownerKey"`
				NextAction            string   `json:"nextAction"`
				DecisionState         string   `json:"decisionState"`
				DecisionGateReason    string   `json:"decisionGateReason"`
				ClaimUse              string   `json:"claimUse"`
				ClaimGateReason       string   `json:"claimGateReason"`
				ProductActionAllowed  bool     `json:"productActionAllowed"`
				AbsenceClaimAllowed   bool     `json:"absenceClaimAllowed"`
				EtaClaimAllowed       bool     `json:"etaClaimAllowed"`
				DueBucket             string   `json:"dueBucket"`
				RiskScore             float64  `json:"riskScore"`
				EvidenceRef           string   `json:"evidenceRef"`
				Badges                []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
				Action struct {
					Key                  string `json:"key"`
					SourceInstance       string `json:"sourceInstance"`
					DecisionState        string `json:"decisionState"`
					ClaimUse             string `json:"claimUse"`
					ClaimGateReason      string `json:"claimGateReason"`
					ProductActionAllowed bool   `json:"productActionAllowed"`
					AbsenceClaimAllowed  bool   `json:"absenceClaimAllowed"`
					EtaClaimAllowed      bool   `json:"etaClaimAllowed"`
				} `json:"action"`
			} `json:"workProgramItems"`
			WorkProgramSummary struct {
				SourceInstance             string   `json:"sourceInstance"`
				TotalCount                 int      `json:"totalCount"`
				NeedsDecisionCount         int      `json:"needsDecisionCount"`
				ValidateSignalCount        int      `json:"validateSignalCount"`
				NowCount                   int      `json:"nowCount"`
				HighRiskCount              int      `json:"highRiskCount"`
				UnassignedCount            int      `json:"unassignedCount"`
				ProductActionCount         int      `json:"productActionCount"`
				ValidationLeadCount        int      `json:"validationLeadCount"`
				SourceCoverageLimitedCount int      `json:"sourceCoverageLimitedCount"`
				OwnerLoadStatus            string   `json:"ownerLoadStatus"`
				OwnerLoadActionCount       int      `json:"ownerLoadActionCount"`
				OverloadedOwnerCount       int      `json:"overloadedOwnerCount"`
				AttentionOwnerCount        int      `json:"attentionOwnerCount"`
				UnassignedActionCount      int      `json:"unassignedActionCount"`
				BlockerCount               int      `json:"blockerCount"`
				ActiveBlockerCount         int      `json:"activeBlockerCount"`
				ValidatingBlockerCount     int      `json:"validatingBlockerCount"`
				BlockerImpactCount         int      `json:"blockerImpactCount"`
				ActiveBlockerImpactCount   int      `json:"activeBlockerImpactCount"`
				DependencyEdgeCount        int      `json:"dependencyEdgeCount"`
				BlockingDependencyCount    int      `json:"blockingDependencyCount"`
				NeedsActionDependencyCount int      `json:"needsActionDependencyCount"`
				OperatingStatus            string   `json:"operatingStatus"`
				DecisionPressure           string   `json:"decisionPressure"`
				ForecastState              string   `json:"forecastState"`
				PrimaryRisk                string   `json:"primaryRisk"`
				RecommendedFocus           string   `json:"recommendedFocus"`
				CapabilityGaps             []string `json:"capabilityGaps"`
				ForecastReadiness          struct {
					SourceInstance   string `json:"sourceInstance"`
					ReadinessState   string `json:"readinessState"`
					EtaForecastReady bool   `json:"etaForecastReady"`
				} `json:"forecastReadiness"`
				Badges []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
				Breakdowns []struct {
					Dimension string `json:"dimension"`
					Key       string `json:"key"`
					Count     int    `json:"count"`
				} `json:"breakdowns"`
				OwnerRollups []struct {
					OwnerKey            string  `json:"ownerKey"`
					OwnerSource         string  `json:"ownerSource"`
					ItemCount           int     `json:"itemCount"`
					NeedsDecisionCount  int     `json:"needsDecisionCount"`
					ValidateSignalCount int     `json:"validateSignalCount"`
					NowCount            int     `json:"nowCount"`
					HighRiskCount       int     `json:"highRiskCount"`
					MaxRiskScore        float64 `json:"maxRiskScore"`
					Badges              []struct {
						Key    string `json:"key"`
						Tone   string `json:"tone"`
						Detail string `json:"detail"`
					} `json:"badges"`
					TopItems []struct {
						Key           string  `json:"key"`
						ProgramStatus string  `json:"programStatus"`
						RiskScore     float64 `json:"riskScore"`
					} `json:"topItems"`
				} `json:"ownerRollups"`
				TopOwnerLoads []struct {
					OwnerKey         string  `json:"ownerKey"`
					LoadStatus       string  `json:"loadStatus"`
					ActionCount      int     `json:"actionCount"`
					MaxPriorityScore float64 `json:"maxPriorityScore"`
					EvidenceRef      string  `json:"evidenceRef"`
					Badges           []struct {
						Key    string `json:"key"`
						Tone   string `json:"tone"`
						Detail string `json:"detail"`
					} `json:"badges"`
				} `json:"topOwnerLoads"`
				TopItems []struct {
					Key           string  `json:"key"`
					ProgramStatus string  `json:"programStatus"`
					RiskScore     float64 `json:"riskScore"`
				} `json:"topItems"`
				TopBlockers []struct {
					Key          string `json:"key"`
					BlockerState string `json:"blockerState"`
					SubjectKey   string `json:"subjectKey"`
					ActionKey    string `json:"actionKey"`
					EvidenceRef  string `json:"evidenceRef"`
				} `json:"topBlockers"`
				TopBlockerImpacts []struct {
					Key         string `json:"key"`
					ImpactState string `json:"impactState"`
					BlockerKey  string `json:"blockerKey"`
					ActionKey   string `json:"actionKey"`
					AffectedKey string `json:"affectedKey"`
					EvidenceRef string `json:"evidenceRef"`
				} `json:"topBlockerImpacts"`
				TopDependencies []struct {
					Key         string `json:"key"`
					EdgeKind    string `json:"edgeKind"`
					FromKey     string `json:"fromKey"`
					ToKey       string `json:"toKey"`
					EvidenceRef string `json:"evidenceRef"`
				} `json:"topDependencies"`
				TopForecasts []struct {
					Key                string  `json:"key"`
					SubjectKey         string  `json:"subjectKey"`
					RiskBand           string  `json:"riskBand"`
					ActionabilityState string  `json:"actionabilityState"`
					RecommendedAction  string  `json:"recommendedAction"`
					EvidenceRef        string  `json:"evidenceRef"`
					RiskScore          float64 `json:"riskScore"`
					Action             *struct {
						Key           string `json:"key"`
						DecisionState string `json:"decisionState"`
					} `json:"action"`
					Badges []struct {
						Key    string `json:"key"`
						Tone   string `json:"tone"`
						Detail string `json:"detail"`
					} `json:"badges"`
				} `json:"topForecasts"`
			} `json:"workProgramSummary"`
			WorkProgramBrief struct {
				SourceInstance   string   `json:"sourceInstance"`
				WorkstreamKey    string   `json:"workstreamKey"`
				GeneratedAt      string   `json:"generatedAt"`
				OperatingStatus  string   `json:"operatingStatus"`
				DecisionPressure string   `json:"decisionPressure"`
				ForecastState    string   `json:"forecastState"`
				PrimaryRisk      string   `json:"primaryRisk"`
				ExecutiveSummary string   `json:"executiveSummary"`
				RecommendedFocus string   `json:"recommendedFocus"`
				NextCadenceFocus string   `json:"nextCadenceFocus"`
				CapabilityGaps   []string `json:"capabilityGaps"`
				Summary          struct {
					TotalCount            int    `json:"totalCount"`
					OperatingStatus       string `json:"operatingStatus"`
					ActiveBlockerCount    int    `json:"activeBlockerCount"`
					OwnerLoadStatus       string `json:"ownerLoadStatus"`
					OverloadedOwnerCount  int    `json:"overloadedOwnerCount"`
					UnassignedActionCount int    `json:"unassignedActionCount"`
				} `json:"summary"`
				StandupSections []struct {
					SectionKind string `json:"sectionKind"`
					Urgency     string `json:"urgency"`
					SubjectKey  string `json:"subjectKey"`
					Action      *struct {
						Key            string `json:"key"`
						SourceInstance string `json:"sourceInstance"`
					} `json:"action"`
				} `json:"standupSections"`
				ImmediateActions []struct {
					SectionKind string `json:"sectionKind"`
					Urgency     string `json:"urgency"`
					SubjectKey  string `json:"subjectKey"`
				} `json:"immediateActions"`
				ValidationQueue []struct {
					SectionKind string `json:"sectionKind"`
					Urgency     string `json:"urgency"`
					SubjectKey  string `json:"subjectKey"`
				} `json:"validationQueue"`
				RiskDrivers []struct {
					DriverKind        string  `json:"driverKind"`
					Key               string  `json:"key"`
					Status            string  `json:"status"`
					SubjectKey        string  `json:"subjectKey"`
					Title             string  `json:"title"`
					RecommendedAction string  `json:"recommendedAction"`
					EvidenceRef       string  `json:"evidenceRef"`
					RankScore         float64 `json:"rankScore"`
				} `json:"riskDrivers"`
				RiskDriverBuckets []struct {
					DriverKind      string `json:"driverKind"`
					Title           string `json:"title"`
					RiskDriverCount int    `json:"riskDriverCount"`
					TopRiskDrivers  []struct {
						DriverKind string  `json:"driverKind"`
						Key        string  `json:"key"`
						Status     string  `json:"status"`
						SubjectKey string  `json:"subjectKey"`
						RankScore  float64 `json:"rankScore"`
					} `json:"topRiskDrivers"`
				} `json:"riskDriverBuckets"`
				ForecastReadiness struct {
					ReadinessState   string `json:"readinessState"`
					EtaForecastReady bool   `json:"etaForecastReady"`
				} `json:"forecastReadiness"`
				InsightEvaluation struct {
					CurrentInsightCount         int  `json:"currentInsightCount"`
					MeasurementLabelCount       int  `json:"measurementLabelCount"`
					OpenReviewRequestCount      int  `json:"openReviewRequestCount"`
					ReadyToMeasurePrecision     bool `json:"readyToMeasurePrecision"`
					ReadyToMeasureActionability bool `json:"readyToMeasureActionability"`
					GatedInsightKindCount       int  `json:"gatedInsightKindCount"`
				} `json:"insightEvaluation"`
				TpmFunctionReadiness []struct {
					FunctionKey           string   `json:"functionKey"`
					FunctionName          string   `json:"functionName"`
					ReadinessState        string   `json:"readinessState"`
					AutomationState       string   `json:"automationState"`
					HumanRequired         bool     `json:"humanRequired"`
					SupportingSignalCount int      `json:"supportingSignalCount"`
					BlockingGateKeys      []string `json:"blockingGateKeys"`
					Detail                string   `json:"detail"`
					RecommendedAction     string   `json:"recommendedAction"`
				} `json:"tpmFunctionReadiness"`
				AdversarialChecks []struct {
					Key               string   `json:"key"`
					CheckKind         string   `json:"checkKind"`
					CheckState        string   `json:"checkState"`
					Severity          string   `json:"severity"`
					Title             string   `json:"title"`
					Detail            string   `json:"detail"`
					RecommendedAction string   `json:"recommendedAction"`
					BlockingGateKeys  []string `json:"blockingGateKeys"`
					EvidenceRefs      []string `json:"evidenceRefs"`
				} `json:"adversarialChecks"`
				QualityGates []struct {
					Key               string `json:"key"`
					GateState         string `json:"gateState"`
					Blocking          bool   `json:"blocking"`
					Detail            string `json:"detail"`
					RecommendedAction string `json:"recommendedAction"`
				} `json:"qualityGates"`
				Caveats []struct {
					Key               string `json:"key"`
					Severity          string `json:"severity"`
					Title             string `json:"title"`
					Detail            string `json:"detail"`
					RecommendedAction string `json:"recommendedAction"`
					EvidenceRef       string `json:"evidenceRef"`
				} `json:"caveats"`
				Badges []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"workProgramBrief"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkProgramItems) != 1 {
		t.Fatalf("expected one program item, got %#v", response.Data.WorkProgramItems)
	}
	got := response.Data.WorkProgramItems[0]
	if got.Key != "work-program-item:test:fixture" || got.SourceInstance != "fixture-source" || got.WorkstreamKey != "flink-kubernetes-operator" {
		t.Fatalf("unexpected program item identity: %#v", got)
	}
	if got.SubjectKind != "pull_request" || got.SubjectKey != "repo/example#72" || got.ProgramStatus != "needs_decision" || got.TpmBucket != "risk" || got.DecisionState != "product_action" || got.DueBucket != "now" {
		t.Fatalf("unexpected program item typed fields: %#v", got)
	}
	if got.ClaimUse != "product_action" || got.ClaimGateReason != "gold labels support escalation" || !got.ProductActionAllowed || got.AbsenceClaimAllowed || got.EtaClaimAllowed {
		t.Fatalf("program item exposed unsafe claim gates: %#v", got)
	}
	if len(got.LinkedTicketKeys) != 2 || got.LinkedTicketKeys[0] != "FLINK-1" || len(got.LinkedPullRequestKeys) != 1 || got.LinkedPullRequestKeys[0] != "repo/example#73" {
		t.Fatalf("program item did not split linked keys: %#v", got)
	}
	if got.OwnerKey != "github:owner" || got.NextAction == "" || got.RiskScore != 91.5 || !strings.Contains(got.EvidenceRef, "tpm_program_register") {
		t.Fatalf("unexpected program item operating fields: %#v", got)
	}
	if got.Action.Key != "tpm-action:test:program" || got.Action.SourceInstance != "fixture-source" || got.Action.DecisionState != "product_action" {
		t.Fatalf("program item did not expose linked same-source action: %#v", got.Action)
	}
	if got.Action.ClaimUse != "product_action" || got.Action.ClaimGateReason != "product_action_gate_passed" || !got.Action.ProductActionAllowed || got.Action.AbsenceClaimAllowed || got.Action.EtaClaimAllowed {
		t.Fatalf("linked action exposed unsafe claim gates: %#v", got.Action)
	}
	for _, key := range []string{"program:status", "program:due_now", "program:ci", "program:risk"} {
		if !hasBadge(got.Badges, key) {
			t.Fatalf("program item missing badge %q from %#v", key, got.Badges)
		}
	}
	summary := response.Data.WorkProgramSummary
	if summary.SourceInstance != "fixture-source" || summary.TotalCount != 2 || summary.NeedsDecisionCount != 1 || summary.ValidateSignalCount != 1 {
		t.Fatalf("unexpected program summary status counts: %#v", summary)
	}
	if summary.NowCount != 1 || summary.HighRiskCount != 1 || summary.UnassignedCount != 1 || summary.ProductActionCount != 1 || summary.ValidationLeadCount != 1 || summary.SourceCoverageLimitedCount != 0 {
		t.Fatalf("unexpected program summary operating counts: %#v", summary)
	}
	if summary.OwnerLoadStatus != "overloaded" || summary.OwnerLoadActionCount != 3 || summary.OverloadedOwnerCount != 1 || summary.AttentionOwnerCount != 0 || summary.UnassignedActionCount != 1 {
		t.Fatalf("unexpected program summary owner-load counts: %#v", summary)
	}
	if summary.BlockerCount != 1 || summary.ActiveBlockerCount != 1 || summary.ValidatingBlockerCount != 0 || summary.BlockerImpactCount != 1 || summary.ActiveBlockerImpactCount != 1 {
		t.Fatalf("unexpected program summary blocker counts: %#v", summary)
	}
	if summary.DependencyEdgeCount != 2 || summary.BlockingDependencyCount != 1 || summary.NeedsActionDependencyCount != 1 {
		t.Fatalf("unexpected program summary dependency counts: %#v", summary)
	}
	if summary.ForecastReadiness.SourceInstance != "fixture-source" || summary.ForecastReadiness.EtaForecastReady {
		t.Fatalf("unexpected program summary forecast readiness: %#v", summary.ForecastReadiness)
	}
	if summary.OperatingStatus != "blocked" || summary.DecisionPressure != "blocked" || summary.ForecastState != "gated" || summary.PrimaryRisk != "active_blockers" {
		t.Fatalf("unexpected program operating posture: %#v", summary)
	}
	if !strings.Contains(summary.RecommendedFocus, "active blocker") || !strings.Contains(summary.RecommendedFocus, "product action") || !strings.Contains(summary.RecommendedFocus, "validation lead") || !strings.Contains(summary.RecommendedFocus, "ETA forecast as gated") {
		t.Fatalf("unexpected program recommended focus: %q", summary.RecommendedFocus)
	}
	for _, gap := range []string{"forecast_gated", "source_authentication_limited", "unassigned_items", "owner_overloaded", "owner_load_unassigned", "active_blockers", "dependency_pressure", "validation_backlog"} {
		if !hasString(summary.CapabilityGaps, gap) {
			t.Fatalf("program summary missing capability gap %q from %#v", gap, summary.CapabilityGaps)
		}
	}
	if !hasBadge(summary.Badges, "program_summary:needs_decision") || !hasBadge(summary.Badges, "program_summary:validate_signal") || !hasBadge(summary.Badges, "program_summary:source_authentication") || !hasBadge(summary.Badges, "program_summary:unassigned") || !hasBadge(summary.Badges, "program_summary:owner_overloaded") || !hasBadge(summary.Badges, "program_summary:unassigned_actions") || !hasBadge(summary.Badges, "program_summary:blockers") || !hasBadge(summary.Badges, "program_summary:blocker_impacts") || !hasBadge(summary.Badges, "program_summary:blocking_dependencies") || !hasBadge(summary.Badges, "program_summary:needs_action_dependencies") || !hasBadge(summary.Badges, "program_summary:forecast_gated") {
		t.Fatalf("summary badges missing expected keys: %#v", summary.Badges)
	}
	if !hasBreakdown(summary.Breakdowns, "program_status", "needs_decision", 1) || !hasBreakdown(summary.Breakdowns, "tpm_bucket", "risk_validation", 1) || !hasBreakdown(summary.Breakdowns, "owner_key", "unassigned", 1) || !hasBreakdown(summary.Breakdowns, "auth_limited_observation_kind", "anonymous_observation", 1) || !hasBreakdown(summary.Breakdowns, "owner_load_status", "overloaded", 1) || !hasBreakdown(summary.Breakdowns, "owner_load_status", "watch", 1) {
		t.Fatalf("summary breakdowns missing expected rows: %#v", summary.Breakdowns)
	}
	if len(summary.TopOwnerLoads) != 2 || summary.TopOwnerLoads[0].OwnerKey != "github:owner" || summary.TopOwnerLoads[0].LoadStatus != "overloaded" || summary.TopOwnerLoads[0].ActionCount != 2 || summary.TopOwnerLoads[0].MaxPriorityScore != 91.5 || !hasBadge(summary.TopOwnerLoads[0].Badges, "owner_load:status") {
		t.Fatalf("summary did not expose current top owner-load rows: %#v", summary.TopOwnerLoads)
	}
	if summary.TopOwnerLoads[1].OwnerKey != "(unassigned)" || summary.TopOwnerLoads[1].LoadStatus != "watch" || summary.TopOwnerLoads[1].ActionCount != 1 {
		t.Fatalf("summary did not expose unassigned owner-load row: %#v", summary.TopOwnerLoads)
	}
	if len(summary.TopItems) != 2 || summary.TopItems[0].Key != "work-program-item:test:fixture" || summary.TopItems[1].Key != "work-program-item:test:validation" {
		t.Fatalf("summary top items not risk ordered: %#v", summary.TopItems)
	}
	if len(summary.TopBlockers) != 1 || summary.TopBlockers[0].Key != blocker.Key || summary.TopBlockers[0].BlockerState != "active" || summary.TopBlockers[0].SubjectKey != "repo/example#72" || summary.TopBlockers[0].ActionKey != action.Key || summary.TopBlockers[0].EvidenceRef == "" {
		t.Fatalf("summary did not expose top typed blockers: %#v", summary.TopBlockers)
	}
	if len(summary.TopBlockerImpacts) != 1 || summary.TopBlockerImpacts[0].Key != "work-blocker-impact:test:program" || summary.TopBlockerImpacts[0].ImpactState != "active" || summary.TopBlockerImpacts[0].BlockerKey != blocker.Key || summary.TopBlockerImpacts[0].ActionKey != action.Key || summary.TopBlockerImpacts[0].AffectedKey != "workstream:flink-kubernetes-operator" || summary.TopBlockerImpacts[0].EvidenceRef == "" {
		t.Fatalf("summary did not expose top typed blocker impacts: %#v", summary.TopBlockerImpacts)
	}
	topDependencyKinds := map[string]string{}
	for _, dependency := range summary.TopDependencies {
		topDependencyKinds[dependency.Key] = dependency.EdgeKind
	}
	if len(summary.TopDependencies) != 2 || topDependencyKinds["work-dependency-edge:test:blocked-by"] != "blocked_by" || topDependencyKinds["work-dependency-edge:test:needs-action"] != "needs_action" {
		t.Fatalf("summary did not expose top typed dependencies: %#v", summary.TopDependencies)
	}
	if len(summary.TopForecasts) != 1 {
		t.Fatalf("summary did not expose top typed forecasts: %#v", summary.TopForecasts)
	}
	forecastRisk := summary.TopForecasts[0]
	if forecastRisk.Key != "work-item-forecast:test:program-critical" || forecastRisk.SubjectKey != "repo/example#72" || forecastRisk.RiskBand != "critical" || forecastRisk.ActionabilityState != "owner_status_needed" || forecastRisk.RiskScore != 97 || !strings.Contains(forecastRisk.RecommendedAction, "TPM risk lead") || forecastRisk.EvidenceRef == "" {
		t.Fatalf("unexpected top forecast risk: %#v", forecastRisk)
	}
	if forecastRisk.Action == nil || forecastRisk.Action.Key != action.Key || forecastRisk.Action.DecisionState != "product_action" {
		t.Fatalf("top forecast did not expose linked work action: %#v", forecastRisk.Action)
	}
	if !hasBadge(forecastRisk.Badges, "forecast:action_owner_status") || !hasBadge(forecastRisk.Badges, "forecast:eta_gated") {
		t.Fatalf("top forecast did not expose actionability/readiness badges: %#v", forecastRisk.Badges)
	}
	brief := response.Data.WorkProgramBrief
	if brief.SourceInstance != "fixture-source" || brief.WorkstreamKey != "flink-kubernetes-operator" || brief.OperatingStatus != "blocked" || brief.DecisionPressure != "blocked" || brief.ForecastState != "gated" || brief.PrimaryRisk != "active_blockers" {
		t.Fatalf("unexpected program brief posture: %#v", brief)
	}
	if brief.Summary.TotalCount != 2 || brief.Summary.OperatingStatus != "blocked" || brief.Summary.ActiveBlockerCount != 1 || brief.Summary.OwnerLoadStatus != "overloaded" || brief.Summary.OverloadedOwnerCount != 1 || brief.Summary.UnassignedActionCount != 1 {
		t.Fatalf("brief did not embed summary rollup: %#v", brief.Summary)
	}
	if !strings.Contains(brief.ExecutiveSummary, "typed program item") || !strings.Contains(brief.ExecutiveSummary, "active blocker") || !strings.Contains(brief.NextCadenceFocus, "blocker review") {
		t.Fatalf("brief did not synthesize TPM narrative: %#v", brief)
	}
	if len(brief.StandupSections) != 2 || brief.StandupSections[0].SectionKind != "product_action" || brief.StandupSections[0].Action == nil || brief.StandupSections[0].Action.Key != action.Key {
		t.Fatalf("brief did not expose persisted standup agenda: %#v", brief.StandupSections)
	}
	if len(brief.ImmediateActions) != 1 || brief.ImmediateActions[0].SectionKind != "product_action" {
		t.Fatalf("brief did not isolate immediate actions: %#v", brief.ImmediateActions)
	}
	if len(brief.ValidationQueue) != 1 || brief.ValidationQueue[0].SectionKind != "validation_lead" {
		t.Fatalf("brief did not isolate validation queue: %#v", brief.ValidationQueue)
	}
	driverKinds := map[string]bool{}
	var forecastDriver *struct {
		DriverKind        string  `json:"driverKind"`
		Key               string  `json:"key"`
		Status            string  `json:"status"`
		SubjectKey        string  `json:"subjectKey"`
		Title             string  `json:"title"`
		RecommendedAction string  `json:"recommendedAction"`
		EvidenceRef       string  `json:"evidenceRef"`
		RankScore         float64 `json:"rankScore"`
	}
	var ownerLoadDriver *struct {
		DriverKind        string  `json:"driverKind"`
		Key               string  `json:"key"`
		Status            string  `json:"status"`
		SubjectKey        string  `json:"subjectKey"`
		Title             string  `json:"title"`
		RecommendedAction string  `json:"recommendedAction"`
		EvidenceRef       string  `json:"evidenceRef"`
		RankScore         float64 `json:"rankScore"`
	}
	for i := range brief.RiskDrivers {
		driver := brief.RiskDrivers[i]
		driverKinds[driver.DriverKind] = true
		if driver.EvidenceRef == "" || driver.Title == "" || driver.Status == "" {
			t.Fatalf("brief risk driver missing display fields: %#v", driver)
		}
		if driver.DriverKind == "forecast_risk" {
			forecastDriver = &brief.RiskDrivers[i]
		}
		if driver.DriverKind == "owner_load" && driver.SubjectKey == "github:owner" {
			ownerLoadDriver = &brief.RiskDrivers[i]
		}
	}
	if !driverKinds["blocker"] || !driverKinds["blocker_impact"] || !driverKinds["dependency"] || !driverKinds["owner_load"] || !driverKinds["forecast_risk"] {
		t.Fatalf("brief did not normalize blocker/impact/dependency drivers: %#v", brief.RiskDrivers)
	}
	if len(brief.RiskDriverBuckets) != 5 {
		t.Fatalf("brief did not expose risk driver buckets: %#v", brief.RiskDriverBuckets)
	}
	expectedBucketCounts := map[string]int{
		"blocker_impact": 1,
		"blocker":        1,
		"dependency":     2,
		"owner_load":     2,
		"forecast_risk":  1,
	}
	expectedBucketOrder := []string{"blocker_impact", "blocker", "dependency", "owner_load", "forecast_risk"}
	for i, expectedKind := range expectedBucketOrder {
		bucket := brief.RiskDriverBuckets[i]
		if bucket.DriverKind != expectedKind || bucket.Title == "" || bucket.RiskDriverCount != expectedBucketCounts[expectedKind] || len(bucket.TopRiskDrivers) != expectedBucketCounts[expectedKind] {
			t.Fatalf("unexpected risk driver bucket %d: %#v", i, bucket)
		}
		if bucket.TopRiskDrivers[0].DriverKind != expectedKind || bucket.TopRiskDrivers[0].Key == "" || bucket.TopRiskDrivers[0].Status == "" || bucket.TopRiskDrivers[0].RankScore == 0 {
			t.Fatalf("risk driver bucket missing top driver fields: %#v", bucket)
		}
	}
	if forecastDriver == nil || forecastDriver.Key != "work-item-forecast:test:program-critical" || forecastDriver.Status != "owner_status_needed" || forecastDriver.SubjectKey != "repo/example#72" || forecastDriver.RankScore != 128.25 || !strings.Contains(forecastDriver.RecommendedAction, "TPM risk lead") {
		t.Fatalf("brief did not expose forecast risk driver: %#v", brief.RiskDrivers)
	}
	if ownerLoadDriver == nil || ownerLoadDriver.Status != "overloaded" || !strings.Contains(ownerLoadDriver.Title, "owner") || !strings.Contains(ownerLoadDriver.RecommendedAction, "Rebalance owner load") || ownerLoadDriver.RankScore != 116.5 {
		t.Fatalf("brief did not expose owner-load risk driver: %#v", brief.RiskDrivers)
	}
	if brief.InsightEvaluation.ReadyToMeasurePrecision || brief.InsightEvaluation.ReadyToMeasureActionability {
		t.Fatalf("brief should expose gated measurement state for unlabeled fixture insights: %#v", brief.InsightEvaluation)
	}
	functions := map[string]struct {
		FunctionKey           string   `json:"functionKey"`
		FunctionName          string   `json:"functionName"`
		ReadinessState        string   `json:"readinessState"`
		AutomationState       string   `json:"automationState"`
		HumanRequired         bool     `json:"humanRequired"`
		SupportingSignalCount int      `json:"supportingSignalCount"`
		BlockingGateKeys      []string `json:"blockingGateKeys"`
		Detail                string   `json:"detail"`
		RecommendedAction     string   `json:"recommendedAction"`
	}{}
	for _, row := range brief.TpmFunctionReadiness {
		functions[row.FunctionKey] = row
		if row.FunctionName == "" || row.ReadinessState == "" || row.AutomationState == "" || row.Detail == "" || row.RecommendedAction == "" {
			t.Fatalf("TPM function readiness missing display fields: %#v", row)
		}
	}
	if len(functions) != 7 {
		t.Fatalf("brief did not expose stable TPM function readiness rows: %#v", brief.TpmFunctionReadiness)
	}
	if row := functions["operating_brief"]; row.ReadinessState != "automatable" || row.AutomationState != "can_publish_operating_brief" || row.HumanRequired || row.SupportingSignalCount != 4 {
		t.Fatalf("unexpected operating brief readiness: %#v", row)
	}
	if row := functions["forecast_triage"]; row.ReadinessState != "blocked" || row.AutomationState != "risk_triage_only" || !row.HumanRequired || !hasString(row.BlockingGateKeys, "forecast_readiness") {
		t.Fatalf("unexpected forecast triage readiness: %#v", row)
	}
	if row := functions["execution_capacity"]; row.ReadinessState != "blocked" || row.AutomationState != "rebalance_required" || !row.HumanRequired || row.SupportingSignalCount != 3 || !hasString(row.BlockingGateKeys, "owner_load") {
		t.Fatalf("unexpected execution capacity readiness: %#v", row)
	}
	if row := functions["source_coverage"]; row.ReadinessState != "automatable" || row.AutomationState != "coverage_ready" || row.HumanRequired || len(row.BlockingGateKeys) != 0 {
		t.Fatalf("unexpected source coverage readiness: %#v", row)
	}
	if row := functions["insight_quality"]; row.ReadinessState != "blocked" || !hasString(row.BlockingGateKeys, "measurement_precision") || !hasString(row.BlockingGateKeys, "measurement_actionability") {
		t.Fatalf("unexpected insight QA readiness: %#v", row)
	}
	if row := functions["product_decisions"]; row.ReadinessState != "supervised" || row.AutomationState != "human_decision_required" || !row.HumanRequired || row.SupportingSignalCount != 2 {
		t.Fatalf("unexpected product decision readiness: %#v", row)
	}
	checks := map[string]struct {
		Key               string   `json:"key"`
		CheckKind         string   `json:"checkKind"`
		CheckState        string   `json:"checkState"`
		Severity          string   `json:"severity"`
		Title             string   `json:"title"`
		Detail            string   `json:"detail"`
		RecommendedAction string   `json:"recommendedAction"`
		BlockingGateKeys  []string `json:"blockingGateKeys"`
		EvidenceRefs      []string `json:"evidenceRefs"`
	}{}
	for _, check := range brief.AdversarialChecks {
		checks[check.Key] = check
		if check.CheckKind == "" || check.CheckState == "" || check.Severity == "" || check.Title == "" || check.Detail == "" || check.RecommendedAction == "" {
			t.Fatalf("adversarial check missing display fields: %#v", check)
		}
	}
	if len(checks) != 9 {
		t.Fatalf("brief did not expose stable adversarial checks: %#v", brief.AdversarialChecks)
	}
	if check := checks["brief_basis"]; check.CheckState != "pass" || check.Severity != "info" || len(check.EvidenceRefs) == 0 {
		t.Fatalf("unexpected brief basis adversarial check: %#v", check)
	}
	if check := checks["forecast_overclaim"]; check.CheckState != "fail" || check.Severity != "critical" || !hasString(check.BlockingGateKeys, "forecast_readiness") || len(check.EvidenceRefs) == 0 {
		t.Fatalf("unexpected forecast overclaim check: %#v", check)
	}
	if check := checks["source_absence_claims"]; check.CheckState != "pass" || len(check.BlockingGateKeys) != 0 {
		t.Fatalf("unexpected source absence adversarial check: %#v", check)
	}
	if check := checks["source_authentication_claims"]; check.CheckState != "warning" || check.Severity != "high" || len(check.BlockingGateKeys) != 0 || !strings.Contains(check.RecommendedAction, "authenticated") {
		t.Fatalf("unexpected source authentication adversarial check: %#v", check)
	}
	if check := checks["generated_claim_provenance"]; check.CheckState != "pass" || check.Severity != "info" {
		t.Fatalf("unexpected generated-claim provenance adversarial check: %#v", check)
	}
	if check := checks["measurement_overclaim"]; check.CheckState != "fail" || !hasString(check.BlockingGateKeys, "measurement_precision") || !hasString(check.BlockingGateKeys, "measurement_actionability") {
		t.Fatalf("unexpected measurement adversarial check: %#v", check)
	}
	if check := checks["execution_assumption"]; check.CheckState != "fail" || check.Severity != "high" || !hasString(check.BlockingGateKeys, "owner_load") || len(check.EvidenceRefs) == 0 {
		t.Fatalf("unexpected execution adversarial check: %#v", check)
	}
	if check := checks["blocker_clearance_claim"]; check.CheckState != "fail" || !hasString(check.BlockingGateKeys, "blocker_clearance") || len(check.EvidenceRefs) == 0 {
		t.Fatalf("unexpected blocker clearance adversarial check: %#v", check)
	}
	if check := checks["human_decision_boundary"]; check.CheckState != "warning" || check.Severity != "high" || !strings.Contains(check.RecommendedAction, "do not automate") {
		t.Fatalf("unexpected human decision boundary adversarial check: %#v", check)
	}
	gateStates := map[string]struct {
		state    string
		blocking bool
	}{}
	for _, gate := range brief.QualityGates {
		gateStates[gate.Key] = struct {
			state    string
			blocking bool
		}{state: gate.GateState, blocking: gate.Blocking}
		if gate.Detail == "" || gate.RecommendedAction == "" {
			t.Fatalf("brief quality gate missing detail/action: %#v", gate)
		}
	}
	for _, key := range []string{"forecast_readiness", "measurement_precision", "measurement_actionability", "owner_load", "blocker_clearance"} {
		gate, ok := gateStates[key]
		if !ok || gate.state != "gated" || !gate.blocking {
			t.Fatalf("brief quality gate %q not gated/blocking: %#v", key, brief.QualityGates)
		}
	}
	if gate, ok := gateStates["source_authentication"]; !ok || gate.state != "watch" || gate.blocking {
		t.Fatalf("brief source authentication gate did not watch validation-only auth limits: %#v", brief.QualityGates)
	}
	if gate, ok := gateStates["source_coverage"]; !ok || gate.state != "passed" || gate.blocking {
		t.Fatalf("brief source coverage gate did not pass: %#v", brief.QualityGates)
	}
	caveatSeverities := map[string]string{}
	for _, caveat := range brief.Caveats {
		caveatSeverities[caveat.Key] = caveat.Severity
		if caveat.Title == "" || caveat.Detail == "" || caveat.RecommendedAction == "" {
			t.Fatalf("brief caveat missing display fields: %#v", caveat)
		}
	}
	for _, key := range []string{"forecast_gated", "measurement_gated", "source_authentication_limited", "owner_load", "active_blockers", "dependency_pressure"} {
		if caveatSeverities[key] == "" {
			t.Fatalf("brief missing caveat %q from %#v", key, brief.Caveats)
		}
	}
	automationBody := `{"query":"query { workProgramBrief(workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"fixture-source\") { automationReadiness { readinessState readinessScore autonomousActionReady humanReviewRequired safeAutomationAreas humanRequiredAreas rationale requiredEvidence evidenceWorkQueue { key gateKey evidenceKind priority targetKind targetKey metricKey executionState backingActionCount currentCount requiredCount missingCount currentRate requiredRate recommendedAction nextExecutionStep } gates { key gateState blocking } } } }"}`
	automationReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(automationBody))
	automationReq.Header.Set("Content-Type", "application/json")
	automationRec := httptest.NewRecorder()
	router.ServeHTTP(automationRec, automationReq)
	if automationRec.Code != http.StatusOK {
		t.Fatalf("expected automation status 200, got %d: %s", automationRec.Code, automationRec.Body.String())
	}
	var automationResponse struct {
		Data struct {
			WorkProgramBrief struct {
				AutomationReadiness struct {
					ReadinessState        string   `json:"readinessState"`
					ReadinessScore        float64  `json:"readinessScore"`
					AutonomousActionReady bool     `json:"autonomousActionReady"`
					HumanReviewRequired   bool     `json:"humanReviewRequired"`
					SafeAutomationAreas   []string `json:"safeAutomationAreas"`
					HumanRequiredAreas    []string `json:"humanRequiredAreas"`
					Rationale             string   `json:"rationale"`
					RequiredEvidence      []string `json:"requiredEvidence"`
					EvidenceWorkQueue     []struct {
						Key                string   `json:"key"`
						GateKey            string   `json:"gateKey"`
						EvidenceKind       string   `json:"evidenceKind"`
						Priority           string   `json:"priority"`
						TargetKind         string   `json:"targetKind"`
						TargetKey          string   `json:"targetKey"`
						MetricKey          string   `json:"metricKey"`
						ExecutionState     string   `json:"executionState"`
						BackingActionCount int      `json:"backingActionCount"`
						CurrentCount       int      `json:"currentCount"`
						RequiredCount      int      `json:"requiredCount"`
						MissingCount       int      `json:"missingCount"`
						CurrentRate        *float64 `json:"currentRate"`
						RequiredRate       *float64 `json:"requiredRate"`
						RecommendedAction  string   `json:"recommendedAction"`
						NextExecutionStep  string   `json:"nextExecutionStep"`
					} `json:"evidenceWorkQueue"`
					Gates []struct {
						Key       string `json:"key"`
						GateState string `json:"gateState"`
						Blocking  bool   `json:"blocking"`
					} `json:"gates"`
				} `json:"automationReadiness"`
			} `json:"workProgramBrief"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(automationRec.Body.Bytes(), &automationResponse); err != nil {
		t.Fatalf("automation response is not JSON: %v", err)
	}
	if len(automationResponse.Errors) > 0 {
		t.Fatalf("automation response had errors: %#v", automationResponse.Errors)
	}
	readiness := automationResponse.Data.WorkProgramBrief.AutomationReadiness
	if readiness.ReadinessState != "blocked" || readiness.ReadinessScore >= 35 || readiness.AutonomousActionReady || !readiness.HumanReviewRequired {
		t.Fatalf("unexpected automation readiness posture: %#v", readiness)
	}
	for _, area := range []string{"agenda_summarization", "risk_driver_ranking", "source_citation", "forecast_triage"} {
		if !hasString(readiness.SafeAutomationAreas, area) {
			t.Fatalf("automation readiness missing safe area %q from %#v", area, readiness.SafeAutomationAreas)
		}
	}
	for _, area := range []string{"eta_commitments", "product_decisions", "measurement_claims", "source_authentication", "blocker_clearance", "owner_load_balancing"} {
		if !hasString(readiness.HumanRequiredAreas, area) {
			t.Fatalf("automation readiness missing human-required area %q from %#v", area, readiness.HumanRequiredAreas)
		}
	}
	if !strings.Contains(readiness.Rationale, "blocked") || len(readiness.RequiredEvidence) < 5 {
		t.Fatalf("automation readiness did not explain gates: %#v", readiness)
	}
	if len(readiness.EvidenceWorkQueue) < 3 || readiness.EvidenceWorkQueue[0].EvidenceKind != "blocker_clearance" || readiness.EvidenceWorkQueue[0].Priority != "critical" {
		t.Fatalf("automation readiness did not prioritize blocker clearance work: %#v", readiness.EvidenceWorkQueue)
	}
	if readiness.EvidenceWorkQueue[0].ExecutionState != "actions_open" || readiness.EvidenceWorkQueue[0].BackingActionCount == 0 || !strings.Contains(readiness.EvidenceWorkQueue[0].NextExecutionStep, "open blocker-clearance") {
		t.Fatalf("automation readiness did not expose blocker execution coverage: %#v", readiness.EvidenceWorkQueue[0])
	}
	for _, key := range []string{"forecast_readiness:backtest", "source_authentication:anonymous_observation", "owner_load:rebalance", "blocker_clearance:active"} {
		if findAutomationEvidenceNeed(readiness.EvidenceWorkQueue, key) == nil {
			t.Fatalf("automation readiness missing evidence work %q from %#v", key, readiness.EvidenceWorkQueue)
		}
	}
	ownerLoadNeed := findAutomationEvidenceNeed(readiness.EvidenceWorkQueue, "owner_load:rebalance")
	if ownerLoadNeed.EvidenceKind != "owner_load_balancing" || ownerLoadNeed.ExecutionState != "owner_load_rows_open" || ownerLoadNeed.BackingActionCount != 3 || ownerLoadNeed.CurrentCount != 2 || !strings.Contains(ownerLoadNeed.NextExecutionStep, "owner-load rows") {
		t.Fatalf("automation readiness did not expose owner-load execution path: %#v", ownerLoadNeed)
	}
	sourceNeed := findAutomationEvidenceNeed(readiness.EvidenceWorkQueue, "source_authentication:anonymous_observation")
	if sourceNeed.EvidenceKind != "source_authentication" || sourceNeed.ExecutionState != "review_actions_open" || sourceNeed.BackingActionCount == 0 || !strings.Contains(sourceNeed.NextExecutionStep, "re-observe anonymous") {
		t.Fatalf("automation readiness did not expose anonymous-source execution path: %#v", sourceNeed)
	}
	readinessGateStates := map[string]struct {
		state    string
		blocking bool
	}{}
	for _, gate := range readiness.Gates {
		readinessGateStates[gate.Key] = struct {
			state    string
			blocking bool
		}{state: gate.GateState, blocking: gate.Blocking}
	}
	for _, key := range []string{"forecast_readiness", "measurement_precision", "measurement_actionability", "owner_load", "blocker_clearance"} {
		gate, ok := readinessGateStates[key]
		if !ok || gate.state != "gated" || !gate.blocking {
			t.Fatalf("automation readiness gate %q not gated/blocking: %#v", key, readiness.Gates)
		}
	}
	if gate, ok := readinessGateStates["source_authentication"]; !ok || gate.state != "watch" || gate.blocking {
		t.Fatalf("automation readiness source authentication gate did not watch validation-only auth limits: %#v", readiness.Gates)
	}
	if len(summary.OwnerRollups) != 2 || summary.OwnerRollups[0].OwnerKey != "github:owner" || summary.OwnerRollups[0].MaxRiskScore != 91.5 {
		t.Fatalf("unexpected owner rollups: %#v", summary.OwnerRollups)
	}
	if summary.OwnerRollups[1].OwnerKey != "unassigned" || summary.OwnerRollups[1].ValidateSignalCount != 1 || !hasBadge(summary.OwnerRollups[1].Badges, "program_owner:validate_signal") {
		t.Fatalf("unexpected unassigned owner rollup: %#v", summary.OwnerRollups[1])
	}

	blankSourceBody := `{"query":"query { workProgramItems(sourceInstance: \"   \") { key } }"}`
	blankSourceReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(blankSourceBody))
	blankSourceReq.Header.Set("Content-Type", "application/json")
	blankSourceRec := httptest.NewRecorder()
	router.ServeHTTP(blankSourceRec, blankSourceReq)
	var blankSourceResponse struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(blankSourceRec.Body.Bytes(), &blankSourceResponse); err != nil {
		t.Fatalf("blank program source graphql response is not JSON: %v", err)
	}
	if len(blankSourceResponse.Errors) == 0 || !strings.Contains(blankSourceResponse.Errors[0].Message, "sourceInstance cannot be blank") {
		t.Fatalf("blank program source query did not return validation error: %s", blankSourceRec.Body.String())
	}
}

func TestGraphQLWorkstreamHealthSnapshotsExposePersistedOperatingState(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	stream := store.Client().Workstream.Create().
		SetKey("workstream:flink-kubernetes-operator").
		SetTitle("Flink Kubernetes Operator").
		SetStatus(workstream.StatusActive).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream").
		SetExternalID("flink-kubernetes-operator").
		SaveX(ctx)
	health := store.Client().WorkstreamHealthSnapshot.Create().
		SetKey("workstream-health-snapshot:test:flink").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)).
		SetOperatingStatus(workstreamhealthsnapshot.OperatingStatusAttentionRequired).
		SetActionItemCount(19).
		SetProductActionCount(4).
		SetValidationLeadCount(11).
		SetCriticalOrHighValidationLeadCount(8).
		SetModelOrRuleQaCount(1).
		SetCloseoutReviewCount(1).
		SetOwnerCount(13).
		SetTopOwnerActionCount(2).
		SetFailingCheckPrCount(2).
		SetOpenFailingCheckPrCount(2).
		SetSourceRepairCount(1).
		SetCoverageLimitedCount(3).
		SetAnonymousObservationCount(2).
		SetTerminalTransitionCount(1).
		SetTerminalTransitionSubjects("apache/flink-kubernetes-operator#1085").
		SetTruthLabelCoverage("10/23").
		SetActionabilityLabelCoverage("10/23").
		SetRecommendedCadenceFocus("triage product actions; validate urgent leads").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream_health_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z").
		SaveX(ctx)
	store.Client().WorkstreamHealthSnapshot.Create().
		SetKey("workstream-health-snapshot:test:other-source").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 22, 7, 3, 16, 0, time.UTC)).
		SetOperatingStatus(workstreamhealthsnapshot.OperatingStatusClear).
		SetActionItemCount(1).
		SetProductActionCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_workstream_health_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-22T07:03:16Z").
		SaveX(ctx)
	healthEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:workstream-health").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("workstream_health_snapshot").
		SetClaimTargetID(health.ID).
		SetClaimField("operating_status").
		SetLocatorKind("workstream_standup").
		SetLocator("flink-kubernetes-operator:2026-06-21T07:03:16Z").
		SetSourceSpanKey("workstream-health:standup").
		SetExcerpt("triage product actions; validate urgent leads").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_generated_evidence").
		SetExternalID("workstream-health").
		SaveX(ctx)
	health.Update().SetLatestEvidence(healthEvidence).SetEvidenceCount(1).SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workstreamHealthSnapshots(workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"fixture-source\") { key sourceInstance workstreamKey workstreamTitle generatedAt operatingStatus actionItemCount productActionCount validationLeadCount criticalOrHighValidationLeadCount modelOrRuleQaCount closeoutReviewCount ownerCount topOwnerActionCount failingCheckPullRequestCount openFailingCheckPullRequestCount sourceRepairCount coverageLimitedCount anonymousObservationCount terminalTransitionCount terminalTransitionSubjects etaForecastReady truthLabelCoverage actionabilityLabelCoverage recommendedCadenceFocus evidenceRef badges { key tone detail } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkstreamHealthSnapshots []struct {
				Key                               string `json:"key"`
				SourceInstance                    string `json:"sourceInstance"`
				WorkstreamKey                     string `json:"workstreamKey"`
				WorkstreamTitle                   string `json:"workstreamTitle"`
				OperatingStatus                   string `json:"operatingStatus"`
				ActionItemCount                   int    `json:"actionItemCount"`
				ProductActionCount                int    `json:"productActionCount"`
				ValidationLeadCount               int    `json:"validationLeadCount"`
				CriticalOrHighValidationLeadCount int    `json:"criticalOrHighValidationLeadCount"`
				CloseoutReviewCount               int    `json:"closeoutReviewCount"`
				OpenFailingCheckPullRequestCount  int    `json:"openFailingCheckPullRequestCount"`
				SourceRepairCount                 int    `json:"sourceRepairCount"`
				CoverageLimitedCount              int    `json:"coverageLimitedCount"`
				AnonymousObservationCount         int    `json:"anonymousObservationCount"`
				TerminalTransitionCount           int    `json:"terminalTransitionCount"`
				TerminalTransitionSubjects        string `json:"terminalTransitionSubjects"`
				EtaForecastReady                  bool   `json:"etaForecastReady"`
				TruthLabelCoverage                string `json:"truthLabelCoverage"`
				ActionabilityLabelCoverage        string `json:"actionabilityLabelCoverage"`
				RecommendedCadenceFocus           string `json:"recommendedCadenceFocus"`
				EvidenceRef                       string `json:"evidenceRef"`
				Badges                            []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"workstreamHealthSnapshots"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkstreamHealthSnapshots) != 1 {
		t.Fatalf("expected one health snapshot, got %#v", response.Data.WorkstreamHealthSnapshots)
	}
	row := response.Data.WorkstreamHealthSnapshots[0]
	if row.SourceInstance != "fixture-source" || row.WorkstreamTitle != "Flink Kubernetes Operator" || row.OperatingStatus != "attention_required" || row.ActionItemCount != 19 || row.ProductActionCount != 4 || row.ValidationLeadCount != 11 {
		t.Fatalf("unexpected health snapshot core fields: %#v", row)
	}
	if row.CloseoutReviewCount != 1 || row.OpenFailingCheckPullRequestCount != 2 || row.SourceRepairCount != 1 || row.CoverageLimitedCount != 3 || row.AnonymousObservationCount != 2 || row.TerminalTransitionCount != 1 || row.EtaForecastReady {
		t.Fatalf("unexpected health snapshot operating counts: %#v", row)
	}
	if row.TruthLabelCoverage != "10/23" || row.ActionabilityLabelCoverage != "10/23" || !strings.Contains(row.EvidenceRef, "workstream_standup") {
		t.Fatalf("unexpected health snapshot evidence/coverage fields: %#v", row)
	}
	for _, key := range []string{"workstream_health:status", "workstream_health:product_actions", "workstream_health:urgent_validation", "workstream_health:source_repair", "workstream_health:coverage_limited", "workstream_health:forecast_gated"} {
		if !hasBadge(row.Badges, key) {
			t.Fatalf("missing health badge %q from %#v", key, row.Badges)
		}
	}
	unscopedBody := `{"query":"query { workstreamHealthSnapshots(workstreamKey: \"workstream:flink-kubernetes-operator\") { sourceInstance operatingStatus actionItemCount } }"}`
	unscopedReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(unscopedBody))
	unscopedReq.Header.Set("Content-Type", "application/json")
	unscopedRec := httptest.NewRecorder()
	router.ServeHTTP(unscopedRec, unscopedReq)
	var unscopedResponse struct {
		Data struct {
			WorkstreamHealthSnapshots []struct {
				SourceInstance  string `json:"sourceInstance"`
				OperatingStatus string `json:"operatingStatus"`
				ActionItemCount int    `json:"actionItemCount"`
			} `json:"workstreamHealthSnapshots"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(unscopedRec.Body.Bytes(), &unscopedResponse); err != nil {
		t.Fatalf("unscoped health graphql response is not JSON: %v", err)
	}
	if len(unscopedResponse.Errors) > 0 {
		t.Fatalf("unscoped health graphql response had errors: %#v", unscopedResponse.Errors)
	}
	if len(unscopedResponse.Data.WorkstreamHealthSnapshots) != 1 || unscopedResponse.Data.WorkstreamHealthSnapshots[0].SourceInstance != "other-source" || unscopedResponse.Data.WorkstreamHealthSnapshots[0].OperatingStatus != "clear" {
		t.Fatalf("unscoped health snapshots did not resolve to one current source: %#v", unscopedResponse.Data.WorkstreamHealthSnapshots)
	}
	blankSourceBody := `{"query":"query { workstreamHealthSnapshots(sourceInstance: \"   \") { key } }"}`
	blankSourceReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(blankSourceBody))
	blankSourceReq.Header.Set("Content-Type", "application/json")
	blankSourceRec := httptest.NewRecorder()
	router.ServeHTTP(blankSourceRec, blankSourceReq)
	var blankSourceResponse struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(blankSourceRec.Body.Bytes(), &blankSourceResponse); err != nil {
		t.Fatalf("blank health source graphql response is not JSON: %v", err)
	}
	if len(blankSourceResponse.Errors) == 0 || !strings.Contains(blankSourceResponse.Errors[0].Message, "sourceInstance cannot be blank") {
		t.Fatalf("blank health source query did not return validation error: %s", blankSourceRec.Body.String())
	}
}

func TestGraphQLOwnerLoadSnapshotsExposeLatestPersistedOwnerRouting(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	stream := store.Client().Workstream.Create().
		SetKey("workstream:flink-kubernetes-operator").
		SetTitle("Flink Kubernetes Operator").
		SetStatus(workstream.StatusActive).
		SetSourceInstance("fixture-source").
		SaveX(ctx)
	runAt := time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)
	owner := store.Client().Person.Create().
		SetKey("person:github:lrsb").
		SetDisplayName("lrsb").
		SetGithubLogin("lrsb").
		SaveX(ctx)
	pr := store.Client().PullRequest.Create().
		SetKey("pr:apache/flink-kubernetes-operator#1134").
		SetRepository("apache/flink-kubernetes-operator").
		SetNumber(1134).
		SetTitle("Stabilize autoscaler reconciliation").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("github.com").
		SetExternalKind("github_pull_request").
		SetExternalID("apache/flink-kubernetes-operator#1134").
		SaveX(ctx)
	evidenceRow := store.Client().Evidence.Create().
		SetKey("evidence:test:owner-load").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_owner_load_snapshot").
		SetClaimTargetID(1).
		SetClaimField("recommended_focus").
		SetLocatorKind("tpm_owner_action_rollup").
		SetLocator("flink-kubernetes-operator:2026-06-21T07:03:16Z:github:lrsb").
		SetSourceSpanKey("owner-load:lrsb").
		SetExcerpt("Review the generated action and record a truth/actionability label.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_generated_evidence").
		SetExternalID("owner-load").
		SaveX(ctx)
	current := store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:lrsb").
		SetWorkstream(stream).
		SetPerson(owner).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("github:lrsb").
		SetOwnerDisplayName("lrsb").
		SetGeneratedAt(runAt).
		SetLoadStatus(workownerloadsnapshot.LoadStatusOverloaded).
		SetActionCount(2).
		SetProductActionCount(1).
		SetValidationLeadCount(1).
		SetCriticalOrHighCount(1).
		SetMaxPriorityScore(93).
		SetAvgPriorityScore(71.5).
		SetValidateSignalCount(1).
		SetNeedsHumanReviewCount(1).
		SetTopActionType("clear_blocker").
		SetTopSubjects("apache/flink-kubernetes-operator#1134, apache/flink-kubernetes-operator#1135").
		SetRecommendedFocus("Review the generated action and record a truth/actionability label.").
		SetFreshnessState(workownerloadsnapshot.FreshnessStateFresh).
		SetConfidence(1.0).
		SetLatestEvidence(evidenceRow).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:github:lrsb").
		SaveX(ctx)
	evidenceRow.Update().SetClaimTargetID(current.ID).SaveX(ctx)
	ownerInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:owner-load:same-source").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityHigh).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("apache/flink-kubernetes-operator#1134").
		SetPullRequest(pr).
		SetTitle("Same-source owner load insight").
		SetScore(93).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:owner-load:same-source").
		SaveX(ctx)
	otherSourceOwnerInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:owner-load:other-source").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityCritical).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("apache/flink-kubernetes-operator#1134").
		SetPullRequest(pr).
		SetTitle("Other-source owner load insight should not leak").
		SetScore(99).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:owner-load:other-source").
		SaveX(ctx)
	topOwnerAction := store.Client().WorkAction.Create().
		SetKey("work-action:test:lrsb:1134").
		SetActionType(workaction.ActionTypeClearBlocker).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecision("ready_for_product_action").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("apache/flink-kubernetes-operator#1134").
		SetPullRequest(pr).
		SetOwnerKey("github:lrsb").
		SetOwnerSource("action_owner_hint").
		SetDueBucket(workaction.DueBucketNow).
		SetCreatedFromRunKey("tpm-action-brief:2026-06-21T07:03:16Z").
		SetOpenedAt(runAt).
		SetFreshnessState(workaction.FreshnessStateFresh).
		SetConfidence(1.0).
		SetRankScore(93).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("work-action:test:lrsb:1134").
		AddSourceInsights(ownerInsight).
		AddSourceInsights(otherSourceOwnerInsight).
		SaveX(ctx)
	store.Client().WorkActionObservation.Create().
		SetKey("work-action-observation:test:owner-load:same-source").
		SetAction(topOwnerAction).
		SetObservationKind(workactionobservation.ObservationKindSourceState).
		SetSourceCoverageState("observed:fixture-source").
		SetSupportsAction(true).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action_observation").
		SetExternalID("work-action-observation:test:owner-load:same-source").
		SaveX(ctx)
	store.Client().WorkActionObservation.Create().
		SetKey("work-action-observation:test:owner-load:other-source").
		SetAction(topOwnerAction).
		SetObservationKind(workactionobservation.ObservationKindSourceState).
		SetSourceCoverageState("observed:other-source").
		SetSupportsAction(false).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_action_observation").
		SetExternalID("work-action-observation:test:owner-load:other-source").
		SaveX(ctx)
	store.Client().WorkAction.Create().
		SetKey("work-action:test:lrsb:other-workstream").
		SetActionType(workaction.ActionTypeDecisionOrOwnerFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetDecision("pending_validation").
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("other-workstream#999").
		SetOwnerKey("github:lrsb").
		SetOwnerSource("action_owner_hint").
		SetDueBucket(workaction.DueBucketNow).
		SetCreatedFromRunKey("tpm-action-brief:2026-06-21T07:03:16Z").
		SetOpenedAt(runAt).
		SetFreshnessState(workaction.FreshnessStateFresh).
		SetConfidence(1.0).
		SetRankScore(999).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("work-action:test:lrsb:other-workstream").
		SaveX(ctx)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:other").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("github:vsantwana").
		SetGeneratedAt(runAt).
		SetLoadStatus(workownerloadsnapshot.LoadStatusWatch).
		SetActionCount(1).
		SetValidationLeadCount(1).
		SetMaxPriorityScore(70).
		SetAvgPriorityScore(70).
		SetFreshnessState(workownerloadsnapshot.FreshnessStateFresh).
		SetConfidence(1.0).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:github:vsantwana").
		SaveX(ctx)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:old").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("github:old").
		SetGeneratedAt(time.Date(2026, 6, 20, 7, 3, 16, 0, time.UTC)).
		SetLoadStatus(workownerloadsnapshot.LoadStatusOverloaded).
		SetActionCount(9).
		SetMaxPriorityScore(100).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-20T07:03:16Z:github:old").
		SaveX(ctx)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:other-source-newer").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("github:other-source").
		SetGeneratedAt(time.Date(2026, 6, 22, 7, 3, 16, 0, time.UTC)).
		SetLoadStatus(workownerloadsnapshot.LoadStatusOverloaded).
		SetActionCount(7).
		SetMaxPriorityScore(100).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-22T07:03:16Z:github:other-source").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { ownerLoadSnapshots(workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"fixture-source\") { ownerKey sourceInstance personKey loadStatus actionCount productActionCount validationLeadCount criticalOrHighCount freshnessState confidence evidenceRef topActions { key actionType decisionState subjectKind subjectKey ownerKey relatedPullRequestKeys observations { supportsAction sourceCoverageState } sourceInsights { insightKind title } } badges { key tone detail } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			OwnerLoadSnapshots []struct {
				OwnerKey            string  `json:"ownerKey"`
				SourceInstance      string  `json:"sourceInstance"`
				PersonKey           string  `json:"personKey"`
				LoadStatus          string  `json:"loadStatus"`
				ActionCount         int     `json:"actionCount"`
				ProductActionCount  int     `json:"productActionCount"`
				ValidationLeadCount int     `json:"validationLeadCount"`
				CriticalOrHighCount int     `json:"criticalOrHighCount"`
				FreshnessState      string  `json:"freshnessState"`
				Confidence          float64 `json:"confidence"`
				EvidenceRef         string  `json:"evidenceRef"`
				TopActions          []struct {
					Key                    string   `json:"key"`
					ActionType             string   `json:"actionType"`
					DecisionState          string   `json:"decisionState"`
					SubjectKind            string   `json:"subjectKind"`
					SubjectKey             string   `json:"subjectKey"`
					OwnerKey               string   `json:"ownerKey"`
					RelatedPullRequestKeys []string `json:"relatedPullRequestKeys"`
					Observations           []struct {
						SupportsAction      bool   `json:"supportsAction"`
						SourceCoverageState string `json:"sourceCoverageState"`
					} `json:"observations"`
					SourceInsights []struct {
						InsightKind string `json:"insightKind"`
						Title       string `json:"title"`
					} `json:"sourceInsights"`
				} `json:"topActions"`
				Badges []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
			} `json:"ownerLoadSnapshots"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.OwnerLoadSnapshots) != 2 {
		t.Fatalf("expected latest-run owner load rows only, got %#v", response.Data.OwnerLoadSnapshots)
	}
	row := response.Data.OwnerLoadSnapshots[0]
	if row.OwnerKey != "github:lrsb" || row.SourceInstance != "fixture-source" || row.PersonKey != "person:github:lrsb" || row.LoadStatus != "overloaded" || row.ActionCount != 2 || row.ProductActionCount != 1 {
		t.Fatalf("unexpected owner load row: %#v", row)
	}
	if row.FreshnessState != "fresh" || row.Confidence != 1.0 || !strings.Contains(row.EvidenceRef, "tpm_owner_action_rollup") {
		t.Fatalf("unexpected owner load evidence/quality fields: %#v", row)
	}
	if len(row.TopActions) != 1 || row.TopActions[0].Key != "work-action:test:lrsb:1134" || row.TopActions[0].ActionType != "clear_blocker" || row.TopActions[0].SubjectKind != "pull_request" || row.TopActions[0].SubjectKey != "apache/flink-kubernetes-operator#1134" {
		t.Fatalf("owner load did not expose typed top action: %#v", row.TopActions)
	}
	if len(row.TopActions[0].RelatedPullRequestKeys) != 1 || row.TopActions[0].RelatedPullRequestKeys[0] != "apache/flink-kubernetes-operator#1134" {
		t.Fatalf("owner load top action did not expose related PR key: %#v", row.TopActions[0])
	}
	if len(row.TopActions[0].Observations) != 1 || !row.TopActions[0].Observations[0].SupportsAction || row.TopActions[0].Observations[0].SourceCoverageState != "observed:fixture-source" {
		t.Fatalf("owner load top action leaked cross-source observations: %#v", row.TopActions[0].Observations)
	}
	if len(row.TopActions[0].SourceInsights) != 1 || row.TopActions[0].SourceInsights[0].InsightKind != "blocker_candidate" || strings.Contains(row.TopActions[0].SourceInsights[0].Title, "Other-source") {
		t.Fatalf("owner load top action leaked cross-source insights: %#v", row.TopActions[0].SourceInsights)
	}
	for _, key := range []string{"owner_load:status", "owner_load:product_actions", "owner_load:validation_leads", "owner_load:urgent_actions", "owner_load:needs_review"} {
		if !hasBadge(row.Badges, key) {
			t.Fatalf("missing owner load badge %q from %#v", key, row.Badges)
		}
	}

	blankSourceBody := `{"query":"query { ownerLoadSnapshots(workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"   \") { ownerKey } }"}`
	blankSourceReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(blankSourceBody))
	blankSourceReq.Header.Set("Content-Type", "application/json")
	blankSourceRec := httptest.NewRecorder()
	router.ServeHTTP(blankSourceRec, blankSourceReq)
	var blankSourceResponse struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(blankSourceRec.Body.Bytes(), &blankSourceResponse); err != nil {
		t.Fatalf("blank source graphql response is not JSON: %v", err)
	}
	if len(blankSourceResponse.Errors) == 0 || !strings.Contains(blankSourceResponse.Errors[0].Message, "sourceInstance cannot be blank") {
		t.Fatalf("blank owner-load source query did not return validation error: %s", blankSourceRec.Body.String())
	}
}

func TestGraphQLOwnerLoadSnapshotsAcceptsUnassignedAlias(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	stream := store.Client().Workstream.Create().
		SetKey("workstream:flink-kubernetes-operator").
		SetTitle("Flink Kubernetes Operator").
		SetStatus(workstream.StatusActive).
		SetSourceInstance("fixture-source").
		SaveX(ctx)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:unassigned").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("(unassigned)").
		SetGeneratedAt(time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)).
		SetLoadStatus(workownerloadsnapshot.LoadStatusWatch).
		SetActionCount(1).
		SetNeedsHumanReviewCount(1).
		SetFreshnessState(workownerloadsnapshot.FreshnessStatePartial).
		SetConfidence(0.85).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:(unassigned)").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { ownerLoadSnapshots(workstreamKey: \"workstream:flink-kubernetes-operator\", ownerKey: \"unassigned\", sourceInstance: \"fixture-source\") { ownerKey loadStatus actionCount } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			OwnerLoadSnapshots []struct {
				OwnerKey    string `json:"ownerKey"`
				LoadStatus  string `json:"loadStatus"`
				ActionCount int    `json:"actionCount"`
			} `json:"ownerLoadSnapshots"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.OwnerLoadSnapshots) != 1 || response.Data.OwnerLoadSnapshots[0].OwnerKey != "(unassigned)" {
		t.Fatalf("expected unassigned alias to resolve to stored row, got %#v", response.Data.OwnerLoadSnapshots)
	}
}

func TestGraphQLOwnerLoadSnapshotsUsesLatestWorkstreamRunBeforeOwnerFilter(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	stream := store.Client().Workstream.Create().
		SetKey("workstream:flink-kubernetes-operator").
		SetTitle("Flink Kubernetes Operator").
		SetStatus(workstream.StatusActive).
		SetSourceInstance("fixture-source").
		SaveX(ctx)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:stale-owner").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("github:lrsb").
		SetGeneratedAt(time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)).
		SetLoadStatus(workownerloadsnapshot.LoadStatusOverloaded).
		SetActionCount(2).
		SetMaxPriorityScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:github:lrsb").
		SaveX(ctx)
	store.Client().WorkOwnerLoadSnapshot.Create().
		SetKey("work-owner-load:test:clear-current").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetOwnerKey("(clear)").
		SetGeneratedAt(time.Date(2026, 6, 22, 7, 3, 16, 0, time.UTC)).
		SetLoadStatus(workownerloadsnapshot.LoadStatusClear).
		SetActionCount(0).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_owner_load_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-22T07:03:16Z:(clear)").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { stale: ownerLoadSnapshots(workstreamKey: \"workstream:flink-kubernetes-operator\", ownerKey: \"github:lrsb\", sourceInstance: \"fixture-source\") { ownerKey loadStatus } clear: ownerLoadSnapshots(workstreamKey: \"workstream:flink-kubernetes-operator\", sourceInstance: \"fixture-source\") { ownerKey loadStatus actionCount } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Stale []struct {
				OwnerKey   string `json:"ownerKey"`
				LoadStatus string `json:"loadStatus"`
			} `json:"stale"`
			Clear []struct {
				OwnerKey    string `json:"ownerKey"`
				LoadStatus  string `json:"loadStatus"`
				ActionCount int    `json:"actionCount"`
			} `json:"clear"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.Stale) != 0 {
		t.Fatalf("expected disappeared owner to stay absent from latest run, got %#v", response.Data.Stale)
	}
	if len(response.Data.Clear) != 1 || response.Data.Clear[0].OwnerKey != "(clear)" || response.Data.Clear[0].LoadStatus != "clear" || response.Data.Clear[0].ActionCount != 0 {
		t.Fatalf("expected latest clear run marker, got %#v", response.Data.Clear)
	}
}

func TestGraphQLWorkstreamStandupSectionsExposePersistedAgenda(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	stream := store.Client().Workstream.Create().
		SetKey("workstream:flink-kubernetes-operator").
		SetTitle("Flink Kubernetes Operator").
		SetStatus(workstream.StatusActive).
		SetSourceInstance("fixture-source").
		SaveX(ctx)
	health := store.Client().WorkstreamHealthSnapshot.Create().
		SetKey("workstream-health-snapshot:test:flink").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)).
		SetOperatingStatus(workstreamhealthsnapshot.OperatingStatusAttentionRequired).
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream_health_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z").
		SaveX(ctx)
	oldHealth := store.Client().WorkstreamHealthSnapshot.Create().
		SetKey("workstream-health-snapshot:test:old").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 20, 7, 3, 16, 0, time.UTC)).
		SetOperatingStatus(workstreamhealthsnapshot.OperatingStatusAttentionRequired).
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream_health_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-20T07:03:16Z").
		SaveX(ctx)
	action := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:standup-one").
		SetActionType(workaction.ActionTypeClearBlocker).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#1079").
		SetOwnerKey("github:owner").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:standup-one").
		SaveX(ctx)
	sectionEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:standup-section").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("workstream_standup_section").
		SetClaimTargetID(1).
		SetClaimField("recommended_action").
		SetLocatorKind("github_pull_body").
		SetLocator("span").
		SetSourceSpanKey("standup-section:span").
		SetExcerpt("Clear blocker candidate. Ask owner for next step.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_generated_evidence").
		SetExternalID("standup-section").
		SaveX(ctx)
	section := store.Client().WorkstreamStandupSection.Create().
		SetKey("workstream-standup-section:test:one").
		SetWorkstreamHealthSnapshot(health).
		SetWorkstream(stream).
		SetWorkAction(action).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 21, 7, 3, 16, 0, time.UTC)).
		SetSectionRank(1).
		SetSectionKind(workstreamstandupsection.SectionKindProductAction).
		SetUrgency(workstreamstandupsection.UrgencyCritical).
		SetOwnerKey("github:owner").
		SetSubjectKind(workstreamstandupsection.SubjectKindPullRequest).
		SetSubjectKey("apache/flink-kubernetes-operator#1079").
		SetActionType("clear_blocker").
		SetStatusSignal("still_open").
		SetSummary("Clear blocker candidate").
		SetRecommendedAction("Ask owner for next step.").
		SetEvidenceRef("github_pull_body span https://example.test/pr/1").
		SetLatestEvidence(sectionEvidence).
		SetEvidenceCount(1).
		SetFreshnessState(workstreamstandupsection.FreshnessStateFresh).
		SetConfidence(0.91).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream_standup_section").
		SetExternalID("flink-kubernetes-operator:2026-06-21T07:03:16Z:1").
		SaveX(ctx)
	sectionEvidence.Update().SetClaimTargetID(section.ID).SaveX(ctx)
	store.Client().WorkstreamStandupSection.Create().
		SetKey("workstream-standup-section:test:old").
		SetWorkstreamHealthSnapshot(oldHealth).
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 20, 7, 3, 16, 0, time.UTC)).
		SetSectionRank(1).
		SetSectionKind(workstreamstandupsection.SectionKindProductAction).
		SetUrgency(workstreamstandupsection.UrgencyLow).
		SetSubjectKind(workstreamstandupsection.SubjectKindPullRequest).
		SetSubjectKey("apache/flink-kubernetes-operator#1000").
		SetActionType("clear_blocker").
		SetSummary("Old standup item").
		SetRecommendedAction("Do not mix this old row into the latest standup.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_workstream_standup_section").
		SetExternalID("flink-kubernetes-operator:2026-06-20T07:03:16Z:1").
		SaveX(ctx)
	store.Client().WorkstreamStandupSection.Create().
		SetKey("workstream-standup-section:test:wrong-producer-newer").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 23, 7, 3, 16, 0, time.UTC)).
		SetSectionRank(1).
		SetSectionKind(workstreamstandupsection.SectionKindProductAction).
		SetUrgency(workstreamstandupsection.UrgencyCritical).
		SetSubjectKind(workstreamstandupsection.SubjectKindPullRequest).
		SetSubjectKey("apache/flink-kubernetes-operator#8888").
		SetActionType("clear_blocker").
		SetSummary("Wrong producer standup item").
		SetRecommendedAction("Do not let a same-source wrong producer win the TPM agenda.").
		SetSourceSystem("manual_import").
		SetSourceInstance("fixture-source").
		SetExternalKind("manual_standup_section").
		SetExternalID("flink-kubernetes-operator:2026-06-23T07:03:16Z:1").
		SaveX(ctx)
	otherSourceHealth := store.Client().WorkstreamHealthSnapshot.Create().
		SetKey("workstream-health-snapshot:test:other-source").
		SetWorkstream(stream).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 22, 7, 3, 16, 0, time.UTC)).
		SetOperatingStatus(workstreamhealthsnapshot.OperatingStatusAttentionRequired).
		SetSourceInstance("other-source").
		SetExternalKind("tpm_workstream_health_snapshot").
		SetExternalID("flink-kubernetes-operator:2026-06-22T07:03:16Z").
		SaveX(ctx)
	otherSourceAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:other-source-standup").
		SetActionType(workaction.ActionTypeClearBlocker).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("apache/flink-kubernetes-operator#9999").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:other-source-standup").
		SaveX(ctx)
	store.Client().WorkstreamStandupSection.Create().
		SetKey("workstream-standup-section:test:other-source-newer").
		SetWorkstreamHealthSnapshot(otherSourceHealth).
		SetWorkstream(stream).
		SetWorkAction(otherSourceAction).
		SetWorkstreamKey("flink-kubernetes-operator").
		SetGeneratedAt(time.Date(2026, 6, 22, 7, 3, 16, 0, time.UTC)).
		SetSectionRank(1).
		SetSectionKind(workstreamstandupsection.SectionKindProductAction).
		SetUrgency(workstreamstandupsection.UrgencyCritical).
		SetSubjectKind(workstreamstandupsection.SubjectKindPullRequest).
		SetSubjectKey("apache/flink-kubernetes-operator#9999").
		SetActionType("clear_blocker").
		SetSummary("Other-source newer standup item").
		SetRecommendedAction("Do not mix this newer row into fixture-source standup.").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_workstream_standup_section").
		SetExternalID("flink-kubernetes-operator:2026-06-22T07:03:16Z:1").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workstreamStandupSections(workstreamKey: \"workstream:flink-kubernetes-operator\", sectionKind: \"product_action\", sourceInstance: \"fixture-source\") { sourceInstance generatedAt sectionRank sectionKind urgency freshnessState confidence ownerKey subjectKey actionType statusSignal summary recommendedAction evidenceRef action { key sourceInstance decisionState subjectKey } } workstreamStandup(sourceInstance: \"fixture-source\") { sourceInstance sections { sourceInstance generatedAt sectionRank sectionKind freshnessState confidence subjectKey action { key sourceInstance } } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkstreamStandupSections []struct {
				SourceInstance    string  `json:"sourceInstance"`
				GeneratedAt       string  `json:"generatedAt"`
				SectionRank       int     `json:"sectionRank"`
				SectionKind       string  `json:"sectionKind"`
				Urgency           string  `json:"urgency"`
				FreshnessState    string  `json:"freshnessState"`
				Confidence        float64 `json:"confidence"`
				OwnerKey          string  `json:"ownerKey"`
				SubjectKey        string  `json:"subjectKey"`
				ActionType        string  `json:"actionType"`
				StatusSignal      string  `json:"statusSignal"`
				Summary           string  `json:"summary"`
				RecommendedAction string  `json:"recommendedAction"`
				EvidenceRef       string  `json:"evidenceRef"`
				Action            struct {
					Key            string `json:"key"`
					SourceInstance string `json:"sourceInstance"`
					DecisionState  string `json:"decisionState"`
					SubjectKey     string `json:"subjectKey"`
				} `json:"action"`
			} `json:"workstreamStandupSections"`
			WorkstreamStandup struct {
				SourceInstance string `json:"sourceInstance"`
				Sections       []struct {
					SourceInstance string  `json:"sourceInstance"`
					GeneratedAt    string  `json:"generatedAt"`
					SectionRank    int     `json:"sectionRank"`
					SectionKind    string  `json:"sectionKind"`
					FreshnessState string  `json:"freshnessState"`
					Confidence     float64 `json:"confidence"`
					SubjectKey     string  `json:"subjectKey"`
					Action         *struct {
						Key            string `json:"key"`
						SourceInstance string `json:"sourceInstance"`
					} `json:"action"`
				} `json:"sections"`
			} `json:"workstreamStandup"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkstreamStandupSections) != 1 {
		t.Fatalf("expected one standup section, got %#v", response.Data.WorkstreamStandupSections)
	}
	row := response.Data.WorkstreamStandupSections[0]
	if row.SourceInstance != "fixture-source" || row.SectionRank != 1 || row.SectionKind != "product_action" || row.Urgency != "critical" || row.FreshnessState != "fresh" || row.Confidence != 0.91 || row.Action.Key != "tpm-action:test:standup-one" || row.Action.SourceInstance != "fixture-source" {
		t.Fatalf("unexpected standup section: %#v", row)
	}
	if row.SubjectKey != "apache/flink-kubernetes-operator#1079" || row.Action.DecisionState != "product_action" || !strings.Contains(row.EvidenceRef, "github_pull_body") {
		t.Fatalf("unexpected standup section action/evidence fields: %#v", row)
	}
	if len(response.Data.WorkstreamStandup.Sections) != 1 {
		t.Fatalf("expected main standup to use latest persisted section only, got %#v", response.Data.WorkstreamStandup.Sections)
	}
	mainSection := response.Data.WorkstreamStandup.Sections[0]
	if response.Data.WorkstreamStandup.SourceInstance != "fixture-source" || mainSection.SourceInstance != "fixture-source" || mainSection.SubjectKey != "apache/flink-kubernetes-operator#1079" || mainSection.Action == nil || mainSection.Action.Key != "tpm-action:test:standup-one" || mainSection.Action.SourceInstance != "fixture-source" {
		t.Fatalf("main standup did not use persisted latest agenda section: %#v", mainSection)
	}
}

func TestGraphQLWorkActionSummaryCountsOpenActionsAndBadges(t *testing.T) {
	ctx := context.Background()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	pr := store.Client().PullRequest.Create().
		SetKey("pull-request:test:repo/example#2").
		SetRepository("repo/example").
		SetNumber(2).
		SetTitle("Example blocker PR").
		SaveX(ctx)
	blockerInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:blocker").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityHigh).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#2").
		SetPullRequest(pr).
		SetTitle("Possible blocker signal").
		SetScore(90).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:blocker").
		SaveX(ctx)
	statusInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:status").
		SetInsightKind(workinsight.InsightKindStatusSummary).
		SetSeverity(workinsight.SeverityHigh).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#2").
		SetPullRequest(pr).
		SetTitle("CI check state needs review").
		SetScore(84).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:status").
		SaveX(ctx)
	forecastInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:forecast-risk").
		SetInsightKind(workinsight.InsightKindForecastRisk).
		SetSeverity(workinsight.SeverityCritical).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#2").
		SetPullRequest(pr).
		SetTitle("Forecast risk needs owner decision").
		SetScore(91).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:forecast-risk").
		SaveX(ctx)
	qualityInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:forecast-quality").
		SetInsightKind(workinsight.InsightKindModelQuality).
		SetSeverity(workinsight.SeverityMedium).
		SetSubjectKind(workinsight.SubjectKindUnknown).
		SetSubjectKey("flink-pr-cycle-forecast").
		SetTitle("Forecast model is not ETA-ready").
		SetDetails("Backtest over 60 merged PRs: median baseline MAE 8.71d, heuristic MAE 11.10d, random forest MAE 10.41d; best K-fold model median_cycle_baseline. Use cycle output as TPM risk triage, not an ETA promise.").
		SetRecommendedAction("Use PR cycle forecasts as prioritization signals only.").
		SetModelMethod("forecast_backtest_quality_gate").
		SetScore(72).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:forecast-quality").
		SaveX(ctx)
	otherSourceInsight := store.Client().WorkInsight.Create().
		SetKey("work-insight:test:other-source-linked").
		SetInsightKind(workinsight.InsightKindBlockerCandidate).
		SetSeverity(workinsight.SeverityCritical).
		SetSubjectKind(workinsight.SubjectKindPullRequest).
		SetSubjectKey("repo/example#2").
		SetPullRequest(pr).
		SetTitle("Other source insight should not leak").
		SetScore(99).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_insight").
		SetExternalID("work-insight:test:other-source-linked").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:blocker:gold").
		SetInsight(blockerInsight).
		SetReviewKind(workinsightreview.ReviewKindEvaluationLabel).
		SetReviewState(workinsightreview.ReviewStateAccepted).
		SetTruthLabel(workinsightreview.TruthLabelTruePositive).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelActionable).
		SetReviewerKind(workinsightreview.ReviewerKindImported).
		SetLabelSet("agent_gold").
		SetLabelQuality(workinsightreview.LabelQualityGold).
		SetMeasurementEligible(true).
		SetReviewerKey("codex_agent_adjudication").
		SetNextAction("Fixture source action.").
		SetRationale("Fixture source measurement label.").
		SetSourceSystem("cubicle_evaluation").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_review_label").
		SetExternalID("work-insight-review:test:blocker:gold").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:status:triage").
		SetInsight(statusInsight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetReviewerKey("flink_tpm_analytics").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight_review").
		SetExternalID("work-insight-review:test:status:triage").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:forecast:triage").
		SetInsight(forecastInsight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetReviewerKey("flink_tpm_analytics").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight_review").
		SetExternalID("work-insight-review:test:forecast:triage").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:model-quality:triage").
		SetInsight(qualityInsight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetReviewerKey("flink_tpm_analytics").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_insight_review").
		SetExternalID("work-insight-review:test:model-quality:triage").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:blocker:other-source").
		SetInsight(blockerInsight).
		SetReviewKind(workinsightreview.ReviewKindHumanAssessment).
		SetReviewState(workinsightreview.ReviewStateDismissed).
		SetTruthLabel(workinsightreview.TruthLabelFalsePositive).
		SetActionabilityLabel(workinsightreview.ActionabilityLabelNotActionable).
		SetReviewerKind(workinsightreview.ReviewerKindHuman).
		SetLabelSet("other_source_gold").
		SetLabelQuality(workinsightreview.LabelQualityGold).
		SetMeasurementEligible(true).
		SetReviewerKey("other_source_reviewer").
		SetNextAction("Cross-source leak should not appear.").
		SetRationale("Cross-source measurement label.").
		SetSourceSystem("cubicle_evaluation").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_review_label").
		SetExternalID("work-insight-review:test:blocker:other-source").
		SaveX(ctx)
	store.Client().WorkInsightReview.Create().
		SetKey("work-insight-review:test:status:manual-wrong-producer").
		SetInsight(statusInsight).
		SetReviewKind(workinsightreview.ReviewKindTriageRequest).
		SetReviewState(workinsightreview.ReviewStateRequested).
		SetReviewerKind(workinsightreview.ReviewerKindSystem).
		SetReviewerKey("manual_import").
		SetSourceSystem("manual_import").
		SetSourceInstance("fixture-source").
		SetExternalKind("manual_review").
		SetExternalID("work-insight-review:test:status:manual-wrong-producer").
		SaveX(ctx)
	productAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:product").
		SetActionType(workaction.ActionTypeClearBlocker).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetDecision("ready_for_product_action").
		SetDecisionReason("measurement and model gates passed").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#2").
		SetOwnerKey("github:blocker-owner").
		SetOwnerSource("pr_author").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(95).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:product").
		AddSourceInsights(blockerInsight).
		AddSourceInsights(otherSourceInsight).
		SaveX(ctx)
	store.Client().WorkActionObservation.Create().
		SetKey("work-action-observation:test:product").
		SetAction(productAction).
		SetObservationKind(workactionobservation.ObservationKindSourceState).
		SetSourceCoverageState("observed:authenticated_api_current_observation").
		SetSupportsAction(true).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action_observation").
		SetExternalID("work-action-observation:test:product").
		SaveX(ctx)
	store.Client().WorkActionObservation.Create().
		SetKey("work-action-observation:test:product:other-source").
		SetAction(productAction).
		SetObservationKind(workactionobservation.ObservationKindSourceState).
		SetSourceCoverageState("observed:other-source").
		SetSupportsAction(false).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_action_observation").
		SetExternalID("work-action-observation:test:product:other-source").
		SaveX(ctx)
	validationAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:ci-summary").
		SetActionType(workaction.ActionTypeCiCheckFollowup).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateValidationLead).
		SetDecision("pending_validation").
		SetDecisionReason("required-check semantics are not modeled yet").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#2").
		SetOwnerKey("github:ci-owner").
		SetOwnerSource("pr_author").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(84).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:ci-summary").
		AddSourceInsights(statusInsight).
		SaveX(ctx)
	store.Client().WorkActionObservation.Create().
		SetKey("work-action-observation:test:ci-summary").
		SetAction(validationAction).
		SetObservationKind(workactionobservation.ObservationKindCiSignal).
		SetCiSignal("failing_or_pending").
		SetSupportsAction(false).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action_observation").
		SetExternalID("work-action-observation:test:ci-summary").
		SaveX(ctx)
	modelQualityAction := store.Client().WorkAction.Create().
		SetKey("tpm-action:test:forecast-quality").
		SetActionType(workaction.ActionTypeModelQualityReview).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateModelOrRuleQa).
		SetDecision("review_forecast_quality").
		SetDecisionReason("model quality gate is about readiness, not product escalation").
		SetSubjectKind(workaction.SubjectKindUnknown).
		SetSubjectKey("flink-pr-cycle-forecast").
		SetDueBucket(workaction.DueBucketThisWeek).
		SetRankScore(72).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:forecast-quality").
		AddSourceInsights(qualityInsight).
		SaveX(ctx)
	store.Client().WorkActionObservation.Create().
		SetKey("work-action-observation:test:forecast-quality").
		SetAction(modelQualityAction).
		SetObservationKind(workactionobservation.ObservationKindModelOrRuleQa).
		SetSourceCoverageState("generated:forecast_backtest").
		SetSupportsAction(false).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action_observation").
		SetExternalID("work-action-observation:test:forecast-quality").
		SaveX(ctx)
	forecastEvidence := store.Client().Evidence.Create().
		SetKey("evidence:test:forecast-summary").
		SetClaimKind(evidence.ClaimKindObjectState).
		SetClaimTargetKind("work_forecast_evaluation").
		SetClaimField("readiness_state").
		SetLocatorKind("forecast_backtest").
		SetLocator("summary").
		SetSourceSpanKey("forecast:summary").
		SetExcerpt("transition forecasting is gated because only 1 distinct observed snapshot time exists").
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_generated_evidence").
		SetExternalID("forecast-summary").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:summary").
		SetEvaluationKind(workforecastevaluation.EvaluationKindSummary).
		SetModelName("median_cycle_baseline").
		SetForecastMethod("typed_forecast_backtest_gate").
		SetBestModelName("median_cycle_baseline").
		SetBaselineSampleCount(60).
		SetOpenCandidateCount(20).
		SetClosedUnmergedCount(20).
		SetObservedSnapshotTimeCount(1).
		SetTransitionCandidateCount(0).
		SetTerminalTransitionCandidateCount(0).
		SetTransitionHistoryReady(false).
		SetMedianCycleDays(5.25).
		SetP75CycleDays(11.28).
		SetReadyForEta(false).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetReadinessReason("typed forecast gate: median-cycle baseline wins; not an ETA promise; transition forecasting is gated because only 1 distinct observed snapshot time exists").
		SetLatestEvidence(forecastEvidence).
		SetEvidenceCount(1).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("summary").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:kfold:median").
		SetEvaluationKind(workforecastevaluation.EvaluationKindKfold).
		SetModelName("median_cycle_baseline").
		SetFold(1).
		SetTrainCount(48).
		SetTestCount(12).
		SetMaeDays(8.71).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("kfold:median_cycle_baseline:1").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:kfold:heuristic").
		SetEvaluationKind(workforecastevaluation.EvaluationKindKfold).
		SetModelName("heuristic_percentile").
		SetFold(1).
		SetTrainCount(48).
		SetTestCount(12).
		SetMaeDays(11.10).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("kfold:heuristic_percentile:1").
		SaveX(ctx)
	store.Client().WorkForecastEvaluation.Create().
		SetKey("work-forecast-evaluation:test:kfold:rf").
		SetEvaluationKind(workforecastevaluation.EvaluationKindKfold).
		SetModelName("random_forest_regressor").
		SetFold(1).
		SetTrainCount(48).
		SetTestCount(12).
		SetMaeDays(10.41).
		SetReadinessState(workforecastevaluation.ReadinessStateGated).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_forecast_evaluation").
		SetExternalID("kfold:random_forest_regressor:1").
		SaveX(ctx)
	store.Client().WorkAction.Create().
		SetKey("tpm-action:test:closed").
		SetActionType(workaction.ActionTypeDismissedSignal).
		SetActionState(workaction.ActionStateClosed).
		SetDecisionState(workaction.DecisionStateSuppressedSignal).
		SetDecision("dismissed_or_suppressed").
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#2").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketWatch).
		SetRankScore(10).
		SetClosedAt(time.Date(2026, 6, 21, 5, 30, 0, 0, time.UTC)).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("fixture-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:closed").
		AddSourceInsights(blockerInsight).
		SaveX(ctx)
	store.Client().WorkAction.Create().
		SetKey("tpm-action:test:other-source-high-rank").
		SetActionType(workaction.ActionTypeClearBlocker).
		SetActionState(workaction.ActionStateOpen).
		SetDecisionState(workaction.DecisionStateProductAction).
		SetSubjectKind(workaction.SubjectKindPullRequest).
		SetSubjectKey("repo/example#2").
		SetPullRequest(pr).
		SetDueBucket(workaction.DueBucketNow).
		SetRankScore(999).
		SetSourceSystem("cubicle_analytics").
		SetSourceInstance("other-source").
		SetExternalKind("tpm_work_action").
		SetExternalID("tpm-action:test:other-source-high-rank").
		SaveX(ctx)

	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
		EntClient:                store.Client(),
	})
	body := `{"query":"query { workActions(limit: 1, sourceInstance: \"fixture-source\") { key sourceInstance decisionState sourceInsights { insightKind reviewNextAction reviewRationale reviewerKey } observations { supportsAction } } workActionSummary(sourceInstance: \"fixture-source\") { sourceInstance actionState totalCount productActionCount validationLeadCount modelOrRuleQaCount suppressedSignalCount nowCount supportsActionObservationCount badges { key tone detail } breakdowns { dimension key count } ownerRollups { ownerKey ownerSource actionCount productActionCount validationLeadCount nowCount highPriorityCount maxRankScore badges { key tone detail } topActions { key sourceInstance actionType decisionState } } forecastReadiness { sourceInstance etaForecastReady readinessState forecastMethod bestBacktestModel medianBaselineMaeDays heuristicMaeDays randomForestMaeDays baselineSampleCount openCandidateCount closedUnmergedCount observedSnapshotTimeCount transitionCandidateCount terminalTransitionCandidateCount transitionHistoryReady medianCycleDays p75CycleDays typedEvaluationCount gatedForecastLeadCount detail readinessReason evidenceRef badges { key tone detail } qualityAction { key sourceInstance actionType decisionState sourceInsights { insightKind title score } } } topActions { key sourceInstance decisionState badges { key tone } observations { supportsAction } } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			WorkActions []struct {
				Key            string `json:"key"`
				SourceInstance string `json:"sourceInstance"`
				DecisionState  string `json:"decisionState"`
				SourceInsights []struct {
					InsightKind      string `json:"insightKind"`
					ReviewNextAction string `json:"reviewNextAction"`
					ReviewRationale  string `json:"reviewRationale"`
					ReviewerKey      string `json:"reviewerKey"`
				} `json:"sourceInsights"`
				Observations []struct {
					SupportsAction bool `json:"supportsAction"`
				} `json:"observations"`
			} `json:"workActions"`
			WorkActionSummary struct {
				SourceInstance                 string `json:"sourceInstance"`
				ActionState                    string `json:"actionState"`
				TotalCount                     int    `json:"totalCount"`
				ProductActionCount             int    `json:"productActionCount"`
				ValidationLeadCount            int    `json:"validationLeadCount"`
				ModelOrRuleQaCount             int    `json:"modelOrRuleQaCount"`
				SuppressedSignalCount          int    `json:"suppressedSignalCount"`
				NowCount                       int    `json:"nowCount"`
				SupportsActionObservationCount int    `json:"supportsActionObservationCount"`
				Badges                         []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
				Breakdowns []struct {
					Dimension string `json:"dimension"`
					Key       string `json:"key"`
					Count     int    `json:"count"`
				} `json:"breakdowns"`
				OwnerRollups []struct {
					OwnerKey            string  `json:"ownerKey"`
					OwnerSource         string  `json:"ownerSource"`
					ActionCount         int     `json:"actionCount"`
					ProductActionCount  int     `json:"productActionCount"`
					ValidationLeadCount int     `json:"validationLeadCount"`
					NowCount            int     `json:"nowCount"`
					HighPriorityCount   int     `json:"highPriorityCount"`
					MaxRankScore        float64 `json:"maxRankScore"`
					Badges              []struct {
						Key    string `json:"key"`
						Tone   string `json:"tone"`
						Detail string `json:"detail"`
					} `json:"badges"`
					TopActions []struct {
						Key            string `json:"key"`
						SourceInstance string `json:"sourceInstance"`
						ActionType     string `json:"actionType"`
						DecisionState  string `json:"decisionState"`
					} `json:"topActions"`
				} `json:"ownerRollups"`
				ForecastReadiness struct {
					SourceInstance         string  `json:"sourceInstance"`
					EtaForecastReady       bool    `json:"etaForecastReady"`
					ReadinessState         string  `json:"readinessState"`
					ForecastMethod         string  `json:"forecastMethod"`
					BestBacktestModel      string  `json:"bestBacktestModel"`
					MedianBaselineMaeDays  float64 `json:"medianBaselineMaeDays"`
					HeuristicMaeDays       float64 `json:"heuristicMaeDays"`
					RandomForestMaeDays    float64 `json:"randomForestMaeDays"`
					BaselineSampleCount    int     `json:"baselineSampleCount"`
					OpenCandidateCount     int     `json:"openCandidateCount"`
					ClosedUnmergedCount    int     `json:"closedUnmergedCount"`
					ObservedSnapshotTimes  int     `json:"observedSnapshotTimeCount"`
					TransitionCandidates   int     `json:"transitionCandidateCount"`
					TerminalTransitions    int     `json:"terminalTransitionCandidateCount"`
					TransitionHistoryReady bool    `json:"transitionHistoryReady"`
					MedianCycleDays        float64 `json:"medianCycleDays"`
					P75CycleDays           float64 `json:"p75CycleDays"`
					TypedEvaluationCount   int     `json:"typedEvaluationCount"`
					GatedForecastLeadCount int     `json:"gatedForecastLeadCount"`
					Detail                 string  `json:"detail"`
					ReadinessReason        string  `json:"readinessReason"`
					EvidenceRef            string  `json:"evidenceRef"`
					Badges                 []struct {
						Key    string `json:"key"`
						Tone   string `json:"tone"`
						Detail string `json:"detail"`
					} `json:"badges"`
					QualityAction struct {
						Key            string `json:"key"`
						SourceInstance string `json:"sourceInstance"`
						ActionType     string `json:"actionType"`
						DecisionState  string `json:"decisionState"`
						SourceInsights []struct {
							InsightKind string  `json:"insightKind"`
							Title       string  `json:"title"`
							Score       float64 `json:"score"`
						} `json:"sourceInsights"`
					} `json:"qualityAction"`
				} `json:"forecastReadiness"`
				TopActions []struct {
					Key            string `json:"key"`
					SourceInstance string `json:"sourceInstance"`
					DecisionState  string `json:"decisionState"`
					Badges         []struct {
						Key  string `json:"key"`
						Tone string `json:"tone"`
					} `json:"badges"`
					Observations []struct {
						SupportsAction bool `json:"supportsAction"`
					} `json:"observations"`
				} `json:"topActions"`
			} `json:"workActionSummary"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if len(response.Data.WorkActions) != 1 {
		t.Fatalf("expected one source-scoped work action, got %#v", response.Data.WorkActions)
	}
	topAction := response.Data.WorkActions[0]
	if topAction.Key != "tpm-action:test:product" || topAction.SourceInstance != "fixture-source" || topAction.DecisionState != "product_action" {
		t.Fatalf("workActions source filter leaked another source or wrong row: %#v", topAction)
	}
	if len(topAction.SourceInsights) != 1 || topAction.SourceInsights[0].InsightKind != "blocker_candidate" || len(topAction.Observations) != 1 || !topAction.Observations[0].SupportsAction {
		t.Fatalf("workActions did not preserve same-source insight/observation details: %#v", topAction)
	}
	if topAction.SourceInsights[0].ReviewNextAction != "Fixture source action." || topAction.SourceInsights[0].ReviewerKey != "codex_agent_adjudication" || strings.Contains(topAction.SourceInsights[0].ReviewRationale, "Cross-source") {
		t.Fatalf("workActions source-scoped review details leaked another source: %#v", topAction.SourceInsights[0])
	}
	summary := response.Data.WorkActionSummary
	if summary.SourceInstance != "fixture-source" {
		t.Fatalf("summary did not expose source instance or leaked another source: %#v", summary)
	}
	if summary.ActionState != "open" || summary.TotalCount != 3 || summary.ProductActionCount != 1 || summary.ValidationLeadCount != 1 || summary.ModelOrRuleQaCount != 1 {
		t.Fatalf("unexpected summary counts: %#v", summary)
	}
	if summary.SuppressedSignalCount != 0 || summary.NowCount != 2 || summary.SupportsActionObservationCount != 1 {
		t.Fatalf("unexpected summary coverage counts: %#v", summary)
	}
	if !hasBadge(summary.Badges, "summary:product_actions") || !hasBadge(summary.Badges, "summary:validation_leads") || !hasBadge(summary.Badges, "summary:ci_gated") {
		t.Fatalf("summary badges missing expected keys: %#v", summary.Badges)
	}
	if !hasBreakdown(summary.Breakdowns, "decision_state", "product_action", 1) || !hasBreakdown(summary.Breakdowns, "action_type", "ci_check_followup", 1) || !hasBreakdown(summary.Breakdowns, "owner_key", "github:blocker-owner", 1) {
		t.Fatalf("summary breakdowns missing expected rows: %#v", summary.Breakdowns)
	}
	if len(summary.OwnerRollups) != 3 || summary.OwnerRollups[0].OwnerKey != "github:blocker-owner" {
		t.Fatalf("unexpected owner rollup order: %#v", summary.OwnerRollups)
	}
	productOwner := summary.OwnerRollups[0]
	if productOwner.OwnerSource != "pr_author" || productOwner.ActionCount != 1 || productOwner.ProductActionCount != 1 || productOwner.ValidationLeadCount != 0 || productOwner.NowCount != 1 || productOwner.HighPriorityCount != 1 || productOwner.MaxRankScore != 95 {
		t.Fatalf("unexpected product owner rollup: %#v", productOwner)
	}
	if !hasBadge(productOwner.Badges, "owner:product_actions") || !hasBadge(productOwner.Badges, "owner:due_now") {
		t.Fatalf("product owner badges missing expected keys: %#v", productOwner.Badges)
	}
	if len(productOwner.TopActions) != 1 || productOwner.TopActions[0].Key != "tpm-action:test:product" || productOwner.TopActions[0].SourceInstance != "fixture-source" || productOwner.TopActions[0].ActionType != "clear_blocker" {
		t.Fatalf("unexpected product owner top actions: %#v", productOwner.TopActions)
	}
	validationOwner := summary.OwnerRollups[1]
	if validationOwner.OwnerKey != "github:ci-owner" || validationOwner.ValidationLeadCount != 1 || !hasBadge(validationOwner.Badges, "owner:validation_leads") {
		t.Fatalf("unexpected validation owner rollup: %#v", validationOwner)
	}
	forecast := summary.ForecastReadiness
	if forecast.SourceInstance != "fixture-source" {
		t.Fatalf("forecast readiness did not stay source-scoped: %#v", forecast)
	}
	if forecast.EtaForecastReady || forecast.ReadinessState != "gated" || forecast.ForecastMethod != "typed_forecast_backtest_gate" || forecast.BestBacktestModel != "median_cycle_baseline" {
		t.Fatalf("unexpected forecast readiness state: %#v", forecast)
	}
	if forecast.MedianBaselineMaeDays != 8.71 || forecast.HeuristicMaeDays != 11.10 || forecast.RandomForestMaeDays != 10.41 || forecast.GatedForecastLeadCount != 0 {
		t.Fatalf("unexpected forecast readiness metrics: %#v", forecast)
	}
	if forecast.BaselineSampleCount != 60 || forecast.OpenCandidateCount != 20 || forecast.ClosedUnmergedCount != 20 || forecast.MedianCycleDays != 5.25 || forecast.P75CycleDays != 11.28 || forecast.TypedEvaluationCount != 4 {
		t.Fatalf("unexpected typed forecast readiness fields: %#v", forecast)
	}
	if forecast.ObservedSnapshotTimes != 1 || forecast.TransitionCandidates != 0 || forecast.TerminalTransitions != 0 || forecast.TransitionHistoryReady {
		t.Fatalf("unexpected forecast transition-history gate: %#v", forecast)
	}
	if !strings.Contains(forecast.EvidenceRef, "forecast_backtest") {
		t.Fatalf("forecast readiness did not expose generated evaluation evidence: %#v", forecast)
	}
	if !strings.Contains(forecast.Detail, "not an ETA promise") || !strings.Contains(forecast.ReadinessReason, "median-cycle baseline wins") || !strings.Contains(forecast.ReadinessReason, "only 1 distinct observed snapshot") || !hasBadge(forecast.Badges, "forecast:eta_gated") || !hasBadge(forecast.Badges, "forecast:baseline_wins") || !hasBadge(forecast.Badges, "forecast:transition_history_gated") {
		t.Fatalf("forecast readiness missing detail or badges: %#v", forecast)
	}
	if forecast.QualityAction.Key != "tpm-action:test:forecast-quality" || forecast.QualityAction.SourceInstance != "fixture-source" || forecast.QualityAction.ActionType != "model_quality_review" || len(forecast.QualityAction.SourceInsights) != 1 || forecast.QualityAction.SourceInsights[0].InsightKind != "model_quality" {
		t.Fatalf("forecast readiness missing quality action: %#v", forecast.QualityAction)
	}
	if len(summary.TopActions) == 0 || summary.TopActions[0].Key != "tpm-action:test:product" || summary.TopActions[0].SourceInstance != "fixture-source" {
		t.Fatalf("expected product action first, got %#v", summary.TopActions)
	}
	if !hasActionBadge(summary.TopActions[0].Badges, "coverage:supports_action") || len(summary.TopActions[0].Observations) != 1 || !summary.TopActions[0].Observations[0].SupportsAction {
		t.Fatalf("expected product action support badge/observation, got %#v", summary.TopActions[0])
	}

	unscopedAggregateBody := `{"query":"query { workActionSummary { sourceInstance totalCount topActions { key sourceInstance } forecastReadiness { sourceInstance } } workstreamStandup { sourceInstance actionItemCount sections { sourceInstance action { sourceInstance } } } }"}`
	unscopedAggregateReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(unscopedAggregateBody))
	unscopedAggregateReq.Header.Set("Content-Type", "application/json")
	unscopedAggregateRec := httptest.NewRecorder()
	router.ServeHTTP(unscopedAggregateRec, unscopedAggregateReq)
	if unscopedAggregateRec.Code != http.StatusOK {
		t.Fatalf("expected unscoped aggregate status 200, got %d: %s", unscopedAggregateRec.Code, unscopedAggregateRec.Body.String())
	}
	var unscopedAggregateResponse struct {
		Data struct {
			WorkActionSummary struct {
				SourceInstance string `json:"sourceInstance"`
				TotalCount     int    `json:"totalCount"`
				TopActions     []struct {
					Key            string `json:"key"`
					SourceInstance string `json:"sourceInstance"`
				} `json:"topActions"`
				ForecastReadiness struct {
					SourceInstance string `json:"sourceInstance"`
				} `json:"forecastReadiness"`
			} `json:"workActionSummary"`
			WorkstreamStandup struct {
				SourceInstance  string `json:"sourceInstance"`
				ActionItemCount int    `json:"actionItemCount"`
				Sections        []struct {
					SourceInstance string `json:"sourceInstance"`
					Action         *struct {
						SourceInstance string `json:"sourceInstance"`
					} `json:"action"`
				} `json:"sections"`
			} `json:"workstreamStandup"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(unscopedAggregateRec.Body.Bytes(), &unscopedAggregateResponse); err != nil {
		t.Fatalf("unscoped aggregate graphql response is not JSON: %v", err)
	}
	if len(unscopedAggregateResponse.Errors) > 0 {
		t.Fatalf("unscoped aggregate graphql response had errors: %#v", unscopedAggregateResponse.Errors)
	}
	unscopedSummary := unscopedAggregateResponse.Data.WorkActionSummary
	if unscopedSummary.SourceInstance != "fixture-source" || unscopedSummary.ForecastReadiness.SourceInstance != "fixture-source" || unscopedSummary.TotalCount != 3 {
		t.Fatalf("unscoped action summary did not resolve to one source: %#v", unscopedSummary)
	}
	for _, action := range unscopedSummary.TopActions {
		if action.SourceInstance != "fixture-source" {
			t.Fatalf("unscoped action summary leaked another source action: %#v", unscopedSummary.TopActions)
		}
	}
	unscopedStandup := unscopedAggregateResponse.Data.WorkstreamStandup
	if unscopedStandup.SourceInstance != "fixture-source" || unscopedStandup.ActionItemCount != 3 {
		t.Fatalf("unscoped standup did not resolve to one source: %#v", unscopedStandup)
	}
	for _, section := range unscopedStandup.Sections {
		if section.SourceInstance != "" && section.SourceInstance != "fixture-source" {
			t.Fatalf("unscoped standup leaked another source section: %#v", unscopedStandup.Sections)
		}
		if section.Action != nil && section.Action.SourceInstance != "fixture-source" {
			t.Fatalf("unscoped standup leaked another source action: %#v", unscopedStandup.Sections)
		}
	}

	standupBody := `{"query":"query { workstreamStandup(sourceInstance: \"fixture-source\") { sourceInstance operatingStatus actionItemCount productActionCount validationLeadCount criticalOrHighValidationLeadCount modelOrRuleQaCount ownerCount topOwnerActionCount nowCount etaForecastReady recommendedCadenceFocus forecastReadiness { sourceInstance readinessState bestBacktestModel } sections { sourceInstance sectionKind urgency ownerKey subjectKey actionType statusSignal summary recommendedAction evidenceRef action { key sourceInstance actionType decisionState } } } }"}`
	standupReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(standupBody))
	standupReq.Header.Set("Content-Type", "application/json")
	standupRec := httptest.NewRecorder()
	router.ServeHTTP(standupRec, standupReq)
	if standupRec.Code != http.StatusOK {
		t.Fatalf("expected standup status 200, got %d: %s", standupRec.Code, standupRec.Body.String())
	}
	var standupResponse struct {
		Data struct {
			WorkstreamStandup struct {
				SourceInstance                    string `json:"sourceInstance"`
				OperatingStatus                   string `json:"operatingStatus"`
				ActionItemCount                   int    `json:"actionItemCount"`
				ProductActionCount                int    `json:"productActionCount"`
				ValidationLeadCount               int    `json:"validationLeadCount"`
				CriticalOrHighValidationLeadCount int    `json:"criticalOrHighValidationLeadCount"`
				ModelOrRuleQaCount                int    `json:"modelOrRuleQaCount"`
				OwnerCount                        int    `json:"ownerCount"`
				TopOwnerActionCount               int    `json:"topOwnerActionCount"`
				NowCount                          int    `json:"nowCount"`
				EtaForecastReady                  bool   `json:"etaForecastReady"`
				RecommendedCadenceFocus           string `json:"recommendedCadenceFocus"`
				ForecastReadiness                 struct {
					SourceInstance    string `json:"sourceInstance"`
					ReadinessState    string `json:"readinessState"`
					BestBacktestModel string `json:"bestBacktestModel"`
				} `json:"forecastReadiness"`
				Sections []struct {
					SourceInstance    string `json:"sourceInstance"`
					SectionKind       string `json:"sectionKind"`
					Urgency           string `json:"urgency"`
					OwnerKey          string `json:"ownerKey"`
					SubjectKey        string `json:"subjectKey"`
					ActionType        string `json:"actionType"`
					StatusSignal      string `json:"statusSignal"`
					Summary           string `json:"summary"`
					RecommendedAction string `json:"recommendedAction"`
					EvidenceRef       string `json:"evidenceRef"`
					Action            struct {
						Key            string `json:"key"`
						SourceInstance string `json:"sourceInstance"`
						ActionType     string `json:"actionType"`
						DecisionState  string `json:"decisionState"`
					} `json:"action"`
				} `json:"sections"`
			} `json:"workstreamStandup"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(standupRec.Body.Bytes(), &standupResponse); err != nil {
		t.Fatalf("standup graphql response is not JSON: %v", err)
	}
	if len(standupResponse.Errors) > 0 {
		t.Fatalf("standup graphql response had errors: %#v", standupResponse.Errors)
	}
	standup := standupResponse.Data.WorkstreamStandup
	if standup.SourceInstance != "fixture-source" || standup.ForecastReadiness.SourceInstance != "fixture-source" {
		t.Fatalf("standup did not stay source-scoped: %#v", standup)
	}
	if standup.OperatingStatus != "attention_required" || standup.ActionItemCount != 3 || standup.ProductActionCount != 1 || standup.ValidationLeadCount != 1 || standup.ModelOrRuleQaCount != 1 {
		t.Fatalf("unexpected standup counts: %#v", standup)
	}
	if standup.CriticalOrHighValidationLeadCount != 1 || standup.OwnerCount != 2 || standup.TopOwnerActionCount != 1 || standup.NowCount != 2 || standup.EtaForecastReady {
		t.Fatalf("unexpected standup operating fields: %#v", standup)
	}
	if standup.ForecastReadiness.ReadinessState != "gated" || standup.ForecastReadiness.BestBacktestModel != "median_cycle_baseline" || !strings.Contains(standup.RecommendedCadenceFocus, "keep forecast output as risk triage") {
		t.Fatalf("unexpected standup forecast/cadence fields: %#v", standup)
	}
	if !hasStandupSection(standup.Sections, "product_action", "tpm-action:test:product") ||
		!hasStandupSection(standup.Sections, "validation_lead", "tpm-action:test:ci-summary") ||
		!hasStandupSection(standup.Sections, "model_quality", "tpm-action:test:forecast-quality") {
		t.Fatalf("standup sections missing expected action drilldowns: %#v", standup.Sections)
	}
	for _, section := range standup.Sections {
		if section.SourceInstance != "" && section.SourceInstance != "fixture-source" {
			t.Fatalf("standup section leaked another source: %#v", section)
		}
		if section.Action.Key != "" && section.Action.SourceInstance != "fixture-source" {
			t.Fatalf("standup section action leaked another source: %#v", section)
		}
	}

	blankSourceBody := `{"query":"query { workActionSummary(sourceInstance: \"   \") { totalCount } }"}`
	blankSourceReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(blankSourceBody))
	blankSourceReq.Header.Set("Content-Type", "application/json")
	blankSourceRec := httptest.NewRecorder()
	router.ServeHTTP(blankSourceRec, blankSourceReq)
	if blankSourceRec.Code != http.StatusOK {
		t.Fatalf("expected blank source validation status 200, got %d: %s", blankSourceRec.Code, blankSourceRec.Body.String())
	}
	var blankSourceResponse struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(blankSourceRec.Body.Bytes(), &blankSourceResponse); err != nil {
		t.Fatalf("blank source graphql response is not JSON: %v", err)
	}
	if len(blankSourceResponse.Errors) == 0 || !strings.Contains(blankSourceResponse.Errors[0].Message, "sourceInstance cannot be blank") {
		t.Fatalf("blank source query did not return validation error: %s", blankSourceRec.Body.String())
	}

	reviewsBody := `{"query":"query { requested: workInsightReviews(limit: 10, sourceInstance: \"fixture-source\", reviewState: \"requested\") { key sourceInstance sourceSystem externalKind reviewKind reviewState measurementEligible reviewerKey reviewNextAction reviewRationale insight { insightKind subjectKey } badges { key tone } } measured: workInsightReviews(limit: 10, sourceInstance: \"fixture-source\", measurementEligible: true) { key sourceInstance sourceSystem externalKind reviewKind reviewState truthLabel actionabilityLabel labelQuality measurementEligible reviewerKey reviewNextAction reviewRationale insight { insightKind subjectKey } badges { key tone } } }"}`
	reviewsReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(reviewsBody))
	reviewsReq.Header.Set("Content-Type", "application/json")
	reviewsRec := httptest.NewRecorder()
	router.ServeHTTP(reviewsRec, reviewsReq)
	if reviewsRec.Code != http.StatusOK {
		t.Fatalf("expected reviews status 200, got %d: %s", reviewsRec.Code, reviewsRec.Body.String())
	}
	var reviewsResponse struct {
		Data struct {
			Requested []struct {
				Key                 string `json:"key"`
				SourceInstance      string `json:"sourceInstance"`
				SourceSystem        string `json:"sourceSystem"`
				ExternalKind        string `json:"externalKind"`
				ReviewKind          string `json:"reviewKind"`
				ReviewState         string `json:"reviewState"`
				MeasurementEligible bool   `json:"measurementEligible"`
				ReviewerKey         string `json:"reviewerKey"`
				ReviewNextAction    string `json:"reviewNextAction"`
				ReviewRationale     string `json:"reviewRationale"`
				Insight             struct {
					InsightKind string `json:"insightKind"`
					SubjectKey  string `json:"subjectKey"`
				} `json:"insight"`
				Badges []struct {
					Key  string `json:"key"`
					Tone string `json:"tone"`
				} `json:"badges"`
			} `json:"requested"`
			Measured []struct {
				Key                 string `json:"key"`
				SourceInstance      string `json:"sourceInstance"`
				SourceSystem        string `json:"sourceSystem"`
				ExternalKind        string `json:"externalKind"`
				ReviewKind          string `json:"reviewKind"`
				ReviewState         string `json:"reviewState"`
				TruthLabel          string `json:"truthLabel"`
				ActionabilityLabel  string `json:"actionabilityLabel"`
				LabelQuality        string `json:"labelQuality"`
				MeasurementEligible bool   `json:"measurementEligible"`
				ReviewerKey         string `json:"reviewerKey"`
				ReviewNextAction    string `json:"reviewNextAction"`
				ReviewRationale     string `json:"reviewRationale"`
				Insight             struct {
					InsightKind string `json:"insightKind"`
					SubjectKey  string `json:"subjectKey"`
				} `json:"insight"`
				Badges []struct {
					Key  string `json:"key"`
					Tone string `json:"tone"`
				} `json:"badges"`
			} `json:"measured"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(reviewsRec.Body.Bytes(), &reviewsResponse); err != nil {
		t.Fatalf("reviews graphql response is not JSON: %v", err)
	}
	if len(reviewsResponse.Errors) > 0 {
		t.Fatalf("reviews graphql response had errors: %#v", reviewsResponse.Errors)
	}
	if len(reviewsResponse.Data.Requested) != 3 {
		t.Fatalf("expected three same-source requested review rows, got %#v", reviewsResponse.Data.Requested)
	}
	for _, review := range reviewsResponse.Data.Requested {
		if review.SourceInstance != "fixture-source" || review.SourceSystem != "cubicle_analytics" || review.ExternalKind != "tpm_insight_review" || review.ReviewKind != "triage_request" || review.ReviewState != "requested" || review.MeasurementEligible || review.ReviewerKey == "manual_import" {
			t.Fatalf("requested review queue leaked a wrong source/producer row: %#v", review)
		}
	}
	if len(reviewsResponse.Data.Measured) != 1 {
		t.Fatalf("expected one same-source measurement review row, got %#v", reviewsResponse.Data.Measured)
	}
	measured := reviewsResponse.Data.Measured[0]
	if measured.SourceInstance != "fixture-source" || measured.SourceSystem != "cubicle_evaluation" || measured.ExternalKind != "tpm_review_label" || measured.ReviewKind != "evaluation_label" || measured.ReviewState != "accepted" || measured.TruthLabel != "true_positive" || measured.ActionabilityLabel != "actionable" || measured.LabelQuality != "gold" || !measured.MeasurementEligible || measured.ReviewerKey != "codex_agent_adjudication" || measured.ReviewNextAction != "Fixture source action." || strings.Contains(measured.ReviewRationale, "Cross-source") || measured.Insight.InsightKind != "blocker_candidate" {
		t.Fatalf("measurement review row did not preserve source-scoped gold label details: %#v", measured)
	}

	blankReviewBody := `{"query":"query { workInsightReviews(sourceInstance: \"   \") { key } }"}`
	blankReviewReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(blankReviewBody))
	blankReviewReq.Header.Set("Content-Type", "application/json")
	blankReviewRec := httptest.NewRecorder()
	router.ServeHTTP(blankReviewRec, blankReviewReq)
	var blankReviewResponse struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(blankReviewRec.Body.Bytes(), &blankReviewResponse); err != nil {
		t.Fatalf("blank review graphql response is not JSON: %v", err)
	}
	if len(blankReviewResponse.Errors) == 0 || !strings.Contains(blankReviewResponse.Errors[0].Message, "sourceInstance cannot be blank") {
		t.Fatalf("blank review source query did not return validation error: %s", blankReviewRec.Body.String())
	}

	evaluationBody := `{"query":"query { workInsightEvaluation { currentInsightCount reviewRowCount measurementLabelCount openReviewRequestCount minLabeledTotalRequired minLabeledPerKindRequired minPrecisionRateForProductAction minUsefulSignalRateForProductAction minActionabilityRateForProductAction precisionRate usefulSignalRate actionabilityRate falsePositiveRate measurementCoverageRate readyToMeasurePrecision readyToMeasureActionability readyInsightKindCount productActionReadyKindCount qualityGatedInsightKindCount gatedInsightKindCount recommendedNextStep badges { key tone detail } kinds { insightKind currentInsightCount reviewRowCount measurementLabelCount openReviewRequestCount truthLabeledCount actionabilityLabeledCount truePositiveCount falsePositiveCount partialCount actionableCount needsOwnerCount precisionRate usefulSignalRate actionabilityRate falsePositiveRate measurementCoverageRate requiredLabelCount readyToMeasure readyForProductAction productActionGateState productActionGateReason recommendedAction badges { key tone detail } } } }"}`
	evaluationReq := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(evaluationBody))
	evaluationReq.Header.Set("Content-Type", "application/json")
	evaluationRec := httptest.NewRecorder()
	router.ServeHTTP(evaluationRec, evaluationReq)
	if evaluationRec.Code != http.StatusOK {
		t.Fatalf("expected evaluation status 200, got %d: %s", evaluationRec.Code, evaluationRec.Body.String())
	}
	var evaluationResponse struct {
		Data struct {
			WorkInsightEvaluation struct {
				CurrentInsightCount          int     `json:"currentInsightCount"`
				ReviewRowCount               int     `json:"reviewRowCount"`
				MeasurementLabelCount        int     `json:"measurementLabelCount"`
				OpenReviewRequestCount       int     `json:"openReviewRequestCount"`
				MinLabeledTotalRequired      int     `json:"minLabeledTotalRequired"`
				MinLabeledPerKindRequired    int     `json:"minLabeledPerKindRequired"`
				MinPrecisionRate             float64 `json:"minPrecisionRateForProductAction"`
				MinUsefulSignalRate          float64 `json:"minUsefulSignalRateForProductAction"`
				MinActionabilityRate         float64 `json:"minActionabilityRateForProductAction"`
				PrecisionRate                float64 `json:"precisionRate"`
				UsefulSignalRate             float64 `json:"usefulSignalRate"`
				ActionabilityRate            float64 `json:"actionabilityRate"`
				FalsePositiveRate            float64 `json:"falsePositiveRate"`
				MeasurementCoverageRate      float64 `json:"measurementCoverageRate"`
				ReadyToMeasurePrecision      bool    `json:"readyToMeasurePrecision"`
				ReadyToMeasureActionability  bool    `json:"readyToMeasureActionability"`
				ReadyInsightKindCount        int     `json:"readyInsightKindCount"`
				ProductActionReadyKindCount  int     `json:"productActionReadyKindCount"`
				QualityGatedInsightKindCount int     `json:"qualityGatedInsightKindCount"`
				GatedInsightKindCount        int     `json:"gatedInsightKindCount"`
				RecommendedNextStep          string  `json:"recommendedNextStep"`
				Badges                       []struct {
					Key    string `json:"key"`
					Tone   string `json:"tone"`
					Detail string `json:"detail"`
				} `json:"badges"`
				Kinds []struct {
					InsightKind               string  `json:"insightKind"`
					CurrentInsightCount       int     `json:"currentInsightCount"`
					ReviewRowCount            int     `json:"reviewRowCount"`
					MeasurementLabelCount     int     `json:"measurementLabelCount"`
					OpenReviewRequestCount    int     `json:"openReviewRequestCount"`
					TruthLabeledCount         int     `json:"truthLabeledCount"`
					ActionabilityLabeledCount int     `json:"actionabilityLabeledCount"`
					TruePositiveCount         int     `json:"truePositiveCount"`
					FalsePositiveCount        int     `json:"falsePositiveCount"`
					PartialCount              int     `json:"partialCount"`
					ActionableCount           int     `json:"actionableCount"`
					NeedsOwnerCount           int     `json:"needsOwnerCount"`
					PrecisionRate             float64 `json:"precisionRate"`
					UsefulSignalRate          float64 `json:"usefulSignalRate"`
					ActionabilityRate         float64 `json:"actionabilityRate"`
					FalsePositiveRate         float64 `json:"falsePositiveRate"`
					MeasurementCoverageRate   float64 `json:"measurementCoverageRate"`
					RequiredLabelCount        int     `json:"requiredLabelCount"`
					ReadyToMeasure            bool    `json:"readyToMeasure"`
					ReadyForProductAction     bool    `json:"readyForProductAction"`
					ProductActionGateState    string  `json:"productActionGateState"`
					ProductActionGateReason   string  `json:"productActionGateReason"`
					RecommendedAction         string  `json:"recommendedAction"`
					Badges                    []struct {
						Key    string `json:"key"`
						Tone   string `json:"tone"`
						Detail string `json:"detail"`
					} `json:"badges"`
				} `json:"kinds"`
			} `json:"workInsightEvaluation"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(evaluationRec.Body.Bytes(), &evaluationResponse); err != nil {
		t.Fatalf("evaluation graphql response is not JSON: %v", err)
	}
	if len(evaluationResponse.Errors) > 0 {
		t.Fatalf("evaluation graphql response had errors: %#v", evaluationResponse.Errors)
	}
	evaluation := evaluationResponse.Data.WorkInsightEvaluation
	if evaluation.CurrentInsightCount != 3 || evaluation.ReviewRowCount != 3 || evaluation.MeasurementLabelCount != 1 || evaluation.OpenReviewRequestCount != 2 {
		t.Fatalf("unexpected evaluation counts: %#v", evaluation)
	}
	if evaluation.MinLabeledTotalRequired != 10 || evaluation.MinLabeledPerKindRequired != 10 || evaluation.ReadyToMeasurePrecision || evaluation.ReadyToMeasureActionability {
		t.Fatalf("unexpected evaluation gate state: %#v", evaluation)
	}
	if evaluation.MinPrecisionRate != 0.7 || evaluation.MinUsefulSignalRate != 0.8 || evaluation.MinActionabilityRate != 0.7 {
		t.Fatalf("unexpected product-action thresholds: %#v", evaluation)
	}
	if evaluation.PrecisionRate != 1 || evaluation.UsefulSignalRate != 1 || evaluation.ActionabilityRate != 1 || evaluation.FalsePositiveRate != 0 || evaluation.MeasurementCoverageRate != 1.0/3.0 {
		t.Fatalf("unexpected aggregate evaluation rates: %#v", evaluation)
	}
	if evaluation.ReadyInsightKindCount != 1 || evaluation.ProductActionReadyKindCount != 1 || evaluation.QualityGatedInsightKindCount != 0 || evaluation.GatedInsightKindCount != 3 || !hasBadge(evaluation.Badges, "evaluation:gated_kinds") || !strings.Contains(evaluation.RecommendedNextStep, "forecast_risk") {
		t.Fatalf("unexpected evaluation readiness split: %#v", evaluation)
	}
	blockerKind := findInsightKindEvaluation(evaluation.Kinds, "blocker_candidate")
	if blockerKind == nil || !blockerKind.ReadyToMeasure || !blockerKind.ReadyForProductAction || blockerKind.ProductActionGateState != "passed" || blockerKind.RequiredLabelCount != 1 || blockerKind.TruePositiveCount != 1 || blockerKind.ActionableCount != 1 {
		t.Fatalf("unexpected blocker evaluation row: %#v", blockerKind)
	}
	if blockerKind.PrecisionRate != 1 || blockerKind.UsefulSignalRate != 1 || blockerKind.ActionabilityRate != 1 || blockerKind.FalsePositiveRate != 0 || blockerKind.MeasurementCoverageRate != 1 {
		t.Fatalf("unexpected blocker evaluation rates: %#v", blockerKind)
	}
	forecastKind := findInsightKindEvaluation(evaluation.Kinds, "forecast_risk")
	if forecastKind == nil || forecastKind.ReadyToMeasure || forecastKind.ReadyForProductAction || forecastKind.ProductActionGateState != "measurement_gated" || forecastKind.RequiredLabelCount != 1 || !strings.Contains(forecastKind.RecommendedAction, "keep ETA output gated") {
		t.Fatalf("unexpected forecast evaluation row: %#v", forecastKind)
	}
	if forecastKind.PrecisionRate != 0 || forecastKind.UsefulSignalRate != 0 || forecastKind.ActionabilityRate != 0 || forecastKind.FalsePositiveRate != 0 || forecastKind.MeasurementCoverageRate != 0 {
		t.Fatalf("unexpected forecast evaluation rates: %#v", forecastKind)
	}
}

func TestGraphQLPlaygroundIsMounted(t *testing.T) {
	router := NewRouter(slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cubicle Ontology GraphQL") {
		t.Fatalf("playground response did not contain title: %s", rec.Body.String())
	}
}

func hasBadge(rows []struct {
	Key    string `json:"key"`
	Tone   string `json:"tone"`
	Detail string `json:"detail"`
}, key string) bool {
	for _, row := range rows {
		if row.Key == key {
			return true
		}
	}
	return false
}

func hasActionBadge(rows []struct {
	Key  string `json:"key"`
	Tone string `json:"tone"`
}, key string) bool {
	for _, row := range rows {
		if row.Key == key {
			return true
		}
	}
	return false
}

func hasBreakdown(rows []struct {
	Dimension string `json:"dimension"`
	Key       string `json:"key"`
	Count     int    `json:"count"`
}, dimension string, key string, count int) bool {
	for _, row := range rows {
		if row.Dimension == dimension && row.Key == key && row.Count == count {
			return true
		}
	}
	return false
}

func hasString(rows []string, key string) bool {
	for _, row := range rows {
		if row == key {
			return true
		}
	}
	return false
}

func findAutomationEvidenceNeed(rows []struct {
	Key                string   `json:"key"`
	GateKey            string   `json:"gateKey"`
	EvidenceKind       string   `json:"evidenceKind"`
	Priority           string   `json:"priority"`
	TargetKind         string   `json:"targetKind"`
	TargetKey          string   `json:"targetKey"`
	MetricKey          string   `json:"metricKey"`
	ExecutionState     string   `json:"executionState"`
	BackingActionCount int      `json:"backingActionCount"`
	CurrentCount       int      `json:"currentCount"`
	RequiredCount      int      `json:"requiredCount"`
	MissingCount       int      `json:"missingCount"`
	CurrentRate        *float64 `json:"currentRate"`
	RequiredRate       *float64 `json:"requiredRate"`
	RecommendedAction  string   `json:"recommendedAction"`
	NextExecutionStep  string   `json:"nextExecutionStep"`
}, key string) *struct {
	Key                string   `json:"key"`
	GateKey            string   `json:"gateKey"`
	EvidenceKind       string   `json:"evidenceKind"`
	Priority           string   `json:"priority"`
	TargetKind         string   `json:"targetKind"`
	TargetKey          string   `json:"targetKey"`
	MetricKey          string   `json:"metricKey"`
	ExecutionState     string   `json:"executionState"`
	BackingActionCount int      `json:"backingActionCount"`
	CurrentCount       int      `json:"currentCount"`
	RequiredCount      int      `json:"requiredCount"`
	MissingCount       int      `json:"missingCount"`
	CurrentRate        *float64 `json:"currentRate"`
	RequiredRate       *float64 `json:"requiredRate"`
	RecommendedAction  string   `json:"recommendedAction"`
	NextExecutionStep  string   `json:"nextExecutionStep"`
} {
	for i := range rows {
		if rows[i].Key == key {
			return &rows[i]
		}
	}
	return nil
}

func hasStandupSection(rows []struct {
	SourceInstance    string `json:"sourceInstance"`
	SectionKind       string `json:"sectionKind"`
	Urgency           string `json:"urgency"`
	OwnerKey          string `json:"ownerKey"`
	SubjectKey        string `json:"subjectKey"`
	ActionType        string `json:"actionType"`
	StatusSignal      string `json:"statusSignal"`
	Summary           string `json:"summary"`
	RecommendedAction string `json:"recommendedAction"`
	EvidenceRef       string `json:"evidenceRef"`
	Action            struct {
		Key            string `json:"key"`
		SourceInstance string `json:"sourceInstance"`
		ActionType     string `json:"actionType"`
		DecisionState  string `json:"decisionState"`
	} `json:"action"`
}, sectionKind string, actionKey string) bool {
	for _, row := range rows {
		if row.SectionKind == sectionKind && row.Action.Key == actionKey {
			return true
		}
	}
	return false
}

func findInsightKindEvaluation(rows []struct {
	InsightKind               string  `json:"insightKind"`
	CurrentInsightCount       int     `json:"currentInsightCount"`
	ReviewRowCount            int     `json:"reviewRowCount"`
	MeasurementLabelCount     int     `json:"measurementLabelCount"`
	OpenReviewRequestCount    int     `json:"openReviewRequestCount"`
	TruthLabeledCount         int     `json:"truthLabeledCount"`
	ActionabilityLabeledCount int     `json:"actionabilityLabeledCount"`
	TruePositiveCount         int     `json:"truePositiveCount"`
	FalsePositiveCount        int     `json:"falsePositiveCount"`
	PartialCount              int     `json:"partialCount"`
	ActionableCount           int     `json:"actionableCount"`
	NeedsOwnerCount           int     `json:"needsOwnerCount"`
	PrecisionRate             float64 `json:"precisionRate"`
	UsefulSignalRate          float64 `json:"usefulSignalRate"`
	ActionabilityRate         float64 `json:"actionabilityRate"`
	FalsePositiveRate         float64 `json:"falsePositiveRate"`
	MeasurementCoverageRate   float64 `json:"measurementCoverageRate"`
	RequiredLabelCount        int     `json:"requiredLabelCount"`
	ReadyToMeasure            bool    `json:"readyToMeasure"`
	ReadyForProductAction     bool    `json:"readyForProductAction"`
	ProductActionGateState    string  `json:"productActionGateState"`
	ProductActionGateReason   string  `json:"productActionGateReason"`
	RecommendedAction         string  `json:"recommendedAction"`
	Badges                    []struct {
		Key    string `json:"key"`
		Tone   string `json:"tone"`
		Detail string `json:"detail"`
	} `json:"badges"`
}, insightKind string) *struct {
	InsightKind               string  `json:"insightKind"`
	CurrentInsightCount       int     `json:"currentInsightCount"`
	ReviewRowCount            int     `json:"reviewRowCount"`
	MeasurementLabelCount     int     `json:"measurementLabelCount"`
	OpenReviewRequestCount    int     `json:"openReviewRequestCount"`
	TruthLabeledCount         int     `json:"truthLabeledCount"`
	ActionabilityLabeledCount int     `json:"actionabilityLabeledCount"`
	TruePositiveCount         int     `json:"truePositiveCount"`
	FalsePositiveCount        int     `json:"falsePositiveCount"`
	PartialCount              int     `json:"partialCount"`
	ActionableCount           int     `json:"actionableCount"`
	NeedsOwnerCount           int     `json:"needsOwnerCount"`
	PrecisionRate             float64 `json:"precisionRate"`
	UsefulSignalRate          float64 `json:"usefulSignalRate"`
	ActionabilityRate         float64 `json:"actionabilityRate"`
	FalsePositiveRate         float64 `json:"falsePositiveRate"`
	MeasurementCoverageRate   float64 `json:"measurementCoverageRate"`
	RequiredLabelCount        int     `json:"requiredLabelCount"`
	ReadyToMeasure            bool    `json:"readyToMeasure"`
	ReadyForProductAction     bool    `json:"readyForProductAction"`
	ProductActionGateState    string  `json:"productActionGateState"`
	ProductActionGateReason   string  `json:"productActionGateReason"`
	RecommendedAction         string  `json:"recommendedAction"`
	Badges                    []struct {
		Key    string `json:"key"`
		Tone   string `json:"tone"`
		Detail string `json:"detail"`
	} `json:"badges"`
} {
	for i := range rows {
		if rows[i].InsightKind == insightKind {
			return &rows[i]
		}
	}
	return nil
}

func TestGraphQLPlaygroundCanBeDisabled(t *testing.T) {
	router := NewRouterWithOptions(slog.Default(), RouterOptions{
		GraphQLPlaygroundEnabled: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRequestLoggerUsesURLPathForUnmatchedRoutes(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	router := NewRouterWithOptions(logger, RouterOptions{
		GraphQLPlaygroundEnabled: false,
	})

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "path=/playground") {
		t.Fatalf("expected unmatched request path in logs, got: %s", logs.String())
	}
}
