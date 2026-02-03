package service

import (
	"testing"

	"github.com/darkodi/url-shortener/internal/model"
	"github.com/darkodi/url-shortener/internal/repository"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestService(t *testing.T) *URLService {
	// Use in-memory SQLite for tests
	repo, err := repository.NewURLRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to create repo: %v", err)
	}
	return NewURLService(repo, "http://localhost:8080")
}

func TestCreateShortURL_Valid(t *testing.T) {
	svc := setupTestService(t)

	resp, err := svc.CreateShortURL(model.CreateURLRequest{
		URL: "https://example.com/some/long/path",
	})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.ShortURL == "" {
		t.Error("Expected short URL, got empty")
	}

	if resp.OriginalURL != "https://example.com/some/long/path" {
		t.Errorf("Original URL mismatch")
	}
}

func TestCreateShortURL_InvalidURL(t *testing.T) {
	svc := setupTestService(t)

	tests := []struct {
		name string
		url  string
	}{
		{"empty", ""},
		{"no scheme", "example.com"},
		{"ftp scheme", "ftp://example.com"},
		{"just text", "not a url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateShortURL(model.CreateURLRequest{URL: tt.url})
			if err == nil {
				t.Errorf("Expected error for URL: %s", tt.url)
			}
		})
	}
}

func TestCreateShortURL_CustomAlias(t *testing.T) {
	svc := setupTestService(t)

	resp, err := svc.CreateShortURL(model.CreateURLRequest{
		URL:         "https://example.com",
		CustomAlias: "my-link",
	})

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if resp.ShortURL != "http://localhost:8080/my-link" {
		t.Errorf("Expected custom alias in URL, got: %s", resp.ShortURL)
	}
}

func TestCreateShortURL_DuplicateAlias(t *testing.T) {
	svc := setupTestService(t)

	// First one should succeed
	_, err := svc.CreateShortURL(model.CreateURLRequest{
		URL:         "https://example.com",
		CustomAlias: "taken",
	})
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	// Second with same alias should fail
	_, err = svc.CreateShortURL(model.CreateURLRequest{
		URL:         "https://other.com",
		CustomAlias: "taken",
	})
	if err != ErrAliasExists {
		t.Errorf("Expected ErrAliasExists, got: %v", err)
	}
}

func TestResolve(t *testing.T) {
	svc := setupTestService(t)

	// Create a URL first
	_, _ = svc.CreateShortURL(model.CreateURLRequest{
		URL:         "https://example.com",
		CustomAlias: "test",
	})

	// Resolve it
	original, err := svc.Resolve("test")
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if original != "https://example.com" {
		t.Errorf("Expected original URL, got: %s", original)
	}

	// Check that click count increased
	stats, _ := svc.GetURLStats("test")
	if stats.ClickCount != 1 {
		t.Errorf("Expected click count 1, got: %d", stats.ClickCount)
	}
}
