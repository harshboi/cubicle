package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"cubicle/services/ontology-service/internal/query"
	"cubicle/services/ontology-service/internal/sampledata"
)

func TestWorkstreamOverviewReturnsClassifiedBuckets(t *testing.T) {
	router := NewRouter(sampledata.NewFakeFlinkAutoscalerMemoryStore(), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v1/workstreams/flink-autoscaler/overview", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var overview query.WorkstreamOverview
	if err := json.Unmarshal(rec.Body.Bytes(), &overview); err != nil {
		t.Fatalf("overview response is not JSON: %v", err)
	}
	if overview.Workstream.Key != "workstream:flink-autoscaler" {
		t.Fatalf("unexpected workstream: %#v", overview.Workstream)
	}
	if len(overview.Tickets) == 0 || len(overview.Blockers) == 0 || len(overview.ActionCandidates) == 0 {
		t.Fatalf("expected classified workstream buckets, got %#v", overview)
	}
}
