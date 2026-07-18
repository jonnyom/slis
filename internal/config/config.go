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
	// Harness selects the agent harness: "claude" (default) or "codex". It picks
	// the launch binary and how slice context is injected (claude gets
	// --append-system-prompt; codex gets neither a positional prompt nor a flag).
	// An explicit Agent overrides the binary verbatim.
	Harness string `yaml:"harness"`
	// Agent is the command launched by the "launch agent" action (TUI `C`).
	// Non-empty wins verbatim over Harness; empty → the Harness binary. May
	// include args, e.g. "claude --resume".
	Agent string `yaml:"agent"`
	// Agents is the optional list of selectable coding agents (name + argv). When
	// it holds more than one entry the front-end offers a picker before launch;
	// empty falls back to the single default agent derived from Harness/Agent
	// (see AgentList). Each entry needs a non-empty name and cmd.
	Agents []AgentSpec `yaml:"agents,omitempty"`
	// Autostart launches the harness in a slice's session automatically when the
	// session is first attached (same as pressing `C`).
	Autostart bool `yaml:"autostart"`
	// AutostartClaude is the legacy name for Autostart, merged into it on load.
	AutostartClaude bool `yaml:"autostart_claude"`
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

// AgentSpec is one selectable coding agent: a display name and the argv launched
// in a slice's session (e.g. {Name: "claude", Cmd: ["claude", "--resume"]}).
type AgentSpec struct {
	Name string   `yaml:"name"`
	Cmd  []string `yaml:"cmd"`
}

// AgentList returns the selectable agents. Explicitly configured agents win;
// otherwise a single default is derived from the resolved AgentCommand (harness
// / agent), so callers always get at least one entry. The default's name is the
// command's first token so "claude --resume" reads as "claude" in a picker.
func (s Sessions) AgentList() []AgentSpec {
	if len(s.Agents) > 0 {
		return s.Agents
	}
	fields := strings.Fields(s.AgentCommand())
	name := "claude"
	if len(fields) > 0 {
		name = fields[0]
	}
	return []AgentSpec{{Name: name, Cmd: fields}}
}

// HarnessName returns the configured harness, defaulting to "claude".
func (s Sessions) HarnessName() string {
	if s.Harness == "" {
		return "claude"
	}
	return s.Harness
}

// AgentCommand returns the command to launch the agent. A non-empty Agent wins
// verbatim; otherwise the harness selects the binary ("claude" or "codex").
func (s Sessions) AgentCommand() string {
	if s.Agent != "" {
		return s.Agent
	}
	if s.HarnessName() == "codex" {
		return "codex"
	}
	return "claude"
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
	// Activate is an optional macOS application bundle id (e.g.
	// "com.mitchellh.ghostty", "com.googlecode.iterm2", "com.apple.Terminal") that
	// terminal-notifier foregrounds when a banner is clicked. Empty = off.
	Activate string `yaml:"activate"`
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

	// Merge the legacy autostart_claude alias into Autostart so old configs keep
	// working after the field was generalised across harnesses.
	if ws.Sessions.AutostartClaude {
		ws.Sessions.Autostart = true
	}

	// Validate any configured agents: each needs a non-empty name and cmd.
	for i, a := range ws.Sessions.Agents {
		if strings.TrimSpace(a.Name) == "" {
			return Workspace{}, fmt.Errorf("sessions.agents[%d]: name is empty", i)
		}
		if len(a.Cmd) == 0 {
			return Workspace{}, fmt.Errorf("sessions.agents[%d] (%s): cmd is empty", i, a.Name)
		}
		for j, arg := range a.Cmd {
			if strings.TrimSpace(arg) == "" {
				return Workspace{}, fmt.Errorf("sessions.agents[%d] (%s): cmd[%d] is empty", i, a.Name, j)
			}
		}
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
