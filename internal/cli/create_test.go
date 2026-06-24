package cli

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/testutil"
)

// gitCreate runs a git command in dir for the create tests, failing on error.
func gitCreate(t *testing.T, dir string, args ...string) {
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

func TestTrunkStartPoint(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()

	// Bare origin + a clone, so the clone has an origin/master tracking ref.
	origin := filepath.Join(root, "origin.git")
	gitCreate(t, root, "init", "-q", "--bare", "-b", "master", origin)
	primary := filepath.Join(root, "primary")
	gitCreate(t, root, "clone", "-q", origin, primary)
	gitCreate(t, primary, "config", "user.email", "t@t")
	gitCreate(t, primary, "config", "user.name", "t")
	gitCreate(t, primary, "commit", "-q", "--allow-empty", "-m", "init")
	gitCreate(t, primary, "push", "-q", "origin", "master")
	gitCreate(t, primary, "fetch", "-q", "origin")

	// Prefers origin/<trunk> over the local branch (the whole point: fresh trunk).
	if got := trunkStartPoint(primary, "master", false); got != "origin/master" {
		t.Errorf("trunkStartPoint(master) = %q, want origin/master", got)
	}
	// Unknown trunk → "" so the caller falls back to current HEAD.
	if got := trunkStartPoint(primary, "nope", false); got != "" {
		t.Errorf("trunkStartPoint(nope) = %q, want \"\"", got)
	}
	// Empty trunk → "".
	if got := trunkStartPoint(primary, "", false); got != "" {
		t.Errorf("trunkStartPoint(\"\") = %q, want \"\"", got)
	}

	// No remote → falls back to the local trunk branch.
	local := testutil.NewRepo(t) // on main, no origin
	if got := trunkStartPoint(local, "main", false); got != "main" {
		t.Errorf("trunkStartPoint(main, no-remote) = %q, want main", got)
	}
	if got := trunkStartPoint(local, "master", false); got != "" {
		t.Errorf("trunkStartPoint(master, no-remote, no-branch) = %q, want \"\"", got)
	}
}
