package review

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// ErrNoComments is returned by Send when the batch is empty — there is nothing
// to deliver.
var ErrNoComments = errors.New("no pending review comments to send")

// ErrNoSession is returned by Send when the slice has no running session to
// inject the prompt into. The caller decides what to do (e.g. tell the user to
// start one) — Send never starts an agent on its own.
var ErrNoSession = errors.New("slice has no running session")

// ErrNoAgent is returned when the session exists but its active pane is not
// running one of the configured coding agents. This prevents a review prompt
// from being pasted into a shell or unrelated foreground process.
var ErrNoAgent = errors.New("slice active pane has no configured agent")

// ErrAgentNotReady means the configured agent process exists, but its visible
// pane is waiting at an interactive startup gate rather than accepting prompts.
var ErrAgentNotReady = errors.New("slice agent is waiting for startup input")

// Session is the delivery target for a composed review prompt: it reports
// whether a slice's session exists and injects the prompt into it. Abstracted so
// Send is testable without tmux; the production implementation is TmuxSession.
type Session interface {
	Exists(slice string) bool
	HasAgent(slice string) bool
	SendPrompt(slice, prompt string) error
}

// Send composes the review batch into an agent prompt and injects it into the
// slice's session. It returns ErrNoComments for an empty batch and ErrNoSession
// when no session exists (leaving the pending comments untouched so the caller
// can retry after starting one). It never auto-starts an agent.
func Send(slice string, comments []Comment, sess Session) error {
	if len(comments) == 0 {
		return ErrNoComments
	}
	if !sess.Exists(slice) {
		return ErrNoSession
	}
	if !sess.HasAgent(slice) {
		return ErrNoAgent
	}
	return sess.SendPrompt(slice, ComposePrompt(comments))
}

// TmuxSession is the production Session backed by the slice's tmux session.
type TmuxSession struct {
	// AgentCommands holds the configured agent executables (e.g. claude,
	// codex, aider) that are valid delivery targets.
	AgentCommands []string
}

// Exists reports whether the slice's tmux session is live.
func (TmuxSession) Exists(slice string) bool { return tmuxctl.SessionExists(slice) }

// commandLineMatchesAgent reports whether the active command or one process
// command line in its pane tree belongs to a configured agent. Script-backed
// CLIs are covered (for example `node .../@anthropic-ai/claude-code/cli.js`).
func commandLineMatchesAgent(current, cmdline string, agents []string) bool {
	current = strings.ToLower(filepath.Base(current))
	line := strings.ToLower(cmdline)
	for _, configured := range agents {
		fields := strings.Fields(configured)
		if len(fields) == 0 {
			continue
		}
		agent := strings.ToLower(filepath.Base(fields[0]))
		if agent == "" {
			continue
		}
		if current == agent {
			return true
		}
		for _, field := range strings.Fields(line) {
			field = strings.Trim(field, "'\"")
			if strings.ToLower(filepath.Base(field)) == agent {
				return true
			}
		}
		if strings.Contains(line, "/"+agent+"/") ||
			strings.Contains(line, "/"+agent+"-") ||
			strings.Contains(line, "@"+agent+"/") {
			return true
		}
	}
	return false
}

// HasAgent checks only the active pane and its descendants. An agent in another
// window is not sufficient because SendPrompt targets the active pane.
func (s TmuxSession) HasAgent(slice string) bool {
	pid := tmuxctl.ActivePanePID(slice)
	if pid <= 0 {
		return false
	}
	processes, _ := proc.SliceProcs([]int{pid})
	current := tmuxctl.ActivePaneCommand(slice)
	for _, p := range processes {
		if commandLineMatchesAgent(current, p.Cmd, s.AgentCommands) {
			return true
		}
	}
	return commandLineMatchesAgent(current, "", s.AgentCommands)
}

// ActivateAgent finds a configured agent anywhere in the slice session and
// makes its pane the active delivery target. It returns false when none exists.
func (s TmuxSession) ActivateAgent(slice string) bool {
	panes, err := tmuxctl.Panes(slice)
	if err != nil {
		return false
	}
	for _, pane := range panes {
		processes, _ := proc.SliceProcs([]int{pane.PID})
		for _, p := range processes {
			if commandLineMatchesAgent(pane.Command, p.Cmd, s.AgentCommands) {
				return tmuxctl.SelectPane(pane.Target) == nil
			}
		}
		if commandLineMatchesAgent(pane.Command, "", s.AgentCommands) {
			return tmuxctl.SelectPane(pane.Target) == nil
		}
	}
	return false
}

// SendPrompt injects the prompt via tmux (bracketed paste + Enter).
func (s TmuxSession) SendPrompt(slice, prompt string) error {
	// Re-check immediately before paste to close the most likely race where an
	// agent exits after Send's initial readiness check.
	if !s.HasAgent(slice) {
		return ErrNoAgent
	}
	if pane, err := tmuxctl.CaptureActivePane(slice); err == nil {
		if blocker := agentInputBlocker(pane); blocker != "" {
			return fmt.Errorf("%w: %s", ErrAgentNotReady, blocker)
		}
	}
	return tmuxctl.SendPrompt(slice, prompt)
}

// agentInputBlocker recognizes agent setup screens that require a human choice.
// Sending a review into one can either lose the prompt or accidentally answer a
// security question, so delivery must stop and preserve the pending batch.
func agentInputBlocker(pane string) string {
	lower := strings.ToLower(pane)
	blockers := []struct {
		needle string
		label  string
	}{
		{"new mcp servers found in this project", "Claude is waiting for MCP server approval"},
		{"select any you wish to enable", "Claude is waiting for MCP server selection"},
		{"do you trust the files in this folder", "the agent is waiting for folder trust approval"},
		{"trust this folder", "the agent is waiting for folder trust approval"},
		{"choose how you want to log in", "the agent is waiting for login"},
	}
	for _, blocker := range blockers {
		if strings.Contains(lower, blocker.needle) {
			return blocker.label
		}
	}
	return ""
}
