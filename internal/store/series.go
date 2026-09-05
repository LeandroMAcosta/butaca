package store

import (
	"database/sql"
	"errors"
)

type Season struct {
	ID        int64
	ItemID    int64
	Number    int
	Monitored bool
}

type Episode struct {
	ID        int64
	SeasonID  int64
	Season    int
	Number    int
	Title     string
	AirDate   string
	Monitored bool
	HasFile   bool
}

func (s *Store) AddSeason(itemID int64, number int, monitored bool) (int64, error) {
	if _, err := s.db.Exec(`
        INSERT INTO seasons (item_id, number, monitored) VALUES (?,?,?)
        ON CONFLICT(item_id, number) DO UPDATE SET monitored=excluded.monitored`,
		itemID, number, boolInt(monitored)); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM seasons WHERE item_id=? AND number=?`, itemID, number).Scan(&id)
	return id, err
}

func (s *Store) AddEpisode(seasonID int64, number int, title, airDate string, monitored bool) (int64, error) {
	if _, err := s.db.Exec(`
        INSERT INTO episodes (season_id, number, title, air_date, monitored) VALUES (?,?,?,?,?)
        ON CONFLICT(season_id, number) DO UPDATE SET title=excluded.title, air_date=excluded.air_date`,
		seasonID, number, title, airDate, boolInt(monitored)); err != nil {
		return 0, err
	}
	var id int64
	err := s.db.QueryRow(`SELECT id FROM episodes WHERE season_id=? AND number=?`, seasonID, number).Scan(&id)
	return id, err
}

// EpisodesForItem lists every episode of a series with whether it has a file.
func (s *Store) EpisodesForItem(itemID int64) ([]*Episode, error) {
	rows, err := s.db.Query(`
		SELECT e.id, e.season_id, se.number, e.number, COALESCE(e.title,''),
		       COALESCE(e.air_date,''), e.monitored,
		       EXISTS(SELECT 1 FROM files f WHERE f.episode_id = e.id)
		FROM episodes e
		JOIN seasons se ON se.id = e.season_id
		WHERE se.item_id = ?
		ORDER BY se.number, e.number`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Episode
	for rows.Next() {
		var e Episode
		var mon, hasFile int
		if err := rows.Scan(&e.ID, &e.SeasonID, &e.Season, &e.Number, &e.Title, &e.AirDate, &mon, &hasFile); err != nil {
			return nil, err
		}
		e.Monitored, e.HasFile = mon != 0, hasFile != 0
		out = append(out, &e)
	}
	return out, rows.Err()
}

// FindEpisode locates one episode by series, season and number.
func (s *Store) FindEpisode(itemID int64, season, number int) (*Episode, error) {
	var e Episode
	var mon int
	err := s.db.QueryRow(`
		SELECT e.id, e.season_id, se.number, e.number, COALESCE(e.title,''), COALESCE(e.air_date,''), e.monitored
		FROM episodes e JOIN seasons se ON se.id = e.season_id
		WHERE se.item_id=? AND se.number=? AND e.number=?`, itemID, season, number).
		Scan(&e.ID, &e.SeasonID, &e.Season, &e.Number, &e.Title, &e.AirDate, &mon)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	e.Monitored = mon != 0
	return &e, err
}

// EpisodeByID fetches one episode with its season number.
func (s *Store) EpisodeByID(id int64) (*Episode, error) {
	var e Episode
	var mon int
	err := s.db.QueryRow(`
		SELECT e.id, e.season_id, se.number, e.number, COALESCE(e.title,''), COALESCE(e.air_date,''), e.monitored
		FROM episodes e JOIN seasons se ON se.id = e.season_id
		WHERE e.id = ?`, id).
		Scan(&e.ID, &e.SeasonID, &e.Season, &e.Number, &e.Title, &e.AirDate, &mon)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	e.Monitored = mon != 0
	return &e, err
}
