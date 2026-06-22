package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
	"github.com/spf13/cobra"
)

// worktreePlan computes the branch name and worktree path for each repo in the
// workspace for the given slice name. It is a pure function (no git calls) so
// it can be unit-tested without any side-effects.
func worktreePlan(ws config.Workspace, slice string) []struct {
	Repo, Primary, Branch, Path string
} {
	result := make([]struct{ Repo, Primary, Branch, Path string }, 0, len(ws.Repos))
	for repoName, repo := range ws.Repos {
		branch := ws.Grouping.StripPrefix + slice
		wtPath := filepath.Join(ws.Root, ".slis", "worktrees", slice, repoName)
		result = append(result, struct{ Repo, Primary, Branch, Path string }{
			Repo:    repoName,
			Primary: repo.Primary,
			Branch:  branch,
			Path:    wtPath,
		})
	}
	return result
}

var createCmd = &cobra.Command{
	Use:   "create <slice>",
	Short: "Create worktrees for all repos in a new slice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		sliceName := args[0]
		noWorktrees, _ := cmd.Flags().GetBool("no-worktrees")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		plans := worktreePlan(ws, sliceName)

		for _, p := range plans {
			if noWorktrees {
				fmt.Printf("would create worktree for %s at %s (branch: %s)\n", p.Repo, p.Path, p.Branch)
				continue
			}

			// Try creating a new branch + worktree.
			_, err := git.Run(p.Primary, "worktree", "add", "-b", p.Branch, p.Path)
			if err != nil {
				// Branch may already exist; try attaching to the existing branch.
				errStr := err.Error()
				if strings.Contains(errStr, "already exists") || strings.Contains(errStr, "already checked out") {
					_, err2 := git.Run(p.Primary, "worktree", "add", p.Path, p.Branch)
					if err2 != nil {
						fmt.Printf("slis: skipping %s — worktree already exists or branch in use: %v\n", p.Repo, err2)
						continue
					}
				} else {
					fmt.Printf("slis: skipping %s — %v\n", p.Repo, err)
					continue
				}
			}

			fmt.Printf("created worktree for %s at %s (branch: %s)\n", p.Repo, p.Path, p.Branch)
		}

		// Start a tmux session for the new slice (best-effort; skip if tmux is absent).
		if !noWorktrees {
			if !tmuxctl.Available() {
				fmt.Println("note: tmux not found — skipping session creation")
			} else {
				members := make([]model.SliceMember, 0, len(plans))
				for _, p := range plans {
					members = append(members, model.SliceMember{
						Repo:         p.Repo,
						WorktreePath: p.Path,
					})
				}
				if err := tmuxctl.EnsureSession(sliceName, members); err != nil {
					fmt.Printf("note: could not start tmux session: %v\n", err)
				} else {
					fmt.Printf("started tmux session slis/%s\n", sliceName)
				}
			}
		}

		return nil
	},
}

func init() {
	createCmd.Flags().Bool("no-worktrees", false, "Print what would be created without running git")
	rootCmd.AddCommand(createCmd)
}
