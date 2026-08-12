package zmxctl

import (
	"reflect"
	"testing"
)

func TestCommandArgvUsesOnlySupportedZmxCommands(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "ensure", got: EnsureArgv("group-root"), want: []string{"attach", "group-root"}},
		{name: "attach", got: AttachArgv("group-root"), want: []string{"attach", "group-root"}},
		{name: "send", got: SendArgv("group-root"), want: []string{"send", "group-root"}},
		{name: "history", got: HistoryArgv("group-root", false), want: []string{"history", "group-root"}},
		{name: "vt history", got: HistoryArgv("group-root", true), want: []string{"history", "group-root", "--vt"}},
		{name: "list", got: ListArgv(), want: []string{"list"}},
		{name: "kill", got: KillArgv("group-root"), want: []string{"kill", "group-root"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !reflect.DeepEqual(test.got, test.want) {
				t.Fatalf("argv = %#v, want %#v", test.got, test.want)
			}
		})
	}
}
