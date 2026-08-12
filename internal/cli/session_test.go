package cli

import (
	"reflect"
	"strings"
	"testing"
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
