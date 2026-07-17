package tui

import (
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

// TestEventsChangedMsgReturnsCmd verifies that Update on an eventsChangedMsg
// returns a non-nil command (the batch that re-issues loadSessionsCmd and
// waitForEventCmd).
func TestEventsChangedMsgReturnsCmd(t *testing.T) {
	m := New(config.Workspace{})
	m.loading = false
	m.slices = []model.Slice{
		{Name: "alpha"},
	}

	_, cmd := m.Update(eventsChangedMsg{})
	if cmd == nil {
		t.Error("Update(eventsChangedMsg{}) must return a non-nil Cmd")
	}
}
