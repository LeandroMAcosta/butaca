package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

func newSubtitlesCmd() *cobra.Command {
	var sync, all, force bool

	cmd := &cobra.Command{
		Use:   "subtitles [<item>]",
		Short: "Fetch missing subtitles, or sync the ones already there",
		Long: "Without --sync, fetches the configured languages that have no subtitle\n" +
			"yet, for one item or the whole library. New downloads are synced\n" +
			"automatically unless they were made for this exact file (a hash match).\n\n" +
			"With --sync, fixes the subtitles already on disk: a hash-matched download\n" +
			"replaces each one when there is one, otherwise it is synced in place\n" +
			"against an embedded subtitle track or the audio, with ffsubsync and then\n" +
			"alass. The original is kept as <name>.<lang>.srt.orig, and subtitles\n" +
			"synced before are skipped unless --force is given.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			ref := strings.Join(args, " ")
			if sync && ref == "" && !all {
				return errors.New("pass an item, or --all for the whole library")
			}
			return withApp(func(ctx context.Context, a *app.App) error {
				if sync {
					return runSync(ctx, a, ref, force)
				}
				var got []string
				var err error
				if ref == "" {
					got, err = a.FillSubtitleGaps(ctx)
				} else {
					it, rerr := resolveItem(a, ref)
					if rerr != nil {
						return rerr
					}
					got, err = a.SubtitlesForItem(ctx, it)
				}
				for _, l := range got {
					fmt.Println(l)
				}
				if len(got) == 0 && err == nil {
					fmt.Println("nothing to fetch")
				}
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&sync, "sync", false, "sync the subtitles already on disk")
	cmd.Flags().BoolVar(&all, "all", false, "with --sync, every item in the library")
	cmd.Flags().BoolVar(&force, "force", false, "with --sync, redo subtitles synced before, from their original")
	return cmd
}

func runSync(ctx context.Context, a *app.App, ref string, force bool) error {
	if ref == "" {
		_, err := a.SyncAllSubtitles(ctx, force, printReport)
		return err
	}
	it, err := resolveItem(a, ref)
	if err != nil {
		return err
	}
	reports, err := a.SyncSubtitles(ctx, it, force)
	for _, r := range reports {
		printReport(r)
	}
	return err
}

func printReport(r app.SyncReport) {
	fmt.Println(r)
	for _, at := range r.Outcome.Attempts {
		fmt.Println("    tried", at)
	}
}
