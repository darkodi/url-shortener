package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"

	"github.com/darkodi/url-shortener/internal/config"
	"github.com/darkodi/url-shortener/internal/handler"
	"github.com/darkodi/url-shortener/internal/middleware"
	"github.com/darkodi/url-shortener/internal/repository"
	"github.com/darkodi/url-shortener/internal/service"
)

func main() {
	// ============================================================
	// LOAD CONFIGURATION
	// ============================================================
	fmt.Println("📋 Loading configuration...")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if cfg.IsDevelopment() {
		fmt.Printf("   Environment: %s\n", cfg.App.Environment)
		fmt.Printf("   Port: %s\n", cfg.Server.Port)
		fmt.Printf("   Database: %s\n", cfg.Database.Path)
		fmt.Printf("   Base URL: %s\n", cfg.App.BaseURL)
	}

	// ============================================================
	// INITIALIZE LAYERS
	// ============================================================
	fmt.Println("🗄️  Connecting to database...")
	repo, err := repository.NewURLRepository(cfg.Database.Path)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	fmt.Println("⚙️  Initializing service...")
	svc := service.NewURLService(repo, cfg.App.BaseURL)

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
	// CREATE SERVER WITH CONFIG TIMEOUTS
	// ============================================================
	addr := ":" + cfg.Server.Port
	server := &http.Server{
		Addr:         addr,
		Handler:      wrappedRouter,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
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
			cfg.Server.ShutdownTimeout,
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
