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
slice was activated.

Before restoring, deactivate checks that each primary is still detached at the
slice tip it was swapped to. If a primary drifted — you committed on the
detached HEAD, switched branches, or the journal is stale from an old build —
that repo is refused with zero state change so no work is lost.

Pass --force to restore a drifted primary anyway. If commits were made on the
detached HEAD, they are first rescued to a slis/rescue/<slice>-<repo> branch
before switching away, so nothing is ever orphaned.`,
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
