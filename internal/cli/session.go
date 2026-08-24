package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"

	"github.com/jonnyom/slis/internal/config"
	sessionmanager "github.com/jonnyom/slis/internal/session"
	"github.com/jonnyom/slis/internal/tmuxctl"
	"github.com/jonnyom/slis/internal/zmxctl"
	"github.com/spf13/cobra"
)

type sessionTabOutput struct {
	ID               string                 `json:"id"`
	Kind             sessionmanager.TabKind `json:"kind"`
	Title            string                 `json:"title"`
	CWD              string                 `json:"cwd"`
	Agent            string                 `json:"agent,omitempty"`
	CurrentDirectory string                 `json:"current_directory,omitempty"`
	Label            string                 `json:"label,omitempty"`
}

type sessionGroupOutput struct {
	ID          string             `json:"id"`
	ActiveTabID string             `json:"active_tab_id"`
	Tabs        []sessionTabOutput `json:"tabs"`
}

type legacyPaneOutput struct {
	Path    string `json:"path"`
	Command string `json:"command"`
	Target  string `json:"target"`
}

type legacySessionOutput struct {
	Name  string             `json:"name"`
	Kind  string             `json:"kind"`
	Panes []legacyPaneOutput `json:"panes"`
}

var sessionHistoryVT bool
var sessionTabKind string
var sessionTabTitle string
var sessionTabCWD string
var sessionListLive bool

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Manage persistent Slis terminal sessions",
}

var sessionEnsureCmd = &cobra.Command{
	Use:   "ensure <slice>",
	Short: "Create or restore a slice session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}
		slice, err := findSlice(ws, args[0])
		if err != nil {
			return err
		}
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		group, err := manager.Ensure(cmd.Context(), slice.Name, membersOfSlice(slice), sessionmanager.LayoutOptions{Root: ws.Root, Layout: ws.Sessions.Layout})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(sessionGroupForOutput(group))
	},
}

var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List persistent Slis sessions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		store := sessionmanager.OpenStore(config.StatePaths().StateDir)
		groups, err := store.List()
		if err != nil {
			return err
		}
		var runtimes []sessionmanager.TabRuntime
		var agentSpecs []config.AgentSpec
		if sessionListLive {
			manager, managerErr := openSessionManager()
			if managerErr != nil {
				return managerErr
			}
			runtimes, err = manager.Runtimes(cmd.Context(), groups)
			if err != nil {
				return err
			}
			ws, workspaceErr := config.LoadWorkspace(config.WorkspacePath())
			if workspaceErr != nil {
				return workspaceErr
			}
			agentSpecs = detectableAgentSpecs(ws.Sessions)
		}
		output := make([]sessionGroupOutput, 0, len(groups))
		for _, group := range groups {
			if sessionListLive {
				output = append(output, sessionGroupForOutputWithRuntime(group, runtimes, agentSpecs))
			} else {
				output = append(output, sessionGroupForOutput(group))
			}
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
	},
}

var sessionBusyCmd = &cobra.Command{
	Use:   "busy <slice> <tab>",
	Short: "Check whether a terminal tab has a running child process",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		busy, err := manager.Busy(cmd.Context(), args[0], args[1])
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Busy bool `json:"busy"`
		}{Busy: busy})
	},
}

var sessionEnsureTabCmd = &cobra.Command{
	Use:   "ensure-tab <slice> <tab>",
	Short: "Create or restore a terminal tab",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		kind, err := parseSessionTabKind(sessionTabKind)
		if err != nil {
			return err
		}
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		title := sessionTabTitle
		if title == "" {
			title = args[1]
		}
		group, err := manager.EnsureTab(cmd.Context(), args[0], sessionmanager.TabSpec{ID: args[1], Kind: kind, Title: title, CWD: sessionTabCWD})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(sessionGroupForOutput(group))
	},
}

var sessionAttachCmd = &cobra.Command{
	Use:   "attach <slice> <tab>",
	Short: "Attach to a persistent terminal tab",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		attach, err := manager.AttachCommand(cmd.Context(), args[0], args[1])
		if err != nil {
			return err
		}
		return syscall.Exec(attach.Path, attach.Args, attach.Env)
	},
}

var sessionLegacyAttachCmd = &cobra.Command{
	Use:    "legacy-attach <session>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !validLegacySessionName(args[0]) {
			return fmt.Errorf("invalid legacy Slis session %q", args[0])
		}
		attach := exec.CommandContext(cmd.Context(), "tmux", "attach", "-t", args[0])
		attach.Stdin = cmd.InOrStdin()
		attach.Stdout = cmd.OutOrStdout()
		attach.Stderr = cmd.ErrOrStderr()
		if err := attach.Run(); err != nil {
			return fmt.Errorf("legacy Slis session attach: %w", err)
		}
		return nil
	},
}

var sessionLegacyListCmd = &cobra.Command{
	Use:    "legacy-list",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !tmuxctl.Available() {
			return json.NewEncoder(cmd.OutOrStdout()).Encode([]legacySessionOutput{})
		}
		panes, err := tmuxctl.ListSessionPanes()
		if err != nil {
			var execError *exec.ExitError
			if errors.As(err, &execError) && execError.ExitCode() == 1 {
				return json.NewEncoder(cmd.OutOrStdout()).Encode([]legacySessionOutput{})
			}
			return fmt.Errorf("legacy Slis session list: %w", err)
		}
		byName := make(map[string]*legacySessionOutput)
		for _, pane := range panes {
			session := byName[pane.Session]
			if session == nil {
				kind := "agent"
				if strings.HasPrefix(pane.Session, "slis-shell/") {
					kind = "shell"
				}
				session = &legacySessionOutput{Name: pane.Session, Kind: kind}
				byName[pane.Session] = session
			}
			session.Panes = append(session.Panes, legacyPaneOutput{Path: pane.Path, Command: pane.Command, Target: pane.Target})
		}
		output := make([]legacySessionOutput, 0, len(byName))
		for _, session := range byName {
			output = append(output, *session)
		}
		sort.Slice(output, func(left, right int) bool { return output[left].Name < output[right].Name })
		return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
	},
}

var sessionLegacyKillCmd = &cobra.Command{
	Use:    "legacy-kill <session>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !validLegacySessionName(args[0]) {
			return fmt.Errorf("invalid legacy Slis session %q", args[0])
		}
		if err := tmuxctl.KillSessionNamed(args[0]); err != nil {
			return fmt.Errorf("legacy Slis session close: %w", err)
		}
		return nil
	},
}

var sessionLegacySendCmd = &cobra.Command{
	Use:    "legacy-send <session>",
	Hidden: true,
	Args:   cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !validLegacySessionName(args[0]) {
			return fmt.Errorf("invalid legacy Slis session %q", args[0])
		}
		input, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}
		load := exec.CommandContext(cmd.Context(), "tmux", "load-buffer", "-")
		load.Stdin = strings.NewReader(string(input))
		if output, err := load.CombinedOutput(); err != nil {
			return fmt.Errorf("legacy Slis session input: %s: %w", strings.TrimSpace(string(output)), err)
		}
		for _, arguments := range [][]string{{"paste-buffer", "-t", args[0]}, {"send-keys", "-t", args[0], "Enter"}} {
			if output, err := exec.CommandContext(cmd.Context(), "tmux", arguments...).CombinedOutput(); err != nil {
				return fmt.Errorf("legacy Slis session input: %s: %w", strings.TrimSpace(string(output)), err)
			}
		}
		return nil
	},
}

var sessionSendCmd = &cobra.Command{
	Use:   "send <slice> <tab>",
	Short: "Send exact stdin bytes to a terminal tab",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		input, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		return manager.Send(cmd.Context(), args[0], args[1], input)
	},
}

var sessionStartCmd = &cobra.Command{
	Use:   "start <slice> <tab>",
	Short: "Start a shell command in a terminal tab",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		input, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}
		if len(input) == 0 {
			return errors.New("session command is required")
		}
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		return manager.StartExistingCommand(cmd.Context(), args[0], args[1], string(input))
	},
}

var sessionHistoryCmd = &cobra.Command{
	Use:   "history <slice> <tab>",
	Short: "Read terminal tab history",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		history, err := manager.History(cmd.Context(), args[0], args[1], sessionHistoryVT)
		if err != nil {
			return err
		}
		_, err = io.WriteString(cmd.OutOrStdout(), history)
		return err
	},
}

var sessionActivateTabCmd = &cobra.Command{
	Use:   "activate-tab <slice> <tab>",
	Short: "Select the active terminal tab",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		return manager.Activate(args[0], args[1])
	},
}

var sessionKillCmd = &cobra.Command{
	Use:   "kill <slice>",
	Short: "Stop and remove a persistent slice session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		return manager.Kill(cmd.Context(), args[0])
	},
}

var sessionKillTabCmd = &cobra.Command{
	Use:   "kill-tab <slice> <tab>",
	Short: "Stop and remove one persistent terminal tab",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		return manager.KillTab(cmd.Context(), args[0], args[1])
	},
}

func openSessionManager() (*sessionmanager.Manager, error) {
	slisBinary, err := os.Executable()
	if err != nil {
		return nil, err
	}
	runtimeBinary, err := zmxctl.ResolveBinary(slisBinary, os.Getenv("SLIS_ZMX_BINARY"))
	if err != nil {
		return nil, err
	}
	runtimeDirectory, err := zmxctl.EnsureRuntimeDirectory(config.WorkspaceScope())
	if err != nil {
		return nil, err
	}
	store := sessionmanager.OpenStore(config.StatePaths().StateDir)
	return sessionmanager.NewManager(store, zmxctl.New(runtimeBinary, runtimeDirectory)), nil
}

func sessionGroupForOutput(group sessionmanager.Group) sessionGroupOutput {
	output := sessionGroupOutput{ID: group.ID, ActiveTabID: group.ActiveTabID, Tabs: make([]sessionTabOutput, 0, len(group.Tabs))}
	for _, tab := range group.Tabs {
		output.Tabs = append(output.Tabs, sessionTabOutput{ID: tab.ID, Kind: tab.Kind, Title: tab.Title, CWD: tab.CWD})
	}
	return output
}

func parseSessionTabKind(value string) (sessionmanager.TabKind, error) {
	switch sessionmanager.TabKind(value) {
	case sessionmanager.TabKindAgent, sessionmanager.TabKindShell, sessionmanager.TabKindReview:
		return sessionmanager.TabKind(value), nil
	default:
		return "", fmt.Errorf("invalid terminal tab kind %q", value)
	}
}

func validLegacySessionName(name string) bool {
	return !strings.ContainsRune(name, '\x00') && (strings.HasPrefix(name, "slis/") || strings.HasPrefix(name, "slis-shell/"))
}

func init() {
	sessionListCmd.Flags().BoolVar(&sessionListLive, "live", false, "Include live agent and directory labels")
	sessionHistoryCmd.Flags().BoolVar(&sessionHistoryVT, "vt", false, "return reconstructed terminal state")
	sessionEnsureTabCmd.Flags().StringVar(&sessionTabKind, "kind", "shell", "terminal tab kind: agent, shell, or review")
	sessionEnsureTabCmd.Flags().StringVar(&sessionTabTitle, "title", "", "terminal tab title")
	sessionEnsureTabCmd.Flags().StringVar(&sessionTabCWD, "cwd", "", "terminal start directory")
	sessionCmd.AddCommand(sessionActivateTabCmd, sessionAttachCmd, sessionBusyCmd, sessionEnsureCmd, sessionEnsureTabCmd, sessionHistoryCmd, sessionKillCmd, sessionKillTabCmd, sessionLegacyAttachCmd, sessionLegacyKillCmd, sessionLegacyListCmd, sessionLegacySendCmd, sessionListCmd, sessionSendCmd, sessionStartCmd)
	rootCmd.AddCommand(sessionCmd)
}
