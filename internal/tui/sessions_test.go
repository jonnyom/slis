package tui

import (
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
)

// TestSessionBadge verifies the glyph returned for each SessionStatus value.
func TestSessionBadge(t *testing.T) {
	cases := []struct {
		status model.SessionStatus
		want   string
	}{
		{model.SessWaitingInput, "⏸"},
		{model.SessRunning, "●"},
		{model.SessDone, "✓"},
		{model.SessNone, "○"},
	}

	for _, tc := range cases {
		got := sessionBadge(tc.status)
		if got != tc.want {
			t.Errorf("sessionBadge(%v): got %q, want %q", tc.status, got, tc.want)
		}
	}
}

// TestSessionsLoadedMsg verifies that feeding a sessionsLoadedMsg stores the
// statuses in m.sessionStatus.
func TestSessionsLoadedMsg(t *testing.T) {
	m := New(config.Workspace{})
	m.loading = false
	m.slices = []model.Slice{
		{Name: "alpha"},
		{Name: "beta"},
	}

	statuses := map[string]model.SessionStatus{
		"alpha": model.SessRunning,
		"beta":  model.SessWaitingInput,
	}

	next, _ := m.Update(sessionsLoadedMsg{statuses: statuses})
	m = next.(Model)

	if m.sessionStatus == nil {
		t.Fatal("sessionStatus should not be nil after sessionsLoadedMsg")
	}
	if got := m.sessionStatus["alpha"]; got != model.SessRunning {
		t.Errorf("alpha: got %v, want SessRunning", got)
	}
	if got := m.sessionStatus["beta"]; got != model.SessWaitingInput {
		t.Errorf("beta: got %v, want SessWaitingInput", got)
	}
}

// TestRenderBrowserShowsSessionBadge verifies that a slice with SessWaitingInput
// shows the ⏸ badge in the browser.
func TestRenderBrowserShowsSessionBadge(t *testing.T) {
	m := New(config.Workspace{})
	m.loading = false
	m.slices = []model.Slice{
		{Name: "alpha"},
	}
	m.sessionStatus = map[string]model.SessionStatus{
		"alpha": model.SessWaitingInput,
	}
	m.focus = 0

	output := renderBrowser(m)

	if !strings.Contains(output, "⏸") {
		t.Errorf("renderBrowser should contain '⏸' badge for SessWaitingInput; got:\n%s", output)
	}
}

// TestSessionDetailContent verifies the cockpit Session detail shows the slice
// name and a status word for the focused slice.
func TestSessionDetailContent(t *testing.T) {
	m := New(config.Workspace{})
	m.loading = false
	m.slices = []model.Slice{
		{
			Name: "feature-login",
			Members: map[string]model.SliceMember{
				"web": {Repo: "web", Branch: "feature-login", WorktreePath: "/tmp/web"},
			},
		},
	}
	m.focus = 0
	m.sessionStatus = map[string]model.SessionStatus{
		"feature-login": model.SessRunning,
	}

	output := sessionDetailContent(m, m.slices[0])

	if !strings.Contains(output, "feature-login") {
		t.Errorf("session detail should contain slice name 'feature-login'; got:\n%s", output)
	}
	if !strings.Contains(output, "running") && !strings.Contains(output, "●") {
		t.Errorf("session detail should contain status info; got:\n%s", output)
	}
}
