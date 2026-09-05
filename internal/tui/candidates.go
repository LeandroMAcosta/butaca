package tui

import (
	"fmt"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/library"
)

// candidateList is the shared release picker used by the search and add
// screens.
//
// A Prowlarr search fans out to every tracker, so a popular film returns a few
// hundred rows of which most are rejects and many are the same release seen
// through two indexers. Showing all of that raw is unreadable, so the list
// deduplicates, collapses each release to one line, and hides rejects until
// asked.
type candidateList struct {
	all      []decide.Candidate // deduplicated, accepted first
	accepted int
	rejected int
	// mirrors counts how many indexers carried each release.
	mirrors      map[string]int
	showRejected bool
	cursor       int
}

func newCandidateList(cands []decide.Candidate) *candidateList {
	deduped, mirrors := dedupeCandidates(cands)
	sortCandidates(deduped)
	l := &candidateList{all: deduped, mirrors: mirrors}
	for _, c := range deduped {
		if c.Accepted() {
			l.accepted++
		} else {
			l.rejected++
		}
	}
	return l
}

// dedupeCandidates collapses the same release seen through several indexers,
// keeping the best-seeded copy.
func dedupeCandidates(cands []decide.Candidate) ([]decide.Candidate, map[string]int) {
	best := map[string]int{}
	mirrors := map[string]int{}
	var out []decide.Candidate

	for _, c := range cands {
		key := releaseKey(c.Release.Title)
		mirrors[key]++
		idx, seen := best[key]
		if !seen {
			best[key] = len(out)
			out = append(out, c)
			continue
		}
		if c.Release.Seeders > out[idx].Release.Seeders {
			out[idx] = c
		}
	}
	return out, mirrors
}

// releaseKey normalises a release name so trivial punctuation differences do
// not defeat deduplication.
func releaseKey(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// visible returns the rows to draw, honouring the rejected toggle.
func (l *candidateList) visible() []decide.Candidate {
	if l.showRejected {
		return l.all
	}
	out := make([]decide.Candidate, 0, l.accepted)
	for _, c := range l.all {
		if c.Accepted() {
			out = append(out, c)
		}
	}
	return out
}

func (l *candidateList) selected() (decide.Candidate, bool) {
	rows := l.visible()
	if l.cursor < 0 || l.cursor >= len(rows) {
		return decide.Candidate{}, false
	}
	return rows[l.cursor], true
}

func (l *candidateList) move(delta int) {
	n := len(l.visible())
	l.cursor += delta
	if l.cursor < 0 {
		l.cursor = 0
	}
	if l.cursor >= n {
		l.cursor = max(0, n-1)
	}
}

func (l *candidateList) toggleRejected() {
	l.showRejected = !l.showRejected
	l.cursor = 0
}

// render draws the list: one line per release, plus the reasons underneath a
// rejected one, which is the only case where a second line earns its place.
func (l *candidateList) render(width, maxRows int) string {
	rows := l.visible()
	if len(rows) == 0 {
		if l.rejected > 0 {
			return warnStyle.Render("  nothing passed your rules") + "\n" +
				dimStyle.Render(fmt.Sprintf("  press x to see why %d releases were rejected", l.rejected))
		}
		return warnStyle.Render("  nothing found")
	}
	if maxRows < 3 {
		maxRows = 3
	}

	titleWidth := width - 42
	if titleWidth < 24 {
		titleWidth = 24
	}

	var s strings.Builder
	s.WriteString(headerRow(fmt.Sprintf("   %5s  %-*s %-7s %9s %7s", "SCORE", titleWidth, "RELEASE", "QUALITY", "SIZE", "SEEDS")))

	used := 0
	for i, c := range rows {
		if used >= maxRows {
			s.WriteString(dimStyle.Render(fmt.Sprintf("   … and %d more", len(rows)-i)) + "\n")
			break
		}
		mirror := ""
		if n := l.mirrors[releaseKey(c.Release.Title)]; n > 1 {
			mirror = fmt.Sprintf(" ×%d", n)
		}
		line := fmt.Sprintf("%5d  %-*s %-7s %9s %6d%s",
			c.Score, titleWidth, truncate(c.Release.Title, titleWidth),
			orDash(c.Parsed.ScreenSize), library.HumanSize(c.Release.Size), c.Release.Seeders, mirror)
		if !c.Accepted() {
			line = warnStyle.Render("✗") + " " + line
		} else {
			line = "  " + line
		}
		s.WriteString(cursorFor(i == l.cursor) + render(i == l.cursor, line) + "\n")
		used++

		if !c.Accepted() && used < maxRows {
			s.WriteString(dimStyle.Render("       "+strings.Join(c.Rejects, " · ")) + "\n")
			used++
		}
	}
	return s.String()
}

// summary is the one line that tells the user what butaca decided.
func (l *candidateList) summary(query string) string {
	total := 0
	for _, n := range l.mirrors {
		total += n
	}
	parts := []string{fmt.Sprintf("%d releases", total)}
	if total != len(l.all) {
		parts = append(parts, fmt.Sprintf("%d after merging duplicates", len(l.all)))
	}
	parts = append(parts, fmt.Sprintf("%d pass your rules", l.accepted))
	return fmt.Sprintf("%s — %s", query, strings.Join(parts, " · "))
}

// pick describes what pressing enter right now would do.
func (l *candidateList) pick() string {
	c, ok := l.selected()
	if !ok {
		return ""
	}
	if !c.Accepted() {
		return warnStyle.Render("this one breaks your rules — enter grabs it anyway")
	}
	return dimStyle.Render("enter grabs " + truncate(c.Release.Title, 60))
}
