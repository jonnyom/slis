package cli

import (
	"fmt"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/editor"
	"github.com/spf13/cobra"
)

var editorCmd = &cobra.Command{
	Use:   "editor",
	Short: "Show or set the editor used by `slis edit` and the TUI (o/e)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		configured := ws.Sessions.Editor
		if configured == "" {
			fmt.Println("configured: (none — auto-detect)")
		} else {
			fmt.Printf("configured: %s\n", configured)
		}

		avail := editor.Available()
		if len(avail) == 0 {
			fmt.Println("detected:   (none on PATH — tried cursor, code, code-insiders, codium, windsurf, zed)")
		} else {
			fmt.Println("detected:")
			for _, e := range avail {
				fmt.Printf("  %-16s %s\n", e.Bin, e.Name)
			}
		}
		if ed, err := editor.Resolve(configured); err == nil {
			fmt.Printf("in use:     %s (%s)\n", ed.Bin, ed.Name)
		}
		return nil
	},
}

var editorSetCmd = &cobra.Command{
	Use:   "set <binary>",
	Short: "Set the editor (e.g. code, cursor, zed) in workspace.yaml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		bin := args[0]
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		if _, err := editor.Resolve(bin); err != nil {
			return err // not on PATH
		}
		ws.Sessions.Editor = bin
		if err := config.SaveWorkspace(config.WorkspacePath(), ws); err != nil {
			return err
		}
		fmt.Printf("editor set to %q\n", bin)
		return nil
	},
}

var editorClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Unset the configured editor (revert to auto-detect)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		ws.Sessions.Editor = ""
		if err := config.SaveWorkspace(config.WorkspacePath(), ws); err != nil {
			return err
		}
		fmt.Println("editor cleared (auto-detect)")
		return nil
	},
}

func init() {
	editorCmd.AddCommand(editorSetCmd)
	editorCmd.AddCommand(editorClearCmd)
	rootCmd.AddCommand(editorCmd)
}
