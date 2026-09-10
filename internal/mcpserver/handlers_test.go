package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/config"
	"github.com/LeandroMAcosta/butaca/internal/store"
)

func TestRemoveDryRunTouchesNothing(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Paths.Movies = t.TempDir()
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	folder := filepath.Join(cfg.Paths.Movies, "Amelie (2001)")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	id, err := a.Store.AddItem(&store.Item{
		Kind: "movie", Title: "Amelie", Year: 2001, Path: folder, Monitored: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	var req mcp.CallToolRequest
	req.Params.Arguments = map[string]any{"item": "Amelie", "dry_run": true}
	res, err := New(a).handleRemove(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	out := res.Content[0].(mcp.TextContent).Text
	for _, want := range []string{"would remove Amelie:", "delete " + folder, `remove "Amelie" from the catalog`} {
		if !strings.Contains(out, want) {
			t.Errorf("plan is missing %q:\n%s", want, out)
		}
	}
	if _, err := os.Stat(folder); err != nil {
		t.Errorf("dry run deleted the folder: %v", err)
	}
	if _, err := a.Store.GetItem(id); err != nil {
		t.Errorf("dry run removed the catalog row: %v", err)
	}
}
