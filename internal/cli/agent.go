package cli

import (
	"fmt"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/spf13/cobra"
)

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Show or set the default coding agent",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		if ws.Sessions.DefaultAgent == "" {
			fmt.Println("default: (none — ask when multiple agents are available)")
		} else {
			fmt.Printf("default: %s\n", ws.Sessions.DefaultAgent)
		}
		return nil
	},
}

var agentSetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default coding agent in workspace.yaml",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := strings.TrimSpace(args[0])
		if name == "" {
			return fmt.Errorf("agent name is empty")
		}
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		ws.Sessions.DefaultAgent = name
		if err := config.SaveWorkspace(config.WorkspacePath(), ws); err != nil {
			return err
		}
		fmt.Printf("default agent set to %q\n", name)
		return nil
	},
}

var agentClearDefaultCmd = &cobra.Command{
	Use:   "clear-default",
	Short: "Forget the default and ask again when multiple agents are available",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		ws.Sessions.DefaultAgent = ""
		if err := config.SaveWorkspace(config.WorkspacePath(), ws); err != nil {
			return err
		}
		fmt.Println("default agent cleared")
		return nil
	},
}

func init() {
	agentCmd.AddCommand(agentSetDefaultCmd)
	agentCmd.AddCommand(agentClearDefaultCmd)
	rootCmd.AddCommand(agentCmd)
}
