// Package tui provides a Bubble Tea terminal UI for slis.
//
// The UI has two levels:
//
//   - Browser (viewBrowser): a scrollable list of slices, each shown as a card
//     with its repos, stack health, PR, session badge, and a one-line summary of
//     what the slice is about. This is the landing screen.
//   - Cockpit (viewCockpit): opened with Enter on a slice. A lazygit-style layout
//     with stacked left panels (Repos & Stack, PRs, Session, Processes) whose
//     focus drives a large right pane (diff / PR detail / processes / summary).
//
// All slow work (git, gh, gt, proc, tmux) runs inside tea.Cmd closures, never in
// Update/View. View is pure.
package tui

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/jonnyom/slis/internal/cleanup"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/summary"
	"github.com/jonnyom/slis/internal/swap"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// viewMode selects which of the two top-level screens is showing.
type viewMode int

const (
	viewBrowser viewMode = iota // slice list (landing)
	viewCockpit                 // single-slice multi-panel detail
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
	focus   int // index of the current slice (selection in browser; subject of cockpit)
	err     error
	width   int
	height  int
	loading bool

	view      viewMode // browser or cockpit
	panel     panel    // focused left panel within the cockpit
	zoom      bool     // cockpit: right pane expanded full-width
	splitDiff bool     // cockpit: side-by-side diff instead of unified

	// Browser filter ("/" to type a substring; matches slice names).
	filter    string
	filtering bool

	// Pending slice-swap confirmation (activate/deactivate the focused slice).
	pendingSwap *swapReq

	// Pending slice-removal confirmation (clear a finished slice).
	pendingRemove *removeReq

	// Browser multi-select + group-naming for manual grouping.
	selected  map[string]bool
	naming    bool   // typing a group name for the selected slices
	groupName string // in-progress group name

	// tmux pane capture for the focused slice's Session panel (what Claude is doing).
	captures       map[string]string // slice name → captured pane text
	captureLoading map[string]bool

	// Per-panel selection within the cockpit.
	repoSel int // selected member in the Repos & Stack panel (drives right-pane diff)
	prSel   int // selected PR in the PRs panel
	procSel int // selected process in the Processes panel
	right   rightMode

	showHelp bool

	// Lazily-loaded per-slice data, keyed by slice name.
	stacks       map[string]map[string]gt.State // slice → repo → gt State
	stackLoading map[string]bool
	diffs        map[string][]diff.RepoDiff // slice → per-repo diffs
	diffLoading  map[string]bool
	cards        map[string]sliceCard // slice → browser summary card
	cardLoading  map[string]bool

	viewport viewport.Model // scrollable right pane (cockpit)

	// Session status badges.
	sessionStatus map[string]model.SessionStatus

	// Process data.
	procs           map[string][]proc.ProcInfo
	procLoading     map[string]bool
	showProcOverlay bool
	overlaySel      int
	overlayProcs    []proc.ProcInfo
	pendingKill     *killReq

	// Summary data.
	summaries      map[string]string
	summaryLoading map[string]bool

	// PR data.
	prs       map[string]map[string]*forge.PR
	prLoading map[string]bool

	// Comments overlay.
	showCommentsOverlay bool
	commentsSel         int

	// Transient status line (e.g. an attach error), shown in the footer.
	status string

	// fsnotify watcher for live event-file updates.
	watcher   *fsnotify.Watcher
	eventsDir string
}

// New returns an initial Model with loading=true. It creates an fsnotify watcher
// for the EventsDir so that Init can start listening immediately.
func New(ws config.Workspace) Model {
	sp := config.StatePaths()
	eventsDir := sp.EventsDir

	var w *fsnotify.Watcher
	if watcher, err := fsnotify.NewWatcher(); err == nil {
		_ = os.MkdirAll(eventsDir, 0o755)
		_ = watcher.Add(eventsDir)
		w = watcher
	}

	return Model{
		ws:             ws,
		loading:        true,
		view:           viewBrowser,
		stacks:         make(map[string]map[string]gt.State),
		stackLoading:   make(map[string]bool),
		diffs:          make(map[string][]diff.RepoDiff),
		diffLoading:    make(map[string]bool),
		cards:          make(map[string]sliceCard),
		cardLoading:    make(map[string]bool),
		procs:          make(map[string][]proc.ProcInfo),
		procLoading:    make(map[string]bool),
		sessionStatus:  make(map[string]model.SessionStatus),
		summaries:      make(map[string]string),
		summaryLoading: make(map[string]bool),
		prs:            make(map[string]map[string]*forge.PR),
		prLoading:      make(map[string]bool),
		captures:       make(map[string]string),
		captureLoading: make(map[string]bool),
		selected:       make(map[string]bool),
		watcher:        w,
		eventsDir:      eventsDir,
	}
}

// Init loads slices in the background and starts the fsnotify watch loop.
func (m Model) Init() tea.Cmd {
	watchCmd := waitForEventCmd(m.watcher)
	if watchCmd == nil {
		return loadSlicesCmd(m.ws)
	}
	return tea.Batch(loadSlicesCmd(m.ws), watchCmd)
}

// loadSlicesCmd discovers slices off the UI goroutine.
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

// currentSlice returns the focused slice and whether one exists.
func (m Model) currentSlice() (model.Slice, bool) {
	if len(m.slices) == 0 || m.focus < 0 || m.focus >= len(m.slices) {
		return model.Slice{}, false
	}
	return m.slices[m.focus], true
}

// ── Lazy loaders for the focused slice ──────────────────────────────────────

func (m *Model) maybeLoadStack() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok {
		return nil
	}
	if _, cached := m.stacks[sl.Name]; cached {
		return nil
	}
	if m.stackLoading[sl.Name] {
		return nil
	}
	m.stackLoading[sl.Name] = true
	return loadStackCmd(sl)
}

func (m *Model) maybeLoadDiff() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok {
		return nil
	}
	if _, cached := m.diffs[sl.Name]; cached {
		return nil
	}
	if m.diffLoading[sl.Name] {
		return nil
	}
	m.diffLoading[sl.Name] = true
	return loadDiffCmd(sl, sliceBase(sl))
}

func (m *Model) maybeLoadProcs() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok {
		return nil
	}
	if _, cached := m.procs[sl.Name]; cached {
		return nil
	}
	if m.procLoading[sl.Name] {
		return nil
	}
	m.procLoading[sl.Name] = true
	return loadProcsCmd(sl.Name)
}

func (m *Model) maybeLoadSummary() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok {
		return nil
	}
	if _, cached := m.summaries[sl.Name]; cached {
		return nil
	}
	if m.summaryLoading[sl.Name] {
		return nil
	}
	m.summaryLoading[sl.Name] = true
	return loadSummaryCmd(sl, sliceBase(sl))
}

// summaryLoadedMsg is delivered when a commit or AI summary has been computed.
type summaryLoadedMsg struct {
	slice string
	text  string
}

// loadSummaryCmd computes the commit summary for sl off the UI goroutine.
func loadSummaryCmd(sl model.Slice, base string) tea.Cmd {
	return func() tea.Msg {
		byRepo, _ := summary.CommitSummary(sl, base)
		md := summary.RenderCommitSummary(byRepo)
		return summaryLoadedMsg{slice: sl.Name, text: summary.RenderMarkdownFixed(md, 0)}
	}
}

// aiSummaryFromSliceCmd builds a combined diff and calls the AI summariser in a
// single off-loop command (avoids a two-step diff-then-summary race).
func aiSummaryFromSliceCmd(sl model.Slice) tea.Cmd {
	return func() tea.Msg {
		diffs, _ := diff.SliceDiff(sl, sliceBase(sl))
		combined := combinedPatch(diffs)
		out, err := summary.AISummary(combined, summary.DefaultClaudeRunner)
		if err != nil {
			return summaryLoadedMsg{slice: sl.Name, text: "AI summary unavailable: " + err.Error()}
		}
		return summaryLoadedMsg{slice: sl.Name, text: summary.RenderMarkdownFixed(out, 0)}
	}
}

// batchLoadAllProcs returns a Batch of loadProcsCmd for every uncached slice.
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

// swapReq is a pending activate/deactivate confirmation for a slice.
type swapReq struct {
	slice      string
	deactivate bool // true = restore primaries; false = swap the slice in
}

// swapFinishedMsg is delivered after a `slis activate/deactivate` subprocess exits.
type swapFinishedMsg struct{}

// slisSwapCmd shells out to the slis binary to (de)activate a slice, reusing the
// data-safety-critical CLI engine rather than duplicating it in the TUI (which
// must not import internal/cli). ExecProcess shows the command's output so
// activate progress and errors (e.g. a dirty primary) are visible on screen.
func slisSwapCmd(req swapReq, stash bool) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	args := []string{"deactivate"}
	if !req.deactivate {
		args = []string{"activate", req.slice}
		if stash {
			args = append(args, "--stash")
		}
	}
	c := exec.Command(self, args...) //nolint:gosec
	return tea.ExecProcess(c, func(error) tea.Msg { return swapFinishedMsg{} })
}

// removeReq is a pending "clear this finished slice" confirmation.
type removeReq struct{ slice string }

// removeDoneMsg carries the outcome of an in-process slice removal.
type removeDoneMsg struct {
	report cleanup.Report
	ok     bool // every member worktree was removed
}

// removeCmd clears a finished slice IN-PROCESS (no subprocess / alt-screen
// flash) so the result can be shown in the status line. On full success it also
// clears the slice's grouping override and status file.
func (m Model) removeCmd(sliceName string, force bool) tea.Cmd {
	var sl model.Slice
	found := false
	for _, s := range m.slices {
		if s.Name == sliceName {
			sl, found = s, true
			break
		}
	}
	if !found {
		return nil
	}
	ws := m.ws
	return func() tea.Msg {
		rep := cleanup.Remove(ws, sl, cleanup.Options{DeleteBranches: true, Force: force})
		ok := len(rep.Repos) > 0
		for _, r := range rep.Repos {
			if r.Err != "" {
				ok = false
			}
		}
		if ok {
			sp := config.StatePaths()
			if ov, err := discovery.LoadOverrides(sp.Overrides); err == nil {
				if _, present := ov[sliceName]; present {
					delete(ov, sliceName)
					_ = discovery.SaveOverrides(sp.Overrides, ov)
				}
			}
			_ = notify.RemoveStatus(sp.EventsDir, sliceName)
		}
		return removeDoneMsg{report: rep, ok: ok}
	}
}

// groupSelectedCmd writes a grouping override that merges the browser's selected
// slices into one named slice, then re-discovers (applying the new override).
func (m Model) groupSelectedCmd(name string) tea.Cmd {
	type rb struct{ repo, branch string }
	var entries []rb
	for _, s := range m.slices {
		if m.selected[s.Name] {
			for _, mem := range s.Members {
				entries = append(entries, rb{mem.Repo, mem.Branch})
			}
		}
	}
	ws := m.ws
	return func() tea.Msg {
		sp := config.StatePaths()
		ov, _ := discovery.LoadOverrides(sp.Overrides)
		if ov == nil {
			ov = discovery.Overrides{}
		}
		if ov[name] == nil {
			ov[name] = make(map[string]string)
		}
		for _, e := range entries {
			ov[name][e.repo] = e.branch
		}
		_ = discovery.SaveOverrides(sp.Overrides, ov)
		return loadSlicesCmd(ws)() // re-discover with the new override applied
	}
}

// ungroupCmd removes the grouping override for name (no-op if absent), then
// re-discovers.
func (m Model) ungroupCmd(name string) tea.Cmd {
	ws := m.ws
	return func() tea.Msg {
		sp := config.StatePaths()
		ov, _ := discovery.LoadOverrides(sp.Overrides)
		if _, ok := ov[name]; ok {
			delete(ov, name)
			_ = discovery.SaveOverrides(sp.Overrides, ov)
		}
		return loadSlicesCmd(ws)()
	}
}

// batchLoadAllPRs loads PR/CI data for every not-yet-loaded slice so CI status
// is visible across the whole browser without visiting each row.
func (m *Model) batchLoadAllPRs() tea.Cmd {
	var cmds []tea.Cmd
	for _, sl := range m.slices {
		if _, ok := m.prs[sl.Name]; ok {
			continue
		}
		if m.prLoading[sl.Name] {
			continue
		}
		m.prLoading[sl.Name] = true
		cmds = append(cmds, loadPRsCmd(sl))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// captureLoadedMsg carries a tmux pane capture for a slice.
type captureLoadedMsg struct {
	slice string
	text  string
}

// loadCaptureCmd captures the slice's tmux panes off the UI goroutine.
func loadCaptureCmd(slice string) tea.Cmd {
	return func() tea.Msg {
		text, _ := tmuxctl.CapturePane(slice)
		return captureLoadedMsg{slice: slice, text: text}
	}
}

// maybeLoadCapture (re)loads the focused slice's pane capture unless one is
// already in flight. Calling it again after completion refreshes the capture.
func (m *Model) maybeLoadCapture() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok || m.captureLoading[sl.Name] {
		return nil
	}
	m.captureLoading[sl.Name] = true
	return loadCaptureCmd(sl.Name)
}

// Update handles incoming messages and returns the updated model and next command.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		m.refreshRight()

	case eventsChangedMsg:
		return m, tea.Batch(
			loadSessionsCmd(m.slices, m.eventsDir),
			waitForEventCmd(m.watcher),
		)

	case sessionsLoadedMsg:
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
		if len(m.slices) == 0 {
			m.focus = 0
		} else if m.focus >= len(m.slices) {
			m.focus = len(m.slices) - 1
		}
		sp := config.StatePaths()
		return m, tea.Batch(loadSessionsCmd(m.slices, sp.EventsDir), m.batchLoadCards(), m.batchLoadAllPRs())

	case cardLoadedMsg:
		m.cards[msg.slice] = msg.card
		delete(m.cardLoading, msg.slice)

	case summaryLoadedMsg:
		m.summaries[msg.slice] = msg.text
		delete(m.summaryLoading, msg.slice)
		m.refreshRight()

	case prsLoadedMsg:
		m.prs[msg.slice] = msg.prs
		delete(m.prLoading, msg.slice)
		m.refreshRight()

	case stackLoadedMsg:
		m.stacks[msg.slice] = msg.stacks
		delete(m.stackLoading, msg.slice)
		m.refreshRight()

	case diffLoadedMsg:
		m.diffs[msg.slice] = msg.diffs
		delete(m.diffLoading, msg.slice)
		m.refreshRight()

	case procsLoadedMsg:
		m.procs[msg.slice] = msg.procs
		delete(m.procLoading, msg.slice)
		if m.showProcOverlay {
			m.overlayProcs = flattenProcs(m.procs)
			m.overlaySel = clamp(m.overlaySel, 0, len(m.overlayProcs)-1)
		}
		m.refreshRight()

	case procKilledMsg:
		return m, m.batchLoadAllProcs()

	case captureLoadedMsg:
		m.captures[msg.slice] = msg.text
		delete(m.captureLoading, msg.slice)
		m.refreshRight()

	case swapFinishedMsg:
		m.pendingSwap = nil
		sp := config.StatePaths()
		return m, tea.Batch(loadSlicesCmd(m.ws), loadSessionsCmd(m.slices, sp.EventsDir))

	case removeDoneMsg:
		if msg.ok {
			m.status = "cleared " + msg.report.Slice
			m.view = viewBrowser // the slice is gone; return to the hub
		} else {
			m.status = "clear failed — dirty worktree or unmerged branch; press d then f to force"
			for _, r := range msg.report.Repos {
				if r.Err != "" {
					m.status = fmt.Sprintf("clear failed (%s): dirty/locked — press d then f to force", r.Repo)
					break
				}
			}
		}
		sp := config.StatePaths()
		return m, tea.Batch(loadSlicesCmd(m.ws), loadSessionsCmd(m.slices, sp.EventsDir))

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleKey routes a key press to overlays, then to the active view.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Overlays take priority.
	if m.pendingSwap != nil {
		return m.updateSwapKeys(msg)
	}
	if m.pendingRemove != nil {
		return m.updateRemoveKeys(msg)
	}
	if m.showProcOverlay {
		return m.updateOverlayKeys(msg)
	}
	if m.showCommentsOverlay {
		return m.updateCommentsOverlayKeys(msg)
	}
	if m.showHelp {
		if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
			m.showHelp = false
		}
		return m, nil
	}

	// Global keys available in both views.
	switch msg.String() {
	case "q", "ctrl+c":
		if m.watcher != nil {
			_ = m.watcher.Close()
		}
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "P":
		m.showProcOverlay = true
		m.overlaySel = 0
		m.overlayProcs = flattenProcs(m.procs)
		return m, m.batchLoadAllProcs()
	case "r":
		// On the cockpit Session panel, [r] refreshes the live pane capture
		// rather than reloading the whole workspace.
		if m.view == viewCockpit && m.panel == panelSession {
			return m, m.maybeLoadCapture()
		}
		m.loading = true
		sp := config.StatePaths()
		return m, tea.Batch(loadSlicesCmd(m.ws), loadSessionsCmd(m.slices, sp.EventsDir))
	}

	if m.view == viewCockpit {
		return m.updateCockpitKeys(msg)
	}
	return m.updateBrowserKeys(msg)
}

// attachCmd ensures and attaches the focused slice's tmux session, surfacing any
// error in the status line instead of silently swallowing it.
func (m *Model) attachCmd() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok {
		return nil
	}
	if !tmuxctl.Available() {
		m.status = "tmux not found on PATH"
		return nil
	}
	if err := tmuxctl.EnsureSession(sl.Name, membersSlice(sl)); err != nil {
		m.status = "session error: " + err.Error()
		return nil
	}
	m.status = ""
	name, args := tmuxctl.AttachArgv(sl.Name, isInsideTmux())
	c := exec.Command(name, args...)
	return tea.ExecProcess(c, func(error) tea.Msg {
		return sessionsRefreshMsg{}
	})
}

// updateSwapKeys handles the activate/deactivate confirmation prompt.
func (m Model) updateSwapKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	req := *m.pendingSwap
	switch msg.String() {
	case "y":
		m.pendingSwap = nil
		return m, slisSwapCmd(req, false)
	case "s":
		if !req.deactivate { // stash dirty primaries, then activate
			m.pendingSwap = nil
			return m, slisSwapCmd(req, true)
		}
	case "n", "esc":
		m.pendingSwap = nil
	}
	return m, nil
}

// requestSwap sets up the activate/deactivate confirmation for the focused slice.
func (m *Model) requestSwap() {
	if sl, ok := m.currentSlice(); ok {
		m.pendingSwap = &swapReq{slice: sl.Name, deactivate: sl.Active}
	}
}

// requestRemove sets up the "clear finished slice" confirmation, refusing up
// front if the slice is live (the CLI enforces this too).
func (m *Model) requestRemove() {
	sl, ok := m.currentSlice()
	if !ok {
		return
	}
	if sl.Active {
		m.status = "slice is live — deactivate (w) before clearing"
		return
	}
	m.pendingRemove = &removeReq{slice: sl.Name}
}

// updateRemoveKeys handles the clear-slice confirmation prompt.
func (m Model) updateRemoveKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	slice := m.pendingRemove.slice
	switch msg.String() {
	case "y":
		m.pendingRemove = nil
		return m, m.removeCmd(slice, false)
	case "f":
		m.pendingRemove = nil
		return m, m.removeCmd(slice, true)
	case "n", "esc":
		m.pendingRemove = nil
	}
	return m, nil
}

// updateOverlayKeys handles key events while the proc overlay is open.
func (m Model) updateOverlayKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.overlayProcs)

	if m.pendingKill != nil {
		switch msg.String() {
		case "y":
			req := *m.pendingKill
			m.pendingKill = nil
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

// updateCommentsOverlayKeys handles key events while the comments overlay is open.
func (m Model) updateCommentsOverlayKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	lines := flattenComments(m)
	n := len(lines)

	switch msg.String() {
	case "j", "down":
		if n > 0 && m.commentsSel < n-1 {
			m.commentsSel++
		}
	case "k", "up":
		if m.commentsSel > 0 {
			m.commentsSel--
		}
	case "c", "esc":
		m.showCommentsOverlay = false
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
	if m.width > 0 && (m.width < 60 || m.height < 16) {
		return fmt.Sprintf("Terminal too small (%dx%d).\nResize to at least 60x16.\n", m.width, m.height)
	}
	if m.showHelp {
		return renderHelp(m.view)
	}
	if m.pendingSwap != nil {
		return renderSwapOverlay(m)
	}
	if m.pendingRemove != nil {
		return renderRemoveOverlay(m)
	}
	if m.showProcOverlay {
		return renderProcOverlay(m)
	}
	if m.showCommentsOverlay {
		return renderCommentsOverlay(m)
	}
	if m.view == viewCockpit {
		return renderCockpit(m)
	}
	return renderBrowser(m)
}

// clamp constrains v to [lo, hi]; if hi < lo it returns lo.
func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Run creates and starts the Bubble Tea program using the alt-screen.
func Run(ws config.Workspace) error {
	p := tea.NewProgram(New(ws), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
