package report

import (
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// DiffFileStat is a JSON-friendly per-file line-change summary. Added/Deleted
// are -1 for binary files (git reports no line counts for them).
type DiffFileStat struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

// DiffStatDTO is the numstat rollup for one repo: the per-file changes plus the
// summed additions and deletions across non-binary files.
type DiffStatDTO struct {
	Files   []DiffFileStat `json:"files"`
	Added   int            `json:"added"`
	Deleted int            `json:"deleted"`
}

// DiffRepoResult is one repo's diff within a slice. Stat is present unless the
// caller asked for patch-only; Patch is present unless the caller asked for
// stat-only. Err is set (and the others empty) when this repo's diff failed.
type DiffRepoResult struct {
	Repo   string       `json:"repo"`
	Branch string       `json:"branch"`
	Stat   *DiffStatDTO `json:"stat,omitempty"`
	Patch  *string      `json:"patch,omitempty"`
	Err    string       `json:"err,omitempty"`
}

// DiffResult is the whole-slice diff: one entry per member repo.
type DiffResult struct {
	Repos []DiffRepoResult `json:"repos"`
}

// SliceDiffScoped computes a slice's diff for one of three scopes and returns
// it as a marshal-ready DiffResult:
//
//   - "working" — the worktree's uncommitted changes only (staged + unstaged +
//     untracked), no base.
//   - "parent"  — the committed branch diff vs the branch's Graphite parent
//     (falling back to the repo trunk when the branch is not stacked).
//   - "trunk"   — the committed branch diff vs the repo trunk.
//
// format selects what each repo entry carries: "stat" (numstat only, cheapest),
// "patch" (the unified diff text only), or "both".
func SliceDiffScoped(sl model.Slice, scope, format string) (DiffResult, error) {
	statOnly := format == "stat"

	var repoDiffs []diff.RepoDiff
	var err error
	switch scope {
	case "working":
		if statOnly {
			repoDiffs, err = diff.SliceDirtyStat(sl)
		} else {
			repoDiffs, err = diff.SliceDirtyDiff(sl)
		}
	default:
		bases := scopeBases(sl, scope)
		if statOnly {
			repoDiffs, err = diff.SliceStatBases(sl, bases)
		} else {
			repoDiffs, err = diff.SliceDiffBases(sl, bases)
		}
	}
	if err != nil {
		return DiffResult{}, err
	}

	branchByRepo := make(map[string]string, len(sl.Members))
	for repo, mem := range sl.Members {
		branchByRepo[repo] = mem.Branch
	}

	repos := make([]DiffRepoResult, 0, len(repoDiffs))
	for _, rd := range repoDiffs {
		entry := DiffRepoResult{Repo: rd.Repo, Branch: branchByRepo[rd.Repo], Err: rd.Err}
		if rd.Err == "" {
			if format != "patch" {
				entry.Stat = statDTO(rd)
			}
			if format != "stat" {
				patch := rd.Patch
				entry.Patch = &patch
			}
		}
		repos = append(repos, entry)
	}
	return DiffResult{Repos: repos}, nil
}

// statDTO converts a diff.RepoDiff's numstat into a DiffStatDTO.
func statDTO(rd diff.RepoDiff) *DiffStatDTO {
	files := make([]DiffFileStat, 0, len(rd.Files))
	for _, f := range rd.Files {
		files = append(files, DiffFileStat{Path: f.Path, Added: f.Added, Deleted: f.Deleted})
	}
	return &DiffStatDTO{Files: files, Added: rd.TotalAdded(), Deleted: rd.TotalDeleted()}
}

// gtParent returns the Graphite parent branch of branch in dir's repo, or "".
func gtParent(dir, branch string) string {
	st, err := gt.ReadStack(dir)
	if err != nil {
		return ""
	}
	bs, ok := st[branch]
	if !ok || len(bs.Parents) == 0 {
		return ""
	}
	return bs.Parents[0].Ref
}

// scopeBases computes the per-repo diff base for a non-working scope, mirroring
// the cockpit diff pane: an explicit slice Base override wins; "parent" uses the
// branch's Graphite parent (falling back to the detected trunk when the branch
// is not stacked); "trunk" (and anything else) uses the detected trunk.
func scopeBases(sl model.Slice, scope string) map[string]string {
	bases := make(map[string]string, len(sl.Members))
	for repo, mem := range sl.Members {
		switch {
		case sl.Base != "":
			bases[repo] = sl.Base
		case scope == "parent":
			if p := gtParent(mem.WorktreePath, mem.Branch); p != "" {
				bases[repo] = p
			} else {
				bases[repo] = git.DetectBase(mem.WorktreePath)
			}
		default:
			bases[repo] = git.DetectBase(mem.WorktreePath)
		}
	}
	return bases
}
