package store

func (s *Store) Enqueue(q *QueueEntry) error {
	_, err := s.db.Exec(`
        INSERT INTO queue (item_id, episode_id, release_title, magnet, info_hash, state, size)
        VALUES (?,?,?,?,?,?,?)
        ON CONFLICT(info_hash) DO UPDATE SET state=excluded.state`,
		q.ItemID, q.EpisodeID, q.ReleaseTitle, q.Magnet, q.InfoHash, q.State, q.Size)
	return err
}

func (s *Store) PendingQueue() ([]*QueueEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, item_id, episode_id, release_title, COALESCE(magnet,''), info_hash, state, size, progress
		FROM queue WHERE state NOT IN ('imported','failed')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*QueueEntry
	for rows.Next() {
		var q QueueEntry
		if err := rows.Scan(&q.ID, &q.ItemID, &q.EpisodeID, &q.ReleaseTitle, &q.Magnet, &q.InfoHash, &q.State, &q.Size, &q.Progress); err != nil {
			return nil, err
		}
		out = append(out, &q)
	}
	return out, rows.Err()
}

func (s *Store) SetQueueState(infoHash, state string, progress float64) error {
	_, err := s.db.Exec(`UPDATE queue SET state=?, progress=? WHERE info_hash=?`, state, progress, infoHash)
	return err
}

func (s *Store) Log(itemID int64, event, detail string) error {
	var id any
	if itemID > 0 {
		id = itemID
	}
	_, err := s.db.Exec(`INSERT INTO history (item_id, event, detail) VALUES (?,?,?)`, id, event, detail)
	return err
}
