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
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"

	"github.com/jonnyom/slis/internal/cleanup"
	"github.com/jonnyom/slis/internal/commentcache"
	"github.com/jonnyom/slis/internal/config"
	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/discovery"
	"github.com/jonnyom/slis/internal/editor"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/notify"
	"github.com/jonnyom/slis/internal/proc"
	"github.com/jonnyom/slis/internal/radar"
	"github.com/jonnyom/slis/internal/restack"
	"github.com/jonnyom/slis/internal/safeterm"
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

	view        viewMode // browser or cockpit
	panel       panel    // focused left panel within the cockpit
	zoom        bool     // cockpit: right pane expanded full-width
	splitDiff   bool     // cockpit: side-by-side diff instead of unified
	diffVsTrunk bool     // cockpit: diff vs trunk (whole downstack) instead of vs the stack parent

	// Failed-CI log shown in the right pane (rightCILog mode).
	ciLog        string
	ciLogLoading bool

	// Browser filter ("/" to type a substring; matches slice names).
	filter    string
	filtering bool

	// Hub (dashboard) state: which state-filter is active, and whether the
	// States rail or the Slices list currently takes j/k.
	filterIdx int
	hubFocus  int // 0 = slices list, 1 = states rail

	// previewScroll is the line offset of the hub preview pane (reset to 0 when
	// the focused slice changes; advanced by the scroll keys).
	previewScroll int

	// Pending slice-swap confirmation (activate/deactivate the focused slice).
	pendingSwap *swapReq

	// Pending slice-removal confirmation (clear a finished slice).
	pendingRemove *removeReq

	// Pending stack-action confirmation (restack / sync the focused slice).
	pendingStack *stackReq

	// Browser multi-select + group-naming for manual grouping.
	selected  map[string]bool
	naming    bool // typing a group name for the selected slices
	groupName string

	// New-slice creation input ("c" in the hub).
	creating   bool
	createName string // in-progress new-slice name

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

	// Cross-slice conflict radar: files changed by more than one slice. Rebuilt
	// from the loaded cards' retained stats as cards arrive (so View stays pure).
	conflicts           *radar.Index
	showConflictOverlay bool
	conflictScroll      int

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

	// Persisted PR comments (survive slice removal; backs the PR-pane fallback).
	commentCache commentcache.Store

	// Editor picker overlay: shown when no editor is configured and several are
	// detected. The pending request is run once the user picks (and the choice is
	// persisted to workspace.yaml).
	showEditorPicker bool
	editorOptions    []editor.Editor
	editorSel        int
	pendingEditor    *editorReq

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

	prefs := config.LoadPrefs(sp.Prefs) // remembered diff-view toggles

	return Model{
		ws:             ws,
		loading:        true,
		view:           viewBrowser,
		splitDiff:      prefs.SplitDiff,
		diffVsTrunk:    prefs.DiffVsTrunk,
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
		commentCache:   commentcache.Store{},
		captures:       make(map[string]string),
		captureLoading: make(map[string]bool),
		selected:       make(map[string]bool),
		watcher:        w,
		eventsDir:      eventsDir,
	}
}

// captureTickMsg drives periodic refresh of the focused slice's tmux capture so
// the session view (hub preview / cockpit Session panel) feels live.
type captureTickMsg struct{}

// captureTickCmd schedules the next capture refresh.
func captureTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return captureTickMsg{} })
}

// Init loads slices in the background, starts the fsnotify watch loop, and the
// capture-refresh ticker.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{loadSlicesCmd(m.ws), captureTickCmd(), prsTickCmd(), loadCommentCacheCmd()}
	if watchCmd := waitForEventCmd(m.watcher); watchCmd != nil {
		cmds = append(cmds, watchCmd)
	}
	return tea.Batch(cmds...)
}

// shouldShowCapture reports whether the current view shows the focused slice's
// tmux capture (so the ticker should refresh it).
func (m Model) shouldShowCapture() bool {
	sl, ok := m.currentSlice()
	if !ok || m.sessionStatus[sl.Name] == model.SessNone {
		return false
	}
	if m.view == viewCockpit {
		return m.panel == panelSession
	}
	return m.view == viewBrowser // hub preview shows recent session output
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
	return loadDiffCmd(sl, m.diffVsTrunk)
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

// ── Force loaders ───────────────────────────────────────────────────────────
// The force variants re-fetch the focused slice ignoring the cache (so entering
// or re-focusing a slice, and the periodic tick, pull fresh data). They keep the
// in-flight guard so a key-repeat can't issue duplicate loads; cached content
// keeps rendering until the new result lands (no "loading…" flicker).

func (m *Model) forceLoadStack() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok || m.stackLoading[sl.Name] {
		return nil
	}
	m.stackLoading[sl.Name] = true
	return loadStackCmd(sl)
}

func (m *Model) forceLoadDiff() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok || m.diffLoading[sl.Name] {
		return nil
	}
	m.diffLoading[sl.Name] = true
	return loadDiffCmd(sl, m.diffVsTrunk)
}

func (m *Model) forceLoadProcs() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok || m.procLoading[sl.Name] {
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

// adoptFinishedMsg is delivered after the `slis adopt` subprocess exits. err is
// the subprocess exit error (non-nil = adopt didn't complete) so the TUI can
// surface an actionable status instead of silently redrawing.
type adoptFinishedMsg struct{ err error }

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

// removeReq is a pending "clear finished slice(s)" confirmation (one or many).
type removeReq struct{ slices []string }

// removeDoneMsg carries the aggregate outcome of an in-process clear.
type removeDoneMsg struct {
	cleared int
	failed  string // first "<slice>/<repo>" that failed, "" if all clean
}

// actionTargets returns the slice names a fleet action applies to: the
// multi-selection if any, else the focused slice.
func (m Model) actionTargets() []string {
	if len(m.selected) == 0 {
		if sl, ok := m.currentSlice(); ok {
			return []string{sl.Name}
		}
		return nil
	}
	var names []string
	for _, s := range m.slices { // m.slices order is stable
		if m.selected[s.Name] {
			names = append(names, s.Name)
		}
	}
	return names
}

// isActive reports whether the named slice is currently swapped in (live).
func (m Model) isActive(name string) bool {
	for _, s := range m.slices {
		if s.Name == name {
			return s.Active
		}
	}
	return false
}

// removeCmd clears one or more finished slices IN-PROCESS (no subprocess /
// alt-screen flash). On full success per slice it also clears that slice's
// grouping override and status file; results are aggregated for the status line.
func (m Model) removeCmd(slices []string, force bool) tea.Cmd {
	byName := make(map[string]model.Slice, len(m.slices))
	for _, s := range m.slices {
		byName[s.Name] = s
	}
	var targets []model.Slice
	for _, n := range slices {
		if sl, ok := byName[n]; ok {
			targets = append(targets, sl)
		}
	}
	ws := m.ws
	return func() tea.Msg {
		sp := config.StatePaths()
		cleared, failed := 0, ""
		for _, sl := range targets {
			rep, err := cleanup.Remove(ws, sl, cleanup.Options{DeleteBranches: true, Force: force, ActiveJournal: sp.ActiveJournal})
			if err != nil {
				// Slice went live between the last refresh and now — the engine's
				// journal re-check refused it. Don't clear it.
				if failed == "" {
					failed = sl.Name + " (live)"
				}
				continue
			}
			ok := len(rep.Repos) > 0
			for _, r := range rep.Repos {
				if r.Err != "" {
					ok = false
					if failed == "" {
						failed = sl.Name + "/" + r.Repo
					}
				}
			}
			if ok {
				cleared++
				if ov, err := discovery.LoadOverrides(sp.Overrides); err == nil {
					if _, present := ov[sl.Name]; present {
						delete(ov, sl.Name)
						_ = discovery.SaveOverrides(sp.Overrides, ov)
					}
				}
				_ = notify.RemoveStatus(sp.EventsDir, sl.Name)
			}
		}
		return removeDoneMsg{cleared: cleared, failed: failed}
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

// stackReq is a pending restack/sync confirmation (one or many slices).
type stackReq struct{ slices []string }

// stackDoneMsg carries the aggregate outcome of an in-process restack.
type stackDoneMsg struct {
	restacked int
	conflict  string // first "<slice>/<repo>" with a conflict, "" if none
	dirty     string // first "<slice>/<repo>" skipped as dirty, "" if none
}

// restackCmd restacks one or more slices' stacks IN-PROCESS (refusing dirty
// worktrees) and aggregates the result for the status line.
func (m Model) restackCmd(slices []string) tea.Cmd {
	byName := make(map[string]model.Slice, len(m.slices))
	for _, s := range m.slices {
		byName[s.Name] = s
	}
	var targets []model.Slice
	for _, n := range slices {
		if sl, ok := byName[n]; ok {
			targets = append(targets, sl)
		}
	}
	return func() tea.Msg {
		agg := stackDoneMsg{}
		for _, sl := range targets {
			for _, r := range restack.Run(sl, gt.Restack).Repos {
				switch {
				case r.Conflict:
					if agg.conflict == "" {
						agg.conflict = sl.Name + "/" + r.Repo
					}
				case r.SkippedDirty:
					if agg.dirty == "" {
						agg.dirty = sl.Name + "/" + r.Repo
					}
				case r.Restacked:
					agg.restacked++
				}
			}
		}
		return agg
	}
}

// slisSubmitCmd hands the terminal to `slis submit <slice>` (interactive
// `gt submit` per repo) so the user can edit PR metadata and see the PR URLs.
func slisSubmitCmd(slice string) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	c := exec.Command(self, "submit", slice) //nolint:gosec
	return tea.ExecProcess(c, func(error) tea.Msg { return swapFinishedMsg{} })
}

// slisMergeCmd hands the terminal to `slis merge <slice>` (`gt merge` per repo)
// — Graphite merges the stack server-side, so slis just triggers it and reloads.
func slisMergeCmd(slice string) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	c := exec.Command(self, "merge", slice) //nolint:gosec
	return tea.ExecProcess(c, func(error) tea.Msg { return swapFinishedMsg{} })
}

// slisSyncCmd hands the terminal to `slis sync <slice>` (interactive `gt sync`
// per repo) so the user can answer its delete/overwrite-trunk prompts.
func slisSyncCmd(slice string) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	c := exec.Command(self, "sync", slice) //nolint:gosec
	return tea.ExecProcess(c, func(error) tea.Msg { return swapFinishedMsg{} })
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
		// Keep the pane's colours (SGR) but strip cursor/OSC/other escapes so a
		// hostile program in the session can't manipulate the slis terminal.
		return captureLoadedMsg{slice: slice, text: safeterm.StripNonSGR(text)}
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
		// Notify when Claude yields control: either blocked on input or finished
		// a turn. Both are "your move" moments for an autonomous session.
		var alerts []sessionAlert
		for _, s := range NewlyInStatus(m.sessionStatus, msg.statuses, model.SessWaitingInput) {
			alerts = append(alerts, sessionAlert{slice: s, status: model.SessWaitingInput})
		}
		for _, s := range NewlyInStatus(m.sessionStatus, msg.statuses, model.SessDone) {
			alerts = append(alerts, sessionAlert{slice: s, status: model.SessDone})
		}
		m.sessionStatus = msg.statuses
		if len(alerts) > 0 {
			return m, notifyCmd(alerts)
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
		return m, tea.Batch(loadSessionsCmd(m.slices, sp.EventsDir), m.batchLoadCards(), m.batchLoadAllPRs(), m.loadPreview())

	case cardLoadedMsg:
		m.cards[msg.slice] = msg.card
		delete(m.cardLoading, msg.slice)
		m.conflicts = m.buildConflicts()

	case summaryLoadedMsg:
		m.summaries[msg.slice] = msg.text
		delete(m.summaryLoading, msg.slice)
		m.refreshRight()

	case prsLoadedMsg:
		m.prs[msg.slice] = msg.prs
		delete(m.prLoading, msg.slice)
		m.refreshRight()
		return m, persistCommentsCmd(msg.slice, msg.prs)

	case commentCacheMsg:
		m.commentCache = msg.store

	case prsTickMsg:
		// Periodically refresh the focused slice so it feels live without a manual
		// reload — PRs/comments (merge state, "Ready"), plus the diff, stack and
		// processes — in either view. Cached content keeps showing until each
		// refresh lands, so there's no flicker.
		return m, tea.Batch(m.forceLoadPRs(), m.forceLoadDiff(), m.forceLoadStack(), m.forceLoadProcs(), prsTickCmd())

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

	case captureTickMsg:
		var cmd tea.Cmd
		if m.shouldShowCapture() {
			cmd = m.maybeLoadCapture()
		}
		return m, tea.Batch(cmd, captureTickCmd())

	case swapFinishedMsg:
		m.pendingSwap = nil
		sp := config.StatePaths()
		return m, tea.Batch(loadSlicesCmd(m.ws), loadSessionsCmd(m.slices, sp.EventsDir))

	case ciRerunMsg:
		switch {
		case msg.err != nil:
			m.status = "CI rerun failed: " + msg.err.Error()
		case msg.n == 0:
			m.status = "no CI runs to rerun"
		default:
			m.status = fmt.Sprintf("re-ran %d CI run(s) — refresh (r) shortly to see status", msg.n)
		}
		return m, nil

	case ciLogLoadedMsg:
		m.ciLogLoading = false
		if msg.err != nil {
			m.ciLog = "CI logs unavailable: " + msg.err.Error()
		} else {
			m.ciLog = msg.log
		}
		m.refreshRight()
		return m, nil

	case adoptFinishedMsg:
		// Surface the outcome — a failed adopt used to redraw silently to "all
		// clear". The common failure is a branch checked out + dirty in a primary.
		if msg.err != nil {
			m.status = "adopt didn't add a slice — if the branch is checked out with uncommitted changes in a primary, commit or stash it there, then press [i] to retry"
		} else {
			m.status = ""
		}
		sp := config.StatePaths()
		return m, tea.Batch(loadSlicesCmd(m.ws), loadSessionsCmd(m.slices, sp.EventsDir))

	case removeDoneMsg:
		m.selected = make(map[string]bool)
		if msg.failed != "" {
			m.status = fmt.Sprintf("clear failed (%s): dirty/locked — press d then f to force", msg.failed)
		} else {
			m.status = fmt.Sprintf("cleared %d slice%s", msg.cleared, plural(msg.cleared))
		}
		m.view = viewBrowser // cleared slices are gone; return to the hub
		sp := config.StatePaths()
		return m, tea.Batch(loadSlicesCmd(m.ws), loadSessionsCmd(m.slices, sp.EventsDir))

	case stackDoneMsg:
		m.selected = make(map[string]bool)
		switch {
		case msg.conflict != "":
			m.status = "restack: conflict in " + msg.conflict + " — attach (a), resolve, then `gt continue`"
		case msg.dirty != "":
			m.status = "restack: " + msg.dirty + " is dirty — commit or stash first"
		default:
			m.status = fmt.Sprintf("restacked %d repo%s", msg.restacked, plural(msg.restacked))
		}
		sp := config.StatePaths()
		return m, tea.Batch(loadSlicesCmd(m.ws), loadSessionsCmd(m.slices, sp.EventsDir), m.batchLoadCards(), m.loadPreview())

	case editorOpenedMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		}
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

// handleMouse routes wheel events to the pane under the cursor: the right pane
// (diff) scrolls; over the left rail the wheel moves the panel/slice selection.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Detect the wheel by button, not by Action — some terminals/tmux deliver
	// wheel events with an Action other than Press, and gating on Press dropped
	// them (mouse scroll appeared dead).
	up := msg.Button == tea.MouseButtonWheelUp
	down := msg.Button == tea.MouseButtonWheelDown
	if !up && !down {
		return m, nil
	}
	// Ignore while an overlay/help is up.
	if m.showHelp || m.pendingSwap != nil || m.pendingRemove != nil ||
		m.pendingStack != nil || m.showProcOverlay ||
		m.showConflictOverlay || m.showEditorPicker {
		return m, nil
	}

	if m.view == viewCockpit {
		leftW, _, _ := m.cockpitDims()
		if m.zoom || msg.X >= leftW { // over the right pane → scroll the diff
			if up {
				m.viewport.ScrollUp(3)
			} else {
				m.viewport.ScrollDown(3)
			}
			return m, nil
		}
		// over the left rail → move the focused panel's selection
		sl, ok := m.currentSlice()
		if !ok {
			return m, nil
		}
		if up {
			m.moveSel(-1, sl)
		} else {
			m.moveSel(1, sl)
		}
		m.right = rightAuto
		m.viewport.GotoTop()
		m.refreshRight()
		return m, m.loadForPanel()
	}

	// Browser: the wheel moves the slice selection.
	vis := m.hubVisible()
	pos := posInFiltered(vis, m.focus)
	switch {
	case up && pos > 0:
		m.focus = vis[pos-1]
		return m, m.loadPreview()
	case down && pos >= 0 && pos < len(vis)-1:
		m.focus = vis[pos+1]
		return m, m.loadPreview()
	}
	return m, nil
}

// handleKey routes a key press to overlays, then to the active view.
func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Transient status: any keypress dismisses the last status line, so messages
	// like "cleared 3 slices" or "inbox zero" never get stuck. A handler may set
	// a fresh one below.
	m.status = ""

	// Overlays take priority.
	if m.pendingSwap != nil {
		return m.updateSwapKeys(msg)
	}
	if m.pendingRemove != nil {
		return m.updateRemoveKeys(msg)
	}
	if m.pendingStack != nil {
		return m.updateStackKeys(msg)
	}
	if m.showProcOverlay {
		return m.updateOverlayKeys(msg)
	}
	if m.showEditorPicker {
		return m.updateEditorPickerKeys(msg)
	}
	if m.showConflictOverlay {
		switch msg.String() {
		case "!", "esc", "q":
			m.showConflictOverlay = false
		case "j", "down":
			m.conflictScroll++
		case "k", "up":
			if m.conflictScroll > 0 {
				m.conflictScroll--
			}
		}
		return m, nil
	}
	if m.showHelp {
		if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
			m.showHelp = false
		}
		return m, nil
	}

	// Text-input modes (new-slice name, group name, search filter) own every key
	// except their own handler's (enter/esc/backspace/runes). Route straight to the
	// browser handler so typing a name containing q/r/?/P/! inserts the letter
	// instead of firing a global command.
	if m.creating || m.naming || m.filtering {
		return m.updateBrowserKeys(msg)
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
	case "!":
		m.showConflictOverlay = true
		m.conflictScroll = 0
		return m, nil
	case "r":
		// On the cockpit Session panel, [r] refreshes the live pane capture
		// rather than reloading the whole workspace.
		if m.view == viewCockpit && m.panel == panelSession {
			return m, m.maybeLoadCapture()
		}
		m.loading = true
		// Drop cached PR state so merge status (→ "Ready") re-fetches: a merge
		// done outside slis is invisible until PRs reload, and batchLoadAllPRs
		// skips slices already in m.prs.
		m.prs = make(map[string]map[string]*forge.PR)
		m.prLoading = make(map[string]bool)
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
	if err := tmuxctl.EnsureSession(sl.Name, membersSlice(sl), m.sessionOpts()); err != nil {
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

// sessionOpts builds the tmux session layout options from the workspace config.
func (m Model) sessionOpts() tmuxctl.SessionOpts {
	return tmuxctl.SessionOpts{Root: m.ws.Root, Layout: m.ws.Sessions.Layout}
}

// launchAgentCmd ensures the slice's session exists, launches the configured
// agent (default "claude") in it when the active pane is at a shell, then
// attaches. Use this to start/resume the agent; plain [a] attach just attaches.
func (m *Model) launchAgentCmd() tea.Cmd {
	sl, ok := m.currentSlice()
	if !ok {
		return nil
	}
	if !tmuxctl.Available() {
		m.status = "tmux not found on PATH"
		return nil
	}
	if err := tmuxctl.EnsureSession(sl.Name, membersSlice(sl), m.sessionOpts()); err != nil {
		m.status = "session error: " + err.Error()
		return nil
	}
	agent := m.ws.Sessions.Agent
	if agent == "" {
		agent = "claude"
	}
	// Tell Claude it's running inside a slis slice (which repos/branches it spans,
	// whether it's live). No-op for a non-claude agent.
	agent = withSlisContext(agent, sl)
	// Only type the launch command at a shell prompt — avoids typing into an
	// already-running agent (then [C] just re-attaches to it).
	if isShellCmd(tmuxctl.ActivePaneCommand(sl.Name)) {
		_ = tmuxctl.SendKeys(sl.Name, agent)
	}
	m.status = ""
	name, args := tmuxctl.AttachArgv(sl.Name, isInsideTmux())
	c := exec.Command(name, args...)
	return tea.ExecProcess(c, func(error) tea.Msg { return sessionsRefreshMsg{} })
}

// isShellCmd reports whether cmd is an interactive shell (safe to type into).
func isShellCmd(cmd string) bool {
	switch cmd {
	case "zsh", "bash", "fish", "sh", "dash", "ksh", "tcsh":
		return true
	}
	return false
}

// slisCreateCmd hands the terminal to `slis create <name>` (worktrees across
// repos + session), then reloads so the new slice appears in the hub.
func slisCreateCmd(name string) tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	c := exec.Command(self, "create", name) //nolint:gosec
	return tea.ExecProcess(c, func(error) tea.Msg { return swapFinishedMsg{} })
}

// savePrefs persists the remembered UI toggles (best-effort).
func (m Model) savePrefs() {
	_ = config.SavePrefs(config.StatePaths().Prefs, config.Prefs{
		SplitDiff:   m.splitDiff,
		DiffVsTrunk: m.diffVsTrunk,
	})
}

// slisAdoptCmd hands the terminal to the interactive `slis adopt` picker (which
// lists adoptable existing branches), then reloads so a newly-adopted slice
// appears in the hub. ExecProcess suspends the TUI so the picker owns the
// terminal — the same approach used for create/submit/merge.
func slisAdoptCmd() tea.Cmd {
	self, err := os.Executable()
	if err != nil {
		return nil
	}
	c := exec.Command(self, "adopt") //nolint:gosec
	return tea.ExecProcess(c, func(err error) tea.Msg { return adoptFinishedMsg{err: err} })
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

// requestRemove sets up the "clear finished slice(s)" confirmation over the
// selection (or focused slice), refusing up front if any target is live.
func (m *Model) requestRemove() {
	targets := m.actionTargets()
	if len(targets) == 0 {
		return
	}
	for _, name := range targets {
		if m.isActive(name) {
			m.status = name + " is live — deactivate (w) before clearing"
			return
		}
	}
	m.pendingRemove = &removeReq{slices: targets}
}

// updateRemoveKeys handles the clear-slice(s) confirmation prompt.
func (m Model) updateRemoveKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	slices := m.pendingRemove.slices
	switch msg.String() {
	case "y":
		m.pendingRemove = nil
		return m, m.removeCmd(slices, false)
	case "f":
		m.pendingRemove = nil
		return m, m.removeCmd(slices, true)
	case "n", "esc":
		m.pendingRemove = nil
	}
	return m, nil
}

// requestStack sets up the restack/sync confirmation over the selection (or
// focused slice).
func (m *Model) requestStack() {
	if targets := m.actionTargets(); len(targets) > 0 {
		m.pendingStack = &stackReq{slices: targets}
	}
}

// updateStackKeys handles the restack/sync prompt.
func (m Model) updateStackKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	slices := m.pendingStack.slices
	switch msg.String() {
	case "r":
		m.pendingStack = nil
		// Invalidate cached stack/card so badges refresh after the restack.
		for _, n := range slices {
			delete(m.stacks, n)
			delete(m.cards, n)
		}
		return m, m.restackCmd(slices)
	case "p":
		m.pendingStack = nil
		if len(slices) > 0 {
			delete(m.prs, slices[0]) // force PR reload after submit
			return m, slisSubmitCmd(slices[0])
		}
	case "m":
		m.pendingStack = nil
		if len(slices) > 0 {
			delete(m.prs, slices[0]) // PR state flips once Graphite's queue lands
			return m, slisMergeCmd(slices[0])
		}
	case "s":
		m.pendingStack = nil
		if len(slices) > 0 {
			return m, slisSyncCmd(slices[0]) // sync is interactive + repo-wide; one at a time
		}
	case "n", "esc":
		m.pendingStack = nil
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
	if m.pendingStack != nil {
		return renderStackOverlay(m)
	}
	if m.showProcOverlay {
		return renderProcOverlay(m)
	}
	if m.showConflictOverlay {
		return renderConflictOverlay(m)
	}
	if m.showEditorPicker {
		return m.editorPickerView()
	}
	if m.view == viewCockpit {
		return renderCockpit(m)
	}
	return renderBrowser(m)
}

// buildConflicts rebuilds the cross-slice conflict radar from the loaded cards'
// retained per-repo file stats. Slices whose card has not loaded yet are simply
// absent — they contribute no overlaps until their stats arrive.
func (m Model) buildConflicts() *radar.Index {
	stats := make(map[string][]diff.RepoDiff, len(m.cards))
	for name, c := range m.cards {
		stats[name] = c.stats
	}
	return radar.Build(stats)
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
	// WithMouseCellMotion enables wheel events (with cursor X/Y) so scrolling can
	// target the pane under the mouse. Trade-off: native text selection needs
	// Shift held (standard for mouse-enabled TUIs like lazygit).
	p := tea.NewProgram(New(ws), tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}
