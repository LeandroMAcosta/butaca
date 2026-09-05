package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

type Series struct {
	TMDBID           int64  `json:"id"`
	Name             string `json:"name"`
	OriginalName     string `json:"original_name"`
	OriginalLanguage string `json:"original_language"`
	FirstAirDate     string `json:"first_air_date"`
	Overview         string `json:"overview"`
}

func (s Series) Year() int {
	if len(s.FirstAirDate) < 4 {
		return 0
	}
	y, _ := strconv.Atoi(s.FirstAirDate[:4])
	return y
}

// Titles returns every name the series is released under.
func (s Series) Titles() []string {
	out := []string{s.Name}
	if s.OriginalName != "" && s.OriginalName != s.Name {
		out = append(out, s.OriginalName)
	}
	return out
}

type Season struct {
	Number       int    `json:"season_number"`
	Name         string `json:"name"`
	EpisodeCount int    `json:"episode_count"`
}

type Episode struct {
	Number  int    `json:"episode_number"`
	Season  int    `json:"season_number"`
	Name    string `json:"name"`
	AirDate string `json:"air_date"`
}

func (c *Client) SearchSeries(ctx context.Context, query string, year int) ([]Series, error) {
	if !c.Enabled() {
		return nil, ErrNoAPIKey
	}
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	q.Set("query", query)
	if year > 0 {
		q.Set("first_air_date_year", strconv.Itoa(year))
	}
	var body struct {
		Results []Series `json:"results"`
	}
	if err := c.getJSON(ctx, "https://api.themoviedb.org/3/search/tv?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	return body.Results, nil
}

// Seasons lists a series' seasons, skipping season 0 (specials).
func (c *Client) Seasons(ctx context.Context, tmdbID int64) ([]Season, error) {
	if !c.Enabled() {
		return nil, ErrNoAPIKey
	}
	q := url.Values{"api_key": {c.apiKey}}
	var body struct {
		Seasons []Season `json:"seasons"`
	}
	u := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d?%s", tmdbID, q.Encode())
	if err := c.getJSON(ctx, u, &body); err != nil {
		return nil, err
	}
	out := make([]Season, 0, len(body.Seasons))
	for _, s := range body.Seasons {
		if s.Number > 0 {
			out = append(out, s)
		}
	}
	return out, nil
}

func (c *Client) Episodes(ctx context.Context, tmdbID int64, season int) ([]Episode, error) {
	if !c.Enabled() {
		return nil, ErrNoAPIKey
	}
	q := url.Values{"api_key": {c.apiKey}}
	var body struct {
		Episodes []Episode `json:"episodes"`
	}
	u := fmt.Sprintf("https://api.themoviedb.org/3/tv/%d/season/%d?%s", tmdbID, season, q.Encode())
	if err := c.getJSON(ctx, u, &body); err != nil {
		return nil, err
	}
	return body.Episodes, nil
}

func (c *Client) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("tmdb rejected the API key")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("tmdb: HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
