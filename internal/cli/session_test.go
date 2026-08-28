package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/proc"
	sessionmanager "github.com/jonnyom/slis/internal/session"
)

func TestSessionCommandOwnsFullTerminalLifecycle(t *testing.T) {
	commands := sessionCmd.Commands()
	names := make([]string, 0, len(commands))
	for _, command := range commands {
		if command.Hidden {
			continue
		}
		names = append(names, command.Name())
	}
	want := []string{"activate-tab", "attach", "busy", "ensure", "ensure-tab", "history", "kill", "kill-tab", "list", "send", "start"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("session commands = %#v, want %#v", names, want)
	}
}

func TestLegacySessionNamesLimitBridgeToSlisSessions(t *testing.T) {
	for _, name := range []string{"slis/feature", "slis-shell/feature"} {
		if !validLegacySessionName(name) {
			t.Fatalf("valid name rejected: %q", name)
		}
	}
	for _, name := range []string{"feature", "other/feature", "slis/feature\x00bad"} {
		if validLegacySessionName(name) {
			t.Fatalf("invalid name accepted: %q", strings.ReplaceAll(name, "\x00", "\\0"))
		}
	}
}

func TestSessionGroupRuntimeLabelsUseAgentThenLiveDirectory(t *testing.T) {
	t.Setenv("HOME", "/Users/jonny")
	group := sessionmanager.Group{
		ID: "feature",
		Tabs: []sessionmanager.Tab{
			{ID: "root", CWD: "/Users/jonny/nory"},
			{ID: "claude", CWD: "/Users/jonny/nory/api"},
		},
	}
	runtimes := []sessionmanager.TabRuntime{
		{GroupID: "feature", TabID: "root", Processes: []proc.ProcInfo{{Cmd: "/opt/codex", CWD: "/Users/jonny/nory/web-app"}}, Busy: true},
		{GroupID: "feature", TabID: "claude", Processes: []proc.ProcInfo{{Cmd: "/bin/zsh", CWD: "/Users/jonny/nory/api"}}},
	}

	output := sessionGroupForOutputWithRuntime(group, runtimes, []config.AgentSpec{{Name: "Codex", Cmd: []string{"codex"}}})
	if output.Tabs[0].Agent != "Codex" || output.Tabs[0].Label != "Codex" || !output.Tabs[0].Busy {
		t.Fatalf("root tab = %#v", output.Tabs[0])
	}
	if output.Tabs[1].Agent != "" || output.Tabs[1].Label != "~/nory/api" {
		t.Fatalf("idle tab = %#v", output.Tabs[1])
	}
}

func TestSessionGroupRuntimeLabelsNumberDuplicateAgents(t *testing.T) {
	group := sessionmanager.Group{ID: "feature", Tabs: []sessionmanager.Tab{{ID: "root"}, {ID: "agent-2"}}}
	runtimes := []sessionmanager.TabRuntime{
		{GroupID: "feature", TabID: "root", Processes: []proc.ProcInfo{{Cmd: "codex"}}},
		{GroupID: "feature", TabID: "agent-2", Processes: []proc.ProcInfo{{Cmd: "node /opt/codex/bin"}}},
	}

	output := sessionGroupForOutputWithRuntime(group, runtimes, []config.AgentSpec{{Name: "Codex", Cmd: []string{"codex"}}})
	if output.Tabs[0].Label != "Codex" || output.Tabs[1].Label != "Codex (1)" {
		t.Fatalf("labels = %q, %q", output.Tabs[0].Label, output.Tabs[1].Label)
	}
}
