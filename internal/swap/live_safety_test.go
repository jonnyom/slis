package swap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/git"
)

func TestSyncWorkingTreeRefusesUnknownPrimaryChangesBeforeUpdatingAnyRepo(t *testing.T) {
	primaryA, worktreeA := setupRepoWithWorktree(t)
	primaryB, worktreeB := setupRepoWithWorktree(t)
	journalPath := filepath.Join(t.TempDir(), "active.json")
	_, err := Activate("s", []RepoActivation{
		{Repo: "a", Primary: primaryA, Branch: "feat", Worktree: worktreeA},
		{Repo: "b", Primary: primaryB, Branch: "feat", Worktree: worktreeB},
	}, journalPath, ActivateOptions{})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if err := os.WriteFile(filepath.Join(primaryA, "unknown.txt"), []byte("human\n"), 0o644); err != nil {
		t.Fatalf("write unknown primary file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeB, "source.txt"), []byte("agent\n"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}

	_, err = SyncWorkingTree(journalPath)
	if err == nil || !strings.Contains(err.Error(), "unknown primary changes") {
		t.Fatalf("SyncWorkingTree error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(primaryB, "source.txt")); !os.IsNotExist(err) {
		t.Fatalf("second repo changed after preflight failure: %v", err)
	}
}

func TestSyncWorkingTreeRefusesToOverwriteIgnoredPrimaryFile(t *testing.T) {
	primary, worktree := setupRepoWithWorktree(t)
	if err := os.WriteFile(filepath.Join(worktree, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatalf("write initial gitignore: %v", err)
	}
	if _, err := git.Run(worktree, "add", ".gitignore"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(worktree, "commit", "-q", "-m", "ignore local file"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	journalPath := filepath.Join(t.TempDir(), "active.json")
	_, err := Activate("s", []RepoActivation{{
		Repo: "web", Primary: primary, Branch: "feat", Worktree: worktree,
	}}, journalPath, ActivateOptions{})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := os.WriteFile(filepath.Join(primary, "ignored.txt"), []byte("human\n"), 0o644); err != nil {
		t.Fatalf("write ignored primary file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".gitignore"), nil, 0o644); err != nil {
		t.Fatalf("write worktree gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "ignored.txt"), []byte("agent\n"), 0o644); err != nil {
		t.Fatalf("write worktree file: %v", err)
	}

	_, err = SyncWorkingTree(journalPath)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("SyncWorkingTree error = %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(primary, "ignored.txt"))
	if readErr != nil || string(data) != "human\n" {
		t.Fatalf("ignored primary file = %q, %v", data, readErr)
	}
}

func TestSyncWorkingTreeRefusesTrackedFileOverIgnoredPrimaryFile(t *testing.T) {
	primary, worktree := setupRepoWithWorktree(t)
	if err := os.WriteFile(filepath.Join(worktree, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatalf("write initial gitignore: %v", err)
	}
	if _, err := git.Run(worktree, "add", ".gitignore"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(worktree, "commit", "-q", "-m", "ignore local file"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	journalPath := filepath.Join(t.TempDir(), "active.json")
	_, err := Activate("s", []RepoActivation{{
		Repo: "web", Primary: primary, Branch: "feat", Worktree: worktree,
	}}, journalPath, ActivateOptions{})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := os.WriteFile(filepath.Join(primary, "ignored.txt"), []byte("human\n"), 0o644); err != nil {
		t.Fatalf("write ignored primary file: %v", err)
	}
	if err := os.Remove(filepath.Join(worktree, ".gitignore")); err != nil {
		t.Fatalf("remove worktree gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "ignored.txt"), []byte("agent\n"), 0o644); err != nil {
		t.Fatalf("write tracked worktree file: %v", err)
	}
	if _, err := git.Run(worktree, "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(worktree, "commit", "-q", "-m", "track ignored file"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	_, err = SyncWorkingTree(journalPath)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("SyncWorkingTree error = %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(primary, "ignored.txt"))
	if readErr != nil || string(data) != "human\n" {
		t.Fatalf("ignored primary file = %q, %v", data, readErr)
	}
}

func TestSyncWorkingTreeRefusesStagedFileOverIgnoredPrimaryFile(t *testing.T) {
	primary, worktree := setupRepoWithWorktree(t)
	if err := os.WriteFile(filepath.Join(worktree, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatalf("write initial gitignore: %v", err)
	}
	if _, err := git.Run(worktree, "add", ".gitignore"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(worktree, "commit", "-q", "-m", "ignore local file"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	journalPath := filepath.Join(t.TempDir(), "active.json")
	_, err := Activate("s", []RepoActivation{{
		Repo: "web", Primary: primary, Branch: "feat", Worktree: worktree,
	}}, journalPath, ActivateOptions{})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := os.WriteFile(filepath.Join(primary, "ignored.txt"), []byte("human\n"), 0o644); err != nil {
		t.Fatalf("write ignored primary file: %v", err)
	}
	if err := os.Remove(filepath.Join(worktree, ".gitignore")); err != nil {
		t.Fatalf("remove worktree gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "ignored.txt"), []byte("agent\n"), 0o644); err != nil {
		t.Fatalf("write staged worktree file: %v", err)
	}
	if _, err := git.Run(worktree, "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}

	_, err = SyncWorkingTree(journalPath)
	if err == nil || !strings.Contains(err.Error(), "target already exists") {
		t.Fatalf("SyncWorkingTree error = %v", err)
	}
	data, readErr := os.ReadFile(filepath.Join(primary, "ignored.txt"))
	if readErr != nil || string(data) != "human\n" {
		t.Fatalf("ignored primary file = %q, %v", data, readErr)
	}
}

func TestSyncWorkingTreeRollsBackWhenJournalSaveFails(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)
	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte("updated\n"), 0o644); err != nil {
		t.Fatalf("write worktree file: %v", err)
	}
	journalDirectory := filepath.Dir(journalPath)
	if err := os.Chmod(journalDirectory, 0o555); err != nil {
		t.Fatalf("make journal directory read-only: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(journalDirectory, 0o755); err != nil {
			t.Errorf("restore journal directory permissions: %v", err)
		}
	})

	_, err := SyncWorkingTree(journalPath)
	if err == nil {
		t.Fatal("SyncWorkingTree succeeded with read-only journal directory")
	}
	data, readErr := os.ReadFile(filepath.Join(primary, "f.txt"))
	if readErr != nil || string(data) != "feat work\n" {
		t.Fatalf("primary after failed save = %q, %v", data, readErr)
	}
}

func TestDeactivateRemovesMirrorAndRestoresStashedPrimaryWork(t *testing.T) {
	primary, worktree := setupRepoWithWorktree(t)
	journalPath := filepath.Join(t.TempDir(), "active.json")
	if err := os.WriteFile(filepath.Join(primary, "original.txt"), []byte("before activation\n"), 0o644); err != nil {
		t.Fatalf("write original primary work: %v", err)
	}
	_, err := Activate("s", []RepoActivation{{
		Repo:     "web",
		Primary:  primary,
		Branch:   "feat",
		Worktree: worktree,
	}}, journalPath, ActivateOptions{Stash: true})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("mirrored\n"), 0o644); err != nil {
		t.Fatalf("write mirrored source: %v", err)
	}
	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("SyncWorkingTree: %v", err)
	}

	if err := Deactivate(journalPath, false); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if branch, err := git.CurrentBranch(primary); err != nil || branch != "main" {
		t.Fatalf("primary branch = %q, %v", branch, err)
	}
	if data, err := os.ReadFile(filepath.Join(primary, "original.txt")); err != nil || string(data) != "before activation\n" {
		t.Fatalf("restored primary work = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(primary, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("mirrored file remains after deactivate: %v", err)
	}
}

func TestDeactivateRefusesUnknownPrimaryChanges(t *testing.T) {
	primary, _, journalPath := activateLiveRepo(t)
	if err := os.WriteFile(filepath.Join(primary, "unknown.txt"), []byte("human\n"), 0o644); err != nil {
		t.Fatalf("write unknown primary file: %v", err)
	}

	err := Deactivate(journalPath, false)
	if err == nil || !strings.Contains(err.Error(), "unknown primary changes") {
		t.Fatalf("Deactivate error = %v", err)
	}
	if branch, branchErr := git.CurrentBranch(primary); branchErr != nil || branch != LiveBranchName("s") {
		t.Fatalf("primary branch = %q, %v", branch, branchErr)
	}
	if journal, loadErr := Load(journalPath); loadErr != nil || journal == nil {
		t.Fatalf("journal = %#v, %v", journal, loadErr)
	}
}

func TestDeactivateAcceptsCleanPrimaryAlreadyOnPriorBranch(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)
	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte("mirrored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatal(err)
	}
	journal, err := Load(journalPath)
	if err != nil || journal == nil {
		t.Fatalf("journal = %#v, %v", journal, err)
	}
	state := journal.Repos[0]
	if _, err := git.Run(primary, "reset", "--hard", state.TargetSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(primary, "switch", state.PriorBranch); err != nil {
		t.Fatal(err)
	}

	if err := Deactivate(journalPath, false); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if active, err := Load(journalPath); err != nil || active != nil {
		t.Fatalf("active journal = %#v, %v", active, err)
	}
	if git.RefExists(primary, "refs/heads/"+state.TempBranch) {
		t.Fatalf("temp branch %q still exists", state.TempBranch)
	}
}

func TestDeactivateRefusesUntrackedWorkOnPriorBranch(t *testing.T) {
	primary, _, journalPath := activateLiveRepo(t)
	journal, err := Load(journalPath)
	if err != nil || journal == nil {
		t.Fatalf("journal = %#v, %v", journal, err)
	}
	state := journal.Repos[0]
	if _, err := git.Run(primary, "switch", state.PriorBranch); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(primary, "manual.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = Deactivate(journalPath, false)
	if err == nil || !strings.Contains(err.Error(), "unknown primary changes") {
		t.Fatalf("Deactivate error = %v", err)
	}
	contents, readErr := os.ReadFile(filepath.Join(primary, "manual.txt"))
	if readErr != nil || string(contents) != "keep\n" {
		t.Fatalf("manual work = %q, %v", contents, readErr)
	}
	if active, loadErr := Load(journalPath); loadErr != nil || active == nil {
		t.Fatalf("active journal = %#v, %v", active, loadErr)
	}
}

func TestDeactivateRefusesKnownMirrorFileIgnoredByPriorBranch(t *testing.T) {
	primary, worktree := setupRepoWithWorktree(t)
	if err := os.WriteFile(filepath.Join(primary, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(primary, "add", ".gitignore"); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(primary, "commit", "-q", "-m", "ignore local file"); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(t.TempDir(), "active.json")
	if _, err := Activate("s", []RepoActivation{{Repo: "web", Primary: primary, Branch: "feat", Worktree: worktree}}, journalPath, ActivateOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "ignored.txt"), []byte("mirrored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatal(err)
	}
	journal, err := Load(journalPath)
	if err != nil || journal == nil {
		t.Fatalf("journal = %#v, %v", journal, err)
	}
	state := journal.Repos[0]
	if _, err := git.Run(primary, "reset", "--hard", state.TargetSHA); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(primary, "switch", state.PriorBranch); err != nil {
		t.Fatal(err)
	}

	err = Deactivate(journalPath, false)
	if err == nil || !strings.Contains(err.Error(), "unknown primary changes") {
		t.Fatalf("Deactivate error = %v", err)
	}
	contents, readErr := os.ReadFile(filepath.Join(primary, "ignored.txt"))
	if readErr != nil || string(contents) != "mirrored\n" {
		t.Fatalf("ignored mirror file = %q, %v", contents, readErr)
	}
	if active, loadErr := Load(journalPath); loadErr != nil || active == nil {
		t.Fatalf("active journal = %#v, %v", active, loadErr)
	}
}
