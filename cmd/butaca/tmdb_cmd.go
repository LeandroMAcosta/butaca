package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

func newTMDBMatchCmd() *cobra.Command {
	var apply bool
	cmd := &cobra.Command{
		Use:   "tmdb-match",
		Short: "Find the TMDB id of movies catalogued without one",
		Long: "Looks every movie with no TMDB id up by title and year, and stores the id,\n" +
			"original language (when none was set) and alternative titles. Titles and\n" +
			"paths do not change, and items that already have an id are skipped.\n" +
			"Recommendations only work from movies with a TMDB id.\n\n" +
			"A film whose TMDB genre puts it in the other library (documentaries or\n" +
			"movies) is reported; --apply moves it there.",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				got, err := a.MatchTMDB(ctx, apply)
				if err != nil {
					return err
				}
				if len(got) == 0 {
					fmt.Println("every movie already has a TMDB id")
				}
				for _, m := range got {
					fmt.Println(m)
				}
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "move films into the library their TMDB genre says")
	return cmd
}
