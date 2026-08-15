package search

import (
	"context"
	"encoding/json"
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
	page, err := p.Search(context.Background(), ProviderQuery{Query: "hello", Count: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 1 || page.Results[0].Title != "T" {
		t.Fatalf("%#v", page)
	}
	if got.URL.Query().Get("q") != "hello" || got.Header.Get("X-Subscription-Token") != "secret" {
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
