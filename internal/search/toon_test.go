package search

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEncodeTOONTabularAndEscaping(t *testing.T) {
	doc, err := EncodeTOON(SearchResponse{Query: "q,go", Results: []SearchResult{{Title: "One", URL: "https://one.test", Snippet: "short, \"quoted\"", Source: "searx"}, {Title: "Two", URL: "https://two.test", Snippet: "", Source: "searx"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc, "sources[2]{id,title,url,snippet,source}:") {
		t.Fatalf("not tabular TOON:\n%s", doc)
	}
	if !strings.Contains(doc, `"short, \"quoted\""`) {
		t.Fatalf("escaping missing: %s", doc)
	}
	if !strings.Contains(doc, "  2,Two,") {
		t.Fatalf("stable IDs missing: %s", doc)
	}
}
func TestEncodeTOONEmpty(t *testing.T) {
	doc, err := EncodeTOON(SearchResponse{Query: "q"})
	if err != nil || doc != "query: q\nsources: []" {
		t.Fatalf("%v %q", err, doc)
	}
}
func TestTOONHandler(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", page: ProviderPage{Results: []ProviderResult{{Title: "T", URL: "https://e.test"}}}}}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/search?q=q&format=toon", nil)
	Handler(s).ServeHTTP(rr, req)
	if rr.Code != 200 || rr.Header().Get("Content-Type") != "text/toon; charset=utf-8" || !strings.Contains(rr.Body.String(), "sources[1]{id,title,url,snippet,source}") {
		t.Fatalf("%d %q", rr.Code, rr.Body.String())
	}
}
