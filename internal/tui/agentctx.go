package tui

import (
	"github.com/jonnyom/slis/internal/agentlaunch"
	"github.com/jonnyom/slis/internal/model"
)

// isClaudeAgent reports whether the configured agent command launches Claude
// (so it's safe to append a --append-system-prompt flag). A non-claude agent is
// left untouched.
func isClaudeAgent(agent string) bool {
	return agentlaunch.IsClaude(agent)
}

// withSlisContext appends a --append-system-prompt flag carrying the slice
// context to a Claude agent command. Non-claude agents (e.g. codex, which has no
// such flag) are returned unchanged.
func withSlisContext(agent string, sl model.Slice) string {
	return agentlaunch.WithSliceContext(agent, sl)
}

// slisEnvPrefix builds the inline env-var prefix prepended to the agent launch
// command, so an agent running inside a slis tmux session can trust these
// values. Each value is single-quoted; the whole thing stays on one line (the
// send-keys constraint). SLIS_WORKTREES is a comma-separated repo=path list.
func slisEnvPrefix(sl model.Slice, wsRoot, harness string) string {
	return agentlaunch.EnvPrefix(sl, wsRoot, harness)
}

// agentLaunchLine builds the full one-line send-keys command: the SLIS_* env
// prefix followed by the agent command (with claude's --append-system-prompt
// flag appended for a claude command; nothing extra for codex).
func agentLaunchLine(agent string, sl model.Slice, wsRoot, harness string) string {
	return agentlaunch.Line(agent, sl, wsRoot, harness)
}
