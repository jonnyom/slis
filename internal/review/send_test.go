package review

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/tmuxctl"
)

// fakeSession records what Send injects and controls whether a session exists.
type fakeSession struct {
	exists     bool
	hasAgent   bool
	gotSlice   string
	gotPrompt  string
	sendCalled bool
}

func (f *fakeSession) Exists(string) bool   { return f.exists }
func (f *fakeSession) HasAgent(string) bool { return f.hasAgent }
func (f *fakeSession) SendPrompt(slice, prompt string) error {
	f.sendCalled = true
	f.gotSlice = slice
	f.gotPrompt = prompt
	return nil
}

func TestSendEmptyBatch(t *testing.T) {
	f := &fakeSession{exists: true, hasAgent: true}
	if err := Send("s", nil, f); !errors.Is(err, ErrNoComments) {
		t.Errorf("Send(empty) = %v, want ErrNoComments", err)
	}
	if f.sendCalled {
		t.Error("Send injected an empty batch")
	}
}

func TestSendNoAgentInActivePane(t *testing.T) {
	f := &fakeSession{exists: true, hasAgent: false}
	c := []Comment{{Slice: "s", Repo: "web", File: "a.go", Line: 1, Body: "x"}}
	if err := Send("s", c, f); !errors.Is(err, ErrNoAgent) {
		t.Errorf("Send(no agent) = %v, want ErrNoAgent", err)
	}
	if f.sendCalled {
		t.Error("Send injected into a shell-only pane")
	}
}

func TestSendNoSession(t *testing.T) {
	f := &fakeSession{exists: false}
	c := []Comment{{Slice: "s", Repo: "web", File: "a.go", Line: 1, Body: "x"}}
	if err := Send("s", c, f); !errors.Is(err, ErrNoSession) {
		t.Errorf("Send(no session) = %v, want ErrNoSession", err)
	}
	if f.sendCalled {
		t.Error("Send injected despite no session")
	}
}

func TestSendInjectsComposedPrompt(t *testing.T) {
	f := &fakeSession{exists: true, hasAgent: true}
	c := []Comment{{Slice: "checkout", Repo: "web", File: "a.go", Line: 3, Body: "tidy up"}}
	if err := Send("checkout", c, f); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if !f.sendCalled {
		t.Fatal("Send did not inject")
	}
	if f.gotSlice != "checkout" {
		t.Errorf("injected slice = %q, want checkout", f.gotSlice)
	}
	if f.gotPrompt != ComposePrompt(c) {
		t.Errorf("injected prompt = %q, want the composed prompt", f.gotPrompt)
	}
}

func TestCommandLineMatchesAgent(t *testing.T) {
	tests := []struct {
		name, current, cmdline string
		agents                 []string
		want                   bool
	}{
		{"direct claude", "claude", "claude --resume", []string{"claude"}, true},
		{"node claude package", "node", "node /opt/@anthropic-ai/claude-code/cli.js", []string{"claude"}, true},
		{"configured codex", "codex", "/opt/bin/codex", []string{"claude", "codex"}, true},
		{"plain shell", "zsh", "-zsh", []string{"claude", "codex"}, false},
		{"editor is not agent", "code", "code review feedback on slice", []string{"claude", "codex"}, false},
		{"unrelated node", "node", "node server.js", []string{"claude", "codex"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandLineMatchesAgent(tt.current, tt.cmdline, tt.agents); got != tt.want {
				t.Errorf("commandLineMatchesAgent(%q, %q, %v) = %v, want %v", tt.current, tt.cmdline, tt.agents, got, tt.want)
			}
		})
	}
}

func TestAgentInputBlocker(t *testing.T) {
	if got := agentInputBlocker("2 new MCP servers found in this project\nSelect any you wish to enable."); !strings.Contains(got, "MCP") {
		t.Fatalf("agentInputBlocker did not detect MCP approval: %q", got)
	}
	if got := agentInputBlocker("Claude Code\n> ready for a prompt"); got != "" {
		t.Fatalf("agentInputBlocker rejected a ready prompt: %q", got)
	}
}

// TestTmuxSessionRoundTrip exercises the real tmux-backed delivery: create a
// detached session, inject a multiline prompt via SendPrompt, and confirm the
// pane received the text (paste-buffer path). Skips when tmux is absent.
func TestTmuxSessionRoundTrip(t *testing.T) {
	if !tmuxctl.Available() {
		t.Skip("tmux not on PATH")
	}
	slice := "review-send-test"
	// A pane running `cat` echoes what is pasted so we can capture it.
	if err := tmuxctl.EnsureSession(slice, nil, tmuxctl.SessionOpts{}); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}
	t.Cleanup(func() { _ = tmuxctl.KillSession(slice) })

	var sess TmuxSession
	if !sess.Exists(slice) {
		t.Fatal("session should exist after EnsureSession")
	}

	if err := exec.Command("tmux", "send-keys", "-t", tmuxctl.SessionName(slice), "cat", "Enter").Run(); err != nil {
		t.Fatalf("start cat: %v", err)
	}

	prompt := "line one\nline two\nline three"
	if err := tmuxctl.SendPrompt(slice, prompt); err != nil {
		t.Fatalf("SendPrompt: %v", err)
	}

	out, err := tmuxctl.CapturePane(slice)
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	for _, want := range []string{"line one", "line two", "line three"} {
		if !strings.Contains(out, want) {
			t.Errorf("pane capture missing %q; got:\n%s", want, out)
		}
	}
}
