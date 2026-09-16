package gt

import (
	"context"
	"os"
	"os/exec"

	"github.com/jonnyom/slis/internal/subproc"
)

// quietGraphiteEnv is the environment for slis's own non-interactive gt spawns.
// Every gt invocation forks detached "background" helpers — `upgrade-prompt`
// and `post-traces` on every call (plus `fetch-pr-info` / `feature-flags` on a
// timer) — each a full Node process that outlives the gt call, escapes the
// spawn cap AND the process group, and idles at ~120MB (600–900MB once the
// machine is swapping). Twenty of them side by side is what a 4-slice cockpit
// looked like in Activity Monitor. Graphite's documented switches turn the
// per-call pair off; for a captured `gt state` poll they are pure overhead.
// The user's interactive gt (slis submit/sync/merge) is left untouched.
func quietGraphiteEnv() []string {
	return append(os.Environ(),
		"GRAPHITE_DISABLE_UPGRADE_PROMPT=1",
		"GRAPHITE_DISABLE_TELEMETRY=1",
	)
}

// stateCmd builds the captured-output `gt state --no-interactive` for repoDir.
func stateCmd(ctx context.Context, repoDir string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "gt", "state", "--no-interactive")
	subproc.Configure(cmd)
	cmd.Dir = repoDir
	cmd.Env = quietGraphiteEnv()
	return cmd
}
