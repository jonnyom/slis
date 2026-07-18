package cli

import (
	"path/filepath"
	"testing"
)

func TestResolveUILaunchPrefersSiblingCompiledBinary(t *testing.T) {
	binPath := "/opt/slis/bin/slis"
	sibling := "/opt/slis/bin/slis-ui"
	exists := func(p string) bool { return p == sibling }

	launch, err := resolveUILaunch(binPath, "/somewhere/tui-js", exists)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if launch.name != sibling {
		t.Errorf("name = %q, want %q", launch.name, sibling)
	}
	if len(launch.args) != 0 {
		t.Errorf("args = %v, want none", launch.args)
	}
	if launch.dir != "" {
		t.Errorf("dir = %q, want empty (sibling binary runs in place)", launch.dir)
	}
}

func TestResolveUILaunchFallsBackToBunDevMode(t *testing.T) {
	binPath := "/opt/slis/bin/slis"
	tuiDir := "/home/jonny/slis/tui-js"
	noSibling := func(string) bool { return false }

	launch, err := resolveUILaunch(binPath, tuiDir, noSibling)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if launch.name != "bun" {
		t.Errorf("name = %q, want bun", launch.name)
	}
	want := []string{"run", filepath.FromSlash("src/index.tsx")}
	if len(launch.args) != len(want) || launch.args[0] != want[0] || launch.args[1] != want[1] {
		t.Errorf("args = %v, want %v", launch.args, want)
	}
	if launch.dir != tuiDir {
		t.Errorf("dir = %q, want %q", launch.dir, tuiDir)
	}
}

func TestResolveUILaunchErrorsWhenNothingAvailable(t *testing.T) {
	binPath := "/opt/slis/bin/slis"
	noSibling := func(string) bool { return false }

	_, err := resolveUILaunch(binPath, "", noSibling)
	if err == nil {
		t.Fatal("expected an error when no compiled binary and no SLIS_TUI_DIR")
	}
}
