package cli

import (
	"github.com/spf13/cobra"
)

var initHooksCmd = &cobra.Command{
	Use:   "init-hooks",
	Short: "Install Claude Code Notification/Stop hooks that call `slis hook <event>`",
	Long: `Installs Claude Code hooks into ~/.claude/settings.json so that the slis
TUI is notified when Claude needs input or has finished a session.

This command edits your Claude Code settings file. It is idempotent — running
it multiple times is safe and will not add duplicate entries.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return installHooks()
	},
}

func init() {
	rootCmd.AddCommand(initHooksCmd)
}
