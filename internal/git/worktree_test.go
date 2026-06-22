package git_test

import (
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

// buildFixture assembles a git worktree list --porcelain -z byte slice from
// raw NUL-delimited records, mirroring what git actually emits.
func buildFixture() []byte {
	// Each attribute line is NUL-terminated; each record is terminated by an
	// extra NUL (so records are separated by \x00\x00).
	main := "worktree /tmp/repo\x00HEAD aabbccddeeff00112233445566778899aabbccdd\x00branch refs/heads/main\x00\x00"
	detached := "worktree /tmp/repo-wt\x00HEAD 1122334455667788990011223344556677889900\x00detached\x00\x00"
	return []byte(main + detached)
}

func TestParseWorktreeList(t *testing.T) {
	data := buildFixture()
	wts := git.ParseWorktreeList(data)

	if len(wts) != 2 {
		t.Fatalf("ParseWorktreeList: got %d entries, want 2", len(wts))
	}

	// First entry: main worktree on branch main.
	first := wts[0]
	if first.Branch != "main" {
		t.Errorf("wts[0].Branch = %q, want %q", first.Branch, "main")
	}
	if first.Detached {
		t.Errorf("wts[0].Detached = true, want false")
	}
	if first.Path != "/tmp/repo" {
		t.Errorf("wts[0].Path = %q, want %q", first.Path, "/tmp/repo")
	}

	// Second entry: detached HEAD, no branch.
	second := wts[1]
	if second.Branch != "" {
		t.Errorf("wts[1].Branch = %q, want %q", second.Branch, "")
	}
	if !second.Detached {
		t.Errorf("wts[1].Detached = false, want true")
	}
}

func TestListWorktreesLive(t *testing.T) {
	repo := testutil.NewRepo(t)
	wtPath := filepath.Join(t.TempDir(), "wt-feature")
	testutil.AddWorktree(t, repo, "feature-x", wtPath)

	wts, err := git.ListWorktrees(repo)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) < 2 {
		t.Fatalf("ListWorktrees: got %d worktrees, want >= 2", len(wts))
	}

	found := false
	for _, wt := range wts {
		if wt.Branch == "feature-x" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListWorktrees: no worktree with Branch == %q found in %v", "feature-x", wts)
	}
}
