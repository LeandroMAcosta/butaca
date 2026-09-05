package store

import (
	"database/sql"
	"errors"
)

// CachedTMDBID returns a previously resolved Letterboxd slug. Resolving one
// costs an HTTP request and the answer never changes, so it is cached forever.
func (s *Store) CachedTMDBID(slug string) (int64, bool) {
	var id int64
	err := s.db.QueryRow(`SELECT COALESCE(tmdb_id,0) FROM letterboxd_films WHERE slug = ?`, slug).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) || err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

func (s *Store) CacheTMDBID(slug string, tmdbID int64, title string, year int) error {
	_, err := s.db.Exec(`
        INSERT INTO letterboxd_films (slug, tmdb_id, title, year) VALUES (?,?,?,?)
        ON CONFLICT(slug) DO UPDATE SET tmdb_id=excluded.tmdb_id, title=excluded.title, year=excluded.year`,
		slug, tmdbID, title, year)
	return err
}
