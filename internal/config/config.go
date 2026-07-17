package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Repo describes a single repository managed by slis.
type Repo struct {
	Primary       string `yaml:"primary"`
	DefaultBranch string `yaml:"default_branch"`
}

// Grouping controls how worktrees are grouped into slices.
type Grouping struct {
	Strategy    string `yaml:"strategy"`
	StripPrefix string `yaml:"strip_prefix"`
	// Ignore is a list of globs for worktree paths that must never be ingested
	// as slices (e.g. agent sandboxes). A built-in default always applies on top
	// of this list; see discovery.DefaultIgnoreGlobs.
	Ignore []string `yaml:"ignore,omitempty"`
}

// SliceNameFromBranch derives a slice name from a branch by removing the
// configured strip_prefix. If the prefix was given without a trailing slash
// (e.g. "jonny" instead of "jonny/"), a leftover leading slash is also trimmed,
// so both "jonny" and "jonny/" turn "jonny/wfm-1" into "wfm-1". A branch that
// doesn't start with the prefix is returned unchanged. Shared by discovery
// (branch → slice) and create (so a fully-qualified name doesn't keep its
// prefix in the slice's display name / path / session).
func SliceNameFromBranch(branch, stripPrefix string) string {
	if stripPrefix == "" || !strings.HasPrefix(branch, stripPrefix) {
		return branch
	}
	name := branch[len(stripPrefix):]
	if !strings.HasSuffix(stripPrefix, "/") {
		name = strings.TrimPrefix(name, "/")
	}
	return name
}

// Sessions holds session-related configuration.
type Sessions struct {
	AutostartClaude bool `yaml:"autostart_claude"`
	// Agent is the command launched by the "launch agent" action (TUI `C`).
	// Empty defaults to "claude". May include args, e.g. "claude --resume".
	Agent string `yaml:"agent"`
	// Editor is the binary used to open worktrees/slices (TUI `o`/`e`, `slis
	// edit`), e.g. "code", "cursor", "zed". Empty → auto-detect (and the TUI
	// prompts once when several are found).
	Editor string `yaml:"editor"`
	// Layout controls a slice's tmux session windows:
	//   "root"  — a single window at the workspace root (run Claude across the stack)
	//   "repos" — one window per repo worktree
	//   "both"  — a root window first, then one per repo
	// Empty defaults to "root" when a workspace root is set, else "repos".
	Layout string `yaml:"layout"`
}

// Processes holds process-monitoring thresholds.
type Processes struct {
	CPUWarnPct int `yaml:"cpu_warn_pct"`
}

// DepReconcile holds the lockfile list and install command for a single repo's
// dependency reconciliation during slice activation.
type DepReconcile struct {
	Lockfiles []string `yaml:"lockfiles"`
	Install   string   `yaml:"install"`
}

// Swap holds post-activation hooks and dependency reconciliation config.
type Swap struct {
	DepReconcile map[string]DepReconcile `yaml:"dep_reconcile"`
	PostActivate string                  `yaml:"post_activate"`
}

// NotifyChannel configures a single notification channel.
type NotifyChannel struct {
	Sound string `yaml:"sound"`
}

// Notify holds notification configuration.
type Notify struct {
	NeedsInput NotifyChannel `yaml:"needs_input"`
	Done       NotifyChannel `yaml:"done"`
}

// Workspace is the top-level config loaded from workspace.yaml.
type Workspace struct {
	Root      string          `yaml:"root"`
	Repos     map[string]Repo `yaml:"repos"`
	Grouping  Grouping        `yaml:"grouping"`
	Swap      Swap            `yaml:"swap"`
	Sessions  Sessions        `yaml:"sessions"`
	Notify    Notify          `yaml:"notify"`
	Processes Processes       `yaml:"processes"`
}

// expandTilde replaces a leading "~" with the current user's home directory.
// Empty strings are returned as-is.
func expandTilde(path string) (string, error) {
	if path == "" {
		return path, nil
	}
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("expandTilde: %w", err)
	}
	return home + path[1:], nil
}

// LoadWorkspace reads and parses the workspace.yaml at path, expands ~ in
// paths, applies defaults, and validates required fields.
func LoadWorkspace(path string) (Workspace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workspace{}, fmt.Errorf("LoadWorkspace: read %q: %w", path, err)
	}

	var ws Workspace
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return Workspace{}, fmt.Errorf("LoadWorkspace: unmarshal %q: %w", path, err)
	}

	// Expand ~ in root.
	ws.Root, err = expandTilde(ws.Root)
	if err != nil {
		return Workspace{}, err
	}

	// Expand ~ in each repo's primary path and validate it is non-empty.
	for name, repo := range ws.Repos {
		if repo.Primary == "" {
			return Workspace{}, fmt.Errorf("repo %q: primary path is empty", name)
		}
		repo.Primary, err = expandTilde(repo.Primary)
		if err != nil {
			return Workspace{}, err
		}
		ws.Repos[name] = repo
	}

	// Apply defaults.
	if ws.Processes.CPUWarnPct == 0 {
		ws.Processes.CPUWarnPct = 150
	}
	if ws.Grouping.Strategy == "" {
		ws.Grouping.Strategy = "branch-name"
	}

	return ws, nil
}

// SaveWorkspace marshals ws to YAML and writes it to path, creating any
// parent directories as needed.
func SaveWorkspace(path string, ws Workspace) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("SaveWorkspace: mkdir %q: %w", filepath.Dir(path), err)
	}
	data, err := yaml.Marshal(ws)
	if err != nil {
		return fmt.Errorf("SaveWorkspace: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("SaveWorkspace: write %q: %w", path, err)
	}
	return nil
}
