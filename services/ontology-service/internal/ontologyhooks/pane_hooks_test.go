package ontologyhooks

import (
	"context"
	"testing"

	genent "cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/enttest"
	"cubicle/services/ontology-service/ent/panedocumentlink"
	"cubicle/services/ontology-service/ent/workpane"
	"cubicle/services/ontology-service/ent/worksurface"

	_ "github.com/mattn/go-sqlite3"
)

func TestRegisterRejectsPaneOnWrongSurface(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-surface")
	defer client.Close()

	documentsSurface := createHookTestSurface(t, ctx, client, "surface:person:hooks:documents", worksurface.SurfaceKindDocuments)
	if _, err := client.WorkPane.Create().
		SetKey("pane:person:hooks:bad-surface").
		SetWorkSurfaceID(documentsSurface.ID).
		SetPaneKind(workpane.PaneKindPullRequestsReviewed).
		SetTargetKind(workpane.TargetKindPullRequest).
		SetDisplayName("Bad Surface").
		Save(ctx); err == nil {
		t.Fatal("expected pull request pane under documents surface to fail")
	}
}

func TestRegisterRejectsPaneDocumentLinkWithWrongRelation(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-document-relation")
	defer client.Close()

	documentsSurface := createHookTestSurface(t, ctx, client, "surface:person:hooks:docs-relation", worksurface.SurfaceKindDocuments)
	createdPane := createHookTestPane(t, ctx, client, documentsSurface.ID, workpane.PaneKindDocumentsCreated, workpane.TargetKindDocument)
	document := createHookTestDocument(t, ctx, client, "document:hooks:relation")

	if _, err := client.PaneDocumentLink.Create().
		SetPaneID(createdPane.ID).
		SetDocumentID(document.ID).
		SetRelationKind(panedocumentlink.RelationKindEdited).
		Save(ctx); err == nil {
		t.Fatal("expected documents_created pane to reject edited relation")
	}

	if _, err := client.PaneDocumentLink.Create().
		SetPaneID(createdPane.ID).
		SetDocumentID(document.ID).
		SetRelationKind(panedocumentlink.RelationKindCreated).
		Save(ctx); err != nil {
		t.Fatalf("expected documents_created pane to accept created relation: %v", err)
	}
}

func TestRegisterRejectsPaneDocumentLinkForTicketPane(t *testing.T) {
	ctx := context.Background()
	client := openHookedClient(t, "ontology-hooks-document-target")
	defer client.Close()

	ticketsSurface := createHookTestSurface(t, ctx, client, "surface:person:hooks:tickets", worksurface.SurfaceKindTickets)
	ticketPane := createHookTestPane(t, ctx, client, ticketsSurface.ID, workpane.PaneKindTicketsOwned, workpane.TargetKindTicket)
	document := createHookTestDocument(t, ctx, client, "document:hooks:target")

	if _, err := client.PaneDocumentLink.Create().
		SetPaneID(ticketPane.ID).
		SetDocumentID(document.ID).
		SetRelationKind(panedocumentlink.RelationKindCreated).
		Save(ctx); err == nil {
		t.Fatal("expected document link table to reject ticket-target pane")
	}
}

func openHookedClient(t *testing.T, databaseName string) *genent.Client {
	t.Helper()
	client := enttest.Open(t, "sqlite3", "file:"+databaseName+"?mode=memory&cache=shared&_fk=1")
	Register(client)
	return client
}

func createHookTestSurface(t *testing.T, ctx context.Context, client *genent.Client, key string, surfaceKind worksurface.SurfaceKind) *genent.WorkSurface {
	t.Helper()
	person := client.Person.Create().
		SetKey(key + ":person").
		SetDisplayName("Hook Test Person").
		SaveX(ctx)
	return client.WorkSurface.Create().
		SetKey(key).
		SetPersonID(person.ID).
		SetSurfaceKind(surfaceKind).
		SetDisplayName(surfaceKind.String()).
		SaveX(ctx)
}

func createHookTestPane(t *testing.T, ctx context.Context, client *genent.Client, surfaceID int, paneKind workpane.PaneKind, targetKind workpane.TargetKind) *genent.WorkPane {
	t.Helper()
	return client.WorkPane.Create().
		SetKey("pane:" + paneKind.String()).
		SetWorkSurfaceID(surfaceID).
		SetPaneKind(paneKind).
		SetTargetKind(targetKind).
		SetDisplayName(paneKind.String()).
		SaveX(ctx)
}

func createHookTestDocument(t *testing.T, ctx context.Context, client *genent.Client, key string) *genent.Document {
	t.Helper()
	return client.Document.Create().
		SetKey(key).
		SetTitle("Hook Test Document").
		SaveX(ctx)
}
