package zmxctl

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureRuntimeDirectoryCreatesPrivateShortPath(t *testing.T) {
	directory, err := EnsureRuntimeDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if len(directory) > 32 {
		t.Fatalf("runtime directory length = %d, want at most 32: %q", len(directory), directory)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if permissions := info.Mode().Perm(); permissions != 0o700 {
		t.Fatalf("permissions = %o, want 700", permissions)
	}
}

func TestEnsureRuntimeDirectoryUsesSlisOverride(t *testing.T) {
	directory := filepath.Join("/tmp", "slis-runtime-test")
	t.Setenv("SLIS_SESSION_RUNTIME_DIR", directory)
	got, err := EnsureRuntimeDirectory()
	if err != nil {
		t.Fatal(err)
	}
	if got != directory {
		t.Fatalf("runtime directory = %q, want %q", got, directory)
	}
}
