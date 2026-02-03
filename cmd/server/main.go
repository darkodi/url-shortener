package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	_ "github.com/mattn/go-sqlite3"

	"github.com/darkodi/url-shortener/internal/handler"
	"github.com/darkodi/url-shortener/internal/repository"
	"github.com/darkodi/url-shortener/internal/service"
)

func main() {
	// Configuration
	port := getEnv("PORT", "8080")
	dbPath := getEnv("DB_PATH", "./data/urls.db")
	baseURL := getEnv("BASE_URL", "http://localhost:"+port)

	// Initialize layers (bottom-up)
	fmt.Println("🗄️  Connecting to database...")
	repo, err := repository.NewURLRepository(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	fmt.Println("⚙️  Initializing service...")
	svc := service.NewURLService(repo, baseURL)

	fmt.Println("🌐 Setting up HTTP handlers...")
	h := handler.NewURLHandler(svc)
	router := h.SetupRoutes()

	// Start server
	addr := ":" + port
	fmt.Printf("🚀 Server starting on http://localhost%s\n", addr)
	fmt.Println("───────────────────────────────────────")
	fmt.Println("Endpoints:")
	fmt.Println("  POST /shorten     - Create short URL")
	fmt.Println("  GET  /{code}      - Redirect to original")
	fmt.Println("  GET  /{code}/stats - View statistics")
	fmt.Println("  GET  /health      - Health check")
	fmt.Println("───────────────────────────────────────")

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
