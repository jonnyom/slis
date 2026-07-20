package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// gatherResult is the per-repo outcome of a gather, emitted with --json.
type gatherResult struct {
	Repo   string   `json:"repo"`
	Tip    string   `json:"tip"`
	Folded []string `json:"folded"`
	Linear bool     `json:"linear"`
}

var gatherCmd = &cobra.Command{
	Use:   "gather <name> <slice>",
	Short: "Gather a Graphite stack into one slice, represented by its tip",
	Long: `Collapse the Graphite stack that <slice> belongs to into a single named
slice. Per repo, slis reads the stack's lineage, keeps the tip branch as the
slice's representative (its commit contains the whole stack), and folds the
intermediate branches so they stop showing as their own slices. Worktrees are
never touched. Reversible with 'slis scatter <name>'.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, sliceName := args[0], args[1]
		useJSON, _ := cmd.Flags().GetBool("json")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		sp := config.StatePaths()

		raw := discovery.Report(ws, sp.Registry).Slices
		ov, _ := discovery.LoadOverrides(sp.Overrides)
		if ov == nil {
			ov = discovery.Overrides{}
		}
		folds, _ := discovery.LoadFolded(sp.Overrides)
		if folds == nil {
			folds = discovery.Folded{}
		}

		var target *model.Slice
		for _, s := range discovery.Apply(raw, ov) {
			if s.Name == sliceName {
				sc := s
				target = &sc
				break
			}
		}
		if target == nil {
			return fmt.Errorf("slice %q not found", sliceName)
		}

		results := make([]gatherResult, 0, len(target.Members))
		for repo, m := range target.Members {
			st, err := gt.ReadStack(m.WorktreePath)
			if err != nil || len(st) == 0 {
				continue
			}
			tip, folded, linear, ok := discovery.GatherStack(st, m.Branch)
			if !ok {
				continue
			}
			if ov[name] == nil {
				ov[name] = map[string]string{}
			}
			if folds[name] == nil {
				folds[name] = map[string][]string{}
			}
			ov[name][repo] = tip
			folds[name][repo] = folded
			results = append(results, gatherResult{Repo: repo, Tip: tip, Folded: folded, Linear: linear})
		}

		if len(results) == 0 {
			return fmt.Errorf("slice %q has no gatherable stack (each branch is standalone)", sliceName)
		}

		if err := discovery.SaveConfig(sp.Overrides, ov, folds); err != nil {
			return fmt.Errorf("save overrides: %w", err)
		}

		if useJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]any{"name": name, "gathered": results})
		}
		for _, r := range results {
			warn := ""
			if !r.Linear {
				warn = " (⚠ non-linear stack — pulled in a whole connected component)"
			}
			fmt.Printf("Gathered %s: tip %s, folded %v into %q%s\n", r.Repo, r.Tip, r.Folded, name, warn)
		}
		return nil
	},
}

var scatterCmd = &cobra.Command{
	Use:   "scatter <name>",
	Short: "Undo a gather (folded branches reappear as their own slices)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sp := config.StatePaths()
		ov, _ := discovery.LoadOverrides(sp.Overrides)
		folds, _ := discovery.LoadFolded(sp.Overrides)
		_, hadOv := ov[name]
		_, hadFold := folds[name]
		if !hadOv && !hadFold {
			return fmt.Errorf("no gather named %q", name)
		}
		delete(ov, name)
		delete(folds, name)
		if err := discovery.SaveConfig(sp.Overrides, ov, folds); err != nil {
			return fmt.Errorf("save overrides: %w", err)
		}
		fmt.Printf("Scattered %q\n", name)
		return nil
	},
}

func init() {
	gatherCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(gatherCmd)
	rootCmd.AddCommand(scatterCmd)
}
