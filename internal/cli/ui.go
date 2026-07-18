package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

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

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "[experimental] Launch the JS (OpenTUI) front-end",
	Long: "Launch the experimental OpenTUI/Bun front-end. Looks for a compiled\n" +
		"`slis-ui` binary next to the `slis` binary; falls back to `bun run` against\n" +
		"the tui-js source when SLIS_TUI_DIR points at it. Bare `slis` still launches\n" +
		"the Go (Bubble Tea) TUI.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		binPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("cannot locate the slis binary: %w", err)
		}

		launch, err := resolveUILaunch(binPath, os.Getenv("SLIS_TUI_DIR"), regularFileExists)
		if err != nil {
			return err
		}

		argv0, err := exec.LookPath(launch.name)
		if err != nil {
			return fmt.Errorf("cannot find %q on PATH: %w", launch.name, err)
		}

		if launch.dir != "" {
			if err := os.Chdir(launch.dir); err != nil {
				return fmt.Errorf("cannot enter %s: %w", launch.dir, err)
			}
		}

		env := os.Environ()
		if os.Getenv("SLIS_BIN") == "" {
			env = append(env, "SLIS_BIN="+binPath)
		}

		return syscall.Exec(argv0, append([]string{launch.name}, launch.args...), env)
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
}
