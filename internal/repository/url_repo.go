package repository

import (
	"database/sql"
	"errors"

	"github.com/darkodi/url-shortener/internal/model"
	_ "github.com/mattn/go-sqlite3"
)

var ErrNotFound = errors.New("url not found")

type URLRepository struct {
	db *sql.DB
}

func NewURLRepository(dbPath string) (*URLRepository, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Create table if not exists
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS urls (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            short_code TEXT UNIQUE NOT NULL,
            original_url TEXT NOT NULL,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
            click_count INTEGER DEFAULT 0
        )
    `)
	if err != nil {
		return nil, err
	}

	return &URLRepository{db: db}, nil
}

func (r *URLRepository) Create(url *model.URL) error {
	result, err := r.db.Exec(
		"INSERT INTO urls (short_code, original_url) VALUES (?, ?)",
		url.ShortCode, url.OriginalURL,
	)
	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()
	url.ID = uint64(id)
	return nil
}

func (r *URLRepository) GetByShortCode(shortCode string) (*model.URL, error) {
	url := &model.URL{}
	err := r.db.QueryRow(
		"SELECT id, short_code, original_url, created_at, click_count FROM urls WHERE short_code = ?",
		shortCode,
	).Scan(&url.ID, &url.ShortCode, &url.OriginalURL, &url.CreatedAt, &url.ClickCount)

	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return url, err
}

func (r *URLRepository) IncrementClickCount(shortCode string) error {
	_, err := r.db.Exec(
		"UPDATE urls SET click_count = click_count + 1 WHERE short_code = ?",
		shortCode,
	)
	return err
}

func (r *URLRepository) GetNextID() (uint64, error) {
	var maxID sql.NullInt64
	err := r.db.QueryRow("SELECT MAX(id) FROM urls").Scan(&maxID)
	if err != nil {
		return 1, err
	}
	if !maxID.Valid {
		return 1, nil
	}
	return uint64(maxID.Int64) + 1, nil
}
