package gt

import (
	"context"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSpawnSlotCapsConcurrency proves the package-level spawn gate never lets
// more than maxConcurrentSpawns gt processes run at once, however many callers
// pile in. This is the backstop that keeps a fan-out bug from pinning the machine
// with runaway gt processes again.
func TestSpawnSlotCapsConcurrency(t *testing.T) {
	const callers = 40

	var live, peak int64
	var wg sync.WaitGroup
	release := make(chan struct{})

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			done := acquireSpawnSlot()
			defer done()

			now := atomic.AddInt64(&live, 1)
			for {
				old := atomic.LoadInt64(&peak)
				if now <= old || atomic.CompareAndSwapInt64(&peak, old, now) {
					break
				}
			}
			<-release
			atomic.AddInt64(&live, -1)
		}()
	}

	// Every goroutine that can hold a slot blocks on release, so live settles at
	// the cap. Wait for that rather than sleeping, then drain.
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt64(&live) < int64(maxConcurrentSpawns) && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	saturated := atomic.LoadInt64(&live)
	close(release)
	wg.Wait()

	if saturated != int64(maxConcurrentSpawns) {
		t.Fatalf("in-flight spawns settled at %d, want exactly the cap %d", saturated, maxConcurrentSpawns)
	}
	if peak > int64(maxConcurrentSpawns) {
		t.Fatalf("peak concurrent gt spawns = %d, want <= %d", peak, maxConcurrentSpawns)
	}
	if got := atomic.LoadInt64(&live); got != 0 {
		t.Fatalf("slots leaked: %d callers still holding", got)
	}
}

// TestReadStateCtxCancelledSpawnsNothing proves the request context reaches the
// spawn decision: a withdrawn read must not start a gt process at all.
func TestReadStateCtxCancelledSpawnsNothing(t *testing.T) {
	if _, err := exec.LookPath("gt"); err != nil {
		t.Skip("gt not installed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	st, err := ReadStateCtx(ctx, t.TempDir())
	if err == nil {
		t.Fatal("want the context error, got nil")
	}
	if len(st) != 0 {
		t.Fatalf("want empty state, got %d branches", len(st))
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("cancelled read took %s — it spawned gt anyway", elapsed)
	}
}
