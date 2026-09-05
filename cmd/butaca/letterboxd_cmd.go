package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func newLetterboxdCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "letterboxd",
		Short: "Import a Letterboxd watchlist and lists",
		Long: "Letterboxd has no public API, so this reads the member's public pages.\n" +
			"Imports land on the watchlist and are never searched until promoted.",
	}

	var opt app.LetterboxdOptions
	var profileName string

	imp := &cobra.Command{
		Use:   "import",
		Short: "Pull the watchlist (and named lists) into the catalog",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				if err := resolveLetterboxdUser(a, &opt, profileName); err != nil {
					return err
				}
				fmt.Printf("reading letterboxd.com/%s\n", opt.User)

				rep, err := a.ImportLetterboxd(ctx, opt)
				if err != nil {
					return err
				}
				verb := "imported"
				if opt.DryRun {
					verb = "would import"
				}
				fmt.Printf("\n%s %d\n", verb, len(rep.Imported))
				for _, l := range rep.Imported {
					fmt.Println("  +", l)
				}
				if len(rep.Skipped) > 0 {
					fmt.Printf("\nalready known (%d)\n", len(rep.Skipped))
					for _, l := range rep.Skipped {
						fmt.Println("  =", l)
					}
				}
				if len(rep.Failed) > 0 {
					fmt.Printf("\nfailed (%d)\n", len(rep.Failed))
					for _, l := range rep.Failed {
						fmt.Println("  !", l)
					}
				}
				if !opt.Monitor && len(rep.Imported) > 0 && !opt.DryRun {
					fmt.Println("\nThese are on the watchlist and will not be searched.")
					fmt.Println("Promote one with: butaca watch <title> --monitor")
				}
				return nil
			})
		},
	}
	imp.Flags().StringVar(&opt.User, "user", "", "Letterboxd username")
	imp.Flags().StringVar(&profileName, "profile", "", "attach the imports to this profile, and use its Letterboxd username")
	imp.Flags().StringSliceVar(&opt.Lists, "list", nil, "also import this list by slug (repeatable)")
	imp.Flags().BoolVar(&opt.Monitor, "monitor", false, "search the imports immediately instead of leaving them on the watchlist")
	imp.Flags().BoolVar(&opt.DryRun, "dry-run", false, "show what would be imported without writing")

	var listUser, listProfile string
	lists := &cobra.Command{
		Use:   "lists",
		Short: "Show the member's public lists",
		RunE: func(_ *cobra.Command, _ []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				o := app.LetterboxdOptions{User: listUser}
				if err := resolveLetterboxdUser(a, &o, listProfile); err != nil {
					return err
				}
				found, err := a.LetterboxdLists(ctx, o.User)
				if err != nil {
					return err
				}
				if len(found) == 0 {
					fmt.Println("no public lists found")
					return nil
				}
				for _, l := range found {
					fmt.Printf("  %-40s --list %s\n", l.Name, l.Slug)
				}
				return nil
			})
		},
	}
	lists.Flags().StringVar(&listUser, "user", "", "Letterboxd username")
	lists.Flags().StringVar(&listProfile, "profile", "", "take the username from this profile")

	cmd.AddCommand(imp, lists)
	return cmd
}

// resolveLetterboxdUser fills in the username and profile id, preferring an
// explicit flag over what the profile carries.
func resolveLetterboxdUser(a *app.App, opt *app.LetterboxdOptions, profileName string) error {
	var prof *store.Profile
	var err error
	if profileName != "" {
		prof, err = a.Store.FindProfile(profileName)
		if err != nil {
			return fmt.Errorf("profile %q: %w", profileName, err)
		}
	} else if prof, err = a.Store.DefaultProfile(); err != nil {
		return err
	}
	if prof != nil {
		opt.ProfileID = prof.ID
		if opt.User == "" {
			opt.User = prof.LetterboxdUser
		}
	}
	if opt.User == "" {
		return fmt.Errorf("no Letterboxd username: pass --user, or set one on a profile with `butaca profile set`")
	}
	return nil
}
