package diff_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/testutil"
)

// commitInDir commits all staged changes in dir with a message, using a fixed identity.
func commitInDir(t *testing.T, dir, msg string) {
	t.Helper()
	if _, err := git.Run(dir, "config", "user.email", "t@t"); err != nil {
		t.Fatalf("config email: %v", err)
	}
	if _, err := git.Run(dir, "config", "user.name", "t"); err != nil {
		t.Fatalf("config name: %v", err)
	}
	if _, err := git.Run(dir, "commit", "-m", msg); err != nil {
		t.Fatalf("commit %q: %v", msg, err)
	}
}

func TestSliceDiffCountsChanges(t *testing.T) {
	// Create base repo on main with one file.
	repo := testutil.NewRepo(t)

	// Write a.txt on main and commit it.
	aPath := filepath.Join(repo, "a.txt")
	if err := os.WriteFile(aPath, []byte("line1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(repo, "add", "a.txt"); err != nil {
		t.Fatal(err)
	}
	commitInDir(t, repo, "add a.txt on main")

	// Create worktree on branch "feat".
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat", wt)

	// In the worktree: modify a.txt (add 2 lines) and add b.txt.
	if err := os.WriteFile(filepath.Join(wt, "a.txt"), []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "b.txt"), []byte("x\ny\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(wt, "add", "a.txt", "b.txt"); err != nil {
		t.Fatal(err)
	}
	commitInDir(t, wt, "feat changes")

	sl := model.Slice{
		Name: "s",
		Members: map[string]model.SliceMember{
			"r": {Repo: "r", Branch: "feat", WorktreePath: wt},
		},
	}

	diffs, err := diff.SliceDiff(sl, "main")
	if err != nil {
		t.Fatalf("SliceDiff: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 RepoDiff, got %d", len(diffs))
	}

	rd := diffs[0]
	if rd.Repo != "r" {
		t.Errorf("Repo = %q, want %q", rd.Repo, "r")
	}
	if rd.Err != "" {
		t.Errorf("unexpected Err: %s", rd.Err)
	}
	if !strings.Contains(rd.Patch, "b.txt") {
		t.Errorf("Patch does not contain b.txt:\n%s", rd.Patch)
	}

	// Build a map of filename -> FileStat for easy lookup.
	statsMap := make(map[string]diff.FileStat)
	for _, fs := range rd.Files {
		statsMap[fs.Path] = fs
	}

	// a.txt: we added 2 lines (line2, line3); original had 1 line → no deletions.
	aStat, ok := statsMap["a.txt"]
	if !ok {
		t.Fatal("a.txt not found in Files")
	}
	if aStat.Added < 2 {
		t.Errorf("a.txt Added = %d, want >= 2", aStat.Added)
	}

	// b.txt: brand new file with 2 lines, no deletions.
	bStat, ok := statsMap["b.txt"]
	if !ok {
		t.Fatal("b.txt not found in Files")
	}
	if bStat.Added != 2 {
		t.Errorf("b.txt Added = %d, want 2", bStat.Added)
	}
	if bStat.Deleted != 0 {
		t.Errorf("b.txt Deleted = %d, want 0", bStat.Deleted)
	}

	// TotalAdded helper should reflect sum of Added across non-binary files.
	total := rd.TotalAdded()
	if total < 4 {
		t.Errorf("TotalAdded = %d, want >= 4", total)
	}
}

func TestSliceDiffIncludesUncommitted(t *testing.T) {
	// A slice's in-progress (uncommitted) work must show in the diff — the agent
	// edits before it commits, and the cockpit should reflect that.
	repo := testutil.NewRepo(t)

	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("line1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(repo, "add", "a.txt"); err != nil {
		t.Fatal(err)
	}
	commitInDir(t, repo, "add a.txt on main")

	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat", wt)

	// Modify a.txt (unstaged) and add a new b.txt (staged) — but DO NOT commit.
	if err := os.WriteFile(filepath.Join(wt, "a.txt"), []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "b.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(wt, "add", "b.txt"); err != nil {
		t.Fatal(err)
	}

	sl := model.Slice{
		Name: "s",
		Members: map[string]model.SliceMember{
			"r": {Repo: "r", Branch: "feat", WorktreePath: wt},
		},
	}
	diffs, err := diff.SliceDiff(sl, "main")
	if err != nil {
		t.Fatalf("SliceDiff: %v", err)
	}
	statsMap := make(map[string]diff.FileStat)
	for _, fs := range diffs[0].Files {
		statsMap[fs.Path] = fs
	}
	if _, ok := statsMap["a.txt"]; !ok {
		t.Error("uncommitted (unstaged) a.txt modification not shown in diff")
	}
	if _, ok := statsMap["b.txt"]; !ok {
		t.Error("uncommitted (staged) new file b.txt not shown in diff")
	}
	if !strings.Contains(diffs[0].Patch, "b.txt") {
		t.Errorf("patch missing uncommitted b.txt:\n%s", diffs[0].Patch)
	}
}

func TestSliceDiffBinaryAndMissingBase(t *testing.T) {
	// Create a fresh repo and a worktree; the base ref "nonexistent-base" won't exist.
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat2", wt)

	sl := model.Slice{
		Name: "s2",
		Members: map[string]model.SliceMember{
			"bad-repo": {Repo: "bad-repo", Branch: "feat2", WorktreePath: wt},
		},
	}

	diffs, err := diff.SliceDiff(sl, "nonexistent-base")
	// The function should NOT return an error — per-repo errors are isolated.
	if err != nil {
		t.Fatalf("SliceDiff returned unexpected error: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 RepoDiff, got %d", len(diffs))
	}
	if diffs[0].Err == "" {
		t.Error("expected non-empty Err for missing base ref, got empty")
	}
}

func TestSliceDiffSortedOrder(t *testing.T) {
	// Two members in a slice; verify results come back in sorted repo order.
	repoA := testutil.NewRepo(t)
	wtA := filepath.Join(t.TempDir(), "wtA")
	testutil.AddWorktree(t, repoA, "featA", wtA)

	repoB := testutil.NewRepo(t)
	wtB := filepath.Join(t.TempDir(), "wtB")
	testutil.AddWorktree(t, repoB, "featB", wtB)

	sl := model.Slice{
		Name: "multi",
		Members: map[string]model.SliceMember{
			"z-repo": {Repo: "z-repo", Branch: "featA", WorktreePath: wtA},
			"a-repo": {Repo: "a-repo", Branch: "featB", WorktreePath: wtB},
		},
	}

	diffs, err := diff.SliceDiff(sl, "main")
	if err != nil {
		t.Fatalf("SliceDiff: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 RepoDiffs, got %d", len(diffs))
	}
	if diffs[0].Repo != "a-repo" || diffs[1].Repo != "z-repo" {
		t.Errorf("expected sorted order [a-repo, z-repo], got [%s, %s]", diffs[0].Repo, diffs[1].Repo)
	}
}
