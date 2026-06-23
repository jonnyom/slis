package cli

import (
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/forge"
)

// TestFixCIPrompt verifies that fixCIPrompt includes the PR number, URL, and
// the name of the failing check — but does NOT require the passing check.
func TestFixCIPrompt(t *testing.T) {
	pr := &forge.PR{
		Number: 7,
		URL:    "u",
		Checks: []forge.Check{
			{Name: "build", State: forge.CheckFail, URL: "cu"},
			{Name: "ok", State: forge.CheckPass, URL: "cu2"},
		},
	}

	got := fixCIPrompt("web", pr)

	if !strings.Contains(got, "7") {
		t.Errorf("fixCIPrompt: expected PR number %q in output, got:\n%s", "7", got)
	}
	if !strings.Contains(got, "u") {
		t.Errorf("fixCIPrompt: expected PR URL %q in output, got:\n%s", "u", got)
	}
	if !strings.Contains(got, "build") {
		t.Errorf("fixCIPrompt: expected failing check name %q in output, got:\n%s", "build", got)
	}
}

// TestFixCIPromptNoFailing verifies that fixCIPrompt does not panic when there
// are no failing checks. The command only calls it for PRs with failures, so
// we just ensure it returns a non-empty string.
func TestFixCIPromptNoFailing(t *testing.T) {
	pr := &forge.PR{
		Number: 1,
		URL:    "https://example.com/pull/1",
		Checks: []forge.Check{
			{Name: "lint", State: forge.CheckPass, URL: "https://example.com/run/1"},
			{Name: "test", State: forge.CheckPass, URL: "https://example.com/run/2"},
		},
	}

	got := fixCIPrompt("api", pr)

	if got == "" {
		t.Error("fixCIPrompt: expected a non-empty string for a PR with no failing checks")
	}
}
