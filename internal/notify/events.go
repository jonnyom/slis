// Package notify provides the per-slice session-status event store.
// Each slice's latest status is kept as a single small JSON file inside
// eventsDir so the TUI can poll or watch for changes.
package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonnyom/slis/internal/model"
)

// Status is the on-disk shape for a single slice's session status.
type Status struct {
	Slice  string `json:"slice"`
	Status string `json:"status"`  // model.SessionStatus.String()
	TimeNS int64  `json:"time_ns"` // caller-supplied timestamp
}

// statusFileName returns the filename (without directory) for a slice's status
// file.  Any path-separator characters in the slice name are replaced with '-'
// so the result is always a plain filename component.
func statusFileName(slice string) string {
	safe := strings.ReplaceAll(slice, "/", "-")
	safe = strings.ReplaceAll(safe, string(os.PathSeparator), "-")
	return safe + ".json"
}

// WriteStatus writes (or overwrites) <eventsDir>/<sanitized-slice>.json with
// the given status and timestamp. The directory is created if it does not exist.
func WriteStatus(eventsDir, slice string, st model.SessionStatus, timeNS int64) error {
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		return err
	}
	s := Status{
		Slice:  slice,
		Status: st.String(),
		TimeNS: timeNS,
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	path := filepath.Join(eventsDir, statusFileName(slice))
	return os.WriteFile(path, data, 0o644)
}

// parseStatus converts a string representation back to a model.SessionStatus.
// Unknown strings map to SessNone.
func parseStatus(s string) model.SessionStatus {
	switch s {
	case "running":
		return model.SessRunning
	case "waiting-input":
		return model.SessWaitingInput
	case "done":
		return model.SessDone
	default:
		return model.SessNone
	}
}

// ReadStatus reads the status file for slice inside eventsDir. If the file is
// absent or cannot be read/parsed, SessNone is returned.
func ReadStatus(eventsDir, slice string) model.SessionStatus {
	path := filepath.Join(eventsDir, statusFileName(slice))
	data, err := os.ReadFile(path)
	if err != nil {
		return model.SessNone
	}
	var s Status
	if err := json.Unmarshal(data, &s); err != nil {
		return model.SessNone
	}
	return parseStatus(s.Status)
}

// ReadAllStatuses returns a map of sliceName → SessionStatus for every status
// file present in eventsDir. Files that cannot be parsed are silently skipped.
func ReadAllStatuses(eventsDir string) map[string]model.SessionStatus {
	result := make(map[string]model.SessionStatus)
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		return result
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(eventsDir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s Status
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.Slice == "" {
			continue
		}
		result[s.Slice] = parseStatus(s.Status)
	}
	return result
}
