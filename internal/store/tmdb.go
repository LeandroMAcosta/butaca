package store

// SetTMDB attaches a TMDB match to an item catalogued without one. Only the
// identity and the fields the caller passes change: title and path stay, so
// nothing on disk has to move.
func (s *Store) SetTMDB(itemID, tmdbID int64, year int, originalLanguage, altTitles string) error {
	_, err := s.db.Exec(`
		UPDATE items SET tmdb_id = ?, year = ?, original_language = ?, alt_titles = ?
		WHERE id = ?`, tmdbID, year, originalLanguage, altTitles, itemID)
	return err
}
