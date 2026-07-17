package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/testutil"
)

// looseWorkspace makes a workspace with one repo whose worktree lives OUTSIDE
// the managed tree (so it is a candidate, not auto-ingested), plus the state
// dir holding the overrides/registry paths.
func looseWorkspace(t *testing.T) (ws config.Workspace, ovPath, wt string) {
	t.Helper()
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	wt = filepath.Join(t.TempDir(), "loose")
	testutil.AddWorktree(t, repo, "jonny/loose", wt)

	ws = config.Workspace{
		Root:     root,
		Repos:    map[string]config.Repo{"web": {Primary: repo, DefaultBranch: "main"}},
		Grouping: config.Grouping{Strategy: "branch-name", StripPrefix: "jonny/"},
	}
	ovPath = filepath.Join(t.TempDir(), "overrides.yaml")
	return ws, ovPath, wt
}

// A discovered-but-unmanaged worktree must surface as a candidate through the
// ls report, not as a slice. After importing it, it must become a slice and the
// candidate must disappear — persisting across a fresh report.
func TestListSlicesReport_CandidateThenImport(t *testing.T) {
	ws, ovPath, wt := looseWorkspace(t)
	jPath := filepath.Join(t.TempDir(), "none.json")
	regPath := registryPathFor(ovPath)

	// Registry exists (empty) → no grandfathering, so the loose worktree is a
	// candidate rather than being swept in.
	if err := config.SaveRegistry(regPath, config.Registry{}); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	res, err := listSlicesReport(ws, ovPath, jPath)
	if err != nil {
		t.Fatalf("listSlicesReport: %v", err)
	}
	if len(res.Slices) != 0 {
		t.Fatalf("candidate must not be a slice, got %+v", res.Slices)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Slice != "loose" {
		t.Fatalf("expected one 'loose' candidate, got %+v", res.Candidates)
	}

	// Import it.
	reg, _, _ := config.LoadRegistry(regPath)
	reg.Import("loose", "web", "jonny/loose", wt)
	if err := config.SaveRegistry(regPath, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	res2, err := listSlicesReport(ws, ovPath, jPath)
	if err != nil {
		t.Fatalf("listSlicesReport: %v", err)
	}
	if len(res2.Slices) != 1 || res2.Slices[0].Name != "loose" {
		t.Fatalf("imported worktree must be a slice, got %+v", res2.Slices)
	}
	if len(res2.Candidates) != 0 {
		t.Fatalf("imported worktree must not remain a candidate, got %+v", res2.Candidates)
	}
}

// A registered slice whose worktree path is gone must surface as missing through
// the ls report.
func TestListSlicesReport_MissingSurfaced(t *testing.T) {
	ws, ovPath, _ := looseWorkspace(t)
	jPath := filepath.Join(t.TempDir(), "none.json")
	regPath := registryPathFor(ovPath)

	reg := config.Registry{Slices: map[string]config.RegistrySlice{
		"ghost": {
			Name:    "ghost",
			Source:  config.SourceImported,
			Members: map[string]config.RegistryMember{"web": {Branch: "jonny/ghost", WorktreePath: filepath.Join(t.TempDir(), "nope")}},
		},
	}}
	if err := config.SaveRegistry(regPath, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	res, err := listSlicesReport(ws, ovPath, jPath)
	if err != nil {
		t.Fatalf("listSlicesReport: %v", err)
	}
	if len(res.Missing) != 1 || res.Missing[0].Slice != "ghost" {
		t.Fatalf("expected 'ghost' missing, got %+v", res.Missing)
	}
}

// forget must remove a slice from the registry without touching git.
func TestRegistryForget(t *testing.T) {
	regPath := filepath.Join(t.TempDir(), "registry.yaml")
	reg := config.Registry{Slices: map[string]config.RegistrySlice{
		"keep": {Name: "keep", Source: config.SourceImported},
		"drop": {Name: "drop", Source: config.SourceImported},
	}}
	if err := config.SaveRegistry(regPath, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	loaded, _, _ := config.LoadRegistry(regPath)
	delete(loaded.Slices, "drop")
	if err := config.SaveRegistry(regPath, loaded); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	after, _, _ := config.LoadRegistry(regPath)
	if _, ok := after.Slices["drop"]; ok {
		t.Fatalf("forgot slice 'drop' must be gone, got %+v", after.Slices)
	}
	if _, ok := after.Slices["keep"]; !ok {
		t.Fatalf("'keep' must remain, got %+v", after.Slices)
	}
}

// The built-in .claude/worktrees ignore must hide agent sandboxes even through
// the ls report, on a fresh (grandfathering) registry.
func TestListSlicesReport_DefaultIgnore(t *testing.T) {
	repo := testutil.NewRepo(t)
	root := t.TempDir()
	sandbox := filepath.Join(root, ".claude", "worktrees", "agent-y")
	if err := os.MkdirAll(filepath.Dir(sandbox), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.AddWorktree(t, repo, "jonny/sandbox", sandbox)

	ws := config.Workspace{
		Root:     root,
		Repos:    map[string]config.Repo{"web": {Primary: repo, DefaultBranch: "main"}},
		Grouping: config.Grouping{Strategy: "branch-name", StripPrefix: "jonny/"},
	}
	ovPath := filepath.Join(t.TempDir(), "overrides.yaml")
	jPath := filepath.Join(t.TempDir(), "none.json")

	res, err := listSlicesReport(ws, ovPath, jPath)
	if err != nil {
		t.Fatalf("listSlicesReport: %v", err)
	}
	if len(res.Slices) != 0 {
		t.Fatalf(".claude/worktrees sandbox must be ignored, got %+v", res.Slices)
	}
	for _, s := range res.Skipped {
		if s.Reason == discovery.ReasonIgnored {
			return
		}
	}
	t.Fatalf("expected an ignored skip, got %+v", res.Skipped)
}
