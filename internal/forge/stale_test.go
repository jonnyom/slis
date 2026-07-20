package forge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gitutil "github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

func TestParsePRListFindsMergedPRForDeletedRemoteBranch(t *testing.T) {
	raw := `[{"number":119,"state":"MERGED","headRefName":"jonny/pay-119","headRefOid":"abc123","url":"https://github.com/Noryai/nory/pull/119"}]`
	pr, err := parsePRList("jonny/pay-119", []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if pr == nil || pr.Number != 119 || pr.State != "MERGED" || pr.HeadSHA != "abc123" {
		t.Fatalf("parsePRList = %+v", pr)
	}
	if !strings.Contains(pr.URL, "/119") {
		t.Fatalf("URL = %q", pr.URL)
	}
}

func TestPRForBranchFallsBackToMergedHistoryAfterRemoteBranchDeletion(t *testing.T) {
	repo := testutil.NewRepo(t)
	worktree := filepath.Join(t.TempDir(), "pay-119")
	testutil.AddWorktree(t, repo, "jonny/pay-119", worktree)
	head, err := gitutil.RevParse(worktree, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	gh := filepath.Join(binDir, "gh")
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "pr" ] && [ "$2" = "view" ]; then
  echo "no pull requests found for branch" >&2
  exit 1
fi
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  echo '[{"number":119,"state":"MERGED","headRefName":"jonny/pay-119","headRefOid":"%s","url":"https://github.com/Noryai/nory/pull/119"}]'
  exit 0
fi
echo '[]'
`, head)
	if err := os.WriteFile(gh, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	pr, err := PRForBranch(worktree, "jonny/pay-119")
	if err != nil {
		t.Fatal(err)
	}
	if pr == nil || pr.State != "MERGED" || pr.Number != 119 {
		t.Fatalf("PRForBranch = %+v", pr)
	}
}

func TestHistoricalPRBecomesStaleWhenCheckoutHasNewWork(t *testing.T) {
	repo := testutil.NewRepo(t)
	head, err := gitutil.RevParse(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	pr := &PR{State: "MERGED", HeadSHA: head}
	if historicalPRIsStale(repo, "main", pr) {
		t.Fatal("clean checkout at merged PR head should still match")
	}
	if err := os.WriteFile(filepath.Join(repo, "new-work.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !historicalPRIsStale(repo, "main", pr) {
		t.Fatal("dirty checkout must not inherit historical merged PR")
	}
}

func TestHistoricalPRBecomesStaleWhenBranchTipMoves(t *testing.T) {
	repo := testutil.NewRepo(t)
	oldHead, err := gitutil.RevParse(repo, "main")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "next.txt"), []byte("next\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := gitutil.Run(repo, "add", "next.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := gitutil.Run(repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "next"); err != nil {
		t.Fatal(err)
	}
	if !historicalPRIsStale(repo, "main", &PR{State: "MERGED", HeadSHA: oldHead}) {
		t.Fatal("advanced branch must not inherit historical merged PR")
	}
	if historicalPRIsStale(repo, "main", &PR{State: "OPEN", HeadSHA: oldHead}) {
		t.Fatal("open PR remains associated by branch name")
	}
}
