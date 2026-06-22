package swap

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jonnyom/slis/internal/git"
)

// RepoActivation is one repo's activation input. The CLI layer builds these
// from the workspace config and a model.Slice.
type RepoActivation struct {
	Repo      string   // logical repo name
	Primary   string   // primary checkout dir (absolute path)
	Branch    string   // slice branch to activate
	Lockfiles []string // lockfiles to diff for dep-reconcile (may be empty)
}

// ActivateOptions controls optional behaviour during Activate.
type ActivateOptions struct {
	// Stash auto-stashes a dirty primary before switching (passed through to
	// each activateRepo call).
	Stash bool
	// Installer is called for a repo when its lockfiles changed between the
	// prior HEAD and the slice branch tip. If nil, dep-reconcile is skipped.
	Installer func(primaryDir string) error
}

// Activate switches every repo's primary to its slice branch tip atomically:
// if any repo fails, all already-activated repos are rolled back and no journal
// is written. On full success it writes the journal and runs dep-reconcile.
//
// Dep-reconcile errors are non-fatal (the swap itself succeeded and the journal
// is saved); they are returned alongside a non-nil journal so the caller can
// warn the user.
func Activate(slice string, repos []RepoActivation, journalPath string, opts ActivateOptions) (*Journal, error) {
	var done []RepoState

	// Phase 1: activate each repo; roll back all on first failure.
	for _, ra := range repos {
		st, err := activateRepo(RepoPlan{
			Repo:    ra.Repo,
			Primary: ra.Primary,
			Branch:  ra.Branch,
			Stash:   opts.Stash,
		})
		if err != nil {
			// Roll back already-activated repos in reverse order (best-effort).
			for i := len(done) - 1; i >= 0; i-- {
				_ = deactivateRepo(done[i]) // ignore rollback errors
			}
			return nil, fmt.Errorf("activate %q: %w", ra.Repo, err)
		}
		done = append(done, st)
	}

	// Phase 2: dep-reconcile (only when Installer is set).
	var reconcileErr error
	if opts.Installer != nil {
		for i, ra := range repos {
			if len(ra.Lockfiles) == 0 {
				continue
			}
			changed, err := LockfilesChanged(ra.Primary, done[i].PriorSHA, done[i].TargetSHA, ra.Lockfiles)
			if err != nil {
				if reconcileErr == nil {
					reconcileErr = fmt.Errorf("lockfiles-changed %q: %w", ra.Repo, err)
				}
				continue
			}
			if changed {
				if e := opts.Installer(ra.Primary); e != nil {
					if reconcileErr == nil {
						reconcileErr = fmt.Errorf("installer %q: %w", ra.Repo, e)
					}
				}
				done[i].Reconciled = true
			}
		}
	}

	// Phase 3: write journal.
	j := &Journal{Slice: slice, Repos: done}
	if err := Save(journalPath, j); err != nil {
		return nil, err
	}

	return j, reconcileErr
}

// Deactivate restores every repo recorded in the journal (best-effort: it
// continues past ErrStashConflict and other per-repo errors, aggregating them),
// then clears the journal file.
func Deactivate(journalPath string) error {
	j, err := Load(journalPath)
	if err != nil {
		return err
	}
	if j == nil {
		return nil // nothing active
	}

	var errs []error
	for _, st := range j.Repos {
		if e := deactivateRepo(st); e != nil {
			errs = append(errs, e)
		}
	}

	if e := Clear(journalPath); e != nil {
		errs = append(errs, e)
	}

	return errors.Join(errs...)
}

// ErrStashConflict is returned by deactivateRepo when popping the stash
// produces a merge conflict. The stash is intentionally left intact so the
// user can resolve the conflict and pop it manually.
var ErrStashConflict = errors.New("stash pop conflicted; resolve manually (stash left intact)")

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

// deactivateRepo restores the primary checkout to its state before activateRepo
// was called. It is the exact inverse of activateRepo:
//
//  1. Switch the primary back to the prior branch (or detach at the prior SHA
//     if the primary was detached before activation).
//  2. If a stash was saved during activation, locate it by its pinned SHA and
//     pop THAT exact entry. On conflict, return ErrStashConflict without
//     dropping the stash (the user must resolve and pop manually).
//
// deactivateRepo never uses --force and never drops/clears the stash on the
// conflict path.
func deactivateRepo(st RepoState) error {
	// 1. Restore the branch (or detached HEAD if prior was detached).
	if st.PriorBranch != "" {
		if _, err := git.Run(st.Primary, "switch", st.PriorBranch); err != nil {
			return fmt.Errorf("switch to prior branch %q in %q: %w", st.PriorBranch, st.Primary, err)
		}
	} else {
		if _, err := git.Run(st.Primary, "switch", "--detach", st.PriorSHA); err != nil {
			return fmt.Errorf("switch --detach to prior SHA %q in %q: %w", st.PriorSHA, st.Primary, err)
		}
	}

	// No stash to restore — done.
	if st.StashRef == "" {
		return nil
	}

	// 2. Locate the exact stash entry by the pinned commit SHA.
	out, err := git.Run(st.Primary, "stash", "list", "--format=%H")
	if err != nil {
		return fmt.Errorf("stash list in %q: %w", st.Primary, err)
	}

	index := -1
	for i, line := range strings.Split(out, "\n") {
		if line == st.StashRef {
			index = i
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("stash %s not found in %q", st.StashRef, st.Primary)
	}

	// 3. Pop that exact entry. On non-zero exit (conflict), git stash pop has
	//    already applied the changes with conflict markers and left the stash
	//    entry intact — so we just surface the error.
	if _, err := git.Run(st.Primary, "stash", "pop", fmt.Sprintf("stash@{%d}", index)); err != nil {
		return fmt.Errorf("%w: %v", ErrStashConflict, err)
	}

	return nil
}
