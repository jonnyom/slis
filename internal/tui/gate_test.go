package tui

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestGatedCmdCapsConcurrency launches far more gated commands than the gate
// allows and asserts that no more than bgConcurrency ever run their body at
// once. This is the core protection against the startup subprocess storm.
func TestGatedCmdCapsConcurrency(t *testing.T) {
	const launches = 50

	var inFlight int32
	var maxObserved int32

	cmd := gatedCmd(func() tea.Msg {
		n := atomic.AddInt32(&inFlight, 1)
		for {
			old := atomic.LoadInt32(&maxObserved)
			if n <= old || atomic.CompareAndSwapInt32(&maxObserved, old, n) {
				break
			}
		}
		// Hold the slot briefly so concurrent launches actually overlap.
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < launches; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cmd()
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxObserved); got > bgConcurrency {
		t.Fatalf("max concurrent gated bodies = %d, want <= %d", got, bgConcurrency)
	}
	if atomic.LoadInt32(&maxObserved) == 0 {
		t.Fatal("gated command never ran")
	}
}

// TestGatedCmdNilPassthrough guards the nil case so wrapping a no-op loader
// stays a no-op.
func TestGatedCmdNilPassthrough(t *testing.T) {
	if gatedCmd(nil) != nil {
		t.Fatal("gatedCmd(nil) should be nil")
	}
}
