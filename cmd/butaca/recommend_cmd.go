package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/recommend"
)

func newRecommendCmd() *cobra.Command {
	opt := recommend.DefaultOptions()
	var add int

	cmd := &cobra.Command{
		Use:     "recommend",
		Aliases: []string{"discover"},
		Short:   "Suggest films based on what your library already holds",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				suggestions, err := a.Recommend(ctx, opt)
				if err != nil {
					return err
				}
				for i, s := range suggestions {
					fmt.Printf("%2d. %-44s %d  %.1f★  %s\n",
						i+1, truncate(s.Movie.Title, 44), s.Movie.Year(),
						s.Movie.VoteAverage, orDash(s.Movie.OriginalLanguage))
					fmt.Printf("     because of: %s\n", strings.Join(s.Seeds, ", "))
				}
				if add > 0 {
					if add > len(suggestions) {
						return fmt.Errorf("there is no suggestion #%d", add)
					}
					s := suggestions[add-1]
					id, err := a.AcceptSuggestion(s, 0)
					if err != nil {
						return err
					}
					fmt.Printf("\nadded %s to the watchlist as #%d\n", s.Movie.Title, id)
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&opt.Limit, "limit", 20, "how many suggestions to show")
	cmd.Flags().IntVar(&opt.MinVotes, "min-votes", 200, "ignore films with fewer TMDB votes")
	cmd.Flags().IntVar(&add, "add", 0, "add suggestion N to the watchlist")
	return cmd
}
