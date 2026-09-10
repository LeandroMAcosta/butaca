package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/indexer"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// ReleaseID names a release across searches, unlike its position in one result
// list. The infohash is the same on every tracker that carries the torrent; the
// Prowlarr GUID is the fallback for indexers that do not report one.
func ReleaseID(r indexer.Release) string {
	switch {
	case r.InfoHash != "":
		return strings.ToLower(r.InfoHash)
	case r.GUID != "":
		return r.GUID
	default:
		return r.DownloadURL
	}
}

// DedupeReleases collapses the same release seen through several indexers,
// keeping the best-seeded copy and counting how many carried it, keyed by
// ReleaseKey.
func DedupeReleases(cands []decide.Candidate) ([]decide.Candidate, map[string]int) {
	best := map[string]int{}
	mirrors := map[string]int{}
	var out []decide.Candidate
	for _, c := range cands {
		key := ReleaseKey(c.Release.Title)
		mirrors[key]++
		idx, seen := best[key]
		if !seen {
			best[key] = len(out)
			out = append(out, c)
			continue
		}
		if c.Release.Seeders > out[idx].Release.Seeders {
			out[idx] = c
		}
	}
	return out, mirrors
}

// ReleaseKey folds a release title to letters and digits, so the same release
// named with different separators on two trackers counts as one.
func ReleaseKey(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// SortByScore puts accepted releases first, best score first within each group.
// It is stable, so equal scores keep the indexer order.
func SortByScore(cands []decide.Candidate) {
	for i := 1; i < len(cands); i++ {
		for j := i; j > 0; j-- {
			a, b := cands[j-1], cands[j]
			better := (b.Accepted() && !a.Accepted()) ||
				(b.Accepted() == a.Accepted() && b.Score > a.Score)
			if !better {
				break
			}
			cands[j-1], cands[j] = cands[j], cands[j-1]
		}
	}
}

// GrabNew searches for query again and downloads the release whose ReleaseID
// is id, cataloguing the film first. It matches against every mirror, not
// only the copy a deduplicated list showed, and it never falls back to
// another release: a caller that confirmed one release must get that one.
func (a *App) GrabNew(ctx context.Context, query, id string, documentary *bool) (*store.Item, decide.Candidate, error) {
	if id == "" {
		return nil, decide.Candidate{}, fmt.Errorf("no release id given")
	}
	cands, err := a.SearchNew(ctx, query)
	if err != nil {
		return nil, decide.Candidate{}, err
	}
	for _, c := range cands {
		if ReleaseID(c.Release) != id {
			continue
		}
		it, err := a.AddFromRelease(ctx, c, query, documentary)
		return it, c, err
	}
	return nil, decide.Candidate{}, fmt.Errorf(
		"release %s is not in the current results for %q; search again", id, query)
}
