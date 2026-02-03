package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/darkodi/url-shortener/internal/handler"
	"github.com/darkodi/url-shortener/internal/middleware"
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

	// wrap router with middleware
	wrappedRouter := middleware.Chain(
		router,
		middleware.RequestID, // first assign request ID
		middleware.Recovery,  // then recover from panics
		middleware.Logging,   // then log requests
	)

	// ============================================================
	// GRACEFUL SHUTDOWN SETUP
	// ============================================================
	// Create server with timeouts
	addr := ":" + port
	server := &http.Server{
		Addr:         addr,
		Handler:      wrappedRouter,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	// Channel to listen for shutdown signals
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Channel to track server errors
	serverErr := make(chan error, 1)

	// Start server in a goroutine
	go func() {
		fmt.Printf("🚀 Server starting on http://localhost%s\n", addr)
		fmt.Println("───────────────────────────────────────")
		fmt.Println("Endpoints:")
		fmt.Println("  POST /shorten      - Create short URL")
		fmt.Println("  GET  /{code}       - Redirect to original")
		fmt.Println("  GET  /{code}/stats - View statistics")
		fmt.Println("  GET  /health       - Health check")
		fmt.Println("───────────────────────────────────────")
		fmt.Println("Press Ctrl+C to shutdown gracefully")

		serverErr <- server.ListenAndServe()
	}()

	// ============================================================
	// WAIT FOR SHUTDOWN OR ERROR
	// ============================================================
	select {
	case err := <-serverErr:
		log.Fatalf("Server error: %v", err)

	case sig := <-shutdown:
		fmt.Printf("\n⚠️  Caught signal %v: shutting down gracefully...\n", sig)
		// Create context with timeout for shutdown
		ctx, cancel := context.WithTimeout(
			context.Background(),
			30*time.Second,
		)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Graceful shutdown failed: %v", err)
			// force close if graceful shutdown fails
			if err := server.Close(); err != nil {
				log.Printf("Forced shutdown failed: %v", err)
			}
		}

		// Close repository (database connection)
		if err := repo.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}

		fmt.Println("✅ Server stopped")
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
