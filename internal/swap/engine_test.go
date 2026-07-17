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

	if err := deactivateRepo("", st, false); err != nil {
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

	if err := deactivateRepo("", st, false); err != nil {
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
	err = deactivateRepo("", st, false)
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

// ---------------------------------------------------------------------------
// Multi-repo Activate / Deactivate helpers
// ---------------------------------------------------------------------------

// setupRepoWithFeatBranch creates a bare repo + worktree on "feat" with one
// commit on feat, and returns (primaryDir, wtDir). The primary stays on "main".
func setupRepoWithFeatBranch(t *testing.T) (primary, wt string) {
	t.Helper()
	return setupRepoWithWorktree(t)
}

// ---------------------------------------------------------------------------
// TestActivateSliceWritesJournal
// ---------------------------------------------------------------------------

func TestActivateSliceWritesJournal(t *testing.T) {
	rA, _ := setupRepoWithFeatBranch(t)
	rB, _ := setupRepoWithFeatBranch(t)
	rC, _ := setupRepoWithFeatBranch(t)

	journalPath := filepath.Join(t.TempDir(), "active.json")

	repos := []RepoActivation{
		{Repo: "a", Primary: rA, Branch: "feat"},
		{Repo: "b", Primary: rB, Branch: "feat"},
		{Repo: "c", Primary: rC, Branch: "feat"},
	}

	j, err := Activate("myslice", repos, journalPath, ActivateOptions{})
	if err != nil {
		t.Fatalf("Activate: unexpected error: %v", err)
	}

	// Journal returned in-memory must have 3 repos.
	if len(j.Repos) != 3 {
		t.Errorf("j.Repos: want 3, got %d", len(j.Repos))
	}
	if j.Slice != "myslice" {
		t.Errorf("j.Slice: want %q, got %q", "myslice", j.Slice)
	}

	// Journal on disk must also have 3 repos.
	loaded, err := Load(journalPath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load returned nil — journal not written")
	}
	if len(loaded.Repos) != 3 {
		t.Errorf("loaded.Repos: want 3, got %d", len(loaded.Repos))
	}

	// Each primary must be detached at its feat tip.
	for _, ra := range repos {
		branch, err := git.CurrentBranch(ra.Primary)
		if err != nil {
			t.Fatalf("CurrentBranch(%s): %v", ra.Repo, err)
		}
		if branch != "" {
			t.Errorf("repo %s: want detached HEAD, got branch %q", ra.Repo, branch)
		}
	}
}

// ---------------------------------------------------------------------------
// TestActivateSliceAtomicRollback
// ---------------------------------------------------------------------------

func TestActivateSliceAtomicRollback(t *testing.T) {
	rA, _ := setupRepoWithFeatBranch(t)
	rB, _ := setupRepoWithFeatBranch(t)
	rC, _ := setupRepoWithFeatBranch(t)

	journalPath := filepath.Join(t.TempDir(), "active.json")

	// Record prior HEADs.
	headA, err := git.RevParse(rA, "HEAD")
	if err != nil {
		t.Fatalf("RevParse A: %v", err)
	}
	headB, err := git.RevParse(rB, "HEAD")
	if err != nil {
		t.Fatalf("RevParse B: %v", err)
	}

	// Repo C gets a non-existent branch — will fail.
	repos := []RepoActivation{
		{Repo: "a", Primary: rA, Branch: "feat"},
		{Repo: "b", Primary: rB, Branch: "feat"},
		{Repo: "c", Primary: rC, Branch: "does-not-exist"},
	}

	_, err = Activate("myslice", repos, journalPath, ActivateOptions{})
	if err == nil {
		t.Fatal("Activate: expected error for bad branch, got nil")
	}

	// Repos A and B must be rolled back to "main" at their prior HEADs.
	for _, tc := range []struct {
		name     string
		primary  string
		priorHEA string
	}{
		{"a", rA, headA},
		{"b", rB, headB},
	} {
		branch, err := git.CurrentBranch(tc.primary)
		if err != nil {
			t.Fatalf("CurrentBranch(%s): %v", tc.name, err)
		}
		if branch != "main" {
			t.Errorf("repo %s after rollback: want branch %q, got %q", tc.name, "main", branch)
		}

		head, err := git.RevParse(tc.primary, "HEAD")
		if err != nil {
			t.Fatalf("RevParse(%s): %v", tc.name, err)
		}
		if head != tc.priorHEA {
			t.Errorf("repo %s HEAD after rollback: want %q, got %q", tc.name, tc.priorHEA, head)
		}
	}

	// No journal must have been written.
	loaded, err := Load(journalPath)
	if err != nil {
		t.Fatalf("Load after failed Activate: %v", err)
	}
	if loaded != nil {
		t.Error("journal was written on failed Activate — must not write journal on rollback")
	}
}

// ---------------------------------------------------------------------------
// TestDeactivateSliceClearsJournal
// ---------------------------------------------------------------------------

func TestDeactivateSliceClearsJournal(t *testing.T) {
	rA, _ := setupRepoWithFeatBranch(t)
	rB, _ := setupRepoWithFeatBranch(t)

	journalPath := filepath.Join(t.TempDir(), "active.json")

	repos := []RepoActivation{
		{Repo: "a", Primary: rA, Branch: "feat"},
		{Repo: "b", Primary: rB, Branch: "feat"},
	}

	if _, err := Activate("myslice", repos, journalPath, ActivateOptions{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if err := Deactivate(journalPath, false); err != nil {
		t.Fatalf("Deactivate: unexpected error: %v", err)
	}

	// Journal must be cleared.
	loaded, err := Load(journalPath)
	if err != nil {
		t.Fatalf("Load after Deactivate: %v", err)
	}
	if loaded != nil {
		t.Error("journal still present after Deactivate")
	}

	// Both primaries must be back on "main".
	for _, ra := range repos {
		branch, err := git.CurrentBranch(ra.Primary)
		if err != nil {
			t.Fatalf("CurrentBranch(%s): %v", ra.Repo, err)
		}
		if branch != "main" {
			t.Errorf("repo %s after Deactivate: want branch %q, got %q", ra.Repo, "main", branch)
		}
	}
}

// ---------------------------------------------------------------------------
// TestDepReconcileInvokesInstaller
// ---------------------------------------------------------------------------

// setupRepoWithLockfileDelta creates a repo where "main" has pnpm-lock.yaml="v1"
// and "feat" worktree has pnpm-lock.yaml="v2", so LockfilesChanged returns true.
func setupRepoWithLockfileDelta(t *testing.T) (primary, wt string) {
	t.Helper()
	r := testutil.NewRepo(t)

	// Commit pnpm-lock.yaml on main.
	if err := os.WriteFile(filepath.Join(r, "pnpm-lock.yaml"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write pnpm-lock.yaml: %v", err)
	}
	if _, err := git.Run(r, "add", "pnpm-lock.yaml"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(r, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "add lockfile"); err != nil {
		t.Fatalf("git commit lockfile: %v", err)
	}

	// Create feat worktree.
	wtPath := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, r, "feat", wtPath)

	// Commit pnpm-lock.yaml="v2" on feat.
	if err := os.WriteFile(filepath.Join(wtPath, "pnpm-lock.yaml"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write pnpm-lock.yaml feat: %v", err)
	}
	if _, err := git.Run(wtPath, "add", "pnpm-lock.yaml"); err != nil {
		t.Fatalf("git add feat: %v", err)
	}
	if _, err := git.Run(wtPath, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "bump lockfile"); err != nil {
		t.Fatalf("git commit feat lockfile: %v", err)
	}

	return r, wtPath
}

func TestDepReconcileInvokesInstaller(t *testing.T) {
	t.Run("lockfile changed — installer called", func(t *testing.T) {
		primary, _ := setupRepoWithLockfileDelta(t)
		journalPath := filepath.Join(t.TempDir(), "active.json")

		var calls []string
		installer := func(dir string) error {
			calls = append(calls, dir)
			return nil
		}

		repos := []RepoActivation{
			{Repo: "x", Primary: primary, Branch: "feat", Lockfiles: []string{"pnpm-lock.yaml"}},
		}

		_, err := Activate("myslice", repos, journalPath, ActivateOptions{Installer: installer})
		if err != nil {
			t.Fatalf("Activate: %v", err)
		}

		if len(calls) != 1 || calls[0] != primary {
			t.Errorf("installer calls: want [%s], got %v", primary, calls)
		}
	})

	t.Run("lockfile unchanged — installer not called", func(t *testing.T) {
		// Use a plain repo+worktree with no lockfile changes.
		primary, _ := setupRepoWithFeatBranch(t)
		journalPath := filepath.Join(t.TempDir(), "active.json")

		var calls []string
		installer := func(dir string) error {
			calls = append(calls, dir)
			return nil
		}

		repos := []RepoActivation{
			{Repo: "y", Primary: primary, Branch: "feat", Lockfiles: []string{"pnpm-lock.yaml"}},
		}

		_, err := Activate("myslice", repos, journalPath, ActivateOptions{Installer: installer})
		if err != nil {
			t.Fatalf("Activate: %v", err)
		}

		if len(calls) != 0 {
			t.Errorf("installer should not have been called, but got calls: %v", calls)
		}
	})
}

// ---------------------------------------------------------------------------
// New tests for adversarial-review fixes
// ---------------------------------------------------------------------------

// TestDeactivateConflictKeepsJournal verifies that when a stash pop conflicts
// during Deactivate, the journal is NOT cleared. Instead, it is rewritten to
// contain only the repos that failed to restore, so `slis deactivate` can be
// re-run to resume.
func TestDeactivateConflictKeepsJournal(t *testing.T) {
	r, _ := setupRepoWithWorktree(t)

	// Commit a tracked file on main so stash works on a tracked file.
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

	journalPath := filepath.Join(t.TempDir(), "active.json")
	repos := []RepoActivation{
		{Repo: "r", Primary: r, Branch: "feat"},
	}

	_, err := Activate("myslice", repos, journalPath, ActivateOptions{Stash: true})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Advance main to a conflicting commit via a second worktree so the stash
	// pop will conflict when we deactivate.
	main2Path := filepath.Join(t.TempDir(), "main2")
	if _, err := git.Run(r, "worktree", "add", main2Path, "main"); err != nil {
		t.Fatalf("worktree add main2: %v", err)
	}
	shared2 := filepath.Join(main2Path, "shared.txt")
	if err := os.WriteFile(shared2, []byte("conflict"), 0o644); err != nil {
		t.Fatalf("write conflict: %v", err)
	}
	if _, err := git.Run(main2Path, "add", "shared.txt"); err != nil {
		t.Fatalf("git add in main2: %v", err)
	}
	if _, err := git.Run(main2Path, "commit", "-q", "-m", "conflict commit"); err != nil {
		t.Fatalf("git commit in main2: %v", err)
	}
	if _, err := git.Run(r, "worktree", "remove", "--force", main2Path); err != nil {
		t.Fatalf("worktree remove main2: %v", err)
	}

	// Deactivate should return a non-nil error (stash conflict).
	deactivateErr := Deactivate(journalPath, false)
	if deactivateErr == nil {
		t.Fatal("Deactivate: expected error on stash conflict, got nil")
	}

	// The journal must NOT have been cleared — it must still be loadable.
	loaded, err := Load(journalPath)
	if err != nil {
		t.Fatalf("Load after failed Deactivate: %v", err)
	}
	if loaded == nil {
		t.Fatal("journal was cleared after a failed Deactivate — it must be kept for resumability")
	}

	// The surviving journal must contain the conflicted repo.
	found := false
	for _, rs := range loaded.Repos {
		if rs.Repo == "r" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("conflicted repo %q not found in surviving journal: %+v", "r", loaded.Repos)
	}
}

// TestReconciledFalseOnInstallerError verifies that when the installer returns
// an error, RepoState.Reconciled is left false, and Activate returns a non-nil
// error alongside a non-nil journal (the swap itself succeeded).
func TestReconciledFalseOnInstallerError(t *testing.T) {
	primary, _ := setupRepoWithLockfileDelta(t)
	journalPath := filepath.Join(t.TempDir(), "active.json")

	installerErr := errors.New("pnpm install failed")
	installer := func(dir string) error { return installerErr }

	repos := []RepoActivation{
		{Repo: "x", Primary: primary, Branch: "feat", Lockfiles: []string{"pnpm-lock.yaml"}},
	}

	j, err := Activate("myslice", repos, journalPath, ActivateOptions{Installer: installer})

	// Activate must return a non-nil error (the installer failed).
	if err == nil {
		t.Fatal("Activate: expected non-nil error when installer fails, got nil")
	}

	// But it must also return a non-nil journal (the swap itself succeeded).
	if j == nil {
		t.Fatal("Activate: expected non-nil journal even when installer fails")
	}

	// Load the on-disk journal and verify Reconciled is false.
	loaded, loadErr := Load(journalPath)
	if loadErr != nil {
		t.Fatalf("Load: %v", loadErr)
	}
	if loaded == nil {
		t.Fatal("journal was not written despite successful swap")
	}
	if len(loaded.Repos) == 0 {
		t.Fatal("journal has no repos")
	}
	if loaded.Repos[0].Reconciled {
		t.Errorf("RepoState.Reconciled: want false when installer errors, got true")
	}
}

// ---------------------------------------------------------------------------
// TestRecoverState
// ---------------------------------------------------------------------------

// TestRecoverState verifies that RecoverState returns the in-progress journal
// after Activate, and nil after Deactivate.
func TestRecoverState(t *testing.T) {
	rA, _ := setupRepoWithFeatBranch(t)
	rB, _ := setupRepoWithFeatBranch(t)

	journalPath := filepath.Join(t.TempDir(), "active.json")

	repos := []RepoActivation{
		{Repo: "a", Primary: rA, Branch: "feat"},
		{Repo: "b", Primary: rB, Branch: "feat"},
	}

	if _, err := Activate("myslice", repos, journalPath, ActivateOptions{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// RecoverState must return a non-nil journal with the right slice + repos.
	j, err := RecoverState(journalPath)
	if err != nil {
		t.Fatalf("RecoverState: %v", err)
	}
	if j == nil {
		t.Fatal("RecoverState: want non-nil journal, got nil")
	}
	if j.Slice != "myslice" {
		t.Errorf("RecoverState: Slice: want %q, got %q", "myslice", j.Slice)
	}
	if len(j.Repos) != 2 {
		t.Errorf("RecoverState: len(Repos): want 2, got %d", len(j.Repos))
	}

	// After Deactivate, RecoverState must return nil.
	if err := Deactivate(journalPath, false); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	j2, err := RecoverState(journalPath)
	if err != nil {
		t.Fatalf("RecoverState after Deactivate: %v", err)
	}
	if j2 != nil {
		t.Errorf("RecoverState after Deactivate: want nil, got %+v", j2)
	}
}

// ---------------------------------------------------------------------------
// TestRefreshMovesToNewTip
// ---------------------------------------------------------------------------

// TestRefreshMovesToNewTip verifies that after a new commit lands on the feat
// branch (in the worktree), Refresh advances the primary's detached HEAD to
// the new tip and persists the updated TargetSHA in the journal.
func TestRefreshMovesToNewTip(t *testing.T) {
	r, wt := setupRepoWithWorktree(t)

	journalPath := filepath.Join(t.TempDir(), "active.json")

	if _, err := Activate("s", []RepoActivation{{Repo: "a", Primary: r, Branch: "feat"}}, journalPath, ActivateOptions{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Record the OLD feat tip (what the primary is currently detached at).
	oldTip, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse primary HEAD before advance: %v", err)
	}

	// Advance feat: commit a new file in the worktree.
	if err := os.WriteFile(filepath.Join(wt, "newfile.txt"), []byte("advance\n"), 0o644); err != nil {
		t.Fatalf("write newfile.txt: %v", err)
	}
	if _, err := git.Run(wt, "add", "newfile.txt"); err != nil {
		t.Fatalf("git add newfile.txt: %v", err)
	}
	if _, err := git.Run(wt, "commit", "-q", "-m", "advance feat"); err != nil {
		t.Fatalf("git commit advance: %v", err)
	}

	// The new feat tip is the worktree HEAD.
	newTip, err := git.RevParse(wt, "HEAD")
	if err != nil {
		t.Fatalf("RevParse wt HEAD: %v", err)
	}

	if newTip == oldTip {
		t.Fatal("worktree HEAD did not advance — test setup error")
	}

	// Refresh must advance the primary to the new tip.
	j2, err := Refresh(journalPath)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if j2 == nil {
		t.Fatal("Refresh: want non-nil journal, got nil")
	}

	// Primary HEAD must equal the new feat tip.
	primaryHEAD, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse primary HEAD after Refresh: %v", err)
	}
	if primaryHEAD != newTip {
		t.Errorf("primary HEAD after Refresh: want %q, got %q", newTip, primaryHEAD)
	}

	// In-memory journal TargetSHA must be updated.
	if j2.Repos[0].TargetSHA != newTip {
		t.Errorf("j2.Repos[0].TargetSHA: want %q, got %q", newTip, j2.Repos[0].TargetSHA)
	}

	// On-disk journal must also have the updated TargetSHA.
	loaded, err := Load(journalPath)
	if err != nil {
		t.Fatalf("Load after Refresh: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load after Refresh: journal cleared — unexpected")
	}
	if loaded.Repos[0].TargetSHA != newTip {
		t.Errorf("loaded.Repos[0].TargetSHA: want %q, got %q", newTip, loaded.Repos[0].TargetSHA)
	}

	// Worktree must still be on branch feat (untouched).
	wtBranch, err := git.CurrentBranch(wt)
	if err != nil {
		t.Fatalf("CurrentBranch(wt): %v", err)
	}
	if wtBranch != "feat" {
		t.Errorf("worktree branch after Refresh: want %q, got %q", "feat", wtBranch)
	}

	// Primary must still be detached.
	primaryBranch, err := git.CurrentBranch(r)
	if err != nil {
		t.Fatalf("CurrentBranch(primary): %v", err)
	}
	if primaryBranch != "" {
		t.Errorf("primary after Refresh: want detached HEAD, got branch %q", primaryBranch)
	}
}

// TestStaleRepos verifies the staleness comparison: a repo is stale only when
// its recorded TargetSHA differs from the current branch tip.
func TestStaleRepos(t *testing.T) {
	j := &Journal{Slice: "s", Repos: []RepoState{
		{Repo: "web", TargetSHA: "aaa"},
		{Repo: "api", TargetSHA: "bbb"},
	}}
	if got := StaleRepos(j, map[string]string{"web": "aaa", "api": "bbb"}); len(got) != 0 {
		t.Errorf("all tips match — want no stale repos, got %v", got)
	}
	got := StaleRepos(j, map[string]string{"web": "aaa", "api": "ZZZ"})
	if len(got) != 1 || got[0] != "api" {
		t.Errorf("api tip advanced — want [api], got %v", got)
	}
	// A repo with no known tip (or empty target) is skipped, not flagged.
	if got := StaleRepos(j, map[string]string{"web": ""}); len(got) != 0 {
		t.Errorf("missing/empty tips should be skipped, got %v", got)
	}
	if got := StaleRepos(nil, map[string]string{"web": "x"}); got != nil {
		t.Errorf("nil journal → nil, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Drift-detection / rescue tests (adversarial)
// ---------------------------------------------------------------------------

// commitOnDetachedPrimary makes a new commit directly on the (detached) primary
// and returns the new HEAD sha, simulating a user committing on a swapped-in
// primary.
func commitOnDetachedPrimary(t *testing.T, primary string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(primary, "on-detached.txt"), []byte("work on detached HEAD\n"), 0o644); err != nil {
		t.Fatalf("write on-detached.txt: %v", err)
	}
	if _, err := git.Run(primary, "add", "on-detached.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(primary, "commit", "-q", "-m", "work on detached HEAD"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	head, err := git.RevParse(primary, "HEAD")
	if err != nil {
		t.Fatalf("RevParse HEAD: %v", err)
	}
	return head
}

// TestDeactivateRefusesCommitOnDetachedPrimaryThenForceRescues verifies that a
// commit made on a detached primary blocks a plain deactivate (zero state
// change, no rescue branch), and that --force first rescues that commit to a
// slis/rescue/<slice>-<repo> branch before restoring.
func TestDeactivateRefusesCommitOnDetachedPrimaryThenForceRescues(t *testing.T) {
	r, _ := setupRepoWithWorktree(t)

	priorHEAD, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse prior HEAD: %v", err)
	}

	st, err := activateRepo(RepoPlan{Repo: "web", Primary: r, Branch: "feat"})
	if err != nil {
		t.Fatalf("activateRepo: %v", err)
	}

	// User commits on the detached primary — HEAD advances beyond TargetSHA.
	newHEAD := commitOnDetachedPrimary(t, r)
	if newHEAD == st.TargetSHA {
		t.Fatal("commit did not advance HEAD — test setup error")
	}

	// Plain deactivate must refuse with ErrPrimaryDrifted and change nothing.
	err = deactivateRepo("myslice", st, false)
	if !errors.Is(err, ErrPrimaryDrifted) {
		t.Fatalf("deactivateRepo (no force): want ErrPrimaryDrifted, got %v", err)
	}
	afterHEAD, _ := git.RevParse(r, "HEAD")
	if afterHEAD != newHEAD {
		t.Errorf("HEAD changed on refused deactivate: want %q, got %q", newHEAD, afterHEAD)
	}
	rescue := "slis/rescue/myslice-web"
	if git.RefExists(r, "refs/heads/"+rescue) {
		t.Errorf("rescue branch %q created without --force", rescue)
	}

	// Forced deactivate rescues the commit, then restores to the prior branch.
	if err := deactivateRepo("myslice", st, true); err != nil {
		t.Fatalf("deactivateRepo (force): %v", err)
	}
	if !git.RefExists(r, "refs/heads/"+rescue) {
		t.Fatalf("rescue branch %q was not created under --force", rescue)
	}
	rescueTip, err := git.RevParse(r, rescue)
	if err != nil {
		t.Fatalf("RevParse rescue branch: %v", err)
	}
	if rescueTip != newHEAD {
		t.Errorf("rescue branch tip: want %q (the detached commit), got %q", newHEAD, rescueTip)
	}
	branch, _ := git.CurrentBranch(r)
	if branch != "main" {
		t.Errorf("branch after forced deactivate: want %q, got %q", "main", branch)
	}
	head, _ := git.RevParse(r, "HEAD")
	if head != priorHEAD {
		t.Errorf("HEAD after forced deactivate: want prior %q, got %q", priorHEAD, head)
	}
}

// TestDeactivateRefusesManualSwitchDrift verifies that when the user manually
// switches the primary to another branch (no commits made on the detached
// HEAD), a plain deactivate refuses cleanly with zero state change.
func TestDeactivateRefusesManualSwitchDrift(t *testing.T) {
	r, _ := setupRepoWithWorktree(t)

	st, err := activateRepo(RepoPlan{Repo: "web", Primary: r, Branch: "feat"})
	if err != nil {
		t.Fatalf("activateRepo: %v", err)
	}

	// User manually switches the primary back to main.
	if _, err := git.Run(r, "switch", "main"); err != nil {
		t.Fatalf("manual switch to main: %v", err)
	}

	err = deactivateRepo("myslice", st, false)
	if !errors.Is(err, ErrPrimaryDrifted) {
		t.Fatalf("want ErrPrimaryDrifted, got %v", err)
	}

	// State unchanged: still on main.
	branch, _ := git.CurrentBranch(r)
	if branch != "main" {
		t.Errorf("branch after refused deactivate: want %q, got %q", "main", branch)
	}
}

// TestDeactivateRefusesWhenPriorBranchGone verifies that if the branch the
// primary was on before activation has been deleted, deactivate errors with
// ErrPriorBranchGone rather than silently detaching.
func TestDeactivateRefusesWhenPriorBranchGone(t *testing.T) {
	r, _ := setupRepoWithWorktree(t)

	// Put the primary on a deletable branch "prior" before activating.
	if _, err := git.Run(r, "switch", "-c", "prior"); err != nil {
		t.Fatalf("create prior branch: %v", err)
	}

	st, err := activateRepo(RepoPlan{Repo: "web", Primary: r, Branch: "feat"})
	if err != nil {
		t.Fatalf("activateRepo: %v", err)
	}
	if st.PriorBranch != "prior" {
		t.Fatalf("PriorBranch: want %q, got %q", "prior", st.PriorBranch)
	}

	// Delete the prior branch while the primary is detached.
	if _, err := git.Run(r, "branch", "-D", "prior"); err != nil {
		t.Fatalf("delete prior branch: %v", err)
	}

	err = deactivateRepo("myslice", st, false)
	if !errors.Is(err, ErrPriorBranchGone) {
		t.Fatalf("want ErrPriorBranchGone, got %v", err)
	}

	// State unchanged: still detached at the slice tip.
	head, _ := git.RevParse(r, "HEAD")
	if head != st.TargetSHA {
		t.Errorf("HEAD changed on refused deactivate: want %q, got %q", st.TargetSHA, head)
	}
}

// TestRefreshRefusesDirtyPrimary verifies that Refresh refuses to advance a
// dirty primary and makes zero state change (mirrors activate's dirty guard).
func TestRefreshRefusesDirtyPrimary(t *testing.T) {
	r, wt := setupRepoWithWorktree(t)
	journalPath := filepath.Join(t.TempDir(), "active.json")

	if _, err := Activate("s", []RepoActivation{{Repo: "a", Primary: r, Branch: "feat"}}, journalPath, ActivateOptions{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	oldTip, err := git.RevParse(r, "HEAD")
	if err != nil {
		t.Fatalf("RevParse old tip: %v", err)
	}

	// Advance feat so Refresh would want to move the primary.
	if err := os.WriteFile(filepath.Join(wt, "advance.txt"), []byte("advance\n"), 0o644); err != nil {
		t.Fatalf("write advance.txt: %v", err)
	}
	if _, err := git.Run(wt, "add", "advance.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(wt, "commit", "-q", "-m", "advance feat"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	// Make the primary dirty with an untracked file.
	if err := os.WriteFile(filepath.Join(r, "dirty.txt"), []byte("wip\n"), 0o644); err != nil {
		t.Fatalf("write dirty.txt: %v", err)
	}

	_, err = Refresh(journalPath)
	if err == nil {
		t.Fatal("Refresh: expected error for dirty primary, got nil")
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Errorf("Refresh error: want mention of 'dirty', got %v", err)
	}

	// Zero state change: primary still at the old tip.
	head, _ := git.RevParse(r, "HEAD")
	if head != oldTip {
		t.Errorf("primary HEAD advanced despite refusal: want %q, got %q", oldTip, head)
	}
	loaded, err := Load(journalPath)
	if err != nil || loaded == nil {
		t.Fatalf("Load: %v (loaded=%v)", err, loaded)
	}
	if loaded.Repos[0].TargetSHA != oldTip {
		t.Errorf("journal TargetSHA changed despite refusal: want %q, got %q", oldTip, loaded.Repos[0].TargetSHA)
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
