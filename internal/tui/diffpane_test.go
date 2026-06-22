package tui

import (
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/model"
)

// TestSliceBase verifies that sliceBase returns "main" when Base is empty and
// returns the actual Base value when set.
func TestSliceBase(t *testing.T) {
	if got := sliceBase(model.Slice{}); got != "main" {
		t.Errorf("sliceBase(empty): want %q, got %q", "main", got)
	}
	if got := sliceBase(model.Slice{Base: "develop"}); got != "develop" {
		t.Errorf("sliceBase(develop): want %q, got %q", "develop", got)
	}
}

// TestCombinedPatch verifies that combinedPatch includes each repo's name header
// and its Patch content.
func TestCombinedPatch(t *testing.T) {
	diffs := []diff.RepoDiff{
		{Repo: "alpha", Patch: "PA"},
		{Repo: "beta", Patch: "PB"},
	}
	out := combinedPatch(diffs)
	if !strings.Contains(out, "alpha") {
		t.Error("combinedPatch: expected repo name 'alpha' in output")
	}
	if !strings.Contains(out, "PA") {
		t.Error("combinedPatch: expected patch 'PA' in output")
	}
	if !strings.Contains(out, "beta") {
		t.Error("combinedPatch: expected repo name 'beta' in output")
	}
	if !strings.Contains(out, "PB") {
		t.Error("combinedPatch: expected patch 'PB' in output")
	}
}

// TestColorizePatchFallback verifies that colorizePatch never panics and preserves
// the meaningful text tokens regardless of whether chroma succeeds or falls back.
func TestColorizePatchFallback(t *testing.T) {
	patch := "+added\n-removed\n@@ hunk @@\n context"
	out := colorizePatch(patch)
	if out == "" {
		t.Fatal("colorizePatch returned empty string")
	}
	for _, token := range []string{"added", "removed", "hunk", "context"} {
		if !strings.Contains(out, token) {
			t.Errorf("colorizePatch: expected token %q in output", token)
		}
	}
}

// TestDiffLoadedMsg verifies that feeding a diffLoadedMsg into Update stores
// the diffs in m.diffs and clears m.diffLoading.
func TestDiffLoadedMsg(t *testing.T) {
	m := New(config.Workspace{})
	m.slices = []model.Slice{{Name: "s", Members: map[string]model.SliceMember{}}}
	m.loading = false

	diffs := []diff.RepoDiff{
		{Repo: "repo1", Patch: "some patch"},
	}

	next, _ := m.Update(diffLoadedMsg{slice: "s", diffs: diffs})
	m = next.(Model)

	if m.diffs == nil {
		t.Fatal("m.diffs should not be nil after diffLoadedMsg")
	}
	got, ok := m.diffs["s"]
	if !ok {
		t.Fatal("m.diffs['s'] should be populated")
	}
	if len(got) != 1 || got[0].Repo != "repo1" {
		t.Errorf("m.diffs['s']: want [{Repo:repo1}], got %v", got)
	}
	if m.diffLoading["s"] {
		t.Error("m.diffLoading['s'] should be false after diffLoadedMsg")
	}
}

// TestExternalEditorCmdAndClipboardCmd verifies:
//   - clipboardCmd returns a non-empty tool name on this platform (darwin)
//   - externalEditorCmd returns ok=true when EDITOR env var is set
func TestExternalEditorCmdAndClipboardCmd(t *testing.T) {
	name, _, ok := clipboardCmd()
	if !ok {
		t.Skip("no clipboard tool available on this platform")
	}
	if name == "" {
		t.Error("clipboardCmd: name should not be empty when ok=true")
	}

	t.Setenv("EDITOR", "vi")
	editorName, _, editorOk := externalEditorCmd()
	if !editorOk {
		t.Error("externalEditorCmd: expected ok=true when EDITOR=vi")
	}
	if editorName == "" {
		t.Error("externalEditorCmd: name should not be empty when ok=true")
	}
}
