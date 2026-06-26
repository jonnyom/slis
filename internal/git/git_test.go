package git_test

import (
	"context"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

func TestArgvBuilderInjectionProof(t *testing.T) {
	userInput := "; rm -rf /"
	argv := git.NewCmd("switch").Arg("--detach").Arg(userInput).Argv()

	want := []string{"switch", "--detach", "; rm -rf /"}
	if len(argv) != len(want) {
		t.Fatalf("Argv() len = %d, want %d; got %v", len(argv), len(want), argv)
	}
	for i, v := range want {
		if argv[i] != v {
			t.Errorf("argv[%d] = %q, want %q", i, argv[i], v)
		}
	}
}

func TestArgvBuilderArgIf(t *testing.T) {
	argv := git.NewCmd("checkout").ArgIf(true, "--track").ArgIf(false, "--no-track").Argv()
	want := []string{"checkout", "--track"}
	if len(argv) != len(want) {
		t.Fatalf("Argv() len = %d, want %d; got %v", len(argv), len(want), argv)
	}
	if argv[0] != want[0] || argv[1] != want[1] {
		t.Errorf("got %v, want %v", argv, want)
	}
}

func TestArgvReturnsACopy(t *testing.T) {
	c := git.NewCmd("log")
	a1 := c.Argv()
	a1[0] = "mutated"
	a2 := c.Argv()
	if a2[0] == "mutated" {
		t.Error("Argv() should return a copy, not a slice backed by the same array")
	}
}

func TestRunInDir(t *testing.T) {
	repo := testutil.NewRepo(t)
	got, err := git.Run(repo, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got != "main" {
		t.Errorf("HEAD branch = %q, want %q", got, "main")
	}
}

// TestRunHasDefaultTimeout guards the timeout plumbing: a hung git subprocess
// must not block a caller (or a TUI background slot) forever.
func TestRunHasDefaultTimeout(t *testing.T) {
	if git.DefaultTimeout <= 0 {
		t.Fatalf("DefaultTimeout = %v, want > 0", git.DefaultTimeout)
	}
}

// TestRunCtxCancels confirms RunCtx honours a cancelled context promptly rather
// than waiting on the subprocess — the property the gate relies on.
func TestRunCtxCancels(t *testing.T) {
	repo := testutil.NewRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	done := make(chan error, 1)
	go func() {
		_, err := git.RunCtx(ctx, repo, "rev-parse", "HEAD")
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunCtx with a cancelled context should return an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunCtx did not return promptly on a cancelled context")
	}
}
