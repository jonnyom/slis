package forge

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func countingLookup(pr *PR) (fn prLookupFunc, calls *atomic.Int32) {
	calls = new(atomic.Int32)
	return func(context.Context, string, string, bool) (*PR, error) {
		calls.Add(1)
		return pr, nil
	}, calls
}

func TestPRCacheReusesResultWithinTTL(t *testing.T) {
	lookup, calls := countingLookup(&PR{Branch: "feat", Number: 7})
	c := newPRCache()

	first, err := c.lookup(context.Background(), "/repo", "feat", true, lookup)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.lookup(context.Background(), "/repo", "feat", true, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("gh lookups = %d, want 1", calls.Load())
	}
	if first.Number != 7 || second.Number != 7 {
		t.Fatalf("cached PR lost content: %+v %+v", first, second)
	}
}

func TestPRCacheExpiresAfterTTL(t *testing.T) {
	lookup, calls := countingLookup(&PR{Branch: "feat", Number: 7})
	c := newPRCache()
	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }

	if _, err := c.lookup(context.Background(), "/repo", "feat", true, lookup); err != nil {
		t.Fatal(err)
	}
	now = now.Add(prCacheTTL - time.Second)
	if _, err := c.lookup(context.Background(), "/repo", "feat", true, lookup); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("gh lookups = %d, want 1 (still fresh)", calls.Load())
	}
	now = now.Add(2 * time.Second)
	if _, err := c.lookup(context.Background(), "/repo", "feat", true, lookup); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("gh lookups = %d, want 2 (TTL passed)", calls.Load())
	}
}

func TestPRCacheRemembersBranchWithoutPR(t *testing.T) {
	// A branch with no PR costs `gh pr view` + `gh pr list` every time; the
	// absence is as worth remembering as a hit.
	lookup, calls := countingLookup(nil)
	c := newPRCache()

	for range 3 {
		pr, err := c.lookup(context.Background(), "/repo", "feat", true, lookup)
		if err != nil || pr != nil {
			t.Fatalf("lookup = %v, %v; want nil, nil", pr, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("gh lookups = %d, want 1", calls.Load())
	}
}

func TestPRCacheDoesNotStoreErrors(t *testing.T) {
	var calls atomic.Int32
	lookup := func(context.Context, string, string, bool) (*PR, error) {
		if calls.Add(1) == 1 {
			// Mirrors prForBranchCtx's partial result: the PR resolved but the
			// inline-comments fetch failed. Callers render it; we must retry it.
			return &PR{Branch: "feat", Number: 7}, errors.New("inline comments: rate limited")
		}
		return &PR{Branch: "feat", Number: 7, Comments: []Comment{{Body: "hi"}}}, nil
	}
	c := newPRCache()

	pr, err := c.lookup(context.Background(), "/repo", "feat", true, lookup)
	if err == nil || pr == nil {
		t.Fatalf("first lookup should return the partial PR and the error, got %v %v", pr, err)
	}
	pr, err = c.lookup(context.Background(), "/repo", "feat", true, lookup)
	if err != nil || len(pr.Comments) != 1 || calls.Load() != 2 {
		t.Fatalf("error result must not be cached: pr=%+v err=%v calls=%d", pr, err, calls.Load())
	}
}

func TestPRCacheKeysByRepoBranchAndInlineFlag(t *testing.T) {
	lookup, calls := countingLookup(&PR{Number: 1})
	c := newPRCache()

	for _, k := range []struct {
		repo, branch string
		inline       bool
	}{
		{"/a", "feat", true},
		{"/a", "feat", false},
		{"/a", "other", true},
		{"/b", "feat", true},
		{"/a", "feat", true}, // repeat → cached
	} {
		if _, err := c.lookup(context.Background(), k.repo, k.branch, k.inline, lookup); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("gh lookups = %d, want 4 distinct keys", calls.Load())
	}
}

func TestPRCacheDoesNotStoreCancelledLookup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	lookup := func(context.Context, string, string, bool) (*PR, error) {
		if calls.Add(1) == 1 {
			cancel()
			return nil, nil // a killed gh looks like "no PR"
		}
		return &PR{Number: 7}, nil
	}
	c := newPRCache()

	_, _ = c.lookup(ctx, "/repo", "feat", true, lookup)
	pr, err := c.lookup(context.Background(), "/repo", "feat", true, lookup)
	if err != nil || pr == nil || calls.Load() != 2 {
		t.Fatalf("cancelled result must not be cached: pr=%v err=%v calls=%d", pr, err, calls.Load())
	}
}

func TestPRCacheCollapsesConcurrentLookupsOfOneKey(t *testing.T) {
	var calls atomic.Int32
	lookup := func(context.Context, string, string, bool) (*PR, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return &PR{Number: 7}, nil
	}
	c := newPRCache()

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.lookup(context.Background(), "/repo", "feat", true, lookup); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("gh lookups = %d, want 1 (single-flight)", calls.Load())
	}
}

// TestPRForBranchGoesThroughCache pins the wiring: the public lookups must
// consult the process-wide cache, or every refresh keeps paying for gh.
func TestPRForBranchGoesThroughCache(t *testing.T) {
	orig := lookupPRUncached
	t.Cleanup(func() {
		lookupPRUncached = orig
		prs = newPRCache()
	})
	prs = newPRCache()
	lookup, calls := countingLookup(&PR{Branch: "feat", Number: 7})
	lookupPRUncached = lookup

	repo := t.TempDir()
	if _, err := PRForBranchCtx(context.Background(), repo, "feat"); err != nil {
		t.Fatal(err)
	}
	got, err := PRsForBranchesCtx(context.Background(), repo, []string{"feat"})
	if err != nil {
		t.Fatal(err)
	}
	if got["feat"] == nil || got["feat"].Number != 7 {
		t.Fatalf("PRsForBranchesCtx = %+v, want cached PR #7", got["feat"])
	}
	if calls.Load() != 1 {
		t.Fatalf("gh lookups = %d, want 1 (second call served from cache)", calls.Load())
	}
}
