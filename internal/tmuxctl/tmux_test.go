package tmuxctl_test

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

func TestRelatedSessionNamesIncludesCanonicalShellAndLegacySessions(t *testing.T) {
	members := []model.SliceMember{{Repo: "nory", WorktreePath: "/worktrees/pay-119/nory"}}
	panes := []tmuxctl.SessionPane{
		{Session: "slis/old-pay-119", Path: "/worktrees/pay-119/nory", Command: "claude"},
		{Session: "slis/unrelated", Path: "/worktrees/elsewhere", Command: "claude"},
	}
	got := tmuxctl.RelatedSessionNames("pay-119", members, panes)
	want := []string{"slis-shell/pay-119", "slis/old-pay-119", "slis/pay-119"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("RelatedSessionNames = %v, want %v", got, want)
	}
}

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
	if err := tmuxctl.EnsureSession(slice, members, tmuxctl.SessionOpts{}); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	// Session should now exist.
	if !tmuxctl.SessionExists(slice) {
		t.Fatal("session should exist after EnsureSession")
	}

	if err := tmuxctl.StartWindow(slice, "review", members[0].WorktreePath, "printf review-ready; sleep 30"); err != nil {
		t.Fatalf("StartWindow: %v", err)
	}
	var reviewOutput string
	for range 20 {
		output, err := exec.Command("tmux", "capture-pane", "-p", "-t", tmuxctl.SessionName(slice)+":review").Output()
		if err == nil {
			reviewOutput = string(output)
			if strings.Contains(reviewOutput, "review-ready") {
				break
			}
		}
	}
	if !strings.Contains(reviewOutput, "review-ready") {
		t.Fatalf("review window output = %q", reviewOutput)
	}

	// Slis sessions enable mouse mode locally so wheel input forwarded by the
	// embedded terminal scrolls tmux history without changing global settings.
	mouse, err := exec.Command("tmux", "show-options", "-v", "-t", tmuxctl.SessionName(slice), "mouse").Output()
	if err != nil {
		t.Fatalf("read session mouse option: %v", err)
	}
	if strings.TrimSpace(string(mouse)) != "on" {
		t.Fatalf("session mouse option = %q, want on", strings.TrimSpace(string(mouse)))
	}

	// Calling EnsureSession again must be idempotent.
	if err := tmuxctl.EnsureSession(slice, members, tmuxctl.SessionOpts{}); err != nil {
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
