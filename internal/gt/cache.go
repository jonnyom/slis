package gt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jonnyom/slis/internal/git"
)

// stackCacheMaxAge bounds how long a cached stack may be served without a
// re-read even when its fingerprint has not changed. It is the backstop for a
// Graphite store the fingerprint cannot see (a metadata backend slis does not
// know about, or a change that leaves refs and store files untouched).
const stackCacheMaxAge = 2 * time.Minute

// stackCache memoises stack reads per REPOSITORY for the lifetime of the
// process. `gt state` is repo-global (every worktree of a repo returns the same
// stack metadata) and the TUI's 30s tick reaches it from several independent
// RPC methods (ls, show, prStack, conflicts), each of which used to spawn its
// own gt process per repo — plus the heavyweight background helpers Graphite
// launches alongside every invocation. Sharing one read per repo, validated by
// a cheap git fingerprint, collapses that to at most one spawn per repo per
// actual stack change.
//
// Correctness over savings: a result is only stored when the read completed
// with a live context and no error; a fingerprint failure bypasses the cache
// entirely; and concurrent callers for one repo wait on the single in-flight
// read instead of racing their own.
type stackCache struct {
	read func(ctx context.Context, repoDir string) (State, error)
	now  func() time.Time

	mu      sync.Mutex
	entries map[string]*stackCacheEntry // common git dir → entry
	dirs    map[string]string           // worktree path → common git dir
}

type stackCacheEntry struct {
	mu          sync.Mutex
	valid       bool
	fingerprint string
	readAt      time.Time
	state       State
}

func newStackCache(read func(ctx context.Context, repoDir string) (State, error)) *stackCache {
	return &stackCache{
		read:    read,
		now:     time.Now,
		entries: map[string]*stackCacheEntry{},
		dirs:    map[string]string{},
	}
}

// Read returns repoDir's stack, from cache when the repo's fingerprint is
// unchanged and the entry is younger than stackCacheMaxAge, else via one
// underlying read shared by every concurrent caller for the same repo.
func (c *stackCache) Read(ctx context.Context, repoDir string) (State, error) {
	key, ok := c.commonDir(ctx, repoDir)
	if !ok {
		return c.read(ctx, repoDir)
	}
	fingerprint, err := stackFingerprint(ctx, repoDir, key)
	if err != nil {
		return c.read(ctx, repoDir)
	}

	entry := c.entry(key)
	entry.mu.Lock()
	defer entry.mu.Unlock()

	now := c.now()
	if entry.valid && entry.fingerprint == fingerprint && now.Sub(entry.readAt) < stackCacheMaxAge {
		return entry.state, nil
	}

	state, err := c.read(ctx, repoDir)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		// The read was cut short: gt may have been killed mid-output and parsed
		// to a partial or empty State. Hand it back (callers degrade) but never
		// remember it as the repo's stack.
		return state, nil
	}
	entry.valid = true
	entry.fingerprint = fingerprint
	entry.readAt = now
	entry.state = state
	return state, nil
}

// Invalidate drops the cached stack for the repo containing repoDir (any
// worktree of it). Mutators that change Graphite metadata without moving a
// ref slis fingerprints (gt track) call this so the next read is fresh.
func (c *stackCache) Invalidate(repoDir string) {
	key, ok := c.commonDir(context.Background(), repoDir)
	if !ok {
		return
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *stackCache) entry(key string) *stackCacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.entries[key]
	if entry == nil {
		entry = &stackCacheEntry{}
		c.entries[key] = entry
	}
	return entry
}

// commonDir resolves repoDir's shared git directory, which is the same for the
// primary checkout and every linked worktree, so it identifies the repository.
// The mapping is immutable for a given path and memoised.
func (c *stackCache) commonDir(ctx context.Context, repoDir string) (string, bool) {
	c.mu.Lock()
	dir, ok := c.dirs[repoDir]
	c.mu.Unlock()
	if ok {
		return dir, true
	}
	out, err := git.RunCtx(ctx, repoDir, "rev-parse", "--git-common-dir")
	if err != nil || strings.TrimSpace(out) == "" {
		return "", false
	}
	dir = strings.TrimSpace(out)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(repoDir, dir)
	}
	dir = filepath.Clean(dir)
	// git reports the primary's dir relative (".git") but a linked worktree's
	// absolute AND symlink-resolved (macOS: /var → /private/var), so resolve
	// both to one canonical key.
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	c.mu.Lock()
	c.dirs[repoDir] = dir
	c.mu.Unlock()
	return dir, true
}

// graphiteStoreGlobs names the files inside the common git dir where Graphite
// persists STACK metadata by stat: the current SQLite store (plus its
// -wal/-shm/-journal siblings) and the pre-SQLite JSON cache. Deliberately NOT
// `.graphite*`: `.graphite_pr_info` and `.gtlocalprinfo` are PR-lookup scratch
// that every gt invocation rewrites, so fingerprinting them would make each
// `gt state` invalidate the next read and re-spawn per tick.
var graphiteStoreGlobs = []string{
	".graphite_metadata.db*",
	".graphite_cache_persist*",
}

// graphiteRepoConfig fixes the repo's trunk(s) but also carries fetch
// timestamps gt bumps on its own schedule, so it is digested by content
// (trunk fields only), never by stat.
const graphiteRepoConfig = ".graphite_repo_config"

// stackFingerprint digests everything Graphite stack shape can derive from:
// local branch tips and the refs-backend metadata namespace (one git spawn),
// size+mtime of Graphite's stack stores inside the common git dir (newer CLIs
// persist metadata there instead of in refs, rewriting the SQLite file in place
// at constant size — hence mtime, at nanosecond resolution), and the trunk
// fields of the repo config.
func stackFingerprint(ctx context.Context, repoDir, commonDir string) (string, error) {
	refs, err := git.RunCtx(ctx, repoDir, "for-each-ref",
		"--format=%(refname)%00%(objectname)", "refs/heads/", "refs/branch-metadata/")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, _ = h.Write([]byte(refs))
	_, _ = h.Write([]byte{0})

	var stores []string
	for _, glob := range graphiteStoreGlobs {
		matches, _ := filepath.Glob(filepath.Join(commonDir, glob))
		stores = append(stores, matches...)
	}
	sort.Strings(stores)
	for _, store := range stores {
		writeStatDigest(h, store)
	}
	writeTrunkDigest(h, filepath.Join(commonDir, graphiteRepoConfig))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// writeTrunkDigest folds the trunk fields of Graphite's repo config into h,
// ignoring its volatile fetch timestamps. A missing or unparseable file
// contributes nothing (the refs and stores still fingerprint the stack).
func writeTrunkDigest(h interface{ Write([]byte) (int, error) }, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var cfg struct {
		Trunk  string `json:"trunk"`
		Trunks []struct {
			Name string `json:"name"`
		} `json:"trunks"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return
	}
	_, _ = fmt.Fprintf(h, "trunk\x00%s\x00", cfg.Trunk)
	for _, t := range cfg.Trunks {
		_, _ = fmt.Fprintf(h, "%s\x00", t.Name)
	}
}

func writeStatDigest(h interface{ Write([]byte) (int, error) }, path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(h, "%s\x00%d\x00%d\x00", path, info.Size(), info.ModTime().UnixNano())
}
