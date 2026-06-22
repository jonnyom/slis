// Package tui provides a Bubble Tea terminal UI for slis.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/discovery"
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
	ws      config.Workspace
	slices  []model.Slice
	focus   int
	err     error
	width   int
	height  int
	loading bool
}

// New returns an initial Model with loading=true.
func New(ws config.Workspace) Model {
	return Model{
		ws:      ws,
		loading: true,
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

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			if len(m.slices) > 0 && m.focus < len(m.slices)-1 {
				m.focus++
			}
		case "k", "up":
			if m.focus > 0 {
				m.focus--
			}
		case "r":
			m.loading = true
			return m, loadSlicesCmd(m.ws)
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
	return renderSliceList(m)
}

// Run creates and starts the Bubble Tea program using the alt-screen.
func Run(ws config.Workspace) error {
	p := tea.NewProgram(New(ws), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
