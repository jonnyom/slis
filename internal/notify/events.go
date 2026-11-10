// Package notify provides the per-session status event store.
// Each Claude session's latest status is kept as a single small JSON file
// inside eventsDir; readers combine the sessions of a slice so the TUI can poll
// or watch one status per slice.
package notify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonnyom/slis/internal/model"
)

// statusMaxAge bounds how long a record counts without a fresh hook event. A
// session that crashed never sends SessionEnd, so its last status would
// otherwise stay on screen forever.
const statusMaxAge = 24 * time.Hour

// Status is the on-disk shape for a single session's status.
type Status struct {
	Slice     string `json:"slice"`
	Status    string `json:"status"`  // model.SessionStatus.String()
	TimeNS    int64  `json:"time_ns"` // caller-supplied timestamp
	SessionID string `json:"session_id,omitempty"`
	Cwd       string `json:"cwd,omitempty"`
	Subagents int    `json:"subagents,omitempty"`
}

// SliceSummary is a slice's combined status: the most urgent status across
// its sessions, plus the background sub-agents still working while a main
// thread waits on them.
type SliceSummary struct {
	Status    model.SessionStatus
	Subagents int
}

func sanitizeFileNamePart(part string) string {
	safe := strings.ReplaceAll(part, "/", "-")
	return strings.ReplaceAll(safe, string(os.PathSeparator), "-")
}

// statusFileName returns the filename (without directory) for one session's
// status. Records without a session id use the slice's legacy file name.
func statusFileName(slice, sessionID string) string {
	if sessionID == "" {
		return sanitizeFileNamePart(slice) + ".json"
	}
	return sanitizeFileNamePart(slice) + "@" + sanitizeFileNamePart(sessionID) + ".json"
}

// RemoveStatus deletes every session status of a slice (used when a slice is
// cleared). A slice with no status is not an error.
func RemoveStatus(eventsDir, slice string) error {
	for _, record := range readStoredRecords(eventsDir, false) {
		if record.status.Slice != slice {
			continue
		}
		if err := os.Remove(record.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// RemoveSessionStatus deletes one session's status (used when the session
// ends). A missing file is not an error.
func RemoveSessionStatus(eventsDir, slice, sessionID string) error {
	err := os.Remove(filepath.Join(eventsDir, statusFileName(slice, sessionID)))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// WriteStatus writes (or overwrites) the slice's session-less status record.
func WriteStatus(eventsDir, slice string, st model.SessionStatus, timeNS int64) error {
	return WriteStatusRecord(eventsDir, Status{Slice: slice, Status: st.String(), TimeNS: timeNS})
}

// WriteStatusRecord replaces one session's status record. The file is renamed
// into place so a concurrent reader never sees a half-written record.
func WriteStatusRecord(eventsDir string, status Status) error {
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(eventsDir, ".status-")
	if err != nil {
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	if err := os.Chmod(temporary.Name(), 0o644); err != nil {
		_ = os.Remove(temporary.Name())
		return err
	}
	return os.Rename(temporary.Name(), filepath.Join(eventsDir, statusFileName(status.Slice, status.SessionID)))
}

// ParseStatus converts a string representation back to a model.SessionStatus.
// Unknown strings map to SessNone.
func ParseStatus(s string) model.SessionStatus {
	switch s {
	case "running":
		return model.SessRunning
	case "waiting-input":
		return model.SessWaitingInput
	case "done":
		return model.SessDone
	case "subagents":
		return model.SessSubagents
	default:
		return model.SessNone
	}
}

// urgency orders statuses for the slice summary: what needs the user first,
// then active work, then finished work.
func urgency(status model.SessionStatus) int {
	switch status {
	case model.SessWaitingInput:
		return 4
	case model.SessRunning:
		return 3
	case model.SessSubagents:
		return 2
	case model.SessDone:
		return 1
	default:
		return 0
	}
}

func moreUrgent(candidate, current Status) bool {
	candidateUrgency := urgency(ParseStatus(candidate.Status))
	currentUrgency := urgency(ParseStatus(current.Status))
	if candidateUrgency != currentUrgency {
		return candidateUrgency > currentUrgency
	}
	return candidate.TimeNS > current.TimeNS
}

type storedRecord struct {
	path   string
	status Status
}

func readStoredRecords(eventsDir string, skipExpired bool) []storedRecord {
	entries, err := os.ReadDir(eventsDir)
	if err != nil {
		return nil
	}
	var records []storedRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if skipExpired {
			info, err := entry.Info()
			if err != nil || time.Since(info.ModTime()) > statusMaxAge {
				continue
			}
		}
		path := filepath.Join(eventsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var status Status
		if err := json.Unmarshal(data, &status); err != nil || status.Slice == "" {
			continue
		}
		records = append(records, storedRecord{path: path, status: status})
	}
	return records
}

// ReadSessionStatus reads one session's current record. A missing or expired
// record reads as the zero Status.
func ReadSessionStatus(eventsDir, slice, sessionID string) Status {
	path := filepath.Join(eventsDir, statusFileName(slice, sessionID))
	info, err := os.Stat(path)
	if err != nil || time.Since(info.ModTime()) > statusMaxAge {
		return Status{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Status{}
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}
	}
	return status
}

// SliceForSession returns the slice that sessionID last reported a status for,
// or "" when it never has. A session keeps its ID for life, so this follows it
// after it `cd`s out of the slice's worktrees. Expired records still count:
// the mapping outlives the status.
func SliceForSession(eventsDir, sessionID string) string {
	if sessionID == "" {
		return ""
	}
	var latest Status
	for _, record := range readStoredRecords(eventsDir, false) {
		if record.status.SessionID == sessionID && record.status.TimeNS >= latest.TimeNS {
			latest = record.status
		}
	}
	return latest.Slice
}

// HasStatusRecord reports whether any session of slice has ever reported a
// status, including records too old to count towards its summary.
func HasStatusRecord(eventsDir, slice string) bool {
	for _, record := range readStoredRecords(eventsDir, false) {
		if record.status.Slice == slice {
			return true
		}
	}
	return false
}

// UnmatchedHook is the latest hook event no slice claimed, kept so `slis doctor`
// can explain a status that never updates.
type UnmatchedHook struct {
	Event     string `json:"event"`
	TimeNS    int64  `json:"time_ns"`
	SessionID string `json:"session_id,omitempty"`
	Cwd       string `json:"cwd"`
}

// unmatchedHookFile has no .json suffix so status scans skip it.
const unmatchedHookFile = "unmatched-hook"

func WriteUnmatchedHook(eventsDir string, hook UnmatchedHook) error {
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(hook)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(eventsDir, unmatchedHookFile), data, 0o644)
}

// ReadUnmatchedHook returns the latest unmatched hook, or ok=false when none
// has been recorded.
func ReadUnmatchedHook(eventsDir string) (hook UnmatchedHook, ok bool) {
	data, err := os.ReadFile(filepath.Join(eventsDir, unmatchedHookFile))
	if err != nil {
		return UnmatchedHook{}, false
	}
	if err := json.Unmarshal(data, &hook); err != nil {
		return UnmatchedHook{}, false
	}
	return hook, true
}

// ReadStatus returns the slice's most urgent session status, or SessNone.
func ReadStatus(eventsDir, slice string) model.SessionStatus {
	return ParseStatus(ReadStatusRecord(eventsDir, slice).Status)
}

// ReadStatusRecord returns the record of the slice's most urgent session.
func ReadStatusRecord(eventsDir, slice string) Status {
	var mostUrgent Status
	for _, record := range readStoredRecords(eventsDir, true) {
		if record.status.Slice == slice && moreUrgent(record.status, mostUrgent) {
			mostUrgent = record.status
		}
	}
	return mostUrgent
}

// ReadAllStatuses returns sliceName → combined status for every slice with a
// current record. Files that cannot be parsed are silently skipped.
func ReadAllStatuses(eventsDir string) map[string]SliceSummary {
	result := make(map[string]SliceSummary)
	for _, record := range readStoredRecords(eventsDir, true) {
		status := ParseStatus(record.status.Status)
		summary := result[record.status.Slice]
		if status == model.SessSubagents {
			summary.Subagents += record.status.Subagents
		}
		if urgency(status) > urgency(summary.Status) {
			summary.Status = status
		}
		result[record.status.Slice] = summary
	}
	for slice, summary := range result {
		if summary.Status != model.SessSubagents {
			summary.Subagents = 0
			result[slice] = summary
		}
	}
	return result
}
