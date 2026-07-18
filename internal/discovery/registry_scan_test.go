package discovery_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/testutil"
)

// regPath returns a fresh registry path inside a temp dir (isolated per test, so
// the user's real registry is never touched).
func regPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "registry.yaml")
}

// writeEmptyRegistry marks the registry as existing (so Report does NOT
// grandfather) while managing nothing.
func writeEmptyRegistry(t *testing.T, path string) {
	t.Helper()
	if err := config.SaveRegistry(path, config.Registry{}); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
}

func candidateFor(t *testing.T, cands []discovery.Candidate, path string) (discovery.Candidate, bool) {
	t.Helper()
	for _, c := range cands {
		if resolvePath(t, c.Path) == resolvePath(t, path) {
			return c, true
		}
	}
	return discovery.Candidate{}, false
}

// A healthy worktree that is neither under the managed tree nor in the registry
// must NOT become a slice — it is a candidate awaiting opt-in import.
func TestReport_CandidateNotAutoIngested(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "loose")
	testutil.AddWorktree(t, repo, "jonny/loose", wt)

	rp := regPath(t)
	writeEmptyRegistry(t, rp) // registry exists → no grandfathering

	res := discovery.Report(wsFor(map[string]string{"web": repo}), rp)

	if len(res.Slices) != 0 {
		t.Fatalf("candidate must not be ingested as a slice, got %+v", res.Slices)
	}
	c, ok := candidateFor(t, res.Candidates, wt)
	if !ok {
		t.Fatalf("expected a candidate for %s, got %+v", wt, res.Candidates)
	}
	if c.Slice != "loose" || c.Branch != "jonny/loose" || c.Repo != "web" {
		t.Fatalf("candidate fields wrong: %+v", c)
	}
}

// After importing (registering) a worktree, it must appear as a slice and keep
// appearing across subsequent discovery runs.
func TestReport_ImportedPersistsAsSlice(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "feat")
	testutil.AddWorktree(t, repo, "jonny/feat", wt)

	rp := regPath(t)
	writeEmptyRegistry(t, rp)
	ws := wsFor(map[string]string{"web": repo})

	// It starts as a candidate.
	if got := discovery.Report(ws, rp); len(got.Slices) != 0 || len(got.Candidates) != 1 {
		t.Fatalf("precondition: want 0 slices / 1 candidate, got %+v", got)
	}

	// Import: register the worktree's slice.
	reg, _, _ := config.LoadRegistry(rp)
	reg.Slices["feat"] = config.RegistrySlice{
		Name:    "feat",
		Source:  config.SourceImported,
		Members: map[string]config.RegistryMember{"web": {Branch: "jonny/feat", WorktreePath: wt}},
	}
	if err := config.SaveRegistry(rp, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	for i := 0; i < 2; i++ { // persists across runs
		res := discovery.Report(ws, rp)
		if len(res.Slices) != 1 || res.Slices[0].Name != "feat" {
			t.Fatalf("run %d: expected slice 'feat', got %+v", i, res.Slices)
		}
		if len(res.Candidates) != 0 {
			t.Fatalf("run %d: imported worktree must not still be a candidate, got %+v", i, res.Candidates)
		}
	}
}

// Creating a stacked branch inside a registered worktree changes its current
// branch, not its slice membership. The repo must remain in the original slice
// and expose the new branch to stack discovery.
func TestReport_RegisteredWorktreeBranchSwitchStaysInSlice(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "test", "web")
	testutil.AddWorktree(t, repo, "test", wt)

	rp := regPath(t)
	reg := config.Registry{Slices: map[string]config.RegistrySlice{
		"test": {
			Name:   "test",
			Source: config.SourceCreated,
			Members: map[string]config.RegistryMember{
				"web": {Branch: "test", WorktreePath: wt},
			},
		},
	}}
	if err := config.SaveRegistry(rp, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
	if _, err := git.Run(wt, "switch", "-c", "jonny/test-2"); err != nil {
		t.Fatalf("switch stacked branch: %v", err)
	}

	res := discovery.Report(wsFor(map[string]string{"web": repo}), rp)
	if len(res.Slices) != 1 || res.Slices[0].Name != "test" {
		t.Fatalf("branch switch must preserve slice test, got %+v", res.Slices)
	}
	member, ok := res.Slices[0].Members["web"]
	if !ok || member.Branch != "jonny/test-2" || resolvePath(t, member.WorktreePath) != resolvePath(t, wt) {
		t.Fatalf("expected current stacked branch in original slice, got %+v", member)
	}
	if len(res.Missing) != 0 {
		t.Fatalf("healthy branch switch must not be missing, got %+v", res.Missing)
	}
}

func TestReport_ManagedTreeBranchSwitchStaysInDirectorySlice(t *testing.T) {
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	wt := filepath.Join(root, ".slis", "worktrees", "test", "web")
	testutil.AddWorktree(t, repo, "test", wt)
	if _, err := git.Run(wt, "switch", "-c", "jonny/test-2"); err != nil {
		t.Fatalf("switch stacked branch: %v", err)
	}

	rp := regPath(t)
	writeEmptyRegistry(t, rp)
	ws := wsFor(map[string]string{"web": repo})
	ws.Root = root
	res := discovery.Report(ws, rp)

	if len(res.Slices) != 1 || res.Slices[0].Name != "test" {
		t.Fatalf("managed path must preserve slice test, got %+v", res.Slices)
	}
	member, ok := res.Slices[0].Members["web"]
	if !ok || member.Branch != "jonny/test-2" {
		t.Fatalf("expected current stacked branch in managed slice, got %+v", member)
	}
}

func TestReport_ManagedTreePreservesNestedSliceName(t *testing.T) {
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	wt := filepath.Join(root, ".slis", "worktrees", "group", "feature", "web")
	testutil.AddWorktree(t, repo, "jonny/feature-2", wt)

	rp := regPath(t)
	writeEmptyRegistry(t, rp)
	ws := wsFor(map[string]string{"web": repo})
	ws.Root = root
	res := discovery.Report(ws, rp)

	if len(res.Slices) != 1 || res.Slices[0].Name != "group/feature" {
		t.Fatalf("nested managed path must preserve group/feature, got %+v", res.Slices)
	}
}

func TestReport_BackfillsManagedWorktreeIntoExistingRegistry(t *testing.T) {
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	wt := filepath.Join(root, ".slis", "worktrees", "legacy", "web")
	testutil.AddWorktree(t, repo, "jonny/legacy", wt)

	rp := regPath(t)
	writeEmptyRegistry(t, rp) // simulates a registry created by an older release
	ws := wsFor(map[string]string{"web": repo})
	ws.Root = root
	res := discovery.Report(ws, rp)
	if len(res.Slices) != 1 || res.Slices[0].Name != "legacy" {
		t.Fatalf("managed worktree must remain visible, got %+v", res.Slices)
	}

	reg, exists, err := config.LoadRegistry(rp)
	if err != nil || !exists {
		t.Fatalf("LoadRegistry: exists=%v err=%v", exists, err)
	}
	slice, ok := reg.Slices["legacy"]
	if !ok || slice.Source != config.SourceCreated {
		t.Fatalf("managed worktree was not backfilled as created: %+v", reg.Slices)
	}
	member, ok := slice.Members["web"]
	if !ok || member.Branch != "jonny/legacy" || resolvePath(t, member.WorktreePath) != resolvePath(t, wt) {
		t.Fatalf("backfilled member is wrong: %+v", member)
	}

	before, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	_ = discovery.Report(ws, rp)
	after, err := os.ReadFile(rp)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("managed registry backfill must be idempotent")
	}
}

func TestReportRemovesOnlyEmptyManagedDirectoryLitter(t *testing.T) {
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	empty := filepath.Join(root, ".slis", "worktrees", "old", "web")
	freshEmpty := filepath.Join(root, ".slis", "worktrees", "creating", "web")
	nonEmpty := filepath.Join(root, ".slis", "worktrees", "inspect", "web")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nonEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(freshEmpty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nonEmpty, "recovery.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	for _, path := range []string{empty, filepath.Dir(empty)} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	rp := regPath(t)
	writeEmptyRegistry(t, rp)
	ws := wsFor(map[string]string{"web": repo})
	ws.Root = root
	_ = discovery.Report(ws, rp)

	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatalf("empty legacy directory was not cleaned up: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nonEmpty, "recovery.txt")); err != nil {
		t.Fatalf("non-empty recovery directory was altered: %v", err)
	}
	if _, err := os.Stat(freshEmpty); err != nil {
		t.Fatalf("fresh empty directory may belong to an in-flight create: %v", err)
	}
}

func TestReportQuarantinesBrokenRegistryAndGrandfathers(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "legacy")
	testutil.AddWorktree(t, repo, "jonny/legacy", wt)
	rp := regPath(t)
	if err := os.WriteFile(rp, []byte("slices: [broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := discovery.Report(wsFor(map[string]string{"web": repo}), rp)
	if len(res.Slices) != 1 || res.Slices[0].Name != "legacy" {
		t.Fatalf("broken registry recovery hid healthy worktrees: %+v", res.Slices)
	}
	reg, exists, err := config.LoadRegistry(rp)
	if err != nil || !exists || reg.Slices["legacy"].Source != config.SourceGrandfathered {
		t.Fatalf("fresh registry was not rebuilt: exists=%v err=%v reg=%+v", exists, err, reg)
	}
	backups, err := filepath.Glob(rp + ".broken-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("broken registry was not quarantined: backups=%v err=%v", backups, err)
	}
}

func TestReportRepairsDeletedManagedWorktreeAndRegistry(t *testing.T) {
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	wt := filepath.Join(root, ".slis", "worktrees", "old", "web")
	testutil.AddWorktree(t, repo, "old", wt)
	rp := regPath(t)
	reg := config.Registry{Slices: map[string]config.RegistrySlice{
		"old": {
			Name:   "old",
			Source: config.SourceCreated,
			Members: map[string]config.RegistryMember{
				"web": {Branch: "old", WorktreePath: wt},
			},
		},
	}}
	if err := config.SaveRegistry(rp, reg); err != nil {
		t.Fatal(err)
	}
	// Simulate the historical bug: the checkout directory was deleted directly,
	// leaving both Git administrative metadata and the Slis registry behind.
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}

	ws := wsFor(map[string]string{"web": repo})
	ws.Root = root
	res := discovery.Report(ws, rp)
	if len(res.Missing) != 0 || len(res.Slices) != 0 {
		t.Fatalf("stale managed state should self-heal, got %+v", res)
	}
	got, _, err := config.LoadRegistry(rp)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := got.Slices["old"]; exists {
		t.Fatalf("stale registry entry survived repair: %+v", got.Slices["old"])
	}
	for _, listed := range mustListWorktrees(t, repo) {
		if resolvePath(t, listed.Path) == resolvePath(t, wt) {
			t.Fatalf("stale Git worktree metadata survived repair: %+v", listed)
		}
	}
}

func TestReportPreservesMissingExternalImportedWorktree(t *testing.T) {
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	missing := filepath.Join(t.TempDir(), "external-gone")
	testutil.AddWorktree(t, repo, "imported", missing)
	rp := regPath(t)
	reg := config.Registry{Slices: map[string]config.RegistrySlice{
		"imported": {
			Name:   "imported",
			Source: config.SourceImported,
			Members: map[string]config.RegistryMember{
				"web": {Branch: "imported", WorktreePath: missing},
			},
		},
	}}
	if err := config.SaveRegistry(rp, reg); err != nil {
		t.Fatal(err)
	}
	// Simulate a temporarily unavailable external checkout. Automatic repair
	// must preserve both its Slis identity and its Git administrative record.
	if err := os.RemoveAll(missing); err != nil {
		t.Fatal(err)
	}

	ws := wsFor(map[string]string{"web": repo})
	ws.Root = root
	res := discovery.Report(ws, rp)
	if len(res.Missing) != 1 || res.Missing[0].Slice != "imported" {
		t.Fatalf("external missing worktree must remain recoverable, got %+v", res.Missing)
	}
	found := false
	for _, listed := range mustListWorktrees(t, repo) {
		if listed.Branch == "imported" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("automatic repair removed external Git worktree metadata")
	}
}

func mustListWorktrees(t *testing.T, repo string) []git.Worktree {
	t.Helper()
	wts, err := git.ListWorktrees(repo)
	if err != nil {
		t.Fatal(err)
	}
	return wts
}

// An ignore glob from config must hide a matching worktree entirely — neither
// slice nor candidate — and count it as an "ignored" skip.
func TestReport_ConfigIgnoreGlobHonored(t *testing.T) {
	repo := testutil.NewRepo(t)
	scratch := filepath.Join(t.TempDir(), "scratch", "wt")
	if err := os.MkdirAll(filepath.Dir(scratch), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.AddWorktree(t, repo, "jonny/scratch", scratch)

	rp := regPath(t)
	writeEmptyRegistry(t, rp)
	ws := wsFor(map[string]string{"web": repo})
	ws.Grouping.Ignore = []string{"**/scratch/**"}

	res := discovery.Report(ws, rp)
	if len(res.Slices) != 0 {
		t.Fatalf("ignored worktree must not be a slice, got %+v", res.Slices)
	}
	if _, ok := candidateFor(t, res.Candidates, scratch); ok {
		t.Fatalf("ignored worktree must not be a candidate, got %+v", res.Candidates)
	}
	if !hasReason(res.Skipped, discovery.ReasonIgnored) {
		t.Fatalf("expected an ignored skip, got %+v", res.Skipped)
	}
}

// Invariant (c): once the registry exists (post-grandfather), a NEW unregistered
// worktree under .claude/worktrees is ignored — the agent-sandbox case.
func TestReport_DefaultClaudeWorktreesIgnoredWhenUnregistered(t *testing.T) {
	repo := testutil.NewRepo(t)
	sandbox := filepath.Join(t.TempDir(), ".claude", "worktrees", "agent-x")
	if err := os.MkdirAll(filepath.Dir(sandbox), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.AddWorktree(t, repo, "jonny/agentwork", sandbox)

	rp := regPath(t)
	writeEmptyRegistry(t, rp) // registry exists → NOT first run; ignore applies

	res := discovery.Report(wsFor(map[string]string{"web": repo}), rp)
	if len(res.Slices) != 0 {
		t.Fatalf("unregistered .claude/worktrees sandbox must be ignored, got slices %+v", res.Slices)
	}
	if _, ok := candidateFor(t, res.Candidates, sandbox); ok {
		t.Fatalf("ignored sandbox must not be a candidate, got %+v", res.Candidates)
	}
	if !hasReason(res.Skipped, discovery.ReasonIgnored) {
		t.Fatalf("expected an ignored skip for the sandbox, got %+v", res.Skipped)
	}
}

// Invariant (a): a registered slice whose worktree path matches an ignore glob
// must stay MANAGED — ignore never hides registered work. This is the exact
// upgrade regression: real feature worktrees under .claude/worktrees vanished.
func TestReport_RegisteredBeatsIgnoreGlob(t *testing.T) {
	repo := testutil.NewRepo(t)
	// A real feature worktree the user created under .claude/worktrees.
	wt := filepath.Join(t.TempDir(), ".claude", "worktrees", "pay-119")
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.AddWorktree(t, repo, "jonny/pay-119", wt)

	rp := regPath(t)
	reg := config.Registry{Slices: map[string]config.RegistrySlice{
		"pay-119": {
			Name:    "pay-119",
			Source:  config.SourceImported,
			Members: map[string]config.RegistryMember{"web": {Branch: "jonny/pay-119", WorktreePath: wt}},
		},
	}}
	if err := config.SaveRegistry(rp, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	res := discovery.Report(wsFor(map[string]string{"web": repo}), rp)
	if len(res.Slices) != 1 || res.Slices[0].Name != "pay-119" {
		t.Fatalf("registered worktree under an ignore glob must stay a slice, got %+v", res.Slices)
	}
	if hasReason(res.Skipped, discovery.ReasonIgnored) {
		t.Fatalf("registered worktree must NOT be ignored, got %+v", res.Skipped)
	}
}

// Invariant (b): a fresh grandfather run (no registry) with a worktree matching
// DefaultIgnoreGlobs must register it and keep it visible — the whole point of
// zero-behavior-change on upgrade. A second run must keep it visible too.
func TestReport_GrandfatherRegistersIgnoredWorktree(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), ".claude", "worktrees", "pay-45-ssp")
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.AddWorktree(t, repo, "jonny/pay-45-ssp", wt)

	rp := regPath(t) // does NOT exist → first run grandfathers everything, pre-ignore
	ws := wsFor(map[string]string{"web": repo})

	res := discovery.Report(ws, rp)
	if len(res.Slices) != 1 || res.Slices[0].Name != "pay-45-ssp" {
		t.Fatalf("grandfathering must keep a .claude/worktrees slice visible, got %+v", res.Slices)
	}
	reg, exists, _ := config.LoadRegistry(rp)
	if !exists {
		t.Fatalf("registry must be written on first run")
	}
	if _, ok := reg.Slices["pay-45-ssp"]; !ok {
		t.Fatalf("grandfathering must register the ignored-path worktree, got %+v", reg.Slices)
	}

	// Second run: still visible (registry precedence), not ignored.
	res2 := discovery.Report(ws, rp)
	if len(res2.Slices) != 1 || res2.Slices[0].Name != "pay-45-ssp" {
		t.Fatalf("registered slice must stay visible on later runs, got %+v", res2.Slices)
	}
	if hasReason(res2.Skipped, discovery.ReasonIgnored) {
		t.Fatalf("registered slice must not be ignored on later runs, got %+v", res2.Skipped)
	}
}

// The first run (no registry file) must grandfather all discovered slices,
// writing the registry exactly once; a second run must be idempotent.
func TestReport_GrandfathersOnceIdempotent(t *testing.T) {
	repo := testutil.NewRepo(t)
	wt := filepath.Join(t.TempDir(), "legacy")
	testutil.AddWorktree(t, repo, "jonny/legacy", wt)

	rp := regPath(t)
	ws := wsFor(map[string]string{"web": repo})

	if _, err := os.Stat(rp); !os.IsNotExist(err) {
		t.Fatalf("registry should not exist yet")
	}

	res := discovery.Report(ws, rp)
	if len(res.Slices) != 1 || res.Slices[0].Name != "legacy" {
		t.Fatalf("grandfathering must keep the existing slice, got %+v", res.Slices)
	}
	reg, exists, _ := config.LoadRegistry(rp)
	if !exists {
		t.Fatalf("registry must have been written on first run")
	}
	got, ok := reg.Slices["legacy"]
	if !ok || got.Source != config.SourceGrandfathered {
		t.Fatalf("expected grandfathered 'legacy' entry, got %+v", reg.Slices)
	}
	before, _ := os.ReadFile(rp)

	// Second run: still a slice, registry not rewritten.
	res2 := discovery.Report(ws, rp)
	if len(res2.Slices) != 1 || res2.Slices[0].Name != "legacy" {
		t.Fatalf("second run must still show the slice, got %+v", res2.Slices)
	}
	after, _ := os.ReadFile(rp)
	if string(before) != string(after) {
		t.Fatalf("registry must not be rewritten on subsequent runs")
	}
}

// A registered slice whose worktree directory no longer exists must surface in
// Missing (not silently vanish).
func TestReport_MissingWhenWorktreeDeleted(t *testing.T) {
	repo := testutil.NewRepo(t)

	// The worktree was created then removed: its directory is gone, but the
	// registry still records it. (A gone path, rather than a live-then-deleted
	// git worktree, keeps this deterministic — deleting a worktree dir out from
	// under git triggers a known macOS TempDir cleanup race.)
	wt := filepath.Join(t.TempDir(), "gone")

	rp := regPath(t)
	reg := config.Registry{Slices: map[string]config.RegistrySlice{
		"gone": {
			Name:    "gone",
			Source:  config.SourceImported,
			Members: map[string]config.RegistryMember{"web": {Branch: "jonny/gone", WorktreePath: wt}},
		},
	}}
	if err := config.SaveRegistry(rp, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	res := discovery.Report(wsFor(map[string]string{"web": repo}), rp)
	if len(res.Missing) != 1 {
		t.Fatalf("expected 1 missing member, got %+v", res.Missing)
	}
	m := res.Missing[0]
	if m.Slice != "gone" || m.Repo != "web" || m.Branch != "jonny/gone" {
		t.Fatalf("missing member fields wrong: %+v", m)
	}
}
