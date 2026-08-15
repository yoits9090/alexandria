package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	if r.Body == nil {
		return fmt.Errorf("request body is required")
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("invalid JSON: multiple values")
		}
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": message, "request_id": w.Header().Get("X-Request-ID")})
}
func respondSearch(w http.ResponseWriter, resp SearchResponse, err error) {
	if resp.RequestID == "" {
		resp.RequestID = w.Header().Get("X-Request-ID")
	}
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		message := err.Error()
		if strings.HasPrefix(message, "query is required") || strings.HasPrefix(message, "max_results") || strings.HasPrefix(message, "max_tokens") || strings.HasPrefix(message, "unknown provider") || strings.HasPrefix(message, "no search providers") || strings.HasPrefix(message, "too many providers") || strings.HasPrefix(message, "content must") || strings.HasPrefix(message, "format must") || strings.HasPrefix(message, "freshness must") || strings.HasPrefix(message, "search_depth") || strings.HasPrefix(message, "content must") || strings.Contains(message, "does not support") {
			status = http.StatusBadRequest
		}
		if resp.Query != "" {
			resp.Usage.Truncated = true
		}
		if status == http.StatusBadRequest {
			writeError(w, status, err.Error())
		} else {
			writeJSON(w, status, resp)
		}
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

type requestIDContextKey struct{}

func requestIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDContextKey{}).(string); ok {
		return v
	}
	return ""
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !validRequestID(id) {
			id = strconv.FormatInt(time.Now().UnixNano(), 10)
		}
		w.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, id))
		next.ServeHTTP(w, r)
	})
}
func validRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}
