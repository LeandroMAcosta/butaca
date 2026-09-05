// Package letterboxd reads a member's public lists and watchlist.
//
// Letterboxd has no public API. The member RSS feed carries lists but omits
// TMDB ids, and the watchlist and ratings feeds answer 403, so the watchlist
// has to come from the HTML pages. That makes this package a scraper: it is
// deliberately isolated here so that when Letterboxd changes its markup, one
// file breaks loudly instead of the whole application misbehaving quietly.
package letterboxd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// browserUA is required: Letterboxd answers 403 to a default Go user agent.
const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

const baseURL = "https://letterboxd.com"

// ErrBlocked means Letterboxd refused the request, which is how it signals both
// a private profile and an endpoint it does not want scraped.
var ErrBlocked = errors.New("letterboxd refused the request")

type Film struct {
	Slug  string
	Title string
	Year  int
}

type List struct {
	Name string
	Slug string
}

type Client struct {
	http *http.Client
	// pageLimit bounds pagination so a huge watchlist cannot loop forever.
	pageLimit int
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}, pageLimit: 40}
}

func (c *Client) get(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("letterboxd unreachable: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusForbidden:
		return "", fmt.Errorf("%w for %s (is the profile public?)", ErrBlocked, path)
	case http.StatusNotFound:
		return "", fmt.Errorf("letterboxd has nothing at %s", path)
	default:
		return "", fmt.Errorf("letterboxd %s: HTTP %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	return string(body), err
}

var (
	// The film grid tags each poster with its slug and display name; both live
	// on the same element, so one pass over the page yields title and year too.
	reItem  = regexp.MustCompile(`data-item-slug="([^"]+)"[^>]*?data-item-link="[^"]*"`)
	reName  = regexp.MustCompile(`data-item-name="([^"]*)"[^>]*?data-item-slug="([^"]+)"`)
	reYear  = regexp.MustCompile(`^(.*)\s+\((\d{4})\)$`)
	reTMDB  = regexp.MustCompile(`data-tmdb-id="(\d+)"`)
	reList  = regexp.MustCompile(`<link>https://letterboxd\.com/[^/]+/list/([^/]+)/</link>`)
	reTitle = regexp.MustCompile(`<title>([^<]*)</title>`)
)

// parseFilms pulls every film tile out of a grid page.
func parseFilms(html string) []Film {
	names := map[string]string{}
	for _, m := range reName.FindAllStringSubmatch(html, -1) {
		names[m[2]] = m[1]
	}

	seen := map[string]bool{}
	var out []Film
	for _, m := range reItem.FindAllStringSubmatch(html, -1) {
		slug := m[1]
		if seen[slug] {
			continue
		}
		seen[slug] = true
		f := Film{Slug: slug, Title: names[slug]}
		if sub := reYear.FindStringSubmatch(f.Title); sub != nil {
			f.Title = strings.TrimSpace(sub[1])
			f.Year, _ = strconv.Atoi(sub[2])
		}
		if f.Title == "" {
			f.Title = strings.ReplaceAll(slug, "-", " ")
		}
		out = append(out, f)
	}
	return out
}

// paged walks a paginated grid until a page adds nothing new.
func (c *Client) paged(ctx context.Context, base string) ([]Film, error) {
	var all []Film
	seen := map[string]bool{}
	for page := 1; page <= c.pageLimit; page++ {
		path := base
		if page > 1 {
			path = strings.TrimRight(base, "/") + "/page/" + strconv.Itoa(page) + "/"
		}
		html, err := c.get(ctx, path)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break // a missing later page just means the list ended
		}
		added := 0
		for _, f := range parseFilms(html) {
			if seen[f.Slug] {
				continue
			}
			seen[f.Slug] = true
			all = append(all, f)
			added++
		}
		if added == 0 {
			break
		}
	}
	return all, nil
}

// Watchlist returns a member's public watchlist.
func (c *Client) Watchlist(ctx context.Context, user string) ([]Film, error) {
	return c.paged(ctx, "/"+user+"/watchlist/")
}

// ListFilms returns the films in one of a member's lists.
func (c *Client) ListFilms(ctx context.Context, user, slug string) ([]Film, error) {
	return c.paged(ctx, "/"+user+"/list/"+slug+"/")
}

// Lists enumerates a member's lists from their RSS feed, which is the one feed
// that answers without a 403.
func (c *Client) Lists(ctx context.Context, user string) ([]List, error) {
	body, err := c.get(ctx, "/"+user+"/rss/")
	if err != nil {
		return nil, err
	}
	var out []List
	seen := map[string]bool{}
	items := strings.Split(body, "<item>")
	for _, item := range items[1:] {
		m := reList.FindStringSubmatch(item)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		name := m[1]
		if t := reTitle.FindStringSubmatch(item); t != nil {
			name = strings.TrimSpace(t[1])
		}
		out = append(out, List{Name: name, Slug: m[1]})
	}
	return out, nil
}

// TMDBID resolves a film slug to its TMDB id, which is what lets a Letterboxd
// entry join the catalog. One request per film, and the answer never changes,
// so callers should cache it.
func (c *Client) TMDBID(ctx context.Context, slug string) (int64, error) {
	html, err := c.get(ctx, "/film/"+slug+"/")
	if err != nil {
		return 0, err
	}
	m := reTMDB.FindStringSubmatch(html)
	if m == nil {
		return 0, fmt.Errorf("no TMDB id on the page for %q", slug)
	}
	return strconv.ParseInt(m[1], 10, 64)
}
