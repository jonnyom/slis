package forge

import (
	"os"
	"path/filepath"
	"testing"

	gitutil "github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

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
