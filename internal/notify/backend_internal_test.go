package notify

import (
	"errors"
	"testing"
)

// found is a lookPath fake that reports every probed binary as present.
func found(string) (string, error) { return "/usr/bin/stub", nil }

// onlyOsascript reports terminal-notifier as missing (so darwin falls back to
// osascript) but everything else as present.
func onlyOsascript(name string) (string, error) {
	if name == "terminal-notifier" {
		return "", errors.New("not found")
	}
	return "/usr/bin/stub", nil
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func TestArgvForBackendSelection(t *testing.T) {
	n := Notification{
		Title:    "slis",
		Subtitle: "alpha",
		Message:  "alpha needs your input",
		Sound:    "Ping",
	}

	t.Run("darwin prefers terminal-notifier when present", func(t *testing.T) {
		name, args, ok := argvFor("darwin", found, n, "")
		if !ok || name != "terminal-notifier" {
			t.Fatalf("got name=%q ok=%v, want terminal-notifier", name, ok)
		}
		for _, want := range []string{n.Title, n.Subtitle, n.Message, n.Sound} {
			if !contains(args, want) {
				t.Errorf("terminal-notifier args %v missing %q", args, want)
			}
		}
		if !contains(args, "-sound") {
			t.Errorf("terminal-notifier args %v missing -sound flag", args)
		}
	})

	t.Run("darwin falls back to osascript when terminal-notifier absent", func(t *testing.T) {
		name, args, ok := argvFor("darwin", onlyOsascript, n, "")
		if !ok || name != "osascript" {
			t.Fatalf("got name=%q ok=%v, want osascript", name, ok)
		}
		if len(args) != 2 || args[0] != "-e" {
			t.Fatalf("osascript args = %v, want [-e <script>]", args)
		}
		script := args[1]
		for _, want := range []string{n.Message, n.Title, n.Subtitle, n.Sound} {
			if !containsSub(script, want) {
				t.Errorf("osascript script %q missing %q", script, want)
			}
		}
	})

	t.Run("darwin with nil lookPath uses osascript", func(t *testing.T) {
		name, _, ok := argvFor("darwin", nil, n, "")
		if !ok || name != "osascript" {
			t.Fatalf("got name=%q ok=%v, want osascript", name, ok)
		}
	})

	t.Run("linux uses notify-send", func(t *testing.T) {
		name, args, ok := argvFor("linux", found, n, "")
		if !ok || name != "notify-send" {
			t.Fatalf("got name=%q ok=%v, want notify-send", name, ok)
		}
		if !contains(args, n.Title) || !contains(args, n.Message) {
			t.Errorf("notify-send args %v missing title/message", args)
		}
	})

	t.Run("unsupported OS returns ok=false", func(t *testing.T) {
		_, _, ok := argvFor("plan9", found, n, "")
		if ok {
			t.Errorf("argvFor(plan9) ok=true, want false")
		}
	})

	t.Run("empty sound omits sound flags", func(t *testing.T) {
		plain := Notification{Title: "slis", Subtitle: "alpha", Message: "hi"}
		_, args, _ := argvFor("darwin", found, plain, "")
		if contains(args, "-sound") {
			t.Errorf("terminal-notifier args %v should not include -sound when Sound empty", args)
		}
		_, osArgs, _ := argvFor("darwin", onlyOsascript, plain, "")
		if containsSub(osArgs[1], "sound name") {
			t.Errorf("osascript script %q should not include sound when Sound empty", osArgs[1])
		}
	})

	t.Run("terminal-notifier sets icon flags when path given", func(t *testing.T) {
		_, args, _ := argvFor("darwin", found, n, "/tmp/slis.png")
		for _, want := range []string{"-appIcon", "-contentImage", "/tmp/slis.png"} {
			if !contains(args, want) {
				t.Errorf("terminal-notifier args %v missing %q", args, want)
			}
		}
	})

	t.Run("terminal-notifier omits icon flags when path empty", func(t *testing.T) {
		_, args, _ := argvFor("darwin", found, n, "")
		for _, unwanted := range []string{"-appIcon", "-contentImage"} {
			if contains(args, unwanted) {
				t.Errorf("terminal-notifier args %v should not include %q when icon empty", args, unwanted)
			}
		}
	})

	t.Run("osascript ignores icon path", func(t *testing.T) {
		_, args, _ := argvFor("darwin", onlyOsascript, n, "/tmp/slis.png")
		if contains(args, "/tmp/slis.png") || contains(args, "-appIcon") {
			t.Errorf("osascript args %v should not reference the icon", args)
		}
	})
}

func containsSub(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}
