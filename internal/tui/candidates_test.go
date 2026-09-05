package tui

import (
	"strings"
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/indexer"
	"github.com/LeandroMAcosta/butaca/internal/parse"
)

func cand(title, indexerName string, seeders int, score int, rejects ...string) decide.Candidate {
	return decide.Candidate{
		Release: indexer.Release{Title: title, Indexer: indexerName, Seeders: seeders, Size: gb(2)},
		Parsed:  parse.Result{ScreenSize: "1080p"},
		Score:   score,
		Rejects: rejects,
	}
}

// Prowlarr fans one query across every tracker, so the same release comes back
// several times. Showing each copy is noise.
func TestDedupeKeepsTheBestSeededCopy(t *testing.T) {
	l := newCandidateList([]decide.Candidate{
		cand("Dune Part Two (2024) [1080p] [BluRay]", "Knaben", 604, 281),
		cand("Dune Part Two (2024) [1080p] [BluRay]", "The Pirate Bay", 109, 251),
	})
	if len(l.all) != 1 {
		t.Fatalf("%d rows after dedupe, want 1", len(l.all))
	}
	if l.all[0].Release.Seeders != 604 {
		t.Errorf("kept the %d-seeder copy, want the 604-seeder one", l.all[0].Release.Seeders)
	}
	if got := l.mirrors[releaseKey("Dune Part Two (2024) [1080p] [BluRay]")]; got != 2 {
		t.Errorf("mirror count = %d, want 2", got)
	}
}

func TestReleaseKeyIgnoresPunctuation(t *testing.T) {
	if releaseKey("Dune.Part.Two.2024") != releaseKey("Dune Part Two (2024)") {
		t.Error("punctuation should not defeat deduplication")
	}
	if releaseKey("Dune Part One") == releaseKey("Dune Part Two") {
		t.Error("different releases must not collapse together")
	}
}

// The default view answers "what will you grab", not "what did every tracker say".
func TestRejectedAreHiddenByDefault(t *testing.T) {
	l := newCandidateList([]decide.Candidate{
		cand("good", "YTS", 500, 250),
		cand("cam", "TPB", 10, 90, "matches reject pattern CAM"),
	})
	if len(l.visible()) != 1 {
		t.Errorf("%d visible rows, want only the accepted one", len(l.visible()))
	}
	if l.rejected != 1 {
		t.Errorf("rejected count = %d, want 1", l.rejected)
	}
	l.toggleRejected()
	if len(l.visible()) != 2 {
		t.Errorf("%d visible after toggling, want both", len(l.visible()))
	}
}

func TestToggleResetsTheCursor(t *testing.T) {
	l := newCandidateList([]decide.Candidate{
		cand("a", "x", 5, 10), cand("b", "x", 5, 9), cand("c", "x", 5, 8),
	})
	l.move(2)
	l.toggleRejected()
	if l.cursor != 0 {
		t.Errorf("cursor = %d after toggling, want 0", l.cursor)
	}
}

func TestMoveStaysInBounds(t *testing.T) {
	l := newCandidateList([]decide.Candidate{cand("a", "x", 5, 10), cand("b", "x", 5, 9)})
	l.move(50)
	if l.cursor != 1 {
		t.Errorf("cursor = %d, want 1", l.cursor)
	}
	l.move(-50)
	if l.cursor != 0 {
		t.Errorf("cursor = %d, want 0", l.cursor)
	}
}

// One line per release is the whole point; a rejected one earns a second line
// for its reason.
func TestRenderIsOneLinePerAcceptedRelease(t *testing.T) {
	l := newCandidateList([]decide.Candidate{
		cand("alpha", "YTS", 500, 250), cand("beta", "Knaben", 300, 240), cand("gamma", "TPB", 100, 230),
	})
	body := l.render(120, 20)
	rows := 0
	for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		if strings.Contains(line, "alpha") || strings.Contains(line, "beta") || strings.Contains(line, "gamma") {
			rows++
		}
	}
	if rows != 3 {
		t.Errorf("%d release lines, want 3 (one each)", rows)
	}
}

func TestRenderCapsRowsAndSaysHowManyRemain(t *testing.T) {
	var many []decide.Candidate
	for i := 0; i < 40; i++ {
		many = append(many, cand("release "+string(rune('a'+i%26))+string(rune('0'+i/26)), "x", 100+i, 200+i))
	}
	body := newCandidateList(many).render(120, 6)
	if !strings.Contains(body, "and") || !strings.Contains(body, "more") {
		t.Errorf("a capped list should say how many were hidden:\n%s", body)
	}
}

func TestSummaryReportsMergedDuplicates(t *testing.T) {
	l := newCandidateList([]decide.Candidate{
		cand("same release", "Knaben", 600, 280),
		cand("same release", "TPB", 100, 250),
		cand("other", "YTS", 90, 240),
	})
	got := l.summary("Dune")
	for _, want := range []string{"3 releases", "2 after merging duplicates", "2 pass your rules"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary %q is missing %q", got, want)
		}
	}
}

func TestPickWarnsWhenForcingARejectedRelease(t *testing.T) {
	l := newCandidateList([]decide.Candidate{cand("cam", "TPB", 10, 90, "matches reject pattern CAM")})
	l.toggleRejected()
	if got := l.pick(); !strings.Contains(got, "breaks your rules") {
		t.Errorf("pick = %q, want a warning", got)
	}
}

func TestEmptyAcceptedListPointsAtTheToggle(t *testing.T) {
	l := newCandidateList([]decide.Candidate{cand("cam", "TPB", 10, 90, "CAM")})
	if got := l.render(120, 10); !strings.Contains(got, "nothing passed your rules") {
		t.Errorf("render = %q", got)
	}
}
