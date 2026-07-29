package discovery

import (
	"strings"

	"github.com/jonnyom/slis/internal/config"
)

// SliceNameIsStale reports whether a slice's name has stopped describing what the
// slice holds: none of its branches is the branch the name came from, or anything
// recognisably that feature.
//
// This happens because a registered worktree keeps its slice name across branch
// changes (that durability is what stops a stacked workflow losing its slice). Once
// the branch the name was derived from is merged and deleted, and something else
// checks out in that worktree, the label lies — a slice called
// `unpaid-leave-f2-endpoint-guards` holding `claude/wage-changes-proration-bffowg`.
//
// It is deliberately quiet about the healthy cases: an auto-grouped slice is named
// from its own branch, and a stacked switch inside the same feature still matches
// by name (branchBelongsToGroup).
func SliceNameIsStale(name string, branches []string, stripPrefix string) bool {
	if name == "" || len(branches) == 0 {
		return false
	}
	for _, branch := range branches {
		if branch == "" {
			continue
		}
		if config.SliceNameFromBranch(branch, stripPrefix) == name ||
			branchBelongsToGroup(name, branch, stripPrefix) {
			return false
		}
	}
	return true
}

// IsToolStateDir reports whether a directory name is agent/tool state rather than
// a repo checkout — `.claude`, `.serena`, and anything else following the
// dot-prefixed convention. Owned here next to the ignore rules so discovery and
// doctor cannot disagree about what counts as tool state.
func IsToolStateDir(name string) bool {
	return strings.HasPrefix(name, ".")
}

// PathDefaultIgnored reports whether a path is excluded by slis's OWN built-in
// globs — an agent's scratch worktree directory. Deliberately not the workspace's
// configured ignore list: a user may ignore a path that still holds real work they
// registered on purpose, and un-managing that for them would be destructive.
func PathDefaultIgnored(path string) bool {
	return matchesAnyGlob(path, DefaultIgnoreGlobs)
}
