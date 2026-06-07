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

func TestErrorResponseJSONShape(t *testing.T) {
	body, err := json.Marshal(ErrorResponse{Code: "invalid_request", Message: "depth must be non-negative"})
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}
	if string(body) != `{"code":"invalid_request","message":"depth must be non-negative"}` {
		t.Fatalf("unexpected error response JSON: %s", body)
	}
}
