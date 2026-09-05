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
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 2 * time.Minute},
	}
}

func (c *Client) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
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
}

func (c *Client) Subtitles(ctx context.Context, path string, languages []string) (*SubtitleResult, error) {
	var out SubtitleResult
	err := c.post(ctx, "/subtitles", map[string]any{"path": path, "languages": languages}, &out)
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
