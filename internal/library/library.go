// Package library moves finished downloads into the media tree.
//
// Import uses a hardlink, never a copy: the file keeps seeding under its
// download name while appearing in the library under a clean one, at no extra
// disk cost. The consequence is that deleting one name frees nothing -- both
// must go, which is why Remove tears down every reference.
package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

// videoExts are the containers worth importing.
var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".m4v": true,
	".mov": true, ".wmv": true, ".mpg": true, ".mpeg": true, ".ts": true,
}

// sampleMarkers identify throwaway files shipped inside releases.
var sampleMarkers = []string{"sample", "trailer", "extras", "featurette"}

var ErrNoVideo = errors.New("no video file found")

// FindVideo returns the largest genuine video file under root. Releases ship
// samples and extras; size is the reliable discriminator.
func FindVideo(root string) (string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		if videoExts[strings.ToLower(filepath.Ext(root))] {
			return root, nil
		}
		return "", fmt.Errorf("%w at %s", ErrNoVideo, root)
	}

	type cand struct {
		path string
		size int64
	}
	var found []cand
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable subtree should not abort the scan
		}
		if !videoExts[strings.ToLower(filepath.Ext(p))] {
			return nil
		}
		lower := strings.ToLower(filepath.Base(p))
		for _, m := range sampleMarkers {
			if strings.Contains(lower, m) {
				return nil
			}
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		found = append(found, cand{p, fi.Size()})
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(found) == 0 {
		return "", fmt.Errorf("%w under %s", ErrNoVideo, root)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].size > found[j].size })
	return found[0].path, nil
}

// MovieFolder is the library folder name for a film: "Title (Year)", with
// characters that are illegal or awkward on disk replaced.
func MovieFolder(title string, year int) string {
	name := sanitize(title)
	if year > 0 {
		name = fmt.Sprintf("%s (%d)", name, year)
	}
	return name
}

func sanitize(s string) string {
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", " -", "*", "", "?", "",
		"\"", "'", "<", "", ">", "", "|", "", "\x00", "",
	)
	out := strings.TrimSpace(replacer.Replace(s))
	// A trailing dot makes a directory awkward to handle on some systems.
	return strings.TrimRight(out, ". ")
}

type ImportResult struct {
	Source      string
	Destination string
	Size        int64
	Hardlinked  bool
}

// ImportMovie links src into destDir as "<folder>/<folder><ext>", creating the
// folder. It refuses to fall back to copying: a silent copy would double disk
// usage, which is exactly the failure this design avoids.
func ImportMovie(src, destRoot, title string, year int) (*ImportResult, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	folder := MovieFolder(title, year)
	destDir := filepath.Join(destRoot, folder)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(destDir, folder+strings.ToLower(filepath.Ext(src)))

	if existing, err := os.Stat(dest); err == nil {
		if os.SameFile(fi, existing) {
			return &ImportResult{Source: src, Destination: dest, Size: fi.Size(), Hardlinked: true}, nil
		}
		return nil, fmt.Errorf("destination already exists: %s", dest)
	}

	if err := os.Link(src, dest); err != nil {
		if isCrossDevice(err) {
			return nil, fmt.Errorf(
				"cannot hardlink %s -> %s: they are on different filesystems. "+
					"Point paths.downloads and paths.movies at the same volume", src, dest)
		}
		return nil, fmt.Errorf("link %s -> %s: %w", src, dest, err)
	}
	return &ImportResult{Source: src, Destination: dest, Size: fi.Size(), Hardlinked: true}, nil
}

func isCrossDevice(err error) bool {
	var le *os.LinkError
	if errors.As(err, &le) {
		return errors.Is(le.Err, syscall.EXDEV)
	}
	return errors.Is(err, syscall.EXDEV)
}

// SameFilesystem reports whether two paths can be hardlinked between. It walks
// up to the nearest existing ancestor so it works before the folders are made.
func SameFilesystem(a, b string) (bool, error) {
	da, err := deviceOf(a)
	if err != nil {
		return false, err
	}
	db, err := deviceOf(b)
	if err != nil {
		return false, err
	}
	return da == db, nil
}

func deviceOf(path string) (uint64, error) {
	for {
		fi, err := os.Stat(path)
		if err == nil {
			st, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				return 0, fmt.Errorf("cannot determine device for %s", path)
			}
			return uint64(st.Dev), nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return 0, fmt.Errorf("no existing ancestor for %s", path)
		}
		path = parent
	}
}

// LinkCount reports how many names point at this file. A freshly imported file
// has 2: the download and the library entry.
func LinkCount(path string) (uint64, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, fmt.Errorf("cannot read link count for %s", path)
	}
	return uint64(st.Nlink), nil
}

// RemoveFolder deletes a library folder and everything in it, including
// subtitles that sit beside the video.
func RemoveFolder(dir string) error {
	if dir == "" || dir == "/" {
		return fmt.Errorf("refusing to remove %q", dir)
	}
	return os.RemoveAll(dir)
}

// EpisodeDestination builds the conventional layout for a series file:
//
//	Series (Year)/Season 01/Series (Year) - S01E02 - Episode Title.mkv
//
// Every media player and scraper understands this shape.
func EpisodeDestination(destRoot, series string, year, season, episode int, epTitle, ext string) string {
	folder := MovieFolder(series, year)
	name := fmt.Sprintf("%s - S%02dE%02d", folder, season, episode)
	if t := sanitize(epTitle); t != "" {
		name += " - " + t
	}
	return filepath.Join(destRoot, folder, fmt.Sprintf("Season %02d", season), name+strings.ToLower(ext))
}

// ImportEpisode links src to the standard series location.
func ImportEpisode(src, destRoot, series string, year, season, episode int, epTitle string) (*ImportResult, error) {
	fi, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	dest := EpisodeDestination(destRoot, series, year, season, episode, epTitle, filepath.Ext(src))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	if existing, err := os.Stat(dest); err == nil {
		if os.SameFile(fi, existing) {
			return &ImportResult{Source: src, Destination: dest, Size: fi.Size(), Hardlinked: true}, nil
		}
		return nil, fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.Link(src, dest); err != nil {
		if isCrossDevice(err) {
			return nil, fmt.Errorf(
				"cannot hardlink %s -> %s: they are on different filesystems. "+
					"Point paths.downloads and paths.tv at the same volume", src, dest)
		}
		return nil, fmt.Errorf("link %s -> %s: %w", src, dest, err)
	}
	return &ImportResult{Source: src, Destination: dest, Size: fi.Size(), Hardlinked: true}, nil
}
