package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/review"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// fakeSession is a test double for review.Session.
type fakeSession struct {
	exists    bool
	hasAgent  bool
	sent      bool
	gotPrompt string
}

func (f *fakeSession) Exists(string) bool   { return f.exists }
func (f *fakeSession) HasAgent(string) bool { return f.hasAgent }
func (f *fakeSession) SendPrompt(_, prompt string) error {
	f.sent = true
	f.gotPrompt = prompt
	return nil
}

func newStore(t *testing.T) *review.Store {
	t.Helper()
	return review.Open(filepath.Join(t.TempDir(), "reviews.json"))
}

func TestRunReviewSendNoComments(t *testing.T) {
	store := newStore(t)
	if _, err := runReviewSend(store, "s", &fakeSession{exists: true, hasAgent: true}, false); err == nil {
		t.Error("runReviewSend with no comments should error")
	}
}

func TestRunReviewSendNoAgentGuidance(t *testing.T) {
	store := newStore(t)
	mustStoreAdd(t, store, review.Comment{Slice: "s", Repo: "web", File: "a.go", Line: 1, Body: "x"})

	_, err := runReviewSend(store, "s", &fakeSession{exists: true}, false)
	if err == nil || !strings.Contains(err.Error(), "no agent is running in its active pane") {
		t.Fatalf("expected active-pane agent guidance, got %v", err)
	}
	if got, _ := store.List("s"); len(got) != 1 {
		t.Errorf("no-agent send dropped pending comments: %d left, want 1", len(got))
	}
}

func TestRunReviewSendNoSessionGuidance(t *testing.T) {
	store := newStore(t)
	mustStoreAdd(t, store, review.Comment{Slice: "s", Repo: "web", File: "a.go", Line: 1, Body: "x"})

	_, err := runReviewSend(store, "s", &fakeSession{exists: false}, false)
	if err == nil {
		t.Fatal("expected a no-session error")
	}
	// Comments are preserved so the user can retry after starting a session.
	if got, _ := store.List("s"); len(got) != 1 {
		t.Errorf("no-session send dropped pending comments: %d left, want 1", len(got))
	}
}

func TestRunReviewSendClearsOnSuccess(t *testing.T) {
	store := newStore(t)
	mustStoreAdd(t, store, review.Comment{Slice: "s", Repo: "web", File: "a.go", Line: 3, Body: "tidy"})
	sess := &fakeSession{exists: true, hasAgent: true}

	n, err := runReviewSend(store, "s", sess, false)
	if err != nil {
		t.Fatalf("runReviewSend: %v", err)
	}
	if n != 1 {
		t.Errorf("delivered %d, want 1", n)
	}
	if !sess.sent {
		t.Error("session never received the prompt")
	}
	if sess.gotPrompt == "" {
		t.Error("injected an empty prompt")
	}
	if got, _ := store.List("s"); len(got) != 0 {
		t.Errorf("pending comments not cleared after send: %d left", len(got))
	}
}

func TestRunReviewSendKeepPreservesComments(t *testing.T) {
	store := newStore(t)
	mustStoreAdd(t, store, review.Comment{Slice: "s", Repo: "web", File: "a.go", Line: 3, Body: "tidy"})

	if _, err := runReviewSend(store, "s", &fakeSession{exists: true, hasAgent: true}, true); err != nil {
		t.Fatalf("runReviewSend: %v", err)
	}
	if got, _ := store.List("s"); len(got) != 1 {
		t.Errorf("--keep should preserve comments: %d left, want 1", len(got))
	}
}

func TestEnsureReviewAgentLaunchesConfiguredAgentFromShell(t *testing.T) {
	if !tmuxctl.Available() {
		t.Skip("tmux not on PATH")
	}
	const slice = "review-autostart-test"
	_ = tmuxctl.KillSession(slice)
	t.Cleanup(func() { _ = tmuxctl.KillSession(slice) })

	worktree := t.TempDir()
	sl := model.Slice{
		Name: slice,
		Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "feature", WorktreePath: worktree},
		},
	}
	ws := config.Workspace{
		Root:     worktree,
		Sessions: config.Sessions{Agent: "sleep 30"},
	}
	sess := review.TmuxSession{AgentCommands: reviewAgentCommands(ws.Sessions)}
	if err := ensureReviewAgent(ws, sl, sess); err != nil {
		t.Fatalf("ensureReviewAgent: %v", err)
	}
	if !sess.HasAgent(slice) {
		t.Fatal("configured agent was not running in the active pane")
	}
}

func TestDefaultReviewAgentUsesFirstConfiguredChoice(t *testing.T) {
	s := config.Sessions{
		Harness: "claude",
		Agents: []config.AgentSpec{
			{Name: "codex", Cmd: []string{"codex", "--full-auto"}},
			{Name: "claude", Cmd: []string{"claude"}},
		},
	}
	cmd, harness := defaultReviewAgent(s)
	if cmd != "codex --full-auto" || harness != "codex" {
		t.Fatalf("defaultReviewAgent = %q/%q, want codex --full-auto/codex", cmd, harness)
	}
}

func mustStoreAdd(t *testing.T, s *review.Store, c review.Comment) {
	t.Helper()
	if _, err := s.Add(c); err != nil {
		t.Fatalf("Add: %v", err)
	}
}
