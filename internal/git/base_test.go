package git_test

import (
	"os/exec"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

// TestDetectBaseMain verifies detection on a repo whose trunk is "main".
func TestDetectBaseMain(t *testing.T) {
	repo := testutil.NewRepo(t) // created on main
	if got := git.DetectBase(repo); got != "main" {
		t.Errorf("DetectBase(main repo) = %q, want %q", got, "main")
	}
}

// TestDetectBaseMaster verifies detection picks up "master" when that is the
// trunk and "main" does not exist — the exact case that broke the diff
// (presuming main when the repo is on master).
func TestDetectBaseMaster(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(cmd.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "master")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	run("commit", "-q", "--allow-empty", "-m", "init")

	if got := git.DetectBase(dir); got != "master" {
		t.Errorf("DetectBase(master repo) = %q, want %q", got, "master")
	}
}

// TestRefExists verifies RefExists distinguishes present from absent refs.
func TestRefExists(t *testing.T) {
	repo := testutil.NewRepo(t)
	if !git.RefExists(repo, "main") {
		t.Error("RefExists(main) = false, want true")
	}
	if git.RefExists(repo, "no-such-branch") {
		t.Error("RefExists(no-such-branch) = true, want false")
	}
}
