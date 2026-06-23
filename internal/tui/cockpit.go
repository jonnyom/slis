package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/tmuxctl"
)

// panel identifies one of the stacked left-hand panels in the cockpit. The
// focused panel drives the content of the large right pane (lazygit-style).
type panel int

const (
	panelStack   panel = iota // repos + their (slice-scoped) Graphite stack
	panelPRs                  // per-repo PR + CI + comment count
	panelSession              // tmux session status + windows
	panelProcs                // processes running in the slice's session
	panelCount
)

func (p panel) title() string {
	switch p {
	case panelStack:
		return "1 Repos & Stack"
	case panelPRs:
		return "2 PRs"
	case panelSession:
		return "3 Session"
	case panelProcs:
		return "4 Processes"
	}
	return "?"
}

// rightMode selects what the right pane shows: the focused panel's detail
// (rightAuto) or the slice summary (rightSummary), toggled with [s].
type rightMode int

const (
	rightAuto rightMode = iota
	rightSummary
)

var (
	panelBorderStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	panelBorderFocusStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("212"))
	panelTitleStyle       = lipgloss.NewStyle().Faint(true)
	panelTitleFocusStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	cockpitHeaderStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	cockpitDimStyle       = lipgloss.NewStyle().Faint(true)
	statusErrStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	selRowStyle           = lipgloss.NewStyle().Bold(true)
)

// cockpitDims returns the left-column width, right-pane width, and body height
// (the rows between the one-line header and footer). leftW is 0 when zoomed or
// when the terminal is too narrow for a sensible split.
func (m Model) cockpitDims() (leftW, rightW, bodyH int) {
	bodyH = m.height - 2
	if bodyH < 3 {
		bodyH = 3
	}
	leftW = 36
	if leftW > m.width/2 {
		leftW = m.width / 2
	}
	if m.zoom {
		leftW = 0
	}
	rightW = m.width - leftW
	if rightW < 10 {
		rightW = 10
	}
	return leftW, rightW, bodyH
}

// resizeViewport (re)creates the right-pane viewport for the current dimensions.
// Called on WindowSizeMsg.
func (m *Model) resizeViewport() {
	_, rightW, bodyH := m.cockpitDims()
	w := rightW - 2 // border
	if w < 1 {
		w = 1
	}
	h := bodyH - 3 // border (2) + title row (1)
	if h < 1 {
		h = 1
	}
	m.viewport.Width = w
	m.viewport.Height = h
	m.refreshRight()
}

// refreshRight recomputes the right-pane content for the focused slice/panel.
func (m *Model) refreshRight() {
	if m.view != viewCockpit {
		return
	}
	m.viewport.SetContent(cockpitRight(*m))
}

// clip truncates s to at most w display cells, ANSI-aware (via lipgloss).
// A non-positive w means "no clipping" (used before the first WindowSizeMsg,
// when width is still unknown).
func clip(s string, w int) string {
	if w <= 0 {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

// justify places left and right on one line padded to width.
func justify(left, right string, width int) string {
	pad := width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// panelBox renders a titled, bordered box of the given total width/height,
// clipping content to fit. The focused box gets a bright border.
func panelBox(title, content string, width, height int, focused bool) string {
	bs, ts := panelBorderStyle, panelTitleStyle
	if focused {
		bs, ts = panelBorderFocusStyle, panelTitleFocusStyle
	}
	cw, ch := width-2, height-2
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	lines := []string{clip(ts.Render(title), cw)}
	for _, ln := range strings.Split(content, "\n") {
		lines = append(lines, clip(ln, cw))
	}
	if len(lines) > ch {
		lines = lines[:ch]
	}
	return bs.Width(cw).Height(ch).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

// renderCockpit renders the full cockpit screen for the focused slice.
func renderCockpit(m Model) string {
	sl, ok := m.currentSlice()
	if !ok {
		return "no slice selected — press esc to go back\n"
	}
	leftW, rightW, bodyH := m.cockpitDims()

	header := clip(justify(
		cockpitHeaderStyle.Render("slis ▸ "+sl.Name),
		cockpitDimStyle.Render("[esc] back  ? help"),
		m.width,
	), m.width)

	rightTitle := clip(panelTitleFocusStyle.Render(rightPaneTitle(m, sl)), rightW-2)
	rightBody := rightTitle + "\n" + m.viewport.View()
	right := panelBorderFocusStyle.Width(rightW - 2).Height(bodyH - 2).MaxHeight(bodyH).Render(rightBody)

	var body string
	if leftW > 0 {
		left := renderLeftColumn(m, sl, leftW, bodyH)
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	} else {
		body = right
	}

	return header + "\n" + body + "\n" + cockpitFooter(m)
}

// renderLeftColumn builds the four stacked panels, sized to fill bodyH.
func renderLeftColumn(m Model, sl model.Slice, w, totalH int) string {
	repos := sl.Repos()

	sessionH, procsH := 5, 5
	prsH := clamp(len(repos)+3, 5, 9)
	stackH := totalH - sessionH - procsH - prsH
	if stackH < 4 { // squeeze PRs panel first on short terminals
		prsH = clamp(prsH-(4-stackH), 3, prsH)
		stackH = totalH - sessionH - procsH - prsH
		if stackH < 1 {
			stackH = 1
		}
	}

	boxes := []string{
		panelBox(panelStack.title(), stackPanelContent(m, sl), w, stackH, m.panel == panelStack),
		panelBox(panelPRs.title(), prsPanelContent(m, sl), w, prsH, m.panel == panelPRs),
		panelBox(panelSession.title(), sessionPanelContent(m, sl), w, sessionH, m.panel == panelSession),
		panelBox(panelProcs.title(), procsPanelContent(m, sl), w, procsH, m.panel == panelProcs),
	}
	return lipgloss.JoinVertical(lipgloss.Left, boxes...)
}

// cockpitFooter renders the context key hints and any transient status message.
func cockpitFooter(m Model) string {
	var hint string
	switch m.panel {
	case panelStack:
		split := "[t]split"
		if m.splitDiff {
			split = "[t]unified"
		}
		hint = "[tab]panel [w]swap [R]restack [s]ummary [a]ttach [o]pen [y]ank " + split + " [⏶⏷^d/^u]scroll [esc]back"
	case panelProcs:
		hint = "[tab]panel [j/k]select [x]kill [X]kill-tree [w]swap [a]ttach [esc]back"
	case panelSession:
		hint = "[tab]panel [a]ttach [r]refresh [w]swap [esc]back"
	default:
		hint = "[tab]panel [w]swap [d]clear [s]ummary [c]omments [Y]copy-stack [F]ix-ci [esc]back"
	}
	if m.status != "" {
		return clip(statusErrStyle.Render(m.status), m.width)
	}
	return clip(cockpitDimStyle.Render(hint), m.width)
}

// rightPaneTitle is the heading shown above the right pane.
func rightPaneTitle(m Model, sl model.Slice) string {
	repos := sl.Repos()
	if m.right == rightSummary {
		return "Summary · " + sl.Name
	}
	repoAt := func(i int) string {
		if len(repos) == 0 {
			return "—"
		}
		return repos[clamp(i, 0, len(repos)-1)]
	}
	switch m.panel {
	case panelStack:
		return repoAt(m.repoSel) + " · Changes"
	case panelPRs:
		return repoAt(m.prSel) + " · PR"
	case panelSession:
		return "Session · " + sl.Name
	case panelProcs:
		return "Processes · " + sl.Name
	}
	return sl.Name
}

// ── Left-panel content ──────────────────────────────────────────────────────

// stackPanelContent renders, per repo, the slice's branch lineage only (its
// ancestors up to trunk plus its descendants) — not every branch in the repo.
func stackPanelContent(m Model, sl model.Slice) string {
	repos := sl.Repos()
	if len(repos) == 0 {
		return cockpitDimStyle.Render("no repos in slice")
	}
	prefix := m.ws.Grouping.StripPrefix
	sliceStacks, loaded := m.stacks[sl.Name]

	var sb strings.Builder
	for i, repo := range repos {
		member := sl.Members[repo]
		marker := "  "
		head := repoHeaderStyle.Render(repo)
		if m.panel == panelStack && i == clamp(m.repoSel, 0, len(repos)-1) {
			marker = "▸ "
			head = selRowStyle.Render(repoHeaderStyle.Render(repo))
		}
		sb.WriteString(marker + head + "\n")

		if !loaded {
			sb.WriteString("    " + cockpitDimStyle.Render("loading…") + "\n")
			continue
		}
		lineage := sliceStacks[repo].Lineage(member.Branch)
		if len(lineage) == 0 {
			// No Graphite data — show the branch on its own.
			sb.WriteString("    " + shortBranch(member.Branch, prefix) + "\n")
			continue
		}
		for _, b := range lineage {
			indent := strings.Repeat("  ", b.Depth+1)
			name := shortBranch(b.Name, prefix)
			switch {
			case b.Trunk:
				sb.WriteString(trunkStyle.Render(indent+name+" [trunk]") + "\n")
			case b.NeedsRestack:
				sb.WriteString(needsRestackStyle.Render(indent+name+" ⚠ restack") + "\n")
			case b.Name == member.Branch:
				sb.WriteString(selRowStyle.Render(indent+name) + "\n")
			default:
				sb.WriteString(indent + name + "\n")
			}
		}
	}
	return sb.String()
}

// prsPanelContent renders one line per repo: PR number, CI emoji, comment count.
func prsPanelContent(m Model, sl model.Slice) string {
	repos := sl.Repos()
	if len(repos) == 0 {
		return cockpitDimStyle.Render("no repos in slice")
	}
	slicePRs, loaded := m.prs[sl.Name]
	loadingPRs := m.prLoading[sl.Name]

	var sb strings.Builder
	for i, repo := range repos {
		marker := "  "
		if m.panel == panelPRs && i == clamp(m.prSel, 0, len(repos)-1) {
			marker = "▸ "
		}
		line := repo + "  "
		switch {
		case loadingPRs && !loaded:
			line += cockpitDimStyle.Render("PR: loading…")
		case loaded:
			pr := slicePRs[repo]
			switch {
			case pr == nil:
				line += cockpitDimStyle.Render("(no PR)")
			case strings.EqualFold(pr.State, "MERGED"):
				line += fmt.Sprintf("#%d ", pr.Number) + mergedStyle.Render("merged") + fmt.Sprintf(" 💬%d", len(pr.Comments))
			default:
				overall, _, _, _ := pr.CISummary()
				line += fmt.Sprintf("#%d %s 💬%d", pr.Number, forge.CIEmoji(overall), len(pr.Comments))
			}
		default:
			line += cockpitDimStyle.Render("—")
		}
		sb.WriteString(marker + line + "\n")
	}
	return sb.String()
}

// sessionPanelContent renders a compact session summary (badge + window count)
// plus the most recent line of session output for a glance.
func sessionPanelContent(m Model, sl model.Slice) string {
	status := model.SessNone
	if m.sessionStatus != nil {
		status = m.sessionStatus[sl.Name]
	}
	out := fmt.Sprintf("%s %s · %d windows", sessionBadge(status), status.String(), len(sl.Repos()))
	if cap, ok := m.captures[sl.Name]; ok {
		if last := tailLines(cap, 1); last != "" {
			out += "\n" + cockpitDimStyle.Render(last)
		}
	}
	return out
}

// tailLines returns the last n lines of s, ignoring trailing blank lines.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// procsPanelContent renders the top processes by CPU with a warning badge.
func procsPanelContent(m Model, sl model.Slice) string {
	if m.procLoading[sl.Name] {
		return cockpitDimStyle.Render("sampling…")
	}
	procs, ok := m.procs[sl.Name]
	if !ok {
		return cockpitDimStyle.Render("no session / not sampled")
	}
	if len(procs) == 0 {
		return cockpitDimStyle.Render("no processes")
	}
	warn := ""
	if overCPUThreshold(procs, m.ws.Processes.CPUWarnPct) {
		warn = cpuWarnStyle.Render(" ⚠")
	}
	top := procs
	if len(top) > 2 {
		top = top[:2]
	}
	var sb strings.Builder
	for _, p := range top {
		sb.WriteString(fmt.Sprintf("● %s %.0f%%\n", p.Cmd, p.CPU))
	}
	sb.WriteString(fmt.Sprintf("Σ %.0f%%%s", sliceCPU(procs), warn))
	return sb.String()
}

// ── Right-pane content ──────────────────────────────────────────────────────

// cockpitRight returns the full right-pane text for the focused panel / mode.
func cockpitRight(m Model) string {
	sl, ok := m.currentSlice()
	if !ok {
		return "no slice"
	}
	if m.right == rightSummary {
		return summaryContent(m, sl)
	}
	switch m.panel {
	case panelStack:
		return repoDiffContent(m, sl)
	case panelPRs:
		return prDetailContent(m, sl)
	case panelSession:
		return sessionDetailContent(m, sl)
	case panelProcs:
		return procDetailContent(m, sl)
	}
	return ""
}

func repoDiffContent(m Model, sl model.Slice) string {
	repos := sl.Repos()
	if len(repos) == 0 {
		return "no repos\n"
	}
	repo := repos[clamp(m.repoSel, 0, len(repos)-1)]
	if m.diffLoading[sl.Name] {
		return "loading diff…\n"
	}
	diffs, ok := m.diffs[sl.Name]
	if !ok {
		return "loading diff…\n"
	}
	var rd *diff.RepoDiff
	for i := range diffs {
		if diffs[i].Repo == repo {
			rd = &diffs[i]
			break
		}
	}
	if rd == nil {
		return "no diff for " + repo + "\n"
	}
	if rd.Err != "" {
		return "error: " + rd.Err + "\n"
	}
	if len(rd.Files) == 0 {
		return "no changes vs trunk\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d files  +%d -%d\n\n", len(rd.Files), rd.TotalAdded(), rd.TotalDeleted())
	for _, f := range rd.Files {
		if f.Added == -1 {
			fmt.Fprintf(&sb, "  %s  (binary)\n", f.Path)
		} else {
			fmt.Fprintf(&sb, "  %s  +%d -%d\n", f.Path, f.Added, f.Deleted)
		}
	}
	if rd.Patch != "" {
		sb.WriteString("\n")
		if m.splitDiff {
			sb.WriteString(renderSplitDiff(rd.Patch, m.viewport.Width) + "\n")
		} else {
			sb.WriteString(colorizePatch(rd.Patch) + "\n")
		}
	}
	return sb.String()
}

func prDetailContent(m Model, sl model.Slice) string {
	repos := sl.Repos()
	if len(repos) == 0 {
		return "no repos\n"
	}
	repo := repos[clamp(m.prSel, 0, len(repos)-1)]
	if m.prLoading[sl.Name] {
		return "loading PRs…\n"
	}
	slicePRs, ok := m.prs[sl.Name]
	if !ok {
		return "PRs not loaded — focus this panel to load.\n"
	}
	pr := slicePRs[repo]
	if pr == nil {
		return repo + ": no PR for this branch\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s  #%d  %s\n", repo, pr.Number, pr.State)
	if pr.Title != "" {
		sb.WriteString(pr.Title + "\n")
	}
	sb.WriteString(pr.URL + "\n")
	if len(pr.Checks) > 0 {
		sb.WriteString("\nCI:\n")
		for _, c := range pr.Checks {
			fmt.Fprintf(&sb, "  %s %s\n", forge.CIEmoji(c.State), c.Name)
		}
	}
	if len(pr.Comments) > 0 {
		sb.WriteString("\nComments:\n")
		for _, c := range pr.Comments {
			sb.WriteString("  " + commentLine(repo, pr, c) + "\n")
		}
	}
	return sb.String()
}

func sessionDetailContent(m Model, sl model.Slice) string {
	status := model.SessNone
	if m.sessionStatus != nil {
		status = m.sessionStatus[sl.Name]
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "session: %s\n", tmuxctl.SessionName(sl.Name))
	fmt.Fprintf(&sb, "status:  %s %s\n", sessionBadge(status), status.String())
	if !tmuxctl.Available() {
		sb.WriteString("\ntmux not found on PATH\n")
		return sb.String()
	}
	if m.ws.Root != "" && m.ws.Sessions.Layout != "repos" {
		fmt.Fprintf(&sb, "\nattach opens at root:  %s\n", m.ws.Root)
	}
	sb.WriteString("\nrepos:\n")
	for _, repo := range sl.Repos() {
		fmt.Fprintf(&sb, "  %s  %s\n", repo, sl.Members[repo].WorktreePath)
	}
	sb.WriteString("\n[a] attach   [r] refresh capture\n")

	// Live peek at what's running in the session (e.g. a Claude prompt) so you
	// can tell when it needs attention without attaching.
	if m.captureLoading[sl.Name] {
		sb.WriteString("\ncapturing session output…\n")
	} else if cap, ok := m.captures[sl.Name]; ok {
		sb.WriteString("\n─── session output (live) ───\n")
		if strings.TrimSpace(cap) == "" {
			sb.WriteString("(empty / no session)\n")
		} else {
			sb.WriteString(cap)
		}
	} else if status == model.SessNone {
		sb.WriteString("\n(no session — press [a] to start one)\n")
	}
	return sb.String()
}

func procDetailContent(m Model, sl model.Slice) string {
	if m.procLoading[sl.Name] {
		return "sampling processes…\n"
	}
	procs, ok := m.procs[sl.Name]
	if !ok {
		return "no tmux session for this slice — [a] to start one, or [P] for the global overlay\n"
	}
	if len(procs) == 0 {
		return "no processes found in this slice's session\n"
	}
	width := m.viewport.Width
	if width < 20 {
		width = 80
	}
	return renderProcTable(procs, clamp(m.procSel, 0, len(procs)-1), width)
}

func summaryContent(m Model, sl model.Slice) string {
	if m.summaryLoading[sl.Name] {
		return "generating summary…\n"
	}
	text, ok := m.summaries[sl.Name]
	if !ok {
		return "loading summary…\n"
	}
	if text == "" {
		return "(no commits vs trunk)\n\n[s] AI summary\n"
	}
	return text + "\n[s] AI summary\n"
}

// renderSwapOverlay renders the activate/deactivate confirmation modal.
func renderSwapOverlay(m Model) string {
	if m.pendingSwap == nil {
		return ""
	}
	req := *m.pendingSwap
	key := panelTitleFocusStyle.Render
	var sb strings.Builder
	if req.deactivate {
		sb.WriteString(cockpitHeaderStyle.Render("Deactivate "+req.slice) + "\n\n")
		sb.WriteString("Restore every repo's primary checkout to where it was before this\n")
		sb.WriteString("slice was swapped in.\n\n")
		sb.WriteString(key("[y]") + " deactivate     " + key("[n]") + " cancel\n")
	} else {
		sb.WriteString(cockpitHeaderStyle.Render("Set running code → "+req.slice) + "\n\n")
		sb.WriteString("Detach each repo's primary HEAD to this slice's branch tips so running\n")
		sb.WriteString("dev servers rebuild this feature. Reversible; worktrees are untouched.\n")
		sb.WriteString("A dirty primary needs " + cockpitDimStyle.Render("--stash") + ".\n\n")
		sb.WriteString(key("[y]") + " activate     " + key("[s]") + " activate + stash dirty     " + key("[n]") + " cancel\n")
	}
	return helpBoxStyle.Render(sb.String())
}

// targetLabel describes a set of slice targets for an overlay title.
func targetLabel(slices []string) string {
	if len(slices) == 1 {
		return slices[0]
	}
	return fmt.Sprintf("%d slices", len(slices))
}

// renderStackOverlay renders the restack/sync action modal.
func renderStackOverlay(m Model) string {
	if m.pendingStack == nil {
		return ""
	}
	key := panelTitleFocusStyle.Render
	var sb strings.Builder
	sb.WriteString(cockpitHeaderStyle.Render("Stack actions — "+targetLabel(m.pendingStack.slices)) + "\n\n")
	sb.WriteString("Restack rebases each branch onto its parent (dirty skipped, conflicts left\n")
	sb.WriteString("for you). Submit pushes the stack + opens/updates PRs. Merge hands off to\n")
	sb.WriteString("Graphite's server-side queue (squash/merge/restack handled for you — no\n")
	sb.WriteString("local waiting). Sync is repo-wide. Submit/merge/sync act on the first target.\n\n")
	sb.WriteString(key("[r]") + " restack   " + key("[p]") + " submit   " + key("[m]") + " merge (Graphite)   " + key("[s]") + " sync   " + key("[n]") + " cancel\n")
	return helpBoxStyle.Render(sb.String())
}

// renderRemoveOverlay renders the clear-finished-slice confirmation modal.
func renderRemoveOverlay(m Model) string {
	if m.pendingRemove == nil {
		return ""
	}
	key := panelTitleFocusStyle.Render
	var sb strings.Builder
	sb.WriteString(cockpitHeaderStyle.Render("Clear "+targetLabel(m.pendingRemove.slices)) + "\n\n")
	sb.WriteString("Remove each repo's worktree, kill the tmux session, and delete merged\n")
	sb.WriteString("branches. Refuses dirty worktrees / unmerged branches unless forced.\n\n")
	sb.WriteString(key("[y]") + " clear     " + key("[f]") + " force (dirty + unmerged)     " + key("[n]") + " cancel\n")
	return helpBoxStyle.Render(sb.String())
}

// shortBranch trims an optional grouping prefix for display.
func shortBranch(b, prefix string) string {
	if prefix != "" {
		b = strings.TrimPrefix(b, prefix)
	}
	return b
}

// ── Cockpit key handling ────────────────────────────────────────────────────

// enterCockpit switches to the cockpit for the focused slice and kicks off the
// loads its panels need.
func (m *Model) enterCockpit() tea.Cmd {
	m.view = viewCockpit
	m.panel = panelStack
	m.right = rightAuto
	m.zoom = false
	m.repoSel, m.prSel, m.procSel = 0, 0, 0
	m.status = ""
	m.resizeViewport()
	cmds := []tea.Cmd{m.maybeLoadStack(), m.maybeLoadDiff(), m.maybeLoadPRs(), m.maybeLoadProcs()}
	m.refreshRight()
	return tea.Batch(filterNil(cmds)...)
}

// loadForPanel returns the loader(s) the newly-focused panel needs.
func (m *Model) loadForPanel() tea.Cmd {
	switch m.panel {
	case panelStack:
		return tea.Batch(filterNil([]tea.Cmd{m.maybeLoadStack(), m.maybeLoadDiff()})...)
	case panelPRs:
		return m.maybeLoadPRs()
	case panelSession:
		return m.maybeLoadCapture()
	case panelProcs:
		return m.maybeLoadProcs()
	}
	return nil
}

// selCount returns the number of selectable rows in the focused panel.
func (m Model) selCount(sl model.Slice) int {
	switch m.panel {
	case panelStack, panelPRs:
		return len(sl.Repos())
	case panelProcs:
		return len(m.procs[sl.Name])
	}
	return 0
}

func (m *Model) moveSel(delta int, sl model.Slice) {
	n := m.selCount(sl)
	if n == 0 {
		return
	}
	switch m.panel {
	case panelStack:
		m.repoSel = clamp(m.repoSel+delta, 0, n-1)
	case panelPRs:
		m.prSel = clamp(m.prSel+delta, 0, n-1)
	case panelProcs:
		m.procSel = clamp(m.procSel+delta, 0, n-1)
	}
}

// updateCockpitKeys handles key events while the cockpit is showing.
func (m Model) updateCockpitKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	sl, ok := m.currentSlice()
	if !ok {
		m.view = viewBrowser
		return m, nil
	}

	switch msg.String() {
	case "esc", "h":
		m.view = viewBrowser
		m.zoom = false
		return m, nil
	case "enter":
		m.zoom = !m.zoom
		m.resizeViewport()
		return m, nil
	case "tab", "L":
		m.panel = (m.panel + 1) % panelCount
		m.right = rightAuto
		m.refreshRight()
		return m, m.loadForPanel()
	case "shift+tab", "H":
		m.panel = (m.panel + panelCount - 1) % panelCount
		m.right = rightAuto
		m.refreshRight()
		return m, m.loadForPanel()
	case "1":
		m.panel, m.right = panelStack, rightAuto
		m.refreshRight()
		return m, m.loadForPanel()
	case "2":
		m.panel, m.right = panelPRs, rightAuto
		m.refreshRight()
		return m, m.loadForPanel()
	case "3":
		m.panel, m.right = panelSession, rightAuto
		m.refreshRight()
		return m, m.loadForPanel()
	case "4":
		m.panel, m.right = panelProcs, rightAuto
		m.refreshRight()
		return m, m.loadForPanel()
	case "j", "down":
		m.moveSel(1, sl)
		m.right = rightAuto
		m.viewport.GotoTop()
		m.refreshRight()
		return m, m.loadForPanel()
	case "k", "up":
		m.moveSel(-1, sl)
		m.right = rightAuto
		m.viewport.GotoTop()
		m.refreshRight()
		return m, m.loadForPanel()
	case "ctrl+d", "pgdown", " ":
		m.viewport.HalfPageDown()
		return m, nil
	case "ctrl+u", "pgup":
		m.viewport.HalfPageUp()
		return m, nil
	case "g":
		m.viewport.GotoTop()
		return m, nil
	case "G":
		m.viewport.GotoBottom()
		return m, nil
	case "s":
		if m.right == rightSummary {
			m.right = rightAuto
			m.refreshRight()
			return m, nil
		}
		m.right = rightSummary
		cmd := m.maybeLoadSummary()
		m.refreshRight()
		return m, cmd
	case "S":
		// Force a fresh AI summary.
		m.right = rightSummary
		m.summaryLoading[sl.Name] = true
		delete(m.summaries, sl.Name)
		m.refreshRight()
		return m, aiSummaryFromSliceCmd(sl)
	case "w":
		m.requestSwap()
		return m, nil
	case "d":
		m.requestRemove()
		return m, nil
	case "R":
		m.requestStack()
		return m, nil
	case "t":
		m.splitDiff = !m.splitDiff
		m.refreshRight()
		return m, nil
	case "a":
		return m, m.attachCmd()
	case "o":
		return m, openExternalCmd(m)
	case "y":
		return m, copyPatchCmd(m)
	case "Y":
		if cmd := copyPRStackCmd(m); cmd != nil {
			return m, cmd
		}
		return m, m.maybeLoadPRs()
	case "c":
		m.showCommentsOverlay = true
		m.commentsSel = 0
		return m, m.maybeLoadPRs()
	case "F":
		return m, fixCICmd(sl)
	case "x":
		if m.panel == panelProcs {
			if procs := m.procs[sl.Name]; len(procs) > 0 {
				m.pendingKill = &killReq{pid: procs[clamp(m.procSel, 0, len(procs)-1)].PID}
				m.showProcOverlay = true
				m.overlayProcs = flattenProcs(m.procs)
			}
		}
		return m, nil
	}
	return m, nil
}

// filterNil drops nil commands so tea.Batch isn't fed empties.
func filterNil(cmds []tea.Cmd) []tea.Cmd {
	out := cmds[:0]
	for _, c := range cmds {
		if c != nil {
			out = append(out, c)
		}
	}
	return out
}
