package repository

import (
	"database/sql"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3"

	"github.com/darkodi/url-shortener/internal/config"
	"github.com/darkodi/url-shortener/internal/model"
)

var ErrNotFound = errors.New("record not found")

// URLRepository handles database operations
type URLRepository struct {
	primary  *sql.DB   // Write operations
	replicas []*sql.DB // Read operations
	rrIndex  uint32    // Round-robin index
	driver   string    // "postgres" or "sqlite3"
}

// NewURLRepository creates repository from config
func NewURLRepository(cfg *config.DatabaseConfig) (*URLRepository, error) {
	var primary *sql.DB
	var replicas []*sql.DB
	var err error

	// ============ OPEN PRIMARY DATABASE ============
	if cfg.Driver == "postgres" {
		primaryConn := cfg.BuildPostgresConnectionString(cfg.Host)
		primary, err = openPostgres(primaryConn, cfg.MaxOpenConns, cfg.MaxIdleConns)
		if err != nil {
			return nil, fmt.Errorf("failed to open primary database: %w", err)
		}

		// Initialize schema
		if err := initPostgresSchema(primary); err != nil {
			primary.Close()
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}

		// ============ OPEN REPLICA DATABASES ============
		replicas = make([]*sql.DB, 0, len(cfg.ReplicaHosts))
		for i, replicaHost := range cfg.ReplicaHosts {
			replicaConn := cfg.BuildPostgresConnectionString(replicaHost)
			replica, err := openPostgres(replicaConn, cfg.MaxOpenConns, cfg.MaxIdleConns)
			if err != nil {
				// Close already opened connections
				primary.Close()
				for _, r := range replicas {
					r.Close()
				}
				return nil, fmt.Errorf("failed to open replica %d: %w", i, err)
			}
			replicas = append(replicas, replica)
		}
	} else {
		// SQLite fallback (for backward compatibility)
		primary, err = openSQLite(cfg.Path, cfg.MaxOpenConns, cfg.MaxIdleConns)
		if err != nil {
			return nil, fmt.Errorf("failed to open SQLite database: %w", err)
		}
		if err := initSQLiteSchema(primary); err != nil {
			primary.Close()
			return nil, fmt.Errorf("failed to initialize schema: %w", err)
		}
	}

	repo := &URLRepository{
		primary:  primary,
		replicas: replicas,
		rrIndex:  0,
		driver:   cfg.Driver,
	}

	fmt.Printf("Database initialized: %s (1 primary + %d replicas)\n",
		cfg.Driver, len(replicas))
	return repo, nil
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
// READ OPERATIONS (use replicas if available)
// ============================================================

func (r *URLRepository) getReadDB() *sql.DB {
	if len(r.replicas) == 0 {
		return r.primary
	}

	// Round-robin across replicas
	idx := atomic.AddUint32(&r.rrIndex, 1)
	return r.replicas[idx%uint32(len(r.replicas))]
}

// GetByShortCode retrieves a URL by short code
func (r *URLRepository) GetByShortCode(shortCode string) (*model.URL, error) {
	db := r.getReadDB()

	query := `SELECT id, short_code, original_url, created_at, click_count 
	          FROM urls WHERE short_code = $1`

	// SQLite uses ? instead of $1
	if r.driver == "sqlite3" {
		query = `SELECT id, short_code, original_url, created_at, click_count 
		         FROM urls WHERE short_code = ?`
	}

	var url model.URL
	err := db.QueryRow(query, shortCode).Scan(
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
// WRITE OPERATIONS (always primary)
// ============================================================

// Create inserts a new URL
func (r *URLRepository) Create(url *model.URL) error {
	query := `INSERT INTO urls (short_code, original_url) VALUES ($1, $2) RETURNING id`

	if r.driver == "sqlite3" {
		// SQLite doesn't support RETURNING
		query = `INSERT INTO urls (short_code, original_url) VALUES (?, ?)`
		result, err := r.primary.Exec(query, url.ShortCode, url.OriginalURL)
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
	err := r.primary.QueryRow(query, url.ShortCode, url.OriginalURL).Scan(&url.ID)
	return err
}

// IncrementClickCount increments click counter
func (r *URLRepository) IncrementClickCount(shortCode string) error {
	query := `UPDATE urls SET click_count = click_count + 1 WHERE short_code = $1`

	if r.driver == "sqlite3" {
		query = `UPDATE urls SET click_count = click_count + 1 WHERE short_code = ?`
	}

	_, err := r.primary.Exec(query, shortCode)
	return err
}

// GetNextID returns next available ID
func (r *URLRepository) GetNextID() (uint64, error) {
	var maxID sql.NullInt64
	query := `SELECT MAX(id) FROM urls`

	err := r.primary.QueryRow(query).Scan(&maxID)
	if err != nil {
		return 0, err
	}

	if !maxID.Valid {
		return 1, nil
	}

	return uint64(maxID.Int64) + 1, nil
}

// ============================================================
// LIFECYCLE
// ============================================================

func (r *URLRepository) Close() error {
	var errs []error

	if err := r.primary.Close(); err != nil {
		errs = append(errs, fmt.Errorf("primary: %w", err))
	}

	for i, replica := range r.replicas {
		if err := replica.Close(); err != nil {
			errs = append(errs, fmt.Errorf("replica %d: %w", i, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %v", errs)
	}
	return nil
}
