package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/jonnyom/slis/internal/hooks"
	"github.com/spf13/cobra"
)

type uiLaunch struct {
	name string
	args []string
	dir  string
}

func resolveUILaunch(binPath, tuiDir string, fileExists func(string) bool) (uiLaunch, error) {
	siblingUI := filepath.Join(filepath.Dir(binPath), "slis-ui")
	if fileExists(siblingUI) {
		return uiLaunch{name: siblingUI}, nil
	}
	if tuiDir == "" {
		return uiLaunch{}, fmt.Errorf(
			"no compiled slis-ui next to %s and SLIS_TUI_DIR not set — "+
				"install slis-ui alongside slis, or set SLIS_TUI_DIR to the tui-js source dir for dev mode",
			binPath)
	}
	return uiLaunch{name: "bun", args: []string{"run", "src/index.tsx"}, dir: tuiDir}, nil
}

func regularFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func chooseDefaultUI(slisTUIEnv string, resolveErr error) (launchJS bool, notice string) {
	if slisTUIEnv == "go" {
		return false, ""
	}
	if resolveErr != nil {
		return false, "slis: JS UI unavailable; launching the Go TUI instead " +
			"(set SLIS_TUI=go to skip this check, or install slis-ui / set SLIS_TUI_DIR for the JS front-end)"
	}
	return true, ""
}

func execJSUI(binPath string, launch uiLaunch) error {
	argv0, err := exec.LookPath(launch.name)
	if err != nil {
		return fmt.Errorf("cannot find %q on PATH: %w", launch.name, err)
	}

	prevDir := ""
	if launch.dir != "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("cannot determine the current directory: %w", err)
		}
		prevDir = wd
		if err := os.Chdir(launch.dir); err != nil {
			return fmt.Errorf("cannot enter %s: %w", launch.dir, err)
		}
	}

	env := os.Environ()
	if os.Getenv("SLIS_BIN") == "" {
		env = append(env, "SLIS_BIN="+binPath)
	}

	execErr := syscall.Exec(argv0, append([]string{launch.name}, launch.args...), env)
	if prevDir != "" {
		if cdErr := os.Chdir(prevDir); cdErr != nil {
			return errors.Join(execErr, cdErr)
		}
	}
	return execErr
}

func migrateExistingHooksBestEffort(binPath string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	_, _ = hooks.MigrateExistingHooks(filepath.Join(home, ".claude", "settings.json"), binPath)
}

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch the JS (OpenTUI) front-end",
	Long: "Launch the OpenTUI/Bun front-end. Looks for a compiled\n" +
		"`slis-ui` binary next to the `slis` binary; falls back to `bun run` against\n" +
		"the tui-js source when SLIS_TUI_DIR points at it. Bare `slis` launches this\n" +
		"same front-end by default (set SLIS_TUI=go for the legacy Go TUI).",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		binPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot locate the slis binary: %w", err)
		}
		migrateExistingHooksBestEffort(binPath)

		launch, err := resolveUILaunch(binPath, os.Getenv("SLIS_TUI_DIR"), regularFileExists)
		if err != nil {
			return err
		}

		return execJSUI(binPath, launch)
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
}
