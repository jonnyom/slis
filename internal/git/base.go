package git

import "strings"

// DetectBase resolves the trunk/base ref to diff a feature branch against, for
// the repository that dir belongs to. dir may be a linked worktree — refs are
// shared with the primary, so trunk branches resolve from there too. Resolution
// This exists because a slice spans several repos whose trunks differ (one repo
// on master, another on main): there is no single slice-wide base, so the base
// must be detected per repo rather than presumed.
func DetectBase(dir string) string {
	// 1. origin/HEAD → the remote's default branch.
	if out, err := Run(dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		name := strings.TrimPrefix(strings.TrimSpace(out), "origin/")
		if ref := resolveTrunk(dir, name); ref != "" {
			return ref
		}
	}
	// 2. common trunk names, local first then remote-tracking.
	for _, name := range []string{"main", "master", "develop", "trunk"} {
		if ref := resolveTrunk(dir, name); ref != "" {
			return ref
		}
	}
	// 3. last resort — may not exist; the caller's diff surfaces a per-repo error.
	return "main"
}

func resolveTrunk(dir, name string) string {
	if name == "" {
		return ""
	}
	if RefExists(dir, "origin/"+name) {
		return "origin/" + name
	}
	if RefExists(dir, name) {
		return name
	}
	return ""
}

// RefExists reports whether ref resolves to a commit in dir's repository.
func RefExists(dir, ref string) bool {
	_, err := Run(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}
