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
	// Process-wide for the whole test (inherited by code-under-test git
	// spawns): silence machine-global git tooling whose detached background
	// work races t.TempDir cleanup — trace2-driven daemons (git-ai) and any
	// global/system config hooks.
	t.Setenv("GIT_TRACE2_EVENT", "0")
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_SYSTEM", "/dev/null")
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
	// Set local identity so commits made directly via git (e.g. in linked
	// worktrees, which share this config) work on machines/CI runners that have
	// no global git identity configured.
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	// Detached background git processes outlive the test and race t.TempDir
	// cleanup ("RemoveAll: directory not empty" flakes): auto-gc/maintenance,
	// and any trace2-driven tooling (e.g. a git-ai shim writing notes via
	// fast-import after each commit). Disable all of them for this repo.
	run("config", "gc.auto", "0")
	run("config", "maintenance.auto", "false")
	run("config", "trace2.eventtarget", "")
	run("config", "trace2.normaltarget", "")
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
