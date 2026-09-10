package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

func newImportCmd() *cobra.Command {
	var watch bool
	var interval time.Duration

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import finished downloads into the library",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				for {
					lines, err := a.ImportReady(ctx)
					if err != nil {
						return err
					}
					for _, l := range lines {
						fmt.Println(" ", l)
					}
					if !watch {
						if len(lines) == 0 {
							fmt.Println("nothing ready to import")
						}
						return nil
					}
					select {
					case <-ctx.Done():
						return nil
					case <-time.After(interval):
					}
				}
			})
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "keep polling until interrupted")
	cmd.Flags().DurationVar(&interval, "interval", 30*time.Second, "poll interval when watching")
	return cmd
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show dependency health and the download queue",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				h := a.Health(ctx)

				fmt.Println("services")
				fmt.Printf("  prowlarr     %s\n", h.Prowlarr)
				for _, i := range h.Indexers {
					fmt.Printf("                 - %s\n", i.Name)
				}
				fmt.Printf("  qbittorrent  %s\n", h.QBittorrent)
				fmt.Printf("  butaca-parse %s\n", h.Parse)

				fmt.Println("\npaths")
				fmt.Printf("  movies       %s\n", a.Cfg.Paths.Movies)
				if d := a.Cfg.Paths.Documentaries; d != "" {
					fmt.Printf("  documentaries %s\n", d)
				}
				fmt.Printf("  tv           %s\n", a.Cfg.Paths.TV)
				fmt.Printf("  downloads    %s\n", a.Cfg.Paths.Downloads)
				if h.Hardlinkable {
					fmt.Println("  hardlinks    ok (downloads and movies share a filesystem)")
				} else {
					fmt.Printf("  hardlinks    UNAVAILABLE: %s\n", h.HardlinkNote)
				}

				queue, err := a.Store.PendingQueue()
				if err != nil {
					return err
				}
				fmt.Printf("\nqueue (%d)\n", len(queue))
				for _, q := range queue {
					fmt.Printf("  %-10s %5.1f%%  %s\n", q.State, q.Progress*100, truncate(q.ReleaseTitle, 56))
				}

				if problems := a.Cfg.Validate(); len(problems) > 0 {
					fmt.Println("\nconfig problems")
					for _, p := range problems {
						fmt.Println("  -", p)
					}
				}
				return nil
			})
		},
	}
}
