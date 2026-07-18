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

func TestSessionsAgentListDefault(t *testing.T) {
	cases := []struct {
		name     string
		s        Sessions
		wantName string
		wantCmd  []string
	}{
		{"empty → claude", Sessions{}, "claude", []string{"claude"}},
		{"codex harness", Sessions{Harness: "codex"}, "codex", []string{"codex"}},
		{"explicit agent with args", Sessions{Agent: "claude --resume"}, "claude", []string{"claude", "--resume"}},
	}
	for _, c := range cases {
		got := c.s.AgentList()
		if len(got) != 1 {
			t.Fatalf("%s: want 1 default agent, got %d", c.name, len(got))
		}
		if got[0].Name != c.wantName {
			t.Errorf("%s: name = %q, want %q", c.name, got[0].Name, c.wantName)
		}
		if len(got[0].Cmd) != len(c.wantCmd) {
			t.Fatalf("%s: cmd = %v, want %v", c.name, got[0].Cmd, c.wantCmd)
		}
		for i := range c.wantCmd {
			if got[0].Cmd[i] != c.wantCmd[i] {
				t.Errorf("%s: cmd[%d] = %q, want %q", c.name, i, got[0].Cmd[i], c.wantCmd[i])
			}
		}
	}
}

func TestSessionsAgentListConfigured(t *testing.T) {
	s := Sessions{Agents: []AgentSpec{
		{Name: "claude", Cmd: []string{"claude"}},
		{Name: "codex", Cmd: []string{"codex", "--full-auto"}},
	}}
	got := s.AgentList()
	if len(got) != 2 {
		t.Fatalf("want 2 agents, got %d", len(got))
	}
	if got[1].Name != "codex" || got[1].Cmd[1] != "--full-auto" {
		t.Errorf("second agent = %+v", got[1])
	}
}

func TestLoadWorkspaceParsesAgents(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "workspace.yaml")
	if err := os.WriteFile(p, []byte(`
root: ~/code
repos:
  web: { primary: ~/code/web, default_branch: main }
sessions:
  default_agent: codex
  agents:
    - name: claude
      cmd: [claude]
    - name: codex
      cmd: [codex, --full-auto]
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := LoadWorkspace(p)
	if err != nil {
		t.Fatal(err)
	}
	agents := ws.Sessions.AgentList()
	if len(agents) != 2 {
		t.Fatalf("want 2 agents, got %d", len(agents))
	}
	if agents[0].Name != "claude" || agents[1].Name != "codex" {
		t.Errorf("agent names = %q, %q", agents[0].Name, agents[1].Name)
	}
	if ws.Sessions.DefaultAgent != "codex" {
		t.Errorf("default agent = %q, want codex", ws.Sessions.DefaultAgent)
	}
	if len(agents[1].Cmd) != 2 || agents[1].Cmd[1] != "--full-auto" {
		t.Errorf("codex cmd = %v", agents[1].Cmd)
	}
}

func TestLoadWorkspaceRejectsInvalidAgents(t *testing.T) {
	cases := map[string]string{
		"empty name": `
root: ~/code
repos:
  web: { primary: ~/code/web }
sessions:
  agents:
    - name: ""
      cmd: [claude]
`,
		"empty cmd": `
root: ~/code
repos:
  web: { primary: ~/code/web }
sessions:
  agents:
    - name: claude
      cmd: []
`,
		"blank cmd arg": `
root: ~/code
repos:
  web: { primary: ~/code/web }
sessions:
  agents:
    - name: claude
      cmd: ["claude", ""]
`,
	}
	for name, yaml := range cases {
		dir := t.TempDir()
		p := filepath.Join(dir, "workspace.yaml")
		if err := os.WriteFile(p, []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadWorkspace(p); err == nil {
			t.Errorf("%s: expected error, got nil", name)
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
