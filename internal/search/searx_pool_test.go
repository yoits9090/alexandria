package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSearXPoolDiscoversAndSearchesOnlineNode(t *testing.T) {
	var registryURL string
	node := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" || r.URL.Query().Get("format") != "json" {
			http.Error(w, "bad", 400)
			return
		}
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]string{"title": "Live", "url": "https://example.test", "content": "real fixture"}}})
	}))
	defer node.Close()
	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"instances": map[string]any{node.URL: map[string]any{"http": map[string]any{"status_code": 200}}}})
	}))
	defer reg.Close()
	registryURL = reg.URL
	p := NewSearXPool(registryURL, 5, reg.Client())
	p.refreshEvery = time.Hour
	p.client = reg.Client()
	page, err := p.Search(context.Background(), ProviderQuery{Query: "q", Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 1 || page.Results[0].Title != "Live" {
		t.Fatalf("%#v", page)
	}
	if !strings.Contains(page.Results[0].URL, "example.test") {
		t.Fatal(page.Results)
	}
}
func TestSearXPoolNoJSONNode(t *testing.T) {
	node := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/html")
		w.Write([]byte("<html/>"))
	}))
	defer node.Close()
	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"instances": map[string]any{node.URL: map[string]any{"http": map[string]any{"status_code": 200}}}})
	}))
	defer reg.Close()
	p := NewSearXPool(reg.URL, 5, reg.Client())
	if _, err := p.Search(context.Background(), ProviderQuery{Query: "q"}); err == nil {
		t.Fatal("expected no JSON node")
	}
}
