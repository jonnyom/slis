//go:build unix

package subproc_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/subproc"
)

// grandchildScript spawns a long-lived background process (the stand-in for the
// `git` children a real `gt` leaves behind), reports its pid, and then waits.
const grandchildScript = `sleep 120 &
echo $! > "$1"
wait
`

// alive reports whether pid is still a live process (signal 0 probe).
func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func waitGone(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !alive(pid) {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return !alive(pid)
}

func readPID(t *testing.T, path string, within time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			if pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil && pid > 0 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("grandchild never reported its pid (%s)", path)
	return 0
}

// TestConfigureKillsGrandchildrenOnCancel is the whole point of the package: a
// cancelled command must take its children with it. Without Configure, Go signals
// only the direct child and the grandchild survives as a runaway.
func TestConfigureKillsGrandchildrenOnCancel(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", grandchildScript, "sh", pidFile)
	subproc.Configure(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	grandchild := readPID(t, pidFile, 5*time.Second)
	if !alive(grandchild) {
		t.Fatalf("grandchild %d should be running before cancellation", grandchild)
	}

	cancel()
	_ = cmd.Wait()

	if !waitGone(grandchild, 3*time.Second) {
		t.Fatalf("grandchild %d survived cancellation", grandchild)
	}
}

// TestConfigureWaitReturnsWhilePipeHeld proves the second failure mode is closed:
// a killed command whose grandchild still holds the stdout pipe must not block
// Wait forever — WaitDelay bounds it.
func TestConfigureWaitReturnsWhilePipeHeld(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The grandchild inherits (and holds) stdout, so the pipe never sees EOF from
	// the direct child's exit alone.
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 120 & wait")
	subproc.Configure(cmd)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
	case <-time.After(subproc.WaitDelay + 3*time.Second):
		t.Fatal("Wait did not return after cancellation — a held pipe wedged the caller")
	}
}

// TestConfigureNilAndContextlessAreSafe keeps Configure usable at every call site
// without a guard.
func TestConfigureNilAndContextlessAreSafe(t *testing.T) {
	subproc.Configure(nil)

	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	subproc.Configure(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("contextless command failed: %v", err)
	}
}
