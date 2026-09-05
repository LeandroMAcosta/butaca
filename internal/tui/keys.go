package tui

// List-mode key handling: navigation, tab switching and the actions that
// operate on the highlighted row.

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func (b *browser) listKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return b, tea.Quit
	case "esc":
		if b.filter != "" {
			b.filter = ""
			b.clampCursor()
		}
		return b, nil
	case "?":
		b.mode = modeHelp
		return b, nil
	case "tab", "right", "l":
		b.tab = (b.tab + 1) % numTabs
		return b, b.enterTab()
	case "shift+tab", "left", "h":
		b.tab = (b.tab + numTabs - 1) % numTabs
		return b, b.enterTab()
	case "1", "2", "3", "4", "5":
		b.tab = tab(k.String()[0] - '1')
		return b, b.enterTab()
	case "up", "k":
		if b.cursor[b.tab] > 0 {
			b.cursor[b.tab]--
		}
		return b, nil
	case "down", "j":
		if b.cursor[b.tab] < b.rowCount()-1 {
			b.cursor[b.tab]++
		}
		return b, nil
	case "g":
		b.cursor[b.tab] = 0
		return b, nil
	case "G":
		b.cursor[b.tab] = max(0, b.rowCount()-1)
		return b, nil
	case "/":
		if b.tab == tabLibrary || b.tab == tabWatchlist {
			b.filtering = true
		}
		return b, nil
	case "r":
		b.status = "refreshing…"
		return b, tea.Batch(b.reload(), b.checkHealth())
	case "i":
		b.working = true
		b.status = "importing…"
		return b, b.importReady()
	case "enter":
		return b.openSelection()
	case "s":
		if it := b.selected(); it != nil {
			b.working, b.err = true, nil
			b.status = "searching " + it.Title + "…"
			return b, b.searchItem(it)
		}
		return b, nil
	case "d":
		return b.startRemove()
	case "w":
		return b.toggleState()
	case "p":
		return b.cycleProfile()
	case "D":
		if b.tab == tabDiscover {
			b.working = true
			b.status = "asking TMDB…"
			return b, b.discover()
		}
		return b, nil
	}
	return b, nil
}

// enterTab loads whatever the tab needs on first visit.
func (b *browser) enterTab() tea.Cmd {
	b.clampCursor()
	if b.tab == tabDiscover && len(b.suggestions) == 0 && !b.working {
		b.working = true
		b.status = "asking TMDB…"
		return b.discover()
	}
	return nil
}

func (b *browser) openSelection() (tea.Model, tea.Cmd) {
	switch b.tab {
	case tabDiscover:
		i := b.cursor[b.tab]
		if i < len(b.suggestions) {
			b.working = true
			return b, b.acceptSuggestion(b.suggestions[i])
		}
	default:
		if it := b.selected(); it != nil {
			b.working = true
			return b, b.loadDetail(it)
		}
	}
	return b, nil
}

func (b *browser) startRemove() (tea.Model, tea.Cmd) {
	it := b.selected()
	if it == nil || b.app == nil {
		return b, nil
	}
	opt := app.RemoveOptions{}
	b.confirm = &confirmState{
		item:  it,
		opt:   opt,
		plan:  b.app.RemovePlan(it, opt),
		title: "Remove " + it.Title + "?",
	}
	b.mode = modeConfirm
	return b, nil
}

func (b *browser) toggleState() (tea.Model, tea.Cmd) {
	it := b.selected()
	if it == nil {
		return b, nil
	}
	next := store.StateWatchlist
	if it.State == store.StateWatchlist {
		next = store.StateMonitored
	}
	b.working = true
	return b, b.setState(it, next)
}

// cycleProfile steps an item through the available profiles, ending on none.
func (b *browser) cycleProfile() (tea.Model, tea.Cmd) {
	it := b.selected()
	if it == nil || len(b.profiles) == 0 {
		if it != nil {
			b.status = "no profiles yet — create one with `butaca profile set <name>`"
		}
		return b, nil
	}
	idx := -1
	for i, p := range b.profiles {
		if p.ID == it.ProfileID {
			idx = i
			break
		}
	}
	idx++
	if idx >= len(b.profiles) {
		b.working = true
		return b, b.assignProfile(it, 0, "none (global rules)")
	}
	b.working = true
	return b, b.assignProfile(it, b.profiles[idx].ID, b.profiles[idx].Name)
}

// filterItems narrows a list by a case-insensitive title substring.
func filterItems(items []*store.Item, filter string) []*store.Item {
	if filter == "" {
		return items
	}
	needle := strings.ToLower(filter)
	out := make([]*store.Item, 0, len(items))
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Title), needle) {
			out = append(out, it)
		}
	}
	return out
}
