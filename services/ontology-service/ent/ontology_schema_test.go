package ent_test

import (
	"context"
	"testing"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/ent/enttest"
	"cubicle/services/ontology-service/ent/migrate"
	ontologyschema "cubicle/services/ontology-service/ent/schema"
	"cubicle/services/ontology-service/ent/workarea"
	"cubicle/services/ontology-service/ent/worklens"

	coreent "entgo.io/ent"
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

// TestWorkLensWindowDeclaresTraversalBoundary documents the partition between
// a broad lens and high-volume result rows.
func TestWorkLensWindowDeclaresTraversalBoundary(t *testing.T) {
	table := findTable(t, "work_lens_windows")
	assertColumn(t, table, "work_lens_id")
	assertColumn(t, table, "lens_window_kind")
	assertColumn(t, table, "window_start_at")
	assertColumn(t, table, "window_end_at")
	assertColumn(t, table, "rank_start")
	assertColumn(t, table, "rank_end")
	assertColumn(t, table, "checkpoint")
	assertIndexColumns(t, table, []string{"work_lens_id", "lens_window_kind", "last_activity_at"})
}

// TestDocumentLensResultCarriesPagingIndex proves the result layer can be
// ranked and paged by window before loading high-cardinality document targets.
func TestDocumentLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "document_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestPullRequestLensResultCarriesPagingIndex proves the result layer can be
// ranked and paged by window before loading high-cardinality pull-request targets.
func TestPullRequestLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "pull_request_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestPersonServingGraphAvoidsDirectHighCardinalityEdges proves person pages
// must cross bounded serving parents before loading work items.
func TestPersonServingGraphAvoidsDirectHighCardinalityEdges(t *testing.T) {
	assertSchemaEdges(t, ontologyschema.Person{}.Edges(), []string{"work_areas", "identities"})
}

// TestWorkLensServingGraphHasNoTargetFanout proves a WorkLens cannot directly
// expose target work rows.
func TestWorkLensServingGraphHasNoTargetFanout(t *testing.T) {
	assertSchemaEdges(t, ontologyschema.WorkLens{}.Edges(), []string{"area", "windows"})
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

// assertIndexColumns fails if table does not contain an index over names.
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

// assertSchemaEdges fails if a handwritten schema exposes unexpected edge names.
func assertSchemaEdges(t *testing.T, edges []coreent.Edge, names []string) {
	t.Helper()
	if len(edges) != len(names) {
		t.Fatalf("schema edge count = %d, want %#v", len(edges), names)
	}
	for i, edge := range edges {
		descriptor := edge.Descriptor()
		if descriptor.Name != names[i] {
			t.Fatalf("schema edge %d = %q, want %q", i, descriptor.Name, names[i])
		}
		if descriptor.Through != nil {
			t.Fatalf("schema edge %q must not be a direct Through edge", descriptor.Name)
		}
	}
}
