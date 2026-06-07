package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cubicle/services/ontology-service/internal/domain"
	"cubicle/services/ontology-service/internal/graphstore"
)

func TestGraphUpsertThenExpandReturnsImportedData(t *testing.T) {
	store := graphstore.NewMemoryStore()
	router := NewRouter(store, slog.Default())

	body := `{
		"nodes": [
			{"kind":"workstream","key":"workstream:test","title":"Test Workstream"},
			{"kind":"ticket","key":"ticket:TEST-1","title":"Imported ticket"}
		],
		"edges": [
			{
				"from":{"kind":"workstream","key":"workstream:test"},
				"to":{"kind":"ticket","key":"ticket:TEST-1"},
				"metadata":{"predicate":"contains","evidence_key":"evidence:test","source":"crawler","confidence":1}
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/graph/upsert", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var response GraphUpsertResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("upsert response is not JSON: %v", err)
	}
	if response.NodeCount != 2 || response.EdgeCount != 1 {
		t.Fatalf("unexpected upsert counts: %#v", response)
	}

	graph, err := store.Expand(t.Context(), domain.ExpandRequest{
		Start:        domain.NodeRef{Kind: domain.KindWorkstream, Key: "workstream:test"},
		Depth:        1,
		LimitPerNode: 10,
	})
	if err != nil {
		t.Fatalf("expand imported graph: %v", err)
	}
	assertGraphNode(t, graph, "ticket:TEST-1")
}

func assertGraphNode(t *testing.T, graph domain.Neighborhood, key string) {
	t.Helper()
	for _, node := range graph.Nodes {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("missing node %s in %#v", key, graph.Nodes)
}
