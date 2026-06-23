package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/git"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// diffLoadedMsg is sent when diff data has been loaded off-loop for a slice.
type diffLoadedMsg struct {
	slice string
	diffs []diff.RepoDiff
}

// gtParent returns the Graphite parent branch of branch in dir's repo, or "".
func gtParent(dir, branch string) string {
	st, err := gt.ReadState(dir)
	if err != nil {
		return ""
	}
	bs, ok := st[branch]
	if !ok || len(bs.Parents) == 0 {
		return ""
	}
	return strings.TrimSpace(bs.Parents[0].Ref)
}

// loadDiffCmd computes each member's diff off the UI goroutine. By default it
// diffs against the branch's Graphite PARENT (so a stacked branch shows only its
// own changes, not the whole downstack), falling back to the repo's trunk when
// the branch isn't stacked. vsTrunk forces diffing against the trunk (the
// cumulative feature change). An explicit sl.Base override wins over both.
func loadDiffCmd(sl model.Slice, vsTrunk bool) tea.Cmd {
	override := sl.Base
	return func() tea.Msg {
		bases := make(map[string]string, len(sl.Members))
		for repo, mem := range sl.Members {
			switch {
			case override != "":
				bases[repo] = override
			case !vsTrunk:
				if p := gtParent(mem.WorktreePath, mem.Branch); p != "" {
					bases[repo] = p
				} else {
					bases[repo] = git.DetectBase(mem.WorktreePath)
				}
			default:
				bases[repo] = git.DetectBase(mem.WorktreePath)
			}
		}
		diffs, _ := diff.SliceDiffBases(sl, bases)
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

// guiEditors open their own window and return immediately — they must NOT go
// through tea.ExecProcess (which suspends the TUI and produces a flash).
var guiEditors = map[string]bool{
	"code": true, "code-insiders": true, "codium": true, "cursor": true,
	"windsurf": true, "subl": true, "sublime_text": true, "zed": true,
	"atom": true, "idea": true, "webstorm": true, "goland": true, "pycharm": true,
}

// resolveOpener picks how to open a worktree: $VISUAL/$EDITOR first (classified
// gui vs terminal), then lazygit (terminal UI), then the OS opener (gui). The
// gui flag tells the caller to spawn detached rather than via ExecProcess.
func resolveOpener() (name string, args []string, gui, ok bool) {
	for _, env := range []string{os.Getenv("VISUAL"), os.Getenv("EDITOR")} {
		if strings.TrimSpace(env) == "" {
			continue
		}
		fields := strings.Fields(env) // $EDITOR may carry args, e.g. "code -w"
		bin := fields[0]
		isGUI := guiEditors[filepath.Base(bin)]
		if path, err := exec.LookPath(bin); err == nil {
			return path, fields[1:], isGUI, true
		}
		return bin, fields[1:], isGUI, true
	}
	if path, err := exec.LookPath("lazygit"); err == nil {
		return path, nil, false, true
	}
	switch runtime.GOOS {
	case "darwin":
		if path, err := exec.LookPath("open"); err == nil {
			return path, nil, true, true
		}
	case "linux":
		if path, err := exec.LookPath("xdg-open"); err == nil {
			return path, nil, true, true
		}
	}
	return "", nil, false, false
}

// osOpener returns the OS "open in default app" command (open / xdg-open) —
// used to open URLs in the browser (unlike resolveOpener, which prefers $EDITOR).
func osOpener() (string, bool) {
	switch runtime.GOOS {
	case "darwin":
		if p, err := exec.LookPath("open"); err == nil {
			return p, true
		}
	case "linux":
		if p, err := exec.LookPath("xdg-open"); err == nil {
			return p, true
		}
	}
	return "", false
}

// openURLCmd opens url in the default browser, detached (no TUI flash).
func openURLCmd(url string) tea.Cmd {
	opener, ok := osOpener()
	if !ok || strings.TrimSpace(url) == "" {
		return nil
	}
	return func() tea.Msg {
		_ = exec.Command(opener, url).Start() //nolint:gosec
		return nil
	}
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

// openExternalCmd opens the selected repo's worktree in the user's editor.
// GUI editors (and the OS opener) are spawned detached so the TUI is never
// suspended; terminal editors (vim, lazygit, …) take over via ExecProcess.
func openExternalCmd(m Model) tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok {
		return nil
	}
	repos := sl.Repos()
	if len(repos) == 0 {
		return nil
	}
	member := sl.Members[repos[clamp(m.repoSel, 0, len(repos)-1)]]
	if member.WorktreePath == "" {
		return nil
	}
	name, args, gui, ok := resolveOpener()
	if !ok {
		return nil
	}
	cmdArgs := append(args, member.WorktreePath)
	c := exec.Command(name, cmdArgs...) //nolint:gosec
	c.Dir = member.WorktreePath
	if gui {
		// Detached: open the window, don't suspend the TUI (no flash).
		return func() tea.Msg {
			_ = c.Start()
			return nil
		}
	}
	return tea.ExecProcess(c, func(error) tea.Msg { return nil })
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
