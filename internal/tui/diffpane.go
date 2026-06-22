package tui

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
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

// sliceBase returns the base ref for a slice, defaulting to "main" if unset.
func sliceBase(sl model.Slice) string {
	if sl.Base != "" {
		return sl.Base
	}
	return "main"
}

// diffContent builds a fully-rendered, colored string for all diffs in the
// focused slice. This is a pure function: it reads from the model but does not
// mutate it. The caller (Update) is responsible for calling
// m.viewport.SetContent with the result.
func diffContent(m Model) string {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return "no slice selected\n"
	}
	sl := m.slices[m.focus]

	if m.diffLoading[sl.Name] {
		return "loading diff…\n"
	}

	diffs, ok := m.diffs[sl.Name]
	if !ok {
		return "loading diff…\n"
	}
	if len(diffs) == 0 {
		return "no diffs\n"
	}

	var sb strings.Builder
	for _, rd := range diffs {
		if rd.Err != "" {
			sb.WriteString(repoHeaderStyle.Render(fmt.Sprintf("▸ %s  error: %s", rd.Repo, rd.Err)))
			sb.WriteString("\n\n")
			continue
		}
		header := fmt.Sprintf("▸ %s  %d files  +%d -%d",
			rd.Repo, len(rd.Files), rd.TotalAdded(), rd.TotalDeleted())
		sb.WriteString(repoHeaderStyle.Render(header))
		sb.WriteString("\n")
		for _, f := range rd.Files {
			var fileLine string
			if f.Added == -1 {
				fileLine = fmt.Sprintf("  %s  (binary)", f.Path)
			} else {
				fileLine = fmt.Sprintf("  %s  +%d -%d", f.Path, f.Added, f.Deleted)
			}
			sb.WriteString(fileLine)
			sb.WriteString("\n")
		}
		if rd.Patch != "" {
			sb.WriteString("\n")
			sb.WriteString(colorizePatch(rd.Patch))
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// colorizePatch applies chroma diff syntax coloring to a git patch string.
// On any chroma error, it falls back to simple lipgloss-based coloring.
func colorizePatch(patch string) string {
	lexer := lexers.Get("diff")
	if lexer == nil {
		return manualColorPatch(patch)
	}
	formatter := formatters.Get("terminal256")
	style := styles.Get("github")
	if style == nil {
		style = styles.Fallback
	}

	it, err := lexer.Tokenise(nil, patch)
	if err != nil {
		return manualColorPatch(patch)
	}

	var buf bytes.Buffer
	if err := formatter.Format(&buf, style, it); err != nil {
		return manualColorPatch(patch)
	}
	return buf.String()
}

var (
	addStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))            // green
	delStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))            // red
	hunkStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true) // cyan+bold
	plainStyle = lipgloss.NewStyle()
)

// manualColorPatch colorizes diff output using lipgloss when chroma is unavailable.
func manualColorPatch(patch string) string {
	var sb strings.Builder
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString(plainStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			sb.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			sb.WriteString(delStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(hunkStyle.Render(line))
		default:
			sb.WriteString(plainStyle.Render(line))
		}
		sb.WriteString("\n")
	}
	return sb.String()
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
