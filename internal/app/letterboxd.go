package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/LeandroMAcosta/butaca/internal/letterboxd"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// LetterboxdOptions controls an import.
type LetterboxdOptions struct {
	User string
	// Lists names the lists to pull in addition to the watchlist. Empty means
	// the watchlist only.
	Lists []string
	// ProfileID attaches the imported items to a profile, so each person's
	// watchlist stays their own.
	ProfileID int64
	// Monitor searches the imports immediately. Off by default: pulling in a
	// watchlist of two hundred films should not start two hundred downloads.
	Monitor bool
	DryRun  bool
}

type LetterboxdReport struct {
	Imported []string
	Skipped  []string
	Failed   []string
}

// ImportLetterboxd pulls a member's watchlist (and optionally named lists) into
// the catalog as watchlist entries.
func (a *App) ImportLetterboxd(ctx context.Context, opt LetterboxdOptions) (*LetterboxdReport, error) {
	if opt.User == "" {
		return nil, errors.New("no Letterboxd username: pass --user or set it on the profile")
	}
	lb := letterboxd.New()
	rep := &LetterboxdReport{}

	films, err := lb.Watchlist(ctx, opt.User)
	if err != nil {
		return nil, fmt.Errorf("watchlist: %w", err)
	}
	seen := map[string]bool{}
	for _, f := range films {
		seen[f.Slug] = true
	}

	for _, name := range opt.Lists {
		more, err := lb.ListFilms(ctx, opt.User, name)
		if err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("list %s: %v", name, err))
			continue
		}
		for _, f := range more {
			if !seen[f.Slug] {
				seen[f.Slug] = true
				films = append(films, f)
			}
		}
	}

	state := store.StateWatchlist
	if opt.Monitor {
		state = store.StateMonitored
	}

	for _, f := range films {
		label := f.Title
		if f.Year > 0 {
			label = fmt.Sprintf("%s (%d)", f.Title, f.Year)
		}

		tmdbID, cached := a.Store.CachedTMDBID(f.Slug)
		if !cached {
			if opt.DryRun {
				rep.Imported = append(rep.Imported, label+"  [tmdb id not resolved yet]")
				continue
			}
			id, err := lb.TMDBID(ctx, f.Slug)
			if err != nil {
				rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", label, err))
				continue
			}
			tmdbID = id
			_ = a.Store.CacheTMDBID(f.Slug, id, f.Title, f.Year)
		}

		if existing, err := a.Store.FindByTMDB("movie", tmdbID); err == nil && existing != nil {
			rep.Skipped = append(rep.Skipped, fmt.Sprintf("%s (already in the catalog as #%d)", label, existing.ID))
			continue
		}
		if opt.DryRun {
			rep.Imported = append(rep.Imported, label)
			continue
		}

		it := &store.Item{
			Kind: "movie", TMDBID: tmdbID, Title: f.Title, Year: f.Year,
			Monitored: true, State: state, Source: "letterboxd", ProfileID: opt.ProfileID,
		}
		// TMDB fills in the original language, which the decision engine needs.
		if a.TMDB.Enabled() {
			if results, err := a.TMDB.SearchMovie(ctx, f.Title, f.Year); err == nil {
				for _, m := range results {
					if m.TMDBID == tmdbID {
						it.OriginalLanguage = m.OriginalLanguage
						it.Title = m.Title
						break
					}
				}
			}
		}
		id, err := a.Store.AddItem(it)
		if err != nil {
			rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", label, err))
			continue
		}
		_ = a.Store.Log(id, "letterboxd_import", f.Slug)
		rep.Imported = append(rep.Imported, label)
	}
	return rep, nil
}

// LetterboxdLists shows what a member has, so the user can pick.
func (a *App) LetterboxdLists(ctx context.Context, user string) ([]letterboxd.List, error) {
	return letterboxd.New().Lists(ctx, user)
}
