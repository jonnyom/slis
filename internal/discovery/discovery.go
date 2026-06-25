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

// Discover iterates over all repos in the workspace, collects their worktrees,
// skips the primary checkout and any detached/bare/branch-less worktrees, then
// groups the remainder into model.Slice values keyed by the branch name with
// ws.Grouping.StripPrefix removed. The returned slice is sorted by name.
func Discover(ws config.Workspace) ([]model.Slice, error) {
	// bucket: slice name → *model.Slice
	buckets := make(map[string]*model.Slice)

	for name, repo := range ws.Repos {
		wts, err := git.ListWorktrees(repo.Primary)
		if err != nil {
			return nil, err
		}

		for _, wt := range wts {
			// Skip the primary worktree itself.
			if samePath(wt.Path, repo.Primary) {
				continue
			}
			// Skip detached, bare, or branch-less worktrees.
			if wt.Detached || wt.Bare || wt.Branch == "" {
				continue
			}
			// A branch name beginning with "-" can't be produced by normal git;
			// it only arises from a forged ref/HEAD in a cloned repo. Passed to a
			// git or gh subcommand without a leading "--", it would be parsed as a
			// flag (argument injection), so refuse to discover it at the source.
			if strings.HasPrefix(wt.Branch, "-") {
				continue
			}

			key := config.SliceNameFromBranch(wt.Branch, ws.Grouping.StripPrefix)

			tip, err := git.RevParse(wt.Path, "HEAD")
			if err != nil {
				return nil, err
			}

			if _, ok := buckets[key]; !ok {
				// Base is left empty: a slice can span repos with different
				// trunks (one on master, another on main), so there is no single
				// slice-wide base. Per-repo trunk detection happens at diff/summary
				// time (git.DetectBase). Base is reserved as an optional override.
				buckets[key] = &model.Slice{
					Name:    key,
					Members: make(map[string]model.SliceMember),
				}
			}
			buckets[key].Members[name] = model.SliceMember{
				Repo:         name,
				Branch:       wt.Branch,
				WorktreePath: wt.Path,
				TipSHA:       tip,
			}
		}
	}

	// Collect and sort keys.
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build the result slice by dereferencing pointers.
	result := make([]model.Slice, len(keys))
	for i, k := range keys {
		result[i] = *buckets[k]
	}
	return result, nil
}
