package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/git"
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

func TestCreateFreshWorktreeRecyclesMergedBranchAtTrunk(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitCreate(t, repo, "branch", "test")
	gitCreate(t, repo, "commit", "-q", "--allow-empty", "-m", "advance trunk")
	want, err := git.RevParse(repo, "main")
	if err != nil {
		t.Fatal(err)
	}

	wt := filepath.Join(t.TempDir(), "test-worktree")
	if err := createFreshWorktree(repo, wt, "test", "main", ""); err != nil {
		t.Fatalf("createFreshWorktree: %v", err)
	}
	got, err := git.RevParse(wt, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("recycled branch HEAD = %s, want fresh trunk %s", got, want)
	}
}

func TestCreateFreshWorktreePreservesDivergentExistingBranch(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitCreate(t, repo, "switch", "-q", "-c", "test")
	gitCreate(t, repo, "commit", "-q", "--allow-empty", "-m", "unmerged work")
	want, err := git.RevParse(repo, "test")
	if err != nil {
		t.Fatal(err)
	}
	gitCreate(t, repo, "switch", "-q", "main")

	wt := filepath.Join(t.TempDir(), "test-worktree")
	err = createFreshWorktree(repo, wt, "test", "main", "")
	if err == nil || !strings.Contains(err.Error(), "not merged") {
		t.Fatalf("error = %v, want unmerged-branch refusal", err)
	}
	if _, statErr := os.Stat(wt); !os.IsNotExist(statErr) {
		t.Fatalf("refused create made worktree %q (stat err %v)", wt, statErr)
	}
	got, revErr := git.RevParse(repo, "test")
	if revErr != nil {
		t.Fatal(revErr)
	}
	if got != want {
		t.Fatalf("divergent branch moved from %s to %s", want, got)
	}
}

func TestCreateFreshWorktreeRecyclesExactMergedPRHeadAfterSquash(t *testing.T) {
	repo := testutil.NewRepo(t)
	gitCreate(t, repo, "switch", "-q", "-c", "test")
	gitCreate(t, repo, "commit", "-q", "--allow-empty", "-m", "old PR head")
	oldHead, err := git.RevParse(repo, "test")
	if err != nil {
		t.Fatal(err)
	}
	gitCreate(t, repo, "switch", "-q", "main")
	gitCreate(t, repo, "commit", "-q", "--allow-empty", "-m", "squash merge")
	want, err := git.RevParse(repo, "main")
	if err != nil {
		t.Fatal(err)
	}

	wt := filepath.Join(t.TempDir(), "test-worktree")
	if err := createFreshWorktree(repo, wt, "test", "main", oldHead); err != nil {
		t.Fatalf("createFreshWorktree: %v", err)
	}
	got, err := git.RevParse(wt, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("recycled merged PR branch HEAD = %s, want fresh trunk %s", got, want)
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
