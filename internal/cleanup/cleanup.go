// Package cleanup removes a finished slice: it deletes each member repo's git
// worktree, optionally deletes the (merged) feature branches, and kills the
// slice's tmux session. It never force-removes dirty worktrees or unmerged
// branches unless Options.Force is set, and it operates per repo so one repo's
// failure does not abort the others.
package cleanup

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// Options controls what a removal touches.
type Options struct {
	DeleteBranches bool   // also delete each member branch (git branch -d, merged-only)
	Force          bool   // git worktree remove --force + git branch -D
	ActiveJournal  string // path to the swap journal; when set, Remove refuses a live slice
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
//
// If opts.ActiveJournal is set, Remove re-reads the journal at removal time and
// refuses (returning an error, touching nothing) when the slice is currently
// live (swapped in). This is the authoritative, race-free guard — callers must
// not rely on a stale in-memory "is live" flag.
func Remove(ws config.Workspace, sl model.Slice, opts Options) (Report, error) {
	rep := Report{Slice: sl.Name}

	if opts.ActiveJournal != "" {
		if j, _ := swap.Load(opts.ActiveJournal); j != nil && j.Slice == sl.Name {
			return rep, fmt.Errorf("slice %q is live (swapped in); run `slis deactivate` first", sl.Name)
		}
	}
	members := make([]model.SliceMember, 0, len(sl.Repos()))
	for _, repo := range sl.Repos() {
		members = append(members, sl.Members[repo])
	}
	panes, _ := tmuxctl.ListSessionPanes()
	sessionNames := tmuxctl.RelatedSessionNames(sl.Name, members, panes)

	allRemoved := len(sl.Repos()) > 0
	for _, repo := range sl.Repos() {
		m := sl.Members[repo]
		res := RepoResult{Repo: repo, Branch: m.Branch}
		primary := ws.Repos[repo].Primary

		if err := git.RemoveWorktree(primary, m.WorktreePath, opts.Force); err != nil {
			res.Err = err.Error()
			allRemoved = false
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
	if allRemoved {
		removeEmptyManagedParents(ws.Root, sl.Name)
	}

	// Kill the tmux session only when every worktree was removed — a failed or
	// partial clear must not destroy a session the user may still be working in.
	if allRemoved && tmuxctl.Available() {
		for _, sessionName := range sessionNames {
			if err := tmuxctl.KillSessionNamed(sessionName); err == nil {
				rep.SessionKilled = true
			}
		}
	}

	return rep, nil
}

// removeEmptyManagedParents removes only empty directories left above worktrees
// that Git already removed. os.Remove refuses non-empty directories, so source
// files or another slice can never be deleted by this housekeeping step.
func removeEmptyManagedParents(root, slice string) {
	if root == "" || slice == "" {
		return
	}
	base := filepath.Clean(filepath.Join(root, ".slis", "worktrees"))
	for dir := filepath.Clean(filepath.Join(base, slice)); dir != base; dir = filepath.Dir(dir) {
		rel, err := filepath.Rel(base, dir)
		if err != nil || rel == "." || rel == ".." || filepath.IsAbs(rel) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return // non-empty or otherwise protected: leave it untouched
		}
	}
}
