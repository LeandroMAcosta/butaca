package tui

import "github.com/LeandroMAcosta/butaca/internal/metadata"

func metadataMovie(title string, year int, lang string, vote float64) metadata.Movie {
	return metadata.Movie{
		Title: title, OriginalLanguage: lang, VoteAverage: vote,
		ReleaseDate: itoa(year) + "-01-01",
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
