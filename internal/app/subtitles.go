package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/parse"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// SyncReport is what happened to one subtitle of one file.
type SyncReport struct {
	Item    string
	Lang    string
	Outcome parse.SyncOutcome
	Err     error
}

func (r SyncReport) String() string {
	if r.Err != nil {
		return fmt.Sprintf("%s [%s]: %v", r.Item, r.Lang, r.Err)
	}
	return fmt.Sprintf("%s [%s]: %s", r.Item, r.Lang, r.Outcome)
}

// fetchFor downloads the given languages for one file, records them, and logs
// how each was synced. It returns a line per language and how many subtitles
// were downloaded.
func (a *App) fetchFor(ctx context.Context, it *store.Item, f *store.File, langs []string) ([]string, int, error) {
	res, err := a.Parse.Subtitles(ctx, f.Path, langs, a.Store.LatestReleaseTitle(it.ID))
	if err != nil {
		return nil, 0, err
	}
	var out []string
	for _, got := range res.Results {
		_, _ = a.Store.AddSubtitle(f.ID, got.Lang, got.Path)
		line := fmt.Sprintf("%s [%s]: %s", it.Title, got.Lang, describeFetched(got))
		_ = a.Store.Log(it.ID, "subtitle_fetched", line)
		out = append(out, line)
	}
	for _, lang := range res.Embedded {
		out = append(out, fmt.Sprintf("%s [%s]: skipped, the file has an embedded track", it.Title, lang))
	}
	return out, len(res.Results), nil
}

func describeFetched(f parse.FetchedSubtitle) string {
	switch {
	case f.HashMatch:
		return fmt.Sprintf("hash match from %s, left as is", f.Provider)
	case f.Sync != nil:
		return fmt.Sprintf("from %s (score %d), %s", f.Provider, f.Score, f.Sync)
	}
	return fmt.Sprintf("from %s (score %d), not synced", f.Provider, f.Score)
}

// SyncSubtitles fixes the subtitles already next to an item's files: a
// hash-matched download replaces each one when there is one, otherwise it is
// synced in place against an embedded subtitle or the audio. Originals are
// kept, and subtitles synced before are skipped unless force is set.
func (a *App) SyncSubtitles(ctx context.Context, it *store.Item, force bool) ([]SyncReport, error) {
	langs := a.Cfg.Subtitles.Languages
	if len(langs) == 0 {
		return nil, fmt.Errorf("no subtitle languages configured")
	}
	files, err := a.Store.FilesForItem(it.ID)
	if err != nil {
		return nil, err
	}
	release := a.Store.LatestReleaseTitle(it.ID)
	var out []SyncReport
	for _, f := range files {
		for _, lang := range langs {
			r := SyncReport{Item: it.Title, Lang: lang}
			got, err := a.Parse.Sync(ctx, f.Path, lang, release, force)
			if err != nil {
				r.Err = err
			} else {
				r.Outcome = *got
			}
			_ = a.Store.Log(it.ID, "subtitle_sync", r.String())
			out = append(out, r)
		}
		// A refetch or first download may have added a file; re-read the
		// folder so the catalog matches the disk.
		a.registerSidecarSubtitles(f)
	}
	return out, nil
}

// SyncAllSubtitles runs SyncSubtitles over every item with files. A whole
// library takes minutes, so progress, when set, sees each report as it lands.
func (a *App) SyncAllSubtitles(ctx context.Context, force bool, progress func(SyncReport)) ([]SyncReport, error) {
	items, err := a.Store.ListItems("")
	if err != nil {
		return nil, err
	}
	var out []SyncReport
	for _, it := range items {
		if it.FileCount == 0 {
			continue
		}
		got, err := a.SyncSubtitles(ctx, it, force)
		if err != nil {
			got = []SyncReport{{Item: it.Title, Lang: strings.Join(a.Cfg.Subtitles.Languages, ","), Err: err}}
		}
		for _, r := range got {
			if progress != nil {
				progress(r)
			}
		}
		out = append(out, got...)
	}
	return out, nil
}
