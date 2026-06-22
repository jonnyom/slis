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
	title := "slis"
	message := "alpha needs input"

	name, args, ok := notify.DesktopNotifyArgv(title, message)

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

	// The message and title must appear somewhere in the args.
	found := false
	for _, a := range args {
		if a == message || (len(a) > 0 && containsSubstr(a, message)) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DesktopNotifyArgv: message %q not found in args %v", message, args)
	}
}

// TestDesktopNotifyArgvTitlePresent verifies the title is included in the result.
func TestDesktopNotifyArgvTitlePresent(t *testing.T) {
	title := "mytitle"
	message := "mymessage"

	name, args, ok := notify.DesktopNotifyArgv(title, message)
	if !ok {
		t.Skip("platform not supported")
	}
	if name == "" {
		t.Error("name must be non-empty")
	}

	// Title must appear somewhere in the combined argv.
	found := false
	for _, a := range args {
		if containsSubstr(a, title) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("title %q not found in args %v", title, args)
	}
}

// TestNotifyNoError verifies that Notify returns nil even when the tool is not
// present (best-effort, no panic).
func TestNotifyNoError(t *testing.T) {
	// Notify is best-effort — it must never return an error even if the
	// notification binary is missing.
	err := notify.Notify("test", "test message")
	if err != nil {
		t.Errorf("Notify returned non-nil error: %v", err)
	}
}

func containsSubstr(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && (s == sub || func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
