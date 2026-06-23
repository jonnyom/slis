package gt

import (
	"errors"
	"os/exec"
	"strings"
)

// ErrNotInstalled is returned by mutating operations when the gt CLI is absent.
var ErrNotInstalled = errors.New("gt CLI not found on PATH")

// Restack runs `gt restack --no-interactive` in dir, rebasing each branch in the
// current stack onto its parent. It returns the combined output and a non-nil
// error if gt is missing or the restack could not complete (e.g. a conflict,
// which gt leaves as an in-progress rebase for the user to resolve + continue).
//
// This is the ONLY mutating operation in package gt; everything else is a
// read-only reader of `gt state`. It rewrites git history in dir's worktree, so
// callers must confirm with the user and refuse dirty worktrees first.
func Restack(dir string) (string, error) {
	if _, err := exec.LookPath("gt"); err != nil {
		return "", ErrNotInstalled
	}
	cmd := exec.Command("gt", "restack", "--no-interactive")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
