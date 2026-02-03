package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/darkodi/url-shortener/internal/model"
	"github.com/darkodi/url-shortener/internal/service"
)

// URLHandler handles HTTP requests for URL operations
type URLHandler struct {
	service *service.URLService
}

// NewURLHandler creates a new handler instance
func NewURLHandler(svc *service.URLService) *URLHandler {
	return &URLHandler{service: svc}
}

// ============ RESPONSE HELPERS ============

// ErrorResponse represents an error in JSON format
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err string, message string) {
	writeJSON(w, status, ErrorResponse{
		Error:   err,
		Message: message,
	})
}

// ============ HANDLERS ============

// HandleShorten creates a new short URL
// POST /shorten
func (h *URLHandler) HandleShorten(w http.ResponseWriter, r *http.Request) {
	// Only accept POST
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST")
		return
	}

	// Parse JSON body
	var req model.CreateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Could not parse request body")
		return
	}

	// Call service
	resp, err := h.service.CreateShortURL(req)
	if err != nil {
		// Map service errors to HTTP status codes
		switch err {
		case service.ErrEmptyURL:
			writeError(w, http.StatusBadRequest, "empty_url", "URL is required")
		case service.ErrInvalidURL:
			writeError(w, http.StatusBadRequest, "invalid_url", "URL must be valid http/https")
		case service.ErrAliasExists:
			writeError(w, http.StatusConflict, "alias_taken", "Custom alias already in use")
		case service.ErrInvalidAlias:
			writeError(w, http.StatusBadRequest, "invalid_alias", "Alias must be 3-20 alphanumeric chars")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
		}
		return
	}

	// Success!
	writeJSON(w, http.StatusCreated, resp)
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

	// Resolve the short code
	originalURL, err := h.service.Resolve(shortCode)
	if err != nil {
		if err == service.ErrURLNotFound {
			writeError(w, http.StatusNotFound, "not_found", "Short URL not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
		return
	}

	// Redirect!
	http.Redirect(w, r, originalURL, http.StatusMovedPermanently)
}

// handleStats returns statistics for a short URL
// GET /{shortCode}/stats
func (h *URLHandler) handleStats(w http.ResponseWriter, r *http.Request, shortCode string) {
	stats, err := h.service.GetURLStats(shortCode)
	if err != nil {
		if err == service.ErrURLNotFound {
			writeError(w, http.StatusNotFound, "not_found", "Short URL not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
		return
	}

	writeJSON(w, http.StatusOK, stats)
}

// HandleHealth returns service health status
// GET /health
func (h *URLHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
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
