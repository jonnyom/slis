// Package discovery discovers git worktrees across a workspace and groups
// them into slices by branch name.
package discovery

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
)

// Reasons a worktree is skipped during discovery. Every skip is surfaced (never
// silently dropped) so `slis ls`/`doctor`/the TUI can explain a missing slice.
const (
	ReasonDetached          = "detached"
	ReasonBranchless        = "branchless"
	ReasonBare              = "bare"
	ReasonPrunable          = "prunable"
	ReasonInvalidBranchName = "invalid-branch-name"
	ReasonRevParseFailed    = "rev-parse-failed"
	ReasonGroupingCollision = "grouping-collision"
	// ReasonIgnored is a worktree dropped because its path matched an ignore
	// glob (config grouping.ignore or the built-in default). It is neither a
	// slice nor a candidate — it is deliberately hidden.
	ReasonIgnored = "ignored"
)

// SkippedWorktree records a worktree that discovery could not turn into a slice
// member, and why. The primary checkout is expected and never reported here.
type SkippedWorktree struct {
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch,omitempty"`
	Reason string `json:"reason"`
}

// RepoError records a repo whose worktree listing failed entirely. The other
// repos are still discovered; this error is surfaced rather than aborting.
type RepoError struct {
	Repo string `json:"repo"`
	Err  string `json:"error"`
}

// Candidate is a healthy worktree that is neither managed nor ignored: slis has
// found it but will NOT ingest it as a slice until the user opts in
// (`slis import`). Surfaced so the user can import or ignore it.
type Candidate struct {
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Slice  string `json:"slice"` // suggested slice name (branch minus strip_prefix)
}

// MissingMember is a registered slice member whose worktree no longer exists (or
// no longer sits on the recorded branch). Surfaced so a known slice never
// silently vanishes.
type MissingMember struct {
	Slice  string `json:"slice"`
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

// Result is the full outcome of discovery: the grouped slices plus everything
// that was skipped or failed, so no worktree ever vanishes without explanation.
type Result struct {
	Slices     []model.Slice     `json:"slices"`
	Skipped    []SkippedWorktree `json:"skipped,omitempty"`
	RepoErrors []RepoError       `json:"repo_errors,omitempty"`
	Candidates []Candidate       `json:"candidates,omitempty"`
	Missing    []MissingMember   `json:"missing,omitempty"`
}

// worktreeRec is a healthy linked worktree (a real branch checked out, HEAD
// resolvable) — the raw material both plain grouping and registry-aware
// classification build on.
type worktreeRec struct {
	repo   string
	path   string
	branch string
	tip    string
}

// samePath returns true if paths a and b refer to the same filesystem location.
// It resolves symlinks via filepath.EvalSymlinks (falling back to
// filepath.Clean) before comparing, which is necessary on macOS where
// t.TempDir() returns a /var/... path that is a symlink to /private/var/...,
// while git worktree list --porcelain returns resolved real paths.
func samePath(a, b string) bool {
	return resolvePath(a) == resolvePath(b)
}

// resolvePath resolves symlinks (falling back to Clean) for a stable comparison
// key across git-reported and filesystem-walked paths.
func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// collect walks every repo's linked worktrees and returns the healthy ones plus
// everything that could not be used (skipped) and any repo whose listing failed
// entirely. It is the shared front-half of DiscoverReport and Report.
func collect(ws config.Workspace) (recs []worktreeRec, skipped []SkippedWorktree, repoErrors []RepoError) {
	for name, repo := range ws.Repos {
		wts, err := git.ListWorktrees(repo.Primary)
		if err != nil {
			repoErrors = append(repoErrors, RepoError{Repo: name, Err: err.Error()})
			continue
		}

		for _, wt := range wts {
			// The primary checkout is expected and not a slice member.
			if samePath(wt.Path, repo.Primary) {
				continue
			}

			skip := func(reason string) {
				skipped = append(skipped, SkippedWorktree{
					Repo:   name,
					Path:   wt.Path,
					Branch: wt.Branch,
					Reason: reason,
				})
			}

			switch {
			// Prunable first: its working dir is gone, so never run git in it.
			case wt.Prunable:
				skip(ReasonPrunable)
				continue
			case wt.Bare:
				skip(ReasonBare)
				continue
			case wt.Detached:
				skip(ReasonDetached)
				continue
			case wt.Branch == "":
				skip(ReasonBranchless)
				continue
			// A branch name beginning with "-" can't be produced by normal git;
			// passed to a git/gh subcommand it would parse as a flag (argument
			// injection), so refuse to discover it at the source.
			case strings.HasPrefix(wt.Branch, "-"):
				skip(ReasonInvalidBranchName)
				continue
			}

			tip, err := git.RevParse(wt.Path, "HEAD")
			if err != nil {
				skip(ReasonRevParseFailed)
				continue
			}

			recs = append(recs, worktreeRec{repo: name, path: wt.Path, branch: wt.Branch, tip: tip})
		}
	}
	return recs, skipped, repoErrors
}

// group folds healthy worktree records into slices keyed by branch name minus
// stripPrefix. One branch per repo per slice: the first member wins and any
// same-repo collision is returned as a grouping-collision skip.
func group(recs []worktreeRec, stripPrefix string) (slices []model.Slice, collisions []SkippedWorktree) {
	buckets := make(map[string]*model.Slice)

	for _, r := range recs {
		key := config.SliceNameFromBranch(r.branch, stripPrefix)
		if _, ok := buckets[key]; !ok {
			// Base is left empty: a slice can span repos with different trunks
			// (one on master, another on main), so there is no single slice-wide
			// base. Per-repo trunk detection happens at diff/summary time.
			buckets[key] = &model.Slice{
				Name:    key,
				Members: make(map[string]model.SliceMember),
			}
		}
		if _, exists := buckets[key].Members[r.repo]; exists {
			collisions = append(collisions, SkippedWorktree{
				Repo:   r.repo,
				Path:   r.path,
				Branch: r.branch,
				Reason: ReasonGroupingCollision,
			})
			continue
		}
		buckets[key].Members[r.repo] = model.SliceMember{
			Repo:         r.repo,
			Branch:       r.branch,
			WorktreePath: r.path,
			TipSHA:       r.tip,
		}
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	slices = make([]model.Slice, len(keys))
	for i, k := range keys {
		slices[i] = *buckets[k]
	}
	return slices, collisions
}

// sortReport sorts a Result's Skipped/RepoErrors/Candidates/Missing for stable
// output.
func sortReport(r *Result) {
	sort.Slice(r.Skipped, func(i, j int) bool {
		if r.Skipped[i].Repo != r.Skipped[j].Repo {
			return r.Skipped[i].Repo < r.Skipped[j].Repo
		}
		if r.Skipped[i].Path != r.Skipped[j].Path {
			return r.Skipped[i].Path < r.Skipped[j].Path
		}
		return r.Skipped[i].Reason < r.Skipped[j].Reason
	})
	sort.Slice(r.RepoErrors, func(i, j int) bool {
		return r.RepoErrors[i].Repo < r.RepoErrors[j].Repo
	})
	sort.Slice(r.Candidates, func(i, j int) bool {
		if r.Candidates[i].Repo != r.Candidates[j].Repo {
			return r.Candidates[i].Repo < r.Candidates[j].Repo
		}
		return r.Candidates[i].Path < r.Candidates[j].Path
	})
	sort.Slice(r.Missing, func(i, j int) bool {
		if r.Missing[i].Slice != r.Missing[j].Slice {
			return r.Missing[i].Slice < r.Missing[j].Slice
		}
		return r.Missing[i].Repo < r.Missing[j].Repo
	})
}

// Discover groups the workspace's linked worktrees into slices by branch name.
// It is a thin wrapper over DiscoverReport for callers that only need the
// slices; use DiscoverReport when you also need the skipped/error report.
//
// Discover/DiscoverReport are the raw, stateless grouping (every healthy
// worktree becomes a slice). For registry-aware, opt-in ingestion — where an
// unmanaged worktree is a candidate rather than a slice — use Report.
func Discover(ws config.Workspace) ([]model.Slice, error) {
	return DiscoverReport(ws).Slices, nil
}

// DiscoverReport iterates over all repos in the workspace, collects their
// worktrees, and groups the ones it can into model.Slice values keyed by the
// branch name with ws.Grouping.StripPrefix removed. Anything it cannot use is
// surfaced instead of dropped:
//   - the primary checkout is skipped silently (expected, not reported);
//   - prunable/bare/detached/branch-less/invalid-name worktrees are recorded in
//     Skipped with a reason;
//   - a single worktree whose rev-parse fails is skipped (rev-parse-failed),
//     never aborting the rest;
//   - a same-repo grouping collision keeps the first member and records the
//     loser as grouping-collision;
//   - a repo whose worktree listing fails entirely is recorded in RepoErrors
//     and the remaining repos still discover normally.
//
// Slices are sorted by name; Skipped and RepoErrors are sorted for stable output.
func DiscoverReport(ws config.Workspace) Result {
	recs, skipped, repoErrors := collect(ws)
	slices, collisions := group(recs, ws.Grouping.StripPrefix)
	result := Result{Slices: slices, Skipped: append(skipped, collisions...), RepoErrors: repoErrors}
	sortReport(&result)
	return result
}
