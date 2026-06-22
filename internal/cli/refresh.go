package cli

import (
	"fmt"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/spf13/cobra"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh the active slice — advance primaries to new branch tips",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := config.StatePaths()
		j, err := swap.Refresh(sp.ActiveJournal)
		if err != nil {
			return err
		}
		if j == nil {
			fmt.Println("no active slice")
			return nil
		}
		fmt.Printf("refreshed slice %q:\n", j.Slice)
		for _, rs := range j.Repos {
			fmt.Printf("  %s: now at %s (branch: %s)\n", rs.Repo, rs.TargetSHA[:min(7, len(rs.TargetSHA))], rs.Branch)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(refreshCmd)
}
