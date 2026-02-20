package handler

import (
	"encoding/json"
	"net/http"

	"github.com/darkodi/url-shortener/internal/errors"
	"github.com/darkodi/url-shortener/internal/model"
	"github.com/darkodi/url-shortener/internal/service"
	"github.com/darkodi/url-shortener/internal/validator"
)

// ShortenHandler handles URL creation requests
type ShortenHandler struct {
	service   *service.URLService
	validator *validator.URLValidator
}

// NewShortenHandler creates a new shorten handler instance
func NewShortenHandler(svc *service.URLService) *ShortenHandler {
	return &ShortenHandler{
		service:   svc,
		validator: validator.NewURLValidator(),
	}
}

// HandleShorten creates a new short URL
// POST /shorten
func (h *ShortenHandler) HandleShorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errors.BadRequest("Use POST method").WriteJSON(w)
		return
	}

	var req model.CreateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errors.InvalidJSON(err.Error()).WriteJSON(w)
		return
	}

	if appErr := h.validator.ValidateURL(req.URL); appErr != nil {
		appErr.WriteJSON(w)
		return
	}

	if appErr := h.validator.ValidateCustomCode(req.CustomAlias); appErr != nil {
		appErr.WriteJSON(w)
		return
	}

	resp, err := h.service.CreateShortURL(req)
	if err != nil {
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}

// HandleHealth returns service health status
// GET /health
func (h *ShortenHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "healthy", "service": "shortener"}`))
}

// SetupRoutes configures routes for the shortener service
func (h *ShortenHandler) SetupRoutes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/shorten", h.HandleShorten)
	mux.HandleFunc("/health", h.HandleHealth)
	return mux
}
