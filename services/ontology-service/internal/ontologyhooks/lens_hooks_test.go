package ontologyhooks

import (
	"context"
	"testing"

	genent "cubicle/services/ontology-service/ent"
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
