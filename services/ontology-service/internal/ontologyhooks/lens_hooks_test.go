package ontologyhooks

import (
	"context"
	"testing"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/documentlensresult"
	"cubicle/services/ontology-service/ent/enttest"
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

// TestRegisterRejectsDocumentResultWithWrongWindow proves result rows cannot cross windows.
func TestRegisterRejectsDocumentResultWithWrongWindow(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-document-result-window")
	defer client.Close()

	documentsArea := createHookTestArea(t, ctx, client, "area:person:hooks:docs-window", workarea.WorkAreaKindDocuments)
	createdLens := createHookTestLens(t, ctx, client, documentsArea.ID, worklens.WorkLensKindDocumentsCreated, worklens.LensTargetKindDocument)
	editedLens := createHookTestLens(t, ctx, client, documentsArea.ID, worklens.WorkLensKindDocumentsEdited, worklens.LensTargetKindDocument)
	editedWindow := createHookTestWindow(t, ctx, client, editedLens.ID, "documents-edited")
	document := createHookTestDocument(t, ctx, client, "document:hooks:wrong-window")

	if _, err := client.DocumentLensResult.Create().
		SetWorkLensID(createdLens.ID).
		SetWorkLensWindowID(editedWindow.ID).
		SetDocumentID(document.ID).
		SetRelationKind(documentlensresult.RelationKindCreated).
		Save(ctx); err == nil {
		t.Fatal("expected document result to reject a window owned by another lens")
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
