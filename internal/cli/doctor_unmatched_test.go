package cli

import (
	"testing"
	"time"

	"github.com/jonnyom/slis/internal/notify"
)

func TestUnmatchedHookFindings(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)

	t.Run("none recorded", func(t *testing.T) {
		if got := unmatchedHookFindings(t.TempDir(), now); len(got) != 0 {
			t.Errorf("findings = %+v, want none", got)
		}
	})

	t.Run("recent event warns", func(t *testing.T) {
		eventsDir := t.TempDir()
		hook := notify.UnmatchedHook{Event: "Stop", TimeNS: now.Add(-time.Hour).UnixNano(), Cwd: "/elsewhere"}
		if err := notify.WriteUnmatchedHook(eventsDir, hook); err != nil {
			t.Fatal(err)
		}
		got := unmatchedHookFindings(eventsDir, now)
		if len(got) != 1 || got[0].Level != lvlWarn {
			t.Fatalf("findings = %+v, want one warning", got)
		}
	})

	t.Run("old event is ignored", func(t *testing.T) {
		eventsDir := t.TempDir()
		hook := notify.UnmatchedHook{Event: "Stop", TimeNS: now.Add(-48 * time.Hour).UnixNano(), Cwd: "/elsewhere"}
		if err := notify.WriteUnmatchedHook(eventsDir, hook); err != nil {
			t.Fatal(err)
		}
		if got := unmatchedHookFindings(eventsDir, now); len(got) != 0 {
			t.Errorf("findings = %+v, want none", got)
		}
	})
}
