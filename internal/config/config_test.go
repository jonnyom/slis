package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkspace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "workspace.yaml")
	os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { primary: ~/code/web, default_branch: main }
  api: { primary: ~/code/api, default_branch: main }
grouping:
  strategy: branch-name
  strip_prefix: "jonny/"
sessions:
  autostart_claude: false
processes:
  cpu_warn_pct: 150
`), 0o644)

	ws, err := LoadWorkspace(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Repos) != 2 {
		t.Fatalf("want 2 repos, got %d", len(ws.Repos))
	}
	if ws.Grouping.StripPrefix != "jonny/" {
		t.Errorf("strip_prefix = %q", ws.Grouping.StripPrefix)
	}
	if ws.Processes.CPUWarnPct != 150 {
		t.Errorf("cpu_warn_pct = %v", ws.Processes.CPUWarnPct)
	}
	// "~" must expand
	if got := ws.Repos["web"].Primary; got == "~/code/web" {
		t.Errorf("primary not expanded: %q", got)
	}
}

func TestLoadWorkspaceDefaults(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "workspace.yaml")
	os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { primary: ~/code/web, default_branch: main }
`), 0o644)

	ws, err := LoadWorkspace(p)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Grouping.Strategy != "branch-name" {
		t.Errorf("default strategy = %q, want branch-name", ws.Grouping.Strategy)
	}
	if ws.Processes.CPUWarnPct != 150 {
		t.Errorf("default cpu_warn_pct = %v, want 150", ws.Processes.CPUWarnPct)
	}
}

func TestLoadWorkspaceEmptyPrimary(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "workspace.yaml")
	os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { default_branch: main }
`), 0o644)

	_, err := LoadWorkspace(p)
	if err == nil {
		t.Error("expected error for empty primary, got nil")
	}
}
