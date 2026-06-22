// Package cli implements the command-level logic for slis subcommands.
// The huh interactive picker is confined to init.go; the core logic for each
// command lives in plain functions (listSlices, etc.) so they are testable
// without TTY or cobra wiring.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:   "slis",
	Short: "Multi-repo worktree cockpit",
	// When invoked with no subcommand, launch the TUI.
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("no workspace — run `slis init` first: %w", err)
		}
		return tui.Run(ws)
	},
}

// Execute runs the root cobra command. It is called from main.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(initCmd)
}
