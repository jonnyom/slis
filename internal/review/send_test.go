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
	gotSlice   string
	gotPrompt  string
	sendCalled bool
}

func (f *fakeSession) Exists(string) bool { return f.exists }
func (f *fakeSession) SendPrompt(slice, prompt string) error {
	f.sendCalled = true
	f.gotSlice = slice
	f.gotPrompt = prompt
	return nil
}

func TestSendEmptyBatch(t *testing.T) {
	f := &fakeSession{exists: true}
	if err := Send("s", nil, f); !errors.Is(err, ErrNoComments) {
		t.Errorf("Send(empty) = %v, want ErrNoComments", err)
	}
	if f.sendCalled {
		t.Error("Send injected an empty batch")
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
	f := &fakeSession{exists: true}
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
	if err := sess.SendPrompt(slice, prompt); err != nil {
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
