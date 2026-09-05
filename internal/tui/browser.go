package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

type tab int

const (
	tabLibrary tab = iota
	tabQueue
	numTabs
)

func (t tab) String() string {
	switch t {
	case tabLibrary:
		return "Library"
	default:
		return "Queue"
	}
}

var (
	tabActive   = lipgloss.NewStyle().Bold(true).Underline(true)
	tabIdle     = lipgloss.NewStyle().Faint(true)
	rowSelected = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	warnStyle   = lipgloss.NewStyle().Bold(true)
)

// refreshed carries a reload of everything the browser displays.
type refreshed struct {
	items []*store.Item
	queue []*store.QueueEntry
	err   error
}

// actionDone reports the outcome of a background action.
type actionDone struct {
	msg string
	err error
}

type browser struct {
	app     *app.App
	tab     tab
	items   []*store.Item
	queue   []*store.QueueEntry
	cursor  int
	status  string
	working bool
	err     error
	height  int
}

// NewBrowser builds the catalog browser.
func NewBrowser(a *app.App) tea.Model {
	return browser{app: a, height: 20, status: "loading…"}
}

// RunBrowser starts the browser in the alternate screen buffer.
func RunBrowser(a *app.App) error {
	_, err := tea.NewProgram(NewBrowser(a), tea.WithAltScreen()).Run()
	return err
}

func (b browser) Init() tea.Cmd { return b.reload() }

func (b browser) reload() tea.Cmd {
	return func() tea.Msg {
		items, err := b.app.Store.ListItems("")
		if err != nil {
			return refreshed{err: err}
		}
		queue, err := b.app.Store.PendingQueue()
		return refreshed{items: items, queue: queue, err: err}
	}
}

func (b browser) searchSelected() tea.Cmd {
	it := b.selected()
	if it == nil {
		return nil
	}
	return func() tea.Msg {
		lines, err := b.app.SearchMissingFor(context.Background(), it)
		if err != nil {
			return actionDone{err: err}
		}
		if len(lines) == 0 {
			return actionDone{msg: it.Title + ": nothing to grab"}
		}
		return actionDone{msg: strings.Join(lines, "; ")}
	}
}

func (b browser) importReady() tea.Cmd {
	return func() tea.Msg {
		lines, err := b.app.ImportReady(context.Background())
		if err != nil {
			return actionDone{err: err}
		}
		if len(lines) == 0 {
			return actionDone{msg: "nothing ready to import"}
		}
		return actionDone{msg: strings.Join(lines, "; ")}
	}
}

func (b browser) selected() *store.Item {
	if b.tab != tabLibrary || b.cursor >= len(b.items) {
		return nil
	}
	return b.items[b.cursor]
}

func (b browser) rowCount() int {
	if b.tab == tabLibrary {
		return len(b.items)
	}
	return len(b.queue)
}

func (b browser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		b.height = m.Height
		return b, nil

	case refreshed:
		b.items, b.queue, b.err = m.items, m.queue, m.err
		b.working = false
		if b.cursor >= b.rowCount() {
			b.cursor = max(0, b.rowCount()-1)
		}
		if m.err == nil {
			b.status = fmt.Sprintf("%d in the library, %d downloading", len(m.items), len(m.queue))
		}
		return b, nil

	case actionDone:
		b.working = false
		if m.err != nil {
			b.err = m.err
		} else {
			b.status = m.msg
		}
		return b, b.reload()

	case tea.KeyMsg:
		return b.handleKey(m)
	}
	return b, nil
}

func (b browser) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c", "esc":
		return b, tea.Quit
	case "tab":
		b.tab = (b.tab + 1) % numTabs
		b.cursor = 0
		return b, nil
	case "up", "k":
		if b.cursor > 0 {
			b.cursor--
		}
		return b, nil
	case "down", "j":
		if b.cursor < b.rowCount()-1 {
			b.cursor++
		}
		return b, nil
	case "r":
		b.status = "refreshing…"
		return b, b.reload()
	case "s":
		if it := b.selected(); it != nil {
			b.working = true
			b.err = nil
			b.status = "searching " + it.Title + "…"
			return b, b.searchSelected()
		}
		return b, nil
	case "i":
		b.working = true
		b.err = nil
		b.status = "importing…"
		return b, b.importReady()
	}
	return b, nil
}

func (b browser) View() string {
	var s strings.Builder

	tabs := make([]string, 0, numTabs)
	for t := tabLibrary; t < numTabs; t++ {
		label := fmt.Sprintf(" %s ", t)
		if t == b.tab {
			tabs = append(tabs, tabActive.Render(label))
		} else {
			tabs = append(tabs, tabIdle.Render(label))
		}
	}
	s.WriteString(strings.Join(tabs, " "))
	s.WriteString("\n\n")

	if b.tab == tabLibrary {
		s.WriteString(b.libraryRows())
	} else {
		s.WriteString(b.queueRows())
	}

	s.WriteString("\n")
	if b.err != nil {
		s.WriteString(warnStyle.Render("error: "+b.err.Error()) + "\n")
	} else {
		s.WriteString(dimStyle.Render(b.status) + "\n")
	}
	s.WriteString(dimStyle.Render("↑↓ move · tab switch · s search · i import · r refresh · q quit"))
	return s.String()
}

func (b browser) libraryRows() string {
	if len(b.items) == 0 {
		return dimStyle.Render("  the library is empty — add something with `butaca add`") + "\n"
	}
	var s strings.Builder
	for i, it := range b.items {
		state := warnStyle.Render("missing")
		if it.FileCount > 0 {
			state = humanSize(it.SizeBytes)
		}
		line := fmt.Sprintf("%-42s %4d  %-3s %10s",
			truncate(it.Title, 42), it.Year, orDash(it.OriginalLanguage), state)
		s.WriteString(cursorFor(i == b.cursor) + render(i == b.cursor, line) + "\n")
	}
	return s.String()
}

func (b browser) queueRows() string {
	if len(b.queue) == 0 {
		return dimStyle.Render("  nothing downloading") + "\n"
	}
	var s strings.Builder
	for i, q := range b.queue {
		line := fmt.Sprintf("%-52s %-12s %5.1f%%", truncate(q.ReleaseTitle, 52), q.State, q.Progress*100)
		s.WriteString(cursorFor(i == b.cursor) + render(i == b.cursor, line) + "\n")
	}
	return s.String()
}
