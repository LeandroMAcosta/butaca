package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/LeandroMAcosta/butaca/internal/app"
	"github.com/LeandroMAcosta/butaca/internal/mcpserver"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run the MCP server on stdin/stdout",
		Long: "Exposes the catalog to an MCP client such as Claude Code. Register it with:\n\n" +
			"  claude mcp add butaca -- butaca mcp\n\n" +
			"The server speaks over stdin/stdout, so it prints nothing else.",
		RunE: func(*cobra.Command, []string) error {
			return withApp(func(_ context.Context, a *app.App) error {
				return mcpserver.New(a).ServeStdio()
			})
		},
	}
}
