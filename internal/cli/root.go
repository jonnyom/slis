// Package cli implements the command-level logic for slis subcommands.
// The huh interactive picker is confined to init.go; the core logic for each
// command lives in plain functions (listSlices, etc.) so they are testable
// without TTY or cobra wiring.
package cli

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "slis",
	Short: "Multi-repo worktree cockpit",
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
