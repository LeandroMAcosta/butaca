package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/store"
)

func loaded() browser {
	b := browser{height: 20}
	m, _ := b.Update(refreshed{
		items: []*store.Item{
			{ID: 1, Title: "Amélie", Year: 2001, OriginalLanguage: "fr", FileCount: 1, SizeBytes: 2 << 30},
			{ID: 2, Title: "Exit 8", Year: 2025, OriginalLanguage: "ja"},
		},
		queue: []*store.QueueEntry{
			{ReleaseTitle: "Exit 8 2025 1080p", State: "downloading", Progress: 0.42},
		},
	})
	return m.(browser)
}

func TestLibraryShowsItemsAndMissingState(t *testing.T) {
	v := loaded().View()
	for _, want := range []string{"Amélie", "Exit 8", "missing", "2 in the library"} {
		if !strings.Contains(v, want) {
			t.Errorf("view is missing %q:\n%s", want, v)
		}
	}
}

func TestTabSwitchesToQueue(t *testing.T) {
	m, _ := loaded().Update(tea.KeyMsg{Type: tea.KeyTab})
	v := m.(browser).View()
	if !strings.Contains(v, "Exit 8 2025 1080p") || !strings.Contains(v, "42.0%") {
		t.Errorf("queue tab should show the download and its progress:\n%s", v)
	}
}

func TestCursorStaysInBounds(t *testing.T) {
	b := loaded()
	for i := 0; i < 5; i++ {
		m, _ := b.Update(tea.KeyMsg{Type: tea.KeyDown})
		b = m.(browser)
	}
	if b.cursor != 1 {
		t.Errorf("cursor = %d, want 1 (two items)", b.cursor)
	}
	for i := 0; i < 5; i++ {
		m, _ := b.Update(tea.KeyMsg{Type: tea.KeyUp})
		b = m.(browser)
	}
	if b.cursor != 0 {
		t.Errorf("cursor = %d, want 0", b.cursor)
	}
}

// Switching tabs must not leave the cursor pointing past the shorter list.
func TestTabResetsCursor(t *testing.T) {
	b := loaded()
	m, _ := b.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.(browser).Update(tea.KeyMsg{Type: tea.KeyTab})
	if got := m.(browser).cursor; got != 0 {
		t.Errorf("cursor = %d after switching tabs, want 0", got)
	}
}

func TestErrorIsShown(t *testing.T) {
	b := loaded()
	m, _ := b.Update(actionDone{err: errFake{}})
	if !strings.Contains(m.(browser).View(), "prowlarr unreachable") {
		t.Error("an action error should be visible in the view")
	}
}

type errFake struct{}

func (errFake) Error() string { return "prowlarr unreachable" }
