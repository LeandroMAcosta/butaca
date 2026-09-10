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

	s.mcp.AddTool(mcp.NewTool("find",
		mcp.WithDescription(
			"Search the trackers for a film that is not in the catalog, without changing "+
				"anything. Returns JSON: releases sorted best first, each with a stable id. "+
				"Pass grab with one of those ids to catalogue the film and download that "+
				"exact release; if it is gone from fresh results this fails rather than "+
				"picking another. Adding the year to the query helps: \"Amelie 2001\"."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Film title, optionally followed by its year")),
		mcp.WithBoolean("all", mcp.Description("Include rejected releases and why (default false)")),
		mcp.WithString("grab", mcp.Description("Release id from a previous find: catalogue the film and download it")),
	), s.handleFind)

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
		mcp.WithBoolean("dry_run", mcp.Description("List what would be deleted without touching anything")),
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

	s.registerExtras()
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

// registerExtras adds the tools that came with profiles, the watchlist and
// recommendations.
func (s *Server) registerExtras() {
	s.mcp.AddTool(mcp.NewTool("watchlist",
		mcp.WithDescription(
			"Show the films queued for some day. Watchlist entries are catalogued but "+
				"never searched until promoted, which is what stops a bulk import from "+
				"starting hundreds of downloads."),
	), s.handleWatchlist)

	s.mcp.AddTool(mcp.NewTool("watch",
		mcp.WithDescription("Promote a watchlist entry so butaca searches for it, or push one back."),
		mcp.WithString("item", mcp.Required(), mcp.Description("Catalog id or part of the title")),
		mcp.WithBoolean("monitor", mcp.Description("true promotes it, false returns it to the watchlist (default true)")),
		mcp.WithBoolean("search", mcp.Description("Search immediately after promoting")),
	), s.handleWatch)

	s.mcp.AddTool(mcp.NewTool("recommend",
		mcp.WithDescription(
			"Suggest films from what the library already holds, via TMDB. Needs a TMDB API key. "+
				"Never suggests something already in the catalog."),
		mcp.WithNumber("limit", mcp.Description("How many suggestions (default 20)")),
		mcp.WithNumber("add", mcp.Description("Add suggestion N to the watchlist")),
	), s.handleRecommend)

	s.mcp.AddTool(mcp.NewTool("languages",
		mcp.WithDescription(
			"Report which audio and subtitle languages the library actually holds, read "+
				"from the media files themselves rather than their names."),
		mcp.WithString("item", mcp.Description("Catalog id or part of a title; omit for the whole library")),
	), s.handleLanguages)

	s.mcp.AddTool(mcp.NewTool("disk",
		mcp.WithDescription("Free space on the media volume and how much the library occupies."),
	), s.handleDisk)

	s.mcp.AddTool(mcp.NewTool("profiles",
		mcp.WithDescription(
			"List the profiles: each is a person's release preferences, subtitle languages "+
				"and Letterboxd account."),
	), s.handleProfiles)

	s.mcp.AddTool(mcp.NewTool("letterboxd_import",
		mcp.WithDescription(
			"Import a Letterboxd member's public watchlist into butaca's watchlist. "+
				"Nothing is downloaded: imports land on the watchlist."),
		mcp.WithString("user", mcp.Description("Letterboxd username; omit to use the default profile's")),
		mcp.WithBoolean("dry_run", mcp.Description("Report what would be imported without writing")),
	), s.handleLetterboxd)
}
