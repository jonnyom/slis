package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestEnvironmentWithValueReplacesExistingValue(t *testing.T) {
	environment := []string{"HOME=/home/jonny", "SLIS_WORKSPACE_CONFIG=/old"}
	got := environmentWithValue(environment, "SLIS_WORKSPACE_CONFIG", "/new")
	want := []string{"HOME=/home/jonny", "SLIS_WORKSPACE_CONFIG=/new"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("environment = %v, want %v", got, want)
	}
}

func TestRedirectStderrSendsFd2ToLogUntilRestored(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "state", "slis-ui.stderr.log")
	restore, err := redirectStderr(logPath)
	if err != nil {
		t.Fatalf("redirectStderr: %v", err)
	}
	_, writeErr := os.Stderr.WriteString("while-redirected\n")
	restore()
	if writeErr != nil {
		t.Fatalf("write stderr: %v", writeErr)
	}

	got, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(got) != "while-redirected\n" {
		t.Errorf("log = %q, want %q", got, "while-redirected\n")
	}
	stderrInfo, err := os.Stderr.Stat()
	if err != nil {
		t.Fatalf("stat stderr: %v", err)
	}
	logInfo, err := os.Stat(logPath)
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if os.SameFile(stderrInfo, logInfo) {
		t.Error("stderr still points at the log after restore")
	}
}

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
