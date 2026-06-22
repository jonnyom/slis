package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/hooks"
)

var initHooksCmd = &cobra.Command{
	Use:   "init-hooks",
	Short: "Install Claude Code Notification/Stop hooks that call `slis hook <event>`",
	Long: `Installs Claude Code hooks into ~/.claude/settings.json so that the slis
TUI is notified when Claude needs input or has finished a session.

This command edits your Claude Code settings file. It is idempotent — running
it multiple times is safe and will not add duplicate entries.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		settingsPath := filepath.Join(home, ".claude", "settings.json")

		binPath, err := os.Executable()
		if err != nil {
			// Fall back to plain name — still functional if it is on PATH.
			binPath = "slis"
		}

		changes, err := hooks.InitHooks(settingsPath, binPath)
		if err != nil {
			return err
		}

		if len(changes) == 0 {
			fmt.Printf("slis hooks already installed in %s\n", settingsPath)
			return nil
		}

		fmt.Printf("Note: the following changes were made to your Claude Code settings (%s):\n", settingsPath)
		for _, c := range changes {
			fmt.Printf("  • %s\n", c)
		}
		fmt.Printf("wrote %s\n", settingsPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initHooksCmd)
}
