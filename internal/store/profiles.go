package store

import (
	"database/sql"
	"errors"
	"strings"
)

// Profile is a person's preferences. Empty fields mean "use the global config",
// so a profile only has to state what it wants to differ on.
type Profile struct {
	ID             int64
	Name           string
	LetterboxdUser string
	Resolutions    []string
	Sources        []string
	MinSize        string
	MaxSize        string
	MinSeeders     int
	LanguageMode   string
	PreferLanguage string
	SubtitleLangs  []string
	IsDefault      bool
}

func splitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinList(v []string) string { return strings.Join(v, ",") }

const profileCols = `id, name, COALESCE(letterboxd_user,''), COALESCE(resolutions,''),
	COALESCE(sources,''), COALESCE(min_size,''), COALESCE(max_size,''), min_seeders,
	language_mode, COALESCE(prefer_language,''), COALESCE(subtitle_langs,''), is_default`

func scanProfile(sc interface{ Scan(...any) error }) (*Profile, error) {
	var p Profile
	var res, src, subs string
	var def int
	err := sc.Scan(&p.ID, &p.Name, &p.LetterboxdUser, &res, &src, &p.MinSize, &p.MaxSize,
		&p.MinSeeders, &p.LanguageMode, &p.PreferLanguage, &subs, &def)
	if err != nil {
		return nil, err
	}
	p.Resolutions, p.Sources, p.SubtitleLangs = splitList(res), splitList(src), splitList(subs)
	p.IsDefault = def != 0
	return &p, nil
}

func (s *Store) SaveProfile(p *Profile) (int64, error) {
	if p.LanguageMode == "" {
		p.LanguageMode = "original"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // rolled back only when Commit did not run

	// Exactly one default, always: promoting one demotes the rest.
	if p.IsDefault {
		if _, err := tx.Exec(`UPDATE profiles SET is_default = 0`); err != nil {
			return 0, err
		}
	}
	if _, err := tx.Exec(`
        INSERT INTO profiles (name, letterboxd_user, resolutions, sources, min_size, max_size,
                              min_seeders, language_mode, prefer_language, subtitle_langs, is_default)
        VALUES (?,?,?,?,?,?,?,?,?,?,?)
        ON CONFLICT(name) DO UPDATE SET
            letterboxd_user=excluded.letterboxd_user, resolutions=excluded.resolutions,
            sources=excluded.sources, min_size=excluded.min_size, max_size=excluded.max_size,
            min_seeders=excluded.min_seeders, language_mode=excluded.language_mode,
            prefer_language=excluded.prefer_language, subtitle_langs=excluded.subtitle_langs,
            is_default=excluded.is_default`,
		p.Name, p.LetterboxdUser, joinList(p.Resolutions), joinList(p.Sources),
		p.MinSize, p.MaxSize, p.MinSeeders, p.LanguageMode, p.PreferLanguage,
		joinList(p.SubtitleLangs), boolInt(p.IsDefault)); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(`SELECT id FROM profiles WHERE name = ?`, p.Name).Scan(&id); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func (s *Store) ListProfiles() ([]*Profile, error) {
	rows, err := s.db.Query(`SELECT ` + profileCols + ` FROM profiles ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetProfile(id int64) (*Profile, error) {
	p, err := scanProfile(s.db.QueryRow(`SELECT `+profileCols+` FROM profiles WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// FindProfile matches on name, case-insensitively.
func (s *Store) FindProfile(name string) (*Profile, error) {
	p, err := scanProfile(s.db.QueryRow(`SELECT `+profileCols+` FROM profiles WHERE name = ? COLLATE NOCASE`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// DefaultProfile returns the profile marked default, or nil when there is none.
func (s *Store) DefaultProfile() (*Profile, error) {
	p, err := scanProfile(s.db.QueryRow(`SELECT ` + profileCols + ` FROM profiles WHERE is_default = 1 LIMIT 1`))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (s *Store) DeleteProfile(id int64) error {
	_, err := s.db.Exec(`DELETE FROM profiles WHERE id = ?`, id)
	return err
}

// AssignProfile points an item at a profile; pass 0 to clear it.
func (s *Store) AssignProfile(itemID, profileID int64) error {
	var v any
	if profileID > 0 {
		v = profileID
	}
	_, err := s.db.Exec(`UPDATE items SET profile_id = ? WHERE id = ?`, v, itemID)
	return err
}
