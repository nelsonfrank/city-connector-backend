package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSwaggerUIEndpoints(t *testing.T) {
	_, h := newTestServer(t)

	// Test /docs serves Swagger UI HTML
	rec := do(h, http.MethodGet, "/docs")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /docs = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "SwaggerUIBundle") {
		t.Errorf("expected Swagger UI HTML, got: %s", rec.Body.String()[:100])
	}

	// Test /docs/openapi.json returns valid JSON specification
	rec = do(h, http.MethodGet, "/docs/openapi.json")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /docs/openapi.json = %d, want 200", rec.Code)
	}
	var spec map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &spec); err != nil {
		t.Fatalf("invalid OpenAPI JSON: %v", err)
	}
	if spec["openapi"] != "3.0.3" {
		t.Errorf("openapi version = %v, want 3.0.3", spec["openapi"])
	}
}
