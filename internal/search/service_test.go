package search

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
