package gt

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// Every gt invocation forks detached "background" helpers — an upgrade check
// and a telemetry flush, each a full Node process (~120MB idle, 600–900MB on a
// swapping machine) that outlives the gt call and escapes slis's spawn cap.
// For slis's own non-interactive reads those helpers are pure overhead, and
// Graphite documents env switches to turn them off.
func TestStateCommandSuppressesGraphiteBackgroundHelpers(t *testing.T) {
	cmd := stateCmd(context.Background(), t.TempDir())

	if got := cmd.Args[1:]; !slices.Equal(got, []string{"state", "--no-interactive"}) {
		t.Fatalf("args = %v, want [state --no-interactive]", got)
	}
	for _, want := range []string{"GRAPHITE_DISABLE_UPGRADE_PROMPT=1", "GRAPHITE_DISABLE_TELEMETRY=1"} {
		if !slices.Contains(cmd.Env, want) {
			t.Errorf("env missing %s; env tail = %v", want, tail(cmd.Env, 4))
		}
	}
}

func TestQuietGraphiteEnvPreservesInheritedEnvironment(t *testing.T) {
	t.Setenv("SLIS_ENV_PROBE", "kept")
	env := quietGraphiteEnv()

	if !slices.Contains(env, "SLIS_ENV_PROBE=kept") {
		t.Fatalf("inherited variable dropped: %v", tail(env, 6))
	}
	var path bool
	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			path = true
		}
	}
	if !path {
		t.Fatal("PATH not inherited — gt could not find git")
	}
}

func tail(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
