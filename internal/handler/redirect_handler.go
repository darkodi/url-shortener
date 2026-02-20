package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/darkodi/url-shortener/internal/errors"
	"github.com/darkodi/url-shortener/internal/service"
	"github.com/darkodi/url-shortener/internal/validator"
)

// RedirectHandler handles URL resolution and stats requests
type RedirectHandler struct {
	service   *service.URLService
	validator *validator.URLValidator
}

// NewRedirectHandler creates a new redirect handler instance
func NewRedirectHandler(svc *service.URLService) *RedirectHandler {
	return &RedirectHandler{
		service:   svc,
		validator: validator.NewURLValidator(),
	}
}

// HandleRedirect resolves a short code and redirects to original URL
// GET /:shortCode
func (h *RedirectHandler) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	shortCode := strings.TrimPrefix(r.URL.Path, "/")

	if shortCode == "" || shortCode == "favicon.ico" {
		http.NotFound(w, r)
		return
	}

	if shortCode == "health" {
		h.HandleHealth(w, r)
		return
	}

	// Check if this is a stats request: /abc/stats
	if strings.HasSuffix(shortCode, "/stats") {
		shortCode = strings.TrimSuffix(shortCode, "/stats")
		h.handleStats(w, r, shortCode)
		return
	}

	if appErr := h.validator.ValidateShortCode(shortCode); appErr != nil {
		appErr.WriteJSON(w)
		return
	}

	originalURL, err := h.service.Resolve(shortCode)
	if err != nil {
		if err == service.ErrURLNotFound {
			errors.URLNotFound(shortCode).WriteJSON(w)
			return
		}
		errors.Internal("").WriteJSON(w)
		return
	}

	http.Redirect(w, r, originalURL, http.StatusMovedPermanently)
}

// handleStats returns statistics for a short URL
// GET /:shortCode/stats
func (h *RedirectHandler) handleStats(w http.ResponseWriter, r *http.Request, shortCode string) {
	if appErr := h.validator.ValidateShortCode(shortCode); appErr != nil {
		appErr.WriteJSON(w)
		return
	}

	stats, err := h.service.GetURLStats(shortCode)
	if err != nil {
		if err == service.ErrURLNotFound {
			errors.URLNotFound(shortCode).WriteJSON(w)
			return
		}
		errors.Internal("").WriteJSON(w)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleHealth returns service health status
// GET /health
func (h *RedirectHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "healthy", "service": "redirect"}`))
}

// SetupRoutes configures routes for the redirect service
func (h *RedirectHandler) SetupRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/", h.HandleRedirect) // catch-all for /:code and /:code/stats
	return mux
}
