package cli

import (
	"fmt"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/editor"
	"github.com/jonnyom/slis/internal/model"
	"github.com/spf13/cobra"
)

// sliceWorktrees returns a slice's member worktree paths in sorted repo order.
func sliceWorktrees(sl model.Slice) []string {
	repos := sl.Repos() // sorted
	paths := make([]string, 0, len(repos))
	for _, r := range repos {
		if p := sl.Members[r].WorktreePath; p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

var editCmd = &cobra.Command{
	Use:   "edit <slice>",
	Short: "Open a slice's worktrees in your editor (one multi-root window)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		repoFlag, _ := cmd.Flags().GetString("repo")
		printOnly, _ := cmd.Flags().GetBool("print")

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		dto, err := findSlice(ws, name)
		if err != nil {
			return err
		}
		sp := config.StatePaths()

		// Single-repo open.
		if repoFlag != "" {
			path := ""
			for _, m := range dto.Members {
				if m.Repo == repoFlag {
					path = m.WorktreePath
				}
			}
			if path == "" {
				return fmt.Errorf("repo %q not found (or has no worktree) in slice %q", repoFlag, name)
			}
			if printOnly {
				fmt.Println(path)
				return nil
			}
			ed, err := editor.Resolve(ws.Sessions.Editor)
			if err != nil {
				return err
			}
			return editor.OpenDir(ed, path)
		}

		// Whole-slice open.
		worktrees := sliceWorktrees(dto)
		if len(worktrees) == 0 {
			return fmt.Errorf("slice %q has no worktrees to open", name)
		}
		// --print emits the .code-workspace path (editor-agnostic; agent-friendly).
		if printOnly {
			f, err := editor.WriteWorkspaceFile(sp.WorkspacesDir, name, worktrees)
			if err != nil {
				return err
			}
			fmt.Println(f)
			return nil
		}
		ed, err := editor.Resolve(ws.Sessions.Editor)
		if err != nil {
			return err
		}
		return editor.OpenSlice(ed, name, worktrees, sp.WorkspacesDir)
	},
}

func init() {
	editCmd.Flags().String("repo", "", "Open only this repo's worktree instead of the whole slice")
	editCmd.Flags().Bool("print", false, "Print the .code-workspace path (or repo worktree path) instead of launching")
	rootCmd.AddCommand(editCmd)
}
