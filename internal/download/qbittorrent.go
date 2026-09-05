// Package download drives qBittorrent's WebUI API. butaca hands it a magnet and
// polls; it never touches the torrent protocol itself.
package download

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Torrent struct {
	Hash        string  `json:"hash"`
	Name        string  `json:"name"`
	State       string  `json:"state"`
	Progress    float64 `json:"progress"`
	Size        int64   `json:"size"`
	ContentPath string  `json:"content_path"`
	SavePath    string  `json:"save_path"`
	NumSeeds    int     `json:"num_seeds"`
	DlSpeed     int64   `json:"dlspeed"`
	Ratio       float64 `json:"ratio"`
}

// Done reports whether the payload is fully on disk. qBittorrent keeps torrents
// around seeding after completion, so progress is the reliable signal.
func (t Torrent) Done() bool { return t.Progress >= 1.0 }

type Client struct {
	baseURL  string
	username string
	password string
	category string
	http     *http.Client

	loginOnce sync.Once
	loginErr  error
}

func New(baseURL, username, password, category string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		username: username,
		password: password,
		category: category,
		http:     &http.Client{Timeout: 60 * time.Second, Jar: jar},
	}
}

// login runs at most once. Installations that bypass auth for localhost need no
// credentials, so empty username is not an error.
func (c *Client) login(ctx context.Context) error {
	c.loginOnce.Do(func() {
		if c.username == "" {
			return
		}
		form := url.Values{"username": {c.username}, "password": {c.password}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/api/v2/auth/login", strings.NewReader(form.Encode()))
		if err != nil {
			c.loginErr = err
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", c.baseURL)
		resp, err := c.http.Do(req)
		if err != nil {
			c.loginErr = fmt.Errorf("qbittorrent unreachable at %s: %w", c.baseURL, err)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			c.loginErr = fmt.Errorf("qbittorrent login: HTTP %d", resp.StatusCode)
		}
	})
	return c.loginErr
}

func (c *Client) post(ctx context.Context, path string, form url.Values) error {
	if err := c.login(ctx); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", c.baseURL)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("qbittorrent unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbittorrent %s: HTTP %d", path, resp.StatusCode)
	}
	return nil
}

func (c *Client) Version(ctx context.Context) (string, error) {
	if err := c.login(ctx); err != nil {
		return "", err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/app/version", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("qbittorrent unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	return strings.TrimSpace(string(buf[:n])), nil
}

// Add submits a magnet or .torrent URL under butaca's category.
func (c *Client) Add(ctx context.Context, link string) error {
	return c.post(ctx, "/api/v2/torrents/add", url.Values{
		"urls":     {link},
		"category": {c.category},
	})
}

// Delete removes the torrent and, when deleteFiles is set, its payload. Both
// halves matter: with hardlinks, removing only one name frees nothing.
func (c *Client) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	return c.post(ctx, "/api/v2/torrents/delete", url.Values{
		"hashes":      {strings.ToLower(hash)},
		"deleteFiles": {fmt.Sprint(deleteFiles)},
	})
}

func (c *Client) List(ctx context.Context) ([]Torrent, error) {
	if err := c.login(ctx); err != nil {
		return nil, err
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/torrents/info", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qbittorrent unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qbittorrent torrents/info: HTTP %d", resp.StatusCode)
	}
	var out []Torrent
	return out, json.NewDecoder(resp.Body).Decode(&out)
}

// Get returns one torrent by hash, or nil when qBittorrent does not have it.
func (c *Client) Get(ctx context.Context, hash string) (*Torrent, error) {
	all, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(hash)
	for i := range all {
		if strings.ToLower(all[i].Hash) == want {
			return &all[i], nil
		}
	}
	return nil, nil
}
