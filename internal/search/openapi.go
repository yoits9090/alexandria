package search

import (
	"encoding/json"
	"net/http"
)

// OpenAPIHandler serves the API specification. The spec is built from Go
// values so paths, schemas, and security wiring stay in one place instead of
// a hand-maintained JSON blob.
func OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(openAPISpec())
}

func param(name, in string, required bool, schema map[string]any) map[string]any {
	return map[string]any{"name": name, "in": in, "required": required, "schema": schema}
}

func searchResponses() map[string]any {
	return map[string]any{
		"200": map[string]any{"description": "Search results (TOON by default; JSON via format or Accept)", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/SearchResponse"}}}},
		"400": map[string]any{"description": "Invalid request"},
		"401": map[string]any{"description": "Missing or invalid API key (when ALEXANDRIA_API_KEY is set)"},
		"429": map[string]any{"description": "Rate limit exceeded (when ALEXANDRIA_RATE_LIMIT is set)"},
		"502": map[string]any{"description": "All selected providers failed"},
		"504": map[string]any{"description": "Provider request timed out"},
	}
}

func searchSecurity() []any {
	return []any{map[string]any{"bearerAuth": []any{}}, map[string]any{"apiKeyHeader": []any{}}}
}

func openAPISpec() map[string]any {
	searchGet := map[string]any{
		"parameters": []any{
			param("q", "query", true, map[string]any{"type": "string"}),
			param("max_results", "query", false, map[string]any{"type": "integer", "minimum": 0}),
			param("max_tokens", "query", false, map[string]any{"type": "integer", "minimum": 0}),
			param("providers", "query", false, map[string]any{"type": "string"}),
			param("freshness", "query", false, map[string]any{"type": "string"}),
			param("search_depth", "query", false, map[string]any{"type": "string"}),
			param("language", "query", false, map[string]any{"type": "string"}),
			param("region", "query", false, map[string]any{"type": "string"}),
			param("safe_search", "query", false, map[string]any{"type": "boolean"}),
			param("include_domains", "query", false, map[string]any{"type": "string"}),
			param("exclude_domains", "query", false, map[string]any{"type": "string"}),
			param("content", "query", false, map[string]any{"type": "string", "enum": []any{"snippets"}}),
			param("format", "query", false, map[string]any{"type": "string", "enum": []any{"json", "text", "html", "toon"}}),
		},
		"responses": searchResponses(),
		"security":  searchSecurity(),
	}
	searchPost := map[string]any{
		"requestBody": map[string]any{
			"required": true,
			"content":  map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/SearchRequest"}}},
		},
		"responses": searchResponses(),
		"security":  searchSecurity(),
	}
	searchPath := map[string]any{"get": searchGet, "post": searchPost}
	probe := func(description string) map[string]any {
		return map[string]any{"get": map[string]any{"responses": map[string]any{"200": map[string]any{"description": description}}}}
	}
	return map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Alexandria Search API",
			"version":     "0.1.0",
			"description": "Provider-neutral search gateway for LLM applications. Authentication and rate limiting are optional, enabled by ALEXANDRIA_API_KEY and ALEXANDRIA_RATE_LIMIT; the documented security schemes apply only when configured.",
		},
		"paths": map[string]any{
			"/healthz":      probe("Liveness probe"),
			"/readyz":       probe("Readiness probe"),
			"/openapi.json": probe("This specification"),
			"/v1/search":    searchPath,
			"/search":       searchPath,
			"/api/search":   searchPath,
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth":   map[string]any{"type": "http", "scheme": "bearer"},
				"apiKeyHeader": map[string]any{"type": "apiKey", "in": "header", "name": "X-API-Key"},
			},
			"schemas": map[string]any{
				"SearchRequest": map[string]any{
					"type":     "object",
					"required": []any{"query"},
					"properties": map[string]any{
						"query":           map[string]any{"type": "string"},
						"providers":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"max_results":     map[string]any{"type": "integer", "minimum": 0},
						"max_tokens":      map[string]any{"type": "integer", "minimum": 0},
						"search_depth":    map[string]any{"type": "string"},
						"freshness":       map[string]any{"type": "string"},
						"include_domains": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"exclude_domains": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"language":        map[string]any{"type": "string"},
						"region":          map[string]any{"type": "string"},
						"safe_search":     map[string]any{"type": "boolean"},
						"content":         map[string]any{"type": "string", "enum": []any{"snippets"}},
						"format":          map[string]any{"type": "string", "enum": []any{"json", "text", "html", "toon"}},
					},
				},
				"SearchResponse": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query":      map[string]any{"type": "string"},
						"results":    map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/SearchResult"}},
						"providers":  map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/ProviderStatus"}},
						"usage":      map[string]any{"$ref": "#/components/schemas/Usage"},
						"request_id": map[string]any{"type": "string"},
					},
				},
				"SearchResult": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":        map[string]any{"type": "string"},
						"url":          map[string]any{"type": "string"},
						"snippet":      map[string]any{"type": "string"},
						"source":       map[string]any{"type": "string"},
						"published_at": map[string]any{"type": "string"},
						"rank":         map[string]any{"type": "integer"},
						"score":        map[string]any{"type": "number"},
					},
				},
				"ProviderStatus": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":       map[string]any{"type": "string"},
						"ok":         map[string]any{"type": "boolean"},
						"error":      map[string]any{"type": "string"},
						"results":    map[string]any{"type": "integer"},
						"latency_ms": map[string]any{"type": "integer"},
					},
				},
				"Usage": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"input_tokens":  map[string]any{"type": "integer"},
						"output_tokens": map[string]any{"type": "integer"},
						"total_tokens":  map[string]any{"type": "integer"},
						"max_tokens":    map[string]any{"type": "integer"},
						"truncated":     map[string]any{"type": "boolean"},
						"estimated":     map[string]any{"type": "boolean"},
					},
				},
			},
		},
	}
}
