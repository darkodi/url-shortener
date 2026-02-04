package middleware

import (
	"context"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/darkodi/url-shortener/internal/logger"
	"github.com/google/uuid"
)

// ============================================================
// TYPES
// ============================================================

// Middleware is a function that wraps an http.Handler
type Middleware func(http.Handler) http.Handler

// ContextKey type for context values
type ContextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
)

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
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
			requestID = uuid.New().String()[:8] // Short ID for readability
		}

		// Add to response headers
		w.Header().Set("X-Request-ID", requestID)

		// Add to request context for use in handlers and other middleware
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ============================================================
// LOGGING MIDDLEWARE (with structured logger)
// ============================================================

// LoggingWithLogger creates a logging middleware with a structured logger
func LoggingWithLogger(log *logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Get request ID from context
			reqID := getRequestID(r.Context())

			// Wrap response writer to capture status code
			wrapped := wrapResponseWriter(w)

			// Process request
			next.ServeHTTP(wrapped, r)

			// Log the request
			log.Info("request completed",
				"request_id", reqID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// ============================================================
// RECOVERY MIDDLEWARE (with structured logger)
// ============================================================

// RecoveryWithLogger creates a recovery middleware with structured logging
func RecoveryWithLogger(log *logger.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					reqID := getRequestID(r.Context())

					log.Error("panic recovered",
						"request_id", reqID,
						"error", err,
						"stack", string(debug.Stack()),
						"method", r.Method,
						"path", r.URL.Path,
					)

					http.Error(w,
						`{"error": "Internal server error"}`,
						http.StatusInternalServerError,
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// ============================================================
// CHAIN HELPER
// ============================================================

// Chain applies middlewares in order (first middleware is outermost)
func Chain(h http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

// ============================================================
// HELPERS
// ============================================================

func getRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok {
		return reqID
	}
	return "unknown"
}
