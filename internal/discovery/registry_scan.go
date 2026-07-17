package discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
)

// DefaultIgnoreGlobs are the built-in ignore patterns applied on top of the
// user's grouping.ignore list. Claude Code spins up throwaway agent worktrees
// under .claude/worktrees; ingesting them made slices "appear out of nowhere".
var DefaultIgnoreGlobs = []string{"**/.claude/worktrees/**"}

// ignoreGlobs returns the effective ignore list: the built-in defaults plus the
// workspace's configured grouping.ignore.
func ignoreGlobs(ws config.Workspace) []string {
	globs := make([]string, 0, len(DefaultIgnoreGlobs)+len(ws.Grouping.Ignore))
	globs = append(globs, DefaultIgnoreGlobs...)
	globs = append(globs, ws.Grouping.Ignore...)
	return globs
}

// Report is the registry-aware, opt-in discovery entry. Unlike DiscoverReport
// (which turns every healthy worktree into a slice), Report only ingests a
// worktree as a slice when it is MANAGED:
//
//   - managed: its path is under <ws.Root>/.slis/worktrees/**, OR the registry
//     records it → grouped into slices, exactly as before;
//   - ignored: its path matches an ignore glob (grouping.ignore + the built-in
//     .claude/worktrees default) → dropped, surfaced in Skipped as "ignored";
//   - candidate: anything else → NOT a slice; surfaced in Candidates so the user
//     can `slis import` (or `slis ignore`) it.
//
// Registered members whose worktree has disappeared (or moved off the recorded
// branch) are surfaced in Missing so a known slice never silently vanishes.
//
// Grandfathering: when no registry file exists yet (first run on upgrade), EVERY
// discovered worktree is grandfathered — pre-ignore, i.e. exactly the group-all
// set the old behavior produced — and written to the registry (source
// grandfathered), so an upgrade hides nothing. Ignore globs only filter unknown,
// unregistered worktrees discovered AFTER grandfathering.
//
// Precedence: a registered (or managed-tree) worktree is always MANAGED, even
// when its path matches an ignore glob. Ignore never hides registered work — it
// filters new/unknown worktrees only.
func Report(ws config.Workspace, registryPath string) Result {
	recs, skipped, repoErrors := collect(ws)

	reg, exists, _ := config.LoadRegistry(registryPath)
	globs := ignoreGlobs(ws)
	registered := registeredIndex(reg)

	var managed []worktreeRec
	var candidates []Candidate
	for _, r := range recs {
		switch {
		// Precedence (invariant 1): registered / managed-tree beats ignore.
		case underManagedTree(r.path, ws.Root) || registered.has(r.repo, r.branch, r.path):
			managed = append(managed, r)
		// First run (invariant 2): grandfather the whole raw discovery, pre-ignore,
		// so nothing that worked before upgrade disappears.
		case !exists:
			managed = append(managed, r)
		// Only unregistered, post-grandfather worktrees are filtered by ignore.
		case matchesAnyGlob(r.path, globs):
			skipped = append(skipped, SkippedWorktree{Repo: r.repo, Path: r.path, Branch: r.branch, Reason: ReasonIgnored})
		default:
			candidates = append(candidates, Candidate{
				Repo:   r.repo,
				Path:   r.path,
				Branch: r.branch,
				Slice:  config.SliceNameFromBranch(r.branch, ws.Grouping.StripPrefix),
			})
		}
	}

	slices, collisions := group(managed, ws.Grouping.StripPrefix)
	skipped = append(skipped, collisions...)

	if !exists {
		_ = config.SaveRegistry(registryPath, grandfatheredRegistry(slices))
	}

	result := Result{
		Slices:     slices,
		Skipped:    skipped,
		RepoErrors: repoErrors,
		Candidates: candidates,
		Missing:    missingMembers(reg),
	}
	sortReport(&result)
	return result
}

// registeredMembers indexes the registry for fast "is this worktree managed?"
// checks during classification. A worktree is registered if its repo+branch
// identity matches (survives a moved worktree path) OR its resolved path
// matches a recorded one.
type registeredMembers struct {
	keys  map[string]bool // "repo\x00branch"
	paths map[string]bool // resolved worktree paths
}

func registeredIndex(reg config.Registry) registeredMembers {
	ri := registeredMembers{keys: map[string]bool{}, paths: map[string]bool{}}
	for _, s := range reg.Slices {
		for repo, m := range s.Members {
			ri.keys[repo+"\x00"+m.Branch] = true
			if m.WorktreePath != "" {
				ri.paths[resolvePath(m.WorktreePath)] = true
			}
		}
	}
	return ri
}

// has reports whether a discovered worktree (repo, branch, path) is registered.
func (ri registeredMembers) has(repo, branch, path string) bool {
	return ri.keys[repo+"\x00"+branch] || ri.paths[resolvePath(path)]
}

// underManagedTree reports whether path lives inside <root>/.slis/worktrees.
// Such worktrees are always managed (slis created them), regardless of the
// registry.
func underManagedTree(path, root string) bool {
	if root == "" {
		return false
	}
	base := resolvePath(filepath.Join(root, ".slis", "worktrees"))
	p := resolvePath(path)
	rel, err := filepath.Rel(base, p)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// grandfatheredRegistry builds a registry from the currently-discovered slices,
// tagging every entry as grandfathered.
func grandfatheredRegistry(slices []model.Slice) config.Registry {
	now := time.Now().UTC()
	reg := config.Registry{Slices: make(map[string]config.RegistrySlice, len(slices))}
	for _, s := range slices {
		members := make(map[string]config.RegistryMember, len(s.Members))
		for repo, m := range s.Members {
			members[repo] = config.RegistryMember{Branch: m.Branch, WorktreePath: m.WorktreePath}
		}
		reg.Slices[s.Name] = config.RegistrySlice{
			Name:    s.Name,
			Members: members,
			Source:  config.SourceGrandfathered,
			At:      now,
		}
	}
	return reg
}

// missingMembers returns registry members whose worktree directory is gone, or
// which no longer sit on the recorded branch — so a known slice that lost its
// worktree surfaces instead of silently vanishing.
func missingMembers(reg config.Registry) []MissingMember {
	var missing []MissingMember
	for name, s := range reg.Slices {
		for repo, m := range s.Members {
			if m.WorktreePath == "" {
				continue
			}
			if worktreeResolvesToBranch(m.WorktreePath, m.Branch) {
				continue
			}
			missing = append(missing, MissingMember{
				Slice:  name,
				Repo:   repo,
				Path:   m.WorktreePath,
				Branch: m.Branch,
			})
		}
	}
	return missing
}

// worktreeResolvesToBranch reports whether the worktree at path still exists and
// currently has branch checked out. A gone directory or a moved-off branch both
// count as "not resolving" (→ missing).
func worktreeResolvesToBranch(path, branch string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	cur, err := git.CurrentBranch(path)
	if err != nil {
		return false
	}
	return cur == branch
}

// matchesAnyGlob reports whether path matches any of the ignore patterns.
func matchesAnyGlob(path string, patterns []string) bool {
	for _, p := range patterns {
		if matchGlob(p, path) {
			return true
		}
	}
	return false
}

// matchGlob reports whether path matches pattern. A pattern with no glob
// metacharacter matches a path that equals it or lives under it (directory
// prefix). A glob pattern supports "**" (any run, crossing "/"), "*" (any run
// within a segment) and "?" (one non-slash char), matched against the resolved
// absolute path.
func matchGlob(pattern, path string) bool {
	rp := resolvePath(path)
	if !strings.ContainsAny(pattern, "*?[") {
		clean := filepath.Clean(pattern)
		return rp == clean || strings.HasPrefix(rp, clean+string(filepath.Separator))
	}
	re, err := globToRegexp(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(rp)
}

// globToRegexp compiles a glob (with **, *, ?) into an anchored regexp.
func globToRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
