package search

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// APIKeyAuth protects search endpoints when configuredKey is non-empty. It is
// intentionally disabled for local development when no key is configured.
// Health, readiness, and OpenAPI remain public so deploy systems can probe the
// service without holding an application credential.
func APIKeyAuth(next http.Handler, configuredKey string) http.Handler {
	configuredKey = strings.TrimSpace(configuredKey)
	if configuredKey == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if constantTimeTokenMatch(requestToken(r), configuredKey) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="alexandria"`)
		writeError(w, http.StatusUnauthorized, "authentication required")
	})
}

func requestToken(r *http.Request) string {
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); auth != "" {
		parts := strings.Fields(auth)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return parts[1]
		}
		return ""
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}

func constantTimeTokenMatch(candidate, configured string) bool {
	// Hash first so the constant-time comparison has a fixed length even when
	// a malformed or attacker-controlled candidate has a different length.
	candidateHash := sha256.Sum256([]byte(candidate))
	configuredHash := sha256.Sum256([]byte(configured))
	return subtle.ConstantTimeCompare(candidateHash[:], configuredHash[:]) == 1
}
