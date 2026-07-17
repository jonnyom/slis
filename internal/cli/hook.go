package cli

import (
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/hooks"
)

var hookCmd = &cobra.Command{
	Use:    "hook <event>",
	Short:  "Handle a Claude Code hook event (machine-invoked)",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			// Fail quiet — slis may not be configured on this machine.
			return nil
		}

		sp := config.StatePaths()

		slices, err := discovery.Discover(ws)
		if err != nil {
			return nil
		}
		ov, _ := discovery.LoadOverrides(sp.Overrides)
		slices = discovery.Apply(slices, ov)

		return hooks.HandleHook(args[0], os.Stdin, slices, sp.EventsDir, ws.Notify, time.Now().UnixNano())
	},
}

func init() {
	rootCmd.AddCommand(hookCmd)
}
