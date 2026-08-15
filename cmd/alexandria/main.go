package main

import (
	"github.com/yoits9090/alexandria/internal/search"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	client := &http.Client{Timeout: 10 * time.Second}
	braveKey := os.Getenv("BRAVE_SEARCH_API_KEY")
	tavilyKey := os.Getenv("TAVILY_API_KEY")
	searxURL := os.Getenv("SEARX_URL")
	serperKey := os.Getenv("SERPER_API_KEY")
	exaKey := os.Getenv("EXA_API_KEY")
	bingKey := os.Getenv("BING_SEARCH_API_KEY")
	googleKey, googleCX := os.Getenv("GOOGLE_CSE_API_KEY"), os.Getenv("GOOGLE_CSE_ID")
	var ps []search.Provider
	defaults := []string{}
	if braveKey != "" {
		ps = append(ps, search.NewBrave(env("BRAVE_SEARCH_URL", "https://api.search.brave.com/res/v1/web/search"), braveKey, client))
		defaults = append(defaults, "brave")
	}
	if tavilyKey != "" {
		ps = append(ps, search.NewTavily(env("TAVILY_SEARCH_URL", "https://api.tavily.com/search"), tavilyKey, client))
		defaults = append(defaults, "tavily")
	}
	if searxURL != "" {
		ps = append(ps, search.NewSearX(searxURL, client))
		defaults = append(defaults, "searx")
	}
	if serperKey != "" {
		ps = append(ps, search.NewSerper(env("SERPER_SEARCH_URL", "https://google.serper.dev/search"), serperKey, client))
		defaults = append(defaults, "serper")
	}
	if exaKey != "" {
		ps = append(ps, search.NewExa(env("EXA_SEARCH_URL", "https://api.exa.ai/search"), exaKey, client))
		defaults = append(defaults, "exa")
	}
	if bingKey != "" {
		ps = append(ps, search.NewBing(env("BING_SEARCH_URL", "https://api.bing.microsoft.com/v7.0/search"), bingKey, client))
		defaults = append(defaults, "bing")
	}
	if googleKey != "" && googleCX != "" {
		ps = append(ps, search.NewGoogleCSE(env("GOOGLE_CSE_URL", "https://www.googleapis.com/customsearch/v1"), googleKey, googleCX, client))
		defaults = append(defaults, "google")
	}
	svc := search.NewService(ps, defaults)
	svc.DefaultTokens = envInt("ALEXANDRIA_DEFAULT_MAX_TOKENS", 1200)
	svc.MaxTokens = envInt("ALEXANDRIA_MAX_TOKENS", 8000)
	svc.MaxResults = envInt("ALEXANDRIA_MAX_RESULTS", 10)
	if d, e := time.ParseDuration(env("ALEXANDRIA_REQUEST_TIMEOUT", "12s")); e == nil {
		svc.Timeout = d
	}
	addr := env("ALEXANDRIA_ADDR", ":8080")
	log.Printf("alexandria listening on %s (providers=%s)", addr, strings.Join(svc.ProviderNames(), ","))
	if err := http.ListenAndServe(addr, search.Handler(svc)); err != nil {
		log.Fatal(err)
	}
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	if v, e := strconv.Atoi(os.Getenv(k)); e == nil && v > 0 {
		return v
	}
	return d
}
