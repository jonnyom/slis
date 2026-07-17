package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

func TestIsDirty(t *testing.T) {
	repo := testutil.NewRepo(t)

	// Clean repo should not be dirty.
	dirty, err := git.IsDirty(repo)
	if err != nil {
		t.Fatalf("IsDirty on clean repo: %v", err)
	}
	if dirty {
		t.Error("IsDirty = true on clean repo, want false")
	}

	// Writing an untracked file makes the repo dirty.
	if err := os.WriteFile(filepath.Join(repo, "x.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	dirty, err = git.IsDirty(repo)
	if err != nil {
		t.Fatalf("IsDirty after writing file: %v", err)
	}
	if !dirty {
		t.Error("IsDirty = false after writing untracked file, want true")
	}
}

func TestRevParse(t *testing.T) {
	repo := testutil.NewRepo(t)

	sha, err := git.RevParse(repo, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("RevParse returned %q (len=%d), want a 40-char SHA", sha, len(sha))
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := testutil.NewRepo(t)

	// On a normal checkout, CurrentBranch returns "main".
	branch, err := git.CurrentBranch(repo)
	if err != nil {
		t.Fatalf("CurrentBranch on normal checkout: %v", err)
	}
	if branch != "main" {
		t.Errorf("CurrentBranch = %q, want %q", branch, "main")
	}

	// Detach HEAD; CurrentBranch should return "".
	sha, err := git.RevParse(repo, "HEAD")
	if err != nil {
		t.Fatalf("RevParse for detach: %v", err)
	}
	if _, err := git.Run(repo, "checkout", "--detach", sha); err != nil {
		t.Fatalf("checkout --detach: %v", err)
	}
	branch, err = git.CurrentBranch(repo)
	if err != nil {
		t.Fatalf("CurrentBranch in detached HEAD: %v", err)
	}
	if branch != "" {
		t.Errorf("CurrentBranch in detached HEAD = %q, want %q", branch, "")
	}
}

func TestIsMergedInto(t *testing.T) {
	repo := testutil.NewRepo(t)

	// A branch pointing at the same commit as main is trivially an ancestor.
	if _, err := git.Run(repo, "branch", "even", "main"); err != nil {
		t.Fatalf("branch even: %v", err)
	}
	if !git.IsMergedInto(repo, "even", "main") {
		t.Error("IsMergedInto(even, main) = false, want true (no divergence)")
	}

	// An unmerged branch that adds a commit is NOT an ancestor of main.
	wt := filepath.Join(t.TempDir(), "wt")
	if out, err := git.Run(repo, "worktree", "add", "-b", "feat", wt); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(wt, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(wt, "add", "f.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(wt, "commit", "-m", "feat work"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if git.IsMergedInto(repo, "feat", "main") {
		t.Error("IsMergedInto(feat, main) = true, want false (unmerged commit)")
	}

	// A (non-checked-out) trunk branch that has feat as an ancestor counts feat
	// as merged into it.
	if _, err := git.Run(repo, "branch", "trunk2", "feat"); err != nil {
		t.Fatalf("branch trunk2: %v", err)
	}
	if !git.IsMergedInto(repo, "feat", "trunk2") {
		t.Error("IsMergedInto(feat, trunk2) = false after merge, want true")
	}

	// Empty refs are never merged.
	if git.IsMergedInto(repo, "", "main") || git.IsMergedInto(repo, "feat", "") {
		t.Error("IsMergedInto with empty ref should be false")
	}
}
