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
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := config.StatePaths()
		if err := swap.Deactivate(sp.ActiveJournal); err != nil {
			return err
		}
		fmt.Println("restored")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deactivateCmd)
}
