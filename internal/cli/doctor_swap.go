package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/swap"
)

// shortSHA truncates a commit SHA to 7 chars for human-facing messages.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// swapFindings inspects the swap journal against the real state of each primary
// checkout and reports data-safety concerns:
//   - a stale journal: a journal exists but no primary is still on its slis/live
//     temp branch (the swap looks already undone). --fix deletes the journal, but
//     only when EVERY primary is on a branch (provably not swapped).
//   - a journal repo whose prior_branch has been deleted, so `slis deactivate`
//     could not restore it.
//   - orphaned slis/live branch or detached primary with no journal.
//
// All checks are read-only except the provably-safe journal deletion under --fix
// and the contained-branch cleanup in orphanSwapFindings.
func swapFindings(ws config.Workspace, dtos []SliceDTO, journalPath string) []doctorFinding {
	j, err := swap.Load(journalPath)
	if err != nil {
		return []doctorFinding{{
			Level:  lvlWarn,
			Title:  "cannot read swap journal",
			Detail: err.Error(),
		}}
	}
	if j == nil {
		// No active swap recorded — the only concern is a primary left on a
		// slis/live temp branch (or detached) with no journal to restore it.
		return orphanSwapFindings(ws, dtos)
	}

	var findings []doctorFinding
	swappedIn := 0
	allOnBranch := true

	for _, rs := range j.Repos {
		head, err := git.RevParse(rs.Primary, "HEAD")
		if err != nil {
			allOnBranch = false
			findings = append(findings, doctorFinding{
				Level:  lvlWarn,
				Title:  fmt.Sprintf("swap: cannot read primary for %s/%s", j.Slice, rs.Repo),
				Detail: err.Error(),
			})
			continue
		}
		cur, _ := git.CurrentBranch(rs.Primary)
		if cur == "" {
			allOnBranch = false
		}
		if repoSwappedIn(rs, head, cur) {
			swappedIn++
		}
		if rs.PriorBranch != "" && !branchExists(rs.Primary, rs.PriorBranch) {
			findings = append(findings, doctorFinding{
				Level: lvlFail,
				Title: fmt.Sprintf("swap: prior branch %q is gone (%s/%s)", rs.PriorBranch, j.Slice, rs.Repo),
				Detail: fmt.Sprintf("`slis deactivate` can't restore it. Recreate with `git -C %s branch %s %s`, then deactivate.",
					rs.Primary, rs.PriorBranch, shortSHA(rs.PriorSHA)),
			})
		}
	}

	if swappedIn == 0 {
		f := doctorFinding{
			Level: lvlWarn,
			Title: fmt.Sprintf("stale swap journal for %q — no primary is on its slis/live branch", j.Slice),
			Detail: "the swap looks already undone. Run `slis deactivate --force` to reconcile, " +
				"or `slis doctor --fix` to delete the journal (only when every primary is on a branch).",
		}
		// S2: the journal is the only pointer to any pinned auto-stash. If any repo
		// still holds one, deleting the journal would orphan the user's stashed work,
		// so the finding is report-only — never offer the --fix deletion. Only when
		// no StashRef exists (and every primary is on a branch) is the deletion safe.
		stashRefs := pinnedStashRefs(j)
		switch {
		case len(stashRefs) > 0:
			f.Detail = fmt.Sprintf("the swap looks already undone, but the journal still pins auto-stash(es): %s. "+
				"Deleting the journal would orphan them, so no automatic fix is offered. Recover your work first with "+
				"`git -C <primary> stash list | grep slis:auto`, then `git -C <primary> stash apply <ref>`; once it is safe, "+
				"run `slis deactivate --force` (or delete the journal manually).",
				strings.Join(stashRefs, ", "))
		case allOnBranch:
			f.fixDesc = "delete the stale swap journal"
			f.fix = func() (string, error) {
				if err := swap.Clear(journalPath); err != nil {
					return "", err
				}
				return "deleted stale journal (every primary is on a branch)", nil
			}
		}
		findings = append(findings, f)
	}

	// Cross-check the journal against the active slice's members and the
	// configured repos: a journal that only records some of the swap catches a
	// crash mid-activate that the per-entry loop above cannot see.
	findings = append(findings, partialSwapFindings(ws, dtos, j)...)

	if len(findings) == 0 {
		findings = append(findings, doctorFinding{
			Level: lvlOK,
			Title: fmt.Sprintf("swap journal healthy — %q active across %d repo(s)", j.Slice, len(j.Repos)),
		})
	}
	return findings
}

// partialSwapFindings cross-checks the active journal against the active slice's
// discovered members and the configured repos, catching a swap that only partly
// took (a crash mid-activate). It reports two report-only warnings:
//
//   - a slice member repo absent from the journal whose primary is NOT swapped in
//     (the activate crashed before it reached this repo);
//   - a configured repo whose primary sits on this slice's slis/live temp branch
//     but has no journal entry (the crash landed between the switch and the
//     journal write).
//
// Both are report-only while a journal exists: `slis deactivate` unwinds the
// journaled repos safely, and auto-fixing an un-journaled branch could discard
// commits the journal can't vouch for. The un-journaled orphan is only cleared by
// doctor's --fix flow AFTER a deactivate has emptied the journal.
func partialSwapFindings(ws config.Workspace, dtos []SliceDTO, j *swap.Journal) []doctorFinding {
	journalRepos := make(map[string]bool, len(j.Repos))
	for _, rs := range j.Repos {
		journalRepos[rs.Repo] = true
	}
	liveBranch := swap.LiveBranchName(j.Slice)

	var findings []doctorFinding

	// (a) Slice members that never made it into the journal (and aren't swapped in
	//     on the live branch — those are the un-journaled case handled by (b)).
	for _, d := range dtos {
		if d.Name != j.Slice {
			continue
		}
		for _, m := range d.Members {
			if journalRepos[m.Repo] {
				continue
			}
			if cur, _ := git.CurrentBranch(ws.Repos[m.Repo].Primary); cur == liveBranch {
				continue // on the live branch but un-journaled — reported by (b)
			}
			findings = append(findings, doctorFinding{
				Level: lvlWarn,
				Title: fmt.Sprintf("partial swap: %q was never swapped (crash during activate?)", m.Repo),
				Detail: fmt.Sprintf("the swap journal for %q covers only %d of the slice's repos, so this one is still on its own branch. Run `slis deactivate` to unwind the swapped repo(s), then `slis activate %s` again.",
					j.Slice, len(j.Repos), j.Slice),
			})
		}
	}

	// (b) Configured repos on this slice's live branch but with no journal entry.
	names := make([]string, 0, len(ws.Repos))
	for name := range ws.Repos {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if journalRepos[name] {
			continue
		}
		primary := ws.Repos[name].Primary
		cur, err := git.CurrentBranch(primary)
		if err != nil || cur != liveBranch {
			continue
		}
		findings = append(findings, doctorFinding{
			Level: lvlWarn,
			Title: fmt.Sprintf("swapped but un-journaled: %q is on %q with no journal entry", name, liveBranch),
			Detail: fmt.Sprintf("a crash between the switch and the journal write left this repo swapped in without a record; it is not auto-fixed while a journal exists. Run `slis deactivate` to unwind the journaled repos, then `slis doctor --fix` (or `git -C %s switch <trunk>`) to restore this one.",
				primary),
		})
	}

	return findings
}

// pinnedStashRefs returns a human-facing list ("<short-sha> (<repo>)") of every
// journal repo that still pins an auto-stash. The journal is the only pointer to
// these stashes, so their presence blocks the stale-journal --fix deletion.
func pinnedStashRefs(j *swap.Journal) []string {
	var refs []string
	for _, rs := range j.Repos {
		if rs.StashRef != "" {
			label := shortSHA(rs.StashRef)
			if rs.Repo != "" {
				label += " (" + rs.Repo + ")"
			}
			refs = append(refs, label)
		}
	}
	return refs
}

// repoSwappedIn reports whether a journal repo is still swapped in. For the
// temp-branch engine that means the primary is on its recorded temp branch; for
// a legacy detached journal it means HEAD is still detached at the slice tip.
func repoSwappedIn(rs swap.RepoState, head, currentBranch string) bool {
	if rs.TempBranch != "" {
		return currentBranch == rs.TempBranch
	}
	return currentBranch == "" && head == rs.TargetSHA
}

// orphanSwapFindings reports configured primaries left in a swapped-looking state
// with no journal to restore them: on a slis/live temp branch (the temp-branch
// engine) or detached (the legacy engine). For an orphaned slis/live branch whose
// commits are fully contained in the slice's branch, --fix switches the primary
// to its trunk and deletes the branch (SHA-verified, nothing lost); otherwise it
// is report-only.
func orphanSwapFindings(ws config.Workspace, dtos []SliceDTO) []doctorFinding {
	livePrefix := swap.LiveBranchName("")
	memberBranch := memberBranchIndex(dtos)

	var findings []doctorFinding
	names := make([]string, 0, len(ws.Repos))
	for name := range ws.Repos {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		primary := ws.Repos[name].Primary
		head, err := git.RevParse(primary, "HEAD")
		if err != nil {
			continue // not a readable repo — leave it to other checks
		}
		cur, _ := git.CurrentBranch(primary)

		switch {
		case strings.HasPrefix(cur, livePrefix):
			findings = append(findings, orphanLiveBranchFinding(ws, memberBranch, name, primary, cur, livePrefix))
		case cur == "":
			findings = append(findings, doctorFinding{
				Level: lvlWarn,
				Title: fmt.Sprintf("primary %q is detached but no slice is active", name),
				Detail: fmt.Sprintf("HEAD %s has no swap journal to restore it. If this was a slis swap, `git -C %s switch <branch>` to get back on a branch.",
					shortSHA(head), primary),
			})
		}
	}
	return findings
}

// orphanLiveBranchFinding builds the finding for a primary stuck on a slis/live
// temp branch with no journal. It offers a --fix (switch to trunk + SHA-verified
// delete) only when the branch's commits are fully contained in the slice's
// branch and the primary is clean — so the delete provably loses nothing.
func orphanLiveBranchFinding(ws config.Workspace, memberBranch map[string]string, repo, primary, liveBranch, livePrefix string) doctorFinding {
	sliceName := strings.TrimPrefix(liveBranch, livePrefix)
	f := doctorFinding{
		Level: lvlWarn,
		Title: fmt.Sprintf("primary %q is on orphaned temp branch %q but no slice is active", repo, liveBranch),
		Detail: fmt.Sprintf("a swap for %q left this branch with no journal to restore the primary. Switch back with `git -C %s switch <branch>`.",
			sliceName, primary),
	}

	sliceBranch := memberBranch[sliceName+"\x00"+repo]
	if sliceBranch == "" {
		return f
	}
	liveTip, err := git.RevParse(primary, liveBranch)
	if err != nil {
		return f
	}
	// Contained ⟺ the temp branch adds nothing that isn't already on the slice
	// branch, so deleting it loses no work.
	if !git.IsAncestor(primary, liveTip, sliceBranch) {
		f.Detail += " (needs manual attention: has commits not on the slice branch)"
		return f
	}
	trunk := ws.Repos[repo].DefaultBranch
	if trunk == "" || !branchExists(primary, trunk) {
		return f
	}
	if dirty, err := git.IsDirty(primary); err != nil || dirty {
		return f
	}

	f.Detail += " (auto-fixable with --fix)"
	f.fixDesc = "switch the primary to its trunk and delete the contained temp branch"
	f.fix = func() (string, error) {
		// Re-verify containment immediately before the delete (nothing changed
		// underneath us) so the -D provably discards nothing.
		tip, err := git.RevParse(primary, liveBranch)
		if err != nil {
			return "", err
		}
		if !git.IsAncestor(primary, tip, sliceBranch) {
			return "", fmt.Errorf("temp branch %q is no longer contained in %q — not deleting", liveBranch, sliceBranch)
		}
		if _, err := git.Run(primary, "switch", trunk); err != nil {
			return "", fmt.Errorf("switch %q to %q: %w", primary, trunk, err)
		}
		if _, err := git.Run(primary, "branch", "-D", "--", liveBranch); err != nil {
			return "", fmt.Errorf("delete temp branch %q: %w", liveBranch, err)
		}
		return fmt.Sprintf("switched to %q and deleted %q (its commits are in %q)", trunk, liveBranch, sliceBranch), nil
	}
	return f
}

// memberBranchIndex maps "<slice>\x00<repo>" → the member's branch name, for
// resolving which real branch a slis/live/<slice> branch shadows in a repo.
func memberBranchIndex(dtos []SliceDTO) map[string]string {
	idx := make(map[string]string)
	for _, d := range dtos {
		for _, m := range d.Members {
			idx[d.Name+"\x00"+m.Repo] = m.Branch
		}
	}
	return idx
}
