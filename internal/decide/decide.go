// Package decide picks which release to grab.
//
// This is the part that exists because Radarr's rigid quality profile could not
// express what was actually wanted. Its language filter is a fixed constant, so
// a profile demanding English rejects every film whose original language is not
// English -- French, Japanese and German titles all had to be grabbed by hand.
// Here the language rule is relative to the item's own original language.
package decide

import (
	"fmt"
	"math"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/indexer"
	"github.com/LeandroMAcosta/butaca/internal/parse"
)

type LanguageMode int

const (
	// LangOriginal accepts the item's own original language and rejects a
	// release that positively declares a different audio language (a dub).
	LangOriginal LanguageMode = iota
	// LangPrefer scores the preferred language higher but rejects nothing.
	LangPrefer
	// LangAny ignores language entirely.
	LangAny
)

func ParseLanguageMode(s string) (LanguageMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "original":
		return LangOriginal, nil
	case "prefer":
		return LangPrefer, nil
	case "any":
		return LangAny, nil
	default:
		return LangOriginal, fmt.Errorf("unknown language_mode %q", s)
	}
}

const (
	// langMatchBonus rewards a release that declares the expected audio
	// language. Deliberately small: it breaks ties, it does not decide.
	langMatchBonus = 15
	seederWeight   = 12
)

type Rules struct {
	MinSeeders  int
	Resolutions []string // ordered by preference, best first
	// Sources ranks the origin of the transfer, best first. A 1080p HDTV rip
	// and a 1080p Blu-ray rip are the same resolution and very different
	// pictures, so resolution alone cannot express the preference.
	Sources        []string
	MinSize        int64
	MaxSize        int64
	PreferGroups   []string
	RejectPatterns []string
	LanguageMode   LanguageMode
	PreferLanguage string // English name or ISO code, used by LangPrefer

	// ScoreFunc adds a caller-defined bonus, for judgements a profile cannot
	// express -- preferring lossless audio on a concert film, for instance.
	ScoreFunc func(Candidate) int
}

// Item is the subset of a catalog entry the engine needs.
type Item struct {
	// Titles holds every name the item is released under: the common title,
	// TMDB's original_title, and any alternative titles. Matching against all
	// of them is what lets "Le Fabuleux Destin d'Amelie Poulain" resolve to
	// Amelie without also letting "Dawn of the Deep Soul" resolve to Soul.
	Titles           []string
	Year             int
	OriginalLanguage string // ISO 639-1 from TMDB, e.g. "fr"
}

func (i Item) Name() string {
	if len(i.Titles) == 0 {
		return ""
	}
	return i.Titles[0]
}

type Candidate struct {
	Release indexer.Release
	Parsed  parse.Result
	Score   int
	Rejects []string
}

func (c Candidate) Accepted() bool { return len(c.Rejects) == 0 }

func (c *Candidate) reject(format string, args ...any) {
	c.Rejects = append(c.Rejects, fmt.Sprintf(format, args...))
}

// Evaluate scores every release against the rules. Nothing is dropped: rejected
// candidates keep their reasons so `--explain` can show why.
func Evaluate(releases []indexer.Release, parsed []parse.Result, item Item, rules Rules) []Candidate {
	out := make([]Candidate, 0, len(releases))
	for i, r := range releases {
		c := Candidate{Release: r}
		if i < len(parsed) {
			c.Parsed = parsed[i]
		}
		score(&c, item, rules)
		out = append(out, c)
	}
	return out
}

func score(c *Candidate, item Item, rules Rules) {
	r, p := c.Release, c.Parsed

	// Identity first. Prowlarr fans the query across trackers and several of
	// them answer with their whole popular list, so most results are unrelated.
	if !titlesMatch(p.Title, item.Titles) {
		c.reject("title %q does not match %q", p.Title, item.Name())
	}
	if item.Year > 0 && p.Year > 0 && abs(p.Year-item.Year) > 1 {
		c.reject("year %d does not match %d", p.Year, item.Year)
	}

	upper := strings.ToUpper(r.Title)
	for _, pat := range rules.RejectPatterns {
		if pat == "" {
			continue
		}
		if containsToken(upper, strings.ToUpper(pat)) {
			c.reject("matches reject pattern %s", pat)
		}
	}

	if rules.MinSeeders > 0 && r.Seeders < rules.MinSeeders {
		c.reject("only %d seeders, want %d", r.Seeders, rules.MinSeeders)
	}
	if rules.MinSize > 0 && r.Size > 0 && r.Size < rules.MinSize {
		c.reject("%s below minimum %s", humanSize(r.Size), humanSize(rules.MinSize))
	}
	if rules.MaxSize > 0 && r.Size > rules.MaxSize {
		c.reject("%s above maximum %s", humanSize(r.Size), humanSize(rules.MaxSize))
	}

	resIdx := -1
	if len(rules.Resolutions) > 0 {
		resIdx = indexOfFold(rules.Resolutions, p.ScreenSize)
		if resIdx < 0 {
			got := p.ScreenSize
			if got == "" {
				got = "unknown"
			}
			c.reject("resolution %s not in %s", got, strings.Join(rules.Resolutions, ", "))
		}
	}

	// Language. p.Language is the audio language and is empty for most release
	// names; absence must stay neutral or nearly everything would be rejected.
	// p.SubtitleLanguage is deliberately ignored here: "English subs" says
	// nothing about the audio.
	langBonus := 0
	lang := p.Language
	if isUnknownLanguage(lang) {
		// "mul" (multiple) and "und" (undetermined) carry no information: a
		// multi-audio release normally includes the original track, so treating
		// them as a mismatch would reject perfectly good releases.
		lang = ""
	}
	switch rules.LanguageMode {
	case LangOriginal:
		if lang != "" && item.OriginalLanguage != "" {
			if languageMatches(lang, item.OriginalLanguage) {
				langBonus = langMatchBonus
			} else {
				c.reject("audio is %s, want the original %s", languageName(lang), languageName(item.OriginalLanguage))
			}
		}
	case LangPrefer:
		if lang != "" && rules.PreferLanguage != "" && languageMatches(lang, rules.PreferLanguage) {
			langBonus = langMatchBonus
		}
	case LangAny:
	}

	// Scoring. Resolution preference dominates, then swarm health, then group.
	s := 100 + langBonus
	if resIdx >= 0 {
		s += (len(rules.Resolutions) - resIdx) * 30
	}
	if r.Seeders > 0 {
		// Diminishing returns: 250 seeders is not 10x better than 25, but the
		// gap still has to outweigh a language match, which is only a
		// tiebreaker -- a wrong language is rejected outright above, so merely
		// stating the right one must not beat a far healthier swarm.
		s += int(math.Round(math.Log2(float64(r.Seeders)+1) * seederWeight))
	}
	if idx := sourceRank(rules.Sources, p.Source); idx >= 0 {
		s += (len(rules.Sources) - idx) * 10
	}
	if p.ReleaseGroup != "" && indexOfFold(rules.PreferGroups, p.ReleaseGroup) >= 0 {
		s += 25
	}
	if rules.ScoreFunc != nil {
		s += rules.ScoreFunc(*c)
	}
	c.Score = s
}

// Pick returns the highest-scoring accepted candidate, or nil when none qualify.
func Pick(cands []Candidate) *Candidate {
	var best *Candidate
	for i := range cands {
		if !cands[i].Accepted() {
			continue
		}
		if best == nil || cands[i].Score > best.Score {
			best = &cands[i]
		}
	}
	return best
}
