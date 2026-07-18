// Package diff computes per-repo git diffs for a slice.
package diff

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/safeterm"
)

// maxUntrackedPatchLines caps how many lines of a single untracked file are
// emitted into the patch (the FileStat count stays exact) so one huge generated
// file can't produce a multi-megabyte diff.
const maxUntrackedPatchLines = 1000

// untrackedDiff returns FileStats and a synthesized unified patch for the
// worktree's untracked (non-ignored) files. `git diff` omits untracked files, so
// an agent's brand-new, not-yet-`git add`-ed files would otherwise read as "no
// changes". This reads them directly (read-only — it never stages anything).
func untrackedDiff(worktree string) ([]FileStat, string) {
	out, err := git.Run(worktree, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil || out == "" {
		return nil, ""
	}
	var stats []FileStat
	var patch strings.Builder
	for _, rel := range strings.Split(out, "\x00") {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(worktree, rel))
		if err != nil {
			continue
		}
		if bytes.IndexByte(data, 0) >= 0 { // binary
			stats = append(stats, FileStat{Path: rel, Added: -1, Deleted: -1})
			fmt.Fprintf(&patch, "diff --git a/%s b/%s\nnew file mode 100644\nBinary files /dev/null and b/%s differ\n", rel, rel, rel)
			continue
		}
		lines := contentLines(data)
		stats = append(stats, FileStat{Path: rel, Added: len(lines), Deleted: 0})
		fmt.Fprintf(&patch, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", rel, rel, rel, len(lines))
		shown := lines
		if len(shown) > maxUntrackedPatchLines {
			shown = shown[:maxUntrackedPatchLines]
		}
		for _, ln := range shown {
			patch.WriteString("+" + ln + "\n")
		}
		if len(lines) > len(shown) {
			fmt.Fprintf(&patch, "… (%d more lines)\n", len(lines)-len(shown))
		}
	}
	return stats, safeterm.Strip(patch.String())
}

// addUntracked appends the worktree's untracked files to rd's Files (and, when
// withPatch, its Patch).
func addUntracked(rd *RepoDiff, worktree string, withPatch bool) {
	uStats, uPatch := untrackedDiff(worktree)
	rd.Files = append(rd.Files, uStats...)
	if withPatch && uPatch != "" {
		if rd.Patch != "" && !strings.HasSuffix(rd.Patch, "\n") {
			rd.Patch += "\n"
		}
		rd.Patch += uPatch
	}
}

// contentLines splits file bytes into content lines, dropping a single trailing
// newline's empty element so the count matches git's added-line count.
func contentLines(data []byte) []string {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	return lines
}

// FileStat holds the per-file line change summary from git diff --numstat.
type FileStat struct {
	Path    string
	Added   int // -1 for binary files
	Deleted int // -1 for binary files
}

// RepoDiff holds the diff result for a single repo member of a slice.
type RepoDiff struct {
	Repo  string
	Base  string // the ref this repo was diffed against (trunk, or the stack parent)
	Files []FileStat
	Patch string // full git diff text (hunks)
	Err   string // non-empty if this repo's diff failed
}

// TotalAdded returns the sum of Added across all non-binary files.
func (d RepoDiff) TotalAdded() int {
	total := 0
	for _, f := range d.Files {
		if f.Added >= 0 {
			total += f.Added
		}
	}
	return total
}

// TotalDeleted returns the sum of Deleted across all non-binary files.
func (d RepoDiff) TotalDeleted() int {
	total := 0
	for _, f := range d.Files {
		if f.Deleted >= 0 {
			total += f.Deleted
		}
	}
	return total
}

// SliceDiff returns one RepoDiff per member of sl, computed in each member's
// worktree using `git diff --merge-base <base>` — i.e. the diff from the
// merge-base of <base> and HEAD to the WORKING TREE. That captures everything
// the slice has changed since it forked from <base>, INCLUDING uncommitted
// (staged + unstaged) edits, so an agent's in-progress work shows up.
// (Untracked, never-`git add`-ed files are not shown — git diff omits them.)
// Members are processed in sorted repo order (sl.Repos()). Per-repo errors are
// captured in RepoDiff.Err and do not abort the whole slice. The top-level
// error is reserved for catastrophic failures (e.g. nil slice).
//
// base is the ref to diff against. Pass "" to auto-detect each repo's trunk
// independently (git.DetectBase) — required for slices spanning repos with
// different trunks. A non-empty base is used verbatim for every member.
func SliceDiff(sl model.Slice, base string) ([]RepoDiff, error) {
	if sl.Members == nil {
		return nil, fmt.Errorf("diff: slice has nil Members map")
	}

	repos := sl.Repos() // sorted
	results := make([]RepoDiff, 0, len(repos))

	for _, repo := range repos {
		m := sl.Members[repo]
		rd := RepoDiff{Repo: repo}

		b := base
		if b == "" {
			b = git.DetectBase(m.WorktreePath)
		}
		rd.Base = b

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", "--merge-base", "--end-of-options", b)
		if err != nil {
			rd.Err = err.Error()
			results = append(results, rd)
			continue
		}

		rd.Files = parseNumstat(numstat)

		// Best-effort full patch; ignore error (already have numstat).
		patch, _ := git.Run(m.WorktreePath, "diff", "--merge-base", "--end-of-options", b)
		// Patch contains repo file contents (and filenames) which can embed
		// terminal escapes; strip them before this string is rendered.
		rd.Patch = safeterm.Strip(patch)
		addUntracked(&rd, m.WorktreePath, true)

		results = append(results, rd)
	}

	return results, nil
}

// SliceDiffBases is like SliceDiff but takes a per-repo base ref (bases[repo]).
// A repo with no entry (or "") falls back to auto-detecting its trunk. This is
// how the cockpit diffs a stacked branch against its Graphite parent (so it
// shows only that branch's changes, not the whole downstack).
func SliceDiffBases(sl model.Slice, bases map[string]string) ([]RepoDiff, error) {
	if sl.Members == nil {
		return nil, fmt.Errorf("diff: slice has nil Members map")
	}
	repos := sl.Repos()
	results := make([]RepoDiff, 0, len(repos))
	for _, repo := range repos {
		m := sl.Members[repo]
		rd := RepoDiff{Repo: repo}

		b := bases[repo]
		if b == "" {
			b = git.DetectBase(m.WorktreePath)
		}
		rd.Base = b

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", "--merge-base", "--end-of-options", b)
		if err != nil {
			rd.Err = err.Error()
			results = append(results, rd)
			continue
		}
		rd.Files = parseNumstat(numstat)
		patch, _ := git.Run(m.WorktreePath, "diff", "--merge-base", "--end-of-options", b)
		// Patch contains repo file contents (and filenames) which can embed
		// terminal escapes; strip them before this string is rendered.
		rd.Patch = safeterm.Strip(patch)
		addUntracked(&rd, m.WorktreePath, true)
		results = append(results, rd)
	}
	return results, nil
}

// BranchAgainstParent computes the committed diff of branch vs parent using
// merge-base (three-dot) semantics (`git diff parent...branch`), so it shows
// only the commits branch adds on top of parent — that branch's own change, not
// the whole downstack. It is a pure ref-to-ref read: no working tree, no
// untracked files. repoDir MUST be the repo's primary checkout (the repo rule);
// refs are shared across a repo's checkouts, so any branch in the stack resolves
// there regardless of which worktree has it checked out. A git failure is
// captured in RepoDiff.Err (not returned) so a caller can render it per-repo.
func BranchAgainstParent(repoDir, parent, branch string, withPatch bool) (RepoDiff, error) {
	rd := RepoDiff{Base: parent}
	spec := parent + "..." + branch
	numstat, err := git.Run(repoDir, "diff", "--numstat", spec)
	if err != nil {
		rd.Err = err.Error()
		return rd, nil
	}
	rd.Files = parseNumstat(numstat)
	if withPatch {
		patch, _ := git.Run(repoDir, "diff", spec)
		// Patch embeds repo file contents (and filenames) which can carry terminal
		// escapes; strip them before this string is rendered.
		rd.Patch = safeterm.Strip(patch)
	}
	return rd, nil
}

// SliceDirtyDiff returns one RepoDiff per member of sl showing only the
// worktree's UNCOMMITTED changes — staged + unstaged edits (`git diff HEAD`)
// plus untracked (never-`git add`-ed) files. It diffs against no base, so a
// clean worktree yields an empty Files slice. This is the cockpit's default
// diff scope: "what have I changed but not yet committed", not the full
// committed branch diff. Per-repo errors are captured in RepoDiff.Err.
func SliceDirtyDiff(sl model.Slice) ([]RepoDiff, error) {
	return sliceDirty(sl, true)
}

// SliceDirtyStat is SliceDirtyDiff without the full patch (numstat only) — the
// lightweight variant for the browser cards.
func SliceDirtyStat(sl model.Slice) ([]RepoDiff, error) {
	return sliceDirty(sl, false)
}

// sliceDirty is the shared implementation of SliceDirtyDiff / SliceDirtyStat.
func sliceDirty(sl model.Slice, withPatch bool) ([]RepoDiff, error) {
	if sl.Members == nil {
		return nil, fmt.Errorf("diff: slice has nil Members map")
	}
	repos := sl.Repos()
	results := make([]RepoDiff, 0, len(repos))
	for _, repo := range repos {
		m := sl.Members[repo]
		rd := RepoDiff{Repo: repo, Base: "HEAD"}

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", "HEAD")
		if err != nil {
			rd.Err = err.Error()
			results = append(results, rd)
			continue
		}
		rd.Files = parseNumstat(numstat)
		if withPatch {
			patch, _ := git.Run(m.WorktreePath, "diff", "HEAD")
			// Patch contains repo file contents (and filenames) which can embed
			// terminal escapes; strip them before this string is rendered.
			rd.Patch = safeterm.Strip(patch)
		}
		addUntracked(&rd, m.WorktreePath, withPatch)
		results = append(results, rd)
	}
	return results, nil
}

// SliceStat is a lightweight variant of SliceDiff that fills only Files (the
// numstat) and omits the full patch. It is used for the slice browser's summary
// cards, where loading every repo's full diff would be wasteful. base follows
// the same "" = auto-detect-per-repo rule as SliceDiff.
func SliceStat(sl model.Slice, base string) ([]RepoDiff, error) {
	bases := make(map[string]string, len(sl.Members))
	for repo := range sl.Members {
		bases[repo] = base
	}
	return SliceStatBases(sl, bases)
}

// SliceStatBases is SliceStat with a per-repo base (bases[repo]); a repo with no
// entry (or "") auto-detects its trunk. Used by the browser cards to stat a
// stacked branch against its Graphite parent.
func SliceStatBases(sl model.Slice, bases map[string]string) ([]RepoDiff, error) {
	if sl.Members == nil {
		return nil, fmt.Errorf("diff: slice has nil Members map")
	}
	repos := sl.Repos()
	results := make([]RepoDiff, 0, len(repos))
	for _, repo := range repos {
		m := sl.Members[repo]
		rd := RepoDiff{Repo: repo}

		b := bases[repo]
		if b == "" {
			b = git.DetectBase(m.WorktreePath)
		}
		rd.Base = b

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", "--merge-base", "--end-of-options", b)
		if err != nil {
			rd.Err = err.Error()
			results = append(results, rd)
			continue
		}
		rd.Files = parseNumstat(numstat)
		addUntracked(&rd, m.WorktreePath, false)
		results = append(results, rd)
	}
	return results, nil
}

// ExpandPaths decodes a path as emitted by `git diff --numstat` into the
// concrete file path(s) it refers to. A plain path returns itself. Git emits a
// rename in one of two forms:
//
//	old/path => new/path        (whole-path rename)
//	a/{old => new}/f.go          (brace form: shared prefix/suffix factored out)
//
// Conflict detection cares about BOTH endpoints — a rename of foo→bar in one
// slice collides with an edit to foo in another — so a rename expands to the
// old and new paths. Returns nil for empty input; never returns empty strings.
// Plain paths (the common case, since the diff command sets no -M) pass through
// untouched, so missing rename detection degrades to a no-op, never a crash.
func ExpandPaths(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	if !strings.Contains(raw, "=>") {
		return []string{raw}
	}

	var oldPath, newPath string
	if open := strings.IndexByte(raw, '{'); open >= 0 {
		if closeIdx := strings.IndexByte(raw, '}'); closeIdx > open {
			prefix, suffix := raw[:open], raw[closeIdx+1:]
			oldSide, newSide, _ := strings.Cut(raw[open+1:closeIdx], "=>")
			oldPath = collapseSlashes(prefix + strings.TrimSpace(oldSide) + suffix)
			newPath = collapseSlashes(prefix + strings.TrimSpace(newSide) + suffix)
		}
	}
	if oldPath == "" && newPath == "" {
		oldSide, newSide, _ := strings.Cut(raw, "=>")
		oldPath = strings.TrimSpace(oldSide)
		newPath = strings.TrimSpace(newSide)
	}

	out := make([]string, 0, 2)
	for _, p := range []string{oldPath, newPath} {
		if p == "" {
			continue
		}
		if len(out) == 1 && out[0] == p {
			continue // collapse a no-op "rename" to one path
		}
		out = append(out, p)
	}
	if len(out) == 0 {
		return []string{raw}
	}
	return out
}

// collapseSlashes removes empty path segments left by the brace rename form when
// one side is empty (e.g. "a/{ => b}/f" → "a//f" → "a/f").
func collapseSlashes(p string) string {
	for strings.Contains(p, "//") {
		p = strings.ReplaceAll(p, "//", "/")
	}
	return strings.TrimSpace(p)
}

// parseNumstat parses the output of `git diff --numstat`.
// Each line is: <added>\t<deleted>\t<path>
// Binary files produce: -\t-\t<path>  (Added and Deleted are set to -1).
func parseNumstat(output string) []FileStat {
	var stats []FileStat
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		fs := FileStat{Path: safeterm.Strip(parts[2])}
		if parts[0] == "-" || parts[1] == "-" {
			fs.Added = -1
			fs.Deleted = -1
		} else {
			added, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			deleted, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}
			fs.Added = added
			fs.Deleted = deleted
		}
		stats = append(stats, fs)
	}
	return stats
}
