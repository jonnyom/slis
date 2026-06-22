package swap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

// ---------------------------------------------------------------------------
// deactivateRepo tests
// ---------------------------------------------------------------------------

// TestDeactivateRestoresExactly verifies that after activate (clean case),
// deactivateRepo returns the primary to its prior branch at the exact prior SHA.
func TestDeactivateRestoresExactly(t *testing.T) {
	r, _ := setupRepoWithWorktree(t)

	// Record primary HEAD before activation.
	priorHEAD, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse before activate: %v", err)
	}

	st, err := activateRepo(RepoPlan{Primary: r, Branch: "feat"})
	if err != nil {
		t.Fatalf("activateRepo: %v", err)
	}

	if err := deactivateRepo(st); err != nil {
		t.Fatalf("deactivateRepo: %v", err)
	}

	branch, err := git.CurrentBranch(r)
	if err != nil {
		t.Fatalf("CurrentBranch after deactivate: %v", err)
	}
	if branch != "main" {
		t.Errorf("branch after deactivate: want %q, got %q", "main", branch)
	}

	head, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse after deactivate: %v", err)
	}
	if head != priorHEAD {
		t.Errorf("HEAD after deactivate: want %q, got %q", priorHEAD, head)
	}
}

// TestDeactivateRestoresStashedEdits verifies that dirty edits stashed during
// activation are exactly restored (pop) during deactivation.
func TestDeactivateRestoresStashedEdits(t *testing.T) {
	r, _ := setupRepoWithWorktree(t)

	// Commit a tracked file on main so stash works on a tracked file.
	sharedPath := filepath.Join(r, "shared.txt")
	if err := os.WriteFile(sharedPath, []byte("base"), 0o644); err != nil {
		t.Fatalf("write shared.txt: %v", err)
	}
	if _, err := git.Run(r, "add", "shared.txt"); err != nil {
		t.Fatalf("git add shared.txt: %v", err)
	}
	if _, err := git.Run(r, "commit", "-q", "-m", "add shared.txt"); err != nil {
		t.Fatalf("git commit shared.txt: %v", err)
	}

	// Dirty edit to the tracked file.
	if err := os.WriteFile(sharedPath, []byte("wip"), 0o644); err != nil {
		t.Fatalf("write wip: %v", err)
	}

	st, err := activateRepo(RepoPlan{Primary: r, Branch: "feat", Stash: true})
	if err != nil {
		t.Fatalf("activateRepo: %v", err)
	}

	if err := deactivateRepo(st); err != nil {
		t.Fatalf("deactivateRepo: %v", err)
	}

	// Branch must be restored.
	branch, err := git.CurrentBranch(r)
	if err != nil {
		t.Fatalf("CurrentBranch after deactivate: %v", err)
	}
	if branch != "main" {
		t.Errorf("branch after deactivate: want %q, got %q", "main", branch)
	}

	// Stashed edit must be back.
	content, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("read shared.txt: %v", err)
	}
	if string(content) != "wip" {
		t.Errorf("shared.txt after deactivate: want %q, got %q", "wip", string(content))
	}
}

// TestDeactivateStashConflictSurfaces verifies that when popping the stash
// causes a merge conflict, deactivateRepo returns ErrStashConflict and leaves
// the stash intact (i.e. does NOT silently discard it).
func TestDeactivateStashConflictSurfaces(t *testing.T) {
	r, _ := setupRepoWithWorktree(t)

	// Commit a tracked file on main.
	sharedPath := filepath.Join(r, "shared.txt")
	if err := os.WriteFile(sharedPath, []byte("base"), 0o644); err != nil {
		t.Fatalf("write shared.txt: %v", err)
	}
	if _, err := git.Run(r, "add", "shared.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(r, "commit", "-q", "-m", "add shared.txt"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Dirty edit on main (will be stashed during activate).
	if err := os.WriteFile(sharedPath, []byte("wip"), 0o644); err != nil {
		t.Fatalf("write wip: %v", err)
	}

	st, err := activateRepo(RepoPlan{Primary: r, Branch: "feat", Stash: true})
	if err != nil {
		t.Fatalf("activateRepo: %v", err)
	}

	// Advance `main` to a conflicting commit using a second worktree on main.
	main2Path := filepath.Join(t.TempDir(), "main2")
	if _, err := git.Run(r, "worktree", "add", main2Path, "main"); err != nil {
		t.Fatalf("worktree add main2: %v", err)
	}
	shared2 := filepath.Join(main2Path, "shared.txt")
	if err := os.WriteFile(shared2, []byte("other"), 0o644); err != nil {
		t.Fatalf("write other: %v", err)
	}
	if _, err := git.Run(main2Path, "add", "shared.txt"); err != nil {
		t.Fatalf("git add in main2: %v", err)
	}
	if _, err := git.Run(main2Path, "commit", "-q", "-m", "conflict commit"); err != nil {
		t.Fatalf("git commit in main2: %v", err)
	}
	// Remove the extra worktree; we only needed it to advance the main branch ref.
	if _, err := git.Run(r, "worktree", "remove", "--force", main2Path); err != nil {
		t.Fatalf("worktree remove main2: %v", err)
	}

	// deactivate: switches primary back to main (shared.txt="other"), then pops
	// stash (base→wip) → CONFLICT.
	err = deactivateRepo(st)
	if !errors.Is(err, ErrStashConflict) {
		t.Fatalf("want ErrStashConflict, got %v", err)
	}

	// Stash must still be present (not silently dropped).
	out, listErr := git.Run(r, "stash", "list", "--format=%H")
	if listErr != nil {
		t.Fatalf("stash list: %v", listErr)
	}
	found := false
	for _, line := range splitLines(out) {
		if line == st.StashRef {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("stash %q was dropped after conflict — want it intact", st.StashRef)
	}
}

// splitLines splits s into non-empty lines.
func splitLines(s string) []string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
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
