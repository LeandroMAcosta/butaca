package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/decide"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func newAddCmd() *cobra.Command {
	var opt app.AddOptions
	var noSearch bool

	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Add a movie and search for it",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			title := strings.Join(args, " ")
			opt.Monitored = true
			return withApp(func(ctx context.Context, a *app.App) error {
				it, err := a.AddMovie(ctx, title, opt)
				if err != nil {
					return err
				}
				lang := it.OriginalLanguage
				if lang == "" {
					lang = "?"
				}
				fmt.Printf("added #%d  %s (%d)  original language: %s\n", it.ID, it.Title, it.Year, lang)
				if noSearch {
					return nil
				}
				return searchAndGrab(ctx, a, it, false)
			})
		},
	}
	cmd.Flags().IntVar(&opt.Year, "year", 0, "release year")
	cmd.Flags().StringVar(&opt.OriginalLanguage, "lang", "", "original language as ISO 639-1 (fr, ja, de) when TMDB is not configured")
	cmd.Flags().StringSliceVar(&opt.AltTitles, "alt-title", nil, "another name the film is released under (repeatable)")
	cmd.Flags().BoolVar(&noSearch, "no-search", false, "add to the catalog without searching")
	return cmd
}

func newSearchCmd() *cobra.Command {
	var explain bool
	var grab bool

	cmd := &cobra.Command{
		Use:   "search <title or id>",
		Short: "Search for releases of something already in the catalog",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				it, err := resolveItem(a, strings.Join(args, " "))
				if err != nil {
					return err
				}
				if explain {
					return explainSearch(ctx, a, it)
				}
				return searchAndGrab(ctx, a, it, !grab)
			})
		},
	}
	cmd.Flags().BoolVar(&explain, "explain", false, "show every release with its score and rejection reasons")
	cmd.Flags().BoolVar(&grab, "grab", false, "grab the winner instead of only reporting it")
	return cmd
}

// searchAndGrab picks the best candidate. dryRun reports without downloading.
func searchAndGrab(ctx context.Context, a *app.App, it *store.Item, dryRun bool) error {
	cands, err := a.SearchItem(ctx, it)
	if err != nil {
		return err
	}
	if len(cands) == 0 {
		fmt.Println("no releases found")
		return nil
	}
	accepted := 0
	for _, c := range cands {
		if c.Accepted() {
			accepted++
		}
	}
	best := decide.Pick(cands)
	fmt.Printf("%d releases, %d passed the rules\n", len(cands), accepted)
	if best == nil {
		fmt.Println("nothing qualified; run with --explain to see why")
		return nil
	}
	fmt.Printf("best: %s\n  %s | %s | %d seeders | score %d\n",
		best.Release.Title, best.Parsed.ScreenSize, humanSize(best.Release.Size), best.Release.Seeders, best.Score)
	if dryRun {
		fmt.Println("(dry run; pass --grab to download)")
		return nil
	}
	if err := a.Grab(ctx, it, *best); err != nil {
		return err
	}
	fmt.Println("sent to qBittorrent")
	return nil
}

func explainSearch(ctx context.Context, a *app.App, it *store.Item) error {
	cands, err := a.SearchItem(ctx, it)
	if err != nil {
		return err
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].Accepted() != cands[j].Accepted() {
			return cands[i].Accepted()
		}
		return cands[i].Score > cands[j].Score
	})
	for _, c := range cands {
		mark := "REJECT"
		if c.Accepted() {
			mark = "ok    "
		}
		fmt.Printf("%s %5d  %s\n", mark, c.Score, truncate(c.Release.Title, 68))
		fmt.Printf("             %s | %s | %d seeders | %s\n",
			orDash(c.Parsed.ScreenSize), humanSize(c.Release.Size), c.Release.Seeders, c.Release.Indexer)
		for _, r := range c.Rejects {
			fmt.Printf("             - %s\n", r)
		}
	}
	return nil
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the catalog",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				items, err := a.Store.ListItems("")
				if err != nil {
					return err
				}
				if len(items) == 0 {
					fmt.Println("catalog is empty")
					return nil
				}
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintln(w, "ID\tTITLE\tYEAR\tLANG\tFILES\tSIZE")
				var total int64
				for _, it := range items {
					total += it.SizeBytes
					fmt.Fprintf(w, "%d\t%s\t%d\t%s\t%d\t%s\n",
						it.ID, truncate(it.Title, 40), it.Year, orDash(it.OriginalLanguage), it.FileCount, humanSize(it.SizeBytes))
				}
				fmt.Fprintf(w, "\t\t\t\t\t%s\n", humanSize(total))
				return w.Flush()
			})
		},
	}
}

func newRemoveCmd() *cobra.Command {
	var keepFiles bool
	cmd := &cobra.Command{
		Use:     "rm <title or id>",
		Aliases: []string{"remove"},
		Short:   "Remove from the catalog, disk and qBittorrent",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return withApp(func(ctx context.Context, a *app.App) error {
				it, err := resolveItem(a, strings.Join(args, " "))
				if err != nil {
					return err
				}
				steps, err := a.RemoveMovie(ctx, it, !keepFiles)
				for _, s := range steps {
					fmt.Println(" ", s)
				}
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&keepFiles, "keep-files", false, "remove from the catalog but leave files and torrent alone")
	return cmd
}

// resolveItem accepts a numeric id or a title substring.
func resolveItem(a *app.App, ref string) (*store.Item, error) {
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return a.Store.GetItem(id)
	}
	found, err := a.Store.FindItems(ref)
	if err != nil {
		return nil, err
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("nothing in the catalog matches %q", ref)
	case 1:
		return found[0], nil
	default:
		var b strings.Builder
		fmt.Fprintf(&b, "%q matches %d entries:", ref, len(found))
		for _, it := range found {
			fmt.Fprintf(&b, "\n  %d  %s (%d)", it.ID, it.Title, it.Year)
		}
		return nil, fmt.Errorf("%s", b.String())
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
