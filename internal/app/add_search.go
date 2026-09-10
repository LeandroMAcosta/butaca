package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// SearchNew looks for something that is not in the catalog yet.
//
// It deliberately does not require TMDB: the query itself is the title to match
// against, and the release names supply the year. That keeps "find me this
// film" working before an API key is configured, and TMDB only enriches the
// result afterwards.
func (a *App) SearchNew(ctx context.Context, query string) ([]decide.Candidate, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("nothing to search for")
	}
	rules, err := a.Rules()
	if err != nil {
		return nil, err
	}
	// An unknown film has no original language, and the engine treats that as
	// "no constraint" rather than rejecting everything.
	item := decide.Item{Titles: []string{stripYear(query)}}
	return a.SearchFor(ctx, item, query, rules)
}

// stripYear removes a trailing year so "Amelie 2001" still matches a release
// parsed as "Amelie".
func stripYear(q string) string {
	fields := strings.Fields(q)
	if len(fields) < 2 {
		return q
	}
	last := fields[len(fields)-1]
	if len(last) == 4 {
		if y := atoiSafe(last); y >= 1880 && y <= 2200 {
			return strings.Join(fields[:len(fields)-1], " ")
		}
	}
	return q
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// AddFromRelease creates a catalog entry out of a chosen release and grabs it.
// Metadata comes from the parsed release name, then from TMDB when available.
func (a *App) AddFromRelease(ctx context.Context, c decide.Candidate, fallbackTitle string, documentary *bool) (*store.Item, error) {
	title := strings.TrimSpace(c.Parsed.Title)
	if title == "" {
		title = strings.TrimSpace(fallbackTitle)
	}
	if title == "" {
		return nil, fmt.Errorf("could not work out a title from %q", c.Release.Title)
	}

	it := &store.Item{
		Kind:      "movie",
		Title:     title,
		Year:      c.Parsed.Year,
		Monitored: true,
		State:     store.StateMonitored,
		Source:    "manual",
	}
	var genres []int
	if a.TMDB.Enabled() {
		if results, err := a.TMDB.SearchMovie(ctx, title, c.Parsed.Year); err == nil && len(results) > 0 {
			m := results[0]
			genres = m.GenreIDs
			it.TMDBID, it.Title, it.OriginalLanguage = m.TMDBID, m.Title, m.OriginalLanguage
			if y := m.Year(); y > 0 {
				it.Year = y
			}
			if alts := m.Titles()[1:]; len(alts) > 0 {
				it.AltTitles = strings.Join(alts, "\n")
			}
		}
	}
	folder, err := a.movieFolder(a.pickLibrary(documentary, genres), it.Title, it.Year)
	if err != nil {
		return nil, err
	}
	it.Path = folder

	id, err := a.Store.AddItem(it)
	if err != nil {
		return nil, err
	}
	it.ID = id
	_ = a.Store.Log(id, "added", it.Title)

	if err := a.Grab(ctx, it, c); err != nil {
		return it, fmt.Errorf("added %s but could not grab it: %w", it.Title, err)
	}
	return it, nil
}
