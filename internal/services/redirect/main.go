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
	fmt.Println("Loading Redirect service configuration...")
	cfg, err := config.Load()
	if err != nil {
		panic("Failed to load configuration for Redirect service: " + err.Error())
	}

	// ============================================================
	// INITIALIZE LOGGER
	// ============================================================
	log := logger.New(logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		Environment: cfg.Log.Environment,
	})
	log.Info("starting Redirect service",
		"level", cfg.Log.Level,
		"format", cfg.Log.Format,
		"environment", cfg.App.Environment,
		"port", cfg.Server.Port,
	)

	// ============================================================
	// INITIALIZE DATABASE
	// ============================================================
	log.Info("connecting to database for Redirect service...")
	repo, err := repository.NewURLRepository(cfg)
	if err != nil {
		log.Error("failed to initialize database for Redirect service", "error", err.Error())
		os.Exit(1)
	}
	defer func() {
		if err := repo.Close(); err != nil {
			log.Error("failed to close database connection for Redirect service", "error", err.Error())
		}
	}()
	log.Info("database connection for Redirect service successful!")

	// ============================================================
	// INITIALIZE REDIS CACHE
	// ============================================================
	log.Info("connecting to Redis for Redirect service...")
	redisCache, err := cache.NewRedisCache(&cfg.Redis)
	if err != nil {
		log.Error("failed to connect to Redis for Redirect service", "error", err.Error())
		os.Exit(1)
	}
	defer func() {
		if err := redisCache.Close(); err != nil {
			log.Error("failed to close Redis client for Redirect service", "error", err.Error())
		}
	}()
	log.Info("Redis connection for Redirect service successful!")

	// ============================================================
	// INITIALIZE SERVICE + HANDLER
	// ============================================================
	log.Info("initializing service layer for Redirect service...")
	svc := service.NewURLService(repo, cfg.App.BaseURL, redisCache)

	log.Info("setting up HTTP handlers for Redirect service...")
	h := handler.NewRedirectHandler(svc)
	router := h.SetupRoutes()

	// ============================================================
	// BUILD MIDDLEWARE CHAIN
	// ============================================================
	// Note: No rate limiter here — rate limiting is the Gateway's responsibility.
	// The redirect path is the hottest path in the system; we keep it as lean as possible.
	middlewares := []middleware.Middleware{
		middleware.RequestID,
		middleware.RecoveryWithLogger(log),
		middleware.LoggingWithLogger(log),
	}

	wrappedRouter := middleware.Chain(router, middlewares...)

	// ============================================================
	// CREATE SERVER
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
		log.Info("Redirect service starting", "addr", addr)
		serverErr <- server.ListenAndServe()
	}()

	// ============================================================
	// WAIT FOR SHUTDOWN OR ERROR
	// ============================================================
	select {
	case err := <-serverErr:
		log.Error("Redirect service error", "error", err.Error())
		os.Exit(1)

	case sig := <-shutdown:
		log.Info("Redirect service shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(
			context.Background(),
			cfg.Server.ShutdownTimeout,
		)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Error("Redirect service graceful shutdown failed", "error", err.Error())
			if err := server.Close(); err != nil {
				log.Error("Redirect service forced shutdown failed", "error", err.Error())
			}
		}
		log.Info("Redirect service stopped")
	}
}
