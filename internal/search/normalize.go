package search

import (
	"net/url"
	"strings"
	"unicode"
)

func cleanText(s string) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func canonicalURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || (strings.ToLower(u.Scheme) != "http" && strings.ToLower(u.Scheme) != "https") {
		return ""
	}
	u.Fragment = ""
	q := u.Query()
	for key := range q {
		lk := strings.ToLower(key)
		if strings.HasPrefix(lk, "utm_") || lk == "gclid" || lk == "fbclid" || lk == "msclkid" {
			q.Del(key)
		}
	}
	u.RawQuery = q.Encode()
	u.Host = strings.ToLower(u.Host)
	return u.String()
}

func normalize(in []ProviderResult, source string) []SearchResult {
	out := make([]SearchResult, 0, len(in))
	seen := map[string]bool{}
	for _, r := range in {
		r.URL = canonicalURL(r.URL)
		if r.URL == "" || seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		title := cleanText(r.Title)
		snippet := cleanText(r.Snippet)
		if title == "" {
			title = r.URL
		}
		out = append(out, SearchResult{Title: title, URL: r.URL, Snippet: snippet, Source: source, PublishedAt: cleanText(r.PublishedAt), Score: r.Score})
	}
	return out
}
