package restack_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/restack"
	"github.com/jonnyom/slis/testutil"
)

func slice(repo, wt string) model.Slice {
	return model.Slice{
		Name:    "s",
		Members: map[string]model.SliceMember{repo: {Repo: repo, Branch: "feat", WorktreePath: wt}},
	}
}

// TestRunRestacksClean verifies a clean worktree is restacked via the runner.
func TestRunRestacksClean(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat", wt)

	called := ""
	rep := restack.Run(slice("r", wt), func(dir string) (string, error) {
		called = dir
		return "Restacked feat.", nil
	})
	if called != wt {
		t.Errorf("runner called with %q, want worktree %q", called, wt)
	}
	if len(rep.Repos) != 1 || !rep.Repos[0].Restacked {
		t.Errorf("expected Restacked=true, got %+v", rep.Repos)
	}
}

// TestRunSkipsDirty verifies a dirty worktree is skipped and the runner is not
// called for it.
func TestRunSkipsDirty(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat", wt)
	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ran := false
	rep := restack.Run(slice("r", wt), func(string) (string, error) { ran = true; return "", nil })
	if ran {
		t.Error("runner should not run for a dirty worktree")
	}
	if !rep.Repos[0].SkippedDirty {
		t.Errorf("expected SkippedDirty=true, got %+v", rep.Repos[0])
	}
}

// TestRunDetectsConflict verifies a conflict-shaped failure is classified as a
// conflict (not a generic error).
func TestRunDetectsConflict(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat", wt)

	rep := restack.Run(slice("r", wt), func(string) (string, error) {
		return "CONFLICT (content): merge conflict in foo. Resolve, then `gt continue`.", errors.New("exit 1")
	})
	r := rep.Repos[0]
	if !r.Conflict || r.Restacked || r.Err != "" {
		t.Errorf("expected Conflict=true only, got %+v", r)
	}
}
