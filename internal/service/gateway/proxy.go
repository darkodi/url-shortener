package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/darkodi/url-shortener/internal/logger"
)

// GatewayConfig holds upstream service addresses
type GatewayConfig struct {
	ShortenerAddr string // e.g. "http://shortener:8081"
	RedirectAddr  string // e.g. "http://redirect:8082"
}

// Gateway is a reverse proxy router
type Gateway struct {
	shortenerProxy *httputil.ReverseProxy
	redirectProxy  *httputil.ReverseProxy
	log            *logger.Logger
}

// NewGateway creates a new Gateway with two upstream proxies
func NewGateway(cfg GatewayConfig, log *logger.Logger) (*Gateway, error) {
	shortenerURL, err := url.Parse(cfg.ShortenerAddr)
	if err != nil {
		return nil, err
	}

	redirectURL, err := url.Parse(cfg.RedirectAddr)
	if err != nil {
		return nil, err
	}

	shortenerProxy := httputil.NewSingleHostReverseProxy(shortenerURL)
	redirectProxy := httputil.NewSingleHostReverseProxy(redirectURL)

	// Custom error handlers so upstream failures return clean JSON
	shortenerProxy.ErrorHandler = proxyErrorHandler(log, "shortener")
	redirectProxy.ErrorHandler = proxyErrorHandler(log, "redirect")

	return &Gateway{
		shortenerProxy: shortenerProxy,
		redirectProxy:  redirectProxy,
		log:            log,
	}, nil
}

// ServeHTTP implements http.Handler — this is the routing decision point
func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	switch {
	// ── Write path ─────────────────────────────────────────────
	// POST /shorten → shortener service
	case path == "/shorten" && r.Method == http.MethodPost:
		g.log.Debug("routing to shortener", "path", path, "method", r.Method)
		g.shortenerProxy.ServeHTTP(w, r)

	// ── Health checks ───────────────────────────────────────────
	// GET /health → gateway itself responds (no upstream needed)
	case path == "/health" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"healthy","service":"gateway"}`))

	// ── Read path ───────────────────────────────────────────────
	// GET /:code        → redirect service
	// GET /:code/stats  → redirect service
	case r.Method == http.MethodGet && isShortCodePath(path):
		g.log.Debug("routing to redirect", "path", path, "method", r.Method)
		g.redirectProxy.ServeHTTP(w, r)

	// ── Catch-all ───────────────────────────────────────────────
	default:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found","code":"ROUTE_NOT_FOUND"}`))
	}
}

// isShortCodePath returns true for /:code and /:code/stats paths
func isShortCodePath(path string) bool {
	// Strip leading slash
	trimmed := strings.TrimPrefix(path, "/")

	// Reject empty or known non-code paths
	if trimmed == "" || trimmed == "favicon.ico" {
		return false
	}

	// Allow /:code and /:code/stats only — no deeper nesting
	parts := strings.Split(trimmed, "/")
	if len(parts) == 1 {
		return true // /:code
	}
	if len(parts) == 2 && parts[1] == "stats" {
		return true // /:code/stats
	}

	return false
}

// proxyErrorHandler returns a clean JSON error when an upstream is unreachable
func proxyErrorHandler(log *logger.Logger, service string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		log.Error("upstream proxy error",
			"service", service,
			"path", r.URL.Path,
			"error", err.Error(),
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte(`{"error":"upstream service unavailable","code":"BAD_GATEWAY"}`))
	}
}
