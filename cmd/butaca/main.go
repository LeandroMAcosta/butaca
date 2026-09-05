// Command butaca is a unified movie, series and subtitle server.
//
// It replaces Radarr, Sonarr and Bazarr with one process, and keeps Prowlarr
// (indexer aggregation) and qBittorrent (downloading) as external services.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/config"
)

var (
	cfgPath string
	cfg     *config.Config
)

func main() {
	root := &cobra.Command{
		Use:   "butaca",
		Short: "Unified movie, series and subtitle server",
		Long: "butaca keeps one catalog for movies and series, decides which release to\n" +
			"grab, imports it with a hardlink and fetches subtitles.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Name() == "help" {
				return nil
			}
			var err error
			if cfg, err = config.Load(cfgPath); err != nil {
				return err
			}
			return maybeRunSetup(cmd.Name())
		},
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "", "config file (default ~/.config/butaca/config.yaml)")

	root.AddCommand(
		newConfigCmd(),
		newAddCmd(),
		newSearchCmd(),
		newListCmd(),
		newRemoveCmd(),
		newImportCmd(),
		newStatusCmd(),
		newMigrateCmd(),
		newOrphansCmd(),
		newSetupCmd(),
		newServeCmd(),
		newMCPCmd(),
		newTUICmd(),
		newScanTracksCmd(),
		newLanguagesCmd(),
		newDiskCmd(),
		newLetterboxdCmd(),
		newWatchlistCmd(),
		newWatchCmd(),
		newUnwatchCmd(),
		newProfileCmd(),
		newRecommendCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// withApp opens the store and clients, and guarantees they are closed.
func withApp(fn func(context.Context, *app.App) error) error {
	a, err := app.New(cfg)
	if err != nil {
		return err
	}
	defer a.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return fn(ctx, a)
}
