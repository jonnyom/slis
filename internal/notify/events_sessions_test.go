package notify_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

func writeSessionStatus(t *testing.T, dir, slice, sessionID string, status model.SessionStatus, subagents int) {
	t.Helper()
	if err := notify.WriteStatusRecord(dir, notify.Status{
		Slice:     slice,
		Status:    status.String(),
		TimeNS:    1,
		SessionID: sessionID,
		Subagents: subagents,
	}); err != nil {
		t.Fatalf("WriteStatusRecord: %v", err)
	}
}

func TestSessionsInOneSliceKeepSeparateStatuses(t *testing.T) {
	dir := t.TempDir()
	writeSessionStatus(t, dir, "checkout", "first", model.SessWaitingInput, 0)
	writeSessionStatus(t, dir, "checkout", "second", model.SessDone, 0)

	if got := notify.ReadStatus(dir, "checkout"); got != model.SessWaitingInput {
		t.Fatalf("ReadStatus = %v, want the most urgent session status waiting-input", got)
	}
	if got := notify.ReadStatusRecord(dir, "checkout").SessionID; got != "first" {
		t.Fatalf("ReadStatusRecord session = %q, want the waiting session", got)
	}
}

func TestSliceSummaryPrefersUrgentStatusesAndCountsSubagents(t *testing.T) {
	dir := t.TempDir()
	writeSessionStatus(t, dir, "checkout", "done", model.SessDone, 0)
	writeSessionStatus(t, dir, "checkout", "background-a", model.SessSubagents, 2)
	writeSessionStatus(t, dir, "checkout", "background-b", model.SessSubagents, 1)

	got := notify.ReadAllStatuses(dir)["checkout"]
	want := notify.SliceSummary{Status: model.SessSubagents, Subagents: 3}
	if got != want {
		t.Fatalf("summary = %+v, want %+v", got, want)
	}

	writeSessionStatus(t, dir, "checkout", "main", model.SessRunning, 0)
	got = notify.ReadAllStatuses(dir)["checkout"]
	if got.Status != model.SessRunning {
		t.Fatalf("summary status = %v, want running to outrank sub-agents", got.Status)
	}
}

func TestStatusesOlderThanADayAreIgnored(t *testing.T) {
	dir := t.TempDir()
	writeSessionStatus(t, dir, "checkout", "old", model.SessWaitingInput, 0)
	old := time.Now().Add(-25 * time.Hour)
	matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	for _, path := range matches {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}

	if got := notify.ReadStatus(dir, "checkout"); got != model.SessNone {
		t.Fatalf("ReadStatus = %v, want none for a day-old record", got)
	}
	if got := notify.ReadAllStatuses(dir); len(got) != 0 {
		t.Fatalf("ReadAllStatuses = %v, want empty", got)
	}
}

func TestRemoveSessionStatusLeavesOtherSessions(t *testing.T) {
	dir := t.TempDir()
	writeSessionStatus(t, dir, "checkout", "ended", model.SessWaitingInput, 0)
	writeSessionStatus(t, dir, "checkout", "alive", model.SessRunning, 0)

	if err := notify.RemoveSessionStatus(dir, "checkout", "ended"); err != nil {
		t.Fatal(err)
	}
	if got := notify.ReadStatus(dir, "checkout"); got != model.SessRunning {
		t.Fatalf("ReadStatus = %v, want running", got)
	}
}

func TestRemoveStatusClearsEverySessionOfTheSlice(t *testing.T) {
	dir := t.TempDir()
	writeSessionStatus(t, dir, "checkout", "first", model.SessRunning, 0)
	writeSessionStatus(t, dir, "checkout", "second", model.SessDone, 0)
	writeSessionStatus(t, dir, "checkout-api", "third", model.SessDone, 0)
	if err := notify.WriteStatus(dir, "checkout", model.SessDone, 1); err != nil {
		t.Fatal(err)
	}

	if err := notify.RemoveStatus(dir, "checkout"); err != nil {
		t.Fatal(err)
	}
	all := notify.ReadAllStatuses(dir)
	if _, ok := all["checkout"]; ok {
		t.Fatalf("checkout still has a status: %v", all)
	}
	if all["checkout-api"].Status != model.SessDone {
		t.Fatalf("RemoveStatus touched another slice: %v", all)
	}
}
