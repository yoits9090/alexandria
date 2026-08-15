package search

import "context"

// SearchRequest is Alexandria's provider-neutral API. max_tokens is a hard
// budget for the returned search context, not a promise to call an LLM.
type SearchRequest struct {
	Query          string   `json:"query"`
	Providers      []string `json:"providers,omitempty"`
	MaxResults     int      `json:"max_results,omitempty"`
	MaxTokens      int      `json:"max_tokens,omitempty"`
	SearchDepth    string   `json:"search_depth,omitempty"`
	Freshness      string   `json:"freshness,omitempty"`
	IncludeDomains []string `json:"include_domains,omitempty"`
	ExcludeDomains []string `json:"exclude_domains,omitempty"`
	Language       string   `json:"language,omitempty"`
	Region         string   `json:"region,omitempty"`
	SafeSearch     *bool    `json:"safe_search,omitempty"`
	Content        string   `json:"content,omitempty"`
}

type SearchResult struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet,omitempty"`
	Source      string  `json:"source"`
	PublishedAt string  `json:"published_at,omitempty"`
	Rank        int     `json:"rank"`
	Score       float64 `json:"score,omitempty"`
}

type ProviderStatus struct {
	Name      string `json:"name"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	Results   int    `json:"results,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

type Usage struct {
	InputTokens  int  `json:"input_tokens,omitempty"`
	OutputTokens int  `json:"output_tokens"`
	TotalTokens  int  `json:"total_tokens"`
	MaxTokens    int  `json:"max_tokens"`
	Truncated    bool `json:"truncated"`
	Estimated    bool `json:"estimated"`
}

type SearchResponse struct {
	Query     string           `json:"query"`
	Results   []SearchResult   `json:"results"`
	Providers []ProviderStatus `json:"providers"`
	Usage     Usage            `json:"usage"`
	RequestID string           `json:"request_id,omitempty"`
}

type ProviderQuery struct {
	Query                          string
	Count                          int
	SearchDepth                    string
	Freshness                      string
	IncludeDomains, ExcludeDomains []string
	Language, Region               string
	SafeSearch                     *bool
}

type ProviderResult struct {
	Title, URL, Snippet, PublishedAt string
	Score                            float64
}

type ProviderPage struct {
	Results   []ProviderResult
	RequestID string
}

type Capabilities struct {
	Freshness, Domains, Language, Region, SafeSearch, SearchDepth bool
	FreshnessValues                                               map[string]bool
	SearchDepthValues                                             map[string]bool
}

type Provider interface {
	Name() string
	Capabilities() Capabilities
	Search(context.Context, ProviderQuery) (ProviderPage, error)
}
