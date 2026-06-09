package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealthzReturnsOK(t *testing.T) {
	router := NewRouter(slog.Default())

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
	if !response.OK || response.Service != "ontology-service" {
		t.Fatalf("expected ok health response, got %#v", response)
	}
}

func TestGraphQLHealthQuery(t *testing.T) {
	router := NewRouter(slog.Default())

	body := `{"query":"query { health { ok service } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Data struct {
			Health HealthResponse `json:"health"`
		} `json:"data"`
		Errors []any `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("graphql response is not JSON: %v", err)
	}
	if len(response.Errors) > 0 {
		t.Fatalf("graphql response had errors: %#v", response.Errors)
	}
	if !response.Data.Health.OK || response.Data.Health.Service != "ontology-service" {
		t.Fatalf("unexpected graphql health response: %#v", response)
	}
}

func TestGraphQLPlaygroundIsMounted(t *testing.T) {
	router := NewRouter(slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cubicle Ontology GraphQL") {
		t.Fatalf("playground response did not contain title: %s", rec.Body.String())
	}
}
