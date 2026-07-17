package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
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
