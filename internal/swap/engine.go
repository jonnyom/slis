package swap

import (
	"errors"
	"fmt"
	"strings"
	"time"

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
// if any repo fails, all already-activated repos are rolled back. The journal
// is written incrementally after each successful activateRepo, so that a crash
// mid-swap still leaves a recoverable journal. On full success it runs
// dep-reconcile.
//
// Dep-reconcile errors are non-fatal (the swap itself succeeded and the journal
// is saved); they are returned alongside a non-nil journal so the caller can
// warn the user.
func Activate(slice string, repos []RepoActivation, journalPath string, opts ActivateOptions) (*Journal, error) {
	j := &Journal{Slice: slice}

	// Phase 1: activate each repo; roll back all on first failure.
	for _, ra := range repos {
		st, err := activateRepo(RepoPlan{
			Repo:    ra.Repo,
			Primary: ra.Primary,
			Branch:  ra.Branch,
			Stash:   opts.Stash,
		})
		if err != nil {
			activateErr := fmt.Errorf("activate %q: %w", ra.Repo, err)
			failed, rbErrs := rollback(j.Repos)
			if len(failed) == 0 {
				// All rollbacks succeeded — clean slate.
				_ = Clear(journalPath)
				return nil, activateErr
			}
			// Some repos failed to roll back — record them in the journal so the
			// user can resume with `slis deactivate`.
			j.Repos = failed
			_ = Save(journalPath, j) // best-effort
			return nil, errors.Join(append([]error{activateErr}, rbErrs...)...)
		}

		// Fix A: write the journal incrementally after each successful activation
		// so that a crash or later failure still leaves a recoverable record.
		j.Repos = append(j.Repos, st)
		if err := Save(journalPath, j); err != nil {
			// Treat a save failure the same as an activation failure.
			saveErr := fmt.Errorf("save journal after activating %q: %w", ra.Repo, err)
			failed, rbErrs := rollback(j.Repos)
			if len(failed) == 0 {
				_ = Clear(journalPath)
				return nil, saveErr
			}
			j.Repos = failed
			_ = Save(journalPath, j)
			return nil, errors.Join(append([]error{saveErr}, rbErrs...)...)
		}
	}

	// Phase 2: dep-reconcile (only when Installer is set).
	var reconcileErr error
	if opts.Installer != nil {
		for i, ra := range repos {
			if len(ra.Lockfiles) == 0 {
				continue
			}
			changed, err := LockfilesChanged(ra.Primary, j.Repos[i].PriorSHA, j.Repos[i].TargetSHA, ra.Lockfiles)
			if err != nil {
				if reconcileErr == nil {
					reconcileErr = fmt.Errorf("lockfiles-changed %q: %w", ra.Repo, err)
				}
				continue
			}
			if changed {
				if e := opts.Installer(ra.Primary); e != nil {
					// Fix F: only set Reconciled=true on installer SUCCESS.
					if reconcileErr == nil {
						reconcileErr = fmt.Errorf("installer %q: %w", ra.Repo, e)
					}
				} else {
					// Fix F: installer succeeded — mark reconciled.
					j.Repos[i].Reconciled = true
				}
			}
		}
	}

	// Phase 3: write final journal state (with Reconciled flags set).
	if err := Save(journalPath, j); err != nil {
		return nil, err
	}

	return j, reconcileErr
}

// rollback deactivates the given RepoStates in REVERSE order, collecting
// errors. It returns (failedStates, errors) where failedStates contains only
// the repos that failed to deactivate (still in an indeterminate state).
func rollback(states []RepoState) (failed []RepoState, errs []error) {
	for i := len(states) - 1; i >= 0; i-- {
		if err := deactivateRepo(states[i]); err != nil {
			failed = append(failed, states[i])
			errs = append(errs, fmt.Errorf("rollback %q: %w", states[i].Repo, err))
		}
	}
	return failed, errs
}

// Deactivate restores every repo recorded in the journal (best-effort: it
// continues past ErrStashConflict and other per-repo errors, aggregating them).
// If all repos deactivate successfully, the journal is cleared. If any repo
// fails, the journal is updated to contain only the failed repos so that
// `slis deactivate` can be re-run to resume.
func Deactivate(journalPath string) error {
	j, err := Load(journalPath)
	if err != nil {
		return err
	}
	if j == nil {
		return nil // nothing active
	}

	// Fix D: collect failed repos; only clear the journal when all succeeded.
	var errs []error
	var failed []RepoState
	for _, st := range j.Repos {
		if e := deactivateRepo(st); e != nil {
			errs = append(errs, e)
			failed = append(failed, st)
		}
	}

	if len(failed) == 0 {
		// All repos restored — clear the journal.
		return Clear(journalPath)
	}

	// Some repos could not be restored — rewrite the journal with only the
	// failing repos so the user can re-run `slis deactivate` to retry.
	j.Repos = failed
	_ = Save(journalPath, j) // best-effort
	return errors.Join(errs...)
}

// RecoverState reads the journal at journalPath and returns the in-progress
// *Journal, or (nil, nil) if no swap is currently active. A non-nil result
// means a swap is recorded on disk — it may have been left behind by a crash
// mid-activation. The caller should inspect the journal and decide whether to
// resume or call Deactivate to roll back.
func RecoverState(journalPath string) (*Journal, error) {
	return Load(journalPath)
}

// Refresh re-resolves each member branch's tip and advances the primary's
// detached HEAD to the new tip when the branch has received new commits since
// the last Activate (or previous Refresh). It does NOT touch stashes, prior
// branches, worktrees, or use --force. On success the journal's TargetSHA
// fields are updated both in-memory and on disk.
//
// If no journal exists, Refresh returns (nil, nil) — there is nothing to
// refresh.
func Refresh(journalPath string) (*Journal, error) {
	j, err := Load(journalPath)
	if err != nil {
		return nil, err
	}
	if j == nil {
		return nil, nil // nothing active
	}

	changed := false
	for i := range j.Repos {
		rs := &j.Repos[i]
		newTarget, err := git.RevParse(rs.Primary, rs.Branch)
		if err != nil {
			return nil, fmt.Errorf("refresh rev-parse %q in %q: %w", rs.Branch, rs.Primary, err)
		}
		if newTarget == rs.TargetSHA {
			continue
		}
		// Advance the detached primary to the branch's new tip.
		if _, err := git.Run(rs.Primary, "switch", "--detach", newTarget); err != nil {
			return nil, fmt.Errorf("refresh switch --detach %q in %q: %w", newTarget, rs.Primary, err)
		}
		rs.TargetSHA = newTarget
		changed = true
	}

	if changed {
		if err := Save(journalPath, j); err != nil {
			return nil, err
		}
	}

	return j, nil
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
//  4. Stash if allowed; record the pinned stash SHA and unique message.
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

	var stashRef, stashMsg string
	if dirty {
		if !plan.Stash {
			// Return error with zero state changes; HEAD is still at priorSHA.
			return RepoState{}, fmt.Errorf("primary %q is dirty; pass --stash to proceed", plan.Primary)
		}

		// Fix F: use a unique stash message per activation to disambiguate.
		repoLabel := plan.Branch
		if plan.Repo != "" {
			repoLabel = plan.Repo + ":" + plan.Branch
		}
		stashMsg = fmt.Sprintf("slis:auto:%s:%d", repoLabel, time.Now().UnixNano())

		// 4. Auto-stash: push all untracked files too (-u) with a recognisable
		//    label.  Then pin the stash to its exact commit SHA so a future
		//    restore can find this specific stash even if other stashes exist.
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
		Branch:      plan.Branch,
		PriorBranch: prior,
		PriorSHA:    priorSHA,
		StashRef:    stashRef,
		StashMsg:    stashMsg,
		TargetSHA:   target,
	}, nil
}

// deactivateRepo restores the primary checkout to its state before activateRepo
// was called. It is the exact inverse of activateRepo:
//
//  1. Switch the primary back to the prior branch (or detach at the prior SHA
//     if the primary was detached before activation). If the branch switch
//     fails, fall back to a detached-SHA restore so the stash pop can still
//     proceed.
//  2. If a stash was saved during activation, locate it by its pinned SHA (and
//     optionally message) and pop THAT exact entry. On conflict, return
//     ErrStashConflict without dropping the stash (the user must resolve and
//     pop manually).
//
// deactivateRepo never uses --force and never drops/clears the stash on the
// conflict path.
func deactivateRepo(st RepoState) error {
	// 1. Restore the branch (or detached HEAD if prior was detached).
	// Fix C: if restoring the prior branch fails, fall back to detached-SHA
	// restore so we can still pop the stash.
	switched := false
	if st.PriorBranch != "" {
		if _, err := git.Run(st.Primary, "switch", st.PriorBranch); err != nil {
			// Branch switch failed (e.g. branch checked out in another worktree).
			// Fall back to detached restore at the prior SHA.
			if _, err2 := git.Run(st.Primary, "switch", "--detach", st.PriorSHA); err2 != nil {
				// Both failed — cannot safely touch the stash.
				return fmt.Errorf("switch to prior branch %q in %q: %w (detach fallback also failed: %v)", st.PriorBranch, st.Primary, err, err2)
			}
			switched = true
		} else {
			switched = true
		}
	} else {
		if _, err := git.Run(st.Primary, "switch", "--detach", st.PriorSHA); err != nil {
			return fmt.Errorf("switch --detach to prior SHA %q in %q: %w", st.PriorSHA, st.Primary, err)
		}
		switched = true
	}

	// No stash to restore — done.
	if !switched || st.StashRef == "" {
		return nil
	}

	// 2. Locate the exact stash entry.
	// Fix F: match by SHA AND message (when StashMsg is present) to avoid
	// ambiguity if multiple stash entries happen to share the same commit SHA.
	out, err := git.Run(st.Primary, "stash", "list", "--format=%H %gs")
	if err != nil {
		return fmt.Errorf("stash list in %q: %w", st.Primary, err)
	}

	index := -1
	for i, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		// Format: "<sha> <subject>"
		parts := strings.SplitN(line, " ", 2)
		sha := parts[0]
		subject := ""
		if len(parts) == 2 {
			subject = parts[1]
		}
		if sha != st.StashRef {
			continue
		}
		// SHA matches. If we have a stored message, verify the subject also
		// contains it (git stores stash subject as "On <branch>: <msg>").
		if st.StashMsg == "" || strings.Contains(subject, st.StashMsg) {
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
