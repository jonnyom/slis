package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

// threeSlices returns a model with three named slices already loaded (loading=false).
func threeSlices(t *testing.T) Model {
	t.Helper()
	m := New(config.Workspace{})
	m.slices = []model.Slice{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}
	m.loading = false
	return m
}

func keyMsg(r rune) tea.Msg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func downMsg() tea.Msg {
	return tea.KeyMsg{Type: tea.KeyDown}
}

func upMsg() tea.Msg {
	return tea.KeyMsg{Type: tea.KeyUp}
}

// TestUpdateNavigation verifies j/down moves focus down (clamped) and k/up moves it up.
func TestUpdateNavigation(t *testing.T) {
	m := threeSlices(t)

	// Initial focus is 0.
	if m.focus != 0 {
		t.Fatalf("want focus=0, got %d", m.focus)
	}

	// Press "j" → focus 1.
	next, _ := m.Update(keyMsg('j'))
	m = next.(Model)
	if m.focus != 1 {
		t.Errorf("after j: want focus=1, got %d", m.focus)
	}

	// Press "j" → focus 2.
	next, _ = m.Update(keyMsg('j'))
	m = next.(Model)
	if m.focus != 2 {
		t.Errorf("after j j: want focus=2, got %d", m.focus)
	}

	// Press "j" again → still 2 (clamped at len-1).
	next, _ = m.Update(keyMsg('j'))
	m = next.(Model)
	if m.focus != 2 {
		t.Errorf("after j j j: want focus clamped at 2, got %d", m.focus)
	}

	// Press "k" → focus 1.
	next, _ = m.Update(keyMsg('k'))
	m = next.(Model)
	if m.focus != 1 {
		t.Errorf("after k: want focus=1, got %d", m.focus)
	}

	// Press "k" "k" → focus 0 (clamped).
	next, _ = m.Update(keyMsg('k'))
	m = next.(Model)
	next, _ = m.Update(keyMsg('k'))
	m = next.(Model)
	if m.focus != 0 {
		t.Errorf("after extra k: want focus clamped at 0, got %d", m.focus)
	}

	// KeyDown also moves focus.
	next, _ = m.Update(downMsg())
	m = next.(Model)
	if m.focus != 1 {
		t.Errorf("after KeyDown: want focus=1, got %d", m.focus)
	}

	// KeyUp also moves focus.
	next, _ = m.Update(upMsg())
	m = next.(Model)
	if m.focus != 0 {
		t.Errorf("after KeyUp: want focus=0, got %d", m.focus)
	}
}

// TestUpdateQuit verifies that pressing "q" returns a non-nil Cmd that resolves to tea.QuitMsg.
func TestUpdateQuit(t *testing.T) {
	m := threeSlices(t)
	_, cmd := m.Update(keyMsg('q'))
	if cmd == nil {
		t.Fatal("pressing q must return a non-nil Cmd")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("cmd() returned %T, want tea.QuitMsg", msg)
	}
}

// TestSlicesLoadedMsg verifies that a slicesLoadedMsg stores slices and clears loading.
func TestSlicesLoadedMsg(t *testing.T) {
	m := New(config.Workspace{})
	if !m.loading {
		t.Fatal("New() should return loading=true")
	}

	next, _ := m.Update(slicesLoadedMsg{
		slices: []model.Slice{
			{Name: "x"},
			{Name: "y"},
			{Name: "z"},
		},
	})
	m = next.(Model)

	if m.loading {
		t.Error("after slicesLoadedMsg: loading should be false")
	}
	if len(m.slices) != 3 {
		t.Errorf("after slicesLoadedMsg: want 3 slices, got %d", len(m.slices))
	}
}

// TestViewRendersNames verifies that View() includes slice names and the focus marker.
func TestViewRendersNames(t *testing.T) {
	m := New(config.Workspace{})
	m.loading = false
	m.slices = []model.Slice{
		{Name: "feature-login"},
		{Name: "feature-signup"},
	}
	m.focus = 0

	view := m.View()

	if !strings.Contains(view, "feature-login") {
		t.Errorf("View() missing slice name 'feature-login'; got:\n%s", view)
	}
	if !strings.Contains(view, "feature-signup") {
		t.Errorf("View() missing slice name 'feature-signup'; got:\n%s", view)
	}

	// Focus marker should appear on the focused row.
	if !strings.Contains(view, ">") {
		t.Errorf("View() missing focus marker '>'; got:\n%s", view)
	}
}

// TestViewLoadingState verifies the loading message is shown when loading=true.
func TestViewLoadingState(t *testing.T) {
	m := New(config.Workspace{})
	// loading is true by default from New()
	view := m.View()
	if !strings.Contains(view, "Loading") {
		t.Errorf("View() should show loading message, got:\n%s", view)
	}
}

// TestViewEmptySlices verifies View() is tolerant of an empty slice list.
func TestViewEmptySlices(t *testing.T) {
	m := New(config.Workspace{})
	m.loading = false
	// No panic expected with zero slices.
	_ = m.View()
}

// TestSummaryLoadedMsg verifies that summaryLoadedMsg stores the text in the
// cache, clears summaryLoading, and does not call claude (pure in-process test).
func TestSummaryLoadedMsg(t *testing.T) {
	m := threeSlices(t)

	// Simulate the [s] key press side-effects: mark loading=true, clear cache.
	m.summaryLoading["alpha"] = true
	delete(m.summaries, "alpha")

	// Deliver a summaryLoadedMsg (as the aiSummaryFromSliceCmd would).
	next, cmd := m.Update(summaryLoadedMsg{slice: "alpha", text: "## Summary\n\n- feat: hello"})
	m = next.(Model)

	// cmd should be nil — no follow-up work needed.
	if cmd != nil {
		t.Errorf("after summaryLoadedMsg: want nil cmd, got non-nil")
	}

	// Cache must now contain the text.
	text, ok := m.summaries["alpha"]
	if !ok {
		t.Fatal("summaries['alpha'] should be present after summaryLoadedMsg")
	}
	if text != "## Summary\n\n- feat: hello" {
		t.Errorf("summaries['alpha'] = %q, want the delivered text", text)
	}

	// Loading flag must be cleared.
	if m.summaryLoading["alpha"] {
		t.Error("summaryLoading['alpha'] should be false after summaryLoadedMsg")
	}
}
