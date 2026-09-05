package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (b *browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		b.width, b.height = m.Width, m.Height
		return b, nil

	case refreshed:
		b.working = false
		if m.err != nil {
			b.err = m.err
			return b, nil
		}
		b.items, b.watchlist, b.queue = m.items, m.watchlist, m.queue
		b.profiles, b.disk, b.occupied = m.profiles, m.disk, m.occupied
		b.langs = m.langs
		b.clampCursor()
		b.status = fmt.Sprintf("%d in the library · %d on the watchlist · %d downloading",
			len(m.items), len(m.watchlist), len(m.queue))
		// Only poll while something is actually transferring.
		if len(b.queue) > 0 && !b.polling {
			b.polling = true
			return b, tea.Batch(tick(), b.checkHealth())
		}
		if len(b.queue) == 0 {
			b.polling = false
		}
		return b, nil

	case healthMsg:
		b.health = m.health
		return b, nil

	case tickMsg:
		if len(b.queue) == 0 {
			b.polling = false
			return b, nil
		}
		return b, tea.Batch(b.syncQueue(), tick())

	case actionDone:
		b.working = false
		if m.err != nil {
			b.err = m.err
		} else if m.msg != "" {
			b.status = m.msg
			b.err = nil
		}
		return b, b.reload()

	case searchDone:
		b.working = false
		if m.err != nil {
			b.err = m.err
			return b, nil
		}
		b.search = newSearchState(m.item, m.cands)
		b.mode = modeSearch
		return b, nil

	case detailDone:
		b.working = false
		if m.err != nil {
			b.err = m.err
			return b, nil
		}
		b.detail = &detailState{item: m.item, langs: m.langs, files: m.files}
		b.mode = modeDetail
		return b, nil

	case discoverDone:
		b.working = false
		if m.err != nil {
			b.err = m.err
			b.status = "discover failed"
			return b, nil
		}
		b.suggestions = m.suggestions
		b.status = fmt.Sprintf("%d suggestions from your library", len(m.suggestions))
		return b, nil

	case tea.KeyMsg:
		return b.handleKey(m)
	}
	return b, nil
}

func (b *browser) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A filter box swallows almost everything while it is open.
	if b.filtering {
		switch k.Type {
		case tea.KeyEnter, tea.KeyEsc:
			b.filtering = false
			if k.Type == tea.KeyEsc {
				b.filter = ""
			}
			b.clampCursor()
		case tea.KeyBackspace:
			if b.filter != "" {
				b.filter = b.filter[:len(b.filter)-1]
			}
			b.cursor[b.tab] = 0
		case tea.KeyRunes:
			b.filter += string(k.Runes)
			b.cursor[b.tab] = 0
		}
		return b, nil
	}

	switch b.mode {
	case modeSearch:
		return b.searchKey(k)
	case modeConfirm:
		return b.confirmKey(k)
	case modeDetail:
		return b.detailKey(k)
	case modeHelp:
		switch k.String() {
		case "q", "esc", "enter", "?":
			b.mode = modeList
		}
		return b, nil
	}
	return b.listKey(k)
}
