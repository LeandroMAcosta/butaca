package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// AddSeries resolves a series against TMDB and stores it with its seasons and
// episodes. A TMDB key is required: unlike a film, a series cannot be tracked
// without knowing which episodes exist.
func (a *App) AddSeries(ctx context.Context, title string, opt AddOptions) (*store.Item, error) {
	if !a.TMDB.Enabled() {
		return nil, fmt.Errorf("series need a TMDB API key: set tmdb.api_key or TMDB_API_KEY")
	}
	results, err := a.TMDB.SearchSeries(ctx, title, opt.Year)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%w for %q on TMDB", ErrNoMatch, title)
	}
	sr := results[0]

	alts := append(sr.Titles()[1:], opt.AltTitles...)
	it := &store.Item{
		Kind:             "series",
		TMDBID:           sr.TMDBID,
		Title:            sr.Name,
		Year:             sr.Year(),
		OriginalLanguage: sr.OriginalLanguage,
		AltTitles:        strings.Join(alts, "\n"),
		Monitored:        opt.Monitored,
	}
	it.Path = filepath.Join(a.Cfg.Paths.TV, library.MovieFolder(it.Title, it.Year))

	id, err := a.Store.AddItem(it)
	if err != nil {
		return nil, err
	}
	it.ID = id

	seasons, err := a.TMDB.Seasons(ctx, sr.TMDBID)
	if err != nil {
		return it, fmt.Errorf("stored %s but could not read its seasons: %w", it.Title, err)
	}
	for _, s := range seasons {
		seasonID, err := a.Store.AddSeason(id, s.Number, opt.Monitored)
		if err != nil {
			return it, err
		}
		eps, err := a.TMDB.Episodes(ctx, sr.TMDBID, s.Number)
		if err != nil {
			continue // a season TMDB cannot serve should not abort the rest
		}
		for _, e := range eps {
			if _, err := a.Store.AddEpisode(seasonID, e.Number, e.Name, e.AirDate, opt.Monitored); err != nil {
				return it, err
			}
		}
	}
	_ = a.Store.Log(id, "added", it.Title)
	return it, nil
}

// SearchEpisode looks for one episode. The query uses the SxxEyy form every
// release group uses.
func (a *App) SearchEpisode(ctx context.Context, it *store.Item, season, episode int) ([]decide.Candidate, error) {
	query := fmt.Sprintf("%s S%02dE%02d", it.Title, season, episode)
	cands, err := a.SearchFor(ctx, ItemFor(it), query)
	if err != nil {
		return nil, err
	}
	// A search for one episode still returns whole-season packs and the wrong
	// episodes, so anything that is not this exact episode is rejected.
	for i := range cands {
		p := cands[i].Parsed
		if p.Season != 0 && p.Season != season {
			cands[i].Rejects = append(cands[i].Rejects,
				fmt.Sprintf("season %d, want %d", p.Season, season))
		}
		if p.Episode != 0 && p.Episode != episode {
			cands[i].Rejects = append(cands[i].Rejects,
				fmt.Sprintf("episode %d, want %d", p.Episode, episode))
		}
		if p.Episode == 0 {
			cands[i].Rejects = append(cands[i].Rejects, "not a single episode")
		}
	}
	return cands, nil
}

// MissingEpisodes lists monitored episodes that have aired and have no file.
func (a *App) MissingEpisodes(it *store.Item) ([]*store.Episode, error) {
	eps, err := a.Store.EpisodesForItem(it.ID)
	if err != nil {
		return nil, err
	}
	var out []*store.Episode
	for _, e := range eps {
		if !e.Monitored || e.HasFile {
			continue
		}
		// An episode with no air date has not been scheduled yet.
		if e.AirDate == "" {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// GrabEpisode sends an episode release to the download client, recording which
// episode it belongs to so the import lands in the right place.
func (a *App) GrabEpisode(ctx context.Context, it *store.Item, ep *store.Episode, c decide.Candidate) error {
	link := c.Release.Link()
	if link == "" {
		return fmt.Errorf("release %q has no magnet or download URL", c.Release.Title)
	}
	if err := a.QBit.Add(ctx, link); err != nil {
		return err
	}
	hash := strings.ToLower(c.Release.InfoHash)
	if hash == "" {
		hash = c.Release.GUID
	}
	epID := ep.ID
	if err := a.Store.Enqueue(&store.QueueEntry{
		ItemID:       it.ID,
		EpisodeID:    &epID,
		ReleaseTitle: c.Release.Title,
		Magnet:       link,
		InfoHash:     hash,
		State:        "downloading",
		Size:         c.Release.Size,
	}); err != nil {
		return err
	}
	return a.Store.Log(it.ID, "grabbed", fmt.Sprintf("S%02dE%02d %s", ep.Season, ep.Number, c.Release.Title))
}

// SearchMissingFor searches one catalog entry, movie or series.
func (a *App) SearchMissingFor(ctx context.Context, it *store.Item) ([]string, error) {
	if it.Kind == "series" {
		return a.searchMissingEpisodes(ctx, it)
	}
	cands, err := a.SearchItem(ctx, it)
	if err != nil {
		return nil, err
	}
	best := decide.Pick(cands)
	if best == nil {
		return []string{fmt.Sprintf("%d releases, none qualified", len(cands))}, nil
	}
	if err := a.Grab(ctx, it, *best); err != nil {
		return nil, err
	}
	return []string{"grabbed " + best.Release.Title}, nil
}
