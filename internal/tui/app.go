// Package tui provides a Bubble Tea terminal UI for slis.
package tui

import (
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fsnotify/fsnotify"

	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/summary"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/jonnyom/slis/internal/tmuxctl"
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
	diffs        map[string][]diff.RepoDiff     // slice name → repo diffs
	diffLoading  map[string]bool                // slice name → diff loading in progress
	viewport     viewport.Model                 // scrollable viewport for Changes tab
	// Session status badges.
	sessionStatus map[string]model.SessionStatus // slice name → status
	// Process overlay fields.
	procs           map[string][]proc.ProcInfo // slice name → sampled procs
	procLoading     map[string]bool            // slice name → proc load in progress
	showProcOverlay bool                       // true when [P] overlay is open
	overlaySel      int                        // selected row in overlay
	overlayProcs    []proc.ProcInfo            // flattened+sorted procs for overlay
	pendingKill     *killReq                   // non-nil when confirm prompt is shown
	// Summary tab fields.
	summaries      map[string]string // slice name → rendered commit summary text
	summaryLoading map[string]bool   // slice name → load in progress
	// fsnotify watcher for live event-file updates.
	watcher   *fsnotify.Watcher
	eventsDir string
}

// New returns an initial Model with loading=true.
// It creates an fsnotify watcher for the EventsDir so that Init can start
// listening immediately.
func New(ws config.Workspace) Model {
	sp := config.StatePaths()
	eventsDir := sp.EventsDir

	// Create the watcher best-effort — if it fails we just won't watch.
	var w *fsnotify.Watcher
	if watcher, err := fsnotify.NewWatcher(); err == nil {
		_ = os.MkdirAll(eventsDir, 0o755)
		_ = watcher.Add(eventsDir)
		w = watcher
	}

	return Model{
		ws:             ws,
		loading:        true,
		stacks:         make(map[string]map[string]gt.State),
		stackLoading:   make(map[string]bool),
		diffs:          make(map[string][]diff.RepoDiff),
		diffLoading:    make(map[string]bool),
		procs:          make(map[string][]proc.ProcInfo),
		procLoading:    make(map[string]bool),
		sessionStatus:  make(map[string]model.SessionStatus),
		summaries:      make(map[string]string),
		summaryLoading: make(map[string]bool),
		watcher:        w,
		eventsDir:      eventsDir,
	}
}

// Init returns the initial command: load slices in the background, and start
// the fsnotify watch loop for live event-file updates.
// Session statuses are loaded after the slicesLoadedMsg arrives (since we need
// the slice list to check tmux session existence per slice).
func (m Model) Init() tea.Cmd {
	watchCmd := waitForEventCmd(m.watcher)
	if watchCmd == nil {
		return loadSlicesCmd(m.ws)
	}
	return tea.Batch(loadSlicesCmd(m.ws), watchCmd)
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

// maybeLoadDiff returns a loadDiffCmd for the focused slice if its diff data is
// not already cached or being loaded. Returns nil if no load is needed.
func (m *Model) maybeLoadDiff() tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	if _, cached := m.diffs[sl.Name]; cached {
		return nil
	}
	if m.diffLoading[sl.Name] {
		return nil
	}
	m.diffLoading[sl.Name] = true
	return loadDiffCmd(sl, sliceBase(sl))
}

// maybeLoadProcs returns a loadProcsCmd for the focused slice if its proc data
// is not already cached or being loaded. Returns nil if no load is needed.
func (m *Model) maybeLoadProcs() tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	if _, cached := m.procs[sl.Name]; cached {
		return nil
	}
	if m.procLoading[sl.Name] {
		return nil
	}
	m.procLoading[sl.Name] = true
	return loadProcsCmd(sl.Name)
}

// summaryLoadedMsg is delivered when a commit or AI summary has been computed off-loop.
type summaryLoadedMsg struct {
	slice string
	text  string
}

// loadSummaryCmd returns a Cmd that computes the commit summary for sl off the
// UI goroutine and delivers a summaryLoadedMsg.
func loadSummaryCmd(sl model.Slice, base string) tea.Cmd {
	return func() tea.Msg {
		byRepo, _ := summary.CommitSummary(sl, base)
		md := summary.RenderCommitSummary(byRepo)
		return summaryLoadedMsg{slice: sl.Name, text: summary.RenderMarkdown(md)}
	}
}

// aiSummaryCmd builds a combined diff and calls claude -p off the UI goroutine,
// delivering a summaryLoadedMsg with the result (or an error note).
func aiSummaryCmd(sl model.Slice, diffs []diff.RepoDiff) tea.Cmd {
	return func() tea.Msg {
		var sb strings.Builder
		for _, rd := range diffs {
			sb.WriteString("# repo: " + rd.Repo + "\n")
			sb.WriteString(rd.Patch)
			if rd.Patch != "" && !strings.HasSuffix(rd.Patch, "\n") {
				sb.WriteString("\n")
			}
		}
		out, err := summary.AISummary(sb.String(), summary.DefaultClaudeRunner)
		if err != nil {
			return summaryLoadedMsg{slice: sl.Name, text: "AI summary unavailable: " + err.Error()}
		}
		return summaryLoadedMsg{slice: sl.Name, text: summary.RenderMarkdown(out)}
	}
}

// aiSummaryFromSliceCmd builds a combined diff from scratch (no cached diff required)
// and calls the AI summariser in a single off-loop command. This avoids the two-step
// diff-then-summary chain that previously caused [s] to hang on "loading…".
func aiSummaryFromSliceCmd(sl model.Slice) tea.Cmd {
	return func() tea.Msg {
		diffs, _ := diff.SliceDiff(sl, sliceBase(sl))
		combined := combinedPatch(diffs)
		out, err := summary.AISummary(combined, summary.DefaultClaudeRunner)
		if err != nil {
			return summaryLoadedMsg{slice: sl.Name, text: "AI summary unavailable: " + err.Error()}
		}
		return summaryLoadedMsg{slice: sl.Name, text: summary.RenderMarkdown(out)}
	}
}

// maybeLoadSummary returns a loadSummaryCmd for the focused slice if its summary
// is not already cached or being loaded. Returns nil if no load is needed.
func (m *Model) maybeLoadSummary() tea.Cmd {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return nil
	}
	sl := m.slices[m.focus]
	if _, cached := m.summaries[sl.Name]; cached {
		return nil
	}
	if m.summaryLoading[sl.Name] {
		return nil
	}
	m.summaryLoading[sl.Name] = true
	return loadSummaryCmd(sl, sliceBase(sl))
}

// batchLoadAllProcs returns a tea.Batch of loadProcsCmd for every slice that is
// not yet cached or being loaded. Used when the overlay opens.
func (m *Model) batchLoadAllProcs() tea.Cmd {
	var cmds []tea.Cmd
	for _, sl := range m.slices {
		if _, cached := m.procs[sl.Name]; cached {
			continue
		}
		if m.procLoading[sl.Name] {
			continue
		}
		m.procLoading[sl.Name] = true
		cmds = append(cmds, loadProcsCmd(sl.Name))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// Update handles incoming messages and returns the updated model and next command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Size the viewport to fill the right pane minus tabs+padding.
		vpHeight := msg.Height - 4 // reserve space for tab bar + padding
		if vpHeight < 1 {
			vpHeight = 1
		}
		rightWidth := msg.Width - 41 // 40 left + 1 separator
		if rightWidth < 1 {
			rightWidth = msg.Width
		}
		m.viewport = viewport.New(rightWidth, vpHeight)
		if m.activeTab == TabChanges {
			m.viewport.SetContent(diffContent(m))
		}

	case eventsChangedMsg:
		// An event-file changed — reload statuses and keep watching.
		return m, tea.Batch(
			loadSessionsCmd(m.slices, m.eventsDir),
			waitForEventCmd(m.watcher),
		)

	case sessionsLoadedMsg:
		// Detect transitions to WaitingInput before updating stored statuses.
		newly := NewlyWaiting(m.sessionStatus, msg.statuses)
		m.sessionStatus = msg.statuses
		if len(newly) > 0 {
			return m, notifyCmd(newly)
		}

	case sessionsRefreshMsg:
		sp := config.StatePaths()
		return m, loadSessionsCmd(m.slices, sp.EventsDir)

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
		// Kick off session status load alongside other tab loads.
		sp := config.StatePaths()
		sessCmd := loadSessionsCmd(m.slices, sp.EventsDir)
		// Kick off stack load for the currently focused slice if on Stack tab.
		if m.activeTab == TabStack {
			if cmd := m.maybeLoadStack(); cmd != nil {
				return m, tea.Batch(sessCmd, cmd)
			}
		}
		// Kick off diff load if on Changes tab.
		if m.activeTab == TabChanges {
			if cmd := m.maybeLoadDiff(); cmd != nil {
				return m, tea.Batch(sessCmd, cmd)
			}
		}
		return m, sessCmd

	case summaryLoadedMsg:
		m.summaries[msg.slice] = msg.text
		delete(m.summaryLoading, msg.slice)

	case stackLoadedMsg:
		m.stacks[msg.slice] = msg.stacks
		delete(m.stackLoading, msg.slice)

	case diffLoadedMsg:
		m.diffs[msg.slice] = msg.diffs
		delete(m.diffLoading, msg.slice)
		// Refresh viewport if the loaded slice is the focused one and we're on Changes.
		if m.activeTab == TabChanges && len(m.slices) > 0 &&
			m.focus >= 0 && m.focus < len(m.slices) &&
			m.slices[m.focus].Name == msg.slice {
			m.viewport.SetContent(diffContent(m))
		}

	case procsLoadedMsg:
		m.procs[msg.slice] = msg.procs
		delete(m.procLoading, msg.slice)
		// If the overlay is open, rebuild its flattened proc list.
		if m.showProcOverlay {
			m.overlayProcs = flattenProcs(m.procs)
			// Clamp selection.
			if m.overlaySel >= len(m.overlayProcs) {
				m.overlaySel = len(m.overlayProcs) - 1
			}
			if m.overlaySel < 0 {
				m.overlaySel = 0
			}
		}

	case procKilledMsg:
		// After a kill, refresh procs for all slices.
		return m, m.batchLoadAllProcs()

	case tea.KeyMsg:
		// ── Overlay key handling — takes full priority when overlay is open. ──
		if m.showProcOverlay {
			return m.updateOverlayKeys(msg)
		}

		// Viewport scroll keys — only when on Changes tab.
		if m.activeTab == TabChanges {
			switch msg.String() {
			case "ctrl+d", "pgdown":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			case "ctrl+u", "pgup":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			case "o":
				return m, openExternalCmd(m)
			case "y":
				return m, copyPatchCmd(m)
			}
		}

		// [s] on Summary tab triggers an AI summary (replaces cached commit summary).
		// This is a single self-contained command: compute the diff and call the AI
		// in one step, so there is no two-step diff-then-summary race.
		if m.activeTab == TabSummary && msg.String() == "s" {
			if len(m.slices) > 0 && m.focus >= 0 && m.focus < len(m.slices) {
				sl := m.slices[m.focus]
				m.summaryLoading[sl.Name] = true
				delete(m.summaries, sl.Name)
				return m, aiSummaryFromSliceCmd(sl)
			}
		}

		switch msg.String() {
		case "P":
			// Open the proc overlay.
			m.showProcOverlay = true
			m.overlaySel = 0
			m.overlayProcs = flattenProcs(m.procs)
			return m, m.batchLoadAllProcs()
		case "q", "ctrl+c":
			if m.watcher != nil {
				_ = m.watcher.Close()
			}
			return m, tea.Quit
		case "j", "down":
			if len(m.slices) > 0 && m.focus < len(m.slices)-1 {
				m.focus++
				if m.activeTab == TabStack {
					if cmd := m.maybeLoadStack(); cmd != nil {
						return m, cmd
					}
				}
				if m.activeTab == TabSummary {
					if cmd := m.maybeLoadSummary(); cmd != nil {
						return m, cmd
					}
				}
				if m.activeTab == TabChanges {
					m.viewport.SetContent(diffContent(m))
					if cmd := m.maybeLoadDiff(); cmd != nil {
						return m, cmd
					}
				}
				if m.activeTab == TabProcesses {
					if cmd := m.maybeLoadProcs(); cmd != nil {
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
				if m.activeTab == TabSummary {
					if cmd := m.maybeLoadSummary(); cmd != nil {
						return m, cmd
					}
				}
				if m.activeTab == TabChanges {
					m.viewport.SetContent(diffContent(m))
					if cmd := m.maybeLoadDiff(); cmd != nil {
						return m, cmd
					}
				}
				if m.activeTab == TabProcesses {
					if cmd := m.maybeLoadProcs(); cmd != nil {
						return m, cmd
					}
				}
			}
		case "a":
			// Attach to (or create) the tmux session for the focused slice.
			if len(m.slices) > 0 && m.focus >= 0 && m.focus < len(m.slices) {
				if !tmuxctl.Available() {
					// tmux not available — no-op; the Sessions tab explains this.
					return m, nil
				}
				sl := m.slices[m.focus]
				// EnsureSession synchronously (fast — tmux new-session -d returns immediately),
				// then hand the terminal to tmux via tea.ExecProcess.
				if err := tmuxctl.EnsureSession(sl.Name, membersSlice(sl)); err != nil {
					return m, nil
				}
				name, args := tmuxctl.AttachArgv(sl.Name, isInsideTmux())
				c := exec.Command(name, args...)
				return m, tea.ExecProcess(c, func(err error) tea.Msg {
					return sessionsRefreshMsg{}
				})
			}
		case "r":
			m.loading = true
			sp := config.StatePaths()
			return m, tea.Batch(loadSlicesCmd(m.ws), loadSessionsCmd(m.slices, sp.EventsDir))
		case "tab", "l":
			m.activeTab = (m.activeTab + 1) % tabCount
			if m.activeTab == TabStack {
				if cmd := m.maybeLoadStack(); cmd != nil {
					return m, cmd
				}
			}
			if m.activeTab == TabSummary {
				if cmd := m.maybeLoadSummary(); cmd != nil {
					return m, cmd
				}
			}
			if m.activeTab == TabChanges {
				m.viewport.SetContent(diffContent(m))
				if cmd := m.maybeLoadDiff(); cmd != nil {
					return m, cmd
				}
			}
			if m.activeTab == TabProcesses {
				if cmd := m.maybeLoadProcs(); cmd != nil {
					return m, cmd
				}
			}
		case "shift+tab", "h":
			m.activeTab = (m.activeTab + tabCount - 1) % tabCount
			if m.activeTab == TabSummary {
				if cmd := m.maybeLoadSummary(); cmd != nil {
					return m, cmd
				}
			}
			if m.activeTab == TabChanges {
				m.viewport.SetContent(diffContent(m))
				if cmd := m.maybeLoadDiff(); cmd != nil {
					return m, cmd
				}
			}
			if m.activeTab == TabProcesses {
				if cmd := m.maybeLoadProcs(); cmd != nil {
					return m, cmd
				}
			}
		case "?":
			m.showHelp = !m.showHelp
		}
	}

	return m, nil
}

// updateOverlayKeys handles key events while the proc overlay is open.
// It returns (model, cmd) so it can be returned directly from Update.
func (m Model) updateOverlayKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.overlayProcs)

	// If a kill is pending, only y/n/esc are accepted.
	if m.pendingKill != nil {
		switch msg.String() {
		case "y":
			req := *m.pendingKill
			m.pendingKill = nil
			// Run the kill, then reload procs for all slices.
			return m, tea.Batch(killCmd(req), m.batchLoadAllProcs())
		case "n", "esc":
			m.pendingKill = nil
		}
		return m, nil
	}

	switch msg.String() {
	case "j", "down":
		if n > 0 && m.overlaySel < n-1 {
			m.overlaySel++
		}
	case "k", "up":
		if m.overlaySel > 0 {
			m.overlaySel--
		}
	case "x":
		if n > 0 && m.overlaySel >= 0 && m.overlaySel < n {
			m.pendingKill = &killReq{pid: m.overlayProcs[m.overlaySel].PID, subtree: false}
		}
	case "X":
		if n > 0 && m.overlaySel >= 0 && m.overlaySel < n {
			m.pendingKill = &killReq{pid: m.overlayProcs[m.overlaySel].PID, subtree: true}
		}
	case "P", "esc":
		m.showProcOverlay = false
		m.pendingKill = nil
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
	if m.showProcOverlay {
		return renderProcOverlay(m)
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
