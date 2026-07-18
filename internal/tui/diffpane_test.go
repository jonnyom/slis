package tui

import (
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/model"
)

// TestSliceBase verifies that sliceBase returns "" when Base is unset (meaning
// auto-detect per repo) and returns the explicit Base override when set.
func TestSliceBase(t *testing.T) {
	if got := sliceBase(model.Slice{}); got != "" {
		t.Errorf("sliceBase(empty): want %q (auto-detect), got %q", "", got)
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

// TestClipboardCmd verifies clipboardCmd returns a non-empty tool name on this
// platform (editor resolution moved to internal/editor, tested there).
func TestClipboardCmd(t *testing.T) {
	name, _, ok := clipboardCmd()
	if !ok {
		t.Skip("no clipboard tool available on this platform")
	}
	if name == "" {
		t.Error("clipboardCmd: name should not be empty when ok=true")
	}
}

func boolPtr(b bool) *bool { return &b }

// TestScopeFromPrefs covers the diff-scope prefs migration: an explicit
// diff_scope wins; otherwise the legacy diff_vs_trunk bool maps true→trunk,
// false→parent, and an absent value defaults to dirty.
func TestScopeFromPrefs(t *testing.T) {
	cases := []struct {
		name string
		p    config.Prefs
		want diffScope
	}{
		{"absent → dirty", config.Prefs{}, scopeDirty},
		{"explicit dirty", config.Prefs{DiffScope: "dirty"}, scopeDirty},
		{"OpenTUI working alias", config.Prefs{DiffScope: "working"}, scopeDirty},
		{"explicit parent", config.Prefs{DiffScope: "parent"}, scopeParent},
		{"explicit trunk", config.Prefs{DiffScope: "trunk"}, scopeTrunk},
		{"legacy true → trunk", config.Prefs{DiffVsTrunk: boolPtr(true)}, scopeTrunk},
		{"legacy false → parent", config.Prefs{DiffVsTrunk: boolPtr(false)}, scopeParent},
		{"scope wins over legacy", config.Prefs{DiffScope: "dirty", DiffVsTrunk: boolPtr(true)}, scopeDirty},
	}
	for _, c := range cases {
		if got := scopeFromPrefs(c.p); got != c.want {
			t.Errorf("%s: scopeFromPrefs = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestDiffScopeCycle verifies [b] cycles dirty → parent → trunk → dirty and the
// persisted string round-trips through scopeFromPrefs.
func TestDiffScopeCycle(t *testing.T) {
	s := scopeDirty
	for _, want := range []diffScope{scopeParent, scopeTrunk, scopeDirty} {
		s = s.next()
		if s != want {
			t.Fatalf("next() = %v, want %v", s, want)
		}
	}
	for _, sc := range []diffScope{scopeDirty, scopeParent, scopeTrunk} {
		if got := scopeFromPrefs(config.Prefs{DiffScope: sc.pref()}); got != sc {
			t.Errorf("round-trip %v: pref %q → %v", sc, sc.pref(), got)
		}
	}
	if scopeDirty.label() == "" || scopeParent.label() == "" || scopeTrunk.label() == "" {
		t.Error("scope labels must be non-empty")
	}
}

// TestSliceMergeStateGitMerged: a member with no PR counts as merged when its
// branch is locally merged into trunk, so an all-git-merged slice is ready even
// without gh; an unmerged member keeps it out of the ready state.
func TestSliceMergeStateGitMerged(t *testing.T) {
	sl := model.Slice{
		Name: "s",
		Members: map[string]model.SliceMember{
			"r": {Repo: "r", Branch: "feat"},
		},
	}

	// Nothing loaded → mergeNone.
	m := Model{prs: map[string]map[string]*forge.PR{}, gitMerged: map[string]map[string]bool{}}
	if got := m.sliceMergeState(sl); got != mergeNone {
		t.Errorf("no data: got %v, want mergeNone", got)
	}

	// No PR but git-merged → ready.
	m.gitMerged["s"] = map[string]bool{"r": true}
	if got := m.sliceMergeState(sl); got != mergeReady {
		t.Errorf("git-merged: got %v, want mergeReady", got)
	}

	// No PR and not git-merged → not ready.
	m.gitMerged["s"] = map[string]bool{"r": false}
	if got := m.sliceMergeState(sl); got == mergeReady {
		t.Errorf("unmerged git branch: got %v, want not ready", got)
	}
}
