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
	tempBranch := LiveBranchName(slice)
	for _, ra := range repos {
		st, err := activateRepo(RepoPlan{
			Repo:       ra.Repo,
			Primary:    ra.Primary,
			Branch:     ra.Branch,
			TempBranch: tempBranch,
			Stash:      opts.Stash,
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
		// During rollback each repo's temp branch was just created at its TargetSHA
		// (no drift possible), so deactivateRepo takes the clean path: restore the
		// prior branch and delete the temp branch after re-verifying its tip still
		// equals TargetSHA (provably nothing to lose). force stays false; no rescue
		// branch is created.
		if err := deactivateRepo("", states[i], false); err != nil {
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
//
// Before restoring each repo, Deactivate checks that the primary is still on
// its recorded temp branch (slis/live/<slice>) at the journal's TargetSHA. If it
// drifted (the user switched the primary off the temp branch, or the journal is
// stale), the repo is refused with zero state change — unless force is true. If
// the user committed on the temp branch, those commits are already safe on a
// named branch, so a plain deactivate refuses and lists them; under force the
// temp branch is renamed to `slis/rescue/<slice>-<repo>` (never deleted) before
// the prior branch is restored. Legacy journals written by the old detached-HEAD
// engine (no temp branch recorded) are still restored via the detached-HEAD path.
func Deactivate(journalPath string, force bool) error {
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
		if e := deactivateRepo(j.Slice, st, force); e != nil {
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

// Refresh re-resolves each member branch's tip and fast-forwards the primary's
// temp branch (slis/live/<slice>) to the new tip when the branch has received
// new commits since the last Activate (or previous Refresh). It advances via
// `git merge --ff-only <newTip>` on the temp branch — never a reset — so a
// diverged branch (non-fast-forward) is refused rather than force-moved. It does
// NOT touch stashes, prior branches, worktrees, or use --force. Legacy detached
// journals advance the detached HEAD directly. On success the journal's
// TargetSHA fields are updated both in-memory and on disk.
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

	// Resolve each branch's new tip up front so we know which repos need to
	// advance. Fix F4: refuse to advance any dirty primary — switching would
	// carry the user's uncommitted work onto the new tip or conflict. This
	// validation pass runs BEFORE any switch, so a dirty primary aborts the whole
	// refresh with zero state change (mirrors activate's dirty guard). Refresh has
	// no --stash of its own: any activation stash is already held and popped on
	// deactivate, so a second stash here would be unmanaged; the user must commit
	// or stash their new edits, then re-run refresh.
	newTargets := make([]string, len(j.Repos))
	for i := range j.Repos {
		rs := &j.Repos[i]
		newTarget, err := git.RevParse(rs.Primary, rs.Branch)
		if err != nil {
			return nil, fmt.Errorf("refresh rev-parse %q in %q: %w", rs.Branch, rs.Primary, err)
		}
		newTargets[i] = newTarget
		if newTarget == rs.TargetSHA {
			continue
		}
		dirty, err := git.IsDirty(rs.Primary)
		if err != nil {
			return nil, fmt.Errorf("refresh is-dirty %q: %w", rs.Primary, err)
		}
		if dirty {
			return nil, fmt.Errorf("refresh: primary %q is dirty; commit or stash your changes, then re-run `slis refresh`", rs.Primary)
		}
	}

	changed := false
	for i := range j.Repos {
		rs := &j.Repos[i]
		if newTargets[i] == rs.TargetSHA {
			continue
		}
		if rs.TempBranch == "" {
			// Legacy detached journal: advance the detached HEAD to the new tip.
			if _, err := git.Run(rs.Primary, "switch", "--detach", newTargets[i]); err != nil {
				return nil, fmt.Errorf("refresh switch --detach %q in %q: %w", newTargets[i], rs.Primary, err)
			}
			rs.TargetSHA = newTargets[i]
			changed = true
			continue
		}
		// Temp-branch model: the primary must still be on its temp branch, and the
		// advance is a fast-forward merge (never a reset), so a diverged branch is
		// refused rather than force-moved.
		cur, err := git.CurrentBranch(rs.Primary)
		if err != nil {
			return nil, fmt.Errorf("refresh current-branch %q: %w", rs.Primary, err)
		}
		if cur != rs.TempBranch {
			return nil, fmt.Errorf("refresh: primary %q is no longer on %q (found %q); run `slis deactivate` to restore it, then re-activate",
				rs.Primary, rs.TempBranch, cur)
		}
		if _, err := git.Run(rs.Primary, "merge", "--ff-only", newTargets[i]); err != nil {
			return nil, fmt.Errorf("refresh: cannot fast-forward %q to %s in %q — the branch diverged; run `slis deactivate` and re-activate: %w",
				rs.TempBranch, shortSHA(newTargets[i]), rs.Primary, err)
		}
		rs.TargetSHA = newTargets[i]
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

// StaleRepos returns the names of the journal's repos whose recorded TargetSHA
// no longer matches the branch tip given in tipByRepo (keyed by repo name) —
// i.e. the slice branch has advanced past the primary's temp branch and
// `slis refresh` would fast-forward the primary. A repo missing from tipByRepo
// (no known tip) is skipped. The result is nil when nothing is stale.
func StaleRepos(j *Journal, tipByRepo map[string]string) []string {
	if j == nil {
		return nil
	}
	var stale []string
	for _, rs := range j.Repos {
		tip, ok := tipByRepo[rs.Repo]
		if !ok || tip == "" || rs.TargetSHA == "" {
			continue
		}
		if tip != rs.TargetSHA {
			stale = append(stale, rs.Repo)
		}
	}
	return stale
}

// RepoPlan describes a single-repo activation request.
type RepoPlan struct {
	Repo       string // logical repo name (may be "" in single-repo use)
	Primary    string // absolute path to the primary checkout dir
	Branch     string // slice branch to activate (must exist in the shared object db)
	TempBranch string // the slis/live/<slice> branch to create on the primary
	Stash      bool   // when true, auto-stash a dirty primary before switching
}

// LiveBranchName is the temp branch slis creates on each primary during
// activation: a real, named branch pointed at the slice branch's tip. A named
// branch (rather than a detached HEAD) keeps Graphite usable in the primary,
// never orphans an accidental commit, and doesn't read as a broken checkout.
// LiveBranchName("") yields the "slis/live/" prefix shared by all such branches.
func LiveBranchName(slice string) string {
	return "slis/live/" + slice
}

// activateRepo puts the primary checkout onto a freshly-created temp branch
// (plan.TempBranch, e.g. slis/live/<slice>) pointed at the tip of plan.Branch,
// leaving the worktree (which holds plan.Branch as a live checkout) completely
// untouched. A named branch keeps Graphite usable in the primary and never
// orphans an accidental commit.
//
// Safe ordering:
//  1. Read current state (prior branch, prior SHA).
//  2. Resolve target SHA early so a bad branch name fails with NO state change.
//  3. Refuse (zero state change) when the temp branch already exists — slis
//     never reuses/force-moves it; `slis doctor` cleans a stray one.
//  4. Dirty-check; if dirty and !Stash → return error (zero changes so far).
//  5. Stash if allowed; record the pinned stash SHA and unique message.
//  6. git switch -c <tempBranch> <targetSHA> (create-only -c, never -C/-B — the
//     temp branch name is never held by the worktree, so this cannot contend).
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

	// 3. Refuse if the temp branch already exists — never force-move or reuse it.
	//    This runs before any state change (before stashing), so a refusal leaves
	//    the primary exactly as it was.
	if git.RefExists(plan.Primary, "refs/heads/"+plan.TempBranch) {
		return RepoState{}, fmt.Errorf("temp branch %q already exists in %q — a previous swap may not have been cleaned up; run `slis doctor` (or delete it manually) then retry",
			plan.TempBranch, plan.Primary)
	}

	// 4. Dirty-check.
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

	// 6. Create the temp branch at the target COMMIT sha and check it out.
	//    -c is create-only (fails if the branch exists) — never -C/-B. The temp
	//    branch name is not the worktree's branch, so this cannot contend.
	if _, err := git.Run(plan.Primary, "switch", "-c", plan.TempBranch, target); err != nil {
		return RepoState{}, fmt.Errorf("switch -c %q %q in %q: %w", plan.TempBranch, target, plan.Primary, err)
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
		TempBranch:  plan.TempBranch,
	}, nil
}

// ErrPriorBranchGone is returned when the branch the primary was on before
// activation no longer exists, so deactivateRepo cannot restore it. The user
// must recreate the branch (the error names the exact `git branch` command)
// before deactivating.
var ErrPriorBranchGone = errors.New("prior branch no longer exists")

// ErrPrimaryDrifted is returned when the primary is no longer on its recorded
// temp branch at the journal's TargetSHA, so restoring blindly could lose work.
// Pass force to restore anyway (commits on the temp branch are preserved by
// renaming it to a rescue branch first).
var ErrPrimaryDrifted = errors.New("primary drifted from the recorded slice temp branch")

// rescueBranchName is the branch a forced deactivate renames the temp branch to
// when the user committed on it, so those commits are preserved (never deleted).
func rescueBranchName(slice string, st RepoState) string {
	name := "slis/rescue/" + slice
	if st.Repo != "" {
		name += "-" + st.Repo
	}
	return name
}

// shortSHA truncates a commit SHA to 7 chars for human-facing messages.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// deactivateRepo restores the primary checkout to its state before activateRepo
// was called. It is the inverse of activateRepo.
//
// For temp-branch journals (the current engine) the expected state is "primary
// on st.TempBranch at st.TargetSHA". Deactivation classifies the drift:
//
//   - not on the temp branch (switched away / detached) → refuse with zero state
//     change unless force; under force restore the prior branch and leave the
//     temp branch intact (it is named + safe, and `slis doctor` reports it).
//   - on the temp branch, HEAD == TargetSHA (no new commits) → switch to the
//     prior branch, then delete the temp branch, re-verifying its tip still
//     equals TargetSHA immediately before the `-D` so the delete provably
//     discards nothing.
//   - on the temp branch, HEAD advanced/diverged (the user committed) → refuse
//     with zero state change unless force; under force rename the temp branch to
//     slis/rescue/<slice>-<repo> (never deleted) then restore the prior branch.
//
// A legacy journal (no temp branch recorded) is handled by the original
// detached-HEAD restore path.
//
// In every case the prior branch is restored (or a detached-SHA fallback when it
// is checked out elsewhere); a deleted prior branch is a hard error; and any
// pinned stash is popped by exact entry (pop conflict → ErrStashConflict, stash
// left intact). deactivateRepo never uses a force git switch and never drops or
// clears the stash.
func deactivateRepo(slice string, st RepoState, force bool) error {
	currentHEAD, err := git.RevParse(st.Primary, "HEAD")
	if err != nil {
		return fmt.Errorf("rev-parse HEAD in %q: %w", st.Primary, err)
	}
	currentBranch, err := git.CurrentBranch(st.Primary)
	if err != nil {
		return fmt.Errorf("current-branch in %q: %w", st.Primary, err)
	}

	// Legacy detached-HEAD journal — no temp branch recorded.
	if st.TempBranch == "" {
		return deactivateLegacyDetached(slice, st, force, currentHEAD, currentBranch)
	}

	switch {
	case currentBranch != st.TempBranch:
		// Drifted off the temp branch (switched away or detached).
		if !force {
			where := "detached at " + shortSHA(currentHEAD)
			if currentBranch != "" {
				where = fmt.Sprintf("on branch %q (%s)", currentBranch, shortSHA(currentHEAD))
			}
			return fmt.Errorf("%w in %q: expected primary on %q, but it is %s; re-run `slis deactivate --force` to restore anyway (%q is left intact)",
				ErrPrimaryDrifted, st.Primary, st.TempBranch, where, st.TempBranch)
		}
		// Forced: restore the prior branch; leave the temp branch intact (named +
		// safe — nothing is deleted; `slis doctor` will report the stray branch).
		if err := restorePriorBranch(st); err != nil {
			return err
		}
		return popPinnedStash(st)

	case currentHEAD == st.TargetSHA:
		// Clean: no new commits on the temp branch.
		if err := restorePriorBranch(st); err != nil {
			return err
		}
		if err := deleteTempBranchIfAtSHA(st, st.TargetSHA); err != nil {
			return err
		}
		return popPinnedStash(st)

	default:
		// The user committed on the temp branch — those commits are already safe on
		// a named branch.
		if !force {
			return fmt.Errorf("%w in %q: you committed on %q since activation (%s). Those commits are safe on that branch — graft them onto the slice branch in the worktree, or re-run `slis deactivate --force` to rename %q to %q and restore",
				ErrPrimaryDrifted, st.Primary, st.TempBranch, commitsAhead(st.Primary, st.TargetSHA, currentHEAD), st.TempBranch, rescueBranchName(slice, st))
		}
		// Forced: preserve the commits by renaming the temp branch to a rescue
		// branch (never delete), then restore the prior branch.
		if err := priorBranchGoneErr(st); err != nil {
			return err
		}
		rescue := rescueBranchName(slice, st)
		if git.RefExists(st.Primary, "refs/heads/"+rescue) {
			return fmt.Errorf("rescue branch %q already exists in %q — inspect/remove it, then re-run", rescue, st.Primary)
		}
		if _, err := git.Run(st.Primary, "branch", "-m", "--", st.TempBranch, rescue); err != nil {
			return fmt.Errorf("rename temp branch %q to rescue %q in %q: %w", st.TempBranch, rescue, st.Primary, err)
		}
		if err := restorePriorBranch(st); err != nil {
			return err
		}
		return popPinnedStash(st)
	}
}

// deactivateLegacyDetached restores a primary recorded by the old detached-HEAD
// engine: the expected state is "detached at TargetSHA". Preserved verbatim so
// journals written by the previous engine still deactivate correctly.
func deactivateLegacyDetached(slice string, st RepoState, force bool, currentHEAD, currentBranch string) error {
	if st.TargetSHA != "" && currentHEAD != st.TargetSHA {
		if !force {
			where := "detached at " + shortSHA(currentHEAD)
			if currentBranch != "" {
				where = fmt.Sprintf("on branch %q (%s)", currentBranch, shortSHA(currentHEAD))
			}
			return fmt.Errorf("%w in %q: journal expected %s, primary is %s; re-run `slis deactivate --force` to restore anyway",
				ErrPrimaryDrifted, st.Primary, shortSHA(st.TargetSHA), where)
		}
		// Forced restore. If commits were made on the detached HEAD (the recorded
		// tip is an ancestor of the current HEAD), protect them on a rescue branch
		// before switching away — never orphan commits.
		if git.IsAncestor(st.Primary, st.TargetSHA, currentHEAD) {
			rescue := rescueBranchName(slice, st)
			if git.RefExists(st.Primary, "refs/heads/"+rescue) {
				return fmt.Errorf("rescue branch %q already exists in %q — inspect/remove it, then re-run", rescue, st.Primary)
			}
			if _, err := git.Run(st.Primary, "branch", "--", rescue, currentHEAD); err != nil {
				return fmt.Errorf("create rescue branch %q in %q: %w", rescue, st.Primary, err)
			}
		}
	}
	if err := restorePriorBranch(st); err != nil {
		return err
	}
	return popPinnedStash(st)
}

// priorBranchGoneErr reports the hard ErrPriorBranchGone error when the branch
// the primary was on before activation has been deleted. Returns nil when the
// prior branch still exists (or the primary was detached before activation).
func priorBranchGoneErr(st RepoState) error {
	if st.PriorBranch != "" && !git.RefExists(st.Primary, "refs/heads/"+st.PriorBranch) {
		return fmt.Errorf("%w: %q in %q — recreate it with `git -C %s branch %s %s`, then re-run `slis deactivate`",
			ErrPriorBranchGone, st.PriorBranch, st.Primary, st.Primary, st.PriorBranch, shortSHA(st.PriorSHA))
	}
	return nil
}

// restorePriorBranch switches the primary back to the branch it was on before
// activation (or a detached HEAD at the prior SHA if it was detached). A prior
// branch that no longer exists is a hard error; one that exists but is checked
// out elsewhere falls back to a detached-SHA restore so a stash pop can proceed.
func restorePriorBranch(st RepoState) error {
	if err := priorBranchGoneErr(st); err != nil {
		return err
	}
	if st.PriorBranch != "" {
		if _, err := git.Run(st.Primary, "switch", st.PriorBranch); err != nil {
			// Branch switch failed (e.g. branch checked out in another worktree).
			// Fall back to detached restore at the prior SHA.
			if _, err2 := git.Run(st.Primary, "switch", "--detach", st.PriorSHA); err2 != nil {
				return fmt.Errorf("switch to prior branch %q in %q: %w (detach fallback also failed: %v)", st.PriorBranch, st.Primary, err, err2)
			}
		}
		return nil
	}
	if _, err := git.Run(st.Primary, "switch", "--detach", st.PriorSHA); err != nil {
		return fmt.Errorf("switch --detach to prior SHA %q in %q: %w", st.PriorSHA, st.Primary, err)
	}
	return nil
}

// deleteTempBranchIfAtSHA deletes st.TempBranch with -D, but only after
// re-verifying its tip still equals wantSHA immediately before the delete — so
// the force-delete provably discards nothing. The caller must already have
// switched off the temp branch.
func deleteTempBranchIfAtSHA(st RepoState, wantSHA string) error {
	tip, err := git.RevParse(st.Primary, st.TempBranch)
	if err != nil {
		return fmt.Errorf("verify temp branch %q tip in %q: %w", st.TempBranch, st.Primary, err)
	}
	if tip != wantSHA {
		return fmt.Errorf("refusing to delete temp branch %q in %q: tip %s no longer equals %s (drift detected between check and delete) — inspect it, then re-run",
			st.TempBranch, st.Primary, shortSHA(tip), shortSHA(wantSHA))
	}
	if _, err := git.Run(st.Primary, "branch", "-D", "--", st.TempBranch); err != nil {
		return fmt.Errorf("delete temp branch %q in %q: %w", st.TempBranch, st.Primary, err)
	}
	return nil
}

// commitsAhead returns a short human-facing description of the commits between
// base and head in dir (for the "you committed" refusal message).
func commitsAhead(dir, base, head string) string {
	out, err := git.Run(dir, "log", "--oneline", "--no-decorate", base+".."+head)
	if err != nil || strings.TrimSpace(out) == "" {
		return "new commits"
	}
	return strings.TrimSpace(out)
}

// popPinnedStash pops the stash pinned during activation, matched by its exact
// commit SHA (and message when present) so the right entry is restored even if
// other stashes exist. A pop conflict returns ErrStashConflict with the stash
// left intact. A no-op when nothing was stashed.
func popPinnedStash(st RepoState) error {
	if st.StashRef == "" {
		return nil
	}

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

	// Pop that exact entry. On non-zero exit (conflict), git stash pop has already
	// applied the changes with conflict markers and left the stash entry intact —
	// so we just surface the error.
	if _, err := git.Run(st.Primary, "stash", "pop", fmt.Sprintf("stash@{%d}", index)); err != nil {
		return fmt.Errorf("%w: %v", ErrStashConflict, err)
	}

	return nil
}
