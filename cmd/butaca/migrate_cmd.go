package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/migrate"
)

func newMigrateCmd() *cobra.Command {
	var radarrDB, sonarrDB string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Import an existing Radarr or Sonarr catalog",
		Long: "Rebuilds butaca's catalog from another application's database.\n" +
			"No file is moved, copied or deleted: only the catalog is written, so this\n" +
			"is safe to run while the old stack is still installed.",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				var entries []migrate.Entry

				if radarrDB != "" {
					got, err := migrate.FromRadarr(radarrDB)
					if err != nil {
						return err
					}
					fmt.Printf("radarr: %d movies in %s\n", len(got), radarrDB)
					entries = append(entries, got...)
				}
				if sonarrDB != "" {
					got, err := migrate.FromSonarr(sonarrDB)
					if err != nil {
						fmt.Println("sonarr:", err)
					} else {
						fmt.Printf("sonarr: %d series in %s\n", len(got), sonarrDB)
						entries = append(entries, got...)
					}
				}
				if len(entries) == 0 {
					fmt.Println("nothing to import")
					return nil
				}

				rep, err := a.ImportEntries(entries, dryRun)
				if err != nil {
					return err
				}

				verb := "imported"
				if dryRun {
					verb = "would import"
				}
				fmt.Printf("\n%s %d\n", verb, len(rep.Imported))
				for _, l := range rep.Imported {
					fmt.Println("  +", l)
				}
				if len(rep.Missing) > 0 {
					fmt.Printf("\ncatalogued but not on disk (%d)\n", len(rep.Missing))
					for _, l := range rep.Missing {
						fmt.Println("  !", l)
					}
				}
				if len(rep.Skipped) > 0 {
					fmt.Printf("\nskipped (%d)\n", len(rep.Skipped))
					for _, l := range rep.Skipped {
						fmt.Println("  -", l)
					}
				}
				if dryRun {
					fmt.Println("\ndry run: nothing was written. Re-run without --dry-run to apply.")
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&radarrDB, "radarr", migrate.DefaultRadarrDB(), "path to radarr.db (empty to skip)")
	cmd.Flags().StringVar(&sonarrDB, "sonarr", migrate.DefaultSonarrDB(), "path to sonarr.db (empty to skip)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be imported without writing")
	return cmd
}

func newOrphansCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "orphans",
		Short: "List downloads that no catalog entry points at",
		Long: "Walks the downloads folder and reports anything whose bytes are not\n" +
			"referenced by a catalogued file. Because imports are hardlinks, identity\n" +
			"is the inode, not the path.",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				orphans, err := a.Orphans()
				if err != nil {
					return err
				}
				if len(orphans) == 0 {
					fmt.Println("no orphans:", a.Cfg.Paths.Downloads, "is fully accounted for")
					return nil
				}
				fmt.Printf("%d orphan(s) in %s\n", len(orphans), a.Cfg.Paths.Downloads)
				for _, o := range orphans {
					fmt.Println("  ?", o)
				}
				return nil
			})
		},
	}
}
