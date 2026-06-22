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

// EnsureSession creates the slice's tmux session (detached) with one window per
// member (window name = repo, cwd = member.WorktreePath) if it does not already
// exist. Idempotent: returns nil if the session already exists. Members are used
// in sorted-by-Repo order; the first becomes the session's initial window.
func EnsureSession(slice string, members []model.SliceMember) error {
	name := SessionName(slice)

	if SessionExists(slice) {
		return nil
	}

	// Sort members by Repo for deterministic window ordering.
	sorted := make([]model.SliceMember, len(members))
	copy(sorted, members)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Repo < sorted[j].Repo
	})

	if len(sorted) == 0 {
		// No members: create a bare session.
		if out, err := exec.Command("tmux", "new-session", "-d", "-s", name).CombinedOutput(); err != nil {
			return fmt.Errorf("tmux new-session: %w: %s", err, out)
		}
		return nil
	}

	// Create the session with the first member as the initial window.
	first := sorted[0]
	args := []string{"new-session", "-d", "-s", name, "-n", first.Repo, "-c", first.WorktreePath}
	if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session: %w: %s", err, out)
	}

	// Add remaining members as additional windows.
	for _, m := range sorted[1:] {
		args := []string{"new-window", "-t", name, "-n", m.Repo, "-c", m.WorktreePath}
		if out, err := exec.Command("tmux", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("tmux new-window %q: %w: %s", m.Repo, err, out)
		}
	}

	return nil
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
