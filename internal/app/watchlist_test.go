package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/config"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// newTestApp builds an app against a throwaway database. Its Prowlarr and
// qBittorrent addresses point nowhere, which is the point: any test that
// reaches the network fails loudly instead of quietly downloading something.
func newTestApp(t *testing.T) *App {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Paths.Movies = filepath.Join(t.TempDir(), "movies")
	cfg.Prowlarr.URL = "http://127.0.0.1:1"
	cfg.QBittorrent.URL = "http://127.0.0.1:1"
	cfg.Parse.URL = "http://127.0.0.1:1"

	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

// The whole promise of the watchlist: importing two hundred films is an
// intention, not two hundred downloads.
func TestSearchMissingIgnoresTheWatchlist(t *testing.T) {
	a := newTestApp(t)
	for _, title := range []string{"Vivarium", "1984", "Mysterious Skin"} {
		if _, err := a.Store.AddItem(&store.Item{
			Kind: "movie", Title: title, Monitored: true,
			State: store.StateWatchlist, Source: "letterboxd",
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Prowlarr is unreachable here, so any attempt to search would surface as
	// an error line. Silence proves nothing was searched.
	got, err := a.SearchMissing(context.Background())
	if err != nil {
		t.Fatalf("SearchMissing: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("watchlist entries were acted on: %v", got)
	}
	queue, err := a.Store.PendingQueue()
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 0 {
		t.Fatalf("%d items queued from the watchlist", len(queue))
	}
}

func TestSearchMissingSkipsUnmonitored(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.Store.AddItem(&store.Item{
		Kind: "movie", Title: "Paused", Monitored: true, State: store.StateUnmonitored,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := a.SearchMissing(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, err %v; unmonitored items must be left alone", got, err)
	}
}

// A monitored item with nothing on disk is exactly what the sweep is for, so
// it must reach the (unreachable) indexer and report the failure.
func TestSearchMissingDoesActOnMonitoredItems(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.Store.AddItem(&store.Item{
		Kind: "movie", Title: "Wanted", Monitored: true, State: store.StateMonitored,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := a.SearchMissing(context.Background())
	if err != nil {
		t.Fatalf("SearchMissing: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("a monitored item with no file should have been searched")
	}
}

// Enforced centrally so no call site can download a watchlist entry by mistake.
func TestSearchMissingForRefusesWatchlistEntries(t *testing.T) {
	a := newTestApp(t)
	it := &store.Item{Kind: "movie", Title: "Vivarium", Monitored: true, State: store.StateWatchlist}
	id, err := a.Store.AddItem(it)
	if err != nil {
		t.Fatal(err)
	}
	it.ID = id

	_, err = a.SearchMissingFor(context.Background(), it)
	if !errors.Is(err, ErrOnWatchlist) {
		t.Fatalf("err = %v, want ErrOnWatchlist", err)
	}
}

// Promoting is what makes it downloadable, and it must survive a reload.
func TestPromotingMakesItSearchable(t *testing.T) {
	a := newTestApp(t)
	id, err := a.Store.AddItem(&store.Item{
		Kind: "movie", Title: "Vivarium", Monitored: true, State: store.StateWatchlist,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := a.SearchMissing(context.Background()); len(got) != 0 {
		t.Fatal("should not act while on the watchlist")
	}
	if err := a.Store.SetState(id, store.StateMonitored); err != nil {
		t.Fatal(err)
	}
	got, err := a.SearchMissing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 {
		t.Fatal("after promoting, the sweep should try to find it")
	}
}

// Re-importing a list must not knock back something already promoted.
func TestReimportDoesNotResetState(t *testing.T) {
	a := newTestApp(t)
	first := &store.Item{Kind: "movie", TMDBID: 458305, Title: "Vivarium", Year: 2019,
		Monitored: true, State: store.StateWatchlist, Source: "letterboxd"}
	id, err := a.Store.AddItem(first)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.SetState(id, store.StateMonitored); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.AddItem(first); err != nil {
		t.Fatal(err)
	}
	got, err := a.Store.GetItem(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != store.StateMonitored {
		t.Errorf("state = %q after re-import, want it to stay monitored", got.State)
	}
}

// A deletion has to leave a trace, or "where did my film go?" is unanswerable.
func TestRemoveIsAudited(t *testing.T) {
	a := newTestApp(t)
	id, err := a.Store.AddItem(&store.Item{
		Kind: "movie", Title: "Wicked City", Year: 1987, Monitored: true, State: store.StateMonitored,
	})
	if err != nil {
		t.Fatal(err)
	}
	it, err := a.Store.GetItem(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Remove(context.Background(), it, RemoveOptions{KeepFiles: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Store.GetItem(id); err == nil {
		t.Fatal("the item should be gone")
	}

	events, err := a.RemovalHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	var sawRequest bool
	for _, e := range events {
		if e.Event == "remove_requested" && strings.Contains(e.Detail, "Wicked City") {
			sawRequest = true
		}
	}
	if !sawRequest {
		t.Fatalf("no audit record survived the deletion: %+v", events)
	}
}
