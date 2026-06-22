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

	got := WorkspacePath()
	want := filepath.Join(tmp, "slis", "workspace.yaml")
	if got != want {
		t.Errorf("WorkspacePath() = %q, want %q", got, want)
	}
}
