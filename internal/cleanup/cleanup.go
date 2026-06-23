// Package cleanup removes a finished slice: it deletes each member repo's git
// worktree, optionally deletes the (merged) feature branches, and kills the
// slice's tmux session. It never force-removes dirty worktrees or unmerged
// branches unless Options.Force is set, and it operates per repo so one repo's
// failure does not abort the others.
package cleanup

import (
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// Options controls what a removal touches.
type Options struct {
	DeleteBranches bool // also delete each member branch (git branch -d, merged-only)
	Force          bool // git worktree remove --force + git branch -D
}

// RepoResult is the per-repo outcome of a removal.
type RepoResult struct {
	Repo            string
	Branch          string
	WorktreeRemoved bool
	BranchDeleted   bool
	BranchKept      string // non-empty reason the branch was intentionally not deleted
	Err             string // non-empty if worktree removal failed for this repo
}

// Report summarises a removal across all of a slice's repos.
type Report struct {
	Slice         string
	Repos         []RepoResult
	SessionKilled bool
}

// Plan describes (without performing) what Remove would do — used for --dry-run.
type Plan struct {
	Slice          string
	Repos          []RepoResult // WorktreeRemoved/BranchDeleted indicate intent
	DeleteBranches bool
	Force          bool
}

// PlanRemove returns the intended actions without touching anything.
func PlanRemove(sl model.Slice, opts Options) Plan {
	p := Plan{Slice: sl.Name, DeleteBranches: opts.DeleteBranches, Force: opts.Force}
	for _, repo := range sl.Repos() {
		m := sl.Members[repo]
		p.Repos = append(p.Repos, RepoResult{
			Repo:            repo,
			Branch:          m.Branch,
			WorktreeRemoved: true,
			BranchDeleted:   opts.DeleteBranches,
		})
	}
	return p
}

// Remove performs the cleanup for sl using the repo primaries from ws. The
// worktree is removed first so the branch is no longer checked out and can be
// deleted. Per-repo errors are captured in the Report rather than aborting.
func Remove(ws config.Workspace, sl model.Slice, opts Options) Report {
	rep := Report{Slice: sl.Name}

	for _, repo := range sl.Repos() {
		m := sl.Members[repo]
		res := RepoResult{Repo: repo, Branch: m.Branch}
		primary := ws.Repos[repo].Primary

		if err := git.RemoveWorktree(primary, m.WorktreePath, opts.Force); err != nil {
			res.Err = err.Error()
			rep.Repos = append(rep.Repos, res)
			continue
		}
		res.WorktreeRemoved = true

		if opts.DeleteBranches {
			if err := git.DeleteBranch(primary, m.Branch, opts.Force); err != nil {
				res.BranchKept = "not merged (use --force to delete)"
			} else {
				res.BranchDeleted = true
			}
		}
		rep.Repos = append(rep.Repos, res)
	}

	// Kill the tmux session best-effort (a finished slice should not keep one).
	if tmuxctl.Available() {
		if err := tmuxctl.KillSession(sl.Name); err == nil {
			rep.SessionKilled = true
		}
	}

	return rep
}
