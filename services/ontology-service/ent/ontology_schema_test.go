package ent_test

import (
	"testing"

	"cubicle/services/ontology-service/ent/enttest"
	"cubicle/services/ontology-service/ent/migrate"

	entsqlschema "entgo.io/ent/dialect/sql/schema"
	_ "github.com/mattn/go-sqlite3"
)

// TestOntologySchemaMigratesCardinalitySafeTables proves the schema migrates
// against SQLite and declares the intentionally bounded 18-table ontology.
func TestOntologySchemaMigratesCardinalitySafeTables(t *testing.T) {
	client := enttest.Open(t, "sqlite3", "file:ontology-schema?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	want := map[string]bool{
		"persons":                   true,
		"workstreams":               true,
		"tickets":                   true,
		"pull_requests":             true,
		"documents":                 true,
		"document_fragments":        true,
		"messages":                  true,
		"evidences":                 true,
		"work_surfaces":             true,
		"work_panes":                true,
		"workstream_tickets":        true,
		"ticket_pull_requests":      true,
		"ticket_document_fragments": true,
		"ticket_messages":           true,
		"pane_document_links":       true,
		"pane_pull_request_links":   true,
		"pane_ticket_links":         true,
		"pane_message_links":        true,
	}

	got := make(map[string]bool, len(migrate.Tables))
	for _, table := range migrate.Tables {
		got[table.Name] = true
	}

	if len(got) != len(want) {
		t.Fatalf("expected %d ontology tables, got %d: %#v", len(want), len(got), got)
	}
	for table := range want {
		if !got[table] {
			t.Fatalf("missing ontology table %q; got %#v", table, got)
		}
	}
}

// TestPaneDocumentLinkCarriesPagingIndex proves the high-cardinality layer has
// the columns needed for ranked, fresh, paged reads before loading documents.
func TestPaneDocumentLinkCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "pane_document_links")
	assertIndexColumns(t, table, []string{"pane_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestWorkPaneDeclaresConcreteTargetEdges documents the Ent-native version of
// the pattern: one generic pane schema with concrete target-specific link tables.
func TestWorkPaneDeclaresConcreteTargetEdges(t *testing.T) {
	table := findTable(t, "work_panes")
	assertColumn(t, table, "pane_kind")
	assertColumn(t, table, "target_kind")
	assertColumn(t, table, "target_count")
	assertColumn(t, table, "last_indexed_at")
}

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

func assertColumn(t *testing.T, table *entsqlschema.Table, name string) {
	t.Helper()
	for _, column := range table.Columns {
		if column.Name == name {
			return
		}
	}
	t.Fatalf("table %q missing column %q", table.Name, name)
}

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
