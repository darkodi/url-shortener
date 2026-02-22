package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3"

	"github.com/darkodi/url-shortener/internal/config"
	"github.com/darkodi/url-shortener/internal/model"
	"github.com/darkodi/url-shortener/internal/sharding"
)

var ErrNotFound = errors.New("record not found")

// URLRepository uses ShardRouter
type URLRepository struct {
	router *sharding.ShardRouter // Manages connections to all shards
	driver string                // "postgres" or "sqlite3"
}

// NewURLRepository creates ShardRouter from full config
func NewURLRepository(cfg *config.Config) (*URLRepository, error) {
	// Check if sharding is enabled
	if cfg.NumShards > 0 && cfg.Database.Driver == "postgres" {
		// Create shard router
		router, err := sharding.NewShardRouter(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create shard router: %w", err)
		}

		// Initialize all shard connections
		if err := router.Initialize(); err != nil {
			return nil, fmt.Errorf("failed to initialize shard router: %w", err)
		}

		repo := &URLRepository{
			router: router,
			driver: cfg.Database.Driver,
		}

		// Run migrations on all shards
		if err := repo.initAllShardSchemas(); err != nil {
			router.Close()
			return nil, fmt.Errorf("failed to initialize schemas: %w", err)
		}

		fmt.Printf("Database initialized: %s (%d shards with replication)\n",
			cfg.Database.Driver, cfg.NumShards)
		return repo, nil
	}

	// FALLBACK: Legacy single-database mode (SQLite or single Postgres)
	return newLegacyRepository(&cfg.Database)
}

// Initialize schema on all shards
func (r *URLRepository) initAllShardSchemas() error {
	primaries, err := r.router.GetAllPrimaryDBs()
	if err != nil {
		return err
	}

	for i, db := range primaries {
		if err := initPostgresSchema(db); err != nil {
			return fmt.Errorf("failed to init schema on shard %d: %w", i, err)
		}
	}
	return nil
}

// Legacy repository for backward compatibility
func newLegacyRepository(cfg *config.DatabaseConfig) (*URLRepository, error) {
	// This is the old implementation for single database
	var primary *sql.DB
	var err error

	if cfg.Driver == "postgres" {
		primaryConn := cfg.BuildPostgresConnectionString()
		primary, err = openPostgres(primaryConn, cfg.MaxOpenConns, cfg.MaxIdleConns)
		if err != nil {
			return nil, fmt.Errorf("failed to open primary database: %w", err)
		}

		if err := initPostgresSchema(primary); err != nil {
			primary.Close()
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}
	} else {
		primary, err = openSQLite(cfg.Path, cfg.MaxOpenConns, cfg.MaxIdleConns)
		if err != nil {
			return nil, fmt.Errorf("failed to open SQLite database: %w", err)
		}
		if err := initSQLiteSchema(primary); err != nil {
			primary.Close()
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}
	}

	// Create a minimal ShardRouter with just one shard
	router := &sharding.ShardRouter{}
	// TODO: Implement legacy single-DB wrapper or keep old struct

	return &URLRepository{
		router: router,
		driver: cfg.Driver,
	}, nil
}

// ============================================================
// DATABASE CONNECTION HELPERS
// ============================================================

func openPostgres(connStr string, maxOpen, maxIdle int) (*sql.DB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func openSQLite(path string, maxOpen, maxIdle int) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

// ============================================================
// SCHEMA INITIALIZATION
// ============================================================

func initPostgresSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS urls (
		id BIGSERIAL PRIMARY KEY,
		short_code VARCHAR(20) UNIQUE NOT NULL,
		original_url TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		click_count BIGINT DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_short_code ON urls(short_code);
	`
	_, err := db.Exec(schema)
	return err
}

func initSQLiteSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS urls (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		short_code TEXT UNIQUE NOT NULL,
		original_url TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		click_count INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_short_code ON urls(short_code);
	`
	_, err := db.Exec(schema)
	return err
}

// ============================================================
// READ OPERATIONS
// ============================================================

// GetByShortCode uses ShardRouter for reads
func (r *URLRepository) GetByShortCode(shortCode string) (*model.URL, error) {
	// Get replica DB for this short code's shard
	db, err := r.router.GetReplicaDB(shortCode)
	if err != nil {
		return nil, fmt.Errorf("failed to get replica DB: %w", err)
	}

	query := `SELECT id, short_code, original_url, created_at, click_count 
	          FROM urls WHERE short_code = $1`

	// SQLite uses ? instead of $1
	if r.driver == "sqlite3" {
		query = `SELECT id, short_code, original_url, created_at, click_count 
		         FROM urls WHERE short_code = ?`
	}

	var url model.URL
	err = db.QueryRow(query, shortCode).Scan(
		&url.ID,
		&url.ShortCode,
		&url.OriginalURL,
		&url.CreatedAt,
		&url.ClickCount,
	)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &url, err
}

// ============================================================
// WRITE OPERATIONS
// ============================================================

// Create uses ShardRouter for writes
func (r *URLRepository) Create(url *model.URL) error {
	// Get primary DB for this short code's shard
	db, err := r.router.GetPrimaryDB(url.ShortCode)
	if err != nil {
		return fmt.Errorf("failed to get primary DB: %w", err)
	}

	query := `INSERT INTO urls (short_code, original_url) VALUES ($1, $2) RETURNING id`

	if r.driver == "sqlite3" {
		// SQLite doesn't support RETURNING
		query = `INSERT INTO urls (short_code, original_url) VALUES (?, ?)`
		result, err := db.Exec(query, url.ShortCode, url.OriginalURL)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		url.ID = uint64(id)
		return nil
	}

	// PostgreSQL with RETURNING
	err = db.QueryRow(query, url.ShortCode, url.OriginalURL).Scan(&url.ID)
	return err
}

// IncrementClickCount uses ShardRouter
func (r *URLRepository) IncrementClickCount(shortCode string) error {
	// Get primary DB for this short code's shard
	db, err := r.router.GetPrimaryDB(shortCode)
	if err != nil {
		return fmt.Errorf("failed to get primary DB: %w", err)
	}

	query := `UPDATE urls SET click_count = click_count + 1 WHERE short_code = $1`

	if r.driver == "sqlite3" {
		query = `UPDATE urls SET click_count = click_count + 1 WHERE short_code = ?`
	}

	_, err = db.Exec(query, shortCode)
	return err
}

// ============================================================
// LIFECYCLE
// ============================================================

// Close closes the ShardRouter
func (r *URLRepository) Close() error {
	if r.router != nil {
		return r.router.Close()
	}
	return nil
}

// GetShardRouter exposes the router for advanced operations
func (r *URLRepository) GetShardRouter() *sharding.ShardRouter {
	return r.router
}
