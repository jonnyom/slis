package tmuxctl_test

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

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

func TestAttachWindowArgv(t *testing.T) {
	name, args := tmuxctl.AttachWindowArgv("payroll-fix", "review-codex", false)
	if name != "tmux" || strings.Join(args, " ") != "attach -t slis/payroll-fix:review-codex" {
		t.Fatalf("outside tmux = %q %#v", name, args)
	}

	name, args = tmuxctl.AttachWindowArgv("payroll-fix", "review-codex", true)
	if name != "tmux" || strings.Join(args, " ") != "switch-client -t slis/payroll-fix:review-codex" {
		t.Fatalf("inside tmux = %q %#v", name, args)
	}
}

func TestStartOrRespawnWindowPreservesCompletedReview(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}
	slice := fmt.Sprintf("slistest-review-%d", time.Now().UnixNano())
	_ = tmuxctl.KillSession(slice)
	t.Cleanup(func() { _ = tmuxctl.KillSession(slice) })
	member := model.SliceMember{Repo: "api", WorktreePath: t.TempDir()}
	if err := tmuxctl.EnsureSession(slice, []model.SliceMember{member}, tmuxctl.SessionOpts{}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := tmuxctl.StartOrRespawnWindow(slice, "review-codex", member.WorktreePath, "printf 'first\\n'"); err != nil {
		t.Fatal(err)
	}
	target := tmuxctl.WindowTarget(slice, "review-codex")
	waitForOutput := func(want string) {
		t.Helper()
		for range 100 {
			output, err := exec.Command("tmux", "capture-pane", "-p", "-S", "-", "-t", target).Output()
			if err == nil && strings.Contains(string(output), want) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("window did not contain %q", want)
	}
	waitForOutput("first")
	if err := tmuxctl.StartOrRespawnWindow(slice, "review-codex", member.WorktreePath, "printf 'second\\n'"); err != nil {
		t.Fatal(err)
	}
	waitForOutput("second")
	if running, err := tmuxctl.WindowRunning(slice, "review-codex"); err != nil || running {
		t.Fatalf("completed window running = %v, err = %v", running, err)
	}
	if err := tmuxctl.StartOrRespawnWindow(slice, "review-codex", member.WorktreePath, "sleep 30"); err != nil {
		t.Fatal(err)
	}
	if running, err := tmuxctl.WindowRunning(slice, "review-codex"); err != nil || !running {
		t.Fatalf("active window running = %v, err = %v", running, err)
	}
	if err := tmuxctl.StartOrRespawnWindow(slice, "review-codex", member.WorktreePath, "printf blocked"); !errors.Is(err, tmuxctl.ErrWindowBusy) {
		t.Fatalf("busy window error = %v", err)
	}
}

func TestSendPromptOnceSkipsACompletedDelivery(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}
	slice := fmt.Sprintf("slistest-delivery-%d", time.Now().UnixNano())
	_ = tmuxctl.KillSession(slice)
	t.Cleanup(func() { _ = tmuxctl.KillSession(slice) })
	member := model.SliceMember{Repo: "api", WorktreePath: t.TempDir()}
	if err := tmuxctl.EnsureSession(slice, []model.SliceMember{member}, tmuxctl.SessionOpts{}); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("tmux", "send-keys", "-t", tmuxctl.SessionName(slice), "cat", "Enter").Run(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	prompt := "slis-once-delivery-marker"
	if err := tmuxctl.SendPromptOnce(slice, prompt, "run-1:request-1"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	first, err := tmuxctl.CaptureActivePane(slice)
	if err != nil {
		t.Fatal(err)
	}
	firstCount := strings.Count(first, prompt)
	if firstCount == 0 {
		t.Fatalf("first delivery missing from pane: %q", first)
	}

	if err := tmuxctl.SendPromptOnce(slice, prompt, "run-1:request-1"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	second, err := tmuxctl.CaptureActivePane(slice)
	if err != nil {
		t.Fatal(err)
	}
	if secondCount := strings.Count(second, prompt); secondCount != firstCount {
		t.Fatalf("delivery count = %d, want %d; pane: %q", secondCount, firstCount, second)
	}
}

func TestSendPromptOnceSerializesConcurrentDelivery(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not found on PATH")
	}
	slice := fmt.Sprintf("slistest-concurrent-delivery-%d", time.Now().UnixNano())
	_ = tmuxctl.KillSession(slice)
	t.Cleanup(func() { _ = tmuxctl.KillSession(slice) })
	member := model.SliceMember{Repo: "api", WorktreePath: t.TempDir()}
	if err := tmuxctl.EnsureSession(slice, []model.SliceMember{member}, tmuxctl.SessionOpts{}); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("tmux", "send-keys", "-t", tmuxctl.SessionName(slice), "cat", "Enter").Run(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	const senders = 8
	prompt := "slis-concurrent-delivery-marker"
	start := make(chan struct{})
	errs := make(chan error, senders)
	for range senders {
		go func() {
			<-start
			errs <- tmuxctl.SendPromptOnce(slice, prompt, "run-1:request-1")
		}()
	}
	close(start)
	for range senders {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	pane, err := tmuxctl.CaptureActivePane(slice)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(pane, prompt); count != 1 {
		t.Fatalf("delivery count = %d, want 1; pane: %q", count, pane)
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
