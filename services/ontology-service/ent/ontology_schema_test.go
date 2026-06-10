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

// TestSourceEvidenceSpineDeclaresCoverageIdentityObservationAndAnchorTables
// documents the source-neutral provenance spine that later ingestion and query
// code must use before returning evidence text.
func TestSourceEvidenceSpineDeclaresCoverageIdentityObservationAndAnchorTables(t *testing.T) {
	sourceRuns := findTable(t, "source_runs")
	assertColumn(t, sourceRuns, "run_key")
	assertColumn(t, sourceRuns, "source_key")
	assertColumn(t, sourceRuns, "source_instance")
	assertColumn(t, sourceRuns, "scope_kind")
	assertColumn(t, sourceRuns, "scope_key")
	assertColumn(t, sourceRuns, "status")
	assertColumn(t, sourceRuns, "checkpoint_token")
	assertUniqueIndexColumns(t, sourceRuns, []string{"source_key", "source_instance", "run_key"})
	assertIndexColumns(t, sourceRuns, []string{"source_key", "source_instance", "scope_kind", "scope_key", "started_at"})

	externalIdentities := findTable(t, "external_identities")
	assertColumn(t, externalIdentities, "target_kind")
	assertColumn(t, externalIdentities, "target_id")
	assertColumn(t, externalIdentities, "external_kind")
	assertColumn(t, externalIdentities, "external_id")
	assertColumn(t, externalIdentities, "identity_status")
	assertColumn(t, externalIdentities, "replaced_by_identity_id")
	assertUniqueIndexColumns(t, externalIdentities, []string{"source_key", "source_instance", "external_kind", "external_id"})
	assertIndexColumns(t, externalIdentities, []string{"target_kind", "target_id", "identity_status"})

	sourceObservations := findTable(t, "source_observations")
	assertColumn(t, sourceObservations, "source_run_id")
	assertColumn(t, sourceObservations, "external_identity_id")
	assertColumn(t, sourceObservations, "observed_kind")
	assertColumn(t, sourceObservations, "is_deleted")
	assertColumn(t, sourceObservations, "permission_policy_key")
	assertColumn(t, sourceObservations, "visibility_hash")
	assertColumn(t, sourceObservations, "content_hash")
	assertUniqueIndexColumns(t, sourceObservations, []string{"source_run_id", "external_identity_id"})
	assertIndexColumns(t, sourceObservations, []string{"permission_policy_key", "visibility_hash"})

	evidenceAnchors := findTable(t, "evidence_anchors")
	assertColumn(t, evidenceAnchors, "source_observation_id")
	assertColumn(t, evidenceAnchors, "anchor_kind")
	assertColumn(t, evidenceAnchors, "anchor_locator")
	assertColumn(t, evidenceAnchors, "source_span_key")
	assertColumn(t, evidenceAnchors, "text_hash")
	assertColumn(t, evidenceAnchors, "text_preview")
	assertColumn(t, evidenceAnchors, "text_preview_truncated")
	assertColumn(t, evidenceAnchors, "lexical_fingerprint")
	assertUniqueIndexColumns(t, evidenceAnchors, []string{"source_observation_id", "anchor_kind", "source_span_key", "text_hash"})

	evidences := findTable(t, "evidences")
	assertColumn(t, evidences, "evidence_anchor_id")
	assertIndexColumns(t, evidences, []string{"evidence_anchor_id"})
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
