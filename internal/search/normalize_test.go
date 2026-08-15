package search

import (
	"testing"
)

func TestCanonicalURLAndNormalize(t *testing.T) {
	got := normalize([]ProviderResult{{Title: "  Hello \n world ", URL: "HTTPS://Example.COM/x?gclid=1&utm_medium=z&a=1#frag"}, {Title: "dup", URL: "https://example.com/x?a=1"}}, "x")
	if len(got) != 1 || got[0].URL != "https://example.com/x?a=1" || got[0].Title != "Hello world" {
		t.Fatalf("%#v", got)
	}
}
func TestBudgetNeverExceeds(t *testing.T) {
	r, u := pack([]SearchResult{{Title: "Title", URL: "https://example.com", Snippet: "lots of words lots of words lots of words"}}, 40, ApproxTokenizer{})
	if u.OutputTokens > 40 {
		t.Fatalf("%#v %#v", r, u)
	}
}

func TestUnsafeURLRejected(t *testing.T) {
	if got := canonicalURL("javascript:alert(1)"); got != "" {
		t.Fatalf("got %q", got)
	}
}
func TestGlobalDedup(t *testing.T) {
	got := dedupeResults([]SearchResult{{URL: "https://e.test"}, {URL: "https://e.test"}, {URL: "https://two.test"}})
	if len(got) != 2 {
		t.Fatalf("%#v", got)
	}
}

func TestTinyBudgetTruncates(t *testing.T) {
	r, u := pack([]SearchResult{{Title: "T", URL: "https://e.test"}}, 1, ApproxTokenizer{})
	if len(r) != 0 || !u.Truncated {
		t.Fatalf("%#v %#v", r, u)
	}
}
