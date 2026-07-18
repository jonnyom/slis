package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
	"github.com/spf13/cobra"
)

// trackInGraphite best-effort registers a slis-born branch in Graphite so a
// gt-native repo keeps the new branch in its stack metadata (parent → branch).
// It is a no-op unless the repo is gt-native and a parent is known; a tracking
// failure only prints a warning and never blocks or rolls back the worktree
// that was just created.
func trackInGraphite(dir, branch, parent string) {
	if branch == "" || parent == "" || !gt.Native(dir) {
		return
	}
	if _, err := gt.Track(dir, branch, parent); err != nil {
		fmt.Printf("note: could not track %s in Graphite (parent %s): %v\n", branch, parent, err)
	}
}

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
// and display stay clean even when create was handed a full branch name.
//
// StartPoint is the commit-ish the new branch forks from — the repo's configured
// trunk (default_branch). Forking from trunk (rather than the primary's current
// HEAD, which may be sitting on unrelated in-flight work after a swap) keeps a
// fresh slice's diff empty until the slice itself makes changes. Pure (no git
// calls) so it can be unit-tested.
func worktreePlan(ws config.Workspace, name, branch string) []struct {
	Repo, Primary, Branch, Path, StartPoint string
} {
	result := make([]struct{ Repo, Primary, Branch, Path, StartPoint string }, 0, len(ws.Repos))
	for repoName, repo := range ws.Repos {
		wtPath := filepath.Join(ws.Root, ".slis", "worktrees", name, repoName)
		result = append(result, struct{ Repo, Primary, Branch, Path, StartPoint string }{
			Repo:       repoName,
			Primary:    repo.Primary,
			Branch:     branch,
			Path:       wtPath,
			StartPoint: repo.DefaultBranch,
		})
	}
	return result
}

// trunkStartPoint resolves the freshest trunk commit-ish a new slice's worktree
// should fork from. When fetch is true it first refreshes the remote-tracking
// trunk (best-effort — offline / no remote is ignored). It prefers
// origin/<trunk> (the latest pushed trunk) over the possibly-stale local <trunk>
// branch, and returns "" when neither resolves so the caller falls back to the
// primary's current HEAD.
func trunkStartPoint(primary, trunk string, fetch bool) string {
	if trunk == "" {
		return ""
	}
	if fetch {
		_, _ = git.Run(primary, "fetch", "origin", trunk) // best-effort; ignore offline/no-remote
	}
	if _, err := git.Run(primary, "rev-parse", "--verify", "--quiet", "origin/"+trunk); err == nil {
		return "origin/" + trunk
	}
	if _, err := git.Run(primary, "rev-parse", "--verify", "--quiet", trunk); err == nil {
		return trunk
	}
	return ""
}

// createFreshWorktree creates branch at start and checks it out at path. A
// local branch with the requested name may be left behind by an older, already
// merged slice. Reusing that ref verbatim would resurrect the old slice (and
// any historical PR attached to its branch name), so recycle it to the fresh
// trunk start only when Git proves its current tip is contained there. A
// divergent branch is preserved and rejected; `slis adopt` is the explicit
// operation for existing work.
func createFreshWorktree(primary, path, branch, start, mergedPRHead string) error {
	effectiveStart := start
	if effectiveStart == "" {
		effectiveStart = "HEAD"
	}

	if !git.RefExists(primary, "refs/heads/"+branch) {
		_, err := git.Run(primary, "worktree", "add", "-b", branch, "--", path, effectiveStart)
		return err
	}

	localTip, _ := git.RevParse(primary, "refs/heads/"+branch)
	mergedHistorically := mergedPRHead != "" && localTip == mergedPRHead
	if !git.IsMergedInto(primary, branch, effectiveStart) && !mergedHistorically {
		return fmt.Errorf("branch %q already exists with work not merged into %s; use `slis adopt %s` to keep that work, or choose a different slice name", branch, effectiveStart, branch)
	}

	// The old ref is fully contained in trunk, so moving it cannot orphan any
	// commits. This also fails safely when the branch is checked out elsewhere.
	if _, err := git.Run(primary, "branch", "-f", "--", branch, effectiveStart); err != nil {
		return fmt.Errorf("recycle merged branch %q at %s: %w", branch, effectiveStart, err)
	}
	if _, err := git.Run(primary, "worktree", "add", "--", path, branch); err != nil {
		return fmt.Errorf("check out recycled branch %q: %w", branch, err)
	}
	return nil
}

// mergedPRHead returns the immutable head commit recorded for a merged PR with
// this branch name. Callers compare it with the current local tip before
// recycling: a historical PR with the same name is not enough on its own,
// because the branch may already have been reused for newer work.
func mergedPRHead(primary, branch string) string {
	pr, _ := forge.PRForBranch(primary, branch)
	if pr == nil || !strings.EqualFold(pr.State, "MERGED") {
		return ""
	}
	return pr.HeadSHA
}

var createCmd = &cobra.Command{
	Use:   "create <slice>",
	Short: "Create worktrees for all repos in a new slice",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawName := args[0]
		noWorktrees, _ := cmd.Flags().GetBool("no-worktrees")
		noFetch, _ := cmd.Flags().GetBool("no-fetch")

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
		createdPlans := make([]struct{ Repo, Primary, Branch, Path, StartPoint string }, 0, len(plans))

		for _, p := range plans {
			// Resolve the freshest trunk to fork from (fetches origin trunk unless
			// --no-fetch / dry-run). Forking from trunk — not the primary's current
			// HEAD — keeps the slice from inheriting unrelated in-flight commits.
			start := trunkStartPoint(p.Primary, p.StartPoint, !noWorktrees && !noFetch)

			if noWorktrees {
				from := start
				if from == "" {
					from = "current HEAD"
				}
				fmt.Printf("would create worktree for %s at %s (branch: %s, from: %s)\n", p.Repo, p.Path, p.Branch, from)
				continue
			}

			historicalHead := ""
			if git.RefExists(p.Primary, "refs/heads/"+p.Branch) && !git.IsMergedInto(p.Primary, p.Branch, start) {
				historicalHead = mergedPRHead(p.Primary, p.Branch)
			}
			if err := createFreshWorktree(p.Primary, p.Path, p.Branch, start, historicalHead); err != nil {
				fmt.Printf("slis: skipping %s — %v\n", p.Repo, err)
				continue
			}

			fmt.Printf("created worktree for %s at %s (branch: %s)\n", p.Repo, p.Path, p.Branch)
			createdPlans = append(createdPlans, p)

			// Both a brand-new branch and a safely recycled merged branch are
			// slis-born from trunk. Track either one best-effort in gt-native repos.
			trackInGraphite(p.Path, p.Branch, p.StartPoint)
		}

		// Persist exactly the worktrees that were actually created. This keeps a
		// partial create honest and gives future cleanup a durable path identity.
		if !noWorktrees && len(createdPlans) > 0 {
			regPath := config.StatePaths().Registry
			reg, _, loadErr := config.LoadRegistry(regPath)
			if loadErr != nil {
				fmt.Printf("note: could not read slice registry: %v\n", loadErr)
			} else {
				for _, p := range createdPlans {
					reg.RegisterCreated(sliceName, p.Repo, p.Branch, p.Path)
				}
				if saveErr := config.SaveRegistry(regPath, reg); saveErr != nil {
					fmt.Printf("note: could not save slice registry: %v\n", saveErr)
				}
			}
		}

		// Start a tmux session for the new slice (best-effort; skip if tmux is absent).
		if !noWorktrees && len(createdPlans) > 0 {
			if !tmuxctl.Available() {
				fmt.Println("note: tmux not found — skipping session creation")
			} else {
				members := make([]model.SliceMember, 0, len(createdPlans))
				for _, p := range createdPlans {
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
	createCmd.Flags().Bool("no-fetch", false, "Skip fetching origin trunk before forking new worktrees")
	rootCmd.AddCommand(createCmd)
}
