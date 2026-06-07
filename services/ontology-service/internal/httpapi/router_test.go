package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cubicle/services/ontology-service/internal/fixtures"
)

func TestHealthzReturnsOK(t *testing.T) {
	router := NewRouter(fixtures.NewFlinkAutoscalerStore(), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "$schema") {
		t.Fatalf("health response leaked schema metadata into DTO body: %s", rec.Body.String())
	}

	var response HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("health response is not JSON: %v", err)
	}
	if !response.OK {
		t.Fatalf("expected ok health response, got %#v", response)
	}
}

func TestOpenAPIDocumentIncludesHealthAndGraph(t *testing.T) {
	router := NewRouter(fixtures.NewFlinkAutoscalerStore(), slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi response is not JSON: %v", err)
	}
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi document has no paths: %#v", doc)
	}
	for _, path := range []string{"/healthz", "/v1/graph/expand"} {
		if _, ok := paths[path]; !ok {
			t.Fatalf("openapi document missing %s: %#v", path, paths)
		}
	}
}
