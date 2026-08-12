package config

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const WorkspaceConfigEnv = "SLIS_WORKSPACE_CONFIG"

// Paths holds the XDG-compliant file-system paths used by slis at runtime.
type Paths struct {
	// StateDir is the root state directory: $XDG_STATE_HOME/slis (or ~/.local/state/slis).
	StateDir string
	// Overrides is the path to the manual-grouping overrides file.
	Overrides string
	// Registry is the path to the managed-slice registry file (opt-in ingestion).
	Registry string
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
	// Reviews is the path to the pending inline-review-comment store (fed to a
	// slice's agent by `slis review send`).
	Reviews string
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
	if scope := WorkspaceScope(); scope != "" {
		stateDir = filepath.Join(stateDir, "workspaces", scope)
	}
	return Paths{
		StateDir:      stateDir,
		Overrides:     filepath.Join(stateDir, "overrides.yaml"),
		Registry:      filepath.Join(stateDir, "registry.yaml"),
		ActiveJournal: filepath.Join(stateDir, "active.json"),
		EventsDir:     filepath.Join(stateDir, "events"),
		Prefs:         filepath.Join(stateDir, "prefs.json"),
		WorkspacesDir: filepath.Join(stateDir, "workspaces"),
		Comments:      filepath.Join(stateDir, "comments.json"),
		Reviews:       filepath.Join(stateDir, "reviews.json"),
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

func WorkspacePath() string {
	if configured := os.Getenv(WorkspaceConfigEnv); configured != "" {
		return canonicalPath(configured)
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return WorkspacePathForRoot(".")
	}
	if selected := workspacePathForDirectory(workingDirectory); selected != "" {
		return selected
	}
	return WorkspacePathForRoot(workingDirectory)
}

func WorkspacePathForRoot(root string) string {
	legacyRoot, err := workspaceRoot(legacyWorkspacePath())
	if err == nil && canonicalPath(legacyRoot) == canonicalPath(root) {
		return legacyWorkspacePath()
	}
	return filepath.Join(ConfigDir(), "workspaces", workspaceKey(root), "workspace.yaml")
}

func WorkspaceScope() string {
	path := WorkspacePath()
	relative, err := filepath.Rel(filepath.Join(ConfigDir(), "workspaces"), path)
	if err != nil {
		return ""
	}
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	if len(parts) != 2 || parts[1] != "workspace.yaml" || parts[0] == ".." {
		return ""
	}
	return parts[0]
}

func legacyWorkspacePath() string {
	return filepath.Join(ConfigDir(), "workspace.yaml")
}

func workspacePathForDirectory(directory string) string {
	candidates := []string{legacyWorkspacePath()}
	entries, err := os.ReadDir(filepath.Join(ConfigDir(), "workspaces"))
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				candidates = append(candidates, filepath.Join(ConfigDir(), "workspaces", entry.Name(), "workspace.yaml"))
			}
		}
	}

	directory = canonicalPath(directory)
	selectedPath := ""
	selectedRoot := ""
	for _, candidate := range candidates {
		workspaceRoot, err := workspaceRoot(candidate)
		if err != nil {
			continue
		}
		root := canonicalPath(workspaceRoot)
		if pathWithinRoot(directory, root) && len(root) > len(selectedRoot) {
			selectedPath = candidate
			selectedRoot = root
		}
	}
	return selectedPath
}

func workspaceRoot(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var workspace struct {
		Root string `yaml:"root"`
	}
	if err := yaml.Unmarshal(data, &workspace); err != nil {
		return "", err
	}
	if strings.TrimSpace(workspace.Root) == "" {
		return "", os.ErrInvalid
	}
	return expandTilde(workspace.Root)
}

func workspaceKey(root string) string {
	digest := sha256.Sum256([]byte(canonicalPath(root)))
	return hex.EncodeToString(digest[:8])
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err == nil {
		path = absolute
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}

func pathWithinRoot(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
