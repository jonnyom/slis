package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/editor"
	"github.com/jonnyom/slis/internal/model"
)

// editorReq is a pending "open in editor" request: a slice, plus an optional
// repo (empty repo = open the whole slice in one window).
type editorReq struct {
	slice model.Slice
	repo  string
}

// editorOpenedMsg carries the result of launching the editor.
type editorOpenedMsg struct{ err error }

// openInEditor resolves which editor to use and either opens immediately or,
// when no editor is configured and several are detected, raises the picker
// overlay. It mutates m (status / overlay state) and returns the Cmd to run.
func (m *Model) openInEditor(req editorReq) tea.Cmd {
	if configured := m.ws.Sessions.Editor; configured != "" {
		ed, err := editor.Resolve(configured)
		if err != nil {
			m.status = err.Error()
			return nil
		}
		return openEditorCmd(ed, req)
	}
	avail := editor.Available()
	switch len(avail) {
	case 0:
		m.status = "no editor found — install cursor/code/zed or set sessions.editor"
		return nil
	case 1:
		return openEditorCmd(avail[0], req)
	default:
		m.showEditorPicker = true
		m.editorOptions = avail
		m.editorSel = 0
		m.pendingEditor = &req
		return nil
	}
}

// worktreePaths returns a slice's member worktree paths in sorted repo order.
func worktreePaths(sl model.Slice) []string {
	repos := sl.Repos()
	paths := make([]string, 0, len(repos))
	for _, r := range repos {
		if p := sl.Members[r].WorktreePath; p != "" {
			paths = append(paths, p)
		}
	}
	return paths
}

// openEditorCmd launches the editor for the request off the UI loop. A whole
// slice (req.repo == "") opens every worktree in one window; otherwise just the
// one repo's worktree.
func openEditorCmd(ed editor.Editor, req editorReq) tea.Cmd {
	wsDir := config.StatePaths().WorkspacesDir
	return func() tea.Msg {
		if req.repo != "" {
			return editorOpenedMsg{err: editor.OpenDir(ed, req.slice.Members[req.repo].WorktreePath)}
		}
		return editorOpenedMsg{err: editor.OpenSlice(ed, req.slice.Name, worktreePaths(req.slice), wsDir)}
	}
}

// updateEditorPickerKeys handles navigation/selection in the editor picker. On
// selection it persists the choice to workspace.yaml (so it never asks again)
// and runs the pending open.
func (m Model) updateEditorPickerKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.showEditorPicker = false
		m.pendingEditor = nil
	case "j", "down":
		m.editorSel = clamp(m.editorSel+1, 0, len(m.editorOptions)-1)
	case "k", "up":
		m.editorSel = clamp(m.editorSel-1, 0, len(m.editorOptions)-1)
	case "enter":
		if m.editorSel < 0 || m.editorSel >= len(m.editorOptions) {
			m.showEditorPicker = false
			return m, nil
		}
		ed := m.editorOptions[m.editorSel]
		req := m.pendingEditor
		m.showEditorPicker = false
		m.pendingEditor = nil
		m.ws.Sessions.Editor = ed.Bin
		if err := config.SaveWorkspace(config.WorkspacePath(), m.ws); err != nil {
			m.status = "could not save editor choice: " + err.Error()
		}
		if req == nil {
			return m, nil
		}
		return m, openEditorCmd(ed, *req)
	}
	return m, nil
}

var (
	editorPickerBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 3)
	editorPickerSel = lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Bold(true)
)

// editorPickerView renders the editor-picker overlay.
func (m Model) editorPickerView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Open in which editor?") + "\n\n")
	for i, e := range m.editorOptions {
		cursor := "  "
		line := e.Name + "  (" + e.Bin + ")"
		if i == m.editorSel {
			cursor = "▸ "
			line = editorPickerSel.Render(line)
		}
		b.WriteString(cursor + line + "\n")
	}
	b.WriteString("\n" + cockpitDimStyle.Render("↑/↓ select · enter open (remembered) · esc cancel"))
	return editorPickerBox.Render(b.String())
}
