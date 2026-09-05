// Package mcpserver exposes butaca over the Model Context Protocol, so the
// catalog can be driven from a chat client the same way as from the CLI.
package mcpserver

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/LeandroMAcosta/butaca/internal/app"
)

const version = "1.0.0"

type Server struct {
	app *app.App
	mcp *server.MCPServer
}

func New(a *app.App) *Server {
	s := &Server{
		app: a,
		mcp: server.NewMCPServer("butaca", version,
			server.WithToolCapabilities(true),
			server.WithRecovery(),
		),
	}
	s.register()
	return s
}

// ServeStdio runs the server over stdin/stdout, which is how MCP clients
// launch it.
func (s *Server) ServeStdio() error { return server.ServeStdio(s.mcp) }

func (s *Server) register() {
	s.mcp.AddTool(mcp.NewTool("list",
		mcp.WithDescription("List the catalog: every movie and series, with file count and size on disk."),
	), s.handleList)

	s.mcp.AddTool(mcp.NewTool("add",
		mcp.WithDescription(
			"Add a movie to the catalog and search for it. Without a TMDB key, pass "+
				"original_language (the film's own language, ISO 639-1) so the release "+
				"rules can match it."),
		mcp.WithString("title", mcp.Required(), mcp.Description("Film title")),
		mcp.WithNumber("year", mcp.Description("Release year")),
		mcp.WithString("original_language", mcp.Description("ISO 639-1 code of the film's original language, e.g. fr, ja, de")),
		mcp.WithString("alt_title", mcp.Description("Another name it is released under, often the original-language title")),
		mcp.WithBoolean("search", mcp.Description("Search and grab immediately (default true)")),
	), s.handleAdd)

	s.mcp.AddTool(mcp.NewTool("search",
		mcp.WithDescription(
			"Search releases for something already in the catalog. Returns every candidate "+
				"with its score, and for rejected ones the reason."),
		mcp.WithString("item", mcp.Required(), mcp.Description("Catalog id or part of the title")),
		mcp.WithBoolean("grab", mcp.Description("Send the best candidate to the download client")),
	), s.handleSearch)

	s.mcp.AddTool(mcp.NewTool("grab",
		mcp.WithDescription(
			"Force a specific release, bypassing the rules. Use when the automatic "+
				"choice is wrong."),
		mcp.WithString("item", mcp.Required(), mcp.Description("Catalog id or part of the title")),
		mcp.WithString("release", mcp.Required(), mcp.Description("Exact release title from a previous search")),
	), s.handleGrab)

	s.mcp.AddTool(mcp.NewTool("remove",
		mcp.WithDescription(
			"Remove an item from the catalog, delete its library folder, and delete the "+
				"torrent with its files. All three matter: library entries are hardlinks to "+
				"the download, so removing only one frees no disk space."),
		mcp.WithString("item", mcp.Required(), mcp.Description("Catalog id or part of the title")),
		mcp.WithBoolean("keep_files", mcp.Description("Remove the catalog entry only, leaving files and torrent")),
	), s.handleRemove)

	s.mcp.AddTool(mcp.NewTool("status",
		mcp.WithDescription("Dependency health, configured paths, and the download queue."),
	), s.handleStatus)

	s.mcp.AddTool(mcp.NewTool("subtitles",
		mcp.WithDescription("Fetch missing subtitles for one item, or for the whole library."),
		mcp.WithString("item", mcp.Description("Catalog id or part of the title; omit to sweep everything")),
	), s.handleSubtitles)

	s.mcp.AddTool(mcp.NewTool("import",
		mcp.WithDescription("Import downloads that have finished but are not in the library yet."),
	), s.handleImport)
}

func text(format string, args ...any) *mcp.CallToolResult {
	return mcp.NewToolResultText(fmt.Sprintf(format, args...))
}

func fail(err error) *mcp.CallToolResult {
	return mcp.NewToolResultError(err.Error())
}

// lines joins output, collapsing the empty case into a readable sentence.
func lines(header string, items []string, empty string) *mcp.CallToolResult {
	if len(items) == 0 {
		return mcp.NewToolResultText(empty)
	}
	var b strings.Builder
	if header != "" {
		b.WriteString(header)
		b.WriteString("\n")
	}
	for _, l := range items {
		b.WriteString("  ")
		b.WriteString(l)
		b.WriteString("\n")
	}
	return mcp.NewToolResultText(strings.TrimRight(b.String(), "\n"))
}
