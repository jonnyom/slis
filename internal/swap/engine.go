package swap

import (
	"fmt"

	"github.com/jonnyom/slis/internal/git"
)

// RepoPlan describes a single-repo activation request.
type RepoPlan struct {
	Repo    string // logical repo name (may be "" in single-repo use)
	Primary string // absolute path to the primary checkout dir
	Branch  string // slice branch to activate (must exist in the shared object db)
	Stash   bool   // when true, auto-stash a dirty primary before switching
}

// activateRepo puts the primary checkout into a detached HEAD at the tip of
// plan.Branch, leaving the worktree (which holds plan.Branch as a live
// checkout) completely untouched.
//
// Safe ordering:
//  1. Read current state (prior branch, prior SHA).
//  2. Resolve target SHA early so a bad branch name fails with NO state change.
//  3. Dirty-check; if dirty and !Stash → return error (zero changes so far).
//  4. Stash if allowed; record the pinned stash SHA.
//  5. git switch --detach <targetSHA> (commit, not branch name — avoids
//     contending with the worktree's branch checkout).
func activateRepo(plan RepoPlan) (RepoState, error) {
	// 1. Record prior branch and HEAD sha before any mutations.
	prior, err := git.CurrentBranch(plan.Primary)
	if err != nil {
		return RepoState{}, fmt.Errorf("currentBranch(%q): %w", plan.Primary, err)
	}

	priorSHA, err := git.RevParse(plan.Primary, "HEAD")
	if err != nil {
		return RepoState{}, fmt.Errorf("rev-parse HEAD in %q: %w", plan.Primary, err)
	}

	// 2. Resolve the target commit SHA early (fail fast on bad branch name,
	//    with zero state changes so far).
	target, err := git.RevParse(plan.Primary, plan.Branch)
	if err != nil {
		return RepoState{}, fmt.Errorf("resolve branch %q in %q: %w", plan.Branch, plan.Primary, err)
	}

	// 3. Dirty-check.
	dirty, err := git.IsDirty(plan.Primary)
	if err != nil {
		return RepoState{}, fmt.Errorf("is-dirty(%q): %w", plan.Primary, err)
	}

	var stashRef string
	if dirty {
		if !plan.Stash {
			// Return error with zero state changes; HEAD is still at priorSHA.
			return RepoState{}, fmt.Errorf("primary %q is dirty; pass --stash to proceed", plan.Primary)
		}

		// 4. Auto-stash: push all untracked files too (-u) with a recognisable
		//    label.  Then pin the stash to its exact commit SHA so a future
		//    restore can find this specific stash even if other stashes exist.
		stashMsg := "slis:auto:" + plan.Branch
		if plan.Repo != "" {
			stashMsg = "slis:auto:" + plan.Repo + ":" + plan.Branch
		}
		if _, err := git.Run(plan.Primary, "stash", "push", "-u", "-m", stashMsg); err != nil {
			return RepoState{}, fmt.Errorf("stash push in %q: %w", plan.Primary, err)
		}

		// Pin the stash to its commit SHA so restore is deterministic.
		stashRef, err = git.RevParse(plan.Primary, "stash@{0}")
		if err != nil {
			return RepoState{}, fmt.Errorf("pin stash@{0} in %q: %w", plan.Primary, err)
		}
	}

	// 5. Detach the primary at the target COMMIT sha, never the branch name.
	//    Git allows checking out a commit that a worktree holds as a branch;
	//    only checking out the *branch* twice is blocked.
	if _, err := git.Run(plan.Primary, "switch", "--detach", target); err != nil {
		return RepoState{}, fmt.Errorf("switch --detach %q in %q: %w", target, plan.Primary, err)
	}

	return RepoState{
		Repo:        plan.Repo,
		Primary:     plan.Primary,
		PriorBranch: prior,
		PriorSHA:    priorSHA,
		StashRef:    stashRef,
		TargetSHA:   target,
	}, nil
}
