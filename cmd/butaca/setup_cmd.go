package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/LeandroMAcosta/butaca/internal/config"
	"github.com/LeandroMAcosta/butaca/internal/tui"
)

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Interactive first-run configuration",
		Long: "Asks where media should live, which audio language to accept, which\n" +
			"subtitles to fetch, and how to reach Prowlarr and qBittorrent.",
		RunE: func(*cobra.Command, []string) error {
			return tui.RunSetup(cfg)
		},
	}
}

// configExists reports whether the user has ever configured butaca.
func configExists(path string) bool {
	if path == "" {
		path = config.DefaultPath()
	}
	_, err := os.Stat(path)
	return err == nil
}

// maybeRunSetup launches the wizard the first time butaca is used
// interactively, so a new install is never a guessing game. Non-interactive
// runs (Docker, cron, MCP) fall through to defaults and environment variables.
func maybeRunSetup(cmdName string) error {
	switch cmdName {
	case "setup", "config", "help", "completion", "version":
		return nil
	}
	if configExists(cfgPath) {
		return nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil
	}
	fmt.Println("No configuration found. Setting butaca up first.")
	fmt.Println()
	if err := tui.RunSetup(cfg); err != nil {
		return err
	}
	fmt.Println()
	return nil
}
