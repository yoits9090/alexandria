package search

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const defaultSearXRegistry = "https://searx.space/data/instances.json"

type searxNode struct {
	url           string
	failures      int
	cooldownUntil time.Time
}

// SearXPool is Alexandria's public-instance SearXNG provider. It discovers
// HTTPS instances from searx.space, rotates requests across them, and applies
// per-instance cooldowns after failures. A fixed SEARX_URL remains available
// for operators who run their own instance, but the default is this pool.
type SearXPool struct {
	registryURL  string
	limit        int
	client       *http.Client
	refreshEvery time.Duration
	now          func() time.Time
	mu           sync.Mutex
	nodes        []searxNode
	next         int
	lastRefresh  time.Time
	refreshing   bool
}

func NewSearXPool(registryURL string, limit int, client *http.Client) *SearXPool {
	if registryURL == "" {
		registryURL = defaultSearXRegistry
	}
	if limit <= 0 {
		limit = 12
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &SearXPool{registryURL: registryURL, limit: limit, client: client, refreshEvery: 15 * time.Minute, now: time.Now}
}
func (p *SearXPool) Name() string { return "searx" }
func (p *SearXPool) Capabilities() Capabilities {
	return Capabilities{Freshness: true, SafeSearch: true, FreshnessValues: map[string]bool{"pd": true, "day": true, "d": true, "pw": true, "week": true, "w": true, "pm": true, "month": true, "m": true, "py": true, "year": true, "y": true}}
}
func (p *SearXPool) refresh(ctx context.Context) error {
	// Fetch a larger candidate set than the active pool size: public nodes
	// frequently challenge or rate-limit automated clients, so the first
	// sorted entries are not necessarily usable.
	candidateLimit := p.limit * 4
	if candidateLimit < 32 {
		candidateLimit = 32
	}
	urls, err := DiscoverSearX(ctx, p.registryURL, candidateLimit, p.client)
	if err != nil {
		return err
	}
	// Registry status is only a hint. Probe JSON support before putting a
	// public instance into rotation because many public nodes rate-limit or
	// expose HTML-only frontends.
	type result struct {
		url string
		ok  bool
	}
	results := make(chan result, len(urls))
	var wg sync.WaitGroup
	for _, raw := range urls {
		wg.Add(1)
		go func(raw string) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			results <- result{url: raw, ok: probeSearX(probeCtx, raw, p.client)}
		}(raw)
	}
	wg.Wait()
	close(results)
	now := p.now()
	nodes := make([]searxNode, 0, len(urls))
	for item := range results {
		if item.ok {
			nodes = append(nodes, searxNode{url: item.url})
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].url < nodes[j].url })
	// Keep all probed healthy candidates for this refresh; request rotation
	// naturally limits each search to a small number of attempts.
	p.mu.Lock()
	p.nodes = nodes
	p.lastRefresh = now
	p.next = 0
	p.mu.Unlock()
	if len(nodes) == 0 {
		return fmt.Errorf("no discovered instance supports JSON search")
	}
	return nil
}

func probeSearX(ctx context.Context, base string, client *http.Client) bool {
	u, err := url.Parse(strings.TrimRight(base, "/") + "/search")
	if err != nil {
		return false
	}
	q := u.Query()
	q.Set("q", "Alexandria online probe")
	q.Set("format", "json")
	q.Set("categories", "general")
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Alexandria/0.1 (SearXNG compatibility probe)")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	var body struct {
		Results []json.RawMessage `json:"results"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 256<<10)).Decode(&body) != nil {
		return false
	}
	return body.Results != nil
}

func (p *SearXPool) ensure(ctx context.Context) error {
	p.mu.Lock()
	stale := len(p.nodes) == 0 || p.now().Sub(p.lastRefresh) >= p.refreshEvery
	p.mu.Unlock()
	if stale {
		return p.refresh(ctx)
	}
	return nil
}
func (p *SearXPool) candidate(now time.Time) (int, searxNode, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.nodes) == 0 {
		return 0, searxNode{}, false
	}
	for i := 0; i < len(p.nodes); i++ {
		idx := (p.next + i) % len(p.nodes)
		if now.Before(p.nodes[idx].cooldownUntil) {
			continue
		}
		p.next = (idx + 1) % len(p.nodes)
		return idx, p.nodes[idx], true
	}
	return 0, searxNode{}, false
}
func (p *SearXPool) mark(idx int, ok bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if idx < 0 || idx >= len(p.nodes) {
		return
	}
	if ok {
		p.nodes[idx].failures = 0
		p.nodes[idx].cooldownUntil = time.Time{}
		return
	}
	p.nodes[idx].failures++
	backoff := time.Duration(1<<minInt(p.nodes[idx].failures, 6)) * time.Second
	if backoff > 10*time.Minute {
		backoff = 10 * time.Minute
	}
	p.nodes[idx].cooldownUntil = p.now().Add(backoff)
}
func (p *SearXPool) Search(ctx context.Context, q ProviderQuery) (ProviderPage, error) {
	if err := p.ensure(ctx); err != nil {
		return ProviderPage{}, fmt.Errorf("searx discovery: %w", err)
	}
	p.mu.Lock()
	count := len(p.nodes)
	p.mu.Unlock()
	if count == 0 {
		return ProviderPage{}, fmt.Errorf("searx discovery returned no HTTPS instances")
	}
	var last error
	for attempt := 0; attempt < count && attempt < 3; attempt++ {
		idx, node, ok := p.candidate(p.now())
		if !ok {
			break
		}
		provider := NewSearX(node.url, p.client)
		page, err := provider.Search(ctx, q)
		if err == nil {
			p.mark(idx, true)
			return page, nil
		}
		p.mark(idx, false)
		last = err
		if ctx.Err() != nil {
			return ProviderPage{}, ctx.Err()
		}
	}
	if last == nil {
		last = fmt.Errorf("all discovered instances are cooling down")
	}
	return ProviderPage{}, last
}
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
