package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func (s *Server) resolve(ref string) (*store.Item, error) {
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return s.app.Store.GetItem(id)
	}
	found, err := s.app.Store.FindItems(ref)
	if err != nil {
		return nil, err
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("nothing in the catalog matches %q", ref)
	case 1:
		return found[0], nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q matches %d entries; use the id:", ref, len(found))
		for _, it := range found {
			fmt.Fprintf(&b, "\n  %d  %s (%d)", it.ID, it.Title, it.Year)
		}
		return nil, fmt.Errorf("%s", b.String())
	}
}

func (s *Server) handleList(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	items, err := s.app.Store.ListItems("")
	if err != nil {
		return fail(err), nil
	}
	var out []string
	var total int64
	for _, it := range items {
		total += it.SizeBytes
		state := "missing"
		if it.FileCount > 0 {
			state = humanSize(it.SizeBytes)
		}
		out = append(out, fmt.Sprintf("%d  %s (%d)  [%s]  %s", it.ID, it.Title, it.Year, orDash(it.OriginalLanguage), state))
	}
	res := lines(fmt.Sprintf("%d items, %s on disk", len(items), humanSize(total)), out, "the catalog is empty")
	return res, nil
}

func (s *Server) handleAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title, err := req.RequireString("title")
	if err != nil {
		return fail(err), nil
	}
	opt := app.AddOptions{
		Year:             req.GetInt("year", 0),
		OriginalLanguage: req.GetString("original_language", ""),
		Monitored:        true,
	}
	if alt := req.GetString("alt_title", ""); alt != "" {
		opt.AltTitles = []string{alt}
	}

	it, err := s.app.AddMovie(ctx, title, opt)
	if err != nil {
		return fail(err), nil
	}
	msg := fmt.Sprintf("added #%d  %s (%d)  original language: %s", it.ID, it.Title, it.Year, orDash(it.OriginalLanguage))
	if !req.GetBool("search", true) {
		return mcp.NewToolResultText(msg), nil
	}

	cands, err := s.app.SearchItem(ctx, it)
	if err != nil {
		return text("%s\nsearch failed: %v", msg, err), nil
	}
	best := decide.Pick(cands)
	if best == nil {
		return text("%s\n%d releases found, none qualified. Use search with the id to see why.", msg, len(cands)), nil
	}
	if err := s.app.Grab(ctx, it, *best); err != nil {
		return text("%s\nfound %s but could not grab it: %v", msg, best.Release.Title, err), nil
	}
	return text("%s\ngrabbed %s (%s, %d seeders)", msg, best.Release.Title, humanSize(best.Release.Size), best.Release.Seeders), nil
}

func (s *Server) handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ref, err := req.RequireString("item")
	if err != nil {
		return fail(err), nil
	}
	it, err := s.resolve(ref)
	if err != nil {
		return fail(err), nil
	}
	cands, err := s.app.SearchItem(ctx, it)
	if err != nil {
		return fail(err), nil
	}
	if len(cands) == 0 {
		return text("no releases found for %s", it.Title), nil
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Accepted() != cands[j].Accepted() {
			return cands[i].Accepted()
		}
		return cands[i].Score > cands[j].Score
	})

	var out []string
	accepted := 0
	for _, c := range cands {
		if c.Accepted() {
			accepted++
		}
	}
	// Only the top slice is useful in a chat reply; a full 200-release dump is not.
	for i, c := range cands {
		if i >= 12 {
			out = append(out, fmt.Sprintf("... and %d more", len(cands)-i))
			break
		}
		mark := "REJECT"
		if c.Accepted() {
			mark = "ok"
		}
		line := fmt.Sprintf("[%s] %d  %s  (%s, %s, %d seeders, %s)",
			mark, c.Score, c.Release.Title, orDash(c.Parsed.ScreenSize),
			humanSize(c.Release.Size), c.Release.Seeders, c.Release.Indexer)
		if len(c.Rejects) > 0 {
			line += "  <- " + strings.Join(c.Rejects, "; ")
		}
		out = append(out, line)
	}

	header := fmt.Sprintf("%s: %d releases, %d passed the rules", it.Title, len(cands), accepted)
	if req.GetBool("grab", false) {
		best := decide.Pick(cands)
		if best == nil {
			header += "\nnothing qualified, so nothing was grabbed"
		} else if err := s.app.Grab(ctx, it, *best); err != nil {
			header += "\ngrab failed: " + err.Error()
		} else {
			header += "\ngrabbed " + best.Release.Title
		}
	}
	return lines(header, out, header), nil
}

func (s *Server) handleGrab(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ref, err := req.RequireString("item")
	if err != nil {
		return fail(err), nil
	}
	want, err := req.RequireString("release")
	if err != nil {
		return fail(err), nil
	}
	it, err := s.resolve(ref)
	if err != nil {
		return fail(err), nil
	}
	cands, err := s.app.SearchItem(ctx, it)
	if err != nil {
		return fail(err), nil
	}
	for _, c := range cands {
		if c.Release.Title != want {
			continue
		}
		if err := s.app.Grab(ctx, it, c); err != nil {
			return fail(err), nil
		}
		note := ""
		if !c.Accepted() {
			note = fmt.Sprintf(" (forced past: %s)", strings.Join(c.Rejects, "; "))
		}
		return text("grabbed %s%s", c.Release.Title, note), nil
	}
	return text("no release titled %q in the current results for %s", want, it.Title), nil
}

func (s *Server) handleRemove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ref, err := req.RequireString("item")
	if err != nil {
		return fail(err), nil
	}
	it, err := s.resolve(ref)
	if err != nil {
		return fail(err), nil
	}
	opt := app.RemoveOptions{KeepFiles: req.GetBool("keep_files", false)}
	if req.GetBool("dry_run", false) {
		return lines("would remove "+it.Title+":", s.app.RemovePlan(it, opt), ""), nil
	}
	steps, err := s.app.Remove(ctx, it, opt)
	out := make([]string, 0, len(steps))
	for _, st := range steps {
		out = append(out, st.String())
	}
	if err != nil {
		return lines("removing "+it.Title+" failed partway:", out, err.Error()), nil
	}
	return lines("removed "+it.Title, out, "removed "+it.Title), nil
}

func (s *Server) handleStatus(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h := s.app.Health(ctx)
	var b strings.Builder
	fmt.Fprintf(&b, "prowlarr: %s\n", h.Prowlarr)
	for _, i := range h.Indexers {
		fmt.Fprintf(&b, "  - %s\n", i.Name)
	}
	fmt.Fprintf(&b, "qbittorrent: %s\nbutaca-parse: %s\n", h.QBittorrent, h.Parse)
	fmt.Fprintf(&b, "movies: %s\nseries: %s\ndownloads: %s\n",
		s.app.Cfg.Paths.Movies, s.app.Cfg.Paths.TV, s.app.Cfg.Paths.Downloads)
	if h.Hardlinkable {
		b.WriteString("hardlinks: ok\n")
	} else {
		fmt.Fprintf(&b, "hardlinks: UNAVAILABLE - %s\n", h.HardlinkNote)
	}
	queue, err := s.app.Store.PendingQueue()
	if err != nil {
		return fail(err), nil
	}
	fmt.Fprintf(&b, "queue: %d\n", len(queue))
	for _, q := range queue {
		fmt.Fprintf(&b, "  %s %.0f%% %s\n", q.State, q.Progress*100, q.ReleaseTitle)
	}
	return mcp.NewToolResultText(strings.TrimRight(b.String(), "\n")), nil
}

func (s *Server) handleSubtitles(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ref := req.GetString("item", "")
	if ref == "" {
		got, err := s.app.FillSubtitleGaps(ctx)
		if err != nil {
			return fail(err), nil
		}
		return lines("subtitle sweep", got, "every file already has the configured subtitles"), nil
	}
	it, err := s.resolve(ref)
	if err != nil {
		return fail(err), nil
	}
	got, err := s.app.SubtitlesForItem(ctx, it)
	if err != nil {
		return fail(err), nil
	}
	return lines(it.Title, got, "nothing to fetch: "+it.Title+" already has its subtitles"), nil
}

func (s *Server) handleImport(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	got, err := s.app.ImportReady(ctx)
	if err != nil {
		return fail(err), nil
	}
	return lines("imported", got, "nothing is ready to import"), nil
}

func orDash(s string) string {
	if s == "" {
		return "?"
	}
	return s
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
