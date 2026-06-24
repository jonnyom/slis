package config

import (
	"os"
	"path/filepath"
)

// Paths holds the XDG-compliant file-system paths used by slis at runtime.
type Paths struct {
	// StateDir is the root state directory: $XDG_STATE_HOME/slis (or ~/.local/state/slis).
	StateDir string
	// Overrides is the path to the manual-grouping overrides file.
	Overrides string
	// ActiveJournal is the path to the active-swap journal file.
	ActiveJournal string
	// EventsDir is the directory where hook events are stored.
	EventsDir string
	// Prefs is the path to the small UI-preferences file (persistent toggles).
	Prefs string
	// WorkspacesDir holds generated editor workspace files (e.g. .code-workspace).
	WorkspacesDir string
	// Comments is the path to the persisted PR-comment cache (survives slice removal).
	Comments string
}

// stateBase returns the base directory for XDG state, honouring XDG_STATE_HOME
// and falling back to ~/.local/state (or ".slis-state" if home cannot be determined).
func stateBase() string {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return base
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".slis-state"
	}
	return filepath.Join(home, ".local", "state")
}

// configBase returns the base directory for XDG config, honouring XDG_CONFIG_HOME
// and falling back to ~/.config (or ".slis-config" if home cannot be determined).
func configBase() string {
	if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
		return base
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".slis-config"
	}
	return filepath.Join(home, ".config")
}

// StatePaths returns the set of runtime state paths rooted at
// $XDG_STATE_HOME/slis (fallback: ~/.local/state/slis).
func StatePaths() Paths {
	stateDir := filepath.Join(stateBase(), "slis")
	return Paths{
		StateDir:      stateDir,
		Overrides:     filepath.Join(stateDir, "overrides.yaml"),
		ActiveJournal: filepath.Join(stateDir, "active.json"),
		EventsDir:     filepath.Join(stateDir, "events"),
		Prefs:         filepath.Join(stateDir, "prefs.json"),
		WorkspacesDir: filepath.Join(stateDir, "workspaces"),
		Comments:      filepath.Join(stateDir, "comments.json"),
	}
}

// EnsureDirs creates the state directory tree required by slis.
// It is idempotent — calling it on an existing tree is a no-op.
func (p Paths) EnsureDirs() error {
	if err := os.MkdirAll(p.StateDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(p.EventsDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(p.WorkspacesDir, 0o755)
}

// ConfigDir returns the slis configuration directory:
// $XDG_CONFIG_HOME/slis (fallback: ~/.config/slis).
func ConfigDir() string {
	return filepath.Join(configBase(), "slis")
}

// WorkspacePath returns the canonical path to workspace.yaml inside ConfigDir.
func WorkspacePath() string {
	return filepath.Join(ConfigDir(), "workspace.yaml")
}
