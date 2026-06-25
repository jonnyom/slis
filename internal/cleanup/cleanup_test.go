package cleanup_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/cleanup"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/jonnyom/slis/testutil"
)

func sliceFor(repoName, repo, branch, wt string) (config.Workspace, model.Slice) {
	ws := config.Workspace{Repos: map[string]config.Repo{repoName: {Primary: repo}}}
	sl := model.Slice{
		Name:    "s",
		Members: map[string]model.SliceMember{repoName: {Repo: repoName, Branch: branch, WorktreePath: wt}},
	}
	return ws, sl
}

// TestRemoveDeletesWorktreeAndMergedBranch verifies the happy path: a clean
// worktree is removed and its (merged) branch deleted.
func TestRemoveDeletesWorktreeAndMergedBranch(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat", wt) // branch tips at main HEAD → merged

	ws, sl := sliceFor("r", repo, "feat", wt)
	rep, err := cleanup.Remove(ws, sl, cleanup.Options{DeleteBranches: true})
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("worktree dir should be gone, stat err = %v", err)
	}
	if git.RefExists(repo, "feat") {
		t.Errorf("branch 'feat' should be deleted")
	}
	if len(rep.Repos) != 1 || !rep.Repos[0].WorktreeRemoved || !rep.Repos[0].BranchDeleted {
		t.Errorf("unexpected report: %+v", rep.Repos)
	}
}

// TestRemoveRefusesDirtyWithoutForce verifies a dirty worktree is preserved
// unless Force is set.
func TestRemoveRefusesDirtyWithoutForce(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat2", wt)
	if err := os.WriteFile(filepath.Join(wt, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, sl := sliceFor("r", repo, "feat2", wt)

	rep, _ := cleanup.Remove(ws, sl, cleanup.Options{DeleteBranches: true})
	if rep.Repos[0].WorktreeRemoved {
		t.Error("dirty worktree should NOT be removed without force")
	}
	if rep.Repos[0].Err == "" {
		t.Error("expected an error for the dirty worktree")
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("worktree should still exist, got %v", err)
	}

	rep2, _ := cleanup.Remove(ws, sl, cleanup.Options{DeleteBranches: true, Force: true})
	if !rep2.Repos[0].WorktreeRemoved {
		t.Error("force should remove the dirty worktree")
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Error("worktree dir should be gone after forced removal")
	}
}

// TestRemoveRefusesLiveSlice verifies the race-free guard: when the journal
// records the slice as live, Remove errors and touches nothing — even if a UI
// thought the slice was idle.
func TestRemoveRefusesLiveSlice(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat-live", wt)

	journal := filepath.Join(t.TempDir(), "active.json")
	if err := swap.Save(journal, &swap.Journal{Slice: "s"}); err != nil {
		t.Fatalf("seed journal: %v", err)
	}

	ws, sl := sliceFor("r", repo, "feat-live", wt) // slice name "s" matches the journal
	rep, err := cleanup.Remove(ws, sl, cleanup.Options{DeleteBranches: true, ActiveJournal: journal})
	if err == nil {
		t.Fatal("expected Remove to refuse a live slice")
	}
	if len(rep.Repos) != 0 {
		t.Errorf("a refused removal must touch nothing, got %+v", rep.Repos)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("worktree must be untouched, stat err = %v", err)
	}
	if !git.RefExists(repo, "feat-live") {
		t.Error("branch must be untouched")
	}
}

// TestPlanRemoveNoSideEffects verifies PlanRemove describes intent without
// touching the filesystem.
func TestPlanRemoveNoSideEffects(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	testutil.AddWorktree(t, repo, "feat3", wt)

	_, sl := sliceFor("r", repo, "feat3", wt)
	p := cleanup.PlanRemove(sl, cleanup.Options{DeleteBranches: true})

	if len(p.Repos) != 1 || !p.Repos[0].BranchDeleted {
		t.Errorf("plan should mark branch deletion: %+v", p)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Errorf("PlanRemove must not remove the worktree, stat err = %v", err)
	}
}
