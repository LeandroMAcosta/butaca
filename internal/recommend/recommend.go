// Package recommend suggests films from what the library already contains.
//
// The seed is deliberately the local catalog rather than Letterboxd ratings:
// a Letterboxd account with no rated films would produce nothing, whereas a
// library always has something to reason from.
package recommend

import (
	"sort"

	"github.com/LeandroMAcosta/butaca/internal/metadata"
)

// Suggestion is a candidate film with the evidence behind it.
type Suggestion struct {
	Movie metadata.Movie
	// Seeds are the titles that led here; more seeds is stronger evidence.
	Seeds []string
	Score float64
}

// Options tunes the ranking.
type Options struct {
	// PreferLanguages boosts films in languages the profile cares about.
	PreferLanguages []string
	// MinVotes drops obscure entries that TMDB scores on a handful of votes.
	MinVotes int
	Limit    int
}

func DefaultOptions() Options {
	return Options{MinVotes: 200, Limit: 40}
}

// Rank aggregates related films per seed into a single ordered list.
//
// related maps a seed title to the films TMDB associated with it. exclude holds
// the TMDB ids already in the catalog, which must never be suggested back.
func Rank(related map[string][]metadata.Movie, exclude map[int64]bool, opt Options) []Suggestion {
	if opt.Limit == 0 {
		opt = DefaultOptions()
	}
	preferred := map[string]bool{}
	for _, l := range opt.PreferLanguages {
		preferred[l] = true
	}

	byID := map[int64]*Suggestion{}
	for seed, movies := range related {
		for _, m := range movies {
			if m.TMDBID == 0 || exclude[m.TMDBID] {
				continue
			}
			if m.VoteCount < opt.MinVotes {
				continue
			}
			s, ok := byID[m.TMDBID]
			if !ok {
				s = &Suggestion{Movie: m}
				byID[m.TMDBID] = s
			}
			s.Seeds = append(s.Seeds, seed)
		}
	}

	out := make([]Suggestion, 0, len(byID))
	for _, s := range byID {
		// Agreement across seeds dominates: a film reached from four of your
		// films is a better bet than one reached from a single popular title.
		s.Score = float64(len(s.Seeds))*10 + s.Movie.VoteAverage
		if preferred[s.Movie.OriginalLanguage] {
			s.Score += 3
		}
		out = append(out, *s)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Movie.VoteCount > out[j].Movie.VoteCount
	})
	for i := range out {
		sort.Strings(out[i].Seeds)
	}
	if len(out) > opt.Limit {
		out = out[:opt.Limit]
	}
	return out
}
