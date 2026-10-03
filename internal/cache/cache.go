package cache

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Cache struct {
	db *sql.DB
}

func Open(dbPath string) (*Cache, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS artwork_cache (
		artist TEXT NOT NULL,
		album TEXT NOT NULL,
		has_artwork INTEGER NOT NULL,
		video_url TEXT,
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (artist, album)
	);
	CREATE INDEX IF NOT EXISTS idx_artist_album ON artwork_cache(artist, album);
	`

	if _, err := db.Exec(createTableQuery); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &Cache{db: db}, nil
}

func (c *Cache) Close() error {
	if c.db != nil {
		return c.db.Close()
	}
	return nil
}

func normalizeKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func (c *Cache) Get(artist, album string) (found bool, hasArtwork bool, videoURL string, err error) {
	row := c.db.QueryRow(
		"SELECT has_artwork, video_url FROM artwork_cache WHERE artist = ? AND album = ?",
		normalizeKey(artist), normalizeKey(album),
	)

	var hasArt int
	var vURL sql.NullString
	if err := row.Scan(&hasArt, &vURL); err != nil {
		if err == sql.ErrNoRows {
			return false, false, "", nil
		}
		return false, false, "", err
	}

	return true, hasArt == 1, vURL.String, nil
}

func (c *Cache) SetNegative(artist, album string) error {
	_, err := c.db.Exec(`
		INSERT INTO artwork_cache (artist, album, has_artwork, video_url, updated_at)
		VALUES (?, ?, 0, '', ?)
		ON CONFLICT(artist, album) DO UPDATE SET
			has_artwork = 0,
			video_url = '',
			updated_at = excluded.updated_at
	`, normalizeKey(artist), normalizeKey(album), time.Now())
	return err
}

func (c *Cache) SetPositive(artist, album, videoURL string) error {
	_, err := c.db.Exec(`
		INSERT INTO artwork_cache (artist, album, has_artwork, video_url, updated_at)
		VALUES (?, ?, 1, ?, ?)
		ON CONFLICT(artist, album) DO UPDATE SET
			has_artwork = 1,
			video_url = excluded.video_url,
			updated_at = excluded.updated_at
	`, normalizeKey(artist), normalizeKey(album), videoURL, time.Now())
	return err
}
