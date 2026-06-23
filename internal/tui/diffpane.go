package tui

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/model"
)

// diffLoadedMsg is sent when diff data has been loaded off-loop for a slice.
type diffLoadedMsg struct {
	slice string
	diffs []diff.RepoDiff
}

// loadDiffCmd returns a Cmd that calls diff.SliceDiff off the UI goroutine and
// delivers a diffLoadedMsg on completion. Per-repo errors are captured inside
// RepoDiff.Err; the top-level error is discarded to keep the model simple.
func loadDiffCmd(sl model.Slice, base string) tea.Cmd {
	return func() tea.Msg {
		diffs, _ := diff.SliceDiff(sl, base)
		return diffLoadedMsg{slice: sl.Name, diffs: diffs}
	}
}

// sliceBase returns the base ref override for a slice, or "" to let diff/summary
// auto-detect each repo's trunk independently (git.DetectBase). Base is only
// non-empty when explicitly overridden — a slice spanning repos with different
// trunks has no single slice-wide base.
func sliceBase(sl model.Slice) string {
	return sl.Base
}

var (
	diffAddStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))            // green (fg only)
	diffDelStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))           // red (fg only)
	diffHunkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true) // blue, bold
	diffHdrStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))           // dim file headers
	diffCtxStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))           // context lines
)

// isDiffHeader reports whether a line is a file/metadata header (not a +/- change).
func isDiffHeader(line string) bool {
	for _, p := range []string{"diff ", "index ", "+++", "---", "new file", "deleted file", "rename ", "similarity ", "old mode", "new mode", "Binary "} {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// colorizePatch colorizes a unified git patch using FOREGROUND colors only — no
// background fills, which read poorly on dark terminals. Headers are dimmed,
// hunks blue, additions green, deletions red, context neutral.
func colorizePatch(patch string) string {
	var sb strings.Builder
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case isDiffHeader(line):
			sb.WriteString(diffHdrStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(diffHunkStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			sb.WriteString(diffAddStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			sb.WriteString(diffDelStyle.Render(line))
		default:
			sb.WriteString(diffCtxStyle.Render(line))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

var splitSepStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

// renderSplitDiff renders a unified git patch as a side-by-side (split) view:
// deletions on the left, additions on the right, paired within each change
// block. Falls back to the unified renderer when the pane is too narrow.
func renderSplitDiff(patch string, width int) string {
	colW := (width - 3) / 2 // 3 cells for " │ "
	if colW < 12 {
		return colorizePatch(patch)
	}

	var sb strings.Builder
	var dels, adds []string

	flush := func() {
		n := max(len(dels), len(adds))
		for i := 0; i < n; i++ {
			l, r := "", ""
			ls, rs := diffCtxStyle, diffCtxStyle
			if i < len(dels) {
				l, ls = dels[i], diffDelStyle
			}
			if i < len(adds) {
				r, rs = adds[i], diffAddStyle
			}
			sb.WriteString(padCell(ls.Render(clip(l, colW)), colW))
			sb.WriteString(splitSepStyle.Render(" │ "))
			sb.WriteString(rs.Render(clip(r, colW)))
			sb.WriteString("\n")
		}
		dels, adds = nil, nil
	}

	for _, line := range strings.Split(patch, "\n") {
		switch {
		case isDiffHeader(line):
			flush()
			sb.WriteString(diffHdrStyle.Render(clip(line, width)) + "\n")
		case strings.HasPrefix(line, "@@"):
			flush()
			sb.WriteString(diffHunkStyle.Render(clip(line, width)) + "\n")
		case strings.HasPrefix(line, "-"):
			dels = append(dels, line)
		case strings.HasPrefix(line, "+"):
			adds = append(adds, line)
		default:
			flush()
			c := diffCtxStyle.Render(clip(line, colW))
			sb.WriteString(padCell(c, colW) + splitSepStyle.Render(" │ ") + c + "\n")
		}
	}
	flush()
	return sb.String()
}

// padCell right-pads an already-styled string to w display cells.
func padCell(colored string, w int) string {
	gap := w - lipgloss.Width(colored)
	if gap < 0 {
		gap = 0
	}
	return colored + strings.Repeat(" ", gap)
}

// combinedPatch concatenates all repo patches into a single string, each
// preceded by a "# repo: <name>" header line.
func combinedPatch(diffs []diff.RepoDiff) string {
	var sb strings.Builder
	for _, rd := range diffs {
		fmt.Fprintf(&sb, "# repo: %s\n", rd.Repo)
		sb.WriteString(rd.Patch)
		if !strings.HasSuffix(rd.Patch, "\n") && rd.Patch != "" {
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// externalEditorCmd returns the command name and args for the external editor.
// It checks $EDITOR first, then falls back to lazygit. Returns (name, args, ok).
func externalEditorCmd() (string, []string, bool) {
	if editor := os.Getenv("EDITOR"); editor != "" {
		if path, err := exec.LookPath(editor); err == nil {
			return path, nil, true
		}
		// Return the name even if LookPath failed — let the caller decide.
		return editor, nil, true
	}
	if path, err := exec.LookPath("lazygit"); err == nil {
		return path, nil, true
	}
	return "", nil, false
}

// clipboardCmd returns the clipboard tool name and args for the current OS.
// Returns (name, args, ok).
func clipboardCmd() (string, []string, bool) {
	switch runtime.GOOS {
	case "darwin":
		if path, err := exec.LookPath("pbcopy"); err == nil {
			return path, nil, true
		}
		return "pbcopy", nil, true // report name even if not found at lookup
	case "linux":
		if path, err := exec.LookPath("xclip"); err == nil {
			return path, []string{"-selection", "clipboard"}, true
		}
		if path, err := exec.LookPath("xsel"); err == nil {
			return path, []string{"--clipboard", "--input"}, true
		}
	}
	return "", nil, false
}

// openExternalCmd returns a tea.Cmd that opens the external editor (or lazygit)
// in the worktree of the first repo of the focused slice. It is a no-op if no
// editor can be found or no slice is focused.
func openExternalCmd(m Model) tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	repos := sl.Repos()
	if len(repos) == 0 {
		return nil
	}
	member := sl.Members[repos[0]]
	if member.WorktreePath == "" {
		return nil
	}
	name, args, ok := externalEditorCmd()
	if !ok {
		return nil
	}
	cmdArgs := append(args, member.WorktreePath)
	c := exec.Command(name, cmdArgs...) //nolint:gosec
	c.Dir = member.WorktreePath
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return nil
	})
}

// copyPatchCmd returns a tea.Cmd that writes the combined patch of the focused
// slice to the system clipboard. It is a no-op if no clipboard tool is found or
// no slice is focused.
func copyPatchCmd(m Model) tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	diffs, ok := m.diffs[sl.Name]
	if !ok || len(diffs) == 0 {
		return nil
	}
	patch := combinedPatch(diffs)
	name, args, ok := clipboardCmd()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		c := exec.Command(name, args...) //nolint:gosec
		c.Stdin = strings.NewReader(patch)
		_ = c.Run() // best-effort; ignore errors
		return nil
	}
}
