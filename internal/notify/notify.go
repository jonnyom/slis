package notify

import (
	"os/exec"
	"runtime"
	"strings"
)

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

// DesktopNotifyArgv returns the argv to show a desktop notification on this OS.
//
//   - darwin → osascript -e 'display notification "<msg>" with title "<title>"'
//   - linux  → notify-send "<title>" "<msg>"
//   - other  → ok=false
//
// On darwin the message/title are embedded in an AppleScript string literal, so
// they are escaped for AppleScript (not Go) to avoid breaking out of the script.
// On linux they are passed as separate argv elements (no shell), so no escaping
// is needed.
func DesktopNotifyArgv(title, message string) (name string, args []string, ok bool) {
	switch runtime.GOOS {
	case "darwin":
		script := "display notification \"" + escapeAppleScript(message) +
			"\" with title \"" + escapeAppleScript(title) + "\""
		return "osascript", []string{"-e", script}, true
	case "linux":
		return "notify-send", []string{title, message}, true
	default:
		return "", nil, false
	}
}

// Notify shows a desktop notification (best-effort; returns nil if unsupported
// or the tool is missing).
func Notify(title, message string) error {
	name, args, ok := DesktopNotifyArgv(title, message)
	if !ok {
		return nil
	}
	if _, err := exec.LookPath(name); err != nil {
		// Tool not installed — silently ignore.
		return nil
	}
	// Run best-effort; ignore errors (notifications are non-critical).
	_ = exec.Command(name, args...).Run()
	return nil
}
