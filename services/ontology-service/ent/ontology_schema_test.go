package ent_test

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/enttest"
	"cubicle/services/ontology-service/ent/migrate"
	"cubicle/services/ontology-service/ent/workarea"
	"cubicle/services/ontology-service/ent/worklens"

	entsqlschema "entgo.io/ent/dialect/sql/schema"
	_ "github.com/mattn/go-sqlite3"
)

// TestWorkLensDeclaresCardinalityBoundary documents the bounded lens table.
func TestWorkLensDeclaresCardinalityBoundary(t *testing.T) {
	table := findTable(t, "work_lenses")
	assertColumn(t, table, "work_area_id")
	assertColumn(t, table, "work_lens_kind")
	assertColumn(t, table, "lens_target_kind")
	assertColumn(t, table, "result_count")
	assertColumn(t, table, "last_indexed_at")
}

// TestDocumentLensResultCarriesPagingIndex proves the result layer can be
// ranked and paged before loading high-cardinality document targets.
func TestDocumentLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "document_lens_results")
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestWorkLensRejectsMismatchedTargetKind proves lens kind is semantic truth.
func TestWorkLensRejectsMismatchedTargetKind(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, "sqlite3", "file:ontology-lens-validation?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	area := createTestArea(t, ctx, client)
	if _, err := client.WorkLens.Create().
		SetKey("lens:person:docs:bad-target").
		SetWorkAreaID(area.ID).
		SetWorkLensKind(worklens.WorkLensKindDocumentsCommentedOn).
		SetLensTargetKind(worklens.LensTargetKindTicket).
		SetDisplayName("Bad target").
		Save(ctx); err == nil {
		t.Fatal("expected mismatched lens target kind to fail")
	}

	if _, err := client.WorkLens.Create().
		SetKey("lens:person:docs:commented-on").
		SetWorkAreaID(area.ID).
		SetWorkLensKind(worklens.WorkLensKindDocumentsCommentedOn).
		SetLensTargetKind(worklens.LensTargetKindDocument).
		SetDisplayName("Documents Commented On").
		Save(ctx); err != nil {
		t.Fatalf("expected valid lens target kind to save: %v", err)
	}
}

// createTestArea creates a document WorkArea for schema tests.
func createTestArea(t *testing.T, ctx context.Context, client *ent.Client) *ent.WorkArea {
	t.Helper()
	person := client.Person.Create().
		SetKey("person:test").
		SetDisplayName("Test Person").
		SaveX(ctx)
	return client.WorkArea.Create().
		SetKey("area:person:test:documents").
		SetPersonID(person.ID).
		SetWorkAreaKind(workarea.WorkAreaKindDocuments).
		SetDisplayName("Documents").
		SaveX(ctx)
}

// findTable returns the generated migration table with the requested name.
func findTable(t *testing.T, name string) *entsqlschema.Table {
	t.Helper()
	for _, table := range migrate.Tables {
		if table.Name == name {
			return table
		}
	}
	t.Fatalf("missing table %q", name)
	return nil
}

// assertColumn fails the test if table does not contain name.
func assertColumn(t *testing.T, table *entsqlschema.Table, name string) {
	t.Helper()
	for _, column := range table.Columns {
		if column.Name == name {
			return
		}
	}
	t.Fatalf("table %q missing column %q", table.Name, name)
}

// assertIndexColumns fails the test if table does not contain a matching index.
func assertIndexColumns(t *testing.T, table *entsqlschema.Table, names []string) {
	t.Helper()
	for _, idx := range table.Indexes {
		if len(idx.Columns) != len(names) {
			continue
		}
		matches := true
		for i, column := range idx.Columns {
			if column.Name != names[i] {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}
	t.Fatalf("table %q missing index over columns %#v", table.Name, names)
}
