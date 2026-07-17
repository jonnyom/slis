// Package tmuxctl manages tmux sessions for slis slices.
// Each slice gets a session named "slis/<slice>" (sanitised), with one window
// per SliceMember whose cwd is the member's WorktreePath.
package tmuxctl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jonnyom/slis/internal/model"
)

var sanitiser = strings.NewReplacer(":", "-", ".", "-")

// SessionName returns the tmux session name for a slice. tmux disallows ':' and
// '.' in session names, so they are replaced with '-'. Format: "slis/<slice>".
func SessionName(slice string) string {
	return "slis/" + sanitiser.Replace(slice)
}

// Available reports whether the tmux binary is on PATH.
func Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// SessionExists reports whether the slice's tmux session exists.
func SessionExists(slice string) bool {
	err := exec.Command("tmux", "has-session", "-t", SessionName(slice)).Run()
	return err == nil
}

// SessionOpts controls the window layout of a slice's tmux session.
type SessionOpts struct {
	// Root is the workspace root. When set and the layout includes it, the
	// session opens a window there (so you can run Claude across the whole stack).
	Root string
	// Layout is "root", "repos", or "both". Empty defaults to "root" when Root is
	// set, else "repos".
	Layout string
}

// window is one tmux window: a name and a starting directory.
type window struct{ name, cwd string }

// perRepoWindows builds one window per member, each cd'd into its worktree.
func perRepoWindows(sorted []model.SliceMember) []window {
	wins := make([]window, 0, len(sorted))
	for _, m := range sorted {
		wins = append(wins, window{name: m.Repo, cwd: m.WorktreePath})
	}
	return wins
}

// rootWindowCwd returns the directory a "root" window should cd into so agents
// launched there operate on the slice worktrees, not the repos' primary
// checkouts. For a single member that is the member's worktree; for several it
// is their shared immediate parent (the created-slice layout,
// ws.Root/.slis/worktrees/<slice>/). ok is false when the members do not share
// one immediate parent (adopted/discovered slices with arbitrary paths), in
// which case a single root window cannot serve them all.
func rootWindowCwd(sorted []model.SliceMember) (string, bool) {
	if len(sorted) == 0 {
		return "", false
	}
	if len(sorted) == 1 {
		return sorted[0].WorktreePath, true
	}
	parent := filepath.Dir(sorted[0].WorktreePath)
	for _, m := range sorted[1:] {
		if filepath.Dir(m.WorktreePath) != parent {
			return "", false
		}
	}
	return parent, true
}

// sessionWindows builds the ordered window list for the given members + opts.
func sessionWindows(members []model.SliceMember, opts SessionOpts) []window {
	sorted := make([]model.SliceMember, len(members))
	copy(sorted, members)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Repo < sorted[j].Repo })

	layout := opts.Layout
	if layout == "" {
		if opts.Root != "" {
			layout = "root"
		} else {
			layout = "repos"
		}
	}

	var wins []window
	if (layout == "root" || layout == "both") && opts.Root != "" {
		cwd, ok := rootWindowCwd(sorted)
		if !ok {
			// Members don't share a common parent: a single root window can't
			// serve them all, so fall back to per-repo windows for this session.
			return perRepoWindows(sorted)
		}
		wins = append(wins, window{name: "root", cwd: cwd})
	}
	if layout == "repos" || layout == "both" {
		wins = append(wins, perRepoWindows(sorted)...)
	}
	// Fallback: if the chosen layout produced nothing (e.g. "root" with no Root),
	// fall back to per-repo windows so the session is still useful.
	if len(wins) == 0 {
		return perRepoWindows(sorted)
	}
	return wins
}

// detachHint is the reminder shown in a slis session's status bar. Claude exits
// on Ctrl-D (EOF), which users reach for instinctively when they mean "leave";
// the correct, Claude-preserving way out is the tmux prefix detach (C-b d).
const detachHint = " detach: C-b d  (Ctrl-D quits Claude) "

// setStatusHint writes detachHint into the named session's status bar. It is set
// per-session (no -g) so it only affects slis-owned sessions, never the user's
// other tmux sessions. Best-effort: any failure (e.g. status bar disabled) is
// ignored — it is a hint, not load-bearing.
func setStatusHint(name string) {
	_ = exec.Command("tmux", "set-option", "-t", name, "status-right-length", "40").Run()
	_ = exec.Command("tmux", "set-option", "-t", name, "status-right", detachHint).Run()
}

// EnsureSession creates the slice's tmux session (detached) if it does not
// already exist, with windows determined by opts (a root window, per-repo
// windows, or both — see SessionOpts). Idempotent: returns nil if the session
// already exists. The first window becomes the session's initial (attached) one.
// Either way the session's status bar is (re)stamped with the detach hint.
func EnsureSession(slice string, members []model.SliceMember, opts SessionOpts) error {
	name := SessionName(slice)
	if SessionExists(slice) {
		setStatusHint(name)
		return nil
	}

	wins := sessionWindows(members, opts)
	if len(wins) == 0 {
		// No members and no root: create a bare session.
		if out, err := exec.Command("tmux", "new-session", "-d", "-s", name).CombinedOutput(); err != nil {
			return fmt.Errorf("tmux new-session: %w: %s", err, out)
		}
		setStatusHint(name)
		return nil
	}

	first := wins[0]
	args := []string{"new-session", "-d", "-s", name, "-n", first.name}
	if first.cwd != "" {
		args = append(args, "-c", first.cwd)
	}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session: %w: %s", err, out)
	}

	for _, w := range wins[1:] {
		a := []string{"new-window", "-t", name, "-n", w.name}
		if w.cwd != "" {
			a = append(a, "-c", w.cwd)
		}
		if out, err := exec.Command("tmux", a...).CombinedOutput(); err != nil {
			return fmt.Errorf("tmux new-window %q: %w: %s", w.name, err, out)
		}
	}

	setStatusHint(name)
	return nil
}

// CapturePane returns the visible contents of each window's active pane in the
// slice's session, prefixed by a "── <window> ──" header — a read-only peek at
// what is running (e.g. a Claude session) without attaching. Returns an error
// if the session does not exist.
func CapturePane(slice string) (string, error) {
	name := SessionName(slice)
	out, err := exec.Command("tmux", "list-windows", "-t", name, "-F", "#{window_index}\t#{window_name}").Output()
	if err != nil {
		return "", fmt.Errorf("tmux list-windows: %w", err)
	}

	var sb strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line == "" {
			continue
		}
		idx, wname, _ := strings.Cut(line, "\t")
		// -e preserves the pane's colour escapes (slis keeps SGR, strips the rest).
		captured, _ := exec.Command("tmux", "capture-pane", "-p", "-e", "-t", name+":"+idx).Output()
		sb.WriteString("── " + wname + " ──\n")
		sb.Write(captured)
		if !strings.HasSuffix(string(captured), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// SendKeys types keys into the slice session's active pane followed by Enter
// (used to launch an agent). No-op error if the session is absent.
func SendKeys(slice, keys string) error {
	err := exec.Command("tmux", "send-keys", "-t", SessionName(slice), keys, "Enter").Run()
	if err != nil {
		return fmt.Errorf("tmux send-keys: %w", err)
	}
	return nil
}

// ActivePaneCommand returns the foreground command of the session's active pane
// (e.g. "zsh", "node", "claude"), or "" if it can't be determined.
func ActivePaneCommand(slice string) string {
	out, err := exec.Command("tmux", "display-message", "-p", "-t", SessionName(slice), "#{pane_current_command}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// PanePIDs returns the pane PIDs across all windows of the slice's session.
func PanePIDs(slice string) ([]int, error) {
	name := SessionName(slice)
	out, err := exec.Command("tmux", "list-panes", "-s", "-t", name, "-F", "#{pane_pid}").Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}

	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("parsing pane pid %q: %w", line, err)
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// AttachArgv returns the argv to attach to the slice's session, choosing
// switch-client when already inside tmux ($TMUX set) or attach otherwise.
// Returned as (name, args) so it is unit-testable without spawning tmux.
func AttachArgv(slice string, insideTmux bool) (string, []string) {
	target := SessionName(slice)
	if insideTmux {
		return "tmux", []string{"switch-client", "-t", target}
	}
	return "tmux", []string{"attach", "-t", target}
}

// Attach attaches the current terminal to the slice's session (switch-client if
// inside tmux, else attach with inherited stdio). Blocks until detach when attaching.
func Attach(slice string) error {
	name, args := AttachArgv(slice, os.Getenv("TMUX") != "")
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Client is a single attached tmux client: its controlling TTY, the session it
// is currently viewing, and its last-activity time (unix seconds) used to pick
// the most recently used client to steal focus onto.
type Client struct {
	TTY          string
	Session      string
	LastActivity int64
}

// clientListFormat is the -F format string for ListClients: tty, session name,
// and last-activity timestamp, tab-separated.
const clientListFormat = "#{client_tty}\t#{client_session}\t#{client_activity}"

// parseClients parses `tmux list-clients -F clientListFormat` output. Lines that
// are blank, malformed, or missing a TTY are skipped. Pure — unit-testable
// without spawning tmux.
func parseClients(raw string) []Client {
	var clients []Client
	for _, line := range strings.Split(strings.TrimRight(raw, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		tty := strings.TrimSpace(parts[0])
		if tty == "" {
			continue
		}
		activity, _ := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		clients = append(clients, Client{
			TTY:          tty,
			Session:      strings.TrimSpace(parts[1]),
			LastActivity: activity,
		})
	}
	return clients
}

// ListClients returns the currently attached tmux clients. tmux only lists
// attached clients, so an empty slice means nobody is attached anywhere.
func ListClients() ([]Client, error) {
	out, err := exec.Command("tmux", "list-clients", "-F", clientListFormat).Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-clients: %w", err)
	}
	return parseClients(string(out)), nil
}

// MostRecentClient returns the client with the greatest last-activity time (the
// one the user most recently used). ok is false when clients is empty.
func MostRecentClient(clients []Client) (Client, bool) {
	if len(clients) == 0 {
		return Client{}, false
	}
	best := clients[0]
	for _, c := range clients[1:] {
		if c.LastActivity > best.LastActivity {
			best = c
		}
	}
	return best, true
}

// SwitchClientArgv returns the argv to point a specific client (by its TTY) at
// the slice's session. Returned as (name, args) so it is unit-testable without
// spawning tmux.
func SwitchClientArgv(clientTTY, slice string) (string, []string) {
	return "tmux", []string{"switch-client", "-c", clientTTY, "-t", SessionName(slice)}
}

// KillSession kills the slice's session (used for cleanup/teardown).
// Returns nil if the session does not exist.
func KillSession(slice string) error {
	err := exec.Command("tmux", "kill-session", "-t", SessionName(slice)).Run()
	if err != nil {
		// Tolerate "session not found" errors.
		if !SessionExists(slice) {
			return nil
		}
		return fmt.Errorf("tmux kill-session: %w", err)
	}
	return nil
}
