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

func TestSlisEnvPrefix(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	sl := model.Slice{
		Name: "wfm-1",
		Members: map[string]model.SliceMember{
			"web": {Repo: "web", Branch: "jonny/wfm-1", WorktreePath: "/wt/web"},
			"api": {Repo: "api", Branch: "jonny/wfm-1", WorktreePath: "/wt/api"},
		},
		Active: true,
	}

	got := slisEnvPrefix(sl, "/root", "codex")

	for _, want := range []string{
		"SLIS_SLICE='wfm-1'",
		"SLIS_ROOT='/root'",
		"SLIS_ACTIVE='1'",
		"SLIS_HARNESS='codex'",
		"SLIS_TERMINAL_APP='ghostty'",
		// Worktrees are sorted by repo and joined repo=path,repo=path.
		"SLIS_WORKTREES='api=/wt/api,web=/wt/web'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("env prefix missing %q in: %s", want, got)
		}
	}
	if strings.Contains(got, "\n") {
		t.Errorf("env prefix must be single-line: %q", got)
	}

	// Inactive slice → SLIS_ACTIVE='0'.
	sl.Active = false
	if !strings.Contains(slisEnvPrefix(sl, "/root", "claude"), "SLIS_ACTIVE='0'") {
		t.Error("inactive slice should set SLIS_ACTIVE='0'")
	}
}

func TestSlisEnvPrefixQuotesDangerousValues(t *testing.T) {
	sl := model.Slice{
		Name:    "it's a slice",
		Members: map[string]model.SliceMember{},
	}
	got := slisEnvPrefix(sl, "/root", "claude")
	// The single quote in the slice name must be escaped, not left bare.
	if !strings.Contains(got, `SLIS_SLICE='it'\''s a slice'`) {
		t.Errorf("single quote not escaped in env prefix: %s", got)
	}
}

func TestAgentLaunchLine(t *testing.T) {
	sl := model.Slice{
		Name:    "wfm-1",
		Members: map[string]model.SliceMember{"web": {Repo: "web", Branch: "jonny/wfm-1", WorktreePath: "/wt/web"}},
	}

	// claude → env prefix + claude with the append-system-prompt flag.
	claudeLine := agentLaunchLine("claude", sl, "/root", "claude")
	if !strings.HasPrefix(claudeLine, "cd '/wt/web' && ") {
		t.Errorf("claude launch line does not start in the slice root: %s", claudeLine)
	}
	if !strings.Contains(claudeLine, "SLIS_HARNESS='claude'") {
		t.Errorf("claude launch line missing env prefix: %s", claudeLine)
	}
	if !strings.Contains(claudeLine, "claude --append-system-prompt ") {
		t.Errorf("claude launch line missing append flag: %s", claudeLine)
	}

	// codex → env prefix + bare codex (no positional prompt, no append flag).
	codexLine := agentLaunchLine("codex", sl, "/root", "codex")
	if !strings.Contains(codexLine, "SLIS_HARNESS='codex'") {
		t.Errorf("codex launch line missing env prefix: %s", codexLine)
	}
	if strings.Contains(codexLine, "--append-system-prompt") {
		t.Errorf("codex launch line must not carry the claude append flag: %s", codexLine)
	}
	if !strings.HasSuffix(codexLine, " codex") {
		t.Errorf("codex launch line should end with the bare codex command: %s", codexLine)
	}
	if strings.Contains(codexLine, "\n") {
		t.Errorf("launch line must be single-line: %q", codexLine)
	}
}
