package cli

import (
	"fmt"
	"sort"

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
//   - a stale journal: a journal exists but no primary is still detached at its
//     recorded slice tip (the swap looks already undone). --fix deletes the
//     journal, but only when EVERY primary is on a branch (provably not swapped).
//   - a journal repo whose prior_branch has been deleted, so `slis deactivate`
//     could not restore it.
//   - orphaned detach: no journal exists but a configured primary is detached.
//
// All checks are read-only except the provably-safe journal deletion under
// --fix.
func swapFindings(ws config.Workspace, journalPath string) []doctorFinding {
	j, err := swap.Load(journalPath)
	if err != nil {
		return []doctorFinding{{
			Level:  lvlWarn,
			Title:  "cannot read swap journal",
			Detail: err.Error(),
		}}
	}
	if j == nil {
		// No active swap recorded — the only concern is a primary left detached
		// with no journal to restore it.
		return orphanDetachFindings(ws)
	}

	var findings []doctorFinding
	detachedAtTarget := 0
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
		if cur, _ := git.CurrentBranch(rs.Primary); cur == "" {
			allOnBranch = false
		}
		if head == rs.TargetSHA {
			detachedAtTarget++
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

	if detachedAtTarget == 0 {
		f := doctorFinding{
			Level: lvlWarn,
			Title: fmt.Sprintf("stale swap journal for %q — no primary is detached at its slice tip", j.Slice),
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

// orphanDetachFindings reports configured primaries that are in a detached HEAD
// state while no swap journal exists — likely a slis swap whose journal was
// lost, or a manual detach. Report-only: it suggests switching back to a branch.
func orphanDetachFindings(ws config.Workspace) []doctorFinding {
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
		if cur, _ := git.CurrentBranch(primary); cur != "" {
			continue // on a branch — nothing to report
		}
		findings = append(findings, doctorFinding{
			Level: lvlWarn,
			Title: fmt.Sprintf("primary %q is detached but no slice is active", name),
			Detail: fmt.Sprintf("HEAD %s has no swap journal to restore it. If this was a slis swap, `git -C %s switch <branch>` to get back on a branch.",
				shortSHA(head), primary),
		})
	}
	return findings
}
