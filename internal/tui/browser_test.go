package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/indexer"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/parse"
	"github.com/LeandroMAcosta/butaca/internal/recommend"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func gb(n float64) int64 { return int64(n * float64(int64(1)<<30)) }

// loaded builds a browser holding a realistic snapshot, without touching the
// database or the network.
func loaded() *browser {
	b := &browser{cursor: map[tab]int{}, width: 120, height: 40}
	b.Update(refreshed{
		items: []*store.Item{
			{ID: 1, Title: "Amélie", Year: 2001, OriginalLanguage: "fr", FileCount: 1, SizeBytes: gb(1.94), State: store.StateMonitored},
			{ID: 2, Title: "Taxi Driver", Year: 1976, OriginalLanguage: "en", FileCount: 1, SizeBytes: gb(2.5), State: store.StateMonitored},
			{ID: 3, Title: "Cooking with Huck Botko", OriginalLanguage: "en", State: store.StateMonitored},
		},
		watchlist: []*store.Item{
			{ID: 20, Title: "Vivarium", Year: 2019, State: store.StateWatchlist, Source: "letterboxd"},
			{ID: 21, Title: "1984", Year: 1956, State: store.StateWatchlist, Source: "letterboxd"},
		},
		queue: []*store.QueueEntry{
			{ItemID: 2, ReleaseTitle: "Taxi.Driver.1976.1080p", State: "downloading", Progress: 0.42, Size: gb(2.5)},
		},
		profiles: []*store.Profile{
			{ID: 1, Name: "Leandro", LetterboxdUser: "leandroacosta", LanguageMode: "original", IsDefault: true},
			{ID: 2, Name: "Casa", LanguageMode: "prefer", PreferLanguage: "es"},
		},
		disk:     library.DiskUsage{Path: "/Users/x/Movies", Total: gb(460), Free: gb(102), Used: gb(358)},
		occupied: gb(129),
	})
	return b
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func press(b *browser, keys ...string) *browser {
	for _, k := range keys {
		b.Update(key(k))
	}
	return b
}

// The header carries the number that decides whether another download is wise.
func TestHeaderShowsDiskAndLibrarySize(t *testing.T) {
	v := loaded().View()
	for _, want := range []string{"102.0 GB free", "460.0 GB", "library 129.0 GB"} {
		if !strings.Contains(v, want) {
			t.Errorf("header is missing %q:\n%s", want, firstLines(v, 3))
		}
	}
}

func TestTabBarCountsEachTab(t *testing.T) {
	v := loaded().View()
	for _, want := range []string{"1 Library 3", "2 Watchlist 2", "3 Queue 1", "5 Profiles 2"} {
		if !strings.Contains(v, want) {
			t.Errorf("tab bar is missing %q:\n%s", want, firstLines(v, 4))
		}
	}
}

func TestLibraryMarksMissingFiles(t *testing.T) {
	v := loaded().View()
	if !strings.Contains(v, "missing") {
		t.Error("an item with no file should be marked missing")
	}
}

func TestNumberKeysSwitchTabs(t *testing.T) {
	b := press(loaded(), "2")
	if b.tab != tabWatchlist {
		t.Fatalf("tab = %v, want Watchlist", b.tab)
	}
	if v := b.View(); !strings.Contains(v, "Vivarium") {
		t.Error("the watchlist tab should list Vivarium")
	}
}

func TestQueueShowsProgressAndBar(t *testing.T) {
	v := press(loaded(), "3").View()
	if !strings.Contains(v, "42.0%") {
		t.Error("queue should show the percentage")
	}
	if !strings.Contains(v, "[=") {
		t.Error("queue should draw a progress bar")
	}
}

// A live queue has to refresh itself; the previous version only updated on a
// keypress, which made downloads look frozen.
func TestQueuePollingStartsWhenSomethingIsDownloading(t *testing.T) {
	if !loaded().polling {
		t.Error("polling should start while the queue is not empty")
	}
}

func TestPollingStopsWhenQueueEmpties(t *testing.T) {
	b := loaded()
	b.Update(refreshed{})
	b.Update(tickMsg{})
	if b.polling {
		t.Error("polling should stop once nothing is downloading")
	}
}

func TestFilterNarrowsTheList(t *testing.T) {
	b := press(loaded(), "/", "t", "a", "x")
	if got := len(b.rows()); got != 1 {
		t.Fatalf("%d rows match 'tax', want 1", got)
	}
	if b.rows()[0].Title != "Taxi Driver" {
		t.Errorf("filtered to %q", b.rows()[0].Title)
	}
	if v := b.View(); !strings.Contains(v, "filter") {
		t.Error("the filter should be visible while typing")
	}
}

func TestEscapeClearsTheFilter(t *testing.T) {
	b := press(loaded(), "/", "z", "z", "esc")
	if b.filter != "" {
		t.Errorf("filter = %q, want empty", b.filter)
	}
	if len(b.rows()) != 3 {
		t.Errorf("%d rows after clearing, want all 3", len(b.rows()))
	}
}

func TestCursorStaysInBounds(t *testing.T) {
	b := loaded()
	for i := 0; i < 10; i++ {
		b.Update(key("down"))
	}
	if b.cursor[tabLibrary] != 2 {
		t.Errorf("cursor = %d, want 2 for three rows", b.cursor[tabLibrary])
	}
	for i := 0; i < 10; i++ {
		b.Update(key("up"))
	}
	if b.cursor[tabLibrary] != 0 {
		t.Errorf("cursor = %d, want 0", b.cursor[tabLibrary])
	}
}

func TestEachTabKeepsItsOwnCursor(t *testing.T) {
	b := press(loaded(), "down", "2")
	if b.cursor[tabWatchlist] != 0 {
		t.Errorf("the watchlist cursor should start at 0, got %d", b.cursor[tabWatchlist])
	}
	press(b, "1")
	if b.cursor[tabLibrary] != 1 {
		t.Errorf("the library cursor should have been remembered, got %d", b.cursor[tabLibrary])
	}
}

func TestHelpScreen(t *testing.T) {
	b := press(loaded(), "?")
	v := b.View()
	if !strings.Contains(v, "delete: catalog, folder and torrent") {
		t.Error("help should explain what delete does")
	}
	press(b, "esc")
	if b.mode != modeList {
		t.Error("escape should leave the help screen")
	}
}

// Without an app there is nothing to plan against, and pressing delete must
// simply do nothing rather than crash.
func TestDeleteIsInertWithoutAnApp(t *testing.T) {
	b := press(loaded(), "d")
	if b.mode != modeList {
		t.Error("delete should not open a confirmation it cannot describe")
	}
}

func TestConfirmCancels(t *testing.T) {
	b := loaded()
	b.mode = modeConfirm
	b.confirm = &confirmState{
		item:  b.items[0],
		plan:  []string{"delete /Movies/Amélie (2001) (1.9 GB)", `remove "Amélie" from the catalog`},
		title: "Remove Amélie?",
	}
	v := b.View()
	if !strings.Contains(v, "delete /Movies/Amélie") || !strings.Contains(v, "from the catalog") {
		t.Errorf("the confirmation must enumerate what will be destroyed:\n%s", v)
	}
	press(b, "esc")
	if b.mode != modeList {
		t.Error("escape should cancel the deletion")
	}
}

// The reason a release was rejected is the whole point of the search screen.
func TestSearchViewShowsScoresAndRejections(t *testing.T) {
	b := loaded()
	b.Update(searchDone{
		item: b.items[0],
		cands: []decide.Candidate{
			{Release: indexer.Release{Title: "Amélie 1080p BluRay", Seeders: 272, Size: gb(1.94), Indexer: "Knaben"},
				Parsed: parse.Result{ScreenSize: "1080p"}, Score: 227},
			{Release: indexer.Release{Title: "Amelie 720p", Seeders: 6, Size: gb(0.73)},
				Parsed: parse.Result{ScreenSize: "720p"}, Score: 149,
				Rejects: []string{"resolution 720p not in 1080p"}},
		},
	})
	if b.mode != modeSearch {
		t.Fatal("a finished search should open the search view")
	}
	v := b.View()
	for _, want := range []string{"227", "REJECT", "resolution 720p not in 1080p", "272 seeders"} {
		if !strings.Contains(v, want) {
			t.Errorf("search view is missing %q:\n%s", want, v)
		}
	}
}

func TestSearchSortsAcceptedFirst(t *testing.T) {
	cands := []decide.Candidate{
		{Release: indexer.Release{Title: "rejected but high"}, Score: 900, Rejects: []string{"nope"}},
		{Release: indexer.Release{Title: "accepted"}, Score: 100},
	}
	s := newSearchState(nil, cands)
	if s.cands[0].Release.Title != "accepted" {
		t.Errorf("first candidate = %q, want the accepted one", s.cands[0].Release.Title)
	}
}

func TestDiscoverShowsWhySomethingIsSuggested(t *testing.T) {
	b := loaded()
	b.Update(discoverDone{suggestions: []recommend.Suggestion{
		{Movie: metadataMovie("Mean Streets", 1973, "en", 7.4), Seeds: []string{"Taxi Driver", "Altered States"}, Score: 27.4},
	}})
	v := press(b, "4").View()
	if !strings.Contains(v, "Mean Streets") {
		t.Error("discover should list the suggestion")
	}
	if !strings.Contains(v, "Taxi Driver") {
		t.Error("discover must show which films led to the suggestion")
	}
}

func TestDiscoverReportsMissingAPIKey(t *testing.T) {
	b := loaded()
	b.Update(discoverDone{err: errFake("recommendations need a TMDB API key")})
	if v := b.View(); !strings.Contains(v, "TMDB API key") {
		t.Errorf("the missing-key error should be visible:\n%s", v)
	}
}

func TestProfilesTabListsThem(t *testing.T) {
	v := press(loaded(), "5").View()
	for _, want := range []string{"Leandro", "leandroacosta", "prefer:es"} {
		if !strings.Contains(v, want) {
			t.Errorf("profiles tab is missing %q:\n%s", want, v)
		}
	}
}

func TestErrorsAreVisible(t *testing.T) {
	b := loaded()
	b.Update(actionDone{err: errFake("prowlarr unreachable")})
	if !strings.Contains(b.View(), "prowlarr unreachable") {
		t.Error("an action error should be shown")
	}
}

type errFake string

func (e errFake) Error() string { return string(e) }

func firstLines(s string, n int) string {
	parts := strings.SplitN(s, "\n", n+1)
	if len(parts) > n {
		parts = parts[:n]
	}
	return strings.Join(parts, "\n")
}
