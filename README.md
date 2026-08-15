# Alexandria

Alexandria is a Go search gateway for LLM applications—the renewal of `ugpt-search` with a provider-neutral API, broad search-provider compatibility, and an explicit token budget. It returns compact, normalized snippets rather than forcing every agent to pay for raw provider payloads.

> Early MVP: Brave Web Search, Tavily, SearXNG, Serper, Exa, and Google Custom Search adapters are included behind one provider interface. The Bing adapter is retained as a legacy compatibility path because the public Bing Search APIs were retired in 2025.

## Why Alexandria

- **LLM-first output:** `max_tokens` is a hard budget for the serialized search context. Results are normalized, URL-deduplicated, tracking parameters removed, and snippets packed greedily at word boundaries.
- **Provider-neutral:** one canonical endpoint, predictable fields, and partial-failure statuses. API keys remain server-side.
- **Compatible by design:** adapter contract maps common provider features (`freshness`, domains, language, region, safe search, depth) without leaking provider-specific payloads.
- **Sleep-safe development:** the repo is self-contained, has no runtime dependency downloads, and includes a Colab workflow for remote smoke tests.

## Quick start

Requires Go 1.23+.

```bash
cp .env.example .env
# set BRAVE_SEARCH_API_KEY and/or TAVILY_API_KEY in your shell (never commit .env)
go test ./...
go run ./cmd/alexandria
```

```bash
curl -sS -X POST http://localhost:8080/v1/search \
  -H 'content-type: application/json' \
  -d '{"query":"Go context cancellation","max_results":5,"max_tokens":500}' | jq
```

Endpoints: `GET /healthz`, `GET /readyz`, `GET /openapi.json`, `GET /v1/search?q=...`, `POST /v1/search`. Compatibility aliases preserve the original split: `/search` defaults to HTML and accepts `json`, `text`, or `html`; `/api/search` is always JSON. `/v1/search` defaults to JSON.

## Request and response

```json
{"query":"latest Go release","max_results":5,"max_tokens":600,"providers":["brave"],"freshness":"pw"}
```

The response includes `results`, per-provider `ok/error/results/latency_ms`, and `usage` (`output_tokens`, `max_tokens`, `truncated`, `estimated`). Token use is estimated with a dependency-free rune/4 fallback; a model-specific tokenizer can be plugged into the service later. The budget includes the result JSON envelope and preserves title/URL before truncating snippets.

A provider failure does not discard successful results; HTTP 502 is returned only when all selected providers fail.

## Provider setup

- Brave: `BRAVE_SEARCH_API_KEY` and optional `BRAVE_SEARCH_URL`.
- Tavily: `TAVILY_API_KEY` and optional `TAVILY_SEARCH_URL`.
- SearXNG: `SEARX_URL` pointing at a trusted instance (for example `https://search.example/search` or its base URL).
- Serper, Exa, and Google CSE: set the corresponding API key variables in `.env.example`; Google also needs `GOOGLE_CSE_ID`. Bing is legacy-only and should not be selected for new deployments.

Set `ALEXANDRIA_API_KEY` to protect `/v1/search`, `/search`, and `/api/search`. Clients may send `Authorization: Bearer <key>` or `X-API-Key: <key>`; `/healthz`, `/readyz`, and `/openapi.json` stay public for probes. Set `ALEXANDRIA_RATE_LIMIT` to a positive requests-per-minute value for an in-process per-client limit; `0` disables it. The limiter keys by direct peer IP and is process-local, so configure shared edge controls for multi-instance deployments. The server never logs keys. For production, put it behind TLS and use a secrets manager. Provider costs and LLM token costs are intentionally separate; future policy can select providers by latency, freshness, and dollar budget.

## LLM smoke harness

`tools/jcode.py` is a tiny OpenAI-compatible harness. It searches Alexandria, then optionally asks a DeepSeek-compatible endpoint to answer only from returned sources. It defaults to `DEEPSEEK_MODEL=deepseek-v4-flash`; set your account's model name explicitly if your endpoint exposes a different “V4 Flash” identifier. **Never put an API key in source, git, or Colab command history.**

```bash
export DEEPSEEK_API_KEY='...'
python3 tools/jcode.py 'What changed in Go recently?' --search-url http://localhost:8080/v1/search
```

## Container and CI

A minimal non-root distroless image is provided:

```bash
docker build -t alexandria .
docker run --rm -p 8080:8080 --env-file .env alexandria
```

GitHub Actions runs Go race tests, vet, builds, and Python harness syntax checks on pushes and pull requests.

## Colab

A persistent CPU Colab node named `alexandria` can be recreated or continued with the CLI. It is useful for sleep-safe remote smoke tests, but the node is billable while alive; stop it when finished:

```bash
colab --auth=adc new -s alexandria
colab --auth=adc upload tools/colab_smoke.py /content/colab_smoke.py
colab --auth=adc exec -s alexandria -f tools/colab_smoke.py
colab --auth=adc stop -s alexandria
```

For a one-shot job that always cleans up, use `colab --auth=adc run tools/colab_smoke.py`.

## Roadmap

1. Add provider circuit breakers and caching.
2. Add optional page extraction with a separate budget and explicit opt-in; snippets remain the cheap default.
3. Add exact tokenizer plugins and model-specific serialization budgets.
4. Add evaluation fixtures measuring answer quality per token and provider cost/latency.
5. Harden dynamic SearXNG discovery with probing, health scoring, and per-instance cooldowns.

## License

GNU General Public License v3.0 or later (GPL-3.0-or-later), retaining the original UGPTSearch project's copyleft direction. This is a new implementation; no code or secrets were copied from the prior repository.
