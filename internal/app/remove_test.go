package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/config"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// fakeQBit holds a fixed set of torrents and records every delete.
type fakeQBit struct {
	mu      sync.Mutex
	hashes  []string
	deleted []string
}

func (f *fakeQBit) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/api/v2/torrents/info":
		var parts []string
		for _, h := range f.hashes {
			parts = append(parts, `{"hash":"`+h+`","progress":1}`)
		}
		_, _ = w.Write([]byte("[" + strings.Join(parts, ",") + "]"))
	case "/api/v2/torrents/delete":
		_ = r.ParseForm()
		f.deleted = append(f.deleted, r.Form.Get("hashes"))
	default:
		http.NotFound(w, r)
	}
}

// An imported film's torrent is the other hardlink to its library file. If
// remove skips it, the disk frees nothing.
func TestRemoveDeletesTheTorrentOfAnImportedFilm(t *testing.T) {
	qbit := &fakeQBit{hashes: []string{"aaaa1111", "bbbb2222"}}
	srv := httptest.NewServer(qbit)
	defer srv.Close()

	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Paths.Movies = t.TempDir()
	cfg.QBittorrent.URL = srv.URL
	cfg.QBittorrent.Username = ""
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	folder := filepath.Join(cfg.Paths.Movies, "AlphaGo (2017)")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := a.Store.AddItem(&store.Item{Kind: "movie", Title: "AlphaGo", Year: 2017, Path: folder, Monitored: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []*store.QueueEntry{
		{ItemID: id, ReleaseTitle: "alphago.1080p", InfoHash: "AAAA1111", State: "imported"},
		// Recorded but already gone from qBittorrent: nothing to delete.
		{ItemID: id, ReleaseTitle: "alphago.720p", InfoHash: "cccc3333", State: "failed"},
	} {
		if err := a.Store.Enqueue(q); err != nil {
			t.Fatal(err)
		}
	}
	it, err := a.Store.GetItem(id)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	plan := strings.Join(a.RemovePlan(ctx, it, app.RemoveOptions{}), "\n")
	if !strings.Contains(plan, "delete torrent aaaa1111") {
		t.Errorf("plan leaves out the imported torrent:\n%s", plan)
	}
	if strings.Contains(plan, "cccc3333") {
		t.Errorf("plan lists a torrent qBittorrent no longer has:\n%s", plan)
	}

	if _, err := a.Remove(ctx, it, app.RemoveOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(qbit.deleted) != 1 || qbit.deleted[0] != "aaaa1111" {
		t.Errorf("deleted %v, want exactly [aaaa1111]", qbit.deleted)
	}
}
