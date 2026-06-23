package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
)

// stackLoadedMsg is sent when stack data for a slice has been loaded.
type stackLoadedMsg struct {
	slice  string
	stacks map[string]gt.State // repo name → State
}

// loadStackCmd loads gt stacks for all members of a slice off the UI goroutine.
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

// Shared styles used by the cockpit's Stack panel and the browser cards.
var (
	repoHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	trunkStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	needsRestackStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
)
