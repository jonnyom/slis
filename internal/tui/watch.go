package tui

import (
	"sort"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

// eventsChangedMsg is delivered by waitForEventCmd when the watcher fires.
type eventsChangedMsg struct{}

// NewlyInStatus returns slice names whose status transitioned TO target between
// the old and new status maps (was not target before, is now). Sorted for
// determinism.
func NewlyInStatus(old, new map[string]model.SessionStatus, target model.SessionStatus) []string {
	var result []string
	for name, newStatus := range new {
		if newStatus != target {
			continue
		}
		if old[name] == target {
			// Already in this status before — not a new transition.
			continue
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// NewlyWaiting returns slice names that just transitioned to SessWaitingInput.
func NewlyWaiting(old, new map[string]model.SessionStatus) []string {
	return NewlyInStatus(old, new, model.SessWaitingInput)
}

// waitForEventCmd returns a Cmd that blocks on the next fsnotify event or error.
// When an event arrives it returns eventsChangedMsg{} to trigger a reload.
// If w is nil it returns nil (no-op).
func waitForEventCmd(w *fsnotify.Watcher) tea.Cmd {
	if w == nil {
		return nil
	}
	return func() tea.Msg {
		select {
		case <-w.Events:
			return eventsChangedMsg{}
		case <-w.Errors:
			return eventsChangedMsg{}
		}
	}
}

// sessionAlert pairs a slice with the status it just entered, so notifyCmd can
// word the desktop notification appropriately.
type sessionAlert struct {
	slice  string
	status model.SessionStatus
}

// notifyCmd returns a Cmd that fires a desktop notification for each alert
// (Claude is waiting on you, or has finished a turn), then returns nil.
func notifyCmd(alerts []sessionAlert) tea.Cmd {
	return func() tea.Msg {
		for _, a := range alerts {
			switch a.status {
			case model.SessWaitingInput:
				_ = notify.Notify("slis", a.slice+" needs your input")
			case model.SessDone:
				_ = notify.Notify("slis", a.slice+" finished — your move")
			}
		}
		return nil
	}
}
