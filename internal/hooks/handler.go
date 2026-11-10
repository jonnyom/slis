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
	Cwd              string           `json:"cwd"`
	SessionID        string           `json:"session_id"`
	HookEventName    string           `json:"hook_event_name"`
	NotificationType string           `json:"notification_type"`
	AgentID          string           `json:"agent_id"`
	BackgroundTasks  []backgroundTask `json:"background_tasks"`
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

// SliceForCwd returns the name of the slice that owns cwd, or "" if none does.
// A member worktree, or a directory inside one, wins first. Failing that, cwd
// may be a slice's root (`slis create` lays worktrees out as <root>/<repo>,
// with the root named after the slice), where a multi-repo session usually
// starts. Both sides are symlink-resolved for robustness on macOS.
func SliceForCwd(slices []model.Slice, cwd string) string {
	resolvedCwd := resolveSymlinks(cwd)

	for _, sl := range slices {
		for _, m := range sl.Members {
			if m.WorktreePath != "" && isWithin(resolvedCwd, resolveSymlinks(m.WorktreePath)) {
				return sl.Name
			}
		}
	}
	for _, sl := range slices {
		root := sliceRoot(sl)
		if root != "" && isWithin(resolvedCwd, root) && !holdsOtherSlice(slices, sl.Name, root) {
			return sl.Name
		}
	}
	return ""
}

// inSlisWorktrees reports whether cwd is inside the directory `slis create`
// puts worktrees in. Only a miss there is worth reporting: sessions elsewhere
// were never meant to belong to a slice.
func inSlisWorktrees(cwd string) bool {
	sep := string(os.PathSeparator)
	return strings.Contains(cwd+sep, sep+filepath.Join(".slis", "worktrees")+sep)
}

func isWithin(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(os.PathSeparator))
}

// sliceRoot returns the directory named after the slice that holds every member
// worktree, or "" when the members do not share one. Requiring the name keeps a
// worktree adopted from a broad directory like ~/code from claiming all of it.
func sliceRoot(sl model.Slice) string {
	root := ""
	for _, m := range sl.Members {
		if m.WorktreePath == "" {
			return ""
		}
		parent := filepath.Dir(resolveSymlinks(m.WorktreePath))
		if root != "" && parent != root {
			return ""
		}
		root = parent
	}
	if filepath.Base(root) != sl.Name {
		return ""
	}
	return root
}

// holdsOtherSlice reports whether root also contains another slice's worktree,
// as a shared directory like ~/code would. Such a root identifies no one slice.
func holdsOtherSlice(slices []model.Slice, owner, root string) bool {
	for _, sl := range slices {
		if sl.Name == owner {
			continue
		}
		for _, m := range sl.Members {
			if m.WorktreePath != "" && isWithin(resolveSymlinks(m.WorktreePath), root) {
				return true
			}
		}
	}
	return false
}

// HandleHook decodes the Claude hook JSON from r, maps cwd → slice, and writes
// the slice's status to eventsDir. A cwd outside every slice falls back to the
// slice the session last reported for. An event from slis's worktrees directory
// that no slice claims is recorded for `slis doctor`; any other is ignored. Decode errors are silently swallowed so a
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
		sliceName = notify.SliceForSession(eventsDir, hi.SessionID)
	}
	if sliceName == "" && !inSlisWorktrees(cwd) {
		return nil
	}
	if sliceName == "" {
		return notify.WriteUnmatchedHook(eventsDir, notify.UnmatchedHook{
			Event:     eName,
			TimeNS:    timeNS,
			SessionID: hi.SessionID,
			Cwd:       cwd,
		})
	}

	previous := notify.ParseStatus(notify.ReadSessionStatus(eventsDir, sliceName, hi.SessionID).Status)
	transition := transitionFor(eName, hi, previous)
	if transition.endSession {
		return notify.RemoveSessionStatus(eventsDir, sliceName, hi.SessionID)
	}
	if transition.unchanged {
		return nil
	}
	if err := notify.WriteStatusRecord(eventsDir, notify.Status{
		Slice:     sliceName,
		Status:    transition.status.String(),
		TimeNS:    timeNS,
		SessionID: hi.SessionID,
		Cwd:       cwd,
		Subagents: transition.subagents,
	}); err != nil {
		return err
	}

	if transition.status != previous {
		fireNotification(sliceName, transition.status, cfg)
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
func focusCommand(slice, terminalApp string) string {
	prefix := ""
	if terminalApp != "" {
		prefix = "SLIS_TERMINAL_APP=" + shellSingleQuote(terminalApp) + " "
	}
	return prefix + shellSingleQuote(selfExecutable()) + " focus " + shellSingleQuote(slice)
}

func notificationTerminal(configuredBundleID string) (string, string) {
	if configuredBundleID != "" {
		if configuredBundleID == "com.mitchellh.ghostty" {
			return configuredBundleID, "ghostty"
		}
		return configuredBundleID, ""
	}
	if strings.EqualFold(os.Getenv("SLIS_TERMINAL_APP"), "ghostty") {
		return "com.mitchellh.ghostty", "ghostty"
	}
	if strings.EqualFold(os.Getenv("TERM_PROGRAM"), "ghostty") {
		return "com.mitchellh.ghostty", "ghostty"
	}
	return "", ""
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
	activate, terminalApp := notificationTerminal(cfg.Activate)
	n.Activate = activate
	n.ExecuteOnClick = focusCommand(slice, terminalApp)
	if err := notifier(n); err != nil {
		fmt.Fprintf(errOut, "slis hook: notification delivery failed: %v\n", err)
	}
}
