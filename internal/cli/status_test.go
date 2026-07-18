package cli

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/report"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// statusTestPaths returns a config.Paths pointing at fresh temp dirs so the
// test never reads the real workspace's overrides/journal/events.
func statusTestPaths(t *testing.T) config.Paths {
	t.Helper()
	tmp := t.TempDir()
	return config.Paths{
		Overrides:     filepath.Join(tmp, "ov.yaml"),   // absent → no overrides
		ActiveJournal: filepath.Join(tmp, "none.json"), // absent → nothing active
		EventsDir:     filepath.Join(tmp, "events"),    // created lazily by WriteStatus
	}
}

func TestSliceStatuses(t *testing.T) {
	ws := makeTestWorkspace(t)
	sp := statusTestPaths(t)

	// Record a session status for "checkout" only; "other" has no event.
	if err := notify.WriteStatus(sp.EventsDir, "checkout", model.SessWaitingInput, 1); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	dtos, err := sliceStatuses(ws, sp)
	if err != nil {
		t.Fatalf("sliceStatuses: %v", err)
	}

	if len(dtos) != 2 {
		t.Fatalf("want 2 statuses, got %d: %v", len(dtos), dtos)
	}

	// Sorted by slice name: checkout < other.
	if dtos[0].Slice != "checkout" || dtos[0].Status != "waiting-input" {
		t.Errorf("dtos[0] = %+v, want {checkout waiting-input}", dtos[0])
	}
	if dtos[1].Slice != "other" || dtos[1].Status != "none" {
		t.Errorf("dtos[1] = %+v, want {other none}", dtos[1])
	}
}

func TestSliceStatusesJSON(t *testing.T) {
	ws := makeTestWorkspace(t)
	sp := statusTestPaths(t)

	if err := notify.WriteStatus(sp.EventsDir, "checkout", model.SessRunning, 1); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	dtos, err := sliceStatuses(ws, sp)
	if err != nil {
		t.Fatalf("sliceStatuses: %v", err)
	}

	data, err := json.Marshal(dtos)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var out []StatusDTO
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json.Unmarshal roundtrip: %v", err)
	}

	var found bool
	for _, dto := range out {
		if dto.Slice == "checkout" {
			found = true
			if dto.Status != "running" {
				t.Errorf("checkout status = %q, want running", dto.Status)
			}
		}
	}
	if !found {
		t.Error("JSON output does not contain a 'checkout' status")
	}
}

// TestReadStatusSingleSlice covers the direct event lookup the single-arg
// `slis status <slice>` path uses: an unknown slice reports "none".
func TestReadStatusSingleSlice(t *testing.T) {
	sp := statusTestPaths(t)

	if got := notify.ReadStatus(sp.EventsDir, "nope").String(); got != "none" {
		t.Errorf("unknown slice status = %q, want none", got)
	}

	if err := notify.WriteStatus(sp.EventsDir, "feat", model.SessDone, 1); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	if got := notify.ReadStatus(sp.EventsDir, "feat").String(); got != "done" {
		t.Errorf("feat status = %q, want done", got)
	}
}

func TestSliceStatusFallsBackToLiveTmuxSession(t *testing.T) {
	if !tmuxctl.Available() {
		t.Skip("tmux not on PATH")
	}
	const slice = "status-live-fallback-test"
	_ = tmuxctl.KillSession(slice)
	t.Cleanup(func() { _ = tmuxctl.KillSession(slice) })
	if err := tmuxctl.EnsureSession(slice, nil, tmuxctl.SessionOpts{}); err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	sp := statusTestPaths(t)
	if got := report.SliceStatus(sp.EventsDir, slice); got != model.SessRunning {
		t.Fatalf("live session status = %v, want running", got)
	}

	// Hook events are more precise than process presence and retain precedence.
	if err := notify.WriteStatus(sp.EventsDir, slice, model.SessWaitingInput, 1); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}
	if got := report.SliceStatus(sp.EventsDir, slice); got != model.SessWaitingInput {
		t.Fatalf("hook status = %v, want waiting-input", got)
	}
}
