package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration
type Config struct {
	Server    ServerConfig
	Database  DatabaseConfig
	App       AppConfig
	Log       LogConfig
	RateLimit RateLimitConfig
	Redis     RedisConfig
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

// DatabaseConfig holds database settings
type DatabaseConfig struct {
	// Common settings
	Driver       string // "postgres" or "sqlite3"
	MaxOpenConns int
	MaxIdleConns int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// SQLite settings (keep for backward compatibility)
	Path string

	// PostgreSQL settings
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string

	// for Read replicas
	ReplicaHosts []string // Replica hostnames
}

// AppConfig holds application-specific settings
type AppConfig struct {
	BaseURL     string
	Environment string // "development", "production"
}

type LogConfig struct {
	Level       string
	Format      string
	Environment string
}

type RateLimitConfig struct {
	Enabled  bool
	Rate     int           // Requests per interval
	Burst    int           // Max burst
	Interval time.Duration // Refill interval
	Cleanup  time.Duration // Cleanup interval
}

type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnv("PORT", "8080"),
			ReadTimeout:     getDurationEnv("SERVER_READ_TIMEOUT", 15*time.Second),
			WriteTimeout:    getDurationEnv("SERVER_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     getDurationEnv("SERVER_IDLE_TIMEOUT", 60*time.Second),
			ShutdownTimeout: getDurationEnv("SERVER_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
		Database: DatabaseConfig{
			Driver:       getEnv("DB_DRIVER", "postgres"), // Default to PostgreSQL
			MaxOpenConns: getIntEnv("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns: getIntEnv("DB_MAX_IDLE_CONNS", 5),
			ReadTimeout:  getDurationEnv("DB_READ_TIMEOUT", 5*time.Second),
			WriteTimeout: getDurationEnv("DB_WRITE_TIMEOUT", 10*time.Second),

			// SQLite (legacy)
			Path: getEnv("DB_PATH", "./data/urls.db"),

			// PostgreSQL
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "urlshortener"),
			Password: getEnv("DB_PASSWORD", "password"),
			DBName:   getEnv("DB_NAME", "urlshortener"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),

			// Read replicas
			ReplicaHosts: getSliceEnv("DB_REPLICA_HOSTS", []string{}),
		},
		App: AppConfig{
			BaseURL:     getEnv("BASE_URL", ""),
			Environment: getEnv("ENVIRONMENT", "development"),
		},
		Log: LogConfig{
			Level:       getEnv("LOG_LEVEL", "info"),
			Format:      getEnv("LOG_FORMAT", "text"),
			Environment: getEnv("ENVIRONMENT", "development"),
		},
		RateLimit: RateLimitConfig{
			Enabled:  getBoolEnv("RATE_LIMIT_ENABLED", true),
			Rate:     getIntEnv("RATE_LIMIT_RATE", 10),
			Burst:    getIntEnv("RATE_LIMIT_BURST", 20),
			Interval: getDurationEnv("RATE_LIMIT_INTERVAL", time.Second),
			Cleanup:  getDurationEnv("RATE_LIMIT_CLEANUP", 5*time.Minute),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnv("REDIS_PORT", "6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getIntEnv("REDIS_DB", 0),
		},
	}

	// Set default BaseURL if not provided
	if cfg.App.BaseURL == "" {
		cfg.App.BaseURL = fmt.Sprintf("http://localhost:%s", cfg.Server.Port)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Creates PostgreSQL connection string
func (d *DatabaseConfig) BuildPostgresConnectionString(host string) string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate port
	port, err := strconv.Atoi(c.Server.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %s (must be 1-65535)", c.Server.Port)
	}

	// Validate database path
	if c.Database.Path == "" {
		return errors.New("database path cannot be empty")
	}

	// Validate environment
	validEnvs := map[string]bool{
		"development": true,
		"production":  true,
		"testing":     true,
	}
	if !validEnvs[c.App.Environment] {
		return fmt.Errorf("invalid environment: %s (must be development, production, or testing)", c.App.Environment)
	}
	// Validate log level
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}
	if !validLevels[c.Log.Level] {
		return fmt.Errorf("invalid log level: %s", c.Log.Level)
	}

	return nil
}

// IsDevelopment returns true if running in development mode
func (c *Config) IsDevelopment() bool {
	return c.App.Environment == "development"
}

// IsProduction returns true if running in production mode
func (c *Config) IsProduction() bool {
	return c.App.Environment == "production"
}

// ============================================================
// HELPER FUNCTIONS
// ============================================================

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue
	}
	return duration
}
func getIntEnv(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	intValue, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return intValue
}
func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
func getSliceEnv(key string, defaultValue []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	// Split by comma
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
