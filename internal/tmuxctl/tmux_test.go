package tmuxctl_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// TestSessionNameSanitises ensures dots and colons are replaced and the prefix is correct.
func TestSessionNameSanitises(t *testing.T) {
	name := tmuxctl.SessionName("a.b:c")
	if strings.ContainsAny(name, ".:") {
		t.Errorf("SessionName %q still contains '.' or ':'", name)
	}
	if !strings.HasPrefix(name, "slis/") {
		t.Errorf("SessionName %q does not start with 'slis/'", name)
	}
}

// TestAttachArgv checks that AttachArgv returns the right command/args for inside-tmux
// and outside-tmux cases without spawning anything.
func TestAttachArgv(t *testing.T) {
	slice := "myslice"
	want := tmuxctl.SessionName(slice)

	// inside tmux → switch-client
	name, args := tmuxctl.AttachArgv(slice, true)
	if name != "tmux" {
		t.Errorf("inside-tmux: expected binary 'tmux', got %q", name)
	}
	if len(args) < 2 || args[0] != "switch-client" {
		t.Errorf("inside-tmux: expected switch-client subcommand, got %v", args)
	}
	if args[len(args)-1] != want {
		t.Errorf("inside-tmux: expected target %q, got %q", want, args[len(args)-1])
	}

	// outside tmux → attach
	name, args = tmuxctl.AttachArgv(slice, false)
	if name != "tmux" {
		t.Errorf("outside-tmux: expected binary 'tmux', got %q", name)
	}
	if len(args) < 2 || args[0] != "attach" {
		t.Errorf("outside-tmux: expected attach subcommand, got %v", args)
	}
	if args[len(args)-1] != want {
		t.Errorf("outside-tmux: expected target %q, got %q", want, args[len(args)-1])
	}
}

// TestEnsureSessionLifecycle is a live test that requires tmux.
// It exercises: EnsureSession (create), SessionExists, idempotent re-create,
// PanePIDs, and KillSession cleanup.
func TestEnsureSessionLifecycle(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}

	const slice = "slistest-lifecycle"

	// Pre-clean any leftover from a previous run.
	_ = tmuxctl.KillSession(slice)

	// Register cleanup so the session is always removed.
	t.Cleanup(func() {
		_ = tmuxctl.KillSession(slice)
	})

	// Build two members with real temporary directories as worktree paths.
	members := []model.SliceMember{
		{Repo: "alpha", Branch: "feat", WorktreePath: t.TempDir(), TipSHA: "aaa"},
		{Repo: "beta", Branch: "feat", WorktreePath: t.TempDir(), TipSHA: "bbb"},
	}

	// Session must not exist before we create it.
	if tmuxctl.SessionExists(slice) {
		t.Fatal("session should not exist before EnsureSession")
	}

	// Create the session.
	if err := tmuxctl.EnsureSession(slice, members); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	// Session should now exist.
	if !tmuxctl.SessionExists(slice) {
		t.Fatal("session should exist after EnsureSession")
	}

	// Calling EnsureSession again must be idempotent.
	if err := tmuxctl.EnsureSession(slice, members); err != nil {
		t.Fatalf("idempotent EnsureSession: %v", err)
	}

	// PanePIDs should return one PID per window (one per member).
	pids, err := tmuxctl.PanePIDs(slice)
	if err != nil {
		t.Fatalf("PanePIDs: %v", err)
	}
	if len(pids) < len(members) {
		t.Errorf("PanePIDs: got %d pids, want >= %d", len(pids), len(members))
	}
	for _, pid := range pids {
		if pid <= 0 {
			t.Errorf("PanePIDs: got non-positive pid %d", pid)
		}
	}

	// Kill the session.
	if err := tmuxctl.KillSession(slice); err != nil {
		t.Fatalf("KillSession: %v", err)
	}

	// Session must no longer exist.
	if tmuxctl.SessionExists(slice) {
		t.Fatal("session should not exist after KillSession")
	}
}
