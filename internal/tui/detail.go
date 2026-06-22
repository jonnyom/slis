package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// Tab identifies which detail pane tab is active.
type Tab int

const (
	TabStack Tab = iota
	TabSummary
	TabChanges
	TabSessions
	TabProcesses
)

// tabCount is the total number of tabs.
const tabCount = 5

// String returns the human-readable name of the tab.
func (t Tab) String() string {
	switch t {
	case TabStack:
		return "Stack"
	case TabSummary:
		return "Summary"
	case TabChanges:
		return "Changes"
	case TabSessions:
		return "Sessions"
	case TabProcesses:
		return "Processes"
	default:
		return "Unknown"
	}
}

// stackLoadedMsg is sent when stack data for a slice has been loaded.
type stackLoadedMsg struct {
	slice  string
	stacks map[string]gt.State // repo name → State
}

// loadStackCmd returns a Cmd that loads gt stacks for all members of a slice off the UI goroutine.
func loadStackCmd(sl model.Slice) tea.Cmd {
	return func() tea.Msg {
		stacks := make(map[string]gt.State, len(sl.Members))
		for repo, member := range sl.Members {
			if member.WorktreePath == "" {
				stacks[repo] = gt.State{}
				continue
			}
			state, err := gt.ReadState(member.WorktreePath)
			if err != nil {
				state = gt.State{}
			}
			stacks[repo] = state
		}
		return stackLoadedMsg{slice: sl.Name, stacks: stacks}
	}
}

var (
	tabActiveStyle    = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("212"))
	tabNormalStyle    = lipgloss.NewStyle().Faint(true)
	detailPaneStyle   = lipgloss.NewStyle().Padding(0, 1)
	repoHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	trunkStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("28"))
	needsRestackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	branchStyle       = lipgloss.NewStyle()
)

// renderDetail renders the right-hand detail pane for the currently focused slice.
func renderDetail(m Model) string {
	var sb strings.Builder

	// Tab bar.
	tabParts := make([]string, tabCount)
	for i := 0; i < tabCount; i++ {
		tab := Tab(i)
		if tab == m.activeTab {
			tabParts[i] = tabActiveStyle.Render("[" + tab.String() + "]")
		} else {
			tabParts[i] = tabNormalStyle.Render(tab.String())
		}
	}
	sb.WriteString(strings.Join(tabParts, "  "))
	sb.WriteString("\n\n")

	// Body depends on the active tab.
	switch m.activeTab {
	case TabStack:
		sb.WriteString(renderStackTab(m))
	case TabChanges:
		sb.WriteString(m.viewport.View())
	case TabSessions:
		sb.WriteString(renderSessionsTab(m))
	case TabProcesses:
		sb.WriteString(renderProcessesTab(m))
	default:
		sb.WriteString(renderPlaceholder(m.activeTab))
	}

	return detailPaneStyle.Render(sb.String())
}

// renderStackTab renders the Stack tab body.
func renderStackTab(m Model) string {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return "no slice selected\n"
	}

	sl := m.slices[m.focus]
	repos := sl.Repos()

	if len(repos) == 0 {
		return "no repos in slice\n"
	}

	sliceStacks, loaded := m.stacks[sl.Name]
	if !loaded {
		if m.stackLoading[sl.Name] {
			return "loading…\n"
		}
		return "loading…\n"
	}

	var sb strings.Builder
	for _, repo := range repos {
		sb.WriteString(repoHeaderStyle.Render(repo))
		sb.WriteString("\n")

		state, ok := sliceStacks[repo]
		if !ok || len(state) == 0 {
			sb.WriteString("  (no stack data)\n")
			continue
		}

		ordered := state.Ordered()
		for _, branch := range ordered {
			indent := strings.Repeat("  ", branch.Depth+1)
			var line string
			if branch.Trunk {
				line = fmt.Sprintf("%s%s [trunk]", indent, branch.Name)
				line = trunkStyle.Render(line)
			} else if branch.NeedsRestack {
				line = fmt.Sprintf("%s%s (needs restack)", indent, branch.Name)
				line = needsRestackStyle.Render(line)
			} else {
				line = fmt.Sprintf("%s%s", indent, branch.Name)
				line = branchStyle.Render(line)
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderProcessesTab renders the Processes tab body for the focused slice.
func renderProcessesTab(m Model) string {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return "no slice selected\n"
	}
	sl := m.slices[m.focus]

	if m.procLoading[sl.Name] {
		return "loading…\n"
	}

	procs, ok := m.procs[sl.Name]
	if !ok {
		return "no tmux session (press [P] for global overlay)\n"
	}
	if len(procs) == 0 {
		return "no processes found\n"
	}

	// Use the right-pane width if known; else a reasonable fallback.
	tableWidth := m.width - 42 // 40 left + 1 sep + 1 padding
	if tableWidth < 40 {
		tableWidth = 80
	}

	return renderProcTable(procs, -1, tableWidth)
}

// renderPlaceholder renders a placeholder body for tabs not yet implemented.
func renderPlaceholder(t Tab) string {
	return fmt.Sprintf("%s: coming soon\n", t.String())
}
