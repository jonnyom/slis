// Package diff computes per-repo git diffs for a slice.
package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/safeterm"
)

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
// worktree using `git diff <base>...HEAD`. Members are processed in sorted
// repo order (sl.Repos()). Per-repo errors are captured in RepoDiff.Err and
// do not abort the whole slice. The top-level error is reserved for
// catastrophic failures (e.g. nil slice).
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

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", "--end-of-options", b+"...HEAD")
		if err != nil {
			rd.Err = err.Error()
			results = append(results, rd)
			continue
		}

		rd.Files = parseNumstat(numstat)

		// Best-effort full patch; ignore error (already have numstat).
		patch, _ := git.Run(m.WorktreePath, "diff", "--end-of-options", b+"...HEAD")
		// Patch contains repo file contents (and filenames) which can embed
		// terminal escapes; strip them before this string is rendered.
		rd.Patch = safeterm.Strip(patch)

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

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", "--end-of-options", b+"...HEAD")
		if err != nil {
			rd.Err = err.Error()
			results = append(results, rd)
			continue
		}
		rd.Files = parseNumstat(numstat)
		patch, _ := git.Run(m.WorktreePath, "diff", "--end-of-options", b+"...HEAD")
		// Patch contains repo file contents (and filenames) which can embed
		// terminal escapes; strip them before this string is rendered.
		rd.Patch = safeterm.Strip(patch)
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

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", "--end-of-options", b+"...HEAD")
		if err != nil {
			rd.Err = err.Error()
			results = append(results, rd)
			continue
		}
		rd.Files = parseNumstat(numstat)
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
