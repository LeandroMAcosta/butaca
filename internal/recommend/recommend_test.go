package recommend

import (
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/metadata"
)

func movie(id int64, title, lang string, vote float64, votes int) metadata.Movie {
	return metadata.Movie{TMDBID: id, Title: title, OriginalLanguage: lang, VoteAverage: vote, VoteCount: votes}
}

func TestRankPrefersAgreementAcrossSeeds(t *testing.T) {
	related := map[string][]metadata.Movie{
		"Taxi Driver":    {movie(1, "Mean Streets", "en", 7.4, 2000), movie(2, "Heat", "en", 8.0, 9000)},
		"Altered States": {movie(1, "Mean Streets", "en", 7.4, 2000)},
		"The Wall":       {movie(1, "Mean Streets", "en", 7.4, 2000)},
	}
	got := Rank(related, nil, DefaultOptions())
	if len(got) == 0 {
		t.Fatal("no suggestions")
	}
	// Mean Streets is reached from three seeds and should beat the better-rated
	// film reached from only one.
	if got[0].Movie.Title != "Mean Streets" {
		t.Errorf("top suggestion = %q, want Mean Streets", got[0].Movie.Title)
	}
	if len(got[0].Seeds) != 3 {
		t.Errorf("seeds = %v, want three", got[0].Seeds)
	}
}

// Suggesting something already owned is the fastest way to make the feature
// useless.
func TestRankExcludesWhatYouAlreadyHave(t *testing.T) {
	related := map[string][]metadata.Movie{
		"Soul": {movie(508442, "Soul", "en", 8.1, 9000), movie(11, "Up", "en", 7.9, 8000)},
	}
	got := Rank(related, map[int64]bool{508442: true}, DefaultOptions())
	for _, s := range got {
		if s.Movie.TMDBID == 508442 {
			t.Fatal("a film already in the catalog was suggested")
		}
	}
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(got))
	}
}

func TestRankDropsObscureEntries(t *testing.T) {
	related := map[string][]metadata.Movie{
		"seed": {movie(1, "Barely Rated", "en", 9.9, 3), movie(2, "Known", "en", 6.0, 5000)},
	}
	got := Rank(related, nil, DefaultOptions())
	if len(got) != 1 || got[0].Movie.Title != "Known" {
		t.Errorf("expected only the well-voted film, got %+v", got)
	}
}

func TestRankBoostsPreferredLanguage(t *testing.T) {
	related := map[string][]metadata.Movie{
		"seed": {movie(1, "English One", "en", 7.0, 5000), movie(2, "Spanish One", "es", 7.0, 5000)},
	}
	opt := DefaultOptions()
	opt.PreferLanguages = []string{"es"}
	got := Rank(related, nil, opt)
	if got[0].Movie.OriginalLanguage != "es" {
		t.Errorf("top = %q, want the preferred language to break the tie", got[0].Movie.Title)
	}
}

func TestRankRespectsLimit(t *testing.T) {
	var many []metadata.Movie
	for i := int64(1); i <= 100; i++ {
		many = append(many, movie(i, "Film", "en", 7, 1000))
	}
	opt := DefaultOptions()
	opt.Limit = 5
	if got := Rank(map[string][]metadata.Movie{"s": many}, nil, opt); len(got) != 5 {
		t.Errorf("got %d suggestions, want 5", len(got))
	}
}
