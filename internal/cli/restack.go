package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/restack"
)

var restackCmd = &cobra.Command{
	Use:   "restack <slice>",
	Short: "Restack a slice's Graphite branches across all its repos (gt restack)",
	Long: `Run 'gt restack' in each of the slice's repo worktrees, rebasing each branch
onto its parent. Dirty worktrees are skipped (commit or stash first). On a
conflict, gt leaves an in-progress rebase to resolve in that worktree, then run
'gt continue'.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		sl, err := findSlice(ws, args[0])
		if err != nil {
			return err
		}

		rep := restack.Run(sl, gt.Restack)
		fmt.Printf("Restack %q:\n", rep.Slice)
		for _, r := range rep.Repos {
			switch {
			case r.SkippedDirty:
				fmt.Printf("  %s: skipped — worktree dirty (commit or stash first)\n", r.Repo)
			case r.Conflict:
				fmt.Printf("  %s: CONFLICT — resolve in the worktree, then `gt continue`\n", r.Repo)
			case r.Err != "":
				fmt.Printf("  %s: failed — %s\n", r.Repo, r.Err)
			case r.Restacked:
				fmt.Printf("  %s: restacked\n", r.Repo)
			}
		}
		return nil
	},
}

// gtPerRepo runs an interactive `gt <args>` in each of the slice's repo
// worktrees with inherited stdio, so the user answers any gt prompts and sees
// its output. Shared by sync / submit / merge.
func gtPerRepo(sliceName, label string, gtArgs ...string) error {
	ws, err := config.LoadWorkspace(config.WorkspacePath())
	if err != nil {
		return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
	}
	sl, err := findSlice(ws, sliceName)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("gt"); err != nil {
		return fmt.Errorf("gt CLI not found on PATH")
	}
	for _, repo := range sl.Repos() {
		m := sl.Members[repo]
		fmt.Printf("\n══ gt %s: %s ══\n", label, repo)
		c := exec.Command("gt", gtArgs...) //nolint:gosec
		c.Dir = m.WorktreePath
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := c.Run(); err != nil {
			fmt.Printf("  %s: gt %s exited: %v\n", repo, label, err)
		}
	}
	return nil
}

var syncCmd = &cobra.Command{
	Use:   "sync <slice>",
	Short: "Sync each of a slice's repos with remote (gt sync — interactive, repo-wide)",
	Long: `Run 'gt sync' in each of the slice's repos. gt sync is repo-wide: it syncs all
branches, may overwrite trunk with the remote, and prompts to delete merged or
closed branches. It runs interactively so you answer those prompts.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return gtPerRepo(args[0], "sync", "sync") },
}

var submitCmd = &cobra.Command{
	Use:   "submit <slice>",
	Short: "Submit a slice's Graphite stack as PRs (gt submit — interactive, pushes + opens/updates PRs)",
	Long: `Run 'gt submit' in each of the slice's repos: force-push the stack and create
or update a PR per branch. It runs interactively so you can edit PR titles and
descriptions. gt validates the stack is restacked first and fails on conflicts —
run 'slis restack <slice>' first if needed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return gtPerRepo(args[0], "submit", "submit") },
}

var mergeCmd = &cobra.Command{
	Use:   "merge <slice>",
	Short: "Merge a slice's stack via Graphite's server-side merge queue (gt merge)",
	Long: `Run 'gt merge' in each of the slice's repos. Graphite merges the PRs from trunk
to the current branch ON ITS SERVERS — handling the squash/merge/restack — so you
don't sit waiting or restacking locally. Submit the stack first ('slis submit')
if the PRs don't exist yet.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return gtPerRepo(args[0], "merge", "merge") },
}

func init() {
	rootCmd.AddCommand(restackCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(submitCmd)
	rootCmd.AddCommand(mergeCmd)
}
