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
	return &HTTPProvider{name: name, baseURL: baseURL, key: key, client: client}
}
func (p *HTTPProvider) Name() string { return p.name }
func (p *HTTPProvider) Capabilities() Capabilities {
	return Capabilities{Freshness: p.name != "exa" && p.name != "google", Domains: p.name == "tavily" || p.name == "exa", Language: true, Region: p.name != "tavily" && p.name != "exa", SafeSearch: p.name == "brave" || p.name == "bing" || p.name == "google" || p.name == "searx", SearchDepth: p.name == "tavily" || p.name == "exa"}
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
	v.Set("count", strconv.Itoa(q.Count))
	if q.Freshness != "" {
		v.Set("freshness", q.Freshness)
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
	depth := q.SearchDepth
	if depth == "" {
		depth = "basic"
	}
	payload := map[string]any{"api_key": p.key, "query": q.Query, "max_results": q.Count, "include_answer": false, "search_depth": depth}
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
	if q.Freshness != "" {
		v.Set("time_range", q.Freshness)
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
	payload := map[string]any{"q": q.Query, "num": q.Count}
	if q.Language != "" {
		payload["hl"] = q.Language
	}
	if q.Region != "" {
		payload["gl"] = q.Region
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
	payload := map[string]any{"query": q.Query, "numResults": q.Count, "contents": map[string]any{"highlights": map[string]any{"maxCharacters": 600}}}
	if q.SearchDepth == "deep" {
		payload["type"] = "deep"
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
	v.Set("count", strconv.Itoa(q.Count))
	if q.Language != "" {
		v.Set("mkt", q.Language)
	}
	if q.Freshness != "" {
		v.Set("freshness", q.Freshness)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (p *HTTPProvider) do(req *http.Request, dst any) error {
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		resp, err := p.client.Do(req)
		if err != nil {
			last = err
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
		last = fmt.Errorf("%s returned %d: %s", p.name, resp.StatusCode, bytes.TrimSpace(body))
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
