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
				sortByScore(cands)

				accepted := 0
				for _, c := range cands {
					if c.Accepted() {
						accepted++
					}
				}
				fmt.Printf("%d releases, %d passed the rules\n\n", len(cands), accepted)

				shown := 0
				for i, c := range cands {
					if !all && !c.Accepted() {
						continue
					}
					shown++
					if !all && shown > 10 {
						break
					}
					mark := "ok    "
					if !c.Accepted() {
						mark = "REJECT"
					}
					fmt.Printf("%2d. [%s] %5d  %s\n", i+1, mark, c.Score, truncate(c.Release.Title, 62))
					fmt.Printf("            %s · %s · %d seeders · %s\n",
						orDash(c.Parsed.ScreenSize), library.HumanSize(c.Release.Size),
						c.Release.Seeders, c.Release.Indexer)
					for _, r := range c.Rejects {
						fmt.Printf("            - %s\n", r)
					}
				}
				if !all && accepted == 0 {
					fmt.Println("nothing qualified; pass --all to see why each release was rejected")
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
