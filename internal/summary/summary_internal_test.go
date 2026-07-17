package summary

import (
	"os/exec"
	"strings"
	"testing"
)

// TestCodexExecArgs verifies the `codex exec` argv: the diff is referenced by
// file path (never piped on stdin) and the prompt carries the instruction.
func TestCodexExecArgs(t *testing.T) {
	args := codexExecArgs("Summarise this.", "/tmp/x.diff")
	if len(args) != 2 {
		t.Fatalf("codexExecArgs len = %d, want 2 (%v)", len(args), args)
	}
	if args[0] != "exec" {
		t.Errorf("codexExecArgs[0] = %q, want exec", args[0])
	}
	if !strings.Contains(args[1], "Summarise this.") {
		t.Errorf("prompt missing instruction: %q", args[1])
	}
	if !strings.Contains(args[1], "/tmp/x.diff") {
		t.Errorf("prompt missing diff file path: %q", args[1])
	}
}

// TestRunnerForHarnessSelection verifies harness → runner selection by
// exercising the missing-binary error path for each (skipping when the binary
// happens to be present on PATH).
func TestRunnerForHarnessSelection(t *testing.T) {
	if _, err := exec.LookPath("codex"); err != nil {
		_, err := RunnerForHarness("codex")("instruction", "diff")
		if err == nil || !strings.Contains(err.Error(), "codex") {
			t.Errorf("codex runner should surface a codex-specific error, got: %v", err)
		}
	}
	if _, err := exec.LookPath("claude"); err != nil {
		_, err := RunnerForHarness("")("instruction", "diff")
		if err == nil || !strings.Contains(err.Error(), "claude") {
			t.Errorf("default runner should surface a claude-specific error, got: %v", err)
		}
	}
}
