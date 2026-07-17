package cli

import (
	"fmt"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/spf13/cobra"
)

var deactivateCmd = &cobra.Command{
	Use:   "deactivate",
	Short: "Deactivate the current slice — restore all repo primaries to their prior branches",
	Long: `deactivate restores every repo primary to the branch it was on before the
slice was activated, then deletes the slis/live temp branch it created.

Before restoring, deactivate checks that each primary is still on its slis/live
temp branch at the slice tip. If a primary drifted — you switched it off the
temp branch, or the journal is stale from an old build — that repo is refused
with zero state change so no work is lost. If you committed on the temp branch,
those commits are already safe on that named branch, so deactivate refuses and
lists them.

Pass --force to restore a drifted primary anyway. If you committed on the temp
branch, --force renames it to slis/rescue/<slice>-<repo> (never deletes it)
before switching away, so nothing is ever lost.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		sp := config.StatePaths()
		if err := swap.Deactivate(sp.ActiveJournal, force); err != nil {
			return err
		}
		fmt.Println("restored")
		return nil
	},
}

func init() {
	deactivateCmd.Flags().Bool("force", false, "Restore even if a primary drifted from its slice tip (rescues any commits to a slis/rescue/* branch first)")
	rootCmd.AddCommand(deactivateCmd)
}
