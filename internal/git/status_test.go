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
