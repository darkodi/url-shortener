package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/darkodi/url-shortener/internal/logger"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	mu       sync.RWMutex
	clients  map[string]*client
	rate     int           // tokens added per interval
	burst    int           // max tokens (bucket size)
	interval time.Duration // how often to add tokens
	cleanup  time.Duration // cleanup old entries
	log      *logger.Logger
}

type client struct {
	tokens    int
	lastCheck time.Time
}

// RateLimiterConfig holds rate limiter settings
type RateLimiterConfig struct {
	Rate     int           // Requests per interval
	Burst    int           // Max burst size
	Interval time.Duration // Token refill interval
	Cleanup  time.Duration // Cleanup interval for old clients
}

// DefaultRateLimiterConfig returns sensible defaults
func DefaultRateLimiterConfig() RateLimiterConfig {
	return RateLimiterConfig{
		Rate:     10,              // 10 requests
		Burst:    20,              // burst up to 20
		Interval: time.Second,    // per second
		Cleanup:  5 * time.Minute, // cleanup every 5 min
	}
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg RateLimiterConfig, log *logger.Logger) *RateLimiter {
	rl := &RateLimiter{
		clients:  make(map[string]*client),
		rate:     cfg.Rate,
		burst:    cfg.Burst,
		interval: cfg.Interval,
		cleanup:  cfg.Cleanup,
		log:      log,
	}

	// Start cleanup goroutine
	go rl.cleanupLoop()

	return rl
}

// Allow checks if a request from the given IP is allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	c, exists := rl.clients[ip]
	if !exists {
		// New client gets full bucket
		rl.clients[ip] = &client{
			tokens:    rl.burst - 1, // -1 for current request
			lastCheck: now,
		}
		return true
	}

	// Calculate tokens to add based on time elapsed
	elapsed := now.Sub(c.lastCheck)
	tokensToAdd := int(elapsed / rl.interval) * rl.rate

	if tokensToAdd > 0 {
		c.tokens = min(c.tokens+tokensToAdd, rl.burst)
		c.lastCheck = now
	}

	// Check if request is allowed
	if c.tokens > 0 {
		c.tokens--
		return true
	}

	return false
}

// cleanupLoop removes old client entries periodically
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanup)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		cutoff := time.Now().Add(-rl.cleanup)
		for ip, c := range rl.clients {
			if c.lastCheck.Before(cutoff) {
				delete(rl.clients, ip)
			}
		}
		count := len(rl.clients)
		rl.mu.Unlock()

		if rl.log != nil {
			rl.log.Debug("rate limiter cleanup", "active_clients", count)
		}
	}
}

// Middleware returns the rate limiting middleware
func (rl *RateLimiter) Middleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get client IP
			ip := getClientIP(r)

			if !rl.Allow(ip) {
				reqID := getRequestID(r.Context())

				if rl.log != nil {
					rl.log.Warn("rate limit exceeded",
						"request_id", reqID,
						"ip", ip,
						"path", r.URL.Path,
					)
				}

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1") // Suggest retry after 1 second
				w.WriteHeader(http.StatusTooManyRequests)
				w.Write([]byte(`{"error": "rate limit exceeded", "retry_after": "1s"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (if behind proxy/load balancer)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	// Remove port if present
	ip := r.RemoteAddr
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == ':' {
			return ip[:i]
		}
	}
	return ip
}
