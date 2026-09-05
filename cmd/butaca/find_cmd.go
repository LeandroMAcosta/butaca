package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/library"
)

func newFindCmd() *cobra.Command {
	var grab int
	var all bool

	cmd := &cobra.Command{
		Use:   "find <title>",
		Short: "Search for a film that is not in the catalog yet",
		Long: "Searches every tracker through Prowlarr and scores the results. Unlike\n" +
			"`add`, this needs no TMDB key: the release supplies the year, and TMDB\n" +
			"fills in the rest afterwards when it is configured.\n\n" +
			"Adding a year helps: `butaca find \"Amelie 2001\"`.",
		Args: cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			query := strings.Join(args, " ")
			return withApp(func(ctx context.Context, a *app.App) error {
				cands, err := a.SearchNew(ctx, query)
				if err != nil {
					return err
				}
				if len(cands) == 0 {
					fmt.Printf("nothing found for %q\n", query)
					return nil
				}
				// The same release comes back from several trackers, so merge
				// them before printing anything.
				total := len(cands)
				cands, mirrors := dedupeByTitle(cands)
				sortByScore(cands)

				accepted := 0
				for _, c := range cands {
					if c.Accepted() {
						accepted++
					}
				}
				fmt.Printf("%d releases", total)
				if total != len(cands) {
					fmt.Printf(", %d after merging duplicates", len(cands))
				}
				fmt.Printf(", %d passed your rules\n\n", accepted)

				fmt.Printf("  #  %5s  %-46s %-7s %9s %7s\n", "SCORE", "RELEASE", "QUALITY", "SIZE", "SEEDS")
				shown := 0
				for i, c := range cands {
					if !all && !c.Accepted() {
						continue
					}
					if !all && shown >= 10 {
						fmt.Printf("     … and %d more, pass --all to see them\n", accepted-shown)
						break
					}
					shown++
					mark := " "
					if !c.Accepted() {
						mark = "x"
					}
					mirror := ""
					if n := mirrors[normalizeTitle(c.Release.Title)]; n > 1 {
						mirror = fmt.Sprintf(" x%d", n)
					}
					fmt.Printf("%s%2d  %5d  %-46s %-7s %9s %6d%s\n",
						mark, i+1, c.Score, truncate(c.Release.Title, 46),
						orDash(c.Parsed.ScreenSize), library.HumanSize(c.Release.Size),
						c.Release.Seeders, mirror)
					for _, r := range c.Rejects {
						fmt.Printf("             - %s\n", r)
					}
				}
				if accepted == 0 {
					fmt.Println("\nnothing qualified; pass --all to see why each release was rejected")
				}

				if grab > 0 {
					if grab > len(cands) {
						return fmt.Errorf("there is no release #%d", grab)
					}
					c := cands[grab-1]
					it, err := a.AddFromRelease(ctx, c, query)
					if err != nil {
						return err
					}
					fmt.Printf("\nadded %s (%d) and sent %s to qBittorrent\n", it.Title, it.Year, c.Release.Title)
				} else {
					fmt.Println("\npass --grab N to add it and start the download")
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&grab, "grab", 0, "add release N to the catalog and download it")
	cmd.Flags().BoolVar(&all, "all", false, "show rejected releases and why")
	return cmd
}

// dedupeByTitle collapses the same release seen through several indexers,
// keeping the best-seeded copy and counting how many carried it.
func dedupeByTitle(cands []decide.Candidate) ([]decide.Candidate, map[string]int) {
	best := map[string]int{}
	mirrors := map[string]int{}
	var out []decide.Candidate
	for _, c := range cands {
		key := normalizeTitle(c.Release.Title)
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

func normalizeTitle(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sortByScore(cands []decide.Candidate) {
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
