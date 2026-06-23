package tui

import (
	"os"

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
