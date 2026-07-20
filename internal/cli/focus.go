package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// focusAction is what `slis focus` decides to do given the attached tmux clients.
type focusAction int

const (
	// focusSwitch points a specific client at the target session.
	focusSwitch focusAction = iota
	// focusAttachHint prints the attach command because no client is attached.
	focusAttachHint
	// focusAlready is a no-op: the chosen client already views the target session.
	focusAlready
)

// decideFocus picks the focus action for the target session given the attached
// clients: no clients → print an attach hint; the most-recently-used client is
// already on the target → no-op; otherwise switch that client to the target.
// Pure so it is unit-testable without spawning tmux.
func decideFocus(clients []tmuxctl.Client, targetSession string) (focusAction, tmuxctl.Client) {
	c, ok := tmuxctl.MostRecentClient(clients)
	if !ok {
		return focusAttachHint, tmuxctl.Client{}
	}
	if c.Session == targetSession {
		return focusAlready, c
	}
	return focusSwitch, c
}

// membersOfSlice returns a slice's members in sorted repo order (for session
// creation, which wants a deterministic window order).
func membersOfSlice(sl model.Slice) []model.SliceMember {
	repos := sl.Repos()
	members := make([]model.SliceMember, 0, len(repos))
	for _, repo := range repos {
		members = append(members, sl.Members[repo])
	}
	return members
}

func targetSessionForSlice(sl model.Slice, panes []tmuxctl.SessionPane) string {
	related := make(map[string]bool)
	for _, pane := range panes {
		if strings.HasPrefix(pane.Session, "slis-shell/") {
			continue
		}
		for _, member := range sl.Members {
			relative, err := filepath.Rel(member.WorktreePath, pane.Path)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				continue
			}
			related[pane.Session] = true
			if !tmuxctl.IsShellCommand(pane.Command) {
				return pane.Session
			}
		}
	}
	canonical := tmuxctl.SessionName(sl.Name)
	if related[canonical] {
		return canonical
	}
	for _, pane := range panes {
		if related[pane.Session] {
			return pane.Session
		}
	}
	return canonical
}

func detachedSessionAttachArgv(terminalApp, target string) (string, []string, bool) {
	if strings.EqualFold(terminalApp, "ghostty") {
		return "open", []string{"-na", "Ghostty.app", "--args", "-e", "tmux", "attach", "-t", target}, true
	}
	return "", nil, false
}

var focusCmd = &cobra.Command{
	Use:   "focus <slice>",
	Short: "Switch the active tmux client to a slice's session (used by notification clicks)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validateSliceName(name); err != nil {
			return err
		}
		if !tmuxctl.Available() {
			return fmt.Errorf("tmux not found on PATH")
		}

		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sl, err := findSlice(ws, name)
		if err != nil {
			return err
		}

		if err := tmuxctl.EnsureSession(sl.Name, membersOfSlice(sl), tmuxctl.SessionOpts{Root: ws.Root, Layout: ws.Sessions.Layout}); err != nil {
			return fmt.Errorf("ensure tmux session: %w", err)
		}

		panes, err := tmuxctl.ListSessionPanes()
		if err != nil {
			return err
		}
		target := targetSessionForSlice(sl, panes)
		clients, err := tmuxctl.ListClients()
		if err != nil {
			return err
		}

		switch action, client := decideFocus(clients, target); action {
		case focusAttachHint:
			if launchName, launchArgs, ok := detachedSessionAttachArgv(os.Getenv("SLIS_TERMINAL_APP"), target); ok {
				if out, err := exec.Command(launchName, launchArgs...).CombinedOutput(); err != nil {
					return fmt.Errorf("open terminal session: %w: %s", err, out)
				}
				return nil
			}
			fmt.Printf("tmux attach -t %s\n", target)
			return nil
		case focusAlready:
			return nil
		default:
			switchName, switchArgs := tmuxctl.SwitchClientTargetArgv(client.TTY, target)
			if out, err := exec.Command(switchName, switchArgs...).CombinedOutput(); err != nil {
				return fmt.Errorf("tmux switch-client: %w: %s", err, out)
			}
			return nil
		}
	},
}

func init() {
	rootCmd.AddCommand(focusCmd)
}
