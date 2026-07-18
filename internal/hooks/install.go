package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookEvents are the Claude Code events slis installs a handler for.
// Notification → waiting-input, Stop → done, UserPromptSubmit → running:
// together a complete running⇄waiting⇄done state machine so the TUI reflects
// live status and can notify on each transition.
var hookEvents = []string{"Notification", "Stop", "UserPromptSubmit"}

// hookCommand returns the command string for the given event using binPath.
// e.g. hookCommand("/abs/slis", "Notification") → "/abs/slis hook Notification"
func hookCommand(binPath, event string) string {
	return fmt.Sprintf("%s hook %s", binPath, event)
}

// hookGroupsContain reports whether any group in groups holds a command-hook
// equal to cmd. groups is the value of settings.hooks.<event> (a list of
// {"hooks": [{"type","command"}]} objects); malformed entries are skipped.
func hookGroupsContain(groups []interface{}, cmd string) bool {
	for _, g := range groups {
		gm, ok := g.(map[string]interface{})
		if !ok {
			continue
		}
		inner, ok := gm["hooks"].([]interface{})
		if !ok {
			continue
		}
		for _, h := range inner {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if c, ok := hm["command"].(string); ok && c == cmd {
				return true
			}
		}
	}
	return false
}

// slisHookForEvent recognizes a hook installed by an older slis executable.
// This lets init-hooks migrate paths after a Homebrew reinstall instead of
// leaving Claude invoking a vanished temporary binary forever.
func slisHookForEvent(command, event string) bool {
	fields := strings.Fields(command)
	if len(fields) != 3 || fields[1] != "hook" || fields[2] != event {
		return false
	}
	bin := filepath.Base(strings.Trim(fields[0], "'\""))
	return strings.HasPrefix(bin, "slis")
}

// migrateHookCommands updates stale slis executable paths and removes duplicate
// slis handlers for the same event. Unrelated hooks and malformed entries are
// preserved exactly as they were.
func migrateHookCommands(groups []interface{}, event, cmd string) ([]interface{}, bool) {
	out := make([]interface{}, 0, len(groups))
	changed := false
	seen := false
	for _, group := range groups {
		gm, ok := group.(map[string]interface{})
		if !ok {
			out = append(out, group)
			continue
		}
		inner, ok := gm["hooks"].([]interface{})
		if !ok {
			out = append(out, group)
			continue
		}
		innerOut := make([]interface{}, 0, len(inner))
		groupChanged := false
		for _, hook := range inner {
			hm, ok := hook.(map[string]interface{})
			if !ok {
				innerOut = append(innerOut, hook)
				continue
			}
			old, _ := hm["command"].(string)
			if !slisHookForEvent(old, event) {
				innerOut = append(innerOut, hook)
				continue
			}
			if seen {
				changed = true
				groupChanged = true
				continue
			}
			seen = true
			if old == cmd {
				innerOut = append(innerOut, hook)
				continue
			}
			hookCopy := make(map[string]interface{}, len(hm))
			for key, value := range hm {
				hookCopy[key] = value
			}
			hookCopy["command"] = cmd
			innerOut = append(innerOut, hookCopy)
			changed = true
			groupChanged = true
		}
		if groupChanged {
			if len(innerOut) == 0 && len(gm) == 1 {
				continue
			}
			groupCopy := make(map[string]interface{}, len(gm))
			for key, value := range gm {
				groupCopy[key] = value
			}
			groupCopy["hooks"] = innerOut
			out = append(out, groupCopy)
		} else {
			out = append(out, group)
		}
	}
	return out, changed
}

// MissingHooks returns the slis hook events that are NOT present in the parsed
// Claude settings for binPath (empty slice = all installed). settings may be nil.
func MissingHooks(settings map[string]interface{}, binPath string) []string {
	hooksMap, _ := settings["hooks"].(map[string]interface{})
	var missing []string
	for _, event := range hookEvents {
		groups, _ := hooksMap[event].([]interface{})
		if !hookGroupsContain(groups, hookCommand(binPath, event)) {
			missing = append(missing, event)
		}
	}
	return missing
}

// mergeHookConfig takes the parsed settings map (may be nil/empty), ensures
// the Notification and Stop events each contain a command-hook running
// `<binPath> hook <event>`, and returns the updated settings plus a list of
// human-readable changes (empty if nothing changed).
//
// It does NOT remove or alter unrelated keys or pre-existing hook groups.
// It is a pure, unit-testable function.
func mergeHookConfig(settings map[string]interface{}, binPath string) (map[string]interface{}, []string) {
	if settings == nil {
		settings = map[string]interface{}{}
	}

	// Deep-copy the settings map at the top level to avoid mutating the input.
	out := make(map[string]interface{}, len(settings))
	for k, v := range settings {
		out[k] = v
	}

	// Get or create the hooks map.
	var hooksMap map[string]interface{}
	if existing, ok := out["hooks"]; ok {
		if m, ok := existing.(map[string]interface{}); ok {
			// Copy the hooks map too.
			hooksMap = make(map[string]interface{}, len(m))
			for k, v := range m {
				hooksMap[k] = v
			}
		}
	}
	if hooksMap == nil {
		hooksMap = map[string]interface{}{}
	}

	var changes []string

	for _, event := range hookEvents {
		cmd := hookCommand(binPath, event)

		// Retrieve existing groups for this event as []interface{}.
		var groups []interface{}
		if existing, ok := hooksMap[event]; ok {
			if arr, ok := existing.([]interface{}); ok {
				groups = arr
			}
			// If the type is wrong/malformed, treat as absent (groups stays nil).
		}

		if migrated, ok := migrateHookCommands(groups, event, cmd); ok {
			hooksMap[event] = migrated
			changes = append(changes, fmt.Sprintf("updated %s hook", event))
			continue
		}
		if hookGroupsContain(groups, cmd) {
			continue
		}

		// Append a new group containing the slis hook command.
		newGroup := map[string]interface{}{
			"hooks": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": cmd,
				},
			},
		}
		groups = append(groups, newGroup)
		hooksMap[event] = groups
		changes = append(changes, fmt.Sprintf("added %s hook", event))
	}

	out["hooks"] = hooksMap
	return out, changes
}

// MigrateExistingHooks updates only slis hooks that are already installed. It
// is safe to run on startup: missing settings and missing hook events remain
// untouched, while Homebrew path changes and duplicate legacy hooks self-heal.
func MigrateExistingHooks(settingsPath, binPath string) ([]string, error) {
	settings, exists, err := readSettings(settingsPath)
	if err != nil || !exists {
		return nil, err
	}
	hooksMap, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		return nil, nil
	}
	out := make(map[string]interface{}, len(settings))
	for key, value := range settings {
		out[key] = value
	}
	hooksOut := make(map[string]interface{}, len(hooksMap))
	for key, value := range hooksMap {
		hooksOut[key] = value
	}
	var changes []string
	for _, event := range hookEvents {
		groups, ok := hooksMap[event].([]interface{})
		if !ok {
			continue
		}
		if migrated, changed := migrateHookCommands(groups, event, hookCommand(binPath, event)); changed {
			hooksOut[event] = migrated
			changes = append(changes, fmt.Sprintf("updated %s hook", event))
		}
	}
	if len(changes) == 0 {
		return nil, nil
	}
	out["hooks"] = hooksOut
	if err := writeSettings(settingsPath, out); err != nil {
		return nil, err
	}
	return changes, nil
}

func readSettings(settingsPath string) (map[string]interface{}, bool, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, false, nil
		}
		return nil, false, fmt.Errorf("reading %s: %w", settingsPath, err)
	}
	settings := map[string]interface{}{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &settings); err != nil {
			return nil, true, fmt.Errorf("parsing %s: %w", settingsPath, err)
		}
	}
	return settings, true, nil
}

// InitHooks reads settingsPath (JSON; treats missing file as empty {}), merges
// the slis hooks via mergeHookConfig, and writes the result back
// (pretty-printed, creating parent dirs as needed).
//
// Returns the list of changes made (empty = already installed). binPath is
// the absolute path to the slis binary.
func InitHooks(settingsPath, binPath string) ([]string, error) {
	settings, _, err := readSettings(settingsPath)
	if err != nil {
		return nil, err
	}

	merged, changes := mergeHookConfig(settings, binPath)

	// Nothing changed — idempotent no-op; do NOT rewrite the file.
	if len(changes) == 0 {
		return nil, nil
	}

	if err := writeSettings(settingsPath, merged); err != nil {
		return nil, err
	}
	return changes, nil
}

func writeSettings(settingsPath string, settings map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return fmt.Errorf("creating parent dirs: %w", err)
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling settings: %w", err)
	}

	// Resolve symlinks first: settings.json is often a symlink into a dotfiles
	// repo. Renaming over the link path would replace the symlink with a regular
	// file (breaking the dotfiles link); instead we write the temp file beside —
	// and rename over — the real target so the symlink and its destination are
	// both preserved. EvalSymlinks fails for a not-yet-existing file, in which
	// case we keep the original path (fresh create).
	target := settingsPath
	if resolved, rerr := filepath.EvalSymlinks(settingsPath); rerr == nil {
		target = resolved
	}

	// Write atomically: a crash or disk-full midway through os.WriteFile would
	// otherwise leave the user's Claude settings.json truncated/corrupted. Write
	// a sibling temp file, then rename over the target (atomic on the same fs).
	tmp, err := os.CreateTemp(filepath.Dir(target), ".slis-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp settings file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp settings file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp settings file: %w", err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("chmod temp settings file: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("replacing %s: %w", target, err)
	}
	return nil
}
