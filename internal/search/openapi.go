package search

import "net/http"

func OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"openapi":"3.0.3","info":{"title":"Alexandria Search API","version":"0.1.0"},"paths":{"/v1/search":{"get":{"parameters":[{"name":"q","in":"query","required":true,"schema":{"type":"string"}}],"responses":{"200":{"description":"Search response"}}},"post":{"requestBody":{"required":true,"content":{"application/json":{"schema":{"$ref":"#/components/schemas/SearchRequest"}}}},"responses":{"200":{"description":"Search response"}}}}},"components":{"schemas":{"SearchRequest":{"type":"object","required":["query"],"properties":{"query":{"type":"string"},"providers":{"type":"array","items":{"type":"string"}},"max_results":{"type":"integer","minimum":1},"max_tokens":{"type":"integer","minimum":1},"freshness":{"type":"string"},"include_domains":{"type":"array","items":{"type":"string"}},"exclude_domains":{"type":"array","items":{"type":"string"}},"language":{"type":"string"},"region":{"type":"string"},"safe_search":{"type":"boolean"}}}}}}`))
}
