// Package git provides an injection-proof argv builder and an exec wrapper
// for running git commands against a specific directory.
package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout bounds a git invocation made via Run. Every git command slis
// issues is a fast, local, read-mostly operation, so a generous ceiling still
// guarantees a hung subprocess (e.g. a credential prompt) can never wedge a
// caller or hold a background concurrency slot forever.
const DefaultTimeout = 60 * time.Second

// Cmd builds a git sub-command argv vector incrementally.
type Cmd struct{ args []string }

// NewCmd starts a new Cmd for the given git sub-command (e.g. "switch").
func NewCmd(sub string) *Cmd { return &Cmd{args: []string{sub}} }

// Arg appends a single argument and returns the receiver for chaining.
func (c *Cmd) Arg(a string) *Cmd { c.args = append(c.args, a); return c }

// ArgIf conditionally appends a single argument.
func (c *Cmd) ArgIf(cond bool, a string) *Cmd {
	if cond {
		c.args = append(c.args, a)
	}
	return c
}

// Argv returns a copy of the accumulated argument slice.
// Because each element is passed directly to exec.Command the vector is
// immune to shell-injection: a user-supplied string like "; rm -rf /" is
// always a single literal argument, never interpreted by a shell.
func (c *Cmd) Argv() []string { return append([]string(nil), c.args...) }

// Run executes `git -C dir <args...>` and returns trimmed stdout. It applies
// DefaultTimeout; callers needing their own deadline use RunCtx.
func Run(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	return RunCtx(ctx, dir, args...)
}

// LocalBranches returns the local branch names (refs/heads) in the repo at dir,
// in git's default (alphabetical) order.
func LocalBranches(dir string) ([]string, error) {
	out, err := Run(dir, "for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			names = append(names, s)
		}
	}
	return names, nil
}

// RunCtx is like Run but accepts a context for cancellation / timeouts.
func RunCtx(ctx context.Context, dir string, args ...string) (string, error) {
	full := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, errb.String())
	}
	return strings.TrimRight(out.String(), "\n"), nil
}
