package proc_test

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/proc"
	goproc "github.com/shirou/gopsutil/v4/process"
)

// TestSliceProcsFindsBusyChild starts a CPU-burning shell loop as a child of
// the test process and verifies that SliceProcs walking from os.Getpid()
// discovers it.
func TestSliceProcsFindsBusyChild(t *testing.T) {
	t.Helper()

	cmd := exec.Command("sh", "-c", "while :; do :; done")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start burner: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	burnerPID := cmd.Process.Pid

	// Let the burner accrue some CPU time.
	time.Sleep(300 * time.Millisecond)

	procs, err := proc.SliceProcs([]int{os.Getpid()})
	if err != nil {
		t.Fatalf("SliceProcs: %v", err)
	}
	if len(procs) == 0 {
		t.Fatal("SliceProcs returned empty slice, expected at least the test process")
	}

	// Verify the burner PID is present (key behaviour).
	found := false
	for _, p := range procs {
		if p.PID == burnerPID {
			found = true
		}
		// Sanity: every entry must have a non-negative CPU value.
		if p.CPU < 0 {
			t.Errorf("PID %d has negative CPU percent %f", p.PID, p.CPU)
		}
	}
	if !found {
		t.Errorf("burner PID %d not found in SliceProcs result (got %d entries)", burnerPID, len(procs))
	}

	// Verify sorted by CPU descending (allow ties).
	for i := 1; i < len(procs); i++ {
		if procs[i].CPU > procs[i-1].CPU {
			t.Errorf("not sorted by CPU desc: index %d (%.2f) > index %d (%.2f)",
				i, procs[i].CPU, i-1, procs[i-1].CPU)
		}
	}
}

// TestKillSubtreeTerminates starts a parent shell with two child sleeps,
// calls KillSubtree on the parent, and asserts the parent is gone.
func TestKillSubtreeTerminates(t *testing.T) {
	t.Helper()

	// Start a parent sh that spawns two background sleeps.
	cmd := exec.Command("sh", "-c", "sleep 30 & sleep 30 & wait")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start subtree: %v", err)
	}

	// Reap the process in a goroutine to avoid zombies regardless of test outcome.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	parentPID := cmd.Process.Pid

	// Give children time to spawn.
	time.Sleep(200 * time.Millisecond)

	if err := proc.KillSubtree(parentPID); err != nil {
		t.Fatalf("KillSubtree: %v", err)
	}

	// Allow a short window for the signal to be delivered.
	select {
	case <-done:
		// cmd.Wait() returned — parent is gone.
	case <-time.After(2 * time.Second):
		// Fall through; we'll check via gopsutil too.
	}

	// Confirm via gopsutil that the process is no longer running.
	p, err := goproc.NewProcess(int32(parentPID))
	if err == nil {
		running, _ := p.IsRunning()
		if running {
			t.Errorf("process %d still running after KillSubtree", parentPID)
		}
	}
	// If NewProcess errors, the process is already gone — that's a pass.
}

// TestKillSendsTERM starts a shell sleeping for 30 seconds, sends SIGTERM,
// and asserts it terminates.
func TestKillSendsTERM(t *testing.T) {
	t.Helper()

	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	pid := cmd.Process.Pid

	if err := proc.Kill(pid); err != nil {
		// Clean up before failing.
		_ = cmd.Process.Kill()
		t.Fatalf("Kill: %v", err)
	}

	select {
	case <-done:
		// Process ended — pass.
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		t.Error("process did not terminate within 2s after SIGTERM")
	}
}
