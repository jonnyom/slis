package cli

import (
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/cleanup"
	"github.com/jonnyom/slis/internal/config"
)

func TestCleanupFullyRemoved(t *testing.T) {
	if cleanupFullyRemoved(cleanup.Report{}) {
		t.Fatal("empty cleanup report must not count as complete")
	}
	if cleanupFullyRemoved(cleanup.Report{Repos: []cleanup.RepoResult{
		{Repo: "web", WorktreeRemoved: true},
		{Repo: "api", WorktreeRemoved: false},
	}}) {
		t.Fatal("partial cleanup must not count as complete")
	}
	if !cleanupFullyRemoved(cleanup.Report{Repos: []cleanup.RepoResult{
		{Repo: "web", WorktreeRemoved: true},
		{Repo: "api", WorktreeRemoved: true},
	}}) {
		t.Fatal("all removed worktrees must count as complete")
	}
}

func TestClearSliceStateForgetsOnlyCompletedCleanup(t *testing.T) {
	stateDir := t.TempDir()
	sp := config.Paths{
		Registry:  filepath.Join(stateDir, "registry.yaml"),
		Overrides: filepath.Join(stateDir, "overrides.yaml"),
		EventsDir: filepath.Join(stateDir, "events"),
	}
	reg := config.Registry{Slices: map[string]config.RegistrySlice{
		"complete": {Name: "complete"},
		"partial":  {Name: "partial"},
		"keep":     {Name: "keep"},
	}}
	if err := config.SaveRegistry(sp.Registry, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	if err := clearSliceState(sp, "partial", false); err != nil {
		t.Fatalf("clear partial state: %v", err)
	}
	if err := clearSliceState(sp, "complete", true); err != nil {
		t.Fatalf("clear complete state: %v", err)
	}

	after, _, err := config.LoadRegistry(sp.Registry)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if _, ok := after.Slices["complete"]; ok {
		t.Fatal("completed slice must be removed from registry")
	}
	if _, ok := after.Slices["partial"]; !ok {
		t.Fatal("partially removed slice must stay registered")
	}
	if _, ok := after.Slices["keep"]; !ok {
		t.Fatal("unrelated slice must stay registered")
	}
}
