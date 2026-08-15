package search

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Service struct {
	providers                            map[string]Provider
	defaultProviders                     []string
	MaxTokens, DefaultTokens, MaxResults int
	Tokenizer                            TokenEstimator
	Timeout                              time.Duration
}

func NewService(ps []Provider, defaults []string) *Service {
	m := map[string]Provider{}
	for _, p := range ps {
		m[p.Name()] = p
	}
	if len(defaults) == 0 {
		for n := range m {
			defaults = append(defaults, n)
		}
		sort.Strings(defaults)
	}
	return &Service{providers: m, defaultProviders: defaults, MaxTokens: 8000, DefaultTokens: 1200, MaxResults: 10, Tokenizer: ApproxTokenizer{}, Timeout: 12 * time.Second}
}
func (s *Service) ProviderNames() []string {
	out := make([]string, 0, len(s.providers))
	for n := range s.providers {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
func (s *Service) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	if strings.TrimSpace(req.Query) == "" {
		return SearchResponse{}, fmt.Errorf("query is required")
	}
	names := req.Providers
	if len(names) == 0 {
		names = s.defaultProviders
	}
	if len(names) == 0 {
		return SearchResponse{}, fmt.Errorf("no search providers configured")
	}
	maxR := req.MaxResults
	if maxR <= 0 {
		maxR = s.MaxResults
	}
	if maxR > s.MaxResults && s.MaxResults > 0 {
		maxR = s.MaxResults
	}
	maxT := req.MaxTokens
	if maxT <= 0 {
		maxT = s.DefaultTokens
	}
	if maxT > s.MaxTokens {
		maxT = s.MaxTokens
	}
	if maxT <= 0 {
		return SearchResponse{}, fmt.Errorf("max_tokens must be positive")
	}
	q := ProviderQuery{Query: strings.TrimSpace(req.Query), Count: maxR * 2, SearchDepth: req.SearchDepth, Freshness: req.Freshness, IncludeDomains: req.IncludeDomains, ExcludeDomains: req.ExcludeDomains, Language: req.Language, Region: req.Region, SafeSearch: req.SafeSearch}
	type result struct {
		name    string
		page    ProviderPage
		err     error
		latency int64
	}
	ch := make(chan result, len(names))
	var wg sync.WaitGroup
	for _, name := range names {
		p, ok := s.providers[name]
		if !ok {
			return SearchResponse{}, fmt.Errorf("unknown provider %q", name)
		}
		if req.Freshness != "" && !p.Capabilities().Freshness {
			return SearchResponse{}, fmt.Errorf("provider %q does not support freshness", name)
		}
		if len(req.IncludeDomains) > 0 && !p.Capabilities().Domains {
			return SearchResponse{}, fmt.Errorf("provider %q does not support include_domains", name)
		}
		wg.Add(1)
		go func(name string, p Provider) {
			defer wg.Done()
			start := time.Now()
			cctx := ctx
			if s.Timeout > 0 {
				var cancel context.CancelFunc
				cctx, cancel = context.WithTimeout(ctx, s.Timeout)
				defer cancel()
			}
			page, err := p.Search(cctx, q)
			ch <- result{name: name, page: page, err: err, latency: time.Since(start).Milliseconds()}
		}(name, p)
	}
	wg.Wait()
	close(ch)
	all := []SearchResult{}
	statuses := []ProviderStatus{}
	success := 0
	for r := range ch {
		st := ProviderStatus{Name: r.name, OK: r.err == nil, LatencyMS: r.latency}
		if r.err != nil {
			st.Error = redactError(r.err)
		} else {
			success++
			norm := normalize(r.page.Results, r.name)
			st.Results = len(norm)
			all = append(all, norm...)
		}
		statuses = append(statuses, st)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].Name < statuses[j].Name })
	if success == 0 {
		return SearchResponse{Query: req.Query, Providers: statuses, Usage: Usage{MaxTokens: maxT}}, fmt.Errorf("all providers failed")
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		if all[i].Source != all[j].Source {
			return all[i].Source < all[j].Source
		}
		return all[i].URL < all[j].URL
	})
	if len(all) > maxR {
		all = all[:maxR]
	}
	for i := range all {
		all[i].Rank = i + 1
	}
	packed, usage := pack(all, maxT, s.Tokenizer)
	return SearchResponse{Query: req.Query, Results: packed, Providers: statuses, Usage: usage}, nil
}
func redactError(err error) string {
	s := err.Error()
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}

func Handler(s *Service) http.Handler {
	return HandlerWithAPIKey(s, "")
}

// HandlerWithAPIKey constructs the gateway handler. The API key applies only
// to search routes; operational endpoints stay available to probes.
func HandlerWithAPIKey(s *Service, apiKey string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/openapi.json", OpenAPIHandler)
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if len(s.providers) == 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "providers": s.ProviderNames()})
	})
	searchEndpoint := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			values := r.URL.Query()
			q := values.Get("q")
			if q == "" {
				q = values.Get("query")
			}
			req := SearchRequest{Query: q, MaxResults: atoiPositive(values.Get("count"), atoiPositive(values.Get("max_results"), 0)), MaxTokens: atoiPositive(values.Get("max_tokens"), 0)}
			if names := splitCSV(values.Get("providers")); len(names) > 0 {
				req.Providers = names
			}
			resp, err := s.Search(r.Context(), req)
			if format := values.Get("format"); format != "" && err == nil {
				writeFormat(w, format, resp)
				return
			}
			respondSearch(w, resp, err)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "GET, POST")
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req SearchRequest
		if err := decodeJSON(r, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		resp, err := s.Search(r.Context(), req)
		respondSearch(w, resp, err)
	}
	protectedSearch := APIKeyAuth(http.HandlerFunc(searchEndpoint), apiKey)
	mux.Handle("/v1/search", protectedSearch)
	// Compatibility aliases for the former ugpt-search/SearX-style clients.
	mux.Handle("/search", protectedSearch)
	mux.Handle("/api/search", protectedSearch)
	return requestID(mux)
}

func splitCSV(v string) []string {
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
func atoiPositive(v string, fallback int) int {
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
func writeFormat(w http.ResponseWriter, format string, resp SearchResponse) {
	switch strings.ToLower(format) {
	case "text", "txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, r := range resp.Results {
			fmt.Fprintf(w, "%d. %s\n%s\n%s\n\n", r.Rank, r.Title, r.URL, r.Snippet)
		}
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, "<!doctype html><meta charset=utf-8><title>Alexandria search</title>")
		for _, r := range resp.Results {
			fmt.Fprintf(w, "<article><h2><a href=\"%s\">%s</a></h2><p>%s</p></article>", html.EscapeString(r.URL), html.EscapeString(r.Title), html.EscapeString(r.Snippet))
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "format must be json, text, or html"})
	}
}
