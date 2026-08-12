// Package cli implements the command-level logic for slis subcommands.
// The huh interactive picker is confined to init.go; the core logic for each
// command lives in plain functions (listSlices, etc.) so they are testable
// without TTY or cobra wiring.
package cli

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags: -X github.com/jonnyom/slis/internal/cli.Version=<tag>
var Version = "dev"

var rootCmd = &cobra.Command{
	Use:     "slis",
	Short:   "Multi-repo worktree cockpit",
	Version: Version,
	// main() prints the error ("slis: …") and sets the exit code; don't let
	// cobra also print "Error: …" (double output) or the usage block on a
	// runtime failure.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runUI()
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
	rootCmd.AddCommand(summaryCmd)
}
