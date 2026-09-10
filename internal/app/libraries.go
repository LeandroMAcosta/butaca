package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// A movie lives in one of two libraries. Which one is not stored separately:
// it is read from where the item's folder is, so the catalog and the disk
// cannot disagree about it.
const (
	LibraryMovies        = "movies"
	LibraryDocumentaries = "documentaries"
)

// tmdbDocumentary is TMDB's genre id for documentaries.
const tmdbDocumentary = 99

// LibraryRoot returns the folder a movie library lives in.
func (a *App) LibraryRoot(name string) (string, error) {
	switch name {
	case LibraryMovies:
		return a.Cfg.Paths.Movies, nil
	case LibraryDocumentaries:
		if a.Cfg.Paths.Documentaries == "" {
			return "", errors.New(
				"paths.documentaries is not set: butaca config set paths.documentaries <dir>")
		}
		return a.Cfg.Paths.Documentaries, nil
	}
	return "", fmt.Errorf("unknown library %q: use movies or documentaries", name)
}

// LibraryOf reports which library a movie belongs to.
func (a *App) LibraryOf(it *store.Item) string {
	if within(it.Path, a.Cfg.Paths.Documentaries) {
		return LibraryDocumentaries
	}
	return LibraryMovies
}

func within(path, root string) bool {
	if path == "" || root == "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

// pickLibrary settles where a new movie goes. An explicit choice wins; without
// one, TMDB's Documentary genre decides, but only once a documentaries library
// exists, so configuring nothing keeps the old behaviour.
func (a *App) pickLibrary(documentary *bool, genres []int) string {
	if documentary != nil {
		if *documentary {
			return LibraryDocumentaries
		}
		return LibraryMovies
	}
	if a.Cfg.Paths.Documentaries != "" {
		for _, g := range genres {
			if g == tmdbDocumentary {
				return LibraryDocumentaries
			}
		}
	}
	return LibraryMovies
}

// movieFolder is where a new movie's files will go.
func (a *App) movieFolder(lib, title string, year int) (string, error) {
	root, err := a.LibraryRoot(lib)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, library.MovieFolder(title, year)), nil
}

// MovePlan returns the folder an item would move from and to.
func (a *App) MovePlan(it *store.Item, to string) (from, dest string, err error) {
	if it.Kind != "movie" {
		return "", "", fmt.Errorf("%s is a series; only movies move between libraries", it.Title)
	}
	if a.LibraryOf(it) == to {
		return "", "", fmt.Errorf("%s is already in %s", it.Title, to)
	}
	root, err := a.LibraryRoot(to)
	if err != nil {
		return "", "", err
	}
	from = it.Path
	name := filepath.Base(from)
	if from == "" {
		name = library.MovieFolder(it.Title, it.Year)
	}
	return from, filepath.Join(root, name), nil
}

// Move puts a movie in the other library: it renames the folder, which keeps
// every hardlink and subtitle intact, then rewrites the catalog's paths. If
// the catalog cannot be updated the folder is moved back.
func (a *App) Move(it *store.Item, to string) (string, error) {
	from, dest, err := a.MovePlan(it, to)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%s already exists", dest)
	}

	moved := false
	if from != "" {
		if _, err := os.Stat(from); err == nil {
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return "", err
			}
			if err := os.Rename(from, dest); err != nil {
				if errors.Is(err, syscall.EXDEV) {
					return "", fmt.Errorf("%s and %s are on different filesystems; a move would break the hardlinks",
						filepath.Dir(from), filepath.Dir(dest))
				}
				return "", err
			}
			moved = true
		}
	}

	if err := a.Store.MoveItemPath(it.ID, from, dest); err != nil {
		if moved {
			if rerr := os.Rename(dest, from); rerr != nil {
				return "", fmt.Errorf("update catalog: %w; and moving the folder back failed: %v", err, rerr)
			}
		}
		return "", fmt.Errorf("update catalog: %w", err)
	}
	_ = a.Store.Log(it.ID, "moved", fmt.Sprintf("%s -> %s", from, dest))
	return dest, nil
}
