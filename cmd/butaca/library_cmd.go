package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func newScanTracksCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "scan-tracks",
		Short: "Read the audio and subtitle languages of every library file",
		Long: "Probes each file with ffprobe through the sidecar. Items migrated from\n" +
			"another application have no track data until this runs.",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				lines, err := a.ScanTracks(ctx, force)
				for _, l := range lines {
					fmt.Println(" ", l)
				}
				if err == nil && len(lines) == 0 {
					fmt.Println("everything already has track data; pass --force to re-probe")
				}
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "re-probe files that already have track data")
	return cmd
}

func newLanguagesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "languages",
		Short: "Show which audio and subtitle languages the library actually holds",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				items, err := a.Store.ListItems("")
				if err != nil {
					return err
				}
				audio, subs := map[string]int{}, map[string]int{}
				for _, it := range items {
					langs, err := a.Store.LanguagesForItem(it.ID)
					if err != nil {
						return err
					}
					if langs.Empty() {
						continue
					}
					fmt.Printf("%-40s audio: %-22s subs: %s\n",
						truncate(it.Title, 40),
						store.Summary(langs.Audio, 5), store.Summary(langs.Subtitles, 6))
					for _, l := range langs.Audio {
						audio[l]++
					}
					for _, l := range langs.Subtitles {
						subs[l]++
					}
				}
				fmt.Printf("\naudio languages across the library: %s\n", strings.Join(sortedKeys(audio), " "))
				fmt.Printf("subtitle languages: %s\n", strings.Join(sortedKeys(subs), " "))
				return nil
			})
		},
	}
}

func newDiskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disk",
		Short: "Show free space and what the library occupies",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				items, err := a.Store.ListItems("")
				if err != nil {
					return err
				}
				var occupied int64
				for _, it := range items {
					occupied += it.SizeBytes
				}
				for _, p := range []struct{ label, path string }{
					{"movies", a.Cfg.Paths.Movies},
					{"series", a.Cfg.Paths.TV},
					{"downloads", a.Cfg.Paths.Downloads},
				} {
					u, err := library.Usage(p.path)
					if err != nil {
						fmt.Printf("%-10s %s: %v\n", p.label, p.path, err)
						continue
					}
					fmt.Printf("%-10s %-38s %s free of %s (%.0f%% used)\n",
						p.label, p.path, library.HumanSize(u.Free), library.HumanSize(u.Total), u.UsedPercent())
				}
				fmt.Printf("\nlibrary occupies %s across %d items\n", library.HumanSize(occupied), len(items))
				return nil
			})
		},
	}
}

func sortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
