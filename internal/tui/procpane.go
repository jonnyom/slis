package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/safeterm"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// killReq represents a pending kill request waiting for user confirmation.
type killReq struct {
	pid     int
	subtree bool
}

// procsLoadedMsg is delivered off-loop once process sampling for a slice completes.
type procsLoadedMsg struct {
	slice string
	procs []proc.ProcInfo
}

// procKilledMsg is delivered after a kill command finishes (success or failure).
type procKilledMsg struct{}

// loadProcsCmd returns a Cmd that samples processes for a slice off the UI goroutine.
// If tmux is unavailable or the session doesn't exist, an empty procsLoadedMsg is returned.
func loadProcsCmd(slice string) tea.Cmd {
	return func() tea.Msg {
		pids, err := tmuxctl.PanePIDs(slice)
		if err != nil {
			return procsLoadedMsg{slice: slice, procs: nil}
		}
		procs, _ := proc.SliceProcs(pids)
		return procsLoadedMsg{slice: slice, procs: procs}
	}
}

// killCmd returns a Cmd that sends a signal to the requested PID (or subtree).
func killCmd(req killReq) tea.Cmd {
	return func() tea.Msg {
		if req.subtree {
			_ = proc.KillSubtree(req.pid)
		} else {
			_ = proc.Kill(req.pid)
		}
		return procKilledMsg{}
	}
}

// sliceCPU returns the sum of CPU percentages across all procs in the list.
func sliceCPU(procs []proc.ProcInfo) float64 {
	var total float64
	for _, p := range procs {
		total += p.CPU
	}
	return total
}

// overCPUThreshold reports whether the total CPU for procs exceeds pct.
// Always returns false when pct <= 0.
func overCPUThreshold(procs []proc.ProcInfo, pct int) bool {
	if pct <= 0 {
		return false
	}
	return sliceCPU(procs) > float64(pct)
}

// flattenProcs merges all per-slice proc lists into a single slice sorted by
// CPU descending, capped at 50 entries.
func flattenProcs(bySlice map[string][]proc.ProcInfo) []proc.ProcInfo {
	const cap = 50
	var all []proc.ProcInfo
	for _, procs := range bySlice {
		all = append(all, procs...)
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].CPU > all[j].CPU
	})
	if len(all) > cap {
		all = all[:cap]
	}
	return all
}

var (
	procTableHeaderStyle = lipgloss.NewStyle().Bold(true).Faint(true)
	// Selected row: a subtle dark band with a soft magenta text accent — NOT a
	// full-width reversed block (the old Reverse(true) painted the whole row, and
	// any newlines, e.g. claude's --system-prompt, smeared pink across lines).
	procTableSelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Background(lipgloss.Color("236"))
	procTableNormalStyle = lipgloss.NewStyle()
	procOverlayBoxStyle  = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				Padding(0, 1).
				BorderForeground(lipgloss.Color("62"))
	procConfirmStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196"))
	cpuWarnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
)

// renderProcTable renders an aligned table of ProcInfo rows.
// sel < 0 means no row is highlighted (read-only mode).
// max is the available width for truncating CMD.
func renderProcTable(procs []proc.ProcInfo, sel int, max int) string {
	if len(procs) == 0 {
		return "(no processes)\n"
	}

	const (
		pidWidth  = 7
		cpuWidth  = 8
		memWidth  = 9
		sepWidth  = 3 // spaces between columns
		fixedCols = pidWidth + cpuWidth + memWidth + sepWidth*3
	)

	cmdWidth := max - fixedCols
	if cmdWidth < 10 {
		cmdWidth = 10
	}

	header := fmt.Sprintf("%-*s  %-*s  %-*s  %s",
		pidWidth, "PID",
		cpuWidth, "CPU%",
		memWidth, "MEM(MB)",
		"CMD",
	)

	var sb strings.Builder
	sb.WriteString(procTableHeaderStyle.Render(header))
	sb.WriteString("\n")

	for i, p := range procs {
		// Flatten the command to a single sanitised line: a process argv can
		// contain newlines/control chars (e.g. claude's multi-line --system-prompt),
		// which would otherwise wrap into a multi-line highlighted block.
		cmd := strings.Join(strings.Fields(safeterm.Strip(p.Cmd)), " ")
		if r := []rune(cmd); len(r) > cmdWidth {
			cmd = string(r[:cmdWidth-1]) + "…"
		}
		row := fmt.Sprintf("%-*d  %-*.1f  %-*.1f  %s",
			pidWidth, p.PID,
			cpuWidth, p.CPU,
			memWidth, p.MemMB,
			cmd,
		)
		if i == sel {
			sb.WriteString(procTableSelStyle.Render(row))
		} else {
			sb.WriteString(procTableNormalStyle.Render(row))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderProcOverlay renders the full-screen process overlay.
func renderProcOverlay(m Model) string {
	var sb strings.Builder

	title := "Processes (all slices) — [x] kill  [X] kill tree  [P/esc] close"
	sb.WriteString(title)
	sb.WriteString("\n\n")

	if len(m.overlayProcs) == 0 {
		sb.WriteString("(loading or no processes found)\n")
	} else {
		tableWidth := m.width - 4 // subtract border+padding
		if tableWidth < 40 {
			tableWidth = 40
		}
		sb.WriteString(renderProcTable(m.overlayProcs, m.overlaySel, tableWidth))
	}

	if m.pendingKill != nil {
		sb.WriteString("\n")
		action := "kill"
		if m.pendingKill.subtree {
			action = "kill tree"
		}
		confirm := fmt.Sprintf("%s PID %d? [y]es / [n]o", action, m.pendingKill.pid)
		sb.WriteString(procConfirmStyle.Render(confirm))
		sb.WriteString("\n")
	}

	return procOverlayBoxStyle.Render(sb.String())
}
