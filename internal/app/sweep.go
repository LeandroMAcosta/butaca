package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// SearchMissing looks for every monitored item that has no file and grabs the
// best qualifying release. Items already in the queue are left alone.
func (a *App) SearchMissing(ctx context.Context) ([]string, error) {
	items, err := a.Store.ListItems("")
	if err != nil {
		return nil, err
	}
	queued, err := a.Store.PendingQueue()
	if err != nil {
		return nil, err
	}
	inQueue := make(map[int64]bool, len(queued))
	for _, q := range queued {
		inQueue[q.ItemID] = true
	}

	var out []string
	for _, it := range items {
		// Watchlist entries are catalogued on purpose but never searched: that
		// is what stops a Letterboxd import from starting hundreds of downloads.
		if !it.Monitored || it.State == store.StateWatchlist || it.State == store.StateUnmonitored {
			continue
		}
		if it.Kind == "series" {
			lines, err := a.searchMissingEpisodes(ctx, it)
			if err != nil {
				out = append(out, fmt.Sprintf("%s: %v", it.Title, err))
			}
			out = append(out, lines...)
			continue
		}
		if it.FileCount > 0 || inQueue[it.ID] {
			continue
		}
		cands, err := a.SearchItem(ctx, it)
		if err != nil {
			out = append(out, fmt.Sprintf("%s: %v", it.Title, err))
			continue
		}
		best := decide.Pick(cands)
		if best == nil {
			continue
		}
		if err := a.Grab(ctx, it, *best); err != nil {
			out = append(out, fmt.Sprintf("%s: %v", it.Title, err))
			continue
		}
		out = append(out, fmt.Sprintf("%s -> %s", it.Title, best.Release.Title))
	}
	return out, nil
}

// FillSubtitleGaps fetches subtitles for library files that do not have them.
// Existence is checked on disk rather than in the database, so subtitles added
// by any other means still count.
func (a *App) FillSubtitleGaps(ctx context.Context) ([]string, error) {
	if !a.Cfg.Subtitles.Auto || len(a.Cfg.Subtitles.Languages) == 0 {
		return nil, nil
	}
	items, err := a.Store.ListItems("")
	if err != nil {
		return nil, err
	}

	var out []string
	for _, it := range items {
		files, err := a.Store.FilesForItem(it.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			missing := a.missingSubtitleLangs(f.Path)
			if len(missing) == 0 {
				continue
			}
			res, err := a.Parse.Subtitles(ctx, f.Path, missing)
			if err != nil {
				out = append(out, fmt.Sprintf("%s: %v", it.Title, err))
				continue
			}
			for _, path := range res.Downloaded {
				if _, err := a.Store.AddSubtitle(f.ID, langOf(path), path); err != nil {
					continue
				}
			}
			if len(res.Downloaded) > 0 {
				out = append(out, fmt.Sprintf("%s: %d subtitle(s)", it.Title, len(res.Downloaded)))
			}
		}
	}
	return out, nil
}

// missingSubtitleLangs returns the configured languages with no sidecar file
// next to the video.
func (a *App) missingSubtitleLangs(videoPath string) []string {
	base := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	var missing []string
	for _, lang := range a.Cfg.Subtitles.Languages {
		if _, err := os.Stat(base + "." + lang + ".srt"); err == nil {
			continue
		}
		missing = append(missing, lang)
	}
	return missing
}

// langOf reads the language out of "Movie.es.srt".
func langOf(subtitlePath string) string {
	name := strings.TrimSuffix(filepath.Base(subtitlePath), filepath.Ext(subtitlePath))
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i+1:]
	}
	return ""
}

// SubtitlesForItem fetches the configured languages for one catalog entry.
func (a *App) SubtitlesForItem(ctx context.Context, it *store.Item) ([]string, error) {
	files, err := a.Store.FilesForItem(it.ID)
	if err != nil {
		return nil, err
	}
	langs := a.Cfg.Subtitles.Languages
	if len(langs) == 0 {
		return nil, fmt.Errorf("no subtitle languages configured")
	}
	var out []string
	for _, f := range files {
		missing := a.missingSubtitleLangs(f.Path)
		if len(missing) == 0 {
			continue
		}
		res, err := a.Parse.Subtitles(ctx, f.Path, missing)
		if err != nil {
			return out, err
		}
		for _, path := range res.Downloaded {
			_, _ = a.Store.AddSubtitle(f.ID, langOf(path), path)
			out = append(out, path)
		}
	}
	return out, nil
}

// searchMissingEpisodes grabs the best release for each aired, monitored
// episode that has no file. Episodes are searched one at a time because season
// packs and single episodes need different handling on import.
func (a *App) searchMissingEpisodes(ctx context.Context, it *store.Item) ([]string, error) {
	missing, err := a.MissingEpisodes(it)
	if err != nil {
		return nil, err
	}
	queued, err := a.Store.PendingQueue()
	if err != nil {
		return nil, err
	}
	inQueue := map[int64]bool{}
	for _, q := range queued {
		if q.EpisodeID != nil {
			inQueue[*q.EpisodeID] = true
		}
	}

	var out []string
	for _, ep := range missing {
		if inQueue[ep.ID] {
			continue
		}
		cands, err := a.SearchEpisode(ctx, it, ep.Season, ep.Number)
		if err != nil {
			return out, err
		}
		best := decide.Pick(cands)
		if best == nil {
			continue
		}
		if err := a.GrabEpisode(ctx, it, ep, *best); err != nil {
			out = append(out, fmt.Sprintf("%s S%02dE%02d: %v", it.Title, ep.Season, ep.Number, err))
			continue
		}
		out = append(out, fmt.Sprintf("%s S%02dE%02d -> %s", it.Title, ep.Season, ep.Number, best.Release.Title))
	}
	return out, nil
}
