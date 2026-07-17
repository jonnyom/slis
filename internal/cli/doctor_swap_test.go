package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/jonnyom/slis/testutil"
)

// swapDoctorFixture creates a single-repo workspace with a "feat" worktree
// carrying one commit (so feat's tip differs from main), activates the slice so
// the primary is detached at feat's tip, and returns the workspace, journal
// path, and primary dir.
func swapDoctorFixture(t *testing.T) (config.Workspace, string, string) {
	t.Helper()
	primary := testutil.NewRepo(t)
	base := t.TempDir()
	featWT := filepath.Join(base, "feat")
	testutil.AddWorktree(t, primary, "feat", featWT)

	if err := os.WriteFile(filepath.Join(featWT, "f.txt"), []byte("feat\n"), 0o644); err != nil {
		t.Fatalf("write f.txt: %v", err)
	}
	if _, err := git.Run(featWT, "add", "f.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(featWT, "commit", "-q", "-m", "feat work"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	journalPath := filepath.Join(t.TempDir(), "active.json")
	if _, err := swap.Activate("myslice", []swap.RepoActivation{{Repo: "web", Primary: primary, Branch: "feat"}}, journalPath, swap.ActivateOptions{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	ws := config.Workspace{
		Repos: map[string]config.Repo{"web": {Primary: primary, DefaultBranch: "main"}},
	}
	return ws, journalPath, primary
}

func countSwapIssues(findings []doctorFinding) (warns, fails, fixable int) {
	for _, f := range findings {
		switch f.Level {
		case lvlWarn:
			warns++
		case lvlFail:
			fails++
		}
		if f.fix != nil {
			fixable++
		}
	}
	return
}

func TestSwapFindingsHealthy(t *testing.T) {
	ws, journalPath, _ := swapDoctorFixture(t)

	findings := swapFindings(ws, journalPath)
	warns, fails, _ := countSwapIssues(findings)
	if warns != 0 || fails != 0 {
		t.Errorf("healthy swap should have no warns/fails, got %d warn / %d fail: %+v", warns, fails, findings)
	}
}

func TestSwapFindingsStaleJournalFixDeletes(t *testing.T) {
	ws, journalPath, primary := swapDoctorFixture(t)

	// Simulate the swap being undone outside slis: put the primary back on a
	// branch, leaving a stale journal behind.
	if _, err := git.Run(primary, "switch", "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}

	findings := swapFindings(ws, journalPath)
	warns, _, fixable := countSwapIssues(findings)
	if warns == 0 {
		t.Fatalf("stale journal should warn, got: %+v", findings)
	}
	if fixable == 0 {
		t.Fatal("stale journal with primary on a branch should offer a --fix")
	}

	// Run the fix — it must delete the journal (every primary is on a branch).
	var ran bool
	for _, f := range findings {
		if f.fix != nil {
			if _, err := f.fix(); err != nil {
				t.Fatalf("fix: %v", err)
			}
			ran = true
		}
	}
	if !ran {
		t.Fatal("no fix ran")
	}
	loaded, err := swap.Load(journalPath)
	if err != nil {
		t.Fatalf("Load after fix: %v", err)
	}
	if loaded != nil {
		t.Error("stale journal was not deleted by --fix")
	}
}

func TestSwapFindingsStaleJournalNoFixWhenStillDetached(t *testing.T) {
	ws, journalPath, primary := swapDoctorFixture(t)

	// User committed on the detached primary — HEAD moved off the slice tip but
	// the primary is still detached (still effectively swapped). The stale-journal
	// finding must NOT offer the delete fix (the safety gate: only delete when
	// every primary is provably on a branch).
	if err := os.WriteFile(filepath.Join(primary, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write x.txt: %v", err)
	}
	if _, err := git.Run(primary, "add", "x.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(primary, "commit", "-q", "-m", "commit on detached"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	findings := swapFindings(ws, journalPath)
	_, _, fixable := countSwapIssues(findings)
	if fixable != 0 {
		t.Errorf("safety gate: no fix should be offered while a primary is still detached, got %d fixable: %+v", fixable, findings)
	}
}

func TestSwapFindingsPriorBranchGone(t *testing.T) {
	primary := testutil.NewRepo(t)
	base := t.TempDir()
	featWT := filepath.Join(base, "feat")
	testutil.AddWorktree(t, primary, "feat", featWT)
	if err := os.WriteFile(filepath.Join(featWT, "f.txt"), []byte("feat\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := git.Run(featWT, "add", "f.txt"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := git.Run(featWT, "commit", "-q", "-m", "feat"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// Activate from a deletable "prior" branch, then delete it.
	if _, err := git.Run(primary, "switch", "-c", "prior"); err != nil {
		t.Fatalf("switch -c prior: %v", err)
	}
	journalPath := filepath.Join(t.TempDir(), "active.json")
	if _, err := swap.Activate("myslice", []swap.RepoActivation{{Repo: "web", Primary: primary, Branch: "feat"}}, journalPath, swap.ActivateOptions{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if _, err := git.Run(primary, "branch", "-D", "prior"); err != nil {
		t.Fatalf("delete prior: %v", err)
	}

	ws := config.Workspace{Repos: map[string]config.Repo{"web": {Primary: primary, DefaultBranch: "main"}}}
	findings := swapFindings(ws, journalPath)
	_, fails, _ := countSwapIssues(findings)
	if fails == 0 {
		t.Errorf("deleted prior branch should produce a fail finding, got: %+v", findings)
	}
}

func TestSwapFindingsOrphanedDetach(t *testing.T) {
	primary := testutil.NewRepo(t)
	// Detach the primary with no journal present.
	head, err := git.RevParse(primary, "HEAD")
	if err != nil {
		t.Fatalf("RevParse: %v", err)
	}
	if _, err := git.Run(primary, "switch", "--detach", head); err != nil {
		t.Fatalf("switch --detach: %v", err)
	}

	ws := config.Workspace{Repos: map[string]config.Repo{"web": {Primary: primary, DefaultBranch: "main"}}}
	missingJournal := filepath.Join(t.TempDir(), "none.json")

	findings := swapFindings(ws, missingJournal)
	warns, _, _ := countSwapIssues(findings)
	if warns == 0 {
		t.Errorf("orphaned detached primary with no journal should warn, got: %+v", findings)
	}
}
