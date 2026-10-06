package gt

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jonnyom/slis/testutil"
)

// countingReader is a fake gt read that records how many times it was invoked
// and returns a fixed State, so the cache's spawn-saving behaviour can be
// asserted without the gt binary.
func countingReader(state State) (read func(context.Context, string) (State, error), calls *atomic.Int32) {
	calls = new(atomic.Int32)
	return func(context.Context, string) (State, error) {
		calls.Add(1)
		return state, nil
	}, calls
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestStackCacheReusesStateWhileRefsUnchanged(t *testing.T) {
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	first, err := c.Read(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.Read(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying reads = %d, want 1", got)
	}
	if !first["main"].Trunk || !second["main"].Trunk {
		t.Fatalf("cached state lost content: first=%v second=%v", first, second)
	}
}

func TestStackCacheRereadsAfterLocalBranchRefChanges(t *testing.T) {
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "commit", "-q", "--allow-empty", "-m", "advance main")
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying reads = %d, want 2 (refs/heads changed)", got)
	}
}

func TestStackCacheRereadsAfterGraphiteMetadataRefChanges(t *testing.T) {
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	// Graphite (refs backend) records stack shape under refs/branch-metadata/;
	// a new entry there must invalidate even though no branch moved.
	head := gitIn(t, repo, "rev-parse", "HEAD")
	gitIn(t, repo, "update-ref", "refs/branch-metadata/feat", head[:40])
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying reads = %d, want 2 (refs/branch-metadata changed)", got)
	}
}

func TestStackCacheIgnoresUnrelatedRefNamespaces(t *testing.T) {
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	head := gitIn(t, repo, "rev-parse", "HEAD")
	gitIn(t, repo, "update-ref", "refs/notes/whatever", head[:40])
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying reads = %d, want 1 (refs/notes is not stack state)", got)
	}
}

func TestStackCacheSharesEntryAcrossWorktreesOfOneRepo(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "feat")
	testutil.AddWorktree(t, repo, "feat", wt)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(context.Background(), wt); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying reads = %d, want 1 (gt state is repo-global)", got)
	}
}

func TestStackCacheIsolatesDistinctRepos(t *testing.T) {
	a := testutil.NewRepo(t)
	b := testutil.NewRepo(t)
	var seen []string
	var mu sync.Mutex
	c := newStackCache(func(_ context.Context, dir string) (State, error) {
		mu.Lock()
		seen = append(seen, dir)
		mu.Unlock()
		return State{"main": {Trunk: true}}, nil
	})

	if _, err := c.Read(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("underlying reads = %v, want one per repo", seen)
	}
}

func TestStackCacheDoesNotStoreErrors(t *testing.T) {
	repo := testutil.NewRepo(t)
	var calls atomic.Int32
	c := newStackCache(func(context.Context, string) (State, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("gt exploded")
		}
		return State{"main": {Trunk: true}}, nil
	})

	if _, err := c.Read(context.Background(), repo); err == nil {
		t.Fatal("first read should surface the error")
	}
	st, err := c.Read(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) == 0 || calls.Load() != 2 {
		t.Fatalf("error must not be cached: state=%v calls=%d", st, calls.Load())
	}
}

func TestStackCacheDoesNotStoreCancelledRead(t *testing.T) {
	repo := testutil.NewRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	c := newStackCache(func(context.Context, string) (State, error) {
		if calls.Add(1) == 1 {
			// A killed `gt state` parses to an empty State and the refs fallback
			// may also be cut short: the result is partial, not authoritative.
			cancel()
			return State{}, nil
		}
		return State{"main": {Trunk: true}}, nil
	})

	_, _ = c.Read(ctx, repo)
	st, err := c.Read(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(st) == 0 || calls.Load() != 2 {
		t.Fatalf("partial read must not be cached: state=%v calls=%d", st, calls.Load())
	}
}

func TestStackCacheCollapsesConcurrentReadsOfOneRepo(t *testing.T) {
	repo := testutil.NewRepo(t)
	var calls atomic.Int32
	c := newStackCache(func(context.Context, string) (State, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return State{"main": {Trunk: true}}, nil
	})

	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Read(context.Background(), repo); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying reads = %d, want 1 (single-flight)", got)
	}
}

func TestStackCacheBypassesForNonRepoDir(t *testing.T) {
	dir := t.TempDir()
	read, calls := countingReader(State{})
	c := newStackCache(read)

	if _, err := c.Read(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying reads = %d, want 2 (no fingerprint → no caching)", got)
	}
}

// TestReadStackCtxSeesMetadataChangeThroughCache is the end-to-end guard: the
// package-level ReadStackCtx must never return stale stack shape after Graphite
// metadata moves. Uses the pure-git refs fallback (gt need not be installed).
func TestReadStackCtxSeesMetadataChangeThroughCache(t *testing.T) {
	if _, err := exec.LookPath("gt"); err == nil {
		t.Skip("gt installed: state would come from gt, not the refs fallback")
	}
	repo := testutil.NewRepo(t)
	gitIn(t, repo, "branch", "feat")
	writeMeta := func(parent string) {
		tmp := filepath.Join(t.TempDir(), "meta.json")
		if err := os.WriteFile(tmp, []byte(`{"parentBranchName":"`+parent+`","parentBranchRevision":"x"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		blob := gitIn(t, repo, "hash-object", "-w", tmp)
		gitIn(t, repo, "update-ref", "refs/branch-metadata/feat", blob[:40])
	}

	writeMeta("main")
	st, err := ReadStackCtx(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := st["feat"].Parents; len(got) != 1 || got[0].Ref != "main" {
		t.Fatalf("feat parents = %v, want [main]", got)
	}

	writeMeta("other")
	st, err = ReadStackCtx(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := st["feat"].Parents; len(got) != 1 || got[0].Ref != "other" {
		t.Fatalf("after metadata change feat parents = %v, want [other] (stale cache?)", got)
	}
}

func TestStackCacheRereadsAfterGraphiteStoreFileChanges(t *testing.T) {
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	// Newer Graphite CLIs keep stack metadata in a store file under .git/
	// rather than in refs; touching it must invalidate the cache.
	store := filepath.Join(repo, ".git", ".graphite_cache_persist")
	if err := os.WriteFile(store, []byte(`{"branches":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying reads = %d, want 2 (.graphite* store changed)", got)
	}

	// And a content change (same name) must invalidate again.
	future := time.Now().Add(2 * time.Second)
	if err := os.WriteFile(store, []byte(`{"branches":[{"name":"feat"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(store, future, future); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("underlying reads = %d, want 3 (.graphite* store rewritten)", got)
	}
}

func TestStackCacheExpiresAfterMaxAge(t *testing.T) {
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)
	now := time.Unix(1_700_000_000, 0)
	c.now = func() time.Time { return now }

	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	now = now.Add(stackCacheMaxAge - time.Second)
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying reads = %d, want 1 (still fresh)", got)
	}
	now = now.Add(2 * time.Second)
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying reads = %d, want 2 (max age passed)", got)
	}
}

func TestStackCacheInvalidateForcesReread(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "feat")
	testutil.AddWorktree(t, repo, "feat", wt)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	// Invalidating through ANY worktree of the repo drops the shared entry, so
	// a mutator run in a linked worktree (gt track / restack) is seen everywhere.
	c.Invalidate(wt)
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying reads = %d, want 2 (after Invalidate)", got)
	}
}

// TestReadStackCtxSeesGraphiteStoreChangeThroughCache runs the real gt CLI:
// modern Graphite keeps stack metadata in .git/.graphite_metadata.db (SQLite),
// never touching refs, and rewrites it in place at a constant size. A `gt
// track` between two cached reads must still be visible, or the cockpit would
// show a stale stack for up to stackCacheMaxAge after every metadata-only edit.
func TestReadStackCtxSeesGraphiteStoreChangeThroughCache(t *testing.T) {
	if _, err := exec.LookPath("gt"); err != nil {
		t.Skip("gt not installed")
	}
	repo := testutil.NewRepo(t)
	gt := func(args ...string) {
		t.Helper()
		cmd := exec.Command("gt", append(args, "--no-interactive")...)
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gt %v: %v\n%s", args, err, out)
		}
	}
	gt("init", "--trunk", "main")
	gitIn(t, repo, "branch", "feat")
	gitIn(t, repo, "branch", "other")

	// Prime the cache with feat untracked.
	st, err := ReadStackCtx(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, tracked := st["feat"]; tracked {
		t.Fatalf("feat unexpectedly tracked before gt track: %v", st)
	}

	gt("track", "--parent", "main", "feat")
	st, err = ReadStackCtx(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := st["feat"].Parents; len(got) != 1 || got[0].Ref != "main" {
		t.Fatalf("after gt track feat parents = %v, want [main] (stale cache?)", got)
	}

	// Metadata-only re-parent: no ref moves, store size unchanged. (Graphite
	// only accepts a tracked branch as parent, so track `other` first.)
	gt("track", "--parent", "main", "other")
	gt("track", "--parent", "other", "feat")
	st, err = ReadStackCtx(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if got := st["feat"].Parents; len(got) != 1 || got[0].Ref != "other" {
		t.Fatalf("after re-parent feat parents = %v, want [other] (stale cache?)", got)
	}
}

func TestStackCacheIgnoresGraphitePRInfoScratchFile(t *testing.T) {
	// Every `gt` invocation rewrites .git/.graphite_pr_info (PR lookup scratch,
	// not stack shape). Fingerprinting it would make each read invalidate the
	// next — exactly the per-tick re-spawn the cache exists to stop.
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	scratch := filepath.Join(repo, ".git", ".graphite_pr_info")
	if err := os.WriteFile(scratch, []byte(`{"prInfoToUpsert":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(scratch, future, future); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying reads = %d, want 1 (.graphite_pr_info is not stack state)", got)
	}
}

func writeRepoConfig(t *testing.T, repo, trunk string, fetchedMs int64) {
	t.Helper()
	content := []byte(`{
  "trunk": "` + trunk + `",
  "trunks": [{"name": "` + trunk + `"}],
  "lastFetchedPRInfoMs": ` + itoa(fetchedMs) + `,
  "lastFetchedFeatureFlagsInMs": 1789507030122
}`)
	path := filepath.Join(repo, ".git", ".graphite_repo_config")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	// Force a distinct mtime per write so a stat-based digest WOULD change;
	// the test then proves the digest ignores it.
	future := time.Now().Add(time.Duration(fetchedMs%1000+1) * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestStackCacheIgnoresRepoConfigFetchTimestamps(t *testing.T) {
	// gt bumps lastFetchedPRInfoMs / lastFetchedFeatureFlagsInMs in
	// .graphite_repo_config on its own schedule; only the trunk fields are
	// stack state.
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	writeRepoConfig(t, repo, "main", 1789507129961)
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	writeRepoConfig(t, repo, "main", 1789507199999)
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying reads = %d, want 1 (only fetch timestamps changed)", got)
	}
}

func TestStackCacheRereadsWhenRepoConfigTrunkChanges(t *testing.T) {
	repo := testutil.NewRepo(t)
	read, calls := countingReader(State{"main": {Trunk: true}})
	c := newStackCache(read)

	writeRepoConfig(t, repo, "main", 1)
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	writeRepoConfig(t, repo, "develop", 1)
	if _, err := c.Read(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying reads = %d, want 2 (trunk changed)", got)
	}
}
