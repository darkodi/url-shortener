package middleware

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/google/uuid"
)

// ============================================================
// TYPES
// ============================================================

// Middleware is a function that wraps an http.Handler
type Middleware func(http.Handler) http.Handler

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.status = code
		rw.wroteHeader = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

// ============================================================
// REQUEST ID MIDDLEWARE
// ============================================================

// RequestID adds a unique request ID to each request
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if request already has an ID (from load balancer, etc.)
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to response headers so client can reference it
		w.Header().Set("X-Request-ID", requestID)

		// Add to request context for use in handlers
		// (We'll enhance this later)

		next.ServeHTTP(w, r)
	})
}

// ============================================================
// LOGGING MIDDLEWARE
// ============================================================

// Logging logs every request with method, path, status, and duration
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := wrapResponseWriter(w)

		// Get request ID from header (set by RequestID middleware)
		requestID := w.Header().Get("X-Request-ID")

		// Process request
		next.ServeHTTP(wrapped, r)

		// Log after request completes
		log.Printf(
			"[%s] %s %s %s %d %v",
			requestID[:8], // First 8 chars of request ID
			r.Method,
			r.URL.Path,
			r.RemoteAddr,
			wrapped.status,
			time.Since(start),
		)
	})
}

// ============================================================
// RECOVERY MIDDLEWARE
// ============================================================

// Recovery catches panics and returns 500 instead of crashing
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the stack trace
				log.Printf(
					"PANIC RECOVERED: %v\n%s",
					err,
					debug.Stack(),
				)

				// Return 500 to client
				http.Error(w,
					"Internal Server Error",
					http.StatusInternalServerError,
				)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// ============================================================
// CHAIN HELPER
// ============================================================

// Chain applies middlewares in order (first middleware is outermost)
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	// Apply in reverse so first middleware is outermost
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}
