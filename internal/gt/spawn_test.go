package gt

import (
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
