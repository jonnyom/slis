package editor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// stubLookPath makes only the named bins resolvable on PATH for the test.
func stubLookPath(t *testing.T, found ...string) {
	t.Helper()
	set := make(map[string]bool, len(found))
	for _, f := range found {
		set[f] = true
	}
	orig := lookPath
	lookPath = func(bin string) (string, error) {
		if set[filepath.Base(bin)] {
			return "/usr/bin/" + filepath.Base(bin), nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { lookPath = orig })
}

func TestAvailablePreferenceOrder(t *testing.T) {
	stubLookPath(t, "zed", "code") // intentionally out of preference order
	got := Available()
	if len(got) != 2 || got[0].Bin != "code" || got[1].Bin != "zed" {
		t.Fatalf("Available() = %+v, want [code, zed] in that order", got)
	}
	if got[0].Mode != ModeWorkspace || got[1].Mode != ModeMultiDir {
		t.Errorf("modes wrong: %+v", got)
	}
}

func TestResolve(t *testing.T) {
	stubLookPath(t, "cursor", "code", "vim")

	// Configured known editor wins, with its known Mode.
	if e, err := Resolve("code"); err != nil || e.Bin != "code" || e.Mode != ModeWorkspace {
		t.Errorf("Resolve(code) = %+v, %v", e, err)
	}
	// Configured unknown-but-present editor → SingleDir.
	if e, err := Resolve("vim"); err != nil || e.Bin != "vim" || e.Mode != ModeSingleDir {
		t.Errorf("Resolve(vim) = %+v, %v", e, err)
	}
	// Configured editor not on PATH → error.
	if _, err := Resolve("nano"); err == nil {
		t.Error("Resolve(nano) want error (not on PATH), got nil")
	}
	// Empty config → first available (cursor, by preference).
	if e, err := Resolve(""); err != nil || e.Bin != "cursor" {
		t.Errorf("Resolve(\"\") = %+v, %v, want cursor", e, err)
	}
}

func TestResolveNoneAvailable(t *testing.T) {
	stubLookPath(t) // nothing found
	if _, err := Resolve(""); err == nil {
		t.Error("Resolve(\"\") with no editors want error, got nil")
	}
}

func TestWriteWorkspaceFile(t *testing.T) {
	dir := t.TempDir()
	wts := []string{
		filepath.Join(dir, "WFM-4123", "Web-App"),
		filepath.Join(dir, "WFM-4123", "nory"),
	}
	path, err := WriteWorkspaceFile(dir, "feat/sub", wts)
	if err != nil {
		t.Fatalf("WriteWorkspaceFile: %v", err)
	}
	if filepath.Base(path) != "feat-sub.code-workspace" {
		t.Errorf("filename = %q, want feat-sub.code-workspace (slash sanitised)", filepath.Base(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ws codeWorkspace
	if err := json.Unmarshal(data, &ws); err != nil {
		t.Fatalf("workspace file is not valid JSON: %v\n%s", err, data)
	}
	if len(ws.Folders) != 2 {
		t.Fatalf("folders = %d, want 2", len(ws.Folders))
	}
	if ws.Folders[0].Path != wts[0] || ws.Folders[0].Name != "Web-App" {
		t.Errorf("folder[0] = %+v, want path=%q name=Web-App", ws.Folders[0], wts[0])
	}
}

func TestSliceArgs(t *testing.T) {
	wsDir := t.TempDir()
	wts := []string{"/p/s/Web-App", "/p/s/nory"}

	// Workspace mode writes a file and returns it.
	args, err := SliceArgs(Editor{Bin: "code", Mode: ModeWorkspace}, "s", wts, wsDir)
	if err != nil || len(args) != 1 || filepath.Ext(args[0]) != ".code-workspace" {
		t.Fatalf("workspace SliceArgs = %v, %v", args, err)
	}
	if _, err := os.Stat(args[0]); err != nil {
		t.Errorf("workspace file not written: %v", err)
	}

	// MultiDir returns the worktrees verbatim.
	args, _ = SliceArgs(Editor{Bin: "zed", Mode: ModeMultiDir}, "s", wts, wsDir)
	if len(args) != 2 || args[0] != wts[0] || args[1] != wts[1] {
		t.Errorf("multidir SliceArgs = %v, want %v", args, wts)
	}

	// SingleDir returns the common parent when there is one.
	args, _ = SliceArgs(Editor{Bin: "vim", Mode: ModeSingleDir}, "s", wts, wsDir)
	if len(args) != 1 || args[0] != "/p/s" {
		t.Errorf("singledir SliceArgs = %v, want [/p/s]", args)
	}
	// …and falls back to the first worktree when parents differ.
	args, _ = SliceArgs(Editor{Bin: "vim", Mode: ModeSingleDir}, "s", []string{"/a/x", "/b/y"}, wsDir)
	if len(args) != 1 || args[0] != "/a/x" {
		t.Errorf("singledir (no common parent) SliceArgs = %v, want [/a/x]", args)
	}
}
