package search

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBraveAdapter(t *testing.T) {
	var got http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"web": map[string]any{"results": []any{map[string]string{"title": "T", "url": "https://e.test", "description": "S"}}}})
	}))
	defer ts.Close()
	p := NewBrave(ts.URL, "secret", ts.Client())
	page, err := p.Search(context.Background(), ProviderQuery{Query: "hello", Count: 2, Freshness: "week"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 1 || page.Results[0].Title != "T" {
		t.Fatalf("%#v", page)
	}
	if got.URL.Query().Get("q") != "hello" || got.URL.Query().Get("freshness") != "pw" || got.Header.Get("X-Subscription-Token") != "secret" {
		t.Fatalf("request %#v", got)
	}
}
func TestTavilyAdapterDoesNotLeakKeyInURL(t *testing.T) {
	var got http.Request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = *r
		w.Header().Set("content-type", "application/json")
		io := map[string]any{"results": []any{map[string]any{"title": "T", "url": "https://e.test", "content": "S", "score": .9}}}
		json.NewEncoder(w).Encode(io)
	}))
	defer ts.Close()
	_, err := NewTavily(ts.URL, "secret", ts.Client()).Search(context.Background(), ProviderQuery{Query: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.URL.String(), "secret") {
		t.Fatal("key leaked in URL")
	}
	if got.Header.Get("Authorization") != "Bearer secret" {
		t.Fatalf("missing auth header: %q", got.Header.Get("Authorization"))
	}
}
func TestRetry429(t *testing.T) {
	n := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 3 {
			http.Error(w, "busy", 429)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"web":{"results":[]}}`))
	}))
	defer ts.Close()
	_, err := NewBrave(ts.URL, "k", ts.Client()).Search(context.Background(), ProviderQuery{Query: "q"})
	if err != nil || n != 3 {
		t.Fatalf("err=%v n=%d", err, n)
	}
}

func TestTavilyRetryPreservesBodyAndSanitizesError(t *testing.T) {
	attempts := 0
	var bodies []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))
		if attempts < 3 {
			http.Error(w, "secret-provider-body", 429)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"results":[]}`))
	}))
	defer ts.Close()
	_, err := NewTavily(ts.URL, "secret-key", ts.Client()).Search(context.Background(), ProviderQuery{Query: "q"})
	if err != nil || attempts != 3 {
		t.Fatalf("err=%v attempts=%d", err, attempts)
	}
	if len(bodies) != 3 || bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatalf("bodies not preserved: %#v", bodies)
	}
	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "secret-provider-body", 500) }))
	defer ts2.Close()
	_, err = NewTavily(ts2.URL, "secret-key", ts2.Client()).Search(context.Background(), ProviderQuery{Query: "q"})
	if err == nil || strings.Contains(err.Error(), "secret-provider-body") || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("unsanitized error: %v", err)
	}
}

func TestTavilyFreshnessPayload(t *testing.T) {
	var body map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"results":[]}`))
	}))
	defer ts.Close()
	_, err := NewTavily(ts.URL, "k", ts.Client()).Search(context.Background(), ProviderQuery{Query: "q", Freshness: "pw", Region: "us", SafeSearch: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if body["time_range"] != "week" {
		t.Fatalf("payload=%#v", body)
	}
	if _, ok := body["api_key"]; ok {
		t.Fatal("api key in body")
	}
}
func TestSerperFreshnessPayload(t *testing.T) {
	var body map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"organic":[]}`))
	}))
	defer ts.Close()
	_, err := NewSerper(ts.URL, "k", ts.Client()).Search(context.Background(), ProviderQuery{Query: "q", Freshness: "pw"})
	if err != nil {
		t.Fatal(err)
	}
	if body["tbs"] != "qdr:w" {
		t.Fatalf("payload=%#v", body)
	}
}

func boolPtr(v bool) *bool { return &v }

func TestProviderRejectsRedirect(t *testing.T) {
	var targetHits int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits++; w.Write([]byte(`{"web":{"results":[]}}`)) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer source.Close()
	_, err := NewBrave(source.URL, "secret-key", source.Client()).Search(context.Background(), ProviderQuery{Query: "q"})
	if err == nil || targetHits != 0 || strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("redirect behavior err=%v hits=%d", err, targetHits)
	}
}

func TestProviderPropagatesRequestID(t *testing.T) {
	var got string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Request-ID")
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"web":{"results":[]}}`))
	}))
	defer ts.Close()
	ctx := context.WithValue(context.Background(), requestIDContextKey{}, "req-123")
	_, err := NewBrave(ts.URL, "k", ts.Client()).Search(ctx, ProviderQuery{Query: "q"})
	if err != nil || got != "req-123" {
		t.Fatalf("err=%v id=%q", err, got)
	}
}
