package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/report"
	"github.com/jonnyom/slis/internal/share"
)

var shareCmd = &cobra.Command{
	Use:   "share <slice>",
	Short: "Copy every PR and its diff stats in a slice as raw Markdown",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		slice, err := findSlice(ws, args[0])
		if err != nil {
			return err
		}
		overrides, _ := discovery.LoadOverrides(config.StatePaths().Overrides)
		slice = shareTargetBranches(slice, overrides, git.RefExists)
		markdown, err := share.Markdown(slice, gt.ReadStack, forge.PRForBranch, report.BranchDiff)
		if err != nil {
			return err
		}
		stdout, _ := cmd.Flags().GetBool("stdout")
		if stdout {
			fmt.Print(markdown)
			return nil
		}
		copied, err := writeToClipboard(markdown)
		if err != nil {
			return fmt.Errorf("clipboard write failed: %w", err)
		}
		if !copied {
			fmt.Print(markdown)
			fmt.Fprintln(os.Stderr, "no clipboard tool found; printed Markdown instead")
			return nil
		}
		fmt.Fprintln(os.Stderr, "copied all slice PRs and diffs to clipboard")
		return nil
	},
}

func shareTargetBranches(slice model.Slice, overrides discovery.Overrides, refExists func(string, string) bool) model.Slice {
	members := make(map[string]model.SliceMember, len(slice.Members))
	for repo, member := range slice.Members {
		if branch := overrides[slice.Name][repo]; branch != "" && refExists(member.WorktreePath, branch) {
			member.Branch = branch
		}
		members[repo] = member
	}
	slice.Members = members
	return slice
}

func init() {
	shareCmd.Flags().Bool("stdout", false, "Print raw Markdown instead of copying it")
	rootCmd.AddCommand(shareCmd)
}
