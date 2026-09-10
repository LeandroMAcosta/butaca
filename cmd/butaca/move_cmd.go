package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

func newMoveCmd() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "move <item> movies|documentaries",
		Short: "Move a movie between the movies and documentaries libraries",
		Long: "Renames the movie's folder into the other library and updates the catalog.\n" +
			"A rename keeps the hardlinks to the download and the subtitles intact.",
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				it, err := resolveItem(a, args[0])
				if err != nil {
					return err
				}
				to := args[1]
				if dryRun {
					from, dest, err := a.MovePlan(it, to)
					if err != nil {
						return err
					}
					fmt.Printf("would move %s\n  from %s\n  to   %s\n", it.Title, orDash(from), dest)
					return nil
				}
				dest, err := a.Move(it, to)
				if err != nil {
					return err
				}
				fmt.Printf("moved %s to %s\n", it.Title, dest)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the from and to folders without moving")
	return cmd
}

// documentaryFlag turns --documentary into the three states AddOptions wants:
// not given (nil, let TMDB decide), true or false.
func documentaryFlag(cmd *cobra.Command, value bool) *bool {
	if !cmd.Flags().Changed("documentary") {
		return nil
	}
	return &value
}
