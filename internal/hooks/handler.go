// Package hooks maps Claude Code hook events to per-slice session statuses and
// persists them to the event store so the TUI can surface which slice needs
// user attention.
package hooks

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
)

// notifier delivers a desktop banner for a status change. It is a package var so
// tests can substitute a recorder; production uses notify.Notify.
var notifier = notify.Notify

// errOut receives non-fatal notification-delivery diagnostics. Overridable in
// tests; Claude Code surfaces a hook's stderr to the user.
var errOut io.Writer = os.Stderr

// hookInput is the subset of the Claude hook JSON payload we care about.
type hookInput struct {
	Cwd           string `json:"cwd"`
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
}

// StatusForEvent maps a Claude hook event name to the corresponding
// model.SessionStatus:
//
//	"Notification"                 → SessWaitingInput
//	"Stop" / "SubagentStop"         → SessDone
//	"UserPromptSubmit" / anything else → SessRunning
func StatusForEvent(event string) model.SessionStatus {
	switch event {
	case "Notification":
		return model.SessWaitingInput
	case "Stop", "SubagentStop":
		return model.SessDone
	case "UserPromptSubmit":
		return model.SessRunning
	default:
		return model.SessRunning
	}
}

// resolveSymlinks attempts to resolve symlinks for a path. On macOS /var is a
// symlink to /private/var, so we need to resolve both sides before comparing.
// If the full path doesn't exist, it walks up the tree to find the longest
// existing prefix, resolves that, and re-appends the remaining suffix.
func resolveSymlinks(p string) string {
	p = filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	// Walk up to find a resolvable prefix, then re-append the rest.
	dir := filepath.Dir(p)
	suffix := filepath.Base(p)
	for dir != p { // stop at filesystem root
		if r, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(r, suffix)
		}
		suffix = filepath.Join(filepath.Base(dir), suffix)
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return p
}

// SliceForCwd returns the name of the first slice whose member's WorktreePath
// is equal to, or is an ancestor directory of, cwd. Returns "" if none match.
// Both sides are symlink-resolved for robustness on macOS.
func SliceForCwd(slices []model.Slice, cwd string) string {
	resolvedCwd := resolveSymlinks(cwd)
	sep := string(os.PathSeparator)

	for _, sl := range slices {
		for _, m := range sl.Members {
			if m.WorktreePath == "" {
				continue
			}
			resolvedWt := resolveSymlinks(m.WorktreePath)
			if resolvedCwd == resolvedWt {
				return sl.Name
			}
			if strings.HasPrefix(resolvedCwd, resolvedWt+sep) {
				return sl.Name
			}
		}
	}
	return ""
}

// HandleHook decodes the Claude hook JSON from r, maps cwd → slice, and writes
// the slice's status to eventsDir. If the cwd cannot be matched to any slice,
// the call is a no-op (returns nil). Decode errors are silently swallowed so a
// misconfigured or empty hook payload never crashes the parent process.
//
// When the status *changes* to waiting-input or done, HandleHook fires a desktop
// notification directly (deduped: an unchanged status never re-fires). Firing is
// best-effort — a delivery failure is reported to stderr but never fails the
// hook. Persisting the status (WriteStatus) stays fatal.
//
// event is the hook event name supplied on the CLI; it takes precedence over
// the hook_event_name field in the JSON payload.
func HandleHook(event string, r io.Reader, slices []model.Slice, eventsDir string, cfg config.Notify, timeNS int64) error {
	var hi hookInput
	if err := json.NewDecoder(r).Decode(&hi); err != nil {
		// Tolerate empty or unparseable payloads — fail quiet.
		return nil
	}

	cwd := hi.Cwd
	if cwd == "" {
		return nil
	}

	// Prefer the explicit event arg; fall back to the JSON field.
	eName := event
	if eName == "" {
		eName = hi.HookEventName
	}

	sliceName := SliceForCwd(slices, cwd)
	if sliceName == "" {
		return nil
	}

	previous := notify.ReadStatus(eventsDir, sliceName)
	status := StatusForEvent(eName)
	if err := notify.WriteStatus(eventsDir, sliceName, status, timeNS); err != nil {
		return err
	}

	if status != previous {
		fireNotification(sliceName, status, cfg)
	}
	return nil
}

// selfExecutable resolves the running slis binary's path so a clicked banner can
// re-invoke it. It is a package var so tests can pin a deterministic value;
// production uses os.Executable, falling back to "slis" on PATH when the path
// cannot be determined.
var selfExecutable = func() string {
	if p, err := os.Executable(); err == nil && p != "" {
		return p
	}
	return "slis"
}

// shellSingleQuote wraps s in single quotes for safe interpolation into a
// /bin/sh command line (terminal-notifier's -execute runs its argument through
// the shell). A literal single quote is emitted as the standard '\” sequence.
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// focusCommand builds the shell command a clicked banner runs: the running slis
// binary invoked as `<self> focus <slice>`, with both the binary path and the
// slice name shell-quoted so spaces or quotes cannot break the command.
func focusCommand(slice string) string {
	return shellSingleQuote(selfExecutable()) + " focus " + shellSingleQuote(slice)
}

// fireNotification best-effort delivers a desktop banner for a slice that just
// entered an alertable status (waiting-input or done). Other statuses are
// silent. Clicking the banner runs `slis focus <slice>` (terminal-notifier
// only) to switch the user's tmux client to that slice's session. Delivery
// errors are reported to stderr, never propagated.
func fireNotification(slice string, status model.SessionStatus, cfg config.Notify) {
	var n notify.Notification
	switch status {
	case model.SessWaitingInput:
		n = notify.Notification{
			Title:    "slis",
			Subtitle: slice,
			Message:  slice + " needs your input",
			Sound:    cfg.NeedsInput.Sound,
		}
	case model.SessDone:
		n = notify.Notification{
			Title:    "slis",
			Subtitle: slice,
			Message:  slice + " finished — your move",
			Sound:    cfg.Done.Sound,
		}
	default:
		return
	}
	n.ExecuteOnClick = focusCommand(slice)
	n.Activate = cfg.Activate
	if err := notifier(n); err != nil {
		fmt.Fprintf(errOut, "slis hook: notification delivery failed: %v\n", err)
	}
}
