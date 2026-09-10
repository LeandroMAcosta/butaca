package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/config"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func newLibraryApp(t *testing.T, documentaries bool) *app.App {
	t.Helper()
	root := t.TempDir()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Paths.Movies = filepath.Join(root, "movies")
	if documentaries {
		cfg.Paths.Documentaries = filepath.Join(root, "documentaries")
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

func TestMoveRenamesTheFolderAndTheCatalog(t *testing.T) {
	a := newLibraryApp(t, true)
	folder := filepath.Join(a.Cfg.Paths.Movies, "The Thinking Game (2024)")
	video := filepath.Join(folder, "The Thinking Game (2024).mkv")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(video, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, _ := a.Store.AddItem(&store.Item{Kind: "movie", Title: "The Thinking Game", Year: 2024, Path: folder, Monitored: true})
	if _, err := a.Store.AddFile(&store.File{ItemID: id, Path: video}); err != nil {
		t.Fatal(err)
	}
	it, _ := a.Store.GetItem(id)
	if got := a.LibraryOf(it); got != app.LibraryMovies {
		t.Fatalf("starts in %s, want movies", got)
	}

	dest, err := a.Move(it, app.LibraryDocumentaries)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(a.Cfg.Paths.Documentaries, "The Thinking Game (2024)")
	if dest != want {
		t.Errorf("moved to %s, want %s", dest, want)
	}
	if _, err := os.Stat(filepath.Join(want, "The Thinking Game (2024).mkv")); err != nil {
		t.Errorf("video did not move: %v", err)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Errorf("old folder still there: %v", err)
	}
	it, _ = a.Store.GetItem(id)
	if got := a.LibraryOf(it); got != app.LibraryDocumentaries {
		t.Errorf("catalog says %s, want documentaries", got)
	}

	if _, err := a.Move(it, app.LibraryDocumentaries); err == nil {
		t.Error("moving into the library it is already in should fail")
	}
	if _, err := a.Move(it, app.LibraryMovies); err != nil {
		t.Errorf("moving back: %v", err)
	}
}

func TestMoveNeedsADocumentariesLibrary(t *testing.T) {
	a := newLibraryApp(t, false)
	it := &store.Item{Kind: "movie", Title: "X", Path: filepath.Join(a.Cfg.Paths.Movies, "X")}
	if _, _, err := a.MovePlan(it, app.LibraryDocumentaries); err == nil {
		t.Error("expected an error when paths.documentaries is unset")
	}
	if _, _, err := a.MovePlan(it, "shorts"); err == nil {
		t.Error("expected an error for an unknown library")
	}
}

func TestAddFilesAnExplicitDocumentary(t *testing.T) {
	a := newLibraryApp(t, true)
	yes, no := true, false
	for _, tc := range []struct {
		flag *bool
		want string
	}{
		{&yes, app.LibraryDocumentaries},
		{&no, app.LibraryMovies},
		// No TMDB key, so nothing can infer the genre.
		{nil, app.LibraryMovies},
	} {
		it, err := a.AddMovie(t.Context(), "Film "+tc.want+boolName(tc.flag), app.AddOptions{
			OriginalLanguage: "en", Documentary: tc.flag,
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := a.LibraryOf(it); got != tc.want {
			t.Errorf("documentary=%s filed under %s, want %s", boolName(tc.flag), got, tc.want)
		}
	}
}

func boolName(b *bool) string {
	switch {
	case b == nil:
		return "unset"
	case *b:
		return "true"
	}
	return "false"
}
