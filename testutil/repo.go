// Package testutil provides shared test scaffolding for slis packages.
package testutil

import (
	"os/exec"
	"testing"
)

// NewRepo makes a temp git repo with one commit on `main`. Returns its path.
func NewRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	run("commit", "-q", "--allow-empty", "-m", "init")
	return dir
}

// AddWorktree creates `<repo>/../<branch-leaf>` worktree on a new branch.
func AddWorktree(t *testing.T, repo, branch, path string) {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}
}
