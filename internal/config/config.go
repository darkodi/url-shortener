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
	Database  DatabaseConfig // Kept for backward compatibility/non-sharded use
	Shards    []ShardConfig  // List of database shards
	NumShards int            // Total number of shards
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

// DatabaseConfig holds database settings.
// This struct represents a single database instance (either a primary or a replica).
type DatabaseConfig struct {
	// Common settings
	Driver       string // "postgres" or "sqlite3"
	MaxOpenConns int
	MaxIdleConns int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// SQLite settings (for backward compatibility)
	Path string

	// PostgreSQL settings
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// ShardConfig represents a single database shard, containing its primary and replica connections
type ShardConfig struct {
	ShardID  int // 0-indexed shard identifier
	Primary  DatabaseConfig
	Replicas []DatabaseConfig
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
		// cfg.Database for backward compatibility.
		// Its values will reflect the *first* shard's primary if DB_HOST/DB_PORT are not explicitly set,
		// or the legacy single-DB configuration if sharding is not used.
		Database: DatabaseConfig{
			Driver:       getEnv("DB_DRIVER", "postgres"), // Default to PostgreSQL
			MaxOpenConns: getIntEnv("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns: getIntEnv("DB_MAX_IDLE_CONNS", 5),
			ReadTimeout:  getDurationEnv("DB_READ_TIMEOUT", 5*time.Second),
			WriteTimeout: getDurationEnv("DB_WRITE_TIMEOUT", 10*time.Second),

			// SQLite (legacy)
			Path: getEnv("DB_PATH", "./data/urls.db"),

			// PostgreSQL (legacy/single-DB settings - will be overridden by shard-specific for sharding)
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "urlshortener"),
			Password: getEnv("DB_PASSWORD", "password"),
			DBName:   getEnv("DB_NAME", "urlshortener"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
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

	// --- Load sharding configuration ---
	numShards := getIntEnv("DB_NUM_SHARDS", 3) // Default to 3 shards
	cfg.NumShards = numShards
	cfg.Shards = make([]ShardConfig, numShards)

	for i := 0; i < numShards; i++ {
		shardIDForEnv := i + 1 // Use 1-based indexing for env var names (DB_SHARD1_...)

		// Primary DB Config for this shard - inheriting common settings from cfg.Database
		primaryConfig := DatabaseConfig{
			Driver:       cfg.Database.Driver,
			MaxOpenConns: cfg.Database.MaxOpenConns,
			MaxIdleConns: cfg.Database.MaxIdleConns,
			ReadTimeout:  cfg.Database.ReadTimeout,
			WriteTimeout: cfg.Database.WriteTimeout,
			User:         cfg.Database.User,
			Password:     cfg.Database.Password,
			DBName:       cfg.Database.DBName,
			SSLMode:      cfg.Database.SSLMode,
			Host:         getShardEnv(shardIDForEnv, "DB_SHARD%d_PRIMARY_HOST", fmt.Sprintf("postgres-shard%d-primary", shardIDForEnv)),
			Port:         getShardEnv(shardIDForEnv, "DB_SHARD%d_PRIMARY_PORT", fmt.Sprintf("%d", 5432+(i*10))), // Default port increment
		}

		// Replica DB Configs for this shard - inheriting common settings
		var replicas []DatabaseConfig
		replica1Config := DatabaseConfig{
			Driver:       cfg.Database.Driver,
			MaxOpenConns: cfg.Database.MaxOpenConns,
			MaxIdleConns: cfg.Database.MaxIdleConns,
			ReadTimeout:  cfg.Database.ReadTimeout,
			WriteTimeout: cfg.Database.WriteTimeout,
			User:         cfg.Database.User,
			Password:     cfg.Database.Password,
			DBName:       cfg.Database.DBName,
			SSLMode:      cfg.Database.SSLMode,
			Host:         getShardEnv(shardIDForEnv, "DB_SHARD%d_REPLICA1_HOST", fmt.Sprintf("postgres-shard%d-replica1", shardIDForEnv)),
			Port:         getShardEnv(shardIDForEnv, "DB_SHARD%d_REPLICA1_PORT", fmt.Sprintf("%d", 5433+(i*10))),
		}
		replicas = append(replicas, replica1Config)

		replica2Config := DatabaseConfig{
			Driver:       cfg.Database.Driver,
			MaxOpenConns: cfg.Database.MaxOpenConns,
			MaxIdleConns: cfg.Database.MaxIdleConns,
			ReadTimeout:  cfg.Database.ReadTimeout,
			WriteTimeout: cfg.Database.WriteTimeout,
			User:         cfg.Database.User,
			Password:     cfg.Database.Password,
			DBName:       cfg.Database.DBName,
			SSLMode:      cfg.Database.SSLMode,
			Host:         getShardEnv(shardIDForEnv, "DB_SHARD%d_REPLICA2_HOST", fmt.Sprintf("postgres-shard%d-replica2", shardIDForEnv)),
			Port:         getShardEnv(shardIDForEnv, "DB_SHARD%d_REPLICA2_PORT", fmt.Sprintf("%d", 5434+(i*10))),
		}
		replicas = append(replicas, replica2Config)

		cfg.Shards[i] = ShardConfig{
			ShardID:  i, // 0-indexed
			Primary:  primaryConfig,
			Replicas: replicas,
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Creates PostgreSQL connection string for this specific DatabaseConfig instance
// No longer takes 'host' argument, uses d.Host and d.Port
func (d *DatabaseConfig) BuildPostgresConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.DBName, d.SSLMode,
	)
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate server port
	port, err := strconv.Atoi(c.Server.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid server port: %s (must be 1-65535)", c.Server.Port)
	}

	// Validate database path (for SQLite, if applicable)
	if c.Database.Driver == "sqlite3" && c.Database.Path == "" {
		return errors.New("database path cannot be empty for SQLite")
	}

	// --- Validate sharding configuration ---
	if c.NumShards <= 0 {
		return errors.New("number of shards (DB_NUM_SHARDS) must be greater than 0")
	}
	if len(c.Shards) != c.NumShards {
		return fmt.Errorf("configured number of shards (%d) does not match loaded shards (%d)", c.NumShards, len(c.Shards))
	}

	for i, shard := range c.Shards {
		if shard.Primary.Host == "" || shard.Primary.Port == "" {
			return fmt.Errorf("shard %d: primary host and port are required", i)
		}
		if len(shard.Replicas) == 0 {
			return fmt.Errorf("shard %d: at least one replica is required", i)
		}
		for j, replica := range shard.Replicas {
			if replica.Host == "" || replica.Port == "" {
				return fmt.Errorf("shard %d, replica %d: host and port are required", i, j)
			}
		}
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

// Retrieves an environment variable for a specific shard, using a pattern.
// Example: getShardEnv(1, "DB_SHARD%d_PRIMARY_HOST", "default-host") will look for DB_SHARD1_PRIMARY_HOST
func getShardEnv(shardID int, keyPattern, defaultValue string) string {
	key := fmt.Sprintf(keyPattern, shardID)
	return getEnv(key, defaultValue)
}
