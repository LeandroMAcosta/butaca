package store

import (
	"database/sql"
	"errors"
)

type Item struct {
	ID               int64
	Kind             string
	TMDBID           int64
	IMDBID           string
	Title            string
	Year             int
	OriginalLanguage string
	// AltTitles holds every other name the item is released under, newline
	// separated: TMDB's original_title plus anything added by hand.
	AltTitles string
	Path      string
	// State separates "go and get this" from "some day": a watchlist entry is
	// catalogued but never searched until it is promoted.
	State     string
	ProfileID int64
	Rating    float64
	Source    string
	Monitored bool
	AddedAt   string

	// Populated by list queries, not stored.
	FileCount int
	SizeBytes int64
}

type File struct {
	ID           int64
	ItemID       int64
	EpisodeID    *int64
	Path         string
	Size         int64
	Quality      string
	Source       string
	VideoCodec   string
	AudioCodec   string
	Languages    string
	ReleaseGroup string
}

type QueueEntry struct {
	ID           int64
	ItemID       int64
	EpisodeID    *int64
	ReleaseTitle string
	Magnet       string
	InfoHash     string
	State        string
	Size         int64
	Progress     float64
}

// AddItem inserts or updates a catalog entry.
//
// Identity is the TMDB id when there is one. Without a TMDB key every item
// would otherwise share tmdb_id 0 and a UNIQUE(kind, tmdb_id) constraint would
// make each add silently overwrite the last, so those rows are keyed on
// (kind, title, year) instead and stored with a NULL tmdb_id.
func (s *Store) AddItem(it *Item) (int64, error) {
	var (
		existing int64
		err      error
	)
	if it.TMDBID > 0 {
		err = s.db.QueryRow(`SELECT id FROM items WHERE kind=? AND tmdb_id=?`, it.Kind, it.TMDBID).Scan(&existing)
	} else {
		err = s.db.QueryRow(`SELECT id FROM items WHERE kind=? AND title=? AND COALESCE(year,0)=? AND tmdb_id IS NULL`,
			it.Kind, it.Title, it.Year).Scan(&existing)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	var tmdb any
	if it.TMDBID > 0 {
		tmdb = it.TMDBID
	}

	if existing > 0 {
		// State is deliberately not overwritten: re-importing a Letterboxd list
		// must not knock an item the user already promoted back to the watchlist.
		_, err = s.db.Exec(`
            UPDATE items SET title=?, year=?, original_language=?, alt_titles=?, path=?, monitored=?
            WHERE id=?`,
			it.Title, it.Year, it.OriginalLanguage, it.AltTitles, it.Path, boolInt(it.Monitored), existing)
		return existing, err
	}

	state := it.State
	if state == "" {
		state = StateMonitored
	}
	var profile any
	if it.ProfileID > 0 {
		profile = it.ProfileID
	}
	res, err := s.db.Exec(`
        INSERT INTO items (kind, tmdb_id, imdb_id, title, year, original_language, alt_titles,
                           path, monitored, state, profile_id, source)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		it.Kind, tmdb, it.IMDBID, it.Title, it.Year, it.OriginalLanguage, it.AltTitles,
		it.Path, boolInt(it.Monitored), state, profile, it.Source)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const itemCols = `i.id, i.kind, COALESCE(i.tmdb_id,0), COALESCE(i.imdb_id,''), i.title,
	COALESCE(i.year,0), COALESCE(i.original_language,''), COALESCE(i.alt_titles,''),
	COALESCE(i.path,''), i.monitored, i.added_at, COALESCE(i.state,'monitored'),
	COALESCE(i.profile_id,0), COALESCE(i.rating,0), COALESCE(i.source,'')`

func scanItem(sc interface{ Scan(...any) error }) (*Item, error) {
	var it Item
	var mon int
	err := sc.Scan(&it.ID, &it.Kind, &it.TMDBID, &it.IMDBID, &it.Title, &it.Year,
		&it.OriginalLanguage, &it.AltTitles, &it.Path, &mon, &it.AddedAt,
		&it.State, &it.ProfileID, &it.Rating, &it.Source, &it.FileCount, &it.SizeBytes)
	if err != nil {
		return nil, err
	}
	it.Monitored = mon != 0
	return &it, nil
}

const listQuery = `
	SELECT ` + itemCols + `,
	       (SELECT COUNT(*) FROM files f WHERE f.item_id = i.id),
	       (SELECT COALESCE(SUM(f.size),0) FROM files f WHERE f.item_id = i.id)
	FROM items i`

// Item states.
const (
	StateWatchlist   = "watchlist"
	StateMonitored   = "monitored"
	StateUnmonitored = "unmonitored"
)

// SetState moves an item between the watchlist and active monitoring.
func (s *Store) SetState(id int64, state string) error {
	_, err := s.db.Exec(`UPDATE items SET state = ? WHERE id = ?`, state, id)
	return err
}

// ListByState returns items in one state, or all of them when state is empty.
func (s *Store) ListByState(state string) ([]*Item, error) {
	q := listQuery
	args := []any{}
	if state != "" {
		q += ` WHERE COALESCE(i.state,'monitored') = ?`
		args = append(args, state)
	}
	q += ` ORDER BY i.title`
	return s.queryItems(q, args...)
}

func (s *Store) queryItems(q string, args ...any) ([]*Item, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) ListItems(kind string) ([]*Item, error) {
	q := listQuery
	args := []any{}
	if kind != "" {
		q += ` WHERE i.kind = ?`
		args = append(args, kind)
	}
	q += ` ORDER BY i.title`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) GetItem(id int64) (*Item, error) {
	it, err := scanItem(s.db.QueryRow(listQuery+` WHERE i.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return it, err
}

// FindItems matches on a title substring, case-insensitive.
func (s *Store) FindItems(term string) ([]*Item, error) {
	rows, err := s.db.Query(listQuery+` WHERE i.title LIKE ? COLLATE NOCASE ORDER BY i.title`, "%"+term+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (s *Store) DeleteItem(id int64) error {
	_, err := s.db.Exec(`DELETE FROM items WHERE id = ?`, id)
	return err
}

func (s *Store) AddFile(f *File) (int64, error) {
	res, err := s.db.Exec(`
        INSERT INTO files (item_id, episode_id, path, size, quality, source, video_codec, audio_codec, languages, release_group)
        VALUES (?,?,?,?,?,?,?,?,?,?)
        ON CONFLICT(path) DO UPDATE SET size=excluded.size, quality=excluded.quality`,
		f.ItemID, f.EpisodeID, f.Path, f.Size, f.Quality, f.Source, f.VideoCodec, f.AudioCodec, f.Languages, f.ReleaseGroup)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) FilesForItem(itemID int64) ([]*File, error) {
	rows, err := s.db.Query(`
		SELECT id, item_id, path, size, COALESCE(quality,''), COALESCE(source,''),
		       COALESCE(video_codec,''), COALESCE(audio_codec,''), COALESCE(languages,''), COALESCE(release_group,'')
		FROM files WHERE item_id = ?`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.ItemID, &f.Path, &f.Size, &f.Quality, &f.Source,
			&f.VideoCodec, &f.AudioCodec, &f.Languages, &f.ReleaseGroup); err != nil {
			return nil, err
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

// FindByTMDB looks an item up by its TMDB id, returning nil when absent.
func (s *Store) FindByTMDB(kind string, tmdbID int64) (*Item, error) {
	if tmdbID <= 0 {
		return nil, nil
	}
	it, err := scanItem(s.db.QueryRow(listQuery+` WHERE i.kind = ? AND i.tmdb_id = ?`, kind, tmdbID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return it, err
}
