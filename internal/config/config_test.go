package config

import (
	"path/filepath"
	"strings"
	"testing"
)

// Without a home directory every derived path used to become relative, so
// butaca would try to create ".config/butaca" under whatever the working
// directory happened to be -- and die on a read-only one. Failing with an
// actionable message is the correct behaviour.
func TestLoadWithoutHomeFailsClearly(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("BUTACA_DATA_DIR", "")

	_, err := Load("")
	if err == nil {
		t.Fatal("expected an error when the home directory cannot be resolved")
	}
	if !strings.Contains(err.Error(), "BUTACA_DATA_DIR") {
		t.Errorf("the error should say how to fix it, got: %v", err)
	}
}

func TestLoadWithoutHomeAcceptsExplicitDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", "")
	t.Setenv("BUTACA_DATA_DIR", dir)

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("an absolute BUTACA_DATA_DIR should be enough: %v", err)
	}
	if cfg.DataDir != dir {
		t.Errorf("DataDir = %q, want %q", cfg.DataDir, dir)
	}
	if !filepath.IsAbs(cfg.DBPath()) {
		t.Errorf("DBPath %q must be absolute", cfg.DBPath())
	}
}

func TestEnvOverridesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := Default()
	cfg.DataDir = dir
	cfg.Paths.Movies = "/from/file"
	if err := func() error { c := cfg; c.path = path; return c.Save() }(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BUTACA_MOVIES_PATH", "/from/env")
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Paths.Movies != "/from/env" {
		t.Errorf("movies = %q, want the environment to win", got.Paths.Movies)
	}
}

func TestValidateReportsLanguageMode(t *testing.T) {
	cfg := Default()
	cfg.Rules.LanguageMode = "nonsense"
	cfg.Prowlarr.APIKey = "k"
	problems := Validate(cfg)
	if len(problems) == 0 {
		t.Fatal("an invalid language_mode should be reported")
	}
}

func Validate(c *Config) []string { return c.Validate() }
