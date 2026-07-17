package gt

import (
	"os/exec"
	"strings"
)

// Track runs `gt track --parent <parent> --no-interactive <branch>` in dir,
// registering branch in Graphite's metadata with the given parent. This is a
// metadata-only operation — it records the parent relationship and never
// rewrites git history — making it the second (and only other) mutator in
// package gt after Restack.
//
// It returns ErrNotInstalled when the gt CLI is absent so best-effort callers
// (slis create / adopt) can warn and carry on rather than fail the worktree
// creation. A non-empty branch and parent are required.
func Track(dir, branch, parent string) (string, error) {
	if _, err := exec.LookPath("gt"); err != nil {
		return "", ErrNotInstalled
	}
	cmd := exec.Command("gt", "track", "--parent", parent, "--no-interactive", branch)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
