package swap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

// helpers to set up a repo+worktree pair with a commit on the worktree branch.
func setupRepoWithWorktree(t *testing.T) (primary, wt string) {
	t.Helper()
	r := testutil.NewRepo(t)
	wtPath := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, r, "feat", wtPath)

	// Commit a file in the worktree so feat tip != main tip.
	if err := os.WriteFile(filepath.Join(wtPath, "f.txt"), []byte("feat work\n"), 0o644); err != nil {
		t.Fatalf("write f.txt: %v", err)
	}
	if _, err := git.Run(wtPath, "add", "f.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(wtPath, "commit", "-q", "-m", "feat work"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	return r, wtPath
}

// TestActivateRepoDetachesPrimaryNotWorktree verifies the core invariant:
// after activateRepo the primary is detached at feat's tip, and the worktree
// is completely untouched.
func TestActivateRepoDetachesPrimaryNotWorktree(t *testing.T) {
	r, wt := setupRepoWithWorktree(t)

	st, err := activateRepo(RepoPlan{Primary: r, Branch: "feat", Stash: false})
	if err != nil {
		t.Fatalf("activateRepo: unexpected error: %v", err)
	}

	// Primary must be detached (CurrentBranch returns "").
	branch, err := git.CurrentBranch(r)
	if err != nil {
		t.Fatalf("CurrentBranch(primary): %v", err)
	}
	if branch != "" {
		t.Errorf("primary: want detached HEAD (branch==\"\"), got %q", branch)
	}

	// Primary HEAD must equal feat tip.
	primaryHEAD, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse(primary, HEAD): %v", err)
	}
	wtHEAD, err := git.RevParse(wt, "HEAD")
	if err != nil {
		t.Fatalf("RevParse(wt, HEAD): %v", err)
	}
	if primaryHEAD != wtHEAD {
		t.Errorf("primary HEAD %q != wt HEAD %q", primaryHEAD, wtHEAD)
	}

	// Worktree must still be on branch feat.
	wtBranch, err := git.CurrentBranch(wt)
	if err != nil {
		t.Fatalf("CurrentBranch(wt): %v", err)
	}
	if wtBranch != "feat" {
		t.Errorf("worktree: want branch %q, got %q", "feat", wtBranch)
	}

	// PriorBranch must record where primary was before activation.
	if st.PriorBranch != "main" {
		t.Errorf("PriorBranch: want %q, got %q", "main", st.PriorBranch)
	}

	// TargetSHA must match the feat tip.
	if st.TargetSHA != wtHEAD {
		t.Errorf("TargetSHA: want %q, got %q", wtHEAD, st.TargetSHA)
	}
}

// TestActivateRefusesDirtyWithoutStash verifies that a dirty primary with
// Stash:false returns an error and makes zero changes to HEAD.
func TestActivateRefusesDirtyWithoutStash(t *testing.T) {
	r, _ := setupRepoWithWorktree(t)

	// Record primary HEAD before the attempt.
	headBefore, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse before: %v", err)
	}

	// Write an untracked file to make the primary dirty.
	if err := os.WriteFile(filepath.Join(r, "dirty.txt"), []byte("oops\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}

	_, err = activateRepo(RepoPlan{Primary: r, Branch: "feat", Stash: false})
	if err == nil {
		t.Fatal("expected error for dirty primary with Stash:false, got nil")
	}

	// HEAD must be unchanged.
	headAfter, err2 := git.RevParse(r, "HEAD")
	if err2 != nil {
		t.Fatalf("RevParse after: %v", err2)
	}
	if headBefore != headAfter {
		t.Errorf("HEAD changed despite error: before %q after %q", headBefore, headAfter)
	}

	// Must still be on main.
	branch, err3 := git.CurrentBranch(r)
	if err3 != nil {
		t.Fatalf("CurrentBranch after: %v", err3)
	}
	if branch != "main" {
		t.Errorf("branch changed: want %q, got %q", "main", branch)
	}
}

// TestActivateStashesDirty verifies that a dirty primary with Stash:true
// succeeds: the primary is detached at feat tip, StashRef is set, and
// the primary working tree is clean.
func TestActivateStashesDirty(t *testing.T) {
	r, wt := setupRepoWithWorktree(t)

	// Write an untracked file to make the primary dirty.
	if err := os.WriteFile(filepath.Join(r, "dirty.txt"), []byte("work in progress\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}

	st, err := activateRepo(RepoPlan{Primary: r, Branch: "feat", Stash: true})
	if err != nil {
		t.Fatalf("activateRepo: unexpected error: %v", err)
	}

	// StashRef must be a non-empty SHA.
	if st.StashRef == "" {
		t.Error("StashRef: want non-empty, got empty")
	}

	// Primary must be clean now.
	dirty, err := git.IsDirty(r)
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if dirty {
		t.Error("primary is still dirty after stash+activate")
	}

	// Primary HEAD must be at feat tip.
	primaryHEAD, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse(primary, HEAD): %v", err)
	}
	wtHEAD, err := git.RevParse(wt, "HEAD")
	if err != nil {
		t.Fatalf("RevParse(wt, HEAD): %v", err)
	}
	if primaryHEAD != wtHEAD {
		t.Errorf("primary HEAD %q != wt HEAD %q", primaryHEAD, wtHEAD)
	}

	// Primary must be detached.
	branch, err := git.CurrentBranch(r)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "" {
		t.Errorf("primary: want detached HEAD, got branch %q", branch)
	}
}
