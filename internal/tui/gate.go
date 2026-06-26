package tui

import tea "github.com/charmbracelet/bubbletea"

// bgConcurrency caps how many slis-spawned background subprocesses (git / gt /
// gh / tmux / proc samples) may run at once. Bubble Tea runs every tea.Cmd in
// its own goroutine, so without a cap a large workspace would burst-spawn one
// subprocess chain per slice and saturate the machine.
const bgConcurrency = 4

var bgGate = make(chan struct{}, bgConcurrency)

// gatedCmd wraps a background tea.Cmd so its body only runs once a gate slot is
// free, bounding concurrent subprocess fan-out to bgConcurrency.
func gatedCmd(c tea.Cmd) tea.Cmd {
	if c == nil {
		return nil
	}
	return func() tea.Msg {
		bgGate <- struct{}{}
		defer func() { <-bgGate }()
		return c()
	}
}
