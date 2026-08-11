package git

import (
	"context"
	"strings"
)

// DetectBase resolves the trunk/base ref to diff a feature branch against, for
// the repository that dir belongs to. dir may be a linked worktree — refs are
// shared with the primary, so trunk branches resolve from there too. Resolution
// This exists because a slice spans several repos whose trunks differ (one repo
// on master, another on main): there is no single slice-wide base, so the base
// must be detected per repo rather than presumed.
func DetectBase(dir string) string {
	return DetectBaseCtx(context.Background(), dir)
}

// DetectBaseCtx is DetectBase with a caller-supplied context, so a cancelled
// request stops probing refs instead of running every fallback to completion.
func DetectBaseCtx(ctx context.Context, dir string) string {
	// No early return for a cancelled ctx: each probe below fails without spawning
	// (exec.CommandContext checks the context before starting), and the existing
	// last-resort return already covers "nothing resolved" — the caller's next git
	// call fails on the same cancelled context rather than trusting this name.
	// 1. origin/HEAD → the remote's default branch.
	if out, err := RunCtx(ctx, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		name := strings.TrimPrefix(strings.TrimSpace(out), "origin/")
		if ref := resolveTrunk(ctx, dir, name); ref != "" {
			return ref
		}
	}
	// 2. common trunk names, local first then remote-tracking.
	for _, name := range []string{"main", "master", "develop", "trunk"} {
		if ref := resolveTrunk(ctx, dir, name); ref != "" {
			return ref
		}
	}
	// 3. last resort — may not exist; the caller's diff surfaces a per-repo error.
	return "main"
}

func resolveTrunk(ctx context.Context, dir, name string) string {
	if name == "" {
		return ""
	}
	if RefExistsCtx(ctx, dir, "origin/"+name) {
		return "origin/" + name
	}
	if RefExistsCtx(ctx, dir, name) {
		return name
	}
	return ""
}

// RefExists reports whether ref resolves to a commit in dir's repository.
func RefExists(dir, ref string) bool {
	return RefExistsCtx(context.Background(), dir, ref)
}

// RefExistsCtx is RefExists with a caller-supplied context.
func RefExistsCtx(ctx context.Context, dir, ref string) bool {
	_, err := RunCtx(ctx, dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}
