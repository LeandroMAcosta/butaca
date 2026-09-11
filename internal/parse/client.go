// Package parse is the client for butaca-parse, the Python sidecar that owns
// guessit and subliminal.
package parse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Result mirrors butaca-parse's ParseResult. Every field is best-effort: a
// release title that omits the codec simply leaves it empty.
type Result struct {
	Title            string `json:"title"`
	Year             int    `json:"year"`
	Language         string `json:"language"`
	SubtitleLanguage string `json:"subtitle_language"`
	ScreenSize       string `json:"screen_size"`
	Source           string `json:"source"`
	VideoCodec       string `json:"video_codec"`
	AudioCodec       string `json:"audio_codec"`
	AudioChannels    string `json:"audio_channels"`
	ReleaseGroup     string `json:"release_group"`
	Season           int    `json:"season"`
	Episode          int    `json:"episode"`
	Type             string `json:"type"`
	Raw              string `json:"raw"`
}

type Client struct {
	baseURL string
	http    *http.Client
	// slow serves subtitle calls: syncing one film against its audio can try
	// ffsubsync twice and alass once, minutes each.
	slow *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 2 * time.Minute},
		slow:    &http.Client{Timeout: 45 * time.Minute},
	}
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	return c.postWith(ctx, c.http, path, in, out)
}

func (c *Client) postWith(ctx context.Context, hc *http.Client, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("butaca-parse unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		return fmt.Errorf("butaca-parse %s: HTTP %d: %s", path, resp.StatusCode, strings.TrimSpace(buf.String()))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Parse sends every title in one request; the sidecar is batch by design.
func (c *Client) Parse(ctx context.Context, titles []string) ([]Result, error) {
	if len(titles) == 0 {
		return nil, nil
	}
	var out []Result
	err := c.post(ctx, "/parse", map[string]any{"titles": titles}, &out)
	return out, err
}

type SubtitleResult struct {
	Downloaded []string `json:"downloaded"`
	Skipped    []string `json:"skipped"`
	// Embedded lists requested languages the file already carries as a track.
	Embedded []string          `json:"embedded"`
	Results  []FetchedSubtitle `json:"results"`
}

// FetchedSubtitle is one downloaded subtitle. HashMatch means it was made for
// this exact file, so it is left alone; anything else went through Sync.
type FetchedSubtitle struct {
	Lang      string       `json:"lang"`
	Path      string       `json:"path"`
	Provider  string       `json:"provider"`
	Score     int          `json:"score"`
	HashMatch bool         `json:"hash_match"`
	Sync      *SyncOutcome `json:"sync"`
}

// SyncOutcome reports what happened to one subtitle.
type SyncOutcome struct {
	Status         string   `json:"status"` // synced | refetched | rejected | failed | skipped
	Method         string   `json:"method"` // hash | embedded | audio | alass-embedded | alass-audio
	OffsetSeconds  *float64 `json:"offset_seconds"`
	FramerateScale *float64 `json:"framerate_scale"`
	Score          *float64 `json:"score"`
	Detail         string   `json:"detail"`
	Attempts       []string `json:"attempts"`
}

func (o SyncOutcome) String() string {
	s := o.Status
	if o.Method != "" {
		s += " via " + o.Method
	}
	if o.Detail != "" {
		s += ": " + o.Detail
	}
	return s
}

// Subtitles fetches the missing languages for a video. releaseName is what
// the file was called before import; it helps scoring and may be empty.
func (c *Client) Subtitles(ctx context.Context, path string, languages []string, releaseName string) (*SubtitleResult, error) {
	var out SubtitleResult
	err := c.postWith(ctx, c.slow, "/subtitles", map[string]any{
		"path": path, "languages": languages, "release_name": releaseName, "sync": true,
	}, &out)
	return &out, err
}

// Sync fixes the subtitle already next to a video: a hash-matched download
// replaces it when one exists, otherwise it is synced in place. The sidecar
// keeps the original as "<name>.<lang>.srt.orig" and skips it next time
// unless force is set.
func (c *Client) Sync(ctx context.Context, path, lang, releaseName string, force bool) (*SyncOutcome, error) {
	var out SyncOutcome
	err := c.postWith(ctx, c.slow, "/sync", map[string]any{
		"path": path, "lang": lang, "release_name": releaseName, "force": force,
	}, &out)
	return &out, err
}

func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("butaca-parse unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("butaca-parse health: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Track is one audio or subtitle stream inside a media file.
type Track struct {
	Kind  string `json:"kind"` // audio | subtitle
	Lang  string `json:"lang"`
	Title string `json:"title"`
}

type TracksResult struct {
	Tracks []Track `json:"tracks"`
}

// Tracks probes a file for its audio and subtitle streams. The sidecar shells
// out to ffprobe, which is the only reliable way to know what a container
// actually holds -- the release name routinely lies or says nothing.
func (c *Client) Tracks(ctx context.Context, path string) ([]Track, error) {
	var out TracksResult
	if err := c.post(ctx, "/tracks", map[string]any{"path": path}, &out); err != nil {
		return nil, err
	}
	return out.Tracks, nil
}
