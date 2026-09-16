package forge

import (
	"context"
	"sync"
	"time"
)

// prCacheTTL is how long a PR lookup (hit or miss) is served from memory. The
// TUI's tick is 30s, so one TTL collapses every burst inside a tick — the
// initial bulk load, a reconnect resync, focus changes in lazy mode, a manual
// refresh — into a single gh round-trip per branch, while a state change made
// outside slis (merge, new review, CI finishing) is visible by the next tick.
const prCacheTTL = 30 * time.Second

// prLookupFunc is the shape of the uncached branch → PR resolution.
type prLookupFunc func(ctx context.Context, repoDir, branch string, includeInlineComments bool) (*PR, error)

// prCache memoises PR lookups per (repo dir, branch, inline-comments flag) for
// prCacheTTL. Each uncached lookup is two to three gh processes (pr view, the
// pr list fallback for branches without an open PR, and the paginated inline
// comments API), all network-bound, and the PR stack panel asks for every
// branch of every member's stack on each refresh.
//
// Only clean results are stored: an error (including the partial PR-plus-error
// that a failed inline-comments fetch yields) or a lookup whose context ended
// is handed back but retried next time. Concurrent callers for one key share
// the in-flight lookup.
type prCache struct {
	now func() time.Time

	mu      sync.Mutex
	entries map[prCacheKey]*prCacheEntry
}

type prCacheKey struct {
	repoDir string
	branch  string
	inline  bool
}

type prCacheEntry struct {
	mu       sync.Mutex
	valid    bool
	storedAt time.Time
	pr       *PR
}

func newPRCache() *prCache {
	return &prCache{now: time.Now, entries: map[prCacheKey]*prCacheEntry{}}
}

func (c *prCache) lookup(ctx context.Context, repoDir, branch string, includeInlineComments bool, fn prLookupFunc) (*PR, error) {
	key := prCacheKey{repoDir: repoDir, branch: branch, inline: includeInlineComments}
	c.mu.Lock()
	entry := c.entries[key]
	if entry == nil {
		entry = &prCacheEntry{}
		c.entries[key] = entry
	}
	c.mu.Unlock()

	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := c.now()
	if entry.valid && now.Sub(entry.storedAt) < prCacheTTL {
		return entry.pr, nil
	}

	pr, err := fn(ctx, repoDir, branch, includeInlineComments)
	if err != nil || ctx.Err() != nil {
		return pr, err
	}
	entry.valid = true
	entry.storedAt = now
	entry.pr = pr
	return pr, nil
}
