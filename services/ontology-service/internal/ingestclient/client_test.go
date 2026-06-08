package ingestclient

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/httpapi"
	"cubicle/services/ontology-service/internal/ingestpipeline"
	"cubicle/services/ontology-service/internal/storage"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestHTTPClientRunsFixtureImportThroughIngestRoutes(t *testing.T) {
	ctx := context.Background()
	store := openEntStore(t, ctx)
	server := httptest.NewServer(httpapi.NewRouter(store, testLogger()))
	t.Cleanup(server.Close)

	client := New(server.URL, server.Client())
	result, err := ingestpipeline.NewFixtureImporter(client, httpFakeMapper{}).Import(ctx, ingestpipeline.FixtureImportConfig{
		FixtureDir:   writeHTTPFixture(t),
		SnapshotRoot: t.TempDir(),
		Now:          time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC),
		RunKeyPrefix: "run:http-client-fixture",
	})
	if err != nil {
		t.Fatalf("import fixture over HTTP: %v", err)
	}
	if result.RunsCompleted != 1 || result.SnapshotsWritten != 1 {
		t.Fatalf("unexpected import result: %#v", result)
	}

	graph, err := store.Expand(ctx, domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:custom-project"},
		Depth:        1,
		LimitPerNode: 20,
	})
	if err != nil {
		t.Fatalf("expand imported graph: %v", err)
	}
	assertHTTPImportedNode(t, graph, "ticket:TICKET-1")
	assertHTTPImportedEdge(t, graph, domain.PredicateContains)

	statuses, err := client.ListSourceStatus(ctx)
	if err != nil {
		t.Fatalf("list source status over HTTP: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("source status count = %d: %#v", len(statuses), statuses)
	}
}

func TestHTTPClientReturnsProblemDetailForConflict(t *testing.T) {
	ctx := context.Background()
	store := openEntStore(t, ctx)
	server := httptest.NewServer(httpapi.NewRouter(store, testLogger()))
	t.Cleanup(server.Close)

	client := New(server.URL, server.Client())
	run, err := client.BeginIngestRun(ctx, domain.IngestRunStart{
		RunKey:         "run:http-client-conflict",
		Source:         "custom",
		SourceInstance: "example/project",
	})
	if err != nil {
		t.Fatalf("begin run: %v", err)
	}
	write := domain.SourceSnapshotWrite{
		RunKey:         run.RunKey,
		Source:         run.Source,
		SourceInstance: run.SourceInstance,
		SnapshotKey:    "snapshot:custom:conflict",
		BodySHA256:     "sha256:first",
		BodyRef:        "sha256/first",
	}
	if _, err := client.WriteSnapshot(ctx, write); err != nil {
		t.Fatalf("write first snapshot: %v", err)
	}
	write.BodySHA256 = "sha256:second"
	write.BodyRef = "sha256/second"
	_, err = client.WriteSnapshot(ctx, write)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, body=%s", httpErr.StatusCode, httpErr.Body)
	}
	if !strings.Contains(httpErr.Body, string(domain.IngestErrorConflict)) {
		t.Fatalf("conflict body missing category: %s", httpErr.Body)
	}
}

func openEntStore(t *testing.T, ctx context.Context) *graphstore.EntStore {
	t.Helper()
	sqlite, err := storage.Open(ctx, storage.Config{DatabasePath: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlite.Close() })
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, sqlite.DB())))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return graphstore.NewEntStore(client)
}

func assertHTTPImportedNode(t *testing.T, graph domain.Neighborhood, key string) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("missing node %q in %#v", key, graph.Nodes)
}

func assertHTTPImportedEdge(t *testing.T, graph domain.Neighborhood, predicate domain.Predicate) {
	t.Helper()
	for _, edge := range graph.Edges {
		if edge.Metadata.Predicate == predicate {
			return
		}
	}
	t.Fatalf("missing edge predicate %q in %#v", predicate, graph.Edges)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func writeHTTPFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{
		"snapshots": [{
			"snapshot_key": "snapshot:custom:ticket:1",
			"source": "custom",
			"source_instance": "example/project",
			"source_object_kind": "custom_ticket",
			"source_object_id": "TICKET-1",
			"source_url": "https://example.test/tickets/1",
			"path": "ticket.json"
		}]
	}`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ticket.json"), []byte(`{"id":"TICKET-1"}`), 0o644); err != nil {
		t.Fatalf("write ticket: %v", err)
	}
	return dir
}

type httpFakeMapper struct{}

func (httpFakeMapper) Map(records []ingestpipeline.SnapshotRecord, opts ingestpipeline.MapOptions) ([]domain.IngestBatch, error) {
	return []domain.IngestBatch{{
		RunKey:         opts.RunKeyPrefix + ":custom",
		Source:         records[0].Source,
		SourceInstance: records[0].SourceInstance,
		Slice:          opts.Slice,
		MapperVersion:  "custom-fixture/v1",
		SnapshotKeys:   []string{records[0].SnapshotKey},
		ObservedAt:     opts.ObservedAt,
		Nodes: []domain.Node{{
			Kind:        domain.KindWorkstream,
			Key:         "workstream:custom-project",
			Title:       "Custom Project",
			SnapshotKey: records[0].SnapshotKey,
		}, {
			Kind:        domain.KindTicket,
			Key:         "ticket:TICKET-1",
			Title:       "TICKET-1",
			SnapshotKey: records[0].SnapshotKey,
		}},
		Evidence: []domain.Evidence{{
			EvidenceKey: "evidence:custom:TICKET-1",
			SnapshotKey: records[0].SnapshotKey,
			TextHash:    "sha256:evidence",
		}},
		Edges: []domain.Edge{{
			From: domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:custom-project"},
			To:   domain.NodeRef{Kind: domain.KindTicket, Key: "ticket:TICKET-1"},
			Metadata: domain.EdgeMetadata{
				Predicate:   domain.PredicateContains,
				EvidenceKey: "evidence:custom:TICKET-1",
				SnapshotKey: records[0].SnapshotKey,
			},
		}},
	}}, nil
}
