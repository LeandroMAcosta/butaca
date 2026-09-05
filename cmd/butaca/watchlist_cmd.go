package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func newWatchlistCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "watchlist",
		Aliases: []string{"wl"},
		Short:   "Show the films queued for some day",
		Long: "Watchlist entries are catalogued but never searched. Promote one with\n" +
			"`butaca watch <title>` when you actually want it downloaded.",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				items, err := a.Store.ListByState(store.StateWatchlist)
				if err != nil {
					return err
				}
				if len(items) == 0 {
					fmt.Println("the watchlist is empty")
					return nil
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tTITLE\tYEAR\tLANG\tFROM")
				for _, it := range items {
					fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%s\n",
						it.ID, truncate(it.Title, 44), it.Year, orDash(it.OriginalLanguage), orDash(it.Source))
				}
				return w.Flush()
			})
		},
	}
	return cmd
}

func newWatchCmd() *cobra.Command {
	var search bool
	cmd := &cobra.Command{
		Use:   "watch <title or id>",
		Short: "Promote a watchlist entry to be searched and downloaded",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				it, err := resolveItem(a, strings.Join(args, " "))
				if err != nil {
					return err
				}
				if err := a.Store.SetState(it.ID, store.StateMonitored); err != nil {
					return err
				}
				fmt.Printf("%s is now monitored\n", it.Title)
				if !search {
					fmt.Println("(pass --search to look for it now)")
					return nil
				}
				it.State = store.StateMonitored
				lines, err := a.SearchMissingFor(ctx, it)
				for _, l := range lines {
					fmt.Println(" ", l)
				}
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&search, "search", false, "search for it immediately")
	return cmd
}

func newUnwatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unwatch <title or id>",
		Short: "Move an item back to the watchlist, so it stops being searched",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				it, err := resolveItem(a, strings.Join(args, " "))
				if err != nil {
					return err
				}
				if err := a.Store.SetState(it.ID, store.StateWatchlist); err != nil {
					return err
				}
				fmt.Printf("%s moved to the watchlist; it will not be searched\n", it.Title)
				if it.FileCount > 0 {
					fmt.Printf("its %s on disk are untouched\n", library.HumanSize(it.SizeBytes))
				}
				return nil
			})
		},
	}
}
