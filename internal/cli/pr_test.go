package cli

import (
	"runtime"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/forge"
)

// TestRenderPRTable verifies the human-readable table output contains expected columns.
func TestRenderPRTable(t *testing.T) {
	rows := []prRow{
		{
			repo:     "api",
			branch:   "feat/new-endpoint",
			pr:       &forge.PR{Number: 123, URL: "https://github.com/acme/api/pull/123", Title: "feat: new endpoint", Checks: []forge.Check{{Name: "ci", State: forge.CheckPass}}, Comments: nil},
			overall:  forge.CheckPass,
			pass:     1,
			fail:     0,
			pending:  0,
			comments: 0,
		},
		{
			repo:     "web",
			branch:   "feat/ui-update",
			pr:       nil,
			overall:  forge.CheckPending,
			pass:     0,
			fail:     0,
			pending:  0,
			comments: 0,
		},
		{
			repo:     "ops",
			branch:   "feat/infra-fix",
			pr:       &forge.PR{Number: 456, URL: "https://github.com/acme/ops/pull/456", Title: "fix: infra issue", Checks: []forge.Check{{Name: "ci", State: forge.CheckFail}}, Comments: []forge.Comment{{Author: "alice", Body: "please fix"}}},
			overall:  forge.CheckFail,
			pass:     0,
			fail:     1,
			pending:  0,
			comments: 1,
		},
	}

	out := renderPRTable(rows)

	// Repo names present
	if !strings.Contains(out, "api") {
		t.Error("output missing repo 'api'")
	}
	if !strings.Contains(out, "web") {
		t.Error("output missing repo 'web'")
	}
	if !strings.Contains(out, "ops") {
		t.Error("output missing repo 'ops'")
	}

	// PR numbers
	if !strings.Contains(out, "#123") {
		t.Error("output missing '#123'")
	}
	if !strings.Contains(out, "#456") {
		t.Error("output missing '#456'")
	}

	// No-PR row shows dash
	if !strings.Contains(out, "-") {
		t.Error("output missing '-' for no-PR row")
	}

	// CI status words
	if !strings.Contains(out, "pass") {
		t.Error("output missing 'pass' CI word")
	}
	if !strings.Contains(out, "fail") {
		t.Error("output missing 'fail' CI word")
	}

	// Comment count for ops
	if !strings.Contains(out, "1") {
		t.Error("output missing comment count '1' for ops row")
	}
}

// TestClipboardArgvDarwin verifies clipboardArgv returns pbcopy on darwin.
func TestClipboardArgvDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only test")
	}
	name, _, ok := clipboardArgv()
	if !ok {
		t.Fatal("clipboardArgv returned ok=false on darwin")
	}
	if name != "pbcopy" {
		t.Errorf("clipboardArgv name = %q, want %q", name, "pbcopy")
	}
}
