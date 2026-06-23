package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/model"
)

var groupCmd = &cobra.Command{
	Use:   "group <name> <slice>...",
	Short: "Manually group slices into one named slice",
	Long: `Group two or more slices into a single named slice. Useful when one feature
spans repos under DIFFERENT branch names (which branch-name auto-grouping keeps
as separate slices). Writes a grouping override; re-runnable and reversible with
'slis ungroup <name>'.`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		wanted := args[1:]

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		sp := config.StatePaths()

		raw, err := discovery.Discover(ws)
		if err != nil {
			return fmt.Errorf("discover: %w", err)
		}
		ov, _ := discovery.LoadOverrides(sp.Overrides)
		if ov == nil {
			ov = discovery.Overrides{}
		}

		byName := make(map[string]model.Slice)
		for _, s := range discovery.Apply(raw, ov) {
			byName[s.Name] = s
		}

		if ov[name] == nil {
			ov[name] = make(map[string]string)
		}
		for _, sn := range wanted {
			s, ok := byName[sn]
			if !ok {
				return fmt.Errorf("slice %q not found", sn)
			}
			for repo, m := range s.Members {
				ov[name][repo] = m.Branch
			}
		}

		if err := discovery.SaveOverrides(sp.Overrides, ov); err != nil {
			return fmt.Errorf("save overrides: %w", err)
		}
		fmt.Printf("Grouped %v into %q\n", wanted, name)
		return nil
	},
}

var ungroupCmd = &cobra.Command{
	Use:   "ungroup <name>",
	Short: "Remove a grouping override (slices revert to branch-name auto-grouping)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sp := config.StatePaths()
		ov, _ := discovery.LoadOverrides(sp.Overrides)
		if _, ok := ov[name]; !ok {
			return fmt.Errorf("no grouping override named %q", name)
		}
		delete(ov, name)
		if err := discovery.SaveOverrides(sp.Overrides, ov); err != nil {
			return fmt.Errorf("save overrides: %w", err)
		}
		fmt.Printf("Ungrouped %q\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(groupCmd)
	rootCmd.AddCommand(ungroupCmd)
}
