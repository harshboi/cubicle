package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/sampledata"
)

func TestGraphExpandReturnsNeighborhood(t *testing.T) {
	router := NewRouter(sampledata.NewFakeFlinkAutoscalerMemoryStore(), slog.Default())

	body := `{"start":{"object_type":"workstream","key":"workstream:flink-autoscaler"},"depth":2,"limit_per_object":10}`
	req := httptest.NewRequest(http.MethodPost, "/v1/graph/expand", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var graph domain.Neighborhood
	if err := json.Unmarshal(rec.Body.Bytes(), &graph); err != nil {
		t.Fatalf("expand response is not graph JSON: %v", err)
	}
	if len(graph.Objects) < 4 {
		t.Fatalf("expected graph objects, got %#v", graph.Objects)
	}
	if len(graph.Associations) < 3 {
		t.Fatalf("expected graph associations, got %#v", graph.Associations)
	}
}

func TestGraphExpandRejectsInvalidBounds(t *testing.T) {
	router := NewRouter(sampledata.NewFakeFlinkAutoscalerMemoryStore(), slog.Default())

	body := `{"start":{"object_type":"workstream","key":"workstream:flink-autoscaler"},"depth":2,"limit_per_object":0}`
	req := httptest.NewRequest(http.MethodPost, "/v1/graph/expand", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "application/problem+json") {
		t.Fatalf("expected problem+json error response, got %q", contentType)
	}

	var problem struct {
		Title  string `json:"title"`
		Status int    `json:"status"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("error response is not problem JSON: %v", err)
	}
	if problem.Status != http.StatusBadRequest || problem.Title == "" || problem.Detail == "" {
		t.Fatalf("unexpected problem response: %#v", problem)
	}
}
