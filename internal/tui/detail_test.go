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

// TestPanelFocusCycling verifies that tab/shift+tab cycle the cockpit's focused
// panel (mod panelCount) once a slice is open.
func TestPanelFocusCycling(t *testing.T) {
	m := modelWithSlices(t)
	m.view = viewCockpit
	m.focus = 0

	if m.panel != panelStack {
		t.Fatalf("initial panel should be panelStack (0), got %d", m.panel)
	}

	for i := 1; i <= int(panelCount); i++ {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
		m = next.(Model)
		want := panel(i % int(panelCount))
		if m.panel != want {
			t.Errorf("after %d tab presses: want panel=%d, got %d", i, want, m.panel)
		}
	}

	// shift+tab goes backwards (wrap from panelStack to the last panel).
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = next.(Model)
	if m.panel != panelProcs {
		t.Errorf("shift+tab from panelStack: want panelProcs (%d), got %d", panelProcs, m.panel)
	}
}

// TestEnterAndBackNavigation verifies that Enter on a slice opens the cockpit
// (focused on the Stack panel) and Esc returns to the browser.
func TestEnterAndBackNavigation(t *testing.T) {
	m := modelWithSlices(t)
	m.focus = 0

	if m.view != viewBrowser {
		t.Fatalf("initial view should be browser, got %d", m.view)
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.view != viewCockpit {
		t.Fatalf("Enter should switch to cockpit, got view=%d", m.view)
	}
	if m.panel != panelStack {
		t.Errorf("cockpit should open focused on panelStack, got %d", m.panel)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.view != viewBrowser {
		t.Errorf("Esc should return to browser, got view=%d", m.view)
	}
}

// TestHelpToggle verifies that "?" opens help and "?"/esc closes it.
func TestHelpToggle(t *testing.T) {
	m := modelWithSlices(t)

	if m.showHelp {
		t.Fatal("showHelp should start false")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m = next.(Model)
	if !m.showHelp {
		t.Error("after first '?': showHelp should be true")
	}

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
		"main":      gt.BranchState{Trunk: true},
		"feature-a": gt.BranchState{Parents: []gt.Parent{{Ref: "main", SHA: "abc123"}}},
	}

	next, _ := m.Update(stackLoadedMsg{
		slice:  "feature-a",
		stacks: map[string]gt.State{"web": state},
	})
	m = next.(Model)

	if m.stacks == nil {
		t.Fatal("stacks map should not be nil after stackLoadedMsg")
	}
	repoStates, ok := m.stacks["feature-a"]
	if !ok {
		t.Fatal("stacks['feature-a'] should be present")
	}
	if _, ok := repoStates["web"]["main"]; !ok {
		t.Error("stacks['feature-a']['web']['main'] should be present")
	}
}

// TestStackPanelContent verifies the cockpit Stack panel shows the slice's
// branch lineage (its branch + trunk) for each repo.
func TestStackPanelContent(t *testing.T) {
	m := modelWithSlices(t)
	m.view = viewCockpit
	m.focus = 0

	state := gt.State{
		"main":      gt.BranchState{Trunk: true},
		"feature-a": gt.BranchState{Parents: []gt.Parent{{Ref: "main", SHA: "abc123"}}},
		// An unrelated branch that must NOT appear in feature-a's lineage.
		"other": gt.BranchState{Parents: []gt.Parent{{Ref: "main"}}},
	}
	m.stacks = map[string]map[string]gt.State{"feature-a": {"web": state}}

	out := stackPanelContent(m, m.slices[0])

	if !strings.Contains(out, "web") {
		t.Errorf("stack panel should contain repo 'web'; got:\n%s", out)
	}
	if !strings.Contains(out, "main") {
		t.Errorf("stack panel should contain trunk 'main'; got:\n%s", out)
	}
	if !strings.Contains(out, "feature-a") {
		t.Errorf("stack panel should contain 'feature-a'; got:\n%s", out)
	}
	if strings.Contains(out, "other") {
		t.Errorf("stack panel should NOT contain unrelated branch 'other'; got:\n%s", out)
	}
}

// TestSummaryContent verifies summaryContent shows a loading state with no cache
// and the cached text (plus the AI hint) once present.
func TestSummaryContent(t *testing.T) {
	m := modelWithSlices(t)
	sl := m.slices[0]

	if out := summaryContent(m, sl); !strings.Contains(out, "loading") {
		t.Errorf("summaryContent with no cache should show 'loading'; got:\n%s", out)
	}

	m.summaries["feature-a"] = "## web\n\n- feat: hello\n"
	out := summaryContent(m, sl)
	if !strings.Contains(out, "feat: hello") {
		t.Errorf("summaryContent should show cached text; got:\n%s", out)
	}
	if !strings.Contains(out, "[s]") {
		t.Errorf("summaryContent should show '[s] AI summary' hint; got:\n%s", out)
	}
}
