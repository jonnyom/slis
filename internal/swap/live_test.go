package swap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/git"
)

func TestRunLiveSyncMirrorsUntilCancelled(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- RunLiveSync(ctx, journalPath, 10*time.Millisecond)
	}()

	if err := os.WriteFile(filepath.Join(worktree, "watched.txt"), []byte("live\n"), 0o644); err != nil {
		t.Fatalf("write watched file: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(filepath.Join(primary, "watched.txt"))
		if err == nil && string(data) == "live\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("live mirror did not update: %q, %v", data, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RunLiveSync: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunLiveSync did not stop")
	}
}

func TestMirrorMismatchDiagnosticsIdentifySourceChangeDuringCopy(t *testing.T) {
	captured := workingSnapshot{Head: "captured-head", Branch: "feature", Patch: []byte("captured"), Changed: []string{"tracked.ts"}, Fingerprint: "captured"}
	actual := workingSnapshot{Head: "captured-head", Branch: "slis/live/feature", Patch: []byte("actual"), Changed: []string{"tracked.ts"}, Fingerprint: "actual"}
	latest := workingSnapshot{Head: "latest-head", Branch: "feature", Patch: []byte("latest"), Changed: []string{"other.ts"}, Untracked: []string{"new.ts"}, Fingerprint: "latest"}

	failure := newMirrorMismatchError(captured, actual, latest)
	if !strings.Contains(failure.Error(), "source worktree changed during mirror") {
		t.Fatalf("error = %q", failure.Error())
	}
	diagnostics := failure.DiagnosticFields()
	if diagnostics["source_changed_during_mirror"] != true {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	latestFields := diagnostics["latest_source"].(map[string]any)
	if latestFields["fingerprint"] != "latest" || latestFields["patch_sha256"] == "" {
		t.Fatalf("latest source diagnostics = %#v", latestFields)
	}
}

func TestMirrorMismatchDiagnosticsIdentifyStableSourceDivergence(t *testing.T) {
	captured := workingSnapshot{Head: "head", Branch: "feature", Fingerprint: "source"}
	actual := workingSnapshot{Head: "head", Branch: "slis/live/feature", Fingerprint: "primary"}

	failure := newMirrorMismatchError(captured, actual, captured)
	if !strings.Contains(failure.Error(), "primary differs from stable source worktree") {
		t.Fatalf("error = %q", failure.Error())
	}
	if failure.DiagnosticFields()["source_changed_during_mirror"] != false {
		t.Fatalf("diagnostics = %#v", failure.DiagnosticFields())
	}
}

func TestSyncWorkingTreeStopsWhenCancelled(t *testing.T) {
	_, _, journalPath := activateLiveRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := syncWorkingTree(ctx, journalPath)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("syncWorkingTree error = %v", err)
	}
}

func activateLiveRepo(t *testing.T) (primary, worktree, journalPath string) {
	t.Helper()
	primary, worktree = setupRepoWithWorktree(t)
	journalPath = filepath.Join(t.TempDir(), "active.json")
	_, err := Activate("s", []RepoActivation{{
		Repo:     "web",
		Primary:  primary,
		Branch:   "feat",
		Worktree: worktree,
	}}, journalPath, ActivateOptions{})
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	return primary, worktree, journalPath
}

func TestSyncWorkingTreeMirrorsGitVisibleChanges(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)

	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "new.txt"), []byte("untracked\n"), 0o755); err != nil {
		t.Fatalf("write untracked file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "ignored.txt"), []byte("cache\n"), 0o644); err != nil {
		t.Fatalf("write ignored file: %v", err)
	}

	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("SyncWorkingTree: %v", err)
	}

	tracked, err := os.ReadFile(filepath.Join(primary, "f.txt"))
	if err != nil || string(tracked) != "edited\n" {
		t.Fatalf("tracked mirror = %q, %v", tracked, err)
	}
	untracked, err := os.ReadFile(filepath.Join(primary, "new.txt"))
	if err != nil || string(untracked) != "untracked\n" {
		t.Fatalf("untracked mirror = %q, %v", untracked, err)
	}
	if info, err := os.Stat(filepath.Join(primary, "new.txt")); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("untracked mode = %v, %v", info, err)
	}
	if _, err := os.Stat(filepath.Join(primary, "ignored.txt")); !os.IsNotExist(err) {
		t.Fatalf("ignored file copied: %v", err)
	}
}

func TestSyncWorkingTreeFollowsPreviouslyMirroredUntrackedFileIntoCommit(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)
	path := "newly_tracked.py"
	if err := os.WriteFile(filepath.Join(worktree, path), []byte("value = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("mirror untracked file: %v", err)
	}
	if _, err := git.Run(worktree, "add", path); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(worktree, "commit", "-q", "-m", "track mirrored file"); err != nil {
		t.Fatal(err)
	}

	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("mirror committed file: %v", err)
	}
	primaryStatus, err := git.Run(primary, "status", "--short", "--", path)
	if err != nil {
		t.Fatal(err)
	}
	if primaryStatus != "" {
		t.Fatalf("primary status = %q", primaryStatus)
	}
	journal, err := Load(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal == nil || journal.Repos[0].Mirror == nil || len(journal.Repos[0].Mirror.Untracked) != 0 {
		t.Fatalf("mirror state = %#v", journal)
	}
}

func TestSyncWorkingTreeIgnoresWatchmanCookies(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)
	worktreeCookie := filepath.Join(worktree, ".watchman-cookie-source-100-1")
	primaryCookie := filepath.Join(primary, ".watchman-cookie-primary-200-2")
	if err := os.WriteFile(worktreeCookie, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(primaryCookie, []byte("primary"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("SyncWorkingTree: %v", err)
	}
	tracked, err := os.ReadFile(filepath.Join(primary, "f.txt"))
	if err != nil || string(tracked) != "edited\n" {
		t.Fatalf("tracked mirror = %q, %v", tracked, err)
	}
	if _, err := os.Stat(filepath.Join(primary, filepath.Base(worktreeCookie))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source Watchman cookie was mirrored: %v", err)
	}
	primaryCookieContents, err := os.ReadFile(primaryCookie)
	if err != nil || string(primaryCookieContents) != "primary" {
		t.Fatalf("primary Watchman cookie = %q, %v", primaryCookieContents, err)
	}
	journal, err := Load(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if journal == nil || journal.Repos[0].Mirror == nil || len(journal.Repos[0].Mirror.Untracked) != 0 {
		t.Fatalf("mirror state = %#v", journal)
	}
}

func TestSyncWorkingTreeMirrorsDeletes(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)

	if err := os.Remove(filepath.Join(worktree, "f.txt")); err != nil {
		t.Fatalf("remove tracked file: %v", err)
	}
	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("SyncWorkingTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(primary, "f.txt")); !os.IsNotExist(err) {
		t.Fatalf("deleted file remains: %v", err)
	}
}

func TestSyncWorkingTreeFollowsRewrittenTip(t *testing.T) {
	primary, worktree, journalPath := activateLiveRepo(t)
	oldTip, err := git.RevParse(worktree, "HEAD")
	if err != nil {
		t.Fatalf("old tip: %v", err)
	}

	if err := os.WriteFile(filepath.Join(worktree, "f.txt"), []byte("amended\n"), 0o644); err != nil {
		t.Fatalf("write amended file: %v", err)
	}
	if _, err := git.Run(worktree, "add", "f.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(worktree, "commit", "-q", "--amend", "-m", "rewritten"); err != nil {
		t.Fatalf("git amend: %v", err)
	}
	newTip, err := git.RevParse(worktree, "HEAD")
	if err != nil || newTip == oldTip {
		t.Fatalf("new tip = %q, %v", newTip, err)
	}

	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("SyncWorkingTree: %v", err)
	}
	primaryTip, err := git.RevParse(primary, "HEAD")
	if err != nil || primaryTip != newTip {
		t.Fatalf("primary tip = %q, want %q: %v", primaryTip, newTip, err)
	}
}

func TestSyncWorkingTreeFollowsBranchRenameAtSameTip(t *testing.T) {
	_, worktree, journalPath := activateLiveRepo(t)
	if _, err := git.Run(worktree, "branch", "-m", "renamed"); err != nil {
		t.Fatalf("rename branch: %v", err)
	}
	if _, err := SyncWorkingTree(journalPath); err != nil {
		t.Fatalf("SyncWorkingTree: %v", err)
	}
	journal, err := Load(journalPath)
	if err != nil || journal == nil {
		t.Fatalf("Load: %#v, %v", journal, err)
	}
	if journal.Repos[0].Branch != "renamed" {
		t.Fatalf("journal branch = %q", journal.Repos[0].Branch)
	}
}
