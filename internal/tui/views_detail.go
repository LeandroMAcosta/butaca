package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

type detailState struct {
	item  *store.Item
	langs store.Languages
	files []*store.File
}

// viewDetail shows everything the list has to leave out, including the full
// language lists that the list view truncates.
func (b *browser) viewDetail() string {
	d := b.detail
	if d == nil {
		return ""
	}
	var s strings.Builder
	fmt.Fprintf(&s, "%s\n", titleStyle.Render(fmt.Sprintf("%s (%d)", d.item.Title, d.item.Year)))

	profile := "global rules"
	for _, p := range b.profiles {
		if p.ID == d.item.ProfileID {
			profile = p.Name
		}
	}
	fmt.Fprintf(&s, "%s\n\n", dimStyle.Render(fmt.Sprintf(
		"%s · %s · original language %s · profile %s · added from %s",
		d.item.Kind, d.item.State, orDash(d.item.OriginalLanguage), profile, orDash(d.item.Source))))

	if len(d.files) == 0 {
		s.WriteString(warnStyle.Render("no file on disk") + "\n")
	}
	for _, f := range d.files {
		fmt.Fprintf(&s, "%s  %s\n", library.HumanSize(f.Size), f.Path)
	}

	s.WriteString("\n")
	if d.langs.Empty() {
		s.WriteString(dimStyle.Render("no track data — run `butaca scan-tracks`") + "\n")
	} else {
		fmt.Fprintf(&s, "%s %s\n", labelStyle.Render("audio    "), wrapLangs(d.langs.Audio, b.width-12))
		fmt.Fprintf(&s, "%s %s\n", labelStyle.Render("subtitles"), wrapLangs(d.langs.Subtitles, b.width-12))
	}

	s.WriteString("\n" + dimStyle.Render("enter open in "+fileManager()+" · esc back"))
	return s.String()
}

// wrapLangs prints every language, folded to the available width. The detail
// view is exactly where the truncation used in lists must not happen.
func wrapLangs(langs []string, width int) string {
	if len(langs) == 0 {
		return "-"
	}
	if width < 20 {
		width = 20
	}
	var lines []string
	cur := ""
	for _, l := range langs {
		if len(cur)+len(l)+1 > width {
			lines = append(lines, cur)
			cur = ""
		}
		if cur != "" {
			cur += " "
		}
		cur += l
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return strings.Join(lines, "\n          ") + dimStyle.Render(fmt.Sprintf("  (%d)", len(langs)))
}

type confirmState struct {
	item  *store.Item
	opt   app.RemoveOptions
	plan  []string
	title string
}

// viewConfirm spells out precisely what a deletion will destroy. Because the
// library entry and the download are hardlinks to the same bytes, half a
// deletion frees nothing, so the list has to be explicit.
func (b *browser) viewConfirm() string {
	c := b.confirm
	if c == nil {
		return ""
	}
	var s strings.Builder
	s.WriteString(warnStyle.Render(c.title) + "\n\n")
	for _, p := range c.plan {
		fmt.Fprintf(&s, "  · %s\n", p)
	}
	s.WriteString("\n")
	if c.opt.KeepFiles {
		s.WriteString(dimStyle.Render("files and torrent will be kept") + "\n")
	}
	s.WriteString("\n" + dimStyle.Render("y confirm · k keep files · esc cancel"))
	return s.String()
}

func (b *browser) confirmKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := b.confirm
	switch k.String() {
	case "y", "Y", "enter":
		b.mode = modeList
		b.working = true
		b.status = "removing " + c.item.Title + "…"
		return b, b.removeItem(c.item, c.opt)
	case "k":
		// Toggling re-computes the plan, so the list always matches the action.
		c.opt.KeepFiles = !c.opt.KeepFiles
		if b.app != nil {
			c.plan = b.app.RemovePlan(c.item, c.opt)
		}
		return b, nil
	case "n", "N", "esc", "q":
		b.mode = modeList
		b.confirm = nil
		b.status = "cancelled"
	}
	return b, nil
}

type searchState struct {
	item *store.Item
	list *candidateList
}

func newSearchState(it *store.Item, cands []decide.Candidate) *searchState {
	return &searchState{item: it, list: newCandidateList(cands)}
}

func (b *browser) searchKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := b.search
	switch k.String() {
	case "esc", "q":
		b.mode = modeList
		b.search = nil
	case "up", "k":
		s.list.move(-1)
	case "down", "j":
		s.list.move(1)
	case "x":
		s.list.toggleRejected()
	case "enter":
		if c, ok := s.list.selected(); ok {
			b.mode = modeList
			b.working = true
			b.status = "grabbing " + c.Release.Title + "…"
			return b, b.grabRelease(s.item, c)
		}
	}
	return b, nil
}

func sortCandidates(cands []decide.Candidate) {
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0; j-- {
			a, bb := cands[j-1], cands[j]
			better := (bb.Accepted() && !a.Accepted()) ||
				(bb.Accepted() == a.Accepted() && bb.Score > a.Score)
			if !better {
				break
			}
			cands[j-1], cands[j] = cands[j], cands[j-1]
		}
	}
}

// detailKey handles the detail view. Enter reveals the item on disk rather than
// closing the view: once you are looking at a film's files, opening them is the
// obvious next step, and escape already goes back.
func (b *browser) detailKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "enter", "o":
		if b.detail == nil {
			return b, nil
		}
		path, isFile := b.detail.revealTarget()
		return b, reveal(path, isFile)
	case "esc", "q", "?":
		b.mode = modeList
	}
	return b, nil
}
