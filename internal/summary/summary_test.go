package summary_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/summary"
	"github.com/jonnyom/slis/testutil"
)

// addCommit creates a file and commits it with the given message in dir.
func addCommit(t *testing.T, dir, msg string) {
	t.Helper()
	env := []string{
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	f := filepath.Join(dir, msg+".txt")
	if err := os.WriteFile(f, []byte(msg), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	run("add", f)
	run("commit", "-q", "-m", msg)
}

// TestCommitSummary verifies that commits on the feature branch (not on base) are returned.
func TestCommitSummary(t *testing.T) {
	repo := testutil.NewRepo(t) // creates main with one empty commit

	// Create a worktree on branch "feat".
	wtPath := filepath.Join(t.TempDir(), "wt-feat")
	testutil.AddWorktree(t, repo, "feat", wtPath)

	// Add 2 commits on the feat worktree.
	addCommit(t, wtPath, "feat: A")
	addCommit(t, wtPath, "feat: B")

	sl := model.Slice{
		Name: "myslice",
		Members: map[string]model.SliceMember{
			"r": {Repo: "r", Branch: "feat", WorktreePath: wtPath},
		},
	}

	byRepo, err := summary.CommitSummary(sl, "main")
	if err != nil {
		t.Fatalf("CommitSummary: %v", err)
	}

	subjects, ok := byRepo["r"]
	if !ok {
		t.Fatal("expected key 'r' in result")
	}
	if len(subjects) != 2 {
		t.Fatalf("expected 2 subjects, got %d: %v", len(subjects), subjects)
	}
	// git log is reverse-chronological; both subjects must be present.
	found := map[string]bool{}
	for _, s := range subjects {
		found[s] = true
	}
	if !found["feat: A"] {
		t.Errorf("expected 'feat: A' in subjects; got %v", subjects)
	}
	if !found["feat: B"] {
		t.Errorf("expected 'feat: B' in subjects; got %v", subjects)
	}
}

// TestCommitSummaryMissingBase verifies that a repo with an absent base ref yields an empty list (no error).
func TestCommitSummaryMissingBase(t *testing.T) {
	repo := testutil.NewRepo(t)
	wtPath := filepath.Join(t.TempDir(), "wt-feat2")
	testutil.AddWorktree(t, repo, "feat2", wtPath)

	sl := model.Slice{
		Name: "myslice",
		Members: map[string]model.SliceMember{
			"r": {Repo: "r", Branch: "feat2", WorktreePath: wtPath},
		},
	}

	byRepo, err := summary.CommitSummary(sl, "nonexistent-base-ref-xyz")
	if err != nil {
		t.Fatalf("CommitSummary with missing base should not error; got: %v", err)
	}
	subjects := byRepo["r"]
	if subjects == nil {
		// nil is fine — means empty list
		return
	}
	if len(subjects) != 0 {
		t.Errorf("expected empty subjects for missing base; got %v", subjects)
	}
}

// TestRenderCommitSummary verifies that the markdown output contains the repo header and a subject.
func TestRenderCommitSummary(t *testing.T) {
	byRepo := map[string][]string{
		"myrepo": {"feat: do something", "fix: another thing"},
		"other":  {},
	}

	out := summary.RenderCommitSummary(byRepo)

	if !strings.Contains(out, "myrepo") {
		t.Errorf("output should contain repo name 'myrepo'; got:\n%s", out)
	}
	if !strings.Contains(out, "feat: do something") {
		t.Errorf("output should contain subject 'feat: do something'; got:\n%s", out)
	}
	if !strings.Contains(out, "other") {
		t.Errorf("output should contain repo name 'other'; got:\n%s", out)
	}
	// Empty repo should show "(no commits)".
	if !strings.Contains(out, "no commits") {
		t.Errorf("output should contain 'no commits' for empty repo; got:\n%s", out)
	}
}

// TestAISummaryUsesRunner verifies that the injected runner receives the diff and the instruction.
func TestAISummaryUsesRunner(t *testing.T) {
	var capturedInstruction, capturedStdin string
	runner := func(instruction, stdin string) (string, error) {
		capturedInstruction = instruction
		capturedStdin = stdin
		return "SUMMARY of " + stdin, nil
	}

	result, err := summary.AISummary("DIFFTEXT", runner)
	if err != nil {
		t.Fatalf("AISummary: %v", err)
	}

	// Result should contain DIFFTEXT (the runner echoes it).
	if !strings.Contains(result, "DIFFTEXT") {
		t.Errorf("result should contain 'DIFFTEXT'; got: %s", result)
	}
	// The instruction should have been passed to the runner and be non-empty.
	if capturedInstruction == "" {
		t.Error("instruction passed to runner should be non-empty")
	}
	if capturedStdin != "DIFFTEXT" {
		t.Errorf("stdin passed to runner = %q, want %q", capturedStdin, "DIFFTEXT")
	}
}

// TestRenderMarkdownNonEmpty verifies that RenderMarkdown returns non-empty output.
func TestRenderMarkdownNonEmpty(t *testing.T) {
	out := summary.RenderMarkdown("# Hi\n- x\n")
	if out == "" {
		t.Error("RenderMarkdown should return non-empty output")
	}
}

// TestDefaultClaudeRunnerMissingClaude verifies that DefaultClaudeRunner returns an error
// when claude binary is absent (guarded by LookPath check).
func TestDefaultClaudeRunnerMissingClaude(t *testing.T) {
	if _, err := exec.LookPath("claude"); err == nil {
		t.Skip("claude is present on PATH; skipping absence test")
	}
	_, err := summary.DefaultClaudeRunner("instruction", "content")
	if err == nil {
		t.Error("DefaultClaudeRunner should return error when claude not on PATH")
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should mention 'claude'; got: %v", err)
	}
}
