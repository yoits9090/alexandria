package search

import (
	"encoding/json"
	"strings"
)

const minimumTokenBudget = 34

// TokenEstimator deliberately has no tokenizer dependency. It is conservative
// enough for an API boundary and can be replaced by a model-specific tokenizer.
type TokenEstimator interface {
	Count(string) int
	Truncate(string, int) string
}
type ApproxTokenizer struct{}

func (ApproxTokenizer) Count(s string) int {
	if s == "" {
		return 0
	}
	return (len([]rune(s)) + 3) / 4
}
func (ApproxTokenizer) Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	limit := n * 4
	if len(rs) <= limit {
		return s
	}
	prefix := string(rs[:limit])
	cut := strings.LastIndexAny(prefix, " .,!?:;\n\t")
	if cut < len(prefix)/2 {
		cut = len(prefix)
	}
	return strings.TrimSpace(prefix[:cut]) + "…"
}

func pack(results []SearchResult, maxTokens int, tok TokenEstimator) ([]SearchResult, Usage) {
	if tok == nil {
		tok = ApproxTokenizer{}
	}
	if maxTokens <= 0 {
		maxTokens = 1
	}
	// Count the actual serialized result envelope on every append. This makes
	// max_tokens a hard upper bound even when the approximation is replaced by
	// an exact model tokenizer later.
	reserve := 32
	packed := make([]SearchResult, 0, len(results))
	truncated := false
	for _, original := range results {
		candidate := original
		fits := func(v SearchResult) bool {
			trial := append(append([]SearchResult{}, packed...), v)
			return reserve+tok.Count(mustJSON(trial)) <= maxTokens
		}
		if fits(candidate) {
			packed = append(packed, candidate)
			continue
		}
		// Preserve title and URL, then binary-search the largest snippet that fits.
		candidate.Snippet = ""
		if !fits(candidate) {
			truncated = true
			break
		}
		lo, hi := 0, len([]rune(original.Snippet))
		best := candidate
		for lo <= hi {
			mid := (lo + hi) / 2
			probe := candidate
			probe.Snippet = tok.Truncate(original.Snippet, mid)
			if fits(probe) {
				best = probe
				lo = mid + 1
			} else {
				hi = mid - 1
			}
		}
		packed = append(packed, best)
		if best.Snippet != original.Snippet {
			truncated = true
		}
	}
	used := reserve + tok.Count(mustJSON(packed))
	if used > maxTokens {
		used = maxTokens
		truncated = true
	}
	return packed, Usage{OutputTokens: used, TotalTokens: used, MaxTokens: maxTokens, Truncated: truncated, Estimated: true}
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
