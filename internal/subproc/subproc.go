// Package subproc makes cancellation of a slis-spawned subprocess actually
// stick. Go's exec.CommandContext, on its own, is not enough for the tools slis
// drives (git, gt, gh): they spawn children of their own, and
//
//   - the default cancel func signals ONLY the direct child, so a cancelled `gt`
//     leaves its `git` grandchildren running — exactly the runaway-process shape
//     slis has to guarantee cannot happen; and
//   - Wait does not return until the goroutines copying the command's stdout /
//     stderr into a caller's buffer see EOF, which a surviving grandchild holding
//     the pipe write end can defer forever. A killed command could therefore hang
//     its caller (and keep a concurrency slot) indefinitely.
//
// Configure fixes both: the child leads its own process group, cancellation
// signals the whole group, and Wait gives up on the pipes shortly after.
package subproc

import (
	"os/exec"
	"time"
)

// WaitDelay bounds how long Wait keeps reading a killed command's pipes before
// giving up and returning. Long enough for a well-behaved tool to flush its
// output, short enough that a wedged grandchild cannot hold the caller.
const WaitDelay = 2 * time.Second

// Configure prepares cmd so that cancelling its context (or its timeout firing)
// tears down the whole process tree rather than just the direct child. Call it
// after exec.CommandContext and before Start/Run:
//
//	cmd := exec.CommandContext(ctx, "gt", "state")
//	subproc.Configure(cmd)
//
// It is a no-op for a cmd built without a context (there is nothing to cancel)
// and on platforms with no process groups.
func Configure(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.WaitDelay = WaitDelay
	setProcessGroup(cmd)
	if cmd.Cancel == nil {
		return
	}
	cmd.Cancel = func() error { return killGroup(cmd) }
}
