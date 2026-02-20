package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/darkodi/url-shortener/internal/config"
	"github.com/darkodi/url-shortener/internal/logger"
	"github.com/darkodi/url-shortener/internal/middleware"
)

func main() {
	// ============================================================
	// LOAD CONFIGURATION
	// ============================================================
	fmt.Println("📋 Loading Gateway configuration...")
	cfg, err := config.Load()
	if err != nil {
		panic("Failed to load Gateway configuration: " + err.Error())
	}

	// ============================================================
	// INITIALIZE LOGGER
	// ============================================================
	log := logger.New(logger.Config{
		Level:       cfg.Log.Level,
		Format:      cfg.Log.Format,
		Environment: cfg.Log.Environment,
	})
	log.Info("starting Gateway service",
		"port", cfg.Server.Port,
		"environment", cfg.App.Environment,
	)

	// ============================================================
	// READ UPSTREAM ADDRESSES FROM ENV
	// ============================================================
	// These are set per-service in docker-compose.yml
	shortenerAddr := getEnv("SHORTENER_ADDR", "http://shortener:8081")
	redirectAddr := getEnv("REDIRECT_ADDR", "http://redirect:8082")

	log.Info("upstream addresses configured",
		"shortener", shortenerAddr,
		"redirect", redirectAddr,
	)

	// ============================================================
	// BUILD GATEWAY (REVERSE PROXY ROUTER)
	// ============================================================
	gateway, err := NewGateway(GatewayConfig{
		ShortenerAddr: shortenerAddr,
		RedirectAddr:  redirectAddr,
	}, log)
	if err != nil {
		log.Error("failed to create gateway", "error", err.Error())
		os.Exit(1)
	}

	// ============================================================
	// BUILD MIDDLEWARE CHAIN
	// Rate limiting lives HERE and ONLY here.
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
		log.Info("rate limiter enabled on gateway",
			"rate", cfg.RateLimit.Rate,
			"burst", cfg.RateLimit.Burst,
		)
	}

	wrappedGateway := middleware.Chain(gateway, middlewares...)

	// ============================================================
	// CREATE SERVER
	// ============================================================
	addr := ":" + cfg.Server.Port
	server := &http.Server{
		Addr:         addr,
		Handler:      wrappedGateway,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)
	serverErr := make(chan error, 1)

	go func() {
		log.Info("Gateway listening", "addr", addr)
		serverErr <- server.ListenAndServe()
	}()

	// ============================================================
	// WAIT FOR SHUTDOWN OR ERROR
	// ============================================================
	select {
	case err := <-serverErr:
		log.Error("Gateway server error", "error", err.Error())
		os.Exit(1)

	case sig := <-shutdown:
		log.Info("Gateway shutdown signal received", "signal", sig.String())
		ctx, cancel := context.WithTimeout(
			context.Background(),
			cfg.Server.ShutdownTimeout,
		)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Error("Gateway graceful shutdown failed", "error", err.Error())
			if err := server.Close(); err != nil {
				log.Error("Gateway forced shutdown failed", "error", err.Error())
			}
		}
		log.Info("Gateway stopped")
	}
}

// getEnv reads an env var with a fallback default
func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
