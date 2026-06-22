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

// NewlyWaiting returns slice names whose status transitioned TO SessWaitingInput
// between the old and new status maps (was not waiting before, is now).
// The result is sorted for determinism.
func NewlyWaiting(old, new map[string]model.SessionStatus) []string {
	var result []string
	for name, newStatus := range new {
		if newStatus != model.SessWaitingInput {
			continue
		}
		if old[name] == model.SessWaitingInput {
			// Already waiting before — not a new transition.
			continue
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
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

// notifyCmd returns a Cmd that fires desktop notifications for each newly-waiting
// slice, then returns nil.
func notifyCmd(slices []string) tea.Cmd {
	return func() tea.Msg {
		for _, s := range slices {
			_ = notify.Notify("slis", s+" needs input")
		}
		return nil
	}
}
