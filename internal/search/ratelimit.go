package search

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	limit    int
	window   time.Duration
	maxKeys  int
	mu       sync.Mutex
	clients  map[string]rateWindow
	requests uint64
}
type rateWindow struct {
	started time.Time
	count   int
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	if window <= 0 {
		window = time.Minute
	}
	return &RateLimiter{limit: limit, window: window, maxKeys: 10000, clients: map[string]rateWindow{}}
}
func (l *RateLimiter) Allow(key string, now time.Time) (bool, int, time.Duration) {
	if l == nil || l.limit <= 0 {
		return true, -1, 0
	}
	if key == "" {
		key = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.requests++
	if l.requests%256 == 0 && len(l.clients) > l.maxKeys {
		l.pruneLocked(now)
	}
	state, ok := l.clients[key]
	if !ok || now.Sub(state.started) >= l.window || now.Before(state.started) {
		state = rateWindow{started: now}
	}
	if state.count >= l.limit {
		l.clients[key] = state
		wait := l.window - now.Sub(state.started)
		if wait < 0 {
			wait = 0
		}
		return false, 0, wait
	}
	state.count++
	l.clients[key] = state
	return true, l.limit - state.count, 0
}
func (l *RateLimiter) pruneLocked(now time.Time) {
	for key, state := range l.clients {
		if now.Sub(state.started) >= l.window || now.Before(state.started) {
			delete(l.clients, key)
		}
	}
	for len(l.clients) > l.maxKeys {
		for key := range l.clients {
			delete(l.clients, key)
			break
		}
	}
}
func RateLimit(next http.Handler, limiter *RateLimiter) http.Handler {
	if limiter == nil || limiter.limit <= 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed, remaining, retry := limiter.Allow(clientIP(r), time.Now())
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limiter.limit))
		if remaining >= 0 {
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		}
		if !allowed {
			seconds := int((retry + time.Second - 1) / time.Second)
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}
