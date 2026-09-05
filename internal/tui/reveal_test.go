package tui

import (
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/store"
)

func TestRevealCommandSelectsTheFile(t *testing.T) {
	name, args, err := revealCommand("/Users/x/Movies/Film (1982)/film.mkv", true)
	if err != nil {
		t.Fatal(err)
	}
	switch runtime.GOOS {
	case "darwin":
		if name != "open" || len(args) != 2 || args[0] != "-R" {
			t.Errorf("got %s %v, want open -R <file>", name, args)
		}
	case "linux":
		// xdg-open cannot select a file, so it gets the containing folder.
		if name != "xdg-open" || args[0] != "/Users/x/Movies/Film (1982)" {
			t.Errorf("got %s %v, want the parent folder", name, args)
		}
	}
}

func TestRevealCommandOpensAFolderDirectly(t *testing.T) {
	_, args, err := revealCommand("/Users/x/Movies/Film (1982)", false)
	if err != nil {
		t.Fatal(err)
	}
	if args[len(args)-1] != "/Users/x/Movies/Film (1982)" {
		t.Errorf("args = %v, want the folder itself", args)
	}
}

// The video is a better target than its folder: it is what you actually want
// to open, and the folder also holds subtitle files.
func TestRevealTargetPrefersTheMediaFile(t *testing.T) {
	d := &detailState{
		item:  &store.Item{Path: "/Movies/The Wall (1982)"},
		files: []*store.File{{Path: "/Movies/The Wall (1982)/wall.mkv"}},
	}
	path, isFile := d.revealTarget()
	if path != "/Movies/The Wall (1982)/wall.mkv" || !isFile {
		t.Errorf("target = %q (isFile=%v)", path, isFile)
	}
}

func TestRevealTargetFallsBackToTheFolder(t *testing.T) {
	d := &detailState{item: &store.Item{Path: "/Movies/Missing (2026)"}}
	path, isFile := d.revealTarget()
	if path != "/Movies/Missing (2026)" || isFile {
		t.Errorf("target = %q (isFile=%v), want the folder", path, isFile)
	}
}

func TestRevealReportsAnItemWithNoPath(t *testing.T) {
	msg := reveal("", false)()
	done, ok := msg.(actionDone)
	if !ok || done.err == nil {
		t.Fatalf("expected an error message, got %#v", msg)
	}
}

// Enter in the detail view must open the file, not dismiss the view.
func TestEnterInDetailRevealsInsteadOfClosing(t *testing.T) {
	b := loaded()
	b.Update(detailDone{
		item:  b.items[0],
		files: []*store.File{{Path: "/Movies/Amélie (2001)/amelie.mkv"}},
	})
	if b.mode != modeDetail {
		t.Fatal("detail view did not open")
	}
	b.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if b.mode != modeDetail {
		t.Error("enter should keep the detail view open and reveal the file")
	}
}

func TestEscapeStillLeavesTheDetailView(t *testing.T) {
	b := loaded()
	b.Update(detailDone{item: b.items[0]})
	b.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if b.mode != modeList {
		t.Error("escape should close the detail view")
	}
}

func TestDetailFooterAdvertisesTheAction(t *testing.T) {
	b := loaded()
	b.Update(detailDone{item: b.items[0], files: []*store.File{{Path: "/x/f.mkv"}}})
	if v := b.View(); !strings.Contains(v, "open in "+fileManager()) {
		t.Errorf("the detail footer should mention opening in %s:\n%s", fileManager(), v)
	}
}
