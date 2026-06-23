package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jonnyom/slis/internal/diff"
	"github.com/jonnyom/slis/internal/forge"
	"github.com/jonnyom/slis/internal/gt"
	"github.com/jonnyom/slis/internal/model"
	"github.com/jonnyom/slis/internal/summary"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	focusStyle    = lipgloss.NewStyle().Bold(true)
	footerStyle   = lipgloss.NewStyle().Faint(true)
	cursorBar     = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	syncedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("35"))
	overviewStyle = lipgloss.NewStyle().Faint(true)
	headerStyle   = lipgloss.NewStyle().Faint(true)
	colHeadStyle  = lipgloss.NewStyle().Faint(true).Bold(true)
	// create-mode input: a filled magenta chip so entering "new slice" mode is
	// unmissable (the old faint label was easy to overlook).
	createChipStyle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("231")).
			Background(lipgloss.Color("198")).
			Padding(0, 1)
	createNameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	waitStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true) // needs-input
	doneStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("213")).Bold(true) // finished a turn — your move
	liveStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)  // currently-active slice
	mergedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("141"))            // a merged PR
	readyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("120")).Bold(true) // ready-to-clear tag
	emptyBoxStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 3)
	codeStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("159"))
)

// renderCreatePrompt renders the active "new slice" input as a loud magenta chip
// plus the typed name and a cursor, so create mode is obvious at a glance.
func renderCreatePrompt(name string) string {
	return "  " + createChipStyle.Render("✎ new slice") + " " +
		createNameStyle.Render(name) + cursorBar.Render("▏")
}

// renderEmptyState renders the hub when there are no slices: a welcome (no
// workspace) or an "all clear" state (workspace set up, nothing in flight).
func renderEmptyState(m Model) string {
	hdr := titleStyle.Render("slis") + headerStyle.Render("  ·  0 slices")
	if m.creating {
		hdr += renderCreatePrompt(m.createName)
	}
	header := clip(hdr, m.width)
	hint := "[c] new slice   [r] refresh   [?] help   [q] quit"
	if m.status != "" {
		hint = m.status
	}
	footer := clip(footerStyle.Render(hint), m.width)

	var b strings.Builder
	if len(m.ws.Repos) == 0 {
		b.WriteString(titleStyle.Render("Welcome to slis") + "\n\n")
		b.WriteString("No workspace configured yet. Point slis at your project:\n\n")
		b.WriteString(codeStyle.Render("slis init <project-root> --repos repoA,repoB --strip-prefix you/") + "\n\n")
		b.WriteString(overviewStyle.Render("…then press [r] to refresh."))
	} else {
		var repos []string
		for n := range m.ws.Repos {
			repos = append(repos, n)
		}
		sort.Strings(repos)

		b.WriteString(readyStyle.Render("✦  All clear — no active slices") + "\n\n")
		b.WriteString("A slice is a feature's git worktrees across your repos.\n")
		b.WriteString("Start one with a worktree on a feature branch:\n\n")
		b.WriteString(readyStyle.Render("press [c]") + " to create one across all repos, or by hand:\n")
		b.WriteString(codeStyle.Render("git -C <repo> worktree add .worktrees/<name> -b <name>") + "\n\n")
		b.WriteString(overviewStyle.Render("workspace:  ") + strings.Join(repos, " · ") + "\n\n")
		b.WriteString(overviewStyle.Render("…then [r] to refresh."))
	}
	card := emptyBoxStyle.Render(b.String())

	if m.width <= 0 || m.height <= 0 {
		return header + "\n\n" + card + "\n" + footer
	}
	centered := lipgloss.Place(m.width, max(1, m.height-2), lipgloss.Center, lipgloss.Center, card)
	return header + "\n" + centered + "\n" + footer
}

// mergeState summarises a slice's PRs for the "ready to clean up" signal.
type mergeState int

const (
	mergeNone    mergeState = iota // no PRs loaded/found
	mergeOpen                      // PRs exist, none merged
	mergePartial                   // some merged, some not
	mergeReady                     // every member PR is merged → ready to clear
)

// sliceMergeState reports whether a slice's PRs are all merged.
func (m Model) sliceMergeState(s model.Slice) mergeState {
	slicePRs, ok := m.prs[s.Name]
	if !ok {
		return mergeNone
	}
	prs, merged := 0, 0
	for _, repo := range s.Repos() {
		if pr := slicePRs[repo]; pr != nil {
			prs++
			if strings.EqualFold(pr.State, "MERGED") {
				merged++
			}
		}
	}
	switch {
	case prs == 0:
		return mergeNone
	case merged == prs:
		return mergeReady
	case merged == 0:
		return mergeOpen
	default:
		return mergePartial
	}
}

// readyCount counts slices whose PRs are all merged (ready to clear).
func (m Model) readyCount() int {
	n := 0
	for _, s := range m.slices {
		if m.sliceMergeState(s) == mergeReady {
			n++
		}
	}
	return n
}

// sliceCard is the lazily-computed browser summary of a slice: what it's about
// (latest commit subject), its diffstat, commit count, and stack health. PR
// information is overlaid at render time from the (separately-loaded) PR cache.
type sliceCard struct {
	overview   string // newest commit subject across the slice's repos
	added      int
	deleted    int
	commits    int
	restack    int  // needs-restack branches across the slice's lineages
	stackKnown bool // Graphite data was available for at least one repo
}

// cardLoadedMsg is delivered when a slice's browser card has been computed.
type cardLoadedMsg struct {
	slice string
	card  sliceCard
}

// loadCardCmd computes a slice's browser card off the UI goroutine: commit
// subjects/count, diffstat (numstat only), and stack health. Stats are measured
// against each branch's Graphite PARENT (this slice's own changes), matching the
// cockpit diff, falling back to the repo trunk when not stacked.
func loadCardCmd(sl model.Slice) tea.Cmd {
	return func() tea.Msg {
		var card sliceCard

		// One gt.ReadState per member: derive the parent (the stat base) and the
		// stack health in a single pass.
		bases := make(map[string]string, len(sl.Members))
		for _, repo := range sl.Repos() {
			member := sl.Members[repo]
			if member.WorktreePath == "" {
				continue
			}
			st, err := gt.ReadState(member.WorktreePath)
			if err != nil || len(st) == 0 {
				continue
			}
			if bs, ok := st[member.Branch]; ok && len(bs.Parents) > 0 {
				bases[repo] = strings.TrimSpace(bs.Parents[0].Ref)
			}
			if lineage := st.Lineage(member.Branch); len(lineage) > 0 {
				card.stackKnown = true
				for _, b := range lineage {
					if b.NeedsRestack {
						card.restack++
					}
				}
			}
		}

		byRepo, _ := summary.CommitSummaryBases(sl, bases)
		for _, repo := range sl.Repos() {
			subs := byRepo[repo]
			card.commits += len(subs)
			if card.overview == "" && len(subs) > 0 {
				card.overview = subs[0] // newest first
			}
		}

		stats, _ := diff.SliceStatBases(sl, bases)
		for _, rd := range stats {
			card.added += rd.TotalAdded()
			card.deleted += rd.TotalDeleted()
		}

		return cardLoadedMsg{slice: sl.Name, card: card}
	}
}

// batchLoadCards loads cards for every slice not yet loaded/loading.
func (m *Model) batchLoadCards() tea.Cmd {
	var cmds []tea.Cmd
	for _, sl := range m.slices {
		if _, ok := m.cards[sl.Name]; ok {
			continue
		}
		if m.cardLoading[sl.Name] {
			continue
		}
		m.cardLoading[sl.Name] = true
		cmds = append(cmds, loadCardCmd(sl))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// posInFiltered returns the position of focus within an index list, or -1.
func posInFiltered(idxs []int, focus int) int {
	for p, i := range idxs {
		if i == focus {
			return p
		}
	}
	return -1
}

// waitingCount counts slices whose session is waiting for input.
func (m Model) waitingCount() int {
	n := 0
	for _, s := range m.slices {
		if m.sessionStatus[s.Name] == model.SessWaitingInput {
			n++
		}
	}
	return n
}

// activeName returns the name of the currently-swapped-in (live) slice, or "".
func (m Model) activeName() string {
	for _, s := range m.slices {
		if s.Active {
			return s.Name
		}
	}
	return ""
}

// ── Slice lifecycle state (drives the dashboard's state-filter rail) ─────────

type sliceState int

const (
	stInProgress sliceState = iota // worktree/commits, no open PR yet
	stNeedsYou                     // Claude waiting for input, or CI failing
	stInReview                     // open PR, CI not failing — awaiting review/merge
	stReady                        // all PRs merged — ready to clear
)

// workState classifies a slice for the state filters, using data already loaded.
func (m Model) workState(s model.Slice) sliceState {
	if st := m.sessionStatus[s.Name]; st == model.SessWaitingInput || st == model.SessDone {
		// Blocked on input, or just finished a turn — either way it's your move.
		return stNeedsYou
	}
	if m.sliceMergeState(s) == mergeReady {
		return stReady
	}
	if slicePRs, ok := m.prs[s.Name]; ok {
		hasOpen := false
		for _, repo := range s.Repos() {
			pr := slicePRs[repo]
			if pr == nil {
				continue
			}
			if overall, _, _, _ := pr.CISummary(); overall == forge.CheckFail {
				return stNeedsYou
			}
			if strings.EqualFold(pr.State, "OPEN") {
				hasOpen = true
			}
		}
		if hasOpen {
			return stInReview
		}
	}
	return stInProgress
}

type hubFilter struct {
	label string
	match func(m Model, s model.Slice) bool
}

func hubFilters() []hubFilter {
	return []hubFilter{
		{"All", func(Model, model.Slice) bool { return true }},
		{"Needs you", func(m Model, s model.Slice) bool { return m.workState(s) == stNeedsYou }},
		{"In review", func(m Model, s model.Slice) bool { return m.workState(s) == stInReview }},
		{"Ready", func(m Model, s model.Slice) bool { return m.workState(s) == stReady }},
		{"In progress", func(m Model, s model.Slice) bool { return m.workState(s) == stInProgress }},
		{"Needs restack", func(m Model, s model.Slice) bool { c, ok := m.cards[s.Name]; return ok && c.restack > 0 }},
		{"Live", func(_ Model, s model.Slice) bool { return s.Active }},
		{"Inbox", func(m Model, s model.Slice) bool { return m.inInbox(s) }},
	}
}

// attentionRank ranks how urgently a slice needs YOUR attention; lower = more
// urgent, 99 = not in the inbox (nothing for you to do right now).
func (m Model) attentionRank(s model.Slice) int {
	if m.sessionStatus[s.Name] == model.SessWaitingInput {
		return 0 // Claude is blocked on you
	}
	if m.sessionStatus[s.Name] == model.SessDone {
		return 1 // Claude finished a turn — review / your move
	}
	if slicePRs, ok := m.prs[s.Name]; ok {
		for _, repo := range s.Repos() {
			if pr := slicePRs[repo]; pr != nil && !strings.EqualFold(pr.State, "MERGED") {
				if overall, _, _, _ := pr.CISummary(); overall == forge.CheckFail {
					return 1 // CI is red
				}
			}
		}
	}
	if c, ok := m.cards[s.Name]; ok && c.restack > 0 {
		return 2 // needs restack
	}
	if m.sliceMergeState(s) == mergeReady {
		return 3 // merged — ready to clear
	}
	return 99
}

// inInbox reports whether a slice needs the user's attention.
func (m Model) inInbox(s model.Slice) bool { return m.attentionRank(s) < 99 }

// attentionOrder returns indices of inbox slices, most-urgent first.
func (m Model) attentionOrder() []int {
	var idxs []int
	for i, s := range m.slices {
		if m.inInbox(s) {
			idxs = append(idxs, i)
		}
	}
	sort.SliceStable(idxs, func(a, b int) bool {
		ra, rb := m.attentionRank(m.slices[idxs[a]]), m.attentionRank(m.slices[idxs[b]])
		if ra != rb {
			return ra < rb
		}
		return m.slices[idxs[a]].Name < m.slices[idxs[b]].Name
	})
	return idxs
}

// restackCount counts slices with at least one branch needing a restack.
func (m Model) restackCount() int {
	n := 0
	for _, s := range m.slices {
		if c, ok := m.cards[s.Name]; ok && c.restack > 0 {
			n++
		}
	}
	return n
}

func (m Model) filterCount(i int) int {
	fs := hubFilters()
	if i < 0 || i >= len(fs) {
		return 0
	}
	n := 0
	for _, s := range m.slices {
		if fs[i].match(m, s) {
			n++
		}
	}
	return n
}

// hubVisible returns indices into m.slices matching the active state filter AND
// the text filter, in order.
func (m Model) hubVisible() []int {
	fs := hubFilters()
	filt := fs[clamp(m.filterIdx, 0, len(fs)-1)]
	f := strings.ToLower(m.filter)
	var out []int
	for i, s := range m.slices {
		if f != "" && !strings.Contains(strings.ToLower(s.Name), f) {
			continue
		}
		if !filt.match(m, s) {
			continue
		}
		out = append(out, i)
	}
	// The Inbox is a triage queue: order by urgency, not by name.
	if filt.label == "Inbox" {
		sort.SliceStable(out, func(a, b int) bool {
			ra, rb := m.attentionRank(m.slices[out[a]]), m.attentionRank(m.slices[out[b]])
			if ra != rb {
				return ra < rb
			}
			return m.slices[out[a]].Name < m.slices[out[b]].Name
		})
	}
	return out
}

// previewSlice returns the slice to preview (the focused one if visible, else
// the first visible).
func (m Model) previewSlice(vis []int) (model.Slice, bool) {
	for _, i := range vis {
		if i == m.focus {
			return m.slices[i], true
		}
	}
	if len(vis) > 0 {
		return m.slices[vis[0]], true
	}
	return model.Slice{}, false
}

// renderBrowser renders the dashboard hub: a pulse bar, a state-filter rail +
// slice list on the left, and a live preview of the selected slice on the right.
func renderBrowser(m Model) string {
	// No slices: a proper empty state (NOT "run init" — inside the TUI a
	// workspace is always loaded, so 0 slices means "all clear", not "unset").
	if len(m.slices) == 0 {
		return renderEmptyState(m)
	}

	// Pre-resize / headless (no known terminal size): a simple list of all
	// visible slices, so the first frame and tests render sensibly.
	if m.width <= 0 || m.height <= 0 {
		var sb strings.Builder
		sb.WriteString(titleStyle.Render("slis") + headerStyle.Render(fmt.Sprintf("  ·  %d slices", len(m.slices))) + "\n\n")
		for _, i := range m.hubVisible() {
			s := m.slices[i]
			marker := "  "
			if i == m.focus {
				marker = cursorBar.Render("▎") + " "
			}
			sb.WriteString(marker + sliceGlyph(m, s) + " " + s.Name + "\n")
		}
		return sb.String()
	}

	// ── Pulse bar. ──
	var head strings.Builder
	head.WriteString(titleStyle.Render("slis"))
	head.WriteString(headerStyle.Render(fmt.Sprintf("  ·  %d slices", len(m.slices))))
	if live := m.activeName(); live != "" {
		head.WriteString("   " + liveStyle.Render("● live: "+live))
	}
	if w := m.waitingCount(); w > 0 {
		head.WriteString("   " + waitStyle.Render(fmt.Sprintf("⏸ %d need you", w)))
	}
	if r := m.readyCount(); r > 0 {
		head.WriteString("   " + readyStyle.Render(fmt.Sprintf("♻ %d ready", r)))
	}
	if rs := m.restackCount(); rs > 0 {
		head.WriteString("   " + needsRestackStyle.Render(fmt.Sprintf("⟳ %d need restack", rs)))
	}
	if m.creating {
		head.WriteString(renderCreatePrompt(m.createName))
	} else if m.naming {
		head.WriteString("   " + headerStyle.Render("group name: ") + m.groupName + "▏")
	} else if n := len(m.selected); n > 0 {
		head.WriteString("   " + focusStyle.Render(fmt.Sprintf("%d selected", n)))
	} else if m.filtering || m.filter != "" {
		cur := ""
		if m.filtering {
			cur = "▏"
		}
		head.WriteString("   " + headerStyle.Render("/") + m.filter + cur)
	}
	top := clip(head.String(), m.width)

	footerText := "n next-todo · enter open · c new · C claude · a attach · w live · d clear · R stack · space/A select · / search · ? help"
	if m.status != "" {
		footerText = m.status
	}
	footer := clip(footerStyle.Render(footerText), m.width)

	leftW := clamp(m.width/4, 20, 30)
	rightW := m.width - leftW
	bodyH := m.height - 2
	if bodyH < 6 {
		bodyH = 6
	}

	filters := hubFilters()
	statesH := len(filters) + 3 // border (2) + title (1) + one row per filter
	if statesH > bodyH-4 {
		statesH = bodyH - 4
	}
	slicesH := bodyH - statesH

	vis := m.hubVisible()
	statesBox := panelBox("States", statesContent(m), leftW, statesH, m.hubFocus == 1)
	slicesBox := panelBox(fmt.Sprintf("Slices %d", len(vis)), slicesContent(m, vis, slicesH-2), leftW, slicesH, m.hubFocus == 0)
	left := lipgloss.JoinVertical(lipgloss.Left, statesBox, slicesBox)

	title := "—"
	preview := overviewStyle.Render("no slices match this filter")
	if sl, ok := m.previewSlice(vis); ok {
		title = sl.Name
		preview = previewContent(m, sl)
	}
	rightBox := panelBox(title, preview, rightW, bodyH, true)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, rightBox)
	return top + "\n" + body + "\n" + footer
}

// statesContent renders the state-filter rail with per-state counts.
func statesContent(m Model) string {
	var sb strings.Builder
	for i, f := range hubFilters() {
		marker := "  "
		if i == m.filterIdx {
			marker = "▸ "
		}
		row := fmt.Sprintf("%s%-11s %2d", marker, f.label, m.filterCount(i))
		if i == m.filterIdx {
			row = focusStyle.Render(row)
		}
		sb.WriteString(row + "\n")
	}
	return sb.String()
}

// slicesContent renders the (filtered) slice list, windowed around the selection.
func slicesContent(m Model, vis []int, rows int) string {
	if len(vis) == 0 {
		return overviewStyle.Render("(none)")
	}
	if rows < 1 {
		rows = 1
	}
	pos := 0
	for p, i := range vis {
		if i == m.focus {
			pos = p
			break
		}
	}
	start := clamp(pos-rows/2, 0, max(0, len(vis)-rows))
	end := min(start+rows, len(vis))

	var sb strings.Builder
	for _, i := range vis[start:end] {
		s := m.slices[i]
		marker := "  "
		switch {
		case m.selected[s.Name]:
			marker = syncedStyle.Render("✓") + " "
		case i == m.focus:
			marker = cursorBar.Render("▎") + " "
		}
		name := s.Name
		if i == m.focus {
			name = focusStyle.Render(name)
		}
		// Live state is carried by the (green) status glyph, not a second dot on
		// the name — avoids the confusing "● ●name" double-dot.
		sb.WriteString(marker + sliceGlyph(m, s) + " " + name + "\n")
	}
	return sb.String()
}

// sliceGlyph is the compact status glyph for a slice in the list.
func sliceGlyph(m Model, s model.Slice) string {
	switch m.workState(s) {
	case stNeedsYou:
		switch m.sessionStatus[s.Name] {
		case model.SessWaitingInput:
			return waitStyle.Render("⏸")
		case model.SessDone:
			return doneStyle.Render("✦") // finished a turn — your move
		default:
			return "❌" // CI failing
		}
	case stReady:
		return readyStyle.Render("♻")
	case stInReview:
		return syncedStyle.Render("✓")
	default:
		if s.Active {
			// Live: this slice is swapped into the repos' primaries.
			return liveStyle.Render("●")
		}
		if m.sessionStatus[s.Name] == model.SessRunning {
			return "●"
		}
		return overviewStyle.Render("·")
	}
}

// previewContent renders a live mini-cockpit for the selected slice: tags, each
// repo's branch + PR/CI, the overview, and a snippet of recent changes.
func previewContent(m Model, sl model.Slice) string {
	var sb strings.Builder

	var tags []string
	if sl.Active {
		tags = append(tags, liveStyle.Render("● live"))
	}
	if m.sliceMergeState(sl) == mergeReady {
		tags = append(tags, readyStyle.Render("♻ ready to clear"))
	}
	if m.sessionStatus[sl.Name] == model.SessWaitingInput {
		tags = append(tags, waitStyle.Render("⏸ needs you"))
	}
	if c, ok := m.cards[sl.Name]; ok && c.restack > 0 {
		tags = append(tags, needsRestackStyle.Render(fmt.Sprintf("⟳ %d need restack", c.restack)))
	}
	if len(tags) > 0 {
		sb.WriteString(strings.Join(tags, "  ") + "\n\n")
	}

	prefix := m.ws.Grouping.StripPrefix
	slicePRs := m.prs[sl.Name]
	for _, repo := range sl.Repos() {
		mem := sl.Members[repo]
		sb.WriteString(repoHeaderStyle.Render(repo) + "  " + overviewStyle.Render(shortBranch(mem.Branch, prefix)))
		if slicePRs != nil {
			if pr := slicePRs[repo]; pr != nil {
				if strings.EqualFold(pr.State, "MERGED") {
					sb.WriteString("  " + mergedStyle.Render(fmt.Sprintf("#%d merged", pr.Number)))
				} else {
					overall, _, _, _ := pr.CISummary()
					sb.WriteString(fmt.Sprintf("  #%d %s 💬%d", pr.Number, forge.CIEmoji(overall), len(pr.Comments)))
				}
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n" + overviewStyle.Render(cardOverview(m, sl)) + "\n")

	// Most recent Claude/session output (live), when a session exists.
	if cap, ok := m.captures[sl.Name]; ok && strings.TrimSpace(cap) != "" {
		sb.WriteString("\n" + colHeadStyle.Render("── recent session output (live) ──") + "\n")
		sb.WriteString(tailLines(cap, 10) + "\n")
	}

	sb.WriteString("\n" + colHeadStyle.Render("── recent changes ──") + "\n")
	switch {
	case m.diffLoading[sl.Name]:
		sb.WriteString(overviewStyle.Render("loading diff…"))
	default:
		diffs, ok := m.diffs[sl.Name]
		if !ok {
			sb.WriteString(overviewStyle.Render("select to load…"))
			break
		}
		// Show every repo (not just the first) with a clear header naming the
		// repo, its stat, and the base it is diffed against — so it's obvious
		// what the changes are and from where. Each patch is capped; the full
		// diff is in the cockpit ([enter]).
		prefix := m.ws.Grouping.StripPrefix
		for _, rd := range diffs {
			base := shortBranch(rd.Base, prefix)
			switch {
			case rd.Err != "":
				sb.WriteString(diffHdrStyle.Render("▸ "+rd.Repo) + " " +
					overviewStyle.Render("diff unavailable") + "\n")
			case rd.Patch == "":
				sb.WriteString(diffHdrStyle.Render("▸ "+rd.Repo) + " " +
					overviewStyle.Render("no changes vs "+base) + "\n")
			default:
				sb.WriteString(diffHdrStyle.Render(fmt.Sprintf("▸ %s  +%d -%d · vs %s",
					rd.Repo, rd.TotalAdded(), rd.TotalDeleted(), base)) + "\n")
				body, more := headLines(colorizePatch(rd.Patch), 10)
				sb.WriteString(body + "\n")
				if more > 0 {
					sb.WriteString(overviewStyle.Render(
						fmt.Sprintf("    … %d more lines — [enter] for full diff", more)) + "\n")
				}
			}
		}
	}
	return sb.String()
}

// cardOverview returns the one-line "what is this slice about" text: the PR
// title if a PR is loaded, else the newest commit subject, plus diffstat.
func cardOverview(m Model, s model.Slice) string {
	c, loaded := m.cards[s.Name]
	if !loaded {
		return "loading…"
	}

	desc := c.overview
	if slicePRs, ok := m.prs[s.Name]; ok {
		for _, repo := range s.Repos() {
			if pr := slicePRs[repo]; pr != nil && pr.Title != "" {
				desc = pr.Title
				break
			}
		}
	}
	if desc == "" {
		desc = "(no commits vs trunk)"
	}

	stat := fmt.Sprintf(" · +%d −%d · %d commit%s", c.added, c.deleted, c.commits, plural(c.commits))
	descW := clamp(m.width-lipgloss.Width(stat)-5, 10, m.width)
	return clip(desc, descW) + stat
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// updateBrowserKeys handles key events while the dashboard hub is showing.
func (m Model) updateBrowserKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// New-slice name input — allowed even with 0 slices (that's when you most
	// need it).
	if m.creating {
		switch msg.Type {
		case tea.KeyEnter:
			name := strings.TrimSpace(m.createName)
			m.creating = false
			m.createName = ""
			if name != "" {
				return m, slisCreateCmd(name)
			}
		case tea.KeyEsc:
			m.creating = false
			m.createName = ""
		case tea.KeyBackspace:
			if len(m.createName) > 0 {
				m.createName = m.createName[:len(m.createName)-1]
			}
		case tea.KeyRunes, tea.KeySpace:
			m.createName += string(msg.Runes)
		}
		return m, nil
	}

	// Empty workspace: only [c] (create) is useful; ignore other browser keys
	// (q / r / ? still work — handled globally). Status was cleared in handleKey.
	if len(m.slices) == 0 {
		if msg.String() == "c" {
			m.creating = true
			m.createName = ""
		}
		return m, nil
	}

	// Text-search input.
	if m.filtering {
		switch msg.Type {
		case tea.KeyEnter:
			m.filtering = false
		case tea.KeyEsc:
			m.filtering = false
			m.filter = ""
		case tea.KeyBackspace:
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
			}
		case tea.KeyRunes, tea.KeySpace:
			m.filter += string(msg.Runes)
		}
		m.snapFocusToFilter()
		return m, m.loadPreview()
	}

	// Group-name input mode (after selecting slices with space, pressing m).
	if m.naming {
		switch msg.Type {
		case tea.KeyEnter:
			name := strings.TrimSpace(m.groupName)
			m.naming = false
			m.groupName = ""
			if name != "" && len(m.selected) > 0 {
				cmd := m.groupSelectedCmd(name)
				m.selected = make(map[string]bool)
				m.focus = 0
				return m, cmd
			}
		case tea.KeyEsc:
			m.naming = false
			m.groupName = ""
		case tea.KeyBackspace:
			if len(m.groupName) > 0 {
				m.groupName = m.groupName[:len(m.groupName)-1]
			}
		case tea.KeyRunes, tea.KeySpace:
			m.groupName += string(msg.Runes)
		}
		return m, nil
	}

	vis := m.hubVisible()
	pos := posInFiltered(vis, m.focus)
	nFilters := len(hubFilters())

	switch msg.String() {
	case "/":
		m.filtering = true
		return m, nil
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.snapFocusToFilter()
		}
		return m, nil
	case "tab", "shift+tab":
		m.hubFocus = (m.hubFocus + 1) % 2
		return m, nil
	case "j", "down":
		if m.hubFocus == 1 {
			m.filterIdx = clamp(m.filterIdx+1, 0, nFilters-1)
			m.snapFocusToFilter()
			return m, m.loadPreview()
		}
		if pos >= 0 && pos < len(vis)-1 {
			m.focus = vis[pos+1]
			return m, m.loadPreview()
		}
	case "k", "up":
		if m.hubFocus == 1 {
			m.filterIdx = clamp(m.filterIdx-1, 0, nFilters-1)
			m.snapFocusToFilter()
			return m, m.loadPreview()
		}
		if pos > 0 {
			m.focus = vis[pos-1]
			return m, m.loadPreview()
		}
	case "1", "2", "3", "4", "5", "6", "7", "8":
		if idx := int(msg.String()[0] - '1'); idx < nFilters {
			m.filterIdx = idx
			m.snapFocusToFilter()
			return m, m.loadPreview()
		}
	case "g":
		if len(vis) > 0 {
			m.focus = vis[0]
			return m, m.loadPreview()
		}
	case "G":
		if len(vis) > 0 {
			m.focus = vis[len(vis)-1]
			return m, m.loadPreview()
		}
	case "enter", "l", "right":
		if _, ok := m.currentSlice(); ok {
			return m, m.enterCockpit()
		}
	case "a":
		return m, m.attachCmd()
	case "C":
		return m, m.launchAgentCmd()
	case "c":
		m.creating = true
		m.createName = ""
		return m, nil
	case "w":
		m.requestSwap()
		return m, nil
	case "d":
		m.requestRemove()
		return m, nil
	case "R":
		m.requestStack()
		return m, nil
	case "n":
		order := m.attentionOrder()
		if len(order) == 0 {
			m.status = "inbox zero — nothing needs you 🎉"
			return m, nil
		}
		p := posInFiltered(order, m.focus) // -1 if focus not in inbox
		m.focus = order[(p+1+len(order))%len(order)]
		return m, m.loadPreview()
	case "N":
		order := m.attentionOrder()
		if len(order) == 0 {
			return m, nil
		}
		p := posInFiltered(order, m.focus)
		if p < 0 {
			p = 0
		}
		m.focus = order[(p-1+len(order))%len(order)]
		return m, m.loadPreview()
	case "A":
		allSel := len(vis) > 0
		for _, i := range vis {
			if !m.selected[m.slices[i].Name] {
				allSel = false
				break
			}
		}
		for _, i := range vis {
			if allSel {
				delete(m.selected, m.slices[i].Name)
			} else {
				m.selected[m.slices[i].Name] = true
			}
		}
		return m, nil
	case " ":
		if sl, ok := m.currentSlice(); ok {
			if m.selected[sl.Name] {
				delete(m.selected, sl.Name)
			} else {
				m.selected[sl.Name] = true
			}
		}
		return m, nil
	case "m":
		if len(m.selected) > 0 {
			m.naming = true
			m.groupName = ""
		} else {
			m.status = "select slices with space, then m to group them"
		}
		return m, nil
	case "u":
		if sl, ok := m.currentSlice(); ok {
			m.status = "ungrouped " + sl.Name
			return m, m.ungroupCmd(sl.Name)
		}
	case "Y":
		if cmd := copyPRStackCmd(m); cmd != nil {
			m.status = "copied PR stack to clipboard"
			return m, cmd
		}
		return m, m.maybeLoadPRs()
	}
	return m, nil
}

// snapFocusToFilter ensures m.focus points at a slice visible under the active
// state + text filter.
func (m *Model) snapFocusToFilter() {
	vis := m.hubVisible()
	if len(vis) == 0 {
		return
	}
	if posInFiltered(vis, m.focus) < 0 {
		m.focus = vis[0]
	}
}

// loadPreview lazily loads the stack, diff, PRs and tmux capture for the focused
// slice so the right-hand preview pane can render it.
func (m *Model) loadPreview() tea.Cmd {
	return tea.Batch(filterNil([]tea.Cmd{m.maybeLoadStack(), m.maybeLoadDiff(), m.maybeLoadPRs(), m.maybeLoadCapture()})...)
}
