package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// prTestModel returns a model pre-loaded with one slice "s" containing
// member "web" with a cached stack.
func prTestModel(t *testing.T) Model {
	t.Helper()
	m := New(config.Workspace{})
	m.slices = []model.Slice{
		{
			Name: "s",
			Members: map[string]model.SliceMember{
				"web": {Repo: "web", Branch: "feature-s", WorktreePath: "/tmp/web"},
			},
		},
	}
	m.loading = false
	m.view = viewCockpit
	// Pre-populate a stack so the cockpit panels don't return "loading…"
	m.stacks = map[string]map[string]gt.State{
		"s": {
			"web": gt.State{
				"main":      gt.BranchState{Trunk: true},
				"feature-s": gt.BranchState{Parents: []gt.Parent{{Ref: "main", SHA: "abc"}}},
			},
		},
	}
	return m
}

// TestPrsLoadedMsg verifies that feeding prsLoadedMsg into Update stores PRs and
// clears the loading flag.
func TestPrsLoadedMsg(t *testing.T) {
	m := prTestModel(t)
	m.prLoading = map[string]bool{"s": true}

	pr := &forge.PR{
		Branch: "feature-s",
		Number: 5,
		URL:    "https://github.com/example/web/pull/5",
		State:  "OPEN",
	}

	next, cmd := m.Update(prsLoadedMsg{
		slice: "s",
		prs:   map[string]*forge.PR{"web": pr},
	})
	m = next.(Model)

	if cmd != nil {
		t.Errorf("after prsLoadedMsg: want nil cmd, got non-nil")
	}

	slicePRs, ok := m.prs["s"]
	if !ok {
		t.Fatal("m.prs['s'] should be present after prsLoadedMsg")
	}
	webPR, ok := slicePRs["web"]
	if !ok {
		t.Fatal("m.prs['s']['web'] should be present after prsLoadedMsg")
	}
	if webPR.Number != 5 {
		t.Errorf("m.prs['s']['web'].Number = %d, want 5", webPR.Number)
	}
	if m.prLoading["s"] {
		t.Error("m.prLoading['s'] should be false after prsLoadedMsg")
	}
}

// TestPRsPanelWithPR verifies the cockpit PRs panel shows the PR number, a CI
// emoji, and the comment-count bubble when PRs are cached.
func TestPRsPanelWithPR(t *testing.T) {
	m := prTestModel(t)

	// PR #9, one failing check, 2 comments.
	pr := &forge.PR{
		Branch:   "feature-s",
		Number:   9,
		URL:      "https://github.com/example/web/pull/9",
		State:    "OPEN",
		Checks:   []forge.Check{{Name: "ci", State: forge.CheckFail}},
		Comments: []forge.Comment{{Author: "alice", Body: "please fix"}, {Author: "bob", Body: "agreed"}},
	}

	m.prs = map[string]map[string]*forge.PR{
		"s": {"web": pr},
	}

	output := prsPanelContent(m, m.slices[0])

	if !strings.Contains(output, "#9") {
		t.Errorf("prsPanelContent: expected '#9' in output; got:\n%s", output)
	}
	if !strings.Contains(output, "❌") {
		t.Errorf("prsPanelContent: expected fail CI emoji '❌' in output; got:\n%s", output)
	}
	if !strings.Contains(output, "💬") {
		t.Errorf("prsPanelContent: expected comment bubble '💬' in output; got:\n%s", output)
	}
}

// TestPRsPanelNoPR verifies that when PRs are loaded but a repo has none,
// "(no PR)" is shown.
func TestPRsPanelNoPR(t *testing.T) {
	m := prTestModel(t)

	m.prs = map[string]map[string]*forge.PR{
		"s": {"web": nil},
	}

	output := prsPanelContent(m, m.slices[0])

	if !strings.Contains(output, "(no PR)") {
		t.Errorf("prsPanelContent: expected '(no PR)' when PR is nil; got:\n%s", output)
	}
}

// TestPRsPanelLoading verifies that while PRs are loading the panel shows
// "PR: loading…".
func TestPRsPanelLoading(t *testing.T) {
	m := prTestModel(t)
	m.prLoading = map[string]bool{"s": true}

	output := prsPanelContent(m, m.slices[0])

	if !strings.Contains(output, "PR: loading") {
		t.Errorf("prsPanelContent: expected 'PR: loading' while prLoading; got:\n%s", output)
	}
}

// TestCommentsOverlayToggle verifies that pressing "c" opens the overlay and
// pressing "c" again (or esc) closes it.
func TestCommentsOverlayToggle(t *testing.T) {
	m := prTestModel(t)

	if m.showCommentsOverlay {
		t.Fatal("showCommentsOverlay should start false")
	}

	// Press "c" → open overlay.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if !m.showCommentsOverlay {
		t.Error("after 'c': showCommentsOverlay should be true")
	}

	// Press "c" again → close overlay.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	if m.showCommentsOverlay {
		t.Error("after second 'c': showCommentsOverlay should be false")
	}

	// Open again, then press esc.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	m = next.(Model)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(Model)
	if m.showCommentsOverlay {
		t.Error("after esc: showCommentsOverlay should be false")
	}
}

// TestRenderCommentsOverlay verifies that when a slice has a PR with a comment,
// the overlay shows the author name and comment body.
func TestRenderCommentsOverlay(t *testing.T) {
	m := prTestModel(t)

	pr := &forge.PR{
		Branch:   "feature-s",
		Number:   3,
		URL:      "https://github.com/example/web/pull/3",
		State:    "OPEN",
		Comments: []forge.Comment{{Author: "alice", Body: "please fix"}},
	}
	m.prs = map[string]map[string]*forge.PR{
		"s": {"web": pr},
	}
	m.showCommentsOverlay = true

	output := renderCommentsOverlay(m)

	if !strings.Contains(output, "alice") {
		t.Errorf("renderCommentsOverlay: expected 'alice' in output; got:\n%s", output)
	}
	if !strings.Contains(output, "please fix") {
		t.Errorf("renderCommentsOverlay: expected 'please fix' in output; got:\n%s", output)
	}
}

// TestCommentsOverlayScrollKeys verifies j/k adjust commentsSel when the overlay is open.
func TestCommentsOverlayScrollKeys(t *testing.T) {
	m := prTestModel(t)
	m.showCommentsOverlay = true
	m.commentsSel = 0

	// Build a PR with 3 comments to have something to scroll through.
	pr := &forge.PR{
		Number: 1,
		Comments: []forge.Comment{
			{Author: "a1", Body: "c1"},
			{Author: "a2", Body: "c2"},
			{Author: "a3", Body: "c3"},
		},
	}
	m.prs = map[string]map[string]*forge.PR{
		"s": {"web": pr},
	}

	// Press "j" → sel should increase.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = next.(Model)
	if m.commentsSel != 1 {
		t.Errorf("after j in comments overlay: want commentsSel=1, got %d", m.commentsSel)
	}

	// Press "k" → back to 0.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = next.(Model)
	if m.commentsSel != 0 {
		t.Errorf("after k in comments overlay: want commentsSel=0, got %d", m.commentsSel)
	}
}
