package tui

import (
	"fmt"
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
	waitStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true) // needs-input
	liveStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)  // currently-active slice
)

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
// subjects/count, diffstat (numstat only), and stack health — each per repo,
// using auto-detected trunks.
func loadCardCmd(sl model.Slice) tea.Cmd {
	return func() tea.Msg {
		var card sliceCard

		byRepo, _ := summary.CommitSummary(sl, "")
		for _, repo := range sl.Repos() {
			subs := byRepo[repo]
			card.commits += len(subs)
			if card.overview == "" && len(subs) > 0 {
				card.overview = subs[0] // newest first
			}
		}

		stats, _ := diff.SliceStat(sl, "")
		for _, rd := range stats {
			card.added += rd.TotalAdded()
			card.deleted += rd.TotalDeleted()
		}

		for _, repo := range sl.Repos() {
			member := sl.Members[repo]
			if member.WorktreePath == "" {
				continue
			}
			st, err := gt.ReadState(member.WorktreePath)
			if err != nil || len(st) == 0 {
				continue
			}
			lineage := st.Lineage(member.Branch)
			if len(lineage) == 0 {
				continue // branch isn't in a Graphite stack — leave health unknown
			}
			card.stackKnown = true
			for _, b := range lineage {
				if b.NeedsRestack {
					card.restack++
				}
			}
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

// filteredIndices returns the indices into m.slices that match the active
// filter, in order. An empty filter matches everything.
func (m Model) filteredIndices() []int {
	var out []int
	f := strings.ToLower(m.filter)
	for i, s := range m.slices {
		if f == "" || strings.Contains(strings.ToLower(s.Name), f) {
			out = append(out, i)
		}
	}
	return out
}

// posInFiltered returns the position of m.focus within the filtered list, or -1.
func posInFiltered(idxs []int, focus int) int {
	for p, i := range idxs {
		if i == focus {
			return p
		}
	}
	return -1
}

// browserCols returns the column widths for the browser table given total width.
func browserCols(w int) (nameW, reposW, stackW, prW int) {
	reposW, stackW, prW = 22, 13, 12
	nameW = w - 2 - reposW - stackW - prW - 9 - 4 // cursor(2) + session(9) + 4 gaps
	nameW = clamp(nameW, 18, 60)
	return
}

// padCol clips a (possibly styled) cell to w cells and left-pads to width w.
func padCol(s string, w int) string { return padCell(clip(s, w), w) }

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

// renderBrowser renders the slice browser (landing screen / project hub).
func renderBrowser(m Model) string {
	idxs := m.filteredIndices()
	nameW, reposW, stackW, prW := browserCols(m.width)

	// ── Top header: identity, live slice, needs-input, filter. ──
	var head strings.Builder
	head.WriteString(titleStyle.Render("slis"))
	head.WriteString(headerStyle.Render(fmt.Sprintf("  ·  %d slices", len(m.slices))))
	if live := m.activeName(); live != "" {
		head.WriteString("   " + liveStyle.Render("● live: "+live))
	}
	if w := m.waitingCount(); w > 0 {
		head.WriteString("   " + waitStyle.Render(fmt.Sprintf("⏸ %d need input", w)))
	}
	if m.filtering || m.filter != "" {
		cur := ""
		if m.filtering {
			cur = "▏"
		}
		head.WriteString("   " + headerStyle.Render("/") + m.filter + cur)
	}
	if n := len(m.selected); n > 0 {
		head.WriteString("   " + focusStyle.Render(fmt.Sprintf("%d selected", n)))
	}
	if m.naming {
		head.WriteString("   " + headerStyle.Render("group name: ") + m.groupName + "▏")
	}
	header := clip(head.String(), m.width)

	// ── Column header row. ──
	colHead := colHeadStyle.Render(
		"  " + padCol("SLICE", nameW) + " " + padCol("REPOS", reposW) + " " +
			padCol("STACK", stackW) + " " + padCol("PR", prW) + " " + "SESSION")
	rule := footerStyle.Render(strings.Repeat("─", clamp(m.width, 1, m.width)))

	footerText := "enter open · space select · m group · u ungroup · w live · d clear · Y copy · / filter · ? help"
	if m.status != "" {
		footerText = m.status
	}
	footer := clip(footerStyle.Render(footerText), m.width)

	if len(m.slices) == 0 {
		return header + "\n\n  No slices found. Run 'slis init' to set up your workspace.\n\n" + footer
	}
	if len(idxs) == 0 {
		return header + "\n\n" + colHead + "\n" + rule + "\n  " +
			overviewStyle.Render("no slices match /"+m.filter) + "\n\n" + footer
	}

	// Adaptive vertical density: 3 lines/card (with a blank separator) when the
	// terminal is tall enough, else a compact 2 lines/card.
	perCard := 2
	visible := len(idxs)
	if m.height > 0 {
		bodyH := m.height - 4 // header(2) + colhead+rule(2)... leave room for footer
		if bodyH < 2 {
			bodyH = 2
		}
		if bodyH >= len(idxs)*3 {
			perCard = 3
		}
		visible = bodyH / perCard
		if visible < 1 {
			visible = 1
		}
	}

	pos := posInFiltered(idxs, m.focus)
	if pos < 0 {
		pos = 0
	}
	start := clamp(pos-visible/2, 0, max(0, len(idxs)-visible))
	end := min(start+visible, len(idxs))

	var body strings.Builder
	for _, i := range idxs[start:end] {
		body.WriteString(renderCard(m, i, i == m.focus, perCard == 3))
	}

	return header + "\n\n" + colHead + "\n" + rule + "\n" +
		strings.TrimRight(body.String(), "\n") + "\n" + footer
}

// renderCard renders one slice row: an identity+status line, then a dim overview
// line, optionally followed by a blank spacer line for breathing room.
func renderCard(m Model, idx int, focused, spacer bool) string {
	s := m.slices[idx]
	nameW, reposW, stackW, prW := browserCols(m.width)

	cursor := "  "
	switch {
	case m.selected[s.Name]:
		cursor = syncedStyle.Render("✓") + " "
	case focused:
		cursor = cursorBar.Render("▎") + " "
	}

	status := model.SessNone
	if m.sessionStatus != nil {
		status = m.sessionStatus[s.Name]
	}

	// Name cell, marked when this slice is the live (swapped-in) one.
	disp := s.Name
	if s.Active {
		disp = "● " + s.Name
	}
	nameCell := clip(disp, nameW)
	switch {
	case s.Active:
		nameCell = liveStyle.Render(nameCell)
	case focused:
		nameCell = focusStyle.Render(nameCell)
	}
	nameCell = padCell(nameCell, nameW)

	line1 := cursor + nameCell + " " +
		padCol(repoTags(s), reposW) + " " +
		padCol(stackBadge(m, s), stackW) + " " +
		padCol(prBadge(m, s), prW) + " " +
		sessionCell(status)

	out := clip(line1, m.width) + "\n" + clip("    "+overviewStyle.Render(cardOverview(m, s)), m.width) + "\n"
	if spacer {
		out += "\n"
	}
	return out
}

// repoTags lists the slice's member repos by name (the columns header is REPOS).
func repoTags(s model.Slice) string {
	return strings.Join(s.Repos(), " ")
}

// stackBadge renders the slice's stack health from its card, in words.
func stackBadge(m Model, s model.Slice) string {
	c, ok := m.cards[s.Name]
	if !ok {
		return overviewStyle.Render("…")
	}
	if !c.stackKnown {
		return overviewStyle.Render("—")
	}
	if c.restack == 0 {
		return syncedStyle.Render("✓ synced")
	}
	return needsRestackStyle.Render(fmt.Sprintf("⚠ %d restack", c.restack))
}

// prBadge renders PR + CI for the slice (first repo with a PR).
func prBadge(m Model, s model.Slice) string {
	slicePRs, ok := m.prs[s.Name]
	if !ok {
		if m.prLoading[s.Name] {
			return overviewStyle.Render("…")
		}
		return overviewStyle.Render("—")
	}
	for _, repo := range s.Repos() {
		if pr := slicePRs[repo]; pr != nil {
			overall, _, _, _ := pr.CISummary()
			return fmt.Sprintf("#%d %s", pr.Number, forge.CIEmoji(overall))
		}
	}
	return overviewStyle.Render("no PR")
}

// sessionCell renders the session status as glyph + word, highlighting "wait".
func sessionCell(s model.SessionStatus) string {
	switch s {
	case model.SessWaitingInput:
		return waitStyle.Render("⏸ wait")
	case model.SessRunning:
		return "● run"
	case model.SessDone:
		return syncedStyle.Render("✓ done")
	default:
		return overviewStyle.Render("○ idle")
	}
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

// updateBrowserKeys handles key events while the browser is showing.
func (m Model) updateBrowserKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		return m, m.loadFocusedPRs()
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
				m.focus = 0 // grouped slice list changes; reset to top
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

	idxs := m.filteredIndices()
	pos := posInFiltered(idxs, m.focus)

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
	case "j", "down":
		if pos >= 0 && pos < len(idxs)-1 {
			m.focus = idxs[pos+1]
			return m, m.loadFocusedPRs()
		}
	case "k", "up":
		if pos > 0 {
			m.focus = idxs[pos-1]
			return m, m.loadFocusedPRs()
		}
	case "g":
		if len(idxs) > 0 {
			m.focus = idxs[0]
			return m, m.loadFocusedPRs()
		}
	case "G":
		if len(idxs) > 0 {
			m.focus = idxs[len(idxs)-1]
			return m, m.loadFocusedPRs()
		}
	case "enter", "l", "right":
		if _, ok := m.currentSlice(); ok {
			return m, m.enterCockpit()
		}
	case "a":
		return m, m.attachCmd()
	case "w":
		m.requestSwap()
		return m, nil
	case "d":
		m.requestRemove()
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

// snapFocusToFilter ensures m.focus points at a slice matching the filter.
func (m *Model) snapFocusToFilter() {
	idxs := m.filteredIndices()
	if len(idxs) == 0 {
		return
	}
	if posInFiltered(idxs, m.focus) < 0 {
		m.focus = idxs[0]
	}
}

// loadFocusedPRs lazily loads PR data for the focused slice (used to fill the
// browser's PR badge and overview title without a gh storm across all slices).
func (m *Model) loadFocusedPRs() tea.Cmd {
	return m.maybeLoadPRs()
}
