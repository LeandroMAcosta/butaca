package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func newProfileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage profiles: one person's preferences, watchlist and Letterboxd account",
		Long: "A profile carries release preferences and a Letterboxd username. Items\n" +
			"assigned to it use its rules; anything it leaves unset falls back to the\n" +
			"global configuration.",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Show the profiles",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				profs, err := a.Store.ListProfiles()
				if err != nil {
					return err
				}
				if len(profs) == 0 {
					fmt.Println("no profiles; the global configuration applies to everything")
					fmt.Println("create one with: butaca profile set <name> --language-mode original")
					return nil
				}
				items, _ := a.Store.ListItems("")
				counts := map[int64]int{}
				for _, it := range items {
					counts[it.ProfileID]++
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tNAME\tLETTERBOXD\tAUDIO\tQUALITY\tSUBS\tITEMS")
				for _, p := range profs {
					name := p.Name
					if p.IsDefault {
						name += " (default)"
					}
					mode := p.LanguageMode
					if p.PreferLanguage != "" {
						mode += ":" + p.PreferLanguage
					}
					fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%d\n",
						p.ID, name, orDash(p.LetterboxdUser), mode,
						orDash(strings.Join(p.Resolutions, ",")),
						orDash(strings.Join(p.SubtitleLangs, ",")), counts[p.ID])
				}
				return w.Flush()
			})
		},
	})

	var p store.Profile
	var subs, res, sources string
	set := &cobra.Command{
		Use:   "set <name>",
		Short: "Create or update a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				p.Name = args[0]
				// Merge onto whatever exists, so setting one field does not
				// silently blank the rest.
				if existing, err := a.Store.FindProfile(p.Name); err == nil && existing != nil {
					merge(&p, existing, subs == "", res == "", sources == "")
				}
				p.SubtitleLangs = splitCSV(subs, p.SubtitleLangs)
				p.Resolutions = splitCSV(res, p.Resolutions)
				p.Sources = splitCSV(sources, p.Sources)

				id, err := a.Store.SaveProfile(&p)
				if err != nil {
					return err
				}
				fmt.Printf("profile %q saved as #%d\n", p.Name, id)
				return nil
			})
		},
	}
	set.Flags().StringVar(&p.LetterboxdUser, "letterboxd", "", "Letterboxd username")
	set.Flags().StringVar(&p.LanguageMode, "language-mode", "", "original, prefer or any")
	set.Flags().StringVar(&p.PreferLanguage, "prefer-language", "", "ISO code used when language-mode is prefer")
	set.Flags().StringVar(&res, "resolutions", "", "comma separated, best first, e.g. 1080p,720p")
	set.Flags().StringVar(&sources, "sources", "", "comma separated, best first, e.g. Blu-ray,Web")
	set.Flags().StringVar(&subs, "subtitles", "", "comma separated subtitle languages, e.g. es,en")
	set.Flags().StringVar(&p.MinSize, "min-size", "", "e.g. 500MB")
	set.Flags().StringVar(&p.MaxSize, "max-size", "", "e.g. 8GB")
	set.Flags().IntVar(&p.MinSeeders, "min-seeders", 0, "minimum seeders")
	set.Flags().BoolVar(&p.IsDefault, "default", false, "make this the default profile")
	cmd.AddCommand(set)

	cmd.AddCommand(&cobra.Command{
		Use:   "assign <profile> <item>",
		Short: "Point an item at a profile",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				prof, err := a.Store.FindProfile(args[0])
				if err != nil {
					return fmt.Errorf("profile %q: %w", args[0], err)
				}
				it, err := resolveItem(a, strings.Join(args[1:], " "))
				if err != nil {
					return err
				}
				if err := a.Store.AssignProfile(it.ID, prof.ID); err != nil {
					return err
				}
				fmt.Printf("%s now uses profile %q\n", it.Title, prof.Name)
				return nil
			})
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "rm <name>",
		Short: "Delete a profile; its items fall back to the global rules",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				prof, err := a.Store.FindProfile(args[0])
				if err != nil {
					return err
				}
				if err := a.Store.DeleteProfile(prof.ID); err != nil {
					return err
				}
				fmt.Printf("deleted profile %q\n", prof.Name)
				return nil
			})
		},
	})
	return cmd
}

// merge carries forward fields the caller did not set on this invocation.
func merge(dst, src *store.Profile, keepSubs, keepRes, keepSources bool) {
	if dst.LetterboxdUser == "" {
		dst.LetterboxdUser = src.LetterboxdUser
	}
	if dst.LanguageMode == "" {
		dst.LanguageMode = src.LanguageMode
	}
	if dst.PreferLanguage == "" {
		dst.PreferLanguage = src.PreferLanguage
	}
	if dst.MinSize == "" {
		dst.MinSize = src.MinSize
	}
	if dst.MaxSize == "" {
		dst.MaxSize = src.MaxSize
	}
	if dst.MinSeeders == 0 {
		dst.MinSeeders = src.MinSeeders
	}
	if keepSubs {
		dst.SubtitleLangs = src.SubtitleLangs
	}
	if keepRes {
		dst.Resolutions = src.Resolutions
	}
	if keepSources {
		dst.Sources = src.Sources
	}
	if !dst.IsDefault {
		dst.IsDefault = src.IsDefault
	}
}

func splitCSV(s string, fallback []string) []string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
