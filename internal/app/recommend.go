package app

import (
	"context"
	"fmt"

	"github.com/LeandroMAcosta/butaca/internal/metadata"
	"github.com/LeandroMAcosta/butaca/internal/recommend"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// maxSeeds bounds the fan-out: every seed costs two TMDB requests, and beyond a
// few dozen the extra coverage stops changing the ranking.
const maxSeeds = 25

// Recommend suggests films based on what the catalog already holds.
func (a *App) Recommend(ctx context.Context, opt recommend.Options) ([]recommend.Suggestion, error) {
	if !a.TMDB.Enabled() {
		return nil, fmt.Errorf(
			"recommendations need a TMDB API key: set tmdb.api_key or TMDB_API_KEY (free at themoviedb.org)")
	}
	items, err := a.Store.ListItems("")
	if err != nil {
		return nil, err
	}

	exclude := make(map[int64]bool, len(items))
	var seeds []*store.Item
	for _, it := range items {
		if it.TMDBID > 0 {
			exclude[it.TMDBID] = true
		}
		// Seed from what is actually owned: a watchlist entry is an intention,
		// a downloaded film is evidence.
		if it.Kind == "movie" && it.TMDBID > 0 && it.FileCount > 0 {
			seeds = append(seeds, it)
		}
	}
	if len(seeds) == 0 {
		return nil, fmt.Errorf("nothing to recommend from: the library has no movies with a TMDB id yet")
	}
	if len(seeds) > maxSeeds {
		seeds = seeds[:maxSeeds]
	}

	related := make(map[string][]metadata.Movie, len(seeds))
	for _, it := range seeds {
		var found []metadata.Movie
		if r, err := a.TMDB.Recommendations(ctx, it.TMDBID); err == nil {
			found = append(found, r...)
		}
		if s, err := a.TMDB.Similar(ctx, it.TMDBID); err == nil {
			found = append(found, s...)
		}
		if len(found) > 0 {
			related[it.Title] = found
		}
	}
	if len(related) == 0 {
		return nil, fmt.Errorf("TMDB returned nothing related to your library")
	}

	if len(opt.PreferLanguages) == 0 {
		opt.PreferLanguages = a.preferredLanguages()
	}
	return recommend.Rank(related, exclude, opt), nil
}

// preferredLanguages reads the default profile's taste, falling back to the
// subtitle languages, which are a decent proxy for what the user watches.
func (a *App) preferredLanguages() []string {
	if prof, err := a.Store.DefaultProfile(); err == nil && prof != nil {
		if prof.PreferLanguage != "" {
			return []string{prof.PreferLanguage}
		}
		if len(prof.SubtitleLangs) > 0 {
			return prof.SubtitleLangs
		}
	}
	return a.Cfg.Subtitles.Languages
}

// AcceptSuggestion puts a recommendation on the watchlist.
func (a *App) AcceptSuggestion(s recommend.Suggestion, profileID int64) (int64, error) {
	return a.Store.AddItem(&store.Item{
		Kind:             "movie",
		TMDBID:           s.Movie.TMDBID,
		Title:            s.Movie.Title,
		Year:             s.Movie.Year(),
		OriginalLanguage: s.Movie.OriginalLanguage,
		Monitored:        true,
		State:            store.StateWatchlist,
		Source:           "recommendation",
		ProfileID:        profileID,
	})
}
