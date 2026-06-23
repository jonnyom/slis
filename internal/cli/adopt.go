package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// checkedOutElsewhere reports whether a `git worktree add` failure was because
// the branch is already checked out (in the primary or another worktree).
func checkedOutElsewhere(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already used by worktree") ||
		strings.Contains(msg, "already checked out")
}

var adoptCmd = &cobra.Command{
	Use:   "adopt <branch>",
	Short: "Adopt an existing branch into a managed slis slice (creates worktrees)",
	Long: `adopt creates slis-managed worktrees for a branch that already exists — work
you started in a primary checkout, or a branch already pushed to origin — so the
slice shows up in the hub with the right diff and PR.

For each repo that has the branch (locally or on origin) a worktree is created
at .slis/worktrees/<slice>/<repo>. A repo where the branch is currently checked
out elsewhere (e.g. the primary) is skipped with a note — git won't check the
same branch out twice. The branch is taken as given; strip_prefix is applied
exactly once (a fully-qualified name like "jonny/wfm-1" is fine).`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw := args[0]
		noSession, _ := cmd.Flags().GetBool("no-session")

		if err := validateSliceName(raw); err != nil {
			return err
		}

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		prefix := ws.Grouping.StripPrefix
		sliceName := config.SliceNameFromBranch(raw, prefix)
		branch := branchForSlice(prefix, raw)
		plans := worktreePlan(ws, sliceName, branch)

		var members []model.SliceMember
		for _, p := range plans {
			hasLocal := git.RefExists(p.Primary, "refs/heads/"+p.Branch)
			hasRemote := git.RefExists(p.Primary, "refs/remotes/origin/"+p.Branch)

			switch {
			case hasLocal:
				if _, err := git.Run(p.Primary, "worktree", "add", "--", p.Path, p.Branch); err != nil {
					if checkedOutElsewhere(err) {
						fmt.Printf("slis: %s — branch %q is checked out elsewhere (the primary?); switch it off there and re-run, or keep working in the primary\n", p.Repo, p.Branch)
					} else {
						fmt.Printf("slis: %s — could not adopt: %v\n", p.Repo, err)
					}
					continue
				}
				fmt.Printf("adopted %s at %s (branch: %s)\n", p.Repo, p.Path, p.Branch)
				members = append(members, model.SliceMember{Repo: p.Repo, WorktreePath: p.Path})

			case hasRemote:
				if _, err := git.Run(p.Primary, "worktree", "add", "-b", p.Branch, "--", p.Path, "origin/"+p.Branch); err != nil {
					fmt.Printf("slis: %s — could not adopt from origin: %v\n", p.Repo, err)
					continue
				}
				fmt.Printf("adopted %s at %s (branch: %s, tracking origin)\n", p.Repo, p.Path, p.Branch)
				members = append(members, model.SliceMember{Repo: p.Repo, WorktreePath: p.Path})

			default:
				fmt.Printf("slis: %s — no branch %q locally or on origin (skipping)\n", p.Repo, p.Branch)
			}
		}

		if len(members) == 0 {
			return fmt.Errorf("nothing adopted: no repo had branch %q free to check out", branch)
		}

		if !noSession {
			if !tmuxctl.Available() {
				fmt.Println("note: tmux not found — skipping session creation")
			} else if err := tmuxctl.EnsureSession(sliceName, members, tmuxctl.SessionOpts{Root: ws.Root, Layout: ws.Sessions.Layout}); err != nil {
				fmt.Printf("note: could not start tmux session: %v\n", err)
			} else {
				fmt.Printf("started tmux session slis/%s\n", sliceName)
			}
		}

		return nil
	},
}

func init() {
	adoptCmd.Flags().Bool("no-session", false, "Do not create a tmux session for the adopted slice")
	rootCmd.AddCommand(adoptCmd)
}
