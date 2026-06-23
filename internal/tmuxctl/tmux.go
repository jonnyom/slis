// Package tmuxctl manages tmux sessions for slis slices.
// Each slice gets a session named "slis/<slice>" (sanitised), with one window
// per SliceMember whose cwd is the member's WorktreePath.
package tmuxctl

import (
	"fmt"
	"os"
	"os/exec"
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
		wins = append(wins, window{name: "root", cwd: opts.Root})
	}
	if layout == "repos" || layout == "both" {
		for _, m := range sorted {
			wins = append(wins, window{name: m.Repo, cwd: m.WorktreePath})
		}
	}
	// Fallback: if the chosen layout produced nothing (e.g. "root" with no Root),
	// fall back to per-repo windows so the session is still useful.
	if len(wins) == 0 {
		for _, m := range sorted {
			wins = append(wins, window{name: m.Repo, cwd: m.WorktreePath})
		}
	}
	return wins
}

// EnsureSession creates the slice's tmux session (detached) if it does not
// already exist, with windows determined by opts (a root window, per-repo
// windows, or both — see SessionOpts). Idempotent: returns nil if the session
// already exists. The first window becomes the session's initial (attached) one.
func EnsureSession(slice string, members []model.SliceMember, opts SessionOpts) error {
	name := SessionName(slice)
	if SessionExists(slice) {
		return nil
	}

	wins := sessionWindows(members, opts)
	if len(wins) == 0 {
		// No members and no root: create a bare session.
		if out, err := exec.Command("tmux", "new-session", "-d", "-s", name).CombinedOutput(); err != nil {
			return fmt.Errorf("tmux new-session: %w: %s", err, out)
		}
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
		captured, _ := exec.Command("tmux", "capture-pane", "-p", "-t", name+":"+idx).Output()
		sb.WriteString("── " + wname + " ──\n")
		sb.Write(captured)
		if !strings.HasSuffix(string(captured), "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
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
