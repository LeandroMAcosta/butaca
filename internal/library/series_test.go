package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEpisodeDestinationLayout(t *testing.T) {
	got := EpisodeDestination("/media/tv", "Better Call Saul", 2015, 1, 2, "Mijo", ".mkv")
	want := filepath.Join("/media/tv", "Better Call Saul (2015)", "Season 01",
		"Better Call Saul (2015) - S01E02 - Mijo.mkv")
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestEpisodeDestinationWithoutTitle(t *testing.T) {
	got := EpisodeDestination("/media/tv", "Severance", 2022, 2, 10, "", ".MKV")
	want := filepath.Join("/media/tv", "Severance (2022)", "Season 02", "Severance (2022) - S02E10.mkv")
	if got != want {
		t.Errorf("got  %s\nwant %s", got, want)
	}
}

func TestEpisodeDestinationSanitizesTitle(t *testing.T) {
	got := EpisodeDestination("/media/tv", "The Wire", 2002, 1, 1, "The Target: Part 1/2", ".mkv")
	if filepath.Base(filepath.Dir(got)) != "Season 01" {
		t.Fatalf("season folder wrong: %s", got)
	}
	base := filepath.Base(got)
	for _, bad := range []string{"/", ":"} {
		if containsRune(base, bad) {
			t.Errorf("filename %q still contains %q", base, bad)
		}
	}
}

func containsRune(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestImportEpisodeHardlinks(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "downloads", "Show.S01E03.1080p", "ep.mkv")
	write(t, src, 2048)

	res, err := ImportEpisode(src, filepath.Join(base, "tv"), "Show", 2020, 1, 3, "Pilot")
	if err != nil {
		t.Fatal(err)
	}
	n, err := LinkCount(res.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("link count = %d, want 2", n)
	}
	if _, err := os.Stat(res.Destination); err != nil {
		t.Fatalf("episode not on disk: %v", err)
	}
}
