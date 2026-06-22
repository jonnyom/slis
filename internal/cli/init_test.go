package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
)

// mustInitRepo creates dir and runs `git init -q` inside it.
// The test is skipped if git is not on PATH.
func mustInitRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mustInitRepo: MkdirAll %q: %v", dir, err)
	}
	cmd := exec.Command("git", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("mustInitRepo: git init in %q: %v\n%s", dir, err, out)
	}
}

// TestInitNonInteractive verifies the non-interactive path:
// Init(root, []string{"a","c"}) scans, keeps only a & c, saves workspace.yaml,
// and returns the path to it.
func TestInitNonInteractive(t *testing.T) {
	root := t.TempDir()
	mustInitRepo(t, filepath.Join(root, "a"))
	mustInitRepo(t, filepath.Join(root, "b"))
	mustInitRepo(t, filepath.Join(root, "c"))

	// Point XDG_CONFIG_HOME to a temp dir so we don't pollute real config.
	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	writtenPath, err := Init(root, []string{"a", "c"}, "")
	if err != nil {
		t.Fatal(err)
	}

	// The path returned should be under our temp config home.
	if writtenPath == "" {
		t.Fatal("Init returned empty path")
	}

	// Load back and verify
	ws, err := config.LoadWorkspace(writtenPath)
	if err != nil {
		t.Fatalf("LoadWorkspace(%q): %v", writtenPath, err)
	}

	if ws.Root != root {
		t.Errorf("Root = %q, want %q", ws.Root, root)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("len(Repos) = %d, want 2", len(ws.Repos))
	}
	if _, ok := ws.Repos["a"]; !ok {
		t.Error("Repos missing 'a'")
	}
	if _, ok := ws.Repos["c"]; !ok {
		t.Error("Repos missing 'c'")
	}
	if _, ok := ws.Repos["b"]; ok {
		t.Error("Repos should not contain 'b'")
	}
}

// TestInitWithSelection mirrors TestInitNonInteractive but via InitWithSelection.
func TestInitWithSelection(t *testing.T) {
	root := t.TempDir()
	mustInitRepo(t, filepath.Join(root, "x"))
	mustInitRepo(t, filepath.Join(root, "y"))
	mustInitRepo(t, filepath.Join(root, "z"))

	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	writtenPath, err := InitWithSelection(root, []string{"x", "z"}, "")
	if err != nil {
		t.Fatal(err)
	}

	ws, err := config.LoadWorkspace(writtenPath)
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}

	if ws.Root != root {
		t.Errorf("Root = %q, want %q", ws.Root, root)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("len(Repos) = %d, want 2", len(ws.Repos))
	}
	if _, ok := ws.Repos["x"]; !ok {
		t.Error("Repos missing 'x'")
	}
	if _, ok := ws.Repos["z"]; !ok {
		t.Error("Repos missing 'z'")
	}
}

// TestInitNonInteractiveNoMatchingRepos verifies an error is returned
// when selected names don't match any discovered repos.
func TestInitNonInteractiveNoMatchingRepos(t *testing.T) {
	root := t.TempDir()
	mustInitRepo(t, filepath.Join(root, "only-one"))

	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	_, err := Init(root, []string{"nonexistent"}, "")
	if err == nil {
		t.Fatal("expected error when no repos match selection, got nil")
	}
}

// TestInitWithSelectionPersistsGroupingDefaults verifies that after
// InitWithSelection, the written workspace.yaml contains the expected
// grouping defaults and cpu_warn_pct.
func TestInitWithSelectionPersistsGroupingDefaults(t *testing.T) {
	root := t.TempDir()
	mustInitRepo(t, filepath.Join(root, "a"))
	mustInitRepo(t, filepath.Join(root, "c"))

	cfgHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgHome)

	writtenPath, err := InitWithSelection(root, []string{"a", "c"}, "jonny/")
	if err != nil {
		t.Fatal(err)
	}

	ws, err := config.LoadWorkspace(writtenPath)
	if err != nil {
		t.Fatalf("LoadWorkspace(%q): %v", writtenPath, err)
	}

	if ws.Grouping.StripPrefix != "jonny/" {
		t.Errorf("Grouping.StripPrefix = %q, want %q", ws.Grouping.StripPrefix, "jonny/")
	}
	if ws.Grouping.Strategy != "branch-name" {
		t.Errorf("Grouping.Strategy = %q, want %q", ws.Grouping.Strategy, "branch-name")
	}
	if ws.Processes.CPUWarnPct != 150 {
		t.Errorf("Processes.CPUWarnPct = %d, want 150", ws.Processes.CPUWarnPct)
	}
}
