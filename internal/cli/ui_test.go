package cli

import (
	"errors"
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

func TestChooseDefaultUIForcesGoWhenEnvIsGo(t *testing.T) {
	launchJS, notice := chooseDefaultUI("go", nil)
	if launchJS {
		t.Error("launchJS = true, want false when SLIS_TUI=go")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty (explicit override is not a fallback)", notice)
	}
}

func TestChooseDefaultUIForcesGoEvenWhenJSResolves(t *testing.T) {
	launchJS, _ := chooseDefaultUI("go", nil)
	if launchJS {
		t.Error("SLIS_TUI=go must win even when the JS UI resolves")
	}
}

func TestChooseDefaultUIPrefersJSWhenResolvable(t *testing.T) {
	launchJS, notice := chooseDefaultUI("", nil)
	if !launchJS {
		t.Error("launchJS = false, want true when JS resolves and no override")
	}
	if notice != "" {
		t.Errorf("notice = %q, want empty on the happy path", notice)
	}
}

func TestChooseDefaultUIFallsBackToGoWhenJSUnresolvable(t *testing.T) {
	launchJS, notice := chooseDefaultUI("", errors.New("no slis-ui"))
	if launchJS {
		t.Error("launchJS = true, want false when the JS UI cannot be resolved")
	}
	if notice == "" {
		t.Error("expected a one-line stderr notice on the Go fallback")
	}
}
