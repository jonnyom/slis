package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// sessionsLoadedMsg is delivered once session-status loading completes.
type sessionsLoadedMsg struct {
	statuses map[string]model.SessionStatus
}

// sessionsRefreshMsg is delivered after attach returns, triggering a re-load.
type sessionsRefreshMsg struct{}

// sessionBadge returns the display glyph for a given SessionStatus.
//
//	SessWaitingInput → "⏸"
//	SessRunning      → "●"
//	SessDone         → "✓"
//	SessNone         → "○"
func sessionBadge(s model.SessionStatus) string {
	switch s {
	case model.SessWaitingInput:
		return "⏸"
	case model.SessRunning:
		return "●"
	case model.SessDone:
		return "✓"
	default:
		return "○"
	}
}

// loadSessionsCmd returns a Cmd that builds a map[string]model.SessionStatus.
// It starts from the event store (notify.ReadAllStatuses) and for any slice
// without an event-file entry it falls back to tmux session presence.
func loadSessionsCmd(slices []model.Slice, eventsDir string) tea.Cmd {
	return func() tea.Msg {
		statuses := notify.ReadAllStatuses(eventsDir)
		for _, sl := range slices {
			if _, ok := statuses[sl.Name]; ok {
				continue
			}
			if tmuxctl.SessionExists(sl.Name) {
				statuses[sl.Name] = model.SessRunning
			} else {
				statuses[sl.Name] = model.SessNone
			}
		}
		return sessionsLoadedMsg{statuses: statuses}
	}
}

// membersSlice converts a slice's Members map to a sorted []model.SliceMember.
func membersSlice(sl model.Slice) []model.SliceMember {
	repos := sl.Repos() // already sorted
	members := make([]model.SliceMember, 0, len(repos))
	for _, repo := range repos {
		members = append(members, sl.Members[repo])
	}
	return members
}

// isInsideTmux reports whether the current process is running inside tmux.
func isInsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// renderSessionsTab renders the Sessions tab body for the focused slice.
func renderSessionsTab(m Model) string {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return "no slice selected\n"
	}

	sl := m.slices[m.focus]
	var sb strings.Builder

	// Session name.
	sb.WriteString(fmt.Sprintf("session: %s\n", sl.Name))

	// Status.
	status := model.SessNone
	if m.sessionStatus != nil {
		status = m.sessionStatus[sl.Name]
	}
	sb.WriteString(fmt.Sprintf("status:  %s %s\n", sessionBadge(status), status.String()))

	// tmux availability guard.
	if !tmuxctl.Available() {
		sb.WriteString("\ntmux not available on this system\n")
		return sb.String()
	}

	// Session existence.
	exists := tmuxctl.SessionExists(sl.Name)
	existsStr := "no"
	if exists {
		existsStr = "yes"
	}
	sb.WriteString(fmt.Sprintf("tmux session exists: %s\n", existsStr))

	// Repo windows.
	repos := sl.Repos()
	if len(repos) > 0 {
		sb.WriteString("\nwindows (repos):\n")
		for _, repo := range repos {
			sm := sl.Members[repo]
			sb.WriteString(fmt.Sprintf("  %s  %s\n", repo, sm.WorktreePath))
		}
	}

	sb.WriteString("\n[a] attach / create session\n")

	return sb.String()
}
