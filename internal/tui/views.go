package tui

import (
	"fmt"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func (b *browser) View() string {
	var s strings.Builder
	s.WriteString(b.header())
	s.WriteString("\n")

	switch b.mode {
	case modeDetail:
		s.WriteString(b.viewDetail())
		return s.String()
	case modeConfirm:
		s.WriteString(b.viewConfirm())
		return s.String()
	case modeSearch:
		s.WriteString(b.viewSearch())
		return s.String()
	case modeHelp:
		s.WriteString(viewHelp())
		return s.String()
	}

	s.WriteString(b.tabBar())
	s.WriteString("\n\n")
	switch b.tab {
	case tabQueue:
		s.WriteString(b.viewQueue())
	case tabDiscover:
		s.WriteString(b.viewDiscover())
	case tabProfiles:
		s.WriteString(b.viewProfiles())
	default:
		s.WriteString(b.viewItems())
	}
	s.WriteString("\n")
	s.WriteString(b.footer())
	return s.String()
}

// header keeps disk pressure permanently visible: it is the number that
// decides whether the next download is a good idea.
func (b *browser) header() string {
	left := titleStyle.Render("butaca")
	disk := dimStyle.Render("disk unknown")
	if b.disk.Total > 0 {
		style := dimStyle
		if b.disk.UsedPercent() >= 90 {
			style = warnStyle
		}
		disk = style.Render(fmt.Sprintf("%s free of %s (%.0f%% used)",
			library.HumanSize(b.disk.Free), library.HumanSize(b.disk.Total), b.disk.UsedPercent()))
	}
	lib := dimStyle.Render(fmt.Sprintf("library %s", library.HumanSize(b.occupied)))

	svc := ""
	if b.health.Prowlarr != "" {
		bad := b.health.Prowlarr != "ok"
		if !strings.HasPrefix(b.health.QBittorrent, "ok") || b.health.Parse != "ok" {
			bad = true
		}
		if bad {
			svc = warnStyle.Render("  services degraded")
		}
	}
	return fmt.Sprintf("%s   %s   %s%s", left, disk, lib, svc)
}

func (b *browser) tabBar() string {
	var parts []string
	counts := []int{len(b.items), len(b.watchlist), len(b.queue), len(b.suggestions), len(b.profiles)}
	for t := tabLibrary; t < numTabs; t++ {
		label := fmt.Sprintf(" %d %s %d ", int(t)+1, t, counts[t])
		if t == b.tab {
			parts = append(parts, tabActive.Render(label))
		} else {
			parts = append(parts, tabIdle.Render(label))
		}
	}
	return strings.Join(parts, "")
}

func (b *browser) footer() string {
	var s strings.Builder
	if b.filtering || b.filter != "" {
		marker := "filter: "
		if b.filtering {
			marker = "filter (typing): "
		}
		s.WriteString(labelStyle.Render(marker) + b.filter + "\n")
	}
	switch {
	case b.err != nil:
		s.WriteString(warnStyle.Render("error: "+b.err.Error()) + "\n")
	case b.working:
		s.WriteString(dimStyle.Render("⋯ "+b.status) + "\n")
	default:
		s.WriteString(dimStyle.Render(b.status) + "\n")
	}
	s.WriteString(dimStyle.Render("↑↓ move · tab switch · enter detail · s search · d delete · w watchlist · p profile · / filter · i import · ? help · q quit"))
	return s.String()
}

// viewItems renders the Library and Watchlist tabs, which share a shape.
func (b *browser) viewItems() string {
	rows := b.rows()
	if len(rows) == 0 {
		if b.filter != "" {
			return dimStyle.Render(fmt.Sprintf("  nothing matches %q", b.filter))
		}
		if b.tab == tabWatchlist {
			return dimStyle.Render("  the watchlist is empty — import one with `butaca letterboxd import`")
		}
		return dimStyle.Render("  the library is empty — add something with `butaca add`")
	}

	var s strings.Builder
	s.WriteString(headerRow(fmt.Sprintf("  %-38s %-5s %-4s %-14s %10s", "TITLE", "YEAR", "LANG", "SUBTITLES", "SIZE")))
	for i, it := range b.visible(rows) {
		idx := i + b.scrollOffset(len(rows))
		size := warnStyle.Render("missing")
		if it.FileCount > 0 {
			size = library.HumanSize(it.SizeBytes)
		}
		subs := store.Summary(b.langs[it.ID].Subtitles, 3)
		line := fmt.Sprintf("%-38s %-5d %-4s %-14s %10s",
			truncate(it.Title, 38), it.Year, orDash(it.OriginalLanguage), truncate(subs, 14), size)
		s.WriteString(cursorFor(idx == b.cursor[b.tab]) + render(idx == b.cursor[b.tab], line) + "\n")
	}
	return s.String()
}

func (b *browser) viewQueue() string {
	if len(b.queue) == 0 {
		return dimStyle.Render("  nothing downloading")
	}
	var s strings.Builder
	s.WriteString(headerRow(fmt.Sprintf("  %-50s %-14s %8s %10s", "RELEASE", "STATE", "SIZE", "PROGRESS")))
	for i, q := range b.queue {
		line := fmt.Sprintf("%-50s %-14s %8s %9.1f%%",
			truncate(q.ReleaseTitle, 50), q.State, library.HumanSize(q.Size), q.Progress*100)
		s.WriteString(cursorFor(i == b.cursor[b.tab]) + render(i == b.cursor[b.tab], line) + "\n")
		s.WriteString("   " + progressBar(q.Progress, 46) + "\n")
	}
	return s.String()
}

func (b *browser) viewDiscover() string {
	if len(b.suggestions) == 0 {
		if b.working {
			return dimStyle.Render("  asking TMDB…")
		}
		return dimStyle.Render("  no suggestions yet — press D (needs a TMDB API key)")
	}
	var s strings.Builder
	s.WriteString(headerRow(fmt.Sprintf("  %-40s %-5s %-5s %-4s %s", "TITLE", "YEAR", "SCORE", "LANG", "BECAUSE OF")))
	for i, sg := range b.suggestions {
		line := fmt.Sprintf("%-40s %-5d %-5.1f %-4s %s",
			truncate(sg.Movie.Title, 40), sg.Movie.Year(), sg.Movie.VoteAverage,
			orDash(sg.Movie.OriginalLanguage), truncate(strings.Join(sg.Seeds, ", "), 40))
		s.WriteString(cursorFor(i == b.cursor[b.tab]) + render(i == b.cursor[b.tab], line) + "\n")
	}
	s.WriteString("\n" + dimStyle.Render("  enter adds to the watchlist · D refreshes"))
	return s.String()
}

func (b *browser) viewProfiles() string {
	if len(b.profiles) == 0 {
		return dimStyle.Render("  no profiles — create one with `butaca profile set <name>`")
	}
	var s strings.Builder
	s.WriteString(headerRow(fmt.Sprintf("  %-18s %-16s %-12s %-14s %s", "NAME", "LETTERBOXD", "AUDIO", "QUALITY", "SUBTITLES")))
	for i, p := range b.profiles {
		name := p.Name
		if p.IsDefault {
			name += " *"
		}
		mode := p.LanguageMode
		if p.PreferLanguage != "" {
			mode += ":" + p.PreferLanguage
		}
		line := fmt.Sprintf("%-18s %-16s %-12s %-14s %s",
			truncate(name, 18), truncate(orDash(p.LetterboxdUser), 16), mode,
			orDash(strings.Join(p.Resolutions, ",")), orDash(strings.Join(p.SubtitleLangs, ",")))
		s.WriteString(cursorFor(i == b.cursor[b.tab]) + render(i == b.cursor[b.tab], line) + "\n")
	}
	return s.String()
}

func (b *browser) viewSearch() string {
	s := b.search
	if s == nil {
		return ""
	}
	accepted := 0
	for _, c := range s.cands {
		if c.Accepted() {
			accepted++
		}
	}
	var out strings.Builder
	fmt.Fprintf(&out, "%s\n%s\n\n",
		titleStyle.Render("Releases for "+s.item.Title),
		dimStyle.Render(fmt.Sprintf("%d found, %d passed the rules", len(s.cands), accepted)))

	limit := b.height - 12
	if limit < 5 {
		limit = 5
	}
	for i, c := range s.cands {
		if i >= limit {
			fmt.Fprintf(&out, "%s\n", dimStyle.Render(fmt.Sprintf("  … and %d more", len(s.cands)-i)))
			break
		}
		mark := warnStyle.Render("REJECT")
		if c.Accepted() {
			mark = okStyle.Render("ok    ")
		}
		line := fmt.Sprintf("%s %5d  %s", mark, c.Score, truncate(c.Release.Title, b.width-24))
		out.WriteString(cursorFor(i == s.cursor) + render(i == s.cursor, line) + "\n")
		fmt.Fprintf(&out, "        %s\n", dimStyle.Render(fmt.Sprintf("%s · %s · %d seeders · %s",
			orDash(c.Parsed.ScreenSize), library.HumanSize(c.Release.Size), c.Release.Seeders, c.Release.Indexer)))
		// The reason a release was rejected is the whole point of this screen.
		for _, r := range c.Rejects {
			fmt.Fprintf(&out, "        %s\n", warnStyle.Render("- "+r))
		}
	}
	out.WriteString("\n" + dimStyle.Render("enter grabs the highlighted release, rules or no rules · esc back"))
	return out.String()
}

func viewHelp() string {
	rows := [][2]string{
		{"↑↓ / j k", "move"},
		{"g / G", "jump to first or last"},
		{"tab / 1-5", "switch tab"},
		{"/", "filter by title (esc clears)"},
		{"enter", "detail, or accept a suggestion"},
		{"s", "search releases, with scores and rejection reasons"},
		{"d", "delete: catalog, folder and torrent"},
		{"w", "move between watchlist and monitored"},
		{"p", "cycle the assigned profile"},
		{"i", "import finished downloads"},
		{"D", "refresh recommendations (Discover tab)"},
		{"r", "refresh"},
		{"q", "quit"},
	}
	var s strings.Builder
	s.WriteString(titleStyle.Render("Keys") + "\n\n")
	for _, r := range rows {
		fmt.Fprintf(&s, "  %-12s %s\n", labelStyle.Render(r[0]), r[1])
	}
	s.WriteString("\n" + dimStyle.Render("The queue refreshes on its own every 2 seconds while something is downloading."))
	s.WriteString("\n\n" + dimStyle.Render("esc back"))
	return s.String()
}
