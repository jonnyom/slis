package tui

import (
	"fmt"
	"strings"

	"github.com/jonnyom/slis/internal/model"
)

// isClaudeAgent reports whether the configured agent command launches Claude
// (so it's safe to append a --append-system-prompt flag). A non-claude agent is
// left untouched.
func isClaudeAgent(agent string) bool {
	fields := strings.Fields(agent)
	if len(fields) == 0 {
		return false
	}
	bin := fields[0]
	return bin == "claude" || strings.HasSuffix(bin, "/claude")
}

// slisAgentContext is the one-line context describing the slice, injected into
// Claude's system prompt so it knows it's running inside a slis slice spanning
// several repos. Kept to a single line — it's typed into the shell via tmux
// send-keys, and an embedded newline would submit the command early.
func slisAgentContext(sl model.Slice) string {
	repos := sl.Repos() // sorted
	parts := make([]string, 0, len(repos))
	for _, r := range repos {
		parts = append(parts, fmt.Sprintf("%s (%s)", r, sl.Members[r].Branch))
	}
	ctx := fmt.Sprintf("You are running inside slis, a multi-repo worktree cockpit. "+
		"Current slice: %q, spanning %d repo(s): %s. Each repo is a separate git worktree on its "+
		"own branch — keep each commit scoped to the right repo.",
		sl.Name, len(repos), strings.Join(parts, ", "))
	if sl.Active {
		ctx += " This slice is LIVE: swapped into the repos' primary checkouts."
	}
	return ctx
}

// shellSingleQuote wraps s in single quotes for safe shell insertion, escaping
// any embedded single quotes.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// withSlisContext appends a --append-system-prompt flag carrying the slice
// context to a Claude agent command. Non-claude agents are returned unchanged.
func withSlisContext(agent string, sl model.Slice) string {
	if !isClaudeAgent(agent) {
		return agent
	}
	return agent + " --append-system-prompt " + shellSingleQuote(slisAgentContext(sl))
}
