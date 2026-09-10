package store

// MoveItemPath points an item, its files and their subtitles at a new folder,
// after the folder itself has been renamed on disk. It is one transaction: a
// half-rewritten catalog would point at files that are no longer there.
//
// Prefixes are compared with SQLite's length(), which counts characters like
// substr() does, so titles such as "Amélie" rewrite correctly.
func (s *Store) MoveItemPath(itemID int64, from, to string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`UPDATE items SET path = ? WHERE id = ?`, to, itemID); err != nil {
		return err
	}
	oldPrefix, newPrefix := from+"/", to+"/"
	if _, err := tx.Exec(`
		UPDATE files SET path = ? || substr(path, length(?) + 1)
		WHERE item_id = ? AND substr(path, 1, length(?)) = ?`,
		newPrefix, oldPrefix, itemID, oldPrefix, oldPrefix); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE subtitles SET path = ? || substr(path, length(?) + 1)
		WHERE file_id IN (SELECT id FROM files WHERE item_id = ?)
		  AND substr(path, 1, length(?)) = ?`,
		newPrefix, oldPrefix, itemID, oldPrefix, oldPrefix); err != nil {
		return err
	}
	return tx.Commit()
}
