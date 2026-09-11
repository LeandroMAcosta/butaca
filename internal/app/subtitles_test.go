package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/config"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

// The sidecar decides how to sync; butaca must pass it the release name the
// file had before import and report each language's outcome.
func TestSyncSubtitlesPassesTheReleaseName(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sync" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"status":"synced","method":"embedded","offset_seconds":-11.13,
			"detail":"offset -11.13s","attempts":["embedded: offset -11.13s"]}`))
	}))
	defer srv.Close()

	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Paths.Movies = t.TempDir()
	cfg.Parse.URL = srv.URL
	cfg.Subtitles.Languages = []string{"es"}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	id, _ := a.Store.AddItem(&store.Item{Kind: "movie", Title: "Ghost in the Shell", Year: 1995, Monitored: true})
	_ = a.Store.Enqueue(&store.QueueEntry{ItemID: id, ReleaseTitle: "Ghost.in.the.Shell.1995.1080p.BluRay-GalaxyRG", InfoHash: "abc", State: "imported"})
	_, _ = a.Store.AddFile(&store.File{ItemID: id, Path: cfg.Paths.Movies + "/Ghost in the Shell (1995).mkv"})
	it, _ := a.Store.GetItem(id)

	reports, err := a.SyncSubtitles(context.Background(), it, false)
	if err != nil {
		t.Fatal(err)
	}
	if got["release_name"] != "Ghost.in.the.Shell.1995.1080p.BluRay-GalaxyRG" || got["lang"] != "es" {
		t.Errorf("sidecar got %v", got)
	}
	if len(reports) != 1 || !strings.Contains(reports[0].String(), "synced via embedded: offset -11.13s") {
		t.Errorf("reports = %v", reports)
	}
}
