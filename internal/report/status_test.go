package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

func TestSliceStatusTreatsExpiredRecordsAsDone(t *testing.T) {
	eventsDir := t.TempDir()
	record := notify.Status{Slice: "feature", Status: model.SessRunning.String(), TimeNS: 1, SessionID: "s1"}
	if err := notify.WriteStatusRecord(eventsDir, record); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(eventsDir, "feature@s1.json"), old, old); err != nil {
		t.Fatal(err)
	}

	if got := SliceStatus(eventsDir, "feature"); got != model.SessDone {
		t.Errorf("SliceStatus = %v, want done", got)
	}
}

func TestSliceStatusWithHooksInstalledDoesNotGuessRunning(t *testing.T) {
	settings := filepath.Join(t.TempDir(), "settings.json")
	body := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"/opt/slis hook Stop"}]}]}}`
	if err := os.WriteFile(settings, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	restore := claudeSettingsPath
	claudeSettingsPath = func() string { return settings }
	t.Cleanup(func() { claudeSettingsPath = restore })

	if got := SliceStatus(t.TempDir(), "feature"); got != model.SessNone {
		t.Errorf("SliceStatus = %v, want none", got)
	}
}
