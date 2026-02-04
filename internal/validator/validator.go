package validator

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/darkodi/url-shortener/internal/errors"
)

// URLValidator validates URL inputs
type URLValidator struct {
	maxLength       int
	allowedSchemes  []string
	blockedDomains  []string
	blockPrivateIPs bool
}

// NewURLValidator creates a validator with default settings
func NewURLValidator() *URLValidator {
	return &URLValidator{
		maxLength:       2048,
		allowedSchemes:  []string{"http", "https"},
		blockedDomains:  []string{},
		blockPrivateIPs: true,
	}
}

// ValidateURL validates a URL string
func (v *URLValidator) ValidateURL(rawURL string) *errors.AppError {
	// Check if empty
	if strings.TrimSpace(rawURL) == "" {
		return errors.MissingField("url")
	}

	// Check length
	if len(rawURL) > v.maxLength {
		return errors.InvalidURL("URL exceeds maximum length of 2048 characters")
	}

	// Parse URL
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return errors.InvalidURL("URL could not be parsed")
	}

	// Check scheme
	if !v.isAllowedScheme(parsedURL.Scheme) {
		return errors.InvalidURL("URL must use http or https scheme")
	}

	// Check host exists
	if parsedURL.Host == "" {
		return errors.InvalidURL("URL must have a valid host")
	}

	// Check for blocked domains
	if v.isBlockedDomain(parsedURL.Host) {
		return errors.InvalidURL("This domain is not allowed")
	}

	// Check for private/local IPs
	if v.blockPrivateIPs && v.isPrivateIP(parsedURL.Host) {
		return errors.InvalidURL("URLs pointing to private IPs are not allowed")
	}

	return nil
}

// ValidateShortCode validates a short code format
func (v *URLValidator) ValidateShortCode(code string) *errors.AppError {
	if code == "" {
		return errors.MissingField("code")
	}

	// Check length (typically 6-10 characters)
	if len(code) < 3 || len(code) > 20 {
		return errors.BadRequest("Short code must be between 4 and 20 characters")
	}

	// Check format (alphanumeric only)
	validCode := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validCode.MatchString(code) {
		return errors.BadRequest("Short code can only contain letters, numbers, hyphens, and underscores")
	}

	return nil
}

// ValidateCustomCode validates a custom short code
func (v *URLValidator) ValidateCustomCode(code string) *errors.AppError {
	if code == "" {
		return nil // Custom code is optional
	}

	// Check reserved words
	reserved := []string{"api", "admin", "health", "shorten", "stats", "static"}
	for _, r := range reserved {
		if strings.EqualFold(code, r) {
			return errors.BadRequest("This short code is reserved and cannot be used")
		}
	}

	return v.ValidateShortCode(code)
}

// ============================================================
// HELPER METHODS
// ============================================================

func (v *URLValidator) isAllowedScheme(scheme string) bool {
	scheme = strings.ToLower(scheme)
	for _, allowed := range v.allowedSchemes {
		if scheme == allowed {
			return true
		}
	}
	return false
}

func (v *URLValidator) isBlockedDomain(host string) bool {
	host = strings.ToLower(host)
	for _, blocked := range v.blockedDomains {
		if strings.Contains(host, blocked) {
			return true
		}
	}
	return false
}

func (v *URLValidator) isPrivateIP(host string) bool {
	// Remove port if present
	hostOnly := host
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		hostOnly = host[:idx]
	}

	// Check for localhost variants
	localPatterns := []string{
		"localhost",
		"127.",
		"0.0.0.0",
		"::1",
		"10.",
		"192.168.",
		"172.16.", "172.17.", "172.18.", "172.19.",
		"172.20.", "172.21.", "172.22.", "172.23.",
		"172.24.", "172.25.", "172.26.", "172.27.",
		"172.28.", "172.29.", "172.30.", "172.31.",
	}

	for _, pattern := range localPatterns {
		if strings.HasPrefix(hostOnly, pattern) || hostOnly == pattern {
			return true
		}
	}

	return false
}

// ============================================================
// CONFIGURATION METHODS
// ============================================================

// WithMaxLength sets maximum URL length
func (v *URLValidator) WithMaxLength(length int) *URLValidator {
	v.maxLength = length
	return v
}

// WithBlockedDomains adds domains to block list
func (v *URLValidator) WithBlockedDomains(domains ...string) *URLValidator {
	v.blockedDomains = append(v.blockedDomains, domains...)
	return v
}

// WithAllowPrivateIPs allows private IP addresses
func (v *URLValidator) WithAllowPrivateIPs() *URLValidator {
	v.blockPrivateIPs = false
	return v
}
