// Package diff computes per-repo git diffs for a slice.
package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
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

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", b+"...HEAD")
		if err != nil {
			rd.Err = err.Error()
			results = append(results, rd)
			continue
		}

		rd.Files = parseNumstat(numstat)

		// Best-effort full patch; ignore error (already have numstat).
		patch, _ := git.Run(m.WorktreePath, "diff", b+"...HEAD")
		rd.Patch = patch

		results = append(results, rd)
	}

	return results, nil
}

// SliceStat is a lightweight variant of SliceDiff that fills only Files (the
// numstat) and omits the full patch. It is used for the slice browser's summary
// cards, where loading every repo's full diff would be wasteful. base follows
// the same "" = auto-detect-per-repo rule as SliceDiff.
func SliceStat(sl model.Slice, base string) ([]RepoDiff, error) {
	if sl.Members == nil {
		return nil, fmt.Errorf("diff: slice has nil Members map")
	}

	repos := sl.Repos()
	results := make([]RepoDiff, 0, len(repos))

	for _, repo := range repos {
		m := sl.Members[repo]
		rd := RepoDiff{Repo: repo}

		b := base
		if b == "" {
			b = git.DetectBase(m.WorktreePath)
		}

		numstat, err := git.Run(m.WorktreePath, "diff", "--numstat", b+"...HEAD")
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
		fs := FileStat{Path: parts[2]}
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
