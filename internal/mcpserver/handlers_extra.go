package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/recommend"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func (s *Server) handleWatchlist(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	items, err := s.app.Store.ListByState(store.StateWatchlist)
	if err != nil {
		return fail(err), nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, fmt.Sprintf("%d  %s (%d)  from %s", it.ID, it.Title, it.Year, orDash(it.Source)))
	}
	return lines(fmt.Sprintf("%d on the watchlist", len(items)), out, "the watchlist is empty"), nil
}

func (s *Server) handleWatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ref, err := req.RequireString("item")
	if err != nil {
		return fail(err), nil
	}
	it, err := s.resolve(ref)
	if err != nil {
		return fail(err), nil
	}
	state := store.StateMonitored
	if !req.GetBool("monitor", true) {
		state = store.StateWatchlist
	}
	if err := s.app.Store.SetState(it.ID, state); err != nil {
		return fail(err), nil
	}
	msg := fmt.Sprintf("%s is now %s", it.Title, state)
	if state == store.StateMonitored && req.GetBool("search", false) {
		it.State = state
		found, err := s.app.SearchMissingFor(ctx, it)
		if err != nil {
			return text("%s\nsearch failed: %v", msg, err), nil
		}
		return lines(msg, found, msg), nil
	}
	return mcp.NewToolResultText(msg), nil
}

func (s *Server) handleRecommend(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opt := recommend.DefaultOptions()
	opt.Limit = req.GetInt("limit", 20)

	suggestions, err := s.app.Recommend(ctx, opt)
	if err != nil {
		return fail(err), nil
	}
	if add := req.GetInt("add", 0); add > 0 {
		if add > len(suggestions) {
			return text("there is no suggestion #%d", add), nil
		}
		sg := suggestions[add-1]
		if _, err := s.app.AcceptSuggestion(sg, 0); err != nil {
			return fail(err), nil
		}
		return text("added %s (%d) to the watchlist", sg.Movie.Title, sg.Movie.Year()), nil
	}
	out := make([]string, 0, len(suggestions))
	for i, sg := range suggestions {
		out = append(out, fmt.Sprintf("%d. %s (%d) %.1f★ %s — because of %s",
			i+1, sg.Movie.Title, sg.Movie.Year(), sg.Movie.VoteAverage,
			orDash(sg.Movie.OriginalLanguage), strings.Join(sg.Seeds, ", ")))
	}
	return lines("suggestions from your library", out, "nothing to suggest"), nil
}

func (s *Server) handleLanguages(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if ref := req.GetString("item", ""); ref != "" {
		it, err := s.resolve(ref)
		if err != nil {
			return fail(err), nil
		}
		langs, err := s.app.Store.LanguagesForItem(it.ID)
		if err != nil {
			return fail(err), nil
		}
		if langs.Empty() {
			return text("%s has no track data yet; run `butaca scan-tracks`", it.Title), nil
		}
		return text("%s\n  audio: %s\n  subtitles: %s", it.Title,
			strings.Join(langs.Audio, " "), strings.Join(langs.Subtitles, " ")), nil
	}

	items, err := s.app.Store.ListItems("")
	if err != nil {
		return fail(err), nil
	}
	audio, subs := map[string]int{}, map[string]int{}
	var out []string
	for _, it := range items {
		langs, err := s.app.Store.LanguagesForItem(it.ID)
		if err != nil || langs.Empty() {
			continue
		}
		for _, l := range langs.Audio {
			audio[l]++
		}
		for _, l := range langs.Subtitles {
			subs[l]++
		}
		out = append(out, fmt.Sprintf("%s — audio %s, subs %s", it.Title,
			store.Summary(langs.Audio, 4), store.Summary(langs.Subtitles, 5)))
	}
	header := fmt.Sprintf("audio across the library: %s\nsubtitles: %s",
		strings.Join(keysOf(audio), " "), strings.Join(keysOf(subs), " "))
	return lines(header, out, "no track data yet; run `butaca scan-tracks`"), nil
}

func (s *Server) handleDisk(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	items, err := s.app.Store.ListItems("")
	if err != nil {
		return fail(err), nil
	}
	var occupied int64
	for _, it := range items {
		occupied += it.SizeBytes
	}
	u, err := library.Usage(s.app.Cfg.Paths.Movies)
	if err != nil {
		return fail(err), nil
	}
	return text("%s free of %s (%.0f%% used)\nlibrary occupies %s across %d items",
		library.HumanSize(u.Free), library.HumanSize(u.Total), u.UsedPercent(),
		library.HumanSize(occupied), len(items)), nil
}

func (s *Server) handleProfiles(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	profs, err := s.app.Store.ListProfiles()
	if err != nil {
		return fail(err), nil
	}
	out := make([]string, 0, len(profs))
	for _, p := range profs {
		name := p.Name
		if p.IsDefault {
			name += " (default)"
		}
		mode := p.LanguageMode
		if p.PreferLanguage != "" {
			mode += ":" + p.PreferLanguage
		}
		out = append(out, fmt.Sprintf("%d  %s  letterboxd=%s  audio=%s  quality=%s  subs=%s",
			p.ID, name, orDash(p.LetterboxdUser), mode,
			orDash(strings.Join(p.Resolutions, ",")), orDash(strings.Join(p.SubtitleLangs, ","))))
	}
	return lines("profiles", out, "no profiles; the global configuration applies"), nil
}

func (s *Server) handleLetterboxd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opt := app.LetterboxdOptions{
		User:   req.GetString("user", ""),
		DryRun: req.GetBool("dry_run", false),
	}
	if opt.User == "" {
		if prof, err := s.app.Store.DefaultProfile(); err == nil && prof != nil {
			opt.User, opt.ProfileID = prof.LetterboxdUser, prof.ID
		}
	}
	rep, err := s.app.ImportLetterboxd(ctx, opt)
	if err != nil {
		return fail(err), nil
	}
	var out []string
	out = append(out, rep.Imported...)
	for _, l := range rep.Skipped {
		out = append(out, "(already known) "+l)
	}
	for _, l := range rep.Failed {
		out = append(out, "(failed) "+l)
	}
	verb := "imported"
	if opt.DryRun {
		verb = "would import"
	}
	return lines(fmt.Sprintf("%s %d to the watchlist", verb, len(rep.Imported)), out, "nothing found"), nil
}

func keysOf(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
