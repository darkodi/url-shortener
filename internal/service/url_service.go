package service

import (
	"errors"
	"net/url"
	"strings"

	"github.com/darkodi/url-shortener/internal/encoder"
	"github.com/darkodi/url-shortener/internal/model"
	"github.com/darkodi/url-shortener/internal/repository"
)

// Custom errors for the service layer
var (
	ErrInvalidURL   = errors.New("invalid URL format")
	ErrEmptyURL     = errors.New("URL cannot be empty")
	ErrAliasExists  = errors.New("custom alias already taken")
	ErrInvalidAlias = errors.New("alias contains invalid characters")
	ErrURLNotFound  = errors.New("short URL not found")
)

// URLService handles business logic for URL operations
type URLService struct {
	repo    *repository.URLRepository
	baseURL string // e.g., "http://localhost:8080"
}

// NewURLService creates a new service instance
func NewURLService(repo *repository.URLRepository, baseURL string) *URLService {
	return &URLService{
		repo:    repo,
		baseURL: strings.TrimRight(baseURL, "/"),
	}
}

// CreateShortURL handles the core business logic of shortening a URL
func (s *URLService) CreateShortURL(req model.CreateURLRequest) (*model.CreateURLResponse, error) {
	// ============ STEP 1: Validation ============
	if err := s.validateURL(req.URL); err != nil {
		return nil, err
	}

	// ============ STEP 2: Determine Short Code ============
	var shortCode string

	if req.CustomAlias != "" {
		// User wants a custom alias
		if err := s.validateAlias(req.CustomAlias); err != nil {
			return nil, err
		}

		// Check if alias is already taken
		_, err := s.repo.GetByShortCode(req.CustomAlias)
		if err == nil {
			return nil, ErrAliasExists // Found existing = taken!
		}
		if err != repository.ErrNotFound {
			return nil, err // Some other database error
		}

		shortCode = req.CustomAlias
	} else {
		// Generate code from next ID
		nextID, err := s.repo.GetNextID()
		if err != nil {
			return nil, err
		}
		shortCode = encoder.Encode(nextID)
	}

	// ============ STEP 3: Create the record ============
	urlRecord := &model.URL{
		ShortCode:   shortCode,
		OriginalURL: req.URL,
	}

	if err := s.repo.Create(urlRecord); err != nil {
		return nil, err
	}

	// ============ STEP 4: Build response ============
	return &model.CreateURLResponse{
		ShortURL:    s.baseURL + "/" + shortCode,
		OriginalURL: req.URL,
	}, nil
}

// Resolve finds the original URL and increments click count
func (s *URLService) Resolve(shortCode string) (string, error) {
	// Find the URL
	urlRecord, err := s.repo.GetByShortCode(shortCode)
	if err == repository.ErrNotFound {
		return "", ErrURLNotFound
	}
	if err != nil {
		return "", err
	}

	// Increment click count (fire and forget - don't fail if this errors)
	_ = s.repo.IncrementClickCount(shortCode)

	return urlRecord.OriginalURL, nil
}

// GetURLStats returns statistics for a short URL
func (s *URLService) GetURLStats(shortCode string) (*model.URL, error) {
	urlRecord, err := s.repo.GetByShortCode(shortCode)
	if err == repository.ErrNotFound {
		return nil, ErrURLNotFound
	}
	return urlRecord, err
}

// ============ VALIDATION HELPERS ============

func (s *URLService) validateURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return ErrEmptyURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidURL
	}

	// Must have scheme (http/https) and host
	if parsed.Scheme == "" || parsed.Host == "" {
		return ErrInvalidURL
	}

	// Only allow http and https
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidURL
	}

	return nil
}

func (s *URLService) validateAlias(alias string) error {
	if len(alias) < 3 || len(alias) > 20 {
		return ErrInvalidAlias
	}

	// Only allow alphanumeric, hyphens, underscores
	for _, char := range alias {
		if !isValidAliasChar(char) {
			return ErrInvalidAlias
		}
	}

	return nil
}

func isValidAliasChar(char rune) bool {
	return (char >= 'a' && char <= 'z') ||
		(char >= 'A' && char <= 'Z') ||
		(char >= '0' && char <= '9') ||
		char == '-' || char == '_'
}
