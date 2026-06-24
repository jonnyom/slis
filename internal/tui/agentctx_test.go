package tui

import (
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/model"
)

func TestIsClaudeAgent(t *testing.T) {
	for _, a := range []string{"claude", "claude --resume", "/usr/local/bin/claude", "claude --system-prompt x"} {
		if !isClaudeAgent(a) {
			t.Errorf("isClaudeAgent(%q) = false, want true", a)
		}
	}
	for _, a := range []string{"", "aider", "opencode --foo", "claude-extra"} {
		if isClaudeAgent(a) {
			t.Errorf("isClaudeAgent(%q) = true, want false", a)
		}
	}
}

func TestWithSlisContext(t *testing.T) {
	sl := model.Slice{
		Name: "wfm-1",
		Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "jonny/wfm-1", WorktreePath: "/root/.slis/worktrees/wfm-1/web"},
			"api": {Repo: "api", Branch: "jonny/wfm-1", WorktreePath: "/root/.slis/worktrees/wfm-1/api"},
		},
		Active: true,
	}

	// Non-claude agent is untouched.
	if got := withSlisContext("aider", sl); got != "aider" {
		t.Errorf("non-claude agent changed: %q", got)
	}

	got := withSlisContext("claude", sl)
	if !strings.HasPrefix(got, "claude --append-system-prompt ") {
		t.Fatalf("expected appended flag, got: %q", got)
	}
	for _, want := range []string{
		"wfm-1", "web", "api", "LIVE",
		// The worktree paths must be present so the agent edits the worktrees…
		"/root/.slis/worktrees/wfm-1/web",
		"/root/.slis/worktrees/wfm-1/api",
		// …and it must be told NOT to touch the primaries.
		"primary",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context missing %q in: %s", want, got)
		}
	}
	// Single line — no embedded newline that would submit early via send-keys.
	if strings.Contains(got, "\n") {
		t.Errorf("agent command must be single-line, got newline: %q", got)
	}
}
