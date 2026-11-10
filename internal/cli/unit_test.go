package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
)

func tempStatePaths(t *testing.T) config.Paths {
	directory := t.TempDir()
	return config.Paths{
		Overrides: filepath.Join(directory, "overrides.yaml"),
		Registry:  filepath.Join(directory, "registry.yaml"),
	}
}

func TestJoinCallerUnitPutsSpunOffSliceInParentUnit(t *testing.T) {
	paths := tempStatePaths(t)

	t.Setenv("SLIS_SLICE", "wfm-mw-latency")
	joinCallerUnit(paths, "wfm-mw-lean")
	t.Setenv("SLIS_SLICE", "wfm-mw-lean")
	joinCallerUnit(paths, "wfm-mw-leaner")

	units, err := discovery.LoadUnits(paths.Overrides)
	if err != nil {
		t.Fatal(err)
	}
	for _, slice := range []string{"wfm-mw-latency", "wfm-mw-lean", "wfm-mw-leaner"} {
		if units[slice] != "wfm-mw-latency" {
			t.Fatalf("units[%q] = %q, want wfm-mw-latency (all: %v)", slice, units[slice], units)
		}
	}
}

func TestJoinCallerUnitFindsParentFromWorkingDirectory(t *testing.T) {
	paths := tempStatePaths(t)
	worktree := t.TempDir()
	registry := config.Registry{}
	registry.RegisterCreated("wfm-mw-latency", "Node-Middleware", "jonny/wfm-mw-latency", worktree)
	if err := config.SaveRegistry(paths.Registry, registry); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(worktree, "src")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SLIS_SLICE", "")
	t.Chdir(nested)
	joinCallerUnit(paths, "wfm-mw-lean")

	units, _ := discovery.LoadUnits(paths.Overrides)
	if units["wfm-mw-lean"] != "wfm-mw-latency" {
		t.Fatalf("units = %v, want wfm-mw-lean in wfm-mw-latency", units)
	}
}

func TestJoinCallerUnitOutsideSliceDoesNothing(t *testing.T) {
	paths := tempStatePaths(t)
	t.Setenv("SLIS_SLICE", "")
	t.Chdir(t.TempDir())
	joinCallerUnit(paths, "fresh")

	units, _ := discovery.LoadUnits(paths.Overrides)
	if len(units) != 0 {
		t.Fatalf("units = %v, want empty", units)
	}
}
