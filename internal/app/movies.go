package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// AddOptions carries the manual overrides that let butaca work before a TMDB
// key is configured.
type AddOptions struct {
	Year             int
	OriginalLanguage string
	AltTitles        []string
	Monitored        bool
}

var ErrNoMatch = errors.New("no metadata match")

// AddMovie resolves a title against TMDB (when configured) and stores it.
func (a *App) AddMovie(ctx context.Context, title string, opt AddOptions) (*store.Item, error) {
	it := &store.Item{
		Kind:             "movie",
		Title:            title,
		Year:             opt.Year,
		OriginalLanguage: opt.OriginalLanguage,
		AltTitles:        strings.Join(opt.AltTitles, "\n"),
		Monitored:        opt.Monitored,
	}

	if a.TMDB.Enabled() {
		results, err := a.TMDB.SearchMovie(ctx, title, opt.Year)
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("%w for %q on TMDB", ErrNoMatch, title)
		}
		m := results[0]
		it.TMDBID = m.TMDBID
		it.Title = m.Title
		it.Year = m.Year()
		it.OriginalLanguage = m.OriginalLanguage
		// TMDB's original_title is how most releases of a foreign-language film
		// are actually named, so it must be searchable.
		alts := append(m.Titles()[1:], opt.AltTitles...)
		it.AltTitles = strings.Join(alts, "\n")
	} else if it.OriginalLanguage == "" {
		return nil, fmt.Errorf(
			"no TMDB API key: pass --lang with the film's original language (e.g. --lang fr) or set tmdb.api_key")
	}

	it.Path = filepath.Join(a.Cfg.Paths.Movies, library.MovieFolder(it.Title, it.Year))
	id, err := a.Store.AddItem(it)
	if err != nil {
		return nil, err
	}
	it.ID = id
	_ = a.Store.Log(id, "added", it.Title)
	return it, nil
}

// SearchItem searches for a stored item using its title and year.
func (a *App) SearchItem(ctx context.Context, it *store.Item) ([]decide.Candidate, error) {
	query := it.Title
	if it.Year > 0 {
		query = fmt.Sprintf("%s %d", it.Title, it.Year)
	}
	return a.SearchFor(ctx, ItemFor(it), query)
}

// ImportReady scans the queue, imports everything qBittorrent has finished, and
// returns a line per import.
func (a *App) ImportReady(ctx context.Context) ([]string, error) {
	pending, err := a.Store.PendingQueue()
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return nil, nil
	}
	torrents, err := a.QBit.List(ctx)
	if err != nil {
		return nil, err
	}
	byHash := make(map[string]int, len(torrents))
	for i, t := range torrents {
		byHash[strings.ToLower(t.Hash)] = i
	}

	var done []string
	for _, q := range pending {
		ti, ok := byHash[strings.ToLower(q.InfoHash)]
		if !ok {
			continue
		}
		t := torrents[ti]
		if !t.Done() {
			_ = a.Store.SetQueueState(q.InfoHash, "downloading", t.Progress)
			continue
		}

		it, err := a.Store.GetItem(q.ItemID)
		if err != nil {
			continue
		}
		line, err := a.importOne(it, t.ContentPath, q.EpisodeID)
		if err != nil {
			_ = a.Store.SetQueueState(q.InfoHash, "failed", t.Progress)
			_ = a.Store.Log(it.ID, "import_failed", err.Error())
			done = append(done, fmt.Sprintf("%s: %v", it.Title, err))
			continue
		}
		_ = a.Store.SetQueueState(q.InfoHash, "imported", 1)
		done = append(done, line)
	}
	return done, nil
}

func (a *App) importOne(it *store.Item, contentPath string, episodeID *int64) (string, error) {
	video, err := library.FindVideo(contentPath)
	if err != nil {
		return "", err
	}

	var res *library.ImportResult
	if episodeID != nil {
		ep, err := a.Store.EpisodeByID(*episodeID)
		if err != nil {
			return "", err
		}
		res, err = library.ImportEpisode(video, a.Cfg.Paths.TV, it.Title, it.Year, ep.Season, ep.Number, ep.Title)
		if err != nil {
			return "", err
		}
	} else {
		res, err = library.ImportMovie(video, a.Cfg.Paths.Movies, it.Title, it.Year)
		if err != nil {
			return "", err
		}
	}

	if _, err := a.Store.AddFile(&store.File{
		ItemID:    it.ID,
		EpisodeID: episodeID,
		Path:      res.Destination,
		Size:      res.Size,
	}); err != nil {
		return "", err
	}
	_ = a.Store.Log(it.ID, "imported", res.Destination)

	line := fmt.Sprintf("%s -> %s", it.Title, res.Destination)
	if a.Cfg.Subtitles.Auto && len(a.Cfg.Subtitles.Languages) > 0 {
		if n, err := a.fetchSubtitles(res.Destination); err == nil && n > 0 {
			line += fmt.Sprintf(" (+%d subtitle)", n)
		}
	}
	return line, nil
}

func (a *App) fetchSubtitles(videoPath string) (int, error) {
	ctx := context.Background()
	res, err := a.Parse.Subtitles(ctx, videoPath, a.Cfg.Subtitles.Languages)
	if err != nil {
		return 0, err
	}
	return len(res.Downloaded), nil
}

// RemoveMovie tears down every reference: the library folder, the torrent and
// its payload, and the catalog row. Missing one frees no disk, because the
// library entry and the download are hardlinks to the same bytes.
func (a *App) RemoveMovie(ctx context.Context, it *store.Item, deleteFiles bool) ([]string, error) {
	var steps []string

	if deleteFiles && it.Path != "" {
		if err := library.RemoveFolder(it.Path); err != nil {
			return steps, fmt.Errorf("remove %s: %w", it.Path, err)
		}
		steps = append(steps, "removed "+it.Path)
	}

	pending, _ := a.Store.PendingQueue()
	for _, q := range pending {
		if q.ItemID != it.ID {
			continue
		}
		if err := a.QBit.Delete(ctx, q.InfoHash, deleteFiles); err != nil {
			steps = append(steps, "qbittorrent: "+err.Error())
		} else {
			steps = append(steps, "removed torrent "+q.InfoHash[:min(8, len(q.InfoHash))])
		}
	}

	if err := a.Store.DeleteItem(it.ID); err != nil {
		return steps, err
	}
	steps = append(steps, "removed from catalog")
	return steps, nil
}
