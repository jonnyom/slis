package zmxctl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestClientSendPassesExactBytesAsOneArgument(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "zmx")
	argumentCountPath := filepath.Join(directory, "argument-count")
	payloadPath := filepath.Join(directory, "payload")
	stdinPath := filepath.Join(directory, "stdin")
	script := "#!/bin/sh\nprintf '%s' \"$#\" > \"$ZMX_TEST_ARGUMENT_COUNT\"\nprintf '%s' \"$3\" > \"$ZMX_TEST_PAYLOAD\"\ncat > \"$ZMX_TEST_STDIN\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZMX_TEST_ARGUMENT_COUNT", argumentCountPath)
	t.Setenv("ZMX_TEST_PAYLOAD", payloadPath)
	t.Setenv("ZMX_TEST_STDIN", stdinPath)

	client := New(binary, filepath.Join(directory, "runtime"))
	input := []byte("first line\nsecond line\r")
	if err := client.Send(context.Background(), "group-root", input); err != nil {
		t.Fatal(err)
	}

	argumentCount, err := os.ReadFile(argumentCountPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(argumentCount), "3"; got != want {
		t.Fatalf("argument count = %q, want %q", got, want)
	}
	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(payload), string(input); got != want {
		t.Fatalf("payload = %q, want %q", got, want)
	}
	stdin, err := os.ReadFile(stdinPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(stdin) != 0 {
		t.Fatalf("stdin = %q, want empty", stdin)
	}
}

func TestClientStartShellCommandStagesFullCommandAndSendsShortLauncher(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "zmx")
	launcherPath := filepath.Join(directory, "launcher")
	script := "#!/bin/sh\nprintf '%s' \"$3\" > \"$ZMX_TEST_LAUNCHER\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZMX_TEST_LAUNCHER", launcherPath)

	client := New(binary, filepath.Join(directory, "runtime"))
	statePathBeforeLaunch := client.shellCommandStatePath("group-root")
	if err := os.MkdirAll(filepath.Dir(statePathBeforeLaunch), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePathBeforeLaunch, []byte("done\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := strings.Repeat("x", 16*1024) + "SLIS_LONG_INPUT_END"
	statePath, err := client.StartShellCommand(context.Background(), "group-root", command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old command state still exists: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(statePath) })
	launcher, err := os.ReadFile(launcherPath)
	if err != nil {
		t.Fatal(err)
	}
	launcherText := string(launcher)
	const prefix = "'/bin/zsh' '"
	const suffix = "'\r"
	if !strings.HasPrefix(launcherText, prefix) || !strings.HasSuffix(launcherText, suffix) {
		t.Fatalf("launcher = %q", launcherText)
	}
	stagedPath := strings.TrimSuffix(strings.TrimPrefix(launcherText, prefix), suffix)
	t.Cleanup(func() { _ = os.Remove(stagedPath) })
	stagedCommand, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stagedCommand), command) || !strings.Contains(string(stagedCommand), statePath) {
		t.Fatalf("staged command length = %d", len(stagedCommand))
	}
}

func TestClientShellCommandRunningChecksRecordedProcess(t *testing.T) {
	directory := t.TempDir()
	client := New("", filepath.Join(directory, "runtime"))
	statePath := client.shellCommandStatePath("group-root")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("started "+strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	running, err := client.ShellCommandRunning("group-root")
	if err != nil {
		t.Fatal(err)
	}
	if !running {
		t.Fatal("recorded process is not running")
	}
	if err := os.WriteFile(statePath, []byte("done\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	running, err = client.ShellCommandRunning("group-root")
	if err != nil {
		t.Fatal(err)
	}
	if running {
		t.Fatal("completed command is running")
	}
}

func TestClientEnsureStartsCleanLoginShellInRequestedDirectory(t *testing.T) {
	directory := t.TempDir()
	binary := filepath.Join(directory, "zmx")
	argsPath := filepath.Join(directory, "args")
	cwdPath := filepath.Join(directory, "cwd")
	runtimePath := filepath.Join(directory, "runtime-value")
	stdinPath := filepath.Join(directory, "stdin")
	workingDirectory := filepath.Join(directory, "worktree")
	if err := os.Mkdir(workingDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" >> \"$ZMX_TEST_ARGS\"\nif [ \"$1\" = \"attach\" ]; then pwd > \"$ZMX_TEST_CWD\"; cat > \"$ZMX_TEST_STDIN\"; fi\nprintf '%s' \"$ZMX_DIR\" > \"$ZMX_TEST_RUNTIME\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ZMX_TEST_ARGS", argsPath)
	t.Setenv("ZMX_TEST_CWD", cwdPath)
	t.Setenv("ZMX_TEST_RUNTIME", runtimePath)
	t.Setenv("ZMX_TEST_STDIN", stdinPath)
	t.Setenv("SHELL", "/bin/zsh")

	runtimeDirectory, err := os.MkdirTemp("/tmp", "slis-zmx-client-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(runtimeDirectory) })
	client := New(binary, runtimeDirectory)
	if err := client.Ensure(context.Background(), "group-root", workingDirectory); err != nil {
		t.Fatal(err)
	}
	resolvedWorkingDirectory, err := filepath.EvalSymlinks(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}

	assertFileContents(t, argsPath, "attach\ngroup-root\n")
	assertFileContents(t, cwdPath, resolvedWorkingDirectory+"\n")
	assertFileContents(t, runtimePath, runtimeDirectory)
	assertFileContents(t, stdinPath, "\x1c")
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(contents); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func TestClientEnsureRejectsSocketPathTooLong(t *testing.T) {
	client := New("unused", "/tmp/"+strings.Repeat("runtime", 12))
	err := client.Ensure(context.Background(), strings.Repeat("session", 12), t.TempDir())
	if !errors.Is(err, ErrSocketPathTooLong) {
		t.Fatalf("error = %v, want ErrSocketPathTooLong", err)
	}
}

func TestClientAttachKeepsTerminalForegroundProcessGroup(t *testing.T) {
	client := New("zmx", "/tmp/slis-session-test")
	command := client.AttachCommand(context.Background(), "group-root")
	if command.SysProcAttr != nil {
		t.Fatalf("attach process attributes = %#v, want inherited foreground process group", command.SysProcAttr)
	}
}
