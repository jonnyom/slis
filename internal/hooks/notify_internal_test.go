package hooks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

// captureNotifier swaps the package notifier for a recorder and returns the
// recorded notifications plus a restore func.
func captureNotifier(t *testing.T) (*[]notify.Notification, func()) {
	t.Helper()
	var fired []notify.Notification
	orig := notifier
	notifier = func(n notify.Notification) error {
		fired = append(fired, n)
		return nil
	}
	return &fired, func() { notifier = orig }
}

func oneSliceAt(dir string) []model.Slice {
	return []model.Slice{
		{
			Name: "alpha",
			Members: map[string]model.SliceMember{
				"repo-a": {Repo: "repo-a", Branch: "feat/alpha", WorktreePath: dir},
			},
		},
	}
}

func sendHook(t *testing.T, event, worktree, eventsDir string, cfg config.Notify, ts int64) {
	t.Helper()
	body := fmt.Sprintf(`{"cwd":%q,"hook_event_name":%q}`, worktree, event)
	if err := HandleHook(event, strings.NewReader(body), oneSliceAt(worktree), eventsDir, cfg, ts); err != nil {
		t.Fatalf("HandleHook(%s): %v", event, err)
	}
}

func TestHandleHookFiresOnceThenDedupes(t *testing.T) {
	fired, restore := captureNotifier(t)
	defer restore()

	wt := t.TempDir()
	ev := t.TempDir()
	cfg := config.Notify{}

	// none → waiting-input: fires.
	sendHook(t, "Notification", wt, ev, cfg, 1)
	// waiting-input → waiting-input: no change, must not fire again.
	sendHook(t, "Notification", wt, ev, cfg, 2)

	if len(*fired) != 1 {
		t.Fatalf("want exactly 1 notification, got %d: %v", len(*fired), *fired)
	}
	got := (*fired)[0]
	if got.Subtitle != "alpha" {
		t.Errorf("subtitle = %q, want alpha", got.Subtitle)
	}
	if !strings.Contains(got.Message, "alpha") {
		t.Errorf("message %q should mention slice name", got.Message)
	}
}

func TestHandleHookRunningToWaitingFires(t *testing.T) {
	fired, restore := captureNotifier(t)
	defer restore()

	wt := t.TempDir()
	ev := t.TempDir()
	cfg := config.Notify{}

	// none → running: running is not an alertable status, no banner.
	sendHook(t, "UserPromptSubmit", wt, ev, cfg, 1)
	if len(*fired) != 0 {
		t.Fatalf("running should not fire, got %v", *fired)
	}
	// running → waiting-input: fires.
	sendHook(t, "Notification", wt, ev, cfg, 2)
	if len(*fired) != 1 || (*fired)[0].Subtitle != "alpha" {
		t.Fatalf("running→waiting-input should fire once, got %v", *fired)
	}
}

func TestHandleHookWaitingToRunningSilent(t *testing.T) {
	fired, restore := captureNotifier(t)
	defer restore()

	wt := t.TempDir()
	ev := t.TempDir()
	cfg := config.Notify{}

	sendHook(t, "Notification", wt, ev, cfg, 1)     // → waiting-input (fires)
	sendHook(t, "UserPromptSubmit", wt, ev, cfg, 2) // waiting-input → running (silent)

	if len(*fired) != 1 {
		t.Fatalf("waiting→running must be silent, got %d notifications: %v", len(*fired), *fired)
	}
}

func TestHandleHookDoneFires(t *testing.T) {
	fired, restore := captureNotifier(t)
	defer restore()

	wt := t.TempDir()
	ev := t.TempDir()
	cfg := config.Notify{}

	sendHook(t, "UserPromptSubmit", wt, ev, cfg, 1) // → running (silent)
	sendHook(t, "Stop", wt, ev, cfg, 2)             // running → done (fires)

	if len(*fired) != 1 {
		t.Fatalf("done should fire once, got %v", *fired)
	}
	if !strings.Contains((*fired)[0].Message, "alpha") {
		t.Errorf("done message %q should mention slice", (*fired)[0].Message)
	}
}

func TestHandleHookAppliesConfiguredSound(t *testing.T) {
	fired, restore := captureNotifier(t)
	defer restore()

	wt := t.TempDir()
	ev := t.TempDir()
	cfg := config.Notify{
		NeedsInput: config.NotifyChannel{Sound: "Ping"},
		Done:       config.NotifyChannel{Sound: "Glass"},
	}

	sendHook(t, "Notification", wt, ev, cfg, 1) // waiting-input → sound Ping
	sendHook(t, "Stop", wt, ev, cfg, 2)         // done → sound Glass

	if len(*fired) != 2 {
		t.Fatalf("want 2 notifications, got %d: %v", len(*fired), *fired)
	}
	if (*fired)[0].Sound != "Ping" {
		t.Errorf("waiting-input sound = %q, want Ping", (*fired)[0].Sound)
	}
	if (*fired)[1].Sound != "Glass" {
		t.Errorf("done sound = %q, want Glass", (*fired)[1].Sound)
	}
}
