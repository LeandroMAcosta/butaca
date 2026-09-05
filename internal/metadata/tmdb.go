// Package metadata resolves titles against TMDB. The field that matters most to
// butaca is original_language: the decision engine compares releases against a
// film's real language instead of a hardcoded one.
package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var ErrNoAPIKey = errors.New("no TMDB API key configured")

type Movie struct {
	TMDBID           int64   `json:"id"`
	Title            string  `json:"title"`
	OriginalTitle    string  `json:"original_title"`
	OriginalLanguage string  `json:"original_language"`
	ReleaseDate      string  `json:"release_date"`
	Overview         string  `json:"overview"`
	VoteAverage      float64 `json:"vote_average"`
	VoteCount        int     `json:"vote_count"`
	Popularity       float64 `json:"popularity"`
	GenreIDs         []int   `json:"genre_ids"`
}

// Titles returns every name the film is released under, most common first.
// The decision engine matches release names against all of them.
func (m Movie) Titles() []string {
	out := []string{m.Title}
	if m.OriginalTitle != "" && m.OriginalTitle != m.Title {
		out = append(out, m.OriginalTitle)
	}
	return out
}

// Year extracts the release year; TMDB sends an empty string for unreleased titles.
func (m Movie) Year() int {
	if len(m.ReleaseDate) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(m.ReleaseDate[:4])
	return y
}

type Client struct {
	apiKey string
	http   *http.Client
}

func New(apiKey string) *Client {
	return &Client{apiKey: apiKey, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Enabled() bool { return c.apiKey != "" }

// SearchMovie returns TMDB matches ordered by relevance. year may be 0.
func (c *Client) SearchMovie(ctx context.Context, query string, year int) ([]Movie, error) {
	if !c.Enabled() {
		return nil, ErrNoAPIKey
	}
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	q.Set("query", query)
	q.Set("include_adult", "false")
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.themoviedb.org/3/search/movie?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tmdb unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("tmdb rejected the API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tmdb search: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Results []Movie `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Results, nil
}
