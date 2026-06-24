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
// several repos. The agent's cwd is the workspace root, where both the primary
// checkouts and the slice worktrees live — so the context must point it at the
// worktree paths and forbid the primaries, otherwise it edits the wrong tree.
// Kept to a single line — it's typed into the shell via tmux send-keys, and an
// embedded newline would submit the command early.
func slisAgentContext(sl model.Slice) string {
	repos := sl.Repos() // sorted
	parts := make([]string, 0, len(repos))
	for _, r := range repos {
		m := sl.Members[r]
		if m.WorktreePath != "" {
			parts = append(parts, fmt.Sprintf("%s → %s (branch %s)", r, m.WorktreePath, m.Branch))
		} else {
			parts = append(parts, fmt.Sprintf("%s (branch %s)", r, m.Branch))
		}
	}
	ctx := fmt.Sprintf("You are running inside slis, a multi-repo worktree cockpit, working on slice %q "+
		"which spans %d repo(s). Make ALL your edits inside this slice's git worktrees, listed here — "+
		"do NOT touch the repos' primary checkouts: %s. Each repo is a separate worktree on its own "+
		"branch; cd into the right worktree for each repo and keep every commit scoped to that worktree.",
		sl.Name, len(repos), strings.Join(parts, "; "))
	if sl.Active {
		ctx += " (This slice is also LIVE — swapped into the primary checkouts so dev servers build it — " +
			"but still make every edit in the worktrees above, never the primaries.)"
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
