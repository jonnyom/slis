package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestHookCommand verifies hookCommand formats the command string correctly.
func TestHookCommand(t *testing.T) {
	got := hookCommand("/x/slis", "Notification")
	want := "/x/slis hook Notification"
	if got != want {
		t.Errorf("hookCommand = %q; want %q", got, want)
	}
}

// TestInitHooksFreshInstall verifies that InitHooks creates the settings file
// and installs both Notification and Stop hooks when the file does not exist.
func TestInitHooksFreshInstall(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")
	changes, err := InitHooks(path, "/x/slis")
	if err != nil {
		t.Fatalf("InitHooks error: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d: %v", len(changes), changes)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading settings file: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	hooksMap, ok := out["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("hooks key missing or wrong type")
	}

	for _, event := range []string{"Notification", "Stop"} {
		groups, ok := hooksMap[event].([]interface{})
		if !ok {
			t.Errorf("hooks.%s is not an array", event)
			continue
		}
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
				if hm["command"] == hookCommand("/x/slis", event) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("command for %s not found in settings", event)
		}
	}
}

// TestInitHooksIdempotent verifies that running InitHooks twice produces no
// changes on the second run and does not alter the file content.
func TestInitHooksIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude", "settings.json")

	_, err := InitHooks(path, "/x/slis")
	if err != nil {
		t.Fatalf("first InitHooks: %v", err)
	}

	after1, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading after first run: %v", err)
	}

	changes, err := InitHooks(path, "/x/slis")
	if err != nil {
		t.Fatalf("second InitHooks: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("expected 0 changes on second run, got %d: %v", len(changes), changes)
	}

	after2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading after second run: %v", err)
	}

	if string(after1) != string(after2) {
		t.Errorf("file content changed on second run:\nbefore: %s\nafter:  %s", after1, after2)
	}
}

// TestInitHooksPreservesExisting verifies that InitHooks preserves other
// top-level keys, other hook events, and adds Notification + Stop correctly.
func TestInitHooksPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	initial := `{"model":"opus","hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"echo hi"}]}]}}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("writing initial settings: %v", err)
	}

	_, err := InitHooks(path, "/x/slis")
	if err != nil {
		t.Fatalf("InitHooks: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// model key preserved
	if out["model"] != "opus" {
		t.Errorf("model key not preserved: %v", out["model"])
	}

	hooksMap, ok := out["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("hooks key missing or wrong type")
	}

	// PreToolUse preserved
	ptu, ok := hooksMap["PreToolUse"].([]interface{})
	if !ok || len(ptu) == 0 {
		t.Error("PreToolUse hook missing or empty")
	}

	// Notification and Stop added
	for _, event := range []string{"Notification", "Stop"} {
		groups, ok := hooksMap[event].([]interface{})
		if !ok || len(groups) == 0 {
			t.Errorf("hooks.%s missing or empty", event)
			continue
		}
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
				if hm["command"] == hookCommand("/x/slis", event) {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("command for %s not found", event)
		}
	}
}

// TestMergeHookConfigNoDuplicate verifies that mergeHookConfig called twice on
// the same settings produces an empty changes list the second time.
func TestMergeHookConfigNoDuplicate(t *testing.T) {
	settings := map[string]interface{}{}

	merged1, changes1 := mergeHookConfig(settings, "/x/slis")
	if len(changes1) != 2 {
		t.Fatalf("first merge: expected 2 changes, got %d: %v", len(changes1), changes1)
	}

	merged2, changes2 := mergeHookConfig(merged1, "/x/slis")
	if len(changes2) != 0 {
		t.Errorf("second merge: expected 0 changes, got %d: %v", len(changes2), changes2)
	}

	// Verify array lengths didn't grow
	hooksMap1, _ := merged1["hooks"].(map[string]interface{})
	hooksMap2, _ := merged2["hooks"].(map[string]interface{})
	for _, event := range []string{"Notification", "Stop"} {
		g1, _ := hooksMap1[event].([]interface{})
		g2, _ := hooksMap2[event].([]interface{})
		if len(g1) != len(g2) {
			t.Errorf("event %s: group count grew from %d to %d", event, len(g1), len(g2))
		}
	}
}
