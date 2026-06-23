package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/hooks"
)

var initCmd = &cobra.Command{
	Use:   "init [root]",
	Short: "Initialise a slis workspace",
	Long: `Scan root (default: current directory) for git repositories and write
workspace.yaml. If --repos is provided the interactive picker is skipped.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := ""
		if len(args) > 0 {
			root = args[0]
		} else {
			var err error
			root, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("init: getwd: %w", err)
			}
		}

		repos, _ := cmd.Flags().GetStringSlice("repos")
		stripPrefix, _ := cmd.Flags().GetString("strip-prefix")

		writtenPath, err := Init(root, repos, stripPrefix)
		if err != nil {
			return err
		}

		fmt.Println("workspace written to:", writtenPath)

		// Install the Claude Code hooks so session notifications ("needs you" /
		// finished) work out of the box — users expect `slis init` to set slis
		// up fully, not to leave the notification half un-wired. Idempotent and
		// append-only; opt out with --no-hooks (or run `slis init-hooks` later).
		noHooks, _ := cmd.Flags().GetBool("no-hooks")
		if !noHooks {
			if err := installHooks(); err != nil {
				// Don't fail init over hooks — the workspace is already written.
				fmt.Fprintf(os.Stderr, "note: could not install Claude hooks (run `slis init-hooks` later): %v\n", err)
			}
		}
		return nil
	},
}

// installHooks wires the slis Claude Code hooks into ~/.claude/settings.json and
// prints what changed. Shared by `slis init` and `slis init-hooks`.
func installHooks() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")

	binPath, err := os.Executable()
	if err != nil {
		binPath = "slis" // fall back to PATH lookup
	}

	changes, err := hooks.InitHooks(settingsPath, binPath)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Printf("Claude hooks already installed in %s\n", settingsPath)
		return nil
	}
	fmt.Printf("installed Claude session hooks in %s:\n", settingsPath)
	for _, c := range changes {
		fmt.Printf("  • %s\n", c)
	}
	return nil
}

func init() {
	initCmd.Flags().StringSlice("repos", nil, "Comma-separated list of repo names to include (non-interactive)")
	initCmd.Flags().String("strip-prefix", "", "Branch-name prefix to strip when naming slices, e.g. 'jonny/'")
	initCmd.Flags().Bool("no-hooks", false, "Skip installing Claude Code session hooks (run `slis init-hooks` separately)")
}
