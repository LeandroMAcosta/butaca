package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LeandroMAcosta/butaca/internal/config"
)

func enter() tea.Msg { return tea.KeyMsg{Type: tea.KeyEnter} }
func down() tea.Msg  { return tea.KeyMsg{Type: tea.KeyDown} }

func typeText(m tea.Model, s string) tea.Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

// selectAll clears the input by sending backspaces, so a step prefilled with a
// default can be overwritten.
func clear(m tea.Model, n int) tea.Model {
	for i := 0; i < n; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	return m
}

func newCfg(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	c, err := config.Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSetupWizardWritesAnswers(t *testing.T) {
	cfg := newCfg(t)
	var m tea.Model = NewSetup(cfg)

	// 1 movies, 2 tv, 3 downloads: overwrite each default.
	for _, path := range []string{"/media/movies", "/media/tv", "/media/downloads"} {
		m = clear(m, 200)
		m = typeText(m, path)
		m, _ = m.Update(enter())
	}
	// 4 language mode: the first option is "original".
	m, _ = m.Update(enter())
	// 5 subtitles: second option, "Spanish and English".
	m, _ = m.Update(down())
	m, _ = m.Update(enter())
	// 6 resolution: first option, 1080p.
	m, _ = m.Update(enter())
	// 7 prowlarr url, 8 api key, 9 qbittorrent url.
	m, _ = m.Update(enter())
	m = typeText(m, "secret-key")
	m, _ = m.Update(enter())
	m, _ = m.Update(enter())

	if cfg.Paths.Movies != "/media/movies" {
		t.Errorf("movies = %q", cfg.Paths.Movies)
	}
	if cfg.Paths.TV != "/media/tv" {
		t.Errorf("tv = %q", cfg.Paths.TV)
	}
	if cfg.Rules.LanguageMode != "original" {
		t.Errorf("language_mode = %q, want original", cfg.Rules.LanguageMode)
	}
	if got := strings.Join(cfg.Subtitles.Languages, ","); got != "es,en" {
		t.Errorf("subtitles = %q, want es,en", got)
	}
	if cfg.Prowlarr.APIKey != "secret-key" {
		t.Errorf("api key = %q", cfg.Prowlarr.APIKey)
	}
	if !cfg.Subtitles.Auto {
		t.Error("subtitles should be enabled")
	}
}

// "Prefer one language" reveals a follow-up step that "Original" must skip.
func TestPreferLanguageRevealsExtraStep(t *testing.T) {
	cfg := newCfg(t)
	var m tea.Model = NewSetup(cfg)

	for i := 0; i < 3; i++ { // the three path steps
		m, _ = m.Update(enter())
	}
	m, _ = m.Update(down()) // move from "Original" to "Prefer one language"
	m, _ = m.Update(enter())

	view := m.View()
	if !strings.Contains(view, "Which language do you prefer?") {
		t.Fatalf("choosing Prefer should reveal the language step, got:\n%s", view)
	}

	m, _ = m.Update(enter()) // Spanish
	if cfg.Rules.PreferLanguage != "" {
		t.Fatal("config must not be written until the wizard finishes")
	}
	for i := 0; i < 5; i++ { // subtitles, resolution, three connection steps
		m, _ = m.Update(enter())
	}
	if cfg.Rules.PreferLanguage != "es" {
		t.Errorf("prefer_language = %q, want es", cfg.Rules.PreferLanguage)
	}
	if cfg.Rules.LanguageMode != "prefer" {
		t.Errorf("language_mode = %q, want prefer", cfg.Rules.LanguageMode)
	}
}

func TestOriginalModeSkipsLanguageChoice(t *testing.T) {
	cfg := newCfg(t)
	var m tea.Model = NewSetup(cfg)
	for i := 0; i < 3; i++ {
		m, _ = m.Update(enter())
	}
	m, _ = m.Update(enter()) // Original
	if strings.Contains(m.View(), "Which language do you prefer?") {
		t.Fatal("Original mode must not ask which language to prefer")
	}
}

func TestEscapeAbortsWithoutWriting(t *testing.T) {
	cfg := newCfg(t)
	var m tea.Model = NewSetup(cfg)
	m = clear(m, 200)
	m = typeText(m, "/should/not/persist")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cfg.Paths.Movies == "/should/not/persist" {
		t.Fatal("escape must discard the answers")
	}
	if !strings.Contains(m.View(), "cancelled") {
		t.Fatalf("expected a cancellation notice, got: %s", m.View())
	}
}

func TestSubtitlesNoneDisablesAuto(t *testing.T) {
	cfg := newCfg(t)
	var m tea.Model = NewSetup(cfg)
	for i := 0; i < 3; i++ {
		m, _ = m.Update(enter())
	}
	m, _ = m.Update(enter()) // original
	for i := 0; i < 3; i++ { // move to "None"
		m, _ = m.Update(down())
	}
	m, _ = m.Update(enter())
	for i := 0; i < 4; i++ {
		m, _ = m.Update(enter())
	}
	if cfg.Subtitles.Auto {
		t.Error("choosing None must disable automatic subtitles")
	}
	if len(cfg.Subtitles.Languages) != 0 {
		t.Errorf("languages = %v, want empty", cfg.Subtitles.Languages)
	}
}
