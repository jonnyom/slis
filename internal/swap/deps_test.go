package swap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

// commitFile writes path with content in repo, stages it, commits, and returns HEAD SHA.
func commitFile(t *testing.T, repo, path, content string) string {
	t.Helper()

	fullPath := filepath.Join(repo, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("commitFile: mkdir %s: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("commitFile: write %s: %v", fullPath, err)
	}

	if _, err := git.Run(repo, "add", path); err != nil {
		t.Fatalf("commitFile: git add %s: %v", path, err)
	}
	if _, err := git.Run(repo, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "-m", "add "+path); err != nil {
		t.Fatalf("commitFile: git commit: %v", err)
	}

	sha, err := git.RevParse(repo, "HEAD")
	if err != nil {
		t.Fatalf("commitFile: rev-parse: %v", err)
	}
	return sha
}

func TestLockfilesChanged_Modified(t *testing.T) {
	repo := testutil.NewRepo(t)

	from := commitFile(t, repo, "pnpm-lock.yaml", "lockfile-v1\n")
	to := commitFile(t, repo, "pnpm-lock.yaml", "lockfile-v2\n")

	changed, err := LockfilesChanged(repo, from, to, []string{"pnpm-lock.yaml"})
	if err != nil {
		t.Fatalf("LockfilesChanged: %v", err)
	}
	if !changed {
		t.Error("LockfilesChanged = false, want true (lockfile content changed)")
	}
}

func TestLockfilesChanged_Same(t *testing.T) {
	repo := testutil.NewRepo(t)

	from := commitFile(t, repo, "pnpm-lock.yaml", "lockfile-vsame\n")
	// Make an unrelated commit so toSHA != fromSHA while lockfile is unchanged.
	to := commitFile(t, repo, "other.txt", "unrelated\n")

	changed, err := LockfilesChanged(repo, from, to, []string{"pnpm-lock.yaml"})
	if err != nil {
		t.Fatalf("LockfilesChanged: %v", err)
	}
	if changed {
		t.Error("LockfilesChanged = true, want false (lockfile content did not change)")
	}
}

func TestLockfilesChanged_AbsentBoth(t *testing.T) {
	repo := testutil.NewRepo(t)

	from := commitFile(t, repo, "readme.txt", "hello\n")
	to := commitFile(t, repo, "readme.txt", "world\n")

	changed, err := LockfilesChanged(repo, from, to, []string{"pnpm-lock.yaml"})
	if err != nil {
		t.Fatalf("LockfilesChanged: %v", err)
	}
	if changed {
		t.Error("LockfilesChanged = true, want false (lockfile absent in both commits)")
	}
}

// TestLockfilesChangedPropagatesRealError verifies Fix E: a bogus from-SHA
// (one that does not name a real object in the repository) must cause
// LockfilesChanged to return a non-nil error rather than silently returning
// (false, nil) as though the file were merely absent.
func TestLockfilesChangedPropagatesRealError(t *testing.T) {
	repo := testutil.NewRepo(t)
	// Create a real commit so we have a valid toSHA.
	validSHA := commitFile(t, repo, "pnpm-lock.yaml", "v1\n")

	// 0000...0000 is a 40-char hex string that is valid SHA syntax but does
	// NOT name any object in the git object database.
	bogusSHA := "0000000000000000000000000000000000000000"

	_, err := LockfilesChanged(repo, bogusSHA, validSHA, []string{"pnpm-lock.yaml"})
	if err == nil {
		t.Error("LockfilesChanged: expected non-nil error for bogus from-SHA, got nil")
	}
}
