// Package tui holds butaca's terminal interfaces. Today that is the first-run
// setup wizard; the full catalog browser lands later.
package tui

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/config"
)

type kind int

const (
	kindInput kind = iota
	kindChoice
)

type option struct {
	label string
	value string
	help  string
}

type step struct {
	kind        kind
	title       string
	help        string
	placeholder string
	value       string
	options     []option
	cursor      int
	// visibleIf hides a step unless the answers so far call for it.
	visibleIf func(answers map[string]string) bool
	key       string
}

func home(sub ...string) string {
	h, _ := os.UserHomeDir()
	return filepath.Join(append([]string{h}, sub...)...)
}

// steps is the wizard, in order. Everything here maps onto a config field.
func newSteps(cfg *config.Config) []step {
	return []step{
		{
			key:         "movies",
			kind:        kindInput,
			title:       "Where should movies live?",
			help:        "The library folder. butaca links finished downloads into it.",
			placeholder: cfg.Paths.Movies,
			value:       cfg.Paths.Movies,
		},
		{
			key:         "tv",
			kind:        kindInput,
			title:       "Where should series live?",
			help:        "Keep this separate from movies. Sharing one folder makes each app scan the other's library.",
			placeholder: home("TV Shows"),
			value:       cfg.Paths.TV,
		},
		{
			key:         "downloads",
			kind:        kindInput,
			title:       "Where does your download client save files?",
			help:        "Must be on the same disk as the folders above: imports are hardlinks, and a hardlink cannot cross filesystems.",
			placeholder: cfg.Paths.Downloads,
			value:       cfg.Paths.Downloads,
		},
		{
			key:   "language_mode",
			kind:  kindChoice,
			title: "Which audio language should butaca accept?",
			help:  "This is the rule that decides whether a foreign-language film can be found at all.",
			options: []option{
				{"Original", "original", "Each film in its own language: Amélie in French, Exit 8 in Japanese. Recommended."},
				{"Prefer one language", "prefer", "Rank one language first, but never reject the others."},
				{"Any", "any", "Ignore audio language entirely."},
			},
		},
		{
			key:   "prefer_language",
			kind:  kindChoice,
			title: "Which language do you prefer?",
			help:  "Ranked first when available. Nothing is rejected for being in another language.",
			options: []option{
				{"Spanish", "es", ""},
				{"English", "en", ""},
				{"French", "fr", ""},
				{"German", "de", ""},
				{"Japanese", "ja", ""},
			},
			visibleIf: func(a map[string]string) bool { return a["language_mode"] == "prefer" },
		},
		{
			key:   "subtitles",
			kind:  kindChoice,
			title: "Which subtitles should butaca fetch automatically?",
			help:  "Downloaded next to the video, so any player picks them up without configuration.",
			options: []option{
				{"Spanish", "es", ""},
				{"Spanish and English", "es,en", ""},
				{"English", "en", ""},
				{"None", "", "Skip subtitles entirely."},
			},
		},
		{
			key:   "resolution",
			kind:  kindChoice,
			title: "Preferred quality?",
			help:  "Releases outside this are rejected, so pick the lowest you would still accept.",
			options: []option{
				{"1080p", "1080p", "About 2 GB per film."},
				{"1080p or 720p", "1080p,720p", "Falls back to 720p when no 1080p release qualifies."},
				{"4K, then 1080p", "2160p,1080p", "Files can exceed 50 GB."},
			},
		},
		{
			key:         "prowlarr_url",
			kind:        kindInput,
			title:       "Prowlarr URL",
			help:        "butaca searches every tracker through Prowlarr.",
			placeholder: cfg.Prowlarr.URL,
			value:       cfg.Prowlarr.URL,
		},
		{
			key:         "prowlarr_key",
			kind:        kindInput,
			title:       "Prowlarr API key",
			help:        "Prowlarr → Settings → General → API Key. Leave empty to set PROWLARR_API_KEY in the environment instead.",
			placeholder: "paste it here",
			value:       cfg.Prowlarr.APIKey,
		},
		{
			key:         "qbittorrent_url",
			kind:        kindInput,
			title:       "qBittorrent URL",
			help:        "Its Web UI address.",
			placeholder: cfg.QBittorrent.URL,
			value:       cfg.QBittorrent.URL,
		},
	}
}

// apply writes the collected answers onto the config.
func apply(cfg *config.Config, a map[string]string) {
	if v := a["movies"]; v != "" {
		cfg.Paths.Movies = expand(v)
	}
	if v := a["tv"]; v != "" {
		cfg.Paths.TV = expand(v)
	}
	if v := a["downloads"]; v != "" {
		cfg.Paths.Downloads = expand(v)
	}
	if v := a["language_mode"]; v != "" {
		cfg.Rules.LanguageMode = v
	}
	if v := a["prefer_language"]; v != "" {
		cfg.Rules.PreferLanguage = v
	}
	if v, ok := a["subtitles"]; ok {
		if v == "" {
			cfg.Subtitles.Auto = false
			cfg.Subtitles.Languages = nil
		} else {
			cfg.Subtitles.Auto = true
			cfg.Subtitles.Languages = strings.Split(v, ",")
		}
	}
	if v := a["resolution"]; v != "" {
		cfg.Rules.Resolutions = strings.Split(v, ",")
	}
	if v := a["prowlarr_url"]; v != "" {
		cfg.Prowlarr.URL = v
	}
	if v := a["prowlarr_key"]; v != "" {
		cfg.Prowlarr.APIKey = v
	}
	if v := a["qbittorrent_url"]; v != "" {
		cfg.QBittorrent.URL = v
	}
}

// expand resolves a leading ~ so typed paths behave as users expect.
func expand(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(home(), strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
	}
	return p
}
