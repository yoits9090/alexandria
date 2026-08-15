package search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAPIHandler(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	Handler(NewService(nil, nil)).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("code=%d type=%q", rr.Code, rr.Header().Get("Content-Type"))
	}
	var spec map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatal(err)
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatal("missing paths")
	}
	for _, p := range []string{"/v1/search", "/search", "/api/search", "/healthz", "/readyz", "/openapi.json"} {
		if paths[p] == nil {
			t.Fatalf("missing path %s", p)
		}
	}
	components, ok := spec["components"].(map[string]any)
	if !ok || components["schemas"] == nil || components["securitySchemes"] == nil {
		t.Fatal("missing components")
	}
	search := paths["/v1/search"].(map[string]any)
	if search["get"] == nil || search["post"] == nil {
		t.Fatal("missing search methods")
	}
}
