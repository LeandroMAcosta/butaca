// Matching helpers: how a release name is compared against a catalog entry.
package decide

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var stripAccents = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// normalizeTitle folds case, accents and punctuation so "Amélie" matches
// "Amelie" and "Christiane F." matches "Christiane F".
func normalizeTitle(s string) string {
	folded, _, err := transform.String(stripAccents, s)
	if err != nil {
		folded = s
	}
	var b strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(folded) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// titlesMatch accepts a release whose parsed title equals one of the item's
// known titles, or begins with one (release names often append an edition or
// the original-language title). Plain substring containment is deliberately NOT
// enough: "Dawn of the Deep Soul" contains "Soul" but is a different film.
func titlesMatch(parsed string, want []string) bool {
	p := normalizeTitle(parsed)
	if p == "" {
		return false
	}
	for _, w := range want {
		n := normalizeTitle(w)
		if n == "" {
			continue
		}
		if p == n {
			return true
		}
		if strings.HasPrefix(p, n+" ") || strings.HasPrefix(n, p+" ") {
			return true
		}
	}
	return false
}

// iso639 maps the ISO codes TMDB returns to the English names guessit emits.
var iso639 = map[string]string{
	"en": "english", "fr": "french", "de": "german", "ja": "japanese",
	"es": "spanish", "it": "italian", "pt": "portuguese", "ru": "russian",
	"ko": "korean", "zh": "chinese", "nl": "dutch", "sv": "swedish",
	"da": "danish", "no": "norwegian", "fi": "finnish", "pl": "polish",
	"tr": "turkish", "ar": "arabic", "hi": "hindi", "cs": "czech",
	"el": "greek", "he": "hebrew", "hu": "hungarian", "ro": "romanian",
	"th": "thai", "uk": "ukrainian", "vi": "vietnamese", "fa": "persian",
}

// isUnknownLanguage reports codes that assert nothing about the audio track.
func isUnknownLanguage(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "mul", "und", "mis", "zxx", "multi":
		return true
	}
	return false
}

func languageName(code string) string {
	if n, ok := iso639[strings.ToLower(code)]; ok {
		return n
	}
	return code
}

// languageMatches compares guessit's English name against a TMDB ISO code,
// tolerating either side already being in the other form.
func languageMatches(guessed, want string) bool {
	g := strings.ToLower(strings.TrimSpace(guessed))
	w := strings.ToLower(strings.TrimSpace(want))
	return g == w || g == languageName(w) || languageName(g) == w
}

// sourceRank matches guessit's source names ("Blu-ray", "Ultra HD Blu-ray",
// "Web", "HDTV") against the configured preference order, loosely enough that
// "bluray" in the config matches "Blu-ray" from guessit.
func sourceRank(order []string, source string) int {
	if source == "" {
		return -1
	}
	norm := func(s string) string {
		return strings.NewReplacer("-", "", " ", "", "_", "").Replace(strings.ToLower(s))
	}
	got := norm(source)
	for i, want := range order {
		w := norm(want)
		if w == "" {
			continue
		}
		if got == w || strings.Contains(got, w) {
			return i
		}
	}
	return -1
}

func indexOfFold(list []string, want string) int {
	if want == "" {
		return -1
	}
	for i, v := range list {
		if strings.EqualFold(v, want) {
			return i
		}
	}
	return -1
}

// containsToken avoids the classic false positive where "TS" matches inside
// "GUARDIANS" -- the pattern must stand alone between separators.
func containsToken(haystack, token string) bool {
	isSep := func(r byte) bool {
		return !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9')
	}
	for i := 0; i+len(token) <= len(haystack); i++ {
		if haystack[i:i+len(token)] != token {
			continue
		}
		if i > 0 && !isSep(haystack[i-1]) {
			continue
		}
		if end := i + len(token); end < len(haystack) && !isSep(haystack[end]) {
			continue
		}
		return true
	}
	return false
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// ParseSize reads "4.16GB", "700 MB", "1024" into bytes.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	for _, u := range []struct {
		suffix string
		factor int64
	}{
		{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10},
		{"T", 1 << 40}, {"G", 1 << 30}, {"M", 1 << 20}, {"K", 1 << 10},
	} {
		if strings.HasSuffix(s, u.suffix) {
			mult = u.factor
			s = strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			break
		}
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%g", &f); err != nil {
		return 0, fmt.Errorf("bad size %q", s)
	}
	return int64(f * float64(mult)), nil
}

// SameTitle reports whether two titles are equal once case, accents and
// punctuation are folded away: "Amelie" and "Amélie" are the same film.
func SameTitle(a, b string) bool {
	n := normalizeTitle(a)
	return n != "" && n == normalizeTitle(b)
}
