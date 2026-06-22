// Package tui provides a Bubble Tea terminal UI for slis.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/swap"
)

// slicesLoadedMsg is sent by loadSlicesCmd when slice discovery completes.
type slicesLoadedMsg struct {
	slices []model.Slice
	err    error
}

// Model is the root Bubble Tea model for the slis TUI.
type Model struct {
	ws           config.Workspace
	slices       []model.Slice
	focus        int
	err          error
	width        int
	height       int
	loading      bool
	activeTab    Tab
	showHelp     bool
	stacks       map[string]map[string]gt.State // slice name → repo name → State
	stackLoading map[string]bool                // slice name → loading in progress
}

// New returns an initial Model with loading=true.
func New(ws config.Workspace) Model {
	return Model{
		ws:           ws,
		loading:      true,
		stacks:       make(map[string]map[string]gt.State),
		stackLoading: make(map[string]bool),
	}
}

// Init returns the initial command: load slices in the background.
func (m Model) Init() tea.Cmd {
	return loadSlicesCmd(m.ws)
}

// loadSlicesCmd returns a Cmd that discovers slices off the UI goroutine.
// All slow git I/O happens inside this closure.
func loadSlicesCmd(ws config.Workspace) tea.Cmd {
	return func() tea.Msg {
		sp := config.StatePaths()

		slices, err := discovery.Discover(ws)
		if err != nil {
			return slicesLoadedMsg{err: err}
		}

		ov, _ := discovery.LoadOverrides(sp.Overrides)
		slices = discovery.Apply(slices, ov)

		j, _ := swap.Load(sp.ActiveJournal)
		for i, s := range slices {
			if j != nil && j.Slice == s.Name {
				slices[i].Active = true
			}
		}

		return slicesLoadedMsg{slices: slices}
	}
}

// maybeLoadStack returns a loadStackCmd for the focused slice if its stack data
// is not already cached or being loaded. Returns nil if no load is needed.
func (m *Model) maybeLoadStack() tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	if _, cached := m.stacks[sl.Name]; cached {
		return nil
	}
	if m.stackLoading[sl.Name] {
		return nil
	}
	m.stackLoading[sl.Name] = true
	return loadStackCmd(sl)
}

// Update handles incoming messages and returns the updated model and next command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case slicesLoadedMsg:
		m.loading = false
		m.err = msg.err
		m.slices = msg.slices
		// Clamp focus to valid range after load.
		if len(m.slices) == 0 {
			m.focus = 0
		} else if m.focus >= len(m.slices) {
			m.focus = len(m.slices) - 1
		}
		// Kick off stack load for the currently focused slice if on Stack tab.
		if m.activeTab == TabStack {
			if cmd := m.maybeLoadStack(); cmd != nil {
				return m, cmd
			}
		}

	case stackLoadedMsg:
		m.stacks[msg.slice] = msg.stacks
		delete(m.stackLoading, msg.slice)

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if len(m.slices) > 0 && m.focus < len(m.slices)-1 {
				m.focus++
				if m.activeTab == TabStack {
					if cmd := m.maybeLoadStack(); cmd != nil {
						return m, cmd
					}
				}
			}
		case "k", "up":
			if m.focus > 0 {
				m.focus--
				if m.activeTab == TabStack {
					if cmd := m.maybeLoadStack(); cmd != nil {
						return m, cmd
					}
				}
			}
		case "r":
			m.loading = true
			return m, loadSlicesCmd(m.ws)
		case "tab", "l":
			m.activeTab = (m.activeTab + 1) % tabCount
			if m.activeTab == TabStack {
				if cmd := m.maybeLoadStack(); cmd != nil {
					return m, cmd
				}
			}
		case "shift+tab", "h":
			m.activeTab = (m.activeTab + tabCount - 1) % tabCount
		case "?":
			m.showHelp = !m.showHelp
		}
	}

	return m, nil
}

// View renders the current model state to a string.
func (m Model) View() string {
	if m.loading {
		return "Loading slices…\n"
	}
	if m.err != nil {
		return "Error: " + m.err.Error() + "\n"
	}
	if m.showHelp {
		return renderHelp()
	}

	left := renderSliceList(m)
	right := renderDetail(m)

	// If width is known, give left ~40 cols and right the remainder.
	// Fall back to side-by-side at natural widths when width is unknown.
	if m.width > 0 {
		leftWidth := 40
		if leftWidth >= m.width {
			leftWidth = m.width / 2
		}
		rightWidth := m.width - leftWidth - 1 // -1 for separator gap
		leftStyle := lipgloss.NewStyle().Width(leftWidth)
		rightStyle := lipgloss.NewStyle().Width(rightWidth)
		return lipgloss.JoinHorizontal(
			lipgloss.Top,
			leftStyle.Render(left),
			rightStyle.Render(right),
		)
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// Run creates and starts the Bubble Tea program using the alt-screen.
func Run(ws config.Workspace) error {
	p := tea.NewProgram(New(ws), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
