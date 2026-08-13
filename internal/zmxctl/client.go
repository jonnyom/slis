package zmxctl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/jonnyom/slis/internal/subproc"
)

var ErrSocketPathTooLong = errors.New("session socket path is too long")

type Client struct {
	binary     string
	runtimeDir string
}

func New(binary, runtimeDir string) *Client {
	return &Client{binary: binary, runtimeDir: runtimeDir}
}

func (client *Client) Ensure(ctx context.Context, name, directory string) error {
	if len(filepath.Join(client.runtimeDir, name)) >= 100 {
		return fmt.Errorf("%w: %s", ErrSocketPathTooLong, filepath.Join(client.runtimeDir, name))
	}
	cmd := client.command(ctx, EnsureArgv(name))
	cmd.Dir = directory
	cmd.Stdin = strings.NewReader("\x1c")
	if output, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf(
				"%w at %q. Reinstall Slis so slis, slis-ui, and zmx come from the same release. Homebrew: brew reinstall --formula jonnyom/tap/slis",
				ErrRuntimeMissing,
				client.binary,
			)
		}
		return fmt.Errorf("session runtime ensure %s: %s: %w", name, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (client *Client) StartLoginShell(ctx context.Context, name string) error {
	if os.Getenv("SHELL") == "" {
		return nil
	}
	if err := client.Send(ctx, name, []byte("exec \"$SHELL\" -l\r")); err != nil {
		return fmt.Errorf("session runtime start login shell %s: %w", name, err)
	}
	return nil
}

func (client *Client) List(ctx context.Context) ([]Session, error) {
	output, err := client.command(ctx, ListArgv()).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("session runtime list: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return ParseList(string(output))
}

func (client *Client) History(ctx context.Context, name string, vt bool) (string, error) {
	output, err := client.command(ctx, HistoryArgv(name, vt)).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("session runtime history %s: %s: %w", name, strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}

func (client *Client) AttachCommand(ctx context.Context, name string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, client.binary, AttachArgv(name)...)
	cmd.Env = runtimeEnvironment(client.runtimeDir)
	return cmd
}

func (client *Client) Send(ctx context.Context, name string, input []byte) error {
	cmd := client.command(ctx, append(SendArgv(name), string(input)))
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("session runtime send %s: %s: %w", name, strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (client *Client) StartShellCommand(ctx context.Context, name, command string) (string, error) {
	stagedFile, err := os.CreateTemp("/tmp", "slis-command-*.sh")
	if err != nil {
		return "", err
	}
	stagedPath := stagedFile.Name()
	statePath := client.shellCommandStatePath(name)
	if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
		_ = stagedFile.Close()
		_ = os.Remove(stagedPath)
		return "", err
	}
	stagedCommand := "rm -f -- " + shellSingleQuote(stagedPath) + "\nprintf 'started %s\\n' \"$$\" > " + shellSingleQuote(statePath) + "\n" + command + "\nprintf 'done\\n' >> " + shellSingleQuote(statePath) + "\n"
	if _, err := stagedFile.WriteString(stagedCommand); err != nil {
		_ = stagedFile.Close()
		_ = os.Remove(stagedPath)
		_ = os.Remove(statePath)
		return "", err
	}
	if err := stagedFile.Close(); err != nil {
		_ = os.Remove(stagedPath)
		_ = os.Remove(statePath)
		return "", err
	}
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(stagedPath)
		return "", err
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	launcher := shellSingleQuote(shell) + " " + shellSingleQuote(stagedPath)
	if err := client.Send(ctx, name, []byte(launcher+"\r")); err != nil {
		_ = os.Remove(stagedPath)
		_ = os.Remove(statePath)
		return "", err
	}
	return statePath, nil
}

func (client *Client) ShellCommandRunning(name string) (bool, error) {
	state, err := os.ReadFile(client.shellCommandStatePath(name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	fields := strings.Fields(string(state))
	if len(fields) != 2 || fields[0] != "started" {
		return false, nil
	}
	pid, err := strconv.Atoi(fields[1])
	if err != nil {
		return false, fmt.Errorf("session runtime command state %s: %w", name, err)
	}
	err = syscall.Kill(pid, 0)
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	return false, err
}

func (client *Client) shellCommandStatePath(name string) string {
	return filepath.Join(client.runtimeDir+"-slis-state", name)
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (client *Client) Kill(ctx context.Context, name string) error {
	output, err := client.command(ctx, KillArgv(name)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("session runtime kill %s: %s: %w", name, strings.TrimSpace(string(output)), err)
	}
	if err := os.Remove(client.shellCommandStatePath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_ = os.Remove(filepath.Dir(client.shellCommandStatePath(name)))
	return nil
}

func (client *Client) command(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, client.binary, args...)
	cmd.Env = runtimeEnvironment(client.runtimeDir)
	subproc.Configure(cmd)
	return cmd
}

func runtimeEnvironment(runtimeDir string) []string {
	environment := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "ZMX_DIR=") && !strings.HasPrefix(value, "SLIS_WORKSPACE_CONFIG=") {
			environment = append(environment, value)
		}
	}
	return append(environment, "ZMX_DIR="+runtimeDir)
}
