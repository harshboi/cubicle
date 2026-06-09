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
	"cubicle/services/ontology-service/internal/ontology"
)

func TestGraphUpsertThenExpandReturnsImportedData(t *testing.T) {
	store := graphstore.NewMemoryStore()
	router := NewRouter(store, slog.Default())

	body := `{
		"objects": [
			{"object_type":"workstream","key":"workstream:test","title":"Test Workstream"},
			{"object_type":"ticket","key":"ticket:TEST-1","title":"Imported ticket"}
		],
		"associations": [
			{
				"from":{"object_type":"workstream","key":"workstream:test"},
				"to":{"object_type":"ticket","key":"ticket:TEST-1"},
				"association_type":"contains",
				"metadata":{"evidence_key":"evidence:test","source":"crawler","confidence":1}
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
	if response.ObjectCount != 2 || response.AssociationCount != 1 {
		t.Fatalf("unexpected upsert counts: %#v", response)
	}

	graph, err := store.Expand(t.Context(), domain.ExpandRequest{
		Start:          domain.ObjectRef{ObjectType: ontology.ObjectWorkstream, Key: "workstream:test"},
		Depth:          1,
		LimitPerObject: 10,
	})
	if err != nil {
		t.Fatalf("expand imported graph: %v", err)
	}
	assertGraphNode(t, graph, "ticket:TEST-1")
}

func assertGraphNode(t *testing.T, graph domain.Neighborhood, key string) {
	t.Helper()
	for _, node := range graph.Objects {
		if node.Key == key {
			return
		}
	}
	t.Fatalf("missing object %s in %#v", key, graph.Objects)
}
