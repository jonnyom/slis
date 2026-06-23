package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
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

// addWorktree runs `git worktree add -- <path> <branch>` against primary.
func addWorktree(primary, path, branch string) error {
	_, err := git.Run(primary, "worktree", "add", "--", path, branch)
	return err
}

// freePrimaryBranch detaches the primary so a branch it has checked out can be
// moved into a worktree. It only proceeds when the primary is actually ON that
// branch. Uncommitted work is stashed first (pinned with a slis-adopt message)
// and the stash's SHA is returned so the caller can pop it in the new worktree.
// Returns (freed, stashRef, message); message explains a no-op (empty = not the
// primary). The stash makes the operation fully recoverable: on any later
// failure the work is preserved in `git stash`.
func freePrimaryBranch(primary, branch string) (freed bool, stashRef, msg string) {
	cur, _ := git.CurrentBranch(primary)
	if cur != branch {
		// Checked out in another worktree, not this primary — leave it alone.
		return false, "", ""
	}
	dirty, err := git.IsDirty(primary)
	if err != nil {
		return false, "", fmt.Sprintf("could not check primary status: %v", err)
	}
	if dirty {
		if _, err := git.Run(primary, "stash", "push", "-u", "-m", "slis-adopt:"+branch); err != nil {
			return false, "", fmt.Sprintf("could not stash uncommitted changes in primary: %v", err)
		}
		if sha, err := git.Run(primary, "rev-parse", "stash@{0}"); err == nil {
			stashRef = sha
		}
	}
	if _, err := git.Run(primary, "switch", "--detach"); err != nil {
		return false, stashRef, fmt.Sprintf("could not detach primary: %v", err)
	}
	if stashRef != "" {
		return true, stashRef, fmt.Sprintf("detached primary off %q (stashed your uncommitted work to move it)", branch)
	}
	return true, "", fmt.Sprintf("detached primary off %q to free it for a worktree", branch)
}

// popStash pops the stash entry whose commit is sha (created by
// freePrimaryBranch) into the worktree at dir, so the moved work lands there.
// Falls back to the top stash if the SHA can't be located.
func popStash(dir, sha string) error {
	ref := "" // resolve sha → stash@{n}
	if out, err := git.Run(dir, "stash", "list", "--format=%H %gd"); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if f := strings.Fields(line); len(f) >= 2 && f[0] == sha {
				ref = f[1]
				break
			}
		}
	}
	args := []string{"stash", "pop"}
	if ref != "" {
		args = append(args, ref)
	}
	_, err := git.Run(dir, args...)
	return err
}

// repoTrunk returns the branch to start a new slice branch from: the repo's
// configured default_branch, else its detected trunk.
func repoTrunk(ws config.Workspace, repo, primary string) string {
	if t := ws.Repos[repo].DefaultBranch; t != "" {
		return t
	}
	return git.DetectBase(primary)
}

// adoptBranch creates managed worktrees for an existing branch (the core of
// `slis adopt`, shared by the direct and interactive paths). With move=true a
// branch checked out in a primary is freed (stashing any uncommitted work)
// before the worktree is created. With createMissing=true, repos that don't have
// the branch get it created off their trunk so the slice spans every repo.
func adoptBranch(ws config.Workspace, raw string, noSession, move, createMissing bool) error {
	prefix := ws.Grouping.StripPrefix
	sliceName := config.SliceNameFromBranch(raw, prefix)
	branch := branchForSlice(prefix, raw)
	plans := worktreePlan(ws, sliceName, branch)

	var members []model.SliceMember
	blockedByPrimary := false
	for _, p := range plans {
		hasLocal := git.RefExists(p.Primary, "refs/heads/"+p.Branch)
		hasRemote := git.RefExists(p.Primary, "refs/remotes/origin/"+p.Branch)

		switch {
		case hasLocal:
			err := addWorktree(p.Primary, p.Path, p.Branch)
			explained := false
			if err != nil && checkedOutElsewhere(err) && move {
				freed, stashRef, msg := freePrimaryBranch(p.Primary, p.Branch)
				if msg != "" {
					fmt.Printf("slis: %s — %s\n", p.Repo, msg)
					explained = true
				}
				if freed {
					err = addWorktree(p.Primary, p.Path, p.Branch)
					explained = false
					if err == nil && stashRef != "" {
						if perr := popStash(p.Path, stashRef); perr != nil {
							fmt.Printf("slis: %s — adopted, but your stashed work needs attention (resolve in the worktree; see `git stash list`): %v\n", p.Repo, perr)
						} else {
							fmt.Printf("slis: %s — moved your uncommitted work into the worktree\n", p.Repo)
						}
					}
				}
			}
			if err != nil {
				if checkedOutElsewhere(err) {
					blockedByPrimary = true
					if !explained {
						fmt.Printf("slis: %s — branch %q is checked out elsewhere; switch the primary off it, or use `slis adopt --move` (clean primary only)\n", p.Repo, p.Branch)
					}
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
			if !createMissing {
				fmt.Printf("slis: %s — no branch %q (skipping; --create-missing to start it here)\n", p.Repo, p.Branch)
				continue
			}
			base := repoTrunk(ws, p.Repo, p.Primary)
			if _, err := git.Run(p.Primary, "worktree", "add", "-b", p.Branch, "--", p.Path, base); err != nil {
				fmt.Printf("slis: %s — could not create branch %q off %q: %v\n", p.Repo, p.Branch, base, err)
				continue
			}
			fmt.Printf("created %s at %s (new branch %q off %s)\n", p.Repo, p.Path, p.Branch, base)
			members = append(members, model.SliceMember{Repo: p.Repo, WorktreePath: p.Path})
		}
	}

	if len(members) == 0 {
		if blockedByPrimary {
			return fmt.Errorf("branch %q is checked out in a primary checkout — commit or stash the work there and `git switch` off the branch, then re-run (--move auto-detaches a clean primary)", branch)
		}
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
}

// adoptCandidate is a branch that could be adopted, grouped by slice name with
// the repos that have it.
type adoptCandidate struct {
	Slice  string
	Branch string
	Repos  []string
}

// isTrunkBranch reports whether b is a repo's trunk (its configured default, or
// a conventional trunk name) and therefore not an adoption candidate.
func isTrunkBranch(b, defaultBranch string) bool {
	if b == defaultBranch {
		return true
	}
	switch b {
	case "main", "master", "develop", "trunk":
		return true
	}
	return false
}

// buildAdoptCandidates groups local branches across repos into adopt candidates,
// excluding trunk branches and branches already managed in a slis worktree. Pure
// (git/IO done by the caller) so it is unit-testable.
func buildAdoptCandidates(prefix string, perRepo map[string][]string, trunks map[string]string, managed map[string]bool) []adoptCandidate {
	byBranch := map[string]*adoptCandidate{}
	for repo, branches := range perRepo {
		for _, b := range branches {
			if isTrunkBranch(b, trunks[repo]) || managed[b] {
				continue
			}
			c, ok := byBranch[b]
			if !ok {
				c = &adoptCandidate{Slice: config.SliceNameFromBranch(b, prefix), Branch: b}
				byBranch[b] = c
			}
			c.Repos = append(c.Repos, repo)
		}
	}
	out := make([]adoptCandidate, 0, len(byBranch))
	for _, c := range byBranch {
		sort.Strings(c.Repos)
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slice < out[j].Slice })
	return out
}

// gatherAdoptCandidates collects adopt candidates for the workspace (lists each
// repo's local branches, excludes trunk + already-managed branches).
func gatherAdoptCandidates(ws config.Workspace) ([]adoptCandidate, error) {
	sp := config.StatePaths()
	managed := map[string]bool{}
	if dtos, err := listSlices(ws, sp.Overrides, sp.ActiveJournal); err == nil {
		for _, d := range dtos {
			for _, m := range d.Members {
				managed[m.Branch] = true
			}
		}
	}

	perRepo := map[string][]string{}
	trunks := map[string]string{}
	for name, repo := range ws.Repos {
		trunks[name] = repo.DefaultBranch
		branches, err := git.LocalBranches(repo.Primary)
		if err != nil {
			return nil, fmt.Errorf("listing branches in %s: %w", name, err)
		}
		perRepo[name] = branches
	}
	return buildAdoptCandidates(ws.Grouping.StripPrefix, perRepo, trunks, managed), nil
}

// pickAdoptCandidate shows an interactive picker: choose a branch, and choose
// whether to free a clean primary that has it (the --move behaviour). Returns
// the chosen branch ("" if cancelled) and the move choice.
func pickAdoptCandidate(candidates []adoptCandidate) (branch string, move, createMissing bool, err error) {
	options := make([]huh.Option[string], len(candidates))
	for i, c := range candidates {
		label := fmt.Sprintf("%s  (%s)", c.Slice, strings.Join(c.Repos, ", "))
		options[i] = huh.NewOption(label, c.Branch)
	}
	var chosen string
	var freePrimary, create bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Adopt a branch into a slis slice").
				Options(options...).
				Value(&chosen),
			huh.NewConfirm().
				Title("If the branch is checked out in a primary, detach it (stashing any uncommitted work) so it can move into the worktree?").
				Value(&freePrimary),
			huh.NewConfirm().
				Title("Create the branch in repos that don't have it (so the slice spans every repo)?").
				Value(&create),
		),
	)
	if e := form.Run(); e != nil {
		if errors.Is(e, huh.ErrUserAborted) {
			return "", false, false, nil // cancelled — not an error
		}
		return "", false, false, e
	}
	return chosen, freePrimary, create, nil
}

var adoptCmd = &cobra.Command{
	Use:   "adopt [branch]",
	Short: "Adopt an existing branch into a managed slis slice (creates worktrees)",
	Long: `adopt creates slis-managed worktrees for a branch that already exists — work
you started in a primary checkout, or a branch already pushed to origin — so the
slice shows up in the hub with the right diff and PR.

With no argument, adopt lists the local branches that aren't already slis slices
and lets you pick one interactively.

For each repo that has the branch (locally or on origin) a worktree is created
at .slis/worktrees/<slice>/<repo>. A repo where the branch is currently checked
out elsewhere (e.g. the primary) is skipped with a note — git won't check the
same branch out twice. Pass --move to detach the primary that has the branch so
it can move into the worktree; uncommitted work there is stashed and re-applied
in the new worktree (recoverable via ` + "`git stash`" + `). Pass --create-missing to
also start the branch (off trunk) in repos that don't have it, so the slice
spans every repo. strip_prefix is applied exactly once.`,
	Args:         cobra.MaximumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		noSession, _ := cmd.Flags().GetBool("no-session")
		move, _ := cmd.Flags().GetBool("move")
		createMissing, _ := cmd.Flags().GetBool("create-missing")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		var raw string
		if len(args) == 1 {
			raw = args[0]
		} else {
			candidates, err := gatherAdoptCandidates(ws)
			if err != nil {
				return err
			}
			if len(candidates) == 0 {
				fmt.Println("no adoptable branches found (every local branch is trunk or already a slice)")
				return nil
			}
			picked, pickedMove, pickedCreate, perr := pickAdoptCandidate(candidates)
			if perr != nil {
				return perr
			}
			if picked == "" {
				return nil // nothing picked
			}
			raw = picked
			move = move || pickedMove
			createMissing = createMissing || pickedCreate
		}

		if err := validateSliceName(raw); err != nil {
			return err
		}
		return adoptBranch(ws, raw, noSession, move, createMissing)
	},
}

func init() {
	adoptCmd.Flags().Bool("no-session", false, "Do not create a tmux session for the adopted slice")
	adoptCmd.Flags().Bool("move", false, "Detach the primary holding the branch (stashing any uncommitted work) so it can move into the worktree")
	adoptCmd.Flags().Bool("create-missing", false, "In repos that don't have the branch, create it off trunk so the slice spans every repo")
	rootCmd.AddCommand(adoptCmd)
}
