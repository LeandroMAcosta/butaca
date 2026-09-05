package app

import "testing"

func TestLooksLikeLanguage(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"es", true}, {"eng", true}, {"fr", true},
		// Release-name debris that a naive suffix split produces.
		{"BZ]", false}, {"bz]", false}, {"mx", true},
		{"", false}, {"a", false}, {"spanish", false}, {"5", false}, {"1080p", false},
	} {
		if got := looksLikeLanguage(tc.in); got != tc.want {
			t.Errorf("looksLikeLanguage(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestLangOfExtractsSuffix(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/x/Movie (2001).es.srt", "es"},
		{"Movie.eng.srt", "eng"},
		// No suffix means no language claim, which must not be mistaken for one.
		{"Movie.srt", ""},
	} {
		if got := langOf(tc.in); got != tc.want {
			t.Errorf("langOf(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
