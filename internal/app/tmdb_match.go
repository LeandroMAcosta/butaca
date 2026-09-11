package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/metadata"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// TMDBMatch is the outcome of looking one uncatalogued-on-TMDB item up.
type TMDBMatch struct {
	Item       *store.Item
	Status     string // matched | ambiguous | not found | duplicate | error
	Movie      metadata.Movie
	Candidates []metadata.Movie
	Note       string
	// Library is where TMDB's genre says the film belongs; Moved is set when
	// apply moved it there.
	Library string
	Moved   bool
	Err     error
}

func (m TMDBMatch) String() string {
	it := m.Item
	head := fmt.Sprintf("%-9s #%d %s (%d)", m.Status, it.ID, it.Title, it.Year)
	switch m.Status {
	case "matched":
		s := fmt.Sprintf("%s -> tmdb %d %q (%d) lang %s", head, m.Movie.TMDBID, m.Movie.Title,
			m.Movie.Year(), orDashStr(m.Movie.OriginalLanguage))
		if m.Note != "" {
			s += "; " + m.Note
		}
		return s
	case "ambiguous", "not found":
		var c []string
		for _, mv := range m.Candidates {
			c = append(c, fmt.Sprintf("%d %q (%d)", mv.TMDBID, mv.Title, mv.Year()))
		}
		if len(c) == 0 {
			return head
		}
		return head + ": " + strings.Join(c, ", ")
	case "error":
		return fmt.Sprintf("%s: %v", head, m.Err)
	}
	return head + ": " + m.Note
}

func orDashStr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// MatchTMDB finds the TMDB id of every movie catalogued without one, which is
// what recommendations and the language rules key on. Items that already have
// an id are left alone, so it is safe to run again. Titles and paths never
// change. A film TMDB files under Documentary is only reported as belonging in
// the other library; with apply, it is moved there.
func (a *App) MatchTMDB(ctx context.Context, apply bool) ([]TMDBMatch, error) {
	if !a.TMDB.Enabled() {
		return nil, metadata.ErrNoAPIKey
	}
	items, err := a.Store.ListItems("movie")
	if err != nil {
		return nil, err
	}
	var out []TMDBMatch
	for _, it := range items {
		if it.TMDBID > 0 {
			continue
		}
		out = append(out, a.matchOne(ctx, it, apply))
	}
	return out, nil
}

func (a *App) matchOne(ctx context.Context, it *store.Item, apply bool) TMDBMatch {
	m := TMDBMatch{Item: it}
	titles := append([]string{it.Title}, splitLines(it.AltTitles)...)

	cands, near, chosen, err := a.tmdbCandidates(ctx, it, titles)
	if err != nil {
		m.Status, m.Err = "error", err
		return m
	}
	switch len(cands) {
	case 0:
		m.Status, m.Candidates = "not found", near
		return m
	case 1:
	default:
		m.Status, m.Candidates = "ambiguous", cands
		return m
	}
	mv := cands[0]
	m.Movie = mv

	if dup, err := a.Store.FindByTMDB("movie", mv.TMDBID); err == nil && dup != nil {
		m.Status, m.Note = "duplicate", fmt.Sprintf("tmdb %d is already #%d %s", mv.TMDBID, dup.ID, dup.Title)
		return m
	}

	// Fill what is missing; a language set by hand stays, since it was chosen
	// for this file and TMDB uses codes like "cn" that releases never carry.
	year, lang := it.Year, it.OriginalLanguage
	var notes []string
	if chosen != "" {
		notes = append(notes, chosen)
	}
	if year == 0 {
		year = mv.Year()
	} else if mv.Year() != year {
		notes = append(notes, fmt.Sprintf("TMDB year %d", mv.Year()))
	}
	if lang == "" {
		lang = mv.OriginalLanguage
	} else if mv.OriginalLanguage != "" && mv.OriginalLanguage != lang {
		notes = append(notes, fmt.Sprintf("kept lang %s, TMDB says %s", lang, mv.OriginalLanguage))
	}
	alts := mergeTitles(it.Title, splitLines(it.AltTitles), mv.Titles())
	if err := a.Store.SetTMDB(it.ID, mv.TMDBID, year, lang, strings.Join(alts, "\n")); err != nil {
		m.Status, m.Err = "error", err
		return m
	}
	_ = a.Store.Log(it.ID, "tmdb_matched", fmt.Sprintf("tmdb %d", mv.TMDBID))
	m.Status = "matched"

	// Where TMDB's genre puts it, against where it is.
	m.Library = a.pickLibrary(nil, mv.GenreIDs)
	if it.Path != "" && m.Library != a.LibraryOf(it) {
		if apply {
			if _, err := a.Move(it, m.Library); err != nil {
				notes = append(notes, fmt.Sprintf("could not move to %s: %v", m.Library, err))
			} else {
				m.Moved = true
				notes = append(notes, "moved to "+m.Library)
			}
		} else {
			notes = append(notes, fmt.Sprintf("TMDB says %s, it is in %s (--apply moves it)", m.Library, a.LibraryOf(it)))
		}
	}
	m.Note = strings.Join(notes, "; ")
	return m
}

// tmdbCandidates returns the TMDB movies that could be the item: same title
// (or original title), released within a year of the item's, since release
// years differ by country. When several qualify, one is still chosen if it is
// clearly the film: the only one from the exact year that is also the most
// voted, or one with twenty times the votes of any other. An obscure namesake
// must not win just because TMDB dates it a year closer. near holds the top
// results when nothing matched, so a "not found" line can show what TMDB had.
func (a *App) tmdbCandidates(ctx context.Context, it *store.Item, titles []string) (cands, near []metadata.Movie, note string, err error) {
	results, err := a.TMDB.SearchMovie(ctx, it.Title, it.Year)
	if err != nil {
		return nil, nil, "", err
	}
	if it.Year > 0 {
		wide, err := a.TMDB.SearchMovie(ctx, it.Title, 0)
		if err != nil {
			return nil, nil, "", err
		}
		results = append(results, wide...)
	}
	cands = filterMovies(results, titles, func(y int) bool {
		return it.Year == 0 || (y >= it.Year-1 && y <= it.Year+1)
	})
	if len(cands) == 0 {
		return nil, results[:min(3, len(results))], "", nil
	}
	cands, note = narrow(cands, it.Year)
	return cands, nil, note, nil
}

// narrow picks the evident film out of several namesakes, or returns them all
// when none is evident.
func narrow(cands []metadata.Movie, year int) ([]metadata.Movie, string) {
	if len(cands) < 2 {
		return cands, ""
	}
	sorted := append([]metadata.Movie(nil), cands...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].VoteCount > sorted[j].VoteCount })
	top, next := sorted[0], sorted[1]
	var exact []metadata.Movie
	for _, mv := range sorted {
		if mv.Year() == year {
			exact = append(exact, mv)
		}
	}
	switch {
	case len(exact) == 1 && exact[0].TMDBID == top.TMDBID:
		return sorted[:1], fmt.Sprintf("chosen over %d namesake(s): exact year and most voted", len(sorted)-1)
	case top.VoteCount >= 100 && top.VoteCount >= 20*next.VoteCount:
		return sorted[:1], fmt.Sprintf("chosen over %d namesake(s): %d votes against %d",
			len(sorted)-1, top.VoteCount, next.VoteCount)
	}
	return sorted, ""
}

func filterMovies(results []metadata.Movie, titles []string, yearOK func(int) bool) []metadata.Movie {
	var out []metadata.Movie
	seen := map[int64]bool{}
	for _, mv := range results {
		if seen[mv.TMDBID] || !yearOK(mv.Year()) {
			continue
		}
		for _, t := range titles {
			if decide.SameTitle(t, mv.Title) || decide.SameTitle(t, mv.OriginalTitle) {
				out = append(out, mv)
				seen[mv.TMDBID] = true
				break
			}
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// mergeTitles keeps every known alternative title once, excluding the title.
func mergeTitles(title string, have, add []string) []string {
	out := []string{}
	for _, t := range append(have, add...) {
		if decide.SameTitle(t, title) {
			continue
		}
		dup := false
		for _, o := range out {
			if decide.SameTitle(o, t) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, t)
		}
	}
	return out
}
