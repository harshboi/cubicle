package entgraph

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/document"
	"cubicle/services/ontology-service/ent/documentlink"
	"cubicle/services/ontology-service/ent/evidence"
	"cubicle/services/ontology-service/ent/message"
	"cubicle/services/ontology-service/ent/person"
	"cubicle/services/ontology-service/ent/pullrequest"
	"cubicle/services/ontology-service/ent/pullrequestauthorship"
	"cubicle/services/ontology-service/ent/pullrequestreview"
	"cubicle/services/ontology-service/ent/sourcescopestate"
	"cubicle/services/ontology-service/ent/sourcesyncissue"
	"cubicle/services/ontology-service/ent/sourcesyncrun"
	"cubicle/services/ontology-service/ent/ticket"
	"cubicle/services/ontology-service/ent/ticketassignment"
	"cubicle/services/ontology-service/ent/ticketdocument"
	"cubicle/services/ontology-service/ent/ticketmessage"
	"cubicle/services/ontology-service/ent/ticketpullrequest"
	"cubicle/services/ontology-service/ent/unresolvedreference"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/entstore"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/ontology"
)

func TestProductExpanderExpandsTicketToPullRequestFromTypedEntRows(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedTicketPullRequestGraph(t, ctx, client)

	expander := NewProductExpander(client)
	got, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectTicket,
			Key:        "ticket:FLINK-1",
		},
		Depth:          1,
		LimitPerObject: 4,
	})
	if err != nil {
		t.Fatalf("expand ticket: %v", err)
	}

	if len(got.Objects) != 2 {
		t.Fatalf("objects = %#v, want ticket and pull request", got.Objects)
	}
	if got.Objects[0].ObjectType != ontology.ObjectTicket || got.Objects[0].Key != "ticket:FLINK-1" {
		t.Fatalf("start object = %#v, want ticket", got.Objects[0])
	}
	if got.Objects[1].ObjectType != ontology.ObjectPullRequest || got.Objects[1].Key != "pull-request:repo/example#1" {
		t.Fatalf("neighbor object = %#v, want pull request", got.Objects[1])
	}
	if len(got.Associations) != 1 {
		t.Fatalf("associations = %#v, want one ticket/pull-request relationship", got.Associations)
	}
	association := got.Associations[0]
	if association.AssociationType != ontology.AssocImplementedBy || association.From.Key != "ticket:FLINK-1" || association.To.Key != "pull-request:repo/example#1" {
		t.Fatalf("association = %#v, want canonical ticket implemented_by pull request", association)
	}
	if association.Metadata.EvidenceKey != "evidence:ticket-pr" || association.Metadata.Source != "jira" || association.Metadata.Visibility != "public" || association.Metadata.FreshnessState != "fresh" || association.Metadata.Confidence != 1 {
		t.Fatalf("association metadata = %#v, want public fresh source evidence", association.Metadata)
	}
}

func TestProductExpanderDoesNotTraverseSourceDiagnostics(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedTicketPullRequestGraph(t, ctx, client)
	seedSourceDiagnosticsForProductObjects(t, ctx, client)

	expander := NewProductExpander(client)
	got, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectTicket,
			Key:        "ticket:FLINK-1",
		},
		Depth:          2,
		LimitPerObject: 8,
	})
	if err != nil {
		t.Fatalf("expand ticket with source diagnostics present: %v", err)
	}
	if len(got.Objects) != 2 || !hasObject(got.Objects, ontology.ObjectTicket, "ticket:FLINK-1") || !hasObject(got.Objects, ontology.ObjectPullRequest, "pull-request:repo/example#1") {
		t.Fatalf("objects = %#v, want only typed product ticket and PR", got.Objects)
	}
	if len(got.Associations) != 1 || got.Associations[0].AssociationType != ontology.AssocImplementedBy {
		t.Fatalf("associations = %#v, want only typed product relationship", got.Associations)
	}

	_, err = expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: domain.ObjectType("source_sync_issue"),
			Key:        "source-sync-issue:github-rate-limited",
		},
		Depth:          1,
		LimitPerObject: 4,
	})
	if !errors.Is(err, graphstore.ErrMissingObject) {
		t.Fatalf("expand source diagnostic err = %v, want missing object", err)
	}
}

func TestProductExpanderCanStartFromPullRequestWithoutInventingReverseRelationship(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedTicketPullRequestGraph(t, ctx, client)

	expander := NewProductExpander(client)
	got, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectPullRequest,
			Key:        "pull-request:repo/example#1",
		},
		Depth:          1,
		LimitPerObject: 4,
	})
	if err != nil {
		t.Fatalf("expand pull request: %v", err)
	}
	if len(got.Objects) != 2 || got.Objects[0].ObjectType != ontology.ObjectPullRequest || got.Objects[1].ObjectType != ontology.ObjectTicket {
		t.Fatalf("objects = %#v, want pull request start and ticket neighbor", got.Objects)
	}
	if len(got.Associations) != 1 {
		t.Fatalf("associations = %#v, want canonical association", got.Associations)
	}
	if got.Associations[0].From.ObjectType != ontology.ObjectTicket || got.Associations[0].To.ObjectType != ontology.ObjectPullRequest || got.Associations[0].AssociationType != ontology.AssocImplementedBy {
		t.Fatalf("association = %#v, want canonical ticket implemented_by pull request", got.Associations[0])
	}
}

func TestProductExpanderRespectsAssociationTypeFilterAndMissingObjects(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedTicketPullRequestGraph(t, ctx, client)

	expander := NewProductExpander(client)
	filtered, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectTicket,
			Key:        "ticket:FLINK-1",
		},
		AssociationTypes: []domain.AssociationType{ontology.AssocDiscussedIn},
		Depth:            1,
		LimitPerObject:   4,
	})
	if err != nil {
		t.Fatalf("expand with filter: %v", err)
	}
	if len(filtered.Objects) != 1 || len(filtered.Associations) != 0 {
		t.Fatalf("filtered graph = %#v, want only start object", filtered)
	}

	_, err = expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectTicket,
			Key:        "ticket:missing",
		},
		Depth:          1,
		LimitPerObject: 4,
	})
	if !errors.Is(err, graphstore.ErrMissingObject) {
		t.Fatalf("missing err = %v, want ErrMissingObject", err)
	}
}

func TestProductExpanderExpandsDocumentAndMessageTypedRows(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedTicketDocumentMessageGraph(t, ctx, client)

	expander := NewProductExpander(client)
	fromDocument, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectDocument,
			Key:        "document:flink-design",
		},
		Depth:          2,
		LimitPerObject: 4,
	})
	if err != nil {
		t.Fatalf("expand document: %v", err)
	}
	if len(fromDocument.Objects) != 3 {
		t.Fatalf("objects = %#v, want document, ticket, and message", fromDocument.Objects)
	}
	if fromDocument.Objects[0].ObjectType != ontology.ObjectDocument || fromDocument.Objects[0].Key != "document:flink-design" {
		t.Fatalf("start object = %#v, want document", fromDocument.Objects[0])
	}
	if !hasObject(fromDocument.Objects, ontology.ObjectTicket, "ticket:DOC-1") {
		t.Fatalf("objects = %#v, want linked ticket", fromDocument.Objects)
	}
	if !hasObject(fromDocument.Objects, ontology.ObjectMessage, "message:standup-1") {
		t.Fatalf("objects = %#v, want linked message reached through ticket", fromDocument.Objects)
	}
	if len(fromDocument.Associations) != 2 {
		t.Fatalf("associations = %#v, want document and message relationships", fromDocument.Associations)
	}
	if !hasCanonicalAssociation(fromDocument.Associations, ontology.ObjectTicket, "ticket:DOC-1", ontology.AssocDocuments, ontology.ObjectDocument, "document:flink-design") {
		t.Fatalf("associations = %#v, want canonical ticket documented_by document", fromDocument.Associations)
	}
	if !hasCanonicalAssociation(fromDocument.Associations, ontology.ObjectTicket, "ticket:DOC-1", ontology.AssocDiscussedIn, ontology.ObjectMessage, "message:standup-1") {
		t.Fatalf("associations = %#v, want canonical ticket discussed_in message", fromDocument.Associations)
	}

	fromMessage, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectMessage,
			Key:        "message:standup-1",
		},
		Depth:          1,
		LimitPerObject: 4,
	})
	if err != nil {
		t.Fatalf("expand message: %v", err)
	}
	if len(fromMessage.Objects) != 2 || fromMessage.Objects[0].ObjectType != ontology.ObjectMessage || !hasObject(fromMessage.Objects, ontology.ObjectTicket, "ticket:DOC-1") {
		t.Fatalf("objects = %#v, want message start and ticket neighbor", fromMessage.Objects)
	}
	if len(fromMessage.Associations) != 1 || !hasCanonicalAssociation(fromMessage.Associations, ontology.ObjectTicket, "ticket:DOC-1", ontology.AssocDiscussedIn, ontology.ObjectMessage, "message:standup-1") {
		t.Fatalf("associations = %#v, want canonical ticket discussed_in message", fromMessage.Associations)
	}

	documentOnly, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectTicket,
			Key:        "ticket:DOC-1",
		},
		AssociationTypes: []domain.AssociationType{ontology.AssocDocuments},
		Depth:            1,
		LimitPerObject:   4,
	})
	if err != nil {
		t.Fatalf("expand ticket with document filter: %v", err)
	}
	if len(documentOnly.Associations) != 1 || documentOnly.Associations[0].AssociationType != ontology.AssocDocuments {
		t.Fatalf("filtered associations = %#v, want only document relationship", documentOnly.Associations)
	}
	if hasObject(documentOnly.Objects, ontology.ObjectMessage, "message:standup-1") {
		t.Fatalf("filtered objects = %#v, should not include message", documentOnly.Objects)
	}
}

func TestProductExpanderDoesNotUseRawMessageBodyAsObjectTitle(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	secretBody := "secret-token raw message body should not become graph title"
	client.Message.Create().
		SetKey("message:body-only").
		SetBody(secretBody).
		SetChannelKey("private-channel").
		SetSourceSystem("slack").
		SetSourceInstance("example").
		SetExternalKind("slack_message").
		SetExternalID("body-only").
		SetFreshnessState(message.FreshnessStateFresh).
		SetVisibility(message.VisibilityPublic).
		SaveX(ctx)

	expander := NewProductExpander(client)
	got, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectMessage,
			Key:        "message:body-only",
		},
		Depth:          0,
		LimitPerObject: 1,
	})
	if err != nil {
		t.Fatalf("expand message: %v", err)
	}
	if len(got.Objects) != 1 {
		t.Fatalf("objects = %#v, want one body-only message", got.Objects)
	}
	if got.Objects[0].Title != "message:body-only" {
		t.Fatalf("message title = %q, want stable key fallback", got.Objects[0].Title)
	}
	if got.Objects[0].Title == secretBody {
		t.Fatalf("message body leaked as graph title")
	}
}

func TestProductExpanderDoesNotUseMessageSummaryAsObjectTitle(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	secretSummary := "RAW_COMMENT_SHOULD_NOT_REACH_PROMPT"
	client.Message.Create().
		SetKey("message:summary-only").
		SetBody("source body belongs in evidence, not object title").
		SetSummary(secretSummary).
		SetChannelKey("jira-comments").
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_comment").
		SetExternalID("summary-only").
		SetFreshnessState(message.FreshnessStateFresh).
		SetVisibility(message.VisibilityPublic).
		SaveX(ctx)

	expander := NewProductExpander(client)
	got, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectMessage,
			Key:        "message:summary-only",
		},
		Depth:          0,
		LimitPerObject: 1,
	})
	if err != nil {
		t.Fatalf("expand message: %v", err)
	}
	if len(got.Objects) != 1 {
		t.Fatalf("objects = %#v, want one summary-only message", got.Objects)
	}
	if got.Objects[0].Title != "message:summary-only" {
		t.Fatalf("message title = %q, want stable key fallback", got.Objects[0].Title)
	}
	if got.Objects[0].Title == secretSummary {
		t.Fatalf("message summary leaked as graph title")
	}
}

func TestProductExpanderUsesObjectTypeAndKeyForTraversalIdentity(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedTicketDocumentSharedKeyGraph(t, ctx, client)

	expander := NewProductExpander(client)
	got, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectDocument,
			Key:        "shared-key",
		},
		Depth:          1,
		LimitPerObject: 4,
	})
	if err != nil {
		t.Fatalf("expand document with shared key: %v", err)
	}
	if len(got.Objects) != 2 {
		t.Fatalf("objects = %#v, want both document and ticket sharing key", got.Objects)
	}
	if !hasObject(got.Objects, ontology.ObjectDocument, "shared-key") || !hasObject(got.Objects, ontology.ObjectTicket, "shared-key") {
		t.Fatalf("objects = %#v, want distinct object types with same key", got.Objects)
	}
	if len(got.Associations) != 1 || !hasCanonicalAssociation(got.Associations, ontology.ObjectTicket, "shared-key", ontology.AssocDocuments, ontology.ObjectDocument, "shared-key") {
		t.Fatalf("associations = %#v, want shared-key ticket documented_by shared-key document", got.Associations)
	}
}

func TestProductExpanderLimitsMixedRelationshipFanoutByRank(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedTicketDocumentMessageGraph(t, ctx, client)
	seedLowRankPullRequestForDocTicket(t, ctx, client)

	expander := NewProductExpander(client)
	got, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectTicket,
			Key:        "ticket:DOC-1",
		},
		Depth:          1,
		LimitPerObject: 1,
	})
	if err != nil {
		t.Fatalf("expand ticket with mixed fanout limit: %v", err)
	}
	if len(got.Associations) != 1 {
		t.Fatalf("associations = %#v, want only highest-ranked relationship", got.Associations)
	}
	if got.Associations[0].AssociationType != ontology.AssocDocuments || got.Associations[0].To.Key != "document:flink-design" {
		t.Fatalf("association = %#v, want highest-ranked document relationship", got.Associations[0])
	}
	if hasObject(got.Objects, ontology.ObjectMessage, "message:standup-1") || hasObject(got.Objects, ontology.ObjectPullRequest, "pull-request:repo/example#9") {
		t.Fatalf("objects = %#v, want only start and highest-ranked document neighbor", got.Objects)
	}
}

func TestProductExpanderReadFilterRunsBeforeTraversalAndFanout(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedTicketPrivatePublicPullRequestGraph(t, ctx, client)

	expander := NewProductExpander(client)
	publicOnly := domain.ExpandReadFilter{
		PrincipalKey: "user:public-only",
		ObjectAllowed: func(object domain.Object) bool {
			return object.Visibility == "" || object.Visibility == domain.VisibilityPublic
		},
		AssociationAllowed: func(association domain.Association) bool {
			return association.Metadata.Visibility == "" || association.Metadata.Visibility == domain.VisibilityPublic
		},
	}
	publicGraph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:AUTH-1"},
		Depth:          1,
		LimitPerObject: 1,
		ReadFilter:     publicOnly,
	})
	if err != nil {
		t.Fatalf("expand public-only graph: %v", err)
	}
	if !hasObject(publicGraph.Objects, ontology.ObjectPullRequest, "pull-request:repo/example#101") {
		t.Fatalf("public-only objects = %#v, want lower-ranked public PR", publicGraph.Objects)
	}
	if hasObject(publicGraph.Objects, ontology.ObjectPullRequest, "pull-request:repo/example#100") {
		t.Fatalf("public-only objects = %#v, should not include private high-ranked PR", publicGraph.Objects)
	}
	if len(publicGraph.Associations) != 1 || publicGraph.Associations[0].To.Key != "pull-request:repo/example#101" {
		t.Fatalf("public-only associations = %#v, want private candidate skipped before fanout limit", publicGraph.Associations)
	}

	allowAll := domain.ExpandReadFilter{
		PrincipalKey:       "user:allowed",
		ObjectAllowed:      func(domain.Object) bool { return true },
		AssociationAllowed: func(domain.Association) bool { return true },
	}
	allowedGraph, err := expander.Expand(ctx, domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectTicket, Key: "ticket:AUTH-1"},
		Depth:          1,
		LimitPerObject: 1,
		ReadFilter:     allowAll,
	})
	if err != nil {
		t.Fatalf("expand allowed graph: %v", err)
	}
	if !hasObject(allowedGraph.Objects, ontology.ObjectPullRequest, "pull-request:repo/example#100") {
		t.Fatalf("allowed objects = %#v, want high-ranked private PR", allowedGraph.Objects)
	}
	if len(allowedGraph.Associations) != 1 || allowedGraph.Associations[0].To.Key != "pull-request:repo/example#100" {
		t.Fatalf("allowed associations = %#v, want high-ranked private relationship", allowedGraph.Associations)
	}
}

func TestProductExpanderExpandsParticipationAndDocumentLinkRows(t *testing.T) {
	ctx := context.Background()
	client := openProductExpanderTestClient(t, ctx)
	seedParticipationAndDocumentLinkGraph(t, ctx, client)

	expander := NewProductExpander(client)
	fromPullRequest, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectPullRequest,
			Key:        "pull-request:repo/example#7",
		},
		Depth:          1,
		LimitPerObject: 8,
	})
	if err != nil {
		t.Fatalf("expand pull request participation: %v", err)
	}
	if !hasObject(fromPullRequest.Objects, ontology.ObjectPerson, "person:alice") || !hasObject(fromPullRequest.Objects, ontology.ObjectPerson, "person:bob") {
		t.Fatalf("pull-request objects = %#v, want author and reviewer people", fromPullRequest.Objects)
	}
	if !hasCanonicalAssociation(fromPullRequest.Associations, ontology.ObjectPullRequest, "pull-request:repo/example#7", domain.AssociationType("author"), ontology.ObjectPerson, "person:alice") {
		t.Fatalf("pull-request associations = %#v, want author relationship", fromPullRequest.Associations)
	}
	if !hasCanonicalAssociation(fromPullRequest.Associations, ontology.ObjectPullRequest, "pull-request:repo/example#7", domain.AssociationType("approver"), ontology.ObjectPerson, "person:bob") {
		t.Fatalf("pull-request associations = %#v, want approver relationship", fromPullRequest.Associations)
	}

	approverOnly, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectPullRequest,
			Key:        "pull-request:repo/example#7",
		},
		AssociationTypes: []domain.AssociationType{domain.AssociationType("approver")},
		Depth:            1,
		LimitPerObject:   8,
	})
	if err != nil {
		t.Fatalf("expand pull request approver filter: %v", err)
	}
	if len(approverOnly.Associations) != 1 || !hasCanonicalAssociation(approverOnly.Associations, ontology.ObjectPullRequest, "pull-request:repo/example#7", domain.AssociationType("approver"), ontology.ObjectPerson, "person:bob") {
		t.Fatalf("approver filtered associations = %#v, want only approver relationship", approverOnly.Associations)
	}
	if hasCanonicalAssociation(approverOnly.Associations, ontology.ObjectPullRequest, "pull-request:repo/example#7", domain.AssociationType("author"), ontology.ObjectPerson, "person:alice") {
		t.Fatalf("approver filtered associations = %#v, should not include author relationship", approverOnly.Associations)
	}

	fromTicket, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectTicket,
			Key:        "ticket:OWNER-1",
		},
		Depth:          1,
		LimitPerObject: 8,
	})
	if err != nil {
		t.Fatalf("expand ticket assignment: %v", err)
	}
	if !hasCanonicalAssociation(fromTicket.Associations, ontology.ObjectTicket, "ticket:OWNER-1", domain.AssociationType("assignee"), ontology.ObjectPerson, "person:alice") {
		t.Fatalf("ticket associations = %#v, want assignee relationship", fromTicket.Associations)
	}

	fromPerson, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectPerson,
			Key:        "person:alice",
		},
		Depth:          1,
		LimitPerObject: 8,
	})
	if err != nil {
		t.Fatalf("expand person participation: %v", err)
	}
	if !hasObject(fromPerson.Objects, ontology.ObjectTicket, "ticket:OWNER-1") || !hasObject(fromPerson.Objects, ontology.ObjectPullRequest, "pull-request:repo/example#7") {
		t.Fatalf("person objects = %#v, want ticket and pull request participation", fromPerson.Objects)
	}

	fromDocument, err := expander.Expand(ctx, domain.ExpandRequest{
		Start: domain.ObjectRef{
			ObjectType: ontology.ObjectDocument,
			Key:        "document:api-plan",
		},
		Depth:          1,
		LimitPerObject: 8,
	})
	if err != nil {
		t.Fatalf("expand document links: %v", err)
	}
	if !hasCanonicalAssociation(fromDocument.Associations, ontology.ObjectDocument, "document:api-plan", domain.AssociationType("links_to"), ontology.ObjectDocument, "document:api-reference") {
		t.Fatalf("document associations = %#v, want links_to relationship", fromDocument.Associations)
	}
}

func openProductExpanderTestClient(t *testing.T, ctx context.Context) *genent.Client {
	t.Helper()
	store, err := entstore.Open(ctx, entstore.Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
		BusyTimeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("close ent store: %v", err)
		}
	})
	return store.Client()
}

func seedTicketPullRequestGraph(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	ticketRow := client.Ticket.Create().
		SetKey("ticket:FLINK-1").
		SetTitle("Example ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("apache-flink").
		SetExternalKind("jira_issue").
		SetExternalID("FLINK-1").
		SetSourceURL("https://issues.example.test/FLINK-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	prRow := client.PullRequest.Create().
		SetKey("pull-request:repo/example#1").
		SetRepository("repo/example").
		SetNumber(1).
		SetTitle("Example PR").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#1").
		SetSourceURL("https://github.com/repo/example/pull/1").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	evidenceRow := client.Evidence.Create().
		SetKey("evidence:ticket-pr").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_pull_request").
		SetRelationshipKind("implemented_by").
		SetLocatorKind("jira_remote_link").
		SetLocator("FLINK-1 remote links").
		SetSourceSystem("jira").
		SetSourceInstance("apache-flink").
		SetExternalKind("jira_remote_link").
		SetExternalID("FLINK-1->repo/example#1").
		SetSourceURL("https://issues.example.test/FLINK-1").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	client.TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(prRow).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance("apache-flink").
		SetExternalKind("jira_remote_link").
		SetExternalID("FLINK-1->repo/example#1").
		SetSourceURL("https://issues.example.test/FLINK-1").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(evidenceRow).
		SaveX(ctx)
}

func seedSourceDiagnosticsForProductObjects(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 10, 30, 0, 0, time.UTC)
	connection := client.SourceConnection.Create().
		SetKey("source-connection:github:repo-example").
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetDisplayName("repo/example GitHub").
		SetConnectorKind("github").
		SaveX(ctx)
	scope := client.SourceScope.Create().
		SetKey("source-scope:github:repo-example:pulls").
		SetConnection(connection).
		SetScopeKind("repository").
		SetScopeKey("repo/example").
		SetDisplayName("repo/example pulls").
		SaveX(ctx)
	run := client.SourceSyncRun.Create().
		SetScope(scope).
		SetRunKey("source-sync-run:github-rate-limited").
		SetSyncMode(sourcesyncrun.SyncModeSnapshot).
		SetCoverageMode(sourcesyncrun.CoverageModePartialScope).
		SetStatus(sourcesyncrun.StatusRateLimited).
		SetStartedAt(observedAt).
		SetCompletedAt(observedAt.Add(time.Minute)).
		SetObjectsSeenCount(1).
		SetIssuesCreatedCount(1).
		SetErrorCode("github_rate_limited").
		SetErrorMessage("rate limited while hydrating pull request details").
		SaveX(ctx)
	client.SourceScopeState.Create().
		SetScope(scope).
		SetFreshnessState(sourcescopestate.FreshnessStatePartial).
		SetCoverageMode(sourcescopestate.CoverageModePartialScope).
		SetLastAttemptedAt(observedAt).
		SetErrorCode("github_rate_limited").
		SetErrorMessage("coverage is partial for PR detail hydration").
		SaveX(ctx)
	client.SourceSyncIssue.Create().
		SetScope(scope).
		SetSyncRun(run).
		SetSeverity(sourcesyncissue.SeverityWarning).
		SetIssueCode("github_rate_limited").
		SetMessage("GitHub detail hydration returned 429 for pull-request:repo/example#1").
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#1").
		SetSourceURL("https://github.com/repo/example/pull/1").
		SaveX(ctx)
	evidenceRow := client.Evidence.Query().Where(evidence.KeyEQ("evidence:ticket-pr")).OnlyX(ctx)
	client.UnresolvedReference.Create().
		SetFromProductKind("ticket").
		SetFromProductKey("ticket:FLINK-1").
		SetReferenceKind(unresolvedreference.ReferenceKindPrNumber).
		SetRawRef("repo/example#404").
		SetNormalizedRef("pull-request:repo/example#404").
		SetResolutionState(unresolvedreference.ResolutionStatePermissionBlocked).
		SetResolver("fixture").
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#404").
		SetSourceURL("https://github.com/repo/example/pull/404").
		SetLatestEvidence(evidenceRow).
		SaveX(ctx)
}

func seedTicketDocumentMessageGraph(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 11, 0, 0, 0, time.UTC)
	ticketRow := client.Ticket.Create().
		SetKey("ticket:DOC-1").
		SetTitle("Documented ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("apache-flink").
		SetExternalKind("jira_issue").
		SetExternalID("DOC-1").
		SetSourceURL("https://issues.example.test/DOC-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	documentRow := client.Document.Create().
		SetKey("document:flink-design").
		SetTitle("Flink design note").
		SetDocumentKind(document.DocumentKindMarkdown).
		SetSourceSystem("docs").
		SetSourceInstance("apache-flink").
		SetExternalKind("markdown").
		SetExternalID("flink-design.md").
		SetSourceURL("https://docs.example.test/flink-design").
		SetFreshnessState(document.FreshnessStateFresh).
		SetVisibility(document.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	messageRow := client.Message.Create().
		SetKey("message:standup-1").
		SetBody("DOC-1 needs the design note before merge.").
		SetSummary("Standup note for DOC-1").
		SetChannelKey("flink-dev").
		SetThreadKey("standup-thread").
		SetSourceSystem("slack").
		SetSourceInstance("apache-flink").
		SetExternalKind("slack_message").
		SetExternalID("standup-1").
		SetSourceURL("https://chat.example.test/archives/flink-dev/p1").
		SetFreshnessState(message.FreshnessStateFresh).
		SetVisibility(message.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	documentEvidence := client.Evidence.Create().
		SetKey("evidence:ticket-document").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_document").
		SetRelationshipKind(string(ontology.AssocDocuments)).
		SetLocatorKind("jira_link").
		SetLocator("DOC-1 linked documentation").
		SetSourceSystem("jira").
		SetSourceInstance("apache-flink").
		SetExternalKind("jira_issue_link").
		SetExternalID("DOC-1->flink-design.md").
		SetSourceURL("https://issues.example.test/DOC-1").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	messageEvidence := client.Evidence.Create().
		SetKey("evidence:ticket-message").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_message").
		SetRelationshipKind("discussed_in").
		SetLocatorKind("slack_message").
		SetLocator("flink-dev/standup-1").
		SetSourceSystem("slack").
		SetSourceInstance("apache-flink").
		SetExternalKind("slack_message").
		SetExternalID("standup-1").
		SetSourceURL("https://chat.example.test/archives/flink-dev/p1").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	client.TicketDocument.Create().
		SetTicket(ticketRow).
		SetDocument(documentRow).
		SetTicketDocumentKind(ticketdocument.TicketDocumentKindDocumentedBy).
		SetSourceSystem("jira").
		SetSourceInstance("apache-flink").
		SetExternalKind("jira_issue_link").
		SetExternalID("DOC-1->flink-design.md").
		SetSourceURL("https://issues.example.test/DOC-1").
		SetFreshnessState(ticketdocument.FreshnessStateFresh).
		SetVisibility(ticketdocument.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(8).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(documentEvidence).
		SaveX(ctx)
	client.TicketMessage.Create().
		SetTicket(ticketRow).
		SetMessage(messageRow).
		SetTicketMessageKind(ticketmessage.TicketMessageKindDiscussedIn).
		SetSourceSystem("slack").
		SetSourceInstance("apache-flink").
		SetExternalKind("slack_message").
		SetExternalID("DOC-1->standup-1").
		SetSourceURL("https://chat.example.test/archives/flink-dev/p1").
		SetFreshnessState(ticketmessage.FreshnessStateFresh).
		SetVisibility(ticketmessage.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(6).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(messageEvidence).
		SaveX(ctx)
}

func seedTicketDocumentSharedKeyGraph(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	ticketRow := client.Ticket.Create().
		SetKey("shared-key").
		SetTitle("Ticket with shared key").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue").
		SetExternalID("SHARED-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	documentRow := client.Document.Create().
		SetKey("shared-key").
		SetTitle("Document with shared key").
		SetDocumentKind(document.DocumentKindMarkdown).
		SetSourceSystem("docs").
		SetSourceInstance("example").
		SetExternalKind("markdown").
		SetExternalID("shared-key.md").
		SetFreshnessState(document.FreshnessStateFresh).
		SetVisibility(document.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	evidenceRow := client.Evidence.Create().
		SetKey("evidence:shared-key-ticket-document").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_document").
		SetRelationshipKind(string(ontology.AssocDocuments)).
		SetLocatorKind("fixture").
		SetLocator("shared-key relationship").
		SetSourceSystem("fixture").
		SetSourceInstance("example").
		SetExternalKind("fixture_relation").
		SetExternalID("shared-key->shared-key").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	client.TicketDocument.Create().
		SetTicket(ticketRow).
		SetDocument(documentRow).
		SetTicketDocumentKind(ticketdocument.TicketDocumentKindDocumentedBy).
		SetSourceSystem("fixture").
		SetSourceInstance("example").
		SetExternalKind("fixture_relation").
		SetExternalID("shared-key->shared-key").
		SetFreshnessState(ticketdocument.FreshnessStateFresh).
		SetVisibility(ticketdocument.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastActivityAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(evidenceRow).
		SaveX(ctx)
}

func seedParticipationAndDocumentLinkGraph(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 13, 0, 0, 0, time.UTC)
	alice := client.Person.Create().
		SetKey("person:alice").
		SetDisplayName("Alice Example").
		SetGithubLogin("alice").
		SetJiraAccountID("alice-jira").
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic).
		SetConfidence(1).
		SaveX(ctx)
	bob := client.Person.Create().
		SetKey("person:bob").
		SetDisplayName("Bob Reviewer").
		SetGithubLogin("bob").
		SetFreshnessState(person.FreshnessStateFresh).
		SetVisibility(person.VisibilityPublic).
		SetConfidence(1).
		SaveX(ctx)
	ticketRow := client.Ticket.Create().
		SetKey("ticket:OWNER-1").
		SetTitle("Owned ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue").
		SetExternalID("OWNER-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	prRow := client.PullRequest.Create().
		SetKey("pull-request:repo/example#7").
		SetRepository("repo/example").
		SetNumber(7).
		SetTitle("Participation PR").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#7").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	apiPlan := client.Document.Create().
		SetKey("document:api-plan").
		SetTitle("API rollout plan").
		SetDocumentKind(document.DocumentKindMarkdown).
		SetSourceSystem("docs").
		SetSourceInstance("example").
		SetExternalKind("markdown").
		SetExternalID("api-plan.md").
		SetFreshnessState(document.FreshnessStateFresh).
		SetVisibility(document.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	apiReference := client.Document.Create().
		SetKey("document:api-reference").
		SetTitle("API reference").
		SetDocumentKind(document.DocumentKindMarkdown).
		SetSourceSystem("docs").
		SetSourceInstance("example").
		SetExternalKind("markdown").
		SetExternalID("api-reference.md").
		SetFreshnessState(document.FreshnessStateFresh).
		SetVisibility(document.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)

	assignmentEvidence := createRelationshipEvidence(t, ctx, client, "evidence:ticket-assignee", "ticket_assignment", "assignee", "OWNER-1->alice", observedAt)
	authorshipEvidence := createRelationshipEvidence(t, ctx, client, "evidence:pr-author", "pull_request_authorship", "author", "repo/example#7->alice", observedAt)
	reviewEvidence := createRelationshipEvidence(t, ctx, client, "evidence:pr-approver", "pull_request_review", "approver", "repo/example#7->bob", observedAt)
	documentLinkEvidence := createRelationshipEvidence(t, ctx, client, "evidence:doc-link", "document_link", "links_to", "api-plan.md->api-reference.md", observedAt)

	client.TicketAssignment.Create().
		SetPerson(alice).
		SetTicket(ticketRow).
		SetAssignmentKind(ticketassignment.AssignmentKindAssignee).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_assignee").
		SetExternalID("OWNER-1->alice").
		SetFreshnessState(ticketassignment.FreshnessStateFresh).
		SetVisibility(ticketassignment.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastActivityAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(assignmentEvidence).
		SaveX(ctx)
	client.PullRequestAuthorship.Create().
		SetPerson(alice).
		SetPullRequest(prRow).
		SetAuthorshipKind(pullrequestauthorship.AuthorshipKindAuthor).
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request_author").
		SetExternalID("repo/example#7->alice").
		SetFreshnessState(pullrequestauthorship.FreshnessStateFresh).
		SetVisibility(pullrequestauthorship.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(9).
		SetLastActivityAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(authorshipEvidence).
		SaveX(ctx)
	client.PullRequestReview.Create().
		SetPerson(bob).
		SetPullRequest(prRow).
		SetReviewKind(pullrequestreview.ReviewKindApprover).
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request_review").
		SetExternalID("repo/example#7->bob:approver").
		SetFreshnessState(pullrequestreview.FreshnessStateFresh).
		SetVisibility(pullrequestreview.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(8).
		SetLastActivityAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(reviewEvidence).
		SaveX(ctx)
	client.DocumentLink.Create().
		SetFromDocument(apiPlan).
		SetToDocument(apiReference).
		SetDocumentLinkKind(documentlink.DocumentLinkKindLinksTo).
		SetSourceSystem("docs").
		SetSourceInstance("example").
		SetExternalKind("markdown_link").
		SetExternalID("api-plan.md->api-reference.md").
		SetFreshnessState(documentlink.FreshnessStateFresh).
		SetVisibility(documentlink.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(7).
		SetLastActivityAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(documentLinkEvidence).
		SaveX(ctx)
}

func seedTicketPrivatePublicPullRequestGraph(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 13, 30, 0, 0, time.UTC)
	ticketRow := client.Ticket.Create().
		SetKey("ticket:AUTH-1").
		SetTitle("Auth fixture ticket").
		SetStatus(ticket.StatusOpen).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_issue").
		SetExternalID("AUTH-1").
		SetFreshnessState(ticket.FreshnessStateFresh).
		SetVisibility(ticket.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	privatePR := client.PullRequest.Create().
		SetKey("pull-request:repo/example#100").
		SetRepository("repo/example").
		SetNumber(100).
		SetTitle("Private high-ranked PR").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#100").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPrivate).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	publicPR := client.PullRequest.Create().
		SetKey("pull-request:repo/example#101").
		SetRepository("repo/example").
		SetNumber(101).
		SetTitle("Public lower-ranked PR").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#101").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	privateEvidence := createRelationshipEvidence(t, ctx, client, "evidence:auth-private-pr", "ticket_pull_request", "implemented_by", "AUTH-1->repo/example#100", observedAt)
	publicEvidence := createRelationshipEvidence(t, ctx, client, "evidence:auth-public-pr", "ticket_pull_request", "implemented_by", "AUTH-1->repo/example#101", observedAt)
	client.TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(privatePR).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("AUTH-1->repo/example#100").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPrivate).
		SetConfidence(1).
		SetRankScore(100).
		SetLastActivityAt(observedAt.Add(time.Hour)).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(privateEvidence).
		SaveX(ctx)
	client.TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(publicPR).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance("example").
		SetExternalKind("jira_remote_link").
		SetExternalID("AUTH-1->repo/example#101").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(10).
		SetLastActivityAt(observedAt).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(publicEvidence).
		SaveX(ctx)
}

func createRelationshipEvidence(t *testing.T, ctx context.Context, client *genent.Client, key string, targetKind string, relationshipKind string, externalID string, observedAt time.Time) *genent.Evidence {
	t.Helper()
	return client.Evidence.Create().
		SetKey(key).
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind(targetKind).
		SetRelationshipKind(relationshipKind).
		SetLocatorKind("fixture").
		SetLocator(externalID).
		SetSourceSystem("fixture").
		SetSourceInstance("example").
		SetExternalKind("fixture_relation").
		SetExternalID(externalID).
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
}

func seedLowRankPullRequestForDocTicket(t *testing.T, ctx context.Context, client *genent.Client) {
	t.Helper()
	observedAt := time.Date(2026, 6, 24, 12, 30, 0, 0, time.UTC)
	ticketRow := client.Ticket.Query().Where(ticket.KeyEQ("ticket:DOC-1")).OnlyX(ctx)
	prRow := client.PullRequest.Create().
		SetKey("pull-request:repo/example#9").
		SetRepository("repo/example").
		SetNumber(9).
		SetTitle("Lower ranked PR").
		SetState(pullrequest.StateOpen).
		SetSourceSystem("github").
		SetSourceInstance("repo/example").
		SetExternalKind("github_pull_request").
		SetExternalID("repo/example#9").
		SetFreshnessState(pullrequest.FreshnessStateFresh).
		SetVisibility(pullrequest.VisibilityPublic).
		SetLastConfirmedAt(observedAt).
		SaveX(ctx)
	evidenceRow := client.Evidence.Create().
		SetKey("evidence:low-rank-ticket-pr").
		SetClaimKind(evidence.ClaimKindRelationship).
		SetClaimTargetKind("ticket_pull_request").
		SetRelationshipKind("implemented_by").
		SetLocatorKind("jira_remote_link").
		SetLocator("DOC-1 lower ranked remote link").
		SetSourceSystem("jira").
		SetSourceInstance("apache-flink").
		SetExternalKind("jira_remote_link").
		SetExternalID("DOC-1->repo/example#9").
		SetProofState(evidence.ProofStateCurrent).
		SetFreshnessState(evidence.FreshnessStateFresh).
		SetVisibility(evidence.VisibilityPublic).
		SetConfidence(1).
		SetObservedAt(observedAt).
		SaveX(ctx)
	client.TicketPullRequest.Create().
		SetTicket(ticketRow).
		SetPullRequest(prRow).
		SetTicketPullRequestKind(ticketpullrequest.TicketPullRequestKindImplementedBy).
		SetSourceSystem("jira").
		SetSourceInstance("apache-flink").
		SetExternalKind("jira_remote_link").
		SetExternalID("DOC-1->repo/example#9").
		SetFreshnessState(ticketpullrequest.FreshnessStateFresh).
		SetVisibility(ticketpullrequest.VisibilityPublic).
		SetConfidence(1).
		SetRankScore(1).
		SetLastActivityAt(observedAt.Add(2 * time.Hour)).
		SetLastConfirmedAt(observedAt).
		SetLatestEvidence(evidenceRow).
		SaveX(ctx)
}

func hasObject(objects []domain.Object, objectType domain.ObjectType, key string) bool {
	for _, object := range objects {
		if object.ObjectType == objectType && object.Key == key {
			return true
		}
	}
	return false
}

func hasCanonicalAssociation(
	associations []domain.Association,
	fromType domain.ObjectType,
	fromKey string,
	associationType domain.AssociationType,
	toType domain.ObjectType,
	toKey string,
) bool {
	for _, association := range associations {
		if association.From.ObjectType == fromType &&
			association.From.Key == fromKey &&
			association.AssociationType == associationType &&
			association.To.ObjectType == toType &&
			association.To.Key == toKey {
			return true
		}
	}
	return false
}
