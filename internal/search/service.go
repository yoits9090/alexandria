package search

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrBadRequest         = errors.New("bad request")
	ErrAllProvidersFailed = errors.New("all providers failed")
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
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return SearchResponse{}, fmt.Errorf("query is required")
	}
	if req.MaxResults < 0 || req.MaxTokens < 0 {
		return SearchResponse{}, fmt.Errorf("max_results and max_tokens must be non-negative")
	}
	if req.Content != "" && req.Content != "snippets" {
		return SearchResponse{}, fmt.Errorf("content must be snippets")
	}
	names := uniqueNames(req.Providers)
	if len(names) == 0 {
		names = append([]string(nil), s.defaultProviders...)
	}
	if len(names) > 16 {
		return SearchResponse{}, fmt.Errorf("too many providers (maximum 16)")
	}
	if len(names) == 0 {
		return SearchResponse{}, fmt.Errorf("no search providers configured")
	}
	maxR := req.MaxResults
	if maxR == 0 {
		maxR = s.MaxResults
	}
	if maxR <= 0 {
		return SearchResponse{}, fmt.Errorf("max_results must be positive")
	}
	if s.MaxResults > 0 && maxR > s.MaxResults {
		maxR = s.MaxResults
	}
	maxT := req.MaxTokens
	if maxT == 0 {
		maxT = s.DefaultTokens
	}
	if maxT > s.MaxTokens {
		maxT = s.MaxTokens
	}
	if maxT < minimumTokenBudget {
		return SearchResponse{}, fmt.Errorf("max_tokens must be at least %d", minimumTokenBudget)
	}
	count := maxR
	if count > int(^uint(0)>>1)/2 {
		count = int(^uint(0) >> 1)
	} else {
		count *= 2
	}
	q := ProviderQuery{Query: query, Count: count, SearchDepth: req.SearchDepth, Freshness: req.Freshness, IncludeDomains: req.IncludeDomains, ExcludeDomains: req.ExcludeDomains, Language: req.Language, Region: req.Region, SafeSearch: req.SafeSearch}
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
		caps := p.Capabilities()
		if req.Freshness != "" && !caps.Freshness {
			return SearchResponse{}, fmt.Errorf("provider %q does not support freshness", name)
		}
		if req.Freshness != "" && caps.FreshnessValues != nil && !caps.FreshnessValues[strings.ToLower(req.Freshness)] {
			return SearchResponse{}, fmt.Errorf("provider %q does not support freshness value %q", name, req.Freshness)
		}
		if (len(req.IncludeDomains) > 0 || len(req.ExcludeDomains) > 0) && !caps.Domains {
			return SearchResponse{}, fmt.Errorf("provider %q does not support domains", name)
		}
		if req.Language != "" && !caps.Language {
			return SearchResponse{}, fmt.Errorf("provider %q does not support language", name)
		}
		if req.Region != "" && !caps.Region {
			return SearchResponse{}, fmt.Errorf("provider %q does not support region", name)
		}
		if req.SafeSearch != nil && !caps.SafeSearch {
			return SearchResponse{}, fmt.Errorf("provider %q does not support safe_search", name)
		}
		if req.SearchDepth != "" && !caps.SearchDepth {
			return SearchResponse{}, fmt.Errorf("provider %q does not support search_depth", name)
		}
		if req.SearchDepth != "" && caps.SearchDepthValues != nil && !caps.SearchDepthValues[strings.ToLower(req.SearchDepth)] {
			return SearchResponse{}, fmt.Errorf("provider %q does not support search_depth value %q", name, req.SearchDepth)
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
		return SearchResponse{Query: query, Providers: statuses, Usage: Usage{MaxTokens: maxT}}, fmt.Errorf("all providers failed")
	}
	// Deduplicate across providers after normalization so duplicate URLs do not
	// consume the caller's max_results budget.
	all = dedupeResults(all)
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
	return SearchResponse{Query: query, Results: packed, Providers: statuses, Usage: usage}, nil
}
func uniqueNames(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, name := range in {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func dedupeResults(in []SearchResult) []SearchResult {
	out := make([]SearchResult, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, r := range in {
		if _, ok := seen[r.URL]; ok {
			continue
		}
		seen[r.URL] = struct{}{}
		out = append(out, r)
	}
	return out
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
	return HandlerWithOptions(s, apiKey, nil)
}

// HandlerWithOptions adds optional API authentication and per-client rate limiting.
func HandlerWithOptions(s *Service, apiKey string, limiter *RateLimiter) http.Handler {
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
			if strings.TrimSpace(q) == "" {
				q = values.Get("query")
			}
			maxResults, err := optionalInt(values, "max_results")
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if count, present := values["count"]; present {
				if len(count) != 1 {
					writeError(w, 400, "count must appear once")
					return
				}
				maxResults, err = parseNonNegative(count[0], "count")
				if err != nil {
					writeError(w, 400, err.Error())
					return
				}
			}
			maxTokens, err := optionalInt(values, "max_tokens")
			if err != nil {
				writeError(w, 400, err.Error())
				return
			}
			req := SearchRequest{Query: q, MaxResults: maxResults, MaxTokens: maxTokens, Freshness: values.Get("freshness"), SearchDepth: values.Get("search_depth"), Language: values.Get("language"), Region: values.Get("region"), Content: values.Get("content")}
			req.IncludeDomains = splitCSV(values.Get("include_domains"))
			req.ExcludeDomains = splitCSV(values.Get("exclude_domains"))
			if rawSafe := values.Get("safe_search"); rawSafe != "" {
				safe, ok := parseSafeSearch(rawSafe)
				if !ok {
					writeError(w, 400, "safe_search must be true or false")
					return
				}
				req.SafeSearch = &safe
			}
			if names := splitCSV(values.Get("providers")); len(names) > 0 {
				req.Providers = names
			}
			format := strings.ToLower(values.Get("format"))
			if format != "" && format != "json" && format != "text" && format != "txt" && format != "html" {
				writeError(w, http.StatusBadRequest, "format must be json, text, or html")
				return
			}
			resp, err := s.Search(r.Context(), req)
			if format != "" && err == nil {
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
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		resp, err := s.Search(r.Context(), req)
		respondSearch(w, resp, err)
	}
	protectedSearch := APIKeyAuth(RateLimit(http.HandlerFunc(searchEndpoint), limiter), apiKey)
	mux.Handle("/v1/search", protectedSearch)
	// Compatibility aliases for the former ugpt-search/SearX-style clients.
	mux.Handle("/search", protectedSearch)
	mux.Handle("/api/search", protectedSearch)
	return requestID(mux)
}

func parseSafeSearch(v string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "strict", "on":
		return true, true
	case "0", "false", "off":
		return false, true
	}
	return false, false
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
func parseNonNegative(v, name string) (int, error) {
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return n, nil
}
func optionalInt(values url.Values, name string) (int, error) {
	raw, present := values[name]
	if !present {
		return 0, nil
	}
	if len(raw) != 1 {
		return 0, fmt.Errorf("%s must appear once", name)
	}
	return parseNonNegative(raw[0], name)
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
	case "json":
		writeJSON(w, http.StatusOK, resp)
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
