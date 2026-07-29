package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
)

// reasonAdvice maps a discovery skip reason to a doctor severity and the manual
// remedy to suggest. Discovery already performs provably safe housekeeping;
// doctor reports anything that remains and leaves ambiguous repairs to people.
type reasonAdvice struct {
	level  doctorLevel
	remedy string
}

var skipReasonAdvice = map[string]reasonAdvice{
	discovery.ReasonDetached: {lvlWarn,
		"HEAD is detached — `git switch <branch>` inside the worktree (or `slis adopt`) to re-attach it so it shows up as a slice."},
	discovery.ReasonPrunable: {lvlWarn,
		"its working directory is gone — Slis normally prunes stale unlocked metadata automatically; inspect locks, then run `git worktree prune` in the repo if it remains."},
	discovery.ReasonBranchless: {lvlWarn,
		"no branch checked out — `git switch <branch>` inside the worktree."},
	discovery.ReasonBare: {lvlInfo,
		"bare worktree — expected to be excluded from slices."},
	discovery.ReasonIgnored: {lvlInfo,
		"hidden by an ignore rule — expected (agent scratch worktrees under .claude/worktrees are ignored by default); `slis import <path>` to manage it anyway."},
	discovery.ReasonInvalidBranchName: {lvlWarn,
		"branch name starts with '-' (unusable) — rename it with `git branch -m`."},
	discovery.ReasonRevParseFailed: {lvlWarn,
		"HEAD could not be resolved (corrupt ref) — inspect the worktree; `git worktree repair` may help."},
	discovery.ReasonGroupingCollision: {lvlWarn,
		"two worktrees in this repo map to the same slice name — rename one branch so each slice has one branch per repo."},
}

// skippedWorktreeFindings turns the discovery skip report into one doctor
// finding per reason, listing the affected worktrees. Nothing is auto-fixed.
func skippedWorktreeFindings(skipped []SkippedWorktreeDTO) []doctorFinding {
	byReason := map[string][]SkippedWorktreeDTO{}
	for _, s := range skipped {
		byReason[s.Reason] = append(byReason[s.Reason], s)
	}
	reasons := make([]string, 0, len(byReason))
	for r := range byReason {
		reasons = append(reasons, r)
	}
	sort.Strings(reasons)

	findings := make([]doctorFinding, 0, len(reasons))
	for _, reason := range reasons {
		items := byReason[reason]
		advice, ok := skipReasonAdvice[reason]
		if !ok {
			advice = reasonAdvice{lvlWarn, "worktree hidden from slices."}
		}
		paths := make([]string, 0, len(items))
		for _, it := range items {
			paths = append(paths, fmt.Sprintf("%s:%s", it.Repo, it.Path))
		}
		findings = append(findings, doctorFinding{
			Level:  advice.level,
			Title:  fmt.Sprintf("%d hidden worktree%s: %s", len(items), plural(len(items)), reason),
			Detail: advice.remedy + " (" + strings.Join(paths, ", ") + ")",
		})
	}
	return findings
}

// staleNameFindings surfaces slices whose name no longer describes what they hold:
// the branch the name came from is gone and the worktree now carries something
// else. The name comes from a registered worktree keeping its slice identity across
// branch changes, which is what stops a stacked workflow losing its slice — but
// once the original branch is merged and deleted, the label misleads.
//
// A slice sitting on a path slis would otherwise ignore (an agent's throwaway
// worktree) is fixable: dropping the registry entry lets the ignore rule apply
// again, touching no git state. Anything else is the user's real work, so it is
// reported with the remedy rather than changed for them.
func staleNameFindings(ws config.Workspace, dtos []SliceDTO, registryPath string) []doctorFinding {
	var findings []doctorFinding
	for _, dto := range staleNamedSlices(dtos, ws.Grouping.StripPrefix) {
		branches := sliceBranches(dto)
		ignoredPath := len(dto.Members) > 0
		for _, m := range dto.Members {
			if !discovery.PathDefaultIgnored(m.WorktreePath) {
				ignoredPath = false
			}
		}
		finding := doctorFinding{
			Level: lvlWarn,
			Title: fmt.Sprintf("slice %q is named after a branch it no longer holds", dto.Name),
			Detail: fmt.Sprintf("now on %s — the name came from a branch that is gone (merged or deleted). `slis forget %s` to drop the registry entry, then `slis import <path>` if you still want it managed.",
				strings.Join(branches, ", "), dto.Name),
		}
		if ignoredPath {
			name := dto.Name
			finding.Detail += " Its worktree is an agent scratch directory slis ignores by default, so --fix drops the entry."
			finding.fixDesc = "forget " + name
			finding.fix = func() (string, error) { return forgetRegisteredSlice(registryPath, name) }
		}
		findings = append(findings, finding)
	}
	return findings
}

// missingSliceFindings surfaces registered slice members whose worktree has
// disappeared (or moved off its branch), so a known slice never silently
// vanishes. Nothing is auto-fixed: the remedy needs a human decision.
func missingSliceFindings(missing []MissingDTO) []doctorFinding {
	bySlice := map[string][]MissingDTO{}
	for _, m := range missing {
		bySlice[m.Slice] = append(bySlice[m.Slice], m)
	}
	names := make([]string, 0, len(bySlice))
	for n := range bySlice {
		names = append(names, n)
	}
	sort.Strings(names)

	findings := make([]doctorFinding, 0, len(names))
	for _, name := range names {
		items := bySlice[name]
		paths := make([]string, 0, len(items))
		for _, it := range items {
			paths = append(paths, fmt.Sprintf("%s:%s", it.Repo, it.Path))
		}
		findings = append(findings, doctorFinding{
			Level: lvlWarn,
			Title: fmt.Sprintf("missing slice %q (%d worktree%s gone)", name, len(items), plural(len(items))),
			Detail: "the registered worktree is gone or moved off its branch — recreate it " +
				"(`slis create`/`slis adopt`) or drop it from the registry (`slis forget " + name + "`). (" +
				strings.Join(paths, ", ") + ")",
		})
	}
	return findings
}

// candidateFindings surfaces unmanaged worktrees slis will not ingest until the
// user opts in. Informational — importing is a choice, not a problem.
func candidateFindings(candidates []CandidateDTO) []doctorFinding {
	if len(candidates) == 0 {
		return nil
	}
	paths := make([]string, 0, len(candidates))
	for _, c := range candidates {
		paths = append(paths, fmt.Sprintf("%s:%s", c.Repo, c.Path))
	}
	return []doctorFinding{{
		Level: lvlInfo,
		Title: fmt.Sprintf("%d new worktree%s not ingested (opt-in)", len(candidates), plural(len(candidates))),
		Detail: "`slis import <path>` (or `slis import --all`) to manage, or `slis ignore <path-or-glob>` to hide. (" +
			strings.Join(paths, ", ") + ")",
	}}
}

// repoErrorFindings surfaces repos whose worktree listing failed entirely.
func repoErrorFindings(repoErrors []RepoErrorDTO) []doctorFinding {
	findings := make([]doctorFinding, 0, len(repoErrors))
	for _, e := range repoErrors {
		findings = append(findings, doctorFinding{
			Level:  lvlFail,
			Title:  fmt.Sprintf("repo %q could not be read", e.Repo),
			Detail: e.Err + " — check the repo path in workspace.yaml and that it is a git repo.",
		})
	}
	return findings
}

// resolvePathClean resolves symlinks (falling back to Clean) so paths from git
// and from filesystem walks compare equal (macOS /var → /private/var).
func resolvePathClean(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// trackedWorktreePaths returns the set of every worktree path git still tracks
// (healthy members plus every skipped-but-listed worktree), resolved.
func trackedWorktreePaths(ws config.Workspace) map[string]bool {
	rep := discovery.DiscoverReport(ws)
	tracked := map[string]bool{}
	for _, s := range rep.Slices {
		for _, m := range s.Members {
			tracked[resolvePathClean(m.WorktreePath)] = true
		}
	}
	for _, sk := range rep.Skipped {
		tracked[resolvePathClean(sk.Path)] = true
	}
	return tracked
}

// orphanWorktreeFindings finds directories under <root>/.slis/worktrees/** that
// git no longer tracks as worktrees: empty litter dirs and full checkouts whose
// .git file points at an admin slot that has been rebound elsewhere. It reports
// both and repairs nothing (removal is left to the user).
func orphanWorktreeFindings(ws config.Workspace) []doctorFinding {
	if ws.Root == "" {
		return nil
	}
	base := filepath.Join(ws.Root, ".slis", "worktrees")
	sliceDirs, err := os.ReadDir(base)
	if err != nil {
		return nil // no managed-worktree tree yet: nothing to check.
	}

	tracked := trackedWorktreePaths(ws)

	var findings []doctorFinding
	for _, sliceEntry := range sliceDirs {
		if !sliceEntry.IsDir() {
			continue
		}
		sliceDir := filepath.Join(base, sliceEntry.Name())
		repoEntries, err := os.ReadDir(sliceDir)
		if err != nil {
			continue
		}
		if !hasNonHiddenEntry(repoEntries) {
			// Only agent state (.claude, .serena) or nothing at all: no checkout is
			// left here, so the slice directory itself is the leftover.
			findings = append(findings, orphanEmptyFinding(sliceDir, len(repoEntries) > 0))
			continue
		}
		for _, repoEntry := range repoEntries {
			if !repoEntry.IsDir() {
				continue
			}
			// Agents keep per-directory state next to the checkouts (.claude,
			// .serena, …). Those are live tool data, not worktree litter — reporting
			// them told the user to delete working state by hand.
			if discovery.IsToolStateDir(repoEntry.Name()) {
				continue
			}
			candidate := filepath.Join(sliceDir, repoEntry.Name())
			if tracked[resolvePathClean(candidate)] {
				continue
			}
			findings = append(findings, classifyOrphan(candidate))
		}
	}
	return findings
}

// hasNonHiddenEntry reports whether entries hold anything other than dot-prefixed
// agent/tool state.
func hasNonHiddenEntry(entries []os.DirEntry) bool {
	for _, entry := range entries {
		if !discovery.IsToolStateDir(entry.Name()) {
			return true
		}
	}
	return false
}

// orphanEmptyFinding reports a slice directory that holds no checkout. agentStateOnly
// says its remaining contents are agent/tool state (.claude, .serena), which is the
// usual reason an otherwise-empty slice directory survives.
func orphanEmptyFinding(dir string, agentStateOnly bool) doctorFinding {
	detail := dir + " — no repo checkout left here"
	if agentStateOnly {
		detail += " (only agent state such as .claude/.serena remains)"
	}
	return doctorFinding{
		Level:  lvlWarn,
		Title:  "orphaned worktree directory (no checkout)",
		Detail: detail + "; remove it by hand once you're sure it holds no work.",
	}
}

// classifyOrphan inspects an untracked directory under the managed worktree
// tree and reports it as either empty litter or a rebound checkout.
func classifyOrphan(candidate string) doctorFinding {
	gitFile := filepath.Join(candidate, ".git")
	info, err := os.Stat(gitFile)
	if err != nil || info.IsDir() {
		return orphanEmptyFinding(candidate, false)
	}
	return doctorFinding{
		Level: lvlWarn,
		Title: "orphaned worktree checkout (admin slot rebound elsewhere)",
		Detail: candidate + " — git no longer lists this checkout (its admin slot points at another path). " +
			"It may hold unmerged work; inspect it, then remove it by hand.",
	}
}
