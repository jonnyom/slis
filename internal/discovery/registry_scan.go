package discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
// Registered members whose worktree has disappeared are surfaced in Missing so
// a known slice never silently vanishes. Switching branches inside the same
// registered worktree is healthy: stacked-branch workflows do this routinely,
// and the worktree remains owned by its registered slice.
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
	// Repair only stale Slis-owned administrative records whose checkout is
	// already gone. Targeting each path avoids touching missing external
	// worktrees, which may simply live on a temporarily unavailable volume.
	pruneStaleManagedWorktreeMetadata(ws)
	removeEmptyManagedDirectories(ws.Root)
	recs, skipped, repoErrors := collect(ws)

	reg, exists, loadErr := config.LoadRegistry(registryPath)
	if loadErr != nil {
		// Older versions wrote the registry directly. If one was interrupted,
		// preserve the damaged file for diagnosis and rebuild from the healthy
		// worktrees below instead of letting the whole cockpit appear empty.
		if quarantineBrokenRegistry(registryPath) == nil {
			reg = config.Registry{Slices: map[string]config.RegistrySlice{}}
			exists = false
		}
	}
	if exists {
		recs, skipped = restoreRegisteredDetached(reg, recs, skipped)
	}
	if exists {
		if repaired, changed := reconcileRegistry(ws.Root, reg, recs); changed {
			reg = repaired
			_ = config.SaveRegistry(registryPath, reg)
		}
	}
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
	// Branch names describe stack position, not durable slice membership. Keep a
	// registered worktree in its recorded slice when the user switches/creates a
	// stacked branch inside it.
	slices = Apply(slices, membershipOverrides(ws.Root, reg, managed))
	skipped = append(skipped, collisions...)

	if !exists {
		reg = grandfatheredRegistry(slices)
		_ = config.SaveRegistry(registryPath, reg)
	} else if registerManagedSlices(&reg, slices, ws.Root) {
		// Older releases created worktrees under the managed tree without adding
		// them to an already-existing registry. Backfill those durable identities
		// so branch switches and later removal remain reliable.
		_ = config.SaveRegistry(registryPath, reg)
	}

	result := Result{
		Slices:     slices,
		Skipped:    skipped,
		RepoErrors: repoErrors,
		Candidates: candidates,
		Missing:    missingMembers(reg, recs),
	}
	sortReport(&result)
	return result
}

func restoreRegisteredDetached(reg config.Registry, recs []worktreeRec, skipped []SkippedWorktree) ([]worktreeRec, []SkippedWorktree) {
	remaining := make([]SkippedWorktree, 0, len(skipped))
	for _, worktree := range skipped {
		if worktree.Reason != ReasonDetached {
			remaining = append(remaining, worktree)
			continue
		}
		member, ok := registeredMemberAtPath(reg, worktree.Repo, worktree.Path)
		if !ok {
			remaining = append(remaining, worktree)
			continue
		}
		tip, err := git.RevParse(worktree.Path, "HEAD")
		if err != nil {
			remaining = append(remaining, worktree)
			continue
		}
		recs = append(recs, worktreeRec{repo: worktree.Repo, path: worktree.Path, branch: member.Branch, tip: tip})
	}
	return recs, remaining
}

func registeredMemberAtPath(reg config.Registry, repo, path string) (config.RegistryMember, bool) {
	resolvedPath := resolvePath(path)
	for _, slice := range reg.Slices {
		member, ok := slice.Members[repo]
		if ok && member.WorktreePath != "" && resolvePath(member.WorktreePath) == resolvedPath {
			return member, true
		}
	}
	return config.RegistryMember{}, false
}

func pruneStaleManagedWorktreeMetadata(ws config.Workspace) {
	for _, repo := range ws.Repos {
		worktrees, err := git.ListWorktrees(repo.Primary)
		if err != nil {
			continue
		}
		for _, worktree := range worktrees {
			if worktree.Prunable && underManagedTree(worktree.Path, ws.Root) {
				_ = git.ForgetMissingWorktree(repo.Primary, worktree.Path)
			}
		}
	}
}

func quarantineBrokenRegistry(path string) error {
	stamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	return os.Rename(path, path+".broken-"+stamp)
}

// removeEmptyManagedDirectories cleans up directory litter left by historical
// worktree removal bugs. os.Remove deliberately refuses non-empty directories,
// and WalkDir does not follow symlinks, so user files are never removed here.
func removeEmptyManagedDirectories(root string) {
	if root == "" {
		return
	}
	base := filepath.Join(root, ".slis", "worktrees")
	type candidate struct {
		path    string
		modTime time.Time
	}
	var dirs []candidate
	_ = filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
		if err != nil || !entry.IsDir() || path == base {
			return nil
		}
		if _, markerErr := os.Lstat(filepath.Join(path, ".git")); markerErr == nil || !os.IsNotExist(markerErr) {
			return filepath.SkipDir
		}
		if info, infoErr := entry.Info(); infoErr == nil {
			dirs = append(dirs, candidate{path: path, modTime: info.ModTime()})
		}
		return nil
	})
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i].path) > len(dirs[j].path) })
	cutoff := time.Now().Add(-5 * time.Minute)
	for _, dir := range dirs {
		// A short grace period avoids racing another slis process between mkdir
		// and `git worktree add`; Monday's legacy litter is far older than this.
		if dir.modTime.Before(cutoff) {
			_ = os.Remove(dir.path)
		}
	}
}

// reconcileRegistry migrates durable identities from older releases. Healthy
// entries are refreshed to the worktree's current branch/path. A missing entry
// is removed automatically only when its recorded path is in Slis's managed
// tree (and therefore Slis-owned); missing imported/external worktrees remain
// visible for human recovery. Branch refs are never deleted here.
func reconcileRegistry(root string, reg config.Registry, recs []worktreeRec) (config.Registry, bool) {
	changed := false
	for sliceName, slice := range reg.Slices {
		for repo, member := range slice.Members {
			var matched *worktreeRec
			for i := range recs {
				if recs[i].repo == repo && member.WorktreePath != "" &&
					resolvePath(recs[i].path) == resolvePath(member.WorktreePath) {
					matched = &recs[i]
					break
				}
			}
			if matched == nil {
				for i := range recs {
					if recs[i].repo == repo && member.Branch != "" && recs[i].branch == member.Branch {
						matched = &recs[i]
						break
					}
				}
			}
			if matched != nil {
				if member.Branch != matched.branch || resolvePath(member.WorktreePath) != resolvePath(matched.path) {
					slice.Members[repo] = config.RegistryMember{Branch: matched.branch, WorktreePath: matched.path}
					changed = true
				}
				continue
			}

			if !underManagedTree(member.WorktreePath, root) || !missingOrEmpty(member.WorktreePath) {
				continue
			}
			// An empty directory at the exact registered worktree path is old
			// litter, not a checkout. Remove only that empty leaf; non-empty paths
			// are preserved for manual inspection.
			_ = os.Remove(member.WorktreePath)
			delete(slice.Members, repo)
			changed = true
		}
		if len(slice.Members) == 0 {
			delete(reg.Slices, sliceName)
			changed = true
			continue
		}
		reg.Slices[sliceName] = slice
	}
	return reg, changed
}

func missingOrEmpty(path string) bool {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return true
	}
	if err != nil || len(entries) != 0 {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.ModTime().Before(time.Now().Add(-5*time.Minute))
}

// registerManagedSlices backfills healthy Slis-owned worktrees that predate
// create-time registry writes. External/grandfathered worktrees are excluded:
// only paths beneath <root>/.slis/worktrees are unambiguously Slis-managed.
func registerManagedSlices(reg *config.Registry, slices []model.Slice, root string) bool {
	changed := false
	for _, slice := range slices {
		for repo, member := range slice.Members {
			if !underManagedTree(member.WorktreePath, root) {
				continue
			}
			existingSlice, ok := reg.Slices[slice.Name]
			if ok {
				if existing, memberExists := existingSlice.Members[repo]; memberExists &&
					existing.Branch == member.Branch &&
					resolvePath(existing.WorktreePath) == resolvePath(member.WorktreePath) {
					continue
				}
			}
			reg.RegisterCreated(slice.Name, repo, member.Branch, member.WorktreePath)
			changed = true
		}
	}
	return changed
}

// membershipOverrides maps each healthy managed worktree's CURRENT branch back
// to its durable slice. A path under .slis/worktrees/<slice>/<repo> owns that
// slice even when no registry entry exists; an explicit registry match wins.
func membershipOverrides(root string, reg config.Registry, recs []worktreeRec) Overrides {
	overrides := Overrides{}
	for _, rec := range recs {
		if sliceName, ok := managedTreeSlice(rec.path, root); ok {
			if overrides[sliceName] == nil {
				overrides[sliceName] = map[string]string{}
			}
			overrides[sliceName][rec.repo] = rec.branch
		}
	}

	// Match the recorded path first (branch switches are expected), then the
	// recorded branch (worktree moves are also supported by registeredIndex).
	for sliceName, slice := range reg.Slices {
		for repo, member := range slice.Members {
			var matched *worktreeRec
			for i := range recs {
				if recs[i].repo == repo && member.WorktreePath != "" &&
					resolvePath(recs[i].path) == resolvePath(member.WorktreePath) {
					matched = &recs[i]
					break
				}
			}
			if matched == nil {
				for i := range recs {
					if recs[i].repo == repo && recs[i].branch == member.Branch {
						matched = &recs[i]
						break
					}
				}
			}
			if matched == nil {
				continue
			}
			if overrides[sliceName] == nil {
				overrides[sliceName] = map[string]string{}
			}
			overrides[sliceName][repo] = matched.branch
		}
	}
	return overrides
}

func managedTreeSlice(path, root string) (string, bool) {
	if root == "" {
		return "", false
	}
	base := resolvePath(filepath.Join(root, ".slis", "worktrees"))
	rel, err := filepath.Rel(base, resolvePath(path))
	if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || parts[0] == "" || parts[0] == "." {
		return "", false
	}
	return strings.Join(parts[:len(parts)-1], "/"), true
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

// missingMembers returns registry members that no longer resolve to a healthy
// worktree by either recorded path or recorded branch. A current branch change
// at the recorded path is healthy and stays in the registered slice.
func missingMembers(reg config.Registry, recs []worktreeRec) []MissingMember {
	var missing []MissingMember
	for name, s := range reg.Slices {
		for repo, m := range s.Members {
			if registeredMemberResolves(repo, m, recs) {
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

func registeredMemberResolves(repo string, member config.RegistryMember, recs []worktreeRec) bool {
	for _, rec := range recs {
		if rec.repo != repo {
			continue
		}
		if member.WorktreePath != "" && resolvePath(rec.path) == resolvePath(member.WorktreePath) {
			return true
		}
		if member.Branch != "" && rec.branch == member.Branch {
			return true
		}
	}
	return false
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
