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

func TestSearXPoolRotationAndCooldown(t *testing.T) {
	good := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]string{"title": "Live", "url": "https://example.test", "content": "real fixture"}}})
	}))
	defer good.Close()
	bad := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	}))
	defer bad.Close()
	p := NewSearXPool("", 5, good.Client())
	p.refreshEvery = time.Hour
	now := time.Now()
	p.now = func() time.Time { return now }
	p.mu.Lock()
	p.nodes = []searxNode{{url: bad.URL}, {url: good.URL}}
	p.next = 0
	p.lastRefresh = now
	p.mu.Unlock()
	page, err := p.Search(context.Background(), ProviderQuery{Query: "q", Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 1 || page.Results[0].Title != "Live" {
		t.Fatalf("results=%#v", page.Results)
	}
	p.mu.Lock()
	cooling := p.nodes[0].cooldownUntil.After(now)
	// Two candidate picks (bad then good) wrap the rotation pointer to 0.
	wrapped := p.next == 0
	p.mu.Unlock()
	if !cooling || !wrapped {
		t.Fatalf("cooldown=%v wrapped=%v", cooling, wrapped)
	}
	page, err = p.Search(context.Background(), ProviderQuery{Query: "q", Count: 2})
	if err != nil || len(page.Results) != 1 {
		t.Fatalf("second search err=%v results=%#v", err, page.Results)
	}
}

func TestSearXPoolAllNodesCoolingDown(t *testing.T) {
	a := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", 500) }))
	defer a.Close()
	b := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "down", 500) }))
	defer b.Close()
	p := NewSearXPool("", 5, a.Client())
	p.refreshEvery = time.Hour
	now := time.Now()
	p.now = func() time.Time { return now }
	p.mu.Lock()
	p.nodes = []searxNode{{url: a.URL, cooldownUntil: now.Add(time.Hour)}, {url: b.URL, cooldownUntil: now.Add(time.Hour)}}
	p.next = 0
	p.lastRefresh = now
	p.mu.Unlock()
	_, err := p.Search(context.Background(), ProviderQuery{Query: "q"})
	if err == nil || !strings.Contains(err.Error(), "cooling down") {
		t.Fatalf("err=%v", err)
	}
}

func TestSearXPoolRefreshFailureFallsBackToCachedNodes(t *testing.T) {
	good := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]string{"title": "Cached", "url": "https://cached.test", "content": "still works"}}})
	}))
	defer good.Close()
	reg := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "registry down", 500)
	}))
	defer reg.Close()
	p := NewSearXPool(reg.URL, 5, good.Client())
	p.refreshEvery = time.Hour
	now := time.Now()
	p.now = func() time.Time { return now }
	p.mu.Lock()
	p.nodes = []searxNode{{url: good.URL}}
	p.next = 0
	p.lastRefresh = now.Add(-2 * time.Hour) // stale, so a refresh is attempted
	p.mu.Unlock()
	page, err := p.Search(context.Background(), ProviderQuery{Query: "q", Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 1 || page.Results[0].Title != "Cached" {
		t.Fatalf("results=%#v", page.Results)
	}
}
