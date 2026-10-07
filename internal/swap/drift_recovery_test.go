package swap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/git"
)

func TestArchiveDriftedActivationPreservesExternalChangesAndRecovery(t *testing.T) {
	for _, switchBranch := range []bool{false, true} {
		name := "edited live checkout"
		if switchBranch {
			name = "switched checkout"
		}
		t.Run(name, func(t *testing.T) {
			primary, worktree, journalPath := activateLiveRepo(t)
			before, err := os.ReadFile(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			if switchBranch {
				if _, err := git.Run(primary, "switch", "main"); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(primary, "user.txt"), []byte("keep my work\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			primaryBefore, err := captureWorkingSnapshot(primary)
			if err != nil {
				t.Fatal(err)
			}
			sourceBefore, err := captureWorkingSnapshot(worktree)
			if err != nil {
				t.Fatal(err)
			}
			recoveryPath, err := ArchiveDriftedActivation(journalPath)
			if err != nil || recoveryPath == "" {
				t.Fatalf("archive = %q, %v", recoveryPath, err)
			}
			archived, err := os.ReadFile(recoveryPath)
			if err != nil || string(archived) != string(before) {
				t.Fatalf("recovery record changed: %v", err)
			}
			if active, err := Load(journalPath); err != nil || active != nil {
				t.Fatalf("activation remains: %#v, %v", active, err)
			}
			primaryAfter, err := captureWorkingSnapshot(primary)
			if err != nil || primaryAfter.Fingerprint != primaryBefore.Fingerprint || primaryAfter.Branch != primaryBefore.Branch {
				t.Fatalf("primary changed: %v", err)
			}
			sourceAfter, err := captureWorkingSnapshot(worktree)
			if err != nil || sourceAfter.Fingerprint != sourceBefore.Fingerprint {
				t.Fatalf("source changed: %v", err)
			}
		})
	}
}

func TestArchiveDriftedActivationLeavesValidActivation(t *testing.T) {
	_, _, journalPath := activateLiveRepo(t)
	recoveryPath, err := ArchiveDriftedActivation(journalPath)
	if err != nil || recoveryPath != "" {
		t.Fatalf("archive = %q, %v", recoveryPath, err)
	}
	if active, err := Load(journalPath); err != nil || active == nil {
		t.Fatalf("valid activation removed: %#v, %v", active, err)
	}
}

func TestArchiveDriftedActivationRetainsPinnedStash(t *testing.T) {
	primary, worktree := setupRepoWithWorktree(t)
	journalPath := filepath.Join(t.TempDir(), "active.json")
	if err := os.WriteFile(filepath.Join(primary, "original.txt"), []byte("before activation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	journal, err := Activate("s", []RepoActivation{{Repo: "web", Primary: primary, Branch: "feat", Worktree: worktree}}, journalPath, ActivateOptions{Stash: true})
	if err != nil {
		t.Fatal(err)
	}
	stashRef := journal.Repos[0].StashRef
	if stashRef == "" {
		t.Fatal("activation did not preserve original work")
	}
	if err := os.WriteFile(filepath.Join(primary, "external.txt"), []byte("new work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recoveryPath, err := ArchiveDriftedActivation(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := Load(recoveryPath)
	if err != nil || recovery == nil || recovery.Repos[0].StashRef != stashRef {
		t.Fatalf("stash reference lost: %#v, %v", recovery, err)
	}
	contents, err := git.Run(primary, "show", stashRef+"^3:original.txt")
	if err != nil || contents != "before activation" {
		t.Fatalf("stashed work changed: %q, %v", contents, err)
	}
}

func TestActivateAfterDriftKeepsRecoveredBranch(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)
	if _, err := git.Run(primary, "switch", "main"); err != nil {
		t.Fatal(err)
	}
	if _, err := ArchiveDriftedActivation(journalPath); err != nil {
		t.Fatal(err)
	}
	priorTip, err := git.RevParse(primary, LiveBranchName("s"))
	if err != nil {
		t.Fatal(err)
	}
	journal, err := Activate("s", []RepoActivation{{Repo: "web", Primary: primary, Branch: "feat", Worktree: worktree}}, journalPath, ActivateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if journal.Repos[0].TempBranch == LiveBranchName("s") {
		t.Fatal("reused recovered branch")
	}
	if err := Deactivate(journalPath, false); err != nil {
		t.Fatal(err)
	}
	if tip, err := git.RevParse(primary, LiveBranchName("s")); err != nil || tip != priorTip {
		t.Fatalf("recovered branch changed: %q, %v", tip, err)
	}
}
