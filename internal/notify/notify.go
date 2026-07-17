package notify

import (
	"os/exec"
	"runtime"
	"strings"
)

// Notification is the content of a single desktop banner.
type Notification struct {
	Title    string
	Subtitle string
	Message  string
	Sound    string // backend-specific sound name; empty = silent
}

// escapeAppleScript turns s into the body of an AppleScript double-quoted string
// literal. AppleScript (unlike Go) only recognises \" and \\ as escapes, so we
// escape backslash and double-quote and drop control characters (a raw newline
// or ESC would otherwise break out of the `-e` script or inject sequences).
func escapeAppleScript(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r < 0x20 || r == 0x7f:
			// drop control characters (incl. newline, ESC)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// terminalNotifierArgs builds the argv tail for the terminal-notifier backend.
func terminalNotifierArgs(n Notification) []string {
	args := []string{"-title", n.Title, "-message", n.Message}
	if n.Subtitle != "" {
		args = append(args, "-subtitle", n.Subtitle)
	}
	if n.Sound != "" {
		args = append(args, "-sound", n.Sound)
	}
	return args
}

// appleScript builds the `display notification` AppleScript for the osascript
// backend. All user-supplied fields are AppleScript-escaped.
func appleScript(n Notification) string {
	script := "display notification \"" + escapeAppleScript(n.Message) +
		"\" with title \"" + escapeAppleScript(n.Title) + "\""
	if n.Subtitle != "" {
		script += " subtitle \"" + escapeAppleScript(n.Subtitle) + "\""
	}
	if n.Sound != "" {
		script += " sound name \"" + escapeAppleScript(n.Sound) + "\""
	}
	return script
}

// argvFor is the pure backend selector: given a target OS and a PATH probe it
// picks a notification backend and builds its argv, without running anything.
//
//   - darwin + terminal-notifier on PATH → terminal-notifier -title/-subtitle/-message[-sound]
//   - darwin otherwise                   → osascript -e '<AppleScript>'
//   - linux                              → notify-send <title> <message>
//   - anything else                      → ok=false
func argvFor(goos string, lookPath func(string) (string, error), n Notification) (name string, args []string, ok bool) {
	switch goos {
	case "darwin":
		if lookPath != nil {
			if _, err := lookPath("terminal-notifier"); err == nil {
				return "terminal-notifier", terminalNotifierArgs(n), true
			}
		}
		return "osascript", []string{"-e", appleScript(n)}, true
	case "linux":
		return "notify-send", []string{n.Title, n.Message}, true
	default:
		return "", nil, false
	}
}

// DesktopNotifyArgv builds the notification argv for the current OS, probing
// PATH via lookPath (pass exec.LookPath in production) to prefer
// terminal-notifier on macOS. It is pure — it never executes a process.
func DesktopNotifyArgv(lookPath func(string) (string, error), n Notification) (name string, args []string, ok bool) {
	return argvFor(runtime.GOOS, lookPath, n)
}

// Notify shows a desktop notification (best-effort). It returns an error only
// when the chosen backend was found and executed but failed; an unsupported OS
// or a missing backend binary is not an error (returns nil).
func Notify(n Notification) error {
	name, args, ok := DesktopNotifyArgv(exec.LookPath, n)
	if !ok {
		return nil
	}
	if _, err := exec.LookPath(name); err != nil {
		// Backend not installed — nothing to deliver.
		return nil
	}
	return exec.Command(name, args...).Run()
}
