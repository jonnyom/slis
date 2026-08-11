package git_test

import (
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

// BranchRenamedFrom is the proof slis uses to tell "the user renamed their branch"
// apart from "this worktree now holds unrelated work" — the difference between
// keeping a grouping override alive and silently adopting a stranger's branch.
func TestBranchRenamedFrom(t *testing.T) {
	repo := testutil.NewRepo(t)

	if _, err := git.Run(repo, "switch", "-c", "jonny/feature-one"); err != nil {
		t.Fatalf("create branch: %v", err)
	}
	if _, err := git.Run(repo, "branch", "-m", "jonny/feature-one", "jonny/feature-two"); err != nil {
		t.Fatalf("rename branch: %v", err)
	}

	if !git.BranchRenamedFrom(repo, "jonny/feature-two", "jonny/feature-one") {
		t.Fatal("an in-place rename must be provable from the new branch's reflog")
	}

	// A branch created independently — the shape a Claude Code worktree takeover
	// leaves behind — is not a rename of anything.
	if _, err := git.Run(repo, "switch", "-c", "claude/other-work"); err != nil {
		t.Fatalf("create unrelated branch: %v", err)
	}
	if git.BranchRenamedFrom(repo, "claude/other-work", "jonny/feature-one") {
		t.Fatal("an unrelated branch must not read as a rename")
	}
	if git.BranchRenamedFrom(repo, "claude/other-work", "jonny/feature-two") {
		t.Fatal("checking a different branch out is not a rename")
	}
}

func TestBranchRenamedFromDegradesQuietly(t *testing.T) {
	repo := testutil.NewRepo(t)

	if git.BranchRenamedFrom(repo, "does-not-exist", "also-missing") {
		t.Fatal("a missing ref must not report a rename")
	}
	if git.BranchRenamedFrom("", "a", "b") || git.BranchRenamedFrom(repo, "a", "a") {
		t.Fatal("empty or identical inputs must report false")
	}
}
