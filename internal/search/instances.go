package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DiscoverSearX fetches the public searx.space registry and returns only
// instances explicitly marked as online. Consumers should still probe and
// rate-limit these endpoints; public instances are best-effort fallbacks.
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
			Status string `json:"status"`
			IsTor  bool   `json:"is_tor"`
		} `json:"instances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := []string{}
	for host, v := range raw.Instances {
		if v.Status != "online" || v.IsTor {
			continue
		}
		host = strings.TrimRight(host, "/")
		if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
			host = "https://" + host
		}
		out = append(out, host)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}
