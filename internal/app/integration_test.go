package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/config"
	"github.com/LeandroMAcosta/butaca/internal/library"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// TestImportFromLiveQBittorrent exercises the whole import path against a real
// qBittorrent: queue lookup, locating the video inside the download, and
// hardlinking it into the library. It imports a torrent that has *already*
// finished, so it downloads nothing.
//
// Run with: BUTACA_IT=1 go test ./internal/app/
func TestImportFromLiveQBittorrent(t *testing.T) {
	if os.Getenv("BUTACA_IT") == "" {
		t.Skip("set BUTACA_IT=1 to run against live services")
	}

	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Paths.Movies = filepath.Join(t.TempDir(), "movies")
	cfg.Subtitles.Auto = false // keep the test offline-ish and fast
	if u := os.Getenv("BUTACA_QBITTORRENT_URL"); u != "" {
		cfg.QBittorrent.URL = u
	}

	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	ctx := context.Background()
	torrents, err := a.QBit.List(ctx)
	if err != nil {
		t.Skipf("qBittorrent not reachable: %v", err)
	}

	// Pick any completed torrent that still has its files on disk.
	var chosen *struct {
		Hash, Name, ContentPath string
	}
	for _, tor := range torrents {
		if !tor.Done() || tor.ContentPath == "" {
			continue
		}
		if _, err := library.FindVideo(tor.ContentPath); err != nil {
			continue
		}
		chosen = &struct{ Hash, Name, ContentPath string }{tor.Hash, tor.Name, tor.ContentPath}
		break
	}
	if chosen == nil {
		t.Skip("no completed torrent with a video file available")
	}
	t.Logf("importing from %s", chosen.Name)

	id, err := a.Store.AddItem(&store.Item{
		Kind: "movie", Title: "Integration Fixture", Year: 1999,
		OriginalLanguage: "en", Monitored: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.Enqueue(&store.QueueEntry{
		ItemID: id, ReleaseTitle: chosen.Name,
		InfoHash: strings.ToLower(chosen.Hash), State: "downloading",
	}); err != nil {
		t.Fatal(err)
	}

	lines, err := a.ImportReady(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Fatal("nothing was imported")
	}
	t.Log(lines[0])

	files, err := a.Store.FilesForItem(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file recorded, got %d", len(files))
	}

	// The defining property: the import is a second name for the same bytes.
	n, err := library.LinkCount(files[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Fatalf("link count %d: the import copied instead of hardlinking", n)
	}
	if _, err := os.Stat(files[0].Path); err != nil {
		t.Fatalf("imported file is not on disk: %v", err)
	}
}
