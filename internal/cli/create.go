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

// validateSliceName rejects slice names that would be unsafe once interpolated
// into a filesystem path (worktree location), a git branch, or a tmux session.
// Internal '/' is allowed because a slice name is a branch minus its strip-prefix
// and branches legitimately nest (e.g. "feat/sub"); what is rejected is anything
// that could escape the worktrees directory or be parsed as a git flag:
//   - empty
//   - a leading '-' (would look like a git option)
//   - an absolute path
//   - a '.' or '..' (or empty) path segment — i.e. traversal or // or leading/trailing '/'
//   - any backslash or ASCII control character
func validateSliceName(name string) error {
	if name == "" {
		return fmt.Errorf("slice name must not be empty")
	}
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("slice name %q must not start with '-'", name)
	}
	if filepath.IsAbs(name) {
		return fmt.Errorf("slice name %q must not be an absolute path", name)
	}
	if strings.ContainsRune(name, '\\') {
		return fmt.Errorf("slice name %q must not contain a backslash", name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("slice name %q must not contain control characters", name)
		}
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("slice name %q contains an invalid path segment %q", name, seg)
		}
	}
	return nil
}

// branchForSlice forms the git branch name for a slice, applying the workspace's
// strip_prefix EXACTLY ONCE. The slice name may already be fully-qualified — e.g.
// when `slis create` is handed a real branch name like "jonny/wfm-123" — and
// blindly prepending the prefix produced malformed doubled branches such as
// "jonnyjonny/wfm-123" (which then match no PR and aren't in Graphite). If the
// name already starts with the prefix, it is used as-is.
func branchForSlice(stripPrefix, slice string) string {
	if stripPrefix == "" || strings.HasPrefix(slice, stripPrefix) {
		return slice
	}
	return stripPrefix + slice
}

// worktreePlan computes the branch name and worktree path for each repo in the
// workspace. name is the canonical slice name (strip_prefix already removed);
// branch is the fully-qualified git branch (strip_prefix applied exactly once).
// Keeping the prefix off the slice name means the worktree path, tmux session,
// and display stay clean even when create was handed a full branch name. Pure
// (no git calls) so it can be unit-tested.
func worktreePlan(ws config.Workspace, name, branch string) []struct {
	Repo, Primary, Branch, Path string
} {
	result := make([]struct{ Repo, Primary, Branch, Path string }, 0, len(ws.Repos))
	for repoName, repo := range ws.Repos {
		wtPath := filepath.Join(ws.Root, ".slis", "worktrees", name, repoName)
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
		rawName := args[0]
		noWorktrees, _ := cmd.Flags().GetBool("no-worktrees")

		if err := validateSliceName(rawName); err != nil {
			return err
		}

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		// A name pasted from Linear is usually a full branch name (e.g.
		// "jonny/wfm-123-..."). Canonicalise: the slice identity (path, session,
		// display) drops the strip_prefix, while the branch keeps it exactly once.
		sliceName := config.SliceNameFromBranch(rawName, ws.Grouping.StripPrefix)
		branch := branchForSlice(ws.Grouping.StripPrefix, rawName)

		plans := worktreePlan(ws, sliceName, branch)

		for _, p := range plans {
			if noWorktrees {
				fmt.Printf("would create worktree for %s at %s (branch: %s)\n", p.Repo, p.Path, p.Branch)
				continue
			}

			// Try creating a new branch + worktree. The "--" separates options
			// from the positional path so a path is never parsed as a git flag.
			_, err := git.Run(p.Primary, "worktree", "add", "-b", p.Branch, "--", p.Path)
			if err != nil {
				// Branch may already exist; try attaching to the existing branch.
				errStr := err.Error()
				if strings.Contains(errStr, "already exists") || strings.Contains(errStr, "already checked out") {
					_, err2 := git.Run(p.Primary, "worktree", "add", "--", p.Path, p.Branch)
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
				if err := tmuxctl.EnsureSession(sliceName, members, tmuxctl.SessionOpts{Root: ws.Root, Layout: ws.Sessions.Layout}); err != nil {
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
