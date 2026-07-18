// Package agentlaunch builds the command used to start a configured coding
// agent inside a slice's tmux session with the correct worktree context.
package agentlaunch

import (
	"fmt"
	"strings"

	"github.com/jonnyom/slis/internal/model"
)

// IsClaude reports whether agent launches Claude Code.
func IsClaude(agent string) bool {
	fields := strings.Fields(agent)
	if len(fields) == 0 {
		return false
	}
	bin := fields[0]
	return bin == "claude" || strings.HasSuffix(bin, "/claude")
}

// SliceContext describes the slice worktrees for injection into Claude's
// system prompt.
func SliceContext(sl model.Slice) string {
	repos := sl.Repos()
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

// ShellSingleQuote safely wraps one shell argument in single quotes.
func ShellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// WithSliceContext adds Claude's supported system-prompt flag. Other agents
// receive their configured command unchanged.
func WithSliceContext(agent string, sl model.Slice) string {
	if !IsClaude(agent) {
		return agent
	}
	return agent + " --append-system-prompt " + ShellSingleQuote(SliceContext(sl))
}

// EnvPrefix returns the SLIS_* environment passed to the agent process.
func EnvPrefix(sl model.Slice, wsRoot, harness string) string {
	active := "0"
	if sl.Active {
		active = "1"
	}
	repos := sl.Repos()
	pairs := make([]string, 0, len(repos))
	for _, r := range repos {
		pairs = append(pairs, r+"="+sl.Members[r].WorktreePath)
	}
	vars := []string{
		"SLIS_SLICE=" + ShellSingleQuote(sl.Name),
		"SLIS_ROOT=" + ShellSingleQuote(wsRoot),
		"SLIS_ACTIVE=" + ShellSingleQuote(active),
		"SLIS_HARNESS=" + ShellSingleQuote(harness),
		"SLIS_WORKTREES=" + ShellSingleQuote(strings.Join(pairs, ",")),
	}
	return strings.Join(vars, " ")
}

// Line builds the complete one-line command typed into the tmux shell.
func Line(agent string, sl model.Slice, wsRoot, harness string) string {
	return EnvPrefix(sl, wsRoot, harness) + " " + WithSliceContext(agent, sl)
}
