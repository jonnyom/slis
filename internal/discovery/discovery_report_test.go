package discovery_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/testutil"
)

// addDetachedWorktree adds a linked worktree with a detached HEAD (no branch).
func addDetachedWorktree(t *testing.T, repo, path string) {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "worktree", "add", "--detach", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worktree add --detach: %v\n%s", err, out)
	}
}

// adminGitDir returns the absolute path to a worktree's admin dir
// (<primary>/.git/worktrees/<name>).
func adminGitDir(t *testing.T, worktreePath string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", worktreePath, "rev-parse", "--absolute-git-dir").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse --absolute-git-dir: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

func reasonsFor(skipped []discovery.SkippedWorktree, path string) []string {
	var reasons []string
	for _, s := range skipped {
		if s.Path == path {
			reasons = append(reasons, s.Reason)
		}
	}
	return reasons
}

func hasReason(skipped []discovery.SkippedWorktree, reason string) bool {
	for _, s := range skipped {
		if s.Reason == reason {
			return true
		}
	}
	return false
}

func wsFor(repos map[string]string) config.Workspace {
	m := make(map[string]config.Repo, len(repos))
	for name, primary := range repos {
		m[name] = config.Repo{Primary: primary, DefaultBranch: "main"}
	}
	return config.Workspace{
		Repos:    m,
		Grouping: config.Grouping{Strategy: "branch-name", StripPrefix: "jonny/"},
	}
}

// A detached worktree must show up in the skipped report (not silently vanish),
// while a healthy sibling worktree still becomes a slice.
func TestDiscoverReport_DetachedIsSurfaced(t *testing.T) {
	repo := testutil.NewRepo(t)
	goodWT := filepath.Join(t.TempDir(), "good")
	detachedWT := filepath.Join(t.TempDir(), "detached")

	testutil.AddWorktree(t, repo, "jonny/good", goodWT)
	addDetachedWorktree(t, repo, detachedWT)

	res := discovery.DiscoverReport(wsFor(map[string]string{"web": repo}))

	if len(res.Slices) != 1 || res.Slices[0].Name != "good" {
		t.Fatalf("expected slice 'good' to survive, got %+v", res.Slices)
	}
	got := reasonsFor(res.Skipped, resolvePath(t, detachedWT))
	if len(got) != 1 || got[0] != discovery.ReasonDetached {
		t.Fatalf("expected detached worktree surfaced as %q, got skipped=%+v",
			discovery.ReasonDetached, res.Skipped)
	}
}

// A broken (prunable) worktree — its directory deleted out from under git —
// must NOT wipe out the other, healthy slices. This is the fail-closed bug.
func TestDiscoverReport_PrunableDoesNotWipeOthers(t *testing.T) {
	repo := testutil.NewRepo(t)
	goodWT := filepath.Join(t.TempDir(), "good")
	brokenWT := filepath.Join(t.TempDir(), "broken")

	testutil.AddWorktree(t, repo, "jonny/good", goodWT)
	testutil.AddWorktree(t, repo, "jonny/broken", brokenWT)

	// Delete the broken worktree's working dir; git now reports it as prunable.
	if err := os.RemoveAll(brokenWT); err != nil {
		t.Fatalf("removing worktree dir: %v", err)
	}

	res := discovery.DiscoverReport(wsFor(map[string]string{"web": repo}))

	if len(res.Slices) != 1 || res.Slices[0].Name != "good" {
		t.Fatalf("healthy slice must survive a broken sibling, got %+v", res.Slices)
	}
	if !hasReason(res.Skipped, discovery.ReasonPrunable) {
		t.Fatalf("expected a prunable skip, got %+v", res.Skipped)
	}
}

// A worktree whose HEAD cannot be resolved (rev-parse fails) must be skipped in
// isolation with reason rev-parse-failed, leaving the other slices intact.
func TestDiscoverReport_RevParseFailureIsolated(t *testing.T) {
	repo := testutil.NewRepo(t)
	goodWT := filepath.Join(t.TempDir(), "good")
	corruptWT := filepath.Join(t.TempDir(), "corrupt")

	testutil.AddWorktree(t, repo, "jonny/good", goodWT)
	testutil.AddWorktree(t, repo, "jonny/corrupt", corruptWT)

	// Point the worktree's HEAD at a ref that does not exist: it still lists a
	// branch (not detached/branchless/prunable) but `rev-parse HEAD` fails.
	admin := adminGitDir(t, corruptWT)
	if err := os.WriteFile(filepath.Join(admin, "HEAD"), []byte("ref: refs/heads/ghost\n"), 0o644); err != nil {
		t.Fatalf("corrupting worktree HEAD: %v", err)
	}

	res := discovery.DiscoverReport(wsFor(map[string]string{"web": repo}))

	if len(res.Slices) != 1 || res.Slices[0].Name != "good" {
		t.Fatalf("healthy slice must survive a rev-parse failure, got %+v", res.Slices)
	}
	if !hasReason(res.Skipped, discovery.ReasonRevParseFailed) {
		t.Fatalf("expected a rev-parse-failed skip, got %+v", res.Skipped)
	}
}

// Two worktrees in the same repo that collapse to the same slice key must keep
// one member and surface the loser as grouping-collision instead of silently
// overwriting.
func TestDiscoverReport_GroupingCollisionSurfaced(t *testing.T) {
	repo := testutil.NewRepo(t)
	prefixedWT := filepath.Join(t.TempDir(), "prefixed")
	bareNameWT := filepath.Join(t.TempDir(), "barename")

	// "jonny/dup" and "dup" both strip to slice key "dup".
	testutil.AddWorktree(t, repo, "jonny/dup", prefixedWT)
	testutil.AddWorktree(t, repo, "dup", bareNameWT)

	res := discovery.DiscoverReport(wsFor(map[string]string{"web": repo}))

	if len(res.Slices) != 1 || res.Slices[0].Name != "dup" {
		t.Fatalf("expected single 'dup' slice, got %+v", res.Slices)
	}
	if len(res.Slices[0].Members) != 1 {
		t.Fatalf("collision must keep exactly one member, got %+v", res.Slices[0].Members)
	}
	if !hasReason(res.Skipped, discovery.ReasonGroupingCollision) {
		t.Fatalf("expected a grouping-collision skip, got %+v", res.Skipped)
	}
}

// A repo whose worktree listing fails entirely (its primary is not a git repo)
// must be recorded in RepoErrors while the other repos discover normally.
func TestDiscoverReport_RepoListFailureRecorded(t *testing.T) {
	goodRepo := testutil.NewRepo(t)
	goodWT := filepath.Join(t.TempDir(), "good")
	testutil.AddWorktree(t, goodRepo, "jonny/good", goodWT)

	brokenRepo := t.TempDir() // not a git repo

	res := discovery.DiscoverReport(wsFor(map[string]string{
		"web":    goodRepo,
		"broken": brokenRepo,
	}))

	if len(res.Slices) != 1 || res.Slices[0].Name != "good" {
		t.Fatalf("good repo must still discover, got %+v", res.Slices)
	}
	if len(res.RepoErrors) != 1 || res.RepoErrors[0].Repo != "broken" {
		t.Fatalf("expected a RepoError for 'broken', got %+v", res.RepoErrors)
	}
	if res.RepoErrors[0].Err == "" {
		t.Fatalf("RepoError must carry a non-empty message")
	}
}

func resolvePath(t *testing.T, p string) string {
	t.Helper()
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}
