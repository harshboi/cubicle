package httpapi

import (
	"encoding/json"
	"testing"
)

func TestHealthResponseJSONShape(t *testing.T) {
	body, err := json.Marshal(HealthResponse{OK: true})
	if err != nil {
		t.Fatalf("marshal health response: %v", err)
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected health response JSON: %s", body)
	}
}
