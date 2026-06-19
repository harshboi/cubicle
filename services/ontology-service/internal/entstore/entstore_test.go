package entstore

import (
	"context"
	"path/filepath"
	"testing"

	"cubicle/services/ontology-service/ent/workarea"
	"cubicle/services/ontology-service/ent/worklens"
)

// TestOpenMigratesSchemaAndRegistersHooks proves runtime startup initializes Ent.
func TestOpenMigratesSchemaAndRegistersHooks(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, Config{
		DatabasePath: filepath.Join(t.TempDir(), "ontology.db"),
	})
	if err != nil {
		t.Fatalf("open ent store: %v", err)
	}
	defer store.Close()

	client := store.Client()
	if client == nil {
		t.Fatal("Client() returned nil")
	}

	person := client.Person.Create().
		SetKey("person:runtime").
		SetDisplayName("Runtime Person").
		SaveX(ctx)
	documentsArea := client.WorkArea.Create().
		SetKey("area:runtime:documents").
		SetPersonID(person.ID).
		SetWorkAreaKind(workarea.WorkAreaKindDocuments).
		SetDisplayName("Documents").
		SaveX(ctx)

	if _, err := client.WorkLens.Create().
		SetKey("lens:runtime:bad-area").
		SetWorkAreaID(documentsArea.ID).
		SetWorkLensKind(worklens.WorkLensKindPullRequestsAuthored).
		SetLensTargetKind(worklens.LensTargetKindPullRequest).
		SetDisplayName("Bad Area").
		Save(ctx); err == nil {
		t.Fatal("expected registered ontology hook to reject a code lens under documents")
	}
}
