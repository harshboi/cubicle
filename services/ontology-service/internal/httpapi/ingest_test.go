package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"cubicle/services/ontology-service/ent"
	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
	"cubicle/services/ontology-service/internal/storage"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestIngestHTTPFlowAndSources(t *testing.T) {
	store := openHTTPEntStore(t)
	router := NewRouter(store, slog.Default())
	runKey := "run-flink-fixture-1"

	rec := postJSON(t, router, "/v1/ingest/runs", `{
		"run_key":"run-flink-fixture-1",
		"source":"jira",
		"source_instance":"apache-jira",
		"slice":"flink-autoscaler",
		"mapper_version":"flink-fixture/v1"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("begin run status = %d: %s", rec.Code, rec.Body.String())
	}

	runPath := url.PathEscape(runKey)
	rec = postJSON(t, router, "/v1/ingest/runs/"+runPath+"/snapshots", `{
		"snapshot_key":"snapshot:jira:FLINK-39743",
		"source_object_kind":"jira_issue",
		"source_object_id":"FLINK-39743",
		"body_sha256":"sha256:issue-body",
		"body_ref":"snapshots/sha256/issue-body.json",
		"source_url":"https://issues.apache.org/jira/browse/FLINK-39743"
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d: %s", rec.Code, rec.Body.String())
	}

	rec = postJSON(t, router, "/v1/ingest/runs/"+runPath+"/batches", `{
		"snapshot_keys":["snapshot:jira:FLINK-39743"],
		"nodes":[
			{"kind":"workstream","key":"workstream:flink-autoscaler","title":"Flink Autoscaler"},
			{"kind":"ticket","key":"ticket:FLINK-39743","title":"Autoscaler bug","snapshot_key":"snapshot:jira:FLINK-39743"}
		],
		"evidence":[
			{"evidence_key":"evidence:jira:FLINK-39743","snapshot_key":"snapshot:jira:FLINK-39743","text_hash":"sha256:evidence","summary":"Ticket is in the autoscaler component."}
		],
		"edges":[
			{
				"from":{"kind":"workstream","key":"workstream:flink-autoscaler"},
				"to":{"kind":"ticket","key":"ticket:FLINK-39743"},
				"metadata":{"predicate":"contains","evidence_key":"evidence:jira:FLINK-39743","snapshot_key":"snapshot:jira:FLINK-39743"}
			}
		],
		"checkpoint":{"checkpoint_key":"jira-start-at","checkpoint_value":"50"}
	}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("batch status = %d: %s", rec.Code, rec.Body.String())
	}
	var batchResult domain.IngestBatchResult
	if err := json.Unmarshal(rec.Body.Bytes(), &batchResult); err != nil {
		t.Fatalf("batch result JSON: %v", err)
	}
	if batchResult.NodesUpserted != 2 || batchResult.EdgesUpserted != 1 {
		t.Fatalf("unexpected batch result: %#v", batchResult)
	}

	rec = postJSON(t, router, "/v1/ingest/runs/"+runPath+"/complete", `{"status":"completed"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("complete status = %d: %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/sources", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sources status = %d: %s", rec.Code, rec.Body.String())
	}
	var statuses []domain.SourceStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &statuses); err != nil {
		t.Fatalf("sources JSON: %v", err)
	}
	if len(statuses) != 1 || statuses[0].LastSuccessfulRunKey != runKey {
		t.Fatalf("unexpected source statuses: %#v", statuses)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/ingest/runs/"+runPath, nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get run status = %d: %s", rec.Code, rec.Body.String())
	}
	var run domain.IngestRun
	if err := json.Unmarshal(rec.Body.Bytes(), &run); err != nil {
		t.Fatalf("run JSON: %v", err)
	}
	if run.Status != domain.IngestRunCompleted {
		t.Fatalf("run status = %q", run.Status)
	}
}

func TestIngestOpenAPIContainsRoutes(t *testing.T) {
	store := openHTTPEntStore(t)
	router := NewRouter(store, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("openapi status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, path := range []string{
		"/v1/ingest/runs",
		"/v1/ingest/runs/{run_id}/snapshots",
		"/v1/ingest/runs/{run_id}/batches",
		"/v1/ingest/runs/{run_id}/complete",
		"/v1/ingest/runs/{run_id}",
		"/v1/sources",
	} {
		if !strings.Contains(body, path) {
			t.Fatalf("openapi missing %s in %s", path, body)
		}
	}
}

func TestIngestHTTPConflictIncludesCategory(t *testing.T) {
	store := openHTTPEntStore(t)
	router := NewRouter(store, slog.Default())
	runKey := "run-flink-fixture-1"
	runPath := url.PathEscape(runKey)

	postJSON(t, router, "/v1/ingest/runs", `{"run_key":"run-flink-fixture-1","source":"jira","source_instance":"apache-jira"}`)
	postJSON(t, router, "/v1/ingest/runs/"+runPath+"/snapshots", `{
		"snapshot_key":"snapshot:jira:FLINK-39743",
		"body_sha256":"sha256:first",
		"body_ref":"snapshots/sha256/first.json"
	}`)
	rec := postJSON(t, router, "/v1/ingest/runs/"+runPath+"/snapshots", `{
		"snapshot_key":"snapshot:jira:FLINK-39743",
		"body_sha256":"sha256:second",
		"body_ref":"snapshots/sha256/second.json"
	}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), string(domain.IngestErrorConflict)) {
		t.Fatalf("conflict response missing category: %s", rec.Body.String())
	}
}

func openHTTPEntStore(t *testing.T) *graphstore.EntStore {
	t.Helper()
	ctx := t.Context()
	sqliteStore, err := storage.Open(ctx, storage.Config{
		DatabasePath: filepath.Join(t.TempDir(), "graph.db"),
	})
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = sqliteStore.Close() })

	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, sqliteStore.DB())))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create ent schema: %v", err)
	}
	return graphstore.NewEntStore(client)
}

func postJSON(t *testing.T, router http.Handler, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
