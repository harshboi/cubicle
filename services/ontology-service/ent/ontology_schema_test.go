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
// ranked and paged before loading high-cardinality document targets.
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
// ranked and paged before loading high-cardinality pull-request targets.
func TestPullRequestLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "pull_request_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestTicketLensResultCarriesPagingIndex proves the result layer can be ranked
// and paged before loading high-cardinality ticket targets.
func TestTicketLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "ticket_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestMessageLensResultCarriesPagingIndex proves the result layer can be
// ranked and paged before loading high-cardinality message targets.
func TestMessageLensResultCarriesPagingIndex(t *testing.T) {
	table := findTable(t, "message_lens_results")
	assertColumn(t, table, "work_lens_window_id")
	assertIndexColumns(t, table, []string{"work_lens_window_id", "freshness_state", "rank_score", "last_activity_at"})
	assertIndexColumns(t, table, []string{"work_lens_id", "freshness_state", "rank_score", "last_activity_at"})
	assertColumn(t, table, "relation_kind")
	assertColumn(t, table, "latest_evidence_id")
	assertColumn(t, table, "evidence_count")
}

// TestPersonServingGraphAvoidsDirectHighCardinalityEdges proves person pages
// must cross WorkArea, WorkLens, and WorkLensWindow before loading work items.
func TestPersonServingGraphAvoidsDirectHighCardinalityEdges(t *testing.T) {
	assertSchemaEdges(t, ontologyschema.Person{}.Edges(), []string{"work_areas", "identities"})
}

// TestWorkLensServingGraphRequiresWindows proves target loading cannot skip
// WorkLensWindow, the serving cardinality boundary.
func TestWorkLensServingGraphRequiresWindows(t *testing.T) {
	assertSchemaEdges(t, ontologyschema.WorkLens{}.Edges(), []string{"area", "windows"})
}

// TestNorthstarGraphSchemaDeclaresTypedGraphAndIsolatedSyncSupport documents the
// selected product graph: source-backed product rows, typed relationship rows,
// locator-grade Evidence, and source-scope sync metadata that is not traversed
// as product graph adjacency.
func TestNorthstarGraphSchemaDeclaresTypedGraphAndIsolatedSyncSupport(t *testing.T) {
	for _, rejected := range []string{
		"source_runs",
		"source_items",
		"source_delta",
		"evidence_anchors",
		"source_references",
		"source_actor_facts",
		"external_identities",
		"document_fragments",
		"ticket_document_fragments",
	} {
		assertNoTable(t, rejected)
	}

	ticket := findTable(t, "tickets")
	assertColumn(t, ticket, "source_system")
	assertColumn(t, ticket, "source_instance")
	assertColumn(t, ticket, "external_kind")
	assertColumn(t, ticket, "external_id")
	assertColumn(t, ticket, "source_version")
	assertColumn(t, ticket, "content_hash")
	assertColumn(t, ticket, "deletion_state")
	assertColumn(t, ticket, "acl_policy_key")
	assertColumn(t, ticket, "visibility_hash")
	assertColumn(t, ticket, "acl_state")
	assertColumn(t, ticket, "source_scope_state_id")
	assertUniqueIndexColumns(t, ticket, []string{"source_system", "source_instance", "external_kind", "external_id"})

	for _, tableName := range []string{"tickets", "pull_requests", "documents", "messages", "workstreams"} {
		table := findTable(t, tableName)
		assertColumn(t, table, "latest_evidence_id")
		assertColumn(t, table, "evidence_count")
	}

	evidences := findTable(t, "evidences")
	assertColumn(t, evidences, "claim_kind")
	assertColumn(t, evidences, "claim_target_kind")
	assertColumn(t, evidences, "claim_target_id")
	assertColumn(t, evidences, "relationship_kind")
	assertColumn(t, evidences, "locator_kind")
	assertColumn(t, evidences, "locator")
	assertColumn(t, evidences, "source_span_key")
	assertColumn(t, evidences, "span_start")
	assertColumn(t, evidences, "span_end")
	assertColumn(t, evidences, "proof_state")
	assertColumn(t, evidences, "acl_policy_key_snapshot")
	assertColumn(t, evidences, "visibility_hash_snapshot")
	assertIndexColumns(t, evidences, []string{"claim_kind", "claim_target_kind", "claim_target_id"})
	assertIndexColumns(t, evidences, []string{"source_system", "source_instance", "external_kind", "external_id", "source_span_key"})

	personIdentities := findTable(t, "person_identities")
	assertColumn(t, personIdentities, "person_id")
	assertColumn(t, personIdentities, "source_system")
	assertColumn(t, personIdentities, "external_kind")
	assertColumn(t, personIdentities, "identity_status")
	assertUniqueIndexColumns(t, personIdentities, []string{"source_system", "source_instance", "external_kind", "external_id"})

	sourceConnections := findTable(t, "source_connections")
	assertUniqueIndexColumns(t, sourceConnections, []string{"source_system", "source_instance"})
	sourceScopes := findTable(t, "source_scopes")
	assertUniqueIndexColumns(t, sourceScopes, []string{"source_connection_id", "scope_kind", "scope_key"})
	sourceScopeStates := findTable(t, "source_scope_states")
	assertColumn(t, sourceScopeStates, "coverage_mode")
	assertColumn(t, sourceScopeStates, "last_successful_sync_run_id")
	assertUniqueIndexColumns(t, sourceScopeStates, []string{"source_scope_id"})
	sourceSyncRuns := findTable(t, "source_sync_runs")
	assertColumn(t, sourceSyncRuns, "source_scope_id")
	assertColumn(t, sourceSyncRuns, "sync_mode")
	assertColumn(t, sourceSyncRuns, "coverage_mode")
	assertColumn(t, sourceSyncRuns, "status")
	assertColumn(t, sourceSyncRuns, "objects_seen_count")
	assertColumn(t, sourceSyncRuns, "objects_created_count")
	assertColumn(t, sourceSyncRuns, "objects_updated_count")
	assertColumn(t, sourceSyncRuns, "objects_deleted_count")
	assertColumn(t, sourceSyncRuns, "relationships_created_count")
	assertColumn(t, sourceSyncRuns, "relationships_updated_count")
	assertColumn(t, sourceSyncRuns, "relationships_deleted_count")
	assertColumn(t, sourceSyncRuns, "evidence_created_count")
	assertColumn(t, sourceSyncRuns, "issues_created_count")
	assertUniqueIndexColumns(t, sourceSyncRuns, []string{"source_scope_id", "run_key"})

	typedRelationships := []struct {
		tableName       string
		firstEndpoint   string
		secondEndpoint  string
		kindColumn      string
		reverseEndpoint string
	}{
		{"ticket_assignments", "person_id", "ticket_id", "assignment_kind", "ticket_id"},
		{"document_authorships", "person_id", "document_id", "authorship_kind", "document_id"},
		{"message_authorships", "person_id", "message_id", "authorship_kind", "message_id"},
		{"pull_request_authorships", "person_id", "pull_request_id", "authorship_kind", "pull_request_id"},
		{"pull_request_reviews", "person_id", "pull_request_id", "review_kind", "pull_request_id"},
		{"message_mentions", "person_id", "message_id", "mention_kind", "message_id"},
		{"ticket_mentions", "person_id", "ticket_id", "mention_kind", "ticket_id"},
		{"ticket_pull_requests", "ticket_id", "pull_request_id", "ticket_pull_request_kind", "pull_request_id"},
		{"ticket_messages", "ticket_id", "message_id", "ticket_message_kind", "message_id"},
		{"ticket_documents", "ticket_id", "document_id", "ticket_document_kind", "document_id"},
		{"document_links", "from_document_id", "to_document_id", "document_link_kind", "to_document_id"},
		{"workstream_tickets", "workstream_id", "ticket_id", "workstream_ticket_kind", "ticket_id"},
	}
	for _, relationship := range typedRelationships {
		table := findTable(t, relationship.tableName)
		assertColumn(t, table, relationship.kindColumn)
		assertColumn(t, table, "latest_evidence_id")
		assertColumn(t, table, "evidence_count")
		assertColumn(t, table, "source_system")
		assertColumn(t, table, "source_scope_state_id")
		assertUniqueIndexColumns(t, table, []string{relationship.firstEndpoint, relationship.secondEndpoint, relationship.kindColumn})
		assertIndexColumns(t, table, []string{relationship.reverseEndpoint, "freshness_state", "rank_score", "last_activity_at"})
	}

	lensResults := []struct {
		tableName      string
		targetEndpoint string
	}{
		{"document_lens_results", "document_id"},
		{"pull_request_lens_results", "pull_request_id"},
		{"ticket_lens_results", "ticket_id"},
		{"message_lens_results", "message_id"},
	}
	for _, result := range lensResults {
		table := findTable(t, result.tableName)
		assertUniqueIndexColumns(t, table, []string{"work_lens_id", result.targetEndpoint, "relation_kind"})
		assertIndexColumns(t, table, []string{result.targetEndpoint, "freshness_state", "rank_score", "last_activity_at"})
	}
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

// assertNoTable fails if the generated migration schema still includes name.
func assertNoTable(t *testing.T, name string) {
	t.Helper()
	for _, table := range migrate.Tables {
		if table.Name == name {
			t.Fatalf("table %q should not be part of the northstar schema", name)
		}
	}
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

// assertUniqueIndexColumns fails if table does not contain a matching unique index.
func assertUniqueIndexColumns(t *testing.T, table *entsqlschema.Table, names []string) {
	t.Helper()
	for _, idx := range table.Indexes {
		if !idx.Unique || len(idx.Columns) != len(names) {
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
	t.Fatalf("table %q missing unique index over columns %#v", table.Name, names)
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
