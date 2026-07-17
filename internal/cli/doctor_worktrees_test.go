package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/testutil"
)

func TestSkippedWorktreeFindings_PerReasonAdvice(t *testing.T) {
	skipped := []SkippedWorktreeDTO{
		{Repo: "web", Path: "/a", Branch: "", Reason: discovery.ReasonDetached},
		{Repo: "api", Path: "/b", Branch: "x", Reason: discovery.ReasonPrunable},
		{Repo: "web", Path: "/c", Branch: "y", Reason: discovery.ReasonDetached},
	}
	findings := skippedWorktreeFindings(skipped)
	if len(findings) != 2 {
		t.Fatalf("expected one finding per reason (2), got %d: %+v", len(findings), findings)
	}
	var detached, prunable *doctorFinding
	for i := range findings {
		switch {
		case strings.Contains(findings[i].Title, discovery.ReasonDetached):
			detached = &findings[i]
		case strings.Contains(findings[i].Title, discovery.ReasonPrunable):
			prunable = &findings[i]
		}
	}
	if detached == nil || !strings.Contains(detached.Title, "2 hidden") {
		t.Fatalf("expected a detached finding counting 2, got %+v", findings)
	}
	if prunable == nil || !strings.Contains(prunable.Detail, "git worktree prune") {
		t.Fatalf("prunable finding must suggest `git worktree prune`, got %+v", prunable)
	}
	// doctor must never auto-prune: no fix closure on these.
	for _, f := range findings {
		if f.fix != nil {
			t.Errorf("skipped-worktree findings must not be auto-fixable: %+v", f)
		}
	}
}

// orphanWorktreeFindings must flag an empty litter dir and a rebound checkout
// under <root>/.slis/worktrees, while leaving a genuinely-tracked worktree alone.
func TestOrphanWorktreeFindings(t *testing.T) {
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	base := filepath.Join(root, ".slis", "worktrees")

	// A real, git-tracked worktree at the managed location — must NOT be flagged.
	trackedWT := filepath.Join(base, "real", "web")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", "-b", "jonny/real", trackedWT).CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}

	// An empty litter directory (repo dir exists but is not a git worktree).
	litter := filepath.Join(base, "litter", "web")
	if err := os.MkdirAll(litter, 0o755); err != nil {
		t.Fatal(err)
	}

	// A full checkout whose .git file points at an admin slot that is not this
	// path (rebound/orphaned) — a .git file, but git doesn't list it.
	ghost := filepath.Join(base, "ghost", "web")
	if err := os.MkdirAll(ghost, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ghost, ".git"),
		[]byte("gitdir: "+filepath.Join(repo, ".git", "worktrees", "someoneelse")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// An entirely empty slice dir (no repo subdirs).
	if err := os.MkdirAll(filepath.Join(base, "hollow"), 0o755); err != nil {
		t.Fatal(err)
	}

	ws := config.Workspace{
		Root:     root,
		Repos:    map[string]config.Repo{"web": {Primary: repo, DefaultBranch: "main"}},
		Grouping: config.Grouping{Strategy: "branch-name"},
	}

	findings := orphanWorktreeFindings(ws)

	joined := ""
	for _, f := range findings {
		joined += f.Title + "|" + f.Detail + "\n"
		if f.fix != nil {
			t.Errorf("orphan findings must be non-destructive (no fix): %+v", f)
		}
	}
	if strings.Contains(joined, trackedWT) {
		t.Errorf("tracked worktree must not be flagged as an orphan:\n%s", joined)
	}
	if !strings.Contains(joined, litter) {
		t.Errorf("empty litter dir %q must be flagged:\n%s", litter, joined)
	}
	if !strings.Contains(joined, ghost) || !strings.Contains(joined, "rebound") {
		t.Errorf("rebound checkout %q must be flagged:\n%s", ghost, joined)
	}
}
