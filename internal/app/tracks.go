package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/store"
)

// ProbeFile reads a file's audio and subtitle streams and records them.
func (a *App) ProbeFile(ctx context.Context, f *store.File) (int, error) {
	found, err := a.Parse.Tracks(ctx, f.Path)
	if err != nil {
		return 0, err
	}
	tracks := make([]store.Track, 0, len(found))
	for _, t := range found {
		tracks = append(tracks, store.Track{FileID: f.ID, Kind: t.Kind, Lang: t.Lang, Title: t.Title})
	}
	if err := a.Store.ReplaceTracks(f.ID, tracks); err != nil {
		return 0, err
	}
	return len(tracks), nil
}

// ScanTracks probes every catalogued file. Items migrated from another
// application have no track data until this runs.
func (a *App) ScanTracks(ctx context.Context, force bool) ([]string, error) {
	items, err := a.Store.ListItems("")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, it := range items {
		if !force {
			if langs, err := a.Store.LanguagesForItem(it.ID); err == nil && !langs.Empty() {
				continue
			}
		}
		files, err := a.Store.FilesForItem(it.ID)
		if err != nil {
			return out, err
		}
		total := 0
		for _, f := range files {
			n, err := a.ProbeFile(ctx, f)
			if err != nil {
				out = append(out, fmt.Sprintf("%s: %v", it.Title, err))
				continue
			}
			total += n
			total += a.registerSidecarSubtitles(f)
		}
		if total > 0 {
			langs, _ := a.Store.LanguagesForItem(it.ID)
			out = append(out, fmt.Sprintf("%s: %d tracks (audio %s, subs %s)",
				it.Title, total, store.Summary(langs.Audio, 4), store.Summary(langs.Subtitles, 4)))
		}
	}
	return out, nil
}

// looksLikeLanguage keeps the sidecar scanner from inventing languages out of
// release-name debris: "Movie [YTS.BZ].srt" would otherwise yield "BZ]".
func looksLikeLanguage(s string) bool {
	if len(s) < 2 || len(s) > 3 {
		return false
	}
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// subtitleExts are the sidecar formats players pick up automatically.
var subtitleExts = map[string]bool{".srt": true, ".ass": true, ".ssa": true, ".sub": true, ".vtt": true}

// registerSidecarSubtitles records the .srt files sitting next to a video.
// Migrated libraries have these on disk with nothing in the database, and they
// are usually the only reason a film is watchable in Spanish at all.
func (a *App) registerSidecarSubtitles(f *store.File) int {
	dir := filepath.Dir(f.Path)
	stem := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))

	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	// Drop what a previous scan recorded, so a corrected parser does not leave
	// its earlier mistakes behind.
	_ = a.Store.ClearSubtitles(f.ID)
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !subtitleExts[strings.ToLower(filepath.Ext(name))] || !strings.HasPrefix(name, stem) {
			continue
		}
		// "Movie.es.srt" -> es; "Movie.srt" carries no language claim.
		lang := strings.ToLower(langOf(name))
		if lang == "" || lang == stem {
			continue
		}
		// "Movie.en.hi.srt" reports "hi" for hearing-impaired; the language is
		// the segment before it.
		if lang == "hi" || lang == "sdh" || lang == "forced" {
			trimmed := strings.TrimSuffix(name, filepath.Ext(name))
			lang = langOf(strings.TrimSuffix(trimmed, filepath.Ext(trimmed)))
		}
		if !looksLikeLanguage(lang) {
			continue
		}
		if _, err := a.Store.AddSubtitle(f.ID, lang, filepath.Join(dir, name)); err == nil {
			n++
		}
	}
	return n
}
