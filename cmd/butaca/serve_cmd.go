package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/scheduler"
)

func newServeCmd() *cobra.Command {
	ivl := scheduler.DefaultIntervals()
	var verbose bool

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run continuously: import, search and fetch subtitles on a schedule",
		Long: "Runs the background jobs that keep the library current. Each job has its\n" +
			"own ticker, so a slow search never delays an import. Set an interval to 0\n" +
			"to disable that job.",
		RunE: func(_ *cobra.Command, _ []string) error {
			level := slog.LevelInfo
			if verbose {
				level = slog.LevelDebug
			}
			log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

			return withApp(func(ctx context.Context, a *app.App) error {
				h := a.Health(ctx)
				if h.Prowlarr != "ok" {
					log.Warn("prowlarr unavailable; searches will fail", "err", h.Prowlarr)
				}
				if !h.Hardlinkable {
					log.Warn("imports will fail", "reason", h.HardlinkNote)
				}
				return scheduler.New(a, log, ivl).Run(ctx)
			})
		},
	}
	cmd.Flags().DurationVar(&ivl.Import, "import-interval", time.Minute, "how often to import finished downloads (0 disables)")
	cmd.Flags().DurationVar(&ivl.Search, "search-interval", 6*time.Hour, "how often to search for missing items (0 disables)")
	cmd.Flags().DurationVar(&ivl.Subtitles, "subtitle-interval", 12*time.Hour, "how often to fill subtitle gaps (0 disables)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "debug logging")
	return cmd
}
