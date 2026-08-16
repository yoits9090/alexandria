# alexandria-search

Installable wrapper for [Alexandria](https://github.com/yoits9090/alexandria), a
provider-neutral search gateway: one HTTP API for Brave, Tavily, SearXNG,
Serper, Exa, and Google CSE.

The npm package manages the daemon lifecycle (`start`/`stop`/`status`/`logs`)
and gives you a `search` command that returns LLM-ready context (TOON by
default). The Go binary is downloaded prebuilt from GitHub releases on
install, or you can build it from source.

## Install

```bash
npm install -g alexandria-search
```

This downloads the prebuilt binary for your platform from GitHub releases.
If no release exists yet (or you prefer a source build), use:

```bash
npm install -g --ignore-scripts alexandria-search
export ALEXANDRIA_SKIP_DOWNLOAD=1
# from a checkout of this repository:
npm run build:binary   # requires Go 1.23+
```

## Usage

```bash
alexandria start                       # daemonizes the gateway on :8080 (or ALEXANDRIA_ADDR)
alexandria search 'go 1.24 release notes'   # TOON output, ready for an LLM prompt
alexandria search 'latest ARM chips' --format json --max-results 5 --max-tokens 600
alexandria status                      # pid + health
alexandria logs -n 50                  # tail the daemon log
alexandria restart
alexandria stop                        # graceful SIGTERM
alexandria version
```

## Configuration

The daemon inherits your shell environment. The useful variables:

| Variable | Purpose |
| --- | --- |
| `ALEXANDRIA_ADDR` | listen address (default `:8080`) |
| `ALEXANDRIA_API_KEY` | protect `/v1/search` with a bearer / `X-API-Key` secret |
| `ALEXANDRIA_RATE_LIMIT` | requests/minute per client (default 0 = off) |
| `BRAVE_SEARCH_API_KEY`, `TAVILY_API_KEY`, `SERPER_API_KEY`, `EXA_API_KEY` | provider keys (any subset) |
| `GOOGLE_CSE_API_KEY` + `GOOGLE_CSE_ID` | Google Programmable Search |
| `SEARX_URL` | fixed SearXNG instance; default is the public online pool |
| `ALEXANDRIA_ENV_FILE` | load a `KEY=VALUE` file into the daemon environment |
| `ALEXANDRIA_RUNTIME_DIR` | pid file + log location (default `~/.alexandria`) |
| `ALEXANDRIA_BIN` | explicit path to the Go binary |
| `ALEXANDRIA_URL` | base URL when the daemon is remote |

Provider keys stay server-side; nothing is logged. See the
[repository README](https://github.com/yoits9090/alexandria) for the full API.

## API

`GET /v1/search?q=...&max_tokens=500` — TOON by default, `format=json` for JSON.
Endpoints: `/healthz`, `/readyz`, `/openapi.json`, `/v1/search`, `/search`,
`/api/search`.

## License

MIT License.
