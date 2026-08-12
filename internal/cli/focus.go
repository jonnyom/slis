package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	sessionmanager "github.com/jonnyom/slis/internal/session"
)

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

func detachedSlisAttachArgv(terminalApp, groupID, tabID string) (string, []string, bool) {
	if strings.EqualFold(terminalApp, "ghostty") {
		return "open", []string{"-na", "Ghostty.app", "--args", "-e", "slis", "session", "attach", groupID, tabID}, true
	}
	return "", nil, false
}

func newFocusRequest(groupID, tabID string) (sessionmanager.FocusRequest, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return sessionmanager.FocusRequest{}, err
	}
	return sessionmanager.FocusRequest{ID: hex.EncodeToString(random), GroupID: groupID, TabID: tabID, TimeNS: time.Now().UnixNano()}, nil
}

var focusCmd = &cobra.Command{
	Use:   "focus <slice>",
	Short: "Focus a slice's persistent Slis session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := validateSliceName(name); err != nil {
			return err
		}
		ws, err := config.LoadWorkspace(config.WorkspacePath())
		if err != nil {
			return fmt.Errorf("workspace not found — run `slis init` first: %w", err)
		}

		sl, err := findSlice(ws, name)
		if err != nil {
			return err
		}

		manager, err := openSessionManager()
		if err != nil {
			return err
		}
		group, err := manager.Ensure(cmd.Context(), sl.Name, membersOfSlice(sl), sessionmanager.LayoutOptions{Root: ws.Root, Layout: ws.Sessions.Layout})
		if err != nil {
			return err
		}
		if group.ActiveTabID == "" {
			return fmt.Errorf("slice %q has no terminal tabs", sl.Name)
		}
		request, err := newFocusRequest(group.ID, group.ActiveTabID)
		if err != nil {
			return err
		}
		focusContext, cancel := context.WithTimeout(cmd.Context(), 750*time.Millisecond)
		focused, err := sessionmanager.RequestFrontendFocus(focusContext, config.StatePaths().StateDir, request)
		cancel()
		if err != nil {
			return err
		}
		if focused {
			return nil
		}
		if launchName, launchArgs, ok := detachedSlisAttachArgv(os.Getenv("SLIS_TERMINAL_APP"), group.ID, group.ActiveTabID); ok {
			if out, err := exec.Command(launchName, launchArgs...).CombinedOutput(); err != nil {
				return fmt.Errorf("open terminal session: %w: %s", err, out)
			}
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "slis session attach %s %s\n", group.ID, group.ActiveTabID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(focusCmd)
}
