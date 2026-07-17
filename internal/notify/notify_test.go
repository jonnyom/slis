package notify_test

import (
	"testing"

	"github.com/jonnyom/slis/internal/notify"
)

// TestDesktopNotifyArgv verifies that DesktopNotifyArgv returns a sensible result.
// On supported platforms (darwin, linux) it returns ok=true with a non-empty
// command name and at least one argument.  On unsupported platforms it returns
// ok=false and we just verify no panic occurred.
func TestDesktopNotifyArgv(t *testing.T) {
	n := notify.Notification{
		Title:    "slis",
		Subtitle: "alpha",
		Message:  "alpha needs input",
	}

	name, args, ok := notify.DesktopNotifyArgv(nil, n)

	if !ok {
		// Unsupported platform — just confirm no panic.
		t.Log("DesktopNotifyArgv: platform not supported (ok=false) — skipping content checks")
		return
	}

	if name == "" {
		t.Error("DesktopNotifyArgv: name must be non-empty when ok=true")
	}
	if len(args) == 0 {
		t.Error("DesktopNotifyArgv: args must be non-empty when ok=true")
	}

	// The message must appear somewhere in the args.
	found := false
	for _, a := range args {
		if containsSubstr(a, n.Message) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DesktopNotifyArgv: message %q not found in args %v", n.Message, args)
	}
}

// TestDesktopNotifyArgvTitlePresent verifies the title is included in the result.
func TestDesktopNotifyArgvTitlePresent(t *testing.T) {
	n := notify.Notification{Title: "mytitle", Message: "mymessage"}

	name, args, ok := notify.DesktopNotifyArgv(nil, n)
	if !ok {
		t.Skip("platform not supported")
	}
	if name == "" {
		t.Error("name must be non-empty")
	}

	found := false
	for _, a := range args {
		if containsSubstr(a, n.Title) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("title %q not found in args %v", n.Title, args)
	}
}

// TestNotifyBestEffort verifies that Notify never panics on this platform. It is
// best-effort: the return value is tolerated (a headless CI backend may fail),
// and a missing/unsupported backend must return nil.
func TestNotifyBestEffort(t *testing.T) {
	_ = notify.Notify(notify.Notification{Title: "test", Message: "test message"})
}

func containsSubstr(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
