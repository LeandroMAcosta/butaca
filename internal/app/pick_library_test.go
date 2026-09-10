package app

import (
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/config"
)

func TestPickLibraryFollowsTMDBOnlyWithADocumentariesPath(t *testing.T) {
	cfg := config.Default()
	a := &App{Cfg: cfg}
	docGenres := []int{18, tmdbDocumentary}
	no := false

	if got := a.pickLibrary(nil, docGenres); got != LibraryMovies {
		t.Errorf("without paths.documentaries got %s, want movies", got)
	}
	cfg.Paths.Documentaries = "/media/documentaries"
	if got := a.pickLibrary(nil, docGenres); got != LibraryDocumentaries {
		t.Errorf("TMDB genre 99 got %s, want documentaries", got)
	}
	if got := a.pickLibrary(&no, docGenres); got != LibraryMovies {
		t.Errorf("explicit false got %s, want movies", got)
	}
	if got := a.pickLibrary(nil, []int{18}); got != LibraryMovies {
		t.Errorf("a drama got %s, want movies", got)
	}
}
