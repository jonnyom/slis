package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Prefs holds small, persistent UI toggles (remembered across runs). It is
// separate from workspace.yaml so toggling the diff view doesn't rewrite the
// user's hand-edited workspace config.
type Prefs struct {
	SplitDiff bool   `json:"split_diff"`           // cockpit: side-by-side diff
	DiffScope string `json:"diff_scope,omitempty"` // cockpit: "dirty"/"working" | "parent" | "trunk"
	Theme     string `json:"theme,omitempty"`      // OpenTUI: "auto" or a palette name
	Agent     string `json:"agent,omitempty"`      // OpenTUI: last selected agent name
	// DiffVsTrunk is the legacy pre-scope toggle, kept only so an old prefs file
	// migrates cleanly: true → trunk scope, false → parent scope, absent (nil) →
	// dirty (the new default). It is no longer written.
	DiffVsTrunk *bool `json:"diff_vs_trunk,omitempty"`
}

// LoadPrefs reads prefs from path. A missing or unreadable file yields zero-value
// Prefs (no error) — preferences are best-effort.
func LoadPrefs(path string) Prefs {
	var p Prefs
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return p
	}
	_ = json.Unmarshal(data, &p) // ignore parse errors → defaults
	return p
}

// SavePrefs writes prefs to path (creating the parent dir). Best-effort: callers
// ignore the error since a failed save just means the toggle isn't remembered.
func SavePrefs(path string, p Prefs) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
