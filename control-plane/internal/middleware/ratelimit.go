// Package middleware — rate limiting.
// Implements per-IP and per-user rate limiting using a token bucket algorithm.
// Critical for preventing brute force attacks on auth endpoints.
package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter holds per-key rate limiters.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
	rate     rate.Limit
	burst    int
	ttl      time.Duration
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a rate limiter.
// r = requests per second, burst = max burst size, ttl = cleanup interval for idle keys.
func NewRateLimiter(r rate.Limit, burst int, ttl time.Duration) *RateLimiter {
	rl := &RateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		rate:     r,
		burst:    burst,
		ttl:      ttl,
	}
	// Background cleanup of stale limiters
	go rl.cleanup()
	return rl
}

// Allow returns true if the key is within rate limits.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	entry, exists := rl.limiters[key]
	if !exists {
		entry = &rateLimiterEntry{
			limiter: rate.NewLimiter(rl.rate, rl.burst),
		}
		rl.limiters[key] = entry
	}
	entry.lastSeen = time.Now()
	return entry.limiter.Allow()
}

// cleanup removes stale rate limiter entries periodically.
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.ttl)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for key, entry := range rl.limiters {
			if time.Since(entry.lastSeen) > rl.ttl {
				delete(rl.limiters, key)
			}
		}
		rl.mu.Unlock()
	}
}

// ── Pre-configured limiters ───────────────────────────────────────────────────

// LoginRateLimiter is a strict limiter for authentication endpoints.
// 10 attempts per 10 minutes per IP.
func LoginRateLimiter() *RateLimiter {
	// 10 requests per 600 seconds = 1 per 60 seconds, burst of 10
	return NewRateLimiter(rate.Every(60*time.Second), 10, 15*time.Minute)
}

// APIRateLimiter is a general limiter for authenticated API endpoints.
// 1000 requests per minute per token.
func APIRateLimiter() *RateLimiter {
	return NewRateLimiter(rate.Every(60*time.Millisecond), 100, 5*time.Minute)
}

// ── Middleware ────────────────────────────────────────────────────────────────

// LimitByIP returns middleware that rate-limits by client IP address.
func LimitByIP(rl *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractClientIP(r)
			if !rl.Allow(ip) {
				w.Header().Set("Retry-After", "60")
				http.Error(w,
					`{"error":"rate limit exceeded, please try again later"}`,
					http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
