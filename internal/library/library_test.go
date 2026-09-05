package library

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindVideoPicksLargestAndSkipsSamples(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Sample", "sample.mkv"), 500)
	write(t, filepath.Join(root, "movie.mkv"), 5000)
	write(t, filepath.Join(root, "readme.txt"), 10)
	write(t, filepath.Join(root, "featurette.mkv"), 9000)

	got, err := FindVideo(root)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got) != "movie.mkv" {
		t.Fatalf("got %s, want movie.mkv", filepath.Base(got))
	}
}

func TestFindVideoNoneReturnsError(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "notes.txt"), 10)
	if _, err := FindVideo(root); err == nil {
		t.Fatal("expected an error when there is no video")
	}
}

func TestImportMovieHardlinks(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "downloads", "Some.Release.1976.1080p", "video.mkv")
	write(t, src, 1234)
	dest := filepath.Join(base, "movies")

	res, err := ImportMovie(src, dest, "Taxi Driver", 1976)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dest, "Taxi Driver (1976)", "Taxi Driver (1976).mkv")
	if res.Destination != want {
		t.Fatalf("destination = %s, want %s", res.Destination, want)
	}

	// The whole point: one set of bytes, two names.
	n, err := LinkCount(res.Destination)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("link count = %d, want 2 (a copy would be 1)", n)
	}

	srcInfo, _ := os.Stat(src)
	dstInfo, _ := os.Stat(res.Destination)
	if !os.SameFile(srcInfo, dstInfo) {
		t.Fatal("source and destination are not the same file")
	}
}

// Re-importing the same download must be a no-op, not an error.
func TestImportMovieIsIdempotent(t *testing.T) {
	base := t.TempDir()
	src := filepath.Join(base, "downloads", "video.mkv")
	write(t, src, 10)
	dest := filepath.Join(base, "movies")

	first, err := ImportMovie(src, dest, "Amélie", 2001)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ImportMovie(src, dest, "Amélie", 2001)
	if err != nil {
		t.Fatalf("second import should succeed: %v", err)
	}
	if first.Destination != second.Destination {
		t.Fatal("second import chose a different destination")
	}
}

func TestMovieFolderSanitizesPathCharacters(t *testing.T) {
	for _, tc := range []struct{ title, want string }{
		{"Amélie", "Amélie (2001)"},
		{"Face/Off", "Face-Off (2001)"},
		{"Pink Floyd: The Wall", "Pink Floyd - The Wall (2001)"},
		{"Christiane F.", "Christiane F (2001)"},
	} {
		if got := MovieFolder(tc.title, 2001); got != tc.want {
			t.Errorf("MovieFolder(%q) = %q, want %q", tc.title, got, tc.want)
		}
	}
}

func TestSameFilesystemWorksBeforeFoldersExist(t *testing.T) {
	base := t.TempDir()
	ok, err := SameFilesystem(filepath.Join(base, "not", "created", "yet"), base)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("paths under the same temp dir must share a filesystem")
	}
}
