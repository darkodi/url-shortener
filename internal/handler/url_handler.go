package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/darkodi/url-shortener/internal/errors"
	"github.com/darkodi/url-shortener/internal/model"
	"github.com/darkodi/url-shortener/internal/service"
	"github.com/darkodi/url-shortener/internal/validator"
)

// URLHandler handles HTTP requests for URL operations
type URLHandler struct {
	service   *service.URLService
	validator *validator.URLValidator
}

// NewURLHandler creates a new handler instance
func NewURLHandler(svc *service.URLService) *URLHandler {
	return &URLHandler{
		service:   svc,
		validator: validator.NewURLValidator(),
	}
}

// ============ HANDLERS ============

// HandleShorten creates a new short URL
// POST /shorten
func (h *URLHandler) HandleShorten(w http.ResponseWriter, r *http.Request) {
	// Only accept POST
	if r.Method != http.MethodPost {
		errors.BadRequest("Use POST method").WriteJSON(w)
		return
	}

	// Parse JSON body
	var req model.CreateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.InvalidJSON(err.Error()).WriteJSON(w)
		return
	}

	// Validate URL with enhanced validator
	if appErr := h.validator.ValidateURL(req.URL); appErr != nil {
		appErr.WriteJSON(w)
		return
	}

	// Validate custom alias if provided
	if appErr := h.validator.ValidateCustomCode(req.CustomAlias); appErr != nil {
		appErr.WriteJSON(w)
		return
	}

	// Call service
	resp, err := h.service.CreateShortURL(req)
	if err != nil {
		// Map service errors to AppErrors
		switch err {
		case service.ErrEmptyURL:
			errors.MissingField("url").WriteJSON(w)
		case service.ErrInvalidURL:
			errors.InvalidURL("URL must be valid http/https").WriteJSON(w)
		case service.ErrAliasExists:
			errors.URLExists(req.CustomAlias).WriteJSON(w)
		case service.ErrInvalidAlias:
			errors.BadRequest("Alias must be 3-20 alphanumeric characters").WriteJSON(w)
		default:
			errors.Internal("").WriteJSON(w)
		}
		return
	}

	// Success!
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// HandleRedirect redirects to the original URL
// GET /{shortCode}
func (h *URLHandler) HandleRedirect(w http.ResponseWriter, r *http.Request) {
	// Extract short code from path: /abc → abc
	shortCode := strings.TrimPrefix(r.URL.Path, "/")

	// Ignore empty or special paths
	if shortCode == "" || shortCode == "favicon.ico" {
		http.NotFound(w, r)
		return
	}

	// Skip if it's a known route
	if shortCode == "shorten" || shortCode == "health" {
		http.NotFound(w, r)
		return
	}

	// Check if this is a stats request: /abc/stats
	if strings.HasSuffix(shortCode, "/stats") {
		shortCode = strings.TrimSuffix(shortCode, "/stats")
		h.handleStats(w, r, shortCode)
		return
	}

	// Validate short code format
	if appErr := h.validator.ValidateShortCode(shortCode); appErr != nil {
		appErr.WriteJSON(w)
		return
	}

	// Resolve the short code
	originalURL, err := h.service.Resolve(shortCode)
	if err != nil {
		if err == service.ErrURLNotFound {
			errors.URLNotFound(shortCode).WriteJSON(w)
			return
		}
		errors.Internal("").WriteJSON(w)
		return
	}

	// Redirect!
	http.Redirect(w, r, originalURL, http.StatusMovedPermanently)
}

// handleStats returns statistics for a short URL
// GET /{shortCode}/stats
func (h *URLHandler) handleStats(w http.ResponseWriter, r *http.Request, shortCode string) {
	// Validate short code format
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
func (h *URLHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "healthy"}`))
}

// ============ ROUTER SETUP ============

// SetupRoutes configures all HTTP routes
func (h *URLHandler) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// Specific routes first
	mux.HandleFunc("/shorten", h.HandleShorten)
	mux.HandleFunc("/health", h.HandleHealth)

	// Catch-all for redirects (must be last)
	mux.HandleFunc("/", h.HandleRedirect)

	return mux
}
