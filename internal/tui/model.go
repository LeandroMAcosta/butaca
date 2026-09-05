package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/recommend"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

type tab int

const (
	tabLibrary tab = iota
	tabWatchlist
	tabQueue
	tabDiscover
	tabProfiles
	numTabs
)

func (t tab) String() string {
	return [...]string{"Library", "Watchlist", "Queue", "Discover", "Profiles"}[t]
}

// mode is which layer of the interface has the keyboard.
type mode int

const (
	modeList mode = iota
	modeDetail
	modeSearch
	modeConfirm
	modeHelp
	modeAdd
)

// queuePoll is how often the queue refreshes while something is downloading.
// Without it the progress column is frozen until the user presses a key, which
// was the main complaint about the first version of this screen.
const queuePoll = 2 * time.Second

type browser struct {
	app *app.App

	tab  tab
	mode mode

	items     []*store.Item
	watchlist []*store.Item
	// langs is loaded with the item list rather than looked up per row: the
	// view repaints on every keystroke, and a query per row per frame is both
	// slow and impossible to test without a database.
	langs       map[int64]store.Languages
	queue       []*store.QueueEntry
	suggestions []recommend.Suggestion
	profiles    []*store.Profile

	disk     library.DiskUsage
	occupied int64
	health   app.Health

	cursor    map[tab]int
	filter    string
	filtering bool

	detail  *detailState
	search  *searchState
	confirm *confirmState
	add     *addState

	status  string
	err     error
	working bool
	polling bool

	width, height int
}

// NewBrowser builds the catalog browser.
func NewBrowser(a *app.App) tea.Model {
	return &browser{
		app:    a,
		cursor: map[tab]int{},
		width:  100,
		height: 30,
		status: "loading…",
	}
}

// RunBrowser starts the browser in the alternate screen buffer.
func RunBrowser(a *app.App) error {
	_, err := tea.NewProgram(NewBrowser(a), tea.WithAltScreen()).Run()
	return err
}

func (b *browser) Init() tea.Cmd { return b.reload() }

// rows returns what the active tab is listing, after filtering.
func (b *browser) rows() []*store.Item {
	var src []*store.Item
	switch b.tab {
	case tabLibrary:
		src = b.items
	case tabWatchlist:
		src = b.watchlist
	default:
		return nil
	}
	return filterItems(src, b.filter)
}

func (b *browser) rowCount() int {
	switch b.tab {
	case tabQueue:
		return len(b.queue)
	case tabDiscover:
		return len(b.suggestions)
	case tabProfiles:
		return len(b.profiles)
	default:
		return len(b.rows())
	}
}

// selected returns the highlighted catalog item, or nil on a tab that has none.
func (b *browser) selected() *store.Item {
	rows := b.rows()
	i := b.cursor[b.tab]
	if i < 0 || i >= len(rows) {
		return nil
	}
	return rows[i]
}

func (b *browser) clampCursor() {
	n := b.rowCount()
	if i := b.cursor[b.tab]; i >= n {
		b.cursor[b.tab] = max(0, n-1)
	}
}

func (b *browser) setErr(err error) {
	b.working = false
	if err != nil {
		b.err = err
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
