package ontologyhooks

import (
	"context"
	"testing"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/documentlensresult"
	"cubicle/services/ontology-service/ent/enttest"
	"cubicle/services/ontology-service/ent/messagelensresult"
	"cubicle/services/ontology-service/ent/pullrequestlensresult"
	"cubicle/services/ontology-service/ent/ticketlensresult"
	"cubicle/services/ontology-service/ent/workarea"
	"cubicle/services/ontology-service/ent/worklens"

	_ "github.com/mattn/go-sqlite3"
)

// TestRegisterRejectsLensOnWrongArea proves WorkLens kind controls placement.
func TestRegisterRejectsLensOnWrongArea(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-lens-area")
	defer client.Close()

	documentsArea := createHookTestArea(t, ctx, client, "area:person:hooks:documents", workarea.WorkAreaKindDocuments)
	if _, err := client.WorkLens.Create().
		SetKey("lens:person:hooks:bad-area").
		SetWorkAreaID(documentsArea.ID).
		SetWorkLensKind(worklens.WorkLensKindPullRequestsReviewed).
		SetLensTargetKind(worklens.LensTargetKindPullRequest).
		SetDisplayName("Bad Area").
		Save(ctx); err == nil {
		t.Fatal("expected pull request lens under documents area to fail")
	}
}

// TestRegisterRejectsMessageResultWithWrongRelation proves relation validation.
func TestRegisterRejectsMessageResultWithWrongRelation(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-message-result-relation")
	defer client.Close()

	communicationsArea := createHookTestArea(t, ctx, client, "area:person:hooks:messages-relation", workarea.WorkAreaKindCommunications)
	authoredLens := createHookTestLens(t, ctx, client, communicationsArea.ID, worklens.WorkLensKindMessagesAuthored, worklens.LensTargetKindMessage)
	authoredWindow := createHookTestWindow(t, ctx, client, authoredLens.ID, "messages-authored")
	message := createHookTestMessage(t, ctx, client, "message:hooks:relation")

	if _, err := client.MessageLensResult.Create().
		SetWorkLensID(authoredLens.ID).
		SetWorkLensWindowID(authoredWindow.ID).
		SetMessageID(message.ID).
		SetRelationKind(messagelensresult.RelationKindRepliedTo).
		Save(ctx); err == nil {
		t.Fatal("expected messages_authored lens to reject replied_to relation")
	}

	if _, err := client.MessageLensResult.Create().
		SetWorkLensID(authoredLens.ID).
		SetWorkLensWindowID(authoredWindow.ID).
		SetMessageID(message.ID).
		SetRelationKind(messagelensresult.RelationKindAuthored).
		Save(ctx); err != nil {
		t.Fatalf("expected messages_authored lens to accept authored relation: %v", err)
	}
}

// TestRegisterRejectsMessageResultForDocumentLens proves target validation.
func TestRegisterRejectsMessageResultForDocumentLens(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-message-result-target")
	defer client.Close()

	documentsArea := createHookTestArea(t, ctx, client, "area:person:hooks:docs-message-target", workarea.WorkAreaKindDocuments)
	documentLens := createHookTestLens(t, ctx, client, documentsArea.ID, worklens.WorkLensKindDocumentsCreated, worklens.LensTargetKindDocument)
	documentWindow := createHookTestWindow(t, ctx, client, documentLens.ID, "docs-message-target")
	message := createHookTestMessage(t, ctx, client, "message:hooks:target")

	if _, err := client.MessageLensResult.Create().
		SetWorkLensID(documentLens.ID).
		SetWorkLensWindowID(documentWindow.ID).
		SetMessageID(message.ID).
		SetRelationKind(messagelensresult.RelationKindAuthored).
		Save(ctx); err == nil {
		t.Fatal("expected message result table to reject document-target lens")
	}
}

// TestRegisterRejectsResultWithWrongWindow proves result rows cannot cross windows.
func TestRegisterRejectsResultWithWrongWindow(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-result-window")
	defer client.Close()

	communicationsArea := createHookTestArea(t, ctx, client, "area:person:hooks:messages-window", workarea.WorkAreaKindCommunications)
	messageLens := createHookTestLens(t, ctx, client, communicationsArea.ID, worklens.WorkLensKindMessagesAuthored, worklens.LensTargetKindMessage)
	documentsArea := createHookTestArea(t, ctx, client, "area:person:hooks:docs-window", workarea.WorkAreaKindDocuments)
	documentLens := createHookTestLens(t, ctx, client, documentsArea.ID, worklens.WorkLensKindDocumentsCreated, worklens.LensTargetKindDocument)
	documentWindow := createHookTestWindow(t, ctx, client, documentLens.ID, "docs-window")
	message := createHookTestMessage(t, ctx, client, "message:hooks:wrong-window")

	if _, err := client.MessageLensResult.Create().
		SetWorkLensID(messageLens.ID).
		SetWorkLensWindowID(documentWindow.ID).
		SetMessageID(message.ID).
		SetRelationKind(messagelensresult.RelationKindAuthored).
		Save(ctx); err == nil {
		t.Fatal("expected message result to reject a window owned by another lens")
	}
}

// TestRegisterRejectsTicketResultWithWrongRelation proves relation validation.
func TestRegisterRejectsTicketResultWithWrongRelation(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-ticket-result-relation")
	defer client.Close()

	ticketsArea := createHookTestArea(t, ctx, client, "area:person:hooks:tickets-relation", workarea.WorkAreaKindTickets)
	ownedLens := createHookTestLens(t, ctx, client, ticketsArea.ID, worklens.WorkLensKindTicketsOwned, worklens.LensTargetKindTicket)
	ownedWindow := createHookTestWindow(t, ctx, client, ownedLens.ID, "tickets-owned")
	ticket := createHookTestTicket(t, ctx, client, "ticket:hooks:relation")

	if _, err := client.TicketLensResult.Create().
		SetWorkLensID(ownedLens.ID).
		SetWorkLensWindowID(ownedWindow.ID).
		SetTicketID(ticket.ID).
		SetRelationKind(ticketlensresult.RelationKindReviewed).
		Save(ctx); err == nil {
		t.Fatal("expected tickets_owned lens to reject reviewed relation")
	}

	if _, err := client.TicketLensResult.Create().
		SetWorkLensID(ownedLens.ID).
		SetWorkLensWindowID(ownedWindow.ID).
		SetTicketID(ticket.ID).
		SetRelationKind(ticketlensresult.RelationKindOwned).
		Save(ctx); err != nil {
		t.Fatalf("expected tickets_owned lens to accept owned relation: %v", err)
	}
}

// TestRegisterRejectsTicketResultForDocumentLens proves target validation.
func TestRegisterRejectsTicketResultForDocumentLens(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-ticket-result-target")
	defer client.Close()

	documentsArea := createHookTestArea(t, ctx, client, "area:person:hooks:docs-ticket-target", workarea.WorkAreaKindDocuments)
	documentLens := createHookTestLens(t, ctx, client, documentsArea.ID, worklens.WorkLensKindDocumentsCreated, worklens.LensTargetKindDocument)
	documentWindow := createHookTestWindow(t, ctx, client, documentLens.ID, "docs-ticket-target")
	ticket := createHookTestTicket(t, ctx, client, "ticket:hooks:target")

	if _, err := client.TicketLensResult.Create().
		SetWorkLensID(documentLens.ID).
		SetWorkLensWindowID(documentWindow.ID).
		SetTicketID(ticket.ID).
		SetRelationKind(ticketlensresult.RelationKindOwned).
		Save(ctx); err == nil {
		t.Fatal("expected ticket result table to reject document-target lens")
	}
}

// TestRegisterRejectsPullRequestResultWithWrongRelation proves relation validation.
func TestRegisterRejectsPullRequestResultWithWrongRelation(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-pr-result-relation")
	defer client.Close()

	codeArea := createHookTestArea(t, ctx, client, "area:person:hooks:code-relation", workarea.WorkAreaKindCode)
	authoredLens := createHookTestLens(t, ctx, client, codeArea.ID, worklens.WorkLensKindPullRequestsAuthored, worklens.LensTargetKindPullRequest)
	authoredWindow := createHookTestWindow(t, ctx, client, authoredLens.ID, "pull-requests-authored")
	pullRequest := createHookTestPullRequest(t, ctx, client, "pull-request:hooks:relation")

	if _, err := client.PullRequestLensResult.Create().
		SetWorkLensID(authoredLens.ID).
		SetWorkLensWindowID(authoredWindow.ID).
		SetPullRequestID(pullRequest.ID).
		SetRelationKind(pullrequestlensresult.RelationKindReviewed).
		Save(ctx); err == nil {
		t.Fatal("expected pull_requests_authored lens to reject reviewed relation")
	}

	if _, err := client.PullRequestLensResult.Create().
		SetWorkLensID(authoredLens.ID).
		SetWorkLensWindowID(authoredWindow.ID).
		SetPullRequestID(pullRequest.ID).
		SetRelationKind(pullrequestlensresult.RelationKindAuthored).
		Save(ctx); err != nil {
		t.Fatalf("expected pull_requests_authored lens to accept authored relation: %v", err)
	}
}

// TestRegisterRejectsPullRequestResultForDocumentLens proves target validation.
func TestRegisterRejectsPullRequestResultForDocumentLens(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-pr-result-target")
	defer client.Close()

	documentsArea := createHookTestArea(t, ctx, client, "area:person:hooks:docs-target", workarea.WorkAreaKindDocuments)
	documentLens := createHookTestLens(t, ctx, client, documentsArea.ID, worklens.WorkLensKindDocumentsCreated, worklens.LensTargetKindDocument)
	documentWindow := createHookTestWindow(t, ctx, client, documentLens.ID, "docs-pr-target")
	pullRequest := createHookTestPullRequest(t, ctx, client, "pull-request:hooks:target")

	if _, err := client.PullRequestLensResult.Create().
		SetWorkLensID(documentLens.ID).
		SetWorkLensWindowID(documentWindow.ID).
		SetPullRequestID(pullRequest.ID).
		SetRelationKind(pullrequestlensresult.RelationKindAuthored).
		Save(ctx); err == nil {
		t.Fatal("expected pull-request result table to reject document-target lens")
	}
}

// TestRegisterRejectsDocumentResultWithWrongRelation proves relation validation.
func TestRegisterRejectsDocumentResultWithWrongRelation(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-document-result-relation")
	defer client.Close()

	documentsArea := createHookTestArea(t, ctx, client, "area:person:hooks:docs-relation", workarea.WorkAreaKindDocuments)
	createdLens := createHookTestLens(t, ctx, client, documentsArea.ID, worklens.WorkLensKindDocumentsCreated, worklens.LensTargetKindDocument)
	createdWindow := createHookTestWindow(t, ctx, client, createdLens.ID, "documents-created")
	document := createHookTestDocument(t, ctx, client, "document:hooks:relation")

	if _, err := client.DocumentLensResult.Create().
		SetWorkLensID(createdLens.ID).
		SetWorkLensWindowID(createdWindow.ID).
		SetDocumentID(document.ID).
		SetRelationKind(documentlensresult.RelationKindEdited).
		Save(ctx); err == nil {
		t.Fatal("expected documents_created lens to reject edited relation")
	}

	if _, err := client.DocumentLensResult.Create().
		SetWorkLensID(createdLens.ID).
		SetWorkLensWindowID(createdWindow.ID).
		SetDocumentID(document.ID).
		SetRelationKind(documentlensresult.RelationKindCreated).
		Save(ctx); err != nil {
		t.Fatalf("expected documents_created lens to accept created relation: %v", err)
	}
}

// TestRegisterRejectsDocumentResultForTicketLens proves target-table validation.
func TestRegisterRejectsDocumentResultForTicketLens(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-document-result-target")
	defer client.Close()

	ticketsArea := createHookTestArea(t, ctx, client, "area:person:hooks:tickets", workarea.WorkAreaKindTickets)
	ticketLens := createHookTestLens(t, ctx, client, ticketsArea.ID, worklens.WorkLensKindTicketsOwned, worklens.LensTargetKindTicket)
	ticketWindow := createHookTestWindow(t, ctx, client, ticketLens.ID, "tickets-document-target")
	document := createHookTestDocument(t, ctx, client, "document:hooks:target")

	if _, err := client.DocumentLensResult.Create().
		SetWorkLensID(ticketLens.ID).
		SetWorkLensWindowID(ticketWindow.ID).
		SetDocumentID(document.ID).
		SetRelationKind(documentlensresult.RelationKindCreated).
		Save(ctx); err == nil {
		t.Fatal("expected document result table to reject ticket-target lens")
	}
}

// openHookedClient opens an in-memory Ent client with ontology hooks installed.
func openHookedClient(t *testing.T, databaseName string) *genent.Client {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+databaseName+"?mode=memory&cache=shared&_fk=1")
	Register(client)
	return client
}

// createHookTestArea creates a person-owned WorkArea for placement tests.
func createHookTestArea(t *testing.T, ctx context.Context, client *genent.Client, key string, areaKind workarea.WorkAreaKind) *genent.WorkArea {
	t.Helper()
	person := client.Person.Create().
		SetKey(key + ":person").
		SetDisplayName("Hook Test Person").
		SaveX(ctx)
	return client.WorkArea.Create().
		SetKey(key).
		SetPersonID(person.ID).
		SetWorkAreaKind(areaKind).
		SetDisplayName(areaKind.String()).
		SaveX(ctx)
}

// createHookTestLens creates a WorkLens for result-hook tests.
func createHookTestLens(t *testing.T, ctx context.Context, client *genent.Client, areaID int, lensKind worklens.WorkLensKind, targetKind worklens.LensTargetKind) *genent.WorkLens {
	t.Helper()
	return client.WorkLens.Create().
		SetKey("lens:" + lensKind.String()).
		SetWorkAreaID(areaID).
		SetWorkLensKind(lensKind).
		SetLensTargetKind(targetKind).
		SetDisplayName(lensKind.String()).
		SaveX(ctx)
}

// createHookTestWindow creates a bounded WorkLensWindow for result-hook tests.
func createHookTestWindow(t *testing.T, ctx context.Context, client *genent.Client, lensID int, keySuffix string) *genent.WorkLensWindow {
	t.Helper()
	return client.WorkLensWindow.Create().
		SetKey("window:" + keySuffix).
		SetWorkLensID(lensID).
		SaveX(ctx)
}

// createHookTestDocument creates a Document target for result-hook tests.
func createHookTestDocument(t *testing.T, ctx context.Context, client *genent.Client, key string) *genent.Document {
	t.Helper()
	return client.Document.Create().
		SetKey(key).
		SetTitle("Hook Test Document").
		SaveX(ctx)
}

// createHookTestPullRequest creates a PullRequest target for result-hook tests.
func createHookTestPullRequest(t *testing.T, ctx context.Context, client *genent.Client, key string) *genent.PullRequest {
	t.Helper()
	return client.PullRequest.Create().
		SetKey(key).
		SetTitle("Hook Test Pull Request").
		SaveX(ctx)
}

// createHookTestTicket creates a Ticket target for result-hook tests.
func createHookTestTicket(t *testing.T, ctx context.Context, client *genent.Client, key string) *genent.Ticket {
	t.Helper()
	return client.Ticket.Create().
		SetKey(key).
		SetTitle("Hook Test Ticket").
		SaveX(ctx)
}

// createHookTestMessage creates a Message target for result-hook tests.
func createHookTestMessage(t *testing.T, ctx context.Context, client *genent.Client, key string) *genent.Message {
	t.Helper()
	return client.Message.Create().
		SetKey(key).
		SetBody("Hook test message").
		SaveX(ctx)
}
