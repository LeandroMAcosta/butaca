package tui

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/recommend"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// Messages carrying work back from the background.
type (
	refreshed struct {
		items     []*store.Item
		watchlist []*store.Item
		queue     []*store.QueueEntry
		profiles  []*store.Profile
		langs     map[int64]store.Languages
		disk      library.DiskUsage
		occupied  int64
		err       error
	}
	healthMsg  struct{ health app.Health }
	actionDone struct {
		msg string
		err error
	}
	searchDone struct {
		item  *store.Item
		cands []decide.Candidate
		err   error
	}
	detailDone struct {
		item  *store.Item
		langs store.Languages
		files []*store.File
		err   error
	}
	discoverDone struct {
		suggestions []recommend.Suggestion
		err         error
	}
	tickMsg time.Time
)

// Every command guards against a nil app so the model can be exercised
// headlessly: the views and key handling are testable without a database or a
// running Prowlarr.
func (b *browser) reload() tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	moviesPath := a.Cfg.Paths.Movies
	return func() tea.Msg {
		var r refreshed
		var err error
		if r.items, err = a.Store.ListByState(store.StateMonitored); err != nil {
			return refreshed{err: err}
		}
		if r.watchlist, err = a.Store.ListByState(store.StateWatchlist); err != nil {
			return refreshed{err: err}
		}
		if r.queue, err = a.Store.PendingQueue(); err != nil {
			return refreshed{err: err}
		}
		if r.profiles, err = a.Store.ListProfiles(); err != nil {
			return refreshed{err: err}
		}
		all, err := a.Store.ListItems("")
		if err != nil {
			return refreshed{err: err}
		}
		r.langs = make(map[int64]store.Languages, len(all))
		for _, it := range all {
			r.occupied += it.SizeBytes
			if l, err := a.Store.LanguagesForItem(it.ID); err == nil {
				r.langs[it.ID] = l
			}
		}
		r.disk, _ = library.Usage(moviesPath)
		return r
	}
}

// checkHealth runs separately from reload: it makes network calls and must not
// hold up the first paint.
func (b *browser) checkHealth() tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg { return healthMsg{health: a.Health(context.Background())} }
}

// tick drives the queue's live progress.
func tick() tea.Cmd {
	return tea.Tick(queuePoll, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// syncQueue asks the download client where each transfer is, then reloads.
func (b *browser) syncQueue() tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		ctx := context.Background()
		torrents, err := a.QBit.List(ctx)
		if err != nil {
			return actionDone{err: err}
		}
		byHash := make(map[string]float64, len(torrents))
		state := make(map[string]string, len(torrents))
		for _, t := range torrents {
			byHash[strings.ToLower(t.Hash)] = t.Progress
			state[strings.ToLower(t.Hash)] = t.State
		}
		pending, err := a.Store.PendingQueue()
		if err != nil {
			return actionDone{err: err}
		}
		for _, q := range pending {
			if p, ok := byHash[strings.ToLower(q.InfoHash)]; ok {
				_ = a.Store.SetQueueState(q.InfoHash, state[strings.ToLower(q.InfoHash)], p)
			}
		}
		return actionDone{}
	}
}

func (b *browser) importReady() tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		lines, err := a.ImportReady(context.Background())
		if err != nil {
			return actionDone{err: err}
		}
		if len(lines) == 0 {
			return actionDone{msg: "nothing ready to import"}
		}
		return actionDone{msg: strings.Join(lines, "; ")}
	}
}

func (b *browser) searchItem(it *store.Item) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		cands, err := a.SearchItem(context.Background(), it)
		return searchDone{item: it, cands: cands, err: err}
	}
}

func (b *browser) grabRelease(it *store.Item, c decide.Candidate) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		if err := a.Grab(context.Background(), it, c); err != nil {
			return actionDone{err: err}
		}
		return actionDone{msg: "grabbed " + c.Release.Title}
	}
}

func (b *browser) loadDetail(it *store.Item) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		langs, err := a.Store.LanguagesForItem(it.ID)
		if err != nil {
			return detailDone{item: it, err: err}
		}
		files, err := a.Store.FilesForItem(it.ID)
		return detailDone{item: it, langs: langs, files: files, err: err}
	}
}

func (b *browser) removeItem(it *store.Item, opt app.RemoveOptions) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		steps, err := a.Remove(context.Background(), it, opt)
		out := make([]string, 0, len(steps))
		for _, s := range steps {
			out = append(out, s.String())
		}
		if err != nil {
			return actionDone{msg: strings.Join(out, "; "), err: err}
		}
		return actionDone{msg: strings.Join(out, "; ")}
	}
}

func (b *browser) setState(it *store.Item, state string) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		if err := a.Store.SetState(it.ID, state); err != nil {
			return actionDone{err: err}
		}
		verb := "moved to the watchlist"
		if state == store.StateMonitored {
			verb = "is now monitored"
		}
		return actionDone{msg: it.Title + " " + verb}
	}
}

func (b *browser) assignProfile(it *store.Item, profileID int64, name string) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		if err := a.Store.AssignProfile(it.ID, profileID); err != nil {
			return actionDone{err: err}
		}
		return actionDone{msg: it.Title + " now uses profile " + name}
	}
}

func (b *browser) discover() tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		s, err := a.Recommend(context.Background(), recommend.DefaultOptions())
		return discoverDone{suggestions: s, err: err}
	}
}

func (b *browser) acceptSuggestion(s recommend.Suggestion) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		if _, err := a.AcceptSuggestion(s, 0); err != nil {
			return actionDone{err: err}
		}
		return actionDone{msg: s.Movie.Title + " added to the watchlist"}
	}
}
