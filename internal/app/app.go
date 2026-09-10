// Package app wires the clients together and holds the operations the CLI, the
// MCP server and the TUI all share.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/config"
	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/download"
	"github.com/LeandroMAcosta/butaca/internal/indexer"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/metadata"
	"github.com/LeandroMAcosta/butaca/internal/parse"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

type App struct {
	Cfg      *config.Config
	Store    *store.Store
	Prowlarr *indexer.Client
	QBit     *download.Client
	Parse    *parse.Client
	TMDB     *metadata.Client
}

func New(cfg *config.Config) (*App, error) {
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		return nil, err
	}
	return &App{
		Cfg:      cfg,
		Store:    st,
		Prowlarr: indexer.New(cfg.Prowlarr.URL, cfg.Prowlarr.APIKey),
		QBit:     download.New(cfg.QBittorrent.URL, cfg.QBittorrent.Username, cfg.QBittorrent.Password, cfg.QBittorrent.Category),
		Parse:    parse.New(cfg.Parse.URL),
		TMDB:     metadata.New(cfg.TMDB.APIKey),
	}, nil
}

func (a *App) Close() error { return a.Store.Close() }

// RulesFor resolves the rules for one item: its profile when it has one, the
// default profile when there is one, and the global config underneath. A
// profile only overrides the fields it actually sets, so "1080p only" does not
// silently reset every other preference.
func (a *App) RulesFor(it *store.Item) (decide.Rules, error) {
	rules, err := a.Rules()
	if err != nil {
		return rules, err
	}
	prof, err := a.profileFor(it)
	if err != nil || prof == nil {
		return rules, err
	}
	return applyProfile(rules, prof)
}

func (a *App) profileFor(it *store.Item) (*store.Profile, error) {
	if it != nil && it.ProfileID > 0 {
		p, err := a.Store.GetProfile(it.ProfileID)
		if err == nil {
			return p, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
	}
	return a.Store.DefaultProfile()
}

func applyProfile(r decide.Rules, p *store.Profile) (decide.Rules, error) {
	if len(p.Resolutions) > 0 {
		r.Resolutions = p.Resolutions
	}
	if len(p.Sources) > 0 {
		r.Sources = p.Sources
	}
	if p.MinSeeders > 0 {
		r.MinSeeders = p.MinSeeders
	}
	if p.MinSize != "" {
		v, err := decide.ParseSize(p.MinSize)
		if err != nil {
			return r, fmt.Errorf("profile %s: min_size: %w", p.Name, err)
		}
		r.MinSize = v
	}
	if p.MaxSize != "" {
		v, err := decide.ParseSize(p.MaxSize)
		if err != nil {
			return r, fmt.Errorf("profile %s: max_size: %w", p.Name, err)
		}
		r.MaxSize = v
	}
	if p.LanguageMode != "" {
		mode, err := decide.ParseLanguageMode(p.LanguageMode)
		if err != nil {
			return r, fmt.Errorf("profile %s: %w", p.Name, err)
		}
		r.LanguageMode = mode
	}
	if p.PreferLanguage != "" {
		r.PreferLanguage = p.PreferLanguage
	}
	return r, nil
}

// SubtitleLangsFor returns the subtitle languages that apply to an item.
func (a *App) SubtitleLangsFor(it *store.Item) []string {
	if prof, err := a.profileFor(it); err == nil && prof != nil && len(prof.SubtitleLangs) > 0 {
		return prof.SubtitleLangs
	}
	return a.Cfg.Subtitles.Languages
}

// Rules converts the config into engine rules, resolving sizes and mode.
func (a *App) Rules() (decide.Rules, error) {
	r := a.Cfg.Rules
	mode, err := decide.ParseLanguageMode(r.LanguageMode)
	if err != nil {
		return decide.Rules{}, err
	}
	minSize, err := decide.ParseSize(r.MinSize)
	if err != nil {
		return decide.Rules{}, fmt.Errorf("rules.min_size: %w", err)
	}
	maxSize, err := decide.ParseSize(r.MaxSize)
	if err != nil {
		return decide.Rules{}, fmt.Errorf("rules.max_size: %w", err)
	}
	return decide.Rules{
		MinSeeders:     r.MinSeeders,
		Resolutions:    r.Resolutions,
		Sources:        r.Sources,
		MinSize:        minSize,
		MaxSize:        maxSize,
		PreferGroups:   r.PreferGroups,
		RejectPatterns: r.RejectPatterns,
		LanguageMode:   mode,
		PreferLanguage: r.PreferLanguage,
	}, nil
}

// Health reports on each dependency so a failure is attributable instead of
// showing up later as an unexplained empty result set.
type Health struct {
	Prowlarr     string
	QBittorrent  string
	Parse        string
	Indexers     []indexer.Indexer
	Hardlinkable bool
	HardlinkNote string
}

func (a *App) Health(ctx context.Context) Health {
	h := Health{Prowlarr: "ok", QBittorrent: "ok", Parse: "ok"}

	idx, err := a.Prowlarr.Indexers(ctx)
	if err != nil {
		h.Prowlarr = err.Error()
	} else {
		h.Indexers = idx
	}
	if v, err := a.QBit.Version(ctx); err != nil {
		h.QBittorrent = err.Error()
	} else {
		h.QBittorrent = "ok (" + v + ")"
	}
	if err := a.Parse.Health(ctx); err != nil {
		h.Parse = err.Error()
	}

	same, err := library.SameFilesystem(a.Cfg.Paths.Downloads, a.Cfg.Paths.Movies)
	switch {
	case err != nil:
		h.HardlinkNote = err.Error()
	case !same:
		h.HardlinkNote = fmt.Sprintf("%s and %s are on different filesystems; imports will fail",
			a.Cfg.Paths.Downloads, a.Cfg.Paths.Movies)
	default:
		h.Hardlinkable = true
	}
	if d := a.Cfg.Paths.Documentaries; h.Hardlinkable && d != "" {
		same, err := library.SameFilesystem(a.Cfg.Paths.Downloads, d)
		switch {
		case err != nil:
			h.Hardlinkable, h.HardlinkNote = false, err.Error()
		case !same:
			h.Hardlinkable = false
			h.HardlinkNote = fmt.Sprintf("%s and %s are on different filesystems; documentary imports will fail",
				a.Cfg.Paths.Downloads, d)
		}
	}
	return h
}

// SearchFor runs a Prowlarr search, parses every result through the sidecar and
// scores them against rules. Rejected candidates are kept so callers can
// explain themselves.
func (a *App) SearchFor(ctx context.Context, item decide.Item, query string, rules decide.Rules) ([]decide.Candidate, error) {
	releases, err := a.Prowlarr.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, nil
	}

	titles := make([]string, len(releases))
	for i, r := range releases {
		titles[i] = r.Title
	}
	parsed, err := a.Parse.Parse(ctx, titles)
	if err != nil {
		return nil, err
	}

	return decide.Evaluate(releases, parsed, item, rules), nil
}

// ItemFor builds the engine's view of a stored catalog entry, including every
// alternative title. Releases are frequently named after the original-language
// title -- "Le Fabuleux Destin d'Amelie Poulain" rather than "Amelie".
func ItemFor(it *store.Item) decide.Item {
	titles := []string{it.Title}
	for _, alt := range strings.Split(it.AltTitles, "\n") {
		if alt = strings.TrimSpace(alt); alt != "" {
			titles = append(titles, alt)
		}
	}
	return decide.Item{Titles: titles, Year: it.Year, OriginalLanguage: it.OriginalLanguage}
}

// Grab hands a release to qBittorrent and records it in the queue.
func (a *App) Grab(ctx context.Context, it *store.Item, c decide.Candidate) error {
	link := c.Release.Link()
	if link == "" {
		return fmt.Errorf("release %q has neither a magnet nor a download URL", c.Release.Title)
	}
	if err := a.QBit.Add(ctx, link); err != nil {
		return err
	}
	hash := strings.ToLower(c.Release.InfoHash)
	if hash == "" {
		hash = c.Release.GUID
	}
	if err := a.Store.Enqueue(&store.QueueEntry{
		ItemID:       it.ID,
		ReleaseTitle: c.Release.Title,
		Magnet:       link,
		InfoHash:     hash,
		State:        "downloading",
		Size:         c.Release.Size,
	}); err != nil {
		return err
	}
	return a.Store.Log(it.ID, "grabbed", c.Release.Title)
}
