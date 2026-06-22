package notify_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

func TestWriteReadStatus(t *testing.T) {
	dir := t.TempDir()

	if err := notify.WriteStatus(dir, "checkout", model.SessWaitingInput, 123); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	got := notify.ReadStatus(dir, "checkout")
	if got != model.SessWaitingInput {
		t.Errorf("ReadStatus = %v, want %v", got, model.SessWaitingInput)
	}

	// Unknown slice returns SessNone.
	unknown := notify.ReadStatus(dir, "no-such-slice")
	if unknown != model.SessNone {
		t.Errorf("ReadStatus(unknown) = %v, want SessNone", unknown)
	}
}

func TestReadAllStatuses(t *testing.T) {
	dir := t.TempDir()

	if err := notify.WriteStatus(dir, "alpha", model.SessRunning, 1); err != nil {
		t.Fatalf("WriteStatus alpha: %v", err)
	}
	if err := notify.WriteStatus(dir, "beta", model.SessDone, 2); err != nil {
		t.Fatalf("WriteStatus beta: %v", err)
	}

	all := notify.ReadAllStatuses(dir)
	if len(all) != 2 {
		t.Fatalf("ReadAllStatuses len = %d, want 2", len(all))
	}
	if all["alpha"] != model.SessRunning {
		t.Errorf("alpha = %v, want SessRunning", all["alpha"])
	}
	if all["beta"] != model.SessDone {
		t.Errorf("beta = %v, want SessDone", all["beta"])
	}
}

func TestStatusFileNameSanitises(t *testing.T) {
	dir := t.TempDir()

	sliceName := "team/feature-x"
	if err := notify.WriteStatus(dir, sliceName, model.SessRunning, 1); err != nil {
		t.Fatalf("WriteStatus: %v", err)
	}

	// There must be a file in dir with no path separator in its basename.
	entries, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected a JSON file to be written")
	}
	for _, e := range entries {
		base := filepath.Base(e)
		if strings.Contains(base, "/") || strings.Contains(base, string(filepath.Separator)) {
			t.Errorf("filename %q contains path separator", base)
		}
	}

	// And reading it back must work.
	got := notify.ReadStatus(dir, sliceName)
	if got != model.SessRunning {
		t.Errorf("ReadStatus = %v, want SessRunning", got)
	}
}
