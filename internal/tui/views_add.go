package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/decide"
)

// addState drives "find me something that is not in the library yet". It is
// deliberately release-first rather than metadata-first: searching Prowlarr for
// a title works without a TMDB key, and the chosen release supplies the year.
type addState struct {
	query    string
	searched bool
	list     *candidateList
}

// addSearched carries the result of looking for an uncatalogued film.
type addSearched struct {
	query string
	cands []decide.Candidate
	err   error
}

func (b *browser) searchNew(query string) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		cands, err := a.SearchNew(contextTODO(), query)
		return addSearched{query: query, cands: cands, err: err}
	}
}

func (b *browser) addRelease(c decide.Candidate, title string) tea.Cmd {
	if b.app == nil {
		return nil
	}
	a := b.app
	return func() tea.Msg {
		it, err := a.AddFromRelease(contextTODO(), c, title)
		if err != nil {
			return actionDone{err: err}
		}
		return actionDone{msg: fmt.Sprintf("added %s (%d) and grabbed %s", it.Title, it.Year, c.Release.Title)}
	}
}

func (b *browser) addKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	s := b.add
	if s == nil {
		b.mode = modeList
		return b, nil
	}

	// Before a search runs, the screen is just a text field.
	if !s.searched {
		switch k.Type {
		case tea.KeyEsc:
			b.mode = modeList
			b.add = nil
		case tea.KeyEnter:
			if strings.TrimSpace(s.query) == "" {
				return b, nil
			}
			b.working = true
			b.status = "searching for " + s.query + "…"
			return b, b.searchNew(s.query)
		case tea.KeyBackspace:
			if s.query != "" {
				r := []rune(s.query)
				s.query = string(r[:len(r)-1])
			}
		case tea.KeyRunes, tea.KeySpace:
			if k.Type == tea.KeySpace {
				s.query += " "
			} else {
				s.query += string(k.Runes)
			}
		}
		return b, nil
	}

	switch k.String() {
	case "esc", "q":
		b.mode = modeList
		b.add = nil
	case "up", "k":
		s.list.move(-1)
	case "down", "j":
		s.list.move(1)
	case "x":
		s.list.toggleRejected()
	case "backspace":
		// Back to editing the query rather than starting over.
		s.searched, s.list = false, nil
	case "enter":
		if c, ok := s.list.selected(); ok {
			b.mode = modeList
			b.working = true
			b.status = "adding " + c.Release.Title + "…"
			cmd := b.addRelease(c, s.query)
			b.add = nil
			return b, cmd
		}
	}
	return b, nil
}

func (b *browser) viewAdd() string {
	s := b.add
	if s == nil {
		return ""
	}
	var out strings.Builder
	out.WriteString(titleStyle.Render("Add a film") + "\n\n")
	// This screen renders without the shared footer, so it has to surface its
	// own errors or a failed search would look like an empty one.
	if b.err != nil {
		out.WriteString(warnStyle.Render("error: "+b.err.Error()) + "\n\n")
	}

	if !s.searched {
		out.WriteString(labelStyle.Render("title: ") + s.query + "▌\n\n")
		out.WriteString(dimStyle.Render(
			"Searches every tracker through Prowlarr. A year helps: \"Amelie 2001\".\n" +
				"No TMDB key needed — the release supplies the year, and TMDB fills in\n" +
				"the rest when it is configured.\n\n" +
				"enter search · esc cancel"))
		return out.String()
	}

	out.WriteString(dimStyle.Render(s.list.summary(s.query)) + "\n")
	out.WriteString(dimStyle.Render("Ordered by butaca's score: the top one is what it would pick.") + "\n\n")
	out.WriteString(s.list.render(b.width, b.height-13))
	out.WriteString("\n" + s.list.pick() + "\n")
	out.WriteString(dimStyle.Render(
		rejectedToggleHint(s.list) + " · backspace edit the title · esc cancel"))
	return out.String()
}
