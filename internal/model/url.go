package model

import "time"

// URL represents a shortened URL mapping
type URL struct {
	ID          uint64    `json:"id"`           // input to Base62 encoder
	ShortCode   string    `json:"short_code"`   // base62 encoded string
	OriginalURL string    `json:"original_url"` // original long URL
	CreatedAt   time.Time `json:"created_at"`   // timestamp of creation
	ClickCount  uint64    `json:"click_count"`  // how many times the short URL was accessed
}

// CreateURLRequest is the API request body
type CreateURLRequest struct {
	URL         string `json:"url"`                    // original long URL
	CustomAlias string `json:"custom_alias,omitempty"` // optional custom short code
}

// CreateURLResponse is the API response
type CreateURLResponse struct {
	ShortURL    string `json:"short_url"`    // full shortened URL
	OriginalURL string `json:"original_url"` // original long URL
}
