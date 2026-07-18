package review

import (
	"errors"

	"github.com/jonnyom/slis/internal/tmuxctl"
)

// ErrNoComments is returned by Send when the batch is empty — there is nothing
// to deliver.
var ErrNoComments = errors.New("no pending review comments to send")

// ErrNoSession is returned by Send when the slice has no running session to
// inject the prompt into. The caller decides what to do (e.g. tell the user to
// start one) — Send never starts an agent on its own.
var ErrNoSession = errors.New("slice has no running session")

// Session is the delivery target for a composed review prompt: it reports
// whether a slice's session exists and injects the prompt into it. Abstracted so
// Send is testable without tmux; the production implementation is TmuxSession.
type Session interface {
	Exists(slice string) bool
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
	return sess.SendPrompt(slice, ComposePrompt(comments))
}

// TmuxSession is the production Session backed by the slice's tmux session.
type TmuxSession struct{}

// Exists reports whether the slice's tmux session is live.
func (TmuxSession) Exists(slice string) bool { return tmuxctl.SessionExists(slice) }

// SendPrompt injects the prompt via tmux (bracketed paste + Enter).
func (TmuxSession) SendPrompt(slice, prompt string) error {
	return tmuxctl.SendPrompt(slice, prompt)
}
