package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// DesktopNotifyArgv returns the argv to show a desktop notification on this OS.
//
//   - darwin → osascript -e 'display notification "<msg>" with title "<title>"'
//   - linux  → notify-send "<title>" "<msg>"
//   - other  → ok=false
func DesktopNotifyArgv(title, message string) (name string, args []string, ok bool) {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf("display notification %q with title %q", message, title)
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
