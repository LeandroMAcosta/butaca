package app

import (
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/metadata"
)

func TestFilterMoviesMatchesOriginalTitleWithoutAccents(t *testing.T) {
	results := []metadata.Movie{
		{TMDBID: 1, Title: "4 Months, 3 Weeks and 2 Days", OriginalTitle: "4 luni, 3 săptămâni și 2 zile", ReleaseDate: "2007-05-17"},
		{TMDBID: 2, Title: "4 luni", ReleaseDate: "2007-01-01"},
		{TMDBID: 1, Title: "4 Months, 3 Weeks and 2 Days", ReleaseDate: "2007-05-17"},
	}
	got := filterMovies(results, []string{"4 luni, 3 saptamâni si 2 zile"}, func(y int) bool { return y == 2007 })
	if len(got) != 1 || got[0].TMDBID != 1 {
		t.Errorf("got %+v, want only tmdb 1, once", got)
	}
}

func TestFilterMoviesHonoursTheYear(t *testing.T) {
	results := []metadata.Movie{
		{TMDBID: 1, Title: "Nosferatu", ReleaseDate: "2024-12-25"},
		{TMDBID: 2, Title: "Nosferatu", OriginalTitle: "Nosferatu, eine Symphonie des Grauens", ReleaseDate: "1922-03-04"},
	}
	got := filterMovies(results, []string{"Nosferatu, eine Symphonie des Grauens"}, func(y int) bool { return y == 1922 })
	if len(got) != 1 || got[0].TMDBID != 2 {
		t.Errorf("got %+v, want the 1922 film", got)
	}
}

func TestMergeTitlesDropsTheTitleAndDuplicates(t *testing.T) {
	got := mergeTitles("Amélie", []string{"Amelie"}, []string{"Amélie", "Le Fabuleux Destin d'Amélie Poulain"})
	if len(got) != 1 || got[0] != "Le Fabuleux Destin d'Amélie Poulain" {
		t.Errorf("got %q", got)
	}
}

// Glazer's Under the Skin is dated 2014 on TMDB; an obscure namesake dated
// 2013 must not win just for matching the catalogued year exactly.
func TestNarrowPrefersTheEvidentFilmOverAnExactYearNamesake(t *testing.T) {
	cands := []metadata.Movie{
		{TMDBID: 698232, Title: "Under the Skin", ReleaseDate: "2013-06-01", VoteCount: 2},
		{TMDBID: 97370, Title: "Under the Skin", ReleaseDate: "2014-03-14", VoteCount: 4500},
	}
	got, note := narrow(cands, 2013)
	if len(got) != 1 || got[0].TMDBID != 97370 || note == "" {
		t.Errorf("got %+v (%q), want 97370 with a note", got, note)
	}
}

func TestNarrowLeavesCloseCallsAmbiguous(t *testing.T) {
	cands := []metadata.Movie{
		{TMDBID: 1, Title: "Climax", ReleaseDate: "2018-01-01", VoteCount: 300},
		{TMDBID: 2, Title: "Climax", ReleaseDate: "2018-06-01", VoteCount: 200},
	}
	if got, _ := narrow(cands, 2018); len(got) != 2 {
		t.Errorf("got %d candidates, want both", len(got))
	}
}
