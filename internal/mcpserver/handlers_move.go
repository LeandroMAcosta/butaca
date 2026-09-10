package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleMove(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ref, err := req.RequireString("item")
	if err != nil {
		return fail(err), nil
	}
	to, err := req.RequireString("to")
	if err != nil {
		return fail(err), nil
	}
	it, err := s.resolve(ref)
	if err != nil {
		return fail(err), nil
	}
	if req.GetBool("dry_run", false) {
		from, dest, err := s.app.MovePlan(it, to)
		if err != nil {
			return fail(err), nil
		}
		return text("would move %s to %s:\n  from %s\n  to   %s", it.Title, to, orDash(from), dest), nil
	}
	dest, err := s.app.Move(it, to)
	if err != nil {
		return fail(err), nil
	}
	return text("moved %s to %s: %s", it.Title, to, dest), nil
}

// optBool reads a boolean argument that has three states: absent (nil), true
// or false. The difference matters where absent means "decide for me".
func optBool(req mcp.CallToolRequest, name string) *bool {
	v, ok := req.GetArguments()[name].(bool)
	if !ok {
		return nil
	}
	return &v
}
