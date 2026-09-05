package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/indexer"
	"github.com/LeandroMAcosta/butaca/internal/parse"
)

func typeQuery(b *browser, q string) *browser {
	for _, r := range q {
		if r == ' ' {
			b.Update(tea.KeyMsg{Type: tea.KeySpace})
			continue
		}
		b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return b
}

func TestAddOpensAPrompt(t *testing.T) {
	b := press(loaded(), "a")
	if b.mode != modeAdd {
		t.Fatalf("mode = %v, want add", b.mode)
	}
	v := b.View()
	if !strings.Contains(v, "Add a film") || !strings.Contains(v, "title:") {
		t.Errorf("the add screen should prompt for a title:\n%s", v)
	}
	// The point of this flow is that it works before TMDB is configured.
	if !strings.Contains(v, "No TMDB key needed") {
		t.Error("the prompt should say a TMDB key is not required")
	}
}

func TestAddCollectsTypedTitle(t *testing.T) {
	b := typeQuery(press(loaded(), "a"), "Blade Runner 2049")
	if b.add.query != "Blade Runner 2049" {
		t.Errorf("query = %q", b.add.query)
	}
	if !strings.Contains(b.View(), "Blade Runner 2049") {
		t.Error("the typed title should be visible")
	}
}

func TestAddBackspaceEditsTheQuery(t *testing.T) {
	b := typeQuery(press(loaded(), "a"), "Amelie")
	b.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if b.add.query != "Ameli" {
		t.Errorf("query = %q, want Ameli", b.add.query)
	}
}

// Typing must not be intercepted by the list shortcuts: "d" is delete on the
// list, but a letter here.
func TestAddSwallowsListShortcuts(t *testing.T) {
	b := typeQuery(press(loaded(), "a"), "dune")
	if b.add.query != "dune" {
		t.Errorf("query = %q, want dune", b.add.query)
	}
	if b.mode != modeAdd {
		t.Error("typing d must not trigger the delete confirmation")
	}
}

func TestAddEscapeCancels(t *testing.T) {
	b := press(loaded(), "a")
	b.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if b.mode != modeList || b.add != nil {
		t.Error("escape should abandon the add flow")
	}
}

func addResults() []decide.Candidate {
	return []decide.Candidate{
		{Release: indexer.Release{Title: "Dune.Part.Two.2024.1080p.BluRay", Seeders: 900, Size: gb(3.2), Indexer: "YTS"},
			Parsed: parse.Result{Title: "Dune Part Two", Year: 2024, ScreenSize: "1080p"}, Score: 240},
		{Release: indexer.Release{Title: "Dune.Part.Two.2024.CAM", Seeders: 12, Size: gb(1)},
			Parsed: parse.Result{Title: "Dune Part Two", Year: 2024}, Score: 90,
			Rejects: []string{"matches reject pattern CAM"}},
	}
}

func TestAddShowsScoredReleasesWithReasons(t *testing.T) {
	b := typeQuery(press(loaded(), "a"), "Dune Part Two")
	b.Update(addSearched{query: "Dune Part Two", cands: addResults()})

	v := b.View()
	// Accepted releases only, one line each: the default view has to be
	// readable, not a dump of every tracker's answer.
	for _, want := range []string{"2 releases", "1 pass your rules", "240", "900"} {
		if !strings.Contains(v, want) {
			t.Errorf("add results are missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "matches reject pattern CAM") {
		t.Error("rejected releases should be hidden until asked for")
	}
	if !strings.Contains(v, "show 1 rejected") {
		t.Error("the footer should offer to show the rejected ones")
	}

	// x reveals them, with the reason.
	b.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if v := b.View(); !strings.Contains(v, "matches reject pattern CAM") {
		t.Errorf("x should reveal why a release was rejected:\n%s", v)
	}
}

func TestAddPutsAcceptedReleasesFirst(t *testing.T) {
	b := press(loaded(), "a")
	b.Update(addSearched{query: "x", cands: []decide.Candidate{
		{Release: indexer.Release{Title: "rejected"}, Score: 999, Rejects: []string{"no"}},
		{Release: indexer.Release{Title: "accepted"}, Score: 10},
	}})
	if b.add.list.all[0].Release.Title != "accepted" {
		t.Errorf("first = %q, want the accepted release", b.add.list.all[0].Release.Title)
	}
}

func TestAddBackspaceReturnsToTheQuery(t *testing.T) {
	b := typeQuery(press(loaded(), "a"), "Dune")
	b.Update(addSearched{query: "Dune", cands: addResults()})
	b.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if b.add.searched {
		t.Error("backspace should return to editing the title")
	}
	if b.add.query != "Dune" {
		t.Errorf("the query should be preserved, got %q", b.add.query)
	}
}

func TestAddReportsAnEmptyResult(t *testing.T) {
	b := typeQuery(press(loaded(), "a"), "zzzz")
	b.Update(addSearched{query: "zzzz", cands: nil})
	if v := b.View(); !strings.Contains(v, "nothing found") {
		t.Errorf("an empty search should say so:\n%s", v)
	}
}

func TestAddSurfacesSearchErrors(t *testing.T) {
	b := press(loaded(), "a")
	b.Update(addSearched{query: "x", err: errFake("prowlarr unreachable")})
	if !strings.Contains(b.View(), "prowlarr unreachable") {
		t.Error("a failed search should be reported")
	}
}
