package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

func newRemovalsCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "removals",
		Short: "Show what has been deleted, and when",
		Long: "Deletions are recorded before they happen, so the trail survives the\n" +
			"catalog row. Use this when something has gone missing.",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				events, err := a.RemovalHistory(limit)
				if err != nil {
					return err
				}
				if len(events) == 0 {
					fmt.Println("nothing has been deleted through butaca")
					return nil
				}
				for _, e := range events {
					fmt.Printf("%s  %-17s %s\n", e.At, e.Event, e.Detail)
				}
				return nil
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "how many events to show")
	return cmd
}
