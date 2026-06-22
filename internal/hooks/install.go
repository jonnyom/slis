package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// hookCommand returns the command string for the given event using binPath.
// e.g. hookCommand("/abs/slis", "Notification") → "/abs/slis hook Notification"
func hookCommand(binPath, event string) string {
	return fmt.Sprintf("%s hook %s", binPath, event)
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

	for _, event := range []string{"Notification", "Stop"} {
		cmd := hookCommand(binPath, event)

		// Retrieve existing groups for this event as []interface{}.
		var groups []interface{}
		if existing, ok := hooksMap[event]; ok {
			if arr, ok := existing.([]interface{}); ok {
				groups = arr
			}
			// If the type is wrong/malformed, treat as absent (groups stays nil).
		}

		// Scan every group → its hooks array → each entry's command string.
		found := false
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
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if found {
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

// InitHooks reads settingsPath (JSON; treats missing file as empty {}), merges
// the slis hooks via mergeHookConfig, and writes the result back
// (pretty-printed, creating parent dirs as needed).
//
// Returns the list of changes made (empty = already installed). binPath is
// the absolute path to the slis binary.
func InitHooks(settingsPath, binPath string) ([]string, error) {
	var settings map[string]interface{}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			settings = map[string]interface{}{}
		} else {
			return nil, fmt.Errorf("reading %s: %w", settingsPath, err)
		}
	} else {
		if len(data) == 0 {
			settings = map[string]interface{}{}
		} else {
			if err := json.Unmarshal(data, &settings); err != nil {
				return nil, fmt.Errorf("parsing %s: %w", settingsPath, err)
			}
		}
	}

	merged, changes := mergeHookConfig(settings, binPath)

	// Nothing changed — idempotent no-op; do NOT rewrite the file.
	if len(changes) == 0 {
		return nil, nil
	}

	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return nil, fmt.Errorf("creating parent dirs: %w", err)
	}

	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return nil, fmt.Errorf("writing %s: %w", settingsPath, err)
	}

	return changes, nil
}
