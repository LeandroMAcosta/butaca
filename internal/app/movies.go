package app

import (
	"context"
	"errors"
	"fmt"
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
	// Documentary picks the library: true or false decides it, nil lets TMDB's
	// genre decide when a documentaries library is configured.
	Documentary *bool
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

	var genres []int
	if a.TMDB.Enabled() {
		results, err := a.TMDB.SearchMovie(ctx, title, opt.Year)
		if err != nil {
			return nil, err
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("%w for %q on TMDB", ErrNoMatch, title)
		}
		m := results[0]
		genres = m.GenreIDs
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

	folder, err := a.movieFolder(a.pickLibrary(opt.Documentary, genres), it.Title, it.Year)
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
	return it, nil
}

// SearchItem searches for a stored item using its title and year.
func (a *App) SearchItem(ctx context.Context, it *store.Item) ([]decide.Candidate, error) {
	query := it.Title
	if it.Year > 0 {
		query = fmt.Sprintf("%s %d", it.Title, it.Year)
	}
	rules, err := a.RulesFor(it)
	if err != nil {
		return nil, err
	}
	return a.SearchFor(ctx, ItemFor(it), query, rules)
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
		root, err := a.LibraryRoot(a.LibraryOf(it))
		if err != nil {
			return "", err
		}
		res, err = library.ImportMovie(video, root, it.Title, it.Year)
		if err != nil {
			return "", err
		}
	}

	fileID, err := a.Store.AddFile(&store.File{
		ItemID:    it.ID,
		EpisodeID: episodeID,
		Path:      res.Destination,
		Size:      res.Size,
	})
	if err != nil {
		return "", err
	}
	// Probe now, while the file is fresh: knowing its audio and subtitle
	// languages is what lets the library answer "do I have this in Spanish?".
	if _, err := a.ProbeFile(context.Background(), &store.File{ID: fileID, Path: res.Destination}); err != nil {
		_ = a.Store.Log(it.ID, "probe_failed", err.Error())
	}
	_ = a.Store.Log(it.ID, "imported", res.Destination)

	line := fmt.Sprintf("%s -> %s", it.Title, res.Destination)
	if a.Cfg.Subtitles.Auto && len(a.Cfg.Subtitles.Languages) > 0 {
		f := &store.File{ID: fileID, Path: res.Destination}
		if _, n, err := a.fetchFor(context.Background(), it, f, a.Cfg.Subtitles.Languages); err == nil && n > 0 {
			line += fmt.Sprintf(" (+%d subtitle)", n)
		}
	}
	return line, nil
}
