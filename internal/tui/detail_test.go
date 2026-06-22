package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// modelWithSlices returns a Model with slices loaded, loading=false, and maps initialised.
func modelWithSlices(t *testing.T) Model {
	t.Helper()
	m := New(config.Workspace{})
	m.slices = []model.Slice{
		{
			Name: "feature-a",
			Members: map[string]model.SliceMember{
				"web": {Repo: "web", Branch: "feature-a", WorktreePath: "/tmp/web"},
			},
		},
		{Name: "feature-b"},
	}
	m.loading = false
	return m
}

// TestTabSwitching verifies that tab/l advances the activeTab (mod 5) and
// shift+tab/h goes backwards.
func TestTabSwitching(t *testing.T) {
	m := modelWithSlices(t)

	if m.activeTab != TabStack {
		t.Fatalf("initial activeTab should be TabStack (0), got %d", m.activeTab)
	}

	// Advance through all 5 tabs with "tab" key.
	for i := 1; i <= 5; i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		expected := Tab(i % 5)
		if m.activeTab != expected {
			t.Errorf("after %d tab presses: want activeTab=%d, got %d", i, expected, m.activeTab)
		}
	}

	// After 5 presses, we're back at TabStack (0).
	if m.activeTab != TabStack {
		t.Errorf("after full cycle: want TabStack, got %d", m.activeTab)
	}

	// Also test "l" key.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = next.(Model)
	if m.activeTab != TabSummary {
		t.Errorf("l key: want TabSummary, got %d", m.activeTab)
	}

	// shift+tab should go backwards.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(Model)
	if m.activeTab != TabStack {
		t.Errorf("shift+tab: want TabStack, got %d", m.activeTab)
	}

	// "h" key should also go backwards; wrap around from 0 to 4.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = next.(Model)
	if m.activeTab != TabProcesses {
		t.Errorf("h from TabStack: want TabProcesses (4), got %d", m.activeTab)
	}
}

// TestHelpToggle verifies that "?" toggles showHelp.
func TestHelpToggle(t *testing.T) {
	m := modelWithSlices(t)

	if m.showHelp {
		t.Fatal("showHelp should start false")
	}

	// First "?" → showHelp = true.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if !m.showHelp {
		t.Error("after first '?': showHelp should be true")
	}

	// Second "?" → showHelp = false.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if m.showHelp {
		t.Error("after second '?': showHelp should be false")
	}
}

// TestStackLoadedMsg verifies that a stackLoadedMsg stores the stacks in the model cache.
func TestStackLoadedMsg(t *testing.T) {
	m := modelWithSlices(t)

	state := gt.State{
		"main": gt.BranchState{Trunk: true},
		"feature-a": gt.BranchState{
			Parents: []gt.Parent{{Ref: "main", SHA: "abc123"}},
		},
	}

	msg := stackLoadedMsg{
		slice:  "feature-a",
		stacks: map[string]gt.State{"web": state},
	}

	next, _ := m.Update(msg)
	m = next.(Model)

	if m.stacks == nil {
		t.Fatal("stacks map should not be nil after stackLoadedMsg")
	}
	repoStates, ok := m.stacks["feature-a"]
	if !ok {
		t.Fatal("stacks['feature-a'] should be present")
	}
	webState, ok := repoStates["web"]
	if !ok {
		t.Fatal("stacks['feature-a']['web'] should be present")
	}
	if _, ok := webState["main"]; !ok {
		t.Error("stacks['feature-a']['web']['main'] should be present")
	}
}

// TestRenderDetailStackTab verifies that renderDetail with TabStack shows tab names
// and branch names from the cached stack.
func TestRenderDetailStackTab(t *testing.T) {
	m := modelWithSlices(t)
	m.focus = 0
	m.activeTab = TabStack

	// Pre-populate the stack cache.
	state := gt.State{
		"main": gt.BranchState{Trunk: true},
		"feature-a": gt.BranchState{
			Parents: []gt.Parent{{Ref: "main", SHA: "abc123"}},
		},
	}
	m.stacks = map[string]map[string]gt.State{
		"feature-a": {"web": state},
	}

	output := renderDetail(m)

	// Tab bar should contain all tab names.
	for _, name := range []string{"Stack", "Summary", "Changes", "Sessions", "Processes"} {
		if !strings.Contains(output, name) {
			t.Errorf("renderDetail should contain tab name %q; output:\n%s", name, output)
		}
	}

	// Stack content should show branch names.
	if !strings.Contains(output, "main") {
		t.Errorf("renderDetail (Stack tab) should contain branch 'main'; output:\n%s", output)
	}
	if !strings.Contains(output, "feature-a") {
		t.Errorf("renderDetail (Stack tab) should contain branch 'feature-a'; output:\n%s", output)
	}
}

// TestRenderDetailPlaceholder verifies non-implemented tabs show a placeholder message.
// (TabSessions is now implemented — this test uses TabSummary instead.)
func TestRenderDetailPlaceholder(t *testing.T) {
	m := modelWithSlices(t)
	m.activeTab = TabSummary

	output := renderDetail(m)

	if !strings.Contains(output, "Summary") {
		t.Errorf("placeholder should mention tab name 'Summary'; output:\n%s", output)
	}
	if !strings.Contains(output, "coming soon") {
		t.Errorf("placeholder should mention 'coming soon'; output:\n%s", output)
	}
}
