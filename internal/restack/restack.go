// Package restack runs `gt restack` across every repo of a slice. It refuses a
// dirty worktree (commit/stash first) and never auto-stashes or auto-aborts: on
// a rebase conflict it leaves gt's in-progress rebase for the user to resolve.
package restack

import (
	"strings"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
)

// Runner restacks the stack rooted at dir's checked-out branch, returning the
// command output. Injected so the engine is testable; the default is gt.Restack.
type Runner func(dir string) (string, error)

// RepoResult is the per-repo outcome of a restack.
type RepoResult struct {
	Repo         string
	Branch       string
	Restacked    bool
	SkippedDirty bool   // worktree had uncommitted/untracked changes
	Conflict     bool   // gt stopped at a rebase conflict (resolve + gt continue)
	Err          string // any other failure
	Output       string
}

// Report summarises a restack across a slice's repos.
type Report struct {
	Slice string
	Repos []RepoResult
}

// Run restacks each member of sl using runner, skipping dirty worktrees.
func Run(sl model.Slice, runner Runner) Report {
	rep := Report{Slice: sl.Name}
	for _, repo := range sl.Repos() {
		m := sl.Members[repo]
		res := RepoResult{Repo: repo, Branch: m.Branch}

		if dirty, err := git.IsDirty(m.WorktreePath); err == nil && dirty {
			res.SkippedDirty = true
			rep.Repos = append(rep.Repos, res)
			continue
		}

		out, err := runner(m.WorktreePath)
		res.Output = out
		switch {
		case err == nil:
			res.Restacked = true
		case looksLikeConflict(out):
			res.Conflict = true
		default:
			res.Err = err.Error()
		}
		rep.Repos = append(rep.Repos, res)
	}
	return rep
}

// looksLikeConflict heuristically detects a rebase-conflict stop from output.
func looksLikeConflict(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "conflict") || strings.Contains(o, "rebase") || strings.Contains(o, "gt continue")
}
