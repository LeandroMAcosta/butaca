package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/migrate"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

type MigrateReport struct {
	Imported []string
	Skipped  []string
	Missing  []string // catalogued but the file is not on disk
}

// ImportEntries brings another application's catalog into butaca. Files are
// left exactly where they are: only the catalog is rebuilt, so this is safe to
// run while the old stack is still installed.
func (a *App) ImportEntries(entries []migrate.Entry, dryRun bool) (*MigrateReport, error) {
	rep := &MigrateReport{}

	for _, e := range entries {
		if e.Kind != "movie" {
			rep.Skipped = append(rep.Skipped,
				fmt.Sprintf("%s (%s): series import needs series support", e.Title, e.Kind))
			continue
		}

		label := e.Title
		if e.Year > 0 {
			label = fmt.Sprintf("%s (%d)", e.Title, e.Year)
		}

		var alts []string
		if e.OriginalTitle != "" && e.OriginalTitle != e.Title {
			alts = append(alts, e.OriginalTitle)
		}

		if dryRun {
			note := label
			if e.FilePath == "" {
				note += "  [no file]"
			}
			rep.Imported = append(rep.Imported, note)
			continue
		}

		id, err := a.Store.AddItem(&store.Item{
			Kind:             "movie",
			TMDBID:           e.TMDBID,
			IMDBID:           e.IMDBID,
			Title:            e.Title,
			Year:             e.Year,
			OriginalLanguage: e.OriginalLanguage,
			AltTitles:        strings.Join(alts, "\n"),
			Path:             e.Path,
			Monitored:        true,
		})
		if err != nil {
			return rep, fmt.Errorf("import %s: %w", label, err)
		}

		if e.FilePath == "" {
			rep.Imported = append(rep.Imported, label+"  [no file]")
			continue
		}
		fi, err := os.Stat(e.FilePath)
		if err != nil {
			rep.Missing = append(rep.Missing, fmt.Sprintf("%s: %s", label, e.FilePath))
			continue
		}
		size := e.FileSize
		if size == 0 {
			size = fi.Size()
		}
		if _, err := a.Store.AddFile(&store.File{
			ItemID:       id,
			Path:         e.FilePath,
			Size:         size,
			Quality:      e.Quality,
			ReleaseGroup: e.ReleaseGroup,
		}); err != nil {
			return rep, fmt.Errorf("record file for %s: %w", label, err)
		}
		_ = a.Store.Log(id, "migrated", e.FilePath)
		rep.Imported = append(rep.Imported, label)
	}
	return rep, nil
}

// Orphans lists download folders that no catalog file points into. These are
// the leftovers of the previous stack: content that was downloaded but never
// imported, or whose catalog entry is gone.
func (a *App) Orphans() ([]string, error) {
	items, err := a.Store.ListItems("")
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, it := range items {
		files, err := a.Store.FilesForItem(it.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			// A library file and its download are hardlinks to the same bytes,
			// so identity has to be the inode, not the path.
			if id, err := fileIdentity(f.Path); err == nil {
				known[id] = true
			}
		}
	}

	dir := a.Cfg.Paths.Downloads
	list, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var orphans []string
	for _, de := range list {
		if strings.HasPrefix(de.Name(), ".") {
			continue
		}
		full := filepath.Join(dir, de.Name())
		if hasKnownFile(full, known) {
			continue
		}
		orphans = append(orphans, full)
	}
	return orphans, nil
}
