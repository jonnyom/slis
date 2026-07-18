// Package cli implements the command-level logic for slis subcommands.
// The huh interactive picker is confined to init.go; the core logic for each
// command lives in plain functions (listSlices, etc.) so they are testable
// without TTY or cobra wiring.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/tui"
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
	// When invoked with no subcommand, launch the JS (OpenTUI) front-end by
	// default, falling back to the Go TUI when the JS front-end can't be
	// resolved (or when SLIS_TUI=go forces it).
	RunE: func(cmd *cobra.Command, args []string) error {
		binPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot locate the slis binary: %w", err)
		}
		migrateExistingHooksBestEffort(binPath)

		launch, resolveErr := resolveUILaunch(binPath, os.Getenv("SLIS_TUI_DIR"), regularFileExists)
		launchJS, notice := chooseDefaultUI(os.Getenv("SLIS_TUI"), resolveErr)
		if launchJS {
			execErr := execJSUI(binPath, launch)
			fmt.Fprintf(os.Stderr, "slis: JS UI failed to launch (%v); falling back to the Go TUI (set SLIS_TUI=go to skip)\n", execErr)
			return runGoTUI()
		}
		if notice != "" {
			fmt.Fprintln(os.Stderr, notice)
		}
		return runGoTUI()
	},
}

func runGoTUI() error {
	ws, err := config.LoadWorkspace(config.WorkspacePath())
	if err != nil {
		return fmt.Errorf("no workspace — run `slis init` first: %w", err)
	}
	return tui.Run(ws)
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
