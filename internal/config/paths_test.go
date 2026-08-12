package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatePathsHonoursXDGStateHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(WorkspaceConfigEnv, legacyWorkspacePath())

	p := StatePaths()

	wantBase := filepath.Join(tmp, "slis")
	if p.StateDir != wantBase {
		t.Errorf("StateDir = %q, want %q", p.StateDir, wantBase)
	}
	if !strings.HasPrefix(p.Overrides, wantBase) {
		t.Errorf("Overrides = %q, want prefix %q", p.Overrides, wantBase)
	}
	if !strings.HasSuffix(p.Overrides, "overrides.yaml") {
		t.Errorf("Overrides = %q, want suffix overrides.yaml", p.Overrides)
	}
	if !strings.HasPrefix(p.ActiveJournal, wantBase) {
		t.Errorf("ActiveJournal = %q, want prefix %q", p.ActiveJournal, wantBase)
	}
	if !strings.HasSuffix(p.ActiveJournal, "active.json") {
		t.Errorf("ActiveJournal = %q, want suffix active.json", p.ActiveJournal)
	}
	if !strings.HasPrefix(p.EventsDir, wantBase) {
		t.Errorf("EventsDir = %q, want prefix %q", p.EventsDir, wantBase)
	}
	if !strings.HasSuffix(p.EventsDir, "events") {
		t.Errorf("EventsDir = %q, want suffix events", p.EventsDir)
	}
}

func TestStatePathsEnsureDirsCreates(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_STATE_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv(WorkspaceConfigEnv, legacyWorkspacePath())

	p := StatePaths()
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}

	for _, dir := range []string{p.StateDir, p.EventsDir} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("expected dir %q to exist, got: %v", dir, err)
		}
	}
}

func TestConfigDirHonoursXDGConfigHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got := ConfigDir()
	want := filepath.Join(tmp, "slis")
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestWorkspacePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	workspaceRoot := t.TempDir()
	t.Chdir(workspaceRoot)
	legacyPath := filepath.Join(tmp, "slis", "workspace.yaml")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPath, []byte("root: /another/workspace\nrepos: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := WorkspacePath()
	want := WorkspacePathForRoot(workspaceRoot)
	if got != want {
		t.Fatalf("WorkspacePath() = %q, want %q", got, want)
	}
}

func TestWorkspacePathSelectsWorkspaceContainingCurrentDirectory(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	secondChild := filepath.Join(secondRoot, "repo")
	if err := os.MkdirAll(secondChild, 0o755); err != nil {
		t.Fatal(err)
	}
	firstPath := WorkspacePathForRoot(firstRoot)
	secondPath := WorkspacePathForRoot(secondRoot)
	if err := SaveWorkspace(firstPath, Workspace{Root: firstRoot}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkspace(secondPath, Workspace{Root: secondRoot}); err != nil {
		t.Fatal(err)
	}

	t.Chdir(secondChild)
	if got := WorkspacePath(); got != secondPath {
		t.Fatalf("WorkspacePath() = %q, want %q", got, secondPath)
	}
}

func TestWorkspacePathSelectsDeepestContainingWorkspace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	parentRoot := t.TempDir()
	childRoot := filepath.Join(parentRoot, "child")
	workingDirectory := filepath.Join(childRoot, "repo")
	if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := WorkspacePathForRoot(parentRoot)
	childPath := WorkspacePathForRoot(childRoot)
	if err := SaveWorkspace(parentPath, Workspace{Root: parentRoot}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkspace(childPath, Workspace{Root: childRoot}); err != nil {
		t.Fatal(err)
	}

	t.Chdir(workingDirectory)
	if got := WorkspacePath(); got != childPath {
		t.Fatalf("WorkspacePath() = %q, want %q", got, childPath)
	}
}

func TestStatePathsSeparateWorkspaces(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	firstRoot := t.TempDir()
	secondRoot := t.TempDir()
	if err := SaveWorkspace(WorkspacePathForRoot(firstRoot), Workspace{Root: firstRoot}); err != nil {
		t.Fatal(err)
	}
	if err := SaveWorkspace(WorkspacePathForRoot(secondRoot), Workspace{Root: secondRoot}); err != nil {
		t.Fatal(err)
	}

	t.Chdir(firstRoot)
	firstState := StatePaths().StateDir
	t.Chdir(secondRoot)
	secondState := StatePaths().StateDir
	if firstState == secondState {
		t.Fatalf("workspace state directories are equal: %q", firstState)
	}
}

func TestWorkspacePathForRootPreservesMatchingLegacyWorkspace(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	legacyPath := legacyWorkspacePath()
	if err := SaveWorkspace(legacyPath, Workspace{Root: root}); err != nil {
		t.Fatal(err)
	}
	if got := WorkspacePathForRoot(root); got != legacyPath {
		t.Fatalf("WorkspacePathForRoot() = %q, want %q", got, legacyPath)
	}
}
