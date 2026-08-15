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
	"time"
)

// DiscoverSearX fetches the public searx.space registry and returns HTTPS
// instances that the registry currently reports as reachable. Discovery is
// deliberately opt-in: public instances are untrusted, best-effort fallbacks
// and should still be probed, rate-limited, and monitored by the caller.
func DiscoverSearX(ctx context.Context, registryURL string, limit int, client *http.Client) ([]string, error) {
	if registryURL == "" {
		registryURL = "https://searx.space/data/instances.json"
	}
	if client == nil {
		client = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, registryURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("instance registry returned %d", resp.StatusCode)
	}
	var raw struct {
		Instances map[string]struct {
			// Older registry fixtures used status/is_tor. The live registry
			// uses http.status_code and network_type, so accept both forms.
			Status      string `json:"status"`
			IsTor       bool   `json:"is_tor"`
			NetworkType string `json:"network_type"`
			HTTP        struct {
				StatusCode int `json:"status_code"`
			} `json:"http"`
		} `json:"instances"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	hosts := make([]string, 0, len(raw.Instances))
	for host, v := range raw.Instances {
		if v.IsTor || strings.EqualFold(v.NetworkType, "tor") {
			continue
		}
		// Preserve compatibility with small fixtures and older registries,
		// while requiring a successful HTTP probe for the current registry.
		if v.Status != "" && v.Status != "online" {
			continue
		}
		if v.Status == "" && v.HTTP.StatusCode != 0 && v.HTTP.StatusCode != http.StatusOK {
			continue
		}
		host = strings.TrimRight(host, "/")
		u, err := url.Parse(host)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			continue
		}
		hosts = append(hosts, u.String())
	}
	sort.Strings(hosts)
	if limit > 0 && len(hosts) > limit {
		hosts = hosts[:limit]
	}
	return hosts, nil
}
