package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWorkspace(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "workspace.yaml")
	if err := os.WriteFile(p, []byte(`
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
`), 0o644); err != nil {
		t.Fatal(err)
	}

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
	if err := os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { primary: ~/code/web, default_branch: main }
`), 0o644); err != nil {
		t.Fatal(err)
	}

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
	if err := os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { default_branch: main }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadWorkspace(p)
	if err == nil {
		t.Error("expected error for empty primary, got nil")
	}
}

func TestSessionsHarnessName(t *testing.T) {
	if got := (Sessions{}).HarnessName(); got != "claude" {
		t.Errorf("empty harness = %q, want claude", got)
	}
	if got := (Sessions{Harness: "codex"}).HarnessName(); got != "codex" {
		t.Errorf("harness = %q, want codex", got)
	}
}

func TestSessionsAgentCommand(t *testing.T) {
	cases := []struct {
		name string
		s    Sessions
		want string
	}{
		{"default claude", Sessions{}, "claude"},
		{"codex harness picks codex", Sessions{Harness: "codex"}, "codex"},
		{"explicit agent wins over claude harness", Sessions{Harness: "claude", Agent: "claude --resume"}, "claude --resume"},
		{"explicit agent wins over codex harness", Sessions{Harness: "codex", Agent: "aider"}, "aider"},
	}
	for _, c := range cases {
		if got := c.s.AgentCommand(); got != c.want {
			t.Errorf("%s: AgentCommand() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAutostartLegacyAlias(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "workspace.yaml")
	if err := os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { primary: ~/code/web, default_branch: main }
sessions:
  autostart_claude: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := LoadWorkspace(p)
	if err != nil {
		t.Fatal(err)
	}
	if !ws.Sessions.Autostart {
		t.Error("legacy autostart_claude: true should set Autostart = true")
	}
}

func TestAutostartExplicit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "workspace.yaml")
	if err := os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { primary: ~/code/web, default_branch: main }
sessions:
  harness: codex
  autostart: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := LoadWorkspace(p)
	if err != nil {
		t.Fatal(err)
	}
	if !ws.Sessions.Autostart {
		t.Error("autostart: true should set Autostart")
	}
	if ws.Sessions.HarnessName() != "codex" {
		t.Errorf("harness = %q, want codex", ws.Sessions.HarnessName())
	}
}
