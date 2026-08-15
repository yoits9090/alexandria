package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTPProvider struct {
	name, baseURL, key, cx string
	client                 *http.Client
}

func NewBrave(baseURL, key string, client *http.Client) *HTTPProvider {
	return newHTTPProvider("brave", baseURL, key, client)
}
func NewTavily(baseURL, key string, client *http.Client) *HTTPProvider {
	return newHTTPProvider("tavily", baseURL, key, client)
}
func NewSearX(baseURL string, client *http.Client) *HTTPProvider {
	return newHTTPProvider("searx", strings.TrimRight(baseURL, "/"), "", client)
}
func NewSerper(baseURL, key string, client *http.Client) *HTTPProvider {
	return newHTTPProvider("serper", baseURL, key, client)
}
func NewExa(baseURL, key string, client *http.Client) *HTTPProvider {
	return newHTTPProvider("exa", baseURL, key, client)
}
func NewBing(baseURL, key string, client *http.Client) *HTTPProvider {
	return newHTTPProvider("bing", baseURL, key, client)
}
func NewGoogleCSE(baseURL, key, cx string, client *http.Client) *HTTPProvider {
	p := newHTTPProvider("google", baseURL, key, client)
	p.cx = cx
	return p
}
func newHTTPProvider(name, baseURL, key string, client *http.Client) *HTTPProvider {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	// Do not inherit a caller's permissive redirect policy: provider requests
	// can carry credentials in headers or (Google CSE) the query string.
	copy := *client
	copy.CheckRedirect = rejectCrossOriginRedirect
	return &HTTPProvider{name: name, baseURL: baseURL, key: key, client: &copy}
}

func rejectCrossOriginRedirect(req *http.Request, via []*http.Request) error {
	// Provider URLs can contain credentials (Google CSE) or carry them in
	// headers. Do not follow any redirect; callers receive the redirect status
	// as a provider failure rather than forwarding secrets elsewhere.
	return http.ErrUseLastResponse
}

func (p *HTTPProvider) Name() string { return p.name }
func (p *HTTPProvider) Capabilities() Capabilities {
	caps := Capabilities{Freshness: p.name != "exa", Domains: p.name == "tavily" || p.name == "exa", Language: p.name != "tavily" && p.name != "exa", Region: p.name != "searx", SafeSearch: p.name == "brave" || p.name == "tavily" || p.name == "bing" || p.name == "google" || p.name == "searx", SearchDepth: p.name == "tavily" || p.name == "exa"}
	if p.name == "searx" {
		caps.FreshnessValues = map[string]bool{"pd": true, "day": true, "d": true, "pm": true, "month": true, "m": true, "py": true, "year": true, "y": true}
	}
	if p.name == "tavily" {
		caps.SearchDepthValues = map[string]bool{"basic": true, "advanced": true, "fast": true, "deep": true, "deep-reasoning": true}
	}
	if p.name == "exa" {
		caps.SearchDepthValues = map[string]bool{"auto": true, "fast": true, "instant": true, "deep-lite": true, "deep": true, "deep-reasoning": true}
	}
	return caps
}
func (p *HTTPProvider) Search(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	if p.name != "searx" && p.key == "" {
		return ProviderPage{}, fmt.Errorf("%s is not configured", p.name)
	}
	switch p.name {
	case "brave":
		return p.brave(ctx, q)
	case "tavily":
		return p.tavily(ctx, q)
	case "searx":
		return p.searx(ctx, q)
	case "serper":
		return p.serper(ctx, q)
	case "exa":
		return p.exa(ctx, q)
	case "bing":
		return p.bing(ctx, q)
	case "google":
		return p.google(ctx, q)
	default:
		return ProviderPage{}, fmt.Errorf("unsupported provider %q", p.name)
	}
}
func (p *HTTPProvider) brave(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return ProviderPage{}, err
	}
	v := u.Query()
	v.Set("q", q.Query)
	v.Set("count", strconv.Itoa(clampCount(q.Count, 20)))
	if freshness := braveFreshness(q.Freshness); freshness != "" {
		v.Set("freshness", freshness)
	}
	if q.Language != "" {
		v.Set("search_lang", q.Language)
	}
	if q.Region != "" {
		v.Set("country", q.Region)
	}
	if q.SafeSearch != nil {
		if *q.SafeSearch {
			v.Set("safesearch", "strict")
		} else {
			v.Set("safesearch", "off")
		}
	}
	u.RawQuery = v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ProviderPage{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", p.key)
	var body braveResponse
	if err := p.do(req, &body); err != nil {
		return ProviderPage{}, err
	}
	out := ProviderPage{}
	for _, r := range body.Web.Results {
		out.Results = append(out.Results, ProviderResult{Title: r.Title, URL: r.URL, Snippet: r.Description, PublishedAt: r.Age})
	}
	return out, nil
}

type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Age         string `json:"age"`
		} `json:"results"`
	} `json:"web"`
}

func (p *HTTPProvider) tavily(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	depth := tavilyDepth(q.SearchDepth)
	if depth == "" {
		depth = "basic"
	}
	payload := map[string]any{"query": q.Query, "max_results": clampCount(q.Count, 20), "include_answer": false, "search_depth": depth}
	if freshness := tavilyFreshness(q.Freshness); freshness != "" {
		payload["time_range"] = freshness
	}
	if q.Region != "" {
		payload["country"] = q.Region
	}
	if q.SafeSearch != nil {
		payload["safe_search"] = *q.SafeSearch
	}
	if len(q.IncludeDomains) > 0 {
		payload["include_domains"] = q.IncludeDomains
	}
	if len(q.ExcludeDomains) > 0 {
		payload["exclude_domains"] = q.ExcludeDomains
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(b))
	if err != nil {
		return ProviderPage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.key)
	var body tavilyResponse
	if err := p.do(req, &body); err != nil {
		return ProviderPage{}, err
	}
	out := ProviderPage{}
	for _, r := range body.Results {
		out.Results = append(out.Results, ProviderResult{Title: r.Title, URL: r.URL, Snippet: r.Content, PublishedAt: r.PublishedDate, Score: r.Score})
	}
	return out, nil
}

type tavilyResponse struct {
	Results []struct {
		Title         string  `json:"title"`
		URL           string  `json:"url"`
		Content       string  `json:"content"`
		PublishedDate string  `json:"published_date"`
		Score         float64 `json:"score"`
	} `json:"results"`
}

func (p *HTTPProvider) searx(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return ProviderPage{}, err
	}
	if !strings.HasSuffix(u.Path, "/search") {
		u.Path = strings.TrimRight(u.Path, "/") + "/search"
	}
	v := u.Query()
	v.Set("q", q.Query)
	v.Set("format", "json")
	v.Set("number_of_results", strconv.Itoa(q.Count))
	if q.Language != "" {
		v.Set("language", q.Language)
	}
	if freshness := searxFreshness(q.Freshness); freshness != "" {
		v.Set("time_range", freshness)
	}
	if q.SafeSearch != nil {
		if *q.SafeSearch {
			v.Set("safesearch", "2")
		} else {
			v.Set("safesearch", "0")
		}
	}
	u.RawQuery = v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ProviderPage{}, err
	}
	req.Header.Set("Accept", "application/json")
	var body searxResponse
	if err := p.do(req, &body); err != nil {
		return ProviderPage{}, err
	}
	out := ProviderPage{}
	for _, r := range body.Results {
		out.Results = append(out.Results, ProviderResult{Title: r.Title, URL: r.URL, Snippet: r.Content, PublishedAt: r.PublishedDate, Score: r.Score})
	}
	return out, nil
}

type searxResponse struct {
	Results []struct {
		Title         string  `json:"title"`
		URL           string  `json:"url"`
		Content       string  `json:"content"`
		PublishedDate string  `json:"publishedDate"`
		Score         float64 `json:"score"`
	} `json:"results"`
}

func (p *HTTPProvider) serper(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	payload := map[string]any{"q": q.Query, "num": clampCount(q.Count, 100)}
	if q.Language != "" {
		payload["hl"] = q.Language
	}
	if q.Region != "" {
		payload["gl"] = q.Region
	}
	if v := googleFreshness(q.Freshness); v != "" {
		payload["tbs"] = v
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(b))
	if err != nil {
		return ProviderPage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", p.key)
	var body serperResponse
	if err := p.do(req, &body); err != nil {
		return ProviderPage{}, err
	}
	out := ProviderPage{}
	for _, r := range body.Organic {
		out.Results = append(out.Results, ProviderResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet, PublishedAt: r.Date})
	}
	return out, nil
}

type serperResponse struct {
	Organic []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
		Date    string `json:"date"`
	} `json:"organic"`
}

func (p *HTTPProvider) exa(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	payload := map[string]any{"query": q.Query, "numResults": clampCount(q.Count, 100), "contents": map[string]any{"highlights": map[string]any{"maxCharacters": 600}}}
	if q.SearchDepth != "" {
		if depth := exaDepth(q.SearchDepth); depth != "" {
			payload["type"] = depth
		}
	}
	if q.Region != "" {
		payload["userLocation"] = q.Region
	}
	if len(q.IncludeDomains) > 0 {
		payload["includeDomains"] = q.IncludeDomains
	}
	if len(q.ExcludeDomains) > 0 {
		payload["excludeDomains"] = q.ExcludeDomains
	}
	b, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL, bytes.NewReader(b))
	if err != nil {
		return ProviderPage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.key)
	var body exaResponse
	if err := p.do(req, &body); err != nil {
		return ProviderPage{}, err
	}
	out := ProviderPage{}
	for _, r := range body.Results {
		snippet := r.Text
		if snippet == "" && len(r.Highlights) > 0 {
			snippet = strings.Join(r.Highlights, " ")
		}
		out.Results = append(out.Results, ProviderResult{Title: r.Title, URL: r.URL, Snippet: snippet, PublishedAt: r.PublishedDate})
	}
	return out, nil
}

type exaResponse struct {
	Results []struct {
		Title         string   `json:"title"`
		URL           string   `json:"url"`
		Text          string   `json:"text"`
		Highlights    []string `json:"highlights"`
		PublishedDate string   `json:"publishedDate"`
	} `json:"results"`
}

func (p *HTTPProvider) bing(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return ProviderPage{}, err
	}
	v := u.Query()
	v.Set("q", q.Query)
	v.Set("count", strconv.Itoa(clampCount(q.Count, 50)))
	if market := bingMarket(q.Language, q.Region); market != "" {
		v.Set("mkt", market)
	} else if q.Region != "" {
		v.Set("cc", q.Region)
	}
	if freshness := bingFreshness(q.Freshness); freshness != "" {
		v.Set("freshness", freshness)
	}
	if q.SafeSearch != nil {
		if *q.SafeSearch {
			v.Set("safeSearch", "Strict")
		} else {
			v.Set("safeSearch", "Off")
		}
	}
	u.RawQuery = v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ProviderPage{}, err
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", p.key)
	req.Header.Set("Accept", "application/json")
	var body bingResponse
	if err := p.do(req, &body); err != nil {
		return ProviderPage{}, err
	}
	out := ProviderPage{}
	for _, r := range body.WebPages.Value {
		out.Results = append(out.Results, ProviderResult{Title: r.Name, URL: r.URL, Snippet: r.Snippet, PublishedAt: r.DateLastCrawled})
	}
	return out, nil
}

type bingResponse struct {
	WebPages struct {
		Value []struct {
			Name            string `json:"name"`
			URL             string `json:"url"`
			Snippet         string `json:"snippet"`
			DateLastCrawled string `json:"dateLastCrawled"`
		} `json:"value"`
	} `json:"webPages"`
}

func (p *HTTPProvider) google(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	u, err := url.Parse(p.baseURL)
	if err != nil {
		return ProviderPage{}, err
	}
	v := u.Query()
	v.Set("key", p.key)
	v.Set("cx", p.cx)
	v.Set("q", q.Query)
	v.Set("num", strconv.Itoa(min(q.Count, 10)))
	if q.Language != "" {
		v.Set("hl", q.Language)
	}
	if q.Region != "" {
		v.Set("gl", q.Region)
	}
	if q.SafeSearch != nil {
		v.Set("safe", map[bool]string{true: "active", false: "off"}[*q.SafeSearch])
	}
	if freshness := googleDateRestrict(q.Freshness); freshness != "" {
		v.Set("dateRestrict", freshness)
	}
	u.RawQuery = v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ProviderPage{}, err
	}
	req.Header.Set("Accept", "application/json")
	var body googleResponse
	if err := p.do(req, &body); err != nil {
		return ProviderPage{}, err
	}
	out := ProviderPage{}
	for _, r := range body.Items {
		out.Results = append(out.Results, ProviderResult{Title: r.Title, URL: r.Link, Snippet: r.Snippet})
	}
	return out, nil
}

type googleResponse struct {
	Items []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Snippet string `json:"snippet"`
	} `json:"items"`
}

func braveFreshness(v string) string {
	switch strings.ToLower(v) {
	case "pd", "day", "d":
		return "pd"
	case "pw", "week", "w":
		return "pw"
	case "pm", "month", "m":
		return "pm"
	case "py", "year", "y":
		return "py"
	}
	return ""
}

func tavilyDepth(v string) string {
	switch strings.ToLower(v) {
	case "basic", "advanced", "fast":
		return strings.ToLower(v)
	case "deep", "deep-reasoning":
		return "advanced"
	}
	return ""
}
func searxFreshness(v string) string {
	switch strings.ToLower(v) {
	case "pd", "day", "d":
		return "day"
	case "pm", "month", "m":
		return "month"
	case "py", "year", "y":
		return "year"
	}
	return ""
}

func exaDepth(v string) string {
	switch strings.ToLower(v) {
	case "auto", "fast", "instant", "deep-lite", "deep", "deep-reasoning":
		return strings.ToLower(v)
	}
	return ""
}

func clampCount(n, max int) int {
	if n < 1 {
		return 1
	}
	if max > 0 && n > max {
		return max
	}
	return n
}
func tavilyFreshness(v string) string {
	switch strings.ToLower(v) {
	case "pd", "day", "d":
		return "day"
	case "pw", "week", "w":
		return "week"
	case "pm", "month", "m":
		return "month"
	case "py", "year", "y":
		return "year"
	}
	return ""
}
func googleDateRestrict(v string) string {
	switch strings.ToLower(v) {
	case "pd", "day", "d":
		return "d1"
	case "pw", "week", "w":
		return "w1"
	case "pm", "month", "m":
		return "m1"
	case "py", "year", "y":
		return "y1"
	}
	return ""
}

func googleFreshness(v string) string {
	switch strings.ToLower(v) {
	case "pd", "day", "d":
		return "qdr:d"
	case "pw", "week", "w":
		return "qdr:w"
	case "pm", "month", "m":
		return "qdr:m"
	case "py", "year", "y":
		return "qdr:y"
	}
	return ""
}
func bingMarket(language, region string) string {
	language = strings.TrimSpace(language)
	region = strings.TrimSpace(region)
	if language == "" {
		return ""
	}
	if strings.Contains(language, "-") {
		return language
	}
	if region != "" {
		return language + "-" + region
	}
	return ""
}

func bingFreshness(v string) string {
	switch strings.ToLower(v) {
	case "pd", "day", "d":
		return "Day"
	case "pw", "week", "w":
		return "Week"
	case "pm", "month", "m":
		return "Month"
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p *HTTPProvider) do(req *http.Request, dst any) error {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		attemptReq := req.Clone(req.Context())
		if req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return fmt.Errorf("%s request body unavailable", p.name)
			}
			attemptReq.Body = body
		} else if req.Body != nil {
			if attempt > 0 {
				return fmt.Errorf("%s request body unavailable", p.name)
			}
			attemptReq.Body = req.Body
		}
		resp, err := p.client.Do(attemptReq)
		if err != nil {
			last = fmt.Errorf("%s request failed", p.name)
			if req.Context().Err() != nil {
				return req.Context().Err()
			}
			break
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if readErr != nil {
			last = readErr
			break
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if err := json.Unmarshal(body, dst); err != nil {
				return fmt.Errorf("%s decode: %w", p.name, err)
			}
			return nil
		}
		// Do not expose provider response bodies: they may contain echoed
		// credentials, query text, or infrastructure details.
		last = fmt.Errorf("%s returned HTTP %d", p.name, resp.StatusCode)
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			break
		}
		if attempt < 2 {
			select {
			case <-time.After(time.Duration(50*(attempt+1)) * time.Millisecond):
			case <-req.Context().Done():
				return req.Context().Err()
			}
		}
	}
	return last
}
