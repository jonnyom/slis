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
		if allOnBranch {
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

	if len(findings) == 0 {
		findings = append(findings, doctorFinding{
			Level: lvlOK,
			Title: fmt.Sprintf("swap journal healthy — %q active across %d repo(s)", j.Slice, len(j.Repos)),
		})
	}
	return findings
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
		return f
	}
	trunk := ws.Repos[repo].DefaultBranch
	if trunk == "" || !branchExists(primary, trunk) {
		return f
	}
	if dirty, err := git.IsDirty(primary); err != nil || dirty {
		return f
	}

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
