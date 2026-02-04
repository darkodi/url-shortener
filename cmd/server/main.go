package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"

	"github.com/darkodi/url-shortener/internal/config"
	"github.com/darkodi/url-shortener/internal/handler"
	"github.com/darkodi/url-shortener/internal/logger"
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
		panic("Failed to load configuration: " + err.Error())
	}

	if cfg.IsDevelopment() {
		fmt.Printf("   Environment: %s\n", cfg.App.Environment)
		fmt.Printf("   Port: %s\n", cfg.Server.Port)
		fmt.Printf("   Database: %s\n", cfg.Database.Path)
		fmt.Printf("   Base URL: %s\n", cfg.App.BaseURL)
	}

	// ============================================================
	// Initialize logger
	// ============================================================
	fmt.Println("📝 Initializing logger...")
	log := logger.New(logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		Environment: cfg.App.Environment,
	})
	log.Info("starting url-shortener",
		"level", cfg.Log.Level,
		"format", cfg.Log.Format,
		"environment", cfg.App.Environment)
	// ============================================================
	// INITIALIZE LAYERS
	// ============================================================
	fmt.Println("🗄️  Connecting to database...")
	repo, err := repository.NewURLRepository(cfg.Database.Path)
	if err != nil {
		log.Error("Failed to initialize database", "error", err.Error())
		os.Exit(1)
	}

	fmt.Println("⚙️  Initializing service...")
	svc := service.NewURLService(repo, cfg.App.BaseURL)

	fmt.Println("🌐 Setting up HTTP handlers...")
	h := handler.NewURLHandler(svc)
	router := h.SetupRoutes()

	// wrap router with middleware
	wrappedRouter := middleware.Chain(
		router,
		middleware.RequestID,               // first assign request ID
		middleware.RecoveryWithLogger(log), // then recover from panics
		middleware.LoggingWithLogger(log),  // then log requests
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
		if cfg.IsDevelopment() {
			fmt.Printf("🚀 Server starting on http://localhost%s\n", addr)
			fmt.Println("───────────────────────────────────────")
			fmt.Println("Endpoints:")
			fmt.Println("  POST /shorten      - Create short URL")
			fmt.Println("  GET  /{code}       - Redirect to original")
			fmt.Println("  GET  /{code}/stats - View statistics")
			fmt.Println("  GET  /health       - Health check")
			fmt.Println("───────────────────────────────────────")
			fmt.Println("Press Ctrl+C to shutdown gracefully")
		}
		log.Info("server starting", "addr", "http://localhost"+addr)
		serverErr <- server.ListenAndServe()
	}()

	// ============================================================
	// WAIT FOR SHUTDOWN OR ERROR
	// ============================================================
	select {
	case err := <-serverErr:
		log.Error("server error", "error", err.Error())
		os.Exit(1)

	case sig := <-shutdown:
		log.Info("shutdown signal received", "signal", sig.String())
		// Create context with timeout for shutdown
		ctx, cancel := context.WithTimeout(
			context.Background(),
			cfg.Server.ShutdownTimeout,
		)
		defer cancel()

		// Attempt graceful shutdown
		if err := server.Shutdown(ctx); err != nil {
			log.Error("graceful shutdown failed", "error", err.Error())
			// force close if graceful shutdown fails
			if err := server.Close(); err != nil {
				log.Error("forced shutdown failed", "error", err.Error())
			}
		}

		// Close repository (database connection)
		if err := repo.Close(); err != nil {
			log.Error("failed to close database", "error", err.Error())
		}

		log.Info("server stopped")
	}
}
