package cli

import (
	"os/exec"
	"testing"

	"github.com/jonnyom/slis/internal/config"
)

// TestGtFindingsNotInstalled: with gt absent, the gt section emits a single
// info finding (slis works without Graphite) and no warns/fails.
func TestGtFindingsNotInstalled(t *testing.T) {
	if _, err := exec.LookPath("gt"); err == nil {
		t.Skip("gt installed; this test asserts the gt-absent branch")
	}
	ws := config.Workspace{Repos: map[string]config.Repo{"web": {Primary: "/nonexistent"}}}
	findings := gtFindings(ws, nil)
	if len(findings) != 1 {
		t.Fatalf("findings = %d; want 1: %+v", len(findings), findings)
	}
	if findings[0].Level != lvlInfo {
		t.Errorf("level = %q; want info", findings[0].Level)
	}
}

// TestGtFindingsUninitialisedRepo: with gt installed but a repo it can't read as
// a Graphite repo, the section warns and points at `gt init`.
func TestGtFindingsUninitialisedRepo(t *testing.T) {
	if _, err := exec.LookPath("gt"); err != nil {
		t.Skip("gt not installed")
	}
	// A path that is not a git repo at all is never gt-native.
	ws := config.Workspace{Repos: map[string]config.Repo{"web": {Primary: t.TempDir()}}}
	findings := gtFindings(ws, nil)
	sawWarn := false
	for _, f := range findings {
		if f.Level == lvlWarn {
			sawWarn = true
		}
	}
	if !sawWarn {
		t.Errorf("expected a warn for an uninitialised repo; got %+v", findings)
	}
}
