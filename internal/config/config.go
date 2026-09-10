// Package config loads butaca settings from YAML with environment overrides.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Paths struct {
	Movies string `yaml:"movies"`
	// Documentaries is an optional second movie library. Empty means every
	// movie goes to Movies.
	Documentaries string `yaml:"documentaries,omitempty"`
	TV            string `yaml:"tv"`
	Downloads     string `yaml:"downloads"`
}

type Service struct {
	URL    string `yaml:"url"`
	APIKey string `yaml:"api_key"`
}

type QBittorrent struct {
	URL      string `yaml:"url"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Category string `yaml:"category"`
}

// Rules drives release selection. See internal/decide.
type Rules struct {
	MinSeeders     int      `yaml:"min_seeders"`
	Resolutions    []string `yaml:"resolutions"`
	Sources        []string `yaml:"sources"`
	MinSize        string   `yaml:"min_size"`
	MaxSize        string   `yaml:"max_size"`
	PreferGroups   []string `yaml:"prefer_groups"`
	RejectPatterns []string `yaml:"reject_patterns"`
	LanguageMode   string   `yaml:"language_mode"` // original | prefer | any
	PreferLanguage string   `yaml:"prefer_language"`
}

type Subtitles struct {
	Languages []string `yaml:"languages"`
	Auto      bool     `yaml:"auto"`
}

type Config struct {
	DataDir     string      `yaml:"data_dir"`
	Paths       Paths       `yaml:"paths"`
	Prowlarr    Service     `yaml:"prowlarr"`
	QBittorrent QBittorrent `yaml:"qbittorrent"`
	Parse       Service     `yaml:"parse"`
	TMDB        Service     `yaml:"tmdb"`
	Rules       Rules       `yaml:"rules"`
	Subtitles   Subtitles   `yaml:"subtitles"`

	path string `yaml:"-"`
}

// homeDir resolves the user's home directory. When the environment does not
// provide one -- as inside a bare sandbox or a launcher with a scrubbed
// environment -- it reports false rather than yielding "", which would silently
// turn every derived path into a relative one.
func homeDir() (string, bool) {
	h, err := os.UserHomeDir()
	if err != nil || h == "" || !filepath.IsAbs(h) {
		return "", false
	}
	return h, true
}

func Default() *Config {
	home, _ := homeDir()
	return &Config{
		DataDir: filepath.Join(home, ".config", "butaca"),
		Paths: Paths{
			Movies:    filepath.Join(home, "Movies"),
			TV:        filepath.Join(home, "TV"),
			Downloads: filepath.Join(home, "Downloads"),
		},
		Prowlarr:    Service{URL: "http://localhost:9696"},
		QBittorrent: QBittorrent{URL: "http://localhost:8080", Category: "butaca"},
		Parse:       Service{URL: "http://localhost:8000"},
		Rules: Rules{
			MinSeeders:     5,
			Resolutions:    []string{"1080p"},
			Sources:        []string{"Blu-ray", "Web", "HDTV", "DVD"},
			MinSize:        "500MB",
			MaxSize:        "20GB",
			RejectPatterns: []string{"CAM", "TS", "HDCAM", "TELESYNC", "SCREENER"},
			LanguageMode:   "original",
		},
		Subtitles: Subtitles{Languages: []string{"es"}, Auto: true},
	}
}

// DefaultPath is where the config lives unless overridden by --config.
func DefaultPath() string {
	home, ok := homeDir()
	if !ok {
		return ""
	}
	return filepath.Join(home, ".config", "butaca", "config.yaml")
}

// Load reads path, falling back to defaults for anything unset. A missing file
// is not an error: butaca runs on defaults until the user writes one.
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultPath()
	}
	cfg := Default()
	cfg.path = path

	if path == "" {
		// No home and no --config: only an explicit data directory can save us,
		// and saying so beats creating ".config/butaca" under whatever the
		// working directory happens to be.
		applyEnv(cfg)
		if !filepath.IsAbs(cfg.DataDir) {
			return nil, errors.New(
				"cannot determine your home directory: set BUTACA_DATA_DIR to an absolute path, or pass --config")
		}
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			applyEnv(cfg)
			if !filepath.IsAbs(cfg.DataDir) {
				return nil, errors.New(
					"cannot determine your home directory: set BUTACA_DATA_DIR to an absolute path")
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.path = path
	applyEnv(cfg)
	if !filepath.IsAbs(cfg.DataDir) {
		return nil, fmt.Errorf("data_dir %q must be an absolute path", cfg.DataDir)
	}
	return cfg, nil
}

// applyEnv lets BUTACA_* and the usual API-key variables win over the file, so
// secrets can stay out of it entirely.
func applyEnv(c *Config) {
	set := func(dst *string, keys ...string) {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				*dst = v
				return
			}
		}
	}
	set(&c.Paths.Movies, "BUTACA_MOVIES_PATH")
	set(&c.Paths.Documentaries, "BUTACA_DOCUMENTARIES_PATH")
	set(&c.Paths.TV, "BUTACA_TV_PATH")
	set(&c.Paths.Downloads, "BUTACA_DOWNLOADS_PATH")
	set(&c.Prowlarr.URL, "BUTACA_PROWLARR_URL")
	set(&c.Prowlarr.APIKey, "BUTACA_PROWLARR_API_KEY", "PROWLARR_API_KEY")
	set(&c.QBittorrent.URL, "BUTACA_QBITTORRENT_URL")
	set(&c.QBittorrent.Username, "BUTACA_QBITTORRENT_USER")
	set(&c.QBittorrent.Password, "BUTACA_QBITTORRENT_PASS")
	set(&c.Parse.URL, "BUTACA_PARSE_URL")
	set(&c.TMDB.APIKey, "BUTACA_TMDB_API_KEY", "TMDB_API_KEY")
	set(&c.DataDir, "BUTACA_DATA_DIR")
}

func (c *Config) Path() string { return c.path }

func (c *Config) DBPath() string { return filepath.Join(c.DataDir, "butaca.db") }

// Save writes the config back, creating the directory if needed.
func (c *Config) Save() error {
	if c.path == "" {
		c.path = DefaultPath()
	}
	if c.path == "" {
		return errors.New("no config path: pass --config, or set HOME")
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, data, 0o600)
}

// Validate catches the misconfigurations that fail silently at runtime.
func (c *Config) Validate() []string {
	var problems []string
	if c.Paths.Movies == "" {
		problems = append(problems, "paths.movies is empty")
	}
	if c.Paths.Downloads == "" {
		problems = append(problems, "paths.downloads is empty")
	}
	if c.Prowlarr.APIKey == "" {
		problems = append(problems, "prowlarr.api_key is empty (set PROWLARR_API_KEY)")
	}
	switch strings.ToLower(c.Rules.LanguageMode) {
	case "original", "prefer", "any":
	default:
		problems = append(problems, fmt.Sprintf("rules.language_mode %q must be original, prefer or any", c.Rules.LanguageMode))
	}
	return problems
}
