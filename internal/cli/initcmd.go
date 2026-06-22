package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
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
		return nil
	},
}

func init() {
	initCmd.Flags().StringSlice("repos", nil, "Comma-separated list of repo names to include (non-interactive)")
	initCmd.Flags().String("strip-prefix", "", "Branch-name prefix to strip when naming slices, e.g. 'jonny/'")
}
