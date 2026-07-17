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

// Result is the full outcome of discovery: the grouped slices plus everything
// that was skipped or failed, so no worktree ever vanishes without explanation.
type Result struct {
	Slices     []model.Slice     `json:"slices"`
	Skipped    []SkippedWorktree `json:"skipped,omitempty"`
	RepoErrors []RepoError       `json:"repo_errors,omitempty"`
}

// samePath returns true if paths a and b refer to the same filesystem location.
// It resolves symlinks via filepath.EvalSymlinks (falling back to
// filepath.Clean) before comparing, which is necessary on macOS where
// t.TempDir() returns a /var/... path that is a symlink to /private/var/...,
// while git worktree list --porcelain returns resolved real paths.
func samePath(a, b string) bool {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	return resolve(a) == resolve(b)
}

// Discover groups the workspace's linked worktrees into slices by branch name.
// It is a thin wrapper over DiscoverReport for callers that only need the
// slices; use DiscoverReport when you also need the skipped/error report.
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
	buckets := make(map[string]*model.Slice)
	var skipped []SkippedWorktree
	var repoErrors []RepoError

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

			key := config.SliceNameFromBranch(wt.Branch, ws.Grouping.StripPrefix)
			if _, ok := buckets[key]; !ok {
				// Base is left empty: a slice can span repos with different
				// trunks (one on master, another on main), so there is no single
				// slice-wide base. Per-repo trunk detection happens at
				// diff/summary time (git.DetectBase); Base is an optional override.
				buckets[key] = &model.Slice{
					Name:    key,
					Members: make(map[string]model.SliceMember),
				}
			}
			// One branch per repo per slice: keep the first, surface the loser.
			if _, exists := buckets[key].Members[name]; exists {
				skip(ReasonGroupingCollision)
				continue
			}
			buckets[key].Members[name] = model.SliceMember{
				Repo:         name,
				Branch:       wt.Branch,
				WorktreePath: wt.Path,
				TipSHA:       tip,
			}
		}
	}

	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]model.Slice, len(keys))
	for i, k := range keys {
		result[i] = *buckets[k]
	}

	sort.Slice(skipped, func(i, j int) bool {
		if skipped[i].Repo != skipped[j].Repo {
			return skipped[i].Repo < skipped[j].Repo
		}
		if skipped[i].Path != skipped[j].Path {
			return skipped[i].Path < skipped[j].Path
		}
		return skipped[i].Reason < skipped[j].Reason
	})
	sort.Slice(repoErrors, func(i, j int) bool {
		return repoErrors[i].Repo < repoErrors[j].Repo
	})

	return Result{Slices: result, Skipped: skipped, RepoErrors: repoErrors}
}
