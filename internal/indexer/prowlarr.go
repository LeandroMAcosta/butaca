// Package indexer talks to Prowlarr, which normalizes every configured tracker
// behind one search API. butaca never speaks to a tracker directly.
package indexer

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Release struct {
	Title       string `json:"title"`
	Indexer     string `json:"indexer"`
	IndexerID   int    `json:"indexerId"`
	Size        int64  `json:"size"`
	Seeders     int    `json:"seeders"`
	Leechers    int    `json:"leechers"`
	MagnetURL   string `json:"magnetUrl"`
	DownloadURL string `json:"downloadUrl"`
	InfoHash    string `json:"infoHash"`
	GUID        string `json:"guid"`
	Protocol    string `json:"protocol"`
	PublishDate string `json:"publishDate"`
	IMDBID      int64  `json:"imdbId"`
}

type Indexer struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Enable   bool   `json:"enable"`
	Protocol string `json:"protocol"`
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		// Indexer fan-out is slow; Prowlarr waits on the slowest tracker.
		http: &http.Client{Timeout: 4 * time.Minute},
	}
}

func (c *Client) do(ctx context.Context, path string, q url.Values, out any) error {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("prowlarr unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prowlarr %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Search fans the query out to every enabled indexer and returns the merged,
// normalized result set.
func (c *Client) Search(ctx context.Context, query string) ([]Release, error) {
	q := url.Values{}
	q.Set("query", query)
	q.Set("type", "search")
	var out []Release
	if err := c.do(ctx, "/api/v1/search", q, &out); err != nil {
		return nil, err
	}
	// Some trackers hand Prowlarr HTML-encoded titles ("Am&eacute;lie"), which
	// would otherwise fail every title comparison downstream.
	for i := range out {
		out[i].Title = html.UnescapeString(out[i].Title)
	}
	return out, nil
}

func (c *Client) Indexers(ctx context.Context) ([]Indexer, error) {
	var out []Indexer
	if err := c.do(ctx, "/api/v1/indexer", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Link returns whatever qBittorrent can be handed for this release.
func (r Release) Link() string {
	if r.MagnetURL != "" {
		return r.MagnetURL
	}
	return r.DownloadURL
}
