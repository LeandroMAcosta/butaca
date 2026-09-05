package app

import (
	"context"
	"fmt"
	"os"

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

	if !opt.KeepFiles && it.Path != "" {
		if _, err := os.Stat(it.Path); err == nil {
			err := library.RemoveFolder(it.Path)
			steps = append(steps, RemoveStep{What: "deleted " + it.Path, Done: err == nil, Err: err})
		} else {
			steps = append(steps, RemoveStep{What: "no folder at " + it.Path})
		}
	}

	if !opt.KeepFiles && !opt.KeepTorrent {
		pending, err := a.Store.PendingQueue()
		if err != nil {
			steps = append(steps, RemoveStep{What: "read queue", Err: err})
		}
		found := false
		for _, q := range pending {
			if q.ItemID != it.ID {
				continue
			}
			found = true
			err := a.QBit.Delete(ctx, q.InfoHash, true)
			steps = append(steps, RemoveStep{
				What: "removed torrent " + shortHash(q.InfoHash), Done: err == nil, Err: err})
		}
		if !found {
			steps = append(steps, RemoveStep{What: "no torrent tracked for this item"})
		}
	}

	if err := a.Store.DeleteItem(it.ID); err != nil {
		steps = append(steps, RemoveStep{What: "remove from catalog", Err: err})
		return steps, fmt.Errorf("remove %s from catalog: %w", it.Title, err)
	}
	steps = append(steps, RemoveStep{What: "removed from catalog", Done: true})
	return steps, nil
}

// RemovePlan describes what Remove would do, for a confirmation prompt.
func (a *App) RemovePlan(it *store.Item, opt RemoveOptions) []string {
	var plan []string
	if !opt.KeepFiles && it.Path != "" {
		if _, err := os.Stat(it.Path); err == nil {
			plan = append(plan, fmt.Sprintf("delete %s (%s)", it.Path, library.HumanSize(it.SizeBytes)))
		}
	}
	if !opt.KeepFiles && !opt.KeepTorrent {
		if pending, err := a.Store.PendingQueue(); err == nil {
			for _, q := range pending {
				if q.ItemID == it.ID {
					plan = append(plan, "delete torrent "+shortHash(q.InfoHash)+" and its files")
				}
			}
		}
	}
	plan = append(plan, fmt.Sprintf("remove %q from the catalog", it.Title))
	return plan
}

func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}
