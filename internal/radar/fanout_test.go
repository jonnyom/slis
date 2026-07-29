package radar_test

import (
	"context"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/radar"
)

// countingReader is a StackReader that records how often each worktree was read
// and the peak number of reads in flight at once. A non-nil blockOn holds every
// read open until closed, so a test can observe the peak.
type countingReader struct {
	mu      sync.Mutex
	byPath  map[string]int
	live    int64
	peak    int64
	blockOn chan struct{}
}

func newCountingReader() *countingReader {
	return &countingReader{byPath: map[string]int{}}
}

func (c *countingReader) read(_ context.Context, worktreePath string) (gt.State, error) {
	now := atomic.AddInt64(&c.live, 1)
	for {
		old := atomic.LoadInt64(&c.peak)
		if now <= old || atomic.CompareAndSwapInt64(&c.peak, old, now) {
			break
		}
	}

	c.mu.Lock()
	c.byPath[worktreePath]++
	c.mu.Unlock()

	if c.blockOn != nil {
		<-c.blockOn
	}
	atomic.AddInt64(&c.live, -1)
	return gt.State{"trunk": {Trunk: true}}, nil
}

// sharedRepoSlices builds n slices that all span the same repos — the real
// ~/nory shape (one worktree per slice per repo) that triggered the runaway.
func sharedRepoSlices(n int, repos ...string) []model.Slice {
	slices := make([]model.Slice, 0, n)
	for i := 0; i < n; i++ {
		name := "slice-" + strconv.Itoa(i)
		members := make(map[string]model.SliceMember, len(repos))
		for _, repo := range repos {
			members[repo] = model.SliceMember{
				Repo:         repo,
				Branch:       name,
				WorktreePath: "/nonexistent/" + repo + "/" + name,
			}
		}
		slices = append(slices, model.Slice{Name: name, Members: members})
	}
	return slices
}

// ownRepoSlices builds n slices that each live in their OWN repo, so nothing is
// shared through the per-repo cache and every slice needs its own gt read. This
// is the shape that exercises the concurrency cap rather than the cache.
func ownRepoSlices(n int) []model.Slice {
	slices := make([]model.Slice, 0, n)
	for i := 0; i < n; i++ {
		name := "slice-" + strconv.Itoa(i)
		repo := "repo-" + strconv.Itoa(i)
		slices = append(slices, model.Slice{Name: name, Members: map[string]model.SliceMember{
			repo: {Repo: repo, Branch: name, WorktreePath: "/nonexistent/" + repo},
		}})
	}
	return slices
}

// TestCollectStatsReadsGraphiteStateOncePerRepo pins the cache: `gt state` is
// repo-global, so a workspace of N slices across R repos must cost R reads, not
// N*R. Before this, `slis conflicts` spawned one gt per slice-member.
func TestCollectStatsReadsGraphiteStateOncePerRepo(t *testing.T) {
	slices := sharedRepoSlices(6, "web", "api", "worker")
	reader := newCountingReader()

	radar.CollectStats(context.Background(), slices, reader.read)

	reader.mu.Lock()
	defer reader.mu.Unlock()
	if len(reader.byPath) != 3 {
		t.Fatalf("want 3 gt reads (one per repo), got %d: %v", len(reader.byPath), reader.byPath)
	}
	for path, n := range reader.byPath {
		if n != 1 {
			t.Fatalf("%s read %d times, want 1", path, n)
		}
	}
}

// TestCollectStatsBoundsConcurrency proves the per-slice fan-out is capped, so a
// large workspace can never burst-spawn one gt (plus its git children) per slice.
func TestCollectStatsBoundsConcurrency(t *testing.T) {
	slices := ownRepoSlices(4 * radar.MaxConcurrentSlices)
	reader := newCountingReader()
	reader.blockOn = make(chan struct{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		radar.CollectStats(context.Background(), slices, reader.read)
	}()

	// Reads block, so live reads settle at the cap. Wait for saturation rather
	// than a fixed sleep, then release everything.
	deadline := time.Now().Add(5 * time.Second)
	for atomic.LoadInt64(&reader.live) < int64(radar.MaxConcurrentSlices) && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	saturated := atomic.LoadInt64(&reader.live)
	close(reader.blockOn)
	<-done

	if saturated != int64(radar.MaxConcurrentSlices) {
		t.Fatalf("in-flight reads settled at %d, want exactly the cap %d", saturated, radar.MaxConcurrentSlices)
	}
	if peak := atomic.LoadInt64(&reader.peak); peak > int64(radar.MaxConcurrentSlices) {
		t.Fatalf("peak concurrent stack reads = %d, want <= %d", peak, radar.MaxConcurrentSlices)
	}
}

// TestCollectStatsNilReaderDegrades keeps the no-gt path working: with no stack
// reader every slice falls back to trunk auto-detection rather than panicking.
func TestCollectStatsNilReaderDegrades(t *testing.T) {
	slices := sharedRepoSlices(2, "web")
	stats := radar.CollectStats(context.Background(), slices, nil)
	if len(stats) != 2 {
		t.Fatalf("want stats for 2 slices, got %d", len(stats))
	}
}

// TestCollectStatsCancelledBeforeStartDoesNoWork closes the loop with the RPC
// layer: a withdrawn `conflicts` request must not spawn a single subprocess.
func TestCollectStatsCancelledBeforeStartDoesNoWork(t *testing.T) {
	slices := sharedRepoSlices(6, "web", "api", "worker")
	reader := newCountingReader()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stats := radar.CollectStats(ctx, slices, reader.read)

	reader.mu.Lock()
	defer reader.mu.Unlock()
	if len(reader.byPath) != 0 {
		t.Fatalf("cancelled radar still read Graphite state: %v", reader.byPath)
	}
	if len(stats) != 0 {
		t.Fatalf("cancelled radar still produced stats for %d slices", len(stats))
	}
}
