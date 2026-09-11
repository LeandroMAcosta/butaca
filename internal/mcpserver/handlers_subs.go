package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

func (s *Server) handleSubtitles(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ref := req.GetString("item", "")
	if req.GetBool("sync", false) {
		return s.syncSubtitles(ctx, ref, req.GetBool("force", false))
	}
	if ref == "" {
		got, err := s.app.FillSubtitleGaps(ctx)
		if err != nil {
			return fail(err), nil
		}
		return lines("subtitle sweep", got, "every file already has the configured subtitles"), nil
	}
	it, err := s.resolve(ref)
	if err != nil {
		return fail(err), nil
	}
	got, err := s.app.SubtitlesForItem(ctx, it)
	if err != nil {
		return fail(err), nil
	}
	return lines(it.Title, got, "nothing to fetch: "+it.Title+" already has its subtitles"), nil
}

func (s *Server) syncSubtitles(ctx context.Context, ref string, force bool) (*mcp.CallToolResult, error) {
	var reports []app.SyncReport
	var err error
	header := "subtitle sync, whole library"
	if ref == "" {
		reports, err = s.app.SyncAllSubtitles(ctx, force, nil)
	} else {
		it, rerr := s.resolve(ref)
		if rerr != nil {
			return fail(rerr), nil
		}
		header = "subtitle sync, " + it.Title
		reports, err = s.app.SyncSubtitles(ctx, it, force)
	}
	if err != nil {
		return fail(err), nil
	}
	out := make([]string, 0, len(reports))
	for _, r := range reports {
		out = append(out, r.String())
	}
	return lines(header, out, "no subtitles to sync"), nil
}
