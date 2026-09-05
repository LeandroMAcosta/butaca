package store

import "strings"

// Track is one audio or subtitle stream inside a media file.
type Track struct {
	ID     int64
	FileID int64
	Kind   string // audio | subtitle
	Lang   string
	Title  string
}

// ReplaceTracks swaps a file's tracks for a freshly probed set, so a re-scan
// never accumulates duplicates.
func (s *Store) ReplaceTracks(fileID int64, tracks []Track) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // rolled back only when Commit did not run

	if _, err := tx.Exec(`DELETE FROM tracks WHERE file_id = ?`, fileID); err != nil {
		return err
	}
	for _, t := range tracks {
		if t.Lang == "" {
			t.Lang = "und"
		}
		if _, err := tx.Exec(`
            INSERT INTO tracks (file_id, kind, lang, title) VALUES (?,?,?,?)
            ON CONFLICT(file_id, kind, lang, title) DO NOTHING`,
			fileID, t.Kind, t.Lang, t.Title); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Languages is what an item offers, deduplicated and ordered as found.
type Languages struct {
	Audio     []string
	Subtitles []string
}

// Empty reports whether nothing has been probed yet.
func (l Languages) Empty() bool { return len(l.Audio) == 0 && len(l.Subtitles) == 0 }

// Summary renders at most max languages, appending "+N" for the remainder.
// Disclosure Day has 10 audio and 44 subtitle tracks, which is exactly why a
// list view cannot print them all.
func Summary(langs []string, max int) string {
	if len(langs) == 0 {
		return "-"
	}
	if len(langs) <= max {
		return strings.Join(langs, " ")
	}
	return strings.Join(langs[:max], " ") + " +" + itoa(len(langs)-max)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// LanguagesForItem merges the tracks of every file an item has, plus the
// external subtitle files recorded alongside them.
func (s *Store) LanguagesForItem(itemID int64) (Languages, error) {
	var out Languages
	rows, err := s.db.Query(`
		SELECT DISTINCT t.kind, t.lang
		FROM tracks t JOIN files f ON f.id = t.file_id
		WHERE f.item_id = ?
		ORDER BY t.kind, t.lang`, itemID)
	if err != nil {
		return out, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	for rows.Next() {
		var kind, lang string
		if err := rows.Scan(&kind, &lang); err != nil {
			return out, err
		}
		key := kind + ":" + lang
		if seen[key] {
			continue
		}
		seen[key] = true
		if kind == "audio" {
			out.Audio = append(out.Audio, lang)
		} else {
			out.Subtitles = append(out.Subtitles, lang)
		}
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	// External .srt files count as available subtitles too.
	subRows, err := s.db.Query(`
		SELECT DISTINCT sub.lang FROM subtitles sub
		JOIN files f ON f.id = sub.file_id WHERE f.item_id = ?`, itemID)
	if err != nil {
		return out, err
	}
	defer subRows.Close()
	for subRows.Next() {
		var lang string
		if err := subRows.Scan(&lang); err != nil {
			return out, err
		}
		if !seen["subtitle:"+lang] {
			seen["subtitle:"+lang] = true
			out.Subtitles = append(out.Subtitles, lang)
		}
	}
	return out, subRows.Err()
}
