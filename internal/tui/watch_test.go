package tui

import (
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

// TestNewlyWaiting verifies that NewlyWaiting returns only slices that
// transitioned TO SessWaitingInput (i.e. were not already waiting before).
func TestNewlyWaiting(t *testing.T) {
	old := map[string]model.SessionStatus{
		"a": model.SessRunning,
		"b": model.SessNone,
	}
	// a: Running → WaitingInput (newly waiting)
	// b: None    → WaitingInput (newly waiting)
	// c: <absent>→ WaitingInput (newly waiting)
	newSt := map[string]model.SessionStatus{
		"a": model.SessWaitingInput,
		"b": model.SessWaitingInput,
		"c": model.SessWaitingInput,
	}

	got := NewlyWaiting(old, newSt)

	if len(got) != 3 {
		t.Fatalf("want 3 newly waiting, got %d: %v", len(got), got)
	}
	// Result must be sorted.
	want := []string{"a", "b", "c"}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d]=%q, want %q", i, got[i], w)
		}
	}
}

// TestNewlyWaitingNoChange verifies that a slice already in WaitingInput
// that stays in WaitingInput does NOT appear in the result.
func TestNewlyWaitingNoChange(t *testing.T) {
	old := map[string]model.SessionStatus{
		"x": model.SessWaitingInput,
		"y": model.SessRunning,
	}
	newSt := map[string]model.SessionStatus{
		"x": model.SessWaitingInput, // still waiting — must not be included
		"y": model.SessRunning,      // not waiting at all
	}

	got := NewlyWaiting(old, newSt)
	if len(got) != 0 {
		t.Errorf("want empty result, got %v", got)
	}
}

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
