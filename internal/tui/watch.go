package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// eventsChangedMsg is delivered by waitForEventCmd when the watcher fires.
type eventsChangedMsg struct{}

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
