package config

import (
	"path/filepath"
	"testing"
)

func TestPrefsRoundTripOpenTUIChoices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "prefs.json")
	want := Prefs{
		SplitDiff: true,
		DiffScope: "parent",
		Theme:     "violet",
		Agent:     "codex",
	}
	if err := SavePrefs(path, want); err != nil {
		t.Fatalf("SavePrefs: %v", err)
	}
	if got := LoadPrefs(path); got.SplitDiff != want.SplitDiff || got.DiffScope != want.DiffScope || got.Theme != want.Theme || got.Agent != want.Agent {
		t.Fatalf("LoadPrefs = %+v, want %+v", got, want)
	}
}
