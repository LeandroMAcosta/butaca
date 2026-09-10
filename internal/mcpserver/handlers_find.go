package mcpserver

import (
	"context"
	"encoding/json"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

// findRelease is one row of a find result. The JSON shape is a contract with
// MCP clients that key on id, so fields are only ever added, never renamed.
type findRelease struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Indexer   string   `json:"indexer"`
	Quality   string   `json:"quality"`
	SizeBytes int64    `json:"size_bytes"`
	Size      string   `json:"size"`
	Seeders   int      `json:"seeders"`
	Score     int      `json:"score"`
	Accepted  bool     `json:"accepted"`
	Rejects   []string `json:"rejects"`
	Mirrors   int      `json:"mirrors"`
}

type findResult struct {
	Query    string        `json:"query"`
	Total    int           `json:"total"`
	Merged   int           `json:"merged"`
	Accepted int           `json:"accepted"`
	Releases []findRelease `json:"releases"`
}

type findGrabbed struct {
	Added struct {
		ID      int64  `json:"id"`
		Title   string `json:"title"`
		Year    int    `json:"year"`
		Library string `json:"library"`
	} `json:"added"`
	Release string `json:"release"`
}

func (s *Server) handleFind(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return fail(err), nil
	}
	if id := req.GetString("grab", ""); id != "" {
		return s.findGrab(ctx, query, id, optBool(req, "documentary"))
	}

	cands, err := s.app.SearchNew(ctx, query)
	if err != nil {
		return fail(err), nil
	}
	res := findResult{Query: query, Total: len(cands), Releases: []findRelease{}}
	cands, mirrors := app.DedupeReleases(cands)
	app.SortByScore(cands)
	res.Merged = len(cands)

	all := req.GetBool("all", false)
	for _, c := range cands {
		if c.Accepted() {
			res.Accepted++
		} else if !all {
			continue
		}
		rejects := c.Rejects
		if rejects == nil {
			rejects = []string{}
		}
		res.Releases = append(res.Releases, findRelease{
			ID:        app.ReleaseID(c.Release),
			Title:     c.Release.Title,
			Indexer:   c.Release.Indexer,
			Quality:   c.Parsed.ScreenSize,
			SizeBytes: c.Release.Size,
			Size:      humanSize(c.Release.Size),
			Seeders:   c.Release.Seeders,
			Score:     c.Score,
			Accepted:  c.Accepted(),
			Rejects:   rejects,
			Mirrors:   mirrors[app.ReleaseKey(c.Release.Title)],
		})
	}
	return jsonResult(res)
}

func (s *Server) findGrab(ctx context.Context, query, id string, documentary *bool) (*mcp.CallToolResult, error) {
	it, c, err := s.app.GrabNew(ctx, query, id, documentary)
	if err != nil {
		return fail(err), nil
	}
	var out findGrabbed
	out.Added.ID, out.Added.Title, out.Added.Year = it.ID, it.Title, it.Year
	out.Added.Library = s.app.LibraryOf(it)
	out.Release = c.Release.Title
	return jsonResult(out)
}

func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return fail(err), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}
