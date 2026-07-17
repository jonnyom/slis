package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/jonnyom/slis/testutil"
)

// swapDoctorFixture creates a single-repo workspace with a "feat" worktree
// carrying one commit (so feat's tip differs from main), activates the slice so
// the primary is on its slis/live temp branch at feat's tip, and returns the
// workspace, journal path, and primary dir.
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

	findings := swapFindings(ws, nil, journalPath)
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

	findings := swapFindings(ws, nil, journalPath)
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

func TestSwapFindingsStaleJournalNoFixWhenStillSwappedIn(t *testing.T) {
	ws, journalPath, primary := swapDoctorFixture(t)

	// User committed on the temp branch — HEAD moved off the slice tip but the
	// primary is still ON its slis/live branch (still swapped in). There must be
	// no stale-journal finding at all, so nothing fixable is offered.
	if err := os.WriteFile(filepath.Join(primary, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write x.txt: %v", err)
	}
	if _, err := git.Run(primary, "add", "x.txt"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if _, err := git.Run(primary, "commit", "-q", "-m", "commit on temp branch"); err != nil {
		t.Fatalf("git commit: %v", err)
	}

	findings := swapFindings(ws, nil, journalPath)
	warns, _, fixable := countSwapIssues(findings)
	if warns != 0 {
		t.Errorf("primary still on its temp branch — no stale-journal warning expected, got: %+v", findings)
	}
	if fixable != 0 {
		t.Errorf("no fix should be offered while a primary is still swapped in, got %d fixable: %+v", fixable, findings)
	}
}

// TestSwapFindingsStaleJournalWithStashIsReportOnly verifies S2: a stale journal
// that still pins an auto-stash is report-only — deleting it would orphan the
// user's stashed work (the journal is the only pointer), so no --fix is offered
// and the detail names the stash for recovery.
func TestSwapFindingsStaleJournalWithStashIsReportOnly(t *testing.T) {
	ws, journalPath, primary := swapDoctorFixture(t)

	// Put the primary back on a branch (stale journal) and pin an auto-stash ref
	// into the journal so deleting it would orphan the user's stashed work.
	if _, err := git.Run(primary, "switch", "main"); err != nil {
		t.Fatalf("switch main: %v", err)
	}
	j, err := swap.Load(journalPath)
	if err != nil || j == nil {
		t.Fatalf("Load: %v (j=%v)", err, j)
	}
	j.Repos[0].StashRef = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	j.Repos[0].StashMsg = "slis:auto:web:feat:123"
	if err := swap.Save(journalPath, j); err != nil {
		t.Fatalf("Save: %v", err)
	}

	findings := swapFindings(ws, nil, journalPath)
	warns, _, fixable := countSwapIssues(findings)
	if warns == 0 {
		t.Fatalf("stale journal with a pinned stash should still warn, got: %+v", findings)
	}
	if fixable != 0 {
		t.Errorf("no --fix must be offered while a pinned auto-stash exists (deletion would orphan it), got %d fixable: %+v", fixable, findings)
	}
	var mentionsStash bool
	for _, f := range findings {
		if strings.Contains(f.Detail, "stash") {
			mentionsStash = true
		}
	}
	if !mentionsStash {
		t.Errorf("report-only finding must name the stash and how to recover it, got: %+v", findings)
	}
}

// twoRepoWebActivated builds a two-repo workspace (web + api) and activates a
// slice covering ONLY web, so the journal records web while api is untouched —
// the seed for the partial-swap cross-check tests.
func twoRepoWebActivated(t *testing.T) (config.Workspace, string, string, string) {
	t.Helper()
	web := testutil.NewRepo(t)
	api := testutil.NewRepo(t)

	base := t.TempDir()
	featWT := filepath.Join(base, "feat")
	testutil.AddWorktree(t, web, "feat", featWT)
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
	if _, err := swap.Activate("myslice", []swap.RepoActivation{{Repo: "web", Primary: web, Branch: "feat"}}, journalPath, swap.ActivateOptions{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	ws := config.Workspace{Repos: map[string]config.Repo{
		"web": {Primary: web, DefaultBranch: "main"},
		"api": {Primary: api, DefaultBranch: "main"},
	}}
	return ws, journalPath, web, api
}

// TestSwapFindingsPartialJournalMissingMember: a slice member (api) is absent
// from the journal and its primary is still on its own branch — a crash during
// activate. doctor must warn (report-only, no --fix) and never call it healthy.
func TestSwapFindingsPartialJournalMissingMember(t *testing.T) {
	ws, journalPath, _, _ := twoRepoWebActivated(t)

	dtos := []SliceDTO{{Name: "myslice", Members: []MemberDTO{
		{Repo: "web", Branch: "feat"},
		{Repo: "api", Branch: "feat"},
	}}}

	findings := swapFindings(ws, dtos, journalPath)
	warns, _, fixable := countSwapIssues(findings)
	if warns == 0 {
		t.Fatalf("partial swap (api never swapped) should warn, got: %+v", findings)
	}
	if fixable != 0 {
		t.Errorf("partial swap must be report-only (no --fix) while a journal exists, got %d fixable: %+v", fixable, findings)
	}
	var found bool
	for _, f := range findings {
		if strings.Contains(f.Title, "partial swap") && strings.Contains(f.Title, "api") {
			found = true
		}
		if f.Level == lvlOK {
			t.Errorf("partial swap must not report the slice as healthy, got: %+v", f)
		}
	}
	if !found {
		t.Errorf("expected a 'partial swap: api' finding, got: %+v", findings)
	}
}

// TestSwapFindingsUnjournaledSwappedRepo: api's primary sits on the slice's
// slis/live branch with no journal entry (crash between switch and journal
// write). doctor must warn and offer no --fix while a journal exists.
func TestSwapFindingsUnjournaledSwappedRepo(t *testing.T) {
	ws, journalPath, _, api := twoRepoWebActivated(t)

	if _, err := git.Run(api, "switch", "-c", swap.LiveBranchName("myslice")); err != nil {
		t.Fatalf("create live branch on api: %v", err)
	}

	// Only web is a recorded member here, isolating the un-journaled check.
	dtos := []SliceDTO{{Name: "myslice", Members: []MemberDTO{{Repo: "web", Branch: "feat"}}}}

	findings := swapFindings(ws, dtos, journalPath)
	warns, _, fixable := countSwapIssues(findings)
	if warns == 0 {
		t.Fatalf("un-journaled swapped repo should warn, got: %+v", findings)
	}
	if fixable != 0 {
		t.Errorf("no --fix must be offered for an un-journaled swap while a journal exists, got %d fixable: %+v", fixable, findings)
	}
	var found bool
	for _, f := range findings {
		if strings.Contains(f.Title, "un-journaled") && strings.Contains(f.Title, "api") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an 'un-journaled: api' finding, got: %+v", findings)
	}
}

// TestSwapFindingsFullSwapNoFalsePositive: both repos swapped in and both in the
// journal — the cross-check must add no warnings (no false positives).
func TestSwapFindingsFullSwapNoFalsePositive(t *testing.T) {
	web := testutil.NewRepo(t)
	api := testutil.NewRepo(t)
	base := t.TempDir()

	setup := func(primary, name string) {
		wt := filepath.Join(base, name)
		testutil.AddWorktree(t, primary, "feat", wt)
		if err := os.WriteFile(filepath.Join(wt, "f.txt"), []byte("feat\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := git.Run(wt, "add", "f.txt"); err != nil {
			t.Fatalf("add: %v", err)
		}
		if _, err := git.Run(wt, "commit", "-q", "-m", "feat"); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
	setup(web, "web")
	setup(api, "api")

	journalPath := filepath.Join(t.TempDir(), "active.json")
	if _, err := swap.Activate("myslice", []swap.RepoActivation{
		{Repo: "web", Primary: web, Branch: "feat"},
		{Repo: "api", Primary: api, Branch: "feat"},
	}, journalPath, swap.ActivateOptions{}); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	ws := config.Workspace{Repos: map[string]config.Repo{
		"web": {Primary: web, DefaultBranch: "main"},
		"api": {Primary: api, DefaultBranch: "main"},
	}}
	dtos := []SliceDTO{{Name: "myslice", Members: []MemberDTO{
		{Repo: "web", Branch: "feat"},
		{Repo: "api", Branch: "feat"},
	}}}

	findings := swapFindings(ws, dtos, journalPath)
	warns, fails, _ := countSwapIssues(findings)
	if warns != 0 || fails != 0 {
		t.Errorf("full swap should have no warns/fails, got %d warn / %d fail: %+v", warns, fails, findings)
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
	findings := swapFindings(ws, nil, journalPath)
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

	findings := swapFindings(ws, nil, missingJournal)
	warns, _, _ := countSwapIssues(findings)
	if warns == 0 {
		t.Errorf("orphaned detached primary with no journal should warn, got: %+v", findings)
	}
}

// TestSwapFindingsOrphanedLiveBranch verifies a primary stuck on a slis/live
// temp branch with no journal warns, and that when the branch's commits are
// fully contained in the slice's branch, --fix switches back to trunk and
// deletes the temp branch (losing nothing).
func TestSwapFindingsOrphanedLiveBranch(t *testing.T) {
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

	// Simulate an orphaned swap: primary on slis/live/myslice at feat's tip, but
	// no journal. The temp branch's commits are fully contained in feat.
	featTip, err := git.RevParse(primary, "feat")
	if err != nil {
		t.Fatalf("rev-parse feat: %v", err)
	}
	if _, err := git.Run(primary, "switch", "-c", "slis/live/myslice", featTip); err != nil {
		t.Fatalf("create live branch: %v", err)
	}

	ws := config.Workspace{Repos: map[string]config.Repo{"web": {Primary: primary, DefaultBranch: "main"}}}
	dtos := []SliceDTO{{Name: "myslice", Members: []MemberDTO{{Repo: "web", Branch: "feat", WorktreePath: featWT}}}}
	missingJournal := filepath.Join(t.TempDir(), "none.json")

	findings := swapFindings(ws, dtos, missingJournal)
	warns, _, fixable := countSwapIssues(findings)
	if warns == 0 {
		t.Fatalf("orphaned slis/live branch should warn, got: %+v", findings)
	}
	if fixable == 0 {
		t.Fatalf("contained orphaned slis/live branch should offer a --fix, got: %+v", findings)
	}
	var autoMarker bool
	for _, f := range findings {
		if strings.Contains(f.Detail, "auto-fixable with --fix") {
			autoMarker = true
		}
	}
	if !autoMarker {
		t.Errorf("contained orphan finding should be marked auto-fixable, got: %+v", findings)
	}

	// Run the fix — primary back on main, temp branch gone.
	for _, f := range findings {
		if f.fix != nil {
			if _, err := f.fix(); err != nil {
				t.Fatalf("fix: %v", err)
			}
		}
	}
	if cur, _ := git.CurrentBranch(primary); cur != "main" {
		t.Errorf("after fix: want primary on main, got %q", cur)
	}
	if git.RefExists(primary, "refs/heads/slis/live/myslice") {
		t.Error("after fix: orphaned temp branch was not deleted")
	}
}

// TestSwapFindingsOrphanedLiveBranchNotContainedIsReportOnly is the safety gate on
// doctor's branch -D: an orphaned slis/live branch carrying commits NOT contained
// in the slice branch must be report-only — no --fix is offered, so the branch and
// its unique commits are never deleted.
func TestSwapFindingsOrphanedLiveBranchNotContainedIsReportOnly(t *testing.T) {
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

	featTip, err := git.RevParse(primary, "feat")
	if err != nil {
		t.Fatalf("rev-parse feat: %v", err)
	}
	// Orphaned live branch that carries a commit NOT on feat — so it is not
	// contained and deleting it would lose that commit.
	if _, err := git.Run(primary, "switch", "-c", "slis/live/myslice", featTip); err != nil {
		t.Fatalf("create live branch: %v", err)
	}
	if err := os.WriteFile(filepath.Join(primary, "extra.txt"), []byte("extra\n"), 0o644); err != nil {
		t.Fatalf("write extra: %v", err)
	}
	if _, err := git.Run(primary, "add", "extra.txt"); err != nil {
		t.Fatalf("add extra: %v", err)
	}
	if _, err := git.Run(primary, "commit", "-q", "-m", "extra commit only on live branch"); err != nil {
		t.Fatalf("commit extra: %v", err)
	}

	ws := config.Workspace{Repos: map[string]config.Repo{"web": {Primary: primary, DefaultBranch: "main"}}}
	dtos := []SliceDTO{{Name: "myslice", Members: []MemberDTO{{Repo: "web", Branch: "feat", WorktreePath: featWT}}}}
	missingJournal := filepath.Join(t.TempDir(), "none.json")

	findings := swapFindings(ws, dtos, missingJournal)
	warns, _, fixable := countSwapIssues(findings)
	if warns == 0 {
		t.Fatalf("orphaned non-contained slis/live branch should warn, got: %+v", findings)
	}
	if fixable != 0 {
		t.Errorf("non-contained orphaned slis/live branch must be report-only (no --fix), got %d fixable: %+v", fixable, findings)
	}
	var manualMarker bool
	for _, f := range findings {
		if strings.Contains(f.Detail, "needs manual attention: has commits not on the slice branch") {
			manualMarker = true
		}
	}
	if !manualMarker {
		t.Errorf("non-contained orphan finding should be marked needs-manual-attention, got: %+v", findings)
	}
	// The branch — and its unique commit — must still be present (nothing deleted).
	if !git.RefExists(primary, "refs/heads/slis/live/myslice") {
		t.Error("non-contained orphaned temp branch must not be deleted")
	}
}
