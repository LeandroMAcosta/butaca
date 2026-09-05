// Package store owns butaca's SQLite state: catalog, files, queue and history.
package store

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

var ErrNotFound = errors.New("not found")

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	// SQLite has no ADD COLUMN IF NOT EXISTS, so older databases are upgraded by
	// attempting each column and treating "duplicate column" as success.
	for _, alter := range []string{
		`ALTER TABLE items ADD COLUMN alt_titles TEXT`,
		`ALTER TABLE items ADD COLUMN state TEXT NOT NULL DEFAULT 'monitored'`,
		`ALTER TABLE items ADD COLUMN profile_id INTEGER`,
		`ALTER TABLE items ADD COLUMN rating REAL`,
		`ALTER TABLE items ADD COLUMN source TEXT`,
	} {
		if _, err := db.Exec(alter); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			db.Close()
			return nil, fmt.Errorf("migrate: %s: %w", alter, err)
		}
	}
	// Indexes on migrated columns must come after the ALTERs: on a database
	// created before `state` existed, schema.sql cannot index a missing column.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_items_state ON items(state)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("index items.state: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) AddSubtitle(fileID int64, lang, path string) (int64, error) {
	res, err := s.db.Exec(`
        INSERT INTO subtitles (file_id, lang, path) VALUES (?,?,?)
        ON CONFLICT(file_id, lang) DO UPDATE SET path=excluded.path`, fileID, lang, path)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ClearSubtitles removes a file's recorded subtitles, so a re-scan replaces
// them rather than accumulating.
func (s *Store) ClearSubtitles(fileID int64) error {
	_, err := s.db.Exec(`DELETE FROM subtitles WHERE file_id = ?`, fileID)
	return err
}

// HistoryEntry is one recorded event.
type HistoryEntry struct {
	At     string
	Event  string
	Detail string
}

// EventsByType returns matching history rows, newest first.
func (s *Store) EventsByType(events []string, limit int) ([]HistoryEntry, error) {
	if len(events) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 50
	}
	placeholders := make([]string, len(events))
	args := make([]any, 0, len(events)+1)
	for i, e := range events {
		placeholders[i] = "?"
		args = append(args, e)
	}
	args = append(args, limit)
	rows, err := s.db.Query(`
		SELECT at, event, COALESCE(detail,'') FROM history
		WHERE event IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		if err := rows.Scan(&h.At, &h.Event, &h.Detail); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
