package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"

	"github.com/darkodi/url-shortener/internal/cache"
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
	fmt.Println("Loading Shortener service configuration...")
	cfg, err := config.Load()
	if err != nil {
		panic("Failed to load configuration for Shortener service: " + err.Error())
	}

	// ============================================================
	// Initialize logger
	// ============================================================
	log := logger.New(logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		Environment: cfg.Log.Environment,
	})
	log.Info("starting Shortener service",
		"level", cfg.Log.Level,
		"format", cfg.Log.Format,
		"environment", cfg.App.Environment,
		"port", cfg.Server.Port,
	)

	// ============================================================
	// INITIALIZE LAYERS
	// ============================================================
	log.Info("Connecting to database for Shortener service...")
	repo, err := repository.NewURLRepository(cfg)
	if err != nil {
		log.Error("Failed to initialize database for Shortener service", "error", err.Error())
		os.Exit(1)
	}
	defer func() {
		if err := repo.Close(); err != nil {
			log.Error("Failed to close database connection for Shortener service", "error", err.Error())
		}
	}()
	log.Info("Database connection for Shortener service successful!")

	log.Info("Connecting to Redis for Shortener service...")
	redisCache, err := cache.NewRedisCache(&cfg.Redis)
	if err != nil {
		log.Error("Failed to connect to Redis for Shortener service", "error", err.Error())
		os.Exit(1)
	}
	defer func() {
		if err := redisCache.Close(); err != nil {
			log.Error("Failed to close Redis client for Shortener service", "error", err.Error())
		}
	}()
	log.Info("Redis connection for Shortener service successful!")

	log.Info("Initializing service layer for Shortener service...")
	// The baseURL here should eventually be the API Gateway's public URL for correct short URL generation
	svc := service.NewURLService(repo, cfg.App.BaseURL, redisCache)

	log.Info("Setting up HTTP handlers for Shortener service...")
	h := handler.NewShortenHandler(svc)
	router := h.SetupRoutes()

	// ============================================================
	// BUILD MIDDLEWARE CHAIN
	// ============================================================
	middlewares := []middleware.Middleware{
		middleware.RequestID,
		middleware.RecoveryWithLogger(log),
		middleware.LoggingWithLogger(log),
	}
	if cfg.RateLimit.Enabled {
		rateLimiter := middleware.NewRateLimiter(
			middleware.RateLimiterConfig{
				Rate:     cfg.RateLimit.Rate,
				Burst:    cfg.RateLimit.Burst,
				Interval: cfg.RateLimit.Interval,
				Cleanup:  cfg.RateLimit.Cleanup,
			},
			log,
		)
		middlewares = append(middlewares, rateLimiter.Middleware())
		log.Info("rate limiter enabled for Shortener service",
			"rate", cfg.RateLimit.Rate,
			"burst", cfg.RateLimit.Burst,
		)
	}

	wrappedRouter := middleware.Chain(router, middlewares...)

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

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	serverErr := make(chan error, 1)

	go func() {
		log.Info("Shortener service starting", "addr", addr)
		serverErr <- server.ListenAndServe()
	}()

	// ============================================================
	// WAIT FOR SHUTDOWN OR ERROR
	// ============================================================
	select {
	case err := <-serverErr:
		log.Error("Shortener service error", "error", err.Error())
		os.Exit(1)

	case sig := <-shutdown:
		log.Info("Shortener service shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(
			context.Background(),
			cfg.Server.ShutdownTimeout,
		)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Error("Shortener service graceful shutdown failed", "error", err.Error())
			if err := server.Close(); err != nil {
				log.Error("Shortener service forced shutdown failed", "error", err.Error())
			}
		}
		log.Info("Shortener service stopped")
	}
}
