package radar_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/radar"
	"github.com/jonnyom/slis/testutil"
)

func commit(t *testing.T, dir, msg string) {
	t.Helper()
	_, _ = git.Run(dir, "config", "user.email", "t@t")
	_, _ = git.Run(dir, "config", "user.name", "t")
	if _, err := git.Run(dir, "commit", "-m", msg); err != nil {
		t.Fatalf("commit %q: %v", msg, err)
	}
}

func writeAdd(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := git.Run(dir, "add", name); err != nil {
		t.Fatal(err)
	}
}

// TestRadar_Integration_DetectsSharedFile builds two real worktrees of one repo
// that both edit the same file, and asserts the radar flags the overlap (and not
// a file only one slice touches). Uses git only — gt being absent makes
// ParentBases fall back to trunk, which is the correct base here.
func TestRadar_Integration_DetectsSharedFile(t *testing.T) {
	repo := testutil.NewRepo(t)
	writeAdd(t, repo, "shared.txt", "base\n")
	commit(t, repo, "base")

	wtA := filepath.Join(t.TempDir(), "a")
	wtB := filepath.Join(t.TempDir(), "b")
	testutil.AddWorktree(t, repo, "feat-a", wtA)
	testutil.AddWorktree(t, repo, "feat-b", wtB)

	writeAdd(t, wtA, "shared.txt", "base\nfrom-a\n")
	writeAdd(t, wtA, "only-a.txt", "a\n")
	commit(t, wtA, "a changes")

	writeAdd(t, wtB, "shared.txt", "base\nfrom-b\n")
	commit(t, wtB, "b changes")

	mk := func(branch, wt string) model.Slice {
		return model.Slice{Name: branch, Members: map[string]model.SliceMember{
			"demo": {Repo: "demo", Branch: branch, WorktreePath: wt},
		}}
	}
	slices := []model.Slice{mk("feat-a", wtA), mk("feat-b", wtB)}

	idx := radar.Build(radar.CollectStats(slices))

	found := false
	for _, o := range idx.Overlaps {
		if o.Repo == "demo" && o.Path == "shared.txt" {
			found = true
		}
		if o.Path == "only-a.txt" {
			t.Errorf("only-a.txt is changed by one slice; should not be an overlap")
		}
	}
	if !found {
		t.Fatalf("expected shared.txt overlap, got %+v", idx.Overlaps)
	}
	if !idx.HasConflict("feat-a") || !idx.HasConflict("feat-b") {
		t.Fatal("both slices should report a conflict")
	}
}
