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
	// Databases created before alt_titles existed need the column added; SQLite
	// has no ADD COLUMN IF NOT EXISTS, so a duplicate error here is expected.
	if _, err := db.Exec(`ALTER TABLE items ADD COLUMN alt_titles TEXT`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column") {
		db.Close()
		return nil, fmt.Errorf("migrate items.alt_titles: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

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
		_, err = s.db.Exec(`
            UPDATE items SET title=?, year=?, original_language=?, alt_titles=?, path=?, monitored=?
            WHERE id=?`,
			it.Title, it.Year, it.OriginalLanguage, it.AltTitles, it.Path, boolInt(it.Monitored), existing)
		return existing, err
	}

	res, err := s.db.Exec(`
        INSERT INTO items (kind, tmdb_id, imdb_id, title, year, original_language, alt_titles, path, monitored)
        VALUES (?,?,?,?,?,?,?,?,?)`,
		it.Kind, tmdb, it.IMDBID, it.Title, it.Year, it.OriginalLanguage, it.AltTitles, it.Path, boolInt(it.Monitored))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

const itemCols = `i.id, i.kind, COALESCE(i.tmdb_id,0), COALESCE(i.imdb_id,''), i.title,
	COALESCE(i.year,0), COALESCE(i.original_language,''), COALESCE(i.alt_titles,''),
	COALESCE(i.path,''), i.monitored, i.added_at`

func scanItem(sc interface{ Scan(...any) error }) (*Item, error) {
	var it Item
	var mon int
	err := sc.Scan(&it.ID, &it.Kind, &it.TMDBID, &it.IMDBID, &it.Title, &it.Year,
		&it.OriginalLanguage, &it.AltTitles, &it.Path, &mon, &it.AddedAt, &it.FileCount, &it.SizeBytes)
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

func (s *Store) Enqueue(q *QueueEntry) error {
	_, err := s.db.Exec(`
        INSERT INTO queue (item_id, release_title, magnet, info_hash, state, size)
        VALUES (?,?,?,?,?,?)
        ON CONFLICT(info_hash) DO UPDATE SET state=excluded.state`,
		q.ItemID, q.ReleaseTitle, q.Magnet, q.InfoHash, q.State, q.Size)
	return err
}

func (s *Store) PendingQueue() ([]*QueueEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, item_id, release_title, COALESCE(magnet,''), info_hash, state, size, progress
		FROM queue WHERE state NOT IN ('imported','failed')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*QueueEntry
	for rows.Next() {
		var q QueueEntry
		if err := rows.Scan(&q.ID, &q.ItemID, &q.ReleaseTitle, &q.Magnet, &q.InfoHash, &q.State, &q.Size, &q.Progress); err != nil {
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

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
