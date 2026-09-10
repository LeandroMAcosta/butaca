package app

import (
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/indexer"
)

func TestReleaseIDPrefersTheInfohash(t *testing.T) {
	for _, tc := range []struct {
		r    indexer.Release
		want string
	}{
		{indexer.Release{InfoHash: "ABCDEF", GUID: "g", DownloadURL: "u"}, "abcdef"},
		{indexer.Release{GUID: "g", DownloadURL: "u"}, "g"},
		{indexer.Release{DownloadURL: "u"}, "u"},
	} {
		if got := ReleaseID(tc.r); got != tc.want {
			t.Errorf("ReleaseID(%+v) = %q, want %q", tc.r, got, tc.want)
		}
	}
}

func TestDedupeKeepsTheBestSeededMirror(t *testing.T) {
	cands := []decide.Candidate{
		{Release: indexer.Release{Title: "Amelie.2001.1080p", Seeders: 5, GUID: "a"}},
		{Release: indexer.Release{Title: "Amelie 2001 1080p", Seeders: 40, GUID: "b"}},
		{Release: indexer.Release{Title: "Other.2001", Seeders: 1, GUID: "c"}},
	}
	out, mirrors := DedupeReleases(cands)
	if len(out) != 2 {
		t.Fatalf("got %d releases, want 2", len(out))
	}
	if out[0].Release.GUID != "b" {
		t.Errorf("kept %q, want the 40-seeder copy", out[0].Release.GUID)
	}
	if n := mirrors[ReleaseKey("Amelie.2001.1080p")]; n != 2 {
		t.Errorf("mirrors = %d, want 2", n)
	}
}

func TestSortByScorePutsAcceptedFirst(t *testing.T) {
	cands := []decide.Candidate{
		{Score: 500, Rejects: []string{"too big"}},
		{Score: 100},
		{Score: 300},
	}
	SortByScore(cands)
	if cands[0].Score != 300 || cands[1].Score != 100 || cands[2].Score != 500 {
		t.Errorf("order = %d, %d, %d; want 300, 100, 500",
			cands[0].Score, cands[1].Score, cands[2].Score)
	}
}
