// Package migrate imports an existing Radarr or Sonarr catalog into butaca,
// without moving a single file. The media stays where it is; only the catalog
// is rebuilt.
package migrate

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Entry is one catalog row lifted from another application.
type Entry struct {
	Kind             string
	TMDBID           int64
	IMDBID           string
	Title            string
	OriginalTitle    string
	Year             int
	OriginalLanguage string // ISO 639-1
	Path             string
	FilePath         string // absolute; empty when the item has no file
	FileSize         int64
	Quality          string
	ReleaseGroup     string
}

// radarrLanguages maps Radarr's internal language enum to ISO 639-1. Radarr
// stores an integer, so the codes have to be reconstructed. The values here are
// confirmed against a live database: 1 English, 2 French, 4 German, 8 Japanese,
// 21 Korean.
var radarrLanguages = map[int]string{
	1: "en", 2: "fr", 3: "es", 4: "de", 5: "it", 6: "da", 7: "nl", 8: "ja",
	9: "is", 10: "zh", 11: "ru", 12: "pl", 13: "vi", 14: "sv", 15: "no",
	16: "fi", 17: "tr", 18: "pt", 19: "nl", 20: "el", 21: "ko", 22: "hu",
	23: "he", 24: "lt", 25: "cs", 26: "ar", 27: "hi", 28: "bg", 29: "ml",
	30: "uk", 31: "sk", 32: "th", 33: "pt", 34: "es", 35: "ro", 36: "lv",
	37: "fa", 38: "ca", 39: "hr", 40: "sr", 41: "bs", 42: "et", 43: "af",
	44: "bn", 45: "sl",
}

// DefaultRadarrDB is where Radarr keeps its database on macOS and Linux.
func DefaultRadarrDB() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "Radarr", "radarr.db")
}

func DefaultSonarrDB() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "Sonarr", "sonarr.db")
}

// FromRadarr reads every movie in a Radarr database. The file is opened
// read-only so a running Radarr is never disturbed.
func FromRadarr(dbPath string) ([]Entry, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("radarr database: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`
		SELECT mm.TmdbId, COALESCE(mm.ImdbId,''), mm.Title, COALESCE(mm.OriginalTitle,''),
		       COALESCE(mm.Year,0), COALESCE(mm.OriginalLanguage,0), COALESCE(m.Path,''),
		       COALESCE(f.RelativePath,''), COALESCE(f.Size,0),
		       COALESCE(f.Quality,''), COALESCE(f.ReleaseGroup,'')
		FROM Movies m
		JOIN MovieMetadata mm ON mm.Id = m.MovieMetadataId
		LEFT JOIN MovieFiles f ON f.MovieId = m.Id
		ORDER BY mm.Title`)
	if err != nil {
		return nil, fmt.Errorf("read Radarr movies: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var langID int
		var relPath string
		if err := rows.Scan(&e.TMDBID, &e.IMDBID, &e.Title, &e.OriginalTitle, &e.Year,
			&langID, &e.Path, &relPath, &e.FileSize, &e.Quality, &e.ReleaseGroup); err != nil {
			return nil, err
		}
		e.Kind = "movie"
		e.OriginalLanguage = radarrLanguages[langID]
		if relPath != "" && e.Path != "" {
			e.FilePath = filepath.Join(e.Path, relPath)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// FromSonarr reads series. Episode-level import lands with series support; for
// now this reports what is there so nothing is silently dropped.
func FromSonarr(dbPath string) ([]Entry, error) {
	if _, err := os.Stat(dbPath); err != nil {
		return nil, fmt.Errorf("sonarr database: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(`SELECT COALESCE(TvdbId,0), Title, COALESCE(Year,0), COALESCE(Path,'') FROM Series ORDER BY Title`)
	if err != nil {
		return nil, fmt.Errorf("read Sonarr series: %w", err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var tvdb int64
		if err := rows.Scan(&tvdb, &e.Title, &e.Year, &e.Path); err != nil {
			return nil, err
		}
		e.Kind = "series"
		out = append(out, e)
	}
	return out, rows.Err()
}
