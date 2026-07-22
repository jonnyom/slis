package agentreview

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

func TestSchemaRequiresEveryObjectProperty(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(Schema), &schema); err != nil {
		t.Fatal(err)
	}
	comments := schema["properties"].(map[string]any)["comments"].(map[string]any)
	item := comments["items"].(map[string]any)
	required := make(map[string]bool)
	for _, name := range item["required"].([]any) {
		required[name.(string)] = true
	}
	for name := range item["properties"].(map[string]any) {
		if !required[name] {
			t.Errorf("property %q is not required", name)
		}
	}
}

func TestResolveAgentUsesConfiguredAndDetectedAgents(t *testing.T) {
	sessions := config.Sessions{Agents: []config.AgentSpec{{Name: "Fast Claude", Cmd: []string{"claude", "--model", "sonnet"}}}}
	lookPath := func(binary string) (string, error) {
		if binary == "codex" {
			return "/usr/local/bin/codex", nil
		}
		return "", errors.New("missing")
	}

	configured, err := ResolveAgent(sessions, "Fast Claude", lookPath)
	if err != nil || configured.Cmd[1] != "--model" {
		t.Fatalf("configured agent = %#v, err = %v", configured, err)
	}
	detected, err := ResolveAgent(sessions, "Codex", lookPath)
	if err != nil || detected.Cmd[0] != "/usr/local/bin/codex" {
		t.Fatalf("detected agent = %#v, err = %v", detected, err)
	}
	if _, err := ResolveAgent(sessions, "OpenCode", lookPath); err == nil {
		t.Fatal("expected unavailable detected agent to fail")
	}
}

func TestCommandForSupportedAgents(t *testing.T) {
	tests := []struct {
		name  string
		agent config.AgentSpec
		want  []string
	}{
		{name: "claude", agent: config.AgentSpec{Name: "Claude Code", Cmd: []string{"claude", "--model", "sonnet"}}, want: []string{"claude", "--model", "sonnet", "-p", "--permission-mode", "plan", "--output-format", "json", "--json-schema", Schema, "prompt"}},
		{name: "codex", agent: config.AgentSpec{Name: "Codex", Cmd: []string{"codex", "--full-auto"}}, want: []string{"codex", "exec", "--full-auto", "--sandbox", "read-only", "--skip-git-repo-check", "--ephemeral", "--output-schema", "schema.json", "--output-last-message", "result.json", "prompt"}},
		{name: "opencode", agent: config.AgentSpec{Name: "OpenCode", Cmd: []string{"opencode"}}, want: []string{"opencode", "run", "prompt"}},
		{name: "gemini", agent: config.AgentSpec{Name: "Gemini CLI", Cmd: []string{"gemini"}}, want: []string{"gemini", "-p", "prompt"}},
		{name: "cursor", agent: config.AgentSpec{Name: "Cursor Agent", Cmd: []string{"cursor-agent"}}, want: []string{"cursor-agent", "-p", "prompt"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Command(test.agent, "prompt", "schema.json", "result.json")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(got, "\x00") != strings.Join(test.want, "\x00") {
				t.Fatalf("command = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestCommandRejectsUnsupportedAgent(t *testing.T) {
	_, err := Command(config.AgentSpec{Name: "Custom", Cmd: []string{"custom-agent"}}, "prompt", "schema", "result")
	if err == nil || !strings.Contains(err.Error(), "unsupported reviewer agent") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseOutput(t *testing.T) {
	want := Finding{Repo: "api", File: "payroll/run.go", Line: 42, EndLine: 44, Side: "new", Body: "This can lose updates."}
	tests := []string{
		`{"comments":[{"repo":"api","file":"payroll/run.go","line":42,"end_line":44,"side":"new","body":"This can lose updates."}]}`,
		"review complete\n```json\n{\"comments\":[{\"repo\":\"api\",\"file\":\"payroll/run.go\",\"line\":42,\"end_line\":44,\"side\":\"new\",\"body\":\"This can lose updates.\"}]}\n```",
		`{"structured_output":{"comments":[{"repo":"api","file":"payroll/run.go","line":42,"end_line":44,"side":"new","body":"This can lose updates."}]}}`,
	}

	for _, raw := range tests {
		got, err := ParseOutput([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != want {
			t.Fatalf("findings = %#v, want %#v", got, want)
		}
	}
}

func TestValidateFindings(t *testing.T) {
	slice := model.Slice{Name: "feature", Members: map[string]model.SliceMember{
		"api": {Repo: "api", Branch: "feature", WorktreePath: "/tmp/api"},
	}}

	valid := []Finding{{Repo: "api", File: "run.go", Line: 3, Side: "new", Body: "Broken invariant."}}
	if err := ValidateFindings(slice, valid); err != nil {
		t.Fatal(err)
	}

	invalid := []Finding{{Repo: "web", File: "../secret", Line: 0, Side: "middle", Body: ""}}
	if err := ValidateFindings(slice, invalid); err == nil {
		t.Fatal("expected invalid findings to fail")
	}
}

func TestPromptIncludesEverySliceMember(t *testing.T) {
	slice := model.Slice{Name: "feature", Members: map[string]model.SliceMember{
		"api": {Repo: "api", Branch: "feature-api", WorktreePath: "/work/api"},
		"web": {Repo: "web", Branch: "feature-web", WorktreePath: "/work/web"},
	}}

	prompt := Prompt(slice)
	for _, expected := range []string{"feature", "api", "feature-api", "/work/api", "web", "feature-web", "/work/web"} {
		if !strings.Contains(prompt, expected) {
			t.Errorf("prompt does not contain %q", expected)
		}
	}
}

func TestRunParsesAndValidatesAgentOutput(t *testing.T) {
	slice := model.Slice{Name: "feature", Members: map[string]model.SliceMember{
		"api": {Repo: "api", Branch: "feature", WorktreePath: "/work/api"},
	}}
	agent := config.AgentSpec{Name: "Claude Code", Cmd: []string{"claude"}}
	execute := func(_ context.Context, cwd string, command []string) ([]byte, error) {
		if cwd != "/workspace" {
			t.Fatalf("cwd = %q", cwd)
		}
		if command[0] != "claude" {
			t.Fatalf("command = %#v", command)
		}
		return []byte(`{"comments":[{"repo":"api","file":"run.go","line":7,"side":"new","body":"Race on shared state."}]}`), nil
	}

	findings, err := Run(context.Background(), "/workspace", slice, agent, execute)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Body != "Race on shared state." {
		t.Fatalf("findings = %#v", findings)
	}
}
