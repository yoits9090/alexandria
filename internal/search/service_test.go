package search

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	name  string
	caps  Capabilities
	page  ProviderPage
	err   error
	delay time.Duration
}

func (f fakeProvider) Name() string               { return f.name }
func (f fakeProvider) Capabilities() Capabilities { return f.caps }
func (f fakeProvider) Search(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return ProviderPage{}, ctx.Err()
	}
	return f.page, f.err
}
func TestServicePartialFailureAndBudget(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", caps: Capabilities{Freshness: true}, page: ProviderPage{Results: []ProviderResult{{Title: "One", URL: "https://example.com/a?utm_source=x", Snippet: "a long useful snippet that should be compacted"}}}}, fakeProvider{name: "b", caps: Capabilities{Freshness: true}, err: errors.New("down")}}, []string{"a", "b"})
	s.MaxTokens = 100
	s.DefaultTokens = 100
	s.MaxResults = 10
	r, err := s.Search(context.Background(), SearchRequest{Query: "q", Freshness: "day", MaxTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Results) != 1 || r.Results[0].URL != "https://example.com/a" {
		t.Fatalf("unexpected results: %#v", r.Results)
	}
	if len(r.Providers) != 2 || !r.Providers[0].OK || r.Providers[1].OK {
		t.Fatalf("statuses: %#v", r.Providers)
	}
	if r.Usage.OutputTokens > 100 {
		t.Fatalf("budget exceeded: %#v", r.Usage)
	}
}
func TestServiceAllFailed(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", err: errors.New("no")}}, nil)
	r, err := s.Search(context.Background(), SearchRequest{Query: "q"})
	if err == nil || r.Providers[0].OK {
		t.Fatalf("expected all-failed: %#v %v", r, err)
	}
}
func TestServiceCapabilityValidation(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a"}}, nil)
	_, err := s.Search(context.Background(), SearchRequest{Query: "q", Freshness: "day"})
	if err == nil {
		t.Fatal("expected capability error")
	}
}
func TestHandler(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", page: ProviderPage{Results: []ProviderResult{{Title: "T", URL: "https://example.com"}}}}}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/search", strings.NewReader(`{"query":"q","max_tokens":200}`))
	Handler(s).ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-Request-ID") == "" {
		t.Fatal("request id missing")
	}
}

func TestAPIKeyAuth(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", page: ProviderPage{Results: []ProviderResult{{Title: "T", URL: "https://example.com"}}}}}, nil)
	h := HandlerWithAPIKey(s, "correct-secret")
	for _, tc := range []struct {
		name, auth, apiKey string
		code               int
	}{
		{"missing", "", "", 401}, {"wrong", "Bearer wrong", "", 401}, {"malformed", "Basic correct-secret", "", 401}, {"bearer", "bearer correct-secret", "", 200}, {"header", "", "correct-secret", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/v1/search", strings.NewReader(`{"query":"q"}`))
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			if tc.apiKey != "" {
				req.Header.Set("X-API-Key", tc.apiKey)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.code {
				t.Fatalf("got %d body=%s", rr.Code, rr.Body.String())
			}
		})
	}
	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("health should be public: %d", rr.Code)
	}
}

func TestHandlerRejectsTrailingJSON(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a"}}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/search", strings.NewReader(`{"query":"q"}{"query":"again"}`))
	Handler(s).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestRateLimit(t *testing.T) {
	limiter := NewRateLimiter(2, time.Minute)
	s := NewService([]Provider{fakeProvider{name: "a", page: ProviderPage{Results: []ProviderResult{{Title: "T", URL: "https://example.com"}}}}}, nil)
	h := HandlerWithOptions(s, "", limiter)
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/v1/search?q=q", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("request %d: %d", i, rr.Code)
		}
	}
	req := httptest.NewRequest("GET", "/v1/search?q=q", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests || rr.Header().Get("Retry-After") == "" {
		t.Fatalf("rate limit: %d %v", rr.Code, rr.Header())
	}
}

func TestRequestIDValidation(t *testing.T) {
	s := NewService(nil, nil)
	h := Handler(s)
	for _, tc := range []struct {
		in    string
		valid bool
	}{{"ok-id_1", true}, {"bad\nvalue", false}, {string(make([]byte, 129)), false}} {
		req := httptest.NewRequest("GET", "/healthz", nil)
		req.Header.Set("X-Request-ID", tc.in)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		got := rr.Header().Get("X-Request-ID")
		if (got == tc.in) != tc.valid {
			t.Fatalf("input valid=%v got %q", tc.valid, got)
		}
	}
}

func TestServiceRejectsTooManyProviders(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a"}}, nil)
	names := make([]string, 17)
	for i := range names {
		names[i] = "a" + strconv.Itoa(i)
	}
	if _, err := s.Search(context.Background(), SearchRequest{Query: "q", Providers: names}); err == nil {
		t.Fatal("expected cap")
	}
}
func TestGetFormatJSON(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", page: ProviderPage{Results: []ProviderResult{{Title: "T", URL: "https://e.test"}}}}}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/search?q=q&format=json", nil)
	Handler(s).ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"results"`) {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
}

func TestErrorResponseCarriesRequestID(t *testing.T) {
	s := NewService(nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/v1/search", strings.NewReader("{}"))
	req.Header.Set("X-Request-ID", "client-req")
	Handler(s).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), `"request_id":"client-req"`) {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
}

func TestFormatJSONCarriesRequestID(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", page: ProviderPage{Results: []ProviderResult{{Title: "T", URL: "https://e.test"}}}}}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/search?q=q&format=json", nil)
	req.Header.Set("X-Request-ID", "fmt-1")
	Handler(s).ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"request_id":"fmt-1"`) {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
}

func TestLegacyRouteFormats(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", page: ProviderPage{Results: []ProviderResult{{Title: "T", URL: "https://e.test"}}}}}, nil)
	cases := []struct {
		path, content string
		code          int
	}{{"/search?q=q", "text/toon", 200}, {"/search?q=q&format=json", "application/json", 200}, {"/api/search?q=q&format=text", "application/json", 200}}
	for _, tc := range cases {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("GET", tc.path, nil)
		Handler(s).ServeHTTP(rr, req)
		if rr.Code != tc.code || !strings.HasPrefix(rr.Header().Get("Content-Type"), tc.content) {
			t.Fatalf("%s: code=%d type=%q body=%s", tc.path, rr.Code, rr.Header().Get("Content-Type"), rr.Body.String())
		}
	}
}

func TestServicePartialTimeoutKeepsSuccess(t *testing.T) {
	s := NewService([]Provider{
		fakeProvider{name: "slow", delay: 50 * time.Millisecond, page: ProviderPage{Results: []ProviderResult{{Title: "slow", URL: "https://slow.test"}}}},
		fakeProvider{name: "fast", page: ProviderPage{Results: []ProviderResult{{Title: "fast", URL: "https://fast.test"}}}},
	}, []string{"slow", "fast"})
	s.Timeout = 5 * time.Millisecond
	r, err := s.Search(context.Background(), SearchRequest{Query: "q"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Results) != 1 || r.Results[0].Title != "fast" {
		t.Fatalf("results=%#v statuses=%#v", r.Results, r.Providers)
	}
}
func TestServiceAllTimeoutReturnsDeadline(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "slow", delay: 50 * time.Millisecond}}, nil)
	s.Timeout = 5 * time.Millisecond
	if _, err := s.Search(context.Background(), SearchRequest{Query: "q"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

func TestPostTOONAndNegotiation(t *testing.T) {
	s := NewService([]Provider{fakeProvider{name: "a", page: ProviderPage{Results: []ProviderResult{{Title: "T", URL: "https://e.test"}}}}}, nil)
	for _, tc := range []struct{ body, accept, typ string }{{`{"query":"q","format":"toon"}`, "", "text/toon"}, {`{"query":"q"}`, "application/json", "application/json"}} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/search", strings.NewReader(tc.body))
		if tc.accept != "" {
			req.Header.Set("Accept", tc.accept)
		}
		Handler(s).ServeHTTP(rr, req)
		if rr.Code != 200 || !strings.HasPrefix(rr.Header().Get("Content-Type"), tc.typ) {
			t.Fatalf("%s: %d %s", tc.body, rr.Code, rr.Body.String())
		}
	}
}
