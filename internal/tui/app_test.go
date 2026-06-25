package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/model"
)

// TestEnterCockpitNoModal verifies that opening a slice enters the cockpit, kicks
// off loads, and that a landing PR load (even with comments) never routes the view
// to a separate modal/overlay — comments live inline in the PR pane now.
func TestEnterCockpitNoModal(t *testing.T) {
	m := threeSlices(t) // focus=0 → "alpha", browser view

	next, cmd := m.Update(keyMsg('l')) // open the cockpit
	m = next.(Model)
	if m.view != viewCockpit {
		t.Fatalf("view = %v, want cockpit", m.view)
	}
	if cmd == nil {
		t.Error("entering a slice should kick off loads")
	}

	// A fresh PR load WITH comments must not change the view away from the cockpit.
	withComments := map[string]*forge.PR{"web": {Number: 1, Comments: []forge.Comment{{Author: "a", Body: "hi"}}}}
	next, _ = m.Update(prsLoadedMsg{slice: "alpha", prs: withComments})
	m = next.(Model)
	if m.view != viewCockpit {
		t.Error("prsLoadedMsg must not change the view (no auto-opened comments modal)")
	}
}

// TestCreateSpinnerLifecycle verifies the in-TUI create flow: the spinner ticks
// while a slice is being created, then stops and reports once it finishes.
func TestCreateSpinnerLifecycle(t *testing.T) {
	m := threeSlices(t)
	m.creatingSlice = "newslice"

	// A tick advances the frame and keeps the spinner going.
	next, cmd := m.Update(spinnerTickMsg{})
	m = next.(Model)
	if m.spinnerFrame != 1 {
		t.Errorf("spinnerFrame = %d, want 1", m.spinnerFrame)
	}
	if cmd == nil {
		t.Error("spinner should keep ticking while a create is in flight")
	}

	// Completion clears the in-flight state, sets a status, and triggers a reload.
	next, cmd = m.Update(createFinishedMsg{name: "newslice"})
	m = next.(Model)
	if m.creatingSlice != "" {
		t.Error("creatingSlice should be cleared once create finishes")
	}
	if !strings.Contains(m.status, "newslice") {
		t.Errorf("status = %q, want it to mention the slice", m.status)
	}
	if cmd == nil {
		t.Error("expected a reload cmd after create finishes")
	}

	// With nothing in flight, further ticks stop rescheduling.
	if _, cmd := m.Update(spinnerTickMsg{}); cmd != nil {
		t.Error("spinner should stop ticking once creation has finished")
	}
}

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

// TestCreatingSwallowsGlobalKeys verifies that while typing a new-slice name,
// keys that are otherwise global commands (r=refresh, q=quit, a=attach) insert
// into the name instead of firing — only esc/enter should act as commands.
func TestCreatingSwallowsGlobalKeys(t *testing.T) {
	m := threeSlices(t)
	m.creating = true
	m.createName = ""

	for _, r := range []rune{'r', 'q', 'a', 'r'} {
		next, cmd := m.Update(keyMsg(r))
		m = next.(Model)
		if cmd != nil {
			t.Fatalf("key %q while creating returned a command (want none)", r)
		}
	}
	if !m.creating {
		t.Fatal("creating flag was cleared by a command key; want still creating")
	}
	if m.createName != "rqar" {
		t.Errorf("createName = %q, want %q", m.createName, "rqar")
	}
}

// TestRefreshClearsPRCache verifies `r` drops cached PR state so merge status
// (→ "Ready") re-fetches after a merge done outside slis.
func TestRefreshClearsPRCache(t *testing.T) {
	m := threeSlices(t) // browser view
	m.prs = map[string]map[string]*forge.PR{
		"alpha": {"web": {Number: 1, State: "OPEN"}},
	}
	next, _ := m.Update(keyMsg('r'))
	m = next.(Model)
	if len(m.prs) != 0 {
		t.Errorf("after r: m.prs should be cleared to force a reload, got %v", m.prs)
	}
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

	// Focus marker (cursor bar) should appear on the focused row.
	if !strings.Contains(view, "▎") {
		t.Errorf("View() missing focus marker '▎'; got:\n%s", view)
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
