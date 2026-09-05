package letterboxd

import (
	"context"
	"os"
	"testing"
)

// Markup captured from a real watchlist page.
const watchlistHTML = `
<li class="poster-container">
<div data-likeable="true" data-image-width="125" data-image-height="187"
 data-item-name="Vivarium (2019)" data-item-slug="vivarium" data-item-link="/film/vivarium/"
 data-item-full-display-name="Vivarium (2019)"></div></li>
<li class="poster-container">
<div data-item-name="1984 (1956)" data-item-slug="1984" data-item-link="/film/1984/"></div></li>
`

func TestParseFilmsSplitsTitleAndYear(t *testing.T) {
	got := parseFilms(watchlistHTML)
	if len(got) != 2 {
		t.Fatalf("parsed %d films, want 2: %+v", len(got), got)
	}
	if got[0].Slug != "vivarium" || got[0].Title != "Vivarium" || got[0].Year != 2019 {
		t.Errorf("first film = %+v", got[0])
	}
	// A title that is itself a number must not be mistaken for a year.
	if got[1].Slug != "1984" || got[1].Title != "1984" || got[1].Year != 1956 {
		t.Errorf("second film = %+v", got[1])
	}
}

func TestParseFilmsDeduplicates(t *testing.T) {
	if got := parseFilms(watchlistHTML + watchlistHTML); len(got) != 2 {
		t.Errorf("parsed %d films from a doubled page, want 2", len(got))
	}
}

func TestParseFilmsEmptyPage(t *testing.T) {
	if got := parseFilms("<html><body>nothing here</body></html>"); len(got) != 0 {
		t.Errorf("parsed %d films from an empty page", len(got))
	}
}

func TestTMDBIDExtraction(t *testing.T) {
	if m := reTMDB.FindStringSubmatch(`<div data-tmdb-id="458305" data-tmdb-type="movie">`); m == nil || m[1] != "458305" {
		t.Errorf("failed to extract the TMDB id: %v", m)
	}
}

// Against the live site. Scraping breaks when markup changes, and this is what
// tells us it has.
func TestLiveWatchlist(t *testing.T) {
	user := os.Getenv("BUTACA_LETTERBOXD_USER")
	if user == "" {
		t.Skip("set BUTACA_LETTERBOXD_USER to run against letterboxd.com")
	}
	c := New()
	films, err := c.Watchlist(context.Background(), user)
	if err != nil {
		t.Fatalf("watchlist: %v", err)
	}
	t.Logf("%d films", len(films))
	for _, f := range films {
		t.Logf("  %s (%d) [%s]", f.Title, f.Year, f.Slug)
	}
	if len(films) == 0 {
		t.Skip("watchlist is empty; nothing to assert")
	}
	id, err := c.TMDBID(context.Background(), films[0].Slug)
	if err != nil {
		t.Fatalf("TMDB id for %s: %v", films[0].Slug, err)
	}
	if id <= 0 {
		t.Errorf("TMDB id = %d", id)
	}
	t.Logf("  %s -> tmdb %d", films[0].Slug, id)
}
