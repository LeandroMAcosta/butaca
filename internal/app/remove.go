package app

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// RemoveOptions controls how much of an item is torn down.
type RemoveOptions struct {
	// KeepFiles leaves the library folder and the torrent alone.
	KeepFiles bool
	// KeepTorrent stops seeding from being interrupted, at the cost of not
	// reclaiming any space: the download and the library entry are hardlinks to
	// the same bytes, so both names must go before the disk frees anything.
	KeepTorrent bool
}

// RemoveStep is one thing that happened, so every caller renders the same
// account of the teardown instead of inventing its own.
type RemoveStep struct {
	What string
	Done bool
	Err  error
}

func (s RemoveStep) String() string {
	switch {
	case s.Err != nil:
		return s.What + ": " + s.Err.Error()
	case s.Done:
		return s.What
	default:
		return s.What + " (skipped)"
	}
}

// Remove tears an item down across the catalog, the disk and the download
// client. It never stops at the first failure: a missing folder must not
// prevent the torrent from being removed.
func (a *App) Remove(ctx context.Context, it *store.Item, opt RemoveOptions) ([]RemoveStep, error) {
	var steps []RemoveStep

	// Record the intent before acting. history.item_id is ON DELETE SET NULL,
	// so this row survives the deletion and leaves an audit trail: without it a
	// removal is invisible afterwards, and "where did my film go?" has no answer.
	_ = a.Store.Log(it.ID, "remove_requested", fmt.Sprintf(
		"%s (%d) path=%s keepFiles=%v keepTorrent=%v",
		it.Title, it.Year, it.Path, opt.KeepFiles, opt.KeepTorrent))

	if !opt.KeepFiles && it.Path != "" {
		if _, err := os.Stat(it.Path); err == nil {
			err := library.RemoveFolder(it.Path)
			steps = append(steps, RemoveStep{What: "deleted " + it.Path, Done: err == nil, Err: err})
		} else {
			steps = append(steps, RemoveStep{What: "no folder at " + it.Path})
		}
	}

	if !opt.KeepFiles && !opt.KeepTorrent {
		hashes, err := a.itemTorrents(ctx, it)
		if err != nil {
			steps = append(steps, RemoveStep{What: "look up torrents", Err: err})
		} else if len(hashes) == 0 {
			steps = append(steps, RemoveStep{What: "no torrent in qBittorrent for this item"})
		}
		for _, h := range hashes {
			err := a.QBit.Delete(ctx, h, true)
			steps = append(steps, RemoveStep{
				What: "removed torrent " + shortHash(h), Done: err == nil, Err: err})
		}
	}

	if err := a.Store.DeleteItem(it.ID); err != nil {
		steps = append(steps, RemoveStep{What: "remove from catalog", Err: err})
		return steps, fmt.Errorf("remove %s from catalog: %w", it.Title, err)
	}
	steps = append(steps, RemoveStep{What: "removed from catalog", Done: true})
	_ = a.Store.Log(0, "removed", it.Title)
	return steps, nil
}

// RemovePlan describes what Remove would do, for a confirmation prompt.
func (a *App) RemovePlan(ctx context.Context, it *store.Item, opt RemoveOptions) []string {
	var plan []string
	if !opt.KeepFiles && it.Path != "" {
		if _, err := os.Stat(it.Path); err == nil {
			plan = append(plan, fmt.Sprintf("delete %s (%s)", it.Path, library.HumanSize(it.SizeBytes)))
		}
	}
	if !opt.KeepFiles && !opt.KeepTorrent {
		// A prompt must not hang on an unresponsive download client.
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		hashes, err := a.itemTorrents(ctx, it)
		if err != nil {
			plan = append(plan, "delete its torrent and files, if qBittorrent has one (could not check: "+err.Error()+")")
		}
		for _, h := range hashes {
			plan = append(plan, "delete torrent "+shortHash(h)+" and its files")
		}
	}
	plan = append(plan, fmt.Sprintf("remove %q from the catalog", it.Title))
	return plan
}

// itemTorrents returns the hashes of the item's torrents that qBittorrent
// still holds. Imported ones are included on purpose: the download is a second
// hardlink to the library file, and while it exists no byte is freed.
func (a *App) itemTorrents(ctx context.Context, it *store.Item) ([]string, error) {
	entries, err := a.Store.QueueForItem(it.ID)
	if err != nil || len(entries) == 0 {
		return nil, err
	}
	held, err := a.QBit.List(ctx)
	if err != nil {
		return nil, err
	}
	have := make(map[string]bool, len(held))
	for _, t := range held {
		have[strings.ToLower(t.Hash)] = true
	}
	var out []string
	for _, q := range entries {
		h := strings.ToLower(q.InfoHash)
		if have[h] {
			out = append(out, h)
			have[h] = false
		}
	}
	return out, nil
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// RemovalHistory returns the audit trail of deletions, newest first. It answers
// "what happened to my film?" after the catalog row is gone.
func (a *App) RemovalHistory(limit int) ([]store.HistoryEntry, error) {
	return a.Store.EventsByType([]string{"remove_requested", "removed"}, limit)
}
